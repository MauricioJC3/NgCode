// terminal.ts holds the integrated terminal's pure logic — kept out of
// main.ts for the same reason as move.ts/search.ts/lsp.ts/confirm-queue.ts:
// main.ts builds real DOM at module-load time and isn't safely importable
// in a unit test without a full jsdom + browser-API setup.

/**
 * Clamps a candidate terminal panel height to the [min, max] range produced
 * by a drag-resize. When min === max the shared value is returned
 * regardless of height.
 */
export function clampPanelHeight(height: number, min: number, max: number): number {
  if (height < min) return min;
  if (height > max) return max;
  return height;
}

/** The subset of a KeyboardEvent isTerminalToggleShortcut needs. */
export interface TerminalToggleKeyEvent {
  ctrlKey: boolean;
  metaKey: boolean;
  key: string;
}

/**
 * Reports whether e is the terminal panel's toggle shortcut: Ctrl+` on
 * Windows/Linux, Cmd+` (metaKey) on macOS. Must fire regardless of what
 * else currently has focus, including the terminal itself — main.ts wires
 * this at a window-level keydown listener rather than relying on any
 * per-widget handler, so xterm.js's own keydown handling never gets a
 * chance to swallow it.
 */
export function isTerminalToggleShortcut(e: TerminalToggleKeyEvent): boolean {
  return (e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 't';
}

/**
 * Converts a pixel width and a single character's pixel width into an
 * integer column count. Floors (never rounds) so the computed dimension
 * never claims more columns than actually fit in the available width —
 * rounding up would let xterm.js believe a partial trailing column is
 * whole, which the real pty (and any $COLUMNS-reading shell prompt) would
 * disagree with. Non-positive widths report 0 columns.
 */
export function resizeToCols(pixelWidth: number, charWidth: number): number {
  if (pixelWidth <= 0) return 0;
  return Math.floor(pixelWidth / charWidth);
}

/**
 * Converts a pixel height and a single character row's pixel height into
 * an integer row count. Same floor-not-round reasoning as resizeToCols.
 */
export function resizeToRows(pixelHeight: number, charHeight: number): number {
  if (pixelHeight <= 0) return 0;
  return Math.floor(pixelHeight / charHeight);
}
