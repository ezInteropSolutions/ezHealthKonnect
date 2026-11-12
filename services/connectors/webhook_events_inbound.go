// services/connectors/webhook_events_inbound.go
//
// Ordered Webhook Events receiver — accepts CloudEvents 1.0 (a single event,
// or a batch as application/cloudevents-batch+json) pushed by a sender that
// delivers strictly in order and waits for the reply before sending more:
// ezIdentitySense's webhooks, for one. Unlike http_rest_inbound, which
// answers 202 as soon as a request is queued in memory, this receiver
// answers only once every event in the request has been fully processed:
//
//  1. The request is authenticated (by default a Standard Webhooks
//     signature over the body).
//  2. Events at or below the saved cursor (the last sequence finished) are
//     skipped, so a sender's retry never processes an event twice.
//  3. The remaining events are emitted ONE AT A TIME, each as its own
//     message (visible in the message viewer like any other). The receiver
//     waits until the engine reports the message's outcome
//     (OutcomeAwareConnector) and, on success, saves the event's sequence
//     as the cursor (CursorAwareConnector) before emitting the next.
//  4. The reply is 200 once all are done. A failed event stops the batch
//     with 500; running out of the reply budget (reply_within_seconds, kept
//     under the sender's own timeout) stops it with 503 and Retry-After. In
//     both cases the sender retries the same batch later, and the cursor
//     makes it resume where processing stopped.
//
// Ordering therefore comes from the sender: it never sends the next batch
// before this one is acknowledged. Only one request is processed at a time;
// a retry arriving while an earlier attempt's last event is still running
// waits for it (within its own budget) rather than running beside it.
//
// Limitation, by design: if an event's outcome doesn't arrive within
// event_timeout_seconds, the batch stops without saving that event's
// sequence. The pipeline may still finish later, and the sender's retry
// then processes the event again.
//
// Configuration:
//
//	port                       int     Listen port (required)
//	endpoint_path              string  Path that accepts POSTs (default "/events")
//	authentication_type        string  "standard_webhooks" (default) | "bearer_token" | "api_key" | "basic_auth"; "none" is refused
//	webhook_secret             string  "whsec_..." secret from the sender (standard_webhooks)
//	webhook_tolerance_seconds  int     Accepted signing-time skew (default 300)
//	sequence_attribute         string  CloudEvents attribute holding the event's sequence (default "sequence")
//	reply_within_seconds       int     Answer within this many seconds (default 8; keep below the sender's timeout)
//	event_timeout_seconds      int     Longest wait for one event's outcome (default 300)
//	max_body_size_mb           int     Largest accepted request (default 5)
//	cursor_key                 string  Name of this receiver's cursor (default: the endpoint path)
//	event_types                []string Event types to process; others are acknowledged and skipped (default: all)
package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"
)

const (
	webhookEventsTypeName          = "webhook_events_inbound"
	webhookEventsDefaultPath       = "/events"
	webhookEventsDefaultSeqAttr    = "sequence"
	webhookEventsDefaultReply      = 8 * time.Second
	webhookEventsDefaultEventWait  = 5 * time.Minute
	webhookEventsDefaultMaxBodyMB  = 5
	webhookEventsCursorSaveTimeout = 5 * time.Second
)

// cloudEvent is one parsed event: the attributes the receiver needs, and the
// event exactly as received, which becomes the message content.
type cloudEvent struct {
	ID       string
	Type     string
	Source   string
	Subject  string
	Sequence int64
	Raw      json.RawMessage
}

// batchResult is how one request's processing ended.
type batchResult struct {
	processed int
	skipped   int
	err       error // a failed event; nil when all finished or time ran out
	partial   bool  // stopped because the reply budget ran out
}

// WebhookEventsInboundConnector implements the ordered webhook receiver.
type WebhookEventsInboundConnector struct {
	*BaseInboundConnector

	port         int
	endpointPath string
	auth         *InboundAuthenticator
	seqAttr      string
	replyBudget  time.Duration
	eventWait    time.Duration
	maxBody      int64
	cursorKey    string
	interfaceID  string
	eventTypes   map[string]bool // nil = every type

	mu          sync.RWMutex
	server      *http.Server
	messageChan chan<- *models.InboundMessage
	cursors     CursorStore

	// busy admits one request's processing at a time.
	busy chan struct{}

	waitersMu sync.Mutex
	waiters   map[string]chan models.MessageOutcome
}

// NewWebhookEventsInboundConnector creates the receiver.
func NewWebhookEventsInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           webhookEventsTypeName,
		DisplayName:        "Ordered Webhook Events (CloudEvents)",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":  false,
			"supports_auth":  true,
			"supports_batch": true,
		},
	}
	return &WebhookEventsInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
		endpointPath:         webhookEventsDefaultPath,
		seqAttr:              webhookEventsDefaultSeqAttr,
		replyBudget:          webhookEventsDefaultReply,
		eventWait:            webhookEventsDefaultEventWait,
		maxBody:              webhookEventsDefaultMaxBodyMB << 20,
		busy:                 make(chan struct{}, 1),
		waiters:              make(map[string]chan models.MessageOutcome),
	}
}

// Initialize parses configuration.
func (c *WebhookEventsInboundConnector) Initialize(config []byte) error {
	if err := c.BaseInboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := c.GetConfig()

	c.port = cfg.GetInt("port")
	if v := strings.TrimSpace(cfg.GetString("endpoint_path")); v != "" {
		c.endpointPath = v
	}
	if !strings.HasPrefix(c.endpointPath, "/") {
		c.endpointPath = "/" + c.endpointPath
	}
	if v := strings.TrimSpace(cfg.GetString("sequence_attribute")); v != "" {
		c.seqAttr = v
	}
	if v := cfg.GetInt("reply_within_seconds"); v > 0 {
		c.replyBudget = time.Duration(v) * time.Second
	}
	if v := cfg.GetInt("event_timeout_seconds"); v > 0 {
		c.eventWait = time.Duration(v) * time.Second
	}
	if v := cfg.GetInt("max_body_size_mb"); v > 0 {
		c.maxBody = int64(v) << 20
	}
	c.cursorKey = strings.TrimSpace(cfg.GetString("cursor_key"))
	if c.cursorKey == "" {
		c.cursorKey = c.endpointPath
	}
	c.interfaceID = cfg.GetString("interface_id")
	if types := cfg.GetStringSlice("event_types"); len(types) > 0 {
		c.eventTypes = make(map[string]bool, len(types))
		for _, t := range types {
			if t = strings.TrimSpace(t); t != "" {
				c.eventTypes[t] = true
			}
		}
	}

	auth, err := NewInboundAuthenticator(cfg.Config, InboundAuthStandardWebhooks)
	if err != nil {
		return err
	}
	c.auth = auth
	return c.Validate()
}

// Validate checks the configuration.
func (c *WebhookEventsInboundConnector) Validate() error {
	if c.port < 1 || c.port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if c.auth == nil {
		return fmt.Errorf("authentication is not configured")
	}
	if c.auth.Type() == InboundAuthNone {
		return fmt.Errorf("authentication_type 'none' is not permitted: this receiver acts on what it is sent")
	}
	return c.auth.Validate()
}

// TestConnection verifies the configured port is free.
func (c *WebhookEventsInboundConnector) TestConnection(ctx context.Context) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", c.port))
	if err != nil {
		return fmt.Errorf("port %d is not available: %w", c.port, err)
	}
	return ln.Close()
}

// SupportsCron returns false: the sender pushes.
func (c *WebhookEventsInboundConnector) SupportsCron() bool { return false }

// SetCursorStore implements CursorAwareConnector. Requests are refused with
// 503 until it has been called.
func (c *WebhookEventsInboundConnector) SetCursorStore(store CursorStore) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cursors = store
}

// OnMessageOutcome implements OutcomeAwareConnector.
func (c *WebhookEventsInboundConnector) OnMessageOutcome(o models.MessageOutcome) {
	c.waitersMu.Lock()
	ch, ok := c.waiters[o.MessageID]
	if ok {
		delete(c.waiters, o.MessageID)
	}
	c.waitersMu.Unlock()
	if ok {
		ch <- o // buffered, one slot per waiter: never blocks
	}
}

// Start binds the port (so a conflict is returned to the engine at once)
// and serves until the context ends.
func (c *WebhookEventsInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", c.port))
	if err != nil {
		return fmt.Errorf("listen on port %d: %w", c.port, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(c.endpointPath, c.handleRequest)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      c.replyBudget + 30*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	c.mu.Lock()
	c.messageChan = messageChan
	c.server = server
	c.mu.Unlock()
	c.SetState(StateRunning)
	c.SetConnected(true)

	go func() {
		log.Printf("🌐 Webhook Events Inbound: listening on :%d%s", c.port, c.endpointPath)
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("❌ Webhook Events Inbound: server error: %v", err)
			c.RecordError(err)
		}
	}()

	<-ctx.Done()
	return c.Stop()
}

// Stop shuts the server down.
func (c *WebhookEventsInboundConnector) Stop() error {
	c.mu.Lock()
	server := c.server
	c.server = nil
	c.mu.Unlock()
	if server == nil {
		return nil
	}
	c.SetState(StateStopped)
	c.SetConnected(false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

// GetStatus reports the receiver's state.
func (c *WebhookEventsInboundConnector) GetStatus() ConnectorStatus {
	st := c.BaseInboundConnector.GetStatus()
	if st.Metadata == nil {
		st.Metadata = map[string]string{}
	}
	st.Metadata["port"] = strconv.Itoa(c.port)
	st.Metadata["endpoint_path"] = c.endpointPath
	if c.auth != nil {
		st.Metadata["auth"] = c.auth.Type()
	}
	return st
}

// --------------------------------------------------------------------------
// Request handling
// --------------------------------------------------------------------------

func (c *WebhookEventsInboundConnector) handleRequest(w http.ResponseWriter, r *http.Request) {
	deadline := time.Now().Add(c.replyBudget)

	if r.Method != http.MethodPost {
		writeWebhookReply(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "only POST is accepted"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, c.maxBody+1))
	if err != nil {
		writeWebhookReply(w, http.StatusBadRequest, map[string]interface{}{"error": "the body could not be read"})
		return
	}
	if int64(len(body)) > c.maxBody {
		writeWebhookReply(w, http.StatusRequestEntityTooLarge, map[string]interface{}{"error": "the body is too large"})
		return
	}
	if !c.auth.AuthenticateRequest(r, body) {
		writeWebhookReply(w, http.StatusUnauthorized, map[string]interface{}{"error": "authentication failed"})
		return
	}

	events, err := parseCloudEvents(body, c.seqAttr)
	if err != nil {
		writeWebhookReply(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
		return
	}

	c.mu.RLock()
	cursors, messageChan := c.cursors, c.messageChan
	c.mu.RUnlock()
	if cursors == nil || messageChan == nil {
		w.Header().Set("Retry-After", "5")
		writeWebhookReply(w, http.StatusServiceUnavailable, map[string]interface{}{"error": "the receiver is starting"})
		return
	}

	// One request's processing at a time.
	select {
	case c.busy <- struct{}{}:
	case <-time.After(time.Until(deadline)):
		w.Header().Set("Retry-After", "1")
		writeWebhookReply(w, http.StatusServiceUnavailable, map[string]interface{}{"error": "an earlier delivery is still being processed"})
		return
	}

	done := make(chan batchResult, 1)
	go func() {
		defer func() { <-c.busy }()
		done <- c.processBatch(events, deadline, cursors, messageChan, sourceHost(r), r.URL.Path)
	}()

	var res batchResult
	select {
	case res = <-done:
	case <-time.After(time.Until(deadline) + 500*time.Millisecond):
		// The current event is still running; it keeps the busy slot and
		// saves the cursor if it succeeds. The sender's retry resumes there.
		res = batchResult{partial: true}
	}

	switch {
	case res.err != nil:
		writeWebhookReply(w, http.StatusInternalServerError, map[string]interface{}{
			"error": res.err.Error(), "processed": res.processed, "skipped": res.skipped,
		})
	case res.partial:
		w.Header().Set("Retry-After", "1")
		writeWebhookReply(w, http.StatusServiceUnavailable, map[string]interface{}{
			"error": "not all events were processed in time; resend to continue", "processed": res.processed, "skipped": res.skipped,
		})
	default:
		writeWebhookReply(w, http.StatusOK, map[string]interface{}{"processed": res.processed, "skipped": res.skipped})
	}
}

// processBatch runs the events in order. It never starts an event after the
// deadline, but always waits for the one it started.
func (c *WebhookEventsInboundConnector) processBatch(events []cloudEvent, deadline time.Time, cursors CursorStore,
	messageChan chan<- *models.InboundMessage, sourceIP, path string) batchResult {

	var res batchResult
	cursor, err := c.loadCursor(cursors)
	if err != nil {
		res.err = fmt.Errorf("the receiver's position could not be read")
		log.Printf("❌ Webhook Events Inbound: loading cursor %q for interface %s: %v", c.cursorKey, c.interfaceID, err)
		return res
	}

	for _, ev := range events {
		if ev.Sequence <= cursor {
			res.skipped++
			continue
		}
		if time.Now().After(deadline) {
			res.partial = true
			return res
		}

		// A type this interface doesn't handle is acknowledged (the cursor
		// moves past it) but never emitted.
		filtered := c.eventTypes != nil && !c.eventTypes[ev.Type]
		outcome := models.MessageOutcome{Status: models.OutcomeProcessed}
		var err error
		if !filtered {
			outcome, err = c.runEvent(ev, messageChan, sourceIP, path)
		}
		if err != nil {
			res.err = err
			return res
		}
		if !outcome.Succeeded() {
			res.err = fmt.Errorf("event %s (sequence %d) was not processed (%s); see message %s",
				ev.ID, ev.Sequence, outcome.Status, outcome.MessageID)
			return res
		}

		saveCtx, cancel := context.WithTimeout(context.Background(), webhookEventsCursorSaveTimeout)
		err = cursors.Save(saveCtx, c.interfaceID, c.cursorKey, strconv.FormatInt(ev.Sequence, 10))
		cancel()
		if err != nil {
			log.Printf("❌ Webhook Events Inbound: saving cursor %d for interface %s: %v", ev.Sequence, c.interfaceID, err)
			res.err = fmt.Errorf("event %s was processed but the receiver's position could not be saved", ev.ID)
			return res
		}
		cursor = ev.Sequence
		if filtered {
			res.skipped++
		} else {
			res.processed++
		}
	}
	return res
}

func (c *WebhookEventsInboundConnector) loadCursor(cursors CursorStore) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), webhookEventsCursorSaveTimeout)
	defer cancel()
	v, found, err := cursors.Load(ctx, c.interfaceID, c.cursorKey)
	if err != nil || !found {
		return 0, err
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("stored cursor %q is not a sequence number", v)
	}
	return n, nil
}

// runEvent emits one event as a message and waits for its outcome.
func (c *WebhookEventsInboundConnector) runEvent(ev cloudEvent, messageChan chan<- *models.InboundMessage,
	sourceIP, path string) (models.MessageOutcome, error) {

	msg := &models.InboundMessage{
		MessageID:      fmt.Sprintf("cloudevent_%d_%d", ev.Sequence, time.Now().UnixNano()),
		CorrelationID:  ev.ID,
		InterfaceID:    c.interfaceID,
		Content:        string(ev.Raw),
		ContentType:    "application/cloudevents+json",
		SourceType:     "webhook_events",
		SourceEndpoint: path,
		SourceIP:       sourceIP,
		ReceivedAt:     time.Now(),
		MessageType:    ev.Type,
		MessageSize:    len(ev.Raw),
		Headers: map[string]string{
			"Content-Type": "application/cloudevents+json",
			"ce-id":        ev.ID,
			"ce-type":      ev.Type,
			"ce-source":    ev.Source,
			"ce-sequence":  strconv.FormatInt(ev.Sequence, 10),
		},
	}

	wait := make(chan models.MessageOutcome, 1)
	c.waitersMu.Lock()
	c.waiters[msg.MessageID] = wait
	c.waitersMu.Unlock()
	forget := func() {
		c.waitersMu.Lock()
		delete(c.waiters, msg.MessageID)
		c.waitersMu.Unlock()
	}

	select {
	case messageChan <- msg:
		c.IncrementMessagesReceived()
	case <-time.After(c.eventWait):
		forget()
		return models.MessageOutcome{}, fmt.Errorf("the interface's queue did not accept event %s", ev.ID)
	}

	select {
	case o := <-wait:
		return o, nil
	case <-time.After(c.eventWait):
		forget()
		return models.MessageOutcome{}, fmt.Errorf("event %s (sequence %d) did not finish within %s; see message %s",
			ev.ID, ev.Sequence, c.eventWait, msg.MessageID)
	}
}

// parseCloudEvents reads a single event or a batch (a JSON array), checks
// each has the attributes the receiver relies on, and returns them in
// sequence order. A request whose events aren't in ascending sequence is
// refused: an in-order sender never produces one.
func parseCloudEvents(body []byte, seqAttr string) ([]cloudEvent, error) {
	trimmed := strings.TrimSpace(string(body))
	var raws []json.RawMessage
	switch {
	case strings.HasPrefix(trimmed, "["):
		if err := json.Unmarshal([]byte(trimmed), &raws); err != nil {
			return nil, fmt.Errorf("the batch is not a JSON array of events")
		}
	case strings.HasPrefix(trimmed, "{"):
		raws = []json.RawMessage{json.RawMessage(trimmed)}
	default:
		return nil, fmt.Errorf("the body is not a CloudEvent or a batch of them")
	}
	if len(raws) == 0 {
		return nil, fmt.Errorf("the batch is empty")
	}

	events := make([]cloudEvent, 0, len(raws))
	var last int64 = -1
	for i, raw := range raws {
		var attrs map[string]interface{}
		if err := json.Unmarshal(raw, &attrs); err != nil {
			return nil, fmt.Errorf("event %d is not a JSON object", i+1)
		}
		str := func(k string) string { v, _ := attrs[k].(string); return v }
		if str("specversion") != "1.0" {
			return nil, fmt.Errorf("event %d is not CloudEvents 1.0", i+1)
		}
		ev := cloudEvent{ID: str("id"), Type: str("type"), Source: str("source"), Subject: str("subject"), Raw: raw}
		if ev.ID == "" || ev.Type == "" {
			return nil, fmt.Errorf("event %d has no id or type", i+1)
		}
		seq, err := sequenceOf(attrs[seqAttr])
		if err != nil {
			return nil, fmt.Errorf("event %s: %s %v", ev.ID, seqAttr, err)
		}
		if seq <= last {
			return nil, fmt.Errorf("events are not in ascending %s order", seqAttr)
		}
		last = seq
		ev.Sequence = seq
		events = append(events, ev)
	}
	return events, nil
}

// sequenceOf reads a sequence attribute sent as a string or a number.
func sequenceOf(v interface{}) (int64, error) {
	switch s := v.(type) {
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil || n < 0 {
			return 0, errors.New("is not a non-negative integer")
		}
		return n, nil
	case float64:
		if s < 0 || s != float64(int64(s)) {
			return 0, errors.New("is not a non-negative integer")
		}
		return int64(s), nil
	case nil:
		return 0, errors.New("is missing")
	default:
		return 0, errors.New("is not a number")
	}
}

func sourceHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeWebhookReply(w http.ResponseWriter, status int, body map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
