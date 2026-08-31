// controllers/engine_metrics_controller_test.go
//
// Unit test for EngineMetricsController.liveDLQDepth's nil-db fallback path
// (TC-ENGM-008 depends on the real-DB path, exercised via the Playwright
// suite's DATABASE_URL-gated fixture instead — see tests/playwright/
// engine-metrics.spec.js).
package controllers

import (
	"context"
	"testing"
)

func TestEngineMetricsController_LiveDLQDepth_NilDBFallsBackGracefully(t *testing.T) {
	ec := NewEngineMetricsController(nil)

	value, ok := ec.liveDLQDepth(context.Background())

	if ok {
		t.Fatal("expected ok=false with a nil db, so the caller falls back to the gauge")
	}
	if value != 0 {
		t.Errorf("value = %v, want 0 when ok=false", value)
	}
}
