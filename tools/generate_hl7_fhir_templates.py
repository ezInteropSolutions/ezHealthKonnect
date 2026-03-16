#!/usr/bin/env python3
"""
HL7 v2 → FHIR R4 Template Generator
Generates hl7_fhir_templates SQL INSERT statements for all major HL7 message types.

Run: python3 tools/generate_hl7_fhir_templates.py > database/migrations/V62__Comprehensive_HL7_FHIR_Templates.sql
"""

import json
import uuid
from datetime import datetime

# ──────────────────────────────────────────────────────────────────
# SHARED SEGMENT FIELD-LEVEL MAPPINGS
# Each entry: (hl7Path, fhirPath, hl7DataType, fhirDataType, transform, required, confidence, valueMap)
# ──────────────────────────────────────────────────────────────────

MSH_MAPPINGS = [
    ("MSH.3.1",  "MessageHeader.source.name",               "ST",  "string",    "string_direct",               False, 0.95, None),
    ("MSH.4.1",  "MessageHeader.source.endpoint",           "ST",  "string",    "string_direct",               False, 0.90, None),
    ("MSH.3",    "MessageHeader.source.endpoint",           "HD",  "uri",       "assigning_authority_to_uri",  False, 0.85, None),
    ("MSH.5.1",  "MessageHeader.destination[0].name",       "ST",  "string",    "string_direct",               False, 0.90, None),
    ("MSH.7",    "MessageHeader.meta.lastUpdated",           "TS",  "instant",   "hl7_timestamp_to_fhir_instant", False, 0.95, None),
    ("MSH.9.1",  "MessageHeader.eventCoding.code",          "ST",  "code",      "string_direct",               True,  1.00, None),
    ("MSH.9.2",  "MessageHeader.eventCoding.display",       "ST",  "string",    "string_direct",               False, 0.90, None),
    ("MSH.10",   "MessageHeader.id",                        "ST",  "id",        "string_direct",               True,  1.00, None),
    ("MSH.12",   "MessageHeader.meta.versionId",            "VID", "string",    "string_direct",               False, 0.80, None),
]

PID_PATIENT_MAPPINGS = [
    # Identifiers
    ("PID.3.1",  "Patient.identifier[0].value",             "ST",  "string",    "string_direct",               True,  1.00, None),
    ("PID.3.4",  "Patient.identifier[0].system",            "HD",  "uri",       "assigning_authority_to_uri",  False, 0.95, None),
    ("PID.3.5",  "Patient.identifier[0].type.coding[0].code","ID", "code",      "identifier_type_mapping",     False, 0.90,
        {"MR": "MR", "PI": "PI", "SS": "SS", "DL": "DL", "MA": "MA", "NH": "NH", "NPI": "NPI"}),
    ("PID.18.1", "Patient.identifier[1].value",             "ST",  "string",    "string_direct",               False, 0.85, None),
    ("PID.19",   "Patient.identifier[2].value",             "ST",  "string",    "string_direct",               False, 0.80, None),
    # Name
    ("PID.5.1",  "Patient.name[0].family",                  "FN",  "string",    "name_component",              True,  1.00, None),
    ("PID.5.2",  "Patient.name[0].given[0]",                "ST",  "string",    "name_component",              False, 0.95, None),
    ("PID.5.3",  "Patient.name[0].given[1]",                "ST",  "string",    "name_component",              False, 0.90, None),
    ("PID.5.4",  "Patient.name[0].suffix[0]",               "ST",  "string",    "name_component",              False, 0.80, None),
    ("PID.5.5",  "Patient.name[0].prefix[0]",               "ST",  "string",    "name_component",              False, 0.80, None),
    ("PID.5.7",  "Patient.name[0].use",                     "ID",  "code",      "name_use_mapping",            False, 0.85,
        {"L": "official", "A": "nickname", "B": "old", "C": "usual", "D": "usual", "M": "maiden", "P": "nickname", "S": "anonymous"}),
    # Mother's maiden name
    ("PID.6.1",  "Patient.name[1].family",                  "FN",  "string",    "name_component",              False, 0.85, None),
    ("PID.6.2",  "Patient.name[1].given[0]",                "ST",  "string",    "name_component",              False, 0.80, None),
    # Date of birth
    ("PID.7",    "Patient.birthDate",                       "TS",  "date",      "hl7_timestamp_to_fhir_date",  False, 1.00, None),
    # Gender
    ("PID.8",    "Patient.gender",                          "IS",  "code",      "hl7_table_0001_gender",       False, 1.00,
        {"M": "male", "F": "female", "O": "other", "U": "unknown", "A": "other", "N": "other"}),
    # Address
    ("PID.11.1", "Patient.address[0].line[0]",              "SAD", "string",    "address_component",           False, 0.95, None),
    ("PID.11.2", "Patient.address[0].district",             "ST",  "string",    "address_component",           False, 0.80, None),
    ("PID.11.3", "Patient.address[0].city",                 "ST",  "string",    "address_component",           False, 0.95, None),
    ("PID.11.4", "Patient.address[0].state",                "ST",  "string",    "address_component",           False, 0.90, None),
    ("PID.11.5", "Patient.address[0].postalCode",           "ST",  "string",    "address_component",           False, 0.95, None),
    ("PID.11.6", "Patient.address[0].country",              "ID",  "string",    "address_component",           False, 0.90, None),
    ("PID.11.7", "Patient.address[0].use",                  "ID",  "code",      "address_use_mapping",         False, 0.85,
        {"M": "home", "H": "home", "B": "work", "O": "work", "C": "billing", "P": "temp", "BDL": "billing", "L": "home", "N": "old"}),
    # Phone - home
    ("PID.13.4", "Patient.telecom[0].value",                "ST",  "string",    "telecom_value",               False, 0.95, None),
    ("PID.13.3", "Patient.telecom[0].system",               "ID",  "code",      "telecom_system_mapping",      False, 0.90,
        {"PH": "phone", "CP": "phone", "FX": "fax", "BP": "pager", "Internet": "email", "X.400": "email"}),
    ("PID.13.2", "Patient.telecom[0].use",                  "ID",  "code",      "telecom_use_mapping",         False, 0.85,
        {"PRN": "home", "WPN": "work", "NET": "home", "ORN": "old", "BPN": "mobile", "MCM": "mobile"}),
    # Phone - business
    ("PID.14.4", "Patient.telecom[1].value",                "ST",  "string",    "telecom_value",               False, 0.90, None),
    ("PID.14.3", "Patient.telecom[1].system",               "ID",  "code",      "telecom_system_mapping",      False, 0.85,
        {"PH": "phone", "CP": "phone", "FX": "fax", "BP": "pager", "Internet": "email"}),
    # Language
    ("PID.15.1", "Patient.communication[0].language.coding[0].code", "CE", "code", "string_direct",           False, 0.85, None),
    # Marital status
    ("PID.16.1", "Patient.maritalStatus.coding[0].code",   "CE",  "code",      "marital_status_mapping",      False, 0.80,
        {"A": "A", "D": "D", "M": "M", "S": "S", "W": "W", "C": "M", "G": "M", "P": "M", "R": "D", "E": "M", "N": "UNK", "I": "UNK", "B": "UNK", "T": "UNK", "U": "UNK"}),
    # Race / Ethnicity (extensions)
    ("PID.10.1", "Patient.extension[0].valueCoding.code",  "CE",  "code",      "string_direct",               False, 0.75, None),
    ("PID.22.1", "Patient.extension[1].valueCoding.code",  "CE",  "code",      "string_direct",               False, 0.75, None),
    # Deceased
    ("PID.29",   "Patient.deceasedDateTime",                "TS",  "dateTime",  "hl7_timestamp_to_fhir_datetime", False, 0.95, None),
    ("PID.30",   "Patient.deceasedBoolean",                 "ID",  "boolean",   "boolean_yn_mapping",          False, 0.90,
        {"Y": "true", "N": "false"}),
]

PV1_ENCOUNTER_MAPPINGS = [
    # Class / type
    ("PV1.2",    "Encounter.class",                         "IS",  "Coding",    "hl7_table_0004_patient_class", True, 1.00,
        {"I": "IMP", "O": "AMB", "E": "EMER", "P": "IMP", "R": "IMP", "B": "IMP", "C": "IMP", "N": "AMB", "U": "EMER"}),
    ("PV1.4.1",  "Encounter.type[0].coding[0].code",        "IS",  "code",      "admission_type_mapping",      False, 0.85,
        {"A": "A", "C": "C", "E": "E", "L": "L", "N": "N", "R": "R", "U": "U"}),
    # Location
    ("PV1.3.1",  "Encounter.location[0].location.display",  "ST",  "string",    "string_direct",               False, 0.90, None),
    ("PV1.3.2",  "Encounter.location[0].location.identifier[0].value", "ST", "string", "string_direct",        False, 0.85, None),
    ("PV1.6.1",  "Encounter.hospitalization.origin.display","ST",  "string",    "string_direct",               False, 0.80, None),
    # Practitioners — attending
    ("PV1.7.1",  "Encounter.participant[0].individual.identifier[0].value", "ST", "string", "string_direct",   False, 0.90, None),
    ("PV1.7.3",  "Encounter.participant[0].individual.display",             "ST", "string", "name_component",  False, 0.85, None),
    # Practitioners — referring
    ("PV1.8.1",  "Encounter.participant[1].individual.identifier[0].value", "ST", "string", "string_direct",   False, 0.85, None),
    # Practitioners — consulting
    ("PV1.9.1",  "Encounter.participant[2].individual.identifier[0].value", "ST", "string", "string_direct",   False, 0.80, None),
    # Service type
    ("PV1.10.1", "Encounter.serviceType.coding[0].code",   "IS",  "code",      "string_direct",               False, 0.85, None),
    # Practitioners — admitting
    ("PV1.17.1", "Encounter.participant[3].individual.identifier[0].value", "ST", "string", "string_direct",   False, 0.85, None),
    # Identifiers
    ("PV1.19.1", "Encounter.identifier[0].value",           "CX",  "string",    "string_direct",               False, 0.90, None),
    # Admit/discharge info
    ("PV1.20.1", "Encounter.hospitalization.admitSource.coding[0].code",    "IS", "code",  "string_direct",    False, 0.85, None),
    ("PV1.36.1", "Encounter.hospitalization.dischargeDisposition.coding[0].code", "IS", "code", "discharge_disposition_mapping", False, 0.85,
        {"01": "home", "02": "other-hcf", "03": "snf", "04": "icf", "05": "other-hcf", "06": "home", "07": "aama", "08": "hosp-d",
         "09": "exp", "10": "not-specified", "20": "other-hcf", "21": "rehab", "30": "other-hcf", "40": "exp", "41": "exp",
         "42": "exp", "43": "hospice", "50": "other-hcf", "51": "other-hcf", "61": "other-hcf", "62": "rehab", "63": "ltc",
         "64": "other-hcf", "65": "psych", "66": "rehab", "69": "other-hcf", "70": "oth", "71": "oth", "72": "oth",
         "81": "oth", "82": "oth", "83": "oth", "84": "oth", "85": "oth", "86": "oth", "87": "oth", "88": "oth", "89": "oth", "90": "oth"}),
    # Period
    ("PV1.44",   "Encounter.period.start",                  "TS",  "dateTime",  "hl7_timestamp_to_fhir_datetime", False, 0.95, None),
    ("PV1.45",   "Encounter.period.end",                    "TS",  "dateTime",  "hl7_timestamp_to_fhir_datetime", False, 0.95, None),
    # Status derived from event — not directly from a single field; set in createBundle
]

OBX_OBSERVATION_MAPPINGS = [
    ("OBX.1",    "Observation.id",                          "SI",  "id",        "string_direct",               False, 0.90, None),
    # OBX.2 = value type indicator — used programmatically, not mapped to a specific path
    ("OBX.3.1",  "Observation.code.coding[0].code",         "ST",  "code",      "string_direct",               True,  1.00, None),
    ("OBX.3.2",  "Observation.code.coding[0].display",      "ST",  "string",    "string_direct",               False, 0.95, None),
    ("OBX.3.3",  "Observation.code.coding[0].system",       "ID",  "uri",       "coding_system_mapping",       False, 0.90,
        {"LN": "http://loinc.org", "LOINC": "http://loinc.org", "SCT": "http://snomed.info/sct",
         "SNOMED": "http://snomed.info/sct", "NCI": "http://ncimeta.nci.nih.gov",
         "HL70078": "http://terminology.hl7.org/CodeSystem/v2-0078"}),
    ("OBX.4",    "Observation.note[0].text",                "ST",  "string",    "string_direct",               False, 0.80, None),
    ("OBX.5",    "Observation.valueString",                 "varies", "string", "observation_value",           False, 0.85, None),
    ("OBX.6.1",  "Observation.valueQuantity.code",          "ST",  "code",      "string_direct",               False, 0.90, None),
    ("OBX.6.2",  "Observation.valueQuantity.unit",          "ST",  "string",    "string_direct",               False, 0.90, None),
    ("OBX.6.3",  "Observation.valueQuantity.system",        "ID",  "uri",       "coding_system_mapping",       False, 0.85,
        {"UCUM": "http://unitsofmeasure.org", "ISO+": "http://unitsofmeasure.org"}),
    ("OBX.7",    "Observation.referenceRange[0].text",      "ST",  "string",    "string_direct",               False, 0.90, None),
    ("OBX.8.1",  "Observation.interpretation[0].coding[0].code", "IS", "code",  "interpretation_mapping",      False, 0.90,
        {"H": "H", "HH": "HH", "L": "L", "LL": "LL", "N": "N", "A": "A", "AA": "AA", "U": "U",
         "D": "D", "B": "B", "W": "W", "S": "S", "R": "R", "I": "I", "MS": "MS", "VS": "VS"}),
    ("OBX.11",   "Observation.status",                      "ID",  "code",      "observation_status_mapping",  True,  1.00,
        {"F": "final", "P": "preliminary", "C": "corrected", "X": "cancelled", "D": "entered-in-error",
         "I": "registered", "S": "registered", "U": "unknown", "W": "entered-in-error"}),
    ("OBX.14",   "Observation.effectiveDateTime",           "TS",  "dateTime",  "hl7_timestamp_to_fhir_datetime", False, 0.95, None),
    ("OBX.16.1", "Observation.performer[0].display",        "ST",  "string",    "name_component",              False, 0.80, None),
    ("OBX.19",   "Observation.issued",                      "TS",  "instant",   "hl7_timestamp_to_fhir_instant", False, 0.90, None),
    ("OBX.23.1", "Observation.performer[1].display",        "ST",  "string",    "string_direct",               False, 0.75, None),
]

OBR_DIAGNOSTIC_REPORT_MAPPINGS = [
    ("OBR.1",    "DiagnosticReport.id",                     "SI",  "id",        "string_direct",               False, 0.90, None),
    ("OBR.2.1",  "DiagnosticReport.identifier[0].value",    "EI",  "string",    "string_direct",               False, 0.90, None),
    ("OBR.3.1",  "DiagnosticReport.identifier[1].value",    "EI",  "string",    "string_direct",               False, 0.90, None),
    ("OBR.4.1",  "DiagnosticReport.code.coding[0].code",    "ST",  "code",      "string_direct",               True,  1.00, None),
    ("OBR.4.2",  "DiagnosticReport.code.coding[0].display", "ST",  "string",    "string_direct",               False, 0.95, None),
    ("OBR.4.3",  "DiagnosticReport.code.coding[0].system",  "ID",  "uri",       "coding_system_mapping",       False, 0.90,
        {"LN": "http://loinc.org", "LOINC": "http://loinc.org", "SCT": "http://snomed.info/sct"}),
    ("OBR.7",    "DiagnosticReport.effectiveDateTime",      "TS",  "dateTime",  "hl7_timestamp_to_fhir_datetime", False, 0.95, None),
    ("OBR.8",    "DiagnosticReport.effectivePeriod.end",    "TS",  "dateTime",  "hl7_timestamp_to_fhir_datetime", False, 0.85, None),
    ("OBR.14",   "DiagnosticReport.issued",                 "TS",  "instant",   "hl7_timestamp_to_fhir_instant", False, 0.90, None),
    ("OBR.16.1", "DiagnosticReport.resultsInterpreter[0].display", "ST", "string", "name_component",           False, 0.85, None),
    ("OBR.22",   "DiagnosticReport.issued",                 "TS",  "instant",   "hl7_timestamp_to_fhir_instant", False, 0.80, None),
    ("OBR.25",   "DiagnosticReport.status",                 "ID",  "code",      "diagnostic_status_mapping",   True,  1.00,
        {"F": "final", "P": "partial", "C": "corrected", "X": "cancelled", "I": "registered",
         "A": "partial", "R": "preliminary", "S": "registered", "Y": "final", "O": "registered"}),
    ("OBR.31.1", "DiagnosticReport.reasonCode[0].coding[0].code", "CE", "code", "string_direct",               False, 0.75, None),
]

AL1_ALLERGY_MAPPINGS = [
    ("AL1.1",    "AllergyIntolerance.id",                   "SI",  "id",        "string_direct",               False, 0.90, None),
    ("AL1.2.1",  "AllergyIntolerance.category[0]",          "IS",  "code",      "allergy_category_mapping",    False, 0.90,
        {"DA": "medication", "FA": "food", "EA": "environment", "MA": "medication",
         "MC": "medication", "D": "medication", "F": "food", "E": "environment", "O": "biologic"}),
    ("AL1.3.1",  "AllergyIntolerance.code.coding[0].code",  "CE",  "code",      "string_direct",               True,  1.00, None),
    ("AL1.3.2",  "AllergyIntolerance.code.coding[0].display", "CE", "string",   "string_direct",               False, 0.95, None),
    ("AL1.3.3",  "AllergyIntolerance.code.coding[0].system", "ID", "uri",       "coding_system_mapping",       False, 0.85,
        {"SCT": "http://snomed.info/sct", "RXNORM": "http://www.nlm.nih.gov/research/umls/rxnorm",
         "NDF-RT": "http://hl7.org/fhir/ndfrt"}),
    ("AL1.4.1",  "AllergyIntolerance.reaction[0].severity", "IS",  "code",      "allergy_severity_mapping",    False, 0.90,
        {"SV": "severe", "MO": "moderate", "MI": "mild", "U": "moderate"}),
    ("AL1.5",    "AllergyIntolerance.reaction[0].description", "ST", "string",  "string_direct",               False, 0.85, None),
    ("AL1.6",    "AllergyIntolerance.onsetDateTime",         "DT",  "dateTime",  "hl7_timestamp_to_fhir_datetime", False, 0.85, None),
]

DG1_CONDITION_MAPPINGS = [
    ("DG1.1",    "Condition.id",                            "SI",  "id",        "string_direct",               False, 0.90, None),
    ("DG1.3.1",  "Condition.code.coding[0].code",           "CE",  "code",      "string_direct",               True,  1.00, None),
    ("DG1.3.2",  "Condition.code.coding[0].display",        "CE",  "string",    "string_direct",               False, 0.95, None),
    ("DG1.3.3",  "Condition.code.coding[0].system",         "ID",  "uri",       "coding_system_mapping",       False, 0.90,
        {"ICD10CM": "http://hl7.org/fhir/sid/icd-10-cm", "ICD-10-CM": "http://hl7.org/fhir/sid/icd-10-cm",
         "ICD9CM": "http://hl7.org/fhir/sid/icd-9-cm", "SCT": "http://snomed.info/sct",
         "I9": "http://hl7.org/fhir/sid/icd-9-cm", "I10": "http://hl7.org/fhir/sid/icd-10-cm"}),
    ("DG1.4",    "Condition.note[0].text",                  "ST",  "string",    "string_direct",               False, 0.80, None),
    ("DG1.5",    "Condition.onsetDateTime",                  "TS",  "dateTime",  "hl7_timestamp_to_fhir_datetime", False, 0.90, None),
    ("DG1.6.1",  "Condition.category[0].coding[0].code",   "IS",  "code",      "diagnosis_type_mapping",      False, 0.85,
        {"A": "encounter-diagnosis", "W": "encounter-diagnosis", "F": "encounter-diagnosis",
         "AD": "encounter-diagnosis", "DG": "encounter-diagnosis", "DX": "encounter-diagnosis",
         "SD": "encounter-diagnosis", "CC": "chief-complaint", "PR": "problem-list-item"}),
    ("DG1.16.1", "Condition.asserter.display",              "XCN", "string",    "name_component",              False, 0.80, None),
    ("DG1.19",   "Condition.recordedDate",                  "TS",  "dateTime",  "hl7_timestamp_to_fhir_datetime", False, 0.85, None),
]

ORC_SERVICE_REQUEST_MAPPINGS = [
    ("ORC.1",    "ServiceRequest.intent",                   "ID",  "code",      "order_control_to_intent",     True,  1.00,
        {"NW": "order", "CA": "order", "DC": "order", "XO": "order", "RO": "original-order",
         "RP": "reflex-order", "PA": "plan", "RE": "order", "OR": "order", "OK": "order"}),
    ("ORC.2.1",  "ServiceRequest.identifier[0].value",      "EI",  "string",    "string_direct",               False, 0.95, None),
    ("ORC.3.1",  "ServiceRequest.identifier[1].value",      "EI",  "string",    "string_direct",               False, 0.90, None),
    ("ORC.5",    "ServiceRequest.status",                   "ID",  "code",      "order_status_mapping",        False, 0.90,
        {"A": "active", "CA": "revoked", "CM": "completed", "DC": "revoked", "ER": "entered-in-error",
         "HD": "on-hold", "IP": "active", "RP": "active", "SC": "active", "NW": "draft"}),
    ("ORC.9",    "ServiceRequest.authoredOn",               "TS",  "dateTime",  "hl7_timestamp_to_fhir_datetime", False, 0.90, None),
    ("ORC.12.1", "ServiceRequest.requester.identifier[0].value", "XCN", "string", "string_direct",              False, 0.85, None),
    ("ORC.12.3", "ServiceRequest.requester.display",        "XCN", "string",    "name_component",              False, 0.85, None),
    ("ORC.15",   "ServiceRequest.occurrenceDateTime",       "TS",  "dateTime",  "hl7_timestamp_to_fhir_datetime", False, 0.85, None),
    ("ORC.16.1", "ServiceRequest.reasonCode[0].coding[0].code", "CE", "code",   "string_direct",               False, 0.80, None),
    ("ORC.25.1", "ServiceRequest.priority",                 "CWE", "code",      "priority_mapping",            False, 0.85,
        {"S": "stat", "A": "asap", "R": "routine", "P": "urgent", "T": "asap"}),
]

OBR_SERVICE_REQUEST_MAPPINGS = [
    ("OBR.2.1",  "ServiceRequest.identifier[2].value",      "EI",  "string",    "string_direct",               False, 0.90, None),
    ("OBR.4.1",  "ServiceRequest.code.coding[0].code",      "CE",  "code",      "string_direct",               True,  1.00, None),
    ("OBR.4.2",  "ServiceRequest.code.coding[0].display",   "CE",  "string",    "string_direct",               False, 0.95, None),
    ("OBR.4.3",  "ServiceRequest.code.coding[0].system",    "ID",  "uri",       "coding_system_mapping",       False, 0.90,
        {"LN": "http://loinc.org", "LOINC": "http://loinc.org", "SCT": "http://snomed.info/sct",
         "C4": "http://www.ama-assn.org/go/cpt", "CPT4": "http://www.ama-assn.org/go/cpt",
         "HCPCS": "http://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets"}),
    ("OBR.6",    "ServiceRequest.occurrenceDateTime",       "TS",  "dateTime",  "hl7_timestamp_to_fhir_datetime", False, 0.85, None),
    ("OBR.13",   "ServiceRequest.note[0].text",             "ST",  "string",    "string_direct",               False, 0.80, None),
    ("OBR.16.1", "ServiceRequest.requester.display",        "XCN", "string",    "name_component",              False, 0.85, None),
    ("OBR.27.6", "ServiceRequest.priority",                 "TQ",  "code",      "priority_mapping",            False, 0.80,
        {"S": "stat", "A": "asap", "R": "routine", "P": "urgent"}),
    ("OBR.31.1", "ServiceRequest.reasonCode[0].coding[0].code", "CE", "code",   "string_direct",               False, 0.80, None),
]

NK1_RELATED_PERSON_MAPPINGS = [
    ("NK1.1",    "RelatedPerson.id",                        "SI",  "id",        "string_direct",               False, 0.90, None),
    ("NK1.2.1",  "RelatedPerson.name[0].family",            "XPN", "string",    "name_component",              False, 0.95, None),
    ("NK1.2.2",  "RelatedPerson.name[0].given[0]",          "XPN", "string",    "name_component",              False, 0.90, None),
    ("NK1.3.1",  "RelatedPerson.relationship[0].coding[0].code", "CE", "code",  "string_direct",               False, 0.90, None),
    ("NK1.3.2",  "RelatedPerson.relationship[0].coding[0].display", "CE", "string", "string_direct",           False, 0.85, None),
    ("NK1.4.1",  "RelatedPerson.address[0].line[0]",        "XAD", "string",    "address_component",           False, 0.85, None),
    ("NK1.4.3",  "RelatedPerson.address[0].city",           "XAD", "string",    "address_component",           False, 0.85, None),
    ("NK1.4.4",  "RelatedPerson.address[0].state",          "XAD", "string",    "address_component",           False, 0.80, None),
    ("NK1.4.5",  "RelatedPerson.address[0].postalCode",     "XAD", "string",    "address_component",           False, 0.80, None),
    ("NK1.5.4",  "RelatedPerson.telecom[0].value",          "XTN", "string",    "telecom_value",               False, 0.90, None),
    ("NK1.7",    "RelatedPerson.period.start",              "DT",  "date",      "hl7_timestamp_to_fhir_date",  False, 0.75, None),
    ("NK1.8",    "RelatedPerson.period.end",                "DT",  "date",      "hl7_timestamp_to_fhir_date",  False, 0.75, None),
    ("NK1.15",   "RelatedPerson.gender",                    "IS",  "code",      "hl7_table_0001_gender",       False, 0.85,
        {"M": "male", "F": "female", "O": "other", "U": "unknown"}),
    ("NK1.16",   "RelatedPerson.birthDate",                 "TS",  "date",      "hl7_timestamp_to_fhir_date",  False, 0.80, None),
]

IN1_COVERAGE_MAPPINGS = [
    ("IN1.1",    "Coverage.id",                             "SI",  "id",        "string_direct",               False, 0.90, None),
    ("IN1.2.1",  "Coverage.class[0].type.coding[0].code",  "CWE", "code",      "string_direct",               False, 0.90, None),
    ("IN1.3.1",  "Coverage.payor[0].identifier[0].value",  "CX",  "string",    "string_direct",               False, 0.90, None),
    ("IN1.4",    "Coverage.payor[0].display",               "ST",  "string",    "string_direct",               False, 0.95, None),
    ("IN1.12",   "Coverage.period.start",                   "DT",  "date",      "hl7_timestamp_to_fhir_date",  False, 0.90, None),
    ("IN1.13",   "Coverage.period.end",                     "DT",  "date",      "hl7_timestamp_to_fhir_date",  False, 0.90, None),
    ("IN1.15",   "Coverage.subscriberId",                   "ST",  "string",    "string_direct",               False, 0.90, None),
    ("IN1.16.1", "Coverage.subscriber.display",             "XPN", "string",    "name_component",              False, 0.85, None),
    ("IN1.17.1", "Coverage.relationship.coding[0].code",    "CWE", "code",      "insured_relationship_mapping", False, 0.85,
        {"SEL": "self", "SPO": "spouse", "PAR": "parent", "CHD": "child",
         "EME": "other", "SCH": "child", "NAL": "other", "SIB": "other",
         "WRD": "other", "FOS": "child", "DEP": "other", "EXF": "other",
         "OTH": "other", "UNK": "other"}),
    ("IN1.35",   "Coverage.type.coding[0].code",            "IS",  "code",      "insurance_type_mapping",      False, 0.80,
        {"MB": "SUBSIDIZ", "MC": "SUBSIDIZ", "CH": "CRIME", "HM": "pay", "PPO": "pay", "POS": "pay"}),
    ("IN1.36",   "Coverage.class[0].value",                 "ST",  "string",    "string_direct",               False, 0.85, None),
    ("IN1.46",   "Coverage.status",                         "IS",  "code",      "coverage_status_mapping",     False, 0.85,
        {"A": "active", "I": "cancelled", "S": "draft", "T": "draft"}),
]

TXA_DOCUMENT_REFERENCE_MAPPINGS = [
    ("TXA.1",    "DocumentReference.id",                    "SI",  "id",        "string_direct",               False, 0.90, None),
    ("TXA.2.1",  "DocumentReference.type.coding[0].code",  "IS",  "code",      "string_direct",               False, 0.90, None),
    ("TXA.3",    "DocumentReference.content[0].attachment.contentType", "ID", "code", "document_content_type", False, 0.85,
        {"TX": "text/plain", "FT": "text/plain", "CE": "text/plain",
         "RP": "application/pdf", "ED": "application/pdf", "HTML": "text/html"}),
    ("TXA.4",    "DocumentReference.date",                  "TS",  "instant",   "hl7_timestamp_to_fhir_instant", False, 0.90, None),
    ("TXA.5.1",  "DocumentReference.author[0].display",     "XCN", "string",    "name_component",              False, 0.85, None),
    ("TXA.6.1",  "DocumentReference.authenticator.display", "XCN", "string",    "name_component",              False, 0.80, None),
    ("TXA.9",    "DocumentReference.identifier[0].value",   "EI",  "string",    "string_direct",               False, 0.90, None),
    ("TXA.12.1", "DocumentReference.identifier[1].value",   "EI",  "string",    "string_direct",               False, 0.85, None),
    ("TXA.17",   "DocumentReference.status",                "ID",  "code",      "document_status_mapping",     True,  1.00,
        {"AU": "current", "CA": "entered-in-error", "OB": "superseded", "UN": "current"}),
    ("TXA.22",   "DocumentReference.content[0].attachment.title", "ST", "string", "string_direct",              False, 0.85, None),
]

SCH_APPOINTMENT_MAPPINGS = [
    ("SCH.1.1",  "Appointment.identifier[0].value",         "EI",  "string",    "string_direct",               False, 0.95, None),
    ("SCH.2.1",  "Appointment.identifier[1].value",         "EI",  "string",    "string_direct",               False, 0.90, None),
    ("SCH.6.1",  "Appointment.serviceType[0].coding[0].code", "CE", "code",     "string_direct",               False, 0.85, None),
    ("SCH.7.1",  "Appointment.specialty[0].coding[0].code", "CE",  "code",      "string_direct",               False, 0.80, None),
    ("SCH.8.1",  "Appointment.appointmentType.coding[0].code", "CE", "code",    "appointment_type_mapping",    False, 0.85,
        {"COMPLETE": "FOLLOWUP", "EMERGENCY": "EMERGENCY", "FOLLOWUP": "FOLLOWUP",
         "ROUTINE": "ROUTINE", "WALKIN": "WALKIN", "CHECKUP": "CHECKUP", "REGULAR": "ROUTINE"}),
    ("SCH.9.1",  "Appointment.reasonCode[0].coding[0].code", "CE", "code",      "string_direct",               False, 0.80, None),
    ("SCH.11.4", "Appointment.start",                       "TS",  "instant",   "hl7_timestamp_to_fhir_instant", False, 0.95, None),
    ("SCH.11.5", "Appointment.minutesDuration",             "NM",  "positiveInt", "numeric_mapping",            False, 0.90, None),
    ("SCH.12",   "Appointment.comment",                     "NM",  "string",    "string_direct",               False, 0.80, None),
    ("SCH.16.1", "Appointment.slot[0].display",             "XCN", "string",    "name_component",              False, 0.80, None),
    ("SCH.25",   "Appointment.status",                      "ID",  "code",      "appointment_status_mapping",  True,  1.00,
        {"COMPLETE": "fulfilled", "CANCELLED": "cancelled", "SCHEDULED": "booked",
         "PENDING": "pending", "WAITLIST": "waitlist", "NOSHOW": "noshow", "ENTERED-IN-ERROR": "entered-in-error"}),
]

RXE_MEDICATION_REQUEST_MAPPINGS = [
    ("RXE.1",    "MedicationRequest.dispenseRequest.quantity.value", "TQ", "decimal", "numeric_mapping",        False, 0.80, None),
    ("RXE.2.1",  "MedicationRequest.medicationCodeableConcept.coding[0].code", "CWE", "code", "string_direct",  True,  1.00, None),
    ("RXE.2.2",  "MedicationRequest.medicationCodeableConcept.coding[0].display", "CWE", "string", "string_direct", False, 0.95, None),
    ("RXE.2.3",  "MedicationRequest.medicationCodeableConcept.coding[0].system", "ID", "uri", "coding_system_mapping", False, 0.90,
        {"RXNORM": "http://www.nlm.nih.gov/research/umls/rxnorm",
         "NDC": "http://hl7.org/fhir/sid/ndc", "NDF-RT": "http://hl7.org/fhir/ndfrt"}),
    ("RXE.3",    "MedicationRequest.dispenseRequest.quantity.value", "NM", "decimal", "numeric_mapping",         False, 0.85, None),
    ("RXE.4",    "MedicationRequest.dispenseRequest.quantity.unit",  "NM", "string",  "string_direct",          False, 0.80, None),
    ("RXE.5.1",  "MedicationRequest.dosageInstruction[0].route.coding[0].code", "CE", "code", "string_direct",  False, 0.85, None),
    ("RXE.7",    "MedicationRequest.dosageInstruction[0].text",      "CE",  "string", "string_direct",          False, 0.80, None),
    ("RXE.10",   "MedicationRequest.dispenseRequest.numberOfRepeatsAllowed", "NM", "unsignedInt", "numeric_mapping", False, 0.85, None),
    ("RXE.15.1", "MedicationRequest.identifier[0].value",            "CX",  "string", "string_direct",          False, 0.90, None),
    ("RXE.21",   "MedicationRequest.status",                         "ID",  "code",   "medication_status_mapping", True, 1.00,
        {"A": "active", "D": "stopped", "C": "active", "H": "on-hold", "I": "active",
         "S": "stopped", "P": "active", "N": "on-hold"}),
    ("RXE.27.1", "MedicationRequest.dosageInstruction[0].site.coding[0].code", "CE", "code", "string_direct",   False, 0.80, None),
]

RXA_IMMUNIZATION_MAPPINGS = [
    ("RXA.1",    "Immunization.id",                         "NM",  "id",        "string_direct",               False, 0.85, None),
    ("RXA.3",    "Immunization.occurrenceDateTime",         "TS",  "dateTime",  "hl7_timestamp_to_fhir_datetime", True, 1.00, None),
    ("RXA.5.1",  "Immunization.vaccineCode.coding[0].code", "CE",  "code",      "string_direct",               True,  1.00, None),
    ("RXA.5.2",  "Immunization.vaccineCode.coding[0].display", "CE", "string",  "string_direct",               False, 0.95, None),
    ("RXA.5.3",  "Immunization.vaccineCode.coding[0].system", "ID", "uri",      "coding_system_mapping",       False, 0.90,
        {"CVX": "http://hl7.org/fhir/sid/cvx", "NDC": "http://hl7.org/fhir/sid/ndc",
         "CPT4": "http://www.ama-assn.org/go/cpt"}),
    ("RXA.6",    "Immunization.doseQuantity.value",         "NM",  "decimal",   "numeric_mapping",             False, 0.85, None),
    ("RXA.7.1",  "Immunization.doseQuantity.unit",          "CE",  "string",    "string_direct",               False, 0.80, None),
    ("RXA.9.1",  "Immunization.reportOrigin.coding[0].code", "CE", "code",      "string_direct",               False, 0.80, None),
    ("RXA.10.1", "Immunization.performer[0].actor.display", "XCN", "string",    "name_component",              False, 0.85, None),
    ("RXA.11.1", "Immunization.location.display",           "LA2", "string",    "string_direct",               False, 0.75, None),
    ("RXA.15",   "Immunization.lotNumber",                  "ST",  "string",    "string_direct",               False, 0.90, None),
    ("RXA.16",   "Immunization.expirationDate",             "DT",  "date",      "hl7_timestamp_to_fhir_date",  False, 0.85, None),
    ("RXA.17.1", "Immunization.manufacturer.display",       "CE",  "string",    "string_direct",               False, 0.85, None),
    ("RXA.20",   "Immunization.status",                     "ID",  "code",      "immunization_status_mapping", True,  1.00,
        {"CP": "completed", "PA": "not-done", "NA": "not-done", "MA": "not-done"}),
]

PR1_PROCEDURE_MAPPINGS = [
    ("PR1.1",    "Procedure.id",                            "SI",  "id",        "string_direct",               False, 0.90, None),
    ("PR1.3.1",  "Procedure.code.coding[0].code",           "CE",  "code",      "string_direct",               True,  1.00, None),
    ("PR1.3.2",  "Procedure.code.coding[0].display",        "CE",  "string",    "string_direct",               False, 0.95, None),
    ("PR1.3.3",  "Procedure.code.coding[0].system",         "ID",  "uri",       "coding_system_mapping",       False, 0.90,
        {"ICD10PCS": "http://www.cms.gov/Medicare/Coding/ICD10", "ICD-10-PCS": "http://www.cms.gov/Medicare/Coding/ICD10",
         "CPT4": "http://www.ama-assn.org/go/cpt", "C4": "http://www.ama-assn.org/go/cpt",
         "HCPCS": "http://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets",
         "SCT": "http://snomed.info/sct"}),
    ("PR1.4",    "Procedure.note[0].text",                  "ST",  "string",    "string_direct",               False, 0.80, None),
    ("PR1.5",    "Procedure.performedDateTime",              "TS",  "dateTime",  "hl7_timestamp_to_fhir_datetime", False, 0.95, None),
    ("PR1.6.1",  "Procedure.category.coding[0].code",       "IS",  "code",      "string_direct",               False, 0.80, None),
    ("PR1.7",    "Procedure.performedPeriod.end",            "NM",  "dateTime",  "hl7_timestamp_to_fhir_datetime", False, 0.75, None),
    ("PR1.8.1",  "Procedure.performer[0].actor.identifier[0].value", "XCN", "string", "string_direct",          False, 0.85, None),
    ("PR1.8.3",  "Procedure.performer[0].actor.display",    "XCN", "string",    "name_component",              False, 0.85, None),
    ("PR1.11.1", "Procedure.location.display",              "CE",  "string",    "string_direct",               False, 0.80, None),
    ("PR1.25",   "Procedure.status",                        "IS",  "code",      "procedure_status_mapping",    True,  1.00,
        {"COMPLETE": "completed", "IN-PROGRESS": "in-progress", "SCHEDULED": "preparation",
         "CANCELLED": "not-done", "ABORTED": "stopped", "HELD": "preparation"}),
]

# ──────────────────────────────────────────────────────────────────
# MESSAGE TYPE DEFINITIONS
# Each entry defines what FHIR resources the message produces
# and which segment mappings apply to each resource.
# ──────────────────────────────────────────────────────────────────

MESSAGE_TYPES = {
    # ── ADT (Admission/Discharge/Transfer) ─────────────────────────
    "ADT^A01": {
        "description": "Patient admission — produces Patient, Encounter, and MessageHeader",
        "resources": {
            "MessageHeader":    MSH_MAPPINGS,
            "Patient":          PID_PATIENT_MAPPINGS,
            "Encounter":        PV1_ENCOUNTER_MAPPINGS,
        },
        "optional_resources": {
            "AllergyIntolerance": AL1_ALLERGY_MAPPINGS,
            "Condition":          DG1_CONDITION_MAPPINGS,
        },
    },
    "ADT^A02": {
        "description": "Patient transfer — produces Patient, Encounter, and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A03": {
        "description": "Patient discharge — produces Patient, Encounter, and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A04": {
        "description": "Register outpatient — produces Patient, Encounter, and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A05": {
        "description": "Pre-admit patient — produces Patient, Encounter, and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A06": {
        "description": "Change outpatient to inpatient — produces Patient, Encounter, and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A07": {
        "description": "Change inpatient to outpatient — produces Patient, Encounter, and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A08": {
        "description": "Update patient information — produces Patient and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
        },
    },
    "ADT^A11": {
        "description": "Cancel admission — produces Patient, Encounter, and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A12": {
        "description": "Cancel transfer — produces Patient, Encounter, and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A13": {
        "description": "Cancel discharge — produces Patient, Encounter, and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A14": {
        "description": "Pending admission — produces Patient, Encounter, and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A15": {
        "description": "Pending transfer — produces Patient, Encounter, and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A16": {
        "description": "Pending discharge — produces Patient, Encounter, and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A17": {
        "description": "Swap patients — produces two Patient resources and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
        },
    },
    "ADT^A20": {
        "description": "Bed status update — produces Encounter and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A21": {
        "description": "Patient goes on leave of absence — produces Patient, Encounter, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A22": {
        "description": "Patient returns from leave of absence — produces Patient, Encounter, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A23": {
        "description": "Delete visit information — produces Patient, Encounter, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A24": {
        "description": "Link patient information — produces Patient and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
        },
    },
    "ADT^A28": {
        "description": "Add person information — produces Patient and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
        },
    },
    "ADT^A29": {
        "description": "Delete person information — produces Patient and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
        },
    },
    "ADT^A31": {
        "description": "Update person information — produces Patient and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
        },
    },
    "ADT^A34": {
        "description": "Merge patient information — produces Patient and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
        },
    },
    "ADT^A35": {
        "description": "Merge patient information — visit to account — produces Patient and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
        },
    },
    "ADT^A36": {
        "description": "Merge patient information — visit to visit — produces Patient, Encounter, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
        },
    },
    "ADT^A39": {
        "description": "Merge person — external ID — produces Patient and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
        },
    },
    "ADT^A40": {
        "description": "Merge patient — patient ID — produces Patient and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
        },
    },

    # ── ORU (Observation Result) ────────────────────────────────────
    "ORU^R01": {
        "description": "Unsolicited observation result — produces Patient, Observation, DiagnosticReport, MessageHeader",
        "resources": {
            "MessageHeader":   MSH_MAPPINGS,
            "Patient":         PID_PATIENT_MAPPINGS,
            "DiagnosticReport": OBR_DIAGNOSTIC_REPORT_MAPPINGS,
            "Observation":     OBX_OBSERVATION_MAPPINGS,
        },
    },
    "ORU^R30": {
        "description": "Point-of-care observation without prior order — produces Observation, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Observation":   OBX_OBSERVATION_MAPPINGS,
        },
    },

    # ── ORM / OMG (Orders) ──────────────────────────────────────────
    "ORM^O01": {
        "description": "General order — produces Patient, ServiceRequest, MessageHeader",
        "resources": {
            "MessageHeader":  MSH_MAPPINGS,
            "Patient":        PID_PATIENT_MAPPINGS,
            "ServiceRequest": ORC_SERVICE_REQUEST_MAPPINGS + OBR_SERVICE_REQUEST_MAPPINGS,
        },
    },
    "OMG^O19": {
        "description": "General clinical order — produces Patient, ServiceRequest, MessageHeader",
        "resources": {
            "MessageHeader":  MSH_MAPPINGS,
            "Patient":        PID_PATIENT_MAPPINGS,
            "ServiceRequest": ORC_SERVICE_REQUEST_MAPPINGS + OBR_SERVICE_REQUEST_MAPPINGS,
        },
    },
    "OML^O21": {
        "description": "Laboratory order — produces Patient, ServiceRequest, MessageHeader",
        "resources": {
            "MessageHeader":  MSH_MAPPINGS,
            "Patient":        PID_PATIENT_MAPPINGS,
            "ServiceRequest": ORC_SERVICE_REQUEST_MAPPINGS + OBR_SERVICE_REQUEST_MAPPINGS,
        },
    },

    # ── MDM (Medical Document Management) ──────────────────────────
    "MDM^T01": {
        "description": "New original document — produces Patient, DocumentReference, MessageHeader",
        "resources": {
            "MessageHeader":     MSH_MAPPINGS,
            "Patient":           PID_PATIENT_MAPPINGS,
            "DocumentReference": TXA_DOCUMENT_REFERENCE_MAPPINGS,
        },
    },
    "MDM^T02": {
        "description": "New original document with content — produces Patient, DocumentReference, MessageHeader",
        "resources": {
            "MessageHeader":     MSH_MAPPINGS,
            "Patient":           PID_PATIENT_MAPPINGS,
            "DocumentReference": TXA_DOCUMENT_REFERENCE_MAPPINGS,
        },
    },
    "MDM^T03": {
        "description": "Document status change — produces Patient, DocumentReference, MessageHeader",
        "resources": {
            "MessageHeader":     MSH_MAPPINGS,
            "Patient":           PID_PATIENT_MAPPINGS,
            "DocumentReference": TXA_DOCUMENT_REFERENCE_MAPPINGS,
        },
    },
    "MDM^T05": {
        "description": "Document addendum and notification — produces Patient, DocumentReference, MessageHeader",
        "resources": {
            "MessageHeader":     MSH_MAPPINGS,
            "Patient":           PID_PATIENT_MAPPINGS,
            "DocumentReference": TXA_DOCUMENT_REFERENCE_MAPPINGS,
        },
    },
    "MDM^T07": {
        "description": "Document edit notification — produces Patient, DocumentReference, MessageHeader",
        "resources": {
            "MessageHeader":     MSH_MAPPINGS,
            "Patient":           PID_PATIENT_MAPPINGS,
            "DocumentReference": TXA_DOCUMENT_REFERENCE_MAPPINGS,
        },
    },
    "MDM^T11": {
        "description": "Document cancel notification — produces Patient, DocumentReference, MessageHeader",
        "resources": {
            "MessageHeader":     MSH_MAPPINGS,
            "Patient":           PID_PATIENT_MAPPINGS,
            "DocumentReference": TXA_DOCUMENT_REFERENCE_MAPPINGS,
        },
    },

    # ── SIU (Scheduling) ────────────────────────────────────────────
    "SIU^S12": {
        "description": "Notification of new appointment — produces Patient, Appointment, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Appointment":   SCH_APPOINTMENT_MAPPINGS,
        },
    },
    "SIU^S13": {
        "description": "Notification of appointment rescheduling — produces Patient, Appointment, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Appointment":   SCH_APPOINTMENT_MAPPINGS,
        },
    },
    "SIU^S14": {
        "description": "Notification of appointment modification — produces Patient, Appointment, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Appointment":   SCH_APPOINTMENT_MAPPINGS,
        },
    },
    "SIU^S15": {
        "description": "Notification of appointment cancellation — produces Patient, Appointment, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Appointment":   SCH_APPOINTMENT_MAPPINGS,
        },
    },
    "SIU^S17": {
        "description": "Notification of appointment deletion — produces Patient, Appointment, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Appointment":   SCH_APPOINTMENT_MAPPINGS,
        },
    },

    # ── RDE / RAS (Pharmacy) ────────────────────────────────────────
    "RDE^O11": {
        "description": "Pharmacy/treatment encoded order — produces Patient, MedicationRequest, MessageHeader",
        "resources": {
            "MessageHeader":     MSH_MAPPINGS,
            "Patient":           PID_PATIENT_MAPPINGS,
            "MedicationRequest": RXE_MEDICATION_REQUEST_MAPPINGS,
        },
    },
    "RAS^O17": {
        "description": "Pharmacy/treatment administration — produces Patient, MedicationAdministration, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Immunization":  RXA_IMMUNIZATION_MAPPINGS,
        },
    },

    # ── VXU (Vaccination) ───────────────────────────────────────────
    "VXU^V04": {
        "description": "Unsolicited vaccination record update — produces Patient, Immunization, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Immunization":  RXA_IMMUNIZATION_MAPPINGS,
        },
    },
    "VXQ^V01": {
        "description": "Query for vaccination record — produces Patient and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
        },
    },

    # ── PPR (Problem) ───────────────────────────────────────────────
    "PPR^PC1": {
        "description": "PC/problem add — produces Patient, Condition, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Condition":     DG1_CONDITION_MAPPINGS,
        },
    },
    "PPR^PC2": {
        "description": "PC/problem update — produces Patient, Condition, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Condition":     DG1_CONDITION_MAPPINGS,
        },
    },
    "PPR^PC3": {
        "description": "PC/problem delete — produces Patient, Condition, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Condition":     DG1_CONDITION_MAPPINGS,
        },
    },

    # ── REF (Referral) ──────────────────────────────────────────────
    "REF^I12": {
        "description": "Patient referral — produces Patient, ServiceRequest, MessageHeader",
        "resources": {
            "MessageHeader":  MSH_MAPPINGS,
            "Patient":        PID_PATIENT_MAPPINGS,
            "ServiceRequest": ORC_SERVICE_REQUEST_MAPPINGS,
        },
    },
    "REF^I14": {
        "description": "Modify patient referral — produces Patient, ServiceRequest, MessageHeader",
        "resources": {
            "MessageHeader":  MSH_MAPPINGS,
            "Patient":        PID_PATIENT_MAPPINGS,
            "ServiceRequest": ORC_SERVICE_REQUEST_MAPPINGS,
        },
    },

    # ── BAR (Billing) ───────────────────────────────────────────────
    "BAR^P01": {
        "description": "Add patient accounts — produces Patient, Coverage, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Coverage":      IN1_COVERAGE_MAPPINGS,
        },
    },
    "BAR^P02": {
        "description": "Purge patient accounts — produces Patient and MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
        },
    },

    # ── DFT (Detailed Financial Transactions) ──────────────────────
    "DFT^P03": {
        "description": "Post detail financial transaction — produces Patient, ChargeItem, MessageHeader",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
        },
    },

    # ── Procedures ─────────────────────────────────────────────────
    "ADT^A01_WITH_PR1": {
        "description": "Admission with procedures — ADT^A01 variant including Procedure resources",
        "resources": {
            "MessageHeader": MSH_MAPPINGS,
            "Patient":       PID_PATIENT_MAPPINGS,
            "Encounter":     PV1_ENCOUNTER_MAPPINGS,
            "Procedure":     PR1_PROCEDURE_MAPPINGS,
        },
    },
}


def make_mapping_entry(row):
    """Convert a mapping tuple to a dict suitable for JSON serialization."""
    hl7Path, fhirPath, hl7DataType, fhirDataType, transform, required, confidence, valueMap = row
    entry = {
        "hl7Path":      hl7Path,
        "fhirPath":     fhirPath,
        "hl7DataType":  hl7DataType,
        "fhirDataType": fhirDataType,
        "transform":    transform,
        "required":     required,
        "confidence":   confidence,
    }
    if valueMap:
        entry["valueMap"] = valueMap
    return entry


def build_template_config(msg_def):
    """Build the template_config JSON for a message type definition."""
    resources = {}
    for resource_type, mappings in msg_def["resources"].items():
        resources[resource_type] = {
            "mappings": [make_mapping_entry(m) for m in mappings]
        }

    # Add optional resources (documented but not always present)
    for resource_type, mappings in msg_def.get("optional_resources", {}).items():
        resources[resource_type] = {
            "optional": True,
            "mappings": [make_mapping_entry(m) for m in mappings]
        }

    return {
        "version": "1.0",
        "fhirVersion": "R4",
        "generatedBy": "hl7-fhir-template-generator",
        "resources": resources,
    }


def escape_sql_string(s):
    """Escape a string for use in a SQL single-quoted literal."""
    return s.replace("'", "''")


def generate_sql():
    lines = []
    lines.append("-- V62: Comprehensive HL7 v2 to FHIR R4 field-level mapping templates")
    lines.append(f"-- Generated: {datetime.now().strftime('%Y-%m-%d %H:%M UTC')}")
    lines.append("-- Covers: ADT, ORU, ORM/OMG/OML, MDM, SIU, RDE, RAS, VXU, PPR, REF, BAR, DFT")
    lines.append("-- Each row represents one message type's complete field-level FHIR mapping")
    lines.append("")
    lines.append("BEGIN;")
    lines.append("")

    # Extend the hl7_fhir_templates table with columns needed for comprehensive templates
    lines.append("-- Extend hl7_fhir_templates with metadata columns if not already present")
    lines.append("ALTER TABLE hl7_fhir_templates")
    lines.append("    ADD COLUMN IF NOT EXISTS fhir_resources JSONB,")
    lines.append("    ADD COLUMN IF NOT EXISTS template_type VARCHAR(50) DEFAULT 'field_mapping',")
    lines.append("    ADD COLUMN IF NOT EXISTS is_system BOOLEAN DEFAULT false,")
    lines.append("    ADD COLUMN IF NOT EXISTS is_public BOOLEAN DEFAULT true,")
    lines.append("    ADD COLUMN IF NOT EXISTS version VARCHAR(10) DEFAULT '1.0';")
    lines.append("")
    lines.append("-- Drop the old constraint that only allowed one default per (message_type, version)")
    lines.append("-- and replace with one that allows multiple message types (one default per type + version)")
    lines.append("ALTER TABLE hl7_fhir_templates")
    lines.append("    DROP CONSTRAINT IF EXISTS unique_default_template;")
    lines.append("ALTER TABLE hl7_fhir_templates")
    lines.append("    ADD CONSTRAINT IF NOT EXISTS unique_default_template_v2")
    lines.append("        UNIQUE (message_type, hl7_version, is_default);")
    lines.append("")
    lines.append("-- Remove any existing auto-generated OOB templates so we can re-seed cleanly")
    lines.append("DELETE FROM hl7_fhir_templates WHERE template_name LIKE 'OOB_%';")
    lines.append("")

    for msg_type, msg_def in MESSAGE_TYPES.items():
        template_id = str(uuid.uuid4())
        template_name = f"OOB_{msg_type.replace('^', '_')}"
        description = msg_def["description"]
        config = build_template_config(msg_def)
        config_json = json.dumps(config, ensure_ascii=False, separators=(',', ':'))

        # Collect just the resource type list for the fhir_resources column
        fhir_resources = list(msg_def["resources"].keys())
        optional_resources = list(msg_def.get("optional_resources", {}).keys())
        all_resources = fhir_resources + optional_resources
        fhir_resources_json = json.dumps(all_resources)

        lines.append(f"-- {msg_type}: {description}")
        lines.append(f"INSERT INTO hl7_fhir_templates (")
        lines.append(f"    id, template_name, message_type, hl7_version, template_type, template_description,")
        lines.append(f"    fhir_resources, template_config, is_default, is_system, is_public,")
        lines.append(f"    version, created_at, updated_at")
        lines.append(f") VALUES (")
        lines.append(f"    '{template_id}',")
        lines.append(f"    '{escape_sql_string(template_name)}',")
        lines.append(f"    '{escape_sql_string(msg_type)}',")
        lines.append(f"    '2.5',")
        lines.append(f"    'field_mapping',")
        lines.append(f"    '{escape_sql_string(description)}',")
        lines.append(f"    '{escape_sql_string(fhir_resources_json)}'::jsonb,")
        lines.append(f"    '{escape_sql_string(config_json)}'::jsonb,")
        lines.append(f"    true,")
        lines.append(f"    true,")
        lines.append(f"    true,")
        lines.append(f"    '1.0',")
        lines.append(f"    NOW(),")
        lines.append(f"    NOW()")
        lines.append(f") ON CONFLICT (message_type, hl7_version, is_default)")
        lines.append(f"  DO UPDATE SET")
        lines.append(f"    template_name    = EXCLUDED.template_name,")
        lines.append(f"    template_config  = EXCLUDED.template_config,")
        lines.append(f"    template_description = EXCLUDED.template_description,")
        lines.append(f"    fhir_resources   = EXCLUDED.fhir_resources,")
        lines.append(f"    is_system        = true,")
        lines.append(f"    updated_at       = NOW();")
        lines.append("")

    lines.append("COMMIT;")
    return "\n".join(lines)


if __name__ == "__main__":
    print(generate_sql())
