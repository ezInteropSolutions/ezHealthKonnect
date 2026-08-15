package validator_test

import (
	"testing"

	cdadocument "ezhealthkonnect/cda/document"
)

// choiceConstraintDoc builds a minimal *cdadocument.CDADocument directly
// (rather than parsing a full XML fixture) with Raw set to a Consultation
// Note templateId so DetectDocumentType resolves consistently, and
// SectionsByKey populated only with the given present section keys — enough
// to exercise Validate()'s SectionChoiceConstraint handling in isolation.
func choiceConstraintDoc(presentKeys ...string) *cdadocument.CDADocument {
	sectionsByKey := make(map[string]*cdadocument.CDASection, len(presentKeys))
	for _, key := range presentKeys {
		sectionsByKey[key] = &cdadocument.CDASection{Key: key}
	}
	return &cdadocument.CDADocument{
		Raw:           `<ClinicalDocument><templateId root="2.16.840.1.113883.10.20.22.1.4"/></ClinicalDocument>`,
		SectionsByKey: sectionsByKey,
	}
}

func TestValidate_ChoiceConstraint_SatisfiedByAssessmentAndPlan(t *testing.T) {
	v := newValidator(t)
	// Consultation Note's own SHALL sections (historyOfPresentIllness,
	// allergiesAndIntolerances, problems) plus the assessmentAndPlan branch.
	report := v.Validate(choiceConstraintDoc("historyOfPresentIllness", "allergiesAndIntolerances", "problems", "assessmentAndPlan"))

	if len(report.ChoiceConstraintReports) != 1 {
		t.Fatalf("got %d choice constraint reports, want 1", len(report.ChoiceConstraintReports))
	}
	ccr := report.ChoiceConstraintReports[0]
	if !ccr.Satisfied {
		t.Errorf("expected constraint satisfied via assessmentAndPlan branch, got unsatisfied")
	}
	if len(ccr.SatisfiedBranch) != 1 || ccr.SatisfiedBranch[0] != "assessmentAndPlan" {
		t.Errorf("SatisfiedBranch = %v, want [assessmentAndPlan]", ccr.SatisfiedBranch)
	}
	if report.ShallScore != 1.0 {
		t.Errorf("ShallScore = %v, want 1.0 (all 3 SHALL sections + satisfied choice constraint)", report.ShallScore)
	}
}

func TestValidate_ChoiceConstraint_SatisfiedByAssessmentAndPlanOfTreatment(t *testing.T) {
	v := newValidator(t)
	report := v.Validate(choiceConstraintDoc("historyOfPresentIllness", "allergiesAndIntolerances", "problems", "assessment", "planOfTreatment"))

	if len(report.ChoiceConstraintReports) != 1 {
		t.Fatalf("got %d choice constraint reports, want 1", len(report.ChoiceConstraintReports))
	}
	ccr := report.ChoiceConstraintReports[0]
	if !ccr.Satisfied {
		t.Errorf("expected constraint satisfied via assessment+planOfTreatment branch, got unsatisfied")
	}
	if len(ccr.SatisfiedBranch) != 2 {
		t.Errorf("SatisfiedBranch = %v, want [assessment planOfTreatment]", ccr.SatisfiedBranch)
	}
	if report.ShallScore != 1.0 {
		t.Errorf("ShallScore = %v, want 1.0", report.ShallScore)
	}
}

func TestValidate_ChoiceConstraint_UnsatisfiedWhenNeitherBranchPresent(t *testing.T) {
	v := newValidator(t)
	// Only assessment alone (not the full assessment+planOfTreatment branch)
	// — a real, common non-conformant case: neither branch is fully present.
	report := v.Validate(choiceConstraintDoc("historyOfPresentIllness", "allergiesAndIntolerances", "problems", "assessment"))

	if len(report.ChoiceConstraintReports) != 1 {
		t.Fatalf("got %d choice constraint reports, want 1", len(report.ChoiceConstraintReports))
	}
	ccr := report.ChoiceConstraintReports[0]
	if ccr.Satisfied {
		t.Errorf("expected constraint unsatisfied (assessment alone satisfies neither branch), got satisfied via %v", ccr.SatisfiedBranch)
	}
	if len(ccr.SatisfiedBranch) != 0 {
		t.Errorf("SatisfiedBranch should be empty when unsatisfied, got %v", ccr.SatisfiedBranch)
	}
	// 3 SHALL sections present + 1 unsatisfied choice constraint unit = 3/4.
	if report.ShallScore != 0.75 {
		t.Errorf("ShallScore = %v, want 0.75 (3 of 4 SHALL units present)", report.ShallScore)
	}
}

func TestValidate_ChoiceConstraint_NoConstraintsForDocumentTypeWithoutAny(t *testing.T) {
	v := newValidator(t)
	// CCD's Raw templateId, no choiceConstraints registered for CCD at all.
	doc := &cdadocument.CDADocument{
		Raw:           `<ClinicalDocument><templateId root="2.16.840.1.113883.10.20.22.1.2"/></ClinicalDocument>`,
		SectionsByKey: map[string]*cdadocument.CDASection{},
	}
	report := v.Validate(doc)
	if len(report.ChoiceConstraintReports) != 0 {
		t.Errorf("expected no choice constraint reports for CCD, got %d", len(report.ChoiceConstraintReports))
	}
}
