package slot

import (
	"testing"
	"io"
	"os"
	"maps"
	"slices"
	"errors"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/tfoertsch123/flock"
	"github.com/tfoertsch123/own-your-pg/lsn"
)

func TestLoadSave(t *testing.T) {
	dir := t.TempDir()

	sl := &Slot{
		header: header{
			SlotType: Change,
			OwnerPid: 1234,
			NextLSN: lsn.LSN(0xe0000abcd),
		},
		lck: flock.New(
			flock.WithPath(filepath.Join(dir, "lck")),
			flock.WithRdWr(),
			flock.WithCreate(unix.S_IRUSR, unix.S_IWUSR),
		),
		writable: true,
	}
	if err := sl.lck.Open(); err != nil {
		t.Fatalf("flock.Open(%v): %v", sl.lck.Path(), err)
	}
	sl.fh = sl.lck.File()
	defer sl.fh.Close()

	slro := &Slot{
		header: header{
			SlotType: Any,
		},
		lck: flock.New(flock.WithPath(sl.lck.Path())),
	}
	if err := slro.lck.Open(); err != nil {
		t.Fatalf("flock.Open(%v): %v", slro.lck.Path(), err)
	}
	slro.fh = slro.lck.File()
	defer slro.fh.Close()

	t.Run("magic", func(t *testing.T) {
		if _, err := slro.loadMagic(); err != io.EOF {
			t.Error("expecting io.EOF")
		}

		if _, err := sl.fh.WriteAt([]byte("x"), 0); err != nil {
			t.Fatalf("write %v", err)
		}

		if _, err := slro.loadMagic(); err != io.ErrUnexpectedEOF {
			t.Errorf("expecting io.ErrUnexpectedEOF, got %v", err)
		}

		if err := sl.saveMagic(); err != nil {
			t.Fatalf("saveMagic %v", err)
		}

		if mag, err := slro.loadMagic(); err != nil {
			t.Errorf("unexpected error: %v", err)
		} else {
			if mag != __mag__ {
				t.Errorf("exp <%v>, got <%v>", __mag__, mag)
			}
		}
	})

	t.Run("header", func(t *testing.T) {
		defer func(hdr header) {slro.header = hdr}(slro.header)
		defer func(hdr header) {sl.header = hdr}(sl.header)
		if err := sl.fh.Truncate(0); err != nil {
			t.Fatalf("Truncate %v", err)
		}

		if err := sl.saveMagic(); err != nil {
			t.Fatalf("saveMagic %v", err)
		}

		if err := slro.loadHeader(); err != io.EOF {
			t.Error("expecting io.EOF")
		}

		if _, err := sl.fh.WriteAt([]byte("x"), int64(len(__magbytes__)));
		   err != nil {
			t.Fatalf("write %v", err)
		}

		if err := slro.loadHeader(); err != io.ErrUnexpectedEOF {
			t.Errorf("expecting io.ErrUnexpectedEOF, got %v", err)
		}

		if err := sl.saveHeader(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := slro.loadHeader(); err != nil {
			t.Errorf("unexpected error: %v", err)
		} else {
			if slro.header != sl.header {
				t.Errorf("exp <%v>, got <%v>", sl.header, slro.header)
			}
		}

		slro.SlotType = Archiver
		if err := slro.loadHeader(); err != ErrInvalidSlotType {
			t.Errorf("unexpected error: %v", err)
		}

		tmp := sl.SlotType
		sl.SlotType = Type(0xff)
		if err := sl.saveHeader(); err != ErrInvalidSlotType {
			t.Errorf("unexpected error: %v", err)
		}
		if err := sl._saveHeader(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		sl.SlotType = tmp

		slro.SlotType = Any
		if err := slro.loadHeader(); err != ErrInvalidSlotType {
			t.Errorf("unexpected error: %v", err)
		}

		t.Run("lockSh", func(t *testing.T) {
			defer func(lck *flock.Lock) { slro.lck = lck }(slro.lck)
			slro.lck = flock.New(flock.WithPath(sl.lck.Path()))
			slro.lck.Close()

			if err := slro.loadHeader();
			   !errors.Is(err, flock.ErrAlreadyClosed) {
				t.Errorf("unexpected error: %v", err)
			}
		})
	})

	t.Run("cfg", func(t *testing.T) {
		defer func(cfg Cfg) {slro.Cfg = cfg}(slro.Cfg)
		defer func(cfg Cfg) {sl.Cfg = cfg}(sl.Cfg)
		if err := sl.fh.Truncate(0); err != nil {
			t.Fatalf("Truncate %v", err)
		}

		if err := sl.saveHeader(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		slro.Config = map[string][]string{"a": []string{"b", "c"}}
		if err := slro.loadCfg(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(slro.Config) != 0 || slro.Config == nil {
			t.Errorf("unexpected config: %v", slro.Config)
		}

		sl.Config = map[string][]string{}
		sl.SetConfig("abc", []string{"123", "456"})
		sl.SetConfig("ABC", []string{"x23", "x56"})
		if err := sl.saveCfg(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := slro.loadCfg(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !maps.EqualFunc(slro.Config, sl.Config, slices.Equal[[]string]) {
			t.Errorf("unexpected config: %v", slro.Config)
		}

		slro.fh = func (fh *os.File) *os.File {
			r, w, err := os.Pipe()
			slro.fh = r
			if err != nil {
				t.Errorf("could not create pipe: %v", err)
			}
			defer r.Close()
			defer w.Close()

			if err = slro.loadCfg(); !errors.Is(err, unix.ESPIPE) {
				t.Errorf("expecting EBADF: %v", err)
			}
			return fh
		}(slro.fh)

		if err := sl.fh.Truncate(0); err != nil {
			t.Fatalf("Truncate %v", err)
		}

		if err := sl.saveHeader(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// append a bit of garbage
		if cfgpos, err := sl.fh.Seek(0, 2); err != nil {
			t.Fatalf("unexpected error: %v", err)
		} else {
			if _, err := sl.fh.WriteAt([]byte("x"), cfgpos); err != nil {
				t.Fatalf("write %v", err)
			}
		}

		// the error is triggered by the attempt of msgp decoding garbage.
		// Is should start with `msgp:`.
		if err := slro.loadCfg(); err == nil || err.Error()[0:5] != "msgp:" {
			t.Error("expecting msgp error")
		}

		if err := slro.saveCfg(); !errors.Is(err, unix.EBADF) {
			t.Errorf("expecting EBADF, got %v", err)
		}

		t.Run("lockSh", func(t *testing.T) {
			defer func(lck *flock.Lock) { slro.lck = lck }(slro.lck)
			slro.lck = flock.New(flock.WithPath(sl.lck.Path()))
			slro.lck.Close()

			if err := slro.loadCfg();
			   !errors.Is(err, flock.ErrAlreadyClosed) {
				t.Errorf("unexpected error: %v", err)
			}
		})
	})

	t.Run("save", func(t *testing.T) {
		defer func(hdr header) {slro.header = hdr}(slro.header)
		defer func(hdr header) {sl.header = hdr}(sl.header)
		defer func(cfg Cfg) {slro.Cfg = cfg}(slro.Cfg)
		defer func(cfg Cfg) {sl.Cfg = cfg}(sl.Cfg)
		if err := sl.fh.Truncate(0); err != nil {
			t.Fatalf("Truncate %v", err)
		}

		// save() does not write Magic. So, after this the magic number
		// should still be wrong.
		if err := sl.save(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := slro.load(); !errors.Is(err, ErrMagicVersion) {
			t.Errorf("unexpected error: %v", err)
		}

		if err := sl.saveMagic(); err != nil {
			t.Fatalf("saveMagic %v", err)
		}

		// slro has SlotType=Any. So, it accepts any type.
		if err := slro.load(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if err := slro.save(); !errors.Is(err, unix.EBADF) {
			t.Errorf("unexpected error: %v", err)
		}

		t.Run("write error in saveCfg()", func(t *testing.T) {
			defer func(fh *os.File) { sl.fh = fh }(sl.fh)
			defer func(cfg Cfg) { sl.Cfg = cfg }(sl.Cfg)

			// In saveCfg we call writePart() after locking the config.
			// This test exercises what happens when that write fails.
			// In order to trigger an error, we replace the actual lock
			// file handle with fifo (named pipe). This supports locking
			// but does not support the seek happening in writePart().
			fifoPath := filepath.Join(dir, "fifo")
			if err := unix.Mkfifo(fifoPath, 0666); err != nil {
				t.Fatalf("Mkfifo %v", err)
			}

			fifo, err := os.OpenFile(fifoPath, os.O_RDWR, 0)
			if err != nil {
				t.Fatalf("OpenFile %v", err)
			}
			defer fifo.Close()
			sl.fh = fifo

			sl.Config = map[string][]string{}
			sl.SetConfig("k", []string{"v"})

			if err := sl.saveCfg(); !errors.Is(err, unix.ESPIPE) {
				t.Errorf("saveCfg: expecting ESPIPE, got %v", err)
			}
		})

		t.Run("saveCfg error in save()", func(t *testing.T) {
			defer func(fh *os.File) { sl.fh = fh }(sl.fh)
			defer func(cfg Cfg) { sl.Cfg = cfg }(sl.Cfg)

			// In save() we first call saveHeader() followed by saveCfg().
			// Both saveHeader() and saveCfg() write to the file. So, if
			// the saveHeader() succeeds, saveCfg() is very likely also
			// going to succeed. The difference between the 2 is that
			// saveCfg() calls truncate(2) in order to adjust the file
			// size after writing. Truncate is not supported by /dev/null
			// So, if we replace the file with /dev/null, we can lock and
			// write and only the truncate operation at the very end fails.
			dn, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
			if err != nil {
				t.Fatalf("OpenFile /dev/null %v", err)
			}
			defer dn.Close()
			sl.fh = dn

			sl.Config = map[string][]string{}
			sl.SetConfig("k", []string{"v"})

			if err := sl.save(); !errors.Is(err, unix.EINVAL) {
				t.Errorf("save: expecting EINVAL, got %v", err)
			}
		})
	})

	t.Run("load", func(t *testing.T) {
		defer func(hdr header) {slro.header = hdr}(slro.header)
		defer func(hdr header) {sl.header = hdr}(sl.header)
		defer func(cfg Cfg) {slro.Cfg = cfg}(slro.Cfg)
		defer func(cfg Cfg) {sl.Cfg = cfg}(sl.Cfg)

		// reset slot to empty
		if err := sl.fh.Truncate(0); err != nil {
			t.Fatalf("Truncate %v", err)
		}

		if err := slro.load(); !errors.Is(err, ErrInit) {
			t.Errorf("unexpected error: %v", err)
		}

		t.Run("saveCfg error in save()", func(t *testing.T) {
			defer func(x bool) { slro.writable = x }(slro.writable)
			slro.writable = true

			if err := slro.load(); !errors.Is(err, unix.EBADF) {
				t.Errorf("unexpected error: %v", err)
			}
		})

		// write garbage
		if _, err := sl.fh.WriteAt([]byte("x"), 0); err != nil {
			t.Fatalf("write %v", err)
		}

		if err := slro.load(); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("unexpected error: %v", err)
		}

		// reset slot to empty
		if err := sl.fh.Truncate(0); err != nil {
			t.Fatalf("Truncate %v", err)
		}

		// sl is open for writing. It will init magic and the header when
		// loaded. After that slro.load should succeed.
		if err := sl.load(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if err := slro.load(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// reset slot to empty
		if err := sl.fh.Truncate(0); err != nil {
			t.Fatalf("Truncate %v", err)
		}

		if err := sl.saveMagic(); err != nil {
			t.Fatalf("saveMagic %v", err)
		}

		// write garbage
		if _, err := sl.fh.WriteAt([]byte("x"), int64(len(__magbytes__)));
		   err != nil {
			t.Fatalf("write %v", err)
		}

		if err := slro.load(); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("unexpected error: %v", err)
		}

		// now write a correct header plus cfg garbage
		if err := sl.saveHeader(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// garbage
		if cfgpos, err := sl.fh.Seek(0, 2); err != nil {
			t.Fatalf("unexpected error: %v", err)
		} else {
			if _, err := sl.fh.WriteAt([]byte("x"), cfgpos); err != nil {
				t.Fatalf("write %v", err)
			}
		}

		// the error message should come from load() and be formatted as
		// `slot %s: %w`. The %s is the name which is an empty string for
		// slro. So, the message should start with `slot : ...`. The `...`
		// part is the underlying error message. This should come from
		// msgp decoding. So, it starts with `msgp:`.
		if err := slro.load(); err == nil || err.Error()[7:12] != "msgp:" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// Local Variables:
// tab-width: 4
// End:
