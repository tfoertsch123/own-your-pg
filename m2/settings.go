package m2

import (
	"errors"
	"strconv"
	"github.com/alecthomas/units"
	"github.com/tfoertsch123/log"
	"github.com/tfoertsch123/own-your-pg/pgmask"
	"github.com/tfoertsch123/own-your-pg/defaults"
)

func (m *Mon) settingChanged(what string, update bool) (bool, string, error) {
	if update {
		// We are about to re-read the slot config. Here we simply invalidate
		// the accepted missing identity cache. It will be rebuilt as soon
		// as the first missing identity is found.
		m.ignMiss = nil
	}
	new, err := m.sl.GetConfig(what, update)
	if err != nil {
		return false, "", err
	}
	old, oldExists := m.settings[what]

	if new == nil && !oldExists {
		return false, "", nil	// neither exists
	}

	if new != nil && oldExists && new[0] == old {
		return false, new[0], nil	// no change
	}

	return true, new[0], nil
}

var ErrMissingConninfo error = errors.New("primary_conninfo not set")
var ErrInvalidConninfo error = errors.New("cannot parse primary_conninfo")
var ErrInvalidLimit error = errors.New("size_limit is invalid")
func (m *Mon) readSettings(update bool) (Next, error) {
	nxt := Recv
	changed, new, err := m.settingChanged("logfile", update)
	if err != nil {
		return Recv, err
	}

	if changed || m.lg == nil {
		m.settings["logfile"] = new
		if m.lg != nil {
			m.lg.Close()
		}
		if new == "" {
			log.Warnf("logfile not set. Please use slottool to configure.")
			m.lg = log.NewC(log.WithTopic("MAIN"))
		} else {
			var lopts []log.Opt
			lopts, err = log.ParseURL(new, func(e error) {
				err = e
			})
			if err != nil {
				return Recv, err
			}
			m.lg = log.NewC(append(lopts, log.WithTopic("MAIN"))...)
			if err != nil {		// Rotate object creation failed
				return Recv, err
			}
		}
		m.relg = m.lg.New(log.WithTopic("RECV"))
		m.mlg = m.lg.New(log.WithTopic("CAPT"))
	}

	// since we are not re-reading the slot, no error can occur
	changed, new, _ = m.settingChanged("primary_conninfo", false)
	if changed {
		m.settings["primary_conninfo"] = new
		if new, err = pgmask.AppendOptionOverride(
			new,
			"replication", "database",
			// wal2json sends a warning if a table without identity
			// has been updated. Search for "no tuple identifier for" in
			// https://github.com/eulerto/wal2json/blob/master/wal2json.c
			"options", `-cclient_min_messages=warning`,
		); err != nil {
			return Recv, ErrInvalidConninfo
		}
		new, _ = pgmask.AppendOptionIfNotExists(
			new, "application_name", "OYPG-"+m.sl.Name(),
		)

		m.ci = new
		nxt = Connect
	} else if new == "" {
		return Recv, ErrMissingConninfo // can only happen at init time
	}

	changed, new, _ = m.settingChanged("primary_slotname", false)
	if changed || new == "" {
		if new == "" {
			delete(m.settings, "primary_slotname")
			new = m.sl.Name()
		} else {
			m.settings["primary_slotname"] = new
		}
		if new != m.sn {
			m.sn = new
			nxt = Connect
		}
	}

	changed, new, _ = m.settingChanged("size_limit", false)
	if changed || new == "" && m.maxSize == 0 {
		if new == "" {
			new = defaults.M2SizeLimit
		}
		if x, err := strconv.ParseUint(new, 10, 64); err == nil {
			m.maxSize = int64(x)
			m.settings["size_limit"] = new
		} else if x, err := units.ParseStrictBytes(new); err == nil {
			m.maxSize = int64(x)
			m.settings["size_limit"] = new
		} else {
			return Recv, ErrInvalidLimit
		}
	}

	return nxt, nil
}

// Local Variables:
// tab-width: 4
// End:
