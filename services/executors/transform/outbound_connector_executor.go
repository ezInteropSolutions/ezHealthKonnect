package transform

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/connectors"
	"ezhealthkonnect/services/executors"
	"ezhealthkonnect/services/executors/format"
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

		// Resolve and serialize content (same logic as production, for accurate preview)
		content, resolvedContentType := resolveAndSerializeContent(inputData, config)

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
			"content_type":       resolvedContentType,
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
			"content_type":       resolvedContentType,
		}
		e.SetStepOutputWithDetails(outputData, variables, details)

		log.Printf("  🧪 [Test Mode] Outbound dry-run complete: config_valid=%v, payload_size=%d", configValid, len(content))
		return outputData, nil
	}

	// Get connector from factory
	factory := connectors.GetFactory()
	connector, err := factory.CreateOutbound(config.ConnectorType)
	if err != nil {
		if fn := models.GetDeliveryStatusFn(ctx); fn != nil {
			ifaceID, _ := ctx.Value("interface_id").(string)
			msgID, _ := ctx.Value("message_id").(string)
			fn(ifaceID, msgID, "failed", fmt.Sprintf("connector type '%s' not found", config.ConnectorType))
		}
		return nil, fmt.Errorf("failed to create outbound connector '%s': %w", config.ConnectorType, err)
	}

	// Initialize with config
	connConfig, _ := json.Marshal(config.Config)
	if err := connector.Initialize(connConfig); err != nil {
		if fn := models.GetDeliveryStatusFn(ctx); fn != nil {
			ifaceID, _ := ctx.Value("interface_id").(string)
			msgID, _ := ctx.Value("message_id").(string)
			fn(ifaceID, msgID, "failed", fmt.Sprintf("connector initialization failed: %v", err))
		}
		return nil, fmt.Errorf("failed to initialize connector '%s': %w", config.ConnectorType, err)
	}
	defer connector.Close()

	// Get content to send — format-aware serialization via Phase 4 serializer
	content, resolvedContentType := resolveAndSerializeContent(inputData, config)

	// Build outbound message
	outMsg := &models.OutboundMessage{
		MessageID:       fmt.Sprintf("pipeline-%d", time.Now().UnixNano()),
		Content:         content,
		ContentType:     resolvedContentType,
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
		// Notify engine of delivery failure so it can update delivery_status in PostgreSQL
		if fn := models.GetDeliveryStatusFn(ctx); fn != nil {
			interfaceID, _ := ctx.Value("interface_id").(string)
			messageID, _ := ctx.Value("message_id").(string)
			fn(interfaceID, messageID, "failed", err.Error())
		}
		variables := map[string]interface{}{
			"success":        false,
			"connector_type": config.ConnectorType,
			"error":          err.Error(),
		}
		details := map[string]interface{}{
			"duration_ms":    durationMs,
			"success":        false,
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
	// Resolve destination address — HTTP uses "endpoint", TCP uses "host"+"port", others vary
	destination := config.Config["endpoint"]
	if destination == nil {
		destination = config.Config["url"]
	}
	if destination == nil {
		if host, ok := config.Config["host"].(string); ok {
			if port, ok := config.Config["port"]; ok {
				destination = fmt.Sprintf("%s:%v", host, port)
			} else {
				destination = host
			}
		}
	}

	// Persist exact sent payload to object storage (full content, no truncation).
	// The URI is stored in the NDJSON log so the frontend can load the full payload on demand.
	outboundURI := ""
	if storeFn := models.GetStoreOutboundFn(ctx); storeFn != nil {
		ifaceID, _ := ctx.Value("interface_id").(string)
		msgID, _ := ctx.Value("message_id").(string)
		outboundURI = storeFn(ifaceID, msgID, content, resolvedContentType)
	}

	// body_preview in step_executions (DB) stays truncated for row-size efficiency.
	// The full payload is in object storage, referenced by body_uri in the NDJSON log.
	bodyPreview := content
	if len(bodyPreview) > 500 {
		bodyPreview = bodyPreview[:500] + "... (truncated)"
	}

	details := map[string]interface{}{
		"duration_ms":    durationMs,
		"success":        true,
		"connector_type": config.ConnectorType,
		"bytes_sent":     len(content),
		"request": map[string]interface{}{
			"method":       "POST",
			"endpoint":     destination,
			"content_type": resolvedContentType,
			"body_size":    len(content),
			"body_preview": bodyPreview,
			"body_uri":     outboundURI,
		},
	}
	if result != nil {
		variables["delivery_success"] = result.Success
		variables["acknowledgment"] = result.Acknowledgment
		// Surface HTTP response details as accessible pipeline variables
		if statusCode, ok := result.Metadata["status_code"]; ok {
			variables["response_status"] = statusCode
			details["response_status"] = statusCode
		}
		if respBody, ok := result.Metadata["response_body"]; ok {
			variables["response_body"] = respBody
			// Truncate in details to keep step metadata lean
			bodyStr := fmt.Sprintf("%v", respBody)
			if len(bodyStr) > 500 {
				bodyStr = bodyStr[:500] + "… (truncated)"
			}
			details["response_body_preview"] = bodyStr
		}
		if headers, ok := result.Metadata["response_headers"]; ok {
			details["response_headers"] = headers
		}
		details["duration_ms"] = result.DurationMs
	}
	e.SetStepOutputWithDetails(outputData, variables, details)

	// Notify engine of delivery outcome — updates delivery_status in PostgreSQL directly
	// Works for all 16 connector types; multiple outbound steps each write their own status
	if fn := models.GetDeliveryStatusFn(ctx); fn != nil {
		interfaceID, _ := ctx.Value("interface_id").(string)
		messageID, _ := ctx.Value("message_id").(string)
		ack := ""
		if result != nil {
			ack = result.Acknowledgment
		}
		deliveryStatus := "delivered"
		if result != nil && !result.Success {
			deliveryStatus = "failed"
		}
		fn(interfaceID, messageID, deliveryStatus, ack)
	}

	log.Printf("  ✅ Outbound Connector delivery complete: %s (%d bytes)", config.ConnectorType, len(content))
	return outputData, nil
}

// resolveAndSerializeContent resolves contentField (or falls back to whole message)
// and serializes it using the Phase 4 format serializer.
// Returns (serialized string, effective content-type).
func resolveAndSerializeContent(inputData map[string]interface{}, config outboundConnectorConfig) (string, string) {
	// Determine output format from message envelope
	outputFormat := ""
	if msg, ok := inputData["message"].(map[string]interface{}); ok {
		outputFormat, _ = msg["_format"].(string)
	}
	// ContentType config can override the format
	switch config.ContentType {
	case "application/hl7-v2":
		outputFormat = "hl7v2"
	case "application/fhir+json":
		outputFormat = "fhir"
	case "text/csv":
		outputFormat = "csv"
	}

	// Resolve raw content value
	var raw interface{}
	if config.ContentField != "" {
		raw = executors.GetFieldValue(inputData, config.ContentField)
		if raw == nil {
			if msg, ok := inputData["message"].(map[string]interface{}); ok {
				raw = executors.GetFieldValue(msg, config.ContentField)
			}
		}
	}
	if raw == nil {
		// Default: whole message envelope
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			raw = msg
		} else {
			raw = inputData
		}
	}

	// If the resolved value is already a string — use it as-is (no double serialization)
	if s, ok := raw.(string); ok {
		ct := config.ContentType
		if ct == "" {
			ct = "application/json"
		}
		return s, ct
	}

	// Serialize via format serializer
	body, ct, err := format.SerializeForOutput(raw, outputFormat)
	if err != nil {
		log.Printf("⚠️  [OutboundConnector] Serialization error (falling back to JSON): %v", err)
		b, _ := json.Marshal(raw)
		return string(b), "application/json"
	}
	return body, ct
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
