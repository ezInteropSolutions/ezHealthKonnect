// services/ai/context_builder.go
// Fetches live system data (interface config, pipeline steps, recent errors)
// and formats it as a system prompt prefix so the LLM understands what the
// user is currently working on — not just generic healthcare standards.
package ai

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// RequestContext describes what the user is currently looking at in the UI.
// The frontend sends this with every AI request.
type RequestContext struct {
	InterfaceID  string `json:"interface_id,omitempty"`
	PipelineID   string `json:"pipeline_id,omitempty"`
	StepID       string `json:"step_id,omitempty"`
	MessageType  string `json:"message_type,omitempty"`
	Page         string `json:"page,omitempty"`         // "pipeline-builder", "monitoring", "wizard", "interfaces"
	ErrorMessage string `json:"error_message,omitempty"` // error currently being investigated
}

// IsEmpty returns true when no context was provided.
func (r RequestContext) IsEmpty() bool {
	return r.InterfaceID == "" && r.PipelineID == "" && r.StepID == "" &&
		r.Page == "" && r.ErrorMessage == ""
}

// ContextBuilder fetches DB data and formats it as a system prompt prefix.
type ContextBuilder struct {
	db *sql.DB
}

// NewContextBuilder creates a new ContextBuilder.
func NewContextBuilder(db *sql.DB) *ContextBuilder {
	return &ContextBuilder{db: db}
}

// BuildSystemPromptPrefix returns a context block to prepend to the LLM system prompt.
// Returns empty string when no relevant context is available.
func (cb *ContextBuilder) BuildSystemPromptPrefix(ctx context.Context, reqCtx RequestContext) string {
	if cb.db == nil || reqCtx.IsEmpty() {
		return ""
	}

	var parts []string

	// Interface context
	if reqCtx.InterfaceID != "" {
		if s := cb.fetchInterfaceContext(ctx, reqCtx.InterfaceID); s != "" {
			parts = append(parts, s)
		}
	}

	// Pipeline context (prefer explicit ID, fall back to interface+messageType lookup)
	if reqCtx.PipelineID != "" {
		if s := cb.fetchPipelineContext(ctx, reqCtx.PipelineID); s != "" {
			parts = append(parts, s)
		}
	} else if reqCtx.InterfaceID != "" && reqCtx.MessageType != "" {
		if s := cb.fetchPipelineByInterface(ctx, reqCtx.InterfaceID, reqCtx.MessageType); s != "" {
			parts = append(parts, s)
		}
	}

	// Step context
	if reqCtx.StepID != "" {
		if s := cb.fetchStepContext(ctx, reqCtx.StepID); s != "" {
			parts = append(parts, s)
		}
	}

	// Page label
	if reqCtx.Page != "" {
		parts = append(parts, fmt.Sprintf("The user is currently on the %s page.", pageLabel(reqCtx.Page)))
	}

	// Active error
	if reqCtx.ErrorMessage != "" {
		parts = append(parts, fmt.Sprintf("Error currently being investigated:\n  %s", reqCtx.ErrorMessage))
	}

	if len(parts) == 0 {
		return ""
	}

	return "### Current System Context\n\n" + strings.Join(parts, "\n\n") + "\n\n---\n\n"
}

// ─── Private fetch helpers ────────────────────────────────────────────────────

func (cb *ContextBuilder) fetchInterfaceContext(ctx context.Context, interfaceID string) string {
	var name, description, sourceType, sourceConn, targetType, targetConn, msgType, status string
	var totalProc, successProc, failedProc int

	err := cb.db.QueryRowContext(ctx, `
		SELECT
			name,
			COALESCE(description, ''),
			source_type,
			COALESCE(source_connectivity, ''),
			target_type,
			COALESCE(target_connectivity, ''),
			COALESCE(message_type, ''),
			COALESCE(interface_status, 'unknown'),
			COALESCE(total_processed, 0),
			COALESCE(successful_processed, 0),
			COALESCE(failed_processed, 0)
		FROM interfaces WHERE id = $1
	`, interfaceID).Scan(&name, &description, &sourceType, &sourceConn,
		&targetType, &targetConn, &msgType, &status,
		&totalProc, &successProc, &failedProc)
	if err != nil {
		return ""
	}

	successRate := 0.0
	if totalProc > 0 {
		successRate = float64(successProc) / float64(totalProc) * 100
	}

	lines := []string{
		fmt.Sprintf(`Interface: "%s"`, name),
		fmt.Sprintf(`  Source: %s (connectivity: %s)`, sourceType, sourceConn),
		fmt.Sprintf(`  Target: %s (connectivity: %s)`, targetType, targetConn),
		fmt.Sprintf(`  Message Type: %s`, msgType),
		fmt.Sprintf(`  Status: %s | %d processed, %.1f%% success rate`, status, totalProc, successRate),
	}
	if description != "" {
		lines = append(lines, fmt.Sprintf(`  Description: %s`, description))
	}

	// Append recent errors for this interface
	if recent := cb.fetchRecentErrors(ctx, interfaceID, 3); recent != "" {
		lines = append(lines, "  Recent errors:\n"+recent)
	}

	return strings.Join(lines, "\n")
}

func (cb *ContextBuilder) fetchRecentErrors(ctx context.Context, interfaceID string, limit int) string {
	rows, err := cb.db.QueryContext(ctx, `
		SELECT COALESCE(error_message, 'unknown'), COUNT(*)
		FROM audit_logs
		WHERE entity_id = $1
		  AND result = 'FAILURE'
		  AND error_message IS NOT NULL
		  AND created_at > NOW() - INTERVAL '7 days'
		GROUP BY error_message
		ORDER BY COUNT(*) DESC
		LIMIT $2
	`, interfaceID, limit)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var msg string
		var count int
		if err := rows.Scan(&msg, &count); err != nil {
			continue
		}
		// Truncate long error messages
		if len(msg) > 120 {
			msg = msg[:120] + "..."
		}
		lines = append(lines, fmt.Sprintf("    - %s (%d times)", msg, count))
	}
	return strings.Join(lines, "\n")
}

func (cb *ContextBuilder) fetchPipelineContext(ctx context.Context, pipelineID string) string {
	var messageType, pipelineName string
	err := cb.db.QueryRowContext(ctx,
		`SELECT message_type, pipeline_name FROM transformation_pipelines WHERE id = $1`,
		pipelineID).Scan(&messageType, &pipelineName)
	if err != nil {
		return ""
	}
	return cb.buildPipelineSummary(ctx, pipelineID, pipelineName, messageType)
}

func (cb *ContextBuilder) fetchPipelineByInterface(ctx context.Context, interfaceID, messageType string) string {
	var pipelineID, pipelineName string
	err := cb.db.QueryRowContext(ctx, `
		SELECT id, pipeline_name FROM transformation_pipelines
		WHERE interface_id = $1 AND message_type = $2 AND enabled = true
		LIMIT 1
	`, interfaceID, messageType).Scan(&pipelineID, &pipelineName)
	if err != nil {
		return ""
	}
	return cb.buildPipelineSummary(ctx, pipelineID, pipelineName, messageType)
}

func (cb *ContextBuilder) buildPipelineSummary(ctx context.Context, pipelineID, pipelineName, messageType string) string {
	rows, err := cb.db.QueryContext(ctx, `
		SELECT step_name, step_type, sequence, enabled
		FROM transformation_steps WHERE pipeline_id = $1 ORDER BY sequence
	`, pipelineID)
	if err != nil {
		return fmt.Sprintf(`Pipeline: "%s" (message type: %s)`, pipelineName, messageType)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`Pipeline: "%s" (message type: %s)`, pipelineName, messageType))
	sb.WriteString("\n  Steps:")

	total, disabled := 0, 0
	for rows.Next() {
		var stepName, stepType string
		var seq int
		var enabled bool
		if err := rows.Scan(&stepName, &stepType, &seq, &enabled); err != nil {
			continue
		}
		total++
		if !enabled {
			disabled++
			continue
		}
		sb.WriteString(fmt.Sprintf("\n    Seq %3d: [%-30s] %s", seq, stepType, stepName))
	}
	if disabled > 0 {
		sb.WriteString(fmt.Sprintf("\n    (%d disabled steps not shown)", disabled))
	}
	if total == 0 {
		sb.WriteString(" (no steps configured)")
	}
	return sb.String()
}

func (cb *ContextBuilder) fetchStepContext(ctx context.Context, stepID string) string {
	var stepName, stepType, configJSON string
	var seq int

	err := cb.db.QueryRowContext(ctx, `
		SELECT step_name, step_type, sequence, COALESCE(config::text, '{}')
		FROM transformation_steps WHERE id = $1
	`, stepID).Scan(&stepName, &stepType, &seq, &configJSON)
	if err != nil {
		return ""
	}

	if len(configJSON) > 400 {
		configJSON = configJSON[:400] + "..."
	}
	return fmt.Sprintf("Currently focused step: [%s] %s (sequence %d)\n  Config: %s",
		stepType, stepName, seq, configJSON)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func pageLabel(page string) string {
	labels := map[string]string{
		"pipeline-builder": "Pipeline Builder",
		"monitoring":       "Monitoring / Alerts",
		"wizard":           "Interface Configuration Wizard",
		"interfaces":       "Interfaces List",
		"dashboard":        "Dashboard",
		"hl7-reader":       "HL7 Reader",
		"fhir-validator":   "FHIR Validator",
		"review-queue":     "FHIR Validation Review Queue",
		"alerts":           "Alert Management",
		"analytics":        "Analytics",
		"settings":         "Settings",
	}
	if l, ok := labels[page]; ok {
		return l
	}
	return page
}
