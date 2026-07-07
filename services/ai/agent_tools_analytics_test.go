// services/ai/agent_tools_analytics_test.go
// Table-driven tests for the read-only analytics tools — confirm they're
// registered with RequiresApproval:false and that Execute forwards to the
// injected dependency without adding logic of its own.
package ai

import (
	"context"
	"encoding/json"
	"testing"

	"ezhealthkonnect/services"
)

type fakeAnalyticsReporter struct {
	lastReportReq *services.ReportExecutionRequest
	reportResult  *services.ReportResult
	summary       *services.MonitoringSummary
	healthCards   []*services.InterfaceHealthCard
}

func (f *fakeAnalyticsReporter) ExecuteReport(_ context.Context, req *services.ReportExecutionRequest) (*services.ReportResult, error) {
	f.lastReportReq = req
	if f.reportResult == nil {
		return &services.ReportResult{Columns: []string{"a"}, Rows: []services.ReportResultRow{{"a": 1}}}, nil
	}
	return f.reportResult, nil
}
func (f *fakeAnalyticsReporter) GetMonitoringSummary(_ context.Context) (*services.MonitoringSummary, error) {
	if f.summary == nil {
		return &services.MonitoringSummary{}, nil
	}
	return f.summary, nil
}
func (f *fakeAnalyticsReporter) GetInterfaceHealthCards(_ context.Context) ([]*services.InterfaceHealthCard, error) {
	return f.healthCards, nil
}

type fakeDLQStats struct {
	systemCalls    int
	interfaceCalls []string
}

func (f *fakeDLQStats) Stats(_ context.Context) (map[string]int, error) {
	f.systemCalls++
	return map[string]int{"pending": 3}, nil
}
func (f *fakeDLQStats) InterfaceStats(_ context.Context, interfaceID string) (map[string]int, error) {
	f.interfaceCalls = append(f.interfaceCalls, interfaceID)
	return map[string]int{"pending": 1}, nil
}

type fakeAlertReader struct {
	historyCalled int
	firedCalled   int
}

func (f *fakeAlertReader) ListFiredAlerts(_ context.Context, _ string, _ bool) ([]*services.FiredAlert, int, error) {
	f.firedCalled++
	return []*services.FiredAlert{{ID: "a1"}}, 1, nil
}
func (f *fakeAlertReader) ListAlertHistory(_ context.Context) ([]*services.FiredAlert, error) {
	f.historyCalled++
	return []*services.FiredAlert{{ID: "h1"}}, nil
}

func TestAnalyticsTools_AllReadOnly(t *testing.T) {
	reg := NewToolRegistry()
	registerAnalyticsTools(reg, &fakeAnalyticsReporter{}, &fakeDLQStats{}, &fakeAlertReader{})

	for _, name := range []string{"run_analytics_report", "get_monitoring_summary", "get_interface_health", "get_dlq_stats", "get_fired_alerts"} {
		tool, ok := reg.Get(name)
		if !ok {
			t.Fatalf("%s should be registered when all dependencies are present", name)
		}
		if tool.RequiresApproval {
			t.Errorf("%s must have RequiresApproval=false — it's read-only and must auto-execute in the bounded loop", name)
		}
	}
}

func TestAnalyticsTools_NilDependenciesOmitted(t *testing.T) {
	reg := NewToolRegistry()
	registerAnalyticsTools(reg, nil, nil, nil)

	for _, name := range []string{"run_analytics_report", "get_monitoring_summary", "get_interface_health", "get_dlq_stats", "get_fired_alerts"} {
		if _, ok := reg.Get(name); ok {
			t.Errorf("%s should not be registered when its dependency is nil", name)
		}
	}
}

func TestRunAnalyticsReportTool_ForwardsRequest(t *testing.T) {
	fake := &fakeAnalyticsReporter{}
	reg := NewToolRegistry()
	registerAnalyticsTools(reg, fake, nil, nil)
	tool, _ := reg.Get("run_analytics_report")

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"data_source":"dlq_analysis","dimensions":"interface_id","limit":50}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.lastReportReq == nil || fake.lastReportReq.DataSource != "dlq_analysis" || fake.lastReportReq.Limit != 50 {
		t.Fatalf("expected the request to be forwarded verbatim, got %+v", fake.lastReportReq)
	}
	if _, ok := result["columns"]; !ok {
		t.Fatalf("expected the ReportResult to round-trip into the result map, got %v", result)
	}
}

func TestGetDLQStatsTool_SystemVsInterfaceScoped(t *testing.T) {
	fake := &fakeDLQStats{}
	reg := NewToolRegistry()
	registerAnalyticsTools(reg, nil, fake, nil)
	tool, _ := reg.Get("get_dlq_stats")

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.systemCalls != 1 {
		t.Fatalf("expected Stats() (system-wide) when interface_id is omitted, got %d calls", fake.systemCalls)
	}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"interface_id":"iface-1"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.interfaceCalls) != 1 || fake.interfaceCalls[0] != "iface-1" {
		t.Fatalf("expected InterfaceStats('iface-1') when interface_id is set, got %+v", fake.interfaceCalls)
	}
}

func TestGetFiredAlertsTool_HistoryFlag(t *testing.T) {
	fake := &fakeAlertReader{}
	reg := NewToolRegistry()
	registerAnalyticsTools(reg, nil, nil, fake)
	tool, _ := reg.Get("get_fired_alerts")

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.firedCalled != 1 || fake.historyCalled != 0 {
		t.Fatalf("expected ListFiredAlerts by default, got fired=%d history=%d", fake.firedCalled, fake.historyCalled)
	}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"history":true}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.historyCalled != 1 {
		t.Fatalf("expected ListAlertHistory when history:true, got %d calls", fake.historyCalled)
	}
}
