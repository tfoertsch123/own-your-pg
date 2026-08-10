package msg

import (
	"fmt"
	"strings"

	"github.com/tfoertsch123/own-your-pg/jval"
)

func Qident(nm string) string {
	return `"`+strings.ReplaceAll(nm, `"`, `""`)+`"`
}

func names_and_values(cols []COL) ([]string, string) {
	names := make([]string, len(cols))
	values := make([]string, len(cols))
	for i, col := range cols {
		names[i] = col.Qident()
		values[i] = col.Qnullable()
	}

	return names, strings.Join(values, ", ")
}

// The first output is the list of quoted column names.
// The 2nd output is the list of rows suitable for a VALUES (), (), ...
// statement.
// The 3rd output contains for each name a flag if neither of the values in
// that column is NULL (true). In that case we can shorten the JOIN
// condition in MD from
//   (tb.col1 IS NULL AND grps.col1 IS NULL OR tb.col1 = grps.col1) AND (...)
// to
//   (tb.col1 = grps.col1) AND (...)
// Note, we cannot use IS NOT DISTINCT FROM because PG will not use an
// index in that case. IS NOT DISTINCT FROM is technically not an operator.
func names_and_Mvalues(cols [][]COL) ([]string, []string, []bool) {
	vvec := make([]string, 0, len(cols))

	row := cols[0]
	names := make([]string, len(row))
	values := make([]string, len(row))
	svec := make([]bool, len(row))
	for i, col := range row {
		names[i] = col.Qident()
		values[i] = col.Qnullable()
		svec[i] = !col.IsNull()
	}

	vvec = append(vvec, strings.Join(values, ", "))

	for _, row := range cols[1:] {
		for i, col := range row {
			values[i] = col.Qnullable()
			if svec[i] {svec[i] = !col.IsNull()}
		}
		vvec = append(vvec, strings.Join(values, ", "))
	}
	return names, vvec, svec
}

func single_where(cols []COL) string {
	x := make([]string, len(cols))
	for i, col := range cols {
		if col.IsNull() {
			x[i] = col.Qident() + ` IS NULL`
		} else {
			x[i] = col.Qident() + ` = ` + col.Qnullable()
		}
	}

	return strings.Join(x, ` AND `)
}

func set_list_diff_only(cols, ident []COL) string {
	m := make(map[string]jval.Val, len(ident))

	for _, col_ := range ident {
		m[col_.Name] = col_.Value
	}

	n := make([]string, 0, len(cols)) // len(cols) is the max size
	v := make([]string, 0, len(cols))

	for _, col := range cols {
		if val_, ok_ := m[col.Name]; ok_ && val_.IsEqualTo(col.Value) {
			// no change
			continue
		}

		n = append(n, col.Qident())
		v = append(v, col.Qnullable())
	}

	if len(n) == 0 {return ""}

	return fmt.Sprintf(`(%s) = row(%s)`,
		strings.Join(n, ", "), strings.Join(v, ", "))
}

// Local Variables:
// tab-width: 4
// End:
