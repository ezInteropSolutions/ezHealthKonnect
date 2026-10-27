// controllers/cda_code_translations_controller_test.go
//
// Drives the real Code Translations endpoints against a real Postgres (DATABASE_URL), through a real gin
// router. Needs migrations through V283 applied.
//
// Run: DATABASE_URL="postgres://..." go test ./controllers/ -v -run TestCDACodeTranslations
package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cdaterminology "ezhealthkonnect/services/cda_terminology"

	"github.com/gin-gonic/gin"
)

type translationsAPI struct {
	t      *testing.T
	router *gin.Engine
}

func (a *translationsAPI) call(method, path, body string) (int, map[string]interface{}) {
	a.t.Helper()
	var rd *strings.Reader
	if body != "" {
		rd = strings.NewReader(body)
	} else {
		rd = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.router.ServeHTTP(w, req)
	var out map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestCDACodeTranslations_FullLifecycle(t *testing.T) {
	db, cleanup, ok := testDB(t)
	if !ok {
		t.Skip("DATABASE_URL not set")
	}
	defer cleanup()

	var indexExists bool
	if err := db.QueryRow(`SELECT to_regclass('uq_cda_code_translations_global') IS NOT NULL`).Scan(&indexExists); err != nil || !indexExists {
		t.Skip("migration V283 not applied (uq_cda_code_translations_global missing)")
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewCDACodeTranslationsController(cdaterminology.NewTranslationRepository(db), nil).RegisterRoutes(router.Group("/api/cda"))
	api := &translationsAPI{t: t, router: router}

	sys := fmt.Sprintf("urn:test:lab-%d", time.Now().UnixNano())
	defer db.Exec(`DELETE FROM cda_code_translations WHERE source_system = $1`, sys)
	const base = "/api/cda/code-translations"
	body := func(code, target string) string {
		return fmt.Sprintf(`{"sourceSystem":%q,"sourceCode":%q,"targetSystem":"http://loinc.org","targetCode":%q,"targetDisplay":"Glucose"}`, sys, code, target)
	}

	// Create a shared translation.
	code, resp := api.call("POST", base, body("GLU", "2345-7"))
	if code != http.StatusCreated {
		t.Fatalf("create: status %d, body %v", code, resp)
	}
	id := resp["data"].(map[string]interface{})["id"].(string)

	// The same source code again is a conflict, not a silent second row (the V283 fix).
	if code, _ := api.call("POST", base, body("GLU", "9999-9")); code != http.StatusConflict {
		t.Fatalf("duplicate shared create: want 409, got %d", code)
	}

	// Change its target.
	if code, resp := api.call("PUT", base+"/"+id, body("GLU", "2339-0")); code != http.StatusOK ||
		resp["data"].(map[string]interface{})["targetCode"] != "2339-0" {
		t.Fatalf("update: status %d, body %v", code, resp)
	}

	// Searching finds it, and only it.
	code, resp = api.call("GET", base+"?scope=global&search="+sys, "")
	if code != http.StatusOK || resp["total"].(float64) != 1 {
		t.Fatalf("list: status %d, body %v", code, resp)
	}

	// Import: one bad row is reported, the others are saved; importing again updates instead of duplicating.
	imp := fmt.Sprintf(`{"rows":[%s,%s,{"sourceSystem":%q,"sourceCode":"","targetSystem":"x","targetCode":"y"}]}`,
		body("NA", "2951-2"), body("K", "2823-3"), sys)
	code, resp = api.call("POST", base+"/import", imp)
	if code != http.StatusOK || resp["saved"].(float64) != 2 || len(resp["rejected"].([]interface{})) != 1 {
		t.Fatalf("import: status %d, body %v", code, resp)
	}
	if rej := resp["rejected"].([]interface{})[0].(map[string]interface{}); rej["row"].(float64) != 3 {
		t.Fatalf("import: rejected row should be #3, got %v", rej)
	}
	api.call("POST", base+"/import", imp)
	if _, resp = api.call("GET", base+"?scope=global&search="+sys, ""); resp["total"].(float64) != 3 {
		t.Fatalf("re-import must update, not duplicate: want 3 rows, got %v", resp["total"])
	}

	// A translation for an interface that does not exist is a clear 400.
	bad := fmt.Sprintf(`{"interfaceId":"00000000-0000-4000-8000-000000000000","sourceSystem":%q,"sourceCode":"X","targetSystem":"s","targetCode":"c"}`, sys)
	if code, _ := api.call("POST", base, bad); code != http.StatusBadRequest {
		t.Fatalf("unknown interface: want 400, got %d", code)
	}

	// Required fields and a malformed id.
	if code, _ := api.call("POST", base, `{"sourceSystem":"a"}`); code != http.StatusBadRequest {
		t.Fatalf("missing fields: want 400, got %d", code)
	}
	if code, _ := api.call("PUT", base+"/not-a-uuid", body("A", "b")); code != http.StatusNotFound {
		t.Fatalf("malformed id: want 404, got %d", code)
	}

	// Delete, then it is gone.
	if code, _ := api.call("DELETE", base+"/"+id, ""); code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d", code)
	}
	if code, _ := api.call("DELETE", base+"/"+id, ""); code != http.StatusNotFound {
		t.Fatalf("second delete: want 404, got %d", code)
	}
}
