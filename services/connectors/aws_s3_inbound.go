// services/connectors/aws_s3_inbound.go
// AWS S3 Inbound Connector — polls an S3 bucket for new objects, downloads them, and
// converts each file into an InboundMessage for the pipeline.
//
// Polling strategy: ListObjectsV2 with a configurable prefix.  State is tracked via
// the last-modified timestamp of the most recently processed object.  Objects are
// sorted by LastModified so incremental delivery is consistent.
//
// After-processing options:
//   - nothing  (default): leave the file in the bucket
//   - delete:             DeleteObject after successful ingestion
//   - archive:            CopyObject to archive_prefix then DeleteObject
//
// Configuration:
//
//	bucket               string  S3 bucket name (required)
//	region               string  AWS region, e.g. us-east-1 (required)
//	prefix               string  Key prefix to filter (e.g. "inbound/")
//	file_pattern         string  Glob suffix filter (e.g. "*.hl7")  — matched against key suffix
//	access_key_id        string  AWS access key (omit to use IAM role / env)
//	secret_access_key    string  AWS secret key
//	session_token        string  AWS session token (for temporary creds)
//	endpoint             string  Custom endpoint (e.g. for LocalStack)
//	force_path_style     bool    Use path-style S3 URLs (needed for LocalStack)
//	after_processing     string  "nothing" | "delete" | "archive"
//	archive_prefix       string  Prefix for archived objects (e.g. "processed/")
//	polling_interval     int     Seconds between polls (default: 60)
//	max_keys             int     Max objects per ListObjectsV2 page (default: 100)
//	max_file_size_mb     int     Skip files larger than this (default: 50)

package connectors

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"sort"
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

const defaultMaxFileSizeMB = 50

// AWSS3InboundConfig holds AWS S3 inbound connector configuration.
type AWSS3InboundConfig struct {
	Bucket            string `json:"bucket"`
	Region            string `json:"region"`
	Prefix            string `json:"prefix"`
	FilePattern       string `json:"file_pattern"`
	AccessKeyID       string `json:"access_key_id"`
	SecretAccessKey   string `json:"secret_access_key"`
	SessionToken      string `json:"session_token"`
	Endpoint          string `json:"endpoint"`
	ForcePathStyle    bool   `json:"force_path_style"`
	AfterProcessing   string `json:"after_processing"`
	ArchivePrefix     string `json:"archive_prefix"`
	PollingInterval   int    `json:"polling_interval"`
	MaxKeys           int    `json:"max_keys"`
	MaxFileSizeMB     int    `json:"max_file_size_mb"`
}

// AWSS3InboundConnector polls an S3 bucket for new HL7/FHIR/JSON files.
type AWSS3InboundConnector struct {
	*BaseInboundConnector
	config        AWSS3InboundConfig
	s3Client      *s3.Client
	lastProcessed time.Time // most recent LastModified seen
	mu            sync.Mutex
}

// NewAWSS3InboundConnector creates a production AWS S3 inbound connector.
func NewAWSS3InboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "aws_s3_inbound",
		DisplayName:        "AWS S3 Bucket Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_iam_role":         true,
			"supports_after_processing": true,
			"supports_patterns":         true,
		},
	}
	return &AWSS3InboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize validates config and creates the S3 client.
func (c *AWSS3InboundConnector) Initialize(configBytes []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := parseJSON(configBytes, &c.config); err != nil {
		return fmt.Errorf("failed to parse S3 inbound config: %w", err)
	}

	// Validate required fields
	if c.config.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	if c.config.Region == "" {
		return fmt.Errorf("region is required")
	}

	// Defaults
	if c.config.PollingInterval == 0 {
		c.config.PollingInterval = 60
	}
	if c.config.MaxKeys == 0 {
		c.config.MaxKeys = 100
	}
	if c.config.MaxFileSizeMB == 0 {
		c.config.MaxFileSizeMB = defaultMaxFileSizeMB
	}
	if c.config.AfterProcessing == "" {
		c.config.AfterProcessing = "nothing"
	}

	// Build AWS config
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

	c.BaseInboundConnector.BaseConnector.initialized = true
	c.BaseInboundConnector.SetState(StateReady)

	log.Printf("✅ AWS S3 Inbound: Initialized (bucket=%s, region=%s, prefix=%q)",
		c.config.Bucket, c.config.Region, c.config.Prefix)
	return nil
}

// buildAWSConfig constructs the aws.Config with explicit or default credentials.
func (c *AWSS3InboundConnector) buildAWSConfig() (aws.Config, error) {
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
	// Fall back to default credential chain (IAM role, env, ~/.aws/credentials)
	return config.LoadDefaultConfig(ctx, config.WithRegion(c.config.Region))
}

// TestConnection calls HeadBucket to verify bucket access.
func (c *AWSS3InboundConnector) TestConnection(ctx context.Context) error {
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
func (c *AWSS3InboundConnector) Validate() error {
	if !c.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if c.config.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	return nil
}

// SupportsCron returns true — S3 polling can be triggered by cron.
func (c *AWSS3InboundConnector) SupportsCron() bool { return true }

// Start launches the polling goroutine.
func (c *AWSS3InboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if !c.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	c.BaseInboundConnector.SetState(StateRunning)
	go c.pollLoop(ctx, messageChan)
	return nil
}

// pollLoop runs a periodic poll against the S3 bucket.
func (c *AWSS3InboundConnector) pollLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	ticker := time.NewTicker(time.Duration(c.config.PollingInterval) * time.Second)
	defer ticker.Stop()

	// Poll immediately
	c.poll(ctx, messageChan)

	for {
		select {
		case <-ctx.Done():
			log.Printf("☁️  AWS S3 Inbound: Context cancelled")
			return
		case <-c.BaseInboundConnector.stopCh:
			log.Printf("☁️  AWS S3 Inbound: Stop signal received")
			return
		case <-ticker.C:
			c.poll(ctx, messageChan)
		}
	}
}

// poll lists objects newer than lastProcessed, downloads each one, and emits messages.
func (c *AWSS3InboundConnector) poll(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	c.mu.Lock()
	client := c.s3Client
	lastSeen := c.lastProcessed
	c.mu.Unlock()

	if client == nil {
		return
	}

	// List objects
	objects, err := c.listNewObjects(ctx, client, lastSeen)
	if err != nil {
		log.Printf("❌ AWS S3 Inbound: ListObjects failed: %v", err)
		return
	}
	if len(objects) == 0 {
		return
	}

	log.Printf("☁️  AWS S3 Inbound: Found %d new objects in s3://%s/%s",
		len(objects), c.config.Bucket, c.config.Prefix)

	var newestSeen time.Time
	for _, obj := range objects {
		if obj.Key == nil || obj.LastModified == nil {
			continue
		}

		// Filter by file pattern if set
		if c.config.FilePattern != "" {
			base := filepath.Base(*obj.Key)
			matched, _ := filepath.Match(c.config.FilePattern, base)
			if !matched {
				continue
			}
		}

		// Skip oversized files
		maxBytes := int64(c.config.MaxFileSizeMB) * 1024 * 1024
		if obj.Size != nil && *obj.Size > maxBytes {
			log.Printf("⚠️  AWS S3 Inbound: Skipping %s (%d MB > limit %d MB)",
				*obj.Key, *obj.Size/1024/1024, c.config.MaxFileSizeMB)
			continue
		}

		// Download and emit
		msg, err := c.downloadObject(ctx, client, *obj.Key, obj)
		if err != nil {
			log.Printf("❌ AWS S3 Inbound: Failed to download %s: %v", *obj.Key, err)
			continue
		}
		messageChan <- msg

		// After-processing
		if err := c.afterProcess(ctx, client, *obj.Key); err != nil {
			log.Printf("⚠️  AWS S3 Inbound: After-processing failed for %s: %v", *obj.Key, err)
		}

		if obj.LastModified.After(newestSeen) {
			newestSeen = *obj.LastModified
		}
	}

	// Advance cursor
	if !newestSeen.IsZero() {
		c.mu.Lock()
		if newestSeen.After(c.lastProcessed) {
			c.lastProcessed = newestSeen
		}
		c.mu.Unlock()
	}
}

// listNewObjects returns objects newer than lastSeen, sorted by LastModified.
func (c *AWSS3InboundConnector) listNewObjects(ctx context.Context, client *s3.Client, lastSeen time.Time) ([]types.Object, error) {
	var all []types.Object
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket:  aws.String(c.config.Bucket),
		Prefix:  aws.String(c.config.Prefix),
		MaxKeys: aws.Int32(int32(c.config.MaxKeys)),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if obj.Key == nil || obj.LastModified == nil {
				continue
			}
			// Skip the prefix itself (virtual directory marker)
			if *obj.Key == c.config.Prefix {
				continue
			}
			if lastSeen.IsZero() || obj.LastModified.After(lastSeen) {
				all = append(all, obj)
			}
		}
	}

	// Sort by LastModified ascending for deterministic processing order
	sort.Slice(all, func(i, j int) bool {
		if all[i].LastModified == nil || all[j].LastModified == nil {
			return false
		}
		return all[i].LastModified.Before(*all[j].LastModified)
	})
	return all, nil
}

// downloadObject retrieves an S3 object and creates an InboundMessage.
func (c *AWSS3InboundConnector) downloadObject(ctx context.Context, client *s3.Client, key string, obj types.Object) (*models.InboundMessage, error) {
	resp, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.config.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read object body: %w", err)
	}

	content := string(bodyBytes)
	contentType := detectS3ContentType(key, content, resp.ContentType)

	// Build a unique message ID from bucket/key
	msgID := fmt.Sprintf("s3_%s_%s", c.config.Bucket,
		strings.ReplaceAll(strings.ReplaceAll(key, "/", "_"), ".", "_"))

	return &models.InboundMessage{
		MessageID:      msgID,
		Content:        content,
		ContentType:    contentType,
		SourceType:     "aws_s3",
		SourceEndpoint: fmt.Sprintf("s3://%s/%s", c.config.Bucket, key),
		MessageSize:    len(content),
		ReceivedAt:     time.Now(),
		Headers: map[string]string{
			"S3-Bucket":        c.config.Bucket,
			"S3-Key":           key,
			"S3-ETag":          aws.ToString(resp.ETag),
			"S3-LastModified":  obj.LastModified.Format(time.RFC3339),
		},
	}, nil
}

// afterProcess executes the configured post-ingestion action on the S3 object.
func (c *AWSS3InboundConnector) afterProcess(ctx context.Context, client *s3.Client, key string) error {
	switch c.config.AfterProcessing {
	case "delete":
		_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(c.config.Bucket),
			Key:    aws.String(key),
		})
		return err

	case "archive":
		archivePrefix := c.config.ArchivePrefix
		if archivePrefix == "" {
			archivePrefix = "processed/"
		}
		base := filepath.Base(key)
		destKey := archivePrefix + base

		// Copy to archive
		_, err := client.CopyObject(ctx, &s3.CopyObjectInput{
			Bucket:     aws.String(c.config.Bucket),
			CopySource: aws.String(c.config.Bucket + "/" + key),
			Key:        aws.String(destKey),
		})
		if err != nil {
			return fmt.Errorf("CopyObject to %s failed: %w", destKey, err)
		}
		// Delete original
		_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(c.config.Bucket),
			Key:    aws.String(key),
		})
		return err
	}
	return nil // "nothing"
}

// detectS3ContentType determines content type from key extension, content sniffing, or S3 header.
func detectS3ContentType(key, content string, s3ContentType *string) string {
	// Extension-based detection
	lower := strings.ToLower(key)
	switch {
	case strings.HasSuffix(lower, ".hl7") || strings.HasSuffix(lower, ".msh"):
		return "application/hl7-v2"
	case strings.HasSuffix(lower, ".json"):
		return "application/json"
	case strings.HasSuffix(lower, ".xml"):
		return "application/xml"
	case strings.HasSuffix(lower, ".csv"):
		return "text/csv"
	}
	// Content sniffing
	if ct := detectContentType(content); ct != "text/plain" {
		return ct
	}
	// S3 Content-Type header fallback
	if s3ContentType != nil && *s3ContentType != "" && *s3ContentType != "binary/octet-stream" {
		return *s3ContentType
	}
	return "text/plain"
}

// Stop halts the polling loop.
func (c *AWSS3InboundConnector) Stop() error {
	log.Printf("☁️  AWS S3 Inbound: Stopping")
	return c.BaseInboundConnector.Stop()
}

// Close is an alias for Stop.
func (c *AWSS3InboundConnector) Close() error { return c.Stop() }
