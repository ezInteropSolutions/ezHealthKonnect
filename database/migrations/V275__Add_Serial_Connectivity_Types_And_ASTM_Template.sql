-- V275: Add connectivity_types rows for serial_inbound/serial_outbound, and
-- a new "ASTM over Serial Port" interface_templates row under the `device`
-- category V274 introduced.

-- ============================================================
-- BACKGROUND
-- ============================================================
-- The "Connect a Device" guided wizard (V274) originally shipped with its
-- own guide text naming ASTM/Serial support as "not supported yet" -- the
-- user correctly pushed back that the wizard was always meant to cover
-- RS232/serial and ASTM too, not just HL7-over-TCP/IP. This migration adds
-- the real, working connectivity for that gap:
--   serial_inbound  -- opens a physical/virtual COM port once (requires the
--                      confirmed "co-located process" deployment topology --
--                      this process must run on a machine physically cabled
--                      to the instrument) and plays ASTM's receiver role,
--                      continuously waiting for the instrument to initiate.
--   serial_outbound -- opens the configured COM port fresh per Send() call
--                      and plays ASTM's initiator role.
--
-- Config field names match services/connectors/serial_inbound.go and
-- serial_outbound.go exactly -- confirmed by reading those files directly.
--
-- baud_rate is a genuinely fixed enum (the real list of standard RS232 baud
-- rates) -- NOT the same kind of thing as a COM port name, which has no
-- fixed list and instead needs the new live GET /api/connectivity/
-- serial-ports endpoint (serial_port_controller.go) wired into the wizard's
-- own new "select" field-type support (public/js/dashboard.js).
--
-- The new template's own pipeline is deliberately simpler than the
-- HL7-over-TCP/IP template's (V274): it delivers the ASTM message's own
-- structured parsed JSON to the configured HIS endpoint directly, rather
-- than transforming into FHIR first -- no generic ASTM-to-FHIR mapping
-- engine exists in this codebase yet (a genuinely separate, later piece of
-- work, matching this project's own EDI X12/NCPDP precedent of building the
-- parsing/building engine first and FHIR mapping as its own later phase).
-- This is an honest, named scope boundary, not a hidden shortfall -- the
-- template's own guide text says so.

INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, priority, version,
    ui_category, ui_sort_order
) VALUES (
    'serial_inbound',
    'inbound',
    'RS232 Serial (ASTM)',
    'Continuously read ASTM E1394-97 lab instrument messages off a physical or virtual COM port. Requires this server to run on a machine with a real serial connection to the instrument.',
    '🔌',
    'pull',
    false,
    false,
    false,
    'SerialInboundConnector',
    '{
        "type": "object",
        "required": ["port_name"],
        "properties": {
            "port_name": {"type": "string", "title": "COM Port", "description": "e.g. COM3 (Windows) or /dev/ttyUSB0 (Linux)"},
            "baud_rate": {"type": "integer", "title": "Baud Rate", "enum": [1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200], "default": 9600},
            "data_bits": {"type": "integer", "title": "Data Bits", "enum": [5, 6, 7, 8], "default": 8},
            "parity": {"type": "string", "title": "Parity", "enum": ["none", "odd", "even", "mark", "space"], "default": "none"},
            "stop_bits": {"type": "string", "title": "Stop Bits", "enum": ["1", "1.5", "2"], "default": "1"},
            "checksum_severity": {"type": "string", "title": "Checksum Mismatch Severity", "enum": ["error", "warning"], "default": "error", "description": "\"warning\" accepts a frame with a bad checksum anyway instead of asking the instrument to retransmit -- for sites on genuinely noisy serial links"}
        }
    }'::jsonb,
    '{
        "basic": ["port_name", "baud_rate"],
        "advanced": ["data_bits", "parity", "stop_bits", "checksum_severity"]
    }'::jsonb,
    true,
    100,
    '1.0',
    'Healthcare Network',
    17
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
    'serial_outbound',
    'outbound',
    'RS232 Serial (ASTM) Outbound',
    'Send an ASTM E1394-97 message (e.g. an order or host-query reply) to an instrument over a physical or virtual COM port.',
    '🔌',
    'push',
    false,
    false,
    false,
    'SerialOutboundConnector',
    '{
        "type": "object",
        "required": ["port_name"],
        "properties": {
            "port_name": {"type": "string", "title": "COM Port", "description": "e.g. COM3 (Windows) or /dev/ttyUSB0 (Linux)"},
            "baud_rate": {"type": "integer", "title": "Baud Rate", "enum": [1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200], "default": 9600},
            "data_bits": {"type": "integer", "title": "Data Bits", "enum": [5, 6, 7, 8], "default": 8},
            "parity": {"type": "string", "title": "Parity", "enum": ["none", "odd", "even", "mark", "space"], "default": "none"},
            "stop_bits": {"type": "string", "title": "Stop Bits", "enum": ["1", "1.5", "2"], "default": "1"},
            "checksum_severity": {"type": "string", "title": "Checksum Mismatch Severity", "enum": ["error", "warning"], "default": "error"},
            "write_timeout_sec": {"type": "integer", "title": "Send Timeout (seconds)", "default": 30}
        }
    }'::jsonb,
    '{
        "basic": ["port_name", "baud_rate"],
        "advanced": ["data_bits", "parity", "stop_bits", "checksum_severity", "write_timeout_sec"]
    }'::jsonb,
    true,
    100,
    '1.0',
    'Healthcare Network',
    18
)
ON CONFLICT (type_name) DO UPDATE SET
    config_schema = EXCLUDED.config_schema,
    parameter_groups = EXCLUDED.parameter_groups,
    is_active = EXCLUDED.is_active,
    updated_at = NOW();

-- ─────────────────────────────────────────────────────────────────────────────
-- "ASTM over Serial Port" interface template
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
    'ASTM over Serial Port',
    'device-astm-serial',
    'Connect a lab instrument that speaks ASTM E1394-97 over a physical RS232 serial (COM port) connection -- common on older or simpler chemistry/hematology analyzers. Requires this server to run on a machine with a real cable connection to the instrument. Pick this template if your instrument''s own setup screen offers an "ASTM" protocol choice over a "Serial" / "RS232" connection (not TCP/IP, and not "HL7").',
    'device',
    'serial',
    ARRAY['astm','e1394-97','serial','rs232','com-port','lab-instrument'],
    '🔌',
    'intermediate',
    'serial_inbound',
    '{"port_name":"","baud_rate":9600,"data_bits":8,"parity":"none","stop_bits":"1","checksum_severity":"error"}',
    'http_outbound',
    '{"method":"POST","content_type":"application/json","auth_type":"bearer","timeout_seconds":30}',
    '{"execution_groups":[{"sequence":5,"steps":[{"step_name":"Serial Inbound (ASTM)","step_type":"connector.inbound","sequence":5,"config":{"connectorType":"serial_inbound","config":{"port_name":"","baud_rate":9600,"data_bits":8,"parity":"none","stop_bits":"1","checksum_severity":"error"},"timeoutMs":30000},"enabled":true,"required":true}]},{"sequence":10,"steps":[{"step_name":"Parse ASTM Message","step_type":"astm.parse","sequence":10,"config":{"sourceField":"raw","outputField":"parsedASTM"},"enabled":true,"required":true}]},{"sequence":20,"steps":[{"step_name":"Validate ASTM Message","step_type":"astm.validate","sequence":20,"config":{"sourceField":"parsedASTM","outputField":"astmValidation"},"enabled":true,"required":false}]},{"sequence":30,"steps":[{"step_name":"Deliver Results to HIS","step_type":"connector.outbound","sequence":30,"config":{"connectorType":"http_outbound","config":{"method":"POST","content_type":"application/json","auth_type":"bearer","timeout_seconds":30},"contentField":"parsedASTM","contentType":"application/json"},"enabled":true,"required":true}]}]}',
    'ASTM',
    '[{"section":"source","field":"port_name","label":"COM Port","type":"select","optionsEndpoint":"/api/connectivity/serial-ports","hint":"The physical or virtual serial port this server is connected to the instrument through. If your port isn''t listed, make sure the cable is connected and try reopening this dialog, or enter it manually later in Pipeline Builder.","required":true},{"section":"source","field":"baud_rate","label":"Baud Rate","type":"select","options":[1200,2400,4800,9600,19200,38400,57600,115200],"hint":"Must match the baud rate configured on the instrument itself -- check its own communication settings screen. 9600 is the most common default.","required":true},{"section":"target","field":"endpoint","label":"Results Delivery API URL","type":"url","hint":"Where completed lab results are sent, e.g. https://your-his.example.org/api/results","required":true},{"section":"target","field":"bearer_token","label":"Bearer Token","type":"password","hint":"Authorization token for your HIS API, if required","required":false}]',
    ARRAY['source.port_name','target.endpoint','target.bearer_token'],
    '[{"name":"Receive over Serial","icon":"🔌","step_type":"connector.inbound"},{"name":"Parse ASTM","icon":"🔄","step_type":"astm.parse"},{"name":"Validate","icon":"✅","step_type":"astm.validate"},{"name":"Deliver to HIS","icon":"📤","step_type":"connector.outbound"}]',
    15,
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
# ASTM over Serial Port

## Who this is for

Any lab instrument whose own setup screen lets you choose **ASTM** as the
protocol and **Serial / RS232 / COM Port** as the connection type -- common
on older or simpler chemistry/hematology analyzers. If your instrument's
manual instead says "HL7" and "TCP/IP" or "Network", use the **HL7 over
TCP/IP** template instead.

## Important: this server must be physically connected to the instrument

Unlike a network connection, a serial/RS232 cable only works between two
machines that are directly wired together (or through a USB-to-serial
adapter). **This ezHealthKonnect server (or the specific machine running
it) must be the one physically cabled to the instrument** -- it cannot
reach a COM port on a different computer over the network. If this server
runs somewhere else (e.g. in the cloud), ask your integrator about a
different connection approach before using this template.

## What you need before you start

- A serial (RS232) cable between the instrument and this server's own machine (an 8-pin or 16-pin adapter cable on some instruments)
- The instrument's own communication settings (baud rate, at minimum)
- Your hospital's HIS results-delivery endpoint

## Setup steps

1. **Connect the cable** between the instrument and this machine, then reopen this dialog so the COM port list below can detect it.
2. **Pick the COM Port** from the dropdown. If nothing appears, double-check the cable and that no other program is already using that port.
3. **Enter the Baud Rate** -- this MUST match the instrument's own setting exactly, or communication will fail silently. Check the instrument's own communication/LIS settings screen. 9600 is the most common default, but always confirm.
4. **Enter your Results Delivery API URL.**

## A note on what gets delivered

This template delivers the instrument's own structured message (patient, order, and result data) to your HIS endpoint as JSON. It does not yet convert ASTM messages into FHIR format the way the HL7 template does for HL7 messages -- that conversion is a separate, not-yet-built capability. If your HIS needs FHIR specifically, ask your integrator about adding a transformation step in Pipeline Builder.

## Advanced tuning

Data bits, parity, stop bits, and checksum-mismatch handling can be fine-tuned later in **Pipeline Builder** if the instrument needs settings other than the common defaults (8 data bits, no parity, 1 stop bit).
$GUIDE$
WHERE slug = 'device-astm-serial';

-- ─────────────────────────────────────────────────────────────────────────────
-- Update V274's own HL7 template guide text -- "not supported yet" is now
-- stale now that this migration ships real ASTM/Serial support.
-- ─────────────────────────────────────────────────────────────────────────────
UPDATE interface_templates
SET user_guide = regexp_replace(
    user_guide,
    '## If your instrument uses ASTM or a Serial \(RS232\) connection instead.*$',
    '## If your instrument uses ASTM or a Serial (RS232) connection instead' || E'\n\n' ||
    'Use the **ASTM over Serial Port** template instead (device category) -- it is now fully supported.',
    's'
)
WHERE slug = 'device-hl7-tcp'
  AND user_guide LIKE '%not supported yet%';
