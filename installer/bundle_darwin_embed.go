//go:build darwin && embedded

package main

import _ "embed"

//go:embed assets/bundle-darwin.zip
var embeddedDarwinBundle []byte
