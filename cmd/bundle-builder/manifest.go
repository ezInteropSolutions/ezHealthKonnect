package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
)

// Manifest is packaging/bundle-manifest.json: what the Standalone bundle contains.
type Manifest struct {
	Files        []string           `json:"files"`
	Dirs         []string           `json:"dirs"`
	Exclude      ExcludeRules       `json:"exclude"`
	WindowsFiles map[string]string  `json:"windowsFiles"`
	Protected    []string           `json:"protected"`
	Runtimes     map[string]Runtime `json:"runtimes"`
	// InstallerDownloads are fetched by the installer at install time (never
	// bundled) and verified against the pinned SHA-256 before use.
	InstallerDownloads map[string]InstallerDownload `json:"installerDownloads"`
}

// InstallerDownload is a pinned file the installer may download.
type InstallerDownload struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
}

// ExcludeRules drop tracked files that are not needed at runtime.
type ExcludeRules struct {
	Suffixes []string `json:"suffixes"`
	DirNames []string `json:"dirNames"`
	Files    []string `json:"files"`
}

// Runtime is a third-party runtime downloaded at build time and verified
// against a pinned SHA-256.
type Runtime struct {
	Version string                     `json:"version"`
	Targets map[string]RuntimeArtifact `json:"targets"` // "linux/amd64", ...
}

// RuntimeArtifact is one downloadable file.
type RuntimeArtifact struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// LoadManifest reads and validates a manifest file.
func LoadManifest(p string) (*Manifest, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return &m, m.Validate()
}

// Validate rejects a manifest that would ship customer data or is ambiguous.
func (m *Manifest) Validate() error {
	protected := map[string]bool{}
	for _, p := range m.Protected {
		protected[p] = true
	}
	for _, e := range m.TopLevelEntries() {
		if protected[e] {
			return fmt.Errorf("manifest ships %q, which is protected customer data", e)
		}
	}
	for _, d := range m.Dirs {
		if d == "" || strings.HasPrefix(d, "/") || strings.Contains(d, "..") {
			return fmt.Errorf("invalid manifest dir %q", d)
		}
	}
	for _, f := range m.Files {
		if f == "" || strings.HasPrefix(f, "/") || strings.Contains(f, "..") {
			return fmt.Errorf("invalid manifest file %q", f)
		}
	}
	if len(m.Protected) == 0 {
		return fmt.Errorf("manifest must list protected data entries")
	}
	return nil
}

// TopLevelEntries returns the bundle's top-level names from files, dirs and
// Windows-only files (runtime/, go-api and node_modules are added by the
// builder itself).
func (m *Manifest) TopLevelEntries() []string {
	set := map[string]bool{}
	add := func(p string) { set[strings.SplitN(p, "/", 2)[0]] = true }
	for _, f := range m.Files {
		add(f)
	}
	for _, d := range m.Dirs {
		add(d)
	}
	for dst := range m.WindowsFiles {
		add(dst)
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Includes reports whether a tracked repository path belongs in the bundle.
func (m *Manifest) Includes(p string) bool {
	return m.listed(p) && !m.Excluded(p)
}

// Excluded reports whether a path is dropped by the exclusion rules (Go
// source, tests, fixtures) regardless of whether it is listed.
func (m *Manifest) Excluded(p string) bool {
	for _, f := range m.Exclude.Files {
		if p == f || path.Base(p) == f {
			return true
		}
	}
	for _, s := range m.Exclude.Suffixes {
		if strings.HasSuffix(p, s) {
			return true
		}
	}
	for _, seg := range strings.Split(path.Dir(p), "/") {
		for _, d := range m.Exclude.DirNames {
			if seg == d {
				return true
			}
		}
	}
	return false
}

func (m *Manifest) listed(p string) bool {
	for _, f := range m.Files {
		if p == f {
			return true
		}
	}
	for _, d := range m.Dirs {
		if strings.HasPrefix(p, d+"/") {
			return true
		}
	}
	return false
}
