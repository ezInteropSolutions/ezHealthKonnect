// internal/routing/routing_engine.go
// Universal Routing Engine with multi-hop support

package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/processing/pkg"
	"github.com/google/uuid"
)

// RoutingEngine manages message routing across interfaces
type RoutingEngine struct {
	// Core configuration
	routes       map[string]*Route
	ruleEngine   *RuleEngine
	loadBalancer *LoadBalancer
	deadLetter   *DeadLetterQueue

	// State management
	isRunning    bool
	mutex        sync.RWMutex

	// Metrics and monitoring
	metrics      *RoutingMetrics
	metricsMutex sync.RWMutex

	// Event handling
	eventHandlers map[RoutingEventType][]RoutingEventHandler
	eventMutex    sync.RWMutex
}

// Route defines a routing path between interfaces
type Route struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	SourceInterface   string                 `json:"sourceInterface"`
	TargetInterfaces  []string               `json:"targetInterfaces"`
	RoutingRules      []RoutingRule          `json:"routingRules"`
	LoadBalancing     LoadBalancingStrategy  `json:"loadBalancing"`
	Transformations   []TransformationStep   `json:"transformations"`
	RetryPolicy       RetryPolicy            `json:"retryPolicy"`
	Timeout           time.Duration          `json:"timeout"`
	Priority          int                    `json:"priority"`
	IsActive          bool                   `json:"isActive"`
	Tags              []string               `json:"tags"`
	CreatedAt         time.Time              `json:"createdAt"`
	UpdatedAt         time.Time              `json:"updatedAt"`
}

// RoutingRule defines conditional routing logic
type RoutingRule struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Condition   string                 `json:"condition"`   // Expression to evaluate
	Action      RoutingAction          `json:"action"`      // What to do when condition matches
	Priority    int                    `json:"priority"`    // Higher priority rules evaluated first
	Enabled     bool                   `json:"enabled"`
	Parameters  map[string]interface{} `json:"parameters"`
	Description string                 `json:"description"`
}

// RoutingAction defines what action to take
type RoutingAction string

const (
	ActionRoute       RoutingAction = "route"        // Route to specific interface
	ActionMulticast   RoutingAction = "multicast"    // Send to multiple interfaces
	ActionTransform   RoutingAction = "transform"    // Apply transformation
	ActionFilter      RoutingAction = "filter"       // Filter message
	ActionDeadLetter  RoutingAction = "dead_letter"  // Send to dead letter queue
	ActionDelay       RoutingAction = "delay"        // Delay processing
	ActionEnrich      RoutingAction = "enrich"       // Enrich message
)

// TransformationStep defines a transformation in the routing pipeline
type TransformationStep struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`          // transformer type
	Configuration map[string]interface{} `json:"configuration"`
	Enabled       bool                   `json:"enabled"`
	OnError       ErrorAction            `json:"onError"`
}

// ErrorAction defines what to do on transformation error
type ErrorAction string

const (
	ErrorActionContinue   ErrorAction = "continue"    // Continue with original message
	ErrorActionFail       ErrorAction = "fail"        // Fail the entire routing
	ErrorActionDeadLetter ErrorAction = "dead_letter" // Send to dead letter queue
	ErrorActionRetry      ErrorAction = "retry"       // Retry transformation
)

// LoadBalancingStrategy defines load balancing approach
type LoadBalancingStrategy string

const (
	LoadBalanceRoundRobin LoadBalancingStrategy = "round_robin"
	LoadBalanceWeighted   LoadBalancingStrategy = "weighted"
	LoadBalanceRandom     LoadBalancingStrategy = "random"
	LoadBalanceLeastConn  LoadBalancingStrategy = "least_connections"
	LoadBalanceFailover   LoadBalancingStrategy = "failover"
)

// RetryPolicy defines retry behavior for failed routing
type RetryPolicy struct {
	MaxRetries   int           `json:"maxRetries"`
	RetryDelay   time.Duration `json:"retryDelay"`
	BackoffType  BackoffType   `json:"backoffType"`
	MaxDelay     time.Duration `json:"maxDelay"`
}

// BackoffType defines retry backoff strategy
type BackoffType string

const (
	BackoffFixed       BackoffType = "fixed"
	BackoffLinear      BackoffType = "linear"
	BackoffExponential BackoffType = "exponential"
)

// RoutingContext provides context for routing decisions
type RoutingContext struct {
	Message        *pkg.MessageContainer  `json:"message"`
	SourceRoute    string                 `json:"sourceRoute"`
	CurrentHop     int                    `json:"currentHop"`
	MaxHops        int                    `json:"maxHops"`
	StartTime      time.Time              `json:"startTime"`
	Variables      map[string]interface{} `json:"variables"`
	Errors         []RoutingError         `json:"errors"`
	Warnings       []string               `json:"warnings"`
}

// RoutingError represents a routing error
type RoutingError struct {
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	Interface string    `json:"interface"`
	Timestamp time.Time `json:"timestamp"`
	Retryable bool      `json:"retryable"`
}

// RoutingResult represents the result of routing operation
type RoutingResult struct {
	Success       bool                     `json:"success"`
	RoutedTo      []string                 `json:"routedTo"`
	TransformationsApplied []string       `json:"transformationsApplied"`
	TotalHops     int                      `json:"totalHops"`
	TotalLatency  time.Duration            `json:"totalLatency"`
	Errors        []RoutingError           `json:"errors"`
	Warnings      []string                 `json:"warnings"`
	Metadata      map[string]interface{}   `json:"metadata"`
}

// RoutingMetrics tracks routing performance
type RoutingMetrics struct {
	TotalMessages     int64                    `json:"totalMessages"`
	SuccessfulRoutes  int64                    `json:"successfulRoutes"`
	FailedRoutes      int64                    `json:"failedRoutes"`
	AverageLatencyMs  float64                  `json:"averageLatencyMs"`
	RouteStats        map[string]*RouteStats   `json:"routeStats"`
	InterfaceStats    map[string]*InterfaceStats `json:"interfaceStats"`
}

// RouteStats tracks statistics for a specific route
type RouteStats struct {
	MessageCount     int64         `json:"messageCount"`
	SuccessCount     int64         `json:"successCount"`
	ErrorCount       int64         `json:"errorCount"`
	AverageLatencyMs float64       `json:"averageLatencyMs"`
	LastUsed         time.Time     `json:"lastUsed"`
}

// InterfaceStats tracks statistics for an interface
type InterfaceStats struct {
	MessagesReceived int64         `json:"messagesReceived"`
	MessagesSent     int64         `json:"messagesSent"`
	ErrorCount       int64         `json:"errorCount"`
	LastActivity     time.Time     `json:"lastActivity"`
}

// RoutingEventType defines types of routing events
type RoutingEventType string

const (
	EventRouteStarted    RoutingEventType = "route_started"
	EventRouteCompleted  RoutingEventType = "route_completed"
	EventRouteFailed     RoutingEventType = "route_failed"
	EventHopCompleted    RoutingEventType = "hop_completed"
	EventTransformation  RoutingEventType = "transformation"
	EventDeadLetter      RoutingEventType = "dead_letter"
)

// RoutingEvent represents a routing event
type RoutingEvent struct {
	Type        RoutingEventType       `json:"type"`
	RouteID     string                 `json:"routeId"`
	MessageID   string                 `json:"messageId"`
	Interface   string                 `json:"interface"`
	Timestamp   time.Time              `json:"timestamp"`
	Data        map[string]interface{} `json:"data"`
}

// RoutingEventHandler handles routing events
type RoutingEventHandler func(event RoutingEvent)

// NewRoutingEngine creates a new routing engine
func NewRoutingEngine() *RoutingEngine {
	return &RoutingEngine{
		routes:        make(map[string]*Route),
		ruleEngine:    NewRuleEngine(),
		loadBalancer:  NewLoadBalancer(),
		deadLetter:    NewDeadLetterQueue(),
		metrics:       &RoutingMetrics{
			RouteStats:     make(map[string]*RouteStats),
			InterfaceStats: make(map[string]*InterfaceStats),
		},
		eventHandlers: make(map[RoutingEventType][]RoutingEventHandler),
	}
}

// Start initializes the routing engine
func (re *RoutingEngine) Start(ctx context.Context) error {
	re.mutex.Lock()
	defer re.mutex.Unlock()

	if re.isRunning {
		return nil
	}

	// Start sub-components
	if err := re.ruleEngine.Start(ctx); err != nil {
		return fmt.Errorf("failed to start rule engine: %w", err)
	}

	if err := re.loadBalancer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start load balancer: %w", err)
	}

	if err := re.deadLetter.Start(ctx); err != nil {
		return fmt.Errorf("failed to start dead letter queue: %w", err)
	}

	re.isRunning = true
	return nil
}

// Stop shuts down the routing engine
func (re *RoutingEngine) Stop() error {
	re.mutex.Lock()
	defer re.mutex.Unlock()

	if !re.isRunning {
		return nil
	}

	// Stop sub-components
	re.ruleEngine.Stop()
	re.loadBalancer.Stop()
	re.deadLetter.Stop()

	re.isRunning = false
	return nil
}

// RouteMessage routes a message through the routing engine
func (re *RoutingEngine) RouteMessage(ctx context.Context, messageContainer *pkg.MessageContainer) (*RoutingResult, error) {
	startTime := time.Now()

	// Create routing context
	routingCtx := &RoutingContext{
		Message:   messageContainer,
		StartTime: startTime,
		MaxHops:   10, // Default max hops to prevent infinite loops
		Variables: make(map[string]interface{}),
		Errors:    []RoutingError{},
		Warnings:  []string{},
	}

	// Fire route started event
	re.fireEvent(RoutingEvent{
		Type:      EventRouteStarted,
		MessageID: messageContainer.Message.ID,
		Timestamp: startTime,
		Data:      map[string]interface{}{"source_interface": messageContainer.Message.SourceInterface},
	})

	// Find applicable routes
	routes := re.findApplicableRoutes(messageContainer)
	if len(routes) == 0 {
		err := fmt.Errorf("no applicable routes found for message from %s", messageContainer.Message.SourceInterface)
		re.recordFailure(messageContainer.Message.SourceInterface, err)
		return &RoutingResult{
			Success:      false,
			Errors:       []RoutingError{{Code: "NO_ROUTE", Message: err.Error(), Timestamp: time.Now()}},
			TotalLatency: time.Since(startTime),
		}, err
	}

	// Execute routing
	result := &RoutingResult{
		RoutedTo:               []string{},
		TransformationsApplied: []string{},
		Metadata:               make(map[string]interface{}),
	}

	var lastError error
	for _, route := range routes {
		routeResult, err := re.executeRoute(ctx, route, messageContainer, routingCtx)
		if err != nil {
			lastError = err
			routingCtx.Errors = append(routingCtx.Errors, RoutingError{
				Code:      "ROUTE_EXECUTION_FAILED",
				Message:   err.Error(),
				Interface: route.ID,
				Timestamp: time.Now(),
				Retryable: true,
			})
			continue
		}

		// Merge results
		result.RoutedTo = append(result.RoutedTo, routeResult.RoutedTo...)
		result.TransformationsApplied = append(result.TransformationsApplied, routeResult.TransformationsApplied...)
		result.TotalHops += routeResult.TotalHops

		// If we successfully routed to at least one target, consider it a success
		if len(routeResult.RoutedTo) > 0 {
			result.Success = true
		}
	}

	result.TotalLatency = time.Since(startTime)
	result.Errors = routingCtx.Errors
	result.Warnings = routingCtx.Warnings

	// Update metrics
	re.updateMetrics(result, messageContainer.Message.SourceInterface)

	// Fire completion event
	eventType := EventRouteCompleted
	if !result.Success {
		eventType = EventRouteFailed
	}

	re.fireEvent(RoutingEvent{
		Type:      eventType,
		MessageID: messageContainer.Message.ID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"success":      result.Success,
			"routed_to":    result.RoutedTo,
			"total_hops":   result.TotalHops,
			"latency_ms":   result.TotalLatency.Milliseconds(),
		},
	})

	if !result.Success && lastError != nil {
		return result, lastError
	}

	return result, nil
}

// AddRoute adds a new route to the routing engine
func (re *RoutingEngine) AddRoute(route *Route) error {
	re.mutex.Lock()
	defer re.mutex.Unlock()

	if route.ID == "" {
		route.ID = uuid.New().String()
	}

	route.CreatedAt = time.Now()
	route.UpdatedAt = time.Now()

	re.routes[route.ID] = route

	// Initialize statistics
	re.metricsMutex.Lock()
	re.metrics.RouteStats[route.ID] = &RouteStats{}
	re.metricsMutex.Unlock()

	return nil
}

// RemoveRoute removes a route from the routing engine
func (re *RoutingEngine) RemoveRoute(routeID string) error {
	re.mutex.Lock()
	defer re.mutex.Unlock()

	delete(re.routes, routeID)

	// Clean up statistics
	re.metricsMutex.Lock()
	delete(re.metrics.RouteStats, routeID)
	re.metricsMutex.Unlock()

	return nil
}

// UpdateRoute updates an existing route
func (re *RoutingEngine) UpdateRoute(route *Route) error {
	re.mutex.Lock()
	defer re.mutex.Unlock()

	if _, exists := re.routes[route.ID]; !exists {
		return fmt.Errorf("route %s not found", route.ID)
	}

	route.UpdatedAt = time.Now()
	re.routes[route.ID] = route

	return nil
}

// GetRoute retrieves a route by ID
func (re *RoutingEngine) GetRoute(routeID string) (*Route, error) {
	re.mutex.RLock()
	defer re.mutex.RUnlock()

	route, exists := re.routes[routeID]
	if !exists {
		return nil, fmt.Errorf("route %s not found", routeID)
	}

	return route, nil
}

// GetRoutes returns all routes
func (re *RoutingEngine) GetRoutes() map[string]*Route {
	re.mutex.RLock()
	defer re.mutex.RUnlock()

	// Return a copy to prevent external modification
	routes := make(map[string]*Route)
	for id, route := range re.routes {
		routes[id] = route
	}

	return routes
}

// GetMetrics returns current routing metrics
func (re *RoutingEngine) GetMetrics() *RoutingMetrics {
	re.metricsMutex.RLock()
	defer re.metricsMutex.RUnlock()

	// Return a copy
	metrics := &RoutingMetrics{
		TotalMessages:    re.metrics.TotalMessages,
		SuccessfulRoutes: re.metrics.SuccessfulRoutes,
		FailedRoutes:     re.metrics.FailedRoutes,
		AverageLatencyMs: re.metrics.AverageLatencyMs,
		RouteStats:       make(map[string]*RouteStats),
		InterfaceStats:   make(map[string]*InterfaceStats),
	}

	// Copy route stats
	for id, stats := range re.metrics.RouteStats {
		metrics.RouteStats[id] = &RouteStats{
			MessageCount:     stats.MessageCount,
			SuccessCount:     stats.SuccessCount,
			ErrorCount:       stats.ErrorCount,
			AverageLatencyMs: stats.AverageLatencyMs,
			LastUsed:         stats.LastUsed,
		}
	}

	// Copy interface stats
	for id, stats := range re.metrics.InterfaceStats {
		metrics.InterfaceStats[id] = &InterfaceStats{
			MessagesReceived: stats.MessagesReceived,
			MessagesSent:     stats.MessagesSent,
			ErrorCount:       stats.ErrorCount,
			LastActivity:     stats.LastActivity,
		}
	}

	return metrics
}

// AddEventHandler adds an event handler for specific event types
func (re *RoutingEngine) AddEventHandler(eventType RoutingEventType, handler RoutingEventHandler) {
	re.eventMutex.Lock()
	defer re.eventMutex.Unlock()

	if _, exists := re.eventHandlers[eventType]; !exists {
		re.eventHandlers[eventType] = []RoutingEventHandler{}
	}

	re.eventHandlers[eventType] = append(re.eventHandlers[eventType], handler)
}

// Private methods

// findApplicableRoutes finds routes that can handle the message
func (re *RoutingEngine) findApplicableRoutes(messageContainer *pkg.MessageContainer) []*Route {
	re.mutex.RLock()
	defer re.mutex.RUnlock()

	var applicableRoutes []*Route

	for _, route := range re.routes {
		if !route.IsActive {
			continue
		}

		// Check if source interface matches
		if route.SourceInterface != "" && route.SourceInterface != messageContainer.Message.SourceInterface {
			continue
		}

		// Evaluate routing rules
		if re.ruleEngine.EvaluateRouteRules(route.RoutingRules, messageContainer) {
			applicableRoutes = append(applicableRoutes, route)
		}
	}

	// Sort by priority (higher priority first)
	for i := 0; i < len(applicableRoutes)-1; i++ {
		for j := i + 1; j < len(applicableRoutes); j++ {
			if applicableRoutes[i].Priority < applicableRoutes[j].Priority {
				applicableRoutes[i], applicableRoutes[j] = applicableRoutes[j], applicableRoutes[i]
			}
		}
	}

	return applicableRoutes
}

// executeRoute executes a specific route
func (re *RoutingEngine) executeRoute(ctx context.Context, route *Route, messageContainer *pkg.MessageContainer, routingCtx *RoutingContext) (*RoutingResult, error) {
	routeStartTime := time.Now()

	// Check for hop limit
	if routingCtx.CurrentHop >= routingCtx.MaxHops {
		return nil, fmt.Errorf("maximum hop limit (%d) exceeded", routingCtx.MaxHops)
	}

	// Apply transformations
	transformedContainer := messageContainer
	for _, transformation := range route.Transformations {
		if !transformation.Enabled {
			continue
		}

		// TODO: Apply transformation using transformer factory
		// This would integrate with the transformer system we built earlier
		routingCtx.Variables["transformation_applied"] = transformation.Name
	}

	// Determine target interfaces using load balancing
	targetInterfaces, err := re.loadBalancer.SelectTargets(route.TargetInterfaces, route.LoadBalancing, messageContainer)
	if err != nil {
		return nil, fmt.Errorf("load balancing failed: %w", err)
	}

	result := &RoutingResult{
		RoutedTo:               targetInterfaces,
		TransformationsApplied: []string{},
		TotalHops:              1,
		Success:                len(targetInterfaces) > 0,
		TotalLatency:           time.Since(routeStartTime),
	}

	// Add hop to message lineage
	for _, targetInterface := range targetInterfaces {
		transformedContainer.AddHop(messageContainer.Message.SourceInterface, targetInterface, pkg.ProtocolHTTP)
	}

	// Update route statistics
	re.updateRouteStats(route.ID, result.Success, result.TotalLatency)

	return result, nil
}

// updateMetrics updates routing metrics
func (re *RoutingEngine) updateMetrics(result *RoutingResult, sourceInterface string) {
	re.metricsMutex.Lock()
	defer re.metricsMutex.Unlock()

	re.metrics.TotalMessages++

	if result.Success {
		re.metrics.SuccessfulRoutes++
	} else {
		re.metrics.FailedRoutes++
	}

	// Update rolling average latency
	latencyMs := float64(result.TotalLatency.Milliseconds())
	if re.metrics.TotalMessages == 1 {
		re.metrics.AverageLatencyMs = latencyMs
	} else {
		re.metrics.AverageLatencyMs = (re.metrics.AverageLatencyMs*0.9) + (latencyMs*0.1)
	}

	// Update interface statistics
	if _, exists := re.metrics.InterfaceStats[sourceInterface]; !exists {
		re.metrics.InterfaceStats[sourceInterface] = &InterfaceStats{}
	}

	re.metrics.InterfaceStats[sourceInterface].MessagesReceived++
	re.metrics.InterfaceStats[sourceInterface].LastActivity = time.Now()

	if !result.Success {
		re.metrics.InterfaceStats[sourceInterface].ErrorCount++
	}

	// Update target interface statistics
	for _, targetInterface := range result.RoutedTo {
		if _, exists := re.metrics.InterfaceStats[targetInterface]; !exists {
			re.metrics.InterfaceStats[targetInterface] = &InterfaceStats{}
		}

		re.metrics.InterfaceStats[targetInterface].MessagesSent++
		re.metrics.InterfaceStats[targetInterface].LastActivity = time.Now()
	}
}

// updateRouteStats updates statistics for a specific route
func (re *RoutingEngine) updateRouteStats(routeID string, success bool, latency time.Duration) {
	re.metricsMutex.Lock()
	defer re.metricsMutex.Unlock()

	stats, exists := re.metrics.RouteStats[routeID]
	if !exists {
		stats = &RouteStats{}
		re.metrics.RouteStats[routeID] = stats
	}

	stats.MessageCount++
	stats.LastUsed = time.Now()

	if success {
		stats.SuccessCount++
	} else {
		stats.ErrorCount++
	}

	// Update rolling average latency
	latencyMs := float64(latency.Milliseconds())
	if stats.MessageCount == 1 {
		stats.AverageLatencyMs = latencyMs
	} else {
		stats.AverageLatencyMs = (stats.AverageLatencyMs*0.9) + (latencyMs*0.1)
	}
}

// recordFailure records a routing failure
func (re *RoutingEngine) recordFailure(sourceInterface string, err error) {
	re.metricsMutex.Lock()
	defer re.metricsMutex.Unlock()

	re.metrics.TotalMessages++
	re.metrics.FailedRoutes++

	if _, exists := re.metrics.InterfaceStats[sourceInterface]; !exists {
		re.metrics.InterfaceStats[sourceInterface] = &InterfaceStats{}
	}

	re.metrics.InterfaceStats[sourceInterface].ErrorCount++
	re.metrics.InterfaceStats[sourceInterface].LastActivity = time.Now()
}

// fireEvent fires a routing event to registered handlers
func (re *RoutingEngine) fireEvent(event RoutingEvent) {
	re.eventMutex.RLock()
	handlers, exists := re.eventHandlers[event.Type]
	re.eventMutex.RUnlock()

	if !exists {
		return
	}

	// Fire events asynchronously to avoid blocking routing
	go func() {
		for _, handler := range handlers {
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Log panic but don't crash routing
					}
				}()
				handler(event)
			}()
		}
	}()
}