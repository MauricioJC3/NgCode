import './style.css';
import './app.css';

import { EditorView, keymap, type ViewUpdate, hoverTooltip, type Tooltip } from '@codemirror/view';
import { EditorState, type Text } from '@codemirror/state';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { tags } from '@lezer/highlight';
import { basicSetup } from 'codemirror';
import { javascript } from '@codemirror/lang-javascript';
import { go } from '@codemirror/lang-go';
import { json } from '@codemirror/lang-json';
import { css } from '@codemirror/lang-css';
import { markdown } from '@codemirror/lang-markdown';
import { autocompletion, type CompletionContext, type CompletionResult, type Completion } from '@codemirror/autocomplete';
import { linter, forceLinting, type Diagnostic } from '@codemirror/lint';
import { search, searchKeymap } from '@codemirror/search';
import {
  OpenFolder, ReadDir, ReadFile, SaveFile, ForceQuit, CreateFile, CreateFolder, MoveEntry,
  LspEnsureStarted, LspStopAll, LspDidOpen, LspDidChange, LspCompletion, LspDefinition, LspHover,
  UpdateAccept, UpdateDismiss,
} from '../wailsjs/go/main/App';
import type { main } from '../wailsjs/go/models';
import { EventsOn } from '../wailsjs/runtime/runtime';
import { handleUpdateAvailable, type UpdateInfo } from './update';

const ICON_CHEVRON = '<svg class="chev icon-sm" viewBox="0 0 12 12" fill="none"><path d="M4 2.5 8 6l-4 3.5" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/></svg>';
const ICON_FOLDER = '<svg class="ico icon-sm" viewBox="0 0 16 16" fill="none"><path d="M2 4.2A1 1 0 0 1 3 3.2h2.6l1 1.2H13a1 1 0 0 1 1 1v7.4a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V4.2Z" stroke="currentColor" stroke-width="1.2"/></svg>';
const ICON_FILE = '<svg class="ico icon-sm" viewBox="0 0 16 16" fill="none"><path d="M4.5 2h5L12.5 5v9a.6.6 0 0 1-.6.6h-7.4a.6.6 0 0 1-.6-.6V2.6a.6.6 0 0 1 .6-.6Z" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/><path d="M9.5 2v3h3" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/></svg>';
const ICON_CLOSE = '<svg viewBox="0 0 12 12" fill="none"><path d="M3 3l6 6M9 3l-6 6" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/></svg>';
const ICON_NEW_FILE = '<svg class="ico icon-sm" viewBox="0 0 16 16" fill="none"><path d="M4.5 2h5L12.5 5v9a.6.6 0 0 1-.6.6h-7.4a.6.6 0 0 1-.6-.6V2.6a.6.6 0 0 1 .6-.6Z" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/><path d="M9.5 2v3h3" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/><path d="M6.3 10.9h3.4M8 9.2v3.4" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/></svg>';
const ICON_NEW_FOLDER = '<svg class="ico icon-sm" viewBox="0 0 16 16" fill="none"><path d="M2 4.2A1 1 0 0 1 3 3.2h2.6l1 1.2H13a1 1 0 0 1 1 1v7.4a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V4.2Z" stroke="currentColor" stroke-width="1.2"/><path d="M6.3 9.6h3.4M8 7.9v3.4" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/></svg>';

