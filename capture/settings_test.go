package capture

import (
	"strings"
	"testing"

	cap "github.com/tfoertsch123/pglogreplsimple"
	"github.com/tfoertsch123/own-your-pg/slot"
)

// newTestSlot creates a slot.Mgr and a producer slot in a temp dir,
// applies the given config values, and saves them.
func newTestSlot(t *testing.T, name string, cfgs map[string][]string) (*slot.Mgr, *slot.Slot) {
	t.Helper()
	dir := t.TempDir()
	mgr, err := slot.NewMgr(dir, slot.WithMgrCreate())
	if err != nil {
		t.Fatalf("NewMgr: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })

	sl, err := mgr.Slot(name, slot.WithAsOwner(), slot.WithType(slot.Producer))
	if err != nil {
		t.Fatalf("Slot: %v", err)
	}
	t.Cleanup(func() { sl.Close() })

	// Trigger loadCfg to initialize the Config map
	_, _ = sl.GetConfig("", true)

	for k, v := range cfgs {
		sl.SetConfig(k, v)
	}
	if err := sl.SaveConfig(true); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	return mgr, sl
}

func TestReadSettings_MissingConninfo(t *testing.T) {
	_, sl := newTestSlot(t, "testslot", nil)
	cfg := &Cfg{sl: sl}
	_, err := cfg.readSettings(false)
	if err != ErrMissingConninfo {
		t.Errorf("got %v, want %v", err, ErrMissingConninfo)
	}
}

func TestReadSettings_InvalidConninfo(t *testing.T) {
	_, sl := newTestSlot(t, "testslot", map[string][]string{
		"primary_conninfo": {"!!!invalid!!!"},
	})
	cfg := &Cfg{sl: sl}
	_, err := cfg.readSettings(false)
	if err != ErrInvalidConninfo {
		t.Errorf("got %v, want %v", err, ErrInvalidConninfo)
	}
}

func TestReadSettings_ValidConninfo(t *testing.T) {
	_, sl := newTestSlot(t, "testslot", map[string][]string{
		"primary_conninfo": {"host=localhost port=5432"},
	})
	cfg := &Cfg{sl: sl}
	nCfg, err := cfg.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if nCfg.recvP.ConnInfo == "" {
		t.Error("ConnInfo should be set")
	}
	if !strings.Contains(nCfg.recvP.ConnInfo, "OYPG-testslot") {
		t.Errorf("ConnInfo should contain OYPG-testslot, got: %s",
			nCfg.recvP.ConnInfo)
	}
}

func TestReadSettings_ApplicationNamePreserved(t *testing.T) {
	_, sl := newTestSlot(t, "testslot", map[string][]string{
		"primary_conninfo": {"host=localhost application_name=myapp"},
	})
	cfg := &Cfg{sl: sl}
	nCfg, err := cfg.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if !strings.Contains(nCfg.recvP.ConnInfo, "application_name=myapp") {
		t.Errorf("ConnInfo should preserve application_name=myapp, got: %s",
			nCfg.recvP.ConnInfo)
	}
	if strings.Contains(nCfg.recvP.ConnInfo, "OYPG-") {
		t.Errorf("ConnInfo should not contain OYPG- prefix, got: %s",
			nCfg.recvP.ConnInfo)
	}
}

func TestReadSettings_DefaultSlotName(t *testing.T) {
	_, sl := newTestSlot(t, "myslot", map[string][]string{
		"primary_conninfo": {"host=localhost"},
	})
	cfg := &Cfg{sl: sl}
	nCfg, err := cfg.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if nCfg.recvP.SlotName != "myslot" {
		t.Errorf("SlotName: got %q, want %q", nCfg.recvP.SlotName, "myslot")
	}
}

func TestReadSettings_ExplicitSlotName(t *testing.T) {
	_, sl := newTestSlot(t, "myslot", map[string][]string{
		"primary_conninfo": {"host=localhost"},
		"primary_slotname": {"explicit_slot"},
	})
	cfg := &Cfg{sl: sl}
	nCfg, err := cfg.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if nCfg.recvP.SlotName != "explicit_slot" {
		t.Errorf("SlotName: got %q, want %q",
			nCfg.recvP.SlotName, "explicit_slot")
	}
}

func TestReadSettings_DefaultSizeLimit(t *testing.T) {
	_, sl := newTestSlot(t, "testslot", map[string][]string{
		"primary_conninfo": {"host=localhost"},
	})
	cfg := &Cfg{sl: sl}
	nCfg, err := cfg.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if nCfg.maxSize != 16*1024*1024 {
		t.Errorf("maxSize: got %d, want %d",
			nCfg.maxSize, 16*1024*1024)
	}
}

func TestReadSettings_SizeLimitNumeric(t *testing.T) {
	_, sl := newTestSlot(t, "testslot", map[string][]string{
		"primary_conninfo": {"host=localhost"},
		"size_limit":       {"1024"},
	})
	cfg := &Cfg{sl: sl}
	nCfg, err := cfg.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if nCfg.maxSize != 1024 {
		t.Errorf("maxSize: got %d, want %d", nCfg.maxSize, 1024)
	}
}

func TestReadSettings_SizeLimitHuman(t *testing.T) {
	_, sl := newTestSlot(t, "testslot", map[string][]string{
		"primary_conninfo": {"host=localhost"},
		"size_limit":       {"4MiB"},
	})
	cfg := &Cfg{sl: sl}
	nCfg, err := cfg.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if nCfg.maxSize != 4*1024*1024 {
		t.Errorf("maxSize: got %d, want %d",
			nCfg.maxSize, 4*1024*1024)
	}
}

func TestReadSettings_InvalidSizeLimit(t *testing.T) {
	_, sl := newTestSlot(t, "testslot", map[string][]string{
		"primary_conninfo": {"host=localhost"},
		"size_limit":       {"garbage"},
	})
	cfg := &Cfg{sl: sl}
	_, err := cfg.readSettings(false)
	if err != ErrInvalidLimit {
		t.Errorf("got %v, want %v", err, ErrInvalidLimit)
	}
}

func TestReadSettings_SynchronousOn(t *testing.T) {
	for _, val := range []string{"on", "true", "1", "ON", "True"} {
		t.Run(val, func(t *testing.T) {
			_, sl := newTestSlot(t, "testslot", map[string][]string{
				"primary_conninfo": {"host=localhost"},
				"synchronous":      {val},
			})
			cfg := &Cfg{sl: sl}
			nCfg, err := cfg.readSettings(false)
			if err != nil {
				t.Fatalf("readSettings: %v", err)
			}
			if !nCfg.recvP.FeedbackOnFlush {
				t.Errorf("FeedbackOnFlush: got false, want true (val=%q)", val)
			}
		})
	}
}

func TestReadSettings_SynchronousOff(t *testing.T) {
	for _, val := range []string{"off", "false", "0", "OFF"} {
		t.Run(val, func(t *testing.T) {
			_, sl := newTestSlot(t, "testslot", map[string][]string{
				"primary_conninfo": {"host=localhost"},
				"synchronous":      {val},
			})
			cfg := &Cfg{sl: sl}
			nCfg, err := cfg.readSettings(false)
			if err != nil {
				t.Fatalf("readSettings: %v", err)
			}
			if nCfg.recvP.FeedbackOnFlush {
				t.Errorf("FeedbackOnFlush: got true, want false (val=%q)", val)
			}
		})
	}
}

func TestReadSettings_SynchronousInvalid(t *testing.T) {
	_, sl := newTestSlot(t, "testslot", map[string][]string{
		"primary_conninfo": {"host=localhost"},
		"synchronous":      {"maybe"},
	})
	cfg := &Cfg{sl: sl}
	_, err := cfg.readSettings(false)
	if err != ErrInvalidSynchronous {
		t.Errorf("got %v, want %v", err, ErrInvalidSynchronous)
	}
}

func TestReadSettings_SynchronousDefault(t *testing.T) {
	_, sl := newTestSlot(t, "testslot", map[string][]string{
		"primary_conninfo": {"host=localhost"},
	})
	cfg := &Cfg{sl: sl}
	nCfg, err := cfg.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if nCfg.recvP.FeedbackOnFlush {
		t.Error("FeedbackOnFlush: got true, want false (default)")
	}
}

func TestReadSettings_IntervalDefaults(t *testing.T) {
	_, sl := newTestSlot(t, "testslot", map[string][]string{
		"primary_conninfo": {"host=localhost"},
	})
	cfg := &Cfg{sl: sl}
	nCfg, err := cfg.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if nCfg.recvP.ErrorRetryInterval != cap.DefaultErrorRetryInterval {
		t.Errorf("ErrorRetryInterval: got %v, want %v",
			nCfg.recvP.ErrorRetryInterval, cap.DefaultErrorRetryInterval)
	}
	if nCfg.recvP.FeedbackInterval != cap.DefaultFeedbackInterval {
		t.Errorf("FeedbackInterval: got %v, want %v",
			nCfg.recvP.FeedbackInterval, cap.DefaultFeedbackInterval)
	}
}

func TestReadSettings_IgnoreMissingIdentity(t *testing.T) {
	_, sl := newTestSlot(t, "testslot", map[string][]string{
		"primary_conninfo":        {"host=localhost"},
		"ignore_missing_identity": {"table1", "table2"},
	})
	cfg := &Cfg{sl: sl}
	nCfg, err := cfg.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if len(nCfg.ignMissId) != 2 {
		t.Fatalf("ignMissId len: got %d, want 2", len(nCfg.ignMissId))
	}
	if nCfg.ignMissId[0] != "table1" || nCfg.ignMissId[1] != "table2" {
		t.Errorf("ignMissId: got %v, want [table1, table2]",
			nCfg.ignMissId)
	}
}

func TestReadSettings_Logfile(t *testing.T) {
	_, sl := newTestSlot(t, "testslot", map[string][]string{
		"primary_conninfo": {"host=localhost"},
		"logfile":          {"file:///tmp/test.log"},
	})
	cfg := &Cfg{sl: sl}
	nCfg, err := cfg.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if nCfg.logURL != "file:///tmp/test.log" {
		t.Errorf("logURL: got %q, want %q",
			nCfg.logURL, "file:///tmp/test.log")
	}
}

func TestPrepareReload_ErrorPropagated(t *testing.T) {
	cfg := &Cfg{}
	testErr := ErrMissingConninfo
	nCfg := &Reloadable{recvP: &cap.Param{}}
	p := cfg.prepareReload(nCfg, testErr)

	if p.OnActivation == nil {
		t.Fatal("OnActivation should be set")
	}
	err := p.OnActivation(p)
	if err != testErr {
		t.Errorf("OnActivation: got %v, want %v", err, testErr)
	}
}

func TestReloadableString(t *testing.T) {
	r := &Reloadable{
		logURL: "file:///tmp/test.log",
		recvP: &cap.Param{
			ConnInfo:        "host=localhost",
			SlotName:        "testslot",
			FeedbackOnFlush: true,
		},
		maxSize:   1024,
		ignMissId: []string{"t1"},
	}
	s := r.String()
	if !strings.Contains(s, "logfile: file:///tmp/test.log") {
		t.Errorf("String() should contain logfile, got: %s", s)
	}
	if !strings.Contains(s, "primary_conninfo: host=localhost") {
		t.Errorf("String() should contain conninfo, got: %s", s)
	}
	if !strings.Contains(s, "primary_slotname: testslot") {
		t.Errorf("String() should contain slotname, got: %s", s)
	}
	if !strings.Contains(s, "synchronous: true") {
		t.Errorf("String() should contain synchronous, got: %s", s)
	}
	if !strings.Contains(s, "max_size: 1024") {
		t.Errorf("String() should contain max_size, got: %s", s)
	}
}

// Local Variables:
// tab-width: 4
// End:
