package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// PostgresTransformationService handles transformations using PostgreSQL atomicMappings
type PostgresTransformationService struct {
	db *sql.DB
}

// AtomicMapping represents a single field mapping from HL7 to FHIR
type AtomicMapping struct {
	ID             string `json:"id"`
	SourcePath     string `json:"sourcePath"`
	TargetPath     string `json:"targetPath"`
	TransformType  string `json:"transformType"`
	DefaultValue   string `json:"defaultValue,omitempty"`
	ValidationRule string `json:"validationRule,omitempty"`
	IsRequired     bool   `json:"isRequired"`
}

// TransformationConfig represents the complete transformation configuration
type TransformationConfig struct {
	AtomicMappings []AtomicMapping        `json:"atomicMappings"`
	DragDropUI     map[string]interface{} `json:"dragDropUI,omitempty"`
}

// PostgresTransformationResult represents the result of a PostgreSQL-based transformation
type PostgresTransformationResult struct {
	Success                bool                   `json:"success"`
	TransformedMessage     map[string]interface{} `json:"transformedMessage"`
	TransformationMetadata map[string]interface{} `json:"transformationMetadata"`
	ValidationErrors       map[string]interface{} `json:"validationErrors,omitempty"`
	FHIRResourceType       string                 `json:"fhirResourceType"`
	FHIRResourceID         string                 `json:"fhirResourceId"`
	ProcessingTimeMs       int64                  `json:"processingTimeMs"`
	ErrorMessage           string                 `json:"errorMessage,omitempty"`
}

// NewPostgresTransformationService creates a new PostgreSQL transformation service
func NewPostgresTransformationService(db *sql.DB) *PostgresTransformationService {
	return &PostgresTransformationService{
		db: db,
	}
}

// GetTransformationConfig retrieves the transformation configuration for an interface
func (pts *PostgresTransformationService) GetTransformationConfig(interfaceID, messageType string) (*TransformationConfig, error) {
	query := `
		SELECT custom_mapping_config
		FROM interface_message_mappings
		WHERE interface_id = $1 AND message_type = $2`

	var configJSON sql.NullString
	err := pts.db.QueryRow(query, interfaceID, messageType).Scan(&configJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no transformation config found for interface %s, message type %s", interfaceID, messageType)
		}
		return nil, fmt.Errorf("failed to query transformation config: %w", err)
	}

	if !configJSON.Valid || configJSON.String == "" {
		return &TransformationConfig{AtomicMappings: []AtomicMapping{}}, nil
	}

	var config TransformationConfig
	if err := json.Unmarshal([]byte(configJSON.String), &config); err != nil {
		return nil, fmt.Errorf("failed to parse transformation config JSON: %w", err)
	}

	log.Printf("📋 Loaded %d atomic mappings for interface %s, message type %s",
		len(config.AtomicMappings), interfaceID, messageType)

	return &config, nil
}

// ExecuteTransformation executes a transformation using PostgreSQL atomicMappings
func (pts *PostgresTransformationService) ExecuteTransformation(interfaceID, messageType string, sourceData map[string]interface{}) (*PostgresTransformationResult, error) {
	startTime := time.Now()

	log.Printf("🔄 Starting PostgreSQL transformation for interface %s, message type %s", interfaceID, messageType)

	// Get transformation configuration
	config, err := pts.GetTransformationConfig(interfaceID, messageType)
	if err != nil {
		return &PostgresTransformationResult{
			Success:          false,
			ErrorMessage:     fmt.Sprintf("Failed to load transformation config: %s", err.Error()),
			ProcessingTimeMs: time.Since(startTime).Milliseconds(),
		}, err
	}

	if len(config.AtomicMappings) == 0 {
		log.Printf("⚠️ No atomic mappings found for interface %s, message type %s", interfaceID, messageType)
		return &PostgresTransformationResult{
			Success:            false,
			ErrorMessage:       "No atomic mappings configured for this interface",
			ProcessingTimeMs:   time.Since(startTime).Milliseconds(),
		}, fmt.Errorf("no atomic mappings configured")
	}

	// Initialize FHIR Patient resource
	fhirPatient := map[string]interface{}{
		"resourceType": "Patient",
		"id":          fmt.Sprintf("patient-%d", time.Now().UnixNano()),
		"identifier":  []map[string]interface{}{},
		"name":        []map[string]interface{}{},
	}

	validationErrors := make(map[string]interface{})
	transformationMetadata := map[string]interface{}{
		"mappings_applied":  []string{},
		"mappings_skipped":  []string{},
		"processing_method": "postgres_atomic_mappings",
		"config_version":    "1.0",
	}

	appliedMappings := []string{}
	skippedMappings := []string{}

	// Apply each atomic mapping
	for _, mapping := range config.AtomicMappings {
		log.Printf("🔧 Applying mapping: %s -> %s", mapping.SourcePath, mapping.TargetPath)

		// Get source value using path traversal
		sourceValue := pts.getValueByPath(sourceData, mapping.SourcePath)

		if sourceValue == nil {
			if mapping.IsRequired {
				validationErrors[mapping.SourcePath] = fmt.Sprintf("Required field %s is missing", mapping.SourcePath)
				log.Printf("❌ Required field missing: %s", mapping.SourcePath)
			}

			// Use default value if available
			if mapping.DefaultValue != "" {
				sourceValue = mapping.DefaultValue
				log.Printf("🔄 Using default value for %s: %s", mapping.SourcePath, mapping.DefaultValue)
			} else {
				skippedMappings = append(skippedMappings, mapping.SourcePath+"->"+mapping.TargetPath)
				log.Printf("⏭️ Skipping mapping %s (no value, no default)", mapping.SourcePath)
				continue
			}
		}

		// Apply transformation based on target path
		err := pts.applyFHIRMapping(fhirPatient, mapping, sourceValue)
		if err != nil {
			log.Printf("❌ Failed to apply mapping %s->%s: %v", mapping.SourcePath, mapping.TargetPath, err)
			validationErrors[mapping.TargetPath] = err.Error()
		} else {
			appliedMappings = append(appliedMappings, mapping.SourcePath+"->"+mapping.TargetPath)
			log.Printf("✅ Applied mapping: %s -> %s (value: %v)", mapping.SourcePath, mapping.TargetPath, sourceValue)
		}
	}

	// Update metadata
	transformationMetadata["mappings_applied"] = appliedMappings
	transformationMetadata["mappings_skipped"] = skippedMappings
	transformationMetadata["total_mappings"] = len(config.AtomicMappings)
	transformationMetadata["applied_count"] = len(appliedMappings)

	processingTime := time.Since(startTime)
	success := len(validationErrors) == 0

	result := &PostgresTransformationResult{
		Success:                success,
		TransformedMessage:     fhirPatient,
		TransformationMetadata: transformationMetadata,
		ValidationErrors:       validationErrors,
		FHIRResourceType:       "Patient",
		FHIRResourceID:         fhirPatient["id"].(string),
		ProcessingTimeMs:       processingTime.Milliseconds(),
	}

	if !success {
		result.ErrorMessage = fmt.Sprintf("Transformation completed with %d validation errors", len(validationErrors))
	}

	log.Printf("✅ PostgreSQL transformation completed in %v (%d mappings applied, %d skipped)",
		processingTime, len(appliedMappings), len(skippedMappings))

	return result, nil
}

// getValueByPath retrieves a value from nested map using dot notation path
func (pts *PostgresTransformationService) getValueByPath(data map[string]interface{}, path string) interface{} {
	// Handle HL7 parsed data structure
	if parsed_hl7, ok := data["parsed_hl7"].(map[string]interface{}); ok {
		// For HL7 paths like "PID.3.1", look in parsed HL7 structure
		if path == "PID.3.1" {
			if pid, ok := parsed_hl7["PID"].(map[string]interface{}); ok {
				if field3, ok := pid["3"].([]interface{}); ok && len(field3) > 0 {
					if firstComponent, ok := field3[0].(map[string]interface{}); ok {
						return firstComponent["1"]
					}
				}
			}
		} else if path == "PID.5.1" {
			if pid, ok := parsed_hl7["PID"].(map[string]interface{}); ok {
				if field5, ok := pid["5"].([]interface{}); ok && len(field5) > 0 {
					if firstComponent, ok := field5[0].(map[string]interface{}); ok {
						return firstComponent["1"]
					}
				}
			}
		} else if path == "PID.5.2" {
			if pid, ok := parsed_hl7["PID"].(map[string]interface{}); ok {
				if field5, ok := pid["5"].([]interface{}); ok && len(field5) > 0 {
					if firstComponent, ok := field5[0].(map[string]interface{}); ok {
						return firstComponent["2"]
					}
				}
			}
		} else if path == "PID.7" {
			if pid, ok := parsed_hl7["PID"].(map[string]interface{}); ok {
				return pid["7"]
			}
		} else if path == "PID.8" {
			if pid, ok := parsed_hl7["PID"].(map[string]interface{}); ok {
				return pid["8"]
			}
		}
	}

	log.Printf("⚠️ Could not resolve path: %s", path)
	return nil
}

// applyFHIRMapping applies a single field mapping to the FHIR resource
func (pts *PostgresTransformationService) applyFHIRMapping(fhirResource map[string]interface{}, mapping AtomicMapping, value interface{}) error {
	switch mapping.TargetPath {
	case "Patient.identifier[0].value":
		// Ensure identifier array exists
		if identifiers, ok := fhirResource["identifier"].([]map[string]interface{}); ok {
			if len(identifiers) == 0 {
				// Create first identifier
				fhirResource["identifier"] = []map[string]interface{}{
					{
						"value": fmt.Sprintf("%v", value),
						"system": "http://hospital.example.org/patient-ids",
					},
				}
			} else {
				identifiers[0]["value"] = fmt.Sprintf("%v", value)
			}
		}

	case "Patient.name[0].family":
		// Ensure name array exists
		if names, ok := fhirResource["name"].([]map[string]interface{}); ok {
			if len(names) == 0 {
				// Create first name
				fhirResource["name"] = []map[string]interface{}{
					{
						"family": fmt.Sprintf("%v", value),
						"use": "official",
					},
				}
			} else {
				names[0]["family"] = fmt.Sprintf("%v", value)
			}
		}

	case "Patient.name[0].given[0]":
		// Ensure name array and given array exist
		if names, ok := fhirResource["name"].([]map[string]interface{}); ok {
			if len(names) == 0 {
				// Create first name with given
				fhirResource["name"] = []map[string]interface{}{
					{
						"given": []string{fmt.Sprintf("%v", value)},
						"use": "official",
					},
				}
			} else {
				// Ensure given array exists
				if _, hasGiven := names[0]["given"]; !hasGiven {
					names[0]["given"] = []string{}
				}
				if given, ok := names[0]["given"].([]string); ok {
					if len(given) == 0 {
						names[0]["given"] = []string{fmt.Sprintf("%v", value)}
					} else {
						given[0] = fmt.Sprintf("%v", value)
						names[0]["given"] = given
					}
				}
			}
		}

	case "Patient.birthDate":
		// Convert HL7 date format (YYYYMMDD) to FHIR date format (YYYY-MM-DD)
		dateStr := fmt.Sprintf("%v", value)
		if len(dateStr) == 8 {
			fhirDate := dateStr[0:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]
			fhirResource["birthDate"] = fhirDate
		} else {
			fhirResource["birthDate"] = dateStr
		}

	case "Patient.gender":
		// Convert HL7 gender codes to FHIR gender codes
		genderStr := fmt.Sprintf("%v", value)
		switch genderStr {
		case "M":
			fhirResource["gender"] = "male"
		case "F":
			fhirResource["gender"] = "female"
		case "O":
			fhirResource["gender"] = "other"
		case "U":
			fhirResource["gender"] = "unknown"
		default:
			fhirResource["gender"] = "unknown"
		}

	default:
		return fmt.Errorf("unsupported target path: %s", mapping.TargetPath)
	}

	return nil
}