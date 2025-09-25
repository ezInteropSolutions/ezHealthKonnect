// services/universal_message.go
// Universal Message Container for ezHealthKonnect Interface Engine
//
// 🎯 PURPOSE: Unified message container supporting all message types (HL7, FHIR, JSON, XML, Binary)
// with complete transformation tracking, lineage, and processing history
package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// =====================================
// UNIVERSAL MESSAGE TYPES
// =====================================

// MessageType defines supported message formats
type MessageType string

const (
	MessageTypeHL7     MessageType = "HL7"
	MessageTypeFHIR    MessageType = "FHIR"
	MessageTypeJSON    MessageType = "JSON"
	MessageTypeXML     MessageType = "XML"
	MessageTypeBinary  MessageType = "BINARY"
	MessageTypeUnknown MessageType = "UNKNOWN"
)

// MessageStatus defines processing states
type MessageStatus string

const (
	StatusReceived     MessageStatus = "RECEIVED"
	StatusParsing      MessageStatus = "PARSING"
	StatusParsed       MessageStatus = "PARSED"
	StatusValidating   MessageStatus = "VALIDATING"
	StatusValidated    MessageStatus = "VALIDATED"
	StatusTransforming MessageStatus = "TRANSFORMING"
	StatusTransformed  MessageStatus = "TRANSFORMED"
	StatusRouting      MessageStatus = "ROUTING"
	StatusDelivering   MessageStatus = "DELIVERING"
	StatusDelivered    MessageStatus = "DELIVERED"
	StatusFailed       MessageStatus = "FAILED"
	StatusRetry        MessageStatus = "RETRY"
)

// Priority levels for message processing
type MessagePriority int

const (
	PriorityLow    MessagePriority = 1
	PriorityNormal MessagePriority = 5
	PriorityHigh   MessagePriority = 8
	PriorityUrgent MessagePriority = 10
)

// =====================================
// TRANSFORMATION TRACKING
// =====================================

// TransformationRecord tracks individual transformation steps
type TransformationRecord struct {
	ID               string                 `json:"id" db:"id"`
	TransformationID uuid.UUID              `json:"transformationId" db:"transformation_id"`
	StepNumber       int                    `json:"stepNumber" db:"step_number"`
	ServiceName      string                 `json:"serviceName" db:"service_name"`
	InputType        MessageType            `json:"inputType" db:"input_type"`
	OutputType       MessageType            `json:"outputType" db:"output_type"`
	StartTime        time.Time              `json:"startTime" db:"start_time"`
	EndTime          *time.Time             `json:"endTime,omitempty" db:"end_time"`
	Duration         time.Duration          `json:"duration" db:"duration"`
	Status           string                 `json:"status" db:"status"`
	Success          bool                   `json:"success" db:"success"`
	ErrorMessage     string                 `json:"errorMessage,omitempty" db:"error_message"`
	WarningCount     int                    `json:"warningCount" db:"warning_count"`
	InputSize        int64                  `json:"inputSize" db:"input_size"`
	OutputSize       int64                  `json:"outputSize" db:"output_size"`
	Metadata         map[string]interface{} `json:"metadata" db:"metadata"`
	Configuration    map[string]interface{} `json:"configuration" db:"configuration"`
}

// TransformationLineage tracks the complete transformation chain
type TransformationLineage struct {
	ID                string                 `json:"id" db:"id"`
	MessageID         string                 `json:"messageId" db:"message_id"`
	SourceFormat      MessageType            `json:"sourceFormat" db:"source_format"`
	TargetFormat      MessageType            `json:"targetFormat" db:"target_format"`
	PipelineID        string                 `json:"pipelineId" db:"pipeline_id"`
	StartTime         time.Time              `json:"startTime" db:"start_time"`
	EndTime           *time.Time             `json:"endTime,omitempty" db:"end_time"`
	TotalDuration     time.Duration          `json:"totalDuration" db:"total_duration"`
	TotalSteps        int                    `json:"totalSteps" db:"total_steps"`
	SuccessfulSteps   int                    `json:"successfulSteps" db:"successful_steps"`
	FailedSteps       int                    `json:"failedSteps" db:"failed_steps"`
	OverallStatus     string                 `json:"overallStatus" db:"overall_status"`
	Records           []TransformationRecord `json:"records"`
	Rollbacks         []RollbackRecord       `json:"rollbacks,omitempty"`
	QualityMetrics    QualityMetrics         `json:"qualityMetrics" db:"quality_metrics"`
	PerformanceStats  PerformanceStats       `json:"performanceStats" db:"performance_stats"`
}

// RollbackRecord tracks rollback operations
type RollbackRecord struct {
	ID            string                 `json:"id" db:"id"`
	LineageID     string                 `json:"lineageId" db:"lineage_id"`
	RollbackTime  time.Time              `json:"rollbackTime" db:"rollback_time"`
	Reason        string                 `json:"reason" db:"reason"`
	StepsRolledBack int                  `json:"stepsRolledBack" db:"steps_rolled_back"`
	Success       bool                   `json:"success" db:"success"`
	ErrorMessage  string                 `json:"errorMessage,omitempty" db:"error_message"`
	Metadata      map[string]interface{} `json:"metadata" db:"metadata"`
}

// QualityMetrics tracks transformation quality
type QualityMetrics struct {
	DataIntegrity     float64            `json:"dataIntegrity" db:"data_integrity"`
	FieldCompleteness float64            `json:"fieldCompleteness" db:"field_completeness"`
	ValidationScore   float64            `json:"validationScore" db:"validation_score"`
	SemanticAccuracy  float64            `json:"semanticAccuracy" db:"semantic_accuracy"`
	Issues            []QualityIssue     `json:"issues"`
	Scores            map[string]float64 `json:"scores" db:"scores"`
}

// QualityIssue represents data quality problems
type QualityIssue struct {
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Field       string `json:"field,omitempty"`
	Description string `json:"description"`
	Impact      string `json:"impact"`
}

// PerformanceStats tracks transformation performance
type PerformanceStats struct {
	ThroughputMsgPerSec float64           `json:"throughputMsgPerSec" db:"throughput_msg_per_sec"`
	MemoryUsageMB       float64           `json:"memoryUsageMB" db:"memory_usage_mb"`
	CPUUtilization      float64           `json:"cpuUtilization" db:"cpu_utilization"`
	IOOperations        int64             `json:"ioOperations" db:"io_operations"`
	NetworkBytes        int64             `json:"networkBytes" db:"network_bytes"`
	CacheHitRatio       float64           `json:"cacheHitRatio" db:"cache_hit_ratio"`
	Metrics             map[string]float64 `json:"metrics" db:"metrics"`
}

// =====================================
// UNIVERSAL MESSAGE CONTAINER
// =====================================

// UniversalMessage is the core container for all message types with complete tracking
type UniversalMessage struct {
	// Core Identity
	ID            string      `json:"id" db:"id"`
	CorrelationID string      `json:"correlationId" db:"correlation_id"`
	MessageType   MessageType `json:"messageType" db:"message_type"`
	SubType       string      `json:"subType,omitempty" db:"sub_type"`
	Version       string      `json:"version,omitempty" db:"version"`

	// Message Content
	RawContent    []byte                 `json:"rawContent" db:"raw_content"`
	ParsedContent map[string]interface{} `json:"parsedContent" db:"parsed_content"`
	Encoding      string                 `json:"encoding" db:"encoding"`
	ContentSize   int64                  `json:"contentSize" db:"content_size"`
	ChecksumMD5   string                 `json:"checksumMd5" db:"checksum_md5"`
	ChecksumSHA256 string                `json:"checksumSha256" db:"checksum_sha256"`

	// Processing State
	Status        MessageStatus   `json:"status" db:"status"`
	Priority      MessagePriority `json:"priority" db:"priority"`
	ReceivedAt    time.Time       `json:"receivedAt" db:"received_at"`
	ProcessedAt   *time.Time      `json:"processedAt,omitempty" db:"processed_at"`
	LastUpdated   time.Time       `json:"lastUpdated" db:"last_updated"`
	RetryCount    int             `json:"retryCount" db:"retry_count"`
	MaxRetries    int             `json:"maxRetries" db:"max_retries"`

	// Source Information
	SourceSystem    string                 `json:"sourceSystem" db:"source_system"`
	SourceInterface string                 `json:"sourceInterface" db:"source_interface"`
	SourceEndpoint  string                 `json:"sourceEndpoint" db:"source_endpoint"`
	SourceIP        string                 `json:"sourceIp" db:"source_ip"`
	SourceMetadata  map[string]interface{} `json:"sourceMetadata" db:"source_metadata"`

	// Target Information
	TargetSystems  []string               `json:"targetSystems" db:"target_systems"`
	TargetFormat   MessageType            `json:"targetFormat,omitempty" db:"target_format"`
	DeliveryMode   string                 `json:"deliveryMode,omitempty" db:"delivery_mode"`
	TargetMetadata map[string]interface{} `json:"targetMetadata" db:"target_metadata"`

	// Transformation Tracking
	TransformationLineage *TransformationLineage `json:"transformationLineage,omitempty" db:"transformation_lineage"`
	CurrentTransformation *TransformationRecord  `json:"currentTransformation,omitempty" db:"current_transformation"`

	// Processing Results
	TransformedContent []TransformedContent `json:"transformedContent,omitempty" db:"transformed_content"`
	ValidationResults  []ValidationResult   `json:"validationResults,omitempty" db:"validation_results"`
	ProcessingErrors   []ProcessingError    `json:"processingErrors,omitempty" db:"processing_errors"`
	ProcessingWarnings []ProcessingWarning  `json:"processingWarnings,omitempty" db:"processing_warnings"`

	// Delivery Tracking
	DeliveryAttempts []DeliveryAttempt      `json:"deliveryAttempts,omitempty" db:"delivery_attempts"`
	DeliveryStatus   string                 `json:"deliveryStatus,omitempty" db:"delivery_status"`
	AcknowledgmentReceived bool             `json:"acknowledgmentReceived" db:"acknowledgment_received"`

	// Audit and Compliance
	AuditTrail    []AuditEntry           `json:"auditTrail" db:"audit_trail"`
	SecurityLevel string                 `json:"securityLevel" db:"security_level"`
	DataClass     string                 `json:"dataClass" db:"data_class"`
	RetentionDate *time.Time             `json:"retentionDate,omitempty" db:"retention_date"`
	ComplianceFlags map[string]bool      `json:"complianceFlags" db:"compliance_flags"`

	// Business Context
	PatientID     string                 `json:"patientId,omitempty" db:"patient_id"`
	EncounterID   string                 `json:"encounterId,omitempty" db:"encounter_id"`
	OrderID       string                 `json:"orderId,omitempty" db:"order_id"`
	BusinessTags  []string               `json:"businessTags" db:"business_tags"`
	CustomFields  map[string]interface{} `json:"customFields" db:"custom_fields"`
}

// =====================================
// SUPPORTING STRUCTURES
// =====================================

// TransformedContent represents output from transformations
type TransformedContent struct {
	ID            string                 `json:"id" db:"id"`
	TargetFormat  MessageType            `json:"targetFormat" db:"target_format"`
	Content       []byte                 `json:"content" db:"content"`
	ParsedContent map[string]interface{} `json:"parsedContent" db:"parsed_content"`
	Size          int64                  `json:"size" db:"size"`
	CreatedAt     time.Time              `json:"createdAt" db:"created_at"`
	TransformID   string                 `json:"transformId" db:"transform_id"`
	Quality       QualityMetrics         `json:"quality" db:"quality"`
	Metadata      map[string]interface{} `json:"metadata" db:"metadata"`
}

// ValidationResult tracks validation outcomes
type ValidationResult struct {
	ID          string                 `json:"id" db:"id"`
	ValidatorID string                 `json:"validatorId" db:"validator_id"`
	IsValid     bool                   `json:"isValid" db:"is_valid"`
	Score       float64                `json:"score" db:"score"`
	Issues      []ValidationIssue      `json:"issues"`
	Timestamp   time.Time              `json:"timestamp" db:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata" db:"metadata"`
}

