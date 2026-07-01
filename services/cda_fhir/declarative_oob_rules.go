// services/cda_fhir/declarative_oob_rules.go
//
// Phase 3 (OOB Template Migration) of the CDA→FHIR Declarative Mapping
// Engine (see architecture/CDA_FHIR_MAPPING_ENGINE_SPRINT_PLAN.md, "Phase 3")
// — the canonical, single-source-of-truth MappingRule literals for the
// Allergies, Medications, and Conditions sections: the three sections this
// session's earlier Go-mapper fidelity fixes specifically targeted
// (negation→refuted, PIVL frequency→dosage.timing.repeat, RSON→reasonCode,
// SIG-text/Instruction(V2) resolution, severity from nested SUBJ
// observation) and the ones architecture/CDA_FHIR_MAPPING_INVENTORY.md rates
// HIGH confidence for these three sections.
//
// These functions are the ONE place this data is authored. The V154
// migration's seed JSON is hand-synced to match these literals exactly;
// declarative_oob_rules_test.go's round-trip test reads the migration file
// back and asserts deep-equality against these functions' output, so any
// drift between the two fails a test instead of silently shipping.
//
// Allergy reactions (MFST→manifestation/severity) WERE deferred in this
// slice's first pass — CollectAll's per-row independent index increment
// couldn't guarantee two parallel CollectAll rows (manifestation, severity)
// landed on the same reaction[i] when one transform produced nil for an item
// the other didn't. Fixed via MappingRow.Fields (declarative_schema.go) —
// see AllergyMappingRules' reaction row below and that field's doc comment.
//
// Still deliberately NOT ported (out of scope, not silently dropped):
//   - Allergy-level (not per-reaction) Severity Observation — the IG allows
//     this directly under the Allergy-Intolerance Observation but marks it
//     SHOULD NOT/deprecated, and FHIR R4 AllergyIntolerance has no top-level
//     severity field to put it in anyway (only reaction[].severity) — needs
//     its own design decision, deferred to a dedicated Phase 6-style session.
//   - MedicationStatement.effectiveDateTime fallback when effectivePeriod
//     resolves to nothing (Go tries CDATimeRangeToPeriod, then
//     CDATimeRangeToOnset into a DIFFERENT target field) — the schema has
//     no "try transform A into path A, else transform B into path B"
//     primitive yet, and no current test exercises the fallback. Only the
//     effectivePeriod path is ported.
package cdafhir

// medicationFreeTextSigTemplateID and medicationInstructionV2TemplateID are
// the same two C-CDA templateIds medication_mapper.go's applyDosageText
// discriminates by — see that function's doc comment for the IG page
// citations (p.590-593, p.624-625).
const (
	medicationFreeTextSigTemplateID   = "2.16.840.1.113883.10.20.22.4.147"
	medicationInstructionV2TemplateID = "2.16.840.1.113883.10.20.22.4.20"

	// commentActivityTemplateID is C-CDA's Comment Activity
	// (https://hl7.org/cda/us/ccda/StructureDefinition-CommentActivity.html,
	// code 48767-8) -- HL7's C-CDA on FHIR IG (CF-medications.html) maps
	// "/entryRelationship/act[code/@code='48767-8']/text" to .note
	// (Annotation), a field this codebase had no row for at all before a
	// user explicitly asked which IG-specified mappings were still missing.
	// Real evidence for typeCode="COMP" (not constrained by the IG's own
	// mapping row, which omits a typeCode predicate): this exact 99397 CCD
	// sample uses entryRelationship typeCode="COMP" for a Comment Activity
	// elsewhere in the same document (on an Encounter, not a medication --
	// no medication entry in this particular sample happens to carry one),
	// confirming this vendor's actual convention rather than guessing at it.
	commentActivityTemplateID = "2.16.840.1.113883.10.20.22.4.64"

	// noteActivityTemplateID is C-CDA's Note Activity
	// (https://hl7.org/cda/us/ccda/StructureDefinition-NoteActivity.html,
	// e.g. code 34109-9 "Note") -- see conditionFields()'s Condition.note
	// row doc comment for why this, not commentActivityTemplateID, is the
	// real-world source for Problem-section overview notes.
	noteActivityTemplateID = "2.16.840.1.113883.10.20.22.4.202"

	// assessmentScaleObservationTemplateID and
	// assessmentScaleSupportingObservationTemplateID are C-CDA's SDOH
	// Assessment Scale Observation
	// (https://hl7.org/cda/us/ccda/StructureDefinition-AssessmentScaleObservation.html)
	// and Assessment Scale Supporting Observation
	// (https://hl7.org/cda/us/ccda/StructureDefinition-AssessmentScaleSupportingObservation.html).
	// Verified against the local CDAR2_IG_CCDA_CLINNOTES_R1_DSTU2.1 spec PDF
	// plus the current hl7.org/cda/us/ccda pages (the 2018 PDF only covers
	// Social History Observation (V3) extension 2015-08-01, which has NO
	// child besides Author Participation; the SPRT-nested Assessment Scale
	// Observation is only valid under the NEWER 2022-06-01 extension, which
	// this 99397 sample's Social History entries also declare a templateId
	// for). entryRelationship typeCode="SPRT" (not COMP) attaches the
	// Assessment Scale Observation to its parent -- confirmed against spec,
	// not guessed from this one document's convention; the Assessment Scale
	// Observation's OWN children (the actual question/answer pairs) attach
	// via typeCode="COMP", per spec. See SocialHistoryMappingRules' own doc
	// comment for the real-data evidence (14 of 24 Social History entries in
	// the 99397 sample use this 3-level nesting for HARK and Social
	// Connection/Isolation SDOH screening instruments).
	assessmentScaleObservationTemplateID           = "2.16.840.1.113883.10.20.22.4.69"
	assessmentScaleSupportingObservationTemplateID = "2.16.840.1.113883.10.20.22.4.86"

	// allergyReactionObservationTemplateID and allergySeverityObservationTemplateID
	// are the Reaction Observation (V2) and Severity Observation (V2)
	// templateIds — see allergy_mapper.go's reaction-loop doc comment and
	// architecture/CDA_FHIR_MAPPING_INVENTORY.md's allergy section ("MFST →
	// Reaction Obs", "Nested SEV (MFST→SUBJ) → severity"), both citing
	// CONF:1098-7447/7907/7449/15955 (Reaction Obs) and CONF:1098-19169
	// (Severity Obs's own code fixed to "SEV"). Cross-checked live against
	// ccda.online's rendered C-CDA 2.1 templates while designing the
	// CollectAll+Fields primitive these power: Reaction Observation is
	// [0..*] under the allergy observation (multiple reactions are normal);
	// Severity Observation is [0..1] PER reaction, nested one level deeper —
	// not a sibling of Reaction Observation. The IG also allows (but marks
	// SHOULD NOT / deprecated) a Severity Observation directly under the
	// Allergy-Intolerance Observation itself, as an "overall allergy
	// severity" rather than a per-reaction one — deliberately NOT ported
	// here: FHIR R4 AllergyIntolerance has no top-level severity field to
	// put it in (only reaction[].severity), so this needs its own design
	// decision, not a guessed-at fallback. Left for a dedicated Phase 6-style
	// session.
	allergyReactionObservationTemplateID = "2.16.840.1.113883.10.20.22.4.9"
	allergySeverityObservationTemplateID = "2.16.840.1.113883.10.20.22.4.8"
)

// NarrativeSectionDef describes a CDA section that carries clinical content
// exclusively in its narrative <text> block (no structured <entry> elements).
// The engine maps these to FHIR DocumentReference resources via a dedicated
// section-level pass in DeclarativeMapDocument (not the entry-dispatch loop).
// LoincCode and DisplayName are fallbacks — the CDA section's own LoincCode
// and Title win when present, which is nearly always true in real CCDs.
type NarrativeSectionDef struct {
	LoincCode   string
	DisplayName string
}

// DefaultNarrativeSectionDefs maps the 6 section keys confirmed as
// narrative-only in the real 4-file corpus to their LOINC fallback codes.
// US Core v9.0.0 + Argonaut consensus: DocumentReference is the right FHIR
// resource for narrative broader than a specific clinical resource type.
// Real evidence:
//   - assessment (51848-0): Epic 99397 + Ascension -- "Visit Diagnoses", narrative table only
//   - hospitalCourse (8648-8): Dignity Health
//   - dischargeInstructions (18776-5): Marshfield + Dignity Health
//   - reasonForReferral (42349-1): Marshfield + Dignity Health
//   - reasonForVisit (29299-5): Ascension
//   - clinicalNote (34109-9): Dignity Health
var DefaultNarrativeSectionDefs = map[string]NarrativeSectionDef{
	"assessment":            {LoincCode: "51848-0", DisplayName: "Evaluation note"},
	"hospitalCourse":        {LoincCode: "8648-8", DisplayName: "Hospital course Discharge note"},
	"dischargeInstructions": {LoincCode: "18776-5", DisplayName: "Plan of treatment note"},
	"reasonForReferral":     {LoincCode: "42349-1", DisplayName: "Reason for referral"},
	"reasonForVisit":        {LoincCode: "29299-5", DisplayName: "Reason for visit"},
	"clinicalNote":          {LoincCode: "34109-9", DisplayName: "Note"},
}

// AllergyMappingRules returns the OOB declarative rule for the
// allergiesAndIntolerances section, porting allergy_mapper.go's
// buildAllergyResource field-by-field. EntryMatch=="" — every entry in the
// section becomes one AllergyIntolerance.
func AllergyMappingRules() []MappingRule {
	return []MappingRule{
		{
			SectionKey:     "allergiesAndIntolerances",
			FHIRResource:   "AllergyIntolerance",
			PatientRefPath: []string{"patient"}, // allergy_mapper.go:35: r["patient"] = ref(patientRef)
			Fields: []MappingRow{
				{
					// allergy_mapper.go:39 — outer Concern Act statusCode.
					SourcePath:  "statusCode",
					Transform:   "allergy_status_to_fhir",
					TargetPath:  "clinicalStatus",
					Required:    true,
					Conformance: "SHALL",
				},
				{
					// allergy_mapper.go:42 — outer Concern Act effectiveTime.low.
					SourcePath: "effectiveTime",
					Transform:  "cda_timerange_to_onset",
					TargetPath: "onsetDateTime",
				},
				{
					// Deliberately NOT allergy_mapper.go:56-68's negation→refuted
					// parity port. Verified against the HL7 C-CDA on FHIR IG
					// (CF-allergies.md, github.com/HL7/ccda-on-fhir): the allergy
					// mapping table has NO verificationStatus row at all, for
					// negationInd=true or otherwise — the IG's only prescribed
					// effect of negation is on .code (the row below), via
					// ConceptMap-CF-NoKnownAllergies. Confirmed independently
					// against base FHIR: AllergyIntolerance verificationStatus
					// "refuted" is defined as disputing "a propensity for a
					// reaction to the identified substance"
					// (hl7.org/fhir/valueset-allergyintolerance-verification.html)
					// — it presupposes an identified substance, which the real
					// "No Known Allergies" idiom (negated, no CSM participant,
					// generic value code) never has. Go's "refuted" port was an
					// unsourced heuristic, not an IG requirement.
					LiteralValue: "confirmed",
					Transform:    "allergy_verification_status_to_fhir",
					TargetPath:   "verificationStatus",
				},
				{
					// allergy_mapper.go:71-73 — allergy type from the SUBJ
					// observation's own value code. ScopeFallbacks=[""] covers the
					// "flat structure, no concern-act wrapper" case
					// (allergy_mapper.go:48-51) — not exercised by any current
					// test, but a real, documented Go behavior.
					Scope:          "entryRelationships[typeCode=SUBJ].entry",
					ScopeFallbacks: []string{""},
					SourcePath:     "value.code.code",
					Transform:      "allergy_type_to_fhir",
					TargetPath:     "type",
				},
				{
					// IG-sourced (CF-allergies.md's two /participant rows), not an
					// allergy_mapper.go:76-97 parity port: that Go code's
					// CSM-else-value fallback had the right shape but the wrong
					// substance code for the "No Known Allergies" idiom — it used
					// the raw negated value as-is (e.g. "Allergy to substance"),
					// where the IG's own ConceptMap-CF-NoKnownAllergies requires
					// inverting it to the negated concept (e.g. "No known
					// allergy"). SourcePath=="" so the whole matched entry (not
					// one field) reaches the transform — the CSM-vs-negation
					// branch needs participants, value, AND negationInd together.
					// See declarativeAllergySubstanceOrNoKnownAllergy's doc comment.
					Scope:          "entryRelationships[typeCode=SUBJ].entry",
					ScopeFallbacks: []string{""},
					Transform:      "allergy_substance_or_no_known_allergy_to_fhir",
					TargetPath:     "code",
					Required:       true,
					Conformance:    "SHALL",
				},
				{
					// allergy_mapper.go:81 — category derived from the SAME CSM
					// substance's code system. Only fires when a CSM participant
					// actually exists, exactly like Go's category assignment
					// living inside the "cc != nil" branch alongside "code".
					Scope:          "entryRelationships[typeCode=SUBJ].entry.participants[typeCode=CSM]",
					ScopeFallbacks: []string{"participants[typeCode=CSM]"},
					SourcePath:     "participantRole.playingEntity.code.codeSystem",
					Transform:      "allergy_category_from_substance_system",
					TargetPath:     "category",
				},
				{
					// IG-sourced (CF-allergies.md's Criticality row), not an
					// allergy_mapper.go parity port -- confirmed via grep that
					// "criticality" had ZERO occurrences anywhere in
					// services/cda_fhir before this row: never ported by the
					// legacy Go mapper either, a genuine gap rather than a
					// deliberate omission. CDA Criticality Observation
					// (templateId .4.145, code 82606-5) nests one level deeper
					// than Reaction/Severity -- directly under the SUBJ
					// allergy-assertion observation, inverted (SUBJ,
					// inversionInd=true), identified by code rather than
					// templateId since that's what the IG's own XPath keys on.
					Scope: "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=82606-5]",
					ScopeFallbacks: []string{
						"entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=82606-5]",
					},
					SourcePath: "value.code.code",
					Transform:  "allergy_criticality_to_fhir",
					TargetPath: "criticality",
				},
				{
					// allergy_mapper.go:99-123 — every MFST relationship on
					// allergyObs becomes one reaction[] element carrying
					// manifestation (and severity, if present); see
					// MappingRow.Fields' doc comment for why this is one
					// CollectAll+Fields row, not two independent CollectAll
					// rows. ScopeFallbacks covers the same SUBJ-wrapper-absent
					// "flat structure" case as the rows above.
					Scope: "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=MFST,inversionInd=true].entry[templateId=" + allergyReactionObservationTemplateID + "]",
					ScopeFallbacks: []string{
						"entryRelationships[typeCode=MFST,inversionInd=true].entry[templateId=" + allergyReactionObservationTemplateID + "]",
					},
					CollectAll: true,
					TargetPath: "reaction",
					Fields: []MappingRow{
						{
							// allergy_mapper.go:106-112 — value preferred,
							// code as fallback; same shared idiom as
							// Medication's RSON and Condition's code.
							Transform:  "cda_value_or_code_to_codeable_concept",
							TargetPath: "manifestation[0]",
						},
						{
							// allergy_mapper.go:114-119 — [0..1] per reaction;
							// absent simply means no "severity" key on this
							// one reaction, not an error.
							Scope:      "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=" + allergySeverityObservationTemplateID + "]",
							SourcePath: "value.code.code",
							Transform:  "allergy_reaction_severity_to_fhir",
							TargetPath: "severity",
						},
					},
				},
			},
		},
	}
}

// MedicationMappingRules returns BOTH medication MappingRules in the order
// BuildResourcesForRules requires for moodCode-driven first-match-wins
// dispatch (medication_mapper.go:22-36): "moodCode=INT" → MedicationRequest
// must be evaluated before the catch-all → MedicationStatement rule.
func MedicationMappingRules() []MappingRule {
	return []MappingRule{
		medicationRequestRule(),
		medicationStatementRule(),
	}
}

// requesterFromAuthor and requesterFromPerformer build a full
// PractitionerRole (-> Practitioner + Organization) for
// MedicationRequest.requester via assignedEntityRoleRow, instead of the
// display-only {display: name} reference the row below them falls back to.
//
// Real gap found in production data: a 99397 CCD sample (Epic source) has,
// on its substanceAdministration's own <author>, a full assignedAuthor --
// NPI (<id root="2.16.840.1.113883.4.6" extension="...">), NUCC specialty
// code, work address, and telecom -- none of which a bare display string
// captures, even though representedOrganization is also present (an id at
// minimum), which is exactly what assignedEntityRoleRow's own
// EmitAsResourceRequiredPaths=["organization"] gate needs to keep the whole
// PractitionerRole rather than discard it.
//
// Tried in author-then-performer order: HL7's C-CDA on FHIR IG
// (CF-medications.html, Medication Activity mapping table) specifies
// "/author -> .requester (Provenance)" as THE source for this field --
// /performer is not listed as a requester source there at all. This is a
// deliberate reversal of the priority order this codebase originally
// inherited from medication_mapper.go ("performer, else author" -- see the
// final display-only row's own comment below, kept verbatim as a record of
// that prior, IG-incorrect order), found by checking the spec directly
// after a user asked which of the two is more reliable. Performer is kept
// as a second tier (better than the literal placeholder if it happens to be
// richer than an absent author) since the IG doesn't say performer is
// WRONG, only that it isn't the specified source.
//
// Each tier is a no-op when its own basePath doesn't resolve
// (assignedEntityRoleRow's nested Practitioner/Organization rows scope to
// it) or when EmitAsResourceRequiredPaths isn't met (no
// representedOrganization data at all) -- in either case applyRow leaves
// "requester" unset and the NEXT tier runs. requesterFromPerformer's
// SkipIfResourceHasAnyOf=["requester"] is what lets it act as author's
// fallback rather than overwriting an already-built author reference. The
// final display-only row (medicationRequestFields, below) carries the same
// skip so it only ever fires when NEITHER rich tier produced anything --
// i.e. when the source genuinely lacks the structured data to justify a
// full resource, not when this code merely failed to look for it.
func requesterFromAuthor() MappingRow {
	return assignedEntityRoleRow("authors[0].assignedAuthor", "requester")
}

func requesterFromPerformer() MappingRow {
	row := assignedEntityRoleRow("performers[0].assignedEntity", "requester")
	row.SkipIfResourceHasAnyOf = []string{"requester"}
	return row
}

// medicationRequestFields returns the field rows medicationRequestRule()
// (medications section) and PlanOfCareMappingRules()'s substanceAdministration
// branch both need — plan_of_care_mapper.go:73 calls the exact same
// buildMedicationRequestResource Go function MapMedications does, so the
// declarative port reuses the same row list rather than duplicating it under
// a second SectionKey's rule.
func medicationRequestFields() []MappingRow {
	fields := []MappingRow{
		{
			// medication_mapper.go:50.
			SourcePath:  "statusCode",
			Transform:   "medication_request_status_to_fhir",
			TargetPath:  "status",
			Required:    true,
			Conformance: "SHALL",
		},
		{
			// FHIR MedicationRequest.intent is 1..1 required and has no CDA
			// source concept of its own -- but medicationRequestRule()'s own
			// EntryMatch ("moodCode=INT") already guarantees every entry this
			// rule matches is an order, so "order" is a sound literal, not a
			// guess. Found via a 747-file real-world corpus run (35 files had
			// MedicationRequest.intent missing -- this rule never wrote it at
			// all, unlike the Go mapper's equivalent which never set it
			// either, so this also closes a pre-existing gap, not a
			// regression introduced by the declarative port).
			LiteralValue: "order",
			TargetPath:   "intent",
			Required:     true,
			Conformance:  "SHALL",
		},
		requesterFromAuthor(),
		requesterFromPerformer(),
		{
			// medication_mapper.go:262-280 (requesterReference) originally read
			// "performer, else author, else a literal placeholder display" --
			// reordered to "author, else performer, else literal" per HL7's
			// C-CDA on FHIR IG (CF-medications.html), which specifies /author as
			// THE source for .requester and doesn't list /performer at all (see
			// requesterFromAuthor's doc comment above). us-core-21 still
			// requires this field is never empty, so the LiteralValue fallback
			// stays. The 3-tier FallbackPaths+LiteralValue chain is exactly the
			// primitive named in declarative_engine.go's resolveRowSourceValue
			// doc comment.
			//
			// SkipIfResourceHasAnyOf: this is now the LAST of 3 requester
			// tiers (see requesterFromAuthor/requesterFromPerformer above,
			// which try a full PractitionerRole first) -- a no-op once either
			// of those already wrote "requester". Safe to keep Required=true
			// alongside the skip: applyRow's SkipIfResourceHasAnyOf check
			// (declarative_engine.go) runs and returns before the
			// Required-empty-value check ever executes, so an early skip here
			// can never produce a false "missing required field" error.
			SourcePath:             "authors[0].assignedAuthor.assignedPerson.names[0]",
			FallbackPaths:          []string{"performers[0].assignedEntity.assignedPerson.names[0]"},
			LiteralValue:           "Ordering Provider",
			Transform:              "cda_name_or_literal_to_display_ref",
			TargetPath:             "requester",
			Required:               true,
			Conformance:            "SHALL",
			SkipIfResourceHasAnyOf: []string{"requester"},
		},
		{
			// medication_mapper.go:66-68.
			SourcePath: "effectiveTime",
			Transform:  "cda_timerange_to_onset",
			TargetPath: "authoredOn",
		},
		{
			// MedicationRequest.dispenseRequest.quantity -- the dispense/refill
			// <supply> entryRelationship (REFR-typed, entryType="supply") is
			// already parsed onto CDAEntry.Quantity/RepeatNumber
			// (entry_parser.go's "supply" case) but no row read it: the
			// narrative table's own "Dispense Quantity"/"Refills" columns (real
			// corpus evidence -- EPIC-sourced encounter, every one of its 10
			// medication entries has a sibling supply act) confirm this is real,
			// present data, not a hypothetical field. MedicationStatement has no
			// dispenseRequest concept at all in FHIR, so these two rows are
			// request-only -- NOT added to medicationCommonRows, unlike every
			// other row in this function.
			Scope:      "entryRelationships[typeCode=REFR].entry[entryType=supply]",
			SourcePath: "quantity",
			Transform:  "cda_quantity_to_fhir",
			TargetPath: "dispenseRequest.quantity",
		},
		{
			// MedicationRequest.dispenseRequest.numberOfRepeatsAllowed.
			// parseRepeatNumber's own doc comment: a documented nullFlavor
			// (e.g. "OTH" -- this corpus's own first medication entry) lands in
			// this same string field verbatim, not as "" -- relying on
			// cda_decimal_string_to_number's existing strconv.ParseFloat failure
			// path (returns (nil, nil), so the row simply doesn't fire) instead
			// of a new transform, since that's already this engine's standard
			// "non-numeric -> omit" behavior, not a hack one-off for this field.
			Scope:      "entryRelationships[typeCode=REFR].entry[entryType=supply]",
			SourcePath: "repeatNumber",
			Transform:  "cda_decimal_string_to_number",
			TargetPath: "dispenseRequest.numberOfRepeatsAllowed",
		},
	}
	return append(fields, medicationCommonRows("dosageInstruction[0]")...)
}

func medicationRequestRule() MappingRule {
	return MappingRule{
		SectionKey:     "medications",
		FHIRResource:   "MedicationRequest",
		EntryMatch:     "moodCode=INT",
		PatientRefPath: []string{"subject"}, // medication_mapper.go:47: r["subject"] = ref(patientRef)
		Fields:         medicationRequestFields(),
	}
}

func medicationStatementRule() MappingRule {
	fields := []MappingRow{
		{
			// medication_mapper.go:108.
			SourcePath:  "statusCode",
			Transform:   "medication_status_to_fhir",
			TargetPath:  "status",
			Required:    true,
			Conformance: "SHALL",
		},
		{
			// medication_mapper.go:121-122 — period preferred.
			SourcePath: "effectiveTime",
			Transform:  "cda_timerange_to_period",
			TargetPath: "effectivePeriod",
		},
		{
			// medication_mapper.go:123-124 — single-value-onset fallback when
			// effectiveTime has no high/low range, only a bare value (period
			// resolves to nothing in that case). Same SkipIfResourceHasAnyOf
			// "transform A into path A, else transform B into path B" shape
			// DeviceMappingRules' timingPeriod/timingDateTime pair already
			// proved out — this was deferred in the original slice only
			// because that pattern hadn't been established yet.
			SourcePath:             "effectiveTime",
			Transform:              "cda_timerange_to_onset",
			TargetPath:             "effectiveDateTime",
			SkipIfResourceHasAnyOf: []string{"effectivePeriod"},
		},
	}
	fields = append(fields, medicationCommonRows("dosage[0]")...)
	return MappingRule{
		SectionKey:     "medications",
		FHIRResource:   "MedicationStatement",
		EntryMatch:     "",
		PatientRefPath: []string{"subject"}, // medication_mapper.go:105: r["subject"] = ref(patientRef)
		Fields:         fields,
	}
}

// medicationCommonRows is shared verbatim by both Request and Statement —
// medication_mapper.go's two builder functions duplicate this exact logic
// (consumable code, route, doseQuantity, PIVL timing, free-text sig/
// instruction, RSON reasonCode) under two different dosage[] key names
// ("dosageInstruction" for Request, "dosage" for Statement); dosagePrefix
// parameterizes only that difference.
func medicationCommonRows(dosagePrefix string) []MappingRow {
	pivlPeriodScope := "effectiveTimes[xsiType=PIVL_TS].period"
	return []MappingRow{
		{
			// MedicationRequest.identifier / MedicationStatement.identifier --
			// the substanceAdministration's own <id>, parsed onto every
			// CDAEntry generically (entry_parser.go) but never read by any row
			// here. Real gap: a 99397 CCD sample (Epic source) has a distinct
			// <id root="..." extension="57530535"/> etc. on every one of its
			// 10 medication entries. No EmbedCDAIdentity -- unlike Encounter's
			// analogous "id[*]" row, nothing in this engine needs to
			// cross-match a Medication resource against another source by
			// shared id, so the plain identifier mapping is sufficient.
			Scope:      "id[*]",
			CollectAll: true,
			Transform:  "cda_ii_to_identifier",
			TargetPath: "identifier",
		},
		{
			// medication_mapper.go:56-63/111-118.
			SourcePath: "consumable.manufacturedProduct.manufacturedMaterial.code",
			Transform:  "cda_code_to_codeable_concept",
			TargetPath: "medicationCodeableConcept",
		},
		{
			// medication_mapper.go:72-76/129-133.
			SourcePath: "routeCode",
			Transform:  "cda_code_to_codeable_concept",
			TargetPath: dosagePrefix + ".route",
		},
		{
			// medication_mapper.go:77-83/134-140.
			SourcePath: "doseQuantity",
			Transform:  "cda_quantity_to_fhir",
			TargetPath: dosagePrefix + ".doseAndRate[0].doseQuantity",
		},
		{
			// medication_mapper.go:158-182 (applyDosageTiming) — frequency is
			// always literally 1 in the current Go logic (it hardcodes this; see
			// that function's own comment), gated on a PIVL_TS effectiveTime
			// actually carrying a <period> child. Scoping to ".period" (not just
			// the PIVL_TS entry) makes the period!=nil guard fall out of Scope
			// resolution for free — see cda_path_resolver.go's "xsiType"
			// predicate-key addition. float64, not int: this rule's canonical
			// form is the JSON a migration seeds (json.Number decodes to
			// float64), and declarative_oob_rules_migration_test.go's
			// round-trip equality check compares against exactly that.
			Scope:        pivlPeriodScope,
			LiteralValue: float64(1),
			TargetPath:   dosagePrefix + ".timing.repeat.frequency",
		},
		{
			// FHIR decimal requires a numeric JSON value -- the CDA <period
			// value="..."/> attribute is a string (e.g. ".5"), which fails
			// validation unless parsed.
			Scope:      pivlPeriodScope,
			SourcePath: "value",
			Transform:  "cda_decimal_string_to_number",
			TargetPath: dosagePrefix + ".timing.repeat.period",
		},
		{
			Scope:      pivlPeriodScope,
			SourcePath: "unit",
			TargetPath: dosagePrefix + ".timing.repeat.periodUnit",
		},
		{
			// medication_mapper.go:184-223 (applyDosageText), Free Text Sig arm.
			Scope:      "entryRelationships[typeCode=COMP].entry[templateId=" + medicationFreeTextSigTemplateID + "]",
			SourcePath: "text",
			TargetPath: dosagePrefix + ".text",
		},
		{
			// Same function, Instruction (V2) arm.
			Scope:      "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=" + medicationInstructionV2TemplateID + "]",
			SourcePath: "text",
			TargetPath: dosagePrefix + ".patientInstruction",
		},
		{
			// medication_mapper.go:225-251 (indicationReasonCodes) — collects
			// ALL RSON matches, not just the first; SourcePath=="" passes each
			// scoped indication entry whole into the shared
			// cda_value_or_code_to_codeable_concept transform, which replicates
			// Go's own "prefer value, fall back to code" branch internally.
			Scope:      "entryRelationships[typeCode=RSON].entry",
			Transform:  "cda_value_or_code_to_codeable_concept",
			CollectAll: true,
			TargetPath: "reasonCode",
		},
		{
			// MedicationRequest.note / MedicationStatement.note (Annotation) --
			// see commentActivityTemplateID's doc comment. Not present on any
			// medication entry in the one real sample available, so this is the
			// IG's specified mapping, not yet exercised against real comment
			// data; single-value (not CollectAll), matching the Free Text
			// Sig/Instruction rows above -- the IG's own table entry is
			// singular too, with no evidence multiple Comment Activities per
			// medication entry is a real pattern worth the extra complexity.
			Scope:      "entryRelationships[typeCode=COMP].entry[templateId=" + commentActivityTemplateID + "]",
			SourcePath: "text",
			TargetPath: "note[0].text",
		},
	}
}

// problemObsScope/problemObsScopeFallbacks are condition_mapper.go's
// findRelByTypeCode(SUBJ)→REFR→&entry three-tier fallback
// (condition_mapper.go:68-74), shared by every Condition row that reads from
// "the problem observation" rather than the outer concern act itself.
const problemObsScope = "entryRelationships[typeCode=SUBJ].entry"

var problemObsScopeFallbacks = []string{"entryRelationships[typeCode=REFR].entry", ""}

// ProblemsMappingRules returns the OOB declarative rule for the "problems"
// section (category="problem-list-item").
func ProblemsMappingRules() []MappingRule {
	return []MappingRule{conditionRule("problems", "problem-list-item")}
}

// HealthConcernsMappingRules returns the OOB declarative rule for the
// "healthConcerns" section (category="health-concern") — the latent bug
// this session fixed (Phase 0 finding #9: category was hardcoded regardless
// of section) is what makes this a SEPARATE rule from ProblemsMappingRules,
// not the same rule reused with a runtime parameter: each rule's category
// row carries its own fixed LiteralValue, so there is no parameter to get
// wrong.
// HealthConcernsMappingRules returns TWO rules for the healthConcerns section,
// ordered so the engine's claiming mechanism routes entries correctly:
//
//  1. healthConcernAssessmentScaleRule — runs FIRST, claims Health Concern Act
//     entries whose REFR-linked nested observation is an Assessment Scale
//     Observation (templateId .4.69, e.g. PHQ-9, PHQ-2, AUDIT-C). These are
//     quantitative scores that must be FHIR Observation, not Condition.
//     Real evidence: Ascension (PHQ-9 44261-6, PHQ-2 55758-7) and Epic 99397
//     (same two) — both produced wrong Condition resources before this rule.
//
//  2. conditionRule — runs SECOND, claims remaining entries (genuine clinical
//     problems referenced as health concerns, e.g. "Hypertension as a concern").
//
// The engine marks each entry as "claimed" after Rule 1 processes it, so
// Rule 2 never sees PHQ-9/PHQ-2 entries (avoiding double-emission).
func HealthConcernsMappingRules() []MappingRule {
	return []MappingRule{
		healthConcernAssessmentScaleRule(),
		conditionRule("healthConcerns", "health-concern"),
	}
}

// healthConcernAssessmentScaleRule maps Health Concern Acts (templateId .4.132)
// whose REFR-linked nested observation is an Assessment Scale Observation
// (.4.69) to FHIR Observation resources.
//
// All SourcePaths/Scopes are prefixed with "entryRelationships[typeCode=REFR].entry."
// to resolve from the OUTER Health Concern Act's JSON node to the INNER
// assessment scale observation's fields — the engine always maps from the
// top-level entry node (the act), not from a pre-scoped sub-node.
func healthConcernAssessmentScaleRule() MappingRule {
	// Prefix shared by every field that lives on the nested observation.
	const refr = "entryRelationships[typeCode=REFR].entry."
	return MappingRule{
		SectionKey:   "healthConcerns",
		FHIRResource: "Observation",
		// refrTemplateId is a new predicate (cda_path_resolver.go) that checks
		// whether any REFR-linked nested entry carries this templateId. Cannot
		// use the standard templateId= predicate here because the outer act
		// always carries .4.132 (Health Concern Act), not .4.69.
		EntryMatch:     "refrTemplateId=" + assessmentScaleObservationTemplateID,
		PatientRefPath: []string{"subject"},
		Fields: []MappingRow{
			{
				SourcePath: refr + "code",
				Transform:  "cda_code_to_codeable_concept",
				TargetPath: "code",
				Required:   true,
				Conformance: "SHALL",
			},
			{
				SourcePath: refr + "statusCode",
				Transform:  "observation_status_to_fhir",
				TargetPath: "status",
			},
			{
				SourcePath: refr + "effectiveTime",
				Transform:  "cda_timerange_to_onset",
				TargetPath: "effectiveDateTime",
			},
			{
				// "survey" is the US Core category for patient-reported outcome
				// measures (PHQ-9, PHQ-2, AUDIT-C, etc.). Same category used by
				// FunctionalStatus/MentalStatus assessment scale supporting
				// observations elsewhere in this file.
				LiteralValue: "survey",
				Transform:    "observation_category_to_fhir",
				TargetPath:   "category[0]",
			},
			{
				// Integer score (PHQ-9 = INT value "0").
				Scope:      refr + "value[type=INT]",
				SourcePath: "integer",
				TargetPath: "valueInteger",
			},
			{
				// Decimal score (some tools emit REAL).
				Scope:      refr + "value[type=REAL]",
				SourcePath: "real",
				TargetPath: "valueQuantity",
				Transform:  "cda_real_to_quantity",
			},
			{
				// Coded interpretation (CD/CE/CS) — e.g. "Minimal depression".
				Scope:          refr + "value[type=CD]",
				ScopeFallbacks: []string{refr + "value[type=CE]", refr + "value[type=CS]"},
				SourcePath:     "code",
				Transform:      "cda_code_to_codeable_concept",
				TargetPath:     "valueCodeableConcept",
			},
			{
				Scope:      refr + "interpretationCode",
				Transform:  "cda_code_to_codeable_concept",
				TargetPath: "interpretation[0]",
			},
		},
	}
}

func conditionRule(sectionKey, categoryCode string) MappingRule {
	return MappingRule{
		SectionKey:     sectionKey,
		FHIRResource:   "Condition",
		PatientRefPath: []string{"subject"}, // condition_mapper.go:63: r["subject"] = ref(patientRef)
		Fields:         conditionFields(categoryCode),
	}
}

// conditionFields is conditionRule's field list, extracted so
// EncounterMappingRules' nested-diagnosis row (an EmitAsResource Condition,
// not a top-level Condition rule) can reuse the exact same Problem-
// Observation field logic instead of a hand-copied duplicate — the source
// CDA shape is identical (a Problem Concern Act .4.80 wrapping a Problem
// Observation .4.4 via entryRelationship[typeCode=SUBJ]) whether that act
// sits at the top of a Problems-section entry or nested inside an
// Encounter's own entryRelationships. Every row's Scope here is relative to
// "the matched Concern Act," which is true in both callers: conditionRule's
// own matched section entry IS the Concern Act; EncounterMappingRules'
// EmitAsResource row builds against the same scoped node its parent
// CollectAll row resolved to (entryRelationships[typeCode=SUBJ].entry on the
// Encounter), which is also the Concern Act.
func conditionFields(categoryCode string) []MappingRow {
	return []MappingRow{
		{
				// condition_mapper.go:77-91 — value preferred, code as fallback;
				// shared with Medication's RSON via the same transform (see that
				// function's doc comment).
				Scope:          problemObsScope,
				ScopeFallbacks: problemObsScopeFallbacks,
				Transform:      "cda_value_or_code_to_codeable_concept",
				TargetPath:     "code",
				Required:       true,
				Conformance:    "SHALL",
			},
			{
				// condition_mapper.go:94.
				Scope:          problemObsScope,
				ScopeFallbacks: problemObsScopeFallbacks,
				SourcePath:     "statusCode",
				Transform:      "condition_status_to_fhir",
				TargetPath:     "clinicalStatus",
				Required:       true,
				Conformance:    "SHALL",
			},
			{
				// condition_mapper.go:96-110 — entry.NegationInd OR'd with
				// problemObs.NegationInd, exactly like Allergy's verification
				// status row (see that row's doc comment for why Scope=="").
				LiteralValue: "confirmed",
				Transform:    "condition_verification_status_to_fhir",
				TargetPath:   "verificationStatus",
				Condition: &RowCondition{
					WhenPath: "negationInd",
					WhenPaths: []string{
						"entryRelationships[typeCode=SUBJ].entry.negationInd",
						"entryRelationships[typeCode=REFR].entry.negationInd",
					},
					Equals:           "true",
					ThenLiteralValue: "refuted",
				},
			},
			{
				// condition_mapper.go:112-122 — severity from a nested SUBJ
				// relationship whose own observation is fixed-coded "SEV"
				// (Severity Obs V2, CONF:1098-19169) — the worked example that
				// justified adding the "code" predicate key (see
				// cda_path_resolver.go). Three candidates mirror problemObs's own
				// three-tier resolution (SUBJ/REFR/self) each followed by the
				// SEV-coded lookup one level deeper.
				Scope: "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ].entry[code=SEV]",
				ScopeFallbacks: []string{
					"entryRelationships[typeCode=REFR].entry.entryRelationships[typeCode=SUBJ].entry[code=SEV]",
					"entryRelationships[typeCode=SUBJ].entry[code=SEV]",
				},
				SourcePath: "value.code",
				Transform:  "cda_code_to_codeable_concept",
				TargetPath: "severity",
			},
			{
				// condition_mapper.go:124-126 (conditionCategoryCC) — purely a
				// function of which section this rule targets, not of any CDA
				// data; see HealthConcernsMappingRules' doc comment.
				LiteralValue: categoryCode,
				Transform:    "condition_category_to_fhir",
				TargetPath:   "category[0]",
			},
			{
				// condition_mapper.go:128-131.
				Scope:          problemObsScope,
				ScopeFallbacks: problemObsScopeFallbacks,
				SourcePath:     "effectiveTime",
				Transform:      "cda_timerange_to_onset",
				TargetPath:     "onsetDateTime",
			},
			{
				// condition_mapper.go:133-136.
				Scope:          problemObsScope,
				ScopeFallbacks: problemObsScopeFallbacks,
				SourcePath:     "effectiveTime.high",
				Transform:      "cda_time_to_fhir_datetime",
				TargetPath:     "abatementDateTime",
			},
			// Condition.recorder -- the Problem Observation's own <author>
			// (entry.Authors, parsed generically for every entry type by
			// entry_parser.go's per-entry author handling, distinct from
			// header.Authors -- see CDAEntry.Authors' own doc comment), never
			// read by any mapper or rule until now. Per the HL7 C-CDA on FHIR
			// IG's own Problems mapping page (build.fhir.org/ig/HL7/
			// ccda-on-fhir/CF-problems.html, verified via WebFetch
			// 2026-06-27): "/entryRelationship[@typeCode='SUBJ']/observation/
			// author" -> ".recorder ... .recorder should be authoritative
			// (latest) author if there are multiple" -- the nested
			// Problem Observation's author, not the outer Concern Act's.
			// problemObsScope drills to that exact level; "authors[0]" takes
			// the first (not necessarily latest -- no corpus evidence of
			// multiple authors on one Problem Observation to justify sorting
			// by <time>, the same simplification firstAuthorWithPerson
			// already makes for the document-level header.Authors case).
			//
			// assignedEntityRoleRow (not organizationLinkRow): there is no
			// pre-existing Practitioner with a known id to link to here
			// (unlike Author/LegalAuthenticator's own representedOrganization
			// rows) -- this IS the first and only source for this specific
			// recorder, so a fresh PractitionerRole+Practitioner+Organization
			// is emitted, the same shape CareTeam's/Encounter's own performer
			// rows already use for the identical "no known id to link to"
			// situation. Condition.recorder accepts PractitionerRole as well
			// as Practitioner (FHIR R4: Reference(Practitioner|
			// PractitionerRole|Patient|RelatedPerson)), so this is valid
			// without unwrapping to a bare Practitioner reference.
			// CDAAssignedAuthor has no Code field (unlike CDAAssignedEntity),
			// so assignedEntityRoleRow's qualification[0].code row simply
			// resolves to nothing here -- harmless, not an error.
			assignedEntityRoleRow(problemObsScope+".authors[0].assignedAuthor", "recorder"),
			// Fallback tier -- see barePractitionerRow's own doc comment for
			// the real gap this closes (all 5 Active Problems in the 99397
			// sample have an author with an NPI but no representedOrganization
			// at all, so the rich tier above never fires for any of them).
			func() MappingRow {
				row := barePractitionerRow(problemObsScope+".authors[0].assignedAuthor", "recorder")
				row.SkipIfResourceHasAnyOf = []string{"recorder"}
				return row
			}(),
			{
				// Condition.recordedDate -- HL7's C-CDA on FHIR IG (CF-problems.html):
				// "/author/time -> .recordedDate" (earliest, if more than one).
				// Real gap: never mapped at all before this, despite every one of
				// the 99397 sample's 5 Active Problems carrying a real
				// <author><time value="..."/></author> on its Problem Observation.
				Scope:          problemObsScope,
				ScopeFallbacks: problemObsScopeFallbacks,
				SourcePath:     "authors[0].time",
				Transform:      "cda_time_to_fhir_datetime",
				TargetPath:     "recordedDate",
			},
			{
				// Condition.note -- C-CDA's Note Activity (templateId
				// 2.16.840.1.113883.10.20.22.4.202, code 34109-9 "Note", e.g.
				// translation 68608-9 "Summary note"), attached to the Concern
				// Act (not the Problem Observation) via entryRelationship
				// typeCode=COMP, sibling to the SUBJ-typed Problem Observation
				// relationship -- so this Scope is relative to the matched
				// Concern Act directly, unlike every other row in this function.
				// Real gap: a 99397 CCD sample (Epic source) has a substantive
				// provider overview note on 4 of its 5 Active Problems (e.g.
				// "last labs looked good, refilled current dosing.") that was
				// never captured anywhere. HL7's C-CDA on FHIR IG's own
				// Comment Activity (templateId .4.64, code 48767-8) -> .note
				// row (verified for Medications, see commentActivityTemplateID)
				// documents the same general "comment-like act's text ->
				// Annotation" pattern; Note Activity is HL7's newer,
				// purpose-built template for exactly this kind of act-level
				// note and isn't separately enumerated in the IG's Problems
				// page, but is structurally identical -- a free-text act linked
				// by entryRelationship -- so the same mapping idiom applies.
				Scope:      "entryRelationships[typeCode=COMP].entry[templateId=" + noteActivityTemplateID + "]",
				SourcePath: "text",
				TargetPath: "note[0].text",
			},
		}
}

// =========================================================
// Vital Signs / Results (Laboratory) / Social History
// =========================================================
//
// All three sections dispatch to the SAME Go function
// (observation_mapper.go's MapObservations), differing only in the
// "category" string literal passed through — exactly the same
// one-mapper/different-LiteralValue shape ProblemsMappingRules/
// HealthConcernsMappingRules already established for Conditions, so these
// three rules share one observationFields() helper the same way
// conditionRule() is shared above.
//
// Deliberately NOT ported in this slice (out of scope, not silently
// dropped):
//   - Systolic+Diastolic -> combined BP panel (observation_mapper.go's
//     extractBloodPressurePanels/buildBloodPressureObservation). This isn't
//     a per-entry field mapping at all -- it COMBINES two sibling Components
//     into one resource, which this engine's one-rule-set-produces-zero-or-
//     more-resources-per-matched-entry model has no way to express without a
//     bespoke, vital-signs-only primitive. Confirmed unnecessary: the
//     already-registered, engine-agnostic assembly.rules.BPPanelSynthesisRule
//     (services/cda_fhir/assembly/rules/bp_panel_rule.go) re-pairs any two
//     standalone Systolic(8480-6)/Diastolic(8462-4) Observation resources
//     sharing a subject, AFTER mapping, regardless of which engine produced
//     them -- it's already "defense in depth" for entries that escape
//     mapper-level pairing per that file's own doc comment. See
//     declarative_oob_rules_test.go's
//     TestDeclarativeEngine_VitalSigns_BPPair_RecombinedByAssemblyLayer for
//     proof this rule's own un-paired output is correctly recombined by that
//     existing, separate mechanism -- not a gap, a deliberate division of
//     responsibility.
//   - Shell-entry substitution (nullFlavor outer code, substitute code/
//     value/effectiveTime from a COMP child) -- observation_mapper.go's own
//     doc comment cites this pattern as an SDOH/AUDIT-C-assessment idiom, a
//     section not yet ported (functionalStatus/mentalStatus/assessment
//     sections), not something architecture/CDA_FHIR_MAPPING_INVENTORY.md
//     cites evidence for in Vital Signs/Results/Social History specifically.
//     Also mechanically awkward to express with FallbackPaths as-is (that
//     primitive treats "path resolved to a non-empty STRING" as the
//     fallback trigger; here the trigger is "the entry's OWN already-
//     resolved CDACode struct has an empty .code AND a set .nullFlavor", a
//     content check on a resolved struct, not an unresolved-path check) --
//     revisit when a section with real evidence for this pattern is ported.
//   - interpretationCode -> Observation.interpretation -- RESOLVED
//     2026-06-22. Added CDAEntry.InterpretationCode + entry_parser.go's
//     direct-sibling parse (CONF:1198-7147, verified against the actual IG)
//     and fixed observation_mapper.go's interpretation logic to read it
//     instead of a COMP child's value -- a genuine bug fix in Go, not just
//     a declarative-only addition, matching real Kareo data's actual shape.
//     See this rule's "interpretationCode" row below.
//   - referenceRange / performer -- Go has zero implementation for either
//     (referenceRange is "registered but never called from the mapper" per
//     the inventory; performer is "NOT implemented" for both sections) --
//     nothing to port declaratively that Go itself doesn't already do.

// VitalSignsMappingRules returns the OOB declarative rule for the
// "vitalSigns" section (category="vital-signs").
func VitalSignsMappingRules() []MappingRule {
	return []MappingRule{observationRule("vitalSigns", "vital-signs")}
}

// ResultsMappingRules returns the OOB declarative rules for the "results"
// section (category="laboratory") and its "labResults" alias --
// typedSectionDispatchers in the now-deleted document_mapper.go mapped both
// keys to the same mapper/category; both must resolve identically here too.
func ResultsMappingRules() []MappingRule {
	return []MappingRule{
		observationRule("results", "laboratory"),
		observationRule("labResults", "laboratory"),
	}
}

// assessmentScaleSupportingObservationRow returns the innermost SDOH-style
// tier: each Assessment Scale Supporting Observation (templateId .4.86,
// attached via entryRelationship typeCode=COMP per spec -- see
// assessmentScaleSupportingObservationTemplateID's own doc comment) is the
// actual question/answer pair (e.g. "Within the last year, have you been
// afraid of your partner or ex-partner?" -> "No"), each becoming its own
// independent Observation referenced from the parent's hasMember[] via the
// CollectAll+EmitAsResource engine primitive (declarative_engine.go's
// applyCollectAllEmitAsResource) -- a variable number (1 to 6+ in the real
// 99397 sample) of these sit as COMP siblings under one Assessment Scale
// Observation, so a fixed single-row mapping can't reach them all.
//
// categoryCode is parameterized (not hardcoded "social-history") because the
// SAME .4.69/.4.86 nesting shape recurs verbatim under Functional Status in
// the 99397 sample (its "Alcohol Use"/PHQ-2 entries) -- see
// FunctionalStatusMappingRules' own doc comment -- where the correct
// Observation.category is "functional-status", not "social-history".
func assessmentScaleSupportingObservationRow(categoryCode string) MappingRow {
	fields := []MappingRow{
		{SourcePath: "code", Transform: "cda_code_to_codeable_concept", TargetPath: "code"},
		{SourcePath: "text", TargetPath: "code.text"},
		{SourcePath: "statusCode", Transform: "observation_status_to_fhir", TargetPath: "status"},
		{LiteralValue: categoryCode, Transform: "observation_category_to_fhir", TargetPath: "category[0]"},
		// Same PractitionerRole/Practitioner tiered idiom as
		// observationRule()'s own performer row (upgraded together for
		// consistency within the same section) -- real evidence: every one
		// of the 99397 sample's Assessment Scale Supporting Observations
		// carries a full <author> with NPI + org.
		assignedEntityRoleRow("authors[0].assignedAuthor", "performer[0]"),
		func() MappingRow {
			row := barePractitionerRow("authors[0].assignedAuthor", "performer[0]")
			row.SkipIfResourceHasAnyOf = []string{"performer"}
			return row
		}(),
	}
	fields = append(fields, observationValueXDispatchRows()...)

	return MappingRow{
		Scope:                        "entryRelationships[typeCode=COMP].entry[templateId=" + assessmentScaleSupportingObservationTemplateID + "]",
		CollectAll:                   true,
		EmitAsResource:               "Observation",
		EmitAsResourcePatientRefPath: []string{"subject"},
		TargetPath:                   "hasMember",
		Fields:                       fields,
	}
}

// sdohAssessmentScaleObservationRow returns the middle SDOH tier: the
// Assessment Scale Observation itself (templateId .4.69, e.g. "HARK
// questionnaire" or "Social Connection and Isolation Panel"), which carries
// the overall instrument's own code/status/effectiveTime and usually an
// interpretationCode (risk level) -- attached to the OUTER Social History
// Observation via entryRelationship typeCode=SPRT (verified against the
// local CDAR2_IG_CCDA_CLINNOTES PDF's base Social History Observation
// constraints, which have NO such child at all, plus the current
// hl7.org/cda/us/ccda page for the SDOH-extension (2022-06-01) version,
// which confirms SPRT specifically -- see assessmentScaleObservationTemplateID's
// own doc comment). Emitted as its own Observation (not folded into the
// shell), referenced from the shell's hasMember[0], with its OWN nested
// hasMember[] fan-out to the individual question/answer Observations below
// it (assessmentScaleSupportingObservationRow).
func sdohAssessmentScaleObservationRow() MappingRow {
	fields := []MappingRow{
		{SourcePath: "code", Transform: "cda_code_to_codeable_concept", TargetPath: "code"},
		{SourcePath: "text", TargetPath: "code.text"},
		{SourcePath: "statusCode", Transform: "observation_status_to_fhir", TargetPath: "status"},
		{SourcePath: "effectiveTime", Transform: "cda_timerange_to_onset", TargetPath: "effectiveDateTime"},
		{LiteralValue: "social-history", Transform: "observation_category_to_fhir", TargetPath: "category[0]"},
		{
			// Real evidence: every Assessment Scale Observation in the
			// 99397 sample carries an interpretationCode (e.g.
			// nullFlavor="OTH" + originalText "Not At Risk" + translation
			// "Low Risk") even when its OWN <value> is nullFlavor="UNK" --
			// the calculated risk level, not a raw score, is the real
			// summary finding here.
			SourcePath: "interpretationCode",
			Transform:  "cda_code_to_codeable_concept",
			TargetPath: "interpretation[0]",
		},
	}
	fields = append(fields, observationValueXDispatchRows()...)
	fields = append(fields, assessmentScaleSupportingObservationRow("social-history"))

	return MappingRow{
		Scope:                        "entryRelationships[typeCode=SPRT].entry[templateId=" + assessmentScaleObservationTemplateID + "]",
		EmitAsResource:               "Observation",
		EmitAsResourcePatientRefPath: []string{"subject"},
		TargetPath:                   "hasMember[0]",
		Fields:                       fields,
	}
}

// SocialHistoryMappingRules returns the OOB declarative rule for the
// "socialHistory" section (category="social-history") -- includes Smoking
// Status, which has no smoking-specific Go code at all (observation_mapper.go's
// generic path handles it; see that file's own doc comment).
//
// Real gap found auditing the 99397 sample's Social History section: 14 of
// its 24 entries are SDOH screening instruments (2x HARK domestic-violence
// questionnaire = 8 questions, Social Connection/Isolation panel = 6
// questions) using the spec-conformant SPRT->COMP nested shape described
// above -- observationRule()'s generic per-entry mapping only ever read the
// OUTER shell (templateId .4.38, generic code "8689-2 History of Social
// function", no value of its own), producing 14 identical, clinically
// useless Observations. sdohAssessmentScaleObservationRow appends the
// missing tiers as additional Fields on the SAME rule, reusing
// observationRule()'s shell mapping unchanged.
func SocialHistoryMappingRules() []MappingRule {
	rule := observationRule("socialHistory", "social-history")
	rule.Fields = append(rule.Fields, sdohAssessmentScaleObservationRow())
	return []MappingRule{rule}
}

// FunctionalStatusMappingRules returns the OOB declarative rule for the
// "functionalStatus" section (category="functional-status"). Phase 4 Slice D
// closes the gap declarativeObservationCategoryToFHIR's own doc comment
// used to flag: this section had no declarative rule at all (Go's
// MapDocument() covered it via mappers.MapObservations(e, p,
// "functional-status")).
//
// Real gap found auditing the Functional Status section of the 99397
// sample: 2 of its 14 entries ("Alcohol Use"/AUDIT-C and a PHQ-2 depression
// screen) are Assessment Scale Observations (templateId .4.69) sitting
// DIRECTLY as top-level section entries (unlike Social History, where this
// same template nests one level deeper, under a SPRT-linked shell that
// always carries a real code -- see SocialHistoryMappingRules' own doc
// comment). Both of this section's .4.69 entries carry code.nullFlavor="UNK"
// (non-conformant per the IG's own AssessmentScaleObservation
// StructureDefinition, which requires code+value both [1..1]) -- before this
// fix, observationRule()'s SkipIfCodeNullFlavor correctly discarded these
// code-less shells, but ALSO silently discarded their COMP-nested Assessment
// Scale Supporting Observation children (templateId .4.86, which DOES
// mandate code [1..1] LOINC + value [1..*]) along with them: 3 AUDIT-C
// question/answer pairs and 2-3 PHQ-2 question/answer pairs (8 real
// Observations total, confirmed against the parsed FHIR output, which had
// zero trace of any of them) were lost entirely, not just the shell.
// declarative_engine.go's buildOneResource now runs Fields (and so any
// EmitAsResource child) BEFORE checking SkipIfCodeNullFlavor, specifically to
// stop blocking this — see that field's own doc comment in
// declarative_schema.go. assessmentScaleSupportingObservationRow (the SAME
// CollectAll+EmitAsResource primitive Social History's analogous .4.86
// children already use, parameterized here with category="functional-status"
// instead of "social-history") is appended directly onto this rule's own
// Fields, not nested under a SPRT row, because there IS no SPRT-linked shell
// here to nest it under -- the .4.69 entry already IS the matched root.
//
// Verified against the IG before writing this fix, not guessed: GitHub
// HL7/CDA-ccda's StructureDefinition-AssessmentScaleObservation.xml and
// StructureDefinition-AssessmentScaleSupportingObservation.xml (fetched
// 2026-06-28), confirming COMP (not SPRT) attaches the Supporting
// Observation, and that Observation.code is SHALL [1..1] LOINC on the
// Supporting Observation specifically (so reusing the existing, tested
// primitive here is IG-correct, not an invented shape).
//
// Deliberately NOT fixed in this same pass (a narrow, documented gap, not a
// silently-dropped one): the PHQ-2 total-score Supporting Observation
// (LOINC 55758-7) in the 99397 sample carries TWO sibling <value> elements
// (an INT raw score AND a CO SNOMED interpretation) -- spec-permitted
// (Supporting Observation's value is [1..*]), but cda/document's CDAEntry.Value
// is a single field (only the first <value> is parsed at all, same
// single-value convention as every other CDACode/CDAValue field on that
// struct), and FHIR R4's Observation.value[x] is itself single-valued, so
// there is no existing target to carry a second value onto one Observation
// without a new design decision (a second Observation? a .component?) this
// session didn't get explicit sign-off for.
func FunctionalStatusMappingRules() []MappingRule {
	rule := observationRule("functionalStatus", "functional-status")
	rule.Fields = append(rule.Fields, assessmentScaleSupportingObservationRow("functional-status"))
	return []MappingRule{rule}
}

// MentalStatusMappingRules returns the OOB declarative rule for the
// "mentalStatus" section (category="cognitive-status"). Same
// assessmentScaleSupportingObservationRow fix as FunctionalStatusMappingRules
// above, for the same reason -- Assessment Scale Observation (.4.69) COMP
// children would otherwise survive the parent (engine-level
// SkipIfCodeNullFlavor fix) but never get extracted into their own
// resources, since nothing in observationRule()'s Fields scopes into
// entryRelationships[typeCode=COMP].entry[templateId=...4.86] -- but unlike
// Functional Status, NOT found via a real corpus example: every real Mental
// Status section available this session (99397 plus 10 other real CCDs) is
// either empty or carries only a single plain Mental Status Observation
// (.4.74), never an Assessment Scale Observation.
//
// Applied anyway, on IG evidence rather than corpus evidence, after explicit
// user sign-off: HL7 CDAR2_IG_CCDA_CLINNOTES_R1_DSTU2.1_2015AUG_Vol2 (Dec
// 2018 errata), Table 146 "Mental Status Section (V2) Contexts" lists
// "Assessment Scale Observation (optional)" as a direct entry sibling of
// Mental Status Organizer/Observation, confirmed by templateId
// 2.16.840.1.113883.10.20.22.4.69 in that section's own constraint #7
// (CONF:1198-28313/28314). Table 232 "Assessment Scale Observation
// Contexts" confirms the reverse direction too -- "Mental Status Section
// (V2) (optional)" and "Mental Status Observation (V3) (optional)" both
// listed as valid containers, right alongside "Functional Status Section
// (V2)"/"Functional Status Observation (V2)" -- and names "Mini-Mental
// Status Exam (assesses cognitive function)" as a worked example of this
// exact template. Constraint #146 on that same page confirms the
// entryRelationship is typeCode=COMP to Assessment Scale Supporting
// Observation (.4.86) -- identical nesting to Functional Status's already-
// fixed case, so the same primitive applies unchanged, just re-parameterized.
func MentalStatusMappingRules() []MappingRule {
	rule := observationRule("mentalStatus", "cognitive-status")
	rule.Fields = append(rule.Fields, assessmentScaleSupportingObservationRow("cognitive-status"))
	return []MappingRule{rule}
}

// ClinicalNoteMappingRules returns the OOB declarative rule for a Note
// Activity entry (templateId 2.16.840.1.113883.10.20.22.4.202, extension
// 2016-11-01), emitting one US Core DocumentReference per entry.
//
// Found auditing the "historyOfPresentIllness" section (LOINC 10164-2) of
// the 99397 sample: Epic titles this section "Progress Notes" and uses it to
// carry the entire visit note as a single Note Activity act, NOT the
// official C-CDA R2.1 "Progress Note (V3)" document template (templateId
// .1.9, confirmed absent from this and every other sample this session) or
// the "Assessment Section" (.2.8, narrative-only, no entry at all per the
// IG). The section's own templateId (1.3.6.1.4.1.19376.1.5.3.1.3.4, the
// legacy IHE Consolidated CDA "History of Present Illness Section",
// CONF:9566) isn't registered in ccda_2_1.json at all -- resolveKey falls
// through to the LOINC match on 10164-2, which IS registered under the
// "historyOfPresentIllness" key, so SectionKey is correct despite the
// templateId mismatch.
//
// User-confirmed this exact entry shape recurs across every Epic site they
// have seen, not just this one sample.
//
// Field provenance, every one read directly off the real entry (no
// speculative fields):
//   - code: primary is LOINC 34109-9 "Note"; translations include LOINC
//     11506-3 "Progress note" (US Core's own DocumentReference.type
//     example value) plus two Epic-internal note-type codes.
//     cda_code_to_codeable_concept folds ALL of these into one
//     CodeableConcept's coding[] -- no row-level cherry-picking needed.
//   - text: <reference value="#NoteNN"/>, already resolved to the full
//     plain-text note body by resolveEntryRefs/parseEntryText (CDA Release
//     2 narrative-reference mechanism, applied generically -- no
//     Note-Activity-specific resolution code needed).
//   - statusCode: "completed" (the only value seen, paired with a
//     participant[typeCode=LA] Signer) -- status is fixed to "current"
//     rather than built from a one-value-observed transform.
//   - effectiveTime: single TS, not a range -- cda_timerange_to_onset
//     already produces a full seconds+timezone ISO 8601 string, which
//     satisfies FHIR instant as-is.
//   - author: entry-level <author><assignedAuthor> (the note's own author,
//     NOT the document header's author -- a different person in general).
//     Mirrors the bare-Practitioner-fallback row observationRule() already
//     uses for performer[0], retargeted to author[0].
//   - informant->assignedEntity->representedOrganization: the facility
//     name ("mumbai Community Health and Affiliates" in the sample) ->
//     DocumentReference.custodian. CDAEntry.Informants is new this change
//     (cda/document/types.go, entry_parser.go) -- no entry anywhere in this
//     codebase parsed <informant> before.
//   - id: two <id> elements -> identifier[], cda_ii_to_identifier.
//
// Deliberately NOT mapped (no real field to source them from on this
// entry): context.encounter (would need EncompassingEncounter linkage this
// entry carries no reference to) and participant[typeCode=LA] Signer (no
// US Core DocumentReference field models attestation the way
// Composition.attester does) -- both documented gaps, not oversights.
func ClinicalNoteMappingRules() []MappingRule {
	return []MappingRule{
		{
			SectionKey:     "historyOfPresentIllness",
			FHIRResource:   "DocumentReference",
			EntryMatch:     "templateId=2.16.840.1.113883.10.20.22.4.202",
			PatientRefPath: []string{"subject"},
			Fields: []MappingRow{
				{
					SourcePath: "code",
					Transform:  "cda_code_to_codeable_concept",
					TargetPath: "type",
				},
				{
					LiteralValue: map[string]interface{}{
						"coding": []interface{}{
							map[string]interface{}{
								"system":  "http://hl7.org/fhir/us/core/CodeSystem/us-core-documentreference-category",
								"code":    "clinical-note",
								"display": "Clinical Note",
							},
						},
					},
					TargetPath: "category[0]",
				},
				{
					LiteralValue: "current",
					TargetPath:   "status",
				},
				{
					SourcePath: "effectiveTime",
					Transform:  "cda_timerange_to_onset",
					TargetPath: "date",
				},
				{
					SourcePath: "text",
					Transform:  "cda_text_to_attachment",
					TargetPath: "content[0].attachment",
				},
				{
					Scope:      "id[*]",
					CollectAll: true,
					Transform:  "cda_ii_to_identifier",
					TargetPath: "identifier",
				},
				{
					Scope: "authors[0].assignedAuthor",
					Fields: []MappingRow{
						{
							Scope:      "assignedPerson.names[*]",
							CollectAll: true,
							Transform:  "cda_name_to_fhir",
							TargetPath: "name",
						},
						{
							Scope:            "ids[*]",
							CollectAll:       true,
							Transform:        "cda_ii_to_identifier",
							TargetPath:       "identifier",
							EmbedCDAIdentity: true,
						},
						{
							Scope:      "telecoms[*]",
							CollectAll: true,
							Transform:  "cda_telecom_to_fhir",
							TargetPath: "telecom",
						},
						{
							Scope:      "addresses[*]",
							CollectAll: true,
							Transform:  "cda_address_to_fhir",
							TargetPath: "address",
						},
					},
					EmitAsResource: "Practitioner",
					TargetPath:     "author[0]",
				},
				{
					Scope: "informants[0].assignedEntity.representedOrganization",
					Fields: []MappingRow{
						{
							SourcePath: "names[0]",
							TargetPath: "name",
						},
						{
							Scope:            "ids[*]",
							CollectAll:       true,
							Transform:        "cda_ii_to_identifier",
							TargetPath:       "identifier",
							EmbedCDAIdentity: true,
						},
						{
							Scope:      "telecoms[*]",
							CollectAll: true,
							Transform:  "cda_telecom_to_fhir",
							TargetPath: "telecom",
						},
						{
							Scope:      "addresses[*]",
							CollectAll: true,
							Transform:  "cda_address_to_fhir",
							TargetPath: "address",
						},
					},
					EmitAsResource: "Organization",
					TargetPath:     "custodian",
				},
			},
		},
	}
}

// observationValueXFHIRKeys lists every Observation.value[x] TargetPath the
// rows below can write -- shared between each value[x] row's own TargetPath
// and the dataAbsentReason row's SkipIfResourceHasAnyOf gate, so the two stay
// in sync by construction rather than by separately-maintained literals.
var observationValueXFHIRKeys = []string{
	"valueQuantity", "valueCodeableConcept", "valueString", "valueBoolean", "valueInteger", "valuePeriod",
}

// observationValueXDispatchRows returns the shared Observation.value[x]
// polymorphic dispatch -- mirrors observation_mapper.go:295-336's
// setObservationValue switch. Used both by observationRule() (Vital Signs/
// Results/Social History/Functional Status/Mental Status) and by
// SocialHistoryMappingRules' SDOH Assessment Scale / Assessment Scale
// Supporting Observation rows, since all three need IDENTICAL value[x]
// handling against a CDAValue at their own Scope root -- extracted instead
// of copy-pasted a third time per this repo's no-copy-paste-between-files
// standard.
func observationValueXDispatchRows() []MappingRow {
	return []MappingRow{
		{
			// PQ — observation_mapper.go:301-306. Kareo: height/weight/BMI.
			Scope:      "value[type=PQ]",
			SourcePath: "quantity",
			Transform:  "cda_quantity_to_fhir",
			TargetPath: "valueQuantity",
		},
		{
			// CD/CE/CS — observation_mapper.go:307-312. Kareo/PF smoking
			// status (SNOMED CD).
			Scope:          "value[type=CD]",
			ScopeFallbacks: []string{"value[type=CE]", "value[type=CS]"},
			SourcePath:     "code",
			Transform:      "cda_code_to_codeable_concept",
			TargetPath:     "valueCodeableConcept",
		},
		{
			// ST/ED — observation_mapper.go:313-316.
			Scope:          "value[type=ST]",
			ScopeFallbacks: []string{"value[type=ED]"},
			SourcePath:     "text",
			TargetPath:     "valueString",
		},
		{
			// BL — observation_mapper.go:317-320. No transform: the
			// resolved value is already a JSON bool (CDAValue.Boolean
			// round-trips through *bool), and the empty-Transform path
			// writes it through unchanged.
			Scope:      "value[type=BL]",
			SourcePath: "boolean",
			TargetPath: "valueBoolean",
		},
		{
			// INT — observation_mapper.go:321-324. Same passthrough
			// reasoning as BL.
			Scope:      "value[type=INT]",
			SourcePath: "integer",
			TargetPath: "valueInteger",
		},
		{
			// REAL — observation_mapper.go:325-328. A bare
			// {"value": ...} Quantity, no unit/system/code, unlike PQ.
			Scope:      "value[type=REAL]",
			SourcePath: "real",
			Transform:  "cda_real_to_bare_quantity",
			TargetPath: "valueQuantity",
		},
		{
			// IVL_TS — observation_mapper.go:329-334.
			Scope:      "value[type=IVL_TS]",
			SourcePath: "timeRange",
			Transform:  "cda_timerange_to_period",
			TargetPath: "valuePeriod",
		},
	}
}

func observationRule(sectionKey, categoryCode string) MappingRule {
	fields := []MappingRow{
		{
			// C-CDA Result Observation's own <id> elements (direct children,
			// siblings of code/value) -- confirmed present in 3/4 corpus
			// files: Ascension (47 observations), Epic 99397 (2), Marshfield
			// (1). Dignity Health's Results are genuinely empty. Same
			// CollectAll+Scope idiom as MedicationMappingRules' identifier row.
			Scope:      "id[*]",
			CollectAll: true,
			Transform:  "cda_ii_to_identifier",
			TargetPath: "identifier",
		},
		{
				// observation_mapper.go:227-229 — Obs code (LOINC).
				SourcePath: "code",
				Transform:  "cda_code_to_codeable_concept",
				TargetPath: "code",
			},
			{
				// code.text override from the entry's own <text><reference>
				// (entry.Text, resolved by section_parser.go's
				// resolveEntryRefs/walkForIDs to exactly the text at the
				// referenced "#id" -- the standard, reliable part of CDA
				// narrative reference resolution; no inference about
				// surrounding narrative structure is layered on top of it,
				// deliberately, since narrative <text> layout is not
				// standardized across EHR vendors and any rule built on a
				// particular vendor's convention there is a bet that won't
				// generalize. Real gap found in production data: some EHR
				// vendors (Epic, this session's corpus) reuse one generic
				// LOINC code (e.g. 54522-8 "Functional status") across many
				// unrelated questions in a section, so
				// cda_code_to_codeable_concept's own code.text (the code's
				// DisplayName) gives no way to tell apart e.g. a Height
				// value of 64 from an Exercise-Days value of "6 days" --
				// whatever text the source document's own <text><reference>
				// points to is still more specific to that one entry than
				// the shared generic display, even when it isn't a clean
				// label. This row runs AFTER the code row above and
				// overwrites code.text (not coding[].display, which stays
				// the LOINC-correct "Functional status") whenever the
				// reference resolved to non-empty text; a no-op otherwise --
				// applyRow/declarative_engine.go:477-481 skips writing
				// anything when SourcePath resolves to nothing, so entries
				// with no <text> element at all (most Vitals/Results) are
				// unaffected.
				SourcePath: "text",
				TargetPath: "code.text",
			},
			{
				// observation_mapper.go:209.
				SourcePath: "statusCode",
				Transform:  "observation_status_to_fhir",
				TargetPath: "status",
			},
			{
				// observation_mapper.go:232-234.
				SourcePath: "effectiveTime",
				Transform:  "cda_timerange_to_onset",
				TargetPath: "effectiveDateTime",
			},
			{
				// observation_mapper.go:212-224 — category is a per-rule
				// constant, not derived from CDA data, exactly like
				// Condition's category row above.
				LiteralValue: categoryCode,
				Transform:    "observation_category_to_fhir",
				TargetPath:   "category[0]",
			},
			// Observation.performer -- upgraded from a bare display string
			// to the same PractitionerRole/Practitioner tiered pattern
			// already applied to Medication.requester, Condition.recorder,
			// and Immunization.performer this session. Real gap found
			// auditing Vital Signs (99397 sample, LOINC 8716-3 section):
			// every author there carries a representedOrganization with a
			// real name + full address + id ("mumbai Community Health and
			// Affiliates") and NO assignedPerson, but the old row only ever
			// produced a bare {display: "..."} string, discarding the
			// organization's identifier/address data entirely. HL7's C-CDA
			// on FHIR IG (CF-vitals.md) maps /author to .performer and
			// defers the resource shape to its shared provenance guidance,
			// which recommends "ideally...a PractitionerRole, which can
			// then support both Practitioner (name) and Organization" --
			// the same citation already used for the other three sections.
			// assignedEntityRoleRow degrades gracefully to an
			// organization-only PractitionerRole when no person exists
			// (the common case here), via buildEmittedSubResource's len<=1
			// gate. This row is shared by VitalSigns/Results/SocialHistory/
			// FunctionalStatus/MentalStatus (all six observationRule()
			// callers), upgrading all of them at once -- a deliberate,
			// user-confirmed scope decision, not an oversight.
			assignedEntityRoleRow("authors[0].assignedAuthor", "performer[0]"),
			// Fallback tier (same shape as the other three sections):
			// when there's a person name but NO representedOrganization at
			// all, assignedEntityRoleRow's own
			// EmitAsResourceRequiredPaths=["organization"] gate discards
			// the whole reference -- this keeps a bare Practitioner
			// instead of losing the name entirely.
			func() MappingRow {
				row := barePractitionerRow("authors[0].assignedAuthor", "performer[0]")
				row.SkipIfResourceHasAnyOf = []string{"performer"}
				return row
			}(),
	}

	// Observation.value[x] — polymorphic dispatch. Each Scope predicate is
	// mutually exclusive with every other one (a CDAValue's "type" can only
	// ever match one), so at most one of these rows ever actually fires per
	// entry.
	fields = append(fields, observationValueXDispatchRows()...)

	fields = append(fields,
		MappingRow{
			// observation_mapper.go's interpretation row (CONF:1198-7147,
			// verified 2026-06-22) -- a direct child of the observation,
			// a sibling of code/statusCode/value, not COMP-nested (the
			// prior, IG-incorrect read this section used to replicate;
			// fixed in Go itself, not just here -- see this section's
			// top doc comment).
			SourcePath: "interpretationCode",
			Transform:  "cda_code_to_codeable_concept",
			TargetPath: "interpretation[0]",
		},
		MappingRow{
			// <referenceRange><observationRange><text> -- a direct child of
			// the observation, sibling of interpretationCode above. Real gap
			// found auditing Results (Ascension Wisconsin sample): all 46 lab
			// Observations in that file carry a real free-text range (e.g.
			// "4.0 - 10.0 Thou/uL") that was parsed nowhere before this --
			// see CDAEntry.ReferenceRangeText's own doc comment for why the
			// already-registered cda_reference_range_to_fhir transform
			// doesn't fit (it expects a structured low/high shape, not the
			// free-text shape actually evidenced). Plain string passthrough,
			// no transform needed -- same idiom as the note[0].text rows
			// elsewhere in this file.
			SourcePath: "referenceRangeText",
			TargetPath: "referenceRange[0].text",
		},
		MappingRow{
			// observation_mapper.go:257-260,265-276 — us-core-2: must
			// have value[x]/component/hasMember/dataAbsentReason. Fires
			// only when none of the value[x] rows above wrote anything
			// (component/hasMember are never written by this rule, so
			// they're not in the skip list -- see this section's own
			// top doc comment for why BP-component-combination isn't
			// part of this rule at all).
			LiteralValue:           "unknown",
			Transform:              "observation_data_absent_reason_to_fhir",
			TargetPath:             "dataAbsentReason",
			SkipIfResourceHasAnyOf: observationValueXFHIRKeys,
		},
	)

	return MappingRule{
		SectionKey:           sectionKey,
		FHIRResource:         "Observation",
		FlattenOrganizers:    true,
		PatientRefPath:       []string{"subject"}, // observation_mapper.go:120/205: r["subject"] = ref(patientRef)
		SkipIfCodeNullFlavor: true,                // observation_mapper.go:195-198 -- see that field's own doc comment
		Fields:               fields,
	}
}

// =========================================================
// Immunizations
// =========================================================
//
// Phase 0's inventory flagged TWO items in this section as confirmed bugs.
// Investigating before porting found one was already fixed (stale inventory
// text, not a live issue) and the other was real and is fixed here, in Go,
// alongside the port -- not replicated declaratively:
//   - "status never reads negationInd" (inventory's Cross-cutting finding
//     #10): NOT current behavior. immunization_mapper.go:42 already calls
//     transforms.ImmunizationStatusToFHIR(entry.StatusCode, entry.NegationInd),
//     and mappers_test.go has 4 tests proving the negated/refused case
//     correctly produces "not-done" plus statusReason from RSON. Already
//     fixed before this session; the inventory text just wasn't updated.
//   - "performer reads the wrong field" (inventory's per-row note): WAS
//     real, confirmed by reading entry_parser.go and cda/document/types.go
//     directly -- entry.Participants is parsed from <participant> elements;
//     a C-CDA Immunization Activity's performer is a <performer
//     typeCode="PRF"> element, parsed into the COMPLETELY SEPARATE
//     entry.Performers field (the same field medication_mapper.go's
//     requesterReference already reads correctly). Fixed in
//     immunization_mapper.go (this session), with a new regression test
//     (mappers_test.go's TestMapImmunizations_Performer_ReadFromPerformersNotParticipants)
//     proving the corrected field is read and the old one is NOT.
//
// Deliberately NOT gated (a real, narrow, evidence-free divergence from Go,
// not silently unconsidered): immunization_mapper.go only looks for an RSON
// refusal-reason relationship when entry.NegationInd is true
// (mappers_test.go's TestMapImmunizations_NotNegated_RSONIgnored exercises
// this exact guard). Expressing "this row only applies when a DIFFERENT
// node's field equals X" needs a new gating primitive distinct from
// everything added so far -- Scope narrows roots but can't reach a sibling
// concept once narrowed, and RowCondition overrides a row's OWN value/
// transform but never suppresses the row entirely. 0/4 corpus vendors have
// an RSON relationship at all (negated or not), and C-CDA's RSON templateId
// here is literally "Immunization Refusal Reason" -- attaching it to a
// non-refused immunization would be non-conformant data, not a realistic
// case worth a new primitive for. The statusReason row below is
// unconditional; on real (conformant) data this is a distinction without a
// difference.
func ImmunizationMappingRules() []MappingRule {
	return []MappingRule{
		{
			SectionKey:     "immunizations",
			FHIRResource:   "Immunization",
			PatientRefPath: []string{"patient"}, // immunization_mapper.go:41: r["patient"] = ref(patientRef)
			Fields: []MappingRow{
				{
					// immunization_mapper.go:42 — negationInd takes priority
					// over statusCode (CONF:1198-8985, SHALL [1..1]).
					// ThenTransform="string_direct" so the Then-branch's
					// literal "not-done" passes through unchanged instead of
					// being re-run through the statusCode switch.
					SourcePath: "statusCode",
					Transform:  "immunization_status_to_fhir",
					TargetPath: "status",
					Condition: &RowCondition{
						WhenPath:         "negationInd",
						Equals:           "true",
						ThenLiteralValue: "not-done",
						ThenTransform:    "string_direct",
					},
				},
				{
					// immunization_mapper.go:46-60 — value preferred, code as
					// fallback; same shared idiom as Allergy reactions/
					// Medication RSON/Condition code. See this function's own
					// top doc comment for why this row is NOT gated on
					// negationInd.
					Scope:      "entryRelationships[typeCode=RSON].entry",
					Transform:  "cda_value_or_code_to_codeable_concept",
					TargetPath: "statusReason",
				},
				{
					// immunization_mapper.go:64-70.
					SourcePath: "consumable.manufacturedProduct.manufacturedMaterial.code",
					Transform:  "cda_code_to_codeable_concept",
					TargetPath: "vaccineCode",
				},
				{
					// immunization_mapper.go:74-76.
					SourcePath: "effectiveTime",
					Transform:  "cda_timerange_to_onset",
					TargetPath: "occurrenceDateTime",
				},
				{
					// immunization_mapper.go:79-83.
					SourcePath: "routeCode",
					Transform:  "cda_code_to_codeable_concept",
					TargetPath: "route",
				},
				{
					// immunization_mapper.go:86-90.
					SourcePath: "doseQuantity",
					Transform:  "cda_quantity_to_fhir",
					TargetPath: "doseQuantity",
				},
				{
					// immunization_mapper.go:93-98 — a bare string, not a
					// CodeableConcept; no transform needed.
					SourcePath: "consumable.manufacturedProduct.manufacturedMaterial.lotNumberText",
					TargetPath: "lotNumber",
				},
				// Immunization.performer -- a REAL gap found auditing the
				// 99397 sample's Immunizations section against this same
				// document's actual data (not the 0/4 corpus, which has no
				// performer data at all): every one of the 13 entries has a
				// /performer/assignedEntity/representedOrganization with a
				// real name (most just a name -- "COSWI58" -- 2 of 13 also
				// carry a named assignedPerson), but the OLD row above only
				// ever produced a bare display string for the 2/13 WITH a
				// person name, silently dropping the organization-only
				// performer info on the other 11/13 entirely. HL7's C-CDA on
				// FHIR IG (CF-immunizations.md) maps /performer to a
				// PractitionerRole (function="AP") containing practitioner
				// + organization -- assignedEntityRoleRow already does
				// exactly this, and (per buildEmittedSubResource's len<=1
				// gate) degrades gracefully to an organization-only
				// PractitionerRole when no person name exists, which is
				// the common case here.
				assignedEntityRoleRow("performers[0].assignedEntity", "performer[0].actor"),
				// Fallback tier (same shape as Medication.requester/
				// Condition.recorder): when there's a person name but NO
				// representedOrganization at all, assignedEntityRoleRow's
				// own EmitAsResourceRequiredPaths=["organization"] gate
				// discards the whole reference -- this keeps a bare
				// Practitioner instead of losing the name entirely. Real
				// 99397 data never hits this tier (organization is always
				// present there), but the pre-existing
				// TestDeclarativeEngine_Immunization_Performer_ReadFromPerformersField
				// fixture (person, no organization) does, and previously
				// relied on the now-removed display-only row to capture it.
				func() MappingRow {
					row := barePractitionerRow("performers[0].assignedEntity", "performer[0].actor")
					row.SkipIfResourceHasAnyOf = []string{"performer"}
					return row
				}(),
				{
					// Immunization.performer.function -- IG-fixed constant
					// "AP" (Administering Provider), the only function code
					// C-CDA's Immunization Activity ever uses for /performer.
					// Scope (not a bare LiteralValue with no Scope) gates
					// this to only fire when a performer entity actually
					// exists, so a document with zero performer data never
					// gets a function code with nothing to attach it to.
					// Placed AFTER both actor tiers above so "performer"
					// being already set (top-level key, checked by
					// SkipIfResourceHasAnyOf) only ever reflects whether an
					// actor tier fired -- not this row.
					Scope:        "performers[0].assignedEntity",
					LiteralValue: "AP",
					Transform:    "immunization_performer_function_to_fhir",
					TargetPath:   "performer[0].function",
				},
				{
					// Immunization.recorded -- IG-specified source
					// (/author/time -> .recorded, CF-immunizations.md),
					// never mapped at all despite every one of the 99397
					// sample's 13 Immunization Activity entries carrying a
					// real <author><time .../></author>.
					SourcePath: "authors[0].time",
					Transform:  "cda_time_to_fhir_datetime",
					TargetPath: "recorded",
				},
				{
					// Immunization.note -- IG-specified source (Comment
					// Activity, templateId 2.16.840.1.113883.10.20.22.4.64,
					// code 48767-8, attached via entryRelationship
					// typeCode=COMP -- CF-immunizations.md). Added for IG
					// correctness; UNLIKE Condition.note (Note Activity) or
					// Medication.note (this same Comment Activity), this row
					// has NO real-data evidence in the 99397 sample -- all 13
					// Immunization Activity entries have zero
					// entryRelationships of any kind. Kept as a standard
					// reference-path mapping (not narrative inference) on the
					// strength of the IG citation alone; flagged as
					// unvalidated until a document that actually populates it
					// is found.
					Scope:      "entryRelationships[typeCode=COMP].entry[templateId=" + commentActivityTemplateID + "]",
					SourcePath: "text",
					TargetPath: "note[0].text",
				},
				{
					// Immunization.identifier -- entry.Id, never read by
					// immunization_mapper.go (confirmed absent there too --
					// same documented-gap shape as Encounter's identifier row).
					Scope:      "id[*]",
					Transform:  "cda_ii_to_identifier",
					CollectAll: true,
					TargetPath: "identifier",
				},
				{
					// Immunization.manufacturer -- manufacturerOrganization is
					// already parsed (CDAOrganization, entry_parser.go's
					// parseProduct) but never read; real corpus evidence (this
					// EPIC-sourced document) has a plain <name> with no id on
					// every one of its 13 immunization entries, so a display-
					// only reference (the same "string" case
					// cda_name_or_literal_to_display_ref already handles for
					// Encounter's location row) is the right level of effort --
					// no id exists to justify an emitted Organization resource.
					SourcePath: "consumable.manufacturedProduct.manufacturerOrganization.names[0]",
					Transform:  "cda_name_or_literal_to_display_ref",
					TargetPath: "manufacturer",
				},
			},
		},
	}
}

// =========================================================
// Encounters
// =========================================================
//
// encounter_mapper.go reads ALL entries under the "encounters" section
// uniformly (no EntryType branching), so EntryMatch is "" here too, same as
// every section so far whose Go mapper does the same.
//
// Deliberately NOT ported (document-header-level constructs, not section-
// entry-level -- the declarative engine operates on repeating section
// entries, a structurally different access pattern; these would need a
// document-header rule kind this engine doesn't have yet, not a tweak to
// this rule):
//   - componentOf/encompassingEncounter (id, effectiveTime,
//     dischargeDisposition, facility) -- parsed into
//     CDAHeader.EncompassingEncounter, never read by any mapper. 0/4 corpus
//     has this element (inventory section 8).
//   - documentationOf/serviceEvent/performer -- parsed into
//     CDAHeader.DocumentOf.Performers, never read. Same root cause as the
//     Practitioner section's identical header-level gap.
func EncounterMappingRules() []MappingRule {
	return []MappingRule{
		{
			SectionKey:     "encounters",
			FHIRResource:   "Encounter",
			PatientRefPath: []string{"subject"}, // encounter_mapper.go:34: r["subject"] = ref(patientRef)
			Fields:         encounterFields(),
		},
	}
}

// encounterFields is the shared Fields slice for any classCode="ENC" entry
// mapping to FHIR Encounter -- used both by EncounterMappingRules() (the
// Encounters section) and, at runtime, by PlanOfCareMappingRules()'s
// entryType=encounter branch when an interface/pipeline resolves its target
// resource to "Encounter" instead of "Appointment" (see
// declarative_document_mapper.go's per-document rule-swap logic).
func encounterFields() []MappingRow {
	return []MappingRow{
		{
			// Real gap found auditing a real Ascension Wisconsin CCD's
			// Plan-of-Care section: 18 "Encounter Activity"-shaped entries
			// with moodCode="APT" (planned/future visit) all carry
			// statusCode="active" -- the SAME statusCode a completed (EVN)
			// encounter would carry. encounter_status_to_fhir alone would
			// map "active" to FHIR "in-progress", which is wrong for a visit
			// that hasn't happened yet. moodCode, not statusCode, is the
			// authoritative CDA signal for planned-vs-event here (the same
			// principle ServiceRequest.intent already uses via
			// service_request_intent_from_mood, kept separate from
			// ServiceRequest.status/statusCode) -- so this row runs FIRST
			// and the statusCode-based row below only fires when this one
			// didn't already set a status (SkipIfResourceHasAnyOf idiom,
			// same as Goal.description/ServiceRequest's "Unknown"
			// placeholder elsewhere in this file).
			SourcePath: "moodCode",
			Transform:  "encounter_status_from_planned_mood",
			TargetPath: "status",
		},
		{
			// encounter_mapper.go:37 calls EncounterStatusToFHIR
			// (entry.StatusCode) UNCONDITIONALLY -- even a
					// nullFlavor="NI" "no information" encounter (real
					// corpus data: kareo_sample.xml, practicefusion_sample.xml
					// both have one) gets a status, because the function's
					// default case maps anything unrecognized/empty to
					// "unknown" rather than omitting the field, and FHIR R4's
					// Encounter.status is a required 1..1 element. Without
					// this row reaching the transform on a missing
					// statusCode, the resource ends up empty (no other field
					// on a null-flavor entry resolves either) and gets
					// silently discarded by buildOneResource's empty check --
					// a real document-level divergence (1 resource vs 0),
					// not a hypothetical one. FallbackPaths repeats
					// SourcePath (not a real second path) purely to reach
					// resolveRowSourceValue's LiteralValue fallback tier --
					// see that function's own doc comment for why
					// LiteralValue alone isn't consulted without it.
					SourcePath:             "statusCode",
					FallbackPaths:          []string{"statusCode"},
					LiteralValue:           "",
					Transform:              "encounter_status_to_fhir",
					TargetPath:             "status",
					SkipIfResourceHasAnyOf: []string{"status"},
				},
				{
					// encounter_mapper.go:40-50 — class. Inherits Go's own
					// architecturally-inconsistent fixed v3-ActCode system;
					// see encounter_class_coding's own doc comment for why
					// this port doesn't fix it.
					SourcePath: "code",
					Transform:  "encounter_class_coding",
					TargetPath: "class",
				},
				{
					// encounter_mapper.go:52-54 — type, from the SAME code
					// as class (Go reads entry.Code twice, once per target).
					SourcePath: "code",
					Transform:  "cda_code_to_codeable_concept",
					TargetPath: "type[0]",
				},
				{
					// encounter_mapper.go:58-60.
					SourcePath: "effectiveTime",
					Transform:  "cda_timerange_to_period",
					TargetPath: "period",
				},
				{
					// encounter_mapper.go:62-72,102-131 — participant. Go
					// applies NO typeCode filter on entry.Participants (only
					// drops an individual participant if it ends up
					// completely empty); CollectAll+Fields' own "append only
					// if len(subObj)>0" gives the same end result without
					// needing an explicit filter. "[*]" (not bare
					// "participants") is required here -- a plain key with no
					// brackets resolves to the array itself as ONE node, not
					// a fan-out; only "[*]"/predicate brackets spread into
					// each element (see ResolveCDAPaths' segmentWildcard vs.
					// default case in cda_path_resolver.go).
					Scope:      "participants[*]",
					CollectAll: true,
					TargetPath: "participant",
					Fields: []MappingRow{
						{SourcePath: "typeCode", Transform: "encounter_participant_type_coding", TargetPath: "type[0]"},
						{Scope: "participantRole.playingEntity.names[0]", Transform: "cda_name_or_literal_to_display_ref", TargetPath: "individual"},
					},
				},
				{
					// encounter_mapper.go:74-97 — location. Go's
					// locName fallback is Family-only (not the full
					// given+family buildDisplayFromName), then
					// Code.DisplayName -- ported via SourcePath+FallbackPaths
					// rather than a new transform, since
					// cda_name_or_literal_to_display_ref already wraps a bare
					// string into {"display": s} when fed one (its "string"
					// case, used here instead of its "CDAName" case).
					//
					// One real, narrow, evidence-free divergence: Go's outer
					// loop visits every COMP entryRelationship and OVERWRITES
					// r["location"] on each LOC match found (last match
					// wins, across possibly-multiple COMP relationships);
					// this engine's non-CollectAll Scope resolution takes the
					// FIRST match instead (scopedNodes[0] in applyRow). 0/4
					// corpus has a Location at all (inventory section 8), so
					// there's no real document to validate either ordering
					// against -- documented, not guessed at with new
					// machinery for an edge case no real data exercises.
					//
					// ScopeFallbacks added after auditing a real Ascension
					// Wisconsin CCD's Plan-of-Care section: its 18 planned
					// (moodCode=APT) encounter entries carry a DIRECT
					// participant typeCode=LOC on the entry itself, not
					// wrapped in a COMP entryRelationship -- the primary
					// Scope above never matches that shape. Same
					// "wrapped, else direct" idiom as problemObsScopeFallbacks.
					Scope:          "entryRelationships[typeCode=COMP].entry.participants[typeCode=LOC]",
					ScopeFallbacks: []string{"participants[typeCode=LOC]"},
					SourcePath:     "participantRole.playingEntity.names[0].family",
					FallbackPaths:  []string{"participantRole.playingEntity.code.displayName"},
					Transform:      "cda_name_or_literal_to_display_ref",
					TargetPath:     "location[0].location",
				},
				{
					// Encounter.identifier -- entry.Id (CDAEntry.Id, the
					// encounter's own <id> elements), never read by Go's
					// encounter_mapper.go at all (confirmed: no Id reference
					// anywhere in that file) -- a genuine gap, not a ported
					// Go limitation, same shape PatientMappingRules'
					// "ids[*]" row already establishes for entry.Id ->
					// identifier (the json tag is singular "id" here because
					// CDAEntry's own struct field is "Id", unlike CDAPatient's
					// "Ids").
					//
					// EmbedCDAIdentity: true -- added so
					// declarative_document_mapper.go can match this Encounter
					// against componentOf/encompassingEncounter by shared
					// CDA <id> (root+extension), per the HL7 C-CDA on FHIR
					// IG's own Encounters mapping page: "when the same
					// encounter is referenced multiple times (such as the
					// encompassingEncounter and an Encounter Activity in the
					// Encounters Section containing the same <id>), it
					// should be converted to a single FHIR resource." See
					// EncompassingEncounterMappingRules' own doc comment for
					// why this match is done with an explicit field-level
					// merge instead of the generic DeduplicationRule.
					Scope:            "id[*]",
					Transform:        "cda_ii_to_identifier",
					CollectAll:       true,
					TargetPath:       "identifier",
					EmbedCDAIdentity: true,
				},
				{
					// Encounter.participant (performer) -- the <performer>
					// element (CDAEntry.Performers/CDAAssignedEntity) is
					// structurally distinct from <participant>
					// (CDAEntry.Participants/CDAParticipantRole) and was never
					// read by Go's encounter_mapper.go either (also confirmed
					// absent). The performing clinician becomes a real,
					// referenceable Practitioner resource instead of being
					// silently dropped.
					//
					// assignedEntityRoleRow (not a bare EmitAsResource=
					// "Practitioner" row): this is the ONLY row that builds a
					// Practitioner for performers[*] -- the sibling
					// "participants[*]" row above only ever produces a
					// display-only {display: name} reference
					// (cda_name_or_literal_to_display_ref), never an emitted
					// Practitioner -- so swapping in PractitionerRole here is
					// a like-for-like replacement, not an addition with a
					// duplication risk to manage (unlike Author/CareTeam's
					// participant-vs-performer split). Closes the same
					// address/representedOrganization gap CareTeam's
					// component performer had: assignedEntity.Addresses/
					// .RepresentedOrganization are parsed but were never read
					// here either.
					Scope:      "performers[*]",
					CollectAll: true,
					TargetPath: "participant",
					Fields: []MappingRow{
						{SourcePath: "typeCode", Transform: "encounter_participant_type_coding", TargetPath: "type[0]"},
						assignedEntityRoleRow("assignedEntity", "individual"),
					},
				},
				{
					// Encounter.diagnosis -- the nested SUBJ-typed
					// entryRelationship on an Encounter entry is the C-CDA
					// "Encounter Diagnosis" idiom: a Problem Concern Act
					// (.4.80) wrapping a Problem Observation (.4.4), the
					// EXACT same shape as a standalone Problems-section entry
					// (see conditionFields' own doc comment) -- just scoped
					// to this one visit instead of the patient's whole
					// problem list. encounter_mapper.go never reads
					// entryRelationships at all, so these visit-specific
					// diagnoses were silently dropped entirely: no Condition
					// resource, no Encounter.diagnosis link, even though the
					// data is fully parsed and available (CDAEntry.
					// EntryRelationships). Confirmed via real corpus data
					// (EPIC-sourced encounter with 4 nested diagnosis acts)
					// that these are NOT duplicated in the document's own
					// standalone Problems section -- this is the only path
					// that captures them.
					//
					// EmitAsResourcePatientRefPath:["subject"] is required
					// here (unlike CareTeam's emitted Practitioner, which
					// needs none) -- buildOneResource's PatientRefPath
					// application never reaches an EmitAsResource-built
					// resource, since it's returned as a sibling "extra"
					// resource, not the rule's own.
					Scope:      "entryRelationships[typeCode=SUBJ].entry",
					CollectAll: true,
					TargetPath: "diagnosis",
					Fields: []MappingRow{
						{
							EmitAsResource:               "Condition",
							EmitAsResourcePatientRefPath: []string{"subject"},
							TargetPath:                   "condition",
							Fields:                        conditionFields("encounter-diagnosis"),
						},
					},
				},
			}
}

// =========================================================
// Procedures
// =========================================================
//
// procedure_mapper.go reads ALL entries under the "procedures" section
// uniformly across all 3 sibling templates (Procedure Activity Procedure
// .4.14, Observation .4.13, Act .4.12) -- buildProcedureResource treats them
// identically, no EntryType branching. Ported the same way: EntryMatch="",
// one rule. The inventory suggests the declarative engine "should enumerate
// the 3 sibling templates as distinct rows since CONF numbers/code-system
// guidance differ slightly" -- not done here, since Go itself doesn't
// differentiate them either; porting Go's verified behavior, not inventing
// a finer split Go's own code doesn't make.
//
// Two inventory-flagged items investigated, both already non-issues by the
// time this slice was written:
//   - Body site "structurally wrong" / "CDAEntry has no TargetSiteCode
//     field at all" (inventory's section 9): STALE. The struct gained
//     TargetSiteCode (cda/document/types.go) and procedure_mapper.go:77 has
//     read it correctly since before this session, with 3 passing tests
//     (mappers_test.go:883-949) already proving the Procedure-Activity-
//     Procedure-only / no-COMP-inference behavior this rule ports as-is.
//   - Performer's "dead computation" (CDANameToFHIR result computed then
//     discarded, buildDisplayFromName recomputes it) — a real code-quality
//     issue in Go, but NOT a correctness bug (the discarded value and the
//     value actually used produce the same non-empty/empty verdict for any
//     real CDAName), so the declarative port below (which only ever calls
//     buildDisplayFromName's equivalent, cda_name_or_literal_to_display_ref)
//     is behaviorally identical without needing a Go-side fix first.
func ProcedureMappingRules() []MappingRule {
	return []MappingRule{
		{
			SectionKey:     "procedures",
			FHIRResource:   "Procedure",
			PatientRefPath: []string{"subject"}, // procedure_mapper.go:34: r["subject"] = ref(patientRef)
			Fields: []MappingRow{
				{
					// procedure_mapper.go:37.
					SourcePath: "statusCode",
					Transform:  "procedure_status_to_fhir",
					TargetPath: "status",
				},
				{
					// procedure_mapper.go:40-42.
					SourcePath: "code",
					Transform:  "cda_code_to_codeable_concept",
					TargetPath: "code",
				},
				{
					// procedure_mapper.go:45-46 — period, preferred.
					SourcePath: "effectiveTime",
					Transform:  "cda_timerange_to_period",
					TargetPath: "performedPeriod",
				},
				{
					// procedure_mapper.go:47-48 — single datetime, only when
					// the period row above wrote nothing (Go's else-if).
					SourcePath:             "effectiveTime",
					Transform:              "cda_timerange_to_onset",
					TargetPath:             "performedDateTime",
					SkipIfResourceHasAnyOf: []string{"performedPeriod"},
				},
				{
					// procedure_mapper.go:51-67 — performer. The OR-list
					// predicate ("PRF|SPRF") is a real, justified path-
					// resolver addition this slice needed (see
					// cda_path_resolver.go's cdaPredicateValueEquals doc
					// comment) -- Go's `p.TypeCode == "PRF" || p.TypeCode ==
					// "SPRF"` has no other clean expression in this schema.
					Scope:      "participants[typeCode=PRF|SPRF]",
					CollectAll: true,
					TargetPath: "performer",
					Fields: []MappingRow{
						{Scope: "participantRole.playingEntity.names[0]", Transform: "cda_name_or_literal_to_display_ref", TargetPath: "actor"},
					},
				},
				{
					// procedure_mapper.go:77-81.
					SourcePath: "targetSiteCode",
					Transform:  "cda_code_to_codeable_concept",
					TargetPath: "bodySite[0]",
				},
			},
		},
	}
}

// =========================================================
// CarePlan / Goal (Plan of Care)
// =========================================================
//
// Investigating before porting confirmed the inventory's suspicion:
// careplan_mapper.go's MapCarePlans (-> CarePlan resource) has ZERO callers
// anywhere outside its own file -- not wired into document_mapper.go's
// typedSectionDispatchers (which routes every Plan-of-Care section-key
// alias to mappers.MapPlanOfCare instead), and zero test references either.
// Confirmed dead code, like MapPractitioners before it -- not ported.
//
// Every field in this slice is deliberately NOT patientRef-dependent
// (Encounter/Procedure/Appointment/SupplyRequest's subject/participant/
// deliverFor-from-patientRef fields are all skipped here), consistent with
// every Phase 3 rule before this one: no declarative rule so far sets
// "subject" or any other patientRef-derived reference, since patientRef is
// a document-level value the per-entry MappingRow model has no parameter
// for. This is a cross-cutting Phase 4 (cutover) concern, not specific to
// this slice.
//
// goalFields() is shared by GoalMappingRules() (the standalone "goals"
// section) and this file's planOfCareRulesForSectionKey() GOL-mood branch,
// mirroring goal_mapper.go/plan_of_care_mapper.go's own
// shared-buildGoalResource reuse. medicationRequestFields() (defined above,
// alongside medicationRequestRule()) is reused the same way for the
// substanceAdministration branch.

// goalFields mirrors goal_mapper.go's buildGoalResource.
func goalFields() []MappingRow {
	return []MappingRow{
		{
			// goal_mapper.go:36.
			SourcePath: "statusCode",
			Transform:  "goal_status_to_fhir",
			TargetPath: "lifecycleStatus",
		},
		{
			// goal_mapper.go:38-43 — description preferred from Value.Code
			// over the entry's own Code (Value, when present, carries the
			// actual stated goal text/code; the entry's Code is the Goal
			// Observation template's own discriminator code, a weaker
			// fallback). FallbackPaths walks both candidates from the SAME
			// root (the entry itself) since value.code and code are just two
			// different depths under it, not two different scopes.
			SourcePath:    "value.code",
			FallbackPaths: []string{"code"},
			Transform:     "cda_code_to_codeable_concept",
			TargetPath:    "description",
		},
		{
			// goal_mapper.go:44-49 — final non-IG placeholder so the
			// resource is never invalid (Goal.description is 1..1).
			// LiteralValue is a CodeableConcept-shaped map, not a string —
			// Transform is deliberately unset (passthrough) so it isn't
			// re-marshaled as a CDACode by the row above's transform; using
			// SkipIfResourceHasAnyOf instead of stacking this onto the row
			// above's FallbackPaths avoids exactly that mismatch (a
			// transform always re-applies to whatever LiteralValue ends up
			// resolved, including a final literal fallback — fine for a bare
			// string, wrong for an already-FHIR-shaped map).
			LiteralValue:           map[string]interface{}{"text": "Goal"},
			TargetPath:             "description",
			SkipIfResourceHasAnyOf: []string{"description"},
		},
		{
			// goal_mapper.go:53-57.
			SourcePath: "effectiveTime",
			Transform:  "cda_timerange_to_onset",
			TargetPath: "target[0].dueDate",
		},
	}
}

// GoalMappingRules returns the OOB declarative rule for the standalone
// "goals" section (document_mapper.go's typedSectionDispatchers["goals"]).
func GoalMappingRules() []MappingRule {
	return []MappingRule{
		{
			SectionKey:     "goals",
			FHIRResource:   "Goal",
			PatientRefPath: []string{"subject"}, // goal_mapper.go:33: r["subject"] = ref(patientRef)
			Fields:         goalFields(),
		},
	}
}

// planOfCareSectionKeys mirrors document_mapper.go's 4 section-key aliases
// for the SAME Plan-of-Care dispatcher (mappers.MapPlanOfCare) — a document
// produces exactly one of these depending on which title/templateId
// resolution path its own section took, never more than one simultaneously,
// so seeding all 4 is real coverage, not speculative duplication.
var planOfCareSectionKeys = []string{"carePlan", "planOfCare", "assessmentAndPlan", "planOfTreatment"}

// isPlanOfCareSectionKey reports whether sectionKey is one of the 4
// Plan-of-Care aliases — used by DeclarativeMapDocument (declarative_document_mapper.go)
// to decide whether the runtime Appointment->Encounter swap below applies.
func isPlanOfCareSectionKey(sectionKey string) bool {
	for _, k := range planOfCareSectionKeys {
		if k == sectionKey {
			return true
		}
	}
	return false
}

// applyPlanOfCareEncounterTarget replaces PlanOfCareMappingRules()'s
// Appointment-targeting "entryType=encounter" rule, within an
// ALREADY-CLONED rules slice (see DeclarativeMapDocument's own clone-before-
// mutate comment — declarativeSectionRuleGroupsCache is a package-level
// singleton read concurrently by every in-flight document and must never be
// mutated in place), with an Encounter-targeting equivalent that reuses
// encounterFields(). PlanOfCareMappingRules()'s own Go literal keeps
// returning Appointment unchanged — this swap is purely a per-document
// runtime decision (see DeclarativeMapDocument's planOfCareEncounterTarget
// resolution), not a change to any OOB rule's hardcoded definition.
func applyPlanOfCareEncounterTarget(rules []MappingRule) []MappingRule {
	for i, rule := range rules {
		if rule.FHIRResource == "Appointment" && rule.EntryMatch == "entryType=encounter" {
			rules[i] = MappingRule{
				SectionKey: rule.SectionKey,
				FHIRResource:      "Encounter",
				EntryMatch:        rule.EntryMatch,
				FlattenOrganizers: rule.FlattenOrganizers,
				// EncounterMappingRules()'s own rule sets this at the
				// MappingRule level (not a Fields row) -- without it, a
				// swapped Encounter never gets its required (US Core 1..1)
				// subject reference. Caught via real end-to-end verification
				// against the Ascension file, not by the earlier unit
				// tests (which used BuildResources/BuildResourcesForRules
				// directly without checking for a missing subject).
				PatientRefPath: EncounterMappingRules()[0].PatientRefPath,
				Fields:         encounterFields(),
			}
		}
	}
	return rules
}

// PlanOfCareMappingRules returns the OOB declarative rules for every
// Plan-of-Care section-key alias, each as its own 6-rule
// BuildResourcesForRules dispatch group mirroring
// plan_of_care_mapper.go's dispatchPlanOfCareEntry exactly:
//
//	moodCode=EVN              -> nothing (already happened, not a plan entry)
//	moodCode=GOL               -> Goal                (checked BEFORE EntryType,
//	                                                    same priority as Go)
//	entryType=substanceAdministration -> MedicationRequest
//	entryType=encounter        -> Appointment
//	entryType=supply           -> SupplyRequest
//	entryType=procedure|observation|act -> ServiceRequest
//
// Rule order within each group matters: BuildResourcesForRules' first-
// match-wins claiming means the two moodCode-based rules MUST be listed
// before the four EntryType-based ones, mirroring Go's own moodCode-checked-
// before-EntryType-switch order — otherwise a moodCode=GOL,
// EntryType=observation entry (a completely normal Goal Observation shape)
// would be claimed by the ServiceRequest rule's "entryType=observation"
// match instead of the Goal rule. The relative order within each pair
// (EVN/GOL, and the four EntryType branches) doesn't matter — those values
// are mutually exclusive on any single real entry.
//
// All 6 rules set FlattenOrganizers: true (see BuildResourcesForRules' own
// doc comment on why every rule sharing a SectionKey must agree on this) —
// mirrors flattenPlanOfCareEntries expanding organizer components before
// Go's own dispatch switch runs (plan_of_care_mapper.go:52-62).
func PlanOfCareMappingRules() []MappingRule {
	var rules []MappingRule
	for _, sectionKey := range planOfCareSectionKeys {
		rules = append(rules, planOfCareRulesForSectionKey(sectionKey)...)
	}
	return rules
}

func planOfCareRulesForSectionKey(sectionKey string) []MappingRule {
	return []MappingRule{
		{
			// plan_of_care_mapper.go:65-66 — "already happened" entries are
			// skipped entirely. An empty Fields list always produces a
			// resource of len<=1 (just resourceType), which buildOneResource
			// already discards — but the entry is still CLAIMED (see
			// BuildResourcesForRules), so it can't fall through to a later
			// rule in this same group.
			SectionKey:        sectionKey,
			FHIRResource:      "",
			EntryMatch:        "moodCode=EVN",
			FlattenOrganizers: true,
			// []MappingRow{}, not nil: the migration seeds this row's fields
			// as a literal JSON "[]", which json.Unmarshal always decodes to
			// a non-nil empty slice -- matching that here keeps
			// reflect.DeepEqual happy in the V158 drift-guard test instead
			// of failing on a nil-vs-empty-slice technicality.
			Fields: []MappingRow{},
		},
		{
			// plan_of_care_mapper.go:68-70 — moodCode=GOL takes priority over
			// EntryType (mappers_test.go:588-599 asserts this priority
			// directly).
			SectionKey:        sectionKey,
			FHIRResource:      "Goal",
			EntryMatch:        "moodCode=GOL",
			FlattenOrganizers: true,
			PatientRefPath:    []string{"subject"}, // goal_mapper.go:33: r["subject"] = ref(patientRef)
			Fields:            goalFields(),
		},
		{
			// plan_of_care_mapper.go:72-73 — reuses medicationRequestFields(),
			// the SAME rows medicationRequestRule() (medications section)
			// uses, since both call the identical Go builder.
			SectionKey:        sectionKey,
			FHIRResource:      "MedicationRequest",
			EntryMatch:        "entryType=substanceAdministration",
			FlattenOrganizers: true,
			PatientRefPath:    []string{"subject"}, // medication_mapper.go:47: r["subject"] = ref(patientRef)
			Fields:            medicationRequestFields(),
		},
		{
			// plan_of_care_mapper.go:74-75,127-153. PatientRefPath
			// deliberately NOT set: buildPlannedAppointmentResource writes
			// patientRef into participant[0].actor (a nested array element,
			// not a bare reference field), which PatientRefPath's
			// single-level resource[path]={"reference":...} write can't
			// express. 0/4 corpus has Plan of Treatment entries (already
			// documented elsewhere in this file), so there's no real data to
			// validate a new primitive against — left as a documented gap,
			// not silently dropped.
			SectionKey:        sectionKey,
			FHIRResource:      "Appointment",
			EntryMatch:        "entryType=encounter",
			FlattenOrganizers: true,
			Fields: []MappingRow{
				{SourcePath: "statusCode", Transform: "appointment_status_to_fhir", TargetPath: "status"},
				{SourcePath: "code", Transform: "cda_code_to_codeable_concept", TargetPath: "serviceType[0]"},
				{SourcePath: "effectiveTime", Transform: "cda_timerange_to_period_start", TargetPath: "start"},
				{SourcePath: "effectiveTime", Transform: "cda_timerange_to_period_end", TargetPath: "end"},
			},
		},
		{
			// plan_of_care_mapper.go:76-77,160-177.
			SectionKey:        sectionKey,
			FHIRResource:      "SupplyRequest",
			EntryMatch:        "entryType=supply",
			FlattenOrganizers: true,
			PatientRefPath:    []string{"deliverFor"}, // plan_of_care_mapper.go:171: r["deliverFor"] = ref(patientRef)
			Fields: []MappingRow{
				{SourcePath: "statusCode", Transform: "supply_request_status_to_fhir", TargetPath: "status"},
				{
					// quantity is hardcoded to 1 in Go too -- CDA's Planned
					// Supply template has no typed quantity field on
					// CDAEntry, and SupplyRequest.quantity is required.
					// float64, not int: this row's canonical form is the JSON
					// a migration seeds (json.Number decodes to float64) --
					// same reasoning as medicationCommonRows' frequency row.
					LiteralValue: map[string]interface{}{"value": float64(1)},
					TargetPath:   "quantity",
				},
				{SourcePath: "code", Transform: "cda_code_to_codeable_concept", TargetPath: "itemCodeableConcept"},
				{SourcePath: "effectiveTime", Transform: "cda_timerange_to_onset", TargetPath: "authoredOn"},
			},
		},
		{
			// plan_of_care_mapper.go:78-79,87-124.
			SectionKey:        sectionKey,
			FHIRResource:      "ServiceRequest",
			EntryMatch:        "entryType=procedure|observation|act",
			FlattenOrganizers: true,
			PatientRefPath:    []string{"subject"}, // plan_of_care_mapper.go:95: r["subject"] = ref(patientRef)
			Fields: []MappingRow{
				{SourcePath: "statusCode", Transform: "service_request_status_to_fhir", TargetPath: "status"},
				{SourcePath: "moodCode", Transform: "service_request_intent_from_mood", TargetPath: "intent"},
				{SourcePath: "code", Transform: "cda_code_to_codeable_concept", TargetPath: "code"},
				{
					// Real gap found auditing 3 independently-sourced real CCDs
					// (Marshfield Clinic, Dignity Health -- neither the 99397
					// Epic sample nor Ascension's, both already covered above):
					// a Planned Act (.4.39) entry whose own <code> is a bare
					// nullFlavor="UNK" with NO <originalText> child at all --
					// unlike the Health Maintenance Observation (.4.44) shape
					// elsewhere in this same section, where the reference lives
					// INSIDE code's own originalText and so already resolves via
					// parseCD/resolveCodeRef before this engine ever sees it.
					// Here the resolved content is the entry's own sibling
					// <text><reference value="#id"/></text> instead -- e.g.
					// "Return to Clinic - Endocrinology 6/17/26" or a full
					// office-visit note -- which the row above never reads
					// (SourcePath="code" only), so every one of these fell
					// through to the generic "Unknown" placeholder below,
					// discarding real, often substantial narrative content.
					// Mirrors the entry.text-to-code.text override row already
					// proven in observationRule()/ClinicalNoteMappingRules().
					SourcePath: "text",
					TargetPath: "code.text",
				},
				{
					// us-core-servicerequest requires code (1..1) — same
					// SkipIfResourceHasAnyOf placeholder idiom as Goal's
					// description above.
					LiteralValue:           map[string]interface{}{"text": "Unknown"},
					TargetPath:             "code",
					SkipIfResourceHasAnyOf: []string{"code"},
				},
				{SourcePath: "effectiveTime", Transform: "cda_timerange_to_period", TargetPath: "occurrencePeriod"},
				{
					SourcePath:             "effectiveTime",
					Transform:              "cda_timerange_to_onset",
					TargetPath:             "occurrenceDateTime",
					SkipIfResourceHasAnyOf: []string{"occurrencePeriod"},
				},
				{
					// plan_of_care_mapper.go:113-120 — first matching RSON
					// only (Go's loop breaks after the first, even if that
					// one's code is empty); the non-CollectAll Scope's own
					// scopedNodes[0]-only semantics already match this.
					//
					// Reads the RSON entry's OWN value first, code as fallback
					// (cda_value_or_code_to_codeable_concept) -- NOT just
					// ".code" like Go's original implementation did. Real
					// corpus evidence (this EPIC-sourced document) shows why
					// that was wrong: the RSON-linked observation is a Problem
					// Observation (.4.19, the SAME template Condition's own
					// code row reads), whose "code" is the IG-fixed wrapper
					// code (29308-4 "Diagnosis" -- a label, not a diagnosis)
					// and whose "value" carries the real SNOMED diagnosis
					// (e.g. 305058001 "Screening for malignant neoplasm of
					// cervix"). Go's own behavior here predates Condition's
					// value-preferred row and was never reconciled with it --
					// this fix brings ServiceRequest.reasonCode in line with
					// the same idiom every other RSON/value-or-code row in
					// this file already uses.
					Scope:      "entryRelationships[typeCode=RSON].entry",
					Transform:  "cda_value_or_code_to_codeable_concept",
					TargetPath: "reasonCode[0]",
				},
				{
					// ServiceRequest.requester -- the ordering provider's
					// <author> (NPI, name, address, telecom,
					// representedOrganization -- real, rich data in this
					// corpus) was never read by plan_of_care_mapper.go at all;
					// same documented-gap shape as Medication's analogous
					// requester row, reused here via the identical transform.
					Scope:      "authors[0].assignedAuthor.assignedPerson.names[0]",
					Transform:  "cda_name_or_literal_to_display_ref",
					TargetPath: "requester",
				},
			},
		},
	}
}

// assignedEntityRoleRow returns a single EmitAsResource="PractitionerRole"
// row that emits a SECOND, independently-correlated Practitioner alongside
// its Organization — the right shape specifically for CareTeam's component
// performer, where the richer assignedEntity data and the sparser
// participant-derived Practitioner it enriches come from two genuinely
// DIFFERENT CDA nodes (<participant> vs a sibling <component><act>
// <performer>) with no structural link between them other than matching on
// an id (DeduplicationRule, matched on _cdaIds — the same "richer resource
// wins" mechanism CareTeamMappingRules already documents and tests). When
// the caller's own Practitioner comes from the EXACT SAME CDA node as the
// assignedEntity (Author, LegalAuthenticator), don't use this — use
// organizationLinkRow below instead, which links to that already-known
// resource directly instead of gambling on an id-match dedup that real
// corpus evidence (cerner_sample.xml's and practicefusion_sample.xml's
// authors) shows is frequently absent on exactly these elements.
//
// basePath is the CDA path to the assignedEntity node itself, relative to
// whatever node the caller's own Scope already resolved (e.g.
// "performers[0].assignedEntity" for a <performer>). targetPath is where the
// caller wants the PractitionerRole's own reference written — irrelevant
// when the caller discards it via the "_emitOnly" convention (see
// CareTeamMappingRules), required when the caller actually keeps it.
//
// PractitionerRole only carries practitioner/organization/specialty/telecom
// here — name, identifier, address etc. belong on the nested Practitioner
// and Organization themselves (FHIR's own separation of "who" from "in what
// capacity, where"), each emitted as its OWN independent top-level resource
// via the singular-row EmitAsResource nesting declarative_engine.go's
// applyRow/buildEmittedSubResource thread extraOut through for (see
// MappingRow.EmitAsResource's doc comment).
//
// EmitAsResourceRequiredPaths: []string{"organization"} discards the whole
// PractitionerRole (and, atomically, the Practitioner/Organization nested
// inside it — see buildEmittedSubResource's localExtra) whenever
// representedOrganization has neither a name nor an id to build an
// Organization from: without this gate, a performer with only a name/id but
// no organization still produced a second, name/id-driven Practitioner plus
// a near-empty PractitionerRole carrying nothing but that duplicate's
// reference — clutter no organization data justified.
// barePractitionerRow returns a single EmitAsResource="Practitioner" row --
// no enclosing PractitionerRole, no Organization, no required-paths gate --
// for use as a fallback tier after assignedEntityRoleRow when the source
// author/performer has no representedOrganization at all but still has a
// name or identifier worth keeping. CareTeam's own participant->member row
// already establishes this exact "bare Practitioner, len(sub)<=1 is the
// only gate" shape; this is that shape extracted into a function so
// Condition.recorder's fallback tier (see conditionFields()) doesn't
// hand-copy it a second time.
//
// Real gap this fixes: a 99397 CCD sample (Epic source) has, on every one
// of its 5 Active Problems' Problem Observation <author>, an NPI id and a
// representedOrganization-less assignedAuthor -- assignedEntityRoleRow's
// own EmitAsResourceRequiredPaths=["organization"] gate (correct for when
// richer data justifies a full PractitionerRole) discarded the WHOLE
// recorder reference for all 5, including the NPI, with no fallback at all
// before this row existed. Mirrors Medication.requester's own
// rich-tier-then-fallback shape (requesterFromAuthor/requesterFromPerformer
// then a display-only row), just stopping one tier earlier since a bare
// Practitioner (unlike a bare display string) still carries the NPI.
func barePractitionerRow(basePath, targetPath string) MappingRow {
	return MappingRow{
		Scope:          basePath,
		EmitAsResource: "Practitioner",
		TargetPath:     targetPath,
		Fields: []MappingRow{
			{
				Scope:      "assignedPerson.names[*]",
				Transform:  "cda_name_to_fhir",
				CollectAll: true,
				TargetPath: "name",
			},
			{
				Scope:            "ids[*]",
				Transform:        "cda_ii_to_identifier",
				CollectAll:       true,
				TargetPath:       "identifier",
				EmbedCDAIdentity: true,
			},
			{
				SourcePath: "code",
				Transform:  "cda_code_to_codeable_concept",
				TargetPath: "qualification[0].code",
			},
			{
				Scope:      "telecoms[*]",
				Transform:  "cda_telecom_to_fhir",
				CollectAll: true,
				TargetPath: "telecom",
			},
			{
				Scope:      "addresses[*]",
				Transform:  "cda_address_to_fhir",
				CollectAll: true,
				TargetPath: "address",
			},
		},
	}
}

func assignedEntityRoleRow(basePath, targetPath string) MappingRow {
	return MappingRow{
		EmitAsResource:              "PractitionerRole",
		EmitAsResourceRequiredPaths: []string{"organization"},
		TargetPath:                  targetPath,
		Fields: []MappingRow{
			{
				Scope:          basePath,
				EmitAsResource: "Practitioner",
				TargetPath:     "practitioner",
				Fields: []MappingRow{
					{
						Scope:      "assignedPerson.names[*]",
						Transform:  "cda_name_to_fhir",
						CollectAll: true,
						TargetPath: "name",
					},
					{
						Scope:            "ids[*]",
						Transform:        "cda_ii_to_identifier",
						CollectAll:       true,
						TargetPath:       "identifier",
						EmbedCDAIdentity: true,
					},
					{
						SourcePath: "code",
						Transform:  "cda_code_to_codeable_concept",
						TargetPath: "qualification[0].code",
					},
					{
						Scope:      "telecoms[*]",
						Transform:  "cda_telecom_to_fhir",
						CollectAll: true,
						TargetPath: "telecom",
					},
					{
						Scope:      "addresses[*]",
						Transform:  "cda_address_to_fhir",
						CollectAll: true,
						TargetPath: "address",
					},
				},
			},
			{
				Scope:          basePath + ".representedOrganization",
				EmitAsResource: "Organization",
				TargetPath:     "organization",
				Fields: []MappingRow{
					{
						// Same bare-string-first-name shape as
						// CustodianMappingRules' org.Names[0] row.
						SourcePath: "names[0]",
						TargetPath: "name",
					},
					{
						Scope:            "ids[*]",
						Transform:        "cda_ii_to_identifier",
						CollectAll:       true,
						TargetPath:       "identifier",
						EmbedCDAIdentity: true,
					},
					{
						Scope:      "telecoms[*]",
						Transform:  "cda_telecom_to_fhir",
						CollectAll: true,
						TargetPath: "telecom",
					},
					{
						Scope:      "addresses[*]",
						Transform:  "cda_address_to_fhir",
						CollectAll: true,
						TargetPath: "address",
					},
				},
			},
			{
				// PractitionerRole.specialty (NOT .code, which is the role
				// TYPE e.g. "doctor" — assignedEntity.code here is the NUCC
				// provider-taxonomy specialty, e.g. 207Q00000X Family
				// Medicine Physician).
				SourcePath: basePath + ".code",
				Transform:  "cda_code_to_codeable_concept",
				TargetPath: "specialty[0]",
			},
			{
				Scope:      basePath + ".telecoms[*]",
				Transform:  "cda_telecom_to_fhir",
				CollectAll: true,
				TargetPath: "telecom",
			},
		},
	}
}

// organizationLinkRow returns a single EmitAsResource="PractitionerRole" row
// that links an ALREADY-built Practitioner — practitionerRef, e.g.
// "Practitioner/author-1" — to its CDA representedOrganization, WITHOUT
// re-emitting a second Practitioner the way assignedEntityRoleRow does.
//
// Use this (not assignedEntityRoleRow) whenever the caller's own Fields
// already build a top-level Practitioner directly from the SAME CDA node
// basePath points at (Author, LegalAuthenticator): there's no ambiguity to
// resolve via dedup-by-id, since the Practitioner and its
// representedOrganization are definitionally the same entity's data, read
// from the exact same XML fragment. practitionerRef is therefore a plain
// literal, not another emitted resource — and a correct one: Author's
// caller (declarative_document_mapper.go) hardcodes "author-1" as that
// resource's id immediately after BuildHeaderResource returns, the same
// convention Custodian's "custodian-1" already uses, for the identical
// reason (at most one Author/Custodian/LegalAuthenticator resource is ever
// built per document, so a fixed id is globally unique on its own).
//
// Relying on dedup-by-id instead (assignedEntityRoleRow's approach) was
// tried first for Author and reverted: real corpus evidence
// (cerner_sample.xml's, practicefusion_sample.xml's authors) shows these
// specific CDA elements frequently carry NO id at all, which left a
// second, un-mergeable Practitioner in the bundle for the same real author
// — a regression, not an improvement. A literal reference has no such
// failure mode.
//
// EmitAsResourceRequiredPaths: []string{"organization"} -- same gate as
// assignedEntityRoleRow's, same reason: discard the whole PractitionerRole
// when there's no representedOrganization to link (the common case for
// Author — most real authors have no organization at all), leaving
// practitionerRef's own resource as the only Practitioner, with nothing
// extra to clutter the bundle.
func organizationLinkRow(basePath, practitionerRef, targetPath string) MappingRow {
	return MappingRow{
		EmitAsResource:              "PractitionerRole",
		EmitAsResourceRequiredPaths: []string{"organization"},
		TargetPath:                  targetPath,
		Fields: []MappingRow{
			{
				LiteralValue: map[string]interface{}{"reference": practitionerRef},
				TargetPath:   "practitioner",
			},
			{
				Scope:          basePath + ".representedOrganization",
				EmitAsResource: "Organization",
				TargetPath:     "organization",
				Fields: []MappingRow{
					{
						// Same bare-string-first-name shape as
						// CustodianMappingRules' org.Names[0] row.
						SourcePath: "names[0]",
						TargetPath: "name",
					},
					{
						Scope:            "ids[*]",
						Transform:        "cda_ii_to_identifier",
						CollectAll:       true,
						TargetPath:       "identifier",
						EmbedCDAIdentity: true,
					},
					{
						Scope:      "telecoms[*]",
						Transform:  "cda_telecom_to_fhir",
						CollectAll: true,
						TargetPath: "telecom",
					},
					{
						Scope:      "addresses[*]",
						Transform:  "cda_address_to_fhir",
						CollectAll: true,
						TargetPath: "address",
					},
				},
			},
			{
				// See assignedEntityRoleRow's identical row for why this is
				// .specialty, not .code.
				SourcePath: basePath + ".code",
				Transform:  "cda_code_to_codeable_concept",
				TargetPath: "specialty[0]",
			},
			{
				Scope:      basePath + ".telecoms[*]",
				Transform:  "cda_telecom_to_fhir",
				CollectAll: true,
				TargetPath: "telecom",
			},
		},
	}
}

// careTeamSectionKeys mirrors document_mapper.go's typedSectionDispatchers:
// "careTeam" is the schema/templateId-resolved key, "careTeams" is what
// title-fallback produces — both wired to mappers.MapCareTeam, never more
// than one populated per real document.
var careTeamSectionKeys = []string{"careTeam", "careTeams"}

// CareTeamMappingRules returns the OOB declarative rule for both of
// document_mapper.go's Care Team section-key aliases.
//
// careteam_mapper.go's MapCareTeam is this slice's reason for two new engine
// primitives (MappingRow.EmitAsResource, MappingRule.RequiredPaths — see
// their doc comments in declarative_schema.go): for each Care Team Organizer
// entry, EVERY participant:lead becomes BOTH a CareTeam.participant entry
// AND its own independent Practitioner resource, cross-referenced by the
// former — the first "one matched entry produces multiple, cross-referenced
// top-level resources" shape this engine has needed to port.
//
// Deliberately NOT ported (documented, not silently dropped):
//   - careteam_mapper.go's buildCareTeamParticipants builds an NPI→name
//     lookup from entry.Components[].Performers[].AssignedEntity to enrich a
//     Practitioner whose outer participant carried only an NPI, no name.
//     This cross-reference (one part of the entry feeding a lookup used
//     while iterating an unrelated part of the same entry) is a distinct
//     capability from EmitAsResource and has zero corpus evidence and zero
//     dedicated Go test coverage of its own (per the inventory) — not
//     justified by any concrete failure, unlike every other primitive added
//     this session. A Practitioner whose only identifying data is an NPI
//     (no playingEntity name) is built with identifier only, matching
//     buildPractitionerResource's own behavior before enrichment.
//
// `subject`/`_cdaIds` (Phase 4 Slice A, closed): PatientRefPath="subject"
// below sets CareTeam's own patient link; the emitted Practitioner's
// identifier row sets EmbedCDAIdentity, mirroring
// practitioner_mapper.go:76's embedCDAIds(r, p.ParticipantRole.Ids) — the
// same dedup marker every other mapper-built Practitioner already gets.
func CareTeamMappingRules() []MappingRule {
	rules := make([]MappingRule, 0, len(careTeamSectionKeys))
	for _, sectionKey := range careTeamSectionKeys {
		rules = append(rules, MappingRule{
			SectionKey:     sectionKey,
			FHIRResource:   "CareTeam",
			RequiredPaths:  []string{"participant"},
			PatientRefPath: []string{"subject"}, // careteam_mapper.go:111: r["subject"] = ref(patientRef)
			Fields: []MappingRow{
				{SourcePath: "statusCode", Transform: "care_team_status_to_fhir", TargetPath: "status"},
				{SourcePath: "code", Transform: "cda_code_to_codeable_concept", TargetPath: "category[0]"},
				{SourcePath: "effectiveTime", Transform: "cda_timerange_to_period", TargetPath: "period"},
				{
					// careteam_mapper.go:41-95 (buildCareTeamParticipants):
					// one CareTeam.participant + one emitted Practitioner per
					// participants[*] match, skipping any participant with
					// neither an id nor a playingEntity (mirrored here by
					// buildEmittedSubResource's own "len(sub) <= 1" discard —
					// no row fires, the Practitioner is never built, and
					// childRow.TargetPath="member" is never set, so the
					// whole participant subObj stays empty and is dropped).
					Scope:      "participants[*]",
					CollectAll: true,
					TargetPath: "participant",
					Fields: []MappingRow{
						{
							EmitAsResource: "Practitioner",
							TargetPath:     "member",
							Fields: []MappingRow{
								{
									Scope:      "participantRole.playingEntity.names[*]",
									Transform:  "cda_name_to_fhir",
									CollectAll: true,
									TargetPath: "name",
								},
								{
									Scope:            "participantRole.ids[*]",
									Transform:        "cda_ii_to_identifier",
									CollectAll:       true,
									TargetPath:       "identifier",
									EmbedCDAIdentity: true, // mirrors practitioner_mapper.go:76's embedCDAIds(r, p.ParticipantRole.Ids)
								},
								{
									SourcePath: "participantRole.code",
									Transform:  "cda_code_to_codeable_concept",
									TargetPath: "qualification[0].code",
								},
								{
									Scope:      "participantRole.telecoms[*]",
									Transform:  "cda_telecom_to_fhir",
									CollectAll: true,
									TargetPath: "telecom",
								},
								{
									Scope:      "participantRole.addresses[*]",
									Transform:  "cda_address_to_fhir",
									CollectAll: true,
									TargetPath: "address",
								},
							},
						},
						{
							SourcePath: "functionCode",
							Transform:  "cda_code_to_codeable_concept",
							TargetPath: "role[0]",
						},
					},
				},
				{
					// Enriches the SAME Practitioner identity emitted above,
					// not CareTeam.* itself -- the merge happens later, via
					// DeduplicationRule's "richer resource wins" rule (see
					// that rule's own doc comment), not here. Real corpus
					// evidence (this EPIC-sourced document): the
					// <participant>'s participantRole carries only a bare
					// NPI -- no name, specialty, address, or telecom -- while
					// the SAME person's full identity (name, specialty,
					// address, phone/fax) sits on a sibling
					// <component><act><performer>, which is structurally
					// unreachable from inside the participant-derived
					// Practitioner's own EmitAsResource Fields (those resolve
					// relative to the matched participant node, never a
					// sibling field on the entry itself -- see
					// applyCollectAllWithFields's own doc comment on why
					// EmitAsResource is "only meaningful one level deep").
					//
					// This row is a CollectAll+Fields wrapper used ONLY as a
					// vehicle to reach EmitAsResource processing (which
					// applyRow only honors inside such a wrapper, never at a
					// rule's own top level) -- its own sub-object output
					// ("_emitOnly") carries nothing meaningful and is
					// discarded before the bundle ships, the same convention
					// "_cdaIds"/"_emitted" already establish for internal-
					// only fields.
					//
					// Emits PractitionerRole -> {Practitioner, Organization}
					// (assignedEntityRoleRow), not a bare Practitioner: this
					// performer's <representedOrganization> (real, rich data
					// in this corpus -- name/address/telecom for "mumbai
					// Community Health and Affiliates") was previously
					// parsed and silently discarded, structurally
					// unreachable from a bare Practitioner row the same way
					// every other representedOrganization in this file's
					// "Deliberately NOT ported" comments documents.
					// PractitionerRole is the correct FHIR building block
					// for "this person, at this organization, in this
					// specialty" -- not a new field on Practitioner itself,
					// which FHIR deliberately keeps organization-agnostic.
					Scope:      "components[*]",
					CollectAll: true,
					TargetPath: "_emitOnly",
					Fields: []MappingRow{
						assignedEntityRoleRow("performers[0].assignedEntity", "ref"),
					},
				},
			},
		})
	}
	return rules
}

// coverageSectionKeys mirrors document_mapper.go's typedSectionDispatchers:
// "payersInsurance" is the schema/templateId-resolved key, "payors" is what
// title-fallback produces — both wired to mappers.MapCoverage.
var coverageSectionKeys = []string{"payersInsurance", "payors"}

// CoverageMappingRules returns the OOB declarative rule for both of
// document_mapper.go's Coverage/Payers section-key aliases.
//
// C-CDA on FHIR IG alignment (V187 re-seed): the Coverage Activity is a
// container only; all real insurance data lives in the COMP-nested Policy
// Activity (CONF:1198-8939). Field sources corrected per IG:
//
//   - type         <- Policy Activity code (SOP 2.16.840.1.113883.3.221.5)
//   - period       <- participant[COV].time low/high (coverage window)
//   - relationship <- participant[COV].participantRole.code (SELF/SPOUSE/...)
//   - subscriberId <- participant[COV].participantRole.ids[0] (member ID)
//   - payor        <- COMP performers[0].assignedEntity.representedOrg.names[0]
//
// beneficiary/subscriber: both via PatientRefPath (same patientRef).
// status: "active" hardcoded -- CDA statusCode always "completed" (CONF:1198-19094).
func CoverageMappingRules() []MappingRule {
	rules := make([]MappingRule, 0, len(coverageSectionKeys))
	for _, sectionKey := range coverageSectionKeys {
		rules = append(rules, MappingRule{
			SectionKey:     sectionKey,
			FHIRResource:   "Coverage",
			PatientRefPath: []string{"beneficiary", "subscriber"},
			Fields: []MappingRow{
				{LiteralValue: "active", TargetPath: "status"},
				{
					// IG: Coverage.type = Policy Activity SOP code, not
					// the outer Coverage Activity's fixed 48768-6 (section code).
					Scope:      "entryRelationships[typeCode=COMP].entry",
					SourcePath: "code",
					Transform:  "coverage_type_to_codeable_concept",
					TargetPath: "type",
				},
				{
					// IG: Coverage.period = participant[COV].time (coverage window).
					// Outer effectiveTime is the record creation date, not coverage dates.
					Scope:      "entryRelationships[typeCode=COMP].entry.participants[typeCode=COV]",
					SourcePath: "time",
					Transform:  "cda_timerange_to_period",
					TargetPath: "period",
				},
				{
					// IG: payor from PAYOR performer's representedOrganization.
					// PAYOR performer listed first in Policy Activity per C-CDA conformance.
					Scope:      "entryRelationships[typeCode=COMP].entry.performers[0].assignedEntity.representedOrganization.names[0]",
					TargetPath: "payor[0].display",
				},
				{
					// US Core Coverage requires payor (1..1) -- "Unknown" fallback.
					LiteralValue:           "Unknown",
					TargetPath:             "payor[0].display",
					SkipIfResourceHasAnyOf: []string{"payor"},
				},
				{
					// IG: Coverage.relationship = participant[COV].participantRole.code
					// (HL7 RoleCode SELF/SPOUSE/CHILD/etc. -> FHIR subscriber-relationship VS).
					Scope:      "entryRelationships[typeCode=COMP].entry.participants[typeCode=COV].participantRole",
					SourcePath: "code",
					Transform:  "coverage_relationship_to_fhir",
					TargetPath: "relationship",
				},
				{
					// Fallback "self" when COV participant has no relationship code.
					LiteralValue: map[string]interface{}{
						"coding": []interface{}{
							map[string]interface{}{
								"system":  "http://terminology.hl7.org/CodeSystem/subscriber-relationship",
								"code":    "self",
								"display": "Self",
							},
						},
					},
					TargetPath:             "relationship",
					SkipIfResourceHasAnyOf: []string{"relationship"},
				},
				{
					// IG: Coverage.subscriberId = participant[COV].participantRole.ids (member ID).
					// Policy Activity's own id is the contract ID (often nullFlavor=UNK).
					Scope:         "entryRelationships[typeCode=COMP].entry.participants[typeCode=COV].participantRole",
					SourcePath:    "ids[0].extension",
					FallbackPaths: []string{"ids[0].root"},
					TargetPath:    "subscriberId",
				},
			},
		})
	}
	return rules
}

// FamilyMemberHistoryMappingRules returns the OOB declarative rule for the
// "familyHistory" section.
//
// Deliberately NOT ported (documented, not silently dropped — all
// confirmed gaps in Go itself per the inventory's section 14, not
// disagreements with Go's behavior): `administrativeGenderCode` →
// `FamilyMemberHistory.sex`; birthTime-vs-effectiveTime age derivation
// (CONF:1198-15983, a directly-cited CONF# describing exactly this
// derivation, never implemented in Go); the Death Observation (CAUS) →
// `deceasedBoolean`/`deceasedAge`; the Age Observation (SUBJ, inversionInd)
// → `condition[].onsetAge`. None of these exist in Go today — porting them
// would be new functionality, not a port.
//
// `patient` (Phase 4 Slice A, closed): set via PatientRefPath below —
// family_history_mapper.go:34: r["patient"] = ref(patientRef).
func FamilyMemberHistoryMappingRules() []MappingRule {
	return []MappingRule{
		{
			SectionKey:     "familyHistory",
			FHIRResource:   "FamilyMemberHistory",
			PatientRefPath: []string{"patient"},
			Fields: []MappingRow{
				{LiteralValue: "completed", TargetPath: "status"},
				{
					Scope:      "participants[typeCode=SBJ].participantRole.code",
					Transform:  "cda_code_to_codeable_concept",
					TargetPath: "relationship",
				},
				{
					// family_history_mapper.go:46-51 -- lossy: only the
					// family/surname component is kept, given name discarded
					// entirely. The inventory flags this as a candidate
					// improvement (concat given+family), not a bug -- ported
					// as-is, not silently fixed.
					Scope:      "participants[typeCode=SBJ].participantRole.playingEntity.names[0]",
					Transform:  "cda_name_to_family_string",
					TargetPath: "name",
				},
				{
					// family_history_mapper.go:56-77 -- one condition[] per
					// component, value only (never code, the problem-type
					// discriminator field -- family_history_mapper.go
					// correctly distinguishes the two per the inventory).
					Scope:      "components[*]",
					CollectAll: true,
					TargetPath: "condition",
					Fields: []MappingRow{
						{
							SourcePath: "value",
							Transform:  "cda_value_to_fhir",
							TargetPath: "code",
						},
						{
							SourcePath: "effectiveTime",
							Transform:  "cda_timerange_to_onset",
							TargetPath: "onsetDateTime",
						},
					},
				},
			},
		},
	}
}

// DeviceMappingRules returns the OOB declarative rule for the
// "medicalEquipment" section, producing DeviceUseStatement resources — not
// a standalone Device resource (device_mapper.go's own doc comment already
// notes this; the UDI/Product-Instance gap that would need a real Device
// resource is a scope gap in Go itself, not ported here either, same
// "don't add features beyond what Go does today" reasoning as
// FamilyMemberHistory's gaps above).
//
// 2026-06-22 IG verification (architecture/CDA_FHIR_MAPPING_INVENTORY.md
// section 15) confirmed device_mapper.go's participant typeCode check was a
// real bug (checked "DEV"/"CSM", IG fixes "PRD" per CONF:1098-8754) — fixed
// in device_mapper.go itself this slice, and this rule uses the corrected
// "PRD" value from the start, not the bug.
func DeviceMappingRules() []MappingRule {
	return []MappingRule{
		{
			SectionKey:     "medicalEquipment",
			FHIRResource:   "DeviceUseStatement",
			PatientRefPath: []string{"subject"}, // device_mapper.go:34: r["subject"] = ref(patientRef)
			Fields: []MappingRow{
				{
					SourcePath: "statusCode",
					Transform:  "device_use_statement_status_to_fhir",
					TargetPath: "status",
				},
				{
					// device_mapper.go:39-54 -- PRD participant's
					// playingEntity code preferred over the entry's own
					// code. Simplification: Go always OVERRIDES with the
					// PRD participant's code whenever that participant is
					// present AND has a non-nil playingEntity, even if that
					// code is itself empty (losing entry.Code in that edge
					// case); ScopeFallbacks here instead prefers the
					// participant code, else entry.Code, whichever actually
					// resolves -- a narrow, deliberate divergence, not a
					// silent guess (0/4 corpus evidence either way).
					Scope:          "participants[typeCode=PRD].participantRole.playingEntity.code",
					ScopeFallbacks: []string{"code"},
					Transform:      "cda_code_to_codeable_concept",
					TargetPath:     "device.codeableConcept",
				},
				{
					SourcePath: "effectiveTime",
					Transform:  "cda_timerange_to_period",
					TargetPath: "timingPeriod",
				},
				{
					SourcePath:             "effectiveTime",
					Transform:              "cda_timerange_to_onset",
					TargetPath:             "timingDateTime",
					SkipIfResourceHasAnyOf: []string{"timingPeriod"},
				},
			},
		},
	}
}

// =========================================================
// Patient (document header)
// =========================================================
//
// PatientMappingRules returns the OOB declarative rule for the document
// header's patient (documentMap["header"]["patient"]), producing the FHIR
// Patient resource every other section's resources link back to via
// MappingRule.PatientRefPath. Direct port of patient_mapper.go's MapPatient
// — "collect every element" for identifier/name/address/telecom (no Go-side
// branching to replicate) plus a handful of scalar demographics fields.
//
// Phase 4 Slice A (the cutover's prerequisite gap-closing slice — see
// architecture/CDA_FHIR_MAPPING_ENGINE_SPRINT_PLAN.md's Phase 4 section):
// this is the FIRST declarative rule to map Patient at all. No PatientRefPath
// here — a Patient resource doesn't reference itself; its own "id" (always
// "patient-1") is assigned by the document-level orchestrator (Phase 4
// Slice B), the same way Custodian/Author's ids are, not by this rule.
//
// Deliberately NOT ported, documented not silently dropped:
//   - sortNamesByUse's "legal name first" reordering (patient_mapper.go:36-38)
//     — CollectAll has no sort primitive; names are written in source-document
//     order instead. Zero corpus evidence of a Patient with multiple
//     differently-`use`d names to validate a new primitive against (check
//     before assuming this matters for a real document).
//   - languageCommunication/modeCode ("Expressed Written" etc.) — parsed
//     into CDALanguage.ModeCode but never read by
//     declarativeLanguageCommunicationToFHIR; base FHIR R4's
//     Patient.communication has no field or known extension for
//     spoken-vs-written mode, so there is no FHIR target to map it onto.
//
// Race/Ethnicity/Religion extensions are NOT Fields rows here — they need
// the document-header CDACode object's nested {code, displayName} shape
// rather than a scalar SourcePath/Transform write, so they're injected as a
// separate post-processing step (USCoreProfileBuilder.InjectPatientExtensions,
// called from DeclarativeMapDocument right after this rule builds the
// Patient resource) instead of a plain field mapping here.
func PatientMappingRules() []MappingRule {
	return []MappingRule{
		{
			HeaderPath:   "patient",
			FHIRResource: "Patient",
			Fields: []MappingRow{
				{
					// patient_mapper.go:23-33.
					Scope:      "ids[*]",
					Transform:  "cda_ii_to_identifier",
					CollectAll: true,
					TargetPath: "identifier",
				},
				{
					// patient_mapper.go:36-48 -- source-document order; see
					// this function's own doc comment on sortNamesByUse.
					Scope:      "names[*]",
					Transform:  "cda_name_to_fhir",
					CollectAll: true,
					TargetPath: "name",
				},
				{
					// patient_mapper.go:51-61.
					Scope:      "addresses[*]",
					Transform:  "cda_address_to_fhir",
					CollectAll: true,
					TargetPath: "address",
				},
				{
					// patient_mapper.go:64-74.
					Scope:      "telecoms[*]",
					Transform:  "cda_telecom_to_fhir",
					CollectAll: true,
					TargetPath: "telecom",
				},
				{
					// patient_mapper.go:77-79.
					SourcePath: "birthDate",
					Transform:  "cda_time_to_fhir_date",
					TargetPath: "birthDate",
				},
				{
					// patient_mapper.go:80-84 -- whole gender CDACode object,
					// not just its bare code, because the guard this mirrors
					// needs both Code and NullFlavor (see
					// declarativeGenderToFHIR's doc comment).
					SourcePath: "gender",
					Transform:  "cda_gender_to_fhir",
					TargetPath: "gender",
				},
				{
					// sdtc:deceasedInd → Patient.deceasedBoolean. No
					// Transform needed: CDAPatient.DeceasedInd is *bool, so
					// ResolveCDAPath returns a real Go bool (true or false)
					// when the element was present, or nil (row no-ops) when
					// it was absent — explicit value="false" is preserved,
					// not treated as "no data" the way a bare bool zero-value
					// would be.
					SourcePath: "deceasedInd",
					TargetPath: "deceasedBoolean",
				},
				{
					// patient_mapper.go:85-89 -- cda_code_to_codeable_concept
					// already returns nil for an all-empty CDACode (see
					// CDACodeToCodeableConcept's own empty-input guard), so
					// this row needs no extra Code!="" check duplicated here.
					SourcePath: "maritalStatus",
					Transform:  "cda_code_to_codeable_concept",
					TargetPath: "maritalStatus",
				},
				{
					// patient_mapper.go:92-116.
					Scope:      "languages[*]",
					Transform:  "cda_language_communication_to_fhir",
					CollectAll: true,
					TargetPath: "communication",
				},
			},
		},
	}
}

// AuthorMappingRules returns the OOB declarative rule for the document
// header's author[] list, producing one Practitioner from the first author
// with a usable assignedPerson — see MappingRule.HeaderPath's doc comment
// and BuildHeaderResource's firstAuthorWithPerson for why this needed the
// header-level rule primitive (patient_mapper.go's MapAuthor/MapCustodian
// are called directly from document_mapper.go before the section-key
// dispatch loop starts, not through any SectionKey at all).
//
// Deliberately NOT ported: `AssignedAuthoringDevice` (device-authored
// entries, explicitly skipped by Go too — `if AssignedPerson == nil {
// continue }` already excludes them via firstAuthorWithPerson's identical
// check).
//
// `_cdaIds` (Phase 4 Slice A, closed): the identifier row below now sets
// EmbedCDAIdentity, mirroring patient_mapper.go:160's
// embedCDAIds(p, a.AssignedAuthor.Ids) exactly.
//
// Address + Organization link (this slice): both used to be confirmed,
// well-evidenced gaps here -- `a.AssignedAuthor.Addresses`/
// `.RepresentedOrganization` exist in the typed struct but
// patient_mapper.go's MapAuthor never read either. Address is now a plain
// sibling row below, directly on this SAME Practitioner -- no duplication
// risk, since it's the exact same CDA node the rows above already read.
// The organization link uses organizationLinkRow, NOT assignedEntityRoleRow
// (CareTeam's helper): that one emits a SECOND Practitioner correlated via
// dedup-by-id, which real corpus evidence (cerner_sample.xml's,
// practicefusion_sample.xml's authors) showed leaves an un-mergeable
// duplicate in the bundle whenever the author carries no id -- common for
// this element. organizationLinkRow instead references THIS resource's own
// well-known id directly ("Practitioner/author-1", hardcoded by
// declarative_document_mapper.go's Author call site immediately after
// BuildHeaderResource returns) -- no dedup gamble, no duplicate, ever. That
// call site must capture and append BuildHeaderResource's extra slice (it
// used to discard it with "_") for the emitted Organization/PractitionerRole
// to actually reach the bundle.
func AuthorMappingRules() []MappingRule {
	return []MappingRule{
		{
			HeaderPath:   "authors",
			FHIRResource: "Practitioner",
			// patient_mapper.go's MapAuthor never checks whether any field
			// beyond resourceType+id actually got set -- it returns the
			// resource unconditionally once an author with >=1 (possibly
			// content-empty) name is found. See SkipEmptyCheck's doc comment
			// in declarative_schema.go for the real corpus case (kareo) this
			// closes a document-level parity gap for.
			SkipEmptyCheck: true,
			Fields: []MappingRow{
				{
					Scope:      "assignedAuthor.assignedPerson.names[*]",
					Transform:  "cda_name_to_fhir",
					CollectAll: true,
					TargetPath: "name",
				},
				{
					Scope:            "assignedAuthor.ids[*]",
					Transform:        "cda_ii_to_identifier",
					CollectAll:       true,
					TargetPath:       "identifier",
					EmbedCDAIdentity: true, // mirrors patient_mapper.go:160's embedCDAIds(p, a.AssignedAuthor.Ids)
				},
				{
					Scope:      "assignedAuthor.telecoms[*]",
					Transform:  "cda_telecom_to_fhir",
					CollectAll: true,
					TargetPath: "telecom",
				},
				{
					Scope:      "assignedAuthor.addresses[*]",
					Transform:  "cda_address_to_fhir",
					CollectAll: true,
					TargetPath: "address",
				},
				organizationLinkRow("assignedAuthor", "Practitioner/author-1", "_emitOnly"),
			},
		},
	}
}

// CustodianMappingRules returns the OOB declarative rule for the document
// header's custodian, producing one Organization.
//
// `_cdaIds` (Phase 4 Slice A, closed): the identifier row below sets
// EmbedCDAIdentity, mirroring patient_mapper.go:207's
// embedCDAIds(r, org.Ids) exactly.
func CustodianMappingRules() []MappingRule {
	return []MappingRule{
		{
			HeaderPath:    "custodian",
			FHIRResource:  "Organization",
			RequiredPaths: []string{"name"},
			Fields: []MappingRow{
				{
					// patient_mapper.go:184-191 -- bare string, first name
					// only (org.Names[0]), not a CodeableConcept.
					SourcePath: "assignedCustodian.representedCustodianOrganization.names[0]",
					TargetPath: "name",
				},
				{
					// us-core-organization requires `active`; the custodian
					// of a document we just received is, by definition, an
					// active organization (patient_mapper.go:192-194's own
					// comment).
					LiteralValue: true,
					TargetPath:   "active",
				},
				{
					Scope:            "assignedCustodian.representedCustodianOrganization.ids[*]",
					Transform:        "cda_ii_to_identifier",
					CollectAll:       true,
					TargetPath:       "identifier",
					EmbedCDAIdentity: true,
				},
				{
					Scope:      "assignedCustodian.representedCustodianOrganization.addresses[*]",
					Transform:  "cda_address_to_fhir",
					CollectAll: true,
					TargetPath: "address",
				},
			},
		},
	}
}

// LegalAuthenticatorMappingRules returns the OOB declarative rule for the
// document header's legalAuthenticator (0..1) — base CDA's
// LegalAuthenticator class: time + signatureCode are required whenever the
// element is present, and assignedEntity must be a person (a document is
// legally signed by a person, never an organization). Verified via WebFetch
// against build.fhir.org/ig/HL7/CDA-core-2.0 and HL7Wiki's "CDA R3
// legalAuthenticator" page, 2026-06-22.
//
// This is genuinely NEW functionality, not a port: no Go mapper has ever
// read header.legalAuthenticator, and it wasn't even parsed before this
// slice (see cda/document/header_parser.go's parseLegalAuthenticator). With
// no existing Go fidelity target to match field-for-field, the full
// CDAAssignedEntity shape (name/identifier/telecom/address) is captured.
//
// Composition.attester wiring (FHIR R4: mode="legal", time, party -> this
// Practitioner) is deferred to Phase 4: composition_mapper.go's
// CompositionMapper builds Composition from document_mapper.go's LIVE
// resource list today, which this declarative rule is not part of yet —
// same dormant-until-cutover status as every other rule in this file. The
// PractitionerRole row below (organizationLinkRow, same helper
// AuthorMappingRules now uses, not assignedEntityRoleRow's
// dedup-by-id-correlated second Practitioner — this resource's
// representedOrganization comes from the exact same CDA node, so there's
// nothing to correlate) is therefore dormant too, ready the moment this
// rule's wired in. "Practitioner/legalauthenticator-1" mirrors Author's own
// "Practitioner/author-1"/Custodian's "Organization/custodian-1" convention
// — whichever future call site wires this rule into
// declarative_document_mapper.go must assign that exact id immediately
// after BuildHeaderResource returns, the same way Author/Custodian's call
// sites already do, for this reference to resolve correctly.
func LegalAuthenticatorMappingRules() []MappingRule {
	return []MappingRule{
		{
			HeaderPath:    "legalAuthenticator",
			FHIRResource:  "Practitioner",
			RequiredPaths: []string{"name"},
			Fields: []MappingRow{
				{
					Scope:      "assignedEntity.assignedPerson.names[*]",
					Transform:  "cda_name_to_fhir",
					CollectAll: true,
					TargetPath: "name",
				},
				{
					Scope:      "assignedEntity.ids[*]",
					Transform:  "cda_ii_to_identifier",
					CollectAll: true,
					TargetPath: "identifier",
				},
				{
					Scope:      "assignedEntity.telecoms[*]",
					Transform:  "cda_telecom_to_fhir",
					CollectAll: true,
					TargetPath: "telecom",
				},
				{
					Scope:      "assignedEntity.addresses[*]",
					Transform:  "cda_address_to_fhir",
					CollectAll: true,
					TargetPath: "address",
				},
				organizationLinkRow("assignedEntity", "Practitioner/legalauthenticator-1", "_emitOnly"),
			},
		},
	}
}


// EncompassingEncounterLocationMappingRules returns the OOB declarative rule
// for the document header's componentOf/encompassingEncounter/location/
// healthCareFacility, producing one FHIR Location.
//
// header_parser.go's parseComponentOf already parses this element into
// CDAHeader.EncompassingEncounter.Location.HealthCareFacility (code,
// location name/address, serviceProviderOrganization) -- it was simply
// never read by any mapper or rule, exactly the same class of gap
// EncounterMappingRules' own doc comment already flagged ("document-header-
// level constructs ... not a tweak to this rule, [need] a document-header
// rule kind this engine doesn't have yet" -- that kind already exists, via
// HeaderPath/BuildHeaderResource, just unused for this element until now).
//
// 0/4 corpus files (cerner/kareo/mtuitive/practicefusion) carry
// componentOf/encompassingEncounter at all -- there is no real document to
// validate this rule's exact shape against; built from the struct shape +
// FHIR R4/US Core Location definitions alone, the same way
// LegalAuthenticatorMappingRules was. Genuinely new functionality, not a
// port: no Go mapper ever read this field either.
//
// managingOrganization links serviceProviderOrganization directly (a plain
// single reference field on Location, unlike Practitioner's address-via-
// PractitionerRole join) -- no organizationLinkRow/assignedEntityRoleRow
// needed here, just a singular EmitAsResource row, the same shape Author's
// own organizationLinkRow row already proves works as a direct (non-nested)
// member of a rule's top-level Fields.
//
// Wiring (declarative_document_mapper.go): unlike LegalAuthenticator, this
// IS wired in (no Composition.attester-style downstream dependency blocks
// it). The caller assigns a fixed id ("location-1", mirroring author-1/
// custodian-1), adds "Location" to the id-renumbering skip-list (the same
// way Patient/Practitioner/Organization are skipped) immediately after
// BuildHeaderResource returns, then a post-merge step links every Encounter
// resource that doesn't already have its own location (i.e. one NOT already
// set by the more specific section-entry-level
// "entryRelationships[typeCode=COMP].entry.participants[typeCode=LOC]" row
// above) to "Location/location-1": componentOf/encompassingEncounter
// describes ONE overarching visit for the whole document, so applying it
// document-wide rather than rebuilding a per-rule reference here is the
// correct interpretation of what this CDA element actually means.
//
// NOT closed here: the facility's own <id> (e.g. root="...549.2.7.2.686980"
// extension="103001002") is silently dropped by header_parser.go --
// SelectElement("id") is never called on hcfEl at all, so it never reaches
// CDAHealthCareFacility, let alone this rule. FHIR Location.identifier could
// carry it once parsed; that's a parser change, out of scope for a mapping-
// rule addition.
func EncompassingEncounterLocationMappingRules() []MappingRule {
	return []MappingRule{
		{
			HeaderPath:    "encompassingEncounter.location.healthCareFacility",
			FHIRResource:  "Location",
			RequiredPaths: []string{"name"},
			Fields: []MappingRow{
				{
					// CDAPlace.Names is []CDAName (the HL7 PN datatype), not
					// []string -- a facility/place <name> is real-world almost
					// always unstructured text (e.g. "Mumbai Women's Care"),
					// which parsePN (datatypes.go) lands in .Family (see that
					// function's own fallback comment). FHIR Location.name is
					// `string`, not a HumanName/CodeableConcept, so this reads
					// .family directly rather than running cda_name_to_fhir.
					SourcePath: "location.names[0].family",
					TargetPath: "name",
				},
				{
					// healthCareFacility's own <code> -- the department/
					// specialty (e.g. "Obstetrics & Gynecology"), Location.type
					// not Location.name. Handles nullFlavor+translations the
					// same as every other cda_code_to_codeable_concept row in
					// this file (e.g. Encounter.type[0] above).
					SourcePath: "code",
					Transform:  "cda_code_to_codeable_concept",
					TargetPath: "type[0]",
				},
				{
					// Location.address is 0..1 (unlike Practitioner/
					// Organization's address[]) -- SourcePath+singular
					// targetPath, not Scope+CollectAll.
					SourcePath: "location.addresses[0]",
					Transform:  "cda_address_to_fhir",
					TargetPath: "address",
				},
				{
					// serviceProviderOrganization -- the same CDAOrganization
					// shape Custodian/CareTeam's representedOrganization rows
					// already read, just reached via a different CDA path.
					Scope:          "serviceProviderOrganization",
					EmitAsResource: "Organization",
					TargetPath:     "managingOrganization",
					Fields: []MappingRow{
						{SourcePath: "names[0]", TargetPath: "name"},
						{
							Scope:            "ids[*]",
							Transform:        "cda_ii_to_identifier",
							CollectAll:       true,
							TargetPath:       "identifier",
							EmbedCDAIdentity: true,
						},
						{Scope: "telecoms[*]", Transform: "cda_telecom_to_fhir", CollectAll: true, TargetPath: "telecom"},
						{Scope: "addresses[*]", Transform: "cda_address_to_fhir", CollectAll: true, TargetPath: "address"},
					},
				},
			},
		},
	}
}

// EncompassingEncounterMappingRules returns the OOB declarative rule for the
// document header's componentOf/encompassingEncounter, producing one FHIR
// Encounter.
//
// Per the official HL7 C-CDA on FHIR Implementation Guide's own Encounters
// mapping page (build.fhir.org/ig/HL7/ccda-on-fhir/CF-encounters.html,
// verified via WebFetch 2026-06-27): "if the document itself contains a
// componentOf/encompassingEncounter, this should also be converted to a
// FHIR Encounter resource. In all cases, when the same encounter is
// referenced multiple times (such as the encompassingEncounter and an
// Encounter Activity in the Encounters Section containing the same <id>),
// it should be converted to a single FHIR resource."
//
// Consolidation is NOT delegated to DeduplicationRule -- the mechanism every
// other multi-source resource in this engine uses (CareTeam's participant
// Practitioner, Author's organizationLinkRow, etc.): that rule's merge
// semantics are whole-resource winner-take-all by top-level field count
// (assembly/rules/dedup_rule.go's own doc comment). The richer in-section
// Encounter (period/class/type/status/participant/diagnosis/identifier)
// would always outscore this header candidate's handful of fields, and the
// LOSING resource's fields are discarded entirely, not merged --
// hospitalization.dischargeDisposition (this rule's one genuinely unique
// field; no in-section row sets it) would be silently lost on every
// document where the ids actually match, which is the exact case this IG
// guidance is about. declarative_document_mapper.go does an explicit,
// field-level merge instead (see its own "EncompassingEncounter" section):
// match by shared _cdaIds (the same identifier row's EmbedCDAIdentity
// EncounterMappingRules' own id[*] row now also sets), copy hospitalization
// across unconditionally, merge identifier, and append period only when the
// in-section side doesn't already have one -- then either discard this
// resource (matched) or keep it standalone (no match).
//
// status defaults to "unknown" via the identical SourcePath="statusCode"/
// FallbackPaths/LiteralValue="" plumbing EncounterMappingRules' own status
// row uses (declarative_oob_rules.go) -- not because encompassingEncounter
// has a statusCode (base CDA's EncompassingEncounter class doesn't define
// one at all; this SourcePath resolves to nothing and falls through to the
// LiteralValue tier every time), but because FHIR R4 Encounter.status is
// 1..1 and this resource must stay valid in the no-match, standalone case.
// When a match IS found, status is one of the fields
// declarative_document_mapper.go's merge step deliberately does NOT copy --
// the in-section Encounter's own statusCode-derived status is authoritative.
//
// This function itself has no RequiredPaths gate -- it always builds a
// resource when encompassingEncounter is present, per the IG's own
// unqualified instruction. declarative_document_mapper.go applies a
// SEPARATE, narrower gate before deciding whether to actually surface or
// merge that resource: a candidate with nothing beyond status (e.g. only
// location/healthCareFacility data, which this rule doesn't even read —
// EncompassingEncounterLocationMappingRules does, and donates it to every
// Encounter lacking one regardless of this rule) is discarded rather than
// becoming a near-duplicate, no-new-information standalone Encounter.
func EncompassingEncounterMappingRules() []MappingRule {
	return []MappingRule{
		{
			HeaderPath:   "encompassingEncounter",
			FHIRResource: "Encounter",
			Fields: []MappingRow{
				{
					SourcePath:    "statusCode",
					FallbackPaths: []string{"statusCode"},
					LiteralValue:  "",
					Transform:     "encounter_status_to_fhir",
					TargetPath:    "status",
				},
				{
					Scope:            "id[*]",
					Transform:        "cda_ii_to_identifier",
					CollectAll:       true,
					TargetPath:       "identifier",
					EmbedCDAIdentity: true,
				},
				{
					SourcePath: "effectiveTime",
					Transform:  "cda_timerange_to_period",
					TargetPath: "period",
				},
				{
					SourcePath: "dischargeDispositionCode",
					Transform:  "cda_code_to_codeable_concept",
					TargetPath: "hospitalization.dischargeDisposition",
				},
			},
		},
	}
}


