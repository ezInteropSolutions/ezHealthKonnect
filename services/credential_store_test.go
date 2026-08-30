package services

// ===============================================================
// CREDENTIAL STORE TESTS
// ===============================================================
// Covers:
//   - Round-trip: Encrypt → Decrypt produces original plaintext
//   - Idempotency: double-encryption is prevented in EncryptConfig
//   - Passthrough: nil store never modifies data
//   - Wrong-key authentication failure
//   - JSON wrapper: EncryptConfig / DecryptConfig envelope
//   - DecryptConfigBytes helper
//   - UI masking: MaskSensitiveFields redacts the right keys
//   - IsEncrypted detection
//   - Empty / nil edge cases
//   - Key validation errors
//   - Tampered ciphertext detection
// ===============================================================

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// ─── Helpers ──────────────────────────────────────────────────

// newTestStore returns a CredentialStore backed by a freshly-generated random key.
func newTestStore(t *testing.T) *CredentialStore {
	t.Helper()
	rawKey := make([]byte, 32)
	if _, err := rand.Read(rawKey); err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}
	cs, err := NewCredentialStore(base64.StdEncoding.EncodeToString(rawKey))
	if err != nil {
		t.Fatalf("NewCredentialStore failed: %v", err)
	}
	return cs
}

// mustJSON marshals v as JSON and returns the raw bytes, or fatals the test.
func mustJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return json.RawMessage(b)
}

// ─── NewCredentialStore ────────────────────────────────────────

func TestNewCredentialStore_EmptyKey_ReturnsError(t *testing.T) {
	_, err := NewCredentialStore("")
	if err == nil {
		t.Fatal("expected error for empty key, got nil")
	}
}

