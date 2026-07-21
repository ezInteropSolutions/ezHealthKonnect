// controllers/cda_csv_schema_controller.go
// GetCSVSections — GET /api/cda/csv/sections, for the cda.section_to_csv
// pipeline step's config UI. Separate file from cda_schema_controller.go
// (same CDASchemaController type — Go allows methods split across files in
// one package) because that file is about the CDA→FHIR declarative mapping
// wizard specifically; this endpoint is about a different step type
// (cda.section_to_csv) with a different, much simpler backing model (static
// Go structs, not the declarative MappingRule engine).

package controllers

import (
	"net/http"

	cdaTransform "ezhealthkonnect/services/executors/transform"

	"github.com/gin-gonic/gin"
)

// GetCSVSections returns every OOB cda.section_to_csv section with its
// resolved column list (name, CDA path, fallback paths, whether it exposes
// code metadata / collects multiple matches) — sourced live from
// cdaTransform.CSVSectionCatalog(), the exact same data buildCSVRows/Execute
// resolve rows and write CSV headers from, so this can never drift from what
// actually runs. Lets the step config UI show a user what's already being
// captured before they add a custom column or override one.
func (cc *CDASchemaController) GetCSVSections(c *gin.Context) {
	sections := cdaTransform.CSVSectionCatalog()
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"sections": sections,
		"count":    len(sections),
	})
}
