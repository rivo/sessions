package sessions

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

const sessionID = "01234567890123456789----"

// Reset the global parameters.
func reset() {
	Persistence = ExtendablePersistenceLayer{}
	SessionExpiry = math.MaxInt64
	SessionIDExpiry = time.Hour
	SessionIDGracePeriod = 5 * time.Minute
	AcceptRemoteIP = 1
	SessionCookie = "sessionid"
	NewSessionCookie = func() *http.Cookie {
		return &http.Cookie{
			Expires:  time.Now().Add(10 * 365 * 24 * time.Hour),
			MaxAge:   10 * 365 * 24 * 60 * 60,
			HttpOnly: true,
		}
	}
	sessions.sessions = make(map[string]*Session)
}

// Test the gob-part for sessions, including Base64 encoding, without logged-in
// user.
func TestSessionGob(t *testing.T) {
	// Initialize session.
	data := map[string]interface{}{
		"field": "value",
		"42":    13,
		"true":  false,
	}
	date, _ := time.Parse("2006-01-02", "2017-06-27")
	session := &Session{
		user:        nil,
		referenceID: "ABCD",
		created:     date,
		lastAccess:  date,
		lastIP:      "192.168.178.1:80",
		data:        data,
	}

	// Serialize to Base64.
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	if err := encoder.Encode(session); err != nil {
		t.Error(err)
	}
	s := base64.StdEncoding.EncodeToString(buffer.Bytes())
	t.Logf("Serialized session: %s (length: %d)", s, len(s))

	// Deserialize.
	d, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Error(err)
	}
	r := bytes.NewReader(d)
	decoder := gob.NewDecoder(r)
	var recoveredSession Session
	if err := decoder.Decode(&recoveredSession); err != nil {
		t.Error(err)
	}

	// Compare sessions.
	if !recoveredSession.created.Equal(session.created) {
		t.Errorf("Recovered session has different creation time (%s) than expected (%s)", recoveredSession.created, session.created)
	}
	if !recoveredSession.lastAccess.Equal(session.lastAccess) {
		t.Errorf("Recovered session has different last access time (%s) than expected (%s)", recoveredSession.lastAccess, session.lastAccess)
	}
	if recoveredSession.referenceID != session.referenceID {
		t.Errorf("Recovered session has different reference ID (%s) than expected (%s)", recoveredSession.referenceID, session.referenceID)
	}
	if recoveredSession.User() != nil {
		t.Errorf("Recovered session has a user (%v) instead of nil", recoveredSession.user)
	}
	if len(recoveredSession.data) != len(session.data) {
		t.Errorf("Recovered session data has different size (%d) than expected (%d)", len(recoveredSession.data), len(session.data))
	}
	for field, value := range data {
		recoveredValue, ok := recoveredSession.data[field]
		if !ok {
			t.Errorf("Field %s not in recovered session data", field)
			continue
		}
		if recoveredValue != value {
			t.Errorf("Value %s for field %s not as expected (%s)", recoveredValue, field, value)
		}
	}
}

// Test the JSON-part for sessions, without logged-in user.
func TestSessionJSON(t *testing.T) {
	// Initialize session.
	data := map[string]interface{}{
		"field": "value",
		"42":    nil,
		"true":  false,
	}
	date, _ := time.Parse("2006-01-02", "2017-06-27")
	session := &Session{
		user:              nil,
		referenceID:       "ABCD",
		created:           date,
		lastAccess:        date,
		lastIP:            "192.168.178.1:80",
		lastUserAgentHash: 12345,
		data:              data,
	}

	// Serialize to JSON.
	j, err := json.Marshal(session)
	if err != nil {
		t.Error(err)
	}
	t.Logf("JSON session: %s", j)

	// Unserialize.
	recoveredSession := &Session{}
	if err := json.Unmarshal(j, recoveredSession); err != nil {
		t.Error(err)
	}

	// Compare sessions.
	if !recoveredSession.created.Equal(session.created) {
		t.Errorf("Recovered session has different creation time (%s) than expected (%s)", recoveredSession.created, session.created)
	}
	if !recoveredSession.lastAccess.Equal(session.lastAccess) {
		t.Errorf("Recovered session has different last access time (%s) than expected (%s)", recoveredSession.lastAccess, session.lastAccess)
	}
	if recoveredSession.referenceID != session.referenceID {
		t.Errorf("Recovered session has different reference ID (%s) than expected (%s)", recoveredSession.referenceID, session.referenceID)
	}
	if recoveredSession.lastIP != session.lastIP {
		t.Errorf("Recovered session has different IP (%s) than expected (%s)", recoveredSession.lastIP, session.lastIP)
	}
	if recoveredSession.lastUserAgentHash != session.lastUserAgentHash {
		t.Errorf("Recovered session has different user agent hash (%d) than expected (%d)", recoveredSession.lastUserAgentHash, session.lastUserAgentHash)
	}
	if recoveredSession.User() != nil {
		t.Errorf("Recovered session has a user (%v) instead of nil", recoveredSession.user)
	}
	if len(recoveredSession.data) != len(session.data) {
		t.Errorf("Recovered session data has different size (%d) than expected (%d)", len(recoveredSession.data), len(session.data))
	}
	for field, value := range data {
		recoveredValue, ok := recoveredSession.data[field]
		if !ok {
			t.Errorf("Field %s not in recovered session data", field)
			continue
		}
		if recoveredValue != value {
			t.Errorf("Value %s for field %s not as expected (%s)", recoveredValue, field, value)
		}
	}
}

// Test the gob-part for sessions, including Base64 encoding, with a logged-in
// user.
func TestSessionGobWithUser(t *testing.T) {
	// Initialize session.
	user := &TestUser{ID: "12345"}
	data := map[string]interface{}{
		"field": "value",
		"42":    13,
		"true":  false,
	}
	date, _ := time.Parse("2006-01-02", "2017-06-27")
	session := &Session{
		user:        user,
		referenceID: "ABCD",
		created:     date,
		lastAccess:  date,
		lastIP:      "192.168.178.1:80",
		data:        data,
	}
	Persistence = ExtendablePersistenceLayer{
		LoadUserFunc: func(id interface{}) (User, error) {
			if id != "12345" {
				return nil, fmt.Errorf("Requested invalid user ID: %s", id)
			}
			return user, nil
		},
	}

	// Serialize to Base64.
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	if err := encoder.Encode(session); err != nil {
		t.Error(err)
	}
	s := base64.StdEncoding.EncodeToString(buffer.Bytes())
	t.Logf("Serialized session: %s (length: %d)", s, len(s))

	// Deserialize.
	d, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Error(err)
	}
	r := bytes.NewReader(d)
	decoder := gob.NewDecoder(r)
	var recoveredSession Session
	if err := decoder.Decode(&recoveredSession); err != nil {
		t.Error(err)
	}

	// Compare sessions.
	if !recoveredSession.created.Equal(session.created) {
		t.Errorf("Recovered session has different creation time (%s) than expected (%s)", recoveredSession.created, session.created)
	}
	if !recoveredSession.lastAccess.Equal(session.lastAccess) {
		t.Errorf("Recovered session has different last access time (%s) than expected (%s)", recoveredSession.lastAccess, session.lastAccess)
	}
	if recoveredSession.referenceID != session.referenceID {
		t.Errorf("Recovered session has different reference ID (%s) than expected (%s)", recoveredSession.referenceID, session.referenceID)
	}
	if recoveredSession.lastIP != session.lastIP {
		t.Errorf("Recovered session has different IP (%s) than expected (%s)", recoveredSession.lastIP, session.lastIP)
	}
	if recoveredSession.lastUserAgentHash != session.lastUserAgentHash {
		t.Errorf("Recovered session has different user agent hash (%d) than expected (%d)", recoveredSession.lastUserAgentHash, session.lastUserAgentHash)
	}
	if recoveredSession.User() != session.User() {
		t.Errorf("Recovered session has different user (%v) than expected (%v)", recoveredSession.user, session.user)
	}
	if len(recoveredSession.data) != len(session.data) {
		t.Errorf("Recovered session data has different size (%d) than expected (%d)", len(recoveredSession.data), len(session.data))
	}
	for field, value := range data {
		recoveredValue, ok := recoveredSession.data[field]
		if !ok {
			t.Errorf("Field %s not in recovered session data", field)
			continue
		}
		if recoveredValue != value {
			t.Errorf("Value %s for field %s not as expected (%s)", recoveredValue, field, value)
		}
	}
}

// Test the JSON-part for sessions, with a logged-in user.
func TestSessionJSONWithUser(t *testing.T) {
	// Initialize session.
	user := &TestUser{ID: "12345"}
	data := map[string]interface{}{
		"field": "value",
		"42":    nil,
		"true":  false,
	}
	date, _ := time.Parse("2006-01-02", "2017-06-27")
	session := &Session{
		user:        user,
		referenceID: "ABCD",
		created:     date,
		lastAccess:  date,
		lastIP:      "192.168.178.1:80",
		data:        data,
	}
	Persistence = ExtendablePersistenceLayer{
		LoadUserFunc: func(id interface{}) (User, error) {
			if id != "12345" {
				return nil, fmt.Errorf("Requested invalid user ID: %s", id)
			}
			return user, nil
		},
	}

	// Serialize to JSON.
	j, err := json.Marshal(session)
	if err != nil {
		t.Error(err)
	}
	t.Logf("JSON session: %s", j)

	// Unserialize.
	recoveredSession := &Session{}
	if err := json.Unmarshal(j, recoveredSession); err != nil {
		t.Error(err)
	}

	// Compare sessions.
	if !recoveredSession.created.Equal(session.created) {
		t.Errorf("Recovered session has different creation time (%s) than expected (%s)", recoveredSession.created, session.created)
	}
	if !recoveredSession.lastAccess.Equal(session.lastAccess) {
		t.Errorf("Recovered session has different last access time (%s) than expected (%s)", recoveredSession.lastAccess, session.lastAccess)
	}
	if recoveredSession.referenceID != session.referenceID {
		t.Errorf("Recovered session has different reference ID (%s) than expected (%s)", recoveredSession.referenceID, session.referenceID)
	}
	if recoveredSession.User() != session.User() {
		t.Errorf("Recovered session has different user (%v) than expected (%v)", recoveredSession.user, session.user)
	}
	if len(recoveredSession.data) != len(session.data) {
		t.Errorf("Recovered session data has different size (%d) than expected (%d)", len(recoveredSession.data), len(session.data))
	}
	for field, value := range data {
		recoveredValue, ok := recoveredSession.data[field]
		if !ok {
			t.Errorf("Field %s not in recovered session data", field)
			continue
		}
		if recoveredValue != value {
			t.Errorf("Value %s for field %s not as expected (%s)", recoveredValue, field, value)
		}
	}
}

// Session start returns no session.
func TestNoSession(t *testing.T) {
	req := httptest.NewRequest("", "/", nil)
	session, err := Start(nil, req, false)
	if err != nil {
		t.Error(err)
	}
	if session != nil {
		t.Error("Expected nil session, received non-empty session")
	}
}

// Session start returns no session because it doesn't exist.
func TestNonExistingSession(t *testing.T) {
	req := httptest.NewRequest("", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sessionID})
	res := httptest.NewRecorder()
	session, err := Start(res, req, false)
	if err != nil {
		t.Error(err)
	}
	if session != nil {
		t.Error("Expected nil session, received non-empty session")
	}
	if !strings.Contains(res.Header().Get("Set-Cookie"), fmt.Sprintf("%s=deleted", SessionCookie)) {
		t.Error("Cookie was not deleted")
	}
}

// Session start returns anonymous session.
func TestAnonSession(t *testing.T) {
	defer reset()
	NewSessionCookie = func() *http.Cookie {
		return &http.Cookie{
			Expires:  time.Now(),
			MaxAge:   10,
			HttpOnly: true,
		}
	}
	req := httptest.NewRequest("", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sessionID})
	res := httptest.NewRecorder()
	session, err := Start(res, req, true)
	if err != nil {
		t.Error(err)
	}
	if session == nil {
		t.Error("Expected session, received nil")
		return
	}
	if len(sessions.sessions) != 1 {
		t.Error("Cache is not size 1")
	}
	cookie := regexp.MustCompile("^" + SessionCookie + "=[0-9a-zA-Z=+/]{24}")
	t.Log(res.Header())
	header := res.Header()
	cookies := header["Set-Cookie"]
	lastCookie := cookies[len(cookies)-1]
	if !cookie.MatchString(lastCookie) {
		t.Error("Cookie was not set")
	}
}

// Session start returns an existing session.
func TestExistingSession(t *testing.T) {
	defer reset()
	Persistence = ExtendablePersistenceLayer{
		LoadSessionFunc: func(id string) (*Session, error) {
			if id != sessionID {
				return nil, fmt.Errorf("Requested wrong session: %s", id)
			}
			return &Session{created: time.Now().Add(-time.Minute), lastAccess: time.Now().Add(-time.Minute), data: map[string]interface{}{"test": true}}, nil
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
	if _, ok := session.data["test"]; !ok {
		t.Error("Did not receive expected session")
	}
}

// Session start returns an expired session.
func TestExpiredSession(t *testing.T) {
	defer reset()
	SessionExpiry = 0
	Persistence = ExtendablePersistenceLayer{
		LoadSessionFunc: func(id string) (*Session, error) {
			if id != sessionID {
				return nil, fmt.Errorf("Requested wrong session: %s", id)
			}
			return &Session{created: time.Now().Add(-time.Minute), lastAccess: time.Now()}, nil
		},
	}
	req := httptest.NewRequest("", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sessionID})
	res := httptest.NewRecorder()
	session, err := Start(res, req, false)
	if err != nil {
		t.Error(err)
	}
	if session != nil {
		t.Error("Expected nil session, received non-empty session")
	}
}

// Session start performs a session ID change.
func TestSessionIDChange(t *testing.T) {
	defer reset()
	SessionIDGracePeriod = 5 * time.Millisecond
	var deleted, saved int
	Persistence = ExtendablePersistenceLayer{
		LoadSessionFunc: func(id string) (*Session, error) {
			if id != sessionID {
				return nil, fmt.Errorf("Requested wrong session: %s", id)
			}
			return &Session{
				created:    time.Now().Add(-2 * time.Hour),
				lastAccess: time.Now().Add(-2 * time.Hour),
				data:       map[string]interface{}{"test": true},
			}, nil
		},
		SaveSessionFunc: func(id string, session *Session) error {
			saved++
			return nil
		},
		DeleteSessionFunc: func(id string) error {
			deleted++
			if id != sessionID {
				return fmt.Errorf("Deleting wrong session ID: %s", id)
			}
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
	if _, ok := session.data["test"]; !ok {
		t.Error("Did not receive expected session")
	}
	cookie := regexp.MustCompile("^" + SessionCookie + "=[0-9a-zA-Z=+/]{24}")
	if !cookie.MatchString(res.Header().Get("Set-Cookie")) {
		t.Error("Cookie was not updated")
	}
	time.Sleep(10 * time.Millisecond)
	if deleted != 1 {
		t.Error("Old session was not deleted")
	}
	if saved != 2 {
		t.Error("New session was not saved")
	}
	// Cover the expiry function.
	if session.Expired() {
		t.Error("Session has expired although it shouldn't have")
	}
}

// Session start returns referenced session.
func TestReferencedSession(t *testing.T) {
	defer reset()
	SessionIDGracePeriod = 10 * time.Millisecond
	Persistence = ExtendablePersistenceLayer{
		LoadSessionFunc: func(id string) (*Session, error) {
			if id == sessionID {
				return &Session{
					referenceID: "ABCDEFGHIJKLMNOPQRSTUVWX",
					created:     time.Now().Add(-5 * time.Millisecond),
					lastAccess:  time.Now().Add(-5 * time.Millisecond),
					data:        nil,
				}, nil
			}
			return &Session{
				created:    time.Now(),
				lastAccess: time.Now(),
				data:       map[string]interface{}{"test": true},
			}, nil
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
	if _, ok := session.data["test"]; !ok {
		t.Error("Did not receive expected session")
	}
	if !strings.Contains(res.Header().Get("Set-Cookie"), fmt.Sprintf("%s=ABCDEFGHIJKLMNOPQRSTUVWX", SessionCookie)) {
		t.Error("Cookie was not updated")
	}
}

// Session start detects that the reference session has expired.
func TestExpiredReferencedSession(t *testing.T) {
	defer reset()
	SessionIDGracePeriod = 10 * time.Millisecond
	Persistence = ExtendablePersistenceLayer{
		LoadSessionFunc: func(id string) (*Session, error) {
			if id != sessionID {
				return nil, errors.New("Wrong session ID")
			}
			return &Session{
				referenceID: "ABCDEFGHIJKLMNOPQRSTUVWX",
				created:     time.Now().Add(-SessionIDExpiry - 2*SessionIDGracePeriod),
				lastAccess:  time.Now().Add(-SessionIDExpiry - 2*SessionIDGracePeriod),
				data:        nil,
			}, nil
		},
	}
	req := httptest.NewRequest("", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sessionID})
	res := httptest.NewRecorder()
	_, err := Start(res, req, false)
	if err == nil {
		t.Error("Expected failure due to expired reference session, no failure however")
	}
}

// Try requesting lots of session ID changes at once, hoping to get multiple new
// sessions.
func TestSessionIDChangeDoS(t *testing.T) {
	defer reset()
	SessionIDGracePeriod = 5 * time.Millisecond
	var deleted, saved int
	Persistence = ExtendablePersistenceLayer{
		LoadSessionFunc: func(id string) (*Session, error) {
			if id != sessionID {
				return nil, fmt.Errorf("Requested wrong session: %s", id)
			}
			return &Session{
				created:    time.Now().Add(-time.Hour - 2*time.Millisecond),
				lastAccess: time.Now().Add(-time.Hour - 2*time.Millisecond),
				data:       map[string]interface{}{"test": true},
			}, nil
		},
		SaveSessionFunc: func(id string, session *Session) error {
			saved++
			return nil
		},
		DeleteSessionFunc: func(id string) error {
			deleted++
			if id != sessionID {
				return fmt.Errorf("Deleting wrong session ID: %s", id)
			}
			return nil
		},
	}
	var (
		sessions []*Session
		mutex    sync.Mutex
		wg       sync.WaitGroup
	)
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("", "/", nil)
			req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sessionID})
			res := httptest.NewRecorder()
			session, err := Start(res, req, false)
			if err != nil {
				t.Error(err)
				return
			}
			mutex.Lock()
			sessions = append(sessions, session)
			mutex.Unlock()
		}()
	}
	wg.Wait()

	// We should have received the same object.
	for index, session := range sessions {
		if index == 0 {
			if session == nil {
				t.Error("Expected session, received nil")
				return
			}
			if _, ok := session.data["test"]; !ok {
				t.Error("Did not receive expected session")
				return
			}
		} else {
			if session != sessions[0] {
				t.Errorf("Received a different session: %d (%v)", index, session)
			}
		}
	}
	time.Sleep(10 * time.Millisecond)
	if deleted != 1 {
		t.Errorf("Old session was not deleted: %d", deleted)
	}
	if saved != 2 {
		t.Error("New session was not saved")
	}
}

// Test remote IP with a valid IP change.
func TestSessionValidRemoteIP(t *testing.T) {
	defer reset()
	AcceptRemoteIP = 3
	Persistence = ExtendablePersistenceLayer{
		LoadSessionFunc: func(id string) (*Session, error) {
			return &Session{
				created:    time.Now(),
				lastAccess: time.Now(),
				lastIP:     "192.168.178.1:80",
				data:       nil,
			}, nil
		},
	}
	req := httptest.NewRequest("", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sessionID})
	req.RemoteAddr = "192.168.100.50:8080"
	res := httptest.NewRecorder()
	session, err := Start(res, req, false)
	if err != nil {
		t.Error(err)
		return
	}
	if session == nil {
		t.Error("Nil session returned, regular session expected")
	}
}

// Test remote IP with an invalid IP change.
func TestSessionInvalidRemoteIP(t *testing.T) {
	defer reset()
	AcceptRemoteIP = 3
	Persistence = ExtendablePersistenceLayer{
		LoadSessionFunc: func(id string) (*Session, error) {
			return &Session{
				created:    time.Now(),
				lastAccess: time.Now(),
				lastIP:     "192.168.178.1:80",
				data:       nil,
			}, nil
		},
	}
	req := httptest.NewRequest("", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sessionID})
	req.RemoteAddr = "192.100.100.50:8080"
	res := httptest.NewRecorder()
	session, err := Start(res, req, false)
	if err != nil {
		t.Error(err)
		return
	}
	if session != nil {
		t.Error("Session returned, nil session expected")
	}
}

