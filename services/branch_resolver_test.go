package services

import (
	"ezhealthkonnect/models"
	"testing"
)

// TestIfThenElseResolver_CanHandle_RealCanonicalStepType guards against the exact
// bug found 2026-09-08: CanHandle only recognized the pre-rename "pre.logic"/
// "pre.logic.ifthenelse" names, never the real "if_then_else" name every saved
// step and the executor's own GetStepType() actually use — so
// BranchResolverFactory.GetResolver("if_then_else") always returned nil and the
// whole auto-skip mechanism was unreachable for any real step, in every engine
// that called it.
func TestIfThenElseResolver_CanHandle_RealCanonicalStepType(t *testing.T) {
	r := NewIfThenElseResolver()
	if !r.CanHandle("if_then_else") {
		t.Fatal("CanHandle(\"if_then_else\") should be true — this is the real, current canonical step type")
	}
	if r.GetStepType() != "if_then_else" {
		t.Fatalf("GetStepType() = %q, want %q", r.GetStepType(), "if_then_else")
	}
	// Legacy names must keep working too — some environment could still have a
	// pre-rename saved step.
	if !r.CanHandle("pre.logic") || !r.CanHandle("pre.logic.ifthenelse") {
		t.Fatal("CanHandle should still accept the legacy pre-rename names")
	}
}

// TestSwitchCaseResolver_CanHandle_RealCanonicalStepType is the same regression
// guard for switch_case.
func TestSwitchCaseResolver_CanHandle_RealCanonicalStepType(t *testing.T) {
	r := NewSwitchCaseResolver()
	if !r.CanHandle("switch_case") {
		t.Fatal("CanHandle(\"switch_case\") should be true — this is the real, current canonical step type")
	}
	if r.GetStepType() != "switch_case" {
		t.Fatalf("GetStepType() = %q, want %q", r.GetStepType(), "switch_case")
	}
	if !r.CanHandle("pre.logic.switch") || !r.CanHandle("pre.logic.switchcase") {
		t.Fatal("CanHandle should still accept the legacy pre-rename names")
	}
}

// TestIfThenElseResolver_GetStepsToSkip_SkipsOnlyTheUntakenBranch proves the
// actual branch-membership logic: a child step tagged with
// ParentConditionalStepID + BranchType="false" gets returned when the "true"
// branch was taken, and a "true"-tagged sibling does not.
func TestIfThenElseResolver_GetStepsToSkip_SkipsOnlyTheUntakenBranch(t *testing.T) {
	trueBranch, falseBranch := "true", "false"
	conditionalID := "cond-1"
	conditionalStep := &models.TransformationStep{ID: conditionalID, StepType: "if_then_else"}

	allSteps := []models.TransformationStep{
		*conditionalStep,
		{ID: "true-child", ParentConditionalStepID: &conditionalID, BranchType: &trueBranch},
		{ID: "false-child", ParentConditionalStepID: &conditionalID, BranchType: &falseBranch},
		{ID: "unrelated", StepType: "field_mapping"},
	}

	r := NewIfThenElseResolver()
	skip := r.GetStepsToSkip(conditionalStep, map[string]interface{}{"branchTaken": "true"}, allSteps)

	if len(skip) != 1 || skip[0] != "false-child" {
		t.Fatalf("expected exactly [\"false-child\"] to be skipped when branchTaken=true, got %v", skip)
	}
}

// TestSwitchCaseResolver_GetStepsToSkip_SkipsEveryOtherCase proves the same for
// switch_case, including that ALL non-matching cases' steps are returned, not
// just one.
func TestSwitchCaseResolver_GetStepsToSkip_SkipsEveryOtherCase(t *testing.T) {
	caseACH, caseCHK, caseNON := "ACH", "CHK", "NON"
	conditionalID := "switch-1"
	conditionalStep := &models.TransformationStep{ID: conditionalID, StepType: "switch_case"}

	allSteps := []models.TransformationStep{
		*conditionalStep,
		{ID: "ach-child", ParentConditionalStepID: &conditionalID, CaseValue: &caseACH},
		{ID: "chk-child", ParentConditionalStepID: &conditionalID, CaseValue: &caseCHK},
		{ID: "non-child", ParentConditionalStepID: &conditionalID, CaseValue: &caseNON},
	}

	r := NewSwitchCaseResolver()
	skip := r.GetStepsToSkip(conditionalStep, map[string]interface{}{"caseMatched": "ACH"}, allSteps)

	if len(skip) != 2 {
		t.Fatalf("expected 2 non-matching case steps to be skipped, got %d: %v", len(skip), skip)
	}
	skipSet := map[string]bool{}
	for _, id := range skip {
		skipSet[id] = true
	}
	if !skipSet["chk-child"] || !skipSet["non-child"] {
		t.Fatalf("expected both chk-child and non-child to be skipped, got %v", skip)
	}
	if skipSet["ach-child"] {
		t.Fatal("the matched case's own child must never be in the skip list")
	}
}

// TestBranchResolverFactory_GetStepsToSkip_ResolvesRealStepTypeEndToEnd proves
// the fix through the actual entry point ExecutePipeline calls
// (tps.branchResolver.GetStepsToSkip), using the real "switch_case" step type
// string a saved pipeline actually has — not a synthetic one.
func TestBranchResolverFactory_GetStepsToSkip_ResolvesRealStepTypeEndToEnd(t *testing.T) {
	caseF, caseM := "F", "M"
	conditionalID := "switch-1"
	conditionalStep := models.TransformationStep{ID: conditionalID, StepType: "switch_case"}

	allSteps := []models.TransformationStep{
		conditionalStep,
		{ID: "female-child", ParentConditionalStepID: &conditionalID, CaseValue: &caseF},
		{ID: "male-child", ParentConditionalStepID: &conditionalID, CaseValue: &caseM},
	}

	factory := NewBranchResolverFactory()
	skip := factory.GetStepsToSkip(&conditionalStep, map[string]interface{}{"caseMatched": "F"}, allSteps)

	if len(skip) != 1 || skip[0] != "male-child" {
		t.Fatalf("expected exactly [\"male-child\"] via the real factory + real step type, got %v", skip)
	}
}
