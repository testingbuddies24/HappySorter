package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/testingbuddies24/HappySorter/internal/database"
)

func TestLogStoreSinceExcludesOldEntries(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now()
	old := now.Add(-2 * time.Hour)
	insert := `INSERT INTO logs (level, message, fields, ts) VALUES (?, ?, ?, ?)`
	if _, err := db.Exec(insert, "INFO", "recent event", "{}", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(insert, "INFO", "old event", "{}", old); err != nil {
		t.Fatal(err)
	}

	s := NewLogStore(db)
	records, err := s.Since(now.Add(-time.Hour), 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1: %+v", len(records), records)
	}
	if records[0].Message != "recent event" {
		t.Errorf("got message %q, want %q", records[0].Message, "recent event")
	}
}
