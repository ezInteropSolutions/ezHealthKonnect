// processing/engine.go
// Simple Processing Engine for Configuration Controller (MVC + OOB)

package processing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services"
	. "ezhealthkonnect/services/connectors" // Import connectors package for factory
)

// ProcessingEngine provides basic interface engine functionality
type ProcessingEngine struct {
	db                   *sql.DB
	activeInterfaces     map[string]*InterfaceStatus
	activeConnectors     map[string]InputConnector               // Track active connectors (ALL 32 types)
	messageChan          map[string]chan *models.InboundMessage  // Message channels per interface (UNIFIED MODEL)
	mutex               sync.RWMutex
	stats               *EngineStats
	running             bool
	connectorFactory     ConnectorFactory                        // Factory for creating connectors (OOB pattern)
	parserService        *services.MessageParserService // JSON conversion service
	mongoService         *services.MongoDBMessageService // MongoDB storage
	outputDeliveryService *services.OutputDeliveryService // Output delivery service (V21)
	errorService         *services.ErrorCaptureService // Error capture service (V23)
	errorHandler         *ErrorHandler                  // Error handler with panic recovery (V23)
	transformationService *services.TransformationPipelineService // Pipeline-based transformation (MVC + OOB)

	// Validation feedback system (universal for all connectors)
	validationConnectors  map[string]ValidationAwareConnector // interfaceID -> connector
	validationMutex       sync.RWMutex
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
		messageChan:      make(map[string]chan *models.InboundMessage), // UNIFIED MODEL
		stats: &EngineStats{
			StartTime:             time.Now(),
			LastActivity:          time.Now(),
			AverageProcessingTime: "0ms",
		},
		running:             false,
		connectorFactory:    NewConnectorFactory(), // OOB: Initialize connector factory
		validationConnectors: make(map[string]ValidationAwareConnector),
	}

	fmt.Printf("✅ Connector Factory initialized (32 connectors registered)\n")

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

	// OOB: Auto-initialize transformation pipeline service (MVC pattern)
	engine.transformationService = services.NewTransformationPipelineService(db)
	if engine.transformationService != nil {
		fmt.Printf("✅ Transformation Pipeline Service initialized (MVC + OOB)\n")
	}

	// OOB: Initialize Output Delivery Service (V21)
	engine.outputDeliveryService = services.NewOutputDeliveryService(db)
	fmt.Printf("✅ Output Delivery Service initialized\n")

	// OOB: Initialize Error Capture Service (V23)
	engine.errorService = services.NewErrorCaptureService(db)
	fmt.Printf("✅ Error Capture Service initialized\n")

	// OOB: Initialize Error Handler with Panic Recovery (V23)
	engine.errorHandler = NewErrorHandler(engine.errorService)
	fmt.Printf("✅ Error Handler initialized - panic recovery enabled\n")

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
	var name, sourceConfigJSON, sourceConnectivityJSON string
	err := pe.db.QueryRow(`
		SELECT name, source_config, COALESCE(source_connectivity::text, '{}')
		FROM interfaces
		WHERE id = $1
	`, interfaceID).Scan(&name, &sourceConfigJSON, &sourceConnectivityJSON)
	if err != nil {
		return fmt.Errorf("interface not found: %v", err)
	}

	// Parse source config
	var sourceConfig map[string]interface{}
	if err := json.Unmarshal([]byte(sourceConfigJSON), &sourceConfig); err != nil {
		return fmt.Errorf("failed to parse source config: %v", err)
	}

	// Parse source connectivity (V30 format: {type, config})
	var sourceConnectivity map[string]interface{}
	if err := json.Unmarshal([]byte(sourceConnectivityJSON), &sourceConnectivity); err == nil {
		// Merge connectivity config into source config
		if connType, ok := sourceConnectivity["type"].(string); ok {
			sourceConfig["connectivity"] = connType
			sourceConfig["type"] = connType // For backward compatibility
		}
		if connConfig, ok := sourceConnectivity["config"].(map[string]interface{}); ok {
			// Merge config fields - source_connectivity config takes precedence over source_config
			for k, v := range connConfig {
				sourceConfig[k] = v // ALWAYS overwrite - source_connectivity is the authoritative source
			}
		}
	}

	// Add interface_id to config for connector
	sourceConfig["interface_id"] = interfaceID

	// DEBUG: Log source config
	fmt.Printf("🔍 Source config for %s: %+v\n", interfaceID, sourceConfig)

	// Detect source type and connectivity to determine connector type
	sourceType, _ := sourceConfig["type"].(string)
	connectivity, _ := sourceConfig["connectivity"].(string)

	// DEBUG: Log detected values
	fmt.Printf("🔍 Detected sourceType='%s', connectivity='%s'\n", sourceType, connectivity)

	// OOB: Determine connector type from connectivity (supports ALL 32 connector types)
	var legacyType string
	if connectivity != "" {
		// Use connectivity directly (file, tcp, http, database, kafka, etc.)
		legacyType = connectivity
	} else if sourceType == "fhir" {
		// Backward compatibility: fhir without connectivity -> http
		legacyType = "http"
	} else if sourceType == "hl7v2" || sourceType == "hl7" {
		// Backward compatibility: hl7 without connectivity -> tcp
		legacyType = "tcp"
	} else {
		// Default fallback
		legacyType = "tcp"
	}

	// OOB: Convert legacy type to OOB type name (supports ALL 32 connectors)
	oobTypeName := MapLegacyConnectorType(legacyType, "inbound")

	fmt.Printf("🔍 Creating %s connector for interface %s\n", oobTypeName, interfaceID)

	// OOB: Create connector using unified factory (ALL 32 connectors)
	connector, err := CreateInputConnector(oobTypeName, sourceConfig)
	if err != nil {
		return fmt.Errorf("failed to create connector '%s': %v", oobTypeName, err)
	}

	// Create message channel for this interface (HIGH-VOLUME BUFFER for enterprise scale)
	bufferSize := 10000 // OOB: Configurable per interface for millions/billions of messages
	if customBuffer, ok := sourceConfig["buffer_size"].(float64); ok {
		bufferSize = int(customBuffer)
	}
	messageChan := make(chan *models.InboundMessage, bufferSize) // UNIFIED MODEL + ENTERPRISE BUFFER

	// Start connector
	ctx := context.Background()
	go func() {
		if err := connector.Start(ctx, messageChan); err != nil {
			fmt.Printf("❌ Connector error for interface %s: %v\n", interfaceID, err)
		}
	}()

	// Register connector for validation feedback (if supported)
	if validationConnector, ok := connector.(ValidationAwareConnector); ok {
		pe.RegisterValidationConnector(interfaceID, validationConnector)
	}

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

// GetAllInterfaceStatuses returns statuses for all interfaces in a single query
func (pe *ProcessingEngine) GetAllInterfaceStatuses() map[string]*InterfaceStatus {
	pe.mutex.RLock()
	defer pe.mutex.RUnlock()

	statuses := make(map[string]*InterfaceStatus)

	// Query all interfaces from database
	rows, err := pe.db.Query(`
		SELECT id, name, status, interface_status,
		       COALESCE(total_processed, 0) as total_processed,
		       COALESCE(failed_processed, 0) as failed_processed,
		       updated_at
		FROM interfaces
		WHERE deleted_at IS NULL
		ORDER BY name
	`)
	if err != nil {
		log.Printf("Error querying interfaces: %v", err)
		return statuses
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, status, interfaceStatus string
		var totalProcessed, failedProcessed int64
		var updatedAt time.Time

		err := rows.Scan(&id, &name, &status, &interfaceStatus, &totalProcessed, &failedProcessed, &updatedAt)
		if err != nil {
			log.Printf("Error scanning interface row: %v", err)
			continue
		}

		// Check if interface is in active list (runtime status)
		if activeStatus, exists := pe.activeInterfaces[id]; exists {
			statuses[id] = activeStatus
		} else {
			// Use database status, prefer interface_status over status
			displayStatus := interfaceStatus
			if displayStatus == "" {
				displayStatus = status
			}

			statuses[id] = &InterfaceStatus{
				InterfaceID:       id,
				Name:              name,
				Status:            displayStatus,
				MessagesProcessed: totalProcessed,
				Errors:            failedProcessed,
				LastActivity:      updatedAt,
			}
		}
	}

	return statuses
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
// ==================== VALIDATION FEEDBACK METHODS ====================

// RegisterValidationConnector registers a connector for validation feedback
func (pe *ProcessingEngine) RegisterValidationConnector(interfaceID string, connector ValidationAwareConnector) {
	if connector == nil || !connector.SupportsValidationFeedback() {
		return
	}

	pe.validationMutex.Lock()
	defer pe.validationMutex.Unlock()

	pe.validationConnectors[interfaceID] = connector
	log.Printf("📋 Registered validation connector for interface %s", interfaceID)
}

// PublishValidationFeedback sends validation results to the appropriate connector
func (pe *ProcessingEngine) PublishValidationFeedback(feedback *models.ValidationFeedback) {
	pe.validationMutex.RLock()
	connector, exists := pe.validationConnectors[feedback.InterfaceID]
	pe.validationMutex.RUnlock()

	if !exists {
		log.Printf("⚠️  No validation connector found for interface %s", feedback.InterfaceID)
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := connector.SendValidationResponse(ctx, feedback); err != nil {
			log.Printf("❌ Failed to send validation response for %s: %v", feedback.MessageID, err)
		}
	}()
}
