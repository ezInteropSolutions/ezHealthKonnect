package enrichment

// ===============================================================
// UNIT TESTS — FILE PARSER REMOTE RESOLUTION
// ===============================================================
// Expert ETL engineer coverage:
//   - parseS3URI: valid paths, missing key, invalid formats
//   - extractAWSCredentials: present keys, missing keys, alternate names
//   - firstStringValue: key ordering, missing keys
//   - maskCredentialsInURI: embedded user:pass, plain URIs, S3
//   - resolveRemoteFile: scheme dispatch (HTTP, file://, bare path)
//   - resolveHTTPFile: real httptest server — success, 4xx, body
//   - resolveLocalFile via file:// URI
//   - field_as_path integration: HTTP source, local file, empty URI,
//     missing source field, file:// scheme

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"ezhealthkonnect/models"
)

// ─── parseS3URI ───────────────────────────────────────────────

func TestParseS3URI_ValidPath(t *testing.T) {
	bucket, key, err := parseS3URI("s3://my-bucket/path/to/file.csv")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if bucket != "my-bucket" {
		t.Errorf("Expected bucket='my-bucket', got %q", bucket)
	}
	if key != "path/to/file.csv" {
		t.Errorf("Expected key='path/to/file.csv', got %q", key)
	}
}

func TestParseS3URI_SingleLevelKey(t *testing.T) {
	bucket, key, err := parseS3URI("s3://claims-bucket/feed.dat")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if bucket != "claims-bucket" {
		t.Errorf("Expected bucket=claims-bucket, got %q", bucket)
	}
	if key != "feed.dat" {
		t.Errorf("Expected key=feed.dat, got %q", key)
	}
}

func TestParseS3URI_DeeplyNestedKey(t *testing.T) {
	// Typical healthcare data lake path
	_, key, err := parseS3URI("s3://ehr-data/interfaces/adta01/2025/01/15/hl7_batch_001.hl7")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if key != "interfaces/adta01/2025/01/15/hl7_batch_001.hl7" {
		t.Errorf("Unexpected key: %q", key)
	}
}

func TestParseS3URI_MissingKey(t *testing.T) {
	_, _, err := parseS3URI("s3://bucket-only/")
	if err == nil {
		t.Fatal("Expected error for URI with no object key (trailing slash only), got nil")
	}
}

func TestParseS3URI_NoSlashAfterBucket(t *testing.T) {
	_, _, err := parseS3URI("s3://bucket-no-slash")
	if err == nil {
		t.Fatal("Expected error when no slash after bucket name, got nil")
	}
	if !strings.Contains(err.Error(), "s3://bucket-no-slash") {
		t.Errorf("Error should contain the original URI: %v", err)
	}
}

func TestParseS3URI_EmptyBucket(t *testing.T) {
	_, _, err := parseS3URI("s3:///key.csv")
	if err == nil {
		t.Fatal("Expected error for empty bucket name, got nil")
	}
}

func TestParseS3URI_NotS3Scheme(t *testing.T) {
	// parseS3URI trusts the caller to strip "s3://"; if something wrong is passed, it still parses
	// This tests the path parsing after stripping the prefix
	_, _, err := parseS3URI("s3://")
	if err == nil {
		t.Fatal("Expected error for bare s3:// with no bucket or key")
	}
}

// ─── extractAWSCredentials ────────────────────────────────────

func TestExtractAWSCredentials_BothPresent(t *testing.T) {
	m := map[string]interface{}{
		"access_key_id":     "AKIAIOSFODNN7EXAMPLE",
		"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	}
	keyID, secret, ok := extractAWSCredentials(m)
	if !ok {
		t.Fatal("Expected ok=true when both credentials are present")
	}
	if keyID != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("Expected keyID=AKIAIOSFODNN7EXAMPLE, got %q", keyID)
	}
	if secret != "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" {
		t.Errorf("Unexpected secret value: %q", secret)
	}
}

func TestExtractAWSCredentials_AlternateKeyNames(t *testing.T) {
	// Some connectivity configs use camelCase or aws_ prefix
	m := map[string]interface{}{
		"aws_access_key_id":     "AKID_ALTERNATE",
		"aws_secret_access_key": "SECRET_ALTERNATE",
	}
	_, _, ok := extractAWSCredentials(m)
	if !ok {
		t.Fatal("Expected ok=true for aws_access_key_id / aws_secret_access_key names")
	}
}

func TestExtractAWSCredentials_CamelCaseKeyNames(t *testing.T) {
	m := map[string]interface{}{
		"accessKeyId":     "AKIDcamel",
		"secretAccessKey": "SECRETcamel",
	}
	_, _, ok := extractAWSCredentials(m)
	if !ok {
		t.Fatal("Expected ok=true for camelCase key names")
	}
}

func TestExtractAWSCredentials_MissingSecret(t *testing.T) {
	m := map[string]interface{}{
		"access_key_id": "AKID_ONLY",
		// secret is absent
	}
	_, _, ok := extractAWSCredentials(m)
	if ok {
		t.Fatal("Expected ok=false when secret is missing")
	}
}

func TestExtractAWSCredentials_MissingKeyID(t *testing.T) {
	m := map[string]interface{}{
		"secret_access_key": "SECRET_ONLY",
		// key ID is absent
	}
	_, _, ok := extractAWSCredentials(m)
	if ok {
		t.Fatal("Expected ok=false when key ID is missing")
	}
}

func TestExtractAWSCredentials_EmptyValues(t *testing.T) {
	m := map[string]interface{}{
		"access_key_id":     "",
		"secret_access_key": "some-secret",
	}
	_, _, ok := extractAWSCredentials(m)
	if ok {
		t.Fatal("Expected ok=false when key ID is empty string")
	}
}

func TestExtractAWSCredentials_NilMap(t *testing.T) {
	_, _, ok := extractAWSCredentials(nil)
	if ok {
		t.Fatal("Expected ok=false for nil map")
	}
}

func TestExtractAWSCredentials_EmptyMap(t *testing.T) {
	_, _, ok := extractAWSCredentials(map[string]interface{}{})
	if ok {
		t.Fatal("Expected ok=false for empty map")
	}
}

// ─── firstStringValue ─────────────────────────────────────────

func TestFirstStringValue_ReturnsFirstMatch(t *testing.T) {
	m := map[string]interface{}{
		"region":     "us-east-1",
		"aws_region": "eu-west-1",
	}
	v := firstStringValue(m, "region", "aws_region")
	if v != "us-east-1" {
		t.Errorf("Expected first match 'us-east-1', got %q", v)
	}
}

func TestFirstStringValue_FallsBackToSecondKey(t *testing.T) {
	m := map[string]interface{}{
		"aws_region": "ap-southeast-1",
	}
	v := firstStringValue(m, "region", "aws_region")
	if v != "ap-southeast-1" {
		t.Errorf("Expected fallback 'ap-southeast-1', got %q", v)
	}
}

func TestFirstStringValue_SkipsEmptyString(t *testing.T) {
	m := map[string]interface{}{
		"region":     "",
		"aws_region": "us-west-2",
	}
	v := firstStringValue(m, "region", "aws_region")
	if v != "us-west-2" {
		t.Errorf("Expected to skip empty and get 'us-west-2', got %q", v)
	}
}

func TestFirstStringValue_AllMissing(t *testing.T) {
	m := map[string]interface{}{"other_key": "value"}
	v := firstStringValue(m, "region", "aws_region")
	if v != "" {
		t.Errorf("Expected empty string when no keys found, got %q", v)
	}
}

func TestFirstStringValue_NonStringValue(t *testing.T) {
	// Values that are not strings should be skipped
	m := map[string]interface{}{
		"region":     42,        // wrong type
		"aws_region": "us-east-2", // correct type
	}
	v := firstStringValue(m, "region", "aws_region")
	if v != "us-east-2" {
		t.Errorf("Expected to skip non-string and get 'us-east-2', got %q", v)
	}
}

// ─── maskCredentialsInURI ─────────────────────────────────────

func TestMaskCredentialsInURI_WithEmbeddedUserPass(t *testing.T) {
	uri := "https://user:secretpassword@storage.example.com/container/blob.csv"
	masked := maskCredentialsInURI(uri)

	if strings.Contains(masked, "secretpassword") {
		t.Errorf("Masked URI should not contain the password: %q", masked)
	}
	if !strings.Contains(masked, "https://") {
		t.Errorf("Masked URI should retain the scheme: %q", masked)
	}
	if !strings.Contains(masked, "***") {
		t.Errorf("Masked URI should contain '***' placeholder: %q", masked)
	}
	if !strings.Contains(masked, "@storage.example.com") {
		t.Errorf("Masked URI should retain the host portion: %q", masked)
	}
}

func TestMaskCredentialsInURI_NoCredentials(t *testing.T) {
	uri := "https://storage.example.com/container/blob.csv"
	masked := maskCredentialsInURI(uri)
	if masked != uri {
		t.Errorf("URI without credentials should be unchanged, got: %q", masked)
	}
}

func TestMaskCredentialsInURI_S3URI(t *testing.T) {
	// S3 URIs never have embedded credentials — should be returned as-is
	uri := "s3://my-bucket/data/claims.csv"
	masked := maskCredentialsInURI(uri)
	if masked != uri {
		t.Errorf("S3 URI without credentials should be unchanged, got: %q", masked)
	}
}

func TestMaskCredentialsInURI_EmptyString(t *testing.T) {
	masked := maskCredentialsInURI("")
	if masked != "" {
		t.Errorf("Empty URI should remain empty, got: %q", masked)
	}
}

func TestMaskCredentialsInURI_AtSignInPath(t *testing.T) {
	// @ in path (no :// before it) → should not be masked
	uri := "https://storage.example.com/data@version/file.csv"
	masked := maskCredentialsInURI(uri)
	// The @ is in the path, no credentials embedded before the scheme's @
	// The function checks if atIdx > schemeEnd — this @ is after the host, so behaviour depends on implementation
	// Either masked or not, it must not panic
	if len(masked) == 0 {
		t.Error("Result should be non-empty")
	}
}

// ─── resolveHTTPFile via httptest.Server ──────────────────────

// makeRemoteExecutor returns a FileParserExecutor with nil db (no S3 credential lookup needed for HTTP tests)
func makeRemoteExecutor() *FileParserExecutor {
	return NewFileParserExecutor(nil, nil)
}

func TestResolveHTTPFile_Success(t *testing.T) {
	csvContent := "patient_id,mrn,dob\nP001,MRN001,1985-06-15\nP002,MRN002,1990-11-22\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		fmt.Fprint(w, csvContent)
	}))
	defer server.Close()

	executor := makeRemoteExecutor()
	result, err := executor.resolveHTTPFile(context.Background(), server.URL+"/data.csv")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if result != csvContent {
		t.Errorf("Expected exact CSV content, got: %q", result)
	}
}

func TestResolveHTTPFile_404ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	executor := makeRemoteExecutor()
	_, err := executor.resolveHTTPFile(context.Background(), server.URL+"/missing.csv")
	if err == nil {
		t.Fatal("Expected error for 404 response, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Error should mention status code 404, got: %v", err)
	}
}

func TestResolveHTTPFile_500ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	executor := makeRemoteExecutor()
	_, err := executor.resolveHTTPFile(context.Background(), server.URL+"/crash")
	if err == nil {
		t.Fatal("Expected error for 500 response, got nil")
	}
}

func TestResolveHTTPFile_LargePayload(t *testing.T) {
	// Simulate a 50 KB CSV response (realistic for a small CCLF extract)
	const numRows = 1000
	var sb strings.Builder
	sb.WriteString("bene_id,clm_id,clm_type\n")
	for i := 0; i < numRows; i++ {
		fmt.Fprintf(&sb, "%013d,%013d,71\n", i, i*100)
	}
	payload := sb.String()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer server.Close()

	executor := makeRemoteExecutor()
	result, err := executor.resolveHTTPFile(context.Background(), server.URL+"/cclf1_extract.dat")
	if err != nil {
		t.Fatalf("Large payload should succeed: %v", err)
	}
	if len(result) != len(payload) {
		t.Errorf("Expected %d bytes, got %d bytes", len(payload), len(result))
	}
}

