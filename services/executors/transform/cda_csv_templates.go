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
//
// Path should point at the coded-concept MAP itself (e.g.
// "...manufacturedMaterial.code", not "...code.displayName") whenever the
// target is a human-readable name/type — cdaMapToCSVString then picks the
// best available text (displayName, else the narrative-resolved
// originalText, else the bare code) instead of hard-committing to
// displayName alone, which real Epic-style CCDs routinely leave empty in
// favor of a <originalText><reference value="#id"/></originalText> anchor
// into the section's narrative (already resolved by
// cda/document/section_parser.go's resolveEntryRefs — this template layer
// just wasn't reading where that resolution landed).
type CSVColumn struct {
	Name string
	Path string
	// FallbackPaths are tried in order, via executors.ResolveCDAFallback,
	// whenever Path resolves to nothing — for entry shapes real C-CDA data
	// uses inconsistently (e.g. a LOC participant sometimes wrapped in a COMP
	// entryRelationship, sometimes a direct child of the entry). Mirrors
	// declarative_oob_rules.go's Scope/ScopeFallbacks idiom for the FHIR side
	// of the same fields.
	FallbackPaths []string
	// ExposeCodeMetadata marks a column whose Path resolves to a WHOLE
	// CDACode map (every "Name"/"Type"/"Value"-style column in this file —
	// never a raw-string leaf like "...code.code", a quantity, or a plain
	// name string). When true, cda_section_to_csv_executor.go's
	// expandCodeMetadataColumns() inserts three sibling columns right after
	// this one, reusing the SAME Path with a suffix appended:
	// "<Name>CodeSystem" (path+".codeSystem"), "<Name>CodeSystemName"
	// (path+".codeSystemName"), "<Name>OriginalText" (path+".originalText") —
	// added so a coded field's system/narrative-reference-text is traceable
	// without guessing, without hand-authoring 3 extra columns per site.
	ExposeCodeMetadata bool
}

// CSVSectionTemplate is the OOB column list for one CDA section.
type CSVSectionTemplate struct {
	SectionKey string
	Columns    []CSVColumn
	// NarrativeOnly marks sections that are narrative-only per C-CDA in real
	// documents (no structured <entry> data at all: Assessment, Reason for
	// Referral, History of Present Illness, ...). Columns is ignored for
	// these; the executor emits one row holding the section's own narrative
	// text instead (see buildNarrativeRow in cda_section_to_csv_executor.go).
	NarrativeOnly bool
}

// cdaCSVSectionTemplates is the OOB registry, keyed by section key — the CSV
// analogue of declarativeSectionRuleGroupsCache in
// services/cda_fhir/declarative_document_mapper.go.
var cdaCSVSectionTemplates = map[string]CSVSectionTemplate{
	"medications": {
		SectionKey: "medications",
		Columns: []CSVColumn{
			{Name: "MedicationName", Path: "consumable.manufacturedProduct.manufacturedMaterial.code", ExposeCodeMetadata: true},
			{Name: "MedicationCode", Path: "consumable.manufacturedProduct.manufacturedMaterial.code.code"},
			{Name: "Status", Path: "statusCode"},
			{Name: "Intent", Path: "moodCode"}, // INT=order, EVN=statement — see medicationRequestRule/medicationStatementRule's EntryMatch split
			// C-CDA's base schema allows effectiveTime to be either an interval
			// (low/high) or a single-point TS -- verified against a real
			// document using the single-point form; FallbackPaths covers it
			// without a schema change (no EndDate equivalent: a single instant
			// has no distinct "end").
			{Name: "StartDate", Path: "effectiveTime.low.value", FallbackPaths: []string{"effectiveTime.value.value"}},
			{Name: "EndDate", Path: "effectiveTime.high.value"},
			{Name: "Route", Path: "routeCode", ExposeCodeMetadata: true},
			{Name: "Dose", Path: "doseQuantity.value"},
			{Name: "DoseUnit", Path: "doseQuantity.unit"},
			// ApproachSite/AdministrationUnit/MaxDose: real gap found auditing
			// several real CCDs -- present with genuine (non-nullFlavor) data,
			// parsed generically by entry_parser.go but never exposed here.
			{Name: "ApproachSite", Path: "approachSiteCode", ExposeCodeMetadata: true},
			{Name: "AdministrationUnit", Path: "administrationUnitCode", ExposeCodeMetadata: true},
			{Name: "MaxDose", Path: "maxDoseQuantity.numerator"},
			{Name: "MaxDosePeriod", Path: "maxDoseQuantity.denominator"},
			// Indication -- WHY this medication was prescribed. Generically
			// parsed already (every entryRelationship regardless of typeCode
			// lands in EntryRelationships[]) but never exposed as its own
			// column; RSON ("reason") is the C-CDA typeCode for this relationship.
			{Name: "Indication", Path: "entryRelationships[typeCode=RSON].entry.value.code", ExposeCodeMetadata: true},
			// Sig and Note used to share the exact same path (any COMP
			// entryRelationship's text) and so always duplicated each other.
			// Scoped by templateId to the distinct C-CDA templates the IG
			// actually uses for each — see medicationFreeTextSigTemplateID /
			// medicationInstructionV2TemplateID / commentActivityTemplateID in
			// declarative_oob_rules.go, whose medicationCommonRows() rows this
			// mirrors. FallbackPaths added after auditing several real CCDs:
			// Instruction (V2) (.4.20, via a SUBJ/inversionInd=true
			// entryRelationship) is actually the MORE common of the two Sig
			// representations across real documents -- Free Text Sig (.4.147)
			// is not universally present.
			// Note used to be its own column here, but it read the exact same
			// Comment Activity template the new universal Comment column
			// (cda_section_to_csv_executor.go's universalEntryColumns) now
			// covers for every section -- removed to avoid a duplicate.
			{
				Name:          "Sig",
				Path:          "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147].text",
				FallbackPaths: []string{"entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.20].text"},
			},
			// Precondition -- PRN administration criterion. Real-world
			// evidence: usually nullFlavor/NI (criterion not actually
			// specified), but exposed in case a document populates it.
			{Name: "Precondition", Path: "precondition", ExposeCodeMetadata: true},
		},
	},
	"allergiesAndIntolerances": {
		SectionKey: "allergiesAndIntolerances",
		Columns: []CSVColumn{
			{Name: "Allergen", Path: "entryRelationships[typeCode=SUBJ].entry.participants[typeCode=CSM].participantRole.playingEntity.code", ExposeCodeMetadata: true},
			{Name: "AllergenCode", Path: "entryRelationships[typeCode=SUBJ].entry.participants[typeCode=CSM].participantRole.playingEntity.code.code"},
			{Name: "AllergyType", Path: "entryRelationships[typeCode=SUBJ].entry.value.code", ExposeCodeMetadata: true},
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
			{Name: "OnsetDate", Path: "effectiveTime.low.value", FallbackPaths: []string{"effectiveTime.value.value"}},
			// Reaction/Severity/Criticality -- verified against C-CDA R2.1 IG
			// (CDAR2_IG_CCDA_CLINNOTES_R1_DSTU2.1_2015AUG_Vol2, Dec 2018 errata),
			// Section 3.103.1 "Allergy - Intolerance Observation (V2)", Table 491
			// constraints #14-16 (CONF:1098-9961..9964, 1098-7447..7449/15955,
			// 1098-32910..32913). All three are SIBLING entryRelationships
			// directly on the Allergy - Intolerance Observation (.4.7) itself --
			// Severity is NOT nested under Reaction as previously assumed:
			//   #14 typeCode=SUBJ  inversionInd=true -> Severity Observation (.4.8)
			//   #15 typeCode=MFST  inversionInd=true -> Reaction Observation (.4.9)
			//   #16 typeCode=SUBJ  inversionInd=true -> Criticality Observation (.4.145)
			// Reaction previously used typeCode=SUBJ (wrong -- spec says MFST,
			// "is manifestation of"); Severity previously nested inside Reaction's
			// own entryRelationships (wrong -- it's a sibling of Reaction, both
			// direct children of the Allergy-Intolerance Observation).
			// Criticality's templateId independently confirmed against a real
			// 99397 sample (LOINC 82606-5 "Criticality"); Reaction/Severity
			// templateIds/relationship shape confirmed via the PDF spec text
			// (every real sample audited this session left Reaction/Severity
			// nullFlavor/UNK, so real-data confirmation wasn't possible).
			{
				Name:               "Reaction",
				Path:               "entryRelationships[typeCode=MFST,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.9].value.code",
				ExposeCodeMetadata: true,
			},
			{
				Name:               "Severity",
				Path:               "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.8].value.code",
				ExposeCodeMetadata: true,
			},
			{
				Name:               "Criticality",
				Path:               "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.145].value.code",
				ExposeCodeMetadata: true,
			},
		},
	},
	"problems": {
		SectionKey: "problems",
		Columns: []CSVColumn{
			// value.displayName doesn't exist -- CDAValue (the polymorphic value
			// wrapper) has no top-level DisplayName field, only a nested
			// Code *CDACode (JSON key "code"). Was silently resolving to nothing
			// on every real document; fixed to match AllergyType's working
			// "value.code" pattern (whole-map resolution via cdaMapToCSVString).
			{Name: "ProblemName", Path: "entryRelationships[typeCode=SUBJ].entry.value.code", ExposeCodeMetadata: true},
			// ProblemCode must drill one level deeper than ProblemName --
			// "value.code" (no further suffix) resolves to the whole CDACode MAP,
			// same as ProblemName above; this column wants only the raw code
			// STRING, same as every other section's "*Code" column (e.g.
			// MedicationCode's "...code.code"). Pre-existing bug: this only
			// "worked" by accident via cdaMapToCSVString's displayName-then-code
			// fallback (problems never carry an inline displayName, so it fell
			// through to the raw code) -- broken by the originalText fallback
			// added above it, which now wins first for the same shared path.
			{Name: "ProblemCode", Path: "entryRelationships[typeCode=SUBJ].entry.value.code.code"},
			// ClinicalStatus here is the Problem Observation's own <statusCode>
			// -- RECORD completeness ("completed" = this entry is finalized),
			// NOT whether the condition itself is active/resolved. The TRUE
			// clinical state lives on a nested Problem Status observation
			// (templateId .4.6, code 33999-4 "Status") via entryRelationship
			// typeCode=REFR -- confirmed against real 99397 data (value
			// "Active", SNOMED 55561003). ConditionStatus reads that instead;
			// keeping ClinicalStatus as-is (not renaming) to avoid a breaking
			// column-name change.
			{Name: "ClinicalStatus", Path: "entryRelationships[typeCode=SUBJ].entry.statusCode"},
			{
				Name:               "ConditionStatus",
				Path:               "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=REFR].entry[templateId=2.16.840.1.113883.10.20.22.4.6].value.code",
				ExposeCodeMetadata: true,
			},
			{Name: "OnsetDate", Path: "entryRelationships[typeCode=SUBJ].entry.effectiveTime.low.value", FallbackPaths: []string{"entryRelationships[typeCode=SUBJ].entry.effectiveTime.value.value"}},
			{Name: "AbatementDate", Path: "entryRelationships[typeCode=SUBJ].entry.effectiveTime.high.value"},
			// AgeAtOnset -- Age Observation (.4.31), a PQ value (e.g. "45" +
			// unit "a" for years). Verified against the PDF spec (Section 3.4
			// "Age Observation", Table 226 "Contains: ... Problem Observation
			// (V3) (optional)"; Problem Observation V3's own constraint list,
			// CONF:1198-9059/9060/9069/15590): the wrapping entryRelationship
			// is @typeCode="SUBJ" @inversionInd="true" -- previously used
			// typeCode=REFR (wrong; REFR is what the SAME table uses one row
			// later for the Age Observation's own nested Prognosis
			// Observation, a different, unrelated relationship, which is what
			// caused the mix-up). Not yet confirmed against a real populated
			// example.
			{Name: "AgeAtOnset", Path: "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.31].value"},
		},
	},
	"vitalSigns": {
		SectionKey: "vitalSigns",
		Columns: []CSVColumn{
			{Name: "VitalSignType", Path: "code", ExposeCodeMetadata: true},
			{Name: "LOINCCode", Path: "code.code"},
			{Name: "Value", Path: "value.quantity.value"},
			{Name: "Unit", Path: "value.quantity.unit"},
			{Name: "Date", Path: "effectiveTime.value.value"},
			{Name: "Status", Path: "statusCode"},
			{Name: "Interpretation", Path: "interpretationCode", ExposeCodeMetadata: true},
		},
	},
	"immunizations": {
		SectionKey: "immunizations",
		Columns: []CSVColumn{
			{Name: "VaccineName", Path: "consumable.manufacturedProduct.manufacturedMaterial.code", ExposeCodeMetadata: true},
			{Name: "VaccineCode", Path: "consumable.manufacturedProduct.manufacturedMaterial.code.code"},
			{Name: "Status", Path: "statusCode"},
			{Name: "Date", Path: "effectiveTime.value.value"},
			{Name: "Route", Path: "routeCode", ExposeCodeMetadata: true},
			{Name: "Dose", Path: "doseQuantity.value"},
			{Name: "DoseUnit", Path: "doseQuantity.unit"},
			// ApproachSite -- the injection site (e.g. "Right Deltoid"). Real
			// gap: substanceAdministration-specific, shared with medications,
			// but genuinely populated on immunization entries too.
			{Name: "ApproachSite", Path: "approachSiteCode", ExposeCodeMetadata: true},
			{Name: "LotNumber", Path: "consumable.manufacturedProduct.manufacturedMaterial.lotNumberText"},
			{Name: "Manufacturer", Path: "consumable.manufacturedProduct.manufacturerOrganization.names[0]"},
			// Refused/RefusalReason -- Immunization Activity's base
			// substanceAdministration carries negationInd="true" when the
			// vaccine was declined rather than given; the reason (e.g. patient
			// refusal, medical contraindication) is the standard
			// entryRelationship typeCode=RSON pattern, same idiom as
			// medications' Indication column.
			{Name: "Refused", Path: "negationInd"},
			{Name: "RefusalReason", Path: "entryRelationships[typeCode=RSON].entry.value.code", ExposeCodeMetadata: true},
		},
	},
	"procedures": {
		SectionKey: "procedures",
		Columns: []CSVColumn{
			{Name: "ProcedureName", Path: "code", ExposeCodeMetadata: true},
			{Name: "ProcedureCode", Path: "code.code"},
			{Name: "Status", Path: "statusCode"},
			{Name: "PerformedDate", Path: "effectiveTime.value.value"},
			{Name: "PerformedDateStart", Path: "effectiveTime.low.value"},
			{Name: "PerformedDateEnd", Path: "effectiveTime.high.value"},
			{Name: "BodySite", Path: "targetSiteCode", ExposeCodeMetadata: true},
			// Indication -- WHY the procedure was performed (same
			// entryRelationship typeCode=RSON idiom as medications).
			{Name: "Indication", Path: "entryRelationships[typeCode=RSON].entry.value.code", ExposeCodeMetadata: true},
		},
	},
	"encounters": {
		SectionKey: "encounters",
		Columns: []CSVColumn{
			{Name: "Status", Path: "statusCode"},
			{Name: "EncounterType", Path: "code", ExposeCodeMetadata: true},
			{Name: "EncounterCode", Path: "code.code"},
			// Encounter Activity's effectiveTime can be a single-point TS instead
			// of an IVL_TS -- verified against a real "AWV" encounter that
			// records only <effectiveTime value="..."/>, no low/high; no
			// PeriodEnd equivalent since a single instant has no distinct end.
			{Name: "PeriodStart", Path: "effectiveTime.low.value", FallbackPaths: []string{"effectiveTime.value.value"}},
			{Name: "PeriodEnd", Path: "effectiveTime.high.value"},
			// Real Epic CCDs (verified against the 99397 sample) put the LOC
			// participant directly on the encounter entry, not wrapped in a COMP
			// entryRelationship -- the FallbackPaths entry mirrors
			// declarative_oob_rules.go's EncounterMappingRules Location row,
			// which already needed the same "wrapped, else direct" fallback.
			{
				Name:          "Location",
				Path:          "entryRelationships[typeCode=COMP].entry.participants[typeCode=LOC].participantRole.playingEntity.names[0].family",
				FallbackPaths: []string{"participants[typeCode=LOC].participantRole.playingEntity.names[0].family"},
			},
			// Diagnosis -- the encounter's associated problem/diagnosis, via
			// Encounter Diagnosis (.4.80) wrapping a nested Problem
			// Observation (V3, .4.4) SUBJ relationship. Verified against the
			// PDF spec (Section 3.22 "Encounter Activity (V3)", Table 268
			// constraint #11, CONF:1198-15492/15973, and Section 3.23
			// "Encounter Diagnosis (V3)", Table 271 constraint #5,
			// CONF:1198-14892/14893/14898): the OUTER entryRelationship
			// (Encounter Activity -> Encounter Diagnosis) has NO normative
			// @typeCode constraint in the IG (no CONF# is given for it,
			// unlike every other entryRelationship in this document) --
			// previously assumed typeCode=COMP, which the spec does not
			// actually require, so this uses a [*] wildcard and relies on
			// the .4.80 templateId (which IS normatively fixed) to find the
			// right entry instead. The INNER relationship (Encounter
			// Diagnosis -> Problem Observation) IS fixed to typeCode=SUBJ
			// per CONF:1198-14893 and Figure 144's example. Not yet
			// confirmed against a real populated structured example (the
			// 99397 sample's narrative table showed this only as plain
			// "Diagnosis: ..." text, not a structured entry).
			{
				Name:               "Diagnosis",
				Path:               "entryRelationships[*].entry[templateId=2.16.840.1.113883.10.20.22.4.80].entryRelationships[typeCode=SUBJ].entry.value.code",
				ExposeCodeMetadata: true,
			},
		},
	},
	"socialHistory": {
		SectionKey: "socialHistory",
		Columns: []CSVColumn{
			{Name: "ObservationType", Path: "code", ExposeCodeMetadata: true},
			{Name: "LOINCCode", Path: "code.code"},
			{Name: "Value", Path: "value.code", ExposeCodeMetadata: true},
			{Name: "Status", Path: "statusCode"},
			{Name: "Date", Path: "effectiveTime.value.value"},
		},
	},
	"results": {
		SectionKey: "results",
		Columns: []CSVColumn{
			{Name: "TestName", Path: "code", ExposeCodeMetadata: true},
			{Name: "LOINCCode", Path: "code.code"},
			{Name: "Value", Path: "value.quantity.value"},
			{Name: "Unit", Path: "value.quantity.unit"},
			{Name: "Date", Path: "effectiveTime.value.value"},
			{Name: "Status", Path: "statusCode"},
			{Name: "Interpretation", Path: "interpretationCode", ExposeCodeMetadata: true},
			{Name: "ReferenceRange", Path: "referenceRangeText"},
		},
	},

	// ── Additional sections (2026-07-12) ──────────────────────────────────
	// Added after auditing 5 real CCDs (Cerner, HITSP C32, and 3 Epic-family
	// documents) that between them use 20+ sections beyond the original 9 —
	// same generic Code/Value/Status/EffectiveTime shape already proven for
	// vitalSigns/socialHistory/results wherever a real example confirmed it
	// fits; a few (careTeam, familyHistory, payersInsurance) needed their own
	// shape because their clinically meaningful fields live on
	// performers/participants or a nested COMP entryRelationship, not on the
	// entry's own code/value.

	"functionalStatus": {
		SectionKey: "functionalStatus",
		Columns: []CSVColumn{
			{Name: "AssessmentType", Path: "code", ExposeCodeMetadata: true},
			{Name: "LOINCCode", Path: "code.code"},
			{Name: "Value", Path: "value.code", ExposeCodeMetadata: true},
			{Name: "Status", Path: "statusCode"},
			{Name: "Date", Path: "effectiveTime.value.value", FallbackPaths: []string{"effectiveTime.low.value"}},
		},
	},
	"mentalStatus": {
		SectionKey: "mentalStatus",
		Columns: []CSVColumn{
			{Name: "AssessmentType", Path: "code", ExposeCodeMetadata: true},
			{Name: "LOINCCode", Path: "code.code"},
			{Name: "Value", Path: "value.code", ExposeCodeMetadata: true},
			{Name: "Status", Path: "statusCode"},
			{Name: "Date", Path: "effectiveTime.value.value", FallbackPaths: []string{"effectiveTime.low.value"}},
		},
	},
	"familyHistory": {
		SectionKey: "familyHistory",
		Columns: []CSVColumn{
			// Family History Organizer: Relationship lives on the organizer's
			// own <subject><relatedSubject><code> (RelatedSubjectCode --
			// entry_parser.go's parseRelatedSubjectCode, added alongside this
			// section), a sibling of Components, not itself a component --
			// FallbackPaths isn't needed here since buildComponentRow already
			// falls back to the parent entry for any column the component
			// itself leaves empty, and no component has its own
			// relatedSubjectCode.
			{Name: "Relationship", Path: "relatedSubjectCode", ExposeCodeMetadata: true},
			{Name: "Condition", Path: "value.code", ExposeCodeMetadata: true},
			{Name: "ConditionCode", Path: "value.code.code"},
			{Name: "Status", Path: "statusCode"},
			{Name: "OnsetDate", Path: "effectiveTime.low.value", FallbackPaths: []string{"effectiveTime.value.value"}},
		},
	},
	"medicalEquipment": {
		SectionKey: "medicalEquipment",
		Columns: []CSVColumn{
			{Name: "EquipmentName", Path: "consumable.manufacturedProduct.manufacturedMaterial.code", ExposeCodeMetadata: true},
			{Name: "EquipmentCode", Path: "consumable.manufacturedProduct.manufacturedMaterial.code.code"},
			{Name: "Status", Path: "statusCode"},
			{Name: "StartDate", Path: "effectiveTime.low.value", FallbackPaths: []string{"effectiveTime.value.value"}},
			{Name: "EndDate", Path: "effectiveTime.high.value"},
		},
	},
	"healthConcerns": {
		SectionKey: "healthConcerns",
		Columns: []CSVColumn{
			{Name: "Concern", Path: "code", ExposeCodeMetadata: true},
			{Name: "ConcernCode", Path: "code.code"},
			{Name: "Status", Path: "statusCode"},
			{Name: "Date", Path: "effectiveTime.low.value", FallbackPaths: []string{"effectiveTime.value.value"}},
			// ConcernDetail -- the Health Concern Act (.4.132) itself is just a
			// wrapper; the actual concern is REFR-linked. typeCode=REFR
			// verified against the PDF spec (Section 3.35 "Health Concern Act
			// (V2)", Table 298, CONF:1198-31008/31158 etc. -- every one of the
			// act's dozen-plus contained observation types, e.g. Problem
			// Observation (V3) and Allergy - Intolerance Observation (V2),
			// uses the same @typeCode="REFR" entryRelationship). No templateId
			// filter is applied since the act can REFR-link to any of ~15
			// different observation types (Problem, Allergy, Social History,
			// Result, Mental Status, Assessment Scale, ...) -- this reads
			// whichever REFR entry appears first, a deliberate V1-scope
			// best-effort rather than enumerating every possible contained
			// type. Not yet confirmed against a real populated example.
			{Name: "ConcernDetail", Path: "entryRelationships[typeCode=REFR].entry.value.code", ExposeCodeMetadata: true},
		},
	},
	"goals": {
		SectionKey: "goals",
		Columns: []CSVColumn{
			{Name: "GoalDescription", Path: "code", ExposeCodeMetadata: true},
			{Name: "Target", Path: "value.code", ExposeCodeMetadata: true},
			{Name: "AchievementStatus", Path: "entryRelationships[typeCode=REFR].entry.value.code", ExposeCodeMetadata: true},
			{Name: "TargetDate", Path: "effectiveTime.high.value"},
			{Name: "Status", Path: "statusCode"},
		},
	},
	"advanceDirectives": {
		SectionKey: "advanceDirectives",
		Columns: []CSVColumn{
			{Name: "DirectiveType", Path: "code", ExposeCodeMetadata: true},
			{Name: "Value", Path: "value.code", ExposeCodeMetadata: true},
			{Name: "Status", Path: "statusCode"},
			{Name: "StartDate", Path: "effectiveTime.low.value"},
			{Name: "EndDate", Path: "effectiveTime.high.value"},
		},
	},
	"careTeam": {
		SectionKey: "careTeam",
		Columns: []CSVColumn{
			// Care Team Organizer -> component/act (one row per team member,
			// via the same organizer/component flattening vitals/family
			// history use) -> performer holds the actual member; the
			// component act's own <code> is a fixed "Care Team Information"
			// label, not useful here, so MemberRole/MemberName read the
			// performer instead.
			{Name: "MemberRole", Path: "performers[0].functionCode", ExposeCodeMetadata: true},
			// MemberName resolves to a CDAName (Prefix/Given/Family/Suffix), not a
			// CDACode -- ExposeCodeMetadata deliberately omitted here (no
			// codeSystem/originalText concept applies to a person's name).
			{Name: "MemberName", Path: "performers[0].assignedEntity.assignedPerson.names[0]"},
			{Name: "Status", Path: "statusCode"},
			{Name: "StartDate", Path: "performers[0].time.low.value"},
			{Name: "EndDate", Path: "performers[0].time.high.value"},
		},
	},
	"payersInsurance": {
		SectionKey: "payersInsurance",
		Columns: []CSVColumn{
			// Coverage Activity (outer, statusCode always "completed" per
			// C-CDA conformance) COMP-wraps exactly one Policy Activity,
			// which carries the real coverage type and PAYOR/GUARANTOR
			// performers plus the covered-party (COV) participant -- see
			// project_ccda_spec_reference.md's Payers structure notes.
			{Name: "PayerName", Path: "entryRelationships[typeCode=COMP].entry.performers[0].assignedEntity.representedOrganization.names[0]"},
			{Name: "CoverageType", Path: "entryRelationships[typeCode=COMP].entry.code", ExposeCodeMetadata: true},
			{Name: "CoverageTypeCode", Path: "entryRelationships[typeCode=COMP].entry.code.code"},
			{Name: "SubscriberName", Path: "entryRelationships[typeCode=COMP].entry.participants[typeCode=COV].participantRole.playingEntity.names[0]"},
			// SubscriberId -- PDF spec verification (Section 3.70 "Policy
			// Activity (V3)", Table 398, CONF:1198-8916/8917/8934/8935/8937)
			// revealed the Policy Activity carries TWO distinct participants:
			// COV "Covered Party" (.4.89, 1..1 SHALL -- whoever is covered,
			// typically the patient) and a SEPARATE HLD "Policyholder" (.4.90,
			// 0..1 SHOULD -- who actually holds/subscribes to the policy,
			// e.g. a parent or spouse). "Subscriber" maps to HLD, not COV as
			// previously assumed. HLD is preferred (the semantically correct
			// source); COV is kept as a fallback for the common
			// patient-is-subscriber case (COV/code="SELF") where no separate
			// HLD participant is populated. Re-checked against the real 99397
			// sample after this fix: it has no HLD block at all (COV's own
			// code="SELF", i.e. patient-is-subscriber) AND its COV
			// participantRole.id carries nullFlavor="MSK" (deliberately
			// masked/de-identified test data) -- confirming this path is
			// structurally correct; it comes back empty on that one sample
			// purely because the source value itself is masked, not a path
			// bug. PolicyNumber = the
			// Policy Activity's own <id> (per project_ccda_spec_reference.md:
			// "Coverage.identifier = Policy Activity.id"); GroupNumber =
			// entryRelationship typeCode=REFR's nested act <id> (per the same
			// notes: "Coverage.class[0].value = entryRelationship[REFR].act.id").
			// extension preferred (the human-meaningful local ID), root as
			// fallback -- same idiom as the universal EntryId column.
			{
				Name: "SubscriberId",
				Path: "entryRelationships[typeCode=COMP].entry.participants[typeCode=HLD].participantRole.id[0].extension",
				FallbackPaths: []string{
					"entryRelationships[typeCode=COMP].entry.participants[typeCode=HLD].participantRole.id[0].root",
					"entryRelationships[typeCode=COMP].entry.participants[typeCode=COV].participantRole.id[0].extension",
					"entryRelationships[typeCode=COMP].entry.participants[typeCode=COV].participantRole.id[0].root",
				},
			},
			{
				Name:          "PolicyNumber",
				Path:          "entryRelationships[typeCode=COMP].entry.id[0].extension",
				FallbackPaths: []string{"entryRelationships[typeCode=COMP].entry.id[0].root"},
			},
			{
				Name:          "GroupNumber",
				Path:          "entryRelationships[typeCode=COMP].entry.entryRelationships[typeCode=REFR].entry.id[0].extension",
				FallbackPaths: []string{"entryRelationships[typeCode=COMP].entry.entryRelationships[typeCode=REFR].entry.id[0].root"},
			},
			{Name: "PeriodStart", Path: "entryRelationships[typeCode=COMP].entry.participants[typeCode=COV].time.low.value"},
			{Name: "PeriodEnd", Path: "entryRelationships[typeCode=COMP].entry.participants[typeCode=COV].time.high.value"},
			{Name: "Status", Path: "statusCode"},
		},
	},
	"nutrition": {
		SectionKey: "nutrition",
		Columns: []CSVColumn{
			{Name: "ObservationType", Path: "code", ExposeCodeMetadata: true},
			{Name: "Value", Path: "value.code", ExposeCodeMetadata: true},
			{Name: "Status", Path: "statusCode"},
			{Name: "Date", Path: "effectiveTime.value.value", FallbackPaths: []string{"effectiveTime.low.value"}},
		},
	},

	// dischargeDiagnosis/admissionDiagnosis: same Problem Observation shape as
	// "problems" (a discharge/admission summary's own Concern Act + nested
	// SUBJ observation), just filed under a different section key/LOINC --
	// kept in sync with "problems"' own columns, including ConditionStatus/
	// AgeAtOnset.
	"dischargeDiagnosis": {
		SectionKey: "dischargeDiagnosis",
		Columns: []CSVColumn{
			{Name: "ProblemName", Path: "entryRelationships[typeCode=SUBJ].entry.value.code", ExposeCodeMetadata: true},
			{Name: "ProblemCode", Path: "entryRelationships[typeCode=SUBJ].entry.value.code.code"},
			{Name: "ClinicalStatus", Path: "entryRelationships[typeCode=SUBJ].entry.statusCode"},
			{
				Name:               "ConditionStatus",
				Path:               "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=REFR].entry[templateId=2.16.840.1.113883.10.20.22.4.6].value.code",
				ExposeCodeMetadata: true,
			},
			{Name: "OnsetDate", Path: "entryRelationships[typeCode=SUBJ].entry.effectiveTime.low.value", FallbackPaths: []string{"entryRelationships[typeCode=SUBJ].entry.effectiveTime.value.value"}},
			{Name: "AbatementDate", Path: "entryRelationships[typeCode=SUBJ].entry.effectiveTime.high.value"},
			{Name: "AgeAtOnset", Path: "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=REFR].entry[templateId=2.16.840.1.113883.10.20.22.4.31].value"},
		},
	},
	"admissionDiagnosis": {
		SectionKey: "admissionDiagnosis",
		Columns: []CSVColumn{
			{Name: "ProblemName", Path: "entryRelationships[typeCode=SUBJ].entry.value.code", ExposeCodeMetadata: true},
			{Name: "ProblemCode", Path: "entryRelationships[typeCode=SUBJ].entry.value.code.code"},
			{Name: "ClinicalStatus", Path: "entryRelationships[typeCode=SUBJ].entry.statusCode"},
			{
				Name:               "ConditionStatus",
				Path:               "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=REFR].entry[templateId=2.16.840.1.113883.10.20.22.4.6].value.code",
				ExposeCodeMetadata: true,
			},
			{Name: "OnsetDate", Path: "entryRelationships[typeCode=SUBJ].entry.effectiveTime.low.value", FallbackPaths: []string{"entryRelationships[typeCode=SUBJ].entry.effectiveTime.value.value"}},
			{Name: "AbatementDate", Path: "entryRelationships[typeCode=SUBJ].entry.effectiveTime.high.value"},
			{Name: "AgeAtOnset", Path: "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=REFR].entry[templateId=2.16.840.1.113883.10.20.22.4.31].value"},
		},
	},

	// dischargeMedications/admissionMedications: same Medication Activity
	// shape as "medications", filed under a different section key/LOINC --
	// kept in sync with "medications"' own columns, including
	// ApproachSite/AdministrationUnit/MaxDose/Indication/Sig/Precondition.
	"dischargeMedications": {
		SectionKey: "dischargeMedications",
		Columns: []CSVColumn{
			{Name: "MedicationName", Path: "consumable.manufacturedProduct.manufacturedMaterial.code", ExposeCodeMetadata: true},
			{Name: "MedicationCode", Path: "consumable.manufacturedProduct.manufacturedMaterial.code.code"},
			{Name: "Status", Path: "statusCode"},
			{Name: "Intent", Path: "moodCode"},
			{Name: "StartDate", Path: "effectiveTime.low.value", FallbackPaths: []string{"effectiveTime.value.value"}},
			{Name: "EndDate", Path: "effectiveTime.high.value"},
			{Name: "Route", Path: "routeCode", ExposeCodeMetadata: true},
			{Name: "Dose", Path: "doseQuantity.value"},
			{Name: "DoseUnit", Path: "doseQuantity.unit"},
			{Name: "ApproachSite", Path: "approachSiteCode", ExposeCodeMetadata: true},
			{Name: "AdministrationUnit", Path: "administrationUnitCode", ExposeCodeMetadata: true},
			{Name: "MaxDose", Path: "maxDoseQuantity.numerator"},
			{Name: "MaxDosePeriod", Path: "maxDoseQuantity.denominator"},
			{Name: "Indication", Path: "entryRelationships[typeCode=RSON].entry.value.code", ExposeCodeMetadata: true},
			{
				Name:          "Sig",
				Path:          "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147].text",
				FallbackPaths: []string{"entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.20].text"},
			},
			{Name: "Precondition", Path: "precondition", ExposeCodeMetadata: true},
		},
	},
	"admissionMedications": {
		SectionKey: "admissionMedications",
		Columns: []CSVColumn{
			{Name: "MedicationName", Path: "consumable.manufacturedProduct.manufacturedMaterial.code", ExposeCodeMetadata: true},
			{Name: "MedicationCode", Path: "consumable.manufacturedProduct.manufacturedMaterial.code.code"},
			{Name: "Status", Path: "statusCode"},
			{Name: "Intent", Path: "moodCode"},
			{Name: "StartDate", Path: "effectiveTime.low.value", FallbackPaths: []string{"effectiveTime.value.value"}},
			{Name: "EndDate", Path: "effectiveTime.high.value"},
			{Name: "Route", Path: "routeCode", ExposeCodeMetadata: true},
			{Name: "Dose", Path: "doseQuantity.value"},
			{Name: "DoseUnit", Path: "doseQuantity.unit"},
			{Name: "ApproachSite", Path: "approachSiteCode", ExposeCodeMetadata: true},
			{Name: "AdministrationUnit", Path: "administrationUnitCode", ExposeCodeMetadata: true},
			{Name: "MaxDose", Path: "maxDoseQuantity.numerator"},
			{Name: "MaxDosePeriod", Path: "maxDoseQuantity.denominator"},
			{Name: "Indication", Path: "entryRelationships[typeCode=RSON].entry.value.code", ExposeCodeMetadata: true},
			{
				Name:          "Sig",
				Path:          "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147].text",
				FallbackPaths: []string{"entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.20].text"},
			},
			{Name: "Precondition", Path: "precondition", ExposeCodeMetadata: true},
		},
	},

	// ── Narrative-only sections ─────────────────────────────────────────
	// Real C-CDA documents carry these as free text with no structured
	// <entry> at all (confirmed absent across all 5 audited documents) --
	// Columns is unused; buildNarrativeRow emits one SourceFile+NarrativeText
	// row from the section's own plain-text narrative instead.
	"chiefComplaint":          {SectionKey: "chiefComplaint", NarrativeOnly: true},
	"historyOfPresentIllness": {SectionKey: "historyOfPresentIllness", NarrativeOnly: true},
	"pastMedicalHistory":      {SectionKey: "pastMedicalHistory", NarrativeOnly: true},
	"reviewOfSystems":         {SectionKey: "reviewOfSystems", NarrativeOnly: true},
	"physicalExamination":     {SectionKey: "physicalExamination", NarrativeOnly: true},
	"dischargePhysical":       {SectionKey: "dischargePhysical", NarrativeOnly: true},
	"assessment":              {SectionKey: "assessment", NarrativeOnly: true},
	"reasonForReferral":       {SectionKey: "reasonForReferral", NarrativeOnly: true},
	"dischargeInstructions":   {SectionKey: "dischargeInstructions", NarrativeOnly: true},
	"hospitalCourse":          {SectionKey: "hospitalCourse", NarrativeOnly: true},
	// assessmentAndPlan and planOfTreatment are two DIFFERENT schema-registered
	// keys (LOINC 51847-2 vs 18776-5) that a real document's Plan of
	// Care/Treatment section can resolve to depending on which templateId it
	// carries -- both registered as narrative-only for the same reason: C-CDA's
	// Plan of Care Activity entry can be an Encounter/Observation/Act/
	// Procedure/SubstanceAdministration/Supply in INT moodCode, a heterogeneous
	// shape a flat CSVColumn table doesn't fit; the narrative table already
	// carries the same information in every real document audited.
	"assessmentAndPlan": {SectionKey: "assessmentAndPlan", NarrativeOnly: true},
	"planOfTreatment":   {SectionKey: "planOfTreatment", NarrativeOnly: true},
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
