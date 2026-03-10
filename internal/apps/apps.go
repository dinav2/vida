// Package apps implements .desktop file indexing and fuzzy application search.
package apps

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// App represents an indexed application entry.
type App struct {
	ID          string
	Name        string
	Description string
	Icon        string
	Exec        string
	Score       float64

	// extra fields used only for search indexing
	genericName string
	keywords    []string
}

// Index holds all indexed apps and the pre-built search item list.
type Index struct {
	apps  []App
	items []searchItem
}

// searchItem maps a lowercase searchable string back to its parent app index.
type searchItem struct {
	text   string // lowercase for case-insensitive matching
	appIdx int
}

// Len returns the number of indexed apps.
func (idx *Index) Len() int { return len(idx.apps) }


// DefaultDirs returns the standard XDG application directories, including
// Flatpak export paths (silently skipped if absent).
func DefaultDirs() []string {
	home := os.Getenv("HOME")
	return []string{
		"/usr/share/applications",
		"/usr/local/share/applications",
		home + "/.local/share/applications",
		// Flatpak: system-wide and per-user installs
		"/var/lib/flatpak/exports/share/applications",
		home + "/.local/share/flatpak/exports/share/applications",
	}
}

// BuildIndex scans dirs for .desktop files and returns a populated Index.
// Directories that don't exist are silently skipped (FR-06a).
func BuildIndex(dirs []string) (*Index, error) {
	var appList []App
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".desktop") {
				continue
			}
			app, err := parseDesktop(filepath.Join(dir, entry.Name()), entry.Name())
			if err != nil || app == nil {
				continue
			}
			appList = append(appList, *app)
		}
	}

	idx := &Index{apps: appList}
	idx.buildItems()
	return idx, nil
}

// buildItems populates the flat search item list from all app fields (FR-06d).
func (idx *Index) buildItems() {
	idx.items = nil
	for i, app := range idx.apps {
		add := func(s string) {
			if s != "" {
				idx.items = append(idx.items, searchItem{strings.ToLower(s), i})
			}
		}
		add(app.Name)
		add(app.genericName)
		add(app.Description)
		for _, kw := range app.keywords {
			add(kw)
		}
	}
}

// Search performs a fuzzy search and returns up to max results by score (FR-06d, TR-05).
// Zero-score results are excluded (TR-05d).
func (idx *Index) Search(query string, max int) []App {
	if len(idx.items) == 0 || query == "" {
		return nil
	}

	lq := strings.ToLower(query)

	// Aggregate best score per app across all its searchable fields.
	bestScore := make(map[int]int, len(idx.apps))
	for _, item := range idx.items {
		s := fuzzyScore(lq, item.text)
		if s > bestScore[item.appIdx] {
			bestScore[item.appIdx] = s
		}
	}

	var results []App
	for appIdx, score := range bestScore {
		if score <= 0 {
			continue
		}
		app := idx.apps[appIdx]
		app.Score = float64(score)
		results = append(results, app)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > max {
		results = results[:max]
	}
	return results
}

// fuzzyScore returns a positive integer if pattern is a subsequence of str,
// or 0 if not. Higher scores indicate tighter matches (fewer gaps, earlier
// start, word-boundary bonuses).
func fuzzyScore(pattern, str string) int {
	if pattern == "" || str == "" {
		return 0
	}

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
		return 0 // pattern is not a subsequence of str
	}

	// Base score; penalise target length and gaps between matched chars.
	score := 1000 - (len(sr) - len(pr))
	for i := 1; i < len(positions); i++ {
		score -= positions[i] - positions[i-1] - 1
	}

	// Word-boundary bonus: first matched char follows start or a separator.
	first := positions[0]
	if first == 0 || sr[first-1] == ' ' || sr[first-1] == '-' || sr[first-1] == '_' {
		score += 50
	}

	if score < 1 {
		score = 1 // any valid subsequence match gets at least 1
	}
	return score
}

// ExpandExec strips .desktop exec placeholders and normalises whitespace (FR-06g).
func ExpandExec(exec string) string {
	for _, ph := range []string{"%u", "%U", "%f", "%F", "%i", "%c", "%k"} {
		exec = strings.ReplaceAll(exec, ph, "")
	}
	return strings.Join(strings.Fields(exec), " ")
}

// --- .desktop file parser ---

type desktopEntry struct {
	name        string
	genericName string
	comment     string
	keywords    []string
	icon        string
	exec        string
	noDisplay   bool
	appType     string
}

func parseDesktop(path, filename string) (*App, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var e desktopEntry
	inEntry := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inEntry = line == "[Desktop Entry]"
			continue
		}
		if !inEntry {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "Name":
			e.name = value
		case "GenericName":
			e.genericName = value
		case "Comment":
			e.comment = value
		case "Icon":
			e.icon = value
		case "Exec":
			e.exec = value
		case "Type":
			e.appType = value
		case "NoDisplay":
			e.noDisplay = strings.EqualFold(value, "true")
		case "Keywords":
			for _, kw := range strings.Split(value, ";") {
				kw = strings.TrimSpace(kw)
				if kw != "" {
					e.keywords = append(e.keywords, kw)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if e.noDisplay || e.name == "" {
		return nil, nil
	}

	return &App{
		ID:          filename,
		Name:        e.name,
		Description: e.comment,
		Icon:        e.icon,
		Exec:        e.exec,
		genericName: e.genericName,
		keywords:    e.keywords,
	}, nil
}
