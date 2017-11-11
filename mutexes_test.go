package sessions

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// Test mutex lock with immediate release.
func TestMutexesLockAndRelease(t *testing.T) {
	m := newMutexes()
	var done bool
	go func() {
		m.Lock("key")
		m.Unlock("key")
		done = true
	}()
	time.Sleep(10 * time.Millisecond)
	if !done {
		t.Error("Mutex still blocking despite release")
	}
}

// Test mutex with two locks on the same key.
func TestMutexesTwoLocks(t *testing.T) {
	m := newMutexes()
	var (
		result string
		wg     sync.WaitGroup
	)
	go0 := make(chan bool)
	go1 := make(chan bool)
	go2 := make(chan bool)
	for i := 0; i < 5; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			m.Lock("key")
			defer m.Unlock("key")
			go0 <- true
			<-go1
			result += "1"
		}()
		go func() {
			defer wg.Done()
			<-go2
			m.Lock("key")
			defer m.Unlock("key")
			result += "2"
		}()
		<-go0
		go2 <- true
		go1 <- true
		wg.Wait()
	}
	if result != "1212121212" {
		t.Errorf("Locking not as expected: %s", result)
	}
}

// Test mutex with two locks on different keys.
func TestMutexesDifferentKeys(t *testing.T) {
	m := newMutexes()
	var (
		result string
		wg     sync.WaitGroup
	)
	go0 := make(chan bool)
	go1 := make(chan bool)
	wg.Add(2)
	go func() {
		defer wg.Done()
		m.Lock("key1")
		defer m.Unlock("key1")
		result += "1"
		go0 <- true
	}()
	go func() {
		defer wg.Done()
		<-go1
		m.Lock("key2")
		defer m.Unlock("key2")
		result += "2"
	}()
	<-go0
	go1 <- true
	wg.Wait()
	if result != "12" {
		t.Errorf("Locking not as expected: %s", result)
	}
}

// Test mutex with many locks on the same key.
func TestMutexesMultipleLocks(t *testing.T) {
	n := 10
	m := newMutexes()
	var (
		result string
		wg     sync.WaitGroup
	)
	go0 := make(chan bool)
	go1 := make(chan bool)
	go2 := make(chan bool)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(nr int) {
			defer wg.Done()
			<-go1 // Will fire when go1 is closed.
			m.Lock("key")
			defer m.Unlock("key")
			result += strconv.Itoa(nr)
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.Lock("key")
		defer m.Unlock("key")
		go0 <- true
		<-go2
		result += "F"
	}()
	<-go0
	close(go1)
	go2 <- true
	wg.Wait()
	t.Log(result)
	if len(result) != n+1 {
		t.Error("Some goroutines still haven't finished")
	}
	if result[0:1] != "F" {
		t.Error("Locks were processed in the wrong order")
	}
	if m.getItem("key").locks != 0 {
		t.Error("Locks are still held")
	}
}

// Test mutexes by holding a long lock.
func TestMutexesLongLock(t *testing.T) {
	m := newMutexes()
	var (
		mutex  sync.Mutex
		result string
		wg     sync.WaitGroup
		sg     sync.WaitGroup
	)
	go1 := make(chan bool)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		sg.Add(1)
		go func(nr int) {
			sg.Done()
			defer wg.Done()
			<-go1
			m.Lock("key")
			mutex.Lock()
			result += strconv.Itoa(nr)
			mutex.Unlock()
			m.Unlock("key")
		}(i)
	}
	sg.Wait()
	m.Lock("key")
	close(go1)
	time.Sleep(time.Millisecond)
	if len(result) > 0 {
		t.Errorf("Some goroutines already wrote despite locks: %s", result)
	}
	m.Unlock("key")
	wg.Wait()
	if len(result) != 10 {
		t.Errorf("Some goroutines did not write despite released lock: %s", result)
	}
}

// Test mutex purging of stale items.
func TestMutexesStalePurge(t *testing.T) {
	m := newMutexes()
	mutexStaleMutexes = time.Millisecond
	m.Lock("key1")
	m.Unlock("key1")
	m.Lock("key2")
	m.Unlock("key2")
	m.Lock("key3")
	m.Unlock("key3")
	if len(m.items) != 3 {
		t.Error("Keys not found in items")
	}
	time.Sleep(3 * time.Millisecond)
	m.purge <- struct{}{}
	time.Sleep(3 * time.Millisecond)
	if len(m.items) != 0 {
		t.Error("Mutex map was not purged")
	}
}

// Test mutex purging due to max size reached.
func TestMutexesSizePurge(t *testing.T) {
	m := newMutexes()
	m.Lock("key1")
	m.Unlock("key1")
	m.Lock("key2")
	m.Unlock("key2")
	m.Lock("key3")
	m.Unlock("key3")
	if len(m.items) != 3 {
		t.Error("Keys not found in items")
	}
	mutexMaxCacheSize = 1
	m.purge <- struct{}{}
	time.Sleep(3 * time.Millisecond)
	if len(m.items) != 1 {
		t.Error("Mutex map was not purged")
	}
}
