package slottool

import (
	"fmt"
	"errors"
	"path/filepath"

	"golang.org/x/sys/unix"
	"github.com/tfoertsch123/log"

	"github.com/tfoertsch123/own-your-pg/slot"
	"github.com/tfoertsch123/own-your-pg/defaults"
)

func (args *Cli) List(m *slot.Mgr) {
	l := func(nm string) {
		sl, err := m.Slot(nm)
		if err != nil {
			log.Panicf("Could not open slot %s: %v", nm, err)
		}

		js, err := sl.AsJSON(slot.WithPidCheck())
		if err != nil {
			log.Errorf("JSON conversion failed: %v", err)
		} else {
			fmt.Println(js)
		}
	}

	if args.Slot == nil {
		slots, err := m.Slots()
		if err != nil {
			log.Panicf("While listing slots: %v", err)
		}
		for _, nm := range slots {
			l(nm)
		}
	} else {
		l(*args.Slot)
	}
}

func (args *Cli) Modify(m *slot.Mgr) {
	nm := *args.Slot
	slo := []slot.SlotOpt{slot.WithCreate()}

	if args.Type != nil || args.LSN != nil {
		slo = append(slo, slot.WithNoPidLock())
	}

	sl, err := m.Slot(nm, slo...)
	if err != nil {
		log.Panicf("Could not open slot %s: %v", nm, err)
	}
	defer sl.Close()

	if args.Type != nil {
		err = sl.SetType(
			*args.Type,
			*args.Type == slot.Config || args.LSN == nil,
		)
		if err != nil {
			log.Errorf("Could write type %s: %v", nm, err)
		}
	}

	if args.LSN != nil {
		err = sl.SetLSN(*args.LSN, true)
		if err != nil {
			log.Errorf("Could write LSN %s: %v", nm, err)
		}
	}

	if len(args.Config) > 0 {
		for _, cfg := range args.Config {
			if cfg.value == nil {
				sl.SetConfig(cfg.name, nil)
			} else {
				sl.SetConfig(cfg.name, *cfg.value)
			}
		}
		err = sl.SaveConfig(true)
		if err != nil {
			log.Errorf("Could write config %s: %v", nm, err)
		}
	}

	js, err := sl.AsJSON(slot.WithPidCheck())
	if err != nil {
		log.Errorf("JSON conversion failed: %v", err)
	} else {
		fmt.Println(js)
	}
}

// mgr opens the slot directory. The slot directory is placed inside the
// working directory and named [defaults.SlotDir]. The input, args.Dir,
// can either point at the working directory or the slot directory itself.
// If create is true, the slot directory will be optionally created. The
// working directory will not be created. It must exist.
func (args *Cli) mgr(create bool) (*slot.Mgr, error) {
	// try .../workingdir/slots
	dir := filepath.Join(args.Dir, defaults.SlotDir)
	var m *slot.Mgr
	var err error

	try := func() bool {
		m, err = slot.NewMgr(dir, slot.WithMgrCheck())
		if err == nil {
			return true
		}
		if !errors.Is(err, unix.ENOENT) {
			return true
		}
		return false
	}

	if try() {
		return m, err
	}

	// ok, perhaps we were given the slots dir directly
	// in this case we require the basename of the dir to be defaults.SlotDir
	_, base := filepath.Split(args.Dir)
	if base == defaults.SlotDir {
		dir = args.Dir

		if try() {
			return m, err
		}
	}

	if !create {
		return nil, err
	}

	return slot.NewMgr(dir, slot.WithMgrCreate())
}

func (args *Cli) Run() {
	if args.Config == nil && args.Type == nil && args.LSN == nil {
		// this is listing only. So, we fail if the mgr does not exist
		// and cannot be created.
		m, err := args.mgr(false)
		if err != nil {
			log.Panicf("Could not create manager: %v", err)
			return
		}
		defer m.Close()

		args.List(m)
	} else {
		if args.Slot == nil {
			log.Panicf("-S slotname required")
		}

		m, err := args.mgr(true)
		if err != nil {
			log.Panicf("Could not create manager: %v", err)
			return
		}
		defer m.Close()

		args.Modify(m)
	}
}

// Local Variables:
// tab-width: 4
// End:
