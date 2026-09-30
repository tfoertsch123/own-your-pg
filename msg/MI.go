package msg

//go:generate go run ./gen table rows

import (
	"fmt"
	"strings"
	"encoding/json/jsontext"
)

type MI struct {
	CommonFields
	Schema string `json:"schema"`
	Table string `json:"table"`
	Rows jsontext.Value `json:"rows"`
	DecodedRows [][]COL `json:"-"`
}

func (x *MI) ToSQL() string {
	n, v, _ := names_and_Mvalues(x.GetRows())

	return fmt.Sprintf(
		"INSERT /* MI %d rows*/ INTO %s.%s(%s) OVERRIDING SYSTEM VALUE "+
			"VALUES\n    (%s)",
		len(v),
		Qident(x.Schema), Qident(x.Table),
		strings.Join(n, ", "), strings.Join(v, "),\n    ("),
	)
}

// Local Variables:
// tab-width: 4
// End:
