# Message-Level Parallel Processing Design

**Your Requirement**: "We receive 5 messages and we start execution in parallel and then each message will be processed through pipeline using proposed flow"

---

## Two Levels of Parallelism

### Level 1: Message-Level Parallelism (THIS DOCUMENT)
**Process multiple messages simultaneously for an interface**

```
Interface receives 5 messages:
    ↓
Message 1 ──┐
Message 2 ──┤
Message 3 ──┼──→ All process in PARALLEL
Message 4 ──┤
Message 5 ──┘
```

### Level 2: Pipeline-Level Parallelism (PREVIOUS DESIGN)
**Each message's pipeline can have parallel steps**

```
Message 1 Pipeline:
  Step 1 (sequential)
      ↓
  Step 2A ⟍
  Step 2B  ⟼ Parallel steps
  Step 2C ⟋
      ↓
  Step 3 (sequential)
```

### Combined Result: Maximum Parallelism!
```
5 Messages × Pipeline with 3 parallel steps = 15 concurrent operations!
```

---

## Current Architecture Analysis

### Current Flow (Sequential Message Processing)

```go
// processing/engine_message_processor.go:26
for msg := range messageChan {
    // Process ONE message at a time
    storeMessage(msg)            // Blocking
    storeAndParseWithRecovery(msg)  // Async but doesn't wait
}
```

**Problem**: Messages are processed **one at a time** from the channel!

**Current Behavior**:
```
Message Chan: [Msg1, Msg2, Msg3, Msg4, Msg5]
    ↓
Msg1 → Store → Parse → Done
    ↓
Msg2 → Store → Parse → Done  ← Must wait for Msg1
    ↓
Msg3 → Store → Parse → Done  ← Must wait for Msg2
...
```

### Why Current Design is Sequential

**The Loop**:
```go
for msg := range messageChan {
    // Blocking operation
    err := pe.storeMessage(interfaceID, msg)

    // Async but loop immediately continues to next message
    go pe.storeAndParseWithRecovery(interfaceID, msg)
}
```

The `for` loop processes messages **sequentially** even though `storeAndParseWithRecovery` is async!

---

## Proposed Architecture: Worker Pool Pattern

### High-Level Design

```
Message Channel (buffer: 10,000)
    ↓
╔═══════════════════════════════════════╗
║     Worker Pool (N workers)           ║
║  Worker 1 ──→ Process Msg1 Pipeline   ║
║  Worker 2 ──→ Process Msg2 Pipeline   ║
║  Worker 3 ──→ Process Msg3 Pipeline   ║
║  ...                                  ║
║  Worker N ──→ Process MsgN Pipeline   ║
╚═══════════════════════════════════════╝
    ↓
PostgreSQL + MongoDB Storage
```

**Key Concept**: N goroutines reading from the **same channel** = N messages processed in parallel!

---

## Implementation Design

### Interface Configuration (User-Controlled)

Add to interface configuration:

```json
{
  "interface_id": "...",
  "name": "Epic ADT Feed",
  "concurrent_workers": 5,  // ← NEW: How many messages to process in parallel
  "max_queue_size": 10000   // Existing buffer size
}
```

**Defaults**:
- `concurrent_workers`: 10 (reasonable default)
- Low-volume interfaces: 1-5 workers
- High-volume interfaces: 20-100 workers

### Modified ProcessMessages Function

```go
// NEW: Start worker pool for an interface
func (pe *ProcessingEngine) processMessages(interfaceID string, messageChan <-chan *models.InboundMessage) {
    log.Printf("📨 Message processor started for interface %s", interfaceID)

    // Get concurrent workers from interface config (default: 10)
    workerCount := pe.getInterfaceWorkerCount(interfaceID)
    log.Printf("🔢 Starting %d concurrent workers for interface %s", workerCount, interfaceID)

    // Create worker pool
    var wg sync.WaitGroup

    // Launch N workers
    for i := 0; i < workerCount; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            pe.messageWorker(interfaceID, workerID, messageChan)
        }(i)
    }

    // Wait for all workers to complete (when channel closes)
    wg.Wait()
    log.Printf("📪 Message processor stopped for interface %s", interfaceID)
}

// NEW: Worker function (processes messages from channel)
func (pe *ProcessingEngine) messageWorker(interfaceID string, workerID int, messageChan <-chan *models.InboundMessage) {
    log.Printf("👷 Worker %d started for interface %s", workerID, interfaceID)

    for msg := range messageChan {
        startTime := time.Now()

        log.Printf("👷‍♂️ Worker %d processing message %s for interface %s",
            workerID, msg.MessageID, interfaceID)

        // STEP 1: Store message in PostgreSQL
        err := pe.storeMessage(interfaceID, msg)
        if err != nil {
            log.Printf("❌ Worker %d failed to store message %s: %v",
                workerID, msg.MessageID, err)
            pe.updateInterfaceError(interfaceID)
            continue
        }

        // STEP 2: Store in MongoDB + Parse to JSON (async)
        go pe.storeAndParseWithRecovery(interfaceID, msg)

        // STEP 3: Execute transformation pipeline (if configured)
        if pe.transformationService != nil {
            go pe.executeTransformationPipeline(interfaceID, msg)
        }

        processingTime := time.Since(startTime)
        log.Printf("✅ Worker %d completed message %s in %v",
            workerID, msg.MessageID, processingTime)

        pe.updateInterfaceStats(interfaceID, processingTime)
    }

    log.Printf("👷 Worker %d stopped for interface %s", workerID, interfaceID)
}
```

### Get Worker Count from Interface Config

```go
// Get concurrent worker count from interface configuration
func (pe *ProcessingEngine) getInterfaceWorkerCount(interfaceID string) int {
    query := `SELECT source_config FROM interfaces WHERE id = $1`

    var configJSON []byte
    err := pe.db.QueryRow(query, interfaceID).Scan(&configJSON)
    if err != nil {
        log.Printf("⚠️  Failed to get interface config, using default workers: %v", err)
        return 10 // Default
    }

    var config map[string]interface{}
    if err := json.Unmarshal(configJSON, &config); err != nil {
        return 10 // Default
    }

    // Get concurrent_workers from config
    if workers, ok := config["concurrent_workers"].(float64); ok {
        return int(workers)
    }

    return 10 // Default
}
```

---

## Concurrency Control

### Thread Safety Considerations

**Shared Resources**:
- PostgreSQL database connection pool ✅ (already thread-safe)
- MongoDB connection pool ✅ (already thread-safe)
- Interface statistics (`pe.activeInterfaces`) ⚠️ **Needs mutex!**

**Already Protected**:
```go
// processing/engine_message_processor.go:123
func (pe *ProcessingEngine) updateInterfaceStats(...) {
    pe.mutex.Lock()  // ✅ Already has mutex!
    defer pe.mutex.Unlock()
    ...
}
```

**No Changes Needed** - existing mutex already protects shared state!

### Database Connection Pooling

PostgreSQL and MongoDB clients already use connection pooling:

```go
// PostgreSQL default pool: 25 connections
// MongoDB default pool: 100 connections
```

**For high concurrency** (100+ workers):
- Increase PostgreSQL max_connections
- Increase connection pool size in database/sql

---

## Performance Analysis

### Current (Sequential Processing)

```
5 Messages received:
Msg1: 200ms (store + parse)
Msg2: 200ms (waits for Msg1)
Msg3: 200ms (waits for Msg2)
Msg4: 200ms (waits for Msg3)
Msg5: 200ms (waits for Msg4)
─────────────────────────────
Total: 1000ms (1 second)
```

### Proposed (Parallel Processing with 5 Workers)

```
5 Messages received, 5 workers:
Worker 1: Msg1 → 200ms ⟍
Worker 2: Msg2 → 200ms  ⟼
Worker 3: Msg3 → 200ms  ⟼ All at once!
Worker 4: Msg4 → 200ms  ⟼
Worker 5: Msg5 → 200ms ⟋
─────────────────────────────
Total: 200ms ✅ 5x FASTER!
```

### High-Volume Scenario (1000 messages, 10 workers)

**Current (Sequential)**:
```
1000 messages × 200ms = 200,000ms = 200 seconds (3.3 minutes)
```

**Proposed (10 Workers)**:
```
1000 messages / 10 workers = 100 messages per worker
100 messages × 200ms = 20,000ms = 20 seconds ✅ 10x FASTER!
```

---

## Configuration Examples

### Low-Volume Interface (1-10 messages/minute)
```json
{
  "interface_id": "...",
  "name": "Small Clinic - Patient Updates",
  "concurrent_workers": 2,
  "max_queue_size": 100
}
```

### Medium-Volume Interface (100-1000 messages/minute)
```json
{
  "interface_id": "...",
  "name": "Regional Hospital - Lab Results",
  "concurrent_workers": 10,
  "max_queue_size": 5000
}
```

### High-Volume Interface (10,000+ messages/minute)
```json
{
  "interface_id": "...",
  "name": "Epic Enterprise Feed",
  "concurrent_workers": 50,
  "max_queue_size": 50000
}
```

---

## Combined with Pipeline-Level Parallelism

### Example: Epic ADT Feed

**Interface Config**:
```json
{
  "concurrent_workers": 20  // Process 20 messages at once
}
```

**Pipeline Config** (for each message):
```
Step 1: Validate (sequential)
Step 2A: EMPI API    ⟍
Step 2B: Lab Lookup   ⟼ 3 parallel steps
Step 2C: Insurance   ⟋
Step 3: HL7→FHIR (sequential)
```

**Result**:
- 20 messages processing simultaneously
- Each message has 3 parallel API calls
- **60 concurrent API calls at peak!**

**Performance**:
```
Without parallelism:
  20 messages × (validate + EMPI + Lab + Insurance + Transform)
  20 messages × (50ms + 300ms + 400ms + 200ms + 100ms)
  20 messages × 1050ms = 21,000ms (21 seconds)

With message + pipeline parallelism:
  max(20 messages) × (50ms + max(300,400,200) + 100ms)
  20 messages × 550ms = 550ms ✅ 38x FASTER!
  (Assuming 20+ workers and sufficient CPU/network)
```

---

## UI Configuration

### Interface Edit Form

Add concurrent workers field:

```html
<div class="form-group">
    <label>Concurrent Workers</label>
    <input type="number" name="concurrent_workers" min="1" max="100" value="10">
    <small class="form-text text-muted">
        How many messages to process simultaneously (default: 10).
        Higher = faster for high-volume, but uses more CPU/memory.
    </small>
</div>
```

### Performance Dashboard

Show worker utilization:

```
Interface: Epic ADT Feed
Workers: 10 active
Queue Size: 2,347 messages
Current Throughput: 487 msg/sec
Average Worker Utilization: 87%
```

---

## Resource Limits & Safety

### CPU/Memory Protection

```go
// Max workers per interface (safety limit)
const MAX_WORKERS_PER_INTERFACE = 100

func (pe *ProcessingEngine) getInterfaceWorkerCount(interfaceID string) int {
    workers := getConfiguredWorkers(interfaceID)

    if workers > MAX_WORKERS_PER_INTERFACE {
        log.Printf("⚠️  Interface %s requested %d workers, limiting to %d",
            interfaceID, workers, MAX_WORKERS_PER_INTERFACE)
        return MAX_WORKERS_PER_INTERFACE
    }

    return workers
}
```

### Database Connection Limits

