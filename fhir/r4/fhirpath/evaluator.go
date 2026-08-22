package fhirpath

import (
	"fmt"
	"strings"
)

// Collection is FHIRPath's core value model: every expression evaluates to
// an ordered list of items. A single value is a 1-item collection; absence
// is a 0-item collection — this is what lets exists()/empty()/count() work
// uniformly on every expression, the single most common pattern across real
// FHIR R4 invariants (see fhir/r4/fhirpath's package doc for the survey this
// was based on).
type Collection []interface{}

// AsBool applies FHIRPath's singleton-boolean-evaluation rule: a 1-item
// collection whose sole item is itself a bool converts directly; any other
// non-empty collection is "true" (pure existence check); an empty collection
// is "false". This is a deliberate simplification of FHIRPath's real 3-valued
// (true/false/empty) logic — every rule this package targets is written so
// that simplification never changes the answer (see doc.go).
func (c Collection) AsBool() (bool, error) {
	if len(c) == 0 {
		return false, nil
	}
	if len(c) == 1 {
		if b, ok := c[0].(bool); ok {
			return b, nil
		}
	}
	return true, nil
}

// maxEvalSteps bounds total node evaluations for a single Eval call. This is
// a correctness safety net (catches a bug in this evaluator causing runaway
// recursion), not a security boundary — Phase 1 rules come only from trusted
// spec data. A future phase letting users author their own rules must add a
// real timeout/goroutine boundary on top of this, the same pattern already
// used by services/executors/enrichment/script_enrichment_executor.go for
// user-authored JavaScript.
const maxEvalSteps = 100000

// EvalContext carries the current evaluation focus plus environment
// variables and a shared step counter.
type EvalContext struct {
	Focus   Collection
	EnvVars map[string]interface{}
	steps   *int
}

func (ctx *EvalContext) step() error {
	*ctx.steps++
	if *ctx.steps > maxEvalSteps {
		return fmt.Errorf("fhirpath: evaluation exceeded step limit (%d) — likely a runaway expression", maxEvalSteps)
	}
	return nil
}

// withFocus returns a new context sharing this context's env vars and step
// counter but with a different current focus — used whenever a node needs to
// evaluate a sub-expression against something other than the ambient focus
// (invocation chaining, where()/select()'s per-item evaluation, ...).
func (ctx *EvalContext) withFocus(focus Collection) *EvalContext {
	return &EvalContext{Focus: focus, EnvVars: ctx.EnvVars, steps: ctx.steps}
}

// Eval evaluates a compiled node against a resource (a plain
// map[string]interface{}, matching how every FHIR resource already flows
// through this codebase) and returns the raw collection result.
func Eval(node Node, resource interface{}) (Collection, error) {
	steps := 0
	ctx := &EvalContext{
		Focus:   Collection{resource},
		EnvVars: map[string]interface{}{"resource": resource, "context": resource},
		steps:   &steps,
	}
	return node.Eval(ctx)
}

// EvalBool evaluates a rule expression and reduces the result via
// Collection.AsBool — what a validator invariant check actually needs.
func EvalBool(node Node, resource interface{}) (bool, error) {
	result, err := Eval(node, resource)
	if err != nil {
		return false, err
	}
	return result.AsBool()
}

// navigateField resolves a named child on every item currently in focus,
// over the plain map[string]interface{}/[]interface{} shapes this codebase
// already uses for every parsed FHIR resource.
func navigateField(focus Collection, name string) Collection {
	var out Collection
	for _, item := range focus {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if v, ok := m[name]; ok && v != nil {
			out = append(out, flattenChild(v)...)
			continue
		}
		// FHIR choice-type elements (value[x], effective[x], abatement[x], ...)
		// are serialized in JSON with a type-name suffix (valueString,
		// abatementDateTime, ...) — real invariant expressions still
		// reference them by their bare base name. fhir/r4/validator.go's
		// validateStructure already solved this exact problem for required-
		// field checks (scanning for "name" + an uppercase-starting suffix);
		// mirrored here rather than reinventing a second convention. Only
		// tried as a fallback after an exact-key match fails, so it can
		// never shadow a real, present field.
		for k, v := range m {
			if v == nil || len(k) <= len(name) || !strings.HasPrefix(k, name) {
				continue
			}
			if r := k[len(name)]; r < 'A' || r > 'Z' {
				continue
			}
			out = append(out, flattenChild(v)...)
		}
	}
	return out
}

func flattenChild(v interface{}) Collection {
	if slice, ok := v.([]interface{}); ok {
		return Collection(slice)
	}
	return Collection{v}
}
