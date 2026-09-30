package slot

import (
	"testing"
	"os"
	"fmt"
	"time"
	"errors"

	"github.com/tfoertsch123/flock"
)

func TestSlotLocking(t *testing.T) {
	dir := t.TempDir()
	m, err := NewMgr(dir)
	if err != nil {
		t.Fatalf("mgr: %v", err)
	}

	t.Run("GetOwner", func(t *testing.T) {
		sl1, err := m.Slot("test", WithAsOwner())
		if err != nil {
			t.Fatalf("sl1: %v", err)
		}
		defer sl1.Close()

		sl2, err := m.Slot("test")
		if err != nil {
			t.Fatalf("sl2: %v", err)
		}
		defer sl2.Close()

		pid, active, err := sl2.GetOwner()
		if err != nil {
			t.Fatalf("sl2.GetOwner: %v", err)
		}
		if pid != int64(os.Getpid()) {
			t.Error("sl2.GetOwner: pid check")
		}
		if !active {
			t.Error("sl2.GetOwner: active")
		}

		sl1.OwnerPid = -3
		err = sl1.saveHeader()
		if err != nil {
			t.Fatalf("sl1.saveHeader: %v", err)
		}
		err = sl1.unlockOwner()
		if err != nil {
			t.Fatalf("sl2.unlockOwner: %v", err)
		}

		// still seeing the old OwnerPid
		pid, active, err = sl2.GetOwner()
		if err != nil {
			t.Fatalf("sl2.GetOwner: %v", err)
		}
		if pid != int64(os.Getpid()) {
			t.Error("sl2.GetOwner: pid check")
		}
		if active {
			t.Error("sl2.GetOwner: active")
		}

		// now with reload
		pid, active, err = sl2.GetOwner(true)
		if err != nil {
			t.Fatalf("sl2.GetOwner: %v", err)
		}
		if pid != -3 {
			t.Error("sl2.GetOwner: pid check (-3)")
		}
		if active {
			t.Error("sl2.GetOwner: active")
		}

		// tryLockPid on closed slot
		err = sl1.Close()
		if err != nil {
			t.Fatalf("sl1.Close(): %v", err)
		}
		active, err = sl1.tryLockPid()
		if active {
			t.Error("sl1.tryLockPid on closed slot")
		}
		if err == nil {
			t.Errorf("sl1.tryLockPid on closed slot, exp: error")
		}
	})

	t.Run("Owner Lock", func(t *testing.T) {
		sl1, err := m.Slot("test", WithAsOwner())
		if err != nil {
			t.Fatalf("sl1: %v", err)
		}
		defer sl1.Close()

		sl2, err := m.Slot("test", WithAsOwner())
		if !errors.Is(err, ErrNoOwnerLock) {
			t.Errorf("sl2: expecting ErrNoOwnerLock, got %v", err)
		}
		if sl2 != nil {
			t.Errorf("sl2: expecting nil, got %v", sl2)
			sl2.Close()
		}
	})

	t.Run("Producer Lock", func(t *testing.T) {
		m2, err := NewMgr(m.Dir())
		if err != nil {
			t.Fatalf("mgr2: %v", err)
		}
		defer m2.Close()

		sl1, err := m.Slot("producer1", WithAsOwner(), WithType(Producer))
		if err != nil {
			t.Fatalf("sl1: %v", err)
		}
		defer sl1.Close()

		sl2, err := m2.Slot("producer2", WithAsOwner(), WithType(Producer))
		if !errors.Is(err, ErrNoOwnerLock) {
			t.Errorf("sl2: expecting ErrNoOwnerLock, got %v", err)
		}
		if sl2 != nil {
			t.Errorf("sl2: expecting nil, got %v", sl2)
			sl2.Close()
		}
	})

	t.Run("failed tryLockPid", func(t *testing.T) {
		sl, err := m.Slot("test-failed", WithCreate())
		if err != nil {
			t.Fatalf("sl: %v", err)
		}
		sl.Close()

		success, err := sl.tryLockPid()
		if success {
			t.Errorf("tryLockPid(): expecting false, got %v", success)
		}
		if !errors.Is(err, flock.ErrAlreadyClosed) {
			t.Errorf("tryLockPid(): expecting false, got %v", err)
		}
	})

	t.Run("failed lockPid", func(t *testing.T) {
		sl, err := m.Slot("test-failed", WithCreate())
		if err != nil {
			t.Fatalf("sl: %v", err)
		}
		sl.Close()

		success, err := sl.lockPid()
		if success {
			t.Errorf("lockPid(): expecting false, got %v", success)
		}
		if !errors.Is(err, flock.ErrAlreadyClosed) {
			t.Errorf("lockPid(): expecting false, got %v", err)
		}
	})

	t.Run("failed lockOwner", func(t *testing.T) {
		sl, err := m.Slot("test-failed", WithCreate())
		if err != nil {
			t.Fatalf("sl: %v", err)
		}
		sl.Close()

		success, err := sl.lockOwner()
		if success {
			t.Errorf("lockOwner(): expecting false, got %v", success)
		}
		if !errors.Is(err, flock.ErrAlreadyClosed) {
			t.Errorf("lockOwner(): expecting ErrAlreadyClosed, got %v", err)
		}
	})

	t.Run("lockHeader ex waiting for sh", func(t *testing.T) {
		var slots [3]*Slot
		for i := 0; i < len(slots); i++ {
			slo := []SlotOpt{}
			if i == 0 {
				slo = append(slo, WithCreate())
			}
			sl, err := m.Slot("lock-hdr", slo...)
			if err != nil {
				t.Fatalf("sl[%d]: %v", i, err)
			}
			defer sl.Close()
			// suicide if we deadlock
			time.AfterFunc(2*time.Second, func(){
				if !sl.lck.IsClosed() {
					panic(fmt.Sprintf("Not Closed %v", sl))
				}
			})
			slots[i] = sl
		}

		done := make(chan string, 10) // send error msgs to the main thread

		c := make(chan int, 10)
		for n := 1; n < len(slots); n++ {
			// Start 1..n go routines each getting a shared header lock.
			// This lock is kept for 100ms and released. Once the lock
			// has been acquired the thread's number is sent out through c.
			// Note, slots[0] will later be used for the excl lock.
			//
			// Once each the lock has been released, the negative of the
			// thread's number is sent through the channel.
			go func(i int) {
				err := slots[i].lockHeaderSh()
				if err != nil {
					t.Errorf(
						"lockHeaderSh(%d): expecting nil, got %v",
						i, err)
				}
				c <- i
				time.Sleep(100 * time.Millisecond)
				err = slots[i].unlockHeader()
				if err != nil {
					t.Errorf(
						"unlockHeaderSh(%d): expecting nil, got %v",
						i, err)
				}
				c <- -i
			}(n)
		}
		// Now start another thread. Here we first wait for 1..n signals
		// from c. This means all shared lockers have entered their 100ms
		// slumber. We expect to reach this state in significantly less
		// than 100ms.
		// Once this state has been reached, we start another thread
		// exclusively locking Header. This now has to wait for about 100ms.
		// The exclusive lock is then released immediately.
		now := time.Now()
		go func() {
			exp := 0
			got := 0
			start0 := true
			for n := 1; n < len(slots); n++ {
				exp += n
				got += <-c
				if start0 {
					start0 = false
					// At this point we got the first shared lock.
					// So, we can start the ex lock.
					go func(i int) {
						err := slots[i].lockHeaderEx()
						if err != nil {
							t.Errorf(
								"lockHeaderEx(%d): expecting nil, got %v",
								i, err)
						}
						err = slots[i].unlockHeader()
						if err != nil {
							t.Errorf(
								"unlockHeaderEx(%d): expecting nil, got %v",
								i, err)
						}
						close(done)
					}(0)
				}
			}
			if got != exp {
				t.Errorf(
					"lockHeaderSh(): exp(1+...+len(slots)-1) = %d, got %d",
					exp, got)
			}
			if time.Now().After(now.Add(50*time.Millisecond)) {
				t.Error("lockHeaderSh(): deadline expired")
			}
			for n := 1; n < len(slots); n++ {
				got += <-c
			}
			if got != 0 {
				t.Errorf(
					"lockHeaderSh(): exp 0 after count down, got %d", got)
			}
		}()

		// wait for the bunch
		<- done
		if time.Now().Before(now.Add(100*time.Millisecond)) {
			t.Error("lockHeaderEx(): too early")
		}
	})

	t.Run("lockHeader sh waiting for ex", func(t *testing.T) {
		var slots [50]*Slot
		for i := 0; i < len(slots); i++ {
			slo := []SlotOpt{}
			if i == 0 {
				slo = append(slo, WithCreate())
			}
			sl, err := m.Slot("lock-hdr-ex", slo...)
			if err != nil {
				t.Fatalf("sl[%d]: %v", i, err)
			}
			defer sl.Close()
			// suicide if we deadlock
			time.AfterFunc(2*time.Second, func(){
				if !sl.lck.IsClosed() {
					panic(fmt.Sprintf("Not Closed %v", sl))
				}
			})
			slots[i] = sl
		}

		c := make(chan int, 10)

		// Start the ex locker
		go func(i int) {
			err := slots[i].lockHeaderEx()
			if err != nil {
				t.Errorf(
					"lockHeaderEx(%d): expecting nil, got %v",
					i, err)
			}
			c <- i
			time.Sleep(100 * time.Millisecond)
			err = slots[i].unlockHeader()
			if err != nil {
				t.Errorf(
					"unlockHeaderEx(%d): expecting nil, got %v",
					i, err)
			}
			c <- i
		}(0)

		// wait for it to lock
		if x := <-c; x != 0 {
			t.Fatalf("expecting 0, got %v -- strange", x)
		}

		now := time.Now()
		for n := 1; n < len(slots); n++ {
			go func(i int) {
				err := slots[i].lockHeaderSh()
				if err != nil {
					t.Errorf(
						"lockHeaderSh(%d): expecting nil, got %v",
						i, err)
				}
				time.Sleep(1 * time.Millisecond)
				err = slots[i].unlockHeader()
				if err != nil {
					t.Errorf(
						"unlockHeaderSh(%d): expecting nil, got %v",
						i, err)
				}
				c <- i
			}(n)
		}

		// The first message must come from thread 0 after unlocking
		if x := <-c; x != 0 {
			t.Fatalf("expecting 0, got %v -- strange", x)
		}

		// now we consume the other threads
		exp := 0
		got := 0
		for n := 1; n < len(slots); n++ {
			exp += n
			got += <-c
		}

		if got != exp {
			t.Errorf(
				"lockHeaderSh(): exp(1+...+len(slots)-1) = %d, got %d",
				exp, got)
		}

		if time.Now().Before(now.Add(100*time.Millisecond)) {
			t.Error("lockHeaderSh(): too early")
		}

		if time.Now().After(now.Add(150*time.Millisecond)) {
			t.Error("lockHeaderSh(): too late")
		}
	})

	t.Run("lockCfg ex waiting for sh", func(t *testing.T) {
		var slots [3]*Slot
		for i := 0; i < len(slots); i++ {
			slo := []SlotOpt{}
			if i == 0 {
				slo = append(slo, WithCreate())
			}
			sl, err := m.Slot("lock-cfg", slo...)
			if err != nil {
				t.Fatalf("sl[%d]: %v", i, err)
			}
			defer sl.Close()
			// suicide if we deadlock
			time.AfterFunc(2*time.Second, func(){
				if !sl.lck.IsClosed() {
					panic(fmt.Sprintf("Not Closed %v", sl))
				}
			})
			slots[i] = sl
		}

		done := make(chan string, 10) // send error msgs to the main thread

		c := make(chan int, 10)
		for n := 1; n < len(slots); n++ {
			// Start 1..n go routines each getting a shared Cfg lock.
			// This lock is kept for 100ms and released. Once the lock
			// has been acquired the thread's number is sent out through c.
			// Note, slots[0] will later be used for the excl lock.
			//
			// Once each the lock has been released, the negative of the
			// thread's number is sent through the channel.
			go func(i int) {
				err := slots[i].lockCfgSh()
				if err != nil {
					t.Errorf(
						"lockCfgSh(%d): expecting nil, got %v",
						i, err)
				}
				c <- i
				time.Sleep(100 * time.Millisecond)
				err = slots[i].unlockCfg()
				if err != nil {
					t.Errorf(
						"unlockCfgSh(%d): expecting nil, got %v",
						i, err)
				}
				c <- -i
			}(n)
		}
		// Now start another thread. Here we first wait for 1..n signals
		// from c. This means all shared lockers have entered their 100ms
		// slumber. We expect to reach this state in significantly less
		// than 100ms.
		// Once this state has been reached, we start another thread
		// exclusively locking Cfg. This now has to wait for about 100ms.
		// The exclusive lock is then released immediately.
		now := time.Now()
		go func() {
			exp := 0
			got := 0
			start0 := true
			for n := 1; n < len(slots); n++ {
				exp += n
				got += <-c
				if start0 {
					start0 = false
					// At this point we got the first shared lock.
					// So, we can start the ex lock.
					go func(i int) {
						err := slots[i].lockCfgEx()
						if err != nil {
							t.Errorf(
								"lockCfgEx(%d): expecting nil, got %v",
								i, err)
						}
						err = slots[i].unlockCfg()
						if err != nil {
							t.Errorf(
								"unlockCfgEx(%d): expecting nil, got %v",
								i, err)
						}
						close(done)
					}(0)
				}
			}
			if got != exp {
				t.Errorf(
					"lockCfgSh(): exp(1+...+len(slots)-1) = %d, got %d",
					exp, got)
			}
			if time.Now().After(now.Add(50*time.Millisecond)) {
				t.Error("lockCfgSh(): deadline expired")
			}
			for n := 1; n < len(slots); n++ {
				got += <-c
			}
			if got != 0 {
				t.Errorf(
					"lockCfgSh(): exp 0 after count down, got %d", got)
			}
		}()

		// wait for the bunch
		<- done
		if time.Now().Before(now.Add(100*time.Millisecond)) {
			t.Error("lockCfgEx(): too early")
		}
	})

	t.Run("lockCfg sh waiting for ex", func(t *testing.T) {
		var slots [50]*Slot
		for i := 0; i < len(slots); i++ {
			slo := []SlotOpt{}
			if i == 0 {
				slo = append(slo, WithCreate())
			}
			sl, err := m.Slot("lock-hdr-ex", slo...)
			if err != nil {
				t.Fatalf("sl[%d]: %v", i, err)
			}
			defer sl.Close()
			// suicide if we deadlock
			time.AfterFunc(2*time.Second, func(){
				if !sl.lck.IsClosed() {
					panic(fmt.Sprintf("Not Closed %v", sl))
				}
			})
			slots[i] = sl
		}

		c := make(chan int, 10)

		// Start the ex locker
		go func(i int) {
			err := slots[i].lockCfgEx()
			if err != nil {
				t.Errorf(
					"lockCfgEx(%d): expecting nil, got %v",
					i, err)
			}
			c <- i
			time.Sleep(100 * time.Millisecond)
			err = slots[i].unlockCfg()
			if err != nil {
				t.Errorf(
					"unlockCfgEx(%d): expecting nil, got %v",
					i, err)
			}
			c <- i
		}(0)

		// wait for it to lock
		if x := <-c; x != 0 {
			t.Fatalf("expecting 0, got %v -- strange", x)
		}

		now := time.Now()
		for n := 1; n < len(slots); n++ {
			go func(i int) {
				err := slots[i].lockCfgSh()
				if err != nil {
					t.Errorf(
						"lockCfgSh(%d): expecting nil, got %v",
						i, err)
				}
				time.Sleep(1 * time.Millisecond)
				err = slots[i].unlockCfg()
				if err != nil {
					t.Errorf(
						"unlockCfgSh(%d): expecting nil, got %v",
						i, err)
				}
				c <- i
			}(n)
		}

		// The first message must come from thread 0 after unlocking
		if x := <-c; x != 0 {
			t.Fatalf("expecting 0, got %v -- strange", x)
		}

		// now we consume the other threads
		exp := 0
		got := 0
		for n := 1; n < len(slots); n++ {
			exp += n
			got += <-c
		}

		if got != exp {
			t.Errorf(
				"lockCfgSh(): exp(1+...+len(slots)-1) = %d, got %d",
				exp, got)
		}

		if time.Now().Before(now.Add(100*time.Millisecond)) {
			t.Error("lockCfgSh(): too early")
		}

		if time.Now().After(now.Add(150*time.Millisecond)) {
			t.Error("lockCfgSh(): too late")
		}
	})
}

// Local Variables:
// tab-width: 4
// End:
