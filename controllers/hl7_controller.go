package controllers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"ezhealthkonnect/config"
	"ezhealthkonnect/hl7"
	hl7validator "ezhealthkonnect/hl7/validator"
	"ezhealthkonnect/services/mapping"

	"github.com/gin-gonic/gin"
)

// HL7Controller handles HL7-related operations
type HL7Controller struct {
	config *config.Config
}

// NewHL7Controller creates a new HL7 controller
func NewHL7Controller(cfg *config.Config) *HL7Controller {
	return &HL7Controller{
		config: cfg,
	}
}

// applyConformanceValidation runs the hl7/validator conformance checks
// (missing required segments, cardinality, data types, table/vocabulary
// bindings) on top of the required-field validation ParseWithRealSchema
// already populates on result.ValidationErrors, appending its findings in
// place. levelOverride is the request's own ValidationLevel when present,
// falling back to ctrl.config.HL7ValidationLevel. A no-op when result is
// nil/unsuccessful, has no schema loaded, or the schema can't be re-resolved
// (e.g. the ParseHL7Enhanced fallback path, which never has a RealHL7Schema
// to validate against).
func (ctrl *HL7Controller) applyConformanceValidation(result *hl7.EnhancedParsedMessage, rawMessage, levelOverride string) {
	if result == nil || !result.Success || !result.SchemaLoaded {
		return
	}
	level := strings.TrimSpace(levelOverride)
	if level == "" {
		level = ctrl.config.HL7ValidationLevel
	}
	schema, err := hl7.ResolveSchemaForMessage(rawMessage)
	if err != nil || schema == nil {
		return
	}
	cr := hl7validator.ValidateMessage(schema, result, hl7validator.ValidationOptions{
		Level: hl7validator.ParseLevel(level),
	})
	result.ValidationErrors = append(result.ValidationErrors, cr.All()...)
}

// effectiveValidationLevel returns the request-level override when set,
// otherwise the server default — the same precedence applyConformanceValidation
// uses, surfaced here purely for accurate response metadata.
func effectiveValidationLevel(configDefault, requestOverride string) string {
	if v := strings.TrimSpace(requestOverride); v != "" {
		return v
	}
	return configDefault
}

// ParseMessage handles HL7 message parsing requests
func (ctrl *HL7Controller) ParseMessage(c *gin.Context) {
    var req hl7.ParseRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, hl7.ParseResponse{
            Success: false,
            Error:   fmt.Sprintf("Invalid request: %v", err),
        })
        return
    }

    // Validate request
    if strings.TrimSpace(req.RawMessage) == "" {
        c.JSON(http.StatusBadRequest, hl7.ParseResponse{
            Success: false,
            Error:   "Empty HL7 message provided",
        })
        return
    }

    // ── Batch detection ──────────────────────────────────────────────────────
    // When the input contains more than one MSH segment, split into individual
    // messages, parse each, and return a batch response.
    if hl7.CountMessages(req.RawMessage) > 1 {
        ctrl.parseBatch(c, req)
        return
    }

    // Track parsing start time
    startTime := time.Now()

    // Only log if verbose logging is enabled
    if ctrl.config.VerboseLogging {
        fmt.Printf("📄 Parsing HL7 message (enhanced: %v, length: %d chars)\n",
            req.UseEnhanced, len(req.RawMessage))
    }

    // Parse with real schema when available (sets SchemaLoaded=true → "enhanced_schema"),
    // otherwise fall back to the basic enhanced parser.
    var result *hl7.EnhancedParsedMessage
    if realLoader := hl7.GetRealSchemaLoader(); realLoader != nil {
        result = hl7.ParseWithRealSchema(req.RawMessage)
        if result == nil || !result.Success {
            // Schema parse failed — fall back to basic enhanced parser
            result = hl7.ParseHL7EnhancedWithOptions(req.RawMessage, hl7.ParseOptions{
                EscapeHandling: req.EscapeHandling,
            })
        } else if req.EscapeHandling == "decode" {
            // Apply escape decoding as post-processing step
            encodingChars := hl7.ExtractEncodingChars(req.RawMessage)
            hl7.ApplyEscapeDecodingToResult(result, encodingChars)
        }
    } else {
        result = hl7.ParseHL7EnhancedWithOptions(req.RawMessage, hl7.ParseOptions{
            EscapeHandling: req.EscapeHandling,
        })
    }

    // Debug logging for parsing flags
    if ctrl.config.VerboseLogging {
        fmt.Printf("🔍 DEBUG: Parsing result flags:\n")
        fmt.Printf("  - result.Success: %v\n", result.Success)
        fmt.Printf("  - result.SchemaLoaded: %v\n", result.SchemaLoaded)
        fmt.Printf("  - result.DictionaryUsed: %v\n", result.DictionaryUsed)
        fmt.Printf("  - Enhanced segments count: %d\n", len(result.EnhancedSegments))
        
        // Check what type of parsing was actually used
        for segName, segment := range result.EnhancedSegments {
            fmt.Printf("  - Segment %s source: %s\n", segName, segment.DictionarySource)
            break // Just check the first one
        }
        
        // Check if real schema loader is available
        if realLoader := hl7.GetRealSchemaLoader(); realLoader != nil {
            fmt.Printf("  - Real schema loader: AVAILABLE\n")
            stats := realLoader.GetStats()
            fmt.Printf("  - Schema loader stats: loads=%d, hits=%d, misses=%d, errors=%d\n", 
                stats.TotalLoads, stats.CacheHits, stats.CacheMisses, stats.LoadErrors)
        } else {
            fmt.Printf("  - Real schema loader: NOT AVAILABLE\n")
        }
    }

    // Calculate parsing time
    parsingTime := time.Since(startTime)

    if result == nil {
        c.JSON(http.StatusInternalServerError, hl7.ParseResponse{
            Success: false,
            Error:   "Parser returned null result",
        })
        return
    }

    if !result.Success {
        c.JSON(http.StatusBadRequest, hl7.ParseResponse{
            Success: false,
            Error:   result.Error,
        })
        return
    }

    // Only log success details if verbose logging is enabled
    if ctrl.config.VerboseLogging {
        fmt.Printf("✅ Successfully parsed HL7 message: %s (segments: %d)\n",
            result.MessageType.Name, len(result.EnhancedSegments))

        fmt.Printf("🔍 DEBUG: Enhanced segments in final result:\n")
        for segName, segment := range result.EnhancedSegments {
            fmt.Printf("  📋 Segment %s: %s (%d fields)\n", segName, segment.Name, len(segment.Fields))
        }
        fmt.Printf("🔍 DEBUG: Segment order: %v\n", result.SegmentOrder)
    }

    // Run HL7 v2 conformance validation (segments/cardinality/data
    // types/table bindings) on top of the required-field check already
    // baked into result.ValidationErrors above.
    ctrl.applyConformanceValidation(result, req.RawMessage, req.ValidationLevel)

    // Determine parse method based on actual usage
    parseMethod := "basic"
    if result.SchemaLoaded {
        parseMethod = "enhanced_schema"
    } else if result.DictionaryUsed {
        parseMethod = "enhanced_dictionary"
    }

    // Get cache stats if available
    var cacheStats *hl7.CacheStats
    if schemaLoader := hl7.GetSchemaLoader(); schemaLoader != nil {
        stats := schemaLoader.GetCacheStats()
        cacheStats = &stats
    }

    c.JSON(http.StatusOK, hl7.ParseResponse{
        Success: true,
        Data:    result,
        Meta: &hl7.ParseMeta{
            ParsingTime:     parsingTime,
            DictionaryUsed:  result.DictionaryUsed,
            ValidationLevel: effectiveValidationLevel(ctrl.config.HL7ValidationLevel, req.ValidationLevel),
            ParserVersion:   "1.0.0",
            SchemaUsed:      result.SchemaLoaded,
            ParseMethod:     parseMethod,
            CacheStats:      cacheStats,
        },
    })
}

// ValidateMessage handles HL7 message validation requests
func (ctrl *HL7Controller) ValidateMessage(c *gin.Context) {
	var req hl7.ParseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	// Parse with real schema when available, so conformance validation has a
	// RealHL7Schema to check against — matching ParseMessage's own
	// real-schema-first, basic-enhanced-fallback behavior for parity.
	var result *hl7.EnhancedParsedMessage
	if realLoader := hl7.GetRealSchemaLoader(); realLoader != nil {
		result = hl7.ParseWithRealSchema(req.RawMessage)
		if result == nil || !result.Success {
			result = hl7.ParseHL7Enhanced(req.RawMessage)
		}
	} else {
		result = hl7.ParseHL7Enhanced(req.RawMessage)
	}

	ctrl.applyConformanceValidation(result, req.RawMessage, req.ValidationLevel)

	// Focus on validation results
	validationResult := gin.H{
		"success":     result.Success,
		"messageType": result.MessageType,
		"valid":       len(result.ValidationErrors) == 0,
		"errors":      result.ValidationErrors,
		"schemaUsed":  result.SchemaLoaded,
	}

	if !result.Success {
		validationResult["parseError"] = result.Error
	}

	c.JSON(http.StatusOK, validationResult)
}

// GetStats returns HL7 processing statistics
func (ctrl *HL7Controller) GetStats(c *gin.Context) {
	// Get real statistics from schema loader
	schemaStats := ctrl.getSchemaLoaderStats()

	stats := gin.H{
		"success": true,
		"stats": gin.H{
			"totalMessagesParsed":   12547,
			"messagesLastHour":      234,
			"averageParsingTime":    "12ms",
			"errorRate":             "0.3%",
			"dictionaryHitRate":     "87%",
			"supportedMessageTypes": []string{"ADT^A01", "ADT^A03", "ADT^A04", "ORU^R01", "ORM^O01"},
			"performance": gin.H{
				"currentThroughput": "45,000 msg/hr",
				"targetThroughput":  "100,000 msg/hr",
				"peakThroughput":    "67,000 msg/hr",
			},
			"schemaLoader": schemaStats,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, stats)
}

// Helper method - uses existing ConvertBasicToEnhanced and converts to map format
func (ctrl *HL7Controller) convertBasicToEnhanced(basicResult *hl7.BasicParsedMessage) *hl7.EnhancedParsedMessage {
	if basicResult == nil {
		// Only log errors, not debug info
		if ctrl.config.VerboseLogging {
			fmt.Printf("❌ ERROR: convertBasicToEnhanced called with nil basicResult\n")
		}
		return &hl7.EnhancedParsedMessage{
			Success: false,
			Error:   "Failed to parse HL7 message",
		}
	}

	// Only log debug info if verbose logging is enabled
	if ctrl.config.VerboseLogging {
		fmt.Printf("🔍 DEBUG: convertBasicToEnhanced called with %d basic segments\n", len(basicResult.Segments))
	}

	// Use the existing ConvertBasicToEnhanced function and convert to map format
	enhancedArray := hl7.ConvertBasicToEnhancedWithDelimiters(basicResult.Segments)
	enhancedSegments := make(map[string]hl7.EnhancedSegment)
	segmentOrder := make([]string, 0, len(enhancedArray))

	for _, enhancedSeg := range enhancedArray {
		enhancedSegments[enhancedSeg.Key] = enhancedSeg
		segmentOrder = append(segmentOrder, enhancedSeg.Key)
	}

	// Only log debug info if verbose logging is enabled
	if ctrl.config.VerboseLogging {
		fmt.Printf("🔍 DEBUG: After conversion - Enhanced segments: %d, Order: %v\n", len(enhancedSegments), segmentOrder)
	}

	result := &hl7.EnhancedParsedMessage{
		Raw:     basicResult.Raw,
		Success: true,
		Version: "2.5",
		MessageType: hl7.MessageTypeInfo{
			Code:        ctrl.extractMessageTypeCode(basicResult.MessageType),
			Event:       ctrl.extractMessageTypeEvent(basicResult.MessageType),
			Name:        basicResult.MessageType,
			Description: ctrl.getMessageDescription(basicResult.MessageType),
		},
		BasicSegments:    basicResult.Segments,
		EnhancedSegments: enhancedSegments,
		SegmentOrder:     segmentOrder,
		ParsedAt:         basicResult.ParsedAt,
		DictionaryUsed:   false,
		SchemaLoaded:     false,
		ValidationErrors: []hl7.ValidationError{},
	}

	// Only log debug info if verbose logging is enabled
	if ctrl.config.VerboseLogging {
		fmt.Printf("✅ DEBUG: Final result has %d enhanced segments\n", len(result.EnhancedSegments))
	}
	return result
}

// Get schema loader statistics
func (ctrl *HL7Controller) getSchemaLoaderStats() gin.H {
	if schemaLoader := hl7.GetSchemaLoader(); schemaLoader != nil {
		stats := schemaLoader.GetCacheStats()
		return gin.H{
			"available":   true,
			"cacheHits":   stats.CacheHits,
			"cacheMisses": stats.CacheMisses,
			"loadErrors":  stats.LoadErrors,
			"totalLoads":  stats.TotalLoads,
			"cacheSize":   stats.CacheSize,
			"lastLoaded":  stats.LastLoaded,
		}
	}

	return gin.H{
		"available": false,
		"message":   "Schema loader not initialized",
	}
}

// List available schemas endpoint
func (ctrl *HL7Controller) ListSchemas(c *gin.Context) {
	if schemaLoader := hl7.GetSchemaLoader(); schemaLoader != nil {
		schemas, err := schemaLoader.ListAvailableSchemas()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   fmt.Sprintf("Failed to list schemas: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"schemas": schemas,
			"count":   len(schemas),
		})
		return
	}

	c.JSON(http.StatusServiceUnavailable, gin.H{
		"success": false,
		"error":   "Schema loader not available",
	})
}

// Get schema loader status endpoint
func (ctrl *HL7Controller) GetSchemaStatus(c *gin.Context) {
	status := hl7.GetSchemaLoaderStatus()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"status":  status,
	})
}

// Helper methods
func (ctrl *HL7Controller) extractMessageTypeCode(messageType string) string {
	if messageType == "UNKNOWN" || messageType == "" {
		return "UNKNOWN"
	}
	parts := strings.Split(messageType, "^")
	if len(parts) > 0 {
		return parts[0]
	}
	return "UNKNOWN"
}

func (ctrl *HL7Controller) extractMessageTypeEvent(messageType string) string {
	if messageType == "UNKNOWN" || messageType == "" {
		return "UNKNOWN"
	}
	parts := strings.Split(messageType, "^")
	if len(parts) > 1 {
		return parts[1]
	}
	return "UNKNOWN"
}

func (ctrl *HL7Controller) getMessageDescription(messageType string) string {
	descriptions := map[string]string{
		"ADT^A01": "Admit/visit notification",
		"ADT^A03": "Discharge/end visit",
		"ADT^A04": "Register a patient",
		"ORU^R01": "Observation result",
		"ORM^O01": "Order message",
	}

	if desc, exists := descriptions[messageType]; exists {
		return desc
	}
	return "Unknown message type"
}

// GetTypeRegistry returns the complete HL7 data type → FHIR type + transform
// registry. Used by the FhirMappingAssistant to detect composite types and
// auto-suggest transforms without keeping a static copy in the frontend.
//
// GET /api/hl7/type-registry
func (ctrl *HL7Controller) GetTypeRegistry(c *gin.Context) {
	types := mapping.AllTypes()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types,
		"count":   len(types),
	})
}

// GetSegmentResources returns the authoritative segment → primary FHIR resource
// table. Used by the FhirMappingAssistant to resolve the default FHIR resource
// for any HL7 segment (including Z-segments such as ZPD → Patient).
//
// GET /api/hl7/segment-resources
func (ctrl *HL7Controller) GetSegmentResources(c *gin.Context) {
	resources := mapping.GetSegmentToResource()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resources,
		"count":   len(resources),
	})
}

// parseBatch parses every individual message in a multi-MSH input and returns
// a single ParseResponse with IsBatch=true and all results in BatchMessages.
// Data is set to the first successfully parsed message for backward compatibility.
func (ctrl *HL7Controller) parseBatch(c *gin.Context, req hl7.ParseRequest) {
	rawMessages := hl7.SplitMessages(req.RawMessage)

	var entries []hl7.BatchParseEntry
	var firstResult *hl7.EnhancedParsedMessage
	var firstMeta *hl7.ParseMeta

	for i, raw := range rawMessages {
		start := time.Now()

		var result *hl7.EnhancedParsedMessage
		if realLoader := hl7.GetRealSchemaLoader(); realLoader != nil {
			result = hl7.ParseWithRealSchema(raw)
			if result == nil || !result.Success {
				result = hl7.ParseHL7EnhancedWithOptions(raw, hl7.ParseOptions{
					EscapeHandling: req.EscapeHandling,
				})
			}
		} else {
			result = hl7.ParseHL7EnhancedWithOptions(raw, hl7.ParseOptions{
				EscapeHandling: req.EscapeHandling,
			})
		}

		ctrl.applyConformanceValidation(result, raw, req.ValidationLevel)

		elapsed := time.Since(start)

		parseMethod := "basic"
		if result != nil && result.SchemaLoaded {
			parseMethod = "enhanced_schema"
		} else if result != nil && result.DictionaryUsed {
			parseMethod = "enhanced_dictionary"
		}

		meta := &hl7.ParseMeta{
			ParsingTime:   elapsed,
			ParserVersion: "1.0.0",
			ParseMethod:   parseMethod,
		}
		if result != nil {
			meta.DictionaryUsed = result.DictionaryUsed
			meta.SchemaUsed = result.SchemaLoaded
		}

		entry := hl7.BatchParseEntry{
			Index:      i,
			RawMessage: raw,
			Meta:       meta,
		}

		if result == nil || !result.Success {
			entry.Success = false
			if result != nil {
				entry.Error = result.Error
			} else {
				entry.Error = "parser returned nil"
			}
		} else {
			entry.Success = true
			entry.Data = result
			if firstResult == nil {
				firstResult = result
				firstMeta = meta
			}
		}

		entries = append(entries, entry)
	}

	resp := hl7.ParseResponse{
		Success:       firstResult != nil,
		IsBatch:       true,
		MessageCount:  len(entries),
		BatchMessages: entries,
		Data:          firstResult,
		Meta:          firstMeta,
	}
	if firstResult == nil {
		resp.Error = "all messages in batch failed to parse"
	}

	c.JSON(http.StatusOK, resp)
}