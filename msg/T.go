package msg

//go:generate go run ./gen table

import (
	"fmt"
)

type T struct {
	CommonFields
	Schema string `json:"schema"`
	Table string `json:"table"`
}

func (x *T) ToSQL() string {
	return fmt.Sprintf(
		`TRUNCATE %s.%s`,
		Qident(x.Schema), Qident(x.Table),
	)
}

// Local Variables:
// tab-width: 4
// End:
