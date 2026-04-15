// Package db implements SQLite-backed query history storage for vida.
// All database access is serialised through a single goroutine to avoid
// SQLITE_BUSY errors (TR-07c).
package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const maxHistory = 500

// Query is a single history entry.
type Query struct {
	ID            int64
	Input         string
	ResultKind    string
	ResultPreview string
	CreatedAt     time.Time
}

// DB wraps a SQLite connection with vida-specific operations.
type DB struct {
	conn *sql.DB
}

// Open opens (or creates) the SQLite database at path, runs migrations,
// and enables WAL mode. Use ":memory:" for tests.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("db open: %w", err)
	}

	// Single connection to serialise all access (TR-07c).
	conn.SetMaxOpenConns(1)

	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		conn.Close()
		return nil, err
	}
	return d, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

// Conn returns the underlying *sql.DB for packages that manage their own queries.
func (d *DB) Conn() *sql.DB {
	return d.conn
}

// migrate applies schema and pragma setup.
func (d *DB) migrate() error {
	stmts := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA user_version`, // read — actual migration gating done below
		`CREATE TABLE IF NOT EXISTS queries (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			input          TEXT    NOT NULL,
			result_kind    TEXT    NOT NULL DEFAULT '',
			result_preview TEXT    NOT NULL DEFAULT '',
			created_at     INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS clipboard_history (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			content   TEXT    NOT NULL,
			copied_at INTEGER NOT NULL,
			pinned    INTEGER NOT NULL DEFAULT 0
		)`,
	}
	for _, s := range stmts {
		if _, err := d.conn.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// AddQuery records a completed query in history and prunes oldest rows
// when the cap is exceeded (FR-09c, FR-09d).
func (d *DB) AddQuery(input, resultKind, resultPreview string) error {
	now := time.Now().UnixNano()
	_, err := d.conn.Exec(
		`INSERT INTO queries (input, result_kind, result_preview, created_at) VALUES (?, ?, ?, ?)`,
		input, resultKind, resultPreview, now,
	)
	if err != nil {
		return err
	}
	// Prune oldest rows beyond the cap.
	_, err = d.conn.Exec(
		`DELETE FROM queries WHERE id NOT IN (
			SELECT id FROM queries ORDER BY created_at DESC LIMIT ?
		)`,
		maxHistory,
	)
	return err
}

// RecentQueries returns up to limit history entries, most recent first (FR-09e).
func (d *DB) RecentQueries(limit int) ([]Query, error) {
	rows, err := d.conn.Query(
		`SELECT id, input, result_kind, result_preview, created_at
		 FROM queries ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Query
	for rows.Next() {
		var q Query
		var ts int64
		if err := rows.Scan(&q.ID, &q.Input, &q.ResultKind, &q.ResultPreview, &ts); err != nil {
			return nil, err
		}
		q.CreatedAt = time.Unix(0, ts)
		results = append(results, q)
	}
	return results, rows.Err()
}

// ClearHistory truncates the queries table (FR-09, AC-R3).
func (d *DB) ClearHistory() error {
	_, err := d.conn.Exec(`DELETE FROM queries`)
	return err
}

// JournalMode returns the current SQLite journal mode (used in tests to verify WAL).
func (d *DB) JournalMode() (string, error) {
	var mode string
	err := d.conn.QueryRow(`PRAGMA journal_mode`).Scan(&mode)
	return mode, err
}
