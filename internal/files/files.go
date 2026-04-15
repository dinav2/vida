// Package files implements filesystem indexing and fuzzy file search.
// See SPEC-20260314-010.
package files

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// alwaysExcluded are directory names skipped unconditionally (FR-02e).
var alwaysExcluded = map[string]bool{
	"node_modules": true,
	".git":         true,
	".svn":         true,
}

// File represents a single indexed file.
type File struct {
	Path string // absolute path
	Name string // basename
	Icon string // GTK icon name (may be empty → UI uses fallback)
}

// Index holds the in-memory file index.
type Index struct {
	files []File
	items []searchItem
}

// searchItem maps a lowercase basename back to its parent file index.
type searchItem struct {
	text    string // lowercase basename
	fileIdx int
}

// Len returns the number of indexed files.
func (idx *Index) Len() int { return len(idx.files) }

// BuildIndex walks dirs recursively and returns a populated Index.
// Hidden files/dirs (names starting with ".") are skipped unless includeHidden is true.
// node_modules, .git, and .svn are always excluded (FR-02e).
// Non-existent dirs are silently skipped (FR-02a).
func BuildIndex(dirs []string, includeHidden bool) (*Index, error) {
	var fileList []File

	for _, root := range dirs {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip unreadable entries
			}

			name := d.Name()

			// Always exclude certain directory names.
			if d.IsDir() && alwaysExcluded[name] {
				return filepath.SkipDir
			}

			// Skip hidden entries unless opted in.
			if !includeHidden && strings.HasPrefix(name, ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if d.IsDir() {
				return nil // descend
			}

			// Only regular files (skip symlinks to dirs, devices, etc.).
			if !d.Type().IsRegular() {
				return nil
			}

			fileList = append(fileList, File{
				Path: path,
				Name: name,
				Icon: iconForName(name),
			})
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			// Non-fatal; log if desired but keep going.
			continue
		}
	}

	idx := &Index{files: fileList}
	idx.buildItems()
	return idx, nil
}

// buildItems populates the flat search item list from all file basenames.
func (idx *Index) buildItems() {
	idx.items = make([]searchItem, len(idx.files))
	for i, f := range idx.files {
		idx.items[i] = searchItem{text: strings.ToLower(f.Name), fileIdx: i}
	}
}

// Search returns up to max Files whose basenames fuzzy-match query (case-insensitive).
// Results are sorted by score descending, then alphabetically by name.
// Returns nil when there are no matches.
func (idx *Index) Search(query string, max int) []File {
	if len(idx.items) == 0 || query == "" {
		return nil
	}

	lq := strings.ToLower(query)

	type scored struct {
		file  File
		score int
	}
	var results []scored

	for _, item := range idx.items {
		s := fuzzyScore(lq, item.text)
		if s > 0 {
			results = append(results, scored{file: idx.files[item.fileIdx], score: s})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].file.Name < results[j].file.Name
	})

	if len(results) > max {
		results = results[:max]
	}

	out := make([]File, len(results))
	for i, r := range results {
		out[i] = r.file
	}
	return out
}

// fuzzyScore returns a positive integer if pattern is a subsequence of str,
// or 0 if not. Higher scores indicate tighter matches.
// Exact prefix matches score highest, followed by substring, then subsequence.
func fuzzyScore(pattern, str string) int {
	if pattern == "" || str == "" {
		return 0
	}

	// Exact prefix match: highest score.
	if strings.HasPrefix(str, pattern) {
		return 2000 - len(str)
	}

	// Substring match (not at start): high score.
	if idx := strings.Index(str, pattern); idx > 0 {
		return 1500 - idx - len(str)
	}

	// Fuzzy subsequence match.
	pr := []rune(pattern)
	sr := []rune(str)
	positions := make([]int, len(pr))

	pi := 0
	for si, r := range sr {
		if r == pr[pi] {
			positions[pi] = si
			pi++
			if pi == len(pr) {
				break
			}
		}
	}
	if pi < len(pr) {
		return 0 // not a subsequence
	}

	score := 1000 - (len(sr) - len(pr))
	for i := 1; i < len(positions); i++ {
		score -= positions[i] - positions[i-1] - 1
	}

	// Word-boundary bonus.
	first := positions[0]
	if first == 0 || sr[first-1] == '_' || sr[first-1] == '-' || sr[first-1] == '.' {
		score += 50
	}

	if score < 1 {
		score = 1
	}
	return score
}

// iconForName returns a GTK icon name derived from the file extension.
// Returns empty string when no specific icon is known (UI falls back to text-x-generic).
func iconForName(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".go":
		return "text-x-script"
	case ".py":
		return "text-x-script"
	case ".js", ".ts", ".jsx", ".tsx":
		return "text-x-script"
	case ".sh", ".bash", ".zsh", ".fish":
		return "text-x-script"
	case ".md", ".txt", ".rst":
		return "text-x-generic"
	case ".json", ".toml", ".yaml", ".yml", ".xml":
		return "text-x-generic"
	case ".pdf":
		return "application-pdf"
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp":
		return "image-x-generic"
	case ".mp4", ".mkv", ".webm", ".avi", ".mov":
		return "video-x-generic"
	case ".mp3", ".flac", ".ogg", ".wav":
		return "audio-x-generic"
	case ".zip", ".tar", ".gz", ".bz2", ".xz", ".zst":
		return "package-x-generic"
	default:
		return ""
	}
}
