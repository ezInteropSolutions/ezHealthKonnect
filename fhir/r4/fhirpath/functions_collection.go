package fhirpath

import "fmt"

func init() {
	RegisterFunction(whereFunc{})
	RegisterFunction(selectFunc{})
	RegisterFunction(firstFunc{})
	RegisterFunction(allFunc{})
	RegisterFunction(childrenFunc{})
}

// whereFunc implements where(criteria) — keeps items satisfying criteria,
// evaluated once per item with that item as the new focus.
type whereFunc struct{}

func (whereFunc) Name() string { return "where" }
func (whereFunc) Call(receiver Collection, args []Node, ctx *EvalContext) (Collection, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("fhirpath: where() requires exactly one argument")
	}
	return filterByCriteria(receiver, args[0], ctx)
}

// selectFunc implements select(projection) — evaluates projection once per
// item (that item as the new focus) and flattens the results.
type selectFunc struct{}

func (selectFunc) Name() string { return "select" }
func (selectFunc) Call(receiver Collection, args []Node, ctx *EvalContext) (Collection, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("fhirpath: select() requires exactly one argument")
	}
	var out Collection
	for _, item := range receiver {
		result, err := args[0].Eval(ctx.withFocus(Collection{item}))
		if err != nil {
			return nil, err
		}
		out = append(out, result...)
	}
	return out, nil
}

// firstFunc implements first() — the first item, or an empty collection.
type firstFunc struct{}

func (firstFunc) Name() string { return "first" }
func (firstFunc) Call(receiver Collection, _ []Node, _ *EvalContext) (Collection, error) {
	if len(receiver) == 0 {
		return Collection{}, nil
	}
	return Collection{receiver[0]}, nil
}

// allFunc implements all(criteria) — true if every item satisfies criteria
// (vacuously true for an empty receiver, matching FHIRPath semantics).
type allFunc struct{}

func (allFunc) Name() string { return "all" }
func (allFunc) Call(receiver Collection, args []Node, ctx *EvalContext) (Collection, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("fhirpath: all() requires exactly one argument")
	}
	for _, item := range receiver {
		result, err := args[0].Eval(ctx.withFocus(Collection{item}))
		if err != nil {
			return nil, err
		}
		if b, _ := result.AsBool(); !b {
			return Collection{false}, nil
		}
	}
	return Collection{true}, nil
}

// childrenFunc implements children() — every direct child value across
// every item in the receiver, one level deep (map values and slice
// elements). Needed by ele-1's `children().count() > id.count()` — "does
// this element carry more content than just its own id".
type childrenFunc struct{}

func (childrenFunc) Name() string { return "children" }
func (childrenFunc) Call(receiver Collection, _ []Node, _ *EvalContext) (Collection, error) {
	var out Collection
	for _, item := range receiver {
		switch v := item.(type) {
		case map[string]interface{}:
			for _, child := range v {
				if slice, ok := child.([]interface{}); ok {
					out = append(out, slice...)
					continue
				}
				out = append(out, child)
			}
		case []interface{}:
			out = append(out, v...)
		}
	}
	return out, nil
}
