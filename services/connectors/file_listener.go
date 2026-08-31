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
	createDirs       bool   // auto-create directory if it does not exist
	messageChan      chan<- *models.InboundMessage
	stopChan         chan struct{}
	mu               sync.RWMutex
	processedFiles   map[string]time.Time // track processed files to avoid duplicates
	pendingSizes     map[string]int64     // path -> size observed on the previous poll, for stability checks
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
		pendingSizes:         make(map[string]int64),
		stopChan:             make(chan struct{}),
	}
}

// ExtractFileListenerDirConfig pulls directory_path/file_pattern/recursive out of a
// connector.inbound config map, applying the same snake_case/camelCase key fallback
// as Initialize(). ok is false when no directory_path is configured. Pure and
// side-effect-free (no logging, no I/O) so it can double as a read-only pre-check
// (e.g. the engine's directory/pattern conflict scan) without side effects.
func ExtractFileListenerDirConfig(cfg map[string]interface{}) (dirPath, pattern string, recursive bool, ok bool) {
	if path, isOk := cfg["directory_path"].(string); isOk && path != "" {
		dirPath = path
	} else if path, isOk := cfg["directoryPath"].(string); isOk && path != "" {
		dirPath = path
	} else {
		return "", "", false, false
	}

	pattern = "*"
	if p, isOk := cfg["file_pattern"].(string); isOk && p != "" {
		pattern = p
	} else if p, isOk := cfg["filePattern"].(string); isOk && p != "" {
		pattern = p
	}

	if r, isOk := cfg["recursive"].(bool); isOk {
		recursive = r
	}

	return dirPath, pattern, recursive, true
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

	dirPath, pattern, recursive, ok := ExtractFileListenerDirConfig(cfg)
	if !ok {
		return fmt.Errorf("directory_path is required")
	}
	f.directoryPath = dirPath
	f.filePattern = pattern
	f.recursive = recursive
	log.Printf("🔍 Directory path set to: %s", f.directoryPath)
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

	// Encoding — "file_encoding" is a legacy key name from before the
	// connectivity_types schema was corrected to match this field; kept as a
	// fallback so the one real interface saved under the old name keeps working.
	if enc, ok := cfg["encoding"].(string); ok && enc != "" {
		f.encoding = enc
	} else if enc, ok := cfg["file_encoding"].(string); ok && enc != "" {
		f.encoding = enc
	}

	// Auto-create directory
	if cd, ok := cfg["create_dirs"].(bool); ok {
		f.createDirs = cd
	} else if cd, ok := cfg["createDirs"].(bool); ok {
		f.createDirs = cd
	}

	// Validate / create directory
	if _, err := os.Stat(f.directoryPath); os.IsNotExist(err) {
		if f.createDirs {
			if mkErr := os.MkdirAll(f.directoryPath, 0755); mkErr != nil {
				return fmt.Errorf("failed to create directory: %w", mkErr)
			}
			log.Printf("📁 File Listener: created directory %s", f.directoryPath)
		} else {
			return fmt.Errorf("directory does not exist: %s", f.directoryPath)
		}
	}

	// Create archive directory if needed
	if (f.afterProcessing == "move" || f.afterProcessing == "archive") && f.archivePath != "" {
		if err := os.MkdirAll(f.archivePath, 0755); err != nil {
			return fmt.Errorf("failed to create archive directory: %w", err)
		}
	}

	log.Printf("✅ File Listener initialized: path=%s, pattern=%s, polling=%v, after=%s",
		f.directoryPath, f.filePattern, f.pollingInterval, f.afterProcessing)
	// Note: not also logged via f.Logger() here — SetInterfaceContext runs
	// after Initialize returns (see the Connector interface doc comment), so
	// at this point Logger() would still resolve to the global fallback.
	// From Start() onward (below), SetInterfaceContext has already run.

	// Mark as initialized for base connector validation
	f.BaseInboundConnector.BaseConnector.initialized = true

	return nil
}

// TestConnection verifies the watch directory is accessible and readable.
func (f *FileListenerConnector) TestConnection(ctx context.Context) error {
	f.mu.RLock()
	dir := f.directoryPath
	f.mu.RUnlock()

	if dir == "" {
		return fmt.Errorf("directory_path not configured")
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", dir)
		}
		return fmt.Errorf("cannot access directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", dir)
	}
	// Verify we can list the directory (read permission check)
	if _, err := os.ReadDir(dir); err != nil {
		return fmt.Errorf("directory not readable: %s: %w", dir, err)
	}
	log.Printf("✅ File Listener TestConnection: directory %s is accessible", dir)
	return nil
}

// Start begins polling the directory for files
func (f *FileListenerConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	f.mu.Lock()
	f.messageChan = messageChan
	f.mu.Unlock()

	log.Printf("📂 File Listener: Polling directory %s (pattern: %s)", f.directoryPath, f.filePattern)
	f.Logger().Info("polling directory",
		"directory_path", f.directoryPath,
		"file_pattern", f.filePattern,
		"recursive", f.recursive,
		"polling_interval", f.pollingInterval.String(),
	)

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
		// filepath.Walk uses Lstat internally, so info here already reflects
		// symlinks without following them — safe to check ModeSymlink directly.
		err = filepath.Walk(f.directoryPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				log.Printf("⚠️  File Listener: Skipping symlink %s (not followed — see path traversal note in afterProcessFile)", path)
				return nil
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
		var globbed []string
		globbed, err = filepath.Glob(pattern)
		if err == nil {
			// filepath.Glob doesn't report file mode — Lstat each match explicitly
			// so a symlink dropped into the watched directory (e.g. by an SFTP
			// upload) can't be used to read an arbitrary file outside the tree.
			for _, path := range globbed {
				if info, statErr := os.Lstat(path); statErr == nil {
					if info.Mode()&os.ModeSymlink != 0 {
						log.Printf("⚠️  File Listener: Skipping symlink %s", path)
						continue
					}
				}
				files = append(files, path)
			}
		}
	}

	if err != nil {
		log.Printf("❌ File Listener: Error scanning directory: %v", err)
		return
	}

	log.Printf("📂 File Listener: Found %d matching files", len(files))

	for _, filePath := range files {
		if f.isFileStable(filePath) {
			f.processFile(filePath)
		}
	}
}

// isFileStable reports whether filePath's size is unchanged since the
// previous poll cycle, requiring two consecutive polls to observe the SAME
// size before a file is ever handed to processFile. Closes a real race
// condition: a file dropped by an upstream writer (e.g. an EHR export job,
// or an atomic-rename-from-a-"_tmp"-name pattern) can be picked up by
// scanDirectory mid-write, before the writer has finished. Confirmed against
// a real incident: a file named "..._tmp.xml" was read at 0 bytes on first
// sight and sent downstream, where it failed with "no parser available for
// format: xml" since there was nothing in it to detect a format from.
//
// A file that stabilizes at 0 bytes is still skipped (logged, not silently
// dropped) and marked processed so it isn't re-checked forever -- a genuinely
// empty file has nothing useful to send downstream either way.
func (f *FileListenerConnector) isFileStable(filePath string) bool {
	info, err := os.Stat(filePath)
	if err != nil {
		// Vanished between scan and stat (e.g. already consumed by another
		// process) -- forget any prior size record so a future file at this
		// same path starts its own stability check from scratch.
		f.mu.Lock()
		delete(f.pendingSizes, filePath)
		f.mu.Unlock()
		return false
	}
	size := info.Size()

	f.mu.Lock()
	defer f.mu.Unlock()

	lastSize, seen := f.pendingSizes[filePath]
	if !seen {
		f.pendingSizes[filePath] = size
		log.Printf("⏳ File Listener: %s detected (%d bytes), waiting for size to stabilize before reading", filePath, size)
		return false
	}
	if lastSize != size {
		f.pendingSizes[filePath] = size
		log.Printf("⏳ File Listener: %s still being written (%d -> %d bytes), waiting for next poll", filePath, lastSize, size)
		return false
	}
	delete(f.pendingSizes, filePath)
	if size == 0 {
		log.Printf("⚠️  File Listener: %s is stable but 0 bytes -- skipping (never populated or write failed)", filePath)
		f.processedFiles[filePath] = time.Now()
		return false
	}
	return true
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

	// Re-check for a symlink right before reading — closes the TOCTOU window between
	// scanDirectory's check and this read (e.g. a regular file swapped for a symlink
	// pointing outside the watched directory between scan and processing).
	if info, statErr := os.Lstat(filePath); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		log.Printf("🚨 File Listener: Refusing to read %s — became a symlink after being queued", filePath)
		return
	}

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
		f.Logger().Info("file queued for processing",
			"file_name", fileName,
			"file_path", filePath,
			"file_size", len(content),
			"message_id", message.MessageID,
		)

		// Apply after-processing action
		if err := f.afterProcessFile(filePath); err != nil {
			log.Printf("⚠️ File Listener: After-processing failed for %s: %v", fileName, err)
			f.Logger().Warn("after-processing failed", "file_name", fileName, "error", err.Error())
		}
	default:
		log.Printf("⚠️ File Listener: Message channel full, skipping file %s", fileName)
		f.Logger().Warn("message channel full, skipping file", "file_name", fileName)
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
		// filepath.Base already strips directory components from fileName, but an
		// unusual on-disk name (e.g. literally "..") could still resolve outside
		// archivePath once joined — verify containment before ever calling Rename.
		fileName := filepath.Base(filePath)
		destPath := filepath.Join(f.archivePath, fileName)
		if !isWithinDir(f.archivePath, destPath) {
			return fmt.Errorf("refusing to move %q: resolved destination %q escapes archive_path %q",
				filePath, destPath, f.archivePath)
		}

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

// isWithinDir reports whether target resolves to a path inside dir (or dir itself),
// after cleaning both to absolute paths. Used to verify a joined destination path
// hasn't escaped its intended parent directory before a move/rename touches disk.
func isWithinDir(dir, target string) bool {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absTarget)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
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
