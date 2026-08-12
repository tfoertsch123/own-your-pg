package msg

import (
	"testing"
	"github.com/tfoertsch123/own-your-pg/lsn"
)

func TestA(t *testing.T) {
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
		it := Common(NewA())
		if res := it.ToSQL(); res != "ABORT" {
			t.Errorf("A.ToSQL: exp %q got %q", "ABORT", res)
		}
	})

	t.Run("NewFromC", func(t *testing.T) {
		it := Common(NewAFromC(c))

		_, ok := it.(*A)
		if !ok {
			t.Error("not a *A")
		}
		if it.GetAction() != "A" {
			t.Error("GetAction is not A")
		}
	})

	t.Run("json", func(t *testing.T) {
		it := Common(NewAFromC(c))

		js := `{"action":"A","xid":4129,`+
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

		if c.IsEqualTo(out) {
			t.Errorf("IsEqualTo: exp <%v> got <%v>", c, out)
		}

		out.(*A).NextLsn = l
		if it.IsEqualTo(out) {
			t.Errorf("IsEqualTo: exp <%v> got <%v>", it, out)
		}
	})
}

// Local Variables:
// mode: Go
// tab-width: 4
// End:
