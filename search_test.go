package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// --- searchSkip -------------------------------------------------------

func TestSearchSkip(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{".git", true},
		{"node_modules", true},
		{"dist", true},
		{"build", true},
		{"vendor", true},
		{".next", true},
		{"target", true},
		{"src", false},
		{"frontend", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := searchSkip(tt.name); got != tt.want {
				t.Errorf("searchSkip(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// --- isBinaryFile -------------------------------------------------------

func TestIsBinaryFileNullByte(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin.dat")
	content := append([]byte("hello"), 0x00, 'w', 'o', 'r', 'l', 'd')
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	binary, err := isBinaryFile(path)
	if err != nil {
		t.Fatalf("isBinaryFile: %v", err)
	}
	if !binary {
		t.Errorf("expected file with null byte in first 8KB to be detected as binary")
	}
}

func TestIsBinaryFileNullByteBeyond8KB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "text.txt")
	// 9000 text bytes then a null byte beyond the first 8KB sniff window.
	content := append([]byte(strings.Repeat("a", 9000)), 0x00)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	binary, err := isBinaryFile(path)
	if err != nil {
		t.Fatalf("isBinaryFile: %v", err)
	}
	if binary {
		t.Errorf("expected null byte beyond first 8KB to NOT be detected as binary")
	}
}

func TestIsBinaryFileOversized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := f.Truncate(6 * 1024 * 1024); err != nil { // 6MB, over the 5MB cap
		t.Fatalf("truncate: %v", err)
	}
	f.Close()

	binary, err := isBinaryFile(path)
	if err != nil {
		t.Fatalf("isBinaryFile: %v", err)
	}
	if !binary {
		t.Errorf("expected file over 5MB to be treated as skippable (binary=true)")
	}
}

func TestIsBinaryFilePlainText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(path, []byte("just some plain text\nwith a couple lines\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	binary, err := isBinaryFile(path)
	if err != nil {
		t.Fatalf("isBinaryFile: %v", err)
	}
	if binary {
		t.Errorf("expected plain text file to NOT be detected as binary")
	}
}

// --- searchLine -------------------------------------------------------

func TestSearchLineCaseInsensitiveDefault(t *testing.T) {
	cols := searchLine("The Foo and foo and FOO", "foo", false)
	want := []int{4, 12, 20}
	if len(cols) != len(want) {
		t.Fatalf("searchLine cols = %v, want %v", cols, want)
	}
	for i := range want {
		if cols[i] != want[i] {
			t.Errorf("searchLine cols[%d] = %d, want %d", i, cols[i], want[i])
		}
	}
}

func TestSearchLineCaseSensitive(t *testing.T) {
	cols := searchLine("The Foo and foo and FOO", "Foo", true)
	want := []int{4}
	if len(cols) != len(want) {
		t.Fatalf("searchLine cols = %v, want %v", cols, want)
	}
	for i := range want {
		if cols[i] != want[i] {
			t.Errorf("searchLine cols[%d] = %d, want %d", i, cols[i], want[i])
		}
	}
}

func TestSearchLineNoMatch(t *testing.T) {
	cols := searchLine("nothing here", "zzz", false)
	if len(cols) != 0 {
		t.Errorf("expected no matches, got %v", cols)
	}
}

func TestSearchLineMultipleMatchesPerLine(t *testing.T) {
	cols := searchLine("abcabcabc", "abc", true)
	want := []int{0, 3, 6}
	if len(cols) != len(want) {
		t.Fatalf("searchLine cols = %v, want %v", cols, want)
	}
	for i := range want {
		if cols[i] != want[i] {
			t.Errorf("searchLine cols[%d] = %d, want %d", i, cols[i], want[i])
		}
	}
}

// --- searchFile -------------------------------------------------------

func TestSearchFilePerFileCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "many.txt")

	var sb strings.Builder
	for i := 0; i < 250; i++ {
		sb.WriteString("needle here\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	matches, err := searchFile(path, "needle", false)
	if err != nil {
		t.Fatalf("searchFile: %v", err)
	}
	if len(matches) != 200 {
		t.Errorf("searchFile matches = %d, want 200 (per-file cap)", len(matches))
	}
}

func TestSearchFilePreviewBounded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "long.txt")
	longLine := "needle " + strings.Repeat("x", 1000)
	if err := os.WriteFile(path, []byte(longLine+"\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	matches, err := searchFile(path, "needle", false)
	if err != nil {
		t.Fatalf("searchFile: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if len(matches[0].Preview) > maxPreviewLen {
		t.Errorf("preview length = %d, want <= %d", len(matches[0].Preview), maxPreviewLen)
	}
}

func TestSearchFileLineAndColumn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "positions.txt")
	content := "first line\nsecond needle line\nthird\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	matches, err := searchFile(path, "needle", false)
	if err != nil {
		t.Fatalf("searchFile: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Line != 1 {
		t.Errorf("match line = %d, want 1 (0-indexed)", matches[0].Line)
	}
	if matches[0].Column != strings.Index("second needle line", "needle") {
		t.Errorf("match column = %d, want %d", matches[0].Column, strings.Index("second needle line", "needle"))
	}
}

func TestSearchFileNoMatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty-of-matches.txt")
	if err := os.WriteFile(path, []byte("nothing to find here\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	matches, err := searchFile(path, "zzz", false)
	if err != nil {
		t.Fatalf("searchFile: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected no matches, got %d", len(matches))
	}
}

// --- runSearch / SearchInWorkspace integration --------------------------

// fakeEmit records emitted events for assertions, guarded by a mutex since
// runSearch's caller (SearchInWorkspace) invokes it from a background
// goroutine.
type fakeEmit struct {
	mu     sync.Mutex
	events []struct {
		name string
		data interface{}
	}
}

func (f *fakeEmit) emit(name string, data interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, struct {
		name string
		data interface{}
	}{name, data})
}

func (f *fakeEmit) snapshot() []struct {
	name string
	data interface{}
} {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]struct {
		name string
		data interface{}
	}, len(f.events))
	copy(out, f.events)
	return out
}

