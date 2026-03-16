// services/storage/driver.go
// ObjectStorageDriver — abstract interface for S3/MinIO, Azure Blob, GCS, or local filesystem.
// All high-level storage operations in the platform go through this interface so that
// the underlying store can be swapped without touching business logic.

package storage

import (
	"context"
	"io"
	"time"
)

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	ContentType  string
	ETag         string
}

// ObjectStorageDriver is the low-level interface that every storage backend must implement.
type ObjectStorageDriver interface {
	// PutObject writes data to bucket/key, returning the storage URI (e.g. s3://bucket/key).
	PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) (uri string, err error)

	// GetObject returns a ReadCloser for the object at bucket/key.
	// Caller is responsible for closing the reader.
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, *ObjectInfo, error)

	// GetObjectBytes is a convenience wrapper that reads the full body.
	GetObjectBytes(ctx context.Context, bucket, key string) ([]byte, *ObjectInfo, error)

	// DeleteObject removes bucket/key.
	DeleteObject(ctx context.Context, bucket, key string) error

	// ListObjects lists all objects with the given prefix.
	ListObjects(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error)

	// AppendObject appends data to an existing object (by download-concat-upload).
	// Used for log files.  Returns the updated URI.
	AppendObject(ctx context.Context, bucket, key string, data []byte) (uri string, err error)

	// EnsureBucket creates the bucket if it does not exist.
	EnsureBucket(ctx context.Context, bucket string) error

	// DriverName returns a human-readable name for the driver (e.g. "s3", "local").
	DriverName() string
}
