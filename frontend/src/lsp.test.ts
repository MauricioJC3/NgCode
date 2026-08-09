import { describe, it, expect } from 'vitest';
import { Text } from '@codemirror/state';
import {
  lspSeverityToCM, lspDiagnosticsForDoc, uriToPath, hoverContentsToText, parseDefinitionResponse,
  type LspDiagnostic,
} from './lsp';

// Oracle for this suite is main.ts's current (pre-extraction) inline
// implementations of these 5 functions:
//
//   function lspSeverityToCM(severity) {
//     if (severity === 1) return 'error';
//     if (severity === 2) return 'warning';
//     return 'info';
//   }
//
//   function lspDiagnosticsForDoc(doc, raw) {
//     const out = [];
//     for (const d of raw) {
//       if (d.range.start.line >= doc.lines || d.range.end.line >= doc.lines) continue;
//       const startLine = doc.line(d.range.start.line + 1);
//       const endLine = doc.line(d.range.end.line + 1);
//       const from = Math.min(startLine.from + d.range.start.character, doc.length);
//       const to = Math.max(Math.min(endLine.from + d.range.end.character, doc.length), from);
//       out.push({ from, to, severity: lspSeverityToCM(d.severity), message: d.message });
//     }
//     return out;
//   }
//
//   function uriToPath(uri) {
//     let p = uri.startsWith('file://') ? uri.slice('file://'.length) : uri;
//     if (p.length >= 3 && p[0] === '/' && p[2] === ':') p = p.slice(1);
//     return p;
//   }
//
//   function hoverContentsToText(contents) {
//     if (!contents) return '';
//     if (typeof contents === 'string') return contents;
//     if (Array.isArray(contents)) {
//       return contents.map((c) => (typeof c === 'string' ? c : c.value ?? '')).filter(Boolean).join('\n\n');
//     }
//     return contents.value ?? '';
//   }
//
//   // inside goToDefinitionAt:
//   let parsed;
//   try { parsed = JSON.parse(raw); } catch { return; }
//   const first = Array.isArray(parsed) ? parsed[0] : parsed;
//   if (!first) return;
//   const uri = first.uri ?? first.targetUri;
//   const range = first.range ?? first.targetSelectionRange;
//   if (!uri || !range?.start) return;

describe('lspSeverityToCM', () => {
  it('maps severity 1 to error', () => {
    expect(lspSeverityToCM(1)).toBe('error');
  });

  it('maps severity 2 to warning', () => {
    expect(lspSeverityToCM(2)).toBe('warning');
  });

  it('maps any other severity to info', () => {
    expect(lspSeverityToCM(3)).toBe('info');
    expect(lspSeverityToCM(4)).toBe('info');
  });

  it('maps undefined severity to info', () => {
    expect(lspSeverityToCM(undefined)).toBe('info');
  });
});

describe('lspDiagnosticsForDoc', () => {
  const doc = Text.of(['line one', 'line two', 'line three']);

  it('converts a well-formed diagnostic range into CM from/to offsets', () => {
    const raw: LspDiagnostic[] = [
      { range: { start: { line: 0, character: 2 }, end: { line: 0, character: 6 } }, severity: 1, message: 'boom' },
    ];
    const out = lspDiagnosticsForDoc(doc, raw);
    expect(out).toEqual([{ from: 2, to: 6, severity: 'error', message: 'boom' }]);
  });

  it('drops diagnostics whose start line is out of range', () => {
    const raw: LspDiagnostic[] = [
      { range: { start: { line: 99, character: 0 }, end: { line: 99, character: 1 } }, severity: 2, message: 'oob' },
    ];
    expect(lspDiagnosticsForDoc(doc, raw)).toEqual([]);
  });

  it('drops diagnostics whose end line is out of range', () => {
    const raw: LspDiagnostic[] = [
      { range: { start: { line: 0, character: 0 }, end: { line: 99, character: 1 } }, severity: 2, message: 'oob-end' },
    ];
    expect(lspDiagnosticsForDoc(doc, raw)).toEqual([]);
  });

  it('clamps from/to to doc.length when character exceeds line bounds', () => {
    const raw: LspDiagnostic[] = [
      { range: { start: { line: 2, character: 999 }, end: { line: 2, character: 999 } }, severity: undefined, message: 'clamp' },
    ];
    const out = lspDiagnosticsForDoc(doc, raw);
    expect(out).toEqual([{ from: doc.length, to: doc.length, severity: 'info', message: 'clamp' }]);
  });

  it('processes multiple diagnostics preserving order', () => {
    const raw: LspDiagnostic[] = [
      { range: { start: { line: 0, character: 0 }, end: { line: 0, character: 4 } }, severity: 1, message: 'first' },
      { range: { start: { line: 1, character: 0 }, end: { line: 1, character: 4 } }, severity: 2, message: 'second' },
    ];
    const out = lspDiagnosticsForDoc(doc, raw);
    expect(out.map((d) => d.message)).toEqual(['first', 'second']);
  });
});

