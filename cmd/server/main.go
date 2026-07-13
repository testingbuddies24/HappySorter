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
	"sort"
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
	"github.com/testingbuddies24/HappySorter/internal/scraper/s1"
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

	fileStore := store.NewFileStore(db)
	metaStore := store.NewMetadataStore(db)
	fileWatcher := watcher.New(cfg.Paths.Watch, logger)

	httpClient := &http.Client{Timeout: time.Duration(cfg.Scraping.TimeoutSeconds) * time.Second}
	mgr := scraper.NewManager(logger, buildAdapters(cfg, httpClient, logger)...)
	org := organiser.New(cfg.Paths.Library, cfg.Rename, httpClient)

	pl := pipeline.New(cfg, fileStore, metaStore, mgr, org, logger)

	queueSize := func() (int, error) {
		return fileStore.CountByState(store.StateScrape)
	}

	srv := httpserver.New(logger, queueSize)
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

// buildAdapters constructs one scraper.Adapter per enabled source in cfg,
// in priority order. Sources with no adapter implemented yet are logged
// and skipped rather than failing startup.
func buildAdapters(cfg *config.Config, client *http.Client, logger *slog.Logger) []scraper.Adapter {
	sources := append([]config.SourceConfig(nil), cfg.Sources...)
	sort.Slice(sources, func(i, j int) bool { return sources[i].Priority < sources[j].Priority })

	var adapters []scraper.Adapter
	for _, sc := range sources {
		if !sc.Enabled {
			continue
		}
		switch sc.Name {
		case "s1":
			adapters = append(adapters, s1.New(client))
		default:
			logger.Warn("source enabled in config but no adapter implemented yet", "source", sc.Name)
		}
	}
	return adapters
}
