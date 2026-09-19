-- V265: NCPDP SCRIPT RxFill to FHIR OOB pipeline template
-- Applied: 2026-09-19

-- ============================================================
-- NCPDP SCRIPT RxFill -> FHIR (MedicationDispense + Patient + Organization)
-- TEMPLATE
-- ============================================================
-- FHIR target: MedicationDispense -- a pharmacy's own notification that a
-- prescription was Filled/NotFilled/PartialFill'd, per
-- ncpdp/schemas/script_2017071/transactions/RxFill.json (sourced from the
-- SCRIPT v10.6 XSD -- see that file's own sourceRefs; no free v2017071
-- schema/sample containing RxFill exists). Base FHIR MedicationDispense's
-- own `required` list is only status + medication[x] unconditionally
-- (performer.actor and substitution.wasSubstituted are conditional on
-- those substructures being present at all -- confirmed directly against
-- schemas/fhir/R4/resources/MedicationDispense.gz, not assumed), so a
-- Patient/Organization-only Bundle (no separate Practitioner resource --
-- RxFill's own schema carries no individual dispensing pharmacist name,
-- only the pharmacy itself) is fully spec-conformant.
--
-- FillStatus's 3-way choice (Filled/NotFilled/PartialFill) maps onto FHIR's
-- own medicationdispense-status ValueSet: Filled -> "completed", PartialFill
-- -> "in-progress" (a partial fill is, by definition, not yet complete),
-- NotFilled -> "declined" -- a code-system translation, never an invented
-- fact; the raw outcome type itself always comes straight from the source.
--
-- Config here is transcribed verbatim from
-- services/ncpdp_rxfill_fhir_builder_test.go
-- (TestNCPDPRxFillFHIRBuilder_FilledOutcome_BuildsCleanValidatingBundle).

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
    'NCPDP SCRIPT RxFill to FHIR (SFTP)',
    'ncpdp-rxfill-to-fhir-sftp',
    'Poll an SFTP directory for NCPDP SCRIPT RxFill (pharmacy fill/dispense notification) XML files, parse and validate them, then build and validate a FHIR R4 Bundle containing Organization (pharmacy), Patient, and MedicationDispense (status derived from Filled/PartialFill/NotFilled) resources.',
    'ncpdp',
    null,
    ARRAY['ncpdp','script','rxfill','pharmacy','e-prescribing','medicationdispense','sftp','fhir','dispense'],
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
                    "step_name": "Receive Rx Fill File (SFTP)",
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
                    "step_name": "Parse Rx Fill",
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
                    "step_alias": "build_pharmacy_organization",
                    "step_type": "fhir.build",
                    "sequence": 100,
                    "config": {
                        "resourceType": "Organization",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirPharmacyOrganization",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.pharmacy.identification.ncpdpid", "transform": "string_prefix", "valueMap": {"prefix": "organization-pharmacy-"}},
                            {"targetPath": "active", "literalValue": "true"},
                            {"targetPath": "name", "sourcePath": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.pharmacy.business_name"},
                            {"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-provider-id"},
                            {"targetPath": "identifier[0].value", "sourcePath": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.pharmacy.identification.ncpdpid"}
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
                    "step_alias": "build_patient",
                    "step_type": "fhir.build",
                    "sequence": 105,
                    "config": {
                        "resourceType": "Patient",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirPatient",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "steps.parse_rx_fill.step_output.parsed_ncpdp.header.message_id", "transform": "string_prefix", "valueMap": {"prefix": "patient-"}},
                            {"targetPath": "name[0].family", "sourcePath": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.patient.human_patient.name.last_name"},
                            {"targetPath": "name[0].given[0]", "sourcePath": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.patient.human_patient.name.first_name"},
                            {"targetPath": "gender", "literalValue": "male", "condition": {"field": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.patient.human_patient.gender", "operator": "equals", "value": "M"}},
                            {"targetPath": "gender", "literalValue": "female", "condition": {"field": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.patient.human_patient.gender", "operator": "equals", "value": "F"}},
                            {"targetPath": "gender", "literalValue": "unknown", "condition": {"field": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.patient.human_patient.gender", "operator": "equals", "value": "U"}}
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 110,
                "steps": [{
                    "step_name": "Build Medication Dispense",
                    "step_alias": "build_medication_dispense",
                    "step_type": "fhir.build",
                    "sequence": 110,
                    "config": {
                        "resourceType": "MedicationDispense",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirMedicationDispense",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "steps.parse_rx_fill.step_output.parsed_ncpdp.header.message_id", "transform": "string_prefix", "valueMap": {"prefix": "medicationdispense-"}},
                            {"targetPath": "status", "literalValue": "completed", "condition": {"field": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.filled", "operator": "exists"}},
                            {"targetPath": "status", "literalValue": "in-progress", "condition": {"field": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.partial_fill", "operator": "exists"}},
                            {"targetPath": "status", "literalValue": "declined", "condition": {"field": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.not_filled", "operator": "exists"}},
                            {"targetPath": "medicationCodeableConcept.text", "sourcePath": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.medication_dispensed.drug_description", "fallbackPaths": ["steps.parse_rx_fill.step_output.parsed_ncpdp.body.medication_prescribed.drug_description"]},
                            {"targetPath": "subject.reference", "sourcePath": "steps.parse_rx_fill.step_output.parsed_ncpdp.header.message_id", "transform": "string_prefix", "valueMap": {"prefix": "Patient/patient-"}},
                            {"targetPath": "performer[0].actor.reference", "sourcePath": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.pharmacy.identification.ncpdpid", "transform": "string_prefix", "valueMap": {"prefix": "Organization/organization-pharmacy-"}},
                            {"targetPath": "quantity.value", "sourcePath": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.medication_dispensed.quantity.value", "transform": "cda_decimal_string_to_number"},
                            {"targetPath": "whenHandedOver", "sourcePath": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.medication_dispensed.last_fill_date.date"},
                            {"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-fill-reference-number"},
                            {"targetPath": "identifier[0].value", "sourcePath": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.filled.reference_number"},
                            {"targetPath": "note[0].text", "sourcePath": "steps.parse_rx_fill.step_output.parsed_ncpdp.body.filled.note", "fallbackPaths": [
                                "steps.parse_rx_fill.step_output.parsed_ncpdp.body.partial_fill.note",
                                "steps.parse_rx_fill.step_output.parsed_ncpdp.body.not_filled.note"
                            ]}
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 200,
                "steps": [{
                    "step_name": "Assemble Rx Fill FHIR Bundle",
                    "step_alias": "assemble_rx_fill_fhir_bundle",
                    "step_type": "payload.builder",
                    "sequence": 200,
                    "config": {
                        "mode": "fhir_bundle",
                        "fhirBundle": {
                            "bundleType": "collection",
                            "resourcePaths": ["message.fhirPharmacyOrganization", "message.fhirPatient", "message.fhirMedicationDispense"]
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
    'RxFill',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_dir","label":"Remote Directory","type":"string","hint":"Directory to poll for RxFill XML files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive Rx Fill (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse Rx Fill","icon":"💊","step_type":"ncpdp.parse"},
        {"name":"Validate","icon":"✅","step_type":"ncpdp.validate"},
        {"name":"Build Organization","icon":"🏥","step_type":"fhir.build"},
        {"name":"Build Patient","icon":"🧑","step_type":"fhir.build"},
        {"name":"Build MedicationDispense","icon":"💊","step_type":"fhir.build"},
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
