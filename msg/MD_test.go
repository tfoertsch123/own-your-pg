package msg

import (
	"testing"
	"encoding/json/jsontext"
	"github.com/tfoertsch123/own-your-pg/lsn"
	"github.com/tfoertsch123/own-your-pg/jval"
)

func TestMD(t *testing.T) {
	c := NewMD()
	l, _ := lsn.ParseLSN("12/13")
	xid := uint32(4129)
	ts := "2026-12-32 10:42:56.123456"
	c.Lsn = l
	c.Xid = &xid
	c.Timestamp = &ts
	c.Schema = "sch"
	c.Table = "tb"
	c.DecodedIdentities = [][]COL{
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

		sql := `WITH /* MD 2 rows */ list("c1", "c2") AS (
    VALUES ('txt'::text, '123'::bigint),
           ('abc'::text, '999'::bigint)
)
, grps AS (
    SELECT "c1", "c2", COUNT(*) AS "_-:cnt:-_"
      FROM list
     GROUP BY "c1", "c2"
)
, lck AS (
    SELECT tb.ctid
         , grps."_-:cnt:-_"
         , tb."c1", tb."c2"
      FROM "sch"."tb" AS tb
      JOIN grps ON (tb."c1" = grps."c1") AND (tb."c2" = grps."c2")
       FOR UPDATE OF tb
)
 , num AS (
    SELECT ctid
         , "_-:cnt:-_"
         , ROW_NUMBER() OVER (PARTITION BY "c1", "c2") AS "_-:rn:-_"
      FROM lck
)
 , del AS (
    DELETE FROM "sch"."tb" AS tb
     USING num
     WHERE tb.ctid = num.ctid
       AND num."_-:rn:-_" <= num."_-:cnt:-_"
    RETURNING *
)
SELECT 1/((SELECT count(*) FROM list)=
          (SELECT count(*) FROM del))::INT`
		if res := it.ToSQL(); res != sql {
			t.Errorf("C.ToSQL: exp <%v> got <%v>", sql, res)
		}
	})

	t.Run("json", func(t *testing.T) {
		it := Common(c)

		js := `{"action":"MD","xid":4129,`+
			`"timestamp":"2026-12-32 10:42:56.123456","lsn":"12/13",`+
			`"schema":"sch","table":"tb",`+
			`"identities":[[{"name":"c1","type":"text","value":"txt"},`+
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
