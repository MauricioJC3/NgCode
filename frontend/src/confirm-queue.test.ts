import { describe, it, expect, vi } from 'vitest';
import { enqueueOrShow, dequeueNext, type ConfirmRequest } from './confirm-queue';

// Oracle for this suite is main.ts's current (pre-extraction) askConfirm /
// closeConfirm implementation:
//
//   function askConfirm(message, opts) {
//     return new Promise((resolve) => {
//       if (confirmResolve) {
//         confirmQueue.push({ message, resolve, opts });
//         return;
//       }
//       showConfirm(message, resolve, opts);
//     });
//   }
//
//   function closeConfirm(result) {
//     confirmOverlayEl.hidden = true;
//     confirmResolve?.(result);
//     confirmResolve = null;
//     const next = confirmQueue.shift();
//     if (next) showConfirm(next.message, next.resolve, next.opts);
//   }
//
// enqueueOrShow(queue, request, isActive) captures the askConfirm decision
// (push-and-return-'queued' when a dialog is already active, vs.
// leave-the-queue-alone-and-return-'show-now' otherwise) and dequeueNext(queue)
// captures closeConfirm's confirmQueue.shift().

function makeRequest(message: string, opts?: ConfirmRequest['opts']): ConfirmRequest {
  return { message, resolve: vi.fn(), opts };
}

describe('enqueueOrShow', () => {
  it('shows immediately (does not enqueue) when no dialog is active', () => {
    const queue: ConfirmRequest[] = [];
    const request = makeRequest('first');
    const result = enqueueOrShow(queue, request, false);
    expect(result).toBe('show-now');
    expect(queue).toHaveLength(0);
  });

  // Historical bug (526c408): a second askConfirm() call while one is
  // already showing used to overwrite confirmResolve and orphan the first
  // promise forever. enqueueOrShow must queue, never show-now, whenever a
  // dialog is already active.
  it('queues (does not show) a second request while one is already showing', () => {
    const queue: ConfirmRequest[] = [];
    const request = makeRequest('second');
    const result = enqueueOrShow(queue, request, true);
    expect(result).toBe('queued');
    expect(queue).toEqual([request]);
  });

  it('preserves opts (button label overrides) through the queue', () => {
    const queue: ConfirmRequest[] = [];
    const request = makeRequest('update available', { okLabel: 'Actualizar ahora', cancelLabel: 'Ahora no' });
    enqueueOrShow(queue, request, true);
    expect(queue[0].opts).toEqual({ okLabel: 'Actualizar ahora', cancelLabel: 'Ahora no' });
  });
});

describe('dequeueNext', () => {
  it('returns undefined for an empty queue', () => {
    const queue: ConfirmRequest[] = [];
    expect(dequeueNext(queue)).toBeUndefined();
  });

  it('dequeues requests in FIFO order', () => {
    const queue: ConfirmRequest[] = [];
    const first = makeRequest('first');
    const second = makeRequest('second');
    enqueueOrShow(queue, first, true);
    enqueueOrShow(queue, second, true);

    expect(dequeueNext(queue)).toBe(first);
    expect(dequeueNext(queue)).toBe(second);
    expect(dequeueNext(queue)).toBeUndefined();
  });

  // Historical bug (0306eac / 526c408 combined scenario): a window-close
  // confirm requested while an update-available confirm is still pending
  // must queue behind it and eventually resolve once dequeued — never
  // orphaned, which is what caused Go's beforeClose channel wait to hang.
  it('close-requested while an update-prompt is pending resolves via FIFO dequeue instead of hanging', async () => {
    const queue: ConfirmRequest[] = [];

    // 1. Update prompt asked first: nothing active yet, so it shows now.
    const updateShown = enqueueOrShow(queue, makeRequest('update available'), false);
    expect(updateShown).toBe('show-now');

    // 2. Window close requested while the update prompt is still showing
    //    (isActive = true because confirmResolve is still set to the
    //    update prompt's resolver): must queue, not show or drop.
    let closeResolved = false;
    const closeRequest: ConfirmRequest = {
      message: 'unsaved changes',
      resolve: () => {
        closeResolved = true;
      },
    };
    const closeQueued = enqueueOrShow(queue, closeRequest, true);
    expect(closeQueued).toBe('queued');
    expect(queue).toEqual([closeRequest]);

    // 3. Closing the update prompt dequeues the close request in FIFO
    //    order, and calling its resolver proves the promise is not
    //    orphaned.
    const next = dequeueNext(queue);
    expect(next).toBe(closeRequest);
    next?.resolve(true);
    expect(closeResolved).toBe(true);

    // 4. Nothing left queued.
    expect(dequeueNext(queue)).toBeUndefined();
  });

  it('leaves the queue empty after dequeuing the only request', () => {
    const queue: ConfirmRequest[] = [];
    enqueueOrShow(queue, makeRequest('only'), true);
    dequeueNext(queue);
    expect(queue).toHaveLength(0);
  });
});
