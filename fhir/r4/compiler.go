package r4

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// rawSchema — mirrors the .gz JSON format on disk; unexported, compile-time only
// ─────────────────────────────────────────────────────────────────────────────

type rawSchema struct {
	ResourceType string                `json:"resourceType"`
	Version      string                `json:"version"`
	Name         string                `json:"name"`
	Description  string                `json:"description"`
	BaseResource string                `json:"baseResource"`
	Elements     map[string]*rawElem   `json:"elements"`
	Required     []string              `json:"required"`
	MustSupport  []string              `json:"mustSupport"`
}

type rawElem struct {
	Path            string   `json:"path"`
	DataType        string   `json:"dataType"`
	Cardinality     string   `json:"cardinality"`   // e.g. "0..1", "1..*"
	Required        bool     `json:"required"`
	MustSupport     bool     `json:"mustSupport"`
	IsModifier      bool     `json:"isModifier"`
	IsSummary       bool     `json:"isSummary"`
	Constraints     []string `json:"constraints"`
	ValueSet        string   `json:"valueSet"`
	BindingStrength string   `json:"bindingStrength"` // required | extensible | preferred | example
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

	// ── Terminology bindings ──────────────────────────────────────────────────

	// Bindings contains only required and extensible bindings; preferred/example
	// are omitted — they carry no validation obligation.
	Bindings []CompiledBinding

	// ── Constraint functions ──────────────────────────────────────────────────

	// Constraints are pre-resolved Go predicate functions for this resource type.
	Constraints []CompiledConstraint

	// ── Informational ─────────────────────────────────────────────────────────

	// MustSupport lists element paths that US Core (or other profiles) require
	// implementors to support; used for information/warning only.
	MustSupport  []string
	ElementCount int
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
		Version:      version,
		ResourceType: raw.ResourceType,
		Profile:      profile,
		Required:     make(map[string]bool, n),
		MinCard:      make(map[string]int, n),
		MaxCard:      make(map[string]int, n),
		DataTypes:    make(map[string]string, n),
		IsModifier:   make(map[string]bool),
		ElementCount: n,
		MustSupport:  raw.MustSupport,
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
		if elem.MustSupport {
			cp.MustSupport = appendUniq(cp.MustSupport, path)
		}

		// Only compile required/extensible bindings — preferred/example carry
		// no normative validation obligation in FHIR R4.
		if elem.ValueSet != "" &&
			(elem.BindingStrength == "required" || elem.BindingStrength == "extensible") {
			cp.Bindings = append(cp.Bindings, CompiledBinding{
				Path:            path,
				ValueSetURL:     normalizeValueSetURL(elem.ValueSet),
				BindingStrength: elem.BindingStrength,
			})
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

	return cp
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
