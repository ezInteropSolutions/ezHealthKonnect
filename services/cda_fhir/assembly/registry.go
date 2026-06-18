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

// All returns the survivors slice (first-registered resource per identity key set).
func (reg *InMemoryResourceRegistry) All() []map[string]interface{} {
	return reg.survivors
}
