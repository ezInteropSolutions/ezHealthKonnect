-- V96__ZSegment_Mapping_Config.sql
--
-- Enterprise Z-segment mapping configuration.
--
-- Architecture:
--   zsegment_templates      — reusable Z-segment definitions (OOB + user-created)
--   zsegment_template_fields — field-level mappings belonging to a template
--   mfn_interface_configs   — per-interface MFI.1 → FHIR resource overrides (Tier 2)
--   zsegment_configs        — per-interface Z-segment assignments (Tier 3)
--   zsegment_field_mappings — per-Z-segment field → FHIR path mappings (Tier 3)
--
-- Templates are standalone; applying one copies its fields into an interface's
-- zsegment_field_mappings so the interface config is fully self-contained and
-- survives template edits or deletion.
--
-- Z-segments are per-interface only (no message_type filter).  Detection is
-- driven by the wizard's sample-message parser or manual "Add Z-segment" action.

-- ── OOB transform catalog (reference values for UI dropdowns) ───────────────
-- Stored as a simple reference table so the UI and Go layer share the same list.
CREATE TABLE IF NOT EXISTS zsegment_transforms (
    id          SERIAL PRIMARY KEY,
    code        VARCHAR(50)  NOT NULL UNIQUE,
    label       VARCHAR(100) NOT NULL,
    description TEXT,
    input_types TEXT[]       NOT NULL DEFAULT '{}',  -- HL7 data types this applies to
    sort_order  INTEGER      NOT NULL DEFAULT 0
);

INSERT INTO zsegment_transforms (code, label, description, input_types, sort_order) VALUES
  ('identity',         'As-Is',                    'Copy value without transformation',                  ARRAY['ST','IS','ID','NM','SI'], 10),
  ('first_component',  'First Component (^)',       'Take first ^-delimited component',                   ARRAY['CWE','CE','CX','HD','EI'], 20),
  ('second_component', 'Second Component (^)',      'Take second ^-delimited component',                  ARRAY['CWE','CE','CX'], 30),
  ('concat_components','Concatenate Components',    'Join all ^ components with a space',                 ARRAY['CWE','CE','XPN','XON'], 40),
  ('xad_to_address',   'XAD → FHIR Address',        'Parse XAD composite into FHIR Address object',       ARRAY['XAD'], 50),
  ('xpn_to_humanname', 'XPN → FHIR HumanName',      'Parse XPN composite into FHIR HumanName object',     ARRAY['XPN'], 60),
  ('xtn_to_telecom',   'XTN → FHIR ContactPoint',   'Parse XTN composite into FHIR ContactPoint object',  ARRAY['XTN'], 70),
  ('cx_to_identifier', 'CX → FHIR Identifier',      'Parse CX composite into FHIR Identifier object',     ARRAY['CX'], 80),
  ('cwe_to_codeable',  'CWE → CodeableConcept',      'Parse CWE/CE into FHIR CodeableConcept',             ARRAY['CWE','CE','CNE'], 90),
  ('xon_to_org_ref',   'XON → Organization ref',    'Parse XON into display reference',                   ARRAY['XON'], 100),
  ('ts_to_date',       'TS → Date (YYYY-MM-DD)',     'Format TS as ISO date only',                         ARRAY['TS','DT'], 110),
  ('ts_to_datetime',   'TS → DateTime (ISO 8601)',   'Format TS as ISO 8601 datetime',                     ARRAY['TS','DTM'], 120),
  ('boolean_yn',       'Y/N → Boolean',              'Convert Y/y → true, N/n → false',                   ARRAY['ID','IS'], 130),
  ('boolean_ai',       'A/I → Boolean',              'Convert A → true (active), I → false (inactive)',    ARRAY['ID','IS'], 140)
ON CONFLICT (code) DO NOTHING;

-- ── Template: reusable Z-segment definition ──────────────────────────────────
CREATE TABLE IF NOT EXISTS zsegment_templates (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_name        VARCHAR(200) NOT NULL,
    segment_id           VARCHAR(10)  NOT NULL,   -- ZEM, ZPI, ZIN, ZPD, ZAL, etc.
    target_fhir_resource VARCHAR(100) NOT NULL,   -- Organization, Coverage, Practitioner, etc.
    resource_role        VARCHAR(20)  NOT NULL DEFAULT 'primary',
    -- 'primary'   = this Z-seg IS the main assembled resource
    -- 'related'   = added as a second resource in the bundle, linked to primary
    -- 'extension' = adds extension fields onto an existing resource
    description          TEXT,
    tags                 TEXT[]       NOT NULL DEFAULT '{}',
    is_system            BOOLEAN      NOT NULL DEFAULT false,  -- OOB template
    is_public            BOOLEAN      NOT NULL DEFAULT true,   -- visible to all users
    created_by_user_id   UUID         REFERENCES users(id) ON DELETE SET NULL,
    usage_count          INTEGER      NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_zsegment_templates_segment ON zsegment_templates(segment_id);
CREATE INDEX IF NOT EXISTS idx_zsegment_templates_system  ON zsegment_templates(is_system, is_public);

-- Template field mappings
CREATE TABLE IF NOT EXISTS zsegment_template_fields (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id          UUID NOT NULL REFERENCES zsegment_templates(id) ON DELETE CASCADE,
    position             VARCHAR(20)  NOT NULL,   -- ZEM.1, ZEM.2, ZEM.3.1, etc.
    field_label          VARCHAR(200),            -- human label: "Employer ID"
    hl7_data_type        VARCHAR(10)  NOT NULL DEFAULT 'ST',
    fhir_path            VARCHAR(300) NOT NULL,   -- name, identifier[0].value, address[0], …
    transform            VARCHAR(50)  NOT NULL DEFAULT 'identity',
    is_required          BOOLEAN      NOT NULL DEFAULT false,
    default_value        TEXT,
    sort_order           INTEGER      NOT NULL DEFAULT 0,
    UNIQUE(template_id, position)
);

CREATE INDEX IF NOT EXISTS idx_zsegment_template_fields ON zsegment_template_fields(template_id, sort_order);

-- ── Per-interface: MFI.1 → FHIR resource override (Tier 2) ──────────────────
CREATE TABLE IF NOT EXISTS mfn_interface_configs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id    UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    mfi_identifier  VARCHAR(20)  NOT NULL,   -- EMP, INS, PAY, M13, custom
    fhir_resource   VARCHAR(100) NOT NULL,   -- Organization, Person, Practitioner, Coverage
    description     TEXT,
    is_active       BOOLEAN      NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE(interface_id, mfi_identifier)
);

CREATE INDEX IF NOT EXISTS idx_mfn_interface_configs ON mfn_interface_configs(interface_id);

-- ── Per-interface: Z-segment assignments (Tier 3) ───────────────────────────
-- No message_type column — Z-segment config is per-interface.
-- Detection is wizard-driven (auto-detect from sample OR manual add).
CREATE TABLE IF NOT EXISTS zsegment_configs (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id         UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    segment_id           VARCHAR(10)  NOT NULL,   -- ZEM, ZPI, ZIN, etc.
    target_fhir_resource VARCHAR(100) NOT NULL,
    resource_role        VARCHAR(20)  NOT NULL DEFAULT 'primary',
    description          TEXT,
    occurrence_index     INTEGER      NOT NULL DEFAULT 0,  -- 0=first, 1=second if same seg appears twice
    source_template_id   UUID         REFERENCES zsegment_templates(id) ON DELETE SET NULL,
    -- non-null = was applied from a template (informational only; fields are copied, not live-linked)
    is_active            BOOLEAN      NOT NULL DEFAULT true,
    sort_order           INTEGER      NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_zsegment_configs_interface ON zsegment_configs(interface_id);

-- Per-Z-segment field mappings (self-contained — independent of template after apply)
CREATE TABLE IF NOT EXISTS zsegment_field_mappings (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    zsegment_config_id  UUID NOT NULL REFERENCES zsegment_configs(id) ON DELETE CASCADE,
    position            VARCHAR(20)  NOT NULL,   -- ZEM.1, ZEM.2.1, etc.
    field_label         VARCHAR(200),
    hl7_data_type       VARCHAR(10)  NOT NULL DEFAULT 'ST',
    fhir_path           VARCHAR(300) NOT NULL,
    transform           VARCHAR(50)  NOT NULL DEFAULT 'identity',
    is_required         BOOLEAN      NOT NULL DEFAULT false,
    default_value       TEXT,
    sort_order          INTEGER      NOT NULL DEFAULT 0,
    UNIQUE(zsegment_config_id, position)
);

CREATE INDEX IF NOT EXISTS idx_zsegment_field_mappings ON zsegment_field_mappings(zsegment_config_id, sort_order);

-- ── OOB templates ────────────────────────────────────────────────────────────
-- ZEM — Employer/Payer Z-segment (common in benefits/insurance feeds)
INSERT INTO zsegment_templates (template_name, segment_id, target_fhir_resource, resource_role, description, tags, is_system)
VALUES (
    'ZEM — Employer Organization',
    'ZEM', 'Organization', 'primary',
    'Employer Z-segment used in benefits/payer MFN feeds. Field positions follow common vendor layouts (e.g. TriZetto, Facets). Adjust positions for your trading partner.',
    ARRAY['employer','organization','payer','benefits'],
    true
) ON CONFLICT DO NOTHING;

-- ZEM fields
WITH t AS (SELECT id FROM zsegment_templates WHERE segment_id = 'ZEM' AND is_system = true LIMIT 1)
INSERT INTO zsegment_template_fields (template_id, position, field_label, hl7_data_type, fhir_path, transform, is_required, sort_order)
SELECT t.id, v.position, v.field_label, v.hl7_data_type, v.fhir_path, v.transform, v.is_required, v.sort_order
FROM t,
(VALUES
    ('ZEM.1', 'Employer ID',    'CWE', 'identifier[0].value', 'first_component', true,  10),
    ('ZEM.2', 'Employer Name',  'ST',  'name',                'identity',        true,  20),
    ('ZEM.3', 'Address',        'XAD', 'address[0]',          'xad_to_address',  false, 30),
    ('ZEM.4', 'Plan/Coverage Type', 'IS', 'contact[0].name.text', 'identity',    false, 40),
    ('ZEM.5', 'Phone',          'XTN', 'telecom[0]',          'xtn_to_telecom',  false, 50)
) AS v(position, field_label, hl7_data_type, fhir_path, transform, is_required, sort_order)
ON CONFLICT DO NOTHING;

-- ZPI — Patient Insurance Z-segment
INSERT INTO zsegment_templates (template_name, segment_id, target_fhir_resource, resource_role, description, tags, is_system)
VALUES (
    'ZPI — Patient Insurance',
    'ZPI', 'Coverage', 'related',
    'Patient insurance Z-segment. Maps insurer ID, plan name, group/member numbers to FHIR Coverage. Field positions vary by vendor.',
    ARRAY['insurance','coverage','patient'],
    true
) ON CONFLICT DO NOTHING;

WITH t AS (SELECT id FROM zsegment_templates WHERE segment_id = 'ZPI' AND is_system = true LIMIT 1)
INSERT INTO zsegment_template_fields (template_id, position, field_label, hl7_data_type, fhir_path, transform, is_required, sort_order)
SELECT t.id, v.position, v.field_label, v.hl7_data_type, v.fhir_path, v.transform, v.is_required, v.sort_order
FROM t,
(VALUES
    ('ZPI.1', 'Plan ID',        'CWE', 'identifier[0].value', 'first_component', true,  10),
    ('ZPI.2', 'Plan Name',      'ST',  'class[0].name',        'identity',        false, 20),
    ('ZPI.3', 'Group Number',   'ST',  'class[0].value',       'identity',        false, 30),
    ('ZPI.4', 'Member ID',      'CX',  'identifier[1].value',  'first_component', false, 40),
    ('ZPI.5', 'Effective Date', 'DT',  'period.start',         'ts_to_date',      false, 50),
    ('ZPI.6', 'Expiry Date',    'DT',  'period.end',           'ts_to_date',      false, 60)
) AS v(position, field_label, hl7_data_type, fhir_path, transform, is_required, sort_order)
ON CONFLICT DO NOTHING;

-- ZIN — Insurance Network / Additional Info
INSERT INTO zsegment_templates (template_name, segment_id, target_fhir_resource, resource_role, description, tags, is_system)
VALUES (
    'ZIN — Insurance Network Info',
    'ZIN', 'Coverage', 'extension',
    'Insurance network Z-segment — adds network/plan detail extensions to a Coverage resource.',
    ARRAY['insurance','network','coverage'],
    true
) ON CONFLICT DO NOTHING;

WITH t AS (SELECT id FROM zsegment_templates WHERE segment_id = 'ZIN' AND is_system = true LIMIT 1)
INSERT INTO zsegment_template_fields (template_id, position, field_label, hl7_data_type, fhir_path, transform, is_required, sort_order)
SELECT t.id, v.position, v.field_label, v.hl7_data_type, v.fhir_path, v.transform, v.is_required, v.sort_order
FROM t,
(VALUES
    ('ZIN.1', 'Network ID',      'IS', 'identifier[0].value', 'identity', false, 10),
    ('ZIN.2', 'Network Name',    'ST', 'class[1].name',        'identity', false, 20),
    ('ZIN.3', 'Copay Amount',    'NM', 'costToBeneficiary[0].valueQuantity.value', 'identity', false, 30)
) AS v(position, field_label, hl7_data_type, fhir_path, transform, is_required, sort_order)
ON CONFLICT DO NOTHING;

-- ZPD — Patient Demographics Extension
INSERT INTO zsegment_templates (template_name, segment_id, target_fhir_resource, resource_role, description, tags, is_system)
VALUES (
    'ZPD — Patient Demographics Extension',
    'ZPD', 'Patient', 'extension',
    'Patient demographics Z-segment. Adds site-specific demographic fields not covered by PID/PD1 to a FHIR Patient.',
    ARRAY['patient','demographics'],
    true
) ON CONFLICT DO NOTHING;

WITH t AS (SELECT id FROM zsegment_templates WHERE segment_id = 'ZPD' AND is_system = true LIMIT 1)
INSERT INTO zsegment_template_fields (template_id, position, field_label, hl7_data_type, fhir_path, transform, is_required, sort_order)
SELECT t.id, v.position, v.field_label, v.hl7_data_type, v.fhir_path, v.transform, v.is_required, v.sort_order
FROM t,
(VALUES
    ('ZPD.1', 'Race',              'CWE', 'extension[0].valueCodeableConcept', 'cwe_to_codeable', false, 10),
    ('ZPD.2', 'Ethnicity',         'CWE', 'extension[1].valueCodeableConcept', 'cwe_to_codeable', false, 20),
    ('ZPD.3', 'Preferred Language','CWE', 'communication[0].language',         'cwe_to_codeable', false, 30)
) AS v(position, field_label, hl7_data_type, fhir_path, transform, is_required, sort_order)
ON CONFLICT DO NOTHING;

-- ZAL — Allergy Extension
INSERT INTO zsegment_templates (template_name, segment_id, target_fhir_resource, resource_role, description, tags, is_system)
VALUES (
    'ZAL — Allergy Extension',
    'ZAL', 'AllergyIntolerance', 'related',
    'Allergy Z-segment. Supplements AL1 with additional allergy detail fields into a FHIR AllergyIntolerance resource.',
    ARRAY['allergy','clinical'],
    true
) ON CONFLICT DO NOTHING;

WITH t AS (SELECT id FROM zsegment_templates WHERE segment_id = 'ZAL' AND is_system = true LIMIT 1)
INSERT INTO zsegment_template_fields (template_id, position, field_label, hl7_data_type, fhir_path, transform, is_required, sort_order)
SELECT t.id, v.position, v.field_label, v.hl7_data_type, v.fhir_path, v.transform, v.is_required, v.sort_order
FROM t,
(VALUES
    ('ZAL.1', 'Allergy Code',     'CWE', 'code',              'cwe_to_codeable', true,  10),
    ('ZAL.2', 'Severity',         'CWE', 'criticality',        'first_component', false, 20),
    ('ZAL.3', 'Onset Date',       'DT',  'onsetDateTime',      'ts_to_date',      false, 30),
    ('ZAL.4', 'Reaction',         'CWE', 'reaction[0].manifestation[0]', 'cwe_to_codeable', false, 40)
) AS v(position, field_label, hl7_data_type, fhir_path, transform, is_required, sort_order)
ON CONFLICT DO NOTHING;

-- ZDX — Diagnosis Extension
INSERT INTO zsegment_templates (template_name, segment_id, target_fhir_resource, resource_role, description, tags, is_system)
VALUES (
    'ZDX — Diagnosis Extension',
    'ZDX', 'Condition', 'related',
    'Diagnosis Z-segment. Adds site-specific diagnosis fields into a FHIR Condition resource.',
    ARRAY['diagnosis','clinical','condition'],
    true
) ON CONFLICT DO NOTHING;

WITH t AS (SELECT id FROM zsegment_templates WHERE segment_id = 'ZDX' AND is_system = true LIMIT 1)
INSERT INTO zsegment_template_fields (template_id, position, field_label, hl7_data_type, fhir_path, transform, is_required, sort_order)
SELECT t.id, v.position, v.field_label, v.hl7_data_type, v.fhir_path, v.transform, v.is_required, v.sort_order
FROM t,
(VALUES
    ('ZDX.1', 'Diagnosis Code',   'CWE', 'code',               'cwe_to_codeable', true,  10),
    ('ZDX.2', 'Diagnosis Date',   'DT',  'onsetDateTime',       'ts_to_date',      false, 20),
    ('ZDX.3', 'Diagnosis Type',   'IS',  'category[0].text',    'identity',        false, 30),
    ('ZDX.4', 'Clinician',        'XCN', 'asserter.display',    'concat_components',false,40)
) AS v(position, field_label, hl7_data_type, fhir_path, transform, is_required, sort_order)
ON CONFLICT DO NOTHING;

-- ── Update trigger for updated_at ────────────────────────────────────────────
CREATE OR REPLACE FUNCTION update_zsegment_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END;
$$;

DO $$
DECLARE tbl TEXT;
BEGIN
    FOREACH tbl IN ARRAY ARRAY['mfn_interface_configs','zsegment_configs','zsegment_templates']
    LOOP
        EXECUTE format(
            'DROP TRIGGER IF EXISTS trg_%s_updated_at ON %s;
             CREATE TRIGGER trg_%s_updated_at
             BEFORE UPDATE ON %s
             FOR EACH ROW EXECUTE FUNCTION update_zsegment_updated_at();',
            tbl, tbl, tbl, tbl
        );
    END LOOP;
END;
$$;
