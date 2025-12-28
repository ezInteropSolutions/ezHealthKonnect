package services

import (
	"context"
	"database/sql"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors/enrichment"
	"fmt"
	"log"
	"sync"
	"time"
)

// ========================================
// INTERFACES (OOP: Contract-based design)
// ========================================

// ConnectionWarmerStrategy defines the interface for different connection warming strategies
type ConnectionWarmerStrategy interface {
	// CanWarm checks if this strategy can warm connections for the given step type
	CanWarm(stepType string) bool

	// WarmConnection warms a connection for a specific step configuration
	WarmConnection(ctx context.Context, stepName string, stepConfig map[string]interface{}) error

	// GetName returns the name of this warming strategy
	GetName() string
}

// PipelineParser extracts warmable configurations from pipeline structure
type PipelineParser interface {
	// ExtractConfigs extracts all warmable step configurations from pipeline
	ExtractConfigs(pipeline map[string]interface{}) []StepConfig
}

// ========================================
// VALUE OBJECTS (OOP: Immutable data)
// ========================================

// StepConfig represents a single step configuration to be warmed
type StepConfig struct {
	StepName   string
	StepType   string
	Config     map[string]interface{}
}

// WarmingResult represents the result of warming a single connection
type WarmingResult struct {
	StepName   string
	StepType   string
	Success    bool
	Duration   time.Duration
	Error      error
}

// ========================================
// MAIN SERVICE (OOP: Orchestrator)
// ========================================

// ConnectionWarmer orchestrates connection pre-warming using strategy pattern
type ConnectionWarmer struct {
	strategies        []ConnectionWarmerStrategy
	parser            PipelineParser
	warmingInProgress map[string]bool
	warmingMutex      sync.RWMutex
}

// NewConnectionWarmer creates a new connection warmer with default strategies
func NewConnectionWarmer(db *sql.DB) *ConnectionWarmer {
	return &ConnectionWarmer{
		strategies: []ConnectionWarmerStrategy{
			NewDatabaseWarmerStrategy(db),
			// Future: NewAPIWarmerStrategy(), NewRedisWarmerStrategy(), etc.
		},
		parser:            NewDefaultPipelineParser(),
		warmingInProgress: make(map[string]bool),
	}
}

// WarmInterfaceConnections pre-warms all connections for an interface
func (cw *ConnectionWarmer) WarmInterfaceConnections(interfaceID int, pipeline map[string]interface{}) error {
	cacheKey := fmt.Sprintf("interface_%d", interfaceID)

	// Check if already warming (thread-safe)
	if !cw.acquireWarmingLock(cacheKey) {
		log.Printf("⏳ Connection warming already in progress for interface %d", interfaceID)
		return nil
	}
	defer cw.releaseWarmingLock(cacheKey)

	log.Printf("🔥 Pre-warming connections for interface %d", interfaceID)

	// Extract configurations using parser (SRP: parsing responsibility)
	configs := cw.parser.ExtractConfigs(pipeline)

	if len(configs) == 0 {
		log.Printf("   ℹ️  No warmable steps found, skipping connection warming")
		return nil
	}

	log.Printf("   📋 Found %d warmable step(s)", len(configs))

	// Warm connections in parallel using strategies (OCP: open for extension)
	results := cw.warmConnectionsParallel(configs)

	// Log summary
	cw.logWarmingSummary(results)

	return nil
}

// ========================================
// PRIVATE METHODS (OOP: Encapsulation)
// ========================================

// acquireWarmingLock tries to acquire a warming lock for the given key
func (cw *ConnectionWarmer) acquireWarmingLock(key string) bool {
	cw.warmingMutex.RLock()
	if cw.warmingInProgress[key] {
		cw.warmingMutex.RUnlock()
		return false
	}
	cw.warmingMutex.RUnlock()

	cw.warmingMutex.Lock()
	cw.warmingInProgress[key] = true
	cw.warmingMutex.Unlock()
	return true
}

// releaseWarmingLock releases the warming lock for the given key
func (cw *ConnectionWarmer) releaseWarmingLock(key string) {
	cw.warmingMutex.Lock()
	delete(cw.warmingInProgress, key)
	cw.warmingMutex.Unlock()
}

// warmConnectionsParallel warms multiple connections in parallel
func (cw *ConnectionWarmer) warmConnectionsParallel(configs []StepConfig) []WarmingResult {
	var wg sync.WaitGroup
	results := make([]WarmingResult, len(configs))

	for i, config := range configs {
		wg.Add(1)
		go func(idx int, cfg StepConfig) {
			defer wg.Done()
			results[idx] = cw.warmSingleConnection(cfg)
		}(i, config)
	}

	wg.Wait()
	return results
}

// warmSingleConnection warms a single connection using appropriate strategy
func (cw *ConnectionWarmer) warmSingleConnection(config StepConfig) WarmingResult {
	start := time.Now()

	// Find appropriate strategy (Strategy Pattern)
	strategy := cw.findStrategy(config.StepType)
	if strategy == nil {
		return WarmingResult{
			StepName: config.StepName,
			StepType: config.StepType,
			Success:  false,
			Duration: time.Since(start),
			Error:    fmt.Errorf("no warming strategy found for step type: %s", config.StepType),
		}
	}

	log.Printf("   🔌 Warming connection for %s using %s strategy...", config.StepName, strategy.GetName())

	// Warm with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := strategy.WarmConnection(ctx, config.StepName, config.Config)

	result := WarmingResult{
		StepName: config.StepName,
		StepType: config.StepType,
		Success:  err == nil,
		Duration: time.Since(start),
		Error:    err,
	}

	if result.Success {
		log.Printf("   ✅ %s connection warmed in %v (cached for reuse)", config.StepName, result.Duration)
	} else {
		log.Printf("   ⚠️  Failed to warm %s: %v (will retry on first message)", config.StepName, err)
	}

	return result
}

// findStrategy finds the appropriate warming strategy for a step type
func (cw *ConnectionWarmer) findStrategy(stepType string) ConnectionWarmerStrategy {
	for _, strategy := range cw.strategies {
		if strategy.CanWarm(stepType) {
			return strategy
		}
	}
	return nil
}

// logWarmingSummary logs the summary of warming results
func (cw *ConnectionWarmer) logWarmingSummary(results []WarmingResult) {
	successCount := 0
	failureCount := 0

	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	log.Printf("✅ Connection warming completed: %d successful, %d failed", successCount, failureCount)
}

// ========================================
// DATABASE WARMING STRATEGY (OOP: Strategy Pattern)
// ========================================

// DatabaseWarmerStrategy warms database connections
type DatabaseWarmerStrategy struct {
	db *sql.DB
}

// NewDatabaseWarmerStrategy creates a new database warming strategy
func NewDatabaseWarmerStrategy(db *sql.DB) *DatabaseWarmerStrategy {
	return &DatabaseWarmerStrategy{db: db}
}

// CanWarm checks if this strategy can warm the given step type
func (s *DatabaseWarmerStrategy) CanWarm(stepType string) bool {
	return stepType == "pre.enrichment.database" ||
	       stepType == "core.enrichment.database" ||
	       stepType == "post.enrichment.database"
}

// WarmConnection warms a database connection
func (s *DatabaseWarmerStrategy) WarmConnection(ctx context.Context, stepName string, stepConfig map[string]interface{}) error {
	// Parse config into DatabaseEnrichmentConfigV2
	config := s.parseConfig(stepConfig)
	if config == nil {
		return fmt.Errorf("failed to parse database config")
	}

	// Create transformation step
	step := &models.TransformationStep{
		StepName: stepName,
		StepType: "pre.enrichment.database",
		Config:   stepConfig,
	}

	// Create executor (DIP: depend on abstraction, not concrete class)
	executor := enrichment.NewDatabaseEnrichmentExecutor(s.db)

	// Create dummy input for connection test
	dummyInput := map[string]interface{}{
		"_test": "connection_warming",
	}

	// Execute to establish connection (will be cached in global pool)
	_, err := executor.Execute(ctx, step, dummyInput)
	return err
}

// GetName returns the strategy name
func (s *DatabaseWarmerStrategy) GetName() string {
	return "Database"
}

// parseConfig parses step config into DatabaseEnrichmentConfigV2
func (s *DatabaseWarmerStrategy) parseConfig(stepConfig map[string]interface{}) *models.DatabaseEnrichmentConfigV2 {
	config := &models.DatabaseEnrichmentConfigV2{}

	// Type-safe parsing with defaults
	if dbType, ok := stepConfig["databaseType"].(string); ok {
		config.DatabaseType = dbType
	}
	if dbHost, ok := stepConfig["dbHost"].(string); ok {
		config.DBHost = dbHost
	}
	if dbPort, ok := stepConfig["dbPort"].(float64); ok {
		config.DBPort = int(dbPort)
	}
	if dbName, ok := stepConfig["dbName"].(string); ok {
		config.DBName = dbName
	}
	if dbUser, ok := stepConfig["dbUser"].(string); ok {
		config.DBUser = dbUser
	}
	if dbPassword, ok := stepConfig["dbPassword"].(string); ok {
		config.DBPassword = dbPassword
	}
	if connString, ok := stepConfig["connectionString"].(string); ok {
		config.ConnectionString = connString
	}
	if connName, ok := stepConfig["connectionName"].(string); ok {
		config.ConnectionName = connName
	}

	// Set warming-specific defaults
	config.TimeoutMs = 3000
	config.FailOnError = false

	// Validate essential fields
	if config.DatabaseType == "" {
		return nil
	}

	return config
}

// ========================================
// DEFAULT PIPELINE PARSER (OOP: SRP)
// ========================================

// DefaultPipelineParser extracts warmable configurations from standard pipeline format
type DefaultPipelineParser struct{}

// NewDefaultPipelineParser creates a new default pipeline parser
func NewDefaultPipelineParser() *DefaultPipelineParser {
	return &DefaultPipelineParser{}
}

// ExtractConfigs extracts all warmable step configurations from pipeline
func (p *DefaultPipelineParser) ExtractConfigs(pipeline map[string]interface{}) []StepConfig {
	configs := make([]StepConfig, 0)

	// Navigate: pipeline -> layers -> [pre/core/post] -> execution_groups -> steps
	layers, ok := pipeline["layers"].(map[string]interface{})
	if !ok {
		return configs
	}

	// Check all layers
	for _, layerName := range []string{"pre", "core", "post"} {
		configs = append(configs, p.extractFromLayer(layers, layerName)...)
	}

	return configs
}

// extractFromLayer extracts configs from a specific layer
func (p *DefaultPipelineParser) extractFromLayer(layers map[string]interface{}, layerName string) []StepConfig {
	configs := make([]StepConfig, 0)

	layerData, exists := layers[layerName]
	if !exists {
		return configs
	}

	layer, ok := layerData.(map[string]interface{})
	if !ok {
		return configs
	}

	// Get execution groups
	groups, ok := layer["execution_groups"].([]interface{})
	if !ok {
		return configs
	}

	// Extract from each group
	for _, groupData := range groups {
		group, ok := groupData.(map[string]interface{})
		if !ok {
			continue
		}

		steps, ok := group["steps"].([]interface{})
		if !ok {
			continue
		}

		// Extract each step
		for _, stepData := range steps {
			stepMap, ok := stepData.(map[string]interface{})
			if !ok {
				continue
			}

			config := p.parseStepConfig(stepMap)
			if config != nil {
				configs = append(configs, *config)
			}
		}
	}

	return configs
}

// parseStepConfig parses a single step configuration
func (p *DefaultPipelineParser) parseStepConfig(stepMap map[string]interface{}) *StepConfig {
	stepType, _ := stepMap["step_type"].(string)
	stepName, _ := stepMap["step_name"].(string)
	stepConfig, ok := stepMap["config"].(map[string]interface{})

	if !ok || stepType == "" {
		return nil
	}

	// Only parse enrichment steps (warmable types)
	if !p.isWarmableStep(stepType) {
		return nil
	}

	return &StepConfig{
		StepName: stepName,
		StepType: stepType,
		Config:   stepConfig,
	}
}

// isWarmableStep checks if a step type requires connection warming
func (p *DefaultPipelineParser) isWarmableStep(stepType string) bool {
	warmableTypes := []string{
		"pre.enrichment.database",
		"core.enrichment.database",
		"post.enrichment.database",
		// Future: "pre.enrichment.api", "pre.enrichment.redis", etc.
	}

	for _, warmType := range warmableTypes {
		if stepType == warmType {
			return true
		}
	}
	return false
}
