package assembly

// ResourceRegistry tracks FHIR resources by IdentityKey for cross-section deduplication.
type ResourceRegistry interface {
	// Register adds a resource under all its identity keys. Returns true when the resource
	// was added (first occurrence), false when a duplicate was detected (already registered).
	Register(resourceType string, keys []IdentityKey, resource map[string]interface{}) bool

	// FindDuplicate returns the first previously registered resource that shares at least
	// one identity key with the given keys, and the matching key string. Returns nil/""
	// when no duplicate exists.
	FindDuplicate(resourceType string, keys []IdentityKey) (existing map[string]interface{}, matchKey string)

	// Replace swaps a previously-registered survivor for a richer duplicate sharing
	// at least one identity key with it. Re-points every key belonging to EITHER
	// resource (oldResource's own keys too, not just newResource's) at newResource,
	// so a future resource matching via a key oldResource had but newResource
	// doesn't still correctly finds the current survivor.
	Replace(resourceType string, oldResource, newResource map[string]interface{})

	// All returns all survivor resources (those that were accepted on first registration).
	All() []map[string]interface{}
}

// InMemoryResourceRegistry is a plain non-thread-safe registry backed by a map.
// Must only be used after all section goroutines have completed (i.e. after wg.Wait()).
type InMemoryResourceRegistry struct {
	byKey     map[string]map[string]interface{} // registryKey → survivor resource
	survivors []map[string]interface{}
}

// NewInMemoryResourceRegistry constructs an empty registry.
func NewInMemoryResourceRegistry() *InMemoryResourceRegistry {
	return &InMemoryResourceRegistry{
		byKey: make(map[string]map[string]interface{}),
	}
}

// Register adds the resource under every provided identity key. If any key is
// already in the registry the resource is a duplicate — the existing entry is
// NOT overwritten and the function returns false.
func (reg *InMemoryResourceRegistry) Register(resourceType string, keys []IdentityKey, resource map[string]interface{}) bool {
	// Check for any existing entry first (one pass).
	for _, k := range keys {
		if k.IsZero() {
			continue
		}
		if _, exists := reg.byKey[k.RegistryKey(resourceType)]; exists {
			return false // duplicate detected; caller uses FindDuplicate to get survivor
		}
	}

	// All keys are new — register and record survivor.
	for _, k := range keys {
		if k.IsZero() {
			continue
		}
		reg.byKey[k.RegistryKey(resourceType)] = resource
	}
	reg.survivors = append(reg.survivors, resource)
	return true
}

// FindDuplicate returns the survivor for the first identity key already in the registry.
func (reg *InMemoryResourceRegistry) FindDuplicate(resourceType string, keys []IdentityKey) (map[string]interface{}, string) {
	for _, k := range keys {
		if k.IsZero() {
			continue
		}
		rk := k.RegistryKey(resourceType)
		if existing, found := reg.byKey[rk]; found {
			return existing, rk
		}
	}
	return nil, ""
}

// Replace re-extracts both resources' own identity keys (rather than requiring the
// caller to pass them) so every key either resource was registered under -- not just
// the ones the caller happened to have on hand -- ends up pointing at newResource.
// Without re-pointing oldResource's own keys too, a later resource matching via a key
// oldResource had but newResource doesn't would Register() as a brand new survivor
// instead of correctly finding the current one.
func (reg *InMemoryResourceRegistry) Replace(resourceType string, oldResource, newResource map[string]interface{}) {
	for _, k := range ExtractIdentityKeys(oldResource) {
		if k.IsZero() {
			continue
		}
		reg.byKey[k.RegistryKey(resourceType)] = newResource
	}
	for _, k := range ExtractIdentityKeys(newResource) {
		if k.IsZero() {
			continue
		}
		reg.byKey[k.RegistryKey(resourceType)] = newResource
	}
	for i := range reg.survivors {
		if sameResource(reg.survivors[i], oldResource) {
			reg.survivors[i] = newResource
			return
		}
	}
}

// sameResource compares by resourceType+id rather than pointer identity -- the
// resource maps flowing through this package are sometimes copied (see
// AssemblyContext.Resources' own "append([]map[string]interface{}(nil), allResources...)"
// shallow copy in declarative_document_mapper.go), so a pointer-equality check would
// silently fail to find the entry to replace.
func sameResource(a, b map[string]interface{}) bool {
	at, _ := a["resourceType"].(string)
	bt, _ := b["resourceType"].(string)
	aid, _ := a["id"].(string)
	bid, _ := b["id"].(string)
	return at == bt && aid == bid && at != ""
}

// All returns the survivors slice (first-registered resource per identity key set).
func (reg *InMemoryResourceRegistry) All() []map[string]interface{} {
	return reg.survivors
}
