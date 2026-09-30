package msg

import (
	"testing"
	"github.com/tfoertsch123/own-your-pg/lsn"
)

func TestZ(t *testing.T) {
	c := NewZ()
	l, _ := lsn.ParseLSN("12/13")
	xid := uint32(4129)
	ts := "2026-12-32 10:42:56.123456"
	c.Lsn = l
	c.Xid = &xid
	c.Timestamp = &ts

	t.Run("ToSQL", func(t *testing.T) {
		it := Common(c)

		sql := `/* EOF */`
		if res := it.ToSQL(); res != sql {
			t.Errorf("C.ToSQL: exp <%v> got <%v>", sql, res)
		}
	})

	t.Run("json", func(t *testing.T) {
		it := Common(c)

		js := `{"action":"Z","xid":4129,`+
			`"timestamp":"2026-12-32 10:42:56.123456","lsn":"12/13"}`
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