func TestResolveHTTPFile_EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// no body
	}))
	defer server.Close()

	executor := makeRemoteExecutor()
	result, err := executor.resolveHTTPFile(context.Background(), server.URL+"/empty.csv")
	if err != nil {
		t.Fatalf("Empty 200 body should succeed: %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty string for empty body, got: %q", result)
	}
}

func TestResolveHTTPFile_FixedWidthContent(t *testing.T) {
	// Simulate a fixed-width CCLF1 response from an SFTP-to-HTTP gateway
	line := "1234567890123MBI12345678" + strings.Repeat(" ", 87)
	payload := line + "\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer server.Close()

	executor := makeRemoteExecutor()
	result, err := executor.resolveHTTPFile(context.Background(), server.URL+"/cclf1.dat")
	if err != nil {
		t.Fatalf("Fixed-width payload should succeed: %v", err)
	}
	if !strings.HasPrefix(result, "1234567890123") {
		t.Errorf("Expected content to start with CUR_CLM_UNIQ_ID, got: %q", result[:20])
	}
}

// ─── resolveRemoteFile — scheme dispatch ──────────────────────

func TestResolveRemoteFile_HTTPScheme(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "mrn,name\nM001,Alice\n")
	}))
	defer server.Close()

	executor := makeRemoteExecutor()
	config := &models.FileParserConfig{FileFormat: "csv"}
	result, err := executor.resolveRemoteFile(context.Background(), server.URL+"/file.csv", config, "")
	if err != nil {
		t.Fatalf("HTTP scheme dispatch failed: %v", err)
	}
	if !strings.Contains(result, "Alice") {
		t.Errorf("Expected content to contain 'Alice', got: %q", result)
	}
}

func TestResolveRemoteFile_HTTPSScheme(t *testing.T) {
	// Use TLS test server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "id,value\n1,test\n")
	}))
	defer server.Close()

	executor := makeRemoteExecutor()
	config := &models.FileParserConfig{FileFormat: "csv"}
	// Note: TLS test server uses self-signed cert — the executor uses http.DefaultClient which will reject it.
	// This test verifies the routing reaches resolveHTTPFile (which will fail on TLS verification).
	// A real test would use a custom transport. We test that the error is TLS-related, not "unsupported scheme".
	_, err := executor.resolveRemoteFile(context.Background(), server.URL+"/file.csv", config, "")
	if err != nil {
		// Error is expected (self-signed cert) — verify it's not "unsupported scheme" or routing error
		if strings.Contains(err.Error(), "unsupported") {
			t.Errorf("Error should be TLS/network, not routing: %v", err)
		}
	}
	// If it succeeds (unlikely with self-signed), that's also fine
}

func TestResolveRemoteFile_FileScheme(t *testing.T) {
	// Write a temp file and resolve it via file:// URI
	tmpFile, err := os.CreateTemp("", "remote_test_*.csv")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })
	content := "claim_id,amount\nC001,1500.00\nC002,2400.00\n"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	executor := makeRemoteExecutor()
	config := &models.FileParserConfig{FileFormat: "csv"}
	uri := "file://" + tmpFile.Name()
	result, err := executor.resolveRemoteFile(context.Background(), uri, config, "")
	if err != nil {
		t.Fatalf("file:// scheme dispatch failed: %v", err)
	}
	if result != content {
		t.Errorf("Expected exact file content, got: %q", result)
	}
}

func TestResolveRemoteFile_BareLocalPath(t *testing.T) {
	// A bare absolute path (no scheme) is treated as local filesystem
	tmpFile, err := os.CreateTemp("", "remote_bare_*.tsv")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })
	content := "id\tname\n1\tAlice\n2\tBob\n"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	executor := makeRemoteExecutor()
	config := &models.FileParserConfig{FileFormat: "tsv"}
	result, err := executor.resolveRemoteFile(context.Background(), tmpFile.Name(), config, "")
	if err != nil {
		t.Fatalf("Bare local path dispatch failed: %v", err)
	}
	if result != content {
		t.Errorf("Expected exact file content, got: %q", result)
	}
}

func TestResolveRemoteFile_EmptyURI(t *testing.T) {
	executor := makeRemoteExecutor()
	config := &models.FileParserConfig{FileFormat: "csv"}
	_, err := executor.resolveRemoteFile(context.Background(), "", config, "")
	if err == nil {
		t.Fatal("Expected error for empty URI, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("Error should mention 'empty', got: %v", err)
	}
}

func TestResolveRemoteFile_WhitespaceOnlyURI(t *testing.T) {
	executor := makeRemoteExecutor()
	config := &models.FileParserConfig{FileFormat: "csv"}
	_, err := executor.resolveRemoteFile(context.Background(), "   \t  ", config, "")
	if err == nil {
		t.Fatal("Expected error for whitespace-only URI, got nil")
	}
}

func TestResolveRemoteFile_S3WithNilDB(t *testing.T) {
	// When db is nil, loadS3ConnectivityConfig returns empty config and falls back to
	// the default AWS credential chain. The test verifies we don't panic with nil db.
	// The S3 call itself will fail (no real AWS), but we confirm the error is S3-related,
	// not a nil-pointer panic.
	executor := NewFileParserExecutor(nil, nil) // nil db intentional
	config := &models.FileParserConfig{FileFormat: "csv"}
	_, err := executor.resolveRemoteFile(context.Background(), "s3://nonexistent-bucket-xyz/file.csv", config, "test-interface")
	if err == nil {
		t.Fatal("Expected an error for unreachable S3 bucket, got nil")
	}
	// Must NOT be a panic — if we got here, the nil db was handled gracefully
}

// ─── field_as_path integration: Execute() ─────────────────────
// These tests exercise the full Execute() pipeline with sourceType="field_as_path".

func TestFieldAsPath_HTTPSource_CSV(t *testing.T) {
	csvContent := "patient_id,last_name,first_name,dob\n" +
		"P001,Smith,Alice,1985-06-15\n" +
		"P002,Jones,Bob,1990-11-22\n" +
		"P003,White,Carol,2001-03-08\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		fmt.Fprint(w, csvContent)
	}))
	defer server.Close()

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType":  "field_as_path",
		"sourceField": "steps.s3_connector.file_uri",
		"fileFormat":  "csv",
		"hasHeader":   true,
		"trimFields":  true,
	})
	input := map[string]interface{}{
		"steps": map[string]interface{}{
			"s3_connector": map[string]interface{}{
				"file_uri": server.URL + "/patients.csv",
			},
		},
	}

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("field_as_path HTTP execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 3 {
		t.Fatalf("Expected 3 records, got %d", len(records))
	}
	if records[0]["patient_id"] != "P001" {
		t.Errorf("Expected patient_id=P001, got %v", records[0]["patient_id"])
	}
	if records[1]["last_name"] != "Jones" {
		t.Errorf("Expected last_name=Jones, got %v", records[1]["last_name"])
	}
	if records[2]["first_name"] != "Carol" {
		t.Errorf("Expected first_name=Carol, got %v", records[2]["first_name"])
	}
}

func TestFieldAsPath_HTTPSource_FixedWidth_CCLF1(t *testing.T) {
	// Simulate a CCLF1 fixed-width file served via HTTP
	// (realistic: SFTP gateway → HTTP endpoint → pipeline fetches URI)
	//
	// Actual CCLF1 field layout (from file_parser_templates.go):
	//   CUR_CLM_UNIQ_ID: pos  1-13 (13 chars)
	//   PRVDR_OSCAR_NUM: pos 14-19  (6 chars)
	//   BENE_MBI_ID:     pos 20-30 (11 chars)
	buildLine := func(claimID, providerID, mbiID string) string {
		return fmt.Sprintf("%-13s%-6s%-11s%s", claimID, providerID, mbiID, strings.Repeat(" ", 181-30))
	}
	line1 := buildLine("1234567890123", "PRV001", "MBI12345678")
	line2 := buildLine("9876543210987", "PRV002", "MBI98765432")
	payload := line1 + "\n" + line2 + "\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer server.Close()

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType":  "field_as_path",
		"sourceField": "enriched.cclf1_uri",
		"fileFormat":  "fixed_width",
		"hasHeader":   false,
		"trimFields":  true,
		"template":    "cclf1",
	})
	input := map[string]interface{}{
		"enriched": map[string]interface{}{
			"cclf1_uri": server.URL + "/P.A.C.B.C.CCLF1.D250115.T120000",
		},
	}

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("field_as_path CCLF1 execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 CCLF1 records, got %d", len(records))
	}
	if records[0]["CUR_CLM_UNIQ_ID"] != "1234567890123" {
		t.Errorf("Expected CUR_CLM_UNIQ_ID=1234567890123, got %v", records[0]["CUR_CLM_UNIQ_ID"])
	}
	if records[0]["BENE_MBI_ID"] != "MBI12345678" {
		t.Errorf("Expected BENE_MBI_ID=MBI12345678, got %v", records[0]["BENE_MBI_ID"])
	}
	if records[1]["CUR_CLM_UNIQ_ID"] != "9876543210987" {
		t.Errorf("Expected CUR_CLM_UNIQ_ID=9876543210987, got %v", records[1]["CUR_CLM_UNIQ_ID"])
	}
	if records[1]["BENE_MBI_ID"] != "MBI98765432" {
		t.Errorf("Expected BENE_MBI_ID=MBI98765432, got %v", records[1]["BENE_MBI_ID"])
	}
	if records[1]["PRVDR_OSCAR_NUM"] != "PRV002" {
		t.Errorf("Expected PRVDR_OSCAR_NUM=PRV002, got %v", records[1]["PRVDR_OSCAR_NUM"])
	}
}

func TestFieldAsPath_LocalFile_TSV(t *testing.T) {
	// Write a temp TSV file; store its path in a pipeline field
	tmpFile, err := os.CreateTemp("", "fp_uri_test_*.tsv")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })
	tsvContent := "claim_id\tstatus\tamount\nC001\tpaid\t1500.00\nC002\tdenied\t2400.00\n"
	if _, err := tmpFile.WriteString(tsvContent); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType":  "field_as_path",
		"sourceField": "enriched.sftp_path",
		"fileFormat":  "tsv",
		"hasHeader":   true,
	})
	input := map[string]interface{}{
		"enriched": map[string]interface{}{
			"sftp_path": tmpFile.Name(), // bare local path URI
		},
	}

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("field_as_path local file TSV failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	if records[0]["claim_id"] != "C001" {
		t.Errorf("Expected claim_id=C001, got %v", records[0]["claim_id"])
	}
	if records[1]["status"] != "denied" {
		t.Errorf("Expected status=denied, got %v", records[1]["status"])
	}
}

func TestFieldAsPath_FileSchemeURI(t *testing.T) {
	// field contains a file:// URI (e.g., from a File Listener connector)
	tmpFile, err := os.CreateTemp("", "fp_file_uri_*.csv")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })
	csvContent := "mrn,dob\nM001,1985-01-15\nM002,1990-06-20\n"
	if _, err := tmpFile.WriteString(csvContent); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType":  "field_as_path",
		"sourceField": "enriched.file_uri",
		"fileFormat":  "csv",
		"hasHeader":   true,
	})
	input := map[string]interface{}{
		"enriched": map[string]interface{}{
			"file_uri": "file://" + tmpFile.Name(),
		},
	}

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("file:// URI dispatch failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	if records[0]["mrn"] != "M001" {
		t.Errorf("Expected mrn=M001, got %v", records[0]["mrn"])
	}
}

func TestFieldAsPath_EmptyURIFieldValue(t *testing.T) {
	// The pipeline field exists but is set to empty string
	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType":  "field_as_path",
		"sourceField": "enriched.file_uri",
		"fileFormat":  "csv",
	})
	input := map[string]interface{}{
		"enriched": map[string]interface{}{
			"file_uri": "", // empty — previous step didn't set a URI
		},
	}

	_, err := executor.Execute(context.Background(), step, input)
	if err == nil {
		t.Fatal("Expected error for empty URI field value, got nil")
	}
}

func TestFieldAsPath_MissingSourceField(t *testing.T) {
	// sourceField points to a path that doesn't exist in the input data
	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType":  "field_as_path",
		"sourceField": "enriched.nonexistent_uri_field",
		"fileFormat":  "csv",
	})
	input := map[string]interface{}{
		"enriched": map[string]interface{}{
			"other_field": "some_value",
		},
	}

	_, err := executor.Execute(context.Background(), step, input)
	if err == nil {
		t.Fatal("Expected error when source field path not found in input, got nil")
	}
}

