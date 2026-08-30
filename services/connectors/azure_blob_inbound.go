// services/connectors/azure_blob_inbound.go
// Azure Blob Storage Inbound Connector — polls an Azure Storage container for
// new blobs, downloads them, and converts each one into an InboundMessage for
// the pipeline. Uses the official Azure SDK for Go (same package as
// azure_blob_outbound.go — a real REST-backed client, not a hand-rolled HTTP
// client), mirroring aws_s3_inbound.go's polling/watermark/after-processing
// design so the two "cloud object storage inbound" connectors behave
// consistently.
//
// Polling strategy: list blobs (optionally prefix-filtered), tracking the
// most recent LastModified timestamp seen as a watermark so only newer blobs
// are picked up on each poll — same approach as aws_s3_inbound.go.
//
// After-processing options (same three as aws_s3_inbound.go):
//   - nothing  (default): leave the blob in the container
//   - delete:             DeleteBlob after successful ingestion
//   - archive:            re-upload the already-downloaded content under
//                         archive_prefix, then DeleteBlob the original —
//                         cheaper than a separate server-side copy call
//                         since the content is already in hand from download.
//
// Authentication — same two real modes as azure_blob_outbound.go:
//	connection_string, or account_name + account_key (with optional endpoint
//	override for Azurite or a non-public-cloud endpoint).
//
// Configuration:
//
//	container          string  Source container name (required)
//	connection_string  string  Full Azure Storage connection string (alternative to account_name+account_key)
//	account_name       string  Storage account name (required if connection_string is not set)
//	account_key        string  Storage account key (required if connection_string is not set)
//	endpoint           string  Override service URL (e.g. for Azurite or a non-public-cloud endpoint)
//	prefix             string  Blob name prefix to filter (e.g. "inbound/")
//	file_pattern       string  Glob suffix filter (e.g. "*.hl7") — matched against the blob name's base
//	after_processing   string  "nothing" | "delete" | "archive" (default: "nothing")
//	archive_prefix     string  Prefix for archived blobs (default: "processed/")
//	polling_interval   int     Seconds between polls (default: 60)
//	max_blobs          int     Max blobs per ListBlobsFlat page (default: 100)
//	max_file_size_mb   int     Skip blobs larger than this (default: 50)
package connectors

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// AzureBlobInboundConfig holds Azure Blob Storage inbound connector configuration.
type AzureBlobInboundConfig struct {
	Container        string `json:"container"`
	ConnectionString string `json:"connection_string"`
	AccountName      string `json:"account_name"`
	AccountKey       string `json:"account_key"`
	Endpoint         string `json:"endpoint"`
	Prefix           string `json:"prefix"`
	FilePattern      string `json:"file_pattern"`
	AfterProcessing  string `json:"after_processing"`
	ArchivePrefix    string `json:"archive_prefix"`
	PollingInterval  int    `json:"polling_interval"`
	MaxBlobs         int    `json:"max_blobs"`
	MaxFileSizeMB    int    `json:"max_file_size_mb"`
}

// AzureBlobInboundConnector polls an Azure Storage container for new blobs.
type AzureBlobInboundConnector struct {
	*BaseInboundConnector
	config        AzureBlobInboundConfig
	client        *azblob.Client
	lastProcessed time.Time
	mu            sync.Mutex
}

// NewAzureBlobInboundConnector creates a production Azure Blob inbound connector.
func NewAzureBlobInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "azure_blob_inbound",
		DisplayName:        "Azure Blob Storage Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_shared_key":       true,
			"supports_conn_string":      true,
			"supports_after_processing": true,
			"supports_patterns":         true,
		},
	}
	return &AzureBlobInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize validates config and creates the Azure Blob client.
