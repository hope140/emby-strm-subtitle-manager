// Package config loads and validates the service configuration.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/hope140/subbridge/internal/pathsecurity"
	"gopkg.in/yaml.v3"
)

const (
	defaultTimeoutSeconds       = 30
	maxTimeoutSeconds           = 300
	maxAllowlistBytes           = 1 << 20
	maxAllowlistEntries         = 1000
	AdminUsernameEnv            = "APP_ADMIN_USERNAME"
	AdminPasswordEnv            = "APP_ADMIN_PASSWORD"
	APIAuthScopeMediaRead       = "media:read"
	APIAuthScopeSubtitleSearch  = "subtitle:search"
	APIAuthScopeSubtitlePreview = "subtitle:preview"
	APIAuthScopeSubtitleWrite   = "subtitle:write"
)

var defaultAPIAuthScopes = []string{
	APIAuthScopeMediaRead,
	APIAuthScopeSubtitleSearch,
	APIAuthScopeSubtitlePreview,
}

// Config is the complete non-secret runtime configuration.
type Config struct {
	Server       ServerConfig   `yaml:"server"`
	Emby         EmbyConfig     `yaml:"emby"`
	Security     SecurityConfig `yaml:"security"`
	Features     FeatureConfig  `yaml:"features"`
	D2           D2Config       `yaml:"d2"`
	D3           D3Config       `yaml:"d3"`
	PathMappings []PathMapping  `yaml:"path_mappings"`
}

type ServerConfig struct {
	ListenAddress string `yaml:"listen_address"`
}

type EmbyConfig struct {
	URL            string `yaml:"url"`
	APIKeyFile     string `yaml:"api_key_file"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

type SecurityConfig struct {
	IdentityKeyFile     string   `yaml:"identity_key_file"`
	APIAuthTokenFile    string   `yaml:"api_auth_token_file"`
	APIAuthScopes       []string `yaml:"api_auth_scopes"`
	SessionCookieSecure bool     `yaml:"session_cookie_secure"`
	SessionTTLSeconds   int      `yaml:"session_ttl_seconds"`
}

type FeatureConfig struct {
	WriteEnabled        bool `yaml:"write_enabled" json:"write_enabled"`
	RemoteSearchEnabled bool `yaml:"remote_search_enabled" json:"remote_search_enabled"`
}

// D2Config contains bounded search/preview settings. Zero values are filled
// with safe defaults by WithDefaults; the feature itself remains disabled
// unless features.remote_search_enabled and the runtime installs an explicit
// daily or Canary Item gate.
type D2Config struct {
	CacheDir              string         `yaml:"cache_dir"`
	DefaultLanguage       string         `yaml:"default_language"`
	CandidateTTLSeconds   int            `yaml:"candidate_ttl_seconds"`
	ArtifactTTLSeconds    int            `yaml:"artifact_ttl_seconds"`
	SearchTimeoutSeconds  int            `yaml:"search_timeout_seconds"`
	FetchTimeoutSeconds   int            `yaml:"fetch_timeout_seconds"`
	PreviewTimeoutSeconds int            `yaml:"preview_timeout_seconds"`
	MaxSubtitleBytes      int64          `yaml:"max_subtitle_bytes"`
	MaxCandidateCount     int            `yaml:"max_candidate_count"`
	MaxConcurrent         int            `yaml:"max_concurrent"`
	Canary                D2CanaryConfig `yaml:"canary"`
}

type D2CanaryConfig struct {
	Enabled           bool   `yaml:"enabled"`
	ItemAllowlistFile string `yaml:"item_allowlist_file"`
}

// D3Config contains the deliberately narrow first write capability. Zero
// values keep D3 disabled and preserve the D1/D2 read-only deployment.
type D3Config struct {
	HistoryDir            string         `yaml:"history_dir"`
	QuarantineDir         string         `yaml:"quarantine_dir"`
	ArchiveDir            string         `yaml:"archive_dir"`
	TrashDir              string         `yaml:"trash_dir"`
	RefreshTimeoutSeconds int            `yaml:"refresh_timeout_seconds"`
	Canary                D3CanaryConfig `yaml:"canary"`
}

type D3CanaryConfig struct {
	Enabled           bool   `yaml:"enabled"`
	ItemAllowlistFile string `yaml:"item_allowlist_file"`
}

// WithDefaults returns the bounded D2 configuration used by the runtime.
func (c D2Config) WithDefaults() D2Config {
	if c.DefaultLanguage == "" {
		c.DefaultLanguage = "zh-CN"
	}
	if c.CandidateTTLSeconds == 0 {
		c.CandidateTTLSeconds = 600
	}
	if c.ArtifactTTLSeconds == 0 {
		c.ArtifactTTLSeconds = 1200
	}
	if c.SearchTimeoutSeconds == 0 {
		c.SearchTimeoutSeconds = 20
	}
	if c.FetchTimeoutSeconds == 0 {
		c.FetchTimeoutSeconds = 25
	}
	if c.PreviewTimeoutSeconds == 0 {
		c.PreviewTimeoutSeconds = 5
	}
	if c.MaxSubtitleBytes == 0 {
		c.MaxSubtitleBytes = 4 << 20
	}
	if c.MaxCandidateCount == 0 {
		c.MaxCandidateCount = 20
	}
	if c.MaxConcurrent == 0 {
		c.MaxConcurrent = 4
	}
	return c
}

func (c D3Config) WithDefaults() D3Config {
	if c.RefreshTimeoutSeconds == 0 {
		c.RefreshTimeoutSeconds = 10
	}
	return c
}

type PathMapping struct {
	Emby  string `yaml:"emby"`
	Local string `yaml:"local"`
}

// LoadFile decodes one YAML document and validates every supported field.
// Unknown fields are rejected to prevent a misspelled security setting from
// silently falling back to a default.
func LoadFile(filename string) (Config, error) {
	var cfg Config
	f, err := os.Open(filename)
	if err != nil {
		return cfg, errors.New("unable to open configuration file")
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("invalid configuration: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return cfg, errors.New("invalid configuration: multiple YAML documents are not supported")
		}
		return cfg, fmt.Errorf("invalid configuration: %w", err)
	}
	if cfg.Emby.TimeoutSeconds == 0 {
		cfg.Emby.TimeoutSeconds = defaultTimeoutSeconds
	}
	cfg.D2 = cfg.D2.WithDefaults()
	cfg.D3 = cfg.D3.WithDefaults()
	cfg.Security.APIAuthScopes = cfg.Security.EffectiveAPIAuthScopes()
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate checks the safe D1 and gated D2 configuration boundaries.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Server.ListenAddress) == "" {
		return errors.New("server.listen_address is required")
	}
	if strings.TrimSpace(c.Emby.URL) == "" {
		return errors.New("emby.url is required")
	}
	u, err := url.Parse(c.Emby.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("emby.url must be an http or https URL without credentials, query, or fragment")
	}
	if strings.TrimSpace(c.Emby.APIKeyFile) == "" {
		return errors.New("emby.api_key_file is required")
	}
	if strings.TrimSpace(c.Security.IdentityKeyFile) == "" {
		return errors.New("security.identity_key_file is required")
	}
	if !isAbsolutePath(c.Security.IdentityKeyFile) {
		return errors.New("security.identity_key_file must be an absolute path")
	}
	if strings.TrimSpace(c.Security.APIAuthTokenFile) == "" {
		return errors.New("security.api_auth_token_file is required")
	}
	if !isAbsolutePath(c.Security.APIAuthTokenFile) {
		return errors.New("security.api_auth_token_file must be an absolute path")
	}
	seenScopes := make(map[string]struct{})
	for _, scope := range c.Security.EffectiveAPIAuthScopes() {
		if scope == "" {
			return errors.New("security.api_auth_scopes cannot contain an empty scope")
		}
		if _, exists := seenScopes[scope]; exists {
			return fmt.Errorf("security.api_auth_scopes contains duplicate scope %q", scope)
		}
		seenScopes[scope] = struct{}{}
		switch scope {
		case APIAuthScopeMediaRead, APIAuthScopeSubtitleSearch, APIAuthScopeSubtitlePreview:
			// Read-only scopes are always available.
		case APIAuthScopeSubtitleWrite:
			if !c.Features.WriteEnabled {
				return errors.New("security.api_auth_scopes cannot include subtitle:write while D3 writes are disabled")
			}
		default:
			return fmt.Errorf("security.api_auth_scopes contains unsupported scope %q", scope)
		}
	}
	if c.Security.SessionTTLSeconds == 0 {
		c.Security.SessionTTLSeconds = 8 * 60 * 60
	}
	if c.Security.SessionTTLSeconds < 60 || c.Security.SessionTTLSeconds > 24*60*60 {
		return errors.New("security.session_ttl_seconds must be between 60 and 86400")
	}
	if c.Emby.TimeoutSeconds < 1 || c.Emby.TimeoutSeconds > maxTimeoutSeconds {
		return fmt.Errorf("emby.timeout_seconds must be between 1 and %d", maxTimeoutSeconds)
	}
	d3 := c.D3.WithDefaults()
	if c.Features.WriteEnabled && !c.Features.RemoteSearchEnabled {
		return errors.New("features.write_enabled requires remote_search_enabled for the validated D2 artifact")
	}
	d2 := c.D2.WithDefaults()
	if !validD2Language(d2.DefaultLanguage) {
		return errors.New("d2.default_language must be a supported Chinese language")
	}
	if c.Features.RemoteSearchEnabled && strings.TrimSpace(d2.CacheDir) == "" {
		return errors.New("d2.cache_dir is required when features.remote_search_enabled is true")
	}
	if d2.CacheDir != "" && !isAbsolutePath(d2.CacheDir) {
		return errors.New("d2.cache_dir must be an absolute path")
	}
	if d2.CandidateTTLSeconds < 1 || d2.CandidateTTLSeconds > 3600 {
		return errors.New("d2.candidate_ttl_seconds must be between 1 and 3600")
	}
	if d2.ArtifactTTLSeconds < 1 || d2.ArtifactTTLSeconds > 7200 {
		return errors.New("d2.artifact_ttl_seconds must be between 1 and 7200")
	}
	if d2.SearchTimeoutSeconds < 1 || d2.SearchTimeoutSeconds > 20 {
		return errors.New("d2.search_timeout_seconds must be between 1 and 20")
	}
	if d2.FetchTimeoutSeconds < 1 || d2.FetchTimeoutSeconds > 25 {
		return errors.New("d2.fetch_timeout_seconds must be between 1 and 25")
	}
	if d2.PreviewTimeoutSeconds < 1 || d2.PreviewTimeoutSeconds > 5 {
		return errors.New("d2.preview_timeout_seconds must be between 1 and 5")
	}
	if d2.MaxSubtitleBytes < 1 || d2.MaxSubtitleBytes > 4<<20 {
		return errors.New("d2.max_subtitle_bytes must be between 1 and 4194304")
	}
	if d2.MaxCandidateCount < 1 || d2.MaxCandidateCount > 20 {
		return errors.New("d2.max_candidate_count must be between 1 and 20")
	}
	if d2.MaxConcurrent < 1 || d2.MaxConcurrent > 4 {
		return errors.New("d2.max_concurrent must be between 1 and 4")
	}
	if d2.Canary.Enabled && strings.TrimSpace(d2.Canary.ItemAllowlistFile) == "" {
		return errors.New("d2.canary.item_allowlist_file is required when Canary is enabled")
	}
	if strings.TrimSpace(d2.Canary.ItemAllowlistFile) != "" && !isAbsolutePath(d2.Canary.ItemAllowlistFile) {
		return errors.New("d2.canary.item_allowlist_file must be an absolute path")
	}
	if len(c.PathMappings) == 0 {
		return errors.New("path_mappings requires at least one mapping")
	}
	for i, mapping := range c.PathMappings {
		if strings.TrimSpace(mapping.Emby) == "" || strings.TrimSpace(mapping.Local) == "" {
			return fmt.Errorf("path_mappings[%d] requires emby and local", i)
		}
		if !isAbsolutePath(mapping.Emby) || !isAbsolutePath(mapping.Local) {
			return fmt.Errorf("path_mappings[%d] emby and local must be absolute paths", i)
		}
	}
	if d2.CacheDir != "" {
		if pathsecurity.IsFilesystemRoot(d2.CacheDir) {
			return errors.New("d2.cache_dir must be a dedicated non-root directory")
		}
		if usesSymlink, inspectErr := pathsecurity.UsesSymlink(d2.CacheDir); inspectErr != nil {
			return errors.New("d2.cache_dir path cannot be safely inspected")
		} else if usesSymlink {
			return errors.New("d2.cache_dir must not use a symlinked path")
		}
		if pathOverlapsMappings(d2.CacheDir, c.PathMappings) {
			return errors.New("d2.cache_dir must be outside media path mappings")
		}
	}
	if d2.Canary.ItemAllowlistFile != "" {
		if usesSymlink, inspectErr := pathsecurity.UsesSymlink(d2.Canary.ItemAllowlistFile); inspectErr != nil {
			return errors.New("d2.canary.item_allowlist_file path cannot be safely inspected")
		} else if usesSymlink {
			return errors.New("d2.canary.item_allowlist_file must not use a symlinked path")
		}
		if pathOverlapsMappings(d2.Canary.ItemAllowlistFile, c.PathMappings) {
			return errors.New("d2.canary.item_allowlist_file must be outside media path mappings")
		}
	}
	if d3.RefreshTimeoutSeconds < 1 || d3.RefreshTimeoutSeconds > 20 {
		return errors.New("d3.refresh_timeout_seconds must be between 1 and 20")
	}
	if d3.Canary.Enabled && strings.TrimSpace(d3.Canary.ItemAllowlistFile) == "" {
		return errors.New("d3.canary.item_allowlist_file is required when D3 Canary is enabled")
	}
	if d3.Canary.ItemAllowlistFile != "" && !isAbsolutePath(d3.Canary.ItemAllowlistFile) {
		return errors.New("d3.canary.item_allowlist_file must be an absolute path")
	}
	for name, value := range map[string]string{
		"d3.history_dir": d3.HistoryDir, "d3.quarantine_dir": d3.QuarantineDir,
		"d3.archive_dir": d3.ArchiveDir, "d3.trash_dir": d3.TrashDir,
		"d3.canary.item_allowlist_file": d3.Canary.ItemAllowlistFile,
	} {
		if value == "" {
			continue
		}
		if !isAbsolutePath(value) {
			return errors.New(name + " must be an absolute path")
		}
		if usesSymlink, inspectErr := pathsecurity.UsesSymlink(value); inspectErr != nil {
			return errors.New(name + " path cannot be safely inspected")
		} else if usesSymlink {
			return errors.New(name + " must not use a symlinked path")
		}
		if pathOverlapsMappings(value, c.PathMappings) {
			return errors.New(name + " must be outside media path mappings")
		}
	}
	if c.Features.WriteEnabled {
		privateDirs := []string{d3.HistoryDir, d3.QuarantineDir, d3.ArchiveDir, d3.TrashDir}
		for _, directory := range privateDirs {
			if strings.TrimSpace(directory) == "" {
				return errors.New("d3.history_dir, d3.quarantine_dir, d3.archive_dir and d3.trash_dir are required when writes are enabled")
			}
			if pathsecurity.IsFilesystemRoot(directory) {
				return errors.New("d3 private directories must be dedicated non-root directories")
			}
		}
		for i := range privateDirs {
			for j := i + 1; j < len(privateDirs); j++ {
				if pathsecurity.Overlaps(privateDirs[i], privateDirs[j]) {
					return errors.New("d3 private directories must not overlap")
				}
			}
		}
		if d2.Canary.Enabled != d3.Canary.Enabled {
			return errors.New("D2 and D3 Canary modes must match when writes are enabled")
		}
	}
	return nil
}

// DefaultAPIAuthScopes returns the read-only scopes assigned to the single
// application Bearer token when a deployment omits an explicit list.
func DefaultAPIAuthScopes() []string {
	return append([]string(nil), defaultAPIAuthScopes...)
}

// EffectiveAPIAuthScopes returns a defensive copy of the configured scopes or
// the safe read-only default. The write scope is accepted only by a validated
// D3-enabled configuration.
func (c SecurityConfig) EffectiveAPIAuthScopes() []string {
	if len(c.APIAuthScopes) == 0 {
		return DefaultAPIAuthScopes()
	}
	return append([]string(nil), c.APIAuthScopes...)
}

func validD2Language(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "zh-cn", "zh", "zho", "chi":
		return true
	default:
		return false
	}
}

func pathOverlapsMappings(candidate string, mappings []PathMapping) bool {
	for _, mapping := range mappings {
		if pathsecurity.Overlaps(candidate, mapping.Emby) || pathsecurity.Overlaps(candidate, mapping.Local) {
			return true
		}
	}
	return false
}

// ReadItemAllowlist reads exact item IDs from a protected, one-ID-per-line
// file for a D2 or D3 Canary. The values are returned only to the in-process
// allowlist and are never included in errors or logs.
func ReadItemAllowlist(filename string) ([]string, error) {
	if strings.TrimSpace(filename) == "" || !isAbsolutePath(filename) {
		return nil, errors.New("invalid Canary allowlist configuration")
	}
	contents, err := os.ReadFile(filename)
	if err != nil || len(contents) > maxAllowlistBytes {
		return nil, errors.New("unable to read Canary allowlist")
	}
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(filename)
		if statErr != nil || info.Mode().Perm()&0o077 != 0 {
			return nil, errors.New("Canary allowlist permissions are too broad")
		}
	}
	seen := make(map[string]struct{})
	items := make([]string, 0)
	for _, raw := range strings.Split(strings.ReplaceAll(string(contents), "\r\n", "\n"), "\n") {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if len([]byte(item)) > 512 || strings.ContainsAny(item, `/\\`) || strings.IndexFunc(item, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
			return nil, errors.New("invalid Canary allowlist")
		}
		if len(items) >= maxAllowlistEntries {
			return nil, errors.New("Canary allowlist is too large")
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, errors.New("Canary allowlist is empty")
	}
	return items, nil
}

func isAbsolutePath(value string) bool {
	value = strings.TrimSpace(value)
	return filepath.IsAbs(value) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\\`) || (len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '\\' || value[2] == '/'))
}

// ReadAPIKey reads and validates the credential without ever including its
// value in an error. The caller should keep the returned value in memory only
// as long as needed by the Emby client.
func ReadAPIKey(filename string) (string, error) {
	contents, err := os.ReadFile(filename)
	if err != nil {
		return "", errors.New("unable to read Emby API key file")
	}
	key := string(contents)
	if strings.HasSuffix(key, "\r\n") {
		key = strings.TrimSuffix(key, "\r\n")
	} else if strings.HasSuffix(key, "\n") || strings.HasSuffix(key, "\r") {
		key = key[:len(key)-1]
	}
	if key == "" || strings.IndexFunc(key, unicode.IsSpace) >= 0 {
		return "", errors.New("invalid Emby API key file")
	}
	return key, nil
}

// ReadIdentityKey reads a persistent, application-specific identity key.
// Keeping this separate from the Emby credential prevents credential rotation
// from changing every public subtitle ID. Errors never include the filename or
// secret contents.
func ReadIdentityKey(filename string) ([]byte, error) {
	contents, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.New("unable to read application identity key")
	}
	encoded := strings.TrimSuffix(string(contents), "\r\n")
	if encoded == string(contents) {
		encoded = strings.TrimSuffix(encoded, "\n")
		encoded = strings.TrimSuffix(encoded, "\r")
	}
	if encoded == "" || strings.IndexFunc(encoded, unicode.IsSpace) >= 0 {
		return nil, errors.New("invalid application identity key")
	}
	key, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(key) < 32 {
		return nil, errors.New("invalid application identity key")
	}
	return key, nil
}

// ReadAPIAuthToken reads the application-facing Bearer token. It requires a
// 256-bit-or-longer single-line token and rejects reuse of either the Emby API
// key or the decoded identity key. Error text never contains a secret or path.
func ReadAPIAuthToken(filename, embyAPIKey string, identityKey []byte) (string, error) {
	contents, err := os.ReadFile(filename)
	if err != nil {
		return "", errors.New("unable to read API auth token file")
	}
	token := string(contents)
	if strings.HasSuffix(token, "\r\n") {
		token = strings.TrimSuffix(token, "\r\n")
	} else if strings.HasSuffix(token, "\n") || strings.HasSuffix(token, "\r") {
		token = token[:len(token)-1]
	}
	if len(token) < 32 || strings.IndexFunc(token, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "", errors.New("invalid API auth token file")
	}
	if token == embyAPIKey || token == string(identityKey) || token == base64.StdEncoding.EncodeToString(identityKey) {
		return "", errors.New("API auth token must be distinct from other secrets")
	}
	return token, nil
}

// ReadAdminCredentialsFromEnv reads the deployment-owned administrator
// credentials from the two fixed Compose environment variables. Neither
// variable has a default, and the values are never included in an error.
func ReadAdminCredentialsFromEnv() (string, string, error) {
	username, usernameSet := os.LookupEnv(AdminUsernameEnv)
	password, passwordSet := os.LookupEnv(AdminPasswordEnv)
	if !usernameSet || username == "" {
		return "", "", errors.New(AdminUsernameEnv + " is required")
	}
	if !passwordSet || password == "" {
		return "", "", errors.New(AdminPasswordEnv + " is required")
	}
	if !validAdminUsername(username) {
		return "", "", errors.New("invalid " + AdminUsernameEnv)
	}
	if !validAdminPassword(password) {
		return "", "", errors.New("invalid " + AdminPasswordEnv)
	}
	return username, password, nil
}

func validAdminUsername(value string) bool {
	return len([]byte(value)) >= 1 && len([]byte(value)) <= 64 && strings.TrimSpace(value) == value && strings.IndexFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0
}

func validAdminPassword(value string) bool {
	return len([]byte(value)) >= 6 && len([]byte(value)) <= 256 && strings.IndexFunc(value, unicode.IsControl) < 0
}

// ValidateAdminPasswordDistinct prevents an administrator password from
// silently becoming an Emby, application Bearer, or identity credential.
// The compared values are never included in the returned error.
func ValidateAdminPasswordDistinct(password, embyAPIKey, apiAuthToken string, identityKey []byte) error {
	if password == embyAPIKey || password == apiAuthToken || password == string(identityKey) || password == base64.StdEncoding.EncodeToString(identityKey) {
		return errors.New("administrator password must be distinct from other secrets")
	}
	return nil
}

// ReadAPIToken is kept as a concise alias for callers using the older name.
func ReadAPIToken(filename, embyAPIKey string, identityKey []byte) (string, error) {
	return ReadAPIAuthToken(filename, embyAPIKey, identityKey)
}
