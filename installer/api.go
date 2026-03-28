package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

type CheckResult struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

type Config struct {
	InstallDir    string `json:"installDir"`
	AppPort       string `json:"appPort"`
	APIPort       string `json:"apiPort"`
	DBPort        string `json:"dbPort"`
	MinioAPIPort  string `json:"minioApiPort"`
	MinioConPort  string `json:"minioConPort"`
	DBPassword    string `json:"dbPassword"`
	MinioPassword string `json:"minioPassword"`
	WithAI        bool   `json:"withAI"`
	RegisterSvc   bool   `json:"registerService"`
}

var (
	mu         sync.Mutex
	subs       []chan string // main install SSE
	dockerSubs []chan string // docker install SSE
)

func registerHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/defaults", handleDefaults)
	mux.HandleFunc("/api/checks", handleChecks)
	mux.HandleFunc("/api/install-docker", handleInstallDocker)
	mux.HandleFunc("/api/docker-progress", handleDockerProgress)
	mux.HandleFunc("/api/install", handleInstall)
	mux.HandleFunc("/api/progress", handleProgress)
	mux.HandleFunc("/api/done", handleDone)
}

func handleDefaults(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
		"installDir":   defaultInstallDir(),
		"appPort":      "3000",
		"apiPort":      "8080",
		"dbPort":       "5432",
		"minioApiPort": "9000",
		"minioConPort": "9001",
	})
}

func defaultInstallDir() string {
	if runtime.GOOS == "windows" {
		return `C:\ezHealthKonnect`
	}
	return "/opt/ezhealthkonnect"
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
