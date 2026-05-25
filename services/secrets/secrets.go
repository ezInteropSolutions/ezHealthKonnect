// services/secrets/secrets.go
//
// SecretStore is a future-proof interface for reading application secrets.
// The default EnvSecretStore reads directly from environment variables.
// Production deployments can swap to a Vault or AWS SSM implementation by
// satisfying the interface — no callers need to change.
//
// Usage:
//
//	var store secrets.SecretStore = secrets.NewEnvStore()
//	key, err := store.Get("APP_CREDENTIAL_KEY")
package secrets

import (
	"fmt"
	"os"
)

// SecretStore retrieves named secrets.
type SecretStore interface {
	// Get returns the secret value for the given key.
	// Returns an error if the secret is not found or cannot be retrieved.
	Get(key string) (string, error)

	// MustGet returns the value or panics — use only during startup validation
	// where a missing secret should halt the program.
	MustGet(key string) string
}

// EnvSecretStore reads secrets from environment variables.
// This is the default implementation; production can swap to Vault/SSM.
type EnvSecretStore struct {
	// prefix is prepended to every key lookup (e.g. "APP_" → looks up "APP_DB_PASSWORD")
	prefix string
}

// NewEnvStore creates an EnvSecretStore with no prefix.
func NewEnvStore() *EnvSecretStore {
	return &EnvSecretStore{}
}

// NewEnvStoreWithPrefix creates an EnvSecretStore that prepends prefix to all key lookups.
func NewEnvStoreWithPrefix(prefix string) *EnvSecretStore {
	return &EnvSecretStore{prefix: prefix}
}

// Get reads the environment variable named prefix+key.
func (s *EnvSecretStore) Get(key string) (string, error) {
	fullKey := s.prefix + key
	val := os.Getenv(fullKey)
	if val == "" {
		return "", fmt.Errorf("secret %q not set", fullKey)
	}
	return val, nil
}

// MustGet returns the value or panics if missing.
func (s *EnvSecretStore) MustGet(key string) string {
	val, err := s.Get(key)
	if err != nil {
		panic(err.Error())
	}
	return val
}

// GetWithDefault returns the value or defaultVal if the secret is not set.
func GetWithDefault(store SecretStore, key, defaultVal string) string {
	val, err := store.Get(key)
	if err != nil {
		return defaultVal
	}
	return val
}
