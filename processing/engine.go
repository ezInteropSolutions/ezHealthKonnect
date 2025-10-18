// processing/engine.go
// Simple Processing Engine for Configuration Controller (MVC + OOB)

package processing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"ezhealthkonnect/services"
)

// ProcessingEngine provides basic interface engine functionality
type ProcessingEngine struct {
	db                   *sql.DB
	activeInterfaces     map[string]*InterfaceStatus
	activeConnectors     map[string]InputConnector       // Track active TCP/HTTP connectors
	messageChan          map[string]chan Message        // Message channels per interface
	mutex               sync.RWMutex
	stats               *EngineStats
	running             bool
	parserService        *services.MessageParserService // JSON conversion service
	mongoService         *services.MongoDBMessageService // MongoDB storage
	// transformationService *services.TransformationService // TODO: Add when service is ready
}

// InterfaceStatus tracks the status of an interface
type InterfaceStatus struct {
	InterfaceID      string    `json:"interface_id"`
	Name             string    `json:"name"`
	Status           string    `json:"status"`
	MessagesProcessed int64     `json:"messages_processed"`
	LastActivity     time.Time `json:"last_activity"`
	Errors           int64     `json:"errors"`
}

// EngineStats provides engine statistics
type EngineStats struct {
	StartTime                time.Time `json:"start_time"`
	LastActivity             time.Time `json:"last_activity"`
	ActiveInterfaces         int       `json:"active_interfaces"`
	TotalMessagesProcessed   int64     `json:"total_messages_processed"`
	TotalErrors              int64     `json:"total_errors"`
	AverageProcessingTime    string    `json:"average_processing_time"`
}

// NewProcessingEngine creates a new processing engine
// OOB: Auto-initializes MongoDB and parser service if available
func NewProcessingEngine(db *sql.DB) *ProcessingEngine {
	engine := &ProcessingEngine{
		db:               db,
		activeInterfaces: make(map[string]*InterfaceStatus),
		activeConnectors: make(map[string]InputConnector),
		messageChan:      make(map[string]chan Message),
		stats: &EngineStats{
			StartTime:             time.Now(),
			LastActivity:          time.Now(),
			AverageProcessingTime: "0ms",
		},
		running: false,
	}

	// OOB: Auto-initialize parser service (includes MongoDB detection)
	ctx := context.Background()
	parserService := services.InitializeMessageParserService(db)
	if parserService != nil {
		engine.parserService = parserService

		// Also get MongoDB service for raw storage
		mongoConnService, err := services.NewMongoDBConnectionService()
		if err == nil {
			err = mongoConnService.Connect(ctx)
			if err == nil {
				engine.mongoService = services.NewMongoDBMessageService(
					mongoConnService.GetClient(),
					mongoConnService.GetDatabase(),
				)
				fmt.Printf("✅ Parser Service initialized with MongoDB\n")
			}
		}
	}

	// OOB: Auto-initialize transformation service (will be nil if MongoDB unavailable)
	// TODO: Re-enable when TransformationService is implemented
	// engine.transformationService = services.InitializeTransformationService(db)

	return engine
}

// Start starts the processing engine
func (pe *ProcessingEngine) Start() error {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if pe.running {
		return fmt.Errorf("engine is already running")
	}

	pe.running = true
	pe.stats.StartTime = time.Now()
	pe.stats.LastActivity = time.Now()

	return nil
}

// Stop stops the processing engine
func (pe *ProcessingEngine) Stop() error {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if !pe.running {
		return fmt.Errorf("engine is not running")
	}

	pe.running = false
	return nil
}

// IsRunning returns whether the engine is running
func (pe *ProcessingEngine) IsRunning() bool {
	pe.mutex.RLock()
	defer pe.mutex.RUnlock()
	return pe.running
}

// ActivateInterface activates an interface for processing
func (pe *ProcessingEngine) ActivateInterface(interfaceID string) error {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	// Check if already active
	if _, exists := pe.activeInterfaces[interfaceID]; exists {
		return fmt.Errorf("interface already active")
	}

	// Get interface details from database
	var name, sourceConfigJSON string
	err := pe.db.QueryRow(`
		SELECT name, source_config
		FROM interfaces
		WHERE id = $1
	`, interfaceID).Scan(&name, &sourceConfigJSON)
	if err != nil {
		return fmt.Errorf("interface not found: %v", err)
	}

	// Parse source config
	var sourceConfig map[string]interface{}
	if err := json.Unmarshal([]byte(sourceConfigJSON), &sourceConfig); err != nil {
		return fmt.Errorf("failed to parse source config: %v", err)
	}

	// Add interface_id to config for connector
	sourceConfig["interface_id"] = interfaceID

	// Create connector based on source type
	connector, err := NewTCPInputConnector(sourceConfig)
	if err != nil {
		return fmt.Errorf("failed to create connector: %v", err)
	}

	// Create message channel for this interface
	messageChan := make(chan Message, 100) // Buffered channel

	// Start connector
	ctx := context.Background()
	go func() {
		if err := connector.Start(ctx, messageChan); err != nil {
			fmt.Printf("❌ Connector error for interface %s: %v\n", interfaceID, err)
		}
	}()

	// Start message processor
	go pe.processMessages(interfaceID, messageChan)

	// Update interface status in database
	_, err = pe.db.Exec("UPDATE interfaces SET status = 'active' WHERE id = $1", interfaceID)
	if err != nil {
		return fmt.Errorf("failed to activate interface: %v", err)
	}

	// Track in memory
	pe.activeInterfaces[interfaceID] = &InterfaceStatus{
		InterfaceID:       interfaceID,
		Name:             name,
		Status:           "active",
		MessagesProcessed: 0,
		LastActivity:     time.Now(),
		Errors:           0,
	}

	pe.activeConnectors[interfaceID] = connector
	pe.messageChan[interfaceID] = messageChan

	pe.stats.LastActivity = time.Now()
	fmt.Printf("✅ Interface activated: %s (%s)\n", name, interfaceID)
	return nil
}

// DeactivateInterface deactivates an interface
func (pe *ProcessingEngine) DeactivateInterface(interfaceID string) error {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	// Update interface status in database
	_, err := pe.db.Exec("UPDATE interfaces SET status = 'inactive' WHERE id = $1", interfaceID)
	if err != nil {
		return fmt.Errorf("failed to deactivate interface: %v", err)
	}

	// Remove from active tracking
	delete(pe.activeInterfaces, interfaceID)

	pe.stats.LastActivity = time.Now()
	return nil
}

// GetInterfaceStatus returns the status of a specific interface
func (pe *ProcessingEngine) GetInterfaceStatus(interfaceID string) *InterfaceStatus {
	pe.mutex.RLock()
	defer pe.mutex.RUnlock()

	if status, exists := pe.activeInterfaces[interfaceID]; exists {
		return status
	}

	// If not in active list, check database
	var name, status string
	err := pe.db.QueryRow("SELECT name, status FROM interfaces WHERE id = $1", interfaceID).Scan(&name, &status)
	if err != nil {
		return &InterfaceStatus{
			InterfaceID: interfaceID,
			Status:      "unknown",
			Name:        "Unknown",
		}
	}

	return &InterfaceStatus{
		InterfaceID: interfaceID,
		Name:        name,
		Status:      status,
		LastActivity: time.Now(),
	}
}

// GetStats returns current engine statistics
func (pe *ProcessingEngine) GetStats() *EngineStats {
	pe.mutex.RLock()
	defer pe.mutex.RUnlock()

	pe.stats.ActiveInterfaces = len(pe.activeInterfaces)

	// Calculate totals from active interfaces
	var totalMessages, totalErrors int64
	for _, iface := range pe.activeInterfaces {
		totalMessages += iface.MessagesProcessed
		totalErrors += iface.Errors
	}

	pe.stats.TotalMessagesProcessed = totalMessages
	pe.stats.TotalErrors = totalErrors

	return pe.stats
}

// GetActiveInterfaces returns all currently active interfaces
func (pe *ProcessingEngine) GetActiveInterfaces() map[string]*InterfaceStatus {
	pe.mutex.RLock()
	defer pe.mutex.RUnlock()

	result := make(map[string]*InterfaceStatus)
	for k, v := range pe.activeInterfaces {
		result[k] = v
	}
	return result
}

// ==================== TRANSFORMATION METHODS (MVC + OOB) ====================
// These methods delegate to the auto-initialized TransformationService

// TransformStoredMessage retrieves parsedJSON from MongoDB and transforms it to FHIR
// MVC + OOB: Delegates to TransformationService which handles:
//   1. Retrieve parsedJSON from MongoDB
//   2. Load interface-specific mapping from interfaces table
//   3. Execute transformation pipeline
//   4. Store FHIR output in hybrid storage
func (pe *ProcessingEngine) TransformStoredMessage(
	ctx context.Context,
	interfaceID string,
	messageID string,
) (*services.TransformationResult, error) {
	// TODO: Implement when TransformationService is ready
	return nil, fmt.Errorf("transformation service not yet implemented")
}

// TransformInterfaceMessages transforms all untransformed messages for an interface
func (pe *ProcessingEngine) TransformInterfaceMessages(
	ctx context.Context,
	interfaceID string,
	limit int,
) ([]*services.TransformationResult, error) {
	// TODO: Implement when TransformationService is ready
	return nil, fmt.Errorf("transformation service not yet implemented")
}