package main

import (
	"database/sql"
	"ezhealthkonnect/config"
	"ezhealthkonnect/controllers"
	"ezhealthkonnect/fhir"
	"ezhealthkonnect/hl7"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

// Track server start time for uptime calculation
var startTime time.Time

func main() {
	// Record start time
	startTime = time.Now()

	// Load configuration
	cfg := config.Load()

	// Log configuration for debugging
	cfg.LogConfiguration()

	// Validate configuration first
	cfg.ValidateSchemaConfig()

	// ADD: Database connection for FHIR transformations
	var db *sql.DB
	var err error

	dbConnectionString := os.Getenv("DATABASE_URL")
	if dbConnectionString == "" {
		dbConnectionString = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME"))
	}

	if dbConnectionString != "" && dbConnectionString != "host= port= user= password= dbname= sslmode=disable" {
		db, err = sql.Open("postgres", dbConnectionString)
		if err != nil {
			log.Printf("⚠️ Failed to connect to database for FHIR transformations: %v", err)
		} else if err = db.Ping(); err != nil {
			log.Printf("⚠️ Database ping failed: %v", err)
			db = nil
		} else {
			log.Printf("✅ Database connected for FHIR transformations")
		}
	} else {
		log.Printf("💡 No database configured for FHIR transformations (set DATABASE_URL to enable)")
	}

	// Initialize schema systems if schemas are available
	if cfg.UseFilesystemSchema() {
		fmt.Printf("🚀 Initializing ezHealthKonnect Schema Systems...\n")

		// Initialize HL7 Schema System
		fmt.Printf("📋 Initializing HL7 Schema System...\n")
		hl7SchemaPath := filepath.Join(cfg.GetSchemaDirectory(), "hl7")
		fmt.Printf("🔍 HL7 Schema Path: %s\n", hl7SchemaPath)
		hl7.InitSchemaLoader(hl7SchemaPath)

		// Configure HL7 schema loader
		schemaLoader := hl7.GetSchemaLoader()
		if schemaLoader != nil {
			schemaLoader.SetMaxCacheSize(cfg.SchemaCacheSize)
			fmt.Printf("✅ HL7 Schema System ready (cache: %v, size: %d)\n",
				cfg.EnableSchemaCache, cfg.SchemaCacheSize)
		} else {
			fmt.Printf("⚠️ Failed to initialize HL7 schema loader\n")
		}

		// Initialize FHIR Schema System
		fmt.Printf("🏥 Initializing FHIR Schema System...\n")
		fhir.InitFHIRSchemaLoader(cfg.GetFHIRSchemaDirectory())

		// Verify FHIR schema loader
		fhirLoader := fhir.GetFHIRSchemaLoader()
		if fhirLoader != nil {
			fmt.Printf("✅ FHIR Schema System ready (cache: %v, size: %d)\n",
				cfg.EnableFHIRCache, cfg.FHIRCacheSize)
		} else {
			fmt.Printf("⚠️ Failed to initialize FHIR schema loader\n")
		}

		fmt.Printf("🎉 ezHealthKonnect Schema Systems initialized successfully!\n")
	} else {
		fmt.Printf("📡 Using legacy HTTP dictionary service: %s\n", cfg.GetDictionaryURL())
		fmt.Printf("💡 Tip: Add schema files to %s to enable enhanced performance\n", cfg.GetSchemaDirectory())
	}

	// Initialize Gin router
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// Configure CORS
	corsConfig := cors.Config{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	router.Use(cors.New(corsConfig))

	// Static files
	router.Static("/static", "./public")
	router.StaticFile("/", "./public/index.html")

	// ENHANCED: Health check endpoint with both HL7 and FHIR status
	router.GET("/health", func(c *gin.Context) {
		health := gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Format(time.RFC3339),
			"version":   "1.0.0",
			"service":   "ezHealthKonnect API",
			"uptime":    time.Since(startTime).String(),
			"features": gin.H{
				"hl7_processing":  true,
				"fhir_support":    cfg.UseFilesystemSchema(),
				"schema_system":   cfg.UseFilesystemSchema(),
				"transformations": true,
				"database":        db != nil,
			},
		}

		// ADD: Database status
		if db != nil {
			health["database"] = gin.H{
				"connected": true,
				"status":    "operational",
			}
		} else {
			health["database"] = gin.H{
				"connected": false,
				"status":    "not configured",
				"note":      "Set DATABASE_URL to enable FHIR transformations",
			}
		}

		// Add schema systems status
		if cfg.UseFilesystemSchema() {
			// HL7 Schema System Status
			schemaLoader := hl7.GetSchemaLoader()
			if schemaLoader != nil {
				stats := schemaLoader.GetCacheStats()
				health["hl7SchemaSystem"] = gin.H{
					"enabled":    true,
					"source":     "filesystem",
					"directory":  cfg.GetSchemaDirectory(),
					"cacheStats": stats,
				}
			}

			// FHIR Schema System Status
			fhirLoader := fhir.GetFHIRSchemaLoader()
			if fhirLoader != nil {
				fhirStats := fhirLoader.GetStats()
				health["fhirSchemaSystem"] = gin.H{
					"enabled":    true,
					"source":     "filesystem",
					"directory":  cfg.GetFHIRSchemaDirectory(),
					"cacheStats": fhirStats,
				}
			}
		} else {
			health["schemaSystem"] = gin.H{
				"enabled": false,
				"source":  "legacy_http",
				"url":     cfg.GetDictionaryURL(),
			}
		}

		c.JSON(http.StatusOK, health)
	})

	// API routes
	api := router.Group("/api")
	{
		// =====================================
		// HL7 ROUTES (Existing - Maintained)
		// =====================================
		hl7Group := api.Group("/hl7")
		hl7Ctrl := controllers.NewHL7Controller(cfg)
		{
			hl7Group.POST("/parse", hl7Ctrl.ParseMessage)
			hl7Group.POST("/validate", hl7Ctrl.ValidateMessage)
			hl7Group.GET("/stats", hl7Ctrl.GetStats)
		}

		// =====================================
		// SYSTEM ROUTES (Fixed controller signature)
		// =====================================
		systemCtrl := controllers.NewSystemController(cfg)
		systemGroup := api.Group("/system")
		{
			systemGroup.GET("/health", systemCtrl.HealthCheck)
			systemGroup.GET("/info", systemCtrl.GetInfo)
			systemGroup.GET("/metrics", systemCtrl.GetMetrics)
		}

		// =====================================
		// FHIR ROUTES (Your existing code - keep as is)
		// =====================================
		fhirGroup := api.Group("/fhir")
		fhirCtrl := controllers.NewSchemaFHIRTransformController(db, cfg)
		//fhirCtrl := controllers.NewFHIRTransformController(db, cfg)
		fhirCtrl.RegisterRoutes(fhirGroup)

		// =====================================
		// INTERFACE ROUTES (Fixed controller signature)
		// =====================================
		interfaceCtrl := controllers.NewInterfaceController(cfg)
		interfaceGroup := api.Group("/interfaces")
		{
			interfaceGroup.POST("", interfaceCtrl.CreateInterface)
			interfaceGroup.GET("", interfaceCtrl.GetInterfaces)
			interfaceGroup.GET("/:id", interfaceCtrl.GetInterface)
			interfaceGroup.PUT("/:id", interfaceCtrl.UpdateInterface)
			interfaceGroup.DELETE("/:id", interfaceCtrl.DeleteInterface)
			interfaceGroup.POST("/:id/start", interfaceCtrl.StartInterface)
			interfaceGroup.POST("/:id/stop", interfaceCtrl.StopInterface)
			interfaceGroup.POST("/:id/pause", interfaceCtrl.PauseInterface)
		}

		// =====================================
		// WIZARD API CONTROLLER (Your integration - MOVED INSIDE API GROUP)
		// =====================================
		wizardCtrl := controllers.NewWizardController(cfg)
		wizardCtrl.RegisterRoutes(api)
		log.Printf("✅ Wizard API routes registered")

		// HL7 Schema management routes (existing - maintained)
		if cfg.UseFilesystemSchema() {
			schemaGroup := api.Group("/schema")
			{
				schemaGroup.GET("/status", func(c *gin.Context) {
					schemaLoader := hl7.GetSchemaLoader()
					if schemaLoader == nil {
						c.JSON(http.StatusServiceUnavailable, gin.H{
							"success": false,
							"error":   "HL7 schema system not initialized",
							"system":  "hl7",
						})
						return
					}

					stats := schemaLoader.GetCacheStats()
					available, err := schemaLoader.ListAvailableSchemas()
					if err != nil {
						available = []string{} // Empty slice if error
					}

					c.JSON(http.StatusOK, gin.H{
						"success": true,
						"system": gin.H{
							"name":           "ezHealthKonnect HL7 Schema System",
							"source":         "filesystem",
							"directory":      cfg.GetSchemaDirectory(),
							"cacheEnabled":   cfg.EnableSchemaCache,
							"maxCacheSize":   cfg.SchemaCacheSize,
							"defaultVersion": cfg.DefaultHL7Version,
						},
						"statistics": stats,
						"available":  available,
					})
				})

				schemaGroup.GET("/available", func(c *gin.Context) {
					schemaLoader := hl7.GetSchemaLoader()
					if schemaLoader == nil {
						c.JSON(http.StatusServiceUnavailable, gin.H{
							"success": false,
							"error":   "HL7 schema system not initialized",
						})
						return
					}

					available, err := schemaLoader.ListAvailableSchemas()
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{
							"success": false,
							"error":   fmt.Sprintf("Failed to list HL7 schemas: %v", err),
						})
						return
					}

					c.JSON(http.StatusOK, gin.H{
						"success": true,
						"schemas": available,
						"count":   len(available),
						"system":  "hl7",
					})
				})

				schemaGroup.POST("/cache/clear", func(c *gin.Context) {
					schemaLoader := hl7.GetSchemaLoader()
					if schemaLoader == nil {
						c.JSON(http.StatusServiceUnavailable, gin.H{
							"success": false,
							"error":   "HL7 schema system not initialized",
						})
						return
					}

					schemaLoader.ClearCache()
					c.JSON(http.StatusOK, gin.H{
						"success": true,
						"message": "HL7 schema cache cleared",
						"system":  "hl7",
					})
				})
			}
		}

		// Transformation Routes (Future HL7→FHIR)
		transformGroup := api.Group("/transform")
		{
			transformGroup.POST("/hl7-to-fhir", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": "HL7 to FHIR transformation endpoint (to be implemented)",
					"note":    "This will use both HL7 and FHIR schema systems for transformation",
				})
			})

			transformGroup.GET("/status", func(c *gin.Context) {
				hl7Ready := false
				fhirReady := false

				if cfg.UseFilesystemSchema() {
					hl7Ready = hl7.GetSchemaLoader() != nil
					fhirReady = fhir.GetFHIRSchemaLoader() != nil
				}

				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"status": gin.H{
						"hl7SystemReady":     hl7Ready,
						"fhirSystemReady":    fhirReady,
						"transformReady":     hl7Ready && fhirReady,
						"databaseReady":      db != nil,
						"fhirTransformReady": db != nil,
					},
					"message": "Transformation system status",
				})
			})
		}
	}

	// Setup additional HTTP handlers for wizard API
	setupWizardAPI()

	// Start server
	port := cfg.Port
	if port == "" {
		port = "8080"
	}

	fmt.Printf("🚀 Starting ezHealthKonnect Server on port %s\n", port)
	fmt.Printf("🌐 Server URL: http://localhost:%s\n", port)
	fmt.Printf("📊 API Health: http://localhost:%s/api/system/health\n", port)
	fmt.Printf("🔧 System Info: http://localhost:%s/api/system/info\n", port)

	// Start server
	log.Printf("Starting ezHealthKonnect server on :%s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

// setupWizardAPI sets up additional HTTP handlers for the wizard API
func setupWizardAPI() {
	log.Printf("🔧 Setting up additional wizard API handlers")

	// Add wizard-specific HTTP handlers here if needed
	http.HandleFunc("/api/wizard/health", corsHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "service": "wizard"}`))
	}))
}

// corsHandler adds CORS headers to HTTP responses
func corsHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// handleMappingRules handles mapping rules requests
func handleMappingRules(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract message type from URL path
	urlParts := strings.Split(r.URL.Path, "/")
	var messageType string
	if len(urlParts) >= 6 {
		messageType = urlParts[5]
	}

	switch r.Method {
	case "GET":
		// Return sample mapping rules
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{
			"success": true,
			"rules": [
				{
					"id": "rule1",
					"source_field": "MSH.3",
					"target_field": "MessageHeader.source.name",
					"transformation_type": "direct",
					"enabled": true,
					"mapped": true
				}
			],
			"metadata": {
				"messageType": "%s",
				"count": 1
			},
			"lastUpdated": "%s"
		}`, messageType, time.Now().Format(time.RFC3339))))

	case "PUT":
		// Handle saving mapping rules
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"success": true,
			"message": "Mapping rules saved successfully",
			"saved": 1
		}`))

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte(`{"error": "Method not allowed"}`))
	}
}
