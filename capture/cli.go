package capture

import (
	"os"
	"os/signal"
	"errors"
	"syscall"
	"path/filepath"
	"bytes"
	"fmt"

	"github.com/jackc/pglogrepl"

	"github.com/tfoertsch123/log"
	"github.com/tfoertsch123/flock"
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

type reloadable struct {
	logURL string
	recvP *cap.Param
	maxSize int64
	ignMissId []string
}

func (r *reloadable) String() string {
	return fmt.Sprintf(
		"logfile: %v\n" +
			"primary_conninfo: %v\n" +
			"primary_slotname: %v\n" +
			"reconnect_interval: %v\n" +
			"feedback_interval: %v\n" +
			"synchronous: %v\n" +
			"max_size: %v\n" +
			"ignore_missing_identity: %v\n",
		r.logURL, r.recvP.ConnInfo, r.recvP.SlotName,
		r.recvP.ErrorRetryInterval, r.recvP.FeedbackInterval,
		r.recvP.FeedbackOnFlush,
		r.maxSize, r.ignMissId,
	)
}

type session struct {
	wd string					// only for error messages
	mgr *slot.Mgr
	sl *slot.Slot
	firstTxnLSN *lsn.LSN		// "nextlsn" of first BEGIN in current file
								// ONLY used to build the history file name
	boBegin int64				// file pos of the most recent B record
	eoCommit int64				// file pos right after the most recent C record
	currDirFd int
	histDirFd int
	meta *os.File
	curr *os.File
	currLck *flock.Lock
	writer *bytes.Buffer

	lg *log.Logger
	mlg *log.Logger				// to be used in the writing part

	reloadable
}

var (
	ErrShutdown          = errors.New("Shutdown signal")
	ErrMissingConninfo   = errors.New("primary_conninfo not set")
	ErrInvalidConninfo   = errors.New("cannot parse primary_conninfo")
	ErrInvalidLimit      = errors.New("size_limit is invalid")
	ErrInvalidSynchronous = errors.New("synchronous is invalid")
	ErrCurrGarbage       = errors.New("current file is garbage")
	ErrMetaGarbage       = errors.New("metadata file is garbage")
)

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

	cfg := session{
		wd: cli.Dir,
		mgr: mgr,
		sl: sl,
		currDirFd: -1,
	}

	cfg.ensureIncDir()

	nCfg, err := cfg.readSettings(false)
	if err != nil {
		log.Panicf("%v", err)
	}
	err = cfg.applySettings(nCfg, nCfg.recvP)
	if err != nil {
		log.Panicf("%v", err)
	}

	// read last committed LSN from metadata and adjust slotLSN if necessary
	// The metadata update represents the commit of the data to disk. There
	// is a small time window between metadata update and writing the slot
	// LSN. If we crashed in this window, the slot LSN is not up to date.
	startLSN, _ := cfg.sl.GetLSN()
	llsn_ := cfg.curInit()
	if llsn_ > startLSN {		// did we crash?
		cfg.lg.Noticef("Metadata LSN (%v) > slot LSN (%v) -- did we crash?",
			llsn_, startLSN)
		err = cfg.sl.SetLSN(llsn_, true)
		if err != nil {
			log.Panicf("%v", err)
		}
	}

	m := cap.NewReceiver(
		cap.WithParams(cfg.recvP),
		cap.WithAcceptedPlugins(map[string][]string{
			"wal2json": []string{
				`"format-version" '2'`,
				`"include-types" 'true'`,
				`"include-xids" 'true'`,
				`"include-timestamp" 'true'`,
				`"numeric-data-types-as-string" 'true'`,
			},
		}),
		cap.WithStartLSN(pglogrepl.LSN(startLSN)),
		cap.WithOnConnect(func (_ *cap.Receiver) error {
			return cfg.truncateToEoc()
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
            case <-shutdownCh:
				m.Shutdown(ErrShutdown)
            case <-reloadCh:
				m.RequestReload(
					*cfg.prepareReload(cfg.readSettings(true)),
				)
            }
        }
    }()

	it, err := m.Produce(nil)
	if err != nil {
		cfg.lg.Panicf("Produce: %v", err)
	}

	cfg.consume(it, m.AckLSN)

	if err = m.Err(); err != nil {
		cfg.lg.Errorf("Err: %v", err)
	}

	cfg.lg.Info("Shutdown complete")	
}

// Local Variables:
// tab-width: 4
// End:
