// services/executors/transform/cda_section_to_csv_executor.go
// CDASectionToCSVExecutor — pipeline step type "cda.section_to_csv".
//
// Converts a parsed CDA/CCD document's clinical sections into flat CSV
// output — one CSV per section, one row per section entry — using the OOB
// column templates in cda_csv_templates.go. Deliberately a CLINICAL DATA
// MODEL view (MedicationName/Dose/Route/Status, ...), NOT a FHIR view: no
// FHIR resource shaping, no Transform functions, no CodeableConcept objects
// — just flat, human-readable columns pulled directly off each entry via the
// same CDA path grammar declarative_oob_rules.go's MappingRow.SourcePath
// already uses.
//
// Config keys (all optional):
//   sourceField  — dot-path to the parsed CDA map (default: looks for _format=ccda)
//   sections     — []string of section keys to export (default: every section
//                  with an OOB CSV template — see SupportedCDACSVSections())
//   outputPrefix — prefix for each section's output field (default: "csv_",
//                  producing e.g. "csv_medications", "csv_allergiesAndIntolerances")
package transform

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	cdaparser "ezhealthkonnect/services/parsers/cda"
)

// CDASectionToCSVExecutor converts parsed CDA sections to per-section CSV text.
type CDASectionToCSVExecutor struct {
	*executors.BaseExecutor
	parserService *cdaparser.CDAParserService // used for auto-parse when _cdaDocument is absent
}

// NewCDASectionToCSVExecutor constructs the executor, loading the CDA parser
// (for the auto-parse fallback) the same way cda_to_fhir_executor.go does.
func NewCDASectionToCSVExecutor() *CDASectionToCSVExecutor {
	exec := &CDASectionToCSVExecutor{
		BaseExecutor: executors.NewBaseExecutor("cda.section_to_csv", models.ExecutorMetadata{
			Name:        "CDA Section → CSV",
			Description: "Converts CDA/CCD clinical sections into flat, one-row-per-entry CSV output (clinical data model, not FHIR)",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "CDA Transform",
		}),
	}

	if svc, err := cdaparser.NewFromSchemaDir("./cda/schemas"); err == nil {
		exec.parserService = svc
	} else {
		log.Printf("⚠️  [cda.section_to_csv] CDAParserService init failed (%v) — auto-parse disabled, requires an upstream cda.parse step", err)
	}

	return exec
}

type cdaSectionToCSVConfig struct {
	SourceField  string   `json:"sourceField"`
	Sections     []string `json:"sections"`
	OutputPrefix string   `json:"outputPrefix"`
}

// Execute resolves the typed CDA document, builds one CSV per requested
// section, and writes each under "<outputPrefix><sectionKey>" in the output
// data plus _stepOutput for the field picker.
func (e *CDASectionToCSVExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	cfg := cdaSectionToCSVConfig{OutputPrefix: "csv_"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.OutputPrefix == "" {
		cfg.OutputPrefix = "csv_"
	}

	sectionKeys := cfg.Sections
	if len(sectionKeys) == 0 {
		sectionKeys = SupportedCDACSVSections()
	}

	documentMap, err := e.resolveDocumentMap(inputData, cfg.SourceField)
	if err != nil {
		return nil, fmt.Errorf("cda.section_to_csv: %w", err)
	}

	outputData := make(map[string]interface{}, len(inputData)+len(sectionKeys))
	for k, v := range inputData {
		outputData[k] = v
	}

	stepOutVars := make(map[string]interface{}, len(sectionKeys))
	sectionRowCounts := make(map[string]int, len(sectionKeys))

	for _, sectionKey := range sectionKeys {
		tmpl, ok := cdaCSVSectionTemplates[sectionKey]
		if !ok {
			continue // no OOB template for this section key — silently skip, matches cda.to_fhir's "no rule" behavior
		}

		rows := buildCSVRows(documentMap, tmpl)
		csvText := writeCSV(tmpl.Columns, rows)

		fieldName := cfg.OutputPrefix + sectionKey
		outputData[fieldName] = csvText
		stepOutVars[fieldName] = csvText
		sectionRowCounts[sectionKey] = len(rows)
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [cda.section_to_csv] Produced %d section CSV(s) in %dms: %v", len(sectionRowCounts), durationMs, sectionRowCounts)

	e.SetStepOutputWithDetails(outputData,
		stepOutVars,
		map[string]interface{}{
			"duration_ms":   durationMs,
			"success":       true,
			"section_rows":  sectionRowCounts,
			"sections_done": len(sectionRowCounts),
		},
	)

	return outputData, nil
}

// resolveDocumentMap prefers the typed *CDADocument left inside the shared
// message object by an upstream cda.parse/cda.dedupe step (findCDADocument,
// cda_document_resolution.go) — same preference order cda_to_fhir_executor.go's
// Execute() uses — and falls back to auto-parsing raw CDA XML found via
// sourceField or a set of well-known fields when no typed document is available.
func (e *CDASectionToCSVExecutor) resolveDocumentMap(inputData map[string]interface{}, sourceField string) (map[string]interface{}, error) {
	if doc, ok := findCDADocument(inputData); ok {
		return marshalDocumentMap(doc)
	}

	if e.parserService == nil {
		return nil, fmt.Errorf("no typed CDA document available (_cdaDocument) and auto-parse is disabled (schema directory unavailable)")
	}

	rawXML := ""
	if sourceField != "" {
		if v, ok := executors.GetFieldValue(inputData, sourceField).(string); ok {
			rawXML = v
		}
	}
	if rawXML == "" {
		candidates := []string{"content", "raw_content", "rawMessage", "rawXML", "cdaXML", "raw"}
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			for _, k := range candidates {
				if v, ok := msg[k].(string); ok && strings.Contains(v, "ClinicalDocument") {
					rawXML = v
					break
				}
			}
		}
		if rawXML == "" {
			for _, k := range candidates {
				if v, ok := inputData[k].(string); ok && strings.Contains(v, "ClinicalDocument") {
					rawXML = v
					break
				}
			}
		}
	}
	if rawXML == "" {
		return nil, fmt.Errorf("no typed CDA document available and no raw CDA XML found (expected _cdaDocument or a raw XML field)")
	}

	result := e.parserService.Parse(rawXML)
	if !result.Success {
		return nil, fmt.Errorf("auto-parse failed: %s", result.Error)
	}
	doc, ok := result.TypedDocument.(*cdadocument.CDADocument)
	if !ok {
		return nil, fmt.Errorf("auto-parse produced no typed document")
	}
	return marshalDocumentMap(doc)
}

// marshalDocumentMap converts a typed *CDADocument into the generic
// map[string]interface{} shape executors.ResolveCDAPath operates on — the
// same round-trip declarative_document_mapper.go's DeclarativeMapDocument
// performs once per document before section dispatch.
func marshalDocumentMap(doc *cdadocument.CDADocument) (map[string]interface{}, error) {
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshalling document: %w", err)
	}
	var documentMap map[string]interface{}
	if err := json.Unmarshal(encoded, &documentMap); err != nil {
		return nil, fmt.Errorf("unmarshalling document: %w", err)
	}
	return documentMap, nil
}

// buildCSVRows resolves one flat row per section entry, per tmpl.Columns.
// A column whose path resolves to nothing produces an empty cell — never an
// error — since most clinical fields are legitimately optional per entry.
func buildCSVRows(documentMap map[string]interface{}, tmpl CSVSectionTemplate) []map[string]string {
	entries := executors.ResolveCDAPaths(documentMap, "sectionsByKey."+tmpl.SectionKey+".entries[*]", false)
	rows := make([]map[string]string, 0, len(entries))
	for _, entryNode := range entries {
		entryMap, ok := entryNode.(map[string]interface{})
		if !ok {
			continue
		}
		row := make(map[string]string, len(tmpl.Columns))
		for _, col := range tmpl.Columns {
			val := executors.ResolveCDAPath(entryMap, col.Path, false)
			row[col.Name] = cdaValueToCSVString(val)
		}
		rows = append(rows, row)
	}
	return rows
}

// cdaValueToCSVString renders a resolved CDA path value as a flat CSV cell.
// Deliberately tolerant of any shape (every field access is a best-effort
// type assertion), matching describeCDAValueForWarning's own philosophy in
// declarative_engine.go — a malformed/unexpected node here must only ever
// produce an empty or approximate cell, never an error.
func cdaValueToCSVString(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case bool:
		return fmt.Sprintf("%v", val)
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%v", val)
	case []interface{}:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			if s := cdaValueToCSVString(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "; ")
	case map[string]interface{}:
		return cdaMapToCSVString(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// cdaMapToCSVString handles the handful of CDA node shapes a column path can
// resolve to as a whole map (rather than a leaf string/number): CDACode
// {code, displayName}, CDAQuantity {value, unit}, CDATimeRange
// {value, low, high}, or a bare {text} node.
func cdaMapToCSVString(m map[string]interface{}) string {
	if display, ok := m["displayName"].(string); ok && display != "" {
		return display
	}
	if code, ok := m["code"].(string); ok && code != "" {
		return code
	}
	if value, hasValue := m["value"]; hasValue {
		if valStr, ok := value.(map[string]interface{}); ok {
			// CDATimeRange's own "value" sub-field (a CDATime), e.g. effectiveTime.value
			if s := cdaValueToCSVString(valStr); s != "" {
				if unit, ok := m["unit"].(string); ok && unit != "" {
					return s + " " + unit
				}
				return s
			}
		} else if s := cdaValueToCSVString(value); s != "" {
			if unit, ok := m["unit"].(string); ok && unit != "" {
				return s + " " + unit
			}
			return s
		}
	}
	if low, ok := m["low"].(map[string]interface{}); ok {
		lowStr := cdaValueToCSVString(low)
		if high, ok := m["high"].(map[string]interface{}); ok {
			if highStr := cdaValueToCSVString(high); highStr != "" {
				return lowStr + " - " + highStr
			}
		}
		return lowStr
	}
	if text, ok := m["text"].(string); ok && text != "" {
		return text
	}
	return ""
}

// writeCSV renders rows as CSV text with columns in the EXACT order given
// (not alphabetized) — deliberately not reusing
// services/executors/format.SerializeForOutput's CSV path, which always
// sorts headers alphabetically; a clinical CSV reads far better as
// Name/Code/Status/StartDate than alphabetically.
func writeCSV(columns []CSVColumn, rows []map[string]string) string {
	var buf strings.Builder
	w := csv.NewWriter(&buf)

	headers := make([]string, len(columns))
	for i, c := range columns {
		headers[i] = c.Name
	}
	_ = w.Write(headers)

	for _, row := range rows {
		record := make([]string, len(columns))
		for i, c := range columns {
			record[i] = row[c.Name]
		}
		_ = w.Write(record)
	}
	w.Flush()
	return buf.String()
}

// Validate checks step configuration.
func (e *CDASectionToCSVExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the per-section CSV outputs for the field picker.
// Section-specific field names (csv_medications, csv_allergiesAndIntolerances,
// ...) can't be enumerated generically here since they depend on config, so
// this documents the shape rather than one entry per possible section.
func (e *CDASectionToCSVExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Section CSV output", Path: "_stepOutput.csv_<sectionKey>", DataType: "string",
			Description: "One CSV string per exported section (e.g. csv_medications, csv_allergiesAndIntolerances) — header row + one data row per clinical entry", Category: "CDA Transform"},
		{Name: "Rows per section", Path: "_stepOutput.section_rows", DataType: "object",
			Description: "Map of sectionKey → number of CSV rows produced for that section", Category: "CDA Transform"},
	}
}
