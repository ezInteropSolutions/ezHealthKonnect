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
//	                       rowsPath? (repeating only), fields: []{fieldKey,
//	                       sourcePath, fallbackPaths?, valueMap?, literalValue?}}
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
	FieldKey      string            `json:"fieldKey"`
	SourcePath    string            `json:"sourcePath"`
	FallbackPaths []string          `json:"fallbackPaths,omitempty"`
	ValueMap      map[string]string `json:"valueMap,omitempty"`
	LiteralValue  string            `json:"literalValue,omitempty"`
}

// hl7SegmentConfig configures one non-MSH segment: either a single instance
// (fields resolve against the whole inputData) or one instance per row found
// at RowsPath (fields resolve against each row) — mirrors
// map_to_canonical_executor.go's sectionMappingRow/fhir.build's
// fhirRepeatingGroup "one sub-object per row" shape for the same reason:
// independent per-field row resolution can't guarantee multiple fields from
// the same source row stay aligned across repeats.
type hl7SegmentConfig struct {
	Segment     string               `json:"segment"`
	Cardinality string               `json:"cardinality"` // "single" (default) | "repeating"
	RowsPath    string               `json:"rowsPath,omitempty"`
	Fields      []hl7FieldMappingRow `json:"fields"`
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
		segments = append(segments, e.buildSegments(inputData, sc)...)
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

// buildSegments produces the segment instance(s) sc describes: exactly one
// for "single" cardinality (always emitted, even if every field happened to
// resolve empty — a deliberately-configured non-repeating segment shouldn't
// silently vanish), or one per row at sc.RowsPath for "repeating" cardinality
// (a row producing zero mapped fields is skipped, mirroring
// map_to_canonical_executor.go's buildSectionEntries: empty entries would
// otherwise misrepresent real repeated data).
func (e *HL7BuildExecutor) buildSegments(inputData map[string]interface{}, sc hl7SegmentConfig) []*hl7builder.Segment {
	if sc.Segment == "" {
		return nil
	}

	if sc.Cardinality != "repeating" {
		seg := hl7builder.NewSegment(sc.Segment)
		for _, f := range sc.Fields {
			e.applyHL7Field(seg, inputData, f)
		}
		return []*hl7builder.Segment{seg}
	}

	rows := resolveRows(inputData, sc.RowsPath)
	out := make([]*hl7builder.Segment, 0, len(rows))
	for _, row := range rows {
		seg := hl7builder.NewSegment(sc.Segment)
		wroteAny := false
		for _, f := range sc.Fields {
			if e.applyHL7Field(seg, row, f) {
				wroteAny = true
			}
		}
		if wroteAny {
			out = append(out, seg)
		}
	}
	return out
}

// applyHL7Field resolves fm's value against source, trying SourcePath, then
// each FallbackPaths entry, then LiteralValue — the same "first present
// value wins" convention map_to_canonical_executor.go's applyFieldMapping
// uses — then writes the (ValueMap-translated) result into seg at
// fm.FieldKey. Returns whether a value was actually written, so
// buildSegments can tell a genuinely empty repeating row from one that
// wrote real data.
func (e *HL7BuildExecutor) applyHL7Field(seg *hl7builder.Segment, source map[string]interface{}, fm hl7FieldMappingRow) bool {
	if fm.FieldKey == "" {
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
