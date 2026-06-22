// services/cda_fhir/declarative_transform_registry.go
//
// DeclarativeTransformRegistry — named, value-in/value-out transform dispatch
// for Phase 2's engine. NOT cda_transform_registry.go: see the Phase 2 design
// note in architecture/CDA_FHIR_MAPPING_ENGINE_SPRINT_PLAN.md for why that
// registry's (entry, fieldKey, vm) flat-extraction convention and its
// not-IG-verified built-in value maps are the wrong reuse target. Most
// transforms here are thin adapters: re-marshal the resolved value (already
// JSON-shaped, since it came from the Phase 1 path resolver) into the exact
// cdadocument.* struct the corresponding services/cda_fhir/transforms
// function expects, call it, return the result — zero duplication of
// Phase-0-verified business logic.
package cdafhir

import (
	"encoding/json"
	"fmt"
	"strings"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// DeclarativeTransformFn converts one resolved Phase-1 value into a FHIR-
// shaped value. vm is the mapping row's ValueMap; nil when absent.
// Returns nil, nil for an empty/absent value — the caller skips the write,
// not an error.
type DeclarativeTransformFn func(value interface{}, vm map[string]string) (interface{}, error)

// DeclarativeTransformRegistry holds all named transforms Phase 2's engine
// can dispatch to by name. Safe for concurrent use (read-only after construction).
type DeclarativeTransformRegistry struct {
	transforms map[string]DeclarativeTransformFn
}

// NewDeclarativeTransformRegistry creates and populates the registry.
func NewDeclarativeTransformRegistry() *DeclarativeTransformRegistry {
	r := &DeclarativeTransformRegistry{transforms: make(map[string]DeclarativeTransformFn)}
	r.registerBuiltins()
	return r
}

// Apply dispatches to the named transform. An empty name is passthrough
// (the resolved value is returned unchanged) — most MappingRows need no
// transform at all (e.g. Free Text Sig's plain string text).
func (r *DeclarativeTransformRegistry) Apply(name string, value interface{}, vm map[string]string) (interface{}, error) {
	if name == "" {
		return value, nil
	}
	fn, ok := r.transforms[name]
	if !ok {
		return nil, fmt.Errorf("declarative_transform: unknown transform %q", name)
	}
	return fn(value, vm)
}

// HasTransform returns whether a transform name is registered.
func (r *DeclarativeTransformRegistry) HasTransform(name string) bool {
	_, ok := r.transforms[name]
	return ok
}

func (r *DeclarativeTransformRegistry) register(name string, fn DeclarativeTransformFn) {
	r.transforms[name] = fn
}

func (r *DeclarativeTransformRegistry) registerBuiltins() {
	r.register("string_direct", declarativeStringDirect)
	r.register("cda_code_to_codeable_concept", declarativeCodeToCodeableConcept)
	r.register("cda_quantity_to_fhir", declarativeQuantityToFHIR)
	r.register("cda_timerange_to_onset", declarativeTimeRangeToOnset)
	// Not a re-marshal adapter — allergy_mapper.go builds this CodeableConcept
	// inline today (buildAllergyResource), so there is no existing
	// transforms.* function to wrap. A reasonable Phase 3 candidate for
	// extraction into transforms/status_transforms.go alongside its siblings.
	r.register("allergy_verification_status_to_fhir", declarativeAllergyVerificationStatus)

	// =====================================================
	// Phase 3 additions — Allergies, Medications, Conditions
	// =====================================================
	r.register("allergy_status_to_fhir", declarativeAllergyStatusToFHIR)
	r.register("allergy_type_to_fhir", declarativeAllergyTypeToFHIR)
	r.register("allergy_category_from_substance_system", declarativeAllergyCategoryFromSubstanceSystem)
	r.register("allergy_reaction_severity_to_fhir", declarativeAllergyReactionSeverityToFHIR)
	r.register("condition_status_to_fhir", declarativeConditionStatusToFHIR)
	r.register("condition_verification_status_to_fhir", declarativeConditionVerificationStatus)
	r.register("condition_category_to_fhir", declarativeConditionCategory)
	r.register("medication_request_status_to_fhir", declarativeMedicationRequestStatusToFHIR)
	r.register("medication_status_to_fhir", declarativeMedicationStatusToFHIR)
	r.register("cda_timerange_to_period", declarativeTimeRangeToPeriod)
	r.register("cda_time_to_fhir_datetime", declarativeTimeToFHIRDateTime)
	r.register("cda_value_or_code_to_codeable_concept", declarativeValueOrCodeToCodeableConcept)
	r.register("cda_name_or_literal_to_display_ref", declarativeNameOrLiteralToDisplayRef)

	// =====================================================
	// Phase 3 additions — Vital Signs, Results, Social History
	// =====================================================
	r.register("observation_status_to_fhir", declarativeObservationStatusToFHIR)
	r.register("cda_real_to_bare_quantity", declarativeRealToBareQuantity)
	r.register("observation_category_to_fhir", declarativeObservationCategoryToFHIR)
	r.register("observation_data_absent_reason_to_fhir", declarativeObservationDataAbsentReasonToFHIR)

	// =====================================================
	// Phase 3 additions — Immunizations
	// =====================================================
	r.register("immunization_status_to_fhir", declarativeImmunizationStatusToFHIR)

	// =====================================================
	// Phase 3 additions — Encounters / Procedures
	// =====================================================
	r.register("encounter_status_to_fhir", declarativeEncounterStatusToFHIR)
	r.register("encounter_class_coding", declarativeEncounterClassCoding)
	r.register("encounter_participant_type_coding", declarativeEncounterParticipantTypeCoding)
	r.register("procedure_status_to_fhir", declarativeProcedureStatusToFHIR)

	// =====================================================
	// Phase 3 additions — CarePlan / Goal (Plan of Care)
	// =====================================================
	r.register("goal_status_to_fhir", declarativeGoalStatusToFHIR)
	r.register("service_request_status_to_fhir", declarativeServiceRequestStatusToFHIR)
	r.register("service_request_intent_from_mood", declarativeServiceRequestIntentFromMood)
	r.register("appointment_status_to_fhir", declarativeAppointmentStatusToFHIR)
	r.register("supply_request_status_to_fhir", declarativeSupplyRequestStatusToFHIR)
	r.register("cda_timerange_to_period_start", declarativeTimeRangeToPeriodStart)
	r.register("cda_timerange_to_period_end", declarativeTimeRangeToPeriodEnd)

	// =====================================================
	// Phase 3 additions — CareTeam / Practitioner
	// =====================================================
	r.register("care_team_status_to_fhir", declarativeCareTeamStatusToFHIR)
	r.register("cda_name_to_fhir", declarativeCDANameToFHIR)
	r.register("cda_ii_to_identifier", declarativeIIToIdentifier)
	r.register("cda_telecom_to_fhir", declarativeCDATelecomToFHIR)

	// =====================================================
	// Phase 3 additions — Coverage / FamilyMemberHistory / Device
	// =====================================================
	r.register("coverage_type_to_codeable_concept", declarativeCoverageTypeToCodeableConcept)
	r.register("device_use_statement_status_to_fhir", declarativeDeviceUseStatementStatusToFHIR)
	r.register("cda_value_to_fhir", declarativeCDAValueToFHIR)
	r.register("cda_name_to_family_string", declarativeCDANameToFamilyString)

	// =====================================================
	// Phase 3 additions — Author / Custodian (header-level)
	// =====================================================
	r.register("cda_address_to_fhir", declarativeCDAAddressToFHIR)
}

// remarshalInto re-marshals a Phase-1-resolved value (already JSON-shaped:
// map[string]interface{}/[]interface{}/string/bool/float64/nil) into the
// typed cdadocument struct a transforms.* function expects. This round-trip
// is exactly what cda_parser_service.go already does once for the whole
// document (json.Marshal(typedDoc) then json.Unmarshal into a map) — here
// applied per-value, in reverse, only for composite shapes a transform
// actually needs typed.
func remarshalInto(value interface{}, target interface{}) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("remarshal: %w", err)
	}
	return json.Unmarshal(raw, target)
}

