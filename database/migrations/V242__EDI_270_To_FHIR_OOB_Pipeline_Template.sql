-- V242: EDI X12 270 (Eligibility Inquiry) to FHIR OOB pipeline template
-- Applied: 2026-09-12

-- ============================================================
-- EDI 270 -> FHIR (CoverageEligibilityRequest + Patient + Organization) TEMPLATE
-- ============================================================
-- EDI X12 Phase 3 (real-time eligibility, transform-only scope): this engine
-- converts X12 <-> FHIR, it never decides eligibility facts. 837 doesn't
-- decide whether a claim is approved -- a real payer's adjudication system
-- does that; the same boundary applies here: this engine has no business
-- inventing "is this patient eligible" -- that answer comes from whatever
-- real system sits behind a real deployment. Mirrors V237/V238's own
-- structure exactly:
--   seq 5    connector.inbound   (edi_x12_inbound  -- polls an SFTP directory for 270 files)
--   seq 20   edi.parse           (raw X12 -> structured JSON)
--   seq 30   edi.validate        (base X12 5010 standard checks; errors block, warnings never do)
--   seq 95   enrichment.script   (Derive 270 Eligibility Contexts -- pure data reshaping only)
--   seq 100  fhir.build          (Organization -- information source / payer, one per context)
--   seq 105  fhir.build          (Organization -- information receiver / provider, one per context)
--   seq 110  fhir.build          (Patient -- rowsPath mode, one per eligibility context)
--   seq 115  fhir.build          (CoverageEligibilityRequest -- rowsPath mode, one per context)
--   seq 200  payload.builder     (fhir_bundle mode -- assembles the Bundle)
--   seq 210  fhir_validation     (strict -- validates the assembled Bundle)
--   seq 295  connector.outbound  (sink_outbound -- store-only terminal, no external I/O)
--
-- Config here is transcribed verbatim from services/edi_270_fhir_builder_test.go
-- (TestEDI270FHIRBuilder_SubscriberAndDependent_BuildsCleanValidatingBundle),
-- which proves this exact chain against a real fixture (a subscriber asking
-- about her own coverage, plus a SEPARATE subscriber asking about her
-- dependent's coverage instead -- matching real X12 usage exactly, a
-- subscriber loop carrying a 2000D dependent answers ABOUT that dependent,
-- never about herself simultaneously) via the real executors, and asserts
-- the resulting Bundle validates at strict level with zero unexpected errors.
--
-- Named simplifications, stated up front rather than silently left out:
--   - Payer/Provider Organization rows are built PER eligibility context, not
--     deduplicated across contexts sharing the same real-world payer/provider
--     -- same "not deduplicated" precedent 837's own Patient/Coverage building
--     already documents (harmless per FHIR Bundle semantics: same stable id).
--   - Loop 2117 (LQ) and loop 2120 (LS/LE-wrapped Related Entity) are not
--     modeled in the underlying 270 schema (edi/schemas/x12_005010/270.json)
--     -- named, deliberate simplifications documented in that file's own
--     _sourceRefs. MPI (Military Personnel Information) is also not modeled.
--   - TRN at 2000C/2000D reuses the existing globally-shared TRN.json
--     (maxUse stays "1", unchanged, to avoid regressing 835/837's own
--     already-proven TRN usage as a single header segment) -- a trading
--     partner sending more than one TRN per subscriber/dependent loop would
--     only have the first captured.
--   - "data in any other form -> 270/271" (the build direction) is already
--     covered by edi.build's existing, fully generic canonical-JSON-in
--     mechanism -- no separate template needed for that direction; it's
--     proven directly by tests/playwright/edi-build-e2e.spec.js's own
--     835/837P/837I precedent, extended to 270/271.

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
    'EDI X12 270 to FHIR (SFTP)',
    'edi-270-to-fhir-sftp',
    'Poll an SFTP directory for X12 270 (Eligibility, Coverage or Benefit Inquiry) files, parse and validate them, then build and validate a FHIR R4 Bundle containing Organization, Patient, and CoverageEligibilityRequest resources -- one Patient/CoverageEligibilityRequest per patient in the file (subscriber or dependent).',
    'edi',
    null,
    ARRAY['edi','x12','270','eligibility','inquiry','sftp','fhir'],
    '🔍',
    'advanced',
    'edi_x12_inbound',
    '{"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["270"]}',
    'sink_outbound',
    '{"enable_logging": true, "enable_validation": true}',
    $pipeline$
{"execution_groups": [{"sequence": 5, "steps": [{"step_name": "Receive 270 File (SFTP)", "step_type": "connector.inbound", "sequence": 5, "config": {"connectorType": "edi_x12_inbound", "config": {"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["270"]}}, "enabled": true, "required": true}]}, {"sequence": 20, "steps": [{"step_name": "Parse 270 -> JSON", "step_type": "edi.parse", "sequence": 20, "config": {"sourceField": "raw", "outputField": "parsedEDI", "transactionSet": "270"}, "enabled": true, "required": true}]}, {"sequence": 30, "steps": [{"step_name": "Validate Against X12 5010", "step_type": "edi.validate", "sequence": 30, "config": {"sourceField": "raw", "outputField": "ediValidation", "customRules": []}, "enabled": true, "required": false}]}, {"sequence": 95, "steps": [{"step_name": "Derive 270 Eligibility Contexts", "step_alias": "derive_270_eligibility_contexts", "step_type": "enrichment.script", "sequence": 95, "description": "Pure data reshaping only -- this engine transforms X12 <-> FHIR, it never decides eligibility facts. Flattens 270's information-source/receiver/subscriber/dependent structure into one flat array (_eligibility_contexts), one row per patient (subscriber or her dependent), so the fhir.build steps below can build one Patient/CoverageEligibilityRequest per patient declaratively via rowsPath.", "config": {"script": "\nvar parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};\nvar loops = parsed.loops || {};\n\nif (parsed.transactionSet !== \"270\") {\n  return ({ _eligibility_contexts: [] });\n}\n\nfunction arr(v) {\n  if (!v) return [];\n  return Array.isArray(v) ? v : [v];\n}\nfunction first(v) {\n  var a = arr(v);\n  return a.length > 0 ? a[0] : {};\n}\nfunction genderToFHIR(code) {\n  if (code === \"M\") return \"male\";\n  if (code === \"F\") return \"female\";\n  return \"unknown\";\n}\nfunction relationshipToFHIR(code) {\n  var map = { \"18\": \"self\", \"01\": \"spouse\", \"19\": \"child\", \"20\": \"employee\" };\n  return map[code] || \"other\";\n}\n\nvar infoSourceLevels = arr(loops[\"2000A\"]);\nvar contexts = [];\nvar nowIso = new Date().toISOString();\n\nfor (var srcIdx = 0; srcIdx < infoSourceLevels.length; srcIdx++) {\n  var srcLevel = infoSourceLevels[srcIdx];\n  var srcNM1 = ((srcLevel.loops || {})[\"2100A\"] || {}).NM1 || {};\n  var payerInfo = { payer_id: srcNM1.identificationCode || \"\", name: srcNM1.nameLastOrOrganizationName || \"\" };\n\n  var receiverLevels = arr((srcLevel.loops || {})[\"2000B\"]);\n  for (var rcvIdx = 0; rcvIdx < receiverLevels.length; rcvIdx++) {\n    var rcvLevel = receiverLevels[rcvIdx];\n    var rcvNM1 = ((rcvLevel.loops || {})[\"2100B\"] || {}).NM1 || {};\n    var providerInfo = { npi: rcvNM1.identificationCode || \"\", name: rcvNM1.nameLastOrOrganizationName || \"\" };\n\n    var subscriberLevels = arr((rcvLevel.loops || {})[\"2000C\"]);\n    for (var subIdx = 0; subIdx < subscriberLevels.length; subIdx++) {\n      var subLevel = subscriberLevels[subIdx];\n      var sub2100C = (subLevel.loops || {})[\"2100C\"] || {};\n      var subNM1 = sub2100C.NM1 || {};\n      var subDMG = sub2100C.DMG || {};\n      var subscriberInfo = {\n        member_id: subNM1.identificationCode || \"\",\n        first_name: subNM1.nameFirst || \"\",\n        last_name: subNM1.nameLastOrOrganizationName || \"\",\n        dob: subDMG.birthDate || \"\",\n        gender_fhir: genderToFHIR(subDMG.genderCode),\n      };\n\n      var dependentLevels = arr((subLevel.loops || {})[\"2000D\"]);\n\n      function pushContext(patientInfo, relationshipFHIR, eqLoops) {\n        var items = [];\n        var eqList = arr(eqLoops);\n        for (var i = 0; i < eqList.length; i++) {\n          var eq = eqList[i].EQ || {};\n          if (!eq.serviceTypeCode) continue;\n          items.push({ category_code: eq.serviceTypeCode });\n        }\n        contexts.push({\n          patient_info: patientInfo,\n          subscriber_info: subscriberInfo,\n          payer_info: payerInfo,\n          provider_info: providerInfo,\n          relationship_fhir: relationshipFHIR,\n          created_at: nowIso,\n          request_identifier: subLevel.TRN ? subLevel.TRN.checkOrEFTTraceNumber : \"\",\n          items: items,\n        });\n      }\n\n      if (dependentLevels.length === 0) {\n        pushContext(subscriberInfo, \"self\", sub2100C.loops ? sub2100C.loops[\"2110C\"] : []);\n      } else {\n        for (var depIdx = 0; depIdx < dependentLevels.length; depIdx++) {\n          var depLevel = dependentLevels[depIdx];\n          var dep2100D = (depLevel.loops || {})[\"2100D\"] || {};\n          var depNM1 = dep2100D.NM1 || {};\n          var depDMG = dep2100D.DMG || {};\n          var depINS = dep2100D.INS || {};\n          var dependentInfo = {\n            member_id: depNM1.identificationCode || subscriberInfo.member_id,\n            first_name: depNM1.nameFirst || \"\",\n            last_name: depNM1.nameLastOrOrganizationName || \"\",\n            dob: depDMG.birthDate || \"\",\n            gender_fhir: genderToFHIR(depDMG.genderCode),\n          };\n          pushContext(dependentInfo, relationshipToFHIR(depINS.individualRelationshipCode), dep2100D.loops ? dep2100D.loops[\"2110D\"] : []);\n        }\n      }\n    }\n  }\n}\n\nreturn ({ _eligibility_contexts: contexts });\n"}, "enabled": true, "required": true}]}, {"sequence": 100, "steps": [{"step_name": "Build Payer Organization", "step_alias": "build_payer_organization_fhir", "step_type": "fhir.build", "sequence": 100, "description": "Builds the information source (2100A, the payer) as a FHIR Organization resource per eligibility context.", "config": {"fields": [{"sourcePath": "payer_info.payer_id", "targetPath": "id", "transform": "string_prefix", "valueMap": {"prefix": "organization-payer-"}}, {"literalValue": "true", "targetPath": "active"}, {"sourcePath": "payer_info.name", "targetPath": "name"}], "outputField": "message.fhirPayerOrganizations", "profile": "base", "resourceType": "Organization", "rowsPath": "steps.derive_270_eligibility_contexts.step_output._eligibility_contexts", "version": "R4"}, "enabled": true, "required": true}]}, {"sequence": 105, "steps": [{"step_name": "Build Provider Organization", "step_alias": "build_provider_organization_fhir", "step_type": "fhir.build", "sequence": 105, "description": "Builds the information receiver (2100B, the provider) as a FHIR Organization resource per eligibility context.", "config": {"fields": [{"sourcePath": "provider_info.npi", "targetPath": "id", "transform": "string_prefix", "valueMap": {"prefix": "organization-provider-"}}, {"literalValue": "true", "targetPath": "active"}, {"sourcePath": "provider_info.name", "targetPath": "name"}], "outputField": "message.fhirProviderOrganizations", "profile": "base", "resourceType": "Organization", "rowsPath": "steps.derive_270_eligibility_contexts.step_output._eligibility_contexts", "version": "R4"}, "enabled": true, "required": true}]}, {"sequence": 110, "steps": [{"step_name": "Build Patient", "step_alias": "build_patient_fhir", "step_type": "fhir.build", "sequence": 110, "description": "Builds one FHIR Patient resource per eligibility context -- the patient is the subscriber when asking about herself, or the dependent (2100D) otherwise.", "config": {"fields": [{"sourcePath": "patient_info.member_id", "targetPath": "id"}, {"literalValue": "http://ezhealthkonnect.local/x12-member-id", "targetPath": "identifier[0].system"}, {"sourcePath": "patient_info.member_id", "targetPath": "identifier[0].value"}, {"sourcePath": "patient_info.last_name", "targetPath": "name[0].family"}, {"sourcePath": "patient_info.first_name", "targetPath": "name[0].given[0]"}, {"sourcePath": "patient_info.dob", "targetPath": "birthDate", "transform": "x12_date_to_fhir_date"}, {"sourcePath": "patient_info.gender_fhir", "targetPath": "gender"}], "outputField": "message.fhirPatients", "profile": "base", "resourceType": "Patient", "rowsPath": "steps.derive_270_eligibility_contexts.step_output._eligibility_contexts", "version": "R4"}, "enabled": true, "required": true}]}, {"sequence": 115, "steps": [{"step_name": "Build CoverageEligibilityRequest", "step_alias": "build_coverage_eligibility_request_fhir", "step_type": "fhir.build", "sequence": 115, "description": "Builds one FHIR CoverageEligibilityRequest resource per eligibility context. item[] built via repeatingGroups off that patient's own EQ inquiries (one row per 2110C/2110D EQ occurrence).", "config": {"fields": [{"sourcePath": "request_identifier", "targetPath": "id", "transform": "string_prefix", "valueMap": {"prefix": "eligibility-request-"}}, {"literalValue": "active", "targetPath": "status"}, {"literalValue": "benefits", "targetPath": "purpose[0]"}, {"sourcePath": "patient_info.member_id", "targetPath": "patient.reference", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}}, {"sourcePath": "created_at", "targetPath": "created"}, {"literalValue": "http://hl7.org/fhir/sid/us-npi", "targetPath": "provider.identifier.system"}, {"sourcePath": "provider_info.npi", "targetPath": "provider.identifier.value"}, {"literalValue": "http://ezhealthkonnect.local/x12-payer-id", "targetPath": "insurer.identifier.system"}, {"sourcePath": "payer_info.payer_id", "targetPath": "insurer.identifier.value"}], "outputField": "message.fhirCoverageEligibilityRequests", "profile": "base", "repeatingGroups": [{"fields": [{"literalValue": "https://codesystem.x12.org/005010/1365", "targetPath": "category.coding[0].system"}, {"sourcePath": "category_code", "targetPath": "category.coding[0].code"}], "rowsPath": "items", "targetPath": "item"}], "resourceType": "CoverageEligibilityRequest", "rowsPath": "steps.derive_270_eligibility_contexts.step_output._eligibility_contexts", "version": "R4"}, "enabled": true, "required": true}]}, {"sequence": 200, "steps": [{"step_name": "Assemble 270 FHIR Bundle", "step_alias": "assemble_270_fhir_bundle", "step_type": "payload.builder", "sequence": 200, "description": "Assembles the payer/provider Organization, Patient, and CoverageEligibilityRequest arrays into one FHIR R4 Bundle with correctly cross-referenced fullUrls.", "config": {"mode": "fhir_bundle", "fhirBundle": {"bundleType": "collection", "resourcePaths": ["message.fhirPayerOrganizations", "message.fhirProviderOrganizations", "message.fhirPatients", "message.fhirCoverageEligibilityRequests"]}}, "enabled": true, "required": true}]}, {"sequence": 210, "steps": [{"step_name": "Validate FHIR Bundle", "step_type": "fhir_validation", "sequence": 210, "config": {"validation_level": "strict", "profile": "base", "fhir_version": "R4", "source_field": "payload"}, "enabled": true, "required": false}]}, {"sequence": 295, "steps": [{"step_name": "Store Result", "step_type": "connector.outbound", "sequence": 295, "config": {"connectorType": "sink_outbound", "config": {"enable_logging": true, "enable_validation": true}, "contentField": "payload"}, "enabled": true, "required": true}]}]}
    $pipeline$::jsonb,
    '270',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_path","label":"Remote Directory","type":"string","hint":"Directory to poll for 270 files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive 270 (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse 270","icon":"📄","step_type":"edi.parse"},
        {"name":"Validate X12","icon":"✅","step_type":"edi.validate"},
        {"name":"Derive Eligibility Contexts","icon":"🧩","step_type":"enrichment.script"},
        {"name":"Build Payer Organization","icon":"🏢","step_type":"fhir.build"},
        {"name":"Build Provider Organization","icon":"🏥","step_type":"fhir.build"},
        {"name":"Build Patient","icon":"🧑","step_type":"fhir.build"},
        {"name":"Build CoverageEligibilityRequest","icon":"🔍","step_type":"fhir.build"},
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
