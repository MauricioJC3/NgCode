import { describe, it, expect, vi } from 'vitest';
import { formatUpdateMessage, handleUpdateAvailable, type UpdateInfo } from './update';

const info: UpdateInfo = { version: 'v1.2.0', currentVersion: 'v1.1.0' };

describe('formatUpdateMessage', () => {
  it('includes the new version in the confirmation message', () => {
    const msg = formatUpdateMessage(info);
    expect(msg).toContain('v1.2.0');
    expect(msg).toMatch(/actualización disponible/i);
  });
});

describe('handleUpdateAvailable', () => {
  it('calls updateAccept when the user accepts', async () => {
    const askConfirm = vi.fn().mockResolvedValue(true);
    const updateAccept = vi.fn().mockResolvedValue(undefined);
    const updateDismiss = vi.fn().mockResolvedValue(undefined);

    await handleUpdateAvailable(info, { askConfirm, updateAccept, updateDismiss });

    expect(askConfirm).toHaveBeenCalledWith(formatUpdateMessage(info));
    expect(updateAccept).toHaveBeenCalledOnce();
    expect(updateDismiss).not.toHaveBeenCalled();
  });

  it('calls updateDismiss and never downloads when the user declines', async () => {
    const askConfirm = vi.fn().mockResolvedValue(false);
    const updateAccept = vi.fn();
    const updateDismiss = vi.fn().mockResolvedValue(undefined);

    await handleUpdateAvailable(info, { askConfirm, updateAccept, updateDismiss });

    expect(updateDismiss).toHaveBeenCalledOnce();
    expect(updateAccept).not.toHaveBeenCalled();
  });

  it('logs the error via console.error when updateAccept fails', async () => {
    const err = new Error('checksum mismatch');
    const askConfirm = vi.fn().mockResolvedValue(true);
    const updateAccept = vi.fn().mockRejectedValue(err);
    const updateDismiss = vi.fn();
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    await handleUpdateAvailable(info, { askConfirm, updateAccept, updateDismiss });

    expect(consoleErrorSpy).toHaveBeenCalledWith(err);
    consoleErrorSpy.mockRestore();
  });
});
