// controllers/narrative_fields_controller.go
//
// REST endpoint backing the interface wizard's narrative field picker (Step 4
// — see public/js/step4-fhir-transform.js), which lets a user restrict which
// fields render in a resource type's auto-generated FHIR narrative
// (resource.text.div). See services/fhir_narrative.NarrativeFieldConfig for
// how the saved selection is applied at transform time.
//
// GET /api/fhir/narrative-fields/:resourceType
//     Returns the resource type's top-level fields from the real loaded FHIR
//     schema — spec-accurate and complete, not derived from one sample
//     message's happenstance content.
package controllers

import (
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"ezhealthkonnect/fhir"
	"ezhealthkonnect/fhir/r4"
	fhirnarrative "ezhealthkonnect/services/fhir_narrative"
)

// NarrativeFieldsController serves the available-field catalog for the
// narrative field picker. Stateless — the FHIR schema registry is a package-
// level global (ezhealthkonnect/fhir/r4), so no constructor dependencies needed.
type NarrativeFieldsController struct{}

func NewNarrativeFieldsController() *NarrativeFieldsController {
	return &NarrativeFieldsController{}
}

func (ctrl *NarrativeFieldsController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/narrative-fields/:resourceType", ctrl.List)
}

// NarrativeFieldDTO is one selectable field in the picker.
type NarrativeFieldDTO struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// List returns resourceType's top-level fields (name + human label) from the
// FHIR R4 base schema.
//
// GET /api/fhir/narrative-fields/AllergyIntolerance
func (ctrl *NarrativeFieldsController) List(c *gin.Context) {
	resourceType := c.Param("resourceType")
	if resourceType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "resourceType is required"})
		return
	}

	reg := r4.GetRegistry()
	if reg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "FHIR schema registry not initialised"})
		return
	}
	cp, ok := reg.Get("R4", resourceType, "base")
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "no schema found for resourceType " + resourceType})
		return
	}
	schema := fhir.BuildLegacySchemaShape(cp)

	prefix := resourceType + "."
	seen := make(map[string]bool)
	var fields []NarrativeFieldDTO
	for path, el := range schema.Elements {
		rest, ok := strings.CutPrefix(path, prefix)
		if !ok || rest == "" || strings.Contains(rest, ".") {
			continue // skip nested paths — top-level fields only
		}
		if fhirnarrative.IsSkippedField(rest) {
			continue // administrative field the renderer never shows anyway
		}
		if seen[rest] {
			continue
		}
		seen[rest] = true
		label := el.Name
		if label == "" {
			label = el.Description
		}
		if label == "" {
			label = rest
		}
		fields = append(fields, NarrativeFieldDTO{Name: rest, Label: label})
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"resourceType": resourceType,
		"fields":       fields,
	})
}
