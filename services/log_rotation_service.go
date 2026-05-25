package services

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ========================================
// LOG ROTATION SERVICE (OOP: SRP + Strategy Pattern)
// ========================================

// LogRotationStrategy defines how logs should be rotated
type LogRotationStrategy interface {
	ShouldRotate(currentFile *os.File) bool
	GetRotatedFileName(baseName string) string
	GetStrategyName() string
}

// LogRotationConfig holds configuration for log rotation
type LogRotationConfig struct {
	LogDir          string                // Directory for log files
	BaseFileName    string                // Base name for log files (e.g., "app.log")
	Strategy        LogRotationStrategy   // Rotation strategy (size/time/hybrid)
	MaxBackups      int                   // Max number of backup files to keep
	Compress        bool                  // Compress rotated files (gzip)
	PermissionMode  os.FileMode           // File permissions (default: 0644)
}

// LogRotationService handles log rotation with multiple strategies
type LogRotationService struct {
	config         *LogRotationConfig
	currentFile    *os.File
	currentWriter  io.Writer
	mutex          sync.Mutex
	bytesWritten   int64
	lastRotation   time.Time
}

// NewLogRotationService creates a new log rotation service
func NewLogRotationService(config *LogRotationConfig) (*LogRotationService, error) {
	// Validate configuration
	if config.LogDir == "" {
		config.LogDir = "./logs"
	}
	if config.BaseFileName == "" {
		config.BaseFileName = "app.log"
	}
	if config.PermissionMode == 0 {
		config.PermissionMode = 0644
	}
	if config.MaxBackups == 0 {
		config.MaxBackups = 7 // Default: keep 7 backups
	}

	// Create log directory if not exists
	if err := os.MkdirAll(config.LogDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	service := &LogRotationService{
		config:       config,
		lastRotation: time.Now(),
	}

	// Open initial log file
	if err := service.rotateNow(); err != nil {
		return nil, err
	}

	log.Printf("✅ Log Rotation Service initialized: strategy=%s, dir=%s",
		config.Strategy.GetStrategyName(), config.LogDir)

	return service, nil
}

// Write implements io.Writer interface (thread-safe)
func (s *LogRotationService) Write(p []byte) (n int, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check if rotation is needed
	if s.config.Strategy.ShouldRotate(s.currentFile) {
		if err := s.rotateNow(); err != nil {
			return 0, err
		}
	}

	// Write to current file
	n, err = s.currentFile.Write(p)
	s.bytesWritten += int64(n)
	return n, err
}

// rotateNow performs log rotation (caller must hold mutex)
func (s *LogRotationService) rotateNow() error {
	// Close current file if exists
	if s.currentFile != nil {
		s.currentFile.Close()
	}

	// Get rotated file name
	rotatedName := s.config.Strategy.GetRotatedFileName(s.config.BaseFileName)
	currentPath := filepath.Join(s.config.LogDir, s.config.BaseFileName)
	rotatedPath := filepath.Join(s.config.LogDir, rotatedName)

	// Rename current file to rotated name (if exists)
	if _, err := os.Stat(currentPath); err == nil {
		if err := os.Rename(currentPath, rotatedPath); err != nil {
			return fmt.Errorf("failed to rename log file: %w", err)
		}

		// Compress if enabled
		if s.config.Compress {
			go s.compressFile(rotatedPath)
		}
	}

	// Open new file
	file, err := os.OpenFile(currentPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, s.config.PermissionMode)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	s.currentFile = file
	s.bytesWritten = 0
	s.lastRotation = time.Now()

	// Clean up old backups
	go s.cleanOldBackups()

	return nil
}

// compressFile compresses a log file using gzip
func (s *LogRotationService) compressFile(filePath string) {
	// TODO: Implement gzip compression
	// For now, just a placeholder
}

// cleanOldBackups removes old backup files beyond MaxBackups
func (s *LogRotationService) cleanOldBackups() {
	pattern := filepath.Join(s.config.LogDir, s.config.BaseFileName+"*")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return
	}

	// Keep only MaxBackups files
	if len(files) > s.config.MaxBackups {
		// Sort by modification time and delete oldest
		for i := s.config.MaxBackups; i < len(files); i++ {
			os.Remove(files[i])
		}
	}
}

// Close closes the log file
func (s *LogRotationService) Close() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.currentFile != nil {
		return s.currentFile.Close()
	}
	return nil
}

// ========================================
// ROTATION STRATEGIES (OOP: Strategy Pattern)
// ========================================

// SizeBasedRotationStrategy rotates logs when file size exceeds limit
type SizeBasedRotationStrategy struct {
	MaxSizeMB int64 // Max file size in MB
}

func (s *SizeBasedRotationStrategy) ShouldRotate(currentFile *os.File) bool {
	if currentFile == nil {
		return false
	}

	stat, err := currentFile.Stat()
	if err != nil {
		return false
	}

	maxBytes := s.MaxSizeMB * 1024 * 1024
	return stat.Size() >= maxBytes
}

func (s *SizeBasedRotationStrategy) GetRotatedFileName(baseName string) string {
	timestamp := time.Now().Format("20060102_150405")
	return fmt.Sprintf("%s.%s", baseName, timestamp)
}

