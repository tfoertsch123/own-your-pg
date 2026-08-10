package jval

import (
	"fmt"
	"errors"
	"encoding/json/jsontext"
)

type Val jsontext.Token

func (v Val) MarshalJSONTo(enc *jsontext.Encoder) error {
	return enc.WriteToken(jsontext.Token(v))
}

var ErrInvalidValue error = errors.New("invalid value")
func (v *Val) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	t, err := dec.ReadToken()
	if err != nil {
		return err
	}
	switch t.Kind() {
	case 'n', '0', '"', 't', 'f':
		*v = Val(t.Clone())
		return nil
	}
	return fmt.Errorf("%w: %v", ErrInvalidValue, t)
}

func (v Val) IsEqualTo(other Val) bool {
	return jsontext.Token(v).String() == jsontext.Token(other).String()
}

// Local Variables:
// tab-width: 4
// End:
