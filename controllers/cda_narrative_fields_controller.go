// controllers/cda_narrative_fields_controller.go
//
// Per-(interface, document_type) narrative field configuration for CDA→FHIR
// resources — the CDA-side mirror of optional_segments_controller.go's
// narrative_fields support, targeting interface_cda_mappings instead of
// interface_message_mappings (CDA already has its own config table, keyed by
// document_type instead of message_type — see database/migrations/V149).
//
// GET   /api/cda/narrative-fields?interfaceId=&docType=
//       Returns the currently-saved narrative_fields config for the interface
//       + document type (or null if unset — meaning "show everything").
//
// PATCH /api/cda/narrative-fields/:interfaceId/:docType
//       Body: {"AllergyIntolerance": ["code","criticality","reaction"], ...}
//       Merges into interface_cda_mappings.custom_mapping_config["narrative_fields"].
//       A resourceType mapped to an empty array clears any prior restriction
//       for that type (same semantics as the HL7-side endpoint).
package controllers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CDANarrativeFieldsController handles reading/writing narrative_fields for
// CDA→FHIR interface configurations.
type CDANarrativeFieldsController struct {
	db *sql.DB
}

func NewCDANarrativeFieldsController(db *sql.DB) *CDANarrativeFieldsController {
	return &CDANarrativeFieldsController{db: db}
}

func (ctrl *CDANarrativeFieldsController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/narrative-fields", ctrl.Get)
	rg.PATCH("/narrative-fields/:interfaceId/:docType", ctrl.Save)
}

// Get returns the currently-saved narrative_fields config.
//
// GET /api/cda/narrative-fields?interfaceId=<uuid>&docType=CCD
func (ctrl *CDANarrativeFieldsController) Get(c *gin.Context) {
	interfaceID := c.Query("interfaceId")
	docType := c.Query("docType")
	if interfaceID == "" || docType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interfaceId and docType are required"})
		return
	}

	narrativeFields := ctrl.loadNarrativeFieldsConfig(c.Request.Context(), interfaceID, docType)

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"interfaceId":     interfaceID,
		"docType":         docType,
		"narrativeFields": narrativeFields,
	})
}

// Save merges narrative_fields into interface_cda_mappings.custom_mapping_config.
//
// PATCH /api/cda/narrative-fields/:interfaceId/:docType
// Body: {"AllergyIntolerance": ["code","criticality"], "Condition": []}
func (ctrl *CDANarrativeFieldsController) Save(c *gin.Context) {
	interfaceID := c.Param("interfaceId")
	docType := c.Param("docType")
	if interfaceID == "" || docType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interfaceId and docType are required"})
		return
	}

	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid JSON body"})
		return
	}
	narrativeFields := toStringStringSliceMap(raw)

	if err := ctrl.mergeConfig(c.Request.Context(), interfaceID, docType, narrativeFields); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "narrative field config saved"})
}

func (ctrl *CDANarrativeFieldsController) loadNarrativeFieldsConfig(
	ctx context.Context, interfaceID, docType string,
) map[string][]string {
	query := `
		SELECT custom_mapping_config
		FROM interface_cda_mappings
		WHERE interface_id = $1 AND document_type = $2
		  AND custom_mapping_config IS NOT NULL
		LIMIT 1
	`
	var configBytes []byte
	if err := ctrl.db.QueryRowContext(ctx, query, interfaceID, docType).Scan(&configBytes); err != nil {
		return nil
	}
	var decoded struct {
		NarrativeFields map[string][]string `json:"narrative_fields"`
	}
	if err := json.Unmarshal(configBytes, &decoded); err != nil {
		return nil
	}
	return decoded.NarrativeFields
}

// mergeConfig reads the existing custom_mapping_config, applies the new
// narrative_fields values, and upserts the interface_cda_mappings row —
// mirrors optional_segments_controller.go's mergeConfig exactly, one key,
// different table.
func (ctrl *CDANarrativeFieldsController) mergeConfig(
	ctx context.Context,
	interfaceID, docType string,
	newNarrativeFields map[string][]string,
) error {
	query := `
		SELECT custom_mapping_config
		FROM interface_cda_mappings
		WHERE interface_id = $1 AND document_type = $2
		LIMIT 1
	`
	var existing []byte
	_ = ctrl.db.QueryRowContext(ctx, query, interfaceID, docType).Scan(&existing)

	cfg := map[string]interface{}{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &cfg)
	}

	current := map[string]interface{}{}
	if prev, ok := cfg["narrative_fields"].(map[string]interface{}); ok {
		current = prev
	}
	for k, v := range newNarrativeFields {
		if len(v) == 0 {
			delete(current, k)
			continue
		}
		fields := make([]interface{}, len(v))
		for i, f := range v {
			fields[i] = f
		}
		current[k] = fields
	}
	cfg["narrative_fields"] = current

	updated, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	// interface_cda_mappings requires uses_standard_template (NOT NULL) — an
	// insert-on-first-save must supply a default; true matches the table's own
	// column default and CdaToFhirStepBuilder's "auto" documentType convention
	// (OOB template unless/until the user's own delta overrides say otherwise).
	upsert := `
		INSERT INTO interface_cda_mappings (interface_id, document_type, custom_mapping_config, uses_standard_template, created_at, updated_at)
		VALUES ($1, $2, $3, true, NOW(), NOW())
		ON CONFLICT (interface_id, document_type)
		DO UPDATE SET
			custom_mapping_config = $3,
			updated_at = NOW()
	`
	_, err = ctrl.db.ExecContext(ctx, upsert, interfaceID, docType, updated)
	return err
}
