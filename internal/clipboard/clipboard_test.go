// Tests for clipboard history storage (SPEC-20260318-011).
package clipboard_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dinav2/vida/internal/clipboard"
	"github.com/dinav2/vida/internal/db"
)

// --- helpers ---

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func openTestStore(t *testing.T) *clipboard.Store {
	t.Helper()
	return clipboard.NewStore(openTestDB(t))
}

var defaultCfg = clipboard.Config{
	Enabled:    true,
	MaxEntries: 500,
	MaxAgeDays: 30,
}

// --- SCN-08: daemon stores new clipboard entry ---

func TestStore_Add_StoresEntry(t *testing.T) { // SCN-08
	s := openTestStore(t)

	stored, err := s.Add("hello world", defaultCfg)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !stored {
		t.Fatal("Add returned false; expected entry to be stored")
	}

	entries, err := s.List("", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Content != "hello world" {
		t.Errorf("Content = %q, want %q", entries[0].Content, "hello world")
	}
}

// --- SCN-09: consecutive duplicate not stored ---

func TestStore_Add_DeduplicatesConsecutive(t *testing.T) { // SCN-09
	s := openTestStore(t)

	s.Add("foo", defaultCfg)
	stored, err := s.Add("foo", defaultCfg)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if stored {
		t.Error("Add returned true for consecutive duplicate; expected false")
	}

	entries, _ := s.List("", 10)
	if len(entries) != 1 {
		t.Errorf("got %d entries after duplicate add, want 1", len(entries))
	}
}

func TestStore_Add_NonConsecutiveDuplicateIsStored(t *testing.T) {
	s := openTestStore(t)

	s.Add("foo", defaultCfg)
	s.Add("bar", defaultCfg)
	stored, _ := s.Add("foo", defaultCfg) // not consecutive
	if !stored {
		t.Error("non-consecutive duplicate should be stored")
	}

	entries, _ := s.List("", 10)
	if len(entries) != 3 {
		t.Errorf("got %d entries, want 3", len(entries))
	}
}

// --- SCN-03: entries listed newest-first ---

func TestStore_List_NewestFirst(t *testing.T) { // SCN-03
	s := openTestStore(t)

	s.Add("first", defaultCfg)
	time.Sleep(time.Millisecond)
	s.Add("second", defaultCfg)
	time.Sleep(time.Millisecond)
	s.Add("third", defaultCfg)

	entries, err := s.List("", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	if entries[0].Content != "third" {
		t.Errorf("entries[0] = %q, want %q", entries[0].Content, "third")
	}
	if entries[2].Content != "first" {
		t.Errorf("entries[2] = %q, want %q", entries[2].Content, "first")
	}
}

// --- SCN-04: search filters entries (case-insensitive substring) ---

func TestStore_List_FiltersByQuery(t *testing.T) { // SCN-04
	s := openTestStore(t)

	s.Add("hello world", defaultCfg)
	s.Add("foo bar", defaultCfg)
	s.Add("HELLO uppercase", defaultCfg)

	entries, err := s.List("hello", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries for query %q, want 2", len(entries), "hello")
	}
	for _, e := range entries {
		if !strings.Contains(strings.ToLower(e.Content), "hello") {
			t.Errorf("entry %q does not match query %q", e.Content, "hello")
		}
	}
}

func TestStore_List_EmptyQueryReturnsAll(t *testing.T) {
	s := openTestStore(t)

	s.Add("a", defaultCfg)
	s.Add("b", defaultCfg)
	s.Add("c", defaultCfg)

	entries, _ := s.List("", 10)
	if len(entries) != 3 {
		t.Errorf("got %d entries, want 3", len(entries))
	}
}

// --- SCN-10: max_entries pruning ---

func TestStore_Add_PrunesOldestBeyondMaxEntries(t *testing.T) { // SCN-10
	s := openTestStore(t)
	cfg := clipboard.Config{Enabled: true, MaxEntries: 3, MaxAgeDays: 0}

	for i := 0; i < 3; i++ {
		s.Add(fmt.Sprintf("entry-%d", i), cfg)
		time.Sleep(time.Millisecond)
	}
	// Fourth entry triggers pruning.
	s.Add("entry-3", cfg)

	entries, _ := s.List("", 10)
	if len(entries) != 3 {
		t.Errorf("got %d entries after pruning, want 3 (max_entries=3)", len(entries))
	}
	for _, e := range entries {
		if e.Content == "entry-0" {
			t.Error("entry-0 (oldest) should have been pruned")
		}
	}
}

// --- SCN-11: max_age_days pruning ---

func TestStore_Add_PrunesExpiredEntries(t *testing.T) { // SCN-11
	database := openTestDB(t)
	cfg := clipboard.Config{Enabled: true, MaxEntries: 500, MaxAgeDays: 2}

	// Add an old entry via a store with a backdated clock.
	oldClock := func() time.Time { return time.Now().AddDate(0, 0, -5) }
	oldStore := clipboard.NewStoreWithClock(database, oldClock)
	oldStore.Add("old entry", cfg)

	// Add a new entry via a normal store — this triggers age pruning.
	newStore := clipboard.NewStore(database)
	newStore.Add("new entry", cfg)

	entries, _ := newStore.List("", 10)
	for _, e := range entries {
		if e.Content == "old entry" {
			t.Error("5-day-old entry should have been pruned (max_age_days=2)")
		}
	}
	found := false
	for _, e := range entries {
		if e.Content == "new entry" {
			found = true
		}
	}
	if !found {
		t.Error("new entry should be present after pruning")
	}
}

// --- SCN-12: pinned entries survive pruning ---

func TestStore_Prune_SkipsPinnedEntries(t *testing.T) { // SCN-12
	s := openTestStore(t)
	cfg := clipboard.Config{Enabled: true, MaxEntries: 1, MaxAgeDays: 0}

	s.Add("unpinned", cfg)
	time.Sleep(time.Millisecond)
	s.Add("to-be-pinned", cfg)

	entries, _ := s.List("to-be-pinned", 1)
	if len(entries) == 0 {
		t.Fatal("setup: to-be-pinned entry not found")
	}
	if err := s.TogglePin(entries[0].ID); err != nil {
		t.Fatalf("TogglePin: %v", err)
	}

	// Adding a new entry with max_entries=1 should prune unpinned only.
	time.Sleep(time.Millisecond)
	s.Add("new unpinned", cfg)

	all, _ := s.List("", 10)
	var foundPinned, foundOldUnpinned bool
	for _, e := range all {
		switch e.Content {
		case "to-be-pinned":
			foundPinned = true
		case "unpinned":
			foundOldUnpinned = true
		}
	}
	if !foundPinned {
		t.Error("pinned entry should survive pruning")
	}
	if foundOldUnpinned {
		t.Error("old unpinned entry should have been pruned")
	}
}

// --- SCN-13: delete removes single entry ---

func TestStore_Delete_RemovesEntry(t *testing.T) { // SCN-13
	s := openTestStore(t)

	s.Add("keep me", defaultCfg)
	s.Add("delete me", defaultCfg)

	entries, _ := s.List("delete me", 1)
	if len(entries) == 0 {
		t.Fatal("setup: delete me entry not found")
	}
	if err := s.Delete(entries[0].ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	all, _ := s.List("", 10)
	for _, e := range all {
		if e.Content == "delete me" {
			t.Error("deleted entry should not appear in list")
		}
	}
	found := false
	for _, e := range all {
		if e.Content == "keep me" {
			found = true
		}
	}
	if !found {
		t.Error("non-deleted entry should still be present")
	}
}

// --- SCN-14: clear removes all unpinned entries ---

func TestStore_Clear_RemovesUnpinnedOnly(t *testing.T) { // SCN-14
	s := openTestStore(t)

	s.Add("unpinned-1", defaultCfg)
	s.Add("unpinned-2", defaultCfg)
	s.Add("to-pin", defaultCfg)

	entries, _ := s.List("to-pin", 1)
	if len(entries) == 0 {
		t.Fatal("setup: to-pin entry not found")
	}
	s.TogglePin(entries[0].ID)

	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	all, _ := s.List("", 10)
	for _, e := range all {
		if e.Content == "unpinned-1" || e.Content == "unpinned-2" {
			t.Errorf("unpinned entry %q should have been cleared", e.Content)
		}
	}
	found := false
	for _, e := range all {
		if e.Content == "to-pin" && e.Pinned {
			found = true
		}
	}
	if !found {
		t.Error("pinned entry should survive Clear()")
	}
}

// --- SCN-15: ignore patterns suppress entries ---

func TestStore_Add_IgnorePatterns(t *testing.T) { // SCN-15
	s := openTestStore(t)
	cfg := clipboard.Config{
		Enabled:    true,
		MaxEntries: 500,
		MaxAgeDays: 30,
		Ignore:     []string{`^sk-`, `password=`},
	}

	cases := []struct {
		content string
		want    bool
	}{
		{"sk-abc123secret", false},
		{"user=alice&password=hunter2", false},
		{"normal clipboard text", true},
	}

	for _, tc := range cases {
		stored, err := s.Add(tc.content, cfg)
		if err != nil {
			t.Fatalf("Add(%q): %v", tc.content, err)
		}
		if stored != tc.want {
			t.Errorf("Add(%q) stored=%v, want %v", tc.content, stored, tc.want)
		}
	}

	entries, _ := s.List("", 10)
	if len(entries) != 1 {
		t.Errorf("got %d stored entries, want 1 (only the non-ignored one)", len(entries))
	}
}

// AC-06: invalid regex pattern causes a warning (not a crash), entry is stored.
func TestStore_Add_InvalidIgnoreRegex_NoCrash(t *testing.T) { // AC-06
	s := openTestStore(t)
	cfg := clipboard.Config{
		Enabled:    true,
		MaxEntries: 500,
		MaxAgeDays: 30,
		Ignore:     []string{`[invalid`},
	}

	stored, err := s.Add("test content", cfg)
	if err != nil {
		t.Fatalf("Add with invalid regex must not error: %v", err)
	}
	if !stored {
		t.Error("entry should be stored when ignore pattern is invalid (pattern skipped)")
	}
}

// --- SCN-16: large entries stored at full length ---

func TestStore_Add_LargeEntryPreservesFullContent(t *testing.T) { // SCN-16
	s := openTestStore(t)
	longContent := strings.Repeat("a", 300)
	s.Add(longContent, defaultCfg)

	entries, _ := s.List("", 10)
	if len(entries) == 0 {
		t.Fatal("expected 1 entry")
	}
	if entries[0].Content != longContent {
		t.Errorf("stored content length = %d, want %d (full content must be stored)", len(entries[0].Content), len(longContent))
	}
}

// --- SCN-17: image placeholder stored and retrievable ---

func TestStore_Add_ImagePlaceholder(t *testing.T) { // SCN-17
	s := openTestStore(t)

	stored, err := s.Add("[image]", defaultCfg)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !stored {
		t.Fatal("[image] placeholder should be stored")
	}

	entries, _ := s.List("", 10)
	if len(entries) == 0 || entries[0].Content != "[image]" {
		t.Errorf("expected [image] entry, got %v", entries)
	}
}

// --- TogglePin ---

func TestStore_TogglePin_SetsAndUnsets(t *testing.T) {
	s := openTestStore(t)
	s.Add("pin me", defaultCfg)

	entries, _ := s.List("", 10)
	id := entries[0].ID

	if err := s.TogglePin(id); err != nil {
		t.Fatalf("TogglePin (pin): %v", err)
	}
	entries, _ = s.List("", 10)
	if !entries[0].Pinned {
		t.Error("entry should be pinned after TogglePin")
	}

	if err := s.TogglePin(id); err != nil {
		t.Fatalf("TogglePin (unpin): %v", err)
	}
	entries, _ = s.List("", 10)
	if entries[0].Pinned {
		t.Error("entry should be unpinned after second TogglePin")
	}
}

// --- Delete nonexistent ID is a no-op ---

func TestStore_Delete_NonexistentID_NoError(t *testing.T) {
	s := openTestStore(t)
	if err := s.Delete(99999); err != nil {
		t.Errorf("Delete of nonexistent ID should not error: %v", err)
	}
}

// --- Clear is idempotent on empty store ---

func TestStore_Clear_Idempotent(t *testing.T) {
	s := openTestStore(t)
	if err := s.Clear(); err != nil {
		t.Errorf("Clear on empty store: %v", err)
	}
	if err := s.Clear(); err != nil {
		t.Errorf("Clear second call: %v", err)
	}
}