func TestRunSearchTotalCap(t *testing.T) {
	dir := t.TempDir()
	// 30 files x 200 matching lines each = 6000 total matches, over the
	// 5000 total cap.
	for i := 0; i < 30; i++ {
		var sb strings.Builder
		for j := 0; j < 200; j++ {
			sb.WriteString("needle\n")
		}
		path := filepath.Join(dir, fmt.Sprintf("file%02d.txt", i))
		if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	a := &App{searchGen: 1}
	emit := &fakeEmit{}
	a.runSearch(1, dir, "needle", false, emit.emit)

	events := emit.snapshot()
	total := 0
	var done *SearchDone
	for _, e := range events {
		switch v := e.data.(type) {
		case SearchResultBatch:
			total += len(v.Matches)
			if v.Generation != 1 {
				t.Errorf("batch generation = %d, want 1", v.Generation)
			}
		case SearchDone:
			d := v
			done = &d
		}
	}

	if total > 5000 {
		t.Errorf("total matches emitted = %d, want <= 5000 (total cap)", total)
	}
	if done == nil {
		t.Fatalf("expected a search:done event")
	}
	if !done.Truncated {
		t.Errorf("expected done.Truncated = true when total cap is reached")
	}
}

func TestRunSearchStaleGenerationDiscarded(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		var sb strings.Builder
		for j := 0; j < 50; j++ {
			sb.WriteString("needle\n")
		}
		path := filepath.Join(dir, fmt.Sprintf("stale%02d.txt", i))
		if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	a := &App{searchGen: 1}
	emit := &fakeEmit{}

	// Simulate generation 2 having started before generation 1's walk
	// finishes: bump searchGen up-front, then run generation 1's walk.
	a.searchGen = 2

	a.runSearch(1, dir, "needle", false, emit.emit)

	events := emit.snapshot()
	for _, e := range events {
		if e.name == "search:results" {
			t.Errorf("expected no search:results events for stale generation 1, got one")
		}
		if e.name == "search:done" {
			t.Errorf("expected no search:done event for stale generation 1, got one")
		}
	}

	sawCancelled := false
	for _, e := range events {
		if e.name == "search:cancelled" {
			sawCancelled = true
		}
	}
	if !sawCancelled {
		t.Errorf("expected a search:cancelled event for the superseded generation")
	}
}

func TestSearchInWorkspaceBumpsGeneration(t *testing.T) {
	a := &App{}
	dir := t.TempDir()

	if err := a.SearchInWorkspace(dir, "nothing", false); err != nil {
		t.Fatalf("SearchInWorkspace: %v", err)
	}
	if a.searchGen != 1 {
		t.Errorf("searchGen = %d, want 1 after first SearchInWorkspace call", a.searchGen)
	}

	if err := a.SearchInWorkspace(dir, "nothing", false); err != nil {
		t.Fatalf("SearchInWorkspace: %v", err)
	}
	if a.searchGen != 2 {
		t.Errorf("searchGen = %d, want 2 after second SearchInWorkspace call", a.searchGen)
	}
}

func TestSearchCancelBumpsGeneration(t *testing.T) {
	a := &App{searchGen: 5}
	a.SearchCancel()
	if a.searchGen != 6 {
		t.Errorf("searchGen = %d, want 6 after SearchCancel", a.searchGen)
	}
}
