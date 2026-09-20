// services/audit/taxonomy.go
//
// The ATNA/RFC 3881-shaped event taxonomy every Go-emitted audit_logs row is
// built from. One entry per Action string ever passed to AuditLogger.Log —
// see audit_events.json. Adding a new audited action anywhere in the Go
// codebase means adding one entry here, never inventing ad hoc result/
// risk_level vocabulary at the call site (that drift — result being
// success/failure/error in some call sites and "blocked"/"info" in others —
// is exactly what this package exists to fix).
package audit

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"sync"
)

//go:embed audit_events.json
var taxonomyFS embed.FS

// TaxonomyEntry describes one audit event type: its ATNA/RFC 3881 identity
// (EventID, EventID display name, EventActionCode — one of C|R|U|D|E) plus
// the HIPAA-compliance defaults (result, risk_level, compliance_flags) this
// codebase applies when an AuditEvent doesn't explicitly override them.
type TaxonomyEntry struct {
	ATNAEventID        string                 `json:"atnaEventId"`
	ATNAEventIDDisplay string                 `json:"atnaEventIdDisplay"`
	EventActionCode    string                 `json:"eventActionCode"`
	DefaultRiskLevel   string                 `json:"defaultRiskLevel"`
	DefaultResult      string                 `json:"defaultResult"`
	ComplianceFlags    map[string]interface{} `json:"complianceFlags"`
}

var (
	taxonomyOnce sync.Once
	taxonomy     map[string]TaxonomyEntry
)

// loadTaxonomy parses the embedded audit_events.json exactly once. A parse
// failure can only come from a genuine coding mistake (the file is compiled
// into the binary, never edited at runtime), so it panics rather than
// degrading silently — the same "fail fast on a broken built-in resource"
// posture schema loaders elsewhere in this codebase already take.
func loadTaxonomy() map[string]TaxonomyEntry {
	taxonomyOnce.Do(func() {
		data, err := taxonomyFS.ReadFile("audit_events.json")
		if err != nil {
			panic(fmt.Sprintf("audit: failed to read embedded audit_events.json: %v", err))
		}
		var parsed map[string]TaxonomyEntry
		if err := json.Unmarshal(data, &parsed); err != nil {
			panic(fmt.Sprintf("audit: failed to parse embedded audit_events.json: %v", err))
		}
		taxonomy = parsed
	})
	return taxonomy
}

// lookupTaxonomy returns the taxonomy entry for action, or a safe generic
// fallback (result="success", risk_level="low", no ATNA/compliance data)
// when action isn't registered. An unregistered action must never block the
// write — it only loses its ATNA enrichment, logged loudly so the gap gets
// noticed and a taxonomy entry gets added.
func lookupTaxonomy(action string) TaxonomyEntry {
	tax := loadTaxonomy()
	if entry, ok := tax[action]; ok {
		return entry
	}
	log.Printf("⚠️  [audit] action %q has no taxonomy entry in services/audit/audit_events.json — using generic defaults", action)
	return TaxonomyEntry{DefaultRiskLevel: "low", DefaultResult: "success"}
}
