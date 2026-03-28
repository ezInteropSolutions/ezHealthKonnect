//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func installDockerDesktop() error {
	emitDocker("info", "Installing Docker Engine via get.docker.com script...")
	emitDocker("info", "sudo password may be required.")

	// Download and run official install script
	cmds := []struct {
		desc string
		args []string
	}{
		{"Downloading install script", []string{"sh", "-c", "curl -fsSL https://get.docker.com -o /tmp/get-docker.sh"}},
		{"Running Docker install script", []string{"sudo", "sh", "/tmp/get-docker.sh"}},
		{"Enabling Docker service", []string{"sudo", "systemctl", "enable", "--now", "docker"}},
	}

	for _, c := range cmds {
		emitDocker("info", c.desc+"...")
		out, err := exec.Command(c.args[0], c.args[1:]...).CombinedOutput()
		if err != nil {
			emitDocker("warn", string(out))
			return fmt.Errorf("%s failed: %w", c.desc, err)
		}
	}

	// Add current user to docker group so we don't need sudo for docker commands
	if user := os.Getenv("SUDO_USER"); user == "" {
		if u := os.Getenv("USER"); u != "" && u != "root" {
			exec.Command("sudo", "usermod", "-aG", "docker", u).Run() //nolint:errcheck
			emitDocker("info", fmt.Sprintf("Added %s to docker group (re-login for effect)", u))
		}
	}

	os.Remove("/tmp/get-docker.sh") //nolint:errcheck
	emitDocker("ok", "Docker installed successfully")

	// Poll until Docker daemon is ready
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.Command("docker", "version", "--format", "{{.Server.Version}}").Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			emitDocker("ok", "Docker is running")
			return nil
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("Docker installed but daemon not responding — try: sudo systemctl start docker")
}
