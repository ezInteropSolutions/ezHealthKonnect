package controllers

import (
	"fmt"
	"net/http"
	"time"

	"ezhealthkonnect/config"
	"ezhealthkonnect/processing"

	"github.com/gin-gonic/gin"
)

// InterfaceController handles interface management operations
type InterfaceController struct {
	config *config.Config
	engine *processing.ProcessingEngine
}

// NewInterfaceController creates a new interface controller
func NewInterfaceController(cfg *config.Config) *InterfaceController {
	return &InterfaceController{
		config: cfg,
		engine: nil, // Will be set via SetEngine
	}
}

// SetEngine sets the processing engine (called after initialization)
func (ctrl *InterfaceController) SetEngine(engine *processing.ProcessingEngine) {
	ctrl.engine = engine
}

// Interface request/response structures
type CreateInterfaceRequest struct {
	Name                  string                 `json:"name" binding:"required"`
	Description           string                 `json:"description"`
	SourceType            string                 `json:"sourceType" binding:"required"`
	TargetType            string                 `json:"targetType" binding:"required"`
	MessageType           string                 `json:"messageType"`
	SourceConfig          map[string]interface{} `json:"sourceConfig"`
	TargetConfig          map[string]interface{} `json:"targetConfig"`
	ProcessingRules       map[string]interface{} `json:"processingRules"`
	TransformationMapping map[string]interface{} `json:"transformationMapping"`
}

