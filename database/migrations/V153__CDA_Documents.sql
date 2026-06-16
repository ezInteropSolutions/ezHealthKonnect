-- V153__CDA_Documents.sql
-- Sprint E: CDA Document Storage — hybrid inline/object-storage routing.
--
-- Documents ≤ 1 MB: raw XML + parsed JSON stored inline in PostgreSQL JSONB.
-- Documents > 1 MB: raw XML + parsed JSON stored in MinIO/S3;
--                   this table holds metadata + object URIs only.
--
-- storage_mode = 'inline' → raw_xml_inline + parsed_json are populated
-- storage_mode = 'object' → raw_xml_uri + parsed_json_uri are populated

CREATE TABLE IF NOT EXISTS cda_documents (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  interface_id          UUID REFERENCES interfaces(id) ON DELETE SET NULL,
  pipeline_execution_id UUID,
  message_id            VARCHAR(255),

  -- CDA document identity (from document header)
  document_id           VARCHAR(255),
  document_type         VARCHAR(100) NOT NULL DEFAULT 'C-CDA 2.1',
  status                VARCHAR(50)  NOT NULL DEFAULT 'parsed',

  -- Patient demographics (denormalized for fast filtering)
  patient_id            VARCHAR(255),
  patient_name          VARCHAR(500),
  document_date         TIMESTAMP WITH TIME ZONE,
  author_name           VARCHAR(500),
  custodian_name        VARCHAR(500),

  -- Parse metrics
  section_count         INTEGER      NOT NULL DEFAULT 0,
  field_count           INTEGER      NOT NULL DEFAULT 0,
  parse_duration_ms     INTEGER,
  raw_xml_size_bytes    INTEGER,

  -- Hybrid storage routing
  storage_mode          VARCHAR(20)  NOT NULL DEFAULT 'inline'
                          CHECK (storage_mode IN ('inline', 'object')),

  -- Inline storage (storage_mode = 'inline')
  raw_xml_inline        TEXT,
  parsed_json           JSONB,

  -- Object storage URIs (storage_mode = 'object')
  raw_xml_uri           VARCHAR(1000),
  parsed_json_uri       VARCHAR(1000),

  -- FHIR output (optional, populated by downstream cda.to_fhir step)
  fhir_bundle           JSONB,
  fhir_bundle_uri       VARCHAR(1000),

  -- Conformance validation (optional, from Sprint C validator)
  compliance_score      DECIMAL(5, 4),
  compliance_report     JSONB,

  -- Error tracking and soft delete
  error_message         TEXT,
  deleted_at            TIMESTAMP WITH TIME ZONE,

  created_at            TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  updated_at            TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Performance indexes
CREATE INDEX IF NOT EXISTS idx_cda_documents_interface_id
  ON cda_documents(interface_id);

CREATE INDEX IF NOT EXISTS idx_cda_documents_message_id
  ON cda_documents(message_id)
  WHERE message_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_cda_documents_document_id
  ON cda_documents(document_id)
  WHERE document_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_cda_documents_patient_id
  ON cda_documents(patient_id)
  WHERE patient_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_cda_documents_created_at
  ON cda_documents(created_at DESC)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_cda_documents_status
  ON cda_documents(status)
  WHERE deleted_at IS NULL;

-- Auto-update updated_at on row modification
CREATE TRIGGER cda_documents_updated_at
  BEFORE UPDATE ON cda_documents
  FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
