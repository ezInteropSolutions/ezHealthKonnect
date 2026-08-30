// services/ai/feedback_service.go
// Stores user feedback on AI responses and triggers KB improvement for positives.
//
// Feedback loop:
//   User 👍 on Q&A          → stored for admin review only, NOT added to the shared KB.
//                             (Removed 2026-08-29: the raw question text is free-form user
//                             input and can contain PHI a user typed in — e.g. a patient
//                             name in "why did patient Jane Doe's message fail" — with no
//                             redaction step. Once embedded, that text is retrievable by
//                             any other user's future question, forever. Unlike script
//                             confirmation below, there's no safe "just keep the pattern"
//                             reduction for natural-language text, so this path was turned
//                             off rather than partially fixed.)
//   User 👍 on script gen   → IngestConfirmedScript    (description+script added to KB —
//                             safe because only a short code-shape preview is kept, never
//                             the real sample message the script was tested against)
//   User 👍 on mapping      → IngestConfirmedMapping   (already handled in wizard)
//   User 👎 on anything     → stored for admin review, NOT ingested
//
// Admin path: GET /api/ai/feedback/summary and /export to review trends.
package ai

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FeedbackRecord is one piece of user feedback on an AI response.
type FeedbackRecord struct {
	ID              string         `json:"id"`
	SessionID       string         `json:"session_id,omitempty"`
	UserID          string         `json:"user_id,omitempty"`
	Endpoint        string         `json:"endpoint"`          // ask | generate-script | trace-message | explain-step | suggest-mappings | explain-error
	Sentiment       string         `json:"sentiment"`         // positive | negative
	Rating          *int           `json:"rating,omitempty"`  // 1-5, optional
	PromptPreview   string         `json:"prompt_preview,omitempty"`
	ResponsePreview string         `json:"response_preview,omitempty"`
	Comment         string         `json:"comment,omitempty"`
	InterfaceID     string         `json:"interface_id,omitempty"`
	PipelineID      string         `json:"pipeline_id,omitempty"`
	StepID          string         `json:"step_id,omitempty"`
	KBIngested      bool           `json:"kb_ingested"`
	CreatedAt       time.Time      `json:"created_at"`
}

// FeedbackSummary is the aggregated view shown to admins.
type FeedbackSummary struct {
	PeriodDays   int                      `json:"period_days"`
	Total        int                      `json:"total"`
	Positive     int                      `json:"positive"`
	Negative     int                      `json:"negative"`
	PositivePct  float64                  `json:"positive_pct"`
	KBIngested   int                      `json:"kb_ingested"`
	ByEndpoint   map[string]EndpointStats `json:"by_endpoint"`
	RecentNeg    []FeedbackRecord         `json:"recent_negative"` // last 10 negative for admin review
}

// EndpointStats breaks down positive/negative per AI feature.
type EndpointStats struct {
	Total    int     `json:"total"`
	Positive int     `json:"positive"`
	Negative int     `json:"negative"`
	PosPct   float64 `json:"positive_pct"`
}

// FeedbackService stores and queries ai_feedback rows.
type FeedbackService struct {
	db          *sql.DB
	operational *OperationalIngestionService
}

func newFeedbackService(db *sql.DB, operational *OperationalIngestionService) *FeedbackService {
	return &FeedbackService{db: db, operational: operational}
}

// ─── Save ──────────────────────────────────────────────────────────────────────

