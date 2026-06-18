package assembly

import (
	"testing"
)

func newRes(rt, id string, keys []IdentityKey) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": rt,
		"id":           id,
	}
	ids := make([]map[string]interface{}, 0, len(keys))
	for _, k := range keys {
		ids = append(ids, map[string]interface{}{"root": k.Root, "extension": k.Extension})
	}
	EmbedIdentityKeys(r, ids)
	return r
}

func TestRegistry_FirstOccurrenceAccepted(t *testing.T) {
	reg := NewInMemoryResourceRegistry()
	keys := []IdentityKey{{Root: "2.16", Extension: "npi-1"}}
	res := newRes("Practitioner", "pr-1", keys)

	if !reg.Register("Practitioner", keys, res) {
		t.Error("first register should return true")
	}
	if len(reg.All()) != 1 {
		t.Errorf("expected 1 survivor, got %d", len(reg.All()))
	}
}

func TestRegistry_DuplicateRejected(t *testing.T) {
	reg := NewInMemoryResourceRegistry()
	keys := []IdentityKey{{Root: "2.16", Extension: "npi-1"}}

	res1 := newRes("Practitioner", "pr-1", keys)
	res2 := newRes("Practitioner", "pr-2", keys)

	if !reg.Register("Practitioner", keys, res1) {
		t.Error("first register should return true")
	}
	if reg.Register("Practitioner", keys, res2) {
		t.Error("second register with same key should return false")
	}
	if len(reg.All()) != 1 {
		t.Errorf("expected 1 survivor, got %d", len(reg.All()))
	}
}

func TestRegistry_FindDuplicate(t *testing.T) {
	reg := NewInMemoryResourceRegistry()
	keys := []IdentityKey{{Root: "2.16", Extension: "npi-1"}}
	res1 := newRes("Practitioner", "pr-1", keys)
	reg.Register("Practitioner", keys, res1)

	existing, matchKey := reg.FindDuplicate("Practitioner", keys)
	if existing == nil {
		t.Fatal("expected FindDuplicate to return existing resource")
	}
	if existing["id"] != "pr-1" {
		t.Errorf("expected survivor id pr-1, got %v", existing["id"])
	}
	if matchKey == "" {
		t.Error("expected non-empty matchKey")
	}
}

func TestRegistry_DifferentResourceTypesIsolated(t *testing.T) {
	reg := NewInMemoryResourceRegistry()
	keys := []IdentityKey{{Root: "2.16", Extension: "123"}}

	res1 := newRes("Practitioner", "pr-1", keys)
	res2 := newRes("Organization", "org-1", keys)

	if !reg.Register("Practitioner", keys, res1) {
		t.Error("practitioner register should succeed")
	}
	if !reg.Register("Organization", keys, res2) {
		t.Error("organization with same key should also succeed (different type)")
	}
	if len(reg.All()) != 2 {
		t.Errorf("expected 2 survivors, got %d", len(reg.All()))
	}
}
