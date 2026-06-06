package main

// standalone_shared.go — utilities shared across all OS standalone installers.
// No build tag — compiled on every platform.

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const releasesBase = "https://github.com/ezInteropSolutions/ezHealthKonnect/releases"

// http1Client forces HTTP/1.1 for all installer downloads.
// EDB's CDN (and some GitHub CDN edges) drop large HTTP/2 streams with
// INTERNAL_ERROR after a few MB — HTTP/1.1 is slower to negotiate but never
// triggers that bug.
var http1Client = &http.Client{
	Transport: &http.Transport{
		TLSNextProto:          make(map[string]func(string, *tls.Conn) http.RoundTripper),
		ResponseHeaderTimeout: 60 * time.Second,
	},
}

// downloadFileProgress downloads url to dest, showing progress every 5 s.
// Retries up to 3 times on error.
func downloadFileProgress(url, dest string) error {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			emit("warn", fmt.Sprintf("  Download error: %v — retrying (%d/%d)...", lastErr, attempt, maxAttempts))
			time.Sleep(4 * time.Second)
		}
		lastErr = downloadOnce(url, dest)
		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}

func downloadOnce(url, dest string) error {
	req, err := http.NewRequest("GET", url, nil) //nolint:gosec
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ezHealthKonnect-Installer/1.0")

	resp, err := http1Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 64*1024)
	last := time.Now()

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			downloaded += int64(n)
			if time.Since(last) > 5*time.Second {
				if total > 0 {
					emit("info", fmt.Sprintf("  Downloading... %.0f%% (%.1f / %.1f MB)",
						float64(downloaded)/float64(total)*100,
						float64(downloaded)/1e6, float64(total)/1e6))
				} else {
					emit("info", fmt.Sprintf("  Downloading... %.1f MB", float64(downloaded)/1e6))
				}
				last = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	return nil
}

// resolveLatestTag calls the GitHub Releases API to find the current latest tag.
func resolveLatestTag() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second, Transport: http1Client.Transport}
	req, err := http.NewRequest("GET",
		"https://api.github.com/repos/ezInteropSolutions/ezHealthKonnect/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.TagName == "" {
		return "", fmt.Errorf("empty tag_name in GitHub API response")
	}
	return result.TagName, nil
}

// pgAdminPassword returns the PostgreSQL superuser password to use for admin commands.
func pgAdminPassword() string {
	if p := os.Getenv("PGPASSWORD"); p != "" {
		return p
	}
	return "postgres"
}

// runMigrationsWithPsql runs SQL migrations in numeric version order using the given psql binary.
func runMigrationsWithPsql(psqlBin, migDir, host, port, dbName, dbUser, dbPassword string) error {
	entries, err := os.ReadDir(migDir)
	if err != nil {
		return fmt.Errorf("migrations dir not found: %w", err)
	}

	// os.ReadDir sorts alphabetically, which puts V100 before V10 before V1.
	// Extract the integer version and sort numerically so V1→V2→…→VN.
	type mig struct {
		version int
		name    string
	}
	var migs []mig
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "V") || !strings.HasSuffix(name, ".sql") {
			continue
		}
		rest := strings.TrimPrefix(name, "V")
		vStr := strings.SplitN(rest, "__", 2)[0]
		v, err := strconv.Atoi(vStr)
		if err != nil {
			emit("warn", "  Skipping unrecognised migration file: "+name)
			continue
		}
		migs = append(migs, mig{version: v, name: name})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })

	emit("info", fmt.Sprintf("  Running %d migrations in version order...", len(migs)))
	failed := 0
	for _, m := range migs {
		sqlFile := filepath.Join(migDir, m.name)
		cmd := exec.Command(psqlBin, "-h", host, "-U", dbUser, "-p", port, "-d", dbName,
			"-f", sqlFile, "-v", "ON_ERROR_STOP=0")
		cmd.Env = append(os.Environ(), "PGPASSWORD="+dbPassword)
		out, err := cmd.CombinedOutput()
		if err != nil {
			output := strings.TrimSpace(string(out))
			if !strings.Contains(output, "already exists") {
				emit("warn", fmt.Sprintf("  V%d: %s", m.version, output))
				failed++
			}
		}
	}

	if failed > 0 {
		emit("warn", fmt.Sprintf("  %d migration(s) had errors (logged above)", failed))
	} else {
		emit("ok", fmt.Sprintf("  All %d migrations applied successfully", len(migs)))
	}
	return nil
}

// readEnvFile parses a .env file and returns key→value pairs (comments and blank lines ignored).
func readEnvFile(path string) map[string]string {
	result := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		result[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return result
}
