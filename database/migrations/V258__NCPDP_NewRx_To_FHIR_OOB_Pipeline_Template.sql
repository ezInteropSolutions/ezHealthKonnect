-- V258: NCPDP SCRIPT NewRx (pharmacy e-prescribing) to FHIR OOB pipeline template
-- Applied: 2026-09-14

-- ============================================================
-- NCPDP SCRIPT NewRx -> FHIR (MedicationRequest + Patient + Practitioner +
-- Organization) TEMPLATE
-- ============================================================
-- The first NCPDP SCRIPT (pharmacy e-prescribing) OOB template in this
-- codebase — a genuinely new clinical domain, built the same schema-driven
-- way EDI X12/CDA were (see ncpdp/schema_types.go, ncpdp/parser.go,
-- ncpdp/builder). Unlike every EDI X12 -> FHIR template in this codebase,
-- NO enrichment.script derive step and NO fhir.build rowsPath are needed: a
-- NewRx message carries exactly one patient, one prescriber, one pharmacy,
-- and one medication -- there is no repeating claim/service-line structure
-- to flatten into row contexts first. Every fhir.build field below sources
-- directly from steps.parse_new_rx.step_output.parsed_ncpdp.{header,body}
-- paths (snake_case: every step's own step_output snapshot is passed through
-- models.OutputNormalizer.NormalizeStepOutput before a later step can read
-- it, which recursively snake-cases any key not already snake_case -- found
-- the hard way via a real Test Pipeline run; see this migration's own Go
-- test source for the full explanation). Adding derive-script machinery here
-- would be exactly the kind of premature complexity this project's own
-- standards call out.
--
--   seq 5    connector.inbound   (sftp_inbound — polls an SFTP directory for NewRx XML files;
--                                 NCPDP SCRIPT has no dedicated connector — it rides the SAME
--                                 generic transport connectors every other format uses)
--   seq 20   ncpdp.parse         (raw NCPDP SCRIPT XML -> structured JSON)
--   seq 30   ncpdp.validate      (schema-required-field checks; errors block, never used to
--                                 gate this template's own delivery — required:false, same
--                                 "validation never blocks the store-only terminal" precedent
--                                 EDI's own OOB templates already establish)
--   seq 100  fhir.build          (Organization — pharmacy)
--   seq 105  fhir.build          (Patient)
--   seq 110  fhir.build          (Practitioner — prescriber)
--   seq 115  fhir.build          (MedicationRequest)
--   seq 200  payload.builder     (fhir_bundle mode — assembles the Bundle)
--   seq 210  fhir_validation     (strict — validates the assembled Bundle)
--   seq 295  connector.outbound  (sink_outbound — store-only terminal, no external I/O)
--
-- Config here is transcribed verbatim from
-- services/ncpdp_newrx_fhir_builder_test.go
-- (TestNCPDPNewRxFHIRBuilder_RealSample_BuildsCleanValidatingBundle), which
-- proves this exact chain against a real, unedited SCRIPT v2017071 NewRx
-- sample (sourced from dgoradia/ncpdp's own test fixtures) via the real
-- executors, and asserts the resulting Bundle validates at strict level
-- with zero unexpected errors.
--
-- Named simplifications, stated up front rather than silently left out:
--   - NewRx (as scoped in ncpdp/schemas/script_2017071's own group
--     definitions) carries no patient-identifier field (no MRN/insurance-
--     card-ID segment was modeled -- that lives in NCPDP's own COO/
--     insurance section, deliberately deferred to a later phase).
--     Patient.id and every cross-resource reference are therefore derived
--     from the message's own Header.messageID -- stable within one
--     message, but NOT a real patient identifier and NOT deduplicated
--     across multiple NewRx messages for the same real-world patient, the
--     same "not deduplicated across claims" precedent EDI 837P/837I's own
--     Patient/Coverage mapping already established.
--   - No cda_gender_to_fhir-style transform is used for Patient.gender --
--     that transform expects a CDA-shaped {"code": ...} object, not
--     NCPDP's own bare "M"/"F"/"U" string. Three conditional literal
--     fields map NCPDP's own gender vocabulary directly instead.
--   - Prescriber/pharmacy correlation with MedicationRequest uses real
--     Reference (not just identifier) since both resources ARE built into
--     the same Bundle -- unlike EDI 837's own careTeam[].provider, which
--     uses a logical (identifier-only) reference because no Practitioner
--     resource is built there.
--   - CancelRx/CancelRxResponse/RxChangeRequest/RxChangeResponse -> FHIR
--     mapping is a separate, following effort -- this template covers
--     NewRx only, proving the mechanism first (matching 835's own
--     "core fields, not exhaustive" / "prove the pattern on one
--     transaction type first" precedent).

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
    'NCPDP SCRIPT NewRx to FHIR (SFTP)',
    'ncpdp-newrx-to-fhir-sftp',
    'Poll an SFTP directory for NCPDP SCRIPT NewRx (pharmacy e-prescribing new prescription) XML files, parse and validate them, then build and validate a FHIR R4 Bundle containing Organization (pharmacy), Patient, Practitioner (prescriber), and MedicationRequest resources.',
    'ncpdp',
    null,
    ARRAY['ncpdp','script','newrx','pharmacy','e-prescribing','medicationrequest','sftp','fhir'],
    '💊',
    'advanced',
    'sftp_inbound',
    '{"remote_dir": "/incoming", "file_pattern": "*.xml", "poll_interval_sec": 300, "after_processing": "archive"}',
    'sink_outbound',
    '{"enable_logging": true, "enable_validation": true}',
    $pipeline$
    {
        "execution_groups": [
            {
                "sequence": 5,
                "steps": [{
                    "step_name": "Receive New Rx File (SFTP)",
                    "step_type": "connector.inbound",
                    "sequence": 5,
                    "config": {
                        "connectorType": "sftp_inbound",
                        "config": {"remote_dir": "/incoming", "file_pattern": "*.xml", "poll_interval_sec": 300, "after_processing": "archive"}
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 20,
                "steps": [{
                    "step_name": "Parse New Rx",
                    "step_type": "ncpdp.parse",
                    "sequence": 20,
                    "config": {"sourceField": "raw", "outputField": "parsedNCPDP"},
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 30,
                "steps": [{
                    "step_name": "Validate Against NCPDP SCRIPT Schema",
                    "step_type": "ncpdp.validate",
                    "sequence": 30,
                    "config": {"sourceField": "parsedNCPDP", "outputField": "ncpdpValidation"},
                    "enabled": true,
                    "required": false
                }]
            },
            {
                "sequence": 100,
                "steps": [{
                    "step_name": "Build Pharmacy Organization",
                    "step_alias": "build_pharmacy_organization_fhir",
                    "step_type": "fhir.build",
                    "sequence": 100,
                    "description": "Builds the pharmacy as a FHIR Organization resource, referenced by MedicationRequest.dispenseRequest.performer below.",
                    "config": {
                        "resourceType": "Organization",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirPharmacyOrganization",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.pharmacy.identification.ncpdpid", "transform": "string_prefix", "valueMap": {"prefix": "organization-pharmacy-"}},
                            {"targetPath": "active", "literalValue": "true"},
                            {"targetPath": "name", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.pharmacy.business_name"},
                            {"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-provider-id"},
                            {"targetPath": "identifier[0].value", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.pharmacy.identification.ncpdpid"},
                            {"targetPath": "identifier[1].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
                            {"targetPath": "identifier[1].value", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.pharmacy.identification.npi"},
                            {"targetPath": "address[0].line[0]", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.pharmacy.address.address_line1"},
                            {"targetPath": "address[0].city", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.pharmacy.address.city"},
                            {"targetPath": "address[0].state", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.pharmacy.address.state_province"},
                            {"targetPath": "address[0].postalCode", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.pharmacy.address.postal_code"},
                            {"targetPath": "telecom[0].system", "literalValue": "phone"},
                            {"targetPath": "telecom[0].value", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.pharmacy.communication_numbers.primary_telephone.number"}
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
                    "description": "Builds the patient as a FHIR Patient resource. id is derived from the message's own Header.messageID (see this template's own named-simplification note above) since NewRx, as scoped, carries no patient-identifier field.",
                    "config": {
                        "resourceType": "Patient",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirPatient",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.header.message_id", "transform": "string_prefix", "valueMap": {"prefix": "patient-"}},
                            {"targetPath": "name[0].family", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.patient.human_patient.name.last_name"},
                            {"targetPath": "name[0].given[0]", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.patient.human_patient.name.first_name"},
                            {"targetPath": "gender", "literalValue": "male", "condition": {"field": "steps.parse_new_rx.step_output.parsed_ncpdp.body.patient.human_patient.gender", "operator": "equals", "value": "M"}},
                            {"targetPath": "gender", "literalValue": "female", "condition": {"field": "steps.parse_new_rx.step_output.parsed_ncpdp.body.patient.human_patient.gender", "operator": "equals", "value": "F"}},
                            {"targetPath": "gender", "literalValue": "unknown", "condition": {"field": "steps.parse_new_rx.step_output.parsed_ncpdp.body.patient.human_patient.gender", "operator": "equals", "value": "U"}},
                            {"targetPath": "birthDate", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.patient.human_patient.date_of_birth.date"},
                            {"targetPath": "address[0].line[0]", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.patient.human_patient.address.address_line1"},
                            {"targetPath": "address[0].city", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.patient.human_patient.address.city"},
                            {"targetPath": "address[0].state", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.patient.human_patient.address.state_province"},
                            {"targetPath": "address[0].postalCode", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.patient.human_patient.address.postal_code"},
                            {"targetPath": "telecom[0].system", "literalValue": "phone"},
                            {"targetPath": "telecom[0].value", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.patient.human_patient.communication_numbers.primary_telephone.number"}
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 110,
                "steps": [{
                    "step_name": "Build Prescriber Practitioner",
                    "step_alias": "build_practitioner_fhir",
                    "step_type": "fhir.build",
                    "sequence": 110,
                    "description": "Builds the prescriber as a FHIR Practitioner resource, referenced by MedicationRequest.requester below.",
                    "config": {
                        "resourceType": "Practitioner",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirPractitioner",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.identification.npi", "transform": "string_prefix", "valueMap": {"prefix": "practitioner-"}},
                            {"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
                            {"targetPath": "identifier[0].value", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.identification.npi"},
                            {"targetPath": "identifier[1].system", "literalValue": "http://ezhealthkonnect.local/dea-number"},
                            {"targetPath": "identifier[1].value", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.identification.dea_number"},
                            {"targetPath": "name[0].family", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.name.last_name"},
                            {"targetPath": "name[0].given[0]", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.name.first_name"},
                            {"targetPath": "name[0].suffix[0]", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.name.suffix"},
                            {"targetPath": "address[0].line[0]", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.address.address_line1"},
                            {"targetPath": "address[0].city", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.address.city"},
                            {"targetPath": "address[0].state", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.address.state_province"},
                            {"targetPath": "address[0].postalCode", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.address.postal_code"},
                            {"targetPath": "telecom[0].system", "literalValue": "phone"},
                            {"targetPath": "telecom[0].value", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.communication_numbers.primary_telephone.number"}
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 115,
                "steps": [{
                    "step_name": "Build Medication Request",
                    "step_alias": "build_medication_request_fhir",
                    "step_type": "fhir.build",
                    "sequence": 115,
                    "description": "Builds the prescribed medication as a FHIR MedicationRequest resource, referencing the Patient/Practitioner/Organization resources built above.",
                    "config": {
                        "resourceType": "MedicationRequest",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirMedicationRequest",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.header.message_id", "transform": "string_prefix", "valueMap": {"prefix": "medicationrequest-"}},
                            {"targetPath": "status", "literalValue": "active"},
                            {"targetPath": "intent", "literalValue": "order"},
                            {"targetPath": "medicationCodeableConcept.text", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.medication_prescribed.drug_description"},
                            {"targetPath": "medicationCodeableConcept.coding[0].system", "literalValue": "http://hl7.org/fhir/sid/ndc"},
                            {"targetPath": "medicationCodeableConcept.coding[0].code", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.medication_prescribed.drug_coded.product_code.code"},
                            {"targetPath": "subject.reference", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.header.message_id", "transform": "string_prefix", "valueMap": {"prefix": "Patient/patient-"}},
                            {"targetPath": "requester.reference", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.identification.npi", "transform": "string_prefix", "valueMap": {"prefix": "Practitioner/practitioner-"}},
                            {"targetPath": "authoredOn", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.medication_prescribed.written_date.date"},
                            {"targetPath": "dosageInstruction[0].text", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.medication_prescribed.sig.sig_text"},
                            {"targetPath": "dispenseRequest.quantity.value", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.medication_prescribed.quantity.value", "transform": "cda_decimal_string_to_number"},
                            {"targetPath": "dispenseRequest.numberOfRepeatsAllowed", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.medication_prescribed.number_of_refills", "transform": "cda_decimal_string_to_number"},
                            {"targetPath": "dispenseRequest.performer.reference", "sourcePath": "steps.parse_new_rx.step_output.parsed_ncpdp.body.pharmacy.identification.ncpdpid", "transform": "string_prefix", "valueMap": {"prefix": "Organization/organization-pharmacy-"}}
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 200,
                "steps": [{
                    "step_name": "Assemble New Rx FHIR Bundle",
                    "step_alias": "assemble_new_rx_fhir_bundle",
                    "step_type": "payload.builder",
                    "sequence": 200,
                    "description": "Assembles the Organization/Patient/Practitioner/MedicationRequest resources built above into one FHIR R4 Bundle.",
                    "config": {
                        "mode": "fhir_bundle",
                        "fhirBundle": {
                            "bundleType": "collection",
                            "resourcePaths": ["message.fhirPharmacyOrganization", "message.fhirPatient", "message.fhirPractitioner", "message.fhirMedicationRequest"]
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
    'NewRx',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_dir","label":"Remote Directory","type":"string","hint":"Directory to poll for NewRx XML files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive New Rx (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse New Rx","icon":"💊","step_type":"ncpdp.parse"},
        {"name":"Validate","icon":"✅","step_type":"ncpdp.validate"},
        {"name":"Build Organization","icon":"🏥","step_type":"fhir.build"},
        {"name":"Build Patient","icon":"🧑","step_type":"fhir.build"},
        {"name":"Build Practitioner","icon":"👨‍⚕️","step_type":"fhir.build"},
        {"name":"Build MedicationRequest","icon":"💊","step_type":"fhir.build"},
        {"name":"Assemble FHIR Bundle","icon":"📦","step_type":"payload.builder"},
        {"name":"Validate FHIR","icon":"🛡️","step_type":"fhir_validation"},
        {"name":"Store Result","icon":"💾","step_type":"connector.outbound"}
    ]',
    15,
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
