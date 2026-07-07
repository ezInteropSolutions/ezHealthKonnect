// services/executors/transform/cda_document_resolution.go
// Shared read/write contract for the typed *cdadocument.CDADocument as it
// flows between CDA-family pipeline steps (cda.parse produces it; cda.dedupe
// mutates it; cda.to_fhir/cda.section_to_csv consume it).
//
// ExecutePipeline only ever carries a step's changes forward by mutating the
// SAME shared message object every step receives as inputData["message"] —
// a step that instead stashes a result as a sibling key on its own (copied)
// outputData is invisible to the next step, because executeStepWithContext
// rebuilds inputData from scratch per step (only message/_metadata/
// _variableContext/steps survive). field_mapping/data_masking already
// follow this contract by construction (they mutate inputData["message"]
// directly); cda.to_fhir's own fhirBundle write does too. _cdaDocument
// previously did not, which is why cda.dedupe's mutation of the parsed
// document was invisible to a following cda.to_fhir/cda.section_to_csv step
// — found and fixed together, since all three consumers shared the same gap.
//
// No backward-compat shim for a top-level sibling-key shape: this project
// has no production interfaces relying on the old (broken) behavior.
package transform

import cdadocument "ezhealthkonnect/cda/document"

// findCDADocument looks for a typed *CDADocument left by an earlier step,
// inside the shared message object.
func findCDADocument(inputData map[string]interface{}) (*cdadocument.CDADocument, bool) {
	msg, ok := inputData["message"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	typed, ok := msg["_cdaDocument"]
	if !ok {
		return nil, false
	}
	doc, ok := typed.(*cdadocument.CDADocument)
	return doc, ok
}

// setCDADocument stores doc inside the shared message object so the next
// step's inputData["message"] carries it forward. Creates the message map
// if a step is somehow invoked without one (defensive; the real pipeline
// engine always provides it).
func setCDADocument(outputData map[string]interface{}, doc *cdadocument.CDADocument) {
	msg, ok := outputData["message"].(map[string]interface{})
	if !ok {
		msg = make(map[string]interface{})
		outputData["message"] = msg
	}
	msg["_cdaDocument"] = doc
}
