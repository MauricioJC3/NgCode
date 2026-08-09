// move.ts holds the pure path-remapping logic behind remapTabPaths — kept
// out of main.ts for the same reason as search.ts/lsp.ts/confirm-queue.ts:
// main.ts builds real DOM at module-load time and isn't safely importable in
// a unit test without a full jsdom + browser-API setup.
//
// remapPath is a verbatim relocation of remapTabPaths' inline `remap`
// closure, parameterized with `path` as an explicit third argument instead
// of being closed over `oldPath`/`newPath`. main.ts's remapTabPaths keeps
// ownership of the tabs/activePath state; this function only decides what
// the new path should be, given a single path to check.

export function remapPath(oldPath: string, newPath: string, path: string): string | null {
  if (path === oldPath) return newPath;
  const sep = path[oldPath.length];
  if (path.startsWith(oldPath) && (sep === '/' || sep === '\\')) {
    return newPath + sep + path.slice(oldPath.length + 1);
  }
  return null;
}