// Test remote user agent with a valid user agent change.
func TestSessionValidRemoteUserAgent(t *testing.T) {
	defer reset()
	AcceptRemoteIP = 3
	Persistence = ExtendablePersistenceLayer{
		LoadSessionFunc: func(id string) (*Session, error) {
			return &Session{
				created:           time.Now(),
				lastAccess:        time.Now(),
				lastIP:            "192.168.178.1:80",
				lastUserAgentHash: 2838198717544347415,
				data:              nil,
			}, nil
		},
	}
	req := httptest.NewRequest("", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sessionID})
	req.RemoteAddr = "192.168.178.1:80"
	req.Header.Add("User-Agent", "My User Agent")
	res := httptest.NewRecorder()
	session, err := Start(res, req, false)
	if err != nil {
		t.Error(err)
		return
	}
	if session == nil {
		t.Error("Nil session returned, regular session expected")
	}
}

// Test remote user agent with an invalid user agent change.
func TestSessionInvalidRemoteUserAgent(t *testing.T) {
	defer reset()
	AcceptRemoteIP = 3
	Persistence = ExtendablePersistenceLayer{
		LoadSessionFunc: func(id string) (*Session, error) {
			return &Session{
				created:           time.Now(),
				lastAccess:        time.Now(),
				lastIP:            "192.168.178.1:80",
				lastUserAgentHash: 2838198717544347415,
				data:              nil,
			}, nil
		},
	}
	req := httptest.NewRequest("", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sessionID})
	req.RemoteAddr = "192.168.178.1:80"
	req.Header.Add("User-Agent", "Not my User Agent")
	res := httptest.NewRecorder()
	session, err := Start(res, req, false)
	if err != nil {
		t.Error(err)
		return
	}
	if session != nil {
		t.Error("Session returned, nil session expected")
	}
}

// Test session data storage.
func TestSessionData(t *testing.T) {
	defer reset()
	req := httptest.NewRequest("", "/", nil)
	res := httptest.NewRecorder()
	session, err := Start(res, req, true)
	if err != nil {
		t.Error(err)
	}
	if session == nil {
		t.Error("Expected session, received none")
		return
	}
	if err := session.Set("key1", true); err != nil {
		t.Error(err)
		return
	}
	if err := session.Set("key1", false); err != nil {
		t.Error(err)
		return
	}
	if err := session.Set("key2", "value"); err != nil {
		t.Error(err)
		return
	}
	if err := session.Set("key3", nil); err != nil {
		t.Error(err)
		return
	}
	if err := session.Delete("key3"); err != nil {
		t.Error(err)
		return
	}
	val1 := session.Get("key1", nil)
	if val1 == nil {
		t.Error("key1 value was not found")
		return
	}
	if b, ok := val1.(bool); !ok {
		t.Error("key1 is not bool")
		return
	} else if b {
		t.Error("key1 was not overwritten")
	}
	val2 := session.Get("key2", nil)
	if val2 == nil {
		t.Error("key2 value was not found")
		return
	}
	if s, ok := val2.(string); !ok {
		t.Error("key2 is not a string")
		return
	} else if s != "value" {
		t.Errorf("key2 is %s, not 'value'", s)
	}
	val3 := session.Get("key3", "key").(string)
	if val3 != "key" {
		t.Error("key3 value is still stored")
		return
	}
	val1a := session.GetAndDelete("key1", nil)
	if val1a == nil {
		t.Error("key1 value was not found")
		return
	}
	val1b := session.GetAndDelete("key1", nil)
	if val1b != nil {
		t.Error("key1 is still stored")
		return
	}
}
