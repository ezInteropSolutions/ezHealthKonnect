-- V130: Fix ORU^R03 OOB template
--
-- Problems in the existing template:
--   1. PID.3  → Patient.id          (whole CX field) — AutoTranslate produces {use,value} object
--   2. OBR.3  → identifier[0].value (whole EI field) — AutoTranslate produces {system,value} object
--   3. OBR.2  → identifier[1].value (whole EI field) — same issue
--   4. No MessageHeader              — bundle type="message" requires bdl-12
--
-- Fixes applied:
--   1-3: Change to sub-component paths (PID.3.1, OBR.3.1, OBR.2.1) so the
--        enrichMappingsWithDataTypes guard skips composite-type enrichment and
--        extractHL7ValueAtomic returns a plain string value.
--   4:   Add MessageHeader resource section with MSH field mappings.

UPDATE hl7_fhir_templates
SET template_config = $template$
{
  "version": "2.0",
  "messageType": "ORU^R03",
  "mappings": {
    "MessageHeader": {
      "segment":      "MSH",
      "resourceType": "MessageHeader",
      "mappings": [
        { "hl7Path": "MSH.10",  "fhirPath": "id" },
        { "hl7Path": "MSH.3.1", "fhirPath": "source.name" },
        { "hl7Path": "MSH.4.1", "fhirPath": "source.endpoint" },
        { "hl7Path": "MSH.9.1", "fhirPath": "eventCoding.code" },
        { "hl7Path": "MSH.9.2", "fhirPath": "eventCoding.display" },
        { "hl7Path": "MSH.7",   "fhirPath": "meta.lastUpdated", "transform": "hl7datetime" }
      ]
    },
    "Patient": {
      "idFrom":       "PID.3.1",
      "resourceType": "Patient",
      "mappings": [
        { "hl7Path": "PID.3.1",  "fhirPath": "id" },
        { "hl7Path": "PID.5.1",  "fhirPath": "name[0].family" },
        { "hl7Path": "PID.5.2",  "fhirPath": "name[0].given[0]" },
        { "hl7Path": "PID.5.3",  "fhirPath": "name[0].given[1]" },
        { "hl7Path": "PID.7",    "fhirPath": "birthDate",             "transform": "hl7date" },
        { "hl7Path": "PID.8",    "fhirPath": "gender",               "transform": "gender" },
        { "hl7Path": "PID.11.1", "fhirPath": "address[0].line[0]" },
        { "hl7Path": "PID.11.3", "fhirPath": "address[0].city" },
        { "hl7Path": "PID.11.4", "fhirPath": "address[0].state" },
        { "hl7Path": "PID.11.5", "fhirPath": "address[0].postalCode" },
        { "hl7Path": "PID.13.4", "fhirPath": "telecom[0].value",     "transform": "phone" }
      ]
    },
    "DiagnosticReport": {
      "idFrom":       "OBR.3 ?? OBR.2",
      "resourceType": "DiagnosticReport",
      "mappings": [
        { "hl7Path": "OBR.25",  "fhirPath": "status",   "required": true,
          "transform": "lookup:oruDRStatus", "fallbackTransform": "code" },
        { "hl7Path": "OBR.4",   "fhirPath": "code.text" },
        { "hl7Path": "OBR.4.1", "fhirPath": "code.coding[0].code" },
        { "hl7Path": "OBR.4.2", "fhirPath": "code.coding[0].display" },
        { "hl7Path": "OBR.7",   "fhirPath": "effectiveDateTime",    "transform": "hl7datetime" },
        { "hl7Path": "OBR.8",   "fhirPath": "effectivePeriod.end",
          "condition": "OBR.8 notempty",                            "transform": "hl7datetime" },
        { "hl7Path": "OBR.22",  "fhirPath": "issued",              "transform": "hl7datetime" },
        { "hl7Path": "OBR.24",  "fhirPath": "category[0].coding[0].code" },
        { "hl7Path": "OBR.3.1", "fhirPath": "identifier[0].value" },
        { "hl7Path": "OBR.2.1", "fhirPath": "identifier[1].value", "condition": "OBR.2 notempty" }
      ],
      "composites": [
        {
          "fhir":      "_procedures",
          "procedure": "assembleObservationsFromOBX",
          "params": {
            "collapseTextOBX":        true,
            "liftEDAttachments":      true,
            "linkToDiagnosticReport": true
          }
        }
      ]
    }
  },
  "valueSets": {
    "oruDRStatus": {
      "C": "amended", "F": "final",  "I": "registered", "P": "preliminary",
      "R": "partial",  "S": "partial","U": "final",       "X": "cancelled"
    },
    "oruObsStatus": {
      "C": "corrected", "D": "entered-in-error", "F": "final",   "I": "registered",
      "P": "preliminary","R": "partial",          "S": "partial", "U": "final",
      "W": "entered-in-error",                    "X": "cancelled"
    }
  }
}
$template$::jsonb
WHERE message_type = 'ORU^R03'
  AND is_default   = true;
