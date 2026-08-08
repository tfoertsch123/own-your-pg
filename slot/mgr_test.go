package slot

import (
	"testing"
	"os"
	"errors"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/tfoertsch123/flock"
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

func TestMgrOptions(t *testing.T) {
	dir := t.TempDir()
	enoent := filepath.Join(dir, "ENOENT")

	// ENOENT without any options is tested in TestMgr

	t.Run("WithMgrCheck", func(t *testing.T) {
		m, err := NewMgr(enoent, WithMgrCheck())
		if !errors.Is(err, unix.ENOENT) {
			t.Errorf("NewMgr(%v): exp %s, got %s", enoent, unix.ENOENT, err)
		}
		if m != nil {
			t.Errorf("NewMgr(%v): exp nil, got %v", enoent, m)
			m.Close()
		}
	})

	t.Run("WithMgrCreate parent missing", func(t *testing.T) {
		m, err := NewMgr(filepath.Join(enoent, "ENOENT"), WithMgrCreate())
		if !errors.Is(err, unix.ENOENT) {
			t.Errorf("NewMgr(%v): exp %s, got %s", enoent, unix.ENOENT, err)
		}
		if m != nil {
			t.Errorf("NewMgr(%v): exp nil, got %v", enoent, m)
			m.Close()
		}
	})

	t.Run("WithMgrCreate success", func(t *testing.T) {
		m, err := NewMgr(enoent, WithMgrCreate())
		if err != nil {
			t.Errorf("NewMgr(%v): exp no error, got %s", enoent, err)
		}
		if m == nil {
			t.Errorf("NewMgr(%v): exp manager, got nil", enoent)
		} else {
			m.Close()
		}

		// check the directory
		finfo, err := os.Stat(enoent)
		if err != nil {
			t.Fatalf("NewMgr(%v): Stat: %s", enoent, err)
		}
		if !finfo.Mode().IsDir() {
			t.Errorf("NewMgr(%v): is not a directory", enoent)
		}

		// if the directory is not empty this will fail (at least on unix)
		err = os.Remove(enoent)
		if err != nil {
			t.Fatalf("NewMgr(%v): Remove: %s", enoent, err)
		}

		// what if the parent dir is read-only?
		err = os.Chmod(dir, 0500)
		defer func(){
			os.Chmod(dir, 0755)
		}()
		if err != nil {
			t.Fatalf("NewMgr(%v): Chmod(parent): %s", enoent, err)
		}

		m, err = NewMgr(enoent, WithMgrCreate())
		if !errors.Is(err, unix.EACCES) {
			t.Errorf("NewMgr(%v): exp EACCES, got %s", enoent, err)
		}
		if m != nil {
			t.Errorf("NewMgr(%v): exp nil, got %v", enoent, m)
			m.Close()
		}
	})

	t.Run("WithMgrCheck bad Check", func(t *testing.T) {
		m, err := NewMgr(dir, WithMgrCheck())
		if err != nil {
			t.Fatalf("NewMgr(%v): exp success, got %v", dir, err)
		}
		defer m.Close()

		err = m.lck.Close()
		if err != nil {
			t.Fatalf("NewMgr(%v): lck.Close(), got %v", dir, err)
		}

		err = m.Check(false)
		if err == nil {
			t.Fatalf("NewMgr(%v): lck.Check(false) exp %v, got %v",
				dir, flock.ErrAlreadyClosed, err)
		}

		err = m.Check(true)
		if err == nil {
			t.Fatalf("NewMgr(%v): lck.Check(true) exp %v, got %v",
				dir, flock.ErrAlreadyClosed, err)
		}
	})
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
