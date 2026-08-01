package slot

import (
	"encoding/json/v2"
	"encoding/json/jsontext"
	"errors"
	"strings"
	"testing"

	"github.com/tfoertsch123/own-your-pg/lsn"
)

// errInvalidUTF8 is the sentinel error that encoding/json/v2 returns when
// marshaling a string containing invalid UTF-8. The error is defined in the
// internal package encoding/json/internal/jsonwire, which cannot be imported
// directly. Instead, we probe the public API once at init time and capture a
// reference to the exact sentinel so that errors.Is can be used in tests.
var errInvalidUTF8 error

func init() {
	_, err := json.Marshal("\xff")
	var serr *jsontext.SyntacticError
	if errors.As(err, &serr) {
		errInvalidUTF8 = serr.Err
	}
}

func TestSlotJSONRoundTrip(t *testing.T) {
	tests := []struct {
		slot Slot
		exp  string
	}{
		{
			slot: Slot{
				name: "all fields populated",
				header: header{
					NextLSN:  lsn.LSN(0x1FFFFFFFF),
					OwnerPid: 12345,
					SlotType: Change,
				},
				Cfg: Cfg{
					Config: map[string][]string{
						"k1": {"s11", "s12"},
						"k2": {"s21"},
						"k3": {"s31", "s32"},
						"k4": {"s41"},
					},
				},
			},
			exp: `{"Name":"all fields populated","NextLSN":"1/FFFFFFFF",`+
				`"OwnerPid":12345,"PidActive":null,"Type":"Change",`+
				`"Config":{"k1":["s11","s12"],"k2":"s21",`+
				`"k3":["s31","s32"],"k4":"s41"}}`,
		},
		{
			slot: Slot{
				name: "empty config",
				header: header{
					NextLSN:  lsn.LSN(0),
					OwnerPid: 1,
					SlotType: Archiver,
				},
				Cfg: Cfg{
					Config: map[string][]string{},
				},
			},
			exp: `{"Name":"empty config","NextLSN":"0/0",`+
				`"OwnerPid":1,"PidActive":null,"Type":"Archiver"}`,
		},
		{
			slot: Slot{
				name: "nil config",
				header: header{
					NextLSN:  lsn.LSN(0xFFFFFFFFFFFFFFFF),
					OwnerPid: 0,
					SlotType: Producer,
				},
			},
			exp: `{"Name":"nil config","NextLSN":"FFFFFFFF/FFFFFFFF",`+
				`"OwnerPid":0,"PidActive":null,"Type":"Producer"}`,
		},
		{
			slot: Slot{
				name: "Config => omit LSN",
				header: header{
					NextLSN:  lsn.LSN(0xFFFFFFFFFFFFFFFF),
					OwnerPid: 19,
					SlotType: Config,
				},
				Cfg: Cfg{
					Config: map[string][]string{
						"k1": {},
						"k2": {"s21"},
						"k3": {"s31", "s32"},
					},
				},
			},
			exp: `{"Name":"Config => omit LSN","OwnerPid":19,`+
				`"PidActive":null,"Type":"Config",`+
				`"Config":{"k1":[],"k2":"s21","k3":["s31","s32"]}}`,
		},
	}

	for _, x := range tests {
		t.Run(x.slot.name, func(t *testing.T) {
			// Without an open file handle, PidActive must be null.
			b, err := json.Marshal(x.slot)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(b) != x.exp {
				t.Errorf("Got: <%s>", string(b))
				t.Errorf("Exp: <%s>", x.exp)
			}

			// Round-trip back into a fresh slot.
			var got Slot
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if x.slot.SlotType != Config && got.NextLSN != x.slot.NextLSN {
				t.Errorf("NextLSN: got %v, want %v",
					got.NextLSN, x.slot.NextLSN)
			}
			if got.OwnerPid != x.slot.OwnerPid {
				t.Errorf("OwnerPid: got %v, want %v",
					got.OwnerPid, x.slot.OwnerPid)
			}
			if got.SlotType != x.slot.SlotType {
				t.Errorf("Type: got %v, want %v",
					got.SlotType, x.slot.SlotType)
			}

			// Config comparison: nil and empty maps are equivalent after
			// round trip.
			wantCfg := x.slot.Config
			if wantCfg == nil {
				wantCfg = map[string][]string{}
			}
			if len(got.Config) != len(wantCfg) {
				t.Errorf("Config size: got %d, want %d",
					len(got.Config), len(wantCfg))
			} else {
				for k, v := range wantCfg {
					gv, ok := got.Config[k]
					if !ok {
						t.Errorf("Config missing key %q", k)
						continue
					}
					if len(gv) != len(v) {
						t.Errorf("Config[%q]: got %v, want %v", k, gv, v)
						continue
					}
					for i := range v {
						if gv[i] != v[i] {
							t.Errorf("Config[%q][%d]: got %q, want %q",
								k, i, gv[i], v[i])
						}
					}
				}
			}
		})
	}
}

func TestSlotJSONPidActiveIgnoredOnUnmarshal(t *testing.T) {
	// PidActive is read-only; a true value in input must not affect the slot.
	in := `{"Name":"test","NextLSN":"0/0","OwnerPid":5,`+
		`"Type":"Change","Config":{"k":["v1"]},"PidActive":true}`

	var got Slot
	if err := json.Unmarshal([]byte(in), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Re-marshal: without an open file, PidActive must be null
	// regardless of input.
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"PidActive":null`) {
		t.Errorf("PidActive should be null after round-trip, got %s", b)
	}
}

func TestSlotJSONConfigRejectsNonString(t *testing.T) {
	in := `{"Name":"s","NextLSN":"0/0","OwnerPid":1,"Type":"Change",`+
		`"Config":{"k":123},"PidActive":null}`
	var got Slot
	if err := json.Unmarshal([]byte(in), &got); err == nil {
		t.Errorf("expected error for non-string/non-array config value")
	}
}

func TestSlotJSONConfigRejectsIncompatibleConfig(t *testing.T) {
	in := `{"Name":"s","NextLSN":"0/0","OwnerPid":1,"Type":"Change",`+
		`"Config":[],"PidActive":null}`
	var got Slot
	if err := json.Unmarshal([]byte(in), &got); err == nil {
		t.Errorf("expected error for non-string/non-array config type")
	}
}

func TestSlotJSONConfigRejectsInvalidStrings(t *testing.T) {
	in := `{"Config":{"key":"test","k2":["tets"]},"Name":"s","NextLSN":"0/0",`+
		`"OwnerPid":1,"Type":"Change","PidActive":null}`
	bts := []byte(in)

	t.Run("Non-UTF8 Key", func(t *testing.T) {
		pos := len(`{"Config":{"k`)
		bts[pos] = 0xff
		defer func(){
			bts[pos] = 'e'
		}()
		
		var got Slot
		if err := json.Unmarshal(bts, &got,
			jsontext.AllowInvalidUTF8(false),
		); err == nil {
			t.Errorf("expected error for non-string/non-array config type")
			t.Logf("%#v", got)
		} else {
			t.Logf("expected error = %v", err)
		}
	})

	t.Run("Non-UTF8 StringValue", func(t *testing.T) {
		pos := len(`{"Config":{"key":"t`)
		bts[pos] = 0xff
		defer func(){
			bts[pos] = 'e'
		}()
		
		var got Slot
		if err := json.Unmarshal(bts, &got,
			jsontext.AllowInvalidUTF8(false),
		); err == nil {
			t.Errorf("expected error for non-string/non-array config type")
			t.Logf("%#v", got)
		} else {
			t.Logf("expected error = %v", err)
		}
	})

	t.Run("Non-UTF8 ArrayValue", func(t *testing.T) {
		pos := len(`{"Config":{"key":"test","k2":["t`)
		bts[pos] = 0xff
		defer func(){
			bts[pos] = 'e'
		}()
		
		var got Slot
		if err := json.Unmarshal(bts, &got,
			jsontext.AllowInvalidUTF8(false),
		); err == nil {
			t.Errorf("expected error for non-string/non-array config type")
			t.Logf("%#v", got)
		} else {
			t.Logf("expected error = %v", err)
		}
	})
}

func TestSlotJsonAsJSON(t *testing.T) {
	sl := &Slot{
		name: ":<>^:\u2028\u2029:",
		header: header{
			NextLSN:  lsn.LSN(0x1FFFFFFFF),
			OwnerPid: 12345,
			SlotType: Change,
		},
		Cfg: Cfg{
			Config: map[string][]string{
				"key1": {"string1&1", "string1&2"},
				"key2": {"string2&1"},
			},
		},
	}

	t.Run("no options", func(t *testing.T) {
		exp := `{
~~"Name":~":<>^:`+"\u2028\u2029"+`:",
~~"NextLSN":~"1/FFFFFFFF",
~~"OwnerPid":~12345,
~~"PidActive":~null,
~~"Type":~"Change",
~~"Config":~{
~~~~"key1":~[
~~~~~~"string1&1",
~~~~~~"string1&2"
~~~~],
~~~~"key2":~"string2&1"
~~}
}`

		js, err := sl.AsJSON()
		if js != strings.ReplaceAll(exp, "~", " ") || err != nil {
			t.Errorf("exp <%s>, got <%s>, err %v",
				exp,
				strings.ReplaceAll(js,  " ", "~"),
				err)
		}
	})

	t.Run("EscHTML", func(t *testing.T) {
		exp := `{
~~"Name":~":\u003c\u003e^:`+"\u2028\u2029"+`:",
~~"NextLSN":~"1/FFFFFFFF",
~~"OwnerPid":~12345,
~~"PidActive":~null,
~~"Type":~"Change",
~~"Config":~{
~~~~"key1":~[
~~~~~~"string1\u00261",
~~~~~~"string1\u00262"
~~~~],
~~~~"key2":~"string2\u00261"
~~}
}`

		js, err := sl.AsJSON(WithEscapeHTML())
		if js != strings.ReplaceAll(exp, "~", " ") || err != nil {
			t.Errorf("exp <%s>, got <%s>, err %v",
				exp,
				strings.ReplaceAll(js,  " ", "~"),
				err)
		}
	})

	t.Run("EscJS", func(t *testing.T) {
		exp := `{
~~"Name":~":<>^:\u2028\u2029:",
~~"NextLSN":~"1/FFFFFFFF",
~~"OwnerPid":~12345,
~~"PidActive":~null,
~~"Type":~"Change",
~~"Config":~{
~~~~"key1":~[
~~~~~~"string1&1",
~~~~~~"string1&2"
~~~~],
~~~~"key2":~"string2&1"
~~}
}`

		js, err := sl.AsJSON(WithEscapeJS())
		if js != strings.ReplaceAll(exp, "~", " ") || err != nil {
			t.Errorf("exp <%s>, got <%s>, err %v",
				exp,
				strings.ReplaceAll(js,  " ", "~"),
				err)
		}
	})

	t.Run("EscJS+Dense", func(t *testing.T) {
		exp := `{"Name":":<>^:\u2028\u2029:","NextLSN":"1/FFFFFFFF",`+
			`"OwnerPid":12345,"PidActive":null,"Type":"Change",`+
			`"Config":{"key1":["string1&1","string1&2"],"key2":"string2&1"}}`

		js, err := sl.AsJSON(WithEscapeJS(), WithDenseJSON(), WithPidCheck())
		if js != strings.ReplaceAll(exp, "~", " ") || err != nil {
			t.Errorf("exp <%s>, got <%s>, err %v",
				exp,
				strings.ReplaceAll(js,  " ", "~"),
				err)
		}
	})

	t.Run("Invalid UTF8", func(t *testing.T) {
		bts := []byte("test")
		bts[1] = 0xff

		nm := sl.name
		sl.name=string(bts)
		defer func(){
			sl.name = nm
		}()

		js, err := sl.AsJSON()
		if err == nil {
			t.Errorf("unexpected success, got <%s>", js)
		} else if !errors.Is(err, errInvalidUTF8) {
			t.Errorf("expecting invalid UTF8, got %v", err)
		}
	})
}

// Local Variables:
// tab-width: 4
// End:
