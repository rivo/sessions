package sessions

import "testing"

// Test password integrity check.
func TestReasonablePassword(t *testing.T) {
	for password, expected := range map[string]int{
		"hflIhf.lKK$982ß":    PasswordOK,
		"abc":                PasswordTooShort,
		"Example.com":        PasswordIsAName,
		"Mail@Example.Com":   PasswordIsAName,
		"football":           PasswordWasCompromised,
		"aardvarks":          PasswordFoundInDictionary,
		"üüüüüüüü":           PasswordRepetitive,
		"defghijklmnopqrstu": PasswordSequential,
	} {
		computed := ReasonablePassword(password, []string{"example.com", "example", "mail@example.com"})
		if expected != computed {
			t.Errorf("Password %s resulted in %d, expected %d", password, computed, expected)
		}
	}
}
