package response

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"

	"github.com/google/uuid"
)

// ResponseMappingService handles CRUD operations for response mapping templates
type ResponseMappingService struct {
	db *sql.DB
}

// NewResponseMappingService creates a new response mapping service
func NewResponseMappingService(db *sql.DB) *ResponseMappingService {
	return &ResponseMappingService{db: db}
}

// CreateTemplate creates a new response mapping template
func (s *ResponseMappingService) CreateTemplate(req models.CreateTemplateRequest, userID string) (*models.ResponseMappingTemplate, error) {
	// Marshal mapping rules to JSONB
	rulesJSON, err := json.Marshal(req.MappingRules)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal mapping rules: %w", err)
	}

	template := &models.ResponseMappingTemplate{
		ID:               uuid.New().String(),
		TemplateName:     req.TemplateName,
		Description:      &req.Description,
		APIType:          &req.APIType,
		Vendor:           &req.Vendor,
		MappingRules:     rulesJSON,
		IsSystemTemplate: false,
		CreatedBy:        &userID,
		OrganizationID:   req.OrganizationID,
		Version:          1,
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	query := `
		INSERT INTO response_mapping_templates (
			id, template_name, description, api_type, vendor,
			mapping_rules, is_system_template, created_by, organization_id,
			version, is_active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at, updated_at
	`

	err = s.db.QueryRow(
		query,
		template.ID,
		template.TemplateName,
		template.Description,
		template.APIType,
		template.Vendor,
		template.MappingRules,
		template.IsSystemTemplate,
		template.CreatedBy,
		template.OrganizationID,
		template.Version,
		template.IsActive,
		template.CreatedAt,
		template.UpdatedAt,
	).Scan(&template.ID, &template.CreatedAt, &template.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create template: %w", err)
	}

	log.Printf("✅ Created response mapping template: %s (ID: %s)", template.TemplateName, template.ID)
	return template, nil
}

// GetTemplateByID retrieves a template by ID
func (s *ResponseMappingService) GetTemplateByID(templateID string) (*models.ResponseMappingTemplate, error) {
	query := `
		SELECT
			id, template_name, description, api_type, vendor,
			mapping_rules, is_system_template, created_by, organization_id,
			version, is_active, created_at, updated_at
		FROM response_mapping_templates
		WHERE id = $1
	`

	template := &models.ResponseMappingTemplate{}
	err := s.db.QueryRow(query, templateID).Scan(
		&template.ID,
		&template.TemplateName,
		&template.Description,
		&template.APIType,
		&template.Vendor,
		&template.MappingRules,
		&template.IsSystemTemplate,
		&template.CreatedBy,
		&template.OrganizationID,
		&template.Version,
		&template.IsActive,
		&template.CreatedAt,
		&template.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("template not found: %s", templateID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}

	return template, nil
}

// ListTemplates lists all templates with optional filters
func (s *ResponseMappingService) ListTemplates(apiType, vendor, userID, orgID string, includeSystem bool) ([]models.ResponseMappingTemplate, error) {
	query := `
		SELECT
			id, template_name, description, api_type, vendor,
			mapping_rules, is_system_template, created_by, organization_id,
			version, is_active, created_at, updated_at
		FROM response_mapping_templates
		WHERE is_active = true
	`

	args := []interface{}{}
	argCount := 0

	// Filter by API type
	if apiType != "" {
		argCount++
		query += fmt.Sprintf(" AND api_type = $%d", argCount)
		args = append(args, apiType)
	}

	// Filter by vendor
	if vendor != "" {
		argCount++
		query += fmt.Sprintf(" AND vendor = $%d", argCount)
		args = append(args, vendor)
	}

	// Access control: show system templates + user's templates + org templates
	if !includeSystem {
		argCount++
		query += fmt.Sprintf(" AND (is_system_template = true OR created_by = $%d", argCount)
		args = append(args, userID)

		if orgID != "" {
			argCount++
			query += fmt.Sprintf(" OR organization_id = $%d", argCount)
			args = append(args, orgID)
		}
		query += ")"
	}

	query += " ORDER BY is_system_template DESC, template_name ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list templates: %w", err)
	}
	defer rows.Close()

	templates := []models.ResponseMappingTemplate{}
	for rows.Next() {
		template := models.ResponseMappingTemplate{}
		err := rows.Scan(
			&template.ID,
			&template.TemplateName,
			&template.Description,
			&template.APIType,
			&template.Vendor,
			&template.MappingRules,
			&template.IsSystemTemplate,
			&template.CreatedBy,
			&template.OrganizationID,
			&template.Version,
			&template.IsActive,
			&template.CreatedAt,
			&template.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan template: %w", err)
		}
		templates = append(templates, template)
	}

	return templates, nil
}

