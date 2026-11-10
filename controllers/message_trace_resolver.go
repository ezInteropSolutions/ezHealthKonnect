package controllers

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// Pure helpers for GET /api/system/message-trace/:id?interfaceId=…
//
// The trace used to look a correlation id up across EVERY interface's message table and take the first hit. A
// correlation id is the sender's reusable control id (HL7 MSH-10), and non-HL7 messages have none, so the old trace
// returned an arbitrary message from some other interface, or "not found". The trace is now scoped to one interface:
// the id is matched against that interface's own message ids first, then its correlation ids (newest first), and the
// caller is told when the id is ambiguous.

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// isUUID reports whether s is a canonical UUID (the only thing accepted as an interface id).
func isUUID(s string) bool { return uuidRE.MatchString(s) }

// quoteIdentifier quotes a SQL identifier (doubling embedded quotes) so a table name can never end its own quoting.
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// traceMatch is one message of the interface that matched the id the user pasted.
type traceMatch struct {
	ID            string    `json:"id"`
	MessageID     string    `json:"message_id"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	Status        string    `json:"status"`
	ReceivedAt    time.Time `json:"received_at"`
	MatchedBy     string    `json:"matched_by"` // "message_id" (exact) or "correlation_id"
}

// rankTraceMatches labels how each row matched and orders them: exact message-id (or row-uuid) matches first, then
// correlation-id matches, newest first within each group. The first element is the message that gets traced.
func rankTraceMatches(rows []traceMatch, id string) []traceMatch {
	out := make([]traceMatch, len(rows))
	copy(out, rows)
	for i := range out {
		if out[i].MessageID == id || out[i].ID == id {
			out[i].MatchedBy = "message_id"
		} else {
			out[i].MatchedBy = "correlation_id"
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ei, ej := out[i].MatchedBy == "message_id", out[j].MatchedBy == "message_id"
		if ei != ej {
			return ei
		}
		return out[i].ReceivedAt.After(out[j].ReceivedAt)
	})
	return out
}

// traceAmbiguous is true when the id resolved only by correlation id and more than one message shares it.
func traceAmbiguous(ranked []traceMatch) bool {
	return len(ranked) > 1 && ranked[0].MatchedBy == "correlation_id"
}
