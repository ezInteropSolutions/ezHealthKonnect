// cda/builder/requirements_catalog.go
//
// GetDocumentTypeRequirements is the single combined catalog the guided
// step-config UIs (cda.map_to_canonical, cda.build) use to know what a
// given document type actually requires — every SHALL/SHOULD/MAY section
// (doc-type-specific, via CDASchemaLoader.GetDocumentTypeSections — the
// same source cda/validator/section_rules.go's buildSectionRules already
// uses for runtime conformance scoring, never a section's own global
// Conformance default) plus every SHALL/SHOULD header field for patient,
// author, and custodian (see header_requirements.go).
package builder

import (
	cdaSchema "ezhealthkonnect/cda"
	"ezhealthkonnect/uscdi"
)

// SectionRequirement is one section's doc-type-specific conformance level.
type SectionRequirement struct {
	Key         string `json:"key"`
	DisplayName string `json:"displayName"`
	LOINCCode   string `json:"loincCode"`
	Conformance string `json:"conformance"` // "SHALL" | "SHOULD" | "MAY" — for THIS document type

	// USCDIClasses is the real, ONC-verified USCDI v3 class set this section
	// maps to (cda/schemas/uscdi_v3.json via uscdi.USCDIVocabulary.
	// ClassesForSection), omitted when unmapped — never fabricated. Same
	// additive, honestly-absent-when-unknown principle Coverage Audit's own
	// CategoryStat.USCDIClasses already established.
	USCDIClasses []string `json:"uscdiClasses,omitempty"`
}

// DocumentTypeRequirements is the combined guided-configuration catalog for
// one document type.
type DocumentTypeRequirements struct {
	DocumentType string                             `json:"documentType"`
	Metadata     *cdaSchema.DocumentTypeMetadata     `json:"metadata,omitempty"`
	Sections     []SectionRequirement                `json:"sections"`
	HeaderGroups map[string][]HeaderFieldRequirement `json:"headerGroups"` // "patient" | "author" | "custodian" | "legalAuthenticator"
}

// headerRequirementGroups is the fixed set of header groups every document
// type requires — CCD and every other C-CDA document type share the same
// header structure (patient/author/custodian/legalAuthenticator), unlike
// sections which vary by document type.
var headerRequirementGroups = []string{"patient", "author", "custodian", "legalAuthenticator"}

// GetDocumentTypeRequirements builds the combined requirements catalog for
// documentType. vocabulary may be nil (graceful degradation — every
// USCDIClasses field simply comes back nil/omitted, same as Coverage Audit's
// own worker_pool.go handles a failed uscdi_v3.json load). Returns nil if
// documentType is unknown (mirrors CDASchemaLoader.GetDocumentTypeSections'
// own nil-for-unknown convention).
func GetDocumentTypeRequirements(loader *cdaSchema.CDASchemaLoader, vocabulary *uscdi.USCDIVocabulary, documentType string) *DocumentTypeRequirements {
	sectionInfo := loader.GetDocumentTypeSections(documentType)
	if sectionInfo == nil {
		return nil
	}

	sections := make([]SectionRequirement, 0, len(sectionInfo.SHALL)+len(sectionInfo.SHOULD)+len(sectionInfo.MAY))
	appendSections := func(keys []string, conformance string) {
		for _, key := range keys {
			sec := loader.GetSection(key)
			if sec == nil {
				continue
			}
			sections = append(sections, SectionRequirement{
				Key: key, DisplayName: sec.DisplayName, LOINCCode: sec.LOINCCode, Conformance: conformance,
				USCDIClasses: vocabulary.ClassesForSection(key),
			})
		}
	}
	appendSections(sectionInfo.SHALL, "SHALL")
	appendSections(sectionInfo.SHOULD, "SHOULD")
	appendSections(sectionInfo.MAY, "MAY")

	headerGroups := make(map[string][]HeaderFieldRequirement, len(headerRequirementGroups))
	for _, group := range headerRequirementGroups {
		headerGroups[group] = HeaderRequirementsCatalog(loader, vocabulary, group)
	}

	return &DocumentTypeRequirements{
		DocumentType: documentType,
		Metadata:     loader.GetDocumentTypeMetadata(documentType),
		Sections:     sections,
		HeaderGroups: headerGroups,
	}
}
