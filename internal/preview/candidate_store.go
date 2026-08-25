package preview

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

var (
	ErrCandidateInvalid  = errors.New("candidate is invalid")
	ErrCandidateExpired  = errors.New("candidate has expired")
	ErrCandidateCapacity = errors.New("candidate store capacity exceeded")
)

type Binding struct {
	ItemID              string `json:"-"`
	SourceID            string `json:"-"`
	Language            string `json:"-"`
	AuthContext         string `json:"-"`
	AllowlistGeneration uint64 `json:"-"`
}

type CandidateInput struct {
	RawID       string `json:"-"`
	Provider    string
	Name        string
	Language    string
	Format      string
	Comment     string
	IsHashMatch bool
	Score       float64
	Reasons     []string
}

type Candidate struct {
	Token         string
	RawID         string `json:"-"`
	Provider      string
	Name          string
	Language      string
	Format        string
	Comment       string
	IsHashMatch   bool
	Score         float64
	Reasons       []string
	ExpiresAt     time.Time
	State         string
	Attempts      int
	FailureCode   string  `json:"-"`
	ArtifactToken string  `json:"-"`
	Binding       Binding `json:"-"`
}

type CandidateStoreOptions struct {
	TTL           time.Duration
	MaxPerContext int
	MaxTotal      int
	Now           func() time.Time
}

type CandidateStore struct {
	mu            sync.Mutex
	ttl           time.Duration
	maxPerContext int
	maxTotal      int
	now           func() time.Time
	entries       map[[32]byte]*Candidate
}

func NewCandidateStore(options CandidateStoreOptions) *CandidateStore {
	if options.TTL <= 0 {
		options.TTL = 10 * time.Minute
	}
	if options.MaxPerContext <= 0 {
		options.MaxPerContext = 100
	}
	if options.MaxTotal <= 0 {
		options.MaxTotal = 1000
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &CandidateStore{ttl: options.TTL, maxPerContext: options.MaxPerContext, maxTotal: options.MaxTotal, now: options.Now, entries: make(map[[32]byte]*Candidate)}
}

func (s *CandidateStore) IssueMany(binding Binding, inputs []CandidateInput) ([]Candidate, error) {
	if s == nil || len(inputs) == 0 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpiredLocked(s.now())
	if len(inputs) > s.maxTotal-len(s.entries) {
		return nil, ErrCandidateCapacity
	}
	activeForContext := 0
	for _, entry := range s.entries {
		if entry.Binding.AuthContext == binding.AuthContext {
			activeForContext++
		}
	}
	if activeForContext+len(inputs) > s.maxPerContext {
		return nil, ErrCandidateCapacity
	}
	issued := make([]Candidate, 0, len(inputs))
	for _, input := range inputs {
		token, digest, err := newToken()
		if err != nil {
			return nil, ErrCandidateCapacity
		}
		entry := &Candidate{
			Token: token, RawID: input.RawID, Provider: input.Provider, Name: input.Name,
			Language: input.Language, Format: input.Format, Comment: input.Comment,
			IsHashMatch: input.IsHashMatch, Score: input.Score, Reasons: append([]string(nil), input.Reasons...),
			ExpiresAt: s.now().Add(s.ttl), State: "ready", Binding: binding,
		}
		s.entries[digest] = entry
		issued = append(issued, cloneCandidate(*entry))
	}
	return issued, nil
}

func (s *CandidateStore) Resolve(token string, binding Binding) (Candidate, error) {
	if s == nil || token == "" {
		return Candidate{}, ErrCandidateInvalid
	}
	digest := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[digest]
	if !ok {
		return Candidate{}, ErrCandidateInvalid
	}
	if !s.now().Before(entry.ExpiresAt) {
		delete(s.entries, digest)
		return Candidate{}, ErrCandidateExpired
	}
	if !sameBinding(entry.Binding, binding) {
		return Candidate{}, ErrCandidateInvalid
	}
	return cloneCandidate(*entry), nil
}

func (s *CandidateStore) RecordFailure(token, code string, attempts int) {
	if s == nil || token == "" {
		return
	}
	digest := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry := s.entries[digest]; entry != nil {
		entry.State = "failed"
		entry.FailureCode = code
		if attempts > entry.Attempts {
			entry.Attempts = attempts
		} else {
			entry.Attempts++
		}
	}
}

func (s *CandidateStore) RecordSuccess(token, artifactToken string, attempts int) {
	if s == nil || token == "" {
		return
	}
	digest := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry := s.entries[digest]; entry != nil {
		entry.State = "fetched"
		entry.ArtifactToken = artifactToken
		if attempts > entry.Attempts {
			entry.Attempts = attempts
		} else if entry.Attempts == 0 {
			entry.Attempts = 1
		}
	}
}

func (s *CandidateStore) RemoveExpired() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpiredLocked(s.now())
}

func (s *CandidateStore) removeExpiredLocked(now time.Time) {
	for digest, entry := range s.entries {
		if !now.Before(entry.ExpiresAt) {
			delete(s.entries, digest)
		}
	}
}

func sameBinding(a, b Binding) bool {
	return a.ItemID == b.ItemID && a.SourceID == b.SourceID && (b.Language == "" || a.Language == b.Language) && a.AuthContext == b.AuthContext && a.AllowlistGeneration == b.AllowlistGeneration
}

func cloneCandidate(value Candidate) Candidate {
	value.Reasons = append([]string(nil), value.Reasons...)
	return value
}

func newToken() (string, [32]byte, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", [32]byte{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	return token, sha256.Sum256([]byte(token)), nil
}