document.querySelector<HTMLDivElement>('#app')!.innerHTML = `
  <div id="app-shell" data-ide-theme="dark">
    <header class="titlebar">
      <div class="titlebar-brand">
        <svg class="brand-mark" viewBox="0 0 20 20" fill="none" aria-hidden="true">
          <path d="M7 5 L3 10 L7 15" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/>
          <path d="M13 5 L17 10 L13 15" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        <span class="brand-word">NgCode</span>
      </div>
      <button class="title-btn" id="open-folder-btn-title" type="button" aria-label="Abrir carpeta">
        ${ICON_FOLDER}
      </button>
      <button class="title-btn" id="save-btn" type="button" aria-label="Guardar" disabled>
        <svg class="icon" viewBox="0 0 20 20" fill="none"><path d="M4 4.5A1.5 1.5 0 0 1 5.5 3h8l3.5 3.5v9a1.5 1.5 0 0 1-1.5 1.5h-10A1.5 1.5 0 0 1 4 15.5v-11Z" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/><path d="M7 3.2v4h6v-4" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/><path d="M6.5 12h7" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/></svg>
      </button>
      <div class="titlebar-crumb" id="crumb">NgCode</div>
      <button class="theme-btn title-btn" id="theme-toggle" type="button" aria-label="Cambiar tema del editor">
        <svg class="icon icon-sun" viewBox="0 0 20 20" fill="none" aria-hidden="true">
          <circle cx="10" cy="10" r="3.4" stroke="currentColor" stroke-width="1.5"/>
          <g stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M10 2.2v2"/><path d="M10 15.8v2"/><path d="M2.2 10h2"/><path d="M15.8 10h2"/>
            <path d="M4.6 4.6l1.4 1.4"/><path d="M14 14l1.4 1.4"/><path d="M15.4 4.6L14 6"/><path d="M6 14l-1.4 1.4"/>
          </g>
        </svg>
        <svg class="icon icon-moon" viewBox="0 0 20 20" fill="none" aria-hidden="true">
          <path d="M16.2 12.4A6.8 6.8 0 1 1 7.6 3.8a5.4 5.4 0 0 0 8.6 8.6Z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
        </svg>
      </button>
    </header>

    <nav class="activitybar" aria-label="Barra de actividad">
      <button class="act-btn is-active" type="button" aria-label="Explorador">
        <svg class="icon" viewBox="0 0 20 20" fill="none"><path d="M3 5.5A1.5 1.5 0 0 1 4.5 4h3l1.4 1.8H15.5A1.5 1.5 0 0 1 17 7.3v7.2A1.5 1.5 0 0 1 15.5 16h-11A1.5 1.5 0 0 1 3 14.5v-9Z" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/></svg>
      </button>
      <button class="act-btn" type="button" aria-label="Buscar">
        <svg class="icon" viewBox="0 0 20 20" fill="none"><circle cx="8.6" cy="8.6" r="4.6" stroke="currentColor" stroke-width="1.4"/><path d="M12.3 12.3 16 16" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/></svg>
      </button>
      <button class="act-btn" type="button" aria-label="Control de versiones">
        <svg class="icon" viewBox="0 0 20 20" fill="none"><circle cx="6" cy="5" r="1.7" stroke="currentColor" stroke-width="1.4"/><circle cx="6" cy="15" r="1.7" stroke="currentColor" stroke-width="1.4"/><circle cx="14" cy="10" r="1.7" stroke="currentColor" stroke-width="1.4"/><path d="M6 6.7V13.3" stroke="currentColor" stroke-width="1.4"/><path d="M6 8.5c0 2.2 1.8 3 4 3h2.3" stroke="currentColor" stroke-width="1.4"/></svg>
      </button>
      <button class="act-btn" type="button" aria-label="Extensiones">
        <svg class="icon" viewBox="0 0 20 20" fill="none"><path d="M7.5 3.8h2.2v2.1a1.1 1.1 0 0 0 1.9.8l1.5-1.5 1.5 1.5a1.1 1.1 0 0 0 1.9-.8V3.8h-2.2M7.5 3.8H4v3.5h2.1a1.1 1.1 0 0 1 .8 1.9l-1.5 1.5 1.5 1.5a1.1 1.1 0 0 1-.8 1.9H4v3.7h3.5v-2.1a1.1 1.1 0 0 1 1.9-.8l1.5 1.5" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/></svg>
      </button>
    </nav>

    <aside class="sidebar" aria-label="Explorador de archivos">
      <div class="side-title">Explorador</div>
      <div class="tree-root" id="tree-root" hidden>
        ${ICON_CHEVRON}<span id="workspace-name" class="tree-root-name"></span>
        <span class="tree-root-actions">
          <button class="tree-action-btn" id="new-file-btn" type="button" aria-label="Nuevo archivo" title="Nuevo archivo">${ICON_NEW_FILE}</button>
          <button class="tree-action-btn" id="new-folder-btn" type="button" aria-label="Nueva carpeta" title="Nueva carpeta">${ICON_NEW_FOLDER}</button>
        </span>
      </div>
      <div class="tree" id="tree"></div>
      <div class="sidebar-empty" id="sidebar-empty">
        <p>Ningún proyecto abierto</p>
        <button class="open-folder-btn" id="open-folder-btn-empty" type="button">Abrir carpeta</button>
      </div>
    </aside>

    <div class="context-menu" id="context-menu" hidden>
      <button class="context-menu-item" id="ctx-new-file" type="button">Nuevo archivo</button>
      <button class="context-menu-item" id="ctx-new-folder" type="button">Nueva carpeta</button>
    </div>

    <section class="editor-area">
      <div class="tabbar" id="tabbar" role="tablist"></div>
      <div class="editor-body">
        <div class="editor-host" id="editor-host"></div>
        <div class="empty-state" id="empty-state">
          <svg viewBox="0 0 20 20" fill="none"><path d="M4.5 2h7L15 5.5v11a.6.6 0 0 1-.6.6H4.5a.6.6 0 0 1-.6-.6V2.6a.6.6 0 0 1 .6-.6Z" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/><path d="M11 2v3.5h3.5" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/></svg>
          <p>Ningún archivo abierto</p>
          <span>Elegí un archivo del explorador para editarlo.</span>
        </div>
      </div>
    </section>

    <footer class="statusbar">
      <div class="status-left">
        <span class="status-item">
          <svg class="icon-sm" viewBox="0 0 16 16" fill="none"><circle cx="5" cy="4" r="1.4" stroke="currentColor" stroke-width="1.2"/><circle cx="5" cy="12" r="1.4" stroke="currentColor" stroke-width="1.2"/><circle cx="11.4" cy="8" r="1.4" stroke="currentColor" stroke-width="1.2"/><path d="M5 5.4V10.6" stroke="currentColor" stroke-width="1.2"/><path d="M5 6.8c0 1.8 1.4 2.4 3.2 2.4h1.8" stroke="currentColor" stroke-width="1.2"/></svg>
          main
        </span>
        <span class="status-item" id="status-cursor"></span>
      </div>
      <div class="status-right">
        <span class="status-item" id="status-lang">—</span>
        <span class="status-item">UTF-8</span>
        <span class="status-item">LF</span>
      </div>
    </footer>

    <div class="modal-overlay" id="confirm-overlay" hidden>
      <div class="modal-box" role="alertdialog" aria-modal="true" aria-labelledby="confirm-message">
        <p id="confirm-message"></p>
        <div class="modal-actions">
          <button class="modal-btn" id="confirm-cancel" type="button">Cancelar</button>
          <button class="modal-btn modal-btn-primary" id="confirm-ok" type="button">Cerrar sin guardar</button>
        </div>
      </div>
    </div>
  </div>
`;

// ---- language support ----

function extOf(path: string): string {
  const dot = path.lastIndexOf('.');
  return dot === -1 ? '' : path.slice(dot + 1).toLowerCase();
}

function langExtension(path: string) {
  switch (extOf(path)) {
    case 'go': return [go()];
    case 'ts': case 'tsx': return [javascript({ typescript: true, jsx: extOf(path) === 'tsx' })];
    case 'js': case 'jsx': return [javascript({ jsx: extOf(path) === 'jsx' })];
    case 'json': return [json()];
    case 'css': return [css()];
    case 'md': return [markdown()];
    default: return [];
  }
}

const LANGUAGE_LABELS: Record<string, string> = {
  go: 'Go', ts: 'TypeScript', tsx: 'TypeScript', js: 'JavaScript', jsx: 'JavaScript',
  json: 'JSON', css: 'CSS', md: 'Markdown',
};

function languageLabel(path: string): string {
  return LANGUAGE_LABELS[extOf(path)] ?? 'Texto plano';
}

const DOT_COLORS: Record<string, string> = {
  go: 'var(--tk-type)', ts: '#4a8fd6', tsx: '#4a8fd6', js: '#e0c341', jsx: '#e0c341',
  json: 'var(--tk-num)', css: 'var(--tk-fn)', md: 'var(--tk-str)',
};

function dotColor(path: string): string {
  return DOT_COLORS[extOf(path)] ?? 'var(--text-tertiary)';
}

// ---- editor ----

const highlightStyle = HighlightStyle.define([
  { tag: tags.keyword, color: 'var(--tk-kw)' },
  { tag: [tags.typeName, tags.standard(tags.typeName)], color: 'var(--tk-type)' },
  { tag: [tags.string, tags.special(tags.string)], color: 'var(--tk-str)' },
  { tag: tags.number, color: 'var(--tk-num)' },
  { tag: [tags.comment, tags.lineComment, tags.blockComment], color: 'var(--tk-cm)', fontStyle: 'italic' },
  { tag: [tags.function(tags.variableName), tags.function(tags.propertyName), tags.propertyName], color: 'var(--tk-fn)' },
  { tag: tags.definition(tags.variableName), color: 'var(--text-primary)' },
  { tag: tags.bracket, color: 'var(--text-secondary)' },
]);

const editorTheme = EditorView.theme({
  '&': { height: '100%', backgroundColor: 'var(--bg-void)', color: 'var(--text-primary)', fontSize: '13px' },
  '.cm-content': {
    fontFamily: 'ui-monospace, "Cascadia Code", "JetBrains Mono", "Fira Code", Consolas, "Liberation Mono", monospace',
    caretColor: 'var(--accent)',
  },
  '.cm-gutters': { backgroundColor: 'var(--bg-void)', color: 'var(--text-tertiary)', border: 'none' },
  '.cm-activeLine': { backgroundColor: 'var(--bg-surface)' },
  '.cm-activeLineGutter': { backgroundColor: 'var(--bg-surface)' },
  '.cm-selectionBackground': { backgroundColor: 'var(--accent-soft) !important' },
  '&.cm-focused .cm-selectionBackground': { backgroundColor: 'var(--accent-soft) !important' },
  '&.cm-focused': { outline: 'none' },
});

// ---- LSP (multi-language: gopls, typescript-language-server, vscode-json/css-language-server) ----

// Mirrors the Go side's languageForPath (app.go/lsp.go) — MUST stay in sync,
// since it decides when LspEnsureStarted is even attempted. Markdown is
// deliberately absent: no LSP here is worth a subprocess over CodeMirror's
// built-in word completion for it.
const LSP_LANGUAGES: Record<string, string> = {
  go: 'go',
  ts: 'typescript', tsx: 'typescript', js: 'typescript', jsx: 'typescript', mjs: 'typescript', cjs: 'typescript',
  json: 'json',
  css: 'css',
};

function lspLanguageFor(path: string): string | null {
  return LSP_LANGUAGES[extOf(path)] ?? null;
}

// lspActiveLangs tracks which languages have a running server *in this
// session* (LspEnsureStarted has already been tried for them, successfully
// or not — see openFileFromTree). Cleared on every openWorkspace(), since
// switching folders stops every server (LspStopAll) and the next one has to
// be re-started rooted at the new folder.
const lspActiveLangs = new Set<string>();

function lspActiveFor(path: string | null): boolean {
  if (!path) return false;
  const lang = lspLanguageFor(path);
  return lang !== null && lspActiveLangs.has(lang);
}

const diagnosticsByPath = new Map<string, LspDiagnostic[]>();

interface LspDiagnostic {
  range: { start: { line: number; character: number }; end: { line: number; character: number } };
  severity?: number;
  message: string;
}

function lspSeverityToCM(severity: number | undefined): 'error' | 'warning' | 'info' {
  if (severity === 1) return 'error';
  if (severity === 2) return 'warning';
  return 'info';
}

function lspDiagnosticsForDoc(doc: Text, raw: LspDiagnostic[]): Diagnostic[] {
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

const lspLinter = linter((view) => {
  if (!activePath) return [];
  const raw = diagnosticsByPath.get(activePath);
  return raw && raw.length ? lspDiagnosticsForDoc(view.state.doc, raw) : [];
});

const COMPLETION_KIND: Record<number, string> = {
  2: 'method', 3: 'function', 4: 'function', 5: 'property', 6: 'variable',
  7: 'type', 8: 'interface', 9: 'namespace', 10: 'property', 13: 'enum',
  14: 'keyword', 21: 'constant', 22: 'type',
};

async function lspCompletionSource(context: CompletionContext): Promise<CompletionResult | null> {
  if (!activePath || !lspActiveFor(activePath)) return null;
  const word = context.matchBefore(/\w*/);
  if (!word || (word.from === word.to && !context.explicit)) return null;

  const line = context.state.doc.lineAt(context.pos);
  let raw: string;
  try {
    raw = await LspCompletion(activePath, line.number - 1, context.pos - line.from);
  } catch (err) {
    console.error(err);
    return null;
  }
  if (!raw) return null;

  let parsed: any;
  try { parsed = JSON.parse(raw); } catch { return null; }
  const items: any[] = Array.isArray(parsed) ? parsed : (parsed.items ?? []);
  if (!items.length) return null;

  const options: Completion[] = items.map((item) => ({
    label: item.label,
    type: COMPLETION_KIND[item.kind] ?? 'text',
    detail: item.detail,
    apply: typeof item.insertText === 'string' ? item.insertText : item.label,
  }));

  return { from: word.from, options };
}

const lspCompletion = autocompletion({ override: [lspCompletionSource] });

// LSP paths come back as file:// URIs; this is the frontend counterpart of the
// backend's fromFileURI. Forward slashes are left as-is (Go's os package accepts
// them on Windows too) so no OS-specific separator handling is needed here.
function uriToPath(uri: string): string {
  let p = uri.startsWith('file://') ? uri.slice('file://'.length) : uri;
  if (p.length >= 3 && p[0] === '/' && p[2] === ':') p = p.slice(1);
  return p;
}

type LspHoverContent = string | { kind?: string; value?: string };

function hoverContentsToText(contents: LspHoverContent | LspHoverContent[] | undefined): string {
  if (!contents) return '';
  if (typeof contents === 'string') return contents;
  if (Array.isArray(contents)) {
    return contents.map((c) => (typeof c === 'string' ? c : c.value ?? '')).filter(Boolean).join('\n\n');
  }
  return contents.value ?? '';
}

const lspHover = hoverTooltip(async (view, pos): Promise<Tooltip | null> => {
  if (!activePath || !lspActiveFor(activePath)) return null;
  const line = view.state.doc.lineAt(pos);
  let raw: string;
  try {
    raw = await LspHover(activePath, line.number - 1, pos - line.from);
  } catch (err) {
    console.error(err);
    return null;
  }
  if (!raw) return null;

  let parsed: any;
  try { parsed = JSON.parse(raw); } catch { return null; }
  const text = hoverContentsToText(parsed?.contents);
  if (!text) return null;

  return {
    pos,
    above: true,
    create() {
      const dom = document.createElement('div');
      dom.className = 'cm-lsp-hover';
      dom.style.whiteSpace = 'pre-wrap';
      dom.style.maxWidth = '480px';
      dom.textContent = text;
      return { dom };
    },
  };
});

function placeCursorAt(view: EditorView, lspLine: number, character: number) {
  if (lspLine >= view.state.doc.lines) return;
  const line = view.state.doc.line(lspLine + 1);
  const pos = Math.min(line.from + character, line.to);
  view.dispatch({ selection: { anchor: pos }, scrollIntoView: true });
  view.focus();
}

async function goToDefinitionAt(view: EditorView, pos: number) {
  if (!activePath) return;
  const line = view.state.doc.lineAt(pos);
  let raw: string;
  try {
    raw = await LspDefinition(activePath, line.number - 1, pos - line.from);
  } catch (err) {
    console.error(err);
    return;
  }
  if (!raw) return;

  let parsed: any;
  try { parsed = JSON.parse(raw); } catch { return; }
  // Response can be Location | Location[] | LocationLink[]; the common gopls
  // case is a single-element array, so taking the first result covers it.
  const first = Array.isArray(parsed) ? parsed[0] : parsed;
  if (!first) return;

  const uri: string | undefined = first.uri ?? first.targetUri;
  const range = first.range ?? first.targetSelectionRange;
  if (!uri || !range?.start) return;

  const targetPath = uriToPath(uri);
  if (targetPath === activePath) {
    placeCursorAt(view, range.start.line, range.start.character);
    return;
  }

  await openFileFromTree(targetPath);
  placeCursorAt(editor, range.start.line, range.start.character);
}

const lspDefinitionClick = EditorView.domEventHandlers({
  mousedown(event, view) {
    if (!(event.ctrlKey || event.metaKey)) return false;
    if (!activePath || !lspActiveFor(activePath)) return false;
    const pos = view.posAtCoords({ x: event.clientX, y: event.clientY });
    if (pos == null) return false;
    event.preventDefault();
    void goToDefinitionAt(view, pos);
    return true;
  },
});

let lspChangeTimer: number | undefined;
function scheduleLspDidChange() {
  if (!activePath || !lspActiveFor(activePath)) return;
  window.clearTimeout(lspChangeTimer);
  lspChangeTimer = window.setTimeout(() => {
    if (!activePath) return;
    LspDidChange(activePath, editor.state.doc.toString()).catch((err) => console.error(err));
  }, 300);
}

EventsOn('lsp:diagnostics', (payload: { path: string; diagnostics: LspDiagnostic[] }) => {
  diagnosticsByPath.set(payload.path, payload.diagnostics ?? []);
  if (payload.path === activePath) forceLinting(editor);
});

function onEditorUpdate(update: ViewUpdate) {
  if (update.selectionSet || update.docChanged) updateCursorStatus();
  if (update.docChanged && activePath) markDirty(activePath);
  if (update.docChanged) scheduleLspDidChange();
}

function buildState(content: string, path: string): EditorState {
  const lspExtensions = lspActiveFor(path) ? [lspCompletion, lspLinter, lspHover, lspDefinitionClick] : [];
  return EditorState.create({
    doc: content,
    extensions: [
      basicSetup,
      ...langExtension(path),
      syntaxHighlighting(highlightStyle),
      editorTheme,
      EditorView.updateListener.of(onEditorUpdate),
      search({ top: true }),
      keymap.of(searchKeymap),
      ...lspExtensions,
    ],
  });
}

const editorHostEl = document.getElementById('editor-host')!;
const editor = new EditorView({ state: buildState('', ''), parent: editorHostEl });

function updateCursorStatus() {
  const pos = editor.state.selection.main.head;
  const line = editor.state.doc.lineAt(pos);
  cursorEl.textContent = `Ln ${line.number}, Col ${pos - line.from + 1}`;
}

// ---- tabs ----

interface Tab {
  path: string;
  content: string;
  dirty: boolean;
}

const tabs: Tab[] = [];
let activePath: string | null = null;

const tabbarEl = document.getElementById('tabbar')!;
const emptyStateEl = document.getElementById('empty-state')!;
const crumbEl = document.getElementById('crumb')!;
const statusLangEl = document.getElementById('status-lang')!;
const cursorEl = document.getElementById('status-cursor')!;
const saveBtn = document.getElementById('save-btn') as HTMLButtonElement;

function fileName(path: string): string {
  return path.split(/[\\/]/).pop() ?? path;
}

function dirOf(path: string): string {
  const idx = Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'));
  return idx === -1 ? '' : path.slice(0, idx);
}

function renderTabs() {
  tabbarEl.innerHTML = tabs.map((tab) => {
    const active = tab.path === activePath ? ' is-active' : '';
    const dirty = tab.dirty ? ' is-dirty' : '';
    return (
      `<div class="tab${active}${dirty}" data-path="${escapeAttr(tab.path)}" role="tab" tabindex="0">` +
        `<span class="tab-dot" style="background:${dotColor(tab.path)}"></span>` +
        `<span class="tab-name">${escapeHtml(fileName(tab.path))}</span>` +
        `<span class="tab-dirty" aria-hidden="true"></span>` +
        `<button class="tab-close" data-close="${escapeAttr(tab.path)}" aria-label="Cerrar ${escapeAttr(fileName(tab.path))}">${ICON_CLOSE}</button>` +
      `</div>`
    );
  }).join('');
}

function markDirty(path: string) {
  const tab = tabs.find((t) => t.path === path);
  if (tab && !tab.dirty) {
    tab.dirty = true;
    renderTabs();
  }
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}
function escapeAttr(s: string): string {
  return escapeHtml(s).replace(/"/g, '&quot;');
}

function syncActiveTabContent() {
  if (!activePath) return;
  const tab = tabs.find((t) => t.path === activePath);
  if (tab) tab.content = editor.state.doc.toString();
}

function showEmptyState() {
  emptyStateEl.hidden = false;
  editorHostEl.style.display = 'none';
  crumbEl.textContent = 'NgCode';
  statusLangEl.textContent = '—';
  cursorEl.textContent = '';
  saveBtn.disabled = true;
}

function activateTab(path: string) {
  syncActiveTabContent();
  activePath = path;
  const tab = tabs.find((t) => t.path === path)!;

  emptyStateEl.hidden = true;
  editorHostEl.style.display = '';
  editor.setState(buildState(tab.content, tab.path));
  updateCursorStatus();

  crumbEl.textContent = tab.path;
  statusLangEl.textContent = languageLabel(tab.path);
  saveBtn.disabled = false;

  renderTabs();
  document.querySelectorAll('.tree-row.is-file').forEach((row) => {
    row.classList.toggle('is-active', row.getAttribute('data-path') === path);
  });
}

async function openFileFromTree(path: string) {
  let tab = tabs.find((t) => t.path === path);
  if (!tab) {
    try {
      const content = await ReadFile(path);
      tab = { path, content, dirty: false };
      tabs.push(tab);
    } catch (err) {
      console.error(err);
      return;
    }

    // Lazily start path's language server on first open of that language in
    // this workspace, rather than eagerly at openWorkspace() time — avoids
    // spawning e.g. typescript-language-server for a workspace nobody edits
    // TypeScript in. buildState() (called from activateTab below) reads
    // lspActiveLangs synchronously, so this must resolve before that.
    const lang = lspLanguageFor(path);
    if (lang && workspaceRoot && !lspActiveLangs.has(lang)) {
      try {
        await LspEnsureStarted(workspaceRoot, path);
        lspActiveLangs.add(lang);
      } catch (err) {
        console.error(`lsp start failed for ${lang}, continuing without it`, err);
      }
    }
    if (lspActiveFor(path)) {
      LspDidOpen(path, tab.content).catch((err) => console.error(err));
    }
  }
  activateTab(path);
}

function closeTab(path: string) {
  const idx = tabs.findIndex((t) => t.path === path);
  if (idx === -1) return;
  const wasActive = activePath === path;
  tabs.splice(idx, 1);

  if (!wasActive) {
    renderTabs();
    return;
  }

  const next = tabs[idx] ?? tabs[idx - 1];
  activePath = null;
  if (next) {
    activateTab(next.path);
  } else {
    renderTabs();
    showEmptyState();
  }
}

async function requestCloseTab(path: string) {
  const tab = tabs.find((t) => t.path === path);
  if (!tab) return;
  if (!tab.dirty) {
    closeTab(path);
    return;
  }
  const ok = await askConfirm(`Hay cambios sin guardar en ${fileName(path)}. ¿Cerrar de todos modos?`);
  if (ok) closeTab(path);
}

tabbarEl.addEventListener('click', (e) => {
  const target = e.target as HTMLElement;
  const closeBtn = target.closest<HTMLElement>('[data-close]');
  if (closeBtn) {
    e.stopPropagation();
    void requestCloseTab(closeBtn.getAttribute('data-close')!);
    return;
  }
  const tab = target.closest<HTMLElement>('.tab');
  if (tab) activateTab(tab.getAttribute('data-path')!);
});

// ---- save ----

function saveActiveTab() {
  if (!activePath) return;
  syncActiveTabContent();
  const tab = tabs.find((t) => t.path === activePath)!;

  SaveFile(tab.path, tab.content)
    .then(() => {
      tab.dirty = false;
      renderTabs();
    })
    .catch((err) => console.error(err));
}

saveBtn.addEventListener('click', saveActiveTab);

window.addEventListener('keydown', (e) => {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
    e.preventDefault();
    saveActiveTab();
  }
});

// ---- confirm modal ----

const confirmOverlayEl = document.getElementById('confirm-overlay')!;
const confirmMessageEl = document.getElementById('confirm-message')!;
const confirmOkBtn = document.getElementById('confirm-ok') as HTMLButtonElement;
const confirmCancelBtn = document.getElementById('confirm-cancel') as HTMLButtonElement;

interface ConfirmOptions {
  okLabel?: string;
  cancelLabel?: string;
}

let confirmResolve: ((ok: boolean) => void) | null = null;
// Queue for stacked confirm requests (e.g. an update-available prompt firing
// while a window-close confirm is already pending). Without this, a second
// askConfirm() call would overwrite confirmResolve and orphan the first
// promise forever — fatal when that first promise is what unblocks Go's
// beforeClose channel wait, since the app would then hang until force-killed.
const confirmQueue: Array<{ message: string; resolve: (ok: boolean) => void; opts?: ConfirmOptions }> = [];

function showConfirm(message: string, resolve: (ok: boolean) => void, opts?: ConfirmOptions) {
  confirmMessageEl.textContent = message;
  confirmOkBtn.textContent = opts?.okLabel ?? 'Cerrar sin guardar';
  confirmCancelBtn.textContent = opts?.cancelLabel ?? 'Cancelar';
  confirmOverlayEl.hidden = false;
  confirmOkBtn.focus();
  confirmResolve = resolve;
}

// opts lets a caller override the default close-flow button labels (e.g. the
// update-available prompt uses "Actualizar ahora" / "Ahora no" instead of
// "Cerrar sin guardar" / "Cancelar" — the same modal backs both flows).
function askConfirm(message: string, opts?: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    if (confirmResolve) {
      confirmQueue.push({ message, resolve, opts });
      return;
    }
    showConfirm(message, resolve, opts);
  });
}

function closeConfirm(result: boolean) {
  confirmOverlayEl.hidden = true;
  confirmResolve?.(result);
  confirmResolve = null;

  const next = confirmQueue.shift();
  if (next) showConfirm(next.message, next.resolve, next.opts);
}

confirmOkBtn.addEventListener('click', () => closeConfirm(true));
confirmCancelBtn.addEventListener('click', () => closeConfirm(false));
confirmOverlayEl.addEventListener('click', (e) => {
  if (e.target === confirmOverlayEl) closeConfirm(false);
});
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && !confirmOverlayEl.hidden) closeConfirm(false);
});

// ---- window close ----

EventsOn('app:close-requested', () => {
  void handleCloseRequested();
});

async function handleCloseRequested() {
  const dirtyTabs = tabs.filter((t) => t.dirty);
  if (dirtyTabs.length === 0) {
    ForceQuit();
    return;
  }
  const message = dirtyTabs.length === 1
    ? `Hay cambios sin guardar en ${fileName(dirtyTabs[0].path)}. ¿Cerrar de todos modos?`
    : `Hay ${dirtyTabs.length} archivos con cambios sin guardar. ¿Cerrar de todos modos?`;
  const ok = await askConfirm(message);
  if (ok) ForceQuit();
}

// ---- auto-update ----

EventsOn('update:available', (payload: UpdateInfo) => {
  void handleUpdateAvailable(payload, {
    askConfirm,
    updateAccept: UpdateAccept,
    updateDismiss: UpdateDismiss,
  });
});

// ---- file tree ----

const treeEl = document.getElementById('tree')!;
const treeRootEl = document.getElementById('tree-root')!;
const workspaceNameEl = document.getElementById('workspace-name')!;
const sidebarEmptyEl = document.getElementById('sidebar-empty')!;

// ---- drag-and-drop move ----

function makeDraggable(row: HTMLDivElement, path: string) {
  row.draggable = true;
  row.addEventListener('dragstart', (e) => {
    e.dataTransfer?.setData('text/plain', path);
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
  });
}

function makeDropTarget(el: HTMLElement, resolveDestDir: () => string | null) {
  el.addEventListener('dragover', (e) => {
    if (!e.dataTransfer?.types.includes('text/plain')) return;
    e.preventDefault();
    el.classList.add('drop-target');
  });
  el.addEventListener('dragleave', () => el.classList.remove('drop-target'));
  el.addEventListener('drop', (e) => {
    e.preventDefault();
    el.classList.remove('drop-target');
    const srcPath = e.dataTransfer?.getData('text/plain');
    const destDir = resolveDestDir();
    if (srcPath && destDir) void moveEntryTo(srcPath, destDir);
  });
}

// Reassigns the path of any open tab affected by a move — oldPath itself
// (file moved) or anything under it (folder moved, taking its open children
// along). Without this, a moved-but-still-open file would keep saving to
// the path it no longer lives at.
function remapTabPaths(oldPath: string, newPath: string) {
  const remap = (p: string): string | null => {
    if (p === oldPath) return newPath;
    const sep = p[oldPath.length];
    if (p.startsWith(oldPath) && (sep === '/' || sep === '\\')) {
      return newPath + sep + p.slice(oldPath.length + 1);
    }
    return null;
  };

  for (const tab of tabs) {
    const remapped = remap(tab.path);
    if (remapped) tab.path = remapped;
  }
  if (activePath) {
    const remapped = remap(activePath);
    if (remapped) {
      activePath = remapped;
      crumbEl.textContent = activePath;
    }
  }
  renderTabs();
}

async function moveEntryTo(srcPath: string, destDir: string) {
  const srcParent = dirOf(srcPath);
  try {
    const newPath = await MoveEntry(srcPath, destDir);
    remapTabPaths(srcPath, newPath);
    await refreshDir(srcParent || workspaceRoot!);
    await refreshDir(destDir);
  } catch (err) {
    console.error(err);
    window.alert(`No se pudo mover: ${err}`);
  }
}

function createFileRow(entry: main.DirEntry, depth: number): HTMLDivElement {
  const row = document.createElement('div');
  row.className = 'tree-row is-file';
  row.dataset.path = entry.Path;
  row.style.setProperty('--d', String(depth));
  row.tabIndex = 0;
  row.setAttribute('role', 'button');
  row.innerHTML = ICON_FILE;
  const label = document.createElement('span');
  label.textContent = entry.Name;
  row.appendChild(label);

  const open = () => openFileFromTree(entry.Path);
  row.addEventListener('click', open);
  row.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); open(); }
  });
  makeDraggable(row, entry.Path);
  return row;
}

function createFolderRow(entry: main.DirEntry, depth: number): HTMLDivElement {
  const row = document.createElement('div');
  row.className = 'tree-row';
  row.dataset.kind = 'folder';
  row.dataset.path = entry.Path;
  row.dataset.expanded = 'false';
  row.style.setProperty('--d', String(depth));
  row.tabIndex = 0;
  row.setAttribute('role', 'button');
  row.innerHTML = `${ICON_CHEVRON}${ICON_FOLDER}`;
  const chev = row.querySelector('.chev')!;
  chev.classList.add('is-closed');
  const label = document.createElement('span');
  label.textContent = entry.Name;
  row.appendChild(label);

  const toggle = () => toggleFolder(row, entry.Path, depth);
  row.addEventListener('click', toggle);
  row.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggle(); }
  });
  makeDraggable(row, entry.Path);
  makeDropTarget(row, () => entry.Path);
  return row;
}

async function toggleFolder(row: HTMLDivElement, path: string, depth: number) {
  const chev = row.querySelector('.chev')!;
  const next = row.nextElementSibling as HTMLDivElement | null;
  const loaded = next && next.dataset.parent === path;

  if (loaded) {
    const collapsing = row.dataset.expanded === 'true';
    next!.hidden = collapsing;
    row.dataset.expanded = String(!collapsing);
    chev.classList.toggle('is-closed', collapsing);
    return;
  }

  try {
    const entries = await ReadDir(path);
    const container = document.createElement('div');
    container.dataset.parent = path;
    entries.forEach((entry) => {
      container.appendChild(entry.IsDir ? createFolderRow(entry, depth + 1) : createFileRow(entry, depth + 1));
    });
    row.after(container);
    row.dataset.expanded = 'true';
    chev.classList.remove('is-closed');
  } catch (err) {
    console.error(err);
  }
}

let workspaceRoot: string | null = null;

