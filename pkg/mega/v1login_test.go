package mega

import (
	"encoding/base64"
	"testing"
)

// TestV1LoginMaterial checks the MEGA v1 password KDF and string hash against
// reference vectors generated with an independent implementation of
// t3rm1n4l/go-mega's password_key/stringhash (see go-mega utils.go).
func TestV1LoginMaterial(t *testing.T) {
	cases := []struct {
		email    string
		password string
		keyB64   string
		uh       string
	}{
		{"user@example.com", "correct horse battery staple", "V9l474LpnqUKsABF6GFNew", "sSgbS6ks0bE"},
		{"mega83@mail.327835.xyz", "g!2!&JGfjL4gr9", "vjbQTSUWjWv5F6aeobeU7Q", "Bsx_9sQn1jE"},
		{"a@b.co", "p", "wmD4eMVwKwwy4uQ2CErzpQ", "yzOmWD5VJyw"},
	}

	// Emails must be lowercased before hashing, like go-mega's login().
	upperKey, upperUh := V1LoginMaterial("MIXED@Case.COM", "g!2!&JGfjL4gr9")
	lowerKey, lowerUh := V1LoginMaterial("mixed@case.com", "g!2!&JGfjL4gr9")
	if upperKey != lowerKey || upperUh != lowerUh {
		t.Errorf("V1LoginMaterial did not normalize email case: (upper) key=%q uh=%q vs (lower) key=%q uh=%q",
			upperKey, upperUh, lowerKey, lowerUh)
	}

	for _, c := range cases {
		keyB64, uh := V1LoginMaterial(c.email, c.password)
		if keyB64 != c.keyB64 {
			k, _ := base64.RawURLEncoding.DecodeString(keyB64)
			want, _ := base64.RawURLEncoding.DecodeString(c.keyB64)
			t.Errorf("V1LoginMaterial(%q) key = %x, want %x", c.email, k, want)
		}
		if uh != c.uh {
			t.Errorf("V1LoginMaterial(%q) uh = %q, want %q", c.email, uh, c.uh)
		}
	}
}
