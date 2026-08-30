// services/connectors/file_writer.go
// File System Writer - Outbound Connector for writing messages to files

package connectors

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileWriterConnector implements file system writing for outbound messages
type FileWriterConnector struct {
	*BaseOutboundConnector
	directoryPath    string
	filenamePattern  string // e.g., "message_{timestamp}.hl7", "{message_id}.xml"
	createSubdirs    bool   // create subdirectories if they don't exist
	encoding         string // UTF-8, ASCII, etc.
	filePermissions  os.FileMode
	appendTimestamp  bool   // append timestamp to prevent overwrites
	overwriteExisting bool  // allow overwriting existing files
	mu               sync.RWMutex
}

// NewFileWriterConnector creates a new file writer connector
func NewFileWriterConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "file_writer",
		DisplayName:        "File System Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":      false,
			"supports_templates":  true,
			"supports_subdirs":    true,
		},
	}
	return &FileWriterConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, true), // supportsBatch = true
		filenamePattern:       "message_{timestamp}.txt",
		encoding:              "UTF-8",
		filePermissions:       0644,
		appendTimestamp:       false,
		overwriteExisting:     false,
		createSubdirs:         true,
	}
}

// Initialize configures the file writer from JSON config
func (f *FileWriterConnector) Initialize(config []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	log.Printf("🔍 File Writer Initialize called with config: %s", string(config))

	var cfg map[string]interface{}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Directory path (required)
	if path, ok := cfg["directory_path"].(string); ok && path != "" {
		f.directoryPath = path
	} else if path, ok := cfg["directoryPath"].(string); ok && path != "" {
		f.directoryPath = path
	} else {
		return fmt.Errorf("directory_path is required")
	}
	log.Printf("🔍 Directory path set to: %s", f.directoryPath)

	// Filename pattern
	if pattern, ok := cfg["filename_pattern"].(string); ok && pattern != "" {
		f.filenamePattern = pattern
	} else if pattern, ok := cfg["filenamePattern"].(string); ok && pattern != "" {
		f.filenamePattern = pattern
	}
	log.Printf("🔍 Filename pattern set to: %s", f.filenamePattern)

	// Create subdirectories — "create_subdirectories" is the connectivity_types
	// schema's old (pre-fix) key name; kept as a fallback for defense in depth
	// even though no real saved config currently uses it.
	if createSub, ok := cfg["create_subdirs"].(bool); ok {
		f.createSubdirs = createSub
	} else if createSub, ok := cfg["createSubdirs"].(bool); ok {
		f.createSubdirs = createSub
	} else if createSub, ok := cfg["create_subdirectories"].(bool); ok {
		f.createSubdirs = createSub
	}

	// Encoding — "file_encoding" is the connectivity_types schema's old
	// (pre-fix) key name; kept as a fallback for defense in depth even though
	// no real saved config currently uses it.
	if enc, ok := cfg["encoding"].(string); ok && enc != "" {
		f.encoding = enc
	} else if enc, ok := cfg["file_encoding"].(string); ok && enc != "" {
		f.encoding = enc
	}

	// Append timestamp
	if appendTs, ok := cfg["append_timestamp"].(bool); ok {
		f.appendTimestamp = appendTs
	} else if appendTs, ok := cfg["appendTimestamp"].(bool); ok {
		f.appendTimestamp = appendTs
	}

	// Overwrite existing
	if overwrite, ok := cfg["overwrite_existing"].(bool); ok {
		f.overwriteExisting = overwrite
	} else if overwrite, ok := cfg["overwriteExisting"].(bool); ok {
		f.overwriteExisting = overwrite
	}

	// Create directory if it doesn't exist
	if f.createSubdirs {
		if err := os.MkdirAll(f.directoryPath, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	} else {
		// Verify directory exists
		if _, err := os.Stat(f.directoryPath); os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", f.directoryPath)
		}
	}

	log.Printf("✅ File Writer initialized: path=%s, pattern=%s, encoding=%s",
		f.directoryPath, f.filenamePattern, f.encoding)

	return nil
}

