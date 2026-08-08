//go:build !windows

package main

import "os/exec"

// hideConsoleWindow is a no-op outside Windows — other platforms don't pop a
// console window for child processes started this way.
func hideConsoleWindow(cmd *exec.Cmd) {}
