package slot

import (
 	us "unsafe"
	"fmt"
)

const myVersion = 1				// current version
const myMagic = "OYPG"
const fext = ".oypg"

type Magic uint64
var  __mag__ Magic
var  __magbytes__ []byte

func init() {
	__magbytes__ = us.Slice((*byte)(us.Pointer(&__mag__)), us.Sizeof(__mag__))

	copy(__magbytes__[:4], []byte(myMagic))

	v := uint32(myVersion)
	copy(__magbytes__[4:], us.Slice((*byte)(us.Pointer(&v)), us.Sizeof(v)))
}

func (m Magic) Parts() (string, uint32) {
	ms := us.Slice((*byte)(us.Pointer(&m)), us.Sizeof(m))
	var v uint32
	vs := us.Slice((*byte)(us.Pointer(&v)), us.Sizeof(v))
	copy(vs, ms[4:])
	return string(ms[:4]), v
}

func (m Magic) String() string {
	c, v := m.Parts()
	return fmt.Sprintf(`%sv%d`, c, v)
}

// Local Variables:
// tab-width: 4
// End:
