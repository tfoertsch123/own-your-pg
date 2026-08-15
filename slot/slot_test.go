package slot

import (
	"testing"
	// "io"
	// "os"
	// "maps"
	// "slices"
	// "errors"
	// "path/filepath"

	// "golang.org/x/sys/unix"

	// "github.com/tfoertsch123/flock"
	// "github.com/tfoertsch123/own-your-pg/lsn"
)

func TestSlot(t *testing.T) {
	t.Run("slotopt", func(t *testing.T) {
		slo := slotopts{}
		WithCreate()(&slo)

		exp := slotopts{create: true, writable: true}
		if slo != exp {
			t.Error("WithCreate")
		}

		slo = slotopts{}
		WithWrite()(&slo)

		exp = slotopts{writable: true}
		if slo != exp {
			t.Error("WithWrite")
		}

		slo = slotopts{}
		WithAsOwner()(&slo)

		exp = slotopts{create: true, writable: true, asowner: true}
		if slo != exp {
			t.Error("WithAsOwner")
		}

		// NoPidLock is the same as AsOwner except that the pid is not
		// locked. It is used by slottool.
		slo = slotopts{}
		WithNoPidLock()(&slo)

		exp = slotopts{create:true, writable:true, asowner:true, nopidlck:true}
		if slo != exp {
			t.Error("WithNoPidLock")
		}

		slo = slotopts{}
		WithType(Change)(&slo)

		exp = slotopts{typ: Change}
		if slo != exp {
			t.Error("WithType")
		}
	})
}

// Local Variables:
// tab-width: 4
// End:
