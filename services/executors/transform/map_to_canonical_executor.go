// services/executors/transform/map_to_canonical_executor.go
// MapToCanonicalExecutor — pipeline step type "cda.map_to_canonical".
//
// The no-code, format-agnostic on-ramp for CCD construction: maps CSV
// columns, DB query columns, or arbitrary JSON fields onto the SAME
// canonical, USCDI-keyed JSON shape cda.parse and format.BundleToCanonicalDoc
// already produce ({"header": {...}, "sections": {key: {"entries": [...]}}}),
// so cda.build can serialize a CCD from a source system with zero new Go
// code — only step configuration. Closes the last gap in the "CDA/FHIR/CSV/
// DB, any source" plan: cda.build handles the CDA-shaped output regardless
// of producer; this step is the producer for everything that isn't already
// CDA XML (cda.parse) or a FHIR Bundle (format.BundleToCanonicalDoc).
//
// Row/field resolution reuses services/executors/field_utils.go's
// GetFieldValue/UpdateFieldValue exactly as
// services/executors/format/fhir_bundle_adapter.go already does — no new
// path-resolution mechanism. Per-field ValueMap (simple string->string
// translation, e.g. a CSV status column's "A" -> the canonical "active")
// mirrors fhir_bundle_adapter.go's kindCodeValue exactly. A full named-
// transform mechanism (services/cda_fhir/declarative_transform_registry.go)
// was deliberately NOT reused here — every one of its ~50 registered
// transforms returns a FHIR-resource-shaped value (nested CodeableConcept/
// HumanName/Address/Quantity maps), not the flat scalar string
// entry_archetypes.go's writeFieldValue/stringValue expects; wiring it in
// would let a user pick a transform that silently produces no output (a
// non-string value fails stringValue's type assertion) instead of failing
// loudly. Instead, an optional per-field Transform dispatches to
// canonical_value_transforms.go's small, pure string->string registry
// (date/datetime reformatting, trim, upper/lowercase) — CDA-target-only, so
// it structurally cannot reproduce that silent-drop failure mode. Applied
// BEFORE ValueMap (see applyFieldMapping/applyHeaderField): Transform
// normalizes the raw value, ValueMap is the exact-match override on top of
// that normalized result.
//
// Config keys:
//
//	outputField — dot-path to write the canonical JSON (default: "parsedCDA")
//	header      — []{group: "patient"|"author", target, sourcePath,
//	              transform?} — flat scalar header fields (see
//	              cda/builder/canonical_field_catalog.go's HeaderFieldCatalog
//	              for the full target-key vocabulary per group, including the
//	              "address.<key>" nesting convention for patient address),
//	              plus one repeating exception: target "ids" (patientRole's
//	              identifiers, e.g. MRN + SSN) accepts a SourcePath that
//	              resolves to an already-shaped []{root,extension} array,
//	              passed through untransformed (see applyHeaderField).
//	              informant/documentationOf/encompassingEncounter remain
//	              intentionally out of scope for this step (cda.build already
//	              synthesizes a safe fallback for documentationOf when absent).
//	sections    — []{sectionKey, rowsPath, fields: []{canonicalField,
//	              sourcePath, fallbackPaths?, transform?, valueMap?,
//	              literalValue?, condition?}, groupBy?, groupedItemsKey?,
//	              entryFields?} — groupBy (one or more source columns)
//	              buckets rows sharing that composite key into ONE entry's
//	              repeating items (see cda/schema_types.go's RepeatingGroup —
//	              the no-code producer for a section's "loop" points, e.g.
//	              Vital Signs' 1..* components). entryFields (same shape as
//	              fields) are resolved once per group, from the group's
//	              first row, for organizer-level data shared across every
//	              item rather than repeated per item (e.g. Vital Signs
//	              Organizer's own effectiveTime). A field may instead set
//	              relatedRows: {relatedRowsPath, joinLocalKey, joinRelatedKey,
//	              fields} — the cross-table join (Option C): attaches rows
//	              from a DIFFERENT pipeline array whose joinRelatedKey
//	              matches this row's own joinLocalKey (e.g. Medications'
//	              "indications", joined from a separate indication-records
//	              array via a shared medicationId column) — mutually
//	              exclusive with sourcePath/fallbackPaths/literalValue/
//	              condition on that same field.
package transform

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// MapToCanonicalExecutor builds canonical USCDI-keyed JSON from configured
// no-code field mappings against arbitrary upstream row data.
type MapToCanonicalExecutor struct {
	*executors.BaseExecutor
}

// NewMapToCanonicalExecutor constructs the executor.
func NewMapToCanonicalExecutor() *MapToCanonicalExecutor {
	return &MapToCanonicalExecutor{
		BaseExecutor: executors.NewBaseExecutor("cda.map_to_canonical", models.ExecutorMetadata{
			Name:        "Map to Canonical CDA JSON",
			Description: "No-code field mapping from CSV/DB/generic-JSON rows into the canonical USCDI-keyed JSON shape cda.build consumes",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "CDA Transform",
		}),
	}
}

// headerFieldRow maps one flat header target key (relative to
// header.<Group>) to a source path in the pipeline's inputData.
type headerFieldRow struct {
	Group      string `json:"group"`      // "patient" | "author"
	Target     string `json:"target"`     // dot-path relative to header.<Group>, e.g. "address.street"
	SourcePath string `json:"sourcePath"` // GetFieldValue-compatible path, relative to inputData

	// LiteralValue is used when SourcePath is empty — a fixed constant
	// written regardless of per-message data (e.g. a coded field's own
	// codeSystem OID, the same "codeSystem is just another mappable field,
	// not a Go-hardcoded constant" convention every SECTION field's
	// fieldMappingRow.LiteralValue already has — this was the one place
	// that convention hadn't reached yet, found via a real gap: a header
	// field needing a fixed companion codeSystem had no way to supply one).
	LiteralValue string `json:"literalValue,omitempty"`

	Transform string `json:"transform,omitempty"` // canonical_value_transforms.go transform name, applied before writing (SourcePath-driven rows only)
}

// rowCondition is the "Block 2, mechanism 1" no-code condition: branch on
// one of the row's OWN source values, rather than the field's normal
// SourcePath/FallbackPaths/LiteralValue resolution. Deliberately narrow
// (equality only) — mirrors services/cda_fhir/declarative_schema.go's
// RowCondition on the parse side exactly, including that same deliberate
// restriction (see that type's own doc comment for why "not a general
// expression evaluator" is the point, not a limitation to work around).
type rowCondition struct {
	WhenPath         string `json:"whenPath"`
	Equals           string `json:"equals"`
	ThenLiteralValue string `json:"thenLiteralValue,omitempty"`
	ThenTransform    string `json:"thenTransform,omitempty"`
}

// relatedRowsMapping is the "Block 3, Option C" cross-table join: attaches
// every row from a DIFFERENT pipeline array (RelatedRowsPath, resolved
// against the step's full inputData, NOT relative to the current row) whose
// JoinRelatedKey value matches the current row's own JoinLocalKey value,
// mapping each match through Fields into one item of the resulting array —
// the no-code producer for a RepeatingGroup fed from a SEPARATE source table
// rather than the same rowset a GroupBy composite key buckets. E.g.
// Medications' Indication entryRelationship (0..* per medication,
// CONF:1098-7536): the medications rowset has no indication columns of its
// own, but a separate "indicationRecords" pipeline array (e.g. a second
// Database Enrichment step's output) carries {medicationId, diagnosisCode,
// diagnosisDisplay} rows — JoinLocalKey="medicationId" (on the medication
// row), JoinRelatedKey="medicationId" (on the indication row).
type relatedRowsMapping struct {
	RelatedRowsPath string            `json:"relatedRowsPath"`
	JoinLocalKey    string            `json:"joinLocalKey"`
	JoinRelatedKey  string            `json:"joinRelatedKey"`
	Fields          []fieldMappingRow `json:"fields"`
}

// fieldMappingRow maps one canonical schema field key (CDAFieldDef.Key, or
// its +Display/+System/+Unit/+Family companion) to a source path relative to
// one row.
type fieldMappingRow struct {
	CanonicalField string            `json:"canonicalField"`
	SourcePath     string            `json:"sourcePath"`
	FallbackPaths  []string          `json:"fallbackPaths,omitempty"`
	Transform      string            `json:"transform,omitempty"` // canonical_value_transforms.go transform name, applied before ValueMap
	ValueMap       map[string]string `json:"valueMap,omitempty"`
	LiteralValue   string            `json:"literalValue,omitempty"`

	// Condition, when set and its WhenPath resolves to a value equal to
	// Equals, replaces the field's normal resolution entirely (including
	// "write nothing" when ThenLiteralValue is empty) — an explicit branch,
	// not an additional fallback tier.
	Condition *rowCondition `json:"condition,omitempty"`

	// RelatedRows, when set, replaces ALL other resolution (SourcePath/
	// FallbackPaths/LiteralValue/Condition are ignored): instead of writing
	// entry[CanonicalField] = scalar, it writes
	// entry[CanonicalField] = []interface{}{...} — one item per matching row
	// from a DIFFERENT pipeline array (see relatedRowsMapping's own doc
	// comment). Mutually exclusive with Condition in practice (a field is
	// either a cross-table join or a scalar/branching one, never both) —
	// checked first in applyFieldMapping.
	RelatedRows *relatedRowsMapping `json:"relatedRows,omitempty"`
}

// sectionMappingRow maps one canonical section's entries from an array of
// source rows found at RowsPath.
type sectionMappingRow struct {
	SectionKey string            `json:"sectionKey"`
	RowsPath   string            `json:"rowsPath"`
	Fields     []fieldMappingRow `json:"fields"`

	// EntriesKey is the canonical sub-key (sections.<SectionKey>.<EntriesKey>)
	// this mapping's entries are written to — defaults to "entries" (the
	// ordinary case, one mapping per SectionKey). Set to a different value
	// (e.g. "procedureEntries") when a section has more than one structural
	// entry shape and needs a SECOND, independent sectionMappingRow sharing
	// the same SectionKey but its own RowsPath/Fields — see
	// cda/schema_types.go's AlternateEntryArchetype for the cda.build-side
	// counterpart (its own EntriesKey must match this one exactly).
	EntriesKey string `json:"entriesKey,omitempty"`

	// GroupBy (Block 3, "Group By" — one or more source columns, a
	// composite key) turns this section from "one row = one entry" into
	// "rows sharing this key's value = one entry's repeating items" — the
	// no-code producer for cda/builder's RepeatingGroup mechanism
	// (cda/schema_types.go). A row missing one of the GroupBy values
	// becomes its own singleton group (with a logged warning) rather than
	// being silently dropped. Requires GroupedItemsKey.
	GroupBy []string `json:"groupBy,omitempty"`

	// GroupedItemsKey is the canonical field name each group's mapped rows
	// are collected under (must match the target section's
	// RepeatingGroup.Key in cda/schemas/ccda_2_1.json, e.g. "components" for
	// Vital Signs) — required when GroupBy is set, ignored otherwise. Kept
	// as explicit user configuration rather than an executor-side schema
	// lookup so this step stays schema-agnostic at execution time (the
	// no-code UI resolves the right value at CONFIGURATION time instead,
	// from the same live catalog it already uses for canonical field
	// targets).
	GroupedItemsKey string `json:"groupedItemsKey,omitempty"`

	// EntryFields (GroupBy mode only) are resolved ONCE per group — from the
	// group's first-seen row, using the SAME applyFieldMapping primitive as
	// Fields — and written directly onto the generated entry map, sibling to
	// GroupedItemsKey, rather than into each item. This is the producer for
	// a section's own organizer-level data that's shared across every
	// component in the panel rather than repeated per component — e.g.
	// Vital Signs Organizer's own effectiveTime (CONF:1198-7288, SHALL,
	// "may be a timestamp or an interval that spans the effectiveTimes of
	// the contained vital signs observations" — the real IG text explicitly
	// describes it as panel-level, not per-component). Left empty for any
	// GroupBy section with no organizer-level data to carry (the ordinary
	// case) — zero effect when unset.
	EntryFields []fieldMappingRow `json:"entryFields,omitempty"`
}

type mapToCanonicalConfig struct {
	OutputField string              `json:"outputField"`
	Header      []headerFieldRow    `json:"header,omitempty"`
	Sections    []sectionMappingRow `json:"sections,omitempty"`
}

// Execute builds canonical JSON per the configured header/section mappings
// and writes it to the configured output field.
func (e *MapToCanonicalExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	cfg := mapToCanonicalConfig{OutputField: "parsedCDA"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "parsedCDA"
	}

	canonicalDoc := map[string]interface{}{
		"header":   map[string]interface{}{},
		"sections": map[string]interface{}{},
	}

	for _, h := range cfg.Header {
		applyHeaderField(canonicalDoc, inputData, h)
	}

	sectionsOut := canonicalDoc["sections"].(map[string]interface{})
	for _, sm := range cfg.Sections {
		entries := buildSectionEntries(inputData, sm)
		if len(entries) == 0 {
			continue
		}
		entriesKey := sm.EntriesKey
		if entriesKey == "" {
			entriesKey = "entries"
		}
		// Merge into (rather than overwrite) an existing SectionKey entry —
		// a section with an AlternateEntryArchetype is configured via TWO
		// sectionMappingRows sharing the same SectionKey but different
		// EntriesKey; overwriting would silently drop whichever one ran
		// first (map assignment, not merge).
		secData, ok := sectionsOut[sm.SectionKey].(map[string]interface{})
		if !ok {
			secData = map[string]interface{}{}
			sectionsOut[sm.SectionKey] = secData
		}
		secData[entriesKey] = entries
	}

	durationMs := time.Since(start).Milliseconds()

	outputData := make(map[string]interface{}, len(inputData)+1)
	for k, v := range inputData {
		outputData[k] = v
	}
	executors.UpdateFieldValue(outputData, cfg.OutputField, canonicalDoc)

	log.Printf("  ✅ [cda.map_to_canonical] Mapped %d section(s) into canonical JSON in %dms", len(sectionsOut), durationMs)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{"parsedCDA": canonicalDoc},
		map[string]interface{}{
			"duration_ms":    durationMs,
			"success":        true,
			"sectionCount":   len(sectionsOut),
			"transformation": "map_to_canonical",
		},
	)

	return outputData, nil
}

// applyHeaderField resolves one header row's SourcePath against inputData
// and, if a non-empty value is found, applies Transform (see
// canonical_value_transforms.go) and writes the result to
// canonicalDoc["header"][Group][Target].
//
// One deliberate exception to this step's "flat scalar header fields only"
// scope (see file header comment): if SourcePath already resolves to an
// array (a source system supplying a pre-shaped []{root,extension} list,
// e.g. for patientRole's repeating "ids" — MRN + SSN as two entries), it's
// passed through untransformed rather than dropped by stringifyValue's
// scalar-only type switch. Transform/ValueMap don't apply to a repeating
// value. This unblocks the one repeating header field the requirements
// catalog already treats as SHALL-pre-populatable ("patientId" ->
// headerKeyTranslation -> "ids") — informant/documentationOf/
// encompassingEncounter remain out of scope; those need row-building
// (multiple source rows -> multiple entries), not just an array passthrough.
func applyHeaderField(canonicalDoc map[string]interface{}, inputData map[string]interface{}, h headerFieldRow) {
	if h.Group == "" || h.Target == "" {
		return
	}
	if h.SourcePath == "" {
		if h.LiteralValue == "" {
			return
		}
		executors.UpdateFieldValue(canonicalDoc, "header."+h.Group+"."+h.Target, h.LiteralValue)
		return
	}
	raw := executors.GetFieldValue(inputData, h.SourcePath)
	if arr, ok := raw.([]interface{}); ok {
		if len(arr) > 0 {
			executors.UpdateFieldValue(canonicalDoc, "header."+h.Group+"."+h.Target, arr)
		}
		return
	}
	if s, ok := stringifyValue(raw); ok {
		s = applyCanonicalTransform(h.Transform, s)
		executors.UpdateFieldValue(canonicalDoc, "header."+h.Group+"."+h.Target, s)
	}
}

// buildSectionEntries resolves sm.RowsPath to an array of source rows and
// applies every field mapping to each one, skipping rows that produce no
// mapped fields at all (empty entries would otherwise satisfy cda.build's
// "SHALL section, zero entries" narrative-only path incorrectly). When
// sm.GroupBy is set, delegates to buildGroupedSectionEntries instead — a
// completely separate code path so the (far more common) flat case is
// unaffected in shape or behavior.
func buildSectionEntries(inputData map[string]interface{}, sm sectionMappingRow) []interface{} {
	rows := resolveRows(inputData, sm.RowsPath)
	if len(sm.GroupBy) > 0 {
		return buildGroupedSectionEntries(rows, sm, inputData)
	}
	entries := make([]interface{}, 0, len(rows))
	for _, row := range rows {
		entry := map[string]interface{}{}
		for _, fm := range sm.Fields {
			applyFieldMapping(entry, row, fm, inputData)
		}
		if len(entry) > 0 {
			entries = append(entries, entry)
		}
	}
	return entries
}

// groupKeySeparator joins composite GroupBy values into one internal bucket
// key. Unit Separator (0x1F) is used specifically because it's vanishingly
// unlikely to appear in real source data, unlike a plain "-" or "|" — two
// different (column A, column B) combinations must never collide into the
// same bucket key just because their concatenation happens to match.
const groupKeySeparator = "\x1f"

// buildGroupedSectionEntries is the no-code producer for cda/builder's
// RepeatingGroup loop mechanism: buckets rows by the composite GroupBy key,
// maps each row through the SAME per-row field-mapping table the flat case
// uses (no separate field-mapping UI needed for the common case), and
// collects each bucket's mapped items under sm.GroupedItemsKey — one
// canonical entry per bucket. A row missing one of the GroupBy values
// becomes its own singleton bucket (logged, not silently dropped) rather
// than being excluded — see this function's own group-key resolution below.
func buildGroupedSectionEntries(rows []map[string]interface{}, sm sectionMappingRow, inputData map[string]interface{}) []interface{} {
	type bucket struct {
		items    []interface{}
		firstRow map[string]interface{}
	}
	order := make([]string, 0, len(rows))
	buckets := make(map[string]*bucket, len(rows))
	singletons := 0

	for _, row := range rows {
		key, complete := groupKey(row, sm.GroupBy)
		if !complete {
			singletons++
			key = singletonGroupKey(singletons)
			log.Printf("  ⚠️  [cda.map_to_canonical] section %q: row missing a Group By value — treated as its own group instead of being dropped", sm.SectionKey)
		}
		b, ok := buckets[key]
		if !ok {
			b = &bucket{firstRow: row}
			buckets[key] = b
			order = append(order, key)
		}
		item := map[string]interface{}{}
		for _, fm := range sm.Fields {
			applyFieldMapping(item, row, fm, inputData)
		}
		if len(item) > 0 {
			b.items = append(b.items, item)
		}
	}

	entries := make([]interface{}, 0, len(order))
	for _, key := range order {
		b := buckets[key]
		if len(b.items) == 0 {
			continue
		}
		entry := map[string]interface{}{sm.GroupedItemsKey: b.items}
		for _, ef := range sm.EntryFields {
			applyFieldMapping(entry, b.firstRow, ef, inputData)
		}
		entries = append(entries, entry)
	}
	return entries
}

// groupKey resolves every configured GroupBy column against row and joins
// them into one composite bucket key. complete=false when ANY column is
// missing/empty — the caller treats that row as its own singleton group
// rather than silently excluding it or corrupting an existing group.
func groupKey(row map[string]interface{}, groupBy []string) (key string, complete bool) {
	parts := make([]string, 0, len(groupBy))
	for _, col := range groupBy {
		s, ok := stringifyValue(executors.GetFieldValue(row, col))
		if !ok {
			return "", false
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, groupKeySeparator), true
}

// singletonGroupKey mints a bucket key guaranteed not to collide with any
// real composite GroupBy key (real keys never start with "!singleton" +
// groupKeySeparator — a real column value equal to this exact sentinel
// would require deliberately crafted adversarial input, not realistic
// source data).
func singletonGroupKey(n int) string {
	return "!singleton" + groupKeySeparator + strconv.Itoa(n)
}

func resolveRows(inputData map[string]interface{}, rowsPath string) []map[string]interface{} {
	arr, ok := executors.GetFieldValue(inputData, rowsPath).([]interface{})
	if !ok {
		return nil
	}
	rows := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]interface{}); ok {
			rows = append(rows, m)
		}
	}
	return rows
}

// applyFieldMapping writes fm.CanonicalField into entry from row, trying
// SourcePath, then each FallbackPaths entry in order, then LiteralValue —
// the same "first present value wins" convention
// services/cda_fhir MappingRow.FallbackPaths already uses. Once a raw value
// is found, Transform runs first (normalizing e.g. a source date into CDA's
// TS format — see canonical_value_transforms.go), then ValueMap translates
// the (possibly transformed) result — matching fhir_bundle_adapter.go's
// kindCodeValue behavior, just with a normalization step ahead of it.
func applyFieldMapping(entry map[string]interface{}, row map[string]interface{}, fm fieldMappingRow, inputData map[string]interface{}) {
	if fm.CanonicalField == "" {
		return
	}
	if fm.RelatedRows != nil {
		// Cross-table join (Block 3, Option C) — replaces ALL other
		// resolution. Checked before Condition: a field is either a join or
		// a scalar/branching field, never both.
		if items := resolveRelatedRows(row, fm.RelatedRows, inputData); len(items) > 0 {
			entry[fm.CanonicalField] = items
		}
		return
	}
	if fm.Condition != nil {
		if s, ok := stringifyValue(executors.GetFieldValue(row, fm.Condition.WhenPath)); ok && s == fm.Condition.Equals {
			// Explicit branch match — replaces normal resolution entirely,
			// including "write nothing" when ThenLiteralValue is empty.
			// Never falls through to SourcePath/FallbackPaths/LiteralValue.
			if fm.Condition.ThenLiteralValue != "" {
				entry[fm.CanonicalField] = mapValue(applyCanonicalTransform(fm.Condition.ThenTransform, fm.Condition.ThenLiteralValue), fm.ValueMap)
			}
			return
		}
	}
	if s, ok := stringifyValue(executors.GetFieldValue(row, fm.SourcePath)); ok {
		entry[fm.CanonicalField] = mapValue(applyCanonicalTransform(fm.Transform, s), fm.ValueMap)
		return
	}
	for _, fp := range fm.FallbackPaths {
		if s, ok := stringifyValue(executors.GetFieldValue(row, fp)); ok {
			entry[fm.CanonicalField] = mapValue(applyCanonicalTransform(fm.Transform, s), fm.ValueMap)
			return
		}
	}
	if fm.LiteralValue != "" {
		entry[fm.CanonicalField] = mapValue(applyCanonicalTransform(fm.Transform, fm.LiteralValue), fm.ValueMap)
	}
}

// resolveRelatedRows implements the cross-table join: resolves rr's OTHER
// pipeline array (relative to inputData, NOT row), keeps only the rows whose
// JoinRelatedKey value matches row's own JoinLocalKey value, and maps each
// match through rr.Fields into one item — the SAME applyFieldMapping
// primitive every other field mapping uses, so a joined row can itself use
// Transform/ValueMap/Condition, just not ANOTHER nested RelatedRows (no
// multi-hop joins in this first pass — one level is what every real case
// identified so far needs). Returns nil (not an empty non-nil slice) when
// row has no local key value or nothing matches — the caller only writes
// the field when len > 0, so a medication with zero indications simply
// doesn't get an "indications" key at all, matching the section's own 0..*
// cardinality.
func resolveRelatedRows(row map[string]interface{}, rr *relatedRowsMapping, inputData map[string]interface{}) []interface{} {
	localKeyVal, ok := stringifyValue(executors.GetFieldValue(row, rr.JoinLocalKey))
	if !ok {
		return nil
	}
	relatedRows := resolveRows(inputData, rr.RelatedRowsPath)
	items := make([]interface{}, 0)
	for _, relRow := range relatedRows {
		relKeyVal, ok := stringifyValue(executors.GetFieldValue(relRow, rr.JoinRelatedKey))
		if !ok || relKeyVal != localKeyVal {
			continue
		}
		item := map[string]interface{}{}
		for _, fm := range rr.Fields {
			applyFieldMapping(item, relRow, fm, inputData)
		}
		if len(item) > 0 {
			items = append(items, item)
		}
	}
	return items
}

func mapValue(s string, vm map[string]string) string {
	if vm == nil {
		return s
	}
	if mapped, ok := vm[s]; ok {
		return mapped
	}
	return s
}

// stringifyValue coerces a resolved field value (string/float64/bool from a
// JSON-shaped source row) to its canonical string form, returning ok=false
// for nil/empty — the same "skip, don't write empty" convention
// entry_archetypes.go's stringValue uses on the CDA-building side.
func stringifyValue(v interface{}) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", false
	case string:
		if t == "" {
			return "", false
		}
		return t, true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(t), true
	default:
		return "", false
	}
}

// Validate checks step configuration. Mapping rows are all optional
// (a step with neither header nor sections configured is a no-op, not an
// error — matching cda.build/cda.to_fhir's own permissive Validate).
func (e *MapToCanonicalExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the canonical JSON output for the field picker.
func (e *MapToCanonicalExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Canonical CDA JSON", Path: "parsedCDA", DataType: "object",
			Description: "USCDI-keyed canonical JSON (header + sections), ready for cda.build", Category: "CDA Transform"},
	}
}
