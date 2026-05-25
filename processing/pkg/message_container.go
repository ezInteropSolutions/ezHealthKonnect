// pkg/message_container.go
// Universal Message Container with advanced interface support

package pkg

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MessageContainer provides a universal container for all message types
type MessageContainer struct {
	// Core Message
	Message *UniversalMessage `json:"message"`

	// Lineage Tracking
	Lineage *MessageLineage `json:"lineage"`

	// Processing Context
	Context *ProcessingContext `json:"context"`

	// Format-Specific Handlers
	formatHandlers map[string]FormatHandler
}

// MessageLineage tracks complete message lineage across hops
type MessageLineage struct {
	OriginalID      string         `json:"originalId"`
	CorrelationID   string         `json:"correlationId"`
	Hops            []LineageHop   `json:"hops"`
	TotalHops       int            `json:"totalHops"`
	TotalLatencyMs  int64          `json:"totalLatencyMs"`
	Created         time.Time      `json:"created"`
	LastUpdated     time.Time      `json:"lastUpdated"`
}

// LineageHop represents one hop in the message journey
type LineageHop struct {
	HopID           string                 `json:"hopId"`
	FromInterface   string                 `json:"fromInterface"`
	ToInterface     string                 `json:"toInterface"`
	Transformation  string                 `json:"transformation,omitempty"`
	Protocol        Protocol               `json:"protocol"`
	StartTime       time.Time              `json:"startTime"`
	EndTime         *time.Time             `json:"endTime,omitempty"`
	LatencyMs       *int64                 `json:"latencyMs,omitempty"`
	Status          HopStatus              `json:"status"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	Error           *MessageError          `json:"error,omitempty"`
}

// HopStatus represents the status of a message hop
type HopStatus string

const (
	HopStatusPending    HopStatus = "pending"
	HopStatusProcessing HopStatus = "processing"
	HopStatusCompleted  HopStatus = "completed"
	HopStatusFailed     HopStatus = "failed"
)

// ProcessingContext provides context for message processing
type ProcessingContext struct {
	InterfaceID     string                 `json:"interfaceId"`
	WorkerID        string                 `json:"workerId"`
	SessionID       string                 `json:"sessionId"`
	Environment     string                 `json:"environment"` // dev, test, prod
	ProcessingMode  ProcessingMode         `json:"processingMode"`

	// Performance tracking
	StartTime       time.Time              `json:"startTime"`
	DeadlineTime    *time.Time             `json:"deadlineTime,omitempty"`

	// Configuration
	Settings        map[string]interface{} `json:"settings"`
	FeatureFlags    map[string]bool        `json:"featureFlags"`

	// Request context
	RequestHeaders  map[string]string      `json:"requestHeaders,omitempty"`
	UserContext     map[string]interface{} `json:"userContext,omitempty"`
}

// ProcessingMode defines how the message should be processed
type ProcessingMode string

const (
	ProcessingModeSync      ProcessingMode = "sync"      // Synchronous processing
	ProcessingModeAsync     ProcessingMode = "async"     // Asynchronous processing
	ProcessingModeBatch     ProcessingMode = "batch"     // Batch processing
	ProcessingModeStreaming ProcessingMode = "streaming" // Streaming processing
)

// FormatHandler interface for handling specific message formats
type FormatHandler interface {
	// Format detection and validation
	CanHandle(contentType string, content string) bool
	DetectFormat(content string) (string, error)
	ValidateFormat(content string) (*ValidationResult, error)

	// Content parsing and manipulation
	ParseContent(content string) (interface{}, error)
	SerializeContent(data interface{}) (string, error)

	// Metadata extraction
	ExtractMetadata(content string) (map[string]interface{}, error)
	ExtractIdentifiers(content string) (MessageIdentifiers, error)

	// Format-specific operations
	GetSchema() (interface{}, error)
	Transform(content string, targetFormat string, rules map[string]interface{}) (string, error)
}

// MessageIdentifiers contains extracted message identifiers
type MessageIdentifiers struct {
	MessageID       string `json:"messageId"`
	MessageType     string `json:"messageType"`
	PatientID       string `json:"patientId,omitempty"`
	VisitID         string `json:"visitId,omitempty"`
	OrderID         string `json:"orderId,omitempty"`
	DocumentID      string `json:"documentId,omitempty"`
	ResourceType    string `json:"resourceType,omitempty"`
	ResourceID      string `json:"resourceId,omitempty"`
}

// NewMessageContainer creates a new universal message container
func NewMessageContainer() *MessageContainer {
	return &MessageContainer{
		Message:        NewUniversalMessage(),
		Lineage:        NewMessageLineage(),
		Context:        NewProcessingContext(),
		formatHandlers: make(map[string]FormatHandler),
	}
}

// NewMessageLineage creates a new message lineage tracker
func NewMessageLineage() *MessageLineage {
	now := time.Now()
	return &MessageLineage{
		OriginalID:    uuid.New().String(),
		CorrelationID: uuid.New().String(),
		Hops:          []LineageHop{},
		TotalHops:     0,
		Created:       now,
		LastUpdated:   now,
	}
}

// NewProcessingContext creates a new processing context
func NewProcessingContext() *ProcessingContext {
	return &ProcessingContext{
		SessionID:      uuid.New().String(),
		ProcessingMode: ProcessingModeSync,
		StartTime:      time.Now(),
		Settings:       make(map[string]interface{}),
		FeatureFlags:   make(map[string]bool),
		RequestHeaders: make(map[string]string),
		UserContext:    make(map[string]interface{}),
	}
}

// RegisterFormatHandler registers a format handler
func (mc *MessageContainer) RegisterFormatHandler(formatType string, handler FormatHandler) {
	mc.formatHandlers[strings.ToLower(formatType)] = handler
}

// GetFormatHandler returns the appropriate format handler
func (mc *MessageContainer) GetFormatHandler(contentType string) (FormatHandler, error) {
	contentType = strings.ToLower(contentType)

	// Direct match
	if handler, exists := mc.formatHandlers[contentType]; exists {
		return handler, nil
	}

	// Pattern matching for complex content types
	for format, handler := range mc.formatHandlers {
		if strings.Contains(contentType, format) {
			return handler, nil
		}
	}

	return nil, fmt.Errorf("no format handler found for content type: %s", contentType)
}

// AutoDetectFormat automatically detects the message format
func (mc *MessageContainer) AutoDetectFormat() (string, error) {
	content := mc.Message.Content

	for _, handler := range mc.formatHandlers {
		if handler.CanHandle(mc.Message.ContentType, content) {
			if detectedFormat, err := handler.DetectFormat(content); err == nil {
				return detectedFormat, nil
			}
		}
	}

	// Fallback to basic detection
	content = strings.TrimSpace(content)

	if strings.HasPrefix(content, "MSH|") {
		return "HL7", nil
	}

	if strings.HasPrefix(content, "{") && strings.HasSuffix(content, "}") {
		var jsonData interface{}
		if err := json.Unmarshal([]byte(content), &jsonData); err == nil {
			// Check for FHIR resource
			if resourceType, ok := jsonData.(map[string]interface{})["resourceType"]; ok {
				return fmt.Sprintf("FHIR_%s", resourceType), nil
			}
			return "JSON", nil
		}
	}

	if strings.HasPrefix(content, "<") {
		return "XML", nil
	}

	return "UNKNOWN", fmt.Errorf("unable to detect message format")
}

// ValidateMessage validates the message using appropriate format handler
func (mc *MessageContainer) ValidateMessage() (*ValidationResult, error) {
	if mc.Message.Content == "" {
		return &ValidationResult{
			Type:         "syntax",
			Valid:        false,
			ErrorCount:   1,
			Details:      []string{"message content is empty"},
			Timestamp:    time.Now(),
		}, nil
	}

	handler, err := mc.GetFormatHandler(mc.Message.ContentType)
	if err != nil {
		// Attempt auto-detection
		if detectedFormat, detectErr := mc.AutoDetectFormat(); detectErr == nil {
			mc.Message.ContentType = detectedFormat
			if handler, err = mc.GetFormatHandler(detectedFormat); err != nil {
				return &ValidationResult{
					Type:         "format",
					Valid:        false,
					ErrorCount:   1,
					Details:      []string{fmt.Sprintf("no validator available for format: %s", detectedFormat)},
					Timestamp:    time.Now(),
				}, nil
			}
		} else {
			return &ValidationResult{
				Type:         "format",
				Valid:        false,
				ErrorCount:   1,
				Details:      []string{"unable to detect message format"},
				Timestamp:    time.Now(),
			}, nil
		}
	}

	result, err := handler.ValidateFormat(mc.Message.Content)
	if err != nil {
		return &ValidationResult{
			Type:         "validation",
			Valid:        false,
			ErrorCount:   1,
			Details:      []string{err.Error()},
			Timestamp:    time.Now(),
		}, nil
	}

	return result, nil
}

// ExtractIdentifiers extracts message identifiers using format handlers
func (mc *MessageContainer) ExtractIdentifiers() (*MessageIdentifiers, error) {
	handler, err := mc.GetFormatHandler(mc.Message.ContentType)
	if err != nil {
		return nil, fmt.Errorf("no format handler available: %w", err)
	}

	identifiers, err := handler.ExtractIdentifiers(mc.Message.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to extract identifiers: %w", err)
	}

	return &identifiers, nil
}

// ExtractMetadata extracts metadata using format handlers
func (mc *MessageContainer) ExtractMetadata() error {
	handler, err := mc.GetFormatHandler(mc.Message.ContentType)
	if err != nil {
		return fmt.Errorf("no format handler available: %w", err)
	}

	metadata, err := handler.ExtractMetadata(mc.Message.Content)
	if err != nil {
		return fmt.Errorf("failed to extract metadata: %w", err)
	}

	// Merge with existing metadata
	for key, value := range metadata {
		mc.Message.Metadata[key] = value
	}

	return nil
}

// AddHop adds a new hop to the message lineage
func (mc *MessageContainer) AddHop(fromInterface, toInterface string, protocol Protocol) *LineageHop {
	hop := LineageHop{
		HopID:         uuid.New().String(),
		FromInterface: fromInterface,
		ToInterface:   toInterface,
		Protocol:      protocol,
		StartTime:     time.Now(),
		Status:        HopStatusPending,
		Metadata:      make(map[string]interface{}),
	}

	mc.Lineage.Hops = append(mc.Lineage.Hops, hop)
	mc.Lineage.TotalHops++
	mc.Lineage.LastUpdated = time.Now()

	// Update message audit trail
	mc.Message.AddAuditEntry("hop_added", "system", map[string]interface{}{
		"hop_id":        hop.HopID,
		"from_interface": fromInterface,
		"to_interface":   toInterface,
		"protocol":       string(protocol),
	})

	return &mc.Lineage.Hops[len(mc.Lineage.Hops)-1]
}

// CompleteCurrentHop marks the current hop as completed
func (mc *MessageContainer) CompleteCurrentHop() error {
	if len(mc.Lineage.Hops) == 0 {
		return fmt.Errorf("no active hops to complete")
	}

	hop := &mc.Lineage.Hops[len(mc.Lineage.Hops)-1]
	if hop.Status != HopStatusPending && hop.Status != HopStatusProcessing {
		return fmt.Errorf("hop is not in a completable state: %s", hop.Status)
	}

	now := time.Now()
	hop.EndTime = &now
	hop.Status = HopStatusCompleted
	latency := now.Sub(hop.StartTime).Milliseconds()
	hop.LatencyMs = &latency

	mc.Lineage.TotalLatencyMs += latency
	mc.Lineage.LastUpdated = now

	// Update message audit trail
	mc.Message.AddAuditEntry("hop_completed", "system", map[string]interface{}{
		"hop_id":     hop.HopID,
		"latency_ms": latency,
		"status":     string(hop.Status),
	})

	return nil
}

// FailCurrentHop marks the current hop as failed
func (mc *MessageContainer) FailCurrentHop(err error) error {
	if len(mc.Lineage.Hops) == 0 {
		return fmt.Errorf("no active hops to fail")
	}

	hop := &mc.Lineage.Hops[len(mc.Lineage.Hops)-1]
	now := time.Now()
	hop.EndTime = &now
	hop.Status = HopStatusFailed
	latency := now.Sub(hop.StartTime).Milliseconds()
	hop.LatencyMs = &latency

	hop.Error = &MessageError{
		Code:        "HOP_FAILURE",
		Message:     err.Error(),
		Stage:       "hop_processing",
		Timestamp:   now,
		Recoverable: true, // Hops can generally be retried
	}

	mc.Lineage.LastUpdated = now

	// Update message audit trail
	mc.Message.AddAuditEntry("hop_failed", "system", map[string]interface{}{
		"hop_id":     hop.HopID,
		"error":      err.Error(),
		"latency_ms": latency,
		"status":     string(hop.Status),
	})

	return nil
}

// SetProcessingMode sets the processing mode
func (mc *MessageContainer) SetProcessingMode(mode ProcessingMode) {
	mc.Context.ProcessingMode = mode
	mc.Message.AddAuditEntry("processing_mode_set", "system", map[string]interface{}{
		"mode": string(mode),
	})
}

// SetDeadline sets a processing deadline
func (mc *MessageContainer) SetDeadline(deadline time.Time) {
	mc.Context.DeadlineTime = &deadline
	mc.Message.AddAuditEntry("deadline_set", "system", map[string]interface{}{
		"deadline": deadline.Format(time.RFC3339),
	})
}

// IsExpired checks if the message has exceeded its deadline
func (mc *MessageContainer) IsExpired() bool {
	if mc.Context.DeadlineTime == nil {
		return false
	}
	return time.Now().After(*mc.Context.DeadlineTime)
}

// GetProcessingDuration returns how long the message has been processing
func (mc *MessageContainer) GetProcessingDuration() time.Duration {
	return time.Since(mc.Context.StartTime)
}

// Clone creates a deep copy of the message container for routing
func (mc *MessageContainer) Clone() (*MessageContainer, error) {
	// Serialize to JSON
	data, err := json.Marshal(mc)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize message container: %w", err)
	}

	// Deserialize to new instance
	newContainer := &MessageContainer{
		formatHandlers: make(map[string]FormatHandler),
	}

	if err := json.Unmarshal(data, newContainer); err != nil {
		return nil, fmt.Errorf("failed to deserialize message container: %w", err)
	}

	// Copy format handlers (they don't serialize)
	for key, handler := range mc.formatHandlers {
		newContainer.formatHandlers[key] = handler
	}

	// Generate new IDs for the clone
	newContainer.Message.ID = uuid.New().String()
	newContainer.Context.SessionID = uuid.New().String()

	// Update audit trail
	newContainer.Message.AddAuditEntry("message_cloned", "system", map[string]interface{}{
		"original_id": mc.Message.ID,
		"clone_id":    newContainer.Message.ID,
	})

	return newContainer, nil
}

// ToJSON converts the message container to JSON
func (mc *MessageContainer) ToJSON() ([]byte, error) {
	return json.Marshal(mc)
}

// FromJSON creates a message container from JSON
func FromJSON(data []byte) (*MessageContainer, error) {
	container := &MessageContainer{
		formatHandlers: make(map[string]FormatHandler),
	}

	if err := json.Unmarshal(data, container); err != nil {
		return nil, fmt.Errorf("failed to deserialize message container: %w", err)
	}

	return container, nil
}

// GetLineageSummary returns a summary of the message lineage
func (mc *MessageContainer) GetLineageSummary() map[string]interface{} {
	summary := map[string]interface{}{
		"original_id":      mc.Lineage.OriginalID,
		"correlation_id":   mc.Lineage.CorrelationID,
		"total_hops":       mc.Lineage.TotalHops,
		"total_latency_ms": mc.Lineage.TotalLatencyMs,
		"created":          mc.Lineage.Created,
		"last_updated":     mc.Lineage.LastUpdated,
	}

	if len(mc.Lineage.Hops) > 0 {
		hops := make([]map[string]interface{}, len(mc.Lineage.Hops))
		for i, hop := range mc.Lineage.Hops {
			hops[i] = map[string]interface{}{
				"hop_id":         hop.HopID,
				"from_interface": hop.FromInterface,
				"to_interface":   hop.ToInterface,
				"protocol":       string(hop.Protocol),
				"status":         string(hop.Status),
				"latency_ms":     hop.LatencyMs,
			}
		}
		summary["hops"] = hops
	}

	return summary
}

// GetProcessingSummary returns a summary of processing status
func (mc *MessageContainer) GetProcessingSummary() map[string]interface{} {
	return map[string]interface{}{
		"message_id":         mc.Message.ID,
		"correlation_id":     mc.Message.CorrelationID,
		"status":             string(mc.Message.Status),
		"content_type":       mc.Message.ContentType,
		"processing_mode":    string(mc.Context.ProcessingMode),
		"processing_duration": mc.GetProcessingDuration().String(),
		"error_count":        mc.Message.ErrorCount,
		"retry_count":        mc.Message.RetryCount,
		"delivery_status":    string(mc.Message.DeliveryStatus),
		"hop_count":          len(mc.Lineage.Hops),
		"total_latency_ms":   mc.Lineage.TotalLatencyMs,
		"expired":            mc.IsExpired(),
	}
}