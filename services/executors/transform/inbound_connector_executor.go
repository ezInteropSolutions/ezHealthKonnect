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

// InboundConnectorExecutor bridges inbound connectors as pipeline steps
// Used for fetching/querying data from external sources mid-pipeline
type InboundConnectorExecutor struct {
	*executors.BaseExecutor
}

func NewInboundConnectorExecutor() *InboundConnectorExecutor {
	return &InboundConnectorExecutor{
		BaseExecutor: executors.NewBaseExecutor("connector.inbound", models.ExecutorMetadata{
			Name:        "Inbound Connector",
			Description: "Fetches data from external systems via configurable connectors",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Connectivity",
		}),
	}
}

type inboundConnectorConfig struct {
	ConnectorType string                 `json:"connectorType"` // e.g., "postgresql_inbound", "mongodb_inbound", etc.
	Config        map[string]interface{} `json:"config"`        // Connector-specific configuration
	OutputField   string                 `json:"outputField"`   // Where to store fetched data (default: "enriched.connector_result")
	TimeoutMs     int                    `json:"timeoutMs"`     // Fetch timeout in ms (default: 30000)
}

func (e *InboundConnectorExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	startTime := time.Now()

	config := inboundConnectorConfig{
		OutputField: "enriched.connector_result",
		TimeoutMs:   30000,
	}
	if step.Config != nil {
		configJSON, _ := json.Marshal(step.Config)
		json.Unmarshal(configJSON, &config)
	}

	if config.ConnectorType == "" {
		return nil, fmt.Errorf("inbound connector requires connectorType in config")
	}

	log.Printf("  📥 Inbound Connector: type=%s", config.ConnectorType)

	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	// Get connector from factory
	factory := connectors.GetFactory()
	connector, err := factory.CreateInbound(config.ConnectorType)
	if err != nil {
		return nil, fmt.Errorf("failed to create inbound connector '%s': %w", config.ConnectorType, err)
	}

	// Initialize with config
	connConfig, _ := json.Marshal(config.Config)
	if err := connector.Initialize(connConfig); err != nil {
		return nil, fmt.Errorf("failed to initialize connector '%s': %w", config.ConnectorType, err)
	}
	defer connector.Close()

	// Create a channel to receive messages
	messageChan := make(chan *models.InboundMessage, 100)

	// Create a timeout context
	fetchCtx, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutMs)*time.Millisecond)
	defer cancel()

	// Start the connector to fetch data
	if err := connector.Start(fetchCtx, messageChan); err != nil {
		return nil, fmt.Errorf("failed to start inbound connector '%s': %w", config.ConnectorType, err)
	}
	defer connector.Stop()

	// Collect messages
	var messages []map[string]interface{}
	collectDone := make(chan struct{})

	go func() {
		defer close(collectDone)
		for {
			select {
			case msg, ok := <-messageChan:
				if !ok {
					return
				}
				msgData := map[string]interface{}{
					"message_id":   msg.MessageID,
					"content":      msg.Content,
					"content_type": msg.ContentType,
					"received_at":  msg.ReceivedAt,
				}
				if msg.SourceMetadata != nil {
					msgData["metadata"] = msg.SourceMetadata
				}
				messages = append(messages, msgData)
			case <-fetchCtx.Done():
				return
			}
		}
	}()

	// Wait for collection to finish or timeout
	select {
	case <-collectDone:
	case <-fetchCtx.Done():
	}

	durationMs := time.Since(startTime).Milliseconds()

	// Store results
	if config.OutputField != "" {
		executors.UpdateFieldValue(outputData, config.OutputField, messages)
	}

	variables := map[string]interface{}{
		"success":        true,
		"connector_type": config.ConnectorType,
		"message_count":  len(messages),
	}
	details := map[string]interface{}{
		"duration_ms":    durationMs,
		"success":        true,
		"connector_type": config.ConnectorType,
		"message_count":  len(messages),
	}
	e.SetStepOutputWithDetails(outputData, variables, details)

	log.Printf("  ✅ Inbound Connector fetch complete: %s (%d messages)", config.ConnectorType, len(messages))
	return outputData, nil
}

func (e *InboundConnectorExecutor) Validate(step *models.TransformationStep) error {
	if step.Config == nil {
		return fmt.Errorf("inbound connector requires config with connectorType")
	}
	ct, _ := step.Config["connectorType"].(string)
	if ct == "" {
		return fmt.Errorf("inbound connector requires connectorType in config")
	}
	return nil
}
