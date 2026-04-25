-- V109: Interface Mapping Delta Model
-- Adds mapping_overrides column to interface_message_mappings so that an interface
-- can store only the fields that differ from its OOB template (delta/override model).
-- Interfaces that use a standard template AND have no overrides remain unchanged.
-- Interfaces with overrides store a compact delta instead of a full mapping copy.

ALTER TABLE interface_message_mappings
ADD COLUMN IF NOT EXISTS mapping_overrides JSONB;

COMMENT ON COLUMN interface_message_mappings.mapping_overrides IS
  'Sparse delta over the standard OOB template: {version, base_template_id, overrides:[{action,hl7Path,fhirPath,transform,...}]}. NULL means use OOB as-is.';

-- Index for delta-aware lookup (only rows that actually have overrides)
CREATE INDEX IF NOT EXISTS idx_imm_has_overrides
ON interface_message_mappings(interface_id, message_type)
WHERE mapping_overrides IS NOT NULL;
