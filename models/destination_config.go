package models

// DestinationConfig represents the configuration for a message delivery destination
type DestinationConfig struct {
	// Destination type and protocol
	Type     string `json:"type"`     // "fhir", "hl7", "database", "file", "api", "queue"
	Protocol string `json:"protocol"` // "http", "https", "tcp", "mllp", "jdbc", "s3", "kafka", etc.

	// Connection details
	Host string `json:"host,omitempty"` // Hostname or IP address
	Port int    `json:"port,omitempty"` // Port number
	Path string `json:"path,omitempty"` // URL path (for HTTP) or file path

	// Full endpoint URL (alternative to host/port/path)
	Endpoint string `json:"endpoint,omitempty"` // e.g., "https://fhir.example.com/Patient"

	// Authentication
	AuthType     string                 `json:"auth_type,omitempty"`     // "none", "basic", "bearer", "oauth2", "api_key"
	Username     string                 `json:"username,omitempty"`      // For basic auth
	Password     string                 `json:"password,omitempty"`      // For basic auth
	BearerToken  string                 `json:"bearer_token,omitempty"`  // For bearer token auth
	APIKey       string                 `json:"api_key,omitempty"`       // For API key auth
	APIKeyHeader string                 `json:"api_key_header,omitempty"` // Header name for API key
	OAuth2Config map[string]interface{} `json:"oauth2_config,omitempty"` // For OAuth2 flows

	// Connection settings
	TimeoutMs int `json:"timeout_ms,omitempty"` // Connection timeout in milliseconds (default: 30000)

	// Retry policy
	RetryPolicy RetryPolicy `json:"retry_policy,omitempty"` // Retry configuration

	// TLS/SSL settings
	TLSEnabled   bool   `json:"tls_enabled,omitempty"`    // Enable TLS/SSL
	TLSCertPath  string `json:"tls_cert_path,omitempty"`  // Path to TLS certificate
	TLSKeyPath   string `json:"tls_key_path,omitempty"`   // Path to TLS key
	TLSVerify    bool   `json:"tls_verify,omitempty"`     // Verify TLS certificate (default: true)

	// HTTP-specific settings
	HTTPMethod      string            `json:"http_method,omitempty"`       // HTTP method (GET, POST, PUT, etc.)
	HTTPHeaders     map[string]string `json:"http_headers,omitempty"`      // Additional HTTP headers
	ContentType     string            `json:"content_type,omitempty"`      // Content-Type header
	AcceptHeader    string            `json:"accept_header,omitempty"`     // Accept header

	// FHIR-specific settings
	FHIRVersion         string `json:"fhir_version,omitempty"`          // "R4", "STU3", etc.
	FHIRServerURL       string `json:"fhir_server_url,omitempty"`       // Base FHIR server URL
	FHIRResourceType    string `json:"fhir_resource_type,omitempty"`    // Target resource type
	FHIRResourceEndpoint string `json:"fhir_resource_endpoint,omitempty"` // Specific resource endpoint

	// HL7-specific settings
	HL7Version     string `json:"hl7_version,omitempty"`      // "2.3", "2.5", "2.7", etc.
	HL7MLLPEnabled bool   `json:"hl7_mllp_enabled,omitempty"` // Use MLLP framing
	HL7WaitForAck  bool   `json:"hl7_wait_for_ack,omitempty"` // Wait for ACK response

	// Database-specific settings
	DatabaseType       string `json:"database_type,omitempty"`        // "postgresql", "mysql", "mongodb", etc.
	DatabaseName       string `json:"database_name,omitempty"`        // Database name
	DatabaseSchema     string `json:"database_schema,omitempty"`      // Schema name (for SQL databases)
	DatabaseTable      string `json:"database_table,omitempty"`       // Table or collection name
	DatabaseConnection string `json:"database_connection,omitempty"`  // Connection string

	// File-specific settings
	FileFormat    string `json:"file_format,omitempty"`     // "json", "xml", "hl7", "csv", etc.
	FilePattern   string `json:"file_pattern,omitempty"`    // File naming pattern with variables
	FileDirectory string `json:"file_directory,omitempty"`  // Target directory
	S3Bucket      string `json:"s3_bucket,omitempty"`       // S3 bucket name
	S3Region      string `json:"s3_region,omitempty"`       // AWS region
	AzureContainer string `json:"azure_container,omitempty"` // Azure blob container

	// Message Queue-specific settings
	QueueName       string `json:"queue_name,omitempty"`        // Queue/topic name
	QueueType       string `json:"queue_type,omitempty"`        // "kafka", "rabbitmq", "sqs", etc.
	KafkaTopic      string `json:"kafka_topic,omitempty"`       // Kafka topic
	KafkaBrokers    []string `json:"kafka_brokers,omitempty"`   // Kafka broker addresses
	RabbitMQExchange string `json:"rabbitmq_exchange,omitempty"` // RabbitMQ exchange
	RabbitMQRoutingKey string `json:"rabbitmq_routing_key,omitempty"` // RabbitMQ routing key

	// Type-specific configuration (flexible JSONB for future extensions)
	Config map[string]interface{} `json:"config,omitempty"` // Additional connector-specific settings
}

// GetTimeout returns the timeout in milliseconds, with default of 30 seconds
func (dc *DestinationConfig) GetTimeout() int {
	if dc.TimeoutMs > 0 {
		return dc.TimeoutMs
	}
	return 30000 // Default 30 seconds
}

// GetRetryPolicy returns the retry policy, with default if not specified
func (dc *DestinationConfig) GetRetryPolicy() RetryPolicy {
	// If retry policy is not configured, use defaults
	if dc.RetryPolicy.MaxAttempts == 0 {
		return DefaultRetryPolicy()
	}
	return dc.RetryPolicy
}

// BuildEndpointURL constructs the full endpoint URL from components
func (dc *DestinationConfig) BuildEndpointURL() string {
	// If full endpoint is provided, use it
	if dc.Endpoint != "" {
		return dc.Endpoint
	}

	// Build from components
	protocol := dc.Protocol
	if protocol == "" {
		protocol = "http" // Default
	}

	// For non-HTTP protocols, don't build URL
	if protocol != "http" && protocol != "https" {
		return ""
	}

	host := dc.Host
	if host == "" {
		host = "localhost"
	}

	// Build URL
	url := protocol + "://" + host

	// Add port if not default
	if dc.Port > 0 {
		if !(protocol == "http" && dc.Port == 80) && !(protocol == "https" && dc.Port == 443) {
			url += ":" + intToString(dc.Port)
		}
	}

	// Add path
	if dc.Path != "" {
		if dc.Path[0] != '/' {
			url += "/"
		}
		url += dc.Path
	}

	return url
}

// Helper function to convert int to string
func intToString(n int) string {
	if n == 0 {
		return "0"
	}

	// Handle negative numbers
	isNegative := false
	if n < 0 {
		isNegative = true
		n = -n
	}

	// Convert to string
	digits := []byte{}
	for n > 0 {
		digit := n % 10
		digits = append([]byte{byte('0' + digit)}, digits...)
		n /= 10
	}

	if isNegative {
		digits = append([]byte{'-'}, digits...)
	}

	return string(digits)
}

// GetContentType returns the content type, with default based on destination type
func (dc *DestinationConfig) GetContentType() string {
	if dc.ContentType != "" {
		return dc.ContentType
	}

	// Return sensible defaults based on type
	switch dc.Type {
	case "fhir":
		return "application/fhir+json"
	case "hl7":
		return "application/hl7-v2"
	case "api":
		return "application/json"
	default:
		return "application/json"
	}
}

// GetAcceptHeader returns the accept header, with default based on destination type
func (dc *DestinationConfig) GetAcceptHeader() string {
	if dc.AcceptHeader != "" {
		return dc.AcceptHeader
	}

	// Return sensible defaults based on type
	switch dc.Type {
	case "fhir":
		return "application/fhir+json"
	default:
		return "application/json"
	}
}

// Validate checks if the destination configuration is valid
func (dc *DestinationConfig) Validate() error {
	// Type is required
	if dc.Type == "" {
		return &ValidationError{Field: "type", Message: "destination type is required"}
	}

	// Protocol is required
	if dc.Protocol == "" {
		return &ValidationError{Field: "protocol", Message: "protocol is required"}
	}

	// Type-specific validation
	switch dc.Type {
	case "fhir", "api":
		// HTTP-based destinations need host or endpoint
		if dc.Host == "" && dc.Endpoint == "" {
			return &ValidationError{Field: "host", Message: "host or endpoint is required for " + dc.Type}
		}

	case "hl7":
		// TCP-based destinations need host and port
		if dc.Host == "" {
			return &ValidationError{Field: "host", Message: "host is required for HL7"}
		}
		if dc.Port == 0 {
			return &ValidationError{Field: "port", Message: "port is required for HL7"}
		}

	case "database":
		// Database destinations need connection details
		if dc.DatabaseConnection == "" && dc.Host == "" {
			return &ValidationError{Field: "host", Message: "host or database_connection is required for database"}
		}

	case "file":
		// File destinations need path or directory
		if dc.Path == "" && dc.FileDirectory == "" && dc.S3Bucket == "" && dc.AzureContainer == "" {
			return &ValidationError{Field: "path", Message: "file path, directory, or cloud storage location is required"}
		}

	case "queue":
		// Queue destinations need queue name
		if dc.QueueName == "" && dc.KafkaTopic == "" {
			return &ValidationError{Field: "queue_name", Message: "queue name or topic is required"}
		}
	}

	return nil
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return "validation error: " + e.Field + ": " + e.Message
}
