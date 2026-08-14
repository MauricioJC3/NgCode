package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// This file exercises App.GitStatus/App.runGitStatus and App.GitDiffForFile
// against REAL git repositories in t.TempDir() — not just that the Go code
// compiles — mirroring search_test.go's runSearch/SearchInWorkspace
// integration tests (fakeEmit, generation-guard assertions) and
// lsp_integration_test.go's convention of exercising the real external
// process (here, the `git` binary) rather than a fake.
//
// Skips itself if `git` isn't on PATH, since CI environments running
// `go test` may not have it installed.

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
}

// initGitRepo creates a fresh git repository in a new t.TempDir(), with
// user.name/user.email configured (required for commits) but no commits
// yet — callers that need a HEAD commit call writeAndCommit next.
func initGitRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)

	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// writeAndCommit writes name/content under dir and commits it, giving the
// repo a HEAD to diff against.
func writeAndCommit(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-q", "-m", "commit "+name)
}

// latestStatus returns the single GitStatusResult emitted to emit (via
// runGitStatus/GitStatus's "git:status" event), failing the test if none
// (or more than one) was emitted.
func latestStatus(t *testing.T, emit *fakeEmit) GitStatusResult {
	t.Helper()
	events := emit.snapshot()
	var results []GitStatusResult
	for _, e := range events {
		if e.name == "git:status" {
			results = append(results, e.data.(GitStatusResult))
		}
	}
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 git:status event, got %d (all events: %+v)", len(results), events)
	}
	return results[0]
}

// --- GitStatus / runGitStatus --------------------------------------------

func TestGitIntegrationNonRepoSilent(t *testing.T) {
	requireGit(t)
	dir := t.TempDir() // not a git repo — no .git dir

	a := &App{}
	emit := &fakeEmit{}
	a.runGitStatus(1, dir, emit.emit)

	events := emit.snapshot()
	if len(events) != 0 {
		t.Errorf("expected no events for a non-repo dir, got %d: %+v", len(events), events)
	}
}

func TestGitIntegrationMixedStatus(t *testing.T) {
	dir := initGitRepo(t)
	writeAndCommit(t, dir, "committed.txt", "line1\n")
	writeAndCommit(t, dir, "gone.txt", "bye\n")

	// modified: unstaged edit to a tracked file.
	if err := os.WriteFile(filepath.Join(dir, "committed.txt"), []byte("line1\nline2\n"), 0644); err != nil {
		t.Fatalf("modify committed.txt: %v", err)
	}
	// added: new file staged with `git add`.
	if err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("new\n"), 0644); err != nil {
		t.Fatalf("write staged.txt: %v", err)
	}
	runGit(t, dir, "add", "staged.txt")
	// untracked: new file, never added.
	if err := os.WriteFile(filepath.Join(dir, "loose.txt"), []byte("loose\n"), 0644); err != nil {
		t.Fatalf("write loose.txt: %v", err)
	}
	// deleted: tracked file removed from disk, not yet committed.
	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatalf("remove gone.txt: %v", err)
	}

	a := &App{gitGen: 1}
	emit := &fakeEmit{}
	a.runGitStatus(1, dir, emit.emit)
	result := latestStatus(t, emit)

	if result.Generation != 1 {
		t.Errorf("Generation = %d, want 1", result.Generation)
	}
	if result.Root != dir {
		t.Errorf("Root = %q, want %q", result.Root, dir)
	}

	want := map[string]string{
		filepath.Join(dir, "committed.txt"): " M",
		filepath.Join(dir, "staged.txt"):    "A ",
		filepath.Join(dir, "loose.txt"):     "??",
		filepath.Join(dir, "gone.txt"):      " D",
	}
	for path, xy := range want {
		if got := result.Statuses[path]; got != xy {
			t.Errorf("Statuses[%q] = %q, want %q (full map: %+v)", path, got, xy, result.Statuses)
		}
	}
}

