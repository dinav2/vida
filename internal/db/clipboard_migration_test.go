// Tests for clipboard_history schema migration (SPEC-20260318-011, TR-02).
package db_test

import (
	"testing"
)

// TR-02: clipboard_history table is created by migrate() with expected columns.
func TestDB_ClipboardHistorySchema(t *testing.T) {
	d := openTestDB(t)

	// Insert a row to verify the table and all columns exist.
	_, err := d.Conn().Exec(
		`INSERT INTO clipboard_history (content, copied_at, pinned) VALUES (?, ?, ?)`,
		"test entry", 1710000000, 0,
	)
	if err != nil {
		t.Fatalf("clipboard_history insert failed (table or columns missing): %v", err)
	}

	var content string
	var pinned int
	err = d.Conn().QueryRow(
		`SELECT content, pinned FROM clipboard_history WHERE content = ?`, "test entry",
	).Scan(&content, &pinned)
	if err != nil {
		t.Fatalf("clipboard_history select failed: %v", err)
	}
	if content != "test entry" {
		t.Errorf("content = %q, want %q", content, "test entry")
	}
	if pinned != 0 {
		t.Errorf("pinned = %d, want 0", pinned)
	}
}

// TR-02: pinned column defaults to 0 when not specified.
func TestDB_ClipboardHistoryPinnedDefault(t *testing.T) {
	d := openTestDB(t)

	_, err := d.Conn().Exec(
		`INSERT INTO clipboard_history (content, copied_at) VALUES (?, ?)`,
		"default pin test", 1710000001,
	)
	if err != nil {
		t.Fatalf("insert without pinned column: %v", err)
	}

	var pinned int
	d.Conn().QueryRow(
		`SELECT pinned FROM clipboard_history WHERE content = ?`, "default pin test",
	).Scan(&pinned)
	if pinned != 0 {
		t.Errorf("pinned default = %d, want 0", pinned)
	}
}
