package msg

//go:generate go run ./gen nextlsn

import (
	"github.com/tfoertsch123/own-your-pg/lsn"
)

type C struct {
	CommonFields
	NextLsn lsn.LSN `json:"nextlsn"`
}

func (x *C) ToSQL() string {
	return "COMMIT"
}

// Local Variables:
// tab-width: 4
// End:
