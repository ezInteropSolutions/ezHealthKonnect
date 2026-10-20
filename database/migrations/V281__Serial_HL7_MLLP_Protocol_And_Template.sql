-- V281: HL7 v2 over RS232 — a "protocol" choice on serial_inbound plus an
-- "HL7 over Serial Port" device template.
--
-- DECISION (2026-10-19): build, not drop. A prospect's ICU equipment
-- advertises "HL7, LAN and RS232". HL7 v2 over a serial line is a real,
-- if uncommon, deployment pattern — mostly older point-of-care and lab
-- devices; most modern devices and nearly all ventilators reach HL7 over the
-- network or through a vendor gateway, so HL7 over TCP/IP stays the
-- recommended template. Messages travel in the same MLLP frame (VT ... FS CR)
-- as on TCP and are answered with the same HL7 ACK (AA/AE/AR), produced by
-- code shared with tcp_mllp_inbound (services/connectors/hl7_mllp_session.go).
--
-- serial_inbound keeps ASTM as the default protocol, so every existing serial
-- interface is unchanged. The bidirectional host-query (order lookup) mode is
-- TCP-only and is not offered here.
--
-- The template's pipeline is the HL7-over-TCP/IP template's (V274/V279)
-- without its host-query steps, with the source connector swapped — generated
-- from the live device-hl7-tcp row, not retyped.

UPDATE connectivity_types
SET display_name = 'RS232 Serial (ASTM / HL7)',
    description  = 'Continuously read lab/device messages off a physical or virtual COM port: ASTM E1394-97 (default) or HL7 v2 with MLLP framing (answered with an HL7 ACK). Requires this server to run on a machine with a real serial connection to the device.',
    config_schema = jsonb_set(
        jsonb_set(config_schema, '{properties,protocol}',
            '{"type": "string", "title": "Protocol", "enum": ["astm", "hl7_mllp"], "default": "astm", "description": "astm = ASTM E1394-97 (ENQ/STX/ETX/EOT handshake); hl7_mllp = HL7 v2 framed with MLLP start/end bytes, acknowledged with an HL7 ACK (see the Acknowledgment tab)"}'::jsonb),
        '{properties,max_message_size_mb}',
        '{"type": "integer", "title": "Max HL7 Message Size (MB)", "default": 10, "description": "HL7 only: frames larger than this are rejected with AE (hard cap 100)"}'::jsonb),
    parameter_groups = '{"basic": ["port_name", "baud_rate", "protocol"], "advanced": ["data_bits", "parity", "stop_bits", "checksum_severity", "max_message_size_mb"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'serial_inbound';

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
    'HL7 over Serial Port',
    'device-hl7-serial',
    'Connect a device that sends HL7 v2 messages over a physical RS232 serial (COM port) connection -- some point-of-care analyzers, blood gas analyzers and older bedside equipment. Requires this server to run on a machine with a real cable connection to the device. If the device can also send HL7 over the network (LAN), prefer the network template.',
    'device',
    'serial',
    ARRAY['hl7v2','serial','rs232','com-port','mllp','point-of-care'],
    '🔌',
    'intermediate',
    'serial_inbound',
    '{"protocol":"hl7_mllp","port_name":"","baud_rate":9600,"data_bits":8,"parity":"none","stop_bits":"1","ack":{"mode":"immediate","on_error":"nack"}}',
    'http_outbound',
    '{"method":"POST","auth_type":"bearer","content_type":"application/fhir+json","timeout_seconds":30}',
    '{"execution_groups":[{"steps":[{"config":{"connectorType":"serial_inbound","config":{"protocol":"hl7_mllp","port_name":"","baud_rate":9600,"data_bits":8,"parity":"none","stop_bits":"1","ack":{"mode":"immediate","on_error":"nack","sending_app":"ezHealthKonnect","sending_facility":"EHK","text_success":"Message received successfully","text_error":"Message processing error"}},"timeoutMs":30000},"enabled":true,"required":true,"sequence":5,"step_name":"Serial Inbound (HL7 v2)","step_type":"connector.inbound"}],"sequence":5},{"steps":[{"config":{"format":"hl7v2","fail_on_error":false,"required_segments":["MSH","PID","OBR","OBX"]},"enabled":true,"required":false,"sequence":10,"step_name":"Validate HL7 ORU","step_type":"enrichment.validation"}],"sequence":10},{"steps":[{"config":{"message_type":"ORU^R01","include_metadata":true,"target_resources":["Observation","DiagnosticReport","Patient"]},"enabled":true,"required":true,"sequence":20,"step_name":"HL7 → FHIR Transform (ORU^R01)","step_type":"hl7_fhir_transform"}],"sequence":20},{"steps":[{"config":{"format":"fhir_r4","fail_on_error":false},"enabled":true,"required":false,"sequence":30,"step_name":"Validate FHIR Bundle","step_type":"enrichment.validation"}],"sequence":30},{"steps":[{"config":{"config":{"method":"POST","auth_type":"bearer","content_type":"application/fhir+json","timeout_seconds":30},"contentType":"application/fhir+json","contentField":"fhirBundle","connectorType":"http_outbound"},"enabled":true,"required":true,"sequence":295,"step_name":"Deliver Results to HIS","step_type":"connector.outbound"}],"sequence":295}]}',
    'ORU^R01',
    '[{"section":"source","field":"port_name","label":"COM Port","type":"select","optionsEndpoint":"/api/connectivity/serial-ports","hint":"The physical or virtual serial port this server is connected to the device through. If your port isn''t listed, make sure the cable is connected and reopen this dialog, or enter it later in Pipeline Builder.","required":true},{"section":"source","field":"baud_rate","label":"Baud Rate","type":"select","options":[1200,2400,4800,9600,19200,38400,57600,115200],"hint":"Must match the device''s own serial settings exactly. 9600 is the most common default.","required":true},{"section":"target","field":"endpoint","label":"Results Delivery API URL","type":"url","hint":"Where results are sent, e.g. https://your-his.example.org/api/results","required":true},{"section":"target","field":"bearer_token","label":"Bearer Token","type":"password","hint":"Authorization token for your HIS API, if required","required":false}]',
    ARRAY['source.port_name','target.endpoint','target.bearer_token'],
    '[{"name":"Receive over Serial","icon":"🔌","step_type":"connector.inbound"},{"name":"HL7→FHIR Transform","icon":"🔄","step_type":"hl7_fhir_transform"},{"name":"Deliver to HIS","icon":"📤","step_type":"connector.outbound"}]',
    15, true, true, 'ezHealthKonnect', 0
)
ON CONFLICT (slug) DO NOTHING;

UPDATE interface_templates
SET user_guide = $GUIDE$
# HL7 over Serial Port

## Who this is for

A device whose communication settings offer **HL7** as the protocol and
**Serial / RS232 / COM Port** as the connection. If the device can also send
HL7 over the network (LAN / TCP/IP), prefer the **HL7 over TCP/IP** template:
no cable distance limit, and the server can run anywhere on the network.

If the device's serial output is ASTM (common on lab analyzers) use
**ASTM over Serial Port** instead. If its serial output is a vendor-specific
protocol (many ventilators and monitors use one), neither serial template
applies -- use the vendor's gateway, which typically delivers HL7 over the
network.

## Important: this server must be physically connected to the device

An RS232 cable (or USB-to-serial adapter) only works between two machines
wired directly together. The machine running ezHealthKonnect must be the one
cabled to the device.

## What you need before you start

- A serial cable between the device and this server (null-modem if the device's manual says so)
- The device's serial settings: baud rate, data bits, parity, stop bits
- Your HIS results-delivery endpoint

## Setup steps

1. **Connect the cable**, then reopen this dialog so the COM port list detects it.
2. **Pick the COM Port** from the dropdown.
3. **Enter the Baud Rate** -- it must match the device exactly, or nothing arrives. 9600 is the most common default.
4. **Enter your Results Delivery API URL.**

## How messages are acknowledged

Each message must be wrapped in MLLP framing (start byte 0x0B, end bytes
0x1C 0x0D) -- the same framing HL7 uses over TCP. Every message is answered
on the same line with an HL7 ACK: **AA** when accepted, **AE** when it could
not be queued (the device should resend), **AR** when the frame is not an HL7
message. Sender identity, ACK text and a custom ACK script are on the
connector's **Acknowledgment** tab in Pipeline Builder.

Devices that send HL7 over serial **without** MLLP framing are not supported;
look in the device's manual for an "MLLP", "<VT>" or "0x0B" start-of-message
setting.

## Advanced tuning

Data bits, parity, stop bits and maximum message size can be changed later
in **Pipeline Builder**.
$GUIDE$
WHERE slug = 'device-hl7-serial';

-- The HL7-over-TCP and ASTM-over-serial guides point to the new template.
UPDATE interface_templates
SET user_guide = replace(user_guide,
    'Use the **ASTM over Serial Port** template (for a physical RS232/serial connection) or the **ASTM over TCP/IP** template (for a network connection) instead -- both are now fully supported.',
    'Use the **ASTM over Serial Port** template (for a physical RS232/serial connection) or the **ASTM over TCP/IP** template (for a network connection) instead -- both are now fully supported. If your device sends **HL7** over an RS232 serial cable, use **HL7 over Serial Port**.')
WHERE slug = 'device-hl7-tcp';

UPDATE interface_templates
SET user_guide = replace(user_guide,
    E'TCP/IP** template instead.
',
    E'TCP/IP** template instead. If it says "HL7" over a "Serial" / "RS232"
connection, use **HL7 over Serial Port**.
')
WHERE slug = 'device-astm-serial'
  AND user_guide NOT LIKE '%HL7 over Serial Port%';
