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
                        "script": "// -- Derive 837P Claim Contexts --\nvar parsed = input.parsedEDI || {};\nvar loops = parsed.loops || {};\n\n// edi.parse's own \"transactionSet\" config key is NOT read by the executor\n// (services/executors/transform/edi_parse_executor.go's ediParseConfig has\n// no such field) -- the real variant resolution happens inside the parser\n// via the composite ST01+GS08 lookup (edi/loop_engine.go), keyed off the\n// FILE's own real GS08, not any step config. A stray 837I file landing in\n// this professional-only mailbox would otherwise silently run through this\n// SV1-shaped derive logic and produce wrong/incomplete Claim resources with\n// no error. Guard explicitly instead.\nif (parsed.transactionSet !== \"837P\") {\n  return ({ _claim_contexts: [], _billing_provider: {} });\n}\n\nfunction arr(v) {\n  if (!v) return [];\n  return Array.isArray(v) ? v : [v];\n}\nfunction first(v) {\n  var a = arr(v);\n  return a.length > 0 ? a[0] : {};\n}\n\nfunction genderToFHIR(code) {\n  if (code === \"M\") return \"male\";\n  if (code === \"F\") return \"female\";\n  return \"unknown\";\n}\n\nfunction relationshipToFHIR(code) {\n  var map = { \"18\": \"self\", \"01\": \"spouse\", \"19\": \"child\", \"20\": \"employee\" };\n  return map[code] || \"other\";\n}\n\nvar billingLevel = first(loops[\"2000A\"]);\nvar billingProviderLoop = (billingLevel.loops || {})[\"2010AA\"] || {};\nvar billingNM1 = first(billingProviderLoop.NM1);\nvar billingProvider = {\n  npi: billingNM1.identificationCode || \"\",\n  name: billingNM1.nameLastOrOrganizationName || \"\"\n};\n\nfunction extractDiagnoses(hi) {\n  var codes = (hi && hi.codes) || [];\n  var out = [];\n  for (var i = 0; i < codes.length; i++) {\n    var c = (codes[i] && codes[i].code) || {};\n    if (!c.code) continue;\n    out.push({ code: c.code, sequence: i + 1 });\n  }\n  return out;\n}\n\nfunction diagnosisPointers(sv1) {\n  var ptr = (sv1 && sv1.diagnosisCodePointer) || {};\n  var out = [];\n  [\"pointer1\", \"pointer2\", \"pointer3\", \"pointer4\"].forEach(function (key) {\n    if (ptr[key]) out.push(parseInt(ptr[key], 10));\n  });\n  return out;\n}\n\nfunction extractServiceLines(claimLoops) {\n  var lines = arr((claimLoops || {})[\"2400\"]);\n  var out = [];\n  for (var i = 0; i < lines.length; i++) {\n    var line = lines[i];\n    var lx = line.LX || {};\n    var sv1 = line.SV1 || {};\n    var proc = sv1.procedureCode || {};\n    var dtpList = arr(line.DTP);\n    var servicedDate = \"\";\n    for (var d = 0; d < dtpList.length; d++) {\n      if (dtpList[d].dateTimeQualifier === \"472\") { servicedDate = dtpList[d].datePeriod; break; }\n    }\n    out.push({\n      sequence: parseInt(lx.assignedNumber || String(i + 1), 10),\n      procedureCode: proc.code || \"\",\n      procedureSystem: (proc.qualifier === \"HC\") ? \"http://www.ama-assn.org/go/cpt\" : \"\",\n      quantity: sv1.serviceUnitCount ? Number(sv1.serviceUnitCount) : 1,\n      net: sv1.lineItemChargeAmount ? Number(sv1.lineItemChargeAmount) : 0,\n      servicedDate: servicedDate,\n      diagnosisPointers: diagnosisPointers(sv1)\n    });\n  }\n  return out;\n}\n\nfunction extractCareTeam(claimLoops) {\n  var team = [];\n  var renderingLoop = (claimLoops || {})[\"2310B\"];\n  if (renderingLoop) {\n    var nm1 = first(renderingLoop.NM1);\n    if (nm1.identificationCode) {\n      team.push({ npi: nm1.identificationCode, role: \"rendering\", sequence: team.length + 1 });\n    }\n  }\n  var referringList = arr((claimLoops || {})[\"2310A\"]);\n  for (var i = 0; i < referringList.length; i++) {\n    var rnm1 = first(referringList[i].NM1);\n    if (rnm1.identificationCode) {\n      team.push({ npi: rnm1.identificationCode, role: \"referring\", sequence: team.length + 1 });\n    }\n  }\n  return team;\n}\n\n// Claim.created is required by the base FHIR R4 Claim resource but has no\n// X12 837 equivalent (837 carries no \"claim record created\" timestamp) --\n// the same \"record processing time, not sourced from the message\" default\n// convention BHT03/BHT04 (creation date/time) already establish for the\n// interchange itself. Computed once for the whole file, not per claim.\nvar nowISO = new Date().toISOString();\n\nfunction buildClaimContext(patientInfo, subscriberInfo, payerInfo, claimLoop) {\n  var clm = claimLoop.CLM || {};\n  var svcLoc = clm.healthCareServiceLocation || {};\n  return {\n    patientInfo: patientInfo,\n    subscriberInfo: subscriberInfo,\n    payerInfo: payerInfo,\n    claim: {\n      patientControlNumber: clm.patientControlNumber || \"\",\n      totalChargeAmount: clm.totalClaimChargeAmount ? Number(clm.totalClaimChargeAmount) : 0,\n      placeOfServiceCode: svcLoc.placeOfServiceCode || \"\",\n      createdAt: nowISO,\n      diagnosisList: extractDiagnoses(claimLoop.HI),\n      serviceLines: extractServiceLines(claimLoop.loops),\n      careTeam: extractCareTeam(claimLoop.loops)\n    }\n  };\n}\n\nfunction personFromNM1AndDMG(nm1, dmg, relationshipCode) {\n  var gender = dmg.genderCode || \"\";\n  return {\n    firstName: nm1.nameFirst || \"\",\n    lastName: nm1.nameLastOrOrganizationName || \"\",\n    memberId: nm1.identificationCode || \"\",\n    dob: dmg.birthDate || \"\",\n    genderFHIR: genderToFHIR(gender),\n    relationshipCode: relationshipCode || \"\",\n    relationshipFHIR: relationshipToFHIR(relationshipCode || \"\")\n  };\n}\n\nvar claimContexts = [];\n\nvar billingLoops = billingLevel.loops || {};\nvar subscriberLevels = arr(billingLoops[\"2000B\"]);\nfor (var s = 0; s < subscriberLevels.length; s++) {\n  var subLevel = subscriberLevels[s];\n  var subLoops = subLevel.loops || {};\n  var sbr = subLevel.SBR || {};\n\n  var subNM1 = first((subLoops[\"2010BA\"] || {}).NM1);\n  var subDMG = (subLoops[\"2010BA\"] || {}).DMG || {};\n  var subscriberInfo = personFromNM1AndDMG(subNM1, subDMG, \"18\");\n\n  var payerNM1 = first((subLoops[\"2010BB\"] || {}).NM1);\n  var payerInfo = {\n    name: payerNM1.nameLastOrOrganizationName || \"\",\n    payerId: payerNM1.identificationCode || \"\"\n  };\n\n  var dependentLevels = arr(subLoops[\"2000C\"]);\n  if (dependentLevels.length > 0) {\n    for (var d = 0; d < dependentLevels.length; d++) {\n      var depLevel = dependentLevels[d];\n      var depLoops = depLevel.loops || {};\n      var depNM1 = first((depLoops[\"2010CA\"] || {}).NM1);\n      var depDMG = (depLoops[\"2010CA\"] || {}).DMG || {};\n      var patientInfo = personFromNM1AndDMG(depNM1, depDMG, sbr.individualRelationshipCode);\n      if (!patientInfo.memberId) patientInfo.memberId = subscriberInfo.memberId + \"-DEP\" + (d + 1);\n\n      var depClaims = arr(depLoops[\"2300\"]);\n      for (var dc = 0; dc < depClaims.length; dc++) {\n        claimContexts.push(buildClaimContext(patientInfo, subscriberInfo, payerInfo, depClaims[dc]));\n      }\n    }\n  } else {\n    var subClaims = arr(subLoops[\"2300\"]);\n    for (var sc = 0; sc < subClaims.length; sc++) {\n      claimContexts.push(buildClaimContext(subscriberInfo, subscriberInfo, payerInfo, subClaims[sc]));\n    }\n  }\n}\n\nreturn ({\n  _claim_contexts: claimContexts,\n  _billing_provider: billingProvider\n});\n"
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
                            {"targetPath": "identifier[0].value", "sourcePath": "_billing_provider.npi"},
                            {"targetPath": "name", "sourcePath": "_billing_provider.name"}
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
                        "rowsPath": "_claim_contexts",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "patientInfo.memberId"},
                            {"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/x12-member-id"},
                            {"targetPath": "identifier[0].value", "sourcePath": "patientInfo.memberId"},
                            {"targetPath": "name[0].family", "sourcePath": "patientInfo.lastName"},
                            {"targetPath": "name[0].given[0]", "sourcePath": "patientInfo.firstName"},
                            {"targetPath": "birthDate", "sourcePath": "patientInfo.dob", "transform": "x12_date_to_fhir_date"},
                            {"targetPath": "gender", "sourcePath": "patientInfo.genderFHIR"}
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
                        "rowsPath": "_claim_contexts",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "claim.patientControlNumber", "transform": "string_prefix", "valueMap": {"prefix": "coverage-"}},
                            {"targetPath": "status", "literalValue": "active"},
                            {"targetPath": "beneficiary.reference", "sourcePath": "patientInfo.memberId", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}},
                            {"targetPath": "subscriberId", "sourcePath": "subscriberInfo.memberId"},
                            {"targetPath": "subscriber.reference", "sourcePath": "subscriberInfo.memberId", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}, "condition": {"field": "patientInfo.relationshipCode", "operator": "equals", "value": "18"}},
                            {"targetPath": "relationship.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/subscriber-relationship"},
                            {"targetPath": "relationship.coding[0].code", "sourcePath": "patientInfo.relationshipFHIR"},
                            {"targetPath": "payor[0].identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
                            {"targetPath": "payor[0].identifier.value", "sourcePath": "payerInfo.payerId"},
                            {"targetPath": "payor[0].display", "sourcePath": "payerInfo.name"}
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
                        "rowsPath": "_claim_contexts",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "claim.patientControlNumber", "transform": "string_prefix", "valueMap": {"prefix": "claim-"}},
                            {"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/x12-claim-control-number"},
                            {"targetPath": "identifier[0].value", "sourcePath": "claim.patientControlNumber"},
                            {"targetPath": "status", "literalValue": "active"},
                            {"targetPath": "type.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/claim-type"},
                            {"targetPath": "type.coding[0].code", "literalValue": "professional"},
                            {"targetPath": "use", "literalValue": "claim"},
                            {"targetPath": "created", "sourcePath": "claim.createdAt"},
                            {"targetPath": "priority.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/processpriority"},
                            {"targetPath": "priority.coding[0].code", "literalValue": "normal"},
                            {"targetPath": "patient.reference", "sourcePath": "patientInfo.memberId", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}},
                            {"targetPath": "provider.reference", "literalValue": "Organization/organization-billing"},
                            {"targetPath": "insurer.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
                            {"targetPath": "insurer.identifier.value", "sourcePath": "payerInfo.payerId"},
                            {"targetPath": "total.value", "sourcePath": "claim.totalChargeAmount"},
                            {"targetPath": "total.currency", "literalValue": "USD"},
                            {"targetPath": "insurance[0].sequence", "literalValue": "1"},
                            {"targetPath": "insurance[0].focal", "literalValue": "true"},
                            {"targetPath": "insurance[0].coverage.reference", "sourcePath": "claim.patientControlNumber", "transform": "string_prefix", "valueMap": {"prefix": "Coverage/coverage-"}}
                        ],
                        "repeatingGroups": [
                            {
                                "targetPath": "diagnosis",
                                "rowsPath": "claim.diagnosisList",
                                "fields": [
                                    {"targetPath": "diagnosisCodeableConcept.coding[0].system", "literalValue": "http://hl7.org/fhir/sid/icd-10-cm"},
                                    {"targetPath": "diagnosisCodeableConcept.coding[0].code", "sourcePath": "code"},
                                    {"targetPath": "sequence", "sourcePath": "sequence"}
                                ]
                            },
                            {
                                "targetPath": "item",
                                "rowsPath": "claim.serviceLines",
                                "fields": [
                                    {"targetPath": "sequence", "sourcePath": "sequence"},
                                    {"targetPath": "productOrService.coding[0].system", "sourcePath": "procedureSystem"},
                                    {"targetPath": "productOrService.coding[0].code", "sourcePath": "procedureCode"},
                                    {"targetPath": "quantity.value", "sourcePath": "quantity"},
                                    {"targetPath": "net.value", "sourcePath": "net"},
                                    {"targetPath": "net.currency", "literalValue": "USD"},
                                    {"targetPath": "servicedDate", "sourcePath": "servicedDate"}
                                ]
                            },
                            {
                                "targetPath": "careTeam",
                                "rowsPath": "claim.careTeam",
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
