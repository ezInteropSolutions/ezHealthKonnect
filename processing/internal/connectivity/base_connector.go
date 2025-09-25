// internal/connectivity/base_connector.go
// Base connector implementation with common functionality

package connectivity

import (
	"context"
	"sync"
	"time"

	"ezhealthkonnect/processing/pkg"
)

// BaseConnector provides common functionality for all connectors
type BaseConnector struct {
	// Configuration
	Config   pkg.ConnectorConfig
	Type     string
	Protocol pkg.Protocol

	// State management
	isRunning   bool
	isConnected bool
	mutex       sync.RWMutex

	// Metrics
	metrics      *ConnectorMetrics
	metricsMutex sync.RWMutex

	// Lifecycle
	startTime time.Time
	stopTime  *time.Time
	ctx       context.Context
	cancel    context.CancelFunc

	// Error handling
	lastError  error
	errorCount int64
}

// ConnectorMetrics tracks connector performance
type ConnectorMetrics struct {
	MessagesProcessed   int64
	BytesTransferred    int64
	ErrorCount          int64
	LastActivity        time.Time
	AverageLatencyMs    float64
	MessagesPerSecond   float64
	ConnectionUptime    time.Duration
	ReconnectionCount   int64
	LastReconnection    *time.Time
}

// NewBaseConnector creates a new base connector
func NewBaseConnector(config pkg.ConnectorConfig, connectorType string) *BaseConnector {
	return &BaseConnector{
		Config:   config,
		Type:     connectorType,
		Protocol: config.Protocol,
		metrics:  &ConnectorMetrics{},
	}
}

// Start initializes the connector
func (bc *BaseConnector) Start(ctx context.Context) error {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()

	if bc.isRunning {
		return nil
	}

	bc.ctx, bc.cancel = context.WithCancel(ctx)
	bc.startTime = time.Now()
	bc.isRunning = true

	// Reset metrics
	bc.metricsMutex.Lock()
	bc.metrics = &ConnectorMetrics{
		LastActivity: time.Now(),
	}
	bc.metricsMutex.Unlock()

	return nil
}

// Stop shuts down the connector
func (bc *BaseConnector) Stop() error {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()

	if !bc.isRunning {
		return nil
	}

	if bc.cancel != nil {
		bc.cancel()
	}

	now := time.Now()
	bc.stopTime = &now
	bc.isRunning = false
	bc.isConnected = false

	return nil
}

// IsRunning returns whether the connector is running
func (bc *BaseConnector) IsRunning() bool {
	bc.mutex.RLock()
	defer bc.mutex.RUnlock()
	return bc.isRunning
}

// SetConnected updates the connection status
func (bc *BaseConnector) SetConnected(connected bool) {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()
	bc.isConnected = connected

	if connected {
		bc.metricsMutex.Lock()
		bc.metrics.ReconnectionCount++
		now := time.Now()
		bc.metrics.LastReconnection = &now
		bc.metricsMutex.Unlock()
	}
}

// IsConnected returns whether the connector is connected
func (bc *BaseConnector) IsConnected() bool {
	bc.mutex.RLock()
	defer bc.mutex.RUnlock()
	return bc.isConnected
}

// GetStatus returns the current connector status
func (bc *BaseConnector) GetStatus() pkg.ConnectorStatus {
	bc.mutex.RLock()
	defer bc.mutex.RUnlock()

	status := "disconnected"
	if bc.isRunning && bc.isConnected {
		status = "connected"
	} else if bc.isRunning {
		status = "connecting"
	}

	var lastErrorStr *string
	if bc.lastError != nil {
		errorStr := bc.lastError.Error()
		lastErrorStr = &errorStr
	}

	bc.metricsMutex.RLock()
	lastActivity := bc.metrics.LastActivity
	bc.metricsMutex.RUnlock()

	return pkg.ConnectorStatus{
		Type:         bc.Type,
		Protocol:     bc.Protocol,
		Status:       status,
		LastActivity: lastActivity,
		IsConnected:  bc.isConnected,
		ErrorCount:   bc.errorCount,
		LastError:    lastErrorStr,
	}
}

// GetMetrics returns current performance metrics
func (bc *BaseConnector) GetMetrics() pkg.ConnectorMetrics {
	bc.metricsMutex.RLock()
	defer bc.metricsMutex.RUnlock()

	uptime := time.Since(bc.startTime)
	if bc.stopTime != nil {
		uptime = bc.stopTime.Sub(bc.startTime)
	}

	return pkg.ConnectorMetrics{
		MessagesProcessed: bc.metrics.MessagesProcessed,
		MessagesPerSecond: bc.metrics.MessagesPerSecond,
		AverageLatency:    time.Duration(bc.metrics.AverageLatencyMs * float64(time.Millisecond)),
		ErrorRate:         float64(bc.metrics.ErrorCount) / float64(bc.metrics.MessagesProcessed+1),
		BytesTransferred:  bc.metrics.BytesTransferred,
		UptimeSeconds:     int64(uptime.Seconds()),
	}
}

// UpdateConfig updates the connector configuration
func (bc *BaseConnector) UpdateConfig(config pkg.ConnectorConfig) error {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()

	bc.Config = config
	bc.Protocol = config.Protocol

	return nil
}

// GetConfig returns the current configuration
func (bc *BaseConnector) GetConfig() pkg.ConnectorConfig {
	bc.mutex.RLock()
	defer bc.mutex.RUnlock()
	return bc.Config
}

// RecordMessage updates metrics for a processed message
func (bc *BaseConnector) RecordMessage(size int64, latencyMs int64) {
	bc.metricsMutex.Lock()
	defer bc.metricsMutex.Unlock()

	bc.metrics.MessagesProcessed++
	bc.metrics.BytesTransferred += size
	bc.metrics.LastActivity = time.Now()

	// Calculate rolling average latency
	if bc.metrics.MessagesProcessed == 1 {
		bc.metrics.AverageLatencyMs = float64(latencyMs)
	} else {
		bc.metrics.AverageLatencyMs = (bc.metrics.AverageLatencyMs*0.9) + (float64(latencyMs)*0.1)
	}

	// Calculate messages per second (over last minute)
	elapsed := time.Since(bc.startTime).Seconds()
	if elapsed > 0 {
		bc.metrics.MessagesPerSecond = float64(bc.metrics.MessagesProcessed) / elapsed
	}
}

// RecordError records an error occurrence
func (bc *BaseConnector) RecordError(err error) {
	bc.mutex.Lock()
	bc.lastError = err
	bc.errorCount++
	bc.mutex.Unlock()

	bc.metricsMutex.Lock()
	bc.metrics.ErrorCount++
	bc.metricsMutex.Unlock()
}

// Context returns the connector's context
func (bc *BaseConnector) Context() context.Context {
	bc.mutex.RLock()
	defer bc.mutex.RUnlock()
	return bc.ctx
}

// SupportsProtocol checks if the connector supports a specific protocol
func (bc *BaseConnector) SupportsProtocol(protocol pkg.Protocol) bool {
	return bc.Protocol == protocol
}

// WaitForContext waits for the context to be cancelled or a timeout
func (bc *BaseConnector) WaitForContext(timeout time.Duration) error {
	if bc.ctx == nil {
		return nil
	}

	select {
	case <-bc.ctx.Done():
		return bc.ctx.Err()
	case <-time.After(timeout):
		return nil
	}
}

// ValidateConfig validates the connector configuration
func (bc *BaseConnector) ValidateConfig() error {
	if bc.Config.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}

	if bc.Config.Timeout <= 0 {
		bc.Config.Timeout = 30 * time.Second
	}

	if bc.Config.Retries < 0 {
		bc.Config.Retries = 3
	}

	return nil
}

// GetConnectionString builds a connection string for the connector
func (bc *BaseConnector) GetConnectionString() string {
	switch bc.Protocol {
	case pkg.ProtocolTCP, pkg.ProtocolMLLP:
		return fmt.Sprintf("%s:%d", bc.Config.Endpoint, bc.Config.Port)
	case pkg.ProtocolHTTP, pkg.ProtocolHTTPS, pkg.ProtocolFHIR:
		scheme := string(bc.Protocol)
		if bc.Config.Path != "" {
			return fmt.Sprintf("%s://%s:%d%s", scheme, bc.Config.Endpoint, bc.Config.Port, bc.Config.Path)
		}
		return fmt.Sprintf("%s://%s:%d", scheme, bc.Config.Endpoint, bc.Config.Port)
	case pkg.ProtocolFile:
		return bc.Config.Endpoint
	case pkg.ProtocolDatabase:
		return bc.Config.Endpoint // Should be a full connection string
	default:
		return bc.Config.Endpoint
	}
}

// IsHealthy checks if the connector is in a healthy state
func (bc *BaseConnector) IsHealthy() bool {
	bc.mutex.RLock()
	defer bc.mutex.RUnlock()

	if !bc.isRunning || !bc.isConnected {
		return false
	}

	// Check error rate
	bc.metricsMutex.RLock()
	errorRate := float64(bc.metrics.ErrorCount) / float64(bc.metrics.MessagesProcessed+1)
	bc.metricsMutex.RUnlock()

	// Consider unhealthy if error rate > 10%
	return errorRate < 0.1
}

// GetUptimeSeconds returns connector uptime in seconds
func (bc *BaseConnector) GetUptimeSeconds() int64 {
	uptime := time.Since(bc.startTime)
	if bc.stopTime != nil {
		uptime = bc.stopTime.Sub(bc.startTime)
	}
	return int64(uptime.Seconds())
}