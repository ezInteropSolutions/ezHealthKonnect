package enrichment

import (
	"bytes"
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// ===============================================================
// HTTP INTEGRATION TESTS FOR FILE PARSER API ENDPOINTS
// ===============================================================
// Tests the two inline Gin handlers from main.go:
//   GET  /api/fhir/file-parser/templates
//   POST /api/fhir/file-parser/preview
//
// Uses httptest.NewRecorder() + a local Gin router that mirrors
// the production handlers exactly — no running server required.

// ─── Router Setup ───────────────────────────────────────────────

// setupFileParserAPIRouter builds a Gin test router that replicates
// the two file-parser handler closures from main.go.
func setupFileParserAPIRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// GET /api/fhir/file-parser/templates
	router.GET("/api/fhir/file-parser/templates", func(c *gin.Context) {
		list := GetTemplateList()
		byCategory := GetTemplatesByCategory()
		c.JSON(http.StatusOK, gin.H{
			"success":     true,
			"templates":   list,
			"by_category": byCategory,
			"count":       len(list),
		})
	})

	// POST /api/fhir/file-parser/preview  (mirrors main.go verbatim)
	router.POST("/api/fhir/file-parser/preview", func(c *gin.Context) {
		var req struct {
			Content string                 `json:"content"`
			Config  map[string]interface{} `json:"config"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}
		if req.Config == nil {
			req.Config = make(map[string]interface{})
		}

		// Cap preview at 10 records unless caller overrides
		if maxR, ok := req.Config["maxRecords"]; !ok || maxR == nil || maxR == float64(0) {
			req.Config["maxRecords"] = 10
		}
		req.Config["sourceField"] = "_preview_content"

		executor := NewFileParserExecutor(nil, nil)
		step := &models.TransformationStep{
			StepName: "preview",
			StepType: "file_parser",
			Enabled:  true,
			Config:   req.Config,
		}
		inputData := map[string]interface{}{
			"_preview_content": req.Content,
		}

		output, execErr := executor.Execute(c.Request.Context(), step, inputData)
		if execErr != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"error":   execErr.Error(),
			})
			return
		}

		stepOutput, _ := output["_stepOutput"].(map[string]interface{})
		execDetails, _ := output["_executionDetails"].(map[string]interface{})

		preview := gin.H{
			"record_count": stepOutput["record_count"],
			"column_count": stepOutput["column_count"],
			"columns":      stepOutput["columns"],
			"records":      stepOutput["records"],
		}
		if execDetails != nil {
			preview["format"] = execDetails["format"]
			if autoDetected, ok := execDetails["auto_detected"]; ok {
				preview["auto_detected"] = autoDetected
			}
			if detectedFmt, ok := execDetails["detected_format"]; ok {
				preview["detected_format"] = detectedFmt
			}
			if detectedDelim, ok := execDetails["detected_delimiter"]; ok {
				preview["detected_delimiter"] = detectedDelim
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"preview": preview,
		})
	})

	return router
}

// ─── Test Helpers ───────────────────────────────────────────────

// doGET performs a GET request against the test router.
func doGET(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// doPOST performs a POST request with a JSON body.
func doPOST(router *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, path,
		bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// parseBody unmarshals the response body into a generic map.
func parseBody(rec *httptest.ResponseRecorder) map[string]interface{} {
	var out map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

// getPreview extracts the "preview" sub-map from the top-level response.
func getPreview(body map[string]interface{}) map[string]interface{} {
	if p, ok := body["preview"].(map[string]interface{}); ok {
		return p
	}
	return nil
}

// ─── SUITE A: GET /api/fhir/file-parser/templates ───────────────

func TestFileParserAPI_Templates_ReturnsHTTP200(t *testing.T) {
	router := setupFileParserAPIRouter()
	rec := doGET(router, "/api/fhir/file-parser/templates")

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
}

func TestFileParserAPI_Templates_SuccessTrue(t *testing.T) {
	router := setupFileParserAPIRouter()
	rec := doGET(router, "/api/fhir/file-parser/templates")
	body := parseBody(rec)

	if body["success"] != true {
		t.Errorf("Expected success=true, got %v", body["success"])
	}
}

func TestFileParserAPI_Templates_ContainsKnownKeys(t *testing.T) {
	router := setupFileParserAPIRouter()
	rec := doGET(router, "/api/fhir/file-parser/templates")
	body := parseBody(rec)

	templates, ok := body["templates"].([]interface{})
	if !ok {
		t.Fatalf("Expected templates array, got %T", body["templates"])
	}
	if len(templates) == 0 {
		t.Fatal("Templates list is empty")
	}

	// Build a set of returned keys for quick lookup
	keySet := make(map[string]bool)
	for _, tmpl := range templates {
		if m, ok := tmpl.(map[string]interface{}); ok {
			if key, ok := m["key"].(string); ok {
				keySet[key] = true
			}
		}
	}

	for _, required := range []string{"cclf1", "cclf2", "nacha_entry", "era_835_header", "eligibility_834"} {
		if !keySet[required] {
			t.Errorf("Expected template key %q not found in list", required)
		}
	}
}

func TestFileParserAPI_Templates_CountMatchesList(t *testing.T) {
	router := setupFileParserAPIRouter()
	rec := doGET(router, "/api/fhir/file-parser/templates")
	body := parseBody(rec)

	templates, _ := body["templates"].([]interface{})
	count, _ := body["count"].(float64)

	if int(count) != len(templates) {
		t.Errorf("count=%d does not match len(templates)=%d", int(count), len(templates))
	}
}

func TestFileParserAPI_Templates_ByCategory_HasExpectedCategories(t *testing.T) {
	router := setupFileParserAPIRouter()
	rec := doGET(router, "/api/fhir/file-parser/templates")
	body := parseBody(rec)

	byCategory, ok := body["by_category"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected by_category map, got %T", body["by_category"])
	}

	for _, cat := range []string{"CMS/CCLF", "NACHA/ACH", "ERA/Remittance"} {
		if _, found := byCategory[cat]; !found {
			t.Errorf("Expected category %q in by_category, but not found", cat)
		}
	}
}

func TestFileParserAPI_Templates_EachEntryHasRequiredFields(t *testing.T) {
	router := setupFileParserAPIRouter()
	rec := doGET(router, "/api/fhir/file-parser/templates")
	body := parseBody(rec)

	templates, _ := body["templates"].([]interface{})
	for i, tmpl := range templates {
		m, ok := tmpl.(map[string]interface{})
		if !ok {
			t.Errorf("templates[%d] is not a map", i)
			continue
		}
		for _, field := range []string{"key", "name", "description", "category", "columnCount"} {
			if _, exists := m[field]; !exists {
				t.Errorf("templates[%d] missing field %q", i, field)
			}
		}
		// columnCount must be > 0
		if cc, ok := m["columnCount"].(float64); !ok || cc <= 0 {
			t.Errorf("templates[%d].columnCount expected > 0, got %v", i, m["columnCount"])
		}
	}
}

// ─── SUITE B: POST /api/fhir/file-parser/preview ────────────────

func TestFileParserAPI_Preview_CSV_ReturnsRecords(t *testing.T) {
	router := setupFileParserAPIRouter()

	payload := map[string]interface{}{
		"content": "name,age,city\nAlice,30,NYC\nBob,25,LA\nCharlie,35,Chicago",
		"config": map[string]interface{}{
			"fileFormat": "csv",
			"hasHeader":  true,
		},
	}
	rec := doPOST(router, "/api/fhir/file-parser/preview", payload)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	body := parseBody(rec)
	if body["success"] != true {
		t.Fatalf("Expected success=true, got %v — body: %s", body["success"], rec.Body.String())
	}

	preview := getPreview(body)
	if preview == nil {
		t.Fatal("preview map not found in response")
	}

	recordCount, _ := preview["record_count"].(float64)
	if recordCount != 3 {
		t.Errorf("Expected record_count=3, got %v", preview["record_count"])
	}

	records, ok := preview["records"].([]interface{})
	if !ok {
		t.Fatalf("Expected records array, got %T", preview["records"])
	}
	first, _ := records[0].(map[string]interface{})
	if first["name"] != "Alice" {
		t.Errorf("Expected first record name=Alice, got %v", first["name"])
	}
}

func TestFileParserAPI_Preview_TSV_Detected(t *testing.T) {
	router := setupFileParserAPIRouter()

	payload := map[string]interface{}{
		"content": "id\tpatient\tdiagnosis\n1\tAlice\tDiabetes\n2\tBob\tHypertension",
		"config": map[string]interface{}{
			"fileFormat": "tsv",
			"hasHeader":  true,
		},
	}
	rec := doPOST(router, "/api/fhir/file-parser/preview", payload)

	body := parseBody(rec)
	if body["success"] != true {
		t.Fatalf("Expected success=true, body: %s", rec.Body.String())
	}

	preview := getPreview(body)
	recordCount, _ := preview["record_count"].(float64)
	if recordCount != 2 {
		t.Errorf("Expected record_count=2 for TSV, got %v", recordCount)
	}

	records, _ := preview["records"].([]interface{})
	first, _ := records[0].(map[string]interface{})
	if first["patient"] != "Alice" {
		t.Errorf("Expected patient=Alice, got %v", first["patient"])
	}
}

func TestFileParserAPI_Preview_FixedWidth_ManualColumns(t *testing.T) {
	router := setupFileParserAPIRouter()

	// Simulate a 3-record CCLF-style file (manually defined columns)
	lines := []string{
		"AA00000000001BCDEFGHIJK11",
		"BB11111111112LMNOPQRSTU22",
		"CC22222222223VWXYZABCDE33",
	}
	content := strings.Join(lines, "\n")

	payload := map[string]interface{}{
		"content": content,
		"config": map[string]interface{}{
			"fileFormat": "fixed_width",
			"hasHeader":  false,
			"columns": []interface{}{
				map[string]interface{}{"name": "ID_PREFIX", "start": 1, "length": 2},
				map[string]interface{}{"name": "CLAIM_ID", "start": 3, "length": 11},
				map[string]interface{}{"name": "SEQ", "start": 14, "length": 1},
				map[string]interface{}{"name": "NAME", "start": 15, "length": 11},
			},
		},
	}
	rec := doPOST(router, "/api/fhir/file-parser/preview", payload)

	body := parseBody(rec)
	if body["success"] != true {
		t.Fatalf("Expected success=true, body: %s", rec.Body.String())
	}

	preview := getPreview(body)
	recordCount, _ := preview["record_count"].(float64)
	if recordCount != 3 {
		t.Errorf("Expected record_count=3 for fixed_width, got %v", recordCount)
	}

	records, _ := preview["records"].([]interface{})
	first, _ := records[0].(map[string]interface{})
	if first["ID_PREFIX"] != "AA" {
		t.Errorf("Expected ID_PREFIX=AA, got %v", first["ID_PREFIX"])
	}
}

func TestFileParserAPI_Preview_OOBTemplate_CCLF1(t *testing.T) {
	router := setupFileParserAPIRouter()

	// Build a CCLF1-format line: CUR_CLM_UNIQ_ID (pos 1-13), BENE_MBI_ID (pos 14-24)
	// pad to appropriate length for other columns
	line1 := "CLAIMID000001MBID1234567  " + strings.Repeat(" ", 150)
	line2 := "CLAIMID000002MBID7654321  " + strings.Repeat(" ", 150)

	payload := map[string]interface{}{
		"content": line1 + "\n" + line2,
		"config": map[string]interface{}{
			"fileFormat": "fixed_width",
			"hasHeader":  false,
			"template":   "cclf1",
		},
	}
	rec := doPOST(router, "/api/fhir/file-parser/preview", payload)

	body := parseBody(rec)
	if body["success"] != true {
		t.Fatalf("Expected success=true (CCLF1 template), body: %s", rec.Body.String())
	}

	preview := getPreview(body)
	recordCount, _ := preview["record_count"].(float64)
	if recordCount != 2 {
		t.Errorf("Expected record_count=2, got %v", recordCount)
	}

	records, _ := preview["records"].([]interface{})
	first, _ := records[0].(map[string]interface{})

	// CCLF1 column: CUR_CLM_UNIQ_ID pos 1-13
	claimID, _ := first["CUR_CLM_UNIQ_ID"].(string)
	if !strings.HasPrefix(strings.TrimSpace(claimID), "CLAIMID") {
		t.Errorf("Expected CUR_CLM_UNIQ_ID to start with CLAIMID, got %q", claimID)
	}
}

func TestFileParserAPI_Preview_AutoDetect_DetectsCSV(t *testing.T) {
	router := setupFileParserAPIRouter()

	payload := map[string]interface{}{
		"content": "patient_id,last_name,dob\nP001,Smith,1985-06-15\nP002,Jones,1990-11-22",
		"config": map[string]interface{}{
			"autoDetect": true,
		},
	}
	rec := doPOST(router, "/api/fhir/file-parser/preview", payload)

	body := parseBody(rec)
	if body["success"] != true {
		t.Fatalf("Expected success=true (auto-detect CSV), body: %s", rec.Body.String())
	}

	preview := getPreview(body)
	recordCount, _ := preview["record_count"].(float64)
	if recordCount != 2 {
		t.Errorf("Expected record_count=2, got %v", recordCount)
	}

	// Verify format was detected as csv
	detectedFormat, _ := preview["format"].(string)
	if detectedFormat != "csv" {
		t.Errorf("Expected detected format=csv, got %q", detectedFormat)
	}

	records, _ := preview["records"].([]interface{})
	first, _ := records[0].(map[string]interface{})
	if first["patient_id"] != "P001" {
		t.Errorf("Expected patient_id=P001, got %v", first["patient_id"])
	}
}

func TestFileParserAPI_Preview_CappedAt10Records(t *testing.T) {
	router := setupFileParserAPIRouter()

	// Build 15-record CSV (header + 15 data rows)
	rows := []string{"id,value"}
	for i := 1; i <= 15; i++ {
		rows = append(rows, strings.Join([]string{
			strings.Repeat("0", 4-len(string(rune('0'+i/10))))+string(rune('0'+i%10)),
			strings.Repeat("V", 5),
		}, ","))
	}
	// Simpler: generate rows programmatically
	var sb strings.Builder
	sb.WriteString("id,value\n")
	for i := 1; i <= 15; i++ {
		sb.WriteString(strings.Repeat("0", 2))
		_ = i // just need some rows
		sb.WriteString("\n")
	}

	// Build 15 valid rows properly
	var content strings.Builder
	content.WriteString("idx,label\n")
	for i := 1; i <= 15; i++ {
		content.WriteString(strings.Repeat("", 0))
		content.WriteString(string(rune('A'+i-1)))
		content.WriteString(",row")
		content.WriteString(string(rune('0' + i)))
		content.WriteString("\n")
	}

	payload := map[string]interface{}{
		"content": content.String(),
		"config": map[string]interface{}{
			"fileFormat": "csv",
			"hasHeader":  true,
			// maxRecords not set → should default to 10
		},
	}
	rec := doPOST(router, "/api/fhir/file-parser/preview", payload)

	body := parseBody(rec)
	if body["success"] != true {
		t.Fatalf("Expected success=true, body: %s", rec.Body.String())
	}

	preview := getPreview(body)
	records, _ := preview["records"].([]interface{})
	if len(records) > 10 {
		t.Errorf("Expected max 10 records from preview (cap), got %d", len(records))
	}
}

func TestFileParserAPI_Preview_BadJSON_Returns400(t *testing.T) {
	router := setupFileParserAPIRouter()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/fhir/file-parser/preview",
		bytes.NewBufferString(`{not valid json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected HTTP 400 for bad JSON, got %d", rec.Code)
	}
	body := parseBody(rec)
	if body["success"] != false {
		t.Errorf("Expected success=false for bad JSON, got %v", body["success"])
	}
}

func TestFileParserAPI_Preview_UnknownFormat_ReturnsError(t *testing.T) {
	router := setupFileParserAPIRouter()

	payload := map[string]interface{}{
		"content": "some data",
		"config": map[string]interface{}{
			"fileFormat": "zarr", // unknown/unsupported format
		},
	}
	rec := doPOST(router, "/api/fhir/file-parser/preview", payload)

	// Handler returns 200 with success:false for executor errors
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200, got %d", rec.Code)
	}
	body := parseBody(rec)
	if body["success"] != false {
		t.Errorf("Expected success=false for unknown format, got %v", body["success"])
	}
	if _, hasErr := body["error"]; !hasErr {
		t.Error("Expected 'error' field in response for unknown format")
	}
}

func TestFileParserAPI_Preview_EmptyContent_ReturnsZeroRecords(t *testing.T) {
	router := setupFileParserAPIRouter()

	payload := map[string]interface{}{
		"content": "",
		"config": map[string]interface{}{
			"fileFormat": "csv",
			"hasHeader":  true,
		},
	}
	rec := doPOST(router, "/api/fhir/file-parser/preview", payload)

	body := parseBody(rec)
	// Empty content either returns 0 records or an error — both acceptable
	if rec.Code == http.StatusOK {
		if body["success"] == true {
			preview := getPreview(body)
			if preview != nil {
				records, _ := preview["records"].([]interface{})
				if len(records) > 0 {
					t.Errorf("Expected 0 records for empty content, got %d", len(records))
				}
			}
		}
		// success:false with an error is also acceptable
	}
}

func TestFileParserAPI_Preview_PipeDelimited_AutoDetect(t *testing.T) {
	router := setupFileParserAPIRouter()

	payload := map[string]interface{}{
		"content": "mrn|last_name|first_name|dob\nMRN001|Smith|Alice|1985-06-15\nMRN002|Jones|Bob|1990-11-22",
		"config": map[string]interface{}{
			"autoDetect": true,
		},
	}
	rec := doPOST(router, "/api/fhir/file-parser/preview", payload)

	body := parseBody(rec)
	if body["success"] != true {
		t.Fatalf("Expected success=true, body: %s", rec.Body.String())
	}

	preview := getPreview(body)
	recordCount, _ := preview["record_count"].(float64)
	if recordCount != 2 {
		t.Errorf("Expected record_count=2 for pipe-delimited, got %v", recordCount)
	}

	records, _ := preview["records"].([]interface{})
	first, _ := records[0].(map[string]interface{})
	if first["mrn"] != "MRN001" {
		t.Errorf("Expected mrn=MRN001, got %v", first["mrn"])
	}
}

func TestFileParserAPI_Preview_NACHATemplate(t *testing.T) {
	router := setupFileParserAPIRouter()

	// NACHA Entry Detail record (Record Type 6): 94 chars wide
	// record_type(1) | transaction_code(2) | routing(9) | account(17) | amount(10) | individual_id(15) | individual_name(22) | discretionary(2) | addenda(1) | trace(15)
	nachaLine := "6" +       // record_type pos 1
		"22" +               // transaction_code pos 2-3
		"021000021" +        // routing_number pos 4-12
		"123456789012345  " + // account_number pos 13-29 (17 chars)
		"0000015000" +       // amount pos 30-39 (10 chars)
		"SMITH         " +   // individual_id pos 40-54 (15 chars)
		"ALICE SMITH           " + // individual_name pos 55-76 (22 chars)
		"  " +               // discretionary_data pos 77-78
		"0" +                // addenda_indicator pos 79
		"021000021000001" // trace_number pos 80-94

	payload := map[string]interface{}{
		"content": nachaLine + "\n" + nachaLine,
		"config": map[string]interface{}{
			"fileFormat": "fixed_width",
			"hasHeader":  false,
			"template":   "nacha_entry",
		},
	}
	rec := doPOST(router, "/api/fhir/file-parser/preview", payload)

	body := parseBody(rec)
	if body["success"] != true {
		t.Fatalf("Expected success=true (NACHA template), body: %s", rec.Body.String())
	}

	preview := getPreview(body)
	recordCount, _ := preview["record_count"].(float64)
	if recordCount != 2 {
		t.Errorf("Expected record_count=2, got %v", recordCount)
	}

	// Verify the RECORD_TYPE_CODE column was parsed correctly
	records, _ := preview["records"].([]interface{})
	first, _ := records[0].(map[string]interface{})
	recordType, _ := first["RECORD_TYPE_CODE"].(string)
	if strings.TrimSpace(recordType) != "6" {
		t.Errorf("Expected RECORD_TYPE_CODE=6 from NACHA template, got %q", recordType)
	}
}

func TestFileParserAPI_Preview_MaxRecords_Override(t *testing.T) {
	router := setupFileParserAPIRouter()

	// Build 10-record CSV
	var content strings.Builder
	content.WriteString("id,name\n")
	for i := 1; i <= 10; i++ {
		content.WriteString(strings.Repeat("", 0))
		// Simple line
		content.WriteString("R")
		content.WriteString(string(rune('0' + i)))
		content.WriteString(",Person")
		content.WriteString(string(rune('A' + i - 1)))
		content.WriteString("\n")
	}

	payload := map[string]interface{}{
		"content": content.String(),
		"config": map[string]interface{}{
			"fileFormat": "csv",
			"hasHeader":  true,
			"maxRecords": 3, // caller explicitly requests only 3
		},
	}
	rec := doPOST(router, "/api/fhir/file-parser/preview", payload)

	body := parseBody(rec)
	if body["success"] != true {
		t.Fatalf("Expected success=true, body: %s", rec.Body.String())
	}

	preview := getPreview(body)
	records, _ := preview["records"].([]interface{})
	if len(records) > 3 {
		t.Errorf("Expected at most 3 records (maxRecords override), got %d", len(records))
	}
}
