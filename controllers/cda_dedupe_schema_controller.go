// controllers/cda_dedupe_schema_controller.go
// GetDedupeSections — GET /api/cda/dedupe/sections, for the cda.dedupe
// pipeline step's config UI. Separate file from cda_schema_controller.go
// (same CDASchemaController type — Go allows methods split across files in
// one package), same convention already established by
// cda_csv_schema_controller.go for the same reason: a different step type
// with a different, much simpler backing model.

package controllers

import (
	"net/http"

	cdaBuilder "ezhealthkonnect/cda/builder"
	cdaTransform "ezhealthkonnect/services/executors/transform"

	"github.com/gin-gonic/gin"
)

// dedupeSectionResponse is the wire shape of one section's field picker
// options, layering a display name + LOINC code (from cdaBuilder.SectionCatalog,
// the same schema-driven catalog cda.build/cda.map_to_canonical use) onto
// cdaTransform.DedupeSectionCatalog()'s field list — so a user configuring
// cda.dedupe only ever sees real, spec-backed CDA sections, never an invented
// or hand-typed one.
type dedupeSectionResponse struct {
	SectionKey  string                          `json:"sectionKey"`
	DisplayName string                          `json:"displayName"`
	LOINCCode   string                          `json:"loincCode,omitempty"`
	Fields      []cdaTransform.DedupeFieldOption `json:"fields"`
}

// GetDedupeSections returns every section cda.dedupe can operate on, with its
// available identity-matching fields and which are pre-selected by the OOB
// rule — sourced live from cdaTransform.DedupeSectionCatalog(), so this can
// never drift from what cda.dedupe's Execute actually runs.
//
// Display name/LOINC code enrichment degrades gracefully rather than failing
// the whole endpoint: cdaTransform.DedupeSectionCatalog() works independently
// of the schema loader (CSVSectionCatalog is a static Go registry), so a
// missing/unavailable schema loader just means sections fall back to their
// raw key as the display name — never a 503, since the field picker itself
// still works fine.
func (cc *CDASchemaController) GetDedupeSections(c *gin.Context) {
	fieldSections := cdaTransform.DedupeSectionCatalog()

	displayInfo := map[string]cdaBuilder.CanonicalSectionInfo{}
	if cc.schemaLoader != nil {
		for _, s := range cdaBuilder.SectionCatalog(cc.schemaLoader) {
			displayInfo[s.Key] = s
		}
	}

	out := make([]dedupeSectionResponse, 0, len(fieldSections))
	for _, sec := range fieldSections {
		info, hasInfo := displayInfo[sec.SectionKey]
		displayName := sec.SectionKey
		loincCode := ""
		if hasInfo && info.DisplayName != "" {
			displayName = info.DisplayName
			loincCode = info.LOINCCode
		}
		out = append(out, dedupeSectionResponse{
			SectionKey:  sec.SectionKey,
			DisplayName: displayName,
			LOINCCode:   loincCode,
			Fields:      sec.Fields,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"sections": out,
		"count":    len(out),
	})
}
