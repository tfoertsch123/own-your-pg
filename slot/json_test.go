package slot

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tfoertsch123/own-your-pg/lsn"
)

func TestSlotJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		slot Slot
	}{
		{
			name: "all fields populated",
			slot: Slot{
				header: header{
					NextLSN:  lsn.LSN(0x1FFFFFFFF),
					OwnerPid: 12345,
					SlotType: Change,
				},
				Cfg: Cfg{
					Config: map[string][]string{
						"key1": {"string11", "string12"},
						"key2": {"string21", "string22"},
					},
				},
			},
		},
		{
			name: "empty config",
			slot: Slot{
				header: header{
					NextLSN:  lsn.LSN(0),
					OwnerPid: 1,
					SlotType: Archiver,
				},
				Cfg: Cfg{
					Config: map[string][]string{},
				},
			},
		},
		{
			name: "nil config",
			slot: Slot{
				header: header{
					NextLSN:  lsn.LSN(0xFFFFFFFFFFFFFFFF),
					OwnerPid: 0,
					SlotType: Producer,
				},
			},
		},
	}

	for _, x := range tests {
		t.Run(x.name, func(t *testing.T) {
			// Without an open file handle, PidActive must be null.
			b, err := json.Marshal(x.slot)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			if !strings.Contains(string(b), `"PidActive":null`) {
				t.Errorf("expected PidActive:null in %s", b)
			}

			// Required top-level keys must be present.
			for _, key := range []string{
				`"Name"`, `"NextLSN"`, `"OwnerPid"`, `"Type"`, `"Config"`, `"PidActive"`,
			} {
				if !strings.Contains(string(b), key) {
					t.Errorf("missing %s in %s", key, b)
				}
			}

			// Round-trip back into a fresh slot.
			var got Slot
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if got.NextLSN != x.slot.NextLSN {
				t.Errorf("NextLSN: got %v, want %v", got.NextLSN, x.slot.NextLSN)
			}
			if got.OwnerPid != x.slot.OwnerPid {
				t.Errorf("OwnerPid: got %v, want %v", got.OwnerPid, x.slot.OwnerPid)
			}
			if got.SlotType != x.slot.SlotType {
				t.Errorf("Type: got %v, want %v", got.SlotType, x.slot.SlotType)
			}

			// Config comparison: nil and empty maps are equivalent after round trip.
			wantCfg := x.slot.Config
			if wantCfg == nil {
				wantCfg = map[string][]string{}
			}
			if len(got.Config) != len(wantCfg) {
				t.Errorf("Config size: got %d, want %d", len(got.Config), len(wantCfg))
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
							t.Errorf("Config[%q][%d]: got %q, want %q", k, i, gv[i], v[i])
						}
					}
				}
			}
		})
	}
}

