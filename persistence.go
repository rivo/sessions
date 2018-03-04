package sessions

// PersistenceLayer provides the methods which read/write user information
// from/to the permanent data store.
type PersistenceLayer interface {
	// LoadSession retrieves a session from the permanent data store and returns
	// it. If no session is found for the given ID, that's not an error. A nil
	// session should be returned in that case.
	//
	// Session stores are typically key-value databases. We can use encoding/gob
	// to unserialize sessions. For example, if your session store accepts only
	// string pairs, this is how we can load a Base64-encoded session:
	//
	//     func LoadSession(id string) (*Session, error) {
	//       // Load Base64-string s from database first...
	//       data, err := base64.StdEncoding.DecodeString(s)
	//       if err != nil {
	//         return nil, err
	//       }
	//       r := bytes.NewReader(data)
	//       decoder := gob.NewDecoder(r)
	//       var session Session
	//       if err := decoder.Decode(&session); err != nil {
	//         return nil, err
	//       }
	//       return &session, nil
	//     }
	//
	// Alternatively, the json.Unmarshaler interface may be used.
	//
	// When using the built-in decoders (gob or json) and a User was attached to
	// the session, LoadUser() is called implicitly with the stored user ID.
	LoadSession(id string) (*Session, error)

	// SaveSession saves a session to the permanent data store. If the store does
	// not contain the session yet, it is inserted. Otherwise, it is simply
	// updated. Session stores are typically key-value databases. We can use
	// encoding/gob to serialize sessions. For example, if your session store
	// accepts only string pairs, this is how we can save a Base64-encoded
	// session:
	//
	//     func SaveSession(id string, session *Session) error {
	//     	 var buffer bytes.Buffer
	//     	 encoder := gob.NewEncoder(&buffer)
	//     	 if err := encoder.Encode(session); err != nil {
	//     	 	return err
	//     	 }
	//     	 s := base64.StdEncoding.EncodeToString(buffer.Bytes())
	//     	 // Now save id + s to database.
	//     	 return nil
	//     }
	//
	// Alternatively, the json.Marshaler interface may be used. Note, however,
	// that while JSON serialization allows you to peek into the serialized data,
	// it may not convert values back the same as they were stored: Any numeric
	// values will convert back as float64 types, all slices will convert back
	// as []interface{}, and all maps will convert back as map[string]interface{}.
	//
	// The internal encoders (gob or json) do not save the full User object but
	// only the user ID.
	//
	// Session IDs are always Base64-encoded strings with a length of 24.
	//
	// The session object is locked while this function is called.
	SaveSession(id string, session *Session) error

	// DeleteSession deletes a session from the permanent data store. It is not
	// an error if the session ID does not exist.
	//
	// Note that this package only deletes expired sessions that are accessed. If
	// a session expires because e.g. the user does not come back, it will not
	// be deleted via this method. It is suggested that you periodically run a
	// cron job to purge sessions that have expired. Use session.Expired() for
	// this or, if you can access session data directly:
	//
	//   session.referenceID != "" &&
	//   time.Since(session.lastAccess) >= SessionIDGracePeriod ||
	//   time.Since(session.lastAccess) >= SessionExpiry &&
	//   time.Since(session.created) >= SessionIDExpiry+SessionIDGracePeriod
	DeleteSession(id string) error

	// UserSessions returns all session IDs of sessions which have the given user
	// (specified by their user ID) attached to them. This is only used to log
	// users out of all of their existing sessions. You may return nil, which will
	// allow users to be logged on with multiple different sessions at the same
	// time.
	UserSessions(userID interface{}) ([]string, error)

	// LoadUser loads the user with the given unqiue user ID (typically the
	// primary key) from the data store.
	LoadUser(id interface{}) (User, error)
}

// ExtendablePersistenceLayer implements the PersistenceLayer interface by doing
// nothing (or the absolute minimum) or, if one of the field functions are set,
// calling those instead.
//
// Use this type if you only intend to use a small part of this package's
// functionality.
type ExtendablePersistenceLayer struct {
	LoadSessionFunc   func(id string) (*Session, error)
	SaveSessionFunc   func(id string, session *Session) error
	DeleteSessionFunc func(id string) error
	UserSessionsFunc  func(userID interface{}) ([]string, error)
	LoadUserFunc      func(id interface{}) (User, error)
}

// LoadSession delegates to LoadSessionFunc or returns a nil session.
func (p ExtendablePersistenceLayer) LoadSession(id string) (*Session, error) {
	if p.LoadSessionFunc != nil {
		return p.LoadSessionFunc(id)
	}
	return nil, nil
}

// SaveSession delegates to SaveSessionFunc or does nothing.
func (p ExtendablePersistenceLayer) SaveSession(id string, session *Session) error {
	if p.SaveSessionFunc != nil {
		return p.SaveSessionFunc(id, session)
	}
	return nil
}

// DeleteSession delegates to DeleteSessionFunc or does nothing.
func (p ExtendablePersistenceLayer) DeleteSession(id string) error {
	if p.DeleteSessionFunc != nil {
		return p.DeleteSessionFunc(id)
	}
	return nil
}

// UserSessions delegates to UserSessionsFunc or returns nil.
func (p ExtendablePersistenceLayer) UserSessions(userID interface{}) ([]string, error) {
	if p.UserSessionsFunc != nil {
		return p.UserSessionsFunc(userID)
	}
	return nil, nil
}

// LoadUser delegates to LoadUserFunc or returns a nil user.
func (p ExtendablePersistenceLayer) LoadUser(id interface{}) (User, error) {
	if p.LoadUserFunc != nil {
		return p.LoadUserFunc(id)
	}
	return nil, nil
}
