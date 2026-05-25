package models

// VariableDefinition represents a variable that can be output by a transformation step
// Used for autocomplete, validation, and reference documentation in the UI
type VariableDefinition struct {
	// Name is the variable identifier (e.g., "insurance_id", "date_of_birth")
	Name string `json:"name"`

	// Path is the full XPath/JSONPath to access this variable (e.g., "enriched.database.insurance_id")
	Path string `json:"path"`

	// DataType indicates the expected type (string, number, boolean, object, array)
	DataType string `json:"data_type,omitempty"`

	// Description provides context about what this variable contains
	Description string `json:"description,omitempty"`

	// UsageExample shows how to access this variable in scripts
	UsageExample string `json:"usage_example,omitempty"`

	// SourceField indicates where this variable comes from (e.g., "PID.7" for HL7 fields)
	SourceField string `json:"source_field,omitempty"`

	// Examples contains sample values for this variable
	Examples []string `json:"examples,omitempty"`

	// Category groups related variables (e.g., "Patient", "Insurance", "Visit")
	Category string `json:"category,omitempty"`

	// Required indicates if this variable will always be present
	Required bool `json:"required,omitempty"`
}

// VariableDefinitionGroup groups variables by step or category
type VariableDefinitionGroup struct {
	StepName  string               `json:"step_name"`
	StepType  string               `json:"step_type"`
	StepIndex int                  `json:"step_index"`
	Variables []VariableDefinition `json:"variables"`
}
