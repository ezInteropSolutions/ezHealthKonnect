// services/parsers/json_parser.go
//
// JSONParser gives JSON content a structured envelope, so pipeline steps can
// address its fields by path ("type", "data.retired") on the live ingestion
// path, as they already could under Test Pipeline. Before it, JSON fell
// through to RawPassthroughParser and every live message was just
// {"raw": "<text>"}: no step could read a JSON field without a script, while
// Test Pipeline (which parses JSON itself) showed the fields resolving — a
// difference found end to end with ezIdentitySense's webhook events.
//
// The envelope is additive: "raw" keeps the original text, unchanged, so a
// pipeline written against the passthrough envelope keeps working.
//
//	object  -> its keys at the top level, plus "raw"
//	array   -> {"items": [...], "raw"}
//	scalar  -> {"value": v, "raw"}
//	invalid -> {"raw"} only (never a failure: the passthrough behaviour)
//
// A top-level key named "raw" in the content is not overwritten: the original
// text is then only under "_raw_content". Keys the engine reserves (a leading
// "_", or the HL7 parser's structure names) aren't copied; they stay in raw.
package parsers

import (
	"encoding/json"
	"strings"
	"time"

	"ezhealthkonnect/models"
)

// JSONParser parses JSON content into a field-addressable envelope.
type JSONParser struct{}

// NewJSONParser constructs the parser.
func NewJSONParser() *JSONParser { return &JSONParser{} }

// Parse implements the parser contract; it always succeeds.
func (p *JSONParser) Parse(rawContent string) *models.ParserResult {
	envelope := map[string]interface{}{}
	var decoded interface{}
	dec := json.NewDecoder(strings.NewReader(rawContent))
	if err := dec.Decode(&decoded); err == nil && !dec.More() {
		// Numbers decode as float64, as everywhere else in the pipeline.
		switch v := decoded.(type) {
		case map[string]interface{}:
			for k, val := range v {
				if reservedEnvelopeKey(k) {
					continue // stays in raw; never shadows engine data
				}
				envelope[k] = val
			}
		case []interface{}:
			envelope["items"] = v
		default:
			envelope["value"] = v
		}
	}
	if _, taken := envelope["raw"]; taken {
		envelope["_raw_content"] = rawContent
	} else {
		envelope["raw"] = rawContent
	}
	return &models.ParserResult{
		Success:    true,
		Format:     models.FormatJSON,
		ParsedJSON: envelope,
		Metadata: models.ParserMetadata{
			ParserVersion: "json-1.0",
			ParsedAt:      time.Now(),
		},
		ValidationResult: models.ValidationResult{IsValid: true},
	}
}

// reservedEnvelopeKey reports keys the engine itself uses in a message's
// envelope ("_format", "_semantic_index", ... and the HL7 parser's
// structures), which a JSON message's own keys must not shadow.
func reservedEnvelopeKey(k string) bool {
	return strings.HasPrefix(k, "_") || k == "enhancedSegments" || k == "segmentGroups" || k == "segmentOrder"
}

// GetSupportedFormat implements the parser contract.
func (p *JSONParser) GetSupportedFormat() models.MessageFormat { return models.FormatJSON }

// ValidateStructure reports whether the content is one JSON value.
func (p *JSONParser) ValidateStructure(rawContent string) (*models.ValidationResult, error) {
	var v interface{}
	dec := json.NewDecoder(strings.NewReader(rawContent))
	if err := dec.Decode(&v); err != nil || dec.More() {
		return &models.ValidationResult{IsValid: false}, nil
	}
	return &models.ValidationResult{IsValid: true}, nil
}
