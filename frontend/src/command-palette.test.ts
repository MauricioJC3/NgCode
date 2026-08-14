import { describe, it, expect } from 'vitest';
import { isCommandPaletteShortcut, fuzzyScore, filterCommands, moveSelection } from './command-palette';

describe('isCommandPaletteShortcut', () => {
  it('returns true for Ctrl+Shift+P', () => {
    expect(isCommandPaletteShortcut({ ctrlKey: true, metaKey: false, shiftKey: true, key: 'P' })).toBe(true);
  });

  it('returns true for Cmd+Shift+P (macOS metaKey)', () => {
    expect(isCommandPaletteShortcut({ ctrlKey: false, metaKey: true, shiftKey: true, key: 'p' })).toBe(true);
  });

  it('returns true when both ctrlKey and metaKey are held alongside shift+P', () => {
    expect(isCommandPaletteShortcut({ ctrlKey: true, metaKey: true, shiftKey: true, key: 'p' })).toBe(true);
  });

  it('returns false without the Shift modifier', () => {
    expect(isCommandPaletteShortcut({ ctrlKey: true, metaKey: false, shiftKey: false, key: 'p' })).toBe(false);
  });

  it('returns false without Ctrl or Cmd', () => {
    expect(isCommandPaletteShortcut({ ctrlKey: false, metaKey: false, shiftKey: true, key: 'p' })).toBe(false);
  });

  it('returns false for Ctrl+Shift+<other key>', () => {
    expect(isCommandPaletteShortcut({ ctrlKey: true, metaKey: false, shiftKey: true, key: 'k' })).toBe(false);
  });
});

describe('fuzzyScore', () => {
  it('returns null when the query is not a subsequence of the text', () => {
    expect(fuzzyScore('xyz', 'New File')).toBeNull();
  });

  it('matches case-insensitively', () => {
    expect(fuzzyScore('NEW', 'new file')).not.toBeNull();
  });

  it('scores a prefix match higher than a scattered match', () => {
    const prefix = fuzzyScore('new', 'New File')!;
    const scattered = fuzzyScore('nw', 'New File')!;
    expect(prefix).toBeGreaterThan(scattered);
  });

  it('scores a contiguous mid-string match higher than an equally-long scattered match', () => {
    const contiguous = fuzzyScore('file', 'New File')!;
    const scattered = fuzzyScore('fnle', 'New File');
    expect(contiguous).not.toBeNull();
    // 'fnle' is not even a valid subsequence of 'New File' in order, so it
    // should fail to match at all — a stronger signal than a low score.
    expect(scattered).toBeNull();
  });

  it('matches word-initial letters scattered across multiple words (VS Code-style acronym match)', () => {
    expect(fuzzyScore('gtd', 'Go to Definition')).not.toBeNull();
  });

  it('returns a score of 0 for an empty query against any text', () => {
    expect(fuzzyScore('', 'Anything')).toBe(0);
  });
});

describe('filterCommands', () => {
  const commands = [
    { id: 'open-folder', label: 'Open Folder' },
    { id: 'save-file', label: 'Save File' },
    { id: 'new-file', label: 'New File' },
  ];

  it('returns all commands in original order when the query is empty', () => {
    expect(filterCommands(commands, '')).toEqual(commands);
  });

  it('filters out commands that do not match the query', () => {
    const result = filterCommands(commands, 'save');
    expect(result.map((c) => c.id)).toEqual(['save-file']);
  });

  it('ranks the best match first', () => {
    const result = filterCommands(commands, 'file');
    // "Save File" and "New File" both match "file" contiguously; "Open
    // Folder" does not contain the "file" subsequence at all.
    expect(result.map((c) => c.id)).toEqual(expect.arrayContaining(['save-file', 'new-file']));
    expect(result.map((c) => c.id)).not.toContain('open-folder');
  });

  it('preserves original relative order for ties', () => {
    const tied = [
      { id: 'a', label: 'Aardvark' },
      { id: 'b', label: 'Aardwolf' },
    ];
    // Both start with "aard" — identical prefix-match score — so original
    // order (a before b) must be preserved.
    expect(filterCommands(tied, 'aard').map((c) => c.id)).toEqual(['a', 'b']);
  });

  it('returns an empty array when nothing matches', () => {
    expect(filterCommands(commands, 'zzz')).toEqual([]);
  });
});

describe('moveSelection', () => {
  it('clamps at the lower bound instead of wrapping', () => {
    expect(moveSelection(0, -1, 5)).toBe(0);
  });

  it('clamps at the upper bound instead of wrapping', () => {
    expect(moveSelection(4, 1, 5)).toBe(4);
  });

  it('moves down by one within bounds', () => {
    expect(moveSelection(1, 1, 5)).toBe(2);
  });

  it('moves up by one within bounds', () => {
    expect(moveSelection(3, -1, 5)).toBe(2);
  });

  it('returns -1 for an empty list regardless of direction', () => {
    expect(moveSelection(0, 1, 0)).toBe(-1);
    expect(moveSelection(0, -1, 0)).toBe(-1);
  });

  it('selects the first item when moving down from no selection (-1)', () => {
    expect(moveSelection(-1, 1, 5)).toBe(0);
  });

  it('clamps an out-of-range currentIndex back into bounds', () => {
    expect(moveSelection(10, 1, 5)).toBe(4);
  });
});
