package controllers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services"

	"github.com/gin-gonic/gin"
)

// MirthMigrationController handles Mirth Connect channel migration endpoints.
// Authentication is enforced at the Node.js proxy layer (X-User-ID injected after
// JWT verification). The Go handler only processes requests that reach it.
type MirthMigrationController struct {
	svc *services.MirthMigrationService
}

func NewMirthMigrationController(db *sql.DB) *MirthMigrationController {
	return &MirthMigrationController{
		svc: services.NewMirthMigrationService(db),
	}
}

// RegisterRoutes wires all migration endpoints under the given router group.
// Expected group prefix: /api/migration
func (ctrl *MirthMigrationController) RegisterRoutes(rg *gin.RouterGroup) {
	mirth := rg.Group("/mirth")
	{
		mirth.POST("/preview", ctrl.Preview)
		mirth.POST("/import", ctrl.Import)
		mirth.GET("/history", ctrl.History)
		mirth.POST("/rewrite-step", ctrl.RewriteStep)
	}
}

// Preview
// POST /api/migration/mirth/preview
//
// Accepts raw Mirth channel XML as body (Content-Type: application/xml)
// Returns a MigrationPreview without touching the database.
func (ctrl *MirthMigrationController) Preview(c *gin.Context) {
	xmlData, err := readXMLBody(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if len(xmlData) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "request body is empty"})
		return
	}

	preview, err := ctrl.svc.Preview(xmlData)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"preview": preview,
	})
}

// Import
// POST /api/migration/mirth/import
//
//	{
//	  "channelXml":      "<raw XML string — NOT base64>",
//	  "interfaceName":   "My ADT Interface",
//	  "description":     "Migrated from Mirth",
//	  "skipStepIndices": [2, 4]
//	}
//
// userId is extracted from the X-User-ID header injected by the Node.js proxy.
func (ctrl *MirthMigrationController) Import(c *gin.Context) {
	var req models.MigrationImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if req.ChannelXML == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "channelXml is required"})
		return
	}
	if len(req.InterfaceName) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interfaceName must not exceed 255 characters"})
		return
	}
	if len(req.Description) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "description must not exceed 1000 characters"})
		return
	}

	// Extract user from the header injected by Node.js proxy (not from request body).
	req.UserID = ctrl.userIDFromContext(c)

	result, err := ctrl.svc.Import(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"result":  result,
	})
}

// History
// GET /api/migration/mirth/history?limit=25
//
// Returns recent Mirth migration history entries.
func (ctrl *MirthMigrationController) History(c *gin.Context) {
	limit := 25
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}

	entries, err := ctrl.svc.History(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if entries == nil {
		entries = []models.MirthMigrationHistoryEntry{}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    entries,
		"count":   len(entries),
	})
}

// RewriteStep
// POST /api/migration/mirth/rewrite-step
//
//	{
//	  "script":   "<original Mirth JavaScript>",
//	  "stepName": "Set Patient ID",
//	  "stepType": "JavaScript"
//	}
//
// Calls the internal AI service to rewrite the Mirth script using ezHK APIs.
// Returns { success, rewrittenScript, note }.
func (ctrl *MirthMigrationController) RewriteStep(c *gin.Context) {
	var req struct {
		Script   string `json:"script"`
		StepName string `json:"stepName"`
		StepType string `json:"stepType"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Script) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "script is required"})
		return
	}

	prompt := buildRewritePrompt(req.Script, req.StepName, req.StepType)

	aiBackend := os.Getenv("AI_BACKEND_URL")
	if aiBackend == "" {
		aiBackend = "http://localhost:" + func() string {
			if p := os.Getenv("PORT"); p != "" {
				return p
			}
			return "3000"
		}()
	}

	aiURL := aiBackend + "/api/ai/ask"

	payload, _ := json.Marshal(map[string]interface{}{
		"question":   prompt,
		"session_id": "mirth-rewrite-" + ctrl.userIDFromContext(c),
		"context":    "Mirth Connect to ezHealthKonnect script migration",
	})

	httpReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, aiURL, bytes.NewReader(payload))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to build AI request: " + err.Error()})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Forward auth cookies so the AI endpoint recognises the session
	for _, cookie := range c.Request.Cookies() {
		httpReq.AddCookie(cookie)
	}
	if xuid := c.GetHeader("X-User-ID"); xuid != "" {
		httpReq.Header.Set("X-User-ID", xuid)
	}

	httpClient := &http.Client{}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": "AI service unavailable: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	var aiResp struct {
		Answer string `json:"answer"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&aiResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to parse AI response"})
		return
	}
	if aiResp.Error != "" {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": aiResp.Error})
		return
	}

	// Extract code block from markdown response if present
	rewritten := extractCodeBlock(aiResp.Answer)

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"rewrittenScript": rewritten,
		"note":            "Rewritten by AI — review before importing",
	})
}

// buildRewritePrompt constructs the AI prompt for Mirth → ezHK script conversion.
func buildRewritePrompt(script, stepName, stepType string) string {
	return fmt.Sprintf(`You are a healthcare integration engineer migrating a Mirth Connect channel to ezHealthKonnect.

Convert the following Mirth Connect %s step "%s" into an ezHealthKonnect-compatible JavaScript enrichment script.

MIRTH CONNECT API → ezHealthKonnect API mapping rules:
- msg['SEG']['SEG.N']['SEG.N.M']  →  GetFieldValue(data, 'SEG.N.M')
- msg['SEG']['SEG.N']              →  GetFieldValue(data, 'SEG.N')
- tmp['varName']                   →  data['varName']
- channelMap.put('key', value)     →  data['key'] = value
- globalMap.put(...)               →  /* globalMap not available — store in data instead */
- $re('patientId')                 →  data['patientId']
- logger.info(msg)                 →  console.log(msg)
- connectorMessage.getRawData()    →  data._raw

Return ONLY a JavaScript function body (no markdown, no explanation). The function receives a single argument named "data" which is the parsed message object. Use GetFieldValue(data, 'SEG.N.M') to read HL7 fields.

Original Mirth script:
%s`, stepType, stepName, script)
}

// extractCodeBlock strips markdown code fences if the AI wrapped the output.
func extractCodeBlock(s string) string {
	s = strings.TrimSpace(s)
	// Strip ```javascript ... ``` or ``` ... ```
	for _, fence := range []string{"```javascript", "```js", "```"} {
		if strings.HasPrefix(s, fence) {
			s = strings.TrimPrefix(s, fence)
			if idx := strings.LastIndex(s, "```"); idx >= 0 {
				s = s[:idx]
			}
			s = strings.TrimSpace(s)
			break
		}
	}
	return s
}

// ============================================================================
// Helpers
// ============================================================================

// readXMLBody reads up to MaxMirthXMLBytes from the request body.
// Supports Content-Type: application/xml, text/xml, or application/json
// with { "channelXml": "<raw xml string>" }.
func readXMLBody(c *gin.Context) ([]byte, error) {
	ct := c.GetHeader("Content-Type")
	if ct == "application/xml" || ct == "text/xml" ||
		ct == "application/xml; charset=utf-8" || ct == "text/xml; charset=utf-8" {
		lr := io.LimitReader(c.Request.Body, int64(services.MaxMirthXMLBytes)+1)
		data, err := io.ReadAll(lr)
		if err != nil {
			return nil, err
		}
		if len(data) > services.MaxMirthXMLBytes {
			return nil, fmt.Errorf("XML exceeds maximum allowed size of %d MB", services.MaxMirthXMLBytes/1024/1024)
		}
		return data, nil
	}

	// JSON body: { "channelXml": "<raw xml>" }
	var body struct {
		ChannelXML string `json:"channelXml"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		return nil, err
	}
	if len(body.ChannelXML) > services.MaxMirthXMLBytes {
		return nil, fmt.Errorf("channelXml exceeds maximum allowed size of %d MB", services.MaxMirthXMLBytes/1024/1024)
	}
	return []byte(body.ChannelXML), nil
}

// userIDFromContext extracts the authenticated user ID.
// The Node.js proxy injects X-User-ID after JWT verification.
func (ctrl *MirthMigrationController) userIDFromContext(c *gin.Context) string {
	if h := c.GetHeader("X-User-ID"); h != "" {
		return h
	}
	if v, exists := c.Get("userID"); exists {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return "unknown"
}
