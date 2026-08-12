//go:build windows

package main

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/aymanbagabas/go-pty"
	"golang.org/x/sys/windows"
)

// terminalJobs tracks the Job Object handle created for each spawned
// shell's PID, so killTerminalSession can find it again at teardown time
// without needing terminalSession (shared with terminal.go/terminal_unix.go)
// to grow a Windows-only field.
var (
	terminalJobsMu sync.Mutex
	terminalJobs   = map[int]windows.Handle{}
)

// terminalAfterSpawn creates a Job Object with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE and assigns cmd's just-started
// process to it — called from terminal.go's defaultSpawnTerminal
// immediately after Cmd.Start() succeeds ("on session start, immediately"
// per design). This ordering matters: Windows only auto-assigns a
// process's FUTURE children to its job (default behavior, unless a child
// explicitly opts out with CREATE_BREAKAWAY_FROM_JOB, which normal dev
// tooling never does) — assigning after the shell has already spawned
// children would miss them, so this must run before the shell has had a
// chance to spawn anything. This makes teardown tree-based rather than
// group-based, strictly more complete than terminal_unix.go's
// process-group approach.
//
// MANUAL-VERIFICATION-ONLY (per design's Testing Strategy and the tasks
// artifact's Phase 2 note, task 2.1): this file is //go:build
// windows-gated and cannot compile or run on the Linux/WSL2 environment
// this change was implemented in — no automated RED/GREEN cycle was
// possible. Written carefully against the design and
// golang.org/x/sys/windows's actual API surface (confirmed via direct
// module source inspection at
// $(go env GOMODCACHE)/golang.org/x/sys@v0.44.0/windows, not guessed),
// but needs a manual Windows smoke pass before merging to main: spawn a
// session that runs a child process (e.g. a long-lived dev server), then
// confirm KillTerminal/ForceQuit leaves no orphan in Task Manager.
func terminalAfterSpawn(cmd *pty.Cmd) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("terminal: CreateJobObject: %w", err)
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("terminal: SetInformationJobObject: %w", err)
	}

	pid := uint32(cmd.Process.Pid)
	procHandle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("terminal: OpenProcess: %w", err)
	}
	defer windows.CloseHandle(procHandle)

	if err := windows.AssignProcessToJobObject(job, procHandle); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("terminal: AssignProcessToJobObject: %w", err)
	}

	terminalJobsMu.Lock()
	terminalJobs[cmd.Process.Pid] = job
	terminalJobsMu.Unlock()

	return nil
}

// killTerminalSession terminates sess's shell and its entire process tree
// via the Job Object created for it in terminalAfterSpawn.
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE means even just closing the job
// handle would terminate the tree, but calling TerminateJobObject
// explicitly is more direct and gives an exit code to the tree.
func killTerminalSession(sess *terminalSession) error {
	defer sess.pty.Close()

	pid := sess.cmd.Process.Pid

	terminalJobsMu.Lock()
	job, ok := terminalJobs[pid]
	if ok {
		delete(terminalJobs, pid)
	}
	terminalJobsMu.Unlock()

	if !ok {
		// terminalAfterSpawn failed or was never reached (shouldn't
		// happen — StartTerminal fails the whole spawn on error from
		// either step) — nothing left to tear down beyond the pty itself.
		return nil
	}

	err := windows.TerminateJobObject(job, 1)
	_ = windows.CloseHandle(job)
	return err
}
