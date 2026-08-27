package capture

import (
	"os"
	"os/signal"
	// "fmt"
	"errors"
	"syscall"
	// "context"
	"path/filepath"

	"github.com/jackc/pglogrepl"

	"github.com/tfoertsch123/log"
	"github.com/tfoertsch123/own-your-pg/slot"
	"github.com/tfoertsch123/own-your-pg/defaults"
	cap "github.com/tfoertsch123/own-your-pg/pglogreplsimple"
)

type Cli struct {
	// Dir specifies the working directory, required.
	Dir string `arg:"" required:"" help:"Working directory."`

	// Slot is optional, short flag -S.
	Slot string `short:"S" help:"Slot name." default:"${basename}"`
}

var ErrShutdown = errors.New("Shutdown signal")
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

	cfg := Cfg{
		sl: sl,
	}
	_, err = cfg.readSettings(false)
	if err != nil {
		log.Panicf("%v", err)
	}

	cfg.lg.Noticef("Slot: %v", sl)
	m := cap.NewReceiver(
		cap.WithParams(cfg.recvP),
		cap.WithAcceptedPlugins(map[string][]string{
			"wal2json": cap.DefaultPlugins["wal2json"],
		}),
		cap.WithGetStartLSN(func() pglogrepl.LSN {
			lsn, _ := cfg.sl.GetLSN()
			return pglogrepl.LSN(lsn)
		}),
	)

	shutdownCh := make(chan os.Signal, 1)
    signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	reloadCh := make(chan os.Signal, 1)
    signal.Notify(reloadCh, syscall.SIGHUP)

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
				cfg.lg.Infof("got signal %v", s)
				m.Shutdown(ErrShutdown)
            case s := <-reloadCh:
				cfg.lg.Infof("got signal %v", s)
				if send, err := cfg.readSettings(true); err != nil {
					cfg.lg.Errorf("reload: %v", err)
				} else if send {
					m.RequestReload(*cfg.recvP)
				}
            }
        }
    }()

	// for msg := range m.Produce(nil) {
	// 	cfg.mlg.Infof(">> %T", msg)
	// }
	// if err = m.Err(); err != nil {
	// 	cfg.lg.Infof("lastErr: %v", err)	
	// }
	i := 0
	nloops := 0
	for {
		nloops++
		it, err := m.Produce(nil)
		if err != nil {
			cfg.lg.Errorf("Produce(): %v", err)
			break
		}
		for msg := range it {
			cfg.mlg.Infof("%d: loop 1(%d) %T message", nloops, i, msg)
			i++
			if i > 10 {
				break
			}
		}
		cfg.mlg.Infof("%d: loop 1 done", nloops)

		cfg.lg.Infof("lastErr: %v", m.Err())	
		if m.State() == cap.Stop {
			break
		}

		it, err = m.Produce(nil)
		if err != nil {
			cfg.lg.Errorf("Produce(): %v", err)
			break
		}
		for msg := range it {
			cfg.mlg.Infof("%d: loop 2(%d) %T message", nloops, i, msg)
			i--
			if i <= 0 {
				break
			}			
		}
		cfg.mlg.Infof("%d: loop 2 done", nloops)
		cfg.lg.Infof("lastErr: %v", m.Err())	
		if m.State() == cap.Stop {
			break
		}

		if nloops == 2 {
			cfg.lg.Infof("m.Close(): %v", m.Close())	
		}
	}

	cfg.lg.Info("Shutdown complete")	
}

// Local Variables:
// tab-width: 4
// End:
