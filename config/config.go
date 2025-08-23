package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	// Server settings
	Port        string
	Environment string

	// Service URLs (keeping for backward compatibility)
	DictionaryServiceURL string
	NodeServiceURL       string

	// ezHealthKonnect Schema System
	SchemaDirectory   string
	EnableSchemaCache bool
	SchemaCacheSize   int
	SchemaSource      string // "filesystem" or "legacy_http"
	DefaultHL7Version string

	// NEW: FHIR Configuration
	FHIRSchemaDir      string
	DefaultFHIRVersion string
	DefaultFHIRProfile string
	EnableFHIRCache    bool
	FHIRCacheSize      int

	// CORS settings
	AllowedOrigins []string

	// Features
	EnablePostgreSQL    bool
	HIPAAComplianceMode bool
	HL7ProcessingMode   string
	HL7ValidationLevel  string
	VerboseLogging      bool

	// Database settings
	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string

	// Security
	JWTSecret     string
	SessionSecret string
	EncryptionKey string
}

// Load configuration from environment variables and .env file
func Load() *Config {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	cfg := &Config{
		// Server settings
		Port:        getEnv("API_PORT", "8080"),
		Environment: getEnv("NODE_ENV", "development"),

		// Service URLs (legacy support)
		DictionaryServiceURL: getEnv("DICTIONARY_SERVICE_URL", "http://localhost:3001"),
		NodeServiceURL:       getEnv("NODE_SERVICE_URL", "http://localhost:3001"),

		// ezHealthKonnect Schema System
		SchemaDirectory:   getEnv("EZHEALTHKONNECT_SCHEMA_DIR", "./schemas"),
		EnableSchemaCache: getBoolEnv("ENABLE_SCHEMA_CACHE", true),
		SchemaCacheSize:   getIntEnv("SCHEMA_CACHE_SIZE", 50),
		SchemaSource:      getEnv("SCHEMA_SOURCE", "filesystem"), // "filesystem" or "legacy_http"
		DefaultHL7Version: getEnv("DEFAULT_HL7_VERSION", "v2.5.1"),

		// NEW: FHIR Configuration
		DefaultFHIRVersion: getEnv("DEFAULT_FHIR_VERSION", "R4"),
		DefaultFHIRProfile: getEnv("DEFAULT_FHIR_PROFILE", "base"),
		EnableFHIRCache:    getBoolEnv("ENABLE_FHIR_CACHE", true),
		FHIRCacheSize:      getIntEnv("FHIR_CACHE_SIZE", 100),

		// CORS settings
		AllowedOrigins: getStringSlice("ADDITIONAL_CORS_ORIGINS",
			"http://localhost:3000,http://127.0.0.1:3000,http://localhost:8000,http://127.0.0.1:8000"),

		// Features
		EnablePostgreSQL:    getBoolEnv("ENABLE_POSTGRESQL", true),
		HIPAAComplianceMode: getBoolEnv("HIPAA_COMPLIANCE_MODE", true),
		HL7ProcessingMode:   getEnv("HL7_PROCESSING_MODE", "enhanced"),
		HL7ValidationLevel:  getEnv("HL7_VALIDATION_LEVEL", "basic"),
		VerboseLogging:      getBoolEnv("VERBOSE_LOGGING", false),

		// Database settings
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBName:     getEnv("DB_NAME", "ezhealthkonnect"),
		DBUser:     getEnv("DB_USER", "ezhealth_user"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBSSLMode:  getEnv("DB_SSL", "disable"),

		// Security
		JWTSecret:     getEnv("JWT_SECRET", "dev-jwt-secret"),
		SessionSecret: getEnv("SESSION_SECRET", "dev-session-secret"),
		EncryptionKey: getEnv("ENCRYPTION_KEY", "dev-encryption-key"),
	}

	// Set FHIR schema directory relative to main schema directory
	cfg.FHIRSchemaDir = getEnv("FHIR_SCHEMA_DIR", filepath.Join(cfg.SchemaDirectory, "fhir"))

	return cfg
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getStringSlice(key, defaultValue string) []string {
	value := getEnv(key, defaultValue)
	return strings.Split(value, ",")
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	return c.Environment == "production"
}

// UseFilesystemSchema returns true if using ezHealthKonnect Schema System
func (c *Config) UseFilesystemSchema() bool {
	return c.SchemaSource == "filesystem"
}

// UseLegacyDictionary returns true if using legacy HTTP dictionary service
func (c *Config) UseLegacyDictionary() bool {
	return c.SchemaSource == "legacy_http"
}

// GetSchemaDirectory returns the directory path for ezHealthKonnect schemas
func (c *Config) GetSchemaDirectory() string {
	return c.SchemaDirectory
}

// GetFHIRSchemaDirectory returns the FHIR schema directory path
func (c *Config) GetFHIRSchemaDirectory() string {
	return c.FHIRSchemaDir
}

// GetDictionaryURL returns the full dictionary service URL (legacy support)
func (c *Config) GetDictionaryURL() string {
	return c.DictionaryServiceURL + "/api/v1/hl7/enhance"
}

// GetSchemaConfig returns schema-related configuration for logging
func (c *Config) GetSchemaConfig() map[string]interface{} {
	return map[string]interface{}{
		"schemaSource":      c.SchemaSource,
		"schemaDirectory":   c.SchemaDirectory,
		"enableCache":       c.EnableSchemaCache,
		"cacheSize":         c.SchemaCacheSize,
		"defaultHL7Version": c.DefaultHL7Version,
	}
}

// GetFHIRConfig returns FHIR-related configuration for logging/status
func (c *Config) GetFHIRConfig() map[string]interface{} {
	return map[string]interface{}{
		"fhirSchemaDir":      c.FHIRSchemaDir,
		"defaultFhirVersion": c.DefaultFHIRVersion,
		"defaultFhirProfile": c.DefaultFHIRProfile,
		"enableFhirCache":    c.EnableFHIRCache,
		"fhirCacheSize":      c.FHIRCacheSize,
	}
}

// ValidateSchemaConfig validates both HL7 and FHIR schema configuration - SILENT
func (c *Config) ValidateSchemaConfig() {
	if c.UseFilesystemSchema() {
		// Validate main schema directory - SILENT unless verbose
		if _, err := os.Stat(c.SchemaDirectory); os.IsNotExist(err) {
			if c.VerboseLogging {
				log.Printf("⚠️ WARNING: Schema directory does not exist: %s", c.SchemaDirectory)
				log.Printf("   Create directory: mkdir -p %s", c.SchemaDirectory)
				log.Printf("   Or set EZHEALTHKONNECT_SCHEMA_DIR environment variable")
			}
		}
		// Silent success - no logging when directory exists

		// Validate HL7 and FHIR schemas - now silent
		c.validateHL7Schemas()
		c.validateFHIRSchemas()
	}

	if c.UseLegacyDictionary() && c.VerboseLogging {
		log.Printf("📡 Using legacy HTTP dictionary service: %s", c.GetDictionaryURL())
		log.Printf("   Consider migrating to ezHealthKonnect Schema System for better performance")
	}
}

// validateHL7Schemas validates HL7 schema configuration - SILENT
func (c *Config) validateHL7Schemas() {
	hl7SchemaDir := filepath.Join(c.SchemaDirectory, "hl7")

	entries, err := os.ReadDir(hl7SchemaDir)
	if err != nil {
		// Only log if verbose logging is enabled
		if c.VerboseLogging {
			log.Printf("⚠️ WARNING: HL7 schema directory not found: %s", hl7SchemaDir)
			log.Printf("   Create directory: mkdir -p %s", hl7SchemaDir)
			log.Printf("   Add your HL7 schema files (.gz) to version subdirectories")
		}
		return
	}

	schemaCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			versionDir := filepath.Join(hl7SchemaDir, entry.Name())
			if versionFiles, err := filepath.Glob(filepath.Join(versionDir, "*.gz")); err == nil {
				schemaCount += len(versionFiles)
			}
		} else if strings.HasSuffix(entry.Name(), ".gz") || strings.HasSuffix(entry.Name(), ".json") {
			schemaCount++
		}
	}

	if schemaCount == 0 && c.VerboseLogging {
		log.Printf("⚠️ WARNING: No HL7 schema files found in %s", hl7SchemaDir)
		log.Printf("   Add your compressed HL7 schema files (.gz) to version subdirectories")
		log.Printf("   Expected structure: %s/v2.5/*.gz", hl7SchemaDir)
	}
	// Silent success - no logging when schemas are found
}

// validateFHIRSchemas validates FHIR schema configuration - SILENT
func (c *Config) validateFHIRSchemas() {
	if _, err := os.Stat(c.FHIRSchemaDir); os.IsNotExist(err) {
		// Only log warnings if verbose logging is enabled
		if c.VerboseLogging {
			log.Printf("⚠️ WARNING: FHIR schema directory does not exist: %s", c.FHIRSchemaDir)
			log.Printf("   Create directory structure:")
			log.Printf("     mkdir -p %s/R4/resources", c.FHIRSchemaDir)
			log.Printf("     mkdir -p %s/R4/profiles/us-core", c.FHIRSchemaDir)
		}
		return
	}

	// Silent when directory exists
	r4ResourcesDir := filepath.Join(c.FHIRSchemaDir, "R4", "resources")
	r4ProfilesDir := filepath.Join(c.FHIRSchemaDir, "R4", "profiles", "us-core")

	resourceCount := 0
	profileCount := 0

	if resourceFiles, err := filepath.Glob(filepath.Join(r4ResourcesDir, "*.gz")); err == nil {
		resourceCount = len(resourceFiles)
	}

	if profileFiles, err := filepath.Glob(filepath.Join(r4ProfilesDir, "*.gz")); err == nil {
		profileCount = len(profileFiles)
	}

	if resourceCount == 0 && profileCount == 0 && c.VerboseLogging {
		log.Printf("⚠️ WARNING: No FHIR schema files found in %s", c.FHIRSchemaDir)
		log.Printf("   Add your FHIR schema files (.gz) to resources/ and profiles/us-core/")
	}
	// Silent success - no logging about found schema counts
}

// LogConfiguration logs both HL7 and FHIR configuration - ONLY IF VERBOSE
func (c *Config) LogConfiguration() {
	if c.VerboseLogging {
		log.Println("🔧 ezHealthKonnect Configuration:")
		log.Printf("   Environment: %s", c.Environment)
		log.Printf("   Port: %s", c.Port)
		log.Printf("   Schema Source: %s", c.SchemaSource)

		// HL7 Configuration
		log.Println("📋 HL7 Configuration:")
		if c.UseFilesystemSchema() {
			log.Printf("   Schema Directory: %s", c.SchemaDirectory)
			log.Printf("   Schema Cache: %v (size: %d)", c.EnableSchemaCache, c.SchemaCacheSize)
		}
		log.Printf("   HL7 Processing Mode: %s", c.HL7ProcessingMode)
		log.Printf("   HL7 Validation Level: %s", c.HL7ValidationLevel)
		log.Printf("   Default HL7 Version: %s", c.DefaultHL7Version)

		// FHIR Configuration
		log.Println("🏥 FHIR Configuration:")
		log.Printf("   FHIR Schema Directory: %s", c.FHIRSchemaDir)
		log.Printf("   Default FHIR Version: %s", c.DefaultFHIRVersion)
		log.Printf("   Default FHIR Profile: %s", c.DefaultFHIRProfile)
		log.Printf("   FHIR Cache: %v (size: %d)", c.EnableFHIRCache, c.FHIRCacheSize)

		// System Configuration
		log.Println("⚙️ System Configuration:")
		log.Printf("   HIPAA Compliance: %v", c.HIPAAComplianceMode)
		log.Printf("   Verbose Logging: %v", c.VerboseLogging)
	}
	// Silent when VerboseLogging is false
}

// GetDatabaseURL constructs and returns the database connection string
func (c *Config) GetDatabaseURL() string {
	// First check for DATABASE_URL environment variable
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		return dbURL
	}

	// If no DATABASE_URL, construct from individual components
	if c.DBHost != "" && c.DBName != "" && c.DBUser != "" {
		sslMode := normalizeSSLMode(c.DBSSLMode)

		port := c.DBPort
		if port == "" {
			port = "5432"
		}

		return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
			c.DBHost, port, c.DBUser, c.DBPassword, c.DBName, sslMode)
	}

	// No database configuration found
	return ""
}

// normalizeSSLMode converts common boolean-like values to valid lib/pq sslmode values
// "false", "0", "no", "disable", "disabled" -> "disable"
// "true", "1", "yes", "enable", "enabled", "require" -> "require"
func normalizeSSLMode(input string) string {
	v := strings.ToLower(strings.TrimSpace(input))
	switch v {
	case "", "false", "0", "no", "disable", "disabled":
		return "disable"
	case "true", "1", "yes", "enable", "enabled", "require":
		return "require"
	default:
		return v
	}
}

// HasDatabaseConfig returns true if database configuration is available
func (c *Config) HasDatabaseConfig() bool {
	return c.GetDatabaseURL() != ""
}
