package capture

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/tfoertsch123/log"
	"github.com/tfoertsch123/own-your-pg/defaults"
	"github.com/tfoertsch123/own-your-pg/slot"
	mylsn "github.com/tfoertsch123/own-your-pg/lsn"
)

// newFileTestCfg creates a Cfg with a slot, mgr, and the incoming directory
// structure set up in a temp dir, ready for file I/O tests.
func newFileTestCfg(t *testing.T) *Cfg {
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

	cfg := &Cfg{
		wd:        dir,
		mgr:       mgr,
		sl:        sl,
		currDirFd: -1,
	}
	cfg.ensureIncDir()
	t.Cleanup(func() {
		if cfg.currDirFd >= 0 {
			unix.Close(cfg.currDirFd)
		}
		if cfg.histDirFd >= 0 {
			unix.Close(cfg.histDirFd)
		}
		if cfg.meta != nil {
			cfg.meta.Close()
		}
	})

	// Set up loggers so functions that use mlg/lg don't panic
	cfg.lg = log.NewR(log.WithTopic("MAIN"))
	cfg.mlg = cfg.lg.New(log.WithTopic("WRT"))

	return cfg
}

// ---- writeMeta / readMeta round-trip ----

func TestWriteReadMeta_RoundTrip(t *testing.T) {
	cfg := newFileTestCfg(t)

	flsn := mylsn.LSN(0x1234567890ABCDEF)
	llsn := mylsn.LSN(0xFEDCBA0987654321)
	epos := int64(4096)

	if err := cfg.writeMeta(epos, &flsn, llsn); err != nil {
		t.Fatalf("writeMeta: %v", err)
	}

	gotEpos, gotFlsn, gotLlsn, err := cfg.readMeta()
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
	cfg := newFileTestCfg(t)

	llsn := mylsn.LSN(0xAAAA)
	epos := int64(100)

	if err := cfg.writeMeta(epos, nil, llsn); err != nil {
		t.Fatalf("writeMeta: %v", err)
	}

	gotEpos, gotFlsn, gotLlsn, err := cfg.readMeta()
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
	cfg := newFileTestCfg(t)

	_, _, _, err := cfg.readMeta()
	if err != ErrMetaGarbage {
		t.Errorf("readMeta on empty file: got %v, want %v", err, ErrMetaGarbage)
	}
}

func TestReadMeta_ShortFile(t *testing.T) {
	cfg := newFileTestCfg(t)

	var bts [8]byte
	binary.BigEndian.PutUint64(bts[:], 42)
	if _, err := cfg.meta.WriteAt(bts[:], 0); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}

	_, _, _, err := cfg.readMeta()
	if err != ErrMetaGarbage {
		t.Errorf("readMeta on short file: got %v, want %v", err, ErrMetaGarbage)
	}
}

// ---- newCur ----

func TestNewCur_EmptyFile(t *testing.T) {
	cfg := newFileTestCfg(t)
	cfg.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	llsn, err := cfg.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}
	if llsn != 0 {
		t.Errorf("llsn: got %v, want 0", llsn)
	}
	if cfg.curr == nil {
		t.Error("cfg.curr should be set")
	}
	if cfg.currLck == nil {
		t.Error("cfg.currLck should be set")
	}
	if cfg.eoCommit != 0 {
		t.Errorf("eoCommit: got %d, want 0", cfg.eoCommit)
	}
	if cfg.firstTxnLSN != nil {
		t.Errorf("firstTxnLSN: got %v, want nil", cfg.firstTxnLSN)
	}

	eoc, flsn, llsn2, err := cfg.readMeta()
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
	cfg := newFileTestCfg(t)
	cfg.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	llsn1, err := cfg.newCur()
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
	if _, err := cfg.curr.Write(testData); err != nil {
		t.Fatalf("Write: %v", err)
	}
	eof := int64(len(testData))
	flsn := mylsn.LSN(0x100)
	llsn := mylsn.LSN(0x100)
	if err := cfg.writeMeta(eof, &flsn, llsn); err != nil {
		t.Fatalf("writeMeta: %v", err)
	}
	cfg.curr.Close()
	cfg.curr = nil
	if cfg.currLck != nil {
		cfg.currLck.Close()
		cfg.currLck = nil
	}

	llsn2, err := cfg.newCur()
	if err != nil {
		t.Fatalf("second newCur: %v", err)
	}
	if llsn2 != llsn {
		t.Errorf("llsn: got %v, want %v", llsn2, llsn)
	}
	if cfg.eoCommit != eof {
		t.Errorf("eoCommit: got %d, want %d", cfg.eoCommit, eof)
	}
	if cfg.firstTxnLSN == nil || *cfg.firstTxnLSN != flsn {
		t.Errorf("firstTxnLSN: got %v, want %v", cfg.firstTxnLSN, flsn)
	}
}

func TestNewCur_Garbage(t *testing.T) {
	cfg := newFileTestCfg(t)
	cfg.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	_, err := cfg.newCur()
	if err != nil {
		t.Fatalf("first newCur: %v", err)
	}

	// Write some data to the current file so eof > 0
	if _, err := cfg.curr.Write([]byte("some data")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Write metadata with eoc pointing far beyond EOF
	if err := cfg.writeMeta(99999, nil, mylsn.LSN(0)); err != nil {
		t.Fatalf("writeMeta: %v", err)
	}
	cfg.curr.Close()
	cfg.curr = nil
	if cfg.currLck != nil {
		cfg.currLck.Close()
		cfg.currLck = nil
	}

	_, err = cfg.newCur()
	if err != ErrCurrGarbage {
		t.Errorf("newCur with garbage: got %v, want %v", err, ErrCurrGarbage)
	}
}

// ---- writeData / flush ----

func TestWriteData(t *testing.T) {
	cfg := newFileTestCfg(t)
	cfg.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	_, err := cfg.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	data := []byte(`{"action":"I","xid":1}`)
	if err := cfg.writeData(data); err != nil {
		t.Fatalf("writeData: %v", err)
	}
	if cfg.writer.Len() != len(data)+1 {
		t.Errorf("writer.Len(): got %d, want %d",
			cfg.writer.Len(), len(data)+1)
	}

	n, err := cfg.flush()
	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	if n != int64(len(data)+1) {
		t.Errorf("flush n: got %d, want %d", n, int64(len(data)+1))
	}
	if cfg.writer.Len() != 0 {
		t.Errorf("writer.Len() after flush: got %d, want 0",
			cfg.writer.Len())
	}
}

func TestWriteData_AutoFlush(t *testing.T) {
	cfg := newFileTestCfg(t)
	cfg.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	_, err := cfg.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	// Write >64KB to trigger auto-flush
	chunk := make([]byte, 32768)
	for i := range chunk {
		chunk[i] = 'x'
	}
	if err := cfg.writeData(chunk); err != nil {
		t.Fatalf("writeData 1: %v", err)
	}
	if err := cfg.writeData(chunk); err != nil {
		t.Fatalf("writeData 2: %v", err)
	}

	if cfg.writer.Len() == 0 {
		eof, err := cfg.curr.Seek(0, 2)
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
	cfg := newFileTestCfg(t)
	cfg.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	_, err := cfg.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	committed := []byte(`{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}
{"action":"C","nextlsn":"0/100"}
`)
	if _, err := cfg.curr.Write(committed); err != nil {
		t.Fatalf("Write committed: %v", err)
	}
	cfg.eoCommit = int64(len(committed))

	uncommitted := []byte(`{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}
{"action":"I","xid":2}
`)
	if _, err := cfg.curr.Write(uncommitted); err != nil {
		t.Fatalf("Write uncommitted: %v", err)
	}

	cfg.writer.Write([]byte("buffered"))

	if err := cfg.truncateToEoc(); err != nil {
		t.Fatalf("truncateToEoc: %v", err)
	}

	eof, err := cfg.curr.Seek(0, 2)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if eof != cfg.eoCommit {
		t.Errorf("eof after truncate: got %d, want %d",
			eof, cfg.eoCommit)
	}

	if cfg.writer.Len() != 0 {
		t.Errorf("writer.Len(): got %d, want 0", cfg.writer.Len())
	}
}

func TestTruncateToEoc_NoTruncationNeeded(t *testing.T) {
	cfg := newFileTestCfg(t)
	cfg.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	_, err := cfg.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	cfg.eoCommit = 0

	if err := cfg.truncateToEoc(); err != nil {
		t.Fatalf("truncateToEoc: %v", err)
	}

	eof, err := cfg.curr.Seek(0, 2)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if eof != 0 {
		t.Errorf("eof: got %d, want 0", eof)
	}
}

// ---- eoc ----

func TestEoc_Commit(t *testing.T) {
	cfg := newFileTestCfg(t)
	cfg.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))
	cfg.maxSize = 16 * 1024 * 1024

	_, err := cfg.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	beginJSON := `{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}`
	if err := cfg.writeData([]byte(beginJSON)); err != nil {
		t.Fatalf("writeData B: %v", err)
	}

	commitLSN := mylsn.LSN(0x200)

	commitJSON := `{"action":"C","nextlsn":"0/200"}`
	if err := cfg.writeData([]byte(commitJSON)); err != nil {
		t.Fatalf("writeData C: %v", err)
	}

	if err := cfg.eoc(commitLSN, true); err != nil {
		t.Fatalf("eoc: %v", err)
	}

	if cfg.eoCommit == 0 {
		t.Error("eoCommit should have advanced from 0")
	}

	if cfg.firstTxnLSN == nil {
		t.Error("firstTxnLSN should be set")
	}

	eoc, flsn, llsn, err := cfg.readMeta()
	if err != nil {
		t.Fatalf("readMeta: %v", err)
	}
	if eoc != cfg.eoCommit {
		t.Errorf("meta eoc: got %d, want %d", eoc, cfg.eoCommit)
	}
	if flsn == nil || *flsn != commitLSN {
		t.Errorf("meta flsn: got %v, want %v", flsn, commitLSN)
	}
	if llsn != commitLSN {
		t.Errorf("meta llsn: got %v, want %v", llsn, commitLSN)
	}

	slotLSN, _ := cfg.sl.GetLSN()
	if slotLSN != commitLSN {
		t.Errorf("slot LSN: got %v, want %v", slotLSN, commitLSN)
	}

	// The B record's placeholder should have been replaced
	eof, err := cfg.curr.Seek(0, 2)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	}
	buf := make([]byte, eof)
	if _, err := cfg.curr.ReadAt(buf, 0); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if bytes.Contains(buf, []byte("XXXXXXXX/YYYYYYYY")) {
		t.Error("placeholder should have been replaced")
	}
}

