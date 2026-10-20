//go:build !windows

package main

import "os"

// scheduleRemoval removes dir immediately: a running program's file can be
// deleted on Unix.
func scheduleRemoval(dir string) error { return os.RemoveAll(dir) }
