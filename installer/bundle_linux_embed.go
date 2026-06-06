//go:build linux && embedded

package main

import _ "embed"

//go:embed assets/bundle-linux-amd64.zip
var embeddedLinuxBundle []byte
