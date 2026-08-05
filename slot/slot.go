/*
   Copyright 2026 Torsten Foertsch

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
 */

package slot

import (
	"os"
	"fmt"
	"errors"

	"golang.org/x/sys/unix"

	"github.com/tfoertsch123/flock"
	"github.com/tfoertsch123/own-your-pg/lsn"
)

type header struct {
	NextLSN  lsn.LSN
	OwnerPid int64
	SlotType Type
}

type Slot struct {
	header
	Cfg
	name     string
	lck      *flock.Lock
	fh       *os.File
	mgr      *Mgr
	writable bool
	owner    bool
	skipCheckPidActive bool
}

type slotopts struct {
	create   bool
	writable bool
	asowner  bool
	nopidlck bool
	typ      Type
}

type SlotOpt func(*slotopts)

// default: O_RONLY
// we want to create it => O_RDWR + O_CREAT
func WithCreate() SlotOpt {
	return func(o *slotopts) {
		o.create = true
		o.writable = true
	}
}

// O_RDWR, no O_CREAT
func WithWrite() SlotOpt {
	return func(o *slotopts) {
		o.writable = true
	}
}

// O_RDWR + O_CREAT + ownerlock
func WithAsOwner() SlotOpt {
	return func(o *slotopts) {
		o.asowner = true
		o.writable = true
		o.create = true
	}
}

// O_RDWR + O_CREAT + ownerlock - pidlock
// used by slottool
func WithNoPidLock() SlotOpt {
	return func(o *slotopts) {
		o.asowner = true
		o.writable = true
		o.create = true
		o.nopidlck = true
	}
}

func WithType(v Type) SlotOpt {
	return func(o *slotopts) {
		o.typ = v
	}
}

var ErrNoOwnerLock error = errors.New(`Could not acquire owner lock`)

// the returned error is formatted
func (m *Mgr) Slot(name string, opts ...SlotOpt) (*Slot, error) {
	slo := &slotopts{}
	for _, o := range opts {
		o(slo)
	}
	sl := &Slot{
		header: header{
			SlotType: slo.typ,
		},
		name: name,
		mgr: m,
		writable: slo.writable,
	}

	if err := m.Check(false); err != nil { // make sure m.lck is valid
		return nil, err
	}
	flo := []flock.Option{
		flock.WithPath(name + fext),
		flock.WithPathAt(m.lck.Fd()),
	}

	if slo.writable {
		flo = append(flo, flock.WithRdWr())
	}

	if slo.create {
		flo = append(flo, flock.WithCreate(unix.S_IRUSR, unix.S_IWUSR))
	}
	sl.lck = flock.New(flo...)
	if err := sl.lck.Open(); err != nil {
		if !sl.writable && errors.Is(err, unix.ENOENT) {
			return nil, nil		// file does not exist in read-only mode
		} else {
			return nil, fmt.Errorf(`slot %s: %w`, sl.name, err)
		}
	}
	sl.fh = sl.lck.File()

	if slo.asowner {
		success, err := sl.lockOwner()
		if !success {
			if err == nil {
				err = ErrNoOwnerLock
			}
			sl.fh.Close()
			return nil, fmt.Errorf(`slot %s: %w`, sl.name, err)
		}
	}

	if err := sl.load(); err != nil {
		sl.fh.Close()
		return nil, err
	}

	// Set OwnerPid and confirm it by acquiring the PID lock
	if slo.asowner && !slo.nopidlck {
		pid := int64(os.Getpid())
		sl.OwnerPid = pid
		sl.saveHeader()
		success, err := sl.lockPid() // this should succeed
		if !success {
			if err == nil {
				err = ErrNoOwnerLock
			}
			sl.fh.Close()
			return nil, fmt.Errorf(`slot %s: UNEXPECTED %w`, sl.name, err)
		}
		sl.owner = true
	}

	return sl, nil
}

func (sl *Slot) Close() error {
	err := sl.lck.Close()
	err2 := sl.fh.Close()
	if err != nil {
		return err
	}
	return err2
}

func (sl *Slot) SetConfig(k string, v []string) {
	if v == nil {
		delete(sl.Config, k)
	} else {
		sl.Config[k] = v
	}
}

func (sl *Slot) GetConfig(k string, update ...bool) []string {
	if len(update) > 0 && update[0] {
		sl.loadCfg()
	}
	v, ok := sl.Config[k]
	if !ok {
		return nil
	}
	return v
}

func (sl *Slot) SaveConfig(sync bool) error {
	if err := sl.saveCfg(); err != nil {
		return err
	}
	if sync {
		if err := sl.fh.Sync(); err != nil {
			return fmt.Errorf(`fsync(slot %s): %w`, sl.name, err)
		}
	}
	return nil
}

// SetLSN sets the NextLSN field. If sync is true, it writes the slot
// header and fsync()s the file.
var ErrNoLSNForConfig error = errors.New("Config slot does not support LSN")
func (sl *Slot) SetLSN(lsn lsn.LSN, sync bool) error {
	if sl.SlotType == Config {
		return ErrNoLSNForConfig
	}
	sl.NextLSN = lsn
	if sync {
		if err := sl.saveHeader(); err != nil {
			return err
		}
		if err := sl.fh.Sync(); err != nil {
			return fmt.Errorf(`fsync(slot %s): %w`, sl.name, err)
		}
	}
	return nil
}

// SetType sets the SlotType field. If sync is true, it writes the slot
// header and fsync()s the file.
func (sl *Slot) SetType(typ Type, sync bool) error {
	sl.SlotType = typ
	if sync {
		if err := sl.saveHeader(); err != nil {
			return err
		}
		if err := sl.fh.Sync(); err != nil {
			return fmt.Errorf(`fsync(slot %s): %w`, sl.name, err)
		}
	}
	return nil
}

// GetOwner determines if the PID set as OwnerPid is running. It does so
// by trying to acquire the lock (flock(2)) on the slot file. If that
// succeeds the owner process is not running. Otherwise it is.
// It returns the OwnerPid, whether or not it is active and an error
// containing the potential error returned by the locking attempt.
// The optional update parameter (all but the first value is ignored)
// indicates whether or not to reload the information from the slot file.
func (sl *Slot) GetOwner(update ...bool) (int64, bool, error) {
	if sl.owner {
		return sl.OwnerPid, true, nil
	}
	success, err := sl.tryLockPid()
	if len(update) > 0 && update[0] {
		sl.loadHeader()
	}
	if success {
		sl.unlockPid()
		return sl.OwnerPid, false, nil
	}
	return sl.OwnerPid, true, err
}

func (sl *Slot) String() string {
	s, err := sl.AsJSON(WithDenseJSON())
	if err != nil {
		s = fmt.Sprintf(`slot %s: AsJSON: %v`, sl.name, err)
	}

	return s
}

// Local Variables:
// tab-width: 4
// End:
