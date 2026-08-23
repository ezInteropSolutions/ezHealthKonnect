/**
 * CDAStepsDocs — Documentation for cda.* pipeline steps
 *
 * cda.to_fhir, cda.parse, cda.build, cda.map_to_canonical, cda.normalize, cda.section_to_csv, cda.dedupe.
 * cda.section_to_csv and cda.dedupe use oobTemplatesLive (fetched from the same live catalog
 * endpoints their own Configuration tab already calls) instead of a hardcoded oobTemplates array —
 * this is what keeps their "Built-in Templates" table from drifting out of sync with the real Go
 * catalogs the way the old hardcoded arrays repeatedly did.
 *
 * Self-registers into StepDocumentationRegistry at load time — this file must be
 * loaded (via <script>) after StepDocumentationRegistry.js and before any step's
 * Documentation tab is opened. Mirrors the StepBuilderRegistry.register() pattern
 * already used by every step's Configuration-tab builder
 * (public/js/pipeline/components/StepBuilderRegistry.js).
 */

(function () {
    const docs = {
            'cda.to_fhir': {
                description: 'Converts a parsed C-CDA/CCD document into a FHIR R4 Bundle using the declarative CDA→FHIR mapping engine (services/cda_fhir). Each CDA section (Allergies, Medications, Problems, Vitals, …) is matched against a MappingRule that names its FHIR resource type and field list, resolved through a three-tier lookup: your interface\'s own field overrides (made in the Sections tab) first, then the out-of-box (OOB) template for that section, and finally nothing if the section has no rule at all — in which case its narrative text (if any) is still captured as a DocumentReference so no clinical content is silently lost. Some sections produce more than one resource per entry: a recorder or asserter on an Allergy/Condition, or a lead participant on a Care Team, is emitted as its own linked Practitioner/PractitionerRole/Organization resource alongside the primary one, so the FHIR Bundle this step produces intentionally contains mixed resource types even for a single-section rule. If no separate cda.parse step runs earlier in the pipeline, this step auto-parses the raw CDA XML itself (looking for it in the message content, or a configured sourceField) — a dedicated cda.parse step is only needed when something else in the pipeline needs the parsed CDA structure before this step runs.',
                useCases: [
                    'Ingest a CCD/C-CDA document from an EHR and produce a standards-compliant FHIR R4 Bundle for downstream systems',
                    'Restrict processing to specific sections (enabledSections) when an interface only ever sends a subset of a full CCD, or when a section is known to be unreliable for a given source system',
                    'Customize which sections are enabled, and add/remove/remap individual fields per interface without touching the OOB template — including mapping into a FHIR Extension via the Sections tab\'s Add Field flow',
                    'Capture ALL matching entries for an ambiguous CDA path as a list (Add Field\'s "Capture ALL matching entries" option) instead of only the first match, when a section can legitimately repeat the same kind of entry',
                    'Route Plan-of-Care "future visit" entries (CDA classCode=ENC, moodCode=APT) to either FHIR Appointment (default) or Encounter, matching how the receiving system expects planned visits to be modeled',
                    'Enable terminology validation to surface codes that don\'t resolve against the loaded ValueSets, without blocking the rest of the Bundle from being produced',
                    'Hide specific fields (e.g. a noisy code-system detail) from a resource type\'s auto-generated narrative summary (resource.text.div) via the Narrative Fields tab, independent of the Sections tab\'s field mappings',
                ],
                example: {
                    sourceField: 'parsedCDA',
                    outputField: 'fhirBundle',
                    documentType: 'CCD',
                    bundleType: 'collection',
                },
                examples: [
                    {
                        label: 'Standard full-document ingestion',
                        description: 'Default configuration — every section the document has a rule for is processed, using US Core profiles and continuing past any single section\'s errors.',
                        config: {
                            sourceField: 'parsedCDA',
                            outputField: 'fhirBundle',
                            bundleType: 'collection',
                            profileMode: 'us-core',
                            onSectionFailure: 'continue',
                        },
                    },
                    {
                        label: 'Restrict to specific sections',
                        description: 'Only the listed sections are mapped; everything else (including its narrative-only DocumentReference capture) is skipped entirely. Section keys are the same ones shown as checkboxes in the Sections tab.',
                        config: {
                            sourceField: 'parsedCDA',
                            outputField: 'fhirBundle',
                            enabledSections: ['allergiesAndIntolerances', 'medications', 'problems', 'vitalSigns'],
                        },
                    },
                    {
                        label: 'Fail fast instead of continuing',
                        description: 'Stops the whole conversion on the first section error instead of skipping that section and continuing — use when a partial Bundle is worse than no Bundle for this interface (e.g. a downstream system that rejects incomplete documents).',
                        config: {
                            sourceField: 'parsedCDA',
                            outputField: 'fhirBundle',
                            onSectionFailure: 'fail-fast',
                        },
                    },
                    {
                        label: 'Plan-of-Care future visits as Encounter, not Appointment',
                        description: 'By default a planned/future visit in the Plan of Care section maps to Appointment. Set this when the receiving system expects every visit — past or future — to be an Encounter.',
                        config: {
                            sourceField: 'parsedCDA',
                            outputField: 'fhirBundle',
                            planOfCareEncounterTarget: 'Encounter',
                        },
                    },
                    {
                        label: 'Terminology validation on, mapping log persistence off',
                        description: 'Turns on ValueSet checks for coded fields (surfaced as field-level notices, not hard failures) while skipping the mapping log write to object storage — useful for a high-volume interface where the per-message log isn\'t needed.',
                        config: {
                            sourceField: 'parsedCDA',
                            outputField: 'fhirBundle',
                            terminologyValidation: true,
                            mappingLogEnabled: false,
                        },
                    },
                ],
                workflow: [
                    { action: 'General tab', description: 'Set the source field (where the parsed CDA document lives on the pipeline data — "parsedCDA" if a cda.parse step ran earlier, or leave default for auto-parse), the output field (default "fhirBundle"), bundle type, and profile mode (us-core vs base FHIR).' },
                    { action: 'Sections tab', description: 'Enable or disable individual sections with the checkboxes; a disabled section is skipped entirely, including its narrative DocumentReference fallback. Use "Add Field" to map a CDA path that the OOB template doesn\'t already cover, optionally nesting it under a repeating group or capturing all matches as a list.' },
                    { action: 'Browse Test Data (inside Add Field)', description: 'Run a Test Pipeline execution first, then use this to click through real parsed field values from your own sample document instead of guessing a CDA path by hand — it also flags when a path is ambiguous and suggests how to disambiguate it.' },
                    { action: 'Terminology tab', description: 'Review which coded fields have a ValueSet binding and whether Code Validation / Code Translation are active for this interface — these are independent of each other (a code can be translated to a different system without also being validated, and vice versa).' },
                    { action: 'Narrative Fields tab', description: 'Choose which fields render in each resource type\'s auto-generated narrative (resource.text.div) for CDA-sourced resources — every populated field shows by default, uncheck to hide. Stored per (interfaceId, documentType) via GET/PATCH /api/cda/narrative-fields, independent of the Sections tab\'s field mappings; the resource type list is a fixed catalog (the 17 types services/fhir_narrative has dedicated rendering for), not derived from the current document.' },
                    { action: 'Advanced / Assembly tab', description: 'Toggle individual post-mapping assembly rules (datatype coercions, OID→URI translation, structural wiring, status-code mapping) off individually if your interface needs one of them handled differently, e.g. in a downstream Script step instead.' },
                    { action: 'Test Pipeline', description: 'Run the pipeline against a real or sample CDA document and check the step\'s Warnings badge — field-level notices (e.g. a dropped multi-value, a section that failed) show up here without stopping the rest of the Bundle from being produced.' },
                ],
                oobTemplates: [
                    { key: 'allergiesAndIntolerances', name: 'Allergies', note: 'AllergyIntolerance (+ Practitioner/PractitionerRole/Organization for recorder/asserter)' },
                    { key: 'medications', name: 'Medications', note: 'MedicationRequest or MedicationStatement, split by moodCode (INT → Request, else → Statement)' },
                    { key: 'problems', name: 'Problems', note: 'Condition, category=problem-list-item' },
                    { key: 'healthConcerns', name: 'Health Concerns', note: 'Condition (category=health-concern) + an assessment-scale supporting Observation' },
                    { key: 'vitalSigns', name: 'Vital Signs', note: 'Observation, category=vital-signs' },
                    { key: 'results / labResults', name: 'Results', note: 'Observation, category=laboratory (both section-key aliases produce the same shape)' },
                    { key: 'socialHistory', name: 'Social History', note: 'Observation, category=social-history + SDOH assessment-scale row' },
                    { key: 'functionalStatus', name: 'Functional Status', note: 'Observation, category=functional-status + assessment-scale supporting observation (e.g. PHQ-2)' },
                    { key: 'mentalStatus', name: 'Mental Status', note: 'Observation, category=cognitive-status + assessment-scale supporting observation' },
                    { key: 'historyOfPresentIllness', name: 'Clinical Note (HPI)', note: 'DocumentReference, matched by the Clinical Note template id' },
                    { key: 'immunizations', name: 'Immunizations', note: 'Immunization' },
                    { key: 'encounters', name: 'Encounters', note: 'Encounter' },
                    { key: 'procedures', name: 'Procedures', note: 'Procedure' },
                    { key: 'goals', name: 'Goals', note: 'Goal' },
                    { key: 'carePlan / planOfCare / assessmentAndPlan / planOfTreatment', name: 'Plan of Care', note: 'Dispatches per entry to Goal, MedicationRequest, Appointment (or Encounter, see planOfCareEncounterTarget), SupplyRequest, ServiceRequest, or Encounter depending on the activity type' },
                    { key: 'careTeam / careTeams', name: 'Care Team', note: 'CareTeam + a linked Practitioner resource for every participant:lead' },
                    { key: 'payersInsurance / payors', name: 'Coverage / Payers', note: 'Coverage, aligned to the C-CDA on FHIR IG — real insurance data is read from the nested Policy Activity, not the outer container' },
                    { key: 'familyHistory', name: 'Family History', note: 'FamilyMemberHistory' },
                    { key: 'medicalEquipment', name: 'Medical Equipment', note: 'DeviceUseStatement' },
                    { key: '(document header)', name: 'Patient / Author / Custodian / Legal Authenticator / Encompassing Encounter', note: 'Header-level constructs (not a section) → Patient, Practitioner, Organization, Practitioner, Encounter + Location respectively — always processed regardless of enabledSections' },
                    { key: '(any section, no rule)', name: 'Narrative-only fallback', note: 'A section with narrative text but no matching rule (or no entries) still produces a DocumentReference so the content isn\'t lost' },
                ],
                parameters: [
                    { name: 'sourceField', type: 'string', required: false, description: 'Dot-path to the parsed CDA map on the pipeline data. Default: auto-detects a root-level object with _format="ccda", or "parsedCDA" if a cda.parse step ran earlier.' },
                    { name: 'outputField', type: 'string', required: false, description: 'Where the resulting FHIR Bundle is written on the pipeline data. Default: "fhirBundle".' },
                    { name: 'docType', type: 'string', required: false, description: 'CDA document type hint (e.g. "CCD", "Discharge Summary"). Default: inferred from the parsed document.' },
                    { name: 'bundleType', type: 'string', required: false, description: 'FHIR Bundle.type. Default: "collection". Other values: "document", "transaction".' },
                    { name: 'profileMode', type: 'string', required: false, description: '"us-core" (default) injects US Core meta.profile + extensions onto every resource; "base" produces plain FHIR R4 with no profile assertions.' },
                    { name: 'onSectionFailure', type: 'string', required: false, description: '"continue" (default): a section that errors is skipped and recorded in processingResult.failedSections, the rest of the Bundle still builds. "fail-fast": the whole step fails on the first section error.' },
                    { name: 'enabledSections', type: 'string[]', required: false, description: 'Whitelist of section keys to process (see the table above for valid keys). Default: all sections with a rule are processed. Managed via the Sections tab checkboxes.' },
                    { name: 'terminologyValidation', type: 'boolean', required: false, description: 'When true, coded fields are checked against their bound ValueSet and non-matching codes are recorded as field-level notices. Default: false.' },
                    { name: 'mappingLogEnabled', type: 'boolean', required: false, description: 'When true (default), a per-message MappingLog (section timing, resource counts, dedup/synthesis events) is persisted to object storage, retrievable via GET /api/messages/:messageId/mapping-log. Set false to skip this for high-volume interfaces that don\'t need it.' },
                    { name: 'planOfCareEncounterTarget', type: 'string', required: false, description: 'Overrides which resource a Plan-of-Care "future visit" entry maps to: "Appointment" (default) or "Encounter". Empty/unset falls back to the interface\'s own default, then to "Encounter".' },
                ],
                bestPractices: [
                    {
                        practice: 'Run a real Test Pipeline before relying on Browse Test Data',
                        reason: 'Add Field\'s Browse Test Data tab reads from the last Test Pipeline run\'s parsed output — without one, it can only show an empty state, not real field values to click through.',
                        example: 'Test Pipeline once with a representative sample document, then open Add Field for the section you\'re customizing.',
                    },
                    {
                        practice: 'Prefer enabledSections over deleting/renaming an OOB rule',
                        reason: 'Disabling a section via the Sections tab checkbox is reversible and interface-scoped; it never touches the OOB template other interfaces still rely on.',
                        example: 'A vendor that never sends a real Social History section: disable "socialHistory" here rather than trying to make the OOB rule tolerate it.',
                    },
                    {
                        practice: 'Use "Capture ALL matching entries" only when the section can legitimately repeat',
                        reason: 'It bypasses the Add Field disambiguation logic entirely (by design) — pointed at a genuinely singular field, it can pull in unrelated entries that merely share the same structural shape.',
                        example: 'A list of prior surgeries under one Procedure entry — correct to capture all. A single free-text reason-for-visit field — should stay single-value.',
                    },
                    {
                        practice: 'Treat mixed resource types in the output Bundle as expected, not a bug',
                        reason: 'Recorder/asserter and Care Team lead sub-resources are intentionally emitted alongside the section\'s primary resource in the same Bundle — filtering fhirBundle.entry by resourceType is the correct way to isolate just the primary resource type downstream.',
                        example: 'Filtering for AllergyIntolerance from the allergies section\'s output may also see a Practitioner or Organization entry for the same section — that\'s the recorder, not an error.',
                    },
                ],
                troubleshooting: [
                    {
                        issue: 'A field I know is in the source document didn\'t make it into the FHIR output',
                        cause: 'Most commonly: the CDA entry has more than one <value> element and the one you expected wasn\'t the first non-empty one, or the section is disabled in enabledSections, or the field simply has no OOB rule yet.',
                        fix: 'Check the step\'s Warnings for a "more than one <value>" notice naming the section/entry. If the section is disabled, re-enable it in the Sections tab. If there\'s truly no rule, use Add Field with Browse Test Data to map it.',
                    },
                    {
                        issue: 'Disabling one section also removed a DocumentReference for a different section',
                        cause: 'enabledSections only gates that one section\'s own processing — a narrative-only section with no entries and no explicit rule gets its own independent DocumentReference fallback, and that is unaffected by disabling a different section.',
                        fix: 'If you\'re seeing this, double-check which section key is actually in enabledSections vs. which one\'s DocumentReference disappeared — they should always match. Report it if they don\'t.',
                    },
                    {
                        issue: 'Bundle has more resources of a type than expected (e.g. 3 Practitioners instead of 1)',
                        cause: 'EmitAsResource rows (Allergy/Condition recorder or asserter, Care Team lead participant) each produce their own linked sub-resource — a document with 3 different asserters across its Allergies and Problems sections legitimately produces 3 Practitioner resources.',
                        fix: 'Not a bug — inspect each Practitioner\'s reference to confirm it\'s linked from a real, distinct recorder/asserter/lead in the source document.',
                    },
                    {
                        issue: 'Auto-parse isn\'t picking up my raw CDA XML',
                        cause: 'When there\'s no separate cda.parse step, this step looks for raw XML in a fixed set of field names (content, raw_content, rawMessage, rawXML, cdaXML) or a sourceField you configured — if your connector wrote the raw XML somewhere else, none of those will match.',
                        fix: 'Add an explicit cda.parse step before this one (its output field is always found), or set sourceField to wherever your connector actually puts the raw XML.',
                    },
                    {
                        issue: '"No FHIR bundle to copy" when trying to export the test result',
                        cause: 'Historical UI bug — the Copy/Export lookup only checked for legacy HL7 step-name keys and a differently-cased field name than this step actually writes.',
                        fix: 'Fixed — the lookup now finds this step\'s output regardless of what you\'ve named it. If you still see this, confirm the step actually ran successfully (check its Warnings/Errors) before assuming the copy path is at fault again.',
                    },
                ],
                securityNotes: [
                    {
                        note: 'The FHIR Bundle and MappingLog both carry PHI',
                        detail: 'The Bundle obviously contains patient clinical data; the MappingLog persisted to object storage (when mappingLogEnabled) records section-level timing and resource counts, not raw field values, but is still tied to a specific message/interface.',
                        recommendation: 'Apply the same access controls to the mapping-log object storage bucket/path as to raw message storage — don\'t treat it as non-sensitive telemetry.',
                    },
                    {
                        note: 'Egress PHI gate applies downstream, not at this step',
                        detail: 'This step only builds the Bundle; the pipeline\'s PHI egress check runs later, immediately before any connector.outbound step delivers it externally.',
                        recommendation: 'If this step\'s output is consumed by something other than a connector.outbound (e.g. a custom Script step that calls out externally), that path does not automatically get the same PHI egress protection — review it separately.',
                    },
                ],
                stepOutput: {
                    description: 'These variables are available to every step after this one in the pipeline, via the field picker or stepOutput.<alias>.<field> syntax.',
                    fields: [
                        { name: 'fhirBundle', type: 'object', description: 'The FHIR R4 Bundle produced from the CDA document.' },
                        { name: 'fhirBundle.entry', type: 'array', description: 'Array of FHIR resources in the Bundle (Patient, AllergyIntolerance, Condition, Practitioner, …) — remember this can include sub-resources for recorder/asserter/lead participants alongside each section\'s primary resource.' },
                        { name: '_stepOutput.processingResult', type: 'object', description: 'Section-level success/failure breakdown: resourcesProduced, sectionsProcessed, successfulSections, failedSections, sectionErrors, partialSuccess.' },
                        { name: '_stepOutput.processingResult.failedSections', type: 'array', description: 'Section keys that encountered an error during mapping (only populated when onSectionFailure="continue").' },
                        { name: '_stepOutput.mappingLog', type: 'object', description: 'Per-section timing, resource counts, dedup/synthesis events from the CDA→FHIR assembly layer.' },
                        { name: '_stepOutput.mappingLog.summary', type: 'object', description: 'Totals across the whole document: resource counts, timing, deduplicated/synthesized counts.' },
                        { name: '_stepOutput.cdaDocument', type: 'object', description: 'The typed *CDADocument this step parsed itself — only present during a Test Pipeline run, and only when no separate cda.parse step already ran earlier in the pipeline.' },
                    ],
                },
                // Rendered as a live, searchable reference (see createDocumentation's
                // transformReference block + _wireCDATransformReference) instead of a
                // hand-copied static list — the registry backing GET /api/cda/transforms
                // (services/cda_fhir/declarative_transform_registry.go) already carries a
                // real description per transform, and hand-copying it here would just be a
                // second place for the two to quietly drift apart.
                transformReference: true,
            },
            'cda.parse': {
                description: 'Parses raw CDA/CCD XML into a USCDI-keyed ParsedJSON document map plus a fully-typed *CDADocument struct, without doing any FHIR mapping. Place it after cda.normalize (if the source emits legacy C32/HITSP) and before cda.to_fhir. You only need this step when something else in the pipeline needs the parsed structure BEFORE cda.to_fhir runs (e.g. a Script step that inspects patient demographics to decide routing) — cda.to_fhir auto-parses raw XML itself when no earlier cda.parse step exists, so a simple ingest-then-map pipeline doesn\'t need this step at all.',
                useCases: [
                    'Inspect parsed CDA fields in a Script or conditional step BEFORE the CDA→FHIR mapping runs, to drive routing decisions',
                    'Persist parsed CDA content independently, even on an interface that never runs cda.to_fhir at all',
                    'Feed a custom (non-declarative-engine) mapping step that reads ParsedJSON directly',
                ],
                example: {
                    sourceField: 'raw',
                    outputField: 'parsedCDA',
                },
                parameters: [
                    { name: 'sourceField', type: 'string', required: false, description: 'Pipeline field holding the raw CDA XML string. Default: "raw".' },
                    { name: 'outputField', type: 'string', required: false, description: 'Where the parsed JSON is written. Default: "parsedCDA". Special value "__root__" merges ParsedJSON\'s keys directly into the root of the pipeline data instead of nesting them under one field.' },
                    { name: 'schemaDir', type: 'string', required: false, description: 'Filesystem path to the CDA schema directory. Default: "./cda/schemas" — only change this in unusual deployment layouts.' },
                ],
                bestPractices: [
                    {
                        practice: 'Only add this step when something needs the parsed structure before cda.to_fhir runs',
                        reason: 'cda.to_fhir already auto-parses raw XML itself when no separate cda.parse step precedes it — adding one unconditionally is redundant work on every message.',
                        example: 'A pipeline that just does connector.inbound → cda.to_fhir → connector.outbound never needs an explicit cda.parse step.',
                    },
                ],
                troubleshooting: [
                    {
                        issue: '"source field is empty or not a string" error',
                        cause: 'sourceField doesn\'t point to where the raw XML actually landed on the pipeline data for this connector.',
                        fix: 'Check what field your inbound connector actually writes the raw message body to, and set sourceField to match.',
                    },
                    {
                        issue: 'Document Type Hint dropdown doesn\'t seem to change anything',
                        cause: 'Known gap: this field is collected into step.config.documentTypeHint by the Form UI, but the backend executor\'s config does not read that key at all yet — document type is always auto-detected from the CDA\'s own templateId OIDs.',
                        fix: 'Not fixable from the UI today. If auto-detection is choosing the wrong document type for a specific source, that needs a backend fix, not a config change.',
                    },
                ],
                stepOutput: {
                    description: 'Available to every step after this one.',
                    fields: [
                        { name: '_stepOutput.parsedCDA', type: 'object', description: 'USCDI-keyed ParsedJSON document map (header + sections).' },
                        { name: '_stepOutput.parsedCDA.cdaProfile', type: 'string', description: 'Detected CDA profile: C-CDA 2.1 | C32 | HITSP.' },
                        { name: '_stepOutput.sectionCount', type: 'number', description: 'Number of CDA sections parsed.' },
                        { name: '_stepOutput.parsedCDA.header.patient', type: 'object', description: 'Patient demographics extracted from the CDA document header.' },
                        { name: '_cdaDocument', type: 'object', description: 'The fully-typed *CDADocument struct — what cda.to_fhir\'s declarative engine actually consumes, and what a following cda.to_fhir step will find and reuse instead of auto-parsing again.' },
                    ],
                },
                securityNotes: [
                    {
                        note: 'Parsed CDA content may be persisted to the document store',
                        detail: 'When a CDADocumentStore is configured, every successfully parsed document is saved asynchronously — this is the same PHI-bearing content as the raw XML, just structured.',
                        recommendation: 'Apply the same access controls to parsed-document storage as to raw message storage.',
                    },
                ],
            },
            'cda.build': {
                description: 'Format-agnostic successor to fhir.to_cda: builds a full C-CDA 2.1 document from canonical USCDI-keyed JSON (the same shape cda.parse produces) or a raw FHIR R4 Bundle, covering every SHALL/SHOULD/MAY section for the selected Document Type — not just the four sections fhir.to_cda handles. For CCD this is all 17 real sections per the C-CDA 2.1 IG (SHALL: Allergies, Medications, Problems, Results, Plan of Treatment, Social History, Vital Signs; SHOULD: Procedures, Payers; MAY, included whenever the source has data: Advance Directives, Encounters, Family History, Functional Status, Immunizations, Medical Equipment, Mental Status, Nutrition). Uses the same CDA schema (cda/schemas/ccda_2_1.json) that drives cda.parse, so a new section or document type is a schema change, not new Go code.',
                useCases: [
                    'Deliver a complete CCD (all 17 real sections — see description) to a legacy downstream system',
                    'Build a CDA document from a FHIR-native interface\'s Bundle without hand-maintaining a second, narrower serializer',
                    'Pair with cda.map_to_canonical to build a CCD directly from CSV rows or database query results — no FHIR Bundle required',
                ],
                example: {
                    sourceField: 'message.parsedCDA',
                    inputFormat: 'canonical',
                    outputField: 'message.cdaXML',
                    documentType: 'CCD',
                },
                parameters: [
                    { name: 'sourceField', type: 'string', required: false, description: 'Pipeline field holding the source data. Default: "parsedCDA" — set to "message.<field>" to match wherever the upstream step (e.g. cda.map_to_canonical) actually wrote it; see the message-prefix troubleshooting entry below.' },
                    { name: 'inputFormat', type: 'string', required: false, description: '"canonical" (default) — the USCDI-keyed shape cda.parse produces — or "fhir_bundle" to read a raw FHIR R4 Bundle instead.' },
                    { name: 'outputField', type: 'string', required: false, description: 'Where the generated CDA XML string is written. Default: "cdaXML" — set to "message.<field>" if a later step needs to read it.' },
                    { name: 'documentType', type: 'string', required: false, description: 'CCD | Discharge Summary | Referral Note | History and Physical | Consultation Note | Progress Note. Determines which sections are SHALL/SHOULD and the document-level LOINC code/title. Default: "CCD".' },
                    { name: 'orgName', type: 'string', required: false, description: 'Custodian organization name, and fallback author organization when the source has none. Default: "ezHealthKonnect".' },
                ],
                troubleshooting: [
                    {
                        issue: '"source data not found at field ..." error',
                        cause: 'sourceField doesn\'t point to where the canonical JSON or FHIR Bundle actually is on the pipeline data — commonly because it\'s missing the "message." prefix (pipeline step data lives under a top-level "message" key, not spread at the root) or doesn\'t match the upstream step\'s own outputField.',
                        fix: 'Confirm the field name against whatever step upstream produced it (commonly cda.map_to_canonical\'s or cda.parse\'s own outputField, e.g. "message.parsedCDA"), or a FHIR-producing step\'s output field with inputFormat set to "fhir_bundle". Verified against a live pipeline run: "parsedCDA" alone fails with this exact error even when cda.map_to_canonical ran successfully just before it; "message.parsedCDA" on both steps resolves correctly.',
                    },
                    {
                        issue: 'A section I expected is missing from the output',
                        cause: 'Either the section has zero entries in the source data (SHOULD/MAY sections are omitted entirely when empty; SHALL sections are still emitted with a "No information available" narrative and no entries), or — when inputFormat is "fhir_bundle" — that section has no FHIR-resource mapping yet (currently: Allergies, Medications, Problems, Results, Vital Signs, Immunizations, Social History, Encounters, Procedures; Plan of Treatment/Functional Status/Family History/Payers/Advance Directives/Medical Equipment/Mental Status/Nutrition require canonical JSON input, not a FHIR Bundle, for now).',
                        fix: 'Check the source data for that section\'s entries directly, or switch to canonical-JSON input if the section you need isn\'t in the FHIR adapter\'s coverage yet.',
                    },
                ],
                stepOutput: {
                    description: 'Available to every step after this one.',
                    fields: [
                        { name: 'cdaXML', type: 'string', description: 'The generated C-CDA 2.1 XML document.' },
                    ],
                },
            },
            'cda.map_to_canonical': {
                description: 'The no-code, format-agnostic on-ramp for CCD construction: maps CSV columns, DB query columns, or arbitrary JSON fields onto the same canonical USCDI-keyed JSON shape cda.parse and cda.build already share, using field mappings (source path → canonical field key, with optional value translation) instead of a script. Header mapping covers the patient and author groups (flat scalar fields, e.g. firstName/lastName/dateOfBirth, plus a nested address.* group for the patient\'s address) — informant/documentationOf/encompassingEncounter are intentionally out of scope, since cda.build already synthesizes a safe fallback for documentationOf when absent. Section mapping resolves an array of source rows at a configurable path (there is no universal "rows" field name across upstream steps — file_parser_executor.go writes "records", database_enrichment_executor.go writes "rows") and applies one field mapping per canonical field per row.',
                useCases: [
                    'Build a CCD from a CSV file (via a File Parser step) with zero new Go code — only step configuration',
                    'Build a CCD from a database query\'s result rows (via a Database Enrichment step)',
                    'Translate a source system\'s own status vocabulary (e.g. "A"/"R") into the canonical CDA vocabulary (e.g. "active"/"resolved") using each field mapping\'s Value Map',
                ],
                example: {
                    outputField: 'message.parsedCDA',
                    header: [
                        { group: 'patient', target: 'firstName', sourcePath: 'message.patientFirstName' },
                    ],
                    sections: [
                        {
                            sectionKey: 'problems',
                            rowsPath: 'message.records',
                            fields: [
                                { canonicalField: 'conditionCode', sourcePath: 'icd10Code' },
                            ],
                        },
                    ],
                },
                parameters: [
                    { name: 'outputField', type: 'string', required: false, description: 'Pipeline field where the canonical JSON is written. Default: "parsedCDA" — but a bare (non-"message."-prefixed) path is only visible to the SAME step\'s own step_output, not to a later step; point cda.build\'s sourceField at "message.parsedCDA" and set THIS field to the same value so the data actually carries forward (see the message-prefix troubleshooting entry below).' },
                    { name: 'header', type: 'array', required: false, description: 'Flat header field mappings: {group: "patient"|"author", target, sourcePath, transform?}. target is a dot-path relative to header.<group> (e.g. "address.street" for the patient\'s nested address). sourcePath should be "message.<field>" to read the inbound message (see the message-prefix troubleshooting entry below).' },
                    { name: 'sections', type: 'array', required: false, description: 'One entry per canonical section: {sectionKey, rowsPath, fields: [{canonicalField, sourcePath, fallbackPaths?, transform?, valueMap?, literalValue?}]}. rowsPath points to the array of source rows for that section (e.g. "message.records") relative to the pipeline\'s root data, NOT relative to the row; each field\'s own sourcePath then resolves relative to one row. transform runs BEFORE valueMap (see the transform troubleshooting entry below).' },
                ],
                troubleshooting: [
                    {
                        issue: 'Every field/section comes back empty even though the source data is definitely there',
                        cause: 'Pipeline step data lives under a top-level "message" key (inputData = {message: {...}, _metadata: {...}, ...}), not spread at the root — a bare sourcePath/rowsPath like "records" or "dob" only matches a literal top-level "records"/"dob" key, which normally doesn\'t exist.',
                        fix: 'Prefix sourcePath/rowsPath/outputField values with "message." (e.g. "message.records", "message.dob") to reach the actual inbound message data. Verified against a live pipeline run: bare paths produce header:{}/sections:{} with sectionCount:0; "message."-prefixed paths resolve correctly.',
                    },
                    {
                        issue: 'A section is missing from the output entirely',
                        cause: 'rowsPath didn\'t resolve to an array on the pipeline data, or every row produced zero mapped fields (a row where every configured sourcePath/fallbackPaths comes back empty, with no literalValue, is skipped rather than emitted as an empty entry).',
                        fix: 'Confirm rowsPath matches the exact field name your upstream step wrote to (commonly "message.records" for a File Parser step\'s output, "message.rows" for a Database Enrichment step\'s) and that at least one field mapping per row has real data.',
                    },
                    {
                        issue: 'A mapped value doesn\'t match what cda.build expects (e.g. a raw status code instead of "active"/"resolved")',
                        cause: 'The source system\'s own vocabulary differs from the canonical CDA vocabulary — this step does plain value copying by default, with no built-in code translation.',
                        fix: 'Add a Value Map on that field mapping (e.g. {"A": "active", "R": "resolved"}) to translate the raw source value before it\'s written.',
                    },
                    {
                        issue: 'A date field is rejected downstream, or Value Map isn\'t matching a date-shaped value',
                        cause: 'Source dates rarely arrive in CDA\'s own YYYYMMDD[HHMMSS] format — a raw "01/15/2024" or "2024-01-15T10:00:00Z" needs reformatting first. Transform runs BEFORE Value Map, so a Value Map keyed on the reformatted value (not the raw one) works correctly once Transform is set.',
                        fix: 'Set that field mapping\'s Transform to "date_to_cda" (accepts YYYY-MM-DD, MM/DD/YYYY, or M/D/YYYY) or "datetime_to_cda" (accepts RFC 3339/ISO 8601) — see GET /api/cda/canonical-transforms for the full, live list with descriptions. This is a small, CDA-target-only registry (services/executors/transform/canonical_value_transforms.go) — deliberately NOT the same Transform vocabulary cda.to_fhir\'s Section field editor uses, since that one emits FHIR-resource-shaped values incompatible with this step\'s flat-string target fields.',
                    },
                ],
                stepOutput: {
                    description: 'Available to every step after this one.',
                    fields: [
                        { name: 'parsedCDA', type: 'object', description: 'USCDI-keyed canonical JSON (header + sections), ready for cda.build.' },
                    ],
                },
            },
            'cda.normalize': {
                description: 'Pre-parse step that rewrites legacy C32/HITSP template OIDs to their C-CDA 2.1 equivalents on a raw XML string, before cda.parse runs. Falls back to passing the XML through unchanged (not an error) if the schema directory couldn\'t be loaded at startup. Only needed when a source system is known to emit C32/HITSP rather than C-CDA 2.1 — running it against an already-C-CDA-2.1 document is a low-cost no-op, not harmful.',
                useCases: [
                    'A legacy EHR or HIE feed still emits C32/HITSP-era CDA — normalize before cda.parse so its C-CDA-2.1-oriented section/entry matching actually recognizes the document',
                    'Audit how many template OID substitutions a given source system\'s messages typically need, via the step output',
                ],
                example: {
                    sourceField: 'raw',
                },
                parameters: [
                    { name: 'sourceField', type: 'string', required: false, description: 'Pipeline field holding the raw C32/HITSP XML string to normalize. Default: "raw".' },
                    { name: 'outputField', type: 'string', required: false, description: 'Where the normalized XML is written. Default: overwrites sourceField in place.' },
                ],
                bestPractices: [
                    {
                        practice: 'Only add this step for source systems known to emit legacy C32/HITSP',
                        reason: 'It\'s a safe no-op on an already-C-CDA-2.1 document, but there\'s no reason to run it on every message from a source you know is already current.',
                        example: 'A modern EHR export → skip cda.normalize entirely. A decade-old HIE feed still on C32 → keep it as the first CDA step in the pipeline.',
                    },
                ],
                troubleshooting: [
                    {
                        issue: '"Strict mode" and "Log template OID substitutions" checkboxes don\'t seem to change behavior',
                        cause: 'Known gap: these Form UI toggles are collected into step.config.strictMode / step.config.logSubstitutions, but the backend executor\'s config doesn\'t read either key yet — it always passes unrecognized OIDs through unchanged rather than failing, and substitution counts are only available via the step output, not the application log.',
                        fix: 'Check _stepOutput.substitutions and _stepOutput.sourceProfile after a Test Pipeline run to see what actually happened, rather than relying on these two checkboxes.',
                    },
                    {
                        issue: '"source field is empty or not a string" error',
                        cause: 'sourceField doesn\'t point to where the raw XML actually is on the pipeline data.',
                        fix: 'Check your connector\'s output field name and match sourceField to it.',
                    },
                ],
                stepOutput: {
                    description: 'Available to every step after this one.',
                    fields: [
                        { name: '_stepOutput.normalizedXML', type: 'string', description: 'The normalized C-CDA 2.1 XML string.' },
                        { name: '_stepOutput.substitutions', type: 'number', description: 'Number of template OIDs rewritten.' },
                        { name: '_stepOutput.sourceProfile', type: 'string', description: 'CDA profile detected before normalization: C32 | HITSP | C-CDA 2.1.' },
                    ],
                },
            },
            'cda.section_to_csv': {
                description: 'Converts CDA/CCD clinical sections into flat, one-row-per-entry CSV text — a clinical data model view (Medication/Code/StartDate/Sig/Status, etc.), NOT a FHIR resource shape. Produces one CSV string per section, each written to its own pipeline field (csv_<sectionKey> by default). Covers 35 CDA sections (23 structured + 12 narrative-only) out of the box, with a dedicated step config UI: enable/disable which sections export, browse every OOB column live (name, CDA path, fallback paths), add a custom column of your own, or override an OOB column outright by reusing its exact name — all without a code change. Every non-narrative row also gets a universal SourceFile column plus EntryId/AuthorName/AuthorOrganization/Comment prepended automatically. It is meant for direct flat-file/database delivery of clinical elements, not FHIR interoperability — use cda.to_fhir for that. If no earlier cda.parse step produced a typed CDA document, this step auto-parses raw CDA XML itself the same way cda.to_fhir does.',
                useCases: [
                    'Deliver CCD clinical data as flat CSV to a data warehouse, analytics pipeline, or legacy system that expects tabular clinical elements rather than FHIR',
                    'Export Medications/Allergies/Problems/Vitals/Immunizations/Procedures/Encounters/Social History/Results as one CSV per section for direct file-based delivery (connector.outbound → file_writer) or bulk load into a database table',
                    'Provide a quick, dependency-free clinical data extract from a CCD when a full FHIR mapping pipeline is unnecessary for the receiving system',
                ],
                example: {
                    sourceField: 'parsedCDA',
                    sections: ['medications', 'allergiesAndIntolerances', 'problems'],
                    outputPrefix: 'csv_',
                },
                parameters: [
                    { name: 'sourceField', type: 'string', required: false, description: 'Pipeline field holding raw CDA XML to auto-parse when no upstream cda.parse step already produced _cdaDocument. Ignored when _cdaDocument is already present. Default: auto-detects from common raw-content field names (content, raw_content, rawMessage, rawXML, cdaXML).' },
                    { name: 'sections', type: 'string[]', required: false, description: 'Which sections to export. Default: all 35 supported sections (23 structured — allergiesAndIntolerances, admissionDiagnosis, admissionMedications, advanceDirectives, careTeam, dischargeDiagnosis, dischargeMedications, encounters, familyHistory, functionalStatus, goals, healthConcerns, immunizations, medicalEquipment, medications, mentalStatus, nutrition, payersInsurance, problems, procedures, results, socialHistory, vitalSigns; plus 12 narrative-only sections — see oobTemplates below for the full list and per-section columns). A section key with no OOB template is silently skipped. The step config UI\'s per-section checkboxes write this array for you.' },
                    { name: 'outputPrefix', type: 'string', required: false, description: 'Prefix for each section\'s output field name. Default: "csv_" — e.g. csv_medications, csv_allergiesAndIntolerances.' },
                    { name: 'customColumns', type: 'object', required: false, description: 'Per-section extra/override columns: { "<sectionKey>": [{ "name": "...", "path": "<CDA path>", "fallbackPaths": [...], "exposeCodeMetadata": false, "multiple": false }, ...] }. A custom column whose name matches an existing OOB column REPLACES it (path/fallbackPaths/etc all win); any other name is appended as a new column after the OOB set. Ignored for narrative-only sections (they have no per-entry columns to extend). The step config UI\'s "+ Add Custom Column" / "Override" / "Reset to OOB" controls read and write this for you — editing it by hand is only needed for scripted/bulk pipeline setup.' },
                ],
                oobTemplatesLive: {
                    endpoint: '/api/cda/csv/sections',
                    adapt: (data) => (data.sections || []).map(s => ({
                        key: s.sectionKey,
                        name: s.sectionKey,
                        note: s.narrativeOnly
                            ? 'Narrative-only — SourceFile + NarrativeText'
                            : (s.columns || []).map(c => c.name).join(', '),
                    })),
                },
                bestPractices: [
                    {
                        practice: 'Use cda.to_fhir instead when the downstream system needs FHIR interoperability',
                        reason: 'This step\'s columns are a fixed, entry-level-direct clinical data model — it deliberately does not support per-interface field customization, ValueSet terminology validation, or nested recorder/asserter/performer sub-resource linkage the way cda.to_fhir does.',
                        example: 'Delivering to a partner\'s FHIR API → cda.to_fhir. Bulk-loading a reporting database table → cda.section_to_csv.',
                    },
                    {
                        practice: 'Restrict sections to only what the downstream consumer actually needs',
                        reason: 'Every listed section produces its own CSV field regardless of whether the document has any entries for it (an empty section still produces a header-only CSV) — narrowing sections avoids delivering empty files downstream.',
                        example: 'A pipeline that only delivers Medications and Allergies data: sections: ["medications", "allergiesAndIntolerances"].',
                    },
                    {
                        practice: 'Prefer overriding an OOB column over hand-editing customColumns JSON',
                        reason: 'The step config UI\'s per-section column browser shows every OOB column\'s exact CDA path live from the running executor (GET /api/cda/csv/sections) — using its "Override" / "+ Add Custom Column" controls means you\'re always building on the current shape, not a stale copy you hand-typed once.',
                        example: 'A source system that populates NDC instead of RxNorm for MedicationCode: override that one column\'s path rather than re-authoring the whole Medications column set.',
                    },
                ],
                troubleshooting: [
                    {
                        issue: 'A section\'s CSV output is just a header row with no data rows',
                        cause: 'Either the source CCD genuinely has no entries in that section (common — not every CCD populates every section), or the section key in sections doesn\'t match one of the 35 supported OOB keys.',
                        fix: 'Check the section actually has <entry> elements in the source document. Confirm the section key spelling against the OOB template list above (case-sensitive, camelCase).',
                    },
                    {
                        issue: 'A column is empty for every row in a section that clearly has that data in the source XML',
                        cause: 'Either the OOB path genuinely doesn\'t match how this source system structures that field (real C-CDA documents vary more than the base IG implies), or the value needs nested Scope-then-SourcePath resolution (used by cda.to_fhir for recorder/asserter sub-resources, reaction manifestation observations, and Social History\'s nested SDOH question/answer tiers) — out of scope for this step\'s entry-level-direct path model.',
                        fix: 'First try overriding the column with your own CDA path via the step config UI\'s per-section "Override" control (Configuration tab) — no code change needed. If the value genuinely needs nested/multi-hop resolution this step can\'t express, use cda.to_fhir instead (its Sections tab Add Field flow supports the full path grammar).',
                    },
                    {
                        issue: 'I need a column this step doesn\'t capture out of the box (a proprietary/vendor-specific element)',
                        cause: 'The 35 OOB section templates cover clinically common fields per section, not every possible vendor extension.',
                        fix: 'Use the step config UI\'s "+ Add Custom Column" control for that section — provide a name and CDA path (optionally fallback paths, and whether it\'s code-metadata or multi-value) and it\'s appended after the OOB columns on the next run. This writes to step.config.customColumns; see the customColumns parameter above.',
                    },
                    {
                        issue: '"no typed CDA document available" error',
                        cause: 'No upstream cda.parse step ran, and no raw CDA XML was found at sourceField or any of the auto-detected candidate field names.',
                        fix: 'Add an explicit cda.parse step before this one, or set sourceField to wherever your connector actually writes the raw CDA XML.',
                    },
                ],
                stepOutput: {
                    description: 'Available to every step after this one.',
                    fields: [
                        { name: '_stepOutput.csv_<sectionKey>', type: 'string', description: 'One CSV string per exported section (e.g. csv_medications, csv_allergiesAndIntolerances) — header row + one data row per clinical entry, columns in the fixed OOB order shown above (not alphabetical).' },
                        { name: '_stepOutput.section_rows', type: 'object', description: 'Map of sectionKey → number of CSV data rows produced for that section (excludes the header row).' },
                    ],
                },
            },
            'cda.dedupe': {
                description: 'Finds and removes duplicate clinical entries — like the same allergy or medication appearing twice in one document. Turn on crossMessage mode to also catch the same entry repeated across separate documents received over time for the same patient. Rather than comparing entries as raw blocks of text, it looks at the specific details that identify each type of clinical fact (e.g. an allergy\'s substance, or a medication\'s code and start date), so near-duplicates with minor formatting differences are still caught. Place this step between cda.parse and cda.to_fhir/cda.section_to_csv so everything downstream automatically works with the cleaned-up, duplicate-free data.',
                useCases: [
                    'A source EHR occasionally double-charts the same allergy or problem within one document — remove the accidental duplicate before it becomes two FHIR resources or two CSV rows',
                    'Clean up a CCD before cda.to_fhir mapping when a specific source system is known to produce redundant entries in a section',
                    'crossMessage: an HIE/EHR feed sends a new CCD per encounter, and each CCD is a full cumulative snapshot of the patient\'s chart (not just what changed) — a patient seen 5 times a year produces 5 documents each re-listing the SAME active medications/allergies/problems. Without crossMessage, all 5 restatements flow through and the downstream system accumulates 5 copies of every fact. crossMessage turns that cumulative feed into an effective incremental one: each fact is delivered once, the first time it\'s ever seen for that patient on this interface.',
                ],
                example: {
                    sections: ['medications', 'allergiesAndIntolerances', 'problems'],
                    strategy: 'first',
                },
                examples: [
                    {
                        label: 'Override an OOB identity rule',
                        description: 'Replaces the built-in vitalSigns rule (code + timestamp) with code-only matching — use when a source system\'s vitals genuinely repeat the same reading with a slightly different timestamp and you want those treated as duplicates too.',
                        config: {
                            sections: ['vitalSigns'],
                            overrides: { vitalSigns: { keyPaths: ['code.code'] } },
                        },
                    },
                    {
                        label: 'Dedupe a section with no OOB rule',
                        description: 'functionalStatus (and any other section not in the OOB list) has no built-in identity rule — providing keyPaths in overrides defines one from scratch, purely via config, no code change.',
                        config: {
                            sections: ['functionalStatus'],
                            overrides: { functionalStatus: { keyPaths: ['code.code', 'effectiveTime.value.value'] } },
                        },
                    },
                    {
                        label: 'Cross-message dedup for the same patient',
                        description: 'Catches the same allergy/problem/medication being restated in every document received for a patient over time, not just within one document. patientIdentifierRoot is the CDA <id> root (OID) that identifies this interface\'s patients — required whenever crossMessage is true; there is no automatic detection, since guessing which of a patient\'s several identifiers (MRN/SSN/Medicare/etc.) is reliable isn\'t something to leave to chance on clinical data.',
                        config: {
                            sections: ['allergiesAndIntolerances', 'problems', 'medications'],
                            crossMessage: true,
                            patientIdentifierRoot: '2.16.840.1.113883.19',
                        },
                    },
                ],
                parameters: [
                    { name: 'sourceField', type: 'string', required: false, description: 'Pipeline field holding raw CDA XML to auto-parse when no upstream cda.parse step already produced _cdaDocument. Ignored when _cdaDocument is already present.' },
                    { name: 'sections', type: 'string[]', required: false, description: 'Which sections to dedupe. Default: every section in the live catalog the step config UI shows (currently 23) — 9 of those (allergiesAndIntolerances, encounters, immunizations, medications, problems, procedures, results, socialHistory, vitalSigns) come with a built-in OOB identity rule pre-checked; the rest have no default identity fields and are effectively skipped until you check at least one field for them (via overrides, or the checkbox picker).' },
                    { name: 'strategy', type: 'string', required: false, description: '"first" (default): keep the earliest occurrence of each duplicate group. "last": keep the most recent occurrence. Unlike remove_duplicates, there is no "merge" option — see Best Practices for why.' },
                    { name: 'overrides', type: 'object', required: false, description: 'Per-section identity-key override: { "<sectionKey>": { "keyPaths": ["<CDA path>", ...] } }. Replaces the OOB identity rule for that section, or defines one from scratch for a section with no OOB rule. Paths use the same CDA path grammar as everywhere else in this step (dot-segments, [key=value] predicates), resolved relative to one section entry. The Configuration tab\'s checkbox picker builds this for you from a live catalog of known fields per section (GET /api/cda/dedupe/sections) — check/uncheck fields to narrow or widen a match, or use its "Add field" row for a CDA path the catalog doesn\'t know about (a proprietary/vendor-specific element). Editing this JSON directly is still supported and is the only way to reach a path the picker\'s "Add field" escape hatch wouldn\'t cover either.' },
                    { name: 'crossMessage', type: 'boolean', required: false, description: 'Default false. When true, also checks each entry against a persistent registry of everything ever seen for this patient on this interface (cda_dedupe_registry table) — not just entries within the current document. Requires patientIdentifierRoot.' },
                    { name: 'patientIdentifierRoot', type: 'string', required: false, description: 'Required when crossMessage is true. The CDA <id> root OID (e.g. "2.16.840.1.113883.19") that identifies which of the patient\'s several identifiers to use as the cross-message matching key, scoped per interface. If a given document\'s patient doesn\'t carry an identifier with this root, crossMessage is silently skipped for that one message (within-document dedup still applies) — check _stepOutput.cross_message_active to see whether it actually ran.' },
                    { name: 'trackSuppressionLineage', type: 'boolean', required: false, description: 'Default true (omit the key to leave it on). Only relevant when crossMessage is on. When true, each entry crossMessage suppresses gets recorded with its identity_key and which earlier message first delivered it (see _stepOutput.section_stats.<key>.cross_message_suppressed below) — this is what powers the "Dedup Suppressions" step on a message\'s Journey tab (Messages page). Set to false only to stop capturing that per-entry detail (e.g. to minimize what gets written to step_executions) — the suppression decision itself is completely unaffected either way, you just lose the "what/where" detail, keeping only the plain removed-count.' },
                ],
                oobTemplatesLive: {
                    endpoint: '/api/cda/dedupe/sections',
                    adapt: (data) => (data.sections || []).map(s => ({
                        key: s.sectionKey,
                        name: s.displayName || s.sectionKey,
                        note: (s.fields || []).map(f => f.name).join(', '),
                    })),
                },
                bestPractices: [
                    {
                        practice: 'There is no "merge" strategy, unlike remove_duplicates — this is deliberate',
                        reason: 'A generic shallow-field merge is safe for arbitrary JSON records, but for clinical entries a false-positive identity match (two entries that coincidentally share a code+date but are actually different facts) combined with a silent field merge risks quietly corrupting patient data. "first"/"last" only ever keeps one entry\'s data intact, never blends two.',
                        example: 'If you need richer conflict resolution than first/last, handle it in a Script step after this one instead of expecting this step to merge fields for you.',
                    },
                    {
                        practice: 'Without crossMessage, this only removes duplicates within ONE document',
                        reason: 'The same problem/medication restated across multiple CCDs received over time for the same patient needs crossMessage: true plus a configured patientIdentifierRoot — without it, this step has no memory of previously-processed messages.',
                        example: 'Five CCDs for the same patient over a year, each listing "Hypertension" — with crossMessage off, this step does not catch that across documents; it only catches accidental double-entries inside a single document.',
                    },
                    {
                        practice: 'Scope crossMessage to sections where restating the same fact is actually expected',
                        reason: 'Allergies/problems/medications are commonly restated verbatim in every visit summary — good crossMessage candidates. Vitals/results are usually genuinely different readings each time (same code, different value/date), so cross-message matching there is less useful and more likely to key on a date that legitimately differs.',
                        example: 'Enable crossMessage for allergiesAndIntolerances/problems/medications; leave vitalSigns/results to within-document dedup only.',
                    },
                    {
                        practice: 'Once an identity is registered, it is suppressed forever — there is no expiry',
                        reason: 'cda_dedupe_registry has no TTL or cleanup job. This is usually exactly what you want (a chronic condition or standing allergy should never need to be "re-delivered"), but it means a genuinely resolved-then-recurring fact with the SAME identity key (e.g. a condition whose identity rule doesn\'t include onset date) will never be delivered again after the first time, even if it legitimately recurs.',
                        example: 'For a section whose OOB identity is code-only (allergiesAndIntolerances), that\'s the correct behavior — an allergy doesn\'t "recur", it\'s continuous. For a section keyed on code + date (problems, medications), a genuinely new occurrence gets a new date and is therefore still caught as new — only an EXACT repeat (same code AND same date) is suppressed.',
                    },
                    {
                        practice: 'crossMessage history is scoped per interface, not globally per patient',
                        reason: 'The registry key is (interface_id, patient_key, section_key, identity_key) — the same patient fed through a second, separate interface (e.g. a different source system or a test/staging copy of this interface) starts with no dedup history at all.',
                        example: 'Cloning this interface for a staging test will not inherit production\'s dedup history — the first document the clone processes will treat everything as new, which is expected, not a bug.',
                    },
                    {
                        practice: 'This step forwards deltas (new facts only) — it is NOT a patient-level summary/CDR builder, and deliberately does not try to be one',
                        reason: 'cda_dedupe_registry stores only an identity FINGERPRINT per fact (a string like "code.code=I10|effectiveTime.low.value=20240115") — never the actual clinical content (no drug name, dose, route, etc). It exists purely to answer "have I already delivered this," not to reconstruct "what is this patient\'s complete current record." Building a true patient-level summary (encounter-based CCDs in, one deduplicated whole-patient CCD out) needs an actual content store per patient that stays current — closer to a small clinical data repository / MPI-adjacent system than a pipeline step, with its own patient-matching and conflict-resolution concerns this engine does not take on.',
                        example: 'If a downstream system needs "give me everything currently true about this patient" rather than "don\'t re-send me what I already have," that\'s a different system to pair this engine with (a CDR, HIE record locator, etc.) — not a config change to crossMessage.',
                    },
                ],
                troubleshooting: [
                    {
                        issue: 'Two entries I expected to be treated as duplicates were both kept',
                        cause: 'Most likely one of the identity-key paths resolved to empty for at least one entry — by design, this step never dedupes when any key component is unresolved, to avoid a false-positive match on missing data.',
                        fix: 'Not a bug — check the source document actually has the expected code/date on both entries. If the data is genuinely present but the path doesn\'t resolve, that\'s a gap in the OOB identity rule for that section, not something configurable today.',
                    },
                    {
                        issue: '"no typed CDA document available" error',
                        cause: 'No upstream cda.parse step ran, and no raw CDA XML was found at sourceField or any auto-detected candidate field name.',
                        fix: 'Add an explicit cda.parse step before this one, or set sourceField to wherever your connector actually writes the raw CDA XML.',
                    },
                    {
                        issue: 'crossMessage is on, but the same fact still shows up again in a later document',
                        cause: 'Most likely patientIdentifierRoot doesn\'t match the OID actually used on the <id> element inside the source documents\' patient header — or a different value is used across documents (e.g. one facility\'s CCDs use an MRN root, another\'s use an SSN or Medicare root for the same patient). When no identifier with the configured root is found, crossMessage is silently skipped for that message (in-document dedup still runs) rather than failing loudly.',
                        fix: 'Check _stepOutput.patient_key_resolved and _stepOutput.cross_message_active for that message — false means crossMessage did not actually run. Open a real source document and confirm the exact root OID on the patient\'s <id> element(s); if the sending system uses more than one identifier root inconsistently, pick whichever one is guaranteed to be present and stable across every document this interface will ever receive.',
                    },
                    {
                        issue: 'A field I need for identity matching isn\'t in the Sections + Identity Fields checkbox list',
                        cause: 'The checkbox picker only lists fields already known to the CDA CSV export catalog (the same one cda.section_to_csv uses) — a genuinely proprietary or vendor-specific element won\'t be there.',
                        fix: 'Use the "Add field" row at the bottom of that section\'s expanded detail to add its CDA path directly (see the parameters entry for overrides above) — no code change required, and it\'s checked by default once added.',
                    },
                    {
                        issue: 'A fact is missing from the FHIR bundle / CSV output and I can\'t tell why it was removed or where it went',
                        cause: 'total_removed/cross_message_removed alone are just aggregate counts, not a list of what was actually dropped — that would be a real data-lineage gap on its own.',
                        fix: 'Easiest path: open that message in Messages → message detail → Journey tab. If crossMessage suppressed anything for it, a "Dedup Suppressions" step appears in the timeline listing exactly which fact(s) were dropped and a clickable link to the earlier message that first delivered each one. That view reads straight from this message\'s own persisted _stepOutput.section_stats.<sectionKey>.cross_message_suppressed — same data, just already assembled for you (requires trackSuppressionLineage, on by default). For a broader view across MANY messages for one patient (not tied to a single message), use Admin → CDA Dedupe Registry to search by interface + patient key directly instead.',
                    },
                ],
                stepOutput: {
                    description: 'Available to every step after this one.',
                    fields: [
                        { name: '_stepOutput.section_stats', type: 'object', description: 'Per section: original_count, dedup_count, removed_count (in_document_removed + cross_message_removed combined), in_document_removed, cross_message_removed.' },
                        { name: '_stepOutput.section_stats.<sectionKey>.cross_message_suppressed', type: 'array', description: 'Lineage for this section\'s cross-message drops — only present when at least one entry was suppressed (and trackSuppressionLineage isn\'t set to false). One {identity_key, first_seen_message_id, first_seen_at} per dropped entry. This is exactly what backs the "Dedup Suppressions" step on a message\'s Journey tab (Messages page) — you don\'t need to read this field directly unless you\'re working with the raw API/DB (GET /api/messages/:messageId/dedupe-suppressions, or step_executions.step_output).' },
                        { name: '_stepOutput.total_removed', type: 'number', description: 'Total duplicate entries removed across every processed section (in-document + cross-message).' },
                        { name: '_stepOutput.cross_message_active', type: 'boolean', description: 'True if crossMessage mode actually ran for this message. False if crossMessage was on but patientIdentifierRoot didn\'t match any of this document\'s patient identifiers, or interface_id couldn\'t be resolved — check this before assuming cross-message dedup happened.' },
                    ],
                },
            },
    };
    Object.keys(docs).forEach((stepType) => StepDocumentationRegistry.register(stepType, docs[stepType]));
})();
