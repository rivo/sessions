package sessions

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestUser is a test class for users.
type TestUser struct {
	ID   string
	Item string
}

// Return the user ID.
func (u *TestUser) GetID() interface{} {
	return u.ID
}

// Return the user's roles.
func (u *TestUser) GetRoles() []string {
	return nil
}

// Test login.
func TestUserLogin(t *testing.T) {
	defer reset()
	var saved int
	Persistence = ExtendablePersistenceLayer{
		LoadSessionFunc: func(id string) (*Session, error) {
			if id != sessionID {
				return nil, fmt.Errorf("Requested wrong session: %s", id)
			}
			return &Session{
				created:    time.Now().Add(-2 * time.Minute),
				lastAccess: time.Now().Add(-2 * time.Minute),
			}, nil
		},
		SaveSessionFunc: func(id string, session *Session) error {
			saved++
			return nil
		},
	}
	req := httptest.NewRequest("", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sessionID})
	res := httptest.NewRecorder()
	session, err := Start(res, req, false)
	if err != nil {
		t.Error(err)
	}
	if session == nil {
		t.Error("Expected session, received nil")
		return
	}
	user := &TestUser{}
	if err := session.LogIn(user, true, res); err != nil {
		t.Error(err)
	}
	if saved != 3 { // 1 from log in, 2 from switch ID.
		t.Errorf("New session was not saved (%d)", saved)
	}
	if session.User() != User(user) {
		t.Error("User was not logged in")
		return
	}
}

// Test logout.
func TestUserLogout(t *testing.T) {
	defer reset()
	user := &TestUser{ID: "userid"}
	Persistence = ExtendablePersistenceLayer{
		LoadSessionFunc: func(id string) (*Session, error) {
			return &Session{
				user:       user,
				created:    time.Now().Add(-2 * time.Minute),
				lastAccess: time.Now().Add(-2 * time.Minute),
			}, nil
		},
		UserSessionsFunc: func(userID interface{}) ([]string, error) {
			return []string{sessionID, "1", "2", "3"}, nil
		},
	}
	req := httptest.NewRequest("", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sessionID})
	res := httptest.NewRecorder()
	session, err := Start(res, req, false)
	if err != nil {
		t.Error(err)
	}
	if session == nil {
		t.Error("Expected session, received nil")
		return
	}
	if err := session.LogOut(); err != nil {
		// We don't need this but let's cover it anyway.
		t.Error(err)
		return
	}
	if err := LogOut(user.ID); err != nil {
		t.Error(err)
		return
	}
	for id, session := range sessions.sessions {
		if session.user != nil {
			t.Errorf("User still logged into session %s", id)
		}
	}
}

// Testing a refresh of all sessions of a given user.
func TestUserRefresh(t *testing.T) {
	defer reset()
	user := &TestUser{ID: "userid", Item: "item"}
	Persistence = ExtendablePersistenceLayer{
		LoadSessionFunc: func(id string) (*Session, error) {
			return &Session{
				user:       user,
				created:    time.Now().Add(-2 * time.Minute),
				lastAccess: time.Now().Add(-2 * time.Minute),
			}, nil
		},
		UserSessionsFunc: func(userID interface{}) ([]string, error) {
			return []string{sessionID, "1", "2", "3"}, nil
		},
	}
	req := httptest.NewRequest("", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sessionID})
	res := httptest.NewRecorder()
	session, err := Start(res, req, false)
	if err != nil {
		t.Error(err)
	}
	if session == nil {
		t.Error("Expected session, received nil")
		return
	}
	modifiedUser := session.User().(*TestUser)
	modifiedUser.Item = "newitem"
	if e := RefreshUser(modifiedUser); e != nil {
		t.Error(e)
		return
	}
	PurgeSessions()

	sessionIDs, err := Persistence.UserSessions("userid")
	if err != nil {
		t.Error(err)
		return
	}
	if len(sessionIDs) != 4 {
		t.Errorf("Invalid number of sessions: %d", len(sessionIDs))
		return
	}
	for _, sessionID := range sessionIDs {
		session, err := sessions.Get(sessionID)
		if err != nil {
			t.Error(err)
			return
		}
		item := session.user.(*TestUser).Item
		if item != "newitem" {
			t.Errorf("Found a user with the wrong item: %s", item)
		}
	}
}
