// controllers/connectivity_controller.go
// Multi-Connectivity Controller - REST API for OOB Connectors

package controllers

import (
	"database/sql"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type ConnectivityController struct {
	service *services.ConnectivityService
}

func NewConnectivityController(db *sql.DB, credStore *services.CredentialStore) *ConnectivityController {
	return &ConnectivityController{
		service: services.NewConnectivityService(db, credStore),
	}
}

// ==================== CONNECTIVITY TYPES (OOB Connectors) ====================

// ListConnectivityTypes godoc
// @Summary List all available connectivity types
// @Description Get list of OOB connector definitions with optional filtering
// @Tags connectivity
// @Accept json
// @Produce json
// @Param category query string false "Filter by category (inbound/outbound)"
// @Param mode query string false "Filter by mode (push/pull/stream)"
// @Param supports_cron query bool false "Filter by cron support"
// @Param is_active query bool false "Filter by active status"
// @Success 200 {object} map[string]interface{}
// @Router /api/connectivity/types [get]
func (cc *ConnectivityController) ListConnectivityTypes(c *gin.Context) {
	filter := &models.ConnectivityTypeFilter{}

	// Parse query parameters
	if category := c.Query("category"); category != "" {
		filter.Category = category
	}
	if mode := c.Query("mode"); mode != "" {
		filter.Mode = mode
	}
	if supportsCron := c.Query("supports_cron"); supportsCron != "" {
		val := supportsCron == "true"
		filter.SupportsCron = &val
	}
	if isActive := c.Query("is_active"); isActive != "" {
		val := isActive == "true"
		filter.IsActive = &val
	}
	if isBeta := c.Query("is_beta"); isBeta != "" {
		val := isBeta == "true"
		filter.IsBeta = &val
	}

	types, err := cc.service.ListConnectivityTypes(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to list connectivity types",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types,
		"count":   len(types),
	})
}

// GetConnectivityType godoc
// @Summary Get connectivity type by ID or name
// @Description Get detailed information about a specific connector type
// @Tags connectivity
// @Accept json
// @Produce json
// @Param identifier path string true "Type ID or type_name"
// @Success 200 {object} map[string]interface{}
// @Router /api/connectivity/types/{identifier} [get]
func (cc *ConnectivityController) GetConnectivityType(c *gin.Context) {
	identifier := c.Param("identifier")

	// Try by ID first
	connType, err := cc.service.GetConnectivityTypeByID(identifier)
	if err != nil {
		// Try by name
		connType, err = cc.service.GetConnectivityTypeByName(identifier)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Connectivity type not found",
				"details": err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    connType,
	})
}

// GetConnectivityTypesByCategory godoc
// @Summary Get connectivity types by category
// @Description Get all inbound or outbound connectors
// @Tags connectivity
// @Accept json
// @Produce json
// @Param category path string true "Category (inbound/outbound)"
// @Success 200 {object} map[string]interface{}
// @Router /api/connectivity/types/category/{category} [get]
func (cc *ConnectivityController) GetConnectivityTypesByCategory(c *gin.Context) {
	category := c.Param("category")

	if category != "inbound" && category != "outbound" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid category. Must be 'inbound' or 'outbound'",
		})
		return
	}

	filter := &models.ConnectivityTypeFilter{
		Category: category,
	}
	active := true
	filter.IsActive = &active

	types, err := cc.service.ListConnectivityTypes(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to list connectivity types",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types,
		"count":   len(types),
	})
}

// ==================== INTERFACE CONNECTIVITY ====================

// CreateInterfaceConnectivity godoc
// @Summary Configure connectivity for an interface
// @Description Set up source (inbound) and target (outbound) connectivity
// @Tags connectivity
// @Accept json
// @Produce json
// @Param interface_id path string true "Interface ID"
// @Param body body models.InterfaceConnectivity true "Connectivity configuration"
// @Success 201 {object} map[string]interface{}
// @Router /api/connectivity/interfaces/{interface_id} [post]
func (cc *ConnectivityController) CreateInterfaceConnectivity(c *gin.Context) {
	interfaceID := c.Param("interface_id")

	var connectivity models.InterfaceConnectivity
	if err := c.ShouldBindJSON(&connectivity); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body",
			"details": err.Error(),
		})
		return
	}

	connectivity.InterfaceID = interfaceID

	if err := cc.service.CreateInterfaceConnectivity(&connectivity); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create interface connectivity",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Interface connectivity created successfully",
		"data":    connectivity,
	})
}

// GetInterfaceConnectivity godoc
// @Summary Get connectivity configuration for an interface
// @Description Get full connectivity details including source, target, and cron job
// @Tags connectivity
// @Accept json
// @Produce json
// @Param interface_id path string true "Interface ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/connectivity/interfaces/{interface_id} [get]
func (cc *ConnectivityController) GetInterfaceConnectivity(c *gin.Context) {
	interfaceID := c.Param("interface_id")

	connectivity, err := cc.service.GetInterfaceConnectivity(interfaceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Interface connectivity not found",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    connectivity,
	})
}

// UpdateInterfaceConnectivity godoc
// @Summary Update connectivity configuration
// @Description Update source/target connectors or cron settings
// @Tags connectivity
// @Accept json
// @Produce json
// @Param interface_id path string true "Interface ID"
// @Param body body models.InterfaceConnectivity true "Updated connectivity"
// @Success 200 {object} map[string]interface{}
// @Router /api/connectivity/interfaces/{interface_id} [put]
func (cc *ConnectivityController) UpdateInterfaceConnectivity(c *gin.Context) {
	interfaceID := c.Param("interface_id")

	var connectivity models.InterfaceConnectivity
	if err := c.ShouldBindJSON(&connectivity); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body",
			"details": err.Error(),
		})
		return
	}

	connectivity.InterfaceID = interfaceID

	if err := cc.service.UpdateInterfaceConnectivity(&connectivity); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update interface connectivity",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Interface connectivity updated successfully",
		"data":    connectivity,
	})
}

// DeleteInterfaceConnectivity godoc
// @Summary Delete interface connectivity
// @Description Remove all connectivity configuration for an interface
// @Tags connectivity
// @Accept json
// @Produce json
// @Param interface_id path string true "Interface ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/connectivity/interfaces/{interface_id} [delete]
func (cc *ConnectivityController) DeleteInterfaceConnectivity(c *gin.Context) {
	interfaceID := c.Param("interface_id")

	if err := cc.service.DeleteInterfaceConnectivity(interfaceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete interface connectivity",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Interface connectivity deleted successfully",
	})
}

// ==================== CONNECTION TESTING ====================

// TestSourceConnection godoc
// @Summary Test inbound connection
// @Description Validate source connector configuration
// @Tags connectivity
// @Accept json
// @Produce json
// @Param interface_id path string true "Interface ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/connectivity/interfaces/{interface_id}/test-source [post]
func (cc *ConnectivityController) TestSourceConnection(c *gin.Context) {
	interfaceID := c.Param("interface_id")

	// TODO: Implement actual connection testing logic
	// This is a placeholder for the actual implementation

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Source connection test initiated",
		"interface_id": interfaceID,
		"status": "testing",
		"note": "Connection testing will be implemented in Phase 2",
	})
}

// TestTargetConnection godoc
// @Summary Test outbound connection
// @Description Validate target connector configuration
// @Tags connectivity
// @Accept json
// @Produce json
// @Param interface_id path string true "Interface ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/connectivity/interfaces/{interface_id}/test-target [post]
func (cc *ConnectivityController) TestTargetConnection(c *gin.Context) {
	interfaceID := c.Param("interface_id")

	// TODO: Implement actual connection testing logic
	// This is a placeholder for the actual implementation

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Target connection test initiated",
		"interface_id": interfaceID,
		"status": "testing",
		"note": "Connection testing will be implemented in Phase 2",
	})
}

// ==================== CRON JOB MANAGEMENT ====================

// EnableCronJob godoc
// @Summary Enable cron scheduling for an interface
// @Description Activate scheduled polling for pull-based connectors
// @Tags connectivity
// @Accept json
// @Produce json
// @Param interface_id path string true "Interface ID"
// @Param body body models.CronSchedule true "Cron configuration"
// @Success 200 {object} map[string]interface{}
// @Router /api/connectivity/interfaces/{interface_id}/cron/enable [post]
func (cc *ConnectivityController) EnableCronJob(c *gin.Context) {
	interfaceID := c.Param("interface_id")

	var cronSchedule models.CronSchedule
	if err := c.ShouldBindJSON(&cronSchedule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid cron schedule",
			"details": err.Error(),
		})
		return
	}

	// Get connectivity configuration
	connectivity, err := cc.service.GetInterfaceConnectivity(interfaceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Interface connectivity not found",
		})
		return
	}

	// Update cron settings
	connectivity.CronEnabled = true
	connectivity.CronExpression = cronSchedule.Expression
	connectivity.CronTimezone = cronSchedule.Timezone

	if err := cc.service.UpdateInterfaceConnectivity(&connectivity.InterfaceConnectivity); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to enable cron job",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Cron job enabled successfully",
		"cron_expression": cronSchedule.Expression,
		"human_readable": cronSchedule.HumanReadable,
	})
}

// DisableCronJob godoc
// @Summary Disable cron scheduling
// @Description Deactivate scheduled polling
// @Tags connectivity
// @Accept json
// @Produce json
// @Param interface_id path string true "Interface ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/connectivity/interfaces/{interface_id}/cron/disable [post]
func (cc *ConnectivityController) DisableCronJob(c *gin.Context) {
	interfaceID := c.Param("interface_id")

	connectivity, err := cc.service.GetInterfaceConnectivity(interfaceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Interface connectivity not found",
		})
		return
	}

	connectivity.CronEnabled = false

	if err := cc.service.UpdateInterfaceConnectivity(&connectivity.InterfaceConnectivity); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to disable cron job",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Cron job disabled successfully",
	})
}

// GetCronJobStatus godoc
// @Summary Get cron job status
// @Description Get current cron job state including circuit breaker status
// @Tags connectivity
// @Accept json
// @Produce json
// @Param interface_id path string true "Interface ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/connectivity/interfaces/{interface_id}/cron/status [get]
func (cc *ConnectivityController) GetCronJobStatus(c *gin.Context) {
	interfaceID := c.Param("interface_id")

	connectivity, err := cc.service.GetInterfaceConnectivity(interfaceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Interface connectivity not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"cron_enabled":     connectivity.CronEnabled,
			"cron_expression":  connectivity.CronExpression,
			"cron_timezone":    connectivity.CronTimezone,
			"next_run_at":      connectivity.NextRunAt,
			"last_run_at":      connectivity.LastRunAt,
			"last_run_status":  connectivity.LastRunStatus,
			"cron_job_details": connectivity.CronJob,
		},
	})
}

// ==================== EXECUTION LOGS ====================

// GetExecutionLogs godoc
// @Summary Get execution history
// @Description Get execution logs with filtering and pagination
// @Tags connectivity
// @Accept json
// @Produce json
// @Param interface_id path string true "Interface ID"
// @Param execution_type query string false "Filter by execution type"
// @Param status query string false "Filter by status"
// @Param limit query int false "Limit results (default 50)"
// @Param offset query int false "Offset for pagination"
// @Success 200 {object} map[string]interface{}
// @Router /api/connectivity/interfaces/{interface_id}/executions [get]
func (cc *ConnectivityController) GetExecutionLogs(c *gin.Context) {
	interfaceID := c.Param("interface_id")

	filter := &models.ExecutionLogFilter{
		InterfaceID: interfaceID,
		Limit:       50,
		Offset:      0,
	}

	// Parse query parameters
	if executionType := c.Query("execution_type"); executionType != "" {
		filter.ExecutionType = executionType
	}
	if status := c.Query("status"); status != "" {
		filter.Status = status
	}
	if limit := c.Query("limit"); limit != "" {
		if val, err := strconv.Atoi(limit); err == nil {
			filter.Limit = val
		}
	}
	if offset := c.Query("offset"); offset != "" {
		if val, err := strconv.Atoi(offset); err == nil {
			filter.Offset = val
		}
	}

	logs, err := cc.service.GetExecutionLogs(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get execution logs",
			"details": err.Error(),
		})
		return
	}

	stats, err := cc.service.GetExecutionStats(interfaceID)
	if err != nil {
		// Don't fail if stats retrieval fails
		stats = nil
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    logs,
		"count":   len(logs),
		"stats":   stats,
	})
}

// GetExecutionStats godoc
// @Summary Get execution statistics
// @Description Get aggregated execution statistics for an interface
// @Tags connectivity
// @Accept json
// @Produce json
// @Param interface_id path string true "Interface ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/connectivity/interfaces/{interface_id}/executions/stats [get]
func (cc *ConnectivityController) GetExecutionStats(c *gin.Context) {
	interfaceID := c.Param("interface_id")

	stats, err := cc.service.GetExecutionStats(interfaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get execution stats",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// ==================== HELPER: CRON EXPRESSION PREVIEW ====================

// PreviewCronSchedule godoc
// @Summary Preview cron schedule
// @Description Get next N execution times for a cron expression
// @Tags connectivity
// @Accept json
// @Produce json
// @Param body body models.CronSchedule true "Cron expression"
// @Success 200 {object} map[string]interface{}
// @Router /api/connectivity/cron/preview [post]
func (cc *ConnectivityController) PreviewCronSchedule(c *gin.Context) {
	var cronSchedule models.CronSchedule
	if err := c.ShouldBindJSON(&cronSchedule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid cron schedule",
			"details": err.Error(),
		})
		return
	}

	// TODO: Implement actual cron expression parsing and preview
	// For now, return placeholder next runs
	nextRuns := []time.Time{
		time.Now().Add(5 * time.Minute),
		time.Now().Add(10 * time.Minute),
		time.Now().Add(15 * time.Minute),
		time.Now().Add(20 * time.Minute),
		time.Now().Add(25 * time.Minute),
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"expression":     cronSchedule.Expression,
			"timezone":       cronSchedule.Timezone,
			"next_runs":      nextRuns,
			"human_readable": cronSchedule.HumanReadable,
		},
	})
}
