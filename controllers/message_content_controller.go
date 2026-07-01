// controllers/message_content_controller.go
// MessageContentController — serves raw message bytes and processing logs
// from object storage (S3/MinIO/local).
//
// Endpoints:
//   GET /api/messages/:messageId/raw?interfaceId=...         — returns inbound raw content
//   GET /api/messages/:messageId/transformed?interfaceId=... — returns transformed outbound content
//   GET /api/messages/:messageId/logs?interfaceId=...        — returns NDJSON log entries as JSON array
//   GET /api/messages/:messageId/mapping-log?interfaceId=... — returns the CDA→FHIR mapping log
//   GET /api/messages/:messageId/parsed-cda?interfaceId=...  — returns the structured parsed CDA content

package controllers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	cdastorage "ezhealthkonnect/services/cda_storage"
	"ezhealthkonnect/services/storage"

	"github.com/gin-gonic/gin"
)

// MessageContentController serves content and logs from object storage.
type MessageContentController struct {
	db          *sql.DB
	objStorage  *storage.ObjectStorageService
	cdaDocStore cdastorage.CDADocumentStore
}

// NewMessageContentController creates the controller.
// objStorage and cdaDocStore may be nil — affected requests will return 503 in that case.
func NewMessageContentController(db *sql.DB, objStorage *storage.ObjectStorageService, cdaDocStore cdastorage.CDADocumentStore) *MessageContentController {
	return &MessageContentController{db: db, objStorage: objStorage, cdaDocStore: cdaDocStore}
}

// RegisterRoutes attaches the routes to a Gin router group.
func (mc *MessageContentController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/messages/:messageId/raw", mc.GetRawContent)
	rg.GET("/messages/:messageId/transformed", mc.GetTransformedContent)
	rg.GET("/messages/:messageId/logs", mc.GetProcessingLogs)
	rg.GET("/messages/:messageId/mapping-log", mc.GetMappingLog)
	rg.GET("/messages/:messageId/parsed-cda", mc.GetParsedCDA)
	rg.GET("/messages/:messageId/parsed-content", mc.GetParsedContent)
}

// lookupMessageURI fetches a stored object URI from the per-interface message table.
// column must be one of the known URI columns; returns "" on any error or miss.
func (mc *MessageContentController) lookupMessageURI(interfaceID, messageID, column string) string {
	if mc.db == nil || interfaceID == "" {
		return ""
	}
	// Guard against SQL injection — only allow known column names
	allowed := map[string]bool{
		"raw_content_uri":         true,
		"parsed_content_uri":      true,
		"transformed_content_uri": true,
	}
	if !allowed[column] {
		return ""
	}
	tableName := "messages_intf_" + strings.ReplaceAll(interfaceID, "-", "_")
	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE message_id = $1 OR id::text = $1 LIMIT 1`,
		column, tableName,
	)
	var uri sql.NullString
	_ = mc.db.QueryRow(query, messageID).Scan(&uri)
	if uri.Valid && uri.String != "" {
		return uri.String
	}
	return ""
}

// GetRawContent serves the original bytes of a message.
// Query param `interfaceId` is optional — resolved from the DB when omitted.
//
// Lookup order:
//  1. raw_content_uri stored in the interface table (correct across day boundaries)
//  2. Reconstruct key from current date (fallback — only works same-day)
func (mc *MessageContentController) GetRawContent(c *gin.Context) {
	if mc.objStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Object storage not configured",
		})
		return
	}

	messageID := c.Param("messageId")
	interfaceID := c.Query("interfaceId")
	if interfaceID == "" {
		// Try to resolve interfaceId from the database using messageId
		interfaceID = mc.resolveInterfaceID(messageID)
	}
	if interfaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "interfaceId query parameter is required",
		})
		return
	}

	// Prefer the stored URI — avoids date-embedded key mismatch for cross-day retrievals
	if uri := mc.lookupMessageURI(interfaceID, messageID, "raw_content_uri"); uri != "" {
		if data, err := mc.objStorage.GetRawMessageByURI(c.Request.Context(), uri); err == nil {
			c.JSON(http.StatusOK, gin.H{
				"success":   true,
				"messageId": messageID,
				"content":   string(data),
				"size":      len(data),
				"driver":    mc.objStorage.DriverName(),
			})
			return
		} else {
			log.Printf("⚠️  GetRawContent: URI %s failed: %v — falling back to key-based lookup", uri, err)
		}
	}

	// Fallback: reconstruct key from current date (works when same-day)
	data, err := mc.objStorage.GetRawMessage(c.Request.Context(), interfaceID, messageID)
	if err != nil {
		log.Printf("⚠️  GetRawContent: %v", err)
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Raw content not found: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"messageId": messageID,
		"content":   string(data),
		"size":      len(data),
		"driver":    mc.objStorage.DriverName(),
	})
}

// GetParsedContent returns the JSON document the raw message was converted to for
// downstream pipeline processing — the literal ParserResult.ParsedJSON produced by
// engine_message_processor.go's "Layer 1" auto-conversion step (format-detected via
// ParserFactory: HL7, CDA/CCD, FHIR, ...) before the transformation pipeline ever
// runs. This is distinct from /parsed-cda (the CDA-document-store's USCDI-schema
// view used by the Mapping Log) — same parser, different persistence path, written
// for every message regardless of format.
func (mc *MessageContentController) GetParsedContent(c *gin.Context) {
	if mc.objStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "Object storage not configured"})
		return
	}

	messageID := c.Param("messageId")
	interfaceID := c.Query("interfaceId")
	if interfaceID == "" {
		interfaceID = mc.resolveInterfaceID(messageID)
	}
	if interfaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interfaceId query parameter is required"})
		return
	}

	// Prefer the stored URI — avoids date-embedded key mismatch for cross-day retrievals
	if uri := mc.lookupMessageURI(interfaceID, messageID, "parsed_content_uri"); uri != "" {
		if data, err := mc.objStorage.GetParsedContentByURI(c.Request.Context(), uri); err == nil {
			c.JSON(http.StatusOK, gin.H{
				"success":       true,
				"messageId":     messageID,
				"parsedContent": data,
			})
			return
		} else {
			log.Printf("⚠️  GetParsedContent: URI %s failed: %v — falling back to key-based lookup", uri, err)
		}
	}

	// Fallback: reconstruct key from current date (works when same-day)
	data, err := mc.objStorage.GetParsedContent(c.Request.Context(), interfaceID, messageID)
	if err != nil {
		if isNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Parsed content not available for this message (not yet processed, or message is older than today)",
			})
			return
		}
		log.Printf("⚠️  GetParsedContent: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": fmt.Sprintf("Failed to retrieve parsed content: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"messageId":     messageID,
		"parsedContent": data,
	})
}

// GetTransformedContent serves the payload that was delivered to the outbound connector.
//
// Lookup order:
//  1. outbound stage — exact bytes sent (written by OutboundConnectorExecutor)
//  2. transformed stage — extract cleaned payload from the pipeline context envelope
//
// Pass ?source=pipeline to skip the outbound stage and return the full pipeline context.
func (mc *MessageContentController) GetTransformedContent(c *gin.Context) {
	if mc.objStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "Object storage not configured"})
		return
	}

	messageID := c.Param("messageId")
	interfaceID := c.Query("interfaceId")
	if interfaceID == "" {
		interfaceID = mc.resolveInterfaceID(messageID)
	}
	if interfaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interfaceId query parameter is required"})
		return
	}

	// 1. Try the outbound stage first — exact payload delivered to the connector
	if c.Query("source") != "pipeline" {
		outContent, outCT, err := mc.objStorage.GetOutboundPayload(c.Request.Context(), interfaceID, messageID)
		if err == nil && outContent != "" {
			c.JSON(http.StatusOK, gin.H{
				"success":      true,
				"messageId":    messageID,
				"content":      outContent,
				"content_type": outCT,
				"size":         len(outContent),
				"source":       "outbound",
				"driver":       mc.objStorage.DriverName(),
			})
			return
		}
	}

	// 2. Fall back to pipeline context envelope — extract payload fields
	data, err := mc.objStorage.GetTransformedContent(c.Request.Context(), interfaceID, messageID)
	if err != nil {
		if isNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Transformed content not yet available"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": fmt.Sprintf("Failed to retrieve transformed content: %v", err)})
		return
	}

	// Extract the payload.
	// Priority 1: pipeline_result — matches the test pipeline response format exactly
	// (includes input, output.payload, steps, status, success).
	// Priority 2: transformed_content / fhirBundle / fallback keys (legacy path).
	var content string
	if pr, ok := data["pipeline_result"].(map[string]interface{}); ok {
		if b, err := json.MarshalIndent(pr, "", "  "); err == nil {
			content = string(b)
		}
	} else if raw, ok := data["transformed_content"].(string); ok && raw != "" {
		content = raw
	} else if nested, ok := data["transformed_content"].(map[string]interface{}); ok {
		if b, err := json.MarshalIndent(nested, "", "  "); err == nil {
			content = string(b)
		}
	} else if raw, ok := data["fhirBundle"].(string); ok && raw != "" {
		content = raw
	} else if nested, ok := data["fhirBundle"].(map[string]interface{}); ok {
		if b, err := json.MarshalIndent(nested, "", "  "); err == nil {
			content = string(b)
		}
	} else {
		payload := map[string]interface{}{}
		for _, key := range []string{"transformed_content", "fhirBundle", "output", "result"} {
			if v, ok := data[key]; ok {
				payload[key] = v
			}
		}
		if len(payload) == 0 {
			payload = data
		}
		if b, err := json.MarshalIndent(payload, "", "  "); err == nil {
			content = string(b)
		}
	}

	contentType := "application/json"
	if v, ok := data["content_type"].(string); ok && v != "" {
		contentType = v
	}

	var pipelineSteps interface{}
	if meta, ok := data["transformation_metadata"].(map[string]interface{}); ok {
		pipelineSteps = meta["steps"]
	}

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"messageId":      messageID,
		"content":        content,
		"content_type":   contentType,
		"size":           len(content),
		"source":         "pipeline",
		"driver":         mc.objStorage.DriverName(),
		"pipeline_steps": pipelineSteps,
	})
}

// GetProcessingLogs returns the processing log entries for a message.
func (mc *MessageContentController) GetProcessingLogs(c *gin.Context) {
	if mc.objStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Object storage not configured",
		})
		return
	}

	messageID := c.Param("messageId")
	interfaceID := c.Query("interfaceId")
	if interfaceID == "" {
		interfaceID = mc.resolveInterfaceID(messageID)
	}
	if interfaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "interfaceId query parameter is required",
		})
		return
	}

	entries, err := mc.objStorage.GetLogs(c.Request.Context(), interfaceID, messageID)
	if err != nil {
		// If no log file exists yet that's not an error — return empty array
		if isNotFound(err) {
			c.JSON(http.StatusOK, gin.H{
				"success":   true,
				"messageId": messageID,
				"logs":      []storage.LogEntry{},
			})
			return
		}
		log.Printf("⚠️  GetProcessingLogs: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to retrieve logs: %v", err),
		})
		return
	}

	if entries == nil {
		entries = []storage.LogEntry{}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"messageId": messageID,
		"count":     len(entries),
		"logs":      entries,
	})
}

// GetMappingLog returns the CDA→FHIR mapping log for a message — per-section
// timing, resource counts, and dedup/synthesis events recorded by document_mapper.go.
// Only present for messages processed through a cda.to_fhir pipeline step with
// mappingLogEnabled not set to false. Same-day-only lookup (see GetTransformedContent).
func (mc *MessageContentController) GetMappingLog(c *gin.Context) {
	if mc.objStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Object storage not configured",
		})
		return
	}

	messageID := c.Param("messageId")
	interfaceID := c.Query("interfaceId")
	if interfaceID == "" {
		interfaceID = mc.resolveInterfaceID(messageID)
	}
	if interfaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "interfaceId query parameter is required",
		})
		return
	}

	data, err := mc.objStorage.GetMappingLog(c.Request.Context(), interfaceID, messageID)
	if err != nil {
		if isNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Mapping log not available for this message (not a CDA→FHIR conversion, logging disabled, or message is older than today)",
			})
			return
		}
		log.Printf("⚠️  GetMappingLog: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to retrieve mapping log: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"messageId":  messageID,
		"mappingLog": data,
	})
}

// GetParsedCDA returns the structured parsed CDA content (sections/entries/fields)
// for a message — the same ParsedJSON the cda.parse executor produces, persisted
// either by an explicit cda.parse pipeline step or automatically by cda.to_fhir's
// auto-parse path. Looked up by message_id via the CDADocumentStore, not by the
// store's own document id (the frontend only ever knows the pipeline message id).
func (mc *MessageContentController) GetParsedCDA(c *gin.Context) {
	if mc.cdaDocStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "CDA document store not configured",
		})
		return
	}

	messageID := c.Param("messageId")
	interfaceID := c.Query("interfaceId")
	if interfaceID == "" {
		interfaceID = mc.resolveInterfaceID(messageID)
	}

	docs, _, err := mc.cdaDocStore.List(c.Request.Context(), cdastorage.ListFilter{
		InterfaceID: interfaceID,
		MessageID:   messageID,
		Limit:       1,
	})
	if err != nil {
		log.Printf("⚠️  GetParsedCDA: list failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": fmt.Sprintf("Failed to look up parsed CDA document: %v", err)})
		return
	}
	if len(docs) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Parsed CDA content not available for this message",
		})
		return
	}

	parsed, err := mc.cdaDocStore.GetParsedJSON(c.Request.Context(), docs[0].ID)
	if err != nil {
		log.Printf("⚠️  GetParsedCDA: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": fmt.Sprintf("Failed to retrieve parsed CDA content: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"messageId": messageID,
		"parsedCDA": parsed,
	})
}

// resolveInterfaceID looks up the interface for a message by scanning per-interface tables.
// Returns empty string if not found.
func (mc *MessageContentController) resolveInterfaceID(messageID string) string {
	if mc.db == nil {
		return ""
	}

	// Query interface_table_metadata to get all known table names
	rows, err := mc.db.Query(`SELECT interface_id, table_name FROM interface_table_metadata WHERE table_name IS NOT NULL`)
	if err != nil {
		return ""
	}
	defer rows.Close()

	type tableInfo struct {
		interfaceID string
		tableName   string
	}
	var tables []tableInfo
	for rows.Next() {
		var t tableInfo
		if err := rows.Scan(&t.interfaceID, &t.tableName); err == nil {
			tables = append(tables, t)
		}
	}

	// Search each interface table for the message ID
	for _, t := range tables {
		query := fmt.Sprintf(`SELECT interface_id FROM %s WHERE message_id = $1 LIMIT 1`, t.tableName)
		var ifID string
		if err := mc.db.QueryRow(query, messageID).Scan(&ifID); err == nil {
			return ifID
		}
	}
	return ""
}

// resolveByCorrelationID scans every interface message table for a row with the
// given correlation_id (populated on every inbound message — see
// models.InboundMessage.CorrelationID) and returns its (interface_id, message_id).
// Returns ("", "") if the correlation_id isn't found in any table. Mirrors
// resolveInterfaceID's cross-table scan pattern, keyed by correlation_id instead
// of message_id since GetMessageTrace's caller only has the correlation_id.
func (mc *MessageContentController) resolveByCorrelationID(correlationID string) (interfaceID, messageID string) {
	if mc.db == nil {
		return "", ""
	}

	rows, err := mc.db.Query(`SELECT interface_id, table_name FROM interface_table_metadata WHERE table_name IS NOT NULL`)
	if err != nil {
		return "", ""
	}
	defer rows.Close()

	type tableInfo struct {
		interfaceID string
		tableName   string
	}
	var tables []tableInfo
	for rows.Next() {
		var t tableInfo
		if err := rows.Scan(&t.interfaceID, &t.tableName); err == nil {
			tables = append(tables, t)
		}
	}

	for _, t := range tables {
		query := fmt.Sprintf(`SELECT message_id FROM %s WHERE correlation_id = $1 LIMIT 1`, t.tableName)
		var msgID string
		if err := mc.db.QueryRow(query, correlationID).Scan(&msgID); err == nil {
			return t.interfaceID, msgID
		}
	}
	return "", ""
}

// GetMessageTrace resolves a correlation_id to its message across every interface
// and returns the full per-message lifecycle log timeline — the same NDJSON entries
// GetProcessingLogs returns, but reachable from just the correlation_id (e.g. pasted
// from an error email or alert) without knowing which interface the message came
// from or its internal message_id.
// GET /api/system/message-trace/:correlationId
func (mc *MessageContentController) GetMessageTrace(c *gin.Context) {
	correlationID := c.Param("correlationId")
	if correlationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "correlationId path parameter is required",
		})
		return
	}

	interfaceID, messageID := mc.resolveByCorrelationID(correlationID)
	if interfaceID == "" || messageID == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "no message found for this correlation_id",
		})
		return
	}

	var entries []storage.LogEntry
	if mc.objStorage != nil {
		var err error
		entries, err = mc.objStorage.GetLogs(c.Request.Context(), interfaceID, messageID)
		if err != nil && !isNotFound(err) {
			log.Printf("⚠️  GetMessageTrace: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"correlation_id": correlationID,
		"interface_id":   interfaceID,
		"message_id":     messageID,
		"timeline":       entries,
		"count":          len(entries),
	})
}

// isNotFound returns true when the error indicates a missing object.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "nosuchkey") ||
		strings.Contains(msg, "does not exist")
}
