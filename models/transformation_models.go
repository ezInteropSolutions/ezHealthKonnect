package models

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Context key for pipeline execution modes
type contextKey string

const (
	// ContextKeyTestMode indicates the pipeline is running in test/dry-run mode
	ContextKeyTestMode contextKey = "pipeline_test_mode"
)

// DeliveryStatusFn is injected into the pipeline context by the processing engine.
// OutboundConnectorExecutor calls it after each connector.Send() so that every
// outbound step owns its own delivery_status update in PostgreSQL.
// Signature: (interfaceID, messageID, status, detail string)
type DeliveryStatusFn func(interfaceID, messageID, status, detail string)

// GetDeliveryStatusFn reads the DeliveryStatusFn from context. Returns nil if not set.
func GetDeliveryStatusFn(ctx context.Context) DeliveryStatusFn {
	if fn, ok := ctx.Value("delivery_status_fn").(DeliveryStatusFn); ok {
		return fn
	}
	return nil
}

// StoreOutboundFn is injected into the pipeline context so OutboundConnectorExecutor
// can persist the exact payload sent to a connector without importing services/storage.
// Returns the storage URI (empty string on error).
type StoreOutboundFn func(interfaceID, messageID, content, contentType string) string

// GetStoreOutboundFn reads the StoreOutboundFn from context. Returns nil if not set.
func GetStoreOutboundFn(ctx context.Context) StoreOutboundFn {
	if fn, ok := ctx.Value("store_outbound_fn").(StoreOutboundFn); ok {
		return fn
	}
	return nil
}

// IsTestMode checks if the context indicates test mode execution
func IsTestMode(ctx context.Context) bool {
	if val, ok := ctx.Value(ContextKeyTestMode).(bool); ok {
		return val
	}
	return false
}

// WithTestMode returns a new context with test mode enabled
func WithTestMode(ctx context.Context) context.Context {
	return context.WithValue(ctx, ContextKeyTestMode, true)
}

// PipelineConnection represents a directed edge between two steps in the flowchart.
// Stored as JSONB in transformation_pipelines.connections (V42 migration).
type PipelineConnection struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type,omitempty"` // "default", "true", "false", case value — informational only
}

// TransformationPipeline represents a transformation pipeline for an interface/message type
type TransformationPipeline struct {
	ID             string                 `json:"id" db:"id"`
	InterfaceID    string                 `json:"interface_id" db:"interface_id"`
	MessageType    string                 `json:"message_type" db:"message_type"`
	PipelineName   string                 `json:"pipeline_name" db:"pipeline_name"`
	Enabled        bool                   `json:"enabled" db:"enabled"`
	Version        int                    `json:"version" db:"version"`
	PipelineConfig map[string]interface{} `json:"pipeline_config,omitempty" db:"pipeline_config"`
	Connections    []PipelineConnection   `json:"connections,omitempty"` // Flowchart edges (from V42)
	CreatedAt      time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at" db:"updated_at"`
	Steps          []TransformationStep   `json:"steps,omitempty"`
}

// TransformationStep represents a single step in a transformation pipeline
type TransformationStep struct {
	ID              string                 `json:"id" db:"id"`
	PipelineID      string                 `json:"pipeline_id" db:"pipeline_id"`
	StepName        string                 `json:"step_name" db:"step_name"`
	StepAlias       *string                `json:"step_alias,omitempty" db:"step_alias"` // User-defined alias for referencing step outputs
	StepType        string                 `json:"step_type" db:"step_type"`
	Sequence        int                    `json:"sequence" db:"sequence"`
	Required        bool                   `json:"required" db:"required"`
	TimeoutMs       int                    `json:"timeout_ms" db:"timeout_ms"`
	Enabled         bool                   `json:"enabled" db:"enabled"`
	Config          map[string]interface{} `json:"config" db:"config"`
	ScriptType      *string                `json:"script_type,omitempty" db:"script_type"` // javascript, lua
	ScriptContent   *string                `json:"script_content,omitempty" db:"script_content"`
	OnErrorStrategy string                 `json:"on_error_strategy" db:"on_error_strategy"` // fail, skip, default
	ExecutionMode   string                 `json:"execution_mode" db:"execution_mode"` // sequential, parallel

	// Canvas position (for visual layout persistence)
	PositionX       *float64               `json:"position_x,omitempty" db:"position_x"`
	PositionY       *float64               `json:"position_y,omitempty" db:"position_y"`

	// Conditional branch tracking (for visual layout persistence)
	// Tracks which conditional step this step is connected to and which branch
	ParentConditionalStepID *string        `json:"parent_conditional_step_id,omitempty" db:"parent_conditional_step_id"`
	BranchType              *string        `json:"branch_type,omitempty" db:"branch_type"`  // For IfThenElse: "true" or "false"
	CaseValue               *string        `json:"case_value,omitempty" db:"case_value"`    // For Switch/Case: the case value (e.g., "M", "F", "default")

	// Container system: tracks which container step this step belongs to
	ParentStepID  *string `json:"parent_step_id,omitempty" db:"parent_step_id"`    // ID of parent container (Loop, Try-Catch, Retry). NULL = top-level
	ContainerZone *string `json:"container_zone,omitempty" db:"container_zone"`    // Zone: "body" (Loop/Retry), "try"/"catch"/"finally" (Try-Catch)

	CreatedAt       time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at" db:"updated_at"`
}

