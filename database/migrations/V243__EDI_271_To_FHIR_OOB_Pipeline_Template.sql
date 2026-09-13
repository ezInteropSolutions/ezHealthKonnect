-- V243: EDI X12 271 (Eligibility Response) to FHIR OOB pipeline template
-- Applied: 2026-09-12

-- ============================================================
-- EDI 271 -> FHIR (CoverageEligibilityResponse + Patient) TEMPLATE
-- ============================================================
-- EDI X12 Phase 3 companion to V242 -- see that migration's own header
-- comment for the full "transform-only, never decides eligibility facts"
-- scope rationale, which applies identically here.
--   seq 5    connector.inbound   (edi_x12_inbound  -- polls an SFTP directory for 271 files)
--   seq 20   edi.parse           (raw X12 -> structured JSON)
--   seq 30   edi.validate        (base X12 5010 standard checks; errors block, warnings never do)
--   seq 95   enrichment.script   (Derive 271 Eligibility Contexts -- pure data reshaping only)
--   seq 100  fhir.build          (Patient -- rowsPath mode, one per coverage context)
--   seq 105  fhir.build          (CoverageEligibilityResponse -- rowsPath mode, one per context)
--   seq 200  payload.builder     (fhir_bundle mode -- assembles the Bundle)
--   seq 210  fhir_validation     (strict -- validates the assembled Bundle)
--   seq 295  connector.outbound  (sink_outbound -- store-only terminal, no external I/O)
--
-- Config here is transcribed verbatim from services/edi_271_fhir_builder_test.go
-- (TestEDI271FHIRBuilder_ActiveCoverageAndRejection_BuildsCleanValidatingBundle),
-- which proves this exact chain against a real fixture (one subscriber
-- getting a real active-coverage EB answer with a nested benefit, and a
-- SEPARATE subscriber whose dependent gets an AAA rejection instead) via the
-- real executors -- proving both the "complete" and "error" outcome
-- branches, and the genuinely 3-level nested insurance[].item[].benefit[]
-- FHIR shape via fhir.build's own nested repeatingGroups mechanism -- and
-- asserts the resulting Bundle validates at strict level with zero
-- unexpected errors.
--
-- Named simplifications (same boundary as V242''s own note):
--   - "insurance" is pre-flattened by the derive script to a 0-or-1-element
--     array (one real-world coverage per patient, or none for a rejected
--     patient) -- a rejected patient''s CoverageEligibilityResponse carries
--     outcome="error" and error[] (from the real AAA reject reason) instead
--     of any insurance[] entry.
--   - request (Reference(CoverageEligibilityRequest)) is a logical
--     (identifier-only) reference keyed on the 271''s own TRN trace number --
--     a 271 processed independently has no real prior
--     CoverageEligibilityRequest resource in the same Bundle to point at,
--     same "logical reference" pattern 837''s own careTeam[].provider/
--     Claim.facility already established.
--   - Loop 2117 (LQ) and loop 2120 (LS/LE-wrapped Related Entity) are not
--     modeled (see edi/schemas/x12_005010/271.json''s own _sourceRefs).

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
    'EDI X12 271 to FHIR (SFTP)',
    'edi-271-to-fhir-sftp',
    'Poll an SFTP directory for X12 271 (Eligibility, Coverage or Benefit Information) files, parse and validate them, then build and validate a FHIR R4 Bundle containing Patient and CoverageEligibilityResponse resources -- one Patient/CoverageEligibilityResponse per patient in the file (subscriber or dependent), correctly reflecting both real benefit answers and AAA rejections.',
    'edi',
    null,
    ARRAY['edi','x12','271','eligibility','response','sftp','fhir'],
    '📋',
    'advanced',
    'edi_x12_inbound',
    '{"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["271"]}',
    'sink_outbound',
    '{"enable_logging": true, "enable_validation": true}',
    $pipeline$
{"execution_groups": [{"sequence": 5, "steps": [{"step_name": "Receive 271 File (SFTP)", "step_type": "connector.inbound", "sequence": 5, "config": {"connectorType": "edi_x12_inbound", "config": {"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["271"]}}, "enabled": true, "required": true}]}, {"sequence": 20, "steps": [{"step_name": "Parse 271 -> JSON", "step_type": "edi.parse", "sequence": 20, "config": {"sourceField": "raw", "outputField": "parsedEDI", "transactionSet": "271"}, "enabled": true, "required": true}]}, {"sequence": 30, "steps": [{"step_name": "Validate Against X12 5010", "step_type": "edi.validate", "sequence": 30, "config": {"sourceField": "raw", "outputField": "ediValidation", "customRules": []}, "enabled": true, "required": false}]}, {"sequence": 95, "steps": [{"step_name": "Derive 271 Eligibility Contexts", "step_alias": "derive_271_eligibility_contexts", "step_type": "enrichment.script", "sequence": 95, "description": "Pure data reshaping only -- this engine transforms X12 <-> FHIR, it never decides eligibility facts. Flattens 271's subscriber/dependent structure into one flat array (_coverage_contexts), pre-computing the complete/error outcome directly from whether that patient's own 2100C/2100D carries an AAA rejection segment -- a structural fact already in the message, never invented.", "config": {"script": "\nvar parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};\nvar loops = parsed.loops || {};\n\nif (parsed.transactionSet !== \"271\") {\n  return ({ _coverage_contexts: [] });\n}\n\nfunction arr(v) {\n  if (!v) return [];\n  return Array.isArray(v) ? v : [v];\n}\n\nvar infoSourceLevels = arr(loops[\"2000A\"]);\nvar contexts = [];\nvar nowIso = new Date().toISOString();\n\nfor (var srcIdx = 0; srcIdx < infoSourceLevels.length; srcIdx++) {\n  var srcLevel = infoSourceLevels[srcIdx];\n  var srcNM1 = ((srcLevel.loops || {})[\"2100A\"] || {}).NM1 || {};\n  var payerInfo = { payer_id: srcNM1.identificationCode || \"\", name: srcNM1.nameLastOrOrganizationName || \"\" };\n\n  var receiverLevels = arr((srcLevel.loops || {})[\"2000B\"]);\n  for (var rcvIdx = 0; rcvIdx < receiverLevels.length; rcvIdx++) {\n    var rcvLevel = receiverLevels[rcvIdx];\n\n    var subscriberLevels = arr((rcvLevel.loops || {})[\"2000C\"]);\n    for (var subIdx = 0; subIdx < subscriberLevels.length; subIdx++) {\n      var subLevel = subscriberLevels[subIdx];\n      var sub2100C = (subLevel.loops || {})[\"2100C\"] || {};\n      var subNM1 = sub2100C.NM1 || {};\n      var subscriberInfo = {\n        member_id: subNM1.identificationCode || \"\",\n        first_name: subNM1.nameFirst || \"\",\n        last_name: subNM1.nameLastOrOrganizationName || \"\",\n      };\n\n      var dependentLevels = arr((subLevel.loops || {})[\"2000D\"]);\n\n      // Builds the item[] rows (one per real 2110C/2110D EB occurrence) that\n      // sit INSIDE the one insurance[] entry -- FHIR's own shape is ONE\n      // coverage (insurance[]) carrying MULTIPLE reported benefits (item[]),\n      // not one insurance entry per benefit fact.\n      function buildInsuranceItemRows(eb2110List) {\n        var out = [];\n        var ebList = arr(eb2110List);\n        for (var i = 0; i < ebList.length; i++) {\n          var eb = ebList[i].EB || {};\n          if (!eb.eligibilityBenefitInformationCode) continue;\n          var benefits = [{\n            eb_code: eb.eligibilityBenefitInformationCode,\n            allowed_amount: eb.monetaryAmount || \"\",\n            plan_description: eb.planCoverageDescription || \"\",\n          }];\n          out.push({ category_code: eb.serviceTypeCode || \"\", benefits: benefits });\n        }\n        return out;\n      }\n\n      function pushContext(patientInfo, requestId, aaaList, eb2110List) {\n        var rejections = arr(aaaList);\n        var isRejected = rejections.length > 0;\n        var errorItems = [];\n        for (var i = 0; i < rejections.length; i++) {\n          errorItems.push({ reject_reason_code: rejections[i].rejectReasonCode || \"\" });\n        }\n        // \"insurance\" is a 0-or-1-element array (this mapping reports exactly\n        // one real-world coverage per patient) so the fhir.build repeatingGroup\n        // naturally produces zero insurance[] entries for a rejected patient --\n        // the same \"empty rowsPath array -> zero repeatingGroup rows\" convention\n        // 837's own diagnosis/item/careTeam repeatingGroups already rely on,\n        // rather than a separate conditional mechanism.\n        var insurance = isRejected ? [] : [{\n          member_id: patientInfo.member_id,\n          items: buildInsuranceItemRows(eb2110List),\n        }];\n        contexts.push({\n          patient_info: patientInfo,\n          payer_info: payerInfo,\n          created_at: nowIso,\n          request_identifier: requestId || \"\",\n          outcome: isRejected ? \"error\" : \"complete\",\n          insurance: insurance,\n          error_items: errorItems,\n        });\n      }\n\n      if (dependentLevels.length === 0) {\n        pushContext(\n          subscriberInfo,\n          subLevel.TRN ? subLevel.TRN.checkOrEFTTraceNumber : \"\",\n          sub2100C.AAA,\n          sub2100C.loops ? sub2100C.loops[\"2110C\"] : []\n        );\n      } else {\n        for (var depIdx = 0; depIdx < dependentLevels.length; depIdx++) {\n          var depLevel = dependentLevels[depIdx];\n          var dep2100D = (depLevel.loops || {})[\"2100D\"] || {};\n          var depNM1 = dep2100D.NM1 || {};\n          var dependentInfo = {\n            member_id: depNM1.identificationCode || subscriberInfo.member_id,\n            first_name: depNM1.nameFirst || \"\",\n            last_name: depNM1.nameLastOrOrganizationName || \"\",\n          };\n          pushContext(\n            dependentInfo,\n            depLevel.TRN ? depLevel.TRN.checkOrEFTTraceNumber : \"\",\n            dep2100D.AAA,\n            dep2100D.loops ? dep2100D.loops[\"2110D\"] : []\n          );\n        }\n      }\n    }\n  }\n}\n\nreturn ({ _coverage_contexts: contexts });\n"}, "enabled": true, "required": true}]}, {"sequence": 100, "steps": [{"step_name": "Build Patient", "step_alias": "build_patient_fhir", "step_type": "fhir.build", "sequence": 100, "description": "Builds one FHIR Patient resource per coverage context -- the patient is the subscriber when the response is about herself, or the dependent (2100D) otherwise.", "config": {"fields": [{"sourcePath": "patient_info.member_id", "targetPath": "id"}, {"literalValue": "http://ezhealthkonnect.local/x12-member-id", "targetPath": "identifier[0].system"}, {"sourcePath": "patient_info.member_id", "targetPath": "identifier[0].value"}, {"sourcePath": "patient_info.last_name", "targetPath": "name[0].family"}, {"sourcePath": "patient_info.first_name", "targetPath": "name[0].given[0]"}], "outputField": "message.fhirPatients", "profile": "base", "resourceType": "Patient", "rowsPath": "steps.derive_271_eligibility_contexts.step_output._coverage_contexts", "version": "R4"}, "enabled": true, "required": true}]}, {"sequence": 105, "steps": [{"step_name": "Build CoverageEligibilityResponse", "step_alias": "build_coverage_eligibility_response_fhir", "step_type": "fhir.build", "sequence": 105, "description": "Builds one FHIR CoverageEligibilityResponse resource per coverage context. insurance[].item[].benefit[] built via nested repeatingGroups off that patient's own EB-derived benefit facts (empty for a rejected patient, whose outcome=error and error[] carry the real AAA reject reason instead).", "config": {"fields": [{"sourcePath": "request_identifier", "targetPath": "id", "transform": "string_prefix", "valueMap": {"prefix": "eligibility-response-"}}, {"literalValue": "active", "targetPath": "status"}, {"literalValue": "benefits", "targetPath": "purpose[0]"}, {"sourcePath": "patient_info.member_id", "targetPath": "patient.reference", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}}, {"sourcePath": "created_at", "targetPath": "created"}, {"sourcePath": "outcome", "targetPath": "outcome"}, {"literalValue": "http://ezhealthkonnect.local/x12-trace-number", "targetPath": "request.identifier.system"}, {"sourcePath": "request_identifier", "targetPath": "request.identifier.value"}, {"literalValue": "http://ezhealthkonnect.local/x12-payer-id", "targetPath": "insurer.identifier.system"}, {"sourcePath": "payer_info.payer_id", "targetPath": "insurer.identifier.value"}], "outputField": "message.fhirCoverageEligibilityResponses", "profile": "base", "repeatingGroups": [{"fields": [{"literalValue": "http://ezhealthkonnect.local/x12-member-id", "targetPath": "coverage.identifier.system"}, {"sourcePath": "member_id", "targetPath": "coverage.identifier.value"}], "repeatingGroups": [{"fields": [{"literalValue": "https://codesystem.x12.org/005010/1365", "targetPath": "category.coding[0].system"}, {"sourcePath": "category_code", "targetPath": "category.coding[0].code"}], "repeatingGroups": [{"fields": [{"literalValue": "https://codesystem.x12.org/005010/1338", "targetPath": "type.coding[0].system"}, {"sourcePath": "eb_code", "targetPath": "type.coding[0].code"}, {"sourcePath": "allowed_amount", "targetPath": "allowedMoney.value"}, {"literalValue": "USD", "targetPath": "allowedMoney.currency"}], "rowsPath": "benefits", "targetPath": "benefit"}], "rowsPath": "items", "targetPath": "item"}], "rowsPath": "insurance", "targetPath": "insurance"}, {"fields": [{"literalValue": "https://codesystem.x12.org/005010/901", "targetPath": "code.coding[0].system"}, {"sourcePath": "reject_reason_code", "targetPath": "code.coding[0].code"}], "rowsPath": "error_items", "targetPath": "error"}], "resourceType": "CoverageEligibilityResponse", "rowsPath": "steps.derive_271_eligibility_contexts.step_output._coverage_contexts", "version": "R4"}, "enabled": true, "required": true}]}, {"sequence": 200, "steps": [{"step_name": "Assemble 271 FHIR Bundle", "step_alias": "assemble_271_fhir_bundle", "step_type": "payload.builder", "sequence": 200, "description": "Assembles the Patient and CoverageEligibilityResponse arrays into one FHIR R4 Bundle with correctly cross-referenced fullUrls.", "config": {"mode": "fhir_bundle", "fhirBundle": {"bundleType": "collection", "resourcePaths": ["message.fhirPatients", "message.fhirCoverageEligibilityResponses"]}}, "enabled": true, "required": true}]}, {"sequence": 210, "steps": [{"step_name": "Validate FHIR Bundle", "step_type": "fhir_validation", "sequence": 210, "config": {"validation_level": "strict", "profile": "base", "fhir_version": "R4", "source_field": "payload"}, "enabled": true, "required": false}]}, {"sequence": 295, "steps": [{"step_name": "Store Result", "step_type": "connector.outbound", "sequence": 295, "config": {"connectorType": "sink_outbound", "config": {"enable_logging": true, "enable_validation": true}, "contentField": "payload"}, "enabled": true, "required": true}]}]}
    $pipeline$::jsonb,
    '271',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_path","label":"Remote Directory","type":"string","hint":"Directory to poll for 271 files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive 271 (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse 271","icon":"📄","step_type":"edi.parse"},
        {"name":"Validate X12","icon":"✅","step_type":"edi.validate"},
        {"name":"Derive Coverage Contexts","icon":"🧩","step_type":"enrichment.script"},
        {"name":"Build Patient","icon":"🧑","step_type":"fhir.build"},
        {"name":"Build CoverageEligibilityResponse","icon":"📋","step_type":"fhir.build"},
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