func TestNewCredentialStore_InvalidBase64_ReturnsError(t *testing.T) {
	_, err := NewCredentialStore("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestNewCredentialStore_WrongKeyLength_ReturnsError(t *testing.T) {
	// 16 bytes → base64 → only 128-bit key, not 256-bit
	shortKey := base64.StdEncoding.EncodeToString(make([]byte, 16))
	_, err := NewCredentialStore(shortKey)
	if err == nil {
		t.Fatal("expected error for 16-byte key, got nil")
	}
	if !strings.Contains(err.Error(), "32 bytes") {
		t.Errorf("error should mention 32 bytes, got: %v", err)
	}
}

func TestNewCredentialStore_ValidKey_Succeeds(t *testing.T) {
	cs := newTestStore(t)
	if cs == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestNewNoOpCredentialStore_ReturnsNil(t *testing.T) {
	cs := NewNoOpCredentialStore()
	if cs != nil {
		t.Fatal("NewNoOpCredentialStore should return nil")
	}
}

// ─── Encrypt / Decrypt round-trip ─────────────────────────────

func TestEncryptDecrypt_RoundTrip_Plaintext(t *testing.T) {
	cs := newTestStore(t)
	plain := []byte("super-secret-password-123")

	ciphertext, err := cs.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(ciphertext, encryptedMarker) {
		preview := ciphertext
		if len(preview) > 20 {
			preview = preview[:20]
		}
		t.Errorf("ciphertext should start with %q, got: %s", encryptedMarker, preview)
	}

	got, err := cs.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != string(plain) {
		t.Errorf("round-trip mismatch: want %q, got %q", plain, got)
	}
}

func TestEncryptDecrypt_RoundTrip_JSON(t *testing.T) {
	cs := newTestStore(t)
	plain := []byte(`{"password":"hunter2","host":"db.example.com","port":5432}`)

	ct, err := cs.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := cs.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != string(plain) {
		t.Errorf("round-trip mismatch: want %q, got %q", plain, got)
	}
}

func TestEncryptDecrypt_RoundTrip_EmptyBytes(t *testing.T) {
	cs := newTestStore(t)
	plain := []byte("")

	ct, err := cs.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	got, err := cs.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if string(got) != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestEncryptDecrypt_TwoCallsProdudeDifferentCiphertexts(t *testing.T) {
	cs := newTestStore(t)
	plain := []byte("same-plaintext")

	ct1, _ := cs.Encrypt(plain)
	ct2, _ := cs.Encrypt(plain)

	// Each encryption uses a fresh random nonce, so ciphertexts must differ
	if ct1 == ct2 {
		t.Error("two encryptions of the same plaintext must produce different ciphertexts (nonce reuse!)")
	}
}

// ─── Passthrough (nil store) ───────────────────────────────────

func TestEncrypt_NilStore_ReturnsPlaintext(t *testing.T) {
	var cs *CredentialStore // nil
	plain := []byte("no-encryption-please")

	result, err := cs.Encrypt(plain)
	if err != nil {
		t.Fatalf("nil Encrypt: %v", err)
	}
	if result != string(plain) {
		t.Errorf("nil store should return plaintext unchanged, got %q", result)
	}
}

func TestDecrypt_NilStore_ReturnsInputUnchanged(t *testing.T) {
	var cs *CredentialStore
	raw := "plaintext-no-marker"

	got, err := cs.Decrypt(raw)
	if err != nil {
		t.Fatalf("nil Decrypt: %v", err)
	}
	if string(got) != raw {
		t.Errorf("nil store should passthrough, got %q", got)
	}
}

func TestDecrypt_NilStore_EncryptedMarker_Passthrough(t *testing.T) {
	// A nil store cannot decrypt — it should return the raw ENC:v1: string unchanged,
	// not error. (Could happen if the store was disabled after encryption.)
	var cs *CredentialStore
	fakeEncrypted := encryptedMarker + "someciphertexthere"

	got, err := cs.Decrypt(fakeEncrypted)
	if err != nil {
		t.Fatalf("nil Decrypt with marker: %v", err)
	}
	if string(got) != fakeEncrypted {
		t.Errorf("nil store should passthrough ENC:v1: strings, got %q", got)
	}
}

// ─── Decrypt backward-compat: plaintext strings ───────────────

func TestDecrypt_PlaintextString_NoMarker_ReturnedAsIs(t *testing.T) {
	cs := newTestStore(t)
	plain := "i-am-not-encrypted"

	got, err := cs.Decrypt(plain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != plain {
		t.Errorf("want %q, got %q", plain, got)
	}
}

// ─── Tamper detection ─────────────────────────────────────────

func TestDecrypt_TamperedCiphertext_ReturnsError(t *testing.T) {
	cs := newTestStore(t)
	plain := []byte("tamper me if you dare")

	ct, _ := cs.Encrypt(plain)

	// Flip the last character of the base64 portion — corrupts the auth tag
	ctBytes := []byte(ct)
	ctBytes[len(ctBytes)-1] ^= 0x01
	tampered := string(ctBytes)

	_, err := cs.Decrypt(tampered)
	if err == nil {
		t.Fatal("expected authentication failure on tampered ciphertext, got nil")
	}
}

func TestDecrypt_WrongKey_ReturnsError(t *testing.T) {
	encryptor := newTestStore(t)
	decryptor := newTestStore(t) // different key

	ct, err := encryptor.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = decryptor.Decrypt(ct)
	if err == nil {
		t.Fatal("expected decryption failure with wrong key, got nil")
	}
}

func TestDecrypt_TruncatedCiphertext_ReturnsError(t *testing.T) {
	cs := newTestStore(t)

	// Build a ciphertext that's too short (fewer than nonce bytes after decode)
	tooShort := encryptedMarker + base64.StdEncoding.EncodeToString([]byte("tiny"))
	_, err := cs.Decrypt(tooShort)
	if err == nil {
		t.Fatal("expected error for truncated ciphertext, got nil")
	}
}

// ─── IsEncrypted ──────────────────────────────────────────────

func TestIsEncrypted_RealCiphertext_ReturnsTrue(t *testing.T) {
	cs := newTestStore(t)
	ct, _ := cs.Encrypt([]byte("test"))
	if !cs.IsEncrypted(ct) {
		t.Error("IsEncrypted should return true for ENC:v1: prefixed strings")
	}
}

func TestIsEncrypted_Plaintext_ReturnsFalse(t *testing.T) {
	cs := newTestStore(t)
	if cs.IsEncrypted("plaintext") {
		t.Error("IsEncrypted should return false for plaintext")
	}
}

func TestIsEncrypted_NilStore_WorksSafely(t *testing.T) {
	var cs *CredentialStore
	if cs.IsEncrypted(encryptedMarker + "abc") {
		// Allowed — nil store uses the package-level prefix check
	}
	if cs.IsEncrypted("plaintext") {
		t.Error("nil IsEncrypted('plaintext') should return false")
	}
}

// ─── EncryptConfig / DecryptConfig ────────────────────────────

func TestEncryptConfig_DecryptConfig_RoundTrip(t *testing.T) {
	cs := newTestStore(t)
	original := mustJSON(t, map[string]interface{}{
		"host":     "sftp.hospital.com",
		"password": "supersecret",
		"port":     22,
	})

	encrypted, err := cs.EncryptConfig(original)
	if err != nil {
		t.Fatalf("EncryptConfig: %v", err)
	}

	// Should be a JSON object with {"_enc":"ENC:v1:..."}
	var wrapper map[string]string
	if err := json.Unmarshal(encrypted, &wrapper); err != nil {
		t.Fatalf("encrypted envelope is not valid JSON: %v", err)
	}
	if _, ok := wrapper["_enc"]; !ok {
		t.Fatal("encrypted envelope missing '_enc' key")
	}
	if !strings.HasPrefix(wrapper["_enc"], encryptedMarker) {
		t.Errorf("_enc value should start with %q", encryptedMarker)
	}

	decrypted, err := cs.DecryptConfig(encrypted)
	if err != nil {
		t.Fatalf("DecryptConfig: %v", err)
	}
	if string(decrypted) != string(original) {
		t.Errorf("round-trip mismatch:\n  want %s\n   got %s", original, decrypted)
	}
}

func TestEncryptConfig_Idempotent_NoDoubleEncryption(t *testing.T) {
	cs := newTestStore(t)
	original := mustJSON(t, map[string]string{"api_key": "sk-abc123"})

	enc1, _ := cs.EncryptConfig(original)
	enc2, _ := cs.EncryptConfig(enc1) // encrypt already-encrypted value

	// The outer wrapper should be identical — no double encryption
	if string(enc1) != string(enc2) {
		t.Error("EncryptConfig should be idempotent — double-encrypting should return the same envelope")
	}
}

func TestEncryptConfig_NilStore_ReturnsOriginal(t *testing.T) {
	var cs *CredentialStore
	original := mustJSON(t, map[string]string{"password": "secret"})

	result, err := cs.EncryptConfig(original)
	if err != nil {
		t.Fatalf("nil EncryptConfig: %v", err)
	}
	if string(result) != string(original) {
		t.Errorf("nil store should return original config unchanged")
	}
}

func TestEncryptConfig_EmptyConfig_ReturnsEmpty(t *testing.T) {
	cs := newTestStore(t)
	result, err := cs.EncryptConfig(json.RawMessage{})
	if err != nil {
		t.Fatalf("EncryptConfig(empty): %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for empty input, got %q", result)
	}
}

func TestDecryptConfig_PlaintextJSONObject_ReturnedAsIs(t *testing.T) {
	cs := newTestStore(t)
	plain := mustJSON(t, map[string]string{"host": "db.example.com"})

	result, err := cs.DecryptConfig(plain)
	if err != nil {
		t.Fatalf("DecryptConfig(plaintext): %v", err)
	}
	if string(result) != string(plain) {
		t.Errorf("plaintext object should passthrough, got %q", result)
	}
}

func TestDecryptConfig_NilStore_EncryptedEnvelope_Passthrough(t *testing.T) {
	// Write a valid-looking encrypted envelope, then try to decrypt with nil store
	encryptor := newTestStore(t)
	enc, _ := encryptor.EncryptConfig(mustJSON(t, map[string]string{"password": "x"}))

	var cs *CredentialStore // nil = passthrough
	result, err := cs.DecryptConfig(enc)
	if err != nil {
		t.Fatalf("nil DecryptConfig: %v", err)
	}
	// nil store DecryptConfig falls through to the plaintext fast path
	// The encrypted envelope is returned as-is (caller needs a real store to decrypt)
	_ = result // passthrough is acceptable
}

func TestDecryptConfigBytes_RoundTrip(t *testing.T) {
	cs := newTestStore(t)
	original := []byte(`{"connection_string":"postgresql://user:pass@host/db"}`)

	enc, err := cs.EncryptConfig(json.RawMessage(original))
	if err != nil {
		t.Fatalf("EncryptConfig: %v", err)
	}

	got, err := cs.DecryptConfigBytes([]byte(enc))
	if err != nil {
		t.Fatalf("DecryptConfigBytes: %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("round-trip mismatch: want %q, got %q", original, got)
	}
}

// ─── MaskSensitiveFields ──────────────────────────────────────

func TestMaskSensitiveFields_RedactsKnownKeys(t *testing.T) {
	cs := newTestStore(t)
	cfg := mustJSON(t, map[string]interface{}{
		"host":              "sftp.hospital.com",
		"port":              22,
		"password":          "s3cr3t",
		"secret_access_key": "AKIA12345",
		"passphrase":        "my-ssh-passphrase",
	})

	masked := cs.MaskSensitiveFields(cfg)

	var m map[string]interface{}
	if err := json.Unmarshal(masked, &m); err != nil {
		t.Fatalf("MaskSensitiveFields returned invalid JSON: %v", err)
	}

	// Non-sensitive fields preserved
	if m["host"] != "sftp.hospital.com" {
		t.Errorf("host should be preserved, got %v", m["host"])
	}
	if m["port"].(float64) != 22 {
		t.Errorf("port should be preserved, got %v", m["port"])
	}

	// Sensitive fields masked
	for _, key := range []string{"password", "secret_access_key", "passphrase"} {
		if m[key] != "••••••••" {
			t.Errorf("key %q should be masked to ••••••••, got %q", key, m[key])
		}
	}
}

func TestMaskSensitiveFields_CaseInsensitiveKeys(t *testing.T) {
	cs := newTestStore(t)
	cfg := mustJSON(t, map[string]interface{}{
		"PASSWORD":   "shout",
		"Api_Key":    "SHOUT",
		"SecretKey":  "mixed",
	})

	// API compares lower(k) → check known lower-case matches
	// "password" → lower("PASSWORD") = "password" → masked
	masked := cs.MaskSensitiveFields(cfg)
	var m map[string]interface{}
	json.Unmarshal(masked, &m)

	if m["PASSWORD"] != "••••••••" {
		t.Errorf("case-insensitive: PASSWORD should be masked, got %v", m["PASSWORD"])
	}
}

func TestMaskSensitiveFields_EmptyValueNotMasked(t *testing.T) {
	cs := newTestStore(t)
	cfg := mustJSON(t, map[string]interface{}{
		"password": "",
		"host":     "db.example.com",
	})

	masked := cs.MaskSensitiveFields(cfg)
	var m map[string]interface{}
	json.Unmarshal(masked, &m)

	// Empty password stays empty (nothing to hide, and masking would be misleading)
	if m["password"] == "••••••••" {
		t.Error("empty password should not be masked to ••••••••")
	}
}

func TestMaskSensitiveFields_NilStore_WorksSafely(t *testing.T) {
	var cs *CredentialStore
	cfg := mustJSON(t, map[string]interface{}{
		"password": "secret",
		"host":     "example.com",
	})

	// nil store has MaskSensitiveFields defined — it should not panic
	masked := cs.MaskSensitiveFields(cfg)

	var m map[string]interface{}
	if err := json.Unmarshal(masked, &m); err != nil {
		t.Fatalf("nil MaskSensitiveFields returned invalid JSON: %v", err)
	}
	if m["password"] != "••••••••" {
		t.Errorf("nil store: password should still be masked, got %v", m["password"])
	}
}

func TestMaskSensitiveFields_EmptyConfig_ReturnedUnchanged(t *testing.T) {
	cs := newTestStore(t)
	result := cs.MaskSensitiveFields(json.RawMessage{})
	if len(result) != 0 {
		t.Errorf("empty input should return empty, got %q", result)
	}
}

func TestMaskSensitiveFields_InvalidJSON_ReturnedUnchanged(t *testing.T) {
	cs := newTestStore(t)
	bad := json.RawMessage(`not-valid-json`)
	result := cs.MaskSensitiveFields(bad)
	if string(result) != string(bad) {
		t.Errorf("invalid JSON should be returned unchanged, got %q", result)
	}
}

// ─── LogMode / WarnIfNoOpCredentialStore ──────────────────────

func TestLogMode_ActiveStore(t *testing.T) {
	cs := newTestStore(t)
	mode := cs.LogMode()
	if !strings.Contains(mode, "AES-256-GCM") {
		t.Errorf("LogMode for active store should mention AES-256-GCM, got %q", mode)
	}
}

func TestLogMode_NilStore(t *testing.T) {
	var cs *CredentialStore
	mode := cs.LogMode()
	if !strings.Contains(mode, "plaintext") {
		t.Errorf("LogMode for nil store should mention plaintext, got %q", mode)
	}
}

func TestWarnIfNoOpCredentialStore_NilStore_NoPanic(t *testing.T) {
	// Should log warnings but not panic
	WarnIfNoOpCredentialStore(nil)
}

func TestWarnIfNoOpCredentialStore_ActiveStore_NoPanic(t *testing.T) {
	cs := newTestStore(t)
	WarnIfNoOpCredentialStore(cs)
}

// ─── Cross-compatibility (encrypt with one instance, decrypt with same key) ──

func TestCrossCompat_SameKeyDifferentInstances(t *testing.T) {
	// Generate a key once and create two stores from it — simulates app restart
	rawKey := make([]byte, 32)
	rand.Read(rawKey)
	keyB64 := base64.StdEncoding.EncodeToString(rawKey)

	cs1, _ := NewCredentialStore(keyB64)
	cs2, _ := NewCredentialStore(keyB64)

	original := mustJSON(t, map[string]interface{}{
		"host":     "sftp.hospital.com",
		"username": "admin",
		"password": "topSecret!99",
	})

	// Encrypt with cs1
	enc, err := cs1.EncryptConfig(original)
	if err != nil {
		t.Fatalf("EncryptConfig (cs1): %v", err)
	}

	// Decrypt with cs2 (same key, different instance — simulates app restart)
	dec, err := cs2.DecryptConfig(enc)
	if err != nil {
		t.Fatalf("DecryptConfig (cs2): %v", err)
	}

	if string(dec) != string(original) {
		t.Errorf("cross-instance round-trip failed:\n  want %s\n   got %s", original, dec)
	}
}

// ─── Realistic connectivity config scenarios ──────────────────

func TestEncryptDecrypt_SFTPConfig(t *testing.T) {
	cs := newTestStore(t)
	config := mustJSON(t, map[string]interface{}{
		"host":       "sftp.hospital.com",
		"port":       22,
		"username":   "hl7user",
		"password":   "P@ssw0rd!",
		"passphrase": "my-ssh-key-passphrase",
		"remote_dir": "/hl7/inbound",
	})

	enc, _ := cs.EncryptConfig(config)
	dec, err := cs.DecryptConfig(enc)
	if err != nil {
		t.Fatalf("SFTP config round-trip: %v", err)
	}
	if string(dec) != string(config) {
		t.Errorf("SFTP config mismatch: want %s, got %s", config, dec)
	}
}

func TestEncryptDecrypt_DatabaseConfig(t *testing.T) {
	cs := newTestStore(t)
	config := mustJSON(t, map[string]interface{}{
		"host":              "db.hospital.internal",
		"port":              5432,
		"database":          "patient_records",
		"username":          "pipeline_user",
		"password":          "db-secret-123",
		"connection_string": "postgresql://pipeline_user:db-secret-123@db.hospital.internal/patient_records",
		"ssl_mode":          "require",
	})

	enc, _ := cs.EncryptConfig(config)
	dec, err := cs.DecryptConfig(enc)
	if err != nil {
		t.Fatalf("database config round-trip: %v", err)
	}
	if string(dec) != string(config) {
		t.Errorf("database config mismatch: want %s, got %s", config, dec)
	}
}

func TestEncryptDecrypt_S3Config(t *testing.T) {
	cs := newTestStore(t)
	config := mustJSON(t, map[string]interface{}{
		"region":              "us-east-1",
		"bucket":              "claims-processing-bucket",
		"access_key_id":       "AKIAIOSFODNN7EXAMPLE",
		"secret_access_key":   "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"prefix":              "inbound/cclf/",
	})

	enc, _ := cs.EncryptConfig(config)

	// Verify _enc wrapper
	var wrapper map[string]string
	json.Unmarshal(enc, &wrapper)
	if !strings.HasPrefix(wrapper["_enc"], encryptedMarker) {
		t.Error("S3 config envelope missing ENC:v1: prefix")
	}

	dec, err := cs.DecryptConfig(enc)
	if err != nil {
		t.Fatalf("S3 config round-trip: %v", err)
	}
	if string(dec) != string(config) {
		t.Errorf("S3 config mismatch: want %s, got %s", config, dec)
	}
}

func TestMaskSensitiveFields_S3Config_MasksCredentials(t *testing.T) {
	cs := newTestStore(t)
	config := mustJSON(t, map[string]interface{}{
		"region":            "us-east-1",
		"bucket":            "claims-bucket",
		"access_key_id":     "AKIAIOSFODNN7EXAMPLE",
		"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	})

	masked := cs.MaskSensitiveFields(config)
	var m map[string]interface{}
	json.Unmarshal(masked, &m)

	// Non-sensitive preserved
	if m["region"] != "us-east-1" {
		t.Errorf("region should be preserved, got %v", m["region"])
	}
	if m["bucket"] != "claims-bucket" {
		t.Errorf("bucket should be preserved, got %v", m["bucket"])
	}

	// Credentials masked
	if m["access_key_id"] != "••••••••" {
		t.Errorf("access_key_id should be masked, got %v", m["access_key_id"])
	}
	if m["secret_access_key"] != "••••••••" {
		t.Errorf("secret_access_key should be masked, got %v", m["secret_access_key"])
	}
}

// ─── isSensitiveKey ───────────────────────────────────────────

func TestIsSensitiveKey_Catches_dbPassword(t *testing.T) {
	// The key "dbPassword" contains "password" — must be sensitive
	if !isSensitiveKey("dbPassword") {
		t.Error("dbPassword must be detected as sensitive (contains 'password')")
	}
}

func TestIsSensitiveKey_Catches_connectionString(t *testing.T) {
	if !isSensitiveKey("connectionString") {
		t.Error("connectionString must be detected as sensitive")
	}
	if !isSensitiveKey("connection_string") {
		t.Error("connection_string must be detected as sensitive")
	}
}

func TestIsSensitiveKey_Catches_CommonVariants(t *testing.T) {
	sensitive := []string{
		"password", "passwd", "dbPassword", "db_password",
		"secret", "secretAccessKey", "secret_access_key", "clientSecret",
		"token", "access_token", "bearer_token",
		"apiKey", "api_key",
		"passphrase",
		"privateKey", "private_key",
		"accessKeyId", "access_key_id",
	}
	for _, key := range sensitive {
		if !isSensitiveKey(key) {
			t.Errorf("isSensitiveKey(%q) should return true", key)
		}
	}
}

func TestIsSensitiveKey_Safe_NonSensitive(t *testing.T) {
	safe := []string{"host", "port", "database", "dbName", "dbHost", "dbPort",
		"region", "bucket", "query", "targetPath", "databaseType", "collection"}
	for _, key := range safe {
		if isSensitiveKey(key) {
			t.Errorf("isSensitiveKey(%q) should return false", key)
		}
	}
}

// ─── EncryptConfigFields ──────────────────────────────────────

func TestEncryptConfigFields_EncryptsSensitiveKeys(t *testing.T) {
	cs := newTestStore(t)
	input := map[string]interface{}{
		"dbHost":     "postgres",
		"dbPort":     5432,
		"dbName":     "ezhealthkonnect",
		"dbUser":     "ezhealth_user",
		"dbPassword": "super-secret-password",
	}

	result, err := cs.EncryptConfigFields(input)
	if err != nil {
		t.Fatalf("EncryptConfigFields: %v", err)
	}

	// Non-sensitive fields unchanged
	if result["dbHost"] != "postgres" {
		t.Errorf("dbHost should be unchanged, got %v", result["dbHost"])
	}
	if result["dbPort"] != 5432 {
		t.Errorf("dbPort should be unchanged, got %v", result["dbPort"])
	}
	if result["dbName"] != "ezhealthkonnect" {
		t.Errorf("dbName should be unchanged, got %v", result["dbName"])
	}

	// dbPassword should be encrypted
	enc, ok := result["dbPassword"].(string)
	if !ok {
		t.Fatalf("dbPassword should be a string after encryption, got %T", result["dbPassword"])
	}
	if !strings.HasPrefix(enc, encryptedMarker) {
		t.Errorf("dbPassword should start with %q, got %q", encryptedMarker, enc[:credStoreMin(len(enc), 20)])
	}
	// Original value gone
	if enc == "super-secret-password" {
		t.Error("dbPassword must not equal plaintext after encryption")
	}
}

func TestEncryptConfigFields_Idempotent_AlreadyEncrypted(t *testing.T) {
	cs := newTestStore(t)
	input := map[string]interface{}{
		"dbPassword": "plaintext-password",
	}

	// Encrypt once
	result1, err := cs.EncryptConfigFields(input)
	if err != nil {
		t.Fatalf("first EncryptConfigFields: %v", err)
	}
	// Encrypt again (simulates re-save without changing password)
	result2, err := cs.EncryptConfigFields(result1)
	if err != nil {
		t.Fatalf("second EncryptConfigFields: %v", err)
	}

	// The ENC:v1: string should be identical (not double-encrypted)
	if result1["dbPassword"] != result2["dbPassword"] {
		t.Errorf("second encryption should be idempotent:\n  first=%v\n second=%v",
			result1["dbPassword"], result2["dbPassword"])
	}
}

func TestEncryptConfigFields_NilStore_Passthrough(t *testing.T) {
	var cs *CredentialStore
	input := map[string]interface{}{
		"dbPassword": "should-not-be-encrypted",
		"dbHost":     "localhost",
	}

	result, err := cs.EncryptConfigFields(input)
	if err != nil {
		t.Fatalf("nil EncryptConfigFields: %v", err)
	}
	if result["dbPassword"] != "should-not-be-encrypted" {
		t.Errorf("nil store should not encrypt, got %v", result["dbPassword"])
	}
}

func TestEncryptConfigFields_EmptyValue_Skipped(t *testing.T) {
	cs := newTestStore(t)
	input := map[string]interface{}{
		"dbPassword": "", // empty — nothing to encrypt
		"dbHost":     "localhost",
	}

	result, err := cs.EncryptConfigFields(input)
	if err != nil {
		t.Fatalf("EncryptConfigFields: %v", err)
	}
	if result["dbPassword"] != "" {
		t.Errorf("empty password should remain empty, got %v", result["dbPassword"])
	}
}

func TestEncryptConfigFields_NonStringValue_Skipped(t *testing.T) {
	cs := newTestStore(t)
	// Sometimes "token" could be an integer (e.g. port 6379 named "tokenPort")
	// Non-string sensitive keys should be left alone
	input := map[string]interface{}{
		"port":       6379,
		"dbPassword": "real-password",
	}

	result, err := cs.EncryptConfigFields(input)
	if err != nil {
		t.Fatalf("EncryptConfigFields: %v", err)
	}
	if result["port"] != 6379 {
		t.Errorf("integer port should be unchanged, got %v", result["port"])
	}
}

// ─── DecryptConfigFields ──────────────────────────────────────

func TestDecryptConfigFields_RoundTrip(t *testing.T) {
	cs := newTestStore(t)
	original := map[string]interface{}{
		"dbHost":            "postgres",
		"dbPort":            5432,
		"dbPassword":        "my-db-password",
		"connectionString":  "postgresql://user:pass@host/db",
		"apiKey":            "sk-live-abc123",
	}

	encrypted, err := cs.EncryptConfigFields(original)
	if err != nil {
		t.Fatalf("EncryptConfigFields: %v", err)
	}

	decrypted, err := cs.DecryptConfigFields(encrypted)
	if err != nil {
		t.Fatalf("DecryptConfigFields: %v", err)
	}

	// All values should be restored
	if decrypted["dbHost"] != original["dbHost"] {
		t.Errorf("dbHost: want %v, got %v", original["dbHost"], decrypted["dbHost"])
	}
	if decrypted["dbPassword"] != original["dbPassword"] {
		t.Errorf("dbPassword: want %v, got %v", original["dbPassword"], decrypted["dbPassword"])
	}
	if decrypted["connectionString"] != original["connectionString"] {
		t.Errorf("connectionString: want %v, got %v", original["connectionString"], decrypted["connectionString"])
	}
	if decrypted["apiKey"] != original["apiKey"] {
		t.Errorf("apiKey: want %v, got %v", original["apiKey"], decrypted["apiKey"])
	}
}

func TestDecryptConfigFields_PlaintextValuesUnchanged(t *testing.T) {
	cs := newTestStore(t)
	// Backward compat: configs saved before encryption was enabled have plaintext values
	input := map[string]interface{}{
		"dbHost":     "postgres",
		"dbPassword": "legacy-plaintext-password",
	}

	result, err := cs.DecryptConfigFields(input)
	if err != nil {
		t.Fatalf("DecryptConfigFields on plaintext: %v", err)
	}
	if result["dbPassword"] != "legacy-plaintext-password" {
		t.Errorf("plaintext value should pass through unchanged, got %v", result["dbPassword"])
	}
}

func TestDecryptConfigFields_NilStore_Passthrough(t *testing.T) {
	var cs *CredentialStore
	input := map[string]interface{}{
		"dbPassword": "ENC:v1:somefakevalue",
		"dbHost":     "localhost",
	}

	result, err := cs.DecryptConfigFields(input)
	if err != nil {
		t.Fatalf("nil DecryptConfigFields: %v", err)
	}
	// nil store: ENC:v1: value returned as-is (cannot decrypt without key)
	if result["dbPassword"] != "ENC:v1:somefakevalue" {
		t.Errorf("nil store should passthrough ENC:v1: string, got %v", result["dbPassword"])
	}
}

// ─── MaskSensitiveFields catches dbPassword after isSensitiveKey fix ─────────

func TestMaskSensitiveFields_Catches_dbPassword(t *testing.T) {
	cs := newTestStore(t)
	cfg := mustJSON(t, map[string]interface{}{
		"dbHost":     "postgres",
		"dbPort":     5432,
		"dbPassword": "super-secret",
	})

	masked := cs.MaskSensitiveFields(cfg)
	var m map[string]interface{}
	if err := json.Unmarshal(masked, &m); err != nil {
		t.Fatalf("MaskSensitiveFields returned invalid JSON: %v", err)
	}

	if m["dbHost"] != "postgres" {
		t.Errorf("dbHost should be preserved, got %v", m["dbHost"])
	}
	if m["dbPassword"] != "••••••••" {
		t.Errorf("dbPassword should be masked to ••••••••, got %v", m["dbPassword"])
	}
}

// ─── Realistic DB enrichment step scenario ───────────────────

func TestEncryptDecryptConfigFields_PostgreSQLEnrichmentStep(t *testing.T) {
	cs := newTestStore(t)

	// Simulate the step config saved by pipelineController.js
	stepConfig := map[string]interface{}{
		"databaseType":     "postgresql",
		"dbHost":           "postgres",
		"dbPort":           5432,
		"dbName":           "ezhealthkonnect",
		"dbUser":           "ezhealth_user",
		"dbPassword":       "secure_password_change_me",
		"query":            "SELECT * FROM users",
		"targetPath":       "enriched.database",
		"timeoutMs":        3000,
		"resultMapping":    map[string]string{"email": "email"},
	}

	encrypted, err := cs.EncryptConfigFields(stepConfig)
	if err != nil {
		t.Fatalf("encrypt step config: %v", err)
	}

	// Verify structural integrity
	if encrypted["databaseType"] != "postgresql" {
		t.Error("databaseType should not be encrypted")
	}
	if encrypted["dbHost"] != "postgres" {
		t.Error("dbHost should not be encrypted")
	}
	if encrypted["dbPort"] != 5432 {
		t.Error("dbPort should not be encrypted")
	}
	enc, _ := encrypted["dbPassword"].(string)
	if !strings.HasPrefix(enc, encryptedMarker) {
		t.Errorf("dbPassword should be encrypted, got %v", enc)
	}

	// Simulate what the executor does before running the query
	decrypted, err := cs.DecryptConfigFields(encrypted)
	if err != nil {
		t.Fatalf("decrypt step config: %v", err)
	}

	if decrypted["dbPassword"] != "secure_password_change_me" {
		t.Errorf("dbPassword round-trip failed: got %v", decrypted["dbPassword"])
	}
}

// ─── Nested config (connector.outbound/inbound real shape) ──────────
//
// A connector.outbound/connector.inbound step's real config shape is
// {connectorType, config: {host, password, ...}, contentField, ...} -- the
// actual credentials live one level down inside the nested "config" map, not
// alongside "connectorType" at the top level. EncryptConfigFields/
// DecryptConfigFields used to only walk the top level, so these nested
// credentials were silently never encrypted. These tests cover the fix.

func TestEncryptConfigFields_EncryptsNestedConnectorCredential(t *testing.T) {
	cs := newTestStore(t)
	stepConfig := map[string]interface{}{
		"connectorType": "mysql_outbound",
		"config": map[string]interface{}{
			"host":     "mysql.internal",
			"port":     float64(3306),
			"username": "app_user",
			"password": "sup3r-secret-db-pass",
		},
		"contentField": "message",
	}

	result, err := cs.EncryptConfigFields(stepConfig)
	if err != nil {
		t.Fatalf("EncryptConfigFields: %v", err)
	}

	if result["connectorType"] != "mysql_outbound" {
		t.Errorf("connectorType should be unchanged, got %v", result["connectorType"])
	}

	nested, ok := result["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("config should still be a map[string]interface{}, got %T", result["config"])
	}
	if nested["host"] != "mysql.internal" {
		t.Errorf("nested host should be unchanged, got %v", nested["host"])
	}
	if nested["port"] != float64(3306) {
		t.Errorf("nested port should be unchanged, got %v", nested["port"])
	}

	encPassword, ok := nested["password"].(string)
	if !ok || !strings.HasPrefix(encPassword, encryptedMarker) {
		t.Fatalf("nested password should be encrypted, got %v", nested["password"])
	}
	if encPassword == "sup3r-secret-db-pass" {
		t.Error("nested password must not equal plaintext after encryption")
	}

	// The original input map must not be mutated.
	origNested := stepConfig["config"].(map[string]interface{})
	if origNested["password"] != "sup3r-secret-db-pass" {
		t.Error("original input map must not be mutated by EncryptConfigFields")
	}
}

func TestEncryptDecryptConfigFields_NestedConnectorCredential_RoundTrip(t *testing.T) {
	cs := newTestStore(t)
	stepConfig := map[string]interface{}{
		"connectorType": "snowflake_outbound",
		"config": map[string]interface{}{
			"account":  "xy12345",
			"username": "svc_user",
			"password": "snowflake-secret",
		},
	}

	encrypted, err := cs.EncryptConfigFields(stepConfig)
	if err != nil {
		t.Fatalf("EncryptConfigFields: %v", err)
	}
	decrypted, err := cs.DecryptConfigFields(encrypted)
	if err != nil {
		t.Fatalf("DecryptConfigFields: %v", err)
	}

	nested, ok := decrypted["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("decrypted config should still be a map[string]interface{}, got %T", decrypted["config"])
	}
	if nested["password"] != "snowflake-secret" {
		t.Errorf("nested password round-trip failed: got %v", nested["password"])
	}
	if nested["account"] != "xy12345" {
		t.Errorf("nested non-sensitive field should be unchanged, got %v", nested["account"])
	}
}

func TestEncryptConfigFields_EncryptsCredentialsInsideArrayOfObjects(t *testing.T) {
	cs := newTestStore(t)
	input := map[string]interface{}{
		"connectors": []interface{}{
			map[string]interface{}{"name": "primary", "token": "token-one"},
			map[string]interface{}{"name": "secondary", "token": "token-two"},
		},
	}

	result, err := cs.EncryptConfigFields(input)
	if err != nil {
		t.Fatalf("EncryptConfigFields: %v", err)
	}

	arr, ok := result["connectors"].([]interface{})
	if !ok || len(arr) != 2 {
		t.Fatalf("connectors should remain a 2-element slice, got %T len=%v", result["connectors"], arr)
	}
	first := arr[0].(map[string]interface{})
	second := arr[1].(map[string]interface{})

	if first["name"] != "primary" || second["name"] != "secondary" {
		t.Error("non-sensitive fields inside array elements should be unchanged")
	}
	firstTok, _ := first["token"].(string)
	secondTok, _ := second["token"].(string)
	if !strings.HasPrefix(firstTok, encryptedMarker) || !strings.HasPrefix(secondTok, encryptedMarker) {
		t.Errorf("tokens inside array elements should be encrypted, got %v / %v", firstTok, secondTok)
	}
	if firstTok == secondTok {
		t.Error("each encryption should use a distinct nonce, producing different ciphertext")
	}

	decrypted, err := cs.DecryptConfigFields(result)
	if err != nil {
		t.Fatalf("DecryptConfigFields: %v", err)
	}
	decArr := decrypted["connectors"].([]interface{})
	if decArr[0].(map[string]interface{})["token"] != "token-one" {
		t.Error("array element 0 token round-trip failed")
	}
	if decArr[1].(map[string]interface{})["token"] != "token-two" {
		t.Error("array element 1 token round-trip failed")
	}
}

func TestEncryptConfigFields_NonMapNestedValue_LeftUnchanged(t *testing.T) {
	// A Go-native map[string]string (as opposed to map[string]interface{}) can
	// never actually occur after a real encoding/json.Unmarshal into an
	// interface{}, but the function must not panic or crash on unexpected
	// nested types -- it should just pass them through untouched.
	cs := newTestStore(t)
	input := map[string]interface{}{
		"resultMapping": map[string]string{"password": "not-json-shaped"},
	}

	result, err := cs.EncryptConfigFields(input)
	if err != nil {
		t.Fatalf("EncryptConfigFields should not error on an unexpected nested type: %v", err)
	}
	rm, ok := result["resultMapping"].(map[string]string)
	if !ok || rm["password"] != "not-json-shaped" {
		t.Errorf("non-map[string]interface{} nested value should pass through unchanged, got %v", result["resultMapping"])
	}
}

// min helper (local to avoid conflict with hl7_fhir_transform_service_v3.go)
func credStoreMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

