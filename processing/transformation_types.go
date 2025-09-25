// processing/transformation_types.go
// Data structures for the transformation system

package processing

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TransformationPipeline represents a complete transformation configuration for an interface
type TransformationPipeline struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	InterfaceID     string             `bson:"interface_id" json:"interface_id"`
	Name            string             `bson:"name" json:"name"`
	Description     string             `bson:"description,omitempty" json:"description,omitempty"`
	SourceFormat    string             `bson:"source_format" json:"source_format"` // "hl7", "json", "xml", etc.
	TargetFormat    string             `bson:"target_format" json:"target_format"` // "fhir", "hl7", "json", etc.
	Steps           []TransformationStep `bson:"steps" json:"steps"`
	IsActive        bool               `bson:"is_active" json:"is_active"`
	CreatedAt       time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time          `bson:"updated_at" json:"updated_at"`
	CreatedBy       string             `bson:"created_by,omitempty" json:"created_by,omitempty"`
	Version         int                `bson:"version" json:"version"`
}

// TransformationStep represents a single transformation operation
type TransformationStep struct {
	ID          string                 `bson:"id" json:"id"`
	Name        string                 `bson:"name" json:"name"`
	StepType    TransformationStepType `bson:"step_type" json:"step_type"`
	Order       int                    `bson:"order" json:"order"`
	Config      map[string]interface{} `bson:"config" json:"config"`
	Conditions  []StepCondition        `bson:"conditions,omitempty" json:"conditions,omitempty"`
	OnError     ErrorHandling          `bson:"on_error" json:"on_error"`
	IsActive    bool                   `bson:"is_active" json:"is_active"`
	Description string                 `bson:"description,omitempty" json:"description,omitempty"`
}

// TransformationStepType defines the types of transformation steps
type TransformationStepType string

const (
	// Core transformation types
	StepTypeFieldMapping     TransformationStepType = "field_mapping"
	StepTypeValueTransform   TransformationStepType = "value_transform"
	StepTypeConditionalLogic TransformationStepType = "conditional_logic"
	StepTypeDataEnrichment   TransformationStepType = "data_enrichment"
	StepTypeValidation       TransformationStepType = "validation"
	StepTypeCustomCode       TransformationStepType = "custom_code"

	// Specialized types
	StepTypeHL7Parse         TransformationStepType = "hl7_parse"
	StepTypeFHIRBuild        TransformationStepType = "fhir_build"
	StepTypeCodeLookup       TransformationStepType = "code_lookup"
	StepTypeTemplateApply    TransformationStepType = "template_apply"
)

// StepCondition defines when a step should execute
type StepCondition struct {
	Field    string      `bson:"field" json:"field"`
	Operator string      `bson:"operator" json:"operator"` // "eq", "ne", "contains", "exists", etc.
	Value    interface{} `bson:"value" json:"value"`
	LogicOp  string      `bson:"logic_op,omitempty" json:"logic_op,omitempty"` // "and", "or"
}

// ErrorHandling defines how to handle step errors
type ErrorHandling struct {
	Action      string `bson:"action" json:"action"`           // "skip", "fail", "retry", "default"
	MaxRetries  int    `bson:"max_retries,omitempty" json:"max_retries,omitempty"`
	DefaultValue interface{} `bson:"default_value,omitempty" json:"default_value,omitempty"`
}

// TransformationTemplate represents reusable transformation patterns
type TransformationTemplate struct {
	ID            primitive.ObjectID   `bson:"_id,omitempty" json:"id,omitempty"`
	Name          string               `bson:"name" json:"name"`
	Description   string               `bson:"description,omitempty" json:"description,omitempty"`
	SourceFormat  string               `bson:"source_format" json:"source_format"`
	TargetFormat  string               `bson:"target_format" json:"target_format"`
	MessageType   string               `bson:"message_type,omitempty" json:"message_type,omitempty"` // e.g., "ADT^A01"
	Steps         []TransformationStep `bson:"steps" json:"steps"`
	IsPublic      bool                 `bson:"is_public" json:"is_public"`
	Category      string               `bson:"category,omitempty" json:"category,omitempty"`
	CreatedAt     time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt     time.Time            `bson:"updated_at" json:"updated_at"`
	UsageCount    int                  `bson:"usage_count" json:"usage_count"`
	Version       string               `bson:"version" json:"version"`
}

// ValueMap represents lookup tables for value transformations
type ValueMap struct {
	ID          primitive.ObjectID     `bson:"_id,omitempty" json:"id,omitempty"`
	Name        string                 `bson:"name" json:"name"`
	Description string                 `bson:"description,omitempty" json:"description,omitempty"`
	Category    string                 `bson:"category" json:"category"` // "gender", "country", "diagnosis_codes", etc.
	Mappings    map[string]interface{} `bson:"mappings" json:"mappings"`
	IsActive    bool                   `bson:"is_active" json:"is_active"`
	CreatedAt   time.Time              `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time              `bson:"updated_at" json:"updated_at"`
}

// TransformationContext holds the state during message processing
type TransformationContext struct {
	MessageID         string                 `json:"message_id"`
	InterfaceID       string                 `json:"interface_id"`
	SourceMessage     map[string]interface{} `json:"source_message"`
	TargetMessage     map[string]interface{} `json:"target_message"`
	Variables         map[string]interface{} `json:"variables"`
	ErrorMessages     []string               `json:"error_messages,omitempty"`
	ProcessedSteps    []string               `json:"processed_steps"`
	StartTime         time.Time              `json:"start_time"`
	CurrentStepIndex  int                    `json:"current_step_index"`
	IsComplete        bool                   `json:"is_complete"`
}

// TransformationResult represents the outcome of a transformation
type TransformationResult struct {
	Success         bool                   `json:"success"`
	TransformedData map[string]interface{} `json:"transformed_data,omitempty"`
	ErrorMessages   []string               `json:"error_messages,omitempty"`
	ProcessingTime  time.Duration          `json:"processing_time"`
	StepsExecuted   int                    `json:"steps_executed"`
	Context         *TransformationContext `json:"context,omitempty"`
}

// Common field mapping configurations
type FieldMappingConfig struct {
	SourcePath      string      `bson:"source_path" json:"source_path"`
	TargetPath      string      `bson:"target_path" json:"target_path"`
	DefaultValue    interface{} `bson:"default_value,omitempty" json:"default_value,omitempty"`
	Required        bool        `bson:"required" json:"required"`
	DataType        string      `bson:"data_type,omitempty" json:"data_type,omitempty"`
	Format          string      `bson:"format,omitempty" json:"format,omitempty"`
	ValueMapID      string      `bson:"value_map_id,omitempty" json:"value_map_id,omitempty"`
}

// Value transformation configurations
type ValueTransformConfig struct {
	Operation    string      `bson:"operation" json:"operation"` // "map", "format", "calculate", "concat"
	Parameters   map[string]interface{} `bson:"parameters" json:"parameters"`
	ValueMapID   string      `bson:"value_map_id,omitempty" json:"value_map_id,omitempty"`
	Expression   string      `bson:"expression,omitempty" json:"expression,omitempty"`
}

// Custom code execution configuration
type CustomCodeConfig struct {
	Language    string            `bson:"language" json:"language"` // "javascript", "go"
	Code        string            `bson:"code" json:"code"`
	Timeout     time.Duration     `bson:"timeout" json:"timeout"`
	Environment map[string]string `bson:"environment,omitempty" json:"environment,omitempty"`
	Sandbox     bool              `bson:"sandbox" json:"sandbox"`
}