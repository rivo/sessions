package sessions

import (
	"math"
	"net/http"
	"time"
)

var (
	// Persistence provides the methods which read/write information from/to an
	// external (permanent) data store.
	Persistence PersistenceLayer = ExtendablePersistenceLayer{}

	// SessionExpiry is the maximum time which may pass before a session that
	// has not been accessed will be destroyed, hence logging a user out.
	SessionExpiry time.Duration = math.MaxInt64

	// SessionIDExpiry is the maximum duration a session ID can be used before it
	// is changed to a new session ID. This helps prevent session hijacking. It
	// may be set to 0, leading to a session ID change with every request.
	// However, this will increase the load on the session persistence layer
	// considerably.
	//
	// Note that expired session IDs will remain active for the duration of
	// SessionIDGracePeriod (leading to session ID overlaps) to avoid race
	// conditions when multiple requests are issued at nearly the same time.
	SessionIDExpiry = time.Hour

	// SessionIDGracePeriod is the duration for a replaced (old) session ID to
	// remain active so multiple concurrent requests from the browser don't
	// accidentally lead to session loss. While the default of five minutes may
	// appear long, in a mobile context or other slow networks, it is a reasonable
	// time.
	SessionIDGracePeriod = 5 * time.Minute

	// AcceptRemoteIP determines how much change of an IPv4 remote IP address is
	// accepted before destroying a session. If set to 4, the last (4th) byte of
	// the client's IP address may change but if the 3rd byte changes compared to
	// the last request, the session is destroyed. And so on. A value of 1 means
	// that any changes in the client's IP address are accepted.
	//
	// When dealing with very sensitive data, it is suggested to set this value
	// to 4 so that when the user connects from a different network, they will be
	// required to log in again. Session hijacking becomes much more difficult
	// that way.
	//
	// IPv6 address or ports, while stored, are currently disregarded.
	//
	// Note that this does not work if your server runs behind a proxy.
	AcceptRemoteIP = 1

	// AcceptChangingUserAgent determines if the remote browser's user agent is
	// checked for consistency. We assume that the user agent for the current
	// session will always remain the same. If it changes, the session is
	// destroyed.
	//
	// By setting this value to "true", sessions will be kept alive even if the
	// user agent string changes.
	AcceptChangingUserAgent = false

	// SessionCookie is the name of the session cookie that will contain the
	// session ID.
	SessionCookie = "id"

	// NewSessionCookie is used to create new session cookies or to renew them.
	// The "Name" and "Value" fields need not be set. It is recommended that you
	// overwrite the default implementation with your specific defaults,
	// especially the "Domain", "Path", and "Secure" fields. Be sure to set
	// "Secure" to true when using TLS (HTTPS). For more information on cookies,
	// refer to:
	//
	//     - https://tools.ietf.org/html/rfc6265
	//     - https://en.wikipedia.org/wiki/HTTP_cookie#Cookie_attributes
	NewSessionCookie = func() *http.Cookie {
		return &http.Cookie{ // Default lifetime is 10 years (i.e. forever).
			Expires:  time.Now().Add(10 * 365 * 24 * time.Hour), // For IE, other browsers will use MaxAge.
			MaxAge:   10 * 365 * 24 * 60 * 60,
			HttpOnly: true,

			// Uncomment and edit the following fields for production use:
			//Domain: "www.example.com",
			//Path:   "/",
			//Secure: true,
		}
	}

	// MaxSessionCacheSize is the maximum size of the local sessions cache. If
	// this value is 0, nothing is cached. If this value is negative, the cache
	// may expand indefinitely. When the maximum size is reached, sessions with
	// the oldest access time are discarded. They are also removed from the cache
	// when their age exceeds SessionCacheExpiry. (This is checked whenever the
	// cache is accessed.)
	//
	// Set this value to 0 if you want to rely on a different cache library. Then
	// connect it via the persistence layer.
	MaxSessionCacheSize = 1024 * 1024

	// SessionCacheExpiry is the maximum duration an inactive session will remain
	// in the local cache.
	SessionCacheExpiry = time.Hour
)
