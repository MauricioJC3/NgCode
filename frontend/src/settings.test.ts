import { describe, it, expect } from 'vitest';
import { isSettingsShortcut, formatShortcut, detectIsMac, KEYBINDINGS } from './settings';

describe('isSettingsShortcut', () => {
  it('returns true for Ctrl+,', () => {
    expect(isSettingsShortcut({ ctrlKey: true, metaKey: false, key: ',' })).toBe(true);
  });

  it('returns true for Cmd+, (macOS metaKey)', () => {
    expect(isSettingsShortcut({ ctrlKey: false, metaKey: true, key: ',' })).toBe(true);
  });

  it('returns true when both ctrlKey and metaKey are held alongside comma', () => {
    expect(isSettingsShortcut({ ctrlKey: true, metaKey: true, key: ',' })).toBe(true);
  });

  it('returns false without Ctrl or Cmd', () => {
    expect(isSettingsShortcut({ ctrlKey: false, metaKey: false, key: ',' })).toBe(false);
  });

  it('returns false for Ctrl+<other key>', () => {
    expect(isSettingsShortcut({ ctrlKey: true, metaKey: false, key: 's' })).toBe(false);
  });
});

describe('formatShortcut', () => {
  it('renders Mod as Ctrl on non-macOS', () => {
    expect(formatShortcut(['Mod', 'S'], false)).toBe('Ctrl+S');
  });

  it('renders Mod as Cmd on macOS', () => {
    expect(formatShortcut(['Mod', 'S'], true)).toBe('Cmd+S');
  });

  it('joins multiple modifier keys in order', () => {
    expect(formatShortcut(['Mod', 'Shift', 'P'], false)).toBe('Ctrl+Shift+P');
  });

  it('leaves non-Mod keys untouched regardless of platform', () => {
    expect(formatShortcut(['F2'], false)).toBe('F2');
    expect(formatShortcut(['F2'], true)).toBe('F2');
  });

  it('formats a Mod+comma shortcut', () => {
    expect(formatShortcut(['Mod', ','], true)).toBe('Cmd+,');
  });
});

describe('detectIsMac', () => {
  it('detects macOS from a Macintosh user agent', () => {
    expect(detectIsMac('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)')).toBe(true);
  });

  it('detects macOS from an iPad user agent', () => {
    expect(detectIsMac('Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X)')).toBe(true);
  });

  it('returns false for a Windows user agent', () => {
    expect(detectIsMac('Mozilla/5.0 (Windows NT 10.0; Win64; x64)')).toBe(false);
  });

  it('returns false for a Linux user agent', () => {
    expect(detectIsMac('Mozilla/5.0 (X11; Linux x86_64)')).toBe(false);
  });

  it('returns false for an empty string', () => {
    expect(detectIsMac('')).toBe(false);
  });
});

describe('KEYBINDINGS', () => {
  it('includes the command palette shortcut', () => {
    const entry = KEYBINDINGS.find((k) => k.id === 'command-palette');
    expect(entry?.keys).toEqual(['Mod', 'Shift', 'P']);
  });

  it('includes the settings panel shortcut', () => {
    const entry = KEYBINDINGS.find((k) => k.id === 'open-settings');
    expect(entry?.keys).toEqual(['Mod', ',']);
  });

  it('has a non-empty label for every entry', () => {
    expect(KEYBINDINGS.every((k) => k.label.length > 0)).toBe(true);
  });
});
