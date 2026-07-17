package httpserver

import (
	"bytes"
	"context"
	"html/template"
	"net/http"
	"sort"
	"strconv"

	"github.com/testingbuddies24/HappySorter/internal/config"
	"github.com/testingbuddies24/HappySorter/internal/scraper"
	"github.com/testingbuddies24/HappySorter/internal/scraper/registry"
)

// ---- /setup/folders ----

var foldersTmpl = template.Must(template.New("folders").Parse(`
<form method="post" action="/setup/folders">
  <fieldset>
    <legend>Folders</legend>
    <label for="watch">Watch folder</label>
    <input type="text" id="watch" name="watch" value="{{.Watch}}">
    <small class="hint">Changing this requires a container/process restart to take effect.</small>

    <label for="library">Library folder</label>
    <input type="text" id="library" name="library" value="{{.Library}}">

    <label for="review_filter">Review: filtered</label>
    <input type="text" id="review_filter" name="review_filter" value="{{.ReviewFilter}}">

    <label for="review_unmatched">Review: unmatched</label>
    <input type="text" id="review_unmatched" name="review_unmatched" value="{{.ReviewUnmatched}}">

    <label for="review_duplicate">Review: duplicate</label>
    <input type="text" id="review_duplicate" name="review_duplicate" value="{{.ReviewDuplicate}}">

    <button type="submit">Save</button>
  </fieldset>
</form>
`))

func (s *Server) handleFoldersGet(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfgStore.Get()
	var buf bytes.Buffer
	if err := foldersTmpl.Execute(&buf, cfg.Paths); err != nil {
		s.logger.Error("rendering folders form", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.render(w, r, "Folders", template.HTML(buf.String()))
}

func (s *Server) handleFoldersPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	watchChanged := false
	err := s.cfgStore.Update(func(next *config.Config) {
		if r.FormValue("watch") != next.Paths.Watch {
			watchChanged = true
		}
		next.Paths.Watch = r.FormValue("watch")
		next.Paths.Library = r.FormValue("library")
		next.Paths.ReviewFilter = r.FormValue("review_filter")
		next.Paths.ReviewUnmatched = r.FormValue("review_unmatched")
		next.Paths.ReviewDuplicate = r.FormValue("review_duplicate")
	})
	if err != nil {
		s.logger.Error("saving folders config", "error", err)
		redirectFlash(w, r, "/setup/folders", "Failed to save: "+err.Error(), true)
		return
	}

	if watchChanged {
		redirectFlash(w, r, "/setup/folders", "Saved. The watch folder change needs a restart to take effect.", true)
		return
	}
	redirectFlash(w, r, "/setup/folders", "Saved.", false)
}

// ---- /setup/sources ----

var sourcesTmpl = template.Must(template.New("sources").Parse(`
<form method="post" action="/setup/sources">
  <fieldset>
    <legend>Sources (tried in priority order, lowest first, until one succeeds)</legend>
    <table>
      <tr><th>Enabled</th><th>Name</th><th>Priority</th><th>QPS</th></tr>
      {{range .}}
      <tr>
        <td><input type="checkbox" name="enabled_{{.Name}}" {{if .Enabled}}checked{{end}}></td>
        <td>{{.Name}}</td>
        <td><input type="number" name="priority_{{.Name}}" value="{{.Priority}}" style="width:5rem"></td>
        <td><input type="number" step="0.1" name="qps_{{.Name}}" value="{{.QPS}}" style="width:5rem"></td>
      </tr>
      {{end}}
    </table>
    <button type="submit">Save</button>
  </fieldset>
</form>
`))

func (s *Server) handleSourcesGet(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfgStore.Get()
	sorted := append([]config.SourceConfig(nil), cfg.Sources...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Priority < sorted[j].Priority })

	var buf bytes.Buffer
	if err := sourcesTmpl.Execute(&buf, sorted); err != nil {
		s.logger.Error("rendering sources form", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.render(w, r, "Sources", template.HTML(buf.String()))
}

func (s *Server) handleSourcesPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	var updated []config.SourceConfig
	err := s.cfgStore.Update(func(next *config.Config) {
		updated = make([]config.SourceConfig, len(next.Sources))
		for i, sc := range next.Sources {
			sc.Enabled = r.FormValue("enabled_"+sc.Name) == "on"
			if v, err := strconv.Atoi(r.FormValue("priority_" + sc.Name)); err == nil {
				sc.Priority = v
			}
			if v, err := strconv.ParseFloat(r.FormValue("qps_"+sc.Name), 64); err == nil {
				sc.QPS = v
			}
			updated[i] = sc
		}
		next.Sources = updated
	})
	if err != nil {
		s.logger.Error("saving sources config", "error", err)
		redirectFlash(w, r, "/setup/sources", "Failed to save: "+err.Error(), true)
		return
	}

	adapters := registry.BuildAdapters(updated, s.httpClient, s.logger)
	manager := scraper.NewManager(s.logger, adapters...)
	s.managerStore.Set(manager)

	if !manager.Empty() {
		// context.Background(), not r.Context(): the request context is
		// cancelled as soon as this handler returns (below), which would
		// otherwise kill the scrapes mid-flight.
		go s.pipeline.DrainQueued(context.Background())
	}

	redirectFlash(w, r, "/setup/sources", "Saved and applied — no restart needed.", false)
}

// ---- /setup/rename ----

var renameTmpl = template.Must(template.New("rename").Parse(`
<form method="post" action="/setup/rename">
  <fieldset>
    <legend>Rename templates</legend>
    <label for="folder_template">Folder template</label>
    <input type="text" id="folder_template" name="folder_template" value="{{.FolderTemplate}}">

    <label for="file_template">File template</label>
    <input type="text" id="file_template" name="file_template" value="{{.FileTemplate}}">

    <label for="unknown_placeholder">Unknown-year placeholder</label>
    <input type="text" id="unknown_placeholder" name="unknown_placeholder" value="{{.UnknownPlaceholder}}">

    <small class="hint">Available tokens: {code}, {year}</small>
    <button type="submit">Save</button>
  </fieldset>
</form>
`))

func (s *Server) handleRenameGet(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfgStore.Get()
	var buf bytes.Buffer
	if err := renameTmpl.Execute(&buf, cfg.Rename); err != nil {
		s.logger.Error("rendering rename form", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.render(w, r, "Rename", template.HTML(buf.String()))
}

func (s *Server) handleRenamePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	err := s.cfgStore.Update(func(next *config.Config) {
		next.Rename.FolderTemplate = r.FormValue("folder_template")
		next.Rename.FileTemplate = r.FormValue("file_template")
		next.Rename.UnknownPlaceholder = r.FormValue("unknown_placeholder")
	})
	if err != nil {
		s.logger.Error("saving rename config", "error", err)
		redirectFlash(w, r, "/setup/rename", "Failed to save: "+err.Error(), true)
		return
	}
	redirectFlash(w, r, "/setup/rename", "Saved.", false)
}
