package capture

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"

	"github.com/tfoertsch123/log"
	cap "github.com/tfoertsch123/pglogreplsimple"
	"github.com/tfoertsch123/own-your-pg/defaults"
	mylsn "github.com/tfoertsch123/own-your-pg/lsn"
	"github.com/tfoertsch123/own-your-pg/slot"
)

// ---------------------------------------------------------------- env gating

const testConnInfoEnv = "OYPG_TEST_CONNINFO"

func requireDB(t *testing.T) string {
	t.Helper()
	connInfo := os.Getenv(testConnInfoEnv)
	if connInfo == "" {
		t.Skipf("set %s to run integration tests", testConnInfoEnv)
	}
	return connInfo
}

// ---------------------------------------------------------------- helpers

const testSlotPrefix = "oypg_"
const testPlugin = "test_decoding"

func slotName(t *testing.T) string {
	t.Helper()
	return testSlotPrefix +
		strings.ToLower(strings.ReplaceAll(t.Name(), "/", "_"))
}

func createSlot(t *testing.T, connInfo, slot string) func() {
	t.Helper()
	tryDropSlot(t, connInfo, slot)
	execSQL(t, connInfo,
		"SELECT pg_create_logical_replication_slot('"+slot+"', '"+testPlugin+"')")
	return func() {
		tryDropSlot(t, connInfo, slot)
	}
}

func tryDropSlot(t *testing.T, connInfo, slot string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := pgconn.Connect(ctx, connInfo)
	if err != nil {
		return
	}
	defer conn.Close(context.Background())

	_, err = conn.Exec(ctx,
		"SELECT 1 FROM pg_replication_slots WHERE slot_name = '"+slot+"'",
	).ReadAll()
	if err != nil {
		return
	}
	_, _ = conn.Exec(ctx,
		"SELECT pg_drop_replication_slot('"+slot+"')").ReadAll()
}

func execSQL(t *testing.T, connInfo, sql string) []*pgconn.Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := pgconn.Connect(ctx, connInfo)
	if err != nil {
		t.Fatalf("connect for exec: %v", err)
	}
	defer conn.Close(context.Background())

	t.Logf("execSQL: %v", sql)
	res, err := conn.Exec(ctx, sql).ReadAll()
	if err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
	return res
}

func setupTable(t *testing.T, connInfo string) {
	t.Helper()
	execSQL(t, connInfo,
		`CREATE TABLE IF NOT EXISTS oypg_test(id int primary key, val text);
		 DELETE FROM oypg_test;`)
}

