// services/telemetry_service.go
// Zero-cost product telemetry — install pings and feedback submissions.
//
// Data sent:
//   install_ping  — fires once on first run: install ID, version, OS, admin email, timezone
//   feedback      — fires when operator clicks "Submit to Team": aggregate counts + admin note
//
// PHI guarantee: no patient data, message content, or credentials are ever included.
// The receiver is a Google Apps Script web app (HTTPS, Google-hosted, zero cost).
// Source IP is captured server-side by the receiver — we never read it from inside the app.
//
// Opt-out: set TELEMETRY_ENABLED=false in environment or disable in system_settings.
package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

// ─── Constants ────────────────────────────────────────────────────────────────

// telemetryEndpoint and telemetrySecret are no longer hardcoded constants.
// They are stored encrypted in the product_config table and read at runtime
// via ProductConfigService. Official installer builds seed real values;
// community / self-built installs get empty rows → telemetry silently disabled.
//
// DB keys:  "telemetry_endpoint"  |  "telemetry_secret"

// ─── Payload types ────────────────────────────────────────────────────────────

// installPing is sent once on first startup after installation.
type installPing struct {
	EventType      string    `json:"event_type"`       // "install_ping"
	InstallID      string    `json:"install_id"`       // UUID, generated once, stored in DB
	InstanceName   string    `json:"instance_name"`    // server hostname — human-friendly identifier
	ProductVersion string    `json:"product_version"`  // e.g. "1.2.0"
	Edition        string    `json:"edition"`          // "community" | "enterprise"
	OS             string    `json:"os"`               // "linux" | "windows" | "darwin"
	Arch           string    `json:"arch"`             // "amd64" | "arm64"
	AdminEmail     string    `json:"admin_email"`      // first user's email — rough contact point
	PublicIP       string    `json:"public_ip"`        // egress IP — fetched from ipify, best-effort
	Timezone       string    `json:"timezone"`         // server timezone — rough geo signal
	RegisteredAt   time.Time `json:"registered_at"`
	Sig            string    `json:"sig"`              // HMAC-SHA256 of install_id+version
}

// feedbackSubmit is the anonymized aggregate report sent from Settings → AI → Feedback.
type feedbackSubmit struct {
	EventType      string            `json:"event_type"`       // "feedback_submit"
	InstallID      string            `json:"install_id"`
	ProductVersion string            `json:"product_version"`
	Edition        string            `json:"edition"`
	PeriodDays     int               `json:"period_days"`
	Total          int               `json:"total"`
	PositivePct    float64           `json:"positive_pct"`
	KBIngested     int               `json:"kb_ingested"`
	ByEndpoint     map[string]interface{} `json:"by_endpoint"`
	AdminComment   string            `json:"admin_comment"`
	SubmittedAt    time.Time         `json:"submitted_at"`
	Sig            string            `json:"sig"`
}

// ─── TelemetryService ─────────────────────────────────────────────────────────

// TelemetryService sends anonymized product usage signals to the ezHealthKonnect team.
// All methods are safe to call with a nil receiver (dev/air-gapped mode).
type TelemetryService struct {
	db         *sql.DB
	client     *http.Client
	version    string
	edition    string
	prodConfig *ProductConfigService
}

// NewTelemetryService creates a TelemetryService. Pass version from build flags, edition
// from environment (EHK_EDITION=community|enterprise, default community).
func NewTelemetryService(db *sql.DB, version, edition string) *TelemetryService {
	if edition == "" {
		edition = "community"
	}
	if version == "" {
		version = "dev"
	}
	return &TelemetryService{
		db:      db,
		// Google Apps Script returns a 302 redirect after executing the script.
		// The redirect is to script.googleusercontent.com and should be followed
		// as a GET to retrieve the response. Go's default behaviour (POST→GET on 302)
		// is exactly right here, so no custom CheckRedirect is needed.
		client:  &http.Client{Timeout: 15 * time.Second},
		version: version,
		edition: edition,
	}
}

// SetProductConfig wires in the ProductConfigService after construction.
// Must be called before SendInstallPingIfNew or SendFeedback.
func (t *TelemetryService) SetProductConfig(pc *ProductConfigService) {
	t.prodConfig = pc
}

// SendInstallPingIfNew fires the install_ping exactly once per installation.
// It checks system_settings for "install_registered" — if already set, returns immediately.
// Designed to be called as a goroutine after DB is ready.
func (t *TelemetryService) SendInstallPingIfNew(ctx context.Context) {
	if t == nil || t.db == nil {
		return
	}

	// Check if already registered
	var already bool
	row := t.db.QueryRowContext(ctx,
		`SELECT value::text = 'true' FROM system_settings WHERE key = 'install_registered' LIMIT 1`)
	_ = row.Scan(&already)
	if already {
		return
	}

	installID := t.getOrCreateInstallID(ctx)
	adminEmail := t.getFirstAdminEmail(ctx)
	tz, _ := time.Now().Zone()
	hostname, _ := os.Hostname()

	payload := installPing{
		EventType:      "install_ping",
		InstallID:      installID,
		InstanceName:   hostname,
		ProductVersion: t.version,
		Edition:        t.edition,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		AdminEmail:     adminEmail,
		PublicIP:       t.fetchPublicIP(ctx),
		Timezone:       tz,
		RegisteredAt:   time.Now().UTC(),
	}
	secret := ""
	if t.prodConfig != nil {
		secret = t.prodConfig.Get(ctx, "telemetry_secret")
	}
	payload.Sig = sign(installID+t.version, secret)

	if err := t.post(ctx, payload); err != nil {
		log.Printf("⚠️  Telemetry install ping failed (non-critical): %v", err)
		return
	}

	// Mark as registered so we never send again
	_, _ = t.db.ExecContext(ctx, `
		INSERT INTO system_settings (key, value, description, updated_at)
		VALUES ('install_registered', 'true', 'Telemetry install ping sent', NOW())
		ON CONFLICT (key) DO UPDATE SET value = 'true', updated_at = NOW()
	`)

	log.Printf("✅ Telemetry: install ping sent (install_id=%s, edition=%s)", installID, t.edition)
}

// SendFeedback sends the aggregate feedback report to the team.
// Called from FeedbackSubmitToTeam after the operator clicks "Submit to Team".
// summary is already built by FeedbackService.GetSummary — we just wrap and sign it.
func (t *TelemetryService) SendFeedback(ctx context.Context, summary interface{}, adminComment string) error {
	if t == nil {
		return fmt.Errorf("telemetry not initialised")
	}

	installID := ""
	if t.db != nil {
		installID = t.getOrCreateInstallID(ctx)
	}

	// Encode summary as a generic map so we don't import the ai package
	b, _ := json.Marshal(summary)
	var decoded map[string]interface{}
	_ = json.Unmarshal(b, &decoded)

	payload := feedbackSubmit{
		EventType:      "feedback_submit",
		InstallID:      installID,
		ProductVersion: t.version,
		Edition:        t.edition,
		SubmittedAt:    time.Now().UTC(),
		AdminComment:   adminComment,
	}
	if v, ok := decoded["period_days"].(float64); ok {
		payload.PeriodDays = int(v)
	}
	if v, ok := decoded["total"].(float64); ok {
		payload.Total = int(v)
	}
	if v, ok := decoded["positive_pct"].(float64); ok {
		payload.PositivePct = v
	}
	if v, ok := decoded["kb_ingested"].(float64); ok {
		payload.KBIngested = int(v)
	}
	if v, ok := decoded["by_endpoint"]; ok {
		if ep, ok2 := v.(map[string]interface{}); ok2 {
			payload.ByEndpoint = ep
		}
	}
	secret := ""
	if t.prodConfig != nil {
		secret = t.prodConfig.Get(ctx, "telemetry_secret")
	}
	payload.Sig = sign(installID+t.version+fmt.Sprintf("%d", payload.Total), secret)

	return t.post(ctx, payload)
}

// responseFeedbackPayload is sent immediately when a user gives thumbs-up/down on any AI response.
type responseFeedbackPayload struct {
	EventType      string    `json:"event_type"`            // "response_feedback"
	InstallID      string    `json:"install_id"`
	ProductVersion string    `json:"product_version"`
	Edition        string    `json:"edition"`
	SessionID      string    `json:"session_id,omitempty"`
	Sentiment      string    `json:"sentiment"`             // "positive" | "negative"
	Endpoint       string    `json:"endpoint"`              // ask | generate-script | trace-message | explain-step
	Comment        string    `json:"comment,omitempty"`
	InterfaceID    string    `json:"interface_id,omitempty"`
	PipelineID     string    `json:"pipeline_id,omitempty"`
	StepID         string    `json:"step_id,omitempty"`
	UserEmail      string    `json:"user_email,omitempty"`
	PromptPreview  string    `json:"prompt_preview,omitempty"`
	SubmittedAt    time.Time `json:"submitted_at"`
	Sig            string    `json:"sig"`
}

