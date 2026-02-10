package transform

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/connectors"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"time"
)

// OutboundConnectorExecutor bridges outbound connectors as pipeline steps
// Supports all 16 outbound connector types (TCP/MLLP, HTTP, File, DB, MQ, Cloud)
type OutboundConnectorExecutor struct {
	*executors.BaseExecutor
}

func NewOutboundConnectorExecutor() *OutboundConnectorExecutor {
	return &OutboundConnectorExecutor{
		BaseExecutor: executors.NewBaseExecutor("connector.outbound", models.ExecutorMetadata{
			Name:        "Outbound Connector",
			Description: "Delivers data to external systems via configurable connectors",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Connectivity",
		}),
	}
}

type outboundConnectorConfig struct {
	ConnectorType string                 `json:"connectorType"` // e.g., "tcp_mllp_outbound", "http_outbound", etc.
	Config        map[string]interface{} `json:"config"`        // Connector-specific configuration
	ContentField  string                 `json:"contentField"`  // Field to send (e.g., "fhirBundle", "enriched.result")
	ContentType   string                 `json:"contentType"`   // MIME type (e.g., "application/fhir+json")
}

func (e *OutboundConnectorExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	startTime := time.Now()

	config := outboundConnectorConfig{
		ContentType: "application/json",
	}
	if step.Config != nil {
		configJSON, _ := json.Marshal(step.Config)
		json.Unmarshal(configJSON, &config)
	}

	if config.ConnectorType == "" {
		return nil, fmt.Errorf("outbound connector requires connectorType in config")
	}

	log.Printf("  📤 Outbound Connector: type=%s", config.ConnectorType)

	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	// TEST MODE: Validate config and preview payload, but do NOT send
	if models.IsTestMode(ctx) {
		log.Printf("  🧪 [Test Mode] Outbound connector dry-run: %s", config.ConnectorType)

		// Resolve content (same logic as production, for accurate preview)
		var content string
		if config.ContentField != "" {
			val := executors.GetFieldValue(inputData, config.ContentField)
			if val == nil {
				if msg, ok := inputData["message"].(map[string]interface{}); ok {
					val = executors.GetFieldValue(msg, config.ContentField)
				}
			}
			if val != nil {
				switch v := val.(type) {
				case string:
					content = v
				default:
					contentBytes, _ := json.Marshal(v)
					content = string(contentBytes)
				}
			}
		}
		if content == "" {
			if msg, ok := inputData["message"].(map[string]interface{}); ok {
				contentBytes, _ := json.Marshal(msg)
				content = string(contentBytes)
			}
		}

		durationMs := time.Since(startTime).Milliseconds()

		// Validate connector configuration (create + initialize, but don't send)
		configValid := true
		configMessage := "Configuration is valid"

		testFactory := connectors.GetFactory()
		testConnector, createErr := testFactory.CreateOutbound(config.ConnectorType)
		if createErr != nil {
			configValid = false
			configMessage = fmt.Sprintf("Connector type '%s' not found: %v", config.ConnectorType, createErr)
		} else {
			testConnConfig, _ := json.Marshal(config.Config)
			if initErr := testConnector.Initialize(testConnConfig); initErr != nil {
				configValid = false
				configMessage = fmt.Sprintf("Configuration invalid: %v", initErr)
			} else {
				testConnector.Close()
			}
		}

		// Build payload preview (truncate if large)
		payloadPreview := content
		if len(payloadPreview) > 2000 {
			payloadPreview = payloadPreview[:2000] + "... (truncated)"
		}

		variables := map[string]interface{}{
			"success":            true,
			"test_mode":          true,
			"connector_type":     config.ConnectorType,
			"config_valid":       configValid,
			"payload_size_bytes": len(content),
			"content_type":       config.ContentType,
			"message":            "Dry-run: payload prepared but NOT sent (test mode)",
		}
		details := map[string]interface{}{
			"duration_ms":        durationMs,
			"success":            true,
			"dry_run":            true,
			"connector_type":     config.ConnectorType,
			"config_valid":       configValid,
			"config_message":     configMessage,
			"payload_preview":    payloadPreview,
			"payload_size_bytes": len(content),
			"content_type":       config.ContentType,
		}
		e.SetStepOutputWithDetails(outputData, variables, details)

		log.Printf("  🧪 [Test Mode] Outbound dry-run complete: config_valid=%v, payload_size=%d", configValid, len(content))
		return outputData, nil
	}

	// Get connector from factory
	factory := connectors.GetFactory()
	connector, err := factory.CreateOutbound(config.ConnectorType)
	if err != nil {
		return nil, fmt.Errorf("failed to create outbound connector '%s': %w", config.ConnectorType, err)
	}

	// Initialize with config
	connConfig, _ := json.Marshal(config.Config)
	if err := connector.Initialize(connConfig); err != nil {
		return nil, fmt.Errorf("failed to initialize connector '%s': %w", config.ConnectorType, err)
	}
	defer connector.Close()

	// Get content to send
	var content string
	if config.ContentField != "" {
		val := executors.GetFieldValue(inputData, config.ContentField)
		if val == nil {
			if msg, ok := inputData["message"].(map[string]interface{}); ok {
				val = executors.GetFieldValue(msg, config.ContentField)
			}
		}
		if val != nil {
			switch v := val.(type) {
			case string:
				content = v
			default:
				contentBytes, _ := json.Marshal(v)
				content = string(contentBytes)
			}
		}
	}
	if content == "" {
		// Default: send entire message as JSON
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			contentBytes, _ := json.Marshal(msg)
			content = string(contentBytes)
		}
	}

	// Build outbound message
	outMsg := &models.OutboundMessage{
		MessageID:       fmt.Sprintf("pipeline-%d", time.Now().UnixNano()),
		Content:         content,
		ContentType:     config.ContentType,
		DestinationType: config.ConnectorType,
		MessageSize:     len(content),
		Metadata: map[string]string{
			"pipeline_step": step.StepName,
			"step_id":       step.ID,
		},
	}

	// Send
	result, err := connector.Send(ctx, outMsg)

	durationMs := time.Since(startTime).Milliseconds()

	if err != nil {
		variables := map[string]interface{}{
			"success":        false,
			"connector_type": config.ConnectorType,
			"error":          err.Error(),
		}
		details := map[string]interface{}{
			"duration_ms":    durationMs,
			"success":        true,
			"connector_type": config.ConnectorType,
			"delivery_error": err.Error(),
		}
		e.SetStepOutputWithDetails(outputData, variables, details)
		return outputData, fmt.Errorf("outbound delivery failed: %w", err)
	}

	variables := map[string]interface{}{
		"success":        true,
		"connector_type": config.ConnectorType,
		"message_id":     outMsg.MessageID,
		"bytes_sent":     len(content),
	}
	if result != nil {
		variables["delivery_success"] = result.Success
		variables["acknowledgment"] = result.Acknowledgment
	}

	details := map[string]interface{}{
		"duration_ms":    durationMs,
		"success":        true,
		"connector_type": config.ConnectorType,
		"bytes_sent":     len(content),
	}
	e.SetStepOutputWithDetails(outputData, variables, details)

	log.Printf("  ✅ Outbound Connector delivery complete: %s (%d bytes)", config.ConnectorType, len(content))
	return outputData, nil
}

func (e *OutboundConnectorExecutor) Validate(step *models.TransformationStep) error {
	if step.Config == nil {
		return fmt.Errorf("outbound connector requires config with connectorType")
	}
	ct, _ := step.Config["connectorType"].(string)
	if ct == "" {
		return fmt.Errorf("outbound connector requires connectorType in config")
	}
	return nil
}
