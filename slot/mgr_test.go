package slot

import (
	"testing"
	"path/filepath"
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

// Local Variables:
// tab-width: 4
// End:
