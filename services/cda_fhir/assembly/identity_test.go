package assembly

import (
	"testing"
)

func TestIdentityKey_IsZero(t *testing.T) {
	if !(IdentityKey{}).IsZero() {
		t.Error("empty key should be zero")
	}
	if (IdentityKey{Root: "2.16"}).IsZero() {
		t.Error("key with root should not be zero")
	}
	if (IdentityKey{Extension: "123"}).IsZero() {
		t.Error("key with extension should not be zero")
	}
}

func TestIdentityKey_RegistryKey(t *testing.T) {
	k := IdentityKey{Root: "2.16.840.1.113883.4.6", Extension: "1013027903"}
	got := k.RegistryKey("Practitioner")
	want := "Practitioner|2.16.840.1.113883.4.6:1013027903"
	if got != want {
		t.Errorf("RegistryKey: got %q want %q", got, want)
	}
}

func TestExtractIdentityKeys_roundtrip(t *testing.T) {
	r := map[string]interface{}{}
	ids := []map[string]interface{}{
		{"root": "2.16.840.1.113883.4.6", "extension": "1013027903"},
		{"root": "2.16.840.1.113883.4.2", "extension": "999-00-1234"},
		{"root": "", "extension": ""}, // should be skipped
	}
	EmbedIdentityKeys(r, ids)

	keys := ExtractIdentityKeys(r)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0].Root != "2.16.840.1.113883.4.6" || keys[0].Extension != "1013027903" {
		t.Errorf("key[0] wrong: %+v", keys[0])
	}
	if keys[1].Root != "2.16.840.1.113883.4.2" || keys[1].Extension != "999-00-1234" {
		t.Errorf("key[1] wrong: %+v", keys[1])
	}
}

func TestExtractIdentityKeys_missing(t *testing.T) {
	r := map[string]interface{}{"resourceType": "Practitioner"}
	if keys := ExtractIdentityKeys(r); keys != nil {
		t.Errorf("expected nil, got %v", keys)
	}
}
