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
// background. Without it, rules were checked only when the monitoring
// summary was built, while someone had the monitoring page open, so an
// alert for a failed message could wait indefinitely. It reads only what the
// rules need (EvaluateAlertRules), never the full summary, whose counts
// across every interface's table are too heavy to run every minute.
func (s *AnalyticsService) StartAlertEvaluation(ctx context.Context, interval time.Duration) {
	go runEvery(ctx, interval, func(c context.Context) {
		if err := s.EvaluateAlertRules(c); err != nil {
			log.Printf("⚠️  alert evaluation: %v", err)
		}
	})
}