// UpdateTemplate updates an existing template
func (s *ResponseMappingService) UpdateTemplate(templateID string, req models.UpdateTemplateRequest, userID string) (*models.ResponseMappingTemplate, error) {
	// Get existing template
	existing, err := s.GetTemplateByID(templateID)
	if err != nil {
		return nil, err
	}

	// Check permissions (can't update system templates unless admin)
	if existing.IsSystemTemplate {
		return nil, fmt.Errorf("cannot modify system templates")
	}
	if existing.CreatedBy != nil && *existing.CreatedBy != userID {
		return nil, fmt.Errorf("unauthorized: not the template creator")
	}

	// Build dynamic update query
	query := "UPDATE response_mapping_templates SET updated_at = $1"
	args := []interface{}{time.Now()}
	argCount := 1

	if req.TemplateName != "" {
		argCount++
		query += fmt.Sprintf(", template_name = $%d", argCount)
		args = append(args, req.TemplateName)
	}

	if req.Description != nil {
		argCount++
		query += fmt.Sprintf(", description = $%d", argCount)
		args = append(args, *req.Description)
	}

	if req.APIType != nil {
		argCount++
		query += fmt.Sprintf(", api_type = $%d", argCount)
		args = append(args, *req.APIType)
	}

	if req.Vendor != nil {
		argCount++
		query += fmt.Sprintf(", vendor = $%d", argCount)
		args = append(args, *req.Vendor)
	}

	if req.MappingRules != nil {
		rulesJSON, err := json.Marshal(req.MappingRules)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal mapping rules: %w", err)
		}
		argCount++
		query += fmt.Sprintf(", mapping_rules = $%d", argCount)
		args = append(args, rulesJSON)
	}

	if req.IsActive != nil {
		argCount++
		query += fmt.Sprintf(", is_active = $%d", argCount)
		args = append(args, *req.IsActive)
	}

	argCount++
	query += fmt.Sprintf(" WHERE id = $%d RETURNING updated_at", argCount)
	args = append(args, templateID)

	var updatedAt time.Time
	err = s.db.QueryRow(query, args...).Scan(&updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to update template: %w", err)
	}

	log.Printf("✅ Updated response mapping template: %s", templateID)
	return s.GetTemplateByID(templateID)
}

// DeleteTemplate soft-deletes a template (sets is_active = false)
func (s *ResponseMappingService) DeleteTemplate(templateID, userID string) error {
	// Get existing template
	existing, err := s.GetTemplateByID(templateID)
	if err != nil {
		return err
	}

	// Check permissions
	if existing.IsSystemTemplate {
		return fmt.Errorf("cannot delete system templates")
	}
	if existing.CreatedBy != nil && *existing.CreatedBy != userID {
		return fmt.Errorf("unauthorized: not the template creator")
	}

	query := `
		UPDATE response_mapping_templates
		SET is_active = false, updated_at = $1
		WHERE id = $2
	`

	_, err = s.db.Exec(query, time.Now(), templateID)
	if err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}

	log.Printf("✅ Deleted response mapping template: %s", templateID)
	return nil
}

