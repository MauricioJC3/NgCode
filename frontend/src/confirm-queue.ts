// confirm-queue.ts holds the pure state-machine logic behind the app's
// confirm-modal queue — kept out of main.ts for the same reason as
// search.ts/update.ts/inline-edit.ts: main.ts builds real DOM at module-load
// time and isn't safely importable in a unit test without a full jsdom +
// browser-API setup. main.ts's askConfirm/closeConfirm own the actual
// `confirmResolve`/`confirmQueue` state and the DOM-touching showConfirm
// call; these two functions only decide what should happen next given that
// state, matching inline-edit.ts's shouldDiscardEdit(active, dirPath)
// param-passing convention (no class, no module-owned mutable state here).
//
// This queue exists because a second askConfirm() call while one dialog is
// already showing would otherwise overwrite confirmResolve and orphan the
// first promise forever — historically fatal, since that first promise was
// what unblocked Go's beforeClose channel wait (fixed by 526c408, then by
// 0306eac for the close-button case specifically).

export interface ConfirmOptions {
  okLabel?: string;
  cancelLabel?: string;
}

export interface ConfirmRequest {
  message: string;
  resolve: (ok: boolean) => void;
  opts?: ConfirmOptions;
}

// enqueueOrShow decides what an askConfirm() call should do next: if a
// dialog is already active (isActive, i.e. confirmResolve !== null in
// main.ts), the request is pushed onto the queue and the caller must NOT
// show it yet ('queued'); otherwise the queue is left untouched and the
// caller should show the request immediately ('show-now').
export function enqueueOrShow(
  queue: ConfirmRequest[],
  request: ConfirmRequest,
  isActive: boolean,
): 'queued' | 'show-now' {
  if (isActive) {
    queue.push(request);
    return 'queued';
  }
  return 'show-now';
}

// dequeueNext mirrors closeConfirm's confirmQueue.shift() — returns the next
// queued request in FIFO order, or undefined when the queue is empty (in
// which case main.ts's closeConfirm leaves the modal hidden and
// confirmResolve null).
export function dequeueNext(queue: ConfirmRequest[]): ConfirmRequest | undefined {
  return queue.shift();
}
