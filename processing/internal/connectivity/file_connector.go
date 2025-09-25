// internal/connectivity/file_connector.go
// File system connectivity handlers

package connectivity

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/processing/pkg"
	"github.com/google/uuid"
)

// FileInputConnector handles file system message monitoring
type FileInputConnector struct {
	*BaseConnector
	watchDir       string
	processedDir   string
	errorDir       string
	messageChan    chan<- *pkg.UniversalMessage
	fileWatcher    *FileWatcher
	filePatterns   []string
	recursive      bool
	deleteAfterRead bool
}

// FileOutputConnector handles file system message writing
type FileOutputConnector struct {
	*BaseConnector
	outputDir      string
	tempDir        string
	fileTemplate   string
	createDirs     bool
	fileMode       os.FileMode
	compression    bool
}

// FileWatcher monitors file system changes
type FileWatcher struct {
	watchDir     string
	pollInterval time.Duration
	patterns     []string
	recursive    bool
	lastScan     time.Time
	stopChan     chan bool
	mutex        sync.RWMutex
	processedFiles map[string]time.Time
}

// FileMessage represents a file-based message
type FileMessage struct {
	FileName     string                 `json:"fileName"`
	FilePath     string                 `json:"filePath"`
	FileSize     int64                  `json:"fileSize"`
	ModTime      time.Time              `json:"modTime"`
	ContentType  string                 `json:"contentType"`
	Content      string                 `json:"content"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// NewFileInputConnector creates a new file input connector
func NewFileInputConnector(config pkg.ConnectorConfig) (*FileInputConnector, error) {
	base := NewBaseConnector(config, "file_input")

	watchDir := config.Endpoint
	if watchDir == "" {
		return nil, fmt.Errorf("watch directory is required")
	}

	// Parse configuration
	processedDir := filepath.Join(watchDir, "processed")
	errorDir := filepath.Join(watchDir, "error")

	if procDir, exists := config.Settings["processed_dir"]; exists {
		if pd, ok := procDir.(string); ok {
			processedDir = pd
		}
	}

	if errDir, exists := config.Settings["error_dir"]; exists {
		if ed, ok := errDir.(string); ok {
			errorDir = ed
		}
	}

	filePatterns := []string{"*"}
	if patterns, exists := config.Settings["file_patterns"]; exists {
		if patternSlice, ok := patterns.([]interface{}); ok {
			filePatterns = make([]string, len(patternSlice))
			for i, p := range patternSlice {
				if pattern, ok := p.(string); ok {
					filePatterns[i] = pattern
				}
			}
		} else if pattern, ok := patterns.(string); ok {
			filePatterns = []string{pattern}
		}
	}

	recursive := false
	if rec, exists := config.Settings["recursive"]; exists {
		if r, ok := rec.(bool); ok {
			recursive = r
		}
	}

	deleteAfterRead := false
	if del, exists := config.Settings["delete_after_read"]; exists {
		if d, ok := del.(bool); ok {
			deleteAfterRead = d
		}
	}

	pollInterval := 5 * time.Second
	if interval, exists := config.Settings["poll_interval"]; exists {
		if intervalStr, ok := interval.(string); ok {
			if parsed, err := time.ParseDuration(intervalStr); err == nil {
				pollInterval = parsed
			}
		}
	}

	// Create file watcher
	fileWatcher := &FileWatcher{
		watchDir:     watchDir,
		pollInterval: pollInterval,
		patterns:     filePatterns,
		recursive:    recursive,
		stopChan:     make(chan bool),
		processedFiles: make(map[string]time.Time),
	}

	connector := &FileInputConnector{
		BaseConnector:   base,
		watchDir:        watchDir,
		processedDir:    processedDir,
		errorDir:        errorDir,
		fileWatcher:     fileWatcher,
		filePatterns:    filePatterns,
		recursive:       recursive,
		deleteAfterRead: deleteAfterRead,
	}

	return connector, nil
}

// NewFileOutputConnector creates a new file output connector
func NewFileOutputConnector(config pkg.ConnectorConfig) (*FileOutputConnector, error) {
	base := NewBaseConnector(config, "file_output")

	outputDir := config.Endpoint
	if outputDir == "" {
		return nil, fmt.Errorf("output directory is required")
	}

	tempDir := filepath.Join(outputDir, "temp")
	if tmpDir, exists := config.Settings["temp_dir"]; exists {
		if td, ok := tmpDir.(string); ok {
			tempDir = td
		}
	}

	fileTemplate := "message_{{.ID}}_{{.Timestamp}}.{{.Extension}}"
	if template, exists := config.Settings["file_template"]; exists {
		if ft, ok := template.(string); ok {
			fileTemplate = ft
		}
	}

	createDirs := true
	if create, exists := config.Settings["create_dirs"]; exists {
		if cd, ok := create.(bool); ok {
			createDirs = cd
		}
	}

	fileMode := os.FileMode(0644)
	if mode, exists := config.Settings["file_mode"]; exists {
		if modeInt, ok := mode.(float64); ok {
			fileMode = os.FileMode(modeInt)
		}
	}

	compression := false
	if comp, exists := config.Settings["compression"]; exists {
		if c, ok := comp.(bool); ok {
			compression = c
		}
	}

	connector := &FileOutputConnector{
		BaseConnector: base,
		outputDir:     outputDir,
		tempDir:       tempDir,
		fileTemplate:  fileTemplate,
		createDirs:    createDirs,
		fileMode:      fileMode,
		compression:   compression,
	}

	return connector, nil
}

// Connect prepares the file system for operations
func (fc *FileInputConnector) Connect() error {
	// Check if watch directory exists
	if _, err := os.Stat(fc.watchDir); os.IsNotExist(err) {
		return fmt.Errorf("watch directory does not exist: %s", fc.watchDir)
	}

	// Create processed and error directories if they don't exist
	if err := os.MkdirAll(fc.processedDir, 0755); err != nil {
		return fmt.Errorf("failed to create processed directory: %w", err)
	}

	if err := os.MkdirAll(fc.errorDir, 0755); err != nil {
		return fmt.Errorf("failed to create error directory: %w", err)
	}

	fc.SetConnected(true)
	return nil
}

// Disconnect stops file system operations
func (fc *FileInputConnector) Disconnect() error {
	fc.SetConnected(false)
	return nil
}

// TestConnection tests file system access
func (fc *FileInputConnector) TestConnection() error {
	// Test read access to watch directory
	if _, err := os.Open(fc.watchDir); err != nil {
		return fmt.Errorf("cannot access watch directory: %w", err)
	}

	// Test write access for processed directory
	testFile := filepath.Join(fc.processedDir, ".test_write")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("cannot write to processed directory: %w", err)
	}
	os.Remove(testFile)

	return nil
}

// Connect prepares the output directory
func (fc *FileOutputConnector) Connect() error {
	if fc.createDirs {
		if err := os.MkdirAll(fc.outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}

		if err := os.MkdirAll(fc.tempDir, 0755); err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
	}

	fc.SetConnected(true)
	return nil
}

// Disconnect is a no-op for file output
func (fc *FileOutputConnector) Disconnect() error {
	fc.SetConnected(false)
	return nil
}

// TestConnection tests file output capabilities
func (fc *FileOutputConnector) TestConnection() error {
	// Test write access to output directory
	testFile := filepath.Join(fc.outputDir, ".test_write")
	if err := os.WriteFile(testFile, []byte("test"), fc.fileMode); err != nil {
		return fmt.Errorf("cannot write to output directory: %w", err)
	}
	os.Remove(testFile)

	return nil
}

// StartListening begins file system monitoring
func (fc *FileInputConnector) StartListening(messageChan chan<- *pkg.UniversalMessage) error {
	if err := fc.Start(fc.ctx); err != nil {
		return err
	}

	if !fc.IsConnected() {
		if err := fc.Connect(); err != nil {
			return err
		}
	}

	fc.messageChan = messageChan

	// Start file watcher
	go fc.fileWatcher.Start(fc.Context(), fc.handleFileEvent)

	return nil
}

// StopListening stops file system monitoring
func (fc *FileInputConnector) StopListening() error {
	fc.fileWatcher.Stop()
	return fc.Stop()
}

// SendMessage writes a message to a file
func (fc *FileOutputConnector) SendMessage(ctx context.Context, message *pkg.UniversalMessage) error {
	if !fc.IsConnected() {
		if err := fc.Connect(); err != nil {
			return err
		}
	}

	startTime := time.Now()

	// Generate filename
	fileName, err := fc.generateFileName(message)
	if err != nil {
		fc.RecordError(err)
		return fmt.Errorf("failed to generate filename: %w", err)
	}

	filePath := filepath.Join(fc.outputDir, fileName)
	tempPath := filepath.Join(fc.tempDir, fileName+".tmp")

	// Write to temporary file first
	content := message.Content
	if fc.compression {
		// TODO: Implement compression
	}

	if err := os.WriteFile(tempPath, []byte(content), fc.fileMode); err != nil {
		fc.RecordError(err)
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Move from temp to final location (atomic operation)
	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath) // Clean up temp file
		fc.RecordError(err)
		return fmt.Errorf("failed to move file to final location: %w", err)
	}

	// Update message status
	message.Status = pkg.StatusDelivered
	now := time.Now()
	message.DeliveredAt = &now
	message.Metadata["output_file_path"] = filePath
	message.Metadata["output_file_size"] = len(content)

	// Record metrics
	latency := time.Since(startTime).Milliseconds()
	fc.RecordMessage(int64(len(content)), latency)

	return nil
}

// SendBatch writes multiple messages to files
func (fc *FileOutputConnector) SendBatch(ctx context.Context, messages []*pkg.UniversalMessage) error {
	for _, message := range messages {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := fc.SendMessage(ctx, message); err != nil {
				return err
			}
		}
	}
	return nil
}

// SupportsAcknowledgment returns false for file connectors
func (fc *FileOutputConnector) SupportsAcknowledgment() bool {
	return false
}

// WaitForAcknowledgment is not applicable for file connectors
func (fc *FileOutputConnector) WaitForAcknowledgment(messageID string, timeout time.Duration) error {
	return nil
}

// handleFileEvent processes a file system event
func (fc *FileInputConnector) handleFileEvent(filePath string, fileInfo os.FileInfo) {
	startTime := time.Now()

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		fc.RecordError(err)
		fc.moveToErrorDir(filePath, fmt.Sprintf("read_error_%d", time.Now().Unix()))
		return
	}

	// Detect content type
	contentType := fc.detectContentType(filePath, string(content))

	// Create universal message
	message := pkg.NewUniversalMessage()
	message.Content = string(content)
	message.ContentType = contentType
	message.SourceProtocol = string(fc.Protocol)
	message.SourceEndpoint = fc.watchDir
	message.Size = int64(len(content))

	// Add file metadata
	message.Metadata["file_name"] = fileInfo.Name()
	message.Metadata["file_path"] = filePath
	message.Metadata["file_size"] = fileInfo.Size()
	message.Metadata["file_mod_time"] = fileInfo.ModTime()

	// Send to processing channel
	select {
	case fc.messageChan <- message:
		// Success - move file to processed directory
		if fc.deleteAfterRead {
			os.Remove(filePath)
		} else {
			fc.moveToProcessedDir(filePath, fileInfo.Name())
		}

		// Record metrics
		latency := time.Since(startTime).Milliseconds()
		fc.RecordMessage(message.Size, latency)

	case <-time.After(5 * time.Second):
		// Channel full - move to error directory
		fc.moveToErrorDir(filePath, fmt.Sprintf("queue_full_%s", fileInfo.Name()))
	}
}

// moveToProcessedDir moves a file to the processed directory
func (fc *FileInputConnector) moveToProcessedDir(filePath, fileName string) {
	processedPath := filepath.Join(fc.processedDir, fileName)

	// Handle name conflicts
	counter := 1
	for {
		if _, err := os.Stat(processedPath); os.IsNotExist(err) {
			break
		}
		ext := filepath.Ext(fileName)
		name := strings.TrimSuffix(fileName, ext)
		processedPath = filepath.Join(fc.processedDir, fmt.Sprintf("%s_%d%s", name, counter, ext))
		counter++
	}

	if err := os.Rename(filePath, processedPath); err != nil {
		fc.RecordError(fmt.Errorf("failed to move file to processed directory: %w", err))
	}
}

// moveToErrorDir moves a file to the error directory
func (fc *FileInputConnector) moveToErrorDir(filePath, fileName string) {
	errorPath := filepath.Join(fc.errorDir, fileName)

	if err := os.Rename(filePath, errorPath); err != nil {
		fc.RecordError(fmt.Errorf("failed to move file to error directory: %w", err))
	}
}

// detectContentType attempts to detect the content type of a file
func (fc *FileInputConnector) detectContentType(filePath, content string) string {
	// Check file extension first
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".hl7", ".txt":
		if strings.HasPrefix(content, "MSH|") {
			return "HL7"
		}
		return "TEXT"
	case ".json":
		return "JSON"
	case ".xml":
		return "XML"
	case ".csv":
		return "CSV"
	case ".fhir":
		return "FHIR"
	}

	// Content-based detection
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "MSH|") {
		return "HL7"
	}
	if strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[") {
		return "JSON"
	}
	if strings.HasPrefix(content, "<") {
		return "XML"
	}

	return "UNKNOWN"
}

// generateFileName generates a filename based on the template
func (fc *FileOutputConnector) generateFileName(message *pkg.UniversalMessage) (string, error) {
	template := fc.fileTemplate

	// Simple template replacement
	replacements := map[string]string{
		"{{.ID}}":          message.ID,
		"{{.CorrelationID}}": message.CorrelationID,
		"{{.Timestamp}}":   time.Now().Format("20060102_150405"),
		"{{.Date}}":        time.Now().Format("20060102"),
		"{{.Time}}":        time.Now().Format("150405"),
		"{{.Extension}}":   fc.getExtensionForContentType(message.ContentType),
	}

	fileName := template
	for placeholder, value := range replacements {
		fileName = strings.ReplaceAll(fileName, placeholder, value)
	}

	// Ensure filename is valid
	fileName = strings.ReplaceAll(fileName, "/", "_")
	fileName = strings.ReplaceAll(fileName, "\\", "_")
	fileName = strings.ReplaceAll(fileName, ":", "_")

	return fileName, nil
}

// getExtensionForContentType returns file extension for content type
func (fc *FileOutputConnector) getExtensionForContentType(contentType string) string {
	switch strings.ToUpper(contentType) {
	case "HL7":
		return "hl7"
	case "FHIR", "JSON":
		return "json"
	case "XML":
		return "xml"
	case "CSV":
		return "csv"
	default:
		return "txt"
	}
}

// FileWatcher methods

// Start begins file system monitoring
func (fw *FileWatcher) Start(ctx context.Context, handler func(string, os.FileInfo)) {
	fw.lastScan = time.Now()
	ticker := time.NewTicker(fw.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-fw.stopChan:
			return
		case <-ticker.C:
			fw.scanForFiles(handler)
		}
	}
}

// Stop stops file system monitoring
func (fw *FileWatcher) Stop() {
	select {
	case fw.stopChan <- true:
	default:
	}
}

// scanForFiles scans for new or modified files
func (fw *FileWatcher) scanForFiles(handler func(string, os.FileInfo)) {
	scanTime := time.Now()

	err := filepath.WalkDir(fw.watchDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			if !fw.recursive && path != fw.watchDir {
				return filepath.SkipDir
			}
			return nil
		}

		// Check file patterns
		if !fw.matchesPattern(d.Name()) {
			return nil
		}

		// Get file info
		fileInfo, err := d.Info()
		if err != nil {
			return nil
		}

		// Check if file is new or modified since last scan
		fw.mutex.RLock()
		lastProcessed, exists := fw.processedFiles[path]
		fw.mutex.RUnlock()

		if !exists || fileInfo.ModTime().After(lastProcessed) {
			// Process file
			handler(path, fileInfo)

			// Update processed files map
			fw.mutex.Lock()
			fw.processedFiles[path] = scanTime
			fw.mutex.Unlock()
		}

		return nil
	})

	if err != nil {
		// Handle walk error
	}

	fw.lastScan = scanTime

	// Clean up old entries from processed files map
	fw.cleanupProcessedFiles()
}

// matchesPattern checks if filename matches any of the patterns
func (fw *FileWatcher) matchesPattern(fileName string) bool {
	for _, pattern := range fw.patterns {
		if matched, _ := filepath.Match(pattern, fileName); matched {
			return true
		}
	}
	return false
}

// cleanupProcessedFiles removes old entries from processed files map
func (fw *FileWatcher) cleanupProcessedFiles() {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour) // Keep entries for 24 hours

	for path, timestamp := range fw.processedFiles {
		if timestamp.Before(cutoff) {
			delete(fw.processedFiles, path)
		}
	}
}