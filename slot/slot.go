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
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/tfoertsch123/log"
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
		owner: slo.asowner,
	}

	flo := []flock.Option{
		flock.WithPath(filepath.Join(m.Dir(), name + fext)),
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

	if sl.owner {
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
	if sl.owner {
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
	}

	return sl, nil
}

func (sl *Slot) Close() error {
	sl.lck.Close()
	return sl.fh.Close()
}

func (sl *Slot) SetConfig(k string, v ...string) {
	sl.Config[k] = v
}

func (sl *Slot) GetConfig(k string) []string {
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
func (sl *Slot) SetLSN(lsn lsn.LSN, sync bool) error {
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

func (sl *Slot) OwnerActive() (bool, error) {
	log.Errorf("OwnerActive: <%#v>, fh:<%v>", sl.lck, sl.fh)
	if sl.owner {
		return true, nil
	}
	success, err := sl.lockPid()
	if success {
		sl.unlockOwner()
		return true, nil
	}
	log.Errorf("OwnerActive: %v", err)
	return false, err
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
