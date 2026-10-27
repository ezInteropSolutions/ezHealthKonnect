//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// releaseOwnConsole detaches the setup process from its console window when
// that window exists only for this process — the case when setup.exe is
// double-clicked in Explorer — so the user sees just the wizard. Started from
// a terminal (cmd, PowerShell, a script), the console is shared with the
// shell and is left alone, so headless commands and their output are
// unaffected. Reports whether it detached.
func releaseOwnConsole() bool {
	k32 := syscall.NewLazyDLL("kernel32.dll")
	getList := k32.NewProc("GetConsoleProcessList")
	if getList.Find() != nil {
		return false
	}
	var pids [2]uint32
	n, _, _ := getList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if n != 1 { // 0 = no console; 2+ = shared with a shell or another process
		return false
	}
	if r, _, _ := k32.NewProc("FreeConsole").Call(); r == 0 {
		return false
	}
	// The console handles are gone; send any later output nowhere instead.
	if null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = null, null
	}
	return true
}