// newTestCfg creates a Cfg with a slot manager in a temp dir, configures
// primary_conninfo, and initializes the capture file structure.
func newTestCfg(t *testing.T, connInfo, sn string) *Cfg {
	t.Helper()
	dir := t.TempDir()

	mgr, err := slot.NewMgr(filepath.Join(dir, defaults.SlotDir),
		slot.WithMgrCreate())
	if err != nil {
		t.Fatalf("NewMgr: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })

	sl, err := mgr.Slot(sn, slot.WithAsOwner(), slot.WithType(slot.Producer))
	if err != nil {
		t.Fatalf("Slot: %v", err)
	}
	t.Cleanup(func() { sl.Close() })

	// Trigger loadCfg to initialize the Config map
	_, _ = sl.GetConfig("", true)
	sl.SetConfig("primary_conninfo", []string{connInfo})
	sl.SetConfig("primary_slotname", []string{sn})
	// Use stderr for logging so applySettings sets up the logger
	sl.SetConfig("logfile", []string{"//stderr"})
	if err := sl.SaveConfig(true); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	cfg := &Cfg{
		wd:        dir,
		mgr:       mgr,
		sl:        sl,
		currDirFd: -1,
	}
	cfg.ensureIncDir()
	t.Cleanup(func() {
		if cfg.currDirFd >= 0 {
			cfg.currDirFd = -1
		}
	})

	// Set up loggers (applySettings needs m.lg to be non-nil)
	cfg.lg = log.NewR(log.WithTopic("MAIN"))
	cfg.mlg = cfg.lg.New(log.WithTopic("WRT"))
	// Initialize recvP so applySettings can compare against it
	cfg.recvP = &cap.Param{}

	nCfg, err := cfg.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	nCfg.recvP.ErrorRetryInterval = 500 * time.Millisecond
	nCfg.recvP.FeedbackInterval = 1 * time.Second

	if err := cfg.applySettings(nCfg, nCfg.recvP); err != nil {
		t.Fatalf("applySettings: %v", err)
	}

	// Initialize the capture file
	cfg.curInit()

	return cfg
}

func newReceiver(cfg *Cfg) *cap.Receiver {
	return cap.NewReceiver(
		cap.WithParams(cfg.recvP),
		cap.WithAcceptedPlugins(map[string][]string{
			testPlugin: {},
		}),
	)
}

// readCurrentFile reads all data from the current file up to eoCommit.
func readCurrentFile(t *testing.T, cfg *Cfg) []byte {
	t.Helper()
	n := cfg.eoCommit
	if n == 0 {
		return nil
	}
	buf := make([]byte, n)
	if _, err := cfg.curr.ReadAt(buf, 0); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	return buf
}

// ---------------------------------------------------------------- tests

func TestIntegrationCaptureSingleTransaction(t *testing.T) {
	connInfo := requireDB(t)
	sn := slotName(t)
	cleanup := createSlot(t, connInfo, sn)
	defer cleanup()
	setupTable(t, connInfo)

	cfg := newTestCfg(t, connInfo, sn)
	r := newReceiver(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	it, err := r.Produce(ctx)
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}

	// Generate WAL after Produce starts (in a goroutine), then
	// cancel after giving the receiver time to process.
	go func() {
		execSQL(t, connInfo,
			`INSERT INTO oypg_test VALUES (1, 'hello')`)
		time.Sleep(2 * time.Second)
		r.Shutdown(nil)
	}()

	xldCount := 0
	for msg := range it {
		switch dat := msg.(type) {
		case *pglogrepl.XLogData:
			xldCount++
			t.Logf("XLD: %s", string(dat.WALData))
			r.AckLSN(dat.WALStart)
		case *pglogrepl.PrimaryKeepaliveMessage:
			r.AckLSN(dat.ServerWALEnd)
		case *pgproto3.NoticeResponse:
			t.Logf("notice: %v", dat.Message)
		}
	}

	if xldCount == 0 {
		t.Error("expected at least one XLogData message")
	}
	t.Logf("received %d XLogData messages", xldCount)
}

func TestIntegrationCaptureTruncateOnReconnect(t *testing.T) {
	connInfo := requireDB(t)
	sn := slotName(t)
	cleanup := createSlot(t, connInfo, sn)
	defer cleanup()
	setupTable(t, connInfo)

	cfg := newTestCfg(t, connInfo, sn)
	r := cap.NewReceiver(
		cap.WithParams(cfg.recvP),
		cap.WithAcceptedPlugins(map[string][]string{
			testPlugin: {},
		}),
		cap.WithOnConnect(func(_ *cap.Receiver) error {
			return cfg.truncateToEoc()
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	it, err := r.Produce(ctx)
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}

	go func() {
		execSQL(t, connInfo,
			`INSERT INTO oypg_test VALUES (1, 'first')`)
		time.Sleep(2 * time.Second)
		r.Shutdown(nil)
	}()

	committed := false
	for msg := range it {
		switch dat := msg.(type) {
		case *pglogrepl.XLogData:
			t.Logf("XLD: %s", string(dat.WALData))
			r.AckLSN(dat.WALStart)
			committed = true
		case *pglogrepl.PrimaryKeepaliveMessage:
			r.AckLSN(dat.ServerWALEnd)
		}
	}

	if !committed {
		t.Fatal("did not capture any XLogData messages")
	}

	// Verify truncateToEoc doesn't corrupt the file
	eofBefore, _ := cfg.curr.Seek(0, 2)
	if err := cfg.truncateToEoc(); err != nil {
		t.Fatalf("truncateToEoc: %v", err)
	}
	eofAfter, _ := cfg.curr.Seek(0, 2)
	if eofAfter > eofBefore {
		t.Errorf("file grew after truncate: %d -> %d", eofBefore, eofAfter)
	}
}

func TestIntegrationCaptureRotation(t *testing.T) {
	connInfo := requireDB(t)
	sn := slotName(t)
	cleanup := createSlot(t, connInfo, sn)
	defer cleanup()
	setupTable(t, connInfo)

	cfg := newTestCfg(t, connInfo, sn)
	cfg.maxSize = 1 // trigger rotation on first commit

	r := newReceiver(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	it, err := r.Produce(ctx)
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}

	go func() {
		for i := 1; i <= 5; i++ {
			execSQL(t, connInfo, fmt.Sprintf(
				`INSERT INTO oypg_test VALUES (%d, 'row%d')`, i, i))
			time.Sleep(100 * time.Millisecond)
		}
		time.Sleep(2 * time.Second)
		r.Shutdown(nil)
	}()

	xldCount := 0
	for msg := range it {
		switch dat := msg.(type) {
		case *pglogrepl.XLogData:
			xldCount++
			t.Logf("XLD #%d: %s", xldCount, string(dat.WALData))
			// Write data through the capture pipeline to trigger rotation
			if err := cfg.writeData(dat.WALData); err != nil {
				t.Fatalf("writeData: %v", err)
			}
			// Call eoc on every COMMIT-like message.
			// test_decoding sends "COMMIT <xid>" as a separate XLD.
			if strings.HasPrefix(string(dat.WALData), "COMMIT") {
				if err := cfg.eoc(
					mylsn.LSN(dat.ServerWALEnd), false,
				); err != nil {
					t.Fatalf("eoc: %v", err)
				}
			}
			r.AckLSN(dat.WALStart)
		case *pglogrepl.PrimaryKeepaliveMessage:
			r.AckLSN(dat.ServerWALEnd)
		}
	}

	if xldCount == 0 {
		t.Fatal("did not capture any XLogData messages")
	}
	t.Logf("captured %d XLogData messages", xldCount)

	// After rotation, check for history files in the parent directory
	entries, err := os.ReadDir(filepath.Join(cfg.wd))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	histCount := 0
	for _, e := range entries {
		name := e.Name()
		if name != defaults.IncDir && name != defaults.SlotDir &&
			!strings.HasPrefix(name, ".") {
			histCount++
			t.Logf("history file: %s", name)
		}
	}
	if histCount == 0 {
		t.Error("expected at least one history file after rotation")
	}

	// The current file should still exist
	curPath := filepath.Join(cfg.wd, defaults.IncDir, defaults.CurFile)
	if _, err := os.Stat(curPath); err != nil {
		t.Errorf("current file should exist: %v", err)
	}
}

// Local Variables:
// tab-width: 4
// End:
