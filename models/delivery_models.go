// models/delivery_models.go
// Delivery payload models for output transmission
// MVC Model Pattern - Pure data structures with clear separation of concerns

package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// TRANSMISSION PAYLOAD - What actually gets sent over the wire
// ============================================================================

// TransmissionPayload represents the actual bytes sent to destination
// This is the ONLY part that gets transmitted - no metadata, no tracking info
type TransmissionPayload struct {
	Content     []byte            `json:"content" bson:"content"`           // Actual message bytes (HL7, FHIR JSON, XML, etc.)
	ContentType string            `json:"content_type" bson:"content_type"` // MIME type: application/fhir+json, x-application/hl7-v2+er7, etc.
	Encoding    string            `json:"encoding" bson:"encoding"`         // utf-8, ascii, base64
	Headers     map[string]string `json:"headers" bson:"headers"`           // Protocol-specific headers (HTTP only, empty for TCP/MLLP)
}

// GetContentAsString converts content bytes to string (for text-based formats)
func (tp *TransmissionPayload) GetContentAsString() string {
	return string(tp.Content)
}

// GetContentSize returns size in bytes
func (tp *TransmissionPayload) GetContentSize() int {
	return len(tp.Content)
}

// ============================================================================
// DELIVERY RESPONSE - Response from destination system
// ============================================================================

// DeliveryResponse represents the response received from destination after delivery attempt
type DeliveryResponse struct {
	// Status
	Success      bool      `json:"success"`       // Overall success/failure
	DeliveryType string    `json:"delivery_type"` // ack, nack, http_response, kafka_confirmation, etc.
	ReceivedAt   time.Time `json:"received_at"`   // When response was received
	ResponseTime int64     `json:"response_time"` // Milliseconds

	// Protocol-specific response data
	StatusCode   int    `json:"status_code"`    // HTTP status code (200, 404, 500) or 0 for non-HTTP
	ResponseBody []byte `json:"response_body"`  // ACK message, HTTP response body, error details
	ErrorMessage string `json:"error_message"`  // Human-readable error if Success=false
	ErrorCode    string `json:"error_code"`     // Protocol-specific error code (HL7 AE/AR, HTTP status, etc.)

	// Connectivity-specific metadata
	HL7Acknowledgment *HL7AckResponse    `json:"hl7_ack,omitempty"`    // HL7 MLLP ACK/NACK details
	HTTPResponse      *HTTPResponse      `json:"http_response,omitempty"` // HTTP response details
	KafkaResponse     *KafkaResponse     `json:"kafka_response,omitempty"` // Kafka delivery confirmation
	DatabaseResponse  *DatabaseResponse  `json:"database_response,omitempty"` // Database operation result
	FileResponse      *FileResponse      `json:"file_response,omitempty"` // File write result
	QueueResponse     *QueueResponse     `json:"queue_response,omitempty"` // Message queue response

	// Retry context
	RetryAttempt  int  `json:"retry_attempt"`  // Which attempt this was (1 = first try)
	WillRetry     bool `json:"will_retry"`     // Whether system will retry
	NextRetryAt   *time.Time `json:"next_retry_at,omitempty"` // When next retry will happen
}

// HL7AckResponse represents HL7 MLLP ACK/NACK details
type HL7AckResponse struct {
	AckType          string `json:"ack_type"`           // AA (Application Accept), AE (Application Error), AR (Application Reject)
	MessageControlID string `json:"message_control_id"` // MSA.2 - Original message control ID
	TextMessage      string `json:"text_message"`       // MSA.3 - Error text
	ErrorCode        string `json:"error_code"`         // ERR segment error code if present
	ErrorLocation    string `json:"error_location"`     // ERR segment field location if present
	RawACK           string `json:"raw_ack"`            // Full ACK message
}

// HTTPResponse represents HTTP-specific response details
type HTTPResponse struct {
	StatusCode    int               `json:"status_code"`    // 200, 201, 400, 500, etc.
	StatusText    string            `json:"status_text"`    // "OK", "Bad Request", "Internal Server Error"
	Headers       map[string]string `json:"headers"`        // Response headers
	Body          string            `json:"body"`           // Response body as string
	ContentType   string            `json:"content_type"`   // Content-Type header
	Location      string            `json:"location,omitempty"` // Location header (for 201 Created)
	OperationOutcome *FHIROperationOutcome `json:"operation_outcome,omitempty"` // FHIR OperationOutcome if present
}

// FHIROperationOutcome represents FHIR error response
type FHIROperationOutcome struct {
	Severity    string `json:"severity"`    // fatal, error, warning, information
	Code        string `json:"code"`        // FHIR issue type
	Diagnostics string `json:"diagnostics"` // Human-readable error
	Expression  string `json:"expression"`  // FHIRPath expression if available
}

// KafkaResponse represents Kafka delivery confirmation
type KafkaResponse struct {
	Partition int32  `json:"partition"` // Which partition message was written to
	Offset    int64  `json:"offset"`    // Message offset in partition
	Topic     string `json:"topic"`     // Topic name
	Timestamp time.Time `json:"timestamp"` // Kafka timestamp
}

// DatabaseResponse represents database operation result
type DatabaseResponse struct {
	RowsAffected int64  `json:"rows_affected"` // Number of rows inserted/updated
	LastInsertID int64  `json:"last_insert_id,omitempty"` // Auto-increment ID if applicable
	TableName    string `json:"table_name"`    // Target table
}

// FileResponse represents file write operation result
type FileResponse struct {
	FilePath     string `json:"file_path"`     // Full path to written file
	BytesWritten int64  `json:"bytes_written"` // Number of bytes written
	FileExists   bool   `json:"file_exists"`   // Whether file was created or overwritten
}

// QueueResponse represents message queue delivery confirmation
type QueueResponse struct {
	MessageID   string `json:"message_id"`   // Queue-assigned message ID
	QueueName   string `json:"queue_name"`   // Queue/topic name
	ExchangeName string `json:"exchange_name,omitempty"` // RabbitMQ exchange if applicable
	RoutingKey  string `json:"routing_key,omitempty"`   // RabbitMQ routing key if applicable
}

// IsRetryable returns true if the error is temporary and should be retried
func (dr *DeliveryResponse) IsRetryable() bool {
	if dr.Success {
		return false
	}

	// HTTP temporary errors (5xx, 429 Too Many Requests, 408 Request Timeout)
	if dr.HTTPResponse != nil {
		if dr.StatusCode >= 500 && dr.StatusCode < 600 {
			return true // Server errors
		}
		if dr.StatusCode == 429 || dr.StatusCode == 408 {
			return true // Rate limit or timeout
		}
	}

	// HL7 MLLP - AE (Application Error) is retryable, AR (Application Reject) is not
	if dr.HL7Acknowledgment != nil {
		return dr.HL7Acknowledgment.AckType == "AE"
	}

	// Kafka/Queue - usually retryable unless it's a serialization error
	if dr.KafkaResponse != nil || dr.QueueResponse != nil {
		return true
	}

	// Database - deadlocks and connection errors are retryable
	if dr.DatabaseResponse != nil {
		// Could check error message for "deadlock", "connection refused", etc.
		return false // Default to non-retryable for now
	}

	return false
}

// ============================================================================
// DELIVERY PAYLOAD - Complete envelope for storage and tracking
// ============================================================================

