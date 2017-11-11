package sessions

import (
	"regexp"
	"testing"
)

// Test generation of CUIDs and collisions.
func TestCUID(t *testing.T) {
	// Generate a whole bunch of CUIDs and test for collisions.
	set := make(map[string]struct{})
	count := 65536
	for i := 0; i < count; i++ {
		cuid := CUID()
		if len(cuid) != 11 {
			t.Errorf("Invalid CUID length: %d", len(cuid))
			return
		}
		set[cuid] = struct{}{}
	}
	if len(set) != count {
		t.Errorf("Found %d CUID collisions", count-len(set))
	}
}

// Test generation of random IDs.
func TestRandomID(t *testing.T) {
	id, err := RandomID(22)
	if err != nil {
		t.Error(err)
		return
	}
	if !regexp.MustCompile("^[0-9A-Za-z]{22}$").MatchString(id) {
		t.Errorf("Generated ID does not have expected format or length: %s (length = %d)", id, len(id))
		return
	}
	t.Logf("Generated ID: %s", id)
}
