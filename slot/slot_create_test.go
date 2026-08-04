package slot

import (
	"testing"
)

func TestSlotCreate1(t *testing.T) {
	mgr, err := NewMgr("tmp")

	if err != nil {
		t.Fatalf("Could not create manager: %v", err)
	}

	sl, err := mgr.Slot("non-existent", WithType(Archiver))
	if sl != nil || err != nil {
		t.Error("Opening a non-existent slot should return nil without error")
	}

	sl, err = mgr.Slot("test", WithCreate(), WithType(Archiver))

	t.Logf("err: %v", err)
	t.Logf("sl: %v", sl)

	sl, err = mgr.Slot("test", WithCreate())

	t.Logf("err: %v", err)
	t.Logf("sl: %v", sl)
}

// Local Variables:
// tab-width: 4
// End:
