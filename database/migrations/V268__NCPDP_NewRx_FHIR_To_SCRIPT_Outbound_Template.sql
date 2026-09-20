-- V268: FHIR MedicationRequest to NCPDP SCRIPT NewRx OUTBOUND pipeline template
-- Applied: 2026-09-20

-- ============================================================
-- FHIR MedicationRequest -> NCPDP SCRIPT NewRx (OUTBOUND) TEMPLATE
-- ============================================================
-- The FIRST genuinely OUTBOUND NCPDP pipeline in this codebase. Every prior
-- NCPDP OOB template (V258-V267, both SCRIPT and D.0) goes wire-format-IN ->
-- FHIR-OUT, terminating at a no-op sink_outbound connector.outbound step —
-- confirmed by direct inspection before this template was written, not
-- assumed. This template proves the reverse direction real e-prescribing
-- customers actually need: a doctor writes a prescription in the EHR (a FHIR
-- MedicationRequest is created) -> ezHealthKonnect translates it into a real
-- NewRx and hands it to a REAL outbound connector (http_outbound here, not
-- sink_outbound) addressed to the patient's pharmacy.
--
-- Honest scope note (see this template's own Go test source for the full
-- rationale): a bare FHIR MedicationRequest carries only References
-- (subject/requester/dispenseRequest.performer), not full inline patient/
-- prescriber/pharmacy demographics -- that data lives on separate Patient/
-- Practitioner/Organization resources in a real system. The derive script
-- below uses Reference.display (a real, spec-legal FHIR field meant exactly
-- for "human-readable identification without the full resource") for names,
-- and Reference.identifier.value for NPIs (the same logical-reference
-- pattern already established in this codebase for EDI 837's
-- Claim.careTeam[].provider). It does not fabricate FHIR structure that
-- isn't there -- a real deployment needing full demographics would send a
-- Bundle-shaped input or add a lookup/enrichment step before this one.
--
--   seq 5    connector.inbound   (http_fhir_inbound — a FHIR MedicationRequest POSTed to this
--                                 listener triggers the pipeline; the resource's own fields land
--                                 FLAT AT ROOT for this first step, confirmed directly in
--                                 processing/engine_message_processor.go's own http_fhir handling)
--   seq 20   enrichment.script   (Derive New Rx Outbound Fields — reshapes the raw FHIR resource
--                                 into flat, already-snake_case fields; EVERY later step addresses
--                                 ONLY this script's own steps.<alias>.step_output, never the raw
--                                 FHIR fields directly, avoiding the "message." nesting ambiguity
--                                 entirely — the same clean pattern 837P/837I's own derive script
--                                 established: compute everything once, address it once)
--   seq 30   ncpdp.map_to_canonical (builds canonical NewRx header/body JSON from the derived fields)
--   seq 35   ncpdp.validate      (checks the canonical JSON's own required field/group completeness
--                                 before build — non-blocking, matching every other NCPDP/EDI
--                                 template's own "validation is advisory, never blocks delivery"
--                                 precedent; added after initial full-stack verification found
--                                 neither outbound template had a validate step at all, unlike every
--                                 inbound-direction template)
--   seq 40   ncpdp.build         (canonical JSON -> real NewRx XML)
--   seq 295  connector.outbound  (http_outbound — a REAL send, not sink_outbound; the config's own
--                                 "endpoint" is a placeholder the deploying user replaces with their
--                                 real pharmacy/switch destination)
--
-- Config here is transcribed verbatim from
-- services/executors/transform/newrx_outbound_test.go
-- (TestNewRxOutbound_FromFHIRMedicationRequest_BuildsValidNewRxXML), which
-- proves this exact chain via the real executors and asserts the built XML
-- re-parses cleanly through ncpdp.ParseMessage — the same bar every other
-- NCPDP transaction type in this codebase proves itself against.
--
-- step_alias note: "Derive New Rx Outbound Fields" normalizes (NormalizeKey)
-- to "derive_new_rx_outbound_fields" -- verified by hand-tracing
-- NormalizeKey's own camelCase-split + lowercase + underscore-join behavior
-- BEFORE writing this migration (the already-documented, twice-independently-
-- rediscovered gotcha in this codebase: steps.<key> addressing is keyed off
-- NormalizeKey(step.StepName), never the configured step_alias field itself).

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
    'FHIR MedicationRequest to NCPDP SCRIPT NewRx (Outbound)',
    'fhir-medicationrequest-to-newrx-outbound',
    'Receive a FHIR R4 MedicationRequest over HTTP, derive the prescribing details, and build+send a real NCPDP SCRIPT NewRx e-prescription to a pharmacy or switch endpoint.',
    'ncpdp',
    null,
    ARRAY['ncpdp','script','newrx','pharmacy','e-prescribing','medicationrequest','fhir','outbound','http'],
    '📤',
    'advanced',
    'http_fhir_inbound',
    '{"port": 9610, "basePath": "/fhir/r4", "fhirVersion": "R4"}',
    'http_outbound',
    '{"url": "https://REPLACE_WITH_PHARMACY_ENDPOINT/newrx", "method": "POST"}',
    $pipeline$
    {
        "execution_groups": [
            {
                "sequence": 5,
                "steps": [{
                    "step_name": "Receive Medication Request (FHIR)",
                    "step_type": "connector.inbound",
                    "sequence": 5,
                    "config": {
                        "connectorType": "http_fhir_inbound",
                        "config": {"port": 9610, "basePath": "/fhir/r4", "fhirVersion": "R4"}
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 20,
                "steps": [{
                    "step_name": "Derive New Rx Outbound Fields",
                    "step_alias": "derive_new_rx_outbound_fields",
                    "step_type": "enrichment.script",
                    "sequence": 20,
                    "description": "Reshapes the raw FHIR MedicationRequest (subject/requester/dispenseRequest.performer References, medicationCodeableConcept, dosageInstruction, authoredOn) into the flat, snake_case fields ncpdp.map_to_canonical below addresses. Splits Reference.display into first/last name (a real, common convenience — not a fabricated FHIR structure) and reads NPIs from Reference.identifier.value.",
                    "config": {
                        "script": "\nvar msg = input.resourceType ? input : ((input.message && input.message.resourceType) ? input.message : input);\nvar subj = msg.subject || {};\nvar req = msg.requester || {};\nvar disp = msg.dispenseRequest || {};\nvar med = msg.medicationCodeableConcept || {};\nvar dosageArr = msg.dosageInstruction || [];\nvar dosage = dosageArr.length > 0 ? dosageArr[0] : {};\nvar performer = disp.performer || {};\nvar quantity = disp.quantity || {};\nvar coding = (med.coding && med.coding.length > 0) ? med.coding[0] : {};\n\nfunction splitDisplayName(display) {\n    if (!display) return { first: \"\", last: \"\" };\n    var parts = display.split(\" \").filter(function(p) { return p.length > 0; });\n    if (parts.length === 0) return { first: \"\", last: \"\" };\n    if (parts.length === 1) return { first: \"\", last: parts[0] };\n    return { first: parts.slice(0, parts.length - 1).join(\" \"), last: parts[parts.length - 1] };\n}\n\nvar patientName = splitDisplayName(subj.display);\nvar prescriberName = splitDisplayName(req.display);\n\nvar quantityValue = \"\";\nif (quantity.value !== undefined && quantity.value !== null) {\n    quantityValue = String(quantity.value);\n}\n\n({\n    message_id: msg.id || \"\",\n    patient_first_name: patientName.first,\n    patient_last_name: patientName.last,\n    prescriber_first_name: prescriberName.first,\n    prescriber_last_name: prescriberName.last,\n    prescriber_npi: (req.identifier && req.identifier.value) || \"\",\n    pharmacy_npi: (performer.identifier && performer.identifier.value) || \"\",\n    drug_description: med.text || \"\",\n    drug_code: coding.code || \"\",\n    quantity_value: quantityValue,\n    written_date: msg.authoredOn || \"\",\n    sig_text: dosage.text || \"\"\n});\n"
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 30,
                "steps": [{
                    "step_name": "Map to Canonical NewRx",
                    "step_type": "ncpdp.map_to_canonical",
                    "sequence": 30,
                    "description": "Builds canonical NewRx header/body JSON from the derive step's own output. to/from are payer-enrollment-level mailbox routing values no FHIR resource could ever carry — placeholders the deploying user replaces via config, the same 'config fills in what data doesn't supply' precedent already established for CdaCustodianConfig/EDI's ISA-GS config.",
                    "config": {
                        "outputField": "parsedNCPDP",
                        "transactionType": "NewRx",
                        "headerFields": [
                            {"fieldKey": "to", "literalValue": "PHARMACY_MAILBOX_ID"},
                            {"fieldKey": "from", "literalValue": "EZHEALTHKONNECT"},
                            {"fieldKey": "messageID", "sourcePath": "steps.derive_new_rx_outbound_fields.step_output.message_id"},
                            {"fieldKey": "sentTime", "sourcePath": "steps.derive_new_rx_outbound_fields.step_output.written_date"}
                        ],
                        "bodyGroups": [
                            {
                                "groupKey": "patient",
                                "groups": [
                                    {
                                        "groupKey": "humanPatient",
                                        "groups": [
                                            {
                                                "groupKey": "name",
                                                "fields": [
                                                    {"fieldKey": "lastName", "sourcePath": "steps.derive_new_rx_outbound_fields.step_output.patient_last_name"},
                                                    {"fieldKey": "firstName", "sourcePath": "steps.derive_new_rx_outbound_fields.step_output.patient_first_name"}
                                                ]
                                            }
                                        ]
                                    }
                                ]
                            },
                            {
                                "groupKey": "prescriber",
                                "groups": [
                                    {
                                        "groupKey": "nonVeterinarian",
                                        "groups": [
                                            {
                                                "groupKey": "identification",
                                                "fields": [
                                                    {"fieldKey": "npi", "sourcePath": "steps.derive_new_rx_outbound_fields.step_output.prescriber_npi"}
                                                ]
                                            },
                                            {
                                                "groupKey": "name",
                                                "fields": [
                                                    {"fieldKey": "lastName", "sourcePath": "steps.derive_new_rx_outbound_fields.step_output.prescriber_last_name"},
                                                    {"fieldKey": "firstName", "sourcePath": "steps.derive_new_rx_outbound_fields.step_output.prescriber_first_name"}
                                                ]
                                            }
                                        ]
                                    }
                                ]
                            },
                            {
                                "groupKey": "medicationPrescribed",
                                "fields": [
                                    {"fieldKey": "drugDescription", "sourcePath": "steps.derive_new_rx_outbound_fields.step_output.drug_description"}
                                ],
                                "groups": [
                                    {
                                        "groupKey": "drugCoded",
                                        "groups": [
                                            {
                                                "groupKey": "productCode",
                                                "fields": [
                                                    {"fieldKey": "code", "sourcePath": "steps.derive_new_rx_outbound_fields.step_output.drug_code"},
                                                    {"fieldKey": "qualifier", "literalValue": "ND"}
                                                ]
                                            }
                                        ]
                                    },
                                    {
                                        "groupKey": "quantity",
                                        "fields": [
                                            {"fieldKey": "value", "sourcePath": "steps.derive_new_rx_outbound_fields.step_output.quantity_value"}
                                        ]
                                    },
                                    {
                                        "groupKey": "writtenDate",
                                        "fields": [
                                            {"fieldKey": "date", "sourcePath": "steps.derive_new_rx_outbound_fields.step_output.written_date", "transform": "datetime_to_ncpdp_date"}
                                        ]
                                    },
                                    {
                                        "groupKey": "sig",
                                        "fields": [
                                            {"fieldKey": "sigText", "sourcePath": "steps.derive_new_rx_outbound_fields.step_output.sig_text"}
                                        ]
                                    }
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
                    "step_name": "Validate Canonical New Rx",
                    "step_type": "ncpdp.validate",
                    "sequence": 35,
                    "description": "Checks the canonical NewRx JSON's own required field/group completeness BEFORE building XML — catches a missing prescriber NPI or similar gap early rather than sending a malformed message out. Non-blocking (required:false), matching this codebase's own established 'validation is advisory, never blocks delivery' precedent for every other NCPDP/EDI template.",
                    "config": {"sourceField": "parsedNCPDP", "outputField": "ncpdpValidation"},
                    "enabled": true,
                    "required": false
                }]
            },
            {
                "sequence": 40,
                "steps": [{
                    "step_name": "Build New Rx XML",
                    "step_type": "ncpdp.build",
                    "sequence": 40,
                    "config": {"sourceField": "parsedNCPDP", "transactionType": "NewRx", "outputField": "ncpdpScript"},
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 295,
                "steps": [{
                    "step_name": "Send New Rx to Pharmacy",
                    "step_type": "connector.outbound",
                    "sequence": 295,
                    "config": {
                        "connectorType": "http_outbound",
                        "config": {"url": "https://REPLACE_WITH_PHARMACY_ENDPOINT/newrx", "method": "POST"},
                        "contentField": "ncpdpScript",
                        "contentType": "application/xml"
                    },
                    "enabled": true,
                    "required": true
                }]
            }
        ]
    }
    $pipeline$::jsonb,
    'MedicationRequest',
    '[
        {"section":"source","field":"port","label":"Listener Port","type":"number","hint":"Port this pipeline listens on for incoming FHIR MedicationRequest resources","required":true},
        {"section":"target","field":"url","label":"Pharmacy/Switch Endpoint URL","type":"string","hint":"Real destination the built NewRx XML is POSTed to","required":true}
    ]',
    ARRAY[]::text[],
    '[
        {"name":"Receive MedicationRequest (FHIR)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Derive Outbound Fields","icon":"🔧","step_type":"enrichment.script"},
        {"name":"Map to Canonical NewRx","icon":"🗺️","step_type":"ncpdp.map_to_canonical"},
        {"name":"Validate","icon":"✅","step_type":"ncpdp.validate"},
        {"name":"Build New Rx XML","icon":"💊","step_type":"ncpdp.build"},
        {"name":"Send to Pharmacy","icon":"📤","step_type":"connector.outbound"}
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
