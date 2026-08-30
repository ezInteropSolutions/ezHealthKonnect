// services/ai/pipeline_step_docs_ingestion.go
// Ingests the real, continuously-maintained pipeline-step documentation
// (public/js/pipeline/documentation/*.js, StepDocumentationRegistry) into the
// AI knowledge base — replacing the hand-written, drift-prone step-type
// entries that used to live in builtin_knowledge.go. The JS registry has no
// server-side form of its own, so scripts/export-pipeline-step-docs.js
// (run via `npm run docs:export-step-docs`) serializes it to
// architecture/generated/pipeline_step_docs.json, which this file reads.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

// pipelineStepDocsFile mirrors the JSON shape scripts/export-pipeline-step-docs.js writes.
type pipelineStepDocsFile struct {
	Registry map[string]map[string]interface{} `json:"registry"`
	Aliases  map[string]string                  `json:"aliases"`
}

// IngestPipelineStepDocs loads the exported step-documentation JSON and embeds
// one chunk-source per step type (plus one per legacy alias, so an old-name
// question still retrieves the right content — RAG similarity search doesn't
// go through StepDocumentationRegistry.get()'s alias resolution). Degrades
// gracefully (like IngestHL7Schemas does for a missing schema dir) when the
// export hasn't been run yet, rather than failing the whole ingestion pass.
func (k *KnowledgeIngestionService) IngestPipelineStepDocs(ctx context.Context, jsonPath string) *IngestionResult {
	result := &IngestionResult{SourceType: "pipeline_step_docs"}

	b, err := os.ReadFile(jsonPath)
	if os.IsNotExist(err) {
		result.Errors = append(result.Errors, fmt.Sprintf("not found (run: npm run docs:export-step-docs): %s", jsonPath))
		return result
	}
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("read %s: %v", jsonPath, err))
		return result
	}

	var file pipelineStepDocsFile
	if err := json.Unmarshal(b, &file); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("parse %s: %v", jsonPath, err))
		return result
	}

	if err := k.embedding.ClearSourceType(ctx, "pipeline_step_docs"); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("clear: %v", err))
	}

	// Sorted for deterministic ingestion order (stable logs/diffs across runs).
	stepTypes := make([]string, 0, len(file.Registry))
	for stepType := range file.Registry {
		stepTypes = append(stepTypes, stepType)
	}
	sort.Strings(stepTypes)

	for _, stepType := range stepTypes {
		result.FilesScanned++
		text := renderStepDocToText(stepType, file.Registry[stepType])
		n, err := k.embedding.IngestText(ctx, "pipeline_step_docs", stepType, "generated:pipeline_step_docs", text, nil)
		result.ChunksStored += n // count whatever succeeded even if some chunks in this entry failed
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", stepType, err))
		}
	}

	aliases := make([]string, 0, len(file.Aliases))
	for alias := range file.Aliases {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	for _, alias := range aliases {
		target := file.Aliases[alias]
		doc, ok := file.Registry[target]
		if !ok {
			continue
		}
		result.FilesScanned++
		text := fmt.Sprintf("'%s' is a legacy/alias name for the '%s' step.\n\n%s", alias, target, renderStepDocToText(target, doc))
		n, err := k.embedding.IngestText(ctx, "pipeline_step_docs", alias, "generated:pipeline_step_docs", text, nil)
		result.ChunksStored += n // count whatever succeeded even if some chunks in this entry failed
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("alias %s: %v", alias, err))
		}
	}

	log.Printf("✅ AI KB — Pipeline Step Docs: %d entries, %d chunks, %d errors",
		result.FilesScanned, result.ChunksStored, len(result.Errors))
	return result
}

