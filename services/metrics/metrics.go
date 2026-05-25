// services/metrics/metrics.go
// Prometheus metrics definitions for ezHealthKonnect.
//
// All metrics are registered once via sync.Once and exposed on the default
// Prometheus registry. The /metrics HTTP endpoint is wired in main.go via
// promhttp.Handler().
//
// Usage (in hot paths):
//
//	metrics.MessagesReceived.WithLabelValues(interfaceID, sourceType).Inc()
//	metrics.ProcessingDuration.WithLabelValues(interfaceID).Observe(elapsed.Seconds())

package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var once sync.Once

// Counters — monotonically increasing event counts.
var (
	// MessagesReceived counts inbound messages per interface and source type.
	// Labels: interface_id, source_type (tcp_mllp, http, sftp, …)
	MessagesReceived *prometheus.CounterVec

	// MessagesProcessed counts pipeline completions per interface and final status.
	// Labels: interface_id, status (delivered, failed, skipped)
	MessagesProcessed *prometheus.CounterVec

	// StepExecutions counts individual step executions per step type and status.
	// Labels: step_type, status (success, error, retried)
	StepExecutions *prometheus.CounterVec

	// DLQEnqueued counts messages written to the dead letter queue.
	// Labels: interface_id, error_type
	DLQEnqueued *prometheus.CounterVec

	// RecoveryMessagesRequeued counts messages re-enqueued by startup recovery.
	RecoveryMessagesRequeued prometheus.Counter
)

// Histograms — latency distributions.
var (
	// ProcessingDuration tracks end-to-end pipeline execution time per interface.
	// Labels: interface_id
	ProcessingDuration *prometheus.HistogramVec

	// StepDuration tracks per-step execution time.
	// Labels: step_type
	StepDuration *prometheus.HistogramVec

	// QueueWaitDuration tracks how long a message sat in message_processing_queue
	// before being picked up.
	// Labels: interface_id
	QueueWaitDuration *prometheus.HistogramVec
)

// Gauges — current state snapshots.
var (
	// ActiveConnectors tracks the number of running inbound connectors by type.
	// Labels: connector_type
	ActiveConnectors *prometheus.GaugeVec

	// WorkerQueueDepth tracks the backpressure worker queue depth per interface.
	// Labels: interface_id
	WorkerQueueDepth *prometheus.GaugeVec

	// DLQDepth tracks the number of unresolved messages in the dead letter queue.
	DLQDepth prometheus.Gauge

	// CircuitBreakerState represents the current circuit breaker state as a number.
	// 0 = closed (healthy), 1 = half-open, 2 = open (tripped).
	// Labels: executor_id
	CircuitBreakerState *prometheus.GaugeVec

	// ActiveInterfaces tracks how many interfaces are currently activated.
	ActiveInterfaces prometheus.Gauge
)

// Init registers all metrics with the default Prometheus registry.
// Safe to call multiple times — registration happens exactly once.
func Init() {
	once.Do(func() {
		MessagesReceived = promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "ezhk",
			Subsystem: "messages",
			Name:      "received_total",
			Help:      "Total inbound messages received, by interface and source connector type.",
		}, []string{"interface_id", "source_type"})

		MessagesProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "ezhk",
			Subsystem: "messages",
			Name:      "processed_total",
			Help:      "Total messages that completed pipeline execution, by interface and final status.",
		}, []string{"interface_id", "status"})

		StepExecutions = promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "ezhk",
			Subsystem: "pipeline",
			Name:      "step_executions_total",
			Help:      "Total pipeline step executions, by step type and outcome.",
		}, []string{"step_type", "status"})

		DLQEnqueued = promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "ezhk",
			Subsystem: "dlq",
			Name:      "enqueued_total",
			Help:      "Total messages written to the dead letter queue, by interface and error type.",
		}, []string{"interface_id", "error_type"})

		RecoveryMessagesRequeued = promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "ezhk",
			Subsystem: "recovery",
			Name:      "messages_requeued_total",
			Help:      "Total messages re-enqueued by the startup recovery scan.",
		})

		// Histograms use the standard Prometheus recommended bucket set.
		// Add finer-grained buckets for the healthcare SLA range (< 500 ms).
		slaLatencyBuckets := []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}

		ProcessingDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "ezhk",
			Subsystem: "messages",
			Name:      "processing_duration_seconds",
			Help:      "End-to-end pipeline execution time in seconds, by interface.",
			Buckets:   slaLatencyBuckets,
		}, []string{"interface_id"})

		StepDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "ezhk",
			Subsystem: "pipeline",
			Name:      "step_duration_seconds",
			Help:      "Per-step execution time in seconds, by step type.",
			Buckets:   slaLatencyBuckets,
		}, []string{"step_type"})

		QueueWaitDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "ezhk",
			Subsystem: "queue",
			Name:      "wait_duration_seconds",
			Help:      "Time a message spent in message_processing_queue before pickup, by interface.",
			Buckets:   []float64{0.1, 0.5, 1, 5, 30, 60, 300},
		}, []string{"interface_id"})

		ActiveConnectors = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "ezhk",
			Subsystem: "connectors",
			Name:      "active_total",
			Help:      "Number of active inbound connectors, by connector type.",
		}, []string{"connector_type"})

		WorkerQueueDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "ezhk",
			Subsystem: "backpressure",
			Name:      "queue_depth",
			Help:      "Current worker queue depth per interface.",
		}, []string{"interface_id"})

		DLQDepth = promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "ezhk",
			Subsystem: "dlq",
			Name:      "depth",
			Help:      "Current number of unresolved messages in the dead letter queue.",
		})

		CircuitBreakerState = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "ezhk",
			Subsystem: "circuit_breaker",
			Name:      "state",
			Help:      "Current circuit breaker state: 0=closed, 1=half-open, 2=open.",
		}, []string{"executor_id"})

		ActiveInterfaces = promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "ezhk",
			Subsystem: "engine",
			Name:      "active_interfaces",
			Help:      "Number of interfaces currently activated in the processing engine.",
		})
	})
}
