package msg

//go:generate go run ./gen nextlsn

import (
	"github.com/tfoertsch123/own-your-pg/lsn"
)

type A struct {
	CommonFields
	NextLsn lsn.LSN `json:"nextlsn"`
}

func NewAFromC(x *C) *A {
	a := NewA()
	// copy fields preserving action
	a.CommonFields, a.NextLsn, a.Action = x.CommonFields, x.NextLsn, a.Action
	return a
}

func (x *A) ToSQL() string {
	return "ABORT"
}

// Local Variables:
// tab-width: 4
// End:
