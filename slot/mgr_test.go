package slot

import (
	"testing"
	"os"
	"errors"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func TestMgr(t *testing.T) {
	dir := t.TempDir()
	m, err := NewMgr(dir)
	if err != nil {
		t.Fatalf("NewMgr(%v): exp %s, got %s", dir, error(nil), err)
	}
	defer m.Close()
	
	if m.dir != dir {
		t.Errorf("NewMgr(%v): exp %s, got %s", dir, dir, m.dir)
	}

	if m.Dir() != dir {
		t.Errorf("Dir(): exp %s, got %s", dir, m.dir)
	}

	success, err := m.lock()
	if !success || err != nil {
		t.Errorf("lock(): exp true/nil, got %v/%v", success, err)
	}

	m2, err := NewMgr(dir)
	if err != nil {
		t.Fatalf("m2(%v): exp %s, got %s", dir, error(nil), err)
	}
	defer m2.Close()

	success, err = m2.lock()
	if success || err != nil {
		t.Errorf("lock(): exp false/nil, got %v/%v", success, err)
	}

	err = m2.unlock()
	if err != nil {
		t.Errorf("unlock(): exp nil, got %v", err)
	}

	err = m2.Close()
	if err != nil {
		t.Errorf("Close(m2/1): exp nil, got %v", err)
	}

	err = m2.Close()
	if err != nil {
		t.Errorf("Close(m2/2): exp nil, got %v", err)
	}

	m3, err := NewMgr(filepath.Join(dir, "ENOENT"))
	if err != nil {
		t.Fatalf("m3(%v): exp %s, got %s", dir, error(nil), err)
	}
	defer m3.Close()

	success, err = m3.lock()
	if success || err == nil {
		t.Errorf("lock(): exp false/err, got %v/%v", success, err)
	}
}

func TestMgrSlots(t *testing.T) {
	dir := t.TempDir()
	m, err := NewMgr(dir)
	if err != nil {
		t.Fatalf("Could not create manager %v", err)
	}
	defer m.Close()

	t.Run("invalid dir", func(t *testing.T) {
		m, err := NewMgr(filepath.Join(dir, "ENOENT"))
		if err != nil {
			t.Fatalf("Could not create ENOENT manager %v", err)
		}
		defer m.Close()

		_, err = m.Slots()
		if !errors.Is(err, unix.ENOENT) {
			t.Fatalf("m.Slots: expected ENOENT")
		}
	})

	t.Run("empty dir", func(t *testing.T) {
		names, err := m.Slots()
		if err != nil {
			t.Fatalf("m.Slots: %v, expected nil", err)
		}

		if len(names) != 0 {
			t.Errorf("m.Slots: got %v names, exp 0", len(names))
		}
	})

	t.Run("only non-slots", func(t *testing.T) {
		err = os.Symlink("test2"+fext, filepath.Join(m.dir, "symlnk"+fext))
		if err != nil {
			t.Fatalf("Could not create symlink %v", err)
		}

		err = os.Mkdir(filepath.Join(m.dir, "dir"+fext), 0777)
		if err != nil {
			t.Fatalf("Could not create dir %v", err)
		}

		names, err := m.Slots()
		if err != nil {
			t.Fatalf("m.Slots: %v, expected nil", err)
		}

		if len(names) != 0 {
			t.Errorf("m.Slots: got %v names, exp 0", len(names))
		}
	})

	t.Run("list_slots", func(t *testing.T) {
		sl, err := m.Slot("some_slot", WithCreate(), WithType(Archiver))
		if err != nil {
			t.Fatalf("Could not create slot %v", err)
		}
		sl.Close()

		sl, err = m.Slot("other_slot", WithCreate(), WithType(Producer))
		if err != nil {
			t.Fatalf("Could not create slot %v", err)
		}
		sl.Close()

		err = os.WriteFile(filepath.Join(m.dir, "test2"+fext), []byte(""), 0777)
		if err != nil {
			t.Fatalf("Could not create file %v", err)
		}

		// in alphabetical order, only files
		exp := []string{"other_slot", "some_slot", "test2"}
		names, err := m.Slots()
		if err != nil {
			t.Fatalf("m.Slots: %v, expected nil", err)
		}

		if len(names) != len(exp) {
			t.Errorf("m.Slots: got %v names, exp %v", len(names), len(exp))
		}

		for i, s := range names {
			if s != exp[i] {
				t.Errorf("m.Slots: %d: <%v> != <%v>", i, s, exp[i])
			}
		}
	})
}

// Local Variables:
// tab-width: 4
// End:
