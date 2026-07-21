package httpserver

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/testingbuddies24/HappySorter/internal/store"
)

var logsTmpl = template.Must(template.New("logs").Parse(`
<form method="get" action="/logs">
  <label for="level">Level</label>
  <select id="level" name="level">
    <option value=""{{if eq .Level ""}} selected{{end}}>All</option>
    <option value="debug"{{if eq .Level "debug"}} selected{{end}}>Debug</option>
    <option value="info"{{if eq .Level "info"}} selected{{end}}>Info</option>
    <option value="warn"{{if eq .Level "warn"}} selected{{end}}>Warn</option>
    <option value="error"{{if eq .Level "error"}} selected{{end}}>Error</option>
  </select>
  <label for="limit">Show last</label>
  <input type="number" id="limit" name="limit" value="{{.Limit}}" style="width:6rem">
  <button type="submit">Filter</button>
</form>

<table>
  <tr><th>Time</th><th>Level</th><th>Message</th><th>Fields</th></tr>
  {{range .Records}}
  <tr>
    <td>{{.Time.Format "2006-01-02 15:04:05"}}</td>
    <td><span class="badge" data-level="{{.Level}}">{{.Level}}</span></td>
    <td>{{.Message}}</td>
    <td>{{.Fields}}</td>
  </tr>
  {{else}}
  <tr><td colspan="4">No log entries.</td></tr>
  {{end}}
</table>
`))

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	level := r.URL.Query().Get("level")
	limit := 200
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}

	records, err := s.logStore.Tail(limit, level)
	if err != nil {
		s.logger.Error("tailing logs", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	err = logsTmpl.Execute(&buf, struct {
		Level   string
		Limit   int
		Records []store.LogRecord
	}{
		Level:   level,
		Limit:   limit,
		Records: records,
	})
	if err != nil {
		s.logger.Error("rendering logs page", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.render(w, r, "Logs", template.HTML(buf.String()))
}

// handleLogsText returns log entries as plain text for one-click copy.
func (s *Server) handleLogsText(w http.ResponseWriter, r *http.Request) {
	limit := 500
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}
	level := r.URL.Query().Get("level")

	records, err := s.logStore.Tail(limit, level)
	if err != nil {
		s.logger.Error("tailing logs for text export", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	var buf bytes.Buffer
	for _, rec := range records {
		fmt.Fprintf(&buf, "[%s] [%s] %s",
			rec.Time.Format("2006-01-02 15:04:05"),
			strings.ToUpper(rec.Level),
			rec.Message,
		)
		if rec.Fields != "" && rec.Fields != "{}" {
			fmt.Fprintf(&buf, " %s", rec.Fields)
		}
		buf.WriteByte('\n')
	}
	if buf.Len() == 0 {
		buf.WriteString("(no log entries)\n")
	}
	w.Write(buf.Bytes())
}
