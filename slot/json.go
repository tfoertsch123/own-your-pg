package slot

import (
	"fmt"
	"strings"
	"encoding/json"

	"github.com/tfoertsch123/own-your-pg/lsn"
)

type slotJSON struct {
	Name      string              `json:"Name"`
	NextLSN   *lsn.LSN            `json:"NextLSN",omitnull`
	OwnerPid  int64               `json:"OwnerPid"`
	PidActive *bool               `json:"PidActive"`
	Type      Type                `json:"Type"`
	Config    jsonConfig          `json:"Config"`
}

// jsonConfig is a map[string][]string that accepts either a JSON string or a
// JSON array of strings for each value when unmarshaling. A bare string is
// normalized to a single-element slice. Marshaling emits a single-element
// slice as a bare JSON string and everything else as a JSON array.
type jsonConfig map[string][]string

func (c jsonConfig) MarshalJSON() ([]byte, error) {
	buf := []byte("{")
	first := true
	for k, v := range c {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf = append(buf, kb...)
		buf = append(buf, ':')
		if len(v) == 1 {
			vb, err := json.Marshal(v[0])
			if err != nil {
				return nil, err
			}
			buf = append(buf, vb...)
		} else {
			vb, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			buf = append(buf, vb...)
		}
	}
	buf = append(buf, '}')
	return buf, nil
}

func (c *jsonConfig) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if *c == nil {
		*c = make(map[string][]string, len(raw))
	}
	for k, v := range raw {
		var arr []string
		if err := json.Unmarshal(v, &arr); err == nil {
			(*c)[k] = arr
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			(*c)[k] = []string{s}
			continue
		}
		return fmt.Errorf("Config[%q]: expected string or []string", k)
	}
	return nil
}

func (sl Slot) MarshalJSON() ([]byte, error) {
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

	return json.Marshal(slotJSON{
		Name:      sl.name,
		NextLSN:   lp,
		OwnerPid:  sl.OwnerPid,
		Type:      sl.SlotType,
		Config:    jsonConfig(sl.Config),
		PidActive: pidActive,
	})
}

func (sl *Slot) UnmarshalJSON(b []byte) error {
	var s slotJSON
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	sl.name = s.Name
	sl.NextLSN = *s.NextLSN
	sl.OwnerPid = s.OwnerPid
	sl.SlotType = s.Type
	sl.Config = s.Config
	return nil
}

type asJSONopts struct {
	dense    bool
	checkPid bool
	escHTML  bool
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

func (sl *Slot) AsJSON(opts ...AsJSONOpt) (string, error) {
	descr := &asJSONopts{}
	for _, o := range opts {o(descr)}

	sl.skipCheckPidActive = !descr.checkPid

	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(descr.escHTML)

	if !descr.dense {
		enc.SetIndent("", "  ")
	}

	if err := enc.Encode(sl); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Local Variables:
// tab-width: 4
// End:
