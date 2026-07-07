// services/executors/transform/cda_csv_templates.go
// OOB CSV column definitions for cda.section_to_csv — one CSVSectionTemplate
// per supported CDA section, each producing a flat clinical-data-model view
// (NOT FHIR) of that section's entries: one CSV row per entry.
//
// Column paths deliberately reuse the SAME CDA path grammar (dot-segments,
// [key=value] bracket predicates, [*] wildcards) already established by
// services/cda_fhir/declarative_oob_rules.go's MappingRow.SourcePath values —
// resolved the same way, via executors.ResolveCDAPath against one entry map.
// Paths below were derived directly from the existing OOB FHIR mapping
// rules' own SourcePath choices (not invented independently), so the two
// stay aligned on what CDA data is clinically meaningful per section; see
// declarative_oob_rules.go's MedicationMappingRules/AllergyMappingRules/etc.
// for the FHIR-side counterpart of each field below.
//
// V1 scope: only entry-level-direct paths (no nested Scope-then-SourcePath
// two-tier resolution for recorder/asserter/performer sub-resources, reaction
// manifestation observations, or Social History's nested SDOH question/answer
// tiers) — deliberately, to ship a correct, useful first cut without
// unbounded scope creep. Each column path is resolved independently and
// missing/absent data simply produces an empty cell, never an error.
package transform

import "sort"

// CSVColumn is one output column: a CSV header name paired with a CDA path
// (relative to one section entry) to resolve its value from.
type CSVColumn struct {
	Name string
	Path string
}

// CSVSectionTemplate is the OOB column list for one CDA section.
type CSVSectionTemplate struct {
	SectionKey string
	Columns    []CSVColumn
}

// cdaCSVSectionTemplates is the OOB registry, keyed by section key — the CSV
// analogue of declarativeSectionRuleGroupsCache in
// services/cda_fhir/declarative_document_mapper.go.
var cdaCSVSectionTemplates = map[string]CSVSectionTemplate{
	"medications": {
		SectionKey: "medications",
		Columns: []CSVColumn{
			{Name: "MedicationName", Path: "consumable.manufacturedProduct.manufacturedMaterial.code.displayName"},
			{Name: "MedicationCode", Path: "consumable.manufacturedProduct.manufacturedMaterial.code.code"},
			{Name: "CodeSystem", Path: "consumable.manufacturedProduct.manufacturedMaterial.code.codeSystem"},
			{Name: "Status", Path: "statusCode"},
			{Name: "Intent", Path: "moodCode"}, // INT=order, EVN=statement — see medicationRequestRule/medicationStatementRule's EntryMatch split
			{Name: "StartDate", Path: "effectiveTime.low.value"},
			{Name: "EndDate", Path: "effectiveTime.high.value"},
			{Name: "Route", Path: "routeCode.displayName"},
			{Name: "Dose", Path: "doseQuantity.value"},
			{Name: "DoseUnit", Path: "doseQuantity.unit"},
			{Name: "Sig", Path: "entryRelationships[typeCode=COMP].entry.text"},
			{Name: "Note", Path: "entryRelationships[typeCode=COMP].entry.text"},
		},
	},
	"allergiesAndIntolerances": {
		SectionKey: "allergiesAndIntolerances",
		Columns: []CSVColumn{
			{Name: "Allergen", Path: "entryRelationships[typeCode=SUBJ].entry.participants[typeCode=CSM].participantRole.playingEntity.code.displayName"},
			{Name: "AllergenCode", Path: "entryRelationships[typeCode=SUBJ].entry.participants[typeCode=CSM].participantRole.playingEntity.code.code"},
			{Name: "AllergyType", Path: "entryRelationships[typeCode=SUBJ].entry.value.code"},
			// Status/OnsetDate read from the OUTER Concern Act directly (no
			// entryRelationships[typeCode=SUBJ].entry prefix) — verified against
			// AllergyMappingRules' own clinicalStatus/onsetDateTime rows
			// (declarative_oob_rules.go), which have no Scope at all, unlike
			// Problems' equivalent fields which genuinely DO live on the nested
			// SUBJ observation (problemObsScope). Conflating the two during
			// authoring was the bug; these are the only two of nine sections'
			// worth of columns where that happened (verified every other
			// section's columns against their real MappingRow SourcePath/Scope).
			{Name: "Status", Path: "statusCode"},
			{Name: "OnsetDate", Path: "effectiveTime.low.value"},
		},
	},
	"problems": {
		SectionKey: "problems",
		Columns: []CSVColumn{
			{Name: "ProblemName", Path: "entryRelationships[typeCode=SUBJ].entry.value.displayName"},
			{Name: "ProblemCode", Path: "entryRelationships[typeCode=SUBJ].entry.value.code"},
			{Name: "ClinicalStatus", Path: "entryRelationships[typeCode=SUBJ].entry.statusCode"},
			{Name: "OnsetDate", Path: "entryRelationships[typeCode=SUBJ].entry.effectiveTime.low.value"},
			{Name: "AbatementDate", Path: "entryRelationships[typeCode=SUBJ].entry.effectiveTime.high.value"},
		},
	},
	"vitalSigns": {
		SectionKey: "vitalSigns",
		Columns: []CSVColumn{
			{Name: "VitalSignType", Path: "code.displayName"},
			{Name: "LOINCCode", Path: "code.code"},
			{Name: "Value", Path: "value.quantity.value"},
			{Name: "Unit", Path: "value.quantity.unit"},
			{Name: "Date", Path: "effectiveTime.value.value"},
			{Name: "Status", Path: "statusCode"},
			{Name: "Interpretation", Path: "interpretationCode.displayName"},
		},
	},
	"immunizations": {
		SectionKey: "immunizations",
		Columns: []CSVColumn{
			{Name: "VaccineName", Path: "consumable.manufacturedProduct.manufacturedMaterial.code.displayName"},
			{Name: "VaccineCode", Path: "consumable.manufacturedProduct.manufacturedMaterial.code.code"},
			{Name: "Status", Path: "statusCode"},
			{Name: "Date", Path: "effectiveTime.value.value"},
			{Name: "Route", Path: "routeCode.displayName"},
			{Name: "Dose", Path: "doseQuantity.value"},
			{Name: "DoseUnit", Path: "doseQuantity.unit"},
			{Name: "LotNumber", Path: "consumable.manufacturedProduct.manufacturedMaterial.lotNumberText"},
			{Name: "Manufacturer", Path: "consumable.manufacturedProduct.manufacturerOrganization.names[0]"},
		},
	},
	"procedures": {
		SectionKey: "procedures",
		Columns: []CSVColumn{
			{Name: "ProcedureName", Path: "code.displayName"},
			{Name: "ProcedureCode", Path: "code.code"},
			{Name: "Status", Path: "statusCode"},
			{Name: "PerformedDate", Path: "effectiveTime.value.value"},
			{Name: "PerformedDateStart", Path: "effectiveTime.low.value"},
			{Name: "PerformedDateEnd", Path: "effectiveTime.high.value"},
			{Name: "BodySite", Path: "targetSiteCode.displayName"},
		},
	},
	"encounters": {
		SectionKey: "encounters",
		Columns: []CSVColumn{
			{Name: "Status", Path: "statusCode"},
			{Name: "EncounterType", Path: "code.displayName"},
			{Name: "EncounterCode", Path: "code.code"},
			{Name: "PeriodStart", Path: "effectiveTime.low.value"},
			{Name: "PeriodEnd", Path: "effectiveTime.high.value"},
			{Name: "Location", Path: "entryRelationships[typeCode=COMP].entry.participants[typeCode=LOC].participantRole.playingEntity.names[0].family"},
		},
	},
	"socialHistory": {
		SectionKey: "socialHistory",
		Columns: []CSVColumn{
			{Name: "ObservationType", Path: "code.displayName"},
			{Name: "LOINCCode", Path: "code.code"},
			{Name: "Value", Path: "value.code.displayName"},
			{Name: "Status", Path: "statusCode"},
			{Name: "Date", Path: "effectiveTime.value.value"},
		},
	},
	"results": {
		SectionKey: "results",
		Columns: []CSVColumn{
			{Name: "TestName", Path: "code.displayName"},
			{Name: "LOINCCode", Path: "code.code"},
			{Name: "Value", Path: "value.quantity.value"},
			{Name: "Unit", Path: "value.quantity.unit"},
			{Name: "Date", Path: "effectiveTime.value.value"},
			{Name: "Status", Path: "statusCode"},
			{Name: "Interpretation", Path: "interpretationCode.displayName"},
			{Name: "ReferenceRange", Path: "referenceRangeText"},
		},
	},
}

// SupportedCDACSVSections returns the sorted list of section keys with an
// OOB CSV template — used by the executor's default "export every section
// we have a template for" behavior and by the Documentation tab.
func SupportedCDACSVSections() []string {
	keys := make([]string, 0, len(cdaCSVSectionTemplates))
	for k := range cdaCSVSectionTemplates {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
