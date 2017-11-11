package sessions

// sessionIDMutexes provides locking on the level of session IDs.
var sessionIDMutexes *mutexes

// Initialize package.
func init() {
	sessionIDMutexes = newMutexes()
	initCUID()
	initCache()
	initPasswords()
}
