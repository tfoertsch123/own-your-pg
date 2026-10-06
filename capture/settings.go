package capture

import (
	"errors"
	"strconv"
	"strings"
	"github.com/alecthomas/units"
	"github.com/tfoertsch123/log"
	"github.com/tfoertsch123/own-your-pg/defaults"
	cap "github.com/tfoertsch123/pglogreplsimple"
	"github.com/tfoertsch123/pgconnstr"
)

var ErrMissingConninfo error = errors.New("primary_conninfo not set")
var ErrInvalidConninfo error = errors.New("cannot parse primary_conninfo")
var ErrInvalidLimit error = errors.New("size_limit is invalid")
var ErrInvalidSynchronous error = errors.New("synchronous is invalid")

// readSettings reads the slot and returns a new *Reloadable parameter package
// or an error.
// readSettings must not access anything other than m.sl, not even a logger.
// It can be called by a separate go routine.
func (m *Cfg) readSettings(update bool) (*Reloadable, error) {
	nCfg := &Reloadable{recvP: &cap.Param{}}
	if x, err := m.sl.GetConfig("logfile", update); err != nil {
		return nCfg, err
	} else if len(x) >= 1 {
		nCfg.logURL = x[0]
	}

	x, _ := m.sl.GetConfig("primary_conninfo", false)
	if len(x) >= 1 {
		if ci, err := pgconnstr.Parse(x[0]); err != nil {
			return nCfg, ErrInvalidConninfo
		} else {
			if _, exists := ci["application_name"]; ! exists {
				ci["application_name"] = "OYPG-"+m.sl.Name()
			}
			ci["options"] = "--client_min_messages=warning " +
				"--log_min_duration_statement=0"
			nCfg.recvP.ConnInfo = ci.URL()
		}
	} else {
		return nCfg, ErrMissingConninfo
	}

	x, _ = m.sl.GetConfig("primary_slotname", false)
	if len(x) >= 1 {
		nCfg.recvP.SlotName = x[0]
	} else {
		nCfg.recvP.SlotName = m.sl.Name()
	}

	x, _ = m.sl.GetConfig("ignore_missing_identity", false)
	nCfg.ignMissId = x

	x, _ = m.sl.GetConfig("size_limit", false)
	if len(x) == 0 {
		x = append(x, defaults.M2SizeLimit)
	}
	if x1, err := strconv.ParseUint(x[0], 10, 64); err == nil {
		nCfg.maxSize = int64(x1)
	} else if x1, err := units.ParseStrictBytes(x[0]); err == nil {
		nCfg.maxSize = int64(x1)
	} else {
		return nCfg, ErrInvalidLimit
	}

	feedbackOnFlush := false
	x, _ = m.sl.GetConfig("synchronous", false)
	if len(x) >= 1 {
		switch strings.ToLower(x[0]) {
		case "on", "true", "1":
			feedbackOnFlush = true
		case "off", "false", "0":
		default:
			return nCfg, ErrInvalidSynchronous
		}
	}
	nCfg.recvP.FeedbackOnFlush = feedbackOnFlush

	nCfg.recvP.ErrorRetryInterval = cap.DefaultErrorRetryInterval
	nCfg.recvP.FeedbackInterval = cap.DefaultFeedbackInterval

	return nCfg, nil
}

// called by the reload signal handler in a separate go routine.
// sets the OnActivation handler
func (m *Cfg) prepareReload(nCfg *Reloadable, err error) *cap.Param {
	nCfg.recvP.OnActivation = func(p *cap.Param) error {
		if err != nil {
			return err
		}
		return m.applySettings(nCfg, p)
	}
	return nCfg.recvP
}

// this is called by the receiver (cap) object in the main go routine. It
// can access all the fields in m. The passed in recvP is not the same as
// nCfg.revcP. Logically it is but it has been copied somewhere along the
// way. So, changes must be made there.
func (m *Cfg) applySettings(nCfg *Reloadable, recvP *cap.Param) error {
	ret := cap.ErrNoChange
	if m.logURL != nCfg.logURL ||
	   m.recvP.ConnInfo != recvP.ConnInfo ||
	   m.recvP.SlotName != recvP.SlotName ||
	   m.recvP.ErrorRetryInterval != recvP.ErrorRetryInterval ||
	   m.recvP.FeedbackInterval != recvP.FeedbackInterval ||
	   m.recvP.FeedbackOnFlush != recvP.FeedbackOnFlush {
		if m.logURL != nCfg.logURL {
			// first we create the new logger. This can fail. So, don't
			// modify any global data yet.
			var newLogger *log.Logger
			if nCfg.logURL == "" {
				log.Warnf("logfile not set. Please use slottool to configure.")
				newLogger = log.NewR(log.WithTopic("MAIN"))
			} else {
				var lopts []log.Opt
				var err error
				lopts, err = log.ParseURL(nCfg.logURL, func(e error) {
					err = e
				})
				if err != nil {
					return err
				}
				newLogger = log.NewR(append(lopts, log.WithTopic("MAIN"))...)
				// The error callback passed to log.ParseURL() is called if
				// the Rotate object possibly needed in log.NewR fails to
				// be created. NewR historically does not return a value.
				// But it might need to create a Rotate object which can fail.
				// The corresponding error is passed to the callback above
				// which sets the err variable here.
				if err != nil {		// Rotate object creation failed
					return err
				}
			}

			// At this point we can't fail anymore. So, it's save to close
			// the old loggers.

			// We can't close m.lg right now. It might still be used by
			// cap.Receiver.
			if m.lg != nil {
				oldLogger := m.lg
				c := make(chan struct{}, 0)
				go func() {
					<- c
					log.Notice("Closing old logger")
					oldLogger.Close()
				}()
				recvP.CloseOnActivation = c
			}

			m.lg = newLogger
			recvP.Logger = newLogger.New(log.WithTopic("RCV"))
			m.mlg = newLogger.New(log.WithTopic("WRT"))

			m.logURL = nCfg.logURL
		}

		ret = nil
		nCfg.recvP = recvP
	}
	m.Reloadable = *nCfg

	m.lg.Infof("Reload Parameters:\n%v", nCfg)
	return ret
}

// Local Variables:
// tab-width: 4
// End:
