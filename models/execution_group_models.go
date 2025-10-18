package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// ExecutionGroup represents a group of steps that execute together (parallel or inline)
type ExecutionGroup struct {
	ID            string    `json:"id" db:"id"`
	PipelineID    string    `json:"pipeline_id" db:"pipeline_id"`
	GroupID       string    `json:"group_id" db:"group_id"` // User-friendly ID (e.g., "parallel_1")
	GroupType     string    `json:"group_type" db:"group_type"` // "parallel" or "inline"
	Layer         string    `json:"layer" db:"layer"` // "pre", "core", "post"
	Sequence      int       `json:"sequence" db:"sequence"`
	MergeStrategy string    `json:"merge_strategy" db:"merge_strategy"` // For parallel: deep_merge, shallow_merge, override
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`

	// Relationships (not in DB, populated by joins)
	Steps        []TransformationStep `json:"steps,omitempty" db:"-"`
	DependsOn    []string            `json:"depends_on,omitempty" db:"-"` // Array of group_ids
}

// StepDependency represents a dependency between execution groups
type StepDependency struct {
	ID               string    `json:"id" db:"id"`
	GroupID          string    `json:"group_id" db:"group_id"`
	DependsOnGroupID string    `json:"depends_on_group_id" db:"depends_on_group_id"`
	DependencyType   string    `json:"dependency_type" db:"dependency_type"` // completion, success, data_flow
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

// PipelineVisualConfig stores visual configuration for drag-drop UI
type PipelineVisualConfig struct {
	ID           string          `json:"id" db:"id"`
	PipelineID   string          `json:"pipeline_id" db:"pipeline_id"`
	CanvasConfig JSONMap         `json:"canvas_config" db:"canvas_config"`
	LayerModes   JSONMap         `json:"layer_modes" db:"layer_modes"` // {"pre": "parallel", "core": "waterfall"}
	Connections  JSONArray       `json:"connections" db:"connections"` // Array of connection objects
	Version      int             `json:"version" db:"version"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
}

// ExecutionFlowMetadata stores cached execution order for performance
type ExecutionFlowMetadata struct {
	ID              string    `json:"id" db:"id"`
	PipelineID      string    `json:"pipeline_id" db:"pipeline_id"`
	ResolvedOrder   JSONArray `json:"resolved_order" db:"resolved_order"` // Pre-computed execution order
	LastComputedAt  time.Time `json:"last_computed_at" db:"last_computed_at"`
	IsValid         bool      `json:"is_valid" db:"is_valid"`
}

// ExecutionOrderGroup represents a resolved execution group with dependencies
type ExecutionOrderGroup struct {
	GroupID      string                 `json:"group_id"`
	GroupType    string                 `json:"group_type"` // parallel or inline
	Layer        string                 `json:"layer"`
	Sequence     int                    `json:"sequence"`
	Steps        []TransformationStep   `json:"steps"`
	DependsOn    []string               `json:"depends_on"` // Array of group_ids this group depends on
}

// JSONMap is a custom type for JSONB map columns
type JSONMap map[string]interface{}

// Value implements driver.Valuer for database storage
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements sql.Scanner for database retrieval
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}

	return json.Unmarshal(bytes, j)
}

// JSONArray is a custom type for JSONB array columns
type JSONArray []interface{}

// Value implements driver.Valuer for database storage
func (j JSONArray) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements sql.Scanner for database retrieval
func (j *JSONArray) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}

	return json.Unmarshal(bytes, j)
}

// PipelineWithGroups is a complete pipeline with execution groups
type PipelineWithGroups struct {
	TransformationPipeline
	ExecutionGroups []ExecutionGroup      `json:"execution_groups"`
	VisualConfig    *PipelineVisualConfig `json:"visual_config,omitempty"`
	FlowMetadata    *ExecutionFlowMetadata `json:"flow_metadata,omitempty"`
}

// HybridExecutionRequest represents a request to execute a hybrid pipeline
type HybridExecutionRequest struct {
	PipelineID string                 `json:"pipeline_id"`
	MessageID  string                 `json:"message_id"`
	InputData  map[string]interface{} `json:"input_data"`
}

// HybridExecutionResult represents the result of hybrid execution
type HybridExecutionResult struct {
	Success        bool                    `json:"success"`
	OutputData     map[string]interface{}  `json:"output_data"`
	ExecutionLog   []GroupExecutionLog     `json:"execution_log"`
	TotalTimeMs    int64                   `json:"total_time_ms"`
	Error          string                  `json:"error,omitempty"`
}

// GroupExecutionLog tracks execution of an execution group
type GroupExecutionLog struct {
	GroupID        string                 `json:"group_id"`
	GroupType      string                 `json:"group_type"` // parallel or inline
	StartedAt      time.Time              `json:"started_at"`
	CompletedAt    time.Time              `json:"completed_at"`
	DurationMs     int64                  `json:"duration_ms"`
	Success        bool                   `json:"success"`
	Error          string                 `json:"error,omitempty"`
	StepsExecuted  []StepExecutionLog     `json:"steps_executed"`
}

// VisualPipeline represents pipeline in visual/UI format
type VisualPipeline struct {
	ID          string                 `json:"id"`
	InterfaceID string                 `json:"interface_id"`
	MessageType string                 `json:"message_type"`
	Name        string                 `json:"name"`
	Layers      map[string]VisualLayer `json:"layers"` // "pre", "core", "post"
}

// VisualLayer represents a layer in visual format
type VisualLayer struct {
	Mode   string             `json:"mode"`  // "parallel", "waterfall", "hybrid"
	Groups []VisualGroup      `json:"groups"`
}

// VisualGroup represents an execution group in visual format
type VisualGroup struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"` // "parallel" or "inline"
	Steps     []VisualStep           `json:"steps"`
	DependsOn []string               `json:"depends_on"`
	Position  map[string]interface{} `json:"position,omitempty"` // {x, y, width, height}
}

// VisualStep represents a step in visual format
type VisualStep struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Type            string                 `json:"type"` // step_type
	TemplateID      string                 `json:"template_id,omitempty"`
	Config          map[string]interface{} `json:"config"`
	Required        bool                   `json:"required"`
	Position        map[string]interface{} `json:"position,omitempty"`
	ScriptContent   string                 `json:"script_content,omitempty"`
	OnErrorStrategy string                 `json:"on_error_strategy"`
}

// ConvertVisualToDatabase converts visual pipeline format to database models
func ConvertVisualToDatabase(visual *VisualPipeline) (*TransformationPipeline, []ExecutionGroup, []StepDependency, error) {
	pipeline := &TransformationPipeline{
		InterfaceID:  visual.InterfaceID,
		MessageType:  visual.MessageType,
		PipelineName: visual.Name,
		Enabled:      true,
		Version:      1,
	}

	var executionGroups []ExecutionGroup
	var dependencies []StepDependency
	var steps []TransformationStep

	sequence := 0

	// Process each layer
	for layerName, layer := range visual.Layers {
		for _, group := range layer.Groups {
			execGroup := ExecutionGroup{
				GroupID:       group.ID,
				GroupType:     group.Type,
				Layer:         layerName,
				Sequence:      sequence,
				MergeStrategy: "deep_merge",
			}

			// Add group steps
			for _, step := range group.Steps {
				transformStep := TransformationStep{
					StepName:        step.Name,
					StepType:        step.Type,
					Sequence:        sequence,
					Layer:           layerName,
					Required:        step.Required,
					TimeoutMs:       5000,
					Enabled:         true,
					Config:          step.Config,
					ScriptContent:   &step.ScriptContent,
					OnErrorStrategy: step.OnErrorStrategy,
				}

				steps = append(steps, transformStep)
				sequence += 10
			}

			// Add dependencies
			for _, depGroupID := range group.DependsOn {
				dep := StepDependency{
					DependsOnGroupID: depGroupID,
					DependencyType:   "completion",
				}
				dependencies = append(dependencies, dep)
			}

			executionGroups = append(executionGroups, execGroup)
		}
	}

	pipeline.Steps = steps

	return pipeline, executionGroups, dependencies, nil
}

// ConvertDatabaseToVisual converts database models to visual pipeline format
func ConvertDatabaseToVisual(pipeline *TransformationPipeline, groups []ExecutionGroup, deps []StepDependency) *VisualPipeline {
	visual := &VisualPipeline{
		ID:          pipeline.ID,
		InterfaceID: pipeline.InterfaceID,
		MessageType: pipeline.MessageType,
		Name:        pipeline.PipelineName,
		Layers:      make(map[string]VisualLayer),
	}

	// Group by layer
	layerGroups := make(map[string][]ExecutionGroup)
	for _, group := range groups {
		layerGroups[group.Layer] = append(layerGroups[group.Layer], group)
	}

	// Build visual layers
	for layerName, layerExecGroups := range layerGroups {
		var visualGroups []VisualGroup

		for _, execGroup := range layerExecGroups {
			var visualSteps []VisualStep

			// Get steps for this group
			for _, step := range execGroup.Steps {
				scriptContent := ""
				if step.ScriptContent != nil {
					scriptContent = *step.ScriptContent
				}

				visualStep := VisualStep{
					ID:              step.ID,
					Name:            step.StepName,
					Type:            step.StepType,
					Config:          step.Config,
					Required:        step.Required,
					ScriptContent:   scriptContent,
					OnErrorStrategy: step.OnErrorStrategy,
				}

				visualSteps = append(visualSteps, visualStep)
			}

			// Get dependencies
			var dependsOn []string
			for _, dep := range deps {
				if dep.GroupID == execGroup.ID {
					dependsOn = append(dependsOn, dep.DependsOnGroupID)
				}
			}

			visualGroup := VisualGroup{
				ID:        execGroup.GroupID,
				Type:      execGroup.GroupType,
				Steps:     visualSteps,
				DependsOn: dependsOn,
			}

			visualGroups = append(visualGroups, visualGroup)
		}

		// Determine layer mode
		mode := "hybrid"
		if len(visualGroups) == 1 {
			if visualGroups[0].Type == "parallel" {
				mode = "parallel"
			} else {
				mode = "waterfall"
			}
		}

		visual.Layers[layerName] = VisualLayer{
			Mode:   mode,
			Groups: visualGroups,
		}
	}

	return visual
}
