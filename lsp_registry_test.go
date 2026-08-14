package main

import "testing"

// This file unit-tests the lsp.go registry (lspServers) and its extension
// mapping (languageForPath) without spawning any real language server
// process — lsp_integration_test.go already covers the real-process smoke
// path for typescript/json/css and skips itself when the binary isn't on
// PATH. These tests just check the pure data/lookup logic, so they always
// run regardless of what's installed on the machine.

func TestLanguageForPath_Python(t *testing.T) {
	if got := languageForPath("main.py"); got != "python" {
		t.Fatalf("languageForPath(main.py) = %q, want %q", got, "python")
	}
}

func TestLanguageForPath_PythonUppercaseExtension(t *testing.T) {
	// languageForPath lowercases the extension before matching (see the
	// strings.ToLower(filepath.Ext(path)) call) — confirm python follows
	// the same case-insensitive behavior as the existing entries.
	if got := languageForPath("SCRIPT.PY"); got != "python" {
		t.Fatalf("languageForPath(SCRIPT.PY) = %q, want %q", got, "python")
	}
}

func TestLanguageForPath_HTML(t *testing.T) {
	if got := languageForPath("index.html"); got != "html" {
		t.Fatalf("languageForPath(index.html) = %q, want %q", got, "html")
	}
}

func TestLanguageForPath_HTMShortExtension(t *testing.T) {
	if got := languageForPath("index.htm"); got != "html" {
		t.Fatalf("languageForPath(index.htm) = %q, want %q", got, "html")
	}
}

func TestLanguageForPath_UnconfiguredExtensionStaysEmpty(t *testing.T) {
	// Sanity check that adding python/html didn't turn languageForPath into
	// a catch-all — an extension with no registered server must still
	// resolve to "" the way it did before this change.
	if got := languageForPath("README.rb"); got != "" {
		t.Fatalf("languageForPath(README.rb) = %q, want empty string", got)
	}
}

func TestLspServersRegistry_Python(t *testing.T) {
	spec, ok := lspServers["python"]
	if !ok {
		t.Fatal(`lspServers["python"] missing`)
	}
	if spec.command != "pyright-langserver" {
		t.Errorf("command = %q, want %q", spec.command, "pyright-langserver")
	}
	if len(spec.args) != 1 || spec.args[0] != "--stdio" {
		t.Errorf("args = %v, want [--stdio]", spec.args)
	}
	if spec.languageID != "python" {
		t.Errorf("languageID = %q, want %q", spec.languageID, "python")
	}
	if spec.installHint == "" {
		t.Error("installHint must not be empty")
	}
}

func TestLspServersRegistry_HTML(t *testing.T) {
	spec, ok := lspServers["html"]
	if !ok {
		t.Fatal(`lspServers["html"] missing`)
	}
	if spec.command != "vscode-html-language-server" {
		t.Errorf("command = %q, want %q", spec.command, "vscode-html-language-server")
	}
	if len(spec.args) != 1 || spec.args[0] != "--stdio" {
		t.Errorf("args = %v, want [--stdio]", spec.args)
	}
	if spec.languageID != "html" {
		t.Errorf("languageID = %q, want %q", spec.languageID, "html")
	}
	if spec.installHint == "" {
		t.Error("installHint must not be empty")
	}
}

// languageForPath's "python"/"html" results must be registry keys that
// actually resolve to a spec — otherwise LspEnsureStarted's lookup in
// app.go silently no-ops (see the `spec, ok := lspServers[lang]` check).
func TestLanguageForPathResolvesToRegisteredServer(t *testing.T) {
	for _, path := range []string{"main.py", "index.html"} {
		lang := languageForPath(path)
		if lang == "" {
			t.Fatalf("languageForPath(%s) returned empty language", path)
		}
		if _, ok := lspServers[lang]; !ok {
			t.Fatalf("languageForPath(%s) = %q has no entry in lspServers", path, lang)
		}
	}
}
