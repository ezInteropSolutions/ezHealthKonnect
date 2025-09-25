// internal/routing/load_balancer.go
// Load balancer for distributing messages across multiple targets

package routing

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"ezhealthkonnect/processing/pkg"
)

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer() *LoadBalancer {
	lb := &LoadBalancer{
		strategies: make(map[LoadBalancingStrategy]LoadBalancingFunc),
		state:      make(map[string]*LoadBalancerState),
	}

	// Register built-in strategies
	lb.registerStrategies()

	return lb
}

// Start initializes the load balancer
func (lb *LoadBalancer) Start(ctx context.Context) error {
	return nil
}

// Stop shuts down the load balancer
func (lb *LoadBalancer) Stop() error {
	return nil
}

// SelectTargets selects targets based on the load balancing strategy
func (lb *LoadBalancer) SelectTargets(targets []string, strategy LoadBalancingStrategy, message *pkg.MessageContainer) ([]string, error) {
	if len(targets) == 0 {
		return nil, fmt.Errorf("no targets available")
	}

	if len(targets) == 1 {
		return targets, nil
	}

	// Get strategy function
	strategyFunc, exists := lb.strategies[strategy]
	if !exists {
		// Default to round robin
		strategyFunc = lb.strategies[LoadBalanceRoundRobin]
	}

	// Get or create state for this target group
	stateKey := lb.getStateKey(targets)
	lb.mutex.Lock()
	state, exists := lb.state[stateKey]
	if !exists {
		state = &LoadBalancerState{
			RoundRobinIndex: 0,
			Weights:         make(map[string]int),
			Connections:     make(map[string]int),
			LastUsed:        make(map[string]time.Time),
			FailureCount:    make(map[string]int),
			IsHealthy:       make(map[string]bool),
		}

		// Initialize default state
		for _, target := range targets {
			state.Weights[target] = 1
			state.Connections[target] = 0
			state.FailureCount[target] = 0
			state.IsHealthy[target] = true
		}

		lb.state[stateKey] = state
	}
	lb.mutex.Unlock()

	// Select targets using strategy
	return strategyFunc(targets, state, message)
}

// UpdateTargetHealth updates the health status of a target
func (lb *LoadBalancer) UpdateTargetHealth(target string, isHealthy bool) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	// Update health status in all relevant states
	for _, state := range lb.state {
		if _, exists := state.IsHealthy[target]; exists {
			state.IsHealthy[target] = isHealthy
			if !isHealthy {
				state.FailureCount[target]++
			} else {
				state.FailureCount[target] = 0 // Reset on recovery
			}
		}
	}
}

// UpdateConnectionCount updates the connection count for a target
func (lb *LoadBalancer) UpdateConnectionCount(target string, delta int) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	// Update connection count in all relevant states
	for _, state := range lb.state {
		if _, exists := state.Connections[target]; exists {
			state.Connections[target] += delta
			if state.Connections[target] < 0 {
				state.Connections[target] = 0
			}
		}
	}
}

// SetTargetWeight sets the weight for a target
func (lb *LoadBalancer) SetTargetWeight(target string, weight int) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	// Update weight in all relevant states
	for _, state := range lb.state {
		if _, exists := state.Weights[target]; exists {
			state.Weights[target] = weight
		}
	}
}

// GetStats returns load balancer statistics
func (lb *LoadBalancer) GetStats() map[string]*LoadBalancerState {
	lb.mutex.RLock()
	defer lb.mutex.RUnlock()

	// Return a copy of the state
	stats := make(map[string]*LoadBalancerState)
	for key, state := range lb.state {
		stateCopy := &LoadBalancerState{
			RoundRobinIndex: state.RoundRobinIndex,
			Weights:         make(map[string]int),
			Connections:     make(map[string]int),
			LastUsed:        make(map[string]time.Time),
			FailureCount:    make(map[string]int),
			IsHealthy:       make(map[string]bool),
		}

		for k, v := range state.Weights {
			stateCopy.Weights[k] = v
		}
		for k, v := range state.Connections {
			stateCopy.Connections[k] = v
		}
		for k, v := range state.LastUsed {
			stateCopy.LastUsed[k] = v
		}
		for k, v := range state.FailureCount {
			stateCopy.FailureCount[k] = v
		}
		for k, v := range state.IsHealthy {
			stateCopy.IsHealthy[k] = v
		}

		stats[key] = stateCopy
	}

	return stats
}

// registerStrategies registers built-in load balancing strategies
func (lb *LoadBalancer) registerStrategies() {
	lb.strategies[LoadBalanceRoundRobin] = lb.roundRobinStrategy
	lb.strategies[LoadBalanceWeighted] = lb.weightedStrategy
	lb.strategies[LoadBalanceRandom] = lb.randomStrategy
	lb.strategies[LoadBalanceLeastConn] = lb.leastConnectionsStrategy
	lb.strategies[LoadBalanceFailover] = lb.failoverStrategy
}

// roundRobinStrategy implements round-robin load balancing
func (lb *LoadBalancer) roundRobinStrategy(targets []string, state *LoadBalancerState, message *pkg.MessageContainer) ([]string, error) {
	healthyTargets := lb.getHealthyTargets(targets, state)
	if len(healthyTargets) == 0 {
		return nil, fmt.Errorf("no healthy targets available")
	}

	// Get next target in round-robin fashion
	index := state.RoundRobinIndex % len(healthyTargets)
	selectedTarget := healthyTargets[index]

	// Update index for next call
	state.RoundRobinIndex++
	state.LastUsed[selectedTarget] = time.Now()

	return []string{selectedTarget}, nil
}

// weightedStrategy implements weighted load balancing
func (lb *LoadBalancer) weightedStrategy(targets []string, state *LoadBalancerState, message *pkg.MessageContainer) ([]string, error) {
	healthyTargets := lb.getHealthyTargets(targets, state)
	if len(healthyTargets) == 0 {
		return nil, fmt.Errorf("no healthy targets available")
	}

	// Calculate total weight
	totalWeight := 0
	for _, target := range healthyTargets {
		totalWeight += state.Weights[target]
	}

	if totalWeight == 0 {
		// Fallback to round robin if no weights
		return lb.roundRobinStrategy(targets, state, message)
	}

	// Select target based on weight
	random := rand.Intn(totalWeight)
	currentWeight := 0

	for _, target := range healthyTargets {
		currentWeight += state.Weights[target]
		if random < currentWeight {
			state.LastUsed[target] = time.Now()
			return []string{target}, nil
		}
	}

	// Fallback (shouldn't reach here)
	return []string{healthyTargets[0]}, nil
}

// randomStrategy implements random load balancing
func (lb *LoadBalancer) randomStrategy(targets []string, state *LoadBalancerState, message *pkg.MessageContainer) ([]string, error) {
	healthyTargets := lb.getHealthyTargets(targets, state)
	if len(healthyTargets) == 0 {
		return nil, fmt.Errorf("no healthy targets available")
	}

	// Select random target
	index := rand.Intn(len(healthyTargets))
	selectedTarget := healthyTargets[index]
	state.LastUsed[selectedTarget] = time.Now()

	return []string{selectedTarget}, nil
}

// leastConnectionsStrategy implements least connections load balancing
func (lb *LoadBalancer) leastConnectionsStrategy(targets []string, state *LoadBalancerState, message *pkg.MessageContainer) ([]string, error) {
	healthyTargets := lb.getHealthyTargets(targets, state)
	if len(healthyTargets) == 0 {
		return nil, fmt.Errorf("no healthy targets available")
	}

	// Find target with least connections
	var selectedTarget string
	minConnections := int(^uint(0) >> 1) // Max int

	for _, target := range healthyTargets {
		connections := state.Connections[target]
		if connections < minConnections {
			minConnections = connections
			selectedTarget = target
		}
	}

	if selectedTarget == "" {
		selectedTarget = healthyTargets[0]
	}

	state.LastUsed[selectedTarget] = time.Now()
	state.Connections[selectedTarget]++

	return []string{selectedTarget}, nil
}

// failoverStrategy implements failover load balancing
func (lb *LoadBalancer) failoverStrategy(targets []string, state *LoadBalancerState, message *pkg.MessageContainer) ([]string, error) {
	// Try targets in order until we find a healthy one
	for _, target := range targets {
		if state.IsHealthy[target] {
			state.LastUsed[target] = time.Now()
			return []string{target}, nil
		}
	}

	return nil, fmt.Errorf("no healthy targets available for failover")
}

// getHealthyTargets filters targets to only include healthy ones
func (lb *LoadBalancer) getHealthyTargets(targets []string, state *LoadBalancerState) []string {
	var healthyTargets []string

	for _, target := range targets {
		if state.IsHealthy[target] {
			healthyTargets = append(healthyTargets, target)
		}
	}

	return healthyTargets
}

// getStateKey generates a unique key for a set of targets
func (lb *LoadBalancer) getStateKey(targets []string) string {
	// Sort targets to ensure consistent key regardless of order
	sortedTargets := make([]string, len(targets))
	copy(sortedTargets, targets)

	// Simple sort
	for i := 0; i < len(sortedTargets)-1; i++ {
		for j := i + 1; j < len(sortedTargets); j++ {
			if sortedTargets[i] > sortedTargets[j] {
				sortedTargets[i], sortedTargets[j] = sortedTargets[j], sortedTargets[i]
			}
		}
	}

	// Join with separator
	key := ""
	for i, target := range sortedTargets {
		if i > 0 {
			key += "|"
		}
		key += target
	}

	return key
}

// NewDeadLetterQueue creates a new dead letter queue
func NewDeadLetterQueue() *DeadLetterQueue {
	return &DeadLetterQueue{
		messages:        []*DeadLetterMessage{},
		maxSize:         1000,
		retentionPeriod: 24 * time.Hour,
	}
}

// Start initializes the dead letter queue
func (dlq *DeadLetterQueue) Start(ctx context.Context) error {
	dlq.mutex.Lock()
	defer dlq.mutex.Unlock()

	dlq.isRunning = true

	// Start cleanup goroutine
	go dlq.cleanupRoutine(ctx)

	return nil
}

// Stop shuts down the dead letter queue
func (dlq *DeadLetterQueue) Stop() error {
	dlq.mutex.Lock()
	defer dlq.mutex.Unlock()

	dlq.isRunning = false
	return nil
}

// AddMessage adds a message to the dead letter queue
func (dlq *DeadLetterQueue) AddMessage(messageContainer *pkg.MessageContainer, reason string) error {
	dlq.mutex.Lock()
	defer dlq.mutex.Unlock()

	// Check if queue is full
	if len(dlq.messages) >= dlq.maxSize {
		// Remove oldest message
		dlq.messages = dlq.messages[1:]
	}

	// Create dead letter message
	deadLetterMsg := &DeadLetterMessage{
		ID:              fmt.Sprintf("dl_%d", time.Now().UnixNano()),
		OriginalMessage: messageContainer,
		Reason:          reason,
		FailureCount:    1,
		LastAttempt:     time.Now(),
		CreatedAt:       time.Now(),
		Metadata:        make(map[string]interface{}),
	}

	// Check if this message already exists (by correlation ID)
	for _, existingMsg := range dlq.messages {
		if existingMsg.OriginalMessage.Message.CorrelationID == messageContainer.Message.CorrelationID {
			// Update existing message
			existingMsg.FailureCount++
			existingMsg.LastAttempt = time.Now()
			existingMsg.Reason = reason
			return nil
		}
	}

	// Add new message
	dlq.messages = append(dlq.messages, deadLetterMsg)

	return nil
}

// GetMessages returns all messages in the dead letter queue
func (dlq *DeadLetterQueue) GetMessages() []*DeadLetterMessage {
	dlq.mutex.RLock()
	defer dlq.mutex.RUnlock()

	// Return a copy
	messages := make([]*DeadLetterMessage, len(dlq.messages))
	copy(messages, dlq.messages)

	return messages
}

