// services/storage/factory.go
// Factory — creates the correct ObjectStorageDriver based on environment variables.
//
// Environment variables:
//   OBJECT_STORAGE_DRIVER   "s3" (default) | "local"
//   OBJECT_STORAGE_BUCKET   Bucket/container name (default: "ezhealthkonnect")
//   OBJECT_STORAGE_REGION   AWS region (default: "us-east-1")
//   OBJECT_STORAGE_ENDPOINT Custom endpoint URL (MinIO: http://minio:9000, LocalStack: http://localstack:4566)
//   OBJECT_STORAGE_PATH_STYLE "true" to enable path-style URLs (required for MinIO)
//   AWS_ACCESS_KEY_ID       AWS / MinIO access key
//   AWS_SECRET_ACCESS_KEY   AWS / MinIO secret key
//   LOCAL_STORAGE_PATH      Base directory for local driver (default: ./data/storage)

package storage

import (
	"fmt"
	"log"
	"os"
	"strings"
)

const (
	DriverS3    = "s3"
	DriverLocal = "local"

	DefaultBucket    = "ezhealthkonnect"
	DefaultRegion    = "us-east-1"
	DefaultLocalPath = "./data/storage"
)

// NewDriverFromEnv reads environment variables and returns the appropriate driver.
// Call this once at startup and inject the returned driver (or the ObjectStorageService
// built on top of it) wherever storage is needed.
func NewDriverFromEnv() (ObjectStorageDriver, error) {
	driverName := strings.ToLower(os.Getenv("OBJECT_STORAGE_DRIVER"))
	if driverName == "" {
		driverName = DriverS3
	}

	switch driverName {
	case DriverS3:
		return newS3DriverFromEnv()
	case DriverLocal:
		return newLocalDriverFromEnv()
	default:
		return nil, fmt.Errorf("storage/factory: unknown driver %q (supported: s3, local)", driverName)
	}
}

func newS3DriverFromEnv() (*S3Driver, error) {
	region := os.Getenv("OBJECT_STORAGE_REGION")
	if region == "" {
		region = DefaultRegion
	}

	bucket := os.Getenv("OBJECT_STORAGE_BUCKET")
	if bucket == "" {
		bucket = DefaultBucket
	}

	endpoint := os.Getenv("OBJECT_STORAGE_ENDPOINT")
	pathStyle := strings.EqualFold(os.Getenv("OBJECT_STORAGE_PATH_STYLE"), "true")

	// Support both OBJECT_STORAGE_* and standard AWS_ env var names
	accessKey := os.Getenv("OBJECT_STORAGE_ACCESS_KEY")
	if accessKey == "" {
		accessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	}
	secretKey := os.Getenv("OBJECT_STORAGE_SECRET_KEY")
	if secretKey == "" {
		secretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")

	cfg := S3DriverConfig{
		Region:          region,
		Bucket:          bucket,
		Endpoint:        endpoint,
		ForcePathStyle:  pathStyle,
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
		SessionToken:    sessionToken,
	}

	log.Printf("🔧 [ObjectStorage] Using S3 driver (region=%s, endpoint=%q, pathStyle=%v)",
		region, endpoint, pathStyle)
	return NewS3Driver(cfg)
}

func newLocalDriverFromEnv() (*LocalDriver, error) {
	basePath := os.Getenv("LOCAL_STORAGE_PATH")
	if basePath == "" {
		basePath = DefaultLocalPath
	}
	log.Printf("🔧 [ObjectStorage] Using local driver (path=%s)", basePath)
	return NewLocalDriver(basePath)
}

// DefaultBucketName returns the configured default bucket name.
func DefaultBucketName() string {
	if b := os.Getenv("OBJECT_STORAGE_BUCKET"); b != "" {
		return b
	}
	return DefaultBucket
}