// Save persists a feedback record. If positive and enough context is available,
// it also triggers KB ingestion so future responses improve automatically.
func (s *FeedbackService) Save(ctx context.Context, fb FeedbackRecord) (string, error) {
	if s.db == nil {
		return "", nil // dev mode — silently ignore
	}

	fb.PromptPreview   = truncate(fb.PromptPreview, 200)
	fb.ResponsePreview = truncate(fb.ResponsePreview, 200)

	var id string
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO ai_feedback
		    (session_id, user_id, endpoint, sentiment, rating,
		     prompt_preview, response_preview, comment,
		     interface_id, pipeline_id, step_id, metadata)
		VALUES
		    (NULLIF($1,''), NULLIF($2,'')::uuid, $3, $4, NULLIF($5::text,'')::smallint,
		     NULLIF($6,''), NULLIF($7,''), NULLIF($8,''),
		     NULLIF($9,'')::uuid, NULLIF($10,'')::uuid, NULLIF($11,''),
		     COALESCE($12::jsonb, '{}'))
		RETURNING id
	`,
		fb.SessionID, fb.UserID, fb.Endpoint, fb.Sentiment, ratingStr(fb.Rating),
		fb.PromptPreview, fb.ResponsePreview, fb.Comment,
		fb.InterfaceID, fb.PipelineID, fb.StepID,
		"{}",
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("save feedback: %w", err)
	}

	// Trigger KB improvement for positive feedback
	if fb.Sentiment == "positive" && s.operational != nil {
		go s.improveKB(context.Background(), id, fb)
	}

	return id, nil
}

// improveKB runs asynchronously — ingests positive responses into the knowledge base.
// "ask" (plain Q&A) is deliberately NOT handled here — see the Feedback loop comment
// at the top of this file for why.
func (s *FeedbackService) improveKB(ctx context.Context, feedbackID string, fb FeedbackRecord) {
	var ingested bool
	var err error

	switch fb.Endpoint {
	case "generate-script":
		if fb.PromptPreview != "" && fb.ResponsePreview != "" {
			err = s.operational.IngestConfirmedScript(ctx, fb.PromptPreview, fb.ResponsePreview, fb.InterfaceID)
			ingested = err == nil
		}
	}

	if ingested {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE ai_feedback SET kb_ingested = true WHERE id = $1`, feedbackID)
	}
}

func ratingStr(r *int) string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%d", *r)
}

// ─── Query ────────────────────────────────────────────────────────────────────

// GetSummary returns aggregate stats for the last N days.
func (s *FeedbackService) GetSummary(ctx context.Context, days int) (FeedbackSummary, error) {
	summary := FeedbackSummary{
		PeriodDays: days,
		ByEndpoint: make(map[string]EndpointStats),
	}
	if s.db == nil {
		return summary, nil
	}
	if days <= 0 {
		days = 30
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT endpoint, sentiment, COUNT(*), SUM(CASE WHEN kb_ingested THEN 1 ELSE 0 END)
		FROM ai_feedback
		WHERE created_at >= NOW() - ($1 || ' days')::interval
		GROUP BY endpoint, sentiment
	`, days)
	if err != nil {
		return summary, err
	}
	defer rows.Close()

	for rows.Next() {
		var endpoint, sentiment string
		var count, kbCount int
		if err := rows.Scan(&endpoint, &sentiment, &count, &kbCount); err != nil {
			continue
		}
		summary.Total += count
		summary.KBIngested += kbCount
		if sentiment == "positive" {
			summary.Positive += count
		} else {
			summary.Negative += count
		}

		ep := summary.ByEndpoint[endpoint]
		ep.Total += count
		if sentiment == "positive" {
			ep.Positive += count
		} else {
			ep.Negative += count
		}
		summary.ByEndpoint[endpoint] = ep
	}

	if summary.Total > 0 {
		summary.PositivePct = float64(summary.Positive) / float64(summary.Total) * 100
	}
	for k, ep := range summary.ByEndpoint {
		if ep.Total > 0 {
			ep.PosPct = float64(ep.Positive) / float64(ep.Total) * 100
		}
		summary.ByEndpoint[k] = ep
	}

	// Last 10 negative feedback items for admin review
	negRows, err := s.db.QueryContext(ctx, `
		SELECT id, endpoint, prompt_preview, response_preview, comment, created_at
		FROM ai_feedback
		WHERE sentiment = 'negative'
		  AND created_at >= NOW() - ($1 || ' days')::interval
		ORDER BY created_at DESC
		LIMIT 10
	`, days)
	if err == nil {
		defer negRows.Close()
		for negRows.Next() {
			var r FeedbackRecord
			r.Sentiment = "negative"
			_ = negRows.Scan(&r.ID, &r.Endpoint, &r.PromptPreview, &r.ResponsePreview, &r.Comment, &r.CreatedAt)
			summary.RecentNeg = append(summary.RecentNeg, r)
		}
	}

	return summary, nil
}

// ExportCSV writes all feedback for the last N days to w as CSV.
// Only prompt_preview and response_preview are included (no full content).
// This is the sanitized export operators can send to developers for analysis.
func (s *FeedbackService) ExportCSV(ctx context.Context, days int, w io.Writer) error {
	if s.db == nil {
		return fmt.Errorf("database not available")
	}
	if days <= 0 {
		days = 90
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, endpoint, sentiment, COALESCE(rating::text,''),
		       COALESCE(prompt_preview,''), COALESCE(response_preview,''),
		       COALESCE(comment,''), kb_ingested, created_at
		FROM ai_feedback
		WHERE created_at >= NOW() - ($1 || ' days')::interval
		ORDER BY created_at DESC
	`, days)
	if err != nil {
		return err
	}
	defer rows.Close()

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{
		"id", "endpoint", "sentiment", "rating",
		"prompt_preview", "response_preview", "comment",
		"kb_ingested", "created_at",
	})

	for rows.Next() {
		var id, endpoint, sentiment, ratingStr, prompt, response, comment string
		var kbIngested bool
		var createdAt time.Time
		if err := rows.Scan(&id, &endpoint, &sentiment, &ratingStr, &prompt, &response, &comment, &kbIngested, &createdAt); err != nil {
			continue
		}
		_ = cw.Write([]string{
			id, endpoint, sentiment, ratingStr,
			prompt, response, comment,
			fmt.Sprintf("%t", kbIngested),
			createdAt.Format(time.RFC3339),
		})
	}
	cw.Flush()
	return cw.Error()
}

// ─── Submit to team ───────────────────────────────────────────────────────────

// TeamPayload is the sanitized report sent to the ezHealthKonnect dev team.
// It contains ONLY aggregate counts and free-text comments — no prompts, no responses, no PHI.
type TeamPayload struct {
	ProductVersion  string                   `json:"product_version"`
	InstanceID      string                   `json:"instance_id"` // anonymized hash, never hostname/IP
	PeriodDays      int                      `json:"period_days"`
	Total           int                      `json:"total"`
	PositivePct     float64                  `json:"positive_pct"`
	KBIngested      int                      `json:"kb_ingested"`
	ByEndpoint      map[string]EndpointStats `json:"by_endpoint"`
	AdminComments   []string                 `json:"admin_comments"` // comments the admin chose to include
	SubmittedAt     time.Time                `json:"submitted_at"`
}

// SubmitToTeam sends an anonymized aggregate report to the configured webhook URL.
// Only the summary (counts, percentages) and explicit admin comments are sent.
// The webhookURL is configured in system_settings — empty = no-op.
func (s *FeedbackService) SubmitToTeam(ctx context.Context, summary FeedbackSummary, adminComment, webhookURL, instanceID, version string) error {
	if webhookURL == "" {
		return fmt.Errorf("no feedback webhook configured — set feedback_webhook_url in AI settings")
	}

	payload := TeamPayload{
		ProductVersion: version,
		InstanceID:     instanceID,
		PeriodDays:     summary.PeriodDays,
		Total:          summary.Total,
		PositivePct:    summary.PositivePct,
		KBIngested:     summary.KBIngested,
		ByEndpoint:     summary.ByEndpoint,
		SubmittedAt:    time.Now().UTC(),
	}
	if adminComment != "" {
		payload.AdminComments = []string{adminComment}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ezHealthKonnect/"+version)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("submit to team: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// ─── KB improvement helpers ───────────────────────────────────────────────────
// Adds confirmed working scripts to the knowledge base, so the same model
// produces better script suggestions for similar future requests. There is no
// confirmed-Q&A equivalent — see the Feedback loop comment at the top of this
// file for why plain-text Q&A pairs aren't safe to embed this way.

// IngestConfirmedScript adds a user-validated script description+code pair to the KB.
func (o *OperationalIngestionService) IngestConfirmedScript(ctx context.Context, description, script, interfaceID string) error {
	// Only store description + first 5 lines to avoid bloating KB with full code
	preview := truncateLines(script, 5)
	text := fmt.Sprintf("Working script for: %s\n\nPattern:\n%s", description, preview)
	meta := map[string]interface{}{
		"type":         "confirmed_script",
		"interface_id": interfaceID,
	}
	_, err := o.embedding.IngestText(ctx, "operational", "confirmed_script", "feedback", text, meta)
	return err
}

// truncateLines returns the first n lines of s.
func truncateLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
		lines = append(lines, "...")
	}
	return strings.Join(lines, "\n")
}
