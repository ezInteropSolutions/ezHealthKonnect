//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func registerService(installDir, composeFile, envFile string) error {
	dockerBin, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("docker not found in PATH: %w", err)
	}

	logDir := filepath.Join(installDir, "logs")
	os.MkdirAll(logDir, 0755) //nolint:errcheck
	logFile := filepath.Join(logDir, "service.log")

	unit := fmt.Sprintf(`[Unit]
Description=ezHealthKonnect Healthcare Integration Platform
After=docker.service network-online.target
Requires=docker.service
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s compose -f %s --env-file %s up
ExecStop=%s compose -f %s down
ExecReload=%s compose -f %s --env-file %s up -d --remove-orphans
Restart=on-failure
RestartSec=10
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=multi-user.target
`,
		installDir,
		dockerBin, composeFile, envFile,
		dockerBin, composeFile,
		dockerBin, composeFile, envFile,
		logFile, logFile,
	)

	unitPath := "/etc/systemd/system/ezhealthkonnect.service"

	// Try direct write first (running as root), fall back to sudo.
	if err := os.WriteFile(unitPath, []byte(unit), 0644); err != nil {
		tmp, terr := os.CreateTemp("", "ezhk-*.service")
		if terr != nil {
			return fmt.Errorf("create temp file: %w", terr)
		}
		tmp.WriteString(unit) //nolint:errcheck
		tmp.Close()
		defer os.Remove(tmp.Name())

		if out, err := exec.Command("sudo", "cp", tmp.Name(), unitPath).CombinedOutput(); err != nil {
			return fmt.Errorf("write unit file (sudo): %s: %w", out, err)
		}
	}

	// systemctl commands — try direct, then sudo.
	for _, args := range [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "ezhealthkonnect"},
		{"systemctl", "start", "ezhealthkonnect"},
	} {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			sudoArgs := append([]string{"sudo"}, args...)
			if out, err2 := exec.Command(sudoArgs[0], sudoArgs[1:]...).CombinedOutput(); err2 != nil {
				emit("warn", fmt.Sprintf("Command %v failed: %s", args, out))
			}
		}
	}
	return nil
}
