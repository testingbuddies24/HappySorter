// Package httpserver serves the setup GUI, review queue, log viewer, and
// health endpoint (docs/ARCHITECTURE.md § 2.10).
package httpserver

import (
	"bytes"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/testingbuddies24/HappySorter/internal/config"
	"github.com/testingbuddies24/HappySorter/internal/pipeline"
	"github.com/testingbuddies24/HappySorter/internal/scraper"
	"github.com/testingbuddies24/HappySorter/internal/store"
	"github.com/testingbuddies24/HappySorter/internal/watcher"
)

// Version is the running build's version string.
const Version = "0.3.0-dev"

// watcherControl is the subset of *watcher.Watcher the GUI needs — kept as
// an interface so tests could fake it, though production always passes the
// real watcher.
type watcherControl interface {
	Pause()
	Resume()
	Paused() bool
	Rescan()
}

type Server struct {
	logger       *slog.Logger
	startedAt    time.Time
	mux          *http.ServeMux
	cfgStore     *config.Store
	fileStore    *store.FileStore
	logStore     *store.LogStore
	managerStore *scraper.ManagerStore
	pipeline     *pipeline.Pipeline
	watcher      watcherControl
	httpClient   *http.Client
}

// New builds the HTTP server. All dependencies are shared with the pipeline
// goroutine, so GUI edits (sources, rename, folders) and actions
// (pause/resume/rescan, review retry/delete) take effect on the same
// running process without a restart, except where noted (watch path, port).
func New(
	logger *slog.Logger,
	cfgStore *config.Store,
	fileStore *store.FileStore,
	logStore *store.LogStore,
	managerStore *scraper.ManagerStore,
	pl *pipeline.Pipeline,
	w *watcher.Watcher,
	httpClient *http.Client,
) *Server {
	s := &Server{
		logger:       logger,
		startedAt:    time.Now(),
		mux:          http.NewServeMux(),
		cfgStore:     cfgStore,
		fileStore:    fileStore,
		logStore:     logStore,
		managerStore: managerStore,
		pipeline:     pl,
		watcher:      w,
		httpClient:   httpClient,
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.handleDashboard)
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)

	s.mux.HandleFunc("GET /setup/folders", s.handleFoldersGet)
	s.mux.HandleFunc("POST /setup/folders", s.handleFoldersPost)
	s.mux.HandleFunc("GET /setup/sources", s.handleSourcesGet)
	s.mux.HandleFunc("POST /setup/sources", s.handleSourcesPost)
	s.mux.HandleFunc("GET /setup/rename", s.handleRenameGet)
	s.mux.HandleFunc("POST /setup/rename", s.handleRenamePost)

	s.mux.HandleFunc("GET /review", s.handleReviewGet)
	s.mux.HandleFunc("POST /review/retry", s.handleReviewRetry)
	s.mux.HandleFunc("POST /review/delete", s.handleReviewDelete)
	s.mux.HandleFunc("POST /review/empty", s.handleReviewEmpty)

	s.mux.HandleFunc("GET /logs", s.handleLogs)

	s.mux.HandleFunc("POST /rescan", s.handleRescan)
	s.mux.HandleFunc("POST /pause", s.handlePause)
	s.mux.HandleFunc("POST /resume", s.handleResume)
}

var dashboardTmpl = template.Must(template.New("dashboard").Parse(`
<p class="meta">Version {{.Version}} &middot; up since {{.StartedAt}} &middot; watcher is <strong>{{if .Paused}}paused{{else}}running{{end}}</strong></p>

<div class="toolbar">
  <form method="post" action="{{if .Paused}}/resume{{else}}/pause{{end}}">
    <button type="submit">{{if .Paused}}Resume{{else}}Pause{{end}} watcher</button>
  </form>
  <form method="post" action="/rescan">
    <button type="submit">Trigger rescan</button>
  </form>
</div>

<h2>Queue</h2>
<table>
  <tr><th>State</th><th>Count</th></tr>
  {{range .Counts}}<tr><td>{{.Label}}</td><td>{{.Count}}</td></tr>{{end}}
</table>

<h2>Recent activity</h2>
<table>
  <tr><th>Updated</th><th>State</th><th>Code</th><th>Path</th><th>Reason</th></tr>
  {{range .Recent}}
  <tr>
    <td>{{.UpdatedAt.Format "2006-01-02 15:04:05"}}</td>
    <td><span class="badge" data-state="{{.State}}">{{.State}}</span></td>
    <td>{{.Code}}</td>
    <td>{{.CurrentPath}}</td>
    <td>{{.Reason}}</td>
  </tr>
  {{else}}
  <tr><td colspan="5">Nothing processed yet — drop a file into the watch folder.</td></tr>
  {{end}}
</table>
`))

type countRow struct {
	Label string
	Count int
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	states := []store.FileState{
		store.StateScrape, store.StateDone,
		store.StateReviewFilter, store.StateReviewUnmatched, store.StateReviewDuplicate,
		store.StateFailed,
	}
	labels := map[store.FileState]string{
		store.StateScrape:          "Queued (awaiting scrape)",
		store.StateDone:            "Organised",
		store.StateReviewFilter:    "Review: filtered",
		store.StateReviewUnmatched: "Review: unmatched",
		store.StateReviewDuplicate: "Review: duplicate",
		store.StateFailed:          "Failed",
	}
	counts := make([]countRow, 0, len(states))
	for _, st := range states {
		n, err := s.fileStore.CountByState(st)
		if err != nil {
			s.logger.Error("counting files by state", "state", st, "error", err)
		}
		counts = append(counts, countRow{Label: labels[st], Count: n})
	}

	recent, err := s.fileStore.ListRecent(20)
	if err != nil {
		s.logger.Error("listing recent files", "error", err)
	}

	var buf bytes.Buffer
	err = dashboardTmpl.Execute(&buf, struct {
		Version   string
		StartedAt string
		Paused    bool
		Counts    []countRow
		Recent    []store.FileRecord
	}{
		Version:   Version,
		StartedAt: s.startedAt.Format(time.RFC3339),
		Paused:    s.watcher.Paused(),
		Counts:    counts,
		Recent:    recent,
	})
	if err != nil {
		s.logger.Error("rendering dashboard", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.render(w, r, "Dashboard", template.HTML(buf.String()))
}

type healthzResponse struct {
	Version      string `json:"version"`
	UptimeSecond int64  `json:"uptime_seconds"`
	QueueSize    int    `json:"queue_size"`
	Paused       bool   `json:"paused"`
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	queueSize, err := s.fileStore.CountByState(store.StateScrape)
	if err != nil {
		s.logger.Error("querying queue size", "error", err)
	}

	resp := healthzResponse{
		Version:      Version,
		UptimeSecond: int64(time.Since(s.startedAt).Seconds()),
		QueueSize:    queueSize,
		Paused:       s.watcher.Paused(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
