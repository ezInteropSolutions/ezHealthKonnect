// services/ai/schema_digest_fhir.go
//
// FHIR structure digester: one document per resource or profile listing its
// elements with type, cardinality, must-support and value-set binding. The
// boilerplate every resource repeats (id, meta, text, contained, extension…)
// is left out so each document is about what makes the resource distinct.
package ai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type fhirStructureDigester struct {
	label string // e.g. "FHIR R4 resource", "US Core profile (FHIR R4)"
}

func newFHIRStructureDigester(label string) *fhirStructureDigester {
	return &fhirStructureDigester{label: label}
}

type fhirStructureFile struct {
	ResourceType string                          `json:"resourceType"`
	Name         string                          `json:"name"`
	Title        string                          `json:"title"`
	Description  string                          `json:"description"`
	BaseResource string                          `json:"baseResource"`
	Profile      string                          `json:"profile"`
	Elements     map[string]fhirStructureElement `json:"elements"`
}

type fhirStructureElement struct {
	Path            string `json:"path"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	DataType        string `json:"dataType"`
	Cardinality     string `json:"cardinality"`
	MustSupport     bool   `json:"mustSupport"`
	IsModifier      bool   `json:"isModifier"`
	ValueSet        string `json:"valueSet"`
	BindingStrength string `json:"bindingStrength"`
}

// fhirRootBoilerplate are inherited Resource/DomainResource elements present
// on every resource; fhirAnyDepthBoilerplate are Element/BackboneElement
// housekeeping repeated on every backbone. Both add size, not meaning. Names
// like "language" or "text" are only boilerplate at the root —
// Patient.communication.language is real content.
var fhirRootBoilerplate = map[string]bool{
	"id": true, "meta": true, "implicitRules": true, "language": true, "text": true,
	"contained": true, "extension": true, "modifierExtension": true,
}
var fhirAnyDepthBoilerplate = map[string]bool{
	"id": true, "extension": true, "modifierExtension": true,
}

func (d *fhirStructureDigester) DigestFile(name string, content []byte) ([]SchemaDigestDoc, error) {
	var f fhirStructureFile
	if err := json.Unmarshal(content, &f); err != nil {
		return nil, err
	}
	resource := f.ResourceType
	if resource == "" {
		resource = name
	}
	title := f.Title
	if title == "" {
		title = f.Name
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", d.label, title)
	if title != resource {
		fmt.Fprintf(&b, " (resource type %s)", resource)
	}
	if f.Description != "" {
		fmt.Fprintf(&b, " — %s", firstSentence(f.Description, 300))
	}
	if f.Profile != "" {
		fmt.Fprintf(&b, "\nCanonical URL: %s", f.Profile)
	}
	b.WriteString("\nElements (type, cardinality; MS = must support):\n")

	paths := make([]string, 0, len(f.Elements))
	for p := range f.Elements {
		if !isFHIRBoilerplate(p) {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	for _, p := range paths {
		e := f.Elements[p]
		fmt.Fprintf(&b, "%s (%s, %s", p, e.DataType, e.Cardinality)
		if e.MustSupport {
			b.WriteString(", MS")
		}
		if e.IsModifier {
			b.WriteString(", modifier")
		}
		b.WriteString(")")
		if e.Name != "" {
			fmt.Fprintf(&b, " — %s", firstSentence(e.Name, 120))
		}
		if e.ValueSet != "" {
			fmt.Fprintf(&b, " [binding %s %s]", e.BindingStrength, e.ValueSet)
		}
		b.WriteString("\n")
	}
	ref := name
	if f.Name != "" {
		ref = f.Name
	}
	return []SchemaDigestDoc{{Ref: ref, Text: b.String()}}, nil
}

// isFHIRBoilerplate reports inherited housekeeping elements (Patient.meta,
// Patient.contact.extension, ...).
func isFHIRBoilerplate(path string) bool {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return false
	}
	leaf := parts[len(parts)-1]
	if len(parts) == 2 {
		return fhirRootBoilerplate[leaf]
	}
	return fhirAnyDepthBoilerplate[leaf]
}
