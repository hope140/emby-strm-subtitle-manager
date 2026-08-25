// Package auth implements the small, single-instance administrator session
// boundary. It deliberately does not provide users, roles, password reset, or
// a persistent account database; deployment-owned Secret files are the source
// of the administrator credential.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	passwordSaltBytes  = 16
	passwordKeyBytes   = 32
	passwordIterations = 120_000
	sessionTokenBytes  = 32
	defaultSessionTTL  = 8 * time.Hour
	maxSessionTTL      = 24 * time.Hour
	maxSessions        = 1024
	maxFailureEntries  = 1024
	maxFailures        = 5
	failureWindow      = 5 * time.Minute
	lockoutDuration    = time.Minute
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrRateLimited        = errors.New("login rate limited")
	ErrSessionUnavailable = errors.New("session store unavailable")
)

// Options controls bounded, in-memory session and login-failure state. The
// random and clock hooks are intentionally injectable for deterministic tests.
type Options struct {
	SessionTTL time.Duration
	Now        func() time.Time
	Random     io.Reader
}

type failureState struct {
	started      time.Time
	count        int
	blockedUntil time.Time
}

// Authenticator verifies one deployment-owned administrator credential and
// stores only salted password-derived material and hashed session tokens.
type Authenticator struct {
	username     string
	passwordSalt []byte
	passwordKey  []byte
	sessionTTL   time.Duration
	now          func() time.Time
	random       io.Reader

	mu       sync.Mutex
	sessions map[[sha256.Size]byte]time.Time
	failures map[string]failureState
}

// New validates the deployment credential and creates an in-memory session
// verifier. The supplied password is not retained after the derived key is
// calculated.
func New(username, password string, options Options) (*Authenticator, error) {
	if strings.TrimSpace(username) == "" || username != strings.TrimSpace(username) || len([]byte(username)) > 64 || strings.IndexFunc(username, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return nil, errors.New("invalid administrator username")
	}
	if password == "" || len([]byte(password)) < 12 || len([]byte(password)) > 256 || strings.IndexFunc(password, unicode.IsControl) >= 0 {
		return nil, errors.New("invalid administrator password")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Random == nil {
		options.Random = rand.Reader
	}
	if options.SessionTTL == 0 {
		options.SessionTTL = defaultSessionTTL
	}
	if options.SessionTTL < time.Minute || options.SessionTTL > maxSessionTTL {
		return nil, errors.New("invalid administrator session TTL")
	}
	salt := make([]byte, passwordSaltBytes)
	if _, err := io.ReadFull(options.Random, salt); err != nil {
		return nil, errors.New("unable to initialize administrator credential")
	}
	key := pbkdf2SHA256([]byte(password), salt, passwordIterations, passwordKeyBytes)
	return &Authenticator{
		username: username, passwordSalt: salt, passwordKey: key,
		sessionTTL: options.SessionTTL, now: options.Now, random: options.Random,
		sessions: make(map[[sha256.Size]byte]time.Time), failures: make(map[string]failureState),
	}, nil
}

// Login verifies the credential and returns a one-time opaque session token.
// clientKey should be a server-derived remote address, not an untrusted
// forwarded header. Failure state is bounded and never logged.
func (a *Authenticator) Login(clientKey, username, password string) (string, error) {
	if a == nil {
		return "", ErrInvalidCredentials
	}
	now := a.now()
	clientKey = normalizeClientKey(clientKey)
	a.mu.Lock()
	a.pruneLocked(now)
	state := a.failures[clientKey]
	if now.Before(state.blockedUntil) {
		a.mu.Unlock()
		return "", ErrRateLimited
	}
	a.mu.Unlock()

	if len([]byte(password)) > 256 {
		a.recordFailure(clientKey, now)
		return "", ErrInvalidCredentials
	}
	validUser := hmac.Equal([]byte(username), []byte(a.username))
	derived := pbkdf2SHA256([]byte(password), a.passwordSalt, passwordIterations, passwordKeyBytes)
	validPassword := hmac.Equal(derived, a.passwordKey)
	if !validUser || !validPassword {
		a.recordFailure(clientKey, now)
		return "", ErrInvalidCredentials
	}

	tokenBytes := make([]byte, sessionTokenBytes)
	if _, err := io.ReadFull(a.random, tokenBytes); err != nil {
		return "", ErrSessionUnavailable
	}
	var tokenHash [sha256.Size]byte
	tokenHash = sha256.Sum256(tokenBytes)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked(now)
	if len(a.sessions) >= maxSessions {
		return "", ErrSessionUnavailable
	}
	delete(a.failures, clientKey)
	a.sessions[tokenHash] = now.Add(a.sessionTTL)
	return base64.RawURLEncoding.EncodeToString(tokenBytes), nil
}

// ValidSession checks a token without extending its fixed expiry.
func (a *Authenticator) ValidSession(token string) bool {
	if a == nil || token == "" {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != sessionTokenBytes {
		return false
	}
	hash := sha256.Sum256(decoded)
	now := a.now()
	a.mu.Lock()
	defer a.mu.Unlock()
	expires, ok := a.sessions[hash]
	if !ok {
		return false
	}
	if !now.Before(expires) {
		delete(a.sessions, hash)
		return false
	}
	return true
}

// SessionTTL reports the fixed lifetime used when issuing browser sessions.
func (a *Authenticator) SessionTTL() time.Duration {
	if a == nil {
		return 0
	}
	return a.sessionTTL
}

func (a *Authenticator) recordFailure(clientKey string, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked(now)
	if _, ok := a.failures[clientKey]; !ok && len(a.failures) >= maxFailureEntries {
		clientKey = "__overflow__"
	}
	state := a.failures[clientKey]
	if state.started.IsZero() || !now.Before(state.started.Add(failureWindow)) {
		state = failureState{started: now}
	}
	state.count++
	if state.count >= maxFailures {
		state.blockedUntil = now.Add(lockoutDuration)
	}
	a.failures[clientKey] = state
}

func (a *Authenticator) pruneLocked(now time.Time) {
	for hash, expires := range a.sessions {
		if !now.Before(expires) {
			delete(a.sessions, hash)
		}
	}
	for key, state := range a.failures {
		if (!state.blockedUntil.IsZero() && !now.Before(state.blockedUntil)) || (!state.started.IsZero() && !now.Before(state.started.Add(failureWindow))) {
			delete(a.failures, key)
		}
	}
}

func normalizeClientKey(value string) string {
	if value == "" || len(value) > 128 {
		return "unknown"
	}
	return value
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLength int) []byte {
	result := make([]byte, 0, keyLength)
	for block := uint32(1); len(result) < keyLength; block++ {
		mac := hmac.New(sha256.New, password)
		_, _ = mac.Write(salt)
		var counter [4]byte
		binary.BigEndian.PutUint32(counter[:], block)
		_, _ = mac.Write(counter[:])
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha256.New, password)
			_, _ = mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		result = append(result, t...)
	}
	return result[:keyLength]
}
