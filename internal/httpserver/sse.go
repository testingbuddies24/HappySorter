package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
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
