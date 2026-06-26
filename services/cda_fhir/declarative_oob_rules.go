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
		{
			// medication_mapper.go:262-280 (requesterReference) — us-core-21
			// requires this field is never empty: performer, else author, else
			// a literal placeholder display. The 3-tier
			// FallbackPaths+LiteralValue chain is exactly the primitive named in
			// declarative_engine.go's resolveRowSourceValue doc comment.
			SourcePath:    "performers[0].assignedEntity.assignedPerson.names[0]",
			FallbackPaths: []string{"authors[0].assignedAuthor.assignedPerson.names[0]"},
			LiteralValue:  "Ordering Provider",
			Transform:     "cda_name_or_literal_to_display_ref",
			TargetPath:    "requester",
			Required:      true,
			Conformance:   "SHALL",
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
func HealthConcernsMappingRules() []MappingRule {
	return []MappingRule{conditionRule("healthConcerns", "health-concern")}
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

// SocialHistoryMappingRules returns the OOB declarative rule for the
// "socialHistory" section (category="social-history") -- includes Smoking
// Status, which has no smoking-specific Go code at all (observation_mapper.go's
// generic path handles it; see that file's own doc comment).
func SocialHistoryMappingRules() []MappingRule {
	return []MappingRule{observationRule("socialHistory", "social-history")}
}

// FunctionalStatusMappingRules returns the OOB declarative rule for the
// "functionalStatus" section (category="functional-status"). Phase 4 Slice D
// closes the gap declarativeObservationCategoryToFHIR's own doc comment
// used to flag: this section had no declarative rule at all (Go's
// MapDocument() covered it via mappers.MapObservations(e, p,
// "functional-status")). No corpus file exercises this section --
// synthetic-data unit test only, same convention as other unexercised
// sections elsewhere in this file.
func FunctionalStatusMappingRules() []MappingRule {
	return []MappingRule{observationRule("functionalStatus", "functional-status")}
}

// MentalStatusMappingRules returns the OOB declarative rule for the
// "mentalStatus" section (category="cognitive-status"). Same gap-closure and
// no-corpus-coverage situation as FunctionalStatusMappingRules above; Go's
// MapDocument() covered it via mappers.MapObservations(e, p,
// "cognitive-status").
func MentalStatusMappingRules() []MappingRule {
	return []MappingRule{observationRule("mentalStatus", "cognitive-status")}
}

// observationValueXFHIRKeys lists every Observation.value[x] TargetPath the
// rows below can write -- shared between each value[x] row's own TargetPath
// and the dataAbsentReason row's SkipIfResourceHasAnyOf gate, so the two stay
// in sync by construction rather than by separately-maintained literals.
var observationValueXFHIRKeys = []string{
	"valueQuantity", "valueCodeableConcept", "valueString", "valueBoolean", "valueInteger", "valuePeriod",
}

func observationRule(sectionKey, categoryCode string) MappingRule {
	rule := MappingRule{
		SectionKey:           sectionKey,
		FHIRResource:         "Observation",
		FlattenOrganizers:    true,
		PatientRefPath:       []string{"subject"}, // observation_mapper.go:120/205: r["subject"] = ref(patientRef)
		SkipIfCodeNullFlavor: true,                // observation_mapper.go:195-198 -- see that field's own doc comment
		Fields: []MappingRow{
			{
				// observation_mapper.go:227-229 — Obs code (LOINC).
				SourcePath: "code",
				Transform:  "cda_code_to_codeable_concept",
				TargetPath: "code",
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
			{
				// observation_mapper.go never mapped performer -- a longstanding
				// gap, not a migration regression. Source <author> is present on
				// most Vital Signs/Results/Social History observations and
				// usually carries at least a recording organization name (often
				// no named person), even when nothing else identifies who
				// recorded it. Mapping it closes the FHIR validator's recurring
				// "all observations should have a performer" Best Practice
				// warning. Person name preferred over org name when both exist.
				Scope:          "authors[0].assignedAuthor.assignedPerson.names[0]",
				ScopeFallbacks: []string{"authors[0].assignedAuthor.representedOrganization.names[0]"},
				Transform:      "cda_name_or_literal_to_display_ref",
				TargetPath:     "performer[0]",
			},

			// Observation.value[x] — polymorphic dispatch mirroring
			// observation_mapper.go:295-336's setObservationValue switch.
			// Each Scope predicate is mutually exclusive with every other
			// one here (a CDAValue's "type" can only ever match one), so at
			// most one of these rows ever actually fires per entry.
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

			{
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
			{
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
		},
	}
	return rule
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
				{
					// immunization_mapper.go:100-115 (just fixed -- see this
					// function's own top doc comment). Takes the first
					// Performer with a usable name unconditionally at index
					// 0, where Go's loop skips a nameless Performer and tries
					// the next one -- 0/4 corpus has ANY performer data, so
					// this multi-performer-with-mixed-name-presence edge case
					// has no real-world evidence either way; documented, not
					// guessed at with an unbounded ScopeFallbacks chain.
					Scope:      "performers[0].assignedEntity.assignedPerson.names[0]",
					Transform:  "cda_name_or_literal_to_display_ref",
					TargetPath: "performer[0].actor",
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
			Fields: []MappingRow{
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
					SourcePath:    "statusCode",
					FallbackPaths: []string{"statusCode"},
					LiteralValue:  "",
					Transform:     "encounter_status_to_fhir",
					TargetPath:    "status",
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
					Scope:         "entryRelationships[typeCode=COMP].entry.participants[typeCode=LOC]",
					SourcePath:    "participantRole.playingEntity.names[0].family",
					FallbackPaths: []string{"participantRole.playingEntity.code.displayName"},
					Transform:     "cda_name_or_literal_to_display_ref",
					TargetPath:    "location[0].location",
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
					Scope:      "id[*]",
					Transform:  "cda_ii_to_identifier",
					CollectAll: true,
					TargetPath: "identifier",
				},
				{
					// Encounter.participant (performer) -- the <performer>
					// element (CDAEntry.Performers/CDAAssignedEntity) is
					// structurally distinct from <participant>
					// (CDAEntry.Participants/CDAParticipantRole) and was never
					// read by Go's encounter_mapper.go either (also confirmed
					// absent). Mirrors CareTeamMappingRules' emitted-
					// Practitioner pattern exactly, just sourced from
					// assignedEntity.* instead of participantRole.* -- the
					// performing clinician becomes a real, referenceable
					// Practitioner resource instead of being silently dropped.
					Scope:      "performers[*]",
					CollectAll: true,
					TargetPath: "participant",
					Fields: []MappingRow{
						{SourcePath: "typeCode", Transform: "encounter_participant_type_coding", TargetPath: "type[0]"},
						{
							EmitAsResource: "Practitioner",
							TargetPath:     "individual",
							Fields: []MappingRow{
								{Scope: "assignedEntity.assignedPerson.names[*]", Transform: "cda_name_to_fhir", CollectAll: true, TargetPath: "name"},
								{
									Scope:            "assignedEntity.ids[*]",
									Transform:        "cda_ii_to_identifier",
									CollectAll:       true,
									TargetPath:       "identifier",
									EmbedCDAIdentity: true,
								},
								{SourcePath: "assignedEntity.code", Transform: "cda_code_to_codeable_concept", TargetPath: "qualification[0].code"},
								{Scope: "assignedEntity.telecoms[*]", Transform: "cda_telecom_to_fhir", CollectAll: true, TargetPath: "telecom"},
							},
						},
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
					Scope:      "components[*]",
					CollectAll: true,
					TargetPath: "_emitOnly",
					Fields: []MappingRow{
						{
							EmitAsResource: "Practitioner",
							TargetPath:     "ref",
							Fields: []MappingRow{
								{
									Scope:      "performers[0].assignedEntity.assignedPerson.names[*]",
									Transform:  "cda_name_to_fhir",
									CollectAll: true,
									TargetPath: "name",
								},
								{
									Scope:            "performers[0].assignedEntity.ids[*]",
									Transform:        "cda_ii_to_identifier",
									CollectAll:       true,
									TargetPath:       "identifier",
									EmbedCDAIdentity: true,
								},
								{
									SourcePath: "performers[0].assignedEntity.code",
									Transform:  "cda_code_to_codeable_concept",
									TargetPath: "qualification[0].code",
								},
								{
									Scope:      "performers[0].assignedEntity.telecoms[*]",
									Transform:  "cda_telecom_to_fhir",
									CollectAll: true,
									TargetPath: "telecom",
								},
							},
						},
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
// 2026-06-22 IG verification (architecture/CDA_FHIR_MAPPING_INVENTORY.md
// section 13) resolved both previously-flagged discrepancies as non-bugs:
// the LOINC code `48768-6` is correct for this project's R2.1 target
// (CONF:1198-19160; `52556-8` belongs to a 2024+ companion-guide revision),
// and CDA's statusCode is unconditionally fixed to "completed"
// (CONF:1198-19094) regardless of real coverage status, so there is no
// CDA-side signal `status` could be derived from instead of the hardcoded
// "active" Go already uses.
//
// `beneficiary`/`subscriber` (Phase 4 Slice A, closed): both set via
// PatientRefPath below — coverage_mapper.go:43-44 assigns the SAME
// patientRef to both fields, the reason PatientRefPath is a slice rather
// than a single string.
//
// Deliberately NOT ported: an actual subscriber-relationship code even when
// one might be present (coverage_mapper.go never reads one either —
// hardcodes "self" unconditionally, a candidate gap the inventory flags but
// doesn't fix here, since fixing it isn't this slice's job).
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
					SourcePath: "code",
					Transform:  "coverage_type_to_codeable_concept",
					TargetPath: "type",
				},
				{
					SourcePath: "effectiveTime",
					Transform:  "cda_timerange_to_period",
					TargetPath: "period",
				},
				{
					// coverage_mapper.go:68-98 -- payer org display, two
					// tiers: outer HLD/PRF participant's scoping-entity
					// description, else the COMP policy-activity's nested
					// performer's organization name. Simplification: Go's
					// second tier scans ALL performers for the first with a
					// non-empty org name; this uses performers[0] only --
					// 0/4 corpus evidence either way.
					Scope: "participants[typeCode=HLD|PRF].participantRole.scopingEntity.desc",
					ScopeFallbacks: []string{
						"entryRelationships[typeCode=COMP].entry.performers[0].assignedEntity.representedOrganization.names[0]",
					},
					TargetPath: "payor[0].display",
				},
				{
					// us-core-coverage requires payor (1..1) -- same
					// SkipIfResourceHasAnyOf placeholder idiom used
					// throughout this session (coverage_mapper.go:100-102).
					LiteralValue:           "Unknown",
					TargetPath:             "payor[0].display",
					SkipIfResourceHasAnyOf: []string{"payor"},
				},
				{
					// coverage_mapper.go:104-114 -- always hardcoded "self".
					LiteralValue: map[string]interface{}{
						"coding": []interface{}{
							map[string]interface{}{
								"system":  "http://terminology.hl7.org/CodeSystem/subscriber-relationship",
								"code":    "self",
								"display": "Self",
							},
						},
					},
					TargetPath: "relationship",
				},
				{
					// coverage_mapper.go:117-131 -- COMP policy-activity
					// entry's first identifier, Extension preferred over
					// Root. Simplification: Go scans ALL ids for the first
					// with a non-empty Root or Extension; this uses id[0]
					// only -- 0/4 corpus evidence either way.
					Scope:         "entryRelationships[typeCode=COMP].entry",
					SourcePath:    "id[0].extension",
					FallbackPaths: []string{"id[0].root"},
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
// Deliberately NOT ported: addresses (`a.AssignedAuthor.Addresses` exists
// in the typed struct but patient_mapper.go's MapAuthor never reads it — a
// confirmed gap in Go itself, not a disagreement with it; adding
// Practitioner.address[] here would be new functionality, not a port);
// `AssignedAuthoringDevice` (device-authored entries, explicitly skipped by
// Go too — `if AssignedPerson == nil { continue }` already excludes them via
// firstAuthorWithPerson's identical check); `RepresentedOrganization` (no
// Organization/PractitionerRole link is produced anywhere in this codebase
// for authors — a confirmed, well-evidenced gap, not fixed here either).
//
// `_cdaIds` (Phase 4 Slice A, closed): the identifier row below now sets
// EmbedCDAIdentity, mirroring patient_mapper.go:160's
// embedCDAIds(p, a.AssignedAuthor.Ids) exactly.
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
// CDAAssignedEntity shape (name/identifier/telecom/address) is captured —
// unlike Author's rule above, which deliberately omits address because
// patient_mapper.go's MapAuthor never reads it.
//
// Composition.attester wiring (FHIR R4: mode="legal", time, party -> this
// Practitioner) is deferred to Phase 4: composition_mapper.go's
// CompositionMapper builds Composition from document_mapper.go's LIVE
// resource list today, which this declarative rule is not part of yet —
// same dormant-until-cutover status as every other rule in this file.
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
			},
		},
	}
}
