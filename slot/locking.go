package slot

import (
	"errors"

	"golang.org/x/sys/unix"
)

// We use Linux OFD locks here. These are range locks tied to the open file
// descriptor. Consequently, they are "inherited" by child processes if
// until the kid closes the file.

// The [0:2) range is locked by the owner. A slot can only have one owner.
// Owner locks are held for the entire lifetime of the process. Explicit
// unlocking is not necessary. This is an exclusive lock.
// The owner locks [0:1) first. If this succeeds it is the owner but the
// PID in the file is not yet trustworthy. Next the owner writes its PID
// to the file. It can safely do that because no other process can get the
// lock on [0:1). Next it locks [1:2). This indicates the PID in the file
// is correct. slottool can then display the PID as active.
//
// The [2:3) range represents the slot header. loadHeader() should get the
// shared lock, saveHeader() the exclusive one.
//
// The [3:4) range represents the slot cfg part. loadCfg() should get the
// shared lock, saveCfg() the exclusive one.

// Exclusive locking requires the file to be opened for write.

// TODO: make this context-aware perhaps with a deadline. But that only makes
// sense if there is a sensible way to continue.

// There can only be one producer per system. This lock is implemented
// as an ordinary flock on the slot directory. If a producer slot is opened
// WithAsOwner, this lock is also acquired.

const (
	lckOwner = 0
	lckPid = 1
	lckHeader = 2
	lckCfg = 3
)

func (sl *Slot) lockOwner() (bool, error) {
	if sl.SlotType == Producer {
		success, err := sl.mgr.lock()
		if !success {
			return success, err
		}
	}

	err := sl.lck.TryLockRangeEx(lckOwner, 1)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return false, err
}

func (sl *Slot) lockPid() (bool, error) {
	err := sl.lck.TryLockRangeEx(lckPid, 1)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return false, err
}

func (sl *Slot) unlockOwner() error {
	return sl.lck.UnlockRange(lckOwner, 2)
}

func (sl *Slot) lockHeaderSh() error {
	return sl.lck.LockRangeSh(lckHeader, 1)
}

func (sl *Slot) lockHeaderEx() error {
	return sl.lck.LockRangeEx(lckHeader, 1)
}

func (sl *Slot) unlockHeader() error {
	return sl.lck.UnlockRange(lckHeader, 1)
}

func (sl *Slot) lockCfgSh() error {
	return sl.lck.LockRangeSh(lckCfg, 1)
}

func (sl *Slot) lockCfgEx() error {
	return sl.lck.LockRangeEx(lckCfg, 1)
}

func (sl *Slot) unlockCfg() error {
	return sl.lck.UnlockRange(lckCfg, 1)
}

// Local Variables:
// tab-width: 4
// End:
