package sessions

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// Session represents a browser session which may persist across multiple HTTP
// requests. A session is usually generated with the Start() function and may
// be destroyed with the Destroy() function.
//
// Sessions are uniquely identified by their session ID. This session ID is
// regenerated, i.e. exchanged, regularly to prevent others from hijacking
// sessions. This can be done explicitly with the RegenerateID() function. And
// it happens automatically based on the rules defined in this package (see
// package variables for details).
//
// The functions for this type are thread-safe.
type Session struct {
	sync.RWMutex
	id                string                 // The session ID. Will not be saved with the session.
	user              User                   // The session user. If nil, no user is attached to this session.
	created           time.Time              // The time when this session was created.
	lastAccess        time.Time              // The last time the session was accessed through this API.
	lastIP            string                 // The remote address (IP:port) of the last request.
	lastUserAgentHash uint64                 // A hash of the remote user agent string of the last request.
	referenceID       string                 // If this session's ID was replaced, this is the ID of the newer session.
	data              map[string]interface{} // Any custom data stored in the session.
}

// Start returns a session for the given HTTP request. Because this function
// may manipulate browser cookies, it must be called before any text is written
// to the response writer.
//
// Sessions are returned from the local cache if contained or loaded into the
// cache first if not.
//
// A nil value may also be returned if "createIfNew" is false and no session was
// previously assigned to this user. Note that if the user's browser rejects
// cookies, this will cause a new session to be created with every request. You
// will also want to respect any privacy laws regarding the use of cookies,
// user and session data.
//
// The following package variables influence the session handling (see their
// comments for details):
//
//   - SessionExpiry
//   - SessionIDExpiry
//   - SessionCookie
//   - NewSessionCookie
func Start(response http.ResponseWriter, request *http.Request, createIfNew bool) (*Session, error) {
	// We may need this hash later.
	var agentHash uint64
	hash := fnv.New64a()
	userAgent := request.Header.Get("User-Agent")
	if userAgent != "" {
		fmt.Fprint(hash, userAgent)
		agentHash = hash.Sum64()
	}

	// Get the session ID from the cookie.
	var id string // The session ID. Empty if it could not be determined.
	cookie, err := request.Cookie(SessionCookie)
	if err == nil {
		id = cookie.Value
	}

	// Get this session from the session cache.
	var session *Session
	if len(id) == 24 {
		// Lock this session ID.
		sessionIDMutexes.Lock(id)
		defer sessionIDMutexes.Unlock(id)

		// Get the session.
		session, err = sessions.Get(id)
		if err != nil {
			return nil, fmt.Errorf("Could not get session from cache: %s", err)
		}

		// If session could not be found, delete the cookie.
		if session == nil {
			deleteCookie(cookie, response)
		}
	}

	if session != nil {
		session.RLock()
		timeUntouched := time.Since(session.lastAccess)
		age := time.Since(session.created)
		ip := session.lastIP
		session.RUnlock()

		// We have a valid session for this user. Check if it's valid.
		valid := true

		// Is it stale?
		if timeUntouched >= SessionExpiry {
			valid = false
		}

		// Has the remote IP changed too much?
		if valid && AcceptRemoteIP > 1 {
			ipFormat := regexp.MustCompile(`^(\d+).(\d+).(\d+).(\d+):\d+$`)
			previousIP := ipFormat.FindStringSubmatch(ip)
			currentIP := ipFormat.FindStringSubmatch(request.RemoteAddr)
			if len(previousIP) == 5 && len(currentIP) == 5 && AcceptRemoteIP <= 4 {
				for i := 1; i < AcceptRemoteIP; i++ {
					if previousIP[i] != currentIP[i] {
						valid = false
						break
					}
				}
			}
		}

		// Has the remote user agent changed?
		if valid && !AcceptChangingUserAgent {
			valid = session.lastUserAgentHash == agentHash
		}

		if !valid {
			// Session is invalid. Delete it.
			if err = session.Destroy(response, request); err != nil {
				return nil, fmt.Errorf("Could not destroy expired session: %s", err)
			}
			session = nil
		} else {
			// It's not stale. Switch IDs?
			if session.referenceID == "" && age >= SessionIDExpiry {
				// Yes, this ID should be replaced.
				err = session.RegenerateID(response)
				if err != nil {
					return nil, err
				}
			} else if age >= SessionIDExpiry+SessionIDGracePeriod {
				// Grace period expired. Remove this session.
				if err = sessions.Delete(id); err != nil {
					return nil, fmt.Errorf("Could not delete session with expired ID: %s", err)
				}

				// Leave the cookie for now, it may be changed by another request. If
				// not, it will be deleted with the next request. In any case, it's
				// illegal to access this session.
				return nil, errors.New("Session expired")
			}

			// If this is a reference session, get the original one.
			if session.referenceID != "" {
				// Redirect cookie to reference session.
				cookie = NewSessionCookie()
				cookie.Name = SessionCookie
				cookie.Value = session.referenceID
				http.SetCookie(response, cookie)

				// Get the referenced session.
				session, err = sessions.Get(session.referenceID)
				if err != nil {
					return nil, fmt.Errorf("Could not get referenced session: %s", err)
				}
				if session == nil {
					return nil, errors.New("Reference session not found")
				}
			}

			// We have a valid session.
			session.Lock()
			defer session.Unlock()
			session.lastAccess = time.Now()
			session.lastIP = request.RemoteAddr
			session.lastUserAgentHash = agentHash
			return session, nil
		}
	}

	if session == nil {
		// We don't have a session for this user.
		if !createIfNew {
			// And we don't want any.
			return nil, nil
		}

		// Create a new session for this user.
		id, err = generateSesssionID()
		if err != nil {
			return nil, fmt.Errorf("Could not generate new session ID: %s", err)
		}
		session = &Session{
			id:                id,
			created:           time.Now(),
			lastAccess:        time.Now(),
			lastIP:            request.RemoteAddr,
			lastUserAgentHash: agentHash,
			data:              make(map[string]interface{}),
		}
		sessions.Set(session)

		// Also set the cookie.
		cookie = NewSessionCookie()
		cookie.Name = SessionCookie
		cookie.Value = id
		http.SetCookie(response, cookie)
	}

	return session, nil
}

// RegenerateID generates a new session ID and replaces it in the current
// session. Use this every time there is a change in user privilege level or a
// related change, e.g. when the user access rights change or when their
// password was changed.
//
// To avoid losing sessions when the network is slow or when many requests for
// the same session ID come in at the same time, the old session (with the old
// key) is turned into a reference session which will be valid for a grace
// period (defined in SessionIDGracePeriod). When that reference session is
// requested, the new session will be returned in its place.
func (s *Session) RegenerateID(response http.ResponseWriter) error {
	// Save this session under a new ID.
	oldID := s.id
	id, err := generateSesssionID()
	if err != nil {
		return fmt.Errorf("Could not generate replacement session ID: %s", err)
	}
	s.Lock()
	s.id = id
	s.created = time.Now()
	s.Unlock()
	if err = sessions.Set(s); err != nil {
		return fmt.Errorf("Could not save session under new session ID: %s", err)
	}

	// Save a reference session under the old ID.
	refSession := &Session{
		id:                oldID,
		created:           s.created,
		lastAccess:        time.Now().Add(-SessionIDExpiry),
		lastIP:            s.lastIP,
		lastUserAgentHash: s.lastUserAgentHash,
		referenceID:       id,
	}
	if err = sessions.Set(refSession); err != nil {
		return fmt.Errorf("Could not save reference session: %s", err)
	}

	// Delete that reference session after the grace period.
	go func() {
		time.Sleep(SessionIDGracePeriod)
		sessions.Delete(oldID)
	}()

	// Change the cookie.
	cookie := NewSessionCookie()
	cookie.Name = SessionCookie
	cookie.Value = id
	http.SetCookie(response, cookie)

	return nil
}

// Destroy marks the end of this session. It is deleted from the session cache,
// the persistence layer, and the user's browser cookie is marked as expired.
//
// The session should not be used anymore after this call.
func (s *Session) Destroy(response http.ResponseWriter, request *http.Request) error {
	// Delete session from cache and persistence layer.
	if err := sessions.Delete(s.id); err != nil {
		return fmt.Errorf("Could not delete session from cache: %s", err)
	}

	// Get the session cookie and delete it.
	cookie, err := request.Cookie(SessionCookie)
	if err != nil {
		return fmt.Errorf("Could not retrieve session cookie: %s", err)
	}
	deleteCookie(cookie, response)

	return nil
}

