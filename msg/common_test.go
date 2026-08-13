package msg

import (
	"testing"
)

func TestCommon(t *testing.T) {
	t.Run("Invalid Action", func(t *testing.T) {
		js := `{"action":"`+"\xff"+`","xid":4129,`+
			`"timestamp":"2026-12-32 10:42:56.123456","lsn":"12/13"}`
		out, err := Parse([]byte(js))
		if err == nil {
			t.Errorf("Parse: got err: %v", err)
		}
		if out != nil {
			t.Errorf("Parse: got object: %v", out)
		}
	})
	for _, typ := range []string{
		"A", "B", "C", "D", "MD", "I", "MI", "M", "S", "T", "U", "Z",
	} {
		t.Run("Invalid "+typ, func(t *testing.T) {
			js := `{"action":"`+typ+`","xid":4129,`+
				`"timestamp":"2026-12-32 10:42:56.123456","lsn":"x12/13"}`
			out, err := Parse([]byte(js))
			if err == nil {
				t.Errorf("Parse: got err: %v", err)
			}
			if out != nil {
				t.Errorf("Parse: got object: %v", out)
			}
		})
	}
}

// Local Variables:
// tab-width: 4
// End:
