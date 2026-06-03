package main

import (
	"bufio"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed assets/docker-compose.prod.yml
var embeddedComposeYML []byte

//go:embed assets/docker-compose.listeners.yml
var embeddedListenersYML []byte

// runInstallation dispatches to the Docker or Standalone path based on cfg.Mode.
func runInstallation(cfg *Config) {
	if cfg.Mode == "standalone" {
		runStandaloneInstallation(cfg)
		return
	}
	runDockerInstallation(cfg)
}

func runDockerInstallation(cfg *Config) {
	success := false
	defer func() {
		if success {
			emitDoneSignal(fmt.Sprintf("http://localhost:%s", cfg.AppPort))
		} else {
			emitInstallFailed()
		}
	}()

	emit("info", "╔══════════════════════════════════════════════╗")
	emit("info", "║   ezHealthKonnect — Starting Installation    ║")
	emit("info", "╚══════════════════════════════════════════════╝")
	emit("info", "")

	// Step 1: Create install directory
	emitStep(1, "Preparing install directory")
	if err := os.MkdirAll(cfg.InstallDir, 0755); err != nil {
		emit("error", "Failed to create directory: "+err.Error())
		return
	}
	emit("ok", "Directory ready: "+cfg.InstallDir)

	// Step 2: Write embedded compose files
	emitStep(2, "Writing compose configuration")
	composeDest := filepath.Join(cfg.InstallDir, "docker-compose.prod.yml")
	if err := os.WriteFile(composeDest, embeddedComposeYML, 0644); err != nil {
		emit("error", "Failed to write docker-compose.prod.yml: "+err.Error())
		return
	}
	listenersDest := filepath.Join(cfg.InstallDir, "docker-compose.listeners.yml")
	if err := os.WriteFile(listenersDest, embeddedListenersYML, 0644); err != nil {
		emit("error", "Failed to write docker-compose.listeners.yml: "+err.Error())
		return
	}
	emit("ok", "Compose files written to "+cfg.InstallDir)

	// Step 3: Write .env
	emitStep(3, "Writing configuration")
	envPath := filepath.Join(cfg.InstallDir, ".env")
	if err := writeEnvFile(envPath, cfg); err != nil {
		emit("error", "Failed to write .env: "+err.Error())
		return
	}
	emit("ok", "Configuration saved to "+envPath)

	// Step 4: Pull image from registry
	emitStep(4, "Pulling ezHealthKonnect image from registry")
	emit("info", "Downloading pre-built image (~300 MB on first run)...")
	emit("info", "")
	if err := runDockerPull(); err != nil {
		emit("error", "Image pull failed: "+err.Error())
		return
	}
	emit("ok", "Image downloaded successfully")

	// Step 4b: Extract database migrations from image to install dir.
	// docker-compose.prod.yml mounts ./database/migrations into the Flyway
	// container. The migrations live inside the app image, not on the host,
	// so we copy them out here once after the pull.
	emitStep(4, "Extracting database migrations")
	if err := extractMigrations(cfg.InstallDir); err != nil {
		emit("error", "Failed to extract migrations: "+err.Error())
		return
	}
	emit("ok", "Migrations ready")

	// Step 5: Start services
	emitStep(5, "Starting services")
	if err := runDockerCompose(composeDest, listenersDest, envPath, cfg.WithAI); err != nil {
		emit("error", "Failed to start services: "+err.Error())
		return
	}
	emit("ok", "All services started")

	// Step 6: Register OS service
	if cfg.RegisterSvc {
		emitStep(6, "Registering system service")
		if err := registerService(cfg.InstallDir, composeDest, listenersDest, envPath); err != nil {
			emit("warn", "Service registration failed: "+err.Error())
			emit("warn", "The application is running — you can register the service later.")
		} else {
			emit("ok", "System service registered (auto-start on boot enabled)")
		}
	}

	// Step 7: Wait for app
	emitStep(7, "Waiting for application to become ready")
	appURL := fmt.Sprintf("http://localhost:%s", cfg.AppPort)
	waitForReady(appURL+"/health", 120*time.Second)

	emit("info", "")
	emit("info", "╔══════════════════════════════════════════════╗")
	emit("ok",   "║   Installation Complete!                     ║")
	emit("info", "╚══════════════════════════════════════════════╝")
	emit("info", "")
	emit("ok", "Platform URL: "+appURL)
	success = true
}

func extractMigrations(installDir string) error {
	dest := filepath.Join(installDir, "database", "migrations")
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	// Copy /app/database/migrations out of the image into the host install dir.
	return streamCmd(exec.Command(
		"docker", "run", "--rm",
		"-v", installDir+":/output",
		"shanawaz107/ezhealthkonnect:latest",
		"sh", "-c", "cp -r /app/database/migrations /output/database/",
	))
}

func runDockerPull() error {
	return streamCmd(exec.Command("docker", "pull", "shanawaz107/ezhealthkonnect:latest"))
}

func runDockerCompose(composeFile, listenersFile, envFile string, withAI bool) error {
	args := []string{"compose", "-f", composeFile, "-f", listenersFile, "--env-file", envFile}
	if withAI {
		args = append(args, "--profile", "ai")
	}
	args = append(args, "up", "-d", "--remove-orphans")
	return streamCmd(exec.Command("docker", args...))
}

func streamCmd(cmd *exec.Cmd) error {
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	scanLines := func(r interface{ Read([]byte) (int, error) }) {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r\n")
			if strings.TrimSpace(line) != "" {
				emit("info", line)
			}
		}
	}
	go scanLines(stdout)
	scanLines(stderr)
	return cmd.Wait()
}

func writeEnvFile(path string, cfg *Config) error {
	session := randomSecret(48)
	jwt := randomSecret(48)
	content := fmt.Sprintf(`# ezHealthKonnect - Production Environment
# Generated by installer on %s
# Keep this file private — it contains secrets.

APP_IMAGE=shanawaz107/ezhealthkonnect:latest
APP_PORT=%s
API_PORT=%s

DB_USER=ezhealth_user
DB_PASSWORD=%s
DB_HOST_PORT=%s
DB_SSL=false

MINIO_USER=ezhealth_user
MINIO_PASSWORD=%s
MINIO_API_PORT=%s
MINIO_CONSOLE_PORT=%s

SESSION_SECRET=%s
JWT_SECRET=%s

OLLAMA_PORT=11434
OLLAMA_CHAT_MODEL=llama3.2:3b
OLLAMA_EMBED_MODEL=nomic-embed-text
`,
		time.Now().Format("2006-01-02 15:04"),
		cfg.AppPort, cfg.APIPort,
		cfg.DBPassword, cfg.DBPort,
		cfg.MinioPassword, cfg.MinioAPIPort, cfg.MinioConPort,
		session, jwt,
	)
	return os.WriteFile(path, []byte(content), 0600)
}

func randomSecret(n int) string {
	b := make([]byte, n)
	rand.Read(b) //nolint:errcheck
	// RawURLEncoding avoids +, /, = which break .env file parsing in Docker.
	return base64.RawURLEncoding.EncodeToString(b)
}

func emitStep(n int, label string) {
	emit("info", fmt.Sprintf("── Step %d: %s", n, label))
}

// runUninstall stops services and removes the install directory.
func runUninstall(cfg *Config) {
	defer func() { emitDoneSignal("") }()

	emit("info", "╔══════════════════════════════════════════════╗")
	emit("info", "║   ezHealthKonnect — Uninstalling             ║")
	emit("info", "╚══════════════════════════════════════════════╝")

	if cfg.Mode == "standalone" {
		runStandaloneUninstall(cfg)
	} else {
		runDockerUninstall(cfg)
	}
}

func runDockerUninstall(cfg *Config) {
	emit("info", "── Stopping Docker services")
	composePath := filepath.Join(cfg.InstallDir, "docker-compose.prod.yml")
	envPath     := filepath.Join(cfg.InstallDir, ".env")
	_ = streamCmd(exec.Command("docker", "compose", "-f", composePath, "--env-file", envPath, "down", "-v", "--remove-orphans"))
	emit("ok", "Docker services stopped")

	emit("info", "── Removing install directory: "+cfg.InstallDir)
	if err := os.RemoveAll(cfg.InstallDir); err != nil {
		emit("warn", "Could not fully remove "+cfg.InstallDir+": "+err.Error())
	} else {
		emit("ok", "Install directory removed")
	}

	emit("ok", "Uninstall complete")
}

func waitForReady(url string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				emit("ok", "Application is ready")
				return
			}
		}
		emit("info", "Waiting for application to start...")
		time.Sleep(5 * time.Second)
	}
	emit("warn", "Health check timed out — the app may still be initialising")
}
