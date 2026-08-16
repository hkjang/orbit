package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hkjang/orbit/internal/config"
	"github.com/hkjang/orbit/internal/secure"
	"github.com/hkjang/orbit/internal/server"
	"github.com/hkjang/orbit/internal/store"
)

var (
	version = "dev"
	commit  = "unknown"
	builtAt = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	vault, err := secure.NewVault(cfg.EncryptionKey)
	if err != nil {
		slog.Error("initialize vault", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.Open(ctx, cfg.DatabaseURL, vault)
	if err != nil {
		slog.Error("initialize database", "error", err)
		os.Exit(1)
	}
	defer st.Close()
	if err = st.Bootstrap(ctx, cfg.BootstrapAdmin, cfg.BootstrapAdminPassword); err != nil {
		slog.Error("bootstrap administrator", "error", err)
		os.Exit(1)
	}
	_, _ = st.DB.Exec(context.Background(), `DELETE FROM sessions WHERE expires_at<=now()`)
	httpServer := &http.Server{Addr: ":8080", Handler: server.New(st, version, commit, builtAt), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 11 * time.Minute, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		slog.Info("orbit started", "version", version, "address", httpServer.Addr)
		if serveErr := httpServer.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			slog.Error("http server failed", "error", serveErr)
			os.Exit(1)
		}
	}()
	<-stop
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err = httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
	slog.Info("orbit stopped")
}
