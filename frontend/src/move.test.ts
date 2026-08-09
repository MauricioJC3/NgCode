import { describe, it, expect } from 'vitest';
import { remapPath } from './move';

// Oracle for this suite is main.ts's current (pre-extraction) remapTabPaths
// implementation:
//
//   function remapTabPaths(oldPath: string, newPath: string) {
//     const remap = (p: string): string | null => {
//       if (p === oldPath) return newPath;
//       const sep = p[oldPath.length];
//       if (p.startsWith(oldPath) && (sep === '/' || sep === '\\')) {
//         return newPath + sep + p.slice(oldPath.length + 1);
//       }
//       return null;
//     };
//     ...
//   }
//
// remapPath(oldPath, newPath, path) captures that inline `remap` closure
// verbatim, parameterized with `path` as an explicit third argument instead
// of being closed over `oldPath`/`newPath`.

describe('remapPath', () => {
  it('remaps a path that exactly matches the moved file (unix separators)', () => {
    expect(remapPath('/root/foo.ts', '/root/bar/foo.ts', '/root/foo.ts')).toBe('/root/bar/foo.ts');
  });

  it('remaps a path that exactly matches the moved file (windows separators)', () => {
    expect(remapPath('C:\\root\\foo.ts', 'C:\\root\\bar\\foo.ts', 'C:\\root\\foo.ts')).toBe(
      'C:\\root\\bar\\foo.ts',
    );
  });

  it('remaps a nested descendant path under a moved folder (unix separators)', () => {
    expect(remapPath('/root/foo', '/root/bar/foo', '/root/foo/child.ts')).toBe('/root/bar/foo/child.ts');
  });

  it('remaps a nested descendant path under a moved folder (windows separators)', () => {
    expect(remapPath('C:\\root\\foo', 'C:\\root\\bar\\foo', 'C:\\root\\foo\\child.ts')).toBe(
      'C:\\root\\bar\\foo\\child.ts',
    );
  });

  it('remaps a deeply nested descendant path, preserving the relative suffix', () => {
    expect(remapPath('/root/foo', '/root/bar/foo', '/root/foo/sub/dir/child.ts')).toBe(
      '/root/bar/foo/sub/dir/child.ts',
    );
  });

  it('leaves a false-prefix sibling path unchanged (foo2 vs moved foo)', () => {
    expect(remapPath('/root/foo', '/root/bar/foo', '/root/foo2/child.ts')).toBeNull();
  });

  it('leaves an unrelated path unchanged', () => {
    expect(remapPath('/root/foo.ts', '/root/bar/foo.ts', '/root/other.ts')).toBeNull();
  });

  it('leaves a path unchanged when it does not start with oldPath at all', () => {
    expect(remapPath('/root/foo', '/root/bar/foo', '/elsewhere/foo/child.ts')).toBeNull();
  });
});
