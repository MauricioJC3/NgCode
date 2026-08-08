// update.ts holds the auto-update UI logic as small, dependency-injected,
// pure/testable functions — kept out of main.ts (which builds real DOM and
// CodeMirror instances at module-load time and is therefore not safely
// importable in a unit test without a full jsdom + browser-API setup).
// main.ts wires this module to the real askConfirm modal and the real
// UpdateAccept/UpdateDismiss wailsjs bindings; that wiring itself is NOT
// covered by an automated test (see apply-progress "NOT covered
// automatically" notes) — only manually verified.

// UpdateInfo mirrors the Go side's UpdateInfo struct (app.go / updater.go),
// received as the "update:available" event payload.
export interface UpdateInfo {
  version: string;
  currentVersion: string;
}

// formatUpdateMessage builds the confirmation message shown in the existing
// askConfirm modal, matching this project's Spanish UI copy convention.
export function formatUpdateMessage(info: UpdateInfo): string {
  return `Hay una actualización disponible (${info.version}). ¿Actualizar ahora?`;
}

export interface UpdateAvailableDeps {
  askConfirm: (message: string, opts?: { okLabel?: string; cancelLabel?: string }) => Promise<boolean>;
  updateAccept: () => Promise<void>;
  updateDismiss: () => Promise<void>;
}

// handleUpdateAvailable orchestrates the user-consent flow for a pending
// update: show the confirm modal, then either dismiss (no download, no
// persistence — spec: re-asks next launch) or accept (download/verify/swap
// via UpdateAccept). A rejected UpdateAccept is surfaced the same way every
// other bound-method failure is surfaced in this codebase: console.error,
// no dedicated toast/error UI (confirmed: no such component exists).
//
// The confirm modal is shared with the close flow, whose default buttons
// read "Cerrar sin guardar" / "Cancelar" — meaningless here, so this call
// overrides them with update-specific labels.
export async function handleUpdateAvailable(info: UpdateInfo, deps: UpdateAvailableDeps): Promise<void> {
  const ok = await deps.askConfirm(formatUpdateMessage(info), { okLabel: 'Actualizar ahora', cancelLabel: 'Ahora no' });
  if (!ok) {
    await deps.updateDismiss();
    return;
  }

  try {
    await deps.updateAccept();
  } catch (err) {
    console.error(err);
  }
}
