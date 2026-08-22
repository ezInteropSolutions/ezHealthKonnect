// Package fhirpath implements a deliberately scoped SUBSET of the FHIRPath
// expression language (http://hl7.org/fhirpath) — just the operations real
// FHIR R4 resource-level invariants (a StructureDefinition's own
// constraint[].expression) actually use in practice, not the full language.
//
// This exists to replace fhir/r4/constraints.go's hand-written Go predicate
// functions with the spec's own formulas, executed directly, instead of a
// human's paraphrase of them — see that file's own comment for why a full
// FHIRPath engine was deliberately not attempted either; this package is the
// generic middle ground: broad enough to cover real invariants without
// requiring a complete implementation of the specification.
//
// Supported: field navigation (a.b.c), the boolean/comparison/arithmetic/
// union operators (and, or, xor, implies, =, !=, >, <, >=, <=, |, +, -, *, /),
// string/number/bool literals, backtick-quoted identifiers (`div`),
// %resource / %context environment variables, and a small self-registering
// function registry (exists, empty, hasValue, count, where, select, first,
// all, children, contains, isDistinct, not — see functions_*.go). Adding a
// new function is a new file, never an edit to a growing switch statement.
//
// Deliberately NOT supported (Compile returns an error, never panics):
// descendants(), resolve(), iif(), ofType(), the infix "in"/"is"/"as"
// operators, date/time literals, and indexers ([n]). A rule using one of
// these fails to compile and must be skipped by the caller, not treated as
// fatal — see fhir/r4/compiler.go's handling of CompiledProfile.SpecConstraints.
package fhirpath
