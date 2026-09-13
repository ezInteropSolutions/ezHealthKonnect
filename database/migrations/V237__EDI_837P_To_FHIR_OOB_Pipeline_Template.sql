-- V237: EDI X12 837P (Professional Claim) to FHIR OOB pipeline template
-- Applied: 2026-09-11

-- ============================================================
-- EDI 837P -> FHIR (Claim + Patient + Coverage + Organization) TEMPLATE
-- ============================================================
-- A NEW, separate interface template — V236's existing store-only
-- "edi-837-inbound-sftp" template (both 837P/837I, parse+validate only) is
-- left completely unmodified. This one is professional-claim-specific and
-- carries the mapping all the way to a validated FHIR R4 Bundle, mirroring
-- V231's own 835-to-FHIR structure and V212's own PAS Zone-2 "derive -> Nx
-- fhir.build -> payload.builder -> fhir_validation" chain:
--   seq 5    connector.inbound   (edi_x12_inbound  — polls an SFTP directory for 837P files)
--   seq 20   edi.parse           (raw X12 -> structured JSON)
--   seq 30   edi.validate        (base X12 5010 standard checks; errors block, warnings never do)
--   seq 95   enrichment.script   (Derive 837P Claim Contexts — pure data reshaping only)
--   seq 100  fhir.build          (Organization — billing provider, built once per file)
--   seq 105  fhir.build          (Patient — rowsPath mode, one per claim context)
--   seq 110  fhir.build          (Coverage — rowsPath mode, one per claim context)
--   seq 115  fhir.build          (Claim — rowsPath mode, one per claim context)
--   seq 200  payload.builder     (fhir_bundle mode — assembles the Bundle)
--   seq 210  fhir_validation     (strict — validates the assembled Bundle)
--   seq 295  connector.outbound  (sink_outbound — store-only terminal, no external I/O)
--
-- Full multi-claim, multi-line-item mapping (user-confirmed scope, unlike
-- 835's own first-pass single-resource shape) — achieved WITHOUT a
-- control.loop wrapper step. A control.loop's own childStepIds config can
-- only hold real, DB-assigned step IDs, which don't exist yet at
-- template-authoring time (see V231's own documented finding: "no template
-- JSON can pre-declare that link") — the same constraint that stopped V231
-- from pre-wiring per-claim ExplanationOfBenefit resources. Solved generically
-- instead: fhir.build gained a new `rowsPath` config key (see
-- services/executors/transform/fhir_build_executor.go's own doc comment) that
-- builds ONE resource PER ROW found at that path, writing an ARRAY to
-- outputField — Patient/Coverage/Claim below each use it against
-- `_claim_contexts` (produced by the one enrichment.script step), needing no
-- loop step or step-ID wiring at all. payload.builder's fhir_bundle mode
-- already accepts array-valued resourcePaths entries (proven by V231's own
-- comment/design), so the 3 arrays plug in directly alongside the single
-- Organization resource.
--
-- Config here is transcribed verbatim from services/edi_837p_fhir_builder_test.go
-- (TestEDI837PFHIRBuilder_MultiClaimMultiLine_BuildsCleanValidatingBundle),
-- which proves this exact chain against a real, multi-claim, multi-line-item
-- fixture (a subscriber''s own claim with 2 service lines/2 diagnoses, plus a
-- dependent''s own claim with 1 service line/1 diagnosis) via the real
-- executors, and asserts the resulting Bundle validates at strict level with
-- zero unexpected errors (the one known, pre-existing, out-of-scope gap —
-- schemas/fhir/R4/valuesets/ClaimTypes.gz compiled with empty codes — is
-- explicitly excluded, matching pas_fhir_builder_test.go''s own precedent).
--
-- Named simplifications, stated up front rather than silently left out:
--   - Assumes ONE billing provider (2000A) per file — the overwhelmingly
--     common real shape; a file with multiple 2000A occurrences only maps
--     its first one to the Organization resource.
--   - Patient/Coverage are built PER CLAIM CONTEXT, not deduplicated across
--     multiple claims for the same real-world patient — a patient with 2
--     claims in one file gets 2 Bundle entries sharing the same stable id
--     (harmless per FHIR Bundle semantics).
--   - No separate Practitioner resource is built for careTeam providers
--     (rendering/referring) — Claim.careTeam[].provider uses a logical
--     (identifier-only) reference instead, avoiding the need to deduplicate
--     N distinct providers across a file into stable, correlated resource ids.
--   - Claim.priority has no X12 837 equivalent (a PAS/preauthorization
--     concept, not a claim-submission one) — fixed to "normal".
--   - NTE/K3/CRC/PWK/most of REF, and item-level diagnosisSequence/
--     careTeamSequence pointer arrays (fhir.build''s repeatingGroups builds
--     object rows, not primitive-array rows) are not mapped in this first
--     pass — Claim''s core identifying/clinical/financial shape is the
--     target, matching 835''s own "core fields, not exhaustive" precedent.
--   - 999 -> FHIR mapping remains explicitly out of scope (a transport-layer
--     acknowledgment has no natural FHIR analogue) and 837I -> FHIR is a
--     separate, following migration (institutional needs a genuinely
--     different derive script and Claim shape — SV2 not SV1, CL1 ->
--     supportingInfo[], HI-carried procedure codes and Present-on-Admission).

INSERT INTO interface_templates
    (id, name, slug, description, category, subcategory, tags, icon, difficulty,
     source_connector_type, source_config_template,
     target_connector_type, target_config_template,
     pipeline_config, message_type,
     required_connection_fields, sanitized_fields, preview_steps,
     estimated_setup_minutes, is_system, is_public, author, usage_count)
VALUES
(
    gen_random_uuid(),
    'EDI X12 837P to FHIR (SFTP)',
    'edi-837p-to-fhir-sftp',
    'Poll an SFTP directory for X12 837P (Professional Claim) files, parse and validate them, then build and validate a FHIR R4 Bundle containing Organization, Patient, Coverage, and Claim resources — one Patient/Coverage/Claim per claim in the file (full multi-claim, multi-service-line mapping, not a single-claim summary).',
    'edi',
    null,
    ARRAY['edi','x12','837p','professional','claims','providers','sftp','fhir'],
    '🩺',
    'advanced',
    'edi_x12_inbound',
    '{"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["837P"]}',
    'sink_outbound',
    '{"enable_logging": true, "enable_validation": true}',
    $pipeline$
    {
        "execution_groups": [
            {
                "sequence": 5,
                "steps": [{
                    "step_name": "Receive 837P File (SFTP)",
                    "step_type": "connector.inbound",
                    "sequence": 5,
                    "config": {
                        "connectorType": "edi_x12_inbound",
                        "config": {"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["837P"]}
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 20,
                "steps": [{
                    "step_name": "Parse 837P -> JSON",
                    "step_type": "edi.parse",
                    "sequence": 20,
                    "config": {"sourceField": "raw", "outputField": "parsedEDI", "transactionSet": "837P"},
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 30,
                "steps": [{
                    "step_name": "Validate Against X12 5010",
                    "step_type": "edi.validate",
                    "sequence": 30,
                    "config": {"sourceField": "raw", "outputField": "ediValidation", "customRules": []},
                    "enabled": true,
                    "required": false
                }]
            },
            {
                "sequence": 95,
                "steps": [{
                    "step_name": "Derive 837P Claim Contexts",
                    "step_alias": "derive_837p_claim_contexts",
                    "step_type": "enrichment.script",
                    "sequence": 95,
                    "description": "Pure data reshaping only -- no FHIR resource-shape or profile knowledge. Flattens 837P's nested billing-provider/subscriber/dependent/claim/service-line/diagnosis structure into one flat array (_claim_contexts) so the fhir.build steps below can build one Patient/Coverage/Claim resource per claim declaratively via rowsPath, with no control.loop wrapper step needed.",
                    "config": {
                        "script": "\n// -- Derive 837P Claim Contexts --\n// A prior step's plain top-level output field (edi.parse's own \"parsedEDI\",\n// per its outputField config) is NOT exposed to a later step's own \"input\"\n// at the top level -- the real pipeline engine nests it under\n// input.message.parsedEDI instead (confirmed by direct instrumentation\n// against a real Test Pipeline run; this project's own prior finding\n// documents the same \"message.\" prefix requirement for fhir.build/hl7.build\n// sourcePath strings, and it turns out to apply here too). Prefer that real\n// shape; fall back to the bare top-level key for Go-level test harnesses\n// that construct { parsedEDI: ... } directly without a \"message\" wrapper\n// (as every *_fhir_builder_test.go and edi_837_real_parser_roundtrip_test.go\n// fixture in this codebase already does).\nvar parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};\nvar loops = parsed.loops || {};\n\n// edi.parse's own \"transactionSet\" config key is NOT read by the executor\n// (services/executors/transform/edi_parse_executor.go's ediParseConfig has\n// no such field) -- the real variant resolution happens inside the parser\n// via the composite ST01+GS08 lookup (edi/loop_engine.go), keyed off the\n// FILE's own real GS08, not any step config. A stray 837I file landing in\n// this professional-only mailbox would otherwise silently run through this\n// SV1-shaped derive logic and produce wrong/incomplete Claim resources with\n// no error. Guard explicitly instead.\nif (parsed.transactionSet !== \"837P\") {\n  return ({ _claim_contexts: [], _billing_provider: {} });\n}\n\nfunction arr(v) {\n  if (!v) return [];\n  return Array.isArray(v) ? v : [v];\n}\nfunction first(v) {\n  var a = arr(v);\n  return a.length > 0 ? a[0] : {};\n}\n\nfunction genderToFHIR(code) {\n  if (code === \"M\") return \"male\";\n  if (code === \"F\") return \"female\";\n  return \"unknown\";\n}\n\nfunction relationshipToFHIR(code) {\n  var map = { \"18\": \"self\", \"01\": \"spouse\", \"19\": \"child\", \"20\": \"employee\" };\n  return map[code] || \"other\";\n}\n\nvar billingLevel = first(loops[\"2000A\"]);\nvar billingProviderLoop = (billingLevel.loops || {})[\"2010AA\"] || {};\nvar billingNM1 = first(billingProviderLoop.NM1);\nvar billingProvider = {\n  npi: billingNM1.identificationCode || \"\",\n  name: billingNM1.nameLastOrOrganizationName || \"\"\n};\n\n// HI is maxUse \">1\" -- a claim can carry SEVERAL separate HI segment\n// occurrences at the same 2300 level (one per qualifier-group; see\n// edi/schemas/x12_005010/segments/HI.json's own maxUse-correction note for\n// why, found via testing against real, unedited samples), each with its own\n// up-to-12-entry codes[] repeat group. Flatten every occurrence's codes[]\n// into one combined list before filtering by qualifier.\nfunction allHICodes(hiList) {\n  var instances = arr(hiList);\n  var out = [];\n  for (var i = 0; i < instances.length; i++) {\n    var codes = (instances[i] && instances[i].codes) || [];\n    for (var j = 0; j < codes.length; j++) out.push(codes[j]);\n  }\n  return out;\n}\n\nfunction extractDiagnoses(codes) {\n  var out = [];\n  for (var i = 0; i < codes.length; i++) {\n    var c = (codes[i] && codes[i].code) || {};\n    if (!c.code) continue;\n    out.push({ code: c.code, sequence: out.length + 1 });\n  }\n  return out;\n}\n// NOTE: 837P's own HI never carries a Present-on-Admission indicator (that's\n// an institutional-only concept, see 837I's own extractDiagnoses), so no\n// on_admission key is ever added here -- unlike 837I, which does.\n\nfunction diagnosisPointers(sv1) {\n  var ptr = (sv1 && sv1.diagnosisCodePointer) || {};\n  var out = [];\n  [\"pointer1\", \"pointer2\", \"pointer3\", \"pointer4\"].forEach(function (key) {\n    if (ptr[key]) out.push(parseInt(ptr[key], 10));\n  });\n  return out;\n}\n\nfunction extractServiceLines(claimLoops) {\n  var lines = arr((claimLoops || {})[\"2400\"]);\n  var out = [];\n  for (var i = 0; i < lines.length; i++) {\n    var line = lines[i];\n    var lx = line.LX || {};\n    var sv1 = line.SV1 || {};\n    var proc = sv1.procedureCode || {};\n    var dtpList = arr(line.DTP);\n    var svc = resolveServicedDate(dtpList);\n    out.push({\n      sequence: parseInt(lx.assignedNumber || String(i + 1), 10),\n      procedure_code: proc.code || \"\",\n      procedure_system: (proc.qualifier === \"HC\") ? \"http://www.ama-assn.org/go/cpt\" : \"\",\n      quantity: sv1.serviceUnitCount ? Number(sv1.serviceUnitCount) : 1,\n      net: sv1.lineItemChargeAmount ? Number(sv1.lineItemChargeAmount) : 0,\n      serviced_date: svc.servicedDate,\n      serviced_start: svc.servicedStart,\n      serviced_end: svc.servicedEnd,\n      diagnosis_pointers: diagnosisPointers(sv1)\n    });\n  }\n  return out;\n}\n\n// DTP02 (Date/Time Period Format Qualifier) \"D8\" is a single CCYYMMDD date;\n// \"RD8\" is a CCYYMMDD-CCYYMMDD range. Claim.item.serviced[x] is a choice type\n// (servicedDate | servicedPeriod) -- only one of the two shapes below is ever\n// populated per DTP*472 occurrence, matching that choice. Found only by\n// testing against a real, unedited 837 sample (databricks-industry-\n// solutions/x12-edi-parser's own CC_837P_EDI.txt/CC_837I_EDI.txt test\n// fixtures) carrying genuine RD8 ranges -- the synthetic Go-test fixture\n// only ever used D8 dates, so this gap was invisible there.\nfunction resolveServicedDate(dtpList) {\n  for (var d = 0; d < dtpList.length; d++) {\n    if (dtpList[d].dateTimeQualifier !== \"472\") continue;\n    var period = dtpList[d].datePeriod || \"\";\n    if (dtpList[d].dateTimePeriodFormatQualifier === \"RD8\" && period.indexOf(\"-\") !== -1) {\n      var parts = period.split(\"-\");\n      return { servicedDate: \"\", servicedStart: parts[0] || \"\", servicedEnd: parts[1] || \"\" };\n    }\n    return { servicedDate: period, servicedStart: \"\", servicedEnd: \"\" };\n  }\n  return { servicedDate: \"\", servicedStart: \"\", servicedEnd: \"\" };\n}\n\n// Covers every 2310A-F role that is genuinely a CARE TEAM MEMBER (a person or\n// organization providing care) -- 2310A Referring (DN), 2310B Rendering\n// (82), 2310D Supervising (DQ). 2310C Service Facility Location (77) is a\n// PLACE, not a care team member -- mapped separately to Claim.facility (see\n// extractFacility below), matching the base FHIR R4 Claim resource's own\n// dedicated 0..1 Reference field for exactly this concept. 2310E/2310F\n// Ambulance Pick-up/Drop-off Location (PW/45) are also genuinely locations\n// (addresses, not provider entities) with no equivalent base-Claim field --\n// a named, deliberate gap, not modeled in this pass. Found via X12.org's own\n// official COB example (Example 3a), whose 2310C carried a real NPI that a\n// prior version of this function silently dropped entirely.\nfunction extractCareTeam(claimLoops) {\n  var team = [];\n  var roleLoops = [\n    { key: \"2310A\", role: \"referring\" },\n    { key: \"2310B\", role: \"rendering\" },\n    { key: \"2310D\", role: \"supervising\" }\n  ];\n  for (var r = 0; r < roleLoops.length; r++) {\n    var list = arr((claimLoops || {})[roleLoops[r].key]);\n    for (var i = 0; i < list.length; i++) {\n      var nm1 = first(list[i].NM1);\n      if (nm1.identificationCode) {\n        team.push({ npi: nm1.identificationCode, role: roleLoops[r].role, sequence: team.length + 1 });\n      }\n    }\n  }\n  return team;\n}\n\n// Claim.facility (base FHIR R4, 0..1 Reference) -- \"Facility where the\n// services were provided.\" A logical (identifier-only) reference, same\n// pattern as careTeam[].provider -- no separate Location resource is built,\n// same rationale as the careTeam design note above.\nfunction extractFacility(claimLoops) {\n  var loop = (claimLoops || {})[\"2310C\"];\n  if (!loop) return {};\n  var nm1 = first(loop.NM1);\n  if (!nm1.identificationCode) return {};\n  return { facility_npi: nm1.identificationCode, facility_name: nm1.nameLastOrOrganizationName || \"\" };\n}\n\n// Claim.created is required by the base FHIR R4 Claim resource but has no\n// X12 837 equivalent (837 carries no \"claim record created\" timestamp) --\n// the same \"record processing time, not sourced from the message\" default\n// convention BHT03/BHT04 (creation date/time) already establish for the\n// interchange itself. Computed once for the whole file, not per claim.\nvar nowISO = new Date().toISOString();\n\n// NOTE: the returned object's own property names are snake_case throughout\n// (patient_info, total_charge_amount, etc.), NOT the camelCase a reader might\n// expect from the rest of this script's own internal variable names. This is\n// REQUIRED, not a style choice: a later fhir.build step can only reach this\n// enrichment.script's own return value via the \"steps.<alias>.step_output.<key>\"\n// reference path (BaseExecutor.SetStepOutputWithDetails stashes it in\n// inputData[\"_stepOutput\"], which executeStepWithContext extracts into that\n// per-step snapshot and then deletes from what actually flows forward as\n// plain inputData -- it never reaches a later step at the top level or under\n// \"message\"). That snapshot is ALWAYS passed through\n// models.OutputNormalizer.NormalizeStepOutput first, which snake_cases every\n// key it doesn't already recognize as snake_case -- so a later step's own\n// sourcePath/rowsPath config strings (see claim837PBuildConfig et al.) MUST\n// address these fields by their post-normalization names. Matching that\n// reality here, instead of writing camelCase and forcing every config\n// sourcePath to reference a silently-renamed key, is what keeps the script\n// and the config it feeds honest about what data shape actually flows\n// between them. Found only by a real browser Test-Pipeline run -- the\n// Go-level tests' own svcInjectStepOutput helper happens to preserve\n// whatever casing the script returns unchanged, so a camelCase/snake_case\n// mismatch here was invisible there.\nfunction buildClaimContext(patientInfo, subscriberInfo, payerInfo, claimLoop) {\n  var clm = claimLoop.CLM || {};\n  var svcLoc = clm.healthCareServiceLocation || {};\n  var facility = extractFacility(claimLoop.loops);\n  var claim = {\n    patient_control_number: clm.patientControlNumber || \"\",\n    total_charge_amount: clm.totalClaimChargeAmount ? Number(clm.totalClaimChargeAmount) : 0,\n    place_of_service_code: svcLoc.placeOfServiceCode || \"\",\n    created_at: nowISO,\n    diagnosis_list: extractDiagnoses(allHICodes(claimLoop.HI)),\n    service_lines: extractServiceLines(claimLoop.loops),\n    care_team: extractCareTeam(claimLoop.loops)\n  };\n  if (facility.facility_npi) {\n    claim.facility_npi = facility.facility_npi;\n    claim.facility_name = facility.facility_name;\n  }\n  return {\n    patient_info: patientInfo,\n    subscriber_info: subscriberInfo,\n    payer_info: payerInfo,\n    claim: claim\n  };\n}\n\nfunction personFromNM1AndDMG(nm1, dmg, relationshipCode) {\n  var gender = dmg.genderCode || \"\";\n  return {\n    first_name: nm1.nameFirst || \"\",\n    last_name: nm1.nameLastOrOrganizationName || \"\",\n    member_id: nm1.identificationCode || \"\",\n    dob: dmg.birthDate || \"\",\n    gender_fhir: genderToFHIR(gender),\n    relationship_code: relationshipCode || \"\",\n    relationship_fhir: relationshipToFHIR(relationshipCode || \"\")\n  };\n}\n\nvar claimContexts = [];\n\nvar billingLoops = billingLevel.loops || {};\nvar subscriberLevels = arr(billingLoops[\"2000B\"]);\nfor (var s = 0; s < subscriberLevels.length; s++) {\n  var subLevel = subscriberLevels[s];\n  var subLoops = subLevel.loops || {};\n  var sbr = subLevel.SBR || {};\n\n  var subNM1 = first((subLoops[\"2010BA\"] || {}).NM1);\n  var subDMG = (subLoops[\"2010BA\"] || {}).DMG || {};\n  var subscriberInfo = personFromNM1AndDMG(subNM1, subDMG, \"18\");\n\n  var payerNM1 = first((subLoops[\"2010BB\"] || {}).NM1);\n  var payerInfo = {\n    name: payerNM1.nameLastOrOrganizationName || \"\",\n    payer_id: payerNM1.identificationCode || \"\"\n  };\n\n  var dependentLevels = arr(subLoops[\"2000C\"]);\n  if (dependentLevels.length > 0) {\n    for (var d = 0; d < dependentLevels.length; d++) {\n      var depLevel = dependentLevels[d];\n      var depLoops = depLevel.loops || {};\n      var depNM1 = first((depLoops[\"2010CA\"] || {}).NM1);\n      var depDMG = (depLoops[\"2010CA\"] || {}).DMG || {};\n      var depPAT = depLevel.PAT || {};\n      // Real, spec-compliant 837 data carries the dependent's own relationship\n      // code on PAT01 (2000C) -- confirmed against X12.org's own official\n      // \"Ben Kildare Service\" 837P example, which leaves the subscriber's own\n      // SBR02 (2000B) BLANK whenever a 2000C dependent loop exists and puts\n      // the real value on PAT01 instead (HL*2...SBR*P**2222-SJ*******CI [SBR02\n      // blank] -> HL*3...PAT*19 [child]). Prefer PAT01; fall back to SBR02 for\n      // trading partners that populate it there instead (flexible, not rigid).\n      var patientInfo = personFromNM1AndDMG(depNM1, depDMG, depPAT.individualRelationshipCode || sbr.individualRelationshipCode);\n      if (!patientInfo.member_id) patientInfo.member_id = subscriberInfo.member_id + \"-DEP\" + (d + 1);\n\n      var depClaims = arr(depLoops[\"2300\"]);\n      for (var dc = 0; dc < depClaims.length; dc++) {\n        claimContexts.push(buildClaimContext(patientInfo, subscriberInfo, payerInfo, depClaims[dc]));\n      }\n    }\n  } else {\n    var subClaims = arr(subLoops[\"2300\"]);\n    for (var sc = 0; sc < subClaims.length; sc++) {\n      claimContexts.push(buildClaimContext(subscriberInfo, subscriberInfo, payerInfo, subClaims[sc]));\n    }\n  }\n}\n\nreturn ({\n  _claim_contexts: claimContexts,\n  _billing_provider: billingProvider\n});\n"
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 100,
                "steps": [{
                    "step_name": "Build Organization",
                    "step_alias": "build_organization_fhir",
                    "step_type": "fhir.build",
                    "sequence": 100,
                    "description": "Builds the billing provider (2010AA) as a FHIR Organization resource, once per file, with a stable literal id referenced by every Claim.provider below.",
                    "config": {
                        "resourceType": "Organization",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirOrganization",
                        "fields": [
                            {"targetPath": "id", "literalValue": "organization-billing"},
                            {"targetPath": "active", "literalValue": "true"},
                            {"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
                            {"targetPath": "identifier[0].value", "sourcePath": "steps.derive_837p_claim_contexts.step_output._billing_provider.npi"},
                            {"targetPath": "name", "sourcePath": "steps.derive_837p_claim_contexts.step_output._billing_provider.name"}
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 105,
                "steps": [{
                    "step_name": "Build Patient",
                    "step_alias": "build_patient_fhir",
                    "step_type": "fhir.build",
                    "sequence": 105,
                    "description": "Builds one FHIR Patient resource per claim context (rowsPath mode) -- the patient is the subscriber when they are their own patient, or the dependent (2010CA) otherwise.",
                    "config": {
                        "resourceType": "Patient",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirPatients",
                        "rowsPath": "steps.derive_837p_claim_contexts.step_output._claim_contexts",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "patient_info.member_id"},
                            {"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/x12-member-id"},
                            {"targetPath": "identifier[0].value", "sourcePath": "patient_info.member_id"},
                            {"targetPath": "name[0].family", "sourcePath": "patient_info.last_name"},
                            {"targetPath": "name[0].given[0]", "sourcePath": "patient_info.first_name"},
                            {"targetPath": "birthDate", "sourcePath": "patient_info.dob", "transform": "x12_date_to_fhir_date"},
                            {"targetPath": "gender", "sourcePath": "patient_info.gender_fhir"}
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 110,
                "steps": [{
                    "step_name": "Build Coverage",
                    "step_alias": "build_coverage_fhir",
                    "step_type": "fhir.build",
                    "sequence": 110,
                    "description": "Builds one FHIR Coverage resource per claim context (rowsPath mode). subscriber.reference is only written when the patient IS the subscriber (relationshipCode 18) -- otherwise it would dangle, since no separate Patient resource is built for the subscriber in the dependent case; subscriberId (a plain string) is always populated regardless.",
                    "config": {
                        "resourceType": "Coverage",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirCoverages",
                        "rowsPath": "steps.derive_837p_claim_contexts.step_output._claim_contexts",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "claim.patient_control_number", "transform": "string_prefix", "valueMap": {"prefix": "coverage-"}},
                            {"targetPath": "status", "literalValue": "active"},
                            {"targetPath": "beneficiary.reference", "sourcePath": "patient_info.member_id", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}},
                            {"targetPath": "subscriberId", "sourcePath": "subscriber_info.member_id"},
                            {"targetPath": "subscriber.reference", "sourcePath": "subscriber_info.member_id", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}, "condition": {"field": "patient_info.relationship_code", "operator": "equals", "value": "18"}},
                            {"targetPath": "relationship.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/subscriber-relationship"},
                            {"targetPath": "relationship.coding[0].code", "sourcePath": "patient_info.relationship_fhir"},
                            {"targetPath": "payor[0].identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
                            {"targetPath": "payor[0].identifier.value", "sourcePath": "payer_info.payer_id"},
                            {"targetPath": "payor[0].display", "sourcePath": "payer_info.name"}
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 115,
                "steps": [{
                    "step_name": "Build Claim",
                    "step_alias": "build_claim_fhir",
                    "step_type": "fhir.build",
                    "sequence": 115,
                    "description": "Builds one FHIR Claim resource per claim context (rowsPath mode). diagnosis[]/item[]/careTeam[] built via repeatingGroups off the context's own pre-flattened arrays. careTeam[].provider is a logical (identifier-only) reference -- no separate Practitioner resource is built. priority is fixed (no X12 837 equivalent).",
                    "config": {
                        "resourceType": "Claim",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirClaims",
                        "rowsPath": "steps.derive_837p_claim_contexts.step_output._claim_contexts",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "claim.patient_control_number", "transform": "string_prefix", "valueMap": {"prefix": "claim-"}},
                            {"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/x12-claim-control-number"},
                            {"targetPath": "identifier[0].value", "sourcePath": "claim.patient_control_number"},
                            {"targetPath": "status", "literalValue": "active"},
                            {"targetPath": "type.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/claim-type"},
                            {"targetPath": "type.coding[0].code", "literalValue": "professional"},
                            {"targetPath": "use", "literalValue": "claim"},
                            {"targetPath": "created", "sourcePath": "claim.created_at"},
                            {"targetPath": "priority.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/processpriority"},
                            {"targetPath": "priority.coding[0].code", "literalValue": "normal"},
                            {"targetPath": "patient.reference", "sourcePath": "patient_info.member_id", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}},
                            {"targetPath": "provider.reference", "literalValue": "Organization/organization-billing"},
                            {"targetPath": "insurer.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
                            {"targetPath": "insurer.identifier.value", "sourcePath": "payer_info.payer_id"},
                            {"targetPath": "facility.identifier.system", "literalValue": "http://hl7.org/fhir/sid/us-npi", "condition": {"field": "claim.facility_npi", "operator": "exists"}},
                            {"targetPath": "facility.identifier.value", "sourcePath": "claim.facility_npi"},
                            {"targetPath": "total.value", "sourcePath": "claim.total_charge_amount"},
                            {"targetPath": "total.currency", "literalValue": "USD"},
                            {"targetPath": "insurance[0].sequence", "literalValue": "1"},
                            {"targetPath": "insurance[0].focal", "literalValue": "true"},
                            {"targetPath": "insurance[0].coverage.reference", "sourcePath": "claim.patient_control_number", "transform": "string_prefix", "valueMap": {"prefix": "Coverage/coverage-"}}
                        ],
                        "repeatingGroups": [
                            {
                                "targetPath": "diagnosis",
                                "rowsPath": "claim.diagnosis_list",
                                "fields": [
                                    {"targetPath": "diagnosisCodeableConcept.coding[0].system", "literalValue": "http://hl7.org/fhir/sid/icd-10-cm"},
                                    {"targetPath": "diagnosisCodeableConcept.coding[0].code", "sourcePath": "code"},
                                    {"targetPath": "sequence", "sourcePath": "sequence"}
                                ]
                            },
                            {
                                "targetPath": "item",
                                "rowsPath": "claim.service_lines",
                                "fields": [
                                    {"targetPath": "sequence", "sourcePath": "sequence"},
                                    {"targetPath": "productOrService.coding[0].system", "sourcePath": "procedure_system"},
                                    {"targetPath": "productOrService.coding[0].code", "sourcePath": "procedure_code"},
                                    {"targetPath": "quantity.value", "sourcePath": "quantity"},
                                    {"targetPath": "net.value", "sourcePath": "net"},
                                    {"targetPath": "net.currency", "literalValue": "USD"},
                                    {"targetPath": "servicedDate", "sourcePath": "serviced_date", "transform": "x12_date_to_fhir_date"},
                                    {"targetPath": "servicedPeriod.start", "sourcePath": "serviced_start", "transform": "x12_date_to_fhir_date"},
                                    {"targetPath": "servicedPeriod.end", "sourcePath": "serviced_end", "transform": "x12_date_to_fhir_date"}
                                ]
                            },
                            {
                                "targetPath": "careTeam",
                                "rowsPath": "claim.care_team",
                                "fields": [
                                    {"targetPath": "sequence", "sourcePath": "sequence"},
                                    {"targetPath": "role.coding[0].code", "sourcePath": "role"},
                                    {"targetPath": "provider.identifier.system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
                                    {"targetPath": "provider.identifier.value", "sourcePath": "npi"}
                                ]
                            }
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 200,
                "steps": [{
                    "step_name": "Assemble 837P FHIR Bundle",
                    "step_alias": "assemble_837p_fhir_bundle",
                    "step_type": "payload.builder",
                    "sequence": 200,
                    "description": "Assembles Organization (single resource) plus the Patient/Coverage/Claim arrays (one entry per claim context) into one FHIR R4 Bundle with correctly cross-referenced fullUrls.",
                    "config": {
                        "mode": "fhir_bundle",
                        "fhirBundle": {
                            "bundleType": "collection",
                            "resourcePaths": ["message.fhirOrganization", "message.fhirPatients", "message.fhirCoverages", "message.fhirClaims"]
                        }
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 210,
                "steps": [{
                    "step_name": "Validate FHIR Bundle",
                    "step_type": "fhir_validation",
                    "sequence": 210,
                    "config": {
                        "validation_level": "strict",
                        "profile": "base",
                        "fhir_version": "R4",
                        "source_field": "payload"
                    },
                    "enabled": true,
                    "required": false
                }]
            },
            {
                "sequence": 295,
                "steps": [{
                    "step_name": "Store Result",
                    "step_type": "connector.outbound",
                    "sequence": 295,
                    "config": {
                        "connectorType": "sink_outbound",
                        "config": {"enable_logging": true, "enable_validation": true},
                        "contentField": "payload"
                    },
                    "enabled": true,
                    "required": true
                }]
            }
        ]
    }
    $pipeline$::jsonb,
    '837P',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_path","label":"Remote Directory","type":"string","hint":"Directory to poll for 837P files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive 837P (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse 837P","icon":"📄","step_type":"edi.parse"},
        {"name":"Validate X12","icon":"✅","step_type":"edi.validate"},
        {"name":"Derive Claim Contexts","icon":"🧩","step_type":"enrichment.script"},
        {"name":"Build Organization","icon":"🏥","step_type":"fhir.build"},
        {"name":"Build Patient","icon":"🧑","step_type":"fhir.build"},
        {"name":"Build Coverage","icon":"💳","step_type":"fhir.build"},
        {"name":"Build Claim","icon":"📋","step_type":"fhir.build"},
        {"name":"Assemble FHIR Bundle","icon":"📦","step_type":"payload.builder"},
        {"name":"Validate FHIR","icon":"🛡️","step_type":"fhir_validation"},
        {"name":"Store Result","icon":"💾","step_type":"connector.outbound"}
    ]',
    25,
    true,
    true,
    'ezHealthKonnect',
    0
)
ON CONFLICT (slug) DO UPDATE SET
    name                   = EXCLUDED.name,
    description            = EXCLUDED.description,
    pipeline_config        = EXCLUDED.pipeline_config,
    preview_steps          = EXCLUDED.preview_steps,
    tags                   = EXCLUDED.tags,
    updated_at             = NOW();
