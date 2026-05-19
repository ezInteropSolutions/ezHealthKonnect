// processing/engine_message_processor.go
// Message processing for ProcessingEngine

package processing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services"
	"ezhealthkonnect/services/connectors"
	"ezhealthkonnect/services/metrics"
	"ezhealthkonnect/services/parsers"
	"ezhealthkonnect/services/storage"
)

// processMessages processes incoming messages from ANY connector channel (ALL 32 connectors)
// UNIFIED MODEL: Uses models.InboundMessage for all connector types
func (pe *ProcessingEngine) processMessages(interfaceID string, messageChan <-chan *models.InboundMessage) {
	log.Printf("📨 Message processor started for interface %s", interfaceID)

	for msg := range messageChan {
		// ── Format-agnostic batch detection ──────────────────────────────────
		// Any connector (TCP/MLLP, file, HTTP, SFTP, Kafka, …) can deliver a
		// payload that contains multiple independent messages.  splitBatchPayload
		// handles HL7 v2 (multiple MSH), JSON arrays, and EDI X12 transaction
		// sets.  Each part is processed as its own InboundMessage so it is
		// stored, ACK'd, and transformed independently.
		result := splitBatchPayload(msg.Content)
		if len(result.Parts) > 1 {
			log.Printf("📦 [batch] interface=%s: %d %s messages in payload %s — splitting",
				interfaceID, len(result.Parts), result.Format, msg.MessageID)
			for i, raw := range result.Parts {
				clone := *msg
				clone.Content = raw
				clone.MessageSize = len(raw)
				clone.MessageID = msg.MessageID + "-part" + strconv.Itoa(i+1)
				// Re-extract the message type from the individual segment so
				// downstream routing and transformation use the correct type.
				// For HL7 v2 this reads MSH-9; for other formats the original
				// MessageType from the connector header is preserved.
				if result.Format == BatchFormatHL7v2 {
					if mt := extractHL7MessageType(raw); mt != "" {
						clone.MessageType = mt
					}
				}
				pe.processSingleMessage(interfaceID, &clone)
			}
			continue
		}

		pe.processSingleMessage(interfaceID, msg)
	}

	log.Printf("📪 Message processor stopped for interface %s", interfaceID)
}

// processSingleMessage runs the full store → ACK → async-parse pipeline for one
// discrete HL7 (or other format) message.  Extracted from the processMessages
// loop so the batch-split path and the single-message path share identical logic.
func (pe *ProcessingEngine) processSingleMessage(interfaceID string, msg *models.InboundMessage) {
	startTime := time.Now()

	log.Printf("📥 Processing message for interface %s: ID=%s, Size=%d bytes",
		interfaceID, msg.MessageID, len(msg.Content))

	// STEP 0: Message family filter — NACK and drop before storing if the
	// message type is not in the interface's accepted_message_families list.
	if !pe.isMessageFamilyAccepted(interfaceID, msg.MessageType) {
		log.Printf("🚫 Interface %s rejected message %s: type %q not in accepted families",
			interfaceID, msg.MessageID, msg.MessageType)
		pe.validationMutex.RLock()
		vc, hasVC := pe.validationConnectors[interfaceID]
		pe.validationMutex.RUnlock()
		if hasVC && vc.SupportsValidationFeedback() {
			feedback := models.NewValidationFeedback(
				msg.MessageID, interfaceID,
				models.ValidationModeStrictReject,
				"rejected",
				[]models.FieldValidationError{
					{Field: "MSH.9", Type: "UNSUPPORTED_MESSAGE_TYPE",
						Message: fmt.Sprintf("Message type %q is not accepted by this interface", msg.MessageType)},
				},
				0, "",
			)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := vc.SendValidationResponse(ctx, feedback); err != nil {
				log.Printf("⚠️  NACK send failed for interface %s: %v", interfaceID, err)
			}
		}
		return
	}

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
		return
	}

	processingTime := time.Since(startTime)
	log.Printf("✅ Message stored in PostgreSQL for interface %s: ID=%s (took %v)",
		interfaceID, msg.MessageID, processingTime)

	// ACK-AFTER-STORE: Send AA to sender now that message is durable in PostgreSQL.
	// This prevents the gap where the sender had ACK but PostgreSQL write never happened.
	// NACK on store failure is already handled above (return skips this).
	pe.sendACKAfterStore(interfaceID, msg)

	// Prometheus: count durably-stored messages
	if metrics.MessagesReceived != nil {
		metrics.MessagesReceived.WithLabelValues(interfaceID, msg.SourceType).Inc()
	}

	// STEP 2: Store in MongoDB + Parse to JSON — submit to bounded worker pool (P9 backpressure)
	pool := backpressureRegistry().GetOrCreate(interfaceID)
	msgCopy := msg // capture for closure
	ifID := interfaceID
	submitted := pool.Submit(func() {
		pe.storeAndParseWithRecovery(ifID, msgCopy)
	})
	if !submitted {
		// Queue full — send NACK via ValidationAwareConnector (e.g. MLLP NAK)
		log.Printf("⚠️  [backpressure] queue full for interface %s (depth=%d/%d), dropping message %s",
			interfaceID, pool.QueueDepth(), pool.MaxQueue(), msg.MessageID)
		pe.validationMutex.RLock()
		vc, hasVC := pe.validationConnectors[interfaceID]
		pe.validationMutex.RUnlock()
		if hasVC && vc.SupportsValidationFeedback() {
			feedback := models.NewValidationFeedback(
				msg.MessageID, interfaceID,
				models.ValidationModeStrictReject,
				"rejected",
				[]models.FieldValidationError{
					{Field: "_backpressure", Type: "QUEUE_FULL", Message: "Server busy — queue at capacity"},
				},
				0, "",
			)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if err := vc.SendValidationResponse(ctx, feedback); err != nil {
					log.Printf("⚠️  NACK send failed for interface %s: %v", interfaceID, err)
				}
			}()
		}
	}

	// Update interface statistics
	pe.updateInterfaceStats(interfaceID, processingTime)
}

// extractHL7MessageType returns the MSH-9 value (e.g. "ADT^A01") from a raw HL7
// message string.  Returns "" when the segment cannot be parsed.
func extractHL7MessageType(raw string) string {
	// Normalise line endings
	norm := strings.ReplaceAll(raw, "\r\n", "\r")
	norm = strings.ReplaceAll(norm, "\n", "\r")
	for _, seg := range strings.Split(norm, "\r") {
		seg = strings.TrimSpace(seg)
		if !strings.HasPrefix(seg, "MSH") {
			continue
		}
		if len(seg) < 4 {
			return ""
		}
		fieldSep := string(seg[3])
		fields := strings.Split(seg, fieldSep)
		// MSH.9 is field index 8 (MSH.1=separator stored at [3], fields[1]=encoding chars)
		if len(fields) > 8 {
			return strings.TrimSpace(fields[8])
		}
		return ""
	}
	return ""
}

// storeMessage stores the raw message in PostgreSQL
func (pe *ProcessingEngine) storeMessage(interfaceID string, msg *models.InboundMessage) error {
	tableName := fmt.Sprintf("messages_intf_%s", sanitizeInterfaceID(interfaceID))

	// UNIFIED MODEL: Use models.InboundMessage fields directly
	sourceType := msg.SourceType       // tcp_mllp, http_rest, file_listener, etc.
	sourceEndpoint := msg.SourceEndpoint // tcp://0.0.0.0:2575, http://..., etc.
	sourceIP := msg.SourceIP            // Client IP address
	messageType := msg.MessageType      // HL7v2, FHIR, JSON, XML, etc.

	// raw_message column is intentionally NULL here.
	// Raw bytes are written to object storage in storeAndParse() and the URI
	// is stored in raw_content_uri.  Keeping it NULL avoids duplicating
	// potentially large payloads inside PostgreSQL.
	query := fmt.Sprintf(`
		INSERT INTO %s (
			message_id, interface_id, status,
			received_at, source_type, source_endpoint, source_ip,
			message_type, message_size, message_encoding,
			created_at, updated_at
		) VALUES (
			$1, $2, 'received',
			$3, $4, $5, $6,
			$7, $8, $9,
			NOW(), NOW()
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
		sourceType,
		sourceEndpoint,
		sourceIP,
		messageType,
		msg.MessageSize,
		encoding,
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

	// Inject ProcessingEngine into context for validation feedback
	ctx = context.WithValue(ctx, "processing_engine", pe)
	ctx = context.WithValue(ctx, "message_id", msg.MessageID)
	ctx = context.WithValue(ctx, "interface_id", interfaceID)

	// Get dynamic metadata from message
	messageType := msg.MessageType // Default: hl7, fhir, etc.
	if mt, ok := msg.Headers["message_type"]; ok && mt != "" {
		messageType = mt
	}

	sourceType := msg.SourceType // Default: tcp, http, etc.
	if st, ok := msg.Headers["source_type"]; ok && st != "" {
		sourceType = st
	}

	// Create processing logger if debug logging enabled
	logger := pe.createLogger(interfaceID, msg.MessageID, msg.CorrelationID, messageType)
	if logger != nil {
		logger.Info(services.LogCategoryConnection, "Message received", map[string]interface{}{
			"source_type": sourceType,
			"message_size": len(msg.Content),
			"source_endpoint": msg.SourceType,
		})
	}

	// STEP 1: Store raw message in object storage (S3/MinIO/local)
	if pe.objectStorage != nil {
		rawBytes := []byte(msg.Content)
		rawURI, err := pe.objectStorage.StoreRawMessage(ctx, interfaceID, msg.MessageID, rawBytes)
		if err != nil {
			log.Printf("⚠️  Failed to store raw message in object storage: %v", err)
			if pe.errorService != nil {
				errCtx := models.NewErrorContext(
					models.StageDatabase,
					models.SeverityWarning,
					models.ErrorTypeDatabase,
					"Failed to store raw message in object storage",
					err.Error(),
					"",
					models.RecoveryNone,
				)
				pe.errorService.CaptureError(interfaceID, msg.MessageID, errCtx)
			}
		} else {
			log.Printf("💾 Raw message stored in object storage: %s → %s", msg.MessageID, rawURI)
			// Update PostgreSQL raw_content_uri (best-effort — column may not exist on older tables)
			pe.updateStorageURI(interfaceID, msg.MessageID, "raw_content_uri", rawURI)

			// Append a log entry
			_ = pe.objectStorage.AppendLog(ctx, interfaceID, msg.MessageID, storage.LogEntry{
				Level:   "info",
				Stage:   "receive",
				Message: "Raw message stored in object storage",
				Fields:  map[string]interface{}{"source_type": sourceType, "message_size": len(rawBytes), "uri": rawURI},
			})
		}
	}

	// STEP 2: Parse to JSON (if parser service available)
	// Note: For FHIR messages, content is already JSON - skip parsing
	if sourceType == "http_fhir" {
		log.Printf("⏭️  Skipping JSON conversion for FHIR message: %s", msg.MessageID)

		if logger != nil {
			logger.Info(services.LogCategoryParsing, "FHIR message received (sink interface)", map[string]interface{}{
				"message_type": messageType,
				"skip_parsing": true,
			})
		}

		// Store the FHIR bundle as-is in object storage parsed content
		if pe.objectStorage != nil {
			var fhirBundle map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Content), &fhirBundle); err == nil {
				parsedURI, err := pe.objectStorage.StoreParsedContent(ctx, interfaceID, msg.MessageID, fhirBundle)
				if err != nil {
					log.Printf("⚠️  Failed to store FHIR bundle in object storage: %v", err)
					if logger != nil {
						logger.Error(services.LogCategoryParsing, "Failed to store FHIR bundle in object storage", map[string]interface{}{
							"error": err.Error(),
						})
					}
				} else {
					log.Printf("✅ FHIR bundle stored in object storage: %s → %s", msg.MessageID, parsedURI)
					pe.updateStorageURI(interfaceID, msg.MessageID, "parsed_content_uri", parsedURI)
					if logger != nil {
						logger.Info(services.LogCategoryParsing, "FHIR bundle saved to object storage", map[string]interface{}{
							"message_id": msg.MessageID,
							"uri":        parsedURI,
						})
					}
				}
			}
		}

		// Update PostgreSQL parsing status
		tableName := fmt.Sprintf("messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))
		query := fmt.Sprintf(`UPDATE %s SET parsing_status = 'completed', parsed_at = $1
			WHERE message_id = $2`, tableName)
		_, dbErr := pe.db.Exec(query, time.Now(), msg.MessageID)
		if dbErr != nil {
			log.Printf("⚠️  Failed to update parsing status in PostgreSQL: %v", dbErr)
		}

		// Build a ParserResult so the transformation pipeline receives FHIR data at root
		// with _format set — identical envelope contract to HL7 messages.
		// Use FHIRParserService (same as the test pipeline) so enhancedFields are
		// populated and the output payload matches what the test pipeline shows.
		if pe.transformationService != nil {
			fhirParser := parsers.NewFHIRParserService()
			parseResult := fhirParser.Parse(msg.Content)
			if parseResult.Success {
				fhirParsedJSON := parseResult.ParsedJSON
				fhirParsedJSON["_format"] = string(models.FormatFHIR)
				if len(parseResult.EnhancedFields) > 0 {
					fhirParsedJSON["enhancedFields"] = parseResult.EnhancedFields
				}
				fhirResult := &models.ParserResult{
					Success:    true,
					Format:     models.FormatFHIR,
					ParsedJSON: fhirParsedJSON,
					Metadata: models.ParserMetadata{
						MessageType: messageType,
						ParsedAt:    time.Now(),
					},
				}
				go pe.executeTransformationPipelineWithLogger(ctx, interfaceID, msg.MessageID, messageType, fhirResult, logger)
			} else {
				log.Printf("⚠️  FHIR JSON parse failed for %s — skipping pipeline: %v", msg.MessageID, parseResult.Error)
			}
		}

	} else if pe.parserService != nil {
		log.Printf("🔄 Starting JSON conversion for message: %s (type: %s)", msg.MessageID, messageType)

		if logger != nil {
			logger.LogParsingStart(messageType)
		}

		result, err := pe.parserService.ParseToJSON(ctx, msg.MessageID, interfaceID, msg.Content)
		if err != nil {
			log.Printf("❌ JSON conversion failed for %s: %v", msg.MessageID, err)

			if logger != nil {
				logger.LogParsingError(messageType, err.Error(), 0)
			}

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

			if logger != nil {
				logger.LogParsingComplete(messageType, result.ParsingTime.Milliseconds(), len(result.ParsedJSON))
			}

			// Store parsed JSON in object storage
			if pe.objectStorage != nil && result.ParsedJSON != nil {
				parsedURI, err := pe.objectStorage.StoreParsedContent(ctx, interfaceID, msg.MessageID, result.ParsedJSON)
				if err != nil {
					log.Printf("⚠️  Failed to store parsed content in object storage: %v", err)
				} else {
					pe.updateStorageURI(interfaceID, msg.MessageID, "parsed_content_uri", parsedURI)
				}
			}

			// STEP 3: Trigger transformation pipeline (MVC + OOB pattern)
			if pe.transformationService != nil {
				go pe.executeTransformationPipelineWithLogger(ctx, interfaceID, msg.MessageID, messageType, result, logger)
			} else {
				log.Printf("⚠️  Transformation service not available - skipping transformation")
			}
		}
	}
}

// executeTransformationPipelineWithLogger executes transformation with logging support
func (pe *ProcessingEngine) executeTransformationPipelineWithLogger(
	ctx context.Context,
	interfaceID string,
	messageID string,
	messageType string,
	parseResult *models.ParserResult,
	logger *services.ProcessingLogger,
) {
	if logger != nil {
		logger.Info(services.LogCategoryTransformation, "Starting transformation pipeline", map[string]interface{}{
			"message_type": messageType,
		})
	}

	// Call existing implementation
	pe.executeTransformationPipeline(ctx, interfaceID, messageID, messageType, parseResult)
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

	// Step 1: Get existing pipeline for this interface + message type (do NOT auto-create)
	// Pipelines should only be created via wizard or manual pipeline builder
	log.Printf("🔍 [MVC PIPELINE] Getting transformation pipeline...")
	pipeline, err := pe.transformationService.GetPipeline(ctx, interfaceID, messageType)
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
		log.Printf("⏭️  [MVC PIPELINE] No transformation pipeline configured for interface %s, message type %s - skipping transformation",
			interfaceID, messageType)
		return
	}

	log.Printf("📋 [MVC PIPELINE] Executing pipeline: %s (%d steps)", pipeline.PipelineName, len(pipeline.Steps))

	// STATUS: mark as 'processing' before pipeline runs
	pe.updateMessageStatus(interfaceID, messageID, "processing", map[string]interface{}{
		"processing_started_at": time.Now(),
	})

	// Step 2: Wire parsed_format into pipeline message envelope as _format.
	// enrichMessageEnvelope() in ExecutePipeline will build _semantic_index and _sensitivity_map.
	if parseResult.ParsedJSON != nil && parseResult.Format != "" {
		parseResult.ParsedJSON["_format"] = string(parseResult.Format)
	}

	// Step 2: Execute pipeline (Model layer handles business logic)
	// Inject delivery callback so OutboundConnectorExecutor can own its DB update.
	// The executor reads this from context and calls it after connector.Send() completes,
	// giving us correct delivery_status regardless of how many outbound steps exist.
	ctx = context.WithValue(ctx, "delivery_status_fn", models.DeliveryStatusFn(pe.updateDeliveryStatus))
	ctx = context.WithValue(ctx, "message_id", messageID)
	ctx = context.WithValue(ctx, "interface_id", interfaceID)
	// Inject store-outbound callback so OutboundConnectorExecutor can persist the exact
	// payload sent to each connector (full content, no truncation) in object storage.
	if pe.objectStorage != nil {
		ctx = context.WithValue(ctx, "store_outbound_fn", models.StoreOutboundFn(func(ifaceID, msgID, content, ct string) string {
			uri, err := pe.objectStorage.StoreOutboundPayload(context.Background(), ifaceID, msgID, content, ct)
			if err != nil {
				log.Printf("⚠️  [Engine] StoreOutboundPayload: %v", err)
				return ""
			}
			pe.updateStorageURI(ifaceID, msgID, "outbound_content_uri", uri)
			return uri
		}))
	}

	result, err := pe.transformationService.ExecutePipeline(ctx, pipeline, parseResult.ParsedJSON)
	if err != nil {
		log.Printf("❌ Pipeline execution failed for message %s: %v", messageID, err)

		// STATUS: mark as 'failed'
		pe.updateMessageStatus(interfaceID, messageID, "failed", map[string]interface{}{
			"last_error_message":     err.Error(),
			"error_count":            1,
			"processing_completed_at": time.Now(),
		})

		// Prometheus: count pipeline errors
		if metrics.MessagesProcessed != nil {
			metrics.MessagesProcessed.WithLabelValues(interfaceID, "failed").Inc()
		}

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

	// STATUS: mark as 'processed' (pipeline completed) or 'failed'.
	// 'processed' means the transformation pipeline ran to completion.
	// Actual delivery to a downstream system is tracked separately in delivery_status
	// by the connector.outbound pipeline step via updateDeliveryStatus().
	finalStatus := "processed"
	if result.Status != "completed" {
		finalStatus = "failed"
	}
	pe.updateMessageStatus(interfaceID, messageID, finalStatus, map[string]interface{}{
		"processing_completed_at": time.Now(),
		"processing_time_ms":      executionTime.Milliseconds(),
	})

	// delivery_status is written directly by OutboundConnectorExecutor via the
	// delivery_status_fn callback injected into context above.
	// If the pipeline had no connector.outbound step at all, delivery_status is
	// still 'pending' here — flip it to 'not_required' so the UI is meaningful.
	if finalStatus == "processed" {
		tbl := fmt.Sprintf("messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))
		_, _ = pe.db.Exec(
			fmt.Sprintf(`UPDATE %s SET delivery_status = 'not_required' WHERE message_id = $1 AND delivery_status = 'pending'`, tbl),
			messageID,
		)
	}

	// Prometheus: record pipeline duration and outcome
	if metrics.ProcessingDuration != nil {
		metrics.ProcessingDuration.WithLabelValues(interfaceID).Observe(executionTime.Seconds())
	}
	if metrics.MessagesProcessed != nil {
		metrics.MessagesProcessed.WithLabelValues(interfaceID, finalStatus).Inc()
	}

	// Step 3: Store transformed output in object storage (View layer)
	log.Printf("🔍 [MVC PIPELINE] Checking storage conditions: objectStorage=%v, status=%s", pe.objectStorage != nil, result.Status)
	if pe.objectStorage != nil && result.Status == "completed" {
		log.Printf("🔍 [MVC PIPELINE] Calling storeTransformedMessage for interface %s...", interfaceID)
		err = pe.storeTransformedMessage(ctx, interfaceID, messageID, messageType, result)
		if err != nil {
			log.Printf("⚠️  [MVC PIPELINE] Failed to store transformed message: %v", err)
		} else {
			log.Printf("💾 [MVC PIPELINE] Transformed message stored in object storage: %s", messageID)
		}
	} else {
		log.Printf("⚠️  [MVC PIPELINE] Skipping storage: objectStorage=%v, status=%s", pe.objectStorage != nil, result.Status)
	}

	// Step 4: Delivery is handled by the connector.outbound pipeline step (seq 295).
	// The OutboundConnectorExecutor calls connector.Send() during ExecutePipeline above.
	// No separate deliverMessage() call is needed here.
}

// storeTransformedMessage stores the transformation output in object storage (View layer)
func (pe *ProcessingEngine) storeTransformedMessage(
	ctx context.Context,
	interfaceID string,
	messageID string,
	messageType string,
	result *models.TransformationExecutionResult,
) error {
	transformedDoc := map[string]interface{}{
		"message_id":                 messageID,
		"correlation_id":             result.CorrelationID,
		"interface_id":               interfaceID,
		"message_type":               messageType,
		"transformation_pipeline_id": result.PipelineID,
		"transformation_status":      result.Status,
		"created_at":                 time.Now(),
		"completed_at":               result.CompletedAt,
		"transformation_time_ms":     int32(result.ExecutionTimeNs / 1000000), // Convert nanoseconds to milliseconds
		"error_count":                len(result.Errors),
		"errors":                     result.Errors,
	}

	// Add transformation steps metadata for UI display
	if result.ExecutionLog != nil && len(result.ExecutionLog) > 0 {
		transformedDoc["transformation_metadata"] = map[string]interface{}{
			"steps": result.ExecutionLog,
		}
		log.Printf("📊 [STORAGE] Storing %d transformation steps in MongoDB", len(result.ExecutionLog))
	}

	// Build pipeline_result in the exact same format as the test pipeline API response
	// so clicking a real message shows the same view as running the test pipeline.
	{
		normalizer := models.NewOutputNormalizer()
		steps := make(map[string]interface{})
		stepNameCounts := make(map[string]int)
		for _, stepLog := range result.ExecutionLog {
			stepMetadata := map[string]interface{}{
				"duration_ms": stepLog.DurationMs,
				"success":     stepLog.Success,
			}
			if stepLog.StepOutput != nil && stepLog.StepOutput.ExecutionDetails != nil {
				for k, v := range stepLog.StepOutput.ExecutionDetails {
					stepMetadata[k] = v
				}
			}
			var stepOutput map[string]interface{}
			if stepLog.StepOutput != nil && stepLog.StepOutput.OutputData != nil {
				// JSON round-trip breaks any circular references before normalizing
				if b, err := json.Marshal(stepLog.StepOutput.OutputData); err == nil {
					var clean map[string]interface{}
					if json.Unmarshal(b, &clean) == nil {
						stepOutput = normalizer.NormalizeStepOutput(clean)
					}
				}
			}
			if stepOutput == nil {
				stepOutput = map[string]interface{}{}
			}
			normalizedName := normalizer.NormalizeKey(stepLog.StepName)
			stepNameCounts[normalizedName]++
			stepKey := normalizedName
			if stepNameCounts[normalizedName] > 1 {
				stepKey = fmt.Sprintf("%s_%d", normalizedName, stepNameCounts[normalizedName])
			}
			steps[stepKey] = map[string]interface{}{
				"step_output":   stepOutput,
				"step_metadata": stepMetadata,
			}
		}
		pipelineResult := map[string]interface{}{
			"success": result.Status == "completed",
			"status":  result.Status,
			"steps":   steps,
		}
		if result.Input != nil {
			pipelineResult["input"] = map[string]interface{}{
				"format":       result.Input.Format,
				"message_type": result.Input.MessageType,
				"version":      result.Input.Version,
				"size_bytes":   result.Input.SizeBytes,
			}
		}
		if result.Output != nil {
			pipelineResult["output"] = map[string]interface{}{
				"format":       result.Output.Format,
				"message_type": result.Output.MessageType,
				"version":      result.Output.Version,
				"size_bytes":   result.Output.SizeBytes,
				"payload":      result.Output.Payload,
			}
		}
		transformedDoc["pipeline_result"] = pipelineResult
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

	// Store in object storage
	transformedURI, err := pe.objectStorage.StoreTransformedContent(ctx, interfaceID, messageID, transformedDoc)
	if err != nil {
		return fmt.Errorf("failed to store transformed message in object storage: %w", err)
	}

	// Update PostgreSQL URI column (best-effort)
	pe.updateStorageURI(interfaceID, messageID, "transformed_content_uri", transformedURI)

	return nil
}

// updateStorageURI updates a URI column (raw_content_uri, parsed_content_uri, etc.) in PostgreSQL.
// The column may not exist on older tables; the error is suppressed silently.
func (pe *ProcessingEngine) updateStorageURI(interfaceID, messageID, column, uri string) {
	tableName := fmt.Sprintf("messages_intf_%s", sanitizeInterfaceID(interfaceID))
	query := fmt.Sprintf(`UPDATE %s SET %s = $1 WHERE message_id = $2`, tableName, column)
	if _, err := pe.db.Exec(query, uri, messageID); err != nil {
		// Column may not exist on tables created before V56 migration — non-fatal
		log.Printf("⚠️  updateStorageURI(%s.%s): %v", tableName, column, err)
	}
}

// deliverMessage sends the transformed message to the destination endpoint
func (pe *ProcessingEngine) deliverMessage(ctx context.Context, interfaceID string, messageID string, payload *models.DeliveryPayload) {
	log.Printf("🚀 [DELIVERY] Starting delivery for message %s to %s (type: %s)",
		messageID, payload.DestinationEndpoint, payload.DestinationType)

	// OOB PATTERN: Use connector factory to create outbound connector
	connector, err := pe.connectorFactory.CreateOutbound(payload.DestinationType)
	if err != nil {
		log.Printf("❌ [DELIVERY] Failed to create outbound connector '%s': %v", payload.DestinationType, err)
		pe.updateDeliveryStatus(interfaceID, messageID, "failed", fmt.Sprintf("Connector creation failed: %v", err))
		return
	}

	// Get target connectivity config from database
	var targetConnectivityJSON string
	err = pe.db.QueryRow(`
		SELECT COALESCE(target_connectivity::text, '{}')
		FROM interfaces
		WHERE id = $1
	`, interfaceID).Scan(&targetConnectivityJSON)
	if err != nil {
		log.Printf("❌ [DELIVERY] Failed to load target_connectivity for interface %s: %v", interfaceID, err)
		pe.updateDeliveryStatus(interfaceID, messageID, "failed", fmt.Sprintf("Config load failed: %v", err))
		return
	}

	// Parse target connectivity
	var targetConnectivity map[string]interface{}
	if err := json.Unmarshal([]byte(targetConnectivityJSON), &targetConnectivity); err != nil {
		log.Printf("❌ [DELIVERY] Failed to parse target_connectivity: %v", err)
		pe.updateDeliveryStatus(interfaceID, messageID, "failed", fmt.Sprintf("Config parse failed: %v", err))
		return
	}

	// Merge config with delivery payload
	config := make(map[string]interface{})
	if connConfig, ok := targetConnectivity["config"].(map[string]interface{}); ok {
		for k, v := range connConfig {
			config[k] = v
		}
	}

	// Add endpoint from delivery payload ONLY if not already in config
	// This prevents overriding the correct endpoint from target_connectivity
	if _, hasEndpoint := config["endpoint"]; !hasEndpoint {
		config["endpoint"] = payload.DestinationEndpoint
	}
	config["interface_id"] = interfaceID

	// Convert config to JSON for connector initialization
	configJSON, err := json.Marshal(config)
	if err != nil {
		log.Printf("❌ [DELIVERY] Failed to marshal config: %v", err)
		pe.updateDeliveryStatus(interfaceID, messageID, "failed", fmt.Sprintf("Config marshal failed: %v", err))
		return
	}

	log.Printf("🔍 [DELIVERY] Initializing %s connector with config: %s", payload.DestinationType, string(configJSON))

	// Initialize connector with config
	if err := connector.Initialize(configJSON); err != nil {
		log.Printf("❌ [DELIVERY] Failed to initialize connector: %v", err)
		pe.updateDeliveryStatus(interfaceID, messageID, "failed", fmt.Sprintf("Connector init failed: %v", err))
		return
	}

	// Create outbound message
	outboundMsg := &models.OutboundMessage{
		MessageID:         messageID,
		InterfaceID:       interfaceID,
		Content:           string(payload.Transmission.Content),
		ContentType:       payload.Transmission.ContentType,
		DestinationType:   payload.DestinationType,
		DestinationConfig: config,
		Headers:           payload.Transmission.Headers,
		CreatedAt:         time.Now(),
		MaxRetries:        3,
		Timeout:           30,
	}

	// Send via connector (uses connector-specific delivery logic)
	result, err := connector.Send(ctx, outboundMsg)
	if err != nil {
		log.Printf("❌ [DELIVERY] Connector send failed: %v", err)
		pe.updateDeliveryStatusWithResponse(interfaceID, messageID, "failed", err.Error(), result)
		return
	}

	// Update delivery status based on result
	if result.Success {
		log.Printf("✅ [DELIVERY] Message delivered successfully in %dms (ack: %s)",
			result.DurationMs, result.Acknowledgment)
		pe.updateDeliveryStatusWithResponse(interfaceID, messageID, "delivered", result.Acknowledgment, result)
	} else {
		log.Printf("⚠️ [DELIVERY] Delivery failed: %s", result.ErrorMessage)
		pe.updateDeliveryStatusWithResponse(interfaceID, messageID, "failed", result.ErrorMessage, result)
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

// updateDeliveryStatus updates the delivery status in PostgreSQL.
func (pe *ProcessingEngine) updateDeliveryStatus(interfaceID string, messageID string, status string, details string) {
	if pe.db == nil {
		return
	}
	tableName := fmt.Sprintf("messages_intf_%s", sanitizeInterfaceID(interfaceID))
	query := fmt.Sprintf(`UPDATE %s SET delivery_status = $1, updated_at = NOW() WHERE message_id = $2`, tableName)
	if _, err := pe.db.Exec(query, status, messageID); err != nil {
		log.Printf("⚠️  [DELIVERY] Failed to update delivery status: %v", err)
	}
}

// updateDeliveryStatusWithResponse updates delivery status with full response details
func (pe *ProcessingEngine) updateDeliveryStatusWithResponse(interfaceID string, messageID string, status string, details string, result *connectors.DeliveryResult) {
	log.Printf("🔧 [DEBUG] updateDeliveryStatusWithResponse called: msgID=%s, status=%s", messageID, status)

	// Update PostgreSQL message table
	if pe.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		inputTableName := fmt.Sprintf("messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))

		updateQuery := fmt.Sprintf(`
			UPDATE %s
			SET delivery_status = $1,
			    delivery_attempts = delivery_attempts + 1,
			    updated_at = NOW()
			WHERE message_id = $2
		`, inputTableName)

		_, err := pe.db.ExecContext(ctx, updateQuery, status, messageID)
		if err != nil {
			log.Printf("⚠️  [DELIVERY] Failed to update PostgreSQL delivery status: %v", err)
		} else {
			log.Printf("✅ [DELIVERY] PostgreSQL updated: message_id=%s, delivery_status=%s", messageID, status)
		}
	}
}

// createLogger creates a processing logger for a message.
// Log verbosity is controlled by the per-interface log_level column:
//
//	debug   — log everything (default)
//	info    — errors, warnings, info
//	warning — errors and warnings only
//	error   — errors only
//	off     — no object-storage logging (console only)
func (pe *ProcessingEngine) createLogger(interfaceID, messageID, correlationID, messageType string) *services.ProcessingLogger {
	if pe.db == nil {
		return nil
	}

	var interfaceName string
	var logLevel sql.NullString
	var debugLogging bool

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Read log_level (V59+) with debug_logging fallback for older schemas
	query := `SELECT name, COALESCE(log_level, ''), debug_logging FROM interfaces WHERE id = $1`
	err := pe.db.QueryRowContext(ctx, query, interfaceID).Scan(&interfaceName, &logLevel, &debugLogging)
	if err != nil {
		// Fallback: try without log_level column (pre-V59 schema)
		err2 := pe.db.QueryRowContext(ctx, `SELECT name, debug_logging FROM interfaces WHERE id = $1`,
			interfaceID).Scan(&interfaceName, &debugLogging)
		if err2 != nil {
			log.Printf("⚠️  [CREATE LOGGER] Query failed: %v", err2)
			return nil
		}
		if !debugLogging {
			return nil
		}
		logLevel = sql.NullString{String: "debug", Valid: true}
	}

	// Resolve effective level: log_level column takes precedence; fall back to debug_logging bool
	level := logLevel.String
	if level == "" {
		if debugLogging {
			level = "debug"
		} else {
			level = "debug" // default: log everything per user preference
		}
	}

	if level == "off" {
		return nil // explicitly disabled
	}

	debugMode := level == "debug" // controls whether Info/Debug entries are written

	return services.NewProcessingLogger(
		interfaceID,
		interfaceName,
		messageID,
		correlationID,
		messageType,
		debugMode,
		pe.objectStorage,
	)
}

// ==================== DURABILITY HELPERS (Phase 2) ====================

// sendACKAfterStore sends AA to the inbound connector after the message is safely written to PostgreSQL.
// This closes the ACK-before-store gap: the sender only gets AA once the message is durable.
// If the connector does not support validation feedback, this is a no-op.
func (pe *ProcessingEngine) sendACKAfterStore(interfaceID string, msg *models.InboundMessage) {
	pe.validationMutex.RLock()
	vc, hasVC := pe.validationConnectors[interfaceID]
	pe.validationMutex.RUnlock()

	if !hasVC || !vc.SupportsValidationFeedback() {
		return
	}

	feedback := models.NewValidationFeedback(
		msg.MessageID, interfaceID,
		models.ValidationModeStrictReject,
		"accepted",
		nil,
		0, "",
	)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := vc.SendValidationResponse(ctx, feedback); err != nil {
			log.Printf("⚠️  ACK send failed for message %s interface %s: %v", msg.MessageID, interfaceID, err)
		} else {
			log.Printf("✅ [ACK-AFTER-STORE] AA sent for message %s", msg.MessageID)
		}
	}()
}

// updateMessageStatus updates the processing status of a message in its interface table.
// Called at key lifecycle points: processing started, delivered, failed.
func (pe *ProcessingEngine) updateMessageStatus(interfaceID, messageID, status string, extraFields map[string]interface{}) {
	if pe.db == nil {
		return
	}

	tableName := fmt.Sprintf("messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))

	setClauses := "status = $1, updated_at = NOW()"
	args := []interface{}{status, messageID}
	argIdx := 3

	for col, val := range extraFields {
		setClauses += fmt.Sprintf(", %s = $%d", col, argIdx)
		args = append(args, val)
		argIdx++
	}
	// message_id is $2
	query := fmt.Sprintf("UPDATE %s SET %s WHERE message_id = $2", tableName, setClauses)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := pe.db.ExecContext(ctx, query, args...); err != nil {
		log.Printf("⚠️  Failed to update message status to '%s' for %s: %v", status, messageID, err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Message family filter
// ─────────────────────────────────────────────────────────────────────────────

// isMessageFamilyAccepted returns true when the message type is allowed by the
// interface's accepted_message_families list, or when no filter is configured.
// Non-HL7 message types (empty string) always pass through.
func (pe *ProcessingEngine) isMessageFamilyAccepted(interfaceID, messageType string) bool {
	pe.familyFilterMu.RLock()
	families, hasFamilies := pe.familyFilter[interfaceID]
	pe.familyFilterMu.RUnlock()

	// No filter configured → accept all.
	if !hasFamilies || len(families) == 0 {
		return true
	}
	// Empty / non-HL7 message type → pass through (FHIR, JSON, etc.).
	if messageType == "" {
		return true
	}

	for _, f := range families {
		if messageMatchesFamily(messageType, f) {
			return true
		}
	}
	return false
}

// messageMatchesFamily returns true when msgType belongs to the given family spec.
//   - Family without ^ (e.g. "ADT")  → matches any ADT^Axx event.
//   - Family with ^    (e.g. "MFN^M02") → exact match only (heterogeneous families).
func messageMatchesFamily(msgType, family string) bool {
	msgType = strings.ToUpper(strings.TrimSpace(msgType))
	family = strings.ToUpper(strings.TrimSpace(family))
	if strings.Contains(family, "^") {
		return msgType == family
	}
	return msgType == family || strings.HasPrefix(msgType, family+"^")
}

// ReloadFamilyFilter re-reads accepted_message_families for an interface from the
// database.  Called by the API layer after an interface config update so the
// in-memory cache stays in sync without requiring a full engine restart.
func (pe *ProcessingEngine) ReloadFamilyFilter(interfaceID string) {
	var familyFilterJSON sql.NullString
	err := pe.db.QueryRow(
		`SELECT accepted_message_families::text FROM interfaces WHERE id = $1`,
		interfaceID,
	).Scan(&familyFilterJSON)
	if err != nil {
		log.Printf("⚠️  ReloadFamilyFilter: could not read interface %s: %v", interfaceID, err)
		return
	}

	pe.familyFilterMu.Lock()
	defer pe.familyFilterMu.Unlock()

	if familyFilterJSON.Valid && familyFilterJSON.String != "" && familyFilterJSON.String != "null" {
		var families []string
		if jsonErr := json.Unmarshal([]byte(familyFilterJSON.String), &families); jsonErr == nil && len(families) > 0 {
			pe.familyFilter[interfaceID] = families
			log.Printf("🔄 ReloadFamilyFilter: interface %s now accepts %v", interfaceID, families)
			return
		}
	}
	delete(pe.familyFilter, interfaceID)
	log.Printf("🔄 ReloadFamilyFilter: interface %s filter cleared (accept all)", interfaceID)
}

