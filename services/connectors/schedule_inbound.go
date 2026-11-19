// services/connectors/schedule_inbound.go
//
// Scheduled Trigger — an inbound connector that starts a pipeline on a
// schedule instead of when something arrives: every N minutes, or every day
// at a set time. It is generic: what the pipeline does when it is triggered
// (call an API, start an export, poll a system) is the pipeline's business.
//
// Each run is a message (visible in the message list like any other):
//
//	{"event": "scheduled", "run": 7,
//	 "scheduled_at": "2026-11-18T03:00:00Z",   // when this run started
//	 "stamp": "20261118T030000Z",              // the same instant, safe in a file name
//	 "since": "2026-11-17T03:00:00Z"}          // when the last SUCCESSFUL run started, less overlap_minutes; "" for the first
//
// "since" is what makes a scheduled pull incremental: a run asks the source
// for what changed after "since". It advances only when a run is fully
// processed (OutcomeAwareConnector), to that run's own start time, so a run
// that fails is covered again by the next one, and nothing falls between
// runs. The value is kept durably (CursorAwareConnector), so a restart does
// not lose it.
//
// One run at a time: a tick that arrives while the previous run is still
// being processed is skipped (and logged), never queued behind it.
//
// Configuration:
//
//	interval_minutes  int     Run every N minutes (use this or daily_at)
//	daily_at          string  Run every day at HH:MM (24 hour) in the time zone below
//	timezone          string  IANA name for daily_at (default "UTC")
//	run_on_start      bool    Also run once when the interface is activated (default false)
//	overlap_minutes   int     Start "since" this many minutes early, so a clock a little behind
//	                          the source's cannot let a change fall between two runs (default 0)
//	cursor_key        string  Name of the saved "since" (default "last_success")
package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"
)

const (
	scheduleTypeName     = "schedule_inbound"
	scheduleDefaultCurs  = "last_success"
	scheduleMinInterval  = 1
	scheduleOutcomeLimit = 24 * time.Hour // longest a run may take before the next is allowed
)

// ScheduleInboundConnector starts a pipeline on a schedule.
type ScheduleInboundConnector struct {
	*BaseInboundConnector

	interval    time.Duration
	dailyHour   int
	dailyMinute int
	useDaily    bool
	location    *time.Location
	runOnStart  bool
	overlap     time.Duration
	cursorKey   string
	interfaceID string
	now         func() time.Time // replaceable in tests

	mu            sync.Mutex
	cursors       CursorStore
	inFlight      string // message ID of the run being processed, "" when idle
	inFlightSince time.Time
	started       map[string]string
	runs          int
}

// NewScheduleInboundConnector creates the trigger.
func NewScheduleInboundConnector() InboundConnector {
	return &ScheduleInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(ConnectorMetadata{
			TypeName: scheduleTypeName, DisplayName: "Scheduled Trigger", Version: "1.0.0",
			Category: "inbound", Mode: "pull", ImplementationLang: "go",
			Capabilities: map[string]bool{"supports_cron": true},
		}),
		cursorKey: scheduleDefaultCurs,
		now:       time.Now,
		started:   map[string]string{},
	}
}

// Initialize parses configuration.
func (c *ScheduleInboundConnector) Initialize(config []byte) error {
	var cfg struct {
		IntervalMinutes int    `json:"interval_minutes"`
		DailyAt         string `json:"daily_at"`
		Timezone        string `json:"timezone"`
		RunOnStart      bool   `json:"run_on_start"`
		OverlapMinutes  int    `json:"overlap_minutes"`
		CursorKey       string `json:"cursor_key"`
		InterfaceID     string `json:"interface_id"`
	}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("schedule trigger: invalid configuration: %w", err)
	}
	daily := strings.TrimSpace(cfg.DailyAt)
	switch {
	case cfg.IntervalMinutes > 0 && daily != "":
		return fmt.Errorf("schedule trigger: set either interval_minutes or daily_at, not both")
	case cfg.IntervalMinutes >= scheduleMinInterval:
		c.interval = time.Duration(cfg.IntervalMinutes) * time.Minute
	case daily != "":
		h, m, err := parseClock(daily)
		if err != nil {
			return err
		}
		c.useDaily, c.dailyHour, c.dailyMinute = true, h, m
	default:
		return fmt.Errorf("schedule trigger: set interval_minutes (at least %d) or daily_at (HH:MM)", scheduleMinInterval)
	}
	tz := strings.TrimSpace(cfg.Timezone)
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return fmt.Errorf("schedule trigger: unknown time zone %q (use an IANA name such as Europe/London)", tz)
	}
	c.location = loc
	c.runOnStart = cfg.RunOnStart
	if cfg.OverlapMinutes < 0 {
		return fmt.Errorf("schedule trigger: overlap_minutes cannot be negative")
	}
	c.overlap = time.Duration(cfg.OverlapMinutes) * time.Minute
	if k := strings.TrimSpace(cfg.CursorKey); k != "" {
		c.cursorKey = k
	}
	c.interfaceID = cfg.InterfaceID
	c.BaseInboundConnector.BaseConnector.initialized = true
	c.SetState(StateReady)
	return nil
}

// parseClock reads "HH:MM" (24 hour).
func parseClock(s string) (hour, minute int, err error) {
	var h, m int
	if n, scanErr := fmt.Sscanf(s, "%d:%d", &h, &m); scanErr != nil || n != 2 || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("schedule trigger: daily_at %q is not a time of day (use HH:MM, 24 hour)", s)
	}
	return h, m, nil
}

// nextRun is when the run after now is due: now plus the interval, or the
// next time the clock reads the daily time in the configured zone.
func (c *ScheduleInboundConnector) nextRun(now time.Time) time.Time {
	if !c.useDaily {
		return now.Add(c.interval)
	}
	local := now.In(c.location)
	next := time.Date(local.Year(), local.Month(), local.Day(), c.dailyHour, c.dailyMinute, 0, 0, c.location)
	if !next.After(local) {
		next = time.Date(local.Year(), local.Month(), local.Day()+1, c.dailyHour, c.dailyMinute, 0, 0, c.location)
	}
	return next
}

// Validate checks the configuration.
func (c *ScheduleInboundConnector) Validate() error {
	if !c.useDaily && c.interval <= 0 {
		return fmt.Errorf("schedule trigger: not configured")
	}
	return nil
}

// TestConnection: nothing to reach.
func (c *ScheduleInboundConnector) TestConnection(ctx context.Context) error { return nil }

// SupportsCron: it is the scheduler.
func (c *ScheduleInboundConnector) SupportsCron() bool { return true }

// SetCursorStore implements CursorAwareConnector.
func (c *ScheduleInboundConnector) SetCursorStore(store CursorStore) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cursors = store
}

// OnMessageOutcome implements OutcomeAwareConnector: a successful run
// moves "since" to when that run started.
func (c *ScheduleInboundConnector) OnMessageOutcome(o models.MessageOutcome) {
	c.mu.Lock()
	at, mine := c.started[o.MessageID]
	if mine {
		delete(c.started, o.MessageID)
		if c.inFlight == o.MessageID {
			c.inFlight = ""
		}
	}
	store := c.cursors
	c.mu.Unlock()
	if !mine {
		return
	}
	if !o.Succeeded() {
		log.Printf("⏰ Schedule: run %s ended %s; the next run will cover it again", o.MessageID, o.Status)
		return
	}
	if store == nil {
		log.Printf("⚠️  Schedule: no cursor store, so 'since' was not saved for %s", o.MessageID)
		return
	}
	if err := store.Save(context.Background(), c.interfaceID, c.cursorKey, at); err != nil {
		log.Printf("❌ Schedule: could not save 'since' (%s): %v; the next run will repeat this window", at, err)
	}
}

// Start runs the schedule until the context ends.
func (c *ScheduleInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	c.SetState(StateRunning)
	c.SetConnected(true)
	if c.runOnStart {
		c.fire(ctx, messageChan)
	}
	for {
		wait := c.nextRun(c.now()).Sub(c.now())
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return c.Stop()
		case <-c.BaseInboundConnector.stopCh:
			timer.Stop()
			return nil
		case <-timer.C:
			c.fire(ctx, messageChan)
		}
	}
}

// fire emits one run, unless the previous is still being processed.
func (c *ScheduleInboundConnector) fire(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	c.mu.Lock()
	if c.inFlight != "" && c.now().Sub(c.inFlightSince) > scheduleOutcomeLimit {
		log.Printf("⚠️  Schedule: no outcome was reported for %s in %s; running again", c.inFlight, scheduleOutcomeLimit)
		delete(c.started, c.inFlight)
		c.inFlight = ""
	}
	if c.inFlight != "" {
		prev := c.inFlight
		c.mu.Unlock()
		log.Printf("⏰ Schedule: skipped a run: %s is still being processed", prev)
		return
	}
	store := c.cursors
	c.runs++
	run := c.runs
	c.mu.Unlock()

	since := ""
	if store != nil {
		if v, found, err := store.Load(ctx, c.interfaceID, c.cursorKey); err != nil {
			log.Printf("❌ Schedule: could not read 'since': %v; skipping this run rather than repeat everything", err)
			return
		} else if found {
			since = v
			if t, perr := time.Parse(time.RFC3339, v); perr == nil && c.overlap > 0 {
				since = t.Add(-c.overlap).UTC().Format(time.RFC3339)
			}
		}
	}
	at := c.now().UTC().Format(time.RFC3339)
	body, _ := json.Marshal(map[string]interface{}{"event": "scheduled", "run": run, "scheduled_at": at,
		"stamp": c.now().UTC().Format("20060102T150405Z"), "since": since})
	msg := &models.InboundMessage{
		MessageID:   fmt.Sprintf("schedule_%s_%d", c.now().UTC().Format("20060102T150405"), run),
		Content:     string(body),
		ContentType: "application/json",
		SourceType:  "schedule",
		ReceivedAt:  c.now(),
		MessageSize: len(body),
		Headers:     map[string]string{"X-Scheduled-At": at},
	}
	c.mu.Lock()
	c.inFlight, c.inFlightSince = msg.MessageID, c.now()
	c.started[msg.MessageID] = at
	c.mu.Unlock()
	select {
	case messageChan <- msg:
		log.Printf("⏰ Schedule: run %d started (since %q)", run, since)
	default:
		c.mu.Lock()
		c.inFlight = ""
		delete(c.started, msg.MessageID)
		c.mu.Unlock()
		log.Printf("⚠️  Schedule: the message queue is full; run %d skipped", run)
	}
}

// Stop ends the schedule.
func (c *ScheduleInboundConnector) Stop() error {
	c.SetState(StateStopped)
	c.SetConnected(false)
	return c.BaseInboundConnector.Stop()
}

// GetStatus reports the connector's state.
func (c *ScheduleInboundConnector) GetStatus() ConnectorStatus {
	st := ConnectorStatus{LastActivity: time.Now(), Metadata: map[string]string{}}
	if c.IsRunning() {
		st.State, st.Connected = StateRunning, true
	} else {
		st.State = StateStopped
	}
	return st
}
