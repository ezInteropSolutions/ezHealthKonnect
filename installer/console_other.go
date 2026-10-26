//go:build !windows

package main

// releaseOwnConsole is Windows-only: on Linux setup runs from a terminal,
// where the printed address and messages are wanted.
func releaseOwnConsole() bool { return false }
