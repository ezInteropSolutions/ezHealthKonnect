// Package secretguard refuses publicly-known secrets.
//
// config/compromised_secrets.json holds SHA-256 fingerprints of every secret
// value that has been public (committed to this public repository, or a
// placeholder in code/docs) plus the environment variables to check. The Node
// service applies the same list (config/SecretsGuard.js); Go checks it too
// because standalone installs run the Go service on its own.
package secretguard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// DefaultPath is the list's location relative to the app's working directory
// (the Docker image and the standalone bundles both ship config/).
const DefaultPath = "config/compromised_secrets.json"

type fileFormat struct {
	GuardedVariables []string `json:"guardedVariables"`
	Fingerprints     []struct {
		SHA256 string `json:"sha256"`
		Note   string `json:"note"`
	} `json:"fingerprints"`
}

// List is a loaded set of publicly-known secret fingerprints.
type List struct {
	guarded []string
	byHash  map[string]string
}

// Finding is one guarded variable holding a publicly-known value. It never
// carries the value itself.
type Finding struct {
	Variable string
	Note     string
}

// New builds a list from guarded variable names and fingerprint→note pairs.
func New(guarded []string, fingerprints map[string]string) *List {
	return &List{guarded: guarded, byHash: fingerprints}
}

// Load reads the fingerprint file.
func Load(path string) (*List, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f fileFormat
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	byHash := make(map[string]string, len(f.Fingerprints))
	for _, fp := range f.Fingerprints {
		byHash[strings.ToLower(fp.SHA256)] = fp.Note
	}
	return New(f.GuardedVariables, byHash), nil
}

// Fingerprint returns the hex SHA-256 of a trimmed value.
func Fingerprint(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

// Match reports whether value is publicly known (placeholders case-insensitively).
func (l *List) Match(value string) (string, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", false
	}
	if note, ok := l.byHash[Fingerprint(v)]; ok {
		return note, true
	}
	note, ok := l.byHash[Fingerprint(strings.ToLower(v))]
	return note, ok
}

// Check returns every guarded variable whose value (via getenv) is publicly known.
func (l *List) Check(getenv func(string) string) []Finding {
	var out []Finding
	for _, name := range l.guarded {
		if note, ok := l.Match(getenv(name)); ok {
			out = append(out, Finding{Variable: name, Note: note})
		}
	}
	return out
}
