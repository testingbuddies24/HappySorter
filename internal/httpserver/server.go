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
	"github.com/testingbuddies24/HappySorter/internal/logging"
	"github.com/testingbuddies24/HappySorter/internal/pipeline"
	"github.com/testingbuddies24/HappySorter/internal/scraper"
	"github.com/testingbuddies24/HappySorter/internal/store"
	"github.com/testingbuddies24/HappySorter/internal/watcher"
)

// Version is the running build's version string, overridden at build time
// via -ldflags "-X .../internal/httpserver.Version=vX.Y.Z" (see Dockerfile
// and .github/workflows/release.yml).
var Version = "dev"

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
	logger         *slog.Logger
	startedAt      time.Time
	mux            *http.ServeMux
	cfgStore       *config.Store
	fileStore      *store.FileStore
	logStore       *store.LogStore
	managerStore   *scraper.ManagerStore
	pipeline       *pipeline.Pipeline
	watcher        watcherControl
	httpClient     *http.Client
	logBroadcaster *logging.Broadcaster
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
	logBroadcaster *logging.Broadcaster,
) *Server {
	s := &Server{
		logger:         logger,
		startedAt:      time.Now(),
		mux:            http.NewServeMux(),
		cfgStore:       cfgStore,
		fileStore:      fileStore,
		logStore:       logStore,
		managerStore:   managerStore,
		pipeline:       pl,
		watcher:        w,
		httpClient:     httpClient,
		logBroadcaster: logBroadcaster,
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

	// TBC (formerly Review) — primary routes.
	s.mux.HandleFunc("GET /tbc", s.handleReviewGet)
	s.mux.HandleFunc("POST /tbc/retry", s.handleReviewRetry)
	s.mux.HandleFunc("POST /tbc/delete", s.handleReviewDelete)
	s.mux.HandleFunc("POST /tbc/empty", s.handleReviewEmpty)

	// Backward-compat: keep old /review routes.
	s.mux.HandleFunc("GET /review", s.handleReviewRedirect)
	s.mux.HandleFunc("POST /review/retry", s.handleReviewRetry)
	s.mux.HandleFunc("POST /review/delete", s.handleReviewDelete)
	s.mux.HandleFunc("POST /review/empty", s.handleReviewEmpty)

	s.mux.HandleFunc("GET /logs", s.handleLogs)

	// JSON & plain-text log APIs for the dashboard.
	s.mux.HandleFunc("GET /api/logs/stream", s.handleLogStream)
	s.mux.HandleFunc("GET /api/logs/text", s.handleLogsText)

	s.mux.HandleFunc("POST /rescan", s.handleRescan)
	s.mux.HandleFunc("POST /pause", s.handlePause)
	s.mux.HandleFunc("POST /resume", s.handleResume)
}

// handleReviewRedirect sends old /review links to /tbc.
func (s *Server) handleReviewRedirect(w http.ResponseWriter, r *http.Request) {
	redirectFlash(w, r, "/tbc", "The Review page has moved to TBC.", false)
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
  <button type="button" id="copy-logs">📋 Copy all logs</button>
</div>

<h2>Queue</h2>
<table>
  <tr><th>State</th><th>Count</th></tr>
  {{range .Counts}}<tr><td>{{.Label}}</td><td>{{.Count}}</td></tr>{{end}}
</table>

<h2>Live activity <span style="font-weight:400;font-size:.8rem;color:var(--muted)">(streaming)</span></h2>
<div class="toolbar">
  <label style="display:inline;font-weight:400;margin:0"><input type="checkbox" id="auto-scroll" checked> Auto-scroll</label>
  <select id="level-filter" style="width:auto;max-width:8rem">
    <option value="">All</option>
    <option value="DEBUG">Debug</option>
    <option value="INFO">Info</option>
    <option value="WARN">Warn</option>
    <option value="ERROR">Error</option>
  </select>
</div>

<table id="live-log">
  <thead>
    <tr><th style="width:9rem">Time</th><th>Level</th><th>Message</th><th>Fields</th></tr>
  </thead>
  <tbody>
    <tr><td colspan="4">Waiting for activity…</td></tr>
  </tbody>
</table>

<script>
(() => {
  const tbody = document.querySelector("#live-log tbody");
  const autoScroll = document.getElementById("auto-scroll");
  const levelFilter = document.getElementById("level-filter");
  const copyBtn = document.getElementById("copy-logs");
  let maxEntries = 200;

  let first = true;

  const es = new EventSource("/api/logs/stream");
  es.onmessage = (ev) => {
    try {
      const r = JSON.parse(ev.data);
      if (levelFilter.value && r.level.toUpperCase() !== levelFilter.value) return;
      if (first) { tbody.textContent = ""; first = false; }

      const tr = document.createElement("tr");
      const time = formatTime(r.time);
      const lvl = r.level;
      const msg = esc(r.message);
      const fields = esc(formatFields(r.fields));

      const tdTime = document.createElement("td");
      tdTime.textContent = time;
      tr.appendChild(tdTime);

      const tdLevel = document.createElement("td");
      const badge = document.createElement("span");
      badge.className = "badge";
      badge.setAttribute("data-level", lvl.toLowerCase());
      badge.textContent = lvl;
      tdLevel.appendChild(badge);
      tr.appendChild(tdLevel);

      const tdMsg = document.createElement("td");
      tdMsg.textContent = msg;
      tr.appendChild(tdMsg);

      const tdFields = document.createElement("td");
      tdFields.textContent = fields;
      tr.appendChild(tdFields);

      tr.style.display = "none";
      tbody.prepend(tr);
      requestAnimationFrame(() => tr.style.display = "");

      while (tbody.children.length > maxEntries) tbody.lastChild.remove();

      if (autoScroll.checked) tr.scrollIntoView({block: "start"});
    } catch {}
  };
  es.onerror = () => {};

  levelFilter.addEventListener("change", () => {
    first = true;
    tbody.textContent = "";
    const row = document.createElement("tr");
    const td = document.createElement("td");
    td.colSpan = 4;
    td.textContent = "Filtering…";
    row.appendChild(td);
    tbody.appendChild(row);
    es.close();
    setTimeout(() => location.reload(), 50);
  });

  copyBtn.addEventListener("click", async () => {
    try {
      const resp = await fetch("/api/logs/text?limit=500");
      const text = await resp.text();
      await navigator.clipboard.writeText(text);
      copyBtn.textContent = "✓ Copied!";
      setTimeout(() => copyBtn.textContent = "📋 Copy all logs", 2000);
    } catch (err) {
      copyBtn.textContent = "✗ Failed";
      setTimeout(() => copyBtn.textContent = "📋 Copy all logs", 2000);
    }
  });

  function formatTime(t) {
    if (!t) return "";
    const d = new Date(t);
    return d.getFullYear() + "-"
      + String(d.getMonth()+1).padStart(2,"0") + "-"
      + String(d.getDate()).padStart(2,"0") + " "
      + String(d.getHours()).padStart(2,"0") + ":"
      + String(d.getMinutes()).padStart(2,"0") + ":"
      + String(d.getSeconds()).padStart(2,"0");
  }
  function esc(s) {
    const d = document.createElement("div");
    d.textContent = s || "";
    return d.textContent;
  }
  function formatFields(f) {
    if (!f || Object.keys(f).length === 0) return "";
    return Object.entries(f).map(([k, v]) => k + "=" + JSON.stringify(v)).join(" ");
  }
})();
</script>
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
		store.StateReviewFilter:    "TBC: filtered",
		store.StateReviewUnmatched: "TBC: unmatched",
		store.StateReviewDuplicate: "TBC: duplicate",
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

	var buf bytes.Buffer
	err := dashboardTmpl.Execute(&buf, struct {
		Version   string
		StartedAt string
		Paused    bool
		Counts    []countRow
	}{
		Version:   Version,
		StartedAt: s.startedAt.Format(time.RFC3339),
		Paused:    s.watcher.Paused(),
		Counts:    counts,
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
