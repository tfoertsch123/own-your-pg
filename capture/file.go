package capture

import (
	"golang.org/x/sys/unix"
	"os"
	"io"
	"fmt"
	"errors"
	"path/filepath"
	"bytes"
	"encoding/binary"
	"encoding/hex"

	"github.com/tfoertsch123/log"
	"github.com/tfoertsch123/own-your-pg/defaults"
	mylsn "github.com/tfoertsch123/own-your-pg/lsn"
)

// We perform a file switch at COMMIT. That means a file can begin with
// a BEGIN or a non-transactional MESSAGE record.
// The file name starts with the expanded LSN of the last COMMIT record
// in the file. Naturally, that is the very last record. The 2nd part of
// file name is the very first BEGIN LSN in the file. This does not have
// be the very first record in the file.
// This means a file must contain at least one complete transaction. A file
// can therefore also grow indefinitely (if it only gets non-transactional
// messages) or it can grow very big if a very big transaction is processed.

// ALL FILE PATHS ARE RELATIVE TO cfg.mgr.DirFd()

// make sure the incoming directory exists
// We can't use cfg.mlg here. It might not be initialized
func (cfg *Cfg) ensureIncDir() {
	err := unix.Mkdirat(
		cfg.mgr.DirFd(), filepath.Join("..", defaults.IncDir), 0777,
	)
	if err != nil && !errors.Is(err, unix.EEXIST) {
		log.Panicf("%s: %v", filepath.Join(cfg.wd, defaults.IncDir), err)
	}
}

// This record is located always at the beginning of the output file.
// The eof value holds the file position right after the last COMMIT
// record in the file. The header must be fixed length. This function
// is NOT thread-safe.
const _hdrdata = `{"C":"CCCCCCCCCCCCCCCC","L":"LLLLLLLLLLLLLLLL"}`+"\n"
const _hdrend = int64(len(_hdrdata))
const _hdrCpos int64 = 6		// start of the first C in _hdr
const _hdrClen int64 = 16		// number of C in _hdr
const _hdrLpos int64 = _hdrCpos+_hdrClen+7 // start of the first L in _hdr
const _hdrLlen int64 = 16		// number of L in _hdr
var _hdr []byte = []byte(_hdrdata)
func (cfg *Cfg) writeHeader(fh io.WriterAt, epos int64, flsn *mylsn.LSN) error {
	// fmt.Sprintf() would work too. But it would allocate a new string
	// every time. Instead we allocate a 8 bytes array and put the bytes
	// in the correct order (BigEndian) in that buffer. Then we hex-encode
	// the buffer directly into _hdr.
	var bts [8]byte
	binary.BigEndian.PutUint64(bts[:], uint64(epos))
	hex.Encode(_hdr[_hdrCpos:_hdrCpos+_hdrClen], bts[:])
	if flsn == nil {
		copy(_hdr[_hdrLpos:_hdrLpos+_hdrLlen], []byte(`LLLLLLLLLLLLLLLL`))
	} else {
		binary.BigEndian.PutUint64(bts[:], uint64(*flsn))
		hex.Encode(_hdr[_hdrLpos:_hdrLpos+_hdrLlen], bts[:])
	}
	_, err := fh.WriteAt(_hdr, 0)
	return err
}

// read the file header and return the eoCommit position
func (cfg *Cfg) readHeader(fh io.ReaderAt) (int64, *mylsn.LSN, error) {
	var buf [_hdrend]byte
	var bts [8]byte
	_, err := fh.ReadAt(buf[:], 0)
	if err != nil {
		return 0, nil, err
	}
	_, err = hex.Decode(bts[:], buf[_hdrCpos:_hdrCpos+_hdrClen])
	if err != nil {
		return 0, nil, err
	}
	eoc := int64(binary.BigEndian.Uint64(bts[:]))

	if string(buf[_hdrLpos:_hdrLpos+_hdrLlen]) == `LLLLLLLLLLLLLLLL` {
		cfg.mlg.Debugf("readHeader: eoc = %#x, flsn = %v", eoc, nil)
		return eoc, nil, nil
	}

	_, err = hex.Decode(bts[:], buf[_hdrLpos:_hdrLpos+_hdrLlen])
	if err != nil {
		return 0, nil, err
	}
	flsn := mylsn.LSN(binary.BigEndian.Uint64(bts[:]))

	cfg.mlg.Debugf("readHeader: eoc = %#x, flsn = %v", eoc, flsn)
	return eoc, &flsn, nil
}

var ErrCurrGarbage = errors.New("current file is garbage")

// create a new and empty incoming/current file
func (cfg *Cfg) newCur() error {
	fd, err := unix.Openat(
		cfg.mgr.DirFd(),
		filepath.Join("..", defaults.IncDir, defaults.CurFile),
		unix.O_RDWR | unix.O_CREAT,
		0666,
	)
	if err != nil {
		return err
	}

	fh := os.NewFile(
		uintptr(fd),
		filepath.Join(defaults.IncDir, defaults.CurFile),
	)
	defer func() {
		if fh != nil {
			fh.Close()
		}
	}()
	eof, err := fh.Seek(0, os.SEEK_END)
	if err != nil {
		return err
	}
	if eof == 0 {
		// empty file: write the header and return
		_, err = fh.Seek(int64(_hdrend), os.SEEK_SET)
		if err != nil {
			return err
		}
		err = cfg.writeHeader(fh, _hdrend, nil)
		if err != nil {
			return err
		}
		cfg.curr, fh = fh, nil
		cfg.firstTxnLSN = nil
		cfg.eoCommit = _hdrend
		return nil
	}

	// We get here only if the file exist. During normal rotation that
	// does not happen, only during initialization.

	// Find the position after the last commit in the file header.
	// If the file length (eof) is beyond that position, truncate the file.

	eoc, flsn, err := cfg.readHeader(fh)
	if err != nil {
		return err
	}

	if eoc > eof {				// somebody tampered with the file
		return ErrCurrGarbage
	}

	// if eoc == _hdrend but eof is ahead, then no 
	if eoc < eof {
		// cut off trailing garbage
		err = fh.Truncate(eoc)
		if err != nil {
			return err
		}
		_, err = fh.Seek(eoc, os.SEEK_SET)
		if err != nil {
			return err
		}
	}

	cfg.curr, fh = fh, nil
	cfg.firstTxnLSN = flsn
	cfg.eoCommit = eoc

	return nil
}

// to be called once during initialization
func (cfg *Cfg) curInit() {
	// create the first current file
	err := cfg.newCur()
	if err != nil {
		cfg.mlg.Panicf("%s: %v", filepath.Join(
			cfg.wd, defaults.IncDir, defaults.CurFile), err,
		)
	}

	// We allocate a 65kb buffer. But we implement a soft limit of 64kb.
	// If the buffer grows beyond 64kb, it is flushed.
	cfg.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))
}

func (cfg *Cfg) flush() (int64, error) {
	return cfg.writer.WriteTo(cfg.curr)
}

func (cfg *Cfg) writeData(data []byte) error {
	cfg.writer.Write(data)
	cfg.writer.WriteByte('\n')

	if cfg.writer.Len() > 64*1024 {
		if _, err := cfg.flush(); err != nil {
			return err
		}
	}

	return nil
}

// flush and sync the buffer. Adjust eoCommit and similar.
// To be called outside of a transaction only -- after COMMIT or
// a non-transactional message.
func (cfg *Cfg) eoc(lsn mylsn.LSN, writeBLSN bool) error {
	if cfg.firstTxnLSN == nil {
		cfg.firstTxnLSN = &lsn
	}
	if _, err := cfg.flush(); err != nil {
		return err
	}

	eof, err := cfg.curr.Seek(0, os.SEEK_END)
	if err != nil {
		return err
	}

	if writeBLSN {
		// cfg.eoCommit still points at the position after the previous
		// commit. That's our start position.
		_, err = cfg.curr.WriteAt(
			[]byte(fmt.Sprintf(
				`%-*s`,
				len(`"PPPPPPPP/QQQQQQQQ",`),
				`"` + lsn.String() + `",`,
			)),
			cfg.eoCommit+int64(len(`{"action":"B","nextlsn":`)))
		if err != nil {
			return err
		}
	}

	// cfg.firstTxnLSN is set in consume.go when the first B record is
	// consumed. Since C always comes after B it should not be nil here.
	err = cfg.writeHeader(cfg.curr, eof, cfg.firstTxnLSN)
	if err != nil {
		return err
	}

	err = cfg.curr.Sync()
	if err != nil {
		return err
	}

	cfg.eoCommit = eof

	err = cfg.sl.SetLSN(lsn, true)
	if err != nil {
		return err
	}

	if cfg.eoCommit > cfg.maxSize {
		err = cfg.rotateFile(lsn)
		if err != nil {
			return err
		}
	}

	return nil
}

// supposed to be called directly after flushing the buffer and syncing
// at COMMIT. No check is performed if there is data in the buffer. Even
// if there was something in the buffer, it would simply be written to the
// new file.
func (cfg *Cfg) rotateFile(endlsn mylsn.LSN) error {
	cfg.curr.Close()
	cfg.curr = nil

	// AWS S3 allows to start lost files from a specific anchor position.
	// All files with names greater than the anchor will be listed. If
	// our history file names start with the last LSN committed in the
	// file and I want to download the file that contains a specific LSN,
	// then I can simply give that specific LSN as anchor. The first file
	// that comes up in the listing is the one I need to download.
	//
	// That is the reason why our file names look like so:
	// LAST_COMMIT_LSN..FIRST_COMMIT_LSN
	//
	// If I then want to find the file that contains LAST_COMMIT_LSN,
	// I anchor the listing at that LSN. Since LAST_COMMIT_LSN with
	// a suffix is guaranteed to be greater than LAST_COMMIT_LSN itself,
	// the first file that comes up is the one. The same applies to any
	// LSN smaller than LAST_COMMIT_LSN.

	fn := endlsn.Expanded() + ".." + cfg.firstTxnLSN.Expanded()

	err := unix.Renameat(
		cfg.mgr.DirFd(),
		filepath.Join("..", defaults.IncDir, defaults.CurFile),
		cfg.mgr.DirFd(),
		filepath.Join("..", fn),		
	)
	if err != nil {
		return err
	}

	return cfg.newCur()
}

// TODO: truncate at shutdown

// Local Variables:
// tab-width: 4
// End:
