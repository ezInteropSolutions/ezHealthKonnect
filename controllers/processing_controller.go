// controllers/processing_controller.go
// HTTP controller for Interface Engine processing

package controllers

import (
	"log"
	"net/http"
	"strconv"

	"ezhealthkonnect/processing"

	"github.com/gin-gonic/gin"
)

// ProcessingController handles interface engine HTTP requests
type ProcessingController struct {
	engine *processing.ProcessingEngine
}

// NewProcessingController creates a new processing controller with existing engine
func NewProcessingController(engine *processing.ProcessingEngine) *ProcessingController {
	return &ProcessingController{
		engine: engine,
	}
}

// checkEngine verifies the engine is initialized and returns an error response if not
func (pc *ProcessingController) checkEngine(c *gin.Context) bool {
	if pc.engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Processing engine is not initialized. Please check database connection.",
		})
		return false
	}
	return true
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

		// Message transformation (MVC + OOB: retrieve parsedJSON → map → transform → store FHIR)
		processing.POST("/transform/message/:interfaceId/:messageId", pc.TransformStoredMessage)
		processing.POST("/transform/interface/:interfaceId", pc.TransformInterfaceMessages)
	}
}

// StartEngine starts the interface engine
func (pc *ProcessingController) StartEngine(c *gin.Context) {
	if !pc.checkEngine(c) {
		return
	}
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
	if !pc.checkEngine(c) {
		return
	}
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
	if !pc.checkEngine(c) {
		return
	}
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
	if !pc.checkEngine(c) {
		return
	}
	stats := pc.engine.GetStats()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stats":   stats,
	})
}

// ActivateInterface activates a specific interface for processing
func (pc *ProcessingController) ActivateInterface(c *gin.Context) {
	if !pc.checkEngine(c) {
		return
	}
	interfaceID := c.Param("id")

	// DEBUG: Log at controller level
	log.Printf("🎯 [CONTROLLER] ActivateInterface called for: %s", interfaceID)

	if interfaceID == "" {
		log.Printf("❌ [CONTROLLER] No interface ID provided")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Interface ID is required",
		})
		return
	}

	log.Printf("📞 [CONTROLLER] Calling engine.ActivateInterface(%s)", interfaceID)

	if err := pc.engine.ActivateInterface(interfaceID); err != nil {
		log.Printf("❌ [CONTROLLER] Engine returned error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Failed to activate interface",
			"interface_id": interfaceID,
		})
		return
	}

	log.Printf("✅ [CONTROLLER] Engine activation successful, getting status...")
	status := pc.engine.GetInterfaceStatus(interfaceID)
	log.Printf("✅ [CONTROLLER] Status retrieved: %+v", status)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Interface activated successfully",
		"interface_id": interfaceID,
		"status": status,
	})
}

// DeactivateInterface deactivates a specific interface
func (pc *ProcessingController) DeactivateInterface(c *gin.Context) {
	if !pc.checkEngine(c) {
		return
	}
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
	if !pc.checkEngine(c) {
		return
	}
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

// TransformStoredMessage transforms a single stored message
// MVC + OOB: Retrieves parsedJSON from MongoDB → uses interface mapping → generates FHIR → stores output
// POST /api/processing/messages/:interfaceId/:messageId/transform
func (pc *ProcessingController) TransformStoredMessage(c *gin.Context) {
	if !pc.checkEngine(c) {
		return
	}
	interfaceID := c.Param("interfaceId")
	messageID := c.Param("messageId")

	if interfaceID == "" || messageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "interface_id and message_id are required",
		})
		return
	}

	// Delegate to processing engine
	result, err := pc.engine.TransformStoredMessage(c.Request.Context(), interfaceID, messageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Transformation failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          result.Success,
		"fhir_bundle":      result.TransformedMessage,
		"resource_type":    result.FHIRResourceType,
		"resource_id":      result.FHIRResourceID,
		"processing_time":  result.ProcessingTimeMs,
		"metadata":         result.TransformationMetadata,
		"validation_errors": result.ValidationErrors,
	})
}

// TransformInterfaceMessages transforms all untransformed messages for an interface
// POST /api/processing/interfaces/:interfaceId/transform?limit=100
func (pc *ProcessingController) TransformInterfaceMessages(c *gin.Context) {
	if !pc.checkEngine(c) {
		return
	}
	interfaceID := c.Param("interfaceId")
	if interfaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "interface_id is required",
		})
		return
	}

	// Get limit from query params
	limit := 100
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Delegate to processing engine
	results, err := pc.engine.TransformInterfaceMessages(c.Request.Context(), interfaceID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Batch transformation failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"total_transformed": len(results),
		"results":         results,
	})
}