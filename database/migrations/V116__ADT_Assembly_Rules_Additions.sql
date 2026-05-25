-- V116: Additional ADT assembly rules
--
-- Wire Condition.subject, RelatedPerson.patient, and Coverage.beneficiary to the
-- Patient resource for all ADT message variants. These cross-resource references
-- are required by FHIR R4 (cardinality 1..1) but were absent from V114 seed data.

BEGIN;

INSERT INTO assembly_rules (message_type, rule_type, source_resource, target_resource, reference_path, sequence)
SELECT msg, rule_type, source_resource, target_resource, reference_path, sequence
FROM (VALUES
  ('subject',   'Condition',     'Patient', 'subject',     150),
  ('subject',   'RelatedPerson', 'Patient', 'patient',     160),
  ('subject',   'Coverage',      'Patient', 'beneficiary', 170)
) AS t(rule_type, source_resource, target_resource, reference_path, sequence)
CROSS JOIN (VALUES
  ('ADT^A01'),('ADT^A02'),('ADT^A03'),('ADT^A04'),('ADT^A05'),
  ('ADT^A06'),('ADT^A07'),('ADT^A08'),('ADT^A09'),('ADT^A10'),
  ('ADT^A11'),('ADT^A12'),('ADT^A13'),('ADT^A28'),('ADT^A31'),
  ('ADT^A40'),('ADT^A47')
) AS m(msg);

DO $$
DECLARE v_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_count FROM assembly_rules
    WHERE source_resource IN ('Condition','RelatedPerson','Coverage')
      AND reference_path IN ('subject','patient','beneficiary')
      AND is_active = true;
    RAISE NOTICE 'V116 complete: % ADT assembly rules added for Condition/RelatedPerson/Coverage', v_count;
END $$;

COMMIT;