type InterfaceResponse struct {
	Success   bool                   `json:"success"`
	Interface map[string]interface{} `json:"interface,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// CreateInterface handles interface creation requests
func (ctrl *InterfaceController) CreateInterface(c *gin.Context) {
	var req CreateInterfaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, InterfaceResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	if ctrl.config.VerboseLogging {
		fmt.Printf("🔗 Creating interface: %s (%s → %s)\n",
			req.Name, req.SourceType, req.TargetType)
	}

	// TODO: Save to database (for now, we'll simulate)
	interfaceData := map[string]interface{}{
		"id":                    ctrl.generateInterfaceID(),
		"name":                  req.Name,
		"description":           req.Description,
		"sourceType":            req.SourceType,
		"targetType":            req.TargetType,
		"messageType":           req.MessageType,
		"sourceConfig":          req.SourceConfig,
		"targetConfig":          req.TargetConfig,
		"processingRules":       req.ProcessingRules,
		"transformationMapping": req.TransformationMapping,
		"status":                "created",
		"createdAt":             time.Now().Format(time.RFC3339),
		"updatedAt":             time.Now().Format(time.RFC3339),
	}

	fmt.Printf("✅ Interface created successfully: ID=%s\n", interfaceData["id"])

	c.JSON(http.StatusCreated, InterfaceResponse{
		Success:   true,
		Interface: interfaceData,
	})
}

// GetInterfaces handles listing all interfaces
func (ctrl *InterfaceController) GetInterfaces(c *gin.Context) {
	// TODO: Fetch from database
	interfaces := []map[string]interface{}{
		{
			"id":                "intf_001",
			"name":              "ADT Patient Registration",
			"status":            "running",
			"sourceType":        "tcp",
			"targetType":        "fhir",
			"messageType":       "ADT^A01",
			"createdAt":         "2025-01-05T10:00:00Z",
			"messagesProcessed": 1250,
			"lastActivity":      time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
		},
		{
			"id":                "intf_002",
			"name":              "Lab Results Integration",
			"status":            "running",
			"sourceType":        "file",
			"targetType":        "database",
			"messageType":       "ORU^R01",
			"createdAt":         "2025-01-04T15:30:00Z",
			"messagesProcessed": 856,
			"lastActivity":      time.Now().Add(-30 * time.Minute).Format(time.RFC3339),
		},
		{
			"id":                "intf_003",
			"name":              "Discharge Notifications",
			"status":            "paused",
			"sourceType":        "tcp",
			"targetType":        "rest",
			"messageType":       "ADT^A03",
			"createdAt":         "2025-01-03T09:15:00Z",
			"messagesProcessed": 432,
			"lastActivity":      time.Now().Add(-4 * time.Hour).Format(time.RFC3339),
		},
	}

	c.JSON(http.StatusOK, map[string]interface{}{
		"success":    true,
		"interfaces": interfaces,
		"total":      len(interfaces),
		"summary": map[string]interface{}{
			"running": 2,
			"paused":  1,
			"stopped": 0,
		},
	})
}

// GetInterface handles getting a single interface
func (ctrl *InterfaceController) GetInterface(c *gin.Context) {
	id := c.Param("id")

	// TODO: Fetch from database
	interfaceData := map[string]interface{}{
		"id":          id,
		"name":        "Sample Interface",
		"description": "Sample interface for demonstration",
		"status":      "running",
		"sourceType":  "tcp",
		"targetType":  "fhir",
		"messageType": "ADT^A01",
		"sourceConfig": map[string]interface{}{
			"host": "localhost",
			"port": 6661,
		},
		"targetConfig": map[string]interface{}{
			"endpoint": "http://fhir-server/Patient",
			"auth":     "bearer",
		},
		"statistics": map[string]interface{}{
			"messagesProcessed": 1250,
			"errorsCount":       5,
			"lastProcessed":     time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
			"averageTime":       "45ms",
		},
		"createdAt": "2025-01-05T10:00:00Z",
		"updatedAt": time.Now().Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, InterfaceResponse{
		Success:   true,
		Interface: interfaceData,
	})
}

// UpdateInterface handles interface updates
func (ctrl *InterfaceController) UpdateInterface(c *gin.Context) {
	id := c.Param("id")

	var updateData map[string]interface{}
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, InterfaceResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	if ctrl.config.VerboseLogging {
		fmt.Printf("🔄 Updating interface: %s\n", id)
	}

	// TODO: Update in database
	updatedInterface := map[string]interface{}{
		"id":        id,
		"updatedAt": time.Now().Format(time.RFC3339),
	}

	// Merge update data
	for key, value := range updateData {
		updatedInterface[key] = value
	}

	c.JSON(http.StatusOK, InterfaceResponse{
		Success:   true,
		Interface: updatedInterface,
	})
}

// DeleteInterface handles interface deletion
func (ctrl *InterfaceController) DeleteInterface(c *gin.Context) {
	id := c.Param("id")

	if ctrl.config.VerboseLogging {
		fmt.Printf("🗑️ Deleting interface: %s\n", id)
	}

	// TODO: Delete from database
	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Interface %s deleted successfully", id),
	})
}

// StartInterface handles starting an interface
func (ctrl *InterfaceController) StartInterface(c *gin.Context) {
	id := c.Param("id")

	if ctrl.config.VerboseLogging {
		fmt.Printf("▶️ Starting interface: %s\n", id)
	}

	// Check if engine is available
	if ctrl.engine == nil {
		c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"success": false,
			"error":   "Processing engine not available",
		})
		return
	}

	// Activate the interface via processing engine
	if err := ctrl.engine.ActivateInterface(id); err != nil {
		fmt.Printf("❌ Failed to start interface %s: %v\n", id, err)
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to start interface: %v", err),
		})
		return
	}

	fmt.Printf("✅ Interface %s started successfully\n", id)
	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Interface %s started", id),
		"status":  "active",
	})
}

// StopInterface handles stopping an interface
func (ctrl *InterfaceController) StopInterface(c *gin.Context) {
	id := c.Param("id")

	if ctrl.config.VerboseLogging {
		fmt.Printf("⏹️ Stopping interface: %s\n", id)
	}

	// Check if engine is available
	if ctrl.engine == nil {
		c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"success": false,
			"error":   "Processing engine not available",
		})
		return
	}

	// Deactivate the interface via processing engine
	if err := ctrl.engine.DeactivateInterface(id); err != nil {
		fmt.Printf("❌ Failed to stop interface %s: %v\n", id, err)
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to stop interface: %v", err),
		})
		return
	}

	fmt.Printf("✅ Interface %s stopped successfully\n", id)
	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Interface %s stopped", id),
		"status":  "inactive",
	})
}

// PauseInterface handles pausing an interface
func (ctrl *InterfaceController) PauseInterface(c *gin.Context) {
	id := c.Param("id")

	if ctrl.config.VerboseLogging {
		fmt.Printf("⏸️ Pausing interface: %s\n", id)
	}

	// TODO: Implement interface pause logic
	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Interface %s paused", id),
		"status":  "paused",
	})
}

// ReloadFamilyFilter tells the processing engine to re-read
// accepted_message_families for an interface from the database.
// Called by the Node.js layer after an updateInterface save so the in-memory
// filter cache reflects the new config without a full engine restart.
func (ctrl *InterfaceController) ReloadFamilyFilter(c *gin.Context) {
	id := c.Param("id")
	if ctrl.engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "engine not ready"})
		return
	}
	ctrl.engine.ReloadFamilyFilter(id)
	c.JSON(http.StatusOK, gin.H{"success": true, "interfaceId": id})
}

// Helper methods
func (ctrl *InterfaceController) generateInterfaceID() string {
	return fmt.Sprintf("intf_%d", time.Now().Unix())
}
