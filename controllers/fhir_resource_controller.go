// controllers/fhir_resource_controller.go
// API Controller for HL7 to FHIR Resource Identification
// Based on official HL7 working group specifications

package controllers

import (
	"log"
	"net/http"

	"ezhealthkonnect/services"

	"github.com/gin-gonic/gin"
)

type FHIRResourceController struct {
	resourceService *services.MessageResourceIdentifierService
}

func NewFHIRResourceController(resourceService *services.MessageResourceIdentifierService) *FHIRResourceController {
	return &FHIRResourceController{
		resourceService: resourceService,
	}
}

// GetResourcesForMessageTypeRequest represents the request body for resource identification
type GetResourcesForMessageTypeRequest struct {
	ParsedHL7   map[string]interface{} `json:"parsedHL7"`
	InterfaceID string                 `json:"interfaceId,omitempty"`
}

// GetResourcesForMessageTypeResponse represents the API response
type GetResourcesForMessageTypeResponse struct {
	Success          bool                               `json:"success"`
	MessageType      string                             `json:"messageType"`
	FHIRResources    []string                           `json:"fhirResources"`
	ResourceLogic    map[string]services.ResourceConfig `json:"resourceLogic"`
	ResourcePriority map[string]int                     `json:"resourcePriority"`
	Source           string                             `json:"source"`
	Statistics       ResourceIdentificationStatistics   `json:"statistics"`
	Error            string                             `json:"error,omitempty"`
}

type ResourceIdentificationStatistics struct {
	TotalPossibleResources int `json:"totalPossibleResources"`
	IdentifiedResources    int `json:"identifiedResources"`
	RequiredResources      int `json:"requiredResources"`
	ConditionalResources   int `json:"conditionalResources"`
	ProcessingTimeMs       int `json:"processingTimeMs"`
}

// SaveInterfaceOverrideRequest represents request for saving interface customizations
type SaveInterfaceOverrideRequest struct {
	InterfaceID      string                             `json:"interfaceId" binding:"required"`
	MessageType      string                             `json:"messageType" binding:"required"`
	CustomResources  []string                           `json:"customResources" binding:"required"`
	CustomConditions map[string]services.ResourceConfig `json:"customConditions"`
	CreatedBy        string                             `json:"createdBy"`
}

// Register routes for FHIR resource identification
func (c *FHIRResourceController) RegisterRoutes(router *gin.RouterGroup) {
	fhirGroup := router.Group("/fhir/transform")
	{
		// GET /api/fhir/transform/resources/{messageType}
		fhirGroup.GET("/resources/:messageType", c.GetResourcesForMessageType)

		// POST /api/fhir/transform/resources/{messageType} (with parsed HL7 data)
		fhirGroup.POST("/resources/:messageType", c.GetResourcesForMessageTypeWithData)

		// POST /api/fhir/transform/resources/interface-override
		fhirGroup.POST("/resources/interface-override", c.SaveInterfaceOverride)

		// GET /api/fhir/transform/resources/test/{messageType} (for testing)
		fhirGroup.GET("/resources/test/:messageType", c.TestResourceIdentification)
	}
}

// GetResourcesForMessageType handles GET requests without parsed HL7 data
// GET /api/fhir/transform/resources/{messageType}?interfaceId=...
func (c *FHIRResourceController) GetResourcesForMessageType(ctx *gin.Context) {
	messageType := ctx.Param("messageType")
	interfaceID := ctx.Query("interfaceId")

	log.Printf("📡 GET /api/fhir/transform/resources/%s (interfaceId: %s)", messageType, interfaceID)

	if messageType == "" {
		ctx.JSON(http.StatusBadRequest, GetResourcesForMessageTypeResponse{
			Success: false,
			Error:   "Message type is required",
		})
		return
	}

	// Use empty parsed HL7 data for basic resource identification
	emptyHL7 := make(map[string]interface{})

	template, err := c.resourceService.GetResourcesForMessage(messageType, emptyHL7, interfaceID)
	if err != nil {
		log.Printf("❌ Error identifying resources: %v", err)
		ctx.JSON(http.StatusInternalServerError, GetResourcesForMessageTypeResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	response := GetResourcesForMessageTypeResponse{
		Success:          true,
		MessageType:      template.MessageType,
		FHIRResources:    template.FHIRResources,
		ResourceLogic:    template.ResourceLogic,
		ResourcePriority: template.ResourcePriority,
		Source:           template.Source,
		Statistics: ResourceIdentificationStatistics{
			TotalPossibleResources: len(template.FHIRResources),
			IdentifiedResources:    len(template.FHIRResources),
			RequiredResources:      c.countRequiredResources(template.ResourceLogic),
			ConditionalResources:   c.countConditionalResources(template.ResourceLogic),
		},
	}

	log.Printf("✅ Identified %d FHIR resources for %s (source: %s)", len(template.FHIRResources), messageType, template.Source)
	ctx.JSON(http.StatusOK, response)
}

// GetResourcesForMessageTypeWithData handles POST requests with parsed HL7 data
// POST /api/fhir/transform/resources/{messageType}
func (c *FHIRResourceController) GetResourcesForMessageTypeWithData(ctx *gin.Context) {
	messageType := ctx.Param("messageType")

	var request GetResourcesForMessageTypeRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		log.Printf("❌ Invalid request body: %v", err)
		ctx.JSON(http.StatusBadRequest, GetResourcesForMessageTypeResponse{
			Success: false,
			Error:   "Invalid request body: " + err.Error(),
		})
		return
	}

	log.Printf("📡 POST /api/fhir/transform/resources/%s (interfaceId: %s, parsed segments: %d)",
		messageType, request.InterfaceID, len(request.ParsedHL7))

	if messageType == "" {
		ctx.JSON(http.StatusBadRequest, GetResourcesForMessageTypeResponse{
			Success: false,
			Error:   "Message type is required",
		})
		return
	}

	template, err := c.resourceService.GetResourcesForMessage(messageType, request.ParsedHL7, request.InterfaceID)
	if err != nil {
		log.Printf("❌ Error identifying resources: %v", err)
		ctx.JSON(http.StatusInternalServerError, GetResourcesForMessageTypeResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	response := GetResourcesForMessageTypeResponse{
		Success:          true,
		MessageType:      template.MessageType,
		FHIRResources:    template.FHIRResources,
		ResourceLogic:    template.ResourceLogic,
		ResourcePriority: template.ResourcePriority,
		Source:           template.Source,
		Statistics: ResourceIdentificationStatistics{
			TotalPossibleResources: c.getTotalPossibleResources(messageType),
			IdentifiedResources:    len(template.FHIRResources),
			RequiredResources:      c.countRequiredResources(template.ResourceLogic),
			ConditionalResources:   c.countConditionalResources(template.ResourceLogic),
		},
	}

	log.Printf("✅ Identified %d FHIR resources for %s with parsed data (source: %s)",
		len(template.FHIRResources), messageType, template.Source)
	ctx.JSON(http.StatusOK, response)
}

// SaveInterfaceOverride allows interfaces to customize their resource mappings
// POST /api/fhir/transform/resources/interface-override
func (c *FHIRResourceController) SaveInterfaceOverride(ctx *gin.Context) {
	var request SaveInterfaceOverrideRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		log.Printf("❌ Invalid override request: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	log.Printf("📡 POST /api/fhir/transform/resources/interface-override (interface: %s, messageType: %s)",
		request.InterfaceID, request.MessageType)

	err := c.resourceService.SaveInterfaceOverride(
		request.InterfaceID,
		request.MessageType,
		request.CustomResources,
		request.CustomConditions,
		request.CreatedBy,
	)

	if err != nil {
		log.Printf("❌ Error saving interface override: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	log.Printf("✅ Saved interface override for %s message type %s", request.InterfaceID, request.MessageType)
	ctx.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "Interface override saved successfully",
		"interfaceId":   request.InterfaceID,
		"messageType":   request.MessageType,
		"resourceCount": len(request.CustomResources),
	})
}

// TestResourceIdentification provides a test endpoint with sample data
// GET /api/fhir/transform/resources/test/{messageType}
func (c *FHIRResourceController) TestResourceIdentification(ctx *gin.Context) {
	messageType := ctx.Param("messageType")

	log.Printf("📡 GET /api/fhir/transform/resources/test/%s", messageType)

	// Create sample parsed HL7 data for testing
	sampleHL7 := c.createSampleHL7Data(messageType)

	template, err := c.resourceService.GetResourcesForMessage(messageType, sampleHL7, "")
	if err != nil {
		log.Printf("❌ Error in test resource identification: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	response := gin.H{
		"success":       true,
		"messageType":   template.MessageType,
		"fhirResources": template.FHIRResources,
		"resourceLogic": template.ResourceLogic,
		"source":        template.Source,
		"sampleHL7Data": sampleHL7,
		"testNote":      "This endpoint uses sample HL7 data for testing resource identification logic",
	}

	log.Printf("✅ Test completed for %s: identified %d resources", messageType, len(template.FHIRResources))
	ctx.JSON(http.StatusOK, response)
}

// Helper methods

func (c *FHIRResourceController) countRequiredResources(resourceLogic map[string]services.ResourceConfig) int {
	count := 0
	for _, config := range resourceLogic {
		if config.Required {
			count++
		}
	}
	return count
}

func (c *FHIRResourceController) countConditionalResources(resourceLogic map[string]services.ResourceConfig) int {
	count := 0
	for _, config := range resourceLogic {
		if config.Condition != "" {
			count++
		}
	}
	return count
}

func (c *FHIRResourceController) getTotalPossibleResources(messageType string) int {
	// This could be enhanced to query the database for total possible resources
	totalsByType := map[string]int{
		"ADT^A01": 14,
		"ORU^R01": 10,
		"ORM^O01": 10,
	}

	if total, exists := totalsByType[messageType]; exists {
		return total
	}
	return 5 // Default estimate
}

func (c *FHIRResourceController) createSampleHL7Data(messageType string) map[string]interface{} {
	// Create realistic sample data based on message type
	switch messageType {
	case "ADT^A01":
		return map[string]interface{}{
			"MSH": map[string]interface{}{"1": "|", "9": "ADT^A01"},
			"PID": map[string]interface{}{"5": "DOE^JOHN^M", "8": "M"},
			"PV1": map[string]interface{}{"2": "I", "3": "ICU^101^A", "20": "SELF"},
			"AL1": map[string]interface{}{"3": "PENICILLIN"},
			"DG1": map[string]interface{}{"3": "K25.9"},
		}
	case "ORU^R01":
		return map[string]interface{}{
			"MSH": map[string]interface{}{"1": "|", "9": "ORU^R01"},
			"PID": map[string]interface{}{"5": "DOE^JANE^M", "8": "F"},
			"PV1": map[string]interface{}{"2": "O", "3": "LAB^101^A"},
			"ORC": map[string]interface{}{"1": "RE", "2": "12345"},
			"OBR": map[string]interface{}{"4": "CBC^COMPLETE BLOOD COUNT"},
			"OBX": map[string]interface{}{"3": "WBC^WHITE BLOOD COUNT", "5": "7.5"},
		}
	case "ORM^O01":
		return map[string]interface{}{
			"MSH": map[string]interface{}{"1": "|", "9": "ORM^O01"},
			"PID": map[string]interface{}{"5": "DOE^ROBERT^K", "8": "M"},
			"ORC": map[string]interface{}{"1": "NW", "2": "ORD12345"},
			"OBR": map[string]interface{}{"4": "XRAY^CHEST X-RAY"},
		}
	default:
		return map[string]interface{}{
			"MSH": map[string]interface{}{"1": "|", "9": messageType},
			"PID": map[string]interface{}{"5": "TEST^PATIENT^A"},
		}
	}
}
