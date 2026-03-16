# Production Readiness Workplan

*Target: Stable, secure, enterprise-grade integration engine capable of processing millions of messages.*

---

## Quick Status

| Phase | Name | Status | Sprint |
|---|---|---|---|
| 1 | Convergence — remove dead code | 🟢 Complete | Done |
| 2 | Durability — ACK-after-store + recovery | 🟢 Complete | Done |
| 3 | Observability — metrics, health, logging | 🟢 Complete | Done |
| 4 | Architecture confirmed: Table-per-Interface | 🟢 Complete | Done |
| 5 | Reliability Hardening | 🔴 Not Started | — |
| 6 | Structured Logging P11 (complete) | 🔴 Not Started | — |

---

## Phase 1 — Convergence
**Goal:** One clear production path. Remove or isolate disconnected parallel code so the codebase is unambiguous.

### Context
Two overlapping systems exist in `processing/`:

| Files | Status | Decision |
|---|---|---|
| `engine.go` + `engine_message_processor.go` | ✅ Active — production path | **Canonical** |
| `transformation_engine.go` | ⚰️ Dead — `TransformationEngine` never instantiated | **Delete** |
| `step_processors.go` | ⚰️ Dead — duplicates `services/executors/` | **Delete** |
| `message_queue.go` | 💤 Built but disconnected — `processMessage()` has TODO+sleep | **Wire up in Phase 2** |
| `connectors.go` | ✅ Active — `InputConnector`/`OutputConnector` aliases used by engine | **Keep** |
| `processing/internal/connectivity/` | ⚰️ Dead — superseded by `services/connectors/` (tested, 46 unit tests) | **Delete** |
| `processing/internal/transformers/` | ⚰️ Dead — superseded by `services/executors/` | **Delete** |
| `processing/internal/storage/` | 💤 Good design, not connected — useful reference for Phase 4 | **Archive** |
| `processing/internal/routing/` | 💤 Has DLQ struct, load balancer — useful reference | **Archive** |
| `processing/pkg/` | 💤 `UniversalMessage`, `MessageLineage` — good models | **Keep — target for Phase 2** |

### Tasks

- [x] **P1-1** Write convergence decisions to this document
- [x] **P1-2** Delete `processing/transformation_engine.go` (TransformationEngine never used)
- [x] **P1-3** Delete `processing/step_processors.go` (duplicates services/executors/)
- [x] **P1-4** Delete `processing/internal/connectivity/` (superseded by services/connectors/)
- [x] **P1-5** Delete `processing/internal/transformers/` (superseded by services/executors/)
- [x] **P1-6** Delete `processing/internal/storage/` (compile errors, archived in ARCHIVE.md note)
- [x] **P1-7** Delete `processing/internal/routing/` (compile errors, archived in ARCHIVE.md note)
- [x] **P1-8** `go build ./...` passes ✅
- [x] **P1-9** *(discovered)* Delete `services/backup/` — redeclared types, never imported
- [x] **P1-10** *(discovered)* Delete root-level `fhir_converter.go`, `test_configuration_engine.go`, `test_interface_engine.go` — duplicate `main()` declarations

### Done When
`go build ./...` passes. No dead types or duplicate implementations exist alongside the active path. Every file in `processing/` either (a) is part of the active engine, (b) is `message_queue.go` earmarked for Phase 2, or (c) is clearly archived.

---

## Phase 2 — Durability
**Goal:** No message is lost after the sender receives ACK. Engine recovers unprocessed messages on restart.

### Context
Current flow has two gaps:
1. **ACK-before-store gap**: MLLP sends `AA` immediately on channel receive. PostgreSQL write happens asynchronously after. Process crash between those two events = message gone despite sender's ACK.
2. **No startup recovery**: Messages stored with `status = 'received'` but never processed are never retried after restart.

`message_queue.go` already has `FOR UPDATE SKIP LOCKED`, priority queuing, retry scheduling, and attempt tracking. It just has `time.Sleep(100ms)` where the actual pipeline call should go.

### Tasks

#### P2-1 — Wire MessageQueue to the transformation pipeline ✅
- `processMessage()` now calls `TransformationPipelineService.ExecutePipeline()`
- `NewMessageQueue(db, transformationSvc)` — pipeline service injected
- `engine.go` instantiates MessageQueue; `Start()` calls `mq.Start(ctx)`

#### P2-2 — ACK after store (not after channel receive) ✅
- `sendACKAfterStore()` added to `engine_message_processor.go`
- Called immediately after `storeMessage()` succeeds
- Looks up `validationConnectors[interfaceID]` and sends `accepted` AA in a goroutine
- No-op if connector does not support validation feedback

#### P2-3 — Status lifecycle ✅
- `updateMessageStatus()` helper added to `engine_message_processor.go`
- Transitions: `received` (on store) → `processing` (before pipeline) → `delivered`/`failed` (after pipeline)
- `processing_started_at` stamped via extra field map

#### P2-4 — Startup recovery loop ✅
- `recoverUnprocessedMessages(ctx)` added to `engine.go`
- Scans all `messages_intf_*` tables via `pg_tables`; 10 s startup delay
- Finds `status IN ('received', 'processing') AND received_at < NOW() - 5 min`
- Re-enqueues via `MessageQueue.EnqueueForRecovery()` at priority 8

#### P2-5 — Wire DLQ table to replay API
- V51 created `dead_letter_queue` table — wire it up
- When pipeline exceeds max retries, insert to DLQ with full error context
- `GET /api/system/dlq` — list messages in DLQ (paginated, filterable by interface)
- `POST /api/system/dlq/replay/:messageId` — requeue one message
- `POST /api/system/dlq/replay-all?interface_id=X` — requeue all for an interface
- File: new `controllers/dlq_controller.go`, route added in `main.go`

### Migration
- **V52** ✅ Applied + registered in Flyway history (rank 52)
  - Adds `processing_started_at TIMESTAMPTZ` to all `messages_intf_*` tables

### Done When
- Kill the process with 50 messages in flight → restart → all 50 process exactly once ✅ (mechanism in place)
- MLLP sender that sent a message never gets `AA` unless the message is in PostgreSQL ✅
- `GET /api/system/dlq` returns failed messages; replay API requeues them ⏳ P2-5 remaining

---

## Phase 3 — Observability
**Goal:** Full visibility into runtime behaviour. Operable without SSH access to the box.

### Tasks

#### P3-1 — Health and readiness endpoints ✅
- `GET /healthz` → always 200 (liveness)
- `GET /readyz` → 200 if `db.Ping()` + `engine.IsRunning()`, else 503 with check details
- File: `main.go`

#### P2-5 — DLQ replay API ✅ (completed alongside Phase 3)
- `GET /api/system/dlq` — paginated list (filter by interface_id, resolved)
- `GET /api/system/dlq/:id` — single entry
- `POST /api/system/dlq/:id/replay` — re-enqueue one message (priority 8)
- `POST /api/system/dlq/replay-all?interface_id=X` — bulk replay
- `POST /api/system/dlq/:id/resolve` — mark resolved without replay
- `DELETE /api/system/dlq/:id` — hard delete
- File: `controllers/dlq_controller.go`

#### P3-2 — Prometheus metrics ✅
- `github.com/prometheus/client_golang` added to go.mod
- `GET /metrics` exposes standard Prometheus scrape endpoint (via `promhttp.Handler()`)
- Metrics package: `services/metrics/metrics.go`

| Metric | Type | Labels | Status |
|---|---|---|---|
| `ezhk_messages_received_total` | Counter | `interface_id`, `source_type` | ✅ wired |
| `ezhk_messages_processed_total` | Counter | `interface_id`, `status` | ✅ wired |
| `ezhk_messages_processing_duration_seconds` | Histogram | `interface_id` | ✅ wired |
| `ezhk_pipeline_step_duration_seconds` | Histogram | `step_type` | defined |
| `ezhk_backpressure_queue_depth` | Gauge | `interface_id` | defined |
| `ezhk_circuit_breaker_state` | Gauge | `executor_id` | defined |
| `ezhk_dlq_depth` | Gauge | — | defined |
| `ezhk_connectors_active_total` | Gauge | `connector_type` | defined |

#### P3-3 — Structured logging — first wave ✅
Converted all `fmt.Printf` in the 2 highest-volume files:
- `processing/engine.go` — 13 calls → `logger.Info/Debug/Error` with structured fields
- `processing/engine_message_processor.go` — already using `log.Printf` (no change needed)
- `services/transformation_pipeline_helpers.go` — 24 calls → `logger.Debug/Info/Warn` carrying `interface_id` + `correlation_id` on every per-step log entry

Remaining: ~900 `fmt.Printf` calls across rest of codebase → Phase 6.

### Done When ✅
- `GET /healthz` returns 200 ✅
- `GET /readyz` returns 503 when DB is down ✅
- `GET /metrics` returns Prometheus exposition format ✅
- `GET /api/system/dlq` returns failed messages ✅
- Hot path log lines carry `correlation_id` + `interface_id` ✅

---

## Phase 4 — Architecture Confirmed: Table-per-Interface
**Goal:** Validate that the table-per-interface design scales and is the right model for this system.

### Decision ✅

**Table-per-interface + MongoDB-collection-per-interface is the correct and intentional architecture.**

PostgreSQL partitioning was evaluated (V53 created the `messages` parent, V54 dropped it) and rejected:

| Concern | Partitioning | Table-per-Interface (chosen) |
|---|---|---|
| Tenant isolation | Needs explicit RLS | Natural — separate physical tables |
| Purge / archive | Still row-level DELETE | `TRUNCATE` or `DROP TABLE` in O(1) |
| Security | All tenants visible via parent without RLS | Table grant = interface grant |
| Reports | Easy cross-interface aggregation | Not needed per-query; use offline ETL |
| MongoDB symmetry | No benefit | `raw_messages_intf_X` mirrors PG table per interface |
| Query generation | Partition routing adds a layer | Caller builds table name directly, no overhead |

### Migration trail
- **V53** `V53__Partition_Messages_Table.sql` — Created `messages` partitioned parent, attempted ATTACH (all 26 existing tables skipped: PostgreSQL 15 requires exact column match, existing tables have diverged schemas)
- **V54** `V54__Drop_Messages_Parent_Table.sql` — Dropped the parent; 26 standalone tables untouched

### Code state ✅
- `interface_message_service.go` `ensureInterfaceTableExists()` — standalone table creation, existence check via `pg_tables`
- `DropInterfaceTable()` — direct `DROP TABLE IF EXISTS ... CASCADE`, no partition logic
- `go build ./...` passes ✅
- All 26 existing interface tables intact and operational

### Decisions recorded
- **D12**: Table-per-interface is the architectural standard for ezHealthKonnect. Cross-interface aggregation (for admin reports) uses offline ETL or a separate `reporting_*` materialized view layer — not a shared parent table.
- **D13**: MongoDB collection-per-interface (`raw_messages_intf_{uuid}`) mirrors the PostgreSQL table-per-interface model, giving consistent isolation at every storage layer.

---

## Phase 5 — Reliability Hardening
**Goal:** Fix known implementation-level gaps that will cause production incidents.

### Tasks

#### P5-1 — SFTP: replace shell commands with native SFTP subsystem
Current `sftp_inbound.go` uses SSH shell `find` + `cat` commands — fails on restricted shells (common in healthcare).
- Add `github.com/pkg/sftp` to `go.mod`
- Replace `find`/`cat` approach with SFTP `ReadDir()` + `Open()` + `Read()`
- Works with SFTP-only servers, restricted shells, commercial healthcare SFTP gateways
- File: `services/connectors/sftp_inbound.go`

#### P5-2 — goja: compile scripts once, execute many
Current `script_enrichment_executor.go` creates a new `goja.Runtime` and compiles the script on every message — 10–100x more expensive than execution at scale.
```go
// Add to executor struct:
compiledCache sync.Map // hash(script) → *goja.Program

// On first execution:
prog, _ := goja.Compile("", script, false)
cache.Store(hash, prog)

// Every execution:
runtime := goja.New()
runtime.RunProgram(prog) // compiled program reused, fresh runtime for isolation
```
File: `services/executors/enrichment/script_enrichment_executor.go`

#### P5-3 — Per-interface rate limiting
- Add token bucket rate limiter on `pool.Submit()` path in `engine_message_processor.go`
- Config field: `max_messages_per_second` on interface config (defaults to unlimited)
- When limit hit: backpressure (NACK) not silent drop
- Use `golang.org/x/time/rate` (already in the standard extended library)

### Done When
- SFTP connector works against a restricted-shell SFTP server (e.g., AWS Transfer Family)
- Script steps handle 10k+ messages/sec without re-compilation overhead
- Rate-limited interfaces return NACK when over limit; burst is handled gracefully

---

## Phase 6 — Structured Logging P11 (Complete)
**Goal:** Zero `fmt.Printf`/`log.Printf` in production code paths.

### Context
P11 started in Sprint 1 — `services/logger` (slog-based) exists and STANDARDS.md is written.
937 calls remain. Phase 3 does the hot paths. Phase 6 finishes the rest.

### Approach
Migrate by package, enforce via CI (`golangci.yml` forbidigo is already configured).
After each package is migrated, enable the lint rule for that package so it can never regress.

### Package Order
1. ~~`processing/engine.go` + `engine_message_processor.go`~~ → Done in Phase 3
2. ~~`services/transformation_pipeline_helpers.go`~~ → Done in Phase 3
3. `services/executors/enrichment/` (~160 calls)
4. `services/executors/transform/` (~80 calls)
5. `services/connectors/` (~100 calls)
6. `services/transformation_pipeline_service.go` (~15 calls)
7. `main.go` + remaining `services/` (~remaining)

### Done When
`golangci-lint run` passes with forbidigo rules enabled for all packages. Zero `fmt.Printf` in production code.

---

## Decisions Log

| # | Decision | Rationale | Date |
|---|---|---|---|
| D1 | `services/connectors/` is canonical connector framework | 46 unit tests, 5 fully implemented connectors, production-tested | 2026-03-08 |
| D2 | `processing/internal/connectivity/` → deleted | Superseded by D1; no tests, not connected to active engine | 2026-03-08 |
| D3 | `services/executors/` is canonical executor framework | Registered in ExecutorRegistry, 241 E2E tests cover it | 2026-03-08 |
| D4 | `processing/internal/transformers/` → deleted | Superseded by D3 | 2026-03-08 |
| D5 | `processing/transformation_engine.go` → deleted | `TransformationEngine` never instantiated; `services/transformation_pipeline_service.go` is the engine | 2026-03-08 |
| D6 | `processing/step_processors.go` → deleted | Duplicates executor logic; never called | 2026-03-08 |
| D7 | `processing/message_queue.go` → wire up in Phase 2 | Has `FOR UPDATE SKIP LOCKED` queue already designed correctly | 2026-03-08 |
| D8 | `processing/internal/storage/` → archived (reference) | Good `UniversalMessage` design; informs Phase 4 partitioning work | 2026-03-08 |
| D9 | `processing/internal/routing/` → archived (reference) | `RoutingEngine` + `LoadBalancer` useful for future horizontal scaling | 2026-03-08 |
| D10 | PostgreSQL partitioning over per-interface tables | Partition pruning gives same isolation; cross-interface queries become possible; avoids catalog bloat at scale | 2026-03-08 |
| D11 | Kafka as connector only (not backbone) | PostgreSQL-backed `MessageQueue` + ACK-after-store achieves durability without adding Kafka dependency; simpler ops | 2026-03-08 |

---

## Key File Reference

| File | Purpose |
|---|---|
| `processing/engine.go` | Engine lifecycle, interface activation, connector startup |
| `processing/engine_message_processor.go` | Per-message processing loop, PostgreSQL store, worker pool submit |
| `processing/message_queue.go` | Database-backed durable queue — **wire up in Phase 2** |
| `processing/connectors.go` | `InputConnector`/`OutputConnector` interface aliases |
| `services/transformation_pipeline_service.go` | Pipeline orchestration, step loading |
| `services/transformation_pipeline_helpers.go` | Pipeline execution loop, step output tracking |
| `services/executor_registry.go` | Executor factory with auto-registration |
| `services/connectors/connector_interface.go` | Universal connector interfaces |
| `services/backpressure/worker_pool.go` | Per-interface bounded worker pool |
| `services/logger/logger.go` | Structured logger (slog-based) — use this everywhere |
| `database/migrations/` | Flyway migrations V1–V51 |
