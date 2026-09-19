-- V252: EDI X12 277 (Claim Status Response) to FHIR OOB pipeline template
-- Applied: 2026-09-14

-- ============================================================
-- EDI 277 -> FHIR (Task) TEMPLATE
-- ============================================================
-- EDI X12 Phase 6 companion to V251 (276 claim status request) -- see that
-- migration's own header comment for the full "transform-only, never
-- decides facts" scope rationale. A 277 carries a REAL status the payer
-- already decided (via its own STC segment) -- this pipeline only
-- re-expresses that status in FHIR's own Task.status vocabulary, the same
-- class of code-system translation 270/271 already established for
-- gender/relationship codes; it never invents or upgrades a status the
-- source content didn't report.
--   seq 5    connector.inbound   (edi_x12_inbound  -- polls an SFTP directory for 277 files)
--   seq 20   edi.parse           (raw X12 -> structured JSON)
--   seq 30   edi.validate        (base X12 5010 standard checks; errors block, warnings never do)
--   seq 95   enrichment.script   (Derive 277 Claim Status Responses -- pure data reshaping + STC->Task.status translation)
--   seq 100  fhir.build          (Organization -- the payer, rowsPath mode, one per context)
--   seq 110  fhir.build          (Patient -- rowsPath mode, one per reported claim status)
--   seq 115  fhir.build          (Task -- rowsPath mode, one per reported claim status)
--   seq 200  payload.builder     (fhir_bundle mode -- assembles the Bundle)
--   seq 210  fhir_validation     (strict -- validates the assembled Bundle)
--   seq 295  connector.outbound  (sink_outbound -- store-only terminal, no external I/O)
--
-- Config here is transcribed programmatically (via a Node script reading the
-- Go source directly, avoiding the hand-retyping bug class an earlier EDI
-- round hit once) from services/edi_277_fhir_builder_test.go
-- (TestEDI277FHIRBuilder_FinalizedAndPendingStatus_BuildsCleanValidatingBundle),
-- which proves this exact chain against a real fixture (a subscriber's own
-- claim coming back FINALIZED/PAID -- STC category F1 -- and a SEPARATE
-- dependent's own claim coming back PENDING -- STC category A2) via the
-- real executors, proving the STC->Task.status translation picks genuinely
-- different real statuses from genuinely different source data, and asserts
-- the resulting Bundle validates at strict level with zero unexpected
-- errors.
--
-- FHIR target: Task (see V251's own header comment for why Task, not
-- ClaimResponse, is the right mapping target here).
--
-- Named simplifications (same discipline as every prior EDI-to-FHIR round):
--   - Task.for/owner are populated as logical (identifier-only) references,
--     same precedent as V251.
--   - The STC category code is carried verbatim as Task.businessStatus
--     (system = the X12 Health Care Claim Status Category Codes code
--     system) alongside the translated Task.status -- both the raw source
--     code and its FHIR-vocabulary translation are preserved, never just
--     one or the other.
--   - 277's own service-line-level STC detail (2220D/2220E) is not modeled
--     into the Task -- only the claim-level overall status (STC's own
--     maxUse=">1" means the FIRST occurrence, the claim-level summary, is
--     used) -- matching V251's own claim-level-only scope.

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
    'EDI X12 277 to FHIR (SFTP)',
    'edi-277-to-fhir-sftp',
    'Poll an SFTP directory for X12 277 (Health Care Claim Status Response) files, parse and validate them, then build and validate a FHIR R4 Bundle containing Organization (payer), Patient, and Task resources -- one Patient/Task per reported claim status in the file (subscriber''s own claim or a dependent''s own claim), with the X12 STC status translated into FHIR''s own Task.status vocabulary.',
    'edi',
    null,
    ARRAY['edi','x12','277','claim-status','response','sftp','fhir'],
    '📋',
    'advanced',
    'edi_x12_inbound',
    '{"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["277"]}',
    'sink_outbound',
    '{"enable_logging": true, "enable_validation": true}',
    $pipeline$
{"execution_groups":[{"sequence":5,"steps":[{"step_name":"Receive 277 File (SFTP)","step_type":"connector.inbound","sequence":5,"config":{"connectorType":"edi_x12_inbound","config":{"transport":"sftp","remote_path":"/incoming","file_pattern":"*.edi","polling_interval_seconds":300,"after_processing":"archive","transaction_types":["277"]}},"enabled":true,"required":true}]},{"sequence":20,"steps":[{"step_name":"Parse 277 -> JSON","step_type":"edi.parse","sequence":20,"config":{"sourceField":"raw","outputField":"parsedEDI","transactionSet":"277"},"enabled":true,"required":true}]},{"sequence":30,"steps":[{"step_name":"Validate Against X12 5010","step_type":"edi.validate","sequence":30,"config":{"sourceField":"raw","outputField":"ediValidation","customRules":[]},"enabled":true,"required":false}]},{"sequence":95,"steps":[{"step_name":"Derive 277 Claim Status Responses","step_alias":"derive_277_claim_status_responses","step_type":"enrichment.script","sequence":95,"description":"Pure structural reshaping only -- this engine transforms X12 <-> FHIR, it never decides claim status facts. Flattens 277's payer/clearinghouse/provider/subscriber/dependent structure into one flat array (_claim_status_responses), one row per reported claim status, translating the X12 STC category code into FHIR's own Task.status vocabulary along the way (a code-system translation, same class as 270/271's own gender/relationship mapping) so the fhir.build steps below can build one Task per response declaratively via rowsPath.","config":{"script":"\nvar parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};\nvar loops = parsed.loops || {};\n\nif (parsed.transactionSet !== \"277\") {\n  return ({ _claim_status_responses: [] });\n}\n\nfunction arr(v) {\n  if (!v) return [];\n  return Array.isArray(v) ? v : [v];\n}\nfunction first(v) {\n  var a = arr(v);\n  return a.length > 0 ? a[0] : {};\n}\nfunction genderToFHIR(code) {\n  if (code === \"M\") return \"male\";\n  if (code === \"F\") return \"female\";\n  return \"unknown\";\n}\nfunction stcCategoryToTaskStatus(code) {\n  var map = {\n    \"A1\": \"received\", \"A2\": \"accepted\", \"A3\": \"rejected\", \"A4\": \"rejected\",\n    \"A6\": \"rejected\", \"A7\": \"rejected\", \"A8\": \"rejected\",\n    \"F0\": \"completed\", \"F1\": \"completed\", \"F2\": \"completed\", \"F3\": \"completed\",\n    \"P0\": \"in-progress\", \"P1\": \"in-progress\", \"P2\": \"in-progress\",\n    \"P3\": \"in-progress\", \"P4\": \"in-progress\", \"P5\": \"in-progress\"\n  };\n  return map[code] || \"in-progress\";\n}\n\nvar infoSourceLevels = arr(loops[\"2000A\"]);\nvar contexts = [];\n\nfor (var srcIdx = 0; srcIdx < infoSourceLevels.length; srcIdx++) {\n  var srcLevel = infoSourceLevels[srcIdx];\n  var srcNM1 = ((srcLevel.loops || {})[\"2100A\"] || {}).NM1 || {};\n  var payerInfo = { payer_id: srcNM1.identificationCode || \"\", name: srcNM1.nameLastOrOrganizationName || \"\" };\n\n  var receiverLevels = arr((srcLevel.loops || {})[\"2000B\"]);\n  for (var rcvIdx = 0; rcvIdx < receiverLevels.length; rcvIdx++) {\n    var rcvLevel = receiverLevels[rcvIdx];\n\n    var providerLevels = arr((rcvLevel.loops || {})[\"2000C\"]);\n    for (var provIdx = 0; provIdx < providerLevels.length; provIdx++) {\n      var provLevel = providerLevels[provIdx];\n      var provNM1 = ((provLevel.loops || {})[\"2100C\"] || {}).NM1 || {};\n      var providerInfo = { npi: provNM1.identificationCode || \"\", name: provNM1.nameLastOrOrganizationName || \"\" };\n\n      function pushContext(patientInfo, claim) {\n        // STC and REF both have maxUse=\">1\" in their own segment\n        // definitions, so the real parser always wraps them as arrays —\n        // take the first (claim-level overall status / control number)\n        // occurrence, even when only one is actually present.\n        var stc = first(claim.STC).healthCareClaimStatus || {};\n        var ref = first(claim.REF);\n        contexts.push({\n          patient_info: patientInfo,\n          payer_info: payerInfo,\n          provider_info: providerInfo,\n          response_identifier: claim.TRN ? claim.TRN.checkOrEFTTraceNumber : \"\",\n          payer_claim_control_number: ref.referenceIdentification || \"\",\n          task_status: stcCategoryToTaskStatus(stc.categoryCode || \"\"),\n          status_category_code: stc.categoryCode || \"\",\n          status_code: stc.statusCode || \"\",\n        });\n      }\n\n      var subscriberLevels = arr((provLevel.loops || {})[\"2000D\"]);\n      for (var subIdx = 0; subIdx < subscriberLevels.length; subIdx++) {\n        var subLevel = subscriberLevels[subIdx];\n        var sub2100D = (subLevel.loops || {})[\"2100D\"] || {};\n        var subNM1 = sub2100D.NM1 || {};\n        var subscriberInfo = { member_id: subNM1.identificationCode || \"\", first_name: subNM1.nameFirst || \"\", last_name: subNM1.nameLastOrOrganizationName || \"\", gender_fhir: \"unknown\" };\n\n        var claim2200D = arr(sub2100D.loops ? sub2100D.loops[\"2200D\"] : []);\n        for (var c = 0; c < claim2200D.length; c++) {\n          pushContext(subscriberInfo, claim2200D[c]);\n        }\n\n        // \"2000E\" is a SIBLING of \"2100D\" within 2000D's OWN \"loops\" map\n        // (matching Dependent's real HL parent being the Subscriber, not\n        // nested inside 2100D's own loops, which only holds \"2200D\") —\n        // read from subLevel (the 2000D instance itself), not sub2100D.\n        var dependentLevels = arr(subLevel.loops ? subLevel.loops[\"2000E\"] : []);\n        for (var depIdx = 0; depIdx < dependentLevels.length; depIdx++) {\n          var depLevel = dependentLevels[depIdx];\n          var dep2100E = (depLevel.loops || {})[\"2100E\"] || {};\n          var depNM1 = dep2100E.NM1 || {};\n          var dependentInfo = { member_id: depNM1.identificationCode || subscriberInfo.member_id, first_name: depNM1.nameFirst || \"\", last_name: depNM1.nameLastOrOrganizationName || \"\", gender_fhir: \"unknown\" };\n          var claim2200E = arr(dep2100E.loops ? dep2100E.loops[\"2200E\"] : []);\n          for (var d = 0; d < claim2200E.length; d++) {\n            pushContext(dependentInfo, claim2200E[d]);\n          }\n        }\n      }\n    }\n  }\n}\n\nreturn ({ _claim_status_responses: contexts });\n"},"enabled":true,"required":true}]},{"sequence":100,"steps":[{"step_name":"Build Payer Organization","step_alias":"build_payer_organization_fhir","step_type":"fhir.build","sequence":100,"description":"Builds the payer (2100A, the information source) as a FHIR Organization resource per claim status response context.","config":{"resourceType":"Organization","profile":"base","version":"R4","outputField":"message.fhirPayerOrganizations","rowsPath":"steps.derive_277_claim_status_responses.step_output._claim_status_responses","fields":[{"targetPath":"id","sourcePath":"payer_info.payer_id","transform":"string_prefix","valueMap":{"prefix":"organization-payer-"}},{"targetPath":"active","literalValue":"true"},{"targetPath":"name","sourcePath":"payer_info.name"}]},"enabled":true,"required":true}]},{"sequence":110,"steps":[{"step_name":"Build Patient","step_alias":"build_patient_fhir","step_type":"fhir.build","sequence":110,"description":"Builds one FHIR Patient resource per context -- the patient is the subscriber when asking about/reporting on herself, or the dependent (2100D/2100E) otherwise.","config":{"resourceType":"Patient","profile":"base","version":"R4","outputField":"message.fhirPatients","rowsPath":"steps.derive_277_claim_status_responses.step_output._claim_status_responses","fields":[{"targetPath":"id","sourcePath":"patient_info.member_id"},{"targetPath":"identifier[0].system","literalValue":"http://ezhealthkonnect.local/x12-member-id"},{"targetPath":"identifier[0].value","sourcePath":"patient_info.member_id"},{"targetPath":"name[0].family","sourcePath":"patient_info.last_name"},{"targetPath":"name[0].given[0]","sourcePath":"patient_info.first_name"},{"targetPath":"gender","sourcePath":"patient_info.gender_fhir"}]},"enabled":true,"required":true}]},{"sequence":115,"steps":[{"step_name":"Build Task","step_alias":"build_task_fhir","step_type":"fhir.build","sequence":115,"description":"Builds one FHIR Task resource per claim status response, translating the X12 STC category code into FHIR's own Task.status vocabulary -- a code-system translation, never an invented fact: the status always comes straight from the source STC.","config":{"resourceType":"Task","profile":"base","version":"R4","outputField":"message.fhirTasks","rowsPath":"steps.derive_277_claim_status_responses.step_output._claim_status_responses","fields":[{"targetPath":"id","sourcePath":"response_identifier","transform":"string_prefix","valueMap":{"prefix":"claim-status-response-"}},{"targetPath":"status","sourcePath":"task_status"},{"targetPath":"intent","literalValue":"order"},{"targetPath":"code.coding[0].system","literalValue":"http://terminology.hl7.org/CodeSystem/task-code"},{"targetPath":"code.coding[0].code","literalValue":"fulfill"},{"targetPath":"description","literalValue":"Health Care Claim Status Response (X12 277)"},{"targetPath":"for.reference","sourcePath":"patient_info.member_id","transform":"string_prefix","valueMap":{"prefix":"Patient/"}},{"targetPath":"owner.identifier.system","literalValue":"http://ezhealthkonnect.local/x12-payer-id"},{"targetPath":"owner.identifier.value","sourcePath":"payer_info.payer_id"},{"targetPath":"businessStatus.coding[0].system","literalValue":"https://x12.org/codes/health-care-claim-status-category-codes"},{"targetPath":"businessStatus.coding[0].code","sourcePath":"status_category_code"},{"targetPath":"identifier[0].system","literalValue":"http://ezhealthkonnect.local/x12-payer-claim-control-number"},{"targetPath":"identifier[0].value","sourcePath":"payer_claim_control_number"}]},"enabled":true,"required":true}]},{"sequence":200,"steps":[{"step_name":"Assemble 277 FHIR Bundle","step_alias":"assemble_277_fhir_bundle","step_type":"payload.builder","sequence":200,"description":"Assembles the payer Organization, Patient, and Task arrays into one FHIR R4 Bundle with correctly cross-referenced fullUrls.","config":{"mode":"fhir_bundle","fhirBundle":{"bundleType":"collection","resourcePaths":["message.fhirPayerOrganizations","message.fhirPatients","message.fhirTasks"]}},"enabled":true,"required":true}]},{"sequence":210,"steps":[{"step_name":"Validate FHIR Bundle","step_type":"fhir_validation","sequence":210,"config":{"validation_level":"strict","profile":"base","fhir_version":"R4","source_field":"payload"},"enabled":true,"required":false}]},{"sequence":295,"steps":[{"step_name":"Store Result","step_type":"connector.outbound","sequence":295,"config":{"connectorType":"sink_outbound","config":{"enable_logging":true,"enable_validation":true},"contentField":"payload"},"enabled":true,"required":true}]}]}
    $pipeline$::jsonb,
    '277',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_path","label":"Remote Directory","type":"string","hint":"Directory to poll for 277 files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive 277 (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse 277","icon":"📄","step_type":"edi.parse"},
        {"name":"Validate X12","icon":"✅","step_type":"edi.validate"},
        {"name":"Derive Claim Status Responses","icon":"🧩","step_type":"enrichment.script"},
        {"name":"Build Payer Organization","icon":"🏢","step_type":"fhir.build"},
        {"name":"Build Patient","icon":"🧑","step_type":"fhir.build"},
        {"name":"Build Task","icon":"📋","step_type":"fhir.build"},
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
