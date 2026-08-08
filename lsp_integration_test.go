package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"testing"
)

// This file's three tests check startLSPClient's generalized
// initialize/initialized handshake against the REAL typescript-language-server,
// vscode-json-language-server, and vscode-css-language-server processes —
// not just that the Go code compiles. Each skips itself when its server (or
// npm, for the TypeScript one) isn't on PATH, since CI never runs `go test`
// and most contributors won't have all three installed. Kept permanently
// (not deleted after writing) because both found real bugs on first run:
// typescript-language-server needs `typescript` as a PROJECT dependency,
// not a global one, and needs it pinned to the v5 line — an unpinned
// install resolves to the new Go-based "TypeScript 7" compiler, whose
// package doesn't ship lib/tsserver.js, which typescript-language-server
// needs. See the installHint on the "typescript" entry in lsp.go's
// lspServers registry.
func TestSmokeStartLSPClientTypeScript(t *testing.T) {
	if _, err := exec.LookPath("typescript-language-server"); err != nil {
		t.Skip("typescript-language-server not on PATH")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not on PATH")
	}

	dir := t.TempDir()
	// Pinned to the v5 line: an unpinned install resolves to the new
	// Go-based "TypeScript 7" compiler, whose package doesn't ship
	// lib/tsserver.js, which typescript-language-server needs.
	install := exec.Command("npm", "install", "--no-audit", "--no-fund", "typescript@^5")
	install.Dir = dir
	if out, err := install.CombinedOutput(); err != nil {
		t.Fatalf("npm install typescript: %v\n%s", err, out)
	}

	tsPath := dir + "/main.ts"
	if err := os.WriteFile(tsPath, []byte("const greeting: string = 'hi';\nconsole.log(greeting.);\n"), 0644); err != nil {
		t.Fatalf("write main.ts: %v", err)
	}

	spec := lspServers["typescript"]
	client, err := startLSPClient(context.Background(), dir, spec)
	if err != nil {
		t.Fatalf("startLSPClient: %v", err)
	}
	defer client.stop()

	if err := client.didOpen(tsPath, "const greeting: string = 'hi';\nconsole.log(greeting.);\n"); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	// Line 1, right after "greeting." — should suggest String.prototype
	// members (length, toUpperCase, etc.) if the server is actually
	// resolving real TypeScript types, not just accepting the request.
	raw, err := client.completion(tsPath, 1, 21)
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	t.Logf("completion result: %s", raw)
	if !bytes.Contains(raw, []byte("toUpperCase")) {
		t.Fatalf("expected completion to include toUpperCase (real TS type info), got: %s", raw)
	}
}

func TestSmokeStartLSPClientJSON(t *testing.T) {
	if _, err := exec.LookPath("vscode-json-language-server"); err != nil {
		t.Skip("vscode-json-language-server not on PATH")
	}

	dir := t.TempDir()
	jsonPath := dir + "/data.json"
	content := "{\n  \"a\": 1,\n  \n}\n"

	spec := lspServers["json"]
	client, err := startLSPClient(context.Background(), dir, spec)
	if err != nil {
		t.Fatalf("startLSPClient: %v", err)
	}
	defer client.stop()

	if err := client.didOpen(jsonPath, content); err != nil {
		t.Fatalf("didOpen: %v", err)
	}
	// Line 2 (0-indexed), inside the empty second property slot — hover on
	// the existing "a" key instead, which the server can describe.
	raw, err := client.hover(jsonPath, 1, 4)
	if err != nil {
		t.Fatalf("hover: %v", err)
	}
	t.Logf("hover result: %s", raw)
}

func TestSmokeStartLSPClientCSS(t *testing.T) {
	if _, err := exec.LookPath("vscode-css-language-server"); err != nil {
		t.Skip("vscode-css-language-server not on PATH")
	}

	dir := t.TempDir()
	cssPath := dir + "/style.css"
	content := ".x { colo }\n"

	spec := lspServers["css"]
	client, err := startLSPClient(context.Background(), dir, spec)
	if err != nil {
		t.Fatalf("startLSPClient: %v", err)
	}
	defer client.stop()

	if err := client.didOpen(cssPath, content); err != nil {
		t.Fatalf("didOpen: %v", err)
	}
	// Right after "colo" — should suggest "color" among real CSS properties.
	raw, err := client.completion(cssPath, 0, 9)
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	t.Logf("completion result: %s", raw)
	if !bytes.Contains(raw, []byte(`"color"`)) {
		t.Fatalf("expected completion to include the color property, got: %s", raw)
	}
}
