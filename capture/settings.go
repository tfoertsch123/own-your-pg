package capture

import (
	"strconv"
	"strings"
	"github.com/alecthomas/units"
	"github.com/tfoertsch123/log"
	"github.com/tfoertsch123/own-your-pg/defaults"
	cap "github.com/tfoertsch123/pglogreplsimple"
	"github.com/tfoertsch123/pgconnstr"
)


// readSettings reads the slot and returns a new *reloadable parameter package
// or an error.
// readSettings must not access anything other than s.sl, not even a logger.
// It can be called by a separate go routine.
func (s *session) readSettings(update bool) (*reloadable, error) {
	nCfg := &reloadable{recvP: &cap.Param{}}
	if x, err := s.sl.GetConfig("logfile", update); err != nil {
		return nCfg, err
	} else if len(x) >= 1 {
		nCfg.logURL = x[0]
	}

	x, _ := s.sl.GetConfig("primary_conninfo", false)
	if len(x) >= 1 {
		if ci, err := pgconnstr.Parse(x[0]); err != nil {
			return nCfg, ErrInvalidConninfo
		} else {
			if _, exists := ci["application_name"]; ! exists {
				ci["application_name"] = "OYPG-"+s.sl.Name()
			}
			ci["options"] = "--client_min_messages=warning " +
				"--log_min_duration_statement=0"
			nCfg.recvP.ConnInfo = ci.URL()
		}
	} else {
		return nCfg, ErrMissingConninfo
	}

	x, _ = s.sl.GetConfig("primary_slotname", false)
	if len(x) >= 1 {
		nCfg.recvP.SlotName = x[0]
	} else {
		nCfg.recvP.SlotName = s.sl.Name()
	}

	x, _ = s.sl.GetConfig("ignore_missing_identity", false)
	nCfg.ignMissId = x

	x, _ = s.sl.GetConfig("size_limit", false)
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
	x, _ = s.sl.GetConfig("synchronous", false)
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
func (s *session) prepareReload(nCfg *reloadable, err error) *cap.Param {
	nCfg.recvP.OnActivation = func(p *cap.Param) error {
		if err != nil {
			return err
		}
		return s.applySettings(nCfg, p)
	}
	return nCfg.recvP
}

// this is called by the receiver (cap) object in the main go routine. It
// can access all the fields in s. The passed in recvP is not the same as
// nCfg.revcP. Logically it is but it has been copied somewhere along the
// way. So, changes must be made there.
func (s *session) applySettings(nCfg *reloadable, recvP *cap.Param) error {
	ret := cap.ErrNoChange
	if s.logURL != nCfg.logURL ||
	   s.recvP.ConnInfo != recvP.ConnInfo ||
	   s.recvP.SlotName != recvP.SlotName ||
	   s.recvP.ErrorRetryInterval != recvP.ErrorRetryInterval ||
	   s.recvP.FeedbackInterval != recvP.FeedbackInterval ||
	   s.recvP.FeedbackOnFlush != recvP.FeedbackOnFlush {
		if s.logURL != nCfg.logURL {
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

			// We can't close s.lg right now. It might still be used by
			// cap.Receiver.
			if s.lg != nil {
				oldLogger := s.lg
				c := make(chan struct{}, 0)
				go func() {
					<- c
					log.Notice("Closing old logger")
					oldLogger.Close()
				}()
				recvP.CloseOnActivation = c
			}

			s.lg = newLogger
			recvP.Logger = newLogger.New(log.WithTopic("RCV"))
			s.mlg = newLogger.New(log.WithTopic("WRT"))

			s.logURL = nCfg.logURL
		}

		ret = nil
		nCfg.recvP = recvP
	}
	s.reloadable = *nCfg

	s.lg.Infof("Reload Parameters:\n%v", nCfg)
	return ret
}

// Local Variables:
// tab-width: 4
// End:
