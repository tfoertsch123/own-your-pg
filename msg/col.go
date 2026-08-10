package msg

import (
	"encoding/json/v2"
	"encoding/json/jsontext"

	"github.com/tfoertsch123/own-your-pg/jval"
)

type COL struct {
	Name  string   `json:"name"`
	Type  string   `json:"type"`
	Value jval.Val `json:"value"`
}

func (c COL) Qnullable() string {
	return c.Value.Qnullable(c.Type)+"::"+c.Type
}

func (c COL) Qident() string {
	return Qident(c.Name)
}

func (c COL) String() string {
	return c.Qident()+`=`+c.Qnullable()
}

func (c COL) IsNull() bool {
	return c.Value.IsNull()
}

func DecodeColVector(in jsontext.Value) ([]COL, error) {
	out := []COL{}
	if err := json.Unmarshal(in, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func EncodeColVector(in []COL) (jsontext.Value, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}

	return jsontext.Value(raw), nil
}

func DecodeMColVector(in jsontext.Value) ([][]COL, error) {
	out := [][]COL{}
	if err := json.Unmarshal(in, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func EncodeMColVector(in [][]COL) (jsontext.Value, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}

	return jsontext.Value(raw), nil
}

// Local Variables:
// tab-width: 4
// End:
