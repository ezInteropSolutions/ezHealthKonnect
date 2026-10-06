// services/cda_coverage/parse_step_lookup.go
//
// Shared, format-agnostic "which <format>.parse step(s) does this interface's
// own pipeline configure" lookup — originally built for NCPDP SCRIPT/Telecom
// D.0 (ncpdp_adapter.go), reused unchanged by EDI X12 (edi_adapter.go) since
// the underlying question and mechanism are identical: a real fhir.build/
// enrichment.script sourcePath addressing this format's own parsed content
// needs to know that interface's OWN parse step's alias (models.
// OutputNormalizer.NormalizeKey(stepName)) and configured outputField, and
// there is no per-format reason for that lookup to be reimplemented per file.
package cdacoverage

import (
	"context"
	"database/sql"
	"encoding/json"

	"ezhealthkonnect/models"
)

// parseStepInfo is what an adapter's BuildInventory needs to know about ONE
// real <format>.parse step configured on this interface's own pipeline(s) —
// an interface can have more than one (different message types, or a
// re-parse step mid-pipeline); every one found is tried as an independent
// candidate address, since a specific message only ever went through one of
// them, and an adapter has no cheap way to know which.
type parseStepInfo struct {
	// alias is models.OutputNormalizer.NormalizeKey(stepName) — the exact
	// "steps.<alias>" address a later step's sourcePath would use.
	alias string
	// outputField is this step's own configured output field (format-specific
	// default, e.g. "parsedNCPDP"/"parsedEDI", but user-configurable, or the
	// literal "__root__" sentinel meaning "merge parsed fields directly into
	// the envelope with no wrapper key at all").
	outputField string
}

// resolveParseSteps queries every stepType step configured across every
// pipeline belonging to interfaceID, returning one parseStepInfo per row
// found. Never fails the caller: a query error or zero rows just means no
// candidate addresses exist, so BuildInventory correctly reports nothing
// trackable rather than guessing — the same fail-closed posture every other
// best-effort lookup in this feature already takes.
func resolveParseSteps(ctx context.Context, db *sql.DB, interfaceID, stepType, defaultOutputField string) []parseStepInfo {
	if db == nil || interfaceID == "" {
		return nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT ts.step_name, ts.config
		FROM transformation_steps ts
		JOIN transformation_pipelines tp ON tp.id = ts.pipeline_id
		WHERE tp.interface_id = $1 AND ts.step_type = $2
	`, interfaceID, stepType)
	if err != nil {
		return nil
	}
	defer rows.Close()

	normalizer := models.NewOutputNormalizer()
	var out []parseStepInfo
	for rows.Next() {
		var stepName string
		var rawConfig []byte
		if err := rows.Scan(&stepName, &rawConfig); err != nil {
			continue
		}
		outputField := defaultOutputField
		var cfg struct {
			OutputField string `json:"outputField"`
		}
		if len(rawConfig) > 0 {
			if err := json.Unmarshal(rawConfig, &cfg); err == nil && cfg.OutputField != "" {
				outputField = cfg.OutputField
			}
		}
		out = append(out, parseStepInfo{
			alias:       normalizer.NormalizeKey(stepName),
			outputField: outputField,
		})
	}
	return out
}