// ValidationIssue represents validation problems
type ValidationIssue struct {
	Severity    string `json:"severity"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	Path        string `json:"path,omitempty"`
	Line        int    `json:"line,omitempty"`
	Column      int    `json:"column,omitempty"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// ProcessingError tracks transformation errors
type ProcessingError struct {
	ID          string                 `json:"id" db:"id"`
	Stage       string                 `json:"stage" db:"stage"`
	ServiceName string                 `json:"serviceName" db:"service_name"`
	ErrorCode   string                 `json:"errorCode" db:"error_code"`
	Message     string                 `json:"message" db:"message"`
	Details     string                 `json:"details" db:"details"`
	Timestamp   time.Time              `json:"timestamp" db:"timestamp"`
	Fatal       bool                   `json:"fatal" db:"fatal"`
	Recoverable bool                   `json:"recoverable" db:"recoverable"`
	StackTrace  string                 `json:"stackTrace,omitempty" db:"stack_trace"`
	Context     map[string]interface{} `json:"context" db:"context"`
}

// ProcessingWarning tracks non-fatal issues
type ProcessingWarning struct {
	ID          string                 `json:"id" db:"id"`
	Stage       string                 `json:"stage" db:"stage"`
	ServiceName string                 `json:"serviceName" db:"service_name"`
	WarningCode string                 `json:"warningCode" db:"warning_code"`
	Message     string                 `json:"message" db:"message"`
	Impact      string                 `json:"impact" db:"impact"`
	Timestamp   time.Time              `json:"timestamp" db:"timestamp"`
	Context     map[string]interface{} `json:"context" db:"context"`
}

// DeliveryAttempt tracks delivery attempts
type DeliveryAttempt struct {
	ID             string                 `json:"id" db:"id"`
	AttemptNumber  int                    `json:"attemptNumber" db:"attempt_number"`
	Timestamp      time.Time              `json:"timestamp" db:"timestamp"`
	TargetSystem   string                 `json:"targetSystem" db:"target_system"`
	TargetEndpoint string                 `json:"targetEndpoint" db:"target_endpoint"`
	Success        bool                   `json:"success" db:"success"`
	ResponseCode   string                 `json:"responseCode" db:"response_code"`
	ResponseTime   time.Duration          `json:"responseTime" db:"response_time"`
	ErrorMessage   string                 `json:"errorMessage,omitempty" db:"error_message"`
	Metadata       map[string]interface{} `json:"metadata" db:"metadata"`
}

// AuditEntry tracks all message operations for compliance
type AuditEntry struct {
	ID          string                 `json:"id" db:"id"`
	Timestamp   time.Time              `json:"timestamp" db:"timestamp"`
	Action      string                 `json:"action" db:"action"`
	ServiceName string                 `json:"serviceName" db:"service_name"`
	UserID      string                 `json:"userId,omitempty" db:"user_id"`
	IPAddress   string                 `json:"ipAddress,omitempty" db:"ip_address"`
	Details     string                 `json:"details" db:"details"`
	Outcome     string                 `json:"outcome" db:"outcome"`
	Metadata    map[string]interface{} `json:"metadata" db:"metadata"`
}

// =====================================
// UNIVERSAL MESSAGE METHODS
// =====================================

// NewUniversalMessage creates a new universal message container
func NewUniversalMessage(messageType MessageType, rawContent []byte, sourceSystem string) *UniversalMessage {
	now := time.Now()
	messageID := uuid.New().String()
	correlationID := uuid.New().String()

	return &UniversalMessage{
		ID:            messageID,
		CorrelationID: correlationID,
		MessageType:   messageType,
		RawContent:    rawContent,
		ContentSize:   int64(len(rawContent)),
		Status:        StatusReceived,
		Priority:      PriorityNormal,
		ReceivedAt:    now,
		LastUpdated:   now,
		MaxRetries:    3,
		SourceSystem:  sourceSystem,
		Encoding:      detectEncoding(rawContent),
		ChecksumMD5:   calculateMD5(rawContent),
		ChecksumSHA256: calculateSHA256(rawContent),
		AuditTrail:    []AuditEntry{},
		ComplianceFlags: make(map[string]bool),
		CustomFields:  make(map[string]interface{}),
		SourceMetadata: make(map[string]interface{}),
		TargetMetadata: make(map[string]interface{}),
	}
}

// UpdateStatus updates the message status and adds audit entry
func (m *UniversalMessage) UpdateStatus(newStatus MessageStatus, serviceName, details string) {
	oldStatus := m.Status
	m.Status = newStatus
	m.LastUpdated = time.Now()

	// Add audit entry
	auditEntry := AuditEntry{
		ID:          uuid.New().String(),
		Timestamp:   m.LastUpdated,
		Action:      "STATUS_CHANGE",
		ServiceName: serviceName,
		Details:     fmt.Sprintf("Status changed from %s to %s: %s", oldStatus, newStatus, details),
		Outcome:     "SUCCESS",
		Metadata: map[string]interface{}{
			"oldStatus": oldStatus,
			"newStatus": newStatus,
		},
	}
	m.AuditTrail = append(m.AuditTrail, auditEntry)
}

// StartTransformation initializes a new transformation step
func (m *UniversalMessage) StartTransformation(serviceName string, inputType, outputType MessageType) *TransformationRecord {
	now := time.Now()

	// Initialize lineage if not exists
	if m.TransformationLineage == nil {
		m.TransformationLineage = &TransformationLineage{
			ID:           uuid.New().String(),
			MessageID:    m.ID,
			SourceFormat: m.MessageType,
			TargetFormat: outputType,
			PipelineID:   uuid.New().String(),
			StartTime:    now,
			Records:      []TransformationRecord{},
			Rollbacks:    []RollbackRecord{},
		}
	}

	// Create new transformation record
	stepNumber := len(m.TransformationLineage.Records) + 1
	transformRecord := TransformationRecord{
		ID:               uuid.New().String(),
		TransformationID: uuid.MustParse(m.TransformationLineage.ID),
		StepNumber:       stepNumber,
		ServiceName:      serviceName,
		InputType:        inputType,
		OutputType:       outputType,
		StartTime:        now,
		Status:           "STARTED",
		Success:          false,
		InputSize:        m.ContentSize,
		Metadata:         make(map[string]interface{}),
		Configuration:    make(map[string]interface{}),
	}

	m.CurrentTransformation = &transformRecord
	m.TransformationLineage.TotalSteps = stepNumber

	return &transformRecord
}

// CompleteTransformation marks a transformation step as complete
func (m *UniversalMessage) CompleteTransformation(success bool, outputSize int64, errorMessage string) {
	if m.CurrentTransformation == nil {
		return
	}

	now := time.Now()
	m.CurrentTransformation.EndTime = &now
	m.CurrentTransformation.Duration = now.Sub(m.CurrentTransformation.StartTime)
	m.CurrentTransformation.Success = success
	m.CurrentTransformation.OutputSize = outputSize

	if success {
		m.CurrentTransformation.Status = "COMPLETED"
		m.TransformationLineage.SuccessfulSteps++
	} else {
		m.CurrentTransformation.Status = "FAILED"
		m.CurrentTransformation.ErrorMessage = errorMessage
		m.TransformationLineage.FailedSteps++
	}

	// Add to lineage records
	m.TransformationLineage.Records = append(m.TransformationLineage.Records, *m.CurrentTransformation)
	m.CurrentTransformation = nil

	// Update overall lineage status
	if m.TransformationLineage.FailedSteps > 0 {
		m.TransformationLineage.OverallStatus = "FAILED"
	} else if m.TransformationLineage.SuccessfulSteps == m.TransformationLineage.TotalSteps {
		m.TransformationLineage.OverallStatus = "COMPLETED"
	} else {
		m.TransformationLineage.OverallStatus = "IN_PROGRESS"
	}
}

// AddError adds a processing error to the message
func (m *UniversalMessage) AddError(stage, serviceName, errorCode, message, details string, fatal bool) {
	error := ProcessingError{
		ID:          uuid.New().String(),
		Stage:       stage,
		ServiceName: serviceName,
		ErrorCode:   errorCode,
		Message:     message,
		Details:     details,
		Timestamp:   time.Now(),
		Fatal:       fatal,
		Recoverable: !fatal,
		Context:     make(map[string]interface{}),
	}

	m.ProcessingErrors = append(m.ProcessingErrors, error)

	// Update status if fatal error
	if fatal {
		m.UpdateStatus(StatusFailed, serviceName, fmt.Sprintf("Fatal error: %s", message))
	}
}

// AddWarning adds a processing warning to the message
func (m *UniversalMessage) AddWarning(stage, serviceName, warningCode, message, impact string) {
	warning := ProcessingWarning{
		ID:          uuid.New().String(),
		Stage:       stage,
		ServiceName: serviceName,
		WarningCode: warningCode,
		Message:     message,
		Impact:      impact,
		Timestamp:   time.Now(),
		Context:     make(map[string]interface{}),
	}

	m.ProcessingWarnings = append(m.ProcessingWarnings, warning)
}

// AddTransformedContent adds transformation output
func (m *UniversalMessage) AddTransformedContent(targetFormat MessageType, content []byte, transformID string) {
	transformedContent := TransformedContent{
		ID:           uuid.New().String(),
		TargetFormat: targetFormat,
		Content:      content,
		Size:         int64(len(content)),
		CreatedAt:    time.Now(),
		TransformID:  transformID,
		Metadata:     make(map[string]interface{}),
	}

	if m.TransformedContent == nil {
		m.TransformedContent = []TransformedContent{}
	}
	m.TransformedContent = append(m.TransformedContent, transformedContent)
}

// CanRetry checks if the message can be retried
func (m *UniversalMessage) CanRetry() bool {
	return m.RetryCount < m.MaxRetries && m.Status == StatusFailed
}

// IncrementRetry increments the retry counter
func (m *UniversalMessage) IncrementRetry() {
	m.RetryCount++
	m.UpdateStatus(StatusRetry, "RETRY_MANAGER", fmt.Sprintf("Retry attempt %d of %d", m.RetryCount, m.MaxRetries))
}

// GetProcessingSummary returns a summary of processing status
func (m *UniversalMessage) GetProcessingSummary() map[string]interface{} {
	return map[string]interface{}{
		"messageId":      m.ID,
		"correlationId":  m.CorrelationID,
		"status":         m.Status,
		"messageType":    m.MessageType,
		"priority":       m.Priority,
		"retryCount":     m.RetryCount,
		"errorCount":     len(m.ProcessingErrors),
		"warningCount":   len(m.ProcessingWarnings),
		"transformationSteps": func() int {
			if m.TransformationLineage != nil {
				return m.TransformationLineage.TotalSteps
			}
			return 0
		}(),
		"hasTransformedContent": len(m.TransformedContent) > 0,
		"deliveryAttempts":      len(m.DeliveryAttempts),
		"processingTime": func() string {
			if m.ProcessedAt != nil {
				return m.ProcessedAt.Sub(m.ReceivedAt).String()
			}
			return time.Since(m.ReceivedAt).String()
		}(),
	}
}

// =====================================
// UTILITY FUNCTIONS
// =====================================

// detectEncoding attempts to detect message encoding
func detectEncoding(content []byte) string {
	if len(content) == 0 {
		return "UNKNOWN"
	}

	// Simple UTF-8 detection
	if isValidUTF8(content) {
		return "UTF-8"
	}

	// Check for common HL7 encoding markers
	contentStr := string(content)
	if strings.Contains(contentStr, "MSH|") {
		return "HL7_PIPE"
	}

	// Check for XML
	if strings.HasPrefix(strings.TrimSpace(contentStr), "<?xml") ||
		strings.HasPrefix(strings.TrimSpace(contentStr), "<") {
		return "XML"
	}

	// Check for JSON
	if strings.HasPrefix(strings.TrimSpace(contentStr), "{") ||
		strings.HasPrefix(strings.TrimSpace(contentStr), "[") {
		return "JSON"
	}

	return "BINARY"
}

// isValidUTF8 checks if content is valid UTF-8
func isValidUTF8(content []byte) bool {
	for len(content) > 0 {
		r, size := decodeRune(content)
		if r == '\uFFFD' && size == 1 {
			return false
		}
		content = content[size:]
	}
	return true
}

// decodeRune decodes a single UTF-8 rune
func decodeRune(p []byte) (r rune, size int) {
	if len(p) == 0 {
		return '\uFFFD', 0
	}

	// Simple ASCII case
	if p[0] < 0x80 {
		return rune(p[0]), 1
	}

	// For now, just return error rune for non-ASCII
	return '\uFFFD', 1
}

// calculateMD5 calculates MD5 checksum
func calculateMD5(content []byte) string {
	// TODO: Implement actual MD5 calculation
	return fmt.Sprintf("md5_%d", len(content))
}

// calculateSHA256 calculates SHA256 checksum
func calculateSHA256(content []byte) string {
	// TODO: Implement actual SHA256 calculation
	return fmt.Sprintf("sha256_%d", len(content))
}

// =====================================
// JSON SERIALIZATION HELPERS
// =====================================

// MarshalJSON custom JSON marshaling
func (m *UniversalMessage) MarshalJSON() ([]byte, error) {
	type Alias UniversalMessage
	return json.Marshal(&struct {
		*Alias
		ProcessingTime string `json:"processingTime"`
	}{
		Alias: (*Alias)(m),
		ProcessingTime: func() string {
			if m.ProcessedAt != nil {
				return m.ProcessedAt.Sub(m.ReceivedAt).String()
			}
			return time.Since(m.ReceivedAt).String()
		}(),
	})
}

// String returns a string representation of the message
func (m *UniversalMessage) String() string {
	return fmt.Sprintf("UniversalMessage{ID: %s, Type: %s, Status: %s, Size: %d bytes}",
		m.ID, m.MessageType, m.Status, m.ContentSize)
}