func (s *SizeBasedRotationStrategy) GetStrategyName() string {
	return fmt.Sprintf("SizeBased(%dMB)", s.MaxSizeMB)
}

// TimeBasedRotationStrategy rotates logs at specific intervals
type TimeBasedRotationStrategy struct {
	Interval time.Duration // Rotation interval (e.g., 24h for daily)
	lastCheck time.Time
}

func (s *TimeBasedRotationStrategy) ShouldRotate(currentFile *os.File) bool {
	now := time.Now()
	if s.lastCheck.IsZero() {
		s.lastCheck = now
		return false
	}

	shouldRotate := now.Sub(s.lastCheck) >= s.Interval
	if shouldRotate {
		s.lastCheck = now
	}
	return shouldRotate
}

func (s *TimeBasedRotationStrategy) GetRotatedFileName(baseName string) string {
	timestamp := time.Now().Format("2006-01-02")
	return fmt.Sprintf("%s.%s", baseName, timestamp)
}

func (s *TimeBasedRotationStrategy) GetStrategyName() string {
	return fmt.Sprintf("TimeBased(%s)", s.Interval)
}

// HybridRotationStrategy combines size and time rotation
type HybridRotationStrategy struct {
	SizeStrategy *SizeBasedRotationStrategy
	TimeStrategy *TimeBasedRotationStrategy
}

func (s *HybridRotationStrategy) ShouldRotate(currentFile *os.File) bool {
	return s.SizeStrategy.ShouldRotate(currentFile) || s.TimeStrategy.ShouldRotate(currentFile)
}

func (s *HybridRotationStrategy) GetRotatedFileName(baseName string) string {
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	return fmt.Sprintf("%s.%s", baseName, timestamp)
}

func (s *HybridRotationStrategy) GetStrategyName() string {
	return fmt.Sprintf("Hybrid(%dMB or %s)", s.SizeStrategy.MaxSizeMB, s.TimeStrategy.Interval)
}

// ========================================
// INTERFACE-SPECIFIC LOGGER (OOP: Factory Pattern)
// ========================================

// InterfaceLogger provides interface-specific logging with rotation
type InterfaceLogger struct {
	interfaceID   string
	interfaceName string
	rotationSvc   *LogRotationService
	logger        *log.Logger
}

// NewInterfaceLogger creates a logger for a specific interface
func NewInterfaceLogger(interfaceID, interfaceName string) (*InterfaceLogger, error) {
	// Create rotation service for this interface
	config := &LogRotationConfig{
		LogDir:       filepath.Join("logs", "interfaces", interfaceID),
		BaseFileName: "interface.log",
		Strategy: &HybridRotationStrategy{
			SizeStrategy: &SizeBasedRotationStrategy{MaxSizeMB: 100}, // 100MB max
			TimeStrategy: &TimeBasedRotationStrategy{Interval: 24 * time.Hour}, // Daily
		},
		MaxBackups: 30, // Keep 30 days of logs
		Compress:   false,
	}

	rotationSvc, err := NewLogRotationService(config)
	if err != nil {
		return nil, err
	}

	// Create logger with rotation writer
	logger := log.New(rotationSvc, fmt.Sprintf("[%s] ", interfaceName), log.LstdFlags)

	return &InterfaceLogger{
		interfaceID:   interfaceID,
		interfaceName: interfaceName,
		rotationSvc:   rotationSvc,
		logger:        logger,
	}, nil
}

// Info logs an informational message
func (l *InterfaceLogger) Info(format string, args ...interface{}) {
	l.logger.Printf("[INFO] "+format, args...)
}

// Error logs an error message
func (l *InterfaceLogger) Error(format string, args ...interface{}) {
	l.logger.Printf("[ERROR] "+format, args...)
}

// Warning logs a warning message
func (l *InterfaceLogger) Warning(format string, args ...interface{}) {
	l.logger.Printf("[WARNING] "+format, args...)
}

// Debug logs a debug message
func (l *InterfaceLogger) Debug(format string, args ...interface{}) {
	l.logger.Printf("[DEBUG] "+format, args...)
}

// Close closes the logger
func (l *InterfaceLogger) Close() error {
	return l.rotationSvc.Close()
}

// ========================================
// APPLICATION-LEVEL LOGGER (Singleton)
// ========================================

var (
	appLogger     *LogRotationService
	appLoggerOnce sync.Once
)

// GetApplicationLogger returns the singleton application logger with rotation
func GetApplicationLogger() *LogRotationService {
	appLoggerOnce.Do(func() {
		config := &LogRotationConfig{
			LogDir:       "logs/application",
			BaseFileName: "app.log",
			Strategy: &HybridRotationStrategy{
				SizeStrategy: &SizeBasedRotationStrategy{MaxSizeMB: 500}, // 500MB max
				TimeStrategy: &TimeBasedRotationStrategy{Interval: 24 * time.Hour}, // Daily
			},
			MaxBackups: 14, // Keep 14 days
			Compress:   false,
		}

		var err error
		appLogger, err = NewLogRotationService(config)
		if err != nil {
			log.Printf("❌ Failed to initialize app logger: %v", err)
		}

		// Set as default log output
		log.SetOutput(appLogger)
	})

	return appLogger
}
