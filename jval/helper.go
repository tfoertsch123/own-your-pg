package jval

import (
	"fmt"
	"strings"
	"encoding/json/jsontext"
)

func (v Val) IsNull() bool {
	return jsontext.Token(v).Kind() == jsontext.KindNull
}

func (v Val) Qnullable(pgtype string) string {
	t := jsontext.Token(v)
	switch t.Kind() {
	case jsontext.KindNull:
		return "NULL"
	case jsontext.KindString:
		switch pgtype {
		case "bytea":
			// wal2json simply prints a hex string. PG wants the \x prefix.
			return `'\x`+t.String()+`'`
		default:
			return `'`+strings.ReplaceAll(t.String(), `'`, `''`)+`'`
		}
	case jsontext.KindNumber:
		return `'`+strings.ReplaceAll(t.String(), `'`, `''`)+`'`
	case jsontext.KindTrue:
		return "TRUE"
	case jsontext.KindFalse:
		return "FALSE"
	}
	panic(fmt.Sprintf("unexpected VAL(kind: %q, string: %q)",
		t.Kind(), t.String()))
}

// Local Variables:
// tab-width: 4
// End:
