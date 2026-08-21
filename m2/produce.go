package m2

import (
	"time"
	"iter"
	"errors"
	"regexp"
	"context"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"

	"github.com/tfoertsch123/own-your-pg/msg"
	"github.com/tfoertsch123/own-your-pg/lsn"
	"github.com/tfoertsch123/own-your-pg/defaults"
)

// MsgItem is a type that is either a *pglogrepl.PrimaryKeepaliveMessage or
// a *pglogrepl.XLogData
type MsgItem interface {}

type Next int8
const (
	Connect Next = iota
	Recv
	Reinit
	Stop
)

func (nxt Next) String() string {
	return []string{"Connect", "Recv", "Reinit", "Stop"}[nxt]
}

// errPause is similar to checkStop both check for a pending shutdown
// or reload. In addition to that, errPause waits for defaults.M2ErrPause
// and returns Connect if the timer hits. errPause is supposed to be used
// by ConnInit.
func (m *Mon) errPause(f string, p ...interface{}) Next {
	m.relg.Errorf(f, p...)
	if m.conn != nil {
		m.conn.Close(context.Background())
		m.conn = nil
	}
	select {
	case <- time.NewTimer(defaults.M2ErrPause).C:
		// As of go 1.23, no special handling is needed to make this
		// timer subject to garbage collection. So, no memleak here.
		return Connect
	case <- m.shutdown_ctx.Done():
		if cause := context.Cause(m.shutdown_ctx); cause != nil {
			m.lg.Infof("%v", cause)
		}
		return Stop
	case <- m.reload:
		return Reinit
	}
}

// checkStop is the equivalent of errPause only to be called by RecvOne
// instead of ConnInit. It checks for pending shutdown and reload events.
// If none are present, it just logs the error.
func (m *Mon) checkStop(x Next) Next {
	select {
	case <- m.shutdown_ctx.Done():
		if cause := context.Cause(m.shutdown_ctx); cause != nil {
			m.lg.Errorf("%v", cause)
		}
		return Stop
	case <- m.reload:
		return Reinit
	default:
	}
	return x
}

// ConnInit() tries to connect to the DB and perform all the actions
// needed to start receiving data. On any error it calls errPause()
// and returns its return value.
// That means if the pause timer hits, Connect is returned. If the shutdown
// context was canceled while waiting, Stop is returned. Or, if a reload
// signal was caught, Reinit is returned.
func (m *Mon) ConnInit() Next {
	if m.conn != nil {
		m.conn.Close(context.Background())
		m.conn = nil
	}

	conn, err := pgconn.Connect(m.shutdown_ctx, m.ci)
	if err != nil {
		return m.errPause("PG Conn: %v", err)
	}
	m.conn = conn

	// identify system is at the moment just for fun
	sysident, err := pglogrepl.IdentifySystem(m.shutdown_ctx, conn)
	if err != nil {
		return m.errPause("IdentifySystem: %v", err)
	}

	m.relg.Infof(
		"SystemID: %v, Timeline: %v, WALPos: %v, DBName: %v",
		sysident.SystemID, sysident.Timeline, sysident.XLogPos,
		sysident.DBName,
	)

	// check if the slot exists and has the expected plugin
	res, err := conn.Exec(
		m.shutdown_ctx,
		`SELECT plugin, confirmed_flush_lsn FROM pg_replication_slots `+
			`WHERE slot_name=`+msg.QliteralString(m.sn),
	).ReadAll()
	if err != nil {
		return m.errPause("Reading replication slot: %v", err)
	}

	if len(res[0].Rows) == 0 {
		return m.errPause("Replication slot %v not found", m.sn)
	}

	if len(res[0].Rows) > 1 {
		return m.errPause("Multiple Replication slots: %v", m.sn)
	}

	plugin := string(res[0].Rows[0][0])
	confirmedFlushLSN := string(res[0].Rows[0][1])
	m.relg.Infof("Plugin of slot %v: %v", m.sn, plugin)
	m.relg.Infof("Confirmed Flush LSN: %v", confirmedFlushLSN)

	plugin_opts := []string{}
	if opt_, ok := defaults.M2ExpectedPlugins[plugin]; !ok {
		m.shutdown_trg(nil)
		return m.errPause("plugin not acceptable: %v", plugin)
	} else {
		plugin_opts = opt_
	}

	// check the lsn. If the slot's confirmedFlushLSN is ahead of our
	// LSN, refuse connection. If our LSN is 0, that's a special case,
	// when we connect for the very first time.
	slotlsn, err := lsn.ParseLSN(confirmedFlushLSN)
	if err != nil {
		m.shutdown_trg(nil)
		return m.errPause("LSN %v: %v", confirmedFlushLSN, err)
	}

	ourlsn, _ := m.sl.GetLSN() // no error possible here
	if ourlsn != lsn.LSN(0) && ourlsn < slotlsn {
		m.shutdown_trg(nil)
		return m.errPause(
			"Confirmed Flush LSN %v is ahead of our LSN %v", slotlsn, ourlsn,
		)
	}
	if ourlsn == lsn.LSN(0) {
		ourlsn = slotlsn
		m.sl.SetLSN(ourlsn, true)
	}
		
	err = pglogrepl.StartReplication(
		m.shutdown_ctx,
		conn,
		m.sn,
		pglogrepl.LSN(ourlsn),
		pglogrepl.StartReplicationOptions{PluginArgs: plugin_opts},
	)
	if err != nil {
		return m.errPause("StartReplication: %v", err)
	}
	m.recvStat.fpos = ourlsn
	m.recvStat.wpos = ourlsn

	m.scheduleFeedback()

	return Recv
}

func (m *Mon) scheduleFeedback() {
	m.nextFeedback = time.Now().Add(defaults.M2FeedbackInterval)
}

func (m *Mon) SendFeedback() error {
	if m.prevStat != m.recvStat {
		m.relg.Infof("Sending Standby status: write: %v, flush: %v",
			m.recvStat.wpos, m.recvStat.fpos)
	}
	// the context parameter here is currently ignored by
	// pglogrepl.SendStandbyStatusUpdate()
	err := pglogrepl.SendStandbyStatusUpdate(
		m.shutdown_ctx,
		m.conn,
		pglogrepl.StandbyStatusUpdate{
			WALWritePosition: pglogrepl.LSN(m.recvStat.wpos),
			WALFlushPosition: pglogrepl.LSN(m.recvStat.fpos),
			WALApplyPosition: pglogrepl.LSN(m.recvStat.fpos),
		},
	)
	if err != nil {
		return err
	}

	m.prevStat = m.recvStat
	m.scheduleFeedback()
	return nil
}

var ms_re = regexp.MustCompile(
	`\Ano tuple identifier for (?:UPDATE|DELETE) in table (".+?"\.".+")\z`,
)
var ErrNoticeMessage error = errors.New("PG Notice")
func (m *Mon) processNotice(msg *pgproto3.NoticeResponse) Next {
	// This happens besides other things when there is no
	// replica identity.
	// We pass the option client_min_messages=warning as
	// connection parameter. So, we should not get anything
	// higher than that. If we still do, we ignore them.
	//
	// We want to:
	// - ignore any message with NOTICE and higher
	// - STOP execution on any ERROR. There is a very limited number
	//   of such messages in the source code and all of them are
	//   real problem.
	// - WARNING where the message starts with "no tuple identifier"
	//   STOP unless the table is configured to be ignored
	// - ignore any other WARNING
	//
	// Search for "no tuple identifier for" in
	// https://github.com/eulerto/wal2json/blob/master/wal2json.c

	switch msg.SeverityUnlocalized {
	case "ERROR":
		m.relg.Errorf(
			"%v: ERROR: %v WHERE: %v", ErrNoticeMessage,
			msg.Message, msg.Where,
		)
		return Connect
	case "WARNING":
		if tb := ms_re.FindStringSubmatch(msg.Message); tb != nil {
			// relying on text matching is quite unfortunate.
			if m.ignMiss == nil {
				m.ignMiss = make(map[string]struct{}, 10)

				// no config reload here. We only re-read the slot
				// on SIGHUP.
				cfg, _ := m.sl.GetConfig("ignore_missing_identity")
				for _, x := range cfg {
					m.ignMiss[x] = struct{}{}
				}
			}

			if _, ok := m.ignMiss[tb[1]]; ok {
				m.relg.Warnf(
					`ignoring missing replica identity for %s`, tb[1],
				)
			} else {
				m.relg.Errorf(
					"%v: %s: %v WHERE: %v", ErrNoticeMessage,
					msg.SeverityUnlocalized, msg.Message, msg.Where,
				)
				return Stop
			}
		}
		m.relg.Errorf(
			"%v: %s:%v WHERE:%v", ErrNoticeMessage,
			msg.SeverityUnlocalized, msg.Message, msg.Where,
		)
	default:
		m.relg.Errorf(
			"%v: %s:%v WHERE:%v", ErrNoticeMessage, msg.SeverityUnlocalized,
			msg.Message, msg.Where,
		)
	}
	return Recv
}

func (m *Mon) processCopyData(
	msg *pgproto3.CopyData,
	yield func(MsgItem) bool,
) Next {
	// Right after connecting to the DB, the DB sends a
	// PrimaryKeepaliveMessage with the slot's confirmed_flush_lsn
	// as payload.
	switch msg.Data[0] {
	case pglogrepl.PrimaryKeepaliveMessageByteID:
		pkm, err := pglogrepl.ParsePrimaryKeepaliveMessage(msg.Data[1:])
		if err != nil {
			m.relg.Errorf("PKAL %v failed to parse: %v", msg.Data[1:], err)
			return Connect
		}
		// m.relg.Debg4f("PKAL: %#v", pkm)
		if pkm.ReplyRequested {
			m.relg.Debg3("PKAL: immediate reply requested")
			m.SendFeedback()
		}
		if !yield(&pkm) {
			return Stop
		}

	case pglogrepl.XLogDataByteID:
		xld, err := pglogrepl.ParseXLogData(msg.Data[1:])
		if err != nil {
			m.relg.Errorf("ParseXLogData failed: %v", err)
			return Connect
		}

		// m.relg.Debg3f("wal2json(WALStart=%v): %s",
		// 	xld.WALStart, string(xld.WALData))
		if !yield(&xld) {
			return Stop
		}
	default:
		m.relg.Errorf("Unexpected CopyData message type: <%v> -- ignored",
			msg.Data[0])
	}
	return Recv
}

func (m *Mon) RecvOne(yield func(MsgItem) bool) Next {
	// check for pending shutdown or reload
	if nxt := m.checkStop(Recv); nxt != Recv {
		return nxt
	}

	if time.Now().After(m.nextFeedback) {
		if err := m.SendFeedback(); err != nil {
			m.relg.Errorf("SendStandbyStatusUpdate failed: %v", err)
			// in case of an error, the safest thing to do is to reconnect.
			return Connect
		}
		return Recv
	}

	// try to read a message
	ctx, cancel := context.WithDeadline(m.shutdown_ctx, m.nextFeedback)
	rawMsg, err := m.conn.ReceiveMessage(ctx)
	cancel()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return Recv
		}
		if !errors.Is(err, context.Canceled) {
			m.relg.Errorf("ReceiveMessage failed: %v", err)
			// in case of an error, the safest thing to do is to reconnect.
			return Connect
		}
		// this will call the next RecvOne and the checkStop call above
		// will then return stop after reporting the cause.
		return Recv
	}

	// analyze the message
	switch msg := rawMsg.(type) {
	case *pgproto3.ErrorResponse:
		m.relg.Errorf("PG error: %v", msg)
		return Connect
	case *pgproto3.NoticeResponse:
		return m.processNotice(msg)
	case *pgproto3.CopyData:
		return m.processCopyData(msg, yield)
	default:
		m.relg.Errorf("ReceiveMessage: got unexpected message of type %t",
			rawMsg)
		return Recv
	}
}

func (m *Mon) Produce() iter.Seq[MsgItem] {
	if _, err := m.readSettings(false); err != nil {
		m.lg.Panicf("%v", err)
	}

	return func(yield func(MsgItem) bool) {
		nxt := Connect
		for {
			switch nxt {
			case Stop:
				if m.conn != nil {
					m.conn.Close(context.Background())
					m.conn = nil
				}
				return
			case Connect:
				nxt = m.ConnInit()
			case Recv:
				nxt = m.RecvOne(yield)
			case Reinit:
				if nxt_, err := m.readSettings(true); err != nil {
					m.lg.Errorf("%v", err)
				} else {
					nxt = nxt_
				}
			}
		}
	}
}
 
// Local Variables:
// tab-width: 4
// End:
