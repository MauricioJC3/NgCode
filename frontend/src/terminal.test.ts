import { describe, it, expect } from 'vitest';
import { clampPanelHeight, isTerminalToggleShortcut, resizeToCols, resizeToRows } from './terminal';

describe('clampPanelHeight', () => {
  it('clamps a height below min up to min', () => {
    expect(clampPanelHeight(50, 100, 600)).toBe(100);
  });

  it('clamps a height above max down to max', () => {
    expect(clampPanelHeight(900, 100, 600)).toBe(600);
  });

  it('passes an in-range height through unchanged', () => {
    expect(clampPanelHeight(300, 100, 600)).toBe(300);
  });

  it('returns the shared value when min === max', () => {
    expect(clampPanelHeight(50, 200, 200)).toBe(200);
    expect(clampPanelHeight(300, 200, 200)).toBe(200);
    expect(clampPanelHeight(200, 200, 200)).toBe(200);
  });
});

describe('isTerminalToggleShortcut', () => {
  it('returns true for Ctrl+T', () => {
    expect(isTerminalToggleShortcut({ ctrlKey: true, metaKey: false, key: 't' })).toBe(true);
  });

  it('returns true for Cmd+T (macOS metaKey)', () => {
    expect(isTerminalToggleShortcut({ ctrlKey: false, metaKey: true, key: 't' })).toBe(true);
  });

  it('returns true for Ctrl+Cmd+T (both modifiers held)', () => {
    expect(isTerminalToggleShortcut({ ctrlKey: true, metaKey: true, key: 't' })).toBe(true);
  });

  it('returns true for Ctrl+T with an uppercase key value (Shift/CapsLock)', () => {
    expect(isTerminalToggleShortcut({ ctrlKey: true, metaKey: false, key: 'T' })).toBe(true);
  });

  it('returns false for a plain t with no ctrl/meta modifier', () => {
    expect(isTerminalToggleShortcut({ ctrlKey: false, metaKey: false, key: 't' })).toBe(false);
  });

  it('returns false for Ctrl+<other key>', () => {
    expect(isTerminalToggleShortcut({ ctrlKey: true, metaKey: false, key: 'k' })).toBe(false);
  });

  it('returns false for Cmd+<other key>', () => {
    expect(isTerminalToggleShortcut({ ctrlKey: false, metaKey: true, key: 'p' })).toBe(false);
  });
});

describe('resizeToCols', () => {
  it('floors a fractional pixel-width/char-width ratio (does not round)', () => {
    // 799 / 8 = 99.875 -> floor 99, not round-to-100
    expect(resizeToCols(799, 8)).toBe(99);
  });

  it('returns an exact integer ratio unchanged', () => {
    expect(resizeToCols(800, 8)).toBe(100);
  });

  it('returns 0 for zero pixel width', () => {
    expect(resizeToCols(0, 8)).toBe(0);
  });

  it('returns 0 for negative pixel width', () => {
    expect(resizeToCols(-50, 8)).toBe(0);
  });
});

describe('resizeToRows', () => {
  it('floors a fractional pixel-height/char-height ratio (does not round)', () => {
    // 399 / 16 = 24.9375 -> floor 24, not round-to-25
    expect(resizeToRows(399, 16)).toBe(24);
  });

  it('returns an exact integer ratio unchanged', () => {
    expect(resizeToRows(400, 16)).toBe(25);
  });

  it('returns 0 for zero pixel height', () => {
    expect(resizeToRows(0, 16)).toBe(0);
  });

  it('returns 0 for negative pixel height', () => {
    expect(resizeToRows(-10, 16)).toBe(0);
  });
});
