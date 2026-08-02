package slot

import (
	"testing"
	"errors"

	"encoding/json/v2"
)

func TestTypeString(t *testing.T) {
	tests := []struct{
		t Type
		s string
	}{
		{Type(0), "Any"},
		{Type(1), "Change"},
		{Type(2), "Archiver"},
		{Type(3), "Config"},
		{Type(4), "Producer"},
		{Type(5), "Unknown"},
	}

	for _, x := range tests {
		if res := x.t.String(); res != x.s {
			t.Errorf("String(%v): exp %s, got %s", x.t, x.s, res)
		}
	}
}

func TestTypeScan(t *testing.T) {
	tests := []struct{
		i any
		t Type
		e error
	}{
		{uint8(0), Any, nil},
		{uint8(1), Change, nil},
		{uint8(2), Archiver, nil},
		{uint8(3), Config, nil},
		{uint8(4), Producer, nil},
		{uint8(5), Type(255), ErrInvalidSlotType},

		{"Any", Type(255), ErrInvalidSlotType}, // No scan for Any
		{"Change", Change, nil},
		{"Archiver", Archiver, nil},
		{"Config", Config, nil},
		{"Producer", Producer, nil},
		{"Garbage", Type(255), ErrInvalidSlotType},

		{(*uint8)(nil), Type(255), ErrCannotScanType},
	}

	for _, x := range tests {
		var v Type
		err := (&v).Scan(x.i)
		if x.t == Type(255) {
			if v != Any || !errors.Is(err, x.e) {
				t.Errorf("Scan(%v): exp 0/%v, got %v/%v", x.i, x.e, v, err)
			}
		} else {
			if v != x.t || err != nil {
				t.Errorf("Scan(%v): exp %v/nil, got %v/%v", x.i, x.t, v, err)
			}
		}
		if bts, ok := x.i.(string); ok {
			var v Type
			err := (&v).UnmarshalText([]byte(bts))
			if x.t == Type(255) {
				if v != Any || !errors.Is(err, x.e) {
					t.Errorf("Scan(%v): exp 0/%v, got %v/%v", x.i, x.e, v, err)
				}
			} else {
				if v != x.t || err != nil {
					t.Errorf("Scan(%v): exp %v/nil, got %v/%v", x.i, x.t, v,err)
				}
			}
		}
	}

	if err := (*Type)(nil).Scan("Change"); err != nil {
		t.Errorf("Scan(nil): exp nil, got %v", err)
	}
}

func TestTypeMarshal(t *testing.T) {
	tests := []struct{
		t Type
		s string
	}{
		{Type(0), `"Any"`},
		{Type(1), `"Change"`},
		{Type(2), `"Archiver"`},
		{Type(3), `"Config"`},
		{Type(4), `"Producer"`},
		{Type(5), `"Unknown"`},
	}

	for _, x := range tests {
		if bts, err := json.Marshal(x.t); string(bts) != x.s || err != nil {
			t.Errorf("Marshal(%v): exp %s, got %s", x.t, x.s, string(bts))
		}
	}
}

func TestTypeUnmarshal(t *testing.T) {
	tests := []struct{
		s string
		t Type
		e error
	}{
		{`"Any"`, Type(255), ErrInvalidSlotType}, // No scan for Any
		{`"Change"`, Change, nil},
		{`"Archiver"`, Archiver, nil},
		{`"Config"`, Config, nil},
		{`"Producer"`, Producer, nil},
		{`"Garbage"`, Type(255), ErrInvalidSlotType},
	}

	for _, x := range tests {
		var v Type
		err := json.Unmarshal([]byte(x.s), &v)
		if x.t == Type(255) {
			if v != Any || !errors.Is(err, x.e) {
				t.Errorf("Unmarshal(%v): exp 0/%v, got %v/%v", x.s, x.e, v, err)
			}
		} else {
			if v != x.t || err != nil {
				t.Errorf("Unmarshal(%v): exp %v/nil, got %v/%v",
					x.s, x.t, v, err)
			}
		}
	}

	if err := json.Unmarshal([]byte(`"Change`), (*Type)(nil)); err == nil {
		t.Error("Unmarshal(`\"Change`): exp error, got nil")
	}
}

// Local Variables:
// tab-width: 4
// End:
