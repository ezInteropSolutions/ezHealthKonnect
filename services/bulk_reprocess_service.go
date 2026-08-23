// services/bulk_reprocess_service.go
//
// BulkReprocessService is the background worker for the bulk_reprocess_jobs table
// (V213 migration). A job is created instantly by the Node.js API (POST
// /api/messages/bulk-reprocess) — this service is what actually does the work,
// polling for active jobs and processing them in small, checkpointed batches so a
// job of any size (a handful of messages, or millions) never blocks the request
// that created it and survives an app restart partway through.
//
// Architecture mirrors RetentionEnforcementService (same repo, same "goroutine loop
// started at boot" shape) and reuses services/backpressure.WorkerPool — the same
// bounded-concurrency primitive already used to stop unbounded goroutine spawning
// under load elsewhere in this codebase — rather than inventing a second one.
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lib/pq"

	"ezhealthkonnect/services/backpressure"
)

// MessageReprocessor is the one ProcessingEngine method this service needs.
// Declared here (accept an interface, not the concrete *processing.ProcessingEngine)
// rather than importing the processing package directly, which would create an
// import cycle: processing already imports services (engine.go uses
// services.LogCategoryConnection etc.), so services importing processing back is
// not possible. main.go passes the real *processing.ProcessingEngine in, which
// already satisfies this interface structurally — no wiring change needed there.
type MessageReprocessor interface {
	// ReprocessMessageSync blocks until the reprocess attempt completes (unlike
	// ReprocessMessage, which is fire-and-forget) — required so this service's own
	// bounded worker pool reflects real concurrency, not just how fast it can loop.
	ReprocessMessageSync(interfaceID, messageID string) error
}

// statusSQLMap mirrors controllers/MessageController.js's getMessages() statusMap.
// Kept as a small, flagged duplication rather than round-tripping a pre-resolved SQL
// fragment through the job's filter JSONB — the map is 6 entries and rarely changes;
// if it does, update both places.
var statusSQLMap = map[string]string{
	"received":         "status = 'received'",
	"processing":       "status IN ('processing','reprocessing')",
	"delivered":        "status IN ('processed','delivered') AND delivery_status = 'delivered'",
	"completed":        "status IN ('processed','delivered') AND delivery_status = 'not_required'",
	"pending_delivery": "status = 'processed' AND delivery_status = 'pending'",
	"failed":           "(status = 'failed' OR delivery_status = 'failed')",
}

// bulkErrorEntry is one row of a job's error_summary — best-effort recent-failure
// visibility, not a full audit trail (that's the source message's own
// last_error_message column, never dropped).
type bulkErrorEntry struct {
	MessageID string `json:"messageId"`
	Error     string `json:"error"`
}

const bulkErrorSummaryCap = 100

type bulkJob struct {
	ID               string
	InterfaceID      string
	TableName        string
	SelectionMode    string
	Filter           map[string]interface{}
	ExplicitIDs      []string
	BatchSize        int
	Concurrency      int
	ThrottlePerSec   int
	LastProcessedID  string
	ProcessedCount   int64
	ErrorSummary     []bulkErrorEntry
	CreatedAt        time.Time
}

// BulkReprocessService polls bulk_reprocess_jobs and processes active jobs.
type BulkReprocessService struct {
	db     *sql.DB
	engine MessageReprocessor
	pool   *backpressure.WorkerPool

	mu       sync.Mutex
	inFlight map[string]bool // job IDs currently owned by a goroutine in THIS process
}

// NewBulkReprocessService constructs the service. Dependencies (db, engine) are
// injected by the caller (main.go), not looked up globally.
func NewBulkReprocessService(db *sql.DB, engine MessageReprocessor) *BulkReprocessService {
	return &BulkReprocessService{
		db:     db,
		engine: engine,
		// A single shared pool for ALL bulk-reprocess work, deliberately separate
		// from the per-interface pools backpressureRegistry() hands out for live
		// inbound traffic — a huge bulk job must never starve real-time processing
		// for the same interface. maxQueueDepth (5000) is comfortably larger than
		// any single batch (batch_size defaults to 500), so Submit essentially never
		// needs to block/retry in normal operation.
		pool:     backpressure.Get().GetOrCreateWith("__bulk_reprocess__", 50, 5000),
		inFlight: make(map[string]bool),
	}
}

// Start launches the poll loop in a background goroutine. Same shape as
// RetentionEnforcementService.Start — ctx cancellation stops the loop.
func (s *BulkReprocessService) Start(ctx context.Context) {
	go s.run(ctx)
}

func (s *BulkReprocessService) run(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.claimAndDispatch(ctx)
		}
	}
}

// claimAndDispatch finds jobs that are queued or already running (the latter covers
// resuming a job after an app restart — its in-memory inFlight tracking is naturally
// empty on a fresh process, so a 'running' job with no live goroutine gets picked
// back up from its last checkpoint) and starts a per-job goroutine for any not
// already owned by this process instance.
func (s *BulkReprocessService) claimAndDispatch(ctx context.Context) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM bulk_reprocess_jobs WHERE status IN ('queued','running')`)
	if err != nil {
		log.Printf("⚠️  [BulkReprocess] poll query failed: %v", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()

	for _, id := range ids {
		s.mu.Lock()
		already := s.inFlight[id]
		if !already {
			s.inFlight[id] = true
		}
		s.mu.Unlock()
		if already {
			continue
		}
		go func(jobID string) {
			defer func() {
				s.mu.Lock()
				delete(s.inFlight, jobID)
				s.mu.Unlock()
			}()
			s.processJob(ctx, jobID)
		}(id)
	}
}

// processJob runs one job's full batch loop until it's exhausted, cancelled, or
// fails outright. Each iteration claims one batch, processes it through the bounded
// worker pool, checkpoints progress, and (if configured) paces itself before the
// next batch — never loading the entire match set into memory at once.
func (s *BulkReprocessService) processJob(ctx context.Context, jobID string) {
	job, err := s.loadJob(jobID)
	if err != nil {
		log.Printf("⚠️  [BulkReprocess] failed to load job %s: %v", jobID, err)
		return
	}
	if sanitizeTableName(job.TableName) == "" {
		s.failJob(jobID, "invalid table name")
		return
	}

	s.markRunning(jobID)
	log.Printf("🔄 [BulkReprocess] job %s starting (mode=%s, interface=%s)", jobID, job.SelectionMode, job.InterfaceID)

	for {
		cancelled, cErr := s.isCancelRequested(jobID)
		if cErr != nil {
			log.Printf("⚠️  [BulkReprocess] job %s: cancel check failed: %v", jobID, cErr)
		}
		if cancelled {
			s.setStatus(jobID, "cancelled")
			log.Printf("🛑 [BulkReprocess] job %s cancelled after %d processed", jobID, job.ProcessedCount)
			return
		}

		batchStart := time.Now()

		var ids []string
		var newCursor string
		if job.SelectionMode == "ids" {
			ids, newCursor = s.nextExplicitIDBatch(job)
		} else {
			ids, newCursor, err = s.nextFilterBatch(ctx, job)
			if err != nil {
				s.failJob(jobID, fmt.Sprintf("batch query failed: %v", err))
				return
			}
		}

		if len(ids) == 0 {
			s.setStatus(jobID, "completed")
			log.Printf("✅ [BulkReprocess] job %s completed — %d processed", jobID, job.ProcessedCount)
			return
		}

		succeeded, failed, batchErrors := s.runBatch(job, ids)

		job.LastProcessedID = newCursor
		job.ProcessedCount += int64(len(ids))
		job.ErrorSummary = capErrorSummary(append(job.ErrorSummary, batchErrors...))

		if err := s.recordProgress(jobID, int64(len(ids)), succeeded, failed, newCursor, job.ErrorSummary); err != nil {
			log.Printf("⚠️  [BulkReprocess] job %s: failed to record progress: %v", jobID, err)
		}

		if job.ThrottlePerSec > 0 {
			elapsed := time.Since(batchStart)
			want := time.Duration(float64(len(ids)) / float64(job.ThrottlePerSec) * float64(time.Second))
			if elapsed < want {
				time.Sleep(want - elapsed)
			}
		}
	}
}

// runBatch processes one batch through the bounded worker pool, blocking until every
// message in the batch has actually finished (not just been submitted) — that's what
// makes concurrency genuinely bounded, unlike calling ProcessingEngine.ReprocessMessage
// (async, fire-and-forget) in a loop would be.
func (s *BulkReprocessService) runBatch(job *bulkJob, ids []string) (succeeded, failed int64, errs []bulkErrorEntry) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, msgID := range ids {
		wg.Add(1)
		id := msgID
		s.submitBlocking(func() {
			defer wg.Done()
			reprocessErr := s.engine.ReprocessMessageSync(job.InterfaceID, id)

			// Authoritative success/failure comes from the message row's own status,
			// not reprocessErr — the pipeline writes 'failed' directly to the row
			// (including its own panic-recovery path) once parsing succeeds and
			// execution moves past ReprocessMessageSync's synchronous return, so a
			// nil error here does not by itself guarantee the pipeline succeeded.
			ok, statusErrMsg := s.messageOutcome(job.TableName, id)
			if ok {
				atomic.AddInt64(&succeeded, 1)
				return
			}
			atomic.AddInt64(&failed, 1)
			errMsg := statusErrMsg
			if errMsg == "" && reprocessErr != nil {
				errMsg = reprocessErr.Error()
			}
			mu.Lock()
			errs = append(errs, bulkErrorEntry{MessageID: id, Error: errMsg})
			mu.Unlock()
		})
	}
	wg.Wait()
	return
}

// submitBlocking retries Submit until it succeeds — the pool's queue is sized well
// above any single batch, so this should rarely spin, but a bulk job must never
// silently drop a message it claimed.
func (s *BulkReprocessService) submitBlocking(fn func()) {
	for !s.pool.Submit(fn) {
		time.Sleep(50 * time.Millisecond)
	}
}

// messageOutcome reads the message row's own status/last_error_message after a
// reprocess attempt — the single source of truth the rest of the app already uses
// (e.g. the "Failed" badge on the Messages list).
func (s *BulkReprocessService) messageOutcome(tableName, messageID string) (succeeded bool, errMsg string) {
	table := sanitizeTableName(tableName)
	if table == "" {
		return false, "invalid table name"
	}
	var status string
	var lastErr sql.NullString
	err := s.db.QueryRow(
		fmt.Sprintf(`SELECT status, last_error_message FROM %s WHERE message_id = $1`, table),
		messageID,
	).Scan(&status, &lastErr)
	if err != nil {
		return false, fmt.Sprintf("could not read outcome: %v", err)
	}
	if status == "failed" {
		return false, lastErr.String
	}
	return true, ""
}

// nextFilterBatch claims the next batch for a filter-mode job via keyset pagination
// (WHERE id > last_processed_id ORDER BY id LIMIT batch_size) — never OFFSET, so
// resuming stays O(batch_size) no matter how deep into a huge job the checkpoint is.
// Also pins the match set to a snapshot as of job creation (received_at <= job's
// created_at) so it never drifts from the count the UI showed when the job started,
// and so the "ORDER BY id" traversal (UUIDs, not chronological) is guaranteed complete
// over a fixed set rather than chasing a moving target.
func (s *BulkReprocessService) nextFilterBatch(ctx context.Context, job *bulkJob) (messageIDs []string, lastID string, err error) {
	table := sanitizeTableName(job.TableName)
	if table == "" {
		return nil, "", fmt.Errorf("invalid table name")
	}

	conditions := []string{"1=1"}
	args := []interface{}{}
	argN := 1

	if statusVal, ok := job.Filter["status"].(string); ok && statusVal != "" {
		if sqlCond, known := statusSQLMap[statusVal]; known {
			conditions = append(conditions, sqlCond)
		}
	}
	if mt, ok := job.Filter["messageType"].(string); ok && mt != "" {
		conditions = append(conditions, fmt.Sprintf("message_type ILIKE $%d", argN))
		args = append(args, "%"+mt+"%")
		argN++
	}
	if from, ok := job.Filter["dateFrom"].(string); ok && from != "" {
		conditions = append(conditions, fmt.Sprintf("received_at >= $%d", argN))
		args = append(args, from)
		argN++
	}
	if to, ok := job.Filter["dateTo"].(string); ok && to != "" {
		conditions = append(conditions, fmt.Sprintf("received_at <= $%d", argN))
		args = append(args, to)
		argN++
	}

	conditions = append(conditions, fmt.Sprintf("received_at <= $%d", argN))
	args = append(args, job.CreatedAt)
	argN++

	if job.LastProcessedID != "" {
		conditions = append(conditions, fmt.Sprintf("id > $%d", argN))
		args = append(args, job.LastProcessedID)
		argN++
	}

	args = append(args, job.BatchSize)
	query := fmt.Sprintf(
		`SELECT id, message_id FROM %s WHERE %s ORDER BY id LIMIT $%d`,
		table, strings.Join(conditions, " AND "), argN,
	)

	rows, qErr := s.db.QueryContext(ctx, query, args...)
	if qErr != nil {
		return nil, "", qErr
	}
	defer rows.Close()

	lastID = job.LastProcessedID
	for rows.Next() {
		var id, messageID string
		if scanErr := rows.Scan(&id, &messageID); scanErr != nil {
			continue
		}
		messageIDs = append(messageIDs, messageID)
		lastID = id
	}
	return messageIDs, lastID, rows.Err()
}

// nextExplicitIDBatch claims the next slice of an 'ids'-mode job's already-materialized
// list — ProcessedCount doubles as the resume cursor, no separate checkpoint needed.
func (s *BulkReprocessService) nextExplicitIDBatch(job *bulkJob) (batch []string, cursor string) {
	start := int(job.ProcessedCount)
	if start >= len(job.ExplicitIDs) {
		return nil, job.LastProcessedID
	}
	end := start + job.BatchSize
	if end > len(job.ExplicitIDs) {
		end = len(job.ExplicitIDs)
	}
	return job.ExplicitIDs[start:end], job.LastProcessedID
}

func capErrorSummary(entries []bulkErrorEntry) []bulkErrorEntry {
	if len(entries) <= bulkErrorSummaryCap {
		return entries
	}
	return entries[len(entries)-bulkErrorSummaryCap:]
}

// ---------------------------------------------------------------------------
// DB helpers
// ---------------------------------------------------------------------------

func (s *BulkReprocessService) loadJob(jobID string) (*bulkJob, error) {
	var j bulkJob
	var filterBytes, errorSummaryBytes []byte
	var explicitIDs []string
	var lastProcessedID sql.NullString

	err := s.db.QueryRow(`
		SELECT interface_id, table_name, selection_mode, filter, explicit_ids,
		       batch_size, concurrency, COALESCE(throttle_per_sec, 0),
		       COALESCE(last_processed_id, ''), processed_count, error_summary, created_at
		FROM bulk_reprocess_jobs WHERE id = $1
	`, jobID).Scan(
		&j.InterfaceID, &j.TableName, &j.SelectionMode, &filterBytes, pq.Array(&explicitIDs),
		&j.BatchSize, &j.Concurrency, &j.ThrottlePerSec,
		&lastProcessedID, &j.ProcessedCount, &errorSummaryBytes, &j.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	j.ID = jobID
	j.ExplicitIDs = explicitIDs
	j.LastProcessedID = lastProcessedID.String

	if len(filterBytes) > 0 {
		_ = json.Unmarshal(filterBytes, &j.Filter)
	}
	if j.Filter == nil {
		j.Filter = map[string]interface{}{}
	}
	if len(errorSummaryBytes) > 0 {
		_ = json.Unmarshal(errorSummaryBytes, &j.ErrorSummary)
	}
	return &j, nil
}

func (s *BulkReprocessService) markRunning(jobID string) {
	_, err := s.db.Exec(
		`UPDATE bulk_reprocess_jobs SET status = 'running', started_at = COALESCE(started_at, NOW()) WHERE id = $1 AND status IN ('queued','running')`,
		jobID,
	)
	if err != nil {
		log.Printf("⚠️  [BulkReprocess] job %s: failed to mark running: %v", jobID, err)
	}
}

func (s *BulkReprocessService) isCancelRequested(jobID string) (bool, error) {
	var cancelled bool
	err := s.db.QueryRow(`SELECT cancel_requested FROM bulk_reprocess_jobs WHERE id = $1`, jobID).Scan(&cancelled)
	return cancelled, err
}

// setStatus is only ever called with a TERMINAL status ('completed' or
// 'cancelled' — see processJob's two call sites) so completed_at is
// unconditionally NOW(), no CASE needed. An earlier version reused $1 both as
// a plain assignment (status = $1) and inside a CASE WHEN $1 IN (...) — lib/pq
// can't unify the inferred type across those two shapes in one prepared
// statement ("pq: inconsistent types deduced for parameter $1"), which failed
// silently (only logged) on every call. Because the status column never
// actually flipped to 'completed', claimAndDispatch kept re-claiming the job
// every poll forever — a real infinite loop, not just a cosmetic bug — caught
// by watching the live app.log, not by reasoning about the SQL alone.
func (s *BulkReprocessService) setStatus(jobID, status string) {
	_, err := s.db.Exec(
		`UPDATE bulk_reprocess_jobs SET status = $1, completed_at = NOW() WHERE id = $2`,
		status, jobID,
	)
	if err != nil {
		log.Printf("⚠️  [BulkReprocess] job %s: failed to set status=%s: %v", jobID, status, err)
	}
}

func (s *BulkReprocessService) failJob(jobID, reason string) {
	log.Printf("❌ [BulkReprocess] job %s failed: %s", jobID, reason)
	entry := bulkErrorEntry{MessageID: "", Error: reason}
	b, _ := json.Marshal([]bulkErrorEntry{entry})
	_, err := s.db.Exec(
		`UPDATE bulk_reprocess_jobs SET status = 'failed', completed_at = NOW(), error_summary = $1 WHERE id = $2`,
		b, jobID,
	)
	if err != nil {
		log.Printf("⚠️  [BulkReprocess] job %s: failed to persist failure reason: %v", jobID, err)
	}
}

func (s *BulkReprocessService) recordProgress(jobID string, batchCount, succeeded, failed int64, lastProcessedID string, errorSummary []bulkErrorEntry) error {
	errBytes, _ := json.Marshal(errorSummary)
	_, err := s.db.Exec(`
		UPDATE bulk_reprocess_jobs
		SET processed_count = processed_count + $1,
		    succeeded_count = succeeded_count + $2,
		    failed_count = failed_count + $3,
		    last_processed_id = $4,
		    error_summary = $5,
		    updated_at = NOW()
		WHERE id = $6
	`, batchCount, succeeded, failed, lastProcessedID, errBytes, jobID)
	return err
}
