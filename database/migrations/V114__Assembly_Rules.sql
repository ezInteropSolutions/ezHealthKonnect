-- V114: Assembly Rules
--
-- Cross-resource wiring that was previously hardcoded in Go assembly files
-- (mfn_assembly.go, assembly.go, post-normalizer switch/case) is now stored
-- here as data.  The Go engine reads these rows and applies them generically
-- after field-mapping completes — no Go changes required for new message types.
--
-- rule_type values:
--   reference  — source.referencePath = { reference: "TargetType/id" }
--   focus      — MessageHeader.focus[]  += { reference: "TargetType/id" }
--   result     — source.referencePath[] += all TargetType refs (DR→Observation[])
--   subject    — source.referencePath   = { reference: "Patient/id" }
--   performer  — source.referencePath[] = [{ reference: "TargetType/id" }]
--   encounter  — source.referencePath   = { reference: "Encounter/id" }
--
-- Rules execute in ascending sequence order.
-- condition_expr (future): FHIRPath expression; NULL = always apply.

BEGIN;

CREATE TABLE IF NOT EXISTS assembly_rules (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    message_type    VARCHAR(20) NOT NULL,
    rule_type       VARCHAR(30) NOT NULL
                    CHECK (rule_type IN ('reference','focus','result','subject','performer','encounter','author')),
    source_resource VARCHAR(60) NOT NULL,
    target_resource VARCHAR(60) NOT NULL,
    reference_path  VARCHAR(120) NOT NULL,
    condition_expr  TEXT,
    sequence        INT         NOT NULL DEFAULT 100,
    is_active       BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_assembly_rules_lookup
    ON assembly_rules (message_type, is_active, sequence);

-- ─────────────────────────────────────────────────────────────────────────────
-- MFN^M02  Staff / Practitioner master file
-- Replaces the hardcoded block added in V113 fix + mfn_assembly.go wiring
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO assembly_rules (message_type, rule_type, source_resource, target_resource, reference_path, sequence) VALUES
  ('MFN^M02', 'reference', 'PractitionerRole', 'Practitioner',     'practitioner', 100),
  ('MFN^M02', 'focus',     'MessageHeader',    'Practitioner',     'focus',        200),
  ('MFN^M02', 'focus',     'MessageHeader',    'PractitionerRole', 'focus',        210);

-- MFN^M05  Location master file
INSERT INTO assembly_rules (message_type, rule_type, source_resource, target_resource, reference_path, sequence) VALUES
  ('MFN^M05', 'focus', 'MessageHeader', 'Location', 'focus', 200);

-- MFN^M04  Charge description master
INSERT INTO assembly_rules (message_type, rule_type, source_resource, target_resource, reference_path, sequence) VALUES
  ('MFN^M04', 'focus', 'MessageHeader', 'ChargeItemDefinition', 'focus', 200);

-- MFN^M06 / MFN^M12  Observation definition master
INSERT INTO assembly_rules (message_type, rule_type, source_resource, target_resource, reference_path, sequence) VALUES
  ('MFN^M06',  'focus', 'MessageHeader', 'ObservationDefinition', 'focus', 200),
  ('MFN^M12',  'focus', 'MessageHeader', 'ObservationDefinition', 'focus', 200);

-- ─────────────────────────────────────────────────────────────────────────────
-- ORU^R01  Observation result
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO assembly_rules (message_type, rule_type, source_resource, target_resource, reference_path, sequence) VALUES
  ('ORU^R01', 'result',    'DiagnosticReport', 'Observation',      'result',     100),
  ('ORU^R01', 'subject',   'DiagnosticReport', 'Patient',          'subject',    110),
  ('ORU^R01', 'subject',   'Observation',      'Patient',          'subject',    120),
  ('ORU^R01', 'performer', 'DiagnosticReport', 'Practitioner',     'performer',  130),
  ('ORU^R01', 'focus',     'MessageHeader',    'DiagnosticReport', 'focus',      200);

-- ─────────────────────────────────────────────────────────────────────────────
-- ADT  Patient administration (A01–A13, A28, A31, A40, A47)
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO assembly_rules (message_type, rule_type, source_resource, target_resource, reference_path, sequence)
SELECT msg, rule_type, source_resource, target_resource, reference_path, sequence
FROM (VALUES
  ('subject',   'Encounter',     'Patient',   'subject',   100),
  ('reference', 'Encounter',     'Location',  'location',  110),
  ('performer', 'Encounter',     'Practitioner','participant',120),
  ('focus',     'MessageHeader', 'Patient',   'focus',     200)
) AS t(rule_type, source_resource, target_resource, reference_path, sequence)
CROSS JOIN (VALUES
  ('ADT^A01'),('ADT^A02'),('ADT^A03'),('ADT^A04'),('ADT^A05'),
  ('ADT^A06'),('ADT^A07'),('ADT^A08'),('ADT^A09'),('ADT^A10'),
  ('ADT^A11'),('ADT^A12'),('ADT^A13'),('ADT^A28'),('ADT^A31'),
  ('ADT^A40'),('ADT^A47')
) AS m(msg);

-- ─────────────────────────────────────────────────────────────────────────────
-- ORM^O01 / OML^O21 / OMG^O19  Orders
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO assembly_rules (message_type, rule_type, source_resource, target_resource, reference_path, sequence)
SELECT msg, rule_type, source_resource, target_resource, reference_path, sequence
FROM (VALUES
  ('subject',   'ServiceRequest', 'Patient',      'subject',   100),
  ('performer', 'ServiceRequest', 'Practitioner', 'performer', 110),
  ('focus',     'MessageHeader',  'ServiceRequest','focus',    200),
  ('focus',     'MessageHeader',  'Patient',      'focus',     210)
) AS t(rule_type, source_resource, target_resource, reference_path, sequence)
CROSS JOIN (VALUES ('ORM^O01'),('OML^O21'),('OMG^O19')) AS m(msg);

-- ─────────────────────────────────────────────────────────────────────────────
-- MDM^T01–T11  Medical document management
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO assembly_rules (message_type, rule_type, source_resource, target_resource, reference_path, sequence)
SELECT msg, rule_type, source_resource, target_resource, reference_path, sequence
FROM (VALUES
  ('subject', 'DocumentReference', 'Patient',      'subject', 100),
  ('author',  'DocumentReference', 'Practitioner', 'author',  110),
  ('focus',   'MessageHeader',     'DocumentReference','focus',200)
) AS t(rule_type, source_resource, target_resource, reference_path, sequence)
CROSS JOIN (VALUES
  ('MDM^T01'),('MDM^T02'),('MDM^T03'),('MDM^T04'),('MDM^T05'),
  ('MDM^T06'),('MDM^T07'),('MDM^T08'),('MDM^T09'),('MDM^T10'),('MDM^T11')
) AS m(msg);

-- ─────────────────────────────────────────────────────────────────────────────
-- VXU^V04  Immunization
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO assembly_rules (message_type, rule_type, source_resource, target_resource, reference_path, sequence) VALUES
  ('VXU^V04', 'subject',   'Immunization',  'Patient',      'patient',    100),
  ('VXU^V04', 'performer', 'Immunization',  'Practitioner', 'performer',  110),
  ('VXU^V04', 'focus',     'MessageHeader', 'Immunization', 'focus',      200),
  ('VXU^V04', 'focus',     'MessageHeader', 'Patient',      'focus',      210);

-- ─────────────────────────────────────────────────────────────────────────────
-- DFT^P03  Detailed financial transaction
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO assembly_rules (message_type, rule_type, source_resource, target_resource, reference_path, sequence) VALUES
  ('DFT^P03', 'subject',   'ChargeItem',    'Patient',   'subject',   100),
  ('DFT^P03', 'encounter', 'ChargeItem',    'Encounter', 'context',   110),
  ('DFT^P03', 'focus',     'MessageHeader', 'Patient',   'focus',     200);

-- ─────────────────────────────────────────────────────────────────────────────
-- SIU^S12–S26  Scheduling
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO assembly_rules (message_type, rule_type, source_resource, target_resource, reference_path, sequence)
SELECT msg, rule_type, source_resource, target_resource, reference_path, sequence
FROM (VALUES
  ('subject', 'Appointment',  'Patient',      'participant', 100),
  ('focus',   'MessageHeader','Appointment',  'focus',       200)
) AS t(rule_type, source_resource, target_resource, reference_path, sequence)
CROSS JOIN (VALUES
  ('SIU^S12'),('SIU^S13'),('SIU^S14'),('SIU^S15'),('SIU^S16'),
  ('SIU^S17'),('SIU^S26')
) AS m(msg);

-- ─────────────────────────────────────────────────────────────────────────────
-- BAR^P01 / BAR^P12  Billing account
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO assembly_rules (message_type, rule_type, source_resource, target_resource, reference_path, sequence) VALUES
  ('BAR^P01', 'subject',   'Account',       'Patient',   'subject', 100),
  ('BAR^P01', 'focus',     'MessageHeader', 'Patient',   'focus',   200),
  ('BAR^P12', 'subject',   'Account',       'Patient',   'subject', 100),
  ('BAR^P12', 'focus',     'MessageHeader', 'Patient',   'focus',   200);

-- ─────────────────────────────────────────────────────────────────────────────
-- Verify
-- ─────────────────────────────────────────────────────────────────────────────
DO $$
DECLARE v_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_count FROM assembly_rules WHERE is_active = true;
    RAISE NOTICE 'V114 complete: % assembly rules seeded', v_count;
END $$;

COMMIT;
