package validator

import (
	"testing"

	"ezhealthkonnect/astm"
)

func testSpec(t *testing.T) *astm.ASTMSpecDef {
	t.Helper()
	loader, err := astm.NewASTMSchemaLoader("../schemas/e1394_97")
	if err != nil {
		t.Fatalf("NewASTMSchemaLoader: %v", err)
	}
	return loader.Spec()
}

func TestValidate_NilResult_ReturnsValidWithNoIssues(t *testing.T) {
	result := Validate(testSpec(t), nil)
	if !result.Valid {
		t.Error("expected Valid=true for a nil ParseResult")
	}
	if len(result.Issues) != 0 {
		t.Errorf("expected no issues, got %d", len(result.Issues))
	}
}

func TestValidate_WellFormedFields_NoIssues(t *testing.T) {
	spec := testSpec(t)
	parsed, err := astm.ParseMessage(spec, "generic_lab_result",
		"H|\\^&|M1||A^1^S|||||LIS||P|E1394-97|20231004120000\r"+
			"P|1||MRN1||Doe^Jane||19800101|F\r"+
			"O|1|SPEC1||GLU|R\r"+
			"R|1|GLU|95|mg/dL|70-110|N||F\r"+
			"L|1|N\r")
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}

	result := Validate(spec, parsed)
	if !result.Valid {
		t.Errorf("expected Valid=true for well-formed fields, got issues: %+v", result.Issues)
	}
}

func TestValidate_MalformedNumericField_FlagsError(t *testing.T) {
	spec := testSpec(t)
	parsed, err := astm.ParseMessage(spec, "generic_lab_result",
		"H|\\^&|M1||A^1^S|||||LIS||P|E1394-97|20231004120000\r"+
			// P.2 (sequenceNumber) is declared NM but given a non-numeric value —
			// a genuinely malformed value per this record's own schema, not a
			// missing-optional-field case.
			"P|NOTANUMBER||MRN1||Doe^Jane||19800101|F\r"+
			"L|1|N\r")
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}

	result := Validate(spec, parsed)
	if result.Valid {
		t.Error("expected Valid=false when a NM field holds a non-numeric value")
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Severity == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected at least one error-severity issue, got: %+v", result.Issues)
	}
}

func TestValidate_MissingOptionalField_NeverBlocks(t *testing.T) {
	// "Flexible, not rigid" — a field the schema marks required (e.g. P.2
	// sequenceNumber, required:true) being ABSENT from the record entirely
	// (as opposed to present-but-malformed, covered above) must never flip
	// Valid to false here — presence/required-ness is documentation only in
	// this validator's own stated scope (see this package's own file header).
	spec := testSpec(t)
	parsed, err := astm.ParseMessage(spec, "generic_lab_result",
		"H|\\^&|M1||A^1^S|||||LIS||P|E1394-97|20231004120000\r"+
			"P||||||||\r"+ // sequenceNumber and every other P field left blank
			"L|1|N\r")
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}

	result := Validate(spec, parsed)
	if !result.Valid {
		t.Errorf("a missing-but-required field must never block Valid on its own, got issues: %+v", result.Issues)
	}
}

func TestValidate_MalformedDateField_FlagsError(t *testing.T) {
	spec := testSpec(t)
	parsed, err := astm.ParseMessage(spec, "generic_lab_result",
		"H|\\^&|M1||A^1^S|||||LIS||P|E1394-97|20231004120000\r"+
			// P.8 (dateOfBirth) is declared DT (must be exactly 8 digits) —
			// given a clearly malformed value.
			"P|1||MRN1||Doe^Jane||NOT-A-DATE|F\r"+
			"L|1|N\r")
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}

	result := Validate(spec, parsed)
	if result.Valid {
		t.Error("expected Valid=false when a DT field holds a non-date value")
	}
}

func TestValidate_IssuePath_IdentifiesTheOffendingField(t *testing.T) {
	spec := testSpec(t)
	parsed, err := astm.ParseMessage(spec, "generic_lab_result",
		"H|\\^&|M1||A^1^S|||||LIS||P|E1394-97|20231004120000\r"+
			"P|BADSEQ||MRN1||Doe^Jane||19800101|F\r"+
			"L|1|N\r")
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}

	result := Validate(spec, parsed)
	if len(result.Issues) == 0 {
		t.Fatal("expected at least one issue")
	}
	for _, issue := range result.Issues {
		if issue.Path == "" {
			t.Error("every issue should carry a non-empty Path identifying the offending field")
		}
	}
}
