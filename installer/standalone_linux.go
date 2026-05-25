//go:build linux

package main

import "fmt"

// runStandaloneInstallation on Linux redirects the user to install.sh.
// The Go installer (browser wizard) is the Docker Edition for all platforms.
// The Linux Standalone Edition is install.sh — a purpose-built shell script
// that is better suited to headless/server environments.
func runStandaloneUninstall(cfg *Config) {
	defer func() { emitDoneSignal("") }()
	emit("info", "To uninstall on Linux, run:")
	emit("info", "")
	emit("info", "  sudo systemctl stop ezhealthkonnect ezhealthkonnect-api")
	emit("info", "  sudo systemctl disable ezhealthkonnect ezhealthkonnect-api")
	emit("info", "  sudo rm /etc/systemd/system/ezhealthkonnect*.service")
	emit("info", "  sudo rm -rf /opt/ezhealthkonnect")
	emit("info", "  sudo userdel ezhealth")
	emit("ok", "Instructions shown — run these commands on your server.")
}

func runStandaloneInstallation(cfg *Config) {
	defer func() { emitInstallFailed() }() // Linux standalone is not a real install — always shows info

	emit("warn", "Linux Standalone Edition uses the shell script installer.")
	emit("info", "")
	emit("info", "Run one of these commands on your Linux server:")
	emit("info", "")
	emit("info", "  # One-liner (recommended):")
	emit("info", "  curl -fsSL https://releases.ezhealthkonnect.com/install.sh | sudo bash")
	emit("info", "")
	emit("info", "  # With custom options:")
	emit("info", fmt.Sprintf(
		"  sudo bash install.sh --port %s --api-port %s --db-password <pass>",
		cfg.AppPort, cfg.APIPort,
	))
	emit("info", "")
	emit("info", "The shell script installs all dependencies (PostgreSQL, Node.js),")
	emit("info", "downloads the app, runs migrations, and registers systemd services.")
}
