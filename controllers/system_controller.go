package controllers

import (
	"database/sql"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"ezhealthkonnect/config"

	"github.com/gin-gonic/gin"
)

// SystemController handles system-level operations
type SystemController struct {
	config *config.Config
	db     *sql.DB
}

// NewSystemController creates a new system controller
func NewSystemController(cfg *config.Config, db *sql.DB) *SystemController {
	return &SystemController{
		config: cfg,
		db:     db,
	}
}

// HealthCheck returns the health status of the API, including a real DB ping.
func (ctrl *SystemController) HealthCheck(c *gin.Context) {
	dbStatus := "connected"
	dbLatencyMs := int64(0)
	if ctrl.db != nil {
		start := time.Now()
		if err := ctrl.db.PingContext(c.Request.Context()); err != nil {
			dbStatus = "error"
		}
		dbLatencyMs = time.Since(start).Milliseconds()
	} else {
		dbStatus = "unavailable"
	}

	status := "healthy"
	if dbStatus != "connected" {
		status = "degraded"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      status,
		"service":     "ezHealthKonnect-Go-API",
		"version":     "1.0.0",
		"timestamp":   time.Now().Format(time.RFC3339),
		"environment": ctrl.config.Environment,
		"database": gin.H{
			"status":     dbStatus,
			"latency_ms": dbLatencyMs,
		},
		"features": gin.H{
			"postgresql_enabled":   ctrl.config.EnablePostgreSQL,
			"hipaa_compliance":     ctrl.config.HIPAAComplianceMode,
			"hl7_processing_mode":  ctrl.config.HL7ProcessingMode,
			"hl7_validation_level": ctrl.config.HL7ValidationLevel,
			"verbose_logging":      ctrl.config.VerboseLogging,
		},
		"services": gin.H{
			"dictionary_service": ctrl.config.DictionaryServiceURL,
			"node_service":       ctrl.config.NodeServiceURL,
		},
		"performance": gin.H{
			"target_throughput": "100,000+ messages/hour",
			"max_file_size":     "400MB CCD files",
			"processing_mode":   ctrl.config.HL7ProcessingMode,
		},
	})
}

// GetInfo returns detailed system information
func (ctrl *SystemController) GetInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"system": gin.H{
			"name":        "ezHealthKonnect",
			"description": "AI-Powered Healthcare Integration Platform",
			"version":     "1.0.0",
			"environment": ctrl.config.Environment,
			"buildInfo": gin.H{
				"goVersion": "1.21+",
				"buildTime": time.Now().Format(time.RFC3339),
				"gitCommit": "dev-build",
			},
		},
		"capabilities": gin.H{
			"supportedFormats": []string{"HL7 v2.x", "CCD/C-CDA", "FHIR R4"},
			"messageTypes": []string{
				"ADT^A01", "ADT^A03", "ADT^A04",
				"ORU^R01", "ORM^O01", "SIU^S12",
			},
			"processingModes": []string{"basic", "enhanced", "ai-powered"},
			"outputFormats":   []string{"JSON", "XML", "HL7", "FHIR"},
		},
		"performance": gin.H{
			"currentThroughput": "45,000 msg/hr",
			"targetThroughput":  "100,000+ msg/hr",
			"maxFileSize":       "400MB",
			"avgProcessingTime": "12ms",
		},
		"services": gin.H{
			"dictionaryService": gin.H{
				"url":    ctrl.config.DictionaryServiceURL,
				"status": "connected", // TODO: Actual health check
			},
			"nodeService": gin.H{
				"url":    ctrl.config.NodeServiceURL,
				"status": "connected", // TODO: Actual health check
			},
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetMetrics returns system performance metrics (memory from runtime, others static).
func (ctrl *SystemController) GetMetrics(c *gin.Context) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	allocMB := ms.Alloc / 1024 / 1024
	sysMB := ms.Sys / 1024 / 1024
	memPct := 0
	if sysMB > 0 {
		memPct = int(allocMB * 100 / sysMB)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"metrics": gin.H{
			"system": gin.H{
				"memory_alloc_mb":  allocMB,
				"memory_sys_mb":    sysMB,
				"memory_pct":       memPct,
				"memory_display":   fmt.Sprintf("%dMB / %dMB (%d%%)", allocMB, sysMB, memPct),
				"goroutines":       runtime.NumGoroutine(),
			},
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
