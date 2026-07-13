// Package httpserver serves the setup GUI and health endpoint
// (docs/ARCHITECTURE.md § 2.10).
//
// Milestone 0 ships only "/" (a placeholder dashboard) and "/healthz".
// Setup pages land in Milestone 3.
package httpserver

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"time"
)

// Version is the running build's version string.
const Version = "0.1.0-dev"

type Server struct {
	logger    *slog.Logger
	startedAt time.Time
	mux       *http.ServeMux
}

func New(logger *slog.Logger) *Server {
	s := &Server{
		logger:    logger,
		startedAt: time.Now(),
		mux:       http.NewServeMux(),
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
}

var dashboardTmpl = template.Must(template.New("dashboard").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>HappySorter</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
  <h1>HappySorter</h1>
  <p>Version {{.Version}} &middot; up since {{.StartedAt}}</p>
  <p>Setup, sources, and logs will live here (Milestone 3).</p>
</body>
</html>
`))

func (s *Server) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := struct {
		Version   string
		StartedAt string
	}{
		Version:   Version,
		StartedAt: s.startedAt.Format(time.RFC3339),
	}
	if err := dashboardTmpl.Execute(w, data); err != nil {
		s.logger.Error("rendering dashboard", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

type healthzResponse struct {
	Version      string `json:"version"`
	UptimeSecond int64  `json:"uptime_seconds"`
	QueueSize    int    `json:"queue_size"`
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	resp := healthzResponse{
		Version:      Version,
		UptimeSecond: int64(time.Since(s.startedAt).Seconds()),
		QueueSize:    0, // pipeline worker lands in Milestone 1
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
