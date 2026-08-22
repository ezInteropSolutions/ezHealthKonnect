package fhirpath

import (
	"fmt"
	"strings"
)

func init() {
	RegisterFunction(containsFunc{})
	RegisterFunction(isDistinctFunc{})
	RegisterFunction(notFunc{})
	RegisterFunction(intersectFunc{})
}

// containsFunc implements contains(substring) — string membership, e.g.
// bdl-8's `fullUrl.contains('/_history/').not()`.
type containsFunc struct{}

func (containsFunc) Name() string { return "contains" }
func (containsFunc) Call(receiver Collection, args []Node, ctx *EvalContext) (Collection, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("fhirpath: contains() requires exactly one argument")
	}
	if len(receiver) != 1 {
		return Collection{}, nil
	}
	s, ok := receiver[0].(string)
	if !ok {
		return Collection{}, nil
	}
	argVal, err := args[0].Eval(ctx)
	if err != nil {
		return nil, err
	}
	if len(argVal) != 1 {
		return Collection{}, nil
	}
	sub, ok := argVal[0].(string)
	if !ok {
		return Collection{}, nil
	}
	return Collection{strings.Contains(s, sub)}, nil
}

// isDistinctFunc implements isDistinct() — true if every item in the
// receiver is unique (bdl-7: fullUrl+versionId pairs must be distinct).
type isDistinctFunc struct{}

func (isDistinctFunc) Name() string { return "isDistinct" }
func (isDistinctFunc) Call(receiver Collection, _ []Node, _ *EvalContext) (Collection, error) {
	seen := make([]interface{}, 0, len(receiver))
	for _, item := range receiver {
		for _, s := range seen {
			if valuesEqual(s, item) {
				return Collection{false}, nil
			}
		}
		seen = append(seen, item)
	}
	return Collection{true}, nil
}

// intersectFunc implements intersect(other) — items present in both the
// receiver and other, by value equality. Needed by obs-7's own duplicate-
// component-code check: `component.code.where(coding.intersect(%resource.code.coding).exists())`.
type intersectFunc struct{}

func (intersectFunc) Name() string { return "intersect" }
func (intersectFunc) Call(receiver Collection, args []Node, ctx *EvalContext) (Collection, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("fhirpath: intersect() requires exactly one argument")
	}
	other, err := args[0].Eval(ctx)
	if err != nil {
		return nil, err
	}
	var out Collection
	for _, item := range receiver {
		for _, o := range other {
			if valuesEqual(item, o) {
				out = append(out, item)
				break
			}
		}
	}
	return out, nil
}

// notFunc implements not() — boolean negation of the receiver's
// singleton-boolean value. A FHIRPath function (postfix `.not()`), not a
// prefix operator — real invariants use it as e.g. `(...).not()`.
type notFunc struct{}

func (notFunc) Name() string { return "not" }
func (notFunc) Call(receiver Collection, _ []Node, _ *EvalContext) (Collection, error) {
	b, err := receiver.AsBool()
	if err != nil {
		return nil, err
	}
	return Collection{!b}, nil
}
