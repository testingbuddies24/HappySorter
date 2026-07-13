// Package store wraps the `files` table: the record of every file
// HappySorter has ever seen, used to make watcher + pipeline processing
// idempotent across restarts (docs/ARCHITECTURE.md § 2.2, § 3).
package store

import (
	"database/sql"
	"time"
)

// FileState mirrors the `files.state` values documented in
// docs/ARCHITECTURE.md § 3.
type FileState string

const (
	StateDetected        FileState = "detected"
	StateReviewFilter    FileState = "review_filter"
	StateReviewUnmatched FileState = "review_unmatched"
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

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
