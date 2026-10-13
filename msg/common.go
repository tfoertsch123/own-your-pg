package msg

import (
	"encoding/json/v2"
	"fmt"

	"github.com/tfoertsch123/own-your-pg/lsn"
)

type Common interface {
	GetAction() string
	GetXid() uint32
	GetTimestamp() string
	GetLsn() lsn.LSN
	IsEqualTo(Common) bool
	AsJSON() string
	String() string
	ToSQL() string
}

type CommonFields struct {
	Action string `json:"action"`
	Xid *uint32 `json:"xid"`
	Timestamp *string `json:"timestamp"`
	Lsn lsn.LSN `json:"lsn"`
}

func (x *CommonFields) IsEqualTo(o *CommonFields) bool {
	return (x.Xid == nil) == (o.Xid == nil) &&
		(x.Xid == nil || *x.Xid == *o.Xid) &&
		(x.Timestamp == nil) == (o.Timestamp == nil) &&
		(x.Timestamp == nil || *x.Timestamp == *o.Timestamp) &&
		x.Action == o.Action && x.Lsn == o.Lsn
}

func (x *CommonFields) GetAction() string {
	return x.Action
}

func (x *CommonFields) GetXid() uint32 {
	if x.Xid == nil {
		return 0
	} else {
		return *x.Xid
	}
}

func (x *CommonFields) GetTimestamp() string {
	if x.Timestamp == nil {
		return ""
	} else {
		return *x.Timestamp
	}
}

func (x *CommonFields) GetLsn() lsn.LSN {
	return x.Lsn
}

func Parse(m []byte) (Common, error) {
	var probe struct{
		Action string `json:"action"`
	}
    if err := json.Unmarshal(m, &probe); err != nil {return nil, err}

	switch probe.Action {
	case "B":
		var a B
		if err := json.Unmarshal(m, &a); err != nil {return nil, err}
		return &a, nil
	case "C":
		var a C
		if err := json.Unmarshal(m, &a); err != nil {return nil, err}
		return &a, nil
	case "A":
		var a A
		if err := json.Unmarshal(m, &a); err != nil {return nil, err}
		return &a, nil
	case "I":
		var a I
		if err := json.Unmarshal(m, &a); err != nil {return nil, err}
		return &a, nil
	case "MI":
		var a MI
		if err := json.Unmarshal(m, &a); err != nil {return nil, err}
		return &a, nil
	case "U":
		var a U
		if err := json.Unmarshal(m, &a); err != nil {return nil, err}
		return &a, nil
	case "D":
		var a D
		if err := json.Unmarshal(m, &a); err != nil {return nil, err}
		return &a, nil
	case "MD":
		var a MD
		if err := json.Unmarshal(m, &a); err != nil {return nil, err}
		return &a, nil
	case "T":
		var a T
		if err := json.Unmarshal(m, &a); err != nil {return nil, err}
		return &a, nil
	case "M":
		var a M
		if err := json.Unmarshal(m, &a); err != nil {return nil, err}
		return &a, nil
	case "S":
		var a S
		if err := json.Unmarshal(m, &a); err != nil {return nil, err}
		return &a, nil
	case "Z":
		var a Z
		if err := json.Unmarshal(m, &a); err != nil {return nil, err}
		return &a, nil
	default:
		return nil, fmt.Errorf("unknown action type: %s\n", probe.Action)
	}
}

// Local Variables:
// tab-width: 4
// End:
