package httpserver

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/testingbuddies24/HappySorter/internal/config"
	"github.com/testingbuddies24/HappySorter/internal/store"
)

var reviewTmpl = template.Must(template.New("review").Parse(`
<p>Files here were not organised automatically. Fix the underlying issue (rename the file, remove a duplicate, etc.),
then click Retry — or Delete to discard the file and its record.</p>

<form method="post" action="/tbc/refresh" style="margin-bottom:1rem">
  <button type="submit">Refresh from disk</button>
</form>

{{range .Sections}}
<h2>{{.Title}}</h2>
{{if .Emptyable}}
<form method="post" action="/tbc/empty" onsubmit="return confirm('Delete ALL junk files from disk? This cannot be undone.');" style="margin-bottom:.5rem">
  <input type="hidden" name="state" value="{{.State}}">
  <button type="submit">Delete all junk</button>
</form>
{{end}}
<table>
  <tr><th>Updated</th><th>Code</th><th>Path</th><th>Reason</th><th>Actions</th></tr>
  {{range .Files}}
  <tr>
    <td>{{.UpdatedAt.Format "2006-01-02 15:04:05"}}</td>
    <td>{{.Code}}</td>
    <td>{{.CurrentPath}}</td>
    <td>{{.Reason}}</td>
    <td class="row-actions">
      <form method="post" action="/tbc/retry"><input type="hidden" name="id" value="{{.ID}}"><button type="submit">Retry</button></form>
      <form method="post" action="/tbc/delete" onsubmit="return confirm('Delete this file from disk?');"><input type="hidden" name="id" value="{{.ID}}"><button type="submit">Delete</button></form>
    </td>
  </tr>
  {{else}}
  <tr><td colspan="5">Nothing here.</td></tr>
  {{end}}
</table>
{{end}}
`))

type reviewSection struct {
	Title     string
	State     store.FileState
	Emptyable bool
	Files     []store.FileRecord
}

func (s *Server) handleReviewGet(w http.ResponseWriter, r *http.Request) {
	sections := []reviewSection{
		{Title: "Filtered (rejected as junk/sample)", State: store.StateReviewFilter, Emptyable: true},
		{Title: "Unmatched (no JAV code found)", State: store.StateReviewUnmatched},
		{Title: "Duplicate (destination already exists)", State: store.StateReviewDuplicate},
		{Title: "Failed (scrape or organise error)", State: store.StateFailed},
	}
	for i, sec := range sections {
		files, err := s.fileStore.ListByStates(sec.State)
		if err != nil {
			s.logger.Error("listing review files", "state", sec.State, "error", err)
		}
		sections[i].Files = files
	}

	var buf bytes.Buffer
	if err := reviewTmpl.Execute(&buf, struct{ Sections []reviewSection }{sections}); err != nil {
		s.logger.Error("rendering review page", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.render(w, r, "TBC", template.HTML(buf.String()))
}

func (s *Server) handleReviewRetry(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	rec, err := s.fileStore.GetByID(id)
	if err != nil {
		s.logger.Error("looking up review file", "id", id, "error", err)
		redirectFlash(w, r, "/tbc", "Could not find that file.", true)
		return
	}

	if _, statErr := os.Stat(rec.CurrentPath); statErr != nil {
		redirectFlash(w, r, "/tbc", "File not found at "+rec.CurrentPath+" — if you renamed or moved it, rename it back first, then retry.", true)
		return
	}

	if err := s.fileStore.Delete(id); err != nil {
		s.logger.Error("clearing stale review record", "id", id, "error", err)
	}
	s.pipeline.Retry(r.Context(), rec.CurrentPath)

	redirectFlash(w, r, "/tbc", "Retried "+rec.CurrentPath+".", false)
}

func (s *Server) handleReviewDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	rec, err := s.fileStore.GetByID(id)
	if err != nil {
		s.logger.Error("looking up review file", "id", id, "error", err)
		redirectFlash(w, r, "/tbc", "Could not find that file.", true)
		return
	}

	if err := os.Remove(rec.CurrentPath); err != nil && !os.IsNotExist(err) {
		s.logger.Error("deleting review file from disk", "path", rec.CurrentPath, "error", err)
		redirectFlash(w, r, "/tbc", "Failed to delete file from disk: "+err.Error(), true)
		return
	}
	if err := s.fileStore.Delete(id); err != nil {
		s.logger.Error("deleting review record", "id", id, "error", err)
	}

	redirectFlash(w, r, "/tbc", "Deleted "+rec.CurrentPath+".", false)
}

// reviewDirFor maps a review state to its on-disk TBC folder.
func reviewDirFor(cfg *config.Config, state store.FileState) (string, bool) {
	switch state {
	case store.StateReviewFilter:
		return cfg.Paths.ReviewFilter, true
	case store.StateReviewUnmatched:
		return cfg.Paths.ReviewUnmatched, true
	case store.StateReviewDuplicate:
		return cfg.Paths.ReviewDuplicate, true
	default:
		return "", false
	}
}

// reviewStates are every state that keeps a file physically parked in one of
// the TBC folders. StateFailed is included because failed files are routed
// into the _unmatched directory, not their own folder.
var reviewStates = []store.FileState{
	store.StateReviewFilter,
	store.StateReviewUnmatched,
	store.StateReviewDuplicate,
	store.StateFailed,
}

// handleReviewRefresh reconciles the `files` table's review-state rows with
// what is actually sitting in the TBC folders on disk — rows for files the
// user renamed or moved away are dropped, and files the user renamed or
// dropped in that have no row yet are adopted, so /tbc always reflects the
// current disk state instead of trusting stale current_path values.
func (s *Server) handleReviewRefresh(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfgStore.Get()
	removed, added := 0, 0
	var warnings []string

	rows, err := s.fileStore.ListByStates(reviewStates...)
	if err != nil {
		s.logger.Error("listing review files to refresh", "error", err)
		redirectFlash(w, r, "/tbc", "Refresh failed: "+err.Error(), true)
		return
	}
	tracked := make(map[string]bool, len(rows))
	for _, rec := range rows {
		if _, statErr := os.Stat(rec.CurrentPath); statErr != nil {
			if !os.IsNotExist(statErr) {
				warnings = append(warnings, rec.CurrentPath+": "+statErr.Error())
			}
			if err := s.fileStore.Delete(rec.ID); err != nil {
				s.logger.Error("deleting stale review record", "id", rec.ID, "error", err)
			}
			removed++
			continue
		}
		tracked[rec.CurrentPath] = true
	}

	for _, state := range []store.FileState{store.StateReviewFilter, store.StateReviewUnmatched, store.StateReviewDuplicate} {
		dir, _ := reviewDirFor(cfg, state)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				s.logger.Error("reading review dir during refresh", "dir", dir, "error", err)
				warnings = append(warnings, dir+": "+err.Error())
			}
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if tracked[path] {
				continue
			}
			if err := s.fileStore.Record(path, path, state, "", "found on disk by refresh"); err != nil {
				s.logger.Error("recording file found on disk", "path", path, "error", err)
				continue
			}
			tracked[path] = true
			added++
		}
	}

	msg := fmt.Sprintf("Refresh complete: %d stale record(s) removed, %d file(s) found on disk.", removed, added)
	if len(warnings) > 0 {
		msg += " Warnings: " + strings.Join(warnings, "; ")
	}
	redirectFlash(w, r, "/tbc", msg, len(warnings) > 0)
}

// emptyableStates are the only states /tbc/empty may bulk-delete from —
// anything else (e.g. "done") gets rejected rather than mass-deleting
// organised files on an unexpected form value.
var emptyableStates = map[store.FileState]bool{
	store.StateReviewFilter:    true,
	store.StateReviewUnmatched: true,
	store.StateReviewDuplicate: true,
}

// handleReviewEmpty bulk-deletes every file sitting in a review state's TBC
// folder on disk (not just the ones with a matching database row — an
// untracked file left behind by a previous partial delete would otherwise
// survive forever), plus the folder's database rows. The confirmation
// prompt lives client-side on the form.
func (s *Server) handleReviewEmpty(w http.ResponseWriter, r *http.Request) {
	state := store.FileState(r.FormValue("state"))
	if !emptyableStates[state] {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	cfg := s.cfgStore.Get()
	dir, ok := reviewDirFor(cfg, state)
	// Guard against config.yaml being hand-edited so a review path collapses
	// onto the library or watch folder — this handler deletes every file it
	// finds in dir, so that misconfiguration must never reach the ReadDir
	// loop below.
	if !ok || dir == "" || dir == cfg.Paths.Library || dir == cfg.Paths.Watch {
		s.logger.Error("refusing to empty unsafe review dir", "state", state, "dir", dir)
		redirectFlash(w, r, "/tbc", "Refusing to empty: review path is misconfigured.", true)
		return
	}

	// Files tracked under a different state (e.g. a failed file physically
	// parked in the _unmatched folder) must not be deleted here.
	otherTracked := map[string]bool{}
	for _, other := range reviewStates {
		if other == state {
			continue
		}
		rows, err := s.fileStore.ListByStates(other)
		if err != nil {
			s.logger.Error("listing files for empty exclusion", "state", other, "error", err)
			continue
		}
		for _, rec := range rows {
			otherTracked[rec.CurrentPath] = true
		}
	}

	deleted, skippedDirs := 0, 0
	var failures []string

	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		s.logger.Error("reading review dir to empty", "dir", dir, "error", err)
		redirectFlash(w, r, "/tbc", "Failed to list files: "+err.Error(), true)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			skippedDirs++
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if otherTracked[path] {
			continue
		}
		if err := os.Remove(path); err != nil {
			s.logger.Error("deleting file during bulk empty", "path", path, "error", err)
			failures = append(failures, entry.Name()+": "+err.Error())
			continue
		}
		deleted++
	}

	rows, err := s.fileStore.ListByStates(state)
	if err != nil {
		s.logger.Error("listing records to prune after empty", "state", state, "error", err)
	}
	for _, rec := range rows {
		if _, statErr := os.Stat(rec.CurrentPath); statErr != nil && os.IsNotExist(statErr) {
			if err := s.fileStore.Delete(rec.ID); err != nil {
				s.logger.Error("deleting record during bulk empty", "id", rec.ID, "error", err)
			}
		}
	}

	msg := fmt.Sprintf("Deleted %d file(s).", deleted)
	if skippedDirs > 0 {
		msg += fmt.Sprintf(" Skipped %d subdirectory(ies).", skippedDirs)
	}
	if len(failures) > 0 {
		shown := failures
		if len(shown) > 3 {
			shown = shown[:3]
		}
		msg = fmt.Sprintf("Deleted %d file(s), %d failed: %s", deleted, len(failures), strings.Join(shown, "; "))
		if len(failures) > 3 {
			msg += "…"
		}
	}
	redirectFlash(w, r, "/tbc", msg, len(failures) > 0)
}
