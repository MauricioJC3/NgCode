import { describe, it, expect, vi } from 'vitest';
import { normalizeEditValue, shouldDiscardEdit, type ActiveEdit } from './inline-edit';

describe('normalizeEditValue', () => {
  it('trims surrounding whitespace from a valid name', () => {
    expect(normalizeEditValue('  foo.ts  ')).toBe('foo.ts');
  });

  it('returns null for an empty string', () => {
    expect(normalizeEditValue('')).toBeNull();
  });

  it('returns null for a whitespace-only string', () => {
    expect(normalizeEditValue('   ')).toBeNull();
  });

  it('passes a valid name through unchanged when already trimmed', () => {
    expect(normalizeEditValue('foo.ts')).toBe('foo.ts');
  });
});

describe('shouldDiscardEdit', () => {
  it('returns false when there is no active edit', () => {
    expect(shouldDiscardEdit(null, '/src')).toBe(false);
  });

  it('returns true when the active edit dirPath matches the rebuild dirPath', () => {
    const active: ActiveEdit = { dirPath: '/src', discard: vi.fn() };
    expect(shouldDiscardEdit(active, '/src')).toBe(true);
  });

  it('returns false when the active edit dirPath differs from the rebuild dirPath', () => {
    const active: ActiveEdit = { dirPath: '/src', discard: vi.fn() };
    expect(shouldDiscardEdit(active, '/other')).toBe(false);
  });
});
