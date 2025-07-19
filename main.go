package main

import (
	"database/sql" // ADD: For FHIR transformations
	"ezhealthkonnect/config"
	"ezhealthkonnect/controllers"
	"ezhealthkonnect/fhir"
	"ezhealthkonnect/hl7"
	"fmt"
	"log"
	"net/http"
	"os" // ADD: For environment variables
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq" // ADD: PostgreSQL driver for FHIR transformations
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
				"database":        db != nil, // ADD: Database status
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

			// FHIR Schema Management Routes
			fhirGroup := api.Group("/fhir")
			{
				fhirSchemaGroup := fhirGroup.Group("/schema")
				{
					fhirSchemaGroup.GET("/status", func(c *gin.Context) {
						fhirLoader := fhir.GetFHIRSchemaLoader()
						if fhirLoader == nil {
							c.JSON(http.StatusServiceUnavailable, gin.H{
								"success": false,
								"error":   "FHIR schema system not initialized",
								"system":  "fhir",
							})
							return
						}

						stats := fhirLoader.GetStats()
						available, err := fhirLoader.ListAvailableSchemas()
						if err != nil {
							available = []string{} // Empty slice if error
						}

						c.JSON(http.StatusOK, gin.H{
							"success": true,
							"system": gin.H{
								"name":              "ezHealthKonnect FHIR Schema System",
								"source":            "filesystem",
								"directory":         cfg.GetFHIRSchemaDirectory(),
								"defaultVersion":    cfg.DefaultFHIRVersion,
								"defaultProfile":    cfg.DefaultFHIRProfile,
								"cacheEnabled":      cfg.EnableFHIRCache,
								"maxCacheSize":      cfg.FHIRCacheSize,
								"supportedVersions": []string{"R4", "R5"},
								"supportedProfiles": []string{"base", "us-core"},
							},
							"statistics": gin.H{
								"totalLoads":   stats.TotalLoads,
								"cacheHits":    stats.CacheHits,
								"cacheMisses":  stats.CacheMisses,
								"loadErrors":   stats.LoadErrors,
								"cacheSize":    stats.CacheSize,
								"lastLoaded":   stats.LastLoaded,
								"lastLoadTime": stats.LastLoadTime,
							},
							"available": gin.H{
								"schemas": available,
								"count":   len(available),
							},
							"performance": gin.H{
								"cacheHitRatio": func() float64 {
									total := stats.CacheHits + stats.CacheMisses
									if total == 0 {
										return 0.0
									}
									return float64(stats.CacheHits) / float64(total) * 100.0
								}(),
								"errorRate": func() float64 {
									if stats.TotalLoads == 0 {
										return 0.0
									}
									return float64(stats.LoadErrors) / float64(stats.TotalLoads) * 100.0
								}(),
							},
							"timestamp": time.Now().Format(time.RFC3339),
						})
					})

					fhirSchemaGroup.GET("/available", func(c *gin.Context) {
						fhirLoader := fhir.GetFHIRSchemaLoader()
						if fhirLoader == nil {
							c.JSON(http.StatusServiceUnavailable, gin.H{
								"success": false,
								"error":   "FHIR schema system not initialized",
							})
							return
						}

						available, err := fhirLoader.ListAvailableSchemas()
						if err != nil {
							c.JSON(http.StatusInternalServerError, gin.H{
								"success": false,
								"error":   "Failed to list FHIR schemas: " + err.Error(),
							})
							return
						}

						// Parse available schemas into structured format
						schemasDetails := make([]gin.H, 0, len(available))
						for _, schema := range available {
							// Parse schema string: R4_base_Patient -> version: R4, profile: base, resource: Patient
							parts := strings.Split(schema, "_")
							if len(parts) >= 3 {
								schemasDetails = append(schemasDetails, gin.H{
									"id":           schema,
									"version":      parts[0],
									"profile":      parts[1],
									"resourceType": parts[2],
									"displayName":  fmt.Sprintf("%s %s (%s)", parts[2], parts[1], parts[0]),
								})
							}
						}

						c.JSON(http.StatusOK, gin.H{
							"success": true,
							"schemas": schemasDetails,
							"summary": gin.H{
								"total": len(available),
								"byVersion": gin.H{
									"R4": func() int {
										count := 0
										for _, schema := range available {
											if strings.HasPrefix(schema, "R4_") {
												count++
											}
										}
										return count
									}(),
								},
								"byProfile": gin.H{
									"base": func() int {
										count := 0
										for _, schema := range available {
											if strings.Contains(schema, "_base_") {
												count++
											}
										}
										return count
									}(),
									"us-core": func() int {
										count := 0
										for _, schema := range available {
											if strings.Contains(schema, "_us-core_") {
												count++
											}
										}
										return count
									}(),
								},
							},
							"timestamp": time.Now().Format(time.RFC3339),
						})
					})

					fhirSchemaGroup.POST("/load", func(c *gin.Context) {
						var request struct {
							ResourceType string `json:"resourceType" binding:"required"`
							Profile      string `json:"profile"`
							Version      string `json:"version"`
							ForceReload  bool   `json:"forceReload"`
						}

						if err := c.ShouldBindJSON(&request); err != nil {
							c.JSON(http.StatusBadRequest, gin.H{
								"success": false,
								"error":   "Invalid request: " + err.Error(),
							})
							return
						}

						fhirLoader := fhir.GetFHIRSchemaLoader()
						if fhirLoader == nil {
							c.JSON(http.StatusServiceUnavailable, gin.H{
								"success": false,
								"error":   "FHIR schema system not initialized",
							})
							return
						}

						// Set defaults
						if request.Version == "" {
							request.Version = cfg.DefaultFHIRVersion
						}
						if request.Profile == "" {
							request.Profile = cfg.DefaultFHIRProfile
						}

						// Force reload by clearing cache if requested
						if request.ForceReload {
							fhirLoader.ClearCache()
						}

						startLoadTime := time.Now()
						schema, err := fhirLoader.LoadFHIRSchema(request.ResourceType, request.Profile, request.Version)
						loadTime := time.Since(startLoadTime)

						if err != nil {
							c.JSON(http.StatusNotFound, gin.H{
								"success": false,
								"error":   "Failed to load FHIR schema: " + err.Error(),
								"request": gin.H{
									"resourceType": request.ResourceType,
									"profile":      request.Profile,
									"version":      request.Version,
								},
							})
							return
						}

						c.JSON(http.StatusOK, gin.H{
							"success": true,
							"schema": gin.H{
								"resourceType":     schema.ResourceType,
								"version":          schema.Version,
								"name":             schema.Name,
								"title":            schema.Title,
								"description":      schema.Description,
								"baseResource":     schema.BaseResource,
								"profile":          schema.Profile,
								"elementCount":     len(schema.Elements),
								"requiredCount":    len(schema.Required),
								"mustSupportCount": len(schema.MustSupport),
								"loadedAt":         schema.LoadedAt,
								"sourceFile":       schema.SourceFile,
							},
							"performance": gin.H{
								"loadTime":  loadTime.String(),
								"fromCache": loadTime < time.Millisecond, // Quick heuristic
							},
							"timestamp": time.Now().Format(time.RFC3339),
						})
					})

					fhirSchemaGroup.POST("/cache/clear", func(c *gin.Context) {
						fhirLoader := fhir.GetFHIRSchemaLoader()
						if fhirLoader == nil {
							c.JSON(http.StatusServiceUnavailable, gin.H{
								"success": false,
								"error":   "FHIR schema system not initialized",
							})
							return
						}

						oldStats := fhirLoader.GetStats()
						fhirLoader.ClearCache()
						newStats := fhirLoader.GetStats()

						c.JSON(http.StatusOK, gin.H{
							"success": true,
							"message": "FHIR schema cache cleared successfully",
							"system":  "fhir",
							"cleared": gin.H{
								"schemasRemoved": oldStats.CacheSize,
								"cacheHits":      oldStats.CacheHits,
								"cacheMisses":    oldStats.CacheMisses,
							},
							"newStats": gin.H{
								"cacheSize":  newStats.CacheSize,
								"totalLoads": newStats.TotalLoads,
							},
							"timestamp": time.Now().Format(time.RFC3339),
						})
					})
				}

				// Existing FHIR validation endpoint
				fhirGroup.POST("/validate", func(c *gin.Context) {
					c.JSON(http.StatusOK, gin.H{
						"success": true,
						"message": "FHIR validation endpoint (to be implemented)",
						"note":    "This will validate FHIR resources against loaded schemas",
					})
				})

				// ADD: FHIR Transformation endpoints
				if db != nil {
					//fhirCtrl := controllers.NewFHIRTransformController(db, cfg)
					schemaFhirCtrl := controllers.NewSchemaFHIRTransformController(db, cfg)
					schemaFhirCtrl.RegisterRoutes(api)
					log.Printf("✅ FHIR transformation endpoints enabled at /api/fhir/transform")
				} else {
					// Provide helpful endpoint when database not configured
					fhirGroup.GET("/transform/status", func(c *gin.Context) {
						c.JSON(http.StatusServiceUnavailable, gin.H{
							"status":  "disabled",
							"error":   "Database not configured",
							"message": "FHIR transformations require database connection",
							"setup": gin.H{
								"step1":   "Set DATABASE_URL environment variable",
								"step2":   "Create hl7_fhir_mappings table",
								"step3":   "Restart server",
								"example": "export DATABASE_URL='postgres://user:pass@localhost:5432/db?sslmode=disable'",
							},
						})
					})
					log.Printf("⚠️ FHIR transformation endpoints disabled (no database connection)")
				}
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
						"databaseReady":      db != nil, // ADD: Database status
						"fhirTransformReady": db != nil, // ADD: FHIR transform status
					},
					"message": "Transformation system status",
				})
			})
		}

		// Interface management routes (existing - maintained)
		interfaceGroup := api.Group("/interfaces")
		{
			interfaceGroup.POST("", func(c *gin.Context) {
				// Your existing interface creation logic
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": "Interface creation endpoint",
				})
			})
		}
	}

	// ADD: Clean up database connection on exit
	if db != nil {
		defer db.Close()
	}

	// Start server
	address := fmt.Sprintf(":%s", cfg.Port)

	log.Printf("🚀 Starting ezHealthKonnect API server on port %s", cfg.Port)
	log.Printf("🌐 Environment: %s", cfg.Environment)
	log.Printf("📋 Schema Source: %s", cfg.SchemaSource)

	if cfg.UseFilesystemSchema() {
		log.Printf("📂 HL7 Schema Directory: %s", cfg.GetSchemaDirectory())
		log.Printf("🏥 FHIR Schema Directory: %s", cfg.GetFHIRSchemaDirectory())
	}

	log.Printf("🔗 Health check: http://localhost:%s/health", cfg.Port)
	log.Printf("📊 HL7 endpoints: http://localhost:%s/api/hl7/*", cfg.Port)
	log.Printf("🏥 FHIR endpoints: http://localhost:%s/api/fhir/*", cfg.Port)
	log.Printf("🔄 Transform endpoints: http://localhost:%s/api/transform/*", cfg.Port)

	// ADD: Log FHIR transformation endpoints
	if db != nil {
		log.Printf("⚡ FHIR Transform Status: http://localhost:%s/api/fhir/transform/status", cfg.Port)
		log.Printf("📋 FHIR Transform Rules: http://localhost:%s/api/fhir/transform/rules?messageType=ADT^A01", cfg.Port)
		log.Printf("🔄 FHIR Transform: http://localhost:%s/api/fhir/transform", cfg.Port)
	}

	if err := router.Run(address); err != nil {
		log.Fatalf("❌ Failed to start server: %v", err)
	}
}
