# CDA→FHIR Declarative Mapping Engine — Sprint Plan

**Status**: Phase 0 complete (inventory + performance baseline + 5 production bug fixes; 1 more confirmed bug fixed 2026-06-22, see below). Phase 1 complete (wildcard/predicate path resolver, +OR-list predicate values and +"entryType" predicate key 2026-06-22). Phase 2 complete (unified mapping schema + execution engine, synthetic-proof only; BuildResourcesForRules gained FlattenOrganizers support 2026-06-22; engine gained EmitAsResource + RequiredPaths, its first multi-resource-per-entry primitive, same day; engine gained HeaderPath + BuildHeaderResource, its first non-section-entry primitive, same day). Phase 3 **all 15 inventoried sections complete** (2026-06-22): Allergies/Medications/Conditions slice (2026-06-21), Allergy reactions (CollectAll+Fields) follow-up same day, Vital Signs/Results/Social History, Immunizations, Encounters/Procedures, CarePlan/Goal, CareTeam/Practitioner, Coverage/FamilyMemberHistory/Device, and Practitioner/Custodian's document-header half (Author/Custodian/legalAuthenticator) all complete. Two small deferred items remain (Medication's `effectiveDateTime` fallback, Condition's shell-entry substitution/`interpretationCode`) — see "Remaining for Phase 3" below. Next up: Phase 4 cutover planning (shadow-mode mechanics) whenever the user wants to start it.
**Owner**: Integration Team
**Last updated**: 2026-06-21
**Execution model**: Multiple sessions. Each phase below is a self-contained
sprint with its own exit criteria — read "Current State" at the top before
starting a session, update it before ending one.

---

## Current State

**Phase 0, 1, and 2 fully complete. Phase 3 started and is itself now a
multi-session, per-section-group process** (mirrored on Phase 6's own
"repeat as a short, focused session per section/group" convention, since
attempting all 15 inventoried sections' HIGH/MEDIUM rows, both
`mappers_test.go`/`cda_fhir_integration_test.go` ports, and a corpus run in
one sitting risks shallower verification per section than this project's
correctness bar — a scoping choice made explicitly with the user at the
start of this session, not discovered mid-stream).

**Phase 3, slice 1 — Allergies / Medications / Conditions — done** (same
day, 2026-06-21). This slice was chosen because it's exactly the set of
sections the Go-mapper fidelity fixes earlier in this session's Phase 0
targeted (negation→refuted, PIVL frequency→`dosage.timing.repeat`,
RSON→`reasonCode`, SIG-text/Instruction(V2) resolution, severity from nested
SUBJ observation) and Phase 2's synthetic test suite already proved
declarative-capable. Delivered:

- **Schema extensions to `declarative_schema.go`** (additive only — every
  Phase 2 test still passes unchanged), each justified by a concrete row
  this slice needed to express, not speculative future-proofing:
  - `RowCondition.WhenPaths []string` — OR semantics alongside the existing
    `WhenPath`. Needed because Allergy/Condition's real negation check is
    `entry.NegationInd || allergyObs.NegationInd` (the OUTER act's own flag
    OR'd with whichever inner SUBJ/REFR observation applies) — a real OR a
    single `WhenPath` can't express, since `Condition` only ever sees the
    row's own `Scope` root. Rows needing this use `Scope=""` (the entry
    itself) so `WhenPath`/`WhenPaths` can each independently reach a
    different candidate from that one shared root.
  - `MappingRow.ScopeFallbacks []string` — the `Scope`-level counterpart to
    `FallbackPaths`, generalizing `allergy_mapper.go`/`condition_mapper.go`'s
    `findRelByTypeCode(SUBJ)`→`REFR`→`&entry` three-tier fallback (e.g.
    `condition_mapper.go:68-74`) into data. `""` in the list means "the
    matched entry itself."
  - `resolveRowSourceValue`: when `FallbackPaths` is exhausted (all resolve
    empty) and `LiteralValue` is also set, `LiteralValue` is now used as a
    final literal fallback instead of nil — the "path, else path, else a
    non-empty placeholder" 3-tier chain `MedicationRequest.requester` needs
    (us-core-21 requires this field is never empty).
  - JSON tags added to `MappingRule`/`MappingRow`/`RowCondition` (camelCase,
    matching the rest of the codebase's JSON conventions) — needed once
    Phase 3 had to actually seed these structs as migration JSONB, not just
    construct them as Go literals in tests.
- **`cda_path_resolver.go` (Phase 1 file) gained two predicate keys** —
  `code` and `xsiType` — both found to be load-bearing while transcribing
  real Go logic, not speculative:
  - `code`: several C-CDA templates discriminate by a *fixed* observation
    code rather than (or in addition to) `templateId` — the Severity
    Observation's code is fixed to `"SEV"` (CONF:1098-19169), the only
    reliable way `allergy_mapper.go`/`condition_mapper.go` find the right
    nested SUBJ observation among possibly-several siblings. Needed its own
    matcher (`matchesCDACodeField`) since, unlike `typeCode`/`classCode`/
    `moodCode`/`inversionInd`, "code" is a nested CD/CE element, not a bare
    attribute on the node itself — same shape problem `templateId` already
    solved, reused the same pattern.
  - `xsiType`: discriminates `CDAEffectiveTimeEntry`'s duration entry
    (`IVL_TS`) from its dosing-frequency entry (`PIVL_TS`/`EIVL_TS`) —
    `medication_mapper.go`'s `applyDosageTiming` needs this and a flat
    wildcard can't express it. No special-casing needed (falls through the
    existing generic attribute-equality branch).
  - Both covered by new unit tests in `cda_path_resolver_test.go`.
- **`declarative_transform_registry.go` gained 11 new named transforms**:
  `allergy_status_to_fhir`, `allergy_type_to_fhir`,
  `allergy_category_from_substance_system`, `condition_status_to_fhir`,
  `condition_verification_status_to_fhir`, `condition_category_to_fhir`,
  `medication_request_status_to_fhir`, `medication_status_to_fhir`,
  `cda_timerange_to_period`, `cda_time_to_fhir_datetime`,
  `cda_value_or_code_to_codeable_concept` (one transform shared verbatim by
  Medication's RSON→`reasonCode` and Condition's `code` row — same "prefer
  value, fall back to code" idiom in both Go mappers), and
  `cda_name_or_literal_to_display_ref` (handles both a CDAName-shaped value
  and a plain string literal, for the requester fallback chain above). All
  but two are thin re-marshal adapters around already-IG-verified
  `transforms/*.go` functions, per the Phase 2 design note's established
  pattern; the two that aren't (`condition_verification_status_to_fhir`,
  `condition_category_to_fhir`) mirror inline Go construction the same way
  `allergy_verification_status_to_fhir` already did in Phase 2.
- **`services/cda_fhir/declarative_oob_rules.go`** — the canonical,
  single-source-of-truth `MappingRule` Go literals: `AllergyMappingRules()`
  (6 rows), `MedicationMappingRules()` (`MedicationRequest` 12 rows +
  `MedicationStatement` 11 rows, dispatched via `BuildResourcesForRules`'
  `moodCode=INT` first-match-wins), `ProblemsMappingRules()`/
  `HealthConcernsMappingRules()` (7 rows each, same shape, different
  `SectionKey` + category `LiteralValue` — two separate rules rather than
  one rule with a runtime category parameter, precisely to avoid
  reintroducing the Phase 0 "category hardcoded regardless of section" bug
  class declaratively). Allergy reactions (MFST→manifestation/severity) were
  initially deferred in this slice (the `CollectAll` index-alignment
  limitation below), then ported same day once the fix landed — see the
  "Allergy reactions follow-up" entry below. Still explicitly NOT ported
  (out of scope, not silently dropped): allergy-level (not per-reaction)
  Severity Observation — the IG allows this directly under the
  Allergy-Intolerance Observation but marks it SHOULD NOT/deprecated, and
  FHIR R4 `AllergyIntolerance` has no top-level `severity` field to put it
  in anyway (only `reaction[].severity`); MedicationStatement's
  `effectiveDateTime` fallback when `effectivePeriod` resolves empty (no
  current test exercises it; the schema has no "transform A into path A,
  else transform B into path B" primitive yet).
- **`database/migrations/V154__CDA_Declarative_Mapping_Rules.sql`** — a NEW
  table (`cda_declarative_mapping_rules`), not a patch to V149 (which seeds
  the old flat row shape and is left untouched, still serving the dormant
  `generic_mapper.go` path). Seeds the 5 rule rows above as hand-written
  JSON matching `declarative_oob_rules.go`'s Go literals exactly. Not yet
  read by any running code path — Phase 4's cutover is what wires a
  repository to this table; this phase's job was proving the rules
  correct and seedable.
- **Drift guard, not just a seed**:
  `declarative_oob_rules_migration_test.go` reads V154's `.sql` file
  straight off disk at test time, regex-extracts each INSERT's JSON, and
  deep-compares the unmarshalled `[]MappingRow` against
  `declarative_oob_rules.go`'s Go literals — any future hand-edit to one
  side without the other fails a test instead of shipping silently.
- **Test parity**: `declarative_oob_rules_test.go` ports all 11 relevant
  assertions from `mappers/mappers_test.go`
  (`TestMapAllergies_NoKnownAllergies_CodeFallsBackToAssertionValue`,
  `TestMapAllergies_NotNegated_VerificationStatusConfirmed`,
  `TestMapConditions_NegatedProblem_VerificationStatusRefuted`,
  `TestMapConditions_SeverityFromNestedSUBJObservation`,
  `TestMapConditions_ProblemListItem_UsesBaseCodeSystem`,
  `TestMapConditions_HealthConcern_UsesUSCoreCodeSystemAndCode`,
  `TestMapMedications_OrderIntent_RequesterFromPerformer`,
  `TestMapMedications_OrderIntent_RequesterFallback_NeverEmpty`,
  `TestMapMedications_PIVLFrequency_SetsDosageTimingRepeat`,
  `TestMapMedications_RSONIndication_SetsReasonCode`,
  `TestMapMedications_FreeTextSigAndInstructionV2_SetDosageTextFields`) plus
  `cda_fhir_integration_test.go`'s
  `TestMapDocument_AllergyCount_MatchesEntries`, all run against
  `DeclarativeEngine` with the SAME `cdadocument.CDAEntry` input literals
  (JSON-round-tripped, same convention `declarative_engine_test.go`
  established) and the SAME expected outputs. One deliberate, documented
  divergence found and kept (not patched away): the ported count test's
  minimal fixture (entries with only a top-level `Code`, no CSM participant,
  no observation `Value`) now produces a `Required`/`SHALL` error per entry
  for the missing `AllergyIntolerance.code` — Go's hardcoded mapper silently
  omits the field instead, since it never enforced the US Core 1..1
  requirement it's actually subject to. The declarative engine still
  returns the resource alongside the error (matching Go's resource count),
  surfacing a real conformance gap Go was quietly swallowing — a deliberate
  improvement flagged in the test itself, not a parity bug.
- **Corpus spot-check**: `declarative_oob_rules_corpus_test.go` runs all 4
  vendor samples (`cerner_sample.xml`, `kareo_sample.xml`,
  `mtuitive_sample.xml`, `practicefusion_sample.xml`) end-to-end (XML→FHIR,
  not just XML→JSON) through the ported Allergy/Medication/Condition rules.
  Zero panics, zero malformed (literal-nil-field) resources across all 12
  vendor×section combinations; spot-checked output (logged in full per
  resource) shows correctly-shaped `CodeableConcept`s, correct per-resource-
  type `CodeSystem`s (e.g. Condition's category split), and sane handling of
  vendor null-flavor idioms (Cerner's NI-flavored allergy section correctly
  yields zero resources rather than garbage). Per this project's standing
  principle, the corpus is evidence of shape/prevalence only — no vendor
  value was treated as ground truth for correctness.
- All verified via the `/go-build-check` Docker path: `go build ./...`,
  `go vet ./...`, and the full `cda/...`/`services/cda_fhir/...`/
  `services/executors/...` suites clean, `gofmt`-clean. The `fhir/r4`
  failure noted at the end of this slice was NOT a product bug — see the
  "fhir/r4 verification recipe" note below.

**Allergy reactions follow-up — CollectAll+Fields, same day (2026-06-21)**.
Two items flagged above as deferred were resolved in a follow-up
sub-session, prompted by the user, that also corrected the original design
assumption: Allergy reactions are `[0..*]` (multiple reactions per allergy
are normal, not edge-case), each with its own `[0..1]` Severity Observation
nested one level deeper — confirmed live against `ccda.online`'s rendered
C-CDA 2.1 templates, templateIds `2.16.840.1.113883.10.20.22.4.9` (Reaction
Observation V2) / `.4.8` (Severity Observation V2), corroborating the
inventory's own CONF-number citations rather than introducing new
unverified claims.

- **`MappingRow.Fields []MappingRow`** (`declarative_schema.go`) — the
  actual fix for the `CollectAll` index-alignment limitation: instead of two
  independent `CollectAll` rows trying to stay aligned (which can drift when
  one row's transform skips an item the other doesn't), a `CollectAll` row
  with `Fields` set builds ONE sub-object per `Scope` match by applying its
  child rows — recursively, via the same `applyRow` every other row goes
  through — against that one matched node. Alignment becomes structural
  (same loop iteration writes both fields into the same sub-object) instead
  of coordinated across passes. New engine method:
  `applyCollectAllWithFields` (`declarative_engine.go`). Considered and
  rejected a "shared group key" design (cross-row index coordination) as a
  band-aid on the two-pass structure that caused the bug in the first place.
- **`AllergyMappingRules()` reaction row added** — `Scope` walks
  `entryRelationships[typeCode=MFST,inversionInd=true].entry[templateId=...4.9]`
  (with the usual SUBJ-wrapper-absent `ScopeFallbacks`), `CollectAll`+`Fields`
  build each `reaction[]` entry: manifestation via the same
  `cda_value_or_code_to_codeable_concept` idiom Medication/Condition already
  share, severity via a new `allergy_reaction_severity_to_fhir` transform
  whose `Scope` walks the nested
  `entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=...4.8]`.
  `inversionInd=true` checked on both predicates — stricter than the legacy
  Go mapper (`findRelByTypeCode` only checks `typeCode`), a deliberate
  fidelity improvement.
- **Extracted, not duplicated**: `allergy_mapper.go`'s private
  `allergySeverityCode` switch (fixed mild/moderate/severe — distinct from
  `Condition.severity`'s full `CodeableConcept`) is now the exported
  `transforms.AllergyReactionSeverityToFHIR` (`status_transforms.go`);
  `allergy_mapper.go` calls it instead of its own copy. Pure extract-method —
  zero behavior change, including the known, already-flagged `255604002`
  ("Very Mild") gap (falls through to "moderate"), preserved verbatim, not
  fixed here.
- **Tests**: a generic engine-mechanics test
  (`declarative_engine_test.go`) proves the alignment fix directly — 3
  matches, the MIDDLE one missing the optional sub-field, asserting no
  cross-contamination and a correctly-packed 3-element array; a
  production-rule test (`declarative_oob_rules_test.go`) exercises
  `AllergyMappingRules()` itself with 2 reactions (one with severity, one
  without); the corpus test (`declarative_oob_rules_corpus_test.go`) gained
  an `assertWellFormedReactions` shape check. Note: the trimmed
  `cda/document/testdata/corpus/` fixtures don't happen to contain the
  richer multi-reaction-with-severity data the inventory cites (that came
  from broader Phase 0 research, not these 4 trimmed samples) — Kareo's
  corpus sample produces a single manifestation-only "No Known Allergies"
  reaction; PracticeFusion's 2 resources have none at all. The feature
  itself is proven by the two hand-built tests above, not the corpus run;
  the corpus run still adds value as an ongoing regression-safety net.
- V154 migration updated to match (`declarative_oob_rules_migration_test.go`
  drift guard still green).
- **fhir/r4 verification recipe** (no product/code/CI change): the
  `fhir/r4` test failures noted at the end of the main slice were a gap in
  ad-hoc verification, not a real bug. `docker-compose.yml` volume-mounts
  `./schemas:/app/schemas` in real operation (`.dockerignore` excludes
  `schemas/` from the Docker *build context* on purpose — it's meant to be
  mounted, not baked in); CI never runs `fhir/r4` tests at all today (only
  `services/hl7assembly/...`). Re-running with `schemas/` mounted exactly
  like docker-compose does (`docker run -v "$(pwd)/schemas:/app/schemas:ro" ...`)
  confirms `fhir/r4` passes clean — the correct recipe for future sessions
  verifying this package.

**Phase 3, slice 3 — Vital Signs / Results (Laboratory) / Social History —
done** (same day, 2026-06-21). Chosen because all three dispatch to the SAME
Go function (`observation_mapper.go`'s `MapObservations`), differing only in
a `category` string literal — the same one-mapper/different-LiteralValue
shape `ProblemsMappingRules`/`HealthConcernsMappingRules` already
established, so one `observationRule(sectionKey, categoryCode)` helper
serves all three. Delivered:

- **Two new engine primitives**, both additive, both proven necessary by
  real Kareo/PracticeFusion corpus data, not speculative:
  - `MappingRule.FlattenOrganizers` (`declarative_schema.go`) — expands an
    organizer entry's `Components` into the resource-roots instead of
    treating the organizer itself as one resource, mirroring
    `flattenObservationEntries` exactly. Confirmed load-bearing against
    Kareo's real Vital Signs Organizer (templateId `.4.26`, 5 components).
  - `MappingRow.SkipIfResourceHasAnyOf` (`declarative_schema.go`) — a row
    only fires if the resource being built does NOT already have any of the
    listed keys set by an earlier row. The us-core-2 invariant
    (`Observation` must carry `value[x]`/`component`/`hasMember`/
    `dataAbsentReason`) needs this: Go's `hasObservationValue` check runs
    only AFTER every `value[x]` row had its chance — a question about the
    OUTPUT resource's accumulated state, which every other conditionality
    primitive on `MappingRow` (scoped to the row's own input) can't answer.
- **`cda_path_resolver.go` gained one predicate key** — `type`, for
  `CDAValue.Type`'s own JSON tag (its xsi:type discriminator:
  PQ/CD/CE/CS/ST/ED/BL/INT/REAL/IVL_TS). A DIFFERENT field from `xsiType`
  (Phase 3 slice 1's addition, for `CDAEffectiveTimeEntry.XSIType` — same
  xsi:type concept on the CDA side, different Go field/JSON-tag entirely).
  Powers `Observation.value[x]`'s polymorphic dispatch
  (`observation_mapper.go`'s `setObservationValue` switch, ported as ~7
  mutually-exclusive `Scope`-gated rows since only one `CDAValue.Type` can
  ever match per entry).
- **4 new transforms**: `observation_status_to_fhir` (thin re-marshal,
  `transforms.ObservationStatusToFHIR`); `cda_real_to_bare_quantity` (a
  `{"value": ...}` Quantity with no unit, unlike PQ — mirrors Go's REAL
  case exactly); `observation_category_to_fhir` (mirrors
  `categorySystem`/`observationCategoryDisplay`'s inline construction the
  same way `declarativeConditionCategory` already does — NOT extracted,
  since those two Go functions are independently unit-tested by name in
  `mappers_test.go` and removing them would break those tests; trimmed to
  the 3 categories this slice ports, extend when `functionalStatus`/etc.
  are); `observation_data_absent_reason_to_fhir` (mirrors
  `dataAbsentReasonUnknown()` exactly).
- **`VitalSignsMappingRules`/`ResultsMappingRules`/`SocialHistoryMappingRules`**
  (`declarative_oob_rules.go`) — one shared `observationRule()` helper, 12
  rows each: code, status, effectiveTime, category, 7 polymorphic
  `value[x]` rows, 1 `dataAbsentReason` fallback row.
- **Found and fixed a real, pre-existing engine bug while running the
  corpus** (not introduced by this slice, just newly exposed by it): a
  transform returning a typed nil (`transforms.CDACodeToCodeableConcept`
  explicitly `return nil` as a `map[string]interface{}`) boxed into
  `interface{}` is NOT `== nil` — the classic Go typed-nil-in-interface
  gotcha. Every `applyRow` nil-check (`transformed == nil`) used to miss
  this, silently writing literal JSON `null` into FHIR fields instead of
  skipping the write — caught via PracticeFusion's real, fully
  `nullFlavor=NI` Results organizer producing `"valueCodeableConcept": null`
  in output. Fixed once at the shared chokepoint: new `isNilValue()` helper
  (reflect-based, checks `Chan`/`Func`/`Interface`/`Map`/`Ptr`/`Slice` kinds)
  used at both `applyRow` call sites. The corpus test's OWN
  `assertWellFormedResource` nil-check had the identical bug (never caught
  it either) — fixed there too. This was latent across every section ported
  so far (Allergy/Medication/Condition), not specific to Observations; worth
  knowing if anything looked "too clean" in earlier corpus runs.
- **Deliberately NOT ported** (documented in `declarative_oob_rules.go`'s
  own doc comment, not silently dropped):
  - Systolic+Diastolic → combined BP panel
    (`extractBloodPressurePanels`/`buildBloodPressureObservation`) — not a
    per-entry field mapping at all; it COMBINES two sibling Components into
    one resource, which this engine's per-matched-entry resource model has
    no way to express without a bespoke, vital-signs-only primitive.
    Confirmed unnecessary: the already-registered, engine-agnostic
    `assembly/rules.BPPanelSynthesisRule` re-pairs any two standalone
    Systolic(8480-6)/Diastolic(8462-4) `Observation` resources sharing a
    subject, AFTER mapping, regardless of which engine produced them — proven
    via `TestDeclarativeEngine_VitalSigns_BPPair_RecombinedByAssemblyLayer`,
    which runs this rule's own (unpaired) output through that existing
    assembly rule and confirms the same combined-panel-with-2-components
    shape Go produces directly. A deliberate division of responsibility, not
    a gap.
  - Shell-entry substitution (nullFlavor outer code, substitute code/value/
    effectiveTime from a COMP child) — `observation_mapper.go`'s own doc
    comment cites this as an SDOH/AUDIT-C-assessment idiom, a section not
    yet ported; no inventory evidence for Vital Signs/Results/Social History
    specifically, and mechanically awkward with `FallbackPaths` as-is (that
    primitive triggers on "path resolved empty," not "resolved struct's own
    field is empty" — a different question).
  - `interpretationCode` → `Observation.interpretation` — the typed
    `cda/document` parser doesn't capture an `InterpretationCode` field at
    all today; Go's own implementation actually reads a COMP child's VALUE
    as a stand-in, which the inventory itself flags as a likely structural
    gap (real Kareo data has `interpretationCode` as a direct sibling, not
    COMP-nested). Needs a parser-layer change to port correctly — out of
    scope for a declarative-rules-only slice.
  - `referenceRange`/`performer` — Go has zero implementation for either;
    nothing to port declaratively that Go itself doesn't already do.
- **Tests**: 2 new engine-mechanics tests (`FlattenOrganizers`,
  `SkipIfResourceHasAnyOf`) + 1 for the `type` predicate key +1 for the
  typed-nil fix, all in `declarative_engine_test.go`; 4 new production-rule
  tests in `declarative_oob_rules_test.go` (non-BP vital sign, the BP-pair/
  assembly-layer recombination proof, nullFlavor→dataAbsentReason, smoking
  status); 3 new corpus end-to-end tests
  (`declarative_oob_rules_corpus_test.go`) confirming us-core-2 holds for
  every real resource these rules produce across all 4 vendor samples.
- **`database/migrations/V155__CDA_Declarative_Mapping_Rules_Observations.sql`**
  — adds `flatten_organizers` column to V154's table (MappingRule-level
  fields get dedicated columns, not JSONB — same pattern as `entry_match`),
  seeds the 3 rules. New drift-guard test
  (`declarative_oob_rules_migration_v155_test.go`, separate from V154's —
  different column list, independent single-purpose regex) passed on first
  write, confirming the hand-synced JSON was transcribed correctly.
- All verified via Docker: `go build ./...`, `go vet ./...`, full `go test
  ./...` (including `fhir/r4`, schemas mounted) green, `gofmt`-clean on
  every touched file.

**Phase 3, slice 4 — Immunizations — done** (same day, 2026-06-21).
Investigating before porting (per this project's standing rule) found the
inventory's two "confirmed bug" claims for this section were no longer both
accurate:

- **"`negationInd` never read" (Phase 0 finding #10) — stale, already
  fixed.** `immunization_mapper.go:42` already calls
  `transforms.ImmunizationStatusToFHIR(entry.StatusCode, entry.NegationInd)`,
  and `mappers_test.go` already has 4 tests proving the negated/refused case
  correctly produces `"not-done"` plus `statusReason` from RSON. Fixed
  before this session; the inventory text just wasn't updated to match. No
  Go change needed here — ported the already-correct behavior as-is.
- **"performer reads the wrong field" — real, confirmed, fixed.**
  `immunization_mapper.go` read `entry.Participants[typeCode=PRF]` for the
  immunization performer. Reading `entry_parser.go` and
  `cda/document/types.go` directly confirmed `entry.Participants` is parsed
  from `<participant>` elements, while a C-CDA Immunization Activity's
  performer is a dedicated `<performer typeCode="PRF">` element, parsed into
  the completely separate `entry.Performers` field — the same field
  `medication_mapper.go`'s `requesterReference` already reads correctly.
  `entry.Participants` could never have matched real performer data. Fixed
  in `immunization_mapper.go` (this session) plus a new regression test
  (`mappers_test.go`'s
  `TestMapImmunizations_Performer_ReadFromPerformersNotParticipants`) — the
  4 existing status/statusReason tests stayed green, confirming the fix is
  isolated.

Delivered:

- **`ImmunizationMappingRules()`** (`declarative_oob_rules.go`) — 8 rows:
  status (via `RowCondition` — `WhenPath="negationInd"`, `Equals="true"`,
  `ThenLiteralValue="not-done"`, `ThenTransform="string_direct"` so the
  literal passes through unchanged instead of re-running the statusCode
  switch), statusReason, vaccineCode, occurrenceDateTime, route,
  doseQuantity, lotNumber, performer (the corrected field).
- **One new transform**: `immunization_status_to_fhir` (thin re-marshal,
  `transforms.ImmunizationStatusToFHIR(s, false)` — negationInd hardcoded
  false since the Condition handles the true case).
- **One real, narrow, evidence-free divergence from Go, documented (not
  silently introduced)**: Go only looks for an RSON refusal-reason
  relationship when `negationInd` is true
  (`TestMapImmunizations_NotNegated_RSONIgnored` exercises this guard); the
  declarative statusReason row has no primitive to gate on a field outside
  its own `Scope` root (negationInd lives on the outer entry, not reachable
  from within `entryRelationships[typeCode=RSON].entry`), and wasn't given
  one for a zero-corpus-evidence, non-conformant-data edge case (C-CDA's
  RSON templateId here is literally "Immunization Refusal Reason" — RSON on
  a non-refused immunization isn't realistic, conformant data). A new test,
  `TestDeclarativeEngine_Immunization_NotNegated_RSONNotGated_KnownDivergenceFromGo`,
  proves and documents the divergence rather than leaving it as an
  unverified comment claim.
- **Tests**: 5 production-rule tests in `declarative_oob_rules_test.go`
  (4 ported 1:1 from `mappers_test.go` + the divergence-documentation test
  above) and a corpus end-to-end test
  (`TestDeclarativeEngine_Corpus_ImmunizationsEndToEnd`) — Kareo's real
  immunization entry has `negationInd="true"`, exercising the exact case
  this slice's status row exists for; PracticeFusion's real (administered)
  flu immunization confirms the non-negated path too.
- **`database/migrations/V156__CDA_Declarative_Mapping_Rules_Immunizations.sql`**
  — same table as V154/V155, `flatten_organizers=false` (not needed here).
  New drift-guard test (`declarative_oob_rules_migration_v156_test.go`,
  reuses V155's regex — identical column order) passed on first write.
- All verified via Docker: `go build ./...`, `go vet ./...`, full `go test
  ./...` (including `fhir/r4`, schemas mounted) green, `gofmt`-clean on
  every touched file.

**Phase 3, slice 5 — Encounters / Procedures — done** (2026-06-22).
Investigating before porting found two of the inventory's flagged items for
this pair were already non-issues, and surfaced one real, justified Phase 1
schema addition:

- **Procedure body site "structurally wrong" / "no TargetSiteCode field at
  all" (inventory section 9) — stale, already fixed.** `CDAEntry` gained a
  `TargetSiteCode *CDACode` field and `procedure_mapper.go:77` has read it
  correctly (direct child of `<procedure>`, not COMP-inferred) since before
  this session, with 3 passing tests already proving it. Ported as-is.
- **Procedure performer's "dead computation" (inventory section 9)** — real
  in Go (a `CDANameToFHIR` call computed only to check non-nil, then
  discarded), but not a correctness bug (the discarded value and the one
  actually used agree on empty/non-empty for any real `CDAName`) — the
  declarative port doesn't inherit it (there is no dead computation to
  inherit) without needing a Go-side fix first.
- **A real, justified Phase 1 addition**: `cda_path_resolver.go`'s
  `cdaPredicateValueEquals` now accepts a `"|"`-separated OR-list as a
  predicate's value (e.g. `participants[typeCode=PRF|SPRF]`) — needed
  because `procedure_mapper.go:54` checks
  `p.TypeCode == "PRF" || p.TypeCode == "SPRF"` (Performer / Secondary
  Performer, both standard HL7 ParticipationType codes), a real Go `||` no
  single-value predicate could express. Backward compatible (every existing
  single-value predicate splits into a 1-element list, unchanged) and
  covered by 2 new tests in `cda_path_resolver_test.go`.

Delivered:

- **`EncounterMappingRules()`** (6 rows: status, class — a single Coding,
  not CodeableConcept, per FHIR R4; type from the same code; period;
  participant via `CollectAll+Fields` on `"participants[*]"`; location via
  `SourcePath`+`FallbackPaths`, reusing `cda_name_or_literal_to_display_ref`'s
  string-wrapping branch instead of a new transform) and
  **`ProcedureMappingRules()`** (5 rows: status, code, performedPeriod,
  performedDateTime gated by `SkipIfResourceHasAnyOf:
  ["performedPeriod"]`, performer via the new OR-list predicate, bodySite).
  Both `EntryMatch=""` — both Go mappers read every section entry uniformly,
  no `EntryType` branching to mirror.
- **Found and fixed a real engine bug while writing the participant
  row**: `Scope: "participants"` (no brackets) resolves to the array *as
  one node*, not a fan-out — only `"[*]"`/predicate brackets spread into
  each element (`ResolveCDAPaths`' `segmentWildcard`/`segmentPredicate`
  cases vs. its plain-key default). Every CollectAll Scope written so far
  happened to use a predicate bracket (which already fans out correctly);
  this slice's *unfiltered* participant scope was the first to need a bare
  wildcard, and writing it as a plain key silently produced zero
  participants with zero errors. Fixed by using `"participants[*]"`;
  documented inline on the row itself so the next unfiltered-CollectAll row
  doesn't repeat it.
- **Four new transforms**: `encounter_status_to_fhir`,
  `procedure_status_to_fhir` (thin wraps), `encounter_class_coding` and
  `encounter_participant_type_coding` (mirror Go's own inline Coding/
  CodeableConcept construction directly — no existing `transforms.*`
  function to wrap, since Go builds these inline too).
- **One real, narrow, evidence-free divergence from Go, documented**: Go's
  location lookup overwrites `r["location"]` on every COMP/LOC match across
  possibly-multiple COMP relationships (last match wins); this engine's
  non-CollectAll Scope resolution takes the first match
  (`scopedNodes[0]`). 0/4 corpus has a Location at all, so there's no real
  document to validate either ordering against.
- **Tests**: 8 production-rule tests (2 ported 1:1 from `mappers_test.go`'s
  Encounter display-name guard, 3 ported 1:1 from its Procedure body-site
  suite, 3 new — participant, location, performer — since the inventory
  notes zero dedicated tests existed for those in Go) plus 2 corpus
  end-to-end tests (`TestDeclarativeEngine_Corpus_EncountersEndToEnd`/
  `_ProceduresEndToEnd` — Kareo and PracticeFusion both have real, if
  minimal, Procedure data; 0/4 corpus has an Encounters section at all,
  consistent with the inventory's "entries present but nullFlavor" note for
  the vendors that DO have encounters, none of which are in this 4-file
  corpus).
- **`database/migrations/V157__CDA_Declarative_Mapping_Rules_Encounters_Procedures.sql`**
  — two independent `INSERT...VALUES` statements (matching V155's own
  precedent, not one INSERT with comma-separated tuples — the drift-guard
  regex requires a literal `VALUES` immediately before each tuple it
  parses). New drift-guard test
  (`declarative_oob_rules_migration_v157_test.go`, reuses V155's regex)
  passed on first write.
- All verified via Docker: `go build ./...`, `go vet ./...`, full `go test
  ./...` (including `fhir/r4`, schemas mounted) green, `gofmt`-clean on
  every touched file.

**Phase 3, slice 6 — CarePlan / Goal (Plan of Care) — done** (2026-06-22).
Investigating before porting confirmed `careplan_mapper.go`'s `MapCarePlans`
(→ `CarePlan` resource) is, as the inventory suspected, fully dead code:
zero callers anywhere outside its own file (not wired into
`document_mapper.go`'s dispatch table, which routes every Plan-of-Care
section-key alias to `mappers.MapPlanOfCare` instead) and zero test
references either. Not ported — same treatment as `MapPractitioners`
before it.

This slice's real shape: `plan_of_care_mapper.go` dispatches ONE entry to
ONE of 5 resource types (ServiceRequest/Appointment/SupplyRequest/
MedicationRequest/Goal) by moodCode-then-EntryType, the same multi-rule
"first-match-wins" dispatch `BuildResourcesForRules` already proved for
Medications — no new dispatch architecture needed, but two real engine
extensions surfaced along the way (both small, both reused immediately
across this slice, not speculative):

- **`cdaPredicateKeys` gained `"entryType"`**: `dispatchPlanOfCareEntry`
  switches on `entry.EntryType` (after moodCode) — `EntryMatch` reuses the
  exact same closed predicate set `Scope` paths use, so without this key an
  `EntryMatch` like `"entryType=substanceAdministration"` would have
  silently matched *every* entry (an unknown predicate key is dropped, and
  zero predicates means "match everything") instead of failing loudly.
  Covered by a new resolver test.
- **`BuildResourcesForRules` now honors `FlattenOrganizers`** (previously
  only `BuildResources` did): `flattenPlanOfCareEntries` expands organizer
  components before Go's own dispatch switch runs, the same "organizer
  wraps several real activities" shape Vital Signs/Results/Social History
  already needed this primitive for, just at the multi-rule dispatch call
  site this time. Documented requirement: every rule sharing one
  `SectionKey` must set `FlattenOrganizers` consistently (all-or-none) —
  `claimed[]` tracks entries by index into that rule's own resolved
  entries slice, and flattening changes that slice's length, so mixing
  flattened/unflattened rules for the same `SectionKey` would silently
  misalign claims across rules. Covered by a new dedicated test proving
  indices stay aligned across two rules sharing one flattened `SectionKey`.

Delivered:

- **`GoalMappingRules()`** (standalone `"goals"` section) and
  **`PlanOfCareMappingRules()`** (all 4 of `document_mapper.go`'s
  Plan-of-Care section-key aliases — `carePlan`/`planOfCare`/
  `assessmentAndPlan`/`planOfTreatment` — each its own independent 6-rule
  dispatch group: an EVN-mood "sentinel" rule with empty `Fields` that
  claims-and-discards already-happened entries before any EntryType rule
  can see them, then Goal (moodCode=GOL, checked before EntryType — matches
  Go's own priority), MedicationRequest, Appointment, SupplyRequest,
  ServiceRequest). A real document only ever populates ONE of the 4 aliases
  depending on its title/templateId resolution path, never more than one
  simultaneously, so seeding all 4 is real coverage for different vendors,
  not speculative duplication.
- **DRY extraction, not duplication**: `medicationRequestRule()`
  (medications section) and `goalFields()` (both call sites) are shared
  helpers — `medicationRequestFields()`/`goalFields()` — reused verbatim by
  both the standalone section's rule and Plan-of-Care's matching dispatch
  branch, mirroring `medication_mapper.go`/`goal_mapper.go`'s own shared
  builder-function reuse exactly.
- **7 new transforms**, all thin wraps except two: `goal_status_to_fhir`,
  `service_request_status_to_fhir`, `service_request_intent_from_mood`,
  `appointment_status_to_fhir`, `supply_request_status_to_fhir`, and
  `cda_timerange_to_period_start`/`_end` (FHIR `Appointment.start`/`.end`
  are flat top-level strings, not nested under a `period` object the way
  `Encounter.period`/`Procedure.performedPeriod` are — these two extract one
  field each from `transforms.CDATimeRangeToPeriod`'s result).
- **A `LiteralValue`+`Transform` pitfall found and avoided**: Goal's
  3-tier description fallback (Value.Code → entry.Code → a literal
  `{"text":"Goal"}` placeholder) can't use `FallbackPaths`+`LiteralValue`
  together the way `MedicationRequest.requester`'s string placeholder does
  — a row's `Transform` always re-applies to whatever value
  `resolveRowSourceValue` resolves, *including* a literal fallback, so a
  CodeableConcept-shaped `LiteralValue` would get re-marshaled as a
  `CDACode` and silently mangled into nothing. Used a second row instead
  (literal-only, gated by `SkipIfResourceHasAnyOf: ["description"]`, the
  same idiom Observation's `dataAbsentReason` row already proved) — caught
  by reasoning through the existing primitive's exact semantics before
  writing the row, not by a failing test.
- **Tests**: 13 production-rule tests (7 ported 1:1 from `mappers_test.go`'s
  `TestMapPlanOfCare_*` suite, 3 new for the standalone Goals section + 1 new
  for Plan-of-Care's SupplyRequest branch — zero dedicated Go tests existed
  for either, per the inventory) plus 2 corpus
  end-to-end tests (`TestDeclarativeEngine_Corpus_GoalsEndToEnd`/
  `_PlanOfCareEndToEnd` — 0/4 corpus prevalence for both, consistent with
  the inventory's own note that Kareo's only structurally-present Plan of
  Treatment section has 0 entries; ran clean, no nulls/garbage, on all 4
  vendor files anyway).
- **A migration drift bug caught by the drift-guard test itself, before any
  product impact**: the first hand-written V158 draft mistranscribed
  `medicationRequestFields()`'s shared rows (wrong PIVL_TS scope, missing
  the `frequency` row, wrong RSON shape) and used `LiteralValue:
  map[string]interface{}{"value": 1}` for SupplyRequest's hardcoded
  quantity — JSON decodes a bare `1` to `float64`, not Go's `int`, so
  `reflect.DeepEqual` correctly failed the comparison until the literal was
  changed to `float64(1)` (same class of issue
  `medicationCommonRows`' own `frequency` row already documented). Both
  fixed; **`database/migrations/V158__CDA_Declarative_Mapping_Rules_CarePlan_Goal.sql`**
  now seeds 25 independent `INSERT...VALUES` rows (1 goals + 4 aliases × 6
  Plan-of-Care rules each) and its own drift-guard test
  (`declarative_oob_rules_migration_v158_test.go`) passes.
- All verified via Docker: `go build ./...`, `go vet ./...`, full `go test
  ./...` (including `fhir/r4`, schemas mounted) green, `gofmt`-clean on
  every touched file.

**Phase 3, slice 7 — CareTeam / Practitioner — done** (2026-06-22). This is
the slice the prior session's scoping discussion flagged as needing a new
"one entry → multiple cross-referenced resources" engine primitive before
it could start — `careteam_mapper.go`'s `MapCareTeam` builds a CareTeam
resource *plus* one independent, cross-referenced Practitioner resource per
care-team-member participant, a shape no row in this engine could express
(every prior row either writes one value or one sub-object, always inside
the ONE resource the matched entry is building).

Two new, narrowly-scoped engine primitives, both added to
`declarative_schema.go`/`declarative_engine.go`:

- **`MappingRow.EmitAsResource`**: a child row nested inside another row's
  `Fields` (i.e. under a `CollectAll`+`Fields` parent) can build a BRAND NEW,
  independent resource from its own nested `Fields` against the same scoped
  node, instead of writing into the shared sub-object its siblings populate
  — the emitted resource gets a synthesized id and the child row's own
  `TargetPath` receives a FHIR Reference to it instead. Implementation:
  `applyCollectAllWithFields`'s child loop special-cases
  `childRow.EmitAsResource != ""` via a new `buildEmittedSubResource` helper;
  `applyRow`/`buildOneResource`/`BuildResources`/`BuildResourcesForRules` all
  gained a threaded `extraOut`/return value to relay emitted resources
  upward, **always appended regardless of whether the main resource itself
  ends up kept or discarded** — mirrors `MapCareTeam`'s own
  `resources = append(resources, practitioners...)` running unconditionally,
  before even checking whether the CareTeam itself got built. Resource ids
  are synthesized via `idx` (the position within the enclosing CollectAll
  loop), not treated as final/global — consistent with the BP-panel
  synthesis precedent; only uniqueness within one build matters at this
  layer.
- **`MappingRule.RequiredPaths`**: after `Fields` finishes, the built
  resource is discarded entirely unless every listed top-level key ended up
  present — replicates `buildCareTeamResource`'s `if len(participants) == 0
  { return nil }` gate, which fires even though status/category/period rows
  would otherwise still populate the resource. Deliberately top-level-key-
  only (no nested path parsing) since CareTeam's only use
  (`RequiredPaths: []string{"participant"}`) doesn't need more.

Both primitives were verified to add ZERO behavior change to every
previously-shipped section before any CareTeam-specific code was written:
the full `cda_fhir`/`executors` suite was re-run immediately after each
engine change and stayed green throughout.

Delivered:

- **`CareTeamMappingRules()`** — one rule per `document_mapper.go` alias
  (`careTeam`, `careTeams`, both wired to `mappers.MapCareTeam`): status,
  category, period, then the `participants[*]` CollectAll+Fields row whose
  nested `EmitAsResource: "Practitioner"` child builds
  name/identifier/qualification/telecom from `participantRole`, with a
  sibling child row writing `functionCode` as the participant's `role`.
- **4 new transforms**: `care_team_status_to_fhir` (thin wrap) and three
  re-marshal adapters for existing `transforms.*` functions that had never
  been wrapped before — `cda_name_to_fhir` (`CDANameToFHIR`),
  `cda_ii_to_identifier` (`IIToIdentifier`), `cda_telecom_to_fhir`
  (`CDATelecomToFHIR`).
- **Deliberately NOT ported** (documented, not silently dropped):
  `buildCareTeamParticipants`' NPI→name enrichment lookup built from
  `entry.Components[].Performers[].AssignedEntity` (a participant whose
  outer entry carries only an NPI, no name, stays identifier-only here,
  same as before enrichment in Go) — a distinct cross-reference capability
  from `EmitAsResource` with zero corpus evidence and zero dedicated Go test
  coverage of its own; `subject` (patientRef-dependent, consistent with
  every prior rule); `_cdaIds` embedding on the emitted Practitioner (the
  assembly layer's cross-resource dedup marker) — CareTeam is the first
  declarative rule to emit a Practitioner at all, so no existing multi-
  section dedup scenario this gap could break today.
- **Tests**: 4 new production-rule tests (2 ported 1:1 from
  `mappers_test.go`'s `TestMapCareTeam_*` suite, including the
  `RequiredPaths` discard-gate proof; 2 new — playingEntity-sourced
  Practitioner fields, and multi-participant id-uniqueness +
  cross-reference correctness within one entry) plus 1 corpus end-to-end
  test (0/4 prevalence, consistent with the inventory's own note for this
  section). `database/migrations/V159__CDA_Declarative_Mapping_Rules_CareTeam.sql`
  adds a `required_paths TEXT[]` column (mirroring V155's
  `flatten_organizers` precedent: a new MappingRule field gets its own
  column) and seeds both aliases; its own drift-guard test
  (`declarative_oob_rules_migration_v159_test.go`, a new regex parsing the
  `ARRAY['participant']` literal — not a `migrationV155InsertPattern`
  reuse, since the column shape genuinely differs) passed on first run.
- All verified via Docker: `go build ./...`, `go vet ./...`, full `go test
  ./...` (including `fhir/r4`, schemas mounted) green, `gofmt`-clean on
  every touched file.

**Phase 3, slice 8 — Coverage / FamilyMemberHistory / Device — done**
(2026-06-22). The user asked to IG-verify, then port, all three remaining
discrepancy-blocked sections in one session. IG verification (via WebSearch/
WebFetch against build.fhir.org/ig/HL7/CDA-ccda-2.2 and cdasearch.hl7.org,
not training-data recall) resolved every flagged item:

- **Coverage LOINC code**: `48768-6` is correct for this project's R2.1
  target (CONF:1198-19160); `52556-8` belongs to a 2024+ companion-guide
  revision — no bug, no change.
- **Coverage statusCode→status**: CDA's `statusCode` is unconditionally
  fixed to "completed" (CONF:1198-19094) regardless of real coverage
  status — no CDA-side signal exists to derive `active`/`inactive` from
  instead, so Go's hardcoded `"active"` is the only defensible choice, not
  a gap. Added a citing code comment so a future reader doesn't re-flag it.
- **FamilyMemberHistory templateId date**: `2015-08-01` confirmed correct;
  the earlier flagged conflict was a rendering artifact.
- **Device participant typeCode**: **confirmed real bug** — IG fixes
  `typeCode="PRD"` (CONF:1098-8754); Go checked `"DEV"`/`"CSM"`, matching
  neither, so the participant-sourced device code path was dead code in
  every conformant document, always silently falling back to the entry's
  own code. **Fixed in `device_mapper.go` itself** (not just the
  declarative port) — zero existing tests on this mapper, zero other
  callers, confirmed safe.

`architecture/CDA_FHIR_MAPPING_INVENTORY.md`'s sections 13-15 updated with
these verified findings and citations.

Two more extract-method refactors (zero behavior change, mirroring this
session's `AllergyReactionSeverityToFHIR` precedent): `coverage_mapper.go`'s
inline LOINC display-correction logic → `transforms.CoverageTypeToCodeable
Concept`; `device_mapper.go`'s private `deviceStatus()` →
`transforms.DeviceUseStatementStatusToFHIR`. Both now have declarative
wraps (`coverage_type_to_codeable_concept`, `device_use_statement_status_to_fhir`)
alongside two more new transforms: `cda_value_to_fhir` (bare `CDAValue` →
FHIR, distinct from `cda_value_or_code_to_codeable_concept`'s whole-entry
value-else-code fallback — FamilyMemberHistory's component observations
only ever read `.Value`, never `.Code`, the problem-type discriminator) and
`cda_name_to_family_string` (the lossy family-only name transform
`family_history_mapper.go` already uses, ported as-is — flagged as a
candidate improvement in the inventory, not fixed here).

`CoverageMappingRules()` (2 aliases: `payersInsurance`/`payors`),
`FamilyMemberHistoryMappingRules()`, `DeviceMappingRules()` — no new engine
primitives needed; all three use only Scope/ScopeFallbacks/
SkipIfResourceHasAnyOf/CollectAll+Fields, already proven by earlier slices.
A few narrow, explicitly-documented simplifications where Go's loop scans
*all* candidates for the first non-empty one and this port checks only the
first/`[0]` candidate (Coverage's COMP-performer-org fallback and
subscriberId; Device's PRD-code-vs-entry-code preference) — 0/4 corpus
evidence either way for any of them, called out in code comments rather
than silently diverging.

Deliberately NOT ported (confirmed gaps in Go itself, not disagreements
with it): FamilyMemberHistory's `administrativeGenderCode`, birthTime-based
age derivation (CONF:1198-15983, well-cited, never implemented), Death
Observation, Age Observation; Device's standalone `Device` resource/UDI
path (Go only ever produces `DeviceUseStatement`); Coverage's
`beneficiary`/`subscriber`/real subscriber-relationship code (patientRef-
dependent or already-hardcoded in Go).

Tests: 8 new production-rule tests (no Go precedent — all three mappers had
zero existing tests) plus 3 corpus end-to-end tests (0/4 prevalence for all
three, consistent with the inventory). `database/migrations/V160__CDA_Declarative_Mapping_Rules_Coverage_FamilyHistory_Device.sql`
seeds all 4 rows (2 Coverage aliases + FamilyMemberHistory + Device) reusing
`migrationV155InsertPattern` (same column shape as V156) — its drift-guard
test passed on first run. All verified via Docker: `go build ./...`,
`go vet ./...`, full `go test ./...` (including `fhir/r4`, schemas mounted)
green, `gofmt`-clean on every touched file (one pre-existing, unrelated
formatting drift in `code_transform.go` fixed in passing).

**Practitioner/Custodian (document-header half) — done** (2026-06-22).
Closes the last of the 15 inventoried sections. CareTeam's earlier slice
only ever covered section 10's entry-level "CareTeam organizer participant
→ Practitioner" row; this slice covers the rest: `MapAuthor`/`MapCustodian`
(wired from `doc.Header.Authors`/`doc.Header.Custodian` directly in
`document_mapper.go`, before the section-key dispatch loop even starts —
not through any `sectionsByKey.*` entry-iterating dispatch) plus the
`legalAuthenticator` gap (not parsed at the Go level at all before this
slice).

New engine primitive — `MappingRule.HeaderPath` + `DeclarativeEngine
.BuildHeaderResource` (`declarative_schema.go`/`declarative_engine.go`):
resolves against `documentMap["header"]` instead of iterating
`sectionsByKey.<key>.entries[*]`, reusing the existing `buildOneResource`
unchanged — every other engine mechanic (Scope/ScopeFallbacks/CollectAll/
RequiredPaths/etc.) applies identically once the one header sub-object is
found. Two HeaderPath values exist: `"custodian"` (a single object) and
`"authors"` (special-cased via a new `firstAuthorWithPerson` helper — "the
first author with a usable assignedPerson" is an existence check the Phase
1 bracket-predicate grammar can't express, only equality).

- `AuthorMappingRules()`/`CustodianMappingRules()` — direct ports of
  `MapAuthor`/`MapCustodian`. Deliberately NOT ported: Author's addresses
  (`a.AssignedAuthor.Addresses` exists in the typed struct but
  `MapAuthor` never reads it — confirmed gap in Go itself, not fixed here);
  `_cdaIds` assembly-layer dedup marker on either resource (same
  deferred-not-forgotten reasoning as CareTeam's emitted Practitioners);
  Author's `AssignedAuthoringDevice`/`RepresentedOrganization` (no
  Organization/PractitionerRole link for device-authored or
  org-represented authors exists anywhere in this codebase).
- `LegalAuthenticatorMappingRules()` — **new functionality, not a port**.
  Added `cda/document/types.go`'s `CDALegalAuthenticator` struct (`time`,
  `signatureCode`, `assignedEntity` — base CDA's LegalAuthenticator class,
  verified via WebFetch against `build.fhir.org/ig/HL7/CDA-core-2.0` and
  HL7Wiki's "CDA R3 legalAuthenticator" page, 2026-06-22: a document is
  legally signed by a person, never an organization) and
  `header_parser.go`'s `parseLegalAuthenticator` (0..1, nil when absent).
  The declarative rule builds a Practitioner from `assignedEntity` with the
  full available shape (name/identifier/telecom/address) since there's no
  existing Go fidelity target to narrow it against.
  **`Composition.attester` wiring is explicitly deferred to Phase 4**:
  `composition_mapper.go`'s `CompositionMapper` builds `Composition` from
  `document_mapper.go`'s LIVE resource list today, which this declarative
  rule is not part of yet — same dormant-until-cutover status as every
  other rule in this file. Wiring `attester` now would mean either (a)
  bypassing the Phase 3/4 separation by hand-wiring one new declarative
  rule into the live pipeline while everything else stays dormant, or (b)
  building a second, throwaway Go-only path — neither is worth doing for a
  field Phase 4's cutover will wire correctly in one pass anyway.
- New transform: `cda_address_to_fhir` (wraps `transforms
  .CDAAddressToFHIR`) — the first declarative row to need Organization/
  Practitioner addresses (every prior section either had no address field
  or didn't port it).
- Tests: 6 production-rule tests for Author/Custodian (1 ported 1:1 from
  `mappers_test.go`'s only existing test, `TestMapCustodian_SetsActiveTrue`
  — Author had zero Go test precedent) + 3 for LegalAuthenticator, plus 5
  corpus end-to-end tests (Author: 3/4 vendors produce a Practitioner;
  Custodian: 4/4; LegalAuthenticator: 3/4 — real prevalence, not synthetic
  guesses). `database/migrations/V161__CDA_Declarative_Mapping_Rules_Author_Custodian.sql`
  adds a `header_path` column and seeds both header.authors/header.custodian
  rows; `V162__CDA_Declarative_Mapping_Rules_LegalAuthenticator.sql` seeds
  the third. Both drift-guard tests pass (`migrationV161InsertPattern`,
  a new regex since header_path is a genuinely different column shape from
  every section-keyed migration before it; V162 reuses it).

All verified via Docker: `go build ./...`, `go vet ./...`, full
`go test ./...` green, `gofmt`-clean on every touched file (one pre-existing,
unrelated formatting drift in `cda/document/types.go` fixed in passing —
caused by gofmt's struct-tag column alignment recalculating once a longer
field name was added, not by a content change).

**Remaining for Phase 3**: the two small items already flagged and
deferred from slice 1 (Medication's `effectiveDateTime` fallback) and
slice 3 (shell-entry substitution and `interpretationCode`; BP-panel
combination is resolved via the assembly layer, not deferred). Neither
needs a new engine primitive. Otherwise, Phase 3 is done — Phase 4
(cutover/shadow-mode design) is the next real decision point.

**Phase 2 — Unified Mapping Schema + Execution Engine — done** (same day,
2026-06-21). Design note is inline under "Phase 2" below — read it before
touching this code; it documents two corrections to this doc's own earlier
framing (the right transform/terminology components to reuse turned out not
to be the ones originally named) found by reading the actual code before
writing any, not by trusting the task list as written. Delivered:

- `services/cda_fhir/declarative_schema.go` — `MappingRule` (entry-selection
  + FHIR resource type) and `MappingRow` (field-level, with `Scope`,
  `SourcePath`/`FallbackPaths`/`LiteralValue`, `CollectAll`, `Condition`) per
  the design note's two-level decomposition.
- `services/cda_fhir/declarative_transform_registry.go` — a new, small,
  named transform registry whose functions are thin adapters re-marshalling
  Phase 1's resolved values into the typed `cdadocument.*` structs the
  already-IG-verified `services/cda_fhir/transforms/*.go` functions expect —
  not `cda_transform_registry.go` (wrong reuse target; see the design note).
- `services/cda_fhir/declarative_engine.go` — `DeclarativeEngine.BuildResources`
  (one rule) and `BuildResourcesForRules` (multiple rules sharing a
  `SectionKey`, first-match-wins exclusivity — e.g. Medications'
  moodCode-driven Request/Statement split). Also wires an actually-new
  capability: optional `fhir/r4.TerminologyRegistry` ValueSet-membership
  checking on row output, via `MappingRow.ValueSetURL` (the dormant engine
  never had this; it only ever did `cda_terminology`'s offline format check,
  a different question — see the design note's terminology correction).
- `services/cda_fhir/fhir_path_writer.go` — `setFHIRPath`/`ensureArray`/
  `parseBracketIndex`/`stripResourcePrefix` **moved** (not duplicated) out of
  `generic_mapper.go` so both the dormant engine and the new one call the
  same path-writer; `generic_mapper.go` itself is otherwise untouched.
- Test suite: `declarative_engine_test.go` — 16 tests proving all 4 worked
  examples named in the design note (allergy negation→verificationStatus via
  `Scope`+`Condition` together; Free-Text-Sig-vs-Instruction(V2) via two
  independently-`Scope`-gated rows on the real `medication_sig_instruction.xml`
  fixture, including a negative test proving the templateId discriminator is
  load-bearing, not coincidental; RSON→`reasonCode[]` via `CollectAll`;
  Medications' moodCode dispatch via `BuildResourcesForRules`), plus engine
  mechanics (Required/SHALL error severity, an unmatched `Scope` silently
  contributing nothing, `ValueSetURL` accept/reject, `FallbackPaths`) and
  transform-registry unit coverage. All green; the two real-fixture tests
  reuse `negation_and_frequency.xml` (Phase 1) and `medication_sig_instruction.xml`
  (already committed, previously unused by any test) rather than re-inventing
  fixtures. Full `cda/...`/`services/cda_fhir/...` suites stay green (zero
  regressions from the `fhir_path_writer.go` extraction) and `go vet ./...`
  is clean — verified via the `/go-build-check` Docker path.
- Performance (`declarative_engine_bench_test.go`, Docker gobuilder stage,
  same i5-1345U host):

  | Benchmark | Fixture | ns/op | B/op | allocs/op |
  |---|---|---|---|---|
  | `BenchmarkDeclarativeEngine_AllergyNegation` | `negation_and_frequency.xml`, 1 rule, 1 row | 2,015 | 1,856 | 32 |
  | `BenchmarkDeclarativeEngine_FreeTextSigVsInstructionV2` | `medication_sig_instruction.xml`, 1 rule, 2 rows | 3,235 | 2,024 | 44 |

  **These are per-rule numbers, not per-document** — not a direct
  apples-to-apples unit against Phase 0's per-document `MapDocument()`
  baseline above (41,650–192,584 ns/op for an entire document, every
  section, assembly, narrative generation, and bundle assembly included).
  Phase 2 deliberately scopes `BuildResources` to one rule at a time; no
  full-document equivalent exists yet to benchmark against because no real
  multi-rule-per-document wiring exists yet (that's Phase 3/4). **Named,
  not hidden, concern**: each rule call independently re-resolves
  `sectionsByKey.<key>.entries[*]` from the document root — when Phase 3/4
  wires a realistic rule count per section (a real OOB section can carry
  5–15 rows), summing today's per-rule cost naively could plausibly exceed
  the Phase 0 per-document floor for small documents, since nothing yet
  shares that resolution across rules targeting the same section.
  `BuildResourcesForRules` already resolves the full entry list once per
  *rule* (not per row) — sharing it once per *section* across every rule
  that targets it is the natural next optimization, deliberately deferred
  until Phase 3/4's real wiring shows whether it's actually needed, per the
  sprint plan's own "design for hypothetical future requirements" caution —
  but explicitly flagged here so it isn't rediscovered cold.

Phase 0's mapping inventory and Phase 1's path resolver are both used
as-is by Phase 2 — nothing in this phase changed either.

**Phase 1 — Path Resolution Language — done** (same day, 2026-06-21). Design
note is inline under "Phase 1" below. Delivered:

- `services/executors/cda_path_resolver.go` — wildcard (`[*]`), numeric-index
  (`[N]`), and predicate (`[typeCode=X]`, comma-separated AND compounds)
  traversal over both the typed `document.*` tree and the generic `xml.*`
  mirror, plus `ResolveCDAFallback` (the "field A, else field B" primitive).
  Wired into `field_utils.go`'s existing `GetFieldValue`/`UpdateFieldValue`
  dispatch — `document.*` getters now route through the new resolver
  (strict superset of the old plain/numeric-index walker, zero behavior
  change for existing callers); a new `xml.*` path type/prefix was added
  for the mirror (getter-only by design — see the note on why).
- A real bug was caught and fixed by the resolver's own test suite before
  merge, not after: a naive `strings.Split(path, ".")` shredded CDA
  templateId OIDs (e.g. `2.16.840.1.113883.10.20.22.4.147`, themselves
  dot-separated) wherever one appeared inside a `[templateId=...]` predicate
  bracket. Fixed with bracket-depth-aware splitting
  (`splitCDAPathOnDots`); regression-tested explicitly
  (`TestParseCDAPath_TemplateIDOIDInsideBracketNotSplitOnDots`), since this
  is the single most common path shape Phase 2 will generate.
- Test suite: `cda_path_resolver_test.go` (hand-built fixtures per backend,
  including the byte-identical-shape Free-Text-Sig-vs-Instruction(V2)
  discriminator from the inventory's cross-cutting finding #1, expressed as
  two chained-predicate paths) + `cda_path_resolver_fixture_test.go` (real
  `negation_and_frequency.xml`/`supply_and_nullflavors.xml` parsed through
  the actual production `CDAParser`/`GenericXMLToJSON` code, JSON-round-tripped
  exactly as `cda_parser_service.go` does, per the exit criteria). All green,
  plus the full pre-existing `cda/...`/`services/cda_fhir/...`/
  `services/executors/...` suites (zero regressions) and `go vet ./...` clean
  — verified via the `/go-build-check` Docker path, not locally.
- Performance baseline (`cda_path_resolver_bench_test.go`, Docker gobuilder
  stage, default `-benchtime`, same i5-1345U host as Phase 0's baseline):

  | Benchmark | Fixture | ns/op | B/op | allocs/op |
  |---|---|---|---|---|
  | `BenchmarkResolveCDAPath_NegationAndFrequency` | `negation_and_frequency.xml` (4 resolutions/iter: predicate, wildcard, xml-mirror predicate, fallback chain) | 3,621 | 3,072 | 47 |
  | `BenchmarkResolveCDAPath_SupplyAndNullFlavors` | `supply_and_nullflavors.xml` (same 4-resolution mix) | 3,446 | 3,440 | 45 |
  | `BenchmarkResolveCDAPath_CorpusLarge_WildcardFanout` | `corpus/cerner_sample.xml` (~36.7 KB, 1 wildcard collect-all over every medication entry) | 733 | 672 | 10 |

  This is the resolver only (fixture parsing happens once outside the timed
  loop) — the **Phase 2+ regression floor for the resolver specifically**,
  separate from Phase 0's `MapDocument()` floor above. Re-run after Phase 2
  wires the resolver into the execution engine and compare directly.

Phase 0's mapping inventory (below) remains the basis for Phase 2's schema
design and Phase 3's row migration — nothing in Phase 1 changed it.

The field-level mapping inventory is done:
[architecture/CDA_FHIR_MAPPING_INVENTORY.md](CDA_FHIR_MAPPING_INVENTORY.md)
covers all 15 sections with a Go mapper today (9 already in the V149 seed,
6 not yet seeded), citing live-researched C-CDA R2.1 IG CONF numbers (no
citation was recalled from training data — unverifiable ones are explicitly
marked `UNVERIFIED`), corpus prevalence against the 4-vendor sample set, and
a confidence rating per field.

Read the inventory's "Cross-cutting findings" section (10 numbered findings)
before starting Phase 1 — it directly informs the path-resolution primitives
Phase 1/2 need (wildcard/predicate traversal, templateId-conditional dispatch,
fallback chains).

**All 5 confirmed production bugs found during Phase 0 are now fixed** (same
session, 2026-06-21), each verified against an authoritative source — live
C-CDA R2.1 IG citations, HL7's official C-CDA-on-FHIR `ConceptMap` resources
fetched from `github.com/HL7/ccda-on-fhir`, and current US Core/FHIR R4 value
sets — never assumed from the 4-vendor corpus, which was evidence of
prevalence/severity only:
1. Immunization `negationInd` ignored (refused vaccines reported as given) — fixed, `services/cda_fhir/mappers/immunization_mapper.go`.
2. Procedure `bodySite` read from the wrong CDA structure — fixed at the parser level (`cda/document/types.go`, `entry_parser.go`) plus the mapper, since the field wasn't even being parsed.
3. `Condition.category` hardcoded regardless of section (Problems vs Health Concerns) — fixed, `condition_mapper.go` + `document_mapper.go` dispatch table.
4. Procedures/Medications status valueMap disagreement between Go and the V149 seed — resolved by fetching HL7's official `ConceptMap/CF-ProcedureStatus` and `ConceptMap/CF-MedicationStatus`: the seed was right and Go had the Procedure bug; a *second*, more specific Medication bug was found in the process (`aborted` wrongly bucketed with `cancelled`). Also discovered: the official IG has **no consensus mapping at all** for `moodCode="EVN"`/MedicationStatement — now documented in code as a best-effort default, not presented as spec-verified.
5. Malformed-NPI `Identifier` fallback (`IIToIdentifier` falling back to the NPI system OID as the value when `@extension` is absent) — fixed with a narrow `isFixedIdentifierSystem()` check that preserves the legitimate root-as-value fallback for generic/facility OIDs.

See the inventory doc's "Confirmed bugs/findings" section for full detail on each. All fixes verified via the `/go-build-check` Docker path: `go build ./...`, `go vet ./...`, and `go test ./...` clean across the repo, with new regression tests for every fix.

**Performance baseline established** (`services/cda_fhir/document_mapper_bench_test.go`,
run via `go test -bench=. -benchmem ./services/cda_fhir/`, Docker gobuilder stage,
`-benchtime=2s`, 13th Gen Intel i5-1345U):

| Bracket | Fixture | Sections | ns/op | B/op | allocs/op |
|---|---|---|---|---|---|
| Small | `minimal_ccd.xml` (~4.8 KB) | 1 | 41,650 | 27,681 | 331 |
| Medium | `full_ccd_nist.xml` (~17.8 KB) | ≥9 | 167,339 | 100,099 | 1,046 |
| Large | `corpus/cerner_sample.xml` (~36.7 KB, real vendor export) | full CCD | 192,584 | 112,385 | 1,231 |

This is `MapDocument()` only (XML parsing happens once outside the timed loop).
**This is the Phase 1 regression floor** — the new path resolver, once wired in,
should not push these numbers up materially for equivalent documents. Re-run
the same benchmark file after Phase 1/2 land and compare directly.

What already exists and is *not* being thrown away (see "What we're keeping"
below): the generic, schema-agnostic XML→JSON mirror (`GenericXMLToJSON`,
`cda/document/generic_xml.go`), the typed `CDADocument` tree, `CDATransformRegistry`,
`TerminologyRegistry`, and the existing `mappers_test.go`/`cda_fhir_integration_test.go`
test suites, which become the migration's acceptance bar.

---

## Why this plan exists

An audit of the current CDA→FHIR mapping found **two parallel systems**,
only one of which runs in production:

- **Live path**: `MapDocument()` (`services/cda_fhir/document_mapper.go` →
  16 files under `services/cda_fhir/mappers/`) — hardcoded Go, reads the
  typed `CDAEntry` struct directly. Every real fidelity fix (negation
  handling, SIG-text resolution, PIVL frequency, RSON indications, severity
  from nested observations) lives here, in Go, not configurable.
- **Dormant path**: `GenericCDAFHIRMapper.Map()` (`services/cda_fhir/generic_mapper.go:775`)
  — a real, working, generic interpreter that reads `cda_fhir_templates.template_config`
  mapping rows and dispatches transforms by name. Fully wired to the DB,
  has REST endpoints, terminology validation built in — but the executor
  (`cda_to_fhir_executor.go:156-168`) only reaches it as a last resort, which
  essentially never happens. Editing the DB templates today changes nothing
  in production.

The dormant path is *also* not expressive enough to replace the hardcoded
mappers as-is: it can only extract flat fields, with no way to express "the
entryRelationship where typeCode=RSON" or "negationInd=true → refuted." That
expressiveness gap is real engineering, not just a wiring fix.

**Goal of this plan**: end with exactly one engine — declarative,
user-editable, OOB-seeded, normatively correct, and at least as capable as
today's hardcoded mappers — and zero hardcoded per-resource-type Go mapper
files left behind.

## Separate step, shared engine — not a unified mega-mapper

CDA keeps its own pipeline step type (`cda.to_fhir` today, or its eventual
successor name) — **distinct** from HL7's `hl7_fhir_transform` step. Do not
merge these into one "any format → FHIR" step. The two formats have
genuinely different addressing needs (CDA's recursive `entryMatch`/`repeating`
from Phase 2 has no HL7 equivalent), and a forced-unified step would mean
every HL7 edit risks touching CDA code and vice versa.

What *is* shared, deliberately, at the library level (not the step level):
`CDATransformRegistry`/HL7's `DataTypeTransform` dispatch are the same
*pattern* even if not the same registry; `TerminologyRegistry`; the path
resolver from Phase 1 (built generically enough that HL7 mapping could
eventually use the same wildcard/predicate capability for its own unfinished
`"repeating": true` gap — see the HL7→FHIR architecture doc — without that
becoming a Phase 0–6 dependency here); and, per the open question already in
this doc, a possible shared value-set/transform-mapping table. Each phase
below should default to "reuse if a clean shared abstraction already exists,
fork if forcing reuse would couple two unrelated formats" — and say
explicitly which it chose.

**Mapping rows are editable on both sides.** Every mapping row exposes
`sourcePath` (the CDA-side input mapping — `document.*`/`xml.*`, extended
with Phase 1's wildcard/predicate syntax) and `targetPath`/`fhirPath` (the
FHIR-side output mapping) as independently user-editable fields, not just
the transform/value-set in between. The Phase 5 UI must expose both for
editing — same shape as HL7's existing editor already does for its own
source/destination pair (`enhanced-mapping-interface.js`).

## What we're keeping (do not rebuild these)

| Component | File | Why it stays |
|---|---|---|
| Generic XML→JSON mirror | `cda/document/generic_xml.go` | Already verified 100% lossless across 14 EHR vendors; the long-term fallback/audit layer |
| Typed `CDADocument` tree | `cda/document/types.go`, `entry_parser.go` | Clean semantic source for the new engine's path resolver to read from |
| `CDATransformRegistry` | `services/cda_fhir/cda_transform_registry.go` | Named transforms + type-pair inference — exactly the transform library the new engine needs |
| `TerminologyRegistry` | `fhir/r4/terminology.go` | Value-set validation, already used by the dormant engine |
| `mappers_test.go` / `cda_fhir_integration_test.go` | `services/cda_fhir/` | Becomes the migration's acceptance suite — every assertion in here must still pass against the new engine's output |
| `field_utils.go` path resolver | `services/executors/field_utils.go` | Foundation to extend with wildcard/predicate support (Phase 1), not replace |

---

## Cross-cutting principles (apply to every phase, not a separate phase)

### Security / HIPAA checklist (re-check at every phase exit)
- No PHI in mapping-row config or test fixtures — synthetic data only, ever.
- Mapping-row CRUD changes are audit-logged (who changed what mapping, when) — reuse the existing `audit_logs` table, do not invent a parallel log.
- `MappingLog` output (already persisted per message) reviewed for accidental PHI leakage in error/trace fields before any schema change ships.
- RBAC: OOB/system templates (`is_system=true`) read-only to non-admins; interface-level overrides scoped to users with access to that interface — mirror the existing `is_system`/`is_public` flags in `cda_fhir_templates`.
- Transform dispatch stays **named, registry-based** (`CDATransformRegistry`-style). Do not allow arbitrary user-supplied script in a mapping row unless it goes through the same sandboxed `goja` path already used by `script_enrichment_executor.go` — a free-text "transform" field is an injection surface, not a feature.

### Performance checklist (re-check at every phase exit)
- Baseline current `MapDocument()` latency per document-size bracket **before** Phase 1 starts (this is the regression floor).
- Cache each interface's *resolved/compiled* mapping config (interface + docType + template version) in memory — never re-parse the template JSON per message.
- Predicate matching (`typeCode=X`, `templateId=Y`) must be O(1) equality/map-lookup in the hot path, not a general expression evaluator.
- No phase merges without a benchmark comparison against the Phase 0 baseline; regressions need an explicit, written sign-off, not a shrug.

### SDLC gate per phase
Design note (short ADR-style, even a few paragraphs) → implementation →
unit + integration + corpus-fidelity tests → `/code-review` → `/security-review`
→ merge. Don't skip the design note even for "small" phases — Phase 1's path
language is foundational; getting its semantics wrong is expensive to find
out about in Phase 3.

---

## Phase 0 — Spec Grounding & Mapping Inventory

**Do not skip this.** This is the direct answer to "do we need to spend more
time understanding CCD/FHIR/the mapping before building" — yes, here, as a
deliverable, not vibes.

**Goal**: produce a reviewed, citation-backed mapping inventory covering every
section the current 16 Go mappers already handle, plus an explicit,
confidence-rated list of what's *not* covered yet.

**Tasks**:
1. For each of the 9 sections already seeded in `V149__CDA_FHIR_Schema.sql`
   plus the additional sections covered by Go mappers but not yet in that
   seed (Encounter, Procedure, CareTeam, CarePlan/Goal, Coverage,
   FamilyMemberHistory, Device, Practitioner/Custodian) — re-verify field-level
   mappings against the actual C-CDA R2.1 IG PDF (page + CONF number), the
   same rigor used this session for the SIG/Instruction(V2) and US Core
   MedicationStatement findings.
2. Cross-check each mapping against the multi-vendor corpus
   (`sample_ccdas`, already cloned/sampled once this session) — does this
   field actually get populated by real EHRs, or is it IG-theoretical?
   Prioritize by real-world prevalence.
3. Rate every field-level mapping: **HIGH** (already implemented + tested
   today, or directly IG-cited this session), **MEDIUM** (clear IG guidance,
   not yet implemented/tested), **LOW** (ambiguous, vendor-variable, or
   genuinely under-specified by the IG).
4. Explicitly list C-CDA sections with *no* current mapper at all (Advance
   Directives, Nutrition, Medical Equipment if uncovered, Payers detail
   beyond Coverage, Plan of Care sub-templates) as out-of-scope for Phase 3,
   destined for the Phase 6 additive process instead.

**Exit criteria**: a reviewed inventory document (can live as a companion
file, e.g. `architecture/CDA_FHIR_MAPPING_INVENTORY.md` or a structured
JSON/YAML — decide format at the start of this phase) listing every field,
its confidence rating, its citation, and its corpus-prevalence signal.
**LOW-confidence items are flagged, not mapped — they wait for Phase 6.**

---

## Phase 1 — Path Resolution Language

**Goal**: give the mapping engine a way to address CDA's recursive/repeating
structure that today only exists as scattered Go helpers
(`findRelByTypeCode`, `hasTemplateID`).

**Tasks**:
- Extend the path resolver (`services/executors/field_utils.go` or a new
  sibling module — decide in the design note) with:
  - Wildcard iteration: `entries[*]`, `entryRelationships[*]`
  - Predicate filtering on the actual RIM discriminators that matter:
    `typeCode`, `inversionInd`, `templateId`, `classCode`, `moodCode` —
    not a general expression language, just these.
- Works against both the typed `document.*` tree and the generic `xml.*`
  mirror.
- Heavily unit-tested in isolation — this is infrastructure every later
  phase depends on; bugs here are expensive to find later.

**Exit criteria**: standalone test suite proving wildcard/predicate
resolution against real fixture documents (reuse `negation_and_frequency.xml`,
`supply_and_nullflavors.xml`, the corpus fixtures already committed under
`cda/document/testdata/`). Performance baseline established (resolve N
paths in target latency) — this number is the Phase 2+ regression floor for
the resolver specifically.

### Design note (2026-06-21)

**Where it lives**: `services/executors/cda_path_resolver.go`, same package
as `field_utils.go` (`executors`), not a new package. `field_utils.go` already
owns the `PathTypeCDADocument` (`document.*`) dispatch and already imports a
CDA-specific helper (`GetXPathForCDA`), so this stays consistent with the
existing architecture rather than opening a new package boundary — and it
keeps `field_utils.go` itself from growing past ~1000 lines. A new `xml.*`
path type/prefix (`PathTypeCDAXMLMirror`) is added alongside `document.*` so
both trees are addressable through the same `GetFieldValue`/`UpdateFieldValue`
entry points every other executor already calls.

**Why not extend `parseJSONPath`/`resolveJSONPathValue` in place**: those are
the hot-path walker for every HL7/FHIR/generic-JSON `GetFieldValue` call in
the codebase. Bending them to understand `[*]` and `[typeCode=X]` risks
regressing formats that have nothing to do with CDA. The new resolver is a
parallel, CDA-specific walker that `document.*`/`xml.*` dispatch routes to
instead — same pattern the codebase already uses for `PathTypeCDA`'s
short-path resolver. Existing numeric-index paths (already shipped and
tested — see `TestGetFieldValue_CDADocumentPath_ResolvesNestedNegationInd`)
resolve through the *same* new walker now (it's a superset), so there is one
implementation, not two, going forward.

**Grammar** (deliberately not a general expression language — see the
Performance checklist):

```
path       = segment ('.' segment)*
segment    = identifier ('[' bracket ']')?
bracket    = '*'                          // wildcard: every element
           | digits                       // numeric index
           | predicate (',' predicate)*   // AND of predicates
predicate  = key '=' value
key        = "typeCode" | "inversionInd" | "templateId" | "classCode" | "moodCode"
```

The compound-predicate, comma-separated syntax (`entryRelationships[typeCode=SUBJ,
inversionInd=true]`) is not invented for this note — it's the exact notation
[CDA_FHIR_MAPPING_INVENTORY.md](CDA_FHIR_MAPPING_INVENTORY.md)'s cross-cutting
finding #1 already uses to describe Instruction(V2)'s discriminator. Note
precisely what that notation covers, though: `typeCode`/`inversionInd` both
live on the `entryRelationship` wrapper itself, so they combine in one
bracket; `templateId` lives one level deeper, on the *nested* clinical
statement (the `act`/`observation`/`substanceAdministration` the relationship
wraps), so the real Free-Text-Sig-vs-Instruction(V2) discriminator is two
chained predicate segments, not one compound bracket:
`entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147]`
(Free Text Sig) vs.
`entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.20]`
(Instruction (V2)) — directly mirroring `medication_mapper.go`'s
`rel.TypeCode == "COMP" && hasTemplateID(nested.TemplateIds, ...)` check, just
expressed as a path instead of Go. A predicate bracket filters candidates
found at its own key by attributes that live on those same candidates;
chaining is how a path reaches into a nested element for the next filter —
this composes to arbitrary depth without any special-casing for "nested
templateId."

**Resolution model**: XPath-style node-set, not single-value. Each segment
either narrows the current node set 1:1 (plain key, numeric index) or fans
it out (`[*]`, predicate — both can match 0, 1, or N elements per input
node). Three exported entry points cover every consumer this phase and Phase
2 need:
- `ResolveCDAPaths(root, path, isXMLMirror) []interface{}` — the full node
  set, for "collect all" cases (Medication's RSON→`reasonCode[]`, which
  today hand-rolls this loop in `indicationReasonCodes`).
- `ResolveCDAPath(root, path, isXMLMirror) interface{}` — first match or
  nil. This is what `GetFieldValue`'s `document.*`/`xml.*` dispatch calls —
  preserves the existing single-value contract every other path type
  already has.
- `ResolveCDAFallback(root, isXMLMirror, paths...) interface{}` — tries each
  path via `ResolveCDAPath` in order, returns the first non-nil/non-empty
  result. This is cross-cutting finding #1's "field A, else field B"
  primitive (Allergy's CSM-code-else-assertion-value fallback,
  `allergy_mapper.go:93-97`) as a named, testable function instead of an
  inline `if`.

**Two backends, one predicate set**: `typeCode`/`classCode`/`moodCode`/
`inversionInd` are plain map keys in the typed `document.*` tree (JSON tags
on `CDAEntryRelationship`/`CDAEntry`) but `@`-prefixed XML attributes in the
`xml.*` mirror (`GenericXMLToJSON`'s convention). `templateId` is the odd one
out structurally on both sides: the typed tree exposes a flat `templateIds`
string array (membership test), while the mirror exposes one-or-more
`<templateId root="...">` child elements (`@root` equality test against
each). `matchesCDAPredicate`/`matchesCDATemplateID` branch on `isXMLMirror`
to pick the right shape — callers never see the difference once they're past
the boolean flag at the entry point.

**The collapsed-single-occurrence problem**: `GenericXMLToJSON` collapses a
child that occurs exactly once to a bare object, not a 1-element array
("auto-array-on-repeat" — see `generic_xml.go`'s own doc comment and
`TestGenericXMLToJSON_SingleOccurrenceNotWrappedInArray`). A document with
one `entryRelationship` and a document with three must still both resolve
correctly through `entryRelationships[*]`. `asCDANodeList` normalises any
resolved child — bare object or real array — into a slice before
wildcard/index/predicate matching runs, so this asymmetry is invisible past
that one helper. The typed `document.*` tree never has this problem
(`encoding/json` always serialises a Go slice as a JSON array regardless of
length), but routing both backends through the same `asCDANodeList` call
costs nothing and removes a second code path to maintain.

**Scope boundary — getter only for wildcard/predicate paths**: `UpdateFieldValue`
keeps using the existing `modifyJSONPathValue` walker for `document.*`
(plain/numeric-index paths only, unchanged) and returns `false` outright for
`xml.*` (the mirror is documented as the read-only audit/fallback layer —
nothing in the pipeline writes through it). Writing through a wildcard or
predicate match is inherently ambiguous (which of N matches gets the new
value?) and nothing in Phase 1 or Phase 2's design needs it — Phase 2's
engine *reads* matched entries and writes to the FHIR-side output tree via
a separate, single-target path-writer (`setFHIRPath`), never back into the
CDA source tree. Revisit only if a concrete Phase 2+ need surfaces.

**Performance**: predicate matching is equality/map-lookup per candidate
node — O(1) per check (`templateId`'s membership test is O(k) in the number
of templateIds on that one node, typically 1-3, not a document-wide scan).
Path-string parsing (`parseCDAPath`) is cheap (one string split + per-segment
bracket parse) and re-run per call in Phase 1, since nothing yet calls the
same path repeatedly per message — caching/compiling parsed paths once per
interface is explicitly deferred to Phase 2, where mapping rows *are*
resolved repeatedly per message and the sprint plan's own Performance
checklist already calls for compiled-config caching at that layer, not this
one.

---

## Phase 2 — Unified Mapping Schema + Execution Engine

**Goal**: one engine, replacing both `MapDocument()` and the dormant `Map()`,
built on Phase 1's resolver.

**Tasks**:
- Design note first: the mapping-row schema. Extends the existing row shape
  (`cdaField`/`fhirPath`/`transform`/`valueMap`/`conformance`/`required`/`confidence`
  from `V149`) with two distinct kinds of conditionality — do not collapse
  these into one mechanism, they answer different questions:
  - **Rule selection** — `entryMatch` (classCode/moodCode/templateId on the
    entry itself) + `repeating`/`scope` (Phase 1's predicate resolver, e.g.
    `entryRelationships[typeCode=COMP]`) decide *which row applies at all*.
    Worked example from this session: Medication Free Text Sig (templateId
    `.147`, via `entryRelationship[typeCode=COMP]`) and Instruction(V2)
    (templateId `.20`, via `entryRelationship[typeCode=SUBJ,
    inversionInd=true]`) look structurally identical (`text`+`reference`)
    but target different FHIR fields (`dosage.text` vs
    `dosage.patientInstruction`) — two separate rows, each gated by its own
    `entryMatch`/scope predicate, not one rule with a flag.
  - **Value branching** — `condition`, evaluated *within* an already-matched
    row, picks the output value/path based on a runtime field value. Worked
    example: `negationInd=true` on an allergy/condition observation flips
    `verificationStatus` from `confirmed` to `refuted` — today an `if` in
    `allergy_mapper.go`/`condition_mapper.go`, becomes row config here.
- Implement one interpreter loop (modeled on `createFHIRResourceFromSection`,
  `generic_mapper.go:775`, but using the new schema) that: resolves matching
  entries → resolves repeating scopes → dispatches `CDATransformRegistry`
  transforms by name → validates via `TerminologyRegistry` where coded →
  writes via one shared path-writer (reuse/extend `setFHIRPath`).
- No real OOB data wired in yet — prove the engine against synthetic
  fixtures matching the design note's own examples.

### Design note (2026-06-21)

**Two corrections to this doc's own earlier framing, found by reading the
actual code before writing any — both change what Phase 2 reuses.**

1. **`CDATransformRegistry` is not the right reuse target.** Its
   `CDATransformFn` signature is `(entry map[string]interface{}, fieldKey
   string, vm map[string]string)` — it reads `entry[fieldKey]` plus
   convention-named companions (`fieldKey+"Display"`, `+"System"`, `+"Unit"`,
   `+"Family"`) off a **pre-flattened** map produced by the legacy XPath-based
   `GenericSectionProcessor`. That convention has nothing to do with Phase 1's
   resolver, which returns a clean resolved value (string/bool/map) directly
   from a path. More importantly, `cda_transform_registry.go`'s own built-in
   value maps (e.g. `medicationDefaultStatus`) are **not** the IG-verified
   ones Phase 0 fixed this session — those live in
   `services/cda_fhir/transforms/*.go` (`AllergyStatusToFHIR`,
   `CDACodeToCodeableConcept`, `MedicationRequestStatusToFHIR`, etc.), take
   **typed `cdadocument.*` structs**, and are what the production typed-tree
   mappers (`mappers/*.go`) actually call. Reusing `CDATransformRegistry`
   as-is would mean Phase 2's engine is *less* correct than the code it's
   meant to eventually replace.
   - **Decision**: a new, small `DeclarativeTransformRegistry`
     (`declarative_transform_registry.go`), named/registry-based exactly per
     the cross-cutting principle ("transform dispatch stays named,
     registry-based... same pattern even if not the same registry" — Phase
     1's note already established this convention for the path resolver
     itself), whose functions are **thin adapters**: re-marshal the resolved
     `interface{}` (already JSON-shaped, since it came from Phase 1's
     resolver) into the exact `cdadocument.*` struct the target `transforms.*`
     function expects, call it, return the result. Zero duplication of
     Phase-0-verified business logic; `cda_transform_registry.go` stays
     untouched (it keeps serving the dormant `Map()` path until Phase 4
     deletes both together).
2. **"`TerminologyRegistry`... already used by the dormant engine" is also
   wrong.** `generic_mapper.go` actually calls
   `services/cda_terminology.TerminologyService` — offline **format-only**
   regex validation (is this string shaped like a SNOMED/LOINC/RxNorm code)
   plus DB-backed per-interface code translation. `fhir/r4.TerminologyRegistry`
   (the component this doc's "What we're keeping" table actually names) is a
   completely different, complementary check: **ValueSet membership** against
   568 pre-compiled R4/US-Core ValueSets (is `"stopped"` actually a member of
   `medicationrequest-status`). Neither one is used by the other.
   - **Decision**: Phase 2 wires up `fhir/r4.GetTerminologyRegistry().Contains(valueSetURL,
     code)` as a new, optional capability — a `MappingRow` can declare a
     `ValueSetURL`, checked after transform/value-map resolution, on the
     **output** FHIR code. This is genuinely new value the dormant engine
     never had. `cda_terminology.TerminologyService`'s format check is left
     wired exactly where it already is (the dormant `Map()` path); nothing
     about it changes in Phase 2.

**Package**: `services/cda_fhir` (same package as `generic_mapper.go`/
`document_mapper.go`), not a new package. Reason: `setFHIRPath`/`ensureArray`/
`parseBracketIndex`/`stripResourcePrefix` (the shared path-writer the task
list calls for reusing) are unexported in `generic_mapper.go`. Rather than
exporting them (new public surface on code Phase 4 deletes) or duplicating
~80 lines of array/bracket-index logic, they're **moved** — not copied — into
a new file, `fhir_path_writer.go`, in the same package. `generic_mapper.go`
keeps calling them unchanged; Phase 2's engine calls the exact same functions.
When Phase 4 deletes `Map()`/`createFHIRResourceFromSection`,
`fhir_path_writer.go` is untouched and keeps serving the new engine alone.

**Schema** (`declarative_schema.go`) — two levels, because the doc's original
draft conflated two different questions under one `entryMatch` label:

```go
// MappingRule selects which CDA entries this whole resource-construction
// rule applies to, and which FHIR resource type they become. Multiple rules
// can target the same SectionKey with different EntryMatch + FHIRResource —
// e.g. Medications: EntryMatch="moodCode=INT" → MedicationRequest, then a
// second rule with EntryMatch="" → MedicationStatement. Rules are evaluated
// in slice order; the first whose EntryMatch matches a given entry wins for
// that entry (first-match-wins group dispatch — the same exclusivity
// medication_mapper.go's `if moodCode == "INT" {...} else {...}` already
// expresses in Go, made declarative).
type MappingRule struct {
    SectionKey   string // "allergiesAndIntolerances"
    FHIRResource string // "AllergyIntolerance"
    EntryMatch   string // Phase 1 predicate clause, no brackets: "moodCode=INT"; "" = every entry
    Fields       []MappingRow
}

// MappingRow is one field-level mapping within an already-selected entry.
// "Which row applies" (the doc's other entryMatch use, e.g. Free-Text-Sig vs
// Instruction(V2)) falls out naturally from Scope resolving to nothing — no
// separate boolean gate is needed: if Scope matches zero nodes, the row
// silently contributes nothing, exactly like a CDA document that simply
// lacks that entryRelationship.
type MappingRow struct {
    Scope         string            // Phase 1 path, relative to the matched entry; "" = the entry itself
    SourcePath    string            // Phase 1 path, relative to the Scope root
    FallbackPaths []string          // tried via ResolveCDAFallback when SourcePath resolves empty
    LiteralValue  interface{}       // used instead of SourcePath when SourcePath == ""
    CollectAll    bool              // true: every Scope match becomes one array element at TargetPath (RSON→reasonCode[], not just the first)
    TargetPath    string            // FHIR-side path within the resource (passed to setFHIRPath)
    Transform     string            // DeclarativeTransformRegistry name; "" = passthrough
    ValueMap      map[string]string
    ValueSetURL   string            // optional fhir/r4.TerminologyRegistry check on the output code
    Condition     *RowCondition     // value branching within this already-matched row
    Required      bool
    Conformance   string
}

// RowCondition evaluates WhenPath (relative to the row's own Scope root) and,
// when its resolved value stringifies to Equals, overrides this row's output
// for this one write. Equality only — deliberately not a general expression
// language, per the Performance checklist. This is the negationInd→refuted
// primitive: the row's own LiteralValue/Transform produce "confirmed";
// WhenPath="negationInd", Equals="true", ThenLiteralValue="refuted" produce
// the flip, as data instead of an `if` in allergy_mapper.go/condition_mapper.go.
type RowCondition struct {
    WhenPath         string
    Equals           string
    ThenLiteralValue interface{}
    ThenTransform    string
    ThenValueMap     map[string]string
    ThenTargetPath   string // "" = same TargetPath as the row's default
}
```

**Interpreter** (`declarative_engine.go`, `DeclarativeEngine.BuildResources`):
for one `MappingRule` against an already-parsed, JSON-round-tripped
`document.*` map (the same shape Phase 1's resolver already targets) —
1. Resolve the entry node-set: build
   `sectionsByKey.<SectionKey>.entries[<EntryMatch>]` (or `entries[*]` when
   `EntryMatch==""`) and call `executors.ResolveCDAPaths`. This is Phase 1's
   wildcard/predicate resolver applied at the section level — confirms the
   cross-cutting principle's bet that "the path resolver... built generically
   enough" would cover entryMatch for free, with no new parsing logic.
2. Per matched entry, build one resource map (`{"resourceType": rule.FHIRResource}`).
3. Per `MappingRow`: resolve `Scope` (relative to the entry) via
   `ResolveCDAPaths` → resolve `SourcePath`/`FallbackPaths`/`LiteralValue`
   relative to each scoped node (first node only, unless `CollectAll`) →
   evaluate `Condition` → dispatch the (possibly condition-overridden)
   `Transform` via `DeclarativeTransformRegistry` → optional `ValueSetURL`
   check via `fhir/r4.TerminologyRegistry` → `setFHIRPath(resource,
   TargetPath, value)` (array elements for `CollectAll` written via
   incrementing `TargetPath[i]` indices — `setFHIRPath`/`ensureArray` already
   handle that, no new array-write code needed).
4. Errors/warnings collected as `[]SectionError` — the exact type
   `MapOutput`/`ProcessingResult` already use, so this slots into the
   existing output contract without inventing a parallel error shape.

`BuildResources` takes one `MappingRule`. A second entry point,
`BuildResourcesForRules`, takes `[]MappingRule` and implements the
first-match-wins exclusivity `MappingRule`'s doc comment promises across
rules sharing a `SectionKey` (Medications' moodCode-driven
MedicationRequest/Statement split): it resolves each section's full entry
list once, and an entry already claimed by an earlier rule is skipped by a
later one targeting the same section — calling `BuildResources` once per
rule independently would let both rules claim the same entry, since each
call's `EntryMatch` is otherwise evaluated in isolation.

**No import cycle**: `services/cda_fhir` importing
`ezhealthkonnect/services/executors` (for `ResolveCDAPaths` et al.) is safe —
verified by reading `services/executors`' own imports (only `ezhealthkonnect/cda`
and `ezhealthkonnect/hl7`, neither of which imports `cda_fhir`) before adding
the import, not just trusting `go build` to catch it after the fact.

**Worked examples this phase's synthetic test suite proves, directly from
the inventory's own cross-cutting finding #1**:
- Allergy negation → `verificationStatus`: one rule (`EntryMatch=""`), one
  row (`Scope="entryRelationships[typeCode=SUBJ].entry"`,
  `LiteralValue="confirmed"`, `Condition{WhenPath:"negationInd", Equals:"true",
  ThenLiteralValue:"refuted"}`) — exercises *both* mechanisms (entry-level
  rule selection is trivial here; the interesting case is Scope navigating to
  the nested observation, then Condition branching the value) in one example.
- Medication Free Text Sig vs Instruction(V2): two rows on one rule, each
  with its own `Scope` predicate (`entryRelationships[typeCode=COMP].entry`
  filtered to `templateId=...147` vs
  `entryRelationships[typeCode=SUBJ,inversionInd=true].entry` filtered to
  `templateId=...20`) targeting `dosage.text` vs `dosage.patientInstruction`
  — proves "which row applies" needs no separate flag.
- RSON → `reasonCode[]`: one row, `CollectAll=true`,
  `Scope="entryRelationships[typeCode=RSON].entry"` — proves the
  "collect-all, not first-match" primitive the inventory's Medication section
  calls out by name (`indicationReasonCodes`).
- Medications moodCode dispatch: two rules, same `SectionKey="medications"`,
  `EntryMatch="moodCode=INT"` → `MedicationRequest` then `EntryMatch=""` →
  `MedicationStatement`, run via `BuildResourcesForRules` — proves
  first-match-wins exclusivity actually holds (an INT entry produces exactly
  one resource, not both).

**Exit criteria**: engine passes a synthetic test suite derived directly
from the design note. Benchmark against Phase 0's `MapDocument()` baseline
using equivalent synthetic load.

---

## Phase 3 — OOB Template Migration

**Status: in progress, slice-by-slice (see "Current State" above for the
per-slice process and what's done).** Slice 1 (Allergies/Medications/
Conditions) complete 2026-06-21. The remaining 12 inventoried sections are
follow-up sessions under this same phase, not a separate phase.

**Goal**: replace today's hardcoded mapper *knowledge* with declarative rows,
proven equivalent.

**Tasks**:
- Port every HIGH and MEDIUM confidence entry from Phase 0's inventory into
  actual mapping-row JSON, seeded via a new migration.
- Migrate the Go-only logic this session specifically fixed (negation →
  refuted, PIVL frequency → `dosage.timing.repeat`, RSON → `reasonCode`,
  SIG-text/Instruction(V2) resolution, severity from nested SUBJ
  observation) into declarative rows using Phase 2's `entryMatch`/`repeating`/
  `condition` capability.
- **Acceptance bar**: port every existing assertion in `mappers_test.go` and
  `cda_fhir_integration_test.go` to run against the new engine's output for
  the same inputs. 1:1 parity required — no partial credit.
- Re-run the multi-vendor corpus end-to-end (XML → FHIR this time, not just
  XML → JSON) and spot-check output shape against expectation — this is the
  "normative" check, distinct from the structural-completeness check already
  done.

**Exit criteria**: 100% existing test parity on the new engine, plus signed-off
corpus spot checks.

---

## Phase 4 — Cutover & Decommission

**Goal**: make the new engine the only engine.

**Tasks**:
- Recommended: **shadow-mode** cutover, not a hard switch. Run both engines
  in parallel for a defined window, diff outputs per message, alert on any
  mismatch. Given HIPAA-grade correctness stakes, prefer this over a
  flag-and-pray cutover.
- Once shadow-mode shows zero unexplained discrepancies over N runs, switch
  `cda_to_fhir_executor.go` to call only the new engine.
- Delete `MapDocument()`, the 16 files under `services/cda_fhir/mappers/`,
  and the old `Map()`/dormant interpreter in `generic_mapper.go`.

**Exit criteria**: old code removed from the codebase, shadow-mode diff
report attached to the PR as evidence, full `go test ./...` green.

---

## Phase 5 — Mapping Editor UI

**Goal**: let users actually exercise the override mechanism that's existed,
unused, since `V149`.

**Tasks**:
- Same "separate consumer, shared library" pattern as the backend (see
  above) — do **not** literally modify `enhanced-mapping-interface.js` to
  branch on format internally; that couples HL7 and CDA UI code exactly the
  way the backend split is designed to avoid.
- Instead: extract the format-agnostic pieces of that editor (mapping-row
  list, transform-name picker, value-map editor, a generic source/target
  field display) into shared, reusable components. Build a **separate**
  CDA mapping editor that composes those shared components and adds the
  CDA-only pieces (`entryMatch`, `repeating`/scope picker) — its own file,
  not a format-flag inside HL7's.
- Wire the new CDA editor to the existing delta-override REST endpoints in
  `controllers/cda_schema_controller.go` (already built, currently unused).
- RBAC per the security checklist above.

**Exit criteria**: a user can view, add, edit, and delete a CDA mapping row
through the UI and see it affect the very next message processed for that
interface.

---

## Phase 6 — Additive Sessions for Remaining Sections

**Goal**: the ongoing process the user explicitly asked for — extend
coverage to LOW-confidence/unmapped sections from Phase 0, gated by
verification, not guesswork.

**Per-section process** (repeat as a short, focused session per
section/group):
1. Re-derive field mappings from the IG with page+CONF citations.
2. Cross-check against corpus prevalence.
3. Human confidence sign-off before merging into the OOB seed.
4. Add regression tests (same convention as `mappers_test.go`'s
   one-fact-per-test style) before considering the section done.

This phase has no fixed exit criteria — it's the steady-state extension
process for as long as new sections/vendor quirks turn up.

---

## Open questions to resolve before/during Phase 0–1

- Where does the Phase 0 inventory live — a markdown table, or a structured
  JSON/YAML that could later double as a migration-generation input? Decide
  before writing it, not after.
- Shared value-set/transform infrastructure: should CDA and HL7 mapping
  share one `*_value_mappings` table (SNOMED/LOINC/RxNorm bindings are
  format-agnostic) instead of duplicating? Worth a short spike before Phase 3.
- Exact shadow-mode mechanics for Phase 4 (sampling rate? full traffic for a
  fixed window? where do diffs get reported?) — decide as part of that
  phase's design note, not improvised mid-cutover.
