// Command server is HappySorter's entrypoint: loads config, opens the
// database, and serves the setup GUI / health endpoint.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/testingbuddies24/HappySorter/internal/config"
	"github.com/testingbuddies24/HappySorter/internal/database"
	"github.com/testingbuddies24/HappySorter/internal/httpserver"
	"github.com/testingbuddies24/HappySorter/internal/logging"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := envOr("HAPPYSORTER_CONFIG", "/config/config.yaml")
	dbPath := envOr("HAPPYSORTER_DB", "/config/happy-sorter.db")

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	logger := logging.New(cfg.Server.LogLevel, db)
	slog.SetDefault(logger)
	logger.Info("starting HappySorter", "config", configPath, "db", dbPath)

	srv := httpserver.New(logger)
	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: srv.Handler(),
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
