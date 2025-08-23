package main

import (
	"database/sql"
	"ezhealthkonnect/config"
	"ezhealthkonnect/controllers"
	"ezhealthkonnect/fhir"
	"ezhealthkonnect/hl7"
	"ezhealthkonnect/services"
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

	// Log configuration for debugging - only if verbose mode
	if cfg.VerboseLogging {
		cfg.LogConfiguration()
	}

	// Database connection for FHIR transformations
	var db *sql.DB
	var err error

	// Use centralized config to build connection string (supports defaults and normalization)
	dbConnectionString := cfg.GetDatabaseURL()
	if dbConnectionString == "" {
		log.Printf("⚠️ WARNING: No database configuration found; DB-backed transformations will be disabled")
	} else {
		db, err = sql.Open("postgres", dbConnectionString)
		if err != nil {
			log.Printf("❌ ERROR: Failed to connect to database for FHIR transformations: %v", err)
		} else if err = db.Ping(); err != nil {
			log.Printf("❌ ERROR: Database ping failed: %v", err)
			db = nil
		}
		// Silent success - no logging when database connects properly
	}

	// Initialize schema systems if schemas are available - ENHANCED
	if cfg.UseFilesystemSchema() {
		// Initialize HL7 Schema System - SILENT
		hl7SchemaPath := filepath.Join(cfg.GetSchemaDirectory(), "hl7")
		hl7.InitSchemaLoader(hl7SchemaPath)

		// Configure HL7 schema loader - SILENT
		schemaLoader := hl7.GetSchemaLoader()
		if schemaLoader != nil {
			schemaLoader.SetMaxCacheSize(cfg.SchemaCacheSize)
			// Silent success - no logging
		} else {
			log.Printf("❌ ERROR: Failed to initialize HL7 schema loader")
		}

		// Initialize FHIR Schema System - ENHANCED WITH FAIL-FAST
		fhirSchemaDir := cfg.GetFHIRSchemaDirectory()
		log.Printf("🔧 Initializing FHIR schema system from: %s", fhirSchemaDir)

		fhir.InitFHIRSchemaLoader(fhirSchemaDir)

		// Verify FHIR schema loader initialization - ENHANCED
		fhirLoader := fhir.GetFHIRSchemaLoader()
		if fhirLoader == nil {
			log.Printf("❌ FATAL: FHIR schema loader failed to initialize")
			log.Printf("💡 SOLUTION: Download FHIR packages to %s", fhirSchemaDir)
			log.Printf("💡 Expected structure:")
			log.Printf("   %s/R4/resources/Patient.gz", fhirSchemaDir)
			log.Printf("   %s/R4/resources/Organization.gz", fhirSchemaDir)
			log.Printf("   %s/R4/profiles/us-core/Patient.gz", fhirSchemaDir)
			log.Printf("🚨 Server will start but FHIR transformations will be limited")
		} else {
			// Check available schemas
			if available, err := fhirLoader.ListAvailableSchemas(); err == nil {
				if len(available) > 0 {
					log.Printf("✅ FHIR schema system initialized with %d schemas", len(available))
					if cfg.VerboseLogging {
						log.Printf("📋 Available FHIR schemas: %v", available[:min(5, len(available))])
					}
				} else {
					log.Printf("⚠️ WARNING: FHIR schema loader initialized but no schemas found")
					log.Printf("💡 Check that .gz files exist in expected directories")
				}
			} else {
				log.Printf("❌ ERROR: Cannot list FHIR schemas: %v", err)
			}
		}
	} else {
		log.Printf("📋 Using legacy HTTP-based schema system")
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

	// Health check endpoint with both HL7 and FHIR status - ENHANCED
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

		// Database status
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

			// FHIR Schema System Status - ENHANCED
			fhirLoader := fhir.GetFHIRSchemaLoader()
			if fhirLoader != nil {
				fhirStats := fhirLoader.GetStats()
				available, err := fhirLoader.ListAvailableSchemas()

				fhirStatus := gin.H{
					"enabled":     true,
					"source":      "filesystem",
					"directory":   cfg.GetFHIRSchemaDirectory(),
					"cacheStats":  fhirStats,
					"initialized": true,
				}

				if err == nil {
					fhirStatus["availableSchemas"] = len(available)
					fhirStatus["schemasReady"] = len(available) > 0
					if len(available) > 0 {
						fhirStatus["status"] = "operational"
						fhirStatus["sampleSchemas"] = available[:min(3, len(available))]
					} else {
						fhirStatus["status"] = "no_schemas"
						fhirStatus["message"] = "Loader initialized but no schema files found"
					}
				} else {
					fhirStatus["status"] = "error"
					fhirStatus["error"] = err.Error()
				}

				health["fhirSchemaSystem"] = fhirStatus
			} else {
				health["fhirSchemaSystem"] = gin.H{
					"enabled":   false,
					"status":    "failed_to_initialize",
					"directory": cfg.GetFHIRSchemaDirectory(),
					"message":   "FHIR schema loader not initialized - check directory structure",
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
		// HL7 ROUTES
		hl7Group := api.Group("/hl7")
		hl7Ctrl := controllers.NewHL7Controller(cfg)
		{
			hl7Group.POST("/parse", hl7Ctrl.ParseMessage)
			hl7Group.POST("/validate", hl7Ctrl.ValidateMessage)
			hl7Group.GET("/stats", hl7Ctrl.GetStats)
		}

		// SYSTEM ROUTES
		systemCtrl := controllers.NewSystemController(cfg)
		systemGroup := api.Group("/system")
		{
			systemGroup.GET("/health", systemCtrl.HealthCheck)
			systemGroup.GET("/info", systemCtrl.GetInfo)
			systemGroup.GET("/metrics", systemCtrl.GetMetrics)
		}

		// FHIR ROUTES
		fhirResourceService := services.NewMessageResourceIdentifierService(db)
		fhirResourceController := controllers.NewFHIRResourceController(fhirResourceService)

		fhirGroup := api.Group("/fhir")
		{
			// EXISTING: Primary HL7→FHIR transformation using existing controller
			newFHIRController := controllers.NewHL7FHIRTransformationController(db, cfg)
			newFHIRController.RegisterRoutes(api) // This adds /api/fhir/transform routes

			// EXISTING: Resource identification (keep this)
			fhirResourceController.RegisterRoutes(api)

			// EXISTING: Schema-driven transformation (keep as alternative/advanced)
			schemaFHIRCtrl := controllers.NewSchemaFHIRTransformController(db, cfg)
			schemaFHIRGroup := fhirGroup.Group("/schema") // Move to /api/fhir/schema/
			schemaFHIRCtrl.RegisterRoutes(schemaFHIRGroup)

			// ADDED: Schema listing endpoint
			fhirGroup.GET("/transform/schemas", func(c *gin.Context) {
				fhirLoader := fhir.GetFHIRSchemaLoader()
				if fhirLoader == nil {
					c.JSON(http.StatusServiceUnavailable, gin.H{
						"success": false,
						"error":   "FHIR schema loader not initialized",
					})
					return
				}

				available, err := fhirLoader.ListAvailableSchemas()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"success": false,
						"error":   "Failed to list schemas",
						"details": err.Error(),
					})
					return
				}

				// Parse and structure schema information
				schemas := make([]map[string]interface{}, 0, len(available))
				versionCount := make(map[string]int)
				profileCount := make(map[string]int)

				for _, schemaID := range available {
					parts := strings.Split(schemaID, "_")
					if len(parts) >= 3 {
						version := parts[0]
						profile := parts[1]
						resourceType := parts[2]

						versionCount[version]++
						profileCount[profile]++

						schemas = append(schemas, map[string]interface{}{
							"id":           schemaID,
							"version":      version,
							"profile":      profile,
							"resourceType": resourceType,
							"displayName":  fmt.Sprintf("%s %s (%s)", resourceType, profile, version),
						})
					}
				}

				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"schemas": schemas,
					"summary": gin.H{
						"total":    len(schemas),
						"versions": versionCount,
						"profiles": profileCount,
					},
					"loader": gin.H{
						"status": "operational",
						"stats":  fhirLoader.GetStats(),
					},
				})
			})

			// ADDED: Individual schema endpoint
			fhirGroup.GET("/transform/schemas/:resourceType", func(c *gin.Context) {
				resourceType := c.Param("resourceType")
				profile := c.DefaultQuery("profile", "base")
				version := c.DefaultQuery("version", "R4")

				fhirLoader := fhir.GetFHIRSchemaLoader()
				if fhirLoader == nil {
					c.JSON(http.StatusServiceUnavailable, gin.H{
						"success": false,
						"error":   "FHIR schema loader not initialized",
					})
					return
				}

				schema, err := fhirLoader.LoadFHIRSchema(resourceType, profile, version)
				if err != nil {
					c.JSON(http.StatusNotFound, gin.H{
						"success":      false,
						"error":        "Schema not found",
						"resourceType": resourceType,
						"profile":      profile,
						"version":      version,
						"details":      err.Error(),
						"suggestion":   "Try with profile=base or check available schemas at /api/fhir/transform/schemas",
					})
					return
				}

				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"schema":  schema,
					"info": gin.H{
						"resourceType": resourceType,
						"profile":      profile,
						"version":      version,
						"elements":     len(schema.Elements),
						"required":     len(schema.Required),
						"mustSupport":  len(schema.MustSupport),
						"loadTime":     schema.LoadTime.String(),
					},
				})
			})

			// Add this to your existing fhirGroup in main.go:

			// TEST ENDPOINT - USE V3 SCHEMA-DRIVEN SERVICE
			fhirGroup.POST("/test-transform-v3", func(c *gin.Context) {
				log.Printf("🧪 TEST: Schema-driven V3 service endpoint hit!")

				// Parse request
				var request services.TransformRequest
				if err := c.ShouldBindJSON(&request); err != nil {
					log.Printf("🧪 TEST: JSON binding failed: %v", err)
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				log.Printf("🧪 TEST: Creating V3 Schema-Driven service instance...")

				// CREATE V3 SERVICE INSTANCE - TRUE SCHEMA-DRIVEN!
				transformServiceV3 := services.NewHL7FHIRTransformServiceV3(db)

				log.Printf("🧪 TEST: Calling V3 Transform method (schema-driven)...")

				// Call V3 service method
				response, err := transformServiceV3.Transform(c.Request.Context(), &request)
				if err != nil {
					log.Printf("🧪 TEST: V3 Transform failed: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}

				log.Printf("🧪 TEST: V3 Transform completed, returning response")
				c.JSON(http.StatusOK, response)
			})

			// DEBUG ENDPOINT - FIXED TO LOOK FOR .GZ FILES IN CORRECT PATHS
			fhirGroup.GET("/debug-schema", func(c *gin.Context) {
				debug := gin.H{}

				// Check what FHIR loader you actually have
				fhirLoader := fhir.GetFHIRSchemaLoader()
				if fhirLoader == nil {
					debug["fhirLoader"] = "NOT AVAILABLE"
				} else {
					debug["fhirLoader"] = "AVAILABLE"
					debug["loaderType"] = fmt.Sprintf("%T", fhirLoader)

					// Get stats
					stats := fhirLoader.GetStats()
					debug["fhirStats"] = stats
				}

				// Check FHIR directory
				fhirDir := cfg.GetFHIRSchemaDirectory()
				debug["fhirDirectory"] = fhirDir

				// FIXED: Check for .gz files in version-specific directories
				var allSchemaFiles []string
				schemaCount := 0

				// Check R4 resources directory
				r4ResourcesDir := filepath.Join(fhirDir, "R4", "resources")
				if r4Files, err := filepath.Glob(filepath.Join(r4ResourcesDir, "*.gz")); err == nil {
					allSchemaFiles = append(allSchemaFiles, r4Files...)
					schemaCount += len(r4Files)
					debug["r4ResourcesFound"] = len(r4Files)
				} else {
					debug["r4ResourcesError"] = err.Error()
				}

				// Check R4 US Core profiles directory
				r4ProfilesDir := filepath.Join(fhirDir, "R4", "profiles", "us-core")
				if r4ProfileFiles, err := filepath.Glob(filepath.Join(r4ProfilesDir, "*.gz")); err == nil {
					allSchemaFiles = append(allSchemaFiles, r4ProfileFiles...)
					schemaCount += len(r4ProfileFiles)
					debug["r4ProfilesFound"] = len(r4ProfileFiles)
				} else {
					debug["r4ProfilesError"] = err.Error()
				}

				debug["schemaFiles"] = schemaCount
				if len(allSchemaFiles) > 0 {
					// Show first 5 files (basenames only)
					maxFiles := len(allSchemaFiles)
					if maxFiles > 5 {
						maxFiles = 5
					}
					sampleFiles := make([]string, maxFiles)
					for i := 0; i < maxFiles; i++ {
						sampleFiles[i] = filepath.Base(allSchemaFiles[i])
					}
					debug["sampleFiles"] = sampleFiles
				}

				// Check if directories exist
				if stat, err := os.Stat(fhirDir); err != nil {
					debug["directoryError"] = err.Error()
					debug["directoryExists"] = false
				} else {
					debug["directoryExists"] = true
					debug["directoryMode"] = stat.Mode().String()
				}

				c.JSON(http.StatusOK, debug)
			})

			// TEST ENDPOINT - USE V2 SERVICE DIRECTLY
			fhirGroup.POST("/test-transform", func(c *gin.Context) {
				log.Printf("🧪 TEST: Direct V2 service call endpoint hit!")

				// Parse request
				var request services.TransformRequest
				if err := c.ShouldBindJSON(&request); err != nil {
					log.Printf("🧪 TEST: JSON binding failed: %v", err)
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				log.Printf("🧪 TEST: Creating V2 service instance...")

				// CREATE V2 SERVICE INSTANCE - THIS IS THE KEY!
				transformService := services.NewHL7FHIRTransformServiceV2(db)

				log.Printf("🧪 TEST: Calling V2 Transform method...")

				// Call your V2 service method directly
				response, err := transformService.Transform(c.Request.Context(), &request)
				if err != nil {
					log.Printf("🧪 TEST: V2 Transform failed: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}

				log.Printf("🧪 TEST: V2 Transform completed, returning response")
				c.JSON(http.StatusOK, response)
			})
		}

		// INTERFACE ROUTES
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

		// WIZARD API CONTROLLER
		wizardCtrl := controllers.NewWizardController(cfg)
		wizardCtrl.RegisterRoutes(api)
		// Silent success - no logging

		// HL7 Schema management routes
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

		// Transformation Routes
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

	// Start server - SILENT (no startup logging unless verbose)
	if cfg.VerboseLogging {
		fmt.Printf("🚀 Starting ezHealthKonnect Server on port %s\n", port)
		fmt.Printf("🌐 Server URL: http://localhost:%s\n", port)
		fmt.Printf("📊 API Health: http://localhost:%s/api/system/health\n", port)
		fmt.Printf("🔧 System Info: http://localhost:%s/api/system/info\n", port)
	}

	log.Printf("Starting ezHealthKonnect server on :%s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

// Helper function for min calculation
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// setupWizardAPI sets up additional HTTP handlers for the wizard API
func setupWizardAPI() {
	// Silent setup - only log if there are errors
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
