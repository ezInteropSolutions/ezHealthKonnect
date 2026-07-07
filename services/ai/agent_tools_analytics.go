// services/ai/agent_tools_analytics.go
// Read-only Agent Mode tools: analytics reports, monitoring summary, DLQ
// stats, and fired/historical alerts. None of these mutate anything, so they
// run with RequiresApproval: false — AgentService executes them immediately
// and feeds the result back to the model so it can narrate a final answer
// (see the bounded loop in AgentService.ProposeAction).
package ai

import (
	"context"
	"encoding/json"

	"ezhealthkonnect/services"
)

// AnalyticsReporter is the read-only subset of *services.AnalyticsService
// used by the analytics tools.
type AnalyticsReporter interface {
	ExecuteReport(ctx context.Context, req *services.ReportExecutionRequest) (*services.ReportResult, error)
	GetMonitoringSummary(ctx context.Context) (*services.MonitoringSummary, error)
	GetInterfaceHealthCards(ctx context.Context) ([]*services.InterfaceHealthCard, error)
}

// DLQStatsReader is the read-only subset of *connectors.DLQService used here.
// *connectors.DLQService already satisfies this structurally.
type DLQStatsReader interface {
	Stats(ctx context.Context) (map[string]int, error)
	InterfaceStats(ctx context.Context, interfaceID string) (map[string]int, error)
}

// AlertReader is the read-only subset of *services.AlertRuleService used here.
type AlertReader interface {
	ListFiredAlerts(ctx context.Context, interfaceID string, countOnly bool) ([]*services.FiredAlert, int, error)
	ListAlertHistory(ctx context.Context) ([]*services.FiredAlert, error)
}

// registerAnalyticsTools adds the read-only analytics tool set directly onto
// an existing registry (additive — safe to call after or before the
// mutating tools are registered). Each tool is skipped if its dependency is
// nil, matching the convention in NewDefaultToolRegistry.
func registerAnalyticsTools(reg *ToolRegistry, analytics AnalyticsReporter, dlqStats DLQStatsReader, alerts AlertReader) {
	if analytics != nil {
		reg.Register(&Tool{
			Name: "run_analytics_report",
			Description: "Run a parameterized analytics report over message flow, pipeline performance, connector activity, DLQ analysis, or user activity data. Use this to answer questions about error rates, volumes, or trends over time.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"data_source": map[string]interface{}{
						"type": "string",
						"enum": []string{"message_flow", "pipeline_performance", "hipaa_audit", "connector_activity", "dlq_analysis", "user_activity"},
						"description": "Which dataset to query.",
					},
					"dimensions": map[string]interface{}{"type": "string", "description": "Comma-separated grouping dimensions, e.g. 'interface_id,day'."},
					"filters": map[string]interface{}{
						"type": "object",
						"description": "Optional filters, e.g. {\"time_range\":\"last_7d\"}. time_range one of last_1h,last_24h,last_7d,last_30d,last_90d.",
					},
					"limit": map[string]interface{}{"type": "integer", "description": "Max rows to return (default 1000, max 10000)."},
				},
				"required": []string{"data_source"},
			},
			EntityType:       "analytics_report",
			RequiresApproval: false, // read-only — safe to auto-execute
			Execute: func(ctx context.Context, args json.RawMessage) (map[string]interface{}, error) {
				var req services.ReportExecutionRequest
				if err := json.Unmarshal(args, &req); err != nil {
					return nil, err
				}
				result, err := analytics.ExecuteReport(ctx, &req)
				if err != nil {
					return nil, err
				}
				return toResultMap(result)
			},
		})

		reg.Register(&Tool{
			Name:             "get_monitoring_summary",
			Description:      "Get current KPIs: messages today, 24h error rate, average processing time, DLQ depth, and alert counts by severity, with 7-point sparkline trends.",
			Parameters:       map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			EntityType:       "monitoring_summary",
			RequiresApproval: false,
			Execute: func(ctx context.Context, _ json.RawMessage) (map[string]interface{}, error) {
				summary, err := analytics.GetMonitoringSummary(ctx)
				if err != nil {
					return nil, err
				}
				return toResultMap(summary)
			},
		})

		reg.Register(&Tool{
			Name:             "get_interface_health",
			Description:      "Get per-interface health cards: today's message count, error rate, hourly sparkline, and last activity time for every active interface.",
			Parameters:       map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			EntityType:       "interface_health",
			RequiresApproval: false,
			Execute: func(ctx context.Context, _ json.RawMessage) (map[string]interface{}, error) {
				cards, err := analytics.GetInterfaceHealthCards(ctx)
				if err != nil {
					return nil, err
				}
				return toResultMap(map[string]interface{}{"cards": cards})
			},
		})
	}

	if dlqStats != nil {
		reg.Register(&Tool{
			Name:        "get_dlq_stats",
			Description: "Get dead-letter queue counts by status, either system-wide or for a single interface.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"interface_id": map[string]interface{}{"type": "string", "description": "Optional — scope to one interface. Omit for system-wide counts."},
				},
			},
			EntityType:       "dlq_stats",
			RequiresApproval: false,
			Execute: func(ctx context.Context, args json.RawMessage) (map[string]interface{}, error) {
				var in struct {
					InterfaceID string `json:"interface_id"`
				}
				_ = json.Unmarshal(args, &in)
				var counts map[string]int
				var err error
				if in.InterfaceID != "" {
					counts, err = dlqStats.InterfaceStats(ctx, in.InterfaceID)
				} else {
					counts, err = dlqStats.Stats(ctx)
				}
				if err != nil {
					return nil, err
				}
				return map[string]interface{}{"counts": counts}, nil
			},
		})
	}

	if alerts != nil {
		reg.Register(&Tool{
			Name:        "get_fired_alerts",
			Description: "Get currently active (unacknowledged) alerts, or recent alert history (last 500), optionally scoped to one interface.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"interface_id": map[string]interface{}{"type": "string", "description": "Optional — scope to one interface."},
					"history":      map[string]interface{}{"type": "boolean", "description": "If true, return recent history instead of only active alerts."},
				},
			},
			EntityType:       "alerts",
			RequiresApproval: false,
			Execute: func(ctx context.Context, args json.RawMessage) (map[string]interface{}, error) {
				var in struct {
					InterfaceID string `json:"interface_id"`
					History     bool   `json:"history"`
				}
				_ = json.Unmarshal(args, &in)
				if in.History {
					rows, err := alerts.ListAlertHistory(ctx)
					if err != nil {
						return nil, err
					}
					return toResultMap(map[string]interface{}{"alerts": rows})
				}
				rows, count, err := alerts.ListFiredAlerts(ctx, in.InterfaceID, false)
				if err != nil {
					return nil, err
				}
				return toResultMap(map[string]interface{}{"alerts": rows, "count": count})
			},
		})
	}
}

// toResultMap converts any JSON-marshalable value into a map[string]interface{}
// so it fits Tool.Execute's return type — a plain round-trip, no business logic.
func toResultMap(v interface{}) (map[string]interface{}, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}