// deleteCookie deletes a cookie from the user's browser.
func deleteCookie(cookie *http.Cookie, response http.ResponseWriter) {
	delCookie := *cookie
	delCookie.Value = "deleted"
	delCookie.Expires = time.Unix(0, 0)
	delCookie.MaxAge = -1
	http.SetCookie(response, &delCookie)
}

// GobDecode unserializes a session from the given byte array.
func (s *Session) GobDecode(from []byte) error {
	s.Lock()
	defer s.Unlock()

	buffer := bytes.NewReader(from)
	decoder := gob.NewDecoder(buffer)

	// Get version.
	var version uint8
	if err := decoder.Decode(&version); err != nil {
		return fmt.Errorf("Unable to decode session version: %s", err)
	}

	// Creation time.
	if err := decoder.Decode(&s.created); err != nil {
		return fmt.Errorf("Unable to decode session creation time: %s", err)
	}

	// Last access time.
	if err := decoder.Decode(&s.lastAccess); err != nil {
		return fmt.Errorf("Unable to decode session last access time: %s", err)
	}

	// Remote IP.
	if err := decoder.Decode(&s.lastIP); err != nil {
		return fmt.Errorf("Unable to decode session remote IP: %s", err)
	}

	// Hash of remote user agent.
	if err := decoder.Decode(&s.lastUserAgentHash); err != nil {
		return fmt.Errorf("Unable to decode hash of session remote user agent: %s", err)
	}

	// Reference session ID.
	if err := decoder.Decode(&s.referenceID); err != nil {
		return fmt.Errorf("Unable to decode session reference ID: %s", err)
	}

	// User.
	var (
		loggedIn bool
		userID   struct{ V interface{} } // We have to take this detour because decoding interface{} values is tricky.
		e        error
	)
	if err := decoder.Decode(&loggedIn); err != nil {
		return fmt.Errorf("Unable to decode log-in state: %s", err)
	}
	if loggedIn {
		if err := decoder.Decode(&userID); err != nil {
			return fmt.Errorf("Unable to decode user ID: %s", err)
		}
		s.user, e = Persistence.LoadUser(userID.V)
		if e != nil {
			return fmt.Errorf("Failed to load user: %s", e)
		}
	}

	// Custom data.
	if err := decoder.Decode(&s.data); err != nil {
		return fmt.Errorf("Unable to decode session data: %s", err)
	}

	return nil
}

// GobEncode serializes a session to a byte array.
func (s *Session) GobEncode() ([]byte, error) {
	s.RLock()
	defer s.RUnlock()

	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)

	// Add a version number first.
	if err := encoder.Encode(uint8(1)); err != nil {
		return nil, fmt.Errorf("Unable to encode session version: %s", err)
	}

	// Creation time.
	if err := encoder.Encode(s.created); err != nil {
		return nil, fmt.Errorf("Unable to encode session creation time: %s", err)
	}

	// Last access time.
	if err := encoder.Encode(s.lastAccess); err != nil {
		return nil, fmt.Errorf("Unable to encode session last access time: %s", err)
	}

	// Remote IP.
	if err := encoder.Encode(s.lastIP); err != nil {
		return nil, fmt.Errorf("Unable to encode session remote IP: %s", err)
	}

	// Hash of remote user agent.
	if err := encoder.Encode(s.lastUserAgentHash); err != nil {
		return nil, fmt.Errorf("Unable to encode hash of sessions remote user agent: %s", err)
	}

	// Reference session ID.
	if err := encoder.Encode(s.referenceID); err != nil {
		return nil, fmt.Errorf("Unable to encode session reference ID: %s", err)
	}

	// User ID.
	if err := encoder.Encode(s.user != nil); err != nil {
		return nil, fmt.Errorf("Unable to encode log-in state: %s", err)
	}
	if s.user != nil {
		if err := encoder.Encode(struct{ V interface{} }{V: s.user.GetID()}); err != nil {
			return nil, fmt.Errorf("Unable to encode user ID: %s", err)
		}
	}

	// Custom data.
	if err := encoder.Encode(s.data); err != nil {
		return nil, fmt.Errorf("Unable to encode session data: %s", err)
	}

	return buffer.Bytes(), nil
}

// MarshalJSON serializes the session into JSON.
func (s *Session) MarshalJSON() ([]byte, error) {
	s.RLock()
	defer s.RUnlock()

	m := map[string]interface{}{
		"v":  1, // Version
		"cr": s.created.Format(time.RFC3339),
		"la": s.lastAccess.Format(time.RFC3339),
		"ip": s.lastIP,
		"ua": strconv.FormatUint(s.lastUserAgentHash, 36),
		"da": s.data,
	}
	if s.referenceID != "" {
		m["rf"] = s.referenceID
	}
	if s.user != nil {
		m["us"] = s.user.GetID()
	}
	return json.Marshal(m)
}

