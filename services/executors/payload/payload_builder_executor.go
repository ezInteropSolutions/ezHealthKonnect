// services/executors/payload/payload_builder_executor.go
// Payload Builder — constructs the final outbound wire payload from pipeline data.
//
// Three modes:
//   pass_through  — resolves a single source path and serializes it
//   template      — {{ variable }} substitution in a JSON/HL7 template string
//   field_builder — builds an object from a list of target→source mappings
//
// Output step_output:
//   payload       string  — the wire-format string to send
//   content_type  string  — MIME type
//   payload_size  int     — bytes
//   output_format string  — fhir_r4 | hl7v2 | json | csv

package payload

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	outboundformat "ezhealthkonnect/services/executors/format"
)

// PayloadBuilderExecutor constructs final wire payloads from pipeline data.
type PayloadBuilderExecutor struct {
	*executors.BaseExecutor
}

// NewPayloadBuilderExecutor creates a new PayloadBuilderExecutor.
func NewPayloadBuilderExecutor() *PayloadBuilderExecutor {
	return &PayloadBuilderExecutor{
		BaseExecutor: executors.NewBaseExecutor("payload.builder", models.ExecutorMetadata{
			Name:        "Payload Builder",
			Description: "Constructs the final outbound payload from pipeline data. Supports pass-through, template, and field-builder modes.",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Output",
		}),
	}
}

// payloadBuilderConfig is the config stored in transformation_steps.config.
type payloadBuilderConfig struct {
	Mode          string         `json:"mode"`           // pass_through | template | field_builder
	SourcePath    string         `json:"source_path"`    // pass_through: pipeline variable path
	OutputFormat  string         `json:"output_format"`  // fhir_r4 | hl7v2 | json | csv
	ContentType   string         `json:"content_type"`   // MIME type override
	Template      string         `json:"template"`       // template mode: the template string
	FieldMappings []fieldMapping `json:"field_mappings"` // field_builder mode
}

type fieldMapping struct {
	Target     string `json:"target"`      // dot-path in output object e.g. "entry[0].resource.id"
	SourceType string `json:"source_type"` // "literal" | "field_ref"
	Value      string `json:"value"`       // for literal
	Source     string `json:"source"`      // for field_ref: pipeline variable path
}

// Execute runs the payload builder step.
func (e *PayloadBuilderExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	cfg := payloadBuilderConfig{
		Mode:         "pass_through",
		OutputFormat: "json",
	}
	if step.Config != nil {
		b, _ := json.Marshal(step.Config)
		_ = json.Unmarshal(b, &cfg)
	}

	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	var (
		payload     string
		contentType string
		err         error
	)

	switch cfg.Mode {
	case "pass_through", "":
		payload, contentType, err = e.buildPassThrough(inputData, cfg)
	case "template":
		payload, contentType, err = e.buildTemplate(inputData, cfg)
	case "field_builder":
		payload, contentType, err = e.buildFromFieldMappings(inputData, cfg)
	default:
		payload, contentType, err = e.buildPassThrough(inputData, cfg)
	}

	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		log.Printf("  ❌ [PayloadBuilder] %s mode failed: %v", cfg.Mode, err)
		outputData["_stepOutput"] = map[string]interface{}{
			"payload":       "",
			"content_type":  contentType,
			"payload_size":  0,
			"output_format": cfg.OutputFormat,
			"error":         err.Error(),
		}
		return outputData, err
	}

	log.Printf("  ✅ [PayloadBuilder] mode=%s format=%s size=%d bytes (%dms)",
		cfg.Mode, cfg.OutputFormat, len(payload), durationMs)

	outputData["_stepOutput"] = map[string]interface{}{
		"payload":       payload,
		"content_type":  contentType,
		"payload_size":  len(payload),
		"output_format": cfg.OutputFormat,
	}
	// Also expose at top level so contentField="payload" works for downstream connectors.
	outputData["payload"] = payload
	outputData["content_type"] = contentType

	return outputData, nil
}

// ── Pass-through ──────────────────────────────────────────────────────────────

