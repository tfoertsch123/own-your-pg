package lsn

import (
	"testing"
	"errors"

	"encoding/json/v2"
)

func TestParseLSN_ok(t *testing.T) {
	tests := []struct{
		s string
		l LSN
	}{
		{`0/3`, 3},
		{`1/0`, (1<<32)},
		{`0/0`, 0},
		{`1/3`, (1<<32)+3},
		{`2-3`, (2<<32)+3},
		{`FFFFFFFF-FFFFFFFF`, (0xFFFFFFFF<<32)+0xFFFFFFFF},
		{`FFFFFFFF/FFFFFFFF`, (0xFFFFFFFF<<32)+0xFFFFFFFF},
	}

	for _, x := range tests {
		lsn, err := ParseLSN(x.s)
		if err != nil || lsn != x.l {
			t.Errorf("string: %v, lsn: %x, err: %v", x.s, x.l, err)
		}
		lsn = LSN(0)
		err = (&lsn).UnmarshalText([]byte(x.s))
		if err != nil || lsn != x.l {
			t.Errorf("string: %v, lsn: %x, err: %v", x.s, x.l, err)
		}
	}
}

func TestParseLSN_fail(t *testing.T) {
	tests := []string{
		``,
		`1`,
		`1-`,
		`1/`,
		`1-o`,
		`1/o`,
		`otto`,
		`1x1`,
		`0x123/0x123`,
	}

	for _, s := range tests {
		lsn, err := ParseLSN(s)
		if !errors.Is(err, ErrInvalidLsn) || lsn != 0 {
			t.Errorf("string: %v, lsn: %x, err: %v", s, lsn, err)
		}
	}
}

func TestString(t *testing.T) {
	tests := []struct{
		l LSN
		s string
	}{
		{0, `0/0`},
		{LSN(0xFFFFFFFFFFFFFFFF), `FFFFFFFF/FFFFFFFF`},
		{LSN(0xFFFFFFFF00000001), `FFFFFFFF/1`},
		{LSN(0x1FFFFFFFF), `1/FFFFFFFF`},
	}

	for _, x := range tests {
		s := x.l.String()
		if x.s != s {
			t.Errorf("lsn: %x, s: %s", x.l, s)
		}
	}
}

func TestExpanded(t *testing.T) {
	tests := []struct{
		l LSN
		s string
	}{
		{0, `00000000-00000000`},
		{LSN(0xFFFFFFFFFFFFFFFF), `FFFFFFFF-FFFFFFFF`},
		{LSN(0xFFFFFFFF00000001), `FFFFFFFF-00000001`},
		{LSN(0x1FFFFFFFF), `00000001-FFFFFFFF`},
	}

	for _, x := range tests {
		s := x.l.Expanded()
		if x.s != s {
			t.Errorf("lsn: %x, s: %s", x.l, s)
		}
	}
}

func TestScan_ok(t *testing.T) {
	tests := []struct{
		s interface{}
		l LSN
	}{
		{uint64(3), 3},
		{uint64(0x300000000), (3<<32)},
		{`0/0`, 0},
		{[]byte(`1/3`), (1<<32)+3},
	}

	for _, x := range tests {
		var lsn LSN
		err := (&lsn).Scan(x.s)
		if err != nil || lsn != x.l {
			t.Errorf("string: %v, lsn: %x, err: %v", x.s, x.l, err)
		}
	}
}

func TestScan_fail(t *testing.T) {
	tests := []struct{
		s interface{}
		e error
	}{
		{`0/`, ErrInvalidLsn},
		{[]byte(`/3`), ErrInvalidLsn},
		{struct{}{}, ErrCannotScanType},
	}

	for _, x := range tests {
		lsn := LSN(123)
		err := (&lsn).Scan(x.s)
		if !errors.Is(err, x.e) || lsn != LSN(123) {
			t.Errorf("string: %v, experr: %v, goterr: %v", x.s, x.e, err)
		}
	}
}

func TestScan_nil(t *testing.T) {
	var lsn *LSN
	err := lsn.Scan(uint64(0))
	if err != nil {
		t.Errorf("experr: %v, goterr: %v", nil, err)
	}
}

func TestMarshalJSON(t *testing.T) {
	tests := []struct{
		l LSN
		s []byte
	}{
		{0, []byte(`"0/0"`)},
		{LSN(0xFFFFFFFFFFFFFFFF), []byte(`"FFFFFFFF/FFFFFFFF"`)},
		{LSN(0xFFFFFFFF00000001), []byte(`"FFFFFFFF/1"`)},
		{LSN(0x1FFFFFFFF), []byte(`"1/FFFFFFFF"`)},
	}

	for _, x := range tests {
		s, err := json.Marshal(x.l)
		if string(x.s) != string(s) || err != nil {
			t.Errorf("lsn: %x, s: %s, err: %v", x.l, s, err)
		}
	}
}

func TestUnmarshalJSON_ok(t *testing.T) {
	tests := []struct{
		s string
		l LSN
	}{
		{`"0/0"`, 0},
		{`"1/2"`, (1<<32)+2},
		{`"3-4"`, (3<<32)+4},
	}

	for _, x := range tests {
		var lsn LSN
		err := json.Unmarshal([]byte(x.s), &lsn)
		if err != nil || lsn != x.l {
			t.Errorf("string: %v, lsn: %#x, err: %v", x.s, x.l, err)
		}
	}
}

func TestUnmarshalJSON_fail(t *testing.T) {
	tests := []struct{
		s string
	}{
		{`0/0"`},
		{`"1/2`},
	}

	for _, x := range tests {
		lsn := LSN(123)
		err := json.Unmarshal([]byte(x.s), &lsn)
		if err == nil || lsn != LSN(123) {
			t.Errorf("string: %v, goterr: %v", x.s, err)
		}
	}
}

// Local Variables:
// tab-width: 4
// End:
