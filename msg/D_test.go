package msg

import (
	"testing"
	"encoding/json/jsontext"
	"github.com/tfoertsch123/own-your-pg/lsn"
	"github.com/tfoertsch123/own-your-pg/jval"
)

func TestD(t *testing.T) {
	c := NewD()
	l, _ := lsn.ParseLSN("12/13")
	xid := uint32(4129)
	ts := "2026-12-32 10:42:56.123456"
	c.Lsn = l
	c.Xid = &xid
	c.Timestamp = &ts
	c.Schema = "sch"
	c.Table = "tb"
	c.DecodedIdentity = []COL{
		{Name: "col1", Type: "text", Value: jval.Val(jsontext.String("txt"))},
		{Name: "col2", Type: "bigint", Value: jval.Val(jsontext.Null)},
	}

	t.Run("ToSQL", func(t *testing.T) {
		it := Common(c)

		sql := `WITH x AS (`+
			`SELECT ctid FROM "sch"."tb" `+
			`WHERE "col1" = 'txt'::text AND "col2" IS NULL `+
			`LIMIT 1 FOR UPDATE`+
			`), d AS (`+
			`DELETE FROM "sch"."tb" AS d USING x `+
			`WHERE d.ctid=x.ctid RETURNING 1) `+
			`SELECT 1/CASE WHEN count(*)=1 THEN 1 ELSE 0 END FROM d`
		if res := it.ToSQL(); res != sql {
			t.Errorf("C.ToSQL: exp <%v> got <%v>", sql, res)
		}
	})

	t.Run("json", func(t *testing.T) {
		it := Common(c)

		js := `{"action":"D","xid":4129,`+
			`"timestamp":"2026-12-32 10:42:56.123456","lsn":"12/13",`+
			`"schema":"sch","table":"tb",`+
			`"identity":[{"name":"col1","type":"text","value":"txt"},`+
			`{"name":"col2","type":"bigint","value":null}]}`
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
