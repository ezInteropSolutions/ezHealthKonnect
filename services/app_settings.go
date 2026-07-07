// services/app_settings.go
// AppSettingsCache — thread-safe, in-process cache for system_settings rows.
//
// Usage:
//   services.InitAppSettings(db)          // called once in main.go after DB connect
//   cfg := services.GetAppSettings().GetHL7FHIRSettings()
//   services.GetAppSettings().Invalidate("hl7_fhir")  // after a PUT to clear stale cache

package services

import (
	"database/sql"
	"encoding/json"
	"sync"
	"time"
)

// ─── Singleton ────────────────────────────────────────────────────────────────

var globalAppSettings *AppSettingsCache

// InitAppSettings initialises the package-level singleton.  Call once in main.go.
func InitAppSettings(db *sql.DB) {
	globalAppSettings = newAppSettingsCache(db)
}

// GetAppSettings returns the singleton.  Safe to call before InitAppSettings —
// returns a no-op cache that always returns defaults.
func GetAppSettings() *AppSettingsCache {
	if globalAppSettings == nil {
		return &AppSettingsCache{
			cache: make(map[string]cachedEntry),
			ttl:   5 * time.Minute,
		}
	}
	return globalAppSettings
}

// ─── Cache ────────────────────────────────────────────────────────────────────

type cachedEntry struct {
	raw json.RawMessage
	exp time.Time
}

// AppSettingsCache provides typed accessors for every system_settings key.
// Cached values expire after ttl (default 5 min); explicit Invalidate() flushes immediately.
type AppSettingsCache struct {
	db    *sql.DB
	mu    sync.RWMutex
	cache map[string]cachedEntry
	ttl   time.Duration
}

func newAppSettingsCache(db *sql.DB) *AppSettingsCache {
	return &AppSettingsCache{
		db:    db,
		cache: make(map[string]cachedEntry),
		ttl:   5 * time.Minute,
	}
}

// get returns the raw JSONB for a settings key, loading from DB on cache miss.
func (c *AppSettingsCache) get(key string) json.RawMessage {
	c.mu.RLock()
	entry, ok := c.cache[key]
	c.mu.RUnlock()

	if ok && time.Now().Before(entry.exp) {
		return entry.raw
	}

	raw := c.loadFromDB(key)

	c.mu.Lock()
	c.cache[key] = cachedEntry{raw: raw, exp: time.Now().Add(c.ttl)}
	c.mu.Unlock()

	return raw
}

func (c *AppSettingsCache) loadFromDB(key string) json.RawMessage {
	if c.db == nil {
		return json.RawMessage(`{}`)
	}
	var raw string
	err := c.db.QueryRow(`SELECT value FROM system_settings WHERE key = $1`, key).Scan(&raw)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(raw)
}

// Invalidate removes a cached entry so the next call reloads from DB.
func (c *AppSettingsCache) Invalidate(key string) {
	c.mu.Lock()
	delete(c.cache, key)
	c.mu.Unlock()
}

// ─── HL7 / FHIR Settings ──────────────────────────────────────────────────────

// HL7FHIRSettings holds processing defaults for HL7 parsing and FHIR transformation.
type HL7FHIRSettings struct {
	DefaultHL7Version   string `json:"default_hl7_version"`
	DefaultFHIRVersion  string `json:"default_fhir_version"`
	HL7ValidationMode   string `json:"hl7_validation_mode"`
	FHIRValidationMode  string `json:"fhir_validation_mode"`
	DefaultEncoding     string `json:"default_encoding"`
	SegmentTerminator   string `json:"segment_terminator"`
	SchemaCacheTTLMins  int    `json:"schema_cache_ttl_minutes"`
}

func (c *AppSettingsCache) GetHL7FHIRSettings() HL7FHIRSettings {
	cfg := HL7FHIRSettings{
		DefaultHL7Version:  "2.5",
		DefaultFHIRVersion: "R4",
		HL7ValidationMode:  "lenient",
		FHIRValidationMode: "lenient",
		DefaultEncoding:    "UTF-8",
		SegmentTerminator:  "CR",
		SchemaCacheTTLMins: 60,
	}
	_ = json.Unmarshal(c.get("hl7_fhir"), &cfg)
	return cfg
}

// ─── Message Queue Settings ───────────────────────────────────────────────────

// MessageQueueSettings controls queue depth, retention, and concurrency.
type MessageQueueSettings struct {
	MaxQueueDepth         int    `json:"max_queue_depth"`
	DLQRetentionDays      int    `json:"dlq_retention_days"`
	MessageRetentionDays  int    `json:"message_retention_days"`
	ProcessingConcurrency int    `json:"processing_concurrency"`
	BatchSize             int    `json:"batch_size"`
	OverflowAction        string `json:"overflow_action"`
	ProcessingTimeoutMs   int    `json:"processing_timeout_ms"`
}

func (c *AppSettingsCache) GetMessageQueueSettings() MessageQueueSettings {
	cfg := MessageQueueSettings{
		MaxQueueDepth:         10000,
		DLQRetentionDays:      30,
		MessageRetentionDays:  90,
		ProcessingConcurrency: 5,
		BatchSize:             100,
		OverflowAction:        "dlq",
		ProcessingTimeoutMs:   30000,
	}
	_ = json.Unmarshal(c.get("message_queue"), &cfg)
	return cfg
}

// ─── Connector Default Settings ───────────────────────────────────────────────

// ConnectorDefaultSettings provides fallback timeouts and pool sizes for all connectors.
type ConnectorDefaultSettings struct {
	ConnectionTimeoutMs  int    `json:"connection_timeout_ms"`
	ReadTimeoutMs        int    `json:"read_timeout_ms"`
	WriteTimeoutMs       int    `json:"write_timeout_ms"`
	PoolSize             int    `json:"pool_size"`
	KeepAliveSeconds     int    `json:"keep_alive_seconds"`
	MaxReconnectAttempts int    `json:"max_reconnect_attempts"`
	TLSValidation        string `json:"tls_validation"`
}

func (c *AppSettingsCache) GetConnectorDefaultSettings() ConnectorDefaultSettings {
	cfg := ConnectorDefaultSettings{
		ConnectionTimeoutMs:  5000,
		ReadTimeoutMs:        30000,
		WriteTimeoutMs:       30000,
		PoolSize:             10,
		KeepAliveSeconds:     30,
		MaxReconnectAttempts: 5,
		TLSValidation:        "strict",
	}
	_ = json.Unmarshal(c.get("connector_defaults"), &cfg)
	return cfg
}

// ─── Alert Default Settings ───────────────────────────────────────────────────

// AlertDefaultSettings provides default thresholds for new alert rules and retention policies.
type AlertDefaultSettings struct {
	ErrorRateThresholdPct  float64 `json:"error_rate_threshold_pct"`
	LatencyThresholdMs     int     `json:"latency_threshold_ms"`
	QueueDepthThresholdPct float64 `json:"queue_depth_threshold_pct"`
	ConsecutiveFailures    int     `json:"consecutive_failures"`
	CooldownMinutes        int     `json:"cooldown_minutes"`
	HistoryRetentionDays   int     `json:"history_retention_days"`
	MetricsRetentionDays   int     `json:"metrics_retention_days"`
	AutoResolveMinutes     int     `json:"auto_resolve_minutes"`
}

func (c *AppSettingsCache) GetAlertDefaultSettings() AlertDefaultSettings {
	cfg := AlertDefaultSettings{
		ErrorRateThresholdPct:  5.0,
		LatencyThresholdMs:     5000,
		QueueDepthThresholdPct: 80.0,
		ConsecutiveFailures:    3,
		CooldownMinutes:        15,
		HistoryRetentionDays:   90,
		MetricsRetentionDays:   30,
		AutoResolveMinutes:     60,
	}
	_ = json.Unmarshal(c.get("alert_defaults"), &cfg)
	return cfg
}

// ─── Performance Settings ─────────────────────────────────────────────────────

// PerformanceSettings controls log verbosity and DB pool sizes.
type PerformanceSettings struct {
	LogLevel              string `json:"log_level"`
	DBMaxOpenConnections  int    `json:"db_max_open_connections"`
	DBMaxIdleConnections  int    `json:"db_max_idle_connections"`
	DBQueryTimeoutMs      int    `json:"db_query_timeout_ms"`
}

func (c *AppSettingsCache) GetPerformanceSettings() PerformanceSettings {
	cfg := PerformanceSettings{
		LogLevel:             "info",
		DBMaxOpenConnections: 25,
		DBMaxIdleConnections: 5,
		DBQueryTimeoutMs:     5000,
	}
	_ = json.Unmarshal(c.get("performance"), &cfg)
	return cfg
}

// ─── AI Settings ─────────────────────────────────────────────────────────────

// AIConfig is the complete AI subsystem configuration stored in system_settings key="ai".
// All inference runs on-premise via Ollama — no PHI leaves the network.
type AIConfig struct {
	Enabled            bool              `json:"enabled"`
	Ollama             OllamaProviderCfg `json:"ollama"`
	Queue              AIQueueCfg        `json:"queue"`
	// FeedbackWebhookURL is the endpoint to POST anonymized aggregate feedback.
	// Only counts + admin comments are sent — never prompt/response content or PHI.
	// Leave empty to disable the "Submit to Team" button.
	FeedbackWebhookURL string            `json:"feedback_webhook_url"`
	// AgentModeEnabled turns on tool-calling ("Agent Mode") in ezCompanion — the
	// model may propose actions (retry a message, activate/deactivate an interface)
	// that a human must approve before they execute. Off by default.
	AgentModeEnabled bool `json:"agent_mode_enabled"`
}

// OllamaProviderCfg configures the local Ollama instance.
type OllamaProviderCfg struct {
	Host       string `json:"host"`        // default: http://ollama:11434
	ChatModel  string `json:"chat_model"`  // default: llama3.2:3b
	EmbedModel string `json:"embed_model"` // default: nomic-embed-text
}

// AIQueueCfg controls concurrency limiting for LLM requests.
type AIQueueCfg struct {
	MaxConcurrent       int `json:"max_concurrent"`        // default 2
	QueueTimeoutSeconds int `json:"queue_timeout_seconds"` // default 30
}

// GetAIConfig returns AI settings from the cache, falling back to sensible defaults.
func (c *AppSettingsCache) GetAIConfig() AIConfig {
	cfg := AIConfig{
		Enabled: true,
		Ollama: OllamaProviderCfg{
			Host:       "http://ollama:11434",
			ChatModel:  "llama3.2:3b",
			EmbedModel: "nomic-embed-text",
		},
		Queue: AIQueueCfg{MaxConcurrent: 2, QueueTimeoutSeconds: 30},
	}
	_ = json.Unmarshal(c.get("ai"), &cfg)
	return cfg
}

// ─── AI Model Catalog ─────────────────────────────────────────────────────────

// OllamaModelInfo describes a recommended Ollama model for ezHealthKonnect.
type OllamaModelInfo struct {
	Name            string   `json:"name"`              // ollama pull name, e.g. "qwen2.5-coder:7b"
	DisplayName     string   `json:"display_name"`
	ParamsBillion   float64  `json:"params_billion"`
	RAMRequiredGB   int      `json:"ram_required_gb"`   // model weights in RAM
	CPUTokensPerSec int      `json:"cpu_tokens_sec"`    // approx on 4-core x86 (no GPU)
	UseCases        []string `json:"use_cases"`         // "code", "reasoning", "general"
	EmbedOnly       bool     `json:"embed_only"`        // true for nomic-embed-text
	Recommended     bool     `json:"recommended"`       // default recommendation
	Notes           string   `json:"notes"`
}

// OllamaModelCatalog returns the curated list of Ollama models for healthcare integration.
// Models are ordered from smallest to largest.
func OllamaModelCatalog() []OllamaModelInfo {
	return []OllamaModelInfo{
		{
			Name: "nomic-embed-text", DisplayName: "Nomic Embed Text",
			ParamsBillion: 0.137, RAMRequiredGB: 1, CPUTokensPerSec: 0,
			UseCases: []string{"embeddings"}, EmbedOnly: true,
			Notes: "Required for RAG knowledge base. Always pull this model.",
		},
		{
			Name: "llama3.2:1b", DisplayName: "Llama 3.2 (1B)",
			ParamsBillion: 1, RAMRequiredGB: 2, CPUTokensPerSec: 20,
			UseCases: []string{"general"},
			Notes: "Fastest on CPU. Limited reasoning. Use only for simple Q&A on low-spec servers.",
		},
		{
			Name: "llama3.2:3b", DisplayName: "Llama 3.2 (3B)",
			ParamsBillion: 3, RAMRequiredGB: 3, CPUTokensPerSec: 10,
			UseCases: []string{"general", "code"}, Recommended: true,
			Notes: "Default. Best balance of speed and quality for CPU-only servers. Good for Q&A and basic scripts.",
		},
		{
			Name: "qwen2.5-coder:7b", DisplayName: "Qwen 2.5 Coder (7B)",
			ParamsBillion: 7, RAMRequiredGB: 6, CPUTokensPerSec: 4,
			UseCases: []string{"code", "reasoning"},
			Notes: "Best for JavaScript script generation and HL7→FHIR mapping logic. Recommended if you have 16GB+ RAM.",
		},
		{
			Name: "llama3.1:8b", DisplayName: "Llama 3.1 (8B)",
			ParamsBillion: 8, RAMRequiredGB: 6, CPUTokensPerSec: 3,
			UseCases: []string{"general", "reasoning"},
			Notes: "Good general reasoning. Slower than 3B. Use when answer quality matters more than speed.",
		},
		{
			Name: "qwen2.5-coder:14b", DisplayName: "Qwen 2.5 Coder (14B)",
			ParamsBillion: 14, RAMRequiredGB: 10, CPUTokensPerSec: 2,
			UseCases: []string{"code", "reasoning"},
			Notes: "Best script generation quality. Requires 16GB+ RAM and GPU recommended for reasonable speed.",
		},
		{
			Name: "codellama:7b", DisplayName: "Code Llama (7B)",
			ParamsBillion: 7, RAMRequiredGB: 6, CPUTokensPerSec: 4,
			UseCases: []string{"code"},
			Notes: "Meta's code-focused model. Good alternative to Qwen Coder for script steps.",
		},
	}
}

// AIServerRequirements describes minimum and recommended server specs for a given message volume.
type AIServerRequirements struct {
	Tier             string `json:"tier"`               // "light" | "medium" | "heavy"
	MessagesPerHour  string `json:"messages_per_hour"`
	MinRAMGB         int    `json:"min_ram_gb"`
	RecommendedRAMGB int    `json:"recommended_ram_gb"`
	MinCPUCores      int    `json:"min_cpu_cores"`
	GPUBenefit       string `json:"gpu_benefit"`        // "none" | "recommended" | "required"
	RecommendedModel string `json:"recommended_model"`
	Notes            string `json:"notes"`
}

// AIServerRequirementsForLoad returns server requirements based on message processing volume.
func AIServerRequirementsForLoad(messagesPerHour int) AIServerRequirements {
	switch {
	case messagesPerHour <= 100:
		return AIServerRequirements{
			Tier: "light", MessagesPerHour: "< 100/hr",
			MinRAMGB: 8, RecommendedRAMGB: 8, MinCPUCores: 4,
			GPUBenefit: "none", RecommendedModel: "llama3.2:3b",
			Notes: "Standard server. CPU-only Ollama handles this volume with llama3.2:3b at ~10 tokens/sec.",
		}
	case messagesPerHour <= 1000:
		return AIServerRequirements{
			Tier: "medium", MessagesPerHour: "100–1,000/hr",
			MinRAMGB: 16, RecommendedRAMGB: 16, MinCPUCores: 8,
			GPUBenefit: "recommended", RecommendedModel: "qwen2.5-coder:7b",
			Notes: "16GB RAM lets you run qwen2.5-coder:7b for better script generation while handling pipeline concurrency. GPU (8GB VRAM) reduces script-gen latency from ~30s to ~3s.",
		}
	default:
		return AIServerRequirements{
			Tier: "heavy", MessagesPerHour: "> 1,000/hr",
			MinRAMGB: 32, RecommendedRAMGB: 32, MinCPUCores: 16,
			GPUBenefit: "required", RecommendedModel: "qwen2.5-coder:14b",
			Notes: "GPU required for acceptable AI response times at this volume. 8–16GB VRAM. Without GPU, limit AI concurrency to 1 and expect 20–60s response times.",
		}
	}
}

// ─── Security Settings (for Go-side consumers) ────────────────────────────────

// SecuritySettings holds auth and access-control policy values.
type SecuritySettings struct {
	SessionTimeoutMinutes  int `json:"session_timeout_minutes"`
	JWTExpiryHours         int `json:"jwt_expiry_hours"`
	MaxLoginAttempts       int `json:"max_login_attempts"`
	LockoutDurationMinutes int `json:"lockout_duration_minutes"`
	PasswordMinLength      int `json:"password_min_length"`
}

func (c *AppSettingsCache) GetSecuritySettings() SecuritySettings {
	cfg := SecuritySettings{
		SessionTimeoutMinutes:  1440,
		JWTExpiryHours:         24,
		MaxLoginAttempts:       5,
		LockoutDurationMinutes: 30,
		PasswordMinLength:      8,
	}
	_ = json.Unmarshal(c.get("security"), &cfg)
	return cfg
}