func TestEoc_Rotation(t *testing.T) {
	cfg := newFileTestCfg(t)
	cfg.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	cfg.maxSize = 1

	_, err := cfg.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	beginJSON := `{"action":"B","nextlsn":"XXXXXXXX/YYYYYYYY"}`
	if err := cfg.writeData([]byte(beginJSON)); err != nil {
		t.Fatalf("writeData B: %v", err)
	}
	commitJSON := `{"action":"C","nextlsn":"0/200"}`
	if err := cfg.writeData([]byte(commitJSON)); err != nil {
		t.Fatalf("writeData C: %v", err)
	}

	commitLSN := mylsn.LSN(0x200)
	if err := cfg.eoc(commitLSN, true); err != nil {
		t.Fatalf("eoc: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(cfg.wd))
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

	if cfg.curr == nil {
		t.Error("cfg.curr should be set after rotation")
	}
	if cfg.firstTxnLSN != nil {
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

	cfg := &Cfg{
		wd:        dir,
		mgr:       mgr,
		sl:        sl,
		currDirFd: -1,
	}
	cfg.ensureIncDir()
	t.Cleanup(func() {
		if cfg.currDirFd >= 0 {
			unix.Close(cfg.currDirFd)
		}
		if cfg.histDirFd >= 0 {
			unix.Close(cfg.histDirFd)
		}
		if cfg.meta != nil {
			cfg.meta.Close()
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

	if cfg.currDirFd < 0 {
		t.Error("currDirFd should be set")
	}
	if cfg.histDirFd < 0 {
		t.Error("histDirFd should be set")
	}
}

// ---- curInit ----

func TestCurInit(t *testing.T) {
	cfg := newFileTestCfg(t)

	llsn := cfg.curInit()
	if llsn != 0 {
		t.Errorf("curInit llsn: got %v, want 0", llsn)
	}
	if cfg.curr == nil {
		t.Error("curr should be set")
	}
	if cfg.writer == nil {
		t.Error("writer should be set")
	}
	if cfg.writer.Cap() != 65*1024 {
		t.Errorf("writer cap: got %d, want %d",
			cfg.writer.Cap(), 65*1024)
	}
}

// ---- flock integration ----

func TestNewCur_FlockStored(t *testing.T) {
	cfg := newFileTestCfg(t)
	cfg.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	_, err := cfg.newCur()
	if err != nil {
		t.Fatalf("newCur: %v", err)
	}

	if cfg.currLck == nil {
		t.Fatal("currLck should be set after newCur")
	}
	if !cfg.currLck.IsOpen() {
		t.Error("currLck should be open after newCur")
	}
	if cfg.curr == nil {
		t.Error("curr should be set after newCur")
	}
}

// Local Variables:
// tab-width: 4
// End:
