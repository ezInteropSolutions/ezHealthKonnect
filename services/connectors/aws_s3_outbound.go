// services/connectors/aws_s3_outbound.go
// AWS S3 Outbound Connector — uploads a message's content as an object to an
// S3 bucket. Object keys are built from a template using the same
// placeholder convention as file_writer.go's filename_pattern, so switching
// between "write to disk" and "write to S3" doesn't require learning a new
// naming scheme.
//
// Configuration:
//
//	bucket                 string  S3 bucket name (required)
//	region                 string  AWS region, e.g. us-east-1 (required)
//	key_pattern            string  Object key template (default: "{message_id}.json").
//	                               Placeholders: {message_id} {interface_id} {timestamp} {date} {time}
//	access_key_id          string  AWS access key (omit to use IAM role / env)
//	secret_access_key      string  AWS secret key
//	session_token          string  AWS session token (for temporary creds)
//	endpoint               string  Custom endpoint (e.g. for LocalStack/MinIO)
//	force_path_style       bool    Use path-style S3 URLs (needed for LocalStack/MinIO)
//	content_type           string  Override Content-Type (default: inferred from message)
//	server_side_encryption string  "AES256" | "aws:kms" | "none" (default: "AES256")
//	kms_key_id             string  KMS key ID/ARN — required when server_side_encryption is "aws:kms"
package connectors

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// AWSS3OutboundConfig holds AWS S3 outbound connector configuration.
type AWSS3OutboundConfig struct {
	Bucket               string `json:"bucket"`
	Region               string `json:"region"`
	KeyPattern           string `json:"key_pattern"`
	AccessKeyID          string `json:"access_key_id"`
	SecretAccessKey      string `json:"secret_access_key"`
	SessionToken         string `json:"session_token"`
	Endpoint             string `json:"endpoint"`
	ForcePathStyle       bool   `json:"force_path_style"`
	ContentType          string `json:"content_type"`
	ServerSideEncryption string `json:"server_side_encryption"`
	KMSKeyID             string `json:"kms_key_id"`
}

// AWSS3OutboundConnector uploads messages as objects to an S3 bucket.
type AWSS3OutboundConnector struct {
	*BaseOutboundConnector
	config   AWSS3OutboundConfig
	s3Client *s3.Client
	mu       sync.Mutex
}

// NewAWSS3OutboundConnector creates a production AWS S3 outbound connector.
func NewAWSS3OutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "aws_s3_outbound",
		DisplayName:        "AWS S3 Bucket Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":      true,
			"supports_iam_role":   true,
			"supports_encryption": true,
			"supports_kms":        true,
			"supports_templates":  true,
		},
	}
	return &AWSS3OutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, true),
	}
}

// Initialize validates config and creates the S3 client.
func (c *AWSS3OutboundConnector) Initialize(configBytes []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := parseJSON(configBytes, &c.config); err != nil {
		return fmt.Errorf("failed to parse S3 outbound config: %w", err)
	}

	if c.config.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	if c.config.Region == "" {
		return fmt.Errorf("region is required")
	}

	if c.config.KeyPattern == "" {
		c.config.KeyPattern = "{message_id}.json"
	}
	if c.config.ServerSideEncryption == "" {
		c.config.ServerSideEncryption = "AES256"
	}
	if c.config.ServerSideEncryption == "aws:kms" && c.config.KMSKeyID == "" {
		return fmt.Errorf("kms_key_id is required when server_side_encryption is \"aws:kms\"")
	}

	awsCfg, err := c.buildAWSConfig()
	if err != nil {
		return fmt.Errorf("failed to build AWS config: %w", err)
	}

	c.s3Client = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if c.config.Endpoint != "" {
			o.BaseEndpoint = aws.String(c.config.Endpoint)
		}
		if c.config.ForcePathStyle {
			o.UsePathStyle = true
		}
	})

	c.BaseOutboundConnector.BaseConnector.initialized = true

	log.Printf("✅ AWS S3 Outbound: Initialized (bucket=%s, region=%s, key_pattern=%q, sse=%s)",
		c.config.Bucket, c.config.Region, c.config.KeyPattern, c.config.ServerSideEncryption)
	return nil
}

// buildAWSConfig constructs the aws.Config with explicit or default credentials —
// same static-credentials-else-default-chain approach as aws_s3_inbound.go.
func (c *AWSS3OutboundConnector) buildAWSConfig() (aws.Config, error) {
	ctx := context.Background()
	if c.config.AccessKeyID != "" && c.config.SecretAccessKey != "" {
		return config.LoadDefaultConfig(ctx,
			config.WithRegion(c.config.Region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				c.config.AccessKeyID,
				c.config.SecretAccessKey,
				c.config.SessionToken,
			)),
		)
	}
	return config.LoadDefaultConfig(ctx, config.WithRegion(c.config.Region))
}

// buildKey renders the key_pattern template for one message — same placeholder
// set as file_writer.go's generateFilename, so the naming convention is
// familiar whether the destination is disk or S3.
func (c *AWSS3OutboundConnector) buildKey(message *models.OutboundMessage) string {
	key := c.config.KeyPattern
	now := time.Now()

	replacements := map[string]string{
		"{timestamp}":    now.Format("20060102_150405"),
		"{date}":         now.Format("20060102"),
		"{time}":         now.Format("150405"),
		"{message_id}":   sanitizeObjectKeySegment(message.MessageID),
		"{interface_id}": sanitizeObjectKeySegment(message.InterfaceID),
	}
	for placeholder, value := range replacements {
		key = strings.ReplaceAll(key, placeholder, value)
	}

	if filepath.Ext(key) == "" {
		key += extensionForContentType(contentTypeOrDefault(c.config.ContentType, message.ContentType))
	}
	return key
}

