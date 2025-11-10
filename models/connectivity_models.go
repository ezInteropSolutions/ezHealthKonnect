// models/connectivity_models.go
// Multi-Connectivity Models - OOB Connectors with Cloud Storage Support

package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ConnectivityType represents an OOB connector definition
type ConnectivityType struct {
	ID                  string          `json:"id" db:"id"`
	TypeName            string          `json:"type_name" db:"type_name"`
	Category            string          `json:"category" db:"category"` // "inbound" | "outbound"
	DisplayName         string          `json:"display_name" db:"display_name"`
	Description         string          `json:"description" db:"description"`
	Icon                string          `json:"icon" db:"icon"`
	Mode                string          `json:"mode" db:"mode"` // "push" | "pull" | "stream"
	SupportsCron        bool            `json:"supports_cron" db:"supports_cron"`
	RequiresAuth        bool            `json:"requires_auth" db:"requires_auth"`
	IsBidirectional     bool            `json:"is_bidirectional" db:"is_bidirectional"`
	ImplementationClass string          `json:"implementation_class" db:"implementation_class"`
	ConfigSchema        json.RawMessage `json:"config_schema" db:"config_schema"`
	DefaultConfig       json.RawMessage `json:"default_config" db:"default_config"`
	WizardTemplate      string          `json:"wizard_template" db:"wizard_template"`
	ParameterGroups     json.RawMessage `json:"parameter_groups" db:"parameter_groups"`
	ValidationRules     json.RawMessage `json:"validation_rules" db:"validation_rules"`
	IsActive            bool            `json:"is_active" db:"is_active"`
	IsBeta              bool            `json:"is_beta" db:"is_beta"`
	Priority            int             `json:"priority" db:"priority"`
	Version             string          `json:"version" db:"version"`
	DocumentationURL    string          `json:"documentation_url" db:"documentation_url"`
	CreatedAt           time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at" db:"updated_at"`
}

// InterfaceConnectivity represents connectivity configuration for an interface
type InterfaceConnectivity struct {
	ID                       string          `json:"id" db:"id"`
	InterfaceID              string          `json:"interface_id" db:"interface_id"`

	// Source (Inbound)
	SourceConnectivityTypeID string          `json:"source_connectivity_type_id" db:"source_connectivity_type_id"`
	SourceConfig             json.RawMessage `json:"source_config" db:"source_config"`
	SourceEnabled            bool            `json:"source_enabled" db:"source_enabled"`

	// Cron Scheduling
	CronEnabled    bool       `json:"cron_enabled" db:"cron_enabled"`
	CronExpression string     `json:"cron_expression" db:"cron_expression"`
	CronTimezone   string     `json:"cron_timezone" db:"cron_timezone"`
	NextRunAt      *time.Time `json:"next_run_at" db:"next_run_at"`
	LastRunAt      *time.Time `json:"last_run_at" db:"last_run_at"`
	LastRunStatus  string     `json:"last_run_status" db:"last_run_status"`

	// Target (Outbound)
	TargetConnectivityTypeID string          `json:"target_connectivity_type_id" db:"target_connectivity_type_id"`
	TargetConfig             json.RawMessage `json:"target_config" db:"target_config"`
	TargetEnabled            bool            `json:"target_enabled" db:"target_enabled"`

	// Multi-target support (future)
	AdditionalTargets json.RawMessage `json:"additional_targets" db:"additional_targets"`

	// Connection State
	ConnectionStatus     string     `json:"connection_status" db:"connection_status"`
	LastConnectionTest   *time.Time `json:"last_connection_test" db:"last_connection_test"`
	LastConnectionError  string     `json:"last_connection_error" db:"last_connection_error"`

	// Metadata
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// CronJob represents a scheduled job for pull-based connectors
type CronJob struct {
	ID                      string     `json:"id" db:"id"`
	InterfaceConnectivityID string     `json:"interface_connectivity_id" db:"interface_connectivity_id"`
	CronExpression          string     `json:"cron_expression" db:"cron_expression"`
	Timezone                string     `json:"timezone" db:"timezone"`
	IsEnabled               bool       `json:"is_enabled" db:"is_enabled"`
	NextExecution           *time.Time `json:"next_execution" db:"next_execution"`
	LastExecution           *time.Time `json:"last_execution" db:"last_execution"`
	ExecutionCount          int        `json:"execution_count" db:"execution_count"`
	FailureCount            int        `json:"failure_count" db:"failure_count"`
	ConsecutiveFailures     int        `json:"consecutive_failures" db:"consecutive_failures"`
	MaxRetries              int        `json:"max_retries" db:"max_retries"`
	RetryDelaySeconds       int        `json:"retry_delay_seconds" db:"retry_delay_seconds"`
	CircuitBreakerThreshold int        `json:"circuit_breaker_threshold" db:"circuit_breaker_threshold"`
	IsCircuitBroken         bool       `json:"is_circuit_broken" db:"is_circuit_broken"`
	CircuitBrokenAt         *time.Time `json:"circuit_broken_at" db:"circuit_broken_at"`
	CreatedAt               time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at" db:"updated_at"`
}

// ConnectivityExecutionLog represents execution history
type ConnectivityExecutionLog struct {
	ID                      string          `json:"id" db:"id"`
	InterfaceConnectivityID string          `json:"interface_connectivity_id" db:"interface_connectivity_id"`
	InterfaceID             string          `json:"interface_id" db:"interface_id"`
	ExecutionType           string          `json:"execution_type" db:"execution_type"`
	StartedAt               time.Time       `json:"started_at" db:"started_at"`
	CompletedAt             *time.Time      `json:"completed_at" db:"completed_at"`
	Status                  string          `json:"status" db:"status"`
	MessagesRetrieved       int             `json:"messages_retrieved" db:"messages_retrieved"`
	MessagesProcessed       int             `json:"messages_processed" db:"messages_processed"`
	MessagesFailed          int             `json:"messages_failed" db:"messages_failed"`
	ErrorDetails            json.RawMessage `json:"error_details" db:"error_details"`
	DurationMs              int             `json:"duration_ms" db:"duration_ms"`
	TriggeredBy             string          `json:"triggered_by" db:"triggered_by"`
	ExecutionMetadata       json.RawMessage `json:"execution_metadata" db:"execution_metadata"`
}

// ConnectivityTypeFilter for querying connectivity types
type ConnectivityTypeFilter struct {
	Category    string // "inbound" or "outbound"
	Mode        string // "push", "pull", "stream"
	RequiresAuth *bool
	SupportsCron *bool
	IsActive    *bool
	IsBeta      *bool
}

// InterfaceConnectivityWithDetails includes related connector information
type InterfaceConnectivityWithDetails struct {
	InterfaceConnectivity
	SourceConnectorType *ConnectivityType `json:"source_connector_type,omitempty"`
	TargetConnectorType *ConnectivityType `json:"target_connector_type,omitempty"`
	CronJob             *CronJob          `json:"cron_job,omitempty"`
}

// ExecutionLogFilter for querying execution logs
type ExecutionLogFilter struct {
	InterfaceID    string
	ExecutionType  string
	Status         string
	StartDateFrom  *time.Time
	StartDateTo    *time.Time
	Limit          int
	Offset         int
}

// ExecutionStats provides execution statistics
type ExecutionStats struct {
	TotalExecutions    int     `json:"total_executions"`
	SuccessfulExecutions int   `json:"successful_executions"`
	FailedExecutions     int   `json:"failed_executions"`
	TotalMessagesRetrieved int `json:"total_messages_retrieved"`
	TotalMessagesProcessed int `json:"total_messages_processed"`
	TotalMessagesFailed    int `json:"total_messages_failed"`
	SuccessRate         float64 `json:"success_rate"`
	AverageDurationMs   float64 `json:"average_duration_ms"`
	LastExecution       *time.Time `json:"last_execution,omitempty"`
}

// CronSchedule represents a user-friendly cron configuration
type CronSchedule struct {
	Mode       string     `json:"mode"`        // "simple" or "advanced"
	Expression string     `json:"expression"`  // Cron expression
	Timezone   string     `json:"timezone"`
	NextRuns   []time.Time `json:"next_runs,omitempty"` // Preview of next 5 runs
	HumanReadable string  `json:"human_readable,omitempty"` // e.g., "Every 5 minutes"
}

// SimpleCronConfig for user-friendly cron configuration
type SimpleCronConfig struct {
	Frequency      string     `json:"frequency"`       // "minutes", "hourly", "daily", "weekly", "monthly"
	IntervalMinutes int       `json:"interval_minutes,omitempty"` // For "minutes" frequency
	IntervalHours   int       `json:"interval_hours,omitempty"`   // For "hourly" frequency
	Minute          int       `json:"minute,omitempty"`           // Minute of hour
	Time            string    `json:"time,omitempty"`             // HH:MM for daily/weekly/monthly
	Days            []int     `json:"days,omitempty"`             // Day of week (0-6) for weekly
	DayOfMonth      int       `json:"day_of_month,omitempty"`     // 1-31 for monthly
	Timezone        string    `json:"timezone"`
}

// ToCronExpression converts SimpleCronConfig to cron expression
func (s *SimpleCronConfig) ToCronExpression() string {
	switch s.Frequency {
	case "minutes":
		return fmt.Sprintf("*/%d * * * *", s.IntervalMinutes)
	case "hourly":
		return fmt.Sprintf("%d */%d * * *", s.Minute, s.IntervalHours)
	case "daily":
		parts := strings.Split(s.Time, ":")
		if len(parts) == 2 {
			return fmt.Sprintf("%s %s * * *", parts[1], parts[0])
		}
	case "weekly":
		parts := strings.Split(s.Time, ":")
		if len(parts) == 2 {
			dayList := make([]string, len(s.Days))
			for i, d := range s.Days {
				dayList[i] = fmt.Sprintf("%d", d)
			}
			return fmt.Sprintf("%s %s * * %s", parts[1], parts[0], strings.Join(dayList, ","))
		}
	case "monthly":
		parts := strings.Split(s.Time, ":")
		if len(parts) == 2 {
			return fmt.Sprintf("%s %s %d * *", parts[1], parts[0], s.DayOfMonth)
		}
	}
	return "*/5 * * * *" // Default: every 5 minutes
}
