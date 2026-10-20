package main

// envfile.go — the installation's .env (configuration + secrets).
//
// Rules that make reinstall/upgrade safe:
//   - an existing value is NEVER replaced by the installer (DB password,
//     session/JWT secrets, APP_CREDENTIAL_KEY — replacing the credential key
//     would make every stored connector credential unreadable);
//   - keys a newer version needs are added when missing;
//   - the file is written atomically and readable only by its owner.

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const credentialKeyEnvVar = "APP_CREDENTIAL_KEY"

// EnvFile is an ordered .env document. Comment and blank lines are kept.
type EnvFile struct {
	lines []envLine
}

type envLine struct {
	key, value string // key == "" for comments/blank lines
	raw        string
}

// ReadEnvFile parses path. A missing file yields an empty EnvFile.
func ReadEnvFile(path string) (*EnvFile, error) {
	e := &EnvFile{}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return e, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			e.lines = append(e.lines, envLine{raw: line})
			continue
		}
		k, v, ok := strings.Cut(trimmed, "=")
		if !ok {
			e.lines = append(e.lines, envLine{raw: line})
			continue
		}
		e.lines = append(e.lines, envLine{key: strings.TrimSpace(k), value: strings.Trim(strings.TrimSpace(v), `"'`)})
	}
	return e, sc.Err()
}

// Get returns a value ("" when absent).
func (e *EnvFile) Get(key string) string {
	for _, l := range e.lines {
		if l.key == key {
			return l.value
		}
	}
	return ""
}

// Has reports whether key is present with a non-empty value.
func (e *EnvFile) Has(key string) bool { return e.Get(key) != "" }

// Set sets key, replacing an existing value or appending.
func (e *EnvFile) Set(key, value string) {
	for i, l := range e.lines {
		if l.key == key {
			e.lines[i].value = value
			return
		}
	}
	e.lines = append(e.lines, envLine{key: key, value: value})
}

// SetIfMissing sets key only when it is absent or empty. Returns true when set.
func (e *EnvFile) SetIfMissing(key, value string) bool {
	if e.Has(key) {
		return false
	}
	e.Set(key, value)
	return true
}

// Comment appends a comment line (used when generating a new file).
func (e *EnvFile) Comment(text string) {
	if text == "" {
		e.lines = append(e.lines, envLine{raw: ""})
		return
	}
	e.lines = append(e.lines, envLine{raw: "# " + text})
}

// Keys lists the keys in file order.
func (e *EnvFile) Keys() []string {
	var out []string
	for _, l := range e.lines {
		if l.key != "" {
			out = append(out, l.key)
		}
	}
	return out
}

// Map returns key -> value.
func (e *EnvFile) Map() map[string]string {
	m := map[string]string{}
	for _, l := range e.lines {
		if l.key != "" {
			m[l.key] = l.value
		}
	}
	return m
}

// String renders the file.
func (e *EnvFile) String() string {
	var b strings.Builder
	for _, l := range e.lines {
		if l.key == "" {
			b.WriteString(l.raw)
		} else {
			b.WriteString(l.key + "=" + l.value)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// WriteFile writes the file atomically with mode 0600.
func (e *EnvFile) WriteFile(path string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".env.tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(e.String()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ── Secret generation ─────────────────────────────────────────────────────────

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err)) // never continue with weak secrets
	}
	return b
}

// randomSecret returns n random bytes as URL-safe base64 (safe in .env files).
func randomSecret(n int) string { return base64.RawURLEncoding.EncodeToString(randomBytes(n)) }

// randomPassword returns a hex password: safe in connection strings, URLs,
// shell arguments and SQL literals.
func randomPassword() string { return hex.EncodeToString(randomBytes(20)) }

// newCredentialKey returns a 32-byte AES key (URL-safe base64; the app
// accepts standard and URL-safe encodings).
func newCredentialKey() string { return base64.RawURLEncoding.EncodeToString(randomBytes(32)) }

// credentialKeyFor returns the APP_CREDENTIAL_KEY already in the .env at
// path, or a new key for a first install.
func credentialKeyFor(path string) string {
	if e, err := ReadEnvFile(path); err == nil && e.Has(credentialKeyEnvVar) {
		return e.Get(credentialKeyEnvVar)
	}
	return newCredentialKey()
}
