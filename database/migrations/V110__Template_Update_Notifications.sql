-- V110: Track OOB template update availability on custom interface mappings.
--
-- When an OOB template is rebuilt (profile_version increments), any
-- interface_message_mappings row that has custom overrides (mapping_overrides IS NOT NULL)
-- and whose stored based_on_version differs from the new template version gets flagged
-- with template_update_available = true.
--
-- The UI reads this flag to show an "update available" badge on the interface card.
-- Clearing the flag (after user applies or dismisses) is handled by the mapping delta API.

ALTER TABLE interface_message_mappings
    ADD COLUMN IF NOT EXISTS template_update_available  BOOLEAN      DEFAULT false,
    ADD COLUMN IF NOT EXISTS available_template_id      UUID         REFERENCES hl7_fhir_templates(id),
    ADD COLUMN IF NOT EXISTS available_template_version VARCHAR(20);

-- Index for fast "which interfaces have pending updates?" query (used by UI dashboard)
CREATE INDEX IF NOT EXISTS idx_imm_update_available
    ON interface_message_mappings(interface_id)
    WHERE template_update_available = true;

COMMENT ON COLUMN interface_message_mappings.template_update_available  IS 'true when a newer OOB template version exists and this mapping has custom overrides';
COMMENT ON COLUMN interface_message_mappings.available_template_id      IS 'ID of the newer OOB template version available for review';
COMMENT ON COLUMN interface_message_mappings.available_template_version IS 'profile_version of the available template (e.g. "2.1")';
