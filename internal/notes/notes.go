// Package notes writes plain Markdown note files (SPEC-20260309-007).
package notes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Config holds user configuration for note storage.
type Config struct {
	Dir         string
	DailySubdir string
	InboxSubdir string
	Template    string
}

// Save writes a note to disk.
// If title is empty the note is a daily note appended to {dir}/{DailySubdir}/{YYYY-MM-DD}.md.
// If title is set the note is saved to {dir}/{InboxSubdir}/{slug}.md with a numeric
// suffix on collision (slug-2.md, slug-3.md, …).
func Save(cfg Config, title, body, tags string) error {
	if cfg.Dir == "" {
		return errors.New("notes: dir not configured")
	}
	tmpl := cfg.Template
	if tmpl == "" {
		tmpl = "# {title}\n\ntags: {tags}\n\n{body}\n"
	}
	content := ApplyTemplate(tmpl, title, tags, body)

	if title == "" {
		return saveDailyNote(cfg, content)
	}
	return saveTitledNote(cfg, title, content)
}

func saveDailyNote(cfg Config, content string) error {
	subdir := cfg.DailySubdir
	if subdir == "" {
		subdir = "Daily"
	}
	dir := filepath.Join(expandHome(cfg.Dir), subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	today := time.Now().Format("2006-01-02")
	path := filepath.Join(dir, today+".md")

	if _, err := os.Stat(path); err == nil {
		// File exists — append with separator.
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = fmt.Fprintf(f, "\n---\n%s", content)
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func saveTitledNote(cfg Config, title, content string) error {
	subdir := cfg.InboxSubdir
	if subdir == "" {
		subdir = "Inbox"
	}
	dir := filepath.Join(expandHome(cfg.Dir), subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	base := Slug(title)
	path := filepath.Join(dir, base+".md")
	if _, err := os.Stat(path); err == nil {
		// Collision — find next available numeric suffix.
		for n := 2; ; n++ {
			candidate := filepath.Join(dir, fmt.Sprintf("%s-%d.md", base, n))
			if _, err := os.Stat(candidate); err != nil {
				path = candidate
				break
			}
		}
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// DailyPath returns the expected path for today's daily note.
func DailyPath(cfg Config) string {
	subdir := cfg.DailySubdir
	if subdir == "" {
		subdir = "Daily"
	}
	today := time.Now().Format("2006-01-02")
	return filepath.Join(expandHome(cfg.Dir), subdir, today+".md")
}

// SlugPath returns the expected path for a titled note (without collision handling).
func SlugPath(cfg Config, title string) string {
	subdir := cfg.InboxSubdir
	if subdir == "" {
		subdir = "Inbox"
	}
	return filepath.Join(expandHome(cfg.Dir), subdir, Slug(title)+".md")
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// Slug converts a title to a lowercase hyphen-separated filename-safe string.
func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// ApplyTemplate substitutes {title}, {tags}, and {body} placeholders.
func ApplyTemplate(tmpl, title, tags, body string) string {
	r := strings.NewReplacer(
		"{title}", title,
		"{tags}", tags,
		"{body}", body,
	)
	return r.Replace(tmpl)
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