func (c *AzureBlobInboundConnector) Initialize(configBytes []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := parseJSON(configBytes, &c.config); err != nil {
		return fmt.Errorf("failed to parse Azure Blob inbound config: %w", err)
	}

	if c.config.Container == "" {
		return fmt.Errorf("container is required")
	}
	if c.config.ConnectionString == "" && (c.config.AccountName == "" || c.config.AccountKey == "") {
		return fmt.Errorf("either connection_string, or both account_name and account_key, are required")
	}

	if c.config.PollingInterval == 0 {
		c.config.PollingInterval = 60
	}
	if c.config.MaxBlobs == 0 {
		c.config.MaxBlobs = 100
	}
	if c.config.MaxFileSizeMB == 0 {
		c.config.MaxFileSizeMB = defaultMaxFileSizeMB
	}
	if c.config.AfterProcessing == "" {
		c.config.AfterProcessing = "nothing"
	}

	client, err := c.buildClient()
	if err != nil {
		return fmt.Errorf("failed to build Azure Blob client: %w", err)
	}

	c.client = client
	c.BaseInboundConnector.BaseConnector.initialized = true
	c.BaseInboundConnector.SetState(StateReady)

	log.Printf("✅ Azure Blob Inbound: Initialized (container=%s, prefix=%q)", c.config.Container, c.config.Prefix)
	return nil
}

