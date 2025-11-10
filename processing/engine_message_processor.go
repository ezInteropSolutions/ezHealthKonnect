// processing/engine_message_processor.go
// Message processing for ProcessingEngine

package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services"
	"go.mongodb.org/mongo-driver/bson"
)

// processMessages processes incoming messages from ANY connector channel (ALL 32 connectors)
// UNIFIED MODEL: Uses models.InboundMessage for all connector types
func (pe *ProcessingEngine) processMessages(interfaceID string, messageChan <-chan *models.InboundMessage) {
	log.Printf("📨 Message processor started for interface %s", interfaceID)

	for msg := range messageChan {
		startTime := time.Now()

		log.Printf("📥 Processing message for interface %s: ID=%s, Size=%d bytes",
			interfaceID, msg.MessageID, len(msg.Content))

		// STEP 1: Store message in PostgreSQL interface table (with error capture)
		err := pe.storeMessage(interfaceID, msg)
		if err != nil {
			log.Printf("❌ Failed to store message for interface %s: %v", interfaceID, err)

			// Capture database error (V23 - Error Handling Enhancement)
			if pe.errorService != nil {
				errCtx := models.NewErrorContext(
					models.StageDatabase,
					models.SeverityError,
					models.ErrorTypeDatabase,
					"Failed to store message in PostgreSQL",
					err.Error(),
					"", // No stack trace for database errors
					models.RecoveryMarkedFailed,
				)
				if captureErr := pe.errorService.CaptureError(interfaceID, msg.MessageID, errCtx); captureErr != nil {
					log.Printf("⚠️  Failed to capture error: %v", captureErr)
				}
			}

			pe.updateInterfaceError(interfaceID)
			continue
		}

		processingTime := time.Since(startTime)
		log.Printf("✅ Message stored in PostgreSQL for interface %s: ID=%s (took %v)",
			interfaceID, msg.MessageID, processingTime)

		// STEP 2: Store in MongoDB + Parse to JSON (async with panic recovery)
		go pe.storeAndParseWithRecovery(interfaceID, msg)

		// Update interface statistics
		pe.updateInterfaceStats(interfaceID, processingTime)
	}

	log.Printf("📪 Message processor stopped for interface %s", interfaceID)
}

// storeMessage stores the raw message in PostgreSQL
func (pe *ProcessingEngine) storeMessage(interfaceID string, msg *models.InboundMessage) error {
	tableName := fmt.Sprintf("messages_intf_%s", sanitizeInterfaceID(interfaceID))

	// UNIFIED MODEL: Use models.InboundMessage fields directly
	sourceType := msg.SourceType       // tcp_mllp, http_rest, file_listener, etc.
	sourceEndpoint := msg.SourceEndpoint // tcp://0.0.0.0:2575, http://..., etc.
	sourceIP := msg.SourceIP            // Client IP address
	messageType := msg.MessageType      // HL7v2, FHIR, JSON, XML, etc.

	query := fmt.Sprintf(`
		INSERT INTO %s (
			message_id, interface_id, status, priority,
			received_at, source_type, source_endpoint, source_ip,
			message_type, message_size, message_encoding,
			raw_message, created_at, updated_at
		) VALUES (
			$1, $2, 'received', 1,
			$3, $4, $5, $6,
			$7, $8, $9,
			$10, NOW(), NOW()
		)
	`, tableName)

	encoding := msg.Encoding
	if encoding == "" {
		encoding = "UTF-8" // Default encoding
	}

	_, err := pe.db.Exec(query,
		msg.MessageID,
		interfaceID,
		msg.ReceivedAt,
		sourceType,      // tcp_mllp, http_rest, etc.
		sourceEndpoint,  // Connection endpoint
		sourceIP,        // Client IP
		messageType,     // Message format
		msg.MessageSize,
		encoding,
		msg.Content,
	)

	return err
}

// sanitizeInterfaceID converts UUID to table-safe format
func sanitizeInterfaceID(id string) string {
	return strings.ReplaceAll(id, "-", "_")
}

// updateInterfaceStats updates interface processing statistics
func (pe *ProcessingEngine) updateInterfaceStats(interfaceID string, processingTime time.Duration) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if iface, exists := pe.activeInterfaces[interfaceID]; exists {
		iface.MessagesProcessed++
		iface.LastActivity = time.Now()
		pe.stats.LastActivity = time.Now()
	}
}

// updateInterfaceError increments error count
func (pe *ProcessingEngine) updateInterfaceError(interfaceID string) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if iface, exists := pe.activeInterfaces[interfaceID]; exists {
		iface.Errors++
		iface.LastActivity = time.Now()
		pe.stats.LastActivity = time.Now()
	}
}

// storeAndParseWithRecovery wraps storeAndParse with panic recovery (V23 - Error Handling Enhancement)
func (pe *ProcessingEngine) storeAndParseWithRecovery(interfaceID string, msg *models.InboundMessage) {
	// Use error handler if available for panic recovery
	if pe.errorHandler != nil {
		pe.errorHandler.SafeExecuteAsync(msg.MessageID, interfaceID, models.StageJSONConversion, func() error {
			pe.storeAndParse(interfaceID, msg)
			return nil
		})
	} else {
		// Fallback: manual panic recovery
		defer func() {
			if r := recover(); r != nil {
				log.Printf("🚨 PANIC RECOVERED in storeAndParse: %v (message: %s)", r, msg.MessageID)
				if pe.errorService != nil {
					errCtx := models.NewErrorContext(
						models.StageJSONConversion,
						models.SeverityCritical,
						models.ErrorTypePanic,
						fmt.Sprintf("Panic during JSON conversion: %v", r),
						"System panic recovered during message parsing",
						"", // Stack trace captured by errorHandler
						models.RecoveryPanicRecovered,
					)
					pe.errorService.CaptureError(interfaceID, msg.MessageID, errCtx)
				}
			}
		}()
		pe.storeAndParse(interfaceID, msg)
	}
}

// storeAndParse stores raw message in MongoDB and triggers JSON parsing
func (pe *ProcessingEngine) storeAndParse(interfaceID string, msg *models.InboundMessage) {
	ctx := context.Background()

	// Get dynamic metadata from message
	messageType := msg.MessageType // Default: hl7, fhir, etc.
	if mt, ok := msg.Headers["message_type"]; ok && mt != "" {
		messageType = mt
	}

	sourceType := msg.SourceType // Default: tcp, http, etc.
	if st, ok := msg.Headers["source_type"]; ok && st != "" {
		sourceType = st
	}

	// STEP 1: Store raw message in MongoDB (if available)
	if pe.mongoService != nil {
		rawDoc := &services.RawMessageDocument{
			MessageID:       msg.MessageID,
			InterfaceID:     interfaceID,
			MessageType:     messageType,  // Dynamic: hl7v2, fhir, etc.
			SourceType:      sourceType,   // Dynamic: tcp, http, etc.
			SourceEndpoint:  msg.SourceType,
			ReceivedAt:      msg.ReceivedAt,
			MessageSize:     len(msg.Content),
			MessageEncoding: "UTF-8",
			RawContent:      msg.Content,
		}

		err := pe.mongoService.StoreRawMessage(ctx, rawDoc)
		if err != nil {
			log.Printf("⚠️  Failed to store raw message in MongoDB: %v", err)

			// Capture MongoDB error (V23 - Error Handling Enhancement)
			if pe.errorService != nil {
				errCtx := models.NewErrorContext(
					models.StageDatabase,
					models.SeverityWarning, // Warning since PostgreSQL storage already succeeded
					models.ErrorTypeDatabase,
					"Failed to store raw message in MongoDB",
					err.Error(),
					"",
					models.RecoveryNone, // Processing continues without MongoDB
				)
				pe.errorService.CaptureError(interfaceID, msg.MessageID, errCtx)
			}

			// Update PostgreSQL mongo_synced status
			pe.updateMongoSyncStatus(interfaceID, msg.MessageID, false)
		} else {
			log.Printf("💾 Message stored in MongoDB: %s (type: %s, source: %s)",
				msg.MessageID, messageType, sourceType)
			// Update PostgreSQL mongo_synced status
			pe.updateMongoSyncStatus(interfaceID, msg.MessageID, true)
		}
	}

	// STEP 2: Parse to JSON (if parser service available)
	// Note: For FHIR messages, content is already JSON - parser will detect and handle accordingly
	if pe.parserService != nil {
		log.Printf("🔄 Starting JSON conversion for message: %s (type: %s)", msg.MessageID, messageType)
		result, err := pe.parserService.ParseToJSON(ctx, msg.MessageID, interfaceID, msg.Content)
		if err != nil {
			log.Printf("❌ JSON conversion failed for %s: %v", msg.MessageID, err)

			// Capture JSON parsing error (V23 - Error Handling Enhancement)
			if pe.errorService != nil {
				errCtx := models.NewErrorContext(
					models.StageJSONConversion,
					models.SeverityError,
					models.ErrorTypeHL7Parse,
					"Failed to parse message to JSON",
					err.Error(),
					"",
					models.RecoveryMarkedFailed,
				)
				pe.errorService.CaptureError(interfaceID, msg.MessageID, errCtx)
			}
		} else {
			log.Printf("✅ JSON conversion completed for %s in %dms",
				msg.MessageID, result.ParsingTime.Milliseconds())

			// STEP 3: Trigger transformation pipeline (MVC + OOB pattern)
			if pe.transformationService != nil {
				go pe.executeTransformationPipeline(ctx, interfaceID, msg.MessageID, messageType, result)
			} else {
				log.Printf("⚠️  Transformation service not available - skipping transformation")
			}
		}
	}
}

// executeTransformationPipeline executes the transformation pipeline (MVC + OOB)
// This is the Controller layer - orchestrates Model (service) and View (output)
func (pe *ProcessingEngine) executeTransformationPipeline(
	ctx context.Context,
	interfaceID string,
	messageID string,
	messageType string,
	parseResult *models.ParserResult,
) {
	log.Printf("🔄 [MVC PIPELINE] FUNCTION ENTERED - message: %s", messageID)
	startTime := time.Now()
	log.Printf("🔄 [MVC PIPELINE] Starting transformation pipeline for message: %s (type: %s)", messageID, messageType)
	log.Printf("🔍 [MVC PIPELINE] Interface: %s, MessageType: %s", interfaceID, messageType)
	log.Printf("🔍 [MVC PIPELINE] About to set up defer for panic recovery...")

	// Defer panic recovery for transformation pipeline
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🚨 PANIC RECOVERED in transformation pipeline: %v (message: %s)", r, messageID)
			if pe.errorService != nil {
				errCtx := models.NewErrorContext(
					models.StageTransformation,
					models.SeverityCritical,
					models.ErrorTypePanic,
					fmt.Sprintf("Panic during transformation: %v", r),
					"System panic recovered during pipeline execution",
					"",
					models.RecoveryPanicRecovered,
				)
				pe.errorService.CaptureError(interfaceID, messageID, errCtx)
			}
		}
	}()

	// Step 1: Get or create pipeline for this interface + message type
	log.Printf("🔍 [MVC PIPELINE] Calling GetOrCreateDefaultPipeline...")
	pipeline, err := pe.transformationService.GetOrCreateDefaultPipeline(ctx, interfaceID, messageType)
	log.Printf("🔍 [MVC PIPELINE] GetOrCreateDefaultPipeline returned: pipeline=%v, err=%v", pipeline != nil, err)
	if err != nil {
		log.Printf("❌ [MVC PIPELINE] Failed to get transformation pipeline for interface %s, message type %s: %v",
			interfaceID, messageType, err)

		if pe.errorService != nil {
			errCtx := models.NewErrorContext(
				models.StageTransformation,
				models.SeverityError,
				models.ErrorTypeTransformation,
				"Failed to get transformation pipeline",
				err.Error(),
				"",
				models.RecoverySkipped,
			)
			pe.errorService.CaptureError(interfaceID, messageID, errCtx)
		}
		return
	}

	if pipeline == nil {
		log.Printf("ℹ️  [MVC PIPELINE] No transformation pipeline configured for interface %s, message type %s - skipping",
			interfaceID, messageType)
		return
	}

	log.Printf("📋 [MVC PIPELINE] Executing pipeline: %s (%d steps)", pipeline.PipelineName, len(pipeline.Steps))

	// Step 2: Execute pipeline (Model layer handles business logic)
	result, err := pe.transformationService.ExecutePipeline(ctx, pipeline, parseResult.ParsedJSON)
	if err != nil {
		log.Printf("❌ Pipeline execution failed for message %s: %v", messageID, err)

		if pe.errorService != nil {
			errCtx := models.NewErrorContext(
				models.StageTransformation,
				models.SeverityError,
				models.ErrorTypeTransformation,
				"Pipeline execution failed",
				err.Error(),
				"",
				models.RecoveryMarkedFailed,
			)
			pe.errorService.CaptureError(interfaceID, messageID, errCtx)
		}
		return
	}

	executionTime := time.Since(startTime)
	log.Printf("✅ Transformation completed for %s in %dms (status: %s)",
		messageID, executionTime.Milliseconds(), result.Status)

	// Step 3: Store transformed output in MongoDB (View layer)
	log.Printf("🔍 [MVC PIPELINE] Checking storage conditions: mongoService=%v, status=%s", pe.mongoService != nil, result.Status)
	if pe.mongoService != nil && result.Status == "completed" {
		log.Printf("🔍 [MVC PIPELINE] Calling storeTransformedMessage for interface %s...", interfaceID)
		err = pe.storeTransformedMessage(ctx, interfaceID, messageID, messageType, result)
		if err != nil {
			log.Printf("⚠️  [MVC PIPELINE] Failed to store transformed message: %v", err)
		} else {
			log.Printf("💾 [MVC PIPELINE] Transformed message stored in MongoDB: %s", messageID)
		}
	} else {
		log.Printf("⚠️  [MVC PIPELINE] Skipping storage: mongoService=%v, status=%s", pe.mongoService != nil, result.Status)
	}

	// Step 4: Deliver to destination endpoint (if delivery payload exists)
	if result.Status == "completed" && result.DeliveryPayload != nil {
		log.Printf("📤 [MVC PIPELINE] Initiating delivery to destination: %s", result.DeliveryPayload.DestinationEndpoint)
		go pe.deliverMessage(ctx, interfaceID, messageID, result.DeliveryPayload)
	}
}

// storeTransformedMessage stores the transformation output in MongoDB (View layer)
func (pe *ProcessingEngine) storeTransformedMessage(
	ctx context.Context,
	interfaceID string,
	messageID string,
	messageType string,
	result *models.TransformationExecutionResult,
) error {
	collectionName := fmt.Sprintf("transformed_messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))

	transformedDoc := map[string]interface{}{
		"message_id":                 messageID,
		"correlation_id":             result.CorrelationID,
		"interface_id":               interfaceID,
		"message_type":               messageType,
		"transformation_pipeline_id": result.PipelineID,
		"transformation_status":      result.Status,
		"created_at":                 time.Now(),
		"completed_at":               result.CompletedAt,
		"transformation_time_ms":     int32(result.ExecutionTime.Milliseconds()), // Convert to int32 for MongoDB schema
		"error_count":                len(result.Errors),
		"errors":                     result.Errors,
	}

	// UNIVERSAL STORAGE DESIGN - Works for all message types
	// The delivery payload is the UNIVERSAL container for all transformed content

	if result.DeliveryPayload != nil {
		// Store the complete delivery payload (universal for all formats)
		transformedDoc["delivery_payload"] = result.DeliveryPayload

		// Store top-level metadata for easy querying
		transformedDoc["delivery_status"] = result.DeliveryPayload.DeliveryStatus
		transformedDoc["destination_type"] = result.DeliveryPayload.DestinationType
		transformedDoc["destination_endpoint"] = result.DeliveryPayload.DestinationEndpoint
		transformedDoc["format"] = result.DeliveryPayload.Format
		transformedDoc["format_version"] = result.DeliveryPayload.FormatVersion
		transformedDoc["message_type"] = result.DeliveryPayload.MessageType
		transformedDoc["resource_type"] = result.DeliveryPayload.ResourceType

		// Extract the actual transformed content from transmission payload
		// This is format-agnostic - works for FHIR, HL7, JSON, XML, CSV, EDI, etc.
		if result.DeliveryPayload.Transmission != nil {
			// Decode the transmission content based on format
			switch result.DeliveryPayload.Format {
			case "fhir":
				// FHIR: Parse the JSON bytes back to map for storage
				var fhirContent map[string]interface{}
				if err := json.Unmarshal(result.DeliveryPayload.Transmission.Content, &fhirContent); err == nil {
					transformedDoc["transformed_content"] = fhirContent

					// If it's a FHIR Bundle, also store it in fhir_bundle for backward compatibility
					if resourceType, ok := fhirContent["resourceType"].(string); ok && resourceType == "Bundle" {
						transformedDoc["fhir_bundle"] = fhirContent

						// Extract individual resources
						if entries, ok := fhirContent["entry"].([]interface{}); ok {
							resources := make([]interface{}, 0, len(entries))
							for _, entry := range entries {
								if entryMap, ok := entry.(map[string]interface{}); ok {
									if resource, ok := entryMap["resource"]; ok {
										resources = append(resources, resource)
									}
								}
							}
							transformedDoc["fhir_resources"] = resources
						}
					}
				} else {
					// Store as raw bytes if parsing fails
					transformedDoc["transformed_content_raw"] = result.DeliveryPayload.Transmission.Content
				}

			case "hl7v2", "hl7":
				// HL7: Store as string
				transformedDoc["transformed_content"] = string(result.DeliveryPayload.Transmission.Content)
				transformedDoc["hl7_message"] = string(result.DeliveryPayload.Transmission.Content)

			case "json":
				// JSON: Parse back to map
				var jsonContent map[string]interface{}
				if err := json.Unmarshal(result.DeliveryPayload.Transmission.Content, &jsonContent); err == nil {
					transformedDoc["transformed_content"] = jsonContent
				} else {
					transformedDoc["transformed_content_raw"] = result.DeliveryPayload.Transmission.Content
				}

			case "xml":
				// XML: Store as string
				transformedDoc["transformed_content"] = string(result.DeliveryPayload.Transmission.Content)
				transformedDoc["xml_content"] = string(result.DeliveryPayload.Transmission.Content)

			case "csv":
				// CSV: Store as string
				transformedDoc["transformed_content"] = string(result.DeliveryPayload.Transmission.Content)
				transformedDoc["csv_content"] = string(result.DeliveryPayload.Transmission.Content)

			case "edi":
				// EDI: Store as string
				transformedDoc["transformed_content"] = string(result.DeliveryPayload.Transmission.Content)
				transformedDoc["edi_content"] = string(result.DeliveryPayload.Transmission.Content)

			default:
				// Unknown format: Store raw bytes
				transformedDoc["transformed_content_raw"] = result.DeliveryPayload.Transmission.Content
			}
		}

		log.Printf("📦 [STORAGE] Delivery payload stored: format=%s, destination=%s, size=%d bytes",
			result.DeliveryPayload.Format, result.DeliveryPayload.DestinationType,
			len(result.DeliveryPayload.Transmission.Content))
	} else {
		// Fallback: No delivery payload (shouldn't happen in NEW MVC pipeline)
		// Store whatever is in result.Output for backward compatibility
		if result.Output != nil {
			transformedDoc["transformed_content"] = result.Output
			log.Printf("⚠️  [STORAGE] No delivery payload - storing raw output (legacy mode)")
		}
	}

	// Store in MongoDB using the mongoService
	collection := pe.mongoService.GetCollection(collectionName)
	_, err := collection.InsertOne(ctx, transformedDoc)
	if err != nil {
		return fmt.Errorf("failed to insert transformed message: %w", err)
	}

	return nil
}

// updateMongoSyncStatus updates the mongo_synced flag in PostgreSQL
func (pe *ProcessingEngine) updateMongoSyncStatus(interfaceID string, messageID string, synced bool) {
	tableName := fmt.Sprintf("messages_intf_%s", sanitizeInterfaceID(interfaceID))
	query := fmt.Sprintf(`UPDATE %s SET mongo_synced = $1 WHERE message_id = $2`, tableName)

	_, err := pe.db.Exec(query, synced, messageID)
	if err != nil {
		log.Printf("⚠️  Failed to update mongo_synced status: %v", err)
	}
}

// deliverMessage sends the transformed message to the destination endpoint
func (pe *ProcessingEngine) deliverMessage(ctx context.Context, interfaceID string, messageID string, payload *models.DeliveryPayload) {
	log.Printf("🚀 [DELIVERY] Starting delivery for message %s to %s", messageID, payload.DestinationEndpoint)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", payload.DestinationEndpoint, bytes.NewReader(payload.Transmission.Content))
	if err != nil {
		log.Printf("❌ [DELIVERY] Failed to create HTTP request: %v", err)
		pe.updateDeliveryStatus(interfaceID, messageID, "failed", err.Error())
		return
	}

	// Add headers from delivery payload
	for key, value := range payload.Transmission.Headers {
		req.Header.Set(key, value)
		log.Printf("🔑 [DELIVERY] Header: %s = %s", key, maskSensitiveHeader(key, value))
	}

	// Execute HTTP request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	startTime := time.Now()
	resp, err := client.Do(req)
	deliveryTime := time.Since(startTime)

	if err != nil {
		log.Printf("❌ [DELIVERY] HTTP request failed after %dms: %v", deliveryTime.Milliseconds(), err)
		pe.updateDeliveryStatus(interfaceID, messageID, "failed", err.Error())
		return
	}
	defer resp.Body.Close()

	// Read response body
	respBody, _ := io.ReadAll(resp.Body)

	// Check response status
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("✅ [DELIVERY] Message delivered successfully in %dms (status: %d)", deliveryTime.Milliseconds(), resp.StatusCode)
		log.Printf("📥 [DELIVERY] Response: %s", string(respBody))
		pe.updateDeliveryStatus(interfaceID, messageID, "delivered", fmt.Sprintf("HTTP %d", resp.StatusCode))
	} else {
		log.Printf("⚠️  [DELIVERY] Delivery failed with HTTP %d after %dms", resp.StatusCode, deliveryTime.Milliseconds())
		log.Printf("📥 [DELIVERY] Error response: %s", string(respBody))
		pe.updateDeliveryStatus(interfaceID, messageID, "failed", fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody)))
	}
}

// maskSensitiveHeader masks sensitive header values for logging
func maskSensitiveHeader(key, value string) string {
	key = strings.ToLower(key)
	if key == "authorization" || key == "x-api-key" {
		if len(value) > 10 {
			return value[:10] + "..."
		}
		return "***"
	}
	return value
}

// updateDeliveryStatus updates the delivery status in the transformed messages collection
func (pe *ProcessingEngine) updateDeliveryStatus(interfaceID string, messageID string, status string, details string) {
	if pe.mongoService == nil {
		return
	}

	collectionName := fmt.Sprintf("transformed_messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))
	collection := pe.mongoService.GetCollection(collectionName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"message_id": messageID}
	update := bson.M{
		"$set": bson.M{
			"delivery_status":  status,
			"delivery_details": details,
			"delivered_at":     time.Now(),
		},
	}

	_, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Printf("⚠️  [DELIVERY] Failed to update delivery status: %v", err)
	}
}
