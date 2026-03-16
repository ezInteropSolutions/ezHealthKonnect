// processing/engine.go
// Simple Processing Engine for Configuration Controller (MVC + OOB)

package processing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services"
	"ezhealthkonnect/services/backpressure"
	. "ezhealthkonnect/services/connectors" // Import connectors package for factory
	"ezhealthkonnect/services/logger"
	"ezhealthkonnect/services/storage"
)

// backpressureRegistry is a package-level alias so engine.go and engine_message_processor.go
// share the same singleton without circular init dependency.
func backpressureRegistry() *backpressure.Registry { return backpressure.Get() }

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
	parserService        *services.MessageParserService   // JSON conversion service
	objectStorage        *storage.ObjectStorageService    // Object storage (S3/MinIO/local)
	outputDeliveryService *services.OutputDeliveryService // Output delivery service (V21)
	errorService         *services.ErrorCaptureService // Error capture service (V23)
	errorHandler         *ErrorHandler                  // Error handler with panic recovery (V23)
	transformationService *services.TransformationPipelineService // Pipeline-based transformation (MVC + OOB)
	connectionWarmer      *services.ConnectionWarmer             // Connection pre-warming for database steps

	// Validation feedback system (universal for all connectors)
	validationConnectors  map[string]ValidationAwareConnector // interfaceID -> connector
	validationMutex       sync.RWMutex

	// Durable message queue for retry and startup recovery (Phase 2 Durability)
	messageQueue *MessageQueue
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

// NewProcessingEngine creates a new processing engine.
// credStore may be nil; if provided, encrypted connectivity configs are decrypted
// transparently by executors that read credentials from the database (e.g. field_as_path + S3).
// OOB: Auto-initializes MongoDB and parser service if available
func NewProcessingEngine(db *sql.DB, credStore *services.CredentialStore) *ProcessingEngine {
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

	logger.Info("connector factory initialized", "connectors", 32)

	// OOB: Auto-initialize parser service
	parserService := services.InitializeMessageParserService(db)
	if parserService != nil {
		engine.parserService = parserService
		logger.Info("parser service initialized")
	}

	// OOB: Auto-initialize object storage (S3/MinIO or local filesystem)
	storageDriver, err := storage.NewDriverFromEnv()
	if err != nil {
		log.Printf("⚠️  Object storage driver unavailable: %v — falling back to DB-only mode", err)
	} else {
		bucket := storage.DefaultBucketName()
		objSvc, err := storage.NewObjectStorageService(storageDriver, bucket)
		if err != nil {
			log.Printf("⚠️  Object storage service init failed: %v", err)
		} else {
			engine.objectStorage = objSvc
			logger.Info("object storage initialized", "driver", storageDriver.DriverName(), "bucket", bucket)
		}
	}

	// OOB: Auto-initialize transformation pipeline service (MVC pattern)
	engine.transformationService = services.NewTransformationPipelineService(db, credStore)
	if engine.transformationService != nil {
		logger.Info("transformation pipeline service initialized")
		// Wire object storage so the pipeline service can log connector delivery details
		if engine.objectStorage != nil {
			engine.transformationService.SetObjectStorage(engine.objectStorage)
		}
	}

	// OOB: Initialize Output Delivery Service (V21)
	engine.outputDeliveryService = services.NewOutputDeliveryService(db)
	logger.Info("output delivery service initialized")

	// OOB: Initialize Error Capture Service (V23)
	engine.errorService = services.NewErrorCaptureService(db)
	logger.Info("error capture service initialized")

	// OOB: Initialize Error Handler with Panic Recovery (V23)
	engine.errorHandler = NewErrorHandler(engine.errorService)
	logger.Info("error handler initialized (panic recovery enabled)")

	// OOB: Initialize Connection Warmer (pre-warm database connections at deployment)
	engine.connectionWarmer = services.NewConnectionWarmer(db)
	logger.Info("connection warmer initialized")

	// Phase 2 Durability: Initialize durable message queue for retry + startup recovery
	engine.messageQueue = NewMessageQueue(db, engine.transformationService)
	logger.Info("message queue initialized (retry + startup recovery)")

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

	// Phase 2 Durability: start the durable queue worker and recovery scan.
	if pe.messageQueue != nil {
		startCtx := context.Background()
		if err := pe.messageQueue.Start(startCtx); err != nil {
			log.Printf("⚠️  Failed to start message queue: %v", err)
		}
		go pe.recoverUnprocessedMessages(startCtx)
	}

	// Auto-restore: re-activate interfaces that were active before restart.
	// Run in a goroutine so Start() returns immediately.
	go pe.restoreActiveInterfaces()

	return nil
}

// restoreActiveInterfaces queries the DB for interfaces with status='active'
// and re-activates them so listeners restart after an engine restart.
func (pe *ProcessingEngine) restoreActiveInterfaces() {
	rows, err := pe.db.Query(`SELECT id FROM interfaces WHERE status = 'active' AND is_active = TRUE`)
	if err != nil {
		log.Printf("⚠️  Auto-restore: failed to query active interfaces: %v", err)
		return
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		log.Printf("ℹ️  Auto-restore: no active interfaces to restore")
		return
	}

	log.Printf("🔄 Auto-restore: restoring %d active interface(s)...", len(ids))
	for _, id := range ids {
		if err := pe.ActivateInterface(id); err != nil {
			log.Printf("⚠️  Auto-restore: failed to activate interface %s: %v", id, err)
		} else {
			log.Printf("✅ Auto-restore: interface %s activated", id)
		}
	}
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

// ActivateInterface activates an interface for processing.
// Primary path: reads connector.inbound steps from the pipeline (pipeline-driven, supports multiple listeners).
// Fallback: reads source_connectivity from the interfaces table (backward compatibility).
func (pe *ProcessingEngine) ActivateInterface(interfaceID string) error {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	// Check if already active
	if _, exists := pe.activeInterfaces[interfaceID]; exists {
		return fmt.Errorf("interface already active")
	}

	// Get interface name and transformation mapping (for status tracking + connection warming)
	var name string
	var transformationMappingJSON sql.NullString
	err := pe.db.QueryRow(`
		SELECT name, transformation_mapping
		FROM interfaces
		WHERE id = $1
	`, interfaceID).Scan(&name, &transformationMappingJSON)
	if err != nil {
		return fmt.Errorf("interface not found: %v", err)
	}

	// ── PRIMARY PATH: pipeline-driven connector.inbound steps ──────────────────
	// Query connector.inbound steps from the pipeline for this interface.
	// One listener goroutine is started per step, enabling multiple inbound sources.
	type inboundStepRecord struct {
		stepID     string
		stepName   string
		configJSON string
	}

	var pipelineSteps []inboundStepRecord
	rows, qErr := pe.db.Query(`
		SELECT ts.id, ts.step_name, COALESCE(ts.config::text, '{}')
		FROM transformation_steps ts
		JOIN transformation_pipelines tp ON tp.id = ts.pipeline_id
		WHERE tp.interface_id = $1
		  AND ts.step_type = 'connector.inbound'
		  AND ts.enabled = true
		ORDER BY ts.sequence
	`, interfaceID)
	if qErr == nil {
		for rows.Next() {
			var s inboundStepRecord
			if scanErr := rows.Scan(&s.stepID, &s.stepName, &s.configJSON); scanErr == nil {
				pipelineSteps = append(pipelineSteps, s)
			}
		}
		rows.Close()
	}

	if len(pipelineSteps) > 0 {
		// Pipeline-driven: start one listener per connector.inbound step
		for _, step := range pipelineSteps {
			var stepCfg struct {
				ConnectorType string                 `json:"connectorType"`
				Config        map[string]interface{} `json:"config"`
			}
			if jsonErr := json.Unmarshal([]byte(step.configJSON), &stepCfg); jsonErr != nil {
				log.Printf("⚠️  Failed to parse connector.inbound config for step '%s': %v", step.stepName, jsonErr)
				continue
			}
			if stepCfg.ConnectorType == "" {
				log.Printf("⚠️  connector.inbound step '%s' has no connectorType — skipping", step.stepName)
				continue
			}

			oobType := MapLegacyConnectorType(stepCfg.ConnectorType, "inbound")
			innerConfig := stepCfg.Config
			if innerConfig == nil {
				innerConfig = make(map[string]interface{})
			}
			innerConfig["interface_id"] = interfaceID

			connector, connErr := CreateInputConnector(oobType, innerConfig)
			if connErr != nil {
				log.Printf("⚠️  Failed to create '%s' connector for step '%s': %v", oobType, step.stepName, connErr)
				continue
			}

			bufferSize := 10000
			if buf, ok := innerConfig["buffer_size"].(float64); ok {
				bufferSize = int(buf)
			}
			msgChan := make(chan *models.InboundMessage, bufferSize)

			ctx := context.Background()

			// Capture immediate start failures (e.g. "bind: address already in use").
			// Listener-type connectors fail synchronously in Start(); if they succeed
			// they block in the accept loop and never write to this channel.
			startErrCh := make(chan error, 1)
			go func(c InputConnector, sName string) {
				if startErr := c.Start(ctx, msgChan); startErr != nil {
					startErrCh <- startErr
					log.Printf("❌ Connector start error for interface %s (step '%s'): %v", interfaceID, sName, startErr)
				}
			}(connector, step.stepName)

			// 200 ms is generous for a synchronous bind failure to surface.
			// On success the goroutine is blocked in Accept() and startErrCh stays empty.
			select {
			case startErr := <-startErrCh:
				if isPortConflictError(startErr) {
					port := extractConnectorPort(innerConfig)
					log.Printf("🚨 [PHI SAFETY] Port :%d conflict detected activating interface %s — halting both interfaces", port, interfaceID)
					pe.haltOnPortConflictLocked(interfaceID, port, startErr)
					return fmt.Errorf("port :%d conflict — both interfaces halted for PHI safety (see HIPAA audit log)", port)
				}
				// Non-conflict start error: log and skip this step
				log.Printf("⚠️  Non-conflict connector error for step '%s': %v", step.stepName, startErr)
				continue
			case <-time.After(200 * time.Millisecond):
				// No immediate error — connector is running normally
			}

			if vc, ok := connector.(ValidationAwareConnector); ok {
				pe.RegisterValidationConnector(interfaceID, vc)
			}

			// Composite key supports multiple connectors per interface
			key := interfaceID + ":" + step.stepID
			pe.activeConnectors[key] = connector
			pe.messageChan[key] = msgChan
			go pe.processMessages(interfaceID, msgChan)

			log.Printf("✅ Started %s connector for interface %s (step: '%s')", oobType, interfaceID, step.stepName)
		}
	} else {
		// ── FALLBACK: legacy source_connectivity approach ───────────────────────
		// Used for interfaces that were not configured through the wizard pipeline builder.
		log.Printf("⚠️  No connector.inbound pipeline steps for interface %s — falling back to source_connectivity", interfaceID)

		var sourceConfigJSON, sourceConnectivityJSON string
		err = pe.db.QueryRow(`
			SELECT source_config, COALESCE(source_connectivity::text, '{}')
			FROM interfaces WHERE id = $1
		`, interfaceID).Scan(&sourceConfigJSON, &sourceConnectivityJSON)
		if err != nil {
			return fmt.Errorf("failed to read source config: %v", err)
		}

		var sourceConfig map[string]interface{}
		if err := json.Unmarshal([]byte(sourceConfigJSON), &sourceConfig); err != nil {
			return fmt.Errorf("failed to parse source config: %v", err)
		}

		var sourceConnectivity map[string]interface{}
		if err := json.Unmarshal([]byte(sourceConnectivityJSON), &sourceConnectivity); err == nil {
			if connType, ok := sourceConnectivity["type"].(string); ok {
				sourceConfig["connectivity"] = connType
				sourceConfig["type"] = connType
			}
			if connConfig, ok := sourceConnectivity["config"].(map[string]interface{}); ok {
				for k, v := range connConfig {
					sourceConfig[k] = v
				}
			}
		}
		sourceConfig["interface_id"] = interfaceID

		logger.Debug("source config resolved (legacy fallback)", "interface_id", interfaceID)

		sourceType, _ := sourceConfig["type"].(string)
		connectivity, _ := sourceConfig["connectivity"].(string)
		logger.Debug("detected connector type (legacy fallback)",
			"interface_id", interfaceID,
			"source_type", sourceType, "connectivity", connectivity)

		var legacyType string
		if connectivity != "" {
			legacyType = connectivity
		} else if sourceType == "fhir" {
			legacyType = "http"
		} else if sourceType == "hl7v2" || sourceType == "hl7" {
			legacyType = "tcp"
		} else {
			legacyType = "tcp"
		}

		oobTypeName := MapLegacyConnectorType(legacyType, "inbound")
		logger.Debug("creating connector (legacy fallback)",
			"interface_id", interfaceID, "connector_type", oobTypeName)

		connector, connErr := CreateInputConnector(oobTypeName, sourceConfig)
		if connErr != nil {
			return fmt.Errorf("failed to create connector '%s': %v", oobTypeName, connErr)
		}

		bufferSize := 10000
		if customBuffer, ok := sourceConfig["buffer_size"].(float64); ok {
			bufferSize = int(customBuffer)
		}
		messageChan := make(chan *models.InboundMessage, bufferSize)

		ctx := context.Background()
		legacyStartErrCh := make(chan error, 1)
		go func() {
			if err := connector.Start(ctx, messageChan); err != nil {
				legacyStartErrCh <- err
				logger.Error("connector error", "interface_id", interfaceID, "error", err)
			}
		}()

		select {
		case startErr := <-legacyStartErrCh:
			if isPortConflictError(startErr) {
				port := extractConnectorPort(sourceConfig)
				log.Printf("🚨 [PHI SAFETY] Port :%d conflict detected (legacy path) activating interface %s — halting both interfaces", port, interfaceID)
				pe.haltOnPortConflictLocked(interfaceID, port, startErr)
				return fmt.Errorf("port :%d conflict — both interfaces halted for PHI safety (see HIPAA audit log)", port)
			}
			logger.Error("non-conflict connector start error", "interface_id", interfaceID, "error", startErr)
			return fmt.Errorf("connector failed to start: %v", startErr)
		case <-time.After(200 * time.Millisecond):
			// No immediate error — connector is running normally
		}

		if validationConnector, ok := connector.(ValidationAwareConnector); ok {
			pe.RegisterValidationConnector(interfaceID, validationConnector)
		}

		pe.activeConnectors[interfaceID] = connector
		pe.messageChan[interfaceID] = messageChan
		go pe.processMessages(interfaceID, messageChan)
	}

	// Update interface status in database (both columns so page-load initial state is correct)
	_, err = pe.db.Exec("UPDATE interfaces SET status = 'active', interface_status = 'active' WHERE id = $1", interfaceID)
	if err != nil {
		return fmt.Errorf("failed to activate interface: %v", err)
	}

	// Track interface status in memory (one entry per interface regardless of connector count)
	pe.activeInterfaces[interfaceID] = &InterfaceStatus{
		InterfaceID:       interfaceID,
		Name:              name,
		Status:            "active",
		MessagesProcessed: 0,
		LastActivity:      time.Now(),
		Errors:            0,
	}

	pe.stats.LastActivity = time.Now()
	logger.Info("interface activated", "interface_id", interfaceID, "name", name)

	// Connection pre-warming (warm database connections AFTER interface activation)
	if pe.connectionWarmer != nil && transformationMappingJSON.Valid {
		var pipeline map[string]interface{}
		if err := json.Unmarshal([]byte(transformationMappingJSON.String), &pipeline); err == nil {
			var interfaceIDInt int
			fmt.Sscanf(interfaceID, "intf_%d", &interfaceIDInt)
			if interfaceIDInt == 0 {
				fmt.Sscanf(interfaceID, "%d", &interfaceIDInt)
			}
			go func() {
				if err := pe.connectionWarmer.WarmInterfaceConnections(interfaceIDInt, pipeline); err != nil {
					log.Printf("⚠️  Connection warming failed for interface %s: %v", interfaceID, err)
				}
			}()
		}
	}

	return nil
}

// DeactivateInterface deactivates an interface
func (pe *ProcessingEngine) DeactivateInterface(interfaceID string) error {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	// Update interface status in database (both columns so page-load initial state is correct)
	_, err := pe.db.Exec("UPDATE interfaces SET status = 'inactive', interface_status = 'configured' WHERE id = $1", interfaceID)
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

// ==================== PHI SAFETY: PORT CONFLICT HALT ====================

// isPortConflictError returns true when the error indicates a TCP port is already bound.
func isPortConflictError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "bind: address") ||
		strings.Contains(msg, "EADDRINUSE")
}

// extractConnectorPort returns the "port" value from a connector config map, or 0.
func extractConnectorPort(config map[string]interface{}) int {
	if config == nil {
		return 0
	}
	switch v := config["port"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

// haltOnPortConflictLocked stops every connector involved in a port conflict and
// marks all affected interfaces as 'error'.  It MUST be called while pe.mutex is
// already held (e.g. from inside ActivateInterface).
//
// PHI safety rationale: when two interfaces share an inbound port it is impossible
// to guarantee that a message lands in the correct pipeline.  Allowing either
// interface to continue running creates an ambiguous PHI routing risk that cannot
// be resolved without operator intervention.
func (pe *ProcessingEngine) haltOnPortConflictLocked(newInterfaceID string, port int, conflictErr error) {
	// Find other ACTIVE interfaces already bound to this port.
	conflicting := pe.findActiveInterfacesByPort(port)

	// Union: all interfaces that must be stopped.
	affected := make(map[string]bool)
	affected[newInterfaceID] = true
	for _, id := range conflicting {
		affected[id] = true
	}

	reason := fmt.Sprintf(
		"PHI_SAFETY_HALT: port :%d conflict — ambiguous PHI routing. "+
			"All interfaces on this port have been stopped. "+
			"Correct the port configuration and re-activate manually.",
		port,
	)

	affectedList := make([]string, 0, len(affected))
	for id := range affected {
		affectedList = append(affectedList, id)
	}
	log.Printf("🚨 [HIPAA] Halting %d interface(s) due to port :%d conflict: %v", len(affected), port, affectedList)

	// Stop connectors and clean up engine maps (lock already held).
	for id := range affected {
		for key, conn := range pe.activeConnectors {
			if strings.HasPrefix(key, id+":") || key == id {
				_ = conn.Stop()
				if ch, ok := pe.messageChan[key]; ok {
					close(ch)
					delete(pe.messageChan, key)
				}
				delete(pe.activeConnectors, key)
			}
		}
		delete(pe.activeInterfaces, id)
	}

	// DB updates and HIPAA audit are I/O-bound — run asynchronously so we don't
	// hold the engine mutex during database writes.
	go pe.persistPortConflictHalt(affectedList, port, reason)
}

// findActiveInterfacesByPort returns the IDs of currently-active interfaces whose
// connector.inbound pipeline step is configured on the given port.
// Must be called while pe.mutex is held (reads pe.activeInterfaces).
func (pe *ProcessingEngine) findActiveInterfacesByPort(port int) []string {
	activeIDs := make([]string, 0, len(pe.activeInterfaces))
	for id := range pe.activeInterfaces {
		activeIDs = append(activeIDs, id)
	}

	var conflicting []string
	for _, ifaceID := range activeIDs {
		rows, err := pe.db.Query(`
			SELECT ts.config::text
			FROM transformation_steps ts
			JOIN transformation_pipelines tp ON tp.id = ts.pipeline_id
			WHERE tp.interface_id = $1
			  AND ts.step_type = 'connector.inbound'
			  AND ts.enabled = true
		`, ifaceID)
		if err != nil {
			continue
		}
		for rows.Next() {
			var cfgJSON string
			if rows.Scan(&cfgJSON) != nil {
				continue
			}
			var wrapper struct {
				Config map[string]interface{} `json:"config"`
			}
			if json.Unmarshal([]byte(cfgJSON), &wrapper) == nil {
				if extractConnectorPort(wrapper.Config) == port {
					conflicting = append(conflicting, ifaceID)
					break
				}
			}
		}
		rows.Close()
	}
	return conflicting
}

// persistPortConflictHalt updates the database and writes HIPAA audit log entries
// for every interface that was halted due to a port conflict.
// Called asynchronously — does NOT hold pe.mutex.
func (pe *ProcessingEngine) persistPortConflictHalt(interfaceIDs []string, port int, reason string) {
	for _, id := range interfaceIDs {
		// Mark interface as error in both status columns.
		if _, err := pe.db.Exec(`
			UPDATE interfaces
			SET status = 'error', interface_status = 'error', updated_at = NOW()
			WHERE id = $1
		`, id); err != nil {
			log.Printf("⚠️  [PHI SAFETY] Failed to update status for interface %s: %v", id, err)
		}

		// Build HIPAA audit payload.
		auditDetails := map[string]interface{}{
			"port":      port,
			"reason":    reason,
			"action":    "PHI_SAFETY_HALT",
			"timestamp": time.Now().UTC(),
		}
		detailsJSON, _ := json.Marshal(auditDetails)
		complianceFlags := map[string]interface{}{
			"hipaa_safety_halt": true,
			"phi_risk":          "port_conflict_ambiguous_routing",
			"requires_operator_review": true,
		}
		flagsJSON, _ := json.Marshal(complianceFlags)

		if _, err := pe.db.Exec(`
			INSERT INTO audit_logs (
				id, action, entity_type, entity_id,
				new_values, metadata, result, error_message,
				risk_level, compliance_flags, created_at
			) VALUES (
				gen_random_uuid(), $1, $2, $3,
				$4::jsonb, $5::jsonb, $6, $7,
				$8, $9::jsonb, NOW()
			)
		`,
			"PHI_SAFETY_HALT",
			"interface",
			id,
			string(detailsJSON),
			string(detailsJSON),
			"blocked",
			reason,
			"critical",
			string(flagsJSON),
		); err != nil {
			log.Printf("⚠️  [PHI SAFETY] Failed to write HIPAA audit log for interface %s: %v", id, err)
		}

		log.Printf("🚨 [HIPAA AUDIT] Interface %s halted — port :%d conflict. "+
			"Risk: ambiguous PHI routing. Manual operator review required.", id, port)
	}
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
// ==================== BACKPRESSURE METRICS ====================

// GetQueueDepths returns current worker pool metrics for all active interfaces.
// Used by GET /api/system/queue-depths.
func (pe *ProcessingEngine) GetQueueDepths() map[string]map[string]int {
	return backpressureRegistry().QueueDepths()
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

// ==================== STARTUP RECOVERY (Phase 2 Durability) ====================

// recoverUnprocessedMessages scans all interface message tables on startup and
// re-enqueues any messages stuck in 'received' or 'processing' state that have not
// progressed in the last 5 minutes (i.e. the process crashed mid-pipeline).
// Called as a goroutine from Start() after the message queue is running.
func (pe *ProcessingEngine) recoverUnprocessedMessages(ctx context.Context) {
	// Brief startup delay — let connectors initialise before we flood the queue.
	time.Sleep(10 * time.Second)

	log.Printf("🔍 [Recovery] Scanning interface tables for stale messages...")

	// Discover all interface message tables dynamically (same pattern as V52 migration).
	rows, err := pe.db.QueryContext(ctx, `
		SELECT tablename
		FROM pg_tables
		WHERE schemaname = 'public'
		  AND tablename LIKE 'messages_intf_%'
		ORDER BY tablename
	`)
	if err != nil {
		log.Printf("❌ [Recovery] Failed to list interface tables: %v", err)
		return
	}

	var tableNames []string
	for rows.Next() {
		var tbl string
		if scanErr := rows.Scan(&tbl); scanErr == nil {
			tableNames = append(tableNames, tbl)
		}
	}
	rows.Close()

	if len(tableNames) == 0 {
		log.Printf("ℹ️  [Recovery] No interface tables found — nothing to recover")
		return
	}

	staleThreshold := 5 * time.Minute
	cutoff := time.Now().Add(-staleThreshold)
	totalRecovered := 0

	for _, table := range tableNames {
		interfaceID := extractInterfaceIDFromTable(table)
		if interfaceID == "" {
			continue
		}

		// #nosec G201 — table name sourced from pg_tables (system catalog), not user input
		query := fmt.Sprintf(`
			SELECT message_id,
			       COALESCE(message_type, ''),
			       COALESCE(raw_message, '')
			FROM %s
			WHERE status IN ('received', 'processing')
			  AND received_at < $1
		`, table)

		msgRows, queryErr := pe.db.QueryContext(ctx, query, cutoff)
		if queryErr != nil {
			log.Printf("⚠️  [Recovery] Failed to query %s: %v", table, queryErr)
			continue
		}

		type staleMsg struct{ id, msgType, raw string }
		var stale []staleMsg
		for msgRows.Next() {
			var m staleMsg
			if scanErr := msgRows.Scan(&m.id, &m.msgType, &m.raw); scanErr == nil {
				stale = append(stale, m)
			}
		}
		msgRows.Close()

		for _, m := range stale {
			if enqErr := pe.messageQueue.EnqueueForRecovery(interfaceID, m.id, m.msgType, m.raw); enqErr != nil {
				log.Printf("⚠️  [Recovery] Failed to re-enqueue %s: %v", m.id, enqErr)
			} else {
				totalRecovered++
				log.Printf("♻️  [Recovery] Re-enqueued stale message %s (interface: %s)", m.id, interfaceID)
			}
		}
	}

	if totalRecovered > 0 {
		log.Printf("✅ [Recovery] Startup recovery complete — %d message(s) re-enqueued", totalRecovered)
	} else {
		log.Printf("✅ [Recovery] Startup recovery complete — no stale messages found")
	}
}

// extractInterfaceIDFromTable converts a table name like
// "messages_intf_085d4474_bc07_449f_9676_dcebf726292c"
// back to UUID "085d4474-bc07-449f-9676-dcebf726292c".
func extractInterfaceIDFromTable(tableName string) string {
	const prefix = "messages_intf_"
	if len(tableName) <= len(prefix) {
		return ""
	}
	uuidPart := tableName[len(prefix):]
	// UUID uses hyphens; table name uses underscores as separators.
	result := make([]byte, len(uuidPart))
	for i := 0; i < len(uuidPart); i++ {
		if uuidPart[i] == '_' {
			result[i] = '-'
		} else {
			result[i] = uuidPart[i]
		}
	}
	return string(result)
}
