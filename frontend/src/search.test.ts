import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  debounce,
  nextGeneration,
  isCurrentGeneration,
  groupMatchesByFile,
  type SearchSession,
  type SearchMatch,
} from './search';

describe('debounce', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('does not call the wrapped function before the interval elapses', () => {
    const fn = vi.fn();
    const debounced = debounce(fn, 300);

    debounced('a');
    vi.advanceTimersByTime(100);

    expect(fn).not.toHaveBeenCalled();
  });

  it('calls the wrapped function once the interval elapses', () => {
    const fn = vi.fn();
    const debounced = debounce(fn, 300);

    debounced('a');
    vi.advanceTimersByTime(300);

    expect(fn).toHaveBeenCalledTimes(1);
    expect(fn).toHaveBeenCalledWith('a');
  });

  it('cancels and resets the timer on repeated calls before the interval elapses', () => {
    const fn = vi.fn();
    const debounced = debounce(fn, 300);

    debounced('a');
    vi.advanceTimersByTime(200);
    debounced('b');
    vi.advanceTimersByTime(200);
    expect(fn).not.toHaveBeenCalled();

    vi.advanceTimersByTime(100);
    expect(fn).toHaveBeenCalledTimes(1);
    expect(fn).toHaveBeenCalledWith('b');
  });
});

describe('SearchSession generation helpers', () => {
  it('nextGeneration increments and returns the new generation', () => {
    const session: SearchSession = { generation: 0 };
    const gen1 = nextGeneration(session);
    expect(gen1).toBe(1);
    expect(session.generation).toBe(1);

    const gen2 = nextGeneration(session);
    expect(gen2).toBe(2);
    expect(session.generation).toBe(2);
  });

  it('isCurrentGeneration is true only for the session current generation', () => {
    const session: SearchSession = { generation: 3 };
    expect(isCurrentGeneration(session, 3)).toBe(true);
    expect(isCurrentGeneration(session, 2)).toBe(false);
    expect(isCurrentGeneration(session, 4)).toBe(false);
  });

  it('a stale generation is discarded after a newer one starts', () => {
    const session: SearchSession = { generation: 0 };
    const gen1 = nextGeneration(session);
    const gen2 = nextGeneration(session);

    expect(isCurrentGeneration(session, gen1)).toBe(false);
    expect(isCurrentGeneration(session, gen2)).toBe(true);
  });
});

describe('groupMatchesByFile', () => {
  it('groups matches by path', () => {
    const matches: SearchMatch[] = [
      { path: 'a.ts', line: 1, column: 0, preview: 'foo' },
      { path: 'b.ts', line: 2, column: 3, preview: 'foo bar' },
      { path: 'a.ts', line: 5, column: 1, preview: 'foo baz' },
    ];

    const grouped = groupMatchesByFile(matches);

    expect(grouped.get('a.ts')).toEqual([
      { path: 'a.ts', line: 1, column: 0, preview: 'foo' },
      { path: 'a.ts', line: 5, column: 1, preview: 'foo baz' },
    ]);
    expect(grouped.get('b.ts')).toEqual([
      { path: 'b.ts', line: 2, column: 3, preview: 'foo bar' },
    ]);
  });

  it('preserves first-seen file order', () => {
    const matches: SearchMatch[] = [
      { path: 'z.ts', line: 1, column: 0, preview: 'x' },
      { path: 'a.ts', line: 1, column: 0, preview: 'x' },
      { path: 'z.ts', line: 2, column: 0, preview: 'x' },
      { path: 'm.ts', line: 1, column: 0, preview: 'x' },
    ];

    const grouped = groupMatchesByFile(matches);

    expect([...grouped.keys()]).toEqual(['z.ts', 'a.ts', 'm.ts']);
  });

  it('returns an empty map for no matches', () => {
    expect(groupMatchesByFile([]).size).toBe(0);
  });
});
