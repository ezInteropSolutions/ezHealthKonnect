-- V116: SIU Encounter Assembly Rules
--
-- Extends the SIU assembly rules from V114 to wire the Encounter resource
-- that PV1 now produces for SIU messages.
--
-- Clinical rationale
-- ──────────────────
-- An HL7 SIU message carries PV1 alongside SCH.  In FHIR R4 the correct
-- workflow is:
--
--   SCH.25 = PENDING/BOOKED  → Appointment.status = "booked"
--                               Encounter.status   = "planned"  (pre-admit stub)
--   SCH.25 = ARRIVED         → Appointment.status = "arrived"
--                               Encounter.status   = "in-progress"
--   SCH.25 = COMPLETE/FULFILLED → Appointment.status = "fulfilled"
--                               Encounter.status   = "finished"  (actual visit)
--
-- The OOB template builder (V116 Go changes) now includes PV1 in the SIU
-- field-mapping pass, producing an Encounter resource block in the template.
-- These assembly rules wire that Encounter to the Appointment and Patient.
--
-- condition_expr column
-- ─────────────────────
-- The engine currently treats condition_expr = NULL as "always apply".
-- Full FHIRPath condition evaluation is deferred (tracked as a future
-- enhancement).  Until then, the Encounter → Appointment reference rule fires
-- whenever an Encounter resource is present in the bundle, which is the
-- correct behaviour for the "always wire if both exist" pattern.
--
-- Known gaps (to address when condition_expr evaluation is implemented)
-- ──────────────────────────────────────────────────────────────────────
--  1. OBX.2 type dispatch   — OBX.5 value[x] should use valueQuantity /
--     valueString / valueCodeableConcept based on OBX.2.  Currently the
--     engine lands OBX.5 on value[x] as a raw string.  Requires runtime
--     polymorphic dispatch keyed on OBX.2.
--  2. ORC.1 state machine   — ORC.1 codes NW/CA/DC/HD map to ServiceRequest
--     status but also imply create vs update operations. Engine currently
--     always creates new resources; an upsert mode is needed for CA/DC.
--  3. DG1 dual resource     — each DG1 creates a Condition AND an
--     Encounter.diagnosis entry (with rank from DG1.15). Assembly rule
--     "reference Condition from Encounter.diagnosis" not yet present.
--  4. IN1.4 implicit Org    — Coverage.payor must be an Organization
--     reference.  IN1.4 (insurance company name) should create a stub
--     Organization and set Coverage.payor[0] as a reference to it.
--  5. NK1.3 relationship VS — NK1.3 uses HL7 v2 table 0063 (MTH, FTH …).
--     A valueset lookup transform to HL7 v3 / SNOMED role codes is needed
--     for RelatedPerson.relationship to be interoperable.

BEGIN;

-- ── Encounter wiring for all SIU scheduling variants ─────────────────────
--
-- Rules added per event (S12–S17, S26):
--   seq 105  subject   Encounter → Patient  (Encounter.subject = Patient/id)
--   seq 110  reference Encounter → Appointment  (Encounter.appointment[0])
--   seq 205  focus     MessageHeader → Encounter  (MessageHeader.focus[])
--
-- Rationale for seq gaps:
--   V114 already inserts seq 100 (Appointment→Patient participant) and
--   seq 200 (MessageHeader focus→Appointment).  The new rules interleave
--   cleanly without renumbering.

INSERT INTO assembly_rules
    (message_type, rule_type, source_resource, target_resource, reference_path, condition_expr, sequence)
SELECT
    msg,
    rule_type,
    source_resource,
    target_resource,
    reference_path,
    NULL,   -- condition_expr: fire whenever both resources are present in bundle
    sequence
FROM (VALUES
    ('subject',   'Encounter',    'Patient',     'subject',     105),
    ('reference', 'Encounter',    'Appointment', 'appointment', 110),
    ('focus',     'MessageHeader','Encounter',   'focus',       205)
) AS t(rule_type, source_resource, target_resource, reference_path, sequence)
CROSS JOIN (VALUES
    ('SIU^S12'),('SIU^S13'),('SIU^S14'),('SIU^S15'),
    ('SIU^S16'),('SIU^S17'),('SIU^S26')
) AS m(msg)
ON CONFLICT DO NOTHING;

-- ── Verify ────────────────────────────────────────────────────────────────
DO $$
DECLARE
    v_siu_count  INTEGER;
    v_total      INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_siu_count
    FROM   assembly_rules
    WHERE  message_type LIKE 'SIU%' AND is_active = true;

    SELECT COUNT(*) INTO v_total
    FROM   assembly_rules
    WHERE  is_active = true;

    RAISE NOTICE 'V116 complete: % SIU assembly rules active (% total)', v_siu_count, v_total;
END $$;

COMMIT;
