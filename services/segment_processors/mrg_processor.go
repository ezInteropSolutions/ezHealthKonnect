// services/segment_processors/mrg_processor.go
//
// MRGProcessor handles the MRG (Merge Patient Information) segment for ADT
// merge events (A39, A40, A41, A42, A43).
//
// Per the HL7 V2-to-FHIR IG (ADT^A40 ConceptMap):
//   - The surviving Patient already exists in the assembled resources.
//   - MRG.1 (Prior Patient Identifier List) identifies the obsolete record.
//   - A second Patient resource is created: active=false, identifier=MRG.1.
//   - Both Patients carry Patient.link entries wiring them together:
//       surviving  → link.type "replaces"     → prior Patient
//       prior      → link.type "replaced-by"  → surviving Patient
//
// References use the "Patient/<id>" relative format.  The bundle assembler's
// rewriteReferences() converts them to urn:uuid: fullUrls post-assembly, so
// links are always consistent regardless of UUID assignment order.
package segment_processors

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// mrgProcessor implements SegmentProcessor for the MRG segment.
type mrgProcessor struct{}

func (mrgProcessor) Name() string { return "MRGProcessor" }

func (mrgProcessor) Process(ctx *SegmentProcessorContext) error {
	priorIDs := extractMRGIdentifiers(ctx.ParsedHL7Data)
	if len(priorIDs) == 0 {
		return nil // no MRG segment or no identifiers — nothing to do
	}

	surviving := ctx.Coordinator.FindFirst("Patient")
	if surviving == nil {
		return fmt.Errorf("MRGProcessor: no Patient resource found to link")
	}

	survivingID, _ := surviving["id"].(string)
	priorID := "patient-prior-" + uuid.New().String()[:18]
	survivingRef := "Patient/" + survivingID
	priorRef := "Patient/" + priorID

	// Build prior (obsolete) Patient resource.
	prior := map[string]interface{}{
		"resourceType": "Patient",
		"id":           priorID,
		"active":       false,
		"identifier":   priorIDs,
		"link": []interface{}{
			map[string]interface{}{
				"other": map[string]interface{}{"reference": survivingRef},
				"type":  "replaced-by",
			},
		},
		"text": map[string]interface{}{
			"status": "generated",
			"div": `<div xmlns="http://www.w3.org/1999/xhtml"><table class="grid" ` +
				`style="border-collapse:collapse;width:100%;"><thead><tr ` +
				`style="background:#f0f0f0;"><th colspan="2" style="padding:8px;` +
				`text-align:left;">Patient (obsolete — merged)</th></tr></thead>` +
				`<tbody><tr><td style="padding:4px 8px;border:1px solid #ddd;` +
				`font-weight:bold;">Status</td><td style="padding:4px 8px;` +
				`border:1px solid #ddd;">Inactive (merged into surviving record)</td></tr>` +
				`</tbody></table></div>`,
		},
	}

	// Add Patient.link on the surviving record.
	ctx.Coordinator.Update(survivingRef, func(r map[string]interface{}) {
		link := map[string]interface{}{
			"other": map[string]interface{}{"reference": priorRef},
			"type":  "replaces",
		}
		switch existing := r["link"].(type) {
		case []interface{}:
			r["link"] = append(existing, link)
		default:
			r["link"] = []interface{}{link}
		}
	})

	ctx.Coordinator.Add(prior)
	return nil
}

// extractMRGIdentifiers reads MRG.1 (Prior Patient Identifier List) from the
// enhanced-schema parsed HL7 data and returns FHIR Identifier objects.
func extractMRGIdentifiers(parsedHL7 map[string]interface{}) []interface{} {
	mrgSeg, ok := getSegment(parsedHL7, "MRG")
	if !ok {
		return nil
	}

	// MRG.1 is the Prior Patient Identifier List (CX composite).
	// Try MRG.1.1 (first component = ID value) first; fall back to the
	// field-level value and strip any ^-delimited composite components.
	value := getFieldValue(mrgSeg, "MRG.1.1")
	if value == "" {
		value = getFieldValue(mrgSeg, "MRG.1")
		if idx := strings.IndexByte(value, '^'); idx != -1 {
			value = value[:idx]
		}
	}
	if value == "" {
		return nil
	}

	return []interface{}{
		map[string]interface{}{
			"use":   "usual",
			"value": value,
		},
	}
}

// getSegment extracts a named segment map from the enhanced-schema parsed HL7.
func getSegment(parsedHL7 map[string]interface{}, segName string) (map[string]interface{}, bool) {
	if es, ok := parsedHL7["enhancedSegments"].(map[string]interface{}); ok {
		if seg, ok := es[segName].(map[string]interface{}); ok {
			return seg, true
		}
	}
	if seg, ok := parsedHL7[segName].(map[string]interface{}); ok {
		return seg, true
	}
	return nil, false
}

// getFieldValue extracts the string value from a segment field by key (e.g. "MRG.1").
func getFieldValue(seg map[string]interface{}, fieldKey string) string {
	if fields, ok := seg["fields"].([]interface{}); ok {
		for _, f := range fields {
			if fm, ok := f.(map[string]interface{}); ok {
				if k, _ := fm["key"].(string); k == fieldKey {
					if v, _ := fm["value"].(string); v != "" {
						return v
					}
				}
			}
		}
	}
	if v, ok := seg[fieldKey].(string); ok {
		return v
	}
	return ""
}

func init() {
	Register(mrgProcessor{})
}
