// processing/engine_ordering.go
//
// Two related engine capabilities, both per interface:
//
//  1. Processing order. "parallel" (the default) runs up to the worker pool's
//     size of messages at once and starts each pipeline in its own goroutine,
//     so messages can finish — and be delivered — in a different order than
//     they arrived. "in_order" gives the interface a one-worker pool and runs
//     each message's pipeline inside that worker, so a message is not started
//     until the one before it has finished. Integration feeds where order
//     carries meaning (an ADT merge must not overtake the registration it
//     merges) need this.
//
//  2. Message outcomes. An inbound connector implementing
//     OutcomeAwareConnector is told, once per message it emitted, how that
//     message ended (processed, failed, rejected, skipped). The outcome is
//     reported from one place per path — the pipeline launcher, or the worker
//     wrapper when the message never reached a pipeline — rather than at each
//     of the processor's many exits.
package processing

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/connectors"
)

// Interface processing orders (interfaces.processing_order).
const (
	ProcessingOrderParallel = "parallel"
	ProcessingOrderInOrder  = "in_order"
)

// orderedPoolWorkers is the worker count that makes an interface's pool
// strictly first-in, first-out.
const orderedPoolWorkers = 1

// normalizeProcessingOrder maps a stored value to a known order. Anything
// unrecognised is treated as parallel, the behaviour every interface had
// before the setting existed.
func normalizeProcessingOrder(v sql.NullString) string {
	if v.Valid && strings.TrimSpace(v.String) == ProcessingOrderInOrder {
		return ProcessingOrderInOrder
	}
	return ProcessingOrderParallel
}

// applyProcessingOrder records an interface's order and sizes its worker pool
// to match. Called at activation, before any connector starts, so no message
// can reach a pool of the wrong size.
func (pe *ProcessingEngine) applyProcessingOrder(interfaceID, order string) {
	pe.orderingMu.Lock()
	if pe.inOrderInterfaces == nil { // engines built as struct literals (tests)
		pe.inOrderInterfaces = make(map[string]bool)
	}
	if order == ProcessingOrderInOrder {
		pe.inOrderInterfaces[interfaceID] = true
	} else {
		delete(pe.inOrderInterfaces, interfaceID)
	}
	pe.orderingMu.Unlock()

	workers := 0 // registry default
	if order == ProcessingOrderInOrder {
		workers = orderedPoolWorkers
	}
	backpressureRegistry().Resize(interfaceID, workers)
	if order == ProcessingOrderInOrder {
		log.Printf("🔢 Interface %s processes messages in order (one at a time)", interfaceID)
	}
}

// isInOrder reports whether an interface processes messages one at a time.
func (pe *ProcessingEngine) isInOrder(interfaceID string) bool {
	pe.orderingMu.RLock()
	defer pe.orderingMu.RUnlock()
	return pe.inOrderInterfaces[interfaceID]
}

// registerOutcomeConnector subscribes a connector to the outcomes of the
// messages on its interface. An interface can have several inbound
// connectors; each ignores outcomes for message IDs it didn't emit.
func (pe *ProcessingEngine) registerOutcomeConnector(interfaceID string, c connectors.OutcomeAwareConnector) {
	if c == nil {
		return
	}
	pe.orderingMu.Lock()
	defer pe.orderingMu.Unlock()
	if pe.outcomeConnectors == nil { // engines built as struct literals (tests)
		pe.outcomeConnectors = make(map[string][]connectors.OutcomeAwareConnector)
	}
	pe.outcomeConnectors[interfaceID] = append(pe.outcomeConnectors[interfaceID], c)
}

// forgetInterfaceOrdering drops an interface's order and outcome subscribers
// (deactivation).
func (pe *ProcessingEngine) forgetInterfaceOrdering(interfaceID string) {
	pe.orderingMu.Lock()
	defer pe.orderingMu.Unlock()
	delete(pe.inOrderInterfaces, interfaceID)
	delete(pe.outcomeConnectors, interfaceID)
}

// reportOutcome tells the interface's outcome-aware connectors how a message
// ended.
func (pe *ProcessingEngine) reportOutcome(outcome models.MessageOutcome) {
	pe.orderingMu.RLock()
	subscribers := append([]connectors.OutcomeAwareConnector(nil), pe.outcomeConnectors[outcome.InterfaceID]...)
	pe.orderingMu.RUnlock()
	for _, c := range subscribers {
		c.OnMessageOutcome(outcome)
	}
}

// outcomeOf builds an outcome; err may be nil.
func outcomeOf(interfaceID, messageID, status string, err error) models.MessageOutcome {
	o := models.MessageOutcome{InterfaceID: interfaceID, MessageID: messageID, Status: status}
	if err != nil {
		o.Error = err.Error()
	}
	return o
}

// launchPipeline runs a parsed message's pipeline and reports its outcome.
// In order, it runs in the caller's goroutine — the interface's single pool
// worker — so the next message waits for it. In parallel it runs in its own
// goroutine, as it always has.
func (pe *ProcessingEngine) launchPipeline(run func() models.MessageOutcome, interfaceID string) {
	if pe.isInOrder(interfaceID) {
		pe.reportOutcome(run())
		return
	}
	go func() { pe.reportOutcome(run()) }()
}

// panicOutcome is the outcome of a message whose processing panicked.
func panicOutcome(interfaceID, messageID string, r interface{}) models.MessageOutcome {
	return outcomeOf(interfaceID, messageID, models.OutcomeFailed, fmt.Errorf("processing panicked: %v", r))
}

// deliveryTracker records whether any outbound delivery of one message
// failed. Outbound steps report a refused delivery (an MLLP NACK, an HTTP
// error status) through the delivery-status callback without failing the
// pipeline, so the outcome reads it from here.
type deliveryTracker struct {
	mu        sync.Mutex
	anyFailed bool
}

func (d *deliveryTracker) record(status string) {
	if status != "failed" {
		return
	}
	d.mu.Lock()
	d.anyFailed = true
	d.mu.Unlock()
}

func (d *deliveryTracker) failed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.anyFailed
}
