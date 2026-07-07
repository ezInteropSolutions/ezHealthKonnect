// services/ai/agent_tools.go
// Agent Mode tool registry — each Tool is a thin adapter over an existing
// internal service method. Tools never contain business logic of their own;
// they only validate arguments and forward the call.
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"ezhealthkonnect/services/connectors"
)

// MessageRedriver redrives a single DLQ entry. Satisfied by dlqRedriverAdapter,
// which wraps *connectors.DLQService and always uses its configured default
// pipeline redriver (mirrors controllers/dlq_controller.go's own usage).
type MessageRedriver interface {
	RedriveMessage(ctx context.Context, dlqID, mode string) error
}

// dlqRedriverAdapter adapts *connectors.DLQService to MessageRedriver.
type dlqRedriverAdapter struct {
	svc *connectors.DLQService
}

// NewDLQRedriverAdapter wraps a DLQService for use as an agent tool dependency.
func NewDLQRedriverAdapter(svc *connectors.DLQService) MessageRedriver {
	return dlqRedriverAdapter{svc: svc}
}

func (a dlqRedriverAdapter) RedriveMessage(ctx context.Context, dlqID, mode string) error {
	// pipelineSvc=nil: ExecuteRedrive falls back to the service's own configured
	// redriver, exactly as controllers/dlq_controller.go already does.
	return a.svc.ExecuteRedrive(ctx, dlqID, mode, nil)
}

// InterfaceActivator starts/stops all connectors for an interface.
// *processing.ProcessingEngine already satisfies this structurally.
type InterfaceActivator interface {
	ActivateInterface(interfaceID string) error
	DeactivateInterface(interfaceID string) error
}

// Tool is one action the model may propose.
type Tool struct {
	Name        string
	Description string
	// Parameters is the JSON Schema describing the tool's arguments to the model.
	Parameters map[string]interface{}
	// EntityType/EntityID are derived from args for the audit row (best-effort).
	EntityType string
	// RequiresApproval gates Execute behind a human approval step (AgentService
	// writes a 'proposed' row and waits). When false, AgentService calls Execute
	// immediately and feeds the result back to the model to keep reasoning —
	// only safe for tools with no side effects (e.g. read-only analytics).
	RequiresApproval bool
	// Prepare runs once, at proposal time, before a RequiresApproval tool's
	// arguments are stored for later approval. It lets a tool finalize its own
	// arguments (e.g. running a generate-and-verify loop) so the human reviews
	// the exact thing Execute will later apply, not a placeholder. Optional —
	// nil means the model's raw arguments are stored as-is (all v1 tools).
	Prepare func(ctx context.Context, rawArgs json.RawMessage) (finalArgs json.RawMessage, preview string, err error)
	Execute func(ctx context.Context, args json.RawMessage) (map[string]interface{}, error)
}

// ToolRegistry holds the tools available to the agent for a given request.
type ToolRegistry struct {
	tools map[string]*Tool
	order []string
}

// NewToolRegistry creates an empty registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]*Tool)}
}

// Register adds a tool, preserving registration order for Specs().
func (r *ToolRegistry) Register(t *Tool) {
	if _, exists := r.tools[t.Name]; !exists {
		r.order = append(r.order, t.Name)
	}
	r.tools[t.Name] = t
}

// Get returns the tool with the given name, if registered.
func (r *ToolRegistry) Get(name string) (*Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Specs returns the ToolSpec list to pass to ChatWithTools, in registration order.
func (r *ToolRegistry) Specs() []ToolSpec {
	specs := make([]ToolSpec, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		specs = append(specs, ToolSpec{Name: t.Name, Description: t.Description, Parameters: t.Parameters})
	}
	return specs
}

// NewDefaultToolRegistry builds the v1 agent tool set. db may be nil (used only
// for the propose_pipeline_script existence check, which degrades to a no-op).
func NewDefaultToolRegistry(db *sql.DB, redriver MessageRedriver, activator InterfaceActivator) *ToolRegistry {
	reg := NewToolRegistry()

	if redriver != nil {
		reg.Register(&Tool{
			Name:        "retry_message",
			Description: "Redrive (retry) a single dead-lettered message, either from the step it failed at or from the beginning of the pipeline.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"dlq_id": map[string]interface{}{"type": "string", "description": "The DLQ entry ID to redrive."},
					"mode":   map[string]interface{}{"type": "string", "enum": []string{"from_failed_step", "from_start"}, "description": "Where to resume processing from."},
				},
				"required": []string{"dlq_id"},
			},
			EntityType:       "dlq_entry",
			RequiresApproval: true,
			Execute: func(ctx context.Context, args json.RawMessage) (map[string]interface{}, error) {
				var in struct {
					DLQID string `json:"dlq_id"`
					Mode  string `json:"mode"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return nil, fmt.Errorf("invalid arguments: %w", err)
				}
				if in.DLQID == "" {
					return nil, fmt.Errorf("dlq_id is required")
				}
				if in.Mode == "" {
					in.Mode = "from_failed_step"
				}
				if err := redriver.RedriveMessage(ctx, in.DLQID, in.Mode); err != nil {
					return nil, err
				}
				return map[string]interface{}{"dlq_id": in.DLQID, "mode": in.Mode, "status": "redriven"}, nil
			},
		})
	}

	if activator != nil {
		reg.Register(&Tool{
			Name:        "activate_interface",
			Description: "Activate an interface, starting all of its configured connectors.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"interface_id": map[string]interface{}{"type": "string", "description": "The interface ID to activate."},
				},
				"required": []string{"interface_id"},
			},
			EntityType:       "interface",
			RequiresApproval: true,
			Execute: func(ctx context.Context, args json.RawMessage) (map[string]interface{}, error) {
				id, err := requireStringArg(args, "interface_id")
				if err != nil {
					return nil, err
				}
				if err := activator.ActivateInterface(id); err != nil {
					return nil, err
				}
				return map[string]interface{}{"interface_id": id, "status": "active"}, nil
			},
		})

		reg.Register(&Tool{
			Name:        "deactivate_interface",
			Description: "Deactivate an interface, stopping all of its configured connectors.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"interface_id": map[string]interface{}{"type": "string", "description": "The interface ID to deactivate."},
				},
				"required": []string{"interface_id"},
			},
			EntityType:       "interface",
			RequiresApproval: true,
			Execute: func(ctx context.Context, args json.RawMessage) (map[string]interface{}, error) {
				id, err := requireStringArg(args, "interface_id")
				if err != nil {
					return nil, err
				}
				if err := activator.DeactivateInterface(id); err != nil {
					return nil, err
				}
				return map[string]interface{}{"interface_id": id, "status": "inactive"}, nil
			},
		})
	}

	// propose_pipeline_script performs no mutation itself — the frontend applies
	// it through the existing Generate/Insert flow in PropertiesPanel.js, which
	// already writes the script into the step and saves via the normal pipeline
	// save path. This tool only validates the step exists.
	reg.Register(&Tool{
		Name:        "propose_pipeline_script",
		Description: "Propose a JavaScript transform(input) script for a pipeline script step. Does not save anything — the user reviews and inserts it manually.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"step_id": map[string]interface{}{"type": "string", "description": "The pipeline step ID this script is for."},
				"script":  map[string]interface{}{"type": "string", "description": "The complete JavaScript transform(input) function body."},
			},
			"required": []string{"step_id", "script"},
		},
		EntityType:       "pipeline_step",
		RequiresApproval: true,
		Execute: func(ctx context.Context, args json.RawMessage) (map[string]interface{}, error) {
			var in struct {
				StepID string `json:"step_id"`
				Script string `json:"script"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if in.StepID == "" || in.Script == "" {
				return nil, fmt.Errorf("step_id and script are required")
			}
			if db != nil {
				var exists bool
				_ = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM transformation_steps WHERE id = $1)`, in.StepID).Scan(&exists)
				if !exists {
					return nil, fmt.Errorf("step %s not found", in.StepID)
				}
			}
			return map[string]interface{}{"step_id": in.StepID, "script": in.Script}, nil
		},
	})

	return reg
}

func requireStringArg(args json.RawMessage, key string) (string, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(args, &m); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	v, ok := m[key].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return v, nil
}
