package capture

import (
	"encoding/json"
	"iter"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgproto3"

	mylsn "github.com/tfoertsch123/own-your-pg/lsn"
	cap "github.com/tfoertsch123/pglogreplsimple"
)

func (cfg *Cfg) consume(
	it iter.Seq[cap.MsgItem],
	ack func(pglogrepl.LSN, ...pglogrepl.LSN),
) {
	// We process primary keepalive (PKAL) and XLogData (XLD) messages in
	// this loop.
	// XLD messages contain PG transactions in JSON format. PKAL message
	// inform us about WAL LSN advances on the source that do not generate
	// logical decoding output.
	// A transaction always starts with a BEGIN XLD message and ends with
	// a COMMIT XLD message. For the same transaction these 2 messages
	// contain a "nextlsn" field with the same value. This value is
	// comparable with the "ServerWALEnd" field transmitted in a PKAL
	// message.
	// With wal2json, a transaction is always transmitted in its entirety
	// before the next transaction starts. So, the sequence
	//   Btx1, some content, Btx2, some content, Ctx1, ..., Ctx2
	// is not possible. It is always
	//   Btx1, some content, Ctx1, Btx2, some content, Ctx2
	//
	// However, PKAL messages can be interleaved in this sequence.
	// The following sequence is possible:
	//   B, some content, PKAL, some content, C
	// Since, B and C records always have the same "nextlsn", we know the
	// C records "nextlsn" already when we read the B.
	// The PKAL in this sequence always has PKAL.ServerWALEnd <= C.nextlsn
	//
	// So, here is what we do:
	// - we don't distinguish between write, flush or replay position.
	//   We don't have the concept of "replay". While a transaction is
	//   being received it is written to a buffer in RAM which is flushed
	//   to disk when full. A received COMMIT then flushes an even half-full
	//   buffer and syncs everything to disk. So, the write and flush
	//   positions are the same.
	// - when a BEGIN record arrives, we set inTxn = true
	// - when a PKAL record arrives and inTxn == true, it is ignored
	// - when a PKAL record arrives and inTxn == false, our flush position
	//   is advanced according to PKAL.ServerWALEnd
	// - when a COMMIT record arrives, we set inTxn = false, flush and sync
	//   the buffer and advance our flush position to C.nextlsn.

	inTxn := false
	for msg := range it {
		switch dat := msg.(type) {
		case *pglogrepl.XLogData:
			// As of PG18, WalSndPrepareWrite sends the same value for
			// WALStart and ServerWALEnd for logical replication.
			cfg.mlg.Debg2f("XLD>> ServerWALEnd=%v, ServerTime=%v",
				dat.ServerWALEnd, dat.ServerTime)
			var jdata interface{}
			err := json.Unmarshal(dat.WALData, &jdata)
			if err != nil {
				cfg.mlg.Panicf("Could not parse JSON content: %v", err)
			}

			switch v := jdata.(type) {
			case map[string]interface{}:
				cfg.mlg.Debg2f("%v", string(dat.WALData))
				if err = cfg.writeData(dat.WALData); err != nil {
					cfg.mlg.Panicf("Could not write record: %v", err)
				}
				switch {
				case v["action"] == "B":
					inTxn = true
				case v["action"] == "C",
					 v["action"] == "M" && !v["transactional"].(bool):
					if err = cfg.eoc(mylsn.LSN(dat.ServerWALEnd)); err != nil {
						cfg.mlg.Panicf("Could not write record: %v", err)
					}
					ack(dat.ServerWALEnd)
					// if we are processing a non-transactional message
					// this is a no-op.
					inTxn = false
				}
			default:
				cfg.mlg.Panicf("unexpeced JSON type %[1]T: %[1]v", v)
			}

		case *pglogrepl.PrimaryKeepaliveMessage:
			cfg.mlg.Debg2f("PKAL>> WALEnd=%v, Time=%v, ReplyReq=%v",
				dat.ServerWALEnd, dat.ServerTime, dat.ReplyRequested)
			if !inTxn {
				err := cfg.sl.SetLSN(mylsn.LSN(dat.ServerWALEnd), true)
				if err != nil {
					cfg.mlg.Panicf("Could update slot LSN: %v", err)
				}
				ack(dat.ServerWALEnd)
			}

		case *pgproto3.NoticeResponse:
			cfg.mlg.Infof("notice>> %v", dat.Message)

		default:
			cfg.mlg.Infof("SHOULD NOT HAPPEN>> %T", msg)
		}
	}
}

// Local Variables:
// tab-width: 4
// End:
