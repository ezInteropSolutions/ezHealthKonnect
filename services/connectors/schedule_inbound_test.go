package connectors

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

type memCursors struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memCursors) Load(_ context.Context, id, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[id+"/"+key]
	return v, ok, nil
}
func (s *memCursors) Save(_ context.Context, id, key, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[id+"/"+key] = v
	return nil
}

func newSchedule(t *testing.T, cfg string, now time.Time) (*ScheduleInboundConnector, *memCursors) {
	t.Helper()
	c := NewScheduleInboundConnector().(*ScheduleInboundConnector)
	if err := c.Initialize([]byte(cfg)); err != nil {
		t.Fatal(err)
	}
	cur := &memCursors{m: map[string]string{}}
	c.SetCursorStore(cur)
	c.now = func() time.Time { return now }
	return c, cur
}

func TestScheduleInitializeRefusesWhatCannotRun(t *testing.T) {
	for name, cfg := range map[string]string{
		"nothing set":      `{}`,
		"both":             `{"interval_minutes":5,"daily_at":"02:00"}`,
		"bad time":         `{"daily_at":"25:00"}`,
		"not a time":       `{"daily_at":"soon"}`,
		"unknown timezone": `{"daily_at":"02:00","timezone":"Mars/Base"}`,
	} {
		if err := NewScheduleInboundConnector().Initialize([]byte(cfg)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestScheduleNextRun(t *testing.T) {
	at := func(s string) time.Time { x, _ := time.Parse(time.RFC3339, s); return x }
	every, _ := newSchedule(t, `{"interval_minutes":15}`, time.Now())
	if got := every.nextRun(at("2026-11-18T10:00:00Z")); !got.Equal(at("2026-11-18T10:15:00Z")) {
		t.Errorf("interval: %v", got)
	}
	daily, _ := newSchedule(t, `{"daily_at":"02:30"}`, time.Now())
	if got := daily.nextRun(at("2026-11-18T01:00:00Z")); !got.Equal(at("2026-11-18T02:30:00Z")) {
		t.Errorf("before today's time: %v", got)
	}
	if got := daily.nextRun(at("2026-11-18T02:30:00Z")); !got.Equal(at("2026-11-19T02:30:00Z")) {
		t.Errorf("at today's time: %v", got)
	}
	// 02:30 in New York (UTC-5 in November)
	ny, _ := newSchedule(t, `{"daily_at":"02:30","timezone":"America/New_York"}`, time.Now())
	if got := ny.nextRun(at("2026-11-18T00:00:00Z")); !got.Equal(at("2026-11-18T07:30:00Z")) {
		t.Errorf("zone: %v", got)
	}
}

func TestScheduleRunCarriesSinceAndAdvancesOnlyOnSuccess(t *testing.T) {
	t0 := time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC)
	c, cur := newSchedule(t, `{"interval_minutes":60,"interface_id":"if1"}`, t0)
	ch := make(chan *models.InboundMessage, 4)

	run := func() map[string]interface{} {
		t.Helper()
		c.fire(context.Background(), ch)
		select {
		case m := <-ch:
			var body map[string]interface{}
			if err := json.Unmarshal([]byte(m.Content), &body); err != nil {
				t.Fatal(err)
			}
			body["_id"] = m.MessageID
			return body
		default:
			return nil
		}
	}

	first := run()
	if first["since"] != "" || first["scheduled_at"] != "2026-11-18T03:00:00Z" || first["event"] != "scheduled" {
		t.Fatalf("first run: %v", first)
	}
	// still being processed: the next tick is skipped, not queued
	if again := run(); again != nil {
		t.Fatalf("a run started while one was in flight: %v", again)
	}
	// it fails: since does not move, and the next run may start
	c.OnMessageOutcome(models.MessageOutcome{MessageID: first["_id"].(string), Status: models.OutcomeFailed})
	if _, found, _ := cur.Load(context.Background(), "if1", "last_success"); found {
		t.Fatal("a failed run moved 'since'")
	}
	c.now = func() time.Time { return t0.Add(time.Hour) }
	second := run()
	if second == nil || second["since"] != "" {
		t.Fatalf("second run: %v", second)
	}
	// it succeeds: since becomes that run's own start time
	c.OnMessageOutcome(models.MessageOutcome{MessageID: second["_id"].(string), Status: models.OutcomeProcessed})
	if v, _, _ := cur.Load(context.Background(), "if1", "last_success"); v != "2026-11-18T04:00:00Z" {
		t.Fatalf("since after success: %q", v)
	}
	c.now = func() time.Time { return t0.Add(2 * time.Hour) }
	if third := run(); third == nil || third["since"] != "2026-11-18T04:00:00Z" {
		t.Fatalf("third run: %v", third)
	}
}

func TestScheduleOverlapStartsSinceEarlierAndStampIsFileSafe(t *testing.T) {
	t0 := time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC)
	c, cur := newSchedule(t, `{"interval_minutes":60,"interface_id":"if1","overlap_minutes":5}`, t0)
	_ = cur.Save(context.Background(), "if1", "last_success", "2026-11-17T03:00:00Z")
	ch := make(chan *models.InboundMessage, 1)
	c.fire(context.Background(), ch)
	var body map[string]interface{}
	if err := json.Unmarshal([]byte((<-ch).Content), &body); err != nil {
		t.Fatal(err)
	}
	if body["since"] != "2026-11-17T02:55:00Z" || body["stamp"] != "20261118T030000Z" {
		t.Errorf("run: %v", body)
	}
	if err := NewScheduleInboundConnector().Initialize([]byte(`{"interval_minutes":5,"overlap_minutes":-1}`)); err == nil {
		t.Error("a negative overlap was accepted")
	}
}

func TestScheduleOutcomeOfSomeoneElsesMessageIsIgnored(t *testing.T) {
	c, cur := newSchedule(t, `{"interval_minutes":5,"interface_id":"if1"}`, time.Now())
	c.OnMessageOutcome(models.MessageOutcome{MessageID: "other", Status: models.OutcomeProcessed})
	if len(cur.m) != 0 {
		t.Errorf("cursor changed: %v", cur.m)
	}
}

func TestScheduleReleasesARunThatNeverReportsAnOutcome(t *testing.T) {
	t0 := time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC)
	c, _ := newSchedule(t, `{"interval_minutes":5}`, t0)
	ch := make(chan *models.InboundMessage, 2)
	c.fire(context.Background(), ch)
	c.now = func() time.Time { return t0.Add(scheduleOutcomeLimit + time.Minute) }
	c.fire(context.Background(), ch)
	if len(ch) != 2 {
		t.Fatalf("%d runs; the stuck one was not released", len(ch))
	}
}

func TestScheduleQueueFullSkipsTheRunAndFreesTheSlot(t *testing.T) {
	c, _ := newSchedule(t, `{"interval_minutes":5}`, time.Now())
	ch := make(chan *models.InboundMessage) // nobody receiving
	c.fire(context.Background(), ch)
	if c.inFlight != "" || len(c.started) != 0 {
		t.Fatalf("slot not freed: %q %v", c.inFlight, c.started)
	}
	if !strings.Contains(scheduleTypeName, "schedule") {
		t.Fatal("type name")
	}
}
