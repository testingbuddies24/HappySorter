package store

import (
	"database/sql"
	"time"
)

// LogRecord is one row of the `logs` table.
type LogRecord struct {
	ID      int64
	Level   string
	Message string
	Fields  string // raw JSON
	Time    time.Time
}

type LogStore struct {
	db *sql.DB
}

func NewLogStore(db *sql.DB) *LogStore {
	return &LogStore{db: db}
}

// Tail returns the most recent limit log entries, newest first, optionally
// filtered to a single level ("" means all levels).
func (s *LogStore) Tail(limit int, level string) ([]LogRecord, error) {
	var rows *sql.Rows
	var err error
	if level == "" {
		rows, err = s.db.Query(
			`SELECT id, level, message, fields, ts FROM logs ORDER BY id DESC LIMIT ?`, limit,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, level, message, fields, ts FROM logs WHERE level = ? ORDER BY id DESC LIMIT ?`, level, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LogRecord
	for rows.Next() {
		var r LogRecord
		if err := rows.Scan(&r.ID, &r.Level, &r.Message, &r.Fields, &r.Time); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Query already orders newest-first (ORDER BY id DESC), which is how the
	// log viewer displays them — most recent at the top.
	return out, nil
}