// buildClient constructs the azblob.Client from either a connection string or
// a shared-key credential + service URL — identical to azure_blob_outbound.go.
func (c *AzureBlobInboundConnector) buildClient() (*azblob.Client, error) {
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

// TestConnection lists (at most) one blob in the container to verify
// credentials and container access without writing anything.
func (c *AzureBlobInboundConnector) TestConnection(ctx context.Context) error {
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
func (c *AzureBlobInboundConnector) Validate() error {
	if !c.BaseInboundConnector.BaseConnector.initialized {
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

// SupportsCron returns true — Azure Blob polling can be triggered by cron.
func (c *AzureBlobInboundConnector) SupportsCron() bool { return true }

// Start launches the polling goroutine.
func (c *AzureBlobInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if !c.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	c.BaseInboundConnector.SetState(StateRunning)
	go c.pollLoop(ctx, messageChan)
	return nil
}

func (c *AzureBlobInboundConnector) pollLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	ticker := time.NewTicker(time.Duration(c.config.PollingInterval) * time.Second)
	defer ticker.Stop()

	c.poll(ctx, messageChan)

	for {
		select {
		case <-ctx.Done():
			log.Printf("☁️  Azure Blob Inbound: Context cancelled")
			return
		case <-c.BaseInboundConnector.stopCh:
			log.Printf("☁️  Azure Blob Inbound: Stop signal received")
			return
		case <-ticker.C:
			c.poll(ctx, messageChan)
		}
	}
}

// azureBlobEntry is the subset of listing metadata this connector needs,
// decoupled from the SDK's own pointer-heavy generated types.
type azureBlobEntry struct {
	name         string
	lastModified time.Time
	size         int64
}

// poll lists blobs newer than lastProcessed, downloads each one, and emits messages.
func (c *AzureBlobInboundConnector) poll(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	c.mu.Lock()
	client := c.client
	lastSeen := c.lastProcessed
	c.mu.Unlock()
	if client == nil {
		return
	}

	entries, err := c.listNewBlobs(ctx, client, lastSeen)
	if err != nil {
		log.Printf("❌ Azure Blob Inbound: List failed: %v", err)
		return
	}
	if len(entries) == 0 {
		return
	}

	log.Printf("☁️  Azure Blob Inbound: Found %d new blob(s) in container %s (prefix=%q)",
		len(entries), c.config.Container, c.config.Prefix)

	var newestSeen time.Time
	for _, entry := range entries {
		if c.config.FilePattern != "" {
			matched, _ := filepath.Match(c.config.FilePattern, filepath.Base(entry.name))
			if !matched {
				continue
			}
		}

		maxBytes := int64(c.config.MaxFileSizeMB) * 1024 * 1024
		if entry.size > maxBytes {
			log.Printf("⚠️  Azure Blob Inbound: Skipping %s (%d MB > limit %d MB)",
				entry.name, entry.size/1024/1024, c.config.MaxFileSizeMB)
			continue
		}

		msg, content, err := c.downloadBlob(ctx, client, entry)
		if err != nil {
			log.Printf("❌ Azure Blob Inbound: Failed to download %s: %v", entry.name, err)
			continue
		}
		messageChan <- msg

		if err := c.afterProcess(ctx, client, entry.name, content); err != nil {
			log.Printf("⚠️  Azure Blob Inbound: After-processing failed for %s: %v", entry.name, err)
		}

		if entry.lastModified.After(newestSeen) {
			newestSeen = entry.lastModified
		}
	}

	if !newestSeen.IsZero() {
		c.mu.Lock()
		if newestSeen.After(c.lastProcessed) {
			c.lastProcessed = newestSeen
		}
		c.mu.Unlock()
	}
}

// listNewBlobs returns blob entries newer than lastSeen, sorted by LastModified.
func (c *AzureBlobInboundConnector) listNewBlobs(ctx context.Context, client *azblob.Client, lastSeen time.Time) ([]azureBlobEntry, error) {
	var all []azureBlobEntry

	opts := &azblob.ListBlobsFlatOptions{MaxResults: to.Ptr(int32(c.config.MaxBlobs))}
	if c.config.Prefix != "" {
		opts.Prefix = to.Ptr(c.config.Prefix)
	}
	pager := client.NewListBlobsFlatPager(c.config.Container, opts)

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Segment.BlobItems {
			if item.Name == nil || item.Properties == nil || item.Properties.LastModified == nil {
				continue
			}
			lastModified := *item.Properties.LastModified
			if !lastSeen.IsZero() && !lastModified.After(lastSeen) {
				continue
			}
			var size int64
			if item.Properties.ContentLength != nil {
				size = *item.Properties.ContentLength
			}
			all = append(all, azureBlobEntry{name: *item.Name, lastModified: lastModified, size: size})
		}
	}

	sort.Slice(all, func(i, j int) bool { return all[i].lastModified.Before(all[j].lastModified) })
	return all, nil
}

// downloadBlob retrieves a blob's content and builds an InboundMessage.
// Returns the raw content too, so afterProcess's "archive" mode can re-upload
// it without a second round trip.
func (c *AzureBlobInboundConnector) downloadBlob(ctx context.Context, client *azblob.Client, entry azureBlobEntry) (*models.InboundMessage, []byte, error) {
	buf := make([]byte, entry.size)
	n, err := client.DownloadBuffer(ctx, c.config.Container, entry.name, buf, nil)
	if err != nil {
		return nil, nil, err
	}
	content := buf[:n]
	contentStr := string(content)
	contentType := detectS3ContentType(entry.name, contentStr, nil)

	msgID := fmt.Sprintf("azblob_%s_%s", c.config.Container,
		strings.ReplaceAll(strings.ReplaceAll(entry.name, "/", "_"), ".", "_"))

	return &models.InboundMessage{
		MessageID:      msgID,
		Content:        contentStr,
		ContentType:    contentType,
		SourceType:     "azure_blob",
		SourceEndpoint: fmt.Sprintf("azblob://%s/%s", c.config.Container, entry.name),
		MessageSize:    len(content),
		ReceivedAt:     time.Now(),
		Headers: map[string]string{
			"Azure-Container":     c.config.Container,
			"Azure-Blob-Name":     entry.name,
			"Azure-Last-Modified": entry.lastModified.Format(time.RFC3339),
		},
	}, content, nil
}

// afterProcess executes the configured post-ingestion action on the blob.
// "archive" re-uploads the already-downloaded content under archive_prefix
// (avoiding a separate server-side copy call) then deletes the original.
func (c *AzureBlobInboundConnector) afterProcess(ctx context.Context, client *azblob.Client, name string, content []byte) error {
	switch c.config.AfterProcessing {
	case "delete":
		_, err := client.DeleteBlob(ctx, c.config.Container, name, nil)
		return err

	case "archive":
		archivePrefix := c.config.ArchivePrefix
		if archivePrefix == "" {
			archivePrefix = "processed/"
		}
		destName := archivePrefix + filepath.Base(name)

		if _, err := client.UploadBuffer(ctx, c.config.Container, destName, content, nil); err != nil {
			return fmt.Errorf("archive upload to %s failed: %w", destName, err)
		}
		if _, err := client.DeleteBlob(ctx, c.config.Container, name, nil); err != nil {
			return fmt.Errorf("archive delete of original %s failed: %w", name, err)
		}
		return nil
	}
	return nil // "nothing"
}

// Stop halts the polling loop.
func (c *AzureBlobInboundConnector) Stop() error {
	log.Printf("☁️  Azure Blob Inbound: Stopping")
	return c.BaseInboundConnector.Stop()
}

// Close is an alias for Stop.
func (c *AzureBlobInboundConnector) Close() error { return c.Stop() }
