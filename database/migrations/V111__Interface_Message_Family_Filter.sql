-- V111: Interface message-family accept list
-- Adds accepted_message_families JSONB column to interfaces.
-- NULL (default) = accept all message types (fully backward compatible).
-- Non-null = only process messages whose MSH-9 family is in the list.
--
-- Values are family names (no ^ = all events) or full event codes for
-- structurally heterogeneous families (MFN):
--   ["ADT", "ORU", "SIU"]          -- accept all ADT/ORU/SIU events
--   ["ADT", "MFN^M02", "MFN^M05"] -- all ADT + specific MFN events only
--
-- Messages not in the accept list receive an HL7 NACK (AR) before being stored.

ALTER TABLE interfaces
ADD COLUMN IF NOT EXISTS accepted_message_families JSONB DEFAULT NULL;

COMMENT ON COLUMN interfaces.accepted_message_families IS
  'User-defined HL7 message family accept list. NULL = accept all. '
  'Array of family codes (e.g. ["ADT","ORU"]) or full event codes for '
  'heterogeneous families (e.g. "MFN^M02"). Non-matching messages receive NACK.';

CREATE INDEX IF NOT EXISTS idx_interfaces_msg_families
ON interfaces USING gin(accepted_message_families)
WHERE accepted_message_families IS NOT NULL;
