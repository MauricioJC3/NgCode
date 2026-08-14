// settings.ts holds the settings/keybindings panel's pure logic — kept out
// of main.ts for the same reason as move.ts/search.ts/lsp.ts/terminal.ts/
// command-palette.ts: main.ts builds real DOM at module-load time and isn't
// safely importable in a unit test without a full jsdom + browser-API setup.

/** The subset of a KeyboardEvent isSettingsShortcut needs. */
export interface SettingsKeyEvent {
  ctrlKey: boolean;
  metaKey: boolean;
  key: string;
}

/**
 * Reports whether e is the settings panel's open shortcut: Ctrl+, on
 * Windows/Linux, Cmd+, (metaKey) on macOS — the VS Code convention for
 * opening Settings. Must fire regardless of what else currently has focus;
 * main.ts wires this at a capture-phase window keydown listener (mirroring
 * isCommandPaletteShortcut's rationale in command-palette.ts) so no focused
 * widget can swallow it first.
 */
export function isSettingsShortcut(e: SettingsKeyEvent): boolean {
  return (e.ctrlKey || e.metaKey) && e.key === ',';
}

/**
 * A single row in the read-only keybindings reference: a command label
 * paired with its key combo. `keys` uses the token 'Mod' for the
 * platform-conventional primary modifier (Ctrl on Windows/Linux, Cmd on
 * macOS) — resolved for display by formatShortcut, never hardcoded here, so
 * the same entry renders correctly on every platform.
 */
export interface KeybindingEntry {
  id: string;
  label: string;
  keys: string[];
}

/**
 * The app's read-only keybindings reference, sourced from the keydown
 * handlers actually wired in main.ts (v1 has no rebinding UI — see the
 * feature's scope boundaries). Each entry's `keys` must match the real
 * shortcut-matcher function backing it (isCommandPaletteShortcut,
 * isTerminalToggleShortcut, isSettingsShortcut, or the inline Ctrl+S/F2/
 * Delete/Escape handlers in main.ts) — this list is documentation of
 * existing behavior, not a source of new behavior.
 */
export const KEYBINDINGS: KeybindingEntry[] = [
  { id: 'save-file', label: 'Guardar archivo', keys: ['Mod', 'S'] },
  { id: 'toggle-terminal', label: 'Mostrar/ocultar terminal', keys: ['Mod', '`'] },
  { id: 'command-palette', label: 'Paleta de comandos', keys: ['Mod', 'Shift', 'P'] },
  { id: 'open-settings', label: 'Configuración', keys: ['Mod', ','] },
  { id: 'rename-entry', label: 'Renombrar (árbol de archivos)', keys: ['F2'] },
  { id: 'delete-entry', label: 'Eliminar (árbol de archivos)', keys: ['Delete'] },
  { id: 'close-dialog', label: 'Cerrar diálogo o panel', keys: ['Escape'] },
];

/**
 * Renders a keybinding's `keys` token list as a display string, e.g.
 * ['Mod', 'Shift', 'P'] -> "Ctrl+Shift+P" (or "Cmd+Shift+P" on macOS). Only
 * the 'Mod' token is platform-dependent; every other token (a literal key
 * name like 'S', 'F2', 'Escape', or ',') passes through unchanged.
 */
export function formatShortcut(keys: string[], isMac: boolean): string {
  return keys.map((key) => (key === 'Mod' ? (isMac ? 'Cmd' : 'Ctrl') : key)).join('+');
}

/**
 * Detects macOS (where the platform's conventional modifier is Cmd/metaKey
 * rather than Ctrl) from a userAgent string. Takes the string as a plain
 * argument rather than reading `navigator.userAgent` itself, so it stays a
 * pure function callable from a test without a browser environment;
 * main.ts is expected to call it as `detectIsMac(navigator.userAgent)`.
 */
export function detectIsMac(userAgent: string): boolean {
  return /Mac|iPhone|iPod|iPad/i.test(userAgent);
}
