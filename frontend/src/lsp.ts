// lsp.ts holds the pure request/response translation logic behind the
// editor's LSP integration — kept out of main.ts for the same reason as
// search.ts/update.ts/inline-edit.ts/confirm-queue.ts: main.ts builds real
// DOM and CodeMirror instances at module-load time and isn't safely
// importable in a unit test without a full jsdom + browser-API setup.
// main.ts's lspLinter/lspHover/goToDefinitionAt own the actual Wails LSP
// calls (LspCompletion/LspHover/LspDefinition) and CodeMirror wiring; these
// functions only translate between raw LSP JSON payloads and the shapes
// main.ts consumes.

import type { Text } from '@codemirror/state';
import type { Diagnostic } from '@codemirror/lint';

export interface LspDiagnostic {
  range: { start: { line: number; character: number }; end: { line: number; character: number } };
  severity?: number;
  message: string;
}

export function lspSeverityToCM(severity: number | undefined): 'error' | 'warning' | 'info' {
  if (severity === 1) return 'error';
  if (severity === 2) return 'warning';
  return 'info';
}

export function lspDiagnosticsForDoc(doc: Text, raw: LspDiagnostic[]): Diagnostic[] {
  const out: Diagnostic[] = [];
  for (const d of raw) {
    if (d.range.start.line >= doc.lines || d.range.end.line >= doc.lines) continue;
    const startLine = doc.line(d.range.start.line + 1);
    const endLine = doc.line(d.range.end.line + 1);
    const from = Math.min(startLine.from + d.range.start.character, doc.length);
    const to = Math.max(Math.min(endLine.from + d.range.end.character, doc.length), from);
    out.push({ from, to, severity: lspSeverityToCM(d.severity), message: d.message });
  }
  return out;
}

// LSP paths come back as file:// URIs; this is the frontend counterpart of the
// backend's fromFileURI. Forward slashes are left as-is (Go's os package accepts
// them on Windows too) so no OS-specific separator handling is needed here.
export function uriToPath(uri: string): string {
  let p = uri.startsWith('file://') ? uri.slice('file://'.length) : uri;
  if (p.length >= 3 && p[0] === '/' && p[2] === ':') p = p.slice(1);
  return p;
}

export type LspHoverContent = string | { kind?: string; value?: string };

export function hoverContentsToText(contents: LspHoverContent | LspHoverContent[] | undefined): string {
  if (!contents) return '';
  if (typeof contents === 'string') return contents;
  if (Array.isArray(contents)) {
    return contents.map((c) => (typeof c === 'string' ? c : c.value ?? '')).filter(Boolean).join('\n\n');
  }
  return contents.value ?? '';
}

export interface LspRange {
  start: { line: number; character: number };
  end?: { line: number; character: number };
}

export interface ParsedDefinition {
  uri: string;
  range: LspRange;
}

// parseDefinitionResponse mirrors goToDefinitionAt's inline JSON.parse +
// shape-normalization block: the response can be Location | Location[] |
// LocationLink[]; the common gopls case is a single-element array, so taking
// the first result covers it. Returns null on any parse/shape failure so
// the caller can bail out the same way the original inline code did.
export function parseDefinitionResponse(raw: string): ParsedDefinition | null {
  let parsed: any;
  try { parsed = JSON.parse(raw); } catch { return null; }
  const first = Array.isArray(parsed) ? parsed[0] : parsed;
  if (!first) return null;

  const uri: string | undefined = first.uri ?? first.targetUri;
  const range = first.range ?? first.targetSelectionRange;
  if (!uri || !range?.start) return null;

  return { uri, range };
}
