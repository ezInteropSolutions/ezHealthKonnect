// services/parsers/cda/cda_path_resolver.go
// CDAPathResolver converts between the three path representations used in
// the smart field picker:
//
//   Short path : "allergiesAndIntolerances.medicationAllergyCode"
//   FHIR path  : "AllergyIntolerance.code.coding[0].code"
//   CDA XPath  : "//section[@code='48765-2']//observation/participant/playingEntity/code/@code"
//
// Short paths are always shown. FHIR path and CDA XPath are only surfaced when
// the user expands a result in the field picker.

package cdaparser

import (
	"fmt"
	"strings"

	cdaSchema "ezhealthkonnect/cda"
	"ezhealthkonnect/uscdi"
)

// CDAPathResolver is stateless — construct once, share freely.
type CDAPathResolver struct {
	schemaLoader *cdaSchema.CDASchemaLoader
	vocabulary   *uscdi.USCDIVocabulary
}

// NewCDAPathResolver creates a path resolver backed by the given schema and vocabulary.
func NewCDAPathResolver(loader *cdaSchema.CDASchemaLoader, vocab *uscdi.USCDIVocabulary) *CDAPathResolver {
	return &CDAPathResolver{schemaLoader: loader, vocabulary: vocab}
}

// ShortPath returns the short path for a section+field combination.
// Format: "sectionKey.fieldKey"  e.g. "allergiesAndIntolerances.medicationAllergyCode"
func (r *CDAPathResolver) ShortPath(sectionKey, fieldKey string) string {
	return fmt.Sprintf("%s.%s", sectionKey, fieldKey)
}

// FHIRPath returns the FHIR R4 path for a section+field combination by
// looking up the matching USCDI element's fhirPath.
// Returns "" if no USCDI mapping exists.
func (r *CDAPathResolver) FHIRPath(sectionKey, fieldKey string) string {
	section := r.schemaLoader.GetSection(sectionKey)
	if section == nil {
		return ""
	}
	field := r.schemaLoader.GetField(sectionKey, fieldKey)
	if field == nil {
		return ""
	}

	// Look up USCDI element by section's USCDI class and field's USCDI element label.
	el := r.vocabulary.GetElement(section.USCDIClass, field.USCDIElement)
	if el == nil {
		return ""
	}
	return el.FHIRPath
}

// XPath returns the absolute CDA XPath from the document root for a given
// section + field. The section LOINC code anchors the path:
//   //section[code/@code='<loincCode>']//<fieldXPath>
func (r *CDAPathResolver) XPath(sectionKey, fieldKey string) string {
	section := r.schemaLoader.GetSection(sectionKey)
	if section == nil {
		return ""
	}
	field := r.schemaLoader.GetField(sectionKey, fieldKey)
	if field == nil || field.XPath == "" {
		return ""
	}

	return fmt.Sprintf("//section[code/@code='%s']//%s", section.LOINCCode, field.XPath)
}

// ResolveShortPath parses a short path ("sectionKey.fieldKey") and returns
// all three representations. Returns empty strings for parts that cannot be resolved.
func (r *CDAPathResolver) ResolveShortPath(shortPath string) (fhirPath, xPath, conformance string) {
	parts := strings.SplitN(shortPath, ".", 2)
	if len(parts) != 2 {
		return
	}
	sectionKey, fieldKey := parts[0], parts[1]

	fhirPath = r.FHIRPath(sectionKey, fieldKey)
	xPath = r.XPath(sectionKey, fieldKey)

	field := r.schemaLoader.GetField(sectionKey, fieldKey)
	if field != nil {
		conformance = field.Conformance
	}
	return
}

// SearchResults builds SearchResult entries for all fields in a section,
// enriched with FHIR path, CDA XPath, and conformance. Used by the smart
// search API to populate expand-on-demand data.
func (r *CDAPathResolver) SearchResults(sectionKey string) []*uscdi.SearchResult {
	section := r.schemaLoader.GetSection(sectionKey)
	if section == nil {
		return nil
	}

	var results []*uscdi.SearchResult
	for _, field := range section.Fields {
		sr := &uscdi.SearchResult{
			ShortPath:   r.ShortPath(sectionKey, field.Key),
			Label:       field.USCDIElement,
			USCDIClass:  section.USCDIClass,
			ValueSet:    field.ValueSet,
			Conformance: field.Conformance,
			FHIRPath:    r.FHIRPath(sectionKey, field.Key),
			CDAXPath:    r.XPath(sectionKey, field.Key),
			Format:      "cda",
		}
		results = append(results, sr)
	}
	return results
}
