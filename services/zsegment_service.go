// services/zsegment_service.go
//
// ZSegmentService: CRUD for Z-segment mapping configuration and template management.
//
// Responsibilities:
//   - Manage zsegment_templates and zsegment_template_fields
//   - Manage mfn_interface_configs (MFI.1 overrides)
//   - Manage zsegment_configs and zsegment_field_mappings per interface
//   - Apply a template to an interface (copy fields, record source_template_id)
//   - Save an interface config back as a template
//   - Load MFNRuntimeConfig for assembly (called at transform time)
//   - Serve the transform catalog for the UI
package services

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"ezhealthkonnect/models"
)

// ZSegmentService provides all Z-segment mapping operations.
type ZSegmentService struct {
	db *sql.DB
}

// NewZSegmentService constructs a ZSegmentService.
func NewZSegmentService(db *sql.DB) *ZSegmentService {
	return &ZSegmentService{db: db}
}

// ─────────────────────────────────────────────────────────────────────────────
// Transform catalog
// ─────────────────────────────────────────────────────────────────────────────

// GetTransformCatalog returns all registered transforms ordered for UI display.
func (s *ZSegmentService) GetTransformCatalog() ([]models.ZSegmentTransform, error) {
	rows, err := s.db.Query(`
		SELECT id, code, label, description, input_types, sort_order
		FROM zsegment_transforms
		ORDER BY sort_order, code`)
	if err != nil {
		return nil, fmt.Errorf("zsegment_service.GetTransformCatalog: %w", err)
	}
	defer rows.Close()

	var out []models.ZSegmentTransform
	for rows.Next() {
		var t models.ZSegmentTransform
		if err := rows.Scan(&t.ID, &t.Code, &t.Label, &t.Description, pq.Array(&t.InputTypes), &t.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// Templates
// ─────────────────────────────────────────────────────────────────────────────

// ListTemplates returns all templates visible to the caller.
// segmentID is optional — pass "" to list all.
func (s *ZSegmentService) ListTemplates(segmentID string) ([]models.ZSegmentTemplate, error) {
	query := `
		SELECT id, template_name, segment_id, target_fhir_resource, resource_role,
		       description, tags, is_system, is_public,
		       created_by_user_id, usage_count, created_at, updated_at
		FROM zsegment_templates
		WHERE is_public = true`
	args := []interface{}{}
	if segmentID != "" {
		query += ` AND segment_id = $1`
		args = append(args, strings.ToUpper(segmentID))
	}
	query += ` ORDER BY is_system DESC, usage_count DESC, template_name`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("zsegment_service.ListTemplates: %w", err)
	}
	defer rows.Close()

	var templates []models.ZSegmentTemplate
	for rows.Next() {
		var t models.ZSegmentTemplate
		if err := rows.Scan(
			&t.ID, &t.TemplateName, &t.SegmentID, &t.TargetFHIRResource, &t.ResourceRole,
			&t.Description, pq.Array(&t.Tags), &t.IsSystem, &t.IsPublic,
			&t.CreatedByUserID, &t.UsageCount, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Eager-load fields for each template
	for i := range templates {
		fields, err := s.getTemplateFields(templates[i].ID)
		if err != nil {
			return nil, err
		}
		templates[i].Fields = fields
	}
	return templates, nil
}

// GetTemplate returns a single template with its fields.
func (s *ZSegmentService) GetTemplate(templateID string) (*models.ZSegmentTemplate, error) {
	var t models.ZSegmentTemplate
	err := s.db.QueryRow(`
		SELECT id, template_name, segment_id, target_fhir_resource, resource_role,
		       description, tags, is_system, is_public,
		       created_by_user_id, usage_count, created_at, updated_at
		FROM zsegment_templates WHERE id = $1`, templateID).Scan(
		&t.ID, &t.TemplateName, &t.SegmentID, &t.TargetFHIRResource, &t.ResourceRole,
		&t.Description, pq.Array(&t.Tags), &t.IsSystem, &t.IsPublic,
		&t.CreatedByUserID, &t.UsageCount, &t.CreatedAt, &t.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("zsegment_service.GetTemplate: %w", err)
	}
	fields, err := s.getTemplateFields(t.ID)
	if err != nil {
		return nil, err
	}
	t.Fields = fields
	return &t, nil
}

// CreateTemplate creates a new user-defined template with its fields.
func (s *ZSegmentService) CreateTemplate(req models.CreateZSegmentTemplateRequest, userID string) (*models.ZSegmentTemplate, error) {
	role := req.ResourceRole
	if role == "" {
		role = "primary"
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var id string
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}
	// created_by_user_id is a nullable UUID FK — pass NULL when userID is empty
	// to avoid FK violation when the caller is unauthenticated.
	var nullableUID interface{}
	if userID != "" {
		nullableUID = userID
	}
	err = tx.QueryRow(`
		INSERT INTO zsegment_templates
		    (template_name, segment_id, target_fhir_resource, resource_role,
		     description, tags, is_system, is_public, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, false, $7, $8)
		RETURNING id`,
		req.TemplateName, strings.ToUpper(req.SegmentID), req.TargetFHIRResource,
		role, req.Description, pq.Array(tags), req.IsPublic, nullableUID,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("zsegment_service.CreateTemplate: %w", err)
	}

	for i, f := range req.Fields {
		if err := insertTemplateField(tx, id, f, i); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetTemplate(id)
}

// DeleteTemplate deletes a user-created template.  System templates cannot be deleted.
func (s *ZSegmentService) DeleteTemplate(templateID string) error {
	res, err := s.db.Exec(
		`DELETE FROM zsegment_templates WHERE id = $1 AND is_system = false`, templateID)
	if err != nil {
		return fmt.Errorf("zsegment_service.DeleteTemplate: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("template not found or is a system template")
	}
	return nil
}

func (s *ZSegmentService) getTemplateFields(templateID string) ([]models.ZSegmentTemplateField, error) {
	rows, err := s.db.Query(`
		SELECT id, template_id, position,
		       COALESCE(field_label,''), COALESCE(hl7_data_type,'ST'),
		       COALESCE(fhir_path,''), COALESCE(transform,'identity'),
		       is_required, COALESCE(default_value,''), sort_order
		FROM zsegment_template_fields
		WHERE template_id = $1
		ORDER BY sort_order, position`, templateID)
	if err != nil {
		return nil, fmt.Errorf("zsegment_service.getTemplateFields(%s): %w", templateID, err)
	}
	defer rows.Close()

	var fields []models.ZSegmentTemplateField
	for rows.Next() {
		var f models.ZSegmentTemplateField
		if err := rows.Scan(&f.ID, &f.TemplateID, &f.Position, &f.FieldLabel,
			&f.HL7DataType, &f.FHIRPath, &f.Transform,
			&f.IsRequired, &f.DefaultValue, &f.SortOrder); err != nil {
			return nil, err
		}
		fields = append(fields, f)
	}
	return fields, rows.Err()
}

func insertTemplateField(tx *sql.Tx, templateID string, f models.ZSegmentTemplateField, order int) error {
	dt := f.HL7DataType
	if dt == "" {
		dt = "ST"
	}
	tr := f.Transform
	if tr == "" {
		tr = "identity"
	}
	sortOrd := f.SortOrder
	if sortOrd == 0 {
		sortOrd = (order + 1) * 10
	}
	_, err := tx.Exec(`
		INSERT INTO zsegment_template_fields
		    (template_id, position, field_label, hl7_data_type,
		     fhir_path, transform, is_required, default_value, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (template_id, position) DO UPDATE SET
		    field_label   = EXCLUDED.field_label,
		    hl7_data_type = EXCLUDED.hl7_data_type,
		    fhir_path     = EXCLUDED.fhir_path,
		    transform     = EXCLUDED.transform,
		    is_required   = EXCLUDED.is_required,
		    default_value = EXCLUDED.default_value,
		    sort_order    = EXCLUDED.sort_order`,
		templateID, f.Position, f.FieldLabel, dt,
		f.FHIRPath, tr, f.IsRequired, f.DefaultValue, sortOrd,
	)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Tier 2 — MFI.1 overrides
// ─────────────────────────────────────────────────────────────────────────────

// GetMFNConfig returns all MFI.1 overrides for an interface.
func (s *ZSegmentService) GetMFNConfig(interfaceID string) ([]models.MFNInterfaceConfig, error) {
	rows, err := s.db.Query(`
		SELECT id, interface_id, mfi_identifier, COALESCE(fhir_resource,''),
		       COALESCE(description,''), is_active, created_at, updated_at
		FROM mfn_interface_configs
		WHERE interface_id = $1
		ORDER BY mfi_identifier`, interfaceID)
	if err != nil {
		return nil, fmt.Errorf("zsegment_service.GetMFNConfig: %w", err)
	}
	defer rows.Close()

	var out []models.MFNInterfaceConfig
	for rows.Next() {
		var c models.MFNInterfaceConfig
		if err := rows.Scan(&c.ID, &c.InterfaceID, &c.MFIIdentifier, &c.FHIRResource,
			&c.Description, &c.IsActive, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpsertMFNConfig creates or updates a single MFI.1 override.
func (s *ZSegmentService) UpsertMFNConfig(interfaceID string, c models.MFNInterfaceConfig) (*models.MFNInterfaceConfig, error) {
	mfi := strings.ToUpper(strings.TrimSpace(c.MFIIdentifier))
	active := true
	if c.IsActive == false {
		active = false
	}

	var id string
	err := s.db.QueryRow(`
		INSERT INTO mfn_interface_configs
		    (interface_id, mfi_identifier, fhir_resource, description, is_active)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (interface_id, mfi_identifier) DO UPDATE SET
		    fhir_resource = EXCLUDED.fhir_resource,
		    description   = EXCLUDED.description,
		    is_active      = EXCLUDED.is_active,
		    updated_at     = NOW()
		RETURNING id`,
		interfaceID, mfi, c.FHIRResource, c.Description, active,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("zsegment_service.UpsertMFNConfig: %w", err)
	}

	var out models.MFNInterfaceConfig
	err = s.db.QueryRow(`
		SELECT id, interface_id, mfi_identifier, COALESCE(fhir_resource,''),
		       COALESCE(description,''), is_active, created_at, updated_at
		FROM mfn_interface_configs WHERE id = $1`, id).Scan(
		&out.ID, &out.InterfaceID, &out.MFIIdentifier, &out.FHIRResource,
		&out.Description, &out.IsActive, &out.CreatedAt, &out.UpdatedAt,
	)
	return &out, err
}

// DeleteMFNConfig removes a single MFI.1 override by ID.
func (s *ZSegmentService) DeleteMFNConfig(interfaceID, configID string) error {
	_, err := s.db.Exec(
		`DELETE FROM mfn_interface_configs WHERE id = $1 AND interface_id = $2`,
		configID, interfaceID)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Tier 3 — Z-segment configs per interface
// ─────────────────────────────────────────────────────────────────────────────

// ListZSegmentConfigs returns all Z-segment configs for an interface, with fields.
func (s *ZSegmentService) ListZSegmentConfigs(interfaceID string) ([]models.ZSegmentConfig, error) {
	rows, err := s.db.Query(`
		SELECT id, interface_id, segment_id,
		       COALESCE(target_fhir_resource,''), COALESCE(resource_role,'primary'),
		       COALESCE(description,''), occurrence_index, source_template_id,
		       is_active, sort_order, created_at, updated_at
		FROM zsegment_configs
		WHERE interface_id = $1
		ORDER BY sort_order, segment_id`, interfaceID)
	if err != nil {
		return nil, fmt.Errorf("zsegment_service.ListZSegmentConfigs: %w", err)
	}
	defer rows.Close()

	var configs []models.ZSegmentConfig
	for rows.Next() {
		var c models.ZSegmentConfig
		if err := rows.Scan(
			&c.ID, &c.InterfaceID, &c.SegmentID, &c.TargetFHIRResource, &c.ResourceRole,
			&c.Description, &c.OccurrenceIndex, &c.SourceTemplateID,
			&c.IsActive, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Eager-load field mappings
	for i := range configs {
		fields, err := s.getFieldMappings(configs[i].ID)
		if err != nil {
			return nil, err
		}
		configs[i].FieldMappings = fields
	}
	return configs, nil
}

// GetZSegmentConfig returns a single config with its field mappings.
func (s *ZSegmentService) GetZSegmentConfig(interfaceID, configID string) (*models.ZSegmentConfig, error) {
	var c models.ZSegmentConfig
	err := s.db.QueryRow(`
		SELECT id, interface_id, segment_id,
		       COALESCE(target_fhir_resource,''), COALESCE(resource_role,'primary'),
		       COALESCE(description,''), occurrence_index, source_template_id,
		       is_active, sort_order, created_at, updated_at
		FROM zsegment_configs WHERE id = $1 AND interface_id = $2`,
		configID, interfaceID).Scan(
		&c.ID, &c.InterfaceID, &c.SegmentID, &c.TargetFHIRResource, &c.ResourceRole,
		&c.Description, &c.OccurrenceIndex, &c.SourceTemplateID,
		&c.IsActive, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("zsegment_service.GetZSegmentConfig: %w", err)
	}
	fields, err := s.getFieldMappings(c.ID)
	if err != nil {
		return nil, err
	}
	c.FieldMappings = fields
	return &c, nil
}

// CreateZSegmentConfig creates a new Z-segment config with optional inline fields.
func (s *ZSegmentService) CreateZSegmentConfig(interfaceID string, req models.CreateZSegmentConfigRequest) (*models.ZSegmentConfig, error) {
	role := req.ResourceRole
	if role == "" {
		role = "primary"
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var id string
	err = tx.QueryRow(`
		INSERT INTO zsegment_configs
		    (interface_id, segment_id, target_fhir_resource, resource_role,
		     description, occurrence_index, source_template_id, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id`,
		interfaceID, strings.ToUpper(req.SegmentID), req.TargetFHIRResource, role,
		req.Description, req.OccurrenceIndex, req.SourceTemplateID, req.SortOrder,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("zsegment_service.CreateZSegmentConfig: %w", err)
	}

	for i, f := range req.FieldMappings {
		if err := insertFieldMapping(tx, id, f, i); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetZSegmentConfig(interfaceID, id)
}

// UpdateZSegmentConfig updates metadata fields on an existing config.
func (s *ZSegmentService) UpdateZSegmentConfig(interfaceID, configID string, req models.UpdateZSegmentConfigRequest) (*models.ZSegmentConfig, error) {
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	_, err := s.db.Exec(`
		UPDATE zsegment_configs SET
		    target_fhir_resource = COALESCE(NULLIF($1,''), target_fhir_resource),
		    resource_role        = COALESCE(NULLIF($2,''), resource_role),
		    description          = $3,
		    occurrence_index     = $4,
		    is_active            = $5,
		    sort_order           = $6,
		    updated_at           = NOW()
		WHERE id = $7 AND interface_id = $8`,
		req.TargetFHIRResource, req.ResourceRole, req.Description,
		req.OccurrenceIndex, isActive, req.SortOrder,
		configID, interfaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("zsegment_service.UpdateZSegmentConfig: %w", err)
	}
	return s.GetZSegmentConfig(interfaceID, configID)
}

// DeleteZSegmentConfig removes a Z-segment config and all its field mappings (cascade).
func (s *ZSegmentService) DeleteZSegmentConfig(interfaceID, configID string) error {
	_, err := s.db.Exec(
		`DELETE FROM zsegment_configs WHERE id = $1 AND interface_id = $2`,
		configID, interfaceID)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Field mappings
// ─────────────────────────────────────────────────────────────────────────────

// UpsertFieldMapping creates or updates a single field mapping on a Z-segment config.
func (s *ZSegmentService) UpsertFieldMapping(interfaceID, configID string, req models.UpsertFieldMappingRequest) (*models.ZSegmentFieldMapping, error) {
	// Verify config belongs to interface
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM zsegment_configs WHERE id = $1 AND interface_id = $2)`,
		configID, interfaceID).Scan(&exists)
	if !exists {
		return nil, fmt.Errorf("zsegment config %s not found for interface %s", configID, interfaceID)
	}

	dt := req.HL7DataType
	if dt == "" {
		dt = "ST"
	}
	tr := req.Transform
	if tr == "" {
		tr = "identity"
	}

	var id string
	err := s.db.QueryRow(`
		INSERT INTO zsegment_field_mappings
		    (zsegment_config_id, position, field_label, hl7_data_type,
		     fhir_path, transform, is_required, default_value, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (zsegment_config_id, position) DO UPDATE SET
		    field_label   = EXCLUDED.field_label,
		    hl7_data_type = EXCLUDED.hl7_data_type,
		    fhir_path     = EXCLUDED.fhir_path,
		    transform     = EXCLUDED.transform,
		    is_required   = EXCLUDED.is_required,
		    default_value = EXCLUDED.default_value,
		    sort_order    = EXCLUDED.sort_order
		RETURNING id`,
		configID, req.Position, req.FieldLabel, dt,
		req.FHIRPath, tr, req.IsRequired, req.DefaultValue, req.SortOrder,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("zsegment_service.UpsertFieldMapping: %w", err)
	}

	var f models.ZSegmentFieldMapping
	err = s.db.QueryRow(`
		SELECT id, zsegment_config_id, position,
		       COALESCE(field_label,''), COALESCE(hl7_data_type,'ST'),
		       COALESCE(fhir_path,''), COALESCE(transform,'identity'),
		       is_required, COALESCE(default_value,''), sort_order
		FROM zsegment_field_mappings WHERE id = $1`, id).Scan(
		&f.ID, &f.ZSegmentConfigID, &f.Position, &f.FieldLabel, &f.HL7DataType,
		&f.FHIRPath, &f.Transform, &f.IsRequired, &f.DefaultValue, &f.SortOrder,
	)
	return &f, err
}

// DeleteFieldMapping removes a single field mapping by ID.
func (s *ZSegmentService) DeleteFieldMapping(interfaceID, configID, fieldID string) error {
	_, err := s.db.Exec(`
		DELETE FROM zsegment_field_mappings
		WHERE id = $1
		  AND zsegment_config_id = $2
		  AND EXISTS (
		      SELECT 1 FROM zsegment_configs
		      WHERE id = $2 AND interface_id = $3
		  )`, fieldID, configID, interfaceID)
	return err
}

func (s *ZSegmentService) getFieldMappings(configID string) ([]models.ZSegmentFieldMapping, error) {
	rows, err := s.db.Query(`
		SELECT id, zsegment_config_id, position,
		       COALESCE(field_label,''), COALESCE(hl7_data_type,'ST'),
		       COALESCE(fhir_path,''), COALESCE(transform,'identity'),
		       is_required, COALESCE(default_value,''), sort_order
		FROM zsegment_field_mappings
		WHERE zsegment_config_id = $1
		ORDER BY sort_order, position`, configID)
	if err != nil {
		return nil, fmt.Errorf("zsegment_service.getFieldMappings(%s): %w", configID, err)
	}
	defer rows.Close()

	var fields []models.ZSegmentFieldMapping
	for rows.Next() {
		var f models.ZSegmentFieldMapping
		if err := rows.Scan(&f.ID, &f.ZSegmentConfigID, &f.Position, &f.FieldLabel,
			&f.HL7DataType, &f.FHIRPath, &f.Transform,
			&f.IsRequired, &f.DefaultValue, &f.SortOrder); err != nil {
			return nil, err
		}
		fields = append(fields, f)
	}
	return fields, rows.Err()
}

func insertFieldMapping(tx *sql.Tx, configID string, f models.ZSegmentFieldMapping, order int) error {
	dt := f.HL7DataType
	if dt == "" {
		dt = "ST"
	}
	tr := f.Transform
	if tr == "" {
		tr = "identity"
	}
	sortOrd := f.SortOrder
	if sortOrd == 0 {
		sortOrd = (order + 1) * 10
	}
	_, err := tx.Exec(`
		INSERT INTO zsegment_field_mappings
		    (zsegment_config_id, position, field_label, hl7_data_type,
		     fhir_path, transform, is_required, default_value, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (zsegment_config_id, position) DO UPDATE SET
		    field_label   = EXCLUDED.field_label,
		    hl7_data_type = EXCLUDED.hl7_data_type,
		    fhir_path     = EXCLUDED.fhir_path,
		    transform     = EXCLUDED.transform,
		    is_required   = EXCLUDED.is_required,
		    default_value = EXCLUDED.default_value,
		    sort_order    = EXCLUDED.sort_order`,
		configID, f.Position, f.FieldLabel, dt,
		f.FHIRPath, tr, f.IsRequired, f.DefaultValue, sortOrd,
	)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Template apply / save-as-template
// ─────────────────────────────────────────────────────────────────────────────

// ApplyTemplate copies a template's fields into a new zsegment_config for an interface.
// Fields are fully copied — the interface config is self-contained and survives
// future template edits.
func (s *ZSegmentService) ApplyTemplate(interfaceID string, req models.ApplyTemplateRequest) (*models.ZSegmentConfig, error) {
	tmpl, err := s.GetTemplate(req.TemplateID)
	if err != nil {
		return nil, err
	}
	if tmpl == nil {
		return nil, fmt.Errorf("template %s not found", req.TemplateID)
	}

	// Build field mappings from template fields
	fieldMappings := make([]models.ZSegmentFieldMapping, len(tmpl.Fields))
	for i, tf := range tmpl.Fields {
		fieldMappings[i] = models.ZSegmentFieldMapping{
			Position:     tf.Position,
			FieldLabel:   tf.FieldLabel,
			HL7DataType:  tf.HL7DataType,
			FHIRPath:     tf.FHIRPath,
			Transform:    tf.Transform,
			IsRequired:   tf.IsRequired,
			DefaultValue: tf.DefaultValue,
			SortOrder:    tf.SortOrder,
		}
	}

	createReq := models.CreateZSegmentConfigRequest{
		SegmentID:          tmpl.SegmentID,
		TargetFHIRResource: tmpl.TargetFHIRResource,
		ResourceRole:       tmpl.ResourceRole,
		Description:        tmpl.Description,
		OccurrenceIndex:    req.OccurrenceIndex,
		SortOrder:          req.SortOrder,
		SourceTemplateID:   &req.TemplateID,
		FieldMappings:      fieldMappings,
	}

	cfg, err := s.CreateZSegmentConfig(interfaceID, createReq)
	if err != nil {
		return nil, err
	}

	// Increment template usage count
	s.db.Exec(`UPDATE zsegment_templates SET usage_count = usage_count + 1 WHERE id = $1`, req.TemplateID)

	return cfg, nil
}

// SaveAsTemplate creates a new template from an existing interface Z-segment config.
func (s *ZSegmentService) SaveAsTemplate(interfaceID, configID string, req models.SaveAsTemplateRequest, userID string) (*models.ZSegmentTemplate, error) {
	cfg, err := s.GetZSegmentConfig(interfaceID, configID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("zsegment config %s not found", configID)
	}

	// Convert field mappings to template fields
	templateFields := make([]models.ZSegmentTemplateField, len(cfg.FieldMappings))
	for i, fm := range cfg.FieldMappings {
		templateFields[i] = models.ZSegmentTemplateField{
			Position:     fm.Position,
			FieldLabel:   fm.FieldLabel,
			HL7DataType:  fm.HL7DataType,
			FHIRPath:     fm.FHIRPath,
			Transform:    fm.Transform,
			IsRequired:   fm.IsRequired,
			DefaultValue: fm.DefaultValue,
			SortOrder:    fm.SortOrder,
		}
	}

	createReq := models.CreateZSegmentTemplateRequest{
		TemplateName:       req.TemplateName,
		SegmentID:          cfg.SegmentID,
		TargetFHIRResource: cfg.TargetFHIRResource,
		ResourceRole:       cfg.ResourceRole,
		Description:        req.Description,
		Tags:               req.Tags,
		IsPublic:           req.IsPublic,
		Fields:             templateFields,
	}

	return s.CreateTemplate(createReq, userID)
}

// ─────────────────────────────────────────────────────────────────────────────
// Runtime config loader (called by assembly at transform time)
// ─────────────────────────────────────────────────────────────────────────────

// GetRuntimeConfig loads the MFNRuntimeConfig for an interface.
// Returns nil (not an error) when no config exists — assembly falls back to
// hardcoded behaviour and logs a warning.
func (s *ZSegmentService) GetRuntimeConfig(interfaceID string) (*models.MFNRuntimeConfig, error) {
	mfiRows, err := s.GetMFNConfig(interfaceID)
	if err != nil {
		return nil, err
	}

	zsegs, err := s.ListZSegmentConfigs(interfaceID)
	if err != nil {
		return nil, err
	}

	if len(mfiRows) == 0 && len(zsegs) == 0 {
		return nil, nil // no config — caller uses fallback
	}

	cfg := &models.MFNRuntimeConfig{
		MFIOverrides: make(map[string]string, len(mfiRows)),
	}
	for _, m := range mfiRows {
		if m.IsActive {
			cfg.MFIOverrides[strings.ToUpper(m.MFIIdentifier)] = m.FHIRResource
		}
	}

	// Only include active Z-segment configs
	for _, z := range zsegs {
		if z.IsActive {
			cfg.ZSegments = append(cfg.ZSegments, z)
		}
	}

	return cfg, nil
}

// ZSegmentParsedField is one field position extracted from a raw Z-segment line.
type ZSegmentParsedField struct {
	Position string `json:"position"` // e.g. "ZEM.2"
	RawValue string `json:"rawValue"` // raw HL7 field value as it appears in the message
}

// DetectZSegmentsInMessage parses a raw HL7 message string and returns the unique
// Z-segment IDs found (e.g. ["ZEM", "ZPI"]).  Used by the wizard detect endpoint.
func DetectZSegmentsInMessage(rawMessage string) []string {
	seen := map[string]bool{}
	var found []string

	normalized := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(rawMessage)
	for _, line := range strings.Split(normalized, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 3 {
			continue
		}
		segID := line[:3]
		if strings.HasPrefix(segID, "Z") && len(segID) == 3 {
			upper := strings.ToUpper(segID)
			if !seen[upper] {
				seen[upper] = true
				found = append(found, upper)
			}
		}
	}
	return found
}

// ParseZSegmentFields extracts the raw field values from every occurrence of
// segmentID in rawMessage.  Returns one []ZSegmentParsedField per occurrence
// (most messages have a single occurrence of each Z-segment).
//
// Fields are indexed from position 1 (segID is position 0).  Only non-empty
// fields are included.  The field separator is read from MSH.1 (defaults to "|").
func ParseZSegmentFields(rawMessage, segmentID string) [][]ZSegmentParsedField {
	// Read field separator from MSH.1 (position 1 in the MSH line, before the
	// encoding characters).  Default "|" covers messages that arrive without MSH.
	fieldSep := "|"
	normalized := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(rawMessage)
	for _, line := range strings.Split(normalized, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "MSH") && len(line) > 3 {
			fieldSep = string(line[3])
			break
		}
	}

	upper := strings.ToUpper(strings.TrimSpace(segmentID))
	var occurrences [][]ZSegmentParsedField

	for _, line := range strings.Split(normalized, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 3 || strings.ToUpper(line[:3]) != upper {
			continue
		}
		parts := strings.Split(line, fieldSep)
		var fields []ZSegmentParsedField
		// parts[0] = segment ID; fields start at parts[1] → position 1
		for i := 1; i < len(parts); i++ {
			val := strings.TrimSpace(parts[i])
			if val != "" {
				fields = append(fields, ZSegmentParsedField{
					Position: fmt.Sprintf("%s.%d", upper, i),
					RawValue: val,
				})
			}
		}
		occurrences = append(occurrences, fields)
	}
	return occurrences
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// unused import guard
var _ = time.Now
