package enrichment

import (
	"ezhealthkonnect/models"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAPIEnrichment_HeaderOutputsCopyHeadersIntoTheOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Location", "http://x/status/7")
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	out, err := runAPIStep(t, map[string]interface{}{
		"endpoint": srv.URL + "/export", "headerOutputs": map[string]interface{}{"statusUrl": "Content-Location", "missing": "X-Nope"},
	}, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	so, _ := out["_stepOutput"].(map[string]interface{})
	if so["statusUrl"] != "http://x/status/7" {
		t.Errorf("output: %v", so)
	}
	if _, has := so["missing"]; has {
		t.Errorf("an absent header was put in the output: %v", so)
	}
}

func TestAPIEnrichment_PollUntilStatus(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) < 3 {
			w.Header().Set("X-Progress", "working")
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"type":"Patient","url":"http://x/f1","count":2}]}`))
	}))
	t.Cleanup(srv.Close)
	out, err := runAPIStep(t, map[string]interface{}{
		"endpoint": srv.URL + "/status", "poll": map[string]interface{}{"untilStatus": []interface{}{200}, "intervalSeconds": 1, "timeoutSeconds": 30},
	}, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("%d requests", n)
	}
	so, _ := out["_stepOutput"].(map[string]interface{})
	if files, _ := so["output"].([]interface{}); len(files) != 1 {
		t.Errorf("manifest not in the output: %v", so)
	}
}

func TestAPIEnrichment_PollUntilStatusTimesOutNamingTheStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) }))
	t.Cleanup(srv.Close)
	_, err := runAPIStep(t, map[string]interface{}{
		"endpoint": srv.URL + "/s", "poll": map[string]interface{}{"untilStatus": []interface{}{200}, "intervalSeconds": 1, "timeoutSeconds": 2},
	}, map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "status 202") {
		t.Fatalf("err: %v", err)
	}
}

func TestAPIEnrichment_SaveToWritesTheBodyAndKeepsItOutOfTheOutput(t *testing.T) {
	const ndjson = "{\"id\":1}\n{\"id\":2}\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(ndjson)) }))
	t.Cleanup(srv.Close)
	dir := filepath.Join(t.TempDir(), "exports", "2026")
	cfg := map[string]interface{}{
		"endpoint": srv.URL + "/f", "fieldMappings": map[string]interface{}{"name": "file"},
		"saveTo": map[string]interface{}{"directory": dir, "fileName": "patients-{name}.ndjson"},
	}
	out, err := runAPIStep(t, cfg, map[string]interface{}{"file": "0001"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "patients-0001.ndjson"))
	if err != nil || string(got) != ndjson {
		t.Fatalf("file: %q %v", got, err)
	}
	so, _ := out["_stepOutput"].(map[string]interface{})
	if so["bytes"] != len(ndjson) || so["path"] != filepath.Join(dir, "patients-0001.ndjson") || len(so) != 2 {
		t.Errorf("output: %v", so)
	}
	details, _ := out["_executionDetails"].(map[string]interface{})
	resp, _ := details["http_response"].(map[string]interface{})
	if _, kept := resp["body_raw"]; kept {
		t.Errorf("the downloaded body was kept in the step's details")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".part-*")); len(leftovers) != 0 {
		t.Errorf("temporary file left: %v", leftovers)
	}
	// an existing file is not replaced unless asked
	if _, err := runAPIStep(t, cfg, map[string]interface{}{"file": "0001"}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("overwrote without being asked: %v", err)
	}
}

func TestSaveResponseBodyNeverWritesOutsideItsDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"../../escape.txt", ".." + "\\" + ".." + "\\" + "escape.txt", "/etc/escape.txt", "a/b/escape.txt"} {
		path, _, err := saveResponseBody(&models.APISaveToConfig{Overwrite: true}, dir, name, []byte("x"))
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		if filepath.Dir(path) != dir || filepath.Base(path) != "escape.txt" {
			t.Errorf("%q written to %s", name, path)
		}
	}
	for _, bad := range []string{"", "..", "  "} {
		if _, _, err := saveResponseBody(&models.APISaveToConfig{Overwrite: true}, dir, bad, []byte("x")); err == nil {
			t.Errorf("%q accepted as a file name", bad)
		}
	}
	if _, _, err := saveResponseBody(&models.APISaveToConfig{Overwrite: true}, "", "a.txt", []byte("x")); err == nil {
		t.Error("an empty directory was accepted")
	}
}
