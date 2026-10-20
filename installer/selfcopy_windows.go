//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// scheduleRemoval deletes dir a few seconds after this process exits (a
// running program cannot delete its own file on Windows).
func scheduleRemoval(dir string) error {
	cmd := exec.Command("cmd.exe", "/c", "ping -n 6 127.0.0.1 >nul & rmdir /s /q \""+dir+"\"")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008} // DETACHED_PROCESS
	return cmd.Start()
}
