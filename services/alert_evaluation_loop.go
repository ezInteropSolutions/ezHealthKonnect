package services

import (
	"context"
	"log"
	"time"
)

// runEvery calls fn once per interval until ctx ends. A panic in fn is
// logged and does not stop the loop.
func runEvery(ctx context.Context, interval time.Duration, fn func(context.Context)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("⚠️  alert evaluation: %v", r)
					}
				}()
				fn(ctx)
			}()
		}
	}
}

// StartAlertEvaluation checks the alert rules every interval in the
// background. Rules are evaluated whenever the monitoring summary is built;
// without this, that happened only while someone had the monitoring page
// open, so an alert for a failed message could wait indefinitely.
func (s *AnalyticsService) StartAlertEvaluation(ctx context.Context, interval time.Duration) {
	go runEvery(ctx, interval, func(c context.Context) {
		if _, err := s.GetMonitoringSummary(c); err != nil {
			log.Printf("⚠️  alert evaluation: %v", err)
		}
	})
}
