package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"ezhealthkonnect/config"
	"ezhealthkonnect/controllers"
	"ezhealthkonnect/fhir"
	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/hl7"
	"ezhealthkonnect/models"
	"ezhealthkonnect/processing"
	"ezhealthkonnect/services"
	"ezhealthkonnect/services/backpressure"
	cdaSchemaLoader "ezhealthkonnect/cda"
	cdafhir "ezhealthkonnect/services/cda_fhir"
	cdastorage "ezhealthkonnect/services/cda_storage"
	cdaterminology "ezhealthkonnect/services/cda_terminology"
	cdaparser "ezhealthkonnect/services/parsers/cda"
	"ezhealthkonnect/services/connectors"
	svcmapping "ezhealthkonnect/services/mapping"
	"ezhealthkonnect/services/storage"
	"ezhealthkonnect/services/dbpool"
	"ezhealthkonnect/services/ai"
	"ezhealthkonnect/services/schema"
	"ezhealthkonnect/services/executors/enrichment"
	fhirnarrative "ezhealthkonnect/services/fhir_narrative"
	"ezhealthkonnect/services/logger"
	appmetrics "ezhealthkonnect/services/metrics"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Track server start time for uptime calculation
var startTime time.Time

// Global PostgreSQL transformation service
var postgresTransformationService *services.PostgresTransformationService

// Global Processing Engine
var processingEngine *processing.ProcessingEngine

// Global DLQ Service (wired into outbound connector executor and DLQController)
var dlqSvc *connectors.DLQService

// Global Code Template Service (wired into script executor via processingEngine)
var codeTemplateSvc *services.CodeTemplateService

// Global Object Storage Service
var objectStorageService *storage.ObjectStorageService

// Global CDA Document Store
var cdaDocumentStore cdastorage.CDADocumentStore

func main() {
	// Load .env before anything reads os.Getenv — harmless if the file doesn't exist
	// (production deployments pass variables via the shell/service manager instead).
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("⚠️  .env not loaded: %v", err)
	}

	// Record start time
	startTime = time.Now()

	// Initialize structured logger first (LOG_FORMAT=json for prod, text for dev)
	logger.Init()
	logger.Info("logger initialized", "format", os.Getenv("LOG_FORMAT"), "level", os.Getenv("LOG_LEVEL"))

	// Initialize Prometheus metrics (P3-2)
	appmetrics.Init()
	log.Printf("✅ Prometheus metrics initialized")

	// Initialize log rotation service (file-based, for app.log)
	appLogger := services.GetApplicationLogger()
	defer appLogger.Close()
	// Close all per-interface log file handles on shutdown
	defer logger.CloseAllInterfaces()
	// Close all DB enrichment connection pools on shutdown (P6)
	defer dbpool.Get().CloseAll()
	// Drain and shut down all per-interface worker pools (P9)
	defer backpressure.Get().ShutdownAll()
	log.Printf("log rotation initialized - logs/application/app.log, logs/interfaces/{id}/interface.log")

	// Load configuration
	cfg := config.Load()

	// Log configuration for debugging - only if verbose mode
	if cfg.VerboseLogging {
		cfg.LogConfiguration()
	}

	// ── Credential Store ─────────────────────────────────────────
	// Encrypts/decrypts connectivity configs (SFTP, S3, DB passwords, etc.)
	// Set APP_CREDENTIAL_KEY to a base64-encoded 32-byte key:
	//   openssl rand -base64 32
	// If unset, credentials are stored in plaintext (dev-only; warning is logged).
	var credStore *services.CredentialStore
	if keyB64 := os.Getenv("APP_CREDENTIAL_KEY"); keyB64 != "" {
		var credErr error
		credStore, credErr = services.NewCredentialStore(keyB64)
		if credErr != nil {
			log.Fatalf("❌ FATAL: APP_CREDENTIAL_KEY is invalid: %v", credErr)
		}
	}
	services.WarnIfNoOpCredentialStore(credStore)

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

			// Initialize Object Storage Service + CDA Document Store
			if storageDriver, storageErr := storage.NewDriverFromEnv(); storageErr == nil {
				bucket := storage.DefaultBucketName()
				if objSvc, objErr := storage.NewObjectStorageService(storageDriver, bucket); objErr == nil {
					objectStorageService = objSvc
					log.Printf("✅ Object Storage Service initialized (driver=%s, bucket=%s)", storageDriver.DriverName(), bucket)
				} else {
					log.Printf("⚠️  Object Storage Service init failed: %v", objErr)
				}
				// CDA Document Store: uses same driver for large-doc routing.
				cdaDocumentStore = cdastorage.NewCDADocumentStore(db, storageDriver, bucket)
			} else {
				log.Printf("⚠️  Object Storage driver unavailable: %v — falling back to DB-only mode", storageErr)
				// CDA Document Store: inline-only mode (no object storage).
				cdaDocumentStore = cdastorage.NewCDADocumentStore(db, nil, "")
			}

			// Initialize schema systems BEFORE the processing engine so the FHIR
			// schema loader is ready when the first queued messages are processed.
			if cfg.UseFilesystemSchema() {
				hl7SchemaPath := filepath.Join(cfg.GetSchemaDirectory(), "hl7")
				hl7.InitSchemaLoader(hl7SchemaPath)
				if sl := hl7.GetSchemaLoader(); sl != nil {
					sl.SetMaxCacheSize(cfg.SchemaCacheSize)
				}
				fhirSchemaDir := cfg.GetFHIRSchemaDirectory()
				log.Printf("🔧 Initializing FHIR schema system from: %s", fhirSchemaDir)
				fhir.InitFHIRSchemaLoader(fhirSchemaDir)
				// r4.InitRegistry compiles all FHIR profiles and valuesets from .gz files.
				// Run in background so the HTTP listener opens immediately.
				// The registry uses sync.Once internally, so any request that needs a
				// compiled profile blocks transparently until loading completes.
				go func() {
					if err := r4.InitRegistry(fhirSchemaDir); err != nil {
						log.Printf("⚠️  [r4] Registry init warning: %v", err)
					} else {
						log.Printf("✅ FHIR R4 registry ready")
					}
				}()
			}

			// Initialize Processing Engine
			processingEngine = processing.NewProcessingEngine(db, credStore)
			if err := processingEngine.Start(); err != nil {
				log.Printf("❌ Failed to start Processing Engine: %v", err)
			} else {
				log.Printf("✅ Processing Engine initialized and started")
			}

			// CDA DOCUMENT STORE: wire into cda.parse executor for auto-persistence
			if cdaDocumentStore != nil {
				processingEngine.SetCDADocumentStore(cdaDocumentStore)
			}

			// CDA→FHIR MAPPING LOGS: wire object storage so mapping logs are persisted asynchronously.
			if objectStorageService != nil {
				processingEngine.SetCDAToFHIRObjectStorage(objectStorageService)
			}

			// CODE TEMPLATES: wire the service into the pipeline executor
			codeTemplateSvc = services.NewCodeTemplateService(db)
			services.SeedSystemTemplates(db)
			processingEngine.SetCodeTemplateService(codeTemplateSvc)
			log.Printf("📦 Code Template Service initialized (6 OOB libraries)")

			// DLQ SERVICE: write delivery failures, redrive via pipeline service
			dlqSvc = connectors.NewDLQService(db)
			processingEngine.SetDLQService(dlqSvc)
			redriver := processingEngine.GetPipelineRedriver()
			dlqSvc.SetPipelineRedriver(redriver)
			dlqSvc.StartPoller(context.Background(), redriver)
			log.Printf("📥 DLQ Service initialized (auto-redrive poller started)")

			// METRICS COLLECTOR: aggregate transformation quality + DLQ metrics every 5 min.
			// AlertNotificationService is wired so threshold breaches dispatch email/webhook
			// notifications. credStore enables decryption of SMTP credentials stored in settings.
			notificationSvc := services.NewAlertNotificationService(db, credStore)
			metricsCollector := services.NewMetricsCollector(db, notificationSvc)
			metricsCollector.Start(context.Background())

			// RETENTION ENFORCEMENT: mandatory HIPAA data retention — purges old messages,
			// DLQ rows, quality scores, and pipeline history on a 1-hour schedule.
			// Also purges object storage artifacts (raw/parsed/transformed/logs/mapping_log)
			// for purged messages when objectStorageService is configured.
			// Retention periods are configured in Admin > Settings > Message Queue.
			retentionSvc := services.NewRetentionEnforcementService(db, 0, objectStorageService)
			retentionSvc.Start(context.Background())

			// Rebuild HL7→FHIR OOB templates for all IG-covered message types.
			// Runs in the background so it never delays server startup.
			// Triggered every boot so new IG seed data (e.g. V115 anchors) is
			// immediately reflected in templates without a manual admin call.
			go func() {
				builder := svcmapping.NewOOBTemplateBuilder(db)
				results := builder.BuildWithIGCoverage(context.Background())
				built, failed := 0, 0
				for _, r := range results {
					if r.Err != nil {
						failed++
					} else {
						built++
					}
				}
				log.Printf("📋 OOB Templates (IG): %d built, %d skipped/failed", built, failed)
			}()
		} else {
			log.Printf("❌ FATAL: Database connection failed after %d retries - transformation pipeline will not be available", maxRetries)
		}
	}















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
		if err := r4.InitRegistry(fhirSchemaDir); err != nil {
			log.Printf("⚠️  [r4] Registry init warning: %v", err)
		}

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

	// ── Observability endpoints ────────────────────────────────────────────────
	// GET /metrics  — Prometheus scrape endpoint (P3-2)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// ── Kubernetes / Docker health probes ─────────────────────────────────────
	// GET /healthz  — liveness:  returns 200 as long as the process is alive
	// GET /readyz   — readiness: returns 503 until all critical dependencies are up.
	// Checked by Docker/Kubernetes before routing traffic to this container.
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "alive"})
	})

	router.GET("/readyz", func(c *gin.Context) {
		type check struct {
			OK      bool   `json:"ok"`
			Message string `json:"message,omitempty"`
		}
		checks := map[string]check{}
		ready := true

		// Database — critical; no DB means no transformation or delivery.
		if db == nil {
			checks["database"] = check{OK: false, Message: "not configured"}
			ready = false
		} else if err := db.Ping(); err != nil {
			checks["database"] = check{OK: false, Message: err.Error()}
			ready = false
		} else {
			checks["database"] = check{OK: true}
		}

		// Object storage — critical when configured; raw HL7 and FHIR bundles stored here.
		// When storage is not configured, passes as DB-only mode (local dev acceptable).
		if objectStorageService != nil {
			pingCtx, pingCancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
			defer pingCancel()
			if err := objectStorageService.Ping(pingCtx); err != nil {
				checks["object_storage"] = check{OK: false, Message: err.Error()}
				ready = false
			} else {
				checks["object_storage"] = check{OK: true, Message: objectStorageService.DriverName()}
			}
		} else {
			checks["object_storage"] = check{OK: true, Message: "not configured (DB-only mode)"}
		}

		// Processing engine — must be running to accept inbound messages.
		if processingEngine == nil || !processingEngine.IsRunning() {
			checks["engine"] = check{OK: false, Message: "not running"}
			ready = false
		} else {
			checks["engine"] = check{OK: true}
		}

		status := http.StatusOK
		statusText := "ready"
		if !ready {
			status = http.StatusServiceUnavailable
			statusText = "not ready"
		}
		c.JSON(status, gin.H{"status": statusText, "checks": checks})
	})

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

	// API routes — all /api endpoints require the request to have passed
	// through the authenticated Node.js proxy (X-Internal-Proxy-Secret check).
	// Role-sensitive sub-groups add requireProxiedAdmin() or requireProxiedSuperAdmin()
	// on top of this base gate.
	WarnIfProxySecretMissing()
	api := router.Group("/api", requireProxiedRequest())
	{
		// HL7 ROUTES
		hl7Group := api.Group("/hl7")
		hl7Ctrl := controllers.NewHL7Controller(cfg)
		{
			hl7Group.POST("/parse", hl7Ctrl.ParseMessage)
			hl7Group.POST("/validate", hl7Ctrl.ValidateMessage)
			hl7Group.GET("/stats", hl7Ctrl.GetStats)
			hl7Group.GET("/type-registry", hl7Ctrl.GetTypeRegistry)
			hl7Group.GET("/segment-resources", hl7Ctrl.GetSegmentResources)
		}

		// SYSTEM ROUTES
		systemCtrl := controllers.NewSystemController(cfg, db)
		systemGroup := api.Group("/system")
		{
			systemGroup.GET("/health", systemCtrl.HealthCheck)
			systemGroup.GET("/info", systemCtrl.GetInfo)
			systemGroup.GET("/metrics", systemCtrl.GetMetrics)

			// SCHEMA PACKAGE MANAGEMENT
			schemaManager := schema.NewSchemaPackageManager(cfg.GetSchemaDirectory(), cfg.GetSchemaDistPath())
			schemaCtrl := controllers.NewSchemaController(schemaManager)
			schemaGroup := systemGroup.Group("/schemas")
			{
				schemaGroup.GET("/catalog", schemaCtrl.GetCatalog)
				schemaGroup.GET("/installed", schemaCtrl.GetInstalled)
				schemaGroup.POST("/install", schemaCtrl.InstallPackage)
				schemaGroup.GET("/progress/:id", schemaCtrl.GetProgress)
				schemaGroup.DELETE("/:id", schemaCtrl.RemovePackage)
			}
			log.Printf("✅ Schema Package Manager initialized (root=%s)", cfg.GetSchemaDirectory())
			// P9 backpressure metrics — queue depth per interface
			systemGroup.GET("/queue-depths", func(c *gin.Context) {
				depths := backpressure.Get().QueueDepths()
				c.JSON(200, gin.H{"success": true, "data": depths, "count": len(depths)})
			})

			// DLQ management moved to /api/fhir/dlq (registered below with the DLQService)

			// Admin settings (storage, SMTP, security, HL7/FHIR, queue, connectors, alerts, performance)
			if db != nil {
				// Initialise the package-level AppSettingsCache so all services can call
				// services.GetAppSettings().GetXxx() without needing explicit DI.
				services.InitAppSettings(db)
				log.Printf("✅ AppSettingsCache initialized")

				settingsCtrl := controllers.NewSettingsController(db, credStore)
				settingsCtrl.RegisterRoutes(systemGroup.Group("/settings"))
				log.Printf("✅ Settings Controller initialized (8 sections)")
			}
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

			// ADDED: FHIR Validation Review Queue
			vqCtrl := controllers.NewFHIRValidationQueueController(db)
			vqGroup := fhirGroup.Group("/validation-queue")
			vqCtrl.RegisterRoutes(vqGroup)

			// ADDED: Transformation Quality Review Queue (integration team only)
			qualityCtrl := controllers.NewQualityController(db)
			qualityCtrl.RegisterRoutes(fhirGroup.Group("/quality", requireProxiedSuperAdmin()))

			// MONITORING DASHBOARD: live ops KPIs, alerts, message feed, per-interface health.
			// Requires proxy auth only (no role check) — all authenticated users can view.
			monitoringCtrl := controllers.NewMonitoringController(db)
			monitoringCtrl.RegisterRoutes(api.Group("/monitoring"))

			// ADDED: Dead-Letter Queue management (admin access)
			if dlqSvc != nil {
				dlqCtrl := controllers.NewDLQController(db, dlqSvc)
				dlqCtrl.RegisterRoutes(fhirGroup.Group("/dlq", requireProxiedAdmin()))
			}

			// ADDED: Ad-hoc FHIR Validator tool (fhir-validator.html)
			fhirValidatorCtrl := controllers.NewFHIRValidatorController()
			fhirValidatorCtrl.RegisterRoutes(fhirGroup)

			// ADDED: Transformation Pipeline Test Routes
			transformTestCtrl := controllers.NewTransformationTestController(db, credStore)
			fhirGroup.POST("/pipeline/test", transformTestCtrl.TestPipeline)
			fhirGroup.POST("/pipeline/test-api-endpoint", transformTestCtrl.TestAPIEndpoint) // Test API endpoint before configuring mapping
			fhirGroup.POST("/pipeline/validate-script", transformTestCtrl.ValidateScript)    // Validate JavaScript script
			fhirGroup.GET("/pipeline/:interfaceId/:messageType", transformTestCtrl.GetPipeline)
			fhirGroup.GET("/field-search", transformTestCtrl.SearchFields)   // USCDI smart search for CDA field picker
			fhirGroup.POST("/cda/parse", transformTestCtrl.ParseCDA)          // CDA document parse + section summary (wizard preview)

			// FHIR Narrative Generator
			// Accepts a FHIR resource as JSON body and returns XHTML narrative.
			// If resource.text.div is already populated it is echoed back unchanged.
			// POST /api/fhir/narrative/:resourceType       — primary form
			// GET  /api/fhir/narrative/:resourceType/:id  — spec form (resource JSON in body)
			narrativeGen := fhirnarrative.NewFHIRNarrativeGenerator()
			narrativeHandler := func(c *gin.Context) {
				var resource map[string]interface{}
				if err := c.ShouldBindJSON(&resource); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{
						"success": false,
						"error":   "FHIR resource JSON required in request body",
					})
					return
				}
				hadExisting := false
				if textMap, ok := resource["text"].(map[string]interface{}); ok {
					if div, ok := textMap["div"].(string); ok && div != "" {
						hadExisting = true
					}
				}
				div := narrativeGen.Generate(resource)
				c.JSON(http.StatusOK, gin.H{
					"success":   true,
					"narrative": div,
					"generated": !hadExisting && div != "",
				})
			}
			fhirGroup.POST("/narrative/:resourceType", narrativeHandler)
			fhirGroup.GET("/narrative/:resourceType/:id", narrativeHandler)

			// ADDED: Delta/override model — interface-level sparse overrides on OOB templates
			mappingDeltaSvc := services.NewHL7FHIRTransformServiceV3(db)
			mappingDeltaCtrl := controllers.NewMappingDeltaController(db, mappingDeltaSvc)
			mappingDeltaCtrl.RegisterRoutes(fhirGroup)

			// ADDED: Optional segment block toggles (per-interface opt-in mappings)
			optionalSegCtrl := controllers.NewOptionalSegmentsController(db)
			optionalSegCtrl.RegisterRoutes(fhirGroup)

			// ADDED: CDA Schema Browser + Mapping Delta APIs (/api/cda/*)
			// Shares the same CDA schema loader and mapper used by the cda.to_fhir executor.
			{
				cdaLoader, cdaLoaderErr := cdaSchemaLoader.NewCDASchemaLoader("./cda/schemas")
				if cdaLoaderErr != nil {
					log.Printf("⚠️  [cda] Schema loader unavailable: %v — /api/cda/schema endpoints will return 503", cdaLoaderErr)
				}
				cdaMapper := cdafhir.NewGenericCDAFHIRMapper(db, cdaLoader)
				cdaTermSvc := cdaterminology.NewTerminologyService(db)
				cdaMapper.WithTerminologyService(cdaTermSvc)
				cdaSchemaCtrl := controllers.NewCDASchemaController(db, cdaLoader, cdaMapper)
				cdaSchemaCtrl.RegisterRoutes(api.Group("/cda"))
				log.Printf("✅ CDA Schema Controller registered (/api/cda/schema, /api/cda/mappings, /api/cda/templates)")

				// Sprint C — conformance validation (POST /api/cda/validate)
				cdaValidationCtrl := controllers.NewCDAValidationController(cdaLoader)
				cdaValidationCtrl.RegisterRoutes(api.Group("/cda"))
				log.Printf("✅ CDA Validation Controller registered (/api/cda/validate)")

				// Sprint E — document storage (POST/GET/DELETE /api/cda/documents)
				var cdaParserForStore *cdaparser.CDAParserService
				if svc, err := cdaparser.NewFromSchemaDir("./cda/schemas"); err == nil {
					cdaParserForStore = svc
				}
				cdaDocCtrl := controllers.NewCDADocumentController(cdaDocumentStore, cdaParserForStore)
				cdaDocCtrl.RegisterRoutes(api.Group("/cda/documents"))
				log.Printf("✅ CDA Document Controller registered (/api/cda/documents)")
			}

			// ── OOB template rebuild ─────────────────────────────────────────────
			// Rebuild endpoints are admin-only and run asynchronously.
			// POST returns 202 immediately; GET /rebuild-status polls progress.
			var oobRebuildMu sync.Mutex
			type oobJobState struct {
				Running      bool       `json:"running"`
				Mode         string     `json:"mode"` // "full" | "ig" | "targeted"
				MessageTypes []string   `json:"messageTypes,omitempty"`
				StartedAt    time.Time  `json:"started_at"`
				FinishedAt   *time.Time `json:"finished_at,omitempty"`
				Built        int        `json:"built"`
				Failed       int        `json:"failed"`
				Results      []gin.H    `json:"results,omitempty"`
				Error        string     `json:"error,omitempty"`
			}
			oobJob := &oobJobState{}

			// startOOBRebuild launches an async rebuild. mode is "full", "ig", or
			// "targeted". For "targeted", pass the message types to rebuild; the
			// caller is responsible for populating oobJob.MessageTypes first.
			startOOBRebuild := func(mode string, messageTypes []string) (accepted bool) {
				oobRebuildMu.Lock()
				defer oobRebuildMu.Unlock()
				if oobJob.Running {
					return false
				}
				oobJob.Running = true
				oobJob.Mode = mode
				oobJob.MessageTypes = messageTypes
				oobJob.StartedAt = time.Now()
				oobJob.FinishedAt = nil
				oobJob.Built = 0
				oobJob.Failed = 0
				oobJob.Results = nil
				oobJob.Error = ""
				go func() {
					builder := svcmapping.NewOOBTemplateBuilder(db)
					ctx := context.Background()
					var results []svcmapping.BuildResult
					switch mode {
					case "ig":
						results = builder.BuildWithIGCoverage(ctx)
					case "targeted":
						results = builder.BuildForMessageTypes(ctx, messageTypes)
					default:
						results = builder.BuildAll(ctx)
					}
					now := time.Now()
					oobRebuildMu.Lock()
					defer oobRebuildMu.Unlock()
					oobJob.Running = false
					oobJob.FinishedAt = &now
					for _, r := range results {
						if r.Err != nil {
							oobJob.Failed++
							oobJob.Results = append(oobJob.Results, gin.H{
								"messageType": r.MessageType, "status": "error", "error": r.Err.Error(),
							})
						} else {
							oobJob.Built++
							oobJob.Results = append(oobJob.Results, gin.H{
								"messageType": r.MessageType, "status": "ok",
								"resources": r.ResourcesBuilt, "mappings": r.MappingsBuilt, "warnings": r.Warnings,
							})
						}
					}
				}()
				return true
			}

			adminTemplates := fhirGroup.Group("/templates", requireProxiedSuperAdmin())
			{
				// POST /api/fhir/templates/rebuild-oob — full rebuild (all message types)
				adminTemplates.POST("/rebuild-oob", func(c *gin.Context) {
					if !startOOBRebuild("full", nil) {
						c.JSON(http.StatusConflict, gin.H{"success": false, "error": "rebuild already in progress"})
						return
					}
					c.JSON(http.StatusAccepted, gin.H{
						"success": true,
						"message": "full OOB template rebuild started — poll /api/fhir/templates/rebuild-status for progress",
					})
				})

				// POST /api/fhir/templates/rebuild-oob-ig — IG-covered message types only (faster)
				adminTemplates.POST("/rebuild-oob-ig", func(c *gin.Context) {
					if !startOOBRebuild("ig", nil) {
						c.JSON(http.StatusConflict, gin.H{"success": false, "error": "rebuild already in progress"})
						return
					}
					c.JSON(http.StatusAccepted, gin.H{
						"success": true,
						"message": "IG-covered OOB template rebuild started — poll /api/fhir/templates/rebuild-status for progress",
					})
				})

				// POST /api/fhir/templates/rebuild-oob-targeted — rebuild only the
				// message types listed in the request body. Much faster than a full
				// rebuild when only a handful of templates need refreshing after an
				// anchor or Rule change.
				// Body: { "messageTypes": ["SIU^S12", "SIU^S13"] }
				adminTemplates.POST("/rebuild-oob-targeted", func(c *gin.Context) {
					var req struct {
						MessageTypes []string `json:"messageTypes" binding:"required,min=1"`
					}
					if err := c.ShouldBindJSON(&req); err != nil {
						c.JSON(http.StatusBadRequest, gin.H{
							"success": false,
							"error":   "body must be { \"messageTypes\": [\"SIU^S12\", ...] }",
						})
						return
					}
					if !startOOBRebuild("targeted", req.MessageTypes) {
						c.JSON(http.StatusConflict, gin.H{"success": false, "error": "rebuild already in progress"})
						return
					}
					c.JSON(http.StatusAccepted, gin.H{
						"success":      true,
						"messageTypes": req.MessageTypes,
						"message":      "targeted OOB rebuild started — poll /api/fhir/templates/rebuild-status for progress",
					})
				})

				// GET /api/fhir/templates/rebuild-status — poll rebuild progress
				adminTemplates.GET("/rebuild-status", func(c *gin.Context) {
					oobRebuildMu.Lock()
					snapshot := *oobJob
					oobRebuildMu.Unlock()
					c.JSON(http.StatusOK, gin.H{"success": true, "job": snapshot})
				})
			}

			// ADDED: Template Preview — resource list + confidence data for a message type
			fhirGroup.GET("/template/preview", func(c *gin.Context) {
				messageType := c.Query("messageType")
				if messageType == "" {
					c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "messageType query param required"})
					return
				}

				type ResourcePreview struct {
					Name           string  `json:"name"`
					FieldCount     int     `json:"field_count"`
					AvgConfidence  float64 `json:"avg_confidence"`
					MinConfidence  float64 `json:"min_confidence"`
					MaxConfidence  float64 `json:"max_confidence"`
					RequiredFields int     `json:"required_fields"`
					Optional       bool    `json:"optional"`
				}

				// Query the OOB template for this message type
				var templateConfigJSON []byte
				err := db.QueryRowContext(c.Request.Context(),
					`SELECT template_config FROM hl7_fhir_templates WHERE message_type = $1 AND is_default = true LIMIT 1`,
					messageType,
				).Scan(&templateConfigJSON)
				if err != nil {
					c.JSON(http.StatusNotFound, gin.H{"success": false, "error": fmt.Sprintf("No template found for message type: %s", messageType)})
					return
				}

				var tmplConfig map[string]interface{}
				if err := json.Unmarshal(templateConfigJSON, &tmplConfig); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to parse template config"})
					return
				}

				resourcesRaw, _ := tmplConfig["resources"].(map[string]interface{})
				previews := make([]ResourcePreview, 0, len(resourcesRaw))

				for resourceName, resourceData := range resourcesRaw {
					rMap, _ := resourceData.(map[string]interface{})
					mappingsRaw, _ := rMap["mappings"].([]interface{})
					optional, _ := rMap["optional"].(bool)

					preview := ResourcePreview{
						Name:     resourceName,
						Optional: optional,
						MinConfidence: 1.0,
					}

					var totalConf float64
					for _, m := range mappingsRaw {
						mMap, _ := m.(map[string]interface{})
						conf, _ := mMap["confidence"].(float64)
						required, _ := mMap["required"].(bool)
						totalConf += conf
						preview.FieldCount++
						if conf < preview.MinConfidence {
							preview.MinConfidence = conf
						}
						if conf > preview.MaxConfidence {
							preview.MaxConfidence = conf
						}
						if required {
							preview.RequiredFields++
						}
					}

					if preview.FieldCount > 0 {
						preview.AvgConfidence = totalConf / float64(preview.FieldCount)
					}
					previews = append(previews, preview)
				}

				// Sort: required resources first, then by name
				sort.Slice(previews, func(i, j int) bool {
					if previews[i].Optional != previews[j].Optional {
						return !previews[i].Optional
					}
					return previews[i].Name < previews[j].Name
				})

				c.JSON(http.StatusOK, gin.H{
					"success":     true,
					"messageType": messageType,
					"resources":   previews,
				})
			})

			// ADDED: Generic Pipeline Routes (reusable across all transformation types)
			api.POST("/pipeline/reference-variables", transformTestCtrl.GetAvailableReferenceVariables) // Get available variables per step

			// TODO: Pipeline Variables Controller needs refactoring to work with interface_id + message_type
			// pipelineVarsCtrl := controllers.NewPipelineVariablesController(db)
			// api.POST("/pipelines/variables", pipelineVarsCtrl.GetAvailableVariables)

			// ADDED: Database Query Test Routes (NO-CODE: Test queries before saving pipeline)
			dbTestCtrl := controllers.NewDatabaseTestController(db)
			api.POST("/database/test-query", dbTestCtrl.TestQuery)
			api.POST("/database/mongodb-schema", dbTestCtrl.GetMongoDBCollectionSchema)

			// ADDED: File Parser Routes (NO-CODE: Preview + OOB Template listing + Server File Browse)
			api.GET("/file-parser/browse", func(c *gin.Context) {
				serverOS := runtime.GOOS // "linux", "windows", "darwin"
				reqPath := c.DefaultQuery("path", "")

				type BrowseEntry struct {
					Name  string `json:"name"`
					Path  string `json:"path"`
					IsDir bool   `json:"isDir"`
					Size  int64  `json:"size"`
				}

				// OS-aware shortcuts
				type Shortcut struct {
					Name string `json:"name"`
					Path string `json:"path"`
					Icon string `json:"icon"`
				}
				shortcuts := []Shortcut{}
				if serverOS == "windows" {
					for _, drive := range "CDEFGHIJKLMNOPQRSTUVWXYZ" {
						dp := string(drive) + ":\\"
						if _, err := os.Stat(dp); err == nil {
							shortcuts = append(shortcuts, Shortcut{Name: string(drive) + ":", Path: filepath.ToSlash(dp), Icon: "💾"})
						}
					}
				} else {
					shortcuts = []Shortcut{
						{Name: "/ (root)", Path: "/", Icon: "🖥️"},
						{Name: "/app", Path: "/app", Icon: "📦"},
						{Name: "/data", Path: "/data", Icon: "🗄️"},
						{Name: "/tmp", Path: "/tmp", Icon: "🗂️"},
						{Name: "/var", Path: "/var", Icon: "📁"},
					}
					// Filter to only paths that exist
					filtered := []Shortcut{}
					for _, s := range shortcuts {
						if _, err := os.Stat(s.Path); err == nil {
							filtered = append(filtered, s)
						}
					}
					shortcuts = filtered
				}

				// Default start path
				if reqPath == "" {
					if serverOS == "windows" {
						reqPath = "C:\\"
					} else {
						reqPath = "/app"
						if _, err := os.Stat("/app"); err != nil {
							reqPath = "/"
						}
					}
				}

				// On Windows, treat "/" as the drives list level
				if serverOS == "windows" && (reqPath == "/" || reqPath == "") {
					c.JSON(http.StatusOK, gin.H{
						"success":   true,
						"path":      "/",
						"parent":    "",
						"entries":   shortcuts,
						"serverOS":  serverOS,
						"shortcuts": shortcuts,
					})
					return
				}

				cleanPath := filepath.Clean(reqPath)

				info, err := os.Stat(cleanPath)
				if err != nil {
					c.JSON(http.StatusOK, gin.H{"success": false, "error": fmt.Sprintf("path not found: %s", cleanPath)})
					return
				}
				if !info.IsDir() {
					cleanPath = filepath.Dir(cleanPath)
				}

				entries, err := os.ReadDir(cleanPath)
				if err != nil {
					c.JSON(http.StatusOK, gin.H{"success": false, "error": fmt.Sprintf("cannot read directory: %s", err.Error())})
					return
				}

				var dirs []BrowseEntry
				var files []BrowseEntry
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), ".") {
						continue
					}
					fi, _ := entry.Info()
					size := int64(0)
					if fi != nil && !entry.IsDir() {
						size = fi.Size()
					}
					be := BrowseEntry{
						Name:  entry.Name(),
						Path:  filepath.ToSlash(filepath.Join(cleanPath, entry.Name())),
						IsDir: entry.IsDir(),
						Size:  size,
					}
					if entry.IsDir() {
						dirs = append(dirs, be)
					} else {
						files = append(files, be)
					}
				}
				allEntries := append(dirs, files...)

				// Parent: empty at drive root (Windows) or filesystem root (Linux)
				parent := ""
				volRoot := filepath.VolumeName(cleanPath) + string(os.PathSeparator)
				if filepath.Clean(cleanPath) != filepath.Clean(volRoot) {
					parent = filepath.ToSlash(filepath.Dir(cleanPath))
				} else if serverOS == "windows" {
					parent = "/" // back to drives list
				}

				c.JSON(http.StatusOK, gin.H{
					"success":   true,
					"path":      filepath.ToSlash(cleanPath),
					"parent":    parent,
					"entries":   allEntries,
					"serverOS":  serverOS,
					"shortcuts": shortcuts,
				})
			})

			api.GET("/file-parser/templates", func(c *gin.Context) {
				list := enrichment.GetTemplateList()
				byCategory := enrichment.GetTemplatesByCategory()
				c.JSON(http.StatusOK, gin.H{
					"success":     true,
					"templates":   list,
					"by_category": byCategory,
					"count":       len(list),
				})
			})

			api.POST("/file-parser/preview", func(c *gin.Context) {
				var req struct {
					Content  string                 `json:"content"`
					FilePath string                 `json:"filePath"` // local server path — used when sourceType=local_path
					Config   map[string]interface{} `json:"config"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
					return
				}
				if req.Config == nil {
					req.Config = make(map[string]interface{})
				}

				// Cap preview at 10 records unless caller specifies fewer
				if maxR, ok := req.Config["maxRecords"]; !ok || maxR == nil || maxR == float64(0) {
					req.Config["maxRecords"] = 10
				}

				var inputData map[string]interface{}

				if req.FilePath != "" {
					// Local file path preview: let executor read the file directly
					req.Config["sourceType"] = "local_path"
					req.Config["filePath"] = req.FilePath
					delete(req.Config, "sourceField")
					inputData = map[string]interface{}{}
				} else {
					// Pasted content preview
					req.Config["sourceField"] = "_preview_content"
					inputData = map[string]interface{}{
						"_preview_content": req.Content,
					}
				}

				executor := enrichment.NewFileParserExecutor(nil, nil) // nil db, nil decrypt: preview doesn't need S3 credentials
				step := &models.TransformationStep{
					StepName: "preview",
					StepType: "file_parser",
					Enabled:  true,
					Config:   req.Config,
				}

				output, execErr := executor.Execute(c.Request.Context(), step, inputData)
				if execErr != nil {
					c.JSON(http.StatusOK, gin.H{
						"success": false,
						"error":   execErr.Error(),
					})
					return
				}

				stepOutput, _ := output["_stepOutput"].(map[string]interface{})
				execDetails, _ := output["_executionDetails"].(map[string]interface{})

				preview := gin.H{
					"record_count": stepOutput["record_count"],
					"column_count": stepOutput["column_count"],
					"columns":      stepOutput["columns"],
					"records":      stepOutput["records"],
				}
				if execDetails != nil {
					preview["format"] = execDetails["format"]
					if autoDetected, ok := execDetails["auto_detected"]; ok {
						preview["auto_detected"] = autoDetected
					}
					if detectedFmt, ok := execDetails["detected_format"]; ok {
						preview["detected_format"] = detectedFmt
					}
					if detectedDelim, ok := execDetails["detected_delimiter"]; ok {
						preview["detected_delimiter"] = detectedDelim
					}
				}

				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"preview": preview,
				})
			})

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

			// GET /api/fhir/mapping-catalogue?messageType=ADT%5EA01&fhirVersion=R4
			// Returns the atomicMappings list from the OOB template without running the
			// full transform pipeline (no HL7 parse, no FHIR schema loader, no bundle).
			// Used by the wizard step-3 preview — replaces the old 2-call flow that took
			// 5-30 s with a single DB read that returns in ~50 ms.
			fhirGroup.GET("/mapping-catalogue", func(c *gin.Context) {
				messageType := c.Query("messageType")
				if messageType == "" {
					c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "messageType query parameter is required"})
					return
				}
				fhirVersion := c.DefaultQuery("fhirVersion", "R4")

				svc := services.NewHL7FHIRTransformServiceV3(db)
				req := &services.TransformRequest{FHIRVersion: fhirVersion}
				fieldMappings, _, err := svc.GetFieldMappingsPublic(c.Request.Context(), messageType, fhirVersion, req)
				if err != nil || len(fieldMappings) == 0 {
					// Fall back to ADT^A01 so the UI always gets something usable
					log.Printf("⚠️ mapping-catalogue: no template for %s (%v), falling back to ADT^A01", messageType, err)
					fallbackReq := &services.TransformRequest{FHIRVersion: fhirVersion}
					fieldMappings, _, err = svc.GetFieldMappingsPublic(c.Request.Context(), "ADT^A01", fhirVersion, fallbackReq)
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
						return
					}
					messageType = "ADT^A01"
				}

				atomicMappings := svc.ConvertToAtomicMappingsPublic(fieldMappings)
				c.JSON(http.StatusOK, gin.H{
					"success":         true,
					"messageType":     messageType,
					"hl7Version":      "2.5",
					"fhirVersion":     fhirVersion,
					"atomicMappings":  atomicMappings,
					"validationErrors": []string{},
				})
			})

			// GET /api/fhir/interfaces/:id/resolved-mappings?messageType=ADT%5EA01&fhirVersion=R4
			// Returns the fully-resolved atomicMappings for the given interface + message type,
			// exactly as the runtime 5-step chain would produce them.
			// This is the single read path for the pipeline builder UI — the step config stores
			// only a reference (interface_id + message_type); the UI fetches from here on open.
			fhirGroup.GET("/interfaces/:interfaceId/resolved-mappings", func(c *gin.Context) {
				interfaceID := c.Param("interfaceId")
				messageType := c.DefaultQuery("messageType", "ADT^A01")
				fhirVersion := c.DefaultQuery("fhirVersion", "R4")

				svc := services.NewHL7FHIRTransformServiceV3(db)
				req := &services.TransformRequest{
					InterfaceID: interfaceID,
					FHIRVersion: fhirVersion,
				}
				fieldMappings, _, err := svc.GetFieldMappingsPublic(c.Request.Context(), messageType, fhirVersion, req)
				if err != nil || len(fieldMappings) == 0 {
					log.Printf("⚠️ resolved-mappings: no mappings for interface %s / %s (%v), falling back to OOB", interfaceID, messageType, err)
					fallbackReq := &services.TransformRequest{FHIRVersion: fhirVersion}
					fieldMappings, _, err = svc.GetFieldMappingsPublic(c.Request.Context(), messageType, fhirVersion, fallbackReq)
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
						return
					}
				}

				// Determine mapping mode so the UI can display it
				mappingMode := "oob"
				if db != nil {
					var usesStd bool
					var hasOverrides bool
					row := db.QueryRowContext(c.Request.Context(),
						`SELECT uses_standard_template,
						        (mapping_overrides IS NOT NULL) AS has_overrides
						 FROM interface_message_mappings
						 WHERE interface_id = $1 AND message_type = $2
						 LIMIT 1`, interfaceID, messageType)
					if scanErr := row.Scan(&usesStd, &hasOverrides); scanErr == nil {
						switch {
						case !usesStd:
							mappingMode = "custom"
						case hasOverrides:
							mappingMode = "delta"
						default:
							mappingMode = "oob"
						}
					}
				}

				atomicMappings := svc.ConvertToAtomicMappingsPublic(fieldMappings)
				c.JSON(http.StatusOK, gin.H{
					"success":         true,
					"interfaceId":     interfaceID,
					"messageType":     messageType,
					"fhirVersion":     fhirVersion,
					"mappingMode":     mappingMode,
					"atomicMappings":  atomicMappings,
					"count":           len(atomicMappings),
				})
			})

			// POST /api/fhir/transform-batch
			// Accepts raw HL7 text, splits on MSH boundaries, transforms each
			// message independently, and returns an array of FHIR bundles.
			fhirGroup.POST("/transform-batch", func(c *gin.Context) {
				var body struct {
					RawHL7         string `json:"rawHL7" binding:"required"`
					InterfaceID    string `json:"interfaceId"`
					CreateBundle   bool   `json:"createBundle"`
					ValidationMode string `json:"validationMode"`
					FHIRVersion    string `json:"fhirVersion"`
				}
				if err := c.ShouldBindJSON(&body); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				rawMessages := hl7.SplitMessages(body.RawHL7)
				if len(rawMessages) == 0 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "no HL7 messages found in input"})
					return
				}

				transformSvc := services.NewHL7FHIRTransformServiceV3(db)
				type batchResult struct {
					Index       int                       `json:"index"`
					MessageType string                    `json:"messageType"`
					Success     bool                      `json:"success"`
					Bundle      map[string]interface{}    `json:"bundle,omitempty"`
					Errors      []string                  `json:"errors,omitempty"`
					Warnings    []string                  `json:"warnings,omitempty"`
				}

				results := make([]batchResult, 0, len(rawMessages))

				for i, raw := range rawMessages {
					var parsed *hl7.EnhancedParsedMessage
					if realLoader := hl7.GetRealSchemaLoader(); realLoader != nil {
						parsed = hl7.ParseWithRealSchema(raw)
						if parsed == nil || !parsed.Success {
							parsed = hl7.ParseHL7Enhanced(raw)
						}
					} else {
						parsed = hl7.ParseHL7Enhanced(raw)
					}

					if parsed == nil || !parsed.Success {
						results = append(results, batchResult{
							Index:   i,
							Success: false,
							Errors:  []string{"parse failed: " + func() string {
								if parsed != nil { return parsed.Error }
								return "nil result"
							}()},
						})
						continue
					}

					// Build parsedHL7Data map from the enhanced result.
					//
					// "segmentGroups" — used by hl7assembly.ExtractSegmentGroup.
					//   Must be map[string][]EnhancedSegment (slices per segment type).
					//   parsed.EnhancedSegments is map[string]EnhancedSegment (one per type),
					//   so we wrap each segment in a single-element slice.
					//
					// "enhancedSegments" — used by the V3 field-mapper (extractEnhancedSegments).
					//   Accepts the original flat map[string]EnhancedSegment.
					segmentGroups := make(map[string][]hl7.EnhancedSegment, len(parsed.EnhancedSegments))
					for k, v := range parsed.EnhancedSegments {
						segmentGroups[k] = []hl7.EnhancedSegment{v}
					}
					parsedMap := map[string]interface{}{
						"segmentGroups":    segmentGroups,
						"enhancedSegments": parsed.EnhancedSegments,
						"segmentOrder":     parsed.SegmentOrder,
						"messageType": map[string]interface{}{
							"name":  parsed.MessageType.Name,
							"code":  parsed.MessageType.Code,
							"event": parsed.MessageType.Event,
						},
						"version": parsed.Version,
					}

					req := &services.TransformRequest{
						ParsedHL7Data:  parsedMap,
						InterfaceID:    body.InterfaceID,
						CreateBundle:   body.CreateBundle || true,
						ValidationMode: body.ValidationMode,
						FHIRVersion:    body.FHIRVersion,
					}

					resp, err := transformSvc.Transform(c.Request.Context(), req)
					if err != nil {
						results = append(results, batchResult{
							Index:   i,
							Success: false,
							Errors:  []string{err.Error()},
						})
						continue
					}

					results = append(results, batchResult{
						Index:       i,
						MessageType: resp.MessageType,
						Success:     resp.Success,
						Bundle:      resp.Bundle,
						Errors:      resp.Errors,
						Warnings:    resp.Warnings,
					})
				}

				log.Printf("📦 transform-batch: processed %d/%d messages", len(results), len(rawMessages))
				c.JSON(http.StatusOK, gin.H{
					"success":      true,
					"messageCount": len(rawMessages),
					"results":      results,
				})
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
			interfaceGroup.POST("/:id/reload-filter", interfaceCtrl.ReloadFamilyFilter)
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
			connectivityCtrl := controllers.NewConnectivityController(db, credStore)
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

				// Ad-hoc connector test (pipeline step context — no interface_id needed)
				connectivityGroup.POST("/test", connectivityCtrl.TestConnectorAdHoc)
			}
			log.Printf("✅ Connectivity Controller initialized (Multi-Connectivity Support)")
		}

		// ZSEGMENT CONTROLLER — enterprise Z-segment mapping configuration
		zsegmentCtrl := controllers.NewZSegmentController(services.NewZSegmentService(db))
		zsegmentCtrl.RegisterRoutes(api.Group("/zsegments"))
		log.Printf("✅ ZSegment Controller initialized (enterprise Z-segment mapping)")

		// MESSAGE CONTENT CONTROLLER — raw content and processing logs from object storage
		msgContentCtrl := controllers.NewMessageContentController(db, objectStorageService, cdaDocumentStore)
		msgContentCtrl.RegisterRoutes(api)
		log.Printf("✅ Message Content Controller initialized (driver=%s)", func() string {
			if objectStorageService != nil {
				return objectStorageService.DriverName()
			}
			return "none"
		}())

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

	// ANALYTICS + ALERT RULE CONTROLLERS
	// AlertRuleService is injected into AnalyticsService so checkAndWriteAlerts
	// uses dynamic DB-driven rules instead of hardcoded thresholds.
	if db != nil {
		analyticsCtrl := controllers.NewAnalyticsController(db)
		analyticsCtrl.RegisterRoutes(api.Group("/analytics"))
		log.Printf("✅ Analytics Controller initialized")

		alertRuleCtrl := controllers.NewAlertRuleController(db)
		alertRuleCtrl.RegisterRoutes(api.Group("/alerts"))
		analyticsCtrl.InjectAlertRuleService(alertRuleCtrl.Service())
		log.Printf("✅ Alert Rule Controller initialized")
	}

	// ── LOCAL AI SERVICE ─────────────────────────────────────────────────────
	// All inference runs on-premise via Ollama — no PHI leaves the network.
	// Routes are always registered; DB-backed features (RAG, memory) degrade
	// gracefully to no-op when db is nil so basic Ollama chat still works.
	{
		aiSvc := ai.NewAIService(db) // db may be nil — all sub-services guard against it
		schemaDir := cfg.GetSchemaDirectory()
		aiCtrl := controllers.NewAIController(aiSvc, schemaDir)
		aiCtrl.RegisterRoutes(api.Group("/ai"))

		log.Printf("✅ AI Service initialized (chat: %s, embed: %s, db: %v)",
			aiSvc.OllamaClient().ChatModel, aiSvc.OllamaClient().EmbedModel, db != nil)

		if db != nil {
			// Background ingestion: static schemas + operational data (interfaces, pipelines, errors).
			// Non-blocking — takes several minutes on first run.
			aiSvc.BackgroundIngestAll(schemaDir)

			// Telemetry: fire install ping once on first run (non-blocking, non-fatal).
			// Sends: install UUID, version, OS, admin email, timezone — never PHI.
			edition := os.Getenv("EHK_EDITION") // "community" | "enterprise" — set in docker-compose
			telSvc := services.NewTelemetryService(db, "1.0.0", edition)

			// ProductConfigService: encrypted product-internal config from DB.
			// Installer seeds real values; empty = feature disabled (community build).
			prodCfg := services.NewProductConfigService(db, credStore)
			telSvc.SetProductConfig(prodCfg)

			// Bootstrap: seed default values only if installer hasn't provided them.
			// These are the dev/community fallback values — replace via installer for production.
			go func() {
				ctx := context.Background()
				prodCfg.SeedIfEmpty(ctx,
					"telemetry_endpoint",
					"https://script.google.com/macros/s/AKfycbzh4wdZHEi2Wg2rc3wEnb08Lcr83tVj5amaxq43gQdwLmFctlKj9AxTsF_mp4azIVMZ/exec",
					"Telemetry receiver endpoint",
				)
				prodCfg.SeedIfEmpty(ctx,
					"telemetry_secret",
					"ehk-t3lem-v1-xK9mPqR7nW2sB4dL",
					"HMAC signing secret for telemetry payloads",
				)
			}()

			aiCtrl.SetTelemetry(telSvc)
			go telSvc.SendInstallPingIfNew(context.Background())
		}
	}

	// CODE TEMPLATE CONTROLLER
	if db != nil && codeTemplateSvc != nil {
		ctCtrl := controllers.NewCodeTemplateController(codeTemplateSvc)
		ctCtrl.RegisterRoutes(api.Group("/code-templates"))
		log.Printf("✅ Code Template Controller initialized")
	}

	// GIT INTEGRATION CONTROLLER
	if db != nil {
		gitCtrl := controllers.NewGitController(db, credStore)
		gitCtrl.RegisterRoutes(api.Group("/git"))
		log.Printf("✅ Git Integration Controller initialized")
	}

	// MIRTH MIGRATION CONTROLLER
	if db != nil {
		mirthCtrl := controllers.NewMirthMigrationController(db)
		mirthCtrl.RegisterRoutes(api.Group("/migration"))
		log.Printf("✅ Mirth Migration Controller initialized")
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

	// Explicitly bind to 0.0.0.0 (IPv4) so the Node.js proxy can reach us via
	// 127.0.0.1.  Gin's router.Run(":port") creates a dual-stack socket on most
	// systems, but on some Linux + Go 1.21+ configurations it binds IPv6-only.
	// Using "0.0.0.0:port" forces an IPv4 socket that the proxy can always reach.
	srv := &http.Server{
		Addr:    "0.0.0.0:" + port,
		Handler: router,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		log.Printf("Starting ezHealthKonnect server on 0.0.0.0:%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Failed to start server:", err)
		}
	}()

	<-quit
	log.Printf("Shutdown signal received — stopping connectors and shutting down...")

	if processingEngine != nil {
		if err := processingEngine.Stop(); err != nil {
			log.Printf("⚠️  Engine stop: %v", err)
		}
	}

	// HTTP server shutdown: allow extra 5 s for open HTTP connections to drain
	// after the pipeline engine has stopped (engine wait runs first above).
	httpShutdownTimeout := 15 * time.Second
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("⚠️  HTTP server forced shutdown: %v", err)
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
