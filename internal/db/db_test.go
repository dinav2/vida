// Package db implements SQLite-backed history and preference storage.
// Tests cover SCN-16, SCN-17, FR-09.
package db_test

import (
	"testing"
	"time"

	"github.com/dinav2/vida/internal/db"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// FR-09b: schema is created on open
func TestDB_Schema(t *testing.T) {
	d := openTestDB(t)
	// Insert and retrieve to confirm schema
	err := d.AddQuery("test input", "calc", "42")
	if err != nil {
		t.Fatalf("AddQuery: %v", err)
	}
}

// FR-09c: every completed query is written to history
func TestDB_AddQuery(t *testing.T) {
	d := openTestDB(t)

	entries := []struct {
		input   string
		kind    string
		preview string
	}{
		{"42 * 1.5", "calc", "63"},
		{"g linux kernel", "shortcut", "https://www.google.com/..."},
		{"firefox", "app_launch", "Firefox Web Browser"},
		{"explain inode", "ai", "An inode is..."},
	}

	for _, e := range entries {
		if err := d.AddQuery(e.input, e.kind, e.preview); err != nil {
			t.Errorf("AddQuery(%q): %v", e.input, err)
		}
	}

	history, err := d.RecentQueries(10)
	if err != nil {
		t.Fatalf("RecentQueries: %v", err)
	}
	if len(history) != len(entries) {
		t.Errorf("got %d history entries, want %d", len(history), len(entries))
	}
}

// SCN-16: history survives across DB close/open (not :memory: for this test)
func TestDB_Persistence(t *testing.T) {
	path := t.TempDir() + "/vida-test.db"

	d1, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	_ = d1.AddQuery("what is inode", "ai", "An inode is...")
	d1.Close()

	d2, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open (reopen): %v", err)
	}
	defer d2.Close()

	history, err := d2.RecentQueries(10)
	if err != nil {
		t.Fatalf("RecentQueries: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("history empty after reopen; expected 1 entry")
	}
	if history[0].Input != "what is inode" {
		t.Errorf("history[0].Input = %q, want %q", history[0].Input, "what is inode")
	}
}

// FR-09e: RecentQueries returns most recent first
func TestDB_RecentFirst(t *testing.T) {
	d := openTestDB(t)

	_ = d.AddQuery("first", "ai", "")
	time.Sleep(time.Millisecond) // ensure different timestamps
	_ = d.AddQuery("second", "ai", "")
	time.Sleep(time.Millisecond)
	_ = d.AddQuery("third", "ai", "")

	history, _ := d.RecentQueries(10)
	if len(history) < 3 {
		t.Fatalf("expected 3 entries, got %d", len(history))
	}
	if history[0].Input != "third" {
		t.Errorf("history[0] = %q, want %q (most recent first)", history[0].Input, "third")
	}
	if history[2].Input != "first" {
		t.Errorf("history[2] = %q, want %q (oldest last)", history[2].Input, "first")
	}
}

// FR-09d: history capped at 500; oldest pruned
func TestDB_Cap(t *testing.T) {
	d := openTestDB(t)

	for i := 0; i < 510; i++ {
		input := "query-" + string(rune('0'+i%10)) + string(rune('0'+i/10%10)) + string(rune('0'+i/100))
		_ = d.AddQuery(input, "ai", "")
	}

	history, err := d.RecentQueries(600)
	if err != nil {
		t.Fatalf("RecentQueries: %v", err)
	}
	if len(history) > 500 {
		t.Errorf("history has %d entries, want ≤ 500 (FR-09d)", len(history))
	}
}

// SCN-17: ClearHistory truncates table
func TestDB_ClearHistory(t *testing.T) {
	d := openTestDB(t)

	for i := 0; i < 10; i++ {
		_ = d.AddQuery("entry", "ai", "")
	}

	if err := d.ClearHistory(); err != nil {
		t.Fatalf("ClearHistory: %v", err)
	}

	history, _ := d.RecentQueries(10)
	if len(history) != 0 {
		t.Errorf("after ClearHistory: got %d entries, want 0", len(history))
	}
}

// AC-R3: ClearHistory is idempotent on empty table
func TestDB_ClearHistory_Idempotent(t *testing.T) {
	d := openTestDB(t)

	// First clear on empty table
	if err := d.ClearHistory(); err != nil {
		t.Errorf("ClearHistory on empty table: %v", err)
	}
	// Second clear
	if err := d.ClearHistory(); err != nil {
		t.Errorf("ClearHistory second call: %v", err)
	}
}

// FR-09a: database path is configurable
func TestDB_CustomPath(t *testing.T) {
	path := t.TempDir() + "/custom.db"
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open(%q): %v", path, err)
	}
	defer d.Close()
	_ = d.AddQuery("test", "ai", "")
}

// TR-07b: WAL mode is enabled (requires a real file; WAL is not applicable to :memory:)
func TestDB_WALMode(t *testing.T) {
	path := t.TempDir() + "/wal-test.db"
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer d.Close()

	mode, err := d.JournalMode()
	if err != nil {
		t.Fatalf("JournalMode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}
