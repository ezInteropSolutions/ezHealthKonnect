-- V276: Add connectivity_types rows for astm_tcp_inbound/astm_tcp_outbound,
-- and a new "ASTM over TCP/IP" interface_templates row under the `device`
-- category — Phase B of the ASTM E1394-97 feature (V275 shipped Phase A:
-- the engine + RS232 serial connector). Built only now that Phase A's own
-- astm_framing.go handshake already proved itself over a real transport,
-- reusing it completely unchanged over a real net.Conn instead of a serial
-- port's io.ReadWriteCloser.

-- ============================================================
-- BACKGROUND
-- ============================================================
--   astm_tcp_inbound  -- a real TCP listener (net.Listen + accept loop),
--                        the same LISTEN pattern tcp_mllp_inbound.go uses,
--                        but a genuinely NEW connector type rather than a
--                        protocol-mode switch on it -- ASTM's own ENQ/ACK
--                        handshake has no MLLP equivalent, which would force
--                        two incompatible state machines behind one config
--                        flag (the same reasoning AS2 Phase 4 already
--                        established for this codebase). Only the common
--                        real-world role is implemented this phase -- the
--                        instrument connects IN and sends ENQ first
--                        (initiate_role="receiver", the only mode Validate()
--                        currently accepts); the rarer "host pulls from a
--                        listening instrument" mode is a named, deliberately
--                        NOT-yet-implemented item (see astm_tcp_inbound.go's
--                        own file header).
--   astm_tcp_outbound -- dials OUT and plays ASTM's initiator role -- no
--                        role ambiguity here, since pushing data to a
--                        remote receiver unambiguously means sending ENQ
--                        first.
--
-- Config field names match services/connectors/astm_tcp_inbound.go and
-- astm_tcp_outbound.go exactly -- confirmed by reading those files directly.
--
-- Port 6614 is deliberately inside the real, Docker-mapped TCP/MLLP range
-- (docker-compose.yml: "6610-6670:6610-6670"), and distinct from the
-- HL7-over-TCP/IP template's own default (6612, V274).

INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, priority, version,
    ui_category, ui_sort_order
) VALUES (
    'astm_tcp_inbound',
    'inbound',
    'ASTM over TCP/IP',
    'Listen for ASTM E1394-97 lab instrument messages over a network (TCP/IP) connection. The instrument connects to this server and sends its own ENQ to begin each transmission.',
    '🔌',
    'push',
    false,
    false,
    false,
    'ASTMTCPInboundConnector',
    '{
        "type": "object",
        "required": ["port"],
        "properties": {
            "port": {"type": "integer", "title": "Listener Port"},
            "max_connections": {"type": "integer", "title": "Max Concurrent Connections", "default": 10},
            "read_timeout_sec": {"type": "integer", "title": "Idle Read Timeout (seconds)", "default": 300},
            "checksum_severity": {"type": "string", "title": "Checksum Mismatch Severity", "enum": ["error", "warning"], "default": "error", "description": "\"warning\" accepts a frame with a bad checksum anyway instead of asking the instrument to retransmit -- for sites on genuinely noisy network links"},
            "initiate_role": {"type": "string", "title": "Who Sends ENQ First", "enum": ["receiver"], "default": "receiver", "description": "Only \"receiver\" (the instrument connects in and sends ENQ first) is implemented today"}
        }
    }'::jsonb,
    '{
        "basic": ["port", "max_connections"],
        "advanced": ["read_timeout_sec", "checksum_severity", "initiate_role"]
    }'::jsonb,
    true,
    100,
    '1.0',
    'Healthcare Network',
    19
)
ON CONFLICT (type_name) DO UPDATE SET
    config_schema = EXCLUDED.config_schema,
    parameter_groups = EXCLUDED.parameter_groups,
    is_active = EXCLUDED.is_active,
    updated_at = NOW();

INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, priority, version,
    ui_category, ui_sort_order
) VALUES (
    'astm_tcp_outbound',
    'outbound',
    'ASTM over TCP/IP Outbound',
    'Send an ASTM E1394-97 message (e.g. an order or host-query reply) to an instrument over a network (TCP/IP) connection.',
    '🔌',
    'push',
    false,
    false,
    false,
    'ASTMTCPOutboundConnector',
    '{
        "type": "object",
        "required": ["host", "port"],
        "properties": {
            "host": {"type": "string", "title": "Host / IP Address"},
            "port": {"type": "integer", "title": "Port"},
            "checksum_severity": {"type": "string", "title": "Checksum Mismatch Severity", "enum": ["error", "warning"], "default": "error"},
            "connect_timeout_sec": {"type": "integer", "title": "Connect Timeout (seconds)", "default": 10},
            "write_timeout_sec": {"type": "integer", "title": "Send Timeout (seconds)", "default": 30}
        }
    }'::jsonb,
    '{
        "basic": ["host", "port"],
        "advanced": ["checksum_severity", "connect_timeout_sec", "write_timeout_sec"]
    }'::jsonb,
    true,
    100,
    '1.0',
    'Healthcare Network',
    20
)
ON CONFLICT (type_name) DO UPDATE SET
    config_schema = EXCLUDED.config_schema,
    parameter_groups = EXCLUDED.parameter_groups,
    is_active = EXCLUDED.is_active,
    updated_at = NOW();

-- ─────────────────────────────────────────────────────────────────────────────
-- "ASTM over TCP/IP" interface template
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO interface_templates
    (id, name, slug, description, category, subcategory, tags, icon, difficulty,
     source_connector_type, source_config_template,
     target_connector_type, target_config_template,
     pipeline_config, message_type,
     required_connection_fields, sanitized_fields, preview_steps,
     estimated_setup_minutes, is_system, is_public, author, usage_count)
VALUES
(
    gen_random_uuid(),
    'ASTM over TCP/IP',
    'device-astm-tcp',
    'Connect a lab instrument that speaks ASTM E1394-97 over a network (TCP/IP) connection, rather than a physical serial cable. Pick this template if your instrument''s own setup screen offers an "ASTM" protocol choice over a "TCP/IP" / "Network" / "Ethernet" connection (not RS232/Serial, and not "HL7"). The instrument itself connects to this server and sends its own ENQ to begin each transmission -- the common real-world case.',
    'device',
    'network',
    ARRAY['astm','e1394-97','tcp-ip','network','lab-instrument'],
    '🔌',
    'intermediate',
    'astm_tcp_inbound',
    '{"port":6614,"max_connections":10,"checksum_severity":"error"}',
    'http_outbound',
    '{"method":"POST","content_type":"application/json","auth_type":"bearer","timeout_seconds":30}',
    '{"execution_groups":[{"sequence":5,"steps":[{"step_name":"ASTM/TCP Inbound","step_type":"connector.inbound","sequence":5,"config":{"connectorType":"astm_tcp_inbound","config":{"port":6614,"max_connections":10,"read_timeout_sec":300,"checksum_severity":"error"},"timeoutMs":30000},"enabled":true,"required":true}]},{"sequence":10,"steps":[{"step_name":"Parse ASTM Message","step_type":"astm.parse","sequence":10,"config":{"sourceField":"raw","outputField":"parsedASTM"},"enabled":true,"required":true}]},{"sequence":20,"steps":[{"step_name":"Validate ASTM Message","step_type":"astm.validate","sequence":20,"config":{"sourceField":"parsedASTM","outputField":"astmValidation"},"enabled":true,"required":false}]},{"sequence":30,"steps":[{"step_name":"Deliver Results to HIS","step_type":"connector.outbound","sequence":30,"config":{"connectorType":"http_outbound","config":{"method":"POST","content_type":"application/json","auth_type":"bearer","timeout_seconds":30},"contentField":"parsedASTM","contentType":"application/json"},"enabled":true,"required":true}]}]}',
    'ASTM',
    '[{"section":"source","field":"port","label":"Port Number","type":"number","hint":"The network port this server listens on. Enter this same number into the instrument''s own LIS/host communication settings.","required":true},{"section":"target","field":"endpoint","label":"Results Delivery API URL","type":"url","hint":"Where completed lab results are sent, e.g. https://your-his.example.org/api/results","required":true},{"section":"target","field":"bearer_token","label":"Bearer Token","type":"password","hint":"Authorization token for your HIS API, if required","required":false}]',
    ARRAY['source.port','target.endpoint','target.bearer_token'],
    '[{"name":"Receive over TCP/IP","icon":"🔌","step_type":"connector.inbound"},{"name":"Parse ASTM","icon":"🔄","step_type":"astm.parse"},{"name":"Validate","icon":"✅","step_type":"astm.validate"},{"name":"Deliver to HIS","icon":"📤","step_type":"connector.outbound"}]',
    10,
    true,
    true,
    'ezHealthKonnect',
    0
);

-- ─────────────────────────────────────────────────────────────────────────────
-- User guide
-- ─────────────────────────────────────────────────────────────────────────────
UPDATE interface_templates
SET user_guide = $GUIDE$
# ASTM over TCP/IP

## Who this is for

Any lab instrument whose own setup screen lets you choose **ASTM** as the
protocol and **TCP/IP / Network / Ethernet** as the connection type. If your
instrument instead says "HL7" and "TCP/IP", use the **HL7 over TCP/IP**
template. If it says "ASTM" and "Serial / RS232", use the **ASTM over
Serial Port** template instead.

## What you need before you start

- A network cable from the instrument to the same network this server is reachable on
- Access to the instrument's own LIS/host communication settings screen
- Your hospital's HIS results-delivery endpoint

## Setup steps

1. **Pick a port number.** The default above is already chosen to work with this deployment -- you can keep it unless you have a reason to change it.
2. **On the instrument itself**, open its LIS/host communication setup screen, choose the **ASTM** protocol over **TCP/IP**, and enter this server's network address plus the **same port number** you entered above.
3. **Enter your Results Delivery API URL.**

## A note on what gets delivered

This template delivers the instrument's own structured message (patient, order, and result data) to your HIS endpoint as JSON -- it does not yet convert ASTM messages into FHIR format. If your HIS needs FHIR specifically, ask your integrator about adding a transformation step in Pipeline Builder.

## A note on connection direction

This template assumes the instrument itself connects IN to this server and starts each transmission (the common real-world case for ASTM-over-TCP/IP instruments). The reverse arrangement (this server connecting out to a listening instrument) is not yet supported.
$GUIDE$
WHERE slug = 'device-astm-tcp';

-- ─────────────────────────────────────────────────────────────────────────────
-- Extend the HL7 template's own guide text (already updated once by V275 to
-- mention the Serial template) to also mention this new TCP/IP template.
-- ─────────────────────────────────────────────────────────────────────────────
UPDATE interface_templates
SET user_guide = regexp_replace(
    user_guide,
    'Use the \*\*ASTM over Serial Port\*\* template instead \(device category\) -- it is now fully supported\.',
    'Use the **ASTM over Serial Port** template (for a physical RS232/serial connection) or the **ASTM over TCP/IP** template (for a network connection) instead -- both are now fully supported.',
    's'
)
WHERE slug = 'device-hl7-tcp'
  AND user_guide LIKE '%ASTM over Serial Port%template instead (device category)%';
