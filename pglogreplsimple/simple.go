package pglogreplsimple

import (
	"time"
	"errors"
	"regexp"
	"context"
	"strings"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"

	"github.com/tfoertsch123/own-your-pg/pglogreplsimple/pgmask"
)

func (r *Receiver) Shutdown(err error) {
	r.shutdownTrg(err)
}

func (r *Receiver) Plugin() string {
	return r.plugin
}

func (r *Receiver) Params() Param {
	return r.p
}

func (r *Receiver) Err() error {
	return r.lastErr
}

func (r *Receiver) State() Next {
	return r.state
}

// errPause is similar to checkStop both check for a pending shutdown
// or reload. In addition to that, errPause waits for defaults.M2ErrPause
// and returns Connect if the timer hits. errPause is supposed to be used
// by ConnInit.
func (r *Receiver) errPause(
	ctx context.Context,
	cancel context.CancelCauseFunc,
	f string, p ...interface{},
) Next {
	r.lg.Errorf(f, p...)
	if r.conn != nil {
		r.conn.Close(context.Background())
		r.conn = nil
	}
	tmout := 5*time.Second
	timer := time.AfterFunc(tmout, func() {
		cancel(context.DeadlineExceeded)
	})
	defer timer.Stop()

	select {
	case <- ctx.Done():
		return Connect
	case <- r.shutdownCtx.Done():
		if cause := context.Cause(r.shutdownCtx); cause != nil {
			r.lastErr = cause
		}
		return Stop
	}
}

// checkStop is the equivalent of errPause only to be called by RecvOne
// instead of ConnInit. It checks for pending shutdown and reload events.
// If none are present, it just logs the error.
func (r *Receiver) checkStop() Next {
	select {
	case <- r.shutdownCtx.Done():
		if cause := context.Cause(r.shutdownCtx); cause != nil {
			r.lastErr = cause
		}
		return Stop
	default:
	}
	return Recv
}

var ErrPlugin error = errors.New("Wrong decoding plugin")
var ErrConfirmedFlushLSN error = errors.New(
	"ConfirmedFlushLSN is in the future",
)

