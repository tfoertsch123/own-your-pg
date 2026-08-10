package msg

//go:generate go run ./gen table identity

import (
	"fmt"
	"encoding/json/jsontext"
)

type D struct {
	CommonFields
	Schema string `json:"schema"`
	Table string `json:"table"`
	Identity jsontext.Value `json:"identity"`
	DecodedIdentity []COL `json:"-"`
}

func (x *D) ToSQL() string {
	where := single_where(x.GetIdentity())

	// This SQL might seem overcomplicated. But we want to abort the
	// transaction if this statement affects other than exactly 1 row.
	// In that case a division by zero exception is generated.
	return fmt.Sprintf(
		`WITH x AS (`+
			`SELECT ctid FROM %[1]s.%[2]s`+
			` WHERE %[3]s`+
			` LIMIT 1 FOR UPDATE`+
			`), d AS (`+
			`DELETE FROM %[1]s.%[2]s AS d USING x WHERE d.ctid=x.ctid`+
			` RETURNING 1`+
			`) SELECT 1/CASE WHEN count(*)=1 THEN 1 ELSE 0 END FROM d`,
		Qident(x.Schema), Qident(x.Table), where,
	)
}

// Local Variables:
// tab-width: 4
// End:
