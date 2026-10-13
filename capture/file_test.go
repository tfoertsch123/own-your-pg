package capture

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/tfoertsch123/flock"
	"github.com/tfoertsch123/log"
	"github.com/tfoertsch123/own-your-pg/defaults"
	"github.com/tfoertsch123/own-your-pg/slot"
	mylsn "github.com/tfoertsch123/own-your-pg/lsn"
)

// newFileTestCfg creates a session with a slot, mgr, and the incoming directory
// structure set up in a temp dir, ready for file I/O tests.
func newFileTestCfg(t *testing.T) *session {
	t.Helper()
	dir := t.TempDir()

	mgr, err := slot.NewMgr(dir, slot.WithMgrCreate())
	if err != nil {
		t.Fatalf("NewMgr: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })

	sl, err := mgr.Slot("test", slot.WithAsOwner(), slot.WithType(slot.Producer))
	if err != nil {
		t.Fatalf("Slot: %v", err)
	}
	t.Cleanup(func() { sl.Close() })

	s := &session{
		wd:        dir,
		mgr:       mgr,
		sl:        sl,
		currDirFd: -1,
	}
	s.ensureIncDir()
	t.Cleanup(func() {
		if s.currDirFd >= 0 {
			unix.Close(s.currDirFd)
		}
		if s.histDirFd >= 0 {
			unix.Close(s.histDirFd)
		}
		if s.meta != nil {
			s.meta.Close()
		}
	})

	// Set up loggers so functions that use mlg/lg don't panic
	s.lg = log.NewR(log.WithTopic("MAIN"))
	s.mlg = s.lg.New(log.WithTopic("WRT"))

	return s
}

// ---- writeMeta / readMeta round-trip ----

func TestWriteReadMeta_RoundTrip(t *testing.T) {
	s := newFileTestCfg(t)

	flsn := mylsn.LSN(0x1234567890ABCDEF)
	llsn := mylsn.LSN(0xFEDCBA0987654321)
	epos := int64(4096)

	if err := s.writeMeta(epos, &flsn, llsn); err != nil {
		t.Fatalf("writeMeta: %v", err)
	}

	gotEpos, gotFlsn, gotLlsn, err := s.readMeta()
	if err != nil {
		t.Fatalf("readMeta: %v", err)
	}
	if gotEpos != epos {
		t.Errorf("epos: got %d, want %d", gotEpos, epos)
	}
	if gotFlsn == nil || *gotFlsn != flsn {
		t.Errorf("flsn: got %v, want %v", gotFlsn, flsn)
	}
	if gotLlsn != llsn {
		t.Errorf("llsn: got %v, want %v", gotLlsn, llsn)
	}
}

func TestWriteReadMeta_NilFlsn(t *testing.T) {
	s := newFileTestCfg(t)

	llsn := mylsn.LSN(0xAAAA)
	epos := int64(100)

	if err := s.writeMeta(epos, nil, llsn); err != nil {
		t.Fatalf("writeMeta: %v", err)
	}

	gotEpos, gotFlsn, gotLlsn, err := s.readMeta()
	if err != nil {
		t.Fatalf("readMeta: %v", err)
	}
	if gotEpos != epos {
		t.Errorf("epos: got %d, want %d", gotEpos, epos)
	}
	if gotFlsn != nil {
		t.Errorf("flsn: got %v, want nil", gotFlsn)
	}
	if gotLlsn != llsn {
		t.Errorf("llsn: got %v, want %v", gotLlsn, llsn)
	}
}

func TestReadMeta_EmptyFile(t *testing.T) {
	s := newFileTestCfg(t)

	_, _, _, err := s.readMeta()
	if err != ErrMetaGarbage {
		t.Errorf("readMeta on empty file: got %v, want %v", err, ErrMetaGarbage)
	}
}

func TestReadMeta_ShortFile(t *testing.T) {
	s := newFileTestCfg(t)

	var bts [8]byte
	binary.BigEndian.PutUint64(bts[:], 42)
	if _, err := s.meta.WriteAt(bts[:], 0); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}

	_, _, _, err := s.readMeta()
	if err != ErrMetaGarbage {
		t.Errorf("readMeta on short file: got %v, want %v", err, ErrMetaGarbage)
	}
}

// ---- newCur ----

func TestNewCur_EmptyFile(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	llsn, err := s.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}
	if llsn != 0 {
		t.Errorf("llsn: got %v, want 0", llsn)
	}
	if s.curr == nil {
		t.Error("s.curr should be set")
	}
	if s.currLck == nil {
		t.Error("s.currLck should be set")
	}
	if s.eoCommit != 0 {
		t.Errorf("eoCommit: got %d, want 0", s.eoCommit)
	}
	if s.firstTxnLSN != nil {
		t.Errorf("firstTxnLSN: got %v, want nil", s.firstTxnLSN)
	}

	eoc, flsn, llsn2, err := s.readMeta()
	if err != nil {
		t.Fatalf("readMeta: %v", err)
	}
	if eoc != 0 {
		t.Errorf("meta eoc: got %d, want 0", eoc)
	}
	if flsn != nil {
		t.Errorf("meta flsn: got %v, want nil", flsn)
	}
	if llsn2 != 0 {
		t.Errorf("meta llsn: got %v, want 0", llsn2)
	}
}

func TestNewCur_ExistingFile(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	llsn1, err := s.newCur()
	if err != nil {
		t.Fatalf("first newCur: %v", err)
	}
	if llsn1 != 0 {
		t.Fatalf("first llsn: got %v, want 0", llsn1)
	}

	// Write data and metadata to simulate a file with one commit
	testData := []byte(`{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}
{"action":"C","nextlsn":"0/100"}
`)
	if _, err := s.curr.Write(testData); err != nil {
		t.Fatalf("Write: %v", err)
	}
	eof := int64(len(testData))
	flsn := mylsn.LSN(0x100)
	llsn := mylsn.LSN(0x100)
	if err := s.writeMeta(eof, &flsn, llsn); err != nil {
		t.Fatalf("writeMeta: %v", err)
	}
	s.curr.Close()
	s.curr = nil
	if s.currLck != nil {
		s.currLck.Close()
		s.currLck = nil
	}

	llsn2, err := s.newCur()
	if err != nil {
		t.Fatalf("second newCur: %v", err)
	}
	if llsn2 != llsn {
		t.Errorf("llsn: got %v, want %v", llsn2, llsn)
	}
	if s.eoCommit != eof {
		t.Errorf("eoCommit: got %d, want %d", s.eoCommit, eof)
	}
	if s.firstTxnLSN == nil || *s.firstTxnLSN != flsn {
		t.Errorf("firstTxnLSN: got %v, want %v", s.firstTxnLSN, flsn)
	}
}

func TestNewCur_Garbage(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	_, err := s.newCur()
	if err != nil {
		t.Fatalf("first newCur: %v", err)
	}

	// Write some data to the current file so eof > 0
	if _, err := s.curr.Write([]byte("some data")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Write metadata with eoc pointing far beyond EOF
	if err := s.writeMeta(99999, nil, mylsn.LSN(0)); err != nil {
		t.Fatalf("writeMeta: %v", err)
	}
	s.curr.Close()
	s.curr = nil
	if s.currLck != nil {
		s.currLck.Close()
		s.currLck = nil
	}

	_, err = s.newCur()
	if err != ErrCurrGarbage {
		t.Errorf("newCur with garbage: got %v, want %v", err, ErrCurrGarbage)
	}
}

// ---- writeData / flush ----

func TestWriteData(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	_, err := s.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	data := []byte(`{"action":"I","xid":1}`)
	if err := s.writeData(data); err != nil {
		t.Fatalf("writeData: %v", err)
	}
	if s.writer.Len() != len(data)+1 {
		t.Errorf("writer.Len(): got %d, want %d",
			s.writer.Len(), len(data)+1)
	}

	n, err := s.flush()
	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	if n != int64(len(data)+1) {
		t.Errorf("flush n: got %d, want %d", n, int64(len(data)+1))
	}
	if s.writer.Len() != 0 {
		t.Errorf("writer.Len() after flush: got %d, want 0",
			s.writer.Len())
	}
}

func TestWriteData_AutoFlush(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	_, err := s.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	// Write >64KB to trigger auto-flush
	chunk := make([]byte, 32768)
	for i := range chunk {
		chunk[i] = 'x'
	}
	if err := s.writeData(chunk); err != nil {
		t.Fatalf("writeData 1: %v", err)
	}
	if err := s.writeData(chunk); err != nil {
		t.Fatalf("writeData 2: %v", err)
	}

	if s.writer.Len() == 0 {
		eof, err := s.curr.Seek(0, 2)
		if err != nil {
			t.Fatalf("Seek: %v", err)
		}
		if eof <= 0 {
			t.Error("file should have content after auto-flush")
		}
	}
}

// ---- truncateToEoc ----

func TestTruncateToEoc(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	_, err := s.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	committed := []byte(`{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}
{"action":"C","nextlsn":"0/100"}
`)
	if _, err := s.curr.Write(committed); err != nil {
		t.Fatalf("Write committed: %v", err)
	}
	s.eoCommit = int64(len(committed))

	uncommitted := []byte(`{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}
{"action":"I","xid":2}
`)
	if _, err := s.curr.Write(uncommitted); err != nil {
		t.Fatalf("Write uncommitted: %v", err)
	}

	s.writer.Write([]byte("buffered"))

	if err := s.truncateToEoc(); err != nil {
		t.Fatalf("truncateToEoc: %v", err)
	}

	eof, err := s.curr.Seek(0, 2)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if eof != s.eoCommit {
		t.Errorf("eof after truncate: got %d, want %d",
			eof, s.eoCommit)
	}

	if s.writer.Len() != 0 {
		t.Errorf("writer.Len(): got %d, want 0", s.writer.Len())
	}
}

func TestTruncateToEoc_NoTruncationNeeded(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	_, err := s.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	s.eoCommit = 0

	if err := s.truncateToEoc(); err != nil {
		t.Fatalf("truncateToEoc: %v", err)
	}

	eof, err := s.curr.Seek(0, 2)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if eof != 0 {
		t.Errorf("eof: got %d, want 0", eof)
	}
}

// ---- eoc ----

func TestEoc_Commit(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))
	s.maxSize = 16 * 1024 * 1024

	_, err := s.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	beginJSON := `{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}`
	if err := s.writeData([]byte(beginJSON)); err != nil {
		t.Fatalf("writeData B: %v", err)
	}

	commitLSN := mylsn.LSN(0x200)

	commitJSON := `{"action":"C","nextlsn":"0/200"}`
	if err := s.writeData([]byte(commitJSON)); err != nil {
		t.Fatalf("writeData C: %v", err)
	}

	if err := s.eoc(commitLSN, true); err != nil {
		t.Fatalf("eoc: %v", err)
	}

	if s.eoCommit == 0 {
		t.Error("eoCommit should have advanced from 0")
	}

	if s.firstTxnLSN == nil {
		t.Error("firstTxnLSN should be set")
	}

	eoc, flsn, llsn, err := s.readMeta()
	if err != nil {
		t.Fatalf("readMeta: %v", err)
	}
	if eoc != s.eoCommit {
		t.Errorf("meta eoc: got %d, want %d", eoc, s.eoCommit)
	}
	if flsn == nil || *flsn != commitLSN {
		t.Errorf("meta flsn: got %v, want %v", flsn, commitLSN)
	}
	if llsn != commitLSN {
		t.Errorf("meta llsn: got %v, want %v", llsn, commitLSN)
	}

	slotLSN, _ := s.sl.GetLSN()
	if slotLSN != commitLSN {
		t.Errorf("slot LSN: got %v, want %v", slotLSN, commitLSN)
	}

	// The B record's placeholder should have been replaced
	eof, err := s.curr.Seek(0, 2)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	}
	buf := make([]byte, eof)
	if _, err := s.curr.ReadAt(buf, 0); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if bytes.Contains(buf, []byte("XXXXXXXX/YYYYYYYY")) {
		t.Error("placeholder should have been replaced")
	}
}

func TestEoc_Rotation(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	s.maxSize = 1

	_, err := s.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	beginJSON := `{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}`
	if err := s.writeData([]byte(beginJSON)); err != nil {
		t.Fatalf("writeData B: %v", err)
	}
	commitJSON := `{"action":"C","nextlsn":"0/200"}`
	if err := s.writeData([]byte(commitJSON)); err != nil {
		t.Fatalf("writeData C: %v", err)
	}

	commitLSN := mylsn.LSN(0x200)
	if err := s.eoc(commitLSN, true); err != nil {
		t.Fatalf("eoc: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(s.wd))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	found := false
	for _, e := range entries {
		name := e.Name()
		if name != defaults.IncDir && name != defaults.SlotDir &&
			!strings.HasPrefix(name, ".") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a history file after rotation")
	}

	if s.curr == nil {
		t.Error("s.curr should be set after rotation")
	}
	if s.firstTxnLSN != nil {
		t.Error("firstTxnLSN should be nil after rotation")
	}
}

// ---- ensureIncDir ----

func TestEnsureIncDir(t *testing.T) {
	dir := t.TempDir()
	sd := filepath.Join(dir, defaults.SlotDir)

	mgr, err := slot.NewMgr(sd, slot.WithMgrCreate())
	if err != nil {
		t.Fatalf("NewMgr: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })

	sl, err := mgr.Slot("test", slot.WithAsOwner(), slot.WithType(slot.Producer))
	if err != nil {
		t.Fatalf("Slot: %v", err)
	}
	t.Cleanup(func() { sl.Close() })

	s := &session{
		wd:        dir,
		mgr:       mgr,
		sl:        sl,
		currDirFd: -1,
	}
	s.ensureIncDir()
	t.Cleanup(func() {
		if s.currDirFd >= 0 {
			unix.Close(s.currDirFd)
		}
		if s.histDirFd >= 0 {
			unix.Close(s.histDirFd)
		}
		if s.meta != nil {
			s.meta.Close()
		}
	})

	incDir := filepath.Join(dir, defaults.IncDir)
	// incDir is created relative to the slot dir as ../incomplete
	info, err := os.Stat(incDir)
	if err != nil {
		t.Fatalf("Stat incDir: %v", err)
	}
	if !info.IsDir() {
		t.Error("incDir should be a directory")
	}

	metaPath := filepath.Join(incDir, defaults.MetaName)
	_ = metaPath // used below
	_, err = os.Stat(metaPath)
	if err != nil {
		t.Fatalf("Stat meta: %v", err)
	}

	if s.currDirFd < 0 {
		t.Error("currDirFd should be set")
	}
	if s.histDirFd < 0 {
		t.Error("histDirFd should be set")
	}
}

// ---- curInit ----

func TestCurInit(t *testing.T) {
	s := newFileTestCfg(t)

	llsn := s.curInit()
	if llsn != 0 {
		t.Errorf("curInit llsn: got %v, want 0", llsn)
	}
	if s.curr == nil {
		t.Error("curr should be set")
	}
	if s.writer == nil {
		t.Error("writer should be set")
	}
	if s.writer.Cap() != 65*1024 {
		t.Errorf("writer cap: got %d, want %d",
			s.writer.Cap(), 65*1024)
	}
}

// ---- flock integration ----

func TestNewCur_FlockStored(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	_, err := s.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	if s.currLck == nil {
		t.Fatal("currLck should be set after newCur")
	}
	if !s.currLck.IsOpen() {
		t.Error("currLck should be open after newCur")
	}
	if s.curr == nil {
		t.Error("curr should be set after newCur")
	}
}

// ---- range locking ----

// tryReaderLock opens the current file on a separate flock.Lock (simulating a
// reader process) and tries to acquire a non-blocking shared range lock at
// the given position. Returns whether the lock was acquired.
func tryReaderLock(s *session, start, length int64) bool {
	lck := flock.New(
		flock.WithPathAt(s.currDirFd),
		flock.WithPath(defaults.CurFile),
	)
	if err := lck.Open(); err != nil {
		return false
	}
	defer lck.Close()
	return lck.TryLockRangeSh(start, length) == nil
}

// TestRangeLock_NewCur_LocksAtEoc verifies that newCur acquires an exclusive
// range lock at the eoCommit position on the current file.
func TestRangeLock_NewCur_LocksAtEoc(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	_, err := s.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	// A reader from a separate fd should NOT be able to acquire a shared
	// lock at position 0 (where eoc is for a new file).
	if tryReaderLock(s, 0, 1) {
		t.Error("shared lock at eoc=0 should be blocked by exclusive lock")
	}

	// A reader should be able to lock beyond eoc (position 1+).
	if !tryReaderLock(s, 1, 1) {
		t.Error("shared lock beyond eoc should succeed")
	}
}

// TestRangeLock_Eoc_AdvancesLock verifies that eoc acquires a new lock at
// the new eoCommit position and releases the old range.
func TestRangeLock_Eoc_AdvancesLock(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))
	s.maxSize = 16 * 1024 * 1024

	_, err := s.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	// Write a BEGIN + COMMIT, call eoc
	beginJSON := `{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}`
	if err := s.writeData([]byte(beginJSON)); err != nil {
		t.Fatalf("writeData B: %v", err)
	}
	commitJSON := `{"action":"C","nextlsn":"0/200"}`
	if err := s.writeData([]byte(commitJSON)); err != nil {
		t.Fatalf("writeData C: %v", err)
	}
	commitLSN := mylsn.LSN(0x200)
	if err := s.eoc(commitLSN, true); err != nil {
		t.Fatalf("eoc: %v", err)
	}

	oldEoc := int64(0) // original eoc
	newEoc := s.eoCommit

	if newEoc == oldEoc {
		t.Fatal("eoc should have advanced")
	}

	// Open a reader fd
	// The old position (0) should now be unlocked (released by UnlockRange)
	if !tryReaderLock(s, oldEoc, 1) {
		t.Error("shared lock at old eoc should succeed after eoc advances")
	}

	// The new position should be locked exclusively
	if tryReaderLock(s, newEoc, 1) {
		t.Error("shared lock at new eoc should be blocked by exclusive lock")
	}

	// Beyond new eoc should be unlocked
	if !tryReaderLock(s, newEoc+1, 1) {
		t.Error("shared lock beyond new eoc should succeed")
	}
}

// TestRangeLock_Eoc_MultipleCommits verifies that after multiple eoc calls,
// only the latest eoCommit position is locked.
func TestRangeLock_Eoc_MultipleCommits(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))
	s.maxSize = 16 * 1024 * 1024

	_, err := s.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	// First commit
	if err := s.writeData([]byte(`{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}`)); err != nil {
		t.Fatalf("writeData B1: %v", err)
	}
	if err := s.writeData([]byte(`{"action":"C","nextlsn":"0/100"}`)); err != nil {
		t.Fatalf("writeData C1: %v", err)
	}
	if err := s.eoc(mylsn.LSN(0x100), true); err != nil {
		t.Fatalf("eoc 1: %v", err)
	}
	eoc1 := s.eoCommit

	// Second commit
	if err := s.writeData([]byte(`{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}`)); err != nil {
		t.Fatalf("writeData B2: %v", err)
	}
	if err := s.writeData([]byte(`{"action":"C","nextlsn":"0/200"}`)); err != nil {
		t.Fatalf("writeData C2: %v", err)
	}
	if err := s.eoc(mylsn.LSN(0x200), true); err != nil {
		t.Fatalf("eoc 2: %v", err)
	}
	eoc2 := s.eoCommit

	// Third commit
	if err := s.writeData([]byte(`{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}`)); err != nil {
		t.Fatalf("writeData B3: %v", err)
	}
	if err := s.writeData([]byte(`{"action":"C","nextlsn":"0/300"}`)); err != nil {
		t.Fatalf("writeData C3: %v", err)
	}
	if err := s.eoc(mylsn.LSN(0x300), true); err != nil {
		t.Fatalf("eoc 3: %v", err)
	}
	eoc3 := s.eoCommit

	// All previous eoc positions should be unlocked
	for _, pos := range []int64{0, eoc1, eoc2} {
		if !tryReaderLock(s, pos, 1) {
			t.Errorf("shared lock at pos %d should succeed", pos)
		}
	}

	// Only the latest eoc should be locked
	if tryReaderLock(s, eoc3, 1) {
		t.Error("shared lock at latest eoc should be blocked")
	}
}

// TestRangeLock_Rotate_ReleasesLock verifies that after rotation, the old
// file's locks are released (fd closed) and the new file is locked at 0.
func TestRangeLock_Rotate_ReleasesLock(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))
	s.maxSize = 1 // trigger rotation

	_, err := s.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	if err := s.writeData([]byte(`{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}`)); err != nil {
		t.Fatalf("writeData B: %v", err)
	}
	if err := s.writeData([]byte(`{"action":"C","nextlsn":"0/200"}`)); err != nil {
		t.Fatalf("writeData C: %v", err)
	}
	commitLSN := mylsn.LSN(0x200)
	if err := s.eoc(commitLSN, true); err != nil {
		t.Fatalf("eoc: %v", err)
	}

	// After rotation, a new current file should exist and be locked at 0
	// Position 0 should be locked (new file, newCur locks at eoc=0)
	if tryReaderLock(s, 0, 1) {
		t.Error("shared lock at position 0 should be blocked after rotation")
	}

	// Position 1 should be unlocked
	if !tryReaderLock(s, 1, 1) {
		t.Error("shared lock at position 1 should succeed after rotation")
	}
}

// TestRangeLock_TruncateKeepsLock verifies that truncateToEoc doesn't
// change the lock position.
func TestRangeLock_TruncateKeepsLock(t *testing.T) {
	s := newFileTestCfg(t)
	s.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))
	s.maxSize = 16 * 1024 * 1024

	_, err := s.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	// Commit one transaction
	if err := s.writeData([]byte(`{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}`)); err != nil {
		t.Fatalf("writeData B: %v", err)
	}
	if err := s.writeData([]byte(`{"action":"C","nextlsn":"0/100"}`)); err != nil {
		t.Fatalf("writeData C: %v", err)
	}
	if err := s.eoc(mylsn.LSN(0x100), true); err != nil {
		t.Fatalf("eoc: %v", err)
	}
	eocPos := s.eoCommit

	// Write uncommitted data beyond eoCommit
	if _, err := s.curr.Write(
		[]byte(`{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}`),
	); err != nil {
		t.Fatalf("Write uncommitted: %v", err)
	}
	s.writer.Write([]byte("buffered"))

	// Truncate
	if err := s.truncateToEoc(); err != nil {
		t.Fatalf("truncateToEoc: %v", err)
	}

	// The lock should still be at eocPos
	if tryReaderLock(s, eocPos, 1) {
		t.Error("shared lock at eoc should still be blocked after truncate")
	}
	// Beyond eoc should be unlocked (truncated away)
	if !tryReaderLock(s, eocPos+1, 1) {
		t.Error("shared lock beyond eoc should succeed after truncate")
	}
}

// Local Variables:
// tab-width: 4
// End:
