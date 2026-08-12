package main

import (
	"fmt"
	"os"

	"github.com/aymanbagabas/go-pty"
	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// terminalReadChunkSize bounds each read-loop iteration pumping pty output
// to the frontend. Terminal output isn't line-oriented (ANSI escapes,
// partial lines), so raw byte chunks are forwarded as they arrive instead
// of accumulating full lines — preserves xterm.js rendering fidelity.
const terminalReadChunkSize = 4096

// terminalSession is one live pty-backed shell, tracked under
// App.terminalMu. v1 supports at most one entry in App.terminals at a
// time, but the map shape (keyed by UUID) is deliberate so multi-terminal
// support later doesn't require an API break.
type terminalSession struct {
	id  string
	pty pty.Pty
	cmd *pty.Cmd
	cwd string
}

// TerminalOutput carries one chunk of pty output for the frontend to
// render, emitted via the "terminal:output" event.
type TerminalOutput struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

// terminalSpawnFunc opens a pty, builds a Cmd for shell rooted at cwd, and
// starts it. Package-level var (defaults to defaultSpawnTerminal) so tests
// can inject a fake that returns a fakePty and a nil *pty.Cmd — exercising
// StartTerminal's session-map, cwd, and spawn-failure logic without
// allocating a real OS pty or spawning a real shell process. The real
// pty-spawn happy path is covered by terminal_integration_test.go (Phase
// 2), which cannot avoid touching the real OS.
var terminalSpawnFunc = defaultSpawnTerminal

// terminalKillFunc tears down a session's OS process (tree). Package-level
// var (defaults to killTerminalSession, defined per-OS in
// terminal_unix.go/terminal_windows.go) so tests can inject a spy/no-op
// without exercising real process-group or Job Object teardown.
var terminalKillFunc = killTerminalSession

// defaultSpawnTerminal is terminalSpawnFunc's production implementation.
func defaultSpawnTerminal(shell, cwd string) (pty.Pty, *pty.Cmd, error) {
	p, err := pty.New()
	if err != nil {
		return nil, nil, err
	}
	cmd := p.Command(shell)
	cmd.Dir = cwd
	if err := cmd.Start(); err != nil {
		_ = p.Close()
		return nil, nil, err
	}
	return p, cmd, nil
}

// shellCommand returns the OS default shell to spawn: COMSPEC on Windows,
// $SHELL falling back to /bin/sh on macOS/Linux. COMSPEC is only ever set
// by Windows (cmd.exe sets it for every process), so checking it first
// selects the right shell without needing a build-tag split or importing
// runtime.GOOS (which would collide with this file's wails "runtime"
// import).
func shellCommand() string {
	if comspec := os.Getenv("COMSPEC"); comspec != "" {
		return comspec
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/sh"
}

// resolveCwd returns workspaceRoot if it's non-empty and exists on disk as
// a directory, otherwise falls back to the user's home directory. Called
// exactly once, at StartTerminal time — the resulting terminalSession.cwd
// is never re-resolved afterward, so closing the workspace folder
// mid-session cannot change a running session's cwd (there is no
// "workspace closed" hook that touches it).
func resolveCwd(workspaceRoot string) string {
	if workspaceRoot != "" {
		if info, err := os.Stat(workspaceRoot); err == nil && info.IsDir() {
			return workspaceRoot
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// sessionByID returns the session registered under id, if any.
func (a *App) sessionByID(id string) (*terminalSession, bool) {
	a.terminalMu.Lock()
	defer a.terminalMu.Unlock()
	sess, ok := a.terminals[id]
	return sess, ok
}

// StartTerminal spawns a new pty-backed shell rooted at workspaceRoot (or
// the user's home directory — see resolveCwd) the first time it's called
// with no session running, and simply returns the existing session's id on
// every subsequent call — v1 supports at most one live terminal session.
func (a *App) StartTerminal(workspaceRoot string) (string, error) {
	a.terminalMu.Lock()
	for _, sess := range a.terminals {
		id := sess.id
		a.terminalMu.Unlock()
		return id, nil
	}
	a.terminalMu.Unlock()

	cwd := resolveCwd(workspaceRoot)

	p, cmd, err := terminalSpawnFunc(shellCommand(), cwd)
	if err != nil {
		return "", fmt.Errorf("failed to start shell: %w", err)
	}

	sess := &terminalSession{id: uuid.NewString(), pty: p, cmd: cmd, cwd: cwd}

	a.terminalMu.Lock()
	if a.terminals == nil {
		a.terminals = make(map[string]*terminalSession)
	}
	a.terminals[sess.id] = sess
	a.terminalMu.Unlock()

	go a.readTerminalOutput(sess)

	return sess.id, nil
}

// WriteToTerminal forwards data (frontend keystrokes) to id's pty stdin.
func (a *App) WriteToTerminal(id string, data string) error {
	sess, ok := a.sessionByID(id)
	if !ok {
		return fmt.Errorf("terminal: unknown session %q", id)
	}
	_, err := sess.pty.Write([]byte(data))
	return err
}

// ResizeTerminal updates id's pty dimensions to match the panel's current
// visible size.
func (a *App) ResizeTerminal(id string, cols int, rows int) error {
	sess, ok := a.sessionByID(id)
	if !ok {
		return fmt.Errorf("terminal: unknown session %q", id)
	}
	return sess.pty.Resize(cols, rows)
}

// KillTerminal removes id from a.terminals and tears down its OS process
// (tree) via terminalKillFunc. Returns an error for an unknown id without
// touching anything.
func (a *App) KillTerminal(id string) error {
	a.terminalMu.Lock()
	sess, ok := a.terminals[id]
	if ok {
		delete(a.terminals, id)
	}
	a.terminalMu.Unlock()

	if !ok {
		return fmt.Errorf("terminal: unknown session %q", id)
	}
	return terminalKillFunc(sess)
}

// readTerminalOutput pumps sess's pty output to the frontend via
// "terminal:output" events, chunk by chunk, until the pty read fails
// (EOF after the shell exits, or an error following KillTerminal closing
// the pty). Mirrors runSearch's a.ctx-nil guard in search.go: a bare
// &App{} used directly in tests has no Wails runtime context, and
// runtime.EventsEmit would otherwise call log.Fatalf (os.Exit) on a nil
// context.
func (a *App) readTerminalOutput(sess *terminalSession) {
	buf := make([]byte, terminalReadChunkSize)
	for {
		n, err := sess.pty.Read(buf)
		if n > 0 && a.ctx != nil {
			runtime.EventsEmit(a.ctx, "terminal:output", TerminalOutput{ID: sess.id, Data: string(buf[:n])})
		}
		if err != nil {
			return
		}
	}
}
