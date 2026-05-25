//go:build windows && embedded

package main

import _ "embed"

// embeddedWindowsBundle holds the pre-packaged app bundle zip.
// Populated at build time when built with -tags embedded.
//
//go:embed assets/bundle-windows.zip
var embeddedWindowsBundle []byte
