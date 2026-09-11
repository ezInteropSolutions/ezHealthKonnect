-- V238: EDI X12 837I (Institutional Claim) to FHIR OOB pipeline template
-- Applied: 2026-09-11

-- ============================================================
-- EDI 837I -> FHIR (Claim + Patient + Coverage + Organization) TEMPLATE
-- ============================================================
-- The institutional counterpart to V237's own 837P-to-FHIR template — same
-- overall pipeline shape and the same fhir.build `rowsPath` mechanism for
-- multi-claim/multi-line-item mapping without a control.loop wrapper step
-- (see V237's own doc comment for the full rationale). A SEPARATE template
-- from V237, per the user's own explicit scope decision: professional and
-- institutional need genuinely different item[]/supportingInfo[] shapes
-- (SV2 not SV1, CL1 admission info, HI shared for diagnosis AND procedure
-- codes), not a single branching template.
--   seq 5    connector.inbound   (edi_x12_inbound  — polls an SFTP directory for 837I files)
--   seq 20   edi.parse           (raw X12 -> structured JSON)
--   seq 30   edi.validate        (base X12 5010 standard checks; errors block, warnings never do)
--   seq 95   enrichment.script   (Derive 837I Claim Contexts — pure data reshaping only)
--   seq 100  fhir.build          (Organization — billing provider, built once per file)
--   seq 105  fhir.build          (Patient — rowsPath mode, one per claim context)
--   seq 110  fhir.build          (Coverage — rowsPath mode, one per claim context)
--   seq 115  fhir.build          (Claim — rowsPath mode, one per claim context)
--   seq 200  payload.builder     (fhir_bundle mode — assembles the Bundle)
--   seq 210  fhir_validation     (strict — validates the assembled Bundle)
--   seq 295  connector.outbound  (sink_outbound — store-only terminal, no external I/O)
--
-- Institutional-specific mapping, confirmed directly against
-- edi/schemas/x12_005010's own loops/2300-claim-institutional.json and
-- segments/{CL1,SV2,HI}.json (not assumed from the professional shape):
--   - SV2 (not SV1) -> Claim.item[]: adds item.revenue from SV2's own
--     revenue code; no item-level diagnosisSequence pointers (institutional
--     service lines don't carry them, unlike professional's SV1).
--   - HI is SHARED for diagnosis AND procedure codes on 837I (unlike 837P,
--     where HI only ever carries diagnoses) — split purely by each
--     repetition's own qualifier: ABK/ABF/ABJ -> Claim.diagnosis[],
--     BBR/BR -> Claim.procedure[]. HI's own Present-on-Admission indicator
--     rides on diagnosis repetitions -> Claim.diagnosis[].onAdmission.
--   - CL1 (Institutional Claim Code — admission type/source, patient
--     status) has no professional equivalent -> Claim.supportingInfo[].
--   - Care team roles are genuinely different role assignments from
--     837P's own 2310A/2310B (Referring/Rendering): 2310A=Attending,
--     2310B=Operating Physician, 2310D=Rendering, 2310F=Referring.
--
-- Organization/Patient/Coverage configs are IDENTICAL in shape to V237's
-- own (same underlying claim-context row structure) — only Claim differs.
--
-- Config here is transcribed verbatim from services/edi_837i_fhir_builder_test.go
-- (TestEDI837IFHIRBuilder_MultiClaimMultiLine_BuildsCleanValidatingBundle),
-- which proves this exact chain against a real, multi-claim, multi-line-item
-- fixture (an inpatient claim with 2 service lines/2 diagnoses [one carrying
-- Present-on-Admission]/1 procedure/attending+rendering care team/3
-- supportingInfo entries, plus a dependent's own outpatient claim with 1
-- service line/1 diagnosis) via the real executors, asserting the resulting
-- Bundle validates at strict level with zero unexpected errors (the same
-- known, pre-existing, out-of-scope ClaimTypes ValueSet gap V237/PAS already
-- document is explicitly excluded).
--
-- Named simplifications — same as V237's own, plus:
--   - Value/occurrence/condition codes (X12 HI qualifiers BE/BH/BG) are not
--     mapped in this first pass — only diagnosis (AB*) and principal/other
--     procedure (BBR/BR) qualifiers are recognized.
--   - Claim.diagnosis[].onAdmission is only populated when HI's own C022
--     sub09 is actually present on that repetition (most diagnosis codes on
--     a real 837I won't carry one) — omitted, not defaulted, when absent.

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
    'EDI X12 837I to FHIR (SFTP)',
    'edi-837i-to-fhir-sftp',
    'Poll an SFTP directory for X12 837I (Institutional Claim) files, parse and validate them, then build and validate a FHIR R4 Bundle containing Organization, Patient, Coverage, and Claim resources — one Patient/Coverage/Claim per claim in the file, with institutional-specific mapping (revenue codes, admission info, principal/other procedure codes, Present-on-Admission indicators).',
    'edi',
    null,
    ARRAY['edi','x12','837i','institutional','claims','hospitals','sftp','fhir'],
    '🏨',
    'advanced',
    'edi_x12_inbound',
    '{"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["837I"]}',
    'sink_outbound',
    '{"enable_logging": true, "enable_validation": true}',
    $pipeline$
    {
        "execution_groups": [
            {
                "sequence": 5,
                "steps": [{
                    "step_name": "Receive 837I File (SFTP)",
                    "step_type": "connector.inbound",
                    "sequence": 5,
                    "config": {
                        "connectorType": "edi_x12_inbound",
                        "config": {"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["837I"]}
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 20,
                "steps": [{
                    "step_name": "Parse 837I -> JSON",
                    "step_type": "edi.parse",
                    "sequence": 20,
                    "config": {"sourceField": "raw", "outputField": "parsedEDI", "transactionSet": "837I"},
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
                    "step_name": "Derive 837I Claim Contexts",
                    "step_alias": "derive_837i_claim_contexts",
                    "step_type": "enrichment.script",
                    "sequence": 95,
                    "description": "Pure data reshaping only -- no FHIR resource-shape or profile knowledge. Flattens 837I's nested billing-provider/subscriber/dependent/claim/service-line/diagnosis/procedure structure into one flat array (_claim_contexts), splitting HI's shared diagnosis/procedure qualifiers, so the fhir.build steps below can build one Patient/Coverage/Claim resource per claim declaratively via rowsPath.",
                    "config": {
                        "script": "// -- Derive 837I Claim Contexts --\nvar parsed = input.parsedEDI || {};\nvar loops = parsed.loops || {};\n\nif (parsed.transactionSet !== \"837I\") {\n  return ({ _claim_contexts: [], _billing_provider: {} });\n}\n\nfunction arr(v) {\n  if (!v) return [];\n  return Array.isArray(v) ? v : [v];\n}\nfunction first(v) {\n  var a = arr(v);\n  return a.length > 0 ? a[0] : {};\n}\n\nfunction genderToFHIR(code) {\n  if (code === \"M\") return \"male\";\n  if (code === \"F\") return \"female\";\n  return \"unknown\";\n}\n\nfunction relationshipToFHIR(code) {\n  var map = { \"18\": \"self\", \"01\": \"spouse\", \"19\": \"child\", \"20\": \"employee\" };\n  return map[code] || \"other\";\n}\n\nvar billingLevel = first(loops[\"2000A\"]);\nvar billingProviderLoop = (billingLevel.loops || {})[\"2010AA\"] || {};\nvar billingNM1 = first(billingProviderLoop.NM1);\nvar billingProvider = {\n  npi: billingNM1.identificationCode || \"\",\n  name: billingNM1.nameLastOrOrganizationName || \"\"\n};\n\n// HI is SHARED for diagnosis and procedure codes on 837I, distinguished\n// purely by each repetition's own qualifier -- ABK/ABF/ABJ = diagnosis,\n// BBR/BR = procedure. Value/occurrence/condition codes (BE/BH/BG) are not\n// modeled in this pass (named simplification, matching 835's own \"core\n// fields, not exhaustive\" precedent).\nfunction extractDiagnoses(hi) {\n  var codes = (hi && hi.codes) || [];\n  var out = [];\n  var seq = 0;\n  for (var i = 0; i < codes.length; i++) {\n    var c = (codes[i] && codes[i].code) || {};\n    if (!c.code) continue;\n    var q = c.qualifier || \"\";\n    if (q.indexOf(\"AB\") !== 0) continue;\n    seq++;\n    var entry = { code: c.code, sequence: seq };\n    // Omit the key entirely (not an empty string) when absent -- an empty\n    // onAdmission.coding[0].code sourcePath still resolves as \"not empty\n    // data\" for a LITERAL system field with no sourcePath of its own, so the\n    // Claim config's own condition (checking this field \"exists\") only works\n    // correctly when the key is truly missing from the row, not blank.\n    if (c.presentOnAdmissionIndicator) { entry.onAdmission = c.presentOnAdmissionIndicator; }\n    out.push(entry);\n  }\n  return out;\n}\n\nfunction extractProcedures(hi) {\n  var codes = (hi && hi.codes) || [];\n  var out = [];\n  var seq = 0;\n  for (var i = 0; i < codes.length; i++) {\n    var c = (codes[i] && codes[i].code) || {};\n    if (!c.code) continue;\n    var q = c.qualifier || \"\";\n    if (q !== \"BBR\" && q !== \"BR\") continue;\n    seq++;\n    out.push({ code: c.code, sequence: seq });\n  }\n  return out;\n}\n\nfunction extractInstitutionalInfo(cl1) {\n  var out = [];\n  var seq = 0;\n  if (!cl1) return out;\n  if (cl1.admissionTypeCode) { seq++; out.push({ category: \"admissiontype\", code: cl1.admissionTypeCode, sequence: seq }); }\n  if (cl1.admissionSourceCode) { seq++; out.push({ category: \"admissionsource\", code: cl1.admissionSourceCode, sequence: seq }); }\n  if (cl1.patientStatusCode) { seq++; out.push({ category: \"patientstatus\", code: cl1.patientStatusCode, sequence: seq }); }\n  return out;\n}\n\nfunction extractServiceLines(claimLoops) {\n  var lines = arr((claimLoops || {})[\"2400\"]);\n  var out = [];\n  for (var i = 0; i < lines.length; i++) {\n    var line = lines[i];\n    var lx = line.LX || {};\n    var sv2 = line.SV2 || {};\n    var proc = sv2.procedureCode || {};\n    var dtpList = arr(line.DTP);\n    var servicedDate = \"\";\n    for (var d = 0; d < dtpList.length; d++) {\n      if (dtpList[d].dateTimeQualifier === \"472\") { servicedDate = dtpList[d].datePeriod; break; }\n    }\n    out.push({\n      sequence: parseInt(lx.assignedNumber || String(i + 1), 10),\n      revenueCode: sv2.serviceLineRevenueCode || \"\",\n      procedureCode: proc.code || \"\",\n      procedureSystem: (proc.qualifier === \"HC\") ? \"http://www.ama-assn.org/go/cpt\" : \"\",\n      quantity: sv2.serviceUnitCount ? Number(sv2.serviceUnitCount) : 1,\n      net: sv2.lineItemChargeAmount ? Number(sv2.lineItemChargeAmount) : 0,\n      servicedDate: servicedDate\n    });\n  }\n  return out;\n}\n\n// Institutional care-team roles are genuinely different assignments from\n// professional's own 2310A/2310B (Referring/Rendering) -- 2310A=Attending,\n// 2310B=Operating Physician, 2310D=Rendering, 2310F=Referring.\nfunction extractCareTeam(claimLoops) {\n  var team = [];\n  var roleLoops = [\n    { key: \"2310A\", role: \"attending\" },\n    { key: \"2310B\", role: \"operating\" },\n    { key: \"2310D\", role: \"rendering\" },\n    { key: \"2310F\", role: \"referring\" }\n  ];\n  for (var r = 0; r < roleLoops.length; r++) {\n    var loop = (claimLoops || {})[roleLoops[r].key];\n    if (!loop) continue;\n    var nm1 = first(loop.NM1);\n    if (nm1.identificationCode) {\n      team.push({ npi: nm1.identificationCode, role: roleLoops[r].role, sequence: team.length + 1 });\n    }\n  }\n  return team;\n}\n\nvar nowISO = new Date().toISOString();\n\nfunction buildClaimContext(patientInfo, subscriberInfo, payerInfo, claimLoop) {\n  var clm = claimLoop.CLM || {};\n  var svcLoc = clm.healthCareServiceLocation || {};\n  return {\n    patientInfo: patientInfo,\n    subscriberInfo: subscriberInfo,\n    payerInfo: payerInfo,\n    claim: {\n      patientControlNumber: clm.patientControlNumber || \"\",\n      totalChargeAmount: clm.totalClaimChargeAmount ? Number(clm.totalClaimChargeAmount) : 0,\n      placeOfServiceCode: svcLoc.placeOfServiceCode || \"\",\n      createdAt: nowISO,\n      diagnosisList: extractDiagnoses(claimLoop.HI),\n      procedureList: extractProcedures(claimLoop.HI),\n      institutionalInfo: extractInstitutionalInfo(claimLoop.CL1),\n      serviceLines: extractServiceLines(claimLoop.loops),\n      careTeam: extractCareTeam(claimLoop.loops)\n    }\n  };\n}\n\nfunction personFromNM1AndDMG(nm1, dmg, relationshipCode) {\n  var gender = dmg.genderCode || \"\";\n  return {\n    firstName: nm1.nameFirst || \"\",\n    lastName: nm1.nameLastOrOrganizationName || \"\",\n    memberId: nm1.identificationCode || \"\",\n    dob: dmg.birthDate || \"\",\n    genderFHIR: genderToFHIR(gender),\n    relationshipCode: relationshipCode || \"\",\n    relationshipFHIR: relationshipToFHIR(relationshipCode || \"\")\n  };\n}\n\nvar claimContexts = [];\n\nvar billingLoops = billingLevel.loops || {};\nvar subscriberLevels = arr(billingLoops[\"2000B\"]);\nfor (var s = 0; s < subscriberLevels.length; s++) {\n  var subLevel = subscriberLevels[s];\n  var subLoops = subLevel.loops || {};\n  var sbr = subLevel.SBR || {};\n\n  var subNM1 = first((subLoops[\"2010BA\"] || {}).NM1);\n  var subDMG = (subLoops[\"2010BA\"] || {}).DMG || {};\n  var subscriberInfo = personFromNM1AndDMG(subNM1, subDMG, \"18\");\n\n  var payerNM1 = first((subLoops[\"2010BB\"] || {}).NM1);\n  var payerInfo = {\n    name: payerNM1.nameLastOrOrganizationName || \"\",\n    payerId: payerNM1.identificationCode || \"\"\n  };\n\n  var dependentLevels = arr(subLoops[\"2000C\"]);\n  if (dependentLevels.length > 0) {\n    for (var d = 0; d < dependentLevels.length; d++) {\n      var depLevel = dependentLevels[d];\n      var depLoops = depLevel.loops || {};\n      var depNM1 = first((depLoops[\"2010CA\"] || {}).NM1);\n      var depDMG = (depLoops[\"2010CA\"] || {}).DMG || {};\n      var patientInfo = personFromNM1AndDMG(depNM1, depDMG, sbr.individualRelationshipCode);\n      if (!patientInfo.memberId) patientInfo.memberId = subscriberInfo.memberId + \"-DEP\" + (d + 1);\n\n      var depClaims = arr(depLoops[\"2300\"]);\n      for (var dc = 0; dc < depClaims.length; dc++) {\n        claimContexts.push(buildClaimContext(patientInfo, subscriberInfo, payerInfo, depClaims[dc]));\n      }\n    }\n  } else {\n    var subClaims = arr(subLoops[\"2300\"]);\n    for (var sc = 0; sc < subClaims.length; sc++) {\n      claimContexts.push(buildClaimContext(subscriberInfo, subscriberInfo, payerInfo, subClaims[sc]));\n    }\n  }\n}\n\nreturn ({\n  _claim_contexts: claimContexts,\n  _billing_provider: billingProvider\n});\n"
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
                    "description": "Builds one FHIR Patient resource per claim context (rowsPath mode) -- the patient is the subscriber when they are their own patient, or the dependent (2010CA) otherwise. Identical shape to V237's own Patient step.",
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
                    "description": "Builds one FHIR Coverage resource per claim context (rowsPath mode). Identical shape to V237's own Coverage step -- subscriber.reference only written when the patient IS the subscriber, avoiding a dangling reference in the dependent case.",
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
                    "description": "Builds one FHIR Claim resource per claim context (rowsPath mode). diagnosis[]/procedure[]/supportingInfo[]/item[]/careTeam[] built via repeatingGroups off the context's own pre-flattened arrays. onAdmission fields are conditional on the row actually carrying one. careTeam[].provider is a logical (identifier-only) reference. priority is fixed (no X12 837 equivalent).",
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
                            {"targetPath": "type.coding[0].code", "literalValue": "institutional"},
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
                                    {"targetPath": "sequence", "sourcePath": "sequence"},
                                    {"targetPath": "onAdmission.coding[0].system", "literalValue": "https://codesystem.x12.org/005010/1352", "condition": {"field": "onAdmission", "operator": "exists"}},
                                    {"targetPath": "onAdmission.coding[0].code", "sourcePath": "onAdmission"}
                                ]
                            },
                            {
                                "targetPath": "procedure",
                                "rowsPath": "claim.procedureList",
                                "fields": [
                                    {"targetPath": "procedureCodeableConcept.coding[0].system", "literalValue": "http://www.cms.gov/Medicare/Coding/ICD10"},
                                    {"targetPath": "procedureCodeableConcept.coding[0].code", "sourcePath": "code"},
                                    {"targetPath": "sequence", "sourcePath": "sequence"}
                                ]
                            },
                            {
                                "targetPath": "supportingInfo",
                                "rowsPath": "claim.institutionalInfo",
                                "fields": [
                                    {"targetPath": "sequence", "sourcePath": "sequence"},
                                    {"targetPath": "category.coding[0].code", "sourcePath": "category"},
                                    {"targetPath": "code.coding[0].code", "sourcePath": "code"}
                                ]
                            },
                            {
                                "targetPath": "item",
                                "rowsPath": "claim.serviceLines",
                                "fields": [
                                    {"targetPath": "sequence", "sourcePath": "sequence"},
                                    {"targetPath": "revenue.coding[0].system", "literalValue": "https://codesystem.x12.org/005010/234"},
                                    {"targetPath": "revenue.coding[0].code", "sourcePath": "revenueCode"},
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
                    "step_name": "Assemble 837I FHIR Bundle",
                    "step_alias": "assemble_837i_fhir_bundle",
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
    '837I',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_path","label":"Remote Directory","type":"string","hint":"Directory to poll for 837I files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive 837I (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse 837I","icon":"📄","step_type":"edi.parse"},
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
