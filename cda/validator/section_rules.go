package validator

import cdaSchema "ezhealthkonnect/cda"

// sectionRule holds the effective conformance level for one section in a specific document type.
// Conformance is sourced from documentTypeSections (CCD, Discharge Summary, etc.) in
// ccda_2_1.json rather than from the global section conformance field, because the same
// section (e.g. allergiesAndIntolerances) is SHALL for a CCD but SHOULD for a Discharge Summary.
type sectionRule struct {
	SectionKey  string
	Conformance string // "SHALL" | "SHOULD" | "MAY"
	SchemaDef   *cdaSchema.CDASectionDef
}

// buildSectionRules returns the ordered set of section conformance rules for a document type.
// If the document type has an entry in documentTypeSections the SHALL/SHOULD/MAY lists
// from that entry are used. Otherwise each section's own Conformance field is used.
func buildSectionRules(loader *cdaSchema.CDASchemaLoader, docType string) []sectionRule {
	dtInfo := loader.GetDocumentTypeSections(docType)
	if dtInfo == nil {
		return fallbackSectionRules(loader)
	}

	seen := make(map[string]struct{})
	var rules []sectionRule

	addKeys := func(keys []string, conformance string) {
		for _, key := range keys {
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			sec := loader.GetSection(key)
			if sec == nil {
				// Section key in documentTypeSections but not in sections array — skip.
				continue
			}
			rules = append(rules, sectionRule{
				SectionKey:  key,
				Conformance: conformance,
				SchemaDef:   sec,
			})
		}
	}

	addKeys(dtInfo.SHALL, "SHALL")
	addKeys(dtInfo.SHOULD, "SHOULD")
	addKeys(dtInfo.MAY, "MAY")
	return rules
}

// fallbackSectionRules uses each section's own Conformance field when no
// document-type-specific precedence list is defined in the schema.
func fallbackSectionRules(loader *cdaSchema.CDASchemaLoader) []sectionRule {
	var rules []sectionRule
	for _, sec := range loader.AllSections() {
		if sec.IsHeader || sec.Conformance == "" {
			continue
		}
		rules = append(rules, sectionRule{
			SectionKey:  sec.Key,
			Conformance: sec.Conformance,
			SchemaDef:   sec,
		})
	}
	return rules
}
