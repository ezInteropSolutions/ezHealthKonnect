// controllers/cda_code_translations_controller.go
// Management API for cda_code_translations — the table that rewrites a source system's local codes into
// standard ones during CDA/FHIR conversion. A translation is either SHARED (applies to every interface) or
// belongs to ONE interface (and wins over a shared one for that interface).
//
//   GET    /api/cda/code-translations          list (scope=all|global|interface, interfaceId, search, limit, offset)
//   POST   /api/cda/code-translations          create one
//   POST   /api/cda/code-translations/import   create or update many (CSV import in the UI)
//   PUT    /api/cda/code-translations/:id      change one
//   DELETE /api/cda/code-translations/:id      delete one
//
// Who may call what is decided by the Node proxy (app.js: readers can GET, write roles can change). Every
// change is audited with the acting user.
package controllers

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"ezhealthkonnect/services/audit"
	cdaterminology "ezhealthkonnect/services/cda_terminology"

	"github.com/gin-gonic/gin"
)

// maxImportRows bounds one import request; larger files are split by the caller.
const maxImportRows = 5000

var translationIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// CDACodeTranslationsController serves the Code Translations page.
type CDACodeTranslationsController struct {
	repo        *cdaterminology.TranslationRepository
	auditLogger audit.AuditLogger
}

// NewCDACodeTranslationsController wires the repository and audit logger in.
func NewCDACodeTranslationsController(repo *cdaterminology.TranslationRepository, auditLogger audit.AuditLogger) *CDACodeTranslationsController {
	return &CDACodeTranslationsController{repo: repo, auditLogger: auditLogger}
}

// RegisterRoutes mounts the endpoints on the /api/cda group.
func (cc *CDACodeTranslationsController) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/code-translations")
	g.GET("", cc.List)
	g.POST("", cc.Create)
	g.POST("/import", cc.Import)
	g.PUT("/:id", cc.Update)
	g.DELETE("/:id", cc.Delete)
}

// translationBody is the editable part of a translation. InterfaceID empty/absent = shared.
type translationBody struct {
	InterfaceID   string `json:"interfaceId"`
	SourceSystem  string `json:"sourceSystem"`
	SourceCode    string `json:"sourceCode"`
	TargetSystem  string `json:"targetSystem"`
	TargetCode    string `json:"targetCode"`
	TargetDisplay string `json:"targetDisplay"`
}

// validate trims the fields and reports the first problem in words a user can act on.
func (b *translationBody) validate() string {
	b.InterfaceID = strings.TrimSpace(b.InterfaceID)
	b.SourceSystem = strings.TrimSpace(b.SourceSystem)
	b.SourceCode = strings.TrimSpace(b.SourceCode)
	b.TargetSystem = strings.TrimSpace(b.TargetSystem)
	b.TargetCode = strings.TrimSpace(b.TargetCode)
	b.TargetDisplay = strings.TrimSpace(b.TargetDisplay)
	switch {
	case b.SourceSystem == "":
		return "Source system is required"
	case b.SourceCode == "":
		return "Source code is required"
	case b.TargetSystem == "":
		return "Target system is required"
	case b.TargetCode == "":
		return "Target code is required"
	case len(b.SourceSystem) > 255 || len(b.TargetSystem) > 255:
		return "A system is limited to 255 characters"
	case len(b.SourceCode) > 100 || len(b.TargetCode) > 100:
		return "A code is limited to 100 characters"
	case len(b.TargetDisplay) > 500:
		return "Display text is limited to 500 characters"
	case b.InterfaceID != "" && !translationIDPattern.MatchString(b.InterfaceID):
		return "interfaceId is not valid"
	}
	return ""
}

func (b translationBody) toModel() cdaterminology.CodeTranslation {
	t := cdaterminology.CodeTranslation{
		SourceSystem: b.SourceSystem, SourceCode: b.SourceCode,
		TargetSystem: b.TargetSystem, TargetCode: b.TargetCode, TargetDisplay: b.TargetDisplay,
	}
	if b.InterfaceID != "" {
		id := b.InterfaceID
		t.InterfaceID = &id
	}
	return t
}

func (cc *CDACodeTranslationsController) audit(c *gin.Context, action, entityID string, meta map[string]interface{}) {
	if cc.auditLogger == nil {
		return
	}
	if err := cc.auditLogger.Log(c.Request.Context(), audit.AuditEvent{
		Action: action, UserID: c.GetHeader("X-User-ID"), EntityType: "cda_code_translation",
		EntityID: entityID, Metadata: meta, IPAddress: c.ClientIP(),
	}); err != nil {
		log.Printf("⚠️  Failed to write audit log for %s: %v", action, err)
	}
}

func translationError(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"success": false, "error": msg})
}

// List returns one page of translations.
func (cc *CDACodeTranslationsController) List(c *gin.Context) {
	scope := c.DefaultQuery("scope", cdaterminology.ScopeAll)
	if scope != cdaterminology.ScopeAll && scope != cdaterminology.ScopeGlobal && scope != cdaterminology.ScopeInterface {
		translationError(c, http.StatusBadRequest, "scope must be all, global or interface")
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	interfaceID := c.Query("interfaceId")
	if interfaceID != "" && !translationIDPattern.MatchString(interfaceID) {
		translationError(c, http.StatusBadRequest, "interfaceId is not valid")
		return
	}
	if scope == cdaterminology.ScopeInterface && interfaceID == "" {
		translationError(c, http.StatusBadRequest, "interfaceId is required when scope is interface")
		return
	}
	page, err := cc.repo.Query(c.Request.Context(), cdaterminology.TranslationFilter{
		Scope: scope, InterfaceID: interfaceID, Search: c.Query("search"), Limit: limit, Offset: offset,
	})
	if err != nil {
		log.Printf("⚠️  code translations list failed: %v", err)
		translationError(c, http.StatusInternalServerError, "failed to list code translations")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": page.Items, "total": page.Total})
}

// Create adds one translation.
func (cc *CDACodeTranslationsController) Create(c *gin.Context) {
	var body translationBody
	if err := c.ShouldBindJSON(&body); err != nil {
		translationError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := body.validate(); msg != "" {
		translationError(c, http.StatusBadRequest, msg)
		return
	}
	t, err := cc.repo.Create(c.Request.Context(), body.toModel())
	if errors.Is(err, cdaterminology.ErrDuplicateTranslation) {
		translationError(c, http.StatusConflict, "A translation for this source system and code already exists here. Edit it instead.")
		return
	}
	if errors.Is(err, cdaterminology.ErrUnknownInterface) {
		translationError(c, http.StatusBadRequest, "That interface does not exist")
		return
	}
	if err != nil {
		log.Printf("⚠️  code translation create failed: %v", err)
		translationError(c, http.StatusInternalServerError, "failed to create the translation")
		return
	}
	cc.audit(c, "CDA_CODE_TRANSLATION_CREATED", t.ID, map[string]interface{}{
		"interface_id": body.InterfaceID, "source_system": t.SourceSystem, "source_code": t.SourceCode,
		"target_system": t.TargetSystem, "target_code": t.TargetCode,
	})
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": t})
}

// Update changes one translation.
func (cc *CDACodeTranslationsController) Update(c *gin.Context) {
	id := c.Param("id")
	if !translationIDPattern.MatchString(id) {
		translationError(c, http.StatusNotFound, "translation not found")
		return
	}
	var body translationBody
	if err := c.ShouldBindJSON(&body); err != nil {
		translationError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := body.validate(); msg != "" {
		translationError(c, http.StatusBadRequest, msg)
		return
	}
	before, err := cc.repo.Get(c.Request.Context(), id)
	if errors.Is(err, cdaterminology.ErrTranslationNotFound) {
		translationError(c, http.StatusNotFound, "translation not found")
		return
	}
	if err != nil {
		translationError(c, http.StatusInternalServerError, "failed to read the translation")
		return
	}
	t, err := cc.repo.Update(c.Request.Context(), id, body.toModel())
	if errors.Is(err, cdaterminology.ErrDuplicateTranslation) {
		translationError(c, http.StatusConflict, "Another translation already uses this source system and code here.")
		return
	}
	if err != nil {
		log.Printf("⚠️  code translation update failed: %v", err)
		translationError(c, http.StatusInternalServerError, "failed to update the translation")
		return
	}
	cc.audit(c, "CDA_CODE_TRANSLATION_UPDATED", id, map[string]interface{}{
		"source_system": t.SourceSystem, "source_code": t.SourceCode,
		"old_target_code": before.TargetCode, "new_target_code": t.TargetCode,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// Delete removes one translation.
func (cc *CDACodeTranslationsController) Delete(c *gin.Context) {
	id := c.Param("id")
	if !translationIDPattern.MatchString(id) {
		translationError(c, http.StatusNotFound, "translation not found")
		return
	}
	before, err := cc.repo.Get(c.Request.Context(), id)
	if errors.Is(err, cdaterminology.ErrTranslationNotFound) {
		translationError(c, http.StatusNotFound, "translation not found")
		return
	}
	if err != nil {
		translationError(c, http.StatusInternalServerError, "failed to read the translation")
		return
	}
	if err := cc.repo.Delete(c.Request.Context(), id); err != nil {
		log.Printf("⚠️  code translation delete failed: %v", err)
		translationError(c, http.StatusInternalServerError, "failed to delete the translation")
		return
	}
	cc.audit(c, "CDA_CODE_TRANSLATION_DELETED", id, map[string]interface{}{
		"source_system": before.SourceSystem, "source_code": before.SourceCode, "target_code": before.TargetCode,
	})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type importRequest struct {
	Rows []translationBody `json:"rows"`
}

type importRowError struct {
	Row   int    `json:"row"` // 1-based position in the submitted list
	Error string `json:"error"`
}

// Import creates or updates many translations in one call. A bad row does not stop the others; the response
// lists each rejected row with the reason. Existing rows (same scope, source system, source code, target
// system) are updated in place.
func (cc *CDACodeTranslationsController) Import(c *gin.Context) {
	var req importRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		translationError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Rows) == 0 {
		translationError(c, http.StatusBadRequest, "no rows to import")
		return
	}
	if len(req.Rows) > maxImportRows {
		translationError(c, http.StatusBadRequest, "too many rows; import at most "+strconv.Itoa(maxImportRows)+" at a time")
		return
	}
	saved := 0
	rejected := []importRowError{}
	for i := range req.Rows {
		row := req.Rows[i]
		if msg := row.validate(); msg != "" {
			rejected = append(rejected, importRowError{Row: i + 1, Error: msg})
			continue
		}
		if err := cc.repo.Upsert(c.Request.Context(), row.toModel()); err != nil {
			log.Printf("⚠️  code translation import row %d failed: %v", i+1, err)
			rejected = append(rejected, importRowError{Row: i + 1, Error: "could not be saved"})
			continue
		}
		saved++
	}
	cc.audit(c, "CDA_CODE_TRANSLATIONS_IMPORTED", "", map[string]interface{}{
		"rows_submitted": len(req.Rows), "rows_saved": saved, "rows_rejected": len(rejected),
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "saved": saved, "rejected": rejected})
}
