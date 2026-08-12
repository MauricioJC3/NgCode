import { describe, it, expect } from 'vitest';
import { gitBadgeClass, gitDiffMarkers } from './git';

describe('gitBadgeClass', () => {
  it('classifies an unstaged modified file (" M")', () => {
    expect(gitBadgeClass(' M')).toBe('modified');
  });

  it('classifies a staged modified file ("M ")', () => {
    expect(gitBadgeClass('M ')).toBe('modified');
  });

  it('classifies a file with both staged and unstaged edits ("MM")', () => {
    expect(gitBadgeClass('MM')).toBe('modified');
  });

  it('classifies a staged added file ("A ")', () => {
    expect(gitBadgeClass('A ')).toBe('added');
  });

  it('classifies an untracked file ("??")', () => {
    expect(gitBadgeClass('??')).toBe('untracked');
  });

  it('classifies an unstaged deleted file (" D")', () => {
    expect(gitBadgeClass(' D')).toBe('deleted');
  });

  it('classifies a staged deleted file ("D ")', () => {
    expect(gitBadgeClass('D ')).toBe('deleted');
  });

  it('treats a path absent from the status map (undefined) as clean — no badge', () => {
    expect(gitBadgeClass(undefined)).toBeNull();
  });
});

describe('gitDiffMarkers', () => {
  it('maps added line numbers to "added" markers', () => {
    expect(gitDiffMarkers({ added: [2, 5], modified: [], removed: [] })).toEqual([
      { line0: 2, type: 'added' },
      { line0: 5, type: 'added' },
    ]);
  });

  it('maps modified line numbers to "modified" markers', () => {
    expect(gitDiffMarkers({ added: [], modified: [1], removed: [] })).toEqual([
      { line0: 1, type: 'modified' },
    ]);
  });

  it('maps removed line numbers to "removed" markers', () => {
    expect(gitDiffMarkers({ added: [], modified: [], removed: [3] })).toEqual([
      { line0: 3, type: 'removed' },
    ]);
  });

  it('merges added/modified/removed into one list sorted by line number', () => {
    expect(gitDiffMarkers({ added: [5], modified: [1], removed: [3] })).toEqual([
      { line0: 1, type: 'modified' },
      { line0: 3, type: 'removed' },
      { line0: 5, type: 'added' },
    ]);
  });

  it('returns an empty array for an empty diff result', () => {
    expect(gitDiffMarkers({ added: [], modified: [], removed: [] })).toEqual([]);
  });
});
