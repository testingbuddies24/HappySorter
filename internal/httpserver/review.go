package httpserver

import (
	"bytes"
	"html/template"
	"net/http"
	"os"
	"strconv"

	"github.com/testingbuddies24/HappySorter/internal/store"
)

var reviewTmpl = template.Must(template.New("review").Parse(`
<p>Files here were not organised automatically. Fix the underlying issue (rename the file, remove a duplicate, etc.),
then click Retry — or Delete to discard the file and its record.</p>

{{range .Sections}}
<h2>{{.Title}}</h2>
{{if eq .Title "Filtered (rejected as junk/sample)"}}
<form method="post" action="/tbc/empty" onsubmit="return confirm('Delete ALL junk files from disk? This cannot be undone.');" style="margin-bottom:.5rem">
  <input type="hidden" name="state" value="review_filter">
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
	Title string
	Files []store.FileRecord
}

func (s *Server) handleReviewGet(w http.ResponseWriter, r *http.Request) {
	sections := []reviewSection{
		{Title: "Filtered (rejected as junk/sample)"},
		{Title: "Unmatched (no JAV code found)"},
		{Title: "Duplicate (destination already exists)"},
		{Title: "Failed (scrape or organise error)"},
	}
	stateGroups := [][]store.FileState{
		{store.StateReviewFilter},
		{store.StateReviewUnmatched},
		{store.StateReviewDuplicate},
		{store.StateFailed},
	}
	for i, states := range stateGroups {
		files, err := s.fileStore.ListByStates(states...)
		if err != nil {
			s.logger.Error("listing review files", "states", states, "error", err)
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

// handleReviewEmpty bulk-deletes every file (disk + record) in a given
// review state — the confirmation prompt lives client-side on the form.
func (s *Server) handleReviewEmpty(w http.ResponseWriter, r *http.Request) {
	state := store.FileState(r.FormValue("state"))
	files, err := s.fileStore.ListByStates(state)
	if err != nil {
		s.logger.Error("listing files to empty", "state", state, "error", err)
		redirectFlash(w, r, "/tbc", "Failed to list files: "+err.Error(), true)
		return
	}

	for _, rec := range files {
		if err := os.Remove(rec.CurrentPath); err != nil && !os.IsNotExist(err) {
			s.logger.Error("deleting file during bulk empty", "path", rec.CurrentPath, "error", err)
			continue
		}
		if err := s.fileStore.Delete(rec.ID); err != nil {
			s.logger.Error("deleting record during bulk empty", "id", rec.ID, "error", err)
		}
	}

	redirectFlash(w, r, "/tbc", "Cleared.", false)
}
