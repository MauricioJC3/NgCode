// command-palette.ts holds the command palette's pure logic — kept out of
// main.ts for the same reason as move.ts/search.ts/lsp.ts/terminal.ts:
// main.ts builds real DOM at module-load time and isn't safely importable
// in a unit test without a full jsdom + browser-API setup.

/** A single entry the palette can list and execute. */
export interface PaletteCommand {
  id: string;
  label: string;
}

/** The subset of a KeyboardEvent isCommandPaletteShortcut needs. */
export interface CommandPaletteKeyEvent {
  ctrlKey: boolean;
  metaKey: boolean;
  shiftKey: boolean;
  key: string;
}

/**
 * Reports whether e is the command palette's open shortcut: Ctrl+Shift+P on
 * Windows/Linux, Cmd+Shift+P (metaKey) on macOS. Must fire regardless of
 * what else currently has focus — main.ts wires this at a capture-phase
 * window keydown listener (mirroring isTerminalToggleShortcut's rationale
 * in terminal.ts) so no focused widget can swallow it first.
 */
export function isCommandPaletteShortcut(e: CommandPaletteKeyEvent): boolean {
  return (e.ctrlKey || e.metaKey) && e.shiftKey && e.key.toLowerCase() === 'p';
}

/**
 * Scores how well query fuzzy-matches text, VS Code Quick Open style:
 * query's characters must appear in text, case-insensitively, in order
 * (a subsequence match) — but not necessarily contiguous. Returns null when
 * query is not a subsequence of text at all.
 *
 * Higher is a better match. The scoring rewards, per matched character:
 *   - a contiguous run immediately following the previous matched
 *     character (+3) — rewards exact substrings like "file" in "New File"
 *     over scattered hits.
 *   - a "word boundary" match, i.e. the matched character is the first
 *     character of text or immediately follows a non-alphanumeric
 *     separator (+5) — rewards acronym-style matches like "gtd" against
 *     "Go to Definition" landing on each word's initial letter.
 *   - a small, distance-based penalty (-1 per skipped character since the
 *     previous match) so two subsequence matches of equal "quality" still
 *     prefer the one that stays closer to a contiguous run.
 *
 * An empty query matches everything with a score of 0 (no signal either
 * way) — filterCommands treats it as "show the full list" regardless.
 */
export function fuzzyScore(query: string, text: string): number | null {
  if (query.length === 0) return 0;

  const q = query.toLowerCase();
  const t = text.toLowerCase();

  let score = 0;
  let qi = 0;
  let lastMatchIndex = -1;

  for (let ti = 0; ti < t.length && qi < q.length; ti++) {
    if (t[ti] !== q[qi]) continue;

    const isWordBoundary = ti === 0 || !/[a-z0-9]/.test(t[ti - 1]);
    const isContiguous = lastMatchIndex !== -1 && ti === lastMatchIndex + 1;
    const gap = lastMatchIndex === -1 ? 0 : ti - lastMatchIndex - 1;

    score += 1;
    if (isContiguous) score += 3;
    if (isWordBoundary) score += 5;
    score -= gap;

    lastMatchIndex = ti;
    qi++;
  }

  return qi === q.length ? score : null;
}

/**
 * Filters commands to those whose label matches query (via fuzzyScore),
 * sorted best-match first. Ties keep their original relative order (a
 * stable sort) rather than reshuffling equally-ranked commands. An empty
 * query returns the full list, unchanged, in its original order.
 */
export function filterCommands<T extends PaletteCommand>(commands: T[], query: string): T[] {
  if (query.length === 0) return commands.slice();

  return commands
    .map((command, index) => ({ command, index, score: fuzzyScore(query, command.label) }))
    .filter((entry): entry is { command: T; index: number; score: number } => entry.score !== null)
    .sort((a, b) => (b.score !== a.score ? b.score - a.score : a.index - b.index))
    .map((entry) => entry.command);
}

/**
 * Computes the next selected index for an Arrow Up/Down keypress over a
 * list of listLength items. Clamps at both ends rather than wrapping —
 * mirrors terminal.ts's clampPanelHeight, which established a clamp-not-wrap
 * convention for this codebase's UI navigation/sizing primitives. currentIndex
 * of -1 (no selection yet) moving down (+1) lands on the first item (0);
 * an empty list (listLength <= 0) always reports -1 (nothing to select).
 */
export function moveSelection(currentIndex: number, direction: 1 | -1, listLength: number): number {
  if (listLength <= 0) return -1;

  const next = currentIndex + direction;
  if (next < 0) return 0;
  if (next > listLength - 1) return listLength - 1;
  return next;
}
