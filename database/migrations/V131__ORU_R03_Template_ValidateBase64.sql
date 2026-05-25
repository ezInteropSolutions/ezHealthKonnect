-- V131: Add validateBase64:true to ORU^R03 OOB template composites params
--
-- The FHIR R4 spec requires Attachment.data to be valid base64 (att-1).
-- Without this flag the assembly pass copies ED data verbatim; any source
-- system that sends malformed base64 (truncated, non-base64 characters, etc.)
-- produces a DocumentReference that fails FHIR validation.
--
-- Setting validateBase64:true in the OOB template causes BuildAttachmentFromED
-- to check decodability before writing the data field.  On failure it omits
-- data+contentType and sets a descriptive title so att-1 is never violated.
--
-- Interfaces that receive data from a trusted source known to send valid
-- base64 can override this to false in their custom composite config to
-- avoid the decode overhead and preserve the exact bytes.

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{mappings,DiagnosticReport,composites,0,params,validateBase64}',
    'true'::jsonb
)
WHERE message_type = 'ORU^R03'
  AND is_default   = true;
