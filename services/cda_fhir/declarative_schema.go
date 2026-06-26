// services/cda_fhir/declarative_schema.go
//
// Phase 2 of the CDA→FHIR Declarative Mapping Engine (see
// architecture/CDA_FHIR_MAPPING_ENGINE_SPRINT_PLAN.md, "Phase 2" design
// note) — the mapping-row schema. Two levels, because "which CDA entries
// become which FHIR resource type" and "which nested value feeds which FHIR
// field" are different questions, each with its own conditionality
// primitive; collapsing them into one flag (as a flat row shape would force)
// can't express Medications' moodCode-driven MedicationRequest/Statement
// split or the Free-Text-Sig/Instruction(V2) discriminator without one or
// the other becoming a hardcoded special case.
package cdafhir

// MappingRule selects which CDA entries in one section become one FHIR
// resource type. Multiple rules can target the same SectionKey with
// different EntryMatch + FHIRResource — e.g. Medications:
// EntryMatch="moodCode=INT" → MedicationRequest, then a second rule with
// EntryMatch="" → MedicationStatement. Rules are evaluated in slice order;
// for a given entry, the first rule whose EntryMatch matches wins — the
// same exclusivity medication_mapper.go's `if moodCode == "INT" {...} else
// {...}` already expresses in Go, made declarative.
type MappingRule struct {
	SectionKey   string `json:"sectionKey,omitempty"` // "allergiesAndIntolerances"
	FHIRResource string `json:"fhirResource"`         // "AllergyIntolerance"

	// HeaderPath, when set, makes this a document-header rule instead of a
	// section-entries rule: BuildHeaderResource (not BuildResources/
	// BuildResourcesForRules) resolves it against documentMap["header"]
	// rather than iterating "sectionsByKey.<SectionKey>.entries[*]" — every
	// other field on this struct (EntryMatch, FlattenOrganizers,
	// RequiredPaths, Fields) still applies the same way once the one
	// header sub-object is found. Exactly one of SectionKey or HeaderPath
	// should be set per rule.
	//
	// Added for Author/Custodian: patient_mapper.go's MapAuthor/MapCustodian
	// are called directly from document_mapper.go before the section-key
	// dispatch loop even starts (doc.Header.Authors/doc.Header.Custodian),
	// not through any SectionKey — the first construct this engine has
	// needed that isn't a repeating section entry at all. Two values are
	// meaningful today: "custodian" (a single object,
	// documentMap["header"]["custodian"]) and "authors" (a special case --
	// see BuildHeaderResource's firstAuthorWithPerson, since "the first
	// author with a usable assignedPerson" is an existence check the Phase 1
	// bracket-predicate grammar can't express, only equality).
	HeaderPath string `json:"headerPath,omitempty"`

	// EntryMatch is a Phase 1 predicate clause with no surrounding brackets,
	// e.g. "moodCode=INT" or "classCode=ACT,moodCode=EVN" — passed straight
	// into "sectionsByKey.<SectionKey>.entries[<EntryMatch>]". Empty means
	// every entry in the section matches this rule.
	EntryMatch string `json:"entryMatch,omitempty"`

	// FlattenOrganizers: when true, BuildResources expands any matched entry
	// whose entryType is "organizer" with non-empty Components into those
	// Components — one resource per component, with the organizer itself
	// producing none — instead of treating the organizer as one resource.
	// Mirrors observation_mapper.go's flattenObservationEntries exactly
	// (Vital Signs/Results/Social History panel organizers: Vital Signs
	// Organizer .4.26, Result Organizer .4.1, both wrapping individual
	// Vital Sign/Result Observations .4.27/.4.2 as Components — confirmed in
	// real Kareo corpus data, a 5-component Vital Signs Organizer). A
	// non-organizer entry, or an organizer with zero Components, passes
	// through unchanged — exactly Go's `len(e.Components) > 0` gate.
	//
	// Currently only honored by BuildResources, not BuildResourcesForRules —
	// no section ported so far needs both organizer-flattening AND
	// moodCode-style multi-rule dispatch on the same SectionKey; adding it
	// to BuildResourcesForRules too is straightforward if that combination
	// is ever needed, not done speculatively here.
	FlattenOrganizers bool `json:"flattenOrganizers,omitempty"`

	// RequiredPaths: after Fields finishes, the built resource is discarded
	// entirely (same as the "len(resource) <= 1" check below) unless EVERY
	// one of these top-level keys ended up present. Deliberately top-level-
	// key-only, no nested path parsing — every current use case only needs
	// this. Added for CareTeam: careteam_mapper.go's buildCareTeamResource
	// returns nil outright when zero usable participants were found, even
	// though status/category/period rows would otherwise still populate the
	// resource — RequiredPaths:["participant"] replicates that gate
	// declaratively instead of as a hardcoded post-hoc Go check.
	RequiredPaths []string `json:"requiredPaths,omitempty"`

	// SkipIfCodeNullFlavor skips building a resource for this entry entirely
	// — before any Fields row runs, same as never having matched at all —
	// when the entry's top-level "code" is a bare nullFlavor with no real
	// code (code.nullFlavor != "" && code.code == ""). Mirrors
	// observation_mapper.go:195-198's explicit early return ("If no
	// substitution resolved the code, skip — no useful clinical data to
	// emit"), found via real corpus data: practicefusion_sample.xml's
	// "results" section has exactly this shape (a Results Organizer whose
	// one Component is fully nullFlavor — code, value, and effectiveTime all
	// NI), and without this check the engine's generic "set dataAbsentReason
	// when no value[x] exists" row (us-core-2) turns that placeholder into a
	// spurious Observation Go deliberately never emits.
	//
	// Deliberately NOT ported: observation_mapper.go:178-194's COMP-
	// entryRelationship substitution, which looks for a nested child entry
	// with a real code/value/time and swaps it in before this same check
	// (the AUDIT-C/SDOH shell-entry pattern its own comment describes). No
	// file in this 4-vendor corpus exercises that nested shape — a narrow,
	// documented gap, not a guessed-at one.
	SkipIfCodeNullFlavor bool `json:"skipIfCodeNullFlavor,omitempty"`

	// SkipEmptyCheck bypasses the "len(resource) <= 1" discard check in
	// buildOneResource, keeping the resource even when zero Fields rows
	// produced any value. Exists for exactly one case found while building
	// Phase 4's document-level orchestrator: patient_mapper.go's MapAuthor
	// pre-populates its resource map with resourceType+id BEFORE running any
	// field logic, then returns it unconditionally once
	// `len(person.Names) > 0` (a length check on the SOURCE data, not a
	// content or emptiness check on the BUILT resource) — unlike
	// practitioner_mapper.go's MapPractitioners (CareTeam's emitted
	// Practitioner), which DOES gate on `len(r) > 2` and so needs no such
	// bypass. Real corpus data exercises this: kareo_sample.xml's one
	// document author has a structurally-present but textually-empty name
	// (no given/family/text) and a nullFlavor identifier — Go still emits a
	// placeholder Practitioner for it; without this flag the declarative
	// engine's (stricter, content-based) empty check would correctly-by-its-
	// own-logic-but-incorrectly-for-parity discard it, producing 0 resources
	// where MapDocument() produces 1.
	SkipEmptyCheck bool `json:"skipEmptyCheck,omitempty"`

	// PatientRefPath lists the FHIR field name(s) this resource type uses to
	// link back to the patient — grepped from every typed Go mapper's own
	// `if patientRef != "" { r[...] = ref(patientRef) }` to get the exact
	// field name per resource type, not assumed from memory: "subject"
	// (Condition/Encounter/Observation/Procedure/Goal/ServiceRequest/
	// MedicationRequest/MedicationStatement/CareTeam/DeviceUseStatement),
	// "patient" (AllergyIntolerance/Immunization/FamilyMemberHistory),
	// "deliverFor" (SupplyRequest), or both "beneficiary" AND "subscriber"
	// (Coverage — coverage_mapper.go:43-44 sets both to the same patientRef,
	// the reason this is a slice and not a single string). Empty/nil for
	// resources with no patient link at all (Practitioner, Organization,
	// CareTeam's emitted Practitioner). Every typed Go mapper takes a
	// patientRef parameter and sets the right-named field itself; no
	// MappingRule did anything equivalent before this field existed — every
	// *MappingRules() function explicitly documented this as a deliberate,
	// deferred gap ("patientRef-dependent, consistent with every prior
	// rule"). When set and a non-empty patientRef is assigned to
	// DeclarativeEngine.PatientRef, the engine sets
	// resource[path] = {"reference": patientRef} for every path here, on
	// every kept resource — generic, not authored per-row, since the value
	// is always the same external string, never derived from CDA data.
	//
	// Not used by Appointment (plan_of_care_mapper.go's
	// buildPlannedAppointmentResource writes patientRef into
	// participant[0].actor, a nested array element this single-path
	// primitive can't express) — deliberately left as a documented gap, not
	// a silently-dropped one; see PlanOfCareMappingRules' doc comment.
	PatientRefPath []string `json:"patientRefPath,omitempty"`

	Fields []MappingRow `json:"fields"`
}

// MappingRow is one field-level mapping within an already-selected entry.
// "Which row applies" (Free-Text-Sig vs Instruction(V2), distinguished only
// by a nested templateId) needs no separate boolean gate: if Scope resolves
// to zero nodes, the row silently contributes nothing — exactly as if the
// source CDA document simply lacked that entryRelationship.
type MappingRow struct {
	// Scope is a Phase 1 path, relative to the matched entry, narrowing the
	// root that SourcePath/Condition.WhenPath resolve against. Empty means
	// the entry itself is the root. Example:
	// "entryRelationships[typeCode=SUBJ].entry" reaches the nested
	// assertion observation an outer Allergy Concern Act wraps.
	Scope string `json:"scope,omitempty"`

	// ScopeFallbacks are additional Scope candidates tried in order, via the
	// same resolution Scope itself uses, when Scope resolves to zero nodes —
	// the Scope-level counterpart to FallbackPaths below. "" within this list
	// means "the matched entry itself" (the same meaning Scope=="" already
	// has). This is Phase 3's generalization of the SUBJ→REFR→flat-structure
	// fallback chain allergy_mapper.go/condition_mapper.go's
	// findRelByTypeCode(SUBJ)-then-REFR-then-&entry idiom already expresses
	// in Go (e.g. condition_mapper.go:68-74) — Scope alone can't express an
	// "or" between two different candidate roots, only FallbackPaths (a
	// SourcePath-level concern, evaluated relative to one already-fixed
	// Scope) could, so this is a distinct, narrow addition rather than reuse.
	ScopeFallbacks []string `json:"scopeFallbacks,omitempty"`

	// SourcePath is a Phase 1 path, relative to the Scope root, to the raw
	// value. Ignored when LiteralValue is set and SourcePath=="".
	SourcePath string `json:"sourcePath,omitempty"`

	// FallbackPaths are additional SourcePath alternatives tried in order
	// via ResolveCDAFallback when SourcePath itself resolves empty — the
	// "field A, else field B" primitive (e.g. Allergy's CSM-participant-code
	// falling back to the assertion observation's own value). When every
	// path (SourcePath + all of FallbackPaths) resolves empty and
	// LiteralValue is set, LiteralValue is used as the final, literal
	// fallback — e.g. MedicationRequest.requester's "performer, else author,
	// else a non-empty placeholder display" (us-core-21 requires this field
	// is never empty; requesterReference in medication_mapper.go ends its own
	// fallback chain the same way, with a literal display string).
	FallbackPaths []string `json:"fallbackPaths,omitempty"`

	// LiteralValue is used instead of path resolution when SourcePath=="",
	// or as the final fallback after FallbackPaths is exhausted (see above).
	// Lets a row express a constant output (e.g. "confirmed") that only a
	// Condition ever overrides — see RowCondition.
	LiteralValue interface{} `json:"literalValue,omitempty"`

	// CollectAll: when true, every node Scope matches (not just the first)
	// contributes one array element at TargetPath, instead of only the
	// first scoped node being used. The "collect all, not first match"
	// primitive Medication's RSON→reasonCode[] needs
	// (indicationReasonCodes in medication_mapper.go).
	CollectAll bool `json:"collectAll,omitempty"`

	// Fields, when set alongside CollectAll, turns each Scope match into ONE
	// sub-object instead of one scalar value: every child row in Fields is
	// applied (recursively, via the same applyRow) against that one matched
	// node, building a fresh map written whole as one TargetPath[i] element.
	// This row's OWN SourcePath/Transform/ValueMap/Condition/ValueSetURL are
	// ignored when Fields is set — they describe a single value, which isn't
	// what this row produces anymore.
	//
	// Added for Allergy reactions: AllergyIntolerance.reaction[] needs BOTH
	// manifestation and severity per MFST match, and two independent
	// top-level CollectAll rows can't guarantee staying aligned (if one row's
	// transform skips an item the other doesn't — e.g. a reaction with no
	// Severity Observation, a real [0..1] per Reaction Observation V2 —
	// reaction[i].severity would silently end up describing a different
	// reaction than reaction[i].manifestation). Building one sub-object per
	// match in one pass makes misalignment structurally impossible instead
	// of needing the two passes to be kept in sync. A child row whose own
	// Scope (optionally with ScopeFallbacks) resolves to nothing simply
	// omits that key from the sub-object — normal [0..1]/[0..*] optionality,
	// not an error; the sub-object only needs ONE child row to succeed to be
	// kept (e.g. manifestation alone keeps a reaction with no severity).
	Fields []MappingRow `json:"fields,omitempty"`

	// EmitAsResource, when set on a child row inside another row's Fields
	// (i.e. a row nested under a CollectAll+Fields parent), changes what that
	// child row builds: instead of writing values into the shared sub-object
	// its siblings populate, it builds a BRAND NEW, independent resource
	// (resourceType=EmitAsResource, a synthesized "id") from ITS OWN nested
	// Fields against the same scoped node, then writes a FHIR Reference
	// ({"reference": "EmitAsResource/id"}) at this row's own TargetPath in
	// the shared sub-object instead. The emitted resource itself is returned
	// alongside the rule's main resource (see BuildResources/
	// BuildResourcesForRules), not nested inside it.
	//
	// Added for CareTeam: careteam_mapper.go's MapCareTeam builds one
	// Practitioner resource PER care-team-member participant, cross-
	// referenced from that same participant's CareTeam.participant[].member
	// — a "one matched entry produces multiple, cross-referenced top-level
	// resources" shape no row in this engine could express before (every
	// prior row either writes one value or one sub-object, always staying
	// inside the ONE resource the matched entry is building). Only
	// meaningful one level deep — a child row using EmitAsResource whose own
	// Fields contains another EmitAsResource row is not a case any current
	// section needs and is not supported.
	EmitAsResource string `json:"emitAsResource,omitempty"`

	// EmitAsResourcePatientRefPath mirrors MappingRule.PatientRefPath but for
	// the resource EmitAsResource builds: the engine's main per-rule
	// PatientRefPath application (buildOneResource) never sees this emitted
	// resource, since it's returned as a sibling "extra" resource, not the
	// rule's own — so a resource type that genuinely needs a subject/patient
	// reference (e.g. Condition, unlike CareTeam's emitted Practitioner,
	// which needs none) must say so here instead. Empty/nil for emitted
	// resources with no patient link.
	EmitAsResourcePatientRefPath []string `json:"emitAsResourcePatientRefPath,omitempty"`

	// TargetPath is the FHIR-side path within the resource (passed to
	// setFHIRPath), e.g. "verificationStatus.coding[0].code".
	TargetPath string `json:"targetPath"`

	// Transform is a DeclarativeTransformRegistry name; "" means the
	// resolved value is written as-is (after ValueMap, if any).
	Transform string            `json:"transform,omitempty"`
	ValueMap  map[string]string `json:"valueMap,omitempty"`

	// ValueSetURL, when set, checks the final output code against
	// fhir/r4.TerminologyRegistry's compiled ValueSet membership table — a
	// capability the dormant engine never had (it only ever did
	// cda_terminology's offline format check, a different question).
	ValueSetURL string `json:"valueSetUrl,omitempty"`

	// Condition evaluates within this already-matched row, picking an
	// alternate output based on a runtime field value — distinct from
	// Scope/EntryMatch, which decide whether the row applies at all.
	Condition *RowCondition `json:"condition,omitempty"`

	Required    bool   `json:"required,omitempty"`
	Conformance string `json:"conformance,omitempty"` // "SHALL" | "SHOULD" | "MAY"

	// SkipIfResourceHasAnyOf: this row is skipped entirely (no-op, same as a
	// Scope that matches nothing) if the resource being built already has
	// ANY of these top-level keys set by an earlier row. Unlike every other
	// conditionality primitive on this struct, this checks the OUTPUT
	// resource's accumulated state so far, not this row's own input value —
	// a genuinely different question ("did some OTHER row already satisfy
	// this requirement") that Scope/Condition/FallbackPaths, all scoped to
	// this row's own data, can't express.
	//
	// Added for Observation's us-core-2 invariant (must carry value[x],
	// component, hasMember, or dataAbsentReason): observation_mapper.go's
	// hasObservationValue check, evaluated only AFTER every value[x] row had
	// its chance to fire. Put the dataAbsentReason row last in Fields with
	// SkipIfResourceHasAnyOf listing every value[x] TargetPath this section's
	// rule actually writes, and it fires only when none of them did — same
	// net effect as Go's `if !hasObservationValue(r) { r["dataAbsentReason"]
	// = ... }`, expressed as data instead of a post-hoc Go check.
	SkipIfResourceHasAnyOf []string `json:"skipIfResourceHasAnyOf,omitempty"`

	// EmbedCDAIdentity: when true on a CollectAll row (no Fields), the raw
	// {root, extension} pairs this row's Scope resolves -- BEFORE Transform
	// is applied -- are also written to resource["_cdaIds"], in addition to
	// this row's normal TargetPath write. Mirrors mappers/cda_ids.go's
	// embedCDAIds(resource, ids) exactly (same {"root":..., "extension":...}
	// shape, same "skip zero-value ids" filtering), reusing this row's
	// already-resolved Scope nodes instead of re-deriving the CDA II list a
	// second time.
	//
	// Added for Phase 4 Slice A: grepping every Go mapper found embedCDAIds
	// is called from exactly 3 places -- patient_mapper.go's MapAuthor
	// (assignedAuthor.Ids) and MapCustodian (org.Ids), and
	// practitioner_mapper.go's buildPractitionerResource (ParticipantRole.Ids,
	// reused live by careteam_mapper.go for CareTeam's emitted Practitioner).
	// Every other section's mapper (Allergy/Condition/Medication/Observation/
	// etc.) never calls it -- _cdaIds-based cross-section dedup has never
	// applied to clinical resources, only to Practitioner/Organization
	// resources built from document-header/CareTeam-participant data. This
	// field is therefore only ever set on AuthorMappingRules/
	// CustodianMappingRules/CareTeamMappingRules' identifier rows -- adding
	// it anywhere else would make declarative output dedup MORE than Go ever
	// did, a real behavior change beyond parity, not a gap fix.
	EmbedCDAIdentity bool `json:"embedCDAIdentity,omitempty"`
}

// RowCondition evaluates WhenPath (relative to the row's own Scope root)
// and, when its resolved value stringifies to Equals, overrides this row's
// output for this one write. Equality only — deliberately not a general
// expression language, per the sprint plan's Performance checklist
// ("not a general expression evaluator").
//
// This is the negationInd→refuted primitive: the row's own
// LiteralValue/Transform produce "confirmed"; WhenPath="negationInd",
// Equals="true", ThenLiteralValue="refuted" produce the flip, as data
// instead of an `if` duplicated in allergy_mapper.go/condition_mapper.go.
type RowCondition struct {
	WhenPath string `json:"whenPath,omitempty"`

	// WhenPaths are additional candidate paths checked the same way as
	// WhenPath (OR semantics — the condition fires if ANY of WhenPath plus
	// WhenPaths equals Equals). Phase 3 addition: allergy_mapper.go/
	// condition_mapper.go's negation check is actually
	// `entry.NegationInd || allergyObs.NegationInd` — the OUTER act's own
	// negationInd OR'd with whichever inner observation Scope resolved to
	// (SUBJ, REFR, or the entry itself) — a real OR a single WhenPath can't
	// express, since Condition only ever sees the row's own Scope root, not
	// the original entry. Rows needing this OR use Scope="" (the entry
	// itself) precisely so WhenPath/WhenPaths can each independently reach
	// into a different candidate location from that one shared root.
	WhenPaths        []string          `json:"whenPaths,omitempty"`
	Equals           string            `json:"equals"`
	ThenLiteralValue interface{}       `json:"thenLiteralValue,omitempty"`
	ThenTransform    string            `json:"thenTransform,omitempty"`
	ThenValueMap     map[string]string `json:"thenValueMap,omitempty"`
	ThenTargetPath   string            `json:"thenTargetPath,omitempty"` // "" = same TargetPath as the row's default
}
