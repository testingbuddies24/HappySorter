package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/testingbuddies24/HappySorter/internal/store"
)

// handleLogStream upgrades to an SSE (Server-Sent Events) connection and
// pushes every new log record to the client in real time.
func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	if s.logBroadcaster == nil {
		http.Error(w, "log streaming unavailable", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	ch, unsub := s.logBroadcaster.Subscribe()
	defer unsub()

	flusher.Flush()

	for {
		select {
		case rec, ok := <-ch:
			if !ok {
				return
			}
			data := logRecordFromSlog(rec)
			js, err := json.Marshal(data)
			if err != nil {
				continue
			}
			_, writeErr := w.Write([]byte("data: " + string(js) + "\n\n"))
			if writeErr != nil {
				return // client disconnected
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// handleLogsRecent returns the last 60 minutes of log records as JSON, newest
// first, so the dashboard's live activity feed can backfill on page load
// instead of starting empty and waiting for the next SSE event.
func (s *Server) handleLogsRecent(w http.ResponseWriter, r *http.Request) {
	records, err := s.logStore.Since(time.Now().Add(-60*time.Minute), 200)
	if err != nil {
		s.logger.Error("loading recent logs", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	out := make([]logRecordJSON, 0, len(records))
	for _, rec := range records {
		out = append(out, logRecordFromStore(rec))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		s.logger.Error("encoding recent logs", "error", err)
	}
}

// logRecordFromStore converts a persisted store.LogRecord into the same JSON
// shape the SSE stream emits, so client-side rendering code is shared.
func logRecordFromStore(r store.LogRecord) logRecordJSON {
	var fields map[string]any
	if r.Fields != "" {
		_ = json.Unmarshal([]byte(r.Fields), &fields)
	}
	return logRecordJSON{
		Time:    r.Time.Format(time.RFC3339),
		Level:   r.Level,
		Message: r.Message,
		Fields:  fields,
	}
}

// logRecordFromSlog converts a slog.Record into a JSON-serialisable struct.
func logRecordFromSlog(r slog.Record) logRecordJSON {
	fields := make(map[string]any)
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.Any()
		return true
	})
	return logRecordJSON{
		Time:    r.Time.Format(time.RFC3339),
		Level:   r.Level.String(),
		Message: r.Message,
		Fields:  fields,
	}
}

type logRecordJSON struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}
