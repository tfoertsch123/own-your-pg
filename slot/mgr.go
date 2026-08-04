package slot

import (
	"os"
	"errors"
	"slices"
	"regexp"

	"golang.org/x/sys/unix"

	"github.com/tfoertsch123/flock"
	"github.com/tfoertsch123/linux-ls"
)

type Mgr struct {
	dir string
	lck *flock.Lock				// producer lock (flock() on dir)
}

type mgrOpts struct {
	check  bool
	create bool
}

type MgrOpt func(*mgrOpts)

func WithMgrCreate() MgrOpt {
	return func(t *mgrOpts) {
		t.create = true
	}
}

func WithMgrCheck() MgrOpt {
	return func(t *mgrOpts) {
		t.check = true
	}
}

func (m *Mgr) Check(create bool) error {
	err := m.lck.Open()
	if err == nil || errors.Is(err, flock.ErrAlreadyOpen) {
		return nil
	}

	if !create {
		return err
	}

	if errors.Is(err, unix.ENOENT) {
		err = os.Mkdir(m.dir, 0777) // use umask
		if err != nil {
			return err
		}
		err = m.lck.Open()
		if err == nil {
			return nil
		}
	}

	return err
}

func NewMgr(dir string, opts ...MgrOpt) (*Mgr, error) {
	x := &mgrOpts{}
	for _, o := range opts {
		o(x)
	}

	flo := []flock.Option{flock.WithPath(dir), flock.WithIsDir()}
	m := &Mgr{
		dir: dir,
		lck: flock.New(flo...),
	}

	if !x.check && !x.create {
		return m, nil
	}

	if err := m.Check(x.create); err != nil {
		return nil, err
	}

	return m, nil
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

func (m *Mgr) Slots() ([]string, error) {
	slots := []string{}
	re := regexp.MustCompile(`\`+fext+`$`)
	for item, err := range ls.LsPath(m.dir, re, unix.DT_REG) {
		if err != nil {
			return nil, err
		}
		slots = append(slots, item.Name[:len(item.Name)-len(fext)])
	}
	slices.Sort(slots)
	return slots, nil
}

// Local Variables:
// tab-width: 4
// End:
