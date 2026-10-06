-- V277: Add connectivity_types row for dicom_storage_inbound, and a new
-- "DICOM Storage SCP" interface_templates row under the `device` category —
-- Phase 2 of the broader device-connectivity effort (Phase 0/1 shipped lab
-- instrument connectivity over HL7/ASTM; this phase targets imaging
-- devices, e.g. a Fujifilm CR-IR 392 CR reader). A genuinely different
-- protocol family (DICOM/DIMSE over TCP), built on github.com/amrshadid/
-- go-dicom v1.6.0 — real DIMSE networking, not hand-rolled the way ASTM's
-- own framing had to be (no comparable library existed for ASTM).

-- ============================================================
-- BACKGROUND
-- ============================================================
--   dicom_storage_inbound -- a DICOM Storage SCP (server): answers C-ECHO
--                             (verification) and C-STORE (image receive)
--                             only, this phase's own explicit scope
--                             boundary -- no Modality Worklist, no MPPS, no
--                             Storage Commitment. The modality (e.g. a CR
--                             reader) always initiates the association as a
--                             Storage SCU -- confirmed against the real
--                             Fujifilm conformance statement this plan was
--                             built from -- so this server never dials out.
--
-- Config field names match services/connectors/dicom_storage_inbound.go
-- exactly -- confirmed by reading that file directly.
--
-- Port 11112 is the conventional DICOM default. Requires a separate,
-- already-applied docker-compose.yml change ("11112-11120:11112-11120")
-- since no DICOM port range existed before this phase.

INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, priority, version,
    ui_category, ui_sort_order
) VALUES (
    'dicom_storage_inbound',
    'inbound',
    'DICOM Storage SCP',
    'Receive DICOM images pushed from an imaging modality (e.g. a CR/DX reader) acting as a Storage SCU. Answers C-ECHO (verification) and C-STORE (image receive) only -- no worklist, MPPS, or storage commitment this phase.',
    '🩻',
    'push',
    false,
    false,
    false,
    'DICOMStorageInboundConnector',
    '{
        "type": "object",
        "required": ["ae_title", "port"],
        "properties": {
            "ae_title": {"type": "string", "title": "This SCP''s AE Title", "description": "The DICOM Application Entity title this server answers to. Enter this exact value into the modality''s own destination/AE configuration."},
            "port": {"type": "integer", "title": "Listener Port", "default": 11112},
            "bind_address": {"type": "string", "title": "Bind Address", "default": "0.0.0.0"},
            "max_associations": {"type": "integer", "title": "Max Concurrent Associations", "default": 0, "description": "0 = unlimited"},
            "allowed_calling_ae_titles": {"type": "array", "items": {"type": "string"}, "title": "Allowed Calling AE Titles", "description": "Leave empty to accept any calling AE title (not recommended for internet-reachable deployments)"},
            "additional_abstract_syntaxes": {"type": "array", "items": {"type": "string"}, "title": "Additional Storage SOP Class UIDs", "description": "Extra SOP Class UIDs to accept beyond the built-in CR/Digital X-Ray/Secondary Capture defaults"},
            "tls_enabled": {"type": "boolean", "title": "Enable TLS", "default": false},
            "tls_cert_file": {"type": "string", "title": "TLS Certificate File Path"},
            "tls_key_file": {"type": "string", "title": "TLS Private Key File Path"}
        }
    }'::jsonb,
    '{
        "basic": ["ae_title", "port", "bind_address"],
        "advanced": ["max_associations", "allowed_calling_ae_titles", "additional_abstract_syntaxes", "tls_enabled", "tls_cert_file", "tls_key_file"]
    }'::jsonb,
    true,
    100,
    '1.0',
    'Healthcare Network',
    21
)
ON CONFLICT (type_name) DO UPDATE SET
    config_schema = EXCLUDED.config_schema,
    parameter_groups = EXCLUDED.parameter_groups,
    is_active = EXCLUDED.is_active,
    updated_at = NOW();

