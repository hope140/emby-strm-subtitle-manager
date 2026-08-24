package config

import (
	"bytes"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func validYAML(keyFile string) string {
	return "server:\n  listen_address: 127.0.0.1:8080\nemby:\n  url: https://emby.example.test\n  api_key_file: " + keyFile + "\n  timeout_seconds: 10\nsecurity:\n  identity_key_file: " + keyFile + "\nfeatures:\n  write_enabled: false\n  remote_search_enabled: false\npath_mappings:\n  - emby: /srv/media\n    local: /media\n"
}

func TestLoadFileAndReadAPIKey(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	secret := "s3cr3t-key-value"
	if err := os.WriteFile(keyFile, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(validYAML(keyFile)), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Emby.TimeoutSeconds != 10 {
		t.Fatalf("timeout = %d, want 10", cfg.Emby.TimeoutSeconds)
	}
	got, err := ReadAPIKey(keyFile)
	if err != nil || got != secret {
		t.Fatalf("ReadAPIKey() = %q, %v", got, err)
	}
}

func TestLoadFileRejectsUnknownFieldsAndFeatureGates(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	if err := os.WriteFile(keyFile, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, suffix := range map[string]string{
		"unknown":          "unknown_field: true\n",
		"write":            "features:\n  write_enabled: true\n  remote_search_enabled: false\n",
		"search":           "features:\n  write_enabled: false\n  remote_search_enabled: true\n",
		"missing-identity": "",
	} {
		t.Run(name, func(t *testing.T) {
			text := validYAML(keyFile)
			if name == "unknown" {
				text += suffix
			} else if name == "missing-identity" {
				text = strings.Replace(text, "security:\n  identity_key_file: "+keyFile+"\n", "", 1)
			} else {
				text = strings.Replace(text, "features:\n  write_enabled: false\n  remote_search_enabled: false\n", suffix, 1)
			}
			file := filepath.Join(dir, name+".yaml")
			if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadFile(file); err == nil {
				t.Fatal("LoadFile() succeeded for invalid configuration")
			}
		})
	}
}

func TestReadAPIKeyNeverReturnsSecretInError(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "empty-key")
	secret := "should-not-appear"
	if err := os.WriteFile(keyFile, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAPIKey(keyFile); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unexpected credential error: %v", err)
	}
	if _, err := ReadAPIKey(filepath.Join(dir, "missing")); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unexpected missing credential error: %v", err)
	}
}

func TestReadAPIKeyRejectsWhitespaceAndMultipleLines(t *testing.T) {
	dir := t.TempDir()
	for name, contents := range map[string]string{
		"empty":          "\n",
		"leading-space":  " key",
		"trailing-space": "key ",
		"embedded-space": "key value",
		"tab":            "key\tvalue",
		"multiple-lines": "key\nother",
		"extra-newline":  "key\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			filename := filepath.Join(dir, name)
			if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if key, err := ReadAPIKey(filename); err == nil || key != "" {
				t.Fatalf("ReadAPIKey() = %q, %v; expected a redacted validation error", key, err)
			}
		})
	}
}

func TestReadIdentityKey(t *testing.T) {
	dir := t.TempDir()
	key := bytes.Repeat([]byte{0x5a}, 32)
	filename := filepath.Join(dir, "identity-key")
	if err := os.WriteFile(filename, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadIdentityKey(filename)
	if err != nil || !bytes.Equal(got, key) {
		t.Fatalf("ReadIdentityKey() returned a different key or error: %v", err)
	}
}

func TestReadIdentityKeyRejectsInvalidValuesWithoutLeakage(t *testing.T) {
	dir := t.TempDir()
	secret := "must-not-appear"
	for name, contents := range map[string]string{
		"plain":      secret,
		"short":      base64.StdEncoding.EncodeToString([]byte("short")),
		"whitespace": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)) + " \n",
	} {
		t.Run(name, func(t *testing.T) {
			filename := filepath.Join(dir, name)
			if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if key, err := ReadIdentityKey(filename); err == nil || key != nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), filename) {
				t.Fatalf("unexpected identity key result: key length=%d error=%v", len(key), err)
			}
		})
	}
	missing := filepath.Join(dir, "missing-secret")
	if key, err := ReadIdentityKey(missing); err == nil || key != nil || strings.Contains(err.Error(), missing) {
		t.Fatalf("unexpected missing identity key result: key length=%d error=%v", len(key), err)
	}
}

func TestIsAbsolutePathPortable(t *testing.T) {
	for _, path := range []string{"/srv/media", `C:\media`, `D:/media`, `\\server\share\media`, `//server/share/media`} {
		if !isAbsolutePath(path) {
			t.Errorf("isAbsolutePath(%q) = false, want true", path)
		}
	}
	for _, path := range []string{"media", `C:media`, `..\media`} {
		if isAbsolutePath(path) {
			t.Errorf("isAbsolutePath(%q) = true, want false", path)
		}
	}
}

func TestExampleConfigIsValid(t *testing.T) {
	if _, err := LoadFile(filepath.Join("..", "..", "config", "config.example.yaml")); err != nil {
		t.Fatalf("config/config.example.yaml is invalid: %v", err)
	}
}

func TestValidateRequiresPathMapping(t *testing.T) {
	cfg := Config{
		Server:   ServerConfig{ListenAddress: "127.0.0.1:8080"},
		Emby:     EmbyConfig{URL: "https://emby.example.test", APIKeyFile: "/run/secrets/emby", TimeoutSeconds: 10},
		Security: SecurityConfig{IdentityKeyFile: "/run/secrets/identity"},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "path_mappings") {
		t.Fatalf("Validate() = %v, want path mapping error", err)
	}
}

func TestCIWorkflowYAMLIsValid(t *testing.T) {
	filename := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	var document any
	if err := decoder.Decode(&document); err != nil {
		t.Fatalf(".github/workflows/ci.yml is invalid: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf(".github/workflows/ci.yml must contain one YAML document, got %v", err)
	}
}

func TestDeploymentYAMLDocumentsAreValid(t *testing.T) {
	for _, filename := range []string{
		filepath.Join("..", "..", "deploy", "compose.example.yaml"),
		filepath.Join("..", "..", "deploy", "compose.host-network.example.yaml"),
		filepath.Join("..", "..", "deploy", "config.example.yaml"),
		filepath.Join("..", "..", "deploy", "config.host-network.example.yaml"),
	} {
		t.Run(filepath.Base(filename), func(t *testing.T) {
			contents, err := os.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			decoder := yaml.NewDecoder(bytes.NewReader(contents))
			var document any
			if err := decoder.Decode(&document); err != nil {
				t.Fatalf("invalid YAML: %v", err)
			}
			var extra any
			if err := decoder.Decode(&extra); err != io.EOF {
				t.Fatalf("expected one YAML document, got %v", err)
			}
		})
	}
}
