package sessions

import (
	"sync"
	"time"
)

var (
	// The maximum cache size. If the items map size exceeds this number, random
	// items (which are not locked) are dropped from the map.
	mutexMaxCacheSize = 1024 * 1024

	// How often the mutex map is checked for stale mutexes.
	mutexCleanupFrequency = 10 * time.Minute

	// The duration after which mutexes which weren't accessed are removed from
	// the map.
	mutexStaleMutexes = time.Hour
)

// mutexes is a locking handler which allows key-based concurrency
// synchronization. On each key, every call to Lock() must be followed by
// exactly one eventual call to Unlock() or else locking behaviour becomes
// undefined.
type mutexes struct {
	items      map[interface{}]*mutexItem
	itemsMutex sync.Mutex
	acquire    chan interface{}
	release    chan interface{}
	purge      chan struct{}
}

// mutexItem is a lockable item.
type mutexItem struct {
	locks      int
	lastAccess time.Time
	release    chan struct{}
}

// newMutexes returns a new locking handler which allows key-based concurrency
// synchronization.
func newMutexes() *mutexes {
	m := &mutexes{
		items:   make(map[interface{}]*mutexItem),
		acquire: make(chan interface{}),
		release: make(chan interface{}),
		purge:   make(chan struct{}),
	}

	// Main goroutine.
	go func() {
		for {
			select {

			// A lock was requested.
			case key := <-m.acquire:
				item := m.getItem(key)
				if item.locks == 0 {
					item.release <- struct{}{}
				}
				item.locks++

			// A lock was released.
			case key := <-m.release:
				item := m.getItem(key)
				if item.locks > 0 { // Only release if locked.
					item.locks--
					if item.locks > 0 { // First lock was already released.
						item.release <- struct{}{}
					}
				}

				// A cleanup was requested.
			case <-m.purge:
				m.itemsMutex.Lock()
				for key, item := range m.items {
					if time.Since(item.lastAccess) > mutexStaleMutexes ||
						len(m.items) > mutexMaxCacheSize && item.locks == 0 {
						// Item is stale. Remove.
						delete(m.items, key)
					}
				}
				m.itemsMutex.Unlock()

			}
		}
	}()

	// Purge items regularly.
	go func() {
		for {
			time.Sleep(mutexCleanupFrequency)
			m.purge <- struct{}{}
		}
	}()

	return m
}

// getItem returns an item for the given key, creating it if it doesn't exist
// yet. Thread-safe.
func (m *mutexes) getItem(key interface{}) *mutexItem {
	m.itemsMutex.Lock()
	defer m.itemsMutex.Unlock()
	item, ok := m.items[key]
	if !ok {
		item = &mutexItem{release: make(chan struct{})}
		m.items[key] = item

		// If the map is too big, request purge.
		if len(m.items) > mutexMaxCacheSize {
			go func() {
				m.purge <- struct{}{}
			}()
		}
	}
	item.lastAccess = time.Now()
	return item
}

// Lock blocks until any other locks held on the given key are released.
func (m *mutexes) Lock(key interface{}) {
	m.acquire <- key
	<-m.getItem(key).release
}

// Unlock releases a previously acquired lock on the given key.
func (m *mutexes) Unlock(key interface{}) {
	m.release <- key
}
