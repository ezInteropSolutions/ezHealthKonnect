package cdastorage

import (
	"database/sql"
	"log"

	"ezhealthkonnect/services/storage"
)

// NewCDADocumentStore constructs the appropriate CDADocumentStore implementation.
//
//   - If driver is non-nil: returns a RoutingDocumentStore that stores documents
//     ≤ 1 MB inline in PostgreSQL and larger documents in MinIO/S3.
//   - If driver is nil: returns a PostgresDocumentStore that always stores inline.
//
// Both variants write metadata to the cda_documents table via db.
func NewCDADocumentStore(db *sql.DB, driver storage.ObjectStorageDriver, bucket string) CDADocumentStore {
	pg := NewPostgresDocumentStore(db)

	if driver == nil {
		log.Printf("📄 [CDADocumentStore] Initialised (mode: postgres-inline — no object storage driver)")
		return pg
	}

	if bucket == "" {
		bucket = storage.DefaultBucketName()
	}

	log.Printf("📄 [CDADocumentStore] Initialised (mode: routing — inline ≤1 MB, object storage via %s bucket=%s)",
		driver.DriverName(), bucket)
	return NewRoutingDocumentStore(pg, driver, bucket)
}
