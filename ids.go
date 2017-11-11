package sessions

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Milliseconds of 2017-01-01 since 1970-01-01.
const referenceDate = 1483228800000

var (
	macAddress  [6]byte    // The unique MAC address of the computer running this program.
	lastMutex   sync.Mutex // The mutex which syncs access to the timestamp and counter.
	lastTime    uint64     // The timestamp of the last CCUID.
	lastCounter uint64     // The counter of the last CCUID.
)

// Initialize variables needed for the CUID.
func initCUID() {
	// Get a unique MAC address.
	interfaces, err := net.Interfaces()
	if err != nil {
		panic(err)
	}
	for _, iface := range interfaces {
		if len(iface.HardwareAddr) >= 6 {
			copy(macAddress[:], iface.HardwareAddr)
			break
		}
	}
}

// CUID returns a compact unique identifier suitable for user IDs. The goal is
// to minimize collisions while keeping the identifier short. The returned
// identifiers are exactly 11 bytes long, consisting of letters and numbers
// (Base62). They are generated from a 64-bit value with the following fields:
//
//     - Bit 64-25: A timestamp. The number of milliseconds since Jan 1, 2017,
//       ommitting all bits above bit 40. Timestamps start over about every 34
//       years. Thus, within these time periods, user IDs should be sortable
//       lexicographically.
//     - Bit 24-9: A 16-bit hash of this computer's MAC address.
//     - Bit 8-1: A counter which increases with every consecutive call to this
//       function which results in the same timestamp. Bits 8 and above, if any,
//       will spill into the MAC address's hash.
//
// To generate IDs for non-user data, you may refer to other libraries such as
// https://github.com/segmentio/ksuid.
func CUID() string {
	lastMutex.Lock()
	defer lastMutex.Unlock()

	// Initialize the bits with the timestamp.
	now := time.Now()
	timestamp := uint64(now.Unix())*1000 - referenceDate + uint64(now.Nanosecond())/1000000
	timestamp &= (1 << 40) - 1

	// Counter.
	if timestamp == lastTime {
		lastCounter++
	} else {
		lastCounter = 0
	}
	lastTime = timestamp
	counter := uint64(lastCounter & 0xff)

	// MAC address.
	var macHash uint16
	for _, b := range macAddress {
		macHash = (macHash << 5) - macHash // *= 31 (a prime).
		macHash += uint16(b)
	}
	spill := lastCounter >> 8
	if spill != 0 {
		macHash += uint16(spill & 0xffff)
	}
	mac := uint64(macHash)

	// Assemble.
	bits := (timestamp << 24) | (mac << 8) | counter

	// Transform to Base62.
	chars := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	base := uint64(len(chars))
	var base64 string
	for len := 0; len < 11; len++ {
		base64 = string(chars[bits%base]) + base64
		bits /= base
	}

	return base64
}

// RandomID returns a random Base62-encoded string with the given length. To
// avoid collisions, use a length of at least 22 (which corresponds to a minimum
// of 128 bits).
func RandomID(length int) (string, error) {
	id := make([]byte, length)
	chars := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	var b [1]byte
	for length > 0 {
		n, err := rand.Reader.Read(b[:])
		if err != nil {
			return "", err
		}
		if n < 1 {
			return "", errors.New("Unable to generate random number")
		}
		length--
		id[length] = chars[int(b[0])%len(chars)]
	}
	return string(id), nil
}

// generateSesssionID generates a random 128-bit, Base64-encoded session ID.
// Collision probability is close to zero. The resulting string is 24 characters
// long.
func generateSesssionID() (string, error) {
	// For more on collisions:
	// https://en.wikipedia.org/wiki/Birthday_problem
	// http://www.wolframalpha.com/input/?i=1-e%5E(-1000000000*(1000000000-1)%2F(2*2%5E128))
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("Could not generate session ID: %s", err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
