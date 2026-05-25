//go:build windows

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

func registerService(installDir, composeFile, listenersFile, envFile string) error {
	nssmDir := filepath.Join(installDir, "tools")
	nssmExe := filepath.Join(nssmDir, "nssm.exe")

	if _, err := os.Stat(nssmExe); os.IsNotExist(err) {
		emit("info", "Downloading NSSM service manager...")
		if err := downloadNSSM(nssmDir, nssmExe); err != nil {
			return fmt.Errorf("NSSM download: %w", err)
		}
		emit("ok", "NSSM downloaded")
	}

	dockerExe, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("docker not found in PATH: %w", err)
	}

	const svcName = "ezHealthKonnect"
	svcArgs := fmt.Sprintf(`compose -f "%s" -f "%s" --env-file "%s" up`, composeFile, listenersFile, envFile)

	nssm := func(args ...string) {
		out, err := exec.Command(nssmExe, args...).CombinedOutput()
		if err != nil {
			emit("warn", fmt.Sprintf("nssm %v: %s", args, string(out)))
		}
	}

	nssm("install", svcName, dockerExe, svcArgs)
	nssm("set", svcName, "DisplayName", "ezHealthKonnect")
	nssm("set", svcName, "Description", "ezHealthKonnect Healthcare Integration Platform")
	nssm("set", svcName, "Start", "SERVICE_AUTO_START")
	nssm("set", svcName, "DependOnService", "com.docker.service")
	nssm("set", svcName, "AppStopMethodConsole", "10000")
	nssm("set", svcName, "AppStopMethodWindow", "5000")
	nssm("set", svcName, "AppStopMethodThreads", "5000")

	logDir := filepath.Join(installDir, "logs")
	os.MkdirAll(logDir, 0755) //nolint:errcheck
	logFile := filepath.Join(logDir, "service.log")
	nssm("set", svcName, "AppStdout", logFile)
	nssm("set", svcName, "AppStderr", logFile)
	nssm("set", svcName, "AppRotateFiles", "1")

	// Start the service (already started via docker compose up -d above, but
	// registering it ensures it restarts on next boot).
	exec.Command("sc", "start", svcName).Run() //nolint:errcheck

	return nil
}

func downloadNSSM(dir, dest string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	zipPath := filepath.Join(dir, "nssm.zip")

	resp, err := http.Get("https://nssm.cc/release/nssm-2.24.zip")
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()

	// Unzip using PowerShell (available on all modern Windows)
	out, err := exec.Command("powershell", "-Command",
		fmt.Sprintf(`Expand-Archive -Path "%s" -DestinationPath "%s" -Force`, zipPath, dir),
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("unzip: %s: %w", out, err)
	}
	os.Remove(zipPath) //nolint:errcheck

	// nssm extracts to nssm-2.24\win64\nssm.exe
	matches, _ := filepath.Glob(filepath.Join(dir, "nssm-*", "win64", "nssm.exe"))
	if len(matches) == 0 {
		return fmt.Errorf("nssm.exe not found after extraction")
	}
	return copyFile(matches[0], dest)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
