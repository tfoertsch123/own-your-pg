package msg

import (
	"testing"
	"github.com/tfoertsch123/own-your-pg/lsn"
)

func TestC(t *testing.T) {
	c := NewC()
	l, _ := lsn.ParseLSN("12/13")
	nl, _ := lsn.ParseLSN("12/113")
	xid := uint32(4129)
	ts := "2026-12-32 10:42:56.123456"
	c.Lsn = l
	c.NextLsn = nl
	c.Xid = &xid
	c.Timestamp = &ts

	t.Run("ToSQL", func(t *testing.T) {
		it := Common(c)
		if res := it.ToSQL(); res != "COMMIT" {
			t.Errorf("C.ToSQL: exp %q got %q", "COMMIT", res)
		}
	})

	t.Run("json", func(t *testing.T) {
		it := Common(c)

		js := `{"action":"C","xid":4129,`+
			`"timestamp":"2026-12-32 10:42:56.123456",`+
			`"lsn":"12/13","nextlsn":"12/113"}`
		if res := it.AsJSON(); res != js {
			t.Errorf("AsJSON exp <%v>, got <%v>", js, res)
		}

		out, err := Parse([]byte(js))
		if err != nil {
			t.Fatalf("Parse: got err: %v", err)
		}

		if !it.IsEqualTo(out) {
			t.Errorf("IsEqualTo: exp <%v> got <%v>", it, out)
		}

		out.(*C).NextLsn = l
		if it.IsEqualTo(out) {
			t.Errorf("IsEqualTo: exp <%v> got <%v>", it, out)
		}
	})
}

// Local Variables:
// tab-width: 4
// End:
