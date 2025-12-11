package enrichment

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// ===============================================================
// METADATA ENRICHMENT EXECUTOR
// ===============================================================

// MetadataEnrichmentExecutor adds metadata fields to messages
// Implements Strategy Pattern - concrete strategy for metadata enrichment
type MetadataEnrichmentExecutor struct {
	*executors.BaseExecutor
}

// NewMetadataEnrichmentExecutor creates a new metadata enrichment executor
func NewMetadataEnrichmentExecutor() *MetadataEnrichmentExecutor {
	metadata := models.ExecutorMetadata{
		Name:        "Metadata Enrichment",
		Description: "Adds processing metadata (timestamps, correlation IDs, custom fields) to messages",
		Version:     "1.0.0",
		Author:      "ezHealthKonnect",
		Category:    "enrichment",
	}

	base := executors.NewBaseExecutor("pre.enrichment.metadata", metadata)

	return &MetadataEnrichmentExecutor{
		BaseExecutor: base,
	}
}

// Execute performs metadata enrichment
func (e *MetadataEnrichmentExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	// Pre-execution validation
	if err := e.PreExecute(ctx, step); err != nil {
		return inputData, err
	}

	// Parse configuration
	config, err := e.parseConfig(step)
	if err != nil {
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}

	// Ensure metadata map exists
	metadata := executors.EnsureMapExists(inputData, "metadata")

	log.Printf("🔍 [MetadataEnrichment] Adding metadata fields...")

	// Add timestamp
	if config.AddTimestamp {
		timestamp := time.Now().UTC().Format(time.RFC3339)
		metadata["receivedAt"] = timestamp
		metadata["processedAt"] = timestamp
		log.Printf("   ✅ Added timestamp: %s", timestamp)
	}

	// Add correlation ID
	if config.AddCorrelationID {
		correlationID := uuid.New().String()
		metadata["correlationId"] = correlationID
		log.Printf("   ✅ Added correlationId: %s", correlationID)
	}

	// Add interface ID (from context or step config)
	if config.AddInterfaceID {
		if interfaceID, exists := inputData["interfaceId"]; exists {
			metadata["interfaceId"] = interfaceID
			log.Printf("   ✅ Added interfaceId: %v", interfaceID)
		} else {
			log.Printf("   ⚠️ interfaceId not found in input data")
		}
	}

	// Add message ID (from HL7 MSH.10 if available)
	if config.AddMessageID {
		if msgID := e.extractMessageID(inputData); msgID != "" {
			metadata["messageId"] = msgID
			log.Printf("   ✅ Added messageId: %s", msgID)
		} else {
			// Generate new message ID if not found
			msgID := fmt.Sprintf("MSG-%s", uuid.New().String()[:8])
			metadata["messageId"] = msgID
			log.Printf("   ✅ Generated messageId: %s", msgID)
		}
	}

	// Add custom metadata fields
	if config.CustomMetadata != nil && len(config.CustomMetadata) > 0 {
		for key, value := range config.CustomMetadata {
			metadata[key] = value
			log.Printf("   ✅ Added custom field: %s = %s", key, value)
		}
	}

	// Post-execution logging
	e.PostExecute(ctx, step, nil, time.Since(start))

	return inputData, nil
}

// Validate checks if the step configuration is valid
func (e *MetadataEnrichmentExecutor) Validate(step *models.TransformationStep) error {
	_, err := e.parseConfig(step)
	return err
}

// parseConfig parses and validates the step configuration
func (e *MetadataEnrichmentExecutor) parseConfig(step *models.TransformationStep) (*models.MetadataEnrichmentConfig, error) {
	if step.Config == nil {
		// Default config - add timestamp and correlation ID
		return &models.MetadataEnrichmentConfig{
			AddTimestamp:     true,
			AddCorrelationID: true,
		}, nil
	}

	// Marshal to JSON then unmarshal to struct for type safety
	configJSON, err := json.Marshal(step.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var config models.MetadataEnrichmentConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

// extractMessageID attempts to extract the message ID from HL7 MSH.10
func (e *MetadataEnrichmentExecutor) extractMessageID(inputData map[string]interface{}) string {
	// Try to get from enhancedSegments.MSH.fields[9].value (MSH.10 - Message Control ID)
	if enhancedSegs, ok := inputData["enhancedSegments"].(map[string]interface{}); ok {
		if msh, ok := enhancedSegs["MSH"].(map[string]interface{}); ok {
			if fields, ok := msh["fields"].([]interface{}); ok {
				if len(fields) > 9 {
					if field, ok := fields[9].(map[string]interface{}); ok {
						if value, ok := field["value"].(string); ok {
							return value
						}
					}
				}
			}
		}
	}

	return ""
}

// GetConfigSchema returns the JSON schema for configuration
func (e *MetadataEnrichmentExecutor) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"addTimestamp": map[string]interface{}{
				"type":        "boolean",
				"description": "Add receivedAt and processedAt timestamps",
				"default":     true,
			},
			"addCorrelationId": map[string]interface{}{
				"type":        "boolean",
				"description": "Generate and add a unique correlation ID (UUID)",
				"default":     true,
			},
			"addInterfaceId": map[string]interface{}{
				"type":        "boolean",
				"description": "Add the interface ID from context",
				"default":     false,
			},
			"addMessageId": map[string]interface{}{
				"type":        "boolean",
				"description": "Extract or generate message ID",
				"default":     false,
			},
			"customMetadata": map[string]interface{}{
				"type":        "object",
				"description": "Custom key-value pairs to add to metadata",
				"additionalProperties": map[string]interface{}{
					"type": "string",
				},
			},
		},
	}
}

// GetConfigExample returns an example configuration
func (e *MetadataEnrichmentExecutor) GetConfigExample() map[string]interface{} {
	return map[string]interface{}{
		"addTimestamp":     true,
		"addCorrelationId": true,
		"addInterfaceId":   true,
		"addMessageId":     true,
		"customMetadata": map[string]string{
			"processingNode": "server-01",
			"environment":    "production",
			"version":        "2.5.0",
		},
	}
}
