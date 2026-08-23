/**
 * CoreMappingStepsDocs — Documentation for core.mapping and its aliases
 *
 * core.mapping (HL7 v2 -> FHIR R4 field mapping + structural assembly), plus the legacy
 * hl7_fhir_transform and field_mapping aliases.
 *
 * Bug fix during migration: field_mapping used to alias to a step type
 * (`core.transformation`) that was never defined anywhere in the old monolithic file,
 * so its Documentation tab silently fell through to the generic fallback text. Fixed to
 * alias to core.mapping, which is what field_mapping actually is.
 *
 * Self-registers into StepDocumentationRegistry at load time — this file must be
 * loaded (via <script>) after StepDocumentationRegistry.js and before any step's
 * Documentation tab is opened. Mirrors the StepBuilderRegistry.register() pattern
 * already used by every step's Configuration-tab builder
 * (public/js/pipeline/components/StepBuilderRegistry.js).
 */

// REMOVED: 'pre.enrichment.metadata' documentation - now merged with Field Mapping (core.mapping)

(function () {
    const docs = {
            'core.mapping': {
                description: 'Transforms HL7 v2 messages to FHIR R4 format in two passes:\n\n' +
                    '1. Field mapping — each row in the Visual Mapping tab maps one HL7 field (e.g. PID.5) to one FHIR path (e.g. Patient.name[0].family). ' +
                    'Enriched data from upstream steps can be referenced as source fields using the ["step_name"].enriched_data.field syntax.\n\n' +
                    '2. Structural assembly (Assembly tab) — runs automatically after field mapping for message types that need it. ' +
                    'For ORU^R01 this converts every OBX segment into a properly structured FHIR Observation (value type dispatch, ' +
                    'structured referenceRange, interpretation flags, laboratory category) and links them all to DiagnosticReport.result[]. ' +
                    'Each assembly rule can be individually disabled in the Assembly tab if your interface requires custom handling. ' +
                    'To extend beyond what the OOB rules produce, add a Script Enrichment step after this one — it receives the fully assembled fhirResources array.',
                useCases: [
                    'Convert ADT^A01 (patient admission) to FHIR Patient + Encounter + AllergyIntolerance',
                    'Transform ORU^R01 (lab results) to FHIR Observation + DiagnosticReport with OBX value dispatch',
                    'Map ORM^O01 (order) to FHIR ServiceRequest',
                    'Convert DFT^P03 (billing) to FHIR Claim',
                    'Enrich FHIR resources with lookup data from a prior Database Enrichment step',
                    'Disable specific assembly rules (e.g. referenceRange parsing) for non-standard interfaces',
                    'Add a Script Enrichment step after this one to post-process or extend assembled FHIR resources',
                    'Hide noisy or irrelevant fields from a resource type\'s auto-generated narrative summary (resource.text.div) via the Narrative Fields tab, without touching the field mappings that produce the resource itself'
                ],
                example: {
                    fhir_version: 'R4',
                    use_template: true,
                    assembleObservations: true,
                    assemblyRules: {
                        obs_value_dispatch: true,
                        obs_reference_range: true,
                        obs_interpretation: true,
                        dr_result_links: true
                    },
                    mappings: [
                        {
                            comment: 'Patient family name from PID.5',
                            hl7Field: 'PID.5',
                            fhirPath: 'Patient.name[0].family',
                            dataType: 'string'
                        },
                        {
                            comment: 'Use enriched data from database lookup',
                            hl7Field: '["database_enrichment"].enriched_data.insuranceProvider',
                            fhirPath: 'Coverage.payor[0].display',
                            dataType: 'string'
                        }
                    ]
                },
                parameters: [
                    { name: 'fhir_version', type: 'string', required: true, description: 'Target FHIR version. Currently R4 is fully supported.' },
                    { name: 'use_template', type: 'boolean', required: false, description: 'Use wizard-configured field mappings (default: true).' },
                    { name: 'mappings', type: 'array', required: false, description: 'Array of field mappings. Each entry maps one HL7 field or enriched value to one FHIR path.' },
                    { name: 'assembleObservations', type: 'boolean', required: false, description: 'Master switch for structural assembly. When false all assembly rules are skipped regardless of individual rule settings. Default: true.' },
                    { name: 'assemblyRules', type: 'object', required: false, description: 'Per-rule on/off flags for structural assembly. All rules default to true when absent. Set a key to false to disable that specific transform. Keys: collapse_text_obx, obs_value_dispatch, obs_value_unit, obs_code, obs_reference_range, obs_interpretation, obs_status, obs_category, obs_subject, obs_effective, obs_nte_note, dr_result_links, dr_subject, dr_code, dr_status, dr_effective, dr_category.' }
                ],
                assemblyRulesDoc: [
                    { key: 'collapse_text_obx',   src: 'OBX.3 + OBX.2', fhir: 'Observation.valueString', desc: '<strong>Merge mode</strong> — controlled in the "Text Report Merging" card at the top of the Assembly tab. When enabled, consecutive TX/FT OBX segments sharing the same OBX.3 code are joined into one Observation.valueString (the HL7 continuation-segment pattern). Example: a 50-line surgical pathology report sent as OBX|1|TX … OBX|50|TX all with OBX.3=4050097^Surg Path produces one Observation instead of 50. Disable (or restrict to specific services via <code>collapse_text_obx_services</code>) when your discrete lab results also happen to use the same OBX.3 across rows. <em>Examples where merging is correct: AP (Anatomic Pathology), RAD (Radiology), SP (Surgical Path). Examples where merging should be OFF or restricted: CH (Chemistry), MB (Microbiology) — each analyte has a distinct OBX.3.</em>' },
                    { key: 'obs_value_dispatch',  src: 'OBX.2 + OBX.5', fhir: 'value[x]',              desc: 'Dispatches OBX value to the correct FHIR type: NM→valueQuantity (float), CE/CWE→valueCodeableConcept, ST/TX→valueString, TS→valueDateTime.' },
                    { key: 'obs_value_unit',       src: 'OBX.6',         fhir: 'valueQuantity.unit',     desc: 'Populates unit + system on valueQuantity. Normalizes UCUM coding system to http://unitsofmeasure.org.' },
                    { key: 'obs_code',             src: 'OBX.3',         fhir: 'Observation.code',       desc: 'Builds CodeableConcept from OBX.3. Normalizes LN → http://loinc.org, local codes are preserved with display text.' },
                    { key: 'obs_reference_range',  src: 'OBX.7',         fhir: 'referenceRange[]',       desc: 'Parses "3.8-11.0" into structured {low:{value,unit}, high:{value,unit}, text}. Disable if your system uses non-numeric or multi-part ranges.' },
                    { key: 'obs_interpretation',   src: 'OBX.8',         fhir: 'interpretation[]',       desc: 'Maps abnormal flags to FHIR interpretation codes: H→High, L→Low, A→Abnormal, AA→Critical, HH→Critical High, LL→Critical Low, N is ignored.' },
                    { key: 'obs_status',           src: 'OBX.11',        fhir: 'Observation.status',     desc: 'Maps OBX status code to FHIR: F→final, P→preliminary, C→corrected, W→entered-in-error, X→cancelled.' },
                    { key: 'obs_category',         src: '(fixed)',        fhir: 'Observation.category[]', desc: 'Adds the standard laboratory category coding (http://terminology.hl7.org/CodeSystem/observation-category | laboratory).' },
                    { key: 'obs_subject',          src: 'Patient.id',    fhir: 'Observation.subject',    desc: 'Links Observation.subject → Patient/<id> using the Patient resource already in the output.' },
                    { key: 'obs_effective',        src: 'OBX.14',        fhir: 'effectiveDateTime',      desc: 'Converts OBX.14 from HL7 TS format (20120410160227) to ISO 8601 (2012-04-10T16:02:27+00:00).' },
                    { key: 'obs_nte_note',         src: 'NTE.3',         fhir: 'note[].text',             desc: 'Clubs consecutive NTE segments following an OBX into Observation.note[0].text. Multiple NTE lines joined with newlines; blank lines become paragraph breaks. NTE after OBR (before first OBX) goes to DiagnosticReport.note.' },
                    { key: 'dr_result_links',      src: 'Observation ids', fhir: 'DiagnosticReport.result[]', desc: 'Populates DiagnosticReport.result[] with references to all assembled Observations.' },
                    { key: 'dr_subject',           src: 'Patient.id',    fhir: 'DiagnosticReport.subject', desc: 'Links DiagnosticReport.subject → Patient/<id>.' },
                    { key: 'dr_code',              src: 'OBR.4',         fhir: 'DiagnosticReport.code',  desc: 'Builds CodeableConcept from OBR.4 (ordered test / panel code). Required field in FHIR R4 DiagnosticReport.' },
                    { key: 'dr_status',            src: 'OBR.25',        fhir: 'DiagnosticReport.status', desc: 'Maps OBR result status to FHIR: F→final, P→preliminary, C→corrected, X→cancelled.' },
                    { key: 'dr_effective',         src: 'OBR.7',         fhir: 'effectiveDateTime',      desc: 'Converts OBR.7 observation date/time from HL7 TS to ISO 8601. Preferred over OBX.14 for the report-level timestamp.' },
                    { key: 'dr_category',          src: '(fixed)',        fhir: 'DiagnosticReport.category[]', desc: 'Adds the standard LAB category coding (http://terminology.hl7.org/CodeSystem/v2-0074 | LAB).' }
                ],
                bestPractices: [
                    {
                        practice: 'Use the Narrative Fields tab to control resource.text.div content, not the Visual Mapping tab',
                        reason: 'Narrative field visibility is stored separately from step.config.mappings — per (interfaceId, messageType), via GET/PATCH /api/fhir/optional-segments — the same config surface the interface wizard\'s Step 4 panel and this step\'s cda.to_fhir/fhir.build siblings all share, since a resource built by any of them should narrate consistently.',
                        example: 'Hiding AllergyIntolerance.criticality from the narrative for one interface: expand it in the Narrative Fields tab and uncheck it — no change to the field mappings that put criticality on the resource in the first place.',
                    },
                ],
                troubleshooting: [
                    {
                        issue: 'A field I unchecked in the Narrative Fields tab still shows up in resource.text.div',
                        cause: 'Narrative field visibility is scoped per (interfaceId, messageType) — resolved from step.config.interface_id/message_type, falling back to the pipeline\'s own interfaceId/messageType. If those didn\'t resolve to what you expected when the tab loaded, the save landed on a different scope than the resource you\'re actually looking at.',
                        fix: 'Confirm the interface and message type shown when the tab loaded match the ones producing the resource you\'re inspecting, then re-check and re-save.',
                    },
                ],
                referenceVariables: {
                    title: 'Using Enriched Data in Field Mappings',
                    description: 'Reference data from any upstream step using the ["step_name"].enriched_data.field syntax in the HL7 Source column.',
                    examples: [
                        {
                            scenario: 'Database lookup result',
                            hl7Field: '["database_enrichment"].enriched_data.insuranceProvider',
                            fhirPath: 'Coverage.payor[0].display',
                            explanation: 'Maps the insuranceProvider field fetched from a database enrichment step into Coverage.payor'
                        },
                        {
                            scenario: 'Script calculation',
                            hl7Field: '["Script_Enrichment"].enriched_data.riskScore',
                            fhirPath: 'Observation.valueQuantity.value',
                            explanation: 'Uses a risk score calculated by a prior Script Enrichment step as the Observation value'
                        },
                        {
                            scenario: 'API response field',
                            hl7Field: '["API_Enrichment"].enriched_data.externalPatientId',
                            fhirPath: 'Patient.identifier[1].value',
                            explanation: 'Adds an external system ID retrieved from an API call as a second Patient identifier'
                        }
                    ]
                }
            },
    };
    Object.keys(docs).forEach((stepType) => StepDocumentationRegistry.register(stepType, docs[stepType]));
})();

StepDocumentationRegistry.registerAlias('hl7_fhir_transform', 'core.mapping');
StepDocumentationRegistry.registerAlias('field_mapping', 'core.mapping'); // bug fix — was aliased to undefined 'core.transformation'
