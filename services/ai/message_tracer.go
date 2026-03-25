// services/ai/message_tracer.go
// Fetches the full step-execution trace for a message and asks the LLM
// to explain where it failed, why, and how to fix it.
package ai

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// StepTrace is one step's execution record.
type StepTrace struct {
	Sequence     int    `json:"sequence"`
	StepName     string `json:"step_name"`
	StepType     string `json:"step_type"`
	Status       string `json:"status"` // completed | failed | skipped | running
	DurationMS   int64  `json:"duration_ms"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// MessageTraceResult is what the caller gets back.
type MessageTraceResult struct {
	MessageID    string      `json:"message_id"`
	InterfaceID  string      `json:"interface_id"`
	MessageType  string      `json:"message_type"`
	OverallStatus string     `json:"overall_status"`
	StartedAt    time.Time   `json:"started_at"`
	DurationMS   int64       `json:"duration_ms"`
	Steps        []StepTrace `json:"steps"`
	FailedStep   *StepTrace  `json:"failed_step,omitempty"`
	// LLM analysis fields
	RootCause   string   `json:"root_cause"`
	Explanation string   `json:"explanation"`
	Suggestions []string `json:"suggestions"`
}

// MessageTracerService fetches execution traces and drives LLM analysis.
type MessageTracerService struct {
	db *sql.DB
}

func newMessageTracerService(db *sql.DB) *MessageTracerService {
	return &MessageTracerService{db: db}
}

// Trace fetches the execution record for a message and returns the step trace.
// It does NOT call the LLM — that's done separately (streaming or non-streaming).
func (t *MessageTracerService) Trace(ctx context.Context, messageID, interfaceID string) (*MessageTraceResult, error) {
	if t.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	// Find the transformation execution for this message
	var execID, msgType, status string
	var startedAt time.Time
	var durationMS sql.NullInt64

	err := t.db.QueryRowContext(ctx, `
		SELECT te.id, COALESCE(tp.message_type,''), COALESCE(te.status,'unknown'),
		       te.started_at, COALESCE(te.total_time_ms, 0)
		FROM transformation_executions te
		LEFT JOIN transformation_pipelines tp ON tp.id = te.pipeline_id
		WHERE te.message_id = $1
		ORDER BY te.started_at DESC
		LIMIT 1
	`, messageID).Scan(&execID, &msgType, &status, &startedAt, &durationMS)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no execution record found for message %s — it may not have been processed yet", messageID)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup execution: %w", err)
	}

	result := &MessageTraceResult{
		MessageID:     messageID,
		InterfaceID:   interfaceID,
		MessageType:   msgType,
		OverallStatus: status,
		StartedAt:     startedAt,
		DurationMS:    durationMS.Int64,
	}

	// Fetch step traces
	rows, err := t.db.QueryContext(ctx, `
		SELECT
			COALESCE(ts.sequence, 0),
			se.step_name,
			se.step_type,
			COALESCE(se.status, 'unknown'),
			COALESCE(se.duration_ms, 0),
			COALESCE(se.error_message, '')
		FROM step_executions se
		LEFT JOIN transformation_steps ts ON ts.id = se.step_id
		WHERE se.execution_id = $1
		ORDER BY COALESCE(ts.sequence, 0), se.started_at
	`, execID)
	if err != nil {
		return result, nil // return partial result
	}
	defer rows.Close()

	for rows.Next() {
		var st StepTrace
		if err := rows.Scan(&st.Sequence, &st.StepName, &st.StepType,
			&st.Status, &st.DurationMS, &st.ErrorMessage); err != nil {
			continue
		}
		result.Steps = append(result.Steps, st)
		if st.Status == "failed" && result.FailedStep == nil {
			cp := st
			result.FailedStep = &cp
		}
	}

	return result, nil
}

// BuildTracePrompt builds the LLM prompt from a trace result.
func BuildTracePrompt(trace *MessageTraceResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(
		"Message ID: %s\nInterface: %s\nMessage Type: %s\nOverall Status: %s\nDuration: %dms\n\n",
		trace.MessageID, trace.InterfaceID, trace.MessageType, trace.OverallStatus, trace.DurationMS,
	))

	sb.WriteString("## Step-by-Step Execution Trace\n\n")
	for _, st := range trace.Steps {
		icon := "✅"
		switch st.Status {
		case "failed":
			icon = "❌"
		case "skipped":
			icon = "⏭"
		case "running":
			icon = "🔄"
		}
		line := fmt.Sprintf("%s Seq %3d [%s] %-35s %dms  status=%s",
			icon, st.Sequence, st.StepType, st.StepName, st.DurationMS, st.Status)
		if st.ErrorMessage != "" {
			errPreview := st.ErrorMessage
			if len(errPreview) > 200 {
				errPreview = errPreview[:200] + "..."
			}
			line += "\n        Error: " + errPreview
		}
		sb.WriteString(line + "\n")
	}

	return sb.String()
}

const messageTraceSystemPrompt = `You are ezCompanion — a healthcare integration expert and co-developer for ezHealthKonnect.

You have been given a step-by-step execution trace for a message that failed to process.

Your job:
1. Identify the EXACT step where processing failed.
2. Explain in plain English WHY it failed (root cause, not symptoms).
3. Give 2-4 concrete, actionable suggestions to fix it.
4. Note any steps that ran unusually slowly (high duration_ms) even if they succeeded.

Be specific: reference step names, step types, and error messages from the trace.
Keep your response structured:
- **Failed at:** [step name and type]
- **Root Cause:** [explanation]
- **Why this happened:** [more detail]
- **Suggestions:** numbered list
- **Performance notes:** (optional, only if relevant)`
