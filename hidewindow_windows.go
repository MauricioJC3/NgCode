//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// hideConsoleWindow prevents Windows from popping up a console window when
// this GUI app spawns a console subprocess (gopls, the relaunched binary
// after an auto-update swap).
func hideConsoleWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
