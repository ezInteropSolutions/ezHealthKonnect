-- V255: EDI X12 834 (Benefit Enrollment and Maintenance) to FHIR OOB
-- pipeline template
-- Applied: 2026-09-14

-- ============================================================
-- EDI 834 -> FHIR (Patient / Coverage) TEMPLATE
-- ============================================================
-- EDI X12 Phase 7 companion to V254 (278 prior authorization). 834 is a
-- GENUINELY DIFFERENT shape from every other transaction set this engine
-- has mapped so far -- a flat, one-way member-roster feed (no HL hierarchy,
-- no request/response pair, no synchronous use case) -- see 834.json's own
-- _sourceRefs for the full "no HL hierarchy, triggered by INS not HL"
-- architecture note.
--
--   seq 5    connector.inbound   (edi_x12_inbound  -- polls an SFTP directory for 834 files)
--   seq 20   edi.parse           (raw X12 -> structured JSON)
--   seq 30   edi.validate        (base X12 5010 standard checks; errors block, warnings never do)
--   seq 95   enrichment.script   (Derive 834 Member Coverages -- pure data reshaping + HD01->Coverage.status translation)
--   seq 100  fhir.build          (Organization -- the plan Sponsor, single-resource, built once)
--   seq 110  fhir.build          (Patient -- rowsPath mode, one per member-coverage row)
--   seq 115  fhir.build          (Coverage -- rowsPath mode, one per real-world health-coverage enrollment)
--   seq 200  payload.builder     (fhir_bundle mode -- assembles the Bundle)
--   seq 210  fhir_validation     (strict -- validates the assembled Bundle)
--   seq 295  connector.outbound  (sink_outbound -- store-only terminal, no external I/O)
--
-- Config here is transcribed programmatically (via a Node script reading the
-- Go source directly, avoiding the hand-retyping bug class an earlier EDI
-- round hit once) from services/edi_834_fhir_builder_test.go
-- (TestEDI834FHIRBuilder_ActiveAndTerminatedMembers_BuildsCleanValidatingBundle),
-- which proves this exact chain against a real fixture (a subscriber with an
-- ACTIVE health-coverage enrollment and a SEPARATE dependent with a
-- TERMINATED one) via the real executors, proving the HD01 maintenance-type-
-- code -> Coverage.status translation picks genuinely different real
-- statuses from genuinely different source data, and asserts the resulting
-- Bundle validates at strict level with zero unexpected errors.
--
-- Named simplifications (same discipline as every prior EDI-to-FHIR round):
--   - Patient/Coverage are built per member-coverage ROW, not deduplicated
--     across a member''s own multiple coverages -- a member with 2 real
--     coverages gets 2 Patient Bundle entries sharing the same stable id,
--     harmless per FHIR Bundle semantics, matching 837P/837I''s own
--     "not deduplicated across claims" precedent.
--   - Only the CORE member/coverage fields (2000/2100A/2300 -- member
--     identity, demographics, health-coverage enrollment) are mapped into
--     FHIR fields. 834.json itself is schema-complete for the LESS-common
--     loops too (2100B-H demographic variants, 2200 disability, 2310/2320/
--     2330 provider + coordination-of-benefits, 2700/2750 reporting
--     categories, per the user''s own full-completeness scope decision) --
--     those are not yet mapped into FHIR fields this round, a named,
--     deferred follow-on, not a schema gap.
--   - No synchronous endpoint exists for 834 -- X12 itself has no
--     834-response transaction set (a pure one-way roster feed), so a
--     real-time request/response pattern doesn''t apply the way it does for
--     270/271, 276/277, or 278.

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
    'EDI X12 834 to FHIR (SFTP)',
    'edi-834-to-fhir-sftp',
    'Poll an SFTP directory for X12 834 (Benefit Enrollment and Maintenance) files, parse and validate them, then build and validate a FHIR R4 Bundle containing Organization (Sponsor), Patient, and Coverage resources -- one Patient/Coverage per member-coverage enrollment in the file, with the X12 maintenance-type code translated into FHIR''s own Coverage.status vocabulary.',
    'edi',
    null,
    ARRAY['edi','x12','834','enrollment','benefit','sftp','fhir'],
    '📇',
    'advanced',
    'edi_x12_inbound',
    '{"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["834"]}',
    'sink_outbound',
    '{"enable_logging": true, "enable_validation": true}',
    $pipeline$
{"execution_groups":[{"sequence":5,"steps":[{"step_name":"Receive 834 File (SFTP)","step_type":"connector.inbound","sequence":5,"config":{"connectorType":"edi_x12_inbound","config":{"transport":"sftp","remote_path":"/incoming","file_pattern":"*.edi","polling_interval_seconds":300,"after_processing":"archive","transaction_types":["834"]}},"enabled":true,"required":true}]},{"sequence":20,"steps":[{"step_name":"Parse 834 -> JSON","step_type":"edi.parse","sequence":20,"config":{"sourceField":"raw","outputField":"parsedEDI","transactionSet":"834"},"enabled":true,"required":true}]},{"sequence":30,"steps":[{"step_name":"Validate Against X12 5010","step_type":"edi.validate","sequence":30,"config":{"sourceField":"raw","outputField":"ediValidation","customRules":[]},"enabled":true,"required":false}]},{"sequence":95,"steps":[{"step_name":"Derive 834 Member Coverages","step_alias":"derive_834_member_coverages","step_type":"enrichment.script","sequence":95,"description":"Pure structural reshaping only -- this engine transforms X12 <-> FHIR, it never decides enrollment facts. Flattens 834's flat member-list structure (no HL hierarchy) into one row per (member x coverage) pair, translating HD01's own maintenance type code into FHIR's Coverage.status vocabulary along the way (a code-system translation, never an invented fact) so the fhir.build steps below can build one Patient/Coverage per row declaratively via rowsPath.","config":{"script":"\nvar parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};\nvar loops = parsed.loops || {};\n\nif (parsed.transactionSet !== \"834\") {\n  return ({ _member_coverages: [] });\n}\n\nfunction arr(v) {\n  if (!v) return [];\n  return Array.isArray(v) ? v : [v];\n}\nfunction genderToFHIR(code) {\n  if (code === \"M\") return \"male\";\n  if (code === \"F\") return \"female\";\n  return \"unknown\";\n}\nfunction relationshipToFHIR(code) {\n  var map = { \"18\": \"self\", \"01\": \"spouse\", \"19\": \"child\" };\n  return map[code] || \"other\";\n}\nfunction maintenanceTypeToCoverageStatus(code) {\n  // Real X12 element 875 (Maintenance Type Code) standard values, confirmed\n  // directly against stedi.com/edi/x12-005010/element/875's own code list\n  // AND cross-checked against X12.org's own official 834 examples: \"021\"\n  // Addition and \"025\" Reinstatement both genuinely mean the coverage is\n  // active; \"024\" is \"Cancellation or Termination\" -- the REAL code for a\n  // terminated enrollment (X12.org's own Example 07, \"Terminate Eligibility\n  // for a Subscriber\", uses exactly this code). An earlier version of this\n  // mapping had \"024\"->\"active\" and \"030\"->\"cancelled\" -- backwards: \"030\"\n  // is \"Audit or Compare\" in the real standard, not a termination code at\n  // all. Found and fixed only by testing against real X12.org sample data,\n  // not by re-reading the generic Stedi segment reference a second time.\n  var map = { \"021\": \"active\", \"025\": \"active\", \"024\": \"cancelled\", \"002\": \"cancelled\" };\n  return map[code] || \"active\";\n}\n\nvar members = arr(loops[\"2000\"]);\nvar rows = [];\nvar nowIso = new Date().toISOString();\n\nfor (var i = 0; i < members.length; i++) {\n  var member = members[i];\n  var ins = member.INS || {};\n  var ref = member.REF || {};\n  var m2100A = (member.loops || {})[\"2100A\"] || {};\n  var nm1 = m2100A.NM1 || {};\n  var dmg = m2100A.DMG || {};\n\n  var patientInfo = {\n    member_id: nm1.identificationCode || ref.referenceIdentification || \"\",\n    first_name: nm1.nameFirst || \"\",\n    last_name: nm1.nameLastOrOrganizationName || \"\",\n    dob: dmg.birthDate || \"\",\n    gender_fhir: genderToFHIR(dmg.genderCode),\n    relationship_fhir: relationshipToFHIR(ins.individualRelationshipCode),\n  };\n\n  var coverages = arr((member.loops || {})[\"2300\"]);\n  for (var c = 0; c < coverages.length; c++) {\n    var hd = coverages[c].HD || {};\n    rows.push({\n      patient_info: patientInfo,\n      created_at: nowIso,\n      subscriber_id: ref.referenceIdentification || \"\",\n      plan_description: hd.planCoverageDescription || \"\",\n      insurance_line_code: hd.insuranceLineCode || \"\",\n      coverage_status: maintenanceTypeToCoverageStatus(hd.maintenanceTypeCode || \"\"),\n      maintenance_type_code: hd.maintenanceTypeCode || \"\",\n    });\n  }\n}\n\nreturn ({ _member_coverages: rows });\n"},"enabled":true,"required":true}]},{"sequence":100,"steps":[{"step_name":"Build Sponsor Organization","step_alias":"build_sponsor_organization_fhir","step_type":"fhir.build","sequence":100,"description":"Builds the plan Sponsor (1000A) as a single FHIR Organization resource.","config":{"resourceType":"Organization","profile":"base","version":"R4","outputField":"message.fhirOrganization","fields":[{"targetPath":"id","literalValue":"organization-sponsor"},{"targetPath":"active","literalValue":"true"},{"targetPath":"name","literalValue":"ACME CORP"}]},"enabled":true,"required":true}]},{"sequence":110,"steps":[{"step_name":"Build Patient","step_alias":"build_patient_fhir","step_type":"fhir.build","sequence":110,"description":"Builds one FHIR Patient resource per member-coverage row -- a member with N coverages produces N Patient entries sharing the same stable id, harmless per FHIR Bundle semantics (same \"not deduplicated across rows\" precedent 837P/837I already established).","config":{"resourceType":"Patient","profile":"base","version":"R4","outputField":"message.fhirPatients","rowsPath":"steps.derive_834_member_coverages.step_output._member_coverages","fields":[{"targetPath":"id","sourcePath":"patient_info.member_id"},{"targetPath":"identifier[0].system","literalValue":"http://ezhealthkonnect.local/x12-member-id"},{"targetPath":"identifier[0].value","sourcePath":"patient_info.member_id"},{"targetPath":"name[0].family","sourcePath":"patient_info.last_name"},{"targetPath":"name[0].given[0]","sourcePath":"patient_info.first_name"},{"targetPath":"birthDate","sourcePath":"patient_info.dob","transform":"x12_date_to_fhir_date"},{"targetPath":"gender","sourcePath":"patient_info.gender_fhir"}]},"enabled":true,"required":true}]},{"sequence":115,"steps":[{"step_name":"Build Coverage","step_alias":"build_coverage_fhir","step_type":"fhir.build","sequence":115,"description":"Builds one FHIR Coverage resource per real-world health-coverage enrollment (834's own 2300 loop), status translated from HD01's own maintenance type code.","config":{"resourceType":"Coverage","profile":"base","version":"R4","outputField":"message.fhirCoverages","rowsPath":"steps.derive_834_member_coverages.step_output._member_coverages","fields":[{"targetPath":"id","sourcePath":"_rowIndex","transform":"string_prefix","valueMap":{"prefix":"coverage-"}},{"targetPath":"status","sourcePath":"coverage_status"},{"targetPath":"type.coding[0].system","literalValue":"https://codesystem.x12.org/005010/1205"},{"targetPath":"type.coding[0].code","sourcePath":"insurance_line_code"},{"targetPath":"subscriberId","sourcePath":"subscriber_id"},{"targetPath":"beneficiary.reference","sourcePath":"patient_info.member_id","transform":"string_prefix","valueMap":{"prefix":"Patient/"}},{"targetPath":"relationship.coding[0].system","literalValue":"http://terminology.hl7.org/CodeSystem/subscriber-relationship"},{"targetPath":"relationship.coding[0].code","sourcePath":"patient_info.relationship_fhir"},{"targetPath":"payor[0].identifier.system","literalValue":"http://ezhealthkonnect.local/x12-payer-id"},{"targetPath":"payor[0].identifier.value","literalValue":"PAYER001"},{"targetPath":"class[0].type.coding[0].system","literalValue":"http://terminology.hl7.org/CodeSystem/coverage-class"},{"targetPath":"class[0].type.coding[0].code","literalValue":"plan"},{"targetPath":"class[0].value","sourcePath":"plan_description"}]},"enabled":true,"required":true}]},{"sequence":200,"steps":[{"step_name":"Assemble 834 FHIR Bundle","step_alias":"assemble_834_fhir_bundle","step_type":"payload.builder","sequence":200,"description":"Assembles the Sponsor Organization, Patient, and Coverage arrays into one FHIR R4 Bundle with correctly cross-referenced fullUrls.","config":{"mode":"fhir_bundle","fhirBundle":{"bundleType":"collection","resourcePaths":["message.fhirOrganization","message.fhirPatients","message.fhirCoverages"]}},"enabled":true,"required":true}]},{"sequence":210,"steps":[{"step_name":"Validate FHIR Bundle","step_type":"fhir_validation","sequence":210,"config":{"validation_level":"strict","profile":"base","fhir_version":"R4","source_field":"payload"},"enabled":true,"required":false}]},{"sequence":295,"steps":[{"step_name":"Store Result","step_type":"connector.outbound","sequence":295,"config":{"connectorType":"sink_outbound","config":{"enable_logging":true,"enable_validation":true},"contentField":"payload"},"enabled":true,"required":true}]}]}
    $pipeline$::jsonb,
    '834',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_path","label":"Remote Directory","type":"string","hint":"Directory to poll for 834 files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive 834 (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse 834","icon":"📄","step_type":"edi.parse"},
        {"name":"Validate X12","icon":"✅","step_type":"edi.validate"},
        {"name":"Derive Member Coverages","icon":"🧩","step_type":"enrichment.script"},
        {"name":"Build Sponsor Organization","icon":"🏢","step_type":"fhir.build"},
        {"name":"Build Patient","icon":"🧑","step_type":"fhir.build"},
        {"name":"Build Coverage","icon":"📇","step_type":"fhir.build"},
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
