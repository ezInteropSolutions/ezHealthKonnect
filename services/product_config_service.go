// services/product_config_service.go
// Encrypted product-internal configuration backed by the product_config table.
//
// This is NOT user-configurable — values are seeded by the official installer
// or build pipeline and read at runtime. Community / self-built installs get
// empty rows which cause features (e.g. telemetry) to silently disable.
//
// All values are AES-256-GCM encrypted using the same CredentialStore that
// protects connector credentials. The encryption key comes from APP_CREDENTIAL_KEY.
package services

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"
)

// ProductConfigService reads and writes encrypted product-internal config
// from the product_config table. It maintains an in-memory cache with a
// short TTL so hot paths (telemetry signing) don't hit the DB on every call.
type ProductConfigService struct {
	db        *sql.DB
	credStore *CredentialStore
	cache     map[string]productConfigEntry
	mu        sync.RWMutex
	cacheTTL  time.Duration
}

type productConfigEntry struct {
	value     string
	expiresAt time.Time
}

// NewProductConfigService constructs the service. credStore may be nil
// (passthrough / dev mode) — values are stored and read as plaintext.
func NewProductConfigService(db *sql.DB, credStore *CredentialStore) *ProductConfigService {
	return &ProductConfigService{
		db:        db,
		credStore: credStore,
		cache:     make(map[string]productConfigEntry),
		cacheTTL:  5 * time.Minute,
	}
}

// Get returns the plaintext value for key, or "" if not set / DB unavailable.
// Results are cached for cacheTTL to avoid repeated DB round-trips.
func (s *ProductConfigService) Get(ctx context.Context, key string) string {
	if s == nil || s.db == nil {
		return ""
	}

	// Check cache first
	s.mu.RLock()
	if entry, ok := s.cache[key]; ok && time.Now().Before(entry.expiresAt) {
		s.mu.RUnlock()
		return entry.value
	}
	s.mu.RUnlock()

	// Read from DB
	value, err := s.readFromDB(ctx, key)
	if err != nil {
		log.Printf("⚠️  ProductConfig: failed to read key %q: %v", key, err)
		return ""
	}

	// Populate cache
	s.mu.Lock()
	s.cache[key] = productConfigEntry{value: value, expiresAt: time.Now().Add(s.cacheTTL)}
	s.mu.Unlock()

	return value
}

// Set encrypts value and writes it to product_config. Clears the cache entry.
// seededBy identifies who is writing: "installer" | "migration" | "api".
func (s *ProductConfigService) Set(ctx context.Context, key, plaintext, seededBy string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("product config: database not available")
	}

	encrypted, err := s.credStore.Encrypt([]byte(plaintext))
	if err != nil {
		return fmt.Errorf("product config: encrypt %q: %w", key, err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO product_config (key, encrypted_value, seeded_by, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (key) DO UPDATE
		  SET encrypted_value = EXCLUDED.encrypted_value,
		      seeded_by       = EXCLUDED.seeded_by,
		      updated_at      = NOW()
	`, key, []byte(encrypted), seededBy)
	if err != nil {
		return fmt.Errorf("product config: write %q: %w", key, err)
	}

	// Invalidate cache
	s.mu.Lock()
	delete(s.cache, key)
	s.mu.Unlock()

	return nil
}

// SeedIfEmpty writes plaintext for key only when the row has no encrypted_value.
// Used by the default bootstrap path so official installer values are never overwritten.
func (s *ProductConfigService) SeedIfEmpty(ctx context.Context, key, plaintext, description string) {
	if s == nil || s.db == nil || plaintext == "" {
		return
	}

	// Check if a non-null value already exists
	var exists bool
	row := s.db.QueryRowContext(ctx,
		`SELECT encrypted_value IS NOT NULL AND length(encrypted_value) > 0
		 FROM product_config WHERE key = $1`, key)
	if err := row.Scan(&exists); err != nil || exists {
		return // row not found or already has a value — don't overwrite
	}

	if err := s.Set(ctx, key, plaintext, "default"); err != nil {
		log.Printf("⚠️  ProductConfig: seed %q: %v", key, err)
	}
}

// readFromDB fetches and decrypts a single key from the DB.
func (s *ProductConfigService) readFromDB(ctx context.Context, key string) (string, error) {
	var encryptedValue []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT encrypted_value FROM product_config WHERE key = $1`, key,
	).Scan(&encryptedValue)

	if err == sql.ErrNoRows || len(encryptedValue) == 0 {
		return "", nil // key not set — feature disabled
	}
	if err != nil {
		return "", err
	}

	plaintext, err := s.credStore.Decrypt(string(encryptedValue))
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}
