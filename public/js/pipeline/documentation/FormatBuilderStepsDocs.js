/**
 * FormatBuilderStepsDocs — Documentation for the no-code format-builder steps
 *
 * fhir.to_cda, fhir.build, hl7.build — the no-code "map columns/fields directly onto a target
 * format" builder steps (as opposed to cda.build/cda.map_to_canonical, which target canonical CDA JSON).
 *
 * Self-registers into StepDocumentationRegistry at load time — this file must be
 * loaded (via <script>) after StepDocumentationRegistry.js and before any step's
 * Documentation tab is opened. Mirrors the StepBuilderRegistry.register() pattern
 * already used by every step's Configuration-tab builder
 * (public/js/pipeline/components/StepBuilderRegistry.js).
 */

(function () {
    const docs = {
            'fhir.to_cda': {
                description: 'Converts a FHIR R4 Bundle back into a C-CDA 2.1 XML document — the reverse of cda.to_fhir. Intended as the last transform step in a pipeline that delivers CDA to a legacy downstream system via a file or MLLP connector.outbound. This step remains MVP-stage: it only serializes Patient demographics plus four sections (Allergies, Medications, Problems, Immunizations), always produces C-CDA 2.1 output regardless of the document template selected, and the narrative/pretty-print toggles are not yet wired to the serializer. For the full CCD SHALL+SHOULD section set (Results, Vital Signs, Social History, Encounters, Procedures, and more) and non-FHIR input (canonical JSON from cda.parse), use cda.build instead — this step is kept as-is for existing pipelines built against it.',
                useCases: [
                    'Deliver a FHIR-native interface\'s data to a legacy downstream system that only accepts CDA XML, when only the four MVP sections are needed',
                    'Bridge a FHIR-based internal pipeline to a partner integration still on C-CDA',
                ],
                example: {
                    sourceField: 'fhirBundle',
                    outputField: 'cdaXML',
                },
                parameters: [
                    { name: 'sourceField', type: 'string', required: false, description: 'Pipeline field holding the FHIR R4 Bundle to serialize. Default: "fhirBundle".' },
                    { name: 'outputField', type: 'string', required: false, description: 'Where the generated CDA XML string is written. Default: "cdaXML".' },
                    { name: 'profile', type: 'string', required: false, description: 'CDA profile to target. Default: "C-CDA 2.1". Currently ignored by the serializer in MVP — every Bundle produces C-CDA 2.1 output regardless of this value.' },
                ],
                troubleshooting: [
                    {
                        issue: 'The Form tab\'s "Document Template", "Include narrative", and "Pretty-print" controls don\'t seem to affect the output',
                        cause: 'Known gap: the Form UI collects these into step.config.template/includeNarrative/prettyPrint, but the backend executor\'s config doesn\'t read any of those three keys — output is always narrative-included, non-pretty-printed C-CDA 2.1 regardless of these values. (The Input Field control itself was fixed and now correctly writes step.config.sourceField.)',
                        fix: 'These three controls have no effect yet. If you need document-type-driven output or pretty-printing, use cda.build instead.',
                    },
                    {
                        issue: '"FHIR Bundle not found at field ..." error',
                        cause: 'sourceField doesn\'t point to where the Bundle actually is on the pipeline data.',
                        fix: 'Confirm the field name against whatever step upstream actually produced the Bundle (commonly cda.to_fhir\'s own outputField).',
                    },
                ],
                stepOutput: {
                    description: 'Available to every step after this one.',
                    fields: [
                        { name: 'cdaXML', type: 'string', description: 'The generated C-CDA 2.1 XML document.' },
                    ],
                },
            },
            'fhir.build': {
                description: 'The no-code, format-agnostic on-ramp for a single FHIR R4 resource: maps CSV columns, DB query columns, or arbitrary JSON fields directly onto a FHIR resource\'s own element paths, using field mappings (source path → target element, with an optional named transform and value translation) instead of a script. Unlike cda.map_to_canonical, there is no separate "build" step afterward — a FHIR resource already IS the JSON output this step produces. Add one fhir.build step per resource type needed, then feed each step\'s outputField into a Payload Builder step (FHIR Bundle mode) to assemble them into one Bundle. Field-level shaping reuses the same DeclarativeTransformRegistry cda.to_fhir dispatches to — bare-scalar transforms (string_direct, the *_status_to_fhir family, cda_decimal_string_to_number) work with any CSV/DB/JSON column; compound-shaped transforms (cda_code_to_codeable_concept, cda_quantity_to_fhir, cda_name_to_fhir, cda_address_to_fhir, ...) need the source value to already be a nested object shaped like the corresponding CDA structure (e.g. {code, codeSystem, displayName}) — a shape mismatch degrades safely to "nothing written," never a wrong value.',
                useCases: [
                    'Build a Patient (or any other) FHIR resource from a CSV file (via a File Parser step) with zero new Go code — only step configuration',
                    'Build a FHIR resource from a database query\'s result rows (via a Database Enrichment step)',
                    'Assemble a multi-resource Bundle from a non-CDA, non-HL7 source system by chaining several fhir.build steps into one Payload Builder (FHIR Bundle mode) step',
                    'Populate a field conditionally (e.g. "if source code system is LOINC do X, else do Y") by computing/normalizing the value in an upstream if_then_else or switch_case step\'s set_value/set_field action, then reading the normalized result here with an ordinary sourcePath — no per-field conditional logic exists (or is needed) inside fhir.build itself; see the troubleshooting entry below for a worked example.',
                    'Control which fields appear in this resource\'s auto-generated narrative (resource.text.div) via the Narrative Fields section — single-resource-type mode, pre-scoped to this step\'s own resourceType, sharing the same per-(interface, message type) storage the interface wizard and this step\'s core.mapping/cda.to_fhir siblings use.',
                ],
                example: {
                    resourceType: 'Patient',
                    profile: 'base',
                    version: 'R4',
                    outputField: 'message.fhirResource',
                    fields: [
                        { targetPath: 'birthDate', sourcePath: 'message.dob' },
                        { targetPath: 'gender', sourcePath: 'message.sexCode', transform: 'string_direct', valueMap: { M: 'male', F: 'female' } },
                    ],
                    repeatingGroups: [
                        {
                            targetPath: 'identifier',
                            rowsPath: 'message.patientIdentifiers',
                            fields: [
                                { targetPath: 'system', sourcePath: 'idSystem' },
                                { targetPath: 'value', sourcePath: 'idValue' },
                            ],
                        },
                    ],
                },
                parameters: [
                    { name: 'resourceType', type: 'string', required: true, description: 'FHIR resource type to build, e.g. "Patient". Must exist in the compiled schema registry for version/profile.' },
                    { name: 'profile', type: 'string', required: false, description: 'Compiled profile name. Default: "base". Also supports named profiles (e.g. "us-core") when available in the schema registry.' },
                    { name: 'version', type: 'string', required: false, description: 'FHIR version. Default: "R4".' },
                    { name: 'outputField', type: 'string', required: false, description: 'Pipeline field where the built resource is written. Default: "fhirResource" — prefix with "message." (e.g. "message.fhirResource") if a later step (e.g. Payload Builder) needs to read it; see the message-prefix troubleshooting entry below.' },
                    { name: 'fields', type: 'array', required: false, description: 'Flat/single-value element mappings: {targetPath, sourcePath, fallbackPaths?, literalValue?, transform?, valueMap?}. targetPath is relative to the resource root (e.g. "birthDate", or a manually-indexed "identifier[0].value"). sourcePath should be "message.<field>" to read the inbound message.' },
                    { name: 'repeatingGroups', type: 'array', required: false, description: 'One entry per repeating element: {targetPath, rowsPath, fields: [...]}. rowsPath points to the array of source rows (e.g. "message.patientIdentifiers"); one sub-object is built per row (keeping multiple fields from the same row aligned), written at targetPath[0], targetPath[1], .... Each field\'s own sourcePath resolves relative to one row, not the pipeline root.' },
                ],
                troubleshooting: [
                    {
                        issue: 'Every field comes back empty even though the source data is definitely there',
                        cause: 'Pipeline step data lives under a top-level "message" key (inputData = {message: {...}, _metadata: {...}, ...}), not spread at the root — a bare sourcePath like "dob" only matches a literal top-level "dob" key, which normally doesn\'t exist.',
                        fix: 'Prefix sourcePath/rowsPath/outputField values with "message." (e.g. "message.dob", "message.patientIdentifiers"). Verified against a live pipeline run: bare paths produce a resource with only resourceType set; "message."-prefixed paths resolve birthDate/gender/identifier[] correctly, including a repeating identifier group staying aligned per source row.',
                    },
                    {
                        issue: 'A field is missing from the built resource entirely',
                        cause: 'sourcePath (and every fallbackPaths candidate) resolved to nothing, with no literalValue set — the field is skipped rather than written as an empty/null value. A named transform can also decide there\'s nothing to write (e.g. an unparsable decimal string).',
                        fix: 'Confirm sourcePath matches the exact field name your upstream step wrote (remembering the "message." prefix above), or add a literalValue as a final fallback.',
                    },
                    {
                        issue: 'A compound-shaped transform (cda_name_to_fhir, cda_address_to_fhir, cda_quantity_to_fhir, ...) silently produces no output',
                        cause: 'These transforms expect the resolved source value to already be a nested object shaped like the corresponding CDA structure (e.g. {given, family} for a name), not a flat scalar column — a shape mismatch is treated as "nothing to write," never an error.',
                        fix: 'Either point sourcePath at a nested JSON object already carrying those field names, or use a bare-scalar transform (string_direct, a *_status_to_fhir transform, cda_decimal_string_to_number) with a flat column instead — see GET /api/fhir/transforms for the full, live list with descriptions.',
                    },
                    {
                        issue: 'Repeating identifier/name/telecom entries have mismatched fields (e.g. identifier[0].value paired with the wrong system)',
                        cause: 'Using two independent top-level fields entries with manual [0]/[1] indices instead of one repeatingGroups entry — independent rows can drift out of alignment across rows.',
                        fix: 'Use repeatingGroups instead: one rowsPath drives all of that group\'s fields together, so system/value (or given/family, etc.) from the same source row always land in the same array element.',
                    },
                    {
                        issue: 'I need "if source field A has value X, populate this field one way, else populate it another way" — fhir.build\'s field row has no condition/predicate option',
                        cause: 'This is by design, not a gap: conditional branching already has a mature, general home at the pipeline level (if_then_else / switch_case step types), and duplicating that logic into fhir.build\'s own field-row schema would fragment the same concept across two step types instead of composing them.',
                        fix: 'Add an if_then_else (or switch_case) step BEFORE this one. Its condition evaluates the source field, and its set_value action (e.g. {action: "set_value", targetField: "message.maritalStatusNormalized", value: "M"}) writes the resolved value onto a plain message.* field. Then this step\'s field row just reads it normally: {targetPath: "maritalStatus", sourcePath: "message.maritalStatusNormalized"} — no conditional logic needed here at all, since the upstream step already resolved which value applies.',
                    },
                    {
                        issue: 'Narrative Fields section shows "Save the interface before configuring narrative fields"',
                        cause: 'Narrative field visibility is saved via a PATCH to /api/fhir/optional-segments/:interfaceId/:messageType — the step needs a resolved interface_id (from step.config.interface_id or the pipeline\'s own interfaceId) to know where to save.',
                        fix: 'Save the interface (and pipeline) first, then reopen this step\'s configuration to set narrative field visibility.',
                    },
                ],
                stepOutput: {
                    description: 'Available to every step after this one.',
                    fields: [
                        { name: 'fhirResource', type: 'object', description: 'FHIR R4 resource built from configured field mappings, ready for a Payload Builder (FHIR Bundle mode) step or direct delivery.' },
                    ],
                },
            },
            'hl7.build': {
                description: 'The no-code, format-agnostic on-ramp for a complete HL7 v2 message: maps CSV columns, DB query columns, or arbitrary JSON fields directly onto HL7 segment/field/component paths (e.g. "PID.3", "PID.5.1"), using field mappings (source path → field key, with optional value translation) instead of a script. MSH is always auto-populated (encoding characters, timestamp, control ID, message type) from top-level config, not a user-configured segment. Every other segment is configured explicitly, in the order it should appear in the message, with "single" (appears once) or "repeating" (one instance per source row) cardinality — the same "one sub-object per row" alignment guarantee fhir.build\'s repeatingGroups and cda.map_to_canonical\'s sections already provide, so multiple fields from the same source row (e.g. a lab result\'s test code and value) never drift out of alignment across repeats.',
                useCases: [
                    'Build an ADT, ORU, or any other HL7 v2 message from a CSV file (via a File Parser step) with zero new Go code — only step configuration',
                    'Build an HL7 message from a database query\'s result rows (via a Database Enrichment step)',
                    'Emit repeating OBX (or NK1, IN1, ...) segments from an array of source rows, one segment instance per row',
                    'Add a site-specific Z-segment (e.g. ZEM) with manually-defined field positions, optionally pre-filled from an existing inbound Z-segment template',
                    'Group a single flat CSV (one row per analyte result) into OBR orders each with their own nested OBX results (groupBy + childSegments) — or nest a child segment under an already-nested JSON structure with no grouping needed at all',
                ],
                example: {
                    messageType: 'ADT',
                    triggerEvent: 'A01',
                    version: '2.5.1',
                    outputField: 'message.hl7Message',
                    segments: [
                        {
                            segment: 'PID',
                            cardinality: 'single',
                            fields: [
                                { fieldKey: 'PID.3', sourcePath: 'message.mrn' },
                                { fieldKey: 'PID.5.1', sourcePath: 'message.lastName' },
                                { fieldKey: 'PID.5.2', sourcePath: 'message.firstName' },
                            ],
                        },
                        {
                            segment: 'OBX',
                            cardinality: 'repeating',
                            rowsPath: 'message.labResults',
                            fields: [
                                { fieldKey: 'OBX.3', sourcePath: 'testCode' },
                                { fieldKey: 'OBX.5', sourcePath: 'value' },
                            ],
                        },
                    ],
                },
                parameters: [
                    { name: 'messageType', type: 'string', required: true, description: 'HL7 message type, e.g. "ADT". Must have a compiled schema for version/triggerEvent.' },
                    { name: 'triggerEvent', type: 'string', required: false, description: 'HL7 trigger event, e.g. "A01".' },
                    { name: 'version', type: 'string', required: false, description: 'HL7 version. Default: "2.5.1".' },
                    { name: 'outputField', type: 'string', required: false, description: 'Pipeline field where the built HL7 message string is written. Default: "hl7Message" — prefix with "message." if a later step needs to read it; see the message-prefix troubleshooting entry below.' },
                    { name: 'sendingApplication / sendingFacility / receivingApplication / receivingFacility / processingId', type: 'string', required: false, description: 'MSH.3/MSH.4/MSH.5/MSH.6/MSH.11 overrides. sendingApplication defaults to "ezHealthKonnect", sendingFacility to "EHK", processingId to "P" — the same defaults the TCP/MLLP ACK generator uses.' },
                    { name: 'segments', type: 'array', required: false, description: 'One entry per non-MSH segment, in message order: {segment, cardinality: "single"|"repeating", rowsPath? (repeating only, e.g. "message.labResults"), condition? ({field, operator, value|compareToField}), fields: [{fieldKey, sourcePath, fallbackPaths?, valueMap?, literalValue?, condition?}]}. fieldKey is fully-qualified (e.g. "PID.5.1"), matching GET /api/hl7/canonical-fields exactly. sourcePath should be "message.<field>" for a "single" segment (resolves against the whole inbound message) but is relative to one row (no "message." prefix) inside a "repeating" segment\'s fields.' },
                    { name: 'segments[].condition / segments[].fields[].condition', type: 'object', required: false, description: 'Optional {field, operator, value} gate — same evaluator control.if_then_else/control.switch_case use (services/executors/condition.go). A segment condition decides whether that segment instance is built at all (once for "single", per-row for "repeating" — so one row of a repeating segment can be excluded without affecting the rest); a field condition decides whether just that field is populated. Evaluated against the row/segment data first, falling back to the top-level pipeline data for anything not found there — e.g. a repeating OBX row\'s condition can check either its own column or a top-level field like "message.country". Operators: equals, not_equals, contains, starts_with, ends_with, greater_than(_or_equal), less_than(_or_equal), exists, not_exists, regex_match, in_list.' },
                    { name: 'segments[].groupBy', type: 'array', required: false, description: 'Repeating segments only. Buckets rowsPath\'s rows by these column(s) BEFORE building one instance per bucket instead of one per row — e.g. a single flat CSV of (orderId, testName, analyte, value) rows grouped by "orderId" becomes one OBR per distinct order instead of one per analyte row. This segment\'s own fields resolve from each bucket\'s FIRST row (e.g. testName); a row missing the groupBy column becomes its own singleton bucket rather than being dropped.' },
                    { name: 'segments[].groupedItemsKey', type: 'string', required: false, description: 'Names the field a groupBy bucket\'s own rows are exposed under to childSegments (default "_rows"). Only meaningful when groupBy is set.' },
                    { name: 'segments[].childSegments', type: 'array', required: false, description: 'Segments (same shape as segments[]) built immediately after EACH instance of this segment — the only way to correctly interleave e.g. OBX results under their own OBR before the next OBR, since HL7\'s flat wire format has no explicit nesting. A child\'s rowsPath/condition resolve against the parent\'s current row (or, when the parent used groupBy, that bucket\'s first row plus its groupedItemsKey) — set a child\'s rowsPath to "_rows" (or a custom groupedItemsKey) to iterate a groupBy\'d parent\'s own bucket, or to an already-nested field name (e.g. "results") when the source data arrives pre-nested and no groupBy is needed at all.' },
                ],
                troubleshooting: [
                    {
                        issue: 'Every field comes back empty even though the source data is definitely there',
                        cause: 'Pipeline step data lives under a top-level "message" key (inputData = {message: {...}, _metadata: {...}, ...}), not spread at the root — a bare sourcePath like "mrn" only matches a literal top-level "mrn" key, which normally doesn\'t exist.',
                        fix: 'Prefix "single"-segment sourcePath/rowsPath/outputField values with "message." (e.g. "message.mrn", "message.labResults"). Verified against a live pipeline run: bare paths built an MSH-only message (PID/OBX both came back empty); "message."-prefixed paths correctly produced "PID|||MRN123||Doe^Jane" and two aligned "OBX|||GLU||95" / "OBX|||K||4.1" segments.',
                    },
                    {
                        issue: 'A repeating segment (e.g. OBX) is missing from the output entirely, or has fewer instances than expected',
                        cause: 'rowsPath didn\'t resolve to an array on the pipeline data, or a given row produced zero mapped fields (every configured sourcePath/fallbackPaths came back empty, with no literalValue) — that row\'s segment instance is skipped rather than emitted empty.',
                        fix: 'Confirm rowsPath matches the exact field name your upstream step wrote (commonly "message.records" for a File Parser step\'s output, "message.rows" for a Database Enrichment step\'s) and that at least one field mapping per row has real data.',
                    },
                    {
                        issue: 'A field lands in the wrong position, or components/subcomponents overwrite each other',
                        cause: 'fieldKey must be fully-qualified and belong to the segment it\'s configured under — a mismatched segment prefix (e.g. "OBX.5" inside a PID segment block) is rejected rather than silently written to the wrong place.',
                        fix: 'Use the exact fieldKey values GET /api/hl7/canonical-fields/:messageType/:triggerEvent/:segment returns (e.g. "PID.5.1" for the name\'s family component, "PID.5.2" for given).',
                    },
                    {
                        issue: 'A coded field (e.g. sex, status) doesn\'t match the target HL7 table\'s expected values',
                        cause: 'The source system\'s own vocabulary differs from the HL7 table this field is bound to (e.g. table 0001 Administrative Sex expects M/F/O/U/A/N) — this step does plain value copying by default, with no built-in code translation.',
                        fix: 'Add a Value Map on that field mapping (e.g. {"male": "M", "female": "F"}) to translate the raw source value before it\'s written.',
                    },
                    {
                        issue: 'The "Add Segment" dropdown doesn\'t offer a segment I know is valid for this message type',
                        cause: 'The picker defaults to only the segments grammatically reachable next, given what\'s already configured (e.g. EVN must be added before PID becomes reachable in ADT^A01) — a deliberately conservative, forward-only guardrail, not a full conformance validator.',
                        fix: 'Use the "⚠ Add anyway (non-standard)" option in the same dropdown to pick from every segment in the schema regardless of position.',
                    },
                    {
                        issue: 'A field I need isn\'t in the Field/Component picker at all',
                        cause: 'No compiled HL7 schema encodes subcomponent-level detail (a 3rd dotted position, e.g. "PID.5.7.2") — the catalog genuinely stops at field+component, and Z-segments have no schema entry at all.',
                        fix: 'Use the picker\'s "Custom / atomized path…" toggle to type the exact fieldKey (e.g. "PID.5.7.2", "ZEM.3") — validated against the same positional format hl7/builder.Segment.Set accepts, so it will resolve correctly at build time.',
                    },
                ],
                stepOutput: {
                    description: 'Available to every step after this one.',
                    fields: [
                        { name: 'hl7Message', type: 'string', description: 'Complete HL7 v2 message built from configured MSH/segment mappings, ready for delivery via a TCP/MLLP Outbound (or other) connector.' },
                    ],
                },
            },
    };
    Object.keys(docs).forEach((stepType) => StepDocumentationRegistry.register(stepType, docs[stepType]));
})();