// =========================================================
// Built-in transforms
// =========================================================

func declarativeStringDirect(value interface{}, vm map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	s, ok := value.(string)
	if !ok {
		s = fmt.Sprintf("%v", value)
	}
	if s == "" {
		return nil, nil
	}
	if vm != nil {
		if mapped, ok := vm[s]; ok {
			return mapped, nil
		}
	}
	return s, nil
}

func declarativeCodeToCodeableConcept(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var code cdadocument.CDACode
	if err := remarshalInto(value, &code); err != nil {
		return nil, fmt.Errorf("cda_code_to_codeable_concept: %w", err)
	}
	return transforms.CDACodeToCodeableConcept(code), nil
}

func declarativeQuantityToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var q cdadocument.CDAQuantity
	if err := remarshalInto(value, &q); err != nil {
		return nil, fmt.Errorf("cda_quantity_to_fhir: %w", err)
	}
	return transforms.CDAQuantityToFHIR(q), nil
}

func declarativeTimeRangeToOnset(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var tr cdadocument.CDATimeRange
	if err := remarshalInto(value, &tr); err != nil {
		return nil, fmt.Errorf("cda_timerange_to_onset: %w", err)
	}
	onset := transforms.CDATimeRangeToOnset(tr)
	if onset == "" {
		return nil, nil
	}
	return onset, nil
}

// declarativeAllergyVerificationStatus mirrors allergy_mapper.go's inline
// verificationStatus construction (buildAllergyResource) — see the doc
// comment on registerBuiltins for why this one isn't a re-marshal adapter.
func declarativeAllergyVerificationStatus(value interface{}, vm map[string]string) (interface{}, error) {
	code, _ := value.(string)
	if code == "" {
		return nil, nil
	}
	if vm != nil {
		if mapped, ok := vm[code]; ok {
			code = mapped
		}
	}
	return map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system":  "http://terminology.hl7.org/CodeSystem/allergyintolerance-verification",
				"code":    code,
				"display": capitalizeFirst(code),
			},
		},
	}, nil
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// =========================================================
// Phase 3 additions — Allergies, Medications, Conditions
// =========================================================

func declarativeAllergyStatusToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.AllergyStatusToFHIR(s), nil
}

func declarativeAllergyTypeToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, ok := value.(string)
	if !ok || s == "" {
		return nil, nil
	}
	return transforms.AllergyTypeToFHIR(s), nil
}

func declarativeAllergyCategoryFromSubstanceSystem(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.AllergyCategoryFromSubstanceSystem(s), nil
}

func declarativeAllergyReactionSeverityToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, ok := value.(string)
	if !ok || s == "" {
		return nil, nil
	}
	return transforms.AllergyReactionSeverityToFHIR(s), nil
}

func declarativeConditionStatusToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.ConditionStatusToFHIR(s), nil
}

// declarativeConditionVerificationStatus mirrors condition_mapper.go's inline
// verificationStatus construction (buildConditionResource) — like
// declarativeAllergyVerificationStatus, this is NOT a re-marshal adapter:
// Condition.verificationStatus uses a different CodeSystem
// ("condition-ver-status") from AllergyIntolerance's
// ("allergyintolerance-verification"), so the two can't share one transform
// despite the identical confirmed/refuted vocabulary.
func declarativeConditionVerificationStatus(value interface{}, vm map[string]string) (interface{}, error) {
	code, _ := value.(string)
	if code == "" {
		return nil, nil
	}
	if vm != nil {
		if mapped, ok := vm[code]; ok {
			code = mapped
		}
	}
	return map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system":  "http://terminology.hl7.org/CodeSystem/condition-ver-status",
				"code":    code,
				"display": capitalizeFirst(code),
			},
		},
	}, nil
}

// declarativeConditionCategory mirrors condition_mapper.go's
// conditionCategoryCC: "problem-list-item" and "health-concern" live in TWO
// DIFFERENT CodeSystems (verified against the published US Core "Problem or
// Health Concern" value set — see condition_mapper.go's own doc comment),
// not one CodeSystem with two codes, so this can't be a plain ValueMap on a
// string_direct row. The category code itself is a Go-side dispatch
// constant in the legacy mapper (passed in as a parameter); declaratively,
// it's simply the row's own LiteralValue — one MappingRule per section
// (SectionKey="problems" vs "healthConcerns"), each with its own
// LiteralValue, needs no extra parameter at all.
func declarativeConditionCategory(value interface{}, _ map[string]string) (interface{}, error) {
	code, _ := value.(string)
	system, display := "http://terminology.hl7.org/CodeSystem/condition-category", "Problem List Item"
	if code == "health-concern" {
		system, display = "http://hl7.org/fhir/us/core/CodeSystem/condition-category", "Health Concern"
	} else {
		code = "problem-list-item"
	}
	return map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system":  system,
				"code":    code,
				"display": display,
			},
		},
	}, nil
}

func declarativeMedicationRequestStatusToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.MedicationRequestStatusToFHIR(s), nil
}

func declarativeMedicationStatusToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.MedicationStatusToFHIR(s), nil
}

func declarativeTimeRangeToPeriod(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var tr cdadocument.CDATimeRange
	if err := remarshalInto(value, &tr); err != nil {
		return nil, fmt.Errorf("cda_timerange_to_period: %w", err)
	}
	return transforms.CDATimeRangeToPeriod(tr), nil
}

func declarativeTimeToFHIRDateTime(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var t cdadocument.CDATime
	if err := remarshalInto(value, &t); err != nil {
		return nil, fmt.Errorf("cda_time_to_fhir_datetime: %w", err)
	}
	dt := transforms.CDATimeToFHIRDateTime(t)
	if dt == "" {
		return nil, nil
	}
	return dt, nil
}

// declarativeValueOrCodeToCodeableConcept replicates the "prefer the
// observation's own polymorphic value, fall back to its code" idiom shared
// verbatim by medication_mapper.go's indicationReasonCodes (RSON→reasonCode)
// and condition_mapper.go's buildConditionResource (problem code) — the
// SAME idiom, just applied to different rows, so one transform serves both
// instead of two near-duplicates. value is the whole scoped CDAEntry-shaped
// node (a MappingRow with SourcePath=="" passes the scoped node itself).
func declarativeValueOrCodeToCodeableConcept(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var entry cdadocument.CDAEntry
	if err := remarshalInto(value, &entry); err != nil {
		return nil, fmt.Errorf("cda_value_or_code_to_codeable_concept: %w", err)
	}
	if entry.Value != nil {
		if cc, ok := transforms.CDAValueToFHIR(*entry.Value).(map[string]interface{}); ok {
			return cc, nil
		}
	}
	return transforms.CDACodeToCodeableConcept(entry.Code), nil
}

// declarativeNameOrLiteralToDisplayRef builds a display-only FHIR Reference
// from either a CDAName-shaped value (the common case: a performer/author's
// person name) or a plain string (the literal, non-empty placeholder a
// FallbackPaths+LiteralValue chain falls through to — see
// MappingRow.FallbackPaths' doc comment on MedicationRequest.requester).
// Mirrors mappers/procedure_mapper.go's private buildDisplayFromName, kept
// as its own small copy here rather than exported across packages for one
// caller — see cdaNameToDisplayString.
func declarativeNameOrLiteralToDisplayRef(value interface{}, _ map[string]string) (interface{}, error) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return nil, nil
		}
		return map[string]interface{}{"display": v}, nil
	case map[string]interface{}:
		var name cdadocument.CDAName
		if err := remarshalInto(v, &name); err != nil {
			return nil, fmt.Errorf("cda_name_or_literal_to_display_ref: %w", err)
		}
		display := cdaNameToDisplayString(name)
		if display == "" {
			return nil, nil
		}
		return map[string]interface{}{"display": display}, nil
	default:
		return nil, nil
	}
}

// cdaNameToDisplayString mirrors mappers/procedure_mapper.go's
// buildDisplayFromName exactly (unexported there, one caller here — not
// worth exporting across a package boundary for).
func cdaNameToDisplayString(n cdadocument.CDAName) string {
	s := ""
	for _, g := range n.Given {
		if g != "" {
			s += g + " "
		}
	}
	s += n.Family
	if s == "" {
		s = strings.TrimSpace(n.Prefix + " " + n.Suffix)
	}
	return s
}

// =========================================================
// Phase 3 additions — Vital Signs, Results, Social History
// =========================================================

func declarativeObservationStatusToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.ObservationStatusToFHIR(s), nil
}

// declarativeRealToBareQuantity mirrors observation_mapper.go's
// setObservationValue REAL case exactly: a Quantity carrying ONLY "value",
// no unit/system/code — unlike PQ's full CDAQuantityToFHIR, since a CDA
// REAL has no unit to carry. value is the already-resolved CDAValue.Real
// float64 (JSON round-tripped from *float64).
func declarativeRealToBareQuantity(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	return map[string]interface{}{"value": value}, nil
}

// declarativeObservationCategoryToFHIR mirrors observation_mapper.go's
// categorySystem/observationCategoryDisplay pair — kept as its own inline
// mirror (not an extracted transforms.* call) the same way
// declarativeConditionCategory already mirrors conditionCategoryCC: this is
// the row's own LiteralValue (the category code, constant per rule) flowing
// through a Transform, not a re-marshal of resolved CDA data. Trimmed to the
// 3 categories this phase ports (vital-signs, laboratory, social-history) —
// not the full switch's functional-status/cognitive-status/etc., which
// belong to sections not yet ported; extend this switch when those are.
func declarativeObservationCategoryToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	code, _ := value.(string)
	if code == "" {
		return nil, nil
	}
	display := code
	switch code {
	case "vital-signs":
		display = "Vital Signs"
	case "laboratory":
		display = "Laboratory"
	case "social-history":
		display = "Social History"
	}
	return map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system":  "http://terminology.hl7.org/CodeSystem/observation-category",
				"code":    code,
				"display": display,
			},
		},
	}, nil
}

// declarativeImmunizationStatusToFHIR wraps transforms.ImmunizationStatusToFHIR
// with negationInd hardcoded false: the negated/refused case ("not-done") is
// handled by the row's OWN RowCondition (WhenPath="negationInd"), which
// overrides this transform's result entirely via ThenLiteralValue +
// ThenTransform="string_direct" -- see ImmunizationMappingRules' status row.
// This adapter only ever needs to compute the NON-negated branch of Go's
// switch (completed/active->completed, aborted/cancelled->not-done,
// default->completed).
func declarativeImmunizationStatusToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.ImmunizationStatusToFHIR(s, false), nil
}

// declarativeObservationDataAbsentReasonToFHIR mirrors
// observation_mapper.go's dataAbsentReasonUnknown() exactly. Always returns
// the fixed "unknown" CodeableConcept regardless of input — the row's
// SkipIfResourceHasAnyOf gate (see declarative_schema.go) is what decides
// whether this row fires at all, not this transform.
func declarativeObservationDataAbsentReasonToFHIR(_ interface{}, _ map[string]string) (interface{}, error) {
	return map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system":  "http://terminology.hl7.org/CodeSystem/data-absent-reason",
				"code":    "unknown",
				"display": "Unknown",
			},
		},
	}, nil
}

// declarativeEncounterStatusToFHIR wraps transforms.EncounterStatusToFHIR.
func declarativeEncounterStatusToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.EncounterStatusToFHIR(s), nil
}

// declarativeEncounterClassCoding mirrors encounter_mapper.go:40-50's inline
// construction exactly, not a re-marshal-and-call-an-existing-transforms.*
// adapter -- there is no shared transforms.* function to wrap, since Go
// builds this Coding inline too. FHIR R4's Encounter.class is a single
// Coding (0..1), not a CodeableConcept/array (that change is R5-only).
// system is HARD-CODED to v3-ActCode regardless of entry.Code's own
// CodeSystem -- an inherited, documented architectural inconsistency (see
// architecture/CDA_FHIR_MAPPING_INVENTORY.md section 8: raw CDA Encounter
// Activity codes are often CPT/SNOMED, not ActEncounterCode), not something
// this port fixes. The corpus has near-zero real codes here (mostly
// nullFlavor) to verify a "correct" alternative against.
func declarativeEncounterClassCoding(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var c cdadocument.CDACode
	if err := remarshalInto(value, &c); err != nil {
		return nil, fmt.Errorf("encounter_class_coding: %w", err)
	}
	if c.Code == "" {
		return nil, nil
	}
	coding := map[string]interface{}{
		"system": "http://terminology.hl7.org/CodeSystem/v3-ActCode",
		"code":   c.Code,
	}
	if c.DisplayName != "" {
		coding["display"] = c.DisplayName
	}
	return coding, nil
}

// declarativeEncounterParticipantTypeCoding mirrors
// encounter_mapper.go:107-118's inline construction of one Encounter
// participant's type CodeableConcept from its bare typeCode string.
func declarativeEncounterParticipantTypeCoding(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	if s == "" {
		return nil, nil
	}
	return map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system": "http://terminology.hl7.org/CodeSystem/v3-ParticipationType",
				"code":   s,
			},
		},
	}, nil
}

// declarativeProcedureStatusToFHIR wraps transforms.ProcedureStatusToFHIR.
func declarativeProcedureStatusToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.ProcedureStatusToFHIR(s), nil
}

// declarativeGoalStatusToFHIR wraps transforms.GoalStatusToFHIR.
func declarativeGoalStatusToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.GoalStatusToFHIR(s), nil
}

// declarativeServiceRequestStatusToFHIR wraps transforms.ServiceRequestStatusToFHIR.
func declarativeServiceRequestStatusToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.ServiceRequestStatusToFHIR(s), nil
}

// declarativeServiceRequestIntentFromMood wraps transforms.ServiceRequestIntentFromMood.
func declarativeServiceRequestIntentFromMood(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.ServiceRequestIntentFromMood(s), nil
}

// declarativeAppointmentStatusToFHIR wraps transforms.AppointmentStatusToFHIR.
func declarativeAppointmentStatusToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.AppointmentStatusToFHIR(s), nil
}

// declarativeSupplyRequestStatusToFHIR wraps transforms.SupplyRequestStatusToFHIR.
func declarativeSupplyRequestStatusToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.SupplyRequestStatusToFHIR(s), nil
}

// declarativeTimeRangeToPeriodStart and declarativeTimeRangeToPeriodEnd
// extract one flat string field from transforms.CDATimeRangeToPeriod's
// Period-shaped result -- needed because FHIR Appointment.start/.end are
// flat top-level strings, not nested under a "period" object the way
// Encounter.period/Procedure.performedPeriod are
// (plan_of_care_mapper.go:136-143 computes the same Period map Encounters
// does, then unpacks exactly these two keys onto the resource directly).
func declarativeTimeRangeToPeriodStart(value interface{}, _ map[string]string) (interface{}, error) {
	return extractPeriodField(value, "start")
}

func declarativeTimeRangeToPeriodEnd(value interface{}, _ map[string]string) (interface{}, error) {
	return extractPeriodField(value, "end")
}

func extractPeriodField(value interface{}, key string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var tr cdadocument.CDATimeRange
	if err := remarshalInto(value, &tr); err != nil {
		return nil, fmt.Errorf("cda_timerange_to_period_%s: %w", key, err)
	}
	period := transforms.CDATimeRangeToPeriod(tr)
	if period == nil {
		return nil, nil
	}
	return period[key], nil
}

// =====================================================
// Phase 3 additions — CareTeam / Practitioner
// =====================================================

// declarativeCareTeamStatusToFHIR wraps transforms.CareTeamStatusToFHIR.
func declarativeCareTeamStatusToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.CareTeamStatusToFHIR(s), nil
}

// declarativeCDANameToFHIR wraps transforms.CDANameToFHIR — used for the
// Practitioner.name[] EmitAsResource builds (careteam_mapper.go's
// buildPractitionerResource reads p.ParticipantRole.PlayingEntity.Names).
func declarativeCDANameToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var name cdadocument.CDAName
	if err := remarshalInto(value, &name); err != nil {
		return nil, fmt.Errorf("cda_name_to_fhir: %w", err)
	}
	return transforms.CDANameToFHIR(name), nil
}

// declarativeIIToIdentifier wraps transforms.IIToIdentifier — used for the
// Practitioner.identifier[] EmitAsResource build (NPI etc).
func declarativeIIToIdentifier(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var id cdadocument.CDAII
	if err := remarshalInto(value, &id); err != nil {
		return nil, fmt.Errorf("cda_ii_to_identifier: %w", err)
	}
	return transforms.IIToIdentifier(id), nil
}

// declarativeCDATelecomToFHIR wraps transforms.CDATelecomToFHIR — used for
// the Practitioner.telecom[] EmitAsResource build.
func declarativeCDATelecomToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var tel cdadocument.CDATelecom
	if err := remarshalInto(value, &tel); err != nil {
		return nil, fmt.Errorf("cda_telecom_to_fhir: %w", err)
	}
	return transforms.CDATelecomToFHIR(tel), nil
}

// =====================================================
// Phase 3 additions — Coverage / FamilyMemberHistory / Device
// =====================================================

// declarativeCoverageTypeToCodeableConcept wraps
// transforms.CoverageTypeToCodeableConcept (CDACodeToCodeableConcept +
// coveragePayerTypeDisplay's LOINC display correction).
func declarativeCoverageTypeToCodeableConcept(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var code cdadocument.CDACode
	if err := remarshalInto(value, &code); err != nil {
		return nil, fmt.Errorf("coverage_type_to_codeable_concept: %w", err)
	}
	return transforms.CoverageTypeToCodeableConcept(code), nil
}

// declarativeDeviceUseStatementStatusToFHIR wraps transforms.DeviceUseStatementStatusToFHIR.
func declarativeDeviceUseStatementStatusToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	s, _ := value.(string)
	return transforms.DeviceUseStatementStatusToFHIR(s), nil
}

// declarativeCDAValueToFHIR wraps transforms.CDAValueToFHIR directly — a
// bare CDAValue input, distinct from declarativeValueOrCodeToCodeableConcept
// (which remarshals a whole CDAEntry and prefers Value, falling back to
// Code). family_history_mapper.go's component-observation rows ONLY ever
// read comp.Value (never comp.Code, which is the problem-type discriminator
// field, not the diagnosis itself — see CDAValueToFHIR's own doc comment
// on that distinction), so the fallback-to-code behavior would be wrong here.
func declarativeCDAValueToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var v cdadocument.CDAValue
	if err := remarshalInto(value, &v); err != nil {
		return nil, fmt.Errorf("cda_value_to_fhir: %w", err)
	}
	result := transforms.CDAValueToFHIR(v)
	// family_history_mapper.go:63-67 only accepts a map-shaped result
	// (`if ccMap, ok := cc.(map[string]interface{}); ok { cond["code"] =
	// ccMap }`) -- a free-text ("ST"/"ED") CDAValue makes CDAValueToFHIR
	// return a bare string, which Go's type assertion silently drops.
	// Replicated here rather than "improved" (writing the string anyway):
	// no corpus evidence either way, and this is exactly the kind of
	// behavior change that belongs in a deliberate follow-up, not a
	// port.
	cc, ok := result.(map[string]interface{})
	if !ok {
		return nil, nil
	}
	return cc, nil
}

// declarativeCDANameToFamilyString wraps transforms.CDANameToFHIR and
// extracts only the "family" component as a bare string — the lossy
// transform family_history_mapper.go uses for FamilyMemberHistory.name
// (a plain string in FHIR R4, not a HumanName): given name is discarded
// entirely. Flagged in the inventory as a candidate improvement, not a bug
// — ported as-is, not silently fixed, per this session's "port current Go
// behavior, document what's deliberately not improved" discipline.
func declarativeCDANameToFamilyString(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var name cdadocument.CDAName
	if err := remarshalInto(value, &name); err != nil {
		return nil, fmt.Errorf("cda_name_to_family_string: %w", err)
	}
	hn := transforms.CDANameToFHIR(name)
	if hn == nil {
		return nil, nil
	}
	family, ok := hn["family"]
	if !ok {
		return nil, nil
	}
	return fmt.Sprintf("%v", family), nil
}

// =====================================================
// Phase 3 additions — Author / Custodian (header-level)
// =====================================================

// declarativeCDAAddressToFHIR wraps transforms.CDAAddressToFHIR — used for
// Organization.address[] (custodian) and Practitioner.address[] (author),
// the first declarative rows to need this transform (every prior section
// either had no address field or didn't port it).
func declarativeCDAAddressToFHIR(value interface{}, _ map[string]string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	var addr cdadocument.CDAAddress
	if err := remarshalInto(value, &addr); err != nil {
		return nil, fmt.Errorf("cda_address_to_fhir: %w", err)
	}
	return transforms.CDAAddressToFHIR(addr), nil
}
