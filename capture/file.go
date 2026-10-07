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
	// "encoding/hex"

	"github.com/tfoertsch123/log"
	"github.com/tfoertsch123/flock"
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
	path := filepath.Join("..", defaults.IncDir)
	err := unix.Mkdirat(cfg.mgr.DirFd(), path, 0777)
	if err != nil && !errors.Is(err, unix.EEXIST) {
		log.Panicf("Cannot create %s: %v", path, err)
	}

	cfg.currDirFd, err = unix.Openat(
		cfg.mgr.DirFd(), path, unix.O_RDONLY | unix.O_DIRECTORY, 0,
	)
	if err != nil {
		log.Panicf("Cannot open %s: %v", defaults.IncDir, err)
	}

	fd, err := unix.Openat(
		cfg.currDirFd, defaults.MetaName, unix.O_RDWR | unix.O_CREAT, 0666,
	)
	path = filepath.Join(defaults.IncDir, defaults.MetaName)
	if err != nil {
		log.Panicf("Cannot open %s: %v", path, err)
	}
	cfg.meta = os.NewFile(uintptr(fd), path)

	cfg.histDirFd, err = unix.Openat(
		cfg.mgr.DirFd(), "..", unix.O_RDONLY | unix.O_DIRECTORY, 0,
	)
	if err != nil {
		log.Panicf("Cannot open working directory: %v", err)
	}
}

// We keep the following metadata while writing the current file.
// - file position of the end of the latest commit - 64 bits
// - commit LSN of the first transaction in the file - 64 bits
//   This is only used to build the history file name. And we only
//   need to read this information at startup in order to init our
//   internal data. We could read the same information from the current
//   file by scanning the first couple of records.
// - the commit LSN of the last transaction in the file - 64 bits
//   This is used as our own commit position. Upon startup, this position
//   takes precedence over the position in the slot if it is ahead of
//   the slot LSN.
func (cfg *Cfg) writeMeta(epos int64, flsn *mylsn.LSN, llsn mylsn.LSN) error {
	var bts [24]byte
	binary.BigEndian.PutUint64(bts[:8], uint64(epos))
	if flsn == nil {
		binary.BigEndian.PutUint64(bts[8:16], uint64(0))
	} else {
		binary.BigEndian.PutUint64(bts[8:16], uint64(*flsn))
	}
	binary.BigEndian.PutUint64(bts[16:], uint64(llsn))

	// We rely on this data to be written atomically. I have found various
	// contradicting sources on this topic. Sqlite claims there is no such
	// thing as any atomic write gurantee when it comes to physical drives.
	// A modification in a block that is being written while the power is
	// being lost can even tamper with the unmodified portion of the block,
	// https://sqlite.org/psow.html.
	// On the other hand, postgres simply appends its commit records to the
	// WAL and fsyncs that. It does not write every commit record into a
	// separate filesystem block. So, if the Sqlite claim was a real problem,
	// a power loss could not only damage the transaction currently being
	// committed but also possibly any other already committed transaction
	// whose commit WAL record happens to be in the same filesystem block.
	// This would be a stark violation of PG durability guarantees.
	_, err := cfg.meta.WriteAt(bts[:], 0)
	if err != nil {
		return err
	}

	return cfg.meta.Sync()
}

func (cfg *Cfg) readMeta() (int64, *mylsn.LSN, mylsn.LSN, error) {
	var bts [24]byte
	_, err := cfg.meta.ReadAt(bts[:], 0)
	if err != nil {
		if err == io.EOF {
			err = ErrMetaGarbage
		}
		return 0, nil, mylsn.LSN(0), err
	}
	eoc := int64(binary.BigEndian.Uint64(bts[:8]))
	flsn := mylsn.LSN(binary.BigEndian.Uint64(bts[8:16]))
	llsn := mylsn.LSN(binary.BigEndian.Uint64(bts[16:]))

	if flsn == mylsn.LSN(0) {
		return eoc, nil, llsn, nil
	}

	return eoc, &flsn, llsn, nil
}

// this is called by the reveiver's OnConnect function. The connection
// can be interrupted in the middle of transmitting a transaction. In
// that case, we want to reset to the most recent commit point. Everything
// after that is invalid so far and will be retransmitted anyway when we
// reconnect to the DB. So, we need to truncate the file to the latest
// commit position and we might need to discard the write buffer content.
func (cfg *Cfg) truncateToEoc() error {
	eof, err := cfg.curr.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	if cfg.eoCommit < eof {
		cfg.mlg.Debugf("Truncation on reconnect: from eof=%v to eoc=%v",
			eof, cfg.eoCommit)
		err = cfg.curr.Truncate(cfg.eoCommit)
		if err != nil {
			return err
		}
		_, err = cfg.curr.Seek(cfg.eoCommit, io.SeekStart)
		if err != nil {
			return err
		}
	}

	blen := cfg.writer.Len()
	if blen > 0 {
		cfg.writer.Truncate(0)
		cfg.mlg.Debugf("%v bytes discarded from write buffer", blen)
	}
	return nil
}

var ErrCurrGarbage = errors.New("current file is garbage")
var ErrMetaGarbage = errors.New("metadata file is garbage")

// create a new and empty incoming/current file
func (cfg *Cfg) newCur() (mylsn.LSN, error) {
	lck := flock.New(
		flock.WithCreate(0666),
		flock.WithRdWr(),
		flock.WithPathAt(cfg.currDirFd),
		flock.WithPath(defaults.CurFile),
	)
	err := lck.Open()
	if err != nil {
		return mylsn.LSN(0), err
	}

	fh := lck.File()
	defer func() {
		if fh != nil {
			fh.Close()
		}
	}()
	eof, err := fh.Seek(0, io.SeekEnd)
	if err != nil {
		return mylsn.LSN(0), err
	}
	if eof == 0 {
		err = cfg.writeMeta(0, nil, mylsn.LSN(0))
		if err != nil {
			return mylsn.LSN(0), err
		}
		err = lck.LockRangeEx(0, 1)
		if err != nil {
			return mylsn.LSN(0), err
		}
		cfg.currLck, cfg.curr, fh = lck, fh, nil
		cfg.firstTxnLSN = nil
		cfg.eoCommit = 0
		return mylsn.LSN(0), nil
	}

	// We get here only if the file exist. During normal rotation that
	// does not happen, only during initialization.

	// Find the position after the last commit in the file header.
	// If the file length (eof) is beyond that position, truncate the file.

	eoc, flsn, llsn, err := cfg.readMeta()
	if err != nil {
		return mylsn.LSN(0), err
	}

	if eoc > eof {				// somebody tampered with the file
		return llsn, ErrCurrGarbage
	}

	if eoc < eof {
		// cut off trailing garbage
		err = fh.Truncate(eoc)
		if err != nil {
			return llsn, err
		}
		_, err = fh.Seek(eoc, io.SeekStart)
		if err != nil {
			return llsn, err
		}
	}

	err = lck.LockRangeEx(eoc, 1)
	if err != nil {
		return mylsn.LSN(0), err
	}

	cfg.currLck, cfg.curr, fh = lck, fh, nil
	cfg.firstTxnLSN = flsn
	cfg.eoCommit = eoc

	return llsn, nil
}

// to be called once during initialization
func (cfg *Cfg) curInit() mylsn.LSN {
	// create the first current file
	llsn, err := cfg.newCur()
	if err != nil {
		cfg.mlg.Panicf("%s: %v", filepath.Join(
			cfg.wd, defaults.IncDir, defaults.CurFile), err,
		)
	}

	// We allocate a 65kb buffer. But we implement a soft limit of 64kb.
	// If the buffer grows beyond 64kb, it is flushed.
	cfg.writer = bytes.NewBuffer(make([]byte, 0, 65*1024))

	return llsn
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
// Calling this function means functionally committing the previous
// transaction.
func (cfg *Cfg) eoc(lsn mylsn.LSN, writeBLSN bool) error {
	if cfg.firstTxnLSN == nil {
		cfg.firstTxnLSN = &lsn
	}
	if _, err := cfg.flush(); err != nil {
		return err
	}

	if writeBLSN {
		// cfg.eoCommit still points at the position after the previous
		// commit. That's our start position.
		_, err := cfg.curr.WriteAt(
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

	eof, err := cfg.curr.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	// cfg.firstTxnLSN is set in consume.go when the first B record is
	// consumed. Since C always comes after B it should not be nil here.
	err = cfg.writeMeta(eof, cfg.firstTxnLSN, lsn)
	if err != nil {
		return err
	}

	err = cfg.curr.Sync()
	if err != nil {
		return err
	}

	cfg.eoCommit = eof
	err = cfg.currLck.LockRangeEx(cfg.eoCommit, 1)
	if err != nil {
		return err
	}

	// There is an edge case here. If cfg.eoCommit ever were 0, then this
	// call would unlock the range from 0 to infinity. However, this cannot
	// happen. eoCommit at this point is always >0.
	err = cfg.currLck.UnlockRange(0, cfg.eoCommit)
	if err != nil {
		return err
	}

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
	cfg.currLck = nil

	// rotateFile() is called by eoc() which is called at COMMIT.
	// So, firstTxnLSN cannot by nil anymore.
	firstlsn := *cfg.firstTxnLSN
	cfg.firstTxnLSN = nil

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

	fn := endlsn.Expanded() + ".." + firstlsn.Expanded()

	err := unix.Renameat(cfg.currDirFd, defaults.CurFile, cfg.histDirFd, fn)
	if err != nil {
		return fmt.Errorf("Could not rotate current file %w", err)
	}
	err1 := unix.Fsync(cfg.histDirFd)
	err2 := unix.Fsync(cfg.currDirFd)
	if err1 != nil {
		return fmt.Errorf("Could sync working directory %w", err1)
	}
	if err2 != nil {
		return fmt.Errorf("Could sync %v %w", defaults.IncDir, err2)
	}

	_, err = cfg.newCur()
	return err
}

// Local Variables:
// tab-width: 4
// End:
