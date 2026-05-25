// services/connectors/dlq_service.go
//
// DLQService manages the dead-letter queue for failed outbound deliveries.
//
// Write side: the OutboundConnectorExecutor calls WriteToDLQ after delivery
// failure. A partial unique index (idx_dlq_unique_active) prevents duplicate
// active rows for the same (message_id, interface_id) pair — subsequent
// failures from retries upsert instead of inserting a new row.
//
// Poll side: StartPoller runs a goroutine every 30 s that picks up pending rows
// and redrives them through the pipeline service via ExecuteRedrive. FOR UPDATE
// SKIP LOCKED prevents double-processing under horizontal scale.
//
// Redrive modes:
//   - from_failed_step: calls pipelineSvc.ExecuteFromStep with pipeline_data_snapshot
//   - from_start:       calls pipelineSvc.ExecuteTransformation with pipeline_input_snapshot
package connectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"
)

// InterfaceDLQConfig holds the per-interface recovery queue settings read from
// interfaces.dlq_config. Zero values indicate "use compiled-in default".
type InterfaceDLQConfig struct {
	MaxAttempts       int     `json:"max_attempts"`
	InitialDelayS     int     `json:"initial_delay_s"`
	RetryDelayS       int     `json:"retry_delay_s"`
	BackoffMultiplier float64 `json:"backoff_multiplier"`
	ExpiresAfterHours int     `json:"expires_after_hours"`
}

// defaults returns a copy with zero fields filled from compiled-in defaults.
func (c InterfaceDLQConfig) defaults() InterfaceDLQConfig {
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 10
	}
	if c.InitialDelayS <= 0 {
		c.InitialDelayS = 30
	}
	if c.RetryDelayS <= 0 {
		c.RetryDelayS = 60
	}
	if c.BackoffMultiplier <= 0 {
		c.BackoffMultiplier = 2.0
	}
	return c
}

// nextRetryDelay returns the delay for the nth attempt (0-indexed) using
// exponential backoff: retryDelay * multiplier^attempt, capped at 24 h.
func (c InterfaceDLQConfig) nextRetryDelay(attempt int) time.Duration {
	cfg := c.defaults()
	delay := float64(cfg.RetryDelayS) * math.Pow(cfg.BackoffMultiplier, float64(attempt))
	if delay > 86400 {
		delay = 86400 // cap at 24 h
	}
	return time.Duration(delay) * time.Second
}

// PipelineRedriver is the interface the DLQService calls to redrive a message.
// TransformationPipelineService satisfies this interface.
type PipelineRedriver interface {
	ExecuteTransformationFromInput(ctx context.Context, messageID, interfaceID, messageType string, input map[string]interface{}) error
	ExecuteFromStep(ctx context.Context, pipelineID, fromStepID string, data map[string]interface{}) error
}

// WriteDLQParams captures everything needed to record an outbound failure.
type WriteDLQParams struct {
	MessageID             string
	InterfaceID           string
	PipelineID            string
	FailedStepID          string
	StepName              string
	ConnectorType         string
	Payload               string
	ContentType           string
	ErrorMessage          string
	AttemptCount          int
	RedriveMode           string // "from_failed_step" | "from_start"
	PipelineInputSnapshot map[string]interface{}
	PipelineDataSnapshot  map[string]interface{}
	ExpiresAt             *time.Time
}

// DLQRow is the projection returned to callers for display and redrive decisions.
type DLQRow struct {
	ID                    string
	MessageID             string
	InterfaceID           string
	PipelineID            string
	FailedStepID          string
	StepName              string
	ConnectorType         string
	Payload               string
	ContentType           string
	ErrorMessage          string
	AttemptCount          int
	Status                string
	RedriveMode           string
	NextRetryAt           *time.Time
	ExpiresAt             *time.Time
	CreatedAt             time.Time
	PipelineInputSnapshot map[string]interface{}
	PipelineDataSnapshot  map[string]interface{}
}

// DLQService writes failed deliveries to delivery_dlq and manages their lifecycle.
type DLQService struct {
	db          *sql.DB
	pipelineSvc PipelineRedriver // set by SetPipelineRedriver; used for auto-poller and manual redrive
}

// NewDLQService creates a DLQService backed by the given DB connection.
func NewDLQService(db *sql.DB) *DLQService {
	return &DLQService{db: db}
}

// SetPipelineRedriver wires the pipeline service so ExecuteRedrive and the auto-poller
// can redrive messages without the caller providing it on each call.
func (s *DLQService) SetPipelineRedriver(svc PipelineRedriver) {
	s.pipelineSvc = svc
}

// DLQServicer is the interface controllers and tests use to access DLQ operations.
// *DLQService satisfies this interface automatically.
type DLQServicer interface {
	ListPending(ctx context.Context, interfaceID, messageID, status string, limit, offset int) ([]*DLQRow, error)
	Stats(ctx context.Context) (map[string]int, error)
	InterfaceStats(ctx context.Context, interfaceID string) (map[string]int, error)
	Get(ctx context.Context, dlqID string) (*DLQRow, error)
	ExecuteRedrive(ctx context.Context, dlqID, modeOverride string, pipelineSvc PipelineRedriver) error
	ScheduleRedrive(ctx context.Context, dlqID, mode string, at time.Time) error
	BulkSchedule(ctx context.Context, dlqIDs []string, mode string, at time.Time) (int64, error)
	Abandon(ctx context.Context, dlqID string) error
	BulkAbandon(ctx context.Context, dlqIDs []string) (int64, error)
}

// getInterfaceConfig reads dlq_config from the interfaces table for the given
// interface ID. Returns compiled-in defaults if the row has no dlq_config.
func (s *DLQService) getInterfaceConfig(ctx context.Context, interfaceID string) InterfaceDLQConfig {
	if s.db == nil || interfaceID == "" {
		return InterfaceDLQConfig{}.defaults()
	}
	var raw sql.NullString
	_ = s.db.QueryRowContext(ctx,
		`SELECT dlq_config FROM interfaces WHERE id = $1`, interfaceID,
	).Scan(&raw)
	if !raw.Valid || raw.String == "" || raw.String == "null" {
		return InterfaceDLQConfig{}.defaults()
	}
	var cfg InterfaceDLQConfig
	if err := json.Unmarshal([]byte(raw.String), &cfg); err != nil {
		return InterfaceDLQConfig{}.defaults()
	}
	return cfg.defaults()
}

// WriteToDLQ records a delivery failure.
// If an active row (status pending/retrying) already exists for (message_id, interface_id),
// it is updated in place rather than inserting a duplicate.
func (s *DLQService) WriteToDLQ(ctx context.Context, p WriteDLQParams) error {
	if p.RedriveMode == "" {
		p.RedriveMode = "from_failed_step"
	}
	cfg := s.getInterfaceConfig(ctx, p.InterfaceID)
	nextRetryAt := time.Now().Add(time.Duration(cfg.InitialDelayS) * time.Second)

	// Set expiry from interface config if not explicitly provided
	if p.ExpiresAt == nil && cfg.ExpiresAfterHours > 0 {
		exp := time.Now().Add(time.Duration(cfg.ExpiresAfterHours) * time.Hour)
		p.ExpiresAt = &exp
	}

	inputJSON, _ := json.Marshal(p.PipelineInputSnapshot)
	dataJSON, _ := json.Marshal(p.PipelineDataSnapshot)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO delivery_dlq
			(message_id, interface_id, pipeline_id, failed_step_id, step_name,
			 connector_type, payload, content_type, error_message,
			 attempt_count, last_attempt_at, next_retry_at, status,
			 redrive_mode, pipeline_input_snapshot, pipeline_data_snapshot, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW(),$11,'pending',$12,$13,$14,$15)
		ON CONFLICT (message_id, interface_id) WHERE status IN ('pending', 'retrying')
		DO UPDATE SET
			error_message          = EXCLUDED.error_message,
			attempt_count          = delivery_dlq.attempt_count + 1,
			last_attempt_at        = NOW(),
			next_retry_at          = EXCLUDED.next_retry_at,
			pipeline_data_snapshot = EXCLUDED.pipeline_data_snapshot,
			redrive_mode           = EXCLUDED.redrive_mode,
			status                 = 'pending'`,
		p.MessageID, p.InterfaceID, nullableStr(p.PipelineID), nullableStr(p.FailedStepID), nullableStr(p.StepName),
		nullableStr(p.ConnectorType), p.Payload, nullableStr(p.ContentType), p.ErrorMessage,
		p.AttemptCount, nextRetryAt,
		p.RedriveMode, string(inputJSON), string(dataJSON), p.ExpiresAt,
	)
	return err
}

// Resolve marks a DLQ row as successfully delivered.
func (s *DLQService) Resolve(ctx context.Context, dlqID string) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE delivery_dlq
		SET status='resolved', resolved_at=NOW()
		WHERE id=$1`, dlqID)
	return err
}

// Abandon permanently marks a DLQ row as abandoned (no further retries).
func (s *DLQService) Abandon(ctx context.Context, dlqID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE delivery_dlq
		SET status='abandoned', resolved_at=NOW()
		WHERE id=$1`, dlqID)
	return err
}

// Fail increments the attempt count, applies exponential backoff from the
// interface's dlq_config, and flips to abandoned once maxAttempts is reached.
// The legacy maxAttempts/nextRetryDelay parameters are ignored — interface config wins.
func (s *DLQService) Fail(ctx context.Context, dlqID, errMsg string, _ int, _ time.Duration) error {
	if s.db == nil {
		return nil
	}
	// Fetch the row to get interface_id and current attempt_count
	var interfaceID string
	var attempt int
	err := s.db.QueryRowContext(ctx,
		`SELECT interface_id, attempt_count FROM delivery_dlq WHERE id = $1`, dlqID,
	).Scan(&interfaceID, &attempt)
	if err != nil {
		return err
	}
	cfg := s.getInterfaceConfig(ctx, interfaceID)
	newAttempt := attempt + 1
	nextDelay := cfg.nextRetryDelay(newAttempt)

	_, err = s.db.ExecContext(ctx, `
		UPDATE delivery_dlq
		SET attempt_count   = $2,
		    error_message   = $3,
		    last_attempt_at = NOW(),
		    next_retry_at   = CASE WHEN $2 >= $4 THEN NULL
		                          ELSE NOW() + ($5::text || ' seconds')::interval END,
		    status          = CASE WHEN $2 >= $4 THEN 'abandoned' ELSE 'pending' END
		WHERE id = $1`,
		dlqID, newAttempt, errMsg, cfg.MaxAttempts, int(nextDelay.Seconds()),
	)
	return err
}

// Get retrieves a single DLQ row by ID.
func (s *DLQService) Get(ctx context.Context, dlqID string) (*DLQRow, error) {
	rows, err := s.queryRows(ctx, `WHERE id = $1`, dlqID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("dlq row %s not found", dlqID)
	}
	return rows[0], nil
}

// ListPending returns DLQ rows matching an optional status filter (empty = all).
func (s *DLQService) ListPending(ctx context.Context, interfaceID, messageID, status string, limit, offset int) ([]*DLQRow, error) {
	where := `WHERE 1=1`
	args := []interface{}{}
	argN := 1

	if interfaceID != "" {
		where += fmt.Sprintf(" AND interface_id = $%d", argN)
		args = append(args, interfaceID)
		argN++
	}
	if messageID != "" {
		where += fmt.Sprintf(" AND message_id = $%d", argN)
		args = append(args, messageID)
		argN++
	}
	if status != "" {
		where += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, status)
		argN++
	}
	where += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argN, argN+1)
	args = append(args, limit, offset)

	return s.queryRows(ctx, where, args...)
}

// Stats returns aggregate counts grouped by status.
func (s *DLQService) Stats(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM delivery_dlq GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var st string
		var cnt int
		if err := rows.Scan(&st, &cnt); err != nil {
			continue
		}
		out[st] = cnt
	}
	return out, nil
}

// ExecuteRedrive triggers a redrive of the given DLQ row through the pipeline service.
// modeOverride overrides the row's stored redrive_mode if non-empty.
// pipelineSvc overrides s.pipelineSvc if non-nil; if both are nil, returns an error.
func (s *DLQService) ExecuteRedrive(ctx context.Context, dlqID, modeOverride string, pipelineSvc PipelineRedriver) error {
	svc := pipelineSvc
	if svc == nil {
		svc = s.pipelineSvc
	}
	if svc == nil {
		return fmt.Errorf("dlq redrive: no pipeline redriver configured")
	}

	row, err := s.Get(ctx, dlqID)
	if err != nil {
		return fmt.Errorf("dlq redrive: %w", err)
	}
	if row.Status == "resolved" {
		return fmt.Errorf("dlq row %s is already resolved — use schedule to reactivate abandoned rows", dlqID)
	}

	mode := row.RedriveMode
	if modeOverride != "" {
		mode = modeOverride
	}

	if _, err := s.db.ExecContext(ctx, `UPDATE delivery_dlq SET status='retrying', last_attempt_at=NOW() WHERE id=$1`, dlqID); err != nil {
		return fmt.Errorf("dlq mark retrying: %w", err)
	}

	var redriveErr error
	switch mode {
	case "from_start":
		if row.PipelineInputSnapshot == nil {
			redriveErr = fmt.Errorf("from_start requested but pipeline_input_snapshot is empty")
		} else {
			redriveErr = svc.ExecuteTransformationFromInput(ctx, row.MessageID, row.InterfaceID, "", row.PipelineInputSnapshot)
		}
	default: // from_failed_step
		if row.PipelineID == "" || row.FailedStepID == "" {
			redriveErr = fmt.Errorf("from_failed_step requested but pipeline_id or failed_step_id is empty")
		} else {
			data := row.PipelineDataSnapshot
			if data == nil {
				data = map[string]interface{}{}
			}
			redriveErr = svc.ExecuteFromStep(ctx, row.PipelineID, row.FailedStepID, data)
		}
	}

	if redriveErr != nil {
		return s.Fail(ctx, dlqID, redriveErr.Error(), 10, 60*time.Second)
	}
	return s.Resolve(ctx, dlqID)
}

// ScheduleRedrive reactivates any row (including abandoned) and queues it for
// redrive at the specified time. attempt_count resets to 0 so the full retry
// budget is available again. mode overrides the stored redrive_mode if non-empty.
func (s *DLQService) ScheduleRedrive(ctx context.Context, dlqID, mode string, at time.Time) error {
	setMode := ""
	if mode != "" {
		setMode = ", redrive_mode = '" + mode + "'"
	}
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		UPDATE delivery_dlq
		SET status          = 'pending',
		    next_retry_at   = $2,
		    attempt_count   = 0,
		    last_attempt_at = NOW(),
		    resolved_at     = NULL
		    %s
		WHERE id = $1`, setMode),
		dlqID, at,
	)
	return err
}

// BulkSchedule reactivates multiple rows and schedules them for redrive at the
// given time. Returns the number of rows updated.
func (s *DLQService) BulkSchedule(ctx context.Context, dlqIDs []string, mode string, at time.Time) (int64, error) {
	if len(dlqIDs) == 0 {
		return 0, nil
	}
	placeholders := make([]byte, 0, len(dlqIDs)*5)
	args := make([]interface{}, 0, len(dlqIDs)+2)
	args = append(args, at) // $1
	for i, id := range dlqIDs {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, []byte(fmt.Sprintf("$%d", i+2))...)
		args = append(args, id)
	}
	setMode := ""
	if mode != "" {
		setMode = ", redrive_mode = '" + mode + "'"
	}
	res, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		UPDATE delivery_dlq
		SET status          = 'pending',
		    next_retry_at   = $1,
		    attempt_count   = 0,
		    last_attempt_at = NOW(),
		    resolved_at     = NULL
		    %s
		WHERE id IN (%s)`, setMode, placeholders),
		args...,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// InterfaceStats returns DLQ counts per status for a specific interface.
func (s *DLQService) InterfaceStats(ctx context.Context, interfaceID string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM delivery_dlq WHERE interface_id = $1 GROUP BY status`,
		interfaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var cnt int
		if err := rows.Scan(&st, &cnt); err != nil {
			continue
		}
		out[st] = cnt
	}
	return out, nil
}

// BulkAbandon marks multiple DLQ rows as abandoned atomically.
func (s *DLQService) BulkAbandon(ctx context.Context, dlqIDs []string) (int64, error) {
	if len(dlqIDs) == 0 {
		return 0, nil
	}
	placeholders := make([]byte, 0, len(dlqIDs)*5)
	args := make([]interface{}, len(dlqIDs))
	for i, id := range dlqIDs {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, []byte(fmt.Sprintf("$%d", i+1))...)
		args[i] = id
	}
	res, err := s.db.ExecContext(ctx,
		fmt.Sprintf(`UPDATE delivery_dlq SET status='abandoned', resolved_at=NOW() WHERE id IN (%s)`, placeholders),
		args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// StartPoller starts a background goroutine that polls the DLQ every 30 s and
// redrives pending rows through pipelineSvc. Stops when ctx is cancelled.
func (s *DLQService) StartPoller(ctx context.Context, pipelineSvc PipelineRedriver) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.pollAndRedrive(ctx, pipelineSvc)
			}
		}
	}()
	log.Printf("[dlq] poller started (30s interval)")
}

// pollAndRedrive fetches up to 20 pending rows and redrives each in its own goroutine.
func (s *DLQService) pollAndRedrive(ctx context.Context, pipelineSvc PipelineRedriver) {
	rows, err := s.db.QueryContext(ctx, `
		UPDATE delivery_dlq
		SET status = 'retrying'
		WHERE id IN (
			SELECT id FROM delivery_dlq
			WHERE status = 'pending'
			  AND next_retry_at <= NOW()
			  AND (expires_at IS NULL OR expires_at > NOW())
			ORDER BY next_retry_at
			LIMIT 20
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, message_id, interface_id, pipeline_id, failed_step_id, step_name,
		          connector_type, payload, content_type, error_message, attempt_count,
		          redrive_mode, pipeline_input_snapshot, pipeline_data_snapshot`)
	if err != nil {
		log.Printf("[dlq] poll error: %v", err)
		return
	}
	defer rows.Close()

	var picked []*DLQRow
	for rows.Next() {
		r, scanErr := scanDLQRow(rows)
		if scanErr != nil {
			log.Printf("[dlq] scan error: %v", scanErr)
			continue
		}
		picked = append(picked, r)
	}
	if len(picked) == 0 {
		return
	}
	log.Printf("[dlq] redriving %d message(s)", len(picked))
	for _, row := range picked {
		go func(r *DLQRow) {
			if err := s.redriveRow(ctx, r, pipelineSvc); err != nil {
				log.Printf("[dlq] redrive failed id=%s: %v", r.ID, err)
				_ = s.Fail(ctx, r.ID, err.Error(), 10, 60*time.Second)
			}
		}(row)
	}
}

// redriveRow executes redrive and resolves/fails the row.
func (s *DLQService) redriveRow(ctx context.Context, row *DLQRow, pipelineSvc PipelineRedriver) error {
	var redriveErr error
	switch row.RedriveMode {
	case "from_start":
		if row.PipelineInputSnapshot == nil {
			return fmt.Errorf("from_start: pipeline_input_snapshot is empty")
		}
		redriveErr = pipelineSvc.ExecuteTransformationFromInput(ctx, row.MessageID, row.InterfaceID, "", row.PipelineInputSnapshot)
	default: // from_failed_step
		if row.PipelineID == "" || row.FailedStepID == "" {
			return fmt.Errorf("from_failed_step: pipeline_id or failed_step_id is empty")
		}
		data := row.PipelineDataSnapshot
		if data == nil {
			data = map[string]interface{}{}
		}
		redriveErr = pipelineSvc.ExecuteFromStep(ctx, row.PipelineID, row.FailedStepID, data)
	}
	if redriveErr != nil {
		return redriveErr
	}
	return s.Resolve(ctx, row.ID)
}

// queryRows is a shared query helper used by Get and ListPending.
func (s *DLQService) queryRows(ctx context.Context, whereClause string, args ...interface{}) ([]*DLQRow, error) {
	query := fmt.Sprintf(`
		SELECT id, message_id, interface_id,
		       COALESCE(pipeline_id::text, ''), COALESCE(failed_step_id, ''), COALESCE(step_name, ''),
		       COALESCE(connector_type, ''), COALESCE(payload, ''), COALESCE(content_type, ''),
		       COALESCE(error_message, ''), attempt_count, status,
		       COALESCE(redrive_mode, 'from_failed_step'),
		       next_retry_at, expires_at, created_at,
		       pipeline_input_snapshot, pipeline_data_snapshot
		FROM delivery_dlq %s`, whereClause)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*DLQRow
	for rows.Next() {
		r, scanErr := scanDLQRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, r)
	}
	return result, nil
}

// rowScanner is satisfied by both *sql.Rows and *sql.Row.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanDLQRow(row rowScanner) (*DLQRow, error) {
	r := &DLQRow{}
	var inputJSON, dataJSON sql.NullString
	err := row.Scan(
		&r.ID, &r.MessageID, &r.InterfaceID,
		&r.PipelineID, &r.FailedStepID, &r.StepName,
		&r.ConnectorType, &r.Payload, &r.ContentType,
		&r.ErrorMessage, &r.AttemptCount, &r.Status,
		&r.RedriveMode,
		&r.NextRetryAt, &r.ExpiresAt, &r.CreatedAt,
		&inputJSON, &dataJSON,
	)
	if err != nil {
		return nil, err
	}
	if inputJSON.Valid && inputJSON.String != "" && inputJSON.String != "null" {
		_ = json.Unmarshal([]byte(inputJSON.String), &r.PipelineInputSnapshot)
	}
	if dataJSON.Valid && dataJSON.String != "" && dataJSON.String != "null" {
		_ = json.Unmarshal([]byte(dataJSON.String), &r.PipelineDataSnapshot)
	}
	return r, nil
}

func nullableStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
