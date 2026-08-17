package m2

import (
	"time"

	"context"
	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	// "github.com/jackc/pgx/v5/pgproto3"

	// "github.com/tfoertsch123/log"
	// "github.com/tfoertsch123/own-your-pg/slot"
	"github.com/tfoertsch123/own-your-pg/msg"
	"github.com/tfoertsch123/own-your-pg/lsn"
	"github.com/tfoertsch123/own-your-pg/defaults"
)


func (m *Mon) errPause(f string, p ...interface{}) bool {
	m.relg.Errorf(f, p...)
	if m.conn != nil {
		m.conn.Close(context.Background())
		m.conn = nil
	}
	select {
	case <- time.NewTimer(defaults.M2ErrPause).C:
		return false
	case <- m.shutdown:
		return true
	}
}

func (m *Mon) ConnInit() bool {
	for {
		ctx := context.Background()
		conn, err := pgconn.Connect(ctx, m.ci)
		if err != nil {
			if doStop := m.errPause("PG Conn: %v", err); doStop {
				return doStop
			}
			continue
		}
		m.conn = conn

		// identify system is at the moment just for fun
		sysident, err := pglogrepl.IdentifySystem(ctx, conn)
		if err != nil {
			if doStop := m.errPause("IdentifySystem: %v", err); doStop {
				return doStop
			}
			continue
		}

		m.relg.Infof(
			"SystemID: %v, Timeline: %v, WALPos: %v, DBName: %v",
			sysident.SystemID, sysident.Timeline, sysident.XLogPos,
			sysident.DBName,
		)

		// check if the slot exists and has the expected plugin
		res, err := conn.Exec(
			ctx,
			`SELECT plugin, confirmed_flush_lsn FROM pg_replication_slots `+
				`WHERE slot_name=`+msg.QliteralString(m.sn),
		).ReadAll()
		if err != nil {
			if doStop := m.errPause("Reading replication slot: %v", err);
			   doStop {
				return doStop
			}
			continue
		}
		if len(res[0].Rows) == 0 {
			if doStop := m.errPause("Replication slot %v not found", m.sn);
			   doStop {
				return doStop
			}
			continue
		}
		if len(res[0].Rows) > 1 {
			if doStop := m.errPause("Multiple Replication slots: %v", m.sn);
			   doStop {
				return doStop
			}
			continue
		}
		plugin := string(res[0].Rows[0][0])
		confirmedFlushLSN := string(res[0].Rows[0][1])
		m.relg.Infof("Plugin of slot %v: %v", m.sn, plugin)
		m.relg.Infof("Confirmed Flush LSN: %v", confirmedFlushLSN)

		if exp, ok := defaults.M2ExpectedPlugins[plugin]; !(ok && exp) {
			close(m.shutdown)
			if doStop := m.errPause("plugin not acceptable: %v", plugin);
			   doStop {
				return doStop
			}
			continue
		}

		// check the lsn. If the slot's confirmedFlushLSN is ahead of our
		// LSN, refuse connection. If our LSN is 0, that's a special case,
		// we connect for the very first time.
		slotlsn, err := lsn.ParseLSN(confirmedFlushLSN)
		if err != nil {
			close(m.shutdown)
			if doStop := m.errPause("LSN %v: %v", confirmedFlushLSN, err);
			   doStop {
				return doStop
			}
			continue
		}
		ourlsn, _ := m.sl.GetLSN() // no error possible here
		if ourlsn != lsn.LSN(0) && ourlsn < slotlsn {
			close(m.shutdown)
			if doStop := m.errPause(
				"Confirmed Flush LSN %v is ahead of our LSN %v",
				slotlsn, ourlsn); doStop {
				return doStop
			}
			continue
		}
		if ourlsn == lsn.LSN(0) {
			ourlsn = slotlsn
			m.sl.SetLSN(ourlsn, true)
		}

		// set client_min_messages
		
		break
	}

	return false
}

// func (m *Mon) Produce() {
// }

// Local Variables:
// tab-width: 4
// End:
