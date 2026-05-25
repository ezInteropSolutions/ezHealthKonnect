package services

import (
	"ezhealthkonnect/models"
	"fmt"
	"log"
)

// ===============================================================
// BRANCH RESOLVER - Strategy Pattern for Conditional Branch Skipping
// ===============================================================
// This provides a unified, OOP-based approach to determining which steps
// should be skipped based on conditional routing decisions.
//
// Supports:
// - IfThenElse: Binary true/false branches
// - Switch/Case: Multiple case-based branches
//
// Usage:
//   resolver := NewBranchResolverFactory()
//   stepsToSkip := resolver.GetStepsToSkip(conditionalStep, routingInfo, allSteps)

// BranchResolver interface defines the contract for branch resolution strategies
type BranchResolver interface {
	// GetStepType returns the step type this resolver handles
	GetStepType() string

	// GetStepsToSkip returns step IDs that should be skipped based on routing decision
	// Parameters:
	//   - conditionalStep: The conditional step that made the routing decision
	//   - routingInfo: The routing information from the executor (_routing map)
	//   - allSteps: All steps in the pipeline for branch membership lookup
	GetStepsToSkip(
		conditionalStep *models.TransformationStep,
		routingInfo map[string]interface{},
		allSteps []models.TransformationStep,
	) []string

	// CanHandle returns true if this resolver can handle the given step type
	CanHandle(stepType string) bool
}

// ===============================================================
// IF-THEN-ELSE RESOLVER
// ===============================================================

// IfThenElseResolver handles branch resolution for IfThenElse steps
type IfThenElseResolver struct{}

// NewIfThenElseResolver creates a new IfThenElse resolver
func NewIfThenElseResolver() *IfThenElseResolver {
	return &IfThenElseResolver{}
}

func (r *IfThenElseResolver) GetStepType() string {
	return "pre.logic"
}

func (r *IfThenElseResolver) CanHandle(stepType string) bool {
	return stepType == "pre.logic" || stepType == "pre.logic.ifthenelse"
}

func (r *IfThenElseResolver) GetStepsToSkip(
	conditionalStep *models.TransformationStep,
	routingInfo map[string]interface{},
	allSteps []models.TransformationStep,
) []string {
	var stepsToSkip []string

	// Get which branch was taken from routing info
	branchTaken, ok := routingInfo["branchTaken"].(string)
	if !ok || branchTaken == "" {
		log.Printf("🔀 [IfThenElseResolver] No branchTaken in routing, skipping branch resolution")
		return stepsToSkip
	}

	// Determine which branch to skip (opposite of taken)
	branchToSkip := "false"
	if branchTaken == "false" {
		branchToSkip = "true"
	}

	log.Printf("🔀 [IfThenElseResolver] Branch taken: %s, will skip: %s branch", branchTaken, branchToSkip)

	// Find all steps that belong to the branch we need to skip
	for _, step := range allSteps {
		if step.ParentConditionalStepID != nil &&
			*step.ParentConditionalStepID == conditionalStep.ID &&
			step.BranchType != nil &&
			*step.BranchType == branchToSkip {
			stepsToSkip = append(stepsToSkip, step.ID)
			log.Printf("   🚫 Will skip step: %s (branch: %s)", step.StepName, branchToSkip)
		}
	}

	return stepsToSkip
}

// ===============================================================
// SWITCH/CASE RESOLVER
// ===============================================================

// SwitchCaseResolver handles branch resolution for Switch/Case steps
type SwitchCaseResolver struct{}

// NewSwitchCaseResolver creates a new Switch/Case resolver
func NewSwitchCaseResolver() *SwitchCaseResolver {
	return &SwitchCaseResolver{}
}

func (r *SwitchCaseResolver) GetStepType() string {
	return "pre.logic.switch"
}

func (r *SwitchCaseResolver) CanHandle(stepType string) bool {
	return stepType == "pre.logic.switch" || stepType == "pre.logic.switchcase"
}

func (r *SwitchCaseResolver) GetStepsToSkip(
	conditionalStep *models.TransformationStep,
	routingInfo map[string]interface{},
	allSteps []models.TransformationStep,
) []string {
	var stepsToSkip []string

	// Get which case was matched from routing info
	caseMatched, ok := routingInfo["caseMatched"].(string)
	if !ok || caseMatched == "" {
		log.Printf("🔀 [SwitchCaseResolver] No caseMatched in routing, skipping branch resolution")
		return stepsToSkip
	}

	log.Printf("🔀 [SwitchCaseResolver] Case matched: %s", caseMatched)

	// Find all steps that belong to OTHER cases (not the matched one)
	for _, step := range allSteps {
		if step.ParentConditionalStepID != nil &&
			*step.ParentConditionalStepID == conditionalStep.ID &&
			step.CaseValue != nil &&
			*step.CaseValue != caseMatched {
			stepsToSkip = append(stepsToSkip, step.ID)
			log.Printf("   🚫 Will skip step: %s (case: %s, matched: %s)", step.StepName, *step.CaseValue, caseMatched)
		}
	}

	return stepsToSkip
}

// ===============================================================
// BRANCH RESOLVER FACTORY
// ===============================================================

// BranchResolverFactory manages branch resolvers using Factory + Strategy pattern
type BranchResolverFactory struct {
	resolvers []BranchResolver
}

// NewBranchResolverFactory creates a factory with all registered resolvers
func NewBranchResolverFactory() *BranchResolverFactory {
	factory := &BranchResolverFactory{
		resolvers: make([]BranchResolver, 0),
	}

	// Register all resolvers
	factory.Register(NewIfThenElseResolver())
	factory.Register(NewSwitchCaseResolver())

	return factory
}

// Register adds a new resolver to the factory
func (f *BranchResolverFactory) Register(resolver BranchResolver) {
	f.resolvers = append(f.resolvers, resolver)
	log.Printf("🔧 [BranchResolverFactory] Registered resolver for: %s", resolver.GetStepType())
}

// GetResolver returns the appropriate resolver for a step type
func (f *BranchResolverFactory) GetResolver(stepType string) BranchResolver {
	for _, resolver := range f.resolvers {
		if resolver.CanHandle(stepType) {
			return resolver
		}
	}
	return nil
}

// GetStepsToSkip is a convenience method that finds the right resolver and gets steps to skip
func (f *BranchResolverFactory) GetStepsToSkip(
	conditionalStep *models.TransformationStep,
	routingInfo map[string]interface{},
	allSteps []models.TransformationStep,
) []string {
	resolver := f.GetResolver(conditionalStep.StepType)
	if resolver == nil {
		log.Printf("⚠️  [BranchResolverFactory] No resolver for step type: %s", conditionalStep.StepType)
		return nil
	}

	return resolver.GetStepsToSkip(conditionalStep, routingInfo, allSteps)
}

// ===============================================================
// BRANCH MEMBERSHIP HELPERS
// ===============================================================

// BranchMembership represents a step's branch affiliation
type BranchMembership struct {
	StepID                  string
	StepName                string
	ParentConditionalStepID string
	BranchType              string // For IfThenElse
	CaseValue               string // For Switch/Case
}

// GetBranchMembership extracts branch membership from a step
func GetBranchMembership(step *models.TransformationStep) *BranchMembership {
	if step.ParentConditionalStepID == nil {
		return nil
	}

	membership := &BranchMembership{
		StepID:                  step.ID,
		StepName:                step.StepName,
		ParentConditionalStepID: *step.ParentConditionalStepID,
	}

	if step.BranchType != nil {
		membership.BranchType = *step.BranchType
	}

	if step.CaseValue != nil {
		membership.CaseValue = *step.CaseValue
	}

	return membership
}

// String returns a human-readable representation
func (m *BranchMembership) String() string {
	if m.CaseValue != "" {
		return fmt.Sprintf("%s (case: %s)", m.StepName, m.CaseValue)
	}
	if m.BranchType != "" {
		return fmt.Sprintf("%s (branch: %s)", m.StepName, m.BranchType)
	}
	return m.StepName
}
