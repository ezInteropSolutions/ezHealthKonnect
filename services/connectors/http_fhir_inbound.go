// services/connectors/http_fhir_inbound.go
// HTTP FHIR Inbound Connector — full FHIR REST server receiver.
//
// Accepts the complete FHIR RESTful API surface (§3.1) and Operations (§3.3).
// Every inbound request is parsed and forwarded to the pipeline as an
// InboundMessage with rich FHIR context headers so Switch/Case steps can
// route on any dimension without needing to re-parse the body.
//
// ── URL routing (parsed from incoming request path) ──────────────────────────
//   POST   <basePath>                   → server-level create / passthrough
//   POST   <basePath>/$process-message  → server-level FHIR operation
//   POST   <basePath>/<RT>              → create resource of type RT
//   GET    <basePath>/<RT>?params       → search (query in X-Query-String)
//   GET    <basePath>/<RT>/<id>         → read instance
//   PUT    <basePath>/<RT>/<id>         → update
//   PATCH  <basePath>/<RT>/<id>         → partial update
//   DELETE <basePath>/<RT>/<id>         → delete
//   POST   <basePath>/<RT>/$validate    → type-level operation
//   POST   <basePath>/<RT>/<id>/$everything → instance-level operation
//   POST   <basePath>/$transaction      → transaction Bundle
//   POST   <basePath>/$batch            → batch Bundle
//   GET    <basePath>/metadata          → CapabilityStatement
//   GET    /health                      → liveness probe
//
// ── Message headers set on every InboundMessage ──────────────────────────────
//   X-HTTP-Method        — GET / POST / PUT / PATCH / DELETE
//   X-FHIR-Version       — R4 / R5 / STU3
//   X-FHIR-ResourceType  — Patient / Observation / Bundle …
//   X-FHIR-ResourceId    — logical id (when present in URL or body)
//   X-FHIR-Operation     — $validate / $everything / $process-message …
//   X-Query-String       — raw query string (search params, _format, etc.)
//   X-Bundle-Type        — transaction / batch / collection / document
//   X-Bundle-Mode        — bundle_as_unit | bundle_unwrap (when bundleMode=unwrap)
//
// ── MessageType conventions ───────────────────────────────────────────────────
//   FHIR:Patient                   single resource (POST/body-detected)
//   FHIR:Patient:read              GET <RT>/<id>
//   FHIR:Patient:search            GET <RT>?params
//   FHIR:Patient:update            PUT <RT>/<id>
//   FHIR:Patient:patch             PATCH <RT>/<id>
//   FHIR:Patient:delete            DELETE <RT>/<id>
//   FHIR:Patient:$validate         type-level operation
//   FHIR:Patient:id:$everything    instance-level operation
//   FHIR:$process-message          server-level operation
//   FHIR:Bundle:transaction        transaction bundle (bundle_as_unit)
//   FHIR:Bundle:batch              batch bundle (bundle_as_unit)

package connectors

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"
)

// ─────────────────────────────────────────────────────────────
// Connector struct
// ─────────────────────────────────────────────────────────────

// HTTPFHIRInboundConnector implements an HTTP FHIR REST API listener.
type HTTPFHIRInboundConnector struct {
	*BaseInboundConnector
	server         *http.Server
	port           int
	basePath       string
	bundleMode     string // "bundle_as_unit" | "bundle_unwrap"
	allowedMethods map[string]bool // nil = allow all
	authType       string
	authUsername   string
	authPassword   string
	apiKey         string
	apiKeyHeader   string // header name for API key auth (default: X-API-Key)
	bearerToken    string
	fhirVersion    string
	enableCORS     bool
	maxBodySize    int64
	requestTimeout time.Duration
	messageChan    chan<- *models.InboundMessage
	mu             sync.RWMutex

	// TLS
	tlsEnabled  bool
	tlsCertFile string
	tlsKeyFile  string

	// IP allowlist (nil = allow all)
	allowedIPs []string
}

// NewHTTPFHIRInboundConnector creates a new HTTP FHIR inbound connector.
func NewHTTPFHIRInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:    "http_fhir_inbound",
		DisplayName: "HTTP FHIR Receiver",
		Version:     "2.0.0",
		Category:    "inbound",
		Mode:        "push",
	}
	return &HTTPFHIRInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
		basePath:             "/fhir/r4",
		bundleMode:           "bundle_as_unit",
		fhirVersion:          "R4",
		enableCORS:           true,
		apiKeyHeader:         "X-API-Key",
		maxBodySize:          10 * 1024 * 1024, // 10 MB
		requestTimeout:       30 * time.Second,
	}
}

// ─────────────────────────────────────────────────────────────
// Initialize
// ─────────────────────────────────────────────────────────────

func (h *HTTPFHIRInboundConnector) Initialize(config []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var cfg map[string]interface{}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Port (required)
	port, ok := cfg["port"].(float64)
	if !ok || port == 0 {
		return fmt.Errorf("port is required")
	}
	h.port = int(port)

	// Base path
	if v, ok := cfg["basePath"].(string); ok && v != "" {
		h.basePath = v
	}

	// FHIR version
	if v, ok := cfg["fhirVersion"].(string); ok && v != "" {
		h.fhirVersion = strings.ToUpper(v)
	}

	// Bundle mode
	if v, ok := cfg["bundleMode"].(string); ok && v != "" {
		switch v {
		case "bundle_as_unit", "bundle_unwrap":
			h.bundleMode = v
		default:
			return fmt.Errorf("bundleMode must be 'bundle_as_unit' or 'bundle_unwrap', got %q", v)
		}
	}

	// Max body size (MB)
	if v, ok := cfg["maxBodySizeMB"].(float64); ok && v > 0 {
		h.maxBodySize = int64(v) * 1024 * 1024
	}

	// Authentication
	if authType, ok := cfg["authType"].(string); ok {
		h.authType = authType
		switch authType {
		case "basic":
			h.authUsername, _ = cfg["username"].(string)
			h.authPassword, _ = cfg["password"].(string)
		case "bearer":
			h.bearerToken, _ = cfg["bearerToken"].(string)
		case "api_key":
			h.apiKey, _ = cfg["apiKey"].(string)
			if v, ok := cfg["apiKeyHeader"].(string); ok && v != "" {
				h.apiKeyHeader = v
			}
		}
	}

	// Allowed HTTP methods (nil = all allowed)
	if raw, ok := cfg["allowedMethods"].([]interface{}); ok && len(raw) > 0 {
		h.allowedMethods = make(map[string]bool, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok && s != "" {
				h.allowedMethods[strings.ToUpper(s)] = true
			}
		}
	}

	// CORS (default: enabled)
	if v, ok := cfg["enableCORS"].(bool); ok {
		h.enableCORS = v
	}

	// Request timeout
	if v, ok := cfg["requestTimeoutSeconds"].(float64); ok && v > 0 {
		h.requestTimeout = time.Duration(v) * time.Second
	}

	// TLS
	if tls, ok := cfg["tls"].(map[string]interface{}); ok {
		if enabled, ok := tls["enabled"].(bool); ok {
			h.tlsEnabled = enabled
		}
		if cert, ok := tls["certFile"].(string); ok && cert != "" {
			h.tlsCertFile = cert
		}
		if key, ok := tls["keyFile"].(string); ok && key != "" {
			h.tlsKeyFile = key
		}
		if h.tlsEnabled && (h.tlsCertFile == "" || h.tlsKeyFile == "") {
			return fmt.Errorf("TLS enabled but certFile and/or keyFile not specified")
		}
	}

	// IP allowlist
	if raw, ok := cfg["allowedIPs"].([]interface{}); ok && len(raw) > 0 {
		h.allowedIPs = make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok && s != "" {
				h.allowedIPs = append(h.allowedIPs, strings.TrimSpace(s))
			}
		}
	}

	log.Printf("✅ HTTP FHIR Inbound initialized: port=%d path=%s version=%s bundleMode=%s auth=%s cors=%v timeout=%v tls=%v allowedIPs=%d",
		h.port, h.basePath, h.fhirVersion, h.bundleMode, h.authType, h.enableCORS, h.requestTimeout, h.tlsEnabled, len(h.allowedIPs))
	return nil
}

// ─────────────────────────────────────────────────────────────
// Start / Stop
// ─────────────────────────────────────────────────────────────

func (h *HTTPFHIRInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	h.mu.Lock()
	h.messageChan = messageChan
	h.mu.Unlock()

	mux := http.NewServeMux()

	// Specific handlers that must shadow the catch-all
	mux.HandleFunc(h.basePath+"/metadata", h.handleCapabilityStatement)

	// Catch-all — handles the full FHIR RESTful API surface + all operations
	mux.HandleFunc(h.basePath, h.handleFHIRRequest)
	mux.HandleFunc(h.basePath+"/", h.handleFHIRRequest)

	// Liveness probe
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	h.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", h.port),
		Handler:      h.ipAllowlistMiddleware(h.loggingMiddleware(h.corsMiddleware(h.authMiddleware(mux)))),
		ReadTimeout:  h.requestTimeout,
		WriteTimeout: h.requestTimeout,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		scheme := "HTTP"
		if h.tlsEnabled {
			scheme = "HTTPS"
		}
		log.Printf("🌐 HTTP FHIR Inbound: listening on :%d (path=%s, %s, mode=%s, %s)",
			h.port, h.basePath, h.fhirVersion, h.bundleMode, scheme)
		var err error
		if h.tlsEnabled {
			err = h.server.ListenAndServeTLS(h.tlsCertFile, h.tlsKeyFile)
		} else {
			err = h.server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Printf("❌ HTTP FHIR Inbound: server error: %v", err)
		}
	}()

	<-ctx.Done()
	return h.Stop()
}

func (h *HTTPFHIRInboundConnector) Stop() error {
	if h.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	log.Printf("🛑 HTTP FHIR Inbound: shutting down on port %d", h.port)
	return h.server.Shutdown(ctx)
}

// ─────────────────────────────────────────────────────────────
// FHIR request handler — full RESTful API + Operations surface
// ─────────────────────────────────────────────────────────────

// fhirPathInfo holds the FHIR routing components extracted from a URL path.
type fhirPathInfo struct {
	ResourceType string // e.g. "Patient", "Bundle", ""
	ResourceID   string // e.g. "123", ""
	Operation    string // e.g. "$validate", "$everything", "$process-message", ""
}

// parseFHIRPath extracts FHIR routing components from the URL path.
// Implements the inverse of BuildFHIRURL (outbound).
//
// Patterns supported:
//
//	<basePath>                          → {empty}
//	<basePath>/$op                      → {Operation: $op}
//	<basePath>/<RT>                     → {ResourceType: RT}
//	<basePath>/<RT>/$op                 → {ResourceType: RT, Operation: $op}
//	<basePath>/<RT>/<id>                → {ResourceType: RT, ResourceID: id}
//	<basePath>/<RT>/<id>/$op            → {ResourceType: RT, ResourceID: id, Operation: $op}
func (h *HTTPFHIRInboundConnector) parseFHIRPath(urlPath string) fhirPathInfo {
	rel := strings.TrimPrefix(urlPath, strings.TrimRight(h.basePath, "/"))
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return fhirPathInfo{}
	}
	segments := strings.Split(rel, "/")

	info := fhirPathInfo{}
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if strings.HasPrefix(seg, "$") {
			// First $-prefixed segment is the operation
			if info.Operation == "" {
				info.Operation = seg
			}
			continue
		}
		// Non-operation segments: first is ResourceType, second is ResourceID
		if info.ResourceType == "" {
			info.ResourceType = seg
		} else if info.ResourceID == "" {
			info.ResourceID = seg
		}
	}
	return info
}

// buildInboundMessageType produces the canonical MessageType for an inbound FHIR request.
// Convention mirrors what Switch/Case steps expect:
//
//	FHIR:<RT>                  — POST create (or body-detected type)
//	FHIR:<RT>:read             — GET <RT>/<id>
//	FHIR:<RT>:search           — GET <RT>?params
//	FHIR:<RT>:update           — PUT  <RT>/<id>
//	FHIR:<RT>:patch            — PATCH <RT>/<id>
//	FHIR:<RT>:delete           — DELETE <RT>/<id>
//	FHIR:<RT>:<$op>            — type-level operation
//	FHIR:<RT>:<id>:<$op>       — instance-level operation
//	FHIR:<$op>                 — server-level operation
//	FHIR:Bundle:<bundleType>   — Bundle as unit
func buildInboundMessageType(method, resourceType, resourceID, operation, bundleType string) string {
	if operation != "" {
		// Server-level operation
		if resourceType == "" {
			return "FHIR:" + operation
		}
		// Instance-level
		if resourceID != "" {
			return fmt.Sprintf("FHIR:%s:%s:%s", resourceType, resourceID, operation)
		}
		// Type-level
		return fmt.Sprintf("FHIR:%s:%s", resourceType, operation)
	}

	if resourceType == "Bundle" && bundleType != "" {
		return "FHIR:Bundle:" + bundleType
	}

	if resourceType == "" {
		return "FHIR:unknown"
	}

	switch method {
	case http.MethodGet:
		if resourceID != "" {
			return fmt.Sprintf("FHIR:%s:read", resourceType)
		}
		return fmt.Sprintf("FHIR:%s:search", resourceType)
	case http.MethodPut:
		return fmt.Sprintf("FHIR:%s:update", resourceType)
	case http.MethodPatch:
		return fmt.Sprintf("FHIR:%s:patch", resourceType)
	case http.MethodDelete:
		return fmt.Sprintf("FHIR:%s:delete", resourceType)
	default: // POST — create
		return "FHIR:" + resourceType
	}
}

func (h *HTTPFHIRInboundConnector) handleFHIRRequest(w http.ResponseWriter, r *http.Request) {
	// ── 0. Method guard ────────────────────────────────────────────────────────
	if len(h.allowedMethods) > 0 && !h.allowedMethods[r.Method] {
		h.sendOperationOutcome(w, http.StatusMethodNotAllowed, "error", "not-supported",
			fmt.Sprintf("Method %s is not allowed on this receiver", r.Method))
		return
	}

	// ── 1. Parse URL path for FHIR routing components ─────────────────────────
	pathInfo := h.parseFHIRPath(r.URL.Path)

	// ── 2. Read body (GET and DELETE may have no body) ─────────────────────────
	var body []byte
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		var err error
		body, err = io.ReadAll(io.LimitReader(r.Body, h.maxBodySize))
		if err != nil {
			h.sendOperationOutcome(w, http.StatusBadRequest, "error", "invalid",
				fmt.Sprintf("Failed to read request body: %v", err))
			return
		}
		defer r.Body.Close()
	}

	// ── 3. Merge body-parsed FHIR identity with URL-parsed info ───────────────
	//   URL takes priority for resourceType/resourceID (explicit routing).
	//   Body fills in what the URL doesn't provide.
	resourceType := pathInfo.ResourceType
	resourceID   := pathInfo.ResourceID
	operation    := pathInfo.Operation
	bundleType   := ""

	if len(body) > 0 {
		if parsed := ParseFHIRResource(string(body)); parsed != nil {
			// Only fill resourceType/resourceID from body when no FHIR operation is
			// present in the URL.  When the URL carries an operation (e.g.
			// /$process-message, /Patient/$validate) the URL-determined routing level
			// (server / type / instance) is authoritative and must not be overridden
			// by the resource identity that happens to be in the body.
			if operation == "" {
				if resourceType == "" {
					resourceType = parsed.ResourceType
				}
				if resourceID == "" {
					resourceID = parsed.ID
				}
			}
			bundleType = parsed.BundleType
		} else if r.Method == http.MethodPost {
			// Body present but not a valid FHIR resource (ParseFHIRResource returned nil).
			// If the body is not parseable as JSON at all (e.g. Content-Type is FHIR JSON
			// but the bytes are garbage), always reject with 400.
			if strings.Contains(r.Header.Get("Content-Type"), "json") {
				var probe interface{}
				if json.Unmarshal(body, &probe) != nil {
					h.sendOperationOutcome(w, http.StatusBadRequest, "error", "structure",
						"Request body is not valid JSON")
					return
				}
			}
			// Valid JSON but no FHIR resourceType: only an error when the URL provides
			// no resource type either (POST to base path with non-FHIR body).
			if resourceType == "" && operation == "" {
				h.sendOperationOutcome(w, http.StatusBadRequest, "error", "structure",
					"Request body is not valid FHIR JSON and no resource type in URL")
				return
			}
		}
	} else if r.Method == http.MethodPost && resourceType == "" && operation == "" {
		// Empty body POST to the base path — a resource body is required.
		h.sendOperationOutcome(w, http.StatusBadRequest, "error", "required",
			"POST to base path requires a FHIR resource body")
		return
	}

	// ── 4. Route Bundle POST (bundle_as_unit vs bundle_unwrap) ────────────────
	if resourceType == "Bundle" && r.Method == http.MethodPost && operation == "" {
		var resource map[string]interface{}
		if err := json.Unmarshal(body, &resource); err != nil {
			h.sendOperationOutcome(w, http.StatusBadRequest, "error", "structure",
				fmt.Sprintf("Invalid Bundle JSON: %v", err))
			return
		}
		h.handleBundle(w, r, body, resource)
		return
	}

	// ── 5. Build message headers ───────────────────────────────────────────────
	headers := map[string]string{
		"X-HTTP-Method":   r.Method,
		"X-FHIR-Version":  h.fhirVersion,
		"Content-Type":    r.Header.Get("Content-Type"),
		"User-Agent":      r.Header.Get("User-Agent"),
	}
	if resourceType != "" {
		headers["X-FHIR-ResourceType"] = resourceType
	}
	if resourceID != "" {
		headers["X-FHIR-ResourceId"] = resourceID
	}
	if operation != "" {
		headers["X-FHIR-Operation"] = operation
	}
	if r.URL.RawQuery != "" {
		headers["X-Query-String"] = r.URL.RawQuery
	}

	// ── 6. Build message type and queue ───────────────────────────────────────
	msgType := buildInboundMessageType(r.Method, resourceType, resourceID, operation, bundleType)
	msg := h.buildMessage(r, body, msgType, headers)

	if !h.enqueue(w, msg) {
		return
	}

	log.Printf("📥 HTTP FHIR Inbound: %s %s → %s (%d bytes) from %s",
		r.Method, r.URL.Path, msgType, len(body), r.RemoteAddr)

	// ── 7. Send FHIR-appropriate response ─────────────────────────────────────
	switch r.Method {
	case http.MethodPost:
		h.sendAccepted(w, fmt.Sprintf("%s accepted for processing", msgType))
	case http.MethodPut, http.MethodPatch:
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"resourceType": "OperationOutcome",
			"issue": []map[string]interface{}{
				{"severity": "information", "code": "informational",
					"diagnostics": fmt.Sprintf("%s queued for processing", msgType)},
			},
		})
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	case http.MethodGet:
		// Search / read — pipeline processes async; return 202 with context
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"resourceType": "OperationOutcome",
			"issue": []map[string]interface{}{
				{"severity": "information", "code": "informational",
					"diagnostics": fmt.Sprintf("%s queued for processing", msgType)},
			},
		})
	default:
		h.sendAccepted(w, fmt.Sprintf("%s accepted", msgType))
	}
}

// handleBundle dispatches based on bundleMode config.
func (h *HTTPFHIRInboundConnector) handleBundle(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	bundle map[string]interface{},
) {
	bundleType, _ := bundle["type"].(string)
	entries := extractBundleEntries(bundle)
	entryCount := len(entries)

	log.Printf("📦 HTTP FHIR Inbound: Bundle type=%q entries=%d mode=%s from %s",
		bundleType, entryCount, h.bundleMode, r.RemoteAddr)

	switch h.bundleMode {
	case "bundle_unwrap":
		h.unwrapAndQueue(w, r, bundle, bundleType, entries)
	default: // "bundle_as_unit"
		msgType := fhirBundleMessageType(bundleType)
		msg := h.buildMessage(r, body, msgType, map[string]string{
			"X-HTTP-Method":       r.Method,
			"X-FHIR-Version":      h.fhirVersion,
			"X-FHIR-ResourceType": "Bundle",
			"X-Bundle-Type":       bundleType,
			"X-Entry-Count":       fmt.Sprintf("%d", entryCount),
			"X-Bundle-Mode":       "bundle_as_unit",
			"Content-Type":        r.Header.Get("Content-Type"),
			"User-Agent":          r.Header.Get("User-Agent"),
		})
		if !h.enqueue(w, msg) {
			return
		}
		// For transaction bundles return a transaction-response Bundle skeleton.
		// Full atomic processing is handled by the pipeline; we acknowledge receipt.
		if bundleType == "transaction" || bundleType == "batch" {
			h.sendBundleResponse(w, bundleType, entryCount)
		} else {
			h.sendAccepted(w, fmt.Sprintf("Bundle (%s, %d entries) accepted for processing", bundleType, entryCount))
		}
	}
}

// unwrapAndQueue fans out each Bundle entry as an individual InboundMessage.
func (h *HTTPFHIRInboundConnector) unwrapAndQueue(
	w http.ResponseWriter,
	r *http.Request,
	bundle map[string]interface{},
	bundleType string,
	entries []map[string]interface{},
) {
	if len(entries) == 0 {
		h.sendOperationOutcome(w, http.StatusUnprocessableEntity, "error", "required",
			"Bundle contains no entries to unwrap")
		return
	}

	queued := 0
	for i, entry := range entries {
		resource, ok := entry["resource"].(map[string]interface{})
		if !ok {
			log.Printf("⚠️  HTTP FHIR Inbound: entry[%d] has no 'resource' field, skipping", i)
			continue
		}
		resType, _ := resource["resourceType"].(string)
		if resType == "" {
			log.Printf("⚠️  HTTP FHIR Inbound: entry[%d] resource has no resourceType, skipping", i)
			continue
		}

		entryBody, err := json.Marshal(resource)
		if err != nil {
			log.Printf("⚠️  HTTP FHIR Inbound: entry[%d] marshal failed: %v", i, err)
			continue
		}

		entryID, _ := resource["id"].(string)
		msgType := "FHIR:" + resType
		entryHeaders := map[string]string{
			"X-HTTP-Method":         r.Method,
			"X-FHIR-Version":        h.fhirVersion,
			"X-FHIR-ResourceType":   resType,
			"X-Bundle-Type":         bundleType,
			"X-Bundle-Entry-Index":  fmt.Sprintf("%d", i),
			"X-Bundle-Mode":         "bundle_unwrap",
			"Content-Type":          "application/fhir+json",
			"User-Agent":            r.Header.Get("User-Agent"),
		}
		if entryID != "" {
			entryHeaders["X-FHIR-ResourceId"] = entryID
		}
		msg := h.buildMessage(r, entryBody, msgType, entryHeaders)

		// Non-blocking enqueue — drop entry if channel is full.
		select {
		case h.messageChan <- msg:
			queued++
			log.Printf("📥 HTTP FHIR Inbound: queued entry[%d] %s from Bundle", i, resType)
		default:
			log.Printf("⚠️  HTTP FHIR Inbound: channel full, dropped entry[%d] %s", i, resType)
		}
	}

	if queued == 0 {
		h.sendOperationOutcome(w, http.StatusServiceUnavailable, "error", "transient",
			"Message queue full — no entries could be accepted, please retry")
		return
	}

	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"resourceType": "OperationOutcome",
		"issue": []map[string]interface{}{
			{
				"severity":    "information",
				"code":        "informational",
				"diagnostics": fmt.Sprintf("Bundle unwrapped: %d of %d entries accepted", queued, len(entries)),
			},
		},
	})
}

// handleSingleResource queues a single non-Bundle FHIR resource.
// ─────────────────────────────────────────────────────────────
// Capability Statement  (GET <basePath>/metadata)
// ─────────────────────────────────────────────────────────────

func (h *HTTPFHIRInboundConnector) handleCapabilityStatement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendOperationOutcome(w, http.StatusMethodNotAllowed, "error", "not-supported",
			"GET is required for /metadata")
		return
	}
	// Shared FHIR utility — covers all known R4 resource types and is kept
	// in sync with the canonical FHIRResourceTypes list in fhir_utils.go.
	capability := BuildCapabilityStatement(
		fmt.Sprintf("http://localhost:%d%s", h.port, h.basePath),
		h.fhirVersion,
	)
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(capability)
}

// ─────────────────────────────────────────────────────────────
// Auth middleware
// ─────────────────────────────────────────────────────────────

func (h *HTTPFHIRInboundConnector) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip for health check and metadata
		if r.URL.Path == "/health" || strings.HasSuffix(r.URL.Path, "/metadata") {
			next.ServeHTTP(w, r)
			return
		}
		if h.authType == "" || h.authType == "none" {
			next.ServeHTTP(w, r)
			return
		}
		switch h.authType {
		case "basic":
			if !h.validateBasicAuth(r) {
				w.Header().Set("WWW-Authenticate", `Basic realm="FHIR API"`)
				h.sendOperationOutcome(w, http.StatusUnauthorized, "error", "security", "Authentication required")
				return
			}
		case "bearer":
			if !h.validateBearerToken(r) {
				h.sendOperationOutcome(w, http.StatusUnauthorized, "error", "security", "Invalid or missing bearer token")
				return
			}
		case "api_key":
			if !h.validateAPIKey(r) {
				h.sendOperationOutcome(w, http.StatusUnauthorized, "error", "security", "Invalid or missing API key")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *HTTPFHIRInboundConnector) validateBasicAuth(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Basic ") {
		return false
	}
	payload, err := base64.StdEncoding.DecodeString(auth[6:])
	if err != nil {
		return false
	}
	parts := strings.SplitN(string(payload), ":", 2)
	if len(parts) != 2 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(parts[0]), []byte(h.authUsername)) == 1 &&
		subtle.ConstantTimeCompare([]byte(parts[1]), []byte(h.authPassword)) == 1
}

func (h *HTTPFHIRInboundConnector) validateBearerToken(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(auth[7:]), []byte(h.bearerToken)) == 1
}

func (h *HTTPFHIRInboundConnector) validateAPIKey(r *http.Request) bool {
	return subtle.ConstantTimeCompare([]byte(r.Header.Get(h.apiKeyHeader)), []byte(h.apiKey)) == 1
}

// ─────────────────────────────────────────────────────────────
// IP allowlist middleware
// ─────────────────────────────────────────────────────────────

// ipAllowlistMiddleware rejects requests not originating from an allowed IP.
// If h.allowedIPs is empty, all IPs are allowed.
func (h *HTTPFHIRInboundConnector) ipAllowlistMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(h.allowedIPs) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		remoteIP := r.RemoteAddr
		if host, _, err := net.SplitHostPort(remoteIP); err == nil {
			remoteIP = host
		}
		for _, allowed := range h.allowedIPs {
			if remoteIP == allowed {
				next.ServeHTTP(w, r)
				return
			}
			// CIDR check
			if _, ipNet, err := net.ParseCIDR(allowed); err == nil {
				if ipNet.Contains(net.ParseIP(remoteIP)) {
					next.ServeHTTP(w, r)
					return
				}
			}
		}
		http.Error(w, `{"resourceType":"OperationOutcome","issue":[{"severity":"fatal","code":"forbidden","diagnostics":"IP not in allowlist"}]}`, http.StatusForbidden)
	})
}

// ─────────────────────────────────────────────────────────────
// CORS middleware
// ─────────────────────────────────────────────────────────────

func (h *HTTPFHIRInboundConnector) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.enableCORS {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ─────────────────────────────────────────────────────────────
// Logging middleware
// ─────────────────────────────────────────────────────────────

func (h *HTTPFHIRInboundConnector) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("📊 HTTP FHIR: %s %s from %s (%v)", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}

// ─────────────────────────────────────────────────────────────
// Connector interface helpers
// ─────────────────────────────────────────────────────────────

func (h *HTTPFHIRInboundConnector) SupportsCron() bool { return false }

func (h *HTTPFHIRInboundConnector) Validate() error {
	if h.port < 1 || h.port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

func (h *HTTPFHIRInboundConnector) TestConnection(ctx context.Context) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", h.port))
	if err != nil {
		return fmt.Errorf("port %d is not available: %w", h.port, err)
	}
	ln.Close()
	return nil
}

func (h *HTTPFHIRInboundConnector) GetStatus() ConnectorStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	st := ConnectorStatus{
		Connected:    h.server != nil,
		LastActivity: time.Now(),
		Metadata: map[string]string{
			"port":       fmt.Sprintf("%d", h.port),
			"basePath":   h.basePath,
			"version":    h.fhirVersion,
			"bundleMode": h.bundleMode,
			"auth":       h.authType,
		},
	}
	if h.server != nil {
		st.State = StateRunning
	} else {
		st.State = StateStopped
	}
	return st
}

// ─────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────

// buildMessage constructs a standardised InboundMessage.
func (h *HTTPFHIRInboundConnector) buildMessage(
	r *http.Request,
	body []byte,
	messageType string,
	extraHeaders map[string]string,
) *models.InboundMessage {
	sourceIP := r.RemoteAddr
	if colon := strings.LastIndex(sourceIP, ":"); colon >= 0 {
		sourceIP = sourceIP[:colon]
	}
	return &models.InboundMessage{
		MessageID:      fmt.Sprintf("fhir_%d", time.Now().UnixNano()),
		Content:        string(body),
		ContentType:    "application/fhir+json",
		SourceType:     "http_fhir",
		SourceEndpoint: r.RequestURI,
		SourceIP:       sourceIP,
		Headers:        extraHeaders,
		ReceivedAt:     time.Now(),
		MessageType:    messageType,
		MessageSize:    len(body),
	}
}

// enqueue sends msg to the processing channel; writes a 503 if full.
// Returns true on success.
func (h *HTTPFHIRInboundConnector) enqueue(w http.ResponseWriter, msg *models.InboundMessage) bool {
	select {
	case h.messageChan <- msg:
		return true
	default:
		h.sendOperationOutcome(w, http.StatusServiceUnavailable, "error", "transient",
			"Message queue full — please retry")
		return false
	}
}

// sendAccepted writes a 201 OperationOutcome acknowledgement.
func (h *HTTPFHIRInboundConnector) sendAccepted(w http.ResponseWriter, diagnostics string) {
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"resourceType": "OperationOutcome",
		"issue": []map[string]interface{}{
			{"severity": "information", "code": "informational", "diagnostics": diagnostics},
		},
	})
}

// sendBundleResponse writes a minimal transaction-response / batch-response Bundle.
func (h *HTTPFHIRInboundConnector) sendBundleResponse(w http.ResponseWriter, bundleType string, entryCount int) {
	responseType := "transaction-response"
	if bundleType == "batch" {
		responseType = "batch-response"
	}
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"resourceType": "Bundle",
		"type":         responseType,
		"entry":        []interface{}{}, // populated by pipeline after processing
		"meta": map[string]interface{}{
			"extension": []map[string]interface{}{
				{
					"url":         "http://ezhealthkonnect.com/fhir/ext/queued-entries",
					"valueInteger": entryCount,
				},
			},
		},
	})
}

// sendOperationOutcome writes a FHIR OperationOutcome error response.
func (h *HTTPFHIRInboundConnector) sendOperationOutcome(
	w http.ResponseWriter, status int, severity, code, message string,
) {
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"resourceType": "OperationOutcome",
		"issue": []map[string]interface{}{
			{"severity": severity, "code": code, "diagnostics": message},
		},
	})
}

// extractBundleEntries returns the entry slice from a Bundle resource.
func extractBundleEntries(bundle map[string]interface{}) []map[string]interface{} {
	raw, ok := bundle["entry"]
	if !ok {
		return nil
	}
	slice, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	entries := make([]map[string]interface{}, 0, len(slice))
	for _, e := range slice {
		if m, ok := e.(map[string]interface{}); ok {
			entries = append(entries, m)
		}
	}
	return entries
}

// fhirBundleMessageType builds a canonical message type string for a Bundle.
func fhirBundleMessageType(bundleType string) string {
	if bundleType == "" {
		return "FHIR:Bundle"
	}
	return "FHIR:Bundle:" + bundleType
}

// getConfigKeys is a debug helper shared by inbound connectors.
func getConfigKeys(cfg map[string]interface{}) []string {
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	return keys
}
