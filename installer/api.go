package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type CheckResult struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
	Warning string `json:"warning,omitempty"` // non-blocking issue — user can resolve via config
}

type Config struct {
	// Common — both editions
	Mode        string `json:"mode"`           // "docker" | "standalone"
	InstallDir  string `json:"installDir"`
	AppPort     string `json:"appPort"`
	APIPort     string `json:"apiPort"`
	DBHost      string `json:"dbHost"`
	DBPort      string `json:"dbPort"`
	DBPassword  string `json:"dbPassword"`
	WithAI      bool   `json:"withAI"`
	RegisterSvc bool   `json:"registerService"`
	Version     string `json:"version"`        // release tag or "latest"

	// Docker Edition only
	MinioAPIPort  string `json:"minioApiPort"`
	MinioConPort  string `json:"minioConPort"`
	MinioPassword string `json:"minioPassword"`

	// Standalone Edition only
	DBUser string `json:"dbUser"`
	DBName string `json:"dbName"`
}

var (
	mu         sync.Mutex
	subs       []chan string // main install SSE
	dockerSubs []chan string // docker install SSE
)

func registerHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/defaults", handleDefaults)
	mux.HandleFunc("/api/checks", handleChecks)
	mux.HandleFunc("/api/checks-standalone", handleChecksStandalone)
	mux.HandleFunc("/api/install-docker", handleInstallDocker)
	mux.HandleFunc("/api/docker-progress", handleDockerProgress)
	mux.HandleFunc("/api/install", handleInstall)
	mux.HandleFunc("/api/progress", handleProgress)
	mux.HandleFunc("/api/done", handleDone)
	mux.HandleFunc("/api/browse", handleBrowse)
	mux.HandleFunc("/api/uninstall", handleUninstall)
	mux.HandleFunc("/api/installed-state", handleInstalledState)
}

// handleBrowse returns subdirectories of a given path for the folder picker.
func handleBrowse(w http.ResponseWriter, r *http.Request) {
	type DirEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	type BrowseResult struct {
		Path   string     `json:"path"`
		Parent string     `json:"parent,omitempty"`
		Dirs   []DirEntry `json:"dirs"`
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		if runtime.GOOS == "windows" {
			path = `C:\`
		} else {
			path = "/"
		}
	}
	path = filepath.Clean(path)

	result := BrowseResult{Path: path}

	// Compute parent
	parent := filepath.Dir(path)
	if parent != path { // not at filesystem root
		result.Parent = parent
	}

	// List subdirectories only
	entries, err := os.ReadDir(path)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result) //nolint:errcheck
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip hidden and system dirs
		if strings.HasPrefix(name, ".") || name == "$Recycle.Bin" || name == "System Volume Information" {
			continue
		}
		result.Dirs = append(result.Dirs, DirEntry{
			Name: name,
			Path: filepath.Join(path, name),
		})
	}
	sort.Slice(result.Dirs, func(i, j int) bool {
		return strings.ToLower(result.Dirs[i].Name) < strings.ToLower(result.Dirs[j].Name)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result) //nolint:errcheck
}

// handleUninstall triggers removal of the installed application.
func handleUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var cfg Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	go runUninstall(&cfg)
	w.WriteHeader(http.StatusAccepted)
}

func handleDefaults(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
		"installDir":   defaultInstallDir(),
		"appPort":      "3000",
		"apiPort":      "8080",
		"dbPort":       detectPostgresPort(),
		"minioApiPort": "9000",
		"minioConPort": "9001",
	})
}

// detectPostgresPort probes the common PostgreSQL ports and returns the first
// one that is actually listening, falling back to "5432".
func detectPostgresPort() string {
	for _, port := range []string{"5432", "5433", "5434"} {
		conn, err := net.DialTimeout("tcp", "localhost:"+port, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return port
		}
	}
	return "5432"
}

func defaultInstallDir() string {
	if runtime.GOOS == "windows" {
		return `C:\ezHealthKonnect`
	}
	return "/opt/ezhealthkonnect"
}

// handleChecksStandalone runs prerequisite checks for Standalone Edition.
// Docker is NOT required; we check OS, disk space, and port availability.
func handleChecksStandalone(w http.ResponseWriter, _ *http.Request) {
	checks := []CheckResult{}

	// OS version
	osCheck := checkOS()
	checks = append(checks, osCheck)

	// Disk space (need ~2 GB free in default install dir parent)
	checks = append(checks, checkDiskSpace())

	// Ports
	for _, port := range []string{"3000", "8080", "5432"} {
		checks = append(checks, checkPortFree(port))
	}

	// winget (Windows only — needed to install PostgreSQL + Node.js)
	if runtime.GOOS == "windows" {
		checks = append(checks, checkWinget())
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(checks) //nolint:errcheck
}

func checkOS() CheckResult {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("cmd", "/c", "ver").Output()
		if err == nil {
			return CheckResult{Name: "Windows", OK: true, Version: strings.TrimSpace(string(out))}
		}
		return CheckResult{Name: "Windows", OK: true, Version: "unknown"}
	case "linux":
		out, err := exec.Command("uname", "-r").Output()
		if err == nil {
			return CheckResult{Name: "Linux Kernel", OK: true, Version: strings.TrimSpace(string(out))}
		}
		return CheckResult{Name: "Linux", OK: true}
	default:
		return CheckResult{Name: runtime.GOOS, OK: false,
			Error: "Standalone Edition supports Windows and Linux only"}
	}
}

func checkDiskSpace() CheckResult {
	var path string
	if runtime.GOOS == "windows" {
		path = `C:\`
	} else {
		path = "/opt"
	}
	// Use df on Linux, WMIC on Windows
	var availMB int64
	if runtime.GOOS == "windows" {
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			fmt.Sprintf(`(Get-PSDrive -Name C).Free / 1MB`)).Output()
		if err == nil {
			fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &availMB)
		}
	} else {
		_ = path
		out, err := exec.Command("df", "-m", "/opt").Output()
		if err != nil {
			out, err = exec.Command("df", "-m", "/").Output()
		}
		if err == nil {
			lines := strings.Split(string(out), "\n")
			if len(lines) >= 2 {
				fields := strings.Fields(lines[1])
				if len(fields) >= 4 {
					fmt.Sscanf(fields[3], "%d", &availMB)
				}
			}
		}
	}

	if availMB < 2048 {
		return CheckResult{Name: "Disk Space",
			OK:    availMB > 512, // warn but don't hard-block if >512 MB
			Error: fmt.Sprintf("%d MB available (2048 MB recommended)", availMB),
		}
	}
	return CheckResult{Name: "Disk Space", OK: true,
		Version: fmt.Sprintf("%d MB available", availMB)}
}

func checkPortFree(port string) CheckResult {
	name := fmt.Sprintf("Port %s", port)
	ln, err := tryListen(port)
	if err != nil {
		// Port conflict is a warning, not a hard error — the user can change
		// the port in the Configuration step before installation begins.
		return CheckResult{Name: name, OK: true,
			Warning: fmt.Sprintf("Port %s is in use — change it in the Configuration step below", port)}
	}
	ln.Close()
	return CheckResult{Name: name, OK: true, Version: "available"}
}

func tryListen(port string) (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:"+port)
}

func checkWinget() CheckResult {
	out, err := exec.Command("winget", "--version").Output()
	if err != nil {
		return CheckResult{Name: "winget (App Installer)", OK: false,
			Error: "Not found — update Windows or install App Installer from the Microsoft Store"}
	}
	return CheckResult{Name: "winget (App Installer)", OK: true,
		Version: strings.TrimSpace(string(out))}
}

func handleChecks(w http.ResponseWriter, _ *http.Request) {
	checks := []CheckResult{}

	out, err := exec.Command("docker", "version", "--format", "{{.Server.Version}}").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		checks = append(checks, CheckResult{Name: "Docker Engine", OK: false,
			Error: "Not running or not installed"})
	} else {
		checks = append(checks, CheckResult{Name: "Docker Engine", OK: true,
			Version: strings.TrimSpace(string(out))})
	}

	out2, err2 := exec.Command("docker", "compose", "version", "--short").Output()
	if err2 != nil || strings.TrimSpace(string(out2)) == "" {
		checks = append(checks, CheckResult{Name: "Docker Compose v2", OK: false,
			Error: "Not found — update Docker Desktop"})
	} else {
		checks = append(checks, CheckResult{Name: "Docker Compose v2", OK: true,
			Version: strings.TrimSpace(string(out2))})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(checks) //nolint:errcheck
}

func handleInstallDocker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	go func() {
		if err := installDockerDesktop(); err != nil {
			emitDocker("error", "Docker installation failed: "+err.Error())
		}
		emitDockerDone()
	}()
	w.WriteHeader(http.StatusAccepted)
}

func handleDockerProgress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	ch := make(chan string, 64)
	mu.Lock()
	dockerSubs = append(dockerSubs, ch)
	mu.Unlock()
	defer func() {
		mu.Lock()
		for i, s := range dockerSubs {
			if s == ch {
				dockerSubs = append(dockerSubs[:i], dockerSubs[i+1:]...)
				break
			}
		}
		mu.Unlock()
	}()
	for {
		select {
		case msg, open := <-ch:
			if !open {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func handleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var cfg Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	go runInstallation(&cfg)
	w.WriteHeader(http.StatusAccepted)
}

func handleProgress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	ch := make(chan string, 128)
	mu.Lock()
	subs = append(subs, ch)
	mu.Unlock()
	defer func() {
		mu.Lock()
		for i, s := range subs {
			if s == ch {
				subs = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		mu.Unlock()
	}()
	for {
		select {
		case msg, open := <-ch:
			if !open {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// InstalledState describes the current installation on this machine.
type InstalledState struct {
	Installed      bool   `json:"installed"`
	InstallDir     string `json:"installDir,omitempty"`
	Mode           string `json:"mode,omitempty"`
	AppPort        string `json:"appPort,omitempty"`
	Version        string `json:"version,omitempty"`
	ServiceRunning bool   `json:"serviceRunning"`
}

func handleInstalledState(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(probeInstalledState()) //nolint:errcheck
}

// probeInstalledState checks whether ezHealthKonnect is installed on this machine
// by looking for the .env file in the default install directory.
func probeInstalledState() InstalledState {
	dir := defaultInstallDir()
	envPath := filepath.Join(dir, ".env")
	info, err := os.Stat(envPath)
	if err != nil || info.IsDir() {
		return InstalledState{Installed: false}
	}

	state := InstalledState{
		Installed:  true,
		InstallDir: dir,
		Mode:       "standalone",
		AppPort:    "3000",
		Version:    version,
	}

	// Parse key values from .env
	data, err := os.ReadFile(envPath)
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			key, val := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			switch key {
			case "PORT":
				state.AppPort = val
			}
		}
	}

	// Check if Windows service is running
	if runtime.GOOS == "windows" {
		out, err := exec.Command("sc", "query", "ezhealthkonnect").Output()
		if err == nil && strings.Contains(string(out), "RUNNING") {
			state.ServiceRunning = true
		}
	}

	return state
}

func handleDone(w http.ResponseWriter, _ *http.Request) {
	select {
	case <-done:
	default:
		close(done)
	}
	w.WriteHeader(http.StatusOK)
}

// emit sends a progress message to all main install SSE subscribers.
func emit(level, message string) {
	payload := fmt.Sprintf(`{"level":%q,"msg":%q}`, level, message)
	mu.Lock()
	for _, ch := range subs {
		select {
		case ch <- payload:
		default:
		}
	}
	mu.Unlock()
	fmt.Printf("[%s] %s\n", strings.ToUpper(level), message)
}

func emitDoneSignal(appURL string) {
	payload := fmt.Sprintf(`{"level":"done","msg":%q}`, appURL)
	mu.Lock()
	for _, ch := range subs {
		select {
		case ch <- payload:
		default:
		}
		close(ch)
	}
	subs = nil
	mu.Unlock()
}

// emitDocker sends a message to Docker install SSE subscribers.
func emitDocker(level, message string) {
	payload := fmt.Sprintf(`{"level":%q,"msg":%q}`, level, message)
	mu.Lock()
	for _, ch := range dockerSubs {
		select {
		case ch <- payload:
		default:
		}
	}
	mu.Unlock()
	fmt.Printf("[DOCKER-%s] %s\n", strings.ToUpper(level), message)
}

// emitInstallFailed signals the UI that installation failed — do NOT advance to success screen.
func emitInstallFailed() {
	payload := `{"level":"failed","msg":"Installation failed — see errors above."}`
	mu.Lock()
	for _, ch := range subs {
		select {
		case ch <- payload:
		default:
		}
		close(ch)
	}
	subs = nil
	mu.Unlock()
}

func emitDockerDone() {
	payload := `{"level":"done","msg":""}`
	mu.Lock()
	for _, ch := range dockerSubs {
		select {
		case ch <- payload:
		default:
		}
		close(ch)
	}
	dockerSubs = nil
	mu.Unlock()
}