// Send writes a message to a file
func (f *FileWriterConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	startTime := time.Now()

	// Generate filename from pattern
	filename := f.generateFilename(message)
	filePath := filepath.Join(f.directoryPath, filename)

	// Check if file exists and handle based on configuration
	if _, err := os.Stat(filePath); err == nil && !f.overwriteExisting {
		if f.appendTimestamp {
			// Add timestamp to make unique
			timestamp := time.Now().Format("20060102_150405")
			ext := filepath.Ext(filename)
			nameWithoutExt := filename[:len(filename)-len(ext)]
			filename = fmt.Sprintf("%s_%s%s", nameWithoutExt, timestamp, ext)
			filePath = filepath.Join(f.directoryPath, filename)
		} else {
			return &DeliveryResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("File already exists: %s", filename),
				DurationMs:   int64(time.Since(startTime).Milliseconds()),
			}, fmt.Errorf("file exists and overwrite is disabled")
		}
	}

	log.Printf("📝 File Writer: Writing to %s (%d bytes)", filePath, len(message.Content))

	// Write file
	err := os.WriteFile(filePath, []byte(message.Content), f.filePermissions)
	if err != nil {
		return &DeliveryResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to write file: %v", err),
			DurationMs:   int64(time.Since(startTime).Milliseconds()),
		}, err
	}

	duration := time.Since(startTime)
	log.Printf("✅ File Writer: Successfully wrote %s in %dms", filename, duration.Milliseconds())

	return &DeliveryResult{
		Success:        true,
		Acknowledgment: fmt.Sprintf("File written: %s", filename),
		DurationMs:     int64(duration.Milliseconds()),
		Metadata: map[string]interface{}{
			"file_path": filePath,
			"file_name": filename,
			"file_size": len(message.Content),
		},
	}, nil
}

// SendBatch is not supported by file writer (each message = one file)
func (f *FileWriterConnector) SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error) {
	results := make([]*DeliveryResult, len(messages))
	for i, msg := range messages {
		result, err := f.Send(ctx, msg)
		if err != nil {
			result = &DeliveryResult{
				Success:      false,
				ErrorMessage: err.Error(),
			}
		}
		results[i] = result
	}
	return results, nil
}

// SupportsBatch returns false - file writer processes messages individually
func (f *FileWriterConnector) SupportsBatch() bool {
	return false
}

// generateFilename creates a filename from the pattern
func (f *FileWriterConnector) generateFilename(message *models.OutboundMessage) string {
	filename := f.filenamePattern

	// Replace placeholders
	replacements := map[string]string{
		"{timestamp}":  time.Now().Format("20060102_150405"),
		"{date}":       time.Now().Format("20060102"),
		"{time}":       time.Now().Format("150405"),
		"{message_id}": f.sanitizeFilename(message.MessageID),
		"{interface_id}": f.sanitizeFilename(message.InterfaceID),
	}

	for placeholder, value := range replacements {
		filename = replaceAll(filename, placeholder, value)
	}

	// If no extension, try to detect from content type
	if filepath.Ext(filename) == "" {
		ext := f.getExtensionFromContentType(message.ContentType)
		filename = filename + ext
	}

	return filename
}

// sanitizeFilename removes invalid characters from filenames
func (f *FileWriterConnector) sanitizeFilename(s string) string {
	// Replace invalid characters with underscores
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := s
	for _, char := range invalid {
		result = replaceAll(result, char, "_")
	}
	return result
}

// getExtensionFromContentType returns file extension based on content type
func (f *FileWriterConnector) getExtensionFromContentType(contentType string) string {
	switch contentType {
	case "x-application/hl7-v2+er7":
		return ".hl7"
	case "application/fhir+json", "application/json":
		return ".json"
	case "application/xml", "application/fhir+xml":
		return ".xml"
	case "text/plain":
		return ".txt"
	default:
		return ".dat"
	}
}

// replaceAll is a simple string replacement helper
func replaceAll(s, old, new string) string {
	result := ""
	for i := 0; i < len(s); {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			result += new
			i += len(old)
		} else {
			result += string(s[i])
			i++
		}
	}
	return result
}