func (e *PayloadBuilderExecutor) buildPassThrough(
	inputData map[string]interface{},
	cfg payloadBuilderConfig,
) (string, string, error) {

	sourcePath := cfg.SourcePath
	if sourcePath == "" {
		// Smart default: try fhirBundle, fhir_bundle, then full message envelope.
		for _, candidate := range []string{"fhirBundle", "fhir_bundle", "message"} {
			if v := executors.GetFieldValue(inputData, candidate); v != nil {
				sourcePath = candidate
				break
			}
		}
	}
	if sourcePath == "" {
		sourcePath = "message"
	}

	raw := executors.GetFieldValue(inputData, sourcePath)
	if raw == nil {
		// Fallback: check inside message envelope.
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			raw = executors.GetFieldValue(msg, sourcePath)
		}
	}
	if raw == nil {
		return "", "application/json", fmt.Errorf("source path %q resolved to nil", sourcePath)
	}

	// If already a string — send as-is.
	if s, ok := raw.(string); ok {
		ct := resolveContentType(cfg)
		return s, ct, nil
	}

	body, ct, err := outboundformat.SerializeForOutput(raw, normalizeFormat(cfg.OutputFormat))
	if cfg.ContentType != "" {
		ct = cfg.ContentType
	}
	return body, ct, err
}

// ── Template mode ─────────────────────────────────────────────────────────────
// Substitutes {{ variable.path }} placeholders with resolved pipeline values.

var templateVarRe = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

func (e *PayloadBuilderExecutor) buildTemplate(
	inputData map[string]interface{},
	cfg payloadBuilderConfig,
) (string, string, error) {

	if cfg.Template == "" {
		return "", "application/json", fmt.Errorf("template is empty")
	}

	result := templateVarRe.ReplaceAllStringFunc(cfg.Template, func(match string) string {
		inner := templateVarRe.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		path := strings.TrimSpace(inner[1])
		val := executors.GetFieldValue(inputData, path)
		if val == nil {
			if msg, ok := inputData["message"].(map[string]interface{}); ok {
				val = executors.GetFieldValue(msg, path)
			}
		}
		if val == nil {
			return "" // replace with empty string — don't blow up
		}
		switch v := val.(type) {
		case string:
			return v
		case map[string]interface{}, []interface{}:
			b, err := json.Marshal(v)
			if err != nil {
				return fmt.Sprintf("%v", v)
			}
			return string(b)
		default:
			return fmt.Sprintf("%v", v)
		}
	})

	ct := resolveContentType(cfg)
	return result, ct, nil
}

// ── Field builder mode ────────────────────────────────────────────────────────
// Builds a map[string]interface{} from field mappings, then serializes.

func (e *PayloadBuilderExecutor) buildFromFieldMappings(
	inputData map[string]interface{},
	cfg payloadBuilderConfig,
) (string, string, error) {

	if len(cfg.FieldMappings) == 0 {
		return "", "application/json", fmt.Errorf("field_mappings is empty")
	}

	output := make(map[string]interface{})

	for _, fm := range cfg.FieldMappings {
		var val interface{}
		switch fm.SourceType {
		case "literal":
			val = fm.Value
		case "field_ref":
			val = executors.GetFieldValue(inputData, fm.Source)
			if val == nil {
				if msg, ok := inputData["message"].(map[string]interface{}); ok {
					val = executors.GetFieldValue(msg, fm.Source)
				}
			}
		default:
			val = fm.Value
		}
		setNestedValue(output, fm.Target, val)
	}

	body, ct, err := outboundformat.SerializeForOutput(output, normalizeFormat(cfg.OutputFormat))
	if cfg.ContentType != "" {
		ct = cfg.ContentType
	}
	return body, ct, err
}

// setNestedValue sets a value in a nested map using a dot-path (e.g. "entry[0].resource.id").
// Handles simple dot notation; array notation strips index for v1 (treated as plain key).
func setNestedValue(obj map[string]interface{}, path string, val interface{}) {
	parts := strings.SplitN(path, ".", 2)
	key := parts[0]
	// Strip array index notation for now — v1 treats as simple key.
	if idx := strings.Index(key, "["); idx >= 0 {
		key = key[:idx]
	}
	if len(parts) == 1 {
		obj[key] = val
		return
	}
	sub, ok := obj[key].(map[string]interface{})
	if !ok {
		sub = make(map[string]interface{})
		obj[key] = sub
	}
	setNestedValue(sub, parts[1], val)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func normalizeFormat(f string) string {
	switch strings.ToLower(f) {
	case "fhir_r4", "fhir-r4", "fhir":
		return "fhir"
	case "hl7v2", "hl7":
		return "hl7v2"
	case "csv":
		return "csv"
	default:
		return "json"
	}
}

func resolveContentType(cfg payloadBuilderConfig) string {
	if cfg.ContentType != "" {
		return cfg.ContentType
	}
	switch normalizeFormat(cfg.OutputFormat) {
	case "fhir":
		return "application/fhir+json"
	case "hl7v2":
		return "application/hl7-v2"
	case "csv":
		return "text/csv"
	default:
		return "application/json"
	}
}

// Validate satisfies the StepExecutor interface. All modes have safe defaults.
func (e *PayloadBuilderExecutor) Validate(step *models.TransformationStep) error {
	return nil
}
