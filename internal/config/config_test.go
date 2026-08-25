package config

import (
	"bytes"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func validYAML(keyFile string) string {
	return "server:\n  listen_address: 127.0.0.1:8080\nemby:\n  url: https://emby.example.test\n  api_key_file: " + keyFile + "\n  timeout_seconds: 10\nsecurity:\n  identity_key_file: " + keyFile + "\n  api_auth_token_file: " + keyFile + "\nfeatures:\n  write_enabled: false\n  remote_search_enabled: false\npath_mappings:\n  - emby: /srv/media\n    local: /media\n"
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

func TestReadAPIAuthTokenRequiresHighEntropyAndDistinctSecrets(t *testing.T) {
	dir := t.TempDir()
	identity := bytes.Repeat([]byte{0x5a}, 32)
	embyKey := "emby-key-never-reused"
	token := strings.Repeat("T", 32)
	filename := filepath.Join(dir, "api-auth-token")
	if err := os.WriteFile(filename, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAPIAuthToken(filename, embyKey, identity)
	if err != nil || got != token {
		t.Fatalf("ReadAPIAuthToken() = %q, %v", got, err)
	}
	identityEncoded := base64.StdEncoding.EncodeToString(identity)
	for name, contents := range map[string]string{
		"empty":        "\n",
		"short":        strings.Repeat("s", 31),
		"whitespace":   strings.Repeat("s", 31) + " x",
		"emby-reuse":   embyKey,
		"identity":     string(identity),
		"identity-b64": identityEncoded,
	} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(dir, name)
			if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if got, err := ReadAPIAuthToken(file, embyKey, identity); err == nil || got != "" || strings.Contains(err.Error(), contents) || strings.Contains(err.Error(), file) {
				t.Fatalf("unexpected token result: %q, %v", got, err)
			}
		})
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
		Security: SecurityConfig{IdentityKeyFile: "/run/secrets/identity", APIAuthTokenFile: "/run/secrets/api-auth-token"},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "path_mappings") {
		t.Fatalf("Validate() = %v, want path mapping error", err)
	}
}

func TestD2DefaultsCanaryAndProtectedAllowlist(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	allowlistFile := filepath.Join(dir, "canary-items")
	if err := os.WriteFile(keyFile, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(allowlistFile, []byte("movie-1\nepisode-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(dir, "d2.yaml")
	text := validYAML(keyFile)
	text = strings.Replace(text, "remote_search_enabled: false", "remote_search_enabled: true", 1)
	text = strings.Replace(text, "path_mappings:\n", "d2:\n  cache_dir: "+filepath.ToSlash(filepath.Join(dir, "cache"))+"\n  canary:\n    enabled: true\n    item_allowlist_file: "+filepath.ToSlash(allowlistFile)+"\npath_mappings:\n", 1)
	if err := os.WriteFile(configFile, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Features.RemoteSearchEnabled || !cfg.D2.Canary.Enabled || cfg.D2.CandidateTTLSeconds != 600 || cfg.D2.ArtifactTTLSeconds != 1200 || cfg.D2.MaxSubtitleBytes != 4<<20 {
		t.Fatalf("D2 config defaults = %#v", cfg.D2)
	}
	items, err := ReadItemAllowlist(cfg.D2.Canary.ItemAllowlistFile)
	if err != nil || len(items) != 2 || items[0] != "movie-1" {
		t.Fatalf("allowlist = %#v, %v", items, err)
	}
}

func TestD2PrivatePathsCannotOverlapMediaMappings(t *testing.T) {
	cfg := Config{
		Server:       ServerConfig{ListenAddress: "127.0.0.1:8080"},
		Emby:         EmbyConfig{URL: "https://emby.example.test", APIKeyFile: "/run/secrets/emby", TimeoutSeconds: 10},
		Security:     SecurityConfig{IdentityKeyFile: "/run/secrets/identity", APIAuthTokenFile: "/run/secrets/api-auth-token"},
		D2:           D2Config{CacheDir: "/srv/media/.preview-cache"},
		PathMappings: []PathMapping{{Emby: "/srv/media", Local: "/media"}},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "outside media") {
		t.Fatalf("Validate() = %v, want private-path error", err)
	}
}

func TestD2EnabledRequiresStableCacheDirectory(t *testing.T) {
	cfg := Config{
		Server:   ServerConfig{ListenAddress: "127.0.0.1:8080"},
		Emby:     EmbyConfig{URL: "https://emby.example.test", APIKeyFile: "/run/secrets/emby", TimeoutSeconds: 10},
		Security: SecurityConfig{IdentityKeyFile: "/run/secrets/identity", APIAuthTokenFile: "/run/secrets/api-auth-token"},
		Features: FeatureConfig{RemoteSearchEnabled: true},
		D2: D2Config{Canary: D2CanaryConfig{
			Enabled: true, ItemAllowlistFile: "/run/secrets/d2-canary-items",
		}},
		PathMappings: []PathMapping{{Emby: "/srv/media", Local: "/media"}},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "d2.cache_dir") {
		t.Fatalf("Validate() = %v, want explicit cache_dir error", err)
	}
}

func TestD2CacheDirectoryMustNotOverlapOrUseFilesystemRoot(t *testing.T) {
	base := t.TempDir()
	mediaRoot := filepath.Join(base, "media")
	localRoot := filepath.Join(base, "local-media")
	root := string(filepath.Separator)
	if volume := filepath.VolumeName(base); volume != "" {
		root = volume + string(filepath.Separator)
	}
	cases := map[string]struct {
		cache string
		want  bool
	}{
		"filesystem-root": {cache: root, want: true},
		"media-ancestor":  {cache: filepath.Dir(mediaRoot), want: true},
		"media-child":     {cache: filepath.Join(mediaRoot, ".d2-cache"), want: true},
		"media-equal":     {cache: mediaRoot, want: true},
		"safe-sibling":    {cache: filepath.Join(base, "d2-preview-cache"), want: false},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := Config{
				Server:       ServerConfig{ListenAddress: "127.0.0.1:8080"},
				Emby:         EmbyConfig{URL: "https://emby.example.test", APIKeyFile: "/run/secrets/emby", TimeoutSeconds: 10},
				Security:     SecurityConfig{IdentityKeyFile: "/run/secrets/identity", APIAuthTokenFile: "/run/secrets/api-auth-token"},
				D2:           D2Config{CacheDir: testCase.cache},
				PathMappings: []PathMapping{{Emby: mediaRoot, Local: localRoot}},
			}
			err := cfg.Validate()
			if testCase.want && err == nil {
				t.Fatalf("Validate() succeeded for unsafe cache_dir %q", testCase.cache)
			}
			if !testCase.want && err != nil {
				t.Fatalf("Validate() rejected dedicated sibling cache: %v", err)
			}
		})
	}
}

func TestD2CacheDirectoryRejectsSymlinkedParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	base := t.TempDir()
	target := filepath.Join(base, "private-target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "cache-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Server:       ServerConfig{ListenAddress: "127.0.0.1:8080"},
		Emby:         EmbyConfig{URL: "https://emby.example.test", APIKeyFile: "/run/secrets/emby", TimeoutSeconds: 10},
		Security:     SecurityConfig{IdentityKeyFile: "/run/secrets/identity", APIAuthTokenFile: "/run/secrets/api-auth-token"},
		D2:           D2Config{CacheDir: filepath.Join(link, "nested")},
		PathMappings: []PathMapping{{Emby: filepath.Join(base, "media"), Local: filepath.Join(base, "local-media")}},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "symlinked") {
		t.Fatalf("Validate() = %v, want symlink protection error", err)
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
		filepath.Join("..", "..", "deploy", "compose.d2-canary.example.yaml"),
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

type composeTestVolume struct {
	Type     string `yaml:"type"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"read_only"`
}

type composeTestSecretRef struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

type composeTestService struct {
	ReadOnly bool                   `yaml:"read_only"`
	Volumes  []composeTestVolume    `yaml:"volumes"`
	Secrets  []composeTestSecretRef `yaml:"secrets"`
}

type composeTestSecret struct {
	File string `yaml:"file"`
}

type composeTestDocument struct {
	Services map[string]composeTestService `yaml:"services"`
	Secrets  map[string]composeTestSecret  `yaml:"secrets"`
}

func readComposeTestDocument(t *testing.T, filename string) composeTestDocument {
	t.Helper()
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var document composeTestDocument
	if err := yaml.Unmarshal(contents, &document); err != nil {
		t.Fatalf("invalid Compose YAML: %v", err)
	}
	return document
}

func TestBaseComposeHasNoD2Dependencies(t *testing.T) {
	for _, filename := range []string{
		filepath.Join("..", "..", "deploy", "compose.example.yaml"),
		filepath.Join("..", "..", "deploy", "compose.host-network.example.yaml"),
	} {
		t.Run(filepath.Base(filename), func(t *testing.T) {
			document := readComposeTestDocument(t, filename)
			app, ok := document.Services["app"]
			if !ok || !app.ReadOnly {
				t.Fatal("base app must keep read_only: true")
			}
			if len(app.Secrets) != 3 {
				t.Fatalf("base Compose must keep exactly the three D1 secrets, got %d", len(app.Secrets))
			}
			mediaFound := false
			for _, mount := range app.Volumes {
				if mount.Target == "/var/lib/emby-strm-subtitle-manager/d2-preview-cache" {
					t.Fatal("base Compose must not mount the D2 cache")
				}
				if mount.Target == "/media" && !mount.ReadOnly {
					t.Fatalf("media mount must remain read-only = %#v", mount)
				}
				if mount.Target == "/media" {
					mediaFound = true
				}
			}
			if !mediaFound {
				t.Fatal("base Compose must keep its media mount")
			}
			for _, ref := range app.Secrets {
				if ref.Source == "d2_canary_items" || ref.Target == "d2_canary_items" {
					t.Fatal("base Compose must not reference the D2 allowlist secret")
				}
			}
			if _, ok := document.Secrets["d2_canary_items"]; ok {
				t.Fatal("base Compose must not define the D2 allowlist secret")
			}
		})
	}
}

func TestD2ComposeOverlayAddsDedicatedWritableCacheAndAllowlistSecret(t *testing.T) {
	overlayFilename := filepath.Join("..", "..", "deploy", "compose.d2-canary.example.yaml")
	overlay := readComposeTestDocument(t, overlayFilename)
	overlayApp, ok := overlay.Services["app"]
	if !ok {
		t.Fatal("D2 overlay is missing app service additions")
	}
	cacheFound := false
	for _, mount := range overlayApp.Volumes {
		if mount.Target == "/var/lib/emby-strm-subtitle-manager/d2-preview-cache" {
			if mount.Type != "bind" || mount.Source != "/replace/with/dedicated/d2-preview-cache" || mount.ReadOnly {
				t.Fatalf("unsafe D2 cache overlay mount = %#v", mount)
			}
			cacheFound = true
		}
	}
	if !cacheFound {
		t.Fatal("D2 overlay is missing its dedicated writable cache mount")
	}
	allowlistRef := false
	for _, ref := range overlayApp.Secrets {
		if ref.Source == "d2_canary_items" && ref.Target == "d2_canary_items" {
			allowlistRef = true
		}
	}
	allowlist, ok := overlay.Secrets["d2_canary_items"]
	if !allowlistRef || !ok || allowlist.File != "./secrets/d2_canary_items" {
		t.Fatalf("D2 overlay allowlist is not a file source: ref=%v definition=%#v", allowlistRef, allowlist)
	}

	for _, baseFilename := range []string{
		filepath.Join("..", "..", "deploy", "compose.example.yaml"),
		filepath.Join("..", "..", "deploy", "compose.host-network.example.yaml"),
	} {
		t.Run(filepath.Base(baseFilename), func(t *testing.T) {
			base := readComposeTestDocument(t, baseFilename)
			baseApp := base.Services["app"]
			mergedVolumes := append(append([]composeTestVolume(nil), baseApp.Volumes...), overlayApp.Volumes...)
			mergedSecrets := append(append([]composeTestSecretRef(nil), baseApp.Secrets...), overlayApp.Secrets...)
			if !baseApp.ReadOnly {
				t.Fatal("base rootfs must remain read-only after overlay merge")
			}
			mergedCache := false
			mergedMediaReadOnly := false
			for _, mount := range mergedVolumes {
				if mount.Target == "/var/lib/emby-strm-subtitle-manager/d2-preview-cache" && !mount.ReadOnly {
					mergedCache = true
				}
				if mount.Target == "/media" && mount.ReadOnly {
					mergedMediaReadOnly = true
				}
			}
			mergedAllowlist := false
			for _, ref := range mergedSecrets {
				if ref.Source == "d2_canary_items" && ref.Target == "d2_canary_items" {
					mergedAllowlist = true
				}
			}
			if !mergedCache || !mergedMediaReadOnly || !mergedAllowlist {
				t.Fatalf("base+overlay D2 contract missing: cache=%v media_read_only=%v allowlist=%v", mergedCache, mergedMediaReadOnly, mergedAllowlist)
			}
		})
	}
}
