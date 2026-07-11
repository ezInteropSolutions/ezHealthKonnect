// services/ai/mapping_suggester_service.go
// MappingSuggesterService — design-time "Suggest Mappings" assist for the
// no-code fhir.build / hl7.build / cda.map_to_canonical step builders.
//
// Given a few sample source rows and the step's OWN live target-field
// catalog (fetched by the frontend from /api/fhir/canonical-fields,
// /api/hl7/canonical-fields, or /api/cda/canonical-fields — the SAME catalog
// that step's datalist-backed mapping table already uses), proposes
// sourcePath -> targetField mappings the human can review, edit, and save —
// never auto-applied by anything in this package.
//
// Deliberately NOT AIService.SuggestMappings: that method's contract is
// "paste one whole raw HL7v2/CDA message, guess a handful of FHIR mappings"
// (a built-in static catalogue keyed off HL7 segment/field names) — a
// different question from "map arbitrary CSV/DB/JSON sample ROWS against an
// explicit, already-fetched target catalog," which has no raw message to
// parse and needs the catalog to be a hard constraint, not a starting guess.
//
// Design-time only: never called from the runtime message pipeline, only
// from the pipeline-builder UI while a human is authoring a step's config —
// no PHI from a live message ever reaches this path. Mirrors
// ScriptGeneratorService's "generate, then human reviews before Save"
// pattern, not AgentService's heavier propose/approve/audit-log ceremony
// (that one is for mutating actions elsewhere in the app); the existing
// step-Save flow is already the human-approval gate here.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// MappingSuggesterService proposes no-code field mappings for the fhir.build/
// hl7.build/cda.map_to_canonical step builders.
type MappingSuggesterService struct{}

func newMappingSuggesterService() *MappingSuggesterService {
	return &MappingSuggesterService{}
}

// TargetFieldInfo is one mappable target field — matches the shape
// fhir/builder.CanonicalFieldInfo, hl7/builder.CanonicalFieldInfo, and
// cda/builder.CanonicalFieldInfo already share.
type TargetFieldInfo struct {
	Key      string `json:"key"`
	Label    string `json:"label,omitempty"`
	DataType string `json:"dataType,omitempty"`
}

// FieldMappingSuggestInput bundles one suggestion request.
type FieldMappingSuggestInput struct {
	StepType     string                   // "fhir.build" | "hl7.build" | "cda.map_to_canonical" — prompt framing only
	SampleRows   []map[string]interface{} // a few representative source rows
	TargetFields []TargetFieldInfo        // the step's OWN live target catalog — suggestions are constrained to these keys
}

// SuggestFieldMappings asks llm to propose sourcePath -> targetField mappings
// for input.SampleRows against input.TargetFields, reusing the same
// MappingSuggestion wire shape AIService.SuggestMappings already returns
// (for frontend consistency). llm is passed in rather than stored, mirroring
// ScriptGeneratorService.GenerateScriptStream's identical parameter shape.
func (s *MappingSuggesterService) SuggestFieldMappings(ctx context.Context, llm LLMProvider, input FieldMappingSuggestInput) ([]MappingSuggestion, error) {
	if len(input.SampleRows) == 0 {
		return nil, fmt.Errorf("mapping_suggester: at least one sample row is required")
	}
	if len(input.TargetFields) == 0 {
		return nil, fmt.Errorf("mapping_suggester: target field catalog is empty")
	}

	answer, err := llm.Generate(ctx, s.buildPrompt(input))
	if err != nil {
		return nil, fmt.Errorf("mapping_suggester: %w", err)
	}

	extracted := extractJSON(answer)
	if extracted == "" {
		return nil, fmt.Errorf("mapping_suggester: could not extract JSON from LLM response")
	}

	var suggestions []MappingSuggestion
	if err := json.Unmarshal([]byte(extracted), &suggestions); err != nil {
		return nil, fmt.Errorf("mapping_suggester: parse suggestions: %w", err)
	}

	return constrainToCatalog(suggestions, input.TargetFields), nil
}

// buildPrompt assembles a compact system+question prompt: the sample rows'
// own field names/example values (never the full rows — only the shape
// matters, keeping the prompt small for CPU inference), the exact target
// field vocabulary, and strict output-format instructions.
func (s *MappingSuggesterService) buildPrompt(input FieldMappingSuggestInput) string {
	var sourceFields []string
	seen := make(map[string]bool)
	for _, row := range input.SampleRows {
		for k, v := range row {
			if seen[k] {
				continue
			}
			seen[k] = true
			sourceFields = append(sourceFields, fmt.Sprintf("%s (e.g. %s)", k, truncate(fmt.Sprintf("%v", v), 40)))
		}
	}

	targetLines := make([]string, 0, len(input.TargetFields))
	for _, f := range input.TargetFields {
		label := f.Label
		if label == "" {
			label = f.Key
		}
		targetLines = append(targetLines, fmt.Sprintf("- %s: %s (%s)", f.Key, label, f.DataType))
	}

	systemPrompt := fmt.Sprintf(`You are a healthcare data mapping expert helping configure a no-code %s pipeline step.
Given the SOURCE fields available in sample data rows and the exact TARGET fields this step supports, propose the best source->target mappings.

Rules:
- target_field MUST be copied EXACTLY from the TARGET FIELDS list below — never invent one.
- Only propose a mapping when you are reasonably confident; omit fields with no good match.
- Output ONLY a compact JSON array, no prose: [{"source_field":"","target_field":"","confidence":0.9,"reasoning":""}]

SOURCE FIELDS (from sample rows):
%s

TARGET FIELDS (this step's exact vocabulary):
%s`, input.StepType, strings.Join(sourceFields, "\n"), strings.Join(targetLines, "\n"))

	return buildRAGPrompt(systemPrompt, "Propose the field mappings.", nil)
}

// constrainToCatalog drops any suggestion whose TargetField isn't literally
// in targetFields — the LLM is instructed not to invent one, but a small
// model can still drift; this is the hard enforcement backing that
// instruction, not a substitute for it.
func constrainToCatalog(suggestions []MappingSuggestion, targetFields []TargetFieldInfo) []MappingSuggestion {
	valid := make(map[string]bool, len(targetFields))
	for _, f := range targetFields {
		valid[f.Key] = true
	}
	out := make([]MappingSuggestion, 0, len(suggestions))
	for _, sug := range suggestions {
		if valid[sug.TargetField] {
			out = append(out, sug)
		}
	}
	return out
}
