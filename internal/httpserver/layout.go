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
    :root {
      --bg: #eef1f7; --surface: #ffffff; --surface-2: #f4f6fb; --border: #e0e4ee;
      --text: #1b1e28; --muted: #6a7185; --accent: #5b5bd6; --accent-hover: #4a4ac2;
      --accent-soft: #ecebfb; --on-accent: #ffffff;
      --ok: #16a34a; --ok-soft: #e7f6ec; --warn: #b45309; --warn-soft: #fbf0dd;
      --err: #dc2626; --err-soft: #fbe9e9; --info: #2563eb; --info-soft: #e7eefc;
      --radius: 12px; --shadow: 0 1px 2px rgba(20,25,40,.06), 0 6px 20px rgba(20,25,40,.05);
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #0e1017; --surface: #171a23; --surface-2: #1d2130; --border: #2a2f40;
        --text: #e7e9f0; --muted: #9298ad; --accent: #7c7cf0; --accent-hover: #9090f4;
        --accent-soft: #232544; --on-accent: #0e1017;
        --ok: #4ade80; --ok-soft: #16281d; --warn: #fbbf24; --warn-soft: #2e2410;
        --err: #f87171; --err-soft: #2e1616; --info: #60a5fa; --info-soft: #14203a;
        --shadow: 0 1px 2px rgba(0,0,0,.3), 0 8px 24px rgba(0,0,0,.35);
      }
    }
    * { box-sizing: border-box; }
    html { -webkit-text-size-adjust: 100%; }
    body {
      font-family: system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
      margin: 0; background: var(--bg); color: var(--text);
      font-size: 15px; line-height: 1.5;
    }

    .topbar {
      position: sticky; top: 0; z-index: 10;
      display: flex; align-items: center; gap: 1.5rem; flex-wrap: wrap;
      padding: .7rem 1.25rem;
      background: color-mix(in srgb, var(--surface) 88%, transparent);
      backdrop-filter: saturate(1.5) blur(8px);
      border-bottom: 1px solid var(--border);
    }
    .brand { display: flex; align-items: center; gap: .55rem; font-weight: 700; letter-spacing: -.02em; text-decoration: none; color: var(--text); }
    .brand .mark {
      display: grid; place-items: center; width: 30px; height: 30px; border-radius: 8px;
      background: linear-gradient(135deg, var(--accent), #8b5cf6); color: #fff;
      font-size: .85rem; font-weight: 800; box-shadow: var(--shadow);
    }
    .brand .name { font-size: 1.05rem; }
    nav { display: flex; gap: .25rem; flex-wrap: wrap; margin-left: auto; }
    nav a {
      text-decoration: none; color: var(--muted); font-weight: 600; font-size: .9rem;
      padding: .4rem .7rem; border-radius: 8px; transition: background .15s, color .15s;
    }
    nav a:hover { color: var(--text); background: var(--surface-2); }
    nav a.active { color: var(--accent); background: var(--accent-soft); }

    main { max-width: 1040px; margin: 1.75rem auto; padding: 0 1.25rem 3rem; }
    h1, h2 { letter-spacing: -.02em; }
    h2 { font-size: 1.15rem; margin: 2rem 0 .75rem; }
    main > h2:first-child, main > p:first-child { margin-top: .25rem; }
    p.meta { color: var(--muted); font-size: .9rem; }
    a { color: var(--accent); }

    .flash {
      display: flex; align-items: center; gap: .5rem;
      background: var(--ok-soft); border: 1px solid color-mix(in srgb, var(--ok) 35%, transparent);
      color: var(--text); padding: .7rem 1rem; border-radius: var(--radius); margin-bottom: 1.25rem;
      font-size: .92rem;
    }
    .flash::before { content: "\2713"; color: var(--ok); font-weight: 800; }
    .flash.warn { background: var(--warn-soft); border-color: color-mix(in srgb, var(--warn) 35%, transparent); }
    .flash.warn::before { content: "\26A0"; color: var(--warn); }

    /* Tables rendered as cards. */
    table {
      border-collapse: separate; border-spacing: 0; width: 100%; margin: .75rem 0 1.5rem;
      background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius);
      overflow: hidden; box-shadow: var(--shadow);
    }
    th, td { padding: .6rem .85rem; text-align: left; font-size: .88rem; vertical-align: top; }
    th {
      background: var(--surface-2); color: var(--muted); font-weight: 600;
      text-transform: uppercase; letter-spacing: .03em; font-size: .74rem;
      border-bottom: 1px solid var(--border);
    }
    td { border-top: 1px solid var(--border); }
    tr:first-child td { border-top: none; }
    tbody tr:hover td, table tr:hover td { background: var(--surface-2); }
    td:has(a), td:nth-child(4) { word-break: break-all; }

    .badge {
      display: inline-block; padding: .12rem .55rem; border-radius: 999px;
      font-size: .74rem; font-weight: 700; letter-spacing: .02em; text-transform: uppercase;
      background: var(--surface-2); color: var(--muted); border: 1px solid var(--border);
    }
    .badge[data-level="error" i], .badge[data-state="failed" i] { background: var(--err-soft); color: var(--err); border-color: transparent; }
    .badge[data-level="warn" i] { background: var(--warn-soft); color: var(--warn); border-color: transparent; }
    .badge[data-level="info" i], .badge[data-state="scrape" i] { background: var(--info-soft); color: var(--info); border-color: transparent; }
    .badge[data-state="done" i] { background: var(--ok-soft); color: var(--ok); border-color: transparent; }
    .badge[data-state^="review" i] { background: var(--warn-soft); color: var(--warn); border-color: transparent; }

    /* Forms. */
    .toolbar { display: flex; gap: .6rem; flex-wrap: wrap; align-items: center; margin: 1rem 0; }
    fieldset {
      border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface);
      padding: 1rem 1.25rem 1.25rem; margin: 0 0 1.5rem; box-shadow: var(--shadow);
    }
    legend { font-weight: 700; padding: 0 .4rem; letter-spacing: -.01em; }
    label { display: block; margin: .8rem 0 .25rem; font-weight: 600; font-size: .9rem; }
    small.hint { display: block; color: var(--muted); font-weight: 400; margin-top: .2rem; font-size: .82rem; }
    input[type=text], input[type=number], select {
      width: 100%; max-width: 480px; padding: .45rem .6rem; font-size: .9rem;
      color: var(--text); background: var(--surface); border: 1px solid var(--border);
      border-radius: 8px;
    }
    input[type=text]:focus, input[type=number]:focus, select:focus {
      outline: none; border-color: var(--accent);
      box-shadow: 0 0 0 3px var(--accent-soft);
    }
    input[type=checkbox] { width: 1.05rem; height: 1.05rem; accent-color: var(--accent); vertical-align: -2px; }

    button {
      font: inherit; font-weight: 600; font-size: .88rem; padding: .5rem 1rem;
      color: var(--on-accent); background: var(--accent); border: 1px solid transparent;
      border-radius: 8px; cursor: pointer; transition: background .15s;
    }
    button:hover { background: var(--accent-hover); }
    /* Secondary + destructive variants keyed off the form's action. */
    form[action="/rescan"] button, form[action="/review/retry"] button {
      background: var(--surface); color: var(--text); border-color: var(--border);
    }
    form[action="/rescan"] button:hover, form[action="/review/retry"] button:hover { background: var(--surface-2); }
    form[action="/review/delete"] button, form[action="/review/empty"] button {
      background: var(--surface); color: var(--err); border-color: color-mix(in srgb, var(--err) 40%, transparent);
    }
    form[action="/review/delete"] button:hover, form[action="/review/empty"] button:hover { background: var(--err-soft); }
    .row-actions form { display: inline; margin-right: .3rem; }
  </style>
</head>
<body>
  <header class="topbar">
    <a class="brand" href="/"><span class="mark">HS</span><span class="name">HappySorter</span></a>
    <nav>
      <a href="/"{{if eq .Path "/"}} class="active"{{end}}>Dashboard</a>
      <a href="/setup/folders"{{if eq .Path "/setup/folders"}} class="active"{{end}}>Folders</a>
      <a href="/setup/sources"{{if eq .Path "/setup/sources"}} class="active"{{end}}>Sources</a>
      <a href="/setup/rename"{{if eq .Path "/setup/rename"}} class="active"{{end}}>Rename</a>
      <a href="/review"{{if eq .Path "/review"}} class="active"{{end}}>Review</a>
      <a href="/logs"{{if eq .Path "/logs"}} class="active"{{end}}>Logs</a>
    </nav>
  </header>
  <main>
    {{if .Flash}}<p class="flash{{if .Warn}} warn{{end}}">{{.Flash}}</p>{{end}}
    {{.Body}}
  </main>
</body>
</html>
`))

type pageData struct {
	Title string
	Path  string
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
		Path:  r.URL.Path,
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