// UnmarshalJSON unserializes a JSON string into a session.
func (s *Session) UnmarshalJSON(data []byte) error {
	s.Lock()
	defer s.Unlock()

	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	var (
		v, cr, la, da, ip, ua, rf, us  interface{}
		created, lastAccess, agentHash string
		version                        float64
		ok                             bool
		err                            error
	)
	if v, ok = obj["v"]; !ok {
		return errors.New("Missing version number")
	}
	if version, ok = v.(float64); !ok {
		return fmt.Errorf("Invalid version type %T", v)
	}
	if version != 1 {
		return fmt.Errorf("Invalid version: %f", version)
	}
	if cr, ok = obj["cr"]; !ok {
		return errors.New("Missing session creation time")
	}
	if created, ok = cr.(string); !ok {
		return fmt.Errorf("Invalid session creation type %T", cr)
	}
	if s.created, err = time.Parse(time.RFC3339, created); err != nil {
		return fmt.Errorf("Cannot parse session creation time: %s", err)
	}
	if la, ok = obj["la"]; !ok {
		return errors.New("Missing session last access time")
	}
	if lastAccess, ok = la.(string); !ok {
		return fmt.Errorf("Invalid session last access type %T", la)
	}
	if s.lastAccess, err = time.Parse(time.RFC3339, lastAccess); err != nil {
		return fmt.Errorf("Cannot parse session last access time: %s", err)
	}
	if ip, ok = obj["ip"]; !ok {
		return errors.New("Missing session remote IP")
	}
	if s.lastIP, ok = ip.(string); !ok {
		return fmt.Errorf("Invalid session remote IP type %T", ip)
	}
	if ua, ok = obj["ua"]; !ok {
		return errors.New("Missing hash of session remote user agent")
	}
	if agentHash, ok = ua.(string); !ok {
		return fmt.Errorf("Invalid hash of session remote user agent type %T", ua)
	}
	if s.lastUserAgentHash, err = strconv.ParseUint(agentHash, 36, 64); err != nil {
		return fmt.Errorf(`Invalid hash of session remote user agent "%s": %s`, agentHash, err)
	}
	if rf, ok = obj["rf"]; ok {
		if s.referenceID, ok = rf.(string); !ok {
			return fmt.Errorf("Invalid reference ID type %T", rf)
		}
	}
	if us, ok = obj["us"]; ok {
		s.user, err = Persistence.LoadUser(us)
		if err != nil {
			return fmt.Errorf("Error loading user: %s", err)
		}
	}
	if da, ok = obj["da"]; !ok {
		return errors.New("Missing session data")
	}
	if s.data, ok = da.(map[string]interface{}); !ok {
		return fmt.Errorf("Invalid session data type %T", da)
	}
	return nil
}

// Expired returns whether or not this session has expired. This is useful to
// frequently purge the session store.
func (s *Session) Expired() bool {
	s.RLock()
	defer s.RUnlock()
	return s.referenceID != "" && time.Since(s.lastAccess) >= SessionIDGracePeriod ||
		time.Since(s.lastAccess) >= SessionExpiry &&
			time.Since(s.created) >= SessionIDExpiry+SessionIDGracePeriod
}

// LastAccess returns the time this session was last accessed.
func (s *Session) LastAccess() time.Time {
	s.RLock()
	defer s.RUnlock()
	return s.lastAccess
}

// User returns the user for this session or nil if no user is attached to it,
// i.e. if the user is logged out. When checking for nil, it is not enough to
// just check for a nil (User) interface. You may also need to cast the
// interface to your own user type and check if it is nil.
func (s *Session) User() User {
	s.RLock()
	defer s.RUnlock()
	return s.user
}

