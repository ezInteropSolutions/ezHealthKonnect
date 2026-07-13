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
//
//	sourceField  — dot-path to the parsed CDA map (default: looks for _format=ccda)
//	sections     — []string of section keys to export (default: every section
//	               with an OOB CSV template — see SupportedCDACSVSections())
//	outputPrefix — prefix for each section's output field (default: "csv_",
//	               producing e.g. "csv_medications", "csv_allergiesAndIntolerances")
package transform

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
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

	// _sourceFile is stamped onto the message envelope once, at ingestion
	// (processing/engine_message_processor.go, from the inbound connector's
	// original filename/identifier) — read it here so every CSV row can be
	// traced back to the document it came from, independent of whatever
	// filename the outbound delivery step later gives the CSV file itself.
	sourceFile := sourceFileFromInputData(inputData)

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

		var rows []map[string]string
		var columns []CSVColumn
		if tmpl.NarrativeOnly {
			rows, columns = buildNarrativeRow(documentMap, tmpl.SectionKey, sourceFile)
		} else {
			rows = buildCSVRows(documentMap, tmpl, sourceFile)
			columns = csvColumnsWithSourceFile(resolvedSectionColumns(tmpl))
		}
		csvText := writeCSV(columns, rows)

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

// sourceFileFromInputData reads the _sourceFile stamped onto the message
// envelope at ingestion (see processing/engine_message_processor.go). Empty
// for non-file-based sources (TCP, HTTP, ...) or when unavailable — never an
// error, since most clinical fields here are best-effort by design.
func sourceFileFromInputData(inputData map[string]interface{}) string {
	msg, ok := inputData["message"].(map[string]interface{})
	if !ok {
		return ""
	}
	sourceFile, _ := msg["_sourceFile"].(string)
	return sourceFile
}

// csvColumnsWithSourceFile prepends the universal SourceFile column to a
// section template's own columns — added once here rather than authored into
// every one of the ~35 section templates individually. Unlike
// universalEntryColumns below, SourceFile has no CDA path to resolve (it's
// document-level ingestion metadata, stamped directly onto each row by
// buildCSVRows) — this is purely a header-list concern for writeCSV, which
// only ever does a row[c.Name] map lookup, never Path resolution.
func csvColumnsWithSourceFile(columns []CSVColumn) []CSVColumn {
	out := make([]CSVColumn, 0, len(columns)+1)
	out = append(out, CSVColumn{Name: "SourceFile"})
	return append(out, columns...)
}

// universalEntryColumns are prepended to every non-NarrativeOnly section's
// own columns (see resolvedSectionColumns) — CDAEntry.Id/Authors are the SAME
// relative shape on every entry regardless of section, unlike CodeSystem/
// CodeSystemName/OriginalText (see ExposeCodeMetadata), which depend on
// where a section's own coded concept lives. Resolved through the ordinary
// column mechanism (buildRowsForEntry/buildComponentRow), so an organizer-
// flattened row (vitals, family history, care team) naturally reads the
// COMPONENT's own id/author first, falling back to the parent organizer —
// same behavior every other column already gets, no special-casing needed.
//
// Real gap found auditing Social History: the entry's own <id>, and its
// <author><assignedAuthor><assignedPerson> (who recorded the observation),
// were both fully parsed already (CDAEntry.Id, CDAEntry.Authors) but never
// read by any CSV template.
var universalEntryColumns = []CSVColumn{
	{Name: "EntryId", Path: "id[0].extension", FallbackPaths: []string{"id[0].root"}},
	{Name: "AuthorName", Path: "authors[0].assignedAuthor.assignedPerson.names[0]"},
	{Name: "AuthorOrganization", Path: "authors[0].assignedAuthor.representedOrganization.names[0]"},
	// Comment Activity (templateId 2.16.840.1.113883.10.20.22.4.64, code
	// 48767-8, entryRelationship typeCode=COMP) is the SAME general-purpose
	// annotation pattern the C-CDA on FHIR IG specifies identically across
	// every section (not just medications, which already used it for its own
	// "Note" column before this) — universal here rather than re-authored
	// per template.
	{Name: "Comment", Path: "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.64].text"},
}

// expandCodeMetadataColumns inserts, immediately after each
// ExposeCodeMetadata column, three sibling columns reading off the exact
// same CDACode map via the exact same Path plus a suffix — see
// CSVColumn.ExposeCodeMetadata's doc comment for why this only makes sense
// for columns whose Path already resolves to a whole CDACode map.
func expandCodeMetadataColumns(columns []CSVColumn) []CSVColumn {
	out := make([]CSVColumn, 0, len(columns))
	for _, col := range columns {
		out = append(out, col)
		if !col.ExposeCodeMetadata {
			continue
		}
		out = append(out,
			CSVColumn{Name: col.Name + "CodeSystem", Path: col.Path + ".codeSystem"},
			CSVColumn{Name: col.Name + "CodeSystemName", Path: col.Path + ".codeSystemName"},
			CSVColumn{Name: col.Name + "OriginalText", Path: col.Path + ".originalText"},
		)
	}
	return out
}

// resolvedSectionColumns returns the actual column list a non-NarrativeOnly
// section's rows are built against and its CSV header written from (SourceFile
// aside, added separately — see csvColumnsWithSourceFile): universalEntryColumns
// first, then the section's own columns with every ExposeCodeMetadata column
// expanded. Single source of truth shared by buildCSVRows (row values) and
// Execute (CSV header), so the two can never drift out of sync.
func resolvedSectionColumns(tmpl CSVSectionTemplate) []CSVColumn {
	withUniversal := make([]CSVColumn, 0, len(tmpl.Columns)+len(universalEntryColumns))
	withUniversal = append(withUniversal, universalEntryColumns...)
	withUniversal = append(withUniversal, tmpl.Columns...)
	return expandCodeMetadataColumns(withUniversal)
}

// narrativeRowColumns is the fixed 2-column shape every NarrativeOnly
// section produces — see buildNarrativeRow.
var narrativeRowColumns = []CSVColumn{{Name: "SourceFile"}, {Name: "NarrativeText"}}

// buildNarrativeRow handles sections that are narrative-only per C-CDA (no
// structured <entry> data at all in real-world documents: Assessment,
// Reason for Referral, History of Present Illness, Review of Systems,
// Physical Exam, Past Medical History, Discharge Instructions, Hospital
// Course, Assessment and Plan / Plan of Treatment) — a per-entry CSVColumn
// table doesn't fit these; instead this emits exactly one row holding the
// section's own narrative text, plain-text rendered (see
// stripCDANarrativeXML). Zero rows (not an error) when the section is
// absent or its narrative is empty/whitespace-only — same "no data, no
// error" contract as every other section.
func buildNarrativeRow(documentMap map[string]interface{}, sectionKey, sourceFile string) ([]map[string]string, []CSVColumn) {
	narrativeXML, _ := executors.ResolveCDAPath(documentMap, "sectionsByKey."+sectionKey+".narrativeText", false).(string)
	text := stripCDANarrativeXML(narrativeXML)
	if text == "" {
		return []map[string]string{}, narrativeRowColumns
	}
	return []map[string]string{{"SourceFile": sourceFile, "NarrativeText": text}}, narrativeRowColumns
}

// blockLevelNarrativeTags are CDA narrative XHTML-like elements whose closing
// tag should insert a line break in stripCDANarrativeXML's plain-text
// rendering, so a table's rows/cells (the dominant narrative shape across
// real C-CDA documents) don't run together into one unreadable line.
var blockLevelNarrativeTags = map[string]bool{
	"tr": true, "table": true, "thead": true, "tbody": true,
	"p": true, "paragraph": true, "div": true, "li": true, "br": true,
}

// stripCDANarrativeXML renders a CDA section's serialised <text> XML
// (CDASection.NarrativeText, produced by cda/document/section_parser.go's
// serializeElement) as plain, readable text for a single CSV cell: tags
// dropped, entities decoded by the XML decoder, block-level elements
// separated by newlines. Malformed or empty input never errors — returns ""
// or whatever text was recovered before the decoder gave up, matching this
// whole executor's "best effort, never fail the section" philosophy.
func stripCDANarrativeXML(narrativeXML string) string {
	if strings.TrimSpace(narrativeXML) == "" {
		return ""
	}
	dec := xml.NewDecoder(strings.NewReader(narrativeXML))
	var buf strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text == "" {
				continue
			}
			if buf.Len() > 0 {
				last := buf.String()[buf.Len()-1]
				if last != '\n' {
					buf.WriteByte(' ')
				}
			}
			buf.WriteString(text)
		case xml.EndElement:
			if blockLevelNarrativeTags[t.Name.Local] {
				buf.WriteByte('\n')
			}
		}
	}
	return strings.TrimSpace(buf.String())
}

// buildCSVRows resolves one flat row per section entry, per
// resolvedSectionColumns(tmpl) (universal entry columns + the section's own,
// with ExposeCodeMetadata columns expanded), plus a SourceFile cell (see
// csvColumnsWithSourceFile) stamped onto every row from the document's own
// ingestion metadata, not the entry itself. A column whose path resolves to
// nothing produces an empty cell — never an error — since most clinical
// fields are legitimately optional per entry.
func buildCSVRows(documentMap map[string]interface{}, tmpl CSVSectionTemplate, sourceFile string) []map[string]string {
	columns := resolvedSectionColumns(tmpl)
	entries := executors.ResolveCDAPaths(documentMap, "sectionsByKey."+tmpl.SectionKey+".entries[*]", false)
	rows := make([]map[string]string, 0, len(entries))
	for _, entryNode := range entries {
		entryMap, ok := entryNode.(map[string]interface{})
		if !ok {
			continue
		}
		for _, row := range buildRowsForEntry(entryMap, columns) {
			row["SourceFile"] = sourceFile
			rows = append(rows, row)
		}
	}
	return rows
}

// buildRowsForEntry resolves one CSV row per organizer component when the
// entry wraps nested components — CDA's cluster/panel pattern (Vital Signs
// Organizer's systolic/diastolic BP, height, weight, BMI; a Result
// Organizer's individual lab analytes) where the clinically meaningful
// values live one level deeper than the entry itself. Without this, such a
// section produces exactly one row per visit/panel carrying only the
// cluster-level code and none of the actual readings — verified against a
// real 99397 Epic CCD's Vital Signs Organizer (5 components: sysBP, diaBP,
// height, weight, BMI), which the flat entry-only resolution silently
// collapsed to a single, value-less row. Falls back to a single row against
// the entry directly for every other (flat, non-organizer) entry shape.
func buildRowsForEntry(entryMap map[string]interface{}, columns []CSVColumn) []map[string]string {
	components, _ := entryMap["components"].([]interface{})
	if len(components) == 0 {
		return []map[string]string{buildRow(entryMap, columns)}
	}
	rows := make([]map[string]string, 0, len(components))
	for _, compNode := range components {
		compMap, ok := compNode.(map[string]interface{})
		if !ok {
			continue
		}
		rows = append(rows, buildComponentRow(compMap, entryMap, columns))
	}
	return rows
}

// buildComponentRow resolves each column against the component first,
// falling back to the parent organizer entry for any column the component
// itself leaves empty — e.g. a shared effectiveTime the IG allows to be
// stated once on the organizer rather than repeated on every component.
func buildComponentRow(compMap, entryMap map[string]interface{}, columns []CSVColumn) map[string]string {
	row := make(map[string]string, len(columns))
	for _, col := range columns {
		cell := resolveColumn(compMap, col)
		if cell == "" {
			cell = resolveColumn(entryMap, col)
		}
		row[col.Name] = cell
	}
	return row
}

// buildRow resolves one row against a single (non-organizer) entry map.
func buildRow(entryMap map[string]interface{}, columns []CSVColumn) map[string]string {
	row := make(map[string]string, len(columns))
	for _, col := range columns {
		row[col.Name] = resolveColumn(entryMap, col)
	}
	return row
}

// resolveColumn resolves one column against node: col.Path first, then each
// of col.FallbackPaths in order via executors.ResolveCDAFallback — the
// "field A, else field B" primitive real C-CDA data needs when the same
// clinical fact is represented two different ways across documents (see
// CSVColumn's own doc comment, and declarative_oob_rules.go's matching
// Scope/ScopeFallbacks idiom on the FHIR side of these same fields).
func resolveColumn(node map[string]interface{}, col CSVColumn) string {
	if len(col.FallbackPaths) == 0 {
		return cdaValueToCSVString(executors.ResolveCDAPath(node, col.Path, false))
	}
	paths := append([]string{col.Path}, col.FallbackPaths...)
	return cdaValueToCSVString(executors.ResolveCDAFallback(node, false, paths...))
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
	// Real Epic-style CCDs routinely omit an inline displayName in favor of
	// <originalText><reference value="#id"/></originalText> pointing into the
	// section's narrative — already resolved into this field by
	// cda/document/section_parser.go's resolveEntryRefs, but never read by
	// this template layer until now (see CSVColumn's doc comment).
	if original, ok := m["originalText"].(string); ok && original != "" {
		return original
	}
	if code, ok := m["code"].(string); ok && code != "" {
		return code
	}
	// CDAName shape (Prefix/Given/Family/Suffix) -- e.g. a Care Team member's
	// or Payer performer's assignedPerson.names[0]. Checked before "value"
	// since a CDAName never has a "value" key of its own.
	if name := cdaNameToCSVString(m); name != "" {
		return name
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

// cdaNameToCSVString renders a CDAName map (Prefix/Given/Family/Suffix, e.g.
// a Care Team member's or Payer performer's assignedPerson.names[0]) as one
// display string: "Jennifer Murdza FNP-BC". Returns "" when the map has
// neither a "given" nor a "family" key (not a name shape at all).
func cdaNameToCSVString(m map[string]interface{}) string {
	given, hasGiven := m["given"].([]interface{})
	family, hasFamily := m["family"].(string)
	if !hasGiven && !hasFamily {
		return ""
	}
	parts := make([]string, 0, len(given)+2)
	if prefix, ok := m["prefix"].(string); ok && prefix != "" {
		parts = append(parts, prefix)
	}
	for _, g := range given {
		if s, ok := g.(string); ok && s != "" {
			parts = append(parts, s)
		}
	}
	if family != "" {
		parts = append(parts, family)
	}
	if suffix, ok := m["suffix"].(string); ok && suffix != "" {
		parts = append(parts, suffix)
	}
	return strings.Join(parts, " ")
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
