package slot

import (
	"testing"
	"strconv"
	us "unsafe"
)

func TestMagic(t *testing.T) {
	if c, v := __mag__.Parts(); c != myMagic || v != myVersion {
		t.Errorf("__mag__.Parts(): got c=%v, v=%v", c, v)
	}

	if s := __mag__.String(); s != myMagic+"v"+strconv.Itoa(myVersion) {
		t.Errorf("__mag__.String(): got %v", s)
	}

	var x Magic
	copy(
		us.Slice((*byte)(us.Pointer(&x)), us.Sizeof(x)),
		[]byte{'A', 'B', 'C', 'D', 0xA5, 0xA5, 0xA5, 0xA5},
	)

	if c, v := x.Parts(); c != "ABCD" || v != 0xA5A5A5A5 {
		t.Errorf("x.Parts(): got c=%v, v=%v", c, v)
	}
}

// Local Variables:
// tab-width: 4
// End:
