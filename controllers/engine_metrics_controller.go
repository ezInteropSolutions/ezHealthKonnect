// controllers/engine_metrics_controller.go
//
// EngineMetricsController exposes the 13 ezhk_* Prometheus metrics
// (services/metrics/metrics.go) as actionable current-value JSON, read
// directly from the in-process Prometheus registry via Gather() — no
// Prometheus server or scrape step involved. Powers the native "Engine
// Metrics" page (public/engine-metrics.html).
//
// Beyond raw current values, each card carries:
//   - a status ("ok"/"warning"/"critical") so the UI can flag what actually
//     needs attention instead of showing a neutral number, and
//   - for per-label gauges/counters (queue depth, failures, circuit
//     breakers), the single worst-offending interface/step by name — not
//     just an aggregate total — so the operator knows exactly where to look.
//
// Route (registered under /api/system): GET /engine-metrics
package controllers

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/client_golang/prometheus"
)

// Sensible operational defaults for engine-level gauges that have no
// per-interface configurable threshold (unlike interface_alert_thresholds,
// which only covers flag-rate/DLQ-depth per interface). These are safe
// starting points for a typical healthcare interface engine, not tuned
// per-deployment config.
const (
	latencyWarnMs    = 500  // matches the 0.5s bucket boundary in metrics.go's slaLatencyBuckets
	latencyCritMs    = 2500 // matches the 2.5s bucket boundary in metrics.go's slaLatencyBuckets
	queueWaitWarnMs  = 5000 // matches the 5s bucket boundary in metrics.go's QueueWaitDuration buckets
	queueWaitCritMs  = 30000
	dlqDepthWarn     = 1
	dlqDepthCrit     = 25
	workerQueueWarn  = 10
	workerQueueCrit  = 50
)

// EngineMetricsController handles the native engine metrics endpoint.
type EngineMetricsController struct {
	db *sql.DB
}

// NewEngineMetricsController creates a new controller. db is used only to
// resolve UUIDs (interface_id, step_id) found in metric labels into
// human-readable names for drill-down detail — no metric data itself comes
// from the database.
func NewEngineMetricsController(db *sql.DB) *EngineMetricsController {
	return &EngineMetricsController{db: db}
}

// RegisterRoutes mounts the engine-metrics endpoint under the supplied router group.
func (ec *EngineMetricsController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/engine-metrics", ec.GetEngineMetrics)
}

// GetEngineMetrics gathers the current value of every ezhk_* metric family,
// rolls them up into KPI-card-friendly groups, and attaches status/drill-down
// detail so each card is actionable rather than purely informational.
func (ec *EngineMetricsController) GetEngineMetrics(c *gin.Context) {
	ctx := c.Request.Context()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}

	byName := make(map[string]*dto.MetricFamily, len(families))
	for _, mf := range families {
		byName[mf.GetName()] = mf
	}

	// ── Throughput ───────────────────────────────────────────────────────────
	byStatus := sumByLabel(byName["ezhk_messages_processed_total"], "status")
	worstFailInterfaceID, failedCount := worstByLabel(byName["ezhk_messages_processed_total"], "interface_id", "status", "failed")
	throughput := gin.H{
		"messages_received_total":      sumCounter(byName["ezhk_messages_received_total"]),
		"messages_processed_total":     sumCounter(byName["ezhk_messages_processed_total"]),
		"messages_processed_by_status": byStatus,
		"step_executions_total":        sumCounter(byName["ezhk_pipeline_step_executions_total"]),
	}
	if failedCount > 0 {
		throughput["worst_failing_interface_id"] = worstFailInterfaceID
		throughput["worst_failing_interface_name"] = ec.resolveInterfaceName(ctx, worstFailInterfaceID)
		throughput["worst_failing_count"] = failedCount
		throughput["status"] = "warning"
	} else {
		throughput["status"] = "ok"
	}

	// ── Latency ──────────────────────────────────────────────────────────────
	avgProcessing := histogramAvgMs(byName["ezhk_messages_processing_duration_seconds"])
	avgStep := histogramAvgMs(byName["ezhk_pipeline_step_duration_seconds"])
	avgQueueWait := histogramAvgMs(byName["ezhk_queue_wait_duration_seconds"])
	latency := gin.H{
		"avg_processing_duration_ms": avgProcessing,
		"avg_processing_status":      statusForThreshold(avgProcessing, latencyWarnMs, latencyCritMs),
		"avg_step_duration_ms":       avgStep,
		"avg_step_status":            statusForThreshold(avgStep, latencyWarnMs, latencyCritMs),
		"avg_queue_wait_duration_ms": avgQueueWait,
		"avg_queue_wait_status":      statusForThreshold(avgQueueWait, queueWaitWarnMs, queueWaitCritMs),
	}

	// ── Queue & Backpressure ─────────────────────────────────────────────────
	queueDepthTotal := sumGauge(byName["ezhk_backpressure_queue_depth"])
	worstQueueInterfaceID, worstQueueDepth := worstByLabel(byName["ezhk_backpressure_queue_depth"], "interface_id", "", "")
	queue := gin.H{
		"worker_queue_depth_total": queueDepthTotal,
		"worker_queue_status":      statusForThreshold(queueDepthTotal, workerQueueWarn, workerQueueCrit),
		"active_interfaces":        singleValue(byName["ezhk_engine_active_interfaces"]),
	}
	if worstQueueDepth > 0 {
		queue["worst_interface_id"] = worstQueueInterfaceID
		queue["worst_interface_name"] = ec.resolveInterfaceName(ctx, worstQueueInterfaceID)
		queue["worst_interface_depth"] = worstQueueDepth
	}

	// ── Reliability ──────────────────────────────────────────────────────────
	dlqDepth := singleValue(byName["ezhk_dlq_depth"])
	openStepIDs := labelsAtGaugeValue(byName["ezhk_circuit_breaker_state"], "executor_id", 2)
	halfOpenCount := countGaugeAtValue(byName["ezhk_circuit_breaker_state"], 1)
	reliability := gin.H{
		"dlq_enqueued_total":               sumCounter(byName["ezhk_dlq_enqueued_total"]),
		"dlq_depth":                        dlqDepth,
		"dlq_depth_status":                 statusForThreshold(dlqDepth, dlqDepthWarn, dlqDepthCrit),
		"recovery_messages_requeued_total": singleValue(byName["ezhk_recovery_messages_requeued_total"]),
		"circuit_breakers_open":            len(openStepIDs),
		"circuit_breakers_half_open":       halfOpenCount,
	}
	if len(openStepIDs) > 0 {
		reliability["circuit_breakers_status"] = "critical"
		reliability["circuit_breakers_open_detail"] = ec.resolveStepDetails(ctx, openStepIDs)
	} else if halfOpenCount > 0 {
		reliability["circuit_breakers_status"] = "warning"
	} else {
		reliability["circuit_breakers_status"] = "ok"
	}

	// ── Capacity ─────────────────────────────────────────────────────────────
	capacity := gin.H{
		"active_connectors_total":   sumGauge(byName["ezhk_connectors_active_total"]),
		"active_connectors_by_type": sumByLabel(byName["ezhk_connectors_active_total"], "connector_type"),
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"throughput":   throughput,
		"latency":      latency,
		"queue":        queue,
		"reliability":  reliability,
		"capacity":     capacity,
	})
}

// resolveInterfaceName looks up an interface's display name by UUID, falling
// back to the raw ID if the lookup fails (matches the COALESCE(name, id)
// fallback pattern used elsewhere, e.g. monitoring_controller.go's Alerts).
func (ec *EngineMetricsController) resolveInterfaceName(ctx context.Context, interfaceID string) string {
	if ec.db == nil || interfaceID == "" {
		return interfaceID
	}
	var name string
	err := ec.db.QueryRowContext(ctx, "SELECT name FROM interfaces WHERE id = $1::uuid", interfaceID).Scan(&name)
	if err != nil {
		return interfaceID
	}
	return name
}

// stepDetail is the drill-down shape returned for a tripped circuit breaker.
type stepDetail struct {
	StepID        string `json:"step_id"`
	StepType      string `json:"step_type"`
	InterfaceName string `json:"interface_name"`
}

// resolveStepDetails resolves circuit-breaker executor_id labels (which are
// raw transformation_steps.id values, see services/executors/enrichment/circuit_breaker.go)
// into their step type and owning interface name.
func (ec *EngineMetricsController) resolveStepDetails(ctx context.Context, stepIDs []string) []stepDetail {
	details := make([]stepDetail, 0, len(stepIDs))
	if ec.db == nil {
		for _, id := range stepIDs {
			details = append(details, stepDetail{StepID: id})
		}
		return details
	}
	for _, id := range stepIDs {
		var stepType, ifaceName string
		err := ec.db.QueryRowContext(ctx, `
			SELECT ts.step_type, i.name
			FROM transformation_steps ts
			JOIN transformation_pipelines tp ON tp.id = ts.pipeline_id
			JOIN interfaces i ON i.id = tp.interface_id
			WHERE ts.id = $1::uuid`, id).Scan(&stepType, &ifaceName)
		if err != nil {
			details = append(details, stepDetail{StepID: id})
			continue
		}
		details = append(details, stepDetail{StepID: id, StepType: stepType, InterfaceName: ifaceName})
	}
	return details
}

// statusForThreshold classifies a value against warn/crit boundaries.
func statusForThreshold(value, warn, crit float64) string {
	if value >= crit {
		return "critical"
	}
	if value >= warn {
		return "warning"
	}
	return "ok"
}

// sumCounter sums a counter's value across every label combination.
func sumCounter(mf *dto.MetricFamily) float64 {
	if mf == nil {
		return 0
	}
	var total float64
	for _, m := range mf.GetMetric() {
		total += m.GetCounter().GetValue()
	}
	return total
}

// sumGauge sums a gauge's value across every label combination.
func sumGauge(mf *dto.MetricFamily) float64 {
	if mf == nil {
		return 0
	}
	var total float64
	for _, m := range mf.GetMetric() {
		total += m.GetGauge().GetValue()
	}
	return total
}

// singleValue reads a no-label counter or gauge's single value.
func singleValue(mf *dto.MetricFamily) float64 {
	if mf == nil {
		return 0
	}
	for _, m := range mf.GetMetric() {
		if m.GetGauge() != nil {
			return m.GetGauge().GetValue()
		}
		if m.GetCounter() != nil {
			return m.GetCounter().GetValue()
		}
	}
	return 0
}

// sumByLabel groups a counter/gauge's values by the value of a given label
// name, summing across any other labels. Used for small breakdowns like
// "messages processed by status" or "active connectors by type".
func sumByLabel(mf *dto.MetricFamily, labelName string) map[string]float64 {
	result := map[string]float64{}
	if mf == nil {
		return result
	}
	for _, m := range mf.GetMetric() {
		labelValue := labelValueOf(m, labelName)
		if labelValue == "" {
			continue
		}
		var v float64
		if m.GetGauge() != nil {
			v = m.GetGauge().GetValue()
		} else if m.GetCounter() != nil {
			v = m.GetCounter().GetValue()
		}
		result[labelValue] += v
	}
	return result
}

// worstByLabel returns the single highest-value label (groupLabel) for a
// counter/gauge, optionally restricted to series where filterLabel equals
// filterValue. Used to name the specific worst-offending interface/step
// instead of only showing an aggregate total.
func worstByLabel(mf *dto.MetricFamily, groupLabel, filterLabel, filterValue string) (string, float64) {
	var bestLabel string
	var bestValue float64
	if mf == nil {
		return "", 0
	}
	for _, m := range mf.GetMetric() {
		if filterLabel != "" && labelValueOf(m, filterLabel) != filterValue {
			continue
		}
		group := labelValueOf(m, groupLabel)
		if group == "" {
			continue
		}
		var v float64
		if m.GetGauge() != nil {
			v = m.GetGauge().GetValue()
		} else if m.GetCounter() != nil {
			v = m.GetCounter().GetValue()
		}
		if v > bestValue {
			bestValue = v
			bestLabel = group
		}
	}
	return bestLabel, bestValue
}

// labelsAtGaugeValue returns every label value (for labelName) whose gauge
// currently equals the given value — e.g. every stepID whose circuit breaker
// is open (value 2).
func labelsAtGaugeValue(mf *dto.MetricFamily, labelName string, value float64) []string {
	var result []string
	if mf == nil {
		return result
	}
	for _, m := range mf.GetMetric() {
		if m.GetGauge().GetValue() != value {
			continue
		}
		if lv := labelValueOf(m, labelName); lv != "" {
			result = append(result, lv)
		}
	}
	return result
}

// countGaugeAtValue counts how many label combinations of a gauge currently
// equal the given value.
func countGaugeAtValue(mf *dto.MetricFamily, value float64) int {
	if mf == nil {
		return 0
	}
	count := 0
	for _, m := range mf.GetMetric() {
		if m.GetGauge().GetValue() == value {
			count++
		}
	}
	return count
}

// labelValueOf returns the value of a named label on a metric, or "".
func labelValueOf(m *dto.Metric, labelName string) string {
	for _, lp := range m.GetLabel() {
		if lp.GetName() == labelName {
			return lp.GetValue()
		}
	}
	return ""
}

// histogramAvgMs computes the mean observed value (sum/count) across every
// label combination of a histogram, in milliseconds. Metrics are recorded in
// seconds (see services/metrics/metrics.go), hence the *1000.
func histogramAvgMs(mf *dto.MetricFamily) float64 {
	if mf == nil {
		return 0
	}
	var sum float64
	var count uint64
	for _, m := range mf.GetMetric() {
		h := m.GetHistogram()
		if h == nil {
			continue
		}
		sum += h.GetSampleSum()
		count += h.GetSampleCount()
	}
	if count == 0 {
		return 0
	}
	return (sum / float64(count)) * 1000
}
