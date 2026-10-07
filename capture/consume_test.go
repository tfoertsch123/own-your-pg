package capture

import (
	"bytes"
	"testing"

	mylsn "github.com/tfoertsch123/own-your-pg/lsn"
)

func TestParseAndAddLsn_Begin(t *testing.T) {
	input := []byte(`{"action":"B","xid":123}`)
	res, err := parseAndAddLsn(input, mylsn.LSN(0))
	if err != nil {
		t.Fatalf("parseAndAddLsn: %v", err)
	}
	if res.action != "B" {
		t.Errorf("action: got %q, want %q", res.action, "B")
	}

	// B records get a placeholder nextlsn
	if !bytes.Contains(res.json, []byte(`"nextlsn":"XXXXXXXX/YYYYYYYY"`)) {
		t.Errorf("B record should contain placeholder nextlsn, got: %s", res.json)
	}
}

func TestParseAndAddLsn_Commit(t *testing.T) {
	lsn := mylsn.LSN(0x1234567890ABCDEF)
	res, err := parseAndAddLsn([]byte(`{"action":"C"}`), lsn)
	if err != nil {
		t.Fatalf("parseAndAddLsn: %v", err)
	}
	if res.action != "C" {
		t.Errorf("action: got %q, want %q", res.action, "C")
	}

	// C records get the real nextlsn = ServerWALEnd
	expected := `"nextlsn":"` + lsn.String() + `"`
	if !bytes.Contains(res.json, []byte(expected)) {
		t.Errorf("C record should contain nextlsn %q, got: %s", expected, res.json)
	}
}

func TestParseAndAddLsn_MessageNonTransactional(t *testing.T) {
	lsn := mylsn.LSN(0xAABBCCDD00112233)
	input := []byte(`{"action":"M","transactional":false}`)
	res, err := parseAndAddLsn(input, lsn)
	if err != nil {
		t.Fatalf("parseAndAddLsn: %v", err)
	}
	if res.action != "M" {
		t.Errorf("action: got %q, want %q", res.action, "M")
	}
	if res.transactional {
		t.Error("transactional: got true, want false")
	}

	// All M records get lsn = ServerWALEnd
	expected := `"lsn":"` + lsn.String() + `"`
	if !bytes.Contains(res.json, []byte(expected)) {
		t.Errorf("M record should contain lsn %q, got: %s", expected, res.json)
	}
}

func TestParseAndAddLsn_MessageTransactional(t *testing.T) {
	lsn := mylsn.LSN(0xAABBCCDD00112233)
	input := []byte(`{"action":"M","transactional":true}`)
	res, err := parseAndAddLsn(input, lsn)
	if err != nil {
		t.Fatalf("parseAndAddLsn: %v", err)
	}
	if res.action != "M" {
		t.Errorf("action: got %q, want %q", res.action, "M")
	}
	if !res.transactional {
		t.Error("transactional: got false, want true")
	}

	// parseAndAddLsn adds lsn to all M records; the transactional
	// distinction is only made in consume(), not here.
	expected := `"lsn":"` + lsn.String() + `"`
	if !bytes.Contains(res.json, []byte(expected)) {
		t.Errorf("transactional M should still contain lsn, got: %s",
			res.json)
	}
}

func TestParseAndAddLsn_Insert(t *testing.T) {
	lsn := mylsn.LSN(0x0)
	input := []byte(`{"action":"I","columns":["a","b"]}`)
	res, err := parseAndAddLsn(input, lsn)
	if err != nil {
		t.Fatalf("parseAndAddLsn: %v", err)
	}
	if res.action != "I" {
		t.Errorf("action: got %q, want %q", res.action, "I")
	}

	// I records don't get nextlsn or lsn
	if bytes.Contains(res.json, []byte(`"nextlsn"`)) {
		t.Errorf("I record should not contain nextlsn, got: %s", res.json)
	}
	if bytes.Contains(res.json, []byte(`"lsn"`)) {
		t.Errorf("I record should not contain lsn, got: %s", res.json)
	}
}

func TestParseAndAddLsn_PreservesExistingFields(t *testing.T) {
	lsn := mylsn.LSN(0x12345678ABCDEF00)
	input := []byte(`{"action":"C","xid":42,"timestamp":"2026-01-01"}`)
	res, err := parseAndAddLsn(input, lsn)
	if err != nil {
		t.Fatalf("parseAndAddLsn: %v", err)
	}

	// All original fields should be preserved
	if !bytes.Contains(res.json, []byte(`"xid":42`)) {
		t.Errorf("xid field lost, got: %s", res.json)
	}
	if !bytes.Contains(res.json, []byte(`"timestamp":"2026-01-01"`)) {
		t.Errorf("timestamp field lost, got: %s", res.json)
	}
}

func TestParseAndAddLsn_TrailingData(t *testing.T) {
	input := []byte(`{"action":"B"}{}`)
	_, err := parseAndAddLsn(input, mylsn.LSN(0))
	if err == nil {
		t.Error("expected error for trailing data after JSON object")
	}
}

func TestParseAndAddLsn_NotAnObject(t *testing.T) {
	input := []byte(`[1,2,3]`)
	_, err := parseAndAddLsn(input, mylsn.LSN(0))
	if err == nil {
		t.Error("expected error for non-object JSON")
	}
}

func TestParseAndAddLsn_MalformedJSON(t *testing.T) {
	input := []byte(`{"action":`)
	_, err := parseAndAddLsn(input, mylsn.LSN(0))
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestParseAndAddLsn_Empty(t *testing.T) {
	input := []byte(``)
	_, err := parseAndAddLsn(input, mylsn.LSN(0))
	if err == nil {
		t.Error("expected error for empty input")
	}
}

// Local Variables:
// tab-width: 4
// End:
