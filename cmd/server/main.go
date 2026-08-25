package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hope140/emby-strm-subtitle-manager/internal/auth"
	"github.com/hope140/emby-strm-subtitle-manager/internal/config"
	"github.com/hope140/emby-strm-subtitle-manager/internal/d2"
	"github.com/hope140/emby-strm-subtitle-manager/internal/embyclient"
	"github.com/hope140/emby-strm-subtitle-manager/internal/httpapi"
	"github.com/hope140/emby-strm-subtitle-manager/internal/httpui"
	"github.com/hope140/emby-strm-subtitle-manager/internal/inventory"
	"github.com/hope140/emby-strm-subtitle-manager/internal/pathmap"
	"github.com/hope140/emby-strm-subtitle-manager/internal/preview"
	"github.com/hope140/emby-strm-subtitle-manager/internal/subtitleprovider"
	"github.com/hope140/emby-strm-subtitle-manager/internal/version"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	configPath := flag.String("config", "config/config.yaml", "path to YAML configuration")
	flag.Parse()

	cfg, err := config.LoadFile(*configPath)
	if err != nil {
		logger.Error("configuration rejected", "error", err.Error())
		os.Exit(1)
	}
	apiKey, err := config.ReadAPIKey(cfg.Emby.APIKeyFile)
	if err != nil {
		logger.Error("credential configuration rejected", "error", err.Error())
		os.Exit(1)
	}
	identityKey, err := config.ReadIdentityKey(cfg.Security.IdentityKeyFile)
	if err != nil {
		logger.Error("identity configuration rejected", "error", err.Error())
		os.Exit(1)
	}
	authToken, err := config.ReadAPIAuthToken(cfg.Security.APIAuthTokenFile, apiKey, identityKey)
	if err != nil {
		logger.Error("API auth token configuration rejected", "error", err.Error())
		os.Exit(1)
	}
	username, password, err := config.ReadAdminCredentialsFromEnv()
	if err != nil {
		logger.Error("administrator environment configuration rejected", "error", err.Error())
		os.Exit(1)
	}
	if err := config.ValidateAdminPasswordDistinct(password, apiKey, authToken, identityKey); err != nil {
		logger.Error("administrator authentication configuration rejected", "error", err.Error())
		os.Exit(1)
	}
	adminAuth, err := auth.New(username, password, auth.Options{SessionTTL: time.Duration(cfg.Security.SessionTTLSeconds) * time.Second})
	if err != nil {
		logger.Error("administrator authentication configuration rejected", "error", err.Error())
		os.Exit(1)
	}
	mappings := make([]pathmap.Mapping, 0, len(cfg.PathMappings))
	localRoots := make([]string, 0, len(cfg.PathMappings))
	for _, mapping := range cfg.PathMappings {
		mappings = append(mappings, pathmap.Mapping{Emby: mapping.Emby, Local: mapping.Local})
		localRoots = append(localRoots, mapping.Local)
	}
	mapper, err := pathmap.New(mappings)
	if err != nil {
		logger.Error("path mapping configuration rejected", "error", err.Error())
		os.Exit(1)
	}
	guard, err := pathmap.NewPathGuard(localRoots)
	if err != nil {
		logger.Error("media root configuration rejected", "error", err.Error())
		os.Exit(1)
	}
	inventoryService, err := inventory.New(inventory.Options{FileSystem: inventory.OSFileSystem{}, IdentityKey: identityKey, Mapper: mapper, Guard: guard})
	if err != nil {
		logger.Error("subtitle inventory configuration rejected", "error", err.Error())
		os.Exit(1)
	}
	client, err := embyclient.New(embyclient.Config{
		BaseURL: cfg.Emby.URL,
		APIKey:  apiKey,
		Timeout: time.Duration(cfg.Emby.TimeoutSeconds) * time.Second,
	})
	if err != nil {
		logger.Error("Emby client configuration rejected", "error", err.Error())
		os.Exit(1)
	}
	var allowlist *preview.Allowlist
	if cfg.D2.Canary.Enabled {
		items, err := config.ReadItemAllowlist(cfg.D2.Canary.ItemAllowlistFile)
		if err != nil {
			logger.Error("D2 Canary allowlist rejected", "error", err.Error())
			os.Exit(1)
		}
		allowlist = preview.NewAllowlist(items)
	}
	d2Service, err := d2.New(d2.Options{
		Config: cfg.D2, RemoteSearchEnabled: cfg.Features.RemoteSearchEnabled,
		CanaryEnabled: cfg.D2.Canary.Enabled, Allowlist: allowlist, Emby: client,
		Provider:    subtitleprovider.NewEmbyRemoteSubtitleProvider(client),
		AuthContext: d2.AuthContextFromToken(authToken),
	})
	if err != nil {
		logger.Error("D2 configuration rejected", "error", err.Error())
		os.Exit(1)
	}
	cleanupContext, stopCleanup := context.WithCancel(context.Background())
	defer stopCleanup()
	go d2Service.RunCleanup(cleanupContext)

	server := &http.Server{
		Addr: cfg.Server.ListenAddress,
		Handler: httpapi.NewServerWithServices(cfg, version.Current(), logger, httpapi.Services{
			Emby: client, D2: d2Service, Mapper: mapper, Guard: guard, Inventory: inventoryService,
			AuthToken: authToken, AuthTokenScopes: cfg.Security.EffectiveAPIAuthScopes(), AdminAuth: adminAuth, UI: httpui.NewHandler(),
		}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "error", err.Error())
		}
	}()

	logger.Info("server starting", "version", version.Version)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped unexpectedly", "error", err.Error())
		os.Exit(1)
	}
}
