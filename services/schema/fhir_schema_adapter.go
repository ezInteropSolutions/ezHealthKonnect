// services/schema/fhir_schema_adapter.go
// SIMPLIFIED VERSION - No conflicts with existing code

package schema

import (
	"database/sql"
	"log"

	"ezhealthkonnect/fhir" // Your existing FHIR package
)

// =====================================
// SIMPLIFIED SCHEMA ADAPTER
// =====================================

type FHIRSchemaAdapter struct {
	db              *sql.DB
	fhirLoader      *fhir.FHIRSchemaLoader
	schemaDirectory string
}

// =====================================
// CONSTRUCTOR
// =====================================

func NewFHIRSchemaAdapter(db *sql.DB, schemaDirectory string) *FHIRSchemaAdapter {
	adapter := &FHIRSchemaAdapter{
		db:              db,
		schemaDirectory: schemaDirectory,
	}

	log.Printf("🔧 Initializing FHIR Schema Adapter with directory: %s", schemaDirectory)

	// Try to get existing FHIR schema loader
	adapter.fhirLoader = fhir.GetFHIRSchemaLoader()
	if adapter.fhirLoader == nil {
		log.Printf("⚠️ FHIR schema loader not initialized, will skip schema validation")
	} else {
		log.Printf("✅ FHIR schema loader found and connected")
	}

	return adapter
}

// =====================================
// SIMPLIFIED VALIDATION (No Schema Dependencies)
// =====================================

func (adapter *FHIRSchemaAdapter) ValidateResource(resource map[string]interface{}) []ValidationIssue {
	var issues []ValidationIssue

	resourceType, ok := resource["resourceType"].(string)
	if !ok {
		issues = append(issues, ValidationIssue{
			Severity: "error",
			Code:     "structure",
			Message:  "Resource must have a resourceType",
		})
		return issues
	}

	// Basic validation - check for narrative (dom-6 constraint)
	if _, hasText := resource["text"]; !hasText {
		issues = append(issues, ValidationIssue{
			Severity:     "warning",
			Code:         "dom-6",
			Message:      "A resource should have narrative for robust management",
			ResourceType: resourceType,
			Path:         resourceType,
		})
	}

	// Add more basic validations as needed
	log.Printf("🔍 Basic validation completed for %s: %d issues", resourceType, len(issues))
	return issues
}

// =====================================
// UTILITY METHODS
// =====================================

func (adapter *FHIRSchemaAdapter) IsSchemaAvailable() bool {
	return adapter.fhirLoader != nil
}

func (adapter *FHIRSchemaAdapter) GetSchemaDirectory() string {
	return adapter.schemaDirectory
}

// =====================================
// VALIDATION ISSUE TYPE
// =====================================

type ValidationIssue struct {
	Severity     string `json:"severity"`
	Code         string `json:"code"`
	Message      string `json:"message"`
	ResourceType string `json:"resourceType,omitempty"`
	Path         string `json:"path,omitempty"`
}
