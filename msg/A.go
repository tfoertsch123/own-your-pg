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
	a.CommonFields = x.CommonFields
	a.NextLsn = x.NextLsn
	return a
}

func (x *A) ToSQL() string {
	return "ABORT"
}

// Local Variables:
// tab-width: 4
// End:
