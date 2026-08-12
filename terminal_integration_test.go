package main

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// This file exercises StartTerminal/WriteToTerminal/KillTerminal against a
// REAL OS pty and a real shell process — not just that the code compiles
// against a fake spawn function (see terminal_test.go's fakePty-based unit
// tests for the session-map/cwd/spawn-failure logic, which deliberately
// avoid touching a real pty). Mirrors lsp_integration_test.go's
// scope/structure: a small number of permanently kept smoke tests against
// the real thing. Cross-platform: go-pty abstracts the spawn itself, and
// only the OS-specific *teardown* mechanism is build-tag gated
// (terminal_unix.go/terminal_windows.go) — the happy path exercised here
// is not.

// terminalIntegrationTimeout bounds how long these tests wait for output
// to arrive on the real pty before failing, instead of hanging forever if
// something regresses.
const terminalIntegrationTimeout = 5 * time.Second

func TestTerminalRealPtyHappyPath(t *testing.T) {
	app := &App{}

	dir := t.TempDir()
	id, err := app.StartTerminal(dir)
	if err != nil {
		t.Fatalf("StartTerminal: unexpected error: %v", err)
	}
	if id == "" {
		t.Fatal("StartTerminal returned an empty id")
	}

	// terminalEmit (terminal.go) is what StartTerminal's read-loop
	// goroutine already calls internally for every chunk of pty output —
	// swap it here to observe that output directly, instead of requiring
	// a real Wails runtime context (mirrors search.go's injected-emit
	// pattern in spirit; here the injection point is a package var since
	// the goroutine is spawned inside StartTerminal itself).
	origEmit := terminalEmit
	var mu sync.Mutex
	var received strings.Builder
	gotMarker := make(chan struct{}, 1)
	const marker = "ngcode-terminal-smoke-test-marker"
	terminalEmit = func(a *App, event string, payload interface{}) {
		out, ok := payload.(TerminalOutput)
		if !ok || event != "terminal:output" {
			return
		}
		mu.Lock()
		received.WriteString(out.Data)
		found := strings.Contains(received.String(), marker)
		mu.Unlock()
		if found {
			select {
			case gotMarker <- struct{}{}:
			default:
			}
		}
	}
	t.Cleanup(func() { terminalEmit = origEmit })

	if err := app.WriteToTerminal(id, "echo "+marker+"\n"); err != nil {
		t.Fatalf("WriteToTerminal: unexpected error: %v", err)
	}

	select {
	case <-gotMarker:
	case <-time.After(terminalIntegrationTimeout):
		mu.Lock()
		got := received.String()
		mu.Unlock()
		t.Fatalf("timed out waiting for echoed marker on the real pty; received so far: %q", got)
	}

	if err := app.ResizeTerminal(id, 100, 40); err != nil {
		t.Errorf("ResizeTerminal: unexpected error on a live session: %v", err)
	}

	if err := app.KillTerminal(id); err != nil {
		t.Fatalf("KillTerminal: unexpected error: %v", err)
	}

	app.terminalMu.Lock()
	_, stillThere := app.terminals[id]
	app.terminalMu.Unlock()
	if stillThere {
		t.Error("expected session removed from app.terminals after KillTerminal")
	}
	// KillTerminal returning nil here means killTerminalSession's
	// OS-specific implementation (terminal_unix.go/terminal_windows.go)
	// already raced cmd.Wait() internally and only returned once the shell
	// process actually exited — no separate assertion needed beyond that.
}
