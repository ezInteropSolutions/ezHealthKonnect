// health_monitor.go
// Startup health checks and graceful degradation for the Interface-Centric Configuration Engine

package services

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/bson"
)

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// ComponentHealth represents the health of a single component
type ComponentHealth struct {
	Name           string                 `json:"name"`
	Status         HealthStatus           `json:"status"`
	LastChecked    time.Time              `json:"last_checked"`
	ResponseTime   time.Duration          `json:"response_time"`
	ErrorMessage   string                 `json:"error_message,omitempty"`
	Details        map[string]interface{} `json:"details,omitempty"`
	Dependencies   []string               `json:"dependencies,omitempty"`
}

// SystemHealth represents the overall system health
type SystemHealth struct {
	OverallStatus HealthStatus                `json:"overall_status"`
	Components    map[string]*ComponentHealth `json:"components"`
	StartupTime   time.Time                   `json:"startup_time"`
	Uptime        time.Duration               `json:"uptime"`
	Version       string                      `json:"version"`
	Environment   string                      `json:"environment"`
	Degradations  []string                    `json:"degradations,omitempty"`
}

// HealthMonitor monitors system health and handles graceful degradation
type HealthMonitor struct {
	components     map[string]HealthChecker
	health         *SystemHealth
	checkInterval  time.Duration
	startupTime    time.Time
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	isRunning      bool

	// Configuration for graceful degradation
	degradationRules map[string]DegradationRule
	fallbackConfigs  map[string]interface{}
}

// HealthChecker interface for components that can be health checked
type HealthChecker interface {
	CheckHealth(ctx context.Context) *ComponentHealth
	GetDependencies() []string
	CanDegrade() bool
	GetFallbackConfig() interface{}
}

// DegradationRule defines how a component should degrade when unhealthy
type DegradationRule struct {
	ComponentName    string        `json:"component_name"`
	MaxFailures      int          `json:"max_failures"`
	FailureWindow    time.Duration `json:"failure_window"`
	DegradationMode  string       `json:"degradation_mode"` // disable, fallback, readonly
	FallbackEnabled  bool         `json:"fallback_enabled"`
	AlertThreshold   int          `json:"alert_threshold"`
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(checkInterval time.Duration) *HealthMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &HealthMonitor{
		components:       make(map[string]HealthChecker),
		health:          &SystemHealth{
			Components:   make(map[string]*ComponentHealth),
			StartupTime:  time.Now(),
			Version:      "1.0.0",
			Environment:  "production",
		},
		checkInterval:    checkInterval,
		startupTime:      time.Now(),
		ctx:              ctx,
		cancel:           cancel,
		degradationRules: make(map[string]DegradationRule),
		fallbackConfigs:  make(map[string]interface{}),
	}
}

// RegisterComponent registers a component for health monitoring
func (hm *HealthMonitor) RegisterComponent(name string, checker HealthChecker) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.components[name] = checker
	hm.health.Components[name] = &ComponentHealth{
		Name:         name,
		Status:       HealthStatusUnknown,
		Dependencies: checker.GetDependencies(),
	}

	log.Printf("✅ Registered health checker for component: %s", name)
}

// RegisterDegradationRule registers a degradation rule for a component
func (hm *HealthMonitor) RegisterDegradationRule(rule DegradationRule) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.degradationRules[rule.ComponentName] = rule
	log.Printf("✅ Registered degradation rule for component: %s", rule.ComponentName)
}

// Start begins health monitoring
func (hm *HealthMonitor) Start() error {
	hm.mu.Lock()
	if hm.isRunning {
		hm.mu.Unlock()
		return fmt.Errorf("health monitor is already running")
	}
	hm.isRunning = true
	hm.mu.Unlock()

	// Perform initial startup checks
	if err := hm.performStartupChecks(); err != nil {
		return fmt.Errorf("startup checks failed: %w", err)
	}

	// Start periodic health checks
	go hm.monitoringLoop()

	log.Printf("✅ Health monitor started with %d components", len(hm.components))
	return nil
}

// Stop stops health monitoring
func (hm *HealthMonitor) Stop() error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if !hm.isRunning {
		return fmt.Errorf("health monitor is not running")
	}

	hm.cancel()
	hm.isRunning = false

	log.Printf("✅ Health monitor stopped")
	return nil
}

// performStartupChecks performs critical health checks during startup
func (hm *HealthMonitor) performStartupChecks() error {
	log.Printf("🔍 Performing startup health checks...")

	ctx, cancel := context.WithTimeout(hm.ctx, 30*time.Second)
	defer cancel()

	criticalComponents := []string{"mongodb", "postgresql"} // Define critical components
	var failures []string

	for _, componentName := range criticalComponents {
		hm.mu.RLock()
		checker, exists := hm.components[componentName]
		hm.mu.RUnlock()

		if !exists {
			log.Printf("⚠️ Critical component %s not registered for health checks", componentName)
			continue
		}

		startTime := time.Now()
		health := checker.CheckHealth(ctx)
		health.ResponseTime = time.Since(startTime)
		health.LastChecked = time.Now()

		hm.mu.Lock()
		hm.health.Components[componentName] = health
		hm.mu.Unlock()

		if health.Status == HealthStatusUnhealthy {
			failures = append(failures, fmt.Sprintf("%s: %s", componentName, health.ErrorMessage))
			log.Printf("❌ Critical component %s failed startup check: %s", componentName, health.ErrorMessage)
		} else {
			log.Printf("✅ Critical component %s passed startup check", componentName)
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("critical startup checks failed: %v", failures)
	}

	log.Printf("✅ All startup health checks passed")
	return nil
}

// monitoringLoop runs the periodic health checking
func (hm *HealthMonitor) monitoringLoop() {
	ticker := time.NewTicker(hm.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-hm.ctx.Done():
			return
		case <-ticker.C:
			hm.checkAllComponents()
			hm.updateOverallHealth()
			hm.evaluateDegradation()
		}
	}
}

// checkAllComponents checks the health of all registered components
func (hm *HealthMonitor) checkAllComponents() {
	hm.mu.RLock()
	components := make(map[string]HealthChecker)
	for name, checker := range hm.components {
		components[name] = checker
	}
	hm.mu.RUnlock()

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(hm.ctx, 30*time.Second)
	defer cancel()

	for name, checker := range components {
		wg.Add(1)
		go func(componentName string, healthChecker HealthChecker) {
			defer wg.Done()

			startTime := time.Now()
			health := healthChecker.CheckHealth(ctx)
			health.ResponseTime = time.Since(startTime)
			health.LastChecked = time.Now()

			hm.mu.Lock()
			hm.health.Components[componentName] = health
			hm.mu.Unlock()

			if health.Status != HealthStatusHealthy {
				log.Printf("⚠️ Component %s health check: %s - %s",
					componentName, health.Status, health.ErrorMessage)
			}
		}(name, checker)
	}

	wg.Wait()
}

// updateOverallHealth calculates the overall system health
func (hm *HealthMonitor) updateOverallHealth() {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.health.Uptime = time.Since(hm.startupTime)

	healthyCount := 0
	degradedCount := 0
	unhealthyCount := 0
	totalCount := len(hm.health.Components)

	for _, component := range hm.health.Components {
		switch component.Status {
		case HealthStatusHealthy:
			healthyCount++
		case HealthStatusDegraded:
			degradedCount++
		case HealthStatusUnhealthy:
			unhealthyCount++
		}
	}

	// Determine overall status
	if unhealthyCount > 0 {
		hm.health.OverallStatus = HealthStatusUnhealthy
	} else if degradedCount > 0 {
		hm.health.OverallStatus = HealthStatusDegraded
	} else if healthyCount == totalCount {
		hm.health.OverallStatus = HealthStatusHealthy
	} else {
		hm.health.OverallStatus = HealthStatusUnknown
	}
}

// evaluateDegradation evaluates if any components should be degraded
func (hm *HealthMonitor) evaluateDegradation() {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	var newDegradations []string

	for componentName, rule := range hm.degradationRules {
		component, exists := hm.health.Components[componentName]
		if !exists {
			continue
		}

		if component.Status == HealthStatusUnhealthy {
			// Check if we should trigger degradation
			shouldDegrade := hm.shouldTriggerDegradation(componentName, rule)
			if shouldDegrade {
				degradationMsg := hm.applyDegradation(componentName, rule)
				newDegradations = append(newDegradations, degradationMsg)
			}
		}
	}

	hm.health.Degradations = newDegradations
}

// shouldTriggerDegradation determines if degradation should be triggered
func (hm *HealthMonitor) shouldTriggerDegradation(componentName string, rule DegradationRule) bool {
	// Simple implementation - in production, you'd track failure history
	component := hm.health.Components[componentName]
	return component.Status == HealthStatusUnhealthy && rule.FallbackEnabled
}

// applyDegradation applies degradation rules to a component
func (hm *HealthMonitor) applyDegradation(componentName string, rule DegradationRule) string {
	message := fmt.Sprintf("Component %s degraded to %s mode", componentName, rule.DegradationMode)

	switch rule.DegradationMode {
	case "disable":
		log.Printf("🚨 Disabling component: %s", componentName)
	case "fallback":
		log.Printf("🔄 Switching component %s to fallback mode", componentName)
		hm.activateFallback(componentName)
	case "readonly":
		log.Printf("📖 Switching component %s to read-only mode", componentName)
	}

	return message
}

// activateFallback activates fallback configuration for a component
func (hm *HealthMonitor) activateFallback(componentName string) {
	if fallbackConfig, exists := hm.fallbackConfigs[componentName]; exists {
		log.Printf("🔄 Activating fallback configuration for %s: %+v", componentName, fallbackConfig)
		// In a real implementation, you'd apply the fallback configuration
	}
}

// GetHealth returns the current system health
func (hm *HealthMonitor) GetHealth() *SystemHealth {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	// Create a copy to avoid race conditions
	health := &SystemHealth{
		OverallStatus: hm.health.OverallStatus,
		Components:    make(map[string]*ComponentHealth),
		StartupTime:   hm.health.StartupTime,
		Uptime:        hm.health.Uptime,
		Version:       hm.health.Version,
		Environment:   hm.health.Environment,
		Degradations:  make([]string, len(hm.health.Degradations)),
	}

	for name, component := range hm.health.Components {
		health.Components[name] = &ComponentHealth{
			Name:         component.Name,
			Status:       component.Status,
			LastChecked:  component.LastChecked,
			ResponseTime: component.ResponseTime,
			ErrorMessage: component.ErrorMessage,
			Details:      component.Details,
			Dependencies: component.Dependencies,
		}
	}

	copy(health.Degradations, hm.health.Degradations)

	return health
}

// IsHealthy returns true if the system is healthy or degraded (operational)
func (hm *HealthMonitor) IsHealthy() bool {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	return hm.health.OverallStatus == HealthStatusHealthy ||
		   hm.health.OverallStatus == HealthStatusDegraded
}

// Component-specific health checkers

// MongoDBHealthChecker checks MongoDB health
type MongoDBHealthChecker struct {
	client   *mongo.Client
	database string
	timeout  time.Duration
}

// NewMongoDBHealthChecker creates a new MongoDB health checker
func NewMongoDBHealthChecker(client *mongo.Client, database string, timeout time.Duration) *MongoDBHealthChecker {
	return &MongoDBHealthChecker{
		client:   client,
		database: database,
		timeout:  timeout,
	}
}

// CheckHealth implements HealthChecker for MongoDB
func (m *MongoDBHealthChecker) CheckHealth(ctx context.Context) *ComponentHealth {
	health := &ComponentHealth{
		Name:    "mongodb",
		Details: make(map[string]interface{}),
	}

	if m.client == nil {
		health.Status = HealthStatusUnhealthy
		health.ErrorMessage = "MongoDB client not initialized"
		return health
	}

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	// Test connection with ping
	err := m.client.Ping(timeoutCtx, nil)
	if err != nil {
		health.Status = HealthStatusUnhealthy
		health.ErrorMessage = fmt.Sprintf("MongoDB ping failed: %v", err)
		return health
	}

	// Test database access
	db := m.client.Database(m.database)
	collections, err := db.ListCollectionNames(timeoutCtx, bson.D{})
	if err != nil {
		health.Status = HealthStatusDegraded
		health.ErrorMessage = fmt.Sprintf("Database access failed: %v", err)
		health.Details["ping"] = "ok"
		health.Details["database_access"] = "failed"
		return health
	}

	// Test basic operations
	testCollection := db.Collection("health_check")
	_, err = testCollection.CountDocuments(timeoutCtx, bson.D{})
	if err != nil {
		health.Status = HealthStatusDegraded
		health.ErrorMessage = fmt.Sprintf("Collection operations failed: %v", err)
		health.Details["ping"] = "ok"
		health.Details["database_access"] = "ok"
		health.Details["operations"] = "failed"
		return health
	}

	health.Status = HealthStatusHealthy
	health.Details["ping"] = "ok"
	health.Details["database_access"] = "ok"
	health.Details["operations"] = "ok"
	health.Details["collections_count"] = len(collections)

	return health
}

// GetDependencies returns MongoDB dependencies
func (m *MongoDBHealthChecker) GetDependencies() []string {
	return []string{"network", "mongodb_server"}
}

// CanDegrade returns true if MongoDB can operate in degraded mode
func (m *MongoDBHealthChecker) CanDegrade() bool {
	return true
}

// GetFallbackConfig returns fallback configuration for MongoDB
func (m *MongoDBHealthChecker) GetFallbackConfig() interface{} {
	return map[string]interface{}{
		"mode":              "local_cache",
		"cache_duration":    "5m",
		"readonly_mode":     true,
		"disable_writes":    true,
	}
}

// PostgreSQLHealthChecker checks PostgreSQL health
type PostgreSQLHealthChecker struct {
	db      *sql.DB
	timeout time.Duration
}

// NewPostgreSQLHealthChecker creates a new PostgreSQL health checker
func NewPostgreSQLHealthChecker(db *sql.DB, timeout time.Duration) *PostgreSQLHealthChecker {
	return &PostgreSQLHealthChecker{
		db:      db,
		timeout: timeout,
	}
}

// CheckHealth implements HealthChecker for PostgreSQL
func (p *PostgreSQLHealthChecker) CheckHealth(ctx context.Context) *ComponentHealth {
	health := &ComponentHealth{
		Name:    "postgresql",
		Details: make(map[string]interface{}),
	}

	if p.db == nil {
		health.Status = HealthStatusUnhealthy
		health.ErrorMessage = "PostgreSQL connection not initialized"
		return health
	}

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	// Test connection with ping
	err := p.db.PingContext(timeoutCtx)
	if err != nil {
		health.Status = HealthStatusUnhealthy
		health.ErrorMessage = fmt.Sprintf("PostgreSQL ping failed: %v", err)
		return health
	}

	// Test basic query
	var version string
	err = p.db.QueryRowContext(timeoutCtx, "SELECT version()").Scan(&version)
	if err != nil {
		health.Status = HealthStatusDegraded
		health.ErrorMessage = fmt.Sprintf("Query execution failed: %v", err)
		health.Details["ping"] = "ok"
		health.Details["query"] = "failed"
		return health
	}

	// Get connection stats
	stats := p.db.Stats()

	health.Status = HealthStatusHealthy
	health.Details["ping"] = "ok"
	health.Details["query"] = "ok"
	health.Details["version"] = version
	health.Details["open_connections"] = stats.OpenConnections
	health.Details["in_use"] = stats.InUse
	health.Details["idle"] = stats.Idle

	// Check if connection pool is under stress
	if stats.OpenConnections > 0 && float64(stats.InUse)/float64(stats.OpenConnections) > 0.8 {
		health.Status = HealthStatusDegraded
		health.ErrorMessage = "Connection pool under stress"
	}

	return health
}

// GetDependencies returns PostgreSQL dependencies
func (p *PostgreSQLHealthChecker) GetDependencies() []string {
	return []string{"network", "postgresql_server"}
}

// CanDegrade returns true if PostgreSQL can operate in degraded mode
func (p *PostgreSQLHealthChecker) CanDegrade() bool {
	return true
}

// GetFallbackConfig returns fallback configuration for PostgreSQL
func (p *PostgreSQLHealthChecker) GetFallbackConfig() interface{} {
	return map[string]interface{}{
		"mode":                 "readonly",
		"disable_writes":       true,
		"reduce_pool_size":     true,
		"increase_timeout":     true,
	}
}

// InterfaceEngineHealthChecker checks Interface Engine health
// COMMENTED OUT - InterfaceEngine moved to processing package
/*
type InterfaceEngineHealthChecker struct {
	engine *InterfaceEngine
}
*/

// COMMENTED OUT - InterfaceEngine moved to processing package
/*
// NewInterfaceEngineHealthChecker creates a new Interface Engine health checker
func NewInterfaceEngineHealthChecker(engine *InterfaceEngine) *InterfaceEngineHealthChecker {
	return &InterfaceEngineHealthChecker{
		engine: engine,
	}
}

// CheckHealth implements HealthChecker for Interface Engine
func (ie *InterfaceEngineHealthChecker) CheckHealth(ctx context.Context) *ComponentHealth {
	health := &ComponentHealth{
		Name:    "interface_engine",
		Details: make(map[string]interface{}),
	}

	if ie.engine == nil {
		health.Status = HealthStatusUnhealthy
		health.ErrorMessage = "Interface Engine not initialized"
		return health
	}

	// Check active messages
	activeMessages := ie.engine.GetActiveMessages()
	activeCount := len(activeMessages)

	health.Details["active_messages"] = activeCount
	health.Details["processors_registered"] = len(ie.engine.inputProcessors)

	// Health status based on active message load
	if activeCount > 1000 {
		health.Status = HealthStatusDegraded
		health.ErrorMessage = "High message processing load"
	} else if activeCount > 100 {
		health.Status = HealthStatusDegraded
		health.ErrorMessage = "Moderate message processing load"
	} else {
		health.Status = HealthStatusHealthy
	}

	return health
}

// GetDependencies returns Interface Engine dependencies
func (ie *InterfaceEngineHealthChecker) GetDependencies() []string {
	return []string{"mongodb", "postgresql"}
}

// CanDegrade returns true if Interface Engine can operate in degraded mode
func (ie *InterfaceEngineHealthChecker) CanDegrade() bool {
	return true
}
*/

// GetFallbackConfig returns fallback configuration for Interface Engine
/*
func (ie *InterfaceEngineHealthChecker) GetFallbackConfig() interface{} {
	return map[string]interface{}{
		"mode":                    "essential_only",
		"disable_business_logic":  true,
		"reduce_validation":       true,
		"simple_transformation":   true,
	}
}
*/