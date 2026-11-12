// services/connectors/fhir_subscription_inbound.go
//
// FHIR Subscription receiver: subscribes to topics on a FHIR server that
// implements HL7 Subscriptions R5 Backport (topic-based subscriptions on
// FHIR R4; ezIdentitySense, for one) and processes the notifications it is
// sent, in order, one event at a time.
//
//  1. Subscriptions are managed here, not by hand: on start, and again every
//     renew_hours, one Subscription per topic is created on the server (or
//     brought up to date), naming callback_url as its rest-hook endpoint.
//     Each carries an Authorization header the server sends back with every
//     notification. Its value is derived from the credential already held
//     for the server (an HMAC of the interface and topic under the client
//     secret or bearer token), so there is no further secret to keep, and
//     it changes when that credential is rotated.
//  2. A notification is accepted only with the header of its own topic.
//     Handshakes and heartbeats are answered at once.
//  3. Event notifications carry numbered events. Events at or below the
//     saved position (per subscription) are skipped, so a retry never
//     processes one twice; a gap (the first number beyond the next expected)
//     is filled first from the server's $events.
//  4. Each event becomes one message (JSON: the topic, its number and time,
//     and the Patients it's about, with their resources when the
//     subscription carries them). The receiver waits for each message's
//     outcome (OutcomeAwareConnector), saves the position on success
//     (CursorAwareConnector), and only then takes the next.
//  5. The reply is 200 once every event is done; a failed one stops with
//     500, running out of time with 503: the server retries the same
//     notification later and processing resumes where it stopped.
//
// Configuration:
//
//	port                  int      Listen port (required)
//	endpoint_path         string   Path notifications are POSTed to (default "/fhir-notifications")
//	callback_url          string   The address the server reaches this endpoint at (required)
//	fhir_base_url         string   The server's FHIR base, e.g. https://empi.example.org/fhir/r4 (required)
//	auth_type             string   "oauth2" (client credentials, default) | "bearer_token"
//	oauth2_token_url, oauth2_client_id, oauth2_client_secret, oauth2_scope, oauth2_audience
//	bearer_token          string   For auth_type bearer_token
//	topics                []string Topic URLs to subscribe to (required)
//	filters               []string Filter criteria applied to every subscription (name=value)
//	content               string   "empty" | "id-only" | "full-resource" (default)
//	heartbeat_seconds     int      Ask for a heartbeat after this long without events (0: none)
//	renew_hours           int      Bring the subscriptions up to date this often (default 12)
//	reply_within_seconds  int      Answer within this many seconds (default 8)
//	event_timeout_seconds int      Longest wait for one event's outcome (default 300)
//	max_body_size_mb      int      Largest accepted notification (default 10)
package connectors

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"
	httpservice "ezhealthkonnect/services/http"
)

const (
	fhirSubTypeName       = "fhir_subscription_inbound"
	fhirSubDefaultPath    = "/fhir-notifications"
	fhirSubDefaultReply   = 8 * time.Second
	fhirSubDefaultWait    = 5 * time.Minute
	fhirSubDefaultMaxBody = 10
	fhirSubDefaultRenew   = 12 * time.Hour
	fhirSubCursorTimeout  = 5 * time.Second

	backportBase       = "http://hl7.org/fhir/uv/subscriptions-backport/StructureDefinition/"
	backportProfile    = backportBase + "backport-subscription"
	backportFilter     = backportBase + "backport-filter-criteria"
	backportContent    = backportBase + "backport-payload-content"
	backportHeartbeat  = backportBase + "backport-heartbeat-period"
	fhirSubContentType = "application/fhir+json"
)

// FHIRSubscriptionInboundConnector implements the topic subscription receiver.
type FHIRSubscriptionInboundConnector struct {
	*BaseInboundConnector

	port         int
	endpointPath string
	callbackURL  string
	base         string
	topics       []string
	filters      []string
	content      string
	heartbeat    int
	renewEvery   time.Duration
	replyBudget  time.Duration
	eventWait    time.Duration
	maxBody      int64
	interfaceID  string
	auth         *httpservice.AuthConfig
	keyMaterial  string // the credential the header tokens are derived from
	http         *httpservice.HTTPClientService

	mu          sync.RWMutex
	server      *http.Server
	messageChan chan<- *models.InboundMessage
	cursors     CursorStore
	stop        context.CancelFunc

	busy chan struct{}

	waitersMu sync.Mutex
	waiters   map[string]chan models.MessageOutcome
}

// NewFHIRSubscriptionInboundConnector creates the receiver.
func NewFHIRSubscriptionInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           fhirSubTypeName,
		DisplayName:        "FHIR Subscription (topic-based)",
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
	return &FHIRSubscriptionInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
		endpointPath:         fhirSubDefaultPath,
		content:              "full-resource",
		renewEvery:           fhirSubDefaultRenew,
		replyBudget:          fhirSubDefaultReply,
		eventWait:            fhirSubDefaultWait,
		maxBody:              fhirSubDefaultMaxBody << 20,
		http:                 httpservice.NewHTTPClientService(30 * time.Second),
		busy:                 make(chan struct{}, 1),
		waiters:              make(map[string]chan models.MessageOutcome),
	}
}

// Initialize parses configuration.
func (c *FHIRSubscriptionInboundConnector) Initialize(config []byte) error {
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
	c.callbackURL = strings.TrimSpace(cfg.GetString("callback_url"))
	c.base = strings.TrimRight(strings.TrimSpace(cfg.GetString("fhir_base_url")), "/")
	for _, t := range cfg.GetStringSlice("topics") {
		if t = strings.TrimSpace(t); t != "" {
			c.topics = append(c.topics, t)
		}
	}
	for _, f := range cfg.GetStringSlice("filters") {
		if f = strings.TrimSpace(f); f != "" {
			c.filters = append(c.filters, f)
		}
	}
	if v := strings.TrimSpace(cfg.GetString("content")); v != "" {
		c.content = v
	}
	c.heartbeat = cfg.GetInt("heartbeat_seconds")
	if v := cfg.GetInt("renew_hours"); v > 0 {
		c.renewEvery = time.Duration(v) * time.Hour
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
	c.interfaceID = cfg.GetString("interface_id")

	switch at := strings.TrimSpace(cfg.GetString("auth_type")); at {
	case "", "oauth2":
		c.auth = &httpservice.AuthConfig{Type: httpservice.AuthTypeOAuth2, OAuth2Config: &httpservice.OAuth2Config{
			TokenURL: cfg.GetString("oauth2_token_url"), ClientID: cfg.GetString("oauth2_client_id"),
			ClientSecret: cfg.GetString("oauth2_client_secret"), GrantType: httpservice.OAuth2GrantType("client_credentials"),
			Scope: cfg.GetString("oauth2_scope"), Audience: cfg.GetString("oauth2_audience")}}
		c.keyMaterial = c.auth.OAuth2Config.ClientSecret
	case "bearer_token":
		c.auth = &httpservice.AuthConfig{Type: httpservice.AuthTypeBearer, BearerToken: cfg.GetString("bearer_token")}
		c.keyMaterial = c.auth.BearerToken
	default:
		return fmt.Errorf("auth_type %q is not oauth2 or bearer_token", at)
	}
	return c.Validate()
}

// Validate checks the configuration.
func (c *FHIRSubscriptionInboundConnector) Validate() error {
	switch {
	case c.port < 1 || c.port > 65535:
		return fmt.Errorf("port must be between 1 and 65535")
	case c.base == "":
		return fmt.Errorf("fhir_base_url is required: the server to subscribe on")
	case c.callbackURL == "":
		return fmt.Errorf("callback_url is required: the address the server sends notifications to")
	case len(c.topics) == 0:
		return fmt.Errorf("topics is required: at least one topic URL to subscribe to")
	case c.content != "empty" && c.content != "id-only" && c.content != "full-resource":
		return fmt.Errorf("content is empty, id-only or full-resource")
	case c.keyMaterial == "":
		return fmt.Errorf("the server's credential (client secret or bearer token) is required")
	case c.auth.Type == httpservice.AuthTypeOAuth2 && (c.auth.OAuth2Config.TokenURL == "" || c.auth.OAuth2Config.ClientID == ""):
		return fmt.Errorf("oauth2_token_url and oauth2_client_id are required")
	}
	for _, u := range []string{c.base, c.callbackURL} {
		if p, err := url.Parse(u); err != nil || (p.Scheme != "https" && p.Scheme != "http") || p.Host == "" {
			return fmt.Errorf("%q is not an http(s) address", u)
		}
	}
	return nil
}

// TestConnection verifies the configured port is free.
func (c *FHIRSubscriptionInboundConnector) TestConnection(ctx context.Context) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", c.port))
	if err != nil {
		return fmt.Errorf("port %d is not available: %w", c.port, err)
	}
	return ln.Close()
}

// SupportsCron returns false: the server pushes.
func (c *FHIRSubscriptionInboundConnector) SupportsCron() bool { return false }

// SetCursorStore implements CursorAwareConnector.
func (c *FHIRSubscriptionInboundConnector) SetCursorStore(store CursorStore) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cursors = store
}

// OnMessageOutcome implements OutcomeAwareConnector.
func (c *FHIRSubscriptionInboundConnector) OnMessageOutcome(o models.MessageOutcome) {
	c.waitersMu.Lock()
	ch, ok := c.waiters[o.MessageID]
	if ok {
		delete(c.waiters, o.MessageID)
	}
	c.waitersMu.Unlock()
	if ok {
		ch <- o
	}
}

// headerToken is the Authorization value a topic's notifications carry.
func (c *FHIRSubscriptionInboundConnector) headerToken(topic string) string {
	mac := hmac.New(sha256.New, []byte(c.keyMaterial))
	mac.Write([]byte("ezhealthkonnect-fhir-subscription|" + c.interfaceID + "|" + topic))
	return hex.EncodeToString(mac.Sum(nil))
}

// topicOfRequest returns the topic whose header the request carries.
func (c *FHIRSubscriptionInboundConnector) topicOfRequest(r *http.Request) (string, bool) {
	got := strings.TrimSpace(r.Header.Get("Authorization"))
	for _, t := range c.topics {
		want := "Bearer " + c.headerToken(t)
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
			return t, true
		}
	}
	return "", false
}

// Start binds the port, then subscribes (the endpoint must be up for the
// handshake), and serves until the context ends.
func (c *FHIRSubscriptionInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
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
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: c.replyBudget + 30*time.Second, IdleTimeout: 60 * time.Second}

	runCtx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.messageChan, c.server, c.stop = messageChan, server, cancel
	c.mu.Unlock()
	c.SetState(StateRunning)
	c.SetConnected(true)

	go func() {
		log.Printf("🌐 FHIR Subscription Inbound: listening on :%d%s", c.port, c.endpointPath)
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("❌ FHIR Subscription Inbound: server error: %v", err)
			c.RecordError(err)
		}
	}()
	go c.keepSubscribed(runCtx)

	<-ctx.Done()
	return c.Stop()
}

// Stop shuts the server down. The subscriptions stay on the server: they
// resume where they stopped when the interface starts again.
func (c *FHIRSubscriptionInboundConnector) Stop() error {
	c.mu.Lock()
	server, stop := c.server, c.stop
	c.server, c.stop = nil, nil
	c.mu.Unlock()
	if stop != nil {
		stop()
	}
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
func (c *FHIRSubscriptionInboundConnector) GetStatus() ConnectorStatus {
	st := c.BaseInboundConnector.GetStatus()
	if st.Metadata == nil {
		st.Metadata = map[string]string{}
	}
	st.Metadata["port"] = strconv.Itoa(c.port)
	st.Metadata["endpoint_path"] = c.endpointPath
	st.Metadata["topics"] = strconv.Itoa(len(c.topics))
	return st
}

// --------------------------------------------------------------------------
// Managing the subscriptions
// --------------------------------------------------------------------------

// keepSubscribed brings the subscriptions up to date now and every
// renewEvery (the server ends a subscription after a while unless it's
// renewed), retrying a failure after a minute.
func (c *FHIRSubscriptionInboundConnector) keepSubscribed(ctx context.Context) {
	for {
		wait := c.renewEvery
		if err := c.ensureSubscriptions(ctx); err != nil {
			log.Printf("⚠️ FHIR Subscription Inbound: interface %s: %v (retrying in a minute)", c.interfaceID, err)
			c.RecordError(err)
			wait = time.Minute
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// ensureSubscriptions creates each topic's subscription, or updates the one
// made before (its id is kept as a cursor).
func (c *FHIRSubscriptionInboundConnector) ensureSubscriptions(ctx context.Context) error {
	c.mu.RLock()
	cursors := c.cursors
	c.mu.RUnlock()
	if cursors == nil {
		return errors.New("the receiver's store isn't ready yet")
	}
	for _, topic := range c.topics {
		key := "subscription|" + topic
		lctx, cancel := context.WithTimeout(ctx, fhirSubCursorTimeout)
		id, found, err := cursors.Load(lctx, c.interfaceID, key)
		cancel()
		if err != nil {
			return fmt.Errorf("reading the subscription for %s: %w", topic, err)
		}
		body := c.subscriptionBody(topic, id)
		if found && id != "" {
			resp, err := c.call(ctx, http.MethodPut, c.base+"/Subscription/"+url.PathEscape(id), body, 404, 410)
			if err != nil {
				return fmt.Errorf("updating the subscription to %s: %w", topic, err)
			}
			if resp.StatusCode < 300 {
				continue
			}
			// Gone from the server: make it again.
			body = c.subscriptionBody(topic, "")
		}
		resp, err := c.call(ctx, http.MethodPost, c.base+"/Subscription", body)
		if err != nil {
			return fmt.Errorf("subscribing to %s: %w", topic, err)
		}
		var made struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(resp.Body, &made); err != nil || made.ID == "" {
			return fmt.Errorf("subscribing to %s: the server's answer has no id", topic)
		}
		sctx, cancel := context.WithTimeout(ctx, fhirSubCursorTimeout)
		err = cursors.Save(sctx, c.interfaceID, key, made.ID)
		cancel()
		if err != nil {
			return fmt.Errorf("keeping the subscription id for %s: %w", topic, err)
		}
		log.Printf("✅ FHIR Subscription Inbound: interface %s subscribed to %s (Subscription/%s)", c.interfaceID, topic, made.ID)
	}
	return nil
}

// subscriptionBody is a topic's Backport-profile Subscription.
func (c *FHIRSubscriptionInboundConnector) subscriptionBody(topic, id string) map[string]interface{} {
	channel := map[string]interface{}{
		"type": "rest-hook", "endpoint": c.callbackURL, "payload": fhirSubContentType,
		"_payload": map[string]interface{}{"extension": []interface{}{map[string]interface{}{"url": backportContent, "valueCode": c.content}}},
		"header":   []string{"Authorization: Bearer " + c.headerToken(topic)},
	}
	if c.heartbeat > 0 {
		channel["extension"] = []interface{}{map[string]interface{}{"url": backportHeartbeat, "valueUnsignedInt": c.heartbeat}}
	}
	sub := map[string]interface{}{
		"resourceType": "Subscription",
		"meta":         map[string]interface{}{"profile": []string{backportProfile}},
		"status":       "requested",
		"reason":       "ezHealthKonnect interface " + c.interfaceID,
		"criteria":     topic,
		"channel":      channel,
	}
	if id != "" {
		sub["id"] = id
	}
	if len(c.filters) > 0 {
		var ext []interface{}
		for _, f := range c.filters {
			ext = append(ext, map[string]interface{}{"url": backportFilter, "valueString": f})
		}
		sub["_criteria"] = map[string]interface{}{"extension": ext}
	}
	return sub
}

// call sends a FHIR request with the server's credential; statuses in
// accept are answers, not errors.
func (c *FHIRSubscriptionInboundConnector) call(ctx context.Context, method, u string, body interface{}, accept ...int) (*httpservice.Response, error) {
	cfg := &httpservice.RequestConfig{Method: method, URL: u, Body: body, Timeout: 20 * time.Second, AcceptStatuses: accept,
		Headers: map[string]string{"Accept": fhirSubContentType}}
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		cfg.Body = string(raw)
		cfg.Headers["Content-Type"] = fhirSubContentType
	}
	return c.http.Execute(ctx, cfg, c.auth)
}

// --------------------------------------------------------------------------
// Notifications
// --------------------------------------------------------------------------

// notificationEvent is one event a notification carries.
type notificationEvent struct {
	Number    int64
	Timestamp string
	Focus     string
	Context   []string
}

// notification is a received notification Bundle, read.
type notification struct {
	Subscription string // its id
	Topic        string
	Type         string
	Events       []notificationEvent
	// Entries are the Patients it carries, by "Patient/<id>": the resource
	// (nil for id-only) and whether it's a deletion.
	Entries map[string]notificationEntry
}

type notificationEntry struct {
	Resource json.RawMessage
	Deleted  bool
}

// parseNotification reads a Backport R4 notification Bundle.
func parseNotification(body []byte) (notification, error) {
	var b struct {
		ResourceType string `json:"resourceType"`
		Type         string `json:"type"`
		Entry        []struct {
			FullURL  string          `json:"fullUrl"`
			Resource json.RawMessage `json:"resource"`
			Request  *struct {
				Method string `json:"method"`
				URL    string `json:"url"`
			} `json:"request"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(body, &b); err != nil || b.ResourceType != "Bundle" || b.Type != "history" || len(b.Entry) == 0 {
		return notification{}, errors.New("the body isn't a notification Bundle")
	}
	var status struct {
		ResourceType string `json:"resourceType"`
		Parameter    []struct {
			Name           string `json:"name"`
			ValueString    string `json:"valueString"`
			ValueCode      string `json:"valueCode"`
			ValueCanonical string `json:"valueCanonical"`
			ValueInstant   string `json:"valueInstant"`
			ValueReference *struct {
				Reference string `json:"reference"`
			} `json:"valueReference"`
			Part []struct {
				Name           string `json:"name"`
				ValueString    string `json:"valueString"`
				ValueInstant   string `json:"valueInstant"`
				ValueReference *struct {
					Reference string `json:"reference"`
				} `json:"valueReference"`
			} `json:"part"`
		} `json:"parameter"`
	}
	if err := json.Unmarshal(b.Entry[0].Resource, &status); err != nil || status.ResourceType != "Parameters" {
		return notification{}, errors.New("the notification's first entry isn't its status")
	}
	n := notification{Entries: map[string]notificationEntry{}}
	for _, p := range status.Parameter {
		switch p.Name {
		case "subscription":
			if p.ValueReference != nil {
				ref := strings.TrimRight(p.ValueReference.Reference, "/")
				n.Subscription = ref[strings.LastIndex(ref, "/")+1:]
			}
		case "topic":
			n.Topic = p.ValueCanonical
		case "type":
			n.Type = p.ValueCode
		case "notification-event":
			var ev notificationEvent
			for _, part := range p.Part {
				switch part.Name {
				case "event-number":
					num, err := strconv.ParseInt(part.ValueString, 10, 64)
					if err != nil || num < 1 {
						return notification{}, fmt.Errorf("event number %q isn't a number", part.ValueString)
					}
					ev.Number = num
				case "timestamp":
					ev.Timestamp = part.ValueInstant
				case "focus":
					if part.ValueReference != nil {
						ev.Focus = relativeReference(part.ValueReference.Reference)
					}
				case "additional-context":
					if part.ValueReference != nil {
						ev.Context = append(ev.Context, relativeReference(part.ValueReference.Reference))
					}
				}
			}
			if ev.Number == 0 {
				return notification{}, errors.New("a notification event has no number")
			}
			n.Events = append(n.Events, ev)
		}
	}
	if n.Subscription == "" || n.Type == "" {
		return notification{}, errors.New("the notification's status names no subscription or type")
	}
	sort.Slice(n.Events, func(i, j int) bool { return n.Events[i].Number < n.Events[j].Number })
	for _, e := range b.Entry[1:] {
		key := relativeReference(e.FullURL)
		if key == "" && e.Request != nil {
			key = relativeReference(e.Request.URL)
		}
		if key == "" {
			continue
		}
		entry := notificationEntry{Resource: e.Resource}
		if e.Request != nil && strings.EqualFold(e.Request.Method, http.MethodDelete) {
			entry.Deleted = true
		}
		if len(entry.Resource) == 0 || string(entry.Resource) == "null" {
			entry.Resource = nil
		}
		n.Entries[key] = entry
	}
	return n, nil
}

// relativeReference reduces a reference or full URL to "Type/id".
func relativeReference(ref string) string {
	ref = strings.TrimSpace(ref)
	if i := strings.Index(ref, "?"); i >= 0 {
		ref = ref[:i]
	}
	parts := strings.Split(strings.TrimRight(ref, "/"), "/")
	if len(parts) < 2 || parts[len(parts)-2] == "" || parts[len(parts)-1] == "" {
		return ""
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}

func (c *FHIRSubscriptionInboundConnector) handleRequest(w http.ResponseWriter, r *http.Request) {
	deadline := time.Now().Add(c.replyBudget)
	if r.Method != http.MethodPost {
		writeWebhookReply(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "only POST is accepted"})
		return
	}
	topic, ok := c.topicOfRequest(r)
	if !ok {
		writeWebhookReply(w, http.StatusUnauthorized, map[string]interface{}{"error": "authentication failed"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, c.maxBody+1))
	if err != nil || int64(len(body)) > c.maxBody {
		writeWebhookReply(w, http.StatusRequestEntityTooLarge, map[string]interface{}{"error": "the body is too large or unreadable"})
		return
	}
	n, err := parseNotification(body)
	if err != nil {
		writeWebhookReply(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
		return
	}
	if n.Topic != "" && n.Topic != topic {
		writeWebhookReply(w, http.StatusBadRequest, map[string]interface{}{"error": "the notification's topic doesn't match its credential"})
		return
	}
	n.Topic = topic
	switch n.Type {
	case "handshake", "heartbeat":
		writeWebhookReply(w, http.StatusOK, map[string]interface{}{"received": n.Type})
		return
	case "event-notification", "query-event":
	default:
		writeWebhookReply(w, http.StatusOK, map[string]interface{}{"ignored": n.Type})
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
	select {
	case c.busy <- struct{}{}:
	case <-time.After(time.Until(deadline)):
		w.Header().Set("Retry-After", "1")
		writeWebhookReply(w, http.StatusServiceUnavailable, map[string]interface{}{"error": "an earlier notification is still being processed"})
		return
	}
	done := make(chan batchResult, 1)
	go func() {
		defer func() { <-c.busy }()
		done <- c.processNotification(n, deadline, cursors, messageChan, sourceHost(r))
	}()
	var res batchResult
	select {
	case res = <-done:
	case <-time.After(time.Until(deadline) + 500*time.Millisecond):
		res = batchResult{partial: true}
	}
	switch {
	case res.err != nil:
		writeWebhookReply(w, http.StatusInternalServerError, map[string]interface{}{"error": res.err.Error(), "processed": res.processed, "skipped": res.skipped})
	case res.partial:
		w.Header().Set("Retry-After", "1")
		writeWebhookReply(w, http.StatusServiceUnavailable, map[string]interface{}{"error": "not all events were processed in time; resend to continue",
			"processed": res.processed, "skipped": res.skipped})
	default:
		writeWebhookReply(w, http.StatusOK, map[string]interface{}{"processed": res.processed, "skipped": res.skipped})
	}
}

// processNotification runs a notification's events in order, filling a gap
// before them from $events. It never starts an event after the deadline,
// but always waits for the one it started.
func (c *FHIRSubscriptionInboundConnector) processNotification(n notification, deadline time.Time, cursors CursorStore,
	messageChan chan<- *models.InboundMessage, sourceIP string) batchResult {

	var res batchResult
	key := "events|" + n.Subscription
	cursor, err := c.loadPosition(cursors, key)
	if err != nil {
		res.err = errors.New("the receiver's position could not be read")
		log.Printf("❌ FHIR Subscription Inbound: loading position %q for interface %s: %v", key, c.interfaceID, err)
		return res
	}
	// A gap before this notification's first event is filled from $events,
	// a page (up to 100) at a time. With no position yet (a new
	// subscription), processing starts from what arrives.
	var queue []notification
	if len(n.Events) > 0 && cursor > 0 {
		next, last := cursor+1, n.Events[0].Number-1
		for next <= last {
			if time.Now().After(deadline) {
				res.partial = true
				return res
			}
			missed, err := c.fetchEvents(n.Subscription, next, last)
			if err != nil {
				res.err = fmt.Errorf("events %d to %d were missed and couldn't be fetched: %v", next, last, err)
				return res
			}
			missed.Topic = n.Topic
			if len(missed.Events) == 0 {
				// The server no longer keeps them (past its retention).
				log.Printf("⚠️ FHIR Subscription Inbound: interface %s: events %d to %d of Subscription/%s are no longer available; continuing",
					c.interfaceID, next, last, n.Subscription)
				break
			}
			queue = append(queue, missed)
			next = missed.Events[len(missed.Events)-1].Number + 1
		}
	}
	queue = append(queue, n)
	for _, q := range queue {
		for _, ev := range q.Events {
			if ev.Number <= cursor {
				res.skipped++
				continue
			}
			if time.Now().After(deadline) {
				res.partial = true
				return res
			}
			outcome, err := c.runEvent(q, ev, messageChan, sourceIP)
			if err != nil {
				res.err = err
				return res
			}
			if !outcome.Succeeded() {
				res.err = fmt.Errorf("event %d was not processed (%s); see message %s", ev.Number, outcome.Status, outcome.MessageID)
				return res
			}
			sctx, cancel := context.WithTimeout(context.Background(), fhirSubCursorTimeout)
			err = cursors.Save(sctx, c.interfaceID, key, strconv.FormatInt(ev.Number, 10))
			cancel()
			if err != nil {
				res.err = fmt.Errorf("event %d was processed but the receiver's position could not be saved", ev.Number)
				return res
			}
			cursor = ev.Number
			res.processed++
		}
	}
	return res
}

func (c *FHIRSubscriptionInboundConnector) loadPosition(cursors CursorStore, key string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fhirSubCursorTimeout)
	defer cancel()
	v, found, err := cursors.Load(ctx, c.interfaceID, key)
	if err != nil || !found {
		return 0, err
	}
	return strconv.ParseInt(v, 10, 64)
}

// fetchEvents asks the server for missed events ($events).
func (c *FHIRSubscriptionInboundConnector) fetchEvents(subscription string, since, until int64) (notification, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	u := fmt.Sprintf("%s/Subscription/%s/$events?eventsSinceNumber=%d&eventsUntilNumber=%d", c.base, url.PathEscape(subscription), since, until)
	resp, err := c.call(ctx, http.MethodGet, u, nil)
	if err != nil {
		return notification{}, err
	}
	return parseNotification(resp.Body)
}

// eventMessage is what one event becomes: its topic, number and time, and
// the Patients it's about.
type eventMessage struct {
	Topic        string          `json:"topic"`
	TopicURL     string          `json:"topicUrl"`
	EventNumber  int64           `json:"eventNumber"`
	Timestamp    string          `json:"timestamp,omitempty"`
	Subscription string          `json:"subscription"`
	Focus        *eventResource  `json:"focus,omitempty"`
	Context      []eventResource `json:"context"`
}

type eventResource struct {
	Reference string          `json:"reference"`
	ID        string          `json:"id"`
	Resource  json.RawMessage `json:"resource,omitempty"`
	Erased    bool            `json:"erased"`
}

func (n notification) resource(ref string) eventResource {
	e := eventResource{Reference: ref, ID: ref[strings.LastIndex(ref, "/")+1:]}
	if entry, ok := n.Entries[ref]; ok {
		e.Resource, e.Erased = entry.Resource, entry.Deleted
	}
	return e
}

// runEvent emits one event as a message and waits for its outcome.
func (c *FHIRSubscriptionInboundConnector) runEvent(n notification, ev notificationEvent, messageChan chan<- *models.InboundMessage,
	sourceIP string) (models.MessageOutcome, error) {

	short := n.Topic[strings.LastIndex(n.Topic, "/")+1:]
	m := eventMessage{Topic: short, TopicURL: n.Topic, EventNumber: ev.Number, Timestamp: ev.Timestamp, Subscription: n.Subscription,
		Context: []eventResource{}}
	if ev.Focus != "" {
		f := n.resource(ev.Focus)
		m.Focus = &f
	}
	for _, ref := range ev.Context {
		m.Context = append(m.Context, n.resource(ref))
	}
	content, err := json.Marshal(m)
	if err != nil {
		return models.MessageOutcome{}, err
	}
	msg := &models.InboundMessage{
		MessageID:      fmt.Sprintf("fhirsub_%s_%d_%d", n.Subscription, ev.Number, time.Now().UnixNano()),
		CorrelationID:  fmt.Sprintf("%s#%d", n.Subscription, ev.Number),
		InterfaceID:    c.interfaceID,
		Content:        string(content),
		ContentType:    "application/json",
		SourceType:     "fhir_subscription",
		SourceEndpoint: c.endpointPath,
		SourceIP:       sourceIP,
		ReceivedAt:     time.Now(),
		MessageType:    short,
		MessageSize:    len(content),
		Headers: map[string]string{
			"Content-Type":      "application/json",
			"fhir-topic":        n.Topic,
			"fhir-event-number": strconv.FormatInt(ev.Number, 10),
			"fhir-subscription": n.Subscription,
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
		return models.MessageOutcome{}, fmt.Errorf("the interface's queue did not accept event %d", ev.Number)
	}
	select {
	case o := <-wait:
		return o, nil
	case <-time.After(c.eventWait):
		forget()
		return models.MessageOutcome{}, fmt.Errorf("event %d did not finish within %s; see message %s", ev.Number, c.eventWait, msg.MessageID)
	}
}
