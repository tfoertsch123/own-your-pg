package slot

import (
	"fmt"
	"errors"
	"encoding/json"
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

func (t Type) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t *Type) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	return t.Scan(s)
}

// Local Variables:
// tab-width: 4
// End:
