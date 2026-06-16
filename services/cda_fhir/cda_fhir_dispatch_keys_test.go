// services/cda_fhir/cda_fhir_dispatch_keys_test.go
//
// Guards against the exact bug class found in production: section resolution
// (cda/document/section_parser.go) correctly resolves a section to its schema
// key via templateId/LOINC, but document_mapper.go's typedSectionDispatchers
// map — a second, independently-maintained key list — has a typo'd or missing
// key, so the section is silently dropped even though it was resolved
// correctly. (This happened to "planOfTreatment" and "payersInsurance": both
// already had templateId+LOINC registered and resolved correctly; the
// dispatch table simply never had matching keys.)
//
// This test fails loudly, at build/test time, if a dispatch key doesn't
// correspond to either a real schema section key or a documented
// title-fallback alias — instead of silently dropping data at runtime.
package cdafhir_test

import (
	"path/filepath"
	"runtime"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	cdafhir "ezhealthkonnect/services/cda_fhir"
)

// dispatchKeyAliases lists dispatch-table keys that intentionally do NOT
// match a schema section key directly — each is a documented fallback for a
// specific title-normalization or legacy-naming variant. Any dispatch key
// not in this list MUST be a real key from cda/schemas/ccda_2_1.json.
var dispatchKeyAliases = map[string]string{
	"careTeams":  "title-fallback plural form of \"Care Teams\" (no templateId/LOINC match)",
	"carePlan":   "alternate title wording for the Plan of Care/Treatment section",
	"planOfCare": "alternate title wording for the Plan of Care/Treatment section",
	"labResults": "alternate title wording for the Results/Laboratory section",
	"payors":     "legacy alias for the Payers/Insurance section",
}

func TestDispatchedSectionKeys_AllResolveToARealSchemaSectionOrDocumentedAlias(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// services/cda_fhir/this_file.go → up two levels → project root → cda/schemas
	root := filepath.Join(filepath.Dir(filename), "..", "..")
	schemaDir := filepath.Join(root, "cda", "schemas")

	loader, err := cdaSchema.NewCDASchemaLoader(schemaDir)
	if err != nil {
		t.Fatalf("failed to load CDA schema: %v", err)
	}

	knownKeys := make(map[string]bool)
	for _, sec := range loader.AllSections() {
		knownKeys[sec.Key] = true
	}

	for _, dispatchKey := range cdafhir.DispatchedSectionKeys() {
		if knownKeys[dispatchKey] {
			continue
		}
		if _, isDocumentedAlias := dispatchKeyAliases[dispatchKey]; isDocumentedAlias {
			continue
		}
		t.Errorf("dispatch key %q does not match any schema section key and is not a "+
			"documented alias in dispatchKeyAliases — this section's entries are being "+
			"silently dropped from FHIR output. Either fix the key to match the schema, "+
			"or add it to dispatchKeyAliases with a comment explaining why.", dispatchKey)
	}
}
