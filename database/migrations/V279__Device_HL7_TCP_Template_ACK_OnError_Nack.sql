-- V279: Device "HL7 over TCP/IP" template — ACK on_error "suppress" -> "nack"
-- Applied: 2026-10-19

-- ============================================================
-- WHY
-- ============================================================
-- V274 seeded the device-hl7-tcp template with ack.on_error = "suppress", which
-- replies AA when a well-formed message could NOT be queued (queue full) — the
-- message is dropped and the device, told it was accepted, never resends. For a
-- clinical device feed (ventilators, monitors, analyzers) that is silent data
-- loss. tcp_mllp_inbound.go's own default changed to "nack" (AE, sender retries)
-- in the same release; this aligns the template so newly created device
-- interfaces get the safe behavior. "suppress" remains available as an explicit,
-- clearly-labelled opt-in in the connector's Acknowledgment tab.
--
-- Scope: the TEMPLATE only. Interfaces already created from it keep their stored
-- configuration (an engineer can switch them in the Acknowledgment tab) — this
-- migration never rewrites a customer's live interface config.
--
-- Data-only and idempotent: re-running finds no "suppress" left to replace.

-- ============================================================
-- TEMPLATE UPDATE
-- ============================================================
UPDATE interface_templates
SET
    source_config_template = regexp_replace(
        source_config_template::text,
        '"on_error"\s*:\s*"suppress"',
        '"on_error": "nack"',
        'g'
    )::jsonb,
    pipeline_config = regexp_replace(
        pipeline_config::text,
        '"on_error"\s*:\s*"suppress"',
        '"on_error": "nack"',
        'g'
    )::jsonb,
    updated_at = NOW()
WHERE slug = 'device-hl7-tcp'
  AND (source_config_template::text ~ '"on_error"\s*:\s*"suppress"'
       OR pipeline_config::text ~ '"on_error"\s*:\s*"suppress"');