// GetTemplateUsage finds all steps using a specific template
func (s *ResponseMappingService) GetTemplateUsage(templateID string) ([]models.TemplateUsageInfo, error) {
	query := `
		SELECT
			i.interface_name,
			tp.pipeline_name,
			ts.step_name,
			ts.id as step_id,
			ts.config->>'responseMapping'->>'mode' as mapping_mode
		FROM transformation_steps ts
		JOIN transformation_pipelines tp ON ts.pipeline_id = tp.id
		JOIN interfaces i ON tp.interface_id = i.id
		WHERE ts.config->'responseMapping'->>'templateId' = $1
		  AND ts.enabled = true
		ORDER BY i.interface_name, tp.pipeline_name, ts.sequence
	`

	rows, err := s.db.Query(query, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get template usage: %w", err)
	}
	defer rows.Close()

	usage := []models.TemplateUsageInfo{}
	for rows.Next() {
		info := models.TemplateUsageInfo{}
		var mappingMode sql.NullString
		err := rows.Scan(
			&info.InterfaceName,
			&info.PipelineName,
			&info.StepName,
			&info.StepID,
			&mappingMode,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage info: %w", err)
		}
		if mappingMode.Valid {
			info.MappingMode = mappingMode.String
		}
		usage = append(usage, info)
	}

	return usage, nil
}

// LoadMappingRulesForStep loads the effective mapping rules for a step
// This resolves template references and applies mode-specific logic
func (s *ResponseMappingService) LoadMappingRulesForStep(stepConfig map[string]interface{}) ([]models.ResponseMappingRule, error) {
	// Extract responseMapping from step config
	responseMappingRaw, exists := stepConfig["responseMapping"]
	if !exists {
		return nil, nil // No response mapping configured
	}

	// Parse response mapping config
	responseMappingJSON, err := json.Marshal(responseMappingRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response mapping config: %w", err)
	}

	var mappingConfig models.ResponseMappingConfig
	if err := json.Unmarshal(responseMappingJSON, &mappingConfig); err != nil {
		return nil, fmt.Errorf("failed to parse response mapping config: %w", err)
	}

	// Handle based on mode
	switch mappingConfig.Mode {
	case models.MappingModeTemplate:
		// Load template and return its rules
		if mappingConfig.TemplateID == "" {
			return nil, fmt.Errorf("templateId required for mode 'template'")
		}
		template, err := s.GetTemplateByID(mappingConfig.TemplateID)
		if err != nil {
			return nil, err
		}
		return template.ParsedMappingRules()

	case models.MappingModeCustom:
		// Return custom extractors directly
		return mappingConfig.Extractors, nil

	case models.MappingModeExtend:
		// Load template rules + add custom extractors
		if mappingConfig.TemplateID == "" {
			return nil, fmt.Errorf("templateId required for mode 'extend'")
		}
		template, err := s.GetTemplateByID(mappingConfig.TemplateID)
		if err != nil {
			return nil, err
		}
		templateRules, err := template.ParsedMappingRules()
		if err != nil {
			return nil, err
		}
		// Append custom extractors
		return append(templateRules, mappingConfig.CustomExtractors...), nil

	case models.MappingModeOverride:
		// Load template rules and apply overrides
		if mappingConfig.TemplateID == "" {
			return nil, fmt.Errorf("templateId required for mode 'override'")
		}
		template, err := s.GetTemplateByID(mappingConfig.TemplateID)
		if err != nil {
			return nil, err
		}
		templateRules, err := template.ParsedMappingRules()
		if err != nil {
			return nil, err
		}
		// Apply overrides (replace specific rules by targetField)
		return s.applyOverrides(templateRules, mappingConfig.Overrides), nil

	default:
		return nil, fmt.Errorf("unknown mapping mode: %s", mappingConfig.Mode)
	}
}

// applyOverrides applies rule overrides to template rules
func (s *ResponseMappingService) applyOverrides(templateRules []models.ResponseMappingRule, overrides map[string]interface{}) []models.ResponseMappingRule {
	result := make([]models.ResponseMappingRule, len(templateRules))
	copy(result, templateRules)

	// For each override, find and replace the matching rule by targetField
	for targetField, overrideRaw := range overrides {
		overrideJSON, err := json.Marshal(overrideRaw)
		if err != nil {
			log.Printf("⚠️  Failed to marshal override for %s: %v", targetField, err)
			continue
		}

		var overrideRule models.ResponseMappingRule
		if err := json.Unmarshal(overrideJSON, &overrideRule); err != nil {
			log.Printf("⚠️  Failed to parse override for %s: %v", targetField, err)
			continue
		}

		// Find and replace the rule
		for i, rule := range result {
			if rule.TargetField == targetField {
				result[i] = overrideRule
				log.Printf("📝 Applied override for field: %s", targetField)
				break
			}
		}
	}

	return result
}
