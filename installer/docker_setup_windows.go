//go:build windows

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const dockerDesktopURL = "https://desktop.docker.com/win/main/amd64/Docker%20Desktop%20Installer.exe"

func installDockerDesktop() error {
	emitDocker("info", "Downloading Docker Desktop (~600 MB)...")
	emitDocker("info", "This may take several minutes depending on your connection.")

	tmp := filepath.Join(os.TempDir(), "DockerDesktopInstaller.exe")
	if err := downloadWithProgress(dockerDesktopURL, tmp); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	emitDocker("ok", "Download complete")

	emitDocker("info", "Running Docker Desktop installer...")
	emitDocker("info", "A User Account Control (UAC) prompt will appear — click Yes to continue.")

	// Run installer elevated via PowerShell
	psCmd := fmt.Sprintf(`Start-Process -FilePath "%s" -ArgumentList "install","--quiet","--accept-license" -Verb RunAs -Wait`, tmp)
	out, err := exec.Command("powershell", "-NoProfile", "-Command", psCmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("installer failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	os.Remove(tmp)
	emitDocker("ok", "Docker Desktop installed")

	emitDocker("info", "Starting Docker Desktop — this can take up to 60 seconds...")
	// Try to start Docker Desktop
	exec.Command("powershell", "-Command", //nolint:errcheck
		`Start-Process "C:\Program Files\Docker\Docker\Docker Desktop.exe"`).Start()

	// Poll until Docker daemon is ready
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		out, err := exec.Command("docker", "version", "--format", "{{.Server.Version}}").Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			emitDocker("ok", "Docker is running")
			return nil
		}
		emitDocker("info", "Waiting for Docker to start...")
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("Docker did not start within 3 minutes — please start Docker Desktop manually and try again")
}

func downloadWithProgress(url, dest string) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)
	lastReport := time.Now()

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			f.Write(buf[:n]) //nolint:errcheck
			downloaded += int64(n)
			if time.Since(lastReport) > 5*time.Second {
				if total > 0 {
					pct := float64(downloaded) / float64(total) * 100
					emitDocker("info", fmt.Sprintf("Downloading... %.0f%% (%.0f / %.0f MB)",
						pct, float64(downloaded)/1e6, float64(total)/1e6))
				} else {
					emitDocker("info", fmt.Sprintf("Downloading... %.0f MB", float64(downloaded)/1e6))
				}
				lastReport = time.Now()
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}
