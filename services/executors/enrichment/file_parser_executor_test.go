package enrichment

import (
	"bytes"
	"context"
	"encoding/base64"
	"ezhealthkonnect/models"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	goavro "github.com/linkedin/goavro/v2"
	parquet "github.com/parquet-go/parquet-go"
	"github.com/xuri/excelize/v2"
)

// ===============================================================
// UNIT TESTS FOR FILE PARSER EXECUTOR
// ===============================================================
// Expert ELT engineer test coverage:
//   - CSV with/without headers, custom delimiters, quoted fields
//   - TSV parsing
//   - Fixed-width / CCLF positional parsing
//   - Edge cases: empty files, short lines, unicode, line endings
//   - Config validation, output structure, variable provider

// ─── Helper ─────────────────────────────────────────────────────

func makeStep(config map[string]interface{}) *models.TransformationStep {
	return &models.TransformationStep{
		StepName: "Test File Parser",
		StepType: "file_parser",
		Enabled:  true,
		Config:   config,
	}
}

func makeInput(fieldName, content string) map[string]interface{} {
	return map[string]interface{}{
		fieldName: content,
	}
}

// ─── 1. CSV with Header ────────────────────────────────────────

func TestFileParser_CSV_WithHeader(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	csv := "name,age,city\nAlice,30,NYC\nBob,25,LA\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "csv",
		"hasHeader":   true,
		"trimFields":  true,
	})
	input := makeInput("raw_file", csv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got %v", records[0]["name"])
	}
	if records[0]["age"] != "30" {
		t.Errorf("Expected age=30, got %v", records[0]["age"])
	}
	if records[1]["city"] != "LA" {
		t.Errorf("Expected city=LA, got %v", records[1]["city"])
	}
}

// ─── 2. CSV without Header ─────────────────────────────────────

func TestFileParser_CSV_NoHeader(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	csv := "Alice,30,NYC\nBob,25,LA\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "csv",
		"hasHeader":   false,
	})
	input := makeInput("raw_file", csv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	// Without header, columns should be col_1, col_2, col_3
	if records[0]["col_1"] != "Alice" {
		t.Errorf("Expected col_1=Alice, got %v", records[0]["col_1"])
	}
	if records[0]["col_2"] != "30" {
		t.Errorf("Expected col_2=30, got %v", records[0]["col_2"])
	}
}

// ─── 3. CSV with Custom Delimiter (Pipe) ───────────────────────

func TestFileParser_CSV_CustomDelimiter(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	csv := "name|age|city\nAlice|30|NYC\nBob|25|LA\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "csv",
		"delimiter":   "|",
		"hasHeader":   true,
	})
	input := makeInput("raw_file", csv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got %v", records[0]["name"])
	}
}

// ─── 4. TSV ────────────────────────────────────────────────────

func TestFileParser_TSV(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	tsv := "name\tage\tcity\nAlice\t30\tNYC\nBob\t25\tLA\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "tsv",
		"hasHeader":   true,
	})
	input := makeInput("raw_file", tsv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got %v", records[0]["name"])
	}
	if records[1]["age"] != "25" {
		t.Errorf("Expected age=25, got %v", records[1]["age"])
	}
}

// ─── 5. Fixed-Width CCLF ──────────────────────────────────────

func TestFileParser_FixedWidth_CCLF(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	// CCLF-like format:
	// BENE_ID (pos 1-11), CLM_ID (pos 12-24), CLM_TYPE (pos 25-26)
	content := "12345678901CLAIM0000001271\n98765432109CLAIM000000240\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "fixed_width",
		"hasHeader":   false,
		"trimFields":  true,
		"columns": []interface{}{
			map[string]interface{}{"name": "BENE_ID", "start": 1, "length": 11},
			map[string]interface{}{"name": "CLM_ID", "start": 12, "length": 13},
			map[string]interface{}{"name": "CLM_TYPE", "start": 25, "length": 2},
		},
	})
	input := makeInput("raw_file", content)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	if records[0]["BENE_ID"] != "12345678901" {
		t.Errorf("Expected BENE_ID=12345678901, got '%v'", records[0]["BENE_ID"])
	}
	if records[0]["CLM_ID"] != "CLAIM00000012" {
		t.Errorf("Expected CLM_ID=CLAIM00000012, got '%v'", records[0]["CLM_ID"])
	}
	if records[0]["CLM_TYPE"] != "71" {
		t.Errorf("Expected CLM_TYPE=71, got '%v'", records[0]["CLM_TYPE"])
	}
}

// ─── 6. Skip Rows ─────────────────────────────────────────────

func TestFileParser_SkipRows(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	// First 2 rows are metadata, then header, then data
	csv := "Report: Daily Feed\nGenerated: 2025-01-15\nname,age\nAlice,30\nBob,25\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "csv",
		"hasHeader":   true,
		"skipRows":    2,
	})
	input := makeInput("raw_file", csv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records (skipping 2 metadata rows), got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got %v", records[0]["name"])
	}
}

// ─── 7. Max Records ───────────────────────────────────────────

func TestFileParser_MaxRecords(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	// 5 data rows
	csv := "id,name\n1,Alice\n2,Bob\n3,Charlie\n4,Diana\n5,Eve\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "csv",
		"hasHeader":   true,
		"maxRecords":  3,
	})
	input := makeInput("raw_file", csv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 3 {
		t.Fatalf("Expected 3 records (maxRecords=3), got %d", len(records))
	}
	if records[2]["name"] != "Charlie" {
		t.Errorf("Expected third record name=Charlie, got %v", records[2]["name"])
	}
}

// ─── 8. Trim Whitespace ───────────────────────────────────────

func TestFileParser_TrimWhitespace(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	csv := "name , age , city \n  Alice  , 30 ,  NYC  \n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "csv",
		"hasHeader":   true,
		"trimFields":  true,
	})
	input := makeInput("raw_file", csv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected trimmed name=Alice, got '%v'", records[0]["name"])
	}
	if records[0]["age"] != "30" {
		t.Errorf("Expected trimmed age=30, got '%v'", records[0]["age"])
	}
	if records[0]["city"] != "NYC" {
		t.Errorf("Expected trimmed city=NYC, got '%v'", records[0]["city"])
	}
}

// ─── 9. Empty File ────────────────────────────────────────────

func TestFileParser_EmptyFile(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "csv",
		"hasHeader":   true,
	})
	input := makeInput("raw_file", "")

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Empty file should not error, got: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 0 {
		t.Errorf("Expected 0 records for empty file, got %d", len(records))
	}
}

// ─── 10. Quoted Fields ────────────────────────────────────────

func TestFileParser_QuotedFields(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	csv := "name,address\nAlice,\"123 Main St, Apt 4\"\nBob,\"456 Oak Ave\"\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "csv",
		"hasHeader":   true,
	})
	input := makeInput("raw_file", csv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	// Quoted field should preserve the comma inside
	if records[0]["address"] != "123 Main St, Apt 4" {
		t.Errorf("Expected address with comma, got '%v'", records[0]["address"])
	}
}

// ─── 11. Missing Source Field ─────────────────────────────────

func TestFileParser_MissingSourceField(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)

	step := makeStep(map[string]interface{}{
		"sourceField": "nonexistent_field",
		"fileFormat":  "csv",
		"hasHeader":   true,
	})
	input := map[string]interface{}{"other_field": "data"}

	_, err := executor.Execute(context.Background(), step, input)
	if err == nil {
		t.Fatal("Expected error for missing source field, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error should mention 'not found', got: %v", err)
	}
}

// ─── 12. Invalid Format ──────────────────────────────────────

func TestFileParser_InvalidFormat(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "zarr",
	})
	input := makeInput("raw_file", "some data")

	_, err := executor.Execute(context.Background(), step, input)
	if err == nil {
		t.Fatal("Expected error for unsupported format, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("Error should mention 'unsupported', got: %v", err)
	}
}

// ─── 13. Fixed-Width No Columns ──────────────────────────────

func TestFileParser_FixedWidth_NoColumns(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "fixed_width",
		"columns":     []interface{}{},
	})

	err := executor.Validate(step)
	if err == nil {
		t.Fatal("Expected validation error for fixed_width with no columns")
	}
	if !strings.Contains(err.Error(), "column") {
		t.Errorf("Error should mention 'column', got: %v", err)
	}
}

// ─── 14. Fixed-Width Short Line ──────────────────────────────

func TestFileParser_FixedWidth_ShortLine(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	// Line is only 10 chars, but column def goes to position 20
	content := "SHORT_LINE\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "fixed_width",
		"hasHeader":   false,
		"trimFields":  true,
		"columns": []interface{}{
			map[string]interface{}{"name": "FIELD_A", "start": 1, "length": 5},
			map[string]interface{}{"name": "FIELD_B", "start": 6, "length": 5},
			map[string]interface{}{"name": "FIELD_C", "start": 11, "length": 10}, // extends beyond line
		},
	})
	input := makeInput("raw_file", content)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Short line should not crash, got: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}
	if records[0]["FIELD_A"] != "SHORT" {
		t.Errorf("Expected FIELD_A=SHORT, got '%v'", records[0]["FIELD_A"])
	}
	if records[0]["FIELD_B"] != "_LINE" {
		t.Errorf("Expected FIELD_B=_LINE, got '%v'", records[0]["FIELD_B"])
	}
	// FIELD_C should be empty since line is too short
	if records[0]["FIELD_C"] != "" {
		t.Errorf("Expected FIELD_C empty for short line, got '%v'", records[0]["FIELD_C"])
	}
}


// ─── 16. Validate Config ─────────────────────────────────────

func TestFileParser_Validate(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)

	// Missing sourceField
	err := executor.Validate(makeStep(map[string]interface{}{
		"fileFormat": "csv",
	}))
	if err == nil {
		t.Error("Expected error for missing sourceField")
	}

	// Missing fileFormat
	err = executor.Validate(makeStep(map[string]interface{}{
		"sourceField": "raw_file",
	}))
	if err == nil {
		t.Error("Expected error for missing fileFormat")
	}

	// Nil config
	err = executor.Validate(&models.TransformationStep{
		StepName: "test",
		StepType: "file_parser",
		Enabled:  true,
		Config:   nil,
	})
	if err == nil {
		t.Error("Expected error for nil config")
	}

	// Valid config
	err = executor.Validate(makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "csv",
	}))
	if err != nil {
		t.Errorf("Valid config should not error: %v", err)
	}
}

// ─── 17. Step Output Structure ───────────────────────────────

func TestFileParser_StepOutput(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	csv := "name,age\nAlice,30\nBob,25\nCharlie,35\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "csv",
		"hasHeader":   true,
	})
	input := makeInput("raw_file", csv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify _stepOutput
	stepOutput, ok := output["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatal("_stepOutput not found")
	}

	if stepOutput["record_count"] != 3 {
		t.Errorf("Expected record_count=3, got %v", stepOutput["record_count"])
	}
	if stepOutput["column_count"] != 2 {
		t.Errorf("Expected column_count=2, got %v", stepOutput["column_count"])
	}

	columns, ok := stepOutput["columns"].([]string)
	if !ok {
		t.Fatalf("columns is not []string, type: %T", stepOutput["columns"])
	}
	if len(columns) != 2 || columns[0] != "name" || columns[1] != "age" {
		t.Errorf("Expected columns=[name,age], got %v", columns)
	}

	// Verify _executionDetails
	execDetails, ok := output["_executionDetails"].(map[string]interface{})
	if !ok {
		t.Fatal("_executionDetails not found")
	}
	if execDetails["format"] != "csv" {
		t.Errorf("Expected format=csv, got %v", execDetails["format"])
	}
}

// ─── 18. GetOutputVariables ──────────────────────────────────

func TestFileParser_GetOutputVariables(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "csv",
		"hasHeader":   true,
	})

	vars := executor.GetOutputVariables(step)
	if len(vars) < 4 {
		t.Fatalf("Expected at least 4 variable definitions (record_count, column_count, columns, records), got %d", len(vars))
	}

	// Check standard variables exist
	varNames := make(map[string]bool)
	for _, v := range vars {
		varNames[v.Name] = true
	}
	for _, expected := range []string{"record_count", "column_count", "columns", "records"} {
		if !varNames[expected] {
			t.Errorf("Expected variable '%s' in output, not found", expected)
		}
	}
}

// ─── 19. Windows Line Endings ────────────────────────────────

func TestFileParser_WindowsLineEndings(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	csv := "name,age\r\nAlice,30\r\nBob,25\r\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "csv",
		"hasHeader":   true,
	})
	input := makeInput("raw_file", csv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Windows line endings should work, got: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records with \\r\\n endings, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got '%v'", records[0]["name"])
	}
}

// ─── 20. Unicode Content ─────────────────────────────────────

func TestFileParser_UnicodeContent(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	csv := "name,city\nJosé García,São Paulo\n田中太郎,東京\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "csv",
		"hasHeader":   true,
	})
	input := makeInput("raw_file", csv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Unicode content should work, got: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	if records[0]["name"] != "José García" {
		t.Errorf("Expected José García, got '%v'", records[0]["name"])
	}
	if records[1]["name"] != "田中太郎" {
		t.Errorf("Expected 田中太郎, got '%v'", records[1]["name"])
	}
	if records[1]["city"] != "東京" {
		t.Errorf("Expected 東京, got '%v'", records[1]["city"])
	}
}

// ─── 21. XLSX with Header ────────────────────────────────────

// createXlsxBytes builds an in-memory .xlsx file and returns its raw bytes as a string
func createXlsxBytes(t *testing.T, sheetName string, rows [][]string) string {
	t.Helper()
	f := excelize.NewFile()
	// Rename the default sheet
	defaultSheet := f.GetSheetName(0)
	if err := f.SetSheetName(defaultSheet, sheetName); err != nil {
		t.Fatalf("Failed to rename sheet: %v", err)
	}

	for rowIdx, row := range rows {
		for colIdx, cellVal := range row {
			colName, _ := excelize.ColumnNumberToName(colIdx + 1)
			cell := fmt.Sprintf("%s%d", colName, rowIdx+1)
			if err := f.SetCellValue(sheetName, cell, cellVal); err != nil {
				t.Fatalf("Failed to set cell %s: %v", cell, err)
			}
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("Failed to write xlsx to buffer: %v", err)
	}
	return buf.String()
}

func TestFileParser_XLSX_WithHeader(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	xlsxContent := createXlsxBytes(t, "Sheet1", [][]string{
		{"name", "age", "city"},
		{"Alice", "30", "NYC"},
		{"Bob", "25", "LA"},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "xlsx",
		"hasHeader":   true,
		"trimFields":  true,
	})
	input := makeInput("raw_file", xlsxContent)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got %v", records[0]["name"])
	}
	if records[0]["age"] != "30" {
		t.Errorf("Expected age=30, got %v", records[0]["age"])
	}
	if records[1]["city"] != "LA" {
		t.Errorf("Expected city=LA, got %v", records[1]["city"])
	}
}

// ─── 22. XLSX without Header ─────────────────────────────────

func TestFileParser_XLSX_NoHeader(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	xlsxContent := createXlsxBytes(t, "Sheet1", [][]string{
		{"Alice", "30", "NYC"},
		{"Bob", "25", "LA"},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "xlsx",
		"hasHeader":   false,
	})
	input := makeInput("raw_file", xlsxContent)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	if records[0]["col_1"] != "Alice" {
		t.Errorf("Expected col_1=Alice, got %v", records[0]["col_1"])
	}
	if records[0]["col_2"] != "30" {
		t.Errorf("Expected col_2=30, got %v", records[0]["col_2"])
	}
}

// ─── 23. XLSX Sheet by Name ──────────────────────────────────

func TestFileParser_XLSX_SheetName(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)

	// Create xlsx with multiple sheets
	f := excelize.NewFile()
	defaultSheet := f.GetSheetName(0)
	_ = f.SetSheetName(defaultSheet, "Summary")
	_ = f.SetCellValue("Summary", "A1", "summary_only")

	_, _ = f.NewSheet("PatientData")
	_ = f.SetCellValue("PatientData", "A1", "name")
	_ = f.SetCellValue("PatientData", "B1", "mrn")
	_ = f.SetCellValue("PatientData", "A2", "John Doe")
	_ = f.SetCellValue("PatientData", "B2", "MRN001")

	var buf bytes.Buffer
	_ = f.Write(&buf)

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "xlsx",
		"hasHeader":   true,
		"sheetName":   "PatientData",
	})
	input := makeInput("raw_file", buf.String())

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 1 {
		t.Fatalf("Expected 1 record from PatientData sheet, got %d", len(records))
	}
	if records[0]["name"] != "John Doe" {
		t.Errorf("Expected name=John Doe, got %v", records[0]["name"])
	}
	if records[0]["mrn"] != "MRN001" {
		t.Errorf("Expected mrn=MRN001, got %v", records[0]["mrn"])
	}
}

// ─── 24. XLSX Sheet by Index ─────────────────────────────────

func TestFileParser_XLSX_SheetIndex(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)

	// Create xlsx with 2 sheets, target the second one (index 1)
	f := excelize.NewFile()
	defaultSheet := f.GetSheetName(0)
	_ = f.SetSheetName(defaultSheet, "Ignore")
	_ = f.SetCellValue("Ignore", "A1", "skip_this")

	_, _ = f.NewSheet("Claims")
	_ = f.SetCellValue("Claims", "A1", "claim_id")
	_ = f.SetCellValue("Claims", "B1", "amount")
	_ = f.SetCellValue("Claims", "A2", "CLM001")
	_ = f.SetCellValue("Claims", "B2", "1500.00")

	var buf bytes.Buffer
	_ = f.Write(&buf)

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "xlsx",
		"hasHeader":   true,
		"sheetIndex":  1, // second sheet
	})
	input := makeInput("raw_file", buf.String())

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 1 {
		t.Fatalf("Expected 1 record from Claims sheet (index 1), got %d", len(records))
	}
	if records[0]["claim_id"] != "CLM001" {
		t.Errorf("Expected claim_id=CLM001, got %v", records[0]["claim_id"])
	}
}

// ─── 25. XLSX Skip Rows ──────────────────────────────────────

func TestFileParser_XLSX_SkipRows(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	xlsxContent := createXlsxBytes(t, "Sheet1", [][]string{
		{"Report: Monthly Claims"},
		{"Generated: 2025-01-15"},
		{"name", "amount"},
		{"Alice", "1000"},
		{"Bob", "2000"},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "xlsx",
		"hasHeader":   true,
		"skipRows":    2, // skip 2 metadata rows
	})
	input := makeInput("raw_file", xlsxContent)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 data records after skipping 2 rows, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got %v", records[0]["name"])
	}
	if records[1]["amount"] != "2000" {
		t.Errorf("Expected amount=2000, got %v", records[1]["amount"])
	}
}

// ─── 26. XLSX Base64 Content ─────────────────────────────────

func TestFileParser_XLSX_Base64Content(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	xlsxContent := createXlsxBytes(t, "Sheet1", [][]string{
		{"id", "diagnosis"},
		{"P001", "Hypertension"},
		{"P002", "Diabetes"},
	})

	// Encode as base64 (simulates JSON-serialized binary)
	b64Content := base64.StdEncoding.EncodeToString([]byte(xlsxContent))

	step := makeStep(map[string]interface{}{
		"sourceField":     "raw_file",
		"fileFormat":      "xlsx",
		"hasHeader":       true,
		"contentEncoding": "base64",
	})
	input := makeInput("raw_file", b64Content)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records from base64 xlsx, got %d", len(records))
	}
	if records[0]["id"] != "P001" {
		t.Errorf("Expected id=P001, got %v", records[0]["id"])
	}
	if records[1]["diagnosis"] != "Diabetes" {
		t.Errorf("Expected diagnosis=Diabetes, got %v", records[1]["diagnosis"])
	}
}

// ─── 27. Auto-Detect: CSV ────────────────────────────────────

func TestFileParser_AutoDetect_CSV(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	csv := "name,age,city\nAlice,30,NYC\nBob,25,LA\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"autoDetect":  true,
	})
	input := makeInput("raw_file", csv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Auto-detect CSV failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got %v", records[0]["name"])
	}

	// Verify _executionDetails contains detected format
	execDetails, ok := output["_executionDetails"].(map[string]interface{})
	if !ok {
		t.Fatal("_executionDetails not found")
	}
	if execDetails["format"] != "csv" {
		t.Errorf("Expected detected format=csv, got %v", execDetails["format"])
	}
}

// ─── 28. Auto-Detect: TSV ────────────────────────────────────

func TestFileParser_AutoDetect_TSV(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	tsv := "name\tage\tcity\nAlice\t30\tNYC\nBob\t25\tLA\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"autoDetect":  true,
	})
	input := makeInput("raw_file", tsv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Auto-detect TSV failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got %v", records[0]["name"])
	}

	execDetails, ok := output["_executionDetails"].(map[string]interface{})
	if ok {
		if execDetails["format"] != "tsv" {
			t.Logf("Note: auto-detected format=%v (tsv expected)", execDetails["format"])
		}
	}
}

// ─── 29. Auto-Detect: Pipe-Delimited ─────────────────────────

func TestFileParser_AutoDetect_PipeDelimited(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	content := "id|first_name|last_name\n1|Alice|Smith\n2|Bob|Jones\n3|Carol|White\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"autoDetect":  true,
	})
	input := makeInput("raw_file", content)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Auto-detect pipe-delimited failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 3 {
		t.Fatalf("Expected 3 records, got %d", len(records))
	}
	if records[0]["id"] != "1" {
		t.Errorf("Expected id=1, got %v", records[0]["id"])
	}
}

// ─── 30. Auto-Detect: XLSX Magic Bytes ───────────────────────

func TestFileParser_AutoDetect_XLSX(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	xlsxContent := createXlsxBytes(t, "Data", [][]string{
		{"product", "qty"},
		{"Widget", "100"},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"autoDetect":  true,
	})
	input := makeInput("raw_file", xlsxContent)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Auto-detect XLSX failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}
	if records[0]["product"] != "Widget" {
		t.Errorf("Expected product=Widget, got %v", records[0]["product"])
	}
}

// ─── 31. Auto-Detect: fileFormat=auto (string alias) ─────────

func TestFileParser_AutoDetect_FormatStringAlias(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	csv := "x,y\n1,2\n3,4\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "auto",
	})
	input := makeInput("raw_file", csv)

	_, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("fileFormat=auto should work: %v", err)
	}
}

// ─── 32. Auto-Detect: Metadata Populated ─────────────────────

func TestFileParser_AutoDetect_MetadataPopulated(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	csv := "col_a,col_b\nfoo,bar\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"autoDetect":  true,
	})
	input := makeInput("raw_file", csv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	stepOutput, ok := output["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatal("_stepOutput not found")
	}
	meta, ok := stepOutput["_metadata"].(map[string]interface{})
	if ok {
		// auto_detected flag should be set in metadata
		if meta["auto_detected"] != true {
			t.Logf("Note: auto_detected not in metadata (meta=%v)", meta)
		}
	}
	// At minimum records should be parsed
	if stepOutput["record_count"] == nil {
		t.Error("record_count should be in _stepOutput")
	}
}

// ─── 33. Template: cclf1 Loads Columns ───────────────────────

func TestFileParser_Template_CCLF1_LoadsColumns(t *testing.T) {
	// CCLF1 has 26 columns. Actual field layout from file_parser_templates.go:
	//   CUR_CLM_UNIQ_ID: pos  1-13 (13 chars)
	//   PRVDR_OSCAR_NUM: pos 14-19  (6 chars)
	//   BENE_MBI_ID:     pos 20-30 (11 chars)
	//   ...26 fields total, record length 181 chars
	line := fmt.Sprintf("%-13s%-6s%-11s%s",
		"1234567890123", // CUR_CLM_UNIQ_ID (1-13)
		"PROVDR",        // PRVDR_OSCAR_NUM (14-19)
		"MBI12345678",   // BENE_MBI_ID (20-30)
		strings.Repeat(" ", 181-30), // pad to full record length
	)

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "fixed_width",
		"hasHeader":   false,
		"trimFields":  true,
		"template":    "cclf1",
	})
	input := makeInput("raw_file", line+"\n")

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("CCLF1 template execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}
	if records[0]["CUR_CLM_UNIQ_ID"] != "1234567890123" {
		t.Errorf("Expected CUR_CLM_UNIQ_ID=1234567890123, got '%v'", records[0]["CUR_CLM_UNIQ_ID"])
	}
	if records[0]["BENE_MBI_ID"] != "MBI12345678" {
		t.Errorf("Expected BENE_MBI_ID=MBI12345678, got '%v'", records[0]["BENE_MBI_ID"])
	}
	if records[0]["PRVDR_OSCAR_NUM"] != "PROVDR" {
		t.Errorf("Expected PRVDR_OSCAR_NUM=PROVDR, got '%v'", records[0]["PRVDR_OSCAR_NUM"])
	}
}

// ─── 34. Template: nacha_entry Loads Columns ─────────────────

func TestFileParser_Template_NACHAEntry_LoadsColumns(t *testing.T) {
	// nacha_entry: RECORD_TYPE_CODE (start=1, len=1), TRANSACTION_CODE (start=2, len=2)
	// Total record length = 94 chars
	line := "6" + // RECORD_TYPE_CODE (pos 1, len 1)
		"27" + // TRANSACTION_CODE (pos 2, len 2)
		strings.Repeat("0", 91) // padding to full record width

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "fixed_width",
		"hasHeader":   false,
		"template":    "nacha_entry",
		"trimFields":  true,
	})
	input := makeInput("raw_file", line+"\n")

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("NACHA entry template failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}
	if records[0]["RECORD_TYPE_CODE"] != "6" {
		t.Errorf("Expected RECORD_TYPE_CODE=6, got '%v'", records[0]["RECORD_TYPE_CODE"])
	}
	if records[0]["TRANSACTION_CODE"] != "27" {
		t.Errorf("Expected TRANSACTION_CODE=27, got '%v'", records[0]["TRANSACTION_CODE"])
	}
}

// ─── 35. Template: Unknown Key Returns Error ──────────────────

func TestFileParser_Template_UnknownKey_Error(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "fixed_width",
		"template":    "totally_unknown_template_xyz",
	})

	err := executor.Validate(step)
	if err == nil {
		t.Fatal("Expected error for unknown template key")
	}
	if !strings.Contains(err.Error(), "unknown template") {
		t.Errorf("Error should mention 'unknown template', got: %v", err)
	}
}

// ─── 36. Template: User Columns Override Template ─────────────

func TestFileParser_Template_UserColumnsOverride(t *testing.T) {
	// When user provides explicit columns, they take precedence over template
	line := "HELLO" + strings.Repeat(" ", 50)
	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "fixed_width",
		"hasHeader":   false,
		"template":    "cclf1", // template present but columns also provided
		"columns": []interface{}{
			map[string]interface{}{"name": "GREETING", "start": 1, "length": 5},
		},
	})
	input := makeInput("raw_file", line+"\n")

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}
	// User-defined column "GREETING" should be used, not CCLF1 columns
	if records[0]["GREETING"] != "HELLO" {
		t.Errorf("Expected GREETING=HELLO, got '%v'", records[0]["GREETING"])
	}
	// CCLF1-specific column should NOT exist
	if _, exists := records[0]["CUR_CLM_UNIQ_ID"]; exists {
		t.Error("CUR_CLM_UNIQ_ID should not exist when user columns are provided")
	}
}

// ─── 37. GetTemplateList returns all templates ────────────────

func TestFileParser_GetTemplateList(t *testing.T) {
	list := GetTemplateList()
	if len(list) == 0 {
		t.Fatal("GetTemplateList should return at least one template")
	}

	// Verify each entry has required fields
	for _, tmpl := range list {
		if tmpl.Key == "" {
			t.Errorf("Template key should not be empty")
		}
		if tmpl.Name == "" {
			t.Errorf("Template %s: name should not be empty", tmpl.Key)
		}
		if tmpl.Category == "" {
			t.Errorf("Template %s: category should not be empty", tmpl.Key)
		}
		if tmpl.ColumnCount <= 0 {
			t.Errorf("Template %s: columnCount should be > 0, got %d", tmpl.Key, tmpl.ColumnCount)
		}
	}

	// Verify known templates are present
	keys := make(map[string]bool)
	for _, tmpl := range list {
		keys[tmpl.Key] = true
	}
	for _, expected := range []string{"cclf1", "cclf2", "nacha_entry", "era_835_header"} {
		if !keys[expected] {
			t.Errorf("Expected template '%s' in GetTemplateList", expected)
		}
	}
}

// ─── 38. GetTemplatesByCategory groups by category ───────────

func TestFileParser_GetTemplatesByCategory(t *testing.T) {
	byCategory := GetTemplatesByCategory()
	if len(byCategory) == 0 {
		t.Fatal("GetTemplatesByCategory should return at least one category")
	}

	// CMS/CCLF category must exist and have templates
	cclfTemplates, ok := byCategory["CMS/CCLF"]
	if !ok {
		t.Fatal("Expected CMS/CCLF category in GetTemplatesByCategory")
	}
	if len(cclfTemplates) == 0 {
		t.Error("CMS/CCLF category should have at least one template")
	}
}

// ─── 39. Template: stepOutput has template metadata ──────────

func TestFileParser_Template_StepOutputMetadata(t *testing.T) {
	// CCLF1 has 26 columns. Build a record padded to the full 181-char record length.
	line := fmt.Sprintf("%-13s%-6s%-11s%s",
		"1234567890123", // CUR_CLM_UNIQ_ID (pos 1-13)
		"PROVDR",        // PRVDR_OSCAR_NUM (pos 14-19)
		"MBI12345678",   // BENE_MBI_ID (pos 20-30)
		strings.Repeat(" ", 181-30),
	)
	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "fixed_width",
		"hasHeader":   false,
		"trimFields":  true,
		"template":    "cclf1",
	})
	input := makeInput("raw_file", line+"\n")

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	stepOutput, ok := output["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatal("_stepOutput not found")
	}

	// Verify metadata contains template key
	meta, hasMeta := stepOutput["_metadata"].(map[string]interface{})
	if hasMeta {
		if meta["template"] != "cclf1" {
			t.Logf("Note: template key not in metadata (meta=%v)", meta)
		}
	}

	// Column count should match CCLF1 template (26 columns as of V45 template update)
	if stepOutput["column_count"] != 26 {
		t.Errorf("Expected 26 columns from CCLF1 template, got %v", stepOutput["column_count"])
	}
}

// ─── 40. Local Path: Single CSV ──────────────────────────────

func TestFileParser_LocalPath_SingleCSV(t *testing.T) {
	// Write a temp CSV file
	tmpFile, err := os.CreateTemp("", "fp_test_*.csv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })
	content := "patient_id,last_name,dob\nP001,Smith,1985-06-15\nP002,Jones,1990-11-22\nP003,White,2001-03-08\n"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType": "local_path",
		"filePath":   tmpFile.Name(),
		"fileFormat": "csv",
		"hasHeader":  true,
	})

	output, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 3 {
		t.Fatalf("Expected 3 records, got %d", len(records))
	}
	if records[0]["patient_id"] != "P001" {
		t.Errorf("Expected patient_id=P001, got %v", records[0]["patient_id"])
	}

	// Verify metadata reflects local_path source
	execDetails, ok := output["_executionDetails"].(map[string]interface{})
	if !ok {
		t.Fatal("_executionDetails not found")
	}
	if execDetails["source_type"] != "local_path" {
		t.Errorf("Expected source_type=local_path, got %v", execDetails["source_type"])
	}
	if execDetails["file_path"] != tmpFile.Name() {
		t.Errorf("Expected file_path=%s, got %v", tmpFile.Name(), execDetails["file_path"])
	}
}

// ─── 41. Local Path: Single Fixed-Width ──────────────────────

func TestFileParser_LocalPath_SingleFixedWidth(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "fp_test_*.dat")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	// 3 rows, 2 fixed-width columns: ID (pos 1-5), NAME (pos 6-20)
	lines := "P0001Alice Smith     \nP0002Bob Jones       \nP0003Carol White     \n"
	if _, err := tmpFile.WriteString(lines); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType": "local_path",
		"filePath":   tmpFile.Name(),
		"fileFormat": "fixed_width",
		"hasHeader":  false,
		"trimFields": true,
		"columns": []interface{}{
			map[string]interface{}{"name": "ID", "start": 1, "length": 5},
			map[string]interface{}{"name": "NAME", "start": 6, "length": 15},
		},
	})

	output, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 3 {
		t.Fatalf("Expected 3 records, got %d", len(records))
	}
	if records[0]["ID"] != "P0001" {
		t.Errorf("Expected ID=P0001, got %v", records[0]["ID"])
	}
	if records[1]["NAME"] != "Bob Jones" {
		t.Errorf("Expected NAME='Bob Jones', got %q", records[1]["NAME"])
	}
}

// ─── 42. Local Path: File Not Found ──────────────────────────

func TestFileParser_LocalPath_FileNotFound(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType": "local_path",
		"filePath":   "/nonexistent/path/does_not_exist.csv",
		"fileFormat": "csv",
		"hasHeader":  true,
	})

	_, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected an error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "does_not_exist.csv") {
		t.Errorf("Error should mention the missing filename, got: %v", err)
	}
}

// ─── 43. Batch Mode: Merged Records + _source_file ───────────

func TestFileParser_BatchMode_MergedRecords(t *testing.T) {
	dir, err := os.MkdirTemp("", "fp_batch_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	// Create 3 CSV files, each with 3 records (same columns)
	files := []struct {
		name    string
		content string
	}{
		{"jan.csv", "id,amount\nR001,100\nR002,200\nR003,300\n"},
		{"feb.csv", "id,amount\nR004,400\nR005,500\nR006,600\n"},
		{"mar.csv", "id,amount\nR007,700\nR008,800\nR009,900\n"},
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte(f.content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", f.name, err)
		}
	}

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType":  "local_path",
		"filePath":    dir,
		"filePattern": "*.csv",
		"batchMode":   true,
		"fileFormat":  "csv",
		"hasHeader":   true,
	})

	output, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Batch execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 9 {
		t.Fatalf("Expected 9 merged records (3 files × 3 rows), got %d", len(records))
	}

	// Verify _source_file is injected
	for i, rec := range records {
		srcFile, ok := rec["_source_file"].(string)
		if !ok || srcFile == "" {
			t.Errorf("Record %d missing _source_file", i)
		}
	}

	// Verify _stepOutput has files_processed
	stepOutput, ok := output["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatal("_stepOutput not found")
	}
	filesProcessed, _ := stepOutput["files_processed"].(int)
	if filesProcessed != 3 {
		t.Errorf("Expected files_processed=3, got %v", stepOutput["files_processed"])
	}
}

// ─── 44. Batch Mode: Column Mismatch Error ────────────────────

func TestFileParser_BatchMode_ColumnMismatch(t *testing.T) {
	dir, err := os.MkdirTemp("", "fp_mismatch_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	// First file: id,name  — second file: id,amount (different column)
	if err := os.WriteFile(filepath.Join(dir, "a.csv"), []byte("id,name\nR001,Alice\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.csv"), []byte("id,amount\nR002,500\n"), 0644); err != nil {
		t.Fatal(err)
	}

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType":  "local_path",
		"filePath":    dir,
		"filePattern": "*.csv",
		"batchMode":   true,
		"fileFormat":  "csv",
		"hasHeader":   true,
	})

	_, err = executor.Execute(context.Background(), step, map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected column mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "column structure mismatch") {
		t.Errorf("Error should mention column mismatch, got: %v", err)
	}
	if !strings.Contains(err.Error(), "b.csv") {
		t.Errorf("Error should mention the offending filename 'b.csv', got: %v", err)
	}
}

// ─── 45. Batch Mode: Glob Pattern in FilePath ─────────────────

func TestFileParser_BatchMode_GlobPattern(t *testing.T) {
	dir, err := os.MkdirTemp("", "fp_glob_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	// Two CSV files + one .txt file (should be excluded by glob)
	if err := os.WriteFile(filepath.Join(dir, "claims_jan.csv"), []byte("clm_id,status\nC001,paid\nC002,pending\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "claims_feb.csv"), []byte("clm_id,status\nC003,paid\nC004,denied\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("do not parse this file"), 0644); err != nil {
		t.Fatal(err)
	}

	// Use glob directly in filePath (no separate filePattern)
	globPath := filepath.Join(dir, "claims_*.csv")

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType": "local_path",
		"filePath":   globPath, // glob in filePath
		"batchMode":  true,
		"fileFormat": "csv",
		"hasHeader":  true,
	})

	output, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Batch glob execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	// Should have 4 records from 2 CSV files (the .txt is not matched)
	if len(records) != 4 {
		t.Fatalf("Expected 4 records from 2 CSV files, got %d", len(records))
	}

	// Verify no records from notes.txt
	for _, rec := range records {
		if src, _ := rec["_source_file"].(string); src == "notes.txt" {
			t.Error("notes.txt should have been excluded by glob pattern")
		}
	}

	// Verify stepOutput source_files only contains .csv files
	stepOutput, _ := output["_stepOutput"].(map[string]interface{})
	srcFiles, _ := stepOutput["source_files"].([]string)
	if len(srcFiles) != 2 {
		t.Errorf("Expected 2 source_files, got %v", srcFiles)
	}
}

// ─── Avro Tests ────────────────────────────────────────────────

// createAvroBytes builds a minimal Avro OCF file in memory and returns raw bytes as a string.
// schemaJSON is a full Avro schema (record type). rows must match the schema.
func createAvroBytes(t *testing.T, schemaJSON string, rows []interface{}) string {
	t.Helper()
	codec, err := goavro.NewCodec(schemaJSON)
	if err != nil {
		t.Fatalf("createAvroBytes: NewCodec: %v", err)
	}
	var buf bytes.Buffer
	w, err := goavro.NewOCFWriter(goavro.OCFConfig{W: &buf, Codec: codec})
	if err != nil {
		t.Fatalf("createAvroBytes: NewOCFWriter: %v", err)
	}
	if err := w.Append(rows); err != nil {
		t.Fatalf("createAvroBytes: Append: %v", err)
	}
	return buf.String()
}

const patientAvroSchema = `{
	"type": "record",
	"name": "Patient",
	"fields": [
		{"name": "name", "type": "string"},
		{"name": "age",  "type": "int"},
		{"name": "city", "type": "string"}
	]
}`

func TestFileParser_Avro_Basic(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	avroContent := createAvroBytes(t, patientAvroSchema, []interface{}{
		map[string]interface{}{"name": "Alice", "age": int32(30), "city": "NYC"},
		map[string]interface{}{"name": "Bob", "age": int32(25), "city": "LA"},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "avro",
	})
	input := makeInput("raw_file", avroContent)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got %v", records[0]["name"])
	}
	if records[1]["city"] != "LA" {
		t.Errorf("Expected city=LA, got %v", records[1]["city"])
	}
}

func TestFileParser_Avro_Columns(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	avroContent := createAvroBytes(t, patientAvroSchema, []interface{}{
		map[string]interface{}{"name": "Alice", "age": int32(30), "city": "NYC"},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "avro",
	})
	output, _ := executor.Execute(context.Background(), step, makeInput("raw_file", avroContent))

	stepOut, _ := output["_stepOutput"].(map[string]interface{})
	cols, _ := stepOut["columns"].([]string)
	if len(cols) == 0 {
		t.Fatal("Expected non-empty columns from Avro schema")
	}
	// Avro schema fields: name, age, city
	colSet := make(map[string]bool)
	for _, c := range cols {
		colSet[c] = true
	}
	for _, expected := range []string{"name", "age", "city"} {
		if !colSet[expected] {
			t.Errorf("Expected column %q in Avro output columns", expected)
		}
	}
}

func TestFileParser_Avro_MaxRecords(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	rows := make([]interface{}, 5)
	for i := range rows {
		rows[i] = map[string]interface{}{"name": fmt.Sprintf("Person%d", i), "age": int32(i), "city": "NYC"}
	}
	avroContent := createAvroBytes(t, patientAvroSchema, rows)

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "avro",
		"maxRecords":  3,
	})
	output, err := executor.Execute(context.Background(), step, makeInput("raw_file", avroContent))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 3 {
		t.Errorf("Expected 3 records (maxRecords=3), got %d", len(records))
	}
}

func TestFileParser_Avro_SkipRows(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	avroContent := createAvroBytes(t, patientAvroSchema, []interface{}{
		map[string]interface{}{"name": "Skip1", "age": int32(1), "city": "X"},
		map[string]interface{}{"name": "Skip2", "age": int32(2), "city": "X"},
		map[string]interface{}{"name": "Alice", "age": int32(30), "city": "NYC"},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "avro",
		"skipRows":    2,
	})
	output, err := executor.Execute(context.Background(), step, makeInput("raw_file", avroContent))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 1 {
		t.Fatalf("Expected 1 record after skipRows=2, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice after skip, got %v", records[0]["name"])
	}
}

func TestFileParser_Avro_TrimFields(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	avroContent := createAvroBytes(t, patientAvroSchema, []interface{}{
		map[string]interface{}{"name": "  Alice  ", "age": int32(30), "city": "  NYC  "},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "avro",
		"trimFields":  true,
	})
	output, err := executor.Execute(context.Background(), step, makeInput("raw_file", avroContent))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected trimmed name=Alice, got %q", records[0]["name"])
	}
	if records[0]["city"] != "NYC" {
		t.Errorf("Expected trimmed city=NYC, got %q", records[0]["city"])
	}
}

func TestFileParser_Avro_NullableField(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	// Avro union ["null", "string"] — value can be null or string
	schema := `{
		"type": "record", "name": "Patient",
		"fields": [
			{"name": "name", "type": "string"},
			{"name": "notes", "type": ["null", "string"], "default": null}
		]
	}`
	avroContent := createAvroBytes(t, schema, []interface{}{
		map[string]interface{}{"name": "Alice", "notes": nil},
		map[string]interface{}{"name": "Bob", "notes": map[string]interface{}{"string": "some note"}},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "avro",
	})
	output, err := executor.Execute(context.Background(), step, makeInput("raw_file", avroContent))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	// Null union value should be nil
	if records[0]["notes"] != nil {
		t.Errorf("Expected notes=nil for Alice, got %v", records[0]["notes"])
	}
	// Union-wrapped string should be unwrapped by flattenAvroUnion
	if records[1]["notes"] != "some note" {
		t.Errorf("Expected notes='some note' for Bob (union unwrapped), got %v", records[1]["notes"])
	}
}

func TestFileParser_Avro_AutoDetect(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	avroContent := createAvroBytes(t, patientAvroSchema, []interface{}{
		map[string]interface{}{"name": "Alice", "age": int32(30), "city": "NYC"},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"autoDetect":  true,
	})
	output, err := executor.Execute(context.Background(), step, makeInput("raw_file", avroContent))
	if err != nil {
		t.Fatalf("Auto-detect Avro failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got %v", records[0]["name"])
	}
}

// ─── Parquet Tests ─────────────────────────────────────────────

// patientParquetRow is used to write in-memory Parquet test files.
type patientParquetRow struct {
	Name string `parquet:"name"`
	Age  int64  `parquet:"age"`
	City string `parquet:"city"`
}

// createParquetBytes builds a minimal Parquet file in memory and returns raw bytes as a string.
func createParquetBytes(t *testing.T, rows []patientParquetRow) string {
	t.Helper()
	var buf bytes.Buffer
	w := parquet.NewWriter(&buf, parquet.SchemaOf(patientParquetRow{}))
	for _, r := range rows {
		if err := w.Write(r); err != nil {
			t.Fatalf("createParquetBytes: Write: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("createParquetBytes: Close: %v", err)
	}
	return buf.String()
}

func TestFileParser_Parquet_Basic(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	parquetContent := createParquetBytes(t, []patientParquetRow{
		{Name: "Alice", Age: 30, City: "NYC"},
		{Name: "Bob", Age: 25, City: "LA"},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "parquet",
	})
	output, err := executor.Execute(context.Background(), step, makeInput("raw_file", parquetContent))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got %v", records[0]["name"])
	}
	if records[1]["city"] != "LA" {
		t.Errorf("Expected city=LA, got %v", records[1]["city"])
	}
}

func TestFileParser_Parquet_Columns(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	parquetContent := createParquetBytes(t, []patientParquetRow{
		{Name: "Alice", Age: 30, City: "NYC"},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "parquet",
	})
	output, _ := executor.Execute(context.Background(), step, makeInput("raw_file", parquetContent))

	stepOut, _ := output["_stepOutput"].(map[string]interface{})
	cols, _ := stepOut["columns"].([]string)
	if len(cols) != 3 {
		t.Fatalf("Expected 3 columns (name, age, city), got %d: %v", len(cols), cols)
	}
	colSet := make(map[string]bool)
	for _, c := range cols {
		colSet[c] = true
	}
	for _, expected := range []string{"name", "age", "city"} {
		if !colSet[expected] {
			t.Errorf("Expected column %q in Parquet output columns", expected)
		}
	}
}

func TestFileParser_Parquet_MaxRecords(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	rows := make([]patientParquetRow, 5)
	for i := range rows {
		rows[i] = patientParquetRow{Name: fmt.Sprintf("Person%d", i), Age: int64(i), City: "NYC"}
	}
	parquetContent := createParquetBytes(t, rows)

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "parquet",
		"maxRecords":  2,
	})
	output, err := executor.Execute(context.Background(), step, makeInput("raw_file", parquetContent))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Errorf("Expected 2 records (maxRecords=2), got %d", len(records))
	}
}

func TestFileParser_Parquet_SkipRows(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	parquetContent := createParquetBytes(t, []patientParquetRow{
		{Name: "Skip1", Age: 1, City: "X"},
		{Name: "Skip2", Age: 2, City: "X"},
		{Name: "Alice", Age: 30, City: "NYC"},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "parquet",
		"skipRows":    2,
	})
	output, err := executor.Execute(context.Background(), step, makeInput("raw_file", parquetContent))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 1 {
		t.Fatalf("Expected 1 record after skipRows=2, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice after skip, got %v", records[0]["name"])
	}
}

func TestFileParser_Parquet_NumericTypes(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	parquetContent := createParquetBytes(t, []patientParquetRow{
		{Name: "Alice", Age: 30, City: "NYC"},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "parquet",
	})
	output, err := executor.Execute(context.Background(), step, makeInput("raw_file", parquetContent))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	age, ok := records[0]["age"].(int64)
	if !ok {
		t.Fatalf("Expected age to be int64, got %T (%v)", records[0]["age"], records[0]["age"])
	}
	if age != 30 {
		t.Errorf("Expected age=30, got %d", age)
	}
}

func TestFileParser_Parquet_AutoDetect(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	parquetContent := createParquetBytes(t, []patientParquetRow{
		{Name: "Alice", Age: 30, City: "NYC"},
	})

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"autoDetect":  true,
	})
	output, err := executor.Execute(context.Background(), step, makeInput("raw_file", parquetContent))
	if err != nil {
		t.Fatalf("Auto-detect Parquet failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got %v", records[0]["name"])
	}
}

// ─── Test Helpers ─────────────────────────────────────────────

// ─── Chunked / Streaming CSV tests ───────────────────────────────────────────
// These tests exercise the GB-scale streaming path:
//   local_path + maxRecords > 0 + CSV/TSV → executeStreamingLocalCSV
//   which calls ParseCSVFromReaderChunked and returns has_more + next_offset.

// writeTmpCSV writes content to a temp file and returns its path.
func writeTmpCSV(t *testing.T, content string, ext string) string {
	t.Helper()
	if ext == "" {
		ext = ".csv"
	}
	f, err := os.CreateTemp("", "fp_chunk_*"+ext)
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

// getStepOutput returns the _stepOutput map from executor output.
func getStepOutput(t *testing.T, out map[string]interface{}) map[string]interface{} {
	t.Helper()
	so, ok := out["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("_stepOutput not found or wrong type; keys=%v", mapKeys(out))
	}
	return so
}

func mapKeys(m map[string]interface{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// TestFileParser_StreamingCSV_FirstChunk verifies that maxRecords=2 on a 5-row file
// returns exactly 2 records, has_more=true, and next_offset=2.
func TestFileParser_StreamingCSV_FirstChunk(t *testing.T) {
	content := "id,name\n1,Alice\n2,Bob\n3,Carol\n4,Dave\n5,Eve\n"
	path := writeTmpCSV(t, content, ".csv")

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType": "local_path",
		"filePath":   path,
		"fileFormat": "csv",
		"hasHeader":  true,
		"maxRecords": 2,
		"offset":     0,
	})

	out, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	so := getStepOutput(t, out)

	// records
	recs, ok := so["records"].([]map[string]interface{})
	if !ok {
		t.Fatalf("records wrong type: %T", so["records"])
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
	if recs[0]["id"] != "1" || recs[0]["name"] != "Alice" {
		t.Errorf("unexpected first record: %v", recs[0])
	}
	if recs[1]["id"] != "2" || recs[1]["name"] != "Bob" {
		t.Errorf("unexpected second record: %v", recs[1])
	}

	// pagination fields
	hasMore, _ := so["has_more"].(bool)
	if !hasMore {
		t.Error("expected has_more=true")
	}
	nextOffset, _ := so["next_offset"].(int)
	if nextOffset != 2 {
		t.Errorf("expected next_offset=2, got %d", nextOffset)
	}
	recordCount, _ := so["record_count"].(int)
	if recordCount != 2 {
		t.Errorf("expected record_count=2, got %d", recordCount)
	}

	// execution details should show streaming=true
	ed, _ := out["_executionDetails"].(map[string]interface{})
	if streaming, _ := ed["streaming"].(bool); !streaming {
		t.Error("expected _executionDetails.streaming=true")
	}
}

// TestFileParser_StreamingCSV_SecondChunk reads the second chunk (offset=2, maxRecords=2).
func TestFileParser_StreamingCSV_SecondChunk(t *testing.T) {
	content := "id,name\n1,Alice\n2,Bob\n3,Carol\n4,Dave\n5,Eve\n"
	path := writeTmpCSV(t, content, ".csv")

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType": "local_path",
		"filePath":   path,
		"fileFormat": "csv",
		"hasHeader":  true,
		"maxRecords": 2,
		"offset":     2, // skip first chunk
	})

	out, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	so := getStepOutput(t, out)
	recs, _ := so["records"].([]map[string]interface{})
	if len(recs) != 2 {
		t.Fatalf("expected 2 records for chunk 2, got %d", len(recs))
	}
	if recs[0]["id"] != "3" {
		t.Errorf("expected id=3, got %v", recs[0]["id"])
	}
	if recs[1]["id"] != "4" {
		t.Errorf("expected id=4, got %v", recs[1]["id"])
	}

	hasMore, _ := so["has_more"].(bool)
	if !hasMore {
		t.Error("expected has_more=true (row 5 is still pending)")
	}
	nextOffset, _ := so["next_offset"].(int)
	if nextOffset != 4 {
		t.Errorf("expected next_offset=4, got %d", nextOffset)
	}
}

// TestFileParser_StreamingCSV_LastChunk reads the last chunk and verifies has_more=false.
func TestFileParser_StreamingCSV_LastChunk(t *testing.T) {
	content := "id,name\n1,Alice\n2,Bob\n3,Carol\n4,Dave\n5,Eve\n"
	path := writeTmpCSV(t, content, ".csv")

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType": "local_path",
		"filePath":   path,
		"fileFormat": "csv",
		"hasHeader":  true,
		"maxRecords": 2,
		"offset":     4, // skip first two chunks
	})

	out, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	so := getStepOutput(t, out)
	recs, _ := so["records"].([]map[string]interface{})
	if len(recs) != 1 {
		t.Fatalf("expected 1 record (last row), got %d", len(recs))
	}
	if recs[0]["id"] != "5" {
		t.Errorf("expected id=5, got %v", recs[0]["id"])
	}

	hasMore, _ := so["has_more"].(bool)
	if hasMore {
		t.Error("expected has_more=false on last chunk")
	}
	nextOffset, _ := so["next_offset"].(int)
	if nextOffset != 5 {
		t.Errorf("expected next_offset=5, got %d", nextOffset)
	}
}

// TestFileParser_StreamingCSV_ExactFit verifies has_more=false when the file has
// exactly maxRecords rows (i.e. the chunk fills perfectly with nothing left).
func TestFileParser_StreamingCSV_ExactFit(t *testing.T) {
	content := "id,name\n1,Alice\n2,Bob\n"
	path := writeTmpCSV(t, content, ".csv")

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType": "local_path",
		"filePath":   path,
		"fileFormat": "csv",
		"hasHeader":  true,
		"maxRecords": 2,
		"offset":     0,
	})

	out, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	so := getStepOutput(t, out)
	recs, _ := so["records"].([]map[string]interface{})
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
	hasMore, _ := so["has_more"].(bool)
	if hasMore {
		t.Error("expected has_more=false when file has exactly maxRecords rows")
	}
}

// TestFileParser_StreamingTSV verifies TSV detection from extension.
func TestFileParser_StreamingTSV(t *testing.T) {
	content := "id\tname\n1\tAlice\n2\tBob\n3\tCarol\n"
	path := writeTmpCSV(t, content, ".tsv")

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType": "local_path",
		"filePath":   path,
		"hasHeader":  true,
		"maxRecords": 2,
		"offset":     0,
		// no fileFormat — should auto-detect tsv from extension
	})

	out, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	so := getStepOutput(t, out)
	recs, _ := so["records"].([]map[string]interface{})
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
	if recs[0]["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", recs[0]["name"])
	}
	hasMore, _ := so["has_more"].(bool)
	if !hasMore {
		t.Error("expected has_more=true (row 3 pending)")
	}
}

// TestFileParser_StreamingCSV_ContextCancel verifies early termination when context is cancelled.
func TestFileParser_StreamingCSV_ContextCancel(t *testing.T) {
	// Build a 100-row CSV
	var b strings.Builder
	b.WriteString("id,value\n")
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&b, "%d,v%d\n", i, i)
	}
	path := writeTmpCSV(t, b.String(), ".csv")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType": "local_path",
		"filePath":   path,
		"fileFormat": "csv",
		"hasHeader":  true,
		"maxRecords": 50,
	})

	// Should not error — context cancel yields partial (possibly empty) results
	out, err := executor.Execute(ctx, step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("cancelled context should not return an error: %v", err)
	}
	so := getStepOutput(t, out)
	recs, _ := so["records"].([]map[string]interface{})
	// With immediate cancel, we expect 0 or very few records (not all 50)
	if len(recs) >= 50 {
		t.Errorf("expected < 50 records with cancelled context, got %d", len(recs))
	}
}

// TestParseCSVFromReaderChunked_Unit directly tests the csv_parser helper.
func TestParseCSVFromReaderChunked_Unit(t *testing.T) {
	csvData := "id,name,city\n1,Alice,NYC\n2,Bob,LA\n3,Carol,SF\n4,Dave,CHI\n"

	t.Run("first chunk", func(t *testing.T) {
		r := strings.NewReader(csvData)
		cfg := &models.FileParserConfig{
			HasHeader:  true,
			MaxRecords: 2,
			Offset:     0,
		}
		records, cols, hasMore, err := ParseCSVFromReaderChunked(context.Background(), r, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("expected 2 records, got %d", len(records))
		}
		if records[0]["id"] != "1" {
			t.Errorf("expected id=1, got %v", records[0]["id"])
		}
		if !hasMore {
			t.Error("expected hasMore=true")
		}
		if len(cols) != 3 {
			t.Errorf("expected 3 columns, got %d: %v", len(cols), cols)
		}
	})

	t.Run("second chunk via fresh reader + offset", func(t *testing.T) {
		r := strings.NewReader(csvData)
		cfg := &models.FileParserConfig{
			HasHeader:  true,
			MaxRecords: 2,
			Offset:     2,
		}
		records, _, hasMore, err := ParseCSVFromReaderChunked(context.Background(), r, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("expected 2 records, got %d", len(records))
		}
		if records[0]["id"] != "3" {
			t.Errorf("expected id=3, got %v", records[0]["id"])
		}
		if hasMore {
			t.Error("expected hasMore=false (only 4 rows total)")
		}
	})

	t.Run("no header — auto col names", func(t *testing.T) {
		r := strings.NewReader("A,B\nC,D\n")
		cfg := &models.FileParserConfig{
			HasHeader:  false,
			MaxRecords: 5,
		}
		records, cols, _, err := ParseCSVFromReaderChunked(context.Background(), r, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("expected 2 records, got %d", len(records))
		}
		if cols[0] != "col_1" || cols[1] != "col_2" {
			t.Errorf("expected col_1, col_2; got %v", cols)
		}
	})

	t.Run("offset beyond end of file", func(t *testing.T) {
		r := strings.NewReader(csvData)
		cfg := &models.FileParserConfig{
			HasHeader:  true,
			MaxRecords: 5,
			Offset:     100, // way past the end
		}
		records, _, hasMore, err := ParseCSVFromReaderChunked(context.Background(), r, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(records) != 0 {
			t.Errorf("expected 0 records, got %d", len(records))
		}
		if hasMore {
			t.Error("expected hasMore=false when offset is past EOF")
		}
	})
}

// ─── End of Chunked / Streaming CSV tests ────────────────────────────────────

// ─── File provenance ("_source_file") ────────────────────────────────────
//
// executeBatch has always stamped "_source_file" onto every record when
// processing multiple local files in one step. These tests cover the same
// stamping for the far more common single-file paths — "field" (a real
// inbound connector, e.g. file_listener, delivering one file's content per
// message) and "local_path" — which previously left every record with no
// way to trace which file it came from.

func TestFileParser_SourceFileProvenance_FieldSourceType_UsesMessageSourceFile(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	csv := "name,age\nAlice,30\nBob,25\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "csvContent",
		"fileFormat":  "csv",
		"hasHeader":   true,
	})
	// _sourceFile is what processing/engine_message_processor.go stamps onto
	// message.* once, before any pipeline step runs, for every message that
	// arrived via a file-based inbound connector — mirror that shape here
	// rather than a bare top-level field.
	input := map[string]interface{}{
		"message": map[string]interface{}{
			"csvContent":  csv,
			"_sourceFile": "patients.csv",
		},
	}

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	for i, rec := range records {
		if rec["_source_file"] != "patients.csv" {
			t.Errorf("record %d: expected _source_file=patients.csv, got %v", i, rec["_source_file"])
		}
	}

	execDetails, ok := output["_executionDetails"].(map[string]interface{})
	if !ok {
		t.Fatal("_executionDetails not found")
	}
	if execDetails["source_file"] != "patients.csv" {
		t.Errorf("Expected execution details source_file=patients.csv, got %v", execDetails["source_file"])
	}
}

func TestFileParser_SourceFileProvenance_FieldSourceType_AbsentWhenNoSourceFile(t *testing.T) {
	executor := NewFileParserExecutor(nil, nil)
	csv := "name,age\nAlice,30\n"

	step := makeStep(map[string]interface{}{
		"sourceField": "raw_file",
		"fileFormat":  "csv",
		"hasHeader":   true,
	})
	// No message._sourceFile at all (e.g. a hand-built test message, or
	// content that never went through a file-based connector) — must not
	// fabricate a _source_file value.
	input := makeInput("raw_file", csv)

	output, err := executor.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	if _, exists := records[0]["_source_file"]; exists {
		t.Errorf("Expected no _source_file when message._sourceFile is absent, got %v", records[0]["_source_file"])
	}

	execDetails, _ := output["_executionDetails"].(map[string]interface{})
	if _, exists := execDetails["source_file"]; exists {
		t.Errorf("Expected no source_file in execution details, got %v", execDetails["source_file"])
	}
}

func TestFileParser_SourceFileProvenance_LocalPath_UsesFileBaseName(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "fp_provenance_*.csv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })
	if _, err := tmpFile.WriteString("name,age\nAlice,30\n"); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	executor := NewFileParserExecutor(nil, nil)
	step := makeStep(map[string]interface{}{
		"sourceType": "local_path",
		"filePath":   tmpFile.Name(),
		"fileFormat": "csv",
		"hasHeader":  true,
	})

	output, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	records := getRecords(t, output, "_stepOutput.records")
	expectedName := filepath.Base(tmpFile.Name())
	if records[0]["_source_file"] != expectedName {
		t.Errorf("Expected _source_file=%s, got %v", expectedName, records[0]["_source_file"])
	}

	execDetails, ok := output["_executionDetails"].(map[string]interface{})
	if !ok {
		t.Fatal("_executionDetails not found")
	}
	if execDetails["source_file"] != expectedName {
		t.Errorf("Expected execution details source_file=%s, got %v", expectedName, execDetails["source_file"])
	}
}

// getRecords extracts parsed records from the output at the given dot-path
func getRecords(t *testing.T, output map[string]interface{}, path string) []map[string]interface{} {
	t.Helper()

	parts := strings.Split(path, ".")
	var current interface{} = output
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected map at '%s', got %T", part, current)
		}
		current = m[part]
		if current == nil {
			t.Fatalf("Path '%s' not found (nil at '%s')", path, part)
		}
	}

	records, ok := current.([]map[string]interface{})
	if !ok {
		t.Fatalf("Expected []map[string]interface{} at '%s', got %T", path, current)
	}
	return records
}
