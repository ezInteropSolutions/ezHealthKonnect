//go:build windows && !embedded

package main

// embeddedWindowsBundle is nil when built without -tags embedded.
// downloadAppBundle will fall back to downloading from GitHub releases.
var embeddedWindowsBundle []byte
