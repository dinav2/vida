// Tests for the notes package (SPEC-20260309-007).
package notes_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dinav2/vida/internal/notes"
)

// Bring exported names into scope for brevity.
type Config = notes.Config

func Save(cfg Config, title, body, tags string) error {
	return notes.Save(cfg, title, body, tags)
}
func DailyPath(cfg Config) string   { return notes.DailyPath(cfg) }
func SlugPath(cfg Config, t string) string { return notes.SlugPath(cfg, t) }
func Slug(s string) string          { return notes.Slug(s) }
func ApplyTemplate(tmpl, title, tags, body string) string {
	return notes.ApplyTemplate(tmpl, title, tags, body)
}

// --- SCN-09: Titled note saved to Inbox subdir ---

func TestSave_TitledNote(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Dir:         dir,
		InboxSubdir: "Inbox",
		DailySubdir: "Daily",
		Template:    "# {title}\n\ntags: {tags}\n\n{body}\n",
	}

	if err := Save(cfg, "my idea", "some text", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}

	expected := filepath.Join(dir, "Inbox", "my-idea.md")
	content, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("file not created at %s: %v", expected, err)
	}
	if !strings.Contains(string(content), "my idea") {
		t.Errorf("file does not contain title: %s", content)
	}
	if !strings.Contains(string(content), "some text") {
		t.Errorf("file does not contain body: %s", content)
	}
}

// --- SCN-10: Daily note created when no title ---

func TestSave_DailyNote(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Dir:         dir,
		InboxSubdir: "Inbox",
		DailySubdir: "Daily",
		Template:    "# {title}\n\ntags: {tags}\n\n{body}\n",
	}

	if err := Save(cfg, "", "quick thought", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	expected := filepath.Join(dir, "Daily", today+".md")
	if _, err := os.ReadFile(expected); err != nil {
		t.Fatalf("daily note not created at %s: %v", expected, err)
	}
}

// --- SCN-13: Daily note appends on collision ---

func TestSave_DailyNoteAppends(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Dir:         dir,
		InboxSubdir: "Inbox",
		DailySubdir: "Daily",
		Template:    "{body}\n",
	}

	if err := Save(cfg, "", "first entry", ""); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if err := Save(cfg, "", "second entry", ""); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	content, err := os.ReadFile(filepath.Join(dir, "Daily", today+".md"))
	if err != nil {
		t.Fatalf("read daily note: %v", err)
	}
	if !strings.Contains(string(content), "first entry") {
		t.Errorf("first entry missing from daily note")
	}
	if !strings.Contains(string(content), "second entry") {
		t.Errorf("second entry missing from daily note")
	}
	if !strings.Contains(string(content), "---") {
		t.Errorf("append separator '---' missing from daily note")
	}
}

// --- SCN-14: Titled note gets numeric suffix on collision ---

func TestSave_TitledNoteNumericSuffix(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Dir:         dir,
		InboxSubdir: "Inbox",
		DailySubdir: "Daily",
		Template:    "{body}\n",
	}

	if err := Save(cfg, "my idea", "first", ""); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if err := Save(cfg, "my idea", "second", ""); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	second := filepath.Join(dir, "Inbox", "my-idea-2.md")
	content, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("my-idea-2.md not created: %v", err)
	}
	if !strings.Contains(string(content), "second") {
		t.Errorf("second note content wrong: %s", content)
	}
}

// --- FR-06b: SlugPath produces correct path ---

func TestSlugPath(t *testing.T) {
	cfg := Config{Dir: "/notes", InboxSubdir: "Inbox"}
	got := SlugPath(cfg, "Hello World!")
	want := "/notes/Inbox/hello-world.md"
	if got != want {
		t.Errorf("SlugPath = %q, want %q", got, want)
	}
}

// --- FR-06e: DailyPath produces correct path ---

func TestDailyPath(t *testing.T) {
	cfg := Config{Dir: "/notes", DailySubdir: "Daily"}
	got := DailyPath(cfg)
	today := time.Now().Format("2006-01-02")
	want := "/notes/Daily/" + today + ".md"
	if got != want {
		t.Errorf("DailyPath = %q, want %q", got, want)
	}
}

// --- FR-06f: Slug lowercases, replaces spaces with hyphens, strips non-alphanum ---

func TestSlug(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"My Idea!", "my-idea"},
		{"Go & Python", "go-python"},
		{"  spaces  ", "spaces"},
		{"already-slugged", "already-slugged"},
	}
	for _, c := range cases {
		got := Slug(c.input)
		if got != c.want {
			t.Errorf("Slug(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// --- FR-06g: Template substitution ---

func TestApplyTemplate(t *testing.T) {
	tmpl := "# {title}\n\ntags: {tags}\n\n{body}\n"
	got := ApplyTemplate(tmpl, "My Note", "go, testing", "This is the body.")
	if !strings.Contains(got, "# My Note") {
		t.Errorf("title not substituted: %s", got)
	}
	if !strings.Contains(got, "tags: go, testing") {
		t.Errorf("tags not substituted: %s", got)
	}
	if !strings.Contains(got, "This is the body.") {
		t.Errorf("body not substituted: %s", got)
	}
}

// --- FR-06c: Save returns error when Dir is empty ---

func TestSave_NoDirReturnsError(t *testing.T) {
	cfg := Config{Dir: "", InboxSubdir: "Inbox", DailySubdir: "Daily"}
	if err := Save(cfg, "title", "body", ""); err == nil {
		t.Errorf("Save with no Dir should return an error (FR-06c)")
	}
}

// --- FR-06h: Save creates parent directories if missing ---

func TestSave_CreatesSubdirs(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Dir:         dir,
		InboxSubdir: "Notes/Inbox",
		DailySubdir: "Notes/Daily",
		Template:    "{body}\n",
	}

	if err := Save(cfg, "deep note", "content", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}

	expected := filepath.Join(dir, "Notes", "Inbox", "deep-note.md")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("file not found at %s: %v", expected, err)
	}
}
