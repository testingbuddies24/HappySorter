// Package logging configures HappySorter's structured logger: JSON to
// stdout plus a fan-out into the `logs` SQLite table backing the GUI's log
// viewer (docs/ARCHITECTURE.md § 2.10, § 11).
package logging

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
)

// New builds a slog.Logger at the given level ("debug", "info", "warn",
// "error"; defaults to info). If db is non-nil, records are also persisted
// to the `logs` table.
func New(level string, db *sql.DB) *slog.Logger {
	lvl := parseLevel(level)
	handlers := []slog.Handler{
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}),
	}
	if db != nil {
		handlers = append(handlers, &dbHandler{db: db, level: lvl})
	}
	return slog.New(&multiHandler{handlers: handlers})
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// multiHandler fans out log records to multiple slog.Handlers.
type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		if err := h.Handle(ctx, r.Clone()); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: next}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: next}
}

// dbHandler persists log records into the `logs` table.
type dbHandler struct {
	db       *sql.DB
	level    slog.Level
	preAttrs []slog.Attr
}

func (h *dbHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *dbHandler) Handle(ctx context.Context, r slog.Record) error {
	fields := make(map[string]any, len(h.preAttrs)+r.NumAttrs())
	for _, a := range h.preAttrs {
		fields[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.Any()
		return true
	})

	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		fieldsJSON = []byte("{}")
	}

	_, err = h.db.ExecContext(ctx,
		`INSERT INTO logs (level, message, fields, ts) VALUES (?, ?, ?, ?)`,
		r.Level.String(), r.Message, string(fieldsJSON), r.Time,
	)
	return err
}

func (h *dbHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(h.preAttrs)+len(attrs))
	next = append(next, h.preAttrs...)
	next = append(next, attrs...)
	return &dbHandler{db: h.db, level: h.level, preAttrs: next}
}

func (h *dbHandler) WithGroup(_ string) slog.Handler { return h }
