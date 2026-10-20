package main

// env_secrets.go — secrets that must survive an installer re-run.
//
// Every installer path (Docker, standalone Linux, standalone macOS) rewrites
// .env unconditionally. That is acceptable for session/JWT secrets (users are
// simply logged out) but NOT for APP_CREDENTIAL_KEY: it encrypts every stored
// connector credential (services/credential_store.go), so replacing it on an
// upgrade would make all of them undecryptable. Before 2026-10-19 no installer
// set APP_CREDENTIAL_KEY at all, so credential encryption silently ran in
// plaintext passthrough mode on customer installs.

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"os"
	"strings"
)

const credentialKeyEnvVar = "APP_CREDENTIAL_KEY"

// existingEnvValue returns the value of key in the .env file at path, or ""
// when the file or key is absent. Surrounding quotes are stripped.
func existingEnvValue(path, key string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	prefix := key + "="
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, prefix) {
			return strings.Trim(strings.TrimPrefix(line, prefix), `"'`)
		}
	}
	return ""
}

// credentialKeyFor returns the APP_CREDENTIAL_KEY already present in the .env
// at path (an upgrade/re-run — must be kept), or a new random 32-byte key for a
// first install. URL-safe unpadded base64 matches randomSecret's .env-safe
// format; the app accepts both standard and URL-safe encodings.
func credentialKeyFor(path string) string {
	if existing := existingEnvValue(path, credentialKeyEnvVar); existing != "" {
		return existing
	}
	b := make([]byte, 32)
	rand.Read(b) //nolint:errcheck
	return base64.RawURLEncoding.EncodeToString(b)
}
