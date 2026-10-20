package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialKeyFor_FirstInstallGenerates32ByteKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env") // does not exist
	key := credentialKeyFor(path)
	raw, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil {
		t.Fatalf("generated key is not URL-safe base64: %v", err)
	}
	if len(raw) != 32 {
		t.Fatalf("generated key decodes to %d bytes, want 32", len(raw))
	}
	if other := credentialKeyFor(path); other == key {
		t.Fatal("two first-install keys must differ")
	}
}

func TestCredentialKeyFor_ReinstallKeepsExistingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	const existing = "ValmmzmZkn3P1lmGAL4GylNB8o7A+Knm0SpyRQIoSzc="
	content := "PORT=3000\nAPP_CREDENTIAL_KEY=" + existing + "\nJWT_SECRET=x\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if got := credentialKeyFor(path); got != existing {
		t.Fatalf("re-install must keep the existing key, got %q", got)
	}
}

func TestExistingEnvValue_StripsQuotesAndMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte("A=\"quoted\"\n# APP_CREDENTIAL_KEY=commented\n"), 0600) //nolint:errcheck
	if got := existingEnvValue(path, "A"); got != "quoted" {
		t.Errorf("got %q, want quoted", got)
	}
	if got := existingEnvValue(path, credentialKeyEnvVar); got != "" {
		t.Errorf("a commented-out key must not be reused, got %q", got)
	}
	if got := existingEnvValue(filepath.Join(t.TempDir(), "missing"), "A"); got != "" {
		t.Errorf("missing file must return empty, got %q", got)
	}
}
