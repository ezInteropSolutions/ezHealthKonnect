-- V214: Fix RDE^O11 MedicationRequest.status/.intent mapping
-- Applied: 2026-08-23

-- ============================================================
-- CONTEXT
-- ============================================================
-- The RDE^O11 OOB template (seeded by V62) built MedicationRequest.status
-- from RXE.21 — but per the real HL7 v2.5 RXE segment definition, RXE.21 is
-- "Pharmacy/Treatment Supplier's Special Dispensing Instructions" (Table
-- 0721), not an order-status field. So even a fully-populated real RDE^O11
-- message would land the wrong content in .status (or, since our test
-- sample never populated RXE.21 at all, land nothing — producing the
-- "required field 'status' is missing" FHIR validation error found during
-- the pipeline-builder HL7 sweep test).
--
-- MedicationRequest.intent had no mapping at all — no HL7 field was ever
-- wired to it, so the required field was always missing regardless of input.
--
-- The actual source data for both lives in the ORC (Common Order) segment,
-- which this template's MedicationRequest section never referenced:
--   ORC.5 (Order Status, HL7 Table 0038)          → MedicationRequest.status
--   ORC.1 (Order Control Code, HL7 Table 0119)    → MedicationRequest.intent
--
-- This migration replaces the RXE.21→status mapping with an ORC.5→status
-- mapping, and adds a new ORC.1→intent mapping. Values that aren't present
-- in the inline valueMap (e.g. ORC.5/ORC.1 omitted by the sender) are still
-- covered defensively by the Go-level MedicationRequest normalizer added in
-- the same change (services/hl7_fhir_transform_service_v3.go), which
-- defaults status to "unknown" and intent to "order" — mirroring the
-- existing Encounter.status and ServiceRequest.intent conventions.

-- ============================================================
-- RDE^O11: replace RXE.21 status mapping, add ORC.1 intent mapping
-- ============================================================

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,MedicationRequest,mappings}',
    '[
        {"hl7Path":"RXE.1","fhirPath":"MedicationRequest.dispenseRequest.quantity.value","hl7DataType":"TQ","fhirDataType":"decimal","transform":"numeric_mapping","required":false,"confidence":0.8},
        {"hl7Path":"RXE.2.1","fhirPath":"MedicationRequest.medicationCodeableConcept.coding[0].code","hl7DataType":"CWE","fhirDataType":"code","transform":"string_direct","required":true,"confidence":1.0},
        {"hl7Path":"RXE.2.2","fhirPath":"MedicationRequest.medicationCodeableConcept.coding[0].display","hl7DataType":"CWE","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.95},
        {"hl7Path":"RXE.2.3","fhirPath":"MedicationRequest.medicationCodeableConcept.coding[0].system","hl7DataType":"ID","fhirDataType":"uri","transform":"coding_system_mapping","required":false,"confidence":0.9,"valueMap":{"RXNORM":"http://www.nlm.nih.gov/research/umls/rxnorm","NDC":"http://hl7.org/fhir/sid/ndc","NDF-RT":"http://hl7.org/fhir/ndfrt"}},
        {"hl7Path":"RXE.3","fhirPath":"MedicationRequest.dispenseRequest.quantity.value","hl7DataType":"NM","fhirDataType":"decimal","transform":"numeric_mapping","required":false,"confidence":0.85},
        {"hl7Path":"RXE.4","fhirPath":"MedicationRequest.dispenseRequest.quantity.unit","hl7DataType":"NM","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.8},
        {"hl7Path":"RXE.5.1","fhirPath":"MedicationRequest.dosageInstruction[0].route.coding[0].code","hl7DataType":"CE","fhirDataType":"code","transform":"string_direct","required":false,"confidence":0.85},
        {"hl7Path":"RXE.7","fhirPath":"MedicationRequest.dosageInstruction[0].text","hl7DataType":"CE","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.8},
        {"hl7Path":"RXE.10","fhirPath":"MedicationRequest.dispenseRequest.numberOfRepeatsAllowed","hl7DataType":"NM","fhirDataType":"unsignedInt","transform":"numeric_mapping","required":false,"confidence":0.85},
        {"hl7Path":"RXE.15.1","fhirPath":"MedicationRequest.identifier[0].value","hl7DataType":"CX","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.9},
        {"hl7Path":"ORC.5","fhirPath":"MedicationRequest.status","hl7DataType":"ID","fhirDataType":"code","transform":"medication_status_mapping","required":true,"confidence":0.9,"valueMap":{"A":"active","CA":"cancelled","CM":"completed","DC":"stopped","ER":"entered-in-error","HD":"on-hold","IP":"active","RP":"stopped","SC":"active"}},
        {"hl7Path":"ORC.1","fhirPath":"MedicationRequest.intent","hl7DataType":"ID","fhirDataType":"code","transform":"medication_intent_mapping","required":true,"confidence":0.9,"valueMap":{"CA":"order","DC":"order","HD":"order","NW":"order","OK":"order","RF":"instance-order","RQ":"instance-order","RR":"instance-order","RU":"order","XO":"order"}},
        {"hl7Path":"RXE.27.1","fhirPath":"MedicationRequest.dosageInstruction[0].site.coding[0].code","hl7DataType":"CE","fhirDataType":"code","transform":"string_direct","required":false,"confidence":0.8}
    ]'::jsonb
),
updated_at = NOW()
WHERE id = '590e6999-0982-4abf-aca9-e311decb52de'
  AND message_type = 'RDE^O11';