// GetMessage returns a specific message by ID
func (dlq *DeadLetterQueue) GetMessage(id string) (*DeadLetterMessage, error) {
	dlq.mutex.RLock()
	defer dlq.mutex.RUnlock()

	for _, msg := range dlq.messages {
		if msg.ID == id {
			return msg, nil
		}
	}

	return nil, fmt.Errorf("message %s not found in dead letter queue", id)
}

// RetryMessage attempts to retry a message from the dead letter queue
func (dlq *DeadLetterQueue) RetryMessage(id string) (*pkg.MessageContainer, error) {
	dlq.mutex.Lock()
	defer dlq.mutex.Unlock()

	for i, msg := range dlq.messages {
		if msg.ID == id {
			// Remove from dead letter queue
			dlq.messages = append(dlq.messages[:i], dlq.messages[i+1:]...)

			// Return the original message for retry
			return msg.OriginalMessage, nil
		}
	}

	return nil, fmt.Errorf("message %s not found in dead letter queue", id)
}

// PurgeMessage removes a message from the dead letter queue
func (dlq *DeadLetterQueue) PurgeMessage(id string) error {
	dlq.mutex.Lock()
	defer dlq.mutex.Unlock()

	for i, msg := range dlq.messages {
		if msg.ID == id {
			dlq.messages = append(dlq.messages[:i], dlq.messages[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("message %s not found in dead letter queue", id)
}

// GetStats returns dead letter queue statistics
func (dlq *DeadLetterQueue) GetStats() map[string]interface{} {
	dlq.mutex.RLock()
	defer dlq.mutex.RUnlock()

	stats := map[string]interface{}{
		"total_messages": len(dlq.messages),
		"max_size":       dlq.maxSize,
		"retention_period": dlq.retentionPeriod.String(),
		"is_running":     dlq.isRunning,
	}

	// Calculate failure reasons distribution
	reasonCounts := make(map[string]int)
	for _, msg := range dlq.messages {
		reasonCounts[msg.Reason]++
	}
	stats["failure_reasons"] = reasonCounts

	// Calculate age distribution
	now := time.Now()
	ageBuckets := map[string]int{
		"< 1h":   0,
		"1-6h":   0,
		"6-24h":  0,
		"> 24h":  0,
	}

	for _, msg := range dlq.messages {
		age := now.Sub(msg.CreatedAt)
		if age < time.Hour {
			ageBuckets["< 1h"]++
		} else if age < 6*time.Hour {
			ageBuckets["1-6h"]++
		} else if age < 24*time.Hour {
			ageBuckets["6-24h"]++
		} else {
			ageBuckets["> 24h"]++
		}
	}
	stats["age_distribution"] = ageBuckets

	return stats
}

// cleanupRoutine periodically cleans up old messages
func (dlq *DeadLetterQueue) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour) // Run cleanup every hour
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dlq.cleanup()
		}
	}
}

// cleanup removes messages older than the retention period
func (dlq *DeadLetterQueue) cleanup() {
	dlq.mutex.Lock()
	defer dlq.mutex.Unlock()

	if !dlq.isRunning {
		return
	}

	cutoff := time.Now().Add(-dlq.retentionPeriod)
	var remainingMessages []*DeadLetterMessage

	for _, msg := range dlq.messages {
		if msg.CreatedAt.After(cutoff) {
			remainingMessages = append(remainingMessages, msg)
		}
	}

	dlq.messages = remainingMessages
}

// SetMaxSize sets the maximum size of the dead letter queue
func (dlq *DeadLetterQueue) SetMaxSize(maxSize int) {
	dlq.mutex.Lock()
	defer dlq.mutex.Unlock()

	dlq.maxSize = maxSize

	// Trim if necessary
	if len(dlq.messages) > maxSize {
		dlq.messages = dlq.messages[len(dlq.messages)-maxSize:]
	}
}

// SetRetentionPeriod sets the retention period for messages
func (dlq *DeadLetterQueue) SetRetentionPeriod(period time.Duration) {
	dlq.mutex.Lock()
	defer dlq.mutex.Unlock()

	dlq.retentionPeriod = period
}