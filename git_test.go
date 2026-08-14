package main

import (
	"testing"
)

// --- parseGitPorcelain -----------------------------------------------------
//
// Oracle: `git status --porcelain=v1 --untracked-files=all` output format is
// "XY<space>path" per entry (or "XY<space>old -> new" for renames/copies).
// Clean files never appear in git's output at all, so the "clean" case here
// is an empty output producing an empty map -- gitBadgeClass (frontend,
// PR 3) treats "path absent from this map" as clean.

func TestParseGitPorcelain(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   map[string]string
	}{
		{
			name:   "unstaged modified file",
			output: " M file.txt\n",
			want:   map[string]string{"file.txt": " M"},
		},
		{
			name:   "staged modified file",
			output: "M  file.txt\n",
			want:   map[string]string{"file.txt": "M "},
		},
		{
			name:   "staged added (new) file",
			output: "A  newfile.txt\n",
			want:   map[string]string{"newfile.txt": "A "},
		},
		{
			name:   "untracked file",
			output: "?? untracked.txt\n",
			want:   map[string]string{"untracked.txt": "??"},
		},
		{
			name:   "unstaged deleted file",
			output: " D deleted.txt\n",
			want:   map[string]string{"deleted.txt": " D"},
		},
		{
			name:   "staged deleted file",
			output: "D  staged-deleted.txt\n",
			want:   map[string]string{"staged-deleted.txt": "D "},
		},
		{
			name:   "renamed file keyed by the new (destination) path",
			output: "R  old.txt -> new.txt\n",
			want:   map[string]string{"new.txt": "R "},
		},
		{
			name:   "multiple entries in one scan",
			output: " M file.txt\nA  newfile.txt\n?? untracked.txt\n",
			want: map[string]string{
				"file.txt":      " M",
				"newfile.txt":   "A ",
				"untracked.txt": "??",
			},
		},
		{
			name:   "empty output (nothing but clean files) yields an empty map",
			output: "",
			want:   map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGitPorcelain(tt.output)
			if len(got) != len(tt.want) {
				t.Fatalf("parseGitPorcelain(%q) = %v, want %v", tt.output, got, tt.want)
			}
			for path, xy := range tt.want {
				if got[path] != xy {
					t.Errorf("parseGitPorcelain(%q)[%q] = %q, want %q", tt.output, path, got[path], xy)
				}
			}
		})
	}
}

// --- parseUnifiedDiff -------------------------------------------------------
//
// Oracle: unified diff hunks from `git diff HEAD -- <file>`. Within a
// contiguous change block, git always emits removed ("-") lines before
// added ("+") lines; this parser pairs them 1:1 in that order into
// "modified", with any surplus added lines becoming "added" and any surplus
// removed lines anchored as "removed" at the next surviving new-file line
// (0-indexed).

func TestParseUnifiedDiffAddedLine(t *testing.T) {
	diff := "diff --git a/f.txt b/f.txt\n" +
		"index 111..222 100644\n" +
		"--- a/f.txt\n" +
		"+++ b/f.txt\n" +
		"@@ -1,2 +1,3 @@\n" +
		" line1\n" +
		"+line2-new\n" +
		" line3\n"

	result := parseUnifiedDiff("f.txt", diff)
	if len(result.Added) != 1 || result.Added[0] != 1 {
		t.Errorf("Added = %v, want [1]", result.Added)
	}
	if len(result.Modified) != 0 {
		t.Errorf("Modified = %v, want []", result.Modified)
	}
	if len(result.Removed) != 0 {
		t.Errorf("Removed = %v, want []", result.Removed)
	}
}

func TestParseUnifiedDiffModifiedLine(t *testing.T) {
	diff := "diff --git a/f.txt b/f.txt\n" +
		"index 111..222 100644\n" +
		"--- a/f.txt\n" +
		"+++ b/f.txt\n" +
		"@@ -1,3 +1,3 @@\n" +
		" line1\n" +
		"-old line2\n" +
		"+new line2\n" +
		" line3\n"

	result := parseUnifiedDiff("f.txt", diff)
	if len(result.Modified) != 1 || result.Modified[0] != 1 {
		t.Errorf("Modified = %v, want [1]", result.Modified)
	}
	if len(result.Added) != 0 {
		t.Errorf("Added = %v, want []", result.Added)
	}
	if len(result.Removed) != 0 {
		t.Errorf("Removed = %v, want []", result.Removed)
	}
}

func TestParseUnifiedDiffRemovedLine(t *testing.T) {
	diff := "diff --git a/f.txt b/f.txt\n" +
		"index 111..222 100644\n" +
		"--- a/f.txt\n" +
		"+++ b/f.txt\n" +
		"@@ -1,3 +1,2 @@\n" +
		" line1\n" +
		"-removed line\n" +
		" line3\n"

	result := parseUnifiedDiff("f.txt", diff)
	// line1 is new-file line 0, "line3" (the surviving next line) is
	// new-file line 1 -- the removed marker anchors there.
	if len(result.Removed) != 1 || result.Removed[0] != 1 {
		t.Errorf("Removed = %v, want [1]", result.Removed)
	}
	if len(result.Added) != 0 {
		t.Errorf("Added = %v, want []", result.Added)
	}
	if len(result.Modified) != 0 {
		t.Errorf("Modified = %v, want []", result.Modified)
	}
}

func TestParseUnifiedDiffMultiHunk(t *testing.T) {
	diff := "diff --git a/f.txt b/f.txt\n" +
		"index 111..222 100644\n" +
		"--- a/f.txt\n" +
		"+++ b/f.txt\n" +
		"@@ -1,2 +1,3 @@\n" +
		" line1\n" +
		"+added-in-hunk1\n" +
		" line2\n" +
		"@@ -10,3 +11,2 @@\n" +
		" line10\n" +
		"-removed-in-hunk2\n" +
		" line12\n"

	result := parseUnifiedDiff("f.txt", diff)
	if len(result.Added) != 1 || result.Added[0] != 1 {
		t.Errorf("Added = %v, want [1]", result.Added)
	}
	if len(result.Removed) != 1 || result.Removed[0] != 11 {
		t.Errorf("Removed = %v, want [11]", result.Removed)
	}
}

func TestParseUnifiedDiffNoChanges(t *testing.T) {
	result := parseUnifiedDiff("f.txt", "")
	if len(result.Added) != 0 || len(result.Modified) != 0 || len(result.Removed) != 0 {
		t.Errorf("expected no markers for empty diff, got %+v", result)
	}
	if result.Path != "f.txt" {
		t.Errorf("Path = %q, want %q", result.Path, "f.txt")
	}
}

// --- isGitGenCurrent ---------------------------------------------------------
//
// Pure comparison mirroring the logic behind isSearchGenCurrent (search.go),
// but kept as a plain function (no App/mutex dependency) so it stays a
// self-contained pure parser here in PR 1 -- App.gitGen is only added in
// PR 2's app.go wiring.

func TestIsGitGenCurrent(t *testing.T) {
	if !isGitGenCurrent(3, 3) {
		t.Error("expected isGitGenCurrent(3, 3) = true when gen matches current")
	}
	if isGitGenCurrent(3, 2) {
		t.Error("expected isGitGenCurrent(3, 2) = false when gen is stale (older than current)")
	}
}
