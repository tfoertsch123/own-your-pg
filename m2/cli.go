package m2

import (
	"path/filepath"

	"github.com/tfoertsch123/log"
	"github.com/tfoertsch123/own-your-pg/slot"
	"github.com/tfoertsch123/own-your-pg/defaults"
)

type Cli struct {
	// Dir specifies the working directory, required.
	Dir string `arg:"" required:"" help:"Working directory."`

	// Slot is optional, short flag -S.
	Slot string `short:"S" help:"Slot name." default:"${basename}"`
}

func (cli *Cli) Run() {
	sd := filepath.Join(cli.Dir, defaults.SlotDir)
	mgr, err := slot.NewMgr(sd, slot.WithMgrCheck())
	if err != nil {
		log.Panicf("%s: %v", sd, err)
	}

	sl, err := mgr.Slot(
		cli.Slot,
		slot.WithAsOwner(),
		slot.WithType(slot.Producer),
	)
	if err != nil {
		log.Panicf("%v", err)
	}

	m := Mon{
		sl: sl,
		reinit: make(chan struct{}, 1),
		shutdown: make(chan struct{}, 0),
		settings: make(map[string]string, 10),
	}
	err = m.readSettings(false)
	if err != nil {
		log.Panicf("%v", err)
	}

	m.ConnInit()
}

// Local Variables:
// tab-width: 4
// End:
