// models/message_models.go
// Data models for inbound and outbound messages in the connector framework

package models

import (
	"time"
)

// InboundMessage represents a message received by an inbound connector
type InboundMessage struct {
	MessageID      string            `json:"message_id"`
	CorrelationID  string            `json:"correlation_id"`
	InterfaceID    string            `json:"interface_id,omitempty"`
	Content        string            `json:"content"`
	ContentType    string            `json:"content_type,omitempty"`
	SourceType     string            `json:"source_type"`
	SourceEndpoint string            `json:"source_endpoint"`
	SourceIP       string            `json:"source_ip,omitempty"`
	SourceMetadata map[string]string `json:"source_metadata,omitempty"`
	ReceivedAt     time.Time         `json:"received_at"`
	MessageType    string            `json:"message_type,omitempty"`
	MessageSize    int               `json:"message_size"`
	Encoding       string            `json:"encoding,omitempty"`
	Priority       int               `json:"priority"`
	Headers        map[string]string `json:"headers,omitempty"`
}

// OutboundMessage represents a message to be sent by an outbound connector
type OutboundMessage struct {
	MessageID         string            `json:"message_id"`
	CorrelationID     string            `json:"correlation_id"`
	InterfaceID       string            `json:"interface_id,omitempty"`
	Content           string            `json:"content"`
	ContentType       string            `json:"content_type,omitempty"`
	DestinationType   string            `json:"destination_type"`
	DestinationConfig map[string]interface{} `json:"destination_config,omitempty"`
	DestinationURL    string            `json:"destination_url,omitempty"`
	MessageType       string            `json:"message_type,omitempty"`
	MessageSize       int               `json:"message_size"`
	Encoding          string            `json:"encoding,omitempty"`
	Priority          int               `json:"priority"`
	Headers           map[string]string `json:"headers,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	RetryCount        int               `json:"retry_count"`
	MaxRetries        int               `json:"max_retries"`
	Timeout           int               `json:"timeout_seconds"`
	CreatedAt         time.Time         `json:"created_at"`
}

// MessageStatus represents the processing status of a message
type MessageStatus string

const (
	MessageStatusReceived   MessageStatus = "received"
	MessageStatusQueued     MessageStatus = "queued"
	MessageStatusProcessing MessageStatus = "processing"
	MessageStatusTransformed MessageStatus = "transformed"
	MessageStatusDelivering MessageStatus = "delivering"
	MessageStatusDelivered  MessageStatus = "delivered"
	MessageStatusFailed     MessageStatus = "failed"
	MessageStatusRetrying   MessageStatus = "retrying"
)

// MessagePriority represents message priority levels
type MessagePriority int

const (
	PriorityLow      MessagePriority = 1
	PriorityNormal   MessagePriority = 5
	PriorityHigh     MessagePriority = 8
	PriorityCritical MessagePriority = 10
)
