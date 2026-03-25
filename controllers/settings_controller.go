// controllers/settings_controller.go
// SettingsController — admin API for system-level runtime configuration.
//
// Endpoints (all under /api/system/settings):
//   Object Storage: GET/PUT /storage, POST /storage/test
//   SMTP:           GET/PUT /smtp,    POST /smtp/test
//   Security:       GET/PUT /security
//   HL7/FHIR:       GET/PUT /hl7-fhir
//   Message Queue:  GET/PUT /message-queue
//   Connectors:     GET/PUT /connector-defaults
//   Alert Defaults: GET/PUT /alert-defaults
//   Performance:    GET/PUT /performance

package controllers

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"

	"ezhealthkonnect/services"
	"ezhealthkonnect/services/storage"

	"github.com/gin-gonic/gin"
)

// StorageSettingsConfig is the shape stored in system_settings.value for key "object_storage".
type StorageSettingsConfig struct {
	Driver    string `json:"driver"`               // "s3" | "local"
	Bucket    string `json:"bucket"`               // bucket / container name
	Region    string `json:"region"`               // AWS region (e.g. "us-east-1")
	Endpoint  string `json:"endpoint"`             // custom endpoint (MinIO, LocalStack)
	PathStyle bool   `json:"path_style"`           // force path-style URLs (required for MinIO)
	AccessKey string `json:"access_key"`           // access key ID (masked in responses)
	SecretKey string `json:"secret_key"`           // secret access key (masked in responses)
	LocalPath string `json:"local_path,omitempty"` // base directory for local driver
}

const settingsKeyStorage = "object_storage"

// SettingsController handles admin settings endpoints.
type SettingsController struct {
	db        *sql.DB
	credStore *services.CredentialStore
}

// NewSettingsController creates the controller.
func NewSettingsController(db *sql.DB, credStore *services.CredentialStore) *SettingsController {
	return &SettingsController{db: db, credStore: credStore}
}

// RegisterRoutes attaches routes to the provided Gin router group.
// Typical usage:  settingsGroup := systemGroup.Group("/settings")
//
//	settingsCtrl.RegisterRoutes(settingsGroup)
func (sc *SettingsController) RegisterRoutes(rg *gin.RouterGroup) {
	// ── Object Storage (existing) ───────────────────────────────────────────
	rg.GET("/storage", sc.GetStorageSettings)
	rg.PUT("/storage", sc.UpdateStorageSettings)
	rg.POST("/storage/test", sc.TestStorageConnection)

	// ── SMTP ────────────────────────────────────────────────────────────────
	rg.GET("/smtp", sc.GetSMTPSettings)
	rg.PUT("/smtp", sc.UpdateSMTPSettings)
	rg.POST("/smtp/test", sc.TestSMTPSettings)

	// ── Security ────────────────────────────────────────────────────────────
	rg.GET("/security", sc.GetSecuritySettings)
	rg.PUT("/security", sc.UpdateSecuritySettings)

	// ── HL7 / FHIR ──────────────────────────────────────────────────────────
	rg.GET("/hl7-fhir", sc.GetHL7FHIRSettings)
	rg.PUT("/hl7-fhir", sc.UpdateHL7FHIRSettings)

	// ── Message Queue ────────────────────────────────────────────────────────
	rg.GET("/message-queue", sc.GetMessageQueueSettings)
	rg.PUT("/message-queue", sc.UpdateMessageQueueSettings)

	// ── Connector Defaults ───────────────────────────────────────────────────
	rg.GET("/connector-defaults", sc.GetConnectorDefaultSettings)
	rg.PUT("/connector-defaults", sc.UpdateConnectorDefaultSettings)

	// ── Alert Defaults ───────────────────────────────────────────────────────
	rg.GET("/alert-defaults", sc.GetAlertDefaultSettings)
	rg.PUT("/alert-defaults", sc.UpdateAlertDefaultSettings)

	// ── Performance ──────────────────────────────────────────────────────────
	rg.GET("/performance", sc.GetPerformanceSettings)
	rg.PUT("/performance", sc.UpdatePerformanceSettings)

	// ── AI Assistant ─────────────────────────────────────────────────────────
	rg.GET("/ai", sc.GetAISettings)
	rg.PUT("/ai", sc.UpdateAISettings)
	rg.POST("/ai/test", sc.TestAIConnection)
	rg.GET("/ai/catalog", sc.GetAIModelCatalog)
}

// ─── GET /api/system/settings/storage ────────────────────────────────────────

// GetStorageSettings returns the current object-storage configuration.
// Sensitive fields (access_key, secret_key) are replaced with "••••••••".
func (sc *SettingsController) GetStorageSettings(c *gin.Context) {
	raw, err := sc.loadRawSetting(settingsKeyStorage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Decrypt if encrypted, then mask for the response
	decrypted, err := sc.credStore.DecryptConfig(raw)
	if err != nil {
		log.Printf("⚠️  SettingsController: decrypt storage config: %v", err)
		decrypted = raw // return as-is on decrypt failure (dev mode)
	}

	masked := sc.credStore.MaskSensitiveFields(decrypted)

	var cfg StorageSettingsConfig
	_ = json.Unmarshal(masked, &cfg)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    cfg,
	})
}

// ─── PUT /api/system/settings/storage ────────────────────────────────────────

// UpdateStorageSettings saves a new object-storage configuration.
// Sensitive fields are encrypted before persisting to the database.
func (sc *SettingsController) UpdateStorageSettings(c *gin.Context) {
	var incoming StorageSettingsConfig
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body: " + err.Error()})
		return
	}

	if err := validateStorageConfig(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	// If masked placeholders were sent back unchanged, load and preserve the real values.
	if incoming.AccessKey == "••••••••" || incoming.SecretKey == "••••••••" {
		existing, loadErr := sc.loadDecryptedStorageConfig()
		if loadErr == nil {
			if incoming.AccessKey == "••••••••" {
				incoming.AccessKey = existing.AccessKey
			}
			if incoming.SecretKey == "••••••••" {
				incoming.SecretKey = existing.SecretKey
			}
		}
	}

	// Marshal then encrypt
	plain, err := json.Marshal(incoming)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "marshal error: " + err.Error()})
		return
	}

	encrypted, err := sc.credStore.EncryptConfig(json.RawMessage(plain))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "encrypt error: " + err.Error()})
		return
	}

	// Upsert into system_settings
	updatedBy := ""
	if u, exists := c.Get("user"); exists {
		if m, ok := u.(map[string]interface{}); ok {
			if email, ok := m["email"].(string); ok {
				updatedBy = email
			}
		}
	}

	_, err = sc.db.ExecContext(c.Request.Context(), `
		INSERT INTO system_settings (key, value, updated_by, updated_at)
		VALUES ($1, $2::jsonb, $3, NOW())
		ON CONFLICT (key) DO UPDATE
		  SET value      = EXCLUDED.value,
		      updated_by = EXCLUDED.updated_by,
		      updated_at = NOW()
	`, settingsKeyStorage, string(encrypted), updatedBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "database error: " + err.Error()})
		return
	}

	log.Printf("✅ [SettingsController] object_storage config updated by %q", updatedBy)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Storage configuration saved successfully",
	})
}

// ─── POST /api/system/settings/storage/test ───────────────────────────────────

