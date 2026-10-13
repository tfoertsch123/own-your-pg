package msg

//go:generate go run ./gen table columns

import (
	"fmt"
	"strings"
	"encoding/json/jsontext"
)

type I struct {
	CommonFields
	Schema string `json:"schema"`
	Table string `json:"table"`

	// defer actual parsing to when it's needed
	Columns jsontext.Value `json:"columns"`
	DecodedColumns []COL `json:"-"`
}

func (x *I) ToSQL() string {
	n, v := names_and_values(x.GetColumns())

	return fmt.Sprintf(
		`INSERT INTO %s.%s(%s) OVERRIDING SYSTEM VALUE VALUES (%s)`,
		Qident(x.Schema), Qident(x.Table),
		strings.Join(n, ", "), v,
	)
}

// Local Variables:
// tab-width: 4
// End:
