// Package store wraps the `files` table: the record of every file
// HappySorter has ever seen, used to make watcher + pipeline processing
// idempotent across restarts (docs/ARCHITECTURE.md § 2.2, § 3).
package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// FileState mirrors the `files.state` values documented in
// docs/ARCHITECTURE.md § 3.
type FileState string

const (
	StateDetected        FileState = "detected"
	StateReviewFilter    FileState = "review_filter"
	StateReviewUnmatched FileState = "review_unmatched"
	StateReviewDuplicate FileState = "review_duplicate"
	StateScrape          FileState = "scrape"
	StateOrganise        FileState = "organise"
	StateDone            FileState = "done"
	StateFailed          FileState = "failed"
)

type FileStore struct {
	db *sql.DB
}

func NewFileStore(db *sql.DB) *FileStore {
	return &FileStore{db: db}
}

// Seen reports whether source_path has already been recorded, in any state.
// This is what makes re-running the watcher over the same folder (restart,
// periodic poll rescan) safe: already-processed paths are skipped.
func (s *FileStore) Seen(sourcePath string) (bool, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM files WHERE source_path = ?`, sourcePath).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Record inserts a row for a file the pipeline has just finished handling.
func (s *FileStore) Record(sourcePath, currentPath string, state FileState, code, reason string) error {
	now := time.Now()
	_, err := s.db.Exec(
		`INSERT INTO files (source_path, current_path, state, code, reason, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sourcePath, currentPath, string(state), nullIfEmpty(code), nullIfEmpty(reason), now, now,
	)
	return err
}

// CountByState returns how many files currently sit in the given state —
// used to report queue_size on /healthz.
func (s *FileStore) CountByState(state FileState) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM files WHERE state = ?`, string(state)).Scan(&n)
	return n, err
}

// FileRecord is one row of the `files` table, for the GUI's review/history
// views.
type FileRecord struct {
	ID          int64
	SourcePath  string
	CurrentPath string
	State       FileState
	Code        string
	Reason      string
	UpdatedAt   time.Time
}

// ListByStates returns files in any of the given states, most-recently
// updated first — backs the /review page's per-folder tabs.
func (s *FileStore) ListByStates(states ...FileState) ([]FileRecord, error) {
	placeholders := make([]string, len(states))
	args := make([]any, len(states))
	for i, st := range states {
		placeholders[i] = "?"
		args[i] = string(st)
	}
	query := fmt.Sprintf(
		`SELECT id, source_path, COALESCE(current_path, ''), state, COALESCE(code, ''), COALESCE(reason, ''), updated_at
		 FROM files WHERE state IN (%s) ORDER BY updated_at DESC`,
		strings.Join(placeholders, ","),
	)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFileRecords(rows)
}

// ListRecent returns the most recently updated files across all states —
// backs the dashboard's "recent activity" panel.
func (s *FileStore) ListRecent(limit int) ([]FileRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, source_path, COALESCE(current_path, ''), state, COALESCE(code, ''), COALESCE(reason, ''), updated_at
		 FROM files ORDER BY updated_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFileRecords(rows)
}

func scanFileRecords(rows *sql.Rows) ([]FileRecord, error) {
	var out []FileRecord
	for rows.Next() {
		var r FileRecord
		var state string
		if err := rows.Scan(&r.ID, &r.SourcePath, &r.CurrentPath, &state, &r.Code, &r.Reason, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.State = FileState(state)
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetByID looks up a single file record, for review retry/delete actions.
func (s *FileStore) GetByID(id int64) (*FileRecord, error) {
	var r FileRecord
	var state string
	err := s.db.QueryRow(
		`SELECT id, source_path, COALESCE(current_path, ''), state, COALESCE(code, ''), COALESCE(reason, ''), updated_at
		 FROM files WHERE id = ?`, id,
	).Scan(&r.ID, &r.SourcePath, &r.CurrentPath, &state, &r.Code, &r.Reason, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	r.State = FileState(state)
	return &r, nil
}

// Delete removes a file's row (the caller is responsible for removing the
// underlying file on disk, if desired).
func (s *FileStore) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM files WHERE id = ?`, id)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
