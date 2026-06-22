# CDA→FHIR Mapping Inventory — Phase 0 Deliverable

**Status**: Phase 0 complete (first pass, all 15 sections)
**Produced**: 2026-06-21, for [CDA_FHIR_MAPPING_ENGINE_SPRINT_PLAN.md](CDA_FHIR_MAPPING_ENGINE_SPRINT_PLAN.md)
**Format choice**: Markdown tables (human-reviewed deliverable per the Phase 0 task). Phase 3 will transcribe HIGH/MEDIUM rows into the JSONB migration seed — this doc is the review artifact, not the seed itself.

## Methodology

- **Go mapper code** (`services/cda_fhir/mappers/*.go`) read in full per section — this is what actually runs in production via `MapDocument()`.
- **DB seed** (`database/migrations/V149__CDA_FHIR_Schema.sql`) compared field-by-field against the Go code for the 9 sections it covers.
- **Corpus prevalence**: grepped against the 4-vendor sample set (`cda/document/testdata/corpus/{cerner,kareo,mtuitive,practicefusion}_sample.xml`) — the only real-world corpus currently committed (not 14 vendors; that count refers to an earlier, separate XML→JSON fidelity pass on `generic_xml.go`, not this section-mapping inventory).
- **IG citations**: pulled live via web research against `build.fhir.org/ig/HL7/CDA-ccda*` and cross-checked against `cdasearch.hl7.org`/`art-decor.org` where reachable. **No CONF number in this document was recalled from training data** — every citation is either a verified CONF# (cited as such) or explicitly marked `UNVERIFIED — no citation found`. Treat UNVERIFIED cells as needing a direct read of the frozen R2.1 PDF before Phase 3, not as license to fabricate a number later.
- **Confidence rating**: HIGH = implemented + tested today, or directly IG-cited; MEDIUM = clear IG guidance, not implemented/tested yet, or implemented-but-untested; LOW = ambiguous, vendor-variable, under-specified, or contradicted by corpus evidence.

---

## Cross-cutting findings (read before the per-section tables)

These patterns recur across multiple sections and matter more to Phase 1–2's engine design than any single field row:

1. **The flat `{cdaField, fhirPath, transform, valueMap}` row shape cannot express most of what the Go mappers actually do.** Every section that uses the "outer concern act wraps an inner assertion observation via SUBJ, with REFR/flat fallback" idiom (Allergies, Conditions), the "templateId-discriminated entryRelationship dispatch" idiom (Medication Free Text Sig `.147` vs Instruction(V2) `.20` — byte-identical shape, distinguished only by templateId), the "nested SUBJ severity observation coded SEV" idiom (Allergies, Conditions), or moodCode-driven resource-type dispatch (Medications: INT→MedicationRequest vs else→MedicationStatement) needs primitives Phase 1's path resolver and Phase 2's `entryMatch`/`repeating`/`condition` design must supply: wildcard/predicate traversal, templateId-conditional dispatch, and a "field A, else field B" fallback chain. This is exactly the gap Phase 1/2 of the parent sprint plan already anticipated — this inventory confirms it with concrete examples per section.
2. **Reverse gaps exist**: the V149 seed is sometimes *ahead* of the Go code, not just behind it. `authorGiven`/`authorNPI` → `recorder` rows are declared in the seed for Allergies, Medications, and Conditions but implemented in **none** of the three corresponding Go mappers. Phase 3 must treat seed rows as proposals to verify against Go, not assumed-correct ground truth.
3. **Two parallel, non-overlapping mapping mechanisms already exist** (confirmed independently by the Encounters/Procedures/Practitioner research): the typed-struct mappers (production path) and a generic JSON-engine registry (`cda_transform_registry.go` + V149 seed, not wired to `MapDocument()`). This is the exact "live path vs. dormant path" split the parent sprint doc opens with — Phase 0 confirms it extends to every section, not just the ones called out in the doc's introduction.
4. **Status-value-map disagreements between Go and seed — RESOLVED 2026-06-21, verified against HL7's official C-CDA-on-FHIR ConceptMaps** (not guessed, not picked by preference): fetched `ConceptMap/CF-ProcedureStatus` and `ConceptMap/CF-MedicationStatus` directly from `github.com/HL7/ccda-on-fhir`. Result: **the V149 seed's Procedure valueMap was already correct**; Go's `ProcedureStatusToFHIR` had the bug (`aborted` was mapped to `not-done`, should be `stopped` — `aborted` and `cancelled` are NOT synonyms despite sharing one CDA source value set). Fixed in `status_transforms.go`. Also found, not previously flagged: the official IG has **no consensus mapping at all** for `moodCode="EVN"` (`MedicationStatement`) — the IG's own text says "no consensus was established... we welcome feedback" — so `MedicationStatusToFHIR` cannot be "verified" the way `MedicationRequestStatusToFHIR` now can; it's documented as a best-effort default constrained to MedicationStatement's actual value set (which has no `cancelled` code at all). See `status_transforms.go` doc comments for full citations. The V149 seed's medication valueMap is a correct *subset* of the official Request-side mapping but still architecturally can't express the Request/Statement resource-type split — that's deferred to Phase 2's schema work, not patched ad hoc, since seed isn't live in production.
5. **Multiple Practitioner-construction code paths are inconsistent.** Three independent paths exist (`patient_mapper.go:MapAuthor`, dead-code `practitioner_mapper.go:MapPractitioners`, and the actually-used `careteam_mapper.go` → `buildPractitionerResource`). `MapAuthor` only ever emits the *first* author on a multi-author document (PracticeFusion's real sample has 3) and never reads `RepresentedOrganization` or `AssignedAuthoringDevice`. The exported `MapPractitioners` is confirmed dead code (zero callers outside its own file).
6. ~~**Legal Authenticator is unparsed, not just unmapped**~~ — **RESOLVED 2026-06-22**: `CDALegalAuthenticator` struct + `parseLegalAuthenticator()` added, declarative rule ported (`LegalAuthenticatorMappingRules()`). See section 10.
7. **`document.componentOf/encompassingEncounter`** (discharge disposition, NUBC codes, facility location) and **`documentationOf/serviceEvent/performer`** (NPI-bearing document-level performer) are both fully parsed into `CDAHeader` but never read by any mapper — confirmed by exhaustive grep. Lower-effort than Legal Authenticator since parsing already exists; pure wiring gaps.
8. **Real-world data quality issues surfaced by the corpus that the engine must tolerate, not just the happy path**:
   - Malformed `<id root="2.16.840.1.113883.4.6"/>` (NPI system asserted, no `@extension` digits) — PracticeFusion, 3 occurrences. Current `IIToIdentifier()` falls back to root-as-value, producing a syntactically-valid-but-semantically-garbage Identifier (`value` = the NPI OID string itself).
   - Empty self-closing `PIVL_TS` elements with no `<period>` child (Kareo, medication frequency) — Go's nil-guard correctly skips these, but it means "PIVL_TS present" ≠ "frequency derivable."
   - SNOMED reaction-severity code `255604002` ("Very Mild") is unmapped in both the Go switch and the seed valueMap for AllergyIntolerance severity — falls through to a default of `moderate`, arguably wrong (closer to `mild`). Confirmed present in Kareo's real allergy data.
   - Cerner's immunization/allergy "no information" idiom uses `nullFlavor="NI"` rather than `negationInd`, a second no-data pattern the negation logic doesn't directly address (though the Go code's existing fallback chains mostly absorb it correctly).
9. **A latent bug was surfaced, not just gaps**: `condition_mapper.go`'s own doc comment says the shared `MapConditions` function serves both the Problems section and the Health Concerns section, but `category` is unconditionally hardcoded to `problem-list-item` with no branch for Health Concerns' `health-concern` category. Zero corpus instances currently exercise the Health Concerns path, so this is latent, not yet observed in production.
10. **One confirmed correctness bug with direct corpus evidence**: Immunization `negationInd` (C-CDA SHALL [1..1] per CONF:1198-8985) is never read by `MapImmunizations`. Kareo's real corpus sample has a pneumococcal immunization entry with `negationInd="true"` + `statusCode="completed"` — the current Go mapper outputs `Immunization.status = "completed"` for this entry, **asserting a vaccine was given when the source document says it was refused**. This is the highest-priority correctness finding in this inventory.

---

## 1. Allergies and Intolerances

**Go mapper**: `services/cda_fhir/mappers/allergy_mapper.go` · **Seed**: V149 `allergiesAndIntolerances` (partial)

| CDA Field | C-CDA R2.1 Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| Allergy Concern Act `statusCode` | Allergy Concern Act `.4.30`, CONF:1198-7485/19086 | `AllergyIntolerance.clinicalStatus` | `AllergyStatusToFHIR` | `allergy_mapper.go:39` | HIGH | 3/4 (Cerner/Kareo/PF) | Seed's `status` row matches Go exactly |
| Concern Act `effectiveTime/low` (onset) | `.4.30`, CONF:1198-7498/7504 | `AllergyIntolerance.onsetDateTime` | `CDATimeRangeToOnset` | `allergy_mapper.go:42` | HIGH | Kareo/PF present; Cerner `nullFlavor=NI` | Seed maps a flat field, not the outer-act traversal Go does |
| SUBJ traversal → Allergy-Intolerance Obs (with flat fallback) | CONF:1198-7509/7915/14925 | structural | `findRelByTypeCode(...,"SUBJ")` | `allergy_mapper.go:47-51` | HIGH | 4/4 use SUBJ pattern | **Not representable in flat seed schema at all** — biggest structural gap |
| `negationInd` (outer or inner) → `verificationStatus` | Allergy-Intolerance Obs V2 `.4.7`, CONF:1098-31526 | `AllergyIntolerance.verificationStatus` | inline (`entry.NegationInd \|\| obs.NegationInd`) | `allergy_mapper.go:56-68` | HIGH — tested (`mappers_test.go:171-252`) | Cerner: NI idiom; PF: real unrefuted entries; `negation_and_frequency.xml` fixture exercises true case | **Zero seed representation** — tested/shipped, no DB row at all |
| Obs `value` (CD) → `type` | CONF:1098-7390 | `AllergyIntolerance.type` | `AllergyTypeToFHIR` | `allergy_mapper.go:71-73` | HIGH | Cerner 419511003 (default branch); PF 416098002 (exact match) | Not in seed |
| CSM participant → substance code/category | CONF:1098-7402-7407/7419 | `AllergyIntolerance.code` + `.category` | `CDACodeToCodeableConcept` + `AllergyCategoryFromSubstanceSystem` | `allergy_mapper.go:76-85` | HIGH | Kareo/PF: RxNorm codes; Cerner: `nullFlavor=NI` (triggers fallback below) | Seed's flat triad is a partial match, no category derivation |
| Fallback: assertion `value` when CSM absent | UNVERIFIED — Go-side US Core conformance workaround, not IG-cited | `AllergyIntolerance.code` (fallback) | `CDACodeToCodeableConcept(obs.Value.Code)` | `allergy_mapper.go:93-97` | HIGH — tested | Cerner's "no info" entry is the real trigger | Needs a fallback-chain primitive, not a flat row |
| MFST → Reaction Obs `value`/`code` | Reaction Obs V2 `.4.9`, via CONF:1098-7447/7907/7449/15955; Reaction Obs itself CONF:1098-7335 | `AllergyIntolerance.reaction[].manifestation` | `CDAValueToFHIR` / `CDACodeToCodeableConcept` | `allergy_mapper.go:101-112` | HIGH | Kareo/PF present (2 reactions); Cerner NI | Not in seed |
| Nested SEV (MFST→SUBJ) → severity | Severity Obs V2 `.4.8`, CONF:1098-19169 (code=SEV), CONF:1098-7356 (value) | `AllergyIntolerance.reaction[].severity` (fixed mild/moderate/severe) | `allergySeverityCode` switch | `allergy_mapper.go:113-119,132-143` | HIGH | Kareo 255604002 "Very Mild" **unmapped → defaults moderate** (likely fidelity gap); Cerner 24484000 exact match | Seed has identical valueMap/gap — Kareo case confirms a real fidelity issue, not just a Go quirk |
| `authorGiven`/`authorNPI` → `recorder` | UNVERIFIED (general Author Participation guidance only) | `.recorder.display`/`.identifier.value` | `cda_name_to_fhir`/`string_direct` | **NOT implemented in Go** — seed-only | MEDIUM | PF has entry-level `<performer>` | Reverse gap: seed ahead of Go |

**Mechanics**: Negation checked at both outer-act and inner-observation level (real "No Known Allergies" docs put it on the inner one). Severity is a double-nested lookup (MFST→SUBJ→value) duplicated almost verbatim in Conditions — candidate for a shared engine primitive rather than per-mapper Go.

---

## 2. Medications

**Go mapper**: `services/cda_fhir/mappers/medication_mapper.go` · **Seed**: V149 `medications` (partial, MedicationStatement-only)

| CDA Field | C-CDA R2.1 Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| `moodCode` dispatch (INT vs EVN) | Medication Activity V2 `.4.16`, CONF:1098-7497 | Resource type: INT→`MedicationRequest`, else→`MedicationStatement` | dispatch in `MapMedications` | `medication_mapper.go:22-36` | HIGH | Cerner/Kareo: `moodCode=INT` (majority pattern) | **Architecturally absent from seed** — one section→one fhirResource assumption breaks here |
| `consumable.../code` | CONF:1098-7520 | `medicationCodeableConcept` | `CDACodeToCodeableConcept` | `medication_mapper.go:56-63,111-118` | HIGH | All 4 vendors | Seed's flat `drugCode` loses the nested traversal path |
| `statusCode` (Request path) | CONF:1098-7507/32360; status values verified against official `ConceptMap/CF-MedicationStatus` (targetUri=`medicationrequest-status`) | `MedicationRequest.status` | `MedicationRequestStatusToFHIR` — **fixed 2026-06-21**: `aborted` now correctly maps to `stopped` (was `cancelled`, the bug); `cancelled` maps to FHIR's own distinct `cancelled` code; added `nullified→entered-in-error` | `medication_mapper.go:50` | HIGH — verified against official ConceptMap, tests added | Cerner active/completed | Seed still only models the Statement-side valueMap — architecturally absent for Request, deferred to Phase 2's multi-resource-type schema, not patched ad hoc since the seed isn't live |
| `statusCode` (Statement path) | same; **no official ConceptMap exists for this direction** — IG text: "no consensus was established" for moodCode=EVN (`CF-medications.md`) | `MedicationStatement.status` | `MedicationStatusToFHIR` (aborted/cancelled→stopped — both must land on `stopped` since MedicationStatement's value set has no `cancelled` code at all; added `nullified→entered-in-error`) | `medication_mapper.go:108` | MEDIUM — best-effort default, not spec-verifiable (no official guidance exists) | — | The seed happens to match this one since it's a correct subset of the Request-side mapping |
| Requester (performer/author, fallback to literal string) | CONF:1098-7522/31150; requester-non-empty is a US Core-21 derived rule, not C-CDA-cited | `MedicationRequest.requester` | `requesterReference` (3-tier fallback, never empty) | `medication_mapper.go:262-280` | HIGH — tested (`mappers_test.go:122-167`) | 0/4 corpus has entry-level performer on meds — fallback-to-literal is the realistic path, not the happy path | Zero seed representation |
| `routeCode` | CONF:1098-7514 | `dosage[].route` | `CDACodeToCodeableConcept` | `medication_mapper.go:72-76,129-133` | HIGH | All 3 (Oral, NCI Thesaurus) | Seed captures bare code only, not full CodeableConcept |
| `doseQuantity` | CONF:1098-7516/16878/16879 | `doseAndRate[].doseQuantity` | `CDAQuantityToFHIR` | `medication_mapper.go:77-83,134-140` | HIGH | Kareo `1.0 cap(s)`; `supply_and_nullflavors.xml` exercises `nullFlavor=UNK` | Seed splits into 2 flat fields, loses UCUM system/code echo |
| **PIVL_TS/EIVL_TS → `timing.repeat`** | CONF:1098-7513/9106/28499 | `dosageInstruction[].timing.repeat` | `applyDosageTiming` (hardcodes frequency=1) | `medication_mapper.go:158-182` | HIGH — tested + fixture-exercised | Kareo: 3 PIVL_TS instances, **all empty** (no `<period>` child) — Go's nil-guard correctly no-ops | **Completely absent from seed.** Clearest "tested-and-shipped but undocumented" gap |
| **Free Text Sig (COMP, templateId `.147`) → `dosage.text`** | `.4.147`, CONF:81-32775/32780/32781/32754/32755/32774; parent linkage CONF:1098-32907-32909 | `dosage[].text` | `applyDosageText` (templateId-discriminated) | `medication_mapper.go:184-223` | HIGH — tested + fixture-exercised | Cerner uses bare `<text><reference>` directly on the substanceAdministration instead (bypasses the templated COMP path) | Zero seed representation. templateId is the **only** discriminator vs. Instruction(V2) below — needs conditional-dispatch primitive |
| **Instruction(V2) (SUBJ+inversionInd, templateId `.20`) → `dosage.patientInstruction`** | `.4.20`, CONF:1098-10503/16884/7396/19106; parent linkage CONF:1098-7539/7540/7542/31387 | `dosage[].patientInstruction` | `applyDosageText` | `medication_mapper.go:184-223` | HIGH — tested + fixture-exercised | Kareo confirms real-world use of this template (code 311401005), though with `nullFlavor=NI` text in that sample | Same gap/discriminator issue as Free Text Sig |
| **RSON → Indication → `reasonCode`** | Indication `.4.19`; linkage CONF:1098-7536/7537/16087 | `reasonCode[]` | `indicationReasonCodes` (collects ALL matches) | `medication_mapper.go:225-251` | HIGH — tested | 0/4 corpus evidence in visible excerpts | Not in seed; "collect-all not first-match" design needs a repeating-relationship primitive |
| `effectiveTime` → period/authoredOn | CONF:1098-7508/32890/32776/32777 | `effectivePeriod` (Statement) / `authoredOn` (Request, single-value only) | `CDATimeRangeToPeriod` / `CDATimeRangeToOnset` | `medication_mapper.go:66-68,121-125` | HIGH | All vendors; Kareo `high=UNK` correctly omitted, not emptied | Seed's `startDate`/`stopDate` matches Statement side; `authoredOn` (Request) has no seed row at all |

**Mechanics**: SIG-text resolution (Free Text Sig vs Instruction V2) is the most structurally interesting case in the whole inventory — byte-identical `<entryRelationship><text><reference>` shape, distinguished **only** by templateId, with typeCode/inversionInd as a secondary signal. Both rely on upstream narrative-text resolution that's a preprocessing dependency the declarative engine must also model.

---

## 3. Problems / Conditions

**Go mapper**: `services/cda_fhir/mappers/condition_mapper.go` · **Seed**: V149 `problems` (partial)

| CDA Field | C-CDA R2.1 Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| Problem Concern Act `statusCode` (read from inner obs) | Problem Concern Act V3 `.4.3`, CONF:1198-9029/31525 | `clinicalStatus` | `ConditionStatusToFHIR` (handles `remission`, 4th branch seed omits) | `condition_mapper.go:65` | HIGH | Cerner/Kareo consistent | Seed matches Go's actual (inner-observation) behavior, name doesn't make that explicit |
| SUBJ (primary) / **REFR (Health Concerns fallback)** | CONF:1198-9034/9035/15980 (SUBJ); REFR-on-Health-Concerns: UNVERIFIED | structural | `findRelByTypeCode` SUBJ→REFR→flat | `condition_mapper.go:39-45` | HIGH (SUBJ) / MEDIUM (REFR, unverified+uncorroborated) | 0/4 corpus uses REFR (no Health Concerns section in any sample) | Not representable in flat seed schema |
| Obs `value` (polymorphic CD/PQ/BL/etc.) → `code` | Problem Obs V3/V4 `.4.4`, CONF:4515-9058/16749/16750 | `Condition.code` | `CDAValueToFHIR` (full polymorphic dispatch) | `condition_mapper.go:48-54` | HIGH | Cerner: SNOMED+ICD-9 translation (legacy, not ICD-10); Kareo: clean SNOMED | Seed's flat `string_direct` is much narrower than Go's polymorphic handling |
| Fallback: `code` when `.value` empty | UNVERIFIED | `Condition.code` (fallback) | `CDACodeToCodeableConcept` | `condition_mapper.go:58-62` | MEDIUM | 0/4 — all corpus uses the value path | Mirrors Allergy's identical fallback idiom |
| `negationInd` → `verificationStatus` | Problem Obs V4, CONF:4515-10139 | `verificationStatus` | identical pattern to Allergy | `condition_mapper.go:67-81` | HIGH — tested | **0/4** — no real "no known problems" assertions in corpus (unlike Allergies, which has 2) | Zero seed representation; weaker real-world corroboration than the Allergy equivalent |
| Nested SEV → `severity` (full CodeableConcept, not fixed enum) | Severity Obs `.4.8` (cross-template link to Problem Obs: UNVERIFIED) | `Condition.severity` | `findRelByTypeCode` SUBJ filtered code=SEV + `CDACodeToCodeableConcept` | `condition_mapper.go:83-93` | HIGH — tested | 0/4 corpus evidence | Explicit doc-comment reuse of the Allergy idiom, applied without direct corpus corroboration |
| `category` = `problem-list-item` (hardcoded) | UNVERIFIED — section-level distinction, not entry-level CONF | `category[].coding[0]` | hardcoded literal | `condition_mapper.go:96-106` | MEDIUM | Both corpus vendors are Problem-List-only (hardcode happens to be correct for 100% of this corpus) | **Latent bug**: doc comment says this function serves both Problems AND Health Concerns sections, but category is never branched — Health Concern entries would get the wrong category once that section starts flowing through |
| `effectiveTime/low` → onset | CONF:4515-15603 | `onsetDateTime` | `CDATimeRangeToOnset` | `condition_mapper.go:109-111` | HIGH | Cerner/Kareo present | Seed match reasonable |
| `effectiveTime/high` → abatement | CONF:4515-15604 | `abatementDateTime` | `CDATimeToFHIRDateTime` | `condition_mapper.go:114-116` | HIGH | **Both** vendors consistently emit `high nullFlavor=UNK`, correctly omitted | **Not in seed at all** — confirmed common real-world pattern entirely missed |
| `authorGiven`/`authorNPI` → `recorder` | UNVERIFIED | `.recorder.*` | — | **NOT implemented** — seed-only | MEDIUM | Document-level author present; entry-level not observed | Reverse gap, same as Allergies |

**Mechanics**: Closest sibling to Allergies — same SUBJ-wrap idiom, same negation pattern, same nested-SEV severity lookup. Corpus shows an asymmetry: negation/severity are real and common for Allergies but **zero** observed instances for Problems in this 4-vendor sample, despite identical, well-tested Go logic — validated only against synthetic fixtures so far, not real vendor exports.

---

## 4. Vital Signs

**Go mapper**: `services/cda_fhir/mappers/observation_mapper.go` (`MapObservations(..., "vital-signs")`) · **Seed**: V149 `vitalSigns`

No LOINC-range heuristic distinguishes Vitals from Results — dispatch is purely by which section key called `MapObservations`, with a `category` string literal passed through (`"vital-signs"` vs `"laboratory"`).

| CDA Field | Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| Vital Signs Organizer presence | Vital Signs Section `.2.4.1`, CONF:1198-15964 (organizer `.4.26`) | n/a (flattened) | `flattenObservationEntries` | `observation_mapper.go:47-89,161-171` | HIGH | Kareo (x3), PF (x2); Cerner/Mtuitive 0 | Correctly not mapped to its own resource |
| Obs `code` (LOINC) | Vital Sign Obs `.4.27`, CONF:1098-7301/32934 | `code.coding[0]` | `CDACodeToCodeableConcept` | `observation_mapper.go:227-229` | HIGH | Kareo: 8302-2/3141-9/39156-5 | Seed's `string_direct` omits display/system Go actually sets |
| Obs `statusCode` | CONF:1098-7303/19119 | `Observation.status` | `ObservationStatusToFHIR` | `observation_mapper.go:209` | HIGH | Kareo: all completed | Go supports more statuses than IG's completed-only constraint — defensively broader |
| Obs `value` (PQ) | CONF:1098-7305/31579 | `valueQuantity` | `CDAQuantityToFHIR` | `observation_mapper.go:301-306` | HIGH | Kareo: height/weight/BMI | |
| Obs `effectiveTime` | CONF:1098-7304 | `effectiveDateTime` | `CDATimeRangeToOnset` | `observation_mapper.go:232-234` | HIGH | Kareo (date-only) | |
| Systolic+Diastolic → combined BP panel | No single CONF — derived from FHIR base "bp" profile, not CDA-side | `code=85354-9` + 2 `component`s | `buildBloodPressureObservation`/`buildBPComponent` | `observation_mapper.go:47-156` | HIGH — tested (`mappers_test.go:61-118`) + HAPI-validated | Kareo/PF carry BP only as narrative text, **not** as discrete 8480-6/8462-4 coded entries — real structured-BP corpus evidence is 0/4 | Second, independent assembly-layer implementation exists (`assembly/rules/bp_panel_rule.go`) as defense-in-depth for entries that escape mapper-level pairing |
| `category` = vital-signs | n/a (FHIR-side) | `category[0].coding[0]` | `categorySystem` | `observation_mapper.go:98-108,212-224` | HIGH | n/a | Correctly uses base CodeSystem |
| Performer/Author Participation | CONF:1098-7310 | `Observation.performer` | — | **NOT implemented** | MEDIUM | Not present in any corpus sample | Inconsistent with Results' seed, which *does* declare performer fields |
| `dataAbsentReason` fallback | n/a (US Core invariant) | `dataAbsentReason` | `dataAbsentReasonUnknown` | `observation_mapper.go:257-276` | HIGH | n/a | Satisfies us-core-2 uniformly across Vitals/Results/SocialHistory |

---

## 5. Results (Laboratory)

**Go mapper**: same `observation_mapper.go` (`MapObservations(..., "laboratory")`) · **Seed**: V149 `results`

| CDA Field | Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| Result Organizer presence | Results Section `.2.3.1`, CONF:1198-15515; Organizer `.4.1`, CONF:4537-7124/7128 | n/a | `flattenObservationEntries` | `observation_mapper.go:161-171` | HIGH | PF: organizer present but fully `nullFlavor=NI` | Same flattening as Vitals, no Results-specific handling |
| Obs `code` (LOINC) | Result Obs `.4.2`, CONF:4537-7133/19212 | `code.coding[0]` | `CDACodeToCodeableConcept` | `observation_mapper.go:227-229` | HIGH | PF: `26436-6` but value/status fully null | |
| Obs `statusCode` | CONF:4537-7134 | `Observation.status` | `ObservationStatusToFHIR` | `observation_mapper.go:209` | HIGH | PF: completed (on an otherwise null entry) | **Seed's `cda_result_status_to_fhir` and Go's `ObservationStatusToFHIR` disagree**: registry has no `held` case (Go maps it to `registered`); registry passes through unmapped codes, Go defaults to `final` |
| Obs `value` (polymorphic) | CONF:4537-32610 | `value{Quantity,CodeableConcept,String}` | `setObservationValue` type switch | `observation_mapper.go:295-336` | HIGH | PF `nullFlavor=NI` → correctly produces `dataAbsentReason` | |
| `referenceRange` | CONF:4537-7150/7151 | `Observation.referenceRange` | registered (`cda_transform_registry.go:180`) | **registered but never called from the mapper** | MEDIUM | 0/4 | Genuine gap between registry intent and mapper reality — seed lists the key, code doesn't wire it |
| `interpretationCode` → `interpretation` | CONF:1198-7147, confirmed via WebFetch against `build.fhir.org/ig/HL7/CDA-ccda-2.1-sd` 2026-06-22: direct child of the observation, sibling of code/statusCode/value | `Observation.interpretation` | `cda_code_to_codeable_concept` | **RESOLVED 2026-06-22**: added `CDAEntry.InterpretationCode` + direct-sibling parse (`entry_parser.go`); `observation_mapper.go` now reads it directly instead of a COMP child's value | HIGH — confirmed bug fix + 4 new tests | Kareo: 5/5 vitalSigns entries carry direct-sibling `interpretationCode`, now correctly produce `interpretation` (verified via corpus test) | Was a confirmed structural gap (Go read a COMP-nested value, a different element than the IG-correct direct child) — now fixed at the parser, Go-mapper, and declarative-rule layers together |
| `performer` | UNVERIFIED | `.performer[0].*` | seed: `cda_name_to_fhir`/`string_direct` | **NOT implemented in Go** — seed-only | LOW | Not present in corpus examined | |
| `category` = laboratory | n/a | `category[0].coding[0]` | `categorySystem` | `observation_mapper.go:212-224` | HIGH — HAPI-validated | n/a | |

**Note**: PracticeFusion's Results section is a real-world instance of fully null-flavored organizer+observation with no COMP child to substitute from — Go's existing nil-guard correctly produces zero resources here, not a bug, but means corpus prevalence for real lab values is effectively zero across all 4 samples.

---

## 6. Social History (incl. Smoking Status)

**Go mapper**: same `observation_mapper.go` (`MapObservations(..., "social-history")`) — **no smoking-specific Go code exists at all**; it's a generic Observation. Note also a structurally separate legacy parser (`services/parsers/cda/section_social_history.go`) producing a different untyped shape — this is the actual source of the seed's field names (`observationCode`, `value`, `valueDisplay`), **not** the typed mapper.

| CDA Field | Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| Social History Section entries | `.2.17`, CONF:1198-7953/14821 | n/a | generic entries | — | HIGH | Kareo/PF: section present (1 each) | |
| Smoking Status `code` (LOINC 72166-2) | Smoking Status MU V2 `.4.78`, CONF:1098-31039/32157 | `code.coding[0]` | `CDACodeToCodeableConcept` (generic, not smoking-aware) | `observation_mapper.go:227-229` | HIGH (citation) / MEDIUM (generic impl) | **Both real samples deviate** from the IG-mandated 72166-2: Kareo uses an `ASSERTION` shell, PF shows only the value coding | Real vendor data diverges from spec in two different ways |
| Smoking Status `value` (CD, SNOMED) | CONF:1098-14810/14817/31019 | `valueCodeableConcept` | `setObservationValue` case CD | `observation_mapper.go:307-312` | HIGH | Kareo: 266927001 "Unknown if ever smoked" (exact special-case match); PF: 266919005 "Never smoker" | Confirms the no-dedicated-mapper architecture is sound for this field |
| Smoking Status `statusCode` | CONF:1098-14809 | `Observation.status` | `ObservationStatusToFHIR` | `observation_mapper.go:209` | HIGH | Kareo: completed | |
| Smoking Status `effectiveTime` | CONF:1098-31928 | `effectiveDateTime` | `CDATimeRangeToOnset` | `observation_mapper.go:232-234` | MEDIUM | Kareo: both low/high `nullFlavor=NI` | Behavior on all-null IVL_TS not directly source-verified in this pass |
| Pregnancy/Sexual Orientation/Gender Identity | UNVERIFIED, no per-field IG check done | generic `code`/`value` | same generic path | — | LOW | 0/4 corpus | Seed lists these as schema/UI metadata keys only — no Go logic distinguishes them from any other social-history Observation |
| `category` = social-history | n/a | `category[0].coding[0]` | `categorySystem` | `observation_mapper.go:212-224` | HIGH | n/a | |

---

## 7. Immunizations

**Go mapper**: `services/cda_fhir/mappers/immunization_mapper.go` · **Seed**: V149 `immunizations`

| CDA Field | Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| Immunization Activity presence | `.2.2.1`, CONF:1198-15494/9019; entry `.4.52` | n/a | `MapImmunizations` loop | `immunization_mapper.go:19-28` | HIGH | Kareo/PF: `.4.52` present | |
| `vaccineCode` (CVX) | Immunization Medication Info V2 `.4.54`, CONF:1098-9007 | `vaccineCode.coding[0]` | `CDACodeToCodeableConcept` | `immunization_mapper.go:42-49` | HIGH | Kareo: CVX 33 | |
| `occurrenceDateTime` | CONF:1198-8834 | `occurrenceDateTime` | `CDATimeRangeToOnset` | `immunization_mapper.go:51-54` | HIGH | Kareo: single-point date | |
| `status` | No direct CONF; derived FHIR field per official C-CDA-on-FHIR mapping table | `Immunization.status` | `ImmunizationStatusToFHIR` | `immunization_mapper.go:39` | **LOW — confirmed bug** | Kareo: `statusCode=completed` + `negationInd=true` → Go outputs `"completed"` | **Cross-cutting finding #10**: `entry.NegationInd` never read; `"refused"` branch in the transform is dead code |
| `@negationInd` (refused) | CONF:1198-8985 (SHALL [1..1]) | `status="not-done"` + `statusReason` | **none** | **NOT implemented** | HIGH confidence this is IG-mandated + unimplemented | Kareo: 1/1 immunization entries negated (100% of this vendor's sample) | Highest-priority gap in this section |
| Immunization Refusal Reason | `.4.53`, code system `2.16.840.1.113883.5.8`; exact CONF# UNVERIFIED (template/example confirmed via cdasearch) | `Immunization.statusReason` | **none** | **NOT implemented** | MEDIUM | Kareo's negated entry has no refusal-reason child present | Even if negationInd were read, no code looks for the RSON child |
| `route` | CONF:1198-8839 | `Immunization.route` | `CDACodeToCodeableConcept` | `immunization_mapper.go:57-61` | HIGH | Not present in Kareo's entry | |
| `doseQuantity` | CONF:1198-8841/8842 | `doseQuantity` | `CDAQuantityToFHIR` | `immunization_mapper.go:64-68` | HIGH | Kareo: 2.0 mL | |
| `lotNumber` | CONF:1098-9014 | `lotNumber` | direct passthrough | `immunization_mapper.go:71-76` | HIGH | Kareo: lot 3422 | |
| `performer` | UNVERIFIED | `performer[0].actor.display` | local helper | `immunization_mapper.go:79-92` | MEDIUM | Not present in Kareo entry | Code reads `Participants[]`, but `Performers []CDAPerformer` is the field used elsewhere (medication requester) — possible wrong-slice bug, needs verification |
| Manufacturer | CONF:1098-9012 | `Immunization.manufacturer` | — | **NOT implemented** | MEDIUM | Kareo: `manufacturerOrganization/name` present and unused | Direct corpus evidence of an unmapped SHOULD field with real data available |

---

## 8. Encounters

**Go mapper**: `services/cda_fhir/mappers/encounter_mapper.go` · **Seed**: V149 `encounters` (partial — Go does more)

| CDA Field | Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| `entry.Code` → dual class+type | Encounter Activity V3 `.4.49` (code CONF# not isolated on rendered page — UNVERIFIED) | `class.coding[0]` AND `type[0].coding[0]` (both, from the same code) | inline + `CDACodeToCodeableConcept` | `encounter_mapper.go:40-54` | HIGH (impl) | Kareo/PF: entries present but `nullFlavor`; real code only in `full_ccd_nist.xml` fixture | **Architecturally inconsistent**: raw CDA code (often CPT/SNOMED) is dual-mapped into `class`, which per FHIR base spec expects ActEncounterCode vocabulary — likely fails strict terminology validation |
| `Code.DisplayName` | UNVERIFIED | `class`/`type` display | guarded direct assignment | `encounter_mapper.go:46-49` | HIGH — tested (`mappers_test.go:449-475`) | — | Explicit empty-string guard (documented bug fix) |
| `StatusCode` | UNVERIFIED (CONF# not located) | `Encounter.status` | `EncounterStatusToFHIR` | `encounter_mapper.go:37` | HIGH | n/a | FHIR status values `planned/arrived/triaged/onleave` have no CDA source — sensible, not a gap |
| `EffectiveTime` | UNVERIFIED | `period.start/.end` | `CDATimeRangeToPeriod` | `encounter_mapper.go:58-60` | HIGH | Kareo/PF: `nullFlavor=NI` | |
| Participant (performer) | UNVERIFIED, loosely Author-Participation-adjacent | `participant[].individual.display` | inline (`buildDisplayFromName`) | `encounter_mapper.go:62-72,102-131` | MEDIUM | No corpus instance populated | `individual` is **display-only**, never a `Reference(Practitioner)` — will fail strict US Core validation requiring a reference |
| Location (entryRelationship LOC) | Service Delivery Location `.4.32` (CONF# UNVERIFIED) | `location[0].location.display` | inline | `encounter_mapper.go:74-97` | MEDIUM | Not present in corpus | Same Reference-vs-display weakness |
| `componentOf/encompassingEncounter` (id, effectiveTime, dischargeDisposition, facility) | dischargeDispositionCode: CONF:1198-32177/32377 (verified) | **no FHIR path produced** | none | **NOT implemented** — fully parsed into `CDAHeader.EncompassingEncounter` but never read by any mapper | HIGH confidence gap (code-read confirmed) | **0/4** corpus has this element | Document-level construct, distinct from section-level Encounter Activity; clean, scoped gap since no corpus risk of regression |
| `documentationOf/serviceEvent/performer` | CONF:1198-9946 (NPI guidance, verified) | **no FHIR path produced** | none | **NOT implemented** — parsed into `CDAHeader.DocumentOf.Performers`, never read | HIGH confidence gap | Not isolated per-file in this pass | Same root cause/fix as the Practitioner section's identical gap |

---

## 9. Procedures

**Go mapper**: `services/cda_fhir/mappers/procedure_mapper.go` · **Seed**: V149 `procedures` (partial) · **Tests: zero** at the FHIR-mapping layer

| CDA Field | Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| `entry.Code` (incl. translations) | Procedure Activity Procedure V2 `.4.14`, CONF:1098-19207/19206 (verified) | `Procedure.code` (full, incl. translations) | `CDACodeToCodeableConcept` | `procedure_mapper.go:40-42` | HIGH | PF: 1 entry, `nullFlavor=NI`; real code (G0439) only in `full_ccd_nist.xml` parser fixture, not FHIR-mapping tests | Seed's flat fields don't capture translations at all |
| `StatusCode` | Verified against official `ConceptMap/CF-ProcedureStatus` (`github.com/HL7/ccda-on-fhir`) | `Procedure.status` | `ProcedureStatusToFHIR` — **fixed 2026-06-21**: `aborted` now correctly maps to `stopped` (was `not-done`, the bug); `cancelled` still maps to `not-done` — they are distinct codes, not synonyms | `procedure_mapper.go:37` | HIGH — verified against official ConceptMap, test added | PF: completed | The seed's `cda_procedure_status_to_fhir` was already correct; Go had the bug |
| `EffectiveTime` → period/datetime (either/or) | CONF# UNVERIFIED | `performedPeriod` or `performedDateTime` | `CDATimeRangeToPeriod`/`CDATimeRangeToOnset` | `procedure_mapper.go:45-49` | HIGH | PF: `nullFlavor=NI` | Clean either/or, avoids FHIR choice-element violation |
| Performer | Author-Participation-adjacent, CONF# unisolated for Procedure specifically | `performer[0].actor.display` | inline, gated on a `CDANameToFHIR()` call whose result is then discarded | `procedure_mapper.go:51-67` | MEDIUM | No populated corpus instance | **Code-quality bug found**: computes `hn` purely to check non-nil, then discards it and rebuilds via `buildDisplayFromName` — dead computation. Also Reference-vs-display weakness (same as Encounters) |
| Body site (read from COMP entryRelationship) | `targetSiteCode` is actually a **direct child element** of `<procedure>`, not COMP-nested, per every example reviewed (CONF# itself UNVERIFIED) | `Procedure.bodySite[0]` | `CDACodeToCodeableConcept` | `procedure_mapper.go:73-80` | **LOW — structurally wrong** | PF's only entry has `targetSiteCode` as a **direct child**, confirming the Go code's COMP-based read would never fire | `CDAEntry` struct has **no `TargetSiteCode` field at all** — needs a parser change before a correct mapping is even possible, not just a Go-mapper fix |
| Procedure Activity Observation variant (`.4.13`) | CONF:1098-19202 (verified, code only) | same `Procedure` shape, no EntryType branching | none — `buildProcedureResource` treats all 3 sibling templates identically | implicit | MEDIUM | No corpus hit | Architecturally fine as a first pass; declarative engine should enumerate the 3 sibling templates (`.4.12` Act, `.4.13` Obs, `.4.14` Procedure) as distinct rows since CONF numbers/code-system guidance differ slightly |
| Procedure Activity Act variant (`.4.12`) | CONF:1098-19190/19189 (verified); encounter cross-ref CONF:1098-16849 (verified) | same | same | implicit | MEDIUM | Kareo: 1 entry matches `.4.12` | `entryRelationship:encounter/encounter/id` cross-reference (to link `Procedure.encounter`) is **not implemented at all** |

**Note**: No dedicated unit tests exist for this mapper at all — a clear Phase 0 testing-debt priority alongside the body-site structural bug.

---

## 10. Practitioner / Custodian

**Go mappers** (three independent paths, only two wired): `patient_mapper.go` (`MapAuthor`, `MapCustodian` — both wired), `practitioner_mapper.go` (`MapPractitioners` — **confirmed dead code**, but its private `buildPractitionerResource()` helper is used via `careteam_mapper.go`). **Not in seed.**

| CDA Field | Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| `Authors[].AssignedAuthor.Ids[]` | Author Participation `.4.119`, CONF:1098-31473 (verified); header CONF:1198-5449/16790 (verified) | `Practitioner.identifier[]` | `IIToIdentifier` | `patient_mapper.go:149-161` | HIGH (impl) | PF: 3 authors, **all with `<id root=".4.6">` and no `@extension`** | **Confirmed real-world edge case**: malformed-NPI fallback produces root-as-value garbage Identifiers — see cross-cutting #8 |
| `AssignedPerson.Names[]` | CONF:1098-31474/31475 (verified) | `Practitioner.name[]` | `CDANameToFHIR` | `patient_mapper.go:138-146` | HIGH (impl) | PF: real names present | `MapAuthor` only ever returns the **first** author with a usable person — PF's 3-author document yields exactly 1 Practitioner; the other 2 are silently dropped |
| `Telecoms[]` | CONF# UNVERIFIED for author telecom specifically | `Practitioner.telecom[]` | `CDATelecomToFHIR` | `patient_mapper.go:164-174` | HIGH (impl) | PF: populated | |
| `RepresentedOrganization` | CONF:1098-31476/31478-31481 (verified) | **not mapped** — no Organization/PractitionerRole link produced | none | **NOT implemented** (full function body read, confirmed absent) | HIGH confidence gap | **All 3 PF authors** carry a populated representedOrganization (name "Get Well Clinic", address) | Clean, well-evidenced gap. No `PractitionerRole` resource exists anywhere in this codebase |
| `AssignedAuthoringDevice` (device-authored entries) | Per CONF:1198-16790, device authorship is a normal IG-sanctioned alternative to person authorship | **not mapped** — explicitly skipped (`if AssignedPerson == nil { continue }`) | none | **NOT implemented (by design)** | HIGH confidence gap | Not confirmed in this 4-file corpus, but common in the wild generally | Device-authored documents produce zero Practitioner/Device/Provenance output |
| `Custodian...RepresentedCustodianOrganization.Names[0]` | Base CDA R2 `AssignedCustodian`; no C-CDA-specific CONF# located (UNVERIFIED) | `Organization.name` | direct assignment | `patient_mapper.go:188-196` | HIGH — tested (`TestMapCustodian_SetsActiveTrue`, name itself not asserted) | **4/4** corpus files have exactly 1 `<custodian>` | `Organization.active=true` hardcoded — defensible but unverifiable assumption baked into the mapper |
| `Custodian...Ids[]`/`Addresses[]` | UNVERIFIED | `Organization.identifier[]`/`.address[]` | `IIToIdentifier`/`CDAAddressToFHIR` | `patient_mapper.go:196-219` | HIGH (impl, untested beyond `active`) | Not independently verified per-file | |
| `legalAuthenticator` (entire element) | Base CDA `LegalAuthenticator` class confirmed via WebFetch against `build.fhir.org/ig/HL7/CDA-core-2.0`, 2026-06-22: `time`/`signatureCode` required (1..1) whenever present, `assignedEntity` 1..1, must be a person never an organization (HL7Wiki "CDA R3 legalAuthenticator"). C-CDA-specific US-Realm-Header CONF# still not located — the structural shape comes from base CDA, which doesn't vary by profile, so this didn't block implementation. | `Practitioner` via `LegalAuthenticatorMappingRules()` (declarative only — no Go mapper); `Composition.attester` wiring **deferred to Phase 4** (see sprint plan) | `cda_name_to_fhir`/`cda_ii_to_identifier`/`cda_telecom_to_fhir`/`cda_address_to_fhir` | **RESOLVED 2026-06-22** — `CDALegalAuthenticator` struct (`cda/document/types.go`) + `parseLegalAuthenticator()` (`header_parser.go`) added; declarative rule ported via the new `HeaderPath`/`BuildHeaderResource` engine primitive | HIGH — 3 new production tests + corpus | **3 of 4** corpus files have `<legalAuthenticator>`; Mtuitive has a **real NPI** — confirmed reproduced correctly post-fix | Was the largest single gap in this inventory; now closed at the parser+declarative-rule level. `Composition.attester`/`Provenance` FHIR-target wiring intentionally deferred — see sprint plan's Practitioner/Custodian writeup for why |
| `DocumentOf.Performers[].AssignedEntity` | CONF:1198-9946 (verified) | not mapped | none | **NOT implemented** — parsed, never read | HIGH confidence gap | Shared root cause with Encounters' identical gap | One fix benefits both inventories |
| CareTeam organizer participant → Practitioner | Care Team Organizer `.4.500` (participant sub-structure CONF# UNVERIFIED) | `name[]`/`identifier[]`/`qualification[].code`/`telecom[]` | `buildPractitionerResource` (private helper) | `practitioner_mapper.go:45-102`, invoked from `careteam_mapper.go:63` | HIGH — **best-tested Practitioner path in the codebase** (`mappers_test.go:581-629`) | Not isolated in this pass | The exported `MapPractitioners()` + its `dedupeKey()` NPI dedup logic is **confirmed dead code** — zero callers outside its own file |

---

## 11. CareTeam

**Go mapper**: `services/cda_fhir/mappers/careteam_mapper.go` · **Not in seed.** Tested: 2 dedicated tests.

| CDA Field | Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| `organizer/templateId` (`.4.500`) | CONF:4515-117/118 (extension `2022-06-01`) | n/a (selector) | — | `careteam_mapper.go:21-32` | HIGH | 0/4 | Targets base templateId without checking extension — forward-compatible |
| `organizer/statusCode` | CONF:4515-113/119 | `CareTeam.status` | `CareTeamStatusToFHIR` | `careteam_mapper.go:107` | HIGH | 0/4 | Tested |
| `organizer/code` | UNVERIFIED for this specific element | `CareTeam.category` | `CDACodeToCodeableConcept` | `careteam_mapper.go:113-115` | MEDIUM | 0/4 | Test fixture uses LOINC 86744-0 but doesn't assert on `category` output |
| `organizer/effectiveTime` | CONF:4515-127/157/158 | `CareTeam.period` | `CDATimeRangeToPeriod` | `careteam_mapper.go:116-118` | HIGH | 0/4 | |
| `participant[PPRF]` | CONF:4515-128/129 | `CareTeam.participant[]` | structural | `careteam_mapper.go:41-95` | HIGH — tested | 0/4 | |
| `participant.functionCode` | CONF:4515-130 | `CareTeam.participant.role` | `CDACodeToCodeableConcept` | `careteam_mapper.go:87-91` | HIGH — tested | 0/4 | |
| `participantRole/id` (NPI) | CONF:4515-131/132/133 | `Practitioner.identifier` + `member` ref | NPI-match join | `careteam_mapper.go:50,63,71,85` | HIGH — tested | 0/4 | |
| Nested component performer name enrichment | UNVERIFIED — empirically-derived workaround, not a single CONF# | `Practitioner.name` | NPI→name lookup map | `careteam_mapper.go:42-55,68-79` | HIGH (impl + rationale documented) — **enrichment branch itself untested** | 0/4 | Existing test's fixture has no nested-name case, so this specific enrichment path isn't actually exercised |

**Note**: Most fully-implemented and best-tested of the newly-inventoried sections, despite zero corpus prevalence — built against IG examples, not observed vendor output.

---

## 12. CarePlan / Goal (Plan of Care)

**Go mappers**: `plan_of_care_mapper.go` (production dispatcher: routes by `EntryType`+`moodCode` to `ServiceRequest`/`Appointment`/`SupplyRequest`/`MedicationRequest`/`Goal`), `goal_mapper.go` (Goal builder, reused by the dispatcher), `careplan_mapper.go` (**orphaned** — builds a `CarePlan` resource the dispatcher's own header comment says shouldn't exist as a section output; not called by the dispatcher, no test references it). **Not in seed.** Best-tested new section: 7 dedicated tests.

| CDA Field | Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| Plan of Treatment section (`.2.10`) | OID/extension confirmed; section-templateId CONF# UNVERIFIED | n/a | — | whole file | HIGH (structure) | **1/4** (Kareo, structure-only, 0 entries; PF's "Assessment and Plan" `.2.9` is the same family, also empty) | Strongest empirical evidence in this inventory that this section is reliably present-but-empty in real exports |
| `moodCode=EVN` → skipped | "limited to prospective/unfulfilled/incomplete orders" (quoted from IG render, no isolated CONF#) | n/a | — | `plan_of_care_mapper.go:65-67` | HIGH — tested | 0/4 | |
| `moodCode=GOL` → Goal | Goal Obs `.4.121` | delegates to `buildGoalResource` | reuse | `plan_of_care_mapper.go:68-70` | HIGH — tested | 0/4 | |
| `EntryType=substanceAdministration` → MedicationRequest | Planned Medication Activity | reuses Medications builder | `buildMedicationRequestResource` | `plan_of_care_mapper.go:72-73` | HIGH — tested | 0/4 | |
| `EntryType=encounter` → Appointment | Planned Encounter | `Appointment.*` | `AppointmentStatusToFHIR` | `plan_of_care_mapper.go:127-153` | HIGH — tested | 0/4 | |
| `EntryType=supply` → SupplyRequest | Planned Supply (quantity-default behavior UNVERIFIED) | `SupplyRequest.*`, quantity hardcoded to 1 when absent | `SupplyRequestStatusToFHIR` | `plan_of_care_mapper.go:160-177` | MEDIUM (documented workaround, untested) | 0/4 | Workaround for us-core-supplyrequest's 1..1 quantity requirement |
| `EntryType∈{procedure,observation,act}` → ServiceRequest | Planned Procedure/Obs/Act, all 3 confirmed in section render | `ServiceRequest.*` | `ServiceRequestStatusToFHIR`/`ServiceRequestIntentFromMood` | `plan_of_care_mapper.go:87-124` | HIGH — tested (both INT and PRP moods) | 0/4 | nullFlavor code fallback to `{"text":"Unknown"}` documented, not test-asserted |
| RSON → `reasonCode` | UNVERIFIED | `ServiceRequest.reasonCode` | `CDACodeToCodeableConcept` | `plan_of_care_mapper.go:112-121` | MEDIUM | 0/4 | Not test-covered |
| Goal `value`/`code` → `description` (3-tier fallback incl. placeholder) | Goal Obs render | `Goal.description` | `CDACodeToCodeableConcept` + fallbacks | `goal_mapper.go:39-50` | MEDIUM | 0/4 | `{"text":"Goal"}` ultimate fallback is a defensive non-IG placeholder, avoids invalid resource |
| Goal `statusCode` → `lifecycleStatus` | IG says CDA statusCode is fixed `active` | `Goal.lifecycleStatus` | `GoalStatusToFHIR` | `goal_mapper.go:36` | HIGH | 0/4 | Go defensively handles variance beyond the IG's fixed-value claim |
| Goal `effectiveTime` → target date | Goal Obs render | `Goal.target[0].dueDate` | `CDATimeRangeToOnset` | `goal_mapper.go:53-57` | HIGH | 0/4 | |
| Goal `author` (patient/provider/negotiated 3-way semantic) | IG explicitly defines this 3-way distinction (quoted from render) | `Goal.expressedBy` | — | **NOT implemented** | LOW | 0/4 | Candidate for Phase 6 only if real need surfaces |
| Orphaned `CarePlan` resource (`careplan_mapper.go`) | Dispatcher's own comments say CarePlan isn't a section-level FHIR output | `CarePlan.*` | `CarePlanStatusToFHIR` | whole file, **possibly dead code** | LOW | 0/4 | **Needs team confirmation** whether this is wired anywhere before porting into the declarative engine |

---

## 13. Coverage

**Go mapper**: `services/cda_fhir/mappers/coverage_mapper.go` · **Not in seed. Zero tests.**

| CDA Field | Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| Coverage Activity templateId (`.4.60`) | OID/extension confirmed; exact CONF# not isolated | n/a | — | `coverage_mapper.go:17-26` | MEDIUM | 0/4 | |
| Coverage Activity `code` (LOINC) | **RESOLVED 2026-06-22** via build.fhir.org/ig/HL7/CDA-ccda-2.2 (R2.1-era page) and cdasearch.hl7.org's Coverage Activity (V3) example: CONF:1198-19160 fixes `code="48768-6"` "Payment sources", codeSystem LOINC per CONF:1198-32156, templateId extension `2015-08-01` (R2.1). `52556-8` is real but belongs to a later companion-guide revision (build.fhir.org/ig/HL7/CDA-ccda's continuous-build STU5 page, templateId extension `2024-05-01`) — not applicable to this project's R2.1 target | `Coverage.type` | `CDACodeToCodeableConcept` + display-correction map | `coverage_mapper.go:31,49-61` | **HIGH — verified against IG, no bug** | 0/4 | Go mapper's `48768-6` constant is correct as-is; no change needed |
| Coverage Activity `statusCode` | **RESOLVED 2026-06-22**: CONF:1198-19094 fixes CDA `statusCode="completed"` unconditionally — the CDA template gives zero additional signal to derive a real-world active/inactive coverage status from (CDA act-status ≠ FHIR Coverage.status, different axes entirely) | `Coverage.status` | hardcoded `"active"` | `coverage_mapper.go:41` | **MEDIUM — deliberate, now confirmed there's no better signal available** | 0/4 | Not a bug: there is no CDA-side data this could be derived from instead. Worth a code comment citing CONF:1198-19094 so a future reader doesn't re-flag it |
| Coverage Activity `effectiveTime` | Confirmed (SHOULD) | `Coverage.period` | `CDATimeRangeToPeriod` | `coverage_mapper.go:64-66` | HIGH (citation)/MEDIUM (untested) | 0/4 | |
| Outer participant (HLD/PRF) → payer fallback | UNVERIFIED for this specific fallback path | `Coverage.payor[0].display` | `ScopingEntity.Desc` | `coverage_mapper.go:71-80` | MEDIUM | 0/4 | |
| Policy Activity (`.4.61`, COMP) → payer org (primary path) | Confirmed: COMP relationship, 1..* | `Coverage.payor[0].display` | nested loop over Performers | `coverage_mapper.go:81-98` | HIGH (citation)/MEDIUM (untested) | 0/4 | Matches confirmed two-level nesting structure |
| Payor unavailable → `"Unknown"` placeholder | n/a, defensive | `Coverage.payor[0].display` | hardcoded | `coverage_mapper.go:100-102` | LOW | 0/4 | Satisfies us-core-coverage's required 1..1 payor |
| `relationship` (subscriber↔patient) | CONF:4537-17139 confirmed (SELF is the implied default case) | `Coverage.relationship` | hardcoded "self" | `coverage_mapper.go:106-114` | HIGH (citation) | 0/4 | Never reads an actual relationship code even when one might be present — candidate gap |
| Policy Activity `id` (subscriber/member ID) | CONF:4537-10120/8984 confirmed | `Coverage.subscriberId` | `Id[].Extension` fallback `.Root` | `coverage_mapper.go:117-131` | HIGH (citation)/MEDIUM (untested) | 0/4 | |
| `beneficiary`/`subscriber` (always = patient) | UNVERIFIED, structural assumption | both | `ref(patientRef)` | `coverage_mapper.go:43-46` | MEDIUM | n/a | Never reads a distinct guarantor/subscriber participant |

**Note**: Zero test coverage caps every row below HIGH regardless of citation quality. Payers data is one of the more reliably-omitted C-CDA sections in ambulatory exports (eligibility usually flows via X12 270/271 instead) — 0/4 corpus prevalence is consistent with that, supporting LOW/MEDIUM confidence and Phase 6 deferral beyond the core payor/subscriberId/period fields. Both previously-blocking discrepancies (LOINC code, statusCode semantics) are now resolved against the actual IG (see rows above) — no remaining blocker for porting this section's core fields.

---

## 14. FamilyMemberHistory

**Go mapper**: `services/cda_fhir/mappers/family_history_mapper.go` · **Not in seed. Zero tests.**

| CDA Field | Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| Family History Organizer templateId (`.4.45`) | **RESOLVED 2026-06-22**: confirmed `2015-08-01` via build.fhir.org/ig/HL7/CDA-ccda-2.2 and multiple corroborating search results — the earlier flagged conflict was a draft/ballot-IG rendering artifact, not a real discrepancy | n/a | — | `family_history_mapper.go:17-26` | HIGH (verified) | 0/4 | No change needed |
| `subject[SBJ]/relatedSubject/code` (relationship) | Confirmed: typeCode fixed SBJ, code bound to Family Member Value valueset | `FamilyMemberHistory.relationship` | `CDACodeToCodeableConcept` | `family_history_mapper.go:41-46` | HIGH (citation)/MEDIUM (untested) | 0/4 | |
| `relatedSubject/subject/name` (lossy: surname only) | UNVERIFIED | `FamilyMemberHistory.name` (string) | `CDANameToFHIR`, takes `family` component only | `family_history_mapper.go:47-51` | MEDIUM | 0/4 | **Notable lossy transform**: given name discarded entirely; worth reconsidering (concat given+family) in Phase 3 |
| `administrativeGenderCode` | Confirmed: required strength, Federal Administrative Sex valueset | `FamilyMemberHistory.sex` | — | **NOT implemented** | LOW (clear gap) | 0/4 | |
| `subject/birthTime` (age derivation) | Confirmed: CONF:1198-15983, age SHOULD be inferred from birthTime vs. effectiveTime | `bornDate`/`age` | — | **NOT implemented** | LOW (clear gap, well-cited) | 0/4 | A directly-cited CONF# describing exactly this derivation, entirely unimplemented |
| `organizer.statusCode` (hardcoded) | UNVERIFIED whether IG requires it to drive output | `FamilyMemberHistory.status` | hardcoded `"completed"` | `family_history_mapper.go:38` | LOW | 0/4 | |
| `component[].observation` (Family History Obs `.4.46`) | Confirmed: code 1..1, value 1..1 (Problem valueset), statusCode fixed completed | `condition[].code` | `CDAValueToFHIR` (correctly uses value, not code) | `family_history_mapper.go:57-74` | HIGH (citation)/MEDIUM (untested) | 0/4 | Correctly distinguishes problem-type discriminator (code) from actual diagnosis (value) |
| `component[].observation.effectiveTime` (onset) | Confirmed | `condition[].onsetDateTime` | `CDATimeRangeToOnset` | `family_history_mapper.go:68-70` | HIGH (citation)/MEDIUM (untested) | 0/4 | |
| Death Observation (CAUS) | Confirmed in section render | `deceasedBoolean`/`deceasedAge` | — | **NOT implemented** | LOW (clear gap) | 0/4 | |
| Age Observation (SUBJ, inversionInd) | Confirmed in section render | `condition[].onsetAge` | — | **NOT implemented** | LOW (clear gap) | 0/4 | |

**Note**: Most gaps of any section relative to well-documented IG guidance — the gap here is implementation effort, not IG ambiguity. Zero test coverage and zero corpus prevalence (not even a structural section header in any of the 4 samples) make this the weakest-evidenced section for real-world behavior.

---

## 15. Device (Medical Equipment)

**Go mapper**: `services/cda_fhir/mappers/device_mapper.go` — **maps to `DeviceUseStatement`, not a standalone `Device` resource.** Not in seed. Zero tests.

| CDA Field | Citation | FHIR Path | Transform | Go Impl | Confidence | Corpus | Notes |
|---|---|---|---|---|---|---|---|
| Medical Equipment section templateId (`.2.23`) | Confirmed | n/a | — | n/a (entry-level mapper doesn't re-check) | MEDIUM | 0/4 | |
| Organizer: Supply Activity OR Procedure Activity Procedure | CONF:1098-32380 confirmed (either/or branch) | structural (`EntryType`) | upstream parser | implicit | HIGH (citation) | 0/4 | Confirms the IG's branching structure matches the Go mapper's "supply entry" assumption |
| Non-Medicinal Supply Activity templateId (`.4.50`) | Confirmed: required pattern, 1..1 | n/a | — | `device_mapper.go:17-26` | HIGH | 0/4 | |
| `statusCode` (default-to-active fallback) | Confirmed: 1..1, cannot be nullFlavor | `DeviceUseStatement.status` | local `deviceStatus()` switch, default→active | `device_mapper.go:37,63-72` | MEDIUM | 0/4 | Silent coercion of unrecognized status to "active" may mask real data-quality issues |
| Device/product code via participant | **CONFIRMED REAL BUG 2026-06-22** via build.fhir.org/ig/HL7/CDA-ccda-2.2's Non-Medicinal Supply Activity definitions page: CONF:1098-8754 fixes the product-identifying participant's `typeCode="PRD"` (Product), templateId extension `2014-06-09`. Go's `p.TypeCode == "DEV" \|\| p.TypeCode == "CSM"` check (neither value the IG actually fixes) will never match a single conformant document — the participant-sourced device code path is dead code in practice, always silently falling back to `entry.Code` instead | `device.codeableConcept` | `CDACodeToCodeableConcept` | `device_mapper.go:40-51` | **HIGH — verified bug, fix is well-defined** | 0/4 | Fix: change the typeCode check to `"PRD"`. Zero corpus evidence either way, but the fix is no longer a guess — it's IG-cited |
| `effectiveTime` | Confirmed (SHOULD, with high boundary SHOULD when present) | `timingPeriod`/`timingDateTime` | `CDATimeRangeToPeriod`/`CDATimeRangeToOnset` | `device_mapper.go:54-58` | HIGH (citation)/MEDIUM (untested) | 0/4 | |
| Product Instance (UDI, identifiers) | Confirmed in section render | would require a standalone `Device` resource | — | **NOT implemented** | LOW | 0/4 | No `Device` resource (with UDI/identifiers) is built anywhere — only the coded-description `DeviceUseStatement` path exists |
| UDI Organizer (COMP) | Confirmed, typeCode fixed COMP | `Device.udiCarrier` | — | **NOT implemented** | LOW | 0/4 | Same root gap as above |
| Instruction Observation (SUBJ, inversionInd) | Confirmed in section render | `DeviceUseStatement.note` | — | **NOT implemented** | LOW | 0/4 | |

**Note**: This section name is broader than what's actually implemented — if the engine is ever expected to produce real `Device` resources (e.g. for recall/device-tracking use cases), this is a scope gap, not just a missing-field gap. The PRD-vs-DEV/CSM typeCode discrepancy is now resolved (confirmed bug, fix identified) — no remaining IG-verification blocker for this section's core fields.

---

## Summary confidence distribution

| Section | Seeded (V149)? | Tests exist? | Corpus prevalence | Overall confidence band |
|---|---|---|---|---|
| Allergies | Yes (partial) | Yes | 3-4/4 | HIGH core, gaps in negation/severity/type |
| Medications | Yes (partial, Statement-only) | Yes | 4/4 | HIGH core, gaps in Request-path fields, SIG-text |
| Problems/Conditions | Yes (partial) | Yes | 2/4 (negation/severity untested in field) | HIGH core, 1 latent bug (category) |
| Vital Signs | Yes | Yes (BP-pair only) | 2-4/4 | HIGH core, performer gap |
| Results | Yes | No section-specific | 0-1/4 (mostly null) | MEDIUM — registry/mapper disagree on status valueMap |
| Social History | Yes | No | 2/4 | MEDIUM — generic path, no dedicated logic |
| Immunizations | Yes | No | 2/4 | **LOW on status correctness** — confirmed negationInd bug |
| Encounters | Yes (partial) | Minimal | 0-2/4 | MEDIUM — class/type conflation, header gaps |
| Procedures | Yes (partial) | **Zero** | 0-1/4 | MEDIUM — bodySite structurally broken |
| Practitioner/Custodian | No (declarative only) | Minimal | 3-4/4 (custodian); 3/4 (legalAuthenticator) | RESOLVED 2026-06-22 — legalAuthenticator parsed + mapped, header-level declarative primitive added |
| CareTeam | No | Yes (2) | 0/4 | HIGH (best new-section coverage) |
| CarePlan/Goal | No | Yes (7) | 0-1/4 (structure only) | HIGH core dispatch, LOW edges |
| Coverage | No | **Zero** | 0/4 | LOW-MEDIUM |
| FamilyMemberHistory | No | **Zero** | 0/4 | LOW — most gaps of any section |
| Device | No | **Zero** | 0/4 | LOW — scope mismatch (DeviceUseStatement vs Device) |

**Coverage, FamilyMemberHistory, and Device have zero test coverage and zero corpus prevalence** — the strongest combined signal for Phase 6 deferral. **CareTeam and CarePlan/Goal** are comparatively mature (tested, IG-aligned) despite also having zero corpus prevalence — built against IG examples rather than observed vendor data, but worth carrying into Phase 3 on test-coverage strength.

---

## Out of scope for Phase 3 (deferred to Phase 6)

Per the parent sprint doc's Phase 0 task 4, these C-CDA sections have **no current Go mapper at all** and were not researched in this pass — they wait for a dedicated Phase 6 session each, gated by the same IG-citation + corpus-prevalence process used above:

- **Advance Directives** (`.2.21`)
- **Nutrition** (no dedicated US Core profile target identified yet)
- **Medical Equipment beyond what `device_mapper.go` covers** (see Device section above — the existing mapper covers Non-Medicinal Supply Activity only; Procedure Activity Procedure as the alternate Medical Equipment entry path is unhandled)
- **Payers detail beyond core Coverage** (eligibility, prior authorization, X12-adjacent constructs)
- **Plan of Care sub-templates beyond what `plan_of_care_mapper.go` covers** (Goal.author 3-way semantic, Instruction-on-planned-act, nested organizer-of-organizers)

Within the 15 sections researched, these specific LOW-confidence items are explicitly flagged as **not ready for Phase 3** and should wait for Phase 6 verification:
- FamilyMemberHistory: `administrativeGenderCode`, birthTime/age derivation, Death Observation, Age Observation (4 unimplemented, well-cited fields)
- Device: Product Instance/UDI, UDI Organizer, Instruction Observation (would require scoping a real `Device` resource, not just `DeviceUseStatement`)
- Coverage: status-code semantics, LOINC code discrepancy (need direct IG-PDF resolution, not just search-snippet citations)
- Encounter/Procedure/Practitioner: Reference-vs-display weaknesses (participant.individual, performer.actor, location.location) — these are FHIR-conformance risks more than mapping-fidelity gaps, worth a dedicated design decision in Phase 2 rather than a per-row fix

## Confirmed bugs/findings to action independently of the engine rebuild

These don't need to wait for Phase 1-4 — they're correctness issues in the **currently-running production code**:

1. ✅ **FIXED 2026-06-21 — Immunization negationInd bug** (Section 7) — refused vaccines were reported as administered. `ImmunizationStatusToFHIR` now takes `negationInd`; `statusReason` populated from the RSON refusal-reason relationship when negated. 4 regression tests added.
2. ✅ **FIXED 2026-06-21 — Procedure bodySite structurally read the wrong source** (Section 9) — added `CDAEntry.TargetSiteCode` parser field + parsing; mapper now reads it directly instead of scanning for a COMP entryRelationship. 5 regression tests added (parser + mapper level).
3. ✅ **FIXED 2026-06-21 — Condition category hardcoding** (Section 3) — `MapConditions` now takes a `category` param; `"problems"`/`"healthConcerns"` dispatch entries pass `"problem-list-item"`/`"health-concern"` respectively, each with the correct CodeSystem per the published US Core "Problem or Health Concern" value set (verified live, not assumed — `health-concern` lives in a different CodeSystem entirely, not just a different code).
4. ✅ **FIXED 2026-06-21 — Procedures/Medications status valueMap disagreement**, resolved by fetching HL7's official `ConceptMap/CF-ProcedureStatus` and `ConceptMap/CF-MedicationStatus` from `github.com/HL7/ccda-on-fhir` (not guessed): the **seed was right and Go had the bug** for Procedures (`aborted→stopped`, not `not-done`); for Medications, fixed `MedicationRequestStatusToFHIR`'s `aborted→stopped` (was wrongly bucketed with `cancelled`) and added the missing `nullified→entered-in-error` case to both Request and Statement variants. Also discovered: the official IG has **no consensus mapping at all** for `moodCode="EVN"`/MedicationStatement — documented as a best-effort default in the code, not presented as spec-verified. The Request/Statement architectural split in the seed is still deferred to Phase 2 (seed isn't live).
5. ✅ **FIXED 2026-06-21 — Malformed-NPI Identifier fallback** (`IIToIdentifier` root-as-value) — added `isFixedIdentifierSystem()`; root-as-value fallback is now suppressed for the fixed national identifier systems (NPI/SSN/Medicare/MBI/DL, where root is always the system OID, never a patient value) but still works for generic/facility OIDs, where some real implementations legitimately encode the value only in root. 4 regression tests added, including one proving the legitimate fallback case still works.
