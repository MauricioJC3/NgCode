//go:build !windows

package main

import (
	"syscall"
	"time"
)

// terminalKillGracePeriod is how long killTerminalSession waits for the
// shell's process group to exit after SIGHUP before escalating to
// SIGKILL — mirrors lspClient.stop()'s 3s Wait timeout in lsp.go, scaled
// down since a shell reacting to SIGHUP is expected to be near-instant
// compared to an LSP server's shutdown handshake.
const terminalKillGracePeriod = 500 * time.Millisecond

// killTerminalSession terminates sess's shell and everything in its
// process group. go-pty's Unix Cmd always calls setsid()+setctty() before
// exec (a hard OS precondition for pty semantics — creack/pty, which
// go-pty wraps on Unix, sets SysProcAttr{Setsid: true, Setctty: true}
// internally), so the shell process is always its own session leader and
// process-group leader: pid == pgid the moment it starts. Consequently
// syscall.Kill(-pid, sig), with a negative pid, targets the whole group,
// not just the shell.
//
// Known limitation (documented, not a blocker — matches VS Code's own
// integrated terminal behavior): a fully detached process (nohup, disown,
// or a double fork into its own session) escapes the group and isn't
// reachable this way. Foreground dev servers/watchers — the case this
// exists for — stay in the shell's process group and are covered.
func killTerminalSession(sess *terminalSession) error {
	defer sess.pty.Close()

	pid := sess.cmd.Process.Pid

	_ = syscall.Kill(-pid, syscall.SIGHUP)

	done := make(chan error, 1)
	go func() { done <- sess.cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(terminalKillGracePeriod):
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		<-done
	}

	return nil
}