// SendResponseFeedback fires immediately when a user submits thumbs-up/down on an AI response.
// Designed to be called in a goroutine — non-blocking, non-fatal.
func (t *TelemetryService) SendResponseFeedback(ctx context.Context, sentiment, endpoint, comment, sessionID, interfaceID, pipelineID, stepID, userEmail, promptPreview string) error {
	if t == nil {
		return nil
	}
	installID := ""
	if t.db != nil {
		installID = t.getOrCreateInstallID(ctx)
	}
	secret := ""
	if t.prodConfig != nil {
		secret = t.prodConfig.Get(ctx, "telemetry_secret")
	}
	payload := responseFeedbackPayload{
		EventType:      "response_feedback",
		InstallID:      installID,
		ProductVersion: t.version,
		Edition:        t.edition,
		SessionID:      sessionID,
		Sentiment:      sentiment,
		Endpoint:       endpoint,
		Comment:        comment,
		InterfaceID:    interfaceID,
		PipelineID:     pipelineID,
		StepID:         stepID,
		UserEmail:      userEmail,
		PromptPreview:  promptPreview,
		SubmittedAt:    time.Now().UTC(),
		Sig:            sign(installID+t.version+sentiment, secret),
	}
	return t.post(ctx, payload)
}

// ─── Internals ────────────────────────────────────────────────────────────────

func (t *TelemetryService) post(ctx context.Context, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telemetry payload: %w", err)
	}

	// Resolve endpoint: operator override → DB product_config → disabled
	endpoint := ""
	if t.db != nil {
		if override := GetAppSettings().GetAIConfig().FeedbackWebhookURL; override != "" {
			endpoint = override
		}
	}
	if endpoint == "" && t.prodConfig != nil {
		endpoint = t.prodConfig.Get(ctx, "telemetry_endpoint")
	}
	if endpoint == "" {
		return nil // telemetry disabled — no endpoint configured
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build telemetry request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ezHealthKonnect/"+t.version)

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("send telemetry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("telemetry receiver returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// getOrCreateInstallID returns the persistent install UUID from system_settings,
// creating and storing it if this is the first call.
func (t *TelemetryService) getOrCreateInstallID(ctx context.Context) string {
	var id string
	// value is JSONB — use #>>'{}'  to extract the raw string without JSON quotes
	row := t.db.QueryRowContext(ctx,
		`SELECT value#>>'{}' FROM system_settings WHERE key = 'install_id' LIMIT 1`)
	if err := row.Scan(&id); err == nil && id != "" {
		return id
	}

	// Generate new install ID
	var newID string
	if err := t.db.QueryRowContext(ctx, `SELECT gen_random_uuid()::text`).Scan(&newID); err != nil {
		return "unknown"
	}
	// to_json() wraps the UUID as a valid JSONB string value
	_, _ = t.db.ExecContext(ctx, `
		INSERT INTO system_settings (key, value, description, updated_at)
		VALUES ('install_id', to_json($1::text), 'Unique installation identifier (anonymized, never contains PHI)', NOW())
		ON CONFLICT (key) DO NOTHING
	`, newID)
	return newID
}

// getFirstAdminEmail returns the email of the first admin user.
// Used only for the install ping so the team has a contact point.
// Operators can remove this by setting telemetry_admin_email = "" in system_settings.
func (t *TelemetryService) getFirstAdminEmail(ctx context.Context) string {
	// Check for explicit override first
	var override string
	_ = t.db.QueryRowContext(ctx,
		`SELECT value FROM system_settings WHERE key = 'telemetry_admin_email' LIMIT 1`).Scan(&override)
	if override == "none" {
		return ""
	}

	var email string
	_ = t.db.QueryRowContext(ctx,
		`SELECT email FROM users WHERE role = 'admin' ORDER BY created_at ASC LIMIT 1`).Scan(&email)
	return email
}

// fetchPublicIP asks api.ipify.org for the server's public egress IP.
// Returns empty string silently on any error — IP is best-effort, never fatal.
func (t *TelemetryService) fetchPublicIP(ctx context.Context) string {
	ipCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ipCtx, http.MethodGet, "https://api.ipify.org?format=text", nil)
	if err != nil {
		return ""
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// sign returns a short HMAC-SHA256 signature of data using secret.
// The Apps Script verifies this to reject requests not from a real installation.
// Returns "" when secret is empty (community build — caller should skip sending).
func sign(data, secret string) string {
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}
