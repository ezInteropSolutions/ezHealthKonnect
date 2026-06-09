// uscdi/vocabulary.go
// USCDIVocabulary — loads USCDI v3 data elements from a JSON file and provides
// clinical-term-anchored search for the smart field picker.
//
// Design:
//   - Loaded once via NewUSCDIVocabulary; safe for concurrent reads.
//   - Search uses case-insensitive token matching with a simple scoring model.
//   - CDA XPath is NOT populated here; the CDA schema layer enriches results.

package uscdi

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// USCDIVocabulary holds the loaded USCDI v3 data elements and maintains
// indexes for fast class-level and format-level lookups.
type USCDIVocabulary struct {
	version  USCDIVersion
	elements []*USCDIElement

	// Indexes built at load time — read-only after construction.
	byClass      map[string][]*USCDIElement // keyed by normalised class name
	byFHIR       map[string][]*USCDIElement // keyed by FHIR resource type
	byCDASection map[string][]*USCDIElement // keyed by CDA section key
	byHL7Field   map[string][]*USCDIElement // keyed by HL7 field (e.g. "PID-5")

	mu sync.RWMutex
}

// NewUSCDIVocabulary loads USCDI elements from the JSON file at schemaPath.
// Returns a ready-to-query vocabulary or an error if the file is missing/invalid.
func NewUSCDIVocabulary(schemaPath string) (*USCDIVocabulary, error) {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("uscdi: cannot read vocabulary file %s: %w", schemaPath, err)
	}

	var elements []*USCDIElement
	if err := json.Unmarshal(data, &elements); err != nil {
		return nil, fmt.Errorf("uscdi: cannot parse vocabulary file %s: %w", schemaPath, err)
	}

	v := &USCDIVocabulary{
		version:      USCDIv3,
		elements:     elements,
		byClass:      make(map[string][]*USCDIElement),
		byFHIR:       make(map[string][]*USCDIElement),
		byCDASection: make(map[string][]*USCDIElement),
		byHL7Field:   make(map[string][]*USCDIElement),
	}
	v.buildIndexes()
	return v, nil
}

// Version returns the USCDI version this vocabulary represents.
func (v *USCDIVocabulary) Version() USCDIVersion {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.version
}

// AllClasses returns the unique USCDI class names in alphabetical order.
func (v *USCDIVocabulary) AllClasses() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	classes := make([]string, 0, len(v.byClass))
	for c := range v.byClass {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	return classes
}

// GetByClass returns all elements belonging to a USCDI class.
// The match is case-insensitive.
func (v *USCDIVocabulary) GetByClass(class string) []*USCDIElement {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.byClass[normalise(class)]
}

// GetByFHIRResource returns all elements whose fhirResource matches the given type.
func (v *USCDIVocabulary) GetByFHIRResource(resource string) []*USCDIElement {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.byFHIR[normalise(resource)]
}

// GetByCDASection returns all elements that map to the given CDA section key
// (e.g. "allergiesAndIntolerances").
func (v *USCDIVocabulary) GetByCDASection(section string) []*USCDIElement {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.byCDASection[normalise(section)]
}

// GetElement returns the first USCDIElement matching both class and element name.
// Returns nil if not found.
func (v *USCDIVocabulary) GetElement(class, element string) *USCDIElement {
	v.mu.RLock()
	defer v.mu.RUnlock()

	nc, ne := normalise(class), normalise(element)
	for _, el := range v.elements {
		if normalise(el.Class) == nc && normalise(el.Element) == ne {
			return el
		}
	}
	return nil
}

// Search returns up to maxResults SearchResult entries ranked by relevance to
// the free-text query. The format parameter ("cda"/"fhir"/"hl7") filters results
// to elements that have mappings for that format; pass "" for all elements.
func (v *USCDIVocabulary) Search(query string, format string, maxResults int) []*SearchResult {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if maxResults <= 0 {
		maxResults = 20
	}

	tokens := tokenise(query)
	if len(tokens) == 0 {
		return nil
	}

	var results []*SearchResult
	for _, el := range v.elements {
		if !matchesFormat(el, format) {
			continue
		}
		score := scoreElement(el, tokens)
		if score <= 0 {
			continue
		}
		results = append(results, buildSearchResult(el, score, format))
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results
}

// =====================================
// Internal helpers
// =====================================

func (v *USCDIVocabulary) buildIndexes() {
	for _, el := range v.elements {
		nc := normalise(el.Class)
		v.byClass[nc] = append(v.byClass[nc], el)

		if el.FHIRResource != "" {
			nf := normalise(el.FHIRResource)
			v.byFHIR[nf] = append(v.byFHIR[nf], el)
		}

		if el.CDASection != "" {
			ns := normalise(el.CDASection)
			v.byCDASection[ns] = append(v.byCDASection[ns], el)
		}

		if el.HL7Field != "" {
			// HL7 fields can be multi-valued ("OBX-3/5"), index first only.
			field := strings.SplitN(el.HL7Field, "/", 2)[0]
			nf := normalise(field)
			v.byHL7Field[nf] = append(v.byHL7Field[nf], el)
		}
	}
}

// scoreElement computes a relevance score ≥ 0. Returns 0 if no token matches.
func scoreElement(el *USCDIElement, tokens []string) float64 {
	corpus := strings.ToLower(
		el.Element + " " + el.Class + " " + el.CDASection + " " +
			el.FHIRResource + " " + el.FHIRPath + " " + el.CDAField,
	)

	var score float64
	for _, tok := range tokens {
		if !strings.Contains(corpus, tok) {
			// All tokens must appear somewhere — any miss scores 0.
			return 0
		}
		// Bonus weights: exact substring matches in higher-priority fields.
		elLower := strings.ToLower(el.Element)
		classLower := strings.ToLower(el.Class)

		if strings.Contains(elLower, tok) {
			score += 3.0
		} else if strings.Contains(classLower, tok) {
			score += 1.5
		} else {
			score += 0.5
		}
		// Exact element name prefix boosts.
		if strings.HasPrefix(elLower, tok) {
			score += 1.0
		}
	}
	return score
}

func buildSearchResult(el *USCDIElement, score float64, format string) *SearchResult {
	shortPath := el.CDASection + "." + el.CDAField
	if el.CDASection == "" || el.CDAField == "" {
		shortPath = strings.ToLower(strings.ReplaceAll(el.Class, " ", "_")) +
			"." + el.CDAField
	}
	if shortPath == "." || shortPath == "" {
		shortPath = el.FHIRPath
	}

	return &SearchResult{
		ShortPath:  shortPath,
		Label:      el.Element,
		USCDIClass: el.Class,
		ValueSet:   el.ValueSet,
		FHIRPath:   el.FHIRPath,
		HL7Field:   el.HL7Field,
		Format:     format,
		Score:      score,
		Source:     el,
	}
}

// matchesFormat returns true if the element has data for the requested format,
// or if format is empty (match all).
func matchesFormat(el *USCDIElement, format string) bool {
	switch strings.ToLower(format) {
	case "cda", "ccda", "ccd":
		return el.CCDInject && el.CDASection != ""
	case "hl7", "hl7v2":
		return el.HL7Inject && el.HL7Field != ""
	case "fhir":
		return el.FHIRPath != ""
	default:
		return true
	}
}

// normalise lowercases a string for index lookups.
func normalise(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// tokenise splits a query into lowercase tokens, filtering single-character noise.
func tokenise(query string) []string {
	raw := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return r == ' ' || r == '_' || r == '-' || r == '.' || r == '/'
	})
	var tokens []string
	for _, t := range raw {
		if len(t) >= 2 {
			tokens = append(tokens, t)
		}
	}
	return tokens
}
