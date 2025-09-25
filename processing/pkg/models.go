// pkg/models.go
// Core data models for the Universal Interface Engine

package pkg

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// UniversalMessage represents any message in the system with complete lineage
type UniversalMessage struct {
	// Core Identity
	ID           string `json:"id" bson:"_id"`
	CorrelationID string `json:"correlationId" bson:"correlationId"`
	ParentID     *string `json:"parentId,omitempty" bson:"parentId,omitempty"`

	// Message Content
	Content      string                 `json:"content" bson:"content"`
	ContentType  string                 `json:"contentType" bson:"contentType"` // HL7, FHIR, JSON, XML, CSV
	Encoding     string                 `json:"encoding" bson:"encoding"`       // UTF-8, ASCII, BASE64
	Size         int64                  `json:"size" bson:"size"`

	// Source Information
	SourceSystem     string                 `json:"sourceSystem" bson:"sourceSystem"`
	SourceInterface  string                 `json:"sourceInterface" bson:"sourceInterface"`
	SourceEndpoint   string                 `json:"sourceEndpoint" bson:"sourceEndpoint"`
	SourceProtocol   string                 `json:"sourceProtocol" bson:"sourceProtocol"` // TCP, HTTP, FILE, DB, MQ
	SourceIP         string                 `json:"sourceIp,omitempty" bson:"sourceIp,omitempty"`

	// Target Information
	TargetSystem     string                 `json:"targetSystem,omitempty" bson:"targetSystem,omitempty"`
	TargetInterface  string                 `json:"targetInterface,omitempty" bson:"targetInterface,omitempty"`
	TargetEndpoint   string                 `json:"targetEndpoint,omitempty" bson:"targetEndpoint,omitempty"`
	TargetProtocol   string                 `json:"targetProtocol,omitempty" bson:"targetProtocol,omitempty"`

	// Processing State
	Status           MessageStatus          `json:"status" bson:"status"`
	Priority         int                    `json:"priority" bson:"priority"` // 1=highest, 10=lowest
	ProcessingStage  string                 `json:"processingStage" bson:"processingStage"`

	// Timestamps
	ReceivedAt       time.Time              `json:"receivedAt" bson:"receivedAt"`
	ProcessingStarted *time.Time            `json:"processingStarted,omitempty" bson:"processingStarted,omitempty"`
	ProcessingCompleted *time.Time          `json:"processingCompleted,omitempty" bson:"processingCompleted,omitempty"`
	DeliveredAt      *time.Time             `json:"deliveredAt,omitempty" bson:"deliveredAt,omitempty"`

	// Processing Information
	ProcessingTimeMs int64                  `json:"processingTimeMs" bson:"processingTimeMs"`
	TransformationApplied string            `json:"transformationApplied,omitempty" bson:"transformationApplied,omitempty"`
	ValidationResults []ValidationResult    `json:"validationResults,omitempty" bson:"validationResults,omitempty"`

	// Error Handling
	ErrorCount       int                    `json:"errorCount" bson:"errorCount"`
	LastError        *MessageError          `json:"lastError,omitempty" bson:"lastError,omitempty"`
	RetryCount       int                    `json:"retryCount" bson:"retryCount"`
	MaxRetries       int                    `json:"maxRetries" bson:"maxRetries"`

	// Delivery Tracking
	DeliveryStatus   DeliveryStatus         `json:"deliveryStatus" bson:"deliveryStatus"`
	DeliveryAttempts int                    `json:"deliveryAttempts" bson:"deliveryAttempts"`
	AckReceived      bool                   `json:"ackReceived" bson:"ackReceived"`
	AckTimestamp     *time.Time             `json:"ackTimestamp,omitempty" bson:"ackTimestamp,omitempty"`

	// Metadata and Extensions
	Metadata         map[string]interface{} `json:"metadata" bson:"metadata"`
	Tags             []string               `json:"tags,omitempty" bson:"tags,omitempty"`

	// Audit Trail
	CreatedAt        time.Time              `json:"createdAt" bson:"createdAt"`
	UpdatedAt        time.Time              `json:"updatedAt" bson:"updatedAt"`
	AuditTrail       []AuditEntry          `json:"auditTrail,omitempty" bson:"auditTrail,omitempty"`
}

// MessageStatus represents the current state of a message
type MessageStatus string

const (
	StatusReceived    MessageStatus = "received"
	StatusValidating  MessageStatus = "validating"
	StatusTransforming MessageStatus = "transforming"
	StatusRouting     MessageStatus = "routing"
	StatusDelivering  MessageStatus = "delivering"
	StatusDelivered   MessageStatus = "delivered"
	StatusAcknowledged MessageStatus = "acknowledged"
	StatusFailed      MessageStatus = "failed"
	StatusRetrying    MessageStatus = "retrying"
	StatusExpired     MessageStatus = "expired"
)

// DeliveryStatus represents delivery state
type DeliveryStatus string

const (
	DeliveryPending   DeliveryStatus = "pending"
	DeliveryInProgress DeliveryStatus = "in_progress"
	DeliverySuccessful DeliveryStatus = "successful"
	DeliveryFailed    DeliveryStatus = "failed"
	DeliveryRetrying  DeliveryStatus = "retrying"
	DeliveryAbandoned DeliveryStatus = "abandoned"
)

// MessageError represents an error that occurred during processing
type MessageError struct {
	Code        string    `json:"code" bson:"code"`
	Message     string    `json:"message" bson:"message"`
	Details     string    `json:"details,omitempty" bson:"details,omitempty"`
	Stage       string    `json:"stage" bson:"stage"`
	Timestamp   time.Time `json:"timestamp" bson:"timestamp"`
	Recoverable bool      `json:"recoverable" bson:"recoverable"`
}

// ValidationResult represents validation results
type ValidationResult struct {
	Type        string    `json:"type" bson:"type"` // schema, syntax, business
	Valid       bool      `json:"valid" bson:"valid"`
	ErrorCount  int       `json:"errorCount" bson:"errorCount"`
	WarningCount int      `json:"warningCount" bson:"warningCount"`
	Details     []string  `json:"details,omitempty" bson:"details,omitempty"`
	Timestamp   time.Time `json:"timestamp" bson:"timestamp"`
}

// AuditEntry represents an audit trail entry
type AuditEntry struct {
	Action      string                 `json:"action" bson:"action"`
	Actor       string                 `json:"actor" bson:"actor"` // system, user, interface
	Timestamp   time.Time              `json:"timestamp" bson:"timestamp"`
	Details     map[string]interface{} `json:"details,omitempty" bson:"details,omitempty"`
	Changes     map[string]interface{} `json:"changes,omitempty" bson:"changes,omitempty"`
}

// InterfaceDefinition represents a complete interface configuration
type InterfaceDefinition struct {
	// Identity
	ID          string `json:"id" bson:"_id"`
	Name        string `json:"name" bson:"name"`
	Description string `json:"description" bson:"description"`
	Version     string `json:"version" bson:"version"`

	// Interface Type
	Type        InterfaceType `json:"type" bson:"type"`
	Protocol    Protocol      `json:"protocol" bson:"protocol"`

	// Source Configuration
	Source      ConnectorConfig `json:"source" bson:"source"`

	// Target Configuration(s) - supports multi-target routing
	Targets     []ConnectorConfig `json:"targets" bson:"targets"`

	// Processing Pipeline
	Pipeline    ProcessingPipeline `json:"pipeline" bson:"pipeline"`

	// Routing Rules
	RoutingRules []RoutingRule `json:"routingRules,omitempty" bson:"routingRules,omitempty"`

	// State
	Status      InterfaceStatus `json:"status" bson:"status"`
	IsActive    bool           `json:"isActive" bson:"isActive"`

	// Configuration
	Settings    InterfaceSettings `json:"settings" bson:"settings"`

	// Timestamps
	CreatedAt   time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt" bson:"updatedAt"`
	LastStarted *time.Time `json:"lastStarted,omitempty" bson:"lastStarted,omitempty"`
	LastStopped *time.Time `json:"lastStopped,omitempty" bson:"lastStopped,omitempty"`
}

// InterfaceType defines the type of interface
type InterfaceType string

const (
	TypeSource      InterfaceType = "source"      // Only receives messages
	TypeTarget      InterfaceType = "target"      // Only sends messages
	TypeBidirectional InterfaceType = "bidirectional" // Both receives and sends
	TypeRouter      InterfaceType = "router"      // Routes between interfaces
	TypeTransformer InterfaceType = "transformer" // Transforms message format
)

// Protocol defines the communication protocol
type Protocol string

const (
	ProtocolTCP       Protocol = "tcp"
	ProtocolMLLP      Protocol = "mllp"
	ProtocolHTTP      Protocol = "http"
	ProtocolHTTPS     Protocol = "https"
	ProtocolFHIR      Protocol = "fhir"
	ProtocolFile      Protocol = "file"
	ProtocolDatabase  Protocol = "database"
	ProtocolRabbitMQ  Protocol = "rabbitmq"
	ProtocolRedis     Protocol = "redis"
	ProtocolWebSocket Protocol = "websocket"
)

// InterfaceStatus represents interface state
type InterfaceStatus string

const (
	InterfaceStatusDraft      InterfaceStatus = "draft"
	InterfaceStatusConfigured InterfaceStatus = "configured"
	InterfaceStatusActive     InterfaceStatus = "active"
	InterfaceStatusPaused     InterfaceStatus = "paused"
	InterfaceStatusError      InterfaceStatus = "error"
	InterfaceStatusDisabled   InterfaceStatus = "disabled"
)

// ConnectorConfig defines connection configuration
type ConnectorConfig struct {
	Type       string                 `json:"type" bson:"type"`
	Protocol   Protocol               `json:"protocol" bson:"protocol"`
	Endpoint   string                 `json:"endpoint" bson:"endpoint"`
	Port       int                    `json:"port,omitempty" bson:"port,omitempty"`
	Path       string                 `json:"path,omitempty" bson:"path,omitempty"`

	// Authentication
	AuthType   string                 `json:"authType,omitempty" bson:"authType,omitempty"`
	Username   string                 `json:"username,omitempty" bson:"username,omitempty"`
	Password   string                 `json:"password,omitempty" bson:"password,omitempty"`
	Token      string                 `json:"token,omitempty" bson:"token,omitempty"`

	// Protocol-specific settings
	Settings   map[string]interface{} `json:"settings" bson:"settings"`

	// Connection behavior
	Timeout    time.Duration          `json:"timeout" bson:"timeout"`
	Retries    int                    `json:"retries" bson:"retries"`
	KeepAlive  bool                   `json:"keepAlive" bson:"keepAlive"`
}

// ProcessingPipeline defines the message processing pipeline
type ProcessingPipeline struct {
	Stages []PipelineStage `json:"stages" bson:"stages"`
}

// PipelineStage represents a stage in the processing pipeline
type PipelineStage struct {
	Name        string                 `json:"name" bson:"name"`
	Type        StageType              `json:"type" bson:"type"`
	Config      map[string]interface{} `json:"config" bson:"config"`
	Enabled     bool                   `json:"enabled" bson:"enabled"`
	Order       int                    `json:"order" bson:"order"`

	// Error handling
	OnError     ErrorAction            `json:"onError" bson:"onError"`
	RetryPolicy *RetryPolicy          `json:"retryPolicy,omitempty" bson:"retryPolicy,omitempty"`
}

// StageType defines types of processing stages
type StageType string

const (
	StageValidation   StageType = "validation"
	StageTransformation StageType = "transformation"
	StageEnrichment   StageType = "enrichment"
	StageFiltering    StageType = "filtering"
	StageRouting      StageType = "routing"
	StageAuditing     StageType = "auditing"
)

// ErrorAction defines what to do when a stage fails
type ErrorAction string

const (
	ErrorActionContinue ErrorAction = "continue"
	ErrorActionRetry    ErrorAction = "retry"
	ErrorActionFail     ErrorAction = "fail"
	ErrorActionDeadLetter ErrorAction = "dead_letter"
)

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxRetries   int           `json:"maxRetries" bson:"maxRetries"`
	RetryDelay   time.Duration `json:"retryDelay" bson:"retryDelay"`
	BackoffType  BackoffType   `json:"backoffType" bson:"backoffType"`
	MaxDelay     time.Duration `json:"maxDelay" bson:"maxDelay"`
}

// BackoffType defines retry backoff strategy
type BackoffType string

const (
	BackoffFixed       BackoffType = "fixed"
	BackoffLinear      BackoffType = "linear"
	BackoffExponential BackoffType = "exponential"
)

// RoutingRule defines message routing logic
type RoutingRule struct {
	ID          string                 `json:"id" bson:"id"`
	Name        string                 `json:"name" bson:"name"`
	Condition   string                 `json:"condition" bson:"condition"` // Expression to evaluate
	TargetID    string                 `json:"targetId" bson:"targetId"`
	Priority    int                    `json:"priority" bson:"priority"`
	Enabled     bool                   `json:"enabled" bson:"enabled"`

	// Advanced routing
	Transform   *TransformConfig       `json:"transform,omitempty" bson:"transform,omitempty"`
	Settings    map[string]interface{} `json:"settings,omitempty" bson:"settings,omitempty"`
}

// TransformConfig defines transformation applied during routing
type TransformConfig struct {
	Type       string                 `json:"type" bson:"type"`
	Template   string                 `json:"template" bson:"template"`
	Parameters map[string]interface{} `json:"parameters" bson:"parameters"`
}

// InterfaceSettings defines interface-level settings
type InterfaceSettings struct {
	// Message handling
	MaxMessageSize    int64         `json:"maxMessageSize" bson:"maxMessageSize"`
	MessageRetention  time.Duration `json:"messageRetention" bson:"messageRetention"`
	EnableValidation  bool          `json:"enableValidation" bson:"enableValidation"`
	EnableTransformation bool       `json:"enableTransformation" bson:"enableTransformation"`

	// Performance
	MaxConcurrency    int           `json:"maxConcurrency" bson:"maxConcurrency"`
	BatchSize         int           `json:"batchSize" bson:"batchSize"`
	BatchTimeout      time.Duration `json:"batchTimeout" bson:"batchTimeout"`

	// Monitoring
	EnableMetrics     bool          `json:"enableMetrics" bson:"enableMetrics"`
	EnableAuditTrail  bool          `json:"enableAuditTrail" bson:"enableAuditTrail"`
	LogLevel          string        `json:"logLevel" bson:"logLevel"`

	// Storage
	StorageStrategy   StorageStrategy `json:"storageStrategy" bson:"storageStrategy"`
	CompressMessages  bool          `json:"compressMessages" bson:"compressMessages"`
}

// StorageStrategy defines where and how messages are stored
type StorageStrategy string

const (
	StoragePostgreSQL StorageStrategy = "postgresql"
	StorageMongoDB    StorageStrategy = "mongodb"
	StorageHybrid     StorageStrategy = "hybrid" // PostgreSQL + MongoDB
	StorageMemory     StorageStrategy = "memory"
)

// MessageTransformer interface for message transformation
type MessageTransformer interface {
	Transform(ctx context.Context, message *UniversalMessage) (*UniversalMessage, error)
	ValidateSource(content string, contentType string) error
	ValidateTarget(content string, contentType string) error
	GetSupportedFormats() ([]string, []string) // source, target formats
	GetTransformationType() string
}

// ConnectorInterface defines the universal connector interface
type ConnectorInterface interface {
	// Lifecycle
	Start(ctx context.Context) error
	Stop() error
	IsRunning() bool

	// Connection management
	Connect() error
	Disconnect() error
	TestConnection() error

	// Status and monitoring
	GetStatus() ConnectorStatus
	GetMetrics() ConnectorMetrics

	// Configuration
	UpdateConfig(config ConnectorConfig) error
	GetConfig() ConnectorConfig
}

// InputConnectorInterface extends ConnectorInterface for input connectors
type InputConnectorInterface interface {
	ConnectorInterface

	// Message reception
	StartListening(messageChan chan<- *UniversalMessage) error
	StopListening() error

	// Protocol-specific
	SupportsProtocol(protocol Protocol) bool
}

// OutputConnectorInterface extends ConnectorInterface for output connectors
type OutputConnectorInterface interface {
	ConnectorInterface

	// Message sending
	SendMessage(ctx context.Context, message *UniversalMessage) error
	SendBatch(ctx context.Context, messages []*UniversalMessage) error

	// Delivery confirmation
	SupportsAcknowledgment() bool
	WaitForAcknowledgment(messageID string, timeout time.Duration) error

	// Protocol-specific
	SupportsProtocol(protocol Protocol) bool
}

// ConnectorStatus represents connector status
type ConnectorStatus struct {
	Type         string    `json:"type"`
	Protocol     Protocol  `json:"protocol"`
	Status       string    `json:"status"`
	LastActivity time.Time `json:"lastActivity"`
	IsConnected  bool      `json:"isConnected"`
	ErrorCount   int64     `json:"errorCount"`
	LastError    *string   `json:"lastError,omitempty"`
}

// ConnectorMetrics represents connector performance metrics
type ConnectorMetrics struct {
	MessagesProcessed int64         `json:"messagesProcessed"`
	MessagesPerSecond float64       `json:"messagesPerSecond"`
	AverageLatency    time.Duration `json:"averageLatency"`
	ErrorRate         float64       `json:"errorRate"`
	BytesTransferred  int64         `json:"bytesTransferred"`
	UptimeSeconds     int64         `json:"uptimeSeconds"`
}

// NewUniversalMessage creates a new universal message with defaults
func NewUniversalMessage() *UniversalMessage {
	now := time.Now()
	return &UniversalMessage{
		ID:              uuid.New().String(),
		CorrelationID:   uuid.New().String(),
		Status:          StatusReceived,
		Priority:        5, // Default priority
		ReceivedAt:      now,
		CreatedAt:       now,
		UpdatedAt:       now,
		ErrorCount:      0,
		RetryCount:      0,
		MaxRetries:      3,
		DeliveryStatus:  DeliveryPending,
		DeliveryAttempts: 0,
		AckReceived:     false,
		Metadata:        make(map[string]interface{}),
		AuditTrail:      []AuditEntry{},
	}
}

// AddAuditEntry adds an entry to the message audit trail
func (m *UniversalMessage) AddAuditEntry(action, actor string, details map[string]interface{}) {
	entry := AuditEntry{
		Action:    action,
		Actor:     actor,
		Timestamp: time.Now(),
		Details:   details,
	}
	m.AuditTrail = append(m.AuditTrail, entry)
	m.UpdatedAt = time.Now()
}

// SetError sets the last error for the message
func (m *UniversalMessage) SetError(code, message, details, stage string, recoverable bool) {
	m.ErrorCount++
	m.LastError = &MessageError{
		Code:        code,
		Message:     message,
		Details:     details,
		Stage:       stage,
		Timestamp:   time.Now(),
		Recoverable: recoverable,
	}
	m.UpdatedAt = time.Now()

	if recoverable && m.RetryCount < m.MaxRetries {
		m.Status = StatusRetrying
	} else {
		m.Status = StatusFailed
	}
}

// CanRetry checks if the message can be retried
func (m *UniversalMessage) CanRetry() bool {
	return m.LastError != nil && m.LastError.Recoverable && m.RetryCount < m.MaxRetries
}

// MarkAsProcessingStarted marks when processing began
func (m *UniversalMessage) MarkAsProcessingStarted() {
	now := time.Now()
	m.ProcessingStarted = &now
	m.UpdatedAt = now
}

// MarkAsProcessingCompleted marks when processing completed
func (m *UniversalMessage) MarkAsProcessingCompleted() {
	now := time.Now()
	m.ProcessingCompleted = &now
	m.UpdatedAt = now

	if m.ProcessingStarted != nil {
		m.ProcessingTimeMs = now.Sub(*m.ProcessingStarted).Milliseconds()
	}
}