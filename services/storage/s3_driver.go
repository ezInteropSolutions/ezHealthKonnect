// services/storage/s3_driver.go
// S3Driver — ObjectStorageDriver backed by AWS S3 or any S3-compatible store (MinIO, LocalStack).
// Uses the same aws-sdk-go-v2 pattern as services/connectors/aws_s3_inbound.go.

package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3DriverConfig holds credentials and endpoint configuration.
type S3DriverConfig struct {
	Region          string // AWS region, e.g. "us-east-1"
	Bucket          string // Default bucket (used by high-level service)
	AccessKeyID     string // Leave empty for IAM role / env credential chain
	SecretAccessKey string
	SessionToken    string
	Endpoint        string // Custom endpoint — set to MinIO URL in development
	ForcePathStyle  bool   // Must be true for MinIO
}

// S3Driver implements ObjectStorageDriver using AWS SDK v2.
type S3Driver struct {
	cfg    S3DriverConfig
	client *s3.Client
}

// NewS3Driver creates and initialises an S3Driver from config.
func NewS3Driver(cfg S3DriverConfig) (*S3Driver, error) {
	d := &S3Driver{cfg: cfg}

	var loadOpts []func(*awsconfig.LoadOptions) error
	if cfg.Region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(cfg.Region))
	}
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		loadOpts = append(loadOpts,
			awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(
					cfg.AccessKeyID,
					cfg.SecretAccessKey,
					cfg.SessionToken,
				),
			),
		)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("storage/s3: load AWS config: %w", err)
	}

	d.client = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		if cfg.ForcePathStyle {
			o.UsePathStyle = true
		}
	})

	log.Printf("✅ [ObjectStorage] S3 driver initialised (region=%s, endpoint=%q, bucket=%s)",
		cfg.Region, cfg.Endpoint, cfg.Bucket)
	return d, nil
}

// DriverName returns "s3".
func (d *S3Driver) DriverName() string { return "s3" }

// Ping verifies the S3/MinIO endpoint is reachable and the bucket exists.
// Uses HeadBucket — a lightweight metadata-only call that does not read data.
func (d *S3Driver) Ping(ctx context.Context) error {
	_, err := d.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(d.cfg.Bucket)})
	if err != nil {
		return fmt.Errorf("storage/s3: bucket unreachable: %w", err)
	}
	return nil
}

// EnsureBucket creates the bucket if it does not already exist.
func (d *S3Driver) EnsureBucket(ctx context.Context, bucket string) error {
	_, err := d.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err == nil {
		return nil // Already exists
	}

	_, err = d.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		return fmt.Errorf("storage/s3: create bucket %q: %w", bucket, err)
	}
	log.Printf("📦 [ObjectStorage] Created bucket %q", bucket)
	return nil
}

// PutObject writes body to s3://bucket/key and returns the URI.
func (d *S3Driver) PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) (string, error) {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	}
	if size >= 0 {
		input.ContentLength = aws.Int64(size)
	}

	_, err := d.client.PutObject(ctx, input)
	if err != nil {
		return "", fmt.Errorf("storage/s3: put s3://%s/%s: %w", bucket, key, err)
	}

	uri := fmt.Sprintf("s3://%s/%s", bucket, key)
	return uri, nil
}

// GetObject returns a ReadCloser for bucket/key.
func (d *S3Driver) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, *ObjectInfo, error) {
	out, err := d.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("storage/s3: get s3://%s/%s: %w", bucket, key, err)
	}

	info := &ObjectInfo{
		Key:         key,
		ContentType: aws.ToString(out.ContentType),
	}
	if out.ContentLength != nil {
		info.Size = *out.ContentLength
	}
	if out.LastModified != nil {
		info.LastModified = *out.LastModified
	}
	if out.ETag != nil {
		info.ETag = *out.ETag
	}

	return out.Body, info, nil
}

// GetObjectBytes reads the full body of bucket/key into memory.
func (d *S3Driver) GetObjectBytes(ctx context.Context, bucket, key string) ([]byte, *ObjectInfo, error) {
	rc, info, err := d.GetObject(ctx, bucket, key)
	if err != nil {
		return nil, nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, nil, fmt.Errorf("storage/s3: read body s3://%s/%s: %w", bucket, key, err)
	}
	return data, info, nil
}

// DeleteObject removes bucket/key.
func (d *S3Driver) DeleteObject(ctx context.Context, bucket, key string) error {
	_, err := d.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("storage/s3: delete s3://%s/%s: %w", bucket, key, err)
	}
	return nil
}

// ListObjects returns all objects with the given prefix.
func (d *S3Driver) ListObjects(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error) {
	var result []ObjectInfo
	paginator := s3.NewListObjectsV2Paginator(d.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("storage/s3: list s3://%s/%s: %w", bucket, prefix, err)
		}
		for _, obj := range page.Contents {
			info := ObjectInfo{
				Key:  aws.ToString(obj.Key),
				Size: aws.ToInt64(obj.Size),
			}
			if obj.LastModified != nil {
				info.LastModified = *obj.LastModified
			}
			if obj.ETag != nil {
				info.ETag = *obj.ETag
			}
			result = append(result, info)
		}
	}
	return result, nil
}

// Compile-time check: S3Driver satisfies ObjectStorageDriver
var _ ObjectStorageDriver = (*S3Driver)(nil)

// AppendObject implements append-by-download-concat-upload for log files.
// For large log files consider switching to S3 multipart upload in the future.
func (d *S3Driver) AppendObject(ctx context.Context, bucket, key string, data []byte) (string, error) {
	// Try to read existing content
	existing, _, err := d.GetObjectBytes(ctx, bucket, key)
	if err != nil {
		// Object doesn't exist yet — treat as empty
		existing = nil
	}

	merged := append(existing, data...)
	return d.PutObject(ctx, bucket, key, bytes.NewReader(merged), int64(len(merged)), "application/x-ndjson")
}
