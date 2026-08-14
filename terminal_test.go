package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/aymanbagabas/go-pty"
)

// fakePty is a no-op pty.Pty used to exercise StartTerminal's session-map
// bookkeeping, cwd resolution, and spawn-failure handling without
// allocating a real OS pty or spawning a real shell process — that's
// covered by terminal_integration_test.go in Phase 2. Read returns EOF
// immediately so the read-loop goroutine spawned by StartTerminal exits
// right away instead of blocking for the lifetime of the test.
type fakePty struct {
	closed bool
}

func (f *fakePty) Read(b []byte) (int, error)  { return 0, io.EOF }
func (f *fakePty) Write(b []byte) (int, error) { return len(b), nil }
func (f *fakePty) Close() error                { f.closed = true; return nil }
func (f *fakePty) Name() string                { return "fake-pty" }
func (f *fakePty) Command(name string, args ...string) *pty.Cmd {
	return nil
}
func (f *fakePty) CommandContext(ctx context.Context, name string, args ...string) *pty.Cmd {
	return nil
}
func (f *fakePty) Resize(width int, height int) error { return nil }
func (f *fakePty) Fd() uintptr                         { return 0 }

// withFakeSpawn swaps terminalSpawnFunc for the duration of the test,
// restoring the original (production) value afterward.
func withFakeSpawn(t *testing.T, spawn func(shell, cwd string) (pty.Pty, *pty.Cmd, error)) {
	t.Helper()
	orig := terminalSpawnFunc
	terminalSpawnFunc = spawn
	t.Cleanup(func() { terminalSpawnFunc = orig })
}

// withFakeKill swaps terminalKillFunc for the duration of the test,
// restoring the original (production, OS-specific) value afterward.
func withFakeKill(t *testing.T, kill func(sess *terminalSession) error) {
	t.Helper()
	orig := terminalKillFunc
	terminalKillFunc = kill
	t.Cleanup(func() { terminalKillFunc = orig })
}

// --- StartTerminal / session map bookkeeping --------------------------------

func TestStartTerminalSessionBookkeeping(t *testing.T) {
	withFakeSpawn(t, func(shell, cwd string) (pty.Pty, *pty.Cmd, error) {
		return &fakePty{}, nil, nil
	})
	withFakeKill(t, func(sess *terminalSession) error { return nil })

	app := &App{}
	id, err := app.StartTerminal("")
	if err != nil {
		t.Fatalf("StartTerminal: unexpected error: %v", err)
	}
	if id == "" {
		t.Fatal("StartTerminal returned an empty id")
	}

	app.terminalMu.Lock()
	_, ok := app.terminals[id]
	app.terminalMu.Unlock()
	if !ok {
		t.Fatalf("expected session %q registered under app.terminals", id)
	}

	if err := app.WriteToTerminal("does-not-exist", "x"); err == nil {
		t.Error("WriteToTerminal: expected error for unknown id, got nil")
	}
	if err := app.ResizeTerminal("does-not-exist", 80, 24); err == nil {
		t.Error("ResizeTerminal: expected error for unknown id, got nil")
	}
	if err := app.KillTerminal("does-not-exist"); err == nil {
		t.Error("KillTerminal: expected error for unknown id, got nil")
	}

	if err := app.KillTerminal(id); err != nil {
		t.Fatalf("KillTerminal: unexpected error: %v", err)
	}
	app.terminalMu.Lock()
	_, stillThere := app.terminals[id]
	app.terminalMu.Unlock()
	if stillThere {
		t.Errorf("expected session %q removed from app.terminals after KillTerminal", id)
	}
}

// --- StartTerminal / reuse existing session ----------------------------------

func TestStartTerminalReusesExistingSession(t *testing.T) {
	var calls int32
	withFakeSpawn(t, func(shell, cwd string) (pty.Pty, *pty.Cmd, error) {
		atomic.AddInt32(&calls, 1)
		return &fakePty{}, nil, nil
	})
	withFakeKill(t, func(sess *terminalSession) error { return nil })

	app := &App{}
	id1, err := app.StartTerminal("")
	if err != nil {
		t.Fatalf("StartTerminal (first): unexpected error: %v", err)
	}
	id2, err := app.StartTerminal("")
	if err != nil {
		t.Fatalf("StartTerminal (second): unexpected error: %v", err)
	}
	if id1 != id2 {
		t.Errorf("expected the second StartTerminal to reuse the existing session, got %q then %q", id1, id2)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected the spawn function called exactly once, got %d", got)
	}
}

// --- resolveCwd ---------------------------------------------------------------

func TestResolveCwd(t *testing.T) {
	t.Run("uses workspaceRoot when it is non-empty and exists on disk", func(t *testing.T) {
		dir := t.TempDir()
		got := resolveCwd(dir)
		if got != dir {
			t.Errorf("resolveCwd(%q) = %q, want %q", dir, got, dir)
		}
	})

	t.Run("falls back to the home directory when workspaceRoot is empty", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("no home directory available in this environment")
		}
		got := resolveCwd("")
		if got != home {
			t.Errorf(`resolveCwd("") = %q, want %q`, got, home)
		}
	})

	t.Run("falls back to the home directory when workspaceRoot does not exist", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("no home directory available in this environment")
		}
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		got := resolveCwd(missing)
		if got != home {
			t.Errorf("resolveCwd(%q) = %q, want %q", missing, got, home)
		}
	})
}

// --- Cwd stability after reopen ------------------------------------------------

func TestStartTerminalCwdStableAcrossReopen(t *testing.T) {
	withFakeSpawn(t, func(shell, cwd string) (pty.Pty, *pty.Cmd, error) {
		return &fakePty{}, nil, nil
	})
	withFakeKill(t, func(sess *terminalSession) error { return nil })

	app := &App{}
	dir := t.TempDir()
	id, err := app.StartTerminal(dir)
	if err != nil {
		t.Fatalf("StartTerminal: unexpected error: %v", err)
	}

	other := t.TempDir()
	id2, err := app.StartTerminal(other)
	if err != nil {
		t.Fatalf("StartTerminal (reopen): unexpected error: %v", err)
	}
	if id2 != id {
		t.Fatalf("expected reopen to reuse the existing session, got a different id")
	}

	app.terminalMu.Lock()
	sess := app.terminals[id]
	app.terminalMu.Unlock()
	if sess == nil {
		t.Fatal("expected session still registered")
	}
	if sess.cwd != dir {
		t.Errorf("cwd changed after reopen: got %q, want %q (the original cwd)", sess.cwd, dir)
	}
}

// --- StartTerminal / spawn failure ---------------------------------------------

func TestStartTerminalSpawnFailure(t *testing.T) {
	spawnErr := errors.New("boom: no such shell")
	withFakeSpawn(t, func(shell, cwd string) (pty.Pty, *pty.Cmd, error) {
		return nil, nil, spawnErr
	})

	app := &App{}
	id, err := app.StartTerminal("")
	if err == nil {
		t.Fatal("expected StartTerminal to return an error, got nil")
	}
	if id != "" {
		t.Errorf("expected an empty id on spawn failure, got %q", id)
	}

	app.terminalMu.Lock()
	n := len(app.terminals)
	app.terminalMu.Unlock()
	if n != 0 {
		t.Errorf("expected no session registered after a spawn failure, got %d", n)
	}
	// a.ctx is nil on this bare &App{}: if StartTerminal had launched the
	// read-loop goroutine (which is the only place EventsEmit is called),
	// readTerminalOutput's ctx-nil guard would still stop it emitting, but
	// no goroutine should be launched at all on the error path.
}