// sanitizeObjectKeySegment removes characters that are awkward in an object
// key/blob name segment (most cloud object stores allow wide UTF-8, but
// slashes would silently create unintended "subdirectories" and other
// characters cause friction with some tooling/URLs). Shared by every
// cloud-object-storage outbound connector (S3, Azure Blob, ...) rather than
// duplicated per connector.
func sanitizeObjectKeySegment(s string) string {
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := s
	for _, ch := range invalid {
		result = strings.ReplaceAll(result, ch, "_")
	}
	return result
}

func contentTypeOrDefault(configured, fromMessage string) string {
	if configured != "" {
		return configured
	}
	return fromMessage
}

// extensionForContentType maps a MIME type to a file extension for keys whose
// pattern doesn't already specify one.
func extensionForContentType(contentType string) string {
	switch {
	case strings.Contains(contentType, "hl7"):
		return ".hl7"
	case strings.Contains(contentType, "json") || strings.Contains(contentType, "fhir"):
		return ".json"
	case strings.Contains(contentType, "xml"):
		return ".xml"
	case strings.Contains(contentType, "csv"):
		return ".csv"
	default:
		return ".txt"
	}
}

// putObjectEncryption applies the configured server-side encryption settings
// to a PutObjectInput.
func (c *AWSS3OutboundConnector) applyEncryption(input *s3.PutObjectInput) {
	switch c.config.ServerSideEncryption {
	case "AES256":
		input.ServerSideEncryption = types.ServerSideEncryptionAes256
	case "aws:kms":
		input.ServerSideEncryption = types.ServerSideEncryptionAwsKms
		input.SSEKMSKeyId = aws.String(c.config.KMSKeyID)
	case "none":
		// Leave unset — bucket default encryption (if any) still applies.
	}
}

// Send uploads a single message as an S3 object.
func (c *AWSS3OutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	c.mu.Lock()
	client := c.s3Client
	c.mu.Unlock()
	if client == nil {
		return nil, fmt.Errorf("connector not initialized")
	}

	startTime := time.Now()
	key := c.buildKey(message)
	contentType := contentTypeOrDefault(c.config.ContentType, message.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	input := &s3.PutObjectInput{
		Bucket:      aws.String(c.config.Bucket),
		Key:         aws.String(key),
		Body:        strings.NewReader(message.Content),
		ContentType: aws.String(contentType),
	}
	c.applyEncryption(input)

	log.Printf("☁️  AWS S3 Outbound: Uploading s3://%s/%s (%d bytes)", c.config.Bucket, key, len(message.Content))

	resp, err := client.PutObject(ctx, input)
	if err != nil {
		return &DeliveryResult{
			Success:      false,
			MessageID:    message.MessageID,
			Timestamp:    time.Now(),
			ErrorMessage: fmt.Sprintf("PutObject failed: %v", err),
			DurationMs:   int64(time.Since(startTime).Milliseconds()),
		}, err
	}

	return &DeliveryResult{
		Success:        true,
		MessageID:      message.MessageID,
		Timestamp:      time.Now(),
		Acknowledgment: fmt.Sprintf("s3://%s/%s (ETag: %s)", c.config.Bucket, key, aws.ToString(resp.ETag)),
		DurationMs:     int64(time.Since(startTime).Milliseconds()),
		Metadata: map[string]interface{}{
			"bucket": c.config.Bucket,
			"key":    key,
			"etag":   aws.ToString(resp.ETag),
		},
	}, nil
}

// SendBatch uploads each message as its own S3 object — S3 has no native
// batch-put API, so this aggregates individual PutObject results the same
// way SendBatch works for every other connector in this package.
func (c *AWSS3OutboundConnector) SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error) {
	results := make([]*DeliveryResult, 0, len(messages))
	successCount, failureCount := 0, 0

	for _, message := range messages {
		result, err := c.Send(ctx, message)
		if err != nil && result == nil {
			result = &DeliveryResult{
				Success:      false,
				MessageID:    message.MessageID,
				Timestamp:    time.Now(),
				ErrorMessage: err.Error(),
			}
		}
		results = append(results, result)
		if result.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	log.Printf("✅ AWS S3 Outbound: Batch complete — success: %d, failed: %d", successCount, failureCount)
	return results, nil
}

// SupportsBatch returns true.
func (c *AWSS3OutboundConnector) SupportsBatch() bool { return true }

// TestConnection calls HeadBucket to verify bucket access without writing anything.
func (c *AWSS3OutboundConnector) TestConnection(ctx context.Context) error {
	c.mu.Lock()
	client := c.s3Client
	c.mu.Unlock()
	if client == nil {
		return fmt.Errorf("not initialized")
	}
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(c.config.Bucket),
	})
	if err != nil {
		return fmt.Errorf("S3 HeadBucket failed for %s: %w", c.config.Bucket, err)
	}
	return nil
}

// Validate checks required fields after initialization.
func (c *AWSS3OutboundConnector) Validate() error {
	if !c.BaseOutboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if c.config.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	if c.config.Region == "" {
		return fmt.Errorf("region is required")
	}
	return nil
}

// Close releases connector resources. The AWS SDK client has no explicit
// close/shutdown method — nothing to release beyond dropping the reference.
func (c *AWSS3OutboundConnector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.s3Client = nil
	return nil
}

// GetStatus returns connector status with S3-specific metadata.
func (c *AWSS3OutboundConnector) GetStatus() ConnectorStatus {
	status := c.BaseOutboundConnector.GetStatus()
	if status.Metadata == nil {
		status.Metadata = map[string]string{}
	}
	status.Metadata["bucket"] = c.config.Bucket
	status.Metadata["region"] = c.config.Region
	status.Metadata["key_pattern"] = c.config.KeyPattern
	return status
}
