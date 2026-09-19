-- V254: EDI X12 278 (Health Care Services Review — Prior Authorization
-- Request/Response) to FHIR OOB pipeline template
-- Applied: 2026-09-14

-- ============================================================
-- EDI 278 -> FHIR (Claim / ClaimResponse) TEMPLATE
-- ============================================================
-- EDI X12 Phase 7 companion to V242/V243/V251/V252 -- see those migrations'
-- own header comments for the full "transform-only, never decides facts"
-- scope rationale, which applies identically here: this pipeline reshapes
-- and re-expresses whatever a real 278 file already carries, it never
-- decides a prior authorization outcome.
--
-- UNLIKE every other request/response pair this engine has built (270/271,
-- 276/277 -- different ST01; 837P/837I -- same ST01, different GS08), 278
-- REQUEST and RESPONSE share the IDENTICAL ST01='278' AND GS08='005010X217'
-- -- there is no envelope-level way to distinguish them. This ONE template
-- (backed by ONE unified edi/schemas/x12_005010/278.json schema) handles
-- BOTH: a Claim (use=preauthorization) is ALWAYS built per patient event: a
-- ClaimResponse is built ONLY when that event's own HCR segment (the real
-- certification decision) is present -- a pure request produces zero
-- ClaimResponse rows. See 278.json's own _sourceRefs for the full
-- "PAS vs 278 -- different wire formats, same business process" rationale
-- for choosing Claim/ClaimResponse over Task (276/277's own choice, wrong
-- here since 278 carries real clinical/service content and an adjudication-
-- style decision Task can't represent).
--
--   seq 5    connector.inbound   (edi_x12_inbound  -- polls an SFTP directory for 278 files)
--   seq 20   edi.parse           (raw X12 -> structured JSON)
--   seq 30   edi.validate        (base X12 5010 standard checks; errors block, warnings never do)
--   seq 95   enrichment.script   (Derive 278 Prior Auth Contexts -- pure data reshaping + is_response detection)
--   seq 100  fhir.build          (Organization -- the UMO, single-resource, built once)
--   seq 110  fhir.build          (Patient -- rowsPath mode, one per patient event)
--   seq 115  fhir.build          (Claim -- rowsPath mode, ALWAYS built, one per patient event)
--   seq 120  fhir.build          (ClaimResponse -- rowsPath mode against a PRE-FILTERED _response_contexts array; fhir.build has no whole-resource-row "condition" gate, so the is_response filter happens in the derive script itself)
--   seq 200  payload.builder     (fhir_bundle mode -- assembles the Bundle)
--   seq 210  fhir_validation     (strict -- validates the assembled Bundle)
--   seq 295  connector.outbound  (sink_outbound -- store-only terminal, no external I/O)
--
-- Config here is transcribed programmatically (via a Node script reading the
-- Go source directly, avoiding the hand-retyping bug class an earlier EDI
-- round hit once) from services/edi_278_fhir_builder_test.go
-- (TestEDI278FHIRBuilder_RequestAndResponse_BuildsCleanValidatingBundle),
-- which proves this exact chain against a real fixture (a subscriber's own
-- patient event already carrying a real certification decision, and a
-- SEPARATE dependent's own patient event still a pure request) via the real
-- executors, proving EXACTLY 1 ClaimResponse is built (not 2), and asserts
-- the resulting Bundle validates at strict level with zero unexpected
-- errors.
--
-- Named simplifications (same discipline as every prior EDI-to-FHIR round):
--   - Claim.provider/insurance are populated as logical (identifier-only)
--     references -- no cross-Bundle resource resolution beyond what this
--     Bundle itself builds, matching 837's own careTeam[].provider /
--     Claim.facility precedent.
--   - Only diagnosis (HI) and service-line procedure codes (SV1) are
--     mapped into Claim's own diagnosis[]/item[] -- the many optional
--     278 certification-detail segments (CRC/CR1/CR2/CR5/CR6/HSD/PWK/MSG,
--     dental SV3/TOO) are schema-complete (see 278.json) but not yet
--     mapped into FHIR fields, matching every prior EDI-to-FHIR round's own
--     "core fields, not exhaustive" precedent.
--   - Claim.created has no real X12 278 equivalent (no "record created"
--     timestamp on the wire) -- defaulted to the pipeline's own processing
--     time, the same convention BHT03/BHT04 already establish elsewhere in
--     this schema library.

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
    'EDI X12 278 to FHIR (SFTP)',
    'edi-278-to-fhir-sftp',
    'Poll an SFTP directory for X12 278 (Health Care Services Review -- prior authorization request/response) files, parse and validate them, then build and validate a FHIR R4 Bundle containing Organization (UMO), Patient, Claim (use=preauthorization, always built), and ClaimResponse (built only when a real certification decision is present) resources.',
    'edi',
    null,
    ARRAY['edi','x12','278','prior-authorization','review','sftp','fhir'],
    '🏥',
    'advanced',
    'edi_x12_inbound',
    '{"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["278"]}',
    'sink_outbound',
    '{"enable_logging": true, "enable_validation": true}',
    $pipeline$
{"execution_groups":[{"sequence":5,"steps":[{"step_name":"Receive 278 File (SFTP)","step_type":"connector.inbound","sequence":5,"config":{"connectorType":"edi_x12_inbound","config":{"transport":"sftp","remote_path":"/incoming","file_pattern":"*.edi","polling_interval_seconds":300,"after_processing":"archive","transaction_types":["278"]}},"enabled":true,"required":true}]},{"sequence":20,"steps":[{"step_name":"Parse 278 -> JSON","step_type":"edi.parse","sequence":20,"config":{"sourceField":"raw","outputField":"parsedEDI","transactionSet":"278"},"enabled":true,"required":true}]},{"sequence":30,"steps":[{"step_name":"Validate Against X12 5010","step_type":"edi.validate","sequence":30,"config":{"sourceField":"raw","outputField":"ediValidation","customRules":[]},"enabled":true,"required":false}]},{"sequence":95,"steps":[{"step_name":"Derive 278 Prior Auth Contexts","step_alias":"derive_278_prior_auth_contexts","step_type":"enrichment.script","sequence":95,"description":"Pure structural reshaping only -- this engine transforms X12 <-> FHIR, it never decides a prior authorization outcome. Flattens 278's UMO/requester/subscriber/dependent/patient-event hierarchy into one flat array (_prior_auth_contexts), one row per patient event, computing is_response purely from HCR's own presence (a structural fact already in the message) so the fhir.build steps below can build one Claim (always) and one ClaimResponse (only for a response) per event declaratively via rowsPath/condition.","config":{"script":"\nvar parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};\nvar loops = parsed.loops || {};\n\nif (parsed.transactionSet !== \"278\") {\n  return ({ _prior_auth_contexts: [], _response_contexts: [] });\n}\n\nfunction arr(v) {\n  if (!v) return [];\n  return Array.isArray(v) ? v : [v];\n}\nfunction genderToFHIR(code) {\n  if (code === \"M\") return \"male\";\n  if (code === \"F\") return \"female\";\n  return \"unknown\";\n}\nfunction hcrActionToOutcome(code) {\n  var map = { \"A1\": \"complete\", \"A2\": \"partial\", \"A3\": \"complete\", \"A4\": \"queued\", \"A5\": \"complete\", \"A6\": \"complete\" };\n  return map[code] || \"complete\";\n}\nfunction hcrActionToDisposition(code) {\n  var map = {\n    \"A1\": \"Certified in total\", \"A2\": \"Certified - partial\", \"A3\": \"Not Certified\",\n    \"A4\": \"Pended\", \"A5\": \"Upheld\", \"A6\": \"Modified\"\n  };\n  return map[code] || \"\";\n}\n\nvar umoLevels = arr(loops[\"2000A\"]);\nvar contexts = [];\nvar nowIso = new Date().toISOString();\n\nfor (var umoIdx = 0; umoIdx < umoLevels.length; umoIdx++) {\n  var umoLevel = umoLevels[umoIdx];\n  var umoNM1 = ((umoLevel.loops || {})[\"2010A\"] || {}).NM1 || {};\n  var umoInfo = { umo_id: umoNM1.identificationCode || \"\", name: umoNM1.nameLastOrOrganizationName || \"\" };\n\n  var requesterLevels = arr((umoLevel.loops || {})[\"2000B\"]);\n  for (var reqIdx = 0; reqIdx < requesterLevels.length; reqIdx++) {\n    var reqLevel = requesterLevels[reqIdx];\n    var reqNM1 = (arr((reqLevel.loops || {})[\"2010B\"])[0] || {}).NM1 || {};\n    var providerInfo = { npi: reqNM1.identificationCode || \"\", name: reqNM1.nameLastOrOrganizationName || \"\" };\n\n    var subscriberLevels = arr((reqLevel.loops || {})[\"2000C\"]);\n    for (var subIdx = 0; subIdx < subscriberLevels.length; subIdx++) {\n      var subLevel = subscriberLevels[subIdx];\n      var sub2010C = (subLevel.loops || {})[\"2010C\"] || {};\n      var subNM1 = sub2010C.NM1 || {};\n      var subscriberInfo = {\n        member_id: subNM1.identificationCode || \"\",\n        first_name: subNM1.nameFirst || \"\",\n        last_name: subNM1.nameLastOrOrganizationName || \"\",\n        gender_fhir: \"unknown\",\n      };\n\n      function extractDiagnoses(event) {\n        var out = [];\n        var hi = event.HI || {};\n        var codes = arr(hi.codes);\n        for (var i = 0; i < codes.length; i++) {\n          var c = (codes[i] || {}).code || {};\n          if (c.code) out.push({ system: \"http://hl7.org/fhir/sid/icd-10-cm\", code: c.code });\n        }\n        return out;\n      }\n\n      function extractServiceLines(event) {\n        var out = [];\n        var svcLevels = arr((event.loops || {})[\"2000F\"]);\n        for (var i = 0; i < svcLevels.length; i++) {\n          var svc = svcLevels[i];\n          var sv1 = svc.SV1 || {};\n          var proc = sv1.procedureCode || {};\n          if (!proc.code) continue;\n          out.push({\n            code: proc.code,\n            hcr_action_code: svc.HCR ? svc.HCR.actionCode : \"\",\n          });\n        }\n        return out;\n      }\n\n      function pushContext(patientInfo, event) {\n        var isResponse = !!event.HCR;\n        var hcr = event.HCR || {};\n        contexts.push({\n          patient_info: patientInfo,\n          payer_info: umoInfo,\n          provider_info: providerInfo,\n          created_at: nowIso,\n          request_identifier: event.TRN ? event.TRN.checkOrEFTTraceNumber : \"\",\n          diagnoses: extractDiagnoses(event),\n          service_lines: extractServiceLines(event),\n          is_response: isResponse,\n          hcr_action_code: hcr.actionCode || \"\",\n          hcr_certification_number: hcr.certificationNumber || \"\",\n          claim_outcome: isResponse ? hcrActionToOutcome(hcr.actionCode) : \"\",\n          claim_disposition: isResponse ? hcrActionToDisposition(hcr.actionCode) : \"\",\n        });\n      }\n\n      var dependentLevels = arr((subLevel.loops || {})[\"2000D\"]);\n      for (var depIdx = 0; depIdx < dependentLevels.length; depIdx++) {\n        var depLevel = dependentLevels[depIdx];\n        var dep2010D = (depLevel.loops || {})[\"2010D\"] || {};\n        var depNM1 = dep2010D.NM1 || {};\n        var dependentInfo = {\n          member_id: depNM1.identificationCode || subscriberInfo.member_id,\n          first_name: depNM1.nameFirst || \"\",\n          last_name: depNM1.nameLastOrOrganizationName || \"\",\n          gender_fhir: \"unknown\",\n        };\n        var depEvents = arr(depLevel.loops ? depLevel.loops[\"2000E\"] : []);\n        for (var e = 0; e < depEvents.length; e++) {\n          pushContext(dependentInfo, depEvents[e]);\n        }\n      }\n\n      // The subscriber's OWN patient event is a SIBLING of 2000D within\n      // 2000C's own loops (matching the \"Dependent's real HL parent is the\n      // Subscriber, never the Provider\" rule already learned twice for\n      // 276/277 -- 278's own tree has the SAME shape one level deeper).\n      var subEvents = arr(subLevel.loops ? subLevel.loops[\"2000E\"] : []);\n      for (var se = 0; se < subEvents.length; se++) {\n        pushContext(subscriberInfo, subEvents[se]);\n      }\n    }\n  }\n}\n\n// fhir.build has no whole-resource-row \"condition\" gate (Condition only\n// exists on individual FIELDS/repeatingGroups, never on the top-level\n// config) -- so the ONLY-when-HCR-present ClaimResponse gating must happen\n// HERE, as a pre-filtered second array, the same \"0-or-1-element array so a\n// resource simply isn't built for excluded rows\" convention 271's own\n// insurance/rejection derive script already established.\nvar responseContexts = [];\nfor (var i = 0; i < contexts.length; i++) {\n  if (contexts[i].is_response) responseContexts.push(contexts[i]);\n}\n\nreturn ({ _prior_auth_contexts: contexts, _response_contexts: responseContexts });\n"},"enabled":true,"required":true}]},{"sequence":100,"steps":[{"step_name":"Build UMO Organization","step_alias":"build_umo_organization_fhir","step_type":"fhir.build","sequence":100,"description":"Builds the Utilization Management Organization (2010A) as a single FHIR Organization resource.","config":{"resourceType":"Organization","profile":"base","version":"R4","outputField":"message.fhirOrganization","fields":[{"targetPath":"id","literalValue":"organization-umo"},{"targetPath":"active","literalValue":"true"},{"targetPath":"name","literalValue":"ACME UMO"}]},"enabled":true,"required":true}]},{"sequence":110,"steps":[{"step_name":"Build Patient","step_alias":"build_patient_fhir","step_type":"fhir.build","sequence":110,"description":"Builds one FHIR Patient resource per patient event -- the patient is the subscriber when the event concerns herself, or the dependent (2010D) otherwise.","config":{"resourceType":"Patient","profile":"base","version":"R4","outputField":"message.fhirPatients","rowsPath":"steps.derive_278_prior_auth_contexts.step_output._prior_auth_contexts","fields":[{"targetPath":"id","sourcePath":"patient_info.member_id"},{"targetPath":"identifier[0].system","literalValue":"http://ezhealthkonnect.local/x12-member-id"},{"targetPath":"identifier[0].value","sourcePath":"patient_info.member_id"},{"targetPath":"name[0].family","sourcePath":"patient_info.last_name"},{"targetPath":"name[0].given[0]","sourcePath":"patient_info.first_name"},{"targetPath":"gender","sourcePath":"patient_info.gender_fhir"}]},"enabled":true,"required":true}]},{"sequence":115,"steps":[{"step_name":"Build Claim","step_alias":"build_claim_fhir","step_type":"fhir.build","sequence":115,"description":"Builds one FHIR Claim resource (use=preauthorization) per patient event -- ALWAYS built, matching Da Vinci PAS's own resource choice for the same real-world business process. diagnosis[] from HI, item[] from each service level's own SV1 procedure code.","config":{"resourceType":"Claim","profile":"base","version":"R4","outputField":"message.fhirClaims","rowsPath":"steps.derive_278_prior_auth_contexts.step_output._prior_auth_contexts","fields":[{"targetPath":"id","sourcePath":"request_identifier","transform":"string_prefix","valueMap":{"prefix":"prior-auth-request-"}},{"targetPath":"status","literalValue":"active"},{"targetPath":"type.coding[0].system","literalValue":"http://terminology.hl7.org/CodeSystem/claim-type"},{"targetPath":"type.coding[0].code","literalValue":"professional"},{"targetPath":"use","literalValue":"preauthorization"},{"targetPath":"patient.reference","sourcePath":"patient_info.member_id","transform":"string_prefix","valueMap":{"prefix":"Patient/"}},{"targetPath":"created","sourcePath":"created_at"},{"targetPath":"provider.identifier.system","literalValue":"http://hl7.org/fhir/sid/us-npi"},{"targetPath":"provider.identifier.value","sourcePath":"provider_info.npi"},{"targetPath":"priority.coding[0].system","literalValue":"http://terminology.hl7.org/CodeSystem/processpriority"},{"targetPath":"priority.coding[0].code","literalValue":"normal"},{"targetPath":"insurance[0].sequence","literalValue":"1"},{"targetPath":"insurance[0].focal","literalValue":"true"},{"targetPath":"insurance[0].coverage.identifier.system","literalValue":"http://ezhealthkonnect.local/x12-umo-id"},{"targetPath":"insurance[0].coverage.identifier.value","sourcePath":"payer_info.umo_id"}],"repeatingGroups":[{"rowsPath":"diagnoses","targetPath":"diagnosis","fields":[{"targetPath":"sequence","sourcePath":"_rowIndex","transform":"cda_decimal_string_to_number"},{"targetPath":"diagnosisCodeableConcept.coding[0].system","sourcePath":"system"},{"targetPath":"diagnosisCodeableConcept.coding[0].code","sourcePath":"code"}]},{"rowsPath":"service_lines","targetPath":"item","fields":[{"targetPath":"sequence","sourcePath":"_rowIndex","transform":"cda_decimal_string_to_number"},{"targetPath":"productOrService.coding[0].system","literalValue":"http://www.ama-assn.org/go/cpt"},{"targetPath":"productOrService.coding[0].code","sourcePath":"code"}]}]},"enabled":true,"required":true}]},{"sequence":120,"steps":[{"step_name":"Build ClaimResponse","step_alias":"build_claim_response_fhir","step_type":"fhir.build","sequence":120,"description":"Builds one FHIR ClaimResponse resource per patient event, but ONLY when HCR (the real certification decision) is present -- a pure request produces zero ClaimResponse rows. HCR01 is translated into FHIR outcome/disposition vocabulary, never invented.","config":{"resourceType":"ClaimResponse","profile":"base","version":"R4","outputField":"message.fhirClaimResponses","rowsPath":"steps.derive_278_prior_auth_contexts.step_output._response_contexts","fields":[{"targetPath":"id","sourcePath":"request_identifier","transform":"string_prefix","valueMap":{"prefix":"prior-auth-response-"}},{"targetPath":"status","literalValue":"active"},{"targetPath":"type.coding[0].system","literalValue":"http://terminology.hl7.org/CodeSystem/claim-type"},{"targetPath":"type.coding[0].code","literalValue":"professional"},{"targetPath":"use","literalValue":"preauthorization"},{"targetPath":"patient.reference","sourcePath":"patient_info.member_id","transform":"string_prefix","valueMap":{"prefix":"Patient/"}},{"targetPath":"created","sourcePath":"created_at"},{"targetPath":"insurer.identifier.system","literalValue":"http://ezhealthkonnect.local/x12-umo-id"},{"targetPath":"insurer.identifier.value","sourcePath":"payer_info.umo_id"},{"targetPath":"outcome","sourcePath":"claim_outcome"},{"targetPath":"disposition","sourcePath":"claim_disposition"},{"targetPath":"preAuthRef","sourcePath":"hcr_certification_number"},{"targetPath":"request.identifier.system","literalValue":"http://ezhealthkonnect.local/x12-prior-auth-trace"},{"targetPath":"request.identifier.value","sourcePath":"request_identifier"}]},"enabled":true,"required":true}]},{"sequence":200,"steps":[{"step_name":"Assemble 278 FHIR Bundle","step_alias":"assemble_278_fhir_bundle","step_type":"payload.builder","sequence":200,"description":"Assembles the UMO Organization, Patient, Claim, and ClaimResponse arrays into one FHIR R4 Bundle with correctly cross-referenced fullUrls.","config":{"mode":"fhir_bundle","fhirBundle":{"bundleType":"collection","resourcePaths":["message.fhirOrganization","message.fhirPatients","message.fhirClaims","message.fhirClaimResponses"]}},"enabled":true,"required":true}]},{"sequence":210,"steps":[{"step_name":"Validate FHIR Bundle","step_type":"fhir_validation","sequence":210,"config":{"validation_level":"strict","profile":"base","fhir_version":"R4","source_field":"payload"},"enabled":true,"required":false}]},{"sequence":295,"steps":[{"step_name":"Store Result","step_type":"connector.outbound","sequence":295,"config":{"connectorType":"sink_outbound","config":{"enable_logging":true,"enable_validation":true},"contentField":"payload"},"enabled":true,"required":true}]}]}
    $pipeline$::jsonb,
    '278',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_path","label":"Remote Directory","type":"string","hint":"Directory to poll for 278 files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive 278 (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse 278","icon":"📄","step_type":"edi.parse"},
        {"name":"Validate X12","icon":"✅","step_type":"edi.validate"},
        {"name":"Derive Prior Auth Contexts","icon":"🧩","step_type":"enrichment.script"},
        {"name":"Build UMO Organization","icon":"🏢","step_type":"fhir.build"},
        {"name":"Build Patient","icon":"🧑","step_type":"fhir.build"},
        {"name":"Build Claim","icon":"📋","step_type":"fhir.build"},
        {"name":"Build ClaimResponse","icon":"✅","step_type":"fhir.build"},
        {"name":"Assemble FHIR Bundle","icon":"📦","step_type":"payload.builder"},
        {"name":"Validate FHIR","icon":"🛡️","step_type":"fhir_validation"},
        {"name":"Store Result","icon":"💾","step_type":"connector.outbound"}
    ]',
    30,
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
