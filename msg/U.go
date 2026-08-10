package msg

//go:generate go run ./gen table columns identity

import (
	"fmt"
	"encoding/json/jsontext"
)

type U struct {
	CommonFields
	Schema string `json:"schema"`
	Table string `json:"table"`
	Columns jsontext.Value `json:"columns"`
	DecodedColumns []COL `json:"-"`
	Identity jsontext.Value `json:"identity"`
	DecodedIdentity []COL `json:"-"`
}

func (x *U) ToSQL() string {
	set := set_list_diff_only(x.GetColumns(), x.GetIdentity())
	where := single_where(x.GetIdentity())

	if set == "" {				// no actual update
		return fmt.Sprintf(
			`SELECT 1/CASE WHEN exists(`+
				`SELECT 1 FROM %[1]s.%[2]s WHERE %[3]s) THEN 1 ELSE 0 END`,
			Qident(x.Schema), Qident(x.Table), where,
		)
	}

	// This SQL might seem overly complicated. But we want to abort the
	// transaction if this statement affects other than exactly 1 row.
	// In that case a division by zero exception is generated.
	return fmt.Sprintf(
		`WITH x AS (`+
			`SELECT ctid FROM %[1]s.%[2]s`+
			` WHERE %[3]s`+
			` LIMIT 1 FOR UPDATE`+
			`), u AS (`+
			`UPDATE %[1]s.%[2]s AS d SET %[4]s FROM x WHERE d.ctid=x.ctid`+
			` RETURNING 1`+
			`) SELECT 1/CASE WHEN count(*)=1 THEN 1 ELSE 0 END FROM u`,
		Qident(x.Schema), Qident(x.Table), where, set,
	)
}

// Local Variables:
// tab-width: 4
// End:
