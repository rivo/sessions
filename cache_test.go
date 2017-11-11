package sessions

import (
	"testing"
	"time"
)

// Test basic cache functionality.
func TestCache(t *testing.T) {
	// A session ID set.
	set := map[string]struct{}{"s3": struct{}{}, "s4": struct{}{}}

	// Some reference counts.
	var loaded, saved, deleted int
	tab := func(step int) {
		t.Logf("%d: Loaded = %d, saved = %d, deleted = %d, cache size = %d, set = %s", step, loaded, saved, deleted, len(sessions.sessions), set)
	}

	// A test persistence layer.
	Persistence = ExtendablePersistenceLayer{
		LoadSessionFunc: func(id string) (*Session, error) {
			loaded++
			if _, ok := set[id]; ok {
				return &Session{id: id, lastAccess: time.Now()}, nil
			}
			return nil, nil
		},
		SaveSessionFunc: func(id string, session *Session) error {
			saved++
			set[id] = struct{}{}
			return nil
		},
		DeleteSessionFunc: func(id string) error {
			deleted++
			delete(set, id)
			return nil
		},
	}

	// Keep cache small so we can test compression.
	MaxSessionCacheSize = 2

	// Also set a small maximum age.
	SessionCacheExpiry = 10 * time.Millisecond

	// Run some operations.

	// Store one session.
	if err := sessions.Set(&Session{id: "s1", lastAccess: time.Now()}); err != nil {
		t.Error(err)
	} // saved = 1
	tab(1)
	// Get it back.
	if _, err := sessions.Get("s1"); err != nil {
		t.Error(err)
	}
	tab(2)
	// Get a non-existing session.
	s2, err := sessions.Get("s2")
	if err != nil {
		t.Error(err)
	} // loaded = 1
	tab(3)
	if s2 != nil {
		t.Error("s2 should be a nil session")
	}
	// Get an existing session.
	s3, err := sessions.Get("s3")
	if err != nil {
		t.Error(err)
	} // loaded = 2
	tab(4)
	// Let s1 get old.
	time.Sleep(15 * time.Millisecond)
	s3.lastAccess = time.Now()
	// Get a fourth session, drop the old one (s1).
	if _, err := sessions.Get("s4"); err != nil {
		t.Error(err)
	} // loaded = 3, saved = 2
	tab(5)
	// Delete that session.
	if err := sessions.Delete("s4"); err != nil {
		t.Error(err)
	} // deleted = 1
	tab(6)
	// Delete an uncached session.
	if err := sessions.Delete("s1"); err != nil {
		t.Error(err)
	} // deleted = 2
	tab(7)
	// Add a session.
	if err := sessions.Set(&Session{id: "s7", lastAccess: time.Now()}); err != nil {
		t.Error(err)
	} // saved = 3
	tab(8)
	// Add a session.
	if err := sessions.Set(&Session{id: "s8", lastAccess: time.Now()}); err != nil {
		t.Error(err)
	} // saved = 5
	tab(9)
	// Add a session, dropping s7.
	if err := sessions.Set(&Session{id: "s9", lastAccess: time.Now()}); err != nil {
		t.Error(err)
	} // saved = 7
	tab(10)
	// Purge sessions.
	PurgeSessions() // saved = 9
	tab(11)

	// Check results.
	if loaded != 3 {
		t.Errorf("Loaded = %d, expected %d", loaded, 4)
	}
	if saved != 9 {
		t.Errorf("Saved = %d, expected %d", saved, 10)
	}
	if deleted != 2 {
		t.Errorf("Deleted = %d, expected %d", deleted, 4)
	}
	if len(sessions.sessions) != 0 {
		t.Errorf("Cache size = %d, expected %d", len(sessions.sessions), 0)
	}
	if len(set) != 4 {
		t.Errorf("Set size = %d, expected %d", len(set), 4)
	}
	for _, id := range []string{"s3", "s7", "s8", "s9"} {
		if _, ok := set[id]; !ok {
			t.Errorf("%s missing from set", id)
		}
	}
}
