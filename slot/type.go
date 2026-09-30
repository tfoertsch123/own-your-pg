package slot

import (
	"fmt"
	"errors"
	"encoding/json/v2"
	"encoding/json/jsontext"
)

type Type uint8
const (
	Any Type = iota
	Change
	Archiver
	Config
	Producer
)

func (t Type) String() string {
	switch t {
	case Any:
		return "Any"
	case Change:
		return "Change"
	case Archiver:
		return "Archiver"
	case Config:
		return "Config"
	case Producer:
		return "Producer"
	default:
		return "Unknown"
	}
}

var ErrInvalidSlotType error = errors.New("Invalid slot type")
var ErrCannotScanType error = errors.New("Cannot scan type")
func (t *Type) Scan(from interface{}) error {
	if t == nil {
		return nil
	}

	switch s := from.(type) {
	case uint8:
		if s > uint8(Producer) {
			return fmt.Errorf("%v: %w", s, ErrInvalidSlotType)
		}
		*t = Type(s)
	case string:
		switch s {
		case "Change":
			*t = Change
		case "Archiver":
			*t = Archiver
		case "Config":
			*t = Config
		case "Producer":
			*t = Producer
		default:
			return fmt.Errorf("%v: %w", s, ErrInvalidSlotType)
		}
	default:
		return fmt.Errorf("%w: %T", ErrCannotScanType, from)
	}
	return nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (t *Type) UnmarshalText(bts []byte) error {
	return t.Scan(string(bts))
}

func (t Type) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, t.String())
}

func (t *Type) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var s string
	if err := json.UnmarshalDecode(dec, &s); err != nil {
		return err
	}
	return t.Scan(s)
}

// Local Variables:
// tab-width: 4
// End:
