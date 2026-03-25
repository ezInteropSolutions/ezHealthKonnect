// services/executors/format/serializer.go
// Format-Agnostic Output Serializer (Phase 4)
//
// SerializeForOutput converts a resolved pipeline content value into the
// wire-format string that an outbound connector sends.
//
// Supported formats:
//   hl7v2   → reconstructed HL7 v2.x pipe-delimited string
//   fhir    → application/fhir+json  (JSON marshal)
//   csv     → text/csv               (CSV lines)
//   json    → application/json       (pretty-printed JSON)
//   (other) → application/json       (fallback)
//
// Usage in outbound_connector_executor.go:
//
//   serialized, contentType, err := format.SerializeForOutput(content, outputFormat)
//
package format

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// SerializeForOutput converts content (any type) to a wire-format string for the
// given output format. Returns (body, contentType, error).
func SerializeForOutput(content interface{}, format string) (string, string, error) {
	switch strings.ToLower(format) {
	case "hl7v2", "hl7":
		return serializeToHL7(content)
	case "fhir", "fhir-r4", "fhir-r4-bundle":
		return serializeToFHIR(content)
	case "csv":
		return serializeToCSV(content)
	case "json", "":
		return serializeToJSON(content)
	default:
		return serializeToJSON(content)
	}
}

// ──────────────────────────────────────────────────────────────
// HL7 v2.x serializer
// Reconstructs a pipe-delimited HL7 string from the enhanced
// segments map stored by the HL7 parser.
// ──────────────────────────────────────────────────────────────

func serializeToHL7(content interface{}) (string, string, error) {
	// Try to get the underlying map
	var msgMap map[string]interface{}
	switch v := content.(type) {
	case map[string]interface{}:
		msgMap = v
	default:
		// Not a map — fallback to JSON
		return serializeToJSON(content)
	}

	// Walk segmentOrder first, then remaining keys alphabetically
	segOrder, _ := getStringSlice(msgMap, "segmentOrder")
	enhancedRaw := msgMap["enhancedSegments"]

	var enhanced map[string]interface{}
	switch e := enhancedRaw.(type) {
	case map[string]interface{}:
		enhanced = e
	default:
		// No enhanced segments — serialize whole map as JSON
		return serializeToJSON(content)
	}

	// Build ordered segment key list
	seen := make(map[string]bool)
	var orderedKeys []string
	for _, k := range segOrder {
		if _, ok := enhanced[k]; ok {
			orderedKeys = append(orderedKeys, k)
			seen[k] = true
		}
	}
	// Append any segments not in segmentOrder
	for k := range enhanced {
		if !seen[k] {
			orderedKeys = append(orderedKeys, k)
		}
	}

	var buf strings.Builder
	for _, segKey := range orderedKeys {
		segRaw := enhanced[segKey]
		line := serializeHL7Segment(segKey, segRaw)
		if line != "" {
			buf.WriteString(line)
			buf.WriteByte('\r')
		}
	}

	return buf.String(), "application/hl7-v2", nil
}

// serializeHL7Segment renders one HL7 segment as "SEG|f1|f2|..." using the
// fields array inside the enhanced segment map.
func serializeHL7Segment(segKey string, segRaw interface{}) string {
	segMap, ok := segRaw.(map[string]interface{})
	if !ok {
		return ""
	}

	fieldsRaw, ok := segMap["fields"]
	if !ok {
		return segKey
	}

	var fields []interface{}
	switch f := fieldsRaw.(type) {
	case []interface{}:
		fields = f
	default:
		return segKey
	}

	// For MSH the first separator field is implicit — we need position-aware serialization.
	// Simple approach: collect field values by position index.
	maxPos := 0
	type fieldEntry struct {
		pos   int
		value string
	}
	var entries []fieldEntry

	for _, fRaw := range fields {
		fMap, ok := fRaw.(map[string]interface{})
		if !ok {
			continue
		}
		pos := 0
		if p, ok := fMap["position"]; ok {
			switch pv := p.(type) {
			case float64:
				pos = int(pv)
			case int:
				pos = pv
			}
		}
		val := ""
		if v, ok := fMap["value"]; ok {
			val = fmt.Sprintf("%v", v)
		}
		if pos > maxPos {
			maxPos = pos
		}
		entries = append(entries, fieldEntry{pos: pos, value: val})
	}

	// Build value array indexed by position
	values := make([]string, maxPos+1)
	for _, e := range entries {
		if e.pos >= 0 && e.pos < len(values) {
			values[e.pos] = e.value
		}
	}

	// For MSH: position 1 = field sep (|), position 2 = encoding chars (^~\&)
	// These are already encoded in the field values from the parser.
	parts := []string{segKey}
	// Start from position 1 (position 0 would be the segment key itself)
	startPos := 1
	if segKey == "MSH" {
		startPos = 1 // MSH.1 is |, MSH.2 is ^~\&
	}
	for p := startPos; p <= maxPos; p++ {
		parts = append(parts, values[p])
	}

	return strings.Join(parts, "|")
}

// ──────────────────────────────────────────────────────────────
// FHIR serializer
//
// Handles three cases:
//  1. Content is already a serialized FHIR string → pass through as-is
//  2. Content is a map with resourceType "Bundle" → marshal directly
//  3. Content is any other map (single resource) → marshal directly
//     (wrapping into a Bundle is the PayloadBuilder's job, not the serializer's)
// ──────────────────────────────────────────────────────────────

func serializeToFHIR(content interface{}) (string, string, error) {
	const ct = "application/fhir+json"

	// Case 1: already a string — validate it looks like FHIR JSON and pass through
	if s, ok := content.(string); ok {
		s = strings.TrimSpace(s)
		if len(s) > 0 && s[0] == '{' {
			// Quick sanity: unmarshal to verify it's valid JSON
			var probe map[string]interface{}
			if err := json.Unmarshal([]byte(s), &probe); err != nil {
				return "", ct, fmt.Errorf("fhir serialize: string content is not valid JSON: %w", err)
			}
			// Ensure resourceType is present (FHIR requirement)
			if _, hasRT := probe["resourceType"]; !hasRT {
				return "", ct, fmt.Errorf("fhir serialize: JSON string is missing 'resourceType' field")
			}
			return s, ct, nil
		}
		return "", ct, fmt.Errorf("fhir serialize: string content does not appear to be a FHIR JSON object")
	}

	// Case 2 & 3: marshal the map/struct directly
	b, err := json.Marshal(content)
	if err != nil {
		return "", ct, fmt.Errorf("fhir serialize: %w", err)
	}

	// Validate resourceType exists in the serialized output
	var probe map[string]interface{}
	if err := json.Unmarshal(b, &probe); err == nil {
		if rt, ok := probe["resourceType"].(string); ok {
			_ = rt // valid FHIR resource
		} else {
			// Not a FHIR resource — still emit it but warn
			fmt.Printf("⚠️  [FHIR Serializer] serialized map has no resourceType — ensure PayloadBuilder assembled a proper FHIR resource\n")
		}
	}

	return string(b), ct, nil
}

// ──────────────────────────────────────────────────────────────
// CSV serializer
// Accepts []map[string]interface{} (from file parser / normalizer).
// ──────────────────────────────────────────────────────────────

func serializeToCSV(content interface{}) (string, string, error) {
	rows, err := toRowSlice(content)
	if err != nil || len(rows) == 0 {
		// Fall back to JSON if content isn't row-oriented
		return serializeToJSON(content)
	}

	// Collect column headers (from first row, sorted for determinism)
	headerSet := make(map[string]bool)
	for _, row := range rows {
		for k := range row {
			headerSet[k] = true
		}
	}
	headers := make([]string, 0, len(headerSet))
	for h := range headerSet {
		headers = append(headers, h)
	}
	sortStrings(headers)

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write(headers)
	for _, row := range rows {
		record := make([]string, len(headers))
		for i, h := range headers {
			if v, ok := row[h]; ok {
				record[i] = fmt.Sprintf("%v", v)
			}
		}
		_ = w.Write(record)
	}
	w.Flush()
	return buf.String(), "text/csv", nil
}

// toRowSlice coerces content into []map[string]interface{}.
func toRowSlice(content interface{}) ([]map[string]interface{}, error) {
	rv := reflect.ValueOf(content)
	if rv.Kind() == reflect.Slice {
		result := make([]map[string]interface{}, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			elem := rv.Index(i).Interface()
			if m, ok := elem.(map[string]interface{}); ok {
				result = append(result, m)
			}
		}
		return result, nil
	}
	return nil, fmt.Errorf("not a slice")
}

// ──────────────────────────────────────────────────────────────
// JSON serializer (default)
// ──────────────────────────────────────────────────────────────

func serializeToJSON(content interface{}) (string, string, error) {
	b, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return "", "application/json", fmt.Errorf("json serialize: %w", err)
	}
	return string(b), "application/json", nil
}

// ──────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────

func getStringSlice(m map[string]interface{}, key string) ([]string, bool) {
	raw, ok := m[key]
	if !ok {
		return nil, false
	}
	switch v := raw.(type) {
	case []string:
		return v, true
	case []interface{}:
		ss := make([]string, 0, len(v))
		for _, item := range v {
			ss = append(ss, fmt.Sprintf("%v", item))
		}
		return ss, true
	}
	return nil, false
}

// sortStrings sorts a string slice in place (avoid importing sort in every caller).
func sortStrings(ss []string) {
	// insertion sort — headers are typically few
	for i := 1; i < len(ss); i++ {
		key := ss[i]
		j := i - 1
		for j >= 0 && ss[j] > key {
			ss[j+1] = ss[j]
			j--
		}
		ss[j+1] = key
	}
}
