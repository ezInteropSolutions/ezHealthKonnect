package services

// ===============================================================
// CREDENTIAL STORE — Application-Level AES-256-GCM Encryption
// ===============================================================
// Encrypts sensitive fields in connectivity configs (S3, SFTP,
// database passwords, API keys, etc.) before they are written
// to PostgreSQL, and decrypts them on read.
//
// Storage format (JSONB-compatible JSON wrapper):
//   Plaintext row:   {"region":"us-east-1","access_key_id":"AKIA..."}
//   Encrypted row:   {"_enc":"ENC:v1:base64GCMciphertext..."}
//
// The "_enc" wrapper preserves JSONB validity while allowing the
// column to hold either plaintext JSON (backward compat / dev mode)
// or an encrypted blob (production).
//
// Key management:
//   Set APP_CREDENTIAL_KEY to a base64-encoded 32-byte random key.
//   Generate:  openssl rand -base64 32
//   If unset:  passthrough mode (plaintext, dev-only — warning logged).
//
// AES-256-GCM properties:
//   - 256-bit key (32 bytes)
//   - 96-bit nonce (12 bytes, randomly generated per encryption)
//   - 128-bit authentication tag (appended by GCM automatically)
//   - Authenticated: any tampering with ciphertext is detected
// ===============================================================

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
)

// encryptedMarker is the version prefix embedded in every ciphertext.
// Bumping to v2 would allow rolling key rotation in future.
const encryptedMarker = "ENC:v1:"

// sensitiveKeySubstrings are substrings checked (case-insensitively) against
// config key names to determine if a value is sensitive. Using substrings catches
// camelCase variants (e.g. "dbPassword" contains "password") without needing an
// exhaustive list of every possible key name.
var sensitiveKeySubstrings = []string{
	"password", "passwd",
	"secret",
	"token",
	"passphrase",
	"apikey", "api_key",
	"privatekey", "private_key",
	"connectionstring", "connection_string",
	"accesskey", "access_key",
}

// isSensitiveKey returns true if the key name indicates a sensitive credential.
// Uses a case-insensitive substring match so "dbPassword", "db_password",
// "secretAccessKey", "connectionString", etc. are all detected automatically.
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveKeySubstrings {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// sensitiveFieldNames is kept for backward compatibility; new code uses isSensitiveKey.
var sensitiveFieldNames = []string{
	"password", "passwd",
	"secret", "secret_key", "secret_access_key", "aws_secret_access_key", "secretAccessKey",
	"token", "access_token", "bearer_token",
	"api_key", "apiKey", "api_secret",
	"access_key_id", "aws_access_key_id", "accessKeyId",
	"private_key", "private_key_id", "privateKey",
	"client_secret", "clientSecret",
	"passphrase",
	"service_account_key", "serviceAccountKey",
	"connection_string", "connectionString",
}

// CredentialStore encrypts and decrypts connectivity configuration blobs.
// A nil CredentialStore is valid — all operations become no-ops (passthrough).
type CredentialStore struct {
	key []byte // 32 bytes for AES-256
}

// NewCredentialStore creates a CredentialStore from a base64-encoded 32-byte key.
// Returns an error if the key is missing or the wrong length.
func NewCredentialStore(keyBase64 string) (*CredentialStore, error) {
	if keyBase64 == "" {
		return nil, fmt.Errorf("APP_CREDENTIAL_KEY is not set")
	}
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, fmt.Errorf("APP_CREDENTIAL_KEY is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("APP_CREDENTIAL_KEY must decode to exactly 32 bytes (got %d) — generate with: openssl rand -base64 32", len(key))
	}
	return &CredentialStore{key: key}, nil
}

// NewNoOpCredentialStore returns a passthrough store for development / tests.
// It never encrypts or decrypts — all values pass through unchanged.
func NewNoOpCredentialStore() *CredentialStore {
	return nil // nil receiver is valid — all methods handle nil
}

// IsEncrypted returns true if the value looks like an encrypted ciphertext
// produced by this store. Safe to call on a nil store.
func (cs *CredentialStore) IsEncrypted(value string) bool {
	return strings.HasPrefix(value, encryptedMarker)
}

// ── Core encryption/decryption ────────────────────────────────

// Encrypt encrypts plaintext bytes using AES-256-GCM and returns
// a base64-encoded string prefixed with the version marker.
// Returns the plaintext string unchanged if the store is nil (no-op mode).
func (cs *CredentialStore) Encrypt(plaintext []byte) (string, error) {
	if cs == nil {
		return string(plaintext), nil
	}

	block, err := aes.NewCipher(cs.key)
	if err != nil {
		return "", fmt.Errorf("credential_store: aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("credential_store: cipher.NewGCM: %w", err)
	}

	// Random nonce — unique per encryption, prepended to ciphertext
	nonce := make([]byte, gcm.NonceSize()) // 12 bytes for standard GCM
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("credential_store: nonce generation: %w", err)
	}

	// gcm.Seal(dst, nonce, plaintext, additionalData)
	// Appends nonce-prefixed ciphertext+tag to dst.
	// Layout: nonce(12) || ciphertext(N) || tag(16)
	combined := gcm.Seal(nonce, nonce, plaintext, nil)
	return encryptedMarker + base64.StdEncoding.EncodeToString(combined), nil
}

// Decrypt decrypts a ciphertext string produced by Encrypt.
// If the value does not start with the version marker, it is returned
// as-is (plaintext passthrough for backward compatibility).
// Returns an error only on authentication failure or corrupt data.
func (cs *CredentialStore) Decrypt(value string) ([]byte, error) {
	if cs == nil || !strings.HasPrefix(value, encryptedMarker) {
		return []byte(value), nil // no-op or legacy plaintext
	}

	data, err := base64.StdEncoding.DecodeString(value[len(encryptedMarker):])
	if err != nil {
		return nil, fmt.Errorf("credential_store: base64 decode: %w", err)
	}

	block, err := aes.NewCipher(cs.key)
	if err != nil {
		return nil, fmt.Errorf("credential_store: aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("credential_store: cipher.NewGCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("credential_store: ciphertext too short (got %d bytes)", len(data))
	}

	nonce, cipherAndTag := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherAndTag, nil)
	if err != nil {
		return nil, fmt.Errorf("credential_store: decryption failed (wrong key or tampered data): %w", err)
	}
	return plaintext, nil
}

// ── JSONB-compatible config helpers ───────────────────────────

// EncryptConfig encrypts a JSON connectivity config (source_config / target_config)
// and wraps it in a JSONB-valid envelope: {"_enc":"ENC:v1:..."}
// If the store is nil, returns the original value unchanged.
func (cs *CredentialStore) EncryptConfig(config json.RawMessage) (json.RawMessage, error) {
	if cs == nil || len(config) == 0 {
		return config, nil
	}

	// Skip double-encryption — idempotent
	if cs.isEncryptedEnvelope(config) {
		return config, nil
	}

	ciphertext, err := cs.Encrypt(config)
	if err != nil {
		return nil, fmt.Errorf("EncryptConfig: %w", err)
	}

	wrapper := map[string]string{"_enc": ciphertext}
	return json.Marshal(wrapper)
}

// DecryptConfig decrypts a JSONB connectivity config.
// If the value is a plaintext JSON object (no "_enc" key), it is returned as-is.
// If it is an encrypted envelope {"_enc":"ENC:v1:..."}, the inner JSON is decrypted.
func (cs *CredentialStore) DecryptConfig(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}

	// Fast path: if it looks like a normal JSON object and not an encrypted envelope
	if !cs.isEncryptedEnvelope(raw) {
		return raw, nil
	}

	// Parse the "_enc" wrapper
	var wrapper map[string]string
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return raw, nil // not a valid wrapper → passthrough
	}
	ciphertext, ok := wrapper["_enc"]
	if !ok {
		return raw, nil
	}

	plaintext, err := cs.Decrypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("DecryptConfig: %w", err)
	}
	return json.RawMessage(plaintext), nil
}

// DecryptConfigBytes is the same as DecryptConfig but accepts and returns []byte.
// Used by executors that receive raw JSONB bytes from a direct SQL scan.
func (cs *CredentialStore) DecryptConfigBytes(raw []byte) ([]byte, error) {
	result, err := cs.DecryptConfig(json.RawMessage(raw))
	return []byte(result), err
}

// isEncryptedEnvelope checks whether a JSON value is the {"_enc":"..."} wrapper.
func (cs *CredentialStore) isEncryptedEnvelope(raw json.RawMessage) bool {
	// Quick heuristic before full parse: must contain the literal "_enc"
	return strings.Contains(string(raw), `"_enc"`)
}

// ── UI masking ────────────────────────────────────────────────

// MaskSensitiveFields replaces the values of known sensitive keys with "••••••••"
// in a decrypted config map before it is sent to the UI.
// This ensures credentials can never be read back once saved.
// Non-sensitive fields (region, host, port, etc.) are preserved for display.
func (cs *CredentialStore) MaskSensitiveFields(config json.RawMessage) json.RawMessage {
	if len(config) == 0 {
		return config
	}

	var m map[string]interface{}
	if err := json.Unmarshal(config, &m); err != nil {
		return config // not a JSON object — return unchanged
	}

	changed := false
	for k, v := range m {
		if v == nil || v == "" {
			continue
		}
		if isSensitiveKey(k) {
			m[k] = "••••••••"
			changed = true
		}
	}

	if !changed {
		return config
	}

	masked, err := json.Marshal(m)
	if err != nil {
		return config
	}
	return masked
}

// ── Step config field-level encryption ───────────────────────
//
// Unlike EncryptConfig (which encrypts an entire connectivity config blob),
// these methods operate on the individual key/value pairs inside a pipeline
// step's config map (transformation_steps.config). Only sensitive values are
// encrypted; non-sensitive values (host, port, database name, query, etc.)
// remain readable. This lets operators inspect configs without decrypting, while
// protecting credentials at rest.
//
// Wire-format: sensitive field values are stored as "ENC:v1:<base64>".
// The same ENC:v1: prefix is used as for blob encryption so the existing
// Encrypt/Decrypt primitives handle both cases.
//
// Both methods walk the FULL config tree, not just the top level. This matters
// for connector.outbound/connector.inbound steps, whose step.Config shape is
// {connectorType, config: {host, password, ...}, contentField, ...} — the
// actual connector credentials live one level down, inside the nested "config"
// map, not at the top level alongside "connectorType". A step-type-specific
// special case wasn't used deliberately: recursion covers any current or
// future step type whose config happens to nest a credential, without needing
// an allowlist of "step types with nested config" that would go stale.