// DeliveryPayload represents the complete delivery envelope
// This contains everything for storage, tracking, and audit
// Only the Transmission field actually gets sent over the wire
type DeliveryPayload struct {
	// Identity & Lineage (NOT sent over wire)
	PayloadID       string    `json:"payload_id" bson:"payload_id"`               // UUID for this specific delivery attempt
	SourceMessageID string    `json:"source_message_id" bson:"source_message_id"` // Original message ID from reception
	CorrelationID   string    `json:"correlation_id" bson:"correlation_id"`       // Correlation ID for tracking through pipeline
	InterfaceID     string    `json:"interface_id" bson:"interface_id"`           // Source interface
	PipelineID      string    `json:"pipeline_id" bson:"pipeline_id"`             // Transformation pipeline that created this
	GeneratedAt     time.Time `json:"generated_at" bson:"generated_at"`           // When payload was generated

	// Transmission (ONLY this gets sent)
	Transmission *TransmissionPayload `json:"transmission" bson:"transmission"` // Actual bytes + headers to send

	// Response (populated after delivery attempt)
	Response *DeliveryResponse `json:"response,omitempty" bson:"response,omitempty"` // Response from destination

	// Destination Configuration (NOT sent over wire)
	DestinationType     string `json:"destination_type" bson:"destination_type"`           // tcp_mllp, http_rest, kafka, s3, database, etc.
	DestinationEndpoint string `json:"destination_endpoint" bson:"destination_endpoint"`   // URL, host:port, topic name, etc.
	DestinationName     string `json:"destination_name" bson:"destination_name"`           // Human-readable name

	// Format Metadata (guides transmission preparation, NOT sent)
	Format         string                 `json:"format" bson:"format"`                   // hl7v2, fhir, json, xml, csv, edi
	FormatVersion  string                 `json:"format_version" bson:"format_version"`   // HL7 2.5, FHIR R4, etc.
	MessageType    string                 `json:"message_type" bson:"message_type"`       // ADT^A01, Patient, etc.
	ResourceType   string                 `json:"resource_type,omitempty" bson:"resource_type,omitempty"` // FHIR resource type (Patient, Bundle, etc.)
	FormatMetadata map[string]interface{} `json:"format_metadata" bson:"format_metadata"` // Format-specific details

	// Connectivity Metadata (guides connector behavior, NOT sent)
	ConnectivityMetadata map[string]interface{} `json:"connectivity_metadata" bson:"connectivity_metadata"` // Connector-specific config

	// Delivery Tracking (NOT sent over wire)
	DeliveryStatus   string     `json:"delivery_status" bson:"delivery_status"`       // pending, in_progress, delivered, failed, retrying
	DeliveryAttempts int        `json:"delivery_attempts" bson:"delivery_attempts"`   // Number of attempts made
	FirstAttemptAt   *time.Time `json:"first_attempt_at" bson:"first_attempt_at"`     // When first delivery was attempted
	LastAttemptAt    *time.Time `json:"last_attempt_at" bson:"last_attempt_at"`       // When last delivery was attempted
	DeliveredAt      *time.Time `json:"delivered_at" bson:"delivered_at"`             // When successfully delivered
	NextRetryAt      *time.Time `json:"next_retry_at" bson:"next_retry_at"`           // When next retry will happen
	MaxRetries       int        `json:"max_retries" bson:"max_retries"`               // Maximum retry attempts allowed
}

// ToJSON serializes DeliveryPayload to JSON for MongoDB storage
func (dp *DeliveryPayload) ToJSON() ([]byte, error) {
	return json.Marshal(dp)
}

// IsDelivered returns true if successfully delivered
func (dp *DeliveryPayload) IsDelivered() bool {
	return dp.DeliveryStatus == "delivered"
}

// IsFailed returns true if permanently failed (max retries exhausted)
func (dp *DeliveryPayload) IsFailed() bool {
	return dp.DeliveryStatus == "failed"
}

// ShouldRetry returns true if delivery should be retried
func (dp *DeliveryPayload) ShouldRetry() bool {
	if dp.DeliveryStatus == "delivered" || dp.DeliveryStatus == "failed" {
		return false
	}
	if dp.DeliveryAttempts >= dp.MaxRetries {
		return false
	}
	if dp.Response != nil && !dp.Response.IsRetryable() {
		return false
	}
	return true
}

// ============================================================================
// DELIVERY RESULT - Immediate result of delivery attempt
// ============================================================================

// DeliveryResult represents the immediate result of a delivery attempt
// Returned by OutboundConnector.Send() to provide feedback to caller
type DeliveryResult struct {
	Success      bool              `json:"success"`
	DeliveryID   string            `json:"delivery_id"`    // PayloadID from DeliveryPayload
	Response     *DeliveryResponse `json:"response"`       // Response from destination
	Error        error             `json:"error,omitempty"` // Error if delivery failed
	AttemptedAt  time.Time         `json:"attempted_at"`
	CompletedAt  time.Time         `json:"completed_at"`
	DeliveryTime int64             `json:"delivery_time"`  // Milliseconds
}

// ============================================================================
// BATCH DELIVERY - For connectors that support batching
// ============================================================================

// BatchDeliveryResult represents result of batch delivery
type BatchDeliveryResult struct {
	TotalMessages     int                `json:"total_messages"`
	SuccessfulCount   int                `json:"successful_count"`
	FailedCount       int                `json:"failed_count"`
	IndividualResults []*DeliveryResult  `json:"individual_results"`
	BatchError        error              `json:"batch_error,omitempty"` // Error affecting entire batch
	StartedAt         time.Time          `json:"started_at"`
	CompletedAt       time.Time          `json:"completed_at"`
	BatchDeliveryTime int64              `json:"batch_delivery_time"` // Milliseconds
}

// ============================================================================
// FACTORY FUNCTIONS - Constructors for common scenarios
// ============================================================================

// NewDeliveryPayload creates a new delivery payload with defaults
func NewDeliveryPayload(
	sourceMessageID string,
	correlationID string,
	interfaceID string,
	pipelineID string,
	transmission *TransmissionPayload,
	destinationType string,
	destinationEndpoint string,
) *DeliveryPayload {
	now := time.Now()
	return &DeliveryPayload{
		PayloadID:           generateUUID(), // Would use uuid.New().String()
		SourceMessageID:     sourceMessageID,
		CorrelationID:       correlationID,
		InterfaceID:         interfaceID,
		PipelineID:          pipelineID,
		GeneratedAt:         now,
		Transmission:        transmission,
		DestinationType:     destinationType,
		DestinationEndpoint: destinationEndpoint,
		DeliveryStatus:      "pending",
		DeliveryAttempts:    0,
		MaxRetries:          3, // Default
		FormatMetadata:      make(map[string]interface{}),
		ConnectivityMetadata: make(map[string]interface{}),
	}
}

// NewHL7TransmissionPayload creates transmission payload for HL7 over MLLP
func NewHL7TransmissionPayload(hl7Content string) *TransmissionPayload {
	return &TransmissionPayload{
		Content:     []byte(hl7Content),
		ContentType: "x-application/hl7-v2+er7",
		Encoding:    "utf-8",
		Headers:     make(map[string]string), // Empty for TCP/MLLP
	}
}

// NewFHIRTransmissionPayload creates transmission payload for FHIR over HTTP
func NewFHIRTransmissionPayload(fhirJSON []byte, httpHeaders map[string]string) *TransmissionPayload {
	if httpHeaders == nil {
		httpHeaders = make(map[string]string)
	}
	// Add standard FHIR headers
	httpHeaders["Content-Type"] = "application/fhir+json"
	httpHeaders["Accept"] = "application/fhir+json"

	return &TransmissionPayload{
		Content:     fhirJSON,
		ContentType: "application/fhir+json",
		Encoding:    "utf-8",
		Headers:     httpHeaders,
	}
}

// NewJSONTransmissionPayload creates transmission payload for generic JSON
func NewJSONTransmissionPayload(jsonContent []byte, httpHeaders map[string]string) *TransmissionPayload {
	if httpHeaders == nil {
		httpHeaders = make(map[string]string)
	}
	httpHeaders["Content-Type"] = "application/json"

	return &TransmissionPayload{
		Content:     jsonContent,
		ContentType: "application/json",
		Encoding:    "utf-8",
		Headers:     httpHeaders,
	}
}

// Helper function for UUID generation
func generateUUID() string {
	return uuid.New().String()
}
