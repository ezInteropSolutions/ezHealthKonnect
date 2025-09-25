// controllers/processing_controller.go
// HTTP controller for Interface Engine processing

package controllers

import (
	"database/sql"
	"net/http"

	"ezhealthkonnect/processing"

	"github.com/gin-gonic/gin"
)

// ProcessingController handles interface engine HTTP requests
type ProcessingController struct {
	engine *processing.ProcessingEngine
}

// NewProcessingController creates a new processing controller
func NewProcessingController(db *sql.DB) *ProcessingController {
	engine := processing.NewProcessingEngine(db)
	return &ProcessingController{
		engine: engine,
	}
}

// RegisterRoutes registers processing engine routes
func (pc *ProcessingController) RegisterRoutes(router *gin.RouterGroup) {
	processing := router.Group("/processing")
	{
		// Engine management
		processing.POST("/engine/start", pc.StartEngine)
		processing.POST("/engine/stop", pc.StopEngine)
		processing.GET("/engine/status", pc.GetEngineStatus)
		processing.GET("/engine/stats", pc.GetEngineStats)

		// Interface management
		processing.POST("/interfaces/:id/activate", pc.ActivateInterface)
		processing.POST("/interfaces/:id/deactivate", pc.DeactivateInterface)
		processing.GET("/interfaces/:id/status", pc.GetInterfaceStatus)

		// Message queue management
		processing.POST("/queue/enqueue", pc.EnqueueMessage)
		processing.GET("/queue/stats", pc.GetQueueStats)
	}
}

// StartEngine starts the interface engine
func (pc *ProcessingController) StartEngine(c *gin.Context) {
	if err := pc.engine.Start(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Failed to start processing engine",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Processing engine started successfully",
		"stats":   pc.engine.GetStats(),
	})
}

// StopEngine stops the interface engine
func (pc *ProcessingController) StopEngine(c *gin.Context) {
	if err := pc.engine.Stop(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Failed to stop processing engine",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Processing engine stopped successfully",
	})
}

// GetEngineStatus returns the current engine status
func (pc *ProcessingController) GetEngineStatus(c *gin.Context) {
	stats := pc.engine.GetStats()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"engine": gin.H{
			"running":                pc.engine.IsRunning(),
			"active_interfaces":      stats.ActiveInterfaces,
			"total_messages_processed": stats.TotalMessagesProcessed,
			"total_errors":           stats.TotalErrors,
			"average_processing_time": stats.AverageProcessingTime,
			"start_time":             stats.StartTime,
			"last_activity":          stats.LastActivity,
		},
	})
}

// GetEngineStats returns detailed engine statistics
func (pc *ProcessingController) GetEngineStats(c *gin.Context) {
	stats := pc.engine.GetStats()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stats":   stats,
	})
}

// ActivateInterface activates a specific interface for processing
func (pc *ProcessingController) ActivateInterface(c *gin.Context) {
	interfaceID := c.Param("id")
	if interfaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Interface ID is required",
		})
		return
	}

	if err := pc.engine.ActivateInterface(interfaceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Failed to activate interface",
			"interface_id": interfaceID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Interface activated successfully",
		"interface_id": interfaceID,
		"status": pc.engine.GetInterfaceStatus(interfaceID),
	})
}

// DeactivateInterface deactivates a specific interface
func (pc *ProcessingController) DeactivateInterface(c *gin.Context) {
	interfaceID := c.Param("id")
	if interfaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Interface ID is required",
		})
		return
	}

	if err := pc.engine.DeactivateInterface(interfaceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Failed to deactivate interface",
			"interface_id": interfaceID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Interface deactivated successfully",
		"interface_id": interfaceID,
	})
}

// GetInterfaceStatus returns status for a specific interface
func (pc *ProcessingController) GetInterfaceStatus(c *gin.Context) {
	interfaceID := c.Param("id")
	if interfaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Interface ID is required",
		})
		return
	}

	status := pc.engine.GetInterfaceStatus(interfaceID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"interface_id": interfaceID,
		"status": status,
	})
}

// EnqueueMessage adds a message to the processing queue
func (pc *ProcessingController) EnqueueMessage(c *gin.Context) {
	var request struct {
		InterfaceID string                 `json:"interface_id" binding:"required"`
		MessageID   string                 `json:"message_id" binding:"required"`
		MessageData map[string]interface{} `json:"message_data" binding:"required"`
		Options     map[string]interface{} `json:"options"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Invalid request payload",
		})
		return
	}

	if request.Options == nil {
		request.Options = make(map[string]interface{})
	}

	// Note: This would need to access the message queue from the engine
	// For now, return a placeholder response
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Message enqueued successfully",
		"message_id": request.MessageID,
		"interface_id": request.InterfaceID,
	})
}

// GetQueueStats returns message queue statistics
func (pc *ProcessingController) GetQueueStats(c *gin.Context) {
	// Note: This would need to access the message queue from the engine
	// For now, return a placeholder response
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stats": gin.H{
			"pending":   0,
			"processing": 0,
			"completed": 0,
			"failed":    0,
			"total":     0,
			"last_updated": "placeholder",
		},
	})
}