// LogIn assigns a user to this session, replacing any previously assigned user.
// If "exclusive" is set to true, all other sessions of this user will be
// deleted, effectively logging them out of any existing sessions first. This
// requires that Persistence.UserSessions() returns all of a user's sessions.
//
// A call to this function also causes a session ID change for security reasons.
// It must be called before any non-header content is sent to the browser.
func (s *Session) LogIn(user User, exclusive bool, response http.ResponseWriter) error {
	// First, log user out of existing sessions.
	if exclusive {
		if err := LogOut(user.GetID()); err != nil {
			return fmt.Errorf("Could not log user out of existing sessions: %s", err)
		}
	} else {
		s.LogOut()
	}

	// Log user into this session.
	s.Lock()
	s.user = user
	s.Unlock()
	if err := sessions.Set(s); err != nil {
		return fmt.Errorf("Could not update session cache: %s", err)
	}

	// Switch session ID.
	sessionIDMutexes.Lock(s.id)
	defer sessionIDMutexes.Unlock(s.id)
	if err := s.RegenerateID(response); err != nil {
		return fmt.Errorf("Could not switch session ID: %s", err)
	}

	return nil
}

// Set stores a value under a key in the session which can then be retrieved
// with Get(). Any previous value stored under the same key will be overwritten.
// Note that since the sessions cache is write-through, this will also result in
// a call to SaveSession() of the persistence layer. The error returned is the
// error from SaveSession().
func (s *Session) Set(key string, value interface{}) error {
	s.Lock()
	s.data[key] = value
	s.Unlock()
	return Persistence.SaveSession(s.id, s)
}

// Get returns a value stored in the session under the given key. If the key is
// not contained, the default "def" is returned.
func (s *Session) Get(key string, def interface{}) interface{} {
	s.RLock()
	defer s.RUnlock()
	value, ok := s.data[key]
	if ok {
		return value
	}
	return def
}

// GetAndDelete returns a value stored in the session under the given key. If
// the key is not contained, the default "def" is returned. The key is also
// deleted from the session.
func (s *Session) GetAndDelete(key string, def interface{}) interface{} {
	s.Lock()
	defer s.Unlock()
	value, ok := s.data[key]
	if ok {
		delete(s.data, key)
		return value
	}
	return def
}

// Delete deletes a key from the session. Note that since the sessions cache is
// write-through, this will also result in a call to SaveSession() of the
// persistence layer. The error returned is the error from SaveSession().
func (s *Session) Delete(key string) error {
	s.Lock()
	delete(s.data, key)
	s.Unlock()
	return Persistence.SaveSession(s.id, s)
}

// LogOut logs the currently logged in user out of this session.
//
// Note that the session will still be alive. If you want to destroy the
// current session, too, call Destroy() afterwards.
//
// If no user is logged into this session, nothing happens.
func (s *Session) LogOut() error {
	s.Lock()

	// Do we have a user at all?
	if s.user == nil {
		s.Unlock()
		return nil
	}

	// Log user out of this session.
	s.user = nil
	s.Unlock()

	return Persistence.SaveSession(s.id, s)
}

// LogOut logs the user with the given ID out of all sessions. This requires
// that Persistence.UserSessions() be implemented, returning all IDs of sessions
// that contain this user.
func LogOut(userID interface{}) error {
	// Get all sessions of this user.
	sessionIDs, err := Persistence.UserSessions(userID)
	if err != nil {
		return err
	}

	// Unset user in each session.
	for _, sessionID := range sessionIDs {
		session, err := sessions.Get(sessionID)
		if err != nil {
			return err
		}
		session.Lock()
		session.user = nil
		session.Unlock()
		if err := sessions.Set(session); err != nil {
			return err
		}
	}

	return nil
}

// RefreshUser gets all sessions for the given user and updates their user
// object. This should be done when the user object has changed (e.g. a
// password change). It ensures that all sessions of a user have the same user
// object. This requires that Persistence.UserSessions() be implemented,
// returning all IDs of sessions that contain this user.
//
// Calling this function is not necessary if you don't use the local cache (i.e.
// MaxSessionCacheSize is 0) and if serialized session objects only contain the
// user ID (as it is with the provided default serlization functions GobEncode()
// and MarshalJSON()).
//
// Note that this call will fail if the user ID itself was changed. Such a
// change is more difficult and is not covered here.
func RefreshUser(user User) error {
	// Get all sessions of this user.
	sessionIDs, err := Persistence.UserSessions(user.GetID())
	if err != nil {
		return err
	}

	// Set new user in each session.
	for _, sessionID := range sessionIDs {
		session, err := sessions.Get(sessionID)
		if err != nil {
			return err
		}
		session.Lock()
		session.user = user
		session.Unlock()
		if err := sessions.Set(session); err != nil {
			return err
		}
	}

	return nil
}
