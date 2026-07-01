-- V187__CDA_Declarative_Mapping_Rules_Coverage_IG_Alignment.sql
--
-- Re-seeds both Coverage rules (payersInsurance + payors section keys) to
-- align with the C-CDA on FHIR IG (HL7, R4 target). Previously all fields
-- were read from the outer Coverage Activity; the IG specifies that the
-- Coverage Activity is a container only -- all insurance data lives in the
-- COMP-nested Policy Activity.
--
-- Fixed field sources (all five corrected):
--
--   type      <- entryRelationships[typeCode=COMP].entry.code
--                (SOP 2.16.840.1.113883.3.221.5 "1"=Medicare, "5"=BCBS, etc.)
--                Was: outer Coverage Activity code (always 48768-6, meaningless
--                as a coverage type).
--
--   period    <- entryRelationships[typeCode=COMP].entry.participants[typeCode=COV].time
--                (authoritative coverage window: low=start, high=end)
--                Was: outer Coverage Activity effectiveTime (document date).
--
--   payor     <- entryRelationships[typeCode=COMP].entry.performers[0]
--                  .assignedEntity.representedOrganization.names[0]
--                (PAYOR performer listed first per C-CDA conformance)
--                Was: outer HLD/PRF participant scopingEntity -- path not
--                evidenced in any of the 4 corpus files; fell back to COMP
--                performers[0] in practice but was not the primary path.
--
--   relationship <- entryRelationships[typeCode=COMP].entry
--                     .participants[typeCode=COV].participantRole.code
--                   (HL7 RoleCode SELF/SPOUSE/CHILD -> FHIR subscriber-relationship VS)
--                   New coverage_relationship_to_fhir transform handles mapping.
--                   Was: hardcoded literal "self" unconditionally.
--
--   subscriberId <- entryRelationships[typeCode=COMP].entry
--                     .participants[typeCode=COV].participantRole.ids[0]
--                   (member ID assigned by the payer)
--                   Was: Policy Activity's own id (contract/policy identifier,
--                   often nullFlavor=UNK in real data -- Dignity Health, Epic).
--
-- New fallback: relationship defaults to literal "self" when COV participant
-- has no code (SkipIfResourceHasAnyOf gate prevents double-write).
--
-- Corpus evidence: Epic 99397 (Medicare SOP "1", COV time 20240101-NA,
-- relationship SELF, MEDICARE org), Dignity Health (SOP "5", member ID present),
-- Marshfield (SOP "5", BCBS), Ascension (no payers section).

-- payersInsurance section key
INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'payersInsurance', 'Coverage', '', 0,
    $rules$
[
  {
    "literalValue": "active",
    "targetPath": "status"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry",
    "sourcePath": "code",
    "transform": "coverage_type_to_codeable_concept",
    "targetPath": "type"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry.participants[typeCode=COV]",
    "sourcePath": "time",
    "transform": "cda_timerange_to_period",
    "targetPath": "period"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry.performers[0].assignedEntity.representedOrganization.names[0]",
    "targetPath": "payor[0].display"
  },
  {
    "literalValue": "Unknown",
    "targetPath": "payor[0].display",
    "skipIfResourceHasAnyOf": ["payor"]
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry.participants[typeCode=COV].participantRole",
    "sourcePath": "code",
    "transform": "coverage_relationship_to_fhir",
    "targetPath": "relationship"
  },
  {
    "literalValue": {
      "coding": [
        {
          "system": "http://terminology.hl7.org/CodeSystem/subscriber-relationship",
          "code": "self",
          "display": "Self"
        }
      ]
    },
    "targetPath": "relationship",
    "skipIfResourceHasAnyOf": ["relationship"]
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry.participants[typeCode=COV].participantRole",
    "sourcePath": "ids[0].extension",
    "fallbackPaths": ["ids[0].root"],
    "targetPath": "subscriberId"
  }
]
    $rules$::jsonb,
    false, false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- payors section key (alias used by some EHRs -- identical fields)
INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'payors', 'Coverage', '', 0,
    $rules$
[
  {
    "literalValue": "active",
    "targetPath": "status"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry",
    "sourcePath": "code",
    "transform": "coverage_type_to_codeable_concept",
    "targetPath": "type"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry.participants[typeCode=COV]",
    "sourcePath": "time",
    "transform": "cda_timerange_to_period",
    "targetPath": "period"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry.performers[0].assignedEntity.representedOrganization.names[0]",
    "targetPath": "payor[0].display"
  },
  {
    "literalValue": "Unknown",
    "targetPath": "payor[0].display",
    "skipIfResourceHasAnyOf": ["payor"]
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry.participants[typeCode=COV].participantRole",
    "sourcePath": "code",
    "transform": "coverage_relationship_to_fhir",
    "targetPath": "relationship"
  },
  {
    "literalValue": {
      "coding": [
        {
          "system": "http://terminology.hl7.org/CodeSystem/subscriber-relationship",
          "code": "self",
          "display": "Self"
        }
      ]
    },
    "targetPath": "relationship",
    "skipIfResourceHasAnyOf": ["relationship"]
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry.participants[typeCode=COV].participantRole",
    "sourcePath": "ids[0].extension",
    "fallbackPaths": ["ids[0].root"],
    "targetPath": "subscriberId"
  }
]
    $rules$::jsonb,
    false, false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();
