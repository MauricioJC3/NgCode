package main

import (
	"bufio"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// GitStatusResult carries the outcome of a GitStatus call: the generation
// that produced it (see isGitGenCurrent -- mirrors search.go's
// SearchResultBatch.Generation/isSearchGenCurrent pairing), the workspace
// root it was computed for, and a map from each non-clean absolute path to
// its 2-character porcelain XY status code. A path with no entry in
// Statuses is clean (git status --porcelain never lists clean paths).
type GitStatusResult struct {
	Generation int64             `json:"generation"`
	Root       string            `json:"root"`
	Statuses   map[string]string `json:"statuses"`
}

// GitDiffResult carries the outcome of a GitDiffForFile call: 0-indexed
// new-file line numbers, grouped by how each line changed relative to HEAD.
type GitDiffResult struct {
	Path     string `json:"path"`
	Added    []int  `json:"added"`
	Modified []int  `json:"modified"`
	Removed  []int  `json:"removed"`
}

// isGitGenCurrent reports whether gen still matches currentGen. Kept as a
// pure two-argument comparison (rather than an App method like
// isSearchGenCurrent) so it has no dependency on App's gitGen/gitMu fields,
// which are only added in the app.go wiring PR -- this keeps git.go's
// parsers, including this one, self-contained pure functions for PR 1.
func isGitGenCurrent(currentGen, gen int64) bool {
	return currentGen == gen
}

// parseGitPorcelain parses the raw output of
// `git status --porcelain=v1 --untracked-files=all` into a map from each
// non-clean path to its 2-character XY status code. Each line is
// "XY<space>path", or "XY<space>old -> new" for renames/copies (X == 'R' or
// 'C'), in which case the entry is keyed by the new (destination) path --
// that's the path currently rendered in the tree. Paths come back relative
// to the -C root git was invoked with; joining them to absolute paths is
// the caller's job (App.GitStatus, app.go), keeping this function a pure,
// git-process-free parser like searchFile/searchLine in search.go.
func parseGitPorcelain(output string) map[string]string {
	statuses := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 4 {
			continue
		}
		xy := line[:2]
		path := line[3:]
		if idx := strings.Index(path, " -> "); idx != -1 {
			path = path[idx+len(" -> "):]
		}
		statuses[path] = xy
	}

	return statuses
}

// hunkHeaderRe matches a unified diff hunk header, e.g. "@@ -12,5 +14,3 @@",
// capturing the new-file starting line number (group 1).
var hunkHeaderRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// parseUnifiedDiff parses the raw output of `git diff HEAD -- <path>` into
// a GitDiffResult. Within each contiguous change block (a run of "-" lines
// immediately followed by a run of "+" lines, which is how git always
// emits them), the first min(removed, added) added lines are paired 1:1
// with removed lines and classified "modified"; any added lines beyond
// that pairing are "added"; any removed lines beyond that pairing have no
// corresponding new-file line, so their marker is anchored at the next
// surviving new-file line (0-indexed) -- the first context/unchanged line
// after the block, or the next hunk/end of file.
func parseUnifiedDiff(path, output string) GitDiffResult {
	result := GitDiffResult{Path: path, Added: []int{}, Modified: []int{}, Removed: []int{}}

	var newLine int // 1-indexed new-file line number of the next line to be consumed
	var removedCount int
	var addedLines []int // 1-indexed new-file line numbers seen in the current change block

	flush := func() {
		paired := removedCount
		if len(addedLines) < paired {
			paired = len(addedLines)
		}
		for i := 0; i < paired; i++ {
			result.Modified = append(result.Modified, addedLines[i]-1)
		}
		for i := paired; i < len(addedLines); i++ {
			result.Added = append(result.Added, addedLines[i]-1)
		}
		if removedCount > paired {
			result.Removed = append(result.Removed, newLine-1)
		}
		removedCount = 0
		addedLines = nil
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "@@"):
			flush()
			m := hunkHeaderRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			n, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			newLine = n
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			// File header lines, not content -- must be checked before the
			// generic "+"/"-" content-line cases below.
		case strings.HasPrefix(line, "+"):
			addedLines = append(addedLines, newLine)
			newLine++
		case strings.HasPrefix(line, "-"):
			removedCount++
		case strings.HasPrefix(line, " "):
			flush()
			newLine++
		default:
			// "diff --git", "index", or any other non-hunk file-header
			// line -- ignore.
		}
	}
	flush()

	return result
}

// gitStatusOutput runs `git status --porcelain=v1 --untracked-files=all`
// rooted at root and returns its raw stdout. Errors (git not on PATH, not a
// repository, etc.) are returned as-is -- the caller (App.GitStatus,
// app.go) swallows them into an empty/silent result per the design's
// "Missing git / not-a-repo / no-HEAD-yet" convention; this wrapper itself
// stays a thin, inspectable exec.Command call, mirroring lsp.go's
// startLSPClient.
func gitStatusOutput(root string) (string, error) {
	cmd := exec.Command("git", "-C", root, "status", "--porcelain=v1", "--untracked-files=all")
	hideConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// gitDiffOutput runs `git diff HEAD -- <path>` rooted at root and returns
// its raw stdout, with the same error-passthrough convention as
// gitStatusOutput. `git diff` (without --exit-code) exits 0 even when the
// file has changes; a non-zero exit here means a real error -- not a repo,
// no HEAD yet (brand-new repo), or similar.
func gitDiffOutput(root, path string) (string, error) {
	cmd := exec.Command("git", "-C", root, "diff", "HEAD", "--", path)
	hideConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