async function openWorkspace() {
  let root: string;
  try {
    root = await OpenFolder();
  } catch (err) {
    console.error(err);
    return;
  }
  if (!root) return;

  // Every running language server is rooted at whatever folder was open
  // when it started; switching folders makes all of them stale. Servers
  // for the new folder are started lazily as files are opened (see
  // openFileFromTree), not eagerly here.
  lspActiveLangs.clear();
  LspStopAll().catch((err) => console.error(err));

  try {
    const entries = await ReadDir(root);
    treeEl.innerHTML = '';
    entries.forEach((entry) => {
      treeEl.appendChild(entry.IsDir ? createFolderRow(entry, 0) : createFileRow(entry, 0));
    });
    workspaceRoot = root;
    workspaceNameEl.textContent = fileName(root).toUpperCase();
    treeRootEl.hidden = false;
    sidebarEmptyEl.hidden = true;
  } catch (err) {
    console.error(err);
  }
}

document.getElementById('open-folder-btn-empty')!.addEventListener('click', openWorkspace);
document.getElementById('open-folder-btn-title')!.addEventListener('click', openWorkspace);

// ---- create file / folder ----
//
// Mirrors VS Code / Zed's "new file / new folder" affordance: a pair of
// buttons in the tree-root header (scoped to the workspace root — the only
// way to add a first file to a freshly opened, still-empty folder) and a
// right-click menu on any folder row (scoped to that folder).

function findFolderRow(dirPath: string): HTMLDivElement | null {
  const rows = treeEl.querySelectorAll<HTMLDivElement>('.tree-row[data-kind="folder"]');
  for (let i = 0; i < rows.length; i++) {
    if (rows[i].dataset.path === dirPath) return rows[i];
  }
  return null;
}

async function refreshDir(dirPath: string) {
  if (dirPath === workspaceRoot) {
    const entries = await ReadDir(dirPath);
    treeEl.innerHTML = '';
    entries.forEach((entry) => {
      treeEl.appendChild(entry.IsDir ? createFolderRow(entry, 0) : createFileRow(entry, 0));
    });
    return;
  }

  const row = findFolderRow(dirPath);
  if (!row) return;
  const depth = Number(row.style.getPropertyValue('--d')) || 0;
  const next = row.nextElementSibling as HTMLDivElement | null;
  if (next && next.dataset.parent === dirPath) next.remove();

  const entries = await ReadDir(dirPath);
  const container = document.createElement('div');
  container.dataset.parent = dirPath;
  entries.forEach((entry) => {
    container.appendChild(entry.IsDir ? createFolderRow(entry, depth + 1) : createFileRow(entry, depth + 1));
  });
  row.after(container);
  row.dataset.expanded = 'true';
  row.querySelector('.chev')!.classList.remove('is-closed');
}

async function createFileIn(dirPath: string) {
  const name = window.prompt('Nombre del archivo:')?.trim();
  if (!name) return;
  try {
    const path = await CreateFile(dirPath, name);
    await refreshDir(dirPath);
    await openFileFromTree(path);
  } catch (err) {
    console.error(err);
    window.alert(`No se pudo crear el archivo: ${err}`);
  }
}

async function createFolderIn(dirPath: string) {
  const name = window.prompt('Nombre de la carpeta:')?.trim();
  if (!name) return;
  try {
    await CreateFolder(dirPath, name);
    await refreshDir(dirPath);
  } catch (err) {
    console.error(err);
    window.alert(`No se pudo crear la carpeta: ${err}`);
  }
}

document.getElementById('new-file-btn')!.addEventListener('click', (e) => {
  e.stopPropagation();
  if (workspaceRoot) void createFileIn(workspaceRoot);
});
document.getElementById('new-folder-btn')!.addEventListener('click', (e) => {
  e.stopPropagation();
  if (workspaceRoot) void createFolderIn(workspaceRoot);
});

const contextMenuEl = document.getElementById('context-menu') as HTMLDivElement;
let contextMenuDir: string | null = null;

function showContextMenu(x: number, y: number, dirPath: string) {
  contextMenuDir = dirPath;
  contextMenuEl.style.left = `${x}px`;
  contextMenuEl.style.top = `${y}px`;
  contextMenuEl.hidden = false;
}

function hideContextMenu() {
  contextMenuEl.hidden = true;
  contextMenuDir = null;
}

treeEl.addEventListener('contextmenu', (e) => {
  const row = (e.target as HTMLElement).closest<HTMLDivElement>('.tree-row[data-kind="folder"]');
  if (!row) return;
  e.preventDefault();
  showContextMenu(e.clientX, e.clientY, row.dataset.path!);
});
treeRootEl.addEventListener('contextmenu', (e) => {
  if (!workspaceRoot) return;
  e.preventDefault();
  showContextMenu(e.clientX, e.clientY, workspaceRoot);
});
makeDropTarget(treeRootEl, () => workspaceRoot);

document.getElementById('ctx-new-file')!.addEventListener('click', () => {
  const dir = contextMenuDir;
  hideContextMenu();
  if (dir) void createFileIn(dir);
});
document.getElementById('ctx-new-folder')!.addEventListener('click', () => {
  const dir = contextMenuDir;
  hideContextMenu();
  if (dir) void createFolderIn(dir);
});
document.addEventListener('click', (e) => {
  if (!contextMenuEl.hidden && !contextMenuEl.contains(e.target as Node)) hideContextMenu();
});
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && !contextMenuEl.hidden) hideContextMenu();
});

// ---- theme toggle ----

const shellEl = document.getElementById('app-shell')!;
document.getElementById('theme-toggle')!.addEventListener('click', () => {
  const next = shellEl.getAttribute('data-ide-theme') === 'dark' ? 'light' : 'dark';
  shellEl.setAttribute('data-ide-theme', next);
});

showEmptyState();
