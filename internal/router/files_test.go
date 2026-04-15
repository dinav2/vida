// File search routing tests for SPEC-20260314-010.
// Tests cover SCN-01 (trigger detection), SCN-05 (priority), SCN-06 (no results).
package router_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dinav2/vida/internal/files"
	"github.com/dinav2/vida/internal/router"
)

// buildFileIndex creates a temp dir with the given filenames, builds an index, and returns it.
func buildFileIndex(t *testing.T, names []string) *files.Index {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(""), 0644); err != nil {
			t.Fatalf("writeFile %q: %v", name, err)
		}
	}
	idx, err := files.BuildIndex([]string{dir}, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	return idx
}

// --- SCN-01: Trigger detection ---

func TestRoute_FileSearch_TildePrefix(t *testing.T) {
	// "~readme" must trigger file search.
	idx := buildFileIndex(t, []string{"README.md"})
	r := router.New(router.WithFileIndex(idx), router.WithNoAI())

	result := r.Route(context.Background(), "~readme")
	if result.Kind != router.KindFileList {
		t.Errorf("Route(\"~readme\").Kind = %q, want %q", result.Kind, router.KindFileList)
	}
}

func TestRoute_FileSearch_TildePrefixWithSpace(t *testing.T) {
	// "~ readme" (space after ~) should still trigger file search.
	idx := buildFileIndex(t, []string{"README.md"})
	r := router.New(router.WithFileIndex(idx), router.WithNoAI())

	result := r.Route(context.Background(), "~ readme")
	if result.Kind != router.KindFileList {
		t.Errorf("Route(\"~ readme\").Kind = %q, want %q", result.Kind, router.KindFileList)
	}
}

func TestRoute_FileSearch_NoTriggerWithoutPrefix(t *testing.T) {
	// "readme" (no ~) must NOT trigger file search; falls through to app/AI.
	idx := buildFileIndex(t, []string{"README.md"})
	r := router.New(
		router.WithFileIndex(idx),
		router.WithApps([]router.AppEntry{{ID: "x", Name: "readme-app", Score: 1.0}}),
		router.WithNoAI(),
	)

	result := r.Route(context.Background(), "readme")
	if result.Kind == router.KindFileList {
		t.Errorf("Route(\"readme\").Kind = KindFileList, expected app or AI result")
	}
}

func TestRoute_FileSearch_BareTildeReturnsEmpty(t *testing.T) {
	// "~" alone (no query) must return KindEmpty (SCN-01e).
	idx := buildFileIndex(t, []string{"README.md"})
	r := router.New(router.WithFileIndex(idx), router.WithNoAI())

	result := r.Route(context.Background(), "~")
	if result.Kind != router.KindEmpty {
		t.Errorf("Route(\"~\").Kind = %q, want %q", result.Kind, router.KindEmpty)
	}
}

func TestRoute_FileSearch_CommandModeNotFile(t *testing.T) {
	// ":files" starts with ":" so must be command mode, not file search (SCN-01).
	idx := buildFileIndex(t, []string{"files.txt"})
	r := router.New(router.WithFileIndex(idx), router.WithNoAI())

	result := r.Route(context.Background(), ":files")
	if result.Kind != router.KindCommandList {
		t.Errorf("Route(\":files\").Kind = %q, want %q", result.Kind, router.KindCommandList)
	}
}

// --- SCN-05: File search priority before apps ---

func TestRoute_FileSearch_BeforeApps(t *testing.T) {
	// With a "~fire" query, file results must win over app results.
	idx := buildFileIndex(t, []string{"firewall.conf"})
	r := router.New(
		router.WithFileIndex(idx),
		router.WithApps([]router.AppEntry{{ID: "firefox.desktop", Name: "Firefox", Score: 1.0}}),
		router.WithNoAI(),
	)

	result := r.Route(context.Background(), "~fire")
	if result.Kind != router.KindFileList {
		t.Errorf("Route(\"~fire\").Kind = %q, want KindFileList (file search before apps)", result.Kind)
	}
}

// --- SCN-06: No results → KindEmpty (not KindFileList with empty slice) ---

func TestRoute_FileSearch_NoMatchReturnsEmpty(t *testing.T) {
	idx := buildFileIndex(t, []string{"alpha.txt"})
	r := router.New(router.WithFileIndex(idx), router.WithNoAI())

	result := r.Route(context.Background(), "~zzznomatch")
	if result.Kind != router.KindEmpty {
		t.Errorf("Route(\"~zzznomatch\").Kind = %q, want %q (no files matched)", result.Kind, router.KindEmpty)
	}
}

// --- Result fields ---

func TestRoute_FileSearch_ResultFiles(t *testing.T) {
	// Verify Files slice is populated with correct Name and absolute Path.
	idx := buildFileIndex(t, []string{"notes.md"})
	r := router.New(router.WithFileIndex(idx), router.WithNoAI())

	result := r.Route(context.Background(), "~notes")
	if result.Kind != router.KindFileList {
		t.Fatalf("expected KindFileList, got %q", result.Kind)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected Files to be non-empty")
	}
	f := result.Files[0]
	if f.Name != "notes.md" {
		t.Errorf("Files[0].Name = %q, want %q", f.Name, "notes.md")
	}
	if !filepath.IsAbs(f.Path) {
		t.Errorf("Files[0].Path = %q, want absolute path", f.Path)
	}
}

// --- No file index configured ---

func TestRoute_FileSearch_NoIndexReturnsEmpty(t *testing.T) {
	// When no file index is configured, "~readme" should return KindEmpty.
	r := router.New(router.WithNoAI())

	result := r.Route(context.Background(), "~readme")
	if result.Kind != router.KindEmpty {
		t.Errorf("Route(\"~readme\") without file index = %q, want KindEmpty", result.Kind)
	}
}
