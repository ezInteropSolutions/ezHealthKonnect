package fhirpath

// Rule is a single FHIR invariant pulled from real spec data: a key/
// severity/human-readable description plus the raw FHIRPath expression,
// compiled once into an AST so evaluation is zero-parse-cost on the
// validation hot path — mirrors CompiledProfile's existing compile-once
// philosophy in fhir/r4/compiler.go.
type Rule struct {
	Key        string
	Severity   string // "error" | "warning"
	Human      string
	Expression string
	compiled   Node
}

// CompileRule parses expression once and returns a Rule ready for repeated
// evaluation. A non-nil error means expression uses something outside this
// package's deliberately scoped subset (see doc.go) — callers must log and
// skip the rule, never treat this as fatal.
func CompileRule(key, severity, human, expression string) (*Rule, error) {
	node, err := Compile(expression)
	if err != nil {
		return nil, err
	}
	return &Rule{
		Key:        key,
		Severity:   severity,
		Human:      human,
		Expression: expression,
		compiled:   node,
	}, nil
}

// Evaluate runs the rule's compiled expression against resource and reduces
// it via Collection.AsBool — true means the invariant holds.
func (r *Rule) Evaluate(resource interface{}) (bool, error) {
	return EvalBool(r.compiled, resource)
}
