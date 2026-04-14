// services/ai/script_generator.go
// Generates JavaScript transform(input) scripts for pipeline script steps.
// Fetches live step + pipeline context so the LLM knows exactly what data
// is available at that point in the pipeline.
package ai

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ScriptGeneratorService builds context-aware JS scripts for pipeline steps.
type ScriptGeneratorService struct {
	db  *sql.DB
	ctx *ContextBuilder
}

func newScriptGeneratorService(db *sql.DB) *ScriptGeneratorService {
	return &ScriptGeneratorService{db: db, ctx: NewContextBuilder(db)}
}

// ScriptGenInput bundles the request from the developer.
type ScriptGenInput struct {
	Description string // plain-English: what the script should do
	StepID      string // optional – used to load existing config
	PipelineID  string // optional – used for step sequence context
	InterfaceID string // optional – used for interface context
	MessageType string // optional – e.g. "ADT^A01"
	StepType    string // e.g. "enrichment.script"
	// CurrentScript is the existing script content (may be empty for new steps)
	CurrentScript string
}

// scriptSystemPrompt is the specialist prompt for JS script generation.
const scriptSystemPrompt = `You are ezCompanion — a JavaScript pipeline script expert for ezHealthKonnect.

## Script Contract
Every pipeline script MUST export a function named ` + "`transform`" + `:
` + "```javascript" + `
function transform(input) {
    // input is the parsed message envelope (see Structure below)
    // Modify input fields, add metadata, return the modified object.
    // Throw an Error to fail the step intentionally.
    return input;
}
` + "```" + `

## Input Structure (available fields)
` + "```" + `
input
├── enhancedSegments          // HL7 segments (if HL7 message)
│   ├── MSH.fields[]          // Message Header fields
│   ├── PID.fields[]          // Patient Identification
│   ├── PV1.fields[]          // Patient Visit
│   ├── OBX.fields[]          // Observation Results
│   └── ...                   // Any segment present in the message
│       └── field.key         // e.g. "PID.5" – patient name
│       └── field.value       // string value
│       └── field.subfields[] // for composite fields
├── _metadata                 // Pipeline metadata you can read/write
│   ├── messageId
│   ├── interfaceId
│   ├── messageType           // e.g. "ADT^A01"
│   ├── receivedAt
│   └── priority              // "normal" | "high" | "low"
├── _routing                  // Set nextStep/nextSteps to redirect flow
└── [any key you add]         // You can add arbitrary fields to output
` + "```" + `

## Helper Patterns
` + "```javascript" + `
// Read an HL7 field safely
function getField(segments, segName, fieldKey) {
    const seg = segments[segName];
    if (!seg || !seg.fields) return '';
    const f = seg.fields.find(f => f.key === fieldKey);
    return f ? (f.value || '') : '';
}

// Read a subfield (composite)
function getSubfield(segments, segName, fieldKey, subfieldKey) {
    const seg = segments[segName];
    if (!seg) return '';
    const f = seg.fields && seg.fields.find(f => f.key === fieldKey);
    if (!f || !f.subfields) return '';
    const sf = f.subfields.find(sf => sf.key === subfieldKey);
    return sf ? (sf.value || '') : '';
}
` + "```" + `

## Rules
- Always return the modified ` + "`input`" + ` object (never return null/undefined).
- Wrap risky logic in try/catch if failure should be non-fatal.
- Use ` + "`input._metadata.priority = 'high'`" + ` to escalate a message.
- Use ` + "`input._routing = { nextStep: 'step-id' }`" + ` for conditional routing.
- Never use ` + "`fetch`" + `, ` + "`require`" + `, or ` + "`import`" + ` — the script runs in a sandboxed goja VM.
- Prefer ` + "`const`" + ` / ` + "`let`" + ` over ` + "`var`" + `.` +
`

## Da Vinci PAS Pipelines
If the pipeline is a Da Vinci PAS prior authorization pipeline (check preceding steps for a
user_mapping step), the input structure is different from HL7 pipelines:

Fields come from the user_mapping step output — NOT from enhancedSegments:
  input.steps["user_mapping"].step_output.patient_first_name
  input.steps["user_mapping"].step_output.member_id
  ... etc.

Available fields: patient_first_name, patient_last_name, patient_dob, patient_gender,
patient_id, member_id, payer_name, payer_id, provider_npi, provider_name, org_npi,
org_name, service_code, service_description, icd10_code, icd10_description,
service_start_date, units_requested, priority.

Scripts that build FHIR resources must return the resource object:
  return { resourceType: "Patient", id: um.patient_id, name: [...], ... };

Decision codes in parse_response step output:
  AA = approved, AD = denied, CP = partial approval, PE = pended (pending review).`

// GenerateScriptStream generates a JS script and streams tokens to onToken.
func (s *ScriptGeneratorService) GenerateScriptStream(
	ctx context.Context,
	input ScriptGenInput,
	ollama LLMProvider,
	onToken func(string) error,
) error {
	var parts []string

	// 1. Live step context from DB
	if input.StepID != "" && s.db != nil {
		if stepCtx := s.fetchStepDetail(ctx, input.StepID); stepCtx != "" {
			parts = append(parts, "### Current Step\n"+stepCtx)
		}
	}

	// 2. Preceding steps context (what data is available)
	if input.PipelineID != "" && s.db != nil {
		if preceding := s.fetchPrecedingSteps(ctx, input.PipelineID, input.StepID); preceding != "" {
			parts = append(parts, "### Steps Before This One (these have already run)\n"+preceding)
		}
	}

	// 3. Interface / message type hint
	if input.InterfaceID != "" {
		if ifaceCtx := s.ctx.fetchInterfaceContext(ctx, input.InterfaceID); ifaceCtx != "" {
			parts = append(parts, "### Interface\n"+ifaceCtx)
		}
	}
	if input.MessageType != "" {
		parts = append(parts, fmt.Sprintf("### Message Type\n%s", input.MessageType))
	}

	// 4. Existing script (if editing, not creating)
	if strings.TrimSpace(input.CurrentScript) != "" {
		parts = append(parts, "### Existing Script (modify or replace as needed)\n```javascript\n"+input.CurrentScript+"\n```")
	}

	liveCtx := strings.Join(parts, "\n\n")

	sysPrompt := scriptSystemPrompt
	if liveCtx != "" {
		sysPrompt += "\n\n" + liveCtx
	}

	prompt := buildRAGPrompt(sysPrompt,
		"Write a complete JavaScript transform(input) function that does the following:\n\n"+input.Description+
			"\n\nReturn ONLY the JavaScript code, no explanation before or after.",
		nil)

	return ollama.GenerateStream(ctx, prompt, onToken)
}

// ─── Private helpers ──────────────────────────────────────────────────────────

func (s *ScriptGeneratorService) fetchStepDetail(ctx context.Context, stepID string) string {
	var stepName, stepType, configJSON string
	var seq int
	err := s.db.QueryRowContext(ctx, `
		SELECT step_name, step_type, sequence, COALESCE(config::text, '{}')
		FROM transformation_steps WHERE id = $1
	`, stepID).Scan(&stepName, &stepType, &seq, &configJSON)
	if err != nil {
		return ""
	}
	if len(configJSON) > 600 {
		configJSON = configJSON[:600] + "..."
	}
	return fmt.Sprintf("Name: %s\nType: %s\nSequence: %d\nConfig: %s", stepName, stepType, seq, configJSON)
}

func (s *ScriptGeneratorService) fetchPrecedingSteps(ctx context.Context, pipelineID, currentStepID string) string {
	// Get sequence of current step first
	currentSeq := 999999
	if currentStepID != "" {
		_ = s.db.QueryRowContext(ctx,
			`SELECT sequence FROM transformation_steps WHERE id = $1`, currentStepID,
		).Scan(&currentSeq)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT step_name, step_type, sequence
		FROM transformation_steps
		WHERE pipeline_id = $1 AND sequence < $2 AND enabled = true
		ORDER BY sequence
	`, pipelineID, currentSeq)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var name, typ string
		var seq int
		if err := rows.Scan(&name, &typ, &seq); err != nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("  Seq %3d: [%s] %s", seq, typ, name))
	}
	return strings.Join(lines, "\n")
}