func TestGitIntegrationRelativeAbsoluteRootParity(t *testing.T) {
	dir := initGitRepo(t)
	writeAndCommit(t, dir, "file.txt", "hello\n")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatalf("modify file.txt: %v", err)
	}

	a := &App{gitGen: 1}

	absEmit := &fakeEmit{}
	a.runGitStatus(1, dir, absEmit.emit)
	absResult := latestStatus(t, absEmit)
	absPath := filepath.Join(dir, "file.txt")
	if absResult.Statuses[absPath] != " M" {
		t.Errorf("absolute root: Statuses[%q] = %q, want %q (full map: %+v)", absPath, absResult.Statuses[absPath], " M", absResult.Statuses)
	}

	a.gitGen = 2
	t.Chdir(dir)
	relEmit := &fakeEmit{}
	a.runGitStatus(2, ".", relEmit.emit)
	relResult := latestStatus(t, relEmit)
	relPath := filepath.Join(".", "file.txt")
	if relResult.Statuses[relPath] != " M" {
		t.Errorf("relative root: Statuses[%q] = %q, want %q (full map: %+v)", relPath, relResult.Statuses[relPath], " M", relResult.Statuses)
	}
}

func TestGitStatusBumpsGeneration(t *testing.T) {
	requireGit(t)
	a := &App{}
	dir := t.TempDir()

	if err := a.GitStatus(dir); err != nil {
		t.Fatalf("GitStatus: %v", err)
	}
	if a.gitGen != 1 {
		t.Errorf("gitGen = %d, want 1 after first GitStatus call", a.gitGen)
	}

	if err := a.GitStatus(dir); err != nil {
		t.Fatalf("GitStatus: %v", err)
	}
	if a.gitGen != 2 {
		t.Errorf("gitGen = %d, want 2 after second GitStatus call", a.gitGen)
	}
}

func TestGitStatusStaleGenerationDiscarded(t *testing.T) {
	dir := initGitRepo(t)
	writeAndCommit(t, dir, "file.txt", "hello\n")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatalf("modify file.txt: %v", err)
	}

	a := &App{gitGen: 2} // simulate generation 2 already having started

	emit := &fakeEmit{}
	a.runGitStatus(1, dir, emit.emit) // stale generation 1

	events := emit.snapshot()
	for _, e := range events {
		if e.name == "git:status" {
			t.Errorf("expected no git:status event for stale generation 1, got one: %+v", e)
		}
	}
}

// --- GitDiffForFile --------------------------------------------------------

func TestGitIntegrationDiffAddedModifiedRemoved(t *testing.T) {
	dir := initGitRepo(t)
	writeAndCommit(t, dir, "file.txt", "line1\nline2\nline3\n")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("line1\nchanged2\nline3\nline4\n"), 0644); err != nil {
		t.Fatalf("modify file.txt: %v", err)
	}

	a := &App{}
	result, err := a.GitDiffForFile(filepath.Join(dir, "file.txt"))
	if err != nil {
		t.Fatalf("GitDiffForFile: %v", err)
	}
	if len(result.Modified) == 0 {
		t.Errorf("expected a Modified marker for the changed line, got %+v", result)
	}
	if len(result.Added) == 0 {
		t.Errorf("expected an Added marker for the new trailing line, got %+v", result)
	}
}

func TestGitIntegrationDiffUntrackedFileEmpty(t *testing.T) {
	dir := initGitRepo(t)
	writeAndCommit(t, dir, "committed.txt", "line1\n")
	loosePath := filepath.Join(dir, "loose.txt")
	if err := os.WriteFile(loosePath, []byte("brand new\n"), 0644); err != nil {
		t.Fatalf("write loose.txt: %v", err)
	}

	a := &App{}
	result, err := a.GitDiffForFile(loosePath)
	if err != nil {
		t.Fatalf("GitDiffForFile: %v", err)
	}
	if len(result.Added) != 0 || len(result.Modified) != 0 || len(result.Removed) != 0 {
		t.Errorf("expected no markers for an untracked file, got %+v", result)
	}
}

func TestGitIntegrationDiffNoHeadSilent(t *testing.T) {
	dir := initGitRepo(t) // no commits yet — repo has no HEAD
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("hi\n"), 0644); err != nil {
		t.Fatalf("write f.txt: %v", err)
	}

	a := &App{}
	result, err := a.GitDiffForFile(path)
	if err != nil {
		t.Fatalf("GitDiffForFile: expected nil error on no-HEAD repo, got %v", err)
	}
	if len(result.Added) != 0 || len(result.Modified) != 0 || len(result.Removed) != 0 {
		t.Errorf("expected no markers on a no-HEAD repo, got %+v", result)
	}
}
