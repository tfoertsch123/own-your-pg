package msg

//go:generate go run ./gen nextlsn

import (
	"github.com/tfoertsch123/own-your-pg/lsn"
)

type B struct {
	CommonFields
	NextLsn lsn.LSN `json:"nextlsn"`
}

func (x *B) ToSQL() string {
	return "BEGIN"
}

// Local Variables:
// tab-width: 4
// End:
