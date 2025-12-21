package main

import (
	"database/sql"
	"ezhealthkonnect/config"
	"ezhealthkonnect/controllers"
	"ezhealthkonnect/fhir"
	"ezhealthkonnect/hl7"
	"ezhealthkonnect/processing"
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

// Global PostgreSQL transformation service
var postgresTransformationService *services.PostgresTransformationService

// Global Processing Engine
var processingEngine *processing.ProcessingEngine

func main() {
	// Record start time
	startTime = time.Now()

	// Load configuration
	cfg := config.Load()

	// Log configuration for debugging - only if verbose mode
	if cfg.VerboseLogging {
		cfg.LogConfiguration()
	}

	// Database connection for FHIR transformations with retry logic
	var db *sql.DB
	var err error

	// Use centralized config to build connection string (supports defaults and normalization)
	dbConnectionString := cfg.GetDatabaseURL()
	if dbConnectionString == "" {
		log.Printf("⚠️ WARNING: No database configuration found; DB-backed transformations will be disabled")
	} else {
		// Retry logic for database connection (PostgreSQL may be starting up)
		maxRetries := 10
		retryDelay := 2 * time.Second

		for i := 0; i < maxRetries; i++ {
			db, err = sql.Open("postgres", dbConnectionString)
			if err != nil {
				log.Printf("❌ ERROR: Failed to connect to database for FHIR transformations (attempt %d/%d): %v", i+1, maxRetries, err)
				time.Sleep(retryDelay)
				continue
			}

			err = db.Ping()
			if err != nil {
				log.Printf("⚠️ Database ping failed (attempt %d/%d): %v - retrying...", i+1, maxRetries, err)
				db.Close()
				db = nil
				time.Sleep(retryDelay)
				continue
			}

			// Success!
			log.Printf("✅ Database connection established (attempt %d/%d)", i+1, maxRetries)
			break
		}

		if db != nil {
			// OLD MLLP System services - no longer needed with ProcessingEngine architecture
			// interfaceMessageService = services.NewInterfaceMessageService(db)
			// log.Printf("✅ InterfaceMessageService initialized")
			// outputMessageService = services.NewOutputMessageService(db, nil, "")
			// log.Printf("✅ OutputMessageService initialized")

			// Initialize PostgreSQL Transformation Service (standalone)
			postgresTransformationService = services.NewPostgresTransformationService(db)
			log.Printf("✅ PostgreSQL Transformation Service initialized")

			// Initialize Processing Engine with OOB pattern (auto-detects MongoDB)
			processingEngine = processing.NewProcessingEngine(db)
			if err := processingEngine.Start(); err != nil {
				log.Printf("❌ Failed to start Processing Engine: %v", err)
			} else {
				log.Printf("✅ Processing Engine initialized and started")
			}
		} else {
			log.Printf("❌ FATAL: Database connection failed after %d retries - transformation pipeline will not be available", maxRetries)
		}
	}

// Note: MongoDB is now auto-detected via ProcessingEngineFactory (OOB pattern)
// If MONGODB_HOST or MONGODB_URI is configured, hybrid storage is enabled automatically














	// OLD MLLP System - Replaced by ProcessingEngine architecture
	// Interface listeners are now managed by ProcessingEngine via /api/processing/interfaces/:id/activate
	// if db != nil {
	// 	log.Printf("🚀 Starting Interface Engine...")
	// 	interfaceEngine := services.NewMLLPConnectivityService(db)
	//
	// 	// Start interface listeners in background
	// 	go func() {
	// 		if err := startInterfaceListeners(db, interfaceEngine); err != nil {
	// 			log.Printf("❌ Failed to start interface listeners: %v", err)
	// 		}
	// 	}()
	//
	// 	log.Printf("✅ Interface Engine initialized")
	// }

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

			// ADDED: Transformation Pipeline Test Routes
			transformTestCtrl := controllers.NewTransformationTestController(db)
			fhirGroup.POST("/pipeline/test", transformTestCtrl.TestPipeline)
			fhirGroup.POST("/pipeline/test-api-endpoint", transformTestCtrl.TestAPIEndpoint) // Test API endpoint before configuring mapping
			fhirGroup.GET("/pipeline/:interfaceId/:messageType", transformTestCtrl.GetPipeline)
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
		// Wire up the processing engine if available
		if processingEngine != nil {
			interfaceCtrl.SetEngine(processingEngine)
			log.Printf("✅ Interface controller connected to Processing Engine")
		}
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

		// PROCESSING ENGINE CONTROLLER - NEW GO BACKEND
		processingCtrl := controllers.NewProcessingController(processingEngine)
		processingCtrl.RegisterRoutes(api)

		// MLLP INTERFACE CONTROLLER - HL7 CONNECTIVITY
		mllpCtrl := controllers.NewMLLPInterfaceController(db)
		mllpCtrl.RegisterRoutes(api)

		// CONNECTIVITY CONTROLLER - MULTI-CONNECTIVITY SUPPORT (OOB CONNECTORS)
		if db != nil {
			connectivityCtrl := controllers.NewConnectivityController(db)
			connectivityGroup := api.Group("/connectivity")
			{
				// Connectivity Types (OOB Connector Definitions)
				connectivityGroup.GET("/types", connectivityCtrl.ListConnectivityTypes)
				connectivityGroup.GET("/types/:identifier", connectivityCtrl.GetConnectivityType)
				connectivityGroup.GET("/types/category/:category", connectivityCtrl.GetConnectivityTypesByCategory)

				// Interface Connectivity Configuration
				connectivityGroup.POST("/interfaces/:interface_id", connectivityCtrl.CreateInterfaceConnectivity)
				connectivityGroup.GET("/interfaces/:interface_id", connectivityCtrl.GetInterfaceConnectivity)
				connectivityGroup.PUT("/interfaces/:interface_id", connectivityCtrl.UpdateInterfaceConnectivity)
				connectivityGroup.DELETE("/interfaces/:interface_id", connectivityCtrl.DeleteInterfaceConnectivity)

				// Connection Testing
				connectivityGroup.POST("/interfaces/:interface_id/test-source", connectivityCtrl.TestSourceConnection)
				connectivityGroup.POST("/interfaces/:interface_id/test-target", connectivityCtrl.TestTargetConnection)

				// Cron Job Management
				connectivityGroup.POST("/interfaces/:interface_id/cron/enable", connectivityCtrl.EnableCronJob)
				connectivityGroup.POST("/interfaces/:interface_id/cron/disable", connectivityCtrl.DisableCronJob)
				connectivityGroup.GET("/interfaces/:interface_id/cron/status", connectivityCtrl.GetCronJobStatus)

				// Execution Logs and Statistics
				connectivityGroup.GET("/interfaces/:interface_id/executions", connectivityCtrl.GetExecutionLogs)
				connectivityGroup.GET("/interfaces/:interface_id/executions/stats", connectivityCtrl.GetExecutionStats)

				// Cron Expression Preview Helper
				connectivityGroup.POST("/cron/preview", connectivityCtrl.PreviewCronSchedule)
			}
			log.Printf("✅ Connectivity Controller initialized (Multi-Connectivity Support)")
		}

		// OUTPUT MESSAGE CONTROLLER - TRANSFORMATION OUTPUT MANAGEMENT
		outputMsgCtrl := controllers.NewOutputMessageController(db)
		outputMsgCtrl.RegisterRoutes(api)
		log.Printf("✅ Output Message Controller initialized")

		// RESPONSE MAPPING CONTROLLER - API RESPONSE MAPPING TEMPLATES
		responseMappingCtrl := controllers.NewResponseMappingController(db)
		responseMappingCtrl.RegisterRoutes(api)
		log.Printf("✅ Response Mapping Controller initialized")

		// UNIVERSAL TRANSFORMATION CONTROLLER - NEW TRANSFORMATION ENGINE
		// TODO: Re-enable Transformation controller after fixing imports
		// transformCtrl := controllers.NewTransformationController(db)
		// transformGroup := api.Group("/transform")
		// transformCtrl.RegisterRoutes(transformGroup)

		// Configuration Engine controller removed - using PostgreSQL-only transformation system

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

	// Graceful shutdown setup (PostgreSQL-only system)
	defer func() {
		if db != nil {
			db.Close()
		}
	}()

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

// ============================================================================
// DEAD CODE - OLD MLLP SYSTEM (Lines 847-1262)
// ============================================================================
// The following functions are part of the OLD MLLP system architecture.
// They have been replaced by the ProcessingEngine architecture which is
// activated via /api/processing/interfaces/:id/activate API endpoint.
//
// DO NOT DELETE - Kept for reference during transition period.
// ============================================================================

/*
// startInterfaceListeners reads interface configurations and starts appropriate listeners
func startInterfaceListeners(db *sql.DB, mllpService *services.MLLPConnectivityService) error {
	log.Printf("🔍 Querying active interfaces from database...")

	// Query active interfaces from database
	rows, err := db.Query(`
		SELECT id, name, source_type, source_config, target_config, transformation_mapping
		FROM interfaces
		WHERE status = 'active'
		ORDER BY created_at
	`)
	if err != nil {
		return fmt.Errorf("failed to query interfaces: %v", err)
	}
	defer rows.Close()

	interfaceCount := 0
	for rows.Next() {
		log.Printf("🔍 Processing interface row...")
		var id, name, sourceType, sourceConfig, targetConfig, transformationMapping string

		err := rows.Scan(&id, &name, &sourceType, &sourceConfig, &targetConfig, &transformationMapping)
		if err != nil {
			log.Printf("❌ Error scanning interface row: %v", err)
			continue
		}

		// Parse source configuration from JSONB (returned as string by PostgreSQL driver)
		var config map[string]interface{}
		if err := json.Unmarshal([]byte(sourceConfig), &config); err != nil {
			log.Printf("❌ Error parsing source config for interface %s: %v", name, err)
			log.Printf("   Raw config: %s", sourceConfig)
			continue
		}

		// Extract port from configuration
		var port int
		switch portVal := config["port"].(type) {
		case float64:
			port = int(portVal)
		case int:
			port = portVal
		default:
			log.Printf("⚠️ No valid port configured for interface %s (got: %T)", name, config["port"])
			continue
		}

		log.Printf("🔧 Starting listener for %s (%s) on port %d", name, sourceType, port)

		// Start appropriate listener based on source type
		switch sourceType {
		case "tcp", "hl7v2":
			// Start MLLP/TCP listener
			mllpConfig := &services.MLLPConfig{
				Host:              "0.0.0.0",
				Port:              port,
				MaxConnections:    10,
				ReadTimeout:       30 * time.Second,
				WriteTimeout:      30 * time.Second,
				MaxMessageSize:    1024 * 1024, // 1MB
				EnableKeepAlive:   true,
				KeepAliveInterval: 60 * time.Second,
			}

			ctx := context.Background()
			listener, err := mllpService.StartListener(ctx, mllpConfig)
			if err != nil {
				log.Printf("❌ Failed to start TCP listener for %s: %v", name, err)
				continue
			}
			log.Printf("✅ TCP listener started for %s on port %d (ID: %s)", name, port, listener.ID)

			// Start message processing goroutine for this interface
			go processInterfaceMessages(listener, id, name, transformationMapping, targetConfig)

		case "http":
			// Start HTTP listener
			go startHttpListener(port, id, name, targetConfig, transformationMapping)
			log.Printf("✅ HTTP listener started for %s on port %d", name, port)

		default:
			log.Printf("⚠️ Unsupported source type %s for interface %s", sourceType, name)
		}

		interfaceCount++
	}

	log.Printf("✅ Interface Engine started %d interface listeners", interfaceCount)
	return nil
}

// Global interface message service
var interfaceMessageService *services.InterfaceMessageService
var outputMessageService *services.OutputMessageService

// processInterfaceMessages processes messages from an MLLP listener for a specific interface
func processInterfaceMessages(listener *services.MLLPListener, interfaceID, interfaceName, transformationMapping, targetConfig string) {
	log.Printf("🔄 Starting message processor for interface %s", interfaceName)

	for {
		select {
		case message, ok := <-listener.MessageChan:
			if !ok {
				// Channel closed, exit
				log.Printf("🔚 Message processor for interface %s stopped", interfaceName)
				return
			}

			log.Printf("📥 Processing HL7 message from %s (ID: %s)", interfaceName, message.ID)
			log.Printf("📄 Message size: %d bytes, Source: %s", message.Size, message.Source)

			// FIRST: Store message using the interface message service
			err := storeMessageViaService(message, interfaceID, interfaceName)
			if err != nil {
				log.Printf("❌ Failed to store message in database for %s: %v", interfaceName, err)
				// Continue processing even if storage fails
			} else {
				log.Printf("💾 Message stored in database table for interface %s", interfaceName)
			}

			// THEN: Process message through transformation pipeline
			err = processMessageWithTransformation(message, interfaceID, interfaceName, targetConfig)
			if err != nil {
				log.Printf("❌ Message transformation failed for %s: %v", interfaceName, err)
				// Update message status to failed
				if interfaceMessageService != nil {
					interfaceMessageService.UpdateMessageStatus(interfaceID, message.ID, "failed", 0, err.Error())
				}
			} else {
				log.Printf("✅ HL7 message processed successfully (Interface: %s, MessageID: %s)", interfaceName, message.ID)
				// Update message status to processed
				if interfaceMessageService != nil {
					interfaceMessageService.UpdateMessageStatus(interfaceID, message.ID, "processed", 0, "")
				}
			}
		}
	}
}

// storeMessageViaService stores the received message using the InterfaceMessageService
func storeMessageViaService(message *services.HL7Message, interfaceID, interfaceName string) error {
	if interfaceMessageService == nil {
		return fmt.Errorf("interface message service not initialized")
	}

	// Extract message type from HL7 message
	messageType := extractHL7MessageType(message.Content)

	// Prepare message data
	messageData := &services.MessageData{
		MessageID:       message.ID,
		CorrelationID:   message.ID, // Use same as message ID for received messages
		InterfaceID:     interfaceID,
		Status:          "received",
		Priority:        5, // Default medium priority
		ReceivedAt:      message.ReceivedAt,
		SourceType:      "mllp",
		SourceEndpoint:  message.Source,
		SourceIP:        "127.0.0.1", // Default for MLLP connections
		MessageType:     messageType,
		MessageSize:     message.Size,
		MessageEncoding: "UTF-8",
		MongoDocumentID: "",
	}

	log.Printf("📊 Storing message via InterfaceMessageService for interface %s", interfaceName)

	err := interfaceMessageService.StoreMessage(interfaceID, messageData)
	if err != nil {
		return fmt.Errorf("InterfaceMessageService.StoreMessage failed: %w", err)
	}

	log.Printf("✅ Message %s stored successfully via service", message.ID)
	return nil
}

// extractHL7MessageType extracts the message type from HL7 message content
func extractHL7MessageType(content string) string {
	lines := strings.Split(content, "\r")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "MSH") {
		parts := strings.Split(lines[0], "|")
		if len(parts) > 8 {
			return parts[8] // MSH.9 - Message Type
		}
	}
	return "Unknown"
}

// processMessageWithTransformation processes a message through the PostgreSQL transformation pipeline
func processMessageWithTransformation(message *services.HL7Message, interfaceID, interfaceName, targetConfig string) error {
	// Check if PostgreSQL transformation service is available
	if postgresTransformationService == nil {
		log.Printf("⚠️ PostgreSQL transformation service not available, skipping transformation for %s", interfaceName)
		return fmt.Errorf("PostgreSQL transformation service not initialized")
	}

	// Extract message type for logging
	messageContent := message.Content
	messageType := "Unknown"
	lines := strings.Split(messageContent, "\r")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "MSH") {
		parts := strings.Split(lines[0], "|")
		if len(parts) > 8 {
			messageType = parts[8] // MSH.9 - Message Type
		}
	}

	log.Printf("📋 HL7 Message Type: %s", messageType)
	log.Printf("🔄 Starting PostgreSQL transformation with atomic mappings for interface %s", interfaceName)

	// Parse HL7 message for PostgreSQL service
	parsedHL7 := hl7.ParseHL7Enhanced(message.Content)
	if parsedHL7 == nil || !parsedHL7.Success {
		return fmt.Errorf("HL7 parsing failed for message %s", message.ID)
	}

	// Prepare source data with parsed HL7 structure
	sourceData := map[string]interface{}{
		"content":       message.Content,
		"message_id":    message.ID,
		"source":        message.Source,
		"received_at":   message.ReceivedAt,
		"connection_id": message.ConnectionID,
		"listener_id":   message.ListenerID,
		"size":         message.Size,
		"message_type":  messageType,
		"parsed_hl7":    parsedHL7.EnhancedSegments,
	}

	// Execute PostgreSQL transformation with atomic mappings
	pgResult, err := postgresTransformationService.ExecuteTransformation(interfaceID, messageType, sourceData)
	if err != nil {
		return fmt.Errorf("PostgreSQL transformation failed: %w", err)
	}

	log.Printf("✅ PostgreSQL transformation completed successfully with atomic mappings")
	return handlePostgresTransformationResult(pgResult, interfaceID, interfaceName, messageType, message)
}

// handlePostgresTransformationResult handles results from PostgreSQL transformation service
func handlePostgresTransformationResult(result *services.PostgresTransformationResult, interfaceID, interfaceName, messageType string, message *services.HL7Message) error {
	// Handle transformation results and store output
	if !result.Success {
		log.Printf("❌ PostgreSQL Transformation failed for %s: %s", interfaceName, result.ErrorMessage)
		// Store failed transformation result
		if outputMessageService != nil {
			outputResult := &services.TransformationResult{
				Success:              false,
				TransformedMessage:   result.TransformedMessage,
				TransformationMetadata: result.TransformationMetadata,
				ValidationErrors:     result.ValidationErrors,
				FHIRResourceType:     result.FHIRResourceType,
				FHIRResourceID:       result.FHIRResourceID,
				ProcessingTimeMs:     result.ProcessingTimeMs,
				ErrorMessage:         result.ErrorMessage,
			}
			outputErr := outputMessageService.StoreTransformationResult(context.Background(), interfaceID, message.ID, message.ID, outputResult)
			if outputErr != nil {
				log.Printf("⚠️ Failed to store failed transformation result: %v", outputErr)
			}
		}
		return fmt.Errorf("transformation failed: %s", result.ErrorMessage)
	}

	log.Printf("✅ PostgreSQL Transformation completed successfully in %dms", result.ProcessingTimeMs)

	// Store successful transformation result
	if outputMessageService != nil {
		outputResult := &services.TransformationResult{
			Success:              true,
			TransformedMessage:   result.TransformedMessage,
			TransformationMetadata: result.TransformationMetadata,
			ValidationErrors:     result.ValidationErrors,
			FHIRResourceType:     result.FHIRResourceType,
			FHIRResourceID:       result.FHIRResourceID,
			ProcessingTimeMs:     result.ProcessingTimeMs,
			ErrorMessage:         "",
		}

		outputErr := outputMessageService.StoreTransformationResult(context.Background(), interfaceID, message.ID, message.ID, outputResult)
		if outputErr != nil {
			log.Printf("⚠️ Failed to store transformation result: %v", outputErr)
		}

		log.Printf("📊 Stored transformation result in output table for interface %s", interfaceName)
	}

	return nil
}


// forwardTransformedMessage forwards the transformed message to the target endpoint
func forwardTransformedMessage(transformedData map[string]interface{}, targetConfig, interfaceName string) error {
	// Parse target configuration
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(targetConfig), &config); err != nil {
		return fmt.Errorf("invalid target config: %w", err)
	}

	// Extract target endpoint information
	endpoint, ok := config["endpoint"].(string)
	if !ok {
		return fmt.Errorf("no endpoint specified in target config")
	}

	protocol, ok := config["protocol"].(string)
	if !ok {
		protocol = "http" // default
	}

	log.Printf("📤 Forwarding transformed message to %s via %s", endpoint, protocol)

	switch protocol {
	case "http", "https":
		return forwardViaHTTP(transformedData, endpoint, interfaceName)
	case "tcp":
		return forwardViaTCP(transformedData, endpoint, interfaceName)
	default:
		return fmt.Errorf("unsupported protocol: %s", protocol)
	}
}

// forwardViaHTTP forwards message via HTTP POST
func forwardViaHTTP(data map[string]interface{}, endpoint, interfaceName string) error {
	// Convert data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", endpoint, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Source-Interface", interfaceName)

	// Send request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("✅ Message forwarded successfully to %s (status: %d)", endpoint, resp.StatusCode)
		return nil
	} else {
		return fmt.Errorf("HTTP request failed with status: %d", resp.StatusCode)
	}
}

// forwardViaTCP forwards message via TCP connection
func forwardViaTCP(data map[string]interface{}, endpoint, interfaceName string) error {
	// TODO: Implement TCP forwarding for HL7 messages
	log.Printf("✅ TCP forwarding to %s completed (stub implementation)", endpoint)
	return nil
}

// startHttpListener starts an HTTP listener for FHIR interfaces
func startHttpListener(port int, interfaceID, interfaceName, targetConfig, transformationMapping string) {
	mux := http.NewServeMux()

	// Handle FHIR resource endpoints
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		log.Printf("📥 FHIR message received on %s (port %d)", interfaceName, port)

		// Read the message body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("❌ Error reading FHIR message: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Log message details for debugging
		log.Printf("📄 FHIR message size: %d bytes", len(body))
		log.Printf("🔍 Content-Type: %s", r.Header.Get("Content-Type"))

		// Store the message (using body content)
		messageID := fmt.Sprintf("fhir_%d_%s", time.Now().UnixNano(), interfaceID)
		log.Printf("💾 Storing FHIR message with ID: %s", messageID)

		// TODO: Implement actual database storage and FHIR processing pipeline
		// This currently just processes the body for validation
		if len(body) == 0 {
			log.Printf("⚠️ Empty FHIR message received")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "Empty message body"}`))
			return
		}

		log.Printf("✅ FHIR message processed successfully (interface: %s, messageID: %s)", interfaceName, messageID)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "accepted", "interface": "` + interfaceName + `"}`))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	log.Printf("🌐 Starting HTTP server for %s on port %d", interfaceName, port)
	if err := server.ListenAndServe(); err != nil {
		log.Printf("❌ HTTP server error for %s: %v", interfaceName, err)
	}
}
*/

// ============================================================================
// END OF DEAD CODE - OLD MLLP SYSTEM
// ============================================================================
