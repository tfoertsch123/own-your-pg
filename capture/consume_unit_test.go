package capture

import (
	"strings"
	"testing"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgproto3"
	cap "github.com/tfoertsch123/pglogreplsimple"
	mylsn "github.com/tfoertsch123/own-your-pg/lsn"
)

// consumeSession returns a fully initialized session in a temp dir,
// with the loggers wired up, ready for consume() tests.
func consumeSession(t *testing.T) *session {
	t.Helper()
	s := newFileTestCfg(t)
	s.maxSize = 1 << 30 // effectively unlimited
	s.curInit()
	return s
}

func xld(data string, lsn pglogrepl.LSN) *pglogrepl.XLogData {
	return &pglogrepl.XLogData{WALData: []byte(data), ServerWALEnd: lsn}
}

func pkal(lsn pglogrepl.LSN) *pglogrepl.PrimaryKeepaliveMessage {
	return &pglogrepl.PrimaryKeepaliveMessage{ServerWALEnd: lsn}
}

func TestConsume_SingleTransaction(t *testing.T) {
	s := consumeSession(t)

	var acked []pglogrepl.LSN
	seq := func(yield func(cap.MsgItem) bool) {
		if !yield(xld(`{"action":"B","xid":1}`, 0x100)) {
			return
		}
		if !yield(xld(`{"action":"I"}`, 0x200)) {
			return
		}
		yield(xld(`{"action":"C"}`, 0x300))
	}

	s.consume(seq, func(l pglogrepl.LSN, _ ...pglogrepl.LSN) {
		acked = append(acked, l)
	})

	if len(acked) != 1 || acked[0] != 0x300 {
		t.Errorf("acked: got %v, want [0x300]", acked)
	}
	if s.eoCommit == 0 {
		t.Error("eoCommit should have advanced")
	}
	if slotLSN, _ := s.sl.GetLSN(); slotLSN != 0x300 {
		t.Errorf("slot LSN: got %v, want 300", slotLSN)
	}
	// The B record placeholder must have been replaced by the C LSN.
	data := readCurrentFile(t, s)
	if stringContainsBytes(data, "XXXXXXXX/YYYYYYYY") {
		t.Error("B placeholder should have been replaced")
	}
	if !stringContainsBytes(data, mylsn.LSN(0x300).String()) {
		t.Error("B.nextlsn should contain the commit LSN")
	}
}

func TestConsume_NonTransactionalMessage(t *testing.T) {
	s := consumeSession(t)

	var acked []pglogrepl.LSN
	seq := func(yield func(cap.MsgItem) bool) {
		if !yield(xld(`{"action":"M","transactional":false}`, 0x100)) {
			return
		}
		yield(xld(`{"action":"M","transactional":true}`, 0x200))
	}

	s.consume(seq, func(l pglogrepl.LSN, _ ...pglogrepl.LSN) {
		acked = append(acked, l)
	})

	// Only the non-transactional M is committed (eoc + ack).
	if len(acked) != 1 || acked[0] != 0x100 {
		t.Errorf("acked: got %v, want [0x100]", acked)
	}
}

func TestConsume_KeepaliveOutsideTransaction(t *testing.T) {
	s := consumeSession(t)

	var acked []pglogrepl.LSN
	seq := func(yield func(cap.MsgItem) bool) {
		if !yield(pkal(0x100)) {
			return
		}
		yield(pkal(0x050)) // older; still processed
	}

	s.consume(seq, func(l pglogrepl.LSN, _ ...pglogrepl.LSN) {
		acked = append(acked, l)
	})

	if len(acked) != 2 {
		t.Fatalf("acked: got %v, want 2 entries", acked)
	}
	if slotLSN, _ := s.sl.GetLSN(); slotLSN != 0x050 {
		t.Errorf("slot LSN: got %v, want 0/50 (backwards move is allowed)",
			slotLSN)
	}
}

func TestConsume_KeepaliveInsideTransactionIgnored(t *testing.T) {
	s := consumeSession(t)

	var acked []pglogrepl.LSN
	seq := func(yield func(cap.MsgItem) bool) {
		if !yield(xld(`{"action":"B"}`, 0x100)) {
			return
		}
		if !yield(pkal(0x200)) { // must be ignored
			return
		}
		yield(xld(`{"action":"C"}`, 0x300))
	}

	s.consume(seq, func(l pglogrepl.LSN, _ ...pglogrepl.LSN) {
		acked = append(acked, l)
	})

	if len(acked) != 1 || acked[0] != 0x300 {
		t.Errorf("acked: got %v, want [0x300]", acked)
	}
}

func TestConsume_NoticeHandling(t *testing.T) {
	s := consumeSession(t)

	acked := 0
	seq := func(yield func(cap.MsgItem) bool) {
		// An ignorable warning passes through.
		if !yield(&pgproto3.NoticeResponse{
			SeverityUnlocalized: "WARNING",
			Message:             "unrelated",
		}) {
			return
		}
		// A transaction completes afterwards.
		yield(xld(`{"action":"C"}`, 0x100))
	}

	s.consume(seq, func(pglogrepl.LSN, ...pglogrepl.LSN) { acked++ })

	if acked != 1 {
		t.Errorf("acked: got %d, want 1", acked)
	}
}

func stringContainsBytes(data []byte, sub string) bool {
	return len(data) >= len(sub) &&
		strings.Index(string(data), sub) >= 0
}

// Local Variables:
// tab-width: 4
// End:
