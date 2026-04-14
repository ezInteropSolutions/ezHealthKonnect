// processing/batch_splitter.go
//
// Format-agnostic batch splitter for the processing engine.
//
// When a single connector payload contains multiple independent messages it
// should be split into individual messages so each one is stored, ACK'd, and
// transformed independently.  The three formats handled here cover the bulk of
// real-world healthcare batch scenarios:
//
//  1. HL7 v2  — multiple MSH segments concatenated (or BHS/BTS-wrapped)
//  2. JSON    — a top-level JSON array whose elements are independent messages
//  3. EDI X12 — multiple ST/SE transaction sets within a single ISA/GS envelope
//
// All other formats (FHIR Bundle, CDA, XML, CSV) are returned as-is; their
// batch semantics are handled by downstream executors, not the connector layer.

package processing

import (
	"encoding/json"
	"strings"

	"ezhealthkonnect/hl7"
)

// BatchFormat identifies how a batch payload was detected.
type BatchFormat string

const (
	BatchFormatHL7v2 BatchFormat = "hl7v2"
	BatchFormatJSON  BatchFormat = "json_array"
	BatchFormatEDI   BatchFormat = "edi_x12"
	BatchFormatNone  BatchFormat = "none"
)

// SplitBatchResult holds the split result and the format that was detected.
type SplitBatchResult struct {
	Parts  []string
	Format BatchFormat
}

// splitBatchPayload inspects content, detects the batch format, and returns
// individual parts.  If the content is not a recognised batch the result
// contains the original string as the only element and Format=BatchFormatNone.
func splitBatchPayload(content string) SplitBatchResult {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return SplitBatchResult{Parts: []string{content}, Format: BatchFormatNone}
	}

	// ── HL7 v2 ──────────────────────────────────────────────────────────────
	// Signature: starts with MSH| or contains \rMSH| / \nMSH|
	// Multiple messages = more than one MSH segment.
	if isHL7v2Content(trimmed) {
		if hl7.CountMessages(trimmed) > 1 {
			parts := hl7.SplitMessages(trimmed)
			if len(parts) > 1 {
				return SplitBatchResult{Parts: parts, Format: BatchFormatHL7v2}
			}
		}
		return SplitBatchResult{Parts: []string{content}, Format: BatchFormatNone}
	}

	// ── JSON array ───────────────────────────────────────────────────────────
	// Signature: top-level "[" character.
	// Only split if it is an array of JSON objects (not a FHIR array, etc.).
	if strings.HasPrefix(trimmed, "[") {
		if parts := splitJSONArray(trimmed); len(parts) > 1 {
			return SplitBatchResult{Parts: parts, Format: BatchFormatJSON}
		}
		return SplitBatchResult{Parts: []string{content}, Format: BatchFormatNone}
	}

	// ── EDI X12 ──────────────────────────────────────────────────────────────
	// Signature: starts with "ISA" interchange envelope header.
	// Multiple messages = multiple ST/SE transaction sets.
	if strings.HasPrefix(trimmed, "ISA") {
		if parts := splitEDITransactions(trimmed); len(parts) > 1 {
			return SplitBatchResult{Parts: parts, Format: BatchFormatEDI}
		}
		return SplitBatchResult{Parts: []string{content}, Format: BatchFormatNone}
	}

	return SplitBatchResult{Parts: []string{content}, Format: BatchFormatNone}
}

// ─── HL7 v2 ──────────────────────────────────────────────────────────────────

func isHL7v2Content(content string) bool {
	return strings.HasPrefix(content, "MSH|") ||
		strings.Contains(content, "\rMSH|") ||
		strings.Contains(content, "\nMSH|") ||
		// BHS-wrapped batches open with the batch header segment
		strings.HasPrefix(content, "BHS|") ||
		strings.HasPrefix(content, "FHS|")
}

// ─── JSON array ──────────────────────────────────────────────────────────────

// splitJSONArray parses a JSON array and returns each element marshalled back
// to a JSON string.  Non-object elements (primitives, nested arrays) are
// skipped — if no valid object elements are found the function returns nil so
// the caller treats the payload as a single message.
func splitJSONArray(content string) []string {
	var elements []json.RawMessage
	if err := json.Unmarshal([]byte(content), &elements); err != nil {
		return nil
	}
	if len(elements) <= 1 {
		return nil
	}

	var parts []string
	for _, elem := range elements {
		trimmed := strings.TrimSpace(string(elem))
		// Only split if elements are JSON objects — arrays of primitives or
		// scalars are not independent messages.
		if !strings.HasPrefix(trimmed, "{") {
			return nil
		}
		parts = append(parts, trimmed)
	}
	return parts
}

// ─── EDI X12 ─────────────────────────────────────────────────────────────────

// splitEDITransactions splits an EDI X12 payload into individual transaction
// sets.  Each transaction set runs from ST through SE (inclusive) and is
// returned as a standalone string.
//
// EDI segment delimiter is typically "\n" or "\r\n" in modern files, but the
// spec allows any single character after the ISA header.  We auto-detect it.
func splitEDITransactions(content string) []string {
	// Detect segment terminator from ISA header (position 105 in a fixed-width ISA)
	segTerm := detectEDISegmentTerminator(content)

	segments := strings.Split(content, segTerm)

	var transactions []string
	var current []string
	inTransaction := false

	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}

		if strings.HasPrefix(seg, "ST") {
			inTransaction = true
			current = []string{seg}
		} else if strings.HasPrefix(seg, "SE") {
			if inTransaction {
				current = append(current, seg)
				transactions = append(transactions, strings.Join(current, segTerm))
				current = nil
				inTransaction = false
			}
		} else if inTransaction {
			current = append(current, seg)
		}
	}

	return transactions
}

// detectEDISegmentTerminator returns the segment terminator character used in
// an EDI X12 payload.  The ISA segment is exactly 106 characters; the last
// character (index 105) is the segment terminator chosen by the sender.
// Falls back to "\n" if the content is too short or malformed.
func detectEDISegmentTerminator(content string) string {
	const isaLength = 106
	if len(content) >= isaLength {
		ch := string(content[isaLength-1])
		// Normalise common variants
		if ch == "\r" || ch == "\n" || ch == "~" || ch == "|" {
			return ch
		}
	}
	// Fallback: try common terminators in order
	for _, t := range []string{"~\n", "~\r\n", "~\r", "~", "\r\n", "\n"} {
		if strings.Contains(content, t) {
			return t
		}
	}
	return "\n"
}
