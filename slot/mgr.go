package slot

import (
	"errors"
	"iter"

	"golang.org/x/sys/unix"

	"github.com/tfoertsch123/flock"
	"github.com/tfoertsch123/own-your-pg/ls"
)

type Mgr struct {
	dir string
	lck *flock.Lock				// producer lock (flock() on dir)
}

// TODO:
// - perhaps check existence
// - perhaps create
// - perhaps open dir
func NewMgr(dir string) (*Mgr, error) {
	return &Mgr{dir: dir}, nil
}

func (m *Mgr) Close() error {
	if m.lck != nil {
		l := m.lck
		m.lck = nil
		return l.Close()
	}

	return nil
}

func (m *Mgr) Dir() string {
	return m.dir
}

func (m *Mgr) lock() (bool, error) {
	if m.lck == nil {
		m.lck = flock.New(
			flock.WithPath(m.dir),
			flock.WithIsDir(),
		)
	}
	err := m.lck.TryLockEx()
	if err == nil {
		return true, nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return false, err
}

func (m *Mgr) unlock() error {
	return m.lck.Unlock()
}

func (m *Mgr) Slots() iter.Seq2[*ls.DirItem, error] {
	return ls.Ls(m.dir, nil)
}

// Local Variables:
// tab-width: 4
// End:
