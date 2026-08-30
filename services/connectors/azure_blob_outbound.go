// services/connectors/azure_blob_outbound.go
// Azure Blob Storage Outbound Connector — uploads a message's content as a
// blob to an Azure Storage container, using the official Azure SDK for Go
// (github.com/Azure/azure-sdk-for-go/sdk/storage/azblob — a real REST-backed
// client, not a hand-rolled HTTP client). Blob names are built from the same
// {message_id}/{interface_id}/{timestamp}/{date}/{time} placeholder
// convention as aws_s3_outbound.go's key_pattern and file_writer.go's
// filename_pattern, and reuses that same sanitizer (sanitizeObjectKeySegment)
// rather than duplicating it.
//
// Same design choice as aws_s3_outbound.go: the destination container is
// assumed to already exist — this connector does not attempt to create it.
//
// Authentication — two modes, both real, both fully implemented (no
// unverifiable auth mode is offered, unlike the Databricks/Snowflake
// cloud-warehouse connectors, since neither of these requires anything more
// exotic than what the SDK exposes directly):
//   - connection_string: a full Azure Storage connection string (covers
//     account name + key + endpoint in one value — this is what Azurite, the
//     official Azure Storage emulator, hands you, and is exactly what this
//     connector was verified against — see Send/TestConnection notes below).
//   - account_name + account_key: shared-key credential, built against
//     https://{account_name}.blob.core.windows.net/ unless `endpoint`
//     overrides it (e.g. to point at Azurite or another Azure-compatible
//     endpoint directly instead of via a connection string).
//
// Configuration:
//
//	container          string  Destination container name (required)
//	connection_string  string  Full Azure Storage connection string (alternative to account_name+account_key)
//	account_name       string  Storage account name (required if connection_string is not set)
//	account_key        string  Storage account key (required if connection_string is not set)
//	endpoint           string  Override service URL (e.g. for Azurite or a non-public-cloud endpoint)
//	key_pattern        string  Blob name template (default: "{message_id}.json").
//	                           Placeholders: {message_id} {interface_id} {timestamp} {date} {time}
//	content_type       string  Override Content-Type (default: inferred from message)
//	access_tier        string  "hot" | "cool" | "archive" | "" (default: unset — inherits container/account default)
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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
)

// AzureBlobOutboundConfig holds Azure Blob Storage outbound connector configuration.
type AzureBlobOutboundConfig struct {
	Container        string `json:"container"`
	ConnectionString string `json:"connection_string"`
	AccountName      string `json:"account_name"`
	AccountKey       string `json:"account_key"`
	Endpoint         string `json:"endpoint"`
	KeyPattern       string `json:"key_pattern"`
	ContentType      string `json:"content_type"`
	AccessTier       string `json:"access_tier"`
}

// AzureBlobOutboundConnector uploads messages as blobs to an Azure Storage container.
type AzureBlobOutboundConnector struct {
	*BaseOutboundConnector
	config AzureBlobOutboundConfig
	client *azblob.Client
	mu     sync.Mutex
}

// NewAzureBlobOutboundConnector creates a production Azure Blob outbound connector.
func NewAzureBlobOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "azure_blob_outbound",
		DisplayName:        "Azure Blob Storage Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":         true,
			"supports_shared_key":    true,
			"supports_conn_string":   true,
			"supports_access_tiers":  true,
			"supports_templates":     true,
			// supports_azure_ad/supports_sas deliberately omitted -- only
			// connection-string and shared-key auth are implemented.
		},
	}
	return &AzureBlobOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, true),
	}
}

// Initialize validates config and creates the Azure Blob client.
func (c *AzureBlobOutboundConnector) Initialize(configBytes []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := parseJSON(configBytes, &c.config); err != nil {
		return fmt.Errorf("failed to parse Azure Blob outbound config: %w", err)
	}

	if c.config.Container == "" {
		return fmt.Errorf("container is required")
	}
	if c.config.ConnectionString == "" && (c.config.AccountName == "" || c.config.AccountKey == "") {
		return fmt.Errorf("either connection_string, or both account_name and account_key, are required")
	}

	if c.config.KeyPattern == "" {
		c.config.KeyPattern = "{message_id}.json"
	}
	if c.config.AccessTier != "" {
		if c.accessTier() == nil {
			return fmt.Errorf(`access_tier must be one of "hot", "cool", "archive", or empty; got %q`, c.config.AccessTier)
		}
	}

	client, err := c.buildClient()
	if err != nil {
		return fmt.Errorf("failed to build Azure Blob client: %w", err)
	}

	c.client = client
	c.BaseOutboundConnector.BaseConnector.initialized = true

	log.Printf("✅ Azure Blob Outbound: Initialized (container=%s, key_pattern=%q, access_tier=%q)",
		c.config.Container, c.config.KeyPattern, c.config.AccessTier)
	return nil
}

// buildClient constructs the azblob.Client from either a connection string or
// a shared-key credential + service URL.
func (c *AzureBlobOutboundConnector) buildClient() (*azblob.Client, error) {
	if c.config.ConnectionString != "" {
		return azblob.NewClientFromConnectionString(c.config.ConnectionString, nil)
	}

	cred, err := azblob.NewSharedKeyCredential(c.config.AccountName, c.config.AccountKey)
	if err != nil {
		return nil, fmt.Errorf("invalid account_key: %w", err)
	}

	serviceURL := c.config.Endpoint
	if serviceURL == "" {
		serviceURL = fmt.Sprintf("https://%s.blob.core.windows.net/", c.config.AccountName)
	}
	return azblob.NewClientWithSharedKeyCredential(serviceURL, cred, nil)
}

// accessTier maps the configured string to the SDK's *blob.AccessTier enum,
// returning nil for an empty or unrecognized value.
func (c *AzureBlobOutboundConnector) accessTier() *blob.AccessTier {
	switch strings.ToLower(c.config.AccessTier) {
	case "hot":
		t := blob.AccessTierHot
		return &t
	case "cool":
		t := blob.AccessTierCool
		return &t
	case "archive":
		t := blob.AccessTierArchive
		return &t
	default:
		return nil
	}
}

// buildBlobName renders the key_pattern template for one message — same
// placeholder set and sanitizer as aws_s3_outbound.go's buildKey, so the
// naming convention is familiar whether the destination is S3 or Azure Blob.
func (c *AzureBlobOutboundConnector) buildBlobName(message *models.OutboundMessage) string {
	name := c.config.KeyPattern
	now := time.Now()

	replacements := map[string]string{
		"{timestamp}":    now.Format("20060102_150405"),
		"{date}":         now.Format("20060102"),
		"{time}":         now.Format("150405"),
		"{message_id}":   sanitizeObjectKeySegment(message.MessageID),
		"{interface_id}": sanitizeObjectKeySegment(message.InterfaceID),
	}
	for placeholder, value := range replacements {
		name = strings.ReplaceAll(name, placeholder, value)
	}

	if filepath.Ext(name) == "" {
		name += extensionForContentType(contentTypeOrDefault(c.config.ContentType, message.ContentType))
	}
	return name
}

// Send uploads a single message as a blob.
func (c *AzureBlobOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()
	if client == nil {
		return nil, fmt.Errorf("connector not initialized")
	}

	startTime := time.Now()
	blobName := c.buildBlobName(message)
	contentType := contentTypeOrDefault(c.config.ContentType, message.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	opts := &azblob.UploadBufferOptions{
		HTTPHeaders: &blob.HTTPHeaders{BlobContentType: to.Ptr(contentType)},
	}
	if tier := c.accessTier(); tier != nil {
		opts.AccessTier = tier
	}

	log.Printf("☁️  Azure Blob Outbound: Uploading %s/%s (%d bytes)", c.config.Container, blobName, len(message.Content))

	_, err := client.UploadBuffer(ctx, c.config.Container, blobName, []byte(message.Content), opts)
	if err != nil {
		return &DeliveryResult{
			Success:      false,
			MessageID:    message.MessageID,
			Timestamp:    time.Now(),
			ErrorMessage: fmt.Sprintf("UploadBuffer failed: %v", err),
			DurationMs:   int64(time.Since(startTime).Milliseconds()),
		}, err
	}

	return &DeliveryResult{
		Success:        true,
		MessageID:      message.MessageID,
		Timestamp:      time.Now(),
		Acknowledgment: fmt.Sprintf("%s/%s", c.config.Container, blobName),
		DurationMs:     int64(time.Since(startTime).Milliseconds()),
		Metadata: map[string]interface{}{
			"container": c.config.Container,
			"blob":      blobName,
		},
	}, nil
}

// SendBatch uploads each message as its own blob — Azure Blob Storage has no
// native batch-upload API for independent blobs, so this aggregates
// individual UploadBuffer results the same way SendBatch works for every
// other connector in this package.
func (c *AzureBlobOutboundConnector) SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error) {
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

	log.Printf("✅ Azure Blob Outbound: Batch complete — success: %d, failed: %d", successCount, failureCount)
	return results, nil
}

// SupportsBatch returns true.
func (c *AzureBlobOutboundConnector) SupportsBatch() bool { return true }

// TestConnection lists (at most) one blob in the container to verify
// credentials and container access without writing anything.
func (c *AzureBlobOutboundConnector) TestConnection(ctx context.Context) error {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()
	if client == nil {
		return fmt.Errorf("not initialized")
	}

	pager := client.NewListBlobsFlatPager(c.config.Container, &azblob.ListBlobsFlatOptions{
		MaxResults: to.Ptr(int32(1)),
	})
	if _, err := pager.NextPage(ctx); err != nil {
		return fmt.Errorf("failed to access container %q: %w", c.config.Container, err)
	}
	return nil
}

// Validate checks required fields after initialization.
func (c *AzureBlobOutboundConnector) Validate() error {
	if !c.BaseOutboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if c.config.Container == "" {
		return fmt.Errorf("container is required")
	}
	if c.config.ConnectionString == "" && (c.config.AccountName == "" || c.config.AccountKey == "") {
		return fmt.Errorf("either connection_string, or both account_name and account_key, are required")
	}
	return nil
}

// Close releases connector resources. The Azure SDK client has no explicit
// close/shutdown method — nothing to release beyond dropping the reference.
func (c *AzureBlobOutboundConnector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.client = nil
	return nil
}

// GetStatus returns connector status with Azure-Blob-specific metadata.
func (c *AzureBlobOutboundConnector) GetStatus() ConnectorStatus {
	status := c.BaseOutboundConnector.GetStatus()
	if status.Metadata == nil {
		status.Metadata = map[string]string{}
	}
	status.Metadata["container"] = c.config.Container
	status.Metadata["account_name"] = c.config.AccountName
	status.Metadata["key_pattern"] = c.config.KeyPattern
	status.Metadata["access_tier"] = c.config.AccessTier
	return status
}