```go
// Ensure DB pool can handle all workers across all interfaces
func (pe *ProcessingEngine) validateWorkerConfig() {
    totalWorkers := 0
    for _, iface := range pe.activeInterfaces {
        totalWorkers += iface.WorkerCount
    }

    dbPoolSize := pe.getDBPoolSize()
    if totalWorkers > dbPoolSize {
        log.Printf("⚠️  Total workers (%d) exceeds DB pool size (%d) - may cause contention",
            totalWorkers, dbPoolSize)
    }
}
```

---

## Migration Strategy

### Phase 1: Backend Implementation (Current)
- ✅ Model already supports configuration
- Implement worker pool pattern
- Add `concurrent_workers` config reading
- Add worker-level logging
- Test with 1, 5, 10 workers

### Phase 2: UI Configuration
- Add concurrent workers field to interface form
- Add default value (10) to existing interfaces
- Add validation (min: 1, max: 100)

### Phase 3: Monitoring & Tuning
- Add worker metrics to dashboard
- Add performance recommendations based on throughput
- Auto-tune worker count based on queue depth?

---

## Implementation Checklist

### Backend (Go)
- [ ] Add `concurrent_workers` to interface model
- [ ] Implement worker pool in `processMessages()`
- [ ] Implement `messageWorker()` function
- [ ] Implement `getInterfaceWorkerCount()`
- [ ] Add worker logging (worker ID in logs)
- [ ] Add worker metrics (active, idle, utilization)
- [ ] Add max worker safety limit
- [ ] Test thread safety with race detector

### Frontend (JavaScript)
- [ ] Add "Concurrent Workers" field to interface form
- [ ] Add default value (10) for existing interfaces
- [ ] Add help text explaining parallelism
- [ ] Add worker metrics to monitoring dashboard

### Database
- [ ] Verify PostgreSQL pool can handle max workers
- [ ] Verify MongoDB pool can handle max workers
- [ ] Add monitoring for connection pool usage

### Testing
- [ ] Test with 1 worker (sequential - baseline)
- [ ] Test with 5 workers (moderate parallelism)
- [ ] Test with 20 workers (high parallelism)
- [ ] Test with 100 workers (stress test)
- [ ] Performance benchmark (throughput vs workers)
- [ ] Race condition testing (`go test -race`)

### Documentation
- [ ] Update architecture docs
- [ ] Add performance tuning guide
- [ ] Add worker count recommendations by volume
- [ ] Add troubleshooting guide

---

## Timeline Estimate

**Phase 1: Core Implementation** (1 day)
- Worker pool pattern
- Configuration reading
- Worker-level logging

**Phase 2: UI Integration** (0.5 days)
- Add form field
- Add default values
- Update validation

**Phase 3: Testing & Tuning** (1 day)
- Performance testing
- Race condition testing
- Optimal worker count recommendations

**Total**: ~2.5 days

---

## Questions for You

1. **Default Worker Count**: What should be the default?
   - 5 (conservative)
   - 10 (balanced - recommended)
   - 20 (aggressive)

2. **Max Workers**: What should be the hard limit?
   - 50 (safe)
   - 100 (recommended)
   - Unlimited (let user decide - risky)

3. **Auto-Tuning**: Should we auto-adjust workers based on queue depth?
   - Yes - if queue > 1000, increase workers
   - No - let user configure manually (recommended)

4. **Priority**: Is this high priority?
   - Critical (implement immediately)
   - Medium (after pipeline parallelism)
   - Low (future enhancement)

---

## Recommendation

**YES - Implement This!**

**Why**:
- ✅ Massive performance gain (10-50x for high-volume)
- ✅ Industry standard pattern (worker pools everywhere)
- ✅ Simple implementation (~2.5 days)
- ✅ User-configurable per interface
- ✅ Safe (default limits, connection pooling)
- ✅ Enables true enterprise scale

**Combined with pipeline parallelism** = Ultimate performance! 🚀

**Should I proceed with implementation?**
