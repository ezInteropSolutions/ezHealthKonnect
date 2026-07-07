// services/executors/transform/cda_to_fhir_executor_test.go
package transform

import (
	"strings"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

func TestFormatSectionErrorWarnings_FiltersToWarningSeverityOnly(t *testing.T) {
	sectionErrors := []cdafhir.SectionError{
		{SectionKey: "functionalStatus", EntryIndex: 13, FieldKey: "value", Error: "entry has more than one <value>; only the first was mapped", Severity: "warning"},
		{SectionKey: "allergiesAndIntolerances", EntryIndex: 0, FieldKey: "code", Error: "required field missing", Severity: "error"},
	}

	warnings := formatSectionErrorWarnings(sectionErrors)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1 (error-severity entries must not be promoted as warnings)", len(warnings))
	}
	if !strings.Contains(warnings[0], "functionalStatus") || !strings.Contains(warnings[0], "entry 13") || !strings.Contains(warnings[0], "value") {
		t.Errorf("warning message = %q, missing expected section/entry/field context", warnings[0])
	}
}

func TestFormatSectionErrorWarnings_NoWarnings_ReturnsNil(t *testing.T) {
	warnings := formatSectionErrorWarnings(nil)
	if len(warnings) != 0 {
		t.Errorf("got %d warnings, want 0", len(warnings))
	}

	warnings = formatSectionErrorWarnings([]cdafhir.SectionError{
		{SectionKey: "results", Severity: "error"},
	})
	if len(warnings) != 0 {
		t.Errorf("got %d warnings, want 0 (only an error-severity entry was present)", len(warnings))
	}
}

func TestFormatSectionErrorWarnings_MultipleWarnings_PreservesOrder(t *testing.T) {
	sectionErrors := []cdafhir.SectionError{
		{SectionKey: "functionalStatus", EntryIndex: 5, FieldKey: "value", Error: "first dropped value", Severity: "warning"},
		{SectionKey: "functionalStatus", EntryIndex: 9, FieldKey: "value", Error: "second dropped value", Severity: "warning"},
	}

	warnings := formatSectionErrorWarnings(sectionErrors)
	if len(warnings) != 2 {
		t.Fatalf("got %d warnings, want 2", len(warnings))
	}
	if !strings.Contains(warnings[0], "first dropped value") || !strings.Contains(warnings[1], "second dropped value") {
		t.Errorf("warnings = %v, want order preserved", warnings)
	}
}

func TestResolveCDAToFHIRConfig_NilConfig_ReturnsDefaults(t *testing.T) {
	cfg := resolveCDAToFHIRConfig(nil)
	if cfg.BundleType != "collection" || cfg.ProfileMode != "us-core" || cfg.OnSectionFailure != "continue" {
		t.Errorf("cfg = %+v, want the documented defaults", cfg)
	}
	if cfg.DocType != "" {
		t.Errorf("DocType = %q, want empty when no config at all", cfg.DocType)
	}
}

func TestResolveCDAToFHIRConfig_DocTypeKey_UsedDirectly(t *testing.T) {
	cfg := resolveCDAToFHIRConfig(map[string]interface{}{"docType": "CCD"})
	if cfg.DocType != "CCD" {
		t.Errorf("DocType = %q, want %q", cfg.DocType, "CCD")
	}
}

// TestResolveCDAToFHIRConfig_DocumentTypeAlias_FallsBackWhenDocTypeAbsent is the
// regression test for the real gap this fix closes: CDAStepBuilder.js's General
// tab writes the Document Type dropdown to step.config.documentType (its own
// established key, also used for OOB template-version/delta-compute lookups
// elsewhere in that file) -- but this struct's field was only ever tagged
// json:"docType", so the dropdown's value silently never reached the mapper.
func TestResolveCDAToFHIRConfig_DocumentTypeAlias_FallsBackWhenDocTypeAbsent(t *testing.T) {
	cfg := resolveCDAToFHIRConfig(map[string]interface{}{"documentType": "Discharge Summary"})
	if cfg.DocType != "Discharge Summary" {
		t.Errorf("DocType = %q, want %q (documentType alias must be honored when docType is absent)", cfg.DocType, "Discharge Summary")
	}
}

func TestResolveCDAToFHIRConfig_DocumentTypeAuto_TreatedAsUnset(t *testing.T) {
	// CDAStepBuilder.js defaults its own dropdown to the literal string "auto"
	// when nothing has been explicitly chosen -- that must not become a literal
	// DocType value the mapper tries to use.
	cfg := resolveCDAToFHIRConfig(map[string]interface{}{"documentType": "auto"})
	if cfg.DocType != "" {
		t.Errorf("DocType = %q, want empty (\"auto\" is the UI's unset sentinel, not a real value)", cfg.DocType)
	}
}

func TestResolveCDAToFHIRConfig_DocTypeTakesPrecedenceOverDocumentType(t *testing.T) {
	// If a step config somehow carries both keys, the canonical "docType" key
	// must win -- documentType is only ever a fallback for when docType is absent.
	cfg := resolveCDAToFHIRConfig(map[string]interface{}{"docType": "CCD", "documentType": "Discharge Summary"})
	if cfg.DocType != "CCD" {
		t.Errorf("DocType = %q, want %q (docType must take precedence)", cfg.DocType, "CCD")
	}
}
