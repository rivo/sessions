# A Go Package for Cookie-Based Web Sessions

[![Godoc Reference](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/rivo/sessions)
[![Go Report](https://img.shields.io/badge/go%20report-A%2B-brightgreen.svg)](https://goreportcard.com/report/github.com/rivo/sessions)

This Go package attempts to free you from the hard work of implementing safe cookie-based web sessions.

Sessions implements a number of OWASP recommendations:

- No data storage on the client
- Automatic session expiry
- Session ID regeneration
- Anomaly detection via IP address and user agent analysis

Additional features:

- Session key/value storage
- Log in/out functions for users
- Various identifier generation functions
- Password strength checks (based on NIST recommendations)
- Role-based access control (RBAC) functions (work in progress)
- Lots of configuration options
- Database-agnostic, choose your own backend
- It's not a framework, everything is based on net/http.
- Extensive documentation

If you want to go one step further and have user signup, login, logout, password reset, email/password change implemented for you, check out [github.com/rivo/users](http://github.com/rivo/users).

## Installation

```
go get github.com/rivo/sessions
```

## Simple Example

```go
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
```

(Providing `true` will _always_ return a session.)

With the session object, you can call:

- `RegenerateID` to switch the session ID,
- `Set`, `Get`, `GetAndDelete`, and `Delete` to (un-)assign values to keys,
- `LogIn` and `LogOut` to attach/detach users,
- `GobEncode`, `GobDecode`, `MarshalJSON`, and `UnmarshalJSON` to (un-)serialize sessions,
- `Destroy` to end a session.

## Configuration Options

- `SessionCookie`: Name of the session cookie.
- `NewSessionCookie`: Function for new cookies (used to set cookie parameters).
- `SessionExpiry`: Time to expiry for inactive sessions.
- `SessionIDExpiry`: Maximum session ID lifetime before automatic regeneration.
- `SessionIDGracePeriod`: Extended lifetime for regenerated session IDs.
- `AcceptRemoteIP`: Accepted level of change for IP addresses.
- `AcceptChangingUserAgent`: Whether or not user agent changes are accepted.
- `MaxSessionCacheSize`: Size of local (write-through) session cache.
- `SessionCacheExpiry`: Maximum session lifetime in local cache.

Then there is `Persistence` used to connect to the session store of your choice (defaults to RAM).

## Documentation

See http://godoc.org/github.com/rivo/sessions for the documentation.

See also the [Wiki](https://github.com/rivo/sessions/wiki/) for more examples and explanations.

## Your Feedback

Add your issue here on GitHub. Feel free to get in touch if you have any questions.

## Release Notes

- v0.1 (2017-11-11)
  - First release.
