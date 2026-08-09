// services/executors/transform/hl7_build_executor.go
// HL7BuildExecutor — pipeline step type "hl7.build".
//
// The no-code, format-agnostic on-ramp for a complete HL7 v2 message: maps
// CSV columns, DB query columns, or arbitrary JSON fields directly onto HL7
// segment/field/component paths (e.g. "PID.3", "PID.5.1"), the HL7-side
// mirror of fhir.build's role for FHIR resources — no separate serialize
// step, since hl7/builder.BuildMessage produces the final wire-format string
// directly from the configured mappings.
//
// MSH is always auto-populated (encoding characters, timestamp, control ID,
// message type) from top-level config, not a user-configured segment — see
// hl7/builder.MSHConfig's own doc comment for why it's structurally special.
// Every other segment is configured explicitly, in the order it should
// appear in the message, with "single" (appears once) or "repeating"
// (one instance per source row, via rowsPath) cardinality.
//
// Row/field resolution reuses this package's own stringifyValue/mapValue/
// resolveRows helpers (already shared by map_to_canonical_executor.go) — no
// new path-resolution mechanism. Value shaping is ValueMap only (no named
// transform registry): HL7 fields are flat strings, the same "plain value
// translation covers the realistic no-code need" call
// map_to_canonical_executor.go originally made before CDA-specific transforms
// were added — a transform registry can be added later if a real gap
// surfaces, not invented speculatively here.
//
// Config keys:
//
//	messageType          — e.g. "ADT" (required)
//	triggerEvent         — e.g. "A01"
//	version              — HL7 version, e.g. "2.5.1" (default: "2.5.1")
//	outputField          — dot-path to write the HL7 message (default: "hl7Message")
//	sendingApplication   — MSH.3 (default: "ezHealthKonnect")
//	sendingFacility      — MSH.4 (default: "EHK")
//	receivingApplication — MSH.5
//	receivingFacility    — MSH.6
//	processingId         — MSH.11 (default: "P")
//	segments             — []{segment, cardinality: "single"|"repeating",
//	                       rowsPath? (repeating only), groupBy? ([]string,
//	                       buckets rowsPath's rows before building one
//	                       instance per bucket instead of per row),
//	                       groupedItemsKey? (default "_rows", names the key a
//	                       GroupBy bucket's own rows are exposed under to
//	                       childSegments), condition?, fields: []{fieldKey,
//	                       sourcePath, fallbackPaths?, valueMap?,
//	                       literalValue?, condition?}, childSegments?
//	                       ([]segment-shaped, built immediately after each
//	                       instance of this segment — e.g. OBX nested under
//	                       OBR so each order's own results are interleaved
//	                       right after it, not appended as one big OBX block
//	                       after every OBR)}
package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/hl7"
	hl7builder "ezhealthkonnect/hl7/builder"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// HL7BuildExecutor builds a complete HL7 v2 message from configured no-code
// segment/field mappings against arbitrary upstream row data.
type HL7BuildExecutor struct {
	*executors.BaseExecutor
}

// NewHL7BuildExecutor constructs the executor. Deliberately holds no cached
// schema loader — hl7.GetRealSchemaLoader() is fetched fresh inside Execute()
// (see the guard there for why).
func NewHL7BuildExecutor() *HL7BuildExecutor {
	return &HL7BuildExecutor{
		BaseExecutor: executors.NewBaseExecutor("hl7.build", models.ExecutorMetadata{
			Name:        "HL7 v2 Message Builder",
			Description: "No-code field mapping from CSV/DB/generic-JSON rows into a complete HL7 v2 message",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "HL7 Transform",
		}),
	}
}

