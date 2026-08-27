package capture

import (
	"errors"
	"slices"
	"strconv"
	"github.com/alecthomas/units"
	"github.com/tfoertsch123/log"
	"github.com/tfoertsch123/own-your-pg/slot"
	"github.com/tfoertsch123/own-your-pg/defaults"
	cap "github.com/tfoertsch123/own-your-pg/pglogreplsimple"
	"github.com/tfoertsch123/own-your-pg/pglogreplsimple/pgmask"
)

type Cfg struct {
	sl *slot.Slot

	logURL string
	lg *log.Logger
	mlg *log.Logger				// to be used in the writing part
	recvP *cap.Param
	maxSize int64
}

var ErrMissingConninfo error = errors.New("primary_conninfo not set")
var ErrInvalidConninfo error = errors.New("cannot parse primary_conninfo")
var ErrInvalidLimit error = errors.New("size_limit is invalid")

// readSettings returns true if a new Param package needs to be sent
// to the Receiver
func (m *Cfg) readSettings(update bool) (bool, error) {
	new := Cfg{recvP: &cap.Param{}}
	if x, err := m.sl.GetConfig("logfile", update); err != nil {
		return false, err
	} else if len(x) >= 1 {
		new.logURL = x[0]
	}

	x, _ := m.sl.GetConfig("primary_conninfo", false)
	if len(x) >= 1 {
		if x, err := pgmask.AppendOptionIfNotExists(
			x[0], "application_name", "OYPG-"+m.sl.Name(),
		); err != nil {
			return false, ErrInvalidConninfo
		} else {
			new.recvP.ConnInfo = x
		}
	} else {
		return false, ErrMissingConninfo
	}

	x, _ = m.sl.GetConfig("primary_slotname", false)
	if len(x) >= 1 {
		new.recvP.SlotName = x[0]
	} else {
		new.recvP.SlotName = m.sl.Name()
	}

	x, _ = m.sl.GetConfig("ignore_missing_identity", false)
	new.recvP.IgnoreMissingIdentity = x

	x, _ = m.sl.GetConfig("size_limit", false)
	if len(x) == 0 {
		x = append(x, defaults.M2SizeLimit)
	}
	if x1, err := strconv.ParseUint(x[0], 10, 64); err == nil {
		new.maxSize = int64(x1)
	} else if x1, err := units.ParseStrictBytes(x[0]); err == nil {
		new.maxSize = int64(x1)
	} else {
		return false, ErrInvalidLimit
	}

	if m.logURL != new.logURL {
		if new.logURL == "" {
			log.Warnf("logfile not set. Please use slottool to configure.")
			new.lg = log.NewR(log.WithTopic("MAIN"))
		} else {
			var lopts []log.Opt
			var err error
			lopts, err = log.ParseURL(new.logURL, func(e error) {
				err = e
			})
			if err != nil {
				return false, err
			}
			new.lg = log.NewR(append(lopts, log.WithTopic("MAIN"))...)
			if err != nil {		// Rotate object creation failed
				return false, err
			}
		}
		new.recvP.Logger = new.lg.New(log.WithTopic("RCV"))
		new.mlg = new.lg.New(log.WithTopic("WRT"))
	}

	// activate the changes
	if m.logURL != new.logURL ||
	   m.recvP.ConnInfo != new.recvP.ConnInfo ||
	   m.recvP.SlotName != new.recvP.SlotName ||
	   !slices.Equal(
		   m.recvP.IgnoreMissingIdentity, new.recvP.IgnoreMissingIdentity,
	   ) {
		if m.logURL != new.logURL {
			// we can't close m.lg right now. It might still be used by
			// cap.Receiver.
			if m.lg != nil {
				old_logger := m.lg
				c := make(chan struct{}, 0)
				go func() {
					<- c
					log.Notice("Closing old logger")
					old_logger.Close()
				}()
				new.recvP.CloseOnActivation = c
			}
			if m.mlg != nil {
				m.mlg.Close()
			}
			m.mlg = new.mlg
			m.lg = new.lg
			m.logURL = new.logURL
		} else {
			// we need to send a config package but want to keep the old
			// logger
			new.recvP.Logger = m.recvP.Logger
		}

		m.recvP = new.recvP
		m.maxSize = new.maxSize
		return true, nil
	} else {
		// only local changes, nothing related to cap.Receiver
		m.maxSize = new.maxSize
		return false, nil
	}
}

// Local Variables:
// tab-width: 4
// End:
