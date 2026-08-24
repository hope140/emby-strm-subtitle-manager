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
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

const (
	defaultTimeoutSeconds = 30
	maxTimeoutSeconds     = 300
)

// Config is the complete non-secret runtime configuration.
type Config struct {
	Server       ServerConfig   `yaml:"server"`
	Emby         EmbyConfig     `yaml:"emby"`
	Security     SecurityConfig `yaml:"security"`
	Features     FeatureConfig  `yaml:"features"`
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
	IdentityKeyFile string `yaml:"identity_key_file"`
}

type FeatureConfig struct {
	WriteEnabled        bool `yaml:"write_enabled" json:"write_enabled"`
	RemoteSearchEnabled bool `yaml:"remote_search_enabled" json:"remote_search_enabled"`
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
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate checks the safe D1 configuration boundary.
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
	if c.Emby.TimeoutSeconds < 1 || c.Emby.TimeoutSeconds > maxTimeoutSeconds {
		return fmt.Errorf("emby.timeout_seconds must be between 1 and %d", maxTimeoutSeconds)
	}
	if c.Features.WriteEnabled {
		return errors.New("features.write_enabled must be false in D1")
	}
	if c.Features.RemoteSearchEnabled {
		return errors.New("features.remote_search_enabled must be false in D1")
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
	return nil
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
