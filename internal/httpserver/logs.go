package httpserver

import (
	"bytes"
	"html/template"
	"net/http"
	"strconv"

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