// hl7FieldMappingRow maps one HL7 field/component key (fully-qualified, e.g.
// "PID.5.1" — see hl7/builder.Segment.Set) to a source path relative to the
// row being applied (inputData for a "single" segment, one repeating-segment
// row for a "repeating" one). A separate type from map_to_canonical's
// fieldMappingRow (not reused across packages): the two rows describe
// different target vocabularies (HL7 field keys vs. canonical CDA field
// keys) that may evolve independently.
type hl7FieldMappingRow struct {
	FieldKey      string                 `json:"fieldKey"`
	SourcePath    string                 `json:"sourcePath"`
	FallbackPaths []string               `json:"fallbackPaths,omitempty"`
	ValueMap      map[string]string      `json:"valueMap,omitempty"`
	LiteralValue  string                 `json:"literalValue,omitempty"`
	// Condition, when set, is an executors.EvaluateCondition {field,
	// operator, value|compareToField} map — the same shape/evaluator
	// control.if_then_else and control.switch_case already use. When
	// present and it evaluates false, this field is skipped entirely (same
	// effect as its source resolving empty with no fallback/literal) — lets
	// a field be populated only when some OTHER field/business condition
	// holds (e.g. only write PID.19 when message.country == "US"), which
	// plain source-path resolution can't express.
	Condition map[string]interface{} `json:"condition,omitempty"`
}

// hl7SegmentConfig configures one non-MSH segment: either a single instance
// (fields resolve against the whole inputData) or one instance per row found
// at RowsPath (fields resolve against each row) — mirrors
// map_to_canonical_executor.go's sectionMappingRow/fhir.build's
// fhirRepeatingGroup "one sub-object per row" shape for the same reason:
// independent per-field row resolution can't guarantee multiple fields from
// the same source row stay aligned across repeats.
type hl7SegmentConfig struct {
	Segment     string `json:"segment"`
	Cardinality string `json:"cardinality"` // "single" (default) | "repeating"
	RowsPath    string `json:"rowsPath,omitempty"`
	// GroupBy, when set on a "repeating" segment, buckets RowsPath's rows by
	// these column(s) (dot-paths relative to each row) BEFORE building one
	// instance per bucket instead of one per row — e.g. group a flat CSV of
	// (orderId, testName, analyte, value) rows by "orderId" so each distinct
	// lab order becomes one OBR instance instead of one per analyte row.
	// This segment's own Fields resolve against each bucket's FIRST row (the
	// order-level columns, e.g. testName) — the exact "fields resolve from
	// the bucket's first row" convention map_to_canonical_executor.go's
	// buildGroupedSectionEntries already established for the identical
	// problem in CDA sections; bucketing itself reuses that file's own
	// groupKey/singletonGroupKey helpers (same package), not a reimplementation.
	GroupBy []string `json:"groupBy,omitempty"`
	// GroupedItemsKey names the key under which a bucket's own rows are
	// exposed to ChildSegments when GroupBy is set (default "_rows" when
	// empty) — a child segment references them via its own RowsPath set to
	// this same name (e.g. an OBX child under a GroupBy'd OBR would set
	// rowsPath: "_rows"). Ignored when GroupBy is empty.
	GroupedItemsKey string `json:"groupedItemsKey,omitempty"`
	// Condition, when set, gates whether THIS segment instance is built at
	// all — evaluated once against inputData for a "single" segment, or once
	// per row/bucket for a "repeating" one (so an individual row, e.g. one
	// OBX result, can be skipped even though other rows in the same array
	// are kept). Same shape/evaluator as hl7FieldMappingRow.Condition.
	Condition map[string]interface{} `json:"condition,omitempty"`
	Fields    []hl7FieldMappingRow    `json:"fields"`
	// ChildSegments are built immediately after each instance of THIS
	// segment — interleaved in the output right after their parent, not
	// after every root segment finishes building — which is what makes e.g.
	// "OBR followed by its own OBX results, before the next OBR" possible; a
	// flat list of independently-looped segments can't express that
	// ordering at all. A child's RowsPath/Condition/Fields resolve against
	// the CURRENT parent instance's row (or, when the parent used GroupBy,
	// that bucket's first row plus its GroupedItemsKey — see buildSegmentTree),
	// with fallback to top-level inputData for anything not found there —
	// the same conditionMet/mergeWithFallback convention every other
	// row/topLevel resolution in this file already uses.
	ChildSegments []hl7SegmentConfig `json:"childSegments,omitempty"`
}

// groupedItemsKey returns the configured GroupedItemsKey, or "_rows" when
// unset — see hl7SegmentConfig.GroupedItemsKey's doc comment.
func (sc hl7SegmentConfig) groupedItemsKey() string {
	if sc.GroupedItemsKey != "" {
		return sc.GroupedItemsKey
	}
	return "_rows"
}

type hl7BuildConfig struct {
	MessageType          string             `json:"messageType"`
	TriggerEvent         string             `json:"triggerEvent"`
	Version              string             `json:"version"`
	OutputField          string             `json:"outputField"`
	SendingApplication   string             `json:"sendingApplication"`
	SendingFacility      string             `json:"sendingFacility"`
	ReceivingApplication string             `json:"receivingApplication"`
	ReceivingFacility    string             `json:"receivingFacility"`
	ProcessingID         string             `json:"processingId"`
	Segments             []hl7SegmentConfig `json:"segments"`
}

// Execute builds the HL7 message per the configured MSH/segment mappings and
// writes it to the configured output field.
func (e *HL7BuildExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	cfg := hl7BuildConfig{Version: "2.5.1", OutputField: "hl7Message"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.Version == "" {
		cfg.Version = "2.5.1"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "hl7Message"
	}
	if cfg.MessageType == "" {
		return nil, fmt.Errorf("hl7.build: messageType is required")
	}

	// hl7.GetRealSchemaLoader() is fetched fresh on every Execute() call,
	// never cached at construction: main.go's schema-init call sites don't
	// guarantee ordering relative to executor construction (see
	// fhir.build's identical lazy-fetch discipline for r4.GetRegistry(),
	// applied here for the same reason).
	loader := hl7.GetRealSchemaLoader()
	if loader == nil {
		return nil, fmt.Errorf("hl7.build: HL7 schema loader not yet initialized")
	}
	if _, err := loader.LoadRealSchema(cfg.Version, cfg.MessageType, cfg.TriggerEvent); err != nil {
		return nil, fmt.Errorf("hl7.build: unknown messageType/triggerEvent/version %q/%q/%q: %w", cfg.MessageType, cfg.TriggerEvent, cfg.Version, err)
	}

	msh := hl7builder.MSHConfig{
		MessageType:          cfg.MessageType,
		TriggerEvent:         cfg.TriggerEvent,
		Version:              cfg.Version,
		SendingApplication:   cfg.SendingApplication,
		SendingFacility:      cfg.SendingFacility,
		ReceivingApplication: cfg.ReceivingApplication,
		ReceivingFacility:    cfg.ReceivingFacility,
		ProcessingID:         cfg.ProcessingID,
	}

	var segments []*hl7builder.Segment
	for _, sc := range cfg.Segments {
		segments = append(segments, e.buildSegmentTree(inputData, inputData, sc)...)
	}

	hl7Message := hl7builder.BuildMessage(msh, segments, hl7builder.BuildOptions{})

	durationMs := time.Since(start).Milliseconds()

	outputData := make(map[string]interface{}, len(inputData)+1)
	for k, v := range inputData {
		outputData[k] = v
	}
	executors.UpdateFieldValue(outputData, cfg.OutputField, hl7Message)

	log.Printf("  ✅ [hl7.build] Built %s^%s message (%d segment(s)) in %dms",
		cfg.MessageType, cfg.TriggerEvent, len(segments), durationMs)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{"hl7Message": hl7Message},
		map[string]interface{}{
			"duration_ms":    durationMs,
			"success":        true,
			"messageType":    cfg.MessageType,
			"triggerEvent":   cfg.TriggerEvent,
			"segmentCount":   len(segments),
			"transformation": "hl7_build",
		},
	)

	return outputData, nil
}

