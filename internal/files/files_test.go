// Package files implements filesystem indexing and fuzzy file search.
// Tests cover SCN-01–SCN-08 from SPEC-20260314-010.
package files_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dinav2/vida/internal/files"
)

// writeFile creates a file at dir/name with empty content and returns its path.
func writeFile(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(""), 0644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	return p
}

// mkdirIn creates a subdirectory inside dir and returns its path.
func mkdirIn(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatalf("mkdirIn: %v", err)
	}
	return p
}

// --- SCN-08: Index.Len ---

func TestIndex_Len(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "alpha.txt")
	writeFile(t, dir, "beta.txt")
	writeFile(t, dir, "gamma.md")
	writeFile(t, dir, "delta.go")
	writeFile(t, dir, "epsilon.sh")

	idx, err := files.BuildIndex([]string{dir}, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if got := idx.Len(); got != 5 {
		t.Errorf("Len() = %d, want 5", got)
	}
}

// --- SCN-02: Hidden file exclusion ---

func TestBuildIndex_HiddenExcludedByDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "visible.txt")
	writeFile(t, dir, ".hidden")
	writeFile(t, dir, ".bashrc")

	idx, err := files.BuildIndex([]string{dir}, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	results := idx.Search("bash", 10)
	for _, f := range results {
		if f.Name == ".bashrc" {
			t.Errorf("hidden file .bashrc appeared in results with includeHidden=false")
		}
	}
	results = idx.Search("hidden", 10)
	for _, f := range results {
		if f.Name == ".hidden" {
			t.Errorf("hidden file .hidden appeared in results with includeHidden=false")
		}
	}
}

func TestBuildIndex_HiddenIncludedWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".bashrc")

	idx, err := files.BuildIndex([]string{dir}, true)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	results := idx.Search("bashrc", 10)
	if len(results) == 0 {
		t.Error("expected .bashrc in results with includeHidden=true, got none")
	}
}

func TestBuildIndex_HiddenDirExcluded(t *testing.T) {
	dir := t.TempDir()
	hiddenDir := mkdirIn(t, dir, ".config")
	writeFile(t, hiddenDir, "config.toml")

	idx, err := files.BuildIndex([]string{dir}, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	results := idx.Search("config", 10)
	if len(results) > 0 {
		t.Errorf("file inside hidden dir appeared in results: %v", results[0].Path)
	}
}

// --- SCN-02e: Always-excluded directories ---

func TestBuildIndex_NodeModulesAlwaysExcluded(t *testing.T) {
	dir := t.TempDir()
	nmDir := mkdirIn(t, dir, "node_modules")
	writeFile(t, nmDir, "index.js")

	idx, err := files.BuildIndex([]string{dir}, true) // even with includeHidden=true
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	results := idx.Search("index", 10)
	for _, f := range results {
		if filepath.Base(filepath.Dir(f.Path)) == "node_modules" {
			t.Errorf("file inside node_modules appeared in results: %v", f.Path)
		}
	}
}

func TestBuildIndex_GitDirAlwaysExcluded(t *testing.T) {
	dir := t.TempDir()
	gitDir := mkdirIn(t, dir, ".git")
	writeFile(t, gitDir, "COMMIT_EDITMSG")

	idx, err := files.BuildIndex([]string{dir}, true)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	results := idx.Search("COMMIT", 10)
	if len(results) > 0 {
		t.Errorf("file inside .git appeared in results: %v", results[0].Path)
	}
}

// --- SCN-03: Fuzzy matching & ranking ---

func TestSearch_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md")

	idx, err := files.BuildIndex([]string{dir}, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	for _, query := range []string{"readme", "README", "Readme", "rEaDmE"} {
		results := idx.Search(query, 10)
		if len(results) == 0 {
			t.Errorf("Search(%q): expected match for README.md, got none", query)
		}
	}
}

func TestSearch_ExactPrefixRanksHighest(t *testing.T) {
	// SCN-03: README.md should rank above my_readme.md and readme_old.txt
	// because it is an exact case-insensitive prefix match.
	dir := t.TempDir()
	writeFile(t, dir, "README.md")
	writeFile(t, dir, "readme_old.txt")
	writeFile(t, dir, "my_readme.md")

	idx, err := files.BuildIndex([]string{dir}, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	results := idx.Search("readme", 10)
	if len(results) == 0 {
		t.Fatal("expected results for 'readme', got none")
	}
	if results[0].Name != "README.md" {
		t.Errorf("top result = %q, want README.md", results[0].Name)
	}
}

func TestSearch_FuzzySubsequence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "firewall_config.txt")

	idx, err := files.BuildIndex([]string{dir}, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	results := idx.Search("fwcfg", 10)
	if len(results) == 0 {
		t.Error("expected fuzzy subsequence match for 'fwcfg' against 'firewall_config.txt', got none")
	}
}

// --- SCN-04: Result count cap ---

func TestSearch_MaxResults(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 30; i++ {
		name := "document_" + string(rune('a'+i%26)) + ".txt"
		writeFile(t, dir, name)
	}

	idx, err := files.BuildIndex([]string{dir}, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	results := idx.Search("doc", 10)
	if len(results) > 10 {
		t.Errorf("Search returned %d results, want ≤ 10", len(results))
	}
}

// --- SCN-06: No results returns nil/empty slice ---

func TestSearch_NoMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "alpha.txt")

	idx, err := files.BuildIndex([]string{dir}, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	results := idx.Search("zzznomatch", 10)
	if len(results) != 0 {
		t.Errorf("expected no results for 'zzznomatch', got %d", len(results))
	}
}

// --- SCN-07: Custom dirs ---

func TestBuildIndex_OnlySearchesSpecifiedDirs(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	writeFile(t, dirA, "inA.txt")
	writeFile(t, dirB, "inB.txt")

	// Index only dirA.
	idx, err := files.BuildIndex([]string{dirA}, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	resultsA := idx.Search("inA", 10)
	if len(resultsA) == 0 {
		t.Error("expected inA.txt in results, got none")
	}
	resultsB := idx.Search("inB", 10)
	if len(resultsB) > 0 {
		t.Errorf("inB.txt from outside specified dir appeared in results: %v", resultsB[0].Path)
	}
}

// --- File struct fields ---

func TestBuildIndex_FileHasPathAndName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "myfile.go")

	idx, err := files.BuildIndex([]string{dir}, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	results := idx.Search("myfile", 10)
	if len(results) == 0 {
		t.Fatal("expected result for 'myfile', got none")
	}
	f := results[0]
	if f.Name != "myfile.go" {
		t.Errorf("Name = %q, want %q", f.Name, "myfile.go")
	}
	if !filepath.IsAbs(f.Path) {
		t.Errorf("Path = %q, want absolute path", f.Path)
	}
	expected := filepath.Join(dir, "myfile.go")
	if f.Path != expected {
		t.Errorf("Path = %q, want %q", f.Path, expected)
	}
}

// --- Subdirectory recursion ---

func TestBuildIndex_RecursiveWalk(t *testing.T) {
	dir := t.TempDir()
	sub := mkdirIn(t, dir, "subdir")
	writeFile(t, sub, "nested.txt")

	idx, err := files.BuildIndex([]string{dir}, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	results := idx.Search("nested", 10)
	if len(results) == 0 {
		t.Error("expected nested.txt in results from recursive walk, got none")
	}
}

// --- Empty dir ---

func TestBuildIndex_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	idx, err := files.BuildIndex([]string{dir}, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if idx.Len() != 0 {
		t.Errorf("Len() = %d, want 0 for empty dir", idx.Len())
	}
}
