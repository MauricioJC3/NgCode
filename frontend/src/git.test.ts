import { describe, it, expect } from 'vitest';
import { gitBadgeClass, gitDiffMarkers, folderGitBadgeClass } from './git';

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

describe('folderGitBadgeClass', () => {
  it('returns "changed" when a direct child file is non-clean', () => {
    const statuses = { '/repo/src/main.ts': ' M' };
    expect(folderGitBadgeClass(statuses, '/repo/src')).toBe('changed');
  });

  it('returns "changed" when a descendant two levels deep is non-clean', () => {
    const statuses = { '/repo/src/sub/deep/file.ts': ' M' };
    expect(folderGitBadgeClass(statuses, '/repo/src')).toBe('changed');
  });

  it('returns null when no descendant appears in the status map', () => {
    const statuses = { '/repo/other/file.ts': ' M' };
    expect(folderGitBadgeClass(statuses, '/repo/src')).toBeNull();
  });

  it('returns null for an empty status map', () => {
    expect(folderGitBadgeClass({}, '/repo/src')).toBeNull();
  });

  it('matches descendants using a backslash path separator (Windows)', () => {
    const statuses = { 'C:\\repo\\src\\sub\\file.ts': ' M' };
    expect(folderGitBadgeClass(statuses, 'C:\\repo\\src')).toBe('changed');
  });

  it('does not match a sibling folder whose name is a string-prefix of the folder path ("src" vs "src-utils")', () => {
    const statuses = { '/repo/src-utils/file.ts': ' M' };
    expect(folderGitBadgeClass(statuses, '/repo/src')).toBeNull();
  });

  it("does not count the folder's own literal path as a descendant", () => {
    const statuses = { '/repo/src': ' M' };
    expect(folderGitBadgeClass(statuses, '/repo/src')).toBeNull();
  });
});
