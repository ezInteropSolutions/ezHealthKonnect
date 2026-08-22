package fhirpath

import "fmt"

// evalBinaryOp handles every infix operator that isn't short-circuited by
// BinaryOpNode.Eval itself (and/or) — implies/xor, equality, relational,
// union, and arithmetic all evaluate both sides first, then combine here.
func evalBinaryOp(op string, left, right Collection) (Collection, error) {
	switch op {
	case "|":
		out := make(Collection, 0, len(left)+len(right))
		out = append(out, left...)
		out = append(out, right...)
		return out, nil
	case "implies":
		lb, _ := left.AsBool()
		if !lb {
			return Collection{true}, nil
		}
		rb, _ := right.AsBool()
		return Collection{rb}, nil
	case "xor":
		lb, _ := left.AsBool()
		rb, _ := right.AsBool()
		return Collection{lb != rb}, nil
	case "=", "!=":
		eq := collectionsEqual(left, right)
		if op == "!=" {
			eq = !eq
		}
		return Collection{eq}, nil
	case ">", "<", ">=", "<=":
		return compareCollections(op, left, right)
	case "+", "-", "*", "/":
		return arithmetic(op, left, right)
	}
	return nil, fmt.Errorf("fhirpath: unsupported operator %q", op)
}

func evalUnaryOp(op string, v Collection) (Collection, error) {
	if op != "-" {
		return nil, fmt.Errorf("fhirpath: unsupported unary operator %q", op)
	}
	if len(v) != 1 {
		return Collection{}, nil
	}
	f, ok := toFloat(v[0])
	if !ok {
		return nil, fmt.Errorf("fhirpath: cannot negate a non-numeric value (%T)", v[0])
	}
	return Collection{-f}, nil
}

// collectionsEqual compares two collections item-by-item, in order — real
// FHIRPath equality for collections of matching length. An empty side never
// equals a non-empty side (mirrors FHIRPath's empty-propagation without
// needing full 3-valued logic — see Collection.AsBool's comment).
func collectionsEqual(a, b Collection) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !valuesEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func valuesEqual(a, b interface{}) bool {
	if af, ok := toFloat(a); ok {
		bf, ok := toFloat(b)
		return ok && af == bf
	}
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	}
	return a == b
}

func compareCollections(op string, left, right Collection) (Collection, error) {
	if len(left) != 1 || len(right) != 1 {
		return Collection{}, nil
	}

	if lf, ok := toFloat(left[0]); ok {
		if rf, ok := toFloat(right[0]); ok {
			return Collection{compareOrdered(op, lf < rf, lf == rf, lf > rf)}, nil
		}
	}
	if ls, ok := left[0].(string); ok {
		if rs, ok := right[0].(string); ok {
			return Collection{compareOrdered(op, ls < rs, ls == rs, ls > rs)}, nil
		}
	}
	return nil, fmt.Errorf("fhirpath: cannot compare %T and %T with %q", left[0], right[0], op)
}

func compareOrdered(op string, lt, eq, gt bool) bool {
	switch op {
	case ">":
		return gt
	case "<":
		return lt
	case ">=":
		return gt || eq
	case "<=":
		return lt || eq
	}
	return false
}

func arithmetic(op string, left, right Collection) (Collection, error) {
	if len(left) != 1 || len(right) != 1 {
		return Collection{}, nil
	}

	if lf, ok := toFloat(left[0]); ok {
		if rf, ok := toFloat(right[0]); ok {
			switch op {
			case "+":
				return Collection{lf + rf}, nil
			case "-":
				return Collection{lf - rf}, nil
			case "*":
				return Collection{lf * rf}, nil
			case "/":
				if rf == 0 {
					return Collection{}, nil
				}
				return Collection{lf / rf}, nil
			}
		}
	}
	if op == "+" {
		if ls, ok := left[0].(string); ok {
			if rs, ok := right[0].(string); ok {
				return Collection{ls + rs}, nil
			}
		}
	}
	return nil, fmt.Errorf("fhirpath: cannot apply %q to %T and %T", op, left[0], right[0])
}

// toFloat coerces the numeric shapes that can appear in a parsed-JSON FHIR
// resource (float64 from encoding/json, plus int/int64 defensively) into a
// float64 for arithmetic/comparison.
func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}
