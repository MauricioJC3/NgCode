// inline-edit.ts holds the pure, branch-heavy logic behind inline tree-row
// editing (Create File, Create Folder, Rename) — kept out of main.ts for the
// same reason as search.ts/update.ts: main.ts builds real DOM at module-load
// time and isn't safely importable in a unit test without a full jsdom +
// browser-API setup. main.ts's startInlineEdit orchestrates the actual DOM
// swap, focus, and commit/cancel wiring around these two functions; that
// wiring itself is only manually verified (see design's Testing Strategy).

export type InlineEditMode = 'create-file' | 'create-folder' | 'rename';

// ActiveEdit tracks the single live inline edit (main.ts enforces at most
// one at a time via the module-level `activeEdit` reference). dirPath is the
// edit's parent directory, used as the re-entrancy/discard key by
// shouldDiscardEdit; discard() is the caller-supplied cleanup (restore label
// span for rename, remove placeholder row for create).
export interface ActiveEdit {
  dirPath: string;
  discard: () => void;
}

// normalizeEditValue trims the raw input value and treats an empty (post-trim)
// result as "no name" — callers use the null return to silently cancel the
// edit (Escape and empty-Enter share this same discard semantics, per spec's
// "Cancel on Escape or empty name" requirement).
export function normalizeEditValue(raw: string): string | null {
  const trimmed = raw.trim();
  return trimmed === '' ? null : trimmed;
}

// shouldDiscardEdit answers whether a directory rebuild (refreshDir /
// toggleFolder's ReadDir branch) covering rebuildDirPath must discard the
// currently active edit first — true only when there is a live edit whose
// dirPath matches the directory being rebuilt. A null active edit never
// needs discarding.
export function shouldDiscardEdit(active: ActiveEdit | null, rebuildDirPath: string): boolean {
  if (active === null) return false;
  return active.dirPath === rebuildDirPath;
}
