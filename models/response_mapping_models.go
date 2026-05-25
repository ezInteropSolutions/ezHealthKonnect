package models

import (
	"encoding/json"
	"time"
)

// ResponseMappingTemplate represents a reusable template for mapping API responses
type ResponseMappingTemplate struct {
	ID               string          `json:"id" db:"id"`
	TemplateName     string          `json:"template_name" db:"template_name"`
	Description      *string         `json:"description,omitempty" db:"description"`
	APIType          *string         `json:"api_type,omitempty" db:"api_type"`
	Vendor           *string         `json:"vendor,omitempty" db:"vendor"`
	MappingRules     json.RawMessage `json:"mapping_rules" db:"mapping_rules"`
	IsSystemTemplate bool            `json:"is_system_template" db:"is_system_template"`
	CreatedBy        *string         `json:"created_by,omitempty" db:"created_by"`
	OrganizationID   *string         `json:"organization_id,omitempty" db:"organization_id"`
	Version          int             `json:"version" db:"version"`
	IsActive         bool            `json:"is_active" db:"is_active"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at" db:"updated_at"`
}

// ResponseMappingRule represents a single extraction/transformation rule
type ResponseMappingRule struct {
	SourcePath      string                 `json:"sourcePath"`      // JSONPath expression
	TargetField     string                 `json:"targetField"`     // Where to store the extracted value
	TransformType   string                 `json:"transformType"`   // none, combine, filter, format, conditional, javascript
	TransformConfig map[string]interface{} `json:"transformConfig,omitempty"` // Transform-specific configuration
	Required        bool                   `json:"required,omitempty"`
	DefaultValue    interface{}            `json:"defaultValue,omitempty"`
	Description     string                 `json:"description,omitempty"`
}

// ResponseMappingConfig represents the response mapping configuration in step config
type ResponseMappingConfig struct {
	Mode             string                `json:"mode"`                       // template, custom, extend, override
	TemplateID       string                `json:"templateId,omitempty"`       // Reference to template
	Extractors       []ResponseMappingRule `json:"extractors,omitempty"`       // Custom extractors (mode=custom)
	CustomExtractors []ResponseMappingRule `json:"customExtractors,omitempty"` // Additional extractors (mode=extend)
	Overrides        map[string]interface{} `json:"overrides,omitempty"`        // Rule overrides (mode=override)
}

// ParsedMappingRules parses the JSONB mapping_rules into a slice of ResponseMappingRule
func (t *ResponseMappingTemplate) ParsedMappingRules() ([]ResponseMappingRule, error) {
	var rules []ResponseMappingRule
	if err := json.Unmarshal(t.MappingRules, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// TransformType constants
const (
	TransformNone        = "none"        // Direct extraction
	TransformCombine     = "combine"     // Combine multiple fields
	TransformFilter      = "filter"      // Filter array by condition
	TransformFormat      = "format"      // Format dates, numbers, strings
	TransformConditional = "conditional" // If-then-else logic
	TransformJavaScript  = "javascript"  // Custom JavaScript (future)
)

// MappingMode constants
const (
	MappingModeTemplate = "template" // Use template as-is
	MappingModeCustom   = "custom"   // Use custom extractors only
	MappingModeExtend   = "extend"   // Use template + add custom
	MappingModeOverride = "override" // Use template with overrides
)

// CombineTransformConfig for combining multiple fields
type CombineTransformConfig struct {
	AdditionalPaths []string `json:"additionalPaths"` // Additional JSONPath expressions
	Separator       string   `json:"separator"`       // Separator between values
	Format          string   `json:"format,omitempty"` // Optional format string (e.g., "{0} {1}")
}

// FilterTransformConfig for filtering arrays
type FilterTransformConfig struct {
	FilterField    string      `json:"filterField"`    // Field to filter on
	FilterOperator string      `json:"filterOperator"` // equals, contains, startsWith, endsWith, gt, lt
	FilterValue    interface{} `json:"filterValue"`    // Value to compare
	ExtractField   string      `json:"extractField"`   // Field to extract after filtering
}

// FormatTransformConfig for formatting values
type FormatTransformConfig struct {
	InputFormat  string `json:"inputFormat,omitempty"`  // For date parsing
	OutputFormat string `json:"outputFormat,omitempty"` // For date formatting
	FormatType   string `json:"formatType,omitempty"`   // date, number, string
}

// ConditionalTransformConfig for conditional extraction
type ConditionalTransformConfig struct {
	Conditions []ConditionalRule `json:"conditions"`
	Default    interface{}       `json:"default,omitempty"`
}

// ConditionalRule represents a single if-then rule
type ConditionalRule struct {
	If   ConditionCheck `json:"if"`
	Then FieldExtract   `json:"then"`
}

// ConditionCheck represents a condition to check
type ConditionCheck struct {
	Field    string      `json:"field"`    // JSONPath to field
	Operator string      `json:"operator"` // equals, contains, gt, lt, etc.
	Value    interface{} `json:"value"`    // Value to compare
}

// FieldExtract represents field extraction in conditional
type FieldExtract struct {
	Field string      `json:"field,omitempty"` // JSONPath to extract
	Value interface{} `json:"value,omitempty"` // Static value
}

// CreateTemplateRequest for creating new templates
type CreateTemplateRequest struct {
	TemplateName   string                `json:"template_name" binding:"required"`
	Description    string                `json:"description"`
	APIType        string                `json:"api_type"`
	Vendor         string                `json:"vendor"`
	MappingRules   []ResponseMappingRule `json:"mapping_rules" binding:"required"`
	OrganizationID *string               `json:"organization_id,omitempty"`
}

// UpdateTemplateRequest for updating templates
type UpdateTemplateRequest struct {
	TemplateName   string                `json:"template_name"`
	Description    *string               `json:"description"`
	APIType        *string               `json:"api_type"`
	Vendor         *string               `json:"vendor"`
	MappingRules   []ResponseMappingRule `json:"mapping_rules"`
	IsActive       *bool                 `json:"is_active"`
	OrganizationID *string               `json:"organization_id"`
}

// TemplateListResponse for listing templates
type TemplateListResponse struct {
	Templates  []ResponseMappingTemplate `json:"templates"`
	TotalCount int                       `json:"total_count"`
}

// TemplateUsageInfo shows where a template is being used
type TemplateUsageInfo struct {
	InterfaceName string `json:"interface_name"`
	PipelineName  string `json:"pipeline_name"`
	StepName      string `json:"step_name"`
	StepID        string `json:"step_id"`
	MappingMode   string `json:"mapping_mode"`
}
