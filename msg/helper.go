package msg

import (
	"fmt"
	"iter"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tfoertsch123/own-your-pg/jval"
)

func Qident(nm string) string {
	return `"`+strings.ReplaceAll(nm, `"`, `""`)+`"`
}

func QliteralString(nm string) string {
	return `'`+strings.ReplaceAll(nm, `'`, `''`)+`'`
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

func readPGQuotedName(input string, index int) (string, int, error) {
	var part strings.Builder

	// The function is called with index pointing at the double-quote
	index++

	for index < len(input) {
		if input[index] != '"' {
			part.WriteByte(input[index])
			index++
			continue
		}
		if index+1 < len(input) && input[index+1] == '"' {
			part.WriteByte('"')
			index += 2
			continue
		}
		return part.String(), index + 1, nil
	}
	return "", index, fmt.Errorf("unterminated quoted identifier")
}

func readPGUnquotedName(input string, index int) (string, int, error) {
	start := index
	for index < len(input) {
		character, size := utf8.DecodeRuneInString(input[index:])
		if character == utf8.RuneError && size == 1 {
			return "", index, fmt.Errorf("invalid utf-8 in identifier")
		}
		if character == '"' {
			return "", index, fmt.Errorf("quote in unquoted identifier")
		}
		if unicode.IsSpace(character) {
			break
		}
		valid := character > 127 ||
			character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character == '_' ||
			index > start &&
				(character >= '0' && character <= '9' || character == '$')
		if !valid {
			break
		}
		index += size
	}
	return strings.ToLower(input[start:index]), index, nil
}

// DecodePGQualifiedName expects input to start with a PG object like
// a table, schema or column. The name can consist of multiple components
// separated by a dot. DecodePGQualifiedName reads as many components
// as are available, not just one, two or three. Once the rest of the
// string does not look like an identifier, the process stops.
// The first return value is the list of components in their canonical form.
// The caller is should check the length of the slice and decide if it
// expected. A list of 4 elements does most likely not represent any valid
// PG object.
// The 2nd return value is the rest of the input string. If the caller
// expects to find exactly one name in the input, this should be an empty
// string.
// The 3rd return value is a possible error.
func DecodePGQualifiedName(input string) ([]string, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, "", fmt.Errorf("empty name")
	}

	var parts []string
	i := 0
	for {
		// At this point we are positioned either at the beginning
		// of non-whitespace input or at the first position after a dot.
		// So, we have to consume possible whitespace.
		i = len(input) - len(strings.TrimLeftFunc(input[i:], unicode.IsSpace))
		if i == len(input) {
			// dangling dot
			return nil, "", fmt.Errorf("empty identifier component")
		}

		var part string
		var err error
		if input[i] == '"' {
			part, i, err = readPGQuotedName(input, i)
		} else {
			part, i, err = readPGUnquotedName(input, i)
		}
		if err != nil {
			return nil, "", err
		}
		if part == "" {
			return nil, "", fmt.Errorf("empty identifier component")
		}
		parts = append(parts, part)

		// Here we have consumed a part. There might be whitespace to
		// consume before the next meaningful piece.
		i = len(input) - len(strings.TrimLeftFunc(input[i:], unicode.IsSpace))

		// Now we are expecting a dot or the rest of the input (if there is any)
		// is not part of this identifier.
		if i == len(input) || input[i] != '.' {
			return parts, input[i:], nil
		}
		i++						// consume the dot
	}
}

// DecodePGQualifiedNames iterates over a list of qualified names delimited
// by delim. Blank input yields nothing. Invalid input yields nil and an
// error, then stops.
func DecodePGQualifiedNames(input, delim string) iter.Seq2[[]string, error] {
	return func(yield func([]string, error) bool) {
		remaining := strings.TrimSpace(input)
		if remaining == "" {
			return
		}
		for {
			parts, rest, err := DecodePGQualifiedName(remaining)
			if err != nil {
				yield(nil, err)
				return
			}
			if rest != "" && rest[0:len(delim)] != delim {
				yield(nil, fmt.Errorf("expected list delimted by %s", delim))
				return
			}
			if !yield(parts, nil) || rest == "" {
				return
			}
			remaining = rest[len(delim):]
		}
	}
}

// Local Variables:
// tab-width: 4
// End:
