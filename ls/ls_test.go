package ls

import (
	"testing"
	// "os"
	// "regexp"

	"golang.org/x/sys/unix"
)

func TestLs_ok(t *testing.T) {
	dir := t.TempDir()

	exp := map[string]uint8{
		".": unix.DT_DIR,
		"..": unix.DT_DIR,
	}
	for item, _ := range Ls(dir, nil) {
		exp_type, ok := exp[item.Name]
		if !ok {
			t.Errorf("got unexpected dirent %#v", item)
			continue
		}
		if exp_type != item.Type {
			t.Errorf("dirent %#v: expected type %v", item, exp_type)
		}
		delete(exp, item.Name)
	}
	if len(exp) > 0 {
		var list []string
		for k, _ := range exp {
			list = append(list, k)
		}
		t.Errorf("missing items: %v", list)
	}
}

func TestLs_usrbin(t *testing.T) {
	for item, _ := range Ls("/usr/bin", nil) {
		t.Logf("%#v", item)
	}
}

// Local Variables:
// tab-width: 4
// End:
