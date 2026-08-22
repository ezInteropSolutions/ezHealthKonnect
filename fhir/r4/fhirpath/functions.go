package fhirpath

import "fmt"

// Function is a single FHIRPath function (exists, where, count, ...).
// Implementations self-register via RegisterFunction in their own file's
// init() — mirrors services/executors/enrichment/format_parsers.go's
// registry pattern. Adding support for a new function is a new file, never
// an edit to a growing switch statement.
type Function interface {
	// Name is the function's FHIRPath name, e.g. "exists".
	Name() string
	// Call evaluates the function against its receiver collection (the
	// current focus) and its unevaluated argument expressions.
	// Arguments are passed unevaluated because functions like where() and
	// select() must evaluate them once per receiver item, with that item
	// as the new focus — not once against the ambient focus.
	Call(receiver Collection, args []Node, ctx *EvalContext) (Collection, error)
}

var functionRegistry = map[string]Function{}

// RegisterFunction adds fn to the global registry. Panics on a duplicate
// name — a startup-time programming error, not a runtime condition, so
// panicking (rather than silently overwriting) is deliberate.
func RegisterFunction(fn Function) {
	name := fn.Name()
	if _, exists := functionRegistry[name]; exists {
		panic(fmt.Sprintf("fhirpath: function %q already registered", name))
	}
	functionRegistry[name] = fn
}

// GetFunction looks up a registered function by name.
func GetFunction(name string) (Function, bool) {
	fn, ok := functionRegistry[name]
	return fn, ok
}

// filterByCriteria evaluates criteria once per receiver item, with that item
// as the new focus, keeping items where the criteria's singleton-boolean
// value is true. Shared by exists(criteria) and where(criteria) — the two
// functions differ only in what they do with the filtered set.
func filterByCriteria(receiver Collection, criteria Node, ctx *EvalContext) (Collection, error) {
	var out Collection
	for _, item := range receiver {
		result, err := criteria.Eval(ctx.withFocus(Collection{item}))
		if err != nil {
			return nil, err
		}
		if b, _ := result.AsBool(); b {
			out = append(out, item)
		}
	}
	return out, nil
}
