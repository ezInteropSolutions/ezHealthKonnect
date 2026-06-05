package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// DockerPortConfig is the shape stored in system_settings for key "docker_ports".
type DockerPortConfig struct {
	HL7PortRange       string `json:"hl7_port_range"`       // e.g. "6500-6700"
	HTTPPortRange      string `json:"http_port_range"`      // e.g. "8081-8099"
	StandardMLLPPort   int    `json:"standard_mllp_port"`   // 2575
	ComposeFilePath    string `json:"compose_file_path"`    // /app/docker-compose.prod.yml
	DeploymentMode     string `json:"deployment_mode"`      // "docker" | "standalone"
}

// DockerPortService manages Docker port configuration and container lifecycle.
type DockerPortService struct {
	httpClient *http.Client
}

// NewDockerPortService creates the service. It connects to the Docker socket
// at /var/run/docker.sock (Linux) if available, otherwise operates in
// config-only mode (standalone installs).
func NewDockerPortService() *DockerPortService {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", "/var/run/docker.sock")
		},
	}
	return &DockerPortService{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// IsDockerAvailable returns true if the Docker socket is reachable.
func (s *DockerPortService) IsDockerAvailable() bool {
	resp, err := s.dockerGet("/_ping")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// GetOwnContainerID reads the container ID from /proc/self/cgroup.
// Returns empty string if not running inside a container.
func (s *DockerPortService) GetOwnContainerID() string {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return ""
	}
	// Look for a 64-char hex ID in the cgroup entries
	re := regexp.MustCompile(`/([a-f0-9]{64})`)
	matches := re.FindStringSubmatch(string(data))
	if len(matches) < 2 {
		// Try hostname — Docker sets the short container ID as hostname
		hostname, err := os.Hostname()
		if err != nil || len(hostname) < 12 {
			return ""
		}
		return hostname
	}
	return matches[1]
}

// UpdateComposePortRanges rewrites the ports: section in the compose file
// to use the supplied HL7 and HTTP ranges.
func (s *DockerPortService) UpdateComposePortRanges(composePath, hl7Range, httpRange string) error {
	data, err := os.ReadFile(composePath)
	if err != nil {
		return fmt.Errorf("read compose file: %w", err)
	}

	content := string(data)

	// Replace HL7 TCP/MLLP range lines (matches "6xxx-6xxx:6xxx-6xxx" patterns)
	hl7Re := regexp.MustCompile(`(?m)^\s*-\s*"0\.0\.0\.0:[0-9]+-[0-9]+:[0-9]+-[0-9]+".*HL7.*\n`)
	newHL7Line := fmt.Sprintf("      - \"0.0.0.0:%s:%s\" # HL7 TCP/MLLP range\n", hl7Range, hl7Range)
	if hl7Re.MatchString(content) {
		content = hl7Re.ReplaceAllString(content, newHL7Line)
	} else {
		// Fallback: replace any existing hl7 range pattern
		fallbackRe := regexp.MustCompile(`(?m)^\s*-\s*"0\.0\.0\.0:[0-9]+-[0-9]+:[0-9]+-[0-9]+"[^\n]*\n`)
		content = fallbackRe.ReplaceAllStringFunc(content, func(line string) string {
			if strings.Contains(line, "808") {
				// Keep HTTP range lines
				return line
			}
			return newHL7Line
		})
	}

	// Replace HTTP/FHIR range
	httpRe := regexp.MustCompile(`(?m)^\s*-\s*"0\.0\.0\.0:808[0-9]-[0-9]+:808[0-9]-[0-9]+"[^\n]*\n`)
	newHTTPLine := fmt.Sprintf("      - \"0.0.0.0:%s:%s\" # HTTP/FHIR range\n", httpRange, httpRange)
	if httpRe.MatchString(content) {
		content = httpRe.ReplaceAllString(content, newHTTPLine)
	}

	if err := os.WriteFile(composePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write compose file: %w", err)
	}
	log.Printf("✅ DockerPortService: compose file updated (HL7: %s, HTTP: %s)", hl7Range, httpRange)
	return nil
}

// RestartAppContainer stops and recreates the app container using the Docker API.
// The container is identified by name "ezhk-app". The new container is created
// with the same config but new port bindings read from the updated compose file.
func (s *DockerPortService) RestartAppContainer(hl7Range, httpRange string) error {
	containerName := "ezhk-app"

	// Inspect current container to get its full config
	resp, err := s.dockerGet("/containers/" + containerName + "/json")
	if err != nil {
		return fmt.Errorf("inspect container: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("inspect container: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var containerInfo map[string]interface{}
	if err := json.Unmarshal(body, &containerInfo); err != nil {
		return fmt.Errorf("parse container info: %w", err)
	}

	// Build new port bindings from the configured ranges
	portBindings := s.buildPortBindings(hl7Range, httpRange, containerInfo)

	// Stop the container
	log.Printf("🔄 DockerPortService: stopping %s...", containerName)
	if stopResp, err := s.dockerPost("/containers/"+containerName+"/stop?t=10", nil); err == nil {
		stopResp.Body.Close()
	}

	// Wait for it to stop
	time.Sleep(3 * time.Second)

	// Remove the container
	log.Printf("🗑️  DockerPortService: removing %s...", containerName)
	if delResp, err := s.dockerDelete("/containers/" + containerName + "?force=true"); err == nil {
		delResp.Body.Close()
	}

	// Create new container with updated port bindings
	hostConfig, _ := containerInfo["HostConfig"].(map[string]interface{})
	if hostConfig == nil {
		hostConfig = map[string]interface{}{}
	}
	hostConfig["PortBindings"] = portBindings

	config, _ := containerInfo["Config"].(map[string]interface{})
	networkSettings, _ := containerInfo["NetworkSettings"].(map[string]interface{})
	networks, _ := networkSettings["Networks"].(map[string]interface{})

	createBody := map[string]interface{}{
		"Image":      config["Image"],
		"Env":        config["Env"],
		"Labels":     config["Labels"],
		"HostConfig": hostConfig,
		"NetworkingConfig": map[string]interface{}{
			"EndpointsConfig": networks,
		},
	}

	createJSON, _ := json.Marshal(createBody)
	log.Printf("🚀 DockerPortService: creating %s with updated ports...", containerName)
	createResp, err := s.dockerPostJSON("/containers/create?name="+containerName, createJSON)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	defer createResp.Body.Close()
	createBody2, _ := io.ReadAll(createResp.Body)
	if createResp.StatusCode != http.StatusCreated {
		return fmt.Errorf("create container: HTTP %d: %s", createResp.StatusCode, string(createBody2))
	}

	var created struct {
		ID string `json:"Id"`
	}
	json.Unmarshal(createBody2, &created) //nolint:errcheck

	// Start the new container
	startResp, err := s.dockerPost("/containers/"+created.ID+"/start", nil)
	if err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	startResp.Body.Close()

	log.Printf("✅ DockerPortService: %s restarted with new port ranges (HL7: %s, HTTP: %s)", containerName, hl7Range, httpRange)
	return nil
}

// buildPortBindings constructs a Docker PortBindings map from the HL7 and HTTP ranges,
// preserving existing fixed-port bindings (3000, 8080, 2575) from the running container.
func (s *DockerPortService) buildPortBindings(hl7Range, httpRange string, containerInfo map[string]interface{}) map[string]interface{} {
	bindings := map[string]interface{}{
		"3000/tcp": []map[string]string{{"HostIp": "0.0.0.0", "HostPort": "3000"}},
		"8080/tcp": []map[string]string{{"HostIp": "0.0.0.0", "HostPort": "8080"}},
		"2575/tcp": []map[string]string{{"HostIp": "0.0.0.0", "HostPort": "2575"}},
	}

	addRange := func(portRange string) {
		parts := strings.SplitN(portRange, "-", 2)
		if len(parts) != 2 {
			return
		}
		var start, end int
		fmt.Sscanf(parts[0], "%d", &start)
		fmt.Sscanf(parts[1], "%d", &end)
		for p := start; p <= end; p++ {
			key := fmt.Sprintf("%d/tcp", p)
			bindings[key] = []map[string]string{{"HostIp": "0.0.0.0", "HostPort": fmt.Sprintf("%d", p)}}
		}
	}

	addRange(hl7Range)
	addRange(httpRange)
	return bindings
}

// ─── Docker HTTP API helpers ───────────────────────────────────────────────────

func (s *DockerPortService) dockerGet(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, "http://localhost/v1.43"+path, nil)
	if err != nil {
		return nil, err
	}
	return s.httpClient.Do(req)
}

func (s *DockerPortService) dockerPost(path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, "http://localhost/v1.43"+path, body)
	if err != nil {
		return nil, err
	}
	return s.httpClient.Do(req)
}

func (s *DockerPortService) dockerPostJSON(path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, "http://localhost/v1.43"+path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return s.httpClient.Do(req)
}

func (s *DockerPortService) dockerDelete(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodDelete, "http://localhost/v1.43"+path, nil)
	if err != nil {
		return nil, err
	}
	return s.httpClient.Do(req)
}