// TransformationResult represents the result of a transformation pipeline execution
type TransformationResult struct {
	Success           bool                   `json:"success"`
	OutputData        map[string]interface{} `json:"output_data"`
	TransformationLog []StepExecutionLog     `json:"transformation_log"`
	TotalTimeMs       int64                  `json:"total_time_ms"`
	Error             string                 `json:"error,omitempty"`
}

// StepExecutionLog tracks execution of individual steps
type StepExecutionLog struct {
	StepID          string                 `json:"step_id"`
	StepName        string                 `json:"step_name"`
	StepAlias       string                 `json:"step_alias,omitempty"`    // User-friendly alias (e.g., "empi", "validate_pid")
	StepType        string                 `json:"step_type"`
	Namespace       string                 `json:"namespace,omitempty"`     // Generated namespace (e.g., "empi_b4c9f1")
	StartedAt       time.Time              `json:"started_at"`
	CompletedAt     time.Time              `json:"completed_at"`
	DurationMs      int64                  `json:"duration_ms"`
	Success         bool                   `json:"success"`
	Error           string                 `json:"error,omitempty"`
	InputSnapshot   map[string]interface{} `json:"input_snapshot,omitempty"`
	OutputSnapshot  map[string]interface{} `json:"output_snapshot,omitempty"`
	StepOutput      *StepOutput            `json:"step_output,omitempty"`   // Step-specific output data
}

// TransformationExecution tracks full pipeline execution in database
type TransformationExecution struct {
	ID                 string                 `json:"id" db:"id"`
	MessageID          string                 `json:"message_id" db:"message_id"`
	InterfaceID        string                 `json:"interface_id" db:"interface_id"`
	PipelineID         string                 `json:"pipeline_id" db:"pipeline_id"`
	StartedAt          time.Time              `json:"started_at" db:"started_at"`
	CompletedAt        *time.Time             `json:"completed_at,omitempty" db:"completed_at"`
	TotalTimeMs        int64                  `json:"total_time_ms" db:"total_time_ms"`
	Status             string                 `json:"status" db:"status"` // running, completed, failed
	StepsExecuted      int                    `json:"steps_executed" db:"steps_executed"`
	StepsFailed        int                    `json:"steps_failed" db:"steps_failed"`
	InputData          map[string]interface{} `json:"input_data,omitempty" db:"input_data"`
	OutputData         map[string]interface{} `json:"output_data,omitempty" db:"output_data"`
	ErrorMessage       string                 `json:"error_message,omitempty" db:"error_message"`
	ExecutionLog       []StepExecutionLog     `json:"execution_log,omitempty" db:"execution_log"`
}

// StepExecutionRecord tracks individual step execution in database
type StepExecutionRecord struct {
	ID              string                 `json:"id" db:"id"`
	ExecutionID     string                 `json:"execution_id" db:"execution_id"`
	StepID          string                 `json:"step_id" db:"step_id"`
	StepName        string                 `json:"step_name" db:"step_name"`
	StepType        string                 `json:"step_type" db:"step_type"`
	StartedAt       time.Time              `json:"started_at" db:"started_at"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty" db:"completed_at"`
	DurationMs      int64                  `json:"duration_ms" db:"duration_ms"`
	Status          string                 `json:"status" db:"status"` // running, completed, failed, skipped
	ErrorMessage    string                 `json:"error_message,omitempty" db:"error_message"`
	InputSnapshot   map[string]interface{} `json:"input_snapshot,omitempty" db:"input_snapshot"`
	OutputSnapshot  map[string]interface{} `json:"output_snapshot,omitempty" db:"output_snapshot"`
}

// Helper methods for JSON marshaling/unmarshaling config fields

// MarshalConfig converts config map to JSON bytes for database storage
func (ts *TransformationStep) MarshalConfig() ([]byte, error) {
	if ts.Config == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(ts.Config)
}

// UnmarshalConfig parses JSON bytes from database into config map
func (ts *TransformationStep) UnmarshalConfig(data []byte) error {
	if len(data) == 0 {
		ts.Config = make(map[string]interface{})
		return nil
	}
	return json.Unmarshal(data, &ts.Config)
}

// MarshalExecutionLog converts execution log to JSON bytes for database storage
func (te *TransformationExecution) MarshalExecutionLog() ([]byte, error) {
	if te.ExecutionLog == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(te.ExecutionLog)
}

// UnmarshalExecutionLog parses JSON bytes from database into execution log
func (te *TransformationExecution) UnmarshalExecutionLog(data []byte) error {
	if len(data) == 0 {
		te.ExecutionLog = []StepExecutionLog{}
		return nil
	}
	return json.Unmarshal(data, &te.ExecutionLog)
}

// TransformationExecutionResult represents the result of a pipeline execution (MVC + OOB)
// TransformationExecutionResult represents the complete result of a pipeline execution
type TransformationExecutionResult struct {
	// === IDENTIFICATION ===
	PipelineID    string `json:"pipeline_id"`
	CorrelationID string `json:"correlation_id"`

	// === STATUS ===
	Status string `json:"status"` // in_progress, completed, failed, partial

	// === TIMING ===
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     time.Time `json:"completed_at"`
	ExecutionTimeNs int64     `json:"execution_time_ns"` // Nanoseconds (explicit for clarity)

	// === DATA LINEAGE (Format-Agnostic) ===
	Input  *MessageContext `json:"input"`  // What came in
	Output *MessageContext `json:"output"` // What went out

	// === EXECUTION TRACKING ===
	ExecutionLog []StepExecutionLog `json:"execution_log"` // Step-by-step execution tracking

	// === DELIVERY ===
	DeliveryPayload *DeliveryPayload `json:"delivery_payload,omitempty"` // Prepared for transmission

	// === ERRORS ===
	Errors []TransformationError `json:"errors,omitempty"`

	// === STEP-LEVEL ERROR CONTEXT (Phase 3) ===
	// Per-step error state for UI display — keyed by step namespace.
	// Additive to ExecutionLog; enables fast O(1) lookup per step.
	StepErrors map[string]StepErrorDetail `json:"step_errors,omitempty"`
}

// StepErrorDetail captures how an individual step handled its error.
type StepErrorDetail struct {
	StepID           string   `json:"step_id"`
	StepName         string   `json:"step_name"`
	Namespace        string   `json:"namespace"`
	Error            string   `json:"error"`
	RetriesAttempted int      `json:"retries_attempted"`
	ErrorStrategy    string   `json:"error_strategy"` // "suppress" | "rethrow" | "default_applied"
	DefaultApplied   bool     `json:"default_applied"`
	PHIViolations    []string `json:"phi_violations,omitempty"` // fields not masked before egress
}

// MessageContext represents a message at any stage of transformation (input or output)
// Supports any format: HL7v2, FHIR, JSON, XML, EDI X12, CSV, etc.
type MessageContext struct {
	// Format identification
	Format      string `json:"format"`                 // hl7v2, fhir-r4, json, xml, edi-x12, csv, etc.
	ContentType string `json:"content_type,omitempty"` // application/json, text/xml, etc.

	// Message metadata
	MessageType string `json:"message_type,omitempty"` // ADT^A01, Bundle, OrderRequest, etc.
	Version     string `json:"version,omitempty"`      // 2.5, 4.0.1, etc.

	// Size tracking
	SizeBytes int64 `json:"size_bytes"`

	// Timestamps — pointers so zero values are omitted from JSON output.
	ReceivedAt    *time.Time `json:"received_at,omitempty"`
	TransformedAt *time.Time `json:"transformed_at,omitempty"`

	// Storage reference (MongoDB document ID or S3 key)
	PayloadRef  string `json:"payload_ref,omitempty"`  // Reference to full payload in MongoDB
	PayloadHash string `json:"payload_hash,omitempty"` // SHA256 hash for integrity verification

	// Actual payload (included for small messages, omitted for large ones stored in MongoDB)
	Payload interface{} `json:"payload,omitempty"`

	// Format-specific metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// TransformationError represents an error during transformation execution
type TransformationError struct {
	Step      string    `json:"step"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Code      string    `json:"code,omitempty"`
	Details   string    `json:"details,omitempty"`
}

// PipelineExecutionContext represents the full execution context with step outputs
type PipelineExecutionContext struct {
	Message         map[string]interface{}    `json:"message"`          // The actual message being transformed
	StepOutputs     map[string]StepOutput     `json:"step_outputs"`     // Namespaced step-specific outputs
	Metadata        map[string]interface{}    `json:"metadata"`         // Pipeline execution metadata
	VariableContext *PipelineVariableContext  `json:"variable_context"` // Flat namespace variable registry (NO-CODE)
}

// StepOutput represents output from a single step execution
// STANDARDIZED STRUCTURE:
//   - OutputData: Contains ONLY user-created variables (e.g., {male_case: "1"})
//   - ExecutionDetails: Contains step-specific execution info (e.g., {field_evaluated: "PID.8", case_matched: true})
//   - Success/DurationMs/Error: Standard execution status fields
// The controller merges ExecutionDetails into step_metadata along with Success/DurationMs
type StepOutput struct {
	StepID           string                 `json:"step_id"`                     // Full UUID of the step
	StepName         string                 `json:"step_name"`                   // Human-readable step name
	StepAlias        string                 `json:"step_alias"`                  // User-friendly alias (e.g., "empi")
	StepType         string                 `json:"step_type"`                   // Executor type (pre.enrichment.api, etc.)
	Namespace        string                 `json:"namespace"`                   // "alias_shortID" (e.g., "empi_b4c9f1")
	Sequence         int                    `json:"sequence"`                    // Step execution order
	OutputData       map[string]interface{} `json:"output_data"`                 // Step-specific output data (variables only)
	ExecutionDetails map[string]interface{} `json:"execution_details,omitempty"` // Step-specific execution info (for step_metadata)
	Success          bool                   `json:"success"`                     // Execution success status
	Error            string                 `json:"error,omitempty"`
	DurationMs       int64                  `json:"duration_ms"` // Execution time in milliseconds
}

// GetStepOutputByAlias retrieves step output by its user-friendly alias
func (ctx *PipelineExecutionContext) GetStepOutputByAlias(alias string) (*StepOutput, error) {
	// Try exact namespace match first (user provided full namespace)
	if output, exists := ctx.StepOutputs[alias]; exists {
		return &output, nil
	}

	// Try alias prefix match (alias without shortID)
	// "empi" matches "empi_b4c9f1"
	for namespace, output := range ctx.StepOutputs {
		parts := strings.Split(namespace, "_")
		if len(parts) >= 2 {
			// Extract alias part (everything except the last part which is the shortID)
			namespaceAlias := strings.Join(parts[:len(parts)-1], "_")
			if namespaceAlias == alias {
				return &output, nil
			}
		}
	}

	return nil, fmt.Errorf("step output not found for alias: %s", alias)
}

// GenerateStepNamespace generates a namespace from step name, ID, and optional alias
func GenerateStepNamespace(stepName string, stepID string, alias *string) string {
	var aliasStr string

	if alias != nil && *alias != "" {
		// User provided custom alias
		aliasStr = *alias
	} else {
		// Generate smart default from step name
		aliasStr = GenerateDefaultAlias(stepName)
	}

	// Sanitize alias (lowercase, replace spaces with underscores, remove special chars)
	sanitized := strings.ToLower(strings.ReplaceAll(aliasStr, " ", "_"))
	sanitized = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return -1 // Remove non-alphanumeric/underscore characters
	}, sanitized)

	// Use first 6 chars of step ID for uniqueness
	shortID := stepID
	if len(stepID) > 6 {
		shortID = stepID[:6]
	}

	return fmt.Sprintf("%s_%s", sanitized, shortID)
}

// NormalizeStepKey converts a step name to a standardized output key
// This ensures consistent naming between step names and output keys
// Examples:
//   - "Field Mapping" -> "field_mapping"
//   - "API Enrichment" -> "api"
//   - "Script Enrichment" -> "script"
//   - "Database Enrichment" -> "database"
func NormalizeStepKey(stepName string) string {
	// Convert to lowercase
	key := strings.ToLower(stepName)

	// Replace spaces with underscores
	key = strings.ReplaceAll(key, " ", "_")

	// Remove "enrichment" suffix for enrichment steps (shorter keys)
	key = strings.TrimSuffix(key, "_enrichment")

	// Special cases for common step types
	switch key {
	case "field_mapping", "core_transformation":
		return "field_mapping"
	case "metadata":
		return "metadata"
	case "api":
		return "api"
	case "database":
		return "database"
	case "script":
		return "script"
	case "calculated":
		return "calculated"
	default:
		return key
	}
}

// GenerateDefaultAlias generates a smart default alias from a step name
// Uses NormalizeStepKey for consistent naming across the system
func GenerateDefaultAlias(stepName string) string {
	// Use the centralized normalization function for consistency
	// This ensures "Field Mapping" → "field_mapping", "Script Enrichment" → "script", etc.
	return NormalizeStepKey(stepName)
}
