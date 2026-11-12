// services/connectors/webhook_events_inbound_test.go
// Tests for the Standard Webhooks verifier, the standard_webhooks inbound
// auth type, and the ordered webhook events receiver.
//
//   TC-SW-001..006   Standard Webhooks verification (good, rotation, tamper,
//                    stale/future timestamp, missing headers, bad secret)
//   TC-SW-007..009   InboundAuthenticator standard_webhooks: body required,
//                    Validate, header-only Authenticate refuses
//   TC-WE-001..004   parseCloudEvents: single, batch, string/number
//                    sequences, malformed and out-of-order refused
//   TC-WE-005..013   Receiver over a real listener with a fake engine:
//                    in-order processing, reply after outcomes, cursor
//                    saved per event, retry skips finished events, failure
//                    stops the batch (500), budget exhaustion (503), bad
//                    signature (401), not-ready (503), one request at a time
package connectors

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

// testWebhookSecret is a synthetic 32-byte key.
var testWebhookSecret = "whsec_" + base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

func signStandardWebhook(t *testing.T, secret, id string, at time.Time, body []byte) string {
	t.Helper()
	key, err := parseStandardWebhookSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + strconv.FormatInt(at.Unix(), 10) + "."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func signedHeaders(t *testing.T, secret string, at time.Time, body []byte) http.Header {
	h := http.Header{}
	h.Set(StandardWebhookIDHeader, "evt_1_2")
	h.Set(StandardWebhookTimestampHeader, strconv.FormatInt(at.Unix(), 10))
	h.Set(StandardWebhookSignatureHeader, signStandardWebhook(t, secret, "evt_1_2", at, body))
	return h
}

func TestStandardWebhookVerify(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	v, err := newStandardWebhookVerifier(testWebhookSecret, 0)
	if err != nil {
		t.Fatal(err)
	}
	v.now = func() time.Time { return now }
	body := []byte(`[{"id":"1"}]`)

	if err := v.Verify(signedHeaders(t, testWebhookSecret, now, body), body); err != nil {
		t.Fatalf("good signature refused: %v", err)
	}

	// Rotation: the sender signs with old and new; either alone suffices.
	other := "whsec_" + base64.StdEncoding.EncodeToString([]byte("ffffffffffffffffffffffffffffffff"))
	h := signedHeaders(t, testWebhookSecret, now, body)
	h.Set(StandardWebhookSignatureHeader, signStandardWebhook(t, other, "evt_1_2", now, body)+" "+h.Get(StandardWebhookSignatureHeader))
	if err := v.Verify(h, body); err != nil {
		t.Fatalf("rotation pair refused: %v", err)
	}

	if err := v.Verify(signedHeaders(t, testWebhookSecret, now, body), []byte(`[{"id":"2"}]`)); err == nil {
		t.Fatal("tampered body accepted")
	}
	if err := v.Verify(signedHeaders(t, testWebhookSecret, now.Add(-6*time.Minute), body), body); err == nil {
		t.Fatal("stale timestamp accepted")
	}
	if err := v.Verify(signedHeaders(t, testWebhookSecret, now.Add(6*time.Minute), body), body); err == nil {
		t.Fatal("future timestamp accepted")
	}
	if err := v.Verify(http.Header{}, body); err == nil {
		t.Fatal("missing headers accepted")
	}
	h = signedHeaders(t, testWebhookSecret, now, body)
	h.Set(StandardWebhookSignatureHeader, "v2,"+strings.TrimPrefix(h.Get(StandardWebhookSignatureHeader), "v1,"))
	if err := v.Verify(h, body); err == nil {
		t.Fatal("unknown signature version accepted")
	}
}

func TestParseStandardWebhookSecret(t *testing.T) {
	for _, bad := range []string{"", "whsec_not base64!", "whsec_" + base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, err := parseStandardWebhookSecret(bad); err == nil {
			t.Errorf("secret %q accepted", bad)
		}
	}
	if _, err := parseStandardWebhookSecret(testWebhookSecret); err != nil {
		t.Fatalf("good secret refused: %v", err)
	}
}

func TestInboundAuthenticatorStandardWebhooks(t *testing.T) {
	a, err := NewInboundAuthenticator(map[string]interface{}{
		"authentication_type": "standard_webhooks", "webhook_secret": testWebhookSecret,
	}, InboundAuthAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !a.RequiresBody() {
		t.Fatal("standard_webhooks must require the body")
	}
	body := []byte(`{"x":1}`)
	r := &http.Request{Header: signedHeaders(t, testWebhookSecret, time.Now(), body)}
	if !a.AuthenticateRequest(r, body) {
		t.Fatal("signed request refused")
	}
	if a.Authenticate(r) {
		t.Fatal("header-only Authenticate accepted a signed-body type")
	}
	if a.AuthenticateRequest(r, []byte(`{"x":2}`)) {
		t.Fatal("other body accepted")
	}

	noSecret, err := NewInboundAuthenticator(map[string]interface{}{"authentication_type": "standard_webhooks"}, InboundAuthAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	if noSecret.Validate() == nil {
		t.Fatal("missing secret passed Validate")
	}
	if noSecret.AuthenticateRequest(r, body) {
		t.Fatal("authenticator without a secret accepted a request")
	}

	// Header types are unaffected by the body.
	k, _ := NewInboundAuthenticator(map[string]interface{}{"authentication_type": "api_key", "api_key": "k"}, InboundAuthAPIKey)
	if k.RequiresBody() || !k.AuthenticateRequest(newAuthReq(map[string]string{"X-API-Key": "k"}), nil) {
		t.Fatal("api_key behaviour changed")
	}
}

func TestParseCloudEvents(t *testing.T) {
	single := `{"specversion":"1.0","id":"a","type":"identity.created","source":"s","sequence":"7"}`
	evs, err := parseCloudEvents([]byte(single), "sequence")
	if err != nil || len(evs) != 1 || evs[0].Sequence != 7 || string(evs[0].Raw) != single {
		t.Fatalf("single: %v %+v", err, evs)
	}

	batch := `[{"specversion":"1.0","id":"a","type":"t","sequence":"1"},{"specversion":"1.0","id":"b","type":"t","sequence":2}]`
	evs, err = parseCloudEvents([]byte(batch), "sequence")
	if err != nil || len(evs) != 2 || evs[1].Sequence != 2 {
		t.Fatalf("batch: %v %+v", err, evs)
	}

	bad := map[string]string{
		"not json":       `nope`,
		"empty batch":    `[]`,
		"wrong version":  `{"specversion":"0.3","id":"a","type":"t","sequence":"1"}`,
		"no id":          `{"specversion":"1.0","type":"t","sequence":"1"}`,
		"no sequence":    `{"specversion":"1.0","id":"a","type":"t"}`,
		"negative":       `{"specversion":"1.0","id":"a","type":"t","sequence":"-1"}`,
		"fraction":       `{"specversion":"1.0","id":"a","type":"t","sequence":1.5}`,
		"out of order":   `[{"specversion":"1.0","id":"a","type":"t","sequence":"2"},{"specversion":"1.0","id":"b","type":"t","sequence":"1"}]`,
		"repeated seq":   `[{"specversion":"1.0","id":"a","type":"t","sequence":"2"},{"specversion":"1.0","id":"b","type":"t","sequence":"2"}]`,
		"array of junk":  `[1,2]`,
	}
	for name, body := range bad {
		if _, err := parseCloudEvents([]byte(body), "sequence"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// ── Receiver over a real listener ─────────────────────────────────────────

type memCursorStore struct {
	mu     sync.Mutex
	values map[string]string
	saves  []string
	failOn string // a value whose Save fails
}

func (m *memCursorStore) Load(_ context.Context, ifc, key string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.values[ifc+"|"+key]
	return v, ok, nil
}

func (m *memCursorStore) Save(_ context.Context, ifc, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if value == m.failOn {
		return fmt.Errorf("disk full")
	}
	if m.values == nil {
		m.values = map[string]string{}
	}
	m.values[ifc+"|"+key] = value
	m.saves = append(m.saves, value)
	return nil
}

func (m *memCursorStore) get() (string, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.values["ifc-1|/events"], append([]string(nil), m.saves...)
}

// fakeEngine reads emitted messages and reports outcomes after a delay,
// recording the order it saw them in. failTypes fail.
type fakeEngine struct {
	mu        sync.Mutex
	seen      []string // ce-sequence values, in emission order
	inFlight  int
	maxFlight int
	delay     time.Duration
	failSeq   string
}

func (f *fakeEngine) run(c *WebhookEventsInboundConnector, ch <-chan *models.InboundMessage) {
	for msg := range ch {
		f.mu.Lock()
		f.seen = append(f.seen, msg.Headers["ce-sequence"])
		f.inFlight++
		if f.inFlight > f.maxFlight {
			f.maxFlight = f.inFlight
		}
		delay, failSeq := f.delay, f.failSeq
		f.mu.Unlock()

		go func(m *models.InboundMessage) {
			time.Sleep(delay)
			status := models.OutcomeProcessed
			if m.Headers["ce-sequence"] == failSeq {
				status = models.OutcomeFailed
			}
			f.mu.Lock()
			f.inFlight--
			f.mu.Unlock()
			c.OnMessageOutcome(models.MessageOutcome{InterfaceID: "ifc-1", MessageID: m.MessageID, Status: status})
		}(msg)
	}
}

func (f *fakeEngine) order() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.seen...)
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

type receiverHarness struct {
	c       *WebhookEventsInboundConnector
	url     string
	cursors *memCursorStore
	engine  *fakeEngine
	cancel  context.CancelFunc
}

func startReceiver(t *testing.T, extra map[string]interface{}, withCursor bool) *receiverHarness {
	t.Helper()
	port := freePort(t)
	cfg := map[string]interface{}{
		"port": port, "webhook_secret": testWebhookSecret, "interface_id": "ifc-1",
		"reply_within_seconds": 3, "event_timeout_seconds": 2,
	}
	for k, v := range extra {
		cfg[k] = v
	}
	raw, _ := json.Marshal(cfg)
	c := NewWebhookEventsInboundConnector().(*WebhookEventsInboundConnector)
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	h := &receiverHarness{c: c, url: fmt.Sprintf("http://127.0.0.1:%d/events", port), cursors: &memCursorStore{}, engine: &fakeEngine{}}
	if withCursor {
		c.SetCursorStore(h.cursors)
	}
	ch := make(chan *models.InboundMessage, 100)
	go h.engine.run(c, ch)
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	errCh := make(chan error, 1)
	go func() { errCh <- c.Start(ctx, ch) }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err == nil {
			conn.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Cleanup(func() { cancel(); close(ch) })
	return h
}

func batchBody(seqs ...int) []byte {
	var parts []string
	for _, s := range seqs {
		parts = append(parts, fmt.Sprintf(`{"specversion":"1.0","id":"evt-%d","type":"identity.created","source":"ezis/test","subject":"EID-%d","sequence":"%d","data":{"eid":"EID-%d"}}`, s, s, s, s))
	}
	return []byte("[" + strings.Join(parts, ",") + "]")
}

func (h *receiverHarness) post(t *testing.T, body []byte, secret string) (int, map[string]interface{}) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, h.url, bytes.NewReader(body))
	req.Header = signedHeaders(t, secret, time.Now(), body)
	req.Header.Set("Content-Type", "application/cloudevents-batch+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestReceiverProcessesInOrderAndRepliesAfterOutcomes(t *testing.T) {
	h := startReceiver(t, nil, true)
	h.engine.delay = 30 * time.Millisecond

	status, out := h.post(t, batchBody(1, 2, 3, 4), testWebhookSecret)
	if status != http.StatusOK || out["processed"] != float64(4) {
		t.Fatalf("status %d body %v", status, out)
	}
	if got := strings.Join(h.engine.order(), ","); got != "1,2,3,4" {
		t.Fatalf("emission order %s", got)
	}
	if h.engine.maxFlight != 1 {
		t.Fatalf("%d events in flight at once, want 1", h.engine.maxFlight)
	}
	cur, saves := h.cursors.get()
	if cur != "4" || strings.Join(saves, ",") != "1,2,3,4" {
		t.Fatalf("cursor %s saves %v", cur, saves)
	}

	// A retry of the same batch, plus one new event: only the new one runs.
	status, out = h.post(t, batchBody(3, 4, 5), testWebhookSecret)
	if status != http.StatusOK || out["processed"] != float64(1) || out["skipped"] != float64(2) {
		t.Fatalf("retry: status %d body %v", status, out)
	}
	if got := strings.Join(h.engine.order(), ","); got != "1,2,3,4,5" {
		t.Fatalf("retry re-ran events: %s", got)
	}
}

func TestReceiverStopsAtFailedEvent(t *testing.T) {
	h := startReceiver(t, nil, true)
	h.engine.failSeq = "2"

	status, out := h.post(t, batchBody(1, 2, 3), testWebhookSecret)
	if status != http.StatusInternalServerError || out["processed"] != float64(1) {
		t.Fatalf("status %d body %v", status, out)
	}
	if got := strings.Join(h.engine.order(), ","); got != "1,2" {
		t.Fatalf("events after the failure ran: %s", got)
	}
	if cur, _ := h.cursors.get(); cur != "1" {
		t.Fatalf("cursor %s, want 1 (the failed event must not advance it)", cur)
	}

	// Once the cause is fixed, the sender's retry resumes at the failed event.
	h.engine.mu.Lock()
	h.engine.failSeq = ""
	h.engine.mu.Unlock()
	status, out = h.post(t, batchBody(1, 2, 3), testWebhookSecret)
	if status != http.StatusOK || out["processed"] != float64(2) {
		t.Fatalf("retry: status %d body %v", status, out)
	}
}

func TestReceiverReplyBudget(t *testing.T) {
	h := startReceiver(t, map[string]interface{}{"reply_within_seconds": 1, "event_timeout_seconds": 5}, true)
	h.engine.delay = 400 * time.Millisecond

	start := time.Now()
	status, out := h.post(t, batchBody(1, 2, 3, 4, 5, 6), testWebhookSecret)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status %d body %v, want 503", status, out)
	}
	if took := time.Since(start); took > 2500*time.Millisecond {
		t.Fatalf("reply took %s, beyond the budget", took)
	}
	// Let the in-flight event finish; its sequence is saved.
	time.Sleep(600 * time.Millisecond)
	cur, _ := h.cursors.get()
	done, _ := strconv.Atoi(cur)
	if done < 2 || done > 4 {
		t.Fatalf("cursor %s after a 1 s budget at 0.4 s per event", cur)
	}

	h.engine.mu.Lock()
	h.engine.delay = 10 * time.Millisecond
	h.engine.mu.Unlock()
	status, _ = h.post(t, batchBody(1, 2, 3, 4, 5, 6), testWebhookSecret)
	if status != http.StatusOK {
		t.Fatalf("resume status %d", status)
	}
	seen := h.engine.order()
	if strings.Join(seen, ",") != "1,2,3,4,5,6" {
		t.Fatalf("events repeated or skipped across the resume: %v", seen)
	}
}

func TestReceiverRefusesBadSignatureAndNotReady(t *testing.T) {
	h := startReceiver(t, nil, true)
	other := "whsec_" + base64.StdEncoding.EncodeToString([]byte("ffffffffffffffffffffffffffffffff"))
	if status, _ := h.post(t, batchBody(1), other); status != http.StatusUnauthorized {
		t.Fatalf("wrong secret: status %d", status)
	}
	if len(h.engine.order()) != 0 {
		t.Fatal("an unauthenticated event was emitted")
	}

	notReady := startReceiver(t, nil, false)
	if status, _ := notReady.post(t, batchBody(1), testWebhookSecret); status != http.StatusServiceUnavailable {
		t.Fatalf("no cursor store: status %d, want 503", status)
	}
}

func TestReceiverRefusesMalformedBatch(t *testing.T) {
	h := startReceiver(t, nil, true)
	if status, _ := h.post(t, []byte(`{"hello":"world"}`), testWebhookSecret); status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", status)
	}
}

func TestReceiverCursorSaveFailureStops(t *testing.T) {
	h := startReceiver(t, nil, true)
	h.cursors.failOn = "2"
	status, out := h.post(t, batchBody(1, 2, 3), testWebhookSecret)
	if status != http.StatusInternalServerError || out["processed"] != float64(1) {
		t.Fatalf("status %d body %v", status, out)
	}
	if got := strings.Join(h.engine.order(), ","); got != "1,2" {
		t.Fatalf("processing continued past an unsaved position: %s", got)
	}
}

func TestReceiverOneRequestAtATime(t *testing.T) {
	h := startReceiver(t, nil, true)
	h.engine.delay = 200 * time.Millisecond

	var wg sync.WaitGroup
	statuses := make([]int, 2)
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			statuses[i], _ = h.post(t, batchBody(1, 2), testWebhookSecret)
		}(i)
	}
	wg.Wait()
	if h.engine.maxFlight != 1 {
		t.Fatalf("%d events in flight across concurrent requests, want 1", h.engine.maxFlight)
	}
	if got := strings.Join(h.engine.order(), ","); got != "1,2" {
		t.Fatalf("concurrent duplicate deliveries ran events twice: %s", got)
	}
	for _, s := range statuses {
		if s != http.StatusOK {
			t.Fatalf("statuses %v", statuses)
		}
	}
}

func TestReceiverValidateRefusesNoAuth(t *testing.T) {
	raw, _ := json.Marshal(map[string]interface{}{"port": 9999, "authentication_type": "none"})
	c := NewWebhookEventsInboundConnector()
	if err := c.Initialize(raw); err == nil {
		t.Fatal("authentication_type none accepted")
	}
	raw, _ = json.Marshal(map[string]interface{}{"port": 9999})
	if err := NewWebhookEventsInboundConnector().Initialize(raw); err == nil {
		t.Fatal("missing webhook secret accepted")
	}
}

func TestReceiverEventTypeFilter(t *testing.T) {
	h := startReceiver(t, map[string]interface{}{"event_types": []interface{}{"identity.created"}}, true)
	body := []byte(`[{"specversion":"1.0","id":"a","type":"task.created","sequence":"1"},` +
		`{"specversion":"1.0","id":"b","type":"identity.created","sequence":"2"},` +
		`{"specversion":"1.0","id":"c","type":"task.resolved","sequence":"3"}]`)
	status, out := h.post(t, body, testWebhookSecret)
	if status != http.StatusOK || out["processed"] != float64(1) || out["skipped"] != float64(2) {
		t.Fatalf("status %d body %v", status, out)
	}
	if got := strings.Join(h.engine.order(), ","); got != "2" {
		t.Fatalf("emitted %s, want only sequence 2", got)
	}
	if cur, _ := h.cursors.get(); cur != "3" {
		t.Fatalf("cursor %s, want 3 (filtered events are acknowledged)", cur)
	}
}