-- ─────────────────────────────────────────────────────────────────────────────
-- "DICOM Storage SCP" interface template
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
    'DICOM Storage SCP',
    'device-dicom-storage-scp',
    'Connect an imaging modality (e.g. a Fujifilm CR-IR 392 CR reader, or any other device sending CR/Digital X-Ray/Secondary Capture images) that pushes DICOM images to this server as a Storage SCU. This template answers C-ECHO (verification) and C-STORE (image receive) only -- the modality always connects out to this server; this server never connects to the modality.',
    'device',
    'imaging',
    ARRAY['dicom','cr','dx','imaging','x-ray','device'],
    '🩻',
    'intermediate',
    'dicom_storage_inbound',
    '{"ae_title":"EZHEALTHKONNECT","port":11112,"bind_address":"0.0.0.0"}',
    'http_outbound',
    '{"method":"POST","content_type":"application/json","auth_type":"bearer","timeout_seconds":30}',
    '{"execution_groups":[{"sequence":5,"steps":[{"step_name":"DICOM Storage SCP","step_type":"connector.inbound","sequence":5,"config":{"connectorType":"dicom_storage_inbound","config":{"ae_title":"EZHEALTHKONNECT","port":11112,"bind_address":"0.0.0.0","max_associations":0},"timeoutMs":30000},"enabled":true,"required":true}]},{"sequence":10,"steps":[{"step_name":"Parse DICOM Metadata","step_type":"dicom.parse","sequence":10,"config":{"sourceField":"raw","outputField":"parsedDICOM"},"enabled":true,"required":true}]},{"sequence":20,"steps":[{"step_name":"Deliver to HIS","step_type":"connector.outbound","sequence":20,"config":{"connectorType":"http_outbound","config":{"method":"POST","content_type":"application/json","auth_type":"bearer","timeout_seconds":30},"contentField":"parsedDICOM","contentType":"application/json"},"enabled":true,"required":true}]}]}',
    'DICOM',
    '[{"section":"source","field":"ae_title","label":"This Server''s AE Title","type":"string","hint":"The DICOM Application Entity title this server answers to. Enter this exact value into the modality''s own destination/AE configuration.","required":true},{"section":"source","field":"port","label":"Listener Port","type":"number","hint":"The network port this server listens on for DICOM associations. Enter this same number into the modality''s own destination configuration.","required":true},{"section":"target","field":"endpoint","label":"Results Delivery API URL","type":"url","hint":"Where received-image metadata (and a reference to the stored image) is sent, e.g. https://your-his.example.org/api/images","required":true},{"section":"target","field":"bearer_token","label":"Bearer Token","type":"password","hint":"Authorization token for your HIS API, if required","required":false}]',
    ARRAY['source.ae_title','source.port','target.endpoint','target.bearer_token'],
    '[{"name":"Receive DICOM Image","icon":"🩻","step_type":"connector.inbound"},{"name":"Parse Metadata","icon":"🔄","step_type":"dicom.parse"},{"name":"Deliver to HIS","icon":"📤","step_type":"connector.outbound"}]',
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
# DICOM Storage SCP

## Who this is for

Any imaging modality that pushes DICOM images to a destination over a
network connection -- most commonly a CR (Computed Radiography) or DX
(Digital X-Ray) reader, such as a Fujifilm CR-IR 392. On the modality's own
side, this is usually configured as a "destination," "remote AE," or
"PACS" entry -- the modality always connects OUT to this server; this
server never connects to the modality.

## What you need before you start

- A network path from the modality to this server (same LAN, or an
  internal device VLAN -- this connector is not designed to be reachable
  from the public internet)
- Access to the modality's own network/destination configuration screen
- This server's own AE title, which you choose below and must enter
  verbatim on the modality's side
- Your hospital's HIS endpoint for receiving image metadata

## Setup steps

1. **Choose an AE Title** for this server (the default above is a
   reasonable starting point) and a **Port Number** (11112 is the
   conventional DICOM default).
2. **On the modality itself**, open its destination/remote-AE
   configuration screen and add this server as a destination: enter this
   server's network address, the **same port number**, and the **same AE
   title** you entered above.
3. **Enter your Results Delivery API URL.**
4. **Send a test image** from the modality (most modalities have a
   "verify connection" / C-ECHO test button -- try that first) and confirm
   it arrives.

## A note on what gets delivered

The full DICOM image is stored intact (pixel data travels as opaque bytes
-- this server does not decode or re-render images). This template
delivers the image's own identifying metadata (patient ID, study/series/
instance UIDs, modality, accession number, etc.) to your HIS endpoint as
JSON. If your HIS needs the image bytes themselves forwarded too, or needs
FHIR `ImagingStudy`/`DocumentReference` resources specifically, ask your
integrator about adding a transformation step in Pipeline Builder.

## A note on scope

This template answers verification (C-ECHO) and image receive (C-STORE)
only. It does not yet support Modality Worklist (the modality querying this
server for a scheduled patient/procedure before scanning), MPPS (procedure
status reporting), or Storage Commitment (the modality asking this server
to confirm long-term archival before deleting its own local copy). Ask your
integrator if your site's workflow depends on any of these.

## A note on security

By default this server accepts a connection from any calling AE title. If
you know which modality(ies) will connect, set "Allowed Calling AE Titles"
in Pipeline Builder's advanced connector settings to restrict connections
to those AE titles specifically.
$GUIDE$
WHERE slug = 'device-dicom-storage-scp';
