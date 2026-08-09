// services/cda_coverage/worker_pool.go
//
// Bounded, isolated background lane for CDA Coverage Audit report building —
// deliberately separate from the live message-processing goroutines so a
// burst of coverage-audit work can never compete with real delivery
// throughput. Sized via CDA_COVERAGE_AUDIT_WORKERS, mirroring
// MAX_CDA_SECTION_WORKERS' env-var-configurable pattern in
// services/parsers/cda/cda_parser_service.go.
//
// Submit() is a non-blocking channel send: if the queue is full, the job is
// dropped and logged rather than ever blocking OutboundConnectorExecutor.Execute
// — this feature is observability, not a delivery gate (see CLAUDE.md's CDA
// Coverage Audit section once this lands), so losing an occasional report
// under extreme backpressure is the correct tradeoff, not a bug to fix by
// adding backpressure onto delivery.
package cdacoverage

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"runtime"
	"strconv"
	"time"

	cdaSchema "ezhealthkonnect/cda"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services"
	"ezhealthkonnect/services/executors"
	"ezhealthkonnect/services/storage"
)

const (
	defaultQueueSize        = 500
	jobTimeout              = 30 * time.Second
	defaultCooldownMinutes  = 60
	notifyDispatchTimeout   = 30 * time.Second
)

// WorkerPool runs CDA Coverage Audit report-building jobs off the critical
// delivery path.
type WorkerPool struct {
	db              *sql.DB
	objStorage      *storage.ObjectStorageService
	schemaLoader    *cdaSchema.CDASchemaLoader
	notificationSvc *services.AlertNotificationService
	jobs            chan models.CoverageAuditJob
}

// NewWorkerPool constructs and starts the pool. objStorage is where parsed
// CDA content (ParsedJSON, including the "xml" fidelity mirror) is fetched
// from at report-build time — the same store/lookup message_content_controller.go's
// GetParsedContent handler already uses, not a new persistence path.
func NewWorkerPool(db *sql.DB, objStorage *storage.ObjectStorageService, schemaLoader *cdaSchema.CDASchemaLoader) *WorkerPool {
	p := &WorkerPool{
		db:           db,
		objStorage:   objStorage,
		schemaLoader: schemaLoader,
		jobs:         make(chan models.CoverageAuditJob, resolveQueueSize()),
	}
	for i := 0; i < resolveWorkerCount(); i++ {
		go p.run()
	}
	return p
}

// SetNotificationService wires in external notification dispatch (email/
// webhook/Slack via the existing alert_channels system) for interfaces that
// have opted into it via cda_coverage_audit_config.notify. Until this is
// called, maybeNotify is a no-op — a nil notificationSvc means gaps are
// still recorded and shown on the message and messages-list, just never
// pushed anywhere externally.
func (p *WorkerPool) SetNotificationService(svc *services.AlertNotificationService) {
	p.notificationSvc = svc
}

// Submit enqueues job for background processing. Returns false (dropped,
// logged) if the queue is full — never blocks the caller.
func (p *WorkerPool) Submit(job models.CoverageAuditJob) bool {
	select {
	case p.jobs <- job:
		return true
	default:
		log.Printf("⚠️  [cda_coverage] queue full — dropping coverage audit for message=%s destination=%s", job.MessageID, job.Destination)
		return false
	}
}

func (p *WorkerPool) run() {
	for job := range p.jobs {
		p.process(job)
	}
}

func (p *WorkerPool) process(job models.CoverageAuditJob) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚠️  [cda_coverage] recovered panic building report for message=%s: %v", job.MessageID, r)
		}
	}()

	tracker, ok := job.Tracker.(*executors.CDACoverageTracker)
	if !ok || tracker == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()

	parsed, err := p.objStorage.GetParsedContent(ctx, job.InterfaceID, job.MessageID)
	if err != nil {
		log.Printf("⚠️  [cda_coverage] could not load parsed content for message=%s: %v", job.MessageID, err)
		return
	}
	xmlMirror, _ := parsed["xml"].(map[string]interface{})
	if xmlMirror == nil {
		return // not a CDA message, or parse didn't reach the fidelity mirror — nothing to audit
	}

	elementLevel := p.resolveElementGranularity(ctx, job.InterfaceID)
	inventory := BuildInventoryWithGranularity(xmlMirror, p.schemaLoader, elementLevel)
	report := BuildReport(inventory, tracker.Snapshot())

	if err := SaveReport(ctx, p.db, job, report); err != nil {
		log.Printf("⚠️  [cda_coverage] failed to save report for message=%s: %v", job.MessageID, err)
	}

	if report.HasGaps() {
		p.maybeNotify(ctx, job, report)
	}
}

// granularityConfig is the "granularity" key of cda_coverage_audit_config --
// "entry" (default, absent, or any unrecognized value) or "element". Kept as
// its own minimal query (not merged into maybeNotify's own separate config
// fetch below) since this one must resolve BEFORE BuildInventory runs, while
// maybeNotify's only matters after a report already exists — two small
// reads of the same row on this async, off-critical-path worker is an
// acceptable tradeoff for not restructuring process()'s existing flow.
type granularityConfig struct {
	Granularity string `json:"granularity"`
}

// resolveElementGranularity reports whether interfaceID has opted into
// element-level CDA Coverage Audit tracking. Defaults to false (entry-level,
// the original behavior) on any error, missing config, or unrecognized
// value — same "never let an audit lookup failure become a pipeline error"
// posture as services/interface_cda_coverage_resolver.go's
// ResolveCDACoverageAuditEnabled.
func (p *WorkerPool) resolveElementGranularity(ctx context.Context, interfaceID string) bool {
	var rawConfig []byte
	err := p.db.QueryRowContext(ctx,
		`SELECT COALESCE(cda_coverage_audit_config::text, '{}') FROM interfaces WHERE id = $1`,
		interfaceID,
	).Scan(&rawConfig)
	if err != nil {
		return false
	}
	var cfg granularityConfig
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return false
	}
	return cfg.Granularity == "element"
}

// notifyConfig is the "notify" sub-object of cda_coverage_audit_config —
// {"enabled": true, "notify": {"channel_ids": [...], "cooldown_minutes": 60}}.
// Absent entirely (the common case: badge-only, no external notification
// configured) is not an error, just nothing to do.
type notifyConfig struct {
	ChannelIDs      []string `json:"channel_ids"`
	CooldownMinutes int      `json:"cooldown_minutes"`
}

// maybeNotify dispatches an external notification (email/webhook/Slack) for
// an interface that has both opted into CDA Coverage Audit and configured at
// least one alert channel, subject to a per-interface cooldown. Never blocks
// or fails the caller — errors are logged only, since a notification is a
// side effect of the audit, not part of what the audit is responsible for
// guaranteeing.
func (p *WorkerPool) maybeNotify(ctx context.Context, job models.CoverageAuditJob, report *Report) {
	if p.notificationSvc == nil {
		return
	}

	var interfaceName string
	var rawConfig []byte
	err := p.db.QueryRowContext(ctx,
		`SELECT name, COALESCE(cda_coverage_audit_config::text, '{}') FROM interfaces WHERE id = $1`,
		job.InterfaceID,
	).Scan(&interfaceName, &rawConfig)
	if err != nil {
		log.Printf("⚠️  [cda_coverage] notify: could not load interface %s: %v", job.InterfaceID, err)
		return
	}

	var cfg struct {
		Notify *notifyConfig `json:"notify"`
	}
	if err := json.Unmarshal(rawConfig, &cfg); err != nil || cfg.Notify == nil || len(cfg.Notify.ChannelIDs) == 0 {
		return // no channels configured — badge-only, nothing to dispatch
	}

	cooldownMinutes := cfg.Notify.CooldownMinutes
	if cooldownMinutes <= 0 {
		cooldownMinutes = defaultCooldownMinutes
	}

	// Atomic cooldown gate: the UPDATE itself is the check, so two jobs for
	// the same interface processed concurrently by different pool workers
	// can't both pass (see this package's doc comment on worker concurrency).
	var wonID string
	err = p.db.QueryRowContext(ctx, `
		UPDATE interfaces
		SET last_coverage_gap_notified_at = NOW()
		WHERE id = $1
		  AND (last_coverage_gap_notified_at IS NULL
		       OR last_coverage_gap_notified_at <= NOW() - ($2 || ' minutes')::interval)
		RETURNING id
	`, job.InterfaceID, cooldownMinutes).Scan(&wonID)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("⚠️  [cda_coverage] notify: cooldown check failed for interface %s: %v", job.InterfaceID, err)
		}
		return // still in cooldown, or lost the race to another worker
	}

	alert := &services.FiredAlert{
		InterfaceID:   &job.InterfaceID,
		InterfaceName: interfaceName,
		AlertType:     "cda_coverage_gap",
		Severity:      "warning",
		Message:       coverageGapSummary(interfaceName, report),
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	for _, channelID := range cfg.Notify.ChannelIDs {
		go p.dispatchOne(channelID, alert)
	}
}

// dispatchOne sends alert to one channel, in its own detached goroutine with
// its own timeout — deliberately NOT sharing the job's own context/budget,
// since DispatchToChannel already retries up to 3 times (0s/2s/4s backoff)
// and a pool worker goroutine tied up on network I/O would delay every other
// queued report behind it.
func (p *WorkerPool) dispatchOne(channelID string, alert *services.FiredAlert) {
	ctx, cancel := context.WithTimeout(context.Background(), notifyDispatchTimeout)
	defer cancel()

	var ch services.AlertChannel
	var rawConfig []byte
	err := p.db.QueryRowContext(ctx,
		`SELECT id, channel_type, config, is_enabled FROM alert_channels WHERE id = $1`,
		channelID,
	).Scan(&ch.ID, &ch.ChannelType, &rawConfig, &ch.IsEnabled)
	if err != nil {
		log.Printf("⚠️  [cda_coverage] notify: channel %s not found: %v", channelID, err)
		return
	}
	if !ch.IsEnabled {
		return
	}
	ch.Config = rawConfig

	if err := p.notificationSvc.DispatchToChannel(ctx, &ch, alert); err != nil {
		log.Printf("⚠️  [cda_coverage] notify: dispatch to channel %s failed: %v", channelID, err)
	}
}

// coverageGapSummary builds a short, structural-only message body (no
// clinical values, matching the report itself) for the alert's Message field.
func coverageGapSummary(interfaceName string, report *Report) string {
	msg := interfaceName + ": CDA Coverage Audit found unmapped source data"
	for _, cat := range report.Categories {
		if len(cat.Missed) == 0 {
			continue
		}
		msg += "\n  - " + cat.Category + ": " + strconv.Itoa(cat.Found) + "/" + strconv.Itoa(cat.Total) + " used"
	}
	return msg
}

func resolveWorkerCount() int {
	if val := os.Getenv("CDA_COVERAGE_AUDIT_WORKERS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			return n
		}
	}
	if n := runtime.NumCPU(); n > 0 {
		return n
	}
	return 2
}

func resolveQueueSize() int {
	if val := os.Getenv("CDA_COVERAGE_AUDIT_QUEUE_SIZE"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			return n
		}
	}
	return defaultQueueSize
}
