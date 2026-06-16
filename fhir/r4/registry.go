package r4

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// RegistryKey — unique identifier for a compiled profile
// ─────────────────────────────────────────────────────────────────────────────

// RegistryKey uniquely identifies a compiled FHIR profile.
type RegistryKey struct {
	Version      string // "R4", "R5", …
	ResourceType string // "Patient", "Observation", …
	Profile      string // "base", "us-core", …
}

func (k RegistryKey) String() string {
	return fmt.Sprintf("%s/%s/%s", k.Version, k.ResourceType, k.Profile)
}

// ─────────────────────────────────────────────────────────────────────────────
// RegistryStats — loader diagnostics
// ─────────────────────────────────────────────────────────────────────────────

// RegistryStats contains diagnostic information about registry initialisation.
type RegistryStats struct {
	TotalProfiles  int            `json:"totalProfiles"`
	VersionCounts  map[string]int `json:"versionCounts"`  // "R4" → 172
	ProfileCounts  map[string]int `json:"profileCounts"`  // "us-core" → 26
	LoadErrors     int            `json:"loadErrors"`
	InitDurationMs int64          `json:"initDurationMs"`
	SchemaDir      string         `json:"schemaDir"`
}

// ─────────────────────────────────────────────────────────────────────────────
// SchemaRegistry — compile-once, read-many profile store
// ─────────────────────────────────────────────────────────────────────────────

// SchemaRegistry holds all pre-compiled FHIR profiles indexed by RegistryKey.
// It implements ProfileProvider.
//
// Thread-safety model: after InitRegistry() returns, profiles is never mutated
// by normal operation; mu is only taken on Reload() (hot-reload via SIGHUP).
type SchemaRegistry struct {
	profiles  map[string]*CompiledProfile // key: RegistryKey.String()
	mu        sync.RWMutex
	schemaDir string
	stats     RegistryStats
}

var (
	globalRegistry     *SchemaRegistry
	globalRegistryOnce sync.Once
)

// ─────────────────────────────────────────────────────────────────────────────
// InitRegistry — called once at startup (idempotent via sync.Once)
// ─────────────────────────────────────────────────────────────────────────────

// InitRegistry scans schemaDir for all FHIR schema files, compiles every
// profile into memory, and stores them in the global registry.
//
// Directory layout expected:
//
//	schemaDir/
//	  R4/
//	    resources/        ← base profiles (Patient.gz, Observation.gz, …)
//	    profiles/
//	      us-core/        ← named profiles
//	      au-base/        ← future named profiles
//	  R5/
//	    resources/        ← R5 base profiles (added later, zero code changes)
//	    profiles/…
//
// If schemaDir does not exist or contains no files, InitRegistry logs a warning
// and returns nil — the validator falls back to basic (type-only) validation.
func InitRegistry(schemaDir string) error {
	var initErr error
	globalRegistryOnce.Do(func() {
		initErr = doInitRegistry(schemaDir)
	})
	return initErr
}

// ForceReinit bypasses sync.Once and re-initialises the registry; intended for
// testing only.
func ForceReinit(schemaDir string) error {
	globalRegistryOnce = sync.Once{}
	return InitRegistry(schemaDir)
}

func doInitRegistry(schemaDir string) error {
	start := time.Now()

	if _, err := os.Stat(schemaDir); os.IsNotExist(err) {
		log.Printf("⚠️  [r4.Registry] Schema directory not found: %s — validation runs in basic mode", schemaDir)
		globalRegistry = &SchemaRegistry{
			profiles:  make(map[string]*CompiledProfile),
			schemaDir: schemaDir,
			stats:     RegistryStats{SchemaDir: schemaDir, VersionCounts: map[string]int{}, ProfileCounts: map[string]int{}},
		}
		return nil
	}

	r := &SchemaRegistry{
		profiles:  make(map[string]*CompiledProfile, 256),
		schemaDir: schemaDir,
		stats: RegistryStats{
			SchemaDir:     schemaDir,
			VersionCounts: make(map[string]int),
			ProfileCounts: make(map[string]int),
		},
	}

	checker := globalConstraintRegistry // may be nil if called before constraints init
	r.scanAndCompile(schemaDir, checker)

	// Load terminology ValueSet files from {schemaDir}/R4/valuesets/
	vsDir := filepath.Join(schemaDir, "R4", "valuesets")
	if err := InitTerminology(vsDir); err != nil {
		log.Printf("⚠️  [r4.Registry] Terminology load warning: %v", err)
	}

	r.stats.TotalProfiles = len(r.profiles)
	r.stats.InitDurationMs = time.Since(start).Milliseconds()

	globalRegistry = r

	log.Printf("✅ [r4.Registry] Initialized: %d profiles in %dms (errors: %d)",
		r.stats.TotalProfiles, r.stats.InitDurationMs, r.stats.LoadErrors)
	for v, cnt := range r.stats.VersionCounts {
		log.Printf("   %s: %d base profiles", v, cnt)
	}
	for p, cnt := range r.stats.ProfileCounts {
		log.Printf("   profile %q: %d resources", p, cnt)
	}

	return nil
}

// scanAndCompile walks the schema directory tree and compiles every .gz file it finds.
func (r *SchemaRegistry) scanAndCompile(schemaDir string, checker ConstraintChecker) {
	// Walk top-level entries — each is a version directory (R4, R5, …).
	entries, err := os.ReadDir(schemaDir)
	if err != nil {
		log.Printf("❌ [r4.Registry] Cannot read schema dir %s: %v", schemaDir, err)
		r.stats.LoadErrors++
		return
	}

	for _, vEntry := range entries {
		if !vEntry.IsDir() {
			continue
		}
		version := vEntry.Name() // "R4", "R5", …
		versionDir := filepath.Join(schemaDir, version)

		// ── Base profiles: {version}/resources/*.gz ───────────────────────────
		r.compileDir(filepath.Join(versionDir, "resources"), version, "base", checker)

		// ── Named profiles: {version}/profiles/{name}/*.gz ────────────────────
		profilesDir := filepath.Join(versionDir, "profiles")
		if pEntries, err := os.ReadDir(profilesDir); err == nil {
			for _, pEntry := range pEntries {
				if !pEntry.IsDir() {
					continue
				}
				profileName := pEntry.Name() // "us-core", "au-base", …
				r.compileDir(filepath.Join(profilesDir, profileName), version, profileName, checker)
			}
		}
	}
}

// compileDir loads and compiles every .gz file in dir as a profile for (version, profileName).
func (r *SchemaRegistry) compileDir(dir, version, profileName string, checker ConstraintChecker) {
	files, err := filepath.Glob(filepath.Join(dir, "*.gz"))
	if err != nil || len(files) == 0 {
		return
	}

	for _, filePath := range files {
		raw, err := loadRawSchema(filePath)
		if err != nil {
			log.Printf("⚠️  [r4.Registry] Load error %s: %v", filePath, err)
			r.stats.LoadErrors++
			continue
		}
		if raw.ResourceType == "" {
			// Infer from filename (Patient.gz → Patient)
			raw.ResourceType = strings.TrimSuffix(filepath.Base(filePath), ".gz")
		}

		cp := compileSchema(raw, version, profileName, checker)
		key := RegistryKey{version, cp.ResourceType, profileName}.String()

		r.profiles[key] = cp

		if profileName == "base" {
			r.stats.VersionCounts[version]++
		} else {
			r.stats.ProfileCounts[profileName]++
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ProfileProvider implementation
// ─────────────────────────────────────────────────────────────────────────────

// GetRegistry returns the global SchemaRegistry.  May be nil before InitRegistry
// is called (validator handles this gracefully).
func GetRegistry() *SchemaRegistry {
	return globalRegistry
}

// Get returns the CompiledProfile for the given (version, resourceType, profile) triple.
// Empty strings default to "R4" and "base" respectively.
func (r *SchemaRegistry) Get(version, resourceType, profile string) (*CompiledProfile, bool) {
	if r == nil {
		// InitRegistry() runs in a background goroutine; a validator built
		// before it completes can be holding a not-yet-populated registry.
		return nil, false
	}
	if version == "" {
		version = "R4"
	}
	if profile == "" {
		profile = "base"
	}
	key := RegistryKey{version, resourceType, profile}.String()
	r.mu.RLock()
	cp, ok := r.profiles[key]
	r.mu.RUnlock()
	return cp, ok
}

// List returns all RegistryKeys registered for version.
func (r *SchemaRegistry) List(version string) []RegistryKey {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var keys []RegistryKey
	prefix := version + "/"
	for k := range r.profiles {
		if strings.HasPrefix(k, prefix) {
			parts := strings.SplitN(k, "/", 3)
			if len(parts) == 3 {
				keys = append(keys, RegistryKey{parts[0], parts[1], parts[2]})
			}
		}
	}
	return keys
}

// GetStats returns a snapshot of registry diagnostics.
func (r *SchemaRegistry) GetStats() RegistryStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.stats
}

// ─────────────────────────────────────────────────────────────────────────────
// Reload — hot-reload without service restart (SIGHUP handler)
// ─────────────────────────────────────────────────────────────────────────────

// Reload re-scans and recompiles all schema files, then atomically swaps the
// profile map.  In-flight validations using the old map are unaffected.
func (r *SchemaRegistry) Reload() error {
	start := time.Now()
	fresh := make(map[string]*CompiledProfile, len(r.profiles))
	stats := RegistryStats{
		SchemaDir:     r.schemaDir,
		VersionCounts: make(map[string]int),
		ProfileCounts: make(map[string]int),
	}

	tmp := &SchemaRegistry{profiles: fresh, schemaDir: r.schemaDir, stats: stats}
	tmp.scanAndCompile(r.schemaDir, globalConstraintRegistry)

	r.mu.Lock()
	r.profiles = fresh
	r.stats = tmp.stats
	r.stats.TotalProfiles = len(fresh)
	r.stats.InitDurationMs = time.Since(start).Milliseconds()
	r.mu.Unlock()

	log.Printf("🔄 [r4.Registry] Reloaded: %d profiles in %dms", len(fresh), time.Since(start).Milliseconds())
	return nil
}
