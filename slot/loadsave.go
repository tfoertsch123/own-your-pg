package slot

import (
	"io"
	"fmt"
	"errors"
 	us "unsafe"

	// "github.com/tfoertsch123/log"
)

var ErrInit error = errors.New("Slot initialized but read-only")
var ErrMagicVersion error = errors.New("Slot magic or version mismatch")

// readPart() returns io.ErrUnexpectedEOF if EOF has been found while
// the buffer was not fully read. Otherwise, it is the same as file.ReadAt()
func (sl *Slot) readPart(b []byte, where int64) (int, error) {
	n, err := sl.fh.ReadAt(b, where)
	if n > 0 && err == io.EOF {
		err = io.ErrUnexpectedEOF
	}

	return n, err
}

func (sl *Slot) writePart(b []byte, where int64) (int, error) {
	return sl.fh.WriteAt(b, where)
}

func (sl *Slot) loadMagic() (Magic, error) {
	var m Magic
	mslice := us.Slice((*byte)(us.Pointer(&m)), us.Sizeof(m))

	_, err := sl.readPart(mslice, 0)

	return m, err
}

func (sl *Slot) loadHeader() error {
	if err := sl.lockHeaderSh(); err != nil {
		return err
	}
	defer sl.unlockHeader()

	hdrpos := int64(us.Sizeof(__mag__))
	hdrslice := us.Slice((*byte)(us.Pointer(&sl.header)), us.Sizeof(sl.header))

	slt := sl.SlotType
	_, err := sl.readPart(hdrslice, hdrpos)
	if err != nil {
		return err
	}

	// check slot type
	if slt != Any {
		if slt != sl.SlotType {
			return ErrInvalidSlotType
		}
	} else {
		if err = sl.SlotType.Scan(uint8(sl.SlotType)); err != nil {
			return ErrInvalidSlotType
		}
	}

	return nil
}

func (sl *Slot) loadCfg() error {
	if err := sl.lockCfgSh(); err != nil {
		return err
	}
	defer sl.unlockCfg()

	cfgpos := int64(us.Sizeof(__mag__)) + int64(us.Sizeof(sl.header))
	eofpos, err := sl.fh.Seek(0, 2)
	if err != nil {
		return err
	}

	if eofpos <= cfgpos {
		sl.Config = map[string][]string{}
		return nil
	}

	bts := make([]byte, eofpos - cfgpos)

	_, err = sl.readPart(bts, cfgpos)
	if err != nil {
		return err
	}

	bts, err = (&sl.Cfg).UnmarshalMsg(bts)
	if err != nil {
		return err
	}

	return nil
}

func (sl *Slot) load() error {
	m, err := sl.loadMagic()

	if err == io.EOF { // file is empty
		if !sl.writable {
			return fmt.Errorf(`slot %s: %w`, sl.name, ErrInit)
		}

		// init slot magic
		if err = sl.saveMagic(); err != nil {
			return fmt.Errorf(`slot %s: %w`, sl.name, err)
		}

		// init slot header
		if err = sl.saveHeader(); err != nil {
			return fmt.Errorf(`slot %s: %w`, sl.name, err)
		}

		// I guess, that's all for this case
		return nil
	}

	if err != nil {				// other read error
		return fmt.Errorf(`slot %s: %w`, sl.name, err)
	}

	// successfully read magic header -- check it
	if m != __mag__ {
		return fmt.Errorf(`slot %s: %w`, sl.name, ErrMagicVersion)
	}

	err = sl.loadHeader()
	if err != nil {
		return fmt.Errorf(`slot %s: %w`, sl.name, err)
	}

	err = sl.loadCfg()
	if err != nil {				// other read error
		return fmt.Errorf(`slot %s: %w`, sl.name, err)
	}

	return nil
}

func (sl *Slot) saveMagic() error {
	_, err := sl.writePart(__magbytes__, 0)
	return err
}

func (sl *Slot) _saveHeader() error {
	if err := sl.lockHeaderEx(); err != nil {
		return err
	}
	defer sl.unlockHeader()

	hdrpos := int64(us.Sizeof(__mag__))
	hdrslice := us.Slice((*byte)(us.Pointer(&sl.header)), us.Sizeof(sl.header))
	_, err := sl.writePart(hdrslice, hdrpos)
	return err
}

func (sl *Slot) saveHeader() error {
	// range check
	if err := sl.SlotType.Scan(uint8(sl.SlotType)); err != nil {
		return ErrInvalidSlotType
	}

	return sl._saveHeader()
}

func (sl *Slot) saveCfg() error {
	var bts []byte
	var err error

	if len(sl.Config) > 0 {
		// this call cannot fail other than OOM
		bts, _ = (&sl.Cfg).MarshalMsg(nil)
	}

	if err = sl.lockCfgEx(); err != nil {
		return err
	}
	defer sl.unlockCfg()

	cfgpos := int64(us.Sizeof(__mag__)) + int64(us.Sizeof(sl.header))
	eofpos := cfgpos

	if len(bts) > 0 {
		_, err = sl.writePart(bts, cfgpos)
		if err != nil {
			return err
		}

		eofpos += int64(len(bts))
	}

	return sl.fh.Truncate(eofpos)
}

func (sl *Slot) save() error {
	// no need to save magic. Magic has been initialized when the slot was
	// created and checked when it was opened.

	if err := sl.saveHeader(); err != nil {
		return fmt.Errorf(`slot %s: %w`, sl.name, err)
	}

	if err := sl.saveCfg(); err != nil {
		return fmt.Errorf(`slot %s: %w`, sl.name, err)
	}
	
	return nil
}

// Local Variables:
// tab-width: 4
// End:
