package slot

import (
	"maps"
	"slices"
	"encoding/json/v2"
	"encoding/json/jsontext"

	"github.com/tfoertsch123/own-your-pg/lsn"
)

type slotJSON struct {
	Name      string              `json:"Name"`
	NextLSN   *lsn.LSN            `json:"NextLSN,omitempty"`
	OwnerPid  int64               `json:"OwnerPid"`
	PidActive *bool               `json:"PidActive"`
	Type      Type                `json:"Type"`
	Config    jsonConfig          `json:"Config,omitempty"`
}

// jsonConfig is a map[string][]string that accepts either a JSON string or a
// JSON array of strings for each value when unmarshaling. A bare string is
// normalized to a single-element slice. Marshaling emits a single-element
// slice as a bare JSON string and everything else as a JSON array.
type jsonConfig map[string][]string

func (c jsonConfig) MarshalJSONTo(enc *jsontext.Encoder) error {
	if err := enc.WriteToken(jsontext.BeginObject); err != nil {
		return err
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
		return err
	}
	return nil
}

func (recv *jsonConfig) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	if k := dec.PeekKind(); k != '{' {
		// The [json] package automatically populates relevant fields
		// in a [json.SemanticError] to provide additional context.
		return &json.SemanticError{JSONKind: k}
	}
	dec.ReadToken()				// we know this is a { -- consume it

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
	dec.ReadToken()				// consume the }

	*recv = c
	return nil
}

func (sl Slot) MarshalJSONTo(enc *jsontext.Encoder) error {
	var pidActive *bool
	if sl.fh != nil && !sl.skipCheckPidActive {
		active, err := sl.OwnerActive()
		if err == nil {			// if an error occurs, we simply print null
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

type asJSONopts struct {
	dense    bool
	checkPid bool
	escHTML  bool
	escJS    bool
}
type AsJSONOpt func(*asJSONopts)

func WithDenseJSON () AsJSONOpt {
	return func(x *asJSONopts) {x.dense = true}
}

func WithPidCheck () AsJSONOpt {
	return func(x *asJSONopts) {x.checkPid = true}
}

func WithEscapeHTML () AsJSONOpt {
	return func(x *asJSONopts) {x.escHTML = true}
}

func WithEscapeJS () AsJSONOpt {
	return func(x *asJSONopts) {x.escJS = true}
}

func (sl *Slot) AsJSON(opts ...AsJSONOpt) (string, error) {
	descr := &asJSONopts{}
	for _, o := range opts {o(descr)}

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
