package assembly

import (
	"fmt"
	"log"
)

// AssemblyRuleEngine runs registered rules sequentially against an AssemblyContext.
type AssemblyRuleEngine interface {
	Register(rule AssemblyRule)
	Run(ctx *AssemblyContext) error
}

// DefaultRuleEngine is the production implementation.
// Rules execute in registration order; a rule error is logged and skipped, not fatal.
type DefaultRuleEngine struct {
	rules []AssemblyRule
}

// NewDefaultRuleEngine returns an engine with no rules registered.
func NewDefaultRuleEngine() *DefaultRuleEngine {
	return &DefaultRuleEngine{}
}

// Register appends a rule to the execution list.
func (e *DefaultRuleEngine) Register(rule AssemblyRule) {
	e.rules = append(e.rules, rule)
}

// Run executes all registered rules in order. It initialises the context's
// DedupRedirects and Removed maps if nil, so callers do not have to.
// A single rule error does not stop subsequent rules.
func (e *DefaultRuleEngine) Run(ctx *AssemblyContext) error {
	if ctx.DedupRedirects == nil {
		ctx.DedupRedirects = make(map[string]string)
	}
	if ctx.Removed == nil {
		ctx.Removed = make(map[string]bool)
	}

	var errs []error
	for _, rule := range e.rules {
		if ctx.Config.IsRuleDisabled(rule.Name()) {
			log.Printf("  [assembly] rule %q disabled by config — skipping", rule.Name())
			continue
		}
		if err := rule.Apply(ctx); err != nil {
			log.Printf("  [assembly] rule %q error: %v — continuing", rule.Name(), err)
			errs = append(errs, fmt.Errorf("rule %q: %w", rule.Name(), err))
		}
	}

	if len(errs) > 0 {
		// Return a combined error so the caller can log it; the rules that did
		// succeed have already updated ctx, so partial results are usable.
		msg := "assembly engine: "
		for i, err := range errs {
			if i > 0 {
				msg += "; "
			}
			msg += err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}
