package msg

//go:generate go run ./gen table identities

import (
	"fmt"
	"strings"
	"encoding/json/jsontext"
)

type MD struct {
	CommonFields
	Schema string `json:"schema"`
	Table string `json:"table"`
	Identities jsontext.Value `json:"identities"`
	DecodedIdentities [][]COL `json:"-"`
	// Pk looks like so: "pk":[{"name":"x","type":"text"}, {...}]
	// Pk jsontext.Value `json:"identity"`
	// DecodedPk []COL `json:"-"`
}

func (x *MD) ToSQL() string {
	n, v, is_simple := names_and_Mvalues(x.GetIdentities())

	join := make([]string, len(n))
	tbn:= make([]string, len(n))
	for i, x := range n {
		tbn[i] = `tb.`+x
		f := `tb.%[1]s IS NULL AND grps.%[1]s IS NULL OR tb.%[1]s = grps.%[1]s`
		if is_simple[i] {f = `tb.%[1]s = grps.%[1]s`}
		join[i] = fmt.Sprintf(f, x)
	}

	return fmt.Sprintf(
		"WITH /* MD %[7]d rows */ list(%[3]s) AS (\n"+
		"    VALUES (%[4]s)\n"+
		")\n"+
		", grps AS (\n"+
		"    SELECT %[3]s, COUNT(*) AS \"_-:cnt:-_\"\n"+
		"      FROM list\n"+
		"     GROUP BY %[3]s\n"+
		")\n"+
		", lck AS (\n"+
		"    SELECT tb.ctid\n"+
		"         , grps.\"_-:cnt:-_\"\n"+
		"         , %[5]s\n"+
		"      FROM %[1]s.%[2]s AS tb\n"+
		"      JOIN grps ON (%[6]s)\n"+
		"       FOR UPDATE OF tb\n"+
		")\n"+
		" , num AS (\n"+
		"    SELECT ctid\n"+
		"         , \"_-:cnt:-_\"\n"+
		"         , ROW_NUMBER() OVER (PARTITION BY %[3]s) AS \"_-:rn:-_\"\n"+
		"      FROM lck\n"+
		")\n"+
		" , del AS (\n"+
		"    DELETE FROM %[1]s.%[2]s AS tb\n"+
		"     USING num\n"+
		"     WHERE tb.ctid = num.ctid\n"+
		"       AND num.\"_-:rn:-_\" <= num.\"_-:cnt:-_\"\n"+
		"    RETURNING *\n"+
		")\n"+
		"SELECT 1/((SELECT count(*) FROM list)=\n"+
		"          (SELECT count(*) FROM del))::INT",
		Qident(x.Schema), Qident(x.Table),
		strings.Join(n, ", "), strings.Join(v, "),\n           ("),
		strings.Join(tbn, ", "),
		strings.Join(join, ") AND ("),
		len(v),
	)
}

// Local Variables:
// tab-width: 4
// End:
