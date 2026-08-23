// services/ai/agent_service.go
// AgentService — the propose/approve/reject loop for Agent Mode.
// The model may propose exactly one tool call per question; it is never
// executed until a human approves it via ApproveAction. All proposals and
// their outcomes are recorded in ai_agent_actions for audit.
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"ezhealthkonnect/services"

	"github.com/google/uuid"
)

// agentSystemPrompt tells the model which tools exist and how to behave.
// Kept separate from ezCompanionSystemPrompt since tool-calling models need
// explicit instruction to prefer a tool call over describing the action in prose.
const agentSystemPrompt = `You are ezCompanion running in Agent Mode for ezHealthKonnect.
When the user asks you to perform an action you have a tool for (retry a message, activate/deactivate an interface, propose a pipeline script), call that tool instead of describing the steps in prose.
Only call a tool when the user's request clearly maps to one — otherwise answer normally.
Never claim an action has been completed: tools you call are only PROPOSED and require human approval before they run.

If a question has multiple parts (e.g. "how many DLQ messages are pending, AND are there any active alerts"), call ONE tool for the first part, wait for its result, then call the next tool for the remaining part — do not answer with only part of the question covered. Only give your final answer once you have gathered everything needed for every part of the question.`

// unsupportedToolModelMsg is returned immediately, without attempting a
// ChatWithTools call, when the active chat model is confirmed (via
// knownModelCapabilities) not to support tool calling — otherwise Agent Mode
// would silently look like "no action needed" for every question, with no
// signal that the real cause is the selected model, not the question.
const unsupportedToolModelMsg = "Agent Mode needs a tool-calling model — '%s' is not one. Switch to a confirmed model (llama3.1, llama3.2, llama3.3, or mistral-nemo) in Settings → AI Assistant, or ask a plain question instead."

// unverifiedToolModelSuffix is appended to a plain (no-tool-call) answer when
// the active chat model isn't in knownModelCapabilities at all — the model
// may have genuinely decided no tool was needed, or it may not support tool
// calling and always returns zero tool calls; this at least tells the user
// which case they might be in.
const unverifiedToolModelSuffix = "\n\n_(Note: '%s' isn't a model ezHealthKonnect has confirmed supports tool calling — if you expected an action here, this may be why.)_"

// ProposedAction is a tool call the model requested, awaiting human approval.
type ProposedAction struct {
	ID         string          `json:"id"`
	ToolName   string          `json:"tool_name"`
	Arguments  json.RawMessage `json:"arguments"`
	EntityType string          `json:"entity_type,omitempty"`
	EntityID   string          `json:"entity_id,omitempty"`
}

// AgentTurn is the result of ProposeAction: either a plain answer, or an
// answer plus a proposed action awaiting approval.
type AgentTurn struct {
	Answer         string          `json:"answer"`
	ProposedAction *ProposedAction `json:"proposed_action,omitempty"`
}

// AgentActionResult is the outcome of approving or rejecting a proposed action.
type AgentActionResult struct {
	ID            string                 `json:"id"`
	Status        string                 `json:"status"` // executed | failed | rejected
	Result        map[string]interface{} `json:"result,omitempty"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	GitCommitError string                `json:"git_commit_error,omitempty"` // set only if a git_repo_id was supplied and the commit itself failed
}

// GitCommitter is the subset of *services.GitRepoService needed to auto-commit
// an interface after an approved action changes its git-tracked definition
// (pipeline/steps/connectivity). *services.GitRepoService satisfies this
// structurally.
type GitCommitter interface {
	CommitInterface(ctx context.Context, repoID string, req services.CommitInterfaceRequest, triggeredBy *string) (string, error)
}

// AgentService drives the tool-calling loop for Agent Mode.
type AgentService struct {
	db           *sql.DB
	llm          ToolCallingProvider
	scriptGen    *ScriptGeneratorService
	tools        *ToolRegistry
	gitCommitter GitCommitter // nil unless SetGitCommitter is called
}

// newAgentService constructs an AgentService with no mutating tools registered
// yet — only propose_pipeline_script (which needs no external dependencies)
// is available until SetDependencies is called from main.go.
func newAgentService(db *sql.DB, llm ToolCallingProvider, scriptGen *ScriptGeneratorService) *AgentService {
	return &AgentService{db: db, llm: llm, scriptGen: scriptGen, tools: NewDefaultToolRegistry(db, nil, nil)}
}

// SetDependencies wires the real tool dependencies once they exist in main.go
// (DLQService, ProcessingEngine are constructed long before AIService today).
// Call this before SetAnalyticsDependencies — it rebuilds the registry from
// scratch, which would otherwise wipe out tools registered additively.
func (a *AgentService) SetDependencies(redriver MessageRedriver, activator InterfaceActivator) {
	a.tools = NewDefaultToolRegistry(a.db, redriver, activator)
}

// SetAnalyticsDependencies adds the read-only analytics tool set onto the
// existing registry (additive — does not disturb tools already registered).
// Call this after SetDependencies.
func (a *AgentService) SetAnalyticsDependencies(analytics AnalyticsReporter, dlqStats DLQStatsReader, alerts AlertReader) {
	registerAnalyticsTools(a.tools, analytics, dlqStats, alerts)
}

// SetScriptVerificationDependencies adds verify_pipeline_script onto the
// existing registry (additive). Call this once *services.TransformationPipelineService
// exists in main.go.
func (a *AgentService) SetScriptVerificationDependencies(verifier ScriptVerifier) {
	registerScriptVerificationTool(a.tools, a.llm, a.scriptGen, verifier)
}

// maxAgentLoopIterations bounds how many read-only tool calls the model can
// chain before it must produce a final answer — prevents a runaway loop if
// a model keeps calling tools instead of concluding.
const maxAgentLoopIterations = 4

// ProposeAction asks the model to answer, or to work the question via tools.
// Read-only tools (RequiresApproval=false) execute immediately and their
// result is fed back so the model can keep reasoning toward a final answer.
// The first tool call that requires approval stops the loop and returns a
// ProposedAction — nothing behind RequiresApproval ever executes here.
func (a *AgentService) ProposeAction(ctx context.Context, sessionID, userID, question string) (*AgentTurn, error) {
	if a.llm == nil {
		return nil, fmt.Errorf("agent mode is not available: current provider does not support tool calling")
	}

	modelName := a.llm.ChatModelName()
	if cap, known := ResolveModelProfile(modelName); known && !cap.SupportsTools {
		return &AgentTurn{Answer: fmt.Sprintf(unsupportedToolModelMsg, modelName)}, nil
	}

	messages := []ChatMessage{
		{Role: "system", Content: agentSystemPrompt},
		{Role: "user", Content: question},
	}

	for i := 0; i < maxAgentLoopIterations; i++ {
		result, err := a.llm.ChatWithTools(ctx, messages, a.tools.Specs())
		if err != nil {
			return nil, err
		}

		if len(result.ToolCalls) == 0 {
			answer := result.Content
			// Only ambiguous on the FIRST iteration — zero tool calls there could
			// mean "no tool needed" or "model can't call tools at all". Once a
			// later iteration is reached, a tool call already succeeded earlier
			// this turn (see below), so the model has already proven it supports
			// tool calling and the diagnostic would be actively wrong here.
			if i == 0 {
				if _, known := ResolveModelProfile(modelName); !known {
					answer += fmt.Sprintf(unverifiedToolModelSuffix, modelName)
				}
			}
			return &AgentTurn{Answer: answer}, nil
		}

		// Only one proposed/pending tool call is surfaced per turn.
		call := result.ToolCalls[0]
		tool, ok := a.tools.Get(call.Name)
		if !ok {
			return &AgentTurn{Answer: result.Content}, nil
		}

		if tool.RequiresApproval {
			return a.proposeForApproval(ctx, sessionID, userID, result.Content, tool, call)
		}

		// Read-only tool: safe to run without a human in the loop. Execute now
		// and feed the result back so the model can keep reasoning.
		execResult, execErr := tool.Execute(ctx, call.Arguments)
		var toolResultJSON []byte
		if execErr != nil {
			toolResultJSON, _ = json.Marshal(map[string]interface{}{"error": execErr.Error()})
		} else {
			toolResultJSON, _ = json.Marshal(execResult)
		}
		messages = append(messages,
			ChatMessage{Role: "assistant", ToolCalls: []ToolCall{call}},
			ChatMessage{Role: "tool", Content: string(toolResultJSON)},
		)
	}

	return &AgentTurn{Answer: "I gathered the requested data but couldn't finish summarizing it in time — try a more specific follow-up question."}, nil
}

// proposeForApproval finalizes a RequiresApproval tool's arguments (running
// Prepare if the tool declares one) and records the proposal for a human to
// approve or reject. Never calls Execute.
func (a *AgentService) proposeForApproval(ctx context.Context, sessionID, userID, answer string, tool *Tool, call ToolCall) (*AgentTurn, error) {
	finalArgs := call.Arguments
	if tool.Prepare != nil {
		prepared, _, err := tool.Prepare(ctx, call.Arguments)
		if err != nil {
			return nil, fmt.Errorf("preparing %s: %w", tool.Name, err)
		}
		finalArgs = prepared
	}

	entityID := bestEffortEntityID(finalArgs)
	proposed := &ProposedAction{
		ID:         uuid.New().String(),
		ToolName:   tool.Name,
		Arguments:  finalArgs,
		EntityType: tool.EntityType,
		EntityID:   entityID,
	}

	if a.db != nil {
		var uid interface{}
		if userID != "" {
			uid = userID
		}
		_, err := a.db.ExecContext(ctx, `
			INSERT INTO ai_agent_actions (id, session_id, user_id, tool_name, arguments, status, entity_type, entity_id, created_at)
			VALUES ($1, $2, $3, $4, $5, 'proposed', $6, $7, $8)
		`, proposed.ID, sessionID, uid, proposed.ToolName, []byte(proposed.Arguments), proposed.EntityType, proposed.EntityID, time.Now())
		if err != nil {
			return nil, fmt.Errorf("record proposed action: %w", err)
		}
	}

	return &AgentTurn{Answer: answer, ProposedAction: proposed}, nil
}

// ApproveAction executes a previously proposed tool call and records the
// outcome. gitRepoID is optional — when supplied and the tool's EntityType
// is "pipeline_step", the interface owning that step is auto-committed to
// the given git repo after a successful execution (see commitToGit). A git
// commit failure never rolls back the already-applied change: the DB write
// is the source of truth, git is a secondary audit/undo trail.
func (a *AgentService) ApproveAction(ctx context.Context, actionID, approvedByUserID, gitRepoID string) (*AgentActionResult, error) {
	if a.db == nil {
		return nil, fmt.Errorf("agent actions require a database connection")
	}

	var toolName, status string
	var argsJSON []byte
	err := a.db.QueryRowContext(ctx, `
		SELECT tool_name, arguments, status FROM ai_agent_actions WHERE id = $1
	`, actionID).Scan(&toolName, &argsJSON, &status)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("action %s not found", actionID)
	}
	if err != nil {
		return nil, err
	}
	if status != "proposed" {
		return nil, fmt.Errorf("action %s is already %s", actionID, status)
	}

	tool, ok := a.tools.Get(toolName)
	if !ok {
		a.finalizeAction(ctx, actionID, "failed", nil, "tool no longer available", approvedByUserID)
		return &AgentActionResult{ID: actionID, Status: "failed", ErrorMessage: "tool no longer available"}, nil
	}

	execResult, execErr := tool.Execute(ctx, argsJSON)
	if execErr != nil {
		a.finalizeAction(ctx, actionID, "failed", nil, execErr.Error(), approvedByUserID)
		return &AgentActionResult{ID: actionID, Status: "failed", ErrorMessage: execErr.Error()}, nil
	}

	a.finalizeAction(ctx, actionID, "executed", execResult, "", approvedByUserID)
	result := &AgentActionResult{ID: actionID, Status: "executed", Result: execResult}

	if gitRepoID != "" && tool.EntityType == "pipeline_step" && a.gitCommitter != nil {
		if commitErr := a.commitToGit(ctx, gitRepoID, actionID, approvedByUserID, execResult); commitErr != nil {
			result.GitCommitError = commitErr.Error()
		}
	}

	return result, nil
}

// SetGitCommitter wires an auto-commit dependency for ApproveAction. Optional
// — Agent Mode works without it, just without the git-commit-on-approve step.
func (a *AgentService) SetGitCommitter(g GitCommitter) {
	a.gitCommitter = g
}

// commitToGit resolves the interface owning execResult's step_id and commits
// its current (already-updated) definition to the given git repo.
func (a *AgentService) commitToGit(ctx context.Context, gitRepoID, actionID, approvedBy string, execResult map[string]interface{}) error {
	stepID, _ := execResult["step_id"].(string)
	if stepID == "" {
		return fmt.Errorf("no step_id in result to resolve an interface")
	}

	var interfaceID string
	err := a.db.QueryRowContext(ctx, `
		SELECT tp.interface_id
		FROM transformation_steps ts
		JOIN transformation_pipelines tp ON tp.id = ts.pipeline_id
		WHERE ts.id = $1
	`, stepID).Scan(&interfaceID)
	if err != nil {
		return fmt.Errorf("resolve interface for step %s: %w", stepID, err)
	}

	var triggeredBy *string
	if approvedBy != "" {
		triggeredBy = &approvedBy
	}

	_, err = a.gitCommitter.CommitInterface(ctx, gitRepoID, services.CommitInterfaceRequest{
		InterfaceID:   interfaceID,
		CommitMessage: fmt.Sprintf("ezCompanion: update script (agent action %s)", actionID),
	}, triggeredBy)
	return err
}

// RejectAction marks a proposed action as rejected without executing it.
func (a *AgentService) RejectAction(ctx context.Context, actionID, rejectedByUserID string) error {
	if a.db == nil {
		return fmt.Errorf("agent actions require a database connection")
	}
	var uid interface{}
	if rejectedByUserID != "" {
		uid = rejectedByUserID
	}
	res, err := a.db.ExecContext(ctx, `
		UPDATE ai_agent_actions SET status = 'rejected', decided_by = $1, decided_at = NOW()
		WHERE id = $2 AND status = 'proposed'
	`, uid, actionID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("action %s not found or already decided", actionID)
	}
	return nil
}

func (a *AgentService) finalizeAction(ctx context.Context, actionID, status string, result map[string]interface{}, errMsg, decidedBy string) {
	resultJSON, _ := json.Marshal(result)
	var uid interface{}
	if decidedBy != "" {
		uid = decidedBy
	}
	_, _ = a.db.ExecContext(ctx, `
		UPDATE ai_agent_actions
		SET status = $1, result = $2, error_message = $3, decided_by = $4, decided_at = NOW()
		WHERE id = $5
	`, status, resultJSON, errMsg, uid, actionID)
}

// bestEffortEntityID pulls a recognisable ID out of a tool call's arguments
// for the audit row's entity_id column — purely descriptive, never parsed back.
func bestEffortEntityID(args json.RawMessage) string {
	var m map[string]interface{}
	if err := json.Unmarshal(args, &m); err != nil {
		return ""
	}
	for _, key := range []string{"interface_id", "dlq_id", "step_id"} {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
