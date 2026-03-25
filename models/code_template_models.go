package models

import "time"

// CodeTemplate represents a named JavaScript function library stored in the database.
// Active global templates are automatically injected into every script step's goja VM.
// Interface-scoped templates are only injected when linked via CodeTemplateInterfaceLink.
type CodeTemplate struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	Category           string    `json:"category"`           // general | hl7 | fhir | date_time | validation | string
	Code               string    `json:"code"`               // JavaScript source
	FunctionSignatures []string  `json:"function_signatures"` // ["formatHL7Date(hl7Date)", ...]
	Scope              string    `json:"scope"`              // global | interface
	IsSystem           bool      `json:"is_system"`
	IsActive           bool      `json:"is_active"`
	SortOrder          int       `json:"sort_order"`
	UsageCount         int       `json:"usage_count"`
	CreatedByUserID    *string   `json:"created_by_user_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// CodeTemplateInterfaceLink associates an interface-scoped template with a specific interface.
type CodeTemplateInterfaceLink struct {
	ID          string    `json:"id"`
	TemplateID  string    `json:"template_id"`
	InterfaceID string    `json:"interface_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// CodeTemplateForExecution is the lightweight projection used by the script executor hot-path.
// It intentionally omits metadata to keep the hot-path lean.
type CodeTemplateForExecution struct {
	ID        string
	Name      string
	Code      string
	SortOrder int
}