describe('uriToPath', () => {
  it('strips the file:// prefix from a unix-style path', () => {
    expect(uriToPath('file:///home/user/project/main.go')).toBe('/home/user/project/main.go');
  });

  it('strips the leading slash before a Windows drive letter', () => {
    expect(uriToPath('file:///C:/Users/dev/project/main.go')).toBe('C:/Users/dev/project/main.go');
  });

  it('returns the input unchanged when it has no file:// prefix', () => {
    expect(uriToPath('/already/a/path')).toBe('/already/a/path');
  });
});

describe('hoverContentsToText', () => {
  it('returns empty string for undefined contents', () => {
    expect(hoverContentsToText(undefined)).toBe('');
  });

  it('returns the string as-is when contents is a plain string', () => {
    expect(hoverContentsToText('some hover text')).toBe('some hover text');
  });

  it('extracts value from a single MarkupContent object', () => {
    expect(hoverContentsToText({ kind: 'markdown', value: 'the value' })).toBe('the value');
  });

  it('returns empty string when a single MarkupContent object has no value', () => {
    expect(hoverContentsToText({ kind: 'markdown' })).toBe('');
  });

  it('joins an array of strings and MarkupContent objects with double newline, dropping empties', () => {
    const out = hoverContentsToText(['first', { value: 'second' }, { value: '' }, '']);
    expect(out).toBe('first\n\nsecond');
  });
});

describe('parseDefinitionResponse', () => {
  it('parses a single Location object', () => {
    const raw = JSON.stringify({
      uri: 'file:///home/user/main.go',
      range: { start: { line: 5, character: 2 }, end: { line: 5, character: 8 } },
    });
    expect(parseDefinitionResponse(raw)).toEqual({
      uri: 'file:///home/user/main.go',
      range: { start: { line: 5, character: 2 }, end: { line: 5, character: 8 } },
    });
  });

  it('takes the first element when the response is an array of Locations', () => {
    const raw = JSON.stringify([
      { uri: 'file:///a.go', range: { start: { line: 1, character: 0 }, end: { line: 1, character: 1 } } },
      { uri: 'file:///b.go', range: { start: { line: 2, character: 0 }, end: { line: 2, character: 1 } } },
    ]);
    expect(parseDefinitionResponse(raw)).toEqual({
      uri: 'file:///a.go',
      range: { start: { line: 1, character: 0 }, end: { line: 1, character: 1 } },
    });
  });

  it('supports LocationLink shape via targetUri/targetSelectionRange', () => {
    const raw = JSON.stringify({
      targetUri: 'file:///link.go',
      targetSelectionRange: { start: { line: 3, character: 4 }, end: { line: 3, character: 9 } },
    });
    expect(parseDefinitionResponse(raw)).toEqual({
      uri: 'file:///link.go',
      range: { start: { line: 3, character: 4 }, end: { line: 3, character: 9 } },
    });
  });

  it('returns null for invalid JSON', () => {
    expect(parseDefinitionResponse('not json')).toBeNull();
  });

  it('returns null for an empty array response', () => {
    expect(parseDefinitionResponse('[]')).toBeNull();
  });

  it('returns null when uri is missing', () => {
    const raw = JSON.stringify({ range: { start: { line: 0, character: 0 } } });
    expect(parseDefinitionResponse(raw)).toBeNull();
  });

  it('returns null when range.start is missing', () => {
    const raw = JSON.stringify({ uri: 'file:///a.go', range: {} });
    expect(parseDefinitionResponse(raw)).toBeNull();
  });
});
