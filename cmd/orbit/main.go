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

// listenAddr는 서버가 여는 주소다. 실행 환경 설정은 환경변수 네 개만 두기로
// 했으므로 여기에 두고, 컨테이너 밖에서는 포트 매핑으로 조정한다.
const listenAddr = ":8080"

// healthcheck는 컨테이너가 스스로를 점검하게 한다.
//
// 실행 이미지는 distroless라 셸도 curl도 없다. 그래서 헬스체크를 바이너리
// 자신이 수행한다. 설정을 읽지 않으므로 DB가 없어도 이 경로는 동작한다.
func healthcheck() int {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1" + listenAddr + "/readyz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func main() {
	// 설정을 읽기 전에 처리한다. 점검은 이미 떠 있는 서버에 묻는 일이라
	// 환경변수가 필요 없다.
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(healthcheck())
	}
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
	httpServer := &http.Server{Addr: listenAddr, Handler: server.New(st, version, commit, builtAt), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 11 * time.Minute, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
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
