-- V115: HL7 v2→FHIR IG — Valueset Maps and Field Anchors
--
-- Piece 2: Populate hl7_fhir_value_mappings with authoritative ConceptMap data
--          from the HL7 v2-to-FHIR R4 Implementation Guide (hl7.org/fhir/uv/v2mappings).
--          Replaces scattered hardcoded Go switch statements.
--
-- Piece 3: Create ig_field_anchors table — stores the IG's segment/field→FHIR path
--          mappings as base data.  The OOB template builder reads this table first;
--          heuristic semantic matching is the fallback.
--
-- Source: https://hl7.org/fhir/uv/v2mappings/ (R4 edition)
-- Tables cited by IG table number (e.g. HL7 Table 0001 = Administrative Sex).

BEGIN;

-- ─────────────────────────────────────────────────────────────────────────────
-- Improve hl7_fhir_value_mappings (created in V5 without PK or indexes)
-- ─────────────────────────────────────────────────────────────────────────────
ALTER TABLE hl7_fhir_value_mappings
    ADD COLUMN IF NOT EXISTS id          UUID    DEFAULT gen_random_uuid(),
    ADD COLUMN IF NOT EXISTS ig_source   VARCHAR(20) DEFAULT 'v2-to-fhir-r4',
    ADD COLUMN IF NOT EXISTS is_active   BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS created_at  TIMESTAMPTZ DEFAULT NOW();

CREATE UNIQUE INDEX IF NOT EXISTS idx_hl7_value_mappings_lookup
    ON hl7_fhir_value_mappings (hl7_table, UPPER(hl7_value)) WHERE is_active;

CREATE INDEX IF NOT EXISTS idx_hl7_value_mappings_table
    ON hl7_fhir_value_mappings (hl7_table) WHERE is_active;

-- ─────────────────────────────────────────────────────────────────────────────
-- IG ConceptMap seed data
-- ─────────────────────────────────────────────────────────────────────────────

-- Table 0001 — Administrative Sex → FHIR administrative-gender
-- IG: http://terminology.hl7.org/CodeSystem/v2-0001
-- FHIR: http://hl7.org/fhir/administrative-gender
INSERT INTO hl7_fhir_value_mappings (hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, ig_source)
VALUES
    ('0001','M','http://hl7.org/fhir/administrative-gender','male','Male','v2-to-fhir-r4'),
    ('0001','F','http://hl7.org/fhir/administrative-gender','female','Female','v2-to-fhir-r4'),
    ('0001','O','http://hl7.org/fhir/administrative-gender','other','Other','v2-to-fhir-r4'),
    ('0001','U','http://hl7.org/fhir/administrative-gender','unknown','Unknown','v2-to-fhir-r4'),
    ('0001','A','http://hl7.org/fhir/administrative-gender','other','Ambiguous','v2-to-fhir-r4'),
    ('0001','N','http://hl7.org/fhir/administrative-gender','other','Not Applicable','v2-to-fhir-r4')
ON CONFLICT DO NOTHING;

-- Table 0002 — Marital Status → FHIR v3-MaritalStatus
-- IG: http://terminology.hl7.org/CodeSystem/v2-0002
-- FHIR: http://terminology.hl7.org/CodeSystem/v3-MaritalStatus
INSERT INTO hl7_fhir_value_mappings (hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, ig_source)
VALUES
    ('0002','A','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','A','Annulled','v2-to-fhir-r4'),
    ('0002','D','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','D','Divorced','v2-to-fhir-r4'),
    ('0002','I','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','I','Interlocutory','v2-to-fhir-r4'),
    ('0002','L','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','L','Legally Separated','v2-to-fhir-r4'),
    ('0002','M','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','M','Married','v2-to-fhir-r4'),
    ('0002','P','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','P','Polygamous','v2-to-fhir-r4'),
    ('0002','S','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','S','Never Married','v2-to-fhir-r4'),
    ('0002','T','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','T','Domestic Partner','v2-to-fhir-r4'),
    ('0002','W','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','W','Widowed','v2-to-fhir-r4'),
    ('0002','C','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','C','Common Law','v2-to-fhir-r4'),
    ('0002','E','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','L','Legally Separated','v2-to-fhir-r4'),
    ('0002','G','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','T','Living Together','v2-to-fhir-r4'),
    ('0002','N','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','A','Annulled','v2-to-fhir-r4'),
    ('0002','O','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','UNK','Other','v2-to-fhir-r4'),
    ('0002','R','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','T','Registered Domestic Partner','v2-to-fhir-r4'),
    ('0002','U','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','UNK','Unknown','v2-to-fhir-r4'),
    ('0002','B','http://terminology.hl7.org/CodeSystem/v3-MaritalStatus','S','Unmarried','v2-to-fhir-r4')
ON CONFLICT DO NOTHING;

-- Table 0004 — Patient Class → FHIR v3 ActCode (encounter class)
-- IG: http://terminology.hl7.org/CodeSystem/v2-0004
-- FHIR: http://terminology.hl7.org/CodeSystem/v3-ActCode
INSERT INTO hl7_fhir_value_mappings (hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, ig_source)
VALUES
    ('0004','E','http://terminology.hl7.org/CodeSystem/v3-ActCode','EMER','Emergency','v2-to-fhir-r4'),
    ('0004','I','http://terminology.hl7.org/CodeSystem/v3-ActCode','IMP','Inpatient Encounter','v2-to-fhir-r4'),
    ('0004','O','http://terminology.hl7.org/CodeSystem/v3-ActCode','AMB','Ambulatory','v2-to-fhir-r4'),
    ('0004','P','http://terminology.hl7.org/CodeSystem/v3-ActCode','PRENC','Pre-Admission','v2-to-fhir-r4'),
    ('0004','R','http://terminology.hl7.org/CodeSystem/v3-ActCode','AMB','Recurring Patient','v2-to-fhir-r4'),
    ('0004','B','http://terminology.hl7.org/CodeSystem/v3-ActCode','IMP','Obstetrics','v2-to-fhir-r4'),
    ('0004','C','http://terminology.hl7.org/CodeSystem/v3-ActCode','AMB','Commercial Account','v2-to-fhir-r4'),
    ('0004','N','http://terminology.hl7.org/CodeSystem/v3-ActCode','AMB','Not Applicable','v2-to-fhir-r4'),
    ('0004','U','http://terminology.hl7.org/CodeSystem/v3-ActCode','AMB','Unknown','v2-to-fhir-r4')
ON CONFLICT DO NOTHING;

-- Table 0085 — Observation Result Status → FHIR observation-status
-- IG: http://terminology.hl7.org/CodeSystem/v2-0085
-- FHIR: http://hl7.org/fhir/observation-status
INSERT INTO hl7_fhir_value_mappings (hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, ig_source)
VALUES
    ('0085','F','http://hl7.org/fhir/observation-status','final','Final','v2-to-fhir-r4'),
    ('0085','P','http://hl7.org/fhir/observation-status','preliminary','Preliminary','v2-to-fhir-r4'),
    ('0085','C','http://hl7.org/fhir/observation-status','corrected','Corrected','v2-to-fhir-r4'),
    ('0085','R','http://hl7.org/fhir/observation-status','registered','Registered','v2-to-fhir-r4'),
    ('0085','X','http://hl7.org/fhir/observation-status','cancelled','Cancelled','v2-to-fhir-r4'),
    ('0085','I','http://hl7.org/fhir/observation-status','registered','In Process','v2-to-fhir-r4'),
    ('0085','S','http://hl7.org/fhir/observation-status','preliminary','Partial','v2-to-fhir-r4'),
    ('0085','U','http://hl7.org/fhir/observation-status','final','Result Status Changed to Final','v2-to-fhir-r4'),
    ('0085','W','http://hl7.org/fhir/observation-status','entered-in-error','Wrong','v2-to-fhir-r4'),
    ('0085','A','http://hl7.org/fhir/observation-status','amended','Amended','v2-to-fhir-r4'),
    ('0085','B','http://hl7.org/fhir/observation-status','amended','Appended','v2-to-fhir-r4'),
    ('0085','D','http://hl7.org/fhir/observation-status','final','Detached','v2-to-fhir-r4'),
    ('0085','O','http://hl7.org/fhir/observation-status','unknown','Order Detail','v2-to-fhir-r4'),
    ('0085','V','http://hl7.org/fhir/observation-status','final','Verified','v2-to-fhir-r4')
ON CONFLICT DO NOTHING;

-- Table 0116 — Bed Status → FHIR v2-0116
INSERT INTO hl7_fhir_value_mappings (hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, ig_source)
VALUES
    ('0116','C','http://terminology.hl7.org/CodeSystem/v2-0116','C','Closed','v2-to-fhir-r4'),
    ('0116','H','http://terminology.hl7.org/CodeSystem/v2-0116','H','Housekeeping','v2-to-fhir-r4'),
    ('0116','I','http://terminology.hl7.org/CodeSystem/v2-0116','I','Isolated','v2-to-fhir-r4'),
    ('0116','K','http://terminology.hl7.org/CodeSystem/v2-0116','K','Contaminated','v2-to-fhir-r4'),
    ('0116','O','http://terminology.hl7.org/CodeSystem/v2-0116','O','Occupied','v2-to-fhir-r4'),
    ('0116','U','http://terminology.hl7.org/CodeSystem/v2-0116','U','Unoccupied','v2-to-fhir-r4')
ON CONFLICT DO NOTHING;

-- Table 0136 — Yes/No Indicator → boolean string
-- Engine reads fhir_code and converts to bool downstream.
INSERT INTO hl7_fhir_value_mappings (hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, ig_source)
VALUES
    ('0136','Y','http://terminology.hl7.org/CodeSystem/v2-0136','true','Yes','v2-to-fhir-r4'),
    ('0136','N','http://terminology.hl7.org/CodeSystem/v2-0136','false','No','v2-to-fhir-r4')
ON CONFLICT DO NOTHING;

-- Table 0183 — Active/Inactive → FHIR boolean string
-- Maps STF.7 (Active/Inactive) for Practitioner.active.
INSERT INTO hl7_fhir_value_mappings (hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, ig_source)
VALUES
    ('0183','A','http://terminology.hl7.org/CodeSystem/v2-0183','true','Active','v2-to-fhir-r4'),
    ('0183','I','http://terminology.hl7.org/CodeSystem/v2-0183','false','Inactive','v2-to-fhir-r4')
ON CONFLICT DO NOTHING;

-- Table 0190 — Address Type → FHIR address-use
-- IG: http://terminology.hl7.org/CodeSystem/v2-0190
-- FHIR: http://hl7.org/fhir/address-use
INSERT INTO hl7_fhir_value_mappings (hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, ig_source)
VALUES
    ('0190','B','http://hl7.org/fhir/address-use','work','Business','v2-to-fhir-r4'),
    ('0190','C','http://hl7.org/fhir/address-use','temp','Current','v2-to-fhir-r4'),
    ('0190','F','http://hl7.org/fhir/address-use','home','Country of Origin','v2-to-fhir-r4'),
    ('0190','H','http://hl7.org/fhir/address-use','home','Home','v2-to-fhir-r4'),
    ('0190','L','http://hl7.org/fhir/address-use','home','Legal Address','v2-to-fhir-r4'),
    ('0190','M','http://hl7.org/fhir/address-use','billing','Mailing','v2-to-fhir-r4'),
    ('0190','N','http://hl7.org/fhir/address-use','billing','Birth (nee)','v2-to-fhir-r4'),
    ('0190','O','http://hl7.org/fhir/address-use','work','Office','v2-to-fhir-r4'),
    ('0190','P','http://hl7.org/fhir/address-use','home','Permanent','v2-to-fhir-r4'),
    ('0190','RH','http://hl7.org/fhir/address-use','temp','Registry Home','v2-to-fhir-r4'),
    ('0190','BA','http://hl7.org/fhir/address-use','billing','Bad Address','v2-to-fhir-r4')
ON CONFLICT DO NOTHING;

-- Table 0200 — Name Type → FHIR name-use
-- IG: http://terminology.hl7.org/CodeSystem/v2-0200
-- FHIR: http://hl7.org/fhir/name-use
INSERT INTO hl7_fhir_value_mappings (hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, ig_source)
VALUES
    ('0200','A','http://hl7.org/fhir/name-use','anonymous','Alias Name','v2-to-fhir-r4'),
    ('0200','B','http://hl7.org/fhir/name-use','old','Name at Birth','v2-to-fhir-r4'),
    ('0200','C','http://hl7.org/fhir/name-use','official','Adopted Name','v2-to-fhir-r4'),
    ('0200','D','http://hl7.org/fhir/name-use','usual','Display Name','v2-to-fhir-r4'),
    ('0200','I','http://hl7.org/fhir/name-use','official','Licensing Name','v2-to-fhir-r4'),
    ('0200','L','http://hl7.org/fhir/name-use','official','Legal Name','v2-to-fhir-r4'),
    ('0200','M','http://hl7.org/fhir/name-use','maiden','Maiden Name','v2-to-fhir-r4'),
    ('0200','N','http://hl7.org/fhir/name-use','nickname','Nickname','v2-to-fhir-r4'),
    ('0200','P','http://hl7.org/fhir/name-use','anonymous','Pseudonym','v2-to-fhir-r4'),
    ('0200','R','http://hl7.org/fhir/name-use','official','Registered Name','v2-to-fhir-r4'),
    ('0200','S','http://hl7.org/fhir/name-use','official','Coded Pseudo','v2-to-fhir-r4'),
    ('0200','T','http://hl7.org/fhir/name-use','temp','Tribal/Clan Name','v2-to-fhir-r4'),
    ('0200','U','http://hl7.org/fhir/name-use','anonymous','Unspecified','v2-to-fhir-r4')
ON CONFLICT DO NOTHING;

-- Table 0201 — Telecommunication Use Code → FHIR ContactPoint.use
-- IG: http://terminology.hl7.org/CodeSystem/v2-0201
-- FHIR: http://hl7.org/fhir/contact-point-use
INSERT INTO hl7_fhir_value_mappings (hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, ig_source)
VALUES
    ('0201','ASN','http://hl7.org/fhir/contact-point-use','home','Answering Service Number','v2-to-fhir-r4'),
    ('0201','BPN','http://hl7.org/fhir/contact-point-use','mobile','Beeper Number','v2-to-fhir-r4'),
    ('0201','EMR','http://hl7.org/fhir/contact-point-use','temp','Emergency Number','v2-to-fhir-r4'),
    ('0201','NET','http://hl7.org/fhir/contact-point-use','home','Network (Email) Address','v2-to-fhir-r4'),
    ('0201','ORN','http://hl7.org/fhir/contact-point-use','old','Other Residence Number','v2-to-fhir-r4'),
    ('0201','PRN','http://hl7.org/fhir/contact-point-use','home','Primary Residence Number','v2-to-fhir-r4'),
    ('0201','PRS','http://hl7.org/fhir/contact-point-use','home','Personal','v2-to-fhir-r4'),
    ('0201','VHN','http://hl7.org/fhir/contact-point-use','temp','Vacation Home Number','v2-to-fhir-r4'),
    ('0201','WPN','http://hl7.org/fhir/contact-point-use','work','Work Number','v2-to-fhir-r4')
ON CONFLICT DO NOTHING;

-- Table 0202 — Telecommunication Equipment Type → FHIR ContactPoint.system
-- IG: http://terminology.hl7.org/CodeSystem/v2-0202
-- FHIR: http://hl7.org/fhir/contact-point-system
INSERT INTO hl7_fhir_value_mappings (hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, ig_source)
VALUES
    ('0202','BP',      'http://hl7.org/fhir/contact-point-system','pager','Beeper','v2-to-fhir-r4'),
    ('0202','CP',      'http://hl7.org/fhir/contact-point-system','phone','Cellular Phone','v2-to-fhir-r4'),
    ('0202','FX',      'http://hl7.org/fhir/contact-point-system','fax','Fax','v2-to-fhir-r4'),
    ('0202','Internet','http://hl7.org/fhir/contact-point-system','email','Internet Address','v2-to-fhir-r4'),
    ('0202','MD',      'http://hl7.org/fhir/contact-point-system','other','Modem','v2-to-fhir-r4'),
    ('0202','PH',      'http://hl7.org/fhir/contact-point-system','phone','Telephone','v2-to-fhir-r4'),
    ('0202','SAT',     'http://hl7.org/fhir/contact-point-system','other','Satellite Phone','v2-to-fhir-r4'),
    ('0202','TDD',     'http://hl7.org/fhir/contact-point-system','other','Telecommunications Device for Deaf','v2-to-fhir-r4'),
    ('0202','TTY',     'http://hl7.org/fhir/contact-point-system','other','Teletypewriter','v2-to-fhir-r4'),
    ('0202','X.400',   'http://hl7.org/fhir/contact-point-system','email','X.400 Email Address','v2-to-fhir-r4')
ON CONFLICT DO NOTHING;

-- Table 0203 — Identifier Type → FHIR identifier-type
-- IG: http://terminology.hl7.org/CodeSystem/v2-0203
-- FHIR: http://terminology.hl7.org/CodeSystem/v2-0203 (passthrough — IG keeps same system)
INSERT INTO hl7_fhir_value_mappings (hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, ig_source)
VALUES
    ('0203','DL', 'http://terminology.hl7.org/CodeSystem/v2-0203','DL','Driver License Number','v2-to-fhir-r4'),
    ('0203','MR', 'http://terminology.hl7.org/CodeSystem/v2-0203','MR','Medical Record Number','v2-to-fhir-r4'),
    ('0203','NPI','http://terminology.hl7.org/CodeSystem/v2-0203','NPI','National Provider Identifier','v2-to-fhir-r4'),
    ('0203','PI', 'http://terminology.hl7.org/CodeSystem/v2-0203','PI','Patient Internal Identifier','v2-to-fhir-r4'),
    ('0203','PT', 'http://terminology.hl7.org/CodeSystem/v2-0203','PT','Patient External Identifier','v2-to-fhir-r4'),
    ('0203','PPN','http://terminology.hl7.org/CodeSystem/v2-0203','PPN','Passport Number','v2-to-fhir-r4'),
    ('0203','SB', 'http://terminology.hl7.org/CodeSystem/v2-0203','SB','Social Beneficiary Identifier','v2-to-fhir-r4'),
    ('0203','SS', 'http://terminology.hl7.org/CodeSystem/v2-0203','SS','Social Security Number','v2-to-fhir-r4'),
    ('0203','VN', 'http://terminology.hl7.org/CodeSystem/v2-0203','VN','Visit Number','v2-to-fhir-r4'),
    ('0203','AN', 'http://terminology.hl7.org/CodeSystem/v2-0203','AN','Account Number','v2-to-fhir-r4'),
    ('0203','DEA','http://terminology.hl7.org/CodeSystem/v2-0203','DEA','DEA Number','v2-to-fhir-r4'),
    ('0203','EN', 'http://terminology.hl7.org/CodeSystem/v2-0203','EN','Employer Number','v2-to-fhir-r4'),
    ('0203','NI', 'http://terminology.hl7.org/CodeSystem/v2-0203','NI','National Unique Individual Identifier','v2-to-fhir-r4'),
    ('0203','TN', 'http://terminology.hl7.org/CodeSystem/v2-0203','TN','Treaty Number','v2-to-fhir-r4')
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- Piece 3: ig_field_anchors — IG-authoritative segment/field → FHIR path
--
-- The OOB template builder queries this table (segment + field_pos) before
-- falling back to heuristic semantic matching in semantic_matcher.go.
-- confidence = 1.0 means "spec says so"; heuristic scores are 0.5–0.95.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS ig_field_anchors (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    segment      VARCHAR(10) NOT NULL,
    field_pos    VARCHAR(20) NOT NULL,   -- "3", "3.1", "3.1.1" (no segment prefix)
    fhir_resource VARCHAR(60) NOT NULL,
    fhir_path    VARCHAR(200) NOT NULL,  -- relative path within resource
    fhir_type    VARCHAR(50),
    hl7_table    VARCHAR(20),            -- NULL when field is not a coded table
    confidence   NUMERIC(4,3) NOT NULL DEFAULT 1.000,
    ig_source    VARCHAR(20)  NOT NULL DEFAULT 'v2-to-fhir-r4',
    condition    TEXT,                   -- FHIRPath condition (NULL = always apply)
    is_active    BOOLEAN      NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ig_field_anchors_lookup
    ON ig_field_anchors (segment, field_pos, fhir_resource) WHERE is_active;

CREATE INDEX IF NOT EXISTS idx_ig_field_anchors_segment
    ON ig_field_anchors (segment, is_active);

COMMENT ON TABLE ig_field_anchors IS
    'HL7 v2→FHIR R4 IG field-level anchors. Source: https://hl7.org/fhir/uv/v2mappings/';

-- ─────────────────────────────────────────────────────────────────────────────
-- MSH — MessageHeader
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO ig_field_anchors (segment, field_pos, fhir_resource, fhir_path, fhir_type, hl7_table, confidence) VALUES
    ('MSH','3.1',  'MessageHeader','source.name',              'string',  NULL,   1.000),
    ('MSH','3.2',  'MessageHeader','source.endpoint',          'url',     NULL,   0.950),
    ('MSH','4.1',  'MessageHeader','source.endpoint',          'url',     NULL,   0.900),
    ('MSH','5.1',  'MessageHeader','destination[0].name',      'string',  NULL,   1.000),
    ('MSH','5.2',  'MessageHeader','destination[0].endpoint',  'url',     NULL,   0.950),
    ('MSH','7',    'MessageHeader','meta.lastUpdated',         'instant', NULL,   1.000),
    ('MSH','9.1',  'MessageHeader','eventCoding.code',         'code',    NULL,   1.000),
    ('MSH','9.2',  'MessageHeader','eventCoding.code',         'code',    NULL,   0.980),
    ('MSH','10',   'MessageHeader','id',                       'id',      NULL,   1.000),
    ('MSH','12',   'MessageHeader','meta.extension[0].valueString','string',NULL, 0.850)
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- PID — Patient
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO ig_field_anchors (segment, field_pos, fhir_resource, fhir_path, fhir_type, hl7_table, confidence) VALUES
    ('PID','3',    'Patient','identifier',              'Identifier',  NULL,   1.000),
    ('PID','3.1',  'Patient','identifier[0].value',     'string',      NULL,   1.000),
    ('PID','3.4',  'Patient','identifier[0].assigner.display','string',NULL,   0.900),
    ('PID','3.5',  'Patient','identifier[0].type.coding[0].code','code','0203',0.950),
    ('PID','5',    'Patient','name',                    'HumanName',   NULL,   1.000),
    ('PID','5.1',  'Patient','name[0].family',          'string',      NULL,   1.000),
    ('PID','5.2',  'Patient','name[0].given[0]',        'string',      NULL,   1.000),
    ('PID','5.3',  'Patient','name[0].given[1]',        'string',      NULL,   0.970),
    ('PID','5.5',  'Patient','name[0].prefix[0]',       'string',      NULL,   0.950),
    ('PID','5.4',  'Patient','name[0].suffix[0]',       'string',      NULL,   0.940),
    ('PID','5.7',  'Patient','name[0].use',             'code',        '0200', 0.950),
    ('PID','7',    'Patient','birthDate',               'date',        NULL,   1.000),
    ('PID','8',    'Patient','gender',                  'code',        '0001', 1.000),
    ('PID','10',   'Patient','extension[us-core-race].valueCoding.code','code',NULL,0.800),
    ('PID','11',   'Patient','address',                 'Address',     NULL,   1.000),
    ('PID','11.1', 'Patient','address[0].line[0]',      'string',      NULL,   1.000),
    ('PID','11.2', 'Patient','address[0].line[1]',      'string',      NULL,   0.950),
    ('PID','11.3', 'Patient','address[0].city',         'string',      NULL,   1.000),
    ('PID','11.4', 'Patient','address[0].state',        'string',      NULL,   1.000),
    ('PID','11.5', 'Patient','address[0].postalCode',   'string',      NULL,   1.000),
    ('PID','11.6', 'Patient','address[0].country',      'string',      NULL,   1.000),
    ('PID','11.7', 'Patient','address[0].use',          'code',        '0190', 0.950),
    ('PID','13',   'Patient','telecom',                 'ContactPoint',NULL,   1.000),
    ('PID','13.4', 'Patient','telecom[0].value',        'string',      NULL,   0.950),
    ('PID','13.3', 'Patient','telecom[0].system',       'code',        '0202', 0.920),
    ('PID','13.2', 'Patient','telecom[0].use',          'code',        '0201', 0.900),
    ('PID','14.4', 'Patient','telecom[1].value',        'string',      NULL,   0.940),
    ('PID','14.3', 'Patient','telecom[1].system',       'code',        '0202', 0.900),
    ('PID','19',   'Patient','identifier[2].value',     'string',      NULL,   0.900),
    ('PID','26',   'Patient','extension[patient-citizenship].valueCodeableConcept.text','string',NULL,0.750),
    ('PID','29',   'Patient','deceasedDateTime',        'dateTime',    NULL,   0.950),
    ('PID','30',   'Patient','deceasedBoolean',         'boolean',     '0136', 0.900)
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- PV1 — Encounter
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO ig_field_anchors (segment, field_pos, fhir_resource, fhir_path, fhir_type, hl7_table, confidence) VALUES
    ('PV1','2',    'Encounter','class.code',            'code',        '0004', 1.000),
    ('PV1','3.1',  'Encounter','location[0].location.display','string',NULL,   0.900),
    ('PV1','4',    'Encounter','type[0].coding[0].code','code',        NULL,   0.850),
    ('PV1','7',    'Encounter','participant[0].individual.display','string',NULL,0.880),
    ('PV1','10',   'Encounter','serviceType.coding[0].code','code',    NULL,   0.840),
    ('PV1','17',   'Encounter','participant[1].individual.display','string',NULL,0.820),
    ('PV1','19',   'Encounter','identifier[0].value',   'string',      NULL,   0.960),
    ('PV1','44',   'Encounter','period.start',          'dateTime',    NULL,   1.000),
    ('PV1','45',   'Encounter','period.end',            'dateTime',    NULL,   1.000),
    ('PV1','50',   'Encounter','identifier[1].value',   'string',      NULL,   0.870),
    ('PV1','51',   'Encounter','identifier[2].value',   'string',      NULL,   0.860)
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- OBX — Observation
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO ig_field_anchors (segment, field_pos, fhir_resource, fhir_path, fhir_type, hl7_table, confidence) VALUES
    ('OBX','3',    'Observation','code',                'CodeableConcept',NULL,1.000),
    ('OBX','3.1',  'Observation','code.coding[0].code', 'code',        NULL,   1.000),
    ('OBX','3.2',  'Observation','code.coding[0].display','string',    NULL,   0.980),
    ('OBX','3.3',  'Observation','code.coding[0].system','uri',        NULL,   0.970),
    ('OBX','5',    'Observation','value[x]',            'varies',      NULL,   1.000),
    ('OBX','6',    'Observation','valueQuantity.unit',  'string',      NULL,   0.950),
    ('OBX','6.1',  'Observation','valueQuantity.value', 'decimal',     NULL,   0.940),
    ('OBX','7',    'Observation','referenceRange[0].text','string',    NULL,   0.900),
    ('OBX','8',    'Observation','interpretation[0].coding[0].code','code',NULL,0.920),
    ('OBX','11',   'Observation','status',              'code',        '0085', 1.000),
    ('OBX','14',   'Observation','effectiveDateTime',   'dateTime',    NULL,   1.000),
    ('OBX','15',   'Observation','performer[0].display','string',      NULL,   0.860),
    ('OBX','16',   'Observation','performer[1].display','string',      NULL,   0.840),
    ('OBX','19',   'Observation','issued',              'instant',     NULL,   0.950),
    ('OBX','23',   'Observation','performer[2].display','string',      NULL,   0.820)
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- STF — Practitioner
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO ig_field_anchors (segment, field_pos, fhir_resource, fhir_path, fhir_type, hl7_table, confidence) VALUES
    ('STF','1',    'Practitioner','identifier[0].value', 'string',     NULL,   0.950),
    ('STF','2.1',  'Practitioner','identifier[1].value', 'string',     NULL,   0.950),
    ('STF','2.4',  'Practitioner','identifier[1].system','uri',        NULL,   0.900),
    ('STF','3',    'Practitioner','name',                'HumanName',  NULL,   1.000),
    ('STF','3.1',  'Practitioner','name[0].family',      'string',     NULL,   1.000),
    ('STF','3.2',  'Practitioner','name[0].given[0]',    'string',     NULL,   1.000),
    ('STF','3.3',  'Practitioner','name[0].given[1]',    'string',     NULL,   0.970),
    ('STF','3.5',  'Practitioner','name[0].prefix[0]',   'string',     NULL,   0.950),
    ('STF','4',    'Practitioner','qualification[0].code.text','string',NULL,  0.850),
    ('STF','5',    'Practitioner','gender',              'code',        '0001', 1.000),
    ('STF','6',    'Practitioner','birthDate',           'date',        NULL,   1.000),
    ('STF','7',    'Practitioner','active',              'boolean',     '0183', 1.000),
    ('STF','10.4', 'Practitioner','telecom[0].value',    'string',     NULL,   0.920),
    ('STF','10.3', 'Practitioner','telecom[0].system',   'code',       '0202', 0.900),
    ('STF','11.1', 'Practitioner','address[0].line[0]',  'string',     NULL,   0.900),
    ('STF','11.3', 'Practitioner','address[0].city',     'string',     NULL,   0.900),
    ('STF','11.4', 'Practitioner','address[0].state',    'string',     NULL,   0.900),
    ('STF','11.5', 'Practitioner','address[0].postalCode','string',    NULL,   0.900)
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- PRA — PractitionerRole
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO ig_field_anchors (segment, field_pos, fhir_resource, fhir_path, fhir_type, hl7_table, confidence) VALUES
    ('PRA','1',    'PractitionerRole','identifier[0].value',          'string', NULL,   0.900),
    ('PRA','3',    'PractitionerRole','specialty[0].coding[0].code',  'code',   NULL,   0.950),
    ('PRA','3',    'PractitionerRole','specialty[0].coding[0].system','uri',    NULL,   0.750),
    ('PRA','5',    'PractitionerRole','code[0].coding[0].code',       'code',   NULL,   0.860),
    ('PRA','6.1',  'PractitionerRole','identifier[0].value',          'string', NULL,   0.900),
    ('PRA','6.3',  'PractitionerRole','identifier[0].system',         'uri',    NULL,   0.850)
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- ORC — ServiceRequest / MedicationRequest (shared order control)
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO ig_field_anchors (segment, field_pos, fhir_resource, fhir_path, fhir_type, hl7_table, confidence) VALUES
    ('ORC','1',    'ServiceRequest','status',            'code',        NULL,   0.920),
    ('ORC','2',    'ServiceRequest','identifier[0].value','string',    NULL,   0.960),
    ('ORC','3',    'ServiceRequest','identifier[1].value','string',    NULL,   0.950),
    ('ORC','5',    'ServiceRequest','status',            'code',        NULL,   0.940),
    ('ORC','9',    'ServiceRequest','authoredOn',        'dateTime',    NULL,   0.950),
    ('ORC','12',   'ServiceRequest','requester.display', 'string',      NULL,   0.880),
    ('ORC','14',   'ServiceRequest','note[0].text',      'string',      NULL,   0.820),
    ('ORC','15',   'ServiceRequest','occurrenceDateTime','dateTime',    NULL,   0.870)
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- DG1 — Condition
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO ig_field_anchors (segment, field_pos, fhir_resource, fhir_path, fhir_type, hl7_table, confidence) VALUES
    ('DG1','3.1',  'Condition','code.coding[0].code',    'code',       NULL,   0.970),
    ('DG1','3.2',  'Condition','code.coding[0].display', 'string',     NULL,   0.960),
    ('DG1','3.3',  'Condition','code.coding[0].system',  'uri',        NULL,   0.950),
    ('DG1','4',    'Condition','code.text',              'string',      NULL,   0.900),
    ('DG1','5',    'Condition','onsetDateTime',          'dateTime',    NULL,   0.940),
    ('DG1','6',    'Condition','category[0].coding[0].code','code',    NULL,   0.870)
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- AL1 — AllergyIntolerance
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO ig_field_anchors (segment, field_pos, fhir_resource, fhir_path, fhir_type, hl7_table, confidence) VALUES
    ('AL1','2',    'AllergyIntolerance','category[0]',                'code',  NULL,   0.940),
    ('AL1','3.1',  'AllergyIntolerance','code.coding[0].code',        'code',  NULL,   0.960),
    ('AL1','3.2',  'AllergyIntolerance','code.coding[0].display',     'string',NULL,   0.940),
    ('AL1','4',    'AllergyIntolerance','criticality',                'code',  NULL,   0.880),
    ('AL1','5.1',  'AllergyIntolerance','reaction[0].manifestation[0].coding[0].display','string',NULL,0.860)
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- Verify
-- ─────────────────────────────────────────────────────────────────────────────
DO $$
DECLARE
    v_valueset_count INTEGER;
    v_anchor_count   INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_valueset_count FROM hl7_fhir_value_mappings WHERE is_active = true;
    SELECT COUNT(*) INTO v_anchor_count   FROM ig_field_anchors          WHERE is_active = true;
    RAISE NOTICE 'V115 complete: % valueset entries, % IG field anchors', v_valueset_count, v_anchor_count;
END $$;

COMMIT;
