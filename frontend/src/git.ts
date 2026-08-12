// git.ts holds the pure structured-data -> UI-shape mapping behind git
// status badges and diff gutter markers — kept out of main.ts for the same
// reason as lsp.ts/move.ts/search.ts: main.ts builds real DOM and
// CodeMirror instances at module-load time and isn't safely importable in
// a unit test without a full jsdom + browser-API setup.
//
// Raw parsing (porcelain status lines, unified diff hunks) happens in Go
// (git.go: parseGitPorcelain/parseUnifiedDiff) and is table-tested there —
// these two functions only translate the already-structured results Wails
// hands back into shapes main.ts's DOM/CodeMirror code consumes, mirroring
// lsp.ts's lspDiagnosticsForDoc/lspSeverityToCM split (Go parses, TS maps).

// gitBadgeClass classifies a 2-character porcelain XY status code (as
// produced by `git status --porcelain=v1`, see git.go's parseGitPorcelain)
// into the badge spec's file-status categories. undefined (the path is
// absent from the status map returned by GitStatus) means clean — no
// badge. Any code this function doesn't recognize (e.g. a renamed/copied
// "R "/"C " or an unmerged "U*") also renders no badge — those categories
// are outside this change's spec (only modified/added/untracked/deleted
// are specified).
export function gitBadgeClass(xy: string | undefined): 'modified' | 'added' | 'untracked' | 'deleted' | null {
  if (!xy) return null;
  if (xy === '??') return 'untracked';
  if (xy.includes('D')) return 'deleted';
  if (xy.includes('A')) return 'added';
  if (xy.includes('M')) return 'modified';
  return null;
}

export interface GitDiffResult {
  added: number[];
  modified: number[];
  removed: number[];
}

export interface GitGutterMark {
  line0: number;
  type: 'added' | 'modified' | 'removed';
}

// gitDiffMarkers flattens a GitDiffResult's three line-number groups (each
// 0-indexed new-file line numbers, see git.go's parseUnifiedDiff) into a
// single list of gutter markers, sorted by line number ascending so a
// CodeMirror RangeSetBuilder (which requires ranges added in document
// order) can consume it directly.
export function gitDiffMarkers(r: GitDiffResult): GitGutterMark[] {
  const out: GitGutterMark[] = [];
  for (const line0 of r.added) out.push({ line0, type: 'added' });
  for (const line0 of r.modified) out.push({ line0, type: 'modified' });
  for (const line0 of r.removed) out.push({ line0, type: 'removed' });
  out.sort((a, b) => a.line0 - b.line0);
  return out;
}
