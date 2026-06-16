package cdastorage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"ezhealthkonnect/services/storage"
)

// RoutingDocumentStore routes CDA document persistence based on document size:
//   - Raw XML ≤ InlineSizeThreshold (1 MB): delegates to PostgresDocumentStore (inline JSONB)
//   - Raw XML > InlineSizeThreshold       : uploads raw XML + parsed JSON to MinIO/S3 via
//     ObjectStorageDriver, then stores metadata + URIs in PostgreSQL
//
// If driver is nil, all documents fall back to inline PostgreSQL storage.
type RoutingDocumentStore struct {
	pg     *PostgresDocumentStore
	driver storage.ObjectStorageDriver
	bucket string
}

// NewRoutingDocumentStore constructs the hybrid store.
// driver may be nil — in that case all documents are stored inline in PostgreSQL.
func NewRoutingDocumentStore(pg *PostgresDocumentStore, driver storage.ObjectStorageDriver, bucket string) *RoutingDocumentStore {
	return &RoutingDocumentStore{pg: pg, driver: driver, bucket: bucket}
}

func (s *RoutingDocumentStore) Save(ctx context.Context, input *SaveInput) (*StoredDocument, error) {
	if s.driver == nil || len(input.RawXML) <= InlineSizeThreshold {
		return s.pg.Save(ctx, input)
	}
	return s.saveToObjectStorage(ctx, input)
}

func (s *RoutingDocumentStore) Get(ctx context.Context, id string) (*StoredDocument, error) {
	return s.pg.Get(ctx, id)
}

func (s *RoutingDocumentStore) GetRawXML(ctx context.Context, id string) (string, error) {
	mode, err := s.pg.getStorageMode(ctx, id)
	if err != nil {
		return "", err
	}
	if mode == StorageModeInline {
		return s.pg.GetRawXML(ctx, id)
	}
	if s.driver == nil {
		return "", fmt.Errorf("cda_storage: document %q is object-stored but object storage is unavailable", id)
	}
	uri, err := s.pg.getRawXMLURI(ctx, id)
	if err != nil {
		return "", err
	}
	bucket, key, err := parseObjectURI(uri)
	if err != nil {
		return "", err
	}
	data, _, err := s.driver.GetObjectBytes(ctx, bucket, key)
	if err != nil {
		return "", fmt.Errorf("cda_storage/routing: fetch raw XML: %w", err)
	}
	return string(data), nil
}

func (s *RoutingDocumentStore) GetParsedJSON(ctx context.Context, id string) (map[string]interface{}, error) {
	mode, err := s.pg.getStorageMode(ctx, id)
	if err != nil {
		return nil, err
	}
	if mode == StorageModeInline {
		return s.pg.GetParsedJSON(ctx, id)
	}
	if s.driver == nil {
		return nil, fmt.Errorf("cda_storage: document %q is object-stored but object storage is unavailable", id)
	}
	uri, err := s.pg.getParsedJSONURI(ctx, id)
	if err != nil {
		return nil, err
	}
	bucket, key, err := parseObjectURI(uri)
	if err != nil {
		return nil, err
	}
	data, _, err := s.driver.GetObjectBytes(ctx, bucket, key)
	if err != nil {
		return nil, fmt.Errorf("cda_storage/routing: fetch parsed JSON: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("cda_storage/routing: unmarshal parsed JSON: %w", err)
	}
	return result, nil
}

func (s *RoutingDocumentStore) List(ctx context.Context, filter ListFilter) ([]*StoredDocument, int, error) {
	return s.pg.List(ctx, filter)
}

func (s *RoutingDocumentStore) Delete(ctx context.Context, id string) error {
	return s.pg.Delete(ctx, id)
}

// ─── Object storage upload ───────────────────────────────────────────────────

// saveToObjectStorage uploads raw XML + parsed JSON to MinIO/S3, then records
// metadata + URIs in PostgreSQL. Falls back to inline storage on any upload error.
func (s *RoutingDocumentStore) saveToObjectStorage(ctx context.Context, input *SaveInput) (*StoredDocument, error) {
	t := time.Now().UTC()
	prefix := fmt.Sprintf("cda-documents/%04d/%02d/%02d/%d", t.Year(), t.Month(), t.Day(), t.UnixNano())

	rawBytes := []byte(input.RawXML)
	rawKey := prefix + ".xml"
	rawURI, err := s.driver.PutObject(ctx, s.bucket, rawKey,
		bytes.NewReader(rawBytes), int64(len(rawBytes)), "application/xml")
	if err != nil {
		log.Printf("⚠️  [cda_storage] Object upload failed (raw XML) — falling back to inline: %v", err)
		return s.pg.Save(ctx, input)
	}

	parsedBytes, _ := json.Marshal(input.ParsedJSON)
	parsedKey := prefix + ".json"
	parsedURI, err := s.driver.PutObject(ctx, s.bucket, parsedKey,
		bytes.NewReader(parsedBytes), int64(len(parsedBytes)), "application/json")
	if err != nil {
		log.Printf("⚠️  [cda_storage] Object upload failed (parsed JSON) — falling back to inline: %v", err)
		return s.pg.Save(ctx, input)
	}

	return s.pg.SaveWithObjectURIs(ctx, input, rawURI, parsedURI)
}

// parseObjectURI extracts bucket and key from s3://bucket/key or local://bucket/key.
func parseObjectURI(uri string) (bucket, key string, err error) {
	for _, pfx := range []string{"s3://", "local://"} {
		if strings.HasPrefix(uri, pfx) {
			rest := uri[len(pfx):]
			idx := strings.IndexByte(rest, '/')
			if idx < 0 {
				return "", "", fmt.Errorf("cda_storage: malformed object URI %q", uri)
			}
			return rest[:idx], rest[idx+1:], nil
		}
	}
	return "", "", fmt.Errorf("cda_storage: unrecognised URI scheme in %q", uri)
}

// Compile-time check.
var _ CDADocumentStore = (*RoutingDocumentStore)(nil)
