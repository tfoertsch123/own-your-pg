package capture

import (
	"testing"
	"time"

	"github.com/tfoertsch123/log"
	cap "github.com/tfoertsch123/pglogreplsimple"
)

func newApplySession(t *testing.T, cfgs map[string][]string) *session {
	t.Helper()
	_, sl := newTestSlot(t, "applytest", cfgs)
	s := &session{
		sl:           sl,
		recvP:        &cap.Param{},
		ignMissIdMap: map[string]struct{}{},
	}
	s.lg = log.NewR(log.WithTopic("TEST"))
	s.mlg = s.lg.New(log.WithTopic("WRT"))
	return s
}

func TestApplySettings_FirstTime(t *testing.T) {
	s := newApplySession(t, map[string][]string{
		"primary_conninfo": {"host=localhost"},
	})

	nCfg, err := s.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}

	if err := s.applySettings(nCfg, nCfg.recvP); err != nil {
		t.Fatalf("applySettings: %v", err)
	}

	if s.lg == nil || s.recvP == nil {
		t.Error("logger and recvP should be initialized")
	}
	if s.recvP.ConnInfo != nCfg.recvP.ConnInfo {
		t.Errorf("ConnInfo not applied")
	}
	if s.recvP.SlotName != nCfg.recvP.SlotName {
		t.Errorf("SlotName not applied: got %q", s.recvP.SlotName)
	}
	if len(s.ignMissIdMap) != len(s.ignMissId) {
		t.Errorf("ignMissIdMap not populated: %d entries for %d items",
			len(s.ignMissIdMap), len(s.ignMissId))
	}
}

func TestApplySettings_NoChange(t *testing.T) {
	s := newApplySession(t, map[string][]string{
		"primary_conninfo": {"host=localhost"},
	})

	nCfg, err := s.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if err := s.applySettings(nCfg, nCfg.recvP); err != nil {
		t.Fatalf("first applySettings: %v", err)
	}

	oldLg := s.lg
	// Applying the same settings again must yield ErrNoChange.
	nCfg2, err := s.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings 2: %v", err)
	}
	if err := s.applySettings(nCfg2, nCfg2.recvP); err != cap.ErrNoChange {
		t.Errorf("expected ErrNoChange, got %v", err)
	}
	if s.lg != oldLg {
		t.Error("logger should not change on no-change reload")
	}
}

func TestApplySettings_IgnoreMissingIdentityMap(t *testing.T) {
	s := newApplySession(t, map[string][]string{
		"primary_conninfo":        {"host=localhost"},
		"ignore_missing_identity": {`"public"."orders"`, "*"},
	})

	nCfg, err := s.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if err := s.applySettings(nCfg, nCfg.recvP); err != nil {
		t.Fatalf("applySettings: %v", err)
	}

	if _, ok := s.ignMissIdMap["*"]; !ok {
		t.Error("* should be in ignMissIdMap")
	}
	if _, ok := s.ignMissIdMap[`"public"."orders"`]; !ok {
		t.Error("quoted table name should be in ignMissIdMap")
	}
}

func TestApplySettings_ChangeIntervals(t *testing.T) {
	s := newApplySession(t, map[string][]string{
		"primary_conninfo": {"host=localhost"},
	})

	nCfg, err := s.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if err := s.applySettings(nCfg, nCfg.recvP); err != nil {
		t.Fatalf("first applySettings: %v", err)
	}

	// Change only intervals -- should not be ErrNoChange.
	// Re-read settings to get a fresh recvP: the first applySettings
	// aliased s.recvP to nCfg.recvP, so mutating nCfg.recvP in place
	// would mutate s.recvP too and register as "no change".
	nCfg2, err := s.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings 2: %v", err)
	}
	nCfg2.recvP.ErrorRetryInterval = 2 * time.Second
	nCfg2.recvP.FeedbackInterval = 3 * time.Second
	if err := s.applySettings(nCfg2, nCfg2.recvP); err != nil {
		t.Errorf("interval change should apply, got %v", err)
	}
	if s.recvP.ErrorRetryInterval != 2*time.Second {
		t.Errorf("ErrorRetryInterval not applied: %v", s.recvP.ErrorRetryInterval)
	}
	if s.recvP.FeedbackInterval != 3*time.Second {
		t.Errorf("FeedbackInterval not applied: %v", s.recvP.FeedbackInterval)
	}
}

// Local Variables:
// tab-width: 4
// End:

func TestApplySettings_LogfileChange(t *testing.T) {
	s := newApplySession(t, map[string][]string{
		"primary_conninfo": {"host=localhost"},
		"logfile":          {"//stderr"},
	})

	nCfg, err := s.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if err := s.applySettings(nCfg, nCfg.recvP); err != nil {
		t.Fatalf("applySettings: %v", err)
	}
	if s.logURL != "//stderr" {
		t.Errorf("logURL: got %q, want //stderr", s.logURL)
	}
	if s.lg == nil {
		t.Error("logger should be set")
	}

	// Changing the logfile again must create a new logger and set
	// CloseOnActivation on the new param.
	oldLg := s.lg
	nCfg2, err := s.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings 2: %v", err)
	}
	nCfg2.logURL = "" // change to default logger
	err = s.applySettings(nCfg2, nCfg2.recvP)
	if err != nil {
		t.Fatalf("applySettings 2: %v", err)
	}
	if s.logURL != "" {
		t.Errorf("logURL: got %q, want empty", s.logURL)
	}
	if s.lg == oldLg {
		t.Error("logger should have been replaced")
	}
	if nCfg2.recvP.CloseOnActivation == nil {
		t.Error("CloseOnActivation should be set so the old logger is closed")
	}
}

func TestApplySettings_LogfileChangeClosesOld(t *testing.T) {
	s := newApplySession(t, map[string][]string{
		"primary_conninfo": {"host=localhost"},
		"logfile":          {"//stderr"},
	})

	nCfg, err := s.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if err := s.applySettings(nCfg, nCfg.recvP); err != nil {
		t.Fatalf("applySettings: %v", err)
	}

	// Simulate the receiver activating the new params: closing the
	// channel must close the old logger without blocking.
	oldLg := s.lg
	nCfg2, err := s.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings 2: %v", err)
	}
	nCfg2.logURL = "" // force a logfile change
	if err := s.applySettings(nCfg2, nCfg2.recvP); err != nil {
		t.Fatalf("applySettings 2: %v", err)
	}
	close(nCfg2.recvP.CloseOnActivation)
	// Give the closer goroutine a moment.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if oldLg.IsClosed() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("old logger was not closed within 2s")
}

func TestApplySettings_LogfileInvalidURL(t *testing.T) {
	s := newApplySession(t, map[string][]string{
		"primary_conninfo": {"host=localhost"},
		"logfile":          {"//stderr"},
	})

	nCfg, err := s.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if err := s.applySettings(nCfg, nCfg.recvP); err != nil {
		t.Fatalf("applySettings: %v", err)
	}

	// An invalid logfile URL must abort the reload without changing
	// the current logger.
	oldLg := s.lg
	nCfg2, err := s.readSettings(false)
	if err != nil {
		t.Fatalf("readSettings 2: %v", err)
	}
	nCfg2.logURL = "nosuchscheme://x"
	err = s.applySettings(nCfg2, nCfg2.recvP)
	if err == nil {
		t.Fatal("invalid logfile URL should fail")
	}
	if s.lg != oldLg {
		t.Error("logger must not change when the new one fails to open")
	}
	if s.logURL != "//stderr" {
		t.Errorf("logURL must not change: got %q", s.logURL)
	}
}
