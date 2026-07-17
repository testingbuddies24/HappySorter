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
	"sync"
	"syscall"
	"time"

	"github.com/testingbuddies24/HappySorter/internal/config"
	"github.com/testingbuddies24/HappySorter/internal/database"
	"github.com/testingbuddies24/HappySorter/internal/httpserver"
	"github.com/testingbuddies24/HappySorter/internal/logging"
	"github.com/testingbuddies24/HappySorter/internal/organiser"
	"github.com/testingbuddies24/HappySorter/internal/pipeline"
	"github.com/testingbuddies24/HappySorter/internal/scraper"
	"github.com/testingbuddies24/HappySorter/internal/scraper/registry"
	"github.com/testingbuddies24/HappySorter/internal/store"
	"github.com/testingbuddies24/HappySorter/internal/watcher"
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

	cfgStore := config.NewStore(configPath, cfg)

	fileStore := store.NewFileStore(db)
	metaStore := store.NewMetadataStore(db)
	logStore := store.NewLogStore(db)
	fileWatcher := watcher.New(cfg.Paths.Watch, logger)

	httpClient := &http.Client{Timeout: time.Duration(cfg.Scraping.TimeoutSeconds) * time.Second}
	managerStore := scraper.NewManagerStore(scraper.NewManager(logger, registry.BuildAdapters(cfg.Sources, httpClient, logger)...))
	org := organiser.New(cfgStore, httpClient)

	pl := pipeline.New(cfgStore, fileStore, metaStore, managerStore, org, logger)

	srv := httpserver.New(logger, cfgStore, fileStore, logStore, managerStore, pl, fileWatcher, httpClient)
	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: srv.Handler(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		fileWatcher.Run(ctx)
	}()
	go func() {
		defer wg.Done()
		pl.Run(ctx, fileWatcher.Events())
	}()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := httpSrv.Shutdown(shutdownCtx)
		wg.Wait()
		return err
	case err := <-errCh:
		stop()
		wg.Wait()
		return err
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
