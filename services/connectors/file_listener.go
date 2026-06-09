// services/connectors/file_listener.go
// File System Listener - Inbound Connector for reading files from directories

package connectors

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileListenerConnector implements file system polling for inbound messages
type FileListenerConnector struct {
	*BaseInboundConnector
	directoryPath    string
	filePattern      string // e.g., "*.hl7", "*.xml", "message_*.txt"
	pollingInterval  time.Duration
	afterProcessing  string // "delete", "move", "archive", "nothing"
	archivePath      string // where to move processed files
	encoding         string // UTF-8, ASCII, etc.
	recursive        bool   // scan subdirectories
	messageChan      chan<- *models.InboundMessage
	stopChan         chan struct{}
	mu               sync.RWMutex
	processedFiles   map[string]time.Time // track processed files to avoid duplicates
}

// NewFileListenerConnector creates a new file listener connector
func NewFileListenerConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "file_listener",
		DisplayName:        "File System Listener",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_after_processing": true,
			"supports_patterns":         true,
		},
	}
	return &FileListenerConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
		pollingInterval:      10 * time.Second, // default
		afterProcessing:      "nothing",
		encoding:             "UTF-8",
		recursive:            false,
		processedFiles:       make(map[string]time.Time),
		stopChan:             make(chan struct{}),
	}
}

// Initialize configures the file listener from JSON config
func (f *FileListenerConnector) Initialize(config []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	log.Printf("🔍 File Listener Initialize called with config: %s", string(config))

	var cfg map[string]interface{}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Directory path (required)
	if path, ok := cfg["directory_path"].(string); ok && path != "" {
		f.directoryPath = path
		log.Printf("🔍 Directory path set to: %s", f.directoryPath)
	} else if path, ok := cfg["directoryPath"].(string); ok && path != "" {
		f.directoryPath = path
		log.Printf("🔍 Directory path set to: %s", f.directoryPath)
	} else {
		return fmt.Errorf("directory_path is required")
	}

	// File pattern (default: *)
	if pattern, ok := cfg["file_pattern"].(string); ok && pattern != "" {
		f.filePattern = pattern
	} else if pattern, ok := cfg["filePattern"].(string); ok && pattern != "" {
		f.filePattern = pattern
	} else {
		f.filePattern = "*" // all files
	}
	log.Printf("🔍 File pattern set to: %s", f.filePattern)

	// Polling interval (in seconds)
	if interval, ok := cfg["polling_interval"].(float64); ok && interval > 0 {
		f.pollingInterval = time.Duration(interval) * time.Second
	} else if interval, ok := cfg["pollingInterval"].(float64); ok && interval > 0 {
		f.pollingInterval = time.Duration(interval) * time.Second
	}
	log.Printf("🔍 Polling interval set to: %v", f.pollingInterval)

	// After processing action
	if action, ok := cfg["after_processing"].(string); ok {
		f.afterProcessing = action
	} else if action, ok := cfg["afterProcessing"].(string); ok {
		f.afterProcessing = action
	}
	log.Printf("🔍 After processing set to: %s", f.afterProcessing)

	// Archive path (for move/archive actions)
	if archPath, ok := cfg["archive_path"].(string); ok && archPath != "" {
		f.archivePath = archPath
	} else if archPath, ok := cfg["archivePath"].(string); ok && archPath != "" {
		f.archivePath = archPath
	}

	// Encoding
	if enc, ok := cfg["encoding"].(string); ok && enc != "" {
		f.encoding = enc
	}

	// Recursive scanning
	if recursive, ok := cfg["recursive"].(bool); ok {
		f.recursive = recursive
	}

	// Validate directory exists
	if _, err := os.Stat(f.directoryPath); os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist: %s", f.directoryPath)
	}

	// Create archive directory if needed
	if (f.afterProcessing == "move" || f.afterProcessing == "archive") && f.archivePath != "" {
		if err := os.MkdirAll(f.archivePath, 0755); err != nil {
			return fmt.Errorf("failed to create archive directory: %w", err)
		}
	}

	log.Printf("✅ File Listener initialized: path=%s, pattern=%s, polling=%v, after=%s",
		f.directoryPath, f.filePattern, f.pollingInterval, f.afterProcessing)

	// Mark as initialized for base connector validation
	f.BaseInboundConnector.BaseConnector.initialized = true

	return nil
}

// Start begins polling the directory for files
func (f *FileListenerConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	f.mu.Lock()
	f.messageChan = messageChan
	f.mu.Unlock()

	log.Printf("📂 File Listener: Polling directory %s (pattern: %s)", f.directoryPath, f.filePattern)

	// Start polling goroutine
	go f.poll(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	return f.Stop()
}

// Stop stops the file listener
func (f *FileListenerConnector) Stop() error {
	close(f.stopChan)
	log.Printf("🛑 File Listener: Stopped polling %s", f.directoryPath)
	return nil
}

// SupportsCron returns true since file listener uses polling
func (f *FileListenerConnector) SupportsCron() bool {
	return true
}

// poll continuously scans the directory for new files
func (f *FileListenerConnector) poll(ctx context.Context) {
	ticker := time.NewTicker(f.pollingInterval)
	defer ticker.Stop()

	// Do initial scan immediately
	f.scanDirectory()

	for {
		select {
		case <-ticker.C:
			f.scanDirectory()
		case <-ctx.Done():
			return
		case <-f.stopChan:
			return
		}
	}
}

// scanDirectory scans the directory and processes matching files
func (f *FileListenerConnector) scanDirectory() {
	var files []string
	var err error

	if f.recursive {
		err = filepath.Walk(f.directoryPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				matched, _ := filepath.Match(f.filePattern, info.Name())
				if matched {
					files = append(files, path)
				}
			}
			return nil
		})
	} else {
		pattern := filepath.Join(f.directoryPath, f.filePattern)
		files, err = filepath.Glob(pattern)
	}

	if err != nil {
		log.Printf("❌ File Listener: Error scanning directory: %v", err)
		return
	}

	log.Printf("📂 File Listener: Found %d matching files", len(files))

	for _, filePath := range files {
		f.processFile(filePath)
	}
}

// processFile reads and processes a single file
func (f *FileListenerConnector) processFile(filePath string) {
	f.mu.RLock()
	messageChan := f.messageChan
	f.mu.RUnlock()

	// Check if already processed
	f.mu.Lock()
	if _, processed := f.processedFiles[filePath]; processed {
		f.mu.Unlock()
		return
	}
	f.processedFiles[filePath] = time.Now()
	f.mu.Unlock()

	// Read file content
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Printf("❌ File Listener: Failed to read file %s: %v", filePath, err)
		return
	}

	fileName := filepath.Base(filePath)
	log.Printf("📄 File Listener: Processing file %s (%d bytes)", fileName, len(content))

	// Create inbound message
	message := &models.InboundMessage{
		MessageID:       f.generateFileMessageID(fileName),
		CorrelationID:   "",
		Content:         string(content), // Convert []byte to string
		ContentType:     f.detectContentType(fileName),
		SourceType:      "file",
		SourceEndpoint:  filePath,
		ReceivedAt:      time.Now(),
		Headers:         map[string]string{
			"File-Name": fileName,
			"File-Path": filePath,
			"File-Size": fmt.Sprintf("%d", len(content)),
		},
	}

	// Send to processing channel
	select {
	case messageChan <- message:
		log.Printf("✅ File Listener: File %s queued for processing", fileName)

		// Apply after-processing action
		if err := f.afterProcessFile(filePath); err != nil {
			log.Printf("⚠️ File Listener: After-processing failed for %s: %v", fileName, err)
		}
	default:
		log.Printf("⚠️ File Listener: Message channel full, skipping file %s", fileName)
	}
}

// afterProcessFile handles the file after successful processing
func (f *FileListenerConnector) afterProcessFile(filePath string) error {
	switch f.afterProcessing {
	case "delete":
		log.Printf("🗑️ File Listener: Deleting %s", filePath)
		return os.Remove(filePath)

	case "move", "archive":
		if f.archivePath == "" {
			return fmt.Errorf("archive path not configured")
		}
		fileName := filepath.Base(filePath)
		destPath := filepath.Join(f.archivePath, fileName)

		// Handle duplicate filenames by adding timestamp
		if _, err := os.Stat(destPath); err == nil {
			timestamp := time.Now().Format("20060102_150405")
			ext := filepath.Ext(fileName)
			nameWithoutExt := strings.TrimSuffix(fileName, ext)
			destPath = filepath.Join(f.archivePath, fmt.Sprintf("%s_%s%s", nameWithoutExt, timestamp, ext))
		}

		log.Printf("📦 File Listener: Moving %s to %s", filePath, destPath)
		return os.Rename(filePath, destPath)

	case "nothing":
		// Do nothing, leave file as is
		return nil

	default:
		return fmt.Errorf("unknown after_processing action: %s", f.afterProcessing)
	}
}

// generateFileMessageID creates a unique message ID for the file
func (f *FileListenerConnector) generateFileMessageID(fileName string) string {
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("file_%s_%d", strings.ReplaceAll(fileName, " ", "_"), timestamp)
}

// detectContentType detects content type from file extension
func (f *FileListenerConnector) detectContentType(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".hl7":
		return "x-application/hl7-v2+er7"
	case ".ccd":
		return "application/hl7-ccd+xml"
	case ".xml":
		return "application/xml"
	case ".json":
		return "application/json"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}
