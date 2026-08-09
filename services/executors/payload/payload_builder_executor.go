// services/executors/payload/payload_builder_executor.go
// Payload Builder — constructs the final outbound wire payload from pipeline data.
//
// Modes:
//   pass_through  — resolves a single source path and serializes it
//   template      — {{ variable }} substitution in a JSON/HL7 template string
//   field_builder — builds an object from a list of target→source mappings
//   fhir_bundle   — wraps one or more FHIR resources into a FHIR Bundle
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

	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	outboundformat "ezhealthkonnect/services/executors/format"
	fhirnarrative "ezhealthkonnect/services/fhir_narrative"
)

// fhirBundleConfig holds config specific to the fhir_bundle mode.
type fhirBundleConfig struct {
	// BundleType controls the FHIR Bundle.type field.
	// Supported: "collection" (default) | "transaction" | "batch" | "document"
	BundleType string `json:"bundleType"`

	// ResourcePaths is an ordered list of pipeline variable paths.
	// Each path must resolve to a map[string]interface{} FHIR resource.
	// Example: ["fhir.patient", "fhir.encounter", "fhir.observation"]
	ResourcePaths []string `json:"resourcePaths"`

	// BundleID is an optional Bundle.id value. If empty one is auto-generated.
	BundleID string `json:"bundleId"`

	// IncludeRequest adds a Bundle.entry.request block (required for transaction bundles).
	// When true the connector builds a minimal request using the resource's resourceType + id.
	IncludeRequest bool `json:"includeRequest"`
}

// PayloadBuilderExecutor constructs final wire payloads from pipeline data.
type PayloadBuilderExecutor struct {
	*executors.BaseExecutor
	narrativeGen *fhirnarrative.FHIRNarrativeGenerator
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
		narrativeGen: fhirnarrative.NewFHIRNarrativeGenerator(),
	}
}

// payloadBuilderConfig is the config stored in transformation_steps.config.
type payloadBuilderConfig struct {
	Mode          string           `json:"mode"`           // pass_through | template | field_builder | fhir_bundle
	SourcePath    string           `json:"source_path"`    // pass_through: pipeline variable path
	OutputFormat  string           `json:"output_format"`  // fhir_r4 | hl7v2 | json | csv
	ContentType   string           `json:"content_type"`   // MIME type override
	Template      string           `json:"template"`       // template mode: the template string
	FieldMappings []fieldMapping   `json:"field_mappings"` // field_builder mode
	FHIRBundle    fhirBundleConfig `json:"fhirBundle"`     // fhir_bundle mode
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
	case "fhir_bundle":
		payload, contentType, err = e.buildFHIRBundle(inputData, cfg)
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

// ── FHIR Bundle builder ───────────────────────────────────────────────────────
// Wraps one or more resolved pipeline resources into a FHIR Bundle.
//
// Supported bundle types:
//   collection   — no entry.request blocks (default, read-only grouping)
//   transaction  — each entry gets entry.request {method, url} for atomic processing
//   batch        — like transaction but non-atomic
//   document     — document Bundle (first entry should be a Composition resource)

func (e *PayloadBuilderExecutor) buildFHIRBundle(
	inputData map[string]interface{},
	cfg payloadBuilderConfig,
) (string, string, error) {
	bc := cfg.FHIRBundle

	bundleType := bc.BundleType
	if bundleType == "" {
		bundleType = "collection"
	}

	// Validate bundle type
	validTypes := map[string]bool{
		"collection": true, "transaction": true, "batch": true, "document": true,
	}
	if !validTypes[bundleType] {
		return "", "application/fhir+json",
			fmt.Errorf("fhir_bundle: unsupported bundleType %q — use collection|transaction|batch|document", bundleType)
	}

	// transaction and batch always need request blocks
	includeRequest := bc.IncludeRequest || bundleType == "transaction" || bundleType == "batch"

	// Resolve resources from pipeline paths
	var resources []map[string]interface{}
	for _, path := range bc.ResourcePaths {
		raw := executors.GetFieldValue(inputData, path)
		if raw == nil {
			// Try inside message envelope
			if msg, ok := inputData["message"].(map[string]interface{}); ok {
				raw = executors.GetFieldValue(msg, path)
			}
		}
		if raw == nil {
			log.Printf("  ⚠️  [PayloadBuilder/fhir_bundle] path %q resolved to nil — skipping", path)
			continue
		}

		resource, ok := raw.(map[string]interface{})
		if !ok {
			// If it's a JSON string, unmarshal it
			if s, ok := raw.(string); ok {
				var m map[string]interface{}
				if err := json.Unmarshal([]byte(s), &m); err != nil {
					log.Printf("  ⚠️  [PayloadBuilder/fhir_bundle] path %q is string but not valid JSON: %v", path, err)
					continue
				}
				resource = m
			} else {
				log.Printf("  ⚠️  [PayloadBuilder/fhir_bundle] path %q is not a FHIR resource map — skipping", path)
				continue
			}
		}

		resources = append(resources, resource)
	}

	if len(resources) == 0 {
		return "", "application/fhir+json",
			fmt.Errorf("fhir_bundle: no resources resolved from resourcePaths %v", bc.ResourcePaths)
	}

	// Auto-generate a narrative (resource.text.div) for any resource that
	// doesn't already have one — same "engine does the mechanical FHIR
	// compliance work, never left for the user to map" precedent as fullUrl
	// assignment below, and the exact sequencing the CDA→FHIR document mapper
	// already establishes (declarative_document_mapper.go: narrative first,
	// then AssembleEntries). A resource that already carries a text.div is
	// left completely untouched (not just the div — the whole text object,
	// including a status the source data set, e.g. "additional") rather than
	// replaced with a freshly built {"status": "generated", ...} object.
	for _, r := range resources {
		if existing, ok := r["text"].(map[string]interface{}); ok {
			if div, ok := existing["div"].(string); ok && div != "" {
				continue
			}
		}
		narrative := e.narrativeGen.Generate(r)
		if narrative != "" {
			r["text"] = map[string]interface{}{
				"status": "generated",
				"div":    narrative,
			}
		}
	}

	// AssembleEntries assigns every entry a urn:uuid: fullUrl and rewrites
	// every "ResourceType/id" reference anywhere in any resource to match —
	// the same already-validated logic services/cda_fhir's Bundle assembly
	// uses (fhir/r4/bundle_assembler.go), reused here instead of maintaining
	// a second, parallel fullUrl scheme. Using a single urn:uuid: scheme for
	// every entry (rather than mixing in "{base}/ResourceType/id" for
	// resources that happen to have their own id) avoids the exact class of
	// "fullUrl does not match the full target URL" warning some validators
	// raise when resolving a relative reference against a mixed-scheme Bundle.
	entries := r4.AssembleEntries(resources)

	if includeRequest {
		for _, rawEntry := range entries {
			entry, ok := rawEntry.(map[string]interface{})
			if !ok {
				continue
			}
			resource, ok := entry["resource"].(map[string]interface{})
			if !ok {
				continue
			}
			entry["request"] = buildBundleEntryRequest(bundleType, resource)
		}
	}

	// Build Bundle.id
	bundleID := bc.BundleID
	if bundleID == "" {
		bundleID = fmt.Sprintf("bundle-%d", time.Now().UnixNano())
	}

	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"id":           bundleID,
		"type":         bundleType,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"entry":        entries,
	}
	// FHIR constraint bun-1: total should only be present for searchset/history
	// bundles. bundleType is validated above to one of
	// collection|transaction|batch|document, none of which is searchset/history,
	// so total is never set here — kept as an explicit conditional (rather than
	// simply never setting it) so a future searchset/history mode addition
	// doesn't silently reintroduce this bug.
	if bundleType == "searchset" || bundleType == "history" {
		bundle["total"] = len(entries)
	}

	body, err := json.Marshal(bundle)
	if err != nil {
		return "", "application/fhir+json", fmt.Errorf("fhir_bundle: marshal failed: %w", err)
	}

	log.Printf("  📦 [PayloadBuilder/fhir_bundle] type=%s entries=%d id=%s", bundleType, len(entries), bundleID)
	return string(body), "application/fhir+json", nil
}

// buildBundleEntryRequest creates the entry.request block for transaction/batch Bundles.
// FHIR REST semantics: resource with id → PUT, without id → POST.
func buildBundleEntryRequest(bundleType string, resource map[string]interface{}) map[string]interface{} {
	resourceType, _ := resource["resourceType"].(string)
	resourceID, _ := resource["id"].(string)

	method := "POST"
	url := resourceType
	if resourceID != "" {
		method = "PUT"
		url = resourceType + "/" + resourceID
	}

	req := map[string]interface{}{
		"method": method,
		"url":    url,
	}

	// For batch bundles include ifNoneExist for conditional creates when no ID is set
	if bundleType == "batch" && resourceID == "" && resourceType != "" {
		req["ifNoneExist"] = "identifier=" + resourceType
	}

	return req
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
