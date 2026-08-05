package slot

import (
	"errors"

	"golang.org/x/sys/unix"
)

// We use Linux OFD locks and normal flock(2) here. Both are tied to the open
// file descriptor. Consequently, they are "inherited" by child processes if
// until the kid closes the file.

// flock() does not require write access for exclusive locking. Both, the
// shared and the exclusive locks, work if the file is opened read-only.
// OFD locks interpret the shared lock as a  read operation and the
// exclusive lock as writing. Hence, the exclusive lock needs write access.

// Out of the 4 slot types, Change, Archiver, Config and Producer, the latter
// is special. There can only be one producer per working directory at a
// time. This is represented by a normal flock() on the manager directory.
// When a Producer slot is opened as owner, this is the first lock taken.
// This exclusive lock is held for the entire lifetime of the slot.

// Next, when a slot is opened as owner, an exclusive OFD lock on the range
// [0:1) is taken. This lock is also held for the entire lifetime of the slot.
// This guarantees only one owner at a time.

// Next the slot data is read. This is done no matter whether or not the slot
// has been opened as owner. The file has 2 content locks, the header lock
// and the config lock. Both are blocking locks. They are meant to be held
// only very briefly. However, this means another buggy process can block
// the slot creation forever. The header lock is an OFD lock on the range
// [1:2) and the config lock uses the range [2:3). loadHeader() takes a
// shared lock on the header while the header portion of the file is read.
// saveHeader() takes the lock exclusively. Similar for the config section.

// Once the data has been loaded during initialization, the owner needs to
// update the OwnerPid field in the header. When that is done, the owner
// takes an exclusive flock() on the slot file itself. This lock is then
// held for the entire lifetime of the slot. The operation is blocking.
// The interpretation here is that while this lock is held by the owner,
// the OwnerPid information can be trusted.

// The [OwnerActive] function uses this by trying to briefly acquire
// a shared lock on the slot file, see [tryLockPid] below. flock() is
// much more widely supported than OFD locks. Using a normal flock()
// here allows the usage of tools like flock(1), for instance.

const (
	lckOwner = 0
	lckHeader = 1
	lckCfg = 2
)

func (sl *Slot) tryLockPid() (bool, error) {
	err := sl.lck.TryLockSh()
	if err == nil {
		return true, nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return false, err
}

func (sl *Slot) lockPid() (bool, error) {
	err := sl.lck.LockEx()
	if err == nil {
		return true, nil
	}
	return false, err
}

func (sl *Slot) unlockPid() error {
	return sl.lck.Unlock()
}

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

func (sl *Slot) unlockOwner() error {
	err1 := sl.unlockPid()
	err2 := sl.lck.UnlockRange(lckOwner, 1)
	if err1 != nil {
		return err1
	}
	return err2
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