// TestStorageConnection builds a driver from the supplied (or saved) config and
// verifies connectivity by ensuring the target bucket is accessible.
func (sc *SettingsController) TestStorageConnection(c *gin.Context) {
	// Accept an optional body; if empty, test the currently-saved config.
	var incoming *StorageSettingsConfig
	var body StorageSettingsConfig
	if err := c.ShouldBindJSON(&body); err == nil && (body.Driver != "" || body.Endpoint != "") {
		// Restore masked placeholders from saved config
		if body.AccessKey == "••••••••" || body.SecretKey == "••••••••" {
			saved, _ := sc.loadDecryptedStorageConfig()
			if saved != nil {
				if body.AccessKey == "••••••••" {
					body.AccessKey = saved.AccessKey
				}
				if body.SecretKey == "••••••••" {
					body.SecretKey = saved.SecretKey
				}
			}
		}
		incoming = &body
	} else {
		saved, err := sc.loadDecryptedStorageConfig()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "no config available to test"})
			return
		}
		incoming = saved
	}

	if err := validateStorageConfig(incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	start := time.Now()
	driver, err := buildDriverFromConfig(incoming)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to initialise driver: %v", err),
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	bucket := incoming.Bucket
	if bucket == "" {
		bucket = storage.DefaultBucket
	}

	if err := driver.EnsureBucket(ctx, bucket); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"driver":     driver.DriverName(),
			"bucket":     bucket,
			"latency_ms": time.Since(start).Milliseconds(),
			"error":      fmt.Sprintf("Bucket check failed: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"driver":     driver.DriverName(),
		"bucket":     bucket,
		"latency_ms": time.Since(start).Milliseconds(),
		"message":    "Connection successful",
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (sc *SettingsController) loadRawSetting(key string) (json.RawMessage, error) {
	if sc.db == nil {
		return json.RawMessage(`{}`), nil
	}
	var raw string
	err := sc.db.QueryRow(`SELECT value FROM system_settings WHERE key = $1`, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return json.RawMessage(`{}`), nil
	}
	if err != nil {
		return nil, fmt.Errorf("query system_settings: %w", err)
	}
	return json.RawMessage(raw), nil
}

func (sc *SettingsController) loadDecryptedStorageConfig() (*StorageSettingsConfig, error) {
	raw, err := sc.loadRawSetting(settingsKeyStorage)
	if err != nil {
		return nil, err
	}
	decrypted, err := sc.credStore.DecryptConfig(raw)
	if err != nil {
		decrypted = raw
	}
	var cfg StorageSettingsConfig
	if err := json.Unmarshal(decrypted, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadStorageConfigFromDB reads the live storage config, merges with env-var
// fallbacks for any empty fields, and returns a ready-to-use StorageSettingsConfig.
// Returns nil if the database has no overrides (caller should use env vars directly).
func LoadStorageConfigFromDB(db *sql.DB, credStore *services.CredentialStore) *StorageSettingsConfig {
	if db == nil {
		return nil
	}
	sc := &SettingsController{db: db, credStore: credStore}
	cfg, err := sc.loadDecryptedStorageConfig()
	if err != nil || cfg == nil {
		return nil
	}
	// If every meaningful field is empty the row is the blank seed — ignore it.
	if cfg.Driver == "" && cfg.Bucket == "" && cfg.Endpoint == "" {
		return nil
	}
	return cfg
}

func validateStorageConfig(cfg *StorageSettingsConfig) error {
	if cfg.Driver != "s3" && cfg.Driver != "local" && cfg.Driver != "" {
		return fmt.Errorf("driver must be \"s3\" or \"local\" (got %q)", cfg.Driver)
	}
	return nil
}

func buildDriverFromConfig(cfg *StorageSettingsConfig) (storage.ObjectStorageDriver, error) {
	switch cfg.Driver {
	case "s3", "":
		s3Cfg := storage.S3DriverConfig{
			Region:          cfg.Region,
			Bucket:          cfg.Bucket,
			Endpoint:        cfg.Endpoint,
			ForcePathStyle:  cfg.PathStyle,
			AccessKeyID:     cfg.AccessKey,
			SecretAccessKey: cfg.SecretKey,
		}
		if s3Cfg.Region == "" {
			s3Cfg.Region = storage.DefaultRegion
		}
		return storage.NewS3Driver(s3Cfg)
	case "local":
		p := cfg.LocalPath
		if p == "" {
			p = storage.DefaultLocalPath
		}
		return storage.NewLocalDriver(p)
	default:
		return nil, fmt.Errorf("unsupported driver %q", cfg.Driver)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ── Shared helpers for non-sensitive settings ──────────────────────────────────
// ═══════════════════════════════════════════════════════════════════════════════

// updatedByFromContext extracts the authenticated user's email from the Gin context.
func (sc *SettingsController) updatedByFromContext(c *gin.Context) string {
	if u, exists := c.Get("user"); exists {
		if m, ok := u.(map[string]interface{}); ok {
			if email, ok := m["email"].(string); ok {
				return email
			}
		}
	}
	return ""
}

// savePlainSetting marshals val and upserts it as unencrypted JSONB.
func (sc *SettingsController) savePlainSetting(ctx context.Context, key, updatedBy string, val interface{}) error {
	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	_, err = sc.db.ExecContext(ctx, `
		INSERT INTO system_settings (key, value, updated_by, updated_at)
		VALUES ($1, $2::jsonb, $3, NOW())
		ON CONFLICT (key) DO UPDATE
		  SET value      = EXCLUDED.value,
		      updated_by = EXCLUDED.updated_by,
		      updated_at = NOW()
	`, key, string(data), updatedBy)
	return err
}

// ═══════════════════════════════════════════════════════════════════════════════
// ── SMTP Settings ─────────────────────────────────────────────────────────────
// ═══════════════════════════════════════════════════════════════════════════════

const settingsKeySMTP = "smtp"

// SMTPSettingsConfig is the shape stored under key "smtp".
// Password is AES-256-GCM encrypted via CredentialStore.
type SMTPSettingsConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	UseTLS      bool   `json:"use_tls"`
	RequireAuth bool   `json:"require_auth"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	FromAddress string `json:"from_address"`
	FromName    string `json:"from_name"`
}

// GetSMTPSettings returns the current SMTP config with password masked.
func (sc *SettingsController) GetSMTPSettings(c *gin.Context) {
	raw, err := sc.loadRawSetting(settingsKeySMTP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	decrypted, err := sc.credStore.DecryptConfig(raw)
	if err != nil {
		decrypted = raw
	}
	masked := sc.credStore.MaskSensitiveFields(decrypted)
	var cfg SMTPSettingsConfig
	_ = json.Unmarshal(masked, &cfg)
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

// UpdateSMTPSettings saves a new SMTP configuration with password encrypted.
func (sc *SettingsController) UpdateSMTPSettings(c *gin.Context) {
	var incoming SMTPSettingsConfig
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body: " + err.Error()})
		return
	}
	if incoming.Port == 0 {
		incoming.Port = 587
	}

	// Restore masked password placeholder
	if incoming.Password == "••••••••" {
		raw, _ := sc.loadRawSetting(settingsKeySMTP)
		if dec, err := sc.credStore.DecryptConfig(raw); err == nil {
			var existing SMTPSettingsConfig
			if json.Unmarshal(dec, &existing) == nil {
				incoming.Password = existing.Password
			}
		}
	}

	plain, err := json.Marshal(incoming)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	encrypted, err := sc.credStore.EncryptConfig(json.RawMessage(plain))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	updatedBy := sc.updatedByFromContext(c)
	_, err = sc.db.ExecContext(c.Request.Context(), `
		INSERT INTO system_settings (key, value, updated_by, updated_at)
		VALUES ($1, $2::jsonb, $3, NOW())
		ON CONFLICT (key) DO UPDATE
		  SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = NOW()
	`, settingsKeySMTP, string(encrypted), updatedBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "database error: " + err.Error()})
		return
	}

	services.GetAppSettings().Invalidate(settingsKeySMTP)
	log.Printf("✅ [SettingsController] smtp config updated by %q", updatedBy)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "SMTP settings saved"})
}

// TestSMTPSettings sends a test email to validate the SMTP configuration.
func (sc *SettingsController) TestSMTPSettings(c *gin.Context) {
	var req struct {
		SMTPSettingsConfig
		TestRecipient string `json:"test_recipient"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body: " + err.Error()})
		return
	}

	// If password is masked, load from DB
	if req.Password == "••••••••" {
		raw, _ := sc.loadRawSetting(settingsKeySMTP)
		if dec, err := sc.credStore.DecryptConfig(raw); err == nil {
			var existing SMTPSettingsConfig
			if json.Unmarshal(dec, &existing) == nil {
				req.Password = existing.Password
			}
		}
	}

	if req.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "SMTP host is required"})
		return
	}
	if req.TestRecipient == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "test_recipient is required"})
		return
	}
	if req.Port == 0 {
		req.Port = 587
	}
	if req.FromAddress == "" {
		req.FromAddress = "alerts@ezhealthkonnect.local"
	}
	if req.FromName == "" {
		req.FromName = "ezHealthKonnect"
	}

	start := time.Now()
	addr := net.JoinHostPort(req.Host, fmt.Sprintf("%d", req.Port))

	subject := "ezHealthKonnect — SMTP Test"
	bodyText := fmt.Sprintf(
		"To: %s\r\nFrom: %s <%s>\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nThis is a test email from ezHealthKonnect.\r\n\r\nIf you received this, your SMTP configuration is working correctly.\r\n\r\nSent at: %s",
		req.TestRecipient, req.FromName, req.FromAddress, subject, time.Now().Format(time.RFC1123),
	)

	var auth smtp.Auth
	if req.RequireAuth && req.Username != "" {
		auth = smtp.PlainAuth("", req.Username, req.Password, req.Host)
	}

	var sendErr error
	if req.UseTLS {
		tlsCfg := &tls.Config{ServerName: req.Host, MinVersion: tls.VersionTLS12}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			// Fall back to STARTTLS
			sendErr = smtp.SendMail(addr, auth, req.FromAddress, []string{req.TestRecipient}, []byte(bodyText))
		} else {
			defer conn.Close()
			client, err := smtp.NewClient(conn, req.Host)
			if err != nil {
				sendErr = err
			} else {
				defer client.Quit()
				if auth != nil {
					_ = client.Auth(auth)
				}
				if err := client.Mail(req.FromAddress); err != nil {
					sendErr = err
				} else if err := client.Rcpt(req.TestRecipient); err != nil {
					sendErr = err
				} else {
					wc, err := client.Data()
					if err != nil {
						sendErr = err
					} else {
						_, sendErr = io.WriteString(wc, bodyText)
						_ = wc.Close()
					}
				}
			}
		}
	} else {
		sendErr = smtp.SendMail(addr, auth, req.FromAddress, []string{req.TestRecipient}, []byte(bodyText))
	}

	latencyMs := time.Since(start).Milliseconds()
	if sendErr != nil {
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"error":      sendErr.Error(),
			"latency_ms": latencyMs,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    fmt.Sprintf("Test email sent to %s", req.TestRecipient),
		"latency_ms": latencyMs,
	})
}

