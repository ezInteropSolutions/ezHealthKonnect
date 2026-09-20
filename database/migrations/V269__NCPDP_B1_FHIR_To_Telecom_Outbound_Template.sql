-- V269: FHIR MedicationDispense to NCPDP Telecom D.0 B1 OUTBOUND pipeline template
-- Applied: 2026-09-20

-- ============================================================
-- FHIR MedicationDispense -> NCPDP Telecom D.0 B1 Claim Billing (OUTBOUND) TEMPLATE
-- ============================================================
-- The D.0 counterpart to V268 (see that migration's own header comment for
-- the full "why this matters" rationale — every prior D.0 template, V266/
-- V267, goes wire-IN -> FHIR-OUT). A drug gets dispensed at a pharmacy -> a
-- FHIR MedicationDispense is created (the SAME resource this codebase's own
-- RxFill inbound mapping already targets, not a new invention) ->
-- ezHealthKonnect builds a real B1 claim and sends it to a payer/switch for
-- adjudication.
--
-- Honest scope note: MedicationDispense carries no insurance/coverage data
-- (that lives on a separate Coverage resource in a real system) —
-- Insurance.cardholderId is approximated here from subject.identifier.value
-- (the patient's OWN identifier, e.g. an MRN) — a real but imperfect proxy,
-- documented as such, not presented as a true cardholder-ID lookup.
-- binNumber/processorControlNumber/serviceProviderIdQualifier are payer-
-- enrollment-level constants no FHIR resource could ever carry — supplied as
-- literal config placeholders the deploying user replaces.
--
-- Patient (patientLastName/patientFirstName, from subject.display, the same
-- split-name technique V268's own NewRx template uses) and PharmacyProvider
-- (providerId from the SAME pharmacy_npi already used for the header's own
-- serviceProviderId) ARE populated — both cleanly derivable from data
-- already on the resource. Pricing is NOT populated and is a real, permanent
-- gap for THIS trigger resource specifically: ingredientCostSubmitted/
-- usualAndCustomaryCharge/grossAmountDue are the pharmacy's OWN point-of-sale
-- cost/pricing data, which FHIR's MedicationDispense does not model at all
-- (it's a billing concern, not a clinical dispensing fact) — fabricating a
-- $0.00 placeholder would be actively misleading to a real payer, not a
-- harmless default, so this is left honestly absent rather than guessed. A
-- real deployment needing complete B1 claims would add pricing data via an
-- enrichment/lookup step against the pharmacy's own POS system before
-- ncpdptelecom.map_to_canonical.
--
--   seq 5    connector.inbound      (http_fhir_inbound — a FHIR MedicationDispense POSTed here
--                                    triggers the pipeline, fields flat at root for this first step)
--   seq 20   enrichment.script      (Derive B1 Outbound Fields — reshapes the raw FHIR resource;
--                                    every later step addresses ONLY this script's own
--                                    steps.<alias>.step_output, never the raw FHIR fields directly)
--   seq 30   ncpdptelecom.map_to_canonical (builds canonical B1 header/transmissionGroup/
--                                    transactionGroups JSON from the derived fields)
--   seq 35   ncpdptelecom.validate  (checks the canonical JSON's own required field/segment
--                                    completeness before build — non-blocking, reports the
--                                    Pricing gap named above rather than silently hiding it)
--   seq 40   ncpdptelecom.build     (canonical JSON -> a real B1 wire transmission)
--   seq 295  connector.outbound     (http_outbound — a REAL send, not sink_outbound; "endpoint" is
--                                    a placeholder the deploying user replaces with their real
--                                    payer/switch destination)
--
-- Config here is transcribed verbatim from
-- services/executors/transform/b1_outbound_test.go
-- (TestB1Outbound_FromFHIRMedicationDispense_BuildsValidB1Transmission), which
-- proves this exact chain via the real executors and asserts the built
-- transmission re-parses cleanly through ncpdptelecom.ParseTransmission.
--
-- A real, generic engine bug was found and fixed building this template's own
-- Go test: ncpdptelecom.Format() (ncpdptelecom/datatypes.go) only accepted
-- native Go numeric types for N/R/RO fields, never a numeric-looking STRING —
-- but ncpdptelecom.map_to_canonical's entire mapping mechanism (sourcePath/
-- literalValue resolution) is string-based by design, the same convention
-- edi.map_to_canonical/ncpdp.map_to_canonical already use. Every prior D.0
-- Go test built its canonical JSON directly with real float64 values, so this
-- gap was invisible until this outbound direction — the first thing in this
-- codebase to actually exercise ncpdptelecom.build via the string-based
-- mapping layer — hit it. Fixed generically in asFloat(), not routed around.
--
-- step_alias note: "Derive B1 Outbound Fields" normalizes (NormalizeKey) to
-- "derive_b1_outbound_fields" — verified by hand-tracing NormalizeKey's own
-- behavior before writing this migration, per this codebase's own
-- twice-independently-rediscovered steps.<key>-addressing gotcha.

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
    'FHIR MedicationDispense to NCPDP D.0 B1 Claim (Outbound)',
    'fhir-medicationdispense-to-b1-outbound',
    'Receive a FHIR R4 MedicationDispense over HTTP, derive the claim details, and build+send a real NCPDP Telecommunication D.0 B1 pharmacy claim to a payer or switch endpoint.',
    'ncpdp',
    null,
    ARRAY['ncpdp','telecom','d0','b1','pharmacy','claim','medicationdispense','fhir','outbound','http'],
    '📤',
    'advanced',
    'http_fhir_inbound',
    '{"port": 9611, "basePath": "/fhir/r4", "fhirVersion": "R4"}',
    'http_outbound',
    '{"url": "https://REPLACE_WITH_PAYER_SWITCH_ENDPOINT/b1", "method": "POST"}',
    $pipeline$
    {
        "execution_groups": [
            {
                "sequence": 5,
                "steps": [{
                    "step_name": "Receive Medication Dispense (FHIR)",
                    "step_type": "connector.inbound",
                    "sequence": 5,
                    "config": {
                        "connectorType": "http_fhir_inbound",
                        "config": {"port": 9611, "basePath": "/fhir/r4", "fhirVersion": "R4"}
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 20,
                "steps": [{
                    "step_name": "Derive B1 Outbound Fields",
                    "step_alias": "derive_b1_outbound_fields",
                    "step_type": "enrichment.script",
                    "sequence": 20,
                    "description": "Reshapes the raw FHIR MedicationDispense (subject, performer[0].actor, medicationCodeableConcept, quantity, daysSupply, whenHandedOver) into the flat, snake_case fields ncpdptelecom.map_to_canonical below addresses.",
                    "config": {
                        "script": "\nvar msg = input.resourceType ? input : ((input.message && input.message.resourceType) ? input.message : input);\nvar subj = msg.subject || {};\nvar performerArr = msg.performer || [];\nvar performer = performerArr.length > 0 ? performerArr[0] : {};\nvar actor = performer.actor || {};\nvar med = msg.medicationCodeableConcept || {};\nvar coding = (med.coding && med.coding.length > 0) ? med.coding[0] : {};\nvar quantity = msg.quantity || {};\nvar daysSupply = msg.daysSupply || {};\nvar identArr = msg.identifier || [];\nvar ident = identArr.length > 0 ? identArr[0] : {};\n\nvar whenHandedOver = msg.whenHandedOver || msg.whenPrepared || \"\";\nvar dateOnly = whenHandedOver ? whenHandedOver.substring(0, 10) : \"\";\n\nvar quantityValue = \"\";\nif (quantity.value !== undefined && quantity.value !== null) { quantityValue = String(quantity.value); }\nvar daysSupplyValue = \"\";\nif (daysSupply.value !== undefined && daysSupply.value !== null) { daysSupplyValue = String(Math.round(daysSupply.value)); }\n\nfunction splitDisplayName(display) {\n    if (!display) return { first: \"\", last: \"\" };\n    var parts = display.split(\" \").filter(function(p) { return p.length > 0; });\n    if (parts.length === 0) return { first: \"\", last: \"\" };\n    if (parts.length === 1) return { first: \"\", last: parts[0] };\n    return { first: parts.slice(0, parts.length - 1).join(\" \"), last: parts[parts.length - 1] };\n}\nvar patientName = splitDisplayName(subj.display);\n\n({\n    prescription_reference_number: ident.value || \"\",\n    cardholder_id: (subj.identifier && subj.identifier.value) || \"\",\n    patient_first_name: patientName.first,\n    patient_last_name: patientName.last,\n    ndc_code: coding.code || \"\",\n    quantity_dispensed: quantityValue,\n    days_supply: daysSupplyValue,\n    date_of_service: dateOnly,\n    pharmacy_npi: (actor.identifier && actor.identifier.value) || \"\"\n});\n"
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 30,
                "steps": [{
                    "step_name": "Map to Canonical B1",
                    "step_type": "ncpdptelecom.map_to_canonical",
                    "sequence": 30,
                    "description": "Builds canonical B1 header/transmissionGroup/transactionGroups JSON from the derive step's own output. binNumber/serviceProviderIdQualifier are payer-enrollment-level constants no FHIR resource could ever carry — placeholders the deploying user replaces via config.",
                    "config": {
                        "outputField": "parsedTelecom",
                        "transactionCode": "B1",
                        "direction": "request",
                        "headerFields": [
                            {"fieldKey": "binNumber", "literalValue": "999999"},
                            {"fieldKey": "transactionCount", "literalValue": "1"},
                            {"fieldKey": "serviceProviderIdQualifier", "literalValue": "01"},
                            {"fieldKey": "serviceProviderId", "sourcePath": "steps.derive_b1_outbound_fields.step_output.pharmacy_npi"},
                            {"fieldKey": "dateOfService", "sourcePath": "steps.derive_b1_outbound_fields.step_output.date_of_service", "transform": "date_to_x12"}
                        ],
                        "transmissionGroupSegments": [
                            {
                                "segmentKey": "Insurance",
                                "fields": [
                                    {"fieldKey": "cardholderId", "sourcePath": "steps.derive_b1_outbound_fields.step_output.cardholder_id"}
                                ]
                            },
                            {
                                "segmentKey": "Patient",
                                "fields": [
                                    {"fieldKey": "patientLastName", "sourcePath": "steps.derive_b1_outbound_fields.step_output.patient_last_name"},
                                    {"fieldKey": "patientFirstName", "sourcePath": "steps.derive_b1_outbound_fields.step_output.patient_first_name"}
                                ]
                            },
                            {
                                "segmentKey": "PharmacyProvider",
                                "fields": [
                                    {"fieldKey": "providerIdQualifier", "literalValue": "01"},
                                    {"fieldKey": "providerId", "sourcePath": "steps.derive_b1_outbound_fields.step_output.pharmacy_npi"}
                                ]
                            }
                        ],
                        "transactionGroupSegments": [
                            {
                                "segmentKey": "Claim",
                                "fields": [
                                    {"fieldKey": "prescriptionReferenceNumberQualifier", "literalValue": "1"},
                                    {"fieldKey": "prescriptionReferenceNumber", "sourcePath": "steps.derive_b1_outbound_fields.step_output.prescription_reference_number"},
                                    {"fieldKey": "productServiceIdQualifier", "literalValue": "03"},
                                    {"fieldKey": "productServiceId", "sourcePath": "steps.derive_b1_outbound_fields.step_output.ndc_code"},
                                    {"fieldKey": "quantityDispensed", "sourcePath": "steps.derive_b1_outbound_fields.step_output.quantity_dispensed"},
                                    {"fieldKey": "daysSupply", "sourcePath": "steps.derive_b1_outbound_fields.step_output.days_supply"},
                                    {"fieldKey": "fillNumber", "literalValue": "01"}
                                ]
                            }
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 35,
                "steps": [{
                    "step_name": "Validate Canonical B1",
                    "step_type": "ncpdptelecom.validate",
                    "sequence": 35,
                    "description": "Checks the canonical B1 JSON's own required field/segment completeness BEFORE building the transmission — catches a missing cardholder ID or similar gap early rather than sending a malformed claim out. Non-blocking (required:false), matching this codebase's own established 'validation is advisory, never blocks delivery' precedent.",
                    "config": {"sourceField": "parsedTelecom", "outputField": "telecomValidation"},
                    "enabled": true,
                    "required": false
                }]
            },
            {
                "sequence": 40,
                "steps": [{
                    "step_name": "Build B1 Transmission",
                    "step_type": "ncpdptelecom.build",
                    "sequence": 40,
                    "config": {"sourceField": "parsedTelecom", "transactionCode": "B1", "direction": "request", "outputField": "ncpdpTelecom"},
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 295,
                "steps": [{
                    "step_name": "Send B1 Claim to Payer",
                    "step_type": "connector.outbound",
                    "sequence": 295,
                    "config": {
                        "connectorType": "http_outbound",
                        "config": {"url": "https://REPLACE_WITH_PAYER_SWITCH_ENDPOINT/b1", "method": "POST"},
                        "contentField": "ncpdpTelecom",
                        "contentType": "application/x-ncpdp-telecom-d0"
                    },
                    "enabled": true,
                    "required": true
                }]
            }
        ]
    }
    $pipeline$::jsonb,
    'MedicationDispense',
    '[
        {"section":"source","field":"port","label":"Listener Port","type":"number","hint":"Port this pipeline listens on for incoming FHIR MedicationDispense resources","required":true},
        {"section":"target","field":"url","label":"Payer/Switch Endpoint URL","type":"string","hint":"Real destination the built B1 transmission is POSTed to","required":true}
    ]',
    ARRAY[]::text[],
    '[
        {"name":"Receive MedicationDispense (FHIR)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Derive Outbound Fields","icon":"🔧","step_type":"enrichment.script"},
        {"name":"Map to Canonical B1","icon":"🗺️","step_type":"ncpdptelecom.map_to_canonical"},
        {"name":"Validate","icon":"✅","step_type":"ncpdptelecom.validate"},
        {"name":"Build B1 Transmission","icon":"💊","step_type":"ncpdptelecom.build"},
        {"name":"Send to Payer","icon":"📤","step_type":"connector.outbound"}
    ]',
    20,
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
