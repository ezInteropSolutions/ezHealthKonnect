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
