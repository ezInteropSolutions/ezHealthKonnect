package validator

import (
	"testing"

	"ezhealthkonnect/edi"
)

func emptySpec() *edi.X12SpecDef {
	return &edi.X12SpecDef{Segments: map[string]*edi.X12SegmentDef{}}
}

func TestValidate_ValidFieldsPass(t *testing.T) {
	result := &edi.ParseResult{
		Fields: map[string]*edi.EDIField{
			"BPR.01": {Path: "BPR.01", Value: "C", DataType: "ID"},
			"BPR.02": {Path: "BPR.02", Value: "1250.00", DataType: "R"},
		},
	}
	got := Validate(emptySpec(), result)
	if !got.Valid {
		t.Errorf("expected valid, got issues: %#v", got.Issues)
	}
}

func TestValidate_BadDataTypeIsAnError(t *testing.T) {
	result := &edi.ParseResult{
		Fields: map[string]*edi.EDIField{
			"BPR.02": {Path: "BPR.02", Value: "not-a-number", DataType: "R"},
		},
	}
	got := Validate(emptySpec(), result)
	if got.Valid {
		t.Fatal("expected invalid for a non-numeric R field")
	}
	if len(got.Issues) != 1 || got.Issues[0].Path != "BPR.02" || got.Issues[0].Severity != "error" {
		t.Errorf("unexpected issues: %#v", got.Issues)
	}
}

func TestValidate_LengthBoundsEnforced(t *testing.T) {
	result := &edi.ParseResult{
		Fields: map[string]*edi.EDIField{
			"N1.01": {Path: "N1.01", Value: "TOOLONG", DataType: "ID", MaxLength: 2},
		},
	}
	got := Validate(emptySpec(), result)
	if got.Valid {
		t.Fatal("expected invalid for a value exceeding MaxLength")
	}
}

func TestValidate_FixedValueMismatchIsAnError(t *testing.T) {
	result := &edi.ParseResult{
		Fields: map[string]*edi.EDIField{
			"ST.01": {Path: "ST.01", Value: "837", DataType: "ID", FixedValue: "835"},
		},
	}
	got := Validate(emptySpec(), result)
	if got.Valid {
		t.Fatal("expected invalid for a fixed-value mismatch")
	}
	if got.Issues[0].Message == "" {
		t.Error("expected a descriptive message for the fixed-value mismatch")
	}
}

func TestValidate_EmptyValuesSkipped(t *testing.T) {
	result := &edi.ParseResult{
		Fields: map[string]*edi.EDIField{
			"REF.01": {Path: "REF.01", Value: "", DataType: "R"}, // empty — should never be recorded by the parser, but validator must not crash if it is
		},
	}
	got := Validate(emptySpec(), result)
	if !got.Valid {
		t.Errorf("expected valid (empty values are skipped), got issues: %#v", got.Issues)
	}
}

// --- SyntaxRule (business-rule) tests: always warnings, never affect Valid ---

func specWithSegment(id string, rules ...edi.SyntaxRule) *edi.X12SpecDef {
	return &edi.X12SpecDef{
		Segments: map[string]*edi.X12SegmentDef{
			id: {ID: id, SyntaxRules: rules},
		},
	}
}

func TestValidate_PairedRule_ViolationIsWarningNotError(t *testing.T) {
	spec := specWithSegment("BPR", edi.SyntaxRule{Type: "P", Positions: []string{"06", "07"}})
	result := &edi.ParseResult{
		SegmentInstances: []edi.SegmentInstance{
			{SegmentID: "BPR", RawByPos: map[string]string{"06": "01"}}, // 06 present, 07 missing — violates P
		},
	}
	got := Validate(spec, result)
	if !got.Valid {
		t.Error("a SyntaxRule violation must never make Result.Valid false")
	}
	if len(got.Issues) != 1 || got.Issues[0].Severity != "warning" {
		t.Errorf("expected exactly one warning, got: %#v", got.Issues)
	}
}

func TestValidate_PairedRule_BothPresentSatisfies(t *testing.T) {
	spec := specWithSegment("BPR", edi.SyntaxRule{Type: "P", Positions: []string{"06", "07"}})
	result := &edi.ParseResult{
		SegmentInstances: []edi.SegmentInstance{
			{SegmentID: "BPR", RawByPos: map[string]string{"06": "01", "07": "999"}},
		},
	}
	got := Validate(spec, result)
	if len(got.Issues) != 0 {
		t.Errorf("expected no issues when both paired elements present, got: %#v", got.Issues)
	}
}

func TestValidate_PairedRule_BothAbsentSatisfies(t *testing.T) {
	spec := specWithSegment("BPR", edi.SyntaxRule{Type: "P", Positions: []string{"06", "07"}})
	result := &edi.ParseResult{
		SegmentInstances: []edi.SegmentInstance{
			{SegmentID: "BPR", RawByPos: map[string]string{}},
		},
	}
	got := Validate(spec, result)
	if len(got.Issues) != 0 {
		t.Errorf("expected no issues when both paired elements absent, got: %#v", got.Issues)
	}
}

func TestValidate_ConditionalRule(t *testing.T) {
	spec := specWithSegment("BPR", edi.SyntaxRule{Type: "C", Positions: []string{"08", "09"}})
	violating := &edi.ParseResult{SegmentInstances: []edi.SegmentInstance{
		{SegmentID: "BPR", RawByPos: map[string]string{"08": "DA"}},
	}}
	if got := Validate(spec, violating); len(got.Issues) != 1 {
		t.Errorf("expected 1 warning for conditional violation, got: %#v", got.Issues)
	}

	satisfying := &edi.ParseResult{SegmentInstances: []edi.SegmentInstance{
		{SegmentID: "BPR", RawByPos: map[string]string{"08": "DA", "09": "123"}},
	}}
	if got := Validate(spec, satisfying); len(got.Issues) != 0 {
		t.Errorf("expected no warnings when condition satisfied, got: %#v", got.Issues)
	}
}

func TestValidate_ListConditionalRule_CASStyleTrio(t *testing.T) {
	// The real CAS shape: if any of the trio (amount, quantity) is present,
	// the reason code (positions[0]) must be too.
	spec := specWithSegment("CAS", edi.SyntaxRule{Type: "L", Positions: []string{"05", "06", "07"}})

	violating := &edi.ParseResult{SegmentInstances: []edi.SegmentInstance{
		{SegmentID: "CAS", RawByPos: map[string]string{"06": "10.00"}}, // amount present, reason code missing
	}}
	if got := Validate(spec, violating); len(got.Issues) != 1 {
		t.Errorf("expected 1 warning, got: %#v", got.Issues)
	}

	satisfying := &edi.ParseResult{SegmentInstances: []edi.SegmentInstance{
		{SegmentID: "CAS", RawByPos: map[string]string{"05": "45", "06": "10.00"}},
	}}
	if got := Validate(spec, satisfying); len(got.Issues) != 0 {
		t.Errorf("expected no warnings, got: %#v", got.Issues)
	}
}

func TestValidate_RequiredAtLeastOneRule(t *testing.T) {
	spec := specWithSegment("REF", edi.SyntaxRule{Type: "R", Positions: []string{"02", "03"}})
	violating := &edi.ParseResult{SegmentInstances: []edi.SegmentInstance{
		{SegmentID: "REF", RawByPos: map[string]string{"01": "EV"}},
	}}
	if got := Validate(spec, violating); len(got.Issues) != 1 {
		t.Errorf("expected 1 warning, got: %#v", got.Issues)
	}

	satisfying := &edi.ParseResult{SegmentInstances: []edi.SegmentInstance{
		{SegmentID: "REF", RawByPos: map[string]string{"01": "EV", "02": "12345"}},
	}}
	if got := Validate(spec, satisfying); len(got.Issues) != 0 {
		t.Errorf("expected no warnings, got: %#v", got.Issues)
	}
}

func TestValidate_ExclusionRule(t *testing.T) {
	spec := specWithSegment("N4", edi.SyntaxRule{Type: "E", Positions: []string{"03", "07"}})
	violating := &edi.ParseResult{SegmentInstances: []edi.SegmentInstance{
		{SegmentID: "N4", RawByPos: map[string]string{"03": "27702", "07": "US-NC"}},
	}}
	if got := Validate(spec, violating); len(got.Issues) != 1 {
		t.Errorf("expected 1 warning, got: %#v", got.Issues)
	}

	satisfying := &edi.ParseResult{SegmentInstances: []edi.SegmentInstance{
		{SegmentID: "N4", RawByPos: map[string]string{"03": "27702"}},
	}}
	if got := Validate(spec, satisfying); len(got.Issues) != 0 {
		t.Errorf("expected no warnings, got: %#v", got.Issues)
	}
}
