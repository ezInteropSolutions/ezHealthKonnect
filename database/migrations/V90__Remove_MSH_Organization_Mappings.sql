-- V90: Remove incorrect MSH→Organization mappings from all OOB HL7-FHIR templates.
--
-- MSH fields (Sending Application / Sending Facility) describe message routing,
-- not an Organisation entity.  They should map to MessageHeader (already present
-- in every template).  The "Organization" resource block was added in error by the
-- template generator and produced misleading noise in the Visual Mapping tab.

UPDATE hl7_fhir_templates
SET
    template_config = jsonb_set(
        template_config,
        '{resources}',
        (template_config -> 'resources') - 'Organization'
    ),
    updated_at = NOW()
WHERE template_config -> 'resources' ? 'Organization';
