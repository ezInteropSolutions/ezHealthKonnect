package models

import (
	"encoding/json"
	"time"
)

// TransformationPipeline represents a transformation pipeline for an interface/message type
type TransformationPipeline struct {
	ID           string    `json:"id" db:"id"`
	InterfaceID  string    `json:"interface_id" db:"interface_id"`
	MessageType  string    `json:"message_type" db:"message_type"`
	PipelineName string    `json:"pipeline_name" db:"pipeline_name"`
	Enabled      bool      `json:"enabled" db:"enabled"`
	Version      int       `json:"version" db:"version"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
	Steps        []TransformationStep `json:"steps,omitempty"`
}

// TransformationStep represents a single step in a transformation pipeline
type TransformationStep struct {
	ID              string                 `json:"id" db:"id"`
	PipelineID      string                 `json:"pipeline_id" db:"pipeline_id"`
	StepName        string                 `json:"step_name" db:"step_name"`
	StepType        string                 `json:"step_type" db:"step_type"` // pre.validation, core.mapping, post.validation, custom
	Sequence        int                    `json:"sequence" db:"sequence"`
	Layer           string                 `json:"layer" db:"layer"` // pre, core, post
	Required        bool                   `json:"required" db:"required"`
	TimeoutMs       int                    `json:"timeout_ms" db:"timeout_ms"`
	Enabled         bool                   `json:"enabled" db:"enabled"`
	Config          map[string]interface{} `json:"config" db:"config"`
	ScriptType      *string                `json:"script_type,omitempty" db:"script_type"` // javascript, lua
	ScriptContent   *string                `json:"script_content,omitempty" db:"script_content"`
	OnErrorStrategy string                 `json:"on_error_strategy" db:"on_error_strategy"` // fail, skip, default
	ExecutionMode   string                 `json:"execution_mode" db:"execution_mode"` // sequential, parallel
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
	StepType        string                 `json:"step_type"`
	StartedAt       time.Time              `json:"started_at"`
	CompletedAt     time.Time              `json:"completed_at"`
	DurationMs      int64                  `json:"duration_ms"`
	Success         bool                   `json:"success"`
	Error           string                 `json:"error,omitempty"`
	InputSnapshot   map[string]interface{} `json:"input_snapshot,omitempty"`
	OutputSnapshot  map[string]interface{} `json:"output_snapshot,omitempty"`
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
type TransformationExecutionResult struct {
	PipelineID    string                 `json:"pipeline_id"`
	CorrelationID string                 `json:"correlation_id"`
	Status        string                 `json:"status"` // in_progress, completed, failed, partial
	StartedAt     time.Time              `json:"started_at"`
	CompletedAt   time.Time              `json:"completed_at"`
	ExecutionTime time.Duration          `json:"execution_time"`
	Output        map[string]interface{} `json:"output"`         // Transformed content (FHIR bundle, JSON, etc.)
	DeliveryPayload *DeliveryPayload     `json:"delivery_payload,omitempty"` // Prepared for transmission
	Errors        []TransformationError  `json:"errors"`
}

// TransformationError represents an error during transformation execution
type TransformationError struct {
	Step      string    `json:"step"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Code      string    `json:"code,omitempty"`
	Details   string    `json:"details,omitempty"`
}