// renderStepDocToText formats one step's doc object (as produced by
// StepDocumentationRegistry) into readable prose for embedding. Known fields
// are rendered specifically; any other field falls back to a pretty-printed
// JSON block so new fields added later degrade gracefully instead of being
// silently dropped.
func renderStepDocToText(stepType string, doc map[string]interface{}) string {
	var sb strings.Builder
	handled := map[string]bool{}

	fmt.Fprintf(&sb, "Pipeline step type: %s\n\n", stepType)

	if desc := asString(doc["description"]); desc != "" {
		sb.WriteString(desc)
		sb.WriteString("\n\n")
	}
	handled["description"] = true

	if items := asStringSlice(doc["useCases"]); len(items) > 0 {
		sb.WriteString("Use cases:\n")
		for _, item := range items {
			fmt.Fprintf(&sb, "- %s\n", item)
		}
		sb.WriteString("\n")
	}
	handled["useCases"] = true

	if params := asSlice(doc["parameters"]); len(params) > 0 {
		sb.WriteString("Configuration parameters:\n")
		for _, p := range params {
			pm := asMap(p)
			if pm == nil {
				continue
			}
			name := asString(pm["name"])
			typ := asString(pm["type"])
			required := "optional"
			if b, ok := pm["required"].(bool); ok && b {
				required = "required"
			}
			desc := asString(pm["description"])
			fmt.Fprintf(&sb, "- %s (%s, %s): %s\n", name, typ, required, desc)
		}
		sb.WriteString("\n")
	}
	handled["parameters"] = true

	if wf := asSlice(doc["workflow"]); len(wf) > 0 {
		sb.WriteString("Configuration workflow:\n")
		for _, w := range wf {
			wm := asMap(w)
			if wm == nil {
				continue
			}
			fmt.Fprintf(&sb, "- %s: %s\n", asString(wm["action"]), asString(wm["description"]))
		}
		sb.WriteString("\n")
	}
	handled["workflow"] = true

	if bp := asSlice(doc["bestPractices"]); len(bp) > 0 {
		sb.WriteString("Best practices:\n")
		for _, item := range bp {
			bm := asMap(item)
			if bm == nil {
				continue
			}
			fmt.Fprintf(&sb, "- %s. Reason: %s", asString(bm["practice"]), asString(bm["reason"]))
			if ex := asString(bm["example"]); ex != "" {
				fmt.Fprintf(&sb, " Example: %s", ex)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	handled["bestPractices"] = true

	if ts := asSlice(doc["troubleshooting"]); len(ts) > 0 {
		sb.WriteString("Troubleshooting:\n")
		for _, item := range ts {
			tm := asMap(item)
			if tm == nil {
				continue
			}
			fmt.Fprintf(&sb, "- Issue: %s\n  Cause: %s\n  Fix: %s\n",
				asString(tm["issue"]), asString(tm["cause"]), asString(tm["fix"]))
		}
		sb.WriteString("\n")
	}
	handled["troubleshooting"] = true

	for _, exampleKey := range []string{"example", "examples"} {
		if ex, ok := doc[exampleKey]; ok {
			if b, err := json.MarshalIndent(ex, "", "  "); err == nil {
				fmt.Fprintf(&sb, "Example configuration (%s):\n%s\n\n", exampleKey, string(b))
			}
		}
		handled[exampleKey] = true
	}

	if so := asMap(doc["stepOutput"]); so != nil {
		sb.WriteString("Step output (available to later steps in the pipeline):\n")
		if desc := asString(so["description"]); desc != "" {
			sb.WriteString(desc + "\n")
		}
		for _, f := range asSlice(so["fields"]) {
			fm := asMap(f)
			if fm == nil {
				continue
			}
			fmt.Fprintf(&sb, "- %s (%s): %s\n", asString(fm["name"]), asString(fm["type"]), asString(fm["description"]))
		}
		sb.WriteString("\n")
	}
	handled["stepOutput"] = true

	// Fallback: any field not specifically rendered above still gets embedded,
	// pretty-printed, rather than silently dropped.
	remainingKeys := make([]string, 0)
	for k := range doc {
		if !handled[k] {
			remainingKeys = append(remainingKeys, k)
		}
	}
	sort.Strings(remainingKeys)
	for _, k := range remainingKeys {
		if b, err := json.MarshalIndent(doc[k], "", "  "); err == nil {
			fmt.Fprintf(&sb, "%s:\n%s\n\n", k, string(b))
		}
	}

	return sb.String()
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func asSlice(v interface{}) []interface{} {
	s, _ := v.([]interface{})
	return s
}

func asMap(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

func asStringSlice(v interface{}) []string {
	items := asSlice(v)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
