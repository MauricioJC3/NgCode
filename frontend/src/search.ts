// search.ts holds the project-wide search UI logic as small, pure/testable
// functions — kept out of main.ts (which builds real DOM and CodeMirror
// instances at module-load time and is therefore not safely importable in a
// unit test without a full jsdom + browser-API setup), same split rationale
// as update.ts/update.test.ts. main.ts wires this module to the real
// SearchInWorkspace/SearchCancel wailsjs bindings and EventsOn listeners;
// that wiring itself is not covered by an automated test — only manually
// verified (see design's Testing Strategy).

// SearchMatch/SearchResultBatch/SearchDone mirror the Go side's structs
// (search.go), received as "search:results"/"search:done" event payloads.
export interface SearchMatch {
  path: string;
  line: number;
  column: number;
  preview: string;
}

export interface SearchResultBatch {
  generation: number;
  matches: SearchMatch[];
  // truncated mirrors search.go's SearchResultBatch.Truncated: true when
  // this specific file's own results were capped (per-file cap or trimmed
  // by the workspace-wide total cap), independent of SearchDone.truncated
  // which only reflects the total cap.
  truncated: boolean;
}

export interface SearchDone {
  generation: number;
  total: number;
  truncated: boolean;
}

// debounce delays invoking fn until ms have elapsed since the last call —
// used to avoid firing a SearchInWorkspace request on every keystroke.
export function debounce<F extends (...args: any[]) => void>(fn: F, ms: number): F {
  let timer: ReturnType<typeof setTimeout> | undefined;
  return ((...args: Parameters<F>) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  }) as F;
}

// SearchSession tracks the frontend-local generation counter, independent
// from (but incremented in lockstep with) the backend's a.searchGen. Every
// "search:results"/"search:done"/"search:cancelled" event carries a
// generation; isCurrentGeneration lets main.ts discard anything that
// doesn't match the latest search the user started, belt-and-suspenders
// alongside the backend's own generation check (see design's Data Flow).
export interface SearchSession {
  generation: number;
}

export function nextGeneration(session: SearchSession): number {
  session.generation += 1;
  return session.generation;
}

export function isCurrentGeneration(session: SearchSession, generation: number): boolean {
  return session.generation === generation;
}

// groupMatchesByFile groups a flat match list by path, preserving the order
// in which each path was first seen — used to render one file section per
// group of matches instead of a flat list.
export function groupMatchesByFile(matches: SearchMatch[]): Map<string, SearchMatch[]> {
  const grouped = new Map<string, SearchMatch[]>();
  for (const match of matches) {
    const existing = grouped.get(match.path);
    if (existing) {
      existing.push(match);
    } else {
      grouped.set(match.path, [match]);
    }
  }
  return grouped;
}

// trackTruncatedFile records a truncated batch's file path into
// truncatedPaths — called once per "search:results" event so
// renderSearchResults (main.ts) can show a per-file truncation indicator on
// that file's result group, distinct from the existing global
// #search-truncated banner which only reflects the total-cap flag on
// search:done. A no-op when the batch isn't truncated or carries no
// matches (a per-file batch always has at least one match by construction,
// but this stays defensive).
export function trackTruncatedFile(truncatedPaths: Set<string>, batch: SearchResultBatch): void {
  if (!batch.truncated || batch.matches.length === 0) return;
  truncatedPaths.add(batch.matches[0].path);
}
