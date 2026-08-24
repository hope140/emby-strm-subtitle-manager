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

	"github.com/hope140/emby-strm-subtitle-manager/internal/config"
	"github.com/hope140/emby-strm-subtitle-manager/internal/httpapi"
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
	if _, err := config.ReadAPIKey(cfg.Emby.APIKeyFile); err != nil {
		logger.Error("credential configuration rejected", "error", err.Error())
		os.Exit(1)
	}
	if _, err := config.ReadIdentityKey(cfg.Security.IdentityKeyFile); err != nil {
		logger.Error("identity configuration rejected", "error", err.Error())
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              cfg.Server.ListenAddress,
		Handler:           httpapi.NewServer(cfg, version.Current(), logger).Handler(),
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
