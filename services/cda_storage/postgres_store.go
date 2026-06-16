package cdastorage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// PostgresDocumentStore stores CDA documents inline in PostgreSQL JSONB.
// Used directly for documents ≤ InlineSizeThreshold, or as the metadata layer
// for the RoutingDocumentStore when object storage is in use.
type PostgresDocumentStore struct {
	db *sql.DB
}

// NewPostgresDocumentStore constructs a PostgresDocumentStore backed by db.
func NewPostgresDocumentStore(db *sql.DB) *PostgresDocumentStore {
	return &PostgresDocumentStore{db: db}
}

// Save inserts a new cda_documents row with inline raw XML and parsed JSON.
func (s *PostgresDocumentStore) Save(ctx context.Context, input *SaveInput) (*StoredDocument, error) {
	meta := extractDocumentMeta(input)
	complianceScore, complianceBytes := extractComplianceScore(input.ComplianceReport)

	parsedBytes, err := marshalJSON(input.ParsedJSON)
	if err != nil {
		return nil, fmt.Errorf("cda_storage/postgres: marshal parsed JSON: %w", err)
	}

	var fhirBytes []byte
	if input.FHIRBundle != nil {
		fhirBytes, _ = marshalJSON(input.FHIRBundle)
	}

	const query = `
		INSERT INTO cda_documents (
			interface_id, pipeline_execution_id, message_id,
			document_id, document_type, status,
			patient_id, patient_name, document_date, author_name, custodian_name,
			section_count, field_count, parse_duration_ms, raw_xml_size_bytes,
			storage_mode, raw_xml_inline, parsed_json,
			fhir_bundle, compliance_score, compliance_report
		) VALUES (
			$1::uuid, $2::uuid, $3,
			$4, $5, 'parsed',
			$6, $7, $8, $9, $10,
			$11, $12, $13, $14,
			'inline', $15, $16,
			$17, $18, $19
		)
		RETURNING id, created_at, updated_at`

	var complianceScoreArg interface{}
	if complianceScore > 0 {
		complianceScoreArg = complianceScore
	}

	row := s.db.QueryRowContext(ctx, query,
		nullStr(input.InterfaceID), nullStr(input.PipelineExecutionID), nullStr(input.MessageID),
		nullStr(meta.DocumentID), meta.DocumentType,
		nullStr(meta.PatientID), nullStr(meta.PatientName), meta.DocumentDate,
		nullStr(meta.AuthorName), nullStr(meta.CustodianName),
		input.SectionCount, input.FieldCount, input.ParseDurationMs, len(input.RawXML),
		input.RawXML, parsedBytes,
		nullBytes(fhirBytes), complianceScoreArg, nullBytes(complianceBytes),
	)

	doc := s.buildStoredDocFromMeta(meta, input)
	doc.StorageMode = StorageModeInline
	if complianceScore > 0 {
		doc.ComplianceScore = complianceScore
	}

	if err := row.Scan(&doc.ID, &doc.CreatedAt, &doc.UpdatedAt); err != nil {
		return nil, fmt.Errorf("cda_storage/postgres: insert cda_documents: %w", err)
	}
	return doc, nil
}

// SaveWithObjectURIs inserts a cda_documents row with object storage URIs (no inline content).
// Called by RoutingDocumentStore after uploading large documents to MinIO/S3.
func (s *PostgresDocumentStore) SaveWithObjectURIs(ctx context.Context, input *SaveInput, rawXMLURI, parsedJSONURI string) (*StoredDocument, error) {
	meta := extractDocumentMeta(input)
	complianceScore, complianceBytes := extractComplianceScore(input.ComplianceReport)

	var fhirBytes []byte
	if input.FHIRBundle != nil {
		fhirBytes, _ = marshalJSON(input.FHIRBundle)
	}

	const query = `
		INSERT INTO cda_documents (
			interface_id, pipeline_execution_id, message_id,
			document_id, document_type, status,
			patient_id, patient_name, document_date, author_name, custodian_name,
			section_count, field_count, parse_duration_ms, raw_xml_size_bytes,
			storage_mode, raw_xml_uri, parsed_json_uri,
			fhir_bundle, compliance_score, compliance_report
		) VALUES (
			$1::uuid, $2::uuid, $3,
			$4, $5, 'parsed',
			$6, $7, $8, $9, $10,
			$11, $12, $13, $14,
			'object', $15, $16,
			$17, $18, $19
		)
		RETURNING id, created_at, updated_at`

	var complianceScoreArg interface{}
	if complianceScore > 0 {
		complianceScoreArg = complianceScore
	}

	row := s.db.QueryRowContext(ctx, query,
		nullStr(input.InterfaceID), nullStr(input.PipelineExecutionID), nullStr(input.MessageID),
		nullStr(meta.DocumentID), meta.DocumentType,
		nullStr(meta.PatientID), nullStr(meta.PatientName), meta.DocumentDate,
		nullStr(meta.AuthorName), nullStr(meta.CustodianName),
		input.SectionCount, input.FieldCount, input.ParseDurationMs, len(input.RawXML),
		rawXMLURI, parsedJSONURI,
		nullBytes(fhirBytes), complianceScoreArg, nullBytes(complianceBytes),
	)

	doc := s.buildStoredDocFromMeta(meta, input)
	doc.StorageMode = StorageModeObject
	if complianceScore > 0 {
		doc.ComplianceScore = complianceScore
	}

	if err := row.Scan(&doc.ID, &doc.CreatedAt, &doc.UpdatedAt); err != nil {
		return nil, fmt.Errorf("cda_storage/postgres: insert cda_documents (object): %w", err)
	}
	return doc, nil
}

func (s *PostgresDocumentStore) Get(ctx context.Context, id string) (*StoredDocument, error) {
	const query = `
		SELECT id,
		       COALESCE(interface_id::text, ''),
		       COALESCE(pipeline_execution_id::text, ''),
		       COALESCE(message_id, ''),
		       COALESCE(document_id, ''),
		       COALESCE(document_type, ''),
		       status,
		       COALESCE(patient_id, ''),
		       COALESCE(patient_name, ''),
		       document_date,
		       COALESCE(author_name, ''),
		       COALESCE(custodian_name, ''),
		       COALESCE(section_count, 0),
		       COALESCE(field_count, 0),
		       COALESCE(parse_duration_ms, 0),
		       COALESCE(raw_xml_size_bytes, 0),
		       storage_mode,
		       COALESCE(compliance_score, 0),
		       COALESCE(error_message, ''),
		       created_at,
		       updated_at
		FROM cda_documents
		WHERE id = $1 AND deleted_at IS NULL`

	var doc StoredDocument
	var docDate sql.NullTime
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&doc.ID, &doc.InterfaceID, &doc.PipelineExecutionID,
		&doc.MessageID, &doc.DocumentID, &doc.DocumentType,
		&doc.Status,
		&doc.PatientID, &doc.PatientName, &docDate,
		&doc.AuthorName, &doc.CustodianName,
		&doc.SectionCount, &doc.FieldCount,
		&doc.ParseDurationMs, &doc.RawXMLSizeBytes,
		&doc.StorageMode, &doc.ComplianceScore,
		&doc.ErrorMessage, &doc.CreatedAt, &doc.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cda_storage: document %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("cda_storage/postgres: get document %q: %w", id, err)
	}
	if docDate.Valid {
		doc.DocumentDate = &docDate.Time
	}
	return &doc, nil
}

func (s *PostgresDocumentStore) GetRawXML(ctx context.Context, id string) (string, error) {
	var xml sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT raw_xml_inline FROM cda_documents WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&xml)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("cda_storage: document %q not found", id)
	}
	if err != nil {
		return "", fmt.Errorf("cda_storage/postgres: get raw XML %q: %w", id, err)
	}
	return xml.String, nil
}

// getRawXMLURI returns the object storage URI for a document's raw XML.
// Used by RoutingDocumentStore to fetch from MinIO/S3.
func (s *PostgresDocumentStore) getRawXMLURI(ctx context.Context, id string) (string, error) {
	var uri sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT raw_xml_uri FROM cda_documents WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&uri)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("cda_storage: document %q not found", id)
	}
	if err != nil {
		return "", fmt.Errorf("cda_storage/postgres: get raw XML URI %q: %w", id, err)
	}
	return uri.String, nil
}

func (s *PostgresDocumentStore) GetParsedJSON(ctx context.Context, id string) (map[string]interface{}, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT parsed_json FROM cda_documents WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cda_storage: document %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("cda_storage/postgres: get parsed JSON %q: %w", id, err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("cda_storage/postgres: unmarshal parsed JSON %q: %w", id, err)
	}
	return result, nil
}

// getParsedJSONURI returns the object storage URI for a document's parsed JSON.
func (s *PostgresDocumentStore) getParsedJSONURI(ctx context.Context, id string) (string, error) {
	var uri sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT parsed_json_uri FROM cda_documents WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&uri)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("cda_storage: document %q not found", id)
	}
	if err != nil {
		return "", fmt.Errorf("cda_storage/postgres: get parsed JSON URI %q: %w", id, err)
	}
	return uri.String, nil
}

func (s *PostgresDocumentStore) List(ctx context.Context, filter ListFilter) ([]*StoredDocument, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	offset := (filter.Page - 1) * filter.Limit

	args := []interface{}{}
	where := "WHERE deleted_at IS NULL"
	n := 1

	if filter.InterfaceID != "" {
		where += fmt.Sprintf(" AND interface_id = $%d::uuid", n)
		args = append(args, filter.InterfaceID)
		n++
	}
	if filter.DateFrom != nil {
		where += fmt.Sprintf(" AND created_at >= $%d", n)
		args = append(args, *filter.DateFrom)
		n++
	}
	if filter.DateTo != nil {
		where += fmt.Sprintf(" AND created_at <= $%d", n)
		args = append(args, *filter.DateTo)
		n++
	}
	if filter.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", n)
		args = append(args, filter.Status)
		n++
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cda_documents "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("cda_storage/postgres: list count: %w", err)
	}

	listSQL := fmt.Sprintf(`
		SELECT id,
		       COALESCE(interface_id::text, ''),
		       COALESCE(pipeline_execution_id::text, ''),
		       COALESCE(message_id, ''),
		       COALESCE(document_id, ''),
		       COALESCE(document_type, ''),
		       status,
		       COALESCE(patient_id, ''),
		       COALESCE(patient_name, ''),
		       document_date,
		       COALESCE(author_name, ''),
		       COALESCE(custodian_name, ''),
		       COALESCE(section_count, 0),
		       COALESCE(field_count, 0),
		       COALESCE(parse_duration_ms, 0),
		       COALESCE(raw_xml_size_bytes, 0),
		       storage_mode,
		       COALESCE(compliance_score, 0),
		       COALESCE(error_message, ''),
		       created_at,
		       updated_at
		FROM cda_documents %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, n, n+1)

	args = append(args, filter.Limit, offset)
	rows, err := s.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("cda_storage/postgres: list query: %w", err)
	}
	defer rows.Close()

	var docs []*StoredDocument
	for rows.Next() {
		var doc StoredDocument
		var docDate sql.NullTime
		if err := rows.Scan(
			&doc.ID, &doc.InterfaceID, &doc.PipelineExecutionID,
			&doc.MessageID, &doc.DocumentID, &doc.DocumentType,
			&doc.Status,
			&doc.PatientID, &doc.PatientName, &docDate,
			&doc.AuthorName, &doc.CustodianName,
			&doc.SectionCount, &doc.FieldCount,
			&doc.ParseDurationMs, &doc.RawXMLSizeBytes,
			&doc.StorageMode, &doc.ComplianceScore,
			&doc.ErrorMessage, &doc.CreatedAt, &doc.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("cda_storage/postgres: scan row: %w", err)
		}
		if docDate.Valid {
			doc.DocumentDate = &docDate.Time
		}
		docs = append(docs, &doc)
	}
	return docs, total, rows.Err()
}

func (s *PostgresDocumentStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE cda_documents SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("cda_storage/postgres: delete %q: %w", id, err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("cda_storage: document %q not found", id)
	}
	return nil
}

// getStorageMode returns the storage_mode for a document.
func (s *PostgresDocumentStore) getStorageMode(ctx context.Context, id string) (string, error) {
	var mode string
	err := s.db.QueryRowContext(ctx,
		`SELECT storage_mode FROM cda_documents WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&mode)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("cda_storage: document %q not found", id)
	}
	return mode, err
}

// ─── Private helpers ─────────────────────────────────────────────────────────

func (s *PostgresDocumentStore) buildStoredDocFromMeta(meta documentMeta, input *SaveInput) *StoredDocument {
	return &StoredDocument{
		InterfaceID:         input.InterfaceID,
		PipelineExecutionID: input.PipelineExecutionID,
		MessageID:           input.MessageID,
		DocumentID:          meta.DocumentID,
		DocumentType:        meta.DocumentType,
		Status:              "parsed",
		PatientID:           meta.PatientID,
		PatientName:         meta.PatientName,
		DocumentDate:        meta.DocumentDate,
		AuthorName:          meta.AuthorName,
		CustodianName:       meta.CustodianName,
		SectionCount:        input.SectionCount,
		FieldCount:          input.FieldCount,
		ParseDurationMs:     int(input.ParseDurationMs),
		RawXMLSizeBytes:     len(input.RawXML),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}
}

// Compile-time check: PostgresDocumentStore satisfies CDADocumentStore.
var _ CDADocumentStore = (*PostgresDocumentStore)(nil)
