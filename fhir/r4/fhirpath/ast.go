package fhirpath

import "fmt"

// Node is a parsed FHIRPath expression node. Every node type knows how to
// evaluate itself against an EvalContext's current focus.
type Node interface {
	Eval(ctx *EvalContext) (Collection, error)
}

// LiteralNode is a bool/number/string constant.
type LiteralNode struct{ Value interface{} }

func (n *LiteralNode) Eval(ctx *EvalContext) (Collection, error) {
	if err := ctx.step(); err != nil {
		return nil, err
	}
	return Collection{n.Value}, nil
}

// PathNode navigates a named child field on every item currently in focus.
type PathNode struct{ Name string }

func (n *PathNode) Eval(ctx *EvalContext) (Collection, error) {
	if err := ctx.step(); err != nil {
		return nil, err
	}
	return navigateField(ctx.Focus, n.Name), nil
}

// InvocationNode is FHIRPath's "." operator: Receiver's result becomes the
// evaluation focus for Step (e.g. `a.b`, `a.exists()`).
type InvocationNode struct{ Receiver, Step Node }

func (n *InvocationNode) Eval(ctx *EvalContext) (Collection, error) {
	if err := ctx.step(); err != nil {
		return nil, err
	}
	recv, err := n.Receiver.Eval(ctx)
	if err != nil {
		return nil, err
	}
	return n.Step.Eval(ctx.withFocus(recv))
}

// FunctionCallNode invokes a registered Function with the current focus as
// its receiver collection and its (unevaluated) argument expressions.
type FunctionCallNode struct {
	Name string
	Args []Node
}

func (n *FunctionCallNode) Eval(ctx *EvalContext) (Collection, error) {
	if err := ctx.step(); err != nil {
		return nil, err
	}
	fn, ok := GetFunction(n.Name)
	if !ok {
		return nil, fmt.Errorf("fhirpath: unknown function %q", n.Name)
	}
	return fn.Call(ctx.Focus, n.Args, ctx)
}

// BinaryOpNode is an infix operator (and, or, xor, implies, comparisons,
// arithmetic, union).
type BinaryOpNode struct {
	Op          string
	Left, Right Node
}

func (n *BinaryOpNode) Eval(ctx *EvalContext) (Collection, error) {
	if err := ctx.step(); err != nil {
		return nil, err
	}
	left, err := n.Left.Eval(ctx)
	if err != nil {
		return nil, err
	}

	// and/or short-circuit — the common case in real invariants
	// (e.g. `a.exists() and b.exists()`), and avoids evaluating a Right
	// side that may not even apply once Left has already decided the result.
	switch n.Op {
	case "and":
		lb, _ := left.AsBool()
		if !lb {
			return Collection{false}, nil
		}
		right, err := n.Right.Eval(ctx)
		if err != nil {
			return nil, err
		}
		rb, _ := right.AsBool()
		return Collection{rb}, nil
	case "or":
		lb, _ := left.AsBool()
		if lb {
			return Collection{true}, nil
		}
		right, err := n.Right.Eval(ctx)
		if err != nil {
			return nil, err
		}
		rb, _ := right.AsBool()
		return Collection{rb}, nil
	}

	right, err := n.Right.Eval(ctx)
	if err != nil {
		return nil, err
	}
	return evalBinaryOp(n.Op, left, right)
}

// UnaryOpNode is a prefix operator (currently only numeric negation).
type UnaryOpNode struct {
	Op      string
	Operand Node
}

func (n *UnaryOpNode) Eval(ctx *EvalContext) (Collection, error) {
	if err := ctx.step(); err != nil {
		return nil, err
	}
	v, err := n.Operand.Eval(ctx)
	if err != nil {
		return nil, err
	}
	return evalUnaryOp(n.Op, v)
}

// EnvVarNode resolves a "%name" environment variable (%resource, %context).
type EnvVarNode struct{ Name string }

func (n *EnvVarNode) Eval(ctx *EvalContext) (Collection, error) {
	if err := ctx.step(); err != nil {
		return nil, err
	}
	v, ok := ctx.EnvVars[n.Name]
	if !ok || v == nil {
		return Collection{}, nil
	}
	return Collection{v}, nil
}
