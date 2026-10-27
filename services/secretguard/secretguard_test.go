package secretguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheck_FindsLeakedValuesWithoutEchoingThem(t *testing.T) {
	leaked := strings.Repeat("L", 40)
	l := New([]string{"JWT_SECRET", "APP_CREDENTIAL_KEY"}, map[string]string{
		Fingerprint(leaked):      "test leak",
		Fingerprint("change-me"): "placeholder",
	})
	env := map[string]string{"JWT_SECRET": leaked, "APP_CREDENTIAL_KEY": strings.Repeat("k", 44)}
	got := l.Check(func(k string) string { return env[k] })
	if len(got) != 1 || got[0].Variable != "JWT_SECRET" || got[0].Note != "test leak" {
		t.Fatalf("findings = %+v, want exactly JWT_SECRET", got)
	}
	if strings.Contains(got[0].Note+got[0].Variable, leaked) {
		t.Fatal("a finding must never carry the secret value")
	}
}

func TestMatch_PlaceholdersCaseInsensitiveAndTrimmed(t *testing.T) {
	l := New(nil, map[string]string{Fingerprint("change-me"): "placeholder"})
	for _, v := range []string{"change-me", "CHANGE-ME", "  change-me \n"} {
		if _, ok := l.Match(v); !ok {
			t.Errorf("%q should match", v)
		}
	}
	if _, ok := l.Match(""); ok {
		t.Error("empty value must not match")
	}
}

// The shipped list must parse and cover the app's secrets.
func TestLoad_ShippedList(t *testing.T) {
	l, err := Load(filepath.Join("..", "..", DefaultPath))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, name := range []string{"SESSION_SECRET", "JWT_SECRET", "INTERNAL_PROXY_SECRET", "APP_CREDENTIAL_KEY", "DB_PASSWORD"} {
		found := false
		for _, g := range l.guarded {
			found = found || g == name
		}
		if !found {
			t.Errorf("%s is not guarded", name)
		}
	}
	if _, ok := l.Match("dev-encryption-key"); !ok {
		t.Error("code placeholders must be in the shipped list")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json")); !os.IsNotExist(err) {
		t.Fatalf("want not-exist error, got %v", err)
	}
}