func TestSlotJSONPidActiveIgnoredOnUnmarshal(t *testing.T) {
	// PidActive is read-only; a true value in input must not affect the slot.
	in := `{"Name":"test","NextLSN":"0/0","OwnerPid":5,"Type":"Change","Config":{"k":["v1"]},"PidActive":true}`

	var got Slot
	if err := json.Unmarshal([]byte(in), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.name != "test" {
		t.Errorf("Name: got %q, want %q", got.name, "test")
	}

	// Re-marshal: without an open file, PidActive must be null regardless of input.
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"PidActive":null`) {
		t.Errorf("PidActive should be null after round-trip, got %s", b)
	}
}

func TestSlotJSONConfigStringOrArray(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string][]string
	}{
		{
			name: "bare string",
			in:   `{"Name":"s","NextLSN":"0/0","OwnerPid":1,"Type":"Change","Config":{"k":"v1"},"PidActive":null}`,
			want: map[string][]string{"k": {"v1"}},
		},
		{
			name: "array",
			in:   `{"Name":"s","NextLSN":"0/0","OwnerPid":1,"Type":"Change","Config":{"k":["v1","v2"]},"PidActive":null}`,
			want: map[string][]string{"k": {"v1", "v2"}},
		},
		{
			name: "mixed",
			in:   `{"Name":"s","NextLSN":"0/0","OwnerPid":1,"Type":"Change","Config":{"a":"x","b":["y","z"]},"PidActive":null}`,
			want: map[string][]string{"a": {"x"}, "b": {"y", "z"}},
		},
		{
			name: "empty array",
			in:   `{"Name":"s","NextLSN":"0/0","OwnerPid":1,"Type":"Change","Config":{"k":[]},"PidActive":null}`,
			want: map[string][]string{"k": {}},
		},
	}

	for _, x := range tests {
		t.Run(x.name, func(t *testing.T) {
			var got Slot
			if err := json.Unmarshal([]byte(x.in), &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if len(got.Config) != len(x.want) {
				t.Fatalf("Config size: got %d, want %d", len(got.Config), len(x.want))
			}
			for k, v := range x.want {
				gv := got.Config[k]
				if len(gv) != len(v) {
					t.Errorf("Config[%q]: got %v, want %v", k, gv, v)
					continue
				}
				for i := range v {
					if gv[i] != v[i] {
						t.Errorf("Config[%q][%d]: got %q, want %q", k, i, gv[i], v[i])
					}
				}
			}
			// Re-marshal: single-element slices become bare strings,
			// multi-element stay arrays. Round-trip must be stable.
			b, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var rt Slot
			if err := json.Unmarshal(b, &rt); err != nil {
				t.Fatalf("re-unmarshal: %v", err)
			}
			if len(rt.Config) != len(x.want) {
				t.Fatalf("Config size: got %d, want %d", len(rt.Config), len(x.want))
			}
			for k, v := range x.want {
				gv := rt.Config[k]
				if len(gv) != len(v) {
					t.Errorf("Config[%q]: got %v, want %v", k, gv, v)
				}
			}
		})
	}
}

func TestSlotJSONConfigRejectsNonString(t *testing.T) {
	in := `{"Name":"s","NextLSN":"0/0","OwnerPid":1,"Type":"Change","Config":{"k":123},"PidActive":null}`
	var got Slot
	if err := json.Unmarshal([]byte(in), &got); err == nil {
		t.Errorf("expected error for non-string/non-array config value")
	}
}

func TestSlotJSONConfigRejectsNonIncompatibleConfig(t *testing.T) {
	in := `{"Name":"s","NextLSN":"0/0","OwnerPid":1,"Type":"Change","Config":[],"PidActive":null}`
	var got Slot
	if err := json.Unmarshal([]byte(in), &got); err == nil {
		t.Errorf("expected error for non-string/non-array config type")
	}
}

func TestSlotJSONConfigMarshalSingleVsArray(t *testing.T) {
	tests := []struct {
		name string
		cfg  map[string][]string
		want string
	}{
		{
			name: "single element -> bare string",
			cfg:  map[string][]string{"k": {"v1"}},
			want: `"k":"v1"`,
		},
		{
			name: "multi element -> array",
			cfg:  map[string][]string{"k": {"v1", "v2"}},
			want: `"k":["v1","v2"]`,
		},
		{
			name: "empty slice -> array",
			cfg:  map[string][]string{"k": {}},
			want: `"k":[]`,
		},
		{
			name: "mixed single and multi",
			cfg:  map[string][]string{"a": {"x"}, "b": {"y", "z"}},
			want: `"a":"x"`,
		},
	}

	for _, x := range tests {
		t.Run(x.name, func(t *testing.T) {
			sl := Slot{Cfg: Cfg{Config: x.cfg}}
			b, err := json.Marshal(sl)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if !strings.Contains(string(b), x.want) {
				t.Errorf("expected %q in %s", x.want, b)
			}
		})
	}
}

func TestSlotJsonAsJSON(t *testing.T) {
	sl := &Slot{
		header: header{
			NextLSN:  lsn.LSN(0x1FFFFFFFF),
			OwnerPid: 12345,
			SlotType: Change,
		},
		Cfg: Cfg{
			Config: map[string][]string{
				"key1": {"string1&1", "string1&2"},
				"key2": {"string2&1", "string2&2"},
			},
		},
	}

	exp := `{
  "Name": "",
  "NextLSN": "1/FFFFFFFF",
  "OwnerPid": 12345,
  "PidActive": null,
  "Type": "Change",
  "Config": {
    "key1": [
      "string1\u00261",
      "string1\u00262"
    ],
    "key2": [
      "string2\u00261",
      "string2\u00262"
    ]
  }
}
`

	js, err := sl.AsJSON(WithPidCheck(), WithEscapeHTML())
	if js != exp || err != nil {
		t.Errorf("exp <%s>, got <%s>, err %v", exp, js, err)
	}
}

// Local Variables:
// tab-width: 4
// End:
