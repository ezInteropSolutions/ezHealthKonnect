// services/connectors/fhir_subscription_inbound_test.go
// Tests for the FHIR Subscription (topic-based) receiver, against a fake
// FHIR server. Synthetic data only.
//
//	TC-FS-001  subscribes to each topic (Backport profile, filters, content,
//	           heartbeat, endpoint, header); a second run updates, not adds
//	TC-FS-002  a subscription gone from the server is made again
//	TC-FS-003  handshakes answered; wrong credential 401; topic mismatch 400
//	TC-FS-004  events in order, one at a time, position saved; a retry skips
//	TC-FS-005  a gap is filled from $events first
//	TC-FS-006  a failed event stops with 500 and saves nothing past it
//	TC-FS-007  notifications read: entries, erasures, context; malformed refused
//	TC-FS-008  configuration validated
package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

const (
	topicCreated = "https://ezinteropsolutions.com/fhir/ezidentitysense/SubscriptionTopic/patient-created"
	topicMerged  = "https://ezinteropsolutions.com/fhir/ezidentitysense/SubscriptionTopic/patient-merged"
)

// fakeFHIR records subscription requests and answers $events.
type fakeFHIR struct {
	mu     sync.Mutex
	subs   map[string]map[string]interface{}
	posts  []map[string]interface{}
	puts   []string
	next   int
	events map[int64]string // event number -> focus EID, for $events
	auth   []string
	// limits refuses subscriptions to a topic with 422: "id-only" refuses
	// full-resource, "all" refuses every one.
	limits   map[string]string
	srv      *httptest.Server
	baseFull string
}

func newFakeFHIR(t *testing.T) *fakeFHIR {
	f := &fakeFHIR{subs: map[string]map[string]interface{}{}, events: map[int64]string{}, limits: map[string]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	f.baseFull = f.srv.URL + "/fhir/r4"
	return f
}

func (f *fakeFHIR) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	path := strings.TrimPrefix(r.URL.Path, "/fhir/r4")
	body, _ := io.ReadAll(r.Body)
	w.Header().Set("Content-Type", "application/fhir+json")
	if (r.Method == http.MethodPost || r.Method == http.MethodPut) && strings.HasPrefix(path, "/Subscription") {
		var sub map[string]interface{}
		_ = json.Unmarshal(body, &sub)
		topic, _ := sub["criteria"].(string)
		if limit := f.limits[topic]; limit == "all" || (limit == "id-only" && subscriptionContent(sub) == "full-resource") {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"resourceType":"OperationOutcome","issue":[{"severity":"error","code":"processing","diagnostics":"refused for the test"}]}`))
			return
		}
	}
	switch {
	case r.Method == http.MethodPost && path == "/Subscription":
		var sub map[string]interface{}
		_ = json.Unmarshal(body, &sub)
		f.next++
		id := fmt.Sprintf("sub-%d", f.next)
		sub["id"] = id
		f.subs[id] = sub
		f.posts = append(f.posts, sub)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(sub)
	case r.Method == http.MethodPut && strings.HasPrefix(path, "/Subscription/"):
		id := strings.TrimPrefix(path, "/Subscription/")
		f.puts = append(f.puts, id)
		if _, ok := f.subs[id]; !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"resourceType":"OperationOutcome"}`))
			return
		}
		var sub map[string]interface{}
		_ = json.Unmarshal(body, &sub)
		f.subs[id] = sub
		_ = json.NewEncoder(w).Encode(sub)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/$events"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/Subscription/"), "/$events")
		since, _ := strconv.ParseInt(r.URL.Query().Get("eventsSinceNumber"), 10, 64)
		until, _ := strconv.ParseInt(r.URL.Query().Get("eventsUntilNumber"), 10, 64)
		var evs []testEvent
		for n := since; n <= until; n++ {
			if eid, ok := f.events[n]; ok {
				evs = append(evs, testEvent{n, eid, nil, false})
			}
		}
		_, _ = w.Write(notificationBundle(id, topicCreated, "query-event", evs))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// subscriptionContent is the payload content a Subscription asks for.
func subscriptionContent(sub map[string]interface{}) string {
	channel, _ := sub["channel"].(map[string]interface{})
	payload, _ := channel["_payload"].(map[string]interface{})
	exts, _ := payload["extension"].([]interface{})
	for _, e := range exts {
		if m, ok := e.(map[string]interface{}); ok && m["url"] == backportContent {
			v, _ := m["valueCode"].(string)
			return v
		}
	}
	return ""
}

type testEvent struct {
	number  int64
	focus   string
	context []string
	erased  bool
}

// notificationBundle is a Backport R4 notification, full-resource.
func notificationBundle(sub, topic, typ string, events []testEvent) []byte {
	var params []interface{}
	params = append(params,
		map[string]interface{}{"name": "subscription", "valueReference": map[string]string{"reference": "https://empi.example/fhir/r4/Subscription/" + sub}},
		map[string]interface{}{"name": "topic", "valueCanonical": topic},
		map[string]interface{}{"name": "status", "valueCode": "active"},
		map[string]interface{}{"name": "type", "valueCode": typ})
	var entries []interface{}
	for _, e := range events {
		parts := []interface{}{map[string]interface{}{"name": "event-number", "valueString": strconv.FormatInt(e.number, 10)},
			map[string]interface{}{"name": "timestamp", "valueInstant": "2026-11-12T10:00:00Z"},
			map[string]interface{}{"name": "focus", "valueReference": map[string]string{"reference": "Patient/" + e.focus}}}
		for _, c := range e.context {
			parts = append(parts, map[string]interface{}{"name": "additional-context", "valueReference": map[string]string{"reference": "Patient/" + c}})
		}
		params = append(params, map[string]interface{}{"name": "notification-event", "part": parts})
		for _, eid := range append([]string{e.focus}, e.context...) {
			entry := map[string]interface{}{"fullUrl": "https://empi.example/fhir/r4/Patient/" + eid}
			if e.erased && eid == e.focus {
				entry["request"] = map[string]string{"method": "DELETE", "url": "Patient/" + eid}
			} else {
				entry["resource"] = map[string]interface{}{"resourceType": "Patient", "id": eid, "name": []interface{}{map[string]string{"family": "Synthwell"}}}
				entry["request"] = map[string]string{"method": "PUT", "url": "Patient/" + eid}
			}
			entries = append(entries, entry)
		}
	}
	status := map[string]interface{}{"resourceType": "Parameters", "parameter": params}
	b, _ := json.Marshal(map[string]interface{}{"resourceType": "Bundle", "type": "history",
		"entry": append([]interface{}{map[string]interface{}{"fullUrl": "urn:uuid:7c9e6679-7425-40de-944b-e07fc1f90ae7", "resource": status}}, entries...)})
	return b
}

// subEngine reads emitted messages and reports outcomes; failNumber fails.
type subEngine struct {
	mu         sync.Mutex
	seen       []string // event numbers, in order
	contents   []string
	failNumber string
}

func (e *subEngine) run(c *FHIRSubscriptionInboundConnector, ch <-chan *models.InboundMessage) {
	for msg := range ch {
		e.mu.Lock()
		e.seen = append(e.seen, msg.Headers["fhir-event-number"])
		e.contents = append(e.contents, msg.Content)
		fail := msg.Headers["fhir-event-number"] == e.failNumber
		e.mu.Unlock()
		go func(m *models.InboundMessage) {
			status := models.OutcomeProcessed
			if fail {
				status = models.OutcomeFailed
			}
			c.OnMessageOutcome(models.MessageOutcome{InterfaceID: "ifc-1", MessageID: m.MessageID, Status: status})
		}(msg)
	}
}

func (e *subEngine) order() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.seen...)
}

type subHarness struct {
	c       *FHIRSubscriptionInboundConnector
	fhir    *fakeFHIR
	cursors *memCursorStore
	engine  *subEngine
	url     string
}

func startSubReceiver(t *testing.T) *subHarness {
	t.Helper()
	f := newFakeFHIR(t)
	port := freePort(t)
	cfg := map[string]interface{}{
		"port": port, "interface_id": "ifc-1", "fhir_base_url": f.baseFull,
		"callback_url": fmt.Sprintf("http://127.0.0.1:%d/fhir-notifications", port),
		"auth_type":    "bearer_token", "bearer_token": "synthetic-bearer",
		"topics": []string{topicCreated, topicMerged}, "filters": []string{"source-system=LAB"},
		"content": "full-resource", "heartbeat_seconds": 60, "reply_within_seconds": 5,
	}
	raw, _ := json.Marshal(cfg)
	c := NewFHIRSubscriptionInboundConnector().(*FHIRSubscriptionInboundConnector)
	if err := c.Initialize(raw); err != nil {
		t.Fatal(err)
	}
	h := &subHarness{c: c, fhir: f, cursors: &memCursorStore{}, engine: &subEngine{}, url: cfg["callback_url"].(string)}
	c.SetCursorStore(h.cursors)
	ch := make(chan *models.InboundMessage)
	go h.engine.run(c, ch)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = c.Start(ctx, ch) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := len(f.posts)
		f.mu.Unlock()
		if n == 2 {
			return h
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the receiver didn't subscribe")
	return nil
}

func (h *subHarness) post(t *testing.T, topic string, body []byte) (int, map[string]interface{}) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, h.url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/fhir+json")
	if topic != "" {
		req.Header.Set("Authorization", "Bearer "+h.c.headerToken(topic))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestFHIRSubscriptionSubscribesAndRenews(t *testing.T) {
	h := startSubReceiver(t)
	h.fhir.mu.Lock()
	posts := append([]map[string]interface{}(nil), h.fhir.posts...)
	auth := append([]string(nil), h.fhir.auth...)
	h.fhir.mu.Unlock()
	for i, topic := range []string{topicCreated, topicMerged} {
		raw, _ := json.Marshal(posts[i])
		s := string(raw)
		for _, want := range []string{`"criteria":"` + topic + `"`, `"type":"rest-hook"`, `"endpoint":"` + h.url + `"`, backportProfile,
			`"valueString":"source-system=LAB"`, `"valueCode":"full-resource"`, `"valueUnsignedInt":60`,
			`"Authorization: Bearer ` + h.c.headerToken(topic) + `"`} {
			if !strings.Contains(s, want) {
				t.Errorf("subscription to %s lacks %s: %s", topic, want, s)
			}
		}
	}
	if auth[0] != "Bearer synthetic-bearer" {
		t.Errorf("the server wasn't called with the credential: %q", auth[0])
	}
	if h.c.headerToken(topicCreated) == h.c.headerToken(topicMerged) {
		t.Error("two topics share a header token")
	}
	// The ids are kept; the next run updates rather than adds.
	if err := h.c.ensureSubscriptions(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.fhir.mu.Lock()
	defer h.fhir.mu.Unlock()
	if len(h.fhir.posts) != 2 || strings.Join(h.fhir.puts, ",") != "sub-1,sub-2" {
		t.Errorf("renewal: %d posts, puts %v", len(h.fhir.posts), h.fhir.puts)
	}
}

func TestFHIRSubscriptionRemadeWhenGone(t *testing.T) {
	h := startSubReceiver(t)
	h.fhir.mu.Lock()
	delete(h.fhir.subs, "sub-1")
	h.fhir.mu.Unlock()
	if err := h.c.ensureSubscriptions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := h.cursors.Load(context.Background(), "ifc-1", "subscription|"+topicCreated); v != "sub-3" {
		t.Errorf("the gone subscription wasn't made again: %q", v)
	}
}

// A topic the server offers less of is subscribed with id-only, by PUT and
// by POST; a topic refused outright doesn't hold up the others.
func TestFHIRSubscriptionContentDowngradedPerTopic(t *testing.T) {
	h := startSubReceiver(t)
	h.fhir.mu.Lock()
	h.fhir.limits[topicMerged] = "id-only"
	h.fhir.mu.Unlock()
	if err := h.c.ensureSubscriptions(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.fhir.mu.Lock()
	if got := subscriptionContent(h.fhir.subs["sub-2"]); got != "id-only" {
		t.Errorf("renewed merged subscription asks for %q, want id-only", got)
	}
	if got := subscriptionContent(h.fhir.subs["sub-1"]); got != "full-resource" {
		t.Errorf("created subscription asks for %q, want full-resource", got)
	}
	delete(h.fhir.subs, "sub-2")
	h.fhir.limits[topicCreated] = "all"
	h.fhir.mu.Unlock()

	err := h.c.ensureSubscriptions(context.Background())
	if err == nil || !strings.Contains(err.Error(), topicCreated) || strings.Contains(err.Error(), topicMerged) {
		t.Fatalf("want an error naming only the refused topic, got %v", err)
	}
	v, _, _ := h.cursors.Load(context.Background(), "ifc-1", "subscription|"+topicMerged)
	h.fhir.mu.Lock()
	defer h.fhir.mu.Unlock()
	if v != "sub-3" || subscriptionContent(h.fhir.subs["sub-3"]) != "id-only" {
		t.Errorf("merged subscription wasn't made again as id-only: id %q, %v", v, h.fhir.subs["sub-3"])
	}
}

// The engine hands over the cursor store just after Start: the receiver
// subscribes within seconds of getting it, not a minute later.
func TestFHIRSubscriptionSubscribesOnceStoreArrives(t *testing.T) {
	f := newFakeFHIR(t)
	port := freePort(t)
	raw, _ := json.Marshal(map[string]interface{}{
		"port": port, "interface_id": "ifc-1", "fhir_base_url": f.baseFull,
		"callback_url": fmt.Sprintf("http://127.0.0.1:%d/fhir-notifications", port),
		"auth_type":    "bearer_token", "bearer_token": "synthetic-bearer", "topics": []string{topicCreated},
	})
	c := NewFHIRSubscriptionInboundConnector().(*FHIRSubscriptionInboundConnector)
	if err := c.Initialize(raw); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = c.Start(ctx, make(chan *models.InboundMessage)) }()
	time.Sleep(300 * time.Millisecond)
	c.SetCursorStore(&memCursorStore{})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := len(f.posts)
		f.mu.Unlock()
		if n == 1 {
			if c.GetStatus().ErrorCount != 0 {
				t.Errorf("waiting for the store was recorded as an error")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the receiver didn't subscribe once its store arrived")
}

func TestFHIRSubscriptionHandshakeAndCredentials(t *testing.T) {
	h := startSubReceiver(t)
	if st, _ := h.post(t, topicCreated, notificationBundle("sub-1", topicCreated, "handshake", nil)); st != 200 {
		t.Errorf("handshake: %d", st)
	}
	if st, _ := h.post(t, topicCreated, notificationBundle("sub-1", topicCreated, "heartbeat", nil)); st != 200 {
		t.Errorf("heartbeat: %d", st)
	}
	if st, _ := h.post(t, "", notificationBundle("sub-1", topicCreated, "handshake", nil)); st != 401 {
		t.Errorf("no credential: %d", st)
	}
	req, _ := http.NewRequest(http.MethodPost, h.url, bytes.NewReader(notificationBundle("sub-1", topicCreated, "handshake", nil)))
	req.Header.Set("Authorization", "Bearer synthetic-bearer")
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 401 {
		t.Errorf("the server's own credential accepted as a notification's")
	}
	// Merged's credential on a created notification.
	if st, _ := h.post(t, topicMerged, notificationBundle("sub-1", topicCreated, "event-notification", []testEvent{{1, "EID-1", nil, false}})); st != 400 {
		t.Errorf("topic mismatch: %d", st)
	}
	if len(h.engine.order()) != 0 {
		t.Error("a refused notification was processed")
	}
}

func TestFHIRSubscriptionEventsInOrderAndRetrySkipped(t *testing.T) {
	h := startSubReceiver(t)
	body := notificationBundle("sub-1", topicCreated, "event-notification", []testEvent{{2, "EID-2", nil, false}, {1, "EID-1", nil, false}})
	st, out := h.post(t, topicCreated, body)
	if st != 200 || out["processed"] != float64(2) {
		t.Fatalf("events: %d %v", st, out)
	}
	if got := strings.Join(h.engine.order(), ","); got != "1,2" {
		t.Errorf("order %s", got)
	}
	var m eventMessage
	_ = json.Unmarshal([]byte(h.engine.contents[0]), &m)
	if m.Topic != "patient-created" || m.EventNumber != 1 || m.Focus == nil || m.Focus.ID != "EID-1" || !strings.Contains(string(m.Focus.Resource), "Synthwell") {
		t.Errorf("message: %s", h.engine.contents[0])
	}
	if v, _, _ := h.cursors.Load(context.Background(), "ifc-1", "events|sub-1"); v != "2" {
		t.Errorf("position %q", v)
	}
	st, out = h.post(t, topicCreated, body)
	if st != 200 || out["skipped"] != float64(2) || len(h.engine.order()) != 2 {
		t.Errorf("retry: %d %v (%d messages)", st, out, len(h.engine.order()))
	}
}

func TestFHIRSubscriptionGapFilled(t *testing.T) {
	h := startSubReceiver(t)
	h.fhir.mu.Lock()
	h.fhir.events[2], h.fhir.events[3] = "EID-2", "EID-3"
	h.fhir.mu.Unlock()
	if st, _ := h.post(t, topicCreated, notificationBundle("sub-1", topicCreated, "event-notification", []testEvent{{1, "EID-1", nil, false}})); st != 200 {
		t.Fatal(st)
	}
	st, out := h.post(t, topicCreated, notificationBundle("sub-1", topicCreated, "event-notification", []testEvent{{4, "EID-4", nil, false}}))
	if st != 200 || out["processed"] != float64(3) {
		t.Fatalf("after a gap: %d %v", st, out)
	}
	if got := strings.Join(h.engine.order(), ","); got != "1,2,3,4" {
		t.Errorf("order %s", got)
	}
}

func TestFHIRSubscriptionFailedEventStops(t *testing.T) {
	h := startSubReceiver(t)
	h.engine.failNumber = "2"
	st, out := h.post(t, topicCreated, notificationBundle("sub-1", topicCreated, "event-notification",
		[]testEvent{{1, "EID-1", nil, false}, {2, "EID-2", nil, false}, {3, "EID-3", nil, false}}))
	if st != 500 || out["processed"] != float64(1) {
		t.Fatalf("failure: %d %v", st, out)
	}
	if v, _, _ := h.cursors.Load(context.Background(), "ifc-1", "events|sub-1"); v != "1" {
		t.Errorf("position after the failure %q", v)
	}
	if got := strings.Join(h.engine.order(), ","); got != "1,2" {
		t.Errorf("went on past the failure: %s", got)
	}
}

func TestParseNotification(t *testing.T) {
	n, err := parseNotification(notificationBundle("sub-9", topicMerged, "event-notification",
		[]testEvent{{7, "EID-1", []string{"EID-2"}, false}, {8, "EID-3", nil, true}}))
	if err != nil {
		t.Fatal(err)
	}
	if n.Subscription != "sub-9" || n.Topic != topicMerged || len(n.Events) != 2 || n.Events[0].Context[0] != "Patient/EID-2" {
		t.Fatalf("%+v", n)
	}
	if e := n.resource("Patient/EID-3"); !e.Erased || e.Resource != nil {
		t.Errorf("erased: %+v", e)
	}
	if e := n.resource("Patient/EID-2"); e.Erased || e.Resource == nil || e.ID != "EID-2" {
		t.Errorf("context: %+v", e)
	}
	for _, bad := range []string{`{}`, `{"resourceType":"Bundle","type":"searchset","entry":[{}]}`,
		`{"resourceType":"Bundle","type":"history","entry":[{"resource":{"resourceType":"Patient"}}]}`} {
		if _, err := parseNotification([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	if relativeReference("https://x.example/fhir/r4/Patient/EID-1") != "Patient/EID-1" || relativeReference("urn:uuid:abc") != "" {
		t.Error("relativeReference")
	}
}

func TestFHIRSubscriptionValidate(t *testing.T) {
	base := map[string]interface{}{"port": 9000, "fhir_base_url": "https://empi.example/fhir/r4", "callback_url": "https://ezhk.example/hook",
		"topics": []string{topicCreated}, "auth_type": "bearer_token", "bearer_token": "x"}
	for name, change := range map[string]map[string]interface{}{
		"no base":       {"fhir_base_url": ""},
		"no callback":   {"callback_url": ""},
		"no topics":     {"topics": []string{}},
		"bad content":   {"content": "everything"},
		"no credential": {"bearer_token": ""},
		"bad auth":      {"auth_type": "basic"},
		"oauth no url":  {"auth_type": "oauth2", "oauth2_client_id": "c", "oauth2_client_secret": "s"},
	} {
		cfg := map[string]interface{}{}
		for k, v := range base {
			cfg[k] = v
		}
		for k, v := range change {
			cfg[k] = v
		}
		raw, _ := json.Marshal(cfg)
		if err := NewFHIRSubscriptionInboundConnector().Initialize(raw); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	raw, _ := json.Marshal(base)
	if err := NewFHIRSubscriptionInboundConnector().Initialize(raw); err != nil {
		t.Errorf("valid config refused: %v", err)
	}
}
