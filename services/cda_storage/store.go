// Package cdastorage provides durable persistence for parsed CDA documents.
// It implements a hybrid routing strategy: documents ≤ 1 MB are stored inline
// in PostgreSQL JSONB; larger documents are stored in MinIO/S3 with metadata
// and object URIs recorded in PostgreSQL.
//
// Usage:
//
//	store, err := cdastorage.NewCDADocumentStore(db, objStorageSvc)
//	stored, err := store.Save(ctx, &cdastorage.SaveInput{...})
package cdastorage

import (
	"context"
	"time"
)

const (
	StorageModeInline = "inline"
	StorageModeObject = "object"

	// InlineSizeThreshold is the maximum raw XML size (bytes) stored inline in
	// PostgreSQL. Documents exceeding this are routed to MinIO/S3.
	InlineSizeThreshold = 1 * 1024 * 1024 // 1 MB
)

// SaveInput encapsulates all data produced by a cda.parse pipeline step.
// TypedDocument is interface{} to avoid importing cda/document in callers
// (e.g. cda_parse_executor.go); the store asserts it to *cdadocument.CDADocument.
type SaveInput struct {
	InterfaceID         string
	PipelineExecutionID string
	MessageID           string
	RawXML              string
	ParsedJSON          map[string]interface{}
	TypedDocument       interface{} // *cdadocument.CDADocument; nil = skip meta extraction
	ParseDurationMs     int64
	SectionCount        int
	FieldCount          int
	ComplianceReport    interface{} // *validator.ComplianceReport; nil = no compliance data
	FHIRBundle          map[string]interface{}
}

// StoredDocument is the metadata record returned by Save and Get.
// It never contains raw XML or full parsed JSON to keep response payloads small;
// callers use GetRawXML / GetParsedJSON for the content.
type StoredDocument struct {
	ID                  string     `json:"id"`
	InterfaceID         string     `json:"interfaceId,omitempty"`
	PipelineExecutionID string     `json:"pipelineExecutionId,omitempty"`
	MessageID           string     `json:"messageId,omitempty"`
	DocumentID          string     `json:"documentId,omitempty"`
	DocumentType        string     `json:"documentType"`
	Status              string     `json:"status"`
	PatientID           string     `json:"patientId,omitempty"`
	PatientName         string     `json:"patientName,omitempty"`
	DocumentDate        *time.Time `json:"documentDate,omitempty"`
	AuthorName          string     `json:"authorName,omitempty"`
	CustodianName       string     `json:"custodianName,omitempty"`
	SectionCount        int        `json:"sectionCount"`
	FieldCount          int        `json:"fieldCount"`
	ParseDurationMs     int        `json:"parseDurationMs"`
	RawXMLSizeBytes     int        `json:"rawXmlSizeBytes"`
	StorageMode         string     `json:"storageMode"`
	ComplianceScore     float64    `json:"complianceScore,omitempty"`
	ErrorMessage        string     `json:"errorMessage,omitempty"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

// ListFilter supports pagination and field-based filtering for the list endpoint.
type ListFilter struct {
	InterfaceID string
	DateFrom    *time.Time
	DateTo      *time.Time
	Status      string
	Page        int // 1-based
	Limit       int // max rows; defaults to 20
}

// CDADocumentStore persists parsed CDA documents and provides content retrieval.
// The zero value is not useful; obtain via NewCDADocumentStore or NewPostgresDocumentStore.
type CDADocumentStore interface {
	// Save persists a parsed CDA document and returns its metadata record.
	// Implementations route between inline PostgreSQL and object storage based on document size.
	Save(ctx context.Context, input *SaveInput) (*StoredDocument, error)

	// Get retrieves document metadata by ID. Returns an error if not found or soft-deleted.
	Get(ctx context.Context, id string) (*StoredDocument, error)

	// GetRawXML retrieves the original CDA XML string for a document.
	// For object-stored documents this fetches from MinIO/S3.
	GetRawXML(ctx context.Context, id string) (string, error)

	// GetParsedJSON retrieves the parsed JSON map for a document.
	// For object-stored documents this fetches from MinIO/S3.
	GetParsedJSON(ctx context.Context, id string) (map[string]interface{}, error)

	// List returns a paginated slice of stored document metadata and the total count.
	List(ctx context.Context, filter ListFilter) ([]*StoredDocument, int, error)

	// Delete soft-deletes a document by setting deleted_at. Returns an error if not found.
	Delete(ctx context.Context, id string) error
}
