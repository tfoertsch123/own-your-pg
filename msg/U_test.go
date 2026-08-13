package msg

import (
	"testing"
	"encoding/json/jsontext"
	"github.com/tfoertsch123/own-your-pg/lsn"
	"github.com/tfoertsch123/own-your-pg/jval"
)

func TestU(t *testing.T) {
	c := NewU()
	l, _ := lsn.ParseLSN("12/13")
	xid := uint32(4129)
	ts := "2026-12-32 10:42:56.123456"
	c.Lsn = l
	c.Xid = &xid
	c.Timestamp = &ts
	c.Schema = "sch"
	c.Table = "tb"
	// out of the identity columns, only c2 changes. So, only it is included
	// in the update list.
	c.DecodedIdentity = []COL{
		{Name: "c1", Type: "text", Value: jval.Val(jsontext.String("txt"))},
		{Name: "c2", Type: "bigint", Value: jval.Val(jsontext.Int(123))},
	}
	c.DecodedColumns = []COL{
		{Name: "c1", Type: "text", Value: jval.Val(jsontext.String("txt"))},
		{Name: "c2", Type: "bigint", Value: jval.Val(jsontext.Int(124))},
		{Name: "p\"1", Type: "text", Value: jval.Val(jsontext.String("pl'1"))},
		{Name: "p\"2", Type: "text", Value: jval.Val(jsontext.String("pl'2"))},
	}

	t.Run("ToSQL", func(t *testing.T) {
		it := Common(c)

		sql := `WITH x AS (`+
			`SELECT ctid FROM "sch"."tb" `+
			`WHERE "c1" = 'txt'::text AND "c2" = '123'::bigint `+
			`LIMIT 1 FOR UPDATE`+
			`), u AS (`+
			`UPDATE "sch"."tb" AS d SET ("c2", "p""1", "p""2") = `+
			`row('124'::bigint, 'pl''1'::text, 'pl''2'::text) `+
			`FROM x WHERE d.ctid=x.ctid RETURNING 1`+
			`) SELECT 1/CASE WHEN count(*)=1 THEN 1 ELSE 0 END FROM u`
		if res := it.ToSQL(); res != sql {
			t.Errorf("C.ToSQL: exp <%v> got <%v>", sql, res)
		}

		// if there are no changes, the resulting SQL just needs to test if
		// the row exists.
		tmp := c.DecodedColumns
		c.DecodedColumns = c.DecodedIdentity

		sql = `SELECT 1/CASE WHEN exists(`+
			`SELECT 1 FROM "sch"."tb" `+
			`WHERE "c1" = 'txt'::text AND "c2" = '123'::bigint`+
			`) THEN 1 ELSE 0 END`
		if res := it.ToSQL(); res != sql {
			t.Errorf("C.ToSQL: exp <%v> got <%v>", sql, res)
		}
		
		c.DecodedColumns = tmp
	})

	t.Run("json", func(t *testing.T) {
		it := Common(c)

		js := `{"action":"U","xid":4129,`+
			`"timestamp":"2026-12-32 10:42:56.123456","lsn":"12/13",`+
			`"schema":"sch","table":"tb",`+
			`"columns":[{"name":"c1","type":"text","value":"txt"},`+
			`{"name":"c2","type":"bigint","value":124},`+
			`{"name":"p\"1","type":"text","value":"pl'1"},`+
			`{"name":"p\"2","type":"text","value":"pl'2"}],`+
			`"identity":[{"name":"c1","type":"text","value":"txt"},`+
			`{"name":"c2","type":"bigint","value":123}]}`
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
