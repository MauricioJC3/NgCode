package main

import (
	"bufio"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// searchSkipDirs is the hardcoded ignore list applied by name (not full
// path) to every directory entry encountered while walking a workspace for
// a project-wide search.
var searchSkipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"dist":         true,
	"build":        true,
	"vendor":       true,
	".next":        true,
	"target":       true,
}

const (
	// maxSearchFileSize skips any file over this size (bytes) as if it were
	// binary — reading and scanning multi-megabyte files line-by-line for a
	// project-wide search isn't worth the cost and such files are rarely
	// meaningful source text anyway.
	maxSearchFileSize = 5 * 1024 * 1024

	// binarySniffLen is how many leading bytes of a file are inspected for
	// a null byte to decide whether it's binary.
	binarySniffLen = 8 * 1024

	// maxPreviewLen bounds SearchMatch.Preview so a single very long
	// (e.g. minified) line doesn't blow up the payload sent to the frontend.
	maxPreviewLen = 300

	// perFileMatchCap and totalMatchCap implement the "Match and Result
	// Caps" requirement.
	perFileMatchCap = 200
	totalMatchCap   = 5000
)

// SearchMatch is one line match within a file, streamed to the frontend in
// per-file batches.
type SearchMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`   // 0-indexed
	Column  int    `json:"column"` // 0-indexed byte offset
	Preview string `json:"preview"`
}

// SearchResultBatch carries all matches found in a single file, tagged with
// the generation of the search that produced them.
type SearchResultBatch struct {
	Generation int64         `json:"generation"`
	Matches    []SearchMatch `json:"matches"`
}

// SearchDone signals a search finished (not superseded), with the final
// match count and whether either cap truncated results.
type SearchDone struct {
	Generation int64 `json:"generation"`
	Total      int   `json:"total"`
	Truncated  bool  `json:"truncated"`
}

// searchSkip reports whether a directory entry named name should be
// excluded from a project-wide search, per the hardcoded ignore list.
func searchSkip(name string) bool {
	return searchSkipDirs[name]
}

// isBinaryFile reports whether path should be treated as binary/unsearchable:
// either it's larger than maxSearchFileSize, or its first binarySniffLen
// bytes contain a null byte (the standard binary-content heuristic).
func isBinaryFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if info.Size() > maxSearchFileSize {
		return true, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, binarySniffLen)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return false, err
	}

	for _, b := range buf[:n] {
		if b == 0 {
			return true, nil
		}
	}
	return false, nil
}

// searchLine returns the 0-indexed byte-offset column of every
// non-overlapping occurrence of query within line, honoring caseSensitive.
func searchLine(line, query string, caseSensitive bool) []int {
	if query == "" {
		return nil
	}

	haystack := line
	needle := query
	if !caseSensitive {
		haystack = strings.ToLower(line)
		needle = strings.ToLower(query)
	}

	var cols []int
	start := 0
	for start <= len(haystack) {
		idx := strings.Index(haystack[start:], needle)
		if idx == -1 {
			break
		}
		col := start + idx
		cols = append(cols, col)
		start = col + len(needle)
	}
	return cols
}

// searchFile scans path line by line for query, returning up to
// perFileMatchCap matches with previews bounded to maxPreviewLen.
func searchFile(path, query string, caseSensitive bool) ([]SearchMatch, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	matches := []SearchMatch{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		if len(matches) >= perFileMatchCap {
			break
		}

		line := scanner.Text()
		cols := searchLine(line, query, caseSensitive)
		for _, col := range cols {
			if len(matches) >= perFileMatchCap {
				break
			}
			matches = append(matches, SearchMatch{
				Path:    path,
				Line:    lineNum,
				Column:  col,
				Preview: boundPreview(line),
			})
		}
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return matches, nil
}

// boundPreview truncates s to maxPreviewLen bytes, if needed.
func boundPreview(s string) string {
	if len(s) <= maxPreviewLen {
		return s
	}
	return s[:maxPreviewLen]
}

// isSearchGenCurrent reports whether gen is still the active search
// generation — the single point where a.searchGen is read, under
// a.searchMu, mirroring currentLSP's role for lspClients in lsp.go.
func (a *App) isSearchGenCurrent(gen int64) bool {
	a.searchMu.Lock()
	defer a.searchMu.Unlock()
	return a.searchGen == gen
}

// runSearch walks root looking for query, emitting "search:results" batches
// (one per file with at least one match) via emit, then either
// "search:done" (generation still current) or "search:cancelled"
// (generation was superseded mid-walk) at the end. gen is checked between
// every directory/file step so a newer generation started by
// SearchInWorkspace or SearchCancel stops this walk promptly.
//
// emit is injected (rather than calling runtime.EventsEmit directly) so
// this can be driven by tests with a fake/no-op callback, without needing a
// real Wails runtime context.
func (a *App) runSearch(gen int64, root, query string, caseSensitive bool, emit func(event string, payload interface{})) {
	total := 0
	truncated := false

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip unreadable entries (permission errors, races with
			// concurrent filesystem changes) rather than aborting the
			// whole search.
			return nil
		}

		if !a.isSearchGenCurrent(gen) {
			return filepath.SkipAll
		}

		if d.IsDir() {
			if path != root && searchSkip(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if total >= totalMatchCap {
			truncated = true
			return filepath.SkipAll
		}

		binary, err := isBinaryFile(path)
		if err != nil || binary {
			return nil
		}

		matches, err := searchFile(path, query, caseSensitive)
		if err != nil || len(matches) == 0 {
			return nil
		}

		remaining := totalMatchCap - total
		if len(matches) > remaining {
			matches = matches[:remaining]
			truncated = true
		}
		total += len(matches)

		if !a.isSearchGenCurrent(gen) {
			return filepath.SkipAll
		}
		emit("search:results", SearchResultBatch{Generation: gen, Matches: matches})

		if total >= totalMatchCap {
			truncated = true
			return filepath.SkipAll
		}
		return nil
	})

	if !a.isSearchGenCurrent(gen) {
		emit("search:cancelled", map[string]interface{}{"generation": gen})
		return
	}

	emit("search:done", SearchDone{Generation: gen, Total: total, Truncated: truncated})
}

// SearchInWorkspace starts a project-wide text search rooted at root for
// query, streaming results asynchronously via "search:results"/
// "search:done"/"search:cancelled" events. Each call bumps the search
// generation, which supersedes (and stops emitting for) any still-running
// previous search — this is what backs both "new keystroke cancels an
// in-flight search" and SearchCancel.
func (a *App) SearchInWorkspace(root string, query string, caseSensitive bool) error {
	a.searchMu.Lock()
	a.searchGen++
	gen := a.searchGen
	a.searchMu.Unlock()

	go a.runSearch(gen, root, query, caseSensitive, func(event string, payload interface{}) {
		// a.ctx is nil in tests that call SearchInWorkspace directly on a
		// bare &App{} without going through startup — runtime.EventsEmit
		// would otherwise call log.Fatalf (os.Exit) on a nil context.
		if a.ctx == nil {
			return
		}
		runtime.EventsEmit(a.ctx, event, payload)
	})

	return nil
}

// SearchCancel supersedes the current search generation without starting a
// new one, so any in-flight runSearch stops emitting further events. Called
// for an empty query and as the first step of App.ForceQuit's teardown.
func (a *App) SearchCancel() {
	a.searchMu.Lock()
	a.searchGen++
	a.searchMu.Unlock()
}
