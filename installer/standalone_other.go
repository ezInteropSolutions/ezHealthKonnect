//go:build !windows && !linux && !darwin

package main

import "fmt"

func runStandaloneUninstall(cfg *Config) {
	defer func() { emitDoneSignal("") }()
	emit("error", "Standalone uninstall is not supported on this platform.")
}

// runStandaloneInstallation is not implemented for non-Windows, non-Linux platforms.
func runStandaloneInstallation(cfg *Config) {
	defer func() {
		emitDoneSignal(fmt.Sprintf("http://localhost:%s", cfg.AppPort))
	}()

	emit("error", "Standalone Edition is not supported on this platform.")
	emit("info", "Supported platforms: Windows 10/11/Server, Linux (Ubuntu/Debian/RHEL/Rocky).")
	emit("info", "Use the Docker Edition on other platforms.")
}
