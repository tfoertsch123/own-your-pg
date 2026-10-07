package capture

import (
	jt "encoding/json/jsontext"
	"encoding/json/v2"
	"iter"
	"bytes"
	"fmt"
	"io"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgproto3"

	mylsn "github.com/tfoertsch123/own-your-pg/lsn"
	cap "github.com/tfoertsch123/pglogreplsimple"
)

type parsed struct {
	json []byte
	action string
	transactional bool
}

func parseAndAddLsn(data []byte, lsn mylsn.LSN) (*parsed, error) {
	res := &parsed{}
	dec := jt.NewDecoder(bytes.NewReader(data))

	var out bytes.Buffer
	enc := jt.NewEncoder(
		&out,
		jt.AllowInvalidUTF8(true),
		jt.EscapeForHTML(false),
		jt.EscapeForJS(false),
		jt.PreserveRawStrings(true),
	)

	start, err := dec.ReadToken()
	if err != nil {
		return nil, err
	}
	if start.Kind() != '{' {
		return nil, fmt.Errorf("expected a JSON object")
	}
	if err := enc.WriteToken(start); err != nil {
		return nil, err
	}

	for dec.PeekKind() != '}' {
		key, err := dec.ReadToken()
		if err != nil {
			return nil, err
		}
		err = enc.WriteToken(key)
		if err != nil {
			return nil, err
		}
		what := key.String()

		raw, err := dec.ReadValue()
		if err != nil {
			return nil, err
		}
		err = enc.WriteValue(raw)
		if err != nil {
			return nil, err
		}

		switch what {
		case "action":
			err = json.Unmarshal(raw, &res.action)
			switch res.action {
			case "B":
				if err := enc.WriteToken(jt.String("nextlsn")); err != nil {
					return nil, err
				}
				placeholder := "XXXXXXXX/YYYYYYYY"
				if err := json.MarshalEncode(enc, placeholder); err != nil {
					return nil, err
				}
			case "C":
				if err := enc.WriteToken(jt.String("nextlsn")); err != nil {
					return nil, err
				}
				if err := json.MarshalEncode(enc, lsn); err != nil {
					return nil, err
				}
			case "M":
				if err := enc.WriteToken(jt.String("lsn")); err != nil {
					return nil, err
				}
				if err := json.MarshalEncode(enc, lsn); err != nil {
					return nil, err
				}
			}
		case "transactional":
			err = json.Unmarshal(raw, &res.transactional)
		}
		if err != nil {
			return nil, err
		}
	}

	// consume }
	if _, err := dec.ReadToken(); err != nil {
		return nil, err
	}
	if _, err := dec.ReadToken(); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("unexpected data after JSON object")
		}
		return nil, err
	}

	if err := enc.WriteToken(jt.EndObject); err != nil {
		return nil, err
	}

	res.json = bytes.TrimSuffix(out.Bytes(), []byte("\n"))
	return res, nil
}

func (cfg *session) consume(
	it iter.Seq[cap.MsgItem],
	ack func(pglogrepl.LSN, ...pglogrepl.LSN),
) {
	// We receive 3 types of messages here:
	// - XLogData (XLD below)
	// - PrimaryKeepaliveMessage (PKAL below)
	// - NoticeResponse
	//
	// XLD contain the actual replication payload. PKAL is a means to inform
	// the client (us) about changes in the WAL position on the source that
	// do not generate replication data. Think of VACUUM, CREATE INDEX or
	// changes in a different database in the same cluster.
	// A NoticeResponse is a rare message informing us about some error
	// or warning. Notably, wal2json generates such messages when a table
	// without a replication identity is updated or deleted from. We need
	// to decide whether or not to proceed after receiving this message.
	//
	// Wal2json transmits one transaction at a time. Transactions are sent
	// in commit order. A transaction consists of multiple XLD messages.
	// These messages can be interlaced with PKAL messages.
	//
	// A transaction always starts with a BEGIN (B) XLD message and ends with
	// a COMMIT (C) XLD message. For the transaction order, only the C message
	// are important. Besides these, the protocol also knows DML messages
	// like I(nsert), U(pdate), D(elete) or T(runcate). Since these are always
	// part of a transaction, they appear between a B/C pair. Then the protocol
	// knows about transactional and non-transactional M(essages).
	// Transactional M are part of the transaction. So, they appear between
	// a B/C pair.
	// Non-transactional M appear outside of a B/C pair. For the following,
	// M represents a non-transactional message while transactional messages
	// are grouped with the rest of DML.
	//
	// LSN and feedback
	// ================
	//
	// Now, each XLD and PKAL message comes with 2 LSNs in the protocol
	// envelope, WALStart and ServerWALEnd. For logical replication, they
	// are ALWAYS the same. So, we only use ServerWALEnd here. This is also
	// the LSN we need to report back to the source in order to indicate
	// our progress and to release the WAL at the source database.
	//
	// Within a B/C pair, a PKAL can be received. So, this would be possible:
	//   B, I, U, PKAL, D, C
	// However, in this scenario PKAL,ServerWALEnd <= C.ServerWALEnd always
	// holds.
	//
	// For transaction/message ordering, only C, M (non-transactional) and
	// PKAL messages are important. We basically collapse an entire B...C
	// sequence into a single C event. Now, if X stands for either a
	// C (the entire transaction) or an M, and we receive the following
	// sequence:
	//   X1, X2, PKAL1, X3
	// then:
	// - X1.ServerWALEnd < X2.ServerWALEnd < X3.ServerWALEnd
	// - PKAL1.ServerWALEnd <= X3.ServerWALEnd
	// - but X2.ServerWALEnd <= PKAL1.ServerWALEnd is NOT necessarily true
	//
	// PG distinguishes between write, flush and replay LSN in the feedback
	// message. We don't. We don't want to lose a transaction even on power
	// loss. So, we sync every C to disk. Non-transactional messages are not
	// so important. But they are rare. So, we treat them similar to a
	// transaction consisting of just one thing.
	//
	// With all of this in mind, we:
	// - advance our feedback LSN on each C or M
	// - ignore PKAL within a B/C pair
	// - advance the feedback LSN on PKAL outside of a transaction if
	//   the PKAL.ServerWALEnd is greater than our feedback LSN
	//
	// The B/C/M JSON records
	// ======================
	//
	// We could call wal2json with the "include-lsn" option. It would then
	// add "lsn" fields to all records and "nextlsn" to B/C records. These
	// LSNs are almost all overhead. We can derive the important ones
	// from ServerWALEnd:
	// - C.nextlsn == C.ServerWALEnd
	// - M.lsn == M.ServerWALEnd
	// The only thing that's not nice is the useless B.ServerWALEnd. The
	// B.nextlsn field contains the subsequent C's ServerWALEnd. However,
	// for a possible consumer of the files written by us, the ability to
	// decide whether or not to replay a transaction would be good to make
	// when the B record is read and not only when C is read.
	//
	// In order to accommodate this we do this in parseAndAddLsn():
	// - add ServerWALEnd as "nextlsn" to every C record.
	// - add ServerWALEnd as "lsn" to any M record.
	// - add a placeholder as "nextlsn" to every B record.
	//
	// The eoc() function then replaces B's placeholder with the actual
	// C.ServerWALEnd. This is done BEFORE the file header is updated
	// with the EOC position.
	//
	// Now a reader of our output files can always read up to the currently
	// published EOC position. It does not matter if the file is a history
	// file or the current one. If the file is the current one, the reader
	// should then poll (inotify) the header until the EOC position moves
	// forward or the file is rotated.

	inTxn := false
	for msg := range it {
		switch dat := msg.(type) {
		case *pglogrepl.XLogData:
			// cfg.mlg.Debg2f("XLD>> ServerWALEnd=%v, ServerTime=%v",
			// 	dat.ServerWALEnd, dat.ServerTime)
			jdata, err := parseAndAddLsn(
				dat.WALData,
				mylsn.LSN(dat.ServerWALEnd),
			)
			if err != nil {
				cfg.mlg.Panicf("Could not parse JSON content: %v", err)
			}

			// cfg.mlg.Debg2f("%v", string(jdata.json))
			if err = cfg.writeData(jdata.json); err != nil {
				cfg.mlg.Panicf("Could not write record: %v", err)
			}
			switch {
			case jdata.action == "B":
				inTxn = true
			case jdata.action == "C",
				 jdata.action == "M" && !jdata.transactional:
				if err = cfg.eoc(
					mylsn.LSN(dat.ServerWALEnd),
					jdata.action == "C", // whether or not to write B.nextlsn
				); err != nil {
					cfg.mlg.Panicf("Could not write record: %v", err)
				}
				ack(dat.ServerWALEnd)
				// if we are processing a non-transactional message
				// this is a no-op.
				inTxn = false
			}

		case *pglogrepl.PrimaryKeepaliveMessage:
			// cfg.mlg.Debg2f("PKAL>> WALEnd=%v, Time=%v, ReplyReq=%v",
			// 	dat.ServerWALEnd, dat.ServerTime, dat.ReplyRequested)
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
			cfg.mlg.Panicf("SHOULD NOT HAPPEN>> %T", msg)
		}
	}
}

// Local Variables:
// tab-width: 4
// End:
