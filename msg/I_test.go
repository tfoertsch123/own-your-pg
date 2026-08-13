package msg

import (
	"testing"
	"encoding/json/jsontext"
	"github.com/tfoertsch123/own-your-pg/lsn"
	"github.com/tfoertsch123/own-your-pg/jval"
)

func TestI(t *testing.T) {
	c := NewI()
	l, _ := lsn.ParseLSN("12/13")
	xid := uint32(4129)
	ts := "2026-12-32 10:42:56.123456"
	c.Lsn = l
	c.Xid = &xid
	c.Timestamp = &ts
	c.Schema = "sch"
	c.Table = "tb"
	c.DecodedColumns = []COL{
		{Name: "c1", Type: "text", Value: jval.Val(jsontext.String("txt"))},
		{Name: "c2", Type: "bigint", Value: jval.Val(jsontext.Int(123))},
		{Name: "p\"1", Type: "text", Value: jval.Val(jsontext.String("pl'1"))},
		{Name: "p\"2", Type: "text", Value: jval.Val(jsontext.String("pl'2"))},
	}

	t.Run("ToSQL", func(t *testing.T) {
		it := Common(c)

		sql := `INSERT INTO "sch"."tb"("c1", "c2", "p""1", "p""2") `+
			`OVERRIDING SYSTEM VALUE VALUES `+
			`('txt'::text, '123'::bigint, 'pl''1'::text, 'pl''2'::text)`
		if res := it.ToSQL(); res != sql {
			t.Errorf("C.ToSQL: exp <%v> got <%v>", sql, res)
		}
	})

	t.Run("json", func(t *testing.T) {
		it := Common(c)

		js := `{"action":"I","xid":4129,`+
			`"timestamp":"2026-12-32 10:42:56.123456","lsn":"12/13",`+
			`"schema":"sch","table":"tb",`+
			`"columns":[{"name":"c1","type":"text","value":"txt"},`+
			`{"name":"c2","type":"bigint","value":123},`+
			`{"name":"p\"1","type":"text","value":"pl'1"},`+
			`{"name":"p\"2","type":"text","value":"pl'2"}]}`
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
// mode: Go
// tab-width: 4
// End:
