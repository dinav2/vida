// Package clipboard manages clipboard history storage for vida (SPEC-20260318-011).
package clipboard

import (
	"database/sql"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/dinav2/vida/internal/db"
)

// Entry is a single clipboard history item.
type Entry struct {
	ID       int64
	Content  string
	CopiedAt time.Time
	Pinned   bool
}

// Config is the clipboard configuration subset passed to Store operations.
type Config struct {
	Enabled    bool
	MaxEntries int
	MaxAgeDays int
	Ignore     []string // Go regex patterns
}

// Store manages clipboard history in the vida SQLite database.
type Store struct {
	conn *sql.DB
	now  func() time.Time
}

// NewStore returns a Store backed by the given database using the real clock.
func NewStore(database *db.DB) *Store {
	return &Store{conn: database.Conn(), now: time.Now}
}

// NewStoreWithClock returns a Store with an injectable clock (for testing).
func NewStoreWithClock(database *db.DB, now func() time.Time) *Store {
	return &Store{conn: database.Conn(), now: now}
}

// Add stores content as a new clipboard entry, applying dedup, ignore patterns,
// and pruning. Returns (true, nil) if stored, (false, nil) if discarded.
func (s *Store) Add(content string, cfg Config) (bool, error) {
	// Check ignore patterns (AC-06: invalid patterns are skipped with a warning).
	for _, pattern := range cfg.Ignore {
		re, err := regexp.Compile(pattern)
		if err != nil {
			log.Printf("clipboard: invalid ignore pattern %q: %v", pattern, err)
			continue
		}
		if re.MatchString(content) {
			return false, nil
		}
	}

	// Dedup: skip if identical to the most recent entry (FR-01c).
	var last string
	row := s.conn.QueryRow(`SELECT content FROM clipboard_history ORDER BY copied_at DESC LIMIT 1`)
	if err := row.Scan(&last); err == nil && last == content {
		return false, nil
	}

	now := s.now().UnixNano()
	_, err := s.conn.Exec(
		`INSERT INTO clipboard_history (content, copied_at) VALUES (?, ?)`,
		content, now,
	)
	if err != nil {
		return false, err
	}

	if err := s.prune(cfg); err != nil {
		return true, err
	}
	return true, nil
}

// prune deletes entries that exceed MaxEntries or MaxAgeDays (pinned exempt).
func (s *Store) prune(cfg Config) error {
	// Count-based pruning (FR-02c).
	if cfg.MaxEntries > 0 {
		_, err := s.conn.Exec(`
			DELETE FROM clipboard_history
			WHERE pinned = 0
			  AND id NOT IN (
			    SELECT id FROM clipboard_history
			    WHERE pinned = 0
			    ORDER BY copied_at DESC
			    LIMIT ?
			  )`, cfg.MaxEntries)
		if err != nil {
			return err
		}
	}

	// Age-based pruning (FR-02d).
	if cfg.MaxAgeDays > 0 {
		cutoff := s.now().AddDate(0, 0, -cfg.MaxAgeDays).UnixNano()
		_, err := s.conn.Exec(
			`DELETE FROM clipboard_history WHERE pinned = 0 AND copied_at < ?`,
			cutoff,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// List returns entries matching query (case-insensitive substring), newest first.
// An empty query returns all entries up to limit.
func (s *Store) List(query string, limit int) ([]Entry, error) {
	var rows *sql.Rows
	var err error

	if query == "" {
		rows, err = s.conn.Query(
			`SELECT id, content, copied_at, pinned
			 FROM clipboard_history
			 ORDER BY copied_at DESC
			 LIMIT ?`, limit)
	} else {
		rows, err = s.conn.Query(
			`SELECT id, content, copied_at, pinned
			 FROM clipboard_history
			 WHERE LOWER(content) LIKE ?
			 ORDER BY copied_at DESC
			 LIMIT ?`,
			"%"+strings.ToLower(query)+"%", limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var ts int64
		var pinned int
		if err := rows.Scan(&e.ID, &e.Content, &ts, &pinned); err != nil {
			return nil, err
		}
		e.CopiedAt = time.Unix(0, ts)
		e.Pinned = pinned == 1
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Delete removes entry by ID. Nonexistent ID is a no-op.
func (s *Store) Delete(id int64) error {
	_, err := s.conn.Exec(`DELETE FROM clipboard_history WHERE id = ?`, id)
	return err
}

// TogglePin flips the pinned flag for the given entry ID.
func (s *Store) TogglePin(id int64) error {
	_, err := s.conn.Exec(
		`UPDATE clipboard_history SET pinned = CASE WHEN pinned = 1 THEN 0 ELSE 1 END WHERE id = ?`,
		id,
	)
	return err
}

// Clear removes all unpinned entries.
func (s *Store) Clear() error {
	_, err := s.conn.Exec(`DELETE FROM clipboard_history WHERE pinned = 0`)
	return err
}
