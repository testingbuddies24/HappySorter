package httpserver

import (
	"html/template"
	"net/http"
	"net/url"
)

var layoutTmpl = template.Must(template.New("layout").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>{{.Title}} &middot; HappySorter</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>
    body { font-family: system-ui, sans-serif; max-width: 960px; margin: 2rem auto; padding: 0 1rem; color: #222; }
    nav a { margin-right: 1rem; }
    table { border-collapse: collapse; width: 100%; margin: 1rem 0; }
    th, td { border: 1px solid #ccc; padding: 0.4rem 0.6rem; text-align: left; font-size: 0.9rem; vertical-align: top; }
    .flash { background: #eef7ee; border: 1px solid #9c9; padding: 0.5rem 1rem; margin-bottom: 1rem; }
    .warn { background: #fff6e5; border: 1px solid #dba617; }
    fieldset { margin-bottom: 1.5rem; }
    label { display: block; margin: 0.6rem 0 0.2rem; font-weight: 600; }
    input[type=text], input[type=number] { width: 100%; max-width: 480px; padding: 0.3rem; box-sizing: border-box; }
    button { padding: 0.4rem 1rem; margin-top: 0.5rem; cursor: pointer; }
    .badge { display: inline-block; padding: 0.1rem 0.5rem; border-radius: 3px; font-size: 0.8rem; background: #eee; }
    .row-actions form { display: inline; }
    small.hint { display: block; color: #666; font-weight: normal; margin-top: 0.15rem; }
  </style>
</head>
<body>
  <nav>
    <a href="/">Dashboard</a>
    <a href="/setup/folders">Folders</a>
    <a href="/setup/sources">Sources</a>
    <a href="/setup/rename">Rename</a>
    <a href="/review">Review</a>
    <a href="/logs">Logs</a>
  </nav>
  <hr>
  {{if .Flash}}<p class="flash{{if .Warn}} warn{{end}}">{{.Flash}}</p>{{end}}
  {{.Body}}
</body>
</html>
`))

type pageData struct {
	Title string
	Flash string
	Warn  bool
	Body  template.HTML
}

// render writes the layout with body (already-rendered, trusted HTML)
// wrapped around it, plus an optional flash message from the query string.
func (s *Server) render(w http.ResponseWriter, r *http.Request, title string, body template.HTML) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := pageData{
		Title: title,
		Flash: r.URL.Query().Get("msg"),
		Warn:  r.URL.Query().Get("warn") == "1",
		Body:  body,
	}
	if err := layoutTmpl.Execute(w, data); err != nil {
		s.logger.Error("rendering page", "error", err)
	}
}

// redirectFlash redirects to path with msg/warn carried as query params, so
// the next GET can render them (Post/Redirect/Get pattern).
func redirectFlash(w http.ResponseWriter, r *http.Request, path, msg string, warn bool) {
	q := url.Values{}
	q.Set("msg", msg)
	if warn {
		q.Set("warn", "1")
	}
	http.Redirect(w, r, path+"?"+q.Encode(), http.StatusSeeOther)
}