// ConnInit() tries to connect to the DB and perform all the actions
// needed to start receiving data. On any error it calls errPause()
// and returns its return value.
// That means if the pause timer hits, Connect is returned. If the shutdown
// context was canceled while waiting, Stop is returned. Or, if a reload
// signal was caught, Reinit is returned.
func (r *Receiver) ConnInit() Next {
	if r.conn != nil {
		r.conn.Close(context.Background())
		r.conn = nil
	}

	// try to read a message
	ctx, cancel := context.WithCancelCause(r.shutdownCtx)
	defer cancel(nil)
	defer r.setCancelCurrent(nil)

	r.setCancelCurrent(cancel)

	ci, err := pgmask.AppendOptionOverride(
		r.p.ConnInfo,
		"replication", "database",
		// wal2json sends a warning if a table without identity
		// has been updated. Search for "no tuple identifier for" in
		// https://github.com/eulerto/wal2json/blob/master/wal2json.c
		"options", `-cclient_min_messages=warning`,
	)
	if err != nil {
		return r.errPause(ctx, cancel, "primary_conninfo: %v", err)
	}
	
	conn, err := pgconn.Connect(ctx, ci)
	if err != nil {
		return r.errPause(ctx, cancel, "PG Conn: %v", err)
	}
	r.conn = conn

	// identify system is at the moment just for fun
	sysident, err := pglogrepl.IdentifySystem(ctx, conn)
	if err != nil {
		return r.errPause(ctx, cancel, "IdentifySystem: %v", err)
	}

	r.lg.Infof(
		"SystemID: %v, Timeline: %v, WALPos: %v, DBName: %v",
		sysident.SystemID, sysident.Timeline, sysident.XLogPos,
		sysident.DBName,
	)

	// check if the slot exists and has the expected plugin
	res, err := conn.Exec(
		ctx,
		`SELECT plugin, confirmed_flush_lsn FROM pg_replication_slots `+
			`WHERE slot_name='`+strings.ReplaceAll(r.p.SlotName, `'`, `''`)+`'`,
	).ReadAll()
	if err != nil {
		return r.errPause(ctx, cancel, "Reading replication slot: %v", err)
	}

	if len(res[0].Rows) == 0 {
		return r.errPause(ctx, cancel,
			"Replication slot %v not found", r.p.SlotName)
	}

	if len(res[0].Rows) > 1 {
		return r.errPause(ctx, cancel,
			"Multiple Replication slots: %v", r.p.SlotName)
	}

	plugin := string(res[0].Rows[0][0])
	confirmedFlushLSN := string(res[0].Rows[0][1])
	r.lg.Infof("Plugin of slot %v: %v", r.p.SlotName, plugin)
	r.lg.Infof("Confirmed Flush LSN: %v", confirmedFlushLSN)

	plugin_opts := []string{}
	if opt_, ok := r.acceptedPlugins[plugin]; !ok {
		r.shutdownTrg(ErrPlugin)
		return r.errPause(ctx, cancel, "plugin not acceptable: %v", plugin)
	} else {
		r.plugin = plugin
		plugin_opts = opt_
	}

	// check the lsn. If the slot's confirmedFlushLSN is ahead of our
	// LSN, refuse connection. If our LSN is 0, that's a special case,
	// when we connect for the very first time.
	slotlsn, err := pglogrepl.ParseLSN(confirmedFlushLSN)
	if err != nil {
		r.shutdownTrg(err)
		return r.errPause(ctx, cancel, "LSN %v: %v", confirmedFlushLSN, err)
	}

	ourlsn := r.getStartLSN()
	if ourlsn == pglogrepl.LSN(0) {
		ourlsn = slotlsn
	}
	if ourlsn < slotlsn {
		r.shutdownTrg(ErrConfirmedFlushLSN)
		return r.errPause(ctx, cancel,
			"Confirmed Flush LSN %v is ahead of our LSN %v", slotlsn, ourlsn,
		)
	}
		
	err = pglogrepl.StartReplication(
		ctx,
		conn,
		r.p.SlotName,
		ourlsn,
		pglogrepl.StartReplicationOptions{PluginArgs: plugin_opts},
	)
	if err != nil {
		return r.errPause(ctx, cancel, "StartReplication: %v", err)
	}
	r.recvStat.fpos = ourlsn
	r.recvStat.wpos = ourlsn
	r.recvStat.rpos = ourlsn

	r.scheduleFeedback()
	r.lg.Debg3f("Successfully connected to DB. First feedback scheduled in: %v",
		time.Until(r.nextFeedback),
	)

	return Recv
}

func (r *Receiver) scheduleFeedback() {
	r.nextFeedback = time.Now().Add(r.p.FeedbackInterval)
}

func (r *Receiver) SendFeedback() error {
	if r.prevStat != r.recvStat {
		r.lg.Debugf("Sending feedback: write: %v, flush: %v, replay: %v",
			r.recvStat.wpos, r.recvStat.fpos, r.recvStat.rpos)
	} else {
		r.lg.Debg3f("Sending feedback anyway: write: %v, flush: %v, replay: %v",
			r.recvStat.wpos, r.recvStat.fpos, r.recvStat.rpos)
	}
	// the context parameter here is currently ignored by
	// pglogrepl.SendStandbyStatusUpdate()
	err := pglogrepl.SendStandbyStatusUpdate(
		r.shutdownCtx,
		r.conn,
		pglogrepl.StandbyStatusUpdate{
			WALWritePosition: r.recvStat.wpos,
			WALFlushPosition: r.recvStat.fpos,
			WALApplyPosition: r.recvStat.rpos,
		},
	)
	if err != nil {
		return err
	}

	r.prevStat = r.recvStat
	r.scheduleFeedback()
	return nil
}

var ms_re = regexp.MustCompile(
	`\Ano tuple identifier for (?:UPDATE|DELETE) in table (".+?"\.".+")\z`,
)
var ErrNoticeMessage error = errors.New("PG Notice")
func (r *Receiver) processNotice(msg *pgproto3.NoticeResponse) Next {
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
		r.lg.Errorf(
			"%v: ERROR: %v WHERE: %v", ErrNoticeMessage,
			msg.Message, msg.Where,
		)
		return Connect
	case "WARNING":
		if tb := ms_re.FindStringSubmatch(msg.Message); tb != nil {
			// relying on text matching is quite unfortunate.
			if r.ignMiss == nil {
				r.ignMiss = make(map[string]struct{}, 10)

				for _, x := range r.p.IgnoreMissingIdentity {
					r.ignMiss[x] = struct{}{}
				}
			}

			if _, ok := r.ignMiss[tb[1]]; ok {
				r.lg.Warnf(
					`ignoring missing replica identity for %s`, tb[1],
				)
			} else {
				r.lg.Errorf(
					"%v: %s: %v WHERE: %v", ErrNoticeMessage,
					msg.SeverityUnlocalized, msg.Message, msg.Where,
				)
				r.lastErr = pgconn.ErrorResponseToPgError(
					(*pgproto3.ErrorResponse)(msg),
				)
				return Stop
			}
		}
		r.lg.Errorf(
			"%v: %s:%v WHERE:%v", ErrNoticeMessage,
			msg.SeverityUnlocalized, msg.Message, msg.Where,
		)
	default:
		r.lg.Errorf(
			"%v: %s:%v WHERE:%v", ErrNoticeMessage, msg.SeverityUnlocalized,
			msg.Message, msg.Where,
		)
	}
	return Recv
}

func (r *Receiver) processCopyData(
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
			r.lg.Errorf("PKAL %v failed to parse: %v", msg.Data[1:], err)
			return Connect
		}
		// m.relg.Debg4f("PKAL: %#v", pkm)
		if pkm.ReplyRequested {
			r.lg.Debg3("PKAL: immediate reply requested")
			r.SendFeedback()
		}
		if !yield(&pkm) {
			return Break
		}

	case pglogrepl.XLogDataByteID:
		xld, err := pglogrepl.ParseXLogData(msg.Data[1:])
		if err != nil {
			r.lg.Errorf("ParseXLogData failed: %v", err)
			return Connect
		}
		if !yield(&xld) {
			return Break
		}
	default:
		r.lg.Errorf("Unexpected CopyData message type: <%v> -- ignored",
			msg.Data[0])
	}
	return Recv
}

func (r *Receiver) _recvOneMsg(
	tmout time.Duration,
) (pgproto3.BackendMessage, error) {
	ctx, cancel := context.WithCancelCause(r.shutdownCtx)
	timer := time.AfterFunc(tmout, func() {
		cancel(context.DeadlineExceeded)
	})
	defer r.setCancelCurrent(nil)
	defer cancel(nil)
	defer timer.Stop()

	r.setCancelCurrent(cancel)
	return r.conn.ReceiveMessage(ctx)
}

func (r *Receiver) RecvOne(yield func(MsgItem) bool) Next {
	// check for pending shutdown or reload
	if nxt := r.checkStop(); nxt != Recv {
		return nxt
	}

	timeLeftUntilFeedback := time.Until(r.nextFeedback)
	if timeLeftUntilFeedback < 5*time.Millisecond {
		if err := r.SendFeedback(); err != nil {
			r.lg.Errorf("SendStandbyStatusUpdate failed: %v", err)
			// in case of an error, the safest thing to do is to reconnect.
			return Connect
		}
		return Recv
	}

	rawMsg, err := r._recvOneMsg(timeLeftUntilFeedback)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// this will call the next RecvOne and the checkStop call above
			// will then return stop after reporting the cause.
			return Recv
		}

		// otherwise, the safest thing to do is to reconnect.
		r.lg.Errorf("ReceiveMessage failed: %v", err)
		return Connect
	}

	// analyze the message
	switch msg := rawMsg.(type) {
	case *pgproto3.ErrorResponse:
		r.lg.Errorf("PG error: %v", msg)
		return Connect
	case *pgproto3.NoticeResponse:
		return r.processNotice(msg)
	case *pgproto3.CopyData:
		return r.processCopyData(msg, yield)
	default:
		r.lg.Errorf("ReceiveMessage: got unexpected message of type %T",
			rawMsg)
		return Recv
	}
}
 
// Local Variables:
// tab-width: 4
// End:
