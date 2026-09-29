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
			// var jdata interface{}
			// err := json.Unmarshal(dat.WALData, &jdata)
			jdata, err := parseAndAddLsn(
				dat.WALData,
				mylsn.LSN(dat.ServerWALEnd),
			)
			if err != nil {
				cfg.mlg.Panicf("Could not parse JSON content: %v", err)
			}

			// It would be good to get rid of include-lsn. We have "almost"
			// enough information to do so. ServerWALEnd is transmitted in
			// the protocol envelope. So, we always have that.
			// B - ServerWALEnd is the LSN very first time the transaction
			//     appears in the WAL. "nextlsn" is the position after the
			//     transaction's COMMIT record.
			// C - Both, ServerWALEnd and "nextlsn", represent the position
			//     after the COMMIT record
			// M - Both, ServerWALEnd and "lsn", represent the position after
			//     the record. This is only relevant for non-transactional
			//     messages.
			// When we read a stream of messages, when we read a B, we need
			// to decide whether or not to skip the transaction. B/C "nextlsn"
			// and M "lsn" are perfect for that. They also come in commit
			// order on the source and represent the same thing as the
			// ServerWALEnd in a PKAL.
			// So, we have 3 options:
			// - keep include-lsn and use B's nextlsn to decide whether or
			//   not to replay.
			// - postpone the decision to C. That would mean we need to
			//   abort a transaction. That's not a good idea.
			// - since a transaction is never split across multiple files,
			//   we could insert a placeholder for B's nextlsn and write
			//   the actual LSN only when the C record is written.
			cfg.mlg.Debg2f("%v", string(jdata.json))
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
