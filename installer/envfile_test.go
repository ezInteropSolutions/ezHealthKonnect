package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnvFile_RoundTripKeepsCommentsAndOrder(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(p, []byte("# header\nA=1\n\nB=\"quoted\"\r\n# note\nC=x=y\n"), 0o600)
	e, err := ReadEnvFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if e.Get("B") != "quoted" || e.Get("C") != "x=y" || strings.Join(e.Keys(), ",") != "A,B,C" {
		t.Fatalf("parsed %v", e.Map())
	}
	if e.SetIfMissing("A", "changed") {
		t.Error("SetIfMissing must not replace an existing value")
	}
	e.SetIfMissing("D", "4")
	if err := e.WriteFile(p); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "# header\nA=1\n\nB=quoted\n# note\nC=x=y\nD=4\n" {
		t.Errorf("rewritten file:\n%s", got)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(p)
		if st.Mode().Perm() != 0o600 {
			t.Errorf("mode %v", st.Mode().Perm())
		}
	}
}

func TestCredentialKeyFor_KeepsExistingAndGeneratesValidKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	k := credentialKeyFor(p)
	raw, err := base64.RawURLEncoding.DecodeString(k)
	if err != nil || len(raw) != 32 {
		t.Fatalf("new key %q is not 32 bytes of URL-safe base64", k)
	}
	if credentialKeyFor(p) == k {
		t.Error("two first installs must not share a key")
	}
	os.WriteFile(p, []byte(credentialKeyEnvVar+"=existing-key\n"), 0o600)
	if credentialKeyFor(p) != "existing-key" {
		t.Error("an existing key must be kept")
	}
}

func TestRandomPasswordIsSafeEverywhere(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p := randomPassword()
		if len(p) != 40 || strings.Trim(p, "0123456789abcdef") != "" || seen[p] {
			t.Fatalf("password %q", p)
		}
		seen[p] = true
	}
}
