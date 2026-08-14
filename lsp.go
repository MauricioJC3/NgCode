package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type lspClient struct {
	ctx        context.Context
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	languageID string

	writeMu sync.Mutex

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan rpcMessage

	docMu   sync.Mutex
	docVers map[string]int
}

// lspServerSpec describes how to launch the language server for one
// language: the command Wails/Go spawns (must be on PATH), its args, the
// LSP "languageId" it expects in textDocument/didOpen, and a human install
// hint surfaced when the command isn't found — mirrors the message gopls
// already gave for the Go-only version of this file.
type lspServerSpec struct {
	command     string
	args        []string
	languageID  string
	installHint string
}

// lspServers is the language -> server registry. Markdown is deliberately
// not here: there's no LSP with meaningfully better completion for it than
// CodeMirror's built-in word completion, so it stays on that instead of
// paying for a subprocess.
var lspServers = map[string]lspServerSpec{
	"go": {
		command:     "gopls",
		args:        []string{"serve"},
		languageID:  "go",
		installHint: "go install golang.org/x/tools/gopls@latest",
	},
	"typescript": {
		command:    "typescript-language-server",
		args:       []string{"--stdio"},
		languageID: "typescript",
		// typescript-language-server resolves TypeScript itself via Node
		// module resolution from the workspace root — a global-only
		// `typescript` install is NOT enough (confirmed: it fails
		// initialize with "Could not find a valid TypeScript
		// installation" even with `typescript` on PATH globally).
		// `typescript` has to be a project dependency, AND pinned to the
		// v5 line: an unpinned `npm install typescript` resolves to the
		// new Go-based "TypeScript 7" compiler, whose package no longer
		// ships lib/tsserver.js — confirmed by installing it and hitting
		// this exact initialize failure again despite being a local dep.
		installHint: "npm install -g typescript-language-server, then npm install --save-dev typescript@^5 inside the project you're editing",
	},
	"json": {
		command:     "vscode-json-language-server",
		args:        []string{"--stdio"},
		languageID:  "json",
		installHint: "npm install -g vscode-langservers-extracted",
	},
	"css": {
		command:     "vscode-css-language-server",
		args:        []string{"--stdio"},
		languageID:  "css",
		installHint: "npm install -g vscode-langservers-extracted",
	},
	"python": {
		command:     "pyright-langserver",
		args:        []string{"--stdio"},
		languageID:  "python",
		installHint: "npm install -g pyright",
	},
	"html": {
		command:     "vscode-html-language-server",
		args:        []string{"--stdio"},
		languageID:  "html",
		installHint: "npm install -g vscode-langservers-extracted",
	},
}

// languageForPath maps a file's extension to the lspServers key backing it,
// or "" if no language server is configured for that extension (markdown,
// plain text, etc.) — the frontend has an identical mapping (lspLanguageFor
// in main.ts) that MUST be kept in sync, since it decides when to even
// attempt LspEnsureStarted.
func languageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		return "typescript"
	case ".json":
		return "json"
	case ".css":
		return "css"
	case ".py":
		return "python"
	case ".html", ".htm":
		return "html"
	default:
		return ""
	}
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toFileURI/fromFileURI implement just enough of the LSP file-URI convention
// (including the Windows drive-letter case) to avoid pulling in a URI library.
func toFileURI(path string) string {
	slashed := filepath.ToSlash(path)
	if len(slashed) >= 2 && slashed[1] == ':' {
		return "file:///" + slashed
	}
	if !strings.HasPrefix(slashed, "/") {
		return "file:///" + slashed
	}
	return "file://" + slashed
}

func fromFileURI(uri string) string {
	p := strings.TrimPrefix(uri, "file://")
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = strings.TrimPrefix(p, "/")
	}
	return filepath.FromSlash(p)
}

func startLSPClient(ctx context.Context, rootPath string, spec lspServerSpec) (*lspClient, error) {
	cmd := exec.Command(spec.command, spec.args...)
	hideConsoleWindow(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s not found on PATH (install with: %s): %w", spec.command, spec.installHint, err)
	}

	c := &lspClient{
		ctx:        ctx,
		cmd:        cmd,
		stdin:      stdin,
		languageID: spec.languageID,
		pending:    make(map[int64]chan rpcMessage),
		docVers:    make(map[string]int),
	}
	go c.readLoop(bufio.NewReader(stdout))

	initParams := map[string]interface{}{
		"processId":    nil,
		"rootUri":      toFileURI(rootPath),
		"capabilities": map[string]interface{}{},
	}
	if _, err := c.request("initialize", initParams); err != nil {
		_ = c.stop()
		return nil, err
	}
	if err := c.notify("initialized", map[string]interface{}{}); err != nil {
		_ = c.stop()
		return nil, err
	}

	return c, nil
}

func readMessage(r *bufio.Reader) (rpcMessage, error) {
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return rpcMessage{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:")))
			if err != nil {
				return rpcMessage{}, err
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return rpcMessage{}, fmt.Errorf("lsp: missing Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return rpcMessage{}, err
	}

	var msg rpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return rpcMessage{}, err
	}
	return msg, nil
}

func (c *lspClient) readLoop(r *bufio.Reader) {
	for {
		msg, err := readMessage(r)
		if err != nil {
			return
		}

		if msg.Method == "" && msg.ID != nil {
			c.mu.Lock()
			ch, ok := c.pending[*msg.ID]
			if ok {
				delete(c.pending, *msg.ID)
			}
			c.mu.Unlock()
			if ok {
				ch <- msg
			}
			continue
		}

		if msg.Method != "" && msg.ID != nil {
			// Server-to-client request (e.g. client/registerCapability). We don't act on
			// these yet, but gopls expects a reply or it may stall waiting for one.
			c.reply(*msg.ID)
			continue
		}

		if msg.Method == "textDocument/publishDiagnostics" {
			c.forwardDiagnostics(msg.Params)
		}
	}
}

func (c *lspClient) writeMessage(v interface{}) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if _, err := fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.stdin.Write(body)
	return err
}

func (c *lspClient) request(method string, params interface{}) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	ch := make(chan rpcMessage, 1)

	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	err := c.writeMessage(struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      int64       `json:"id"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params,omitempty"`
	}{"2.0", id, method, params})
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("lsp: %s failed: %s", method, resp.Error.Message)
		}
		return resp.Result, nil
	case <-time.After(10 * time.Second):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("lsp: %s timed out", method)
	}
}

func (c *lspClient) notify(method string, params interface{}) error {
	return c.writeMessage(struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params,omitempty"`
	}{"2.0", method, params})
}

func (c *lspClient) reply(id int64) {
	_ = c.writeMessage(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int64           `json:"id"`
		Result  json.RawMessage `json:"result"`
	}{"2.0", id, json.RawMessage("null")})
}

func (c *lspClient) forwardDiagnostics(params json.RawMessage) {
	var p struct {
		URI         string          `json:"uri"`
		Diagnostics json.RawMessage `json:"diagnostics"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	runtime.EventsEmit(c.ctx, "lsp:diagnostics", map[string]interface{}{
		"path":        fromFileURI(p.URI),
		"diagnostics": p.Diagnostics,
	})
}

func (c *lspClient) didOpen(path, content string) error {
	c.docMu.Lock()
	c.docVers[path] = 1
	c.docMu.Unlock()

	return c.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        toFileURI(path),
			"languageId": c.languageID,
			"version":    1,
			"text":       content,
		},
	})
}

func (c *lspClient) didChange(path, content string) error {
	c.docMu.Lock()
	c.docVers[path]++
	version := c.docVers[path]
	c.docMu.Unlock()

	return c.notify("textDocument/didChange", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":     toFileURI(path),
			"version": version,
		},
		"contentChanges": []map[string]interface{}{
			{"text": content},
		},
	})
}

func (c *lspClient) completion(path string, line, character int) (json.RawMessage, error) {
	return c.request("textDocument/completion", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": toFileURI(path)},
		"position":     map[string]interface{}{"line": line, "character": character},
	})
}

func (c *lspClient) definition(path string, line, character int) (json.RawMessage, error) {
	return c.request("textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": toFileURI(path)},
		"position":     map[string]interface{}{"line": line, "character": character},
	})
}

func (c *lspClient) hover(path string, line, character int) (json.RawMessage, error) {
	return c.request("textDocument/hover", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": toFileURI(path)},
		"position":     map[string]interface{}{"line": line, "character": character},
	})
}

func (c *lspClient) stop() error {
	_, _ = c.request("shutdown", nil)
	_ = c.notify("exit", nil)
	_ = c.stdin.Close()

	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = c.cmd.Process.Kill()
	}
	return nil
}
