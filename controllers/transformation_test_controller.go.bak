package controllers

import (
	"context"
	"database/sql"
	"net/http"

	"ezhealthkonnect/hl7"
	"ezhealthkonnect/services"

	"github.com/gin-gonic/gin"
)

type TransformationTestController struct {
	db                       *sql.DB
	transformPipelineService *services.TransformationPipelineService
}

func NewTransformationTestController(db *sql.DB) *TransformationTestController {
	return &TransformationTestController{
		db:                       db,
		transformPipelineService: services.NewTransformationPipelineService(db),
	}
}

func (ctrl *TransformationTestController) TestPipeline(c *gin.Context) {
	var request struct {
		InterfaceID string `json:"interfaceId" binding:"required"`
		MessageType string `json:"messageType" binding:"required"`
		HL7Message  string `json:"hl7Message" binding:"required"`
	}

	if err := c.BindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Parse HL7 message
	parsedMessage := hl7.ParseWithRealSchema(request.HL7Message)
	if parsedMessage == nil || !parsedMessage.Success {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse HL7 message"})
		return
	}

	// Convert to map[string]interface{}
	enhancedSegments := make(map[string]interface{})
	for segmentName, segmentData := range parsedMessage.EnhancedSegments {
		enhancedSegments[segmentName] = segmentData
	}

	// Prepare input
	input := map[string]interface{}{
		"raw":              request.HL7Message,
		"enhancedSegments": enhancedSegments,
	}

	// Execute transformation pipeline
	ctx := context.Background()
	result, err := ctrl.transformPipelineService.ExecuteTransformation(
		ctx,
		"test-message-id",
		request.InterfaceID,
		request.MessageType,
		input,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  err.Error(),
			"result": result,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"result":  result,
	})
}

func (ctrl *TransformationTestController) GetPipeline(c *gin.Context) {
	interfaceID := c.Param("interfaceId")
	messageType := c.Param("messageType")

	pipeline, err := ctrl.transformPipelineService.GetPipeline(context.Background(), interfaceID, messageType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, pipeline)
}
