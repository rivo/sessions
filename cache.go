package sessions

import (
	"sync"
	"time"
)

// cache implements a simple LRU write-though cache for user sessions. It
// is used implicitly by all sessions functions.
//
// Member functions should not be called while sessions are locked.
type cache struct {
	sync.RWMutex
	sessions map[string]*Session
}

// sessions is the global sessions cache.
var sessions *cache

// initCache initalizes the global sessions cache.
func initCache() {
	sessions = &cache{
		sessions: make(map[string]*Session),
	}
}

// Get returns a session with the given ID from the cache. If the session is not
// cached, the persistence layer is asked to load and return the session. If no
// such session exists, a nil session may be returned. This function does not
// update the session's last access date.
func (c *cache) Get(id string) (*Session, error) {
	c.RLock()
	defer c.RUnlock()

	// Do we have a cached session?
	session, ok := c.sessions[id]
	if !ok {
		// Not cached. Query the persistence layer for a session.
		var err error
		session, err = Persistence.LoadSession(id)
		if err != nil {
			return nil, err
		}

		if session != nil {
			// Save it in the cache.
			if MaxSessionCacheSize != 0 {
				c.compact(1)
				c.sessions[id] = session
			}

			// Store ID.
			session.Lock()
			session.id = id
			session.Unlock()
		}
	}

	return session, nil
}

// Set inserts or updates a session in the cache. Since this is a write-through
// cache, the persistence layer is also triggered to save the session.
func (c *cache) Set(session *Session) error {
	c.Lock()
	defer c.Unlock()
	session.Lock()
	session.lastAccess = time.Now()
	id := session.id
	session.Unlock()

	// Try to compact the cache.
	var requiredSpace int
	if _, ok := c.sessions[id]; !ok {
		requiredSpace = 1
	}
	c.compact(requiredSpace)

	// Save in cache.
	if MaxSessionCacheSize != 0 {
		c.sessions[id] = session
	}

	// Write through to database.
	session.Lock()
	defer session.Unlock()
	if err := Persistence.SaveSession(id, session); err != nil {
		return nil
	}

	return nil
}

// Delete deletes a session. A logged-in user will be logged out.
func (c *cache) Delete(id string) error {
	c.Lock()
	defer c.Unlock()

	// Remove from cache.
	delete(c.sessions, id)

	// Remove from database.
	return Persistence.DeleteSession(id)
}

// compact drops sessions from the cache to make space for the given number
// of sessions. It also drops sessions that have been in the cache longer than
// SessionCacheExpiry. The number of dropped sessions are returned. Dropped
// sessions are updated in the persistence layer to update the last access time.
//
// This function does not synchronize concurrent access to the cache.
func (c *cache) compact(requiredSpace int) (int, error) {
	// Check for old sessions.
	for id, session := range c.sessions {
		session.RLock()
		age := time.Since(session.lastAccess)
		session.RUnlock()
		if age > SessionCacheExpiry {
			if err := Persistence.SaveSession(id, session); err != nil {
				return 0, err
			}
			delete(c.sessions, id)
		}
	}

	// Cache may still grow.
	if MaxSessionCacheSize < 0 || len(c.sessions)+requiredSpace <= MaxSessionCacheSize {
		return 0, nil
	}

	// Drop the oldest sessions.
	var dropped int
	if requiredSpace > MaxSessionCacheSize {
		requiredSpace = MaxSessionCacheSize // We can't request more than is allowed.
	}
	for len(c.sessions)+requiredSpace > MaxSessionCacheSize {
		// Find oldest sessions and delete them.
		var (
			oldestAccessTime time.Time
			oldestSessionID  string
		)
		for id, session := range c.sessions {
			session.RLock()
			before := session.lastAccess.Before(oldestAccessTime)
			session.RUnlock()
			if oldestSessionID == "" || before {
				oldestSessionID = id
				oldestAccessTime = session.lastAccess
			}
		}
		if err := Persistence.SaveSession(oldestSessionID, c.sessions[oldestSessionID]); err != nil {
			return 0, err
		}
		delete(c.sessions, oldestSessionID)
		dropped++
	}

	return dropped, nil
}

// PurgeSessions removes all sessions from the local cache. The current cache
// content is also saved via the persistence layer, to update the session last
// access times.
func PurgeSessions() {
	sessions.Lock()
	defer sessions.Unlock()

	// Update all sessions in the database.
	for id, session := range sessions.sessions {
		Persistence.SaveSession(id, session)
		// We only do this to update the last access time. Errors are not that
		// bad.
	}

	sessions.sessions = make(map[string]*Session, MaxSessionCacheSize)
}
