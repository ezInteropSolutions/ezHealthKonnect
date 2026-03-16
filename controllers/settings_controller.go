// controllers/settings_controller.go
// SettingsController — admin API for system-level runtime configuration.
//
// Endpoints (all under /api/system/settings):
//   GET  /storage        — return current object-storage config (sensitive fields masked)
//   PUT  /storage        — save new config (sensitive fields encrypted in DB)
//   POST /storage/test   — validate credentials by connecting to storage and listing the bucket

package controllers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	rg.GET("/storage", sc.GetStorageSettings)
	rg.PUT("/storage", sc.UpdateStorageSettings)
	rg.POST("/storage/test", sc.TestStorageConnection)
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