// EncryptConfigFields encrypts the values of sensitive keys anywhere in a step
// config map, including inside nested maps and arrays of maps.
// Returns a new map — the input is never mutated.
// Values already prefixed with ENC:v1: are skipped (idempotent), as are empty or
// non-string values.
// If the store is nil, returns the input map unchanged (passthrough mode).
func (cs *CredentialStore) EncryptConfigFields(m map[string]interface{}) (map[string]interface{}, error) {
	if cs == nil || len(m) == 0 {
		return m, nil
	}
	return cs.encryptMapRecursive(m)
}

func (cs *CredentialStore) encryptMapRecursive(m map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		encV, err := cs.encryptValueRecursive(k, v)
		if err != nil {
			return nil, fmt.Errorf("EncryptConfigFields: key %q: %w", k, err)
		}
		result[k] = encV
	}
	return result, nil
}

// encryptValueRecursive dispatches on the runtime type of v: nested JSON
// objects/arrays (always map[string]interface{}/[]interface{} after a real
// encoding/json.Unmarshal into an interface{}) are walked recursively; a
// string is encrypted only if its OWN key name looks sensitive; anything else
// (numbers, bools, nil, or a non-JSON-shaped Go type from a hand-built test
// fixture) passes through unchanged.
func (cs *CredentialStore) encryptValueRecursive(key string, v interface{}) (interface{}, error) {
	switch val := v.(type) {
	case map[string]interface{}:
		return cs.encryptMapRecursive(val)
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, item := range val {
			encItem, err := cs.encryptValueRecursive(key, item)
			if err != nil {
				return nil, err
			}
			out[i] = encItem
		}
		return out, nil
	case string:
		if !isSensitiveKey(key) || val == "" || strings.HasPrefix(val, encryptedMarker) {
			return val, nil
		}
		return cs.Encrypt([]byte(val))
	default:
		return v, nil
	}
}

// DecryptConfigFields decrypts any ENC:v1:-prefixed string values anywhere in a
// step config map, including inside nested maps and arrays of maps.
// Returns a new map — the input is never mutated.
// Decryption applies to ALL string values starting with ENC:v1:, regardless of key
// name, so anything encrypted by EncryptConfigFields (or by the JS equivalent in
// pipelineController.js) is transparently unwrapped before reaching the executor.
// Non-ENC:v1: values are copied unchanged (backward compat for existing plaintext data).
// If the store is nil, returns the input map unchanged (passthrough mode).
func (cs *CredentialStore) DecryptConfigFields(m map[string]interface{}) (map[string]interface{}, error) {
	if cs == nil || len(m) == 0 {
		return m, nil
	}
	return cs.decryptMapRecursive(m)
}

func (cs *CredentialStore) decryptMapRecursive(m map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		decV, err := cs.decryptValueRecursive(v)
		if err != nil {
			return nil, fmt.Errorf("DecryptConfigFields: key %q: %w", k, err)
		}
		result[k] = decV
	}
	return result, nil
}

func (cs *CredentialStore) decryptValueRecursive(v interface{}) (interface{}, error) {
	switch val := v.(type) {
	case map[string]interface{}:
		return cs.decryptMapRecursive(val)
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, item := range val {
			decItem, err := cs.decryptValueRecursive(item)
			if err != nil {
				return nil, err
			}
			out[i] = decItem
		}
		return out, nil
	case string:
		if !strings.HasPrefix(val, encryptedMarker) {
			return val, nil
		}
		plain, err := cs.Decrypt(val)
		if err != nil {
			return nil, err
		}
		return string(plain), nil
	default:
		return v, nil
	}
}

// ── Logging helper ────────────────────────────────────────────

// LogMode returns a human-readable description of the current mode for startup logging.
func (cs *CredentialStore) LogMode() string {
	if cs == nil {
		return "disabled (plaintext — set APP_CREDENTIAL_KEY for production)"
	}
	return "enabled (AES-256-GCM)"
}

// WarnIfNoOp logs a prominent warning when running without encryption.
// Call once at startup.
func WarnIfNoOpCredentialStore(cs *CredentialStore) {
	if cs == nil {
		log.Printf("⚠️  [CredentialStore] Running in PLAINTEXT mode — connectivity credentials are NOT encrypted at rest.")
		log.Printf("⚠️  [CredentialStore] Set APP_CREDENTIAL_KEY (32-byte base64) before production deployment.")
		log.Printf("⚠️  [CredentialStore] Generate:  openssl rand -base64 32")
	} else {
		log.Printf("🔐 [CredentialStore] AES-256-GCM encryption active — connectivity credentials encrypted at rest.")
	}
}
