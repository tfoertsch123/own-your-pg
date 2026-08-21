package m2

import (
	"os"
	"os/signal"
	"fmt"
	"errors"
	"syscall"
	"context"
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

var ErrShutdown = errors.New("Shutdown ")
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

    ctx, cancel := context.WithCancelCause(context.Background())
    defer cancel(nil)

	shutdownCh := make(chan os.Signal, 1)
    signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	reloadCh := make(chan os.Signal, 1)
    signal.Notify(reloadCh, syscall.SIGHUP)

	reload_chan := make(chan struct{}, 1)
	
	// signal loop
    go func() {
		defer func() {
			signal.Stop(shutdownCh)
			signal.Stop(reloadCh)
			close(shutdownCh)
			close(reloadCh)
		}()
        for {
            select {
            case s := <-shutdownCh:
				log.Noticef("got signal %v", s)
                cancel(fmt.Errorf("%s: %w", s, ErrShutdown))
				signal.Stop(shutdownCh) // one shutdown should be enough
            case <-reloadCh:
				select {case reload_chan <- struct{}{}: default:}
            case <-ctx.Done():
                return
            }
        }
    }()

	m := Mon{
		sl: sl,
		shutdown_ctx: ctx,
		shutdown_trg: cancel,
		reload: reload_chan,
		settings: make(map[string]string, 10),
	}

	for msg := range m.Produce() {
		m.mlg.Debugf("%#v", msg)
	}

	m.lg.Info("Shutdown complete")	
}

// Local Variables:
// tab-width: 4
// End:
