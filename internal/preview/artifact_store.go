package preview

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hope140/subbridge/internal/pathsecurity"
	"github.com/hope140/subbridge/internal/subtitle"
)

var (
	ErrArtifactInvalid     = errors.New("artifact is invalid")
	ErrArtifactExpired     = errors.New("artifact has expired")
	ErrArtifactCapacity    = errors.New("artifact store capacity exceeded")
	ErrArtifactTooLarge    = errors.New("artifact is too large")
	ErrArtifactUnavailable = errors.New("artifact store is unavailable")
)

type Artifact struct {
	Token       string
	Format      string
	Language    string
	ByteLength  int
	CueCount    int
	ContentHash string
	ExpiresAt   time.Time
	Cues        []subtitle.Cue
	Binding     Binding `json:"-"`
}

type ArtifactStoreOptions struct {
	Directory     string
	TTL           time.Duration
	MaxTotal      int
	MaxPerContext int
	MaxBytes      int64
	Now           func() time.Time
}

type artifactEntry struct {
	Artifact
	Filename string
}

type ArtifactStore struct {
	mu            sync.Mutex
	directory     string
	ttl           time.Duration
	maxTotal      int
	maxPerContext int
	maxBytes      int64
	now           func() time.Time
	entries       map[[32]byte]*artifactEntry
}

func NewArtifactStore(options ArtifactStoreOptions) (*ArtifactStore, error) {
	return newArtifactStore(options, os.Chmod)
}

func newArtifactStore(options ArtifactStoreOptions, chmod func(string, fs.FileMode) error) (*ArtifactStore, error) {
	directoryInput := strings.TrimSpace(options.Directory)
	if directoryInput == "" || !filepath.IsAbs(directoryInput) {
		return nil, ErrArtifactUnavailable
	}
	if options.TTL <= 0 {
		options.TTL = 20 * time.Minute
	}
	if options.MaxTotal <= 0 {
		options.MaxTotal = 256
	}
	if options.MaxPerContext <= 0 {
		options.MaxPerContext = 64
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = 4 << 20
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	directory, err := filepath.Abs(directoryInput)
	if err != nil {
		return nil, ErrArtifactUnavailable
	}
	if pathsecurity.IsFilesystemRoot(directory) {
		return nil, ErrArtifactUnavailable
	}
	if usesSymlink, inspectErr := pathsecurity.UsesSymlink(directory); inspectErr != nil || usesSymlink {
		return nil, ErrArtifactUnavailable
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, ErrArtifactUnavailable
	}
	if usesSymlink, inspectErr := pathsecurity.UsesSymlink(directory); inspectErr != nil || usesSymlink {
		return nil, ErrArtifactUnavailable
	}
	// Chmod is the private-directory gate. Any error, including a permission
	// error, means that the required private mode cannot be confirmed.
	if chmod == nil {
		return nil, ErrArtifactUnavailable
	}
	if err := chmod(directory, 0o700); err != nil {
		return nil, ErrArtifactUnavailable
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, ErrArtifactUnavailable
	}
	for _, entry := range entries {
		if entry.IsDir() || (filepath.Ext(entry.Name()) != ".subtitle" && !strings.HasSuffix(entry.Name(), ".subtitle.tmp")) {
			continue
		}
		if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, ErrArtifactUnavailable
		}
	}
	return &ArtifactStore{directory: directory, ttl: options.TTL, maxTotal: options.MaxTotal, maxPerContext: options.MaxPerContext, maxBytes: options.MaxBytes, now: options.Now, entries: make(map[[32]byte]*artifactEntry)}, nil
}

func (s *ArtifactStore) Create(binding Binding, format, language string, content []byte, cues []subtitle.Cue) (Artifact, error) {
	if s == nil {
		return Artifact{}, ErrArtifactUnavailable
	}
	if strings.TrimSpace(binding.Language) == "" {
		return Artifact{}, ErrArtifactUnavailable
	}
	// Candidate.Language is display-only. Artifact language is always derived
	// from the canonical language in the binding.
	language = binding.Language
	if int64(len(content)) > s.maxBytes {
		return Artifact{}, ErrArtifactTooLarge
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpiredLocked(s.now())
	if len(s.entries) >= s.maxTotal {
		return Artifact{}, ErrArtifactCapacity
	}
	activeForContext := 0
	for _, entry := range s.entries {
		if entry.Binding.AuthContext == binding.AuthContext {
			activeForContext++
		}
	}
	if activeForContext >= s.maxPerContext {
		return Artifact{}, ErrArtifactCapacity
	}
	token, digest, err := newToken()
	if err != nil {
		return Artifact{}, ErrArtifactUnavailable
	}
	hash := sha256.Sum256(content)
	artifact := Artifact{Token: token, Format: format, Language: language, ByteLength: len(content), CueCount: len(cues), ContentHash: hex.EncodeToString(hash[:]), ExpiresAt: s.now().Add(s.ttl), Cues: cloneCues(cues), Binding: binding}
	filename := filepath.Join(s.directory, hex.EncodeToString(digest[:])+".subtitle")
	temporary := filename + ".tmp"
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return Artifact{}, ErrArtifactUnavailable
	}
	writeOK := false
	defer func() {
		_ = file.Close()
		if !writeOK {
			_ = os.Remove(temporary)
		}
	}()
	if _, err := file.Write(content); err != nil {
		return Artifact{}, ErrArtifactUnavailable
	}
	if err := file.Sync(); err != nil {
		return Artifact{}, ErrArtifactUnavailable
	}
	if err := file.Close(); err != nil {
		return Artifact{}, ErrArtifactUnavailable
	}
	if err := os.Rename(temporary, filename); err != nil {
		return Artifact{}, ErrArtifactUnavailable
	}
	writeOK = true
	s.entries[digest] = &artifactEntry{Artifact: artifact, Filename: filename}
	return cloneArtifact(artifact), nil
}

func (s *ArtifactStore) Get(token string, binding Binding) (Artifact, error) {
	artifact, _, err := s.GetContent(token, binding)
	return artifact, err
}

// GetContent returns a validated artifact and its canonical bytes for the
// explicitly gated D3 Add flow. The binding and integrity checks are exactly
// the same as Get; callers must not use it as an arbitrary file reader.
func (s *ArtifactStore) GetContent(token string, binding Binding) (Artifact, []byte, error) {
	if s == nil || token == "" {
		return Artifact{}, nil, ErrArtifactInvalid
	}
	digest := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[digest]
	if !ok {
		return Artifact{}, nil, ErrArtifactInvalid
	}
	if !s.now().Before(entry.ExpiresAt) {
		delete(s.entries, digest)
		_ = os.Remove(entry.Filename)
		return Artifact{}, nil, ErrArtifactExpired
	}
	if !sameBinding(entry.Binding, binding) {
		return Artifact{}, nil, ErrArtifactInvalid
	}
	file, err := os.Open(entry.Filename)
	if err != nil {
		return Artifact{}, nil, ErrArtifactUnavailable
	}
	content, readErr := io.ReadAll(io.LimitReader(file, s.maxBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || int64(len(content)) > s.maxBytes {
		return Artifact{}, nil, ErrArtifactUnavailable
	}
	hash := sha256.Sum256(content)
	if len(content) != entry.ByteLength || hex.EncodeToString(hash[:]) != entry.ContentHash {
		return Artifact{}, nil, ErrArtifactUnavailable
	}
	return cloneArtifact(entry.Artifact), append([]byte(nil), content...), nil
}

func (s *ArtifactStore) RemoveExpired() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpiredLocked(s.now())
}

func (s *ArtifactStore) removeExpiredLocked(now time.Time) {
	for digest, entry := range s.entries {
		if !now.Before(entry.ExpiresAt) {
			delete(s.entries, digest)
			_ = os.Remove(entry.Filename)
		}
	}
}

func cloneArtifact(value Artifact) Artifact {
	value.Cues = cloneCues(value.Cues)
	return value
}

func cloneCues(values []subtitle.Cue) []subtitle.Cue {
	return append([]subtitle.Cue(nil), values...)
}

// TokenDigest is useful for tests and internal HMAC-style correlation without
// exposing the token itself. It is not used as a public identifier.
func TokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}
