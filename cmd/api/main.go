package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ZhiH2333/WildBoat/internal/config"
	"github.com/ZhiH2333/WildBoat/internal/httpapi"
	"github.com/ZhiH2333/WildBoat/internal/httpapi/handlers"
	"github.com/ZhiH2333/WildBoat/internal/logging"
	"github.com/ZhiH2333/WildBoat/internal/persistence"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		logging.New("error").Error("config_invalid", "error", err.Error())
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel)
	logger.Info("starting",
		"app_env", cfg.AppEnv,
		"http_addr", cfg.HTTPAddr,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := persistence.MigrateUp(cfg.DatabaseURL); err != nil {
		logger.Error("migrate_failed", "error", err.Error())
		os.Exit(1)
	}
	logger.Info("migrations_applied")

	db, err := persistence.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db_connect_failed", "error", err.Error())
		os.Exit(1)
	}
	defer db.Close()

	router := httpapi.NewRouter(logger, handlers.Health{DB: db})
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http_listen", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown_signal")
	case err := <-errCh:
		if err != nil {
			logger.Error("http_server_failed", "error", err.Error())
			os.Exit(1)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown_failed", "error", err.Error())
		os.Exit(1)
	}
	logger.Info("shutdown_complete")
}
