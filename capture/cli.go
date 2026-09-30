package capture

import (
	"os"
	"os/signal"
	"errors"
	"syscall"
	"path/filepath"
	"bytes"

	"github.com/jackc/pglogrepl"

	"github.com/tfoertsch123/log"
	"github.com/tfoertsch123/own-your-pg/slot"
	"github.com/tfoertsch123/own-your-pg/defaults"
	"github.com/tfoertsch123/own-your-pg/lsn"
	cap "github.com/tfoertsch123/pglogreplsimple"
)

type Cli struct {
	// Dir specifies the working directory, required.
	Dir string `arg:"" required:"" help:"Working directory."`

	// Slot is optional, short flag -S.
	Slot string `short:"S" help:"Slot name." default:"${basename}"`
}

type Cfg struct {
	wd string					// only for error messages
	mgr *slot.Mgr
	sl *slot.Slot
	firstTxnLSN *lsn.LSN		// "nextlsn" of first BEGIN in current file
								// ONLY used to build the history file name
	boBegin int64				// file pos of the most recent B record
	eoCommit int64				// file pos right after the most recent C record
	curr *os.File
	writer *bytes.Buffer

	logURL string
	lg *log.Logger
	mlg *log.Logger				// to be used in the writing part
	recvP *cap.Param
	maxSize int64
	ignMissId []string
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
		wd: cli.Dir,
		mgr: mgr,
		sl: sl,
	}

	cfg.ensureIncDir()

	_, err = cfg.readSettings(false)
	if err != nil {
		log.Panicf("%v", err)
	}

	// TODO: read last committed LSN from file and adjust slotLSN if necessary
	cfg.curInit()

	cfg.lg.Noticef("Slot: %v", sl)

	startLSN, _ := cfg.sl.GetLSN()
	m := cap.NewReceiver(
		cap.WithParams(cfg.recvP),
		cap.WithAcceptedPlugins(map[string][]string{
			"wal2json": []string{
				`"format-version" '2'`,
				`"include-types" 'true'`,
				`"include-xids" 'true'`,
				`"include-timestamp" 'true'`,
				// `"include-lsn" 'true'`,
				`"numeric-data-types-as-string" 'true'`,
			},
		}),
		cap.WithStartLSN(pglogrepl.LSN(startLSN)),
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

	it, err := m.Produce(nil)
	if err != nil {
		cfg.lg.Panicf("Produce: %v", err)
	}

	cfg.consume(it, m.AckLSN)

	if err = m.Err(); err != nil {
		cfg.lg.Infof("lastErr: %v", err)
	}

	cfg.lg.Info("Shutdown complete")	
}

// Local Variables:
// tab-width: 4
// End:
