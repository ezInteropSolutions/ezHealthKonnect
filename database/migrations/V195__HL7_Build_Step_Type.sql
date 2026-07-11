-- V195: Register the hl7.build pipeline step type in transformation_templates.
-- The no-code, format-agnostic on-ramp for a complete HL7 v2 message: maps
-- CSV columns, DB query columns, or arbitrary JSON fields directly onto HL7
-- segment/field/component paths -- the HL7-side mirror of V194's fhir.build.
-- MSH is always auto-populated (encoding characters, timestamp, control ID,
-- message type); every other segment is configured explicitly with "single"
-- or "repeating" cardinality.

INSERT INTO transformation_templates
    (template_name, template_type, description, default_config, is_system, is_public)
SELECT
    'HL7 v2 Message Builder',
    'hl7.build',
    'Maps CSV columns, DB query columns, or arbitrary JSON fields directly '
    'onto HL7 segment/field/component paths (e.g. "PID.3", "PID.5.1"), using '
    'no-code field mappings instead of a script. MSH is auto-populated '
    '(encoding characters, timestamp, control ID, message type); add one '
    'segment entry per HL7 segment needed, in message order, with '
    '"single" or "repeating" cardinality for repeated segments like OBX.',
    '{"messageType":"ADT","triggerEvent":"A01","version":"2.5.1","outputField":"hl7Message","segments":[]}'::jsonb,
    true,
    true
WHERE NOT EXISTS (
    SELECT 1 FROM transformation_templates WHERE template_type = 'hl7.build'
);
