package msg

import (
	"testing"
	"encoding/json/jsontext"
	"github.com/tfoertsch123/own-your-pg/lsn"
	"github.com/tfoertsch123/own-your-pg/jval"
)

func TestMI(t *testing.T) {
	c := NewMI()
	l, _ := lsn.ParseLSN("12/13")
	xid := uint32(4129)
	ts := "2026-12-32 10:42:56.123456"
	c.Lsn = l
	c.Xid = &xid
	c.Timestamp = &ts
	c.Schema = "sch"
	c.Table = "tb"
	c.DecodedRows = [][]COL{
		[]COL{
			{Name: "c1", Type: "text", Value: jval.Val(jsontext.String("txt"))},
			{Name: "c2", Type: "bigint", Value: jval.Val(jsontext.Int(123))},
		},
		[]COL{
			{Name: "c1", Type: "text", Value: jval.Val(jsontext.String("abc"))},
			{Name: "c2", Type: "bigint", Value: jval.Val(jsontext.Int(999))},
		},
	}

	t.Run("ToSQL", func(t *testing.T) {
		it := Common(c)

		sql := `INSERT /* MI 2 rows*/ INTO "sch"."tb"("c1", "c2") OVERRIDING SYSTEM VALUE VALUES
    ('txt'::text, '123'::bigint),
    ('abc'::text, '999'::bigint)`
		if res := it.ToSQL(); res != sql {
			t.Errorf("C.ToSQL: exp <%v> got <%v>", sql, res)
		}
	})

	t.Run("json", func(t *testing.T) {
		it := Common(c)

		js := `{"action":"MI","xid":4129,`+
			`"timestamp":"2026-12-32 10:42:56.123456","lsn":"12/13",`+
			`"schema":"sch","table":"tb",`+
			`"rows":[[{"name":"c1","type":"text","value":"txt"},`+
			`{"name":"c2","type":"bigint","value":123}],`+
			`[{"name":"c1","type":"text","value":"abc"},`+
			`{"name":"c2","type":"bigint","value":999}]]}`
		if res := it.AsJSON(); res != js {
			t.Errorf("AsJSON exp <%v>, got <%v>", js, res)
		}

		if res := it.String(); res != js {
			t.Errorf("String exp <%v>, got <%v>", js, res)
		}

		out, err := Parse([]byte(js))
		if err != nil {
			t.Fatalf("Parse: got err: %v", err)
		}

		if !it.IsEqualTo(out) {
			t.Errorf("IsEqualTo: exp <%v> got <%v>", it, out)
		}
	})
}

// Local Variables:
// tab-width: 4
// End:
