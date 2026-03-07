// Package apps implements .desktop file indexing and fuzzy app search.
// Tests cover SCN-09, SCN-10, SCN-11, FR-06.
package apps_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dinav2/vida/internal/apps"
)

// writeDesktopFile writes a minimal .desktop file to dir and returns its path.
func writeDesktopFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	p := filepath.Join(dir, filename)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("writeDesktopFile: %v", err)
	}
	return p
}

func makeIndex(t *testing.T, entries []struct{ file, content string }) *apps.Index {
	t.Helper()
	dir := t.TempDir()
	for _, e := range entries {
		writeDesktopFile(t, dir, e.file, e.content)
	}
	idx, err := apps.BuildIndex([]string{dir})
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	return idx
}

// --- Parsing ---

func TestBuildIndex_ParsesFields(t *testing.T) {
	idx := makeIndex(t, []struct{ file, content string }{
		{"firefox.desktop", `[Desktop Entry]
Name=Firefox Web Browser
GenericName=Web Browser
Comment=Browse the World Wide Web
Keywords=web;browser;internet
Exec=firefox %u
Icon=firefox
Type=Application
`},
	})

	results := idx.Search("firefox", 10)
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'firefox', got none")
	}
	app := results[0]
	if app.Name != "Firefox Web Browser" {
		t.Errorf("Name = %q, want %q", app.Name, "Firefox Web Browser")
	}
	if app.ID != "firefox.desktop" {
		t.Errorf("ID = %q, want %q", app.ID, "firefox.desktop")
	}
}

// SCN-09: exact match returns top-ranked result
func TestSearch_ExactMatch(t *testing.T) {
	idx := makeIndex(t, []struct{ file, content string }{
		{"firefox.desktop", `[Desktop Entry]
Name=Firefox Web Browser
Exec=firefox %u
Type=Application
`},
		{"files.desktop", `[Desktop Entry]
Name=Files
Exec=nautilus
Type=Application
`},
	})

	results := idx.Search("firefox", 10)
	if len(results) == 0 {
		t.Fatal("expected results for 'firefox'")
	}
	if results[0].Name != "Firefox Web Browser" {
		t.Errorf("top result = %q, want Firefox Web Browser", results[0].Name)
	}
}

// SCN-10: fuzzy match on partial/scattered input
func TestSearch_FuzzyMatch(t *testing.T) {
	idx := makeIndex(t, []struct{ file, content string }{
		{"nautilus.desktop", `[Desktop Entry]
Name=GNOME Files
GenericName=File Manager
Exec=nautilus
Type=Application
`},
	})

	results := idx.Search("fls", 10)
	if len(results) == 0 {
		t.Errorf("expected fuzzy match for 'fls' against 'GNOME Files', got none")
	}
}

// SCN-11: NoDisplay=true excludes entry
func TestSearch_NoDisplayExcluded(t *testing.T) {
	idx := makeIndex(t, []struct{ file, content string }{
		{"hidden-app.desktop", `[Desktop Entry]
Name=Hidden App
Exec=hidden-app
Type=Application
NoDisplay=true
`},
	})

	results := idx.Search("Hidden App", 10)
	if len(results) > 0 {
		t.Errorf("NoDisplay=true app appeared in results: %v", results)
	}
}

// FR-06d: search matches against Name, GenericName, Comment, Keywords
func TestSearch_MatchesMultipleFields(t *testing.T) {
	idx := makeIndex(t, []struct{ file, content string }{
		{"calc.desktop", `[Desktop Entry]
Name=Calculator
GenericName=Math Tool
Comment=Perform calculations
Keywords=math;calculator;arithmetic
Exec=gnome-calculator
Type=Application
`},
	})

	for _, query := range []string{"Calculator", "Math Tool", "arithmetic"} {
		results := idx.Search(query, 10)
		if len(results) == 0 {
			t.Errorf("Search(%q): expected match via field content, got none", query)
		}
	}
}

// FR-06e: results capped at max
func TestSearch_MaxResults(t *testing.T) {
	entries := []struct{ file, content string }{}
	for i := 0; i < 20; i++ {
		entries = append(entries, struct{ file, content string }{
			file: "app" + string(rune('a'+i)) + ".desktop",
			content: `[Desktop Entry]
Name=App ` + string(rune('A'+i)) + `
Exec=app` + string(rune('a'+i)) + `
Type=Application
`,
		})
	}
	idx := makeIndex(t, entries)

	results := idx.Search("App", 5)
	if len(results) > 5 {
		t.Errorf("Search returned %d results, want ≤ 5", len(results))
	}
}

// TR-05c: search < 10ms for 500 apps
func TestSearch_Performance(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 500; i++ {
		name := "perf-app-" + string(rune('a'+i%26)) + "-" + string(rune('0'+i%10))
		content := "[Desktop Entry]\nName=" + name + "\nExec=" + name + "\nType=Application\n"
		writeDesktopFile(t, dir, name+".desktop", content)
	}
	idx, err := apps.BuildIndex([]string{dir})
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	start := time.Now()
	_ = idx.Search("perf", 8)
	elapsed := time.Since(start)

	if elapsed > 10*time.Millisecond {
		t.Errorf("Search on 500 apps took %v, want < 10ms (TR-05c)", elapsed)
	}
}

// FR-06g: exec placeholder expansion
func TestExpandExec(t *testing.T) {
	cases := []struct {
		exec string
		want string
	}{
		{"firefox %u", "firefox"},
		{"nautilus %f", "nautilus"},
		{"code %U", "code"},
		{"app %F extra", "app extra"},
		{"plain-binary", "plain-binary"},
	}
	for _, tc := range cases {
		got := apps.ExpandExec(tc.exec)
		if got != tc.want {
			t.Errorf("ExpandExec(%q) = %q, want %q", tc.exec, got, tc.want)
		}
	}
}

// Zero-score results excluded (TR-05d)
func TestSearch_ZeroScoreExcluded(t *testing.T) {
	idx := makeIndex(t, []struct{ file, content string }{
		{"unrelated.desktop", `[Desktop Entry]
Name=Completely Unrelated App
Exec=unrelated
Type=Application
`},
	})

	results := idx.Search("zxqwerty", 10)
	for _, r := range results {
		if r.Score == 0 {
			t.Errorf("result with zero score included: %v", r)
		}
	}
}
