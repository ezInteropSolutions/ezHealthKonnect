// controllers/output_message_controller.go
// HTTP controller for output message management and retrieval

package controllers

import (
	"database/sql"
	"net/http"
	"strconv"

	"ezhealthkonnect/services"

	"github.com/gin-gonic/gin"
)

// OutputMessageController handles output message HTTP requests
type OutputMessageController struct {
	service *services.OutputMessageService
}

// NewOutputMessageController creates a new output message controller
func NewOutputMessageController(db *sql.DB) *OutputMessageController {
	// Create service with PostgreSQL only (MongoDB optional)
	service := services.NewOutputMessageService(db, nil, "")
	return &OutputMessageController{
		service: service,
	}
}

// RegisterRoutes registers output message routes
func (omc *OutputMessageController) RegisterRoutes(router *gin.RouterGroup) {
	output := router.Group("/output")
	{
		// Message retrieval
		output.GET("/interfaces/:interfaceId/messages", omc.GetInterfaceOutputMessages)
		output.GET("/interfaces/:interfaceId/messages/:inputMessageId", omc.GetOutputMessage)

		// Statistics and analytics
		output.GET("/interfaces/:interfaceId/stats", omc.GetInterfaceOutputStats)
		output.GET("/interfaces/:interfaceId/count", omc.GetOutputMessageCount)

		// Delivery management
		output.PUT("/interfaces/:interfaceId/messages/:inputMessageId/delivery", omc.UpdateDeliveryStatus)
		output.POST("/interfaces/:interfaceId/messages/:inputMessageId/redeliver", omc.RedeliverMessage)

		// Administrative
		output.GET("/interfaces", omc.GetInterfacesWithOutput)
	}
}

// GetInterfaceOutputMessages retrieves output messages for a specific interface
func (omc *OutputMessageController) GetInterfaceOutputMessages(c *gin.Context) {
	interfaceId := c.Param("interfaceId")
	if interfaceId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Interface ID is required"})
		return
	}

	// Get pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	status := c.Query("status")
	messageType := c.Query("messageType")
	deliveryStatus := c.Query("deliveryStatus")

	// Build query conditions based on parameters
	// Note: This would require extending the service with pagination support
	// For now, return basic count and stats

	count, err := omc.service.GetOutputMessageCount(interfaceId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve output message count",
			"details": err.Error(),
		})
		return
	}

	stats, err := omc.service.GetInterfaceOutputStats(interfaceId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve output statistics",
			"details": err.Error(),
		})
		return
	}

	response := gin.H{
		"interface_id": interfaceId,
		"total_messages": count,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total_pages": (count + limit - 1) / limit, // Ceiling division
		},
		"filters": gin.H{
			"status": status,
			"message_type": messageType,
			"delivery_status": deliveryStatus,
		},
		"statistics": stats,
	}

	c.JSON(http.StatusOK, response)
}

// GetOutputMessage retrieves a specific output message by input message ID
func (omc *OutputMessageController) GetOutputMessage(c *gin.Context) {
	interfaceId := c.Param("interfaceId")
	inputMessageId := c.Param("inputMessageId")

	if interfaceId == "" || inputMessageId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Interface ID and input message ID are required"})
		return
	}

	outputMessage, err := omc.service.GetOutputMessage(interfaceId, inputMessageId)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Output message not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve output message",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"output_message": outputMessage,
		"metadata": gin.H{
			"interface_id": interfaceId,
			"input_message_id": inputMessageId,
			"retrieved_at": "now",
		},
	})
}

// GetInterfaceOutputStats retrieves statistics for an interface's output messages
func (omc *OutputMessageController) GetInterfaceOutputStats(c *gin.Context) {
	interfaceId := c.Param("interfaceId")
	if interfaceId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Interface ID is required"})
		return
	}

	stats, err := omc.service.GetInterfaceOutputStats(interfaceId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve output statistics",
			"details": err.Error(),
		})
		return
	}

	response := gin.H{
		"interface_id": interfaceId,
		"statistics": stats,
		"generated_at": "now",
	}

	c.JSON(http.StatusOK, response)
}

// GetOutputMessageCount retrieves the count of output messages for an interface
func (omc *OutputMessageController) GetOutputMessageCount(c *gin.Context) {
	interfaceId := c.Param("interfaceId")
	if interfaceId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Interface ID is required"})
		return
	}

	count, err := omc.service.GetOutputMessageCount(interfaceId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve message count",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"interface_id": interfaceId,
		"message_count": count,
	})
}

// UpdateDeliveryStatus updates the delivery status of an output message
func (omc *OutputMessageController) UpdateDeliveryStatus(c *gin.Context) {
	interfaceId := c.Param("interfaceId")
	inputMessageId := c.Param("inputMessageId")

	if interfaceId == "" || inputMessageId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Interface ID and input message ID are required"})
		return
	}

	var request struct {
		DeliveryStatus   string `json:"delivery_status" binding:"required"`
		DeliveryResponse string `json:"delivery_response,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate delivery status
	validStatuses := map[string]bool{
		"pending":   true,
		"delivered": true,
		"failed":    true,
		"retry":     true,
	}

	if !validStatuses[request.DeliveryStatus] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid delivery status",
			"valid_statuses": []string{"pending", "delivered", "failed", "retry"},
		})
		return
	}

	err := omc.service.UpdateDeliveryStatus(interfaceId, inputMessageId, request.DeliveryStatus, request.DeliveryResponse)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to update delivery status",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Delivery status updated successfully",
		"interface_id": interfaceId,
		"input_message_id": inputMessageId,
		"new_delivery_status": request.DeliveryStatus,
	})
}

// RedeliverMessage attempts to redeliver a failed output message
func (omc *OutputMessageController) RedeliverMessage(c *gin.Context) {
	interfaceId := c.Param("interfaceId")
	inputMessageId := c.Param("inputMessageId")

	if interfaceId == "" || inputMessageId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Interface ID and input message ID are required"})
		return
	}

	// Get the output message first
	outputMessage, err := omc.service.GetOutputMessage(interfaceId, inputMessageId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Output message not found",
			"details": err.Error(),
		})
		return
	}

	// Check if message is in a redeliverable state
	if outputMessage.DeliveryStatus == "delivered" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Message already delivered",
			"current_status": outputMessage.DeliveryStatus,
		})
		return
	}

	// Update status to retry
	err = omc.service.UpdateDeliveryStatus(interfaceId, inputMessageId, "retry", "Manual redelivery requested")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to mark message for redelivery",
			"details": err.Error(),
		})
		return
	}

	// In a real implementation, this would trigger the actual redelivery process
	// For now, we just update the status

	c.JSON(http.StatusOK, gin.H{
		"message": "Message marked for redelivery",
		"interface_id": interfaceId,
		"input_message_id": inputMessageId,
		"delivery_status": "retry",
		"delivery_attempts": outputMessage.DeliveryAttempts + 1,
	})
}

// GetInterfacesWithOutput retrieves all interfaces that have output messages
func (omc *OutputMessageController) GetInterfacesWithOutput(c *gin.Context) {
	// This would require a query across multiple output tables
	// For now, return a simple response indicating the feature needs implementation

	c.JSON(http.StatusOK, gin.H{
		"message": "Feature available - requires cross-table query implementation",
		"note": "Use interface-specific endpoints for now",
		"available_endpoints": []string{
			"GET /api/output/interfaces/{interfaceId}/messages",
			"GET /api/output/interfaces/{interfaceId}/stats",
			"GET /api/output/interfaces/{interfaceId}/count",
		},
	})
}

// Health check endpoint for output message service
func (omc *OutputMessageController) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service": "output-message-controller",
		"status": "healthy",
		"timestamp": "now",
		"version": "1.0",
	})
}