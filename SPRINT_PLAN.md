# ezHealthKonnect — Sprint Plan & Tech Debt Tracker

*Last assessed: 2026-02-26*

---

## Overall Status

| Sprint | Plans | Status |
|--------|-------|--------|
| Sprint 1 | P12, P11, P8, P13 | ✅ Complete |
| Sprint 2 | P3, P6 | ✅ Complete |
| Sprint 3 | P2, P5 | ✅ Complete |
| Sprint 4 | P7 | 🔶 Partially done |
| Sprint 5 | P9, P1 | ⬜ Not started |
| Sprint 6 | P10 | 🔶 Partially done |
| Sprint 7 | P4 | 🔶 Partially done |

---

## Plan Details

---

### ✅ P12 — Layer Model Removal
**Goal**: Remove the pre/core/post layer concept entirely. Steps execute by sequence only.

**Done**:
- V50 migration drops `layer` from `transformation_steps` + `transformation_templates`
- `pipelineController.js` INSERT no longer includes `layer` *(fixed 2026-02-24)*
- `templateController.js` all SELECT/INSERT queries cleaned *(fixed 2026-02-24)*
- Frontend `PipelineModels.js` — `step.layer` removed
- Frontend `ToolboxManager.js` — no layer references
- `STANDARDS.md` rule: "Never set or reference `step.Layer`"

**Remaining**:
- `models/execution_group_models.go:15` — `Layer string db:"layer"` tag still present.
  Safe for now (struct is never scanned from raw SQL), but should be cleaned:
  ```go
  // Remove db:"layer" tag — column no longer exists
  Layer string `json:"layer"` // deprecated, kept for JSON compat only
  ```
- `controllers/pipelineController_old.js` — dead file, still references `layer` in SQL.
  **Action**: Delete the file (it is not imported anywhere).

---

### ✅ P11 — Structured Logging
**Goal**: All Go logging through `services/logger` (slog-based). No `fmt.Printf` / `log.Printf` in production code.

**Done**:
- `services/logger/logger.go` + `services/logger/interface_logger.go` implemented
- `STANDARDS.md` logging rules written
- `.golangci.yml` `forbidigo` linter configured to block `fmt.Printf/Println`, `log.Printf/Println`

**Remaining** (large surface area — migrate gradually):
- **306** `fmt.Printf/Println` calls remain in `services/executors/`
- **937** across all `services/` (excluding logger itself)
- `services/transformation_pipeline_service.go` alone has ~15 `fmt.Printf` calls
- **Migration order**: New code first (enforced by CI). Existing files: executors → pipeline_service → connectors → main.go last.
- **Action per file**: Replace `fmt.Printf("msg %v\n", x)` → `logger.Info("msg", "field", x)`

---

### ✅ P8 — Normalizer Performance
**Goal**: Replace JSON marshal/unmarshal cycle detection with reflect-based visited-set DFS.

**Done**:
- `models/output_normalizer.go` uses `reflect` package for type traversal (confirmed line 5, 141-151)
- No JSON round-trip for cycle detection

---

### ✅ P13 — Standards Enforcement
**Goal**: Lint config, standards check script, written standards doc.

**Done**:
- `.golangci.yml` — govet, staticcheck, gofmt, forbidigo configured
- `scripts/check-standards.sh` — full + quick modes, checks logging, layer removal, migration naming
- `STANDARDS.md` — logging rules, executor template, frontend builder template, migration conventions

---

### ✅ P3 — execCtx.Message Immutability *(Sprint 2)*
**Goal**: Prevent steps from silently mutating shared message state. Required for safe parallel execution later.

**Done**:
- `services/transformation_pipeline_service.go` — `shallowCopyMap()` helper added; `executor.Execute()` now receives a copy of `execContext.Message` instead of the live map
- Each step's output is merged back into `execContext.Message` after execution

---

### ✅ P6 — DB Enrichment Connection Pooling *(Sprint 2)*
**Goal**: One `sql.DB` pool per connection string (reuse across step executions).

**Done**:
- `services/dbpool/pool_registry.go` — new package: thread-safe registry, SHA-256 keyed, double-check locking, `GetOrCreate` / `Close` / `CloseAll`
- `services/executors/enrichment/database_enrichment_executor.go` — removed ad-hoc `dbConnectionPool` map; replaced with `dbpool.Get().GetOrCreate(...)`
- `models/enrichment_models.go` — added `PoolMaxOpen`, `PoolMaxIdle`, `PoolMaxLifetimeMinutes` config fields
- `main.go` — `defer dbpool.Get().CloseAll()` on shutdown
- `services/dbpool/pool_registry_test.go` — 20 unit tests (pure functions + concurrency); all pass
- **Quick Wins completed alongside P6**: deleted `pipelineController_old.js`, removed `db:"layer"` tag, added `catch→suppress` alias in `retry_utils.go`

**Test coverage**: `tests/playwright/sprint2-regression.spec.js` — 12/15 passed, 3 skipped (UI steps requiring specific visible state)

---

### ✅ P2 — Error Handling Cleanup + Circuit Breaker *(Sprint 3)*
**Goal**: Remove "catch" option (alias for "suppress"), add circuit breaker for external enrichment APIs.

**Done**:
- "catch" removed from frontend dropdown (only "suppress" + "rethrow" remain)
- PropertiesPanel fallback changed from `'catch'` → `'suppress'`
- `retry_utils.go` — `catch→suppress` alias added in `applyErrorHandling()`
- `services/executors/enrichment/circuit_breaker.go` — new file: thread-safe three-state machine (closed/open/half-open), global SHA-256-keyed registry, `testInFlight` guard against half-open race
- `models/enrichment_models.go` — `APIEnrichmentConfig` extended with `FailureThreshold int`, `OpenDurationSeconds int`
- `api_enrichment_executor.go` — CB wired: check before HTTP call, `RecordFailure`/`RecordSuccess` on result, `circuit_breaker_state` in `_executionDetails`
- `services/executors/enrichment/circuit_breaker_test.go` — 11 unit tests covering all state transitions, race guard, and registry singleton; all pass
- **Alerting**: webhook/email when failure rate exceeds threshold — lower priority, Sprint 5 candidate

---

### ✅ P5 — Enrichment Result Caching *(Sprint 3)*
**Goal**: Per-executor in-memory result cache to avoid redundant lookups (e.g. same patient MRN).

**Done**:
- `services/executors/enrichment/result_cache.go` — new file: LRU+TTL in-memory cache, `container/list`-based, SHA-256 keyed, global registry by cache type, deep copy on Get/Set to prevent mutation leaks
- `models/enrichment_models.go` — `APIEnrichmentConfig` extended with `CacheTtlSeconds int`, `CacheMaxEntries int`; `DatabaseEnrichmentConfigV2` extended with `CacheMaxEntries int`
- `api_enrichment_executor.go` — cache check before HTTP call, cache populate after success; `cache_hit` (bool) and `cached_at` (RFC3339) in `_executionDetails`
- `database_enrichment_executor.go` — existing `CacheResults`/`CacheTTL` config fields now actually used; `cache_hit` in `_executionDetails`
- `services/executors/enrichment/result_cache_test.go` — 17 unit tests covering hit/miss, TTL expiry, LRU eviction, deep copy isolation, concurrent access, registry singleton; all pass
- **Sprint 3 E2E coverage**: `tests/playwright/sprint3-enrichment-resilience.spec.js` — 12 tests (7 Sprint 3 features + 5 Sprint 2 regression); all pass

**Test summary**: 144 passed, 6 skipped, 0 failed (full regression suite)

---

### 🔶 P7 — enriched.* Accumulation Fix *(Sprint 4 — HIGH RISK)*
**Goal**: Remove the `enriched.*` wrapper key. Executors return flat maps; engine stores under `steps.{ns}.step_output.*`.

**Current state** — 16 `"enriched."` references remain in executors:
- `api_enrichment_executor.go` — default targetPath `"enriched.api"`
- `database_enrichment_executor.go` — default targetPath `"enriched.database"`
- `script_enrichment_executor.go` — default targetPath `"enriched.script"`
- `field_mapping_executor.go` — default targetPath `"enriched.field_mapping"`
- `file_parser_executor.go` — default targetPath `"enriched.file_parser"`

**What to build**:
1. Each executor returns flat output map (e.g. `{"mrn": "12345"}` not `{"enriched": {"api": {"mrn": "12345"}}}`)
2. Engine merges into `steps.{ns}.step_output.*` automatically (P3 prerequisite)
3. Migration: on first pipeline save, rewrite old `enriched.*` path references in step configs → `steps.{ns}.step_output.*`

**Risk**: HIGH — breaks existing pipeline configs that reference `enriched.*` paths. Requires migration + backward compat period.
**Prerequisite**: P3 (execCtx immutability) must be done first.

---

### ⬜ P9 — Backpressure *(Sprint 5)*
**Goal**: Bounded worker pool per interface. Prevent unbounded goroutine spawning under load.

**Current state**: No backpressure. Each incoming message spawns a goroutine immediately.

**What to build**:
```go
type InterfaceWorkerPool struct {
    semaphore chan struct{}        // bounded concurrency
    jobQueue  chan PipelineJob     // buffered job channel
    maxWorkers    int
    maxQueueDepth int
}
```
- Per-pipeline config: `maxWorkers` (default 10), `maxQueueDepth` (default 100), `rateLimitPerSecond`
- Queue depth metrics at `GET /api/system/queue-depths`
- TCP/MLLP inbound sends NACK (NAK) when pool is at 90% capacity
- Graceful shutdown: drain queue before exit

---

### ⬜ P1 — HIPAA Strengthening *(Sprint 5)*
**Goal**: Per-step PHI audit trail, dead-letter queue, pipeline RBAC, deidentify step.

**Current state**: Nothing implemented yet.

**What to build**:

**V51 migration** — two new tables:
```sql
CREATE TABLE pipeline_step_audit (
    id UUID PRIMARY KEY,
    execution_id UUID, pipeline_id UUID, step_name VARCHAR,
    input_field_hash TEXT,   -- SHA-256 of PHI fields (never raw data)
    output_field_hash TEXT,
    processed_at TIMESTAMPTZ, duration_ms INT
);

CREATE TABLE dead_letter_queue (
    id UUID PRIMARY KEY,
    interface_id UUID, pipeline_id UUID,
    raw_message TEXT, error_message TEXT,
    failed_at TIMESTAMPTZ, retry_count INT, resolved BOOLEAN DEFAULT false
);
```

**RBAC roles**: `viewer` / `analyst` / `operator` / `admin` on pipeline resource.

**`pre.deidentify` executor**: PHI masking/hashing step type using configurable field list.

---

### 🔶 P10 — Frontend OOP Per-Step Builders *(Sprint 6 — HIGH RISK)*
**Goal**: Every step type has its own `XxxBuilder.js`. PropertiesPanel is a thin orchestrator only.

**Done**:
- `StepBuilderRegistry.js` — singleton registry ✅
- `BaseStepConfigBuilder.js` — abstract base class ✅
- `StepConfigBuilderFactory.js` ✅
- `FileParserBuilder.js` ✅ (registered + wired)
- `RemoveDuplicatesBuilder.js` ✅ (registered + wired)
- `SwitchCaseBuilder.js`, `IfThenElseBuilder.js`, `ValidationRuleBuilder.js`, `ForEachLoopBuilder.js`, many more ✅

**Remaining** — steps still rendered inline in `PropertiesPanel.js` (not via registry):
- `ScriptEnrichmentEditor.js` exists but wiring in PropertiesPanel is partial (line 259, 4887)
- Generic field renderer (`case 'text'`, `case 'select'`, etc., lines 4941-5081) — still inline
- API enrichment, database enrichment, field mapping, outbound connector — no dedicated builder files

**Action**: For each remaining step type:
1. Create `XxxBuilder.js` extending `BaseStepConfigBuilder`
2. Register: `StepBuilderRegistry.register('step_type', XxxBuilder)`
3. Add `<script>` tag in `pipeline-builder.html`
4. Remove inline case from `PropertiesPanel.createDynamicFormFields()`

---

### 🔶 P4 — Connector Catalog *(Sprint 7)*
**Goal**: Implement the 24 stub connectors. Priority order: Tier 1 first.

**Done** (11 files, ~8 real implementations):
- `tcp_mllp_inbound.go` ✅ (full MLLP, TLS, ACK/NACK)
- `http_outbound.go` ✅
- `http_fhir_inbound.go` ✅
- `file_listener.go` ✅
- `file_writer.go` ✅
- `postgresql_inbound.go` ✅
- `postgresql_outbound.go` ✅
- `base_connector.go`, `connector_factory.go`, `connector_interface.go`, `database_base.go` — framework

**Remaining** (24 stubs in `connector_stubs.go`):
| Tier | Connector | Priority |
|------|-----------|----------|
| 1 | `tcp_mllp_outbound` | HIGH — middleware scenario |
| 1 | `sftp_inbound`, `sftp_outbound` | HIGH — file exchange |
| 1 | `mongodb_outbound` | HIGH — FHIR persistence |
| 2 | `kafka_inbound`, `kafka_outbound` | MEDIUM |
| 2 | `rabbitmq_inbound`, `rabbitmq_outbound` | MEDIUM |
| 2 | `aws_s3_inbound`, `aws_s3_outbound` | MEDIUM |
| 3 | `mysql_*`, `sqlserver_*`, `redis_*`, `azure_*`, `gcs_*`, `ftp_*`, `oracle_*` | LOW |

---

## Sprint Execution Order (Revised)

```
Sprint 1  ✅  P12 (layer), P11 (logging), P8 (normalizer), P13 (standards)
Sprint 2  ✅  P3 (immutability), P6 (DB pooling)
Sprint 3  ⬜  P2 (error handling / circuit breaker), P5 (result caching)  ← NEXT
Sprint 4  ⬜  P7 (enriched.* removal)                      ← HIGH RISK, alone
Sprint 5  ⬜  P9 (backpressure), P1 (HIPAA)
Sprint 6  ⬜  P10 (frontend OOP completion)                 ← HIGH RISK, alone
Sprint 7  ⬜  P4 (connector catalog, Tier 1)
```

## Quick Wins (can be done any time, < 1 hour each)

- [x] ~~Delete `controllers/pipelineController_old.js`~~ *(done — Sprint 2)*
- [x] ~~`models/execution_group_models.go:15` — remove `db:"layer"` tag~~ *(done — Sprint 2)*
- [x] ~~`services/transformation_pipeline_service.go` — add `catch → suppress` alias~~ *(done — Sprint 2)*
- [ ] Remaining `fmt.Printf` → `logger.Info` in `transformation_pipeline_service.go` (~15 calls)
