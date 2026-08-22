package r4

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"ezhealthkonnect/fhir/r4/fhirpath"
)

// ─────────────────────────────────────────────────────────────────────────────
// rawSchema — mirrors the .gz JSON format on disk; unexported, compile-time only
// ─────────────────────────────────────────────────────────────────────────────

type rawSchema struct {
	ResourceType        string               `json:"resourceType"`
	Version             string               `json:"version"`
	Name                string               `json:"name"`
	Description         string               `json:"description"`
	BaseResource        string               `json:"baseResource"`
	Elements            map[string]*rawElem  `json:"elements"`
	// ResourceConstraints are the invariants declared on the resource's own
	// root element (obs-6, bun-1, con-4, ...) — where the majority of real,
	// genuinely resource-specific business rules live, as opposed to the
	// ele-1/ext-1 boilerplate repeated on every child element. See
	// cmd/build-fhir-schemas/build_fhir_schemas.py's own comment on this
	// field for why it's separate from any one element's own Constraints.
	ResourceConstraints []RawConstraint      `json:"resourceConstraints"`
	Required            []string             `json:"required"`
	MustSupport         []string             `json:"mustSupport"`
}

type rawElem struct {
	Path            string          `json:"path"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	DataType        string          `json:"dataType"`
	Cardinality     string          `json:"cardinality"` // e.g. "0..1", "1..*"
	Required        bool            `json:"required"`
	MustSupport     bool            `json:"mustSupport"`
	IsModifier      bool            `json:"isModifier"`
	IsSummary       bool            `json:"isSummary"`
	Constraints     []RawConstraint `json:"constraints"`
	ValueSet        string          `json:"valueSet"`
	BindingStrength string          `json:"bindingStrength"` // required | extensible | preferred | example
}

// RawConstraint mirrors one entry of a StructureDefinition element's
// constraint[] array: key/severity/human-text/the real FHIRPath expression.
type RawConstraint struct {
	Key        string `json:"key"`
	Severity   string `json:"severity"`
	Human      string `json:"human"`
	Expression string `json:"expression"`
}

// ─────────────────────────────────────────────────────────────────────────────
// CompiledProfile — immutable after construction; all fields are read-only
// ─────────────────────────────────────────────────────────────────────────────

// CompiledProfile is a pre-compiled, immutable FHIR resource profile.
// It is built once at startup from the raw .gz schema and used for all
// subsequent validation calls with no further parsing or file I/O.
type CompiledProfile struct {
	Version      string
	ResourceType string
	Profile      string // "base", "us-core", …

	// Name/Description are the resource-level display text from the source
	// StructureDefinition (e.g. Name="Patient", Description="Demographics
	// and administrative information about an individual..."). Informational
	// only — never used for validation — for UI/catalog consumers like
	// services/parsers/fhir_parser_service.go and
	// controllers/cda_schema_controller.go, which previously had to go
	// through the separate legacy loader (fhir/schema_loader.go) for this
	// text since it was compiled away here until now.
	Name        string
	Description string

	// ── Structure rules ───────────────────────────────────────────────────────

	// Required contains every element path whose min cardinality is ≥ 1.
	Required map[string]bool
	// MinCard is the minimum number of occurrences per element path (0 or 1).
	MinCard map[string]int
	// MaxCard is the maximum number of occurrences per path (-1 = unbounded).
	MaxCard map[string]int
	// DataTypes maps each element path to its FHIR data type string.
	DataTypes map[string]string
	// IsModifier flags modifier elements (semantic significance warnings).
	IsModifier map[string]bool
	// IsSummary flags elements included in a resource's _summary=true view.
	IsSummary map[string]bool
	// ElementNames/ElementDescriptions carry the same informational,
	// validation-irrelevant per-element display text as Name/Description
	// above, keyed by element path.
	ElementNames        map[string]string
	ElementDescriptions map[string]string

	// ── Terminology bindings ──────────────────────────────────────────────────

	// Bindings contains only required and extensible bindings; preferred/example
	// are omitted — they carry no validation obligation.
	Bindings []CompiledBinding
	// AllBindings carries every element's valueSet/bindingStrength regardless
	// of strength (including preferred/example) — for display/UX consumers
	// (e.g. a field-picker UI showing "this field's suggested codes") that
	// want to show a binding exists even when it imposes no validation
	// obligation. Bindings above stays scoped to enforcement; this is a
	// separate, purely informational concern.
	AllBindings []CompiledBinding

	// ── Constraint functions ──────────────────────────────────────────────────

	// Constraints are pre-resolved Go predicate functions for this resource type
	// (fhir/r4/constraints.go) — hand-written invariants not (yet, or ever, for
	// IG-specific ones like Da Vinci PAS) covered by SpecConstraints below.
	Constraints []CompiledConstraint

	// SpecConstraints are the resource's real, resource-root FHIR invariants
	// (obs-6, bun-1, con-4, ...), compiled once from RawConstraint data via
	// fhir/r4/fhirpath — evaluated generically, not hand-ported to Go. A raw
	// constraint whose expression uses something fhirpath doesn't support
	// yet is skipped here (logged, not fatal) — see compileSchema.
	SpecConstraints []*fhirpath.Rule

	// ── Informational ─────────────────────────────────────────────────────────

	// MustSupport lists element paths that US Core (or other profiles) require
	// implementors to support; used for information/warning only.
	MustSupport  []string
	ElementCount int
}

// Cardinality reconstructs the "min..max" display string (e.g. "0..1", "1..*")
// for path from MinCard/MaxCard. Shared by every consumer that wants the
// human-readable form instead of the parsed ints — e.g. UI/catalog endpoints
// that display a resource's fields (controllers/cda_schema_controller.go,
// services/parsers/fhir_parser_service.go, and the fhir.FHIRSchema-shaped
// adapter services/transform_fhir_setter.go's legacy-loader migration
// produces) — kept here once rather than re-implemented per caller.
func (cp *CompiledProfile) Cardinality(path string) string {
	min, max := cp.MinCard[path], cp.MaxCard[path]
	if max == -1 {
		return strconv.Itoa(min) + "..*"
	}
	return strconv.Itoa(min) + ".." + strconv.Itoa(max)
}

// IsMustSupport reports whether path appears in MustSupport.
func (cp *CompiledProfile) IsMustSupport(path string) bool {
	for _, p := range cp.MustSupport {
		if p == path {
			return true
		}
	}
	return false
}

// Binding finds path's valueSet/bindingStrength in AllBindings (every
// strength, not just required/extensible — for display purposes, not
// validation; see AllBindings' own doc comment).
func (cp *CompiledProfile) Binding(path string) (valueSet, strength string, ok bool) {
	for _, b := range cp.AllBindings {
		if b.Path == path {
			return b.ValueSetURL, b.BindingStrength, true
		}
	}
	return "", "", false
}

// CompiledBinding is a pre-compiled terminology binding for a single element.
type CompiledBinding struct {
	Path            string
	ValueSetURL     string // version suffix stripped (no "|4.0.1")
	BindingStrength string // "required" | "extensible"
}

// CompiledConstraint is a pre-resolved R4 invariant predicate.
type CompiledConstraint struct {
	Key         string           // e.g. "obs-6"
	Severity    string           // "error" | "warning"
	Description string
	Check       ConstraintFunc
}

// ConstraintFunc is a Go predicate that returns true when the constraint is
// satisfied (i.e. when it returns false, a violation is reported).
type ConstraintFunc func(resource map[string]interface{}) bool

// ConstraintViolation is produced when a ConstraintFunc returns false.
type ConstraintViolation struct {
	Key         string
	Severity    string
	Description string
}

// ─────────────────────────────────────────────────────────────────────────────
// loadRawSchema — reads and decodes a gzip-compressed schema file from disk
// ─────────────────────────────────────────────────────────────────────────────

func loadRawSchema(filePath string) (*rawSchema, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gzip %s: %w", filePath, err)
	}
	defer gz.Close()

	var s rawSchema
	if err := json.NewDecoder(gz).Decode(&s); err != nil {
		return nil, fmt.Errorf("decode %s: %w", filePath, err)
	}
	return &s, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// compileSchema — converts a rawSchema into an immutable CompiledProfile
// ─────────────────────────────────────────────────────────────────────────────

// compileSchema is a pure function: given a raw schema + version + profile name
// it returns a fully populated CompiledProfile with no side effects.
func compileSchema(raw *rawSchema, version, profile string, checker ConstraintChecker) *CompiledProfile {
	n := len(raw.Elements)
	cp := &CompiledProfile{
		Version:             version,
		ResourceType:        raw.ResourceType,
		Profile:             profile,
		Name:                raw.Name,
		Description:         raw.Description,
		Required:            make(map[string]bool, n),
		MinCard:             make(map[string]int, n),
		MaxCard:             make(map[string]int, n),
		DataTypes:           make(map[string]string, n),
		IsModifier:          make(map[string]bool),
		IsSummary:           make(map[string]bool),
		ElementNames:        make(map[string]string, n),
		ElementDescriptions: make(map[string]string, n),
		ElementCount:        n,
		MustSupport:         raw.MustSupport,
	}

	for path, elem := range raw.Elements {
		min, max := parseCardinality(elem.Cardinality)
		cp.MinCard[path] = min
		cp.MaxCard[path] = max

		if min >= 1 || elem.Required {
			cp.Required[path] = true
		}
		if elem.DataType != "" {
			cp.DataTypes[path] = elem.DataType
		}
		if elem.IsModifier {
			cp.IsModifier[path] = true
		}
		if elem.IsSummary {
			cp.IsSummary[path] = true
		}
		if elem.MustSupport {
			cp.MustSupport = appendUniq(cp.MustSupport, path)
		}
		if elem.Name != "" {
			cp.ElementNames[path] = elem.Name
		}
		if elem.Description != "" {
			cp.ElementDescriptions[path] = elem.Description
		}

		if elem.ValueSet != "" {
			binding := CompiledBinding{
				Path:            path,
				ValueSetURL:     normalizeValueSetURL(elem.ValueSet),
				BindingStrength: elem.BindingStrength,
			}
			// AllBindings carries every strength for display/UX consumers;
			// Bindings stays scoped to required/extensible — the only
			// strengths that carry a normative validation obligation in FHIR R4.
			cp.AllBindings = append(cp.AllBindings, binding)
			if elem.BindingStrength == "required" || elem.BindingStrength == "extensible" {
				cp.Bindings = append(cp.Bindings, binding)
			}
		}
	}

	// Also honour the top-level required array (some schemas use this instead of
	// element-level required:true).
	for _, path := range raw.Required {
		cp.Required[path] = true
		if cp.MinCard[path] < 1 {
			cp.MinCard[path] = 1
		}
	}

	// Attach pre-compiled constraint predicates for this resource type.
	if checker != nil {
		// We only need the definitions, not to run them here.
		cp.Constraints = constraintDefsFor(raw.ResourceType)
	}

	cp.SpecConstraints = compileSpecConstraints(raw.ResourceType, raw.ResourceConstraints)

	return cp
}

// compileSpecConstraints parses every raw resource-level invariant once at
// schema-compile time. An expression using something outside fhirpath's
// deliberately scoped subset (see fhir/r4/fhirpath's package doc) fails to
// compile — that rule is logged and skipped, never fatal, so a partial
// function set degrades gracefully instead of blocking startup.
func compileSpecConstraints(resourceType string, raw []RawConstraint) []*fhirpath.Rule {
	if len(raw) == 0 {
		return nil
	}
	rules := make([]*fhirpath.Rule, 0, len(raw))
	for _, rc := range raw {
		rule, err := fhirpath.CompileRule(rc.Key, rc.Severity, rc.Human, rc.Expression)
		if err != nil {
			log.Printf("⚠️  [r4.Compiler] %s: skipping constraint %s (unsupported expression): %v", resourceType, rc.Key, err)
			continue
		}
		rules = append(rules, rule)
	}
	return rules
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// parseCardinality parses FHIR cardinality strings ("0..1", "1..*", etc.)
// into integer min and max values.  max=-1 means unbounded ("*").
func parseCardinality(s string) (min, max int) {
	parts := strings.SplitN(s, "..", 2)
	if len(parts) != 2 {
		return 0, -1 // treat unknown as optional-unbounded
	}
	min, _ = strconv.Atoi(parts[0])
	if parts[1] == "*" {
		max = -1
	} else {
		max, _ = strconv.Atoi(parts[1])
	}
	return
}

// normalizeValueSetURL strips version qualifiers like "|4.0.1" from a ValueSet URL.
func normalizeValueSetURL(url string) string {
	if idx := strings.Index(url, "|"); idx != -1 {
		return url[:idx]
	}
	return url
}

// appendUniq appends s to slice only if not already present.
func appendUniq(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}