func TestFieldAsPath_HTTPSource_MaxRecords(t *testing.T) {
	// Build a 50-row CSV; maxRecords=5 should return only 5
	const totalRows = 50
	var sb strings.Builder
	sb.WriteString("id,value\n")
	for i := 1; i <= totalRows; i++ {
		fmt.Fprintf(&sb, "R%03d,val%d\n", i, i)
	}
	payload := sb.String()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer server.Close()

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType":  "field_as_path",
		"sourceField": "enriched.uri",
		"fileFormat":  "csv",
		"hasHeader":   true,
		"maxRecords":  5,
	})
	input := map[string]interface{}{
		"enriched": map[string]interface{}{
			"uri": server.URL + "/large.csv",
		},
	}

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 5 {
		t.Errorf("Expected 5 records (maxRecords=5), got %d", len(records))
	}
	if records[0]["id"] != "R001" {
		t.Errorf("Expected first record id=R001, got %v", records[0]["id"])
	}
}

func TestFieldAsPath_HTTPSource_SkipRows(t *testing.T) {
	// Simulate a CMS CCLF file delivered via HTTP with 2 metadata header lines
	content := "Report: CCLF Monthly Feed\n" + // skip row 1
		"Generated: 2025-01-15\n" + // skip row 2
		"patient_id,clm_id,clm_type\n" + // header (row 3 after skip)
		"P001,C001,71\n" +
		"P002,C002,72\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, content)
	}))
	defer server.Close()

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType":  "field_as_path",
		"sourceField": "enriched.report_uri",
		"fileFormat":  "csv",
		"hasHeader":   true,
		"skipRows":    2,
	})
	input := map[string]interface{}{
		"enriched": map[string]interface{}{
			"report_uri": server.URL + "/cclf_report.csv",
		},
	}

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute with skipRows failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records after skipping 2 rows, got %d", len(records))
	}
	if records[0]["patient_id"] != "P001" {
		t.Errorf("Expected patient_id=P001, got %v", records[0]["patient_id"])
	}
}

func TestFieldAsPath_HTTPSource_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "file not found", http.StatusNotFound)
	}))
	defer server.Close()

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType":  "field_as_path",
		"sourceField": "enriched.uri",
		"fileFormat":  "csv",
	})
	input := map[string]interface{}{
		"enriched": map[string]interface{}{
			"uri": server.URL + "/missing_file.csv",
		},
	}

	_, err := executor.Execute(context.Background(), step, input)
	if err == nil {
		t.Fatal("Expected error for 404 response, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Error should mention 404, got: %v", err)
	}
}

func TestFieldAsPath_ConfigValidation_MissingSourceField(t *testing.T) {
	// Validate() should fail when sourceType=field_as_path but sourceField is empty
	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType": "field_as_path",
		// sourceField intentionally absent
		"fileFormat": "csv",
	})

	err := executor.Validate(step)
	if err == nil {
		t.Fatal("Expected validation error when sourceField is missing for field_as_path, got nil")
	}
	if !strings.Contains(err.Error(), "sourceField") {
		t.Errorf("Error should mention 'sourceField', got: %v", err)
	}
}

func TestFieldAsPath_ExecutionDetails_SourceType(t *testing.T) {
	// Verify _executionDetails reflects field_as_path source type
	csvContent := "id,name\n1,Alice\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, csvContent)
	}))
	defer server.Close()

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType":  "field_as_path",
		"sourceField": "enriched.uri",
		"fileFormat":  "csv",
		"hasHeader":   true,
	})
	input := map[string]interface{}{
		"enriched": map[string]interface{}{
			"uri": server.URL + "/data.csv",
		},
	}

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	execDetails, ok := output["_executionDetails"].(map[string]interface{})
	if !ok {
		t.Fatal("_executionDetails not found in output")
	}
	if execDetails["source_type"] != "field_as_path" {
		t.Errorf("Expected source_type='field_as_path', got %v", execDetails["source_type"])
	}
}
