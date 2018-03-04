/*
Package sessions provides tools to manage cookie-based web sessions. Special
emphasis is placed on security by implementing OWASP recommendations,
specifically the following features:

  - No data storage on the client
  - Automatic session expiry
  - Session ID regeneration
  - Anomaly detection via IP address and user agent analysis

In addition, the package provides the following functionality:

  - Session key/value storage
  - Log in/out functions for users
  - Various identifier generation functions
  - Password strength checks (based on NIST recommendations)

While simple to use, the package offers a number of extensively documented
configuration variables. It also does not assume specific backend technologies.
That is, any session storage system may be used simply by implementing the
PersistenceLayer interface (or parts of it).

This package is currently not written to be run on multiple machines in a
distributed fashion without a load balancer that implements sticky sessions.
This may change in the future.

Basic Example

Although some more configuration needs to happen for production readiness, the
package's defaults allow you to get started very quickly. To get access to the
current session, simply call Start():

  func MyHandler(response http.ResponseWriter, request *http.Request) {
  	session, err := sessions.Start(response, request, false)
  	if err != nil {
  		panic(err)
  	}
  	if session != nil {
  		fmt.Println("We have a session")
  	} else {
  		fmt.Println("We have no session")
  	}
  }

By providing "true" instead of "false" to the Start() function, you can force
the creation of a session, even if there previously was none.

Once you have a session, you can identify a user across multiple HTTP requests.
You may add values to the session, attach a user to it, cause its session ID
to change, or destroy it again. For more extensive user-centered functions
(for example, signing up, logging in and out, changing passwords etc.), see the
subdirectory "users".

Configuration

Before putting your application into production, you must implement the
NewSessionCookie function:

  NewSessionCookie = func() *http.Cookie {
    return &http.Cookie{
      Expires:  time.Now().Add(10 * 365 * 24 * time.Hour),
      MaxAge:   10 * 365 * 24 * 60 * 60,
      HttpOnly: true,
      Domain:   "www.example.com",
      Path:     "/",
      Secure:   true,
    }
  }

You may choose a different expiry date, domain, and path but the other fields
are mandatory (given that you are using TLS which you certainly should).

You can change the name of the cookie by changing the SessionCookie variable.
The default is the inconspicuous string "id".

The following timeout values may be adjusted according to the requirements of
your application:

  - SessionExpiry: The maximum time which may pass before a session that has not
    been accessed will be destroyed. The default is "forever", meaning unused
    sessions will not time out.
  - SessionIDExpiry: The maximum duration a session ID can be used before it is
    changed to a new session ID. Session ID renewals reduce the risk of session
    hijacking attacks.
  - SessionIDGracePeriod: Session ID renewals require the previous session ID
    to remain active for some time so sessions don't get lost, e.g. because of
    a slow network. This variable specifies how long a previous session ID
    remains active when a new session ID is already in place.

To further reduce the risk of session hijacking attacks, this package checks
client IP addresses as well as user agent strings and destroys sessions if
changes in these properties were detected. Refer to the AcceptRemoteIP and
AcceptChangingUserAgent variables for more information.

The Session Cache and the Persistence Layer

Sessions are stored in a local RAM cache (which is a simpe map) whose size is
defined by the MaxSessionCacheSize variable. If you set this variable to 0,
no sessions are held locally. The SessionCacheExpiry controls when a session
will be purged from the cache based on the last time it was used.

The cache is write-through (except for session last access times). That is,
every time a change was made to a session, that change is forwarded to the
package's persistence layer to be saved. The persistence layer is a collection
of functions which allow the storage and retrieval of objects from a permanent
data store. For example, you may use an SQL database or a key-value store.

See the documentation of PersistenceLayer for details on the functions to be
implemented. If you need to implement only some of the functions, you may use
ExtendablePersistenceLayer instead of creating your own class. The package
default is to do nothing. That is, sessions are not persisted and therefore
will get lost when purged from the local cache or when the application exits.

Session objects implement gob.GobEncoder/gob.GobDecoder and
json.Marshaler/json.Unmarshaler. While encoding to JSON allows you to easily
inspect session attributes in your database, GOB serialization is preferred as
it will restore session objects precisely. (For example, the JSON package always
unmarshals numbers into floats even if they were originally integers.)

It is recommended that you purge your data store from expired sessions from time
to time, e.g. by using a cron job, because users may abandon your website which
will leave old sessions in your store.

It is recommended to call PurgeSessions() before exiting the program. This will
cause session last access times to be updated.

Utility Functions

This package provides a number of utility functions which may be useful in the
context of session and user management.

The CUID() function generates Base-62 "compact unique identifiers" suitable for
user IDs.

The RandomID() function generates random Base-62 strings of any length.

The ReasonablePassword() function checks the strength of a password based on the
recommendations of NIST SP 800-63B.
*/
package sessions
