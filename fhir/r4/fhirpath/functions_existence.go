package fhirpath

func init() {
	RegisterFunction(existsFunc{})
	RegisterFunction(emptyFunc{})
	RegisterFunction(hasValueFunc{})
	RegisterFunction(countFunc{})
}

// existsFunc implements exists([criteria]) — the single most common
// operation across real FHIR invariants. With no argument, true if the
// receiver has any items. With a criteria argument, true if any item
// satisfies it (evaluated once per item, that item as the new focus).
type existsFunc struct{}

func (existsFunc) Name() string { return "exists" }
func (existsFunc) Call(receiver Collection, args []Node, ctx *EvalContext) (Collection, error) {
	if len(args) == 0 {
		return Collection{len(receiver) > 0}, nil
	}
	filtered, err := filterByCriteria(receiver, args[0], ctx)
	if err != nil {
		return nil, err
	}
	return Collection{len(filtered) > 0}, nil
}

// emptyFunc implements empty() — true if the receiver has no items. The
// natural complement to exists(), used just as often in real invariants
// (e.g. obs-6: "dataAbsentReason.empty() or value.empty()").
type emptyFunc struct{}

func (emptyFunc) Name() string { return "empty" }
func (emptyFunc) Call(receiver Collection, _ []Node, _ *EvalContext) (Collection, error) {
	return Collection{len(receiver) == 0}, nil
}

// hasValueFunc implements hasValue() — true if the receiver is a single
// primitive value. Real FHIRPath distinguishes a primitive-with-extension
// wrapper from a plain object; this codebase's plain-map resource
// representation has no such wrapper, so "has a value" reduces to "is a
// single scalar, not an object/array/nil" — which is exactly what every
// real ele-1 usage needs (`hasValue() or (children().count() > id.count())`:
// the rule only needs ONE side to hold).
type hasValueFunc struct{}

func (hasValueFunc) Name() string { return "hasValue" }
func (hasValueFunc) Call(receiver Collection, _ []Node, _ *EvalContext) (Collection, error) {
	if len(receiver) != 1 {
		return Collection{false}, nil
	}
	switch receiver[0].(type) {
	case map[string]interface{}, []interface{}, nil:
		return Collection{false}, nil
	default:
		return Collection{true}, nil
	}
}

// countFunc implements count() — the number of items in the receiver.
type countFunc struct{}

func (countFunc) Name() string { return "count" }
func (countFunc) Call(receiver Collection, _ []Node, _ *EvalContext) (Collection, error) {
	return Collection{float64(len(receiver))}, nil
}
