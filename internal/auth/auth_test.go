package auth

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func testAuth(t *testing.T, now *time.Time) *Authenticator {
	t.Helper()
	a, err := New("admin", "correct horse battery staple", Options{Now: func() time.Time { return *now }, Random: bytes.NewReader(bytes.Repeat([]byte{7}, 1024)), SessionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestLoginAndSessionExpiry(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	a := testAuth(t, &now)
	if _, err := a.Login("198.51.100.1:1", "admin", "wrong password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v", err)
	}
	token, err := a.Login("198.51.100.1:1", "admin", "correct horse battery staple")
	if err != nil || token == "" || !a.ValidSession(token) {
		t.Fatalf("login token=%q err=%v valid=%v", token, err, a.ValidSession(token))
	}
	if a.ValidSession(token) && len(token) < 40 {
		t.Fatalf("session token is unexpectedly short: %q", token)
	}
	if testAuth(t, &now).ValidSession(token) {
		t.Fatal("session survived authenticator restart")
	}
	now = now.Add(time.Hour)
	if a.ValidSession(token) {
		t.Fatal("session remained valid at fixed expiry")
	}
}

func TestLoginFailureLimitIsBounded(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	a := testAuth(t, &now)
	for i := 0; i < maxFailures; i++ {
		if _, err := a.Login("198.51.100.1:1", "admin", "wrong password"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failure %d error = %v", i, err)
		}
	}
	if _, err := a.Login("198.51.100.1:1", "admin", "correct horse battery staple"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("locked login error = %v", err)
	}
	now = now.Add(lockoutDuration)
	if _, err := a.Login("198.51.100.1:1", "admin", "correct horse battery staple"); err != nil {
		t.Fatalf("login after lockout = %v", err)
	}
}

func TestLoginDoesNotExposeCredentialOrStoreRawSession(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	a := testAuth(t, &now)
	password := "correct horse battery staple"
	token, err := a.Login("198.51.100.1:1", "admin", password)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a.passwordKey, []byte(password)) {
		t.Fatal("password was retained as the verifier")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatal(err)
	}
	var raw [sha256.Size]byte
	copy(raw[:], decoded)
	for stored := range a.sessions {
		if stored == raw {
			t.Fatal("raw session token was stored")
		}
	}
}

func TestNewRejectsControlCharactersAndOversizedCredentials(t *testing.T) {
	cases := []struct {
		name     string
		username string
		password string
	}{
		{name: "username control", username: "admin\x00", password: "correct horse battery staple"},
		{name: "password control", username: "admin", password: "correct horse\nbattery staple"},
		{name: "password too short", username: "admin", password: "12345"},
		{name: "password oversized", username: "admin", password: string(bytes.Repeat([]byte{'x'}, 257))},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := New(testCase.username, testCase.password, Options{}); err == nil {
				t.Fatal("New accepted an unsafe administrator credential")
			}
		})
	}
}