// LoadSMTPConfigFromDB decrypts and returns the system SMTP settings.
// Returns nil if unconfigured (no host set). Used by alert_notification_service.
func LoadSMTPConfigFromDB(db *sql.DB, credStore *services.CredentialStore) *SMTPSettingsConfig {
	if db == nil {
		return nil
	}
	sc := &SettingsController{db: db, credStore: credStore}
	raw, err := sc.loadRawSetting(settingsKeySMTP)
	if err != nil {
		return nil
	}
	dec, err := credStore.DecryptConfig(raw)
	if err != nil {
		dec = raw
	}
	var cfg SMTPSettingsConfig
	if err := json.Unmarshal(dec, &cfg); err != nil || cfg.Host == "" {
		return nil
	}
	return &cfg
}

// ═══════════════════════════════════════════════════════════════════════════════
// ── Security Settings ─────────────────────────────────────────────────────────
// ═══════════════════════════════════════════════════════════════════════════════

const settingsKeySecurity = "security"

type SecuritySettingsConfig struct {
	SessionTimeoutMinutes  int `json:"session_timeout_minutes"`
	JWTExpiryHours         int `json:"jwt_expiry_hours"`
	MaxLoginAttempts       int `json:"max_login_attempts"`
	LockoutDurationMinutes int `json:"lockout_duration_minutes"`
	PasswordMinLength      int `json:"password_min_length"`
}

func (sc *SettingsController) GetSecuritySettings(c *gin.Context) {
	cfg := SecuritySettingsConfig{
		SessionTimeoutMinutes: 1440, JWTExpiryHours: 24,
		MaxLoginAttempts: 5, LockoutDurationMinutes: 30, PasswordMinLength: 8,
	}
	raw, err := sc.loadRawSetting(settingsKeySecurity)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	_ = json.Unmarshal(raw, &cfg)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

func (sc *SettingsController) UpdateSecuritySettings(c *gin.Context) {
	var incoming SecuritySettingsConfig
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if incoming.SessionTimeoutMinutes < 1 {
		incoming.SessionTimeoutMinutes = 1440
	}
	if incoming.JWTExpiryHours < 1 {
		incoming.JWTExpiryHours = 24
	}
	if incoming.MaxLoginAttempts < 1 {
		incoming.MaxLoginAttempts = 5
	}
	if incoming.LockoutDurationMinutes < 1 {
		incoming.LockoutDurationMinutes = 30
	}
	if incoming.PasswordMinLength < 6 {
		incoming.PasswordMinLength = 6
	}

	if err := sc.savePlainSetting(c.Request.Context(), settingsKeySecurity, sc.updatedByFromContext(c), incoming); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	services.GetAppSettings().Invalidate(settingsKeySecurity)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Security settings saved. JWT expiry and login lockout apply on next login. Session timeout applies on next Node.js restart."})
}

// ═══════════════════════════════════════════════════════════════════════════════
// ── HL7 / FHIR Settings ───────────────────────────────────────────────────────
// ═══════════════════════════════════════════════════════════════════════════════

const settingsKeyHL7FHIR = "hl7_fhir"

type HL7FHIRSettingsConfig struct {
	DefaultHL7Version  string `json:"default_hl7_version"`
	DefaultFHIRVersion string `json:"default_fhir_version"`
	HL7ValidationMode  string `json:"hl7_validation_mode"`
	FHIRValidationMode string `json:"fhir_validation_mode"`
	DefaultEncoding    string `json:"default_encoding"`
	SegmentTerminator  string `json:"segment_terminator"`
	SchemaCacheTTLMins int    `json:"schema_cache_ttl_minutes"`
}

func (sc *SettingsController) GetHL7FHIRSettings(c *gin.Context) {
	cfg := HL7FHIRSettingsConfig{
		DefaultHL7Version: "2.5", DefaultFHIRVersion: "R4",
		HL7ValidationMode: "lenient", FHIRValidationMode: "lenient",
		DefaultEncoding: "UTF-8", SegmentTerminator: "CR", SchemaCacheTTLMins: 60,
	}
	raw, err := sc.loadRawSetting(settingsKeyHL7FHIR)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	_ = json.Unmarshal(raw, &cfg)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

func (sc *SettingsController) UpdateHL7FHIRSettings(c *gin.Context) {
	var incoming HL7FHIRSettingsConfig
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if incoming.SchemaCacheTTLMins < 1 {
		incoming.SchemaCacheTTLMins = 60
	}
	if err := sc.savePlainSetting(c.Request.Context(), settingsKeyHL7FHIR, sc.updatedByFromContext(c), incoming); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	services.GetAppSettings().Invalidate(settingsKeyHL7FHIR)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "HL7/FHIR settings saved. Applied to all transformations on next request."})
}

// ═══════════════════════════════════════════════════════════════════════════════
// ── Message Queue Settings ────────────────────────────────────────────────────
// ═══════════════════════════════════════════════════════════════════════════════

const settingsKeyMessageQueue = "message_queue"

type MessageQueueSettingsConfig struct {
	MaxQueueDepth         int    `json:"max_queue_depth"`
	DLQRetentionDays      int    `json:"dlq_retention_days"`
	MessageRetentionDays  int    `json:"message_retention_days"`
	ProcessingConcurrency int    `json:"processing_concurrency"`
	BatchSize             int    `json:"batch_size"`
	OverflowAction        string `json:"overflow_action"`
	ProcessingTimeoutMs   int    `json:"processing_timeout_ms"`
}

func (sc *SettingsController) GetMessageQueueSettings(c *gin.Context) {
	cfg := MessageQueueSettingsConfig{
		MaxQueueDepth: 10000, DLQRetentionDays: 30, MessageRetentionDays: 90,
		ProcessingConcurrency: 5, BatchSize: 100, OverflowAction: "dlq", ProcessingTimeoutMs: 30000,
	}
	raw, err := sc.loadRawSetting(settingsKeyMessageQueue)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	_ = json.Unmarshal(raw, &cfg)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

func (sc *SettingsController) UpdateMessageQueueSettings(c *gin.Context) {
	var incoming MessageQueueSettingsConfig
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if incoming.MaxQueueDepth < 100 {
		incoming.MaxQueueDepth = 100
	}
	if incoming.DLQRetentionDays < 1 {
		incoming.DLQRetentionDays = 1
	}
	if incoming.MessageRetentionDays < 1 {
		incoming.MessageRetentionDays = 1
	}
	if incoming.ProcessingConcurrency < 1 {
		incoming.ProcessingConcurrency = 1
	}
	if incoming.BatchSize < 1 {
		incoming.BatchSize = 1
	}
	if incoming.ProcessingTimeoutMs < 1000 {
		incoming.ProcessingTimeoutMs = 1000
	}
	if err := sc.savePlainSetting(c.Request.Context(), settingsKeyMessageQueue, sc.updatedByFromContext(c), incoming); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	services.GetAppSettings().Invalidate(settingsKeyMessageQueue)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Message queue settings saved."})
}

// ═══════════════════════════════════════════════════════════════════════════════
// ── Connector Default Settings ────────────────────────────────────────────────
// ═══════════════════════════════════════════════════════════════════════════════

const settingsKeyConnectorDefaults = "connector_defaults"

type ConnectorDefaultSettingsConfig struct {
	ConnectionTimeoutMs  int    `json:"connection_timeout_ms"`
	ReadTimeoutMs        int    `json:"read_timeout_ms"`
	WriteTimeoutMs       int    `json:"write_timeout_ms"`
	PoolSize             int    `json:"pool_size"`
	KeepAliveSeconds     int    `json:"keep_alive_seconds"`
	MaxReconnectAttempts int    `json:"max_reconnect_attempts"`
	TLSValidation        string `json:"tls_validation"`
}

func (sc *SettingsController) GetConnectorDefaultSettings(c *gin.Context) {
	cfg := ConnectorDefaultSettingsConfig{
		ConnectionTimeoutMs: 5000, ReadTimeoutMs: 30000, WriteTimeoutMs: 30000,
		PoolSize: 10, KeepAliveSeconds: 30, MaxReconnectAttempts: 5, TLSValidation: "strict",
	}
	raw, err := sc.loadRawSetting(settingsKeyConnectorDefaults)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	_ = json.Unmarshal(raw, &cfg)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

func (sc *SettingsController) UpdateConnectorDefaultSettings(c *gin.Context) {
	var incoming ConnectorDefaultSettingsConfig
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if incoming.ConnectionTimeoutMs < 500 {
		incoming.ConnectionTimeoutMs = 500
	}
	if incoming.PoolSize < 1 {
		incoming.PoolSize = 1
	}
	if incoming.MaxReconnectAttempts < 0 {
		incoming.MaxReconnectAttempts = 0
	}
	if err := sc.savePlainSetting(c.Request.Context(), settingsKeyConnectorDefaults, sc.updatedByFromContext(c), incoming); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	services.GetAppSettings().Invalidate(settingsKeyConnectorDefaults)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Connector defaults saved. Applied to new connectors on next initialization."})
}

// ═══════════════════════════════════════════════════════════════════════════════
// ── Alert Default Settings ────────────────────────────────────────────────────
// ═══════════════════════════════════════════════════════════════════════════════

const settingsKeyAlertDefaults = "alert_defaults"

type AlertDefaultSettingsConfig struct {
	ErrorRateThresholdPct  float64 `json:"error_rate_threshold_pct"`
	LatencyThresholdMs     int     `json:"latency_threshold_ms"`
	QueueDepthThresholdPct float64 `json:"queue_depth_threshold_pct"`
	ConsecutiveFailures    int     `json:"consecutive_failures"`
	CooldownMinutes        int     `json:"cooldown_minutes"`
	HistoryRetentionDays   int     `json:"history_retention_days"`
	MetricsRetentionDays   int     `json:"metrics_retention_days"`
	AutoResolveMinutes     int     `json:"auto_resolve_minutes"`
}

func (sc *SettingsController) GetAlertDefaultSettings(c *gin.Context) {
	cfg := AlertDefaultSettingsConfig{
		ErrorRateThresholdPct: 5.0, LatencyThresholdMs: 5000, QueueDepthThresholdPct: 80.0,
		ConsecutiveFailures: 3, CooldownMinutes: 15, HistoryRetentionDays: 90,
		MetricsRetentionDays: 30, AutoResolveMinutes: 60,
	}
	raw, err := sc.loadRawSetting(settingsKeyAlertDefaults)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	_ = json.Unmarshal(raw, &cfg)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

func (sc *SettingsController) UpdateAlertDefaultSettings(c *gin.Context) {
	var incoming AlertDefaultSettingsConfig
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if incoming.CooldownMinutes < 1 {
		incoming.CooldownMinutes = 1
	}
	if incoming.HistoryRetentionDays < 1 {
		incoming.HistoryRetentionDays = 1
	}
	if incoming.MetricsRetentionDays < 1 {
		incoming.MetricsRetentionDays = 1
	}
	if err := sc.savePlainSetting(c.Request.Context(), settingsKeyAlertDefaults, sc.updatedByFromContext(c), incoming); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	services.GetAppSettings().Invalidate(settingsKeyAlertDefaults)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Alert defaults saved. Pre-filled when creating new alert rules."})
}

// ═══════════════════════════════════════════════════════════════════════════════
// ── Performance Settings ──────────────────────────────────────────────────────
// ═══════════════════════════════════════════════════════════════════════════════

const settingsKeyPerformance = "performance"

type PerformanceSettingsConfig struct {
	LogLevel             string `json:"log_level"`
	DBMaxOpenConnections int    `json:"db_max_open_connections"`
	DBMaxIdleConnections int    `json:"db_max_idle_connections"`
	DBQueryTimeoutMs     int    `json:"db_query_timeout_ms"`
}

func (sc *SettingsController) GetPerformanceSettings(c *gin.Context) {
	cfg := PerformanceSettingsConfig{
		LogLevel: "info", DBMaxOpenConnections: 25, DBMaxIdleConnections: 5, DBQueryTimeoutMs: 5000,
	}
	raw, err := sc.loadRawSetting(settingsKeyPerformance)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	_ = json.Unmarshal(raw, &cfg)

	// Reflect live log level from environment if set
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		cfg.LogLevel = envLevel
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

func (sc *SettingsController) UpdatePerformanceSettings(c *gin.Context) {
	var incoming PerformanceSettingsConfig
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if incoming.DBMaxOpenConnections < 1 {
		incoming.DBMaxOpenConnections = 1
	}
	if incoming.DBMaxIdleConnections < 0 {
		incoming.DBMaxIdleConnections = 0
	}
	if incoming.DBQueryTimeoutMs < 100 {
		incoming.DBQueryTimeoutMs = 100
	}
	if incoming.LogLevel == "" {
		incoming.LogLevel = "info"
	}

	if err := sc.savePlainSetting(c.Request.Context(), settingsKeyPerformance, sc.updatedByFromContext(c), incoming); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	services.GetAppSettings().Invalidate(settingsKeyPerformance)

	// Apply log level dynamically via environment variable (picked up by logger on next structured log call)
	_ = os.Setenv("LOG_LEVEL", incoming.LogLevel)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Performance settings saved. Log level applied immediately. DB pool changes require a Go backend restart.",
	})
}

// ═══════════════════════════════════════════════════════════════════════════════
// ── AI Settings ───────────────────────────────────────────────────────────────
// ═══════════════════════════════════════════════════════════════════════════════

const settingsKeyAI = "ai"

// GetAISettings returns current AI config plus the static model catalog and
// server requirements for the currently selected message volume.
func (sc *SettingsController) GetAISettings(c *gin.Context) {
	raw, err := sc.loadRawSetting(settingsKeyAI)
	if err != nil {
		// Return defaults — AI settings may not exist yet (migration not applied)
		raw = json.RawMessage(`{}`)
	}

	cfg := services.AIConfig{
		Enabled: true,
		Ollama: services.OllamaProviderCfg{
			Host:       "http://ollama:11434",
			ChatModel:  "llama3.2:3b",
			EmbedModel: "nomic-embed-text",
		},
		Queue: services.AIQueueCfg{MaxConcurrent: 2, QueueTimeoutSeconds: 30},
	}
	_ = json.Unmarshal(raw, &cfg)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":              cfg.Enabled,
			"ollama":               cfg.Ollama,
			"queue":                cfg.Queue,
			"feedback_webhook_url": cfg.FeedbackWebhookURL,
			"catalog":              services.OllamaModelCatalog(),
		},
	})
}

// UpdateAISettings saves the AI configuration.
func (sc *SettingsController) UpdateAISettings(c *gin.Context) {
	var incoming services.AIConfig
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body: " + err.Error()})
		return
	}

	// Enforce non-empty embed model — always required for RAG
	if incoming.Ollama.EmbedModel == "" {
		incoming.Ollama.EmbedModel = "nomic-embed-text"
	}
	if incoming.Ollama.Host == "" {
		incoming.Ollama.Host = "http://ollama:11434"
	}
	if incoming.Ollama.ChatModel == "" {
		incoming.Ollama.ChatModel = "llama3.2:3b"
	}
	if incoming.Queue.MaxConcurrent < 1 {
		incoming.Queue.MaxConcurrent = 1
	}
	if incoming.Queue.MaxConcurrent > 10 {
		incoming.Queue.MaxConcurrent = 10
	}
	if incoming.Queue.QueueTimeoutSeconds < 5 {
		incoming.Queue.QueueTimeoutSeconds = 5
	}

	if err := sc.savePlainSetting(c.Request.Context(), settingsKeyAI, sc.updatedByFromContext(c), incoming); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	services.GetAppSettings().Invalidate(settingsKeyAI)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "AI settings saved. Changes apply on the next request (no restart required).",
	})
}

// TestAIConnection tests connectivity to the configured Ollama host and returns
// available models. This does NOT save the config.
func (sc *SettingsController) TestAIConnection(c *gin.Context) {
	var body struct {
		Host string `json:"host"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Host == "" {
		// Fall back to stored config
		body.Host = services.GetAppSettings().GetAIConfig().Ollama.Host
	}
	if body.Host == "" {
		body.Host = "http://ollama:11434"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(body.Host, "/")+"/api/tags", nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error":   "Invalid host URL: " + err.Error(),
		})
		return
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error":   "Ollama unreachable at " + body.Host + ": " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Ollama returned HTTP %d", resp.StatusCode),
		})
		return
	}

	var tagsResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": "Failed to parse Ollama response: " + err.Error()})
		return
	}

	models := make([]string, len(tagsResp.Models))
	for i, m := range tagsResp.Models {
		models[i] = m.Name
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"host":       body.Host,
			"reachable":  true,
			"models":     models,
			"model_count": len(models),
		},
	})
}

// GetAIModelCatalog returns the static curated model catalog with server requirements.
func (sc *SettingsController) GetAIModelCatalog(c *gin.Context) {
	// Optional query param: messages_per_hour for server requirements advisor
	msgsPerHour := 0
	if v := c.Query("messages_per_hour"); v != "" {
		_, _ = fmt.Sscanf(v, "%d", &msgsPerHour)
	}

	result := gin.H{
		"catalog": services.OllamaModelCatalog(),
	}
	if msgsPerHour > 0 {
		result["server_requirements"] = services.AIServerRequirementsForLoad(msgsPerHour)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