// buildSegmentTree produces sc's own segment instance(s) AND, immediately
// after each one, that instance's ChildSegments (see hl7SegmentConfig's own
// doc comment for why interleaving — not "all of sc, then all children" —
// matters). context is the row/bucket this call's RowsPath/Condition/Fields
// resolve against: inputData for a root segment; the parent's current row
// (or bucket-with-GroupedItemsKey, when the parent used GroupBy) for a
// nested one. topLevel is always the original inputData, used only as the
// Condition fallback (see conditionMet) — it never changes as recursion
// descends, unlike context.
//
// Cardinality/skip rules are unchanged from before ChildSegments existed:
// "single" is always emitted, even with every field empty; a "repeating"
// row/bucket producing zero mapped fields is skipped (and its children never
// get a chance to build) — matching map_to_canonical_executor.go's
// buildSectionEntries: an empty entry would misrepresent real repeated data.
func (e *HL7BuildExecutor) buildSegmentTree(context, topLevel map[string]interface{}, sc hl7SegmentConfig) []*hl7builder.Segment {
	if sc.Segment == "" {
		return nil
	}
	isRepeating := sc.Cardinality == "repeating"

	buildOne := func(row map[string]interface{}) []*hl7builder.Segment {
		if !e.conditionMet(sc.Condition, row, topLevel) {
			return nil
		}
		seg := hl7builder.NewSegment(sc.Segment)
		wroteAny := false
		for _, f := range sc.Fields {
			if e.applyHL7Field(seg, row, topLevel, f) {
				wroteAny = true
			}
		}
		if isRepeating && !wroteAny {
			return nil
		}
		out := []*hl7builder.Segment{seg}
		for _, child := range sc.ChildSegments {
			out = append(out, e.buildSegmentTree(row, topLevel, child)...)
		}
		return out
	}

	if !isRepeating {
		return buildOne(context)
	}

	rows := resolveRows(context, sc.RowsPath)

	if len(sc.GroupBy) == 0 {
		var out []*hl7builder.Segment
		for _, row := range rows {
			out = append(out, buildOne(row)...)
		}
		return out
	}

	var out []*hl7builder.Segment
	for _, b := range bucketHL7Rows(rows, sc.GroupBy) {
		bucketContext := mergeWithFallback(b.firstRow, map[string]interface{}{
			sc.groupedItemsKey(): rowsToInterfaceSlice(b.rows),
		})
		out = append(out, buildOne(bucketContext)...)
	}
	return out
}

// hl7RowBucket is one GroupBy bucket: every row sharing that composite key,
// plus the first one seen (the source for the parent instance's own Fields —
// see hl7SegmentConfig.GroupBy's doc comment).
type hl7RowBucket struct {
	rows     []map[string]interface{}
	firstRow map[string]interface{}
}

// bucketHL7Rows groups rows by groupBy, preserving first-seen bucket order
// (so e.g. a CBC order appearing before a CMP order in the source data stays
// first in the output) — reuses map_to_canonical_executor.go's own
// groupKey/singletonGroupKey (same package) rather than a second bucketing
// implementation; a row missing a GroupBy value becomes its own singleton
// bucket instead of being silently dropped, matching that file's rule.
func bucketHL7Rows(rows []map[string]interface{}, groupBy []string) []*hl7RowBucket {
	order := make([]string, 0, len(rows))
	buckets := make(map[string]*hl7RowBucket, len(rows))
	singletons := 0
	for _, row := range rows {
		key, complete := groupKey(row, groupBy)
		if !complete {
			singletons++
			key = singletonGroupKey(singletons)
		}
		b, ok := buckets[key]
		if !ok {
			b = &hl7RowBucket{firstRow: row}
			buckets[key] = b
			order = append(order, key)
		}
		b.rows = append(b.rows, row)
	}
	out := make([]*hl7RowBucket, 0, len(order))
	for _, key := range order {
		out = append(out, buckets[key])
	}
	return out
}

// rowsToInterfaceSlice converts resolveRows' own []map[string]interface{}
// back into the []interface{} shape executors.GetFieldValue/resolveRows
// expect when a ChildSegment re-resolves this bucket's rows via RowsPath.
func rowsToInterfaceSlice(rows []map[string]interface{}) []interface{} {
	out := make([]interface{}, len(rows))
	for i, r := range rows {
		out[i] = r
	}
	return out
}

// conditionMet reports whether condition holds against source (the current
// row for a repeating segment, or inputData for a single one), falling back
// to topLevel for anything condition's field/compareToField doesn't find in
// source — so a repeating OBX row's condition can check either its own
// column (e.g. row.resultStatus) or a top-level field (e.g.
// message.country) without the config needing to know which. A nil/empty
// condition always holds (no condition configured = always build/populate,
// today's existing behavior). Evaluation errors (e.g. a malformed regex) are
// logged and treated as not-met — a broken condition should suppress the
// segment/field, not crash the whole message build.
func (e *HL7BuildExecutor) conditionMet(condition map[string]interface{}, source, topLevel map[string]interface{}) bool {
	if len(condition) == 0 {
		return true
	}
	met, err := executors.EvaluateCondition(condition, mergeWithFallback(source, topLevel))
	if err != nil {
		log.Printf("  ⚠️  [hl7.build] condition evaluation failed, treating as not met: %v", err)
		return false
	}
	return met
}

// mergeWithFallback returns a map where primary's keys win, and fallback's
// keys fill in anything primary doesn't have — the "row data with fallback
// to top-level" convention conditionMet needs. Cheap even when primary IS
// topLevel (the "single" segment case): merging a map with itself is a
// harmless no-op copy, not worth special-casing away.
func mergeWithFallback(primary, fallback map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{}, len(fallback)+len(primary))
	for k, v := range fallback {
		merged[k] = v
	}
	for k, v := range primary {
		merged[k] = v
	}
	return merged
}

// applyHL7Field resolves fm's value against source, trying SourcePath, then
// each FallbackPaths entry, then LiteralValue — the same "first present
// value wins" convention map_to_canonical_executor.go's applyFieldMapping
// uses — then writes the (ValueMap-translated) result into seg at
// fm.FieldKey. Returns whether a value was actually written, so
// buildSegments can tell a genuinely empty repeating row from one that
// wrote real data. fm.Condition (when set) is checked first, against source
// with fallback to topLevel — see conditionMet.
func (e *HL7BuildExecutor) applyHL7Field(seg *hl7builder.Segment, source, topLevel map[string]interface{}, fm hl7FieldMappingRow) bool {
	if fm.FieldKey == "" {
		return false
	}
	if !e.conditionMet(fm.Condition, source, topLevel) {
		return false
	}
	if s, ok := stringifyValue(executors.GetFieldValue(source, fm.SourcePath)); ok {
		return e.setSegmentField(seg, fm.FieldKey, mapValue(s, fm.ValueMap))
	}
	for _, fp := range fm.FallbackPaths {
		if s, ok := stringifyValue(executors.GetFieldValue(source, fp)); ok {
			return e.setSegmentField(seg, fm.FieldKey, mapValue(s, fm.ValueMap))
		}
	}
	if fm.LiteralValue != "" {
		return e.setSegmentField(seg, fm.FieldKey, mapValue(fm.LiteralValue, fm.ValueMap))
	}
	return false
}

// setSegmentField writes value at fieldKey, logging (not failing the whole
// step) on a malformed fieldKey — a single bad mapping in a no-code UI
// shouldn't take down the entire message build.
func (e *HL7BuildExecutor) setSegmentField(seg *hl7builder.Segment, fieldKey, value string) bool {
	if err := seg.Set(fieldKey, value); err != nil {
		log.Printf("  ⚠️  [hl7.build] %v", err)
		return false
	}
	return true
}

// Validate checks step configuration. Segment/field rows are all optional
// (a step with none configured still produces a bare MSH-only message, not
// an error) — matching cda.build/cda.map_to_canonical/fhir.build's own
// permissive Validate.
func (e *HL7BuildExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the built message for the field picker.
func (e *HL7BuildExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "HL7 Message", Path: "hl7Message", DataType: "string",
			Description: "HL7 v2 message built from configured segment/field mappings", Category: "HL7 Transform"},
	}
}
