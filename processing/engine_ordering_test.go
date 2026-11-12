// processing/engine_ordering_test.go
// Tests for per-interface processing order and message outcomes
// (engine_ordering.go).
//
//   TC-ORD-001  normalizeProcessingOrder: only "in_order" means in order
//   TC-ORD-002  applyProcessingOrder: records the order and sizes the pool
//   TC-ORD-003  in order: launchPipeline runs in the caller and reports first
//   TC-ORD-004  parallel: launchPipeline returns before the pipeline finishes
//   TC-ORD-005  in order through a real pool: completion order = arrival order,
//               even when earlier messages are slower
//   TC-ORD-006  outcomes reach every subscriber of the interface, and only it
//   TC-ORD-007  forgetInterfaceOrdering clears order and subscribers
//   TC-ORD-008  struct-literal engine (nil maps) doesn't panic
package processing

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/connectors"
	"ezhealthkonnect/services/backpressure"
)

type recordingOutcomeConnector struct {
	mu       sync.Mutex
	outcomes []models.MessageOutcome
}

func (r *recordingOutcomeConnector) OnMessageOutcome(o models.MessageOutcome) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outcomes = append(r.outcomes, o)
}

func (r *recordingOutcomeConnector) snapshot() []models.MessageOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]models.MessageOutcome(nil), r.outcomes...)
}

func newOrderingTestEngine() *ProcessingEngine {
	return &ProcessingEngine{
		inOrderInterfaces: make(map[string]bool),
		outcomeConnectors: make(map[string][]connectors.OutcomeAwareConnector),
	}
}

func TestNormalizeProcessingOrder(t *testing.T) {
	cases := []struct {
		in   sql.NullString
		want string
	}{
		{sql.NullString{}, ProcessingOrderParallel},
		{sql.NullString{String: "parallel", Valid: true}, ProcessingOrderParallel},
		{sql.NullString{String: "in_order", Valid: true}, ProcessingOrderInOrder},
		{sql.NullString{String: " in_order ", Valid: true}, ProcessingOrderInOrder},
		{sql.NullString{String: "IN_ORDER", Valid: true}, ProcessingOrderParallel},
		{sql.NullString{String: "fifo", Valid: true}, ProcessingOrderParallel},
	}
	for _, c := range cases {
		if got := normalizeProcessingOrder(c.in); got != c.want {
			t.Errorf("normalizeProcessingOrder(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestApplyProcessingOrderSizesPool(t *testing.T) {
	pe := newOrderingTestEngine()
	id := fmt.Sprintf("test-order-%d", time.Now().UnixNano())
	defer backpressure.Get().Remove(id)

	pe.applyProcessingOrder(id, ProcessingOrderInOrder)
	if !pe.isInOrder(id) {
		t.Fatal("in_order interface not recorded as in order")
	}
	if got := backpressure.Get().GetOrCreate(id).MaxWorkers(); got != 1 {
		t.Fatalf("in-order pool has %d workers, want 1", got)
	}

	pe.applyProcessingOrder(id, ProcessingOrderParallel)
	if pe.isInOrder(id) {
		t.Fatal("parallel interface still recorded as in order")
	}
	if got := backpressure.Get().GetOrCreate(id).MaxWorkers(); got <= 1 {
		t.Fatalf("parallel pool has %d workers, want the default (>1)", got)
	}
}

func TestLaunchPipelineInOrderRunsInCaller(t *testing.T) {
	pe := newOrderingTestEngine()
	rec := &recordingOutcomeConnector{}
	pe.registerOutcomeConnector("ifc", rec)
	pe.inOrderInterfaces["ifc"] = true

	ran := false
	pe.launchPipeline(func() models.MessageOutcome {
		ran = true
		return outcomeOf("ifc", "m1", models.OutcomeProcessed, nil)
	}, "ifc")

	if !ran {
		t.Fatal("in-order pipeline did not run before launchPipeline returned")
	}
	got := rec.snapshot()
	if len(got) != 1 || got[0].MessageID != "m1" || !got[0].Succeeded() {
		t.Fatalf("outcome not reported synchronously: %+v", got)
	}
}

func TestLaunchPipelineParallelReturnsAtOnce(t *testing.T) {
	pe := newOrderingTestEngine()
	rec := &recordingOutcomeConnector{}
	pe.registerOutcomeConnector("ifc", rec)

	release := make(chan struct{})
	pe.launchPipeline(func() models.MessageOutcome {
		<-release
		return outcomeOf("ifc", "m1", models.OutcomeFailed, fmt.Errorf("boom"))
	}, "ifc")

	if n := len(rec.snapshot()); n != 0 {
		t.Fatalf("parallel launch reported %d outcomes before the pipeline finished", n)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for len(rec.snapshot()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	got := rec.snapshot()
	if len(got) != 1 || got[0].Status != models.OutcomeFailed || got[0].Error != "boom" {
		t.Fatalf("parallel outcome = %+v, want one failed outcome with its error", got)
	}
}

func TestInOrderPoolCompletesInArrivalOrder(t *testing.T) {
	pe := newOrderingTestEngine()
	id := fmt.Sprintf("test-fifo-%d", time.Now().UnixNano())
	defer backpressure.Get().Remove(id)
	rec := &recordingOutcomeConnector{}
	pe.registerOutcomeConnector(id, rec)
	pe.applyProcessingOrder(id, ProcessingOrderInOrder)

	pool := backpressure.Get().GetOrCreate(id)
	const n = 8
	for i := 0; i < n; i++ {
		i := i
		// Earlier messages are slower: in parallel they would finish last.
		delay := time.Duration(n-i) * 5 * time.Millisecond
		if !pool.Submit(func() {
			pe.launchPipeline(func() models.MessageOutcome {
				time.Sleep(delay)
				return outcomeOf(id, fmt.Sprintf("m%d", i), models.OutcomeProcessed, nil)
			}, id)
		}) {
			t.Fatalf("submit %d refused", i)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	for len(rec.snapshot()) < n && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	got := rec.snapshot()
	if len(got) != n {
		t.Fatalf("got %d outcomes, want %d", len(got), n)
	}
	for i, o := range got {
		if want := fmt.Sprintf("m%d", i); o.MessageID != want {
			t.Fatalf("outcome %d is %s, want %s (order: %v)", i, o.MessageID, want, got)
		}
	}
}

func TestOutcomesReachOnlyTheirInterface(t *testing.T) {
	pe := newOrderingTestEngine()
	a1, a2, b := &recordingOutcomeConnector{}, &recordingOutcomeConnector{}, &recordingOutcomeConnector{}
	pe.registerOutcomeConnector("A", a1)
	pe.registerOutcomeConnector("A", a2)
	pe.registerOutcomeConnector("B", b)
	pe.registerOutcomeConnector("A", nil) // ignored

	pe.reportOutcome(outcomeOf("A", "x", models.OutcomeRejected, fmt.Errorf("queue full")))

	if len(a1.snapshot()) != 1 || len(a2.snapshot()) != 1 {
		t.Fatalf("both subscribers of A should hear the outcome: %v %v", a1.snapshot(), a2.snapshot())
	}
	if len(b.snapshot()) != 0 {
		t.Fatalf("B heard A's outcome: %v", b.snapshot())
	}
}

func TestForgetInterfaceOrdering(t *testing.T) {
	pe := newOrderingTestEngine()
	rec := &recordingOutcomeConnector{}
	pe.registerOutcomeConnector("A", rec)
	pe.inOrderInterfaces["A"] = true

	pe.forgetInterfaceOrdering("A")
	pe.reportOutcome(outcomeOf("A", "x", models.OutcomeProcessed, nil))

	if pe.isInOrder("A") {
		t.Fatal("order not cleared")
	}
	if len(rec.snapshot()) != 0 {
		t.Fatal("a forgotten subscriber still heard outcomes")
	}
}

func TestOrderingOnStructLiteralEngine(t *testing.T) {
	pe := &ProcessingEngine{}
	id := fmt.Sprintf("test-literal-%d", time.Now().UnixNano())
	defer backpressure.Get().Remove(id)

	pe.forgetInterfaceOrdering(id)
	pe.applyProcessingOrder(id, ProcessingOrderInOrder)
	pe.registerOutcomeConnector(id, &recordingOutcomeConnector{})
	pe.reportOutcome(outcomeOf(id, "m", models.OutcomeProcessed, nil))
	if !pe.isInOrder(id) {
		t.Fatal("order not recorded on a struct-literal engine")
	}
}
