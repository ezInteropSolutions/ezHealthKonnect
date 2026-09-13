-- V240: EDI X12 837I to FHIR -- multi-billing-provider + COB secondary-payer
-- support. Institutional counterpart to V239 -- same fix, same rationale
-- (see V239's own header comment for the full design). Updates the SAME
-- 'edi-837i-to-fhir-sftp' row V238 created (never edits V238 in place) via
-- the same idempotent INSERT ... ON CONFLICT (slug) DO UPDATE pattern.
--
-- Verified against services/edi_837i_fhir_builder_test.go
-- (TestEDI837IFHIRBuilder_MultiBillingProvider_BuildsDistinctOrganizationsPerProvider,
-- TestEDI837IFHIRBuilder_COBSecondaryPayer_BuildsSecondCoverageAndInsuranceEntry).
-- No real 837I COB sample exists in edi/testdata/real_samples/, so the COB
-- fix is proven at the synthetic-fixture level only on the institutional
-- side (the 2320/2330B loop logic itself is shared, byte-identical code with
-- 837P's own, which IS proven against a real X12.org sample).
--
-- Below this point, config is transcribed verbatim from
-- services/edi_837i_fhir_builder_test.go, same discipline as V238 itself.
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
--     productOrService is ALWAYS anchored on the revenue code (coding[0] --
--     the one identifier every real institutional line carries), with the
--     HCPCS/CPT code (when SV202 is actually present) added as an
--     ADDITIONAL coding[1], never a conditional either/or -- confirmed
--     directly against X12.org's own official "Jones Hospital" 837I example,
--     whose real service lines carry both a revenue code AND a CPT code
--     together on every line (user-corrected design, September 2026; see
--     edi_837i_fhir_builder_test.go's own header comment for the full
--     before/after).
--   - HI is SHARED for diagnosis AND procedure codes on 837I (unlike 837P,
--     where HI only ever carries diagnoses) — split purely by each
--     repetition's own qualifier: BK/ABK/BF/ABF/BJ/ABJ/BN/ABN/PR/APR (ICD-9/
--     ICD-10 pairs) -> Claim.diagnosis[], BR/BBR/BQ/BBQ -> Claim.procedure[].
--     HI's own Present-on-Admission indicator rides on diagnosis
--     repetitions -> Claim.diagnosis[].onAdmission.
--   - CL1 (Institutional Claim Code — admission type/source, patient
--     status) has no professional equivalent -> Claim.supportingInfo[].
--   - Care team roles are genuinely different role assignments from
--     837P's own 2310A/2310B (Referring/Rendering): 2310A=Attending,
--     2310B=Operating Physician, 2310D=Rendering, 2310F=Referring.
--
-- Organization/Patient/Coverage configs are IDENTICAL in shape to V239's
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
-- Named simplifications — same as V239's own, plus:
--   - Value/occurrence/condition codes (X12 HI qualifiers BE/BH/BG) and DRG
--     (DR) are not mapped in this first pass — only the diagnosis and
--     principal/other procedure qualifiers above are recognized.
--   - Claim.diagnosis[].onAdmission is only populated when HI's own C022
--     sub09 is actually present on that repetition (most diagnosis codes on
--     a real 837I won't carry one) — omitted, not defaulted, when absent.
--   - Claim.careTeam[] entries require the provider role's own NM1 to carry
--     an identificationCode (NPI) — a role loop present with no ID (e.g. a
--     pre-NPI-era claim identifying its attending physician by UPIN via a
--     REF segment instead, as X12.org's own "Jones Hospital" example does)
--     is silently skipped rather than correlated by name. A named,
--     out-of-scope limitation, not a crash risk.

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
                        "script": "\n// -- Derive 837I Claim Contexts --\n// See edi_837p_fhir_builder_test.go's own derive script comment for the full\n// \"message.\" nesting rationale -- same fix applies here.\nvar parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};\nvar loops = parsed.loops || {};\n\nif (parsed.transactionSet !== \"837I\") {\n  return ({ _claim_contexts: [], _coverage_contexts: [], _billing_providers: [] });\n}\n\nfunction arr(v) {\n  if (!v) return [];\n  return Array.isArray(v) ? v : [v];\n}\nfunction first(v) {\n  var a = arr(v);\n  return a.length > 0 ? a[0] : {};\n}\n\nfunction genderToFHIR(code) {\n  if (code === \"M\") return \"male\";\n  if (code === \"F\") return \"female\";\n  return \"unknown\";\n}\n\nfunction relationshipToFHIR(code) {\n  var map = { \"18\": \"self\", \"01\": \"spouse\", \"19\": \"child\", \"20\": \"employee\" };\n  return map[code] || \"other\";\n}\n\n// See edi_837p_fhir_builder_test.go's own derive script comment for why every\n// 2000A is walked instead of just the first, and for the billingProviderId\n// scheme.\nfunction billingProviderId(npi, idx) {\n  return \"organization-billing-\" + (npi || String(idx + 1));\n}\nvar billingProviderLevels = arr(loops[\"2000A\"]);\nvar billingProviders = [];\nfor (var bpIdx = 0; bpIdx < billingProviderLevels.length; bpIdx++) {\n  var bpProviderLoop = (billingProviderLevels[bpIdx].loops || {})[\"2010AA\"] || {};\n  var bpNM1 = first(bpProviderLoop.NM1);\n  var bpNpi = bpNM1.identificationCode || \"\";\n  billingProviders.push({\n    id: billingProviderId(bpNpi, bpIdx),\n    npi: bpNpi,\n    name: bpNM1.nameLastOrOrganizationName || \"\"\n  });\n}\n\n// HI is SHARED for diagnosis and procedure codes on 837I, distinguished\n// purely by each repetition's own qualifier. Diagnosis qualifiers come in\n// ICD-9/ICD-10 pairs (BK/ABK = principal, BF/ABF = other, BJ/ABJ =\n// admitting, BN/ABN = external cause of injury, PR/APR = patient's reason\n// for visit) -- an X12.org real-world sample (the \"Jones Hospital\" 005010X223\n// example, ICD-9-CM era) caught a real gap here: an earlier version of\n// extractDiagnoses only matched the \"AB\"-prefixed ICD-10 forms, silently\n// dropping every ICD-9 diagnosis (BK/BF/etc). Procedure qualifiers are\n// BR/BBR (principal) and BQ/BBQ (other). Value/occurrence/condition codes\n// (BE/BH/BG) and DRG (DR) are not modeled in this pass (named\n// simplification, matching 835's own \"core fields, not exhaustive\"\n// precedent) -- excluded automatically since they're not in either\n// whitelist below.\nvar DIAGNOSIS_HI_QUALIFIERS = [\"BK\", \"ABK\", \"BF\", \"ABF\", \"BJ\", \"ABJ\", \"BN\", \"ABN\", \"PR\", \"APR\"];\nvar PROCEDURE_HI_QUALIFIERS = [\"BR\", \"BBR\", \"BQ\", \"BBQ\"];\n// HI is maxUse \">1\" -- a claim can carry SEVERAL separate HI segment\n// occurrences at the same 2300 level (one per qualifier-group; see\n// edi/schemas/x12_005010/segments/HI.json's own maxUse-correction note for\n// why, found via testing against real, unedited samples), each with its own\n// up-to-12-entry codes[] repeat group. Flatten every occurrence's codes[]\n// into one combined list before filtering by qualifier.\nfunction allHICodes(hiList) {\n  var instances = arr(hiList);\n  var out = [];\n  for (var i = 0; i < instances.length; i++) {\n    var codes = (instances[i] && instances[i].codes) || [];\n    for (var j = 0; j < codes.length; j++) out.push(codes[j]);\n  }\n  return out;\n}\n\nfunction extractDiagnoses(codes) {\n  var out = [];\n  for (var i = 0; i < codes.length; i++) {\n    var c = (codes[i] && codes[i].code) || {};\n    if (!c.code) continue;\n    var q = c.qualifier || \"\";\n    if (DIAGNOSIS_HI_QUALIFIERS.indexOf(q) === -1) continue;\n    var entry = { code: c.code, sequence: out.length + 1 };\n    // Omit the key entirely (not an empty string) when absent -- an empty\n    // onAdmission.coding[0].code sourcePath still resolves as \"not empty\n    // data\" for a LITERAL system field with no sourcePath of its own, so the\n    // Claim config's own condition (checking this field \"exists\") only works\n    // correctly when the key is truly missing from the row, not blank.\n    if (c.presentOnAdmissionIndicator) { entry.on_admission = c.presentOnAdmissionIndicator; }\n    out.push(entry);\n  }\n  return out;\n}\n\nfunction extractProcedures(codes) {\n  var out = [];\n  for (var i = 0; i < codes.length; i++) {\n    var c = (codes[i] && codes[i].code) || {};\n    if (!c.code) continue;\n    var q = c.qualifier || \"\";\n    if (PROCEDURE_HI_QUALIFIERS.indexOf(q) === -1) continue;\n    out.push({ code: c.code, sequence: out.length + 1 });\n  }\n  return out;\n}\n\nfunction extractInstitutionalInfo(cl1) {\n  var out = [];\n  var seq = 0;\n  if (!cl1) return out;\n  if (cl1.admissionTypeCode) { seq++; out.push({ category: \"admissiontype\", code: cl1.admissionTypeCode, sequence: seq }); }\n  if (cl1.admissionSourceCode) { seq++; out.push({ category: \"admissionsource\", code: cl1.admissionSourceCode, sequence: seq }); }\n  if (cl1.patientStatusCode) { seq++; out.push({ category: \"patientstatus\", code: cl1.patientStatusCode, sequence: seq }); }\n  return out;\n}\n\nfunction extractServiceLines(claimLoops) {\n  var lines = arr((claimLoops || {})[\"2400\"]);\n  var out = [];\n  for (var i = 0; i < lines.length; i++) {\n    var line = lines[i];\n    var lx = line.LX || {};\n    var sv2 = line.SV2 || {};\n    var proc = sv2.procedureCode || {};\n    var dtpList = arr(line.DTP);\n    var svc = resolveServicedDate(dtpList);\n    var entry = {\n      sequence: parseInt(lx.assignedNumber || String(i + 1), 10),\n      revenue_code: sv2.serviceLineRevenueCode || \"\",\n      quantity: sv2.serviceUnitCount ? Number(sv2.serviceUnitCount) : 1,\n      net: sv2.lineItemChargeAmount ? Number(sv2.lineItemChargeAmount) : 0,\n      serviced_date: svc.servicedDate,\n      serviced_start: svc.servicedStart,\n      serviced_end: svc.servicedEnd\n    };\n    // Claim.item.productOrService is ALWAYS anchored on the revenue code\n    // (Claim config's own productOrService.coding[0], built directly off\n    // revenueCode) -- the one identifier EVERY real institutional line\n    // carries, and the field an official X12.org example (005010X223A2\n    // Example 1a) shows populated on every line even when a procedure code\n    // is ALSO present. SV202 (procedure code) is genuinely OPTIONAL on real\n    // institutional claims -- present on many ancillary/outpatient lines\n    // (often required under CMS OPPS rules for specific revenue codes),\n    // absent on others (room & board, some inpatient DRG-based lines; a\n    // real, unedited sample checked this round had NO procedure code on any\n    // of its 9 lines). When present, it's added as a SECOND coding in the\n    // SAME productOrService CodeableConcept (Claim config's own\n    // productOrService.coding[1]) -- never a replacement for the revenue-\n    // code coding. Keys omitted entirely (not set to \"\") when absent,\n    // matching the same \"a condition checking a field 'exists' must see a\n    // truly missing key, not an empty string\" convention onAdmission\n    // already established.\n    if (proc.code) {\n      entry.procedure_code = proc.code;\n      entry.procedure_system = (proc.qualifier === \"HC\") ? \"http://www.ama-assn.org/go/cpt\" : \"\";\n    }\n    out.push(entry);\n  }\n  return out;\n}\n\n// DTP02 (Date/Time Period Format Qualifier) \"D8\" is a single CCYYMMDD date;\n// \"RD8\" is a CCYYMMDD-CCYYMMDD range. Claim.item.serviced[x] is a choice type\n// (servicedDate | servicedPeriod) -- only one of the two shapes below is ever\n// populated per DTP*472 occurrence, matching that choice. Found only by\n// testing against a real, unedited 837 sample (databricks-industry-\n// solutions/x12-edi-parser's own CC_837P_EDI.txt/CC_837I_EDI.txt test\n// fixtures) carrying genuine RD8 ranges -- the synthetic Go-test fixture\n// only ever used D8 dates, so this gap was invisible there.\nfunction resolveServicedDate(dtpList) {\n  for (var d = 0; d < dtpList.length; d++) {\n    if (dtpList[d].dateTimeQualifier !== \"472\") continue;\n    var period = dtpList[d].datePeriod || \"\";\n    if (dtpList[d].dateTimePeriodFormatQualifier === \"RD8\" && period.indexOf(\"-\") !== -1) {\n      var parts = period.split(\"-\");\n      return { servicedDate: \"\", servicedStart: parts[0] || \"\", servicedEnd: parts[1] || \"\" };\n    }\n    return { servicedDate: period, servicedStart: \"\", servicedEnd: \"\" };\n  }\n  return { servicedDate: \"\", servicedStart: \"\", servicedEnd: \"\" };\n}\n\n// Institutional care-team roles are genuinely different assignments from\n// professional's own 2310A/2310B (Referring/Rendering) -- 2310A=Attending,\n// 2310B=Operating Physician, 2310D=Rendering, 2310F=Referring.\nfunction extractCareTeam(claimLoops) {\n  var team = [];\n  var roleLoops = [\n    { key: \"2310A\", role: \"attending\" },\n    { key: \"2310B\", role: \"operating\" },\n    { key: \"2310D\", role: \"rendering\" },\n    { key: \"2310F\", role: \"referring\" }\n  ];\n  for (var r = 0; r < roleLoops.length; r++) {\n    var loop = (claimLoops || {})[roleLoops[r].key];\n    if (!loop) continue;\n    var nm1 = first(loop.NM1);\n    if (nm1.identificationCode) {\n      team.push({ npi: nm1.identificationCode, role: roleLoops[r].role, sequence: team.length + 1 });\n    }\n  }\n  return team;\n}\n\n// Claim.facility (base FHIR, 0..1 Reference) -- 2310E Service Facility\n// Location on 837I (a genuinely different loop position from 837P's own\n// 2310C, since institutional's role numbering differs -- see this file's own\n// header comment). Same logical (identifier-only) reference pattern as\n// careTeam[].provider; not a care-team member. See\n// edi_837p_fhir_builder_test.go's own extractFacility for the full rationale\n// (found via X12.org's official COB example on the professional side).\nfunction extractFacility(claimLoops) {\n  var loop = (claimLoops || {})[\"2310E\"];\n  if (!loop) return {};\n  var nm1 = first(loop.NM1);\n  if (!nm1.identificationCode) return {};\n  return { facility_npi: nm1.identificationCode, facility_name: nm1.nameLastOrOrganizationName || \"\" };\n}\n\n// See edi_837p_fhir_builder_test.go's own extractOtherPayers/\n// buildInsuranceAndCoverage for the full COB rationale -- 2320/2330B is\n// identical shape across 837P/837I.\nfunction extractOtherPayers(claimLoops) {\n  var out = [];\n  var list = arr((claimLoops || {})[\"2320\"]);\n  for (var i = 0; i < list.length; i++) {\n    var payerNM1 = first(((list[i].loops || {})[\"2330B\"] || {}).NM1);\n    if (!payerNM1.identificationCode) continue;\n    out.push({ payer_id: payerNM1.identificationCode, payer_name: payerNM1.nameLastOrOrganizationName || \"\" });\n  }\n  return out;\n}\n\nfunction buildInsuranceAndCoverage(payerInfo, patientInfo, subscriberInfo, claimLoops, pcn) {\n  var baseId = \"coverage-\" + pcn;\n  var insuranceList = [{ sequence: 1, focal: true, coverage_id: baseId }];\n  var coverageRows = [{\n    patient_info: patientInfo, subscriber_info: subscriberInfo, payer_info: payerInfo,\n    coverage_id: baseId, sequence: 1, focal: true\n  }];\n  var others = extractOtherPayers(claimLoops);\n  for (var i = 0; i < others.length; i++) {\n    var seq = i + 2;\n    var cid = baseId + \"-\" + seq;\n    insuranceList.push({ sequence: seq, focal: false, coverage_id: cid });\n    coverageRows.push({\n      patient_info: patientInfo, subscriber_info: subscriberInfo,\n      payer_info: { name: others[i].payer_name, payer_id: others[i].payer_id },\n      coverage_id: cid, sequence: seq, focal: false\n    });\n  }\n  return { insurance_list: insuranceList, coverage_rows: coverageRows };\n}\n\nvar nowISO = new Date().toISOString();\n\n// See edi_837p_fhir_builder_test.go's own buildClaimContext doc comment for\n// why every returned property name here is snake_case, not camelCase.\nfunction buildClaimContext(patientInfo, subscriberInfo, payerInfo, claimLoop, billingProviderId) {\n  var clm = claimLoop.CLM || {};\n  var svcLoc = clm.healthCareServiceLocation || {};\n  var facility = extractFacility(claimLoop.loops);\n  var pcn = clm.patientControlNumber || \"\";\n  var insCov = buildInsuranceAndCoverage(payerInfo, patientInfo, subscriberInfo, claimLoop.loops, pcn);\n  var claim = {\n    patient_control_number: pcn,\n    total_charge_amount: clm.totalClaimChargeAmount ? Number(clm.totalClaimChargeAmount) : 0,\n    place_of_service_code: svcLoc.placeOfServiceCode || \"\",\n    created_at: nowISO,\n    billing_provider_id: billingProviderId,\n    diagnosis_list: extractDiagnoses(allHICodes(claimLoop.HI)),\n    procedure_list: extractProcedures(allHICodes(claimLoop.HI)),\n    institutional_info: extractInstitutionalInfo(claimLoop.CL1),\n    service_lines: extractServiceLines(claimLoop.loops),\n    care_team: extractCareTeam(claimLoop.loops),\n    insurance_list: insCov.insurance_list\n  };\n  if (facility.facility_npi) {\n    claim.facility_npi = facility.facility_npi;\n    claim.facility_name = facility.facility_name;\n  }\n  return {\n    context: {\n      patient_info: patientInfo,\n      subscriber_info: subscriberInfo,\n      payer_info: payerInfo,\n      claim: claim\n    },\n    coverage_rows: insCov.coverage_rows\n  };\n}\n\nfunction personFromNM1AndDMG(nm1, dmg, relationshipCode) {\n  var gender = dmg.genderCode || \"\";\n  return {\n    first_name: nm1.nameFirst || \"\",\n    last_name: nm1.nameLastOrOrganizationName || \"\",\n    member_id: nm1.identificationCode || \"\",\n    dob: dmg.birthDate || \"\",\n    gender_fhir: genderToFHIR(gender),\n    relationship_code: relationshipCode || \"\",\n    relationship_fhir: relationshipToFHIR(relationshipCode || \"\")\n  };\n}\n\nvar claimContexts = [];\nvar coverageContexts = [];\n\nfunction pushClaimContext(built) {\n  claimContexts.push(built.context);\n  for (var cr = 0; cr < built.coverage_rows.length; cr++) coverageContexts.push(built.coverage_rows[cr]);\n}\n\nfor (var bp = 0; bp < billingProviderLevels.length; bp++) {\n  var bpId = billingProviders[bp].id;\n  var billingLoops = billingProviderLevels[bp].loops || {};\n  var subscriberLevels = arr(billingLoops[\"2000B\"]);\n  for (var s = 0; s < subscriberLevels.length; s++) {\n    var subLevel = subscriberLevels[s];\n    var subLoops = subLevel.loops || {};\n    var sbr = subLevel.SBR || {};\n\n    var subNM1 = first((subLoops[\"2010BA\"] || {}).NM1);\n    var subDMG = (subLoops[\"2010BA\"] || {}).DMG || {};\n    var subscriberInfo = personFromNM1AndDMG(subNM1, subDMG, \"18\");\n\n    var payerNM1 = first((subLoops[\"2010BB\"] || {}).NM1);\n    var payerInfo = {\n      name: payerNM1.nameLastOrOrganizationName || \"\",\n      payer_id: payerNM1.identificationCode || \"\"\n    };\n\n    var dependentLevels = arr(subLoops[\"2000C\"]);\n    if (dependentLevels.length > 0) {\n      for (var d = 0; d < dependentLevels.length; d++) {\n        var depLevel = dependentLevels[d];\n        var depLoops = depLevel.loops || {};\n        var depNM1 = first((depLoops[\"2010CA\"] || {}).NM1);\n        var depDMG = (depLoops[\"2010CA\"] || {}).DMG || {};\n        var depPAT = depLevel.PAT || {};\n        // See edi_837p_fhir_builder_test.go's own dependent-relationship comment\n        // -- same real-world PAT01-vs-SBR02 correction applies here (the 2000C\n        // loop and PAT segment are identical shape across 837P/837I).\n        var patientInfo = personFromNM1AndDMG(depNM1, depDMG, depPAT.individualRelationshipCode || sbr.individualRelationshipCode);\n        if (!patientInfo.member_id) patientInfo.member_id = subscriberInfo.member_id + \"-DEP\" + (d + 1);\n\n        var depClaims = arr(depLoops[\"2300\"]);\n        for (var dc = 0; dc < depClaims.length; dc++) {\n          pushClaimContext(buildClaimContext(patientInfo, subscriberInfo, payerInfo, depClaims[dc], bpId));\n        }\n      }\n    } else {\n      var subClaims = arr(subLoops[\"2300\"]);\n      for (var sc = 0; sc < subClaims.length; sc++) {\n        pushClaimContext(buildClaimContext(subscriberInfo, subscriberInfo, payerInfo, subClaims[sc], bpId));\n      }\n    }\n  }\n}\n\nreturn ({\n  _claim_contexts: claimContexts,\n  _coverage_contexts: coverageContexts,\n  _billing_providers: billingProviders\n});\n"
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
                    "description": "Builds one FHIR Organization resource per distinct 2000A billing provider found in the file (rowsPath mode) -- id is NPI-anchored (organization-billing-<npi>), referenced by each claim's own Claim.provider via its billing_provider_id.",
                    "config": {
                        "resourceType": "Organization",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirOrganizations",
                        "rowsPath": "steps.derive_837i_claim_contexts.step_output._billing_providers",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "id"},
                            {"targetPath": "active", "literalValue": "true"},
                            {"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
                            {"targetPath": "identifier[0].value", "sourcePath": "npi"},
                            {"targetPath": "name", "sourcePath": "name"}
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
                        "rowsPath": "steps.derive_837i_claim_contexts.step_output._claim_contexts",
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
                    "description": "Builds one FHIR Coverage resource per claim x payer pair (rowsPath mode) -- a claim with a COB secondary payer gets its own second Coverage. coverage_id is pre-computed per row by the derive script. Identical shape to V239's own Coverage step -- subscriber.reference only written when the patient IS the subscriber, avoiding a dangling reference in the dependent case.",
                    "config": {
                        "resourceType": "Coverage",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirCoverages",
                        "rowsPath": "steps.derive_837i_claim_contexts.step_output._coverage_contexts",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "coverage_id"},
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
                    "description": "Builds one FHIR Claim resource per claim context (rowsPath mode). diagnosis[]/procedure[]/supportingInfo[]/item[]/careTeam[] built via repeatingGroups off the context's own pre-flattened arrays. onAdmission fields are conditional on the row actually carrying one. careTeam[].provider is a logical (identifier-only) reference. priority is fixed (no X12 837 equivalent).",
                    "config": {
                        "resourceType": "Claim",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirClaims",
                        "rowsPath": "steps.derive_837i_claim_contexts.step_output._claim_contexts",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "claim.patient_control_number", "transform": "string_prefix", "valueMap": {"prefix": "claim-"}},
                            {"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/x12-claim-control-number"},
                            {"targetPath": "identifier[0].value", "sourcePath": "claim.patient_control_number"},
                            {"targetPath": "status", "literalValue": "active"},
                            {"targetPath": "type.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/claim-type"},
                            {"targetPath": "type.coding[0].code", "literalValue": "institutional"},
                            {"targetPath": "use", "literalValue": "claim"},
                            {"targetPath": "created", "sourcePath": "claim.created_at"},
                            {"targetPath": "priority.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/processpriority"},
                            {"targetPath": "priority.coding[0].code", "literalValue": "normal"},
                            {"targetPath": "patient.reference", "sourcePath": "patient_info.member_id", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}},
                            {"targetPath": "provider.reference", "sourcePath": "claim.billing_provider_id", "transform": "string_prefix", "valueMap": {"prefix": "Organization/"}},
                            {"targetPath": "insurer.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
                            {"targetPath": "insurer.identifier.value", "sourcePath": "payer_info.payer_id"},
                            {"targetPath": "facility.identifier.system", "literalValue": "http://hl7.org/fhir/sid/us-npi", "condition": {"field": "claim.facility_npi", "operator": "exists"}},
                            {"targetPath": "facility.identifier.value", "sourcePath": "claim.facility_npi"},
                            {"targetPath": "total.value", "sourcePath": "claim.total_charge_amount"},
                            {"targetPath": "total.currency", "literalValue": "USD"}
                        ],
                        "repeatingGroups": [
                            {
                                "targetPath": "diagnosis",
                                "rowsPath": "claim.diagnosis_list",
                                "fields": [
                                    {"targetPath": "diagnosisCodeableConcept.coding[0].system", "literalValue": "http://hl7.org/fhir/sid/icd-10-cm"},
                                    {"targetPath": "diagnosisCodeableConcept.coding[0].code", "sourcePath": "code"},
                                    {"targetPath": "sequence", "sourcePath": "sequence"},
                                    {"targetPath": "onAdmission.coding[0].system", "literalValue": "https://codesystem.x12.org/005010/1352", "condition": {"field": "on_admission", "operator": "exists"}},
                                    {"targetPath": "onAdmission.coding[0].code", "sourcePath": "on_admission"}
                                ]
                            },
                            {
                                "targetPath": "procedure",
                                "rowsPath": "claim.procedure_list",
                                "fields": [
                                    {"targetPath": "procedureCodeableConcept.coding[0].system", "literalValue": "http://www.cms.gov/Medicare/Coding/ICD10"},
                                    {"targetPath": "procedureCodeableConcept.coding[0].code", "sourcePath": "code"},
                                    {"targetPath": "sequence", "sourcePath": "sequence"}
                                ]
                            },
                            {
                                "targetPath": "supportingInfo",
                                "rowsPath": "claim.institutional_info",
                                "fields": [
                                    {"targetPath": "sequence", "sourcePath": "sequence"},
                                    {"targetPath": "category.coding[0].code", "sourcePath": "category"},
                                    {"targetPath": "code.coding[0].code", "sourcePath": "code"}
                                ]
                            },
                            {
                                "targetPath": "item",
                                "rowsPath": "claim.service_lines",
                                "fields": [
                                    {"targetPath": "sequence", "sourcePath": "sequence"},
                                    {"targetPath": "revenue.coding[0].system", "literalValue": "https://codesystem.x12.org/005010/234"},
                                    {"targetPath": "revenue.coding[0].code", "sourcePath": "revenue_code"},
                                    {"targetPath": "productOrService.coding[0].system", "literalValue": "https://codesystem.x12.org/005010/234"},
                                    {"targetPath": "productOrService.coding[0].code", "sourcePath": "revenue_code"},
                                    {"targetPath": "productOrService.coding[1].system", "sourcePath": "procedure_system"},
                                    {"targetPath": "productOrService.coding[1].code", "sourcePath": "procedure_code"},
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
                            },
                            {
                                "targetPath": "insurance",
                                "rowsPath": "claim.insurance_list",
                                "fields": [
                                    {"targetPath": "sequence", "sourcePath": "sequence"},
                                    {"targetPath": "focal", "sourcePath": "focal"},
                                    {"targetPath": "coverage.reference", "sourcePath": "coverage_id", "transform": "string_prefix", "valueMap": {"prefix": "Coverage/"}}
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
                    "description": "Assembles the Organization/Patient/Coverage/Claim arrays (one Organization per billing provider, one Patient/Claim per claim context, one Coverage per claim x payer pair) into one FHIR R4 Bundle with correctly cross-referenced fullUrls.",
                    "config": {
                        "mode": "fhir_bundle",
                        "fhirBundle": {
                            "bundleType": "collection",
                            "resourcePaths": ["message.fhirOrganizations", "message.fhirPatients", "message.fhirCoverages", "message.fhirClaims"]
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
