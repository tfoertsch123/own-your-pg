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

func IntroHelp(link func(url, text string) string) string {
	return `iuvbipbv pioubvapqfiurvp
vupiqabvui
vpquierbv
vqwehnbvq
bqwb
wqtreb
wqtb
wqtvb casnb casd crgqer gqer gqerg qerg qerg qerg qergf qerg qerg qerg erq
gqer gqer gqer g qerg eqrg reqg qer gqe rg qerg eqrg qerg eqr geq
gqer geqr gq erg eqrg erqg erqg eqrg eqrg eqrg qer geqr g qerg er g
fveqrfvgqerv

vfqer
vqer
vqer
vqer
vqrviohqpvuifoq vq ervqervunbqerv qrev qerv qervqer vqerv qerv`
}

// SlotConfigHelp returns the slot configuration help text. The link
// function is used to render URLs; if the terminal supports OSC 8
// hyperlinks, link wraps the URL in escape sequences, otherwise it
// returns the URL as-is.
func SlotHelp(link func(url, text string) string) string {
	return `Slot configuration:

The capture process reads its configuration from the slot.
The following slot parameters are recognized:

  primary_conninfo         libpq connection string (required)
  primary_slotname         PG replication slot name (default: slot name)
  logfile                  log destination URL (e.g. //stderr, file:///path)
  size_limit               file size limit to trigger rotation (default: 16MiB)
  synchronous              on/true/1 for synchronous mode (default: off)
  ignore_missing_identity  tables to ignore missing replication identity

For more information about the logfile specification, see
` + link("https://pkg.go.dev/github.com/tfoertsch123/log#ParseURL",
		"the log package documentation") + `

The size_limit if given as a simple integer number specifies the size in bytes.
A unit can be appended according to the ParseStrictBytes function in
` + link("https://pkg.go.dev/github.com/alecthomas/units#ParseStrictBytes",
		"alecthomas/units package") + `

The actual file size can significantly exceed size_limit. The capture process
never breaks up a DB transaction into several files. So, a large change in
one transaction can create GB-sized files even if size_limit=10KiB.

Use slottool to set these parameters.`
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

	s := session{
		wd: cli.Dir,
		mgr: mgr,
		sl: sl,
		currDirFd: -1,
	}

	s.ensureIncDir()

	nCfg, err := s.readSettings(false)
	if err != nil {
		log.Panicf("%v", err)
	}
	err = s.applySettings(nCfg, nCfg.recvP)
	if err != nil {
		log.Panicf("%v", err)
	}

	// read last committed LSN from metadata and adjust slotLSN if necessary
	// The metadata update represents the commit of the data to disk. There
	// is a small time window between metadata update and writing the slot
	// LSN. If we crashed in this window, the slot LSN is not up to date.
	startLSN, _ := s.sl.GetLSN()
	llsn_ := s.curInit()
	if llsn_ > startLSN {		// did we crash?
		s.lg.Noticef("Metadata LSN (%v) > slot LSN (%v) -- did we crash?",
			llsn_, startLSN)
		err = s.sl.SetLSN(llsn_, true)
		if err != nil {
			log.Panicf("%v", err)
		}
	}

	m := cap.NewReceiver(
		cap.WithParams(s.recvP),
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
			return s.truncateToEoc()
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
					*s.prepareReload(s.readSettings(true)),
				)
            }
        }
    }()

	it, err := m.Produce(nil)
	if err != nil {
		s.lg.Panicf("Produce: %v", err)
	}

	s.consume(it, m.AckLSN)

	if err = m.Err(); err != nil {
		s.lg.Errorf("Err: %v", err)
	}

	s.lg.Info("Shutdown complete")
}

// Local Variables:
// tab-width: 4
// End:
