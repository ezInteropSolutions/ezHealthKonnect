// services/executors/transform/edi_generate_999_executor.go
// EDIGenerate999Executor — pipeline step type "edi.generate_999".
//
// NOT a hidden, automatic side-effect of connector ingestion — matches this
// project's own explicit composable-pipeline-steps philosophy (and the
// user's own "flexible, not rigid" product direction already governing
// edi.validate's custom rules, see edi/validator's own doc comment). A user
// adds this step explicitly to an inbound-837 pipeline (typically after
// edi.validate) when they want auto-acknowledgment, delivered via the
// existing generic connector.outbound bridge.
//
// Re-parses/validates the ORIGINAL raw content from sourceField — same
// re-parse-from-raw discipline edi.validate itself uses, for the same
// reason: needs the TYPED edi.ParseResult (Fields with their own
// SegmentPosition, SegmentInstances) a prior step's JSON-shaped output
// doesn't carry.
//
// Maps edi/validator.Result.Issues -> IK3 (segment-level error) / IK4
// (element-level error, when the issue traces back to a specific
// EDIField) segments, and computes AK9/IK5's accept/reject code from
// whether any ERROR-severity issue exists — warnings never cause a
// rejection, consistent with the two-severity model edi/validator itself
// establishes. Builds the real 999 X12 text via the SAME edi/builder engine
// every other EDI step uses — zero new build-side Go code needed once the
// 999 schema exists (see edi/schemas/x12_005010/transactionSets/999.json).
//
// Named, deliberate simplifications (not oversights):
//   - CTX (Segment Context) generation is out of scope: it's supplementary
//     business-context detail (e.g. a TRN02/NM109 business-unit pointer)
//     this validator has no source data for — IK3/IK4 alone already
//     produce a genuinely useful, spec-plausible 999.
//   - IK403 (Implementation Data Element Syntax Error Code) is classified
//     from edi.Validate's own free-form Go error MESSAGE text via a small
//     heuristic (classifyElementSyntaxErrorCode) — edi/datatypes.go's
//     Validate has no structured error-code return, so this is a
//     best-effort mapping onto the X12 code table, not an authoritative one.
//   - A SyntaxRule-sourced WARNING's own Issue.Path is a bare segment ID
//     (edi/validator.checkSyntaxRules has no way to disambiguate WHICH
//     occurrence violated the rule when a segment repeats) — resolved to
//     that segment ID's FIRST occurrence position, a named, honest
//     approximation rather than pretending precision the source data
//     doesn't carry.
//   - GS01/GS08 envelope-identity issues (edi/validator.checkEnvelopeIdentity)
//     are excluded from IK3 entirely: a 999 reports on the ST...SE
//     transaction-set BODY, not the surrounding interchange envelope
//     (TA1's own domain, itself out of scope — see edi/builder's own note).
//
// Config keys:
//   sourceField — dot-path to the field holding the ORIGINAL raw EDI content being acknowledged (default: "raw")
//   outputField — dot-path to write the built 999 text (default: "generated999")
//   senderId    — 999's own ISA06/GS02 (default: the original interchange's own receiverId — the 999 goes back FROM whoever received the original message)
//   receiverId  — 999's own ISA08/GS03 (default: the original interchange's own senderId)

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"ezhealthkonnect/edi"
	ediBuilder "ezhealthkonnect/edi/builder"
	"ezhealthkonnect/edi/validator"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// EDIGenerate999Executor builds a real X12 999 Implementation Acknowledgment
// from a re-validated original EDI message. Self-initialises from the
// schema directory — same convention as every other edi.* executor.
type EDIGenerate999Executor struct {
	*executors.BaseExecutor
	loader *edi.X12SchemaLoader
}

// NewEDIGenerate999Executor constructs the executor and loads the EDI
// schema from the default schema directory "./edi/schemas/x12_005010".
func NewEDIGenerate999Executor() *EDIGenerate999Executor {
	exec := &EDIGenerate999Executor{
		BaseExecutor: executors.NewBaseExecutor("edi.generate_999", models.ExecutorMetadata{
			Name:        "EDI X12 999 Generator",
			Description: "Builds a real X12 999 Implementation Acknowledgment from re-validating the original EDI message — errors reject, warnings never do",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "EDI Transform",
		}),
	}

	loader, err := edi.NewX12SchemaLoader("./edi/schemas/x12_005010")
	if err != nil {
		log.Printf("⚠️  [edi.generate_999] EDI schema load failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.loader = loader
	log.Printf("✅ [edi.generate_999] EDI schema loaded from ./edi/schemas/x12_005010")
	return exec
}

type ediGenerate999Config struct {
	SourceField string `json:"sourceField"`
	OutputField string `json:"outputField"`
	SenderID    string `json:"senderId,omitempty"`
	ReceiverID  string `json:"receiverId,omitempty"`
}

// Execute re-parses and re-validates the original raw EDI content, builds a
// real 999 acknowledging it, and writes the built text to outputField.
func (e *EDIGenerate999Executor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}
	if e.loader == nil {
		return nil, fmt.Errorf("edi.generate_999: EDI schema is not initialised (schema directory missing)")
	}

	cfg := ediGenerate999Config{SourceField: "raw", OutputField: "generated999"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "raw"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "generated999"
	}

	rawEDI := ""
	if v := executors.GetFieldValue(inputData, cfg.SourceField); v != nil {
		rawEDI, _ = v.(string)
	}
	if rawEDI == "" {
		if v, ok := inputData["raw"].(string); ok {
			rawEDI = v
		}
	}
	if rawEDI == "" {
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			if v := executors.GetFieldValue(msg, cfg.SourceField); v != nil {
				rawEDI, _ = v.(string)
			}
			if rawEDI == "" {
				if v, ok := msg["raw"].(string); ok {
					rawEDI = v
				}
			}
		}
	}
	if rawEDI == "" {
		return nil, fmt.Errorf("edi.generate_999: source field %q is empty or not a string", cfg.SourceField)
	}

	spec := e.loader.Spec()
	original, err := edi.ParseTransactionSet(spec, rawEDI)
	if err != nil {
		return nil, fmt.Errorf("edi.generate_999: parsing original message: %w", err)
	}
	valResult := validator.Validate(spec, original)

	txSet999 := e.loader.GetTransactionSet("999")
	if txSet999 == nil {
		return nil, fmt.Errorf("edi.generate_999: 999 transaction set not found in loaded schema")
	}
	originalTxSet := spec.TransactionSets[original.TransactionSet]
	if originalTxSet == nil {
		return nil, fmt.Errorf("edi.generate_999: original transaction set %q not found in loaded schema", original.TransactionSet)
	}

	ik3Entries := buildIK3Entries(original, valResult.Issues)

	errorCount := 0
	for _, issue := range valResult.Issues {
		if issue.Severity == "error" {
			errorCount++
		}
	}
	ackCode := "A"
	acceptedCount := "1"
	if errorCount > 0 {
		ackCode = "R"
		acceptedCount = "0"
	}

	senderID := cfg.SenderID
	receiverID := cfg.ReceiverID
	if senderID == "" {
		senderID, _ = original.Interchange["receiverId"].(string) // the 999 goes back FROM whoever received the original
	}
	if receiverID == "" {
		receiverID, _ = original.Interchange["senderId"].(string)
	}

	// original.Interchange only carries functionalIdentifierCode/gsControlNumber/
	// versionReleaseIndustryCode when the original message had a real GS
	// segment to read them from — a bare-ST message (no envelope, a real,
	// common case this project already handles elsewhere, e.g.
	// TestParseTransactionSet_BareSTNoEnvelope) has none. Fall back to the
	// resolved original transaction set's own known identity rather than
	// writing Go's zero-value interface{}(nil) into the canonical map (which
	// would render literally as the string "<nil>" on the wire).
	ak1FunctionalID, _ := original.Interchange["functionalIdentifierCode"].(string)
	if ak1FunctionalID == "" {
		ak1FunctionalID = originalTxSet.FunctionalIdentifierCode
	}
	ak1GroupControl, _ := original.Interchange["gsControlNumber"].(string)
	if ak1GroupControl == "" {
		ak1GroupControl = "1" // no real GS to echo — best-effort placeholder
	}
	ak1VersionCode, _ := original.Interchange["versionReleaseIndustryCode"].(string)
	if ak1VersionCode == "" {
		ak1VersionCode = originalTxSet.VersionReleaseIndustryCode
	}
	ak1 := map[string]interface{}{
		"functionalIdentifierCode":   ak1FunctionalID,
		"groupControlNumber":         ak1GroupControl,
		"versionReleaseIndustryCode": ak1VersionCode,
	}
	ak2 := map[string]interface{}{
		"transactionSetIdentifierCode": originalTxSet.EffectiveST01(),
		"transactionSetControlNumber":  original.Interchange["stControlNumber"],
	}
	if stHeader, ok := original.Header["ST"].(map[string]interface{}); ok {
		if ref, ok := stHeader["implementationConventionReference"]; ok {
			ak2["implementationConventionReference"] = ref
		}
	}

	loop2100 := make([]interface{}, 0, len(ik3Entries))
	for _, entry := range ik3Entries {
		loop2100 = append(loop2100, entry.toLoopInstance())
	}

	buildInput := ediBuilder.BuildInput{
		TransactionSet: "999",
		Interchange:    map[string]interface{}{"senderId": senderID, "receiverId": receiverID},
		Header:         map[string]interface{}{"AK1": ak1},
		Loops: map[string]interface{}{
			"2000": []interface{}{
				map[string]interface{}{
					"AK2":  ak2,
					"loops": map[string]interface{}{"2100": loop2100},
					"IK5":  map[string]interface{}{"transactionSetAcknowledgmentCode": ackCode},
				},
			},
		},
		Trailer: map[string]interface{}{
			"AK9": map[string]interface{}{
				"functionalGroupAcknowledgeCode":  ackCode,
				"numberOfTransactionSetsIncluded": "1",
				"numberOfReceivedTransactionSets": "1",
				"numberOfAcceptedTransactionSets": acceptedCount,
			},
		},
	}

	built, err := ediBuilder.BuildDocument(spec, buildInput)
	if err != nil {
		return nil, fmt.Errorf("edi.generate_999: building 999: %w", err)
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [edi.generate_999] Generated 999 for %s: ackCode=%s, %d error(s), %d IK3 entr(y/ies) in %dms",
		original.TransactionSet, ackCode, errorCount, len(ik3Entries), durationMs)

	outputData := make(map[string]interface{}, len(inputData)+2)
	for k, v := range inputData {
		outputData[k] = v
	}
	executors.SetNestedValue(outputData, cfg.OutputField, built)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{
			cfg.OutputField: built,
			"ackCode":       ackCode,
			"errorCount":    errorCount,
			"ik3Count":      len(ik3Entries),
		},
		map[string]interface{}{
			"duration_ms": durationMs,
			"success":     true,
			"ackCode":     ackCode,
		},
	)

	return outputData, nil
}

// ik3Entry is one segment occurrence's own aggregated error detail — every
// Issue traced back to the SAME (segmentID, position) merges into one IK3
// with multiple nested IK4s, matching real X12 999 semantics (one IK3 per
// erroring segment OCCURRENCE, not one per issue).
type ik3Entry struct {
	SegmentID string
	Position  int
	LoopID    string
	IK4s      []ik4Entry
}

type ik4Entry struct {
	ElementPosition int
	SyntaxErrorCode string
	BadData         string
}

func (e ik3Entry) toLoopInstance() map[string]interface{} {
	ik3 := map[string]interface{}{
		"segmentIdCode":          e.SegmentID,
		"segmentPosition":        strconv.Itoa(e.Position),
		"segmentSyntaxErrorCode": "8", // Segment Has Data Element Errors — the only class of error this executor's own source data (edi/validator.Issue) ever represents
	}
	if e.LoopID != "" {
		ik3["loopIdentifierCode"] = e.LoopID
	}
	instance := map[string]interface{}{"IK3": ik3}
	if len(e.IK4s) > 0 {
		ik4List := make([]interface{}, 0, len(e.IK4s))
		for _, ik4 := range e.IK4s {
			// Fields must be wrapped under an "IK4" key, exactly like IK3's
			// own fields are wrapped under "IK3" above — writeSegmentSequence
			// looks up data["IK4"] specifically (see edi/builder/segment_writer.go),
			// not the field names directly at this map's own top level.
			ik4Fields := map[string]interface{}{
				"implementationDataElementSyntaxErrorCode": ik4.SyntaxErrorCode,
			}
			if ik4.ElementPosition > 0 {
				ik4Fields["positionInSegment"] = map[string]interface{}{"elementPosition": strconv.Itoa(ik4.ElementPosition)}
			}
			if ik4.BadData != "" {
				ik4Fields["copyOfBadDataElement"] = ik4.BadData
			}
			ik4List = append(ik4List, map[string]interface{}{"IK4": ik4Fields})
		}
		instance["loops"] = map[string]interface{}{"2110": ik4List}
	}
	return instance
}

// buildIK3Entries maps validator issues onto IK3/IK4 detail, grouped by
// erroring segment occurrence. See this file's own header comment for the
// named simplifications this mapping makes.
func buildIK3Entries(result *edi.ParseResult, issues []validator.Issue) []ik3Entry {
	firstPositionForSegment := map[string]int{}
	for _, inst := range result.SegmentInstances {
		if _, ok := firstPositionForSegment[inst.SegmentID]; !ok {
			firstPositionForSegment[inst.SegmentID] = inst.Position
		}
	}

	var entries []ik3Entry
	indexOf := map[string]int{} // "segmentID|position" -> index into entries

	for _, issue := range issues {
		if strings.HasPrefix(issue.Path, "GS.") || strings.HasPrefix(issue.Path, "ISA.") {
			continue // envelope-level — a 999 reports on the ST...SE body, not the surrounding interchange
		}

		if field, ok := result.Fields[issue.Path]; ok {
			segID := segmentIDFromEDIFieldPath(field.Path)
			if segID == "" {
				continue
			}
			key := fmt.Sprintf("%s|%d", segID, field.SegmentPosition)
			idx, exists := indexOf[key]
			if !exists {
				entries = append(entries, ik3Entry{SegmentID: segID, Position: field.SegmentPosition, LoopID: loopIDFromEDIFieldPath(field.Path)})
				idx = len(entries) - 1
				indexOf[key] = idx
			}
			elemPos, _ := strconv.Atoi(field.Key)
			entries[idx].IK4s = append(entries[idx].IK4s, ik4Entry{
				ElementPosition: elemPos,
				SyntaxErrorCode: classifyElementSyntaxErrorCode(issue.Message),
				BadData:         field.Value,
			})
			continue
		}

		// SyntaxRule-sourced warning: Issue.Path is a bare segment ID with no
		// occurrence disambiguation available — see this file's own header
		// comment for why the first occurrence is used.
		segID := issue.Path
		pos, known := firstPositionForSegment[segID]
		if !known {
			continue // segment ID unrecognized (shouldn't happen for real data) — skip rather than fabricate a position
		}
		key := fmt.Sprintf("%s|%d", segID, pos)
		if _, exists := indexOf[key]; !exists {
			entries = append(entries, ik3Entry{SegmentID: segID, Position: pos})
			indexOf[key] = len(entries) - 1
		}
	}
	return entries
}

// segmentIDFromEDIFieldPath extracts the segment ID from a flat, loop-
// qualified EDIField.Path (e.g. "2100.CLP.02" or "2000[2].2100[1].CLP.02")
// — the second-to-last dot component, stripped of any "[n]" repeat suffix.
func segmentIDFromEDIFieldPath(path string) string {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return ""
	}
	seg := parts[len(parts)-2]
	if idx := strings.IndexByte(seg, '['); idx >= 0 {
		seg = seg[:idx]
	}
	return seg
}

// loopIDFromEDIFieldPath extracts the immediately-enclosing loop ID, if any
// — the third-to-last dot component, stripped of any "[n]" suffix. Empty
// when the field belongs to the header/trailer (no enclosing loop).
func loopIDFromEDIFieldPath(path string) string {
	parts := strings.Split(path, ".")
	if len(parts) < 3 {
		return ""
	}
	loop := parts[len(parts)-3]
	if idx := strings.IndexByte(loop, '['); idx >= 0 {
		loop = loop[:idx]
	}
	return loop
}

// classifyElementSyntaxErrorCode maps edi.Validate's own free-form Go error
// message onto the closest real X12 Implementation Data Element Syntax
// Error Code (IK403) — a best-effort heuristic, not an authoritative
// mapping (edi/datatypes.go's Validate has no structured error-code return
// to classify on instead). See this file's own header comment.
func classifyElementSyntaxErrorCode(message string) string {
	switch {
	case strings.Contains(message, "shorter than minimum length"):
		return "4" // Data Element Too Short
	case strings.Contains(message, "longer than maximum length"):
		return "5" // Data Element Too Long
	case strings.Contains(message, "must be 8 digits"):
		return "8" // Invalid Date
	case strings.Contains(message, "must be HHMM"):
		return "9" // Invalid Time
	case strings.Contains(message, "expected fixed value"):
		return "7" // Invalid Code Value
	case strings.Contains(message, "not numeric"), strings.Contains(message, "not a valid"):
		return "6" // Invalid Character In Data Element
	default:
		return "7" // Invalid Code Value — safest generic default for "value doesn't satisfy a constraint"
	}
}

// Validate checks step configuration.
func (e *EDIGenerate999Executor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the generated 999 output for the field picker.
func (e *EDIGenerate999Executor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Generated 999", Path: "_stepOutput.generated999", DataType: "string",
			Description: "Complete built 999 Implementation Acknowledgment X12 text", Category: "EDI Transform"},
		{Name: "Acknowledgment code", Path: "_stepOutput.ackCode", DataType: "string",
			Description: "A (Accepted) or R (Rejected) — reflects AK9/IK5; R only when at least one ERROR-severity issue exists", Category: "EDI Transform"},
		{Name: "Error count", Path: "_stepOutput.errorCount", DataType: "number",
			Description: "Count of ERROR-severity issues found on the original message", Category: "EDI Transform"},
		{Name: "IK3 entry count", Path: "_stepOutput.ik3Count", DataType: "number",
			Description: "Count of distinct segment occurrences with at least one reported issue", Category: "EDI Transform"},
	}
}
