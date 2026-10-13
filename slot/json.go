package slot

import (
	"maps"
	"slices"
	"encoding/json/v2"
	"encoding/json/jsontext"

	"github.com/tfoertsch123/own-your-pg/lsn"
)

// slotJSON is the JSON representation of a [Slot]. It decouples the on-disk
// binary layout of [Slot] from its JSON serialization. The PidActive field is
// not stored on disk; it is computed at marshal time from the live state of
// the slot (if an open file handle is available) and is ignored on unmarshal.
//
// The NextLSN field is omitted from the JSON output when the slot type is
// Config, since configuration slots do not track a log sequence number.
type slotJSON struct {
	Name      string     `json:"Name"`
	NextLSN   *lsn.LSN   `json:"NextLSN,omitempty"`
	OwnerPid  int64      `json:"OwnerPid"`
	PidActive *bool      `json:"PidActive"`
	Type      Type       `json:"Type"`
	Config    jsonConfig `json:"Config,omitempty"`
}

// jsonConfig is a map[string][]string that accepts either a JSON string or a
// JSON array of strings for each value when unmarshaling. A bare string is
// normalized to a single-element slice. Marshaling emits a single-element
// slice as a bare JSON string and everything else as a JSON array.
type jsonConfig map[string][]string

// MarshalJSONTo implements the [json.MarshalerTo] interface for [jsonConfig].
// It writes the config as a JSON object whose keys are sorted lexicographically.
// Values with exactly one element are written as bare JSON strings; values
// with zero or more than one element are written as JSON arrays.
func (c jsonConfig) MarshalJSONTo(enc *jsontext.Encoder) error {
	if err := enc.WriteToken(jsontext.BeginObject); err != nil {
		return err				// no idea if this can happen at all
	}
	for _, k := range slices.Sorted(maps.Keys(c)) {
		v := c[k]
		if err := enc.WriteToken(jsontext.String(k)); err != nil {
			return err
		}

		if len(v) == 1 {
			if err := json.MarshalEncode(enc, &v[0]); err != nil {
				return err
			}
		} else {
			if err := json.MarshalEncode(enc, &v); err != nil {
				return err
			}
		}
	}
	if err := enc.WriteToken(jsontext.EndObject); err != nil {
		return err				// no idea if this can happen at all
	}
	return nil
}

// UnmarshalJSONFrom implements the [json.UnmarshalerFrom] interface for
// [jsonConfig]. Each value in the JSON object may be either a bare JSON string
// or a JSON array of strings. A bare string is normalized to a single-element
// slice. If the input is not a JSON object, a [json.SemanticError] is returned.
func (recv *jsonConfig) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	if k := dec.PeekKind(); k != '{' {
		// The [json] package automatically populates relevant fields
		// in a [json.SemanticError] to provide additional context.
		return &json.SemanticError{JSONKind: k}
	}
	dec.ReadToken() // we know this is a { -- consume it

	c := make(map[string][]string, 10)
	for dec.PeekKind() != '}' {
		var key string
		var vals []string
		if err := json.UnmarshalDecode(dec, &key); err != nil {
			return err
		}

		if k := dec.PeekKind(); k == '[' {
			if err := json.UnmarshalDecode(dec, &vals); err != nil {
				return err
			}
		} else {
			var val string
			if err := json.UnmarshalDecode(dec, &val); err != nil {
				return err
			}
			vals = append(vals, val)
		}
		c[key] = vals
	}
	dec.ReadToken() // consume the }

	*recv = c
	return nil
}

// MarshalJSONTo implements the [json.MarshalerTo] interface for [Slot].
// It delegates to [slotJSON], computing the PidActive field at marshal time
// if an open file handle is available and PID checking is not suppressed.
// The NextLSN field is omitted from the output when the slot type is Config.
func (sl Slot) MarshalJSONTo(enc *jsontext.Encoder) error {
	var pidActive *bool
	if sl.fh != nil && !sl.skipCheckPidActive {
		_, active, err := sl.GetOwner()
		if err == nil { // if an error occurs, we simply print null
			pidActive = &active
		}
	}
	var lp *lsn.LSN
	if sl.SlotType != Config {
		lp = &sl.NextLSN
	}

	return json.MarshalEncode(enc, &slotJSON{
		Name:      sl.name,
		NextLSN:   lp,
		OwnerPid:  sl.OwnerPid,
		Type:      sl.SlotType,
		Config:    jsonConfig(sl.Config),
		PidActive: pidActive,
	})
}

// UnmarshalJSONFrom implements the [json.UnmarshalerFrom] interface for [Slot].
// It populates the slot's header fields and config from the decoded JSON.
// The PidActive field in the input is ignored. If NextLSN is absent or null,
// it is set to the zero-value [lsn.LSN].
func (sl *Slot) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var s slotJSON
	if err := json.UnmarshalDecode(dec, &s); err != nil {
		return err
	}

	sl.name = s.Name
	if s.NextLSN == nil {
		sl.NextLSN = lsn.LSN(0)
	} else {
		sl.NextLSN = *s.NextLSN
	}
	sl.OwnerPid = s.OwnerPid
	sl.SlotType = s.Type
	sl.Config = s.Config
	return nil
}

// asJSONopts holds the options for [Slot.AsJSON]. They are set by the
// corresponding [AsJSONOpt] functional option values.
type asJSONopts struct {
	dense    bool
	checkPid bool
	escHTML  bool
	escJS    bool
}

// AsJSONOpt is a functional option for [Slot.AsJSON].
type AsJSONOpt func(*asJSONopts)

// WithDenseJSON produces compact JSON output without indentation or line
// breaks. Without this option [Slot.AsJSON] emits pretty-printed, indented
// output.
func WithDenseJSON() AsJSONOpt {
	return func(x *asJSONopts) { x.dense = true }
}

// WithPidCheck enables checking whether the owner PID is still active at
// marshal time. Without this option the PidActive field is always null.
func WithPidCheck() AsJSONOpt {
	return func(x *asJSONopts) { x.checkPid = true }
}

// WithEscapeHTML enables escaping of HTML special characters (<, >, &) in the
// JSON output using their \uXXXX representations.
func WithEscapeHTML() AsJSONOpt {
	return func(x *asJSONopts) { x.escHTML = true }
}

// WithEscapeJS enables escaping of characters that have special meaning in
// JavaScript (U+2028, U+2029) using their \uXXXX representations.
func WithEscapeJS() AsJSONOpt {
	return func(x *asJSONopts) { x.escJS = true }
}

// AsJSON serializes the [Slot] to a JSON string. By default the output is
// pretty-printed with two-space indentation; pass [WithDenseJSON] for compact
// output. [WithPidCheck] controls whether the PidActive field is populated
// from the live system state. [WithEscapeHTML] and [WithEscapeJS] control
// character escaping in the output.
func (sl *Slot) AsJSON(opts ...AsJSONOpt) (string, error) {
	descr := &asJSONopts{}
	for _, o := range opts {
		o(descr)
	}

	// the actual check runs in the Marshal function
	sl.skipCheckPidActive = !descr.checkPid

	jsopts := []jsontext.Options{
		jsontext.AllowInvalidUTF8(false),
		jsontext.EscapeForHTML(descr.escHTML),
		jsontext.EscapeForJS(descr.escJS),
	}
	if !descr.dense {
		jsopts = append(jsopts,
			jsontext.Multiline(true),
			jsontext.SpaceAfterColon(true),
			// jsontext.SpaceAfterComma(true),
			jsontext.WithIndent("  "),
		)
	}

	b, err := json.Marshal(&sl, jsopts...)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

// Local Variables:
// tab-width: 4
// End:
