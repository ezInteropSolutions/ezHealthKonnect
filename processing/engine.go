// processing/engine.go
// Simple Processing Engine for Configuration Controller

package processing

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// ProcessingEngine provides basic interface engine functionality
type ProcessingEngine struct {
	db               *sql.DB
	activeInterfaces map[string]*InterfaceStatus
	mutex           sync.RWMutex
	stats           *EngineStats
	running         bool
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
func NewProcessingEngine(db *sql.DB) *ProcessingEngine {
	return &ProcessingEngine{
		db:               db,
		activeInterfaces: make(map[string]*InterfaceStatus),
		stats: &EngineStats{
			StartTime:             time.Now(),
			LastActivity:          time.Now(),
			AverageProcessingTime: "0ms",
		},
		running: false,
	}
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

	// Get interface details from database
	var name string
	err := pe.db.QueryRow("SELECT name FROM interfaces WHERE id = $1", interfaceID).Scan(&name)
	if err != nil {
		return fmt.Errorf("interface not found: %v", err)
	}

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

	pe.stats.LastActivity = time.Now()
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