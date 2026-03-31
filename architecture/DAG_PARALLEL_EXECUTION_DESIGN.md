# DAG Parallel Execution Design

**Status**: Planned — Sprint N+1
**Replaces**: Sequence-number based execution in `services/transformation_pipeline_service.go`
**Related**: [TRANSFORMATION_PIPELINE_DESIGN.md](TRANSFORMATION_PIPELINE_DESIGN.md)

---

## The Core Question

> "Steps which are not dependent on any other — do they stay afloat (start immediately),
> or do we have multiple start points connecting to a succeeding step?"

**Answer: Both patterns are valid and the same rule covers them both.**

The execution model is a **Directed Acyclic Graph (DAG)**. One rule governs all:

> **A step starts as soon as every step with a direct arrow pointing TO it has finished.**

- Zero predecessors → pending count = 0 → starts immediately (floats)
- One predecessor → waits for that one step
- N predecessors → waits for all N (convergence point)

---

## The Two Patterns

### Pattern A — Floating Steps (Zero Predecessors)

Steps with no incoming connections start at time zero, in parallel with any other
zero-predecessor steps including `connector.inbound`.

```
[Fetch Reference Data]  ← no predecessors → starts at time zero
[connector.inbound]     ← no predecessors → starts at time zero
        ↓
[Validate Patient]      ← 1 predecessor (connector.inbound)
        ↓
[Map to FHIR]
```

Use cases: pre-fetch lookup tables, warm up external connections, write audit log entry.

---

### Pattern B — Multiple Entry Points Converging

Multiple parallel branches feed a single successor. It waits for all of them.

```
[connector.inbound] ─┬→ [Validate Patient ID]  ─┐
                     └→ [Enrich from Epic API]  ─┴→ [Map to FHIR] → [Send]
```

User draws two outgoing arrows from `connector.inbound`. Both start simultaneously.
`Map to FHIR` has pending = 2. Starts when both predecessors finish.

---

## Full Topology Examples

### Linear (no change from today)
```
[connector.inbound] → [Validate] → [Map to FHIR] → [connector.outbound]
```

### Parallel Enrichment
```
[connector.inbound] ─┬→ [Enrich from Epic]      ─┐
                     ├→ [Enrich from Lab System]  ─┤→ [Map to FHIR] → [Send]
                     └→ [Validate Patient ID]     ─┘
```
Net time = `connector.inbound` + `max(3 parallel steps)` + `Map to FHIR` + `Send`
Without parallelism: sum of all. At 200ms each, saves 400ms per message.

### Diamond (branch and rejoin)
```
                    ┌→ [Enrich Demographics] ─┐
[connector.inbound] ┤                          ├→ [Assemble FHIR] → [Send]
                    └→ [Enrich Insurance]    ─┘
```

### Conditional Branch Inside DAG
```
[connector.inbound] → [Switch on Message Type]
                              ↓
                    ┌── ADT^A01 ──→ [ADT Handler]  ─┐
                    ├── ORU^R01 ──→ [ORU Handler]  ─┤→ [connector.outbound]
                    └── default ──→ [Log Unknown]  ─┘
```
Non-taken branches are cascade-skipped (see Gap 4 below).

---

## Identified Design Gaps and Their Resolutions

The initial design sketch had five real problems. Each is documented and resolved here.

---

### Gap 1 — Race Condition on Shared Message Map (CRITICAL)

**Problem**: The current pipeline executor writes results back to `execCtx.Message`
(a plain `map[string]interface{}`). If two parallel steps both write to this map
simultaneously, Go's map is not safe for concurrent writes — this panics at runtime.

**Resolution: Replace `execCtx.Message` with `sync.Map` during parallel execution**

Steps already write to separate namespace keys in `StepOutputs` (`alias_shortID` format).
For the shared message map, switching to `sync.Map` makes concurrent reads and writes
safe without any changes to individual executors. Two parallel steps writing to
*different* keys (the normal case for independent enrichments) have zero conflict.

If two parallel steps write to the *same key*, last-write-wins behaviour applies —
semantically acceptable for independent enrichments that shouldn't overlap.

**Pipeline save-time warning** (non-blocking): if two parallel steps (no path between
them) are both configured to write to the same output field, warn the user:
```
⚠ "Enrich Epic" and "Enrich Lab" both write to "patient.mrn".
  Running in parallel — last to finish wins. Add a connection if order matters.
```

---

### Gap 2 — Loop Container Blocks a Worker (PERFORMANCE)

**Problem**: A Loop step that iterates 1000 times at 100ms each blocks one worker
for 100 seconds. If `workerCount = 3` and three loop containers run simultaneously,
all workers are consumed and no other steps can run.

**Resolution: Container executor uses a separate internal goroutine, not the DAG worker**

The DAG worker pool is for *step dispatch* only — picking up a step ID from the queue
and handing it off. Container steps (Loop, Try-Catch) immediately spawn their own
internal goroutine and release the DAG worker:

```go
// In the Loop executor — conceptual change
func (e *LoopExecutor) Execute(ctx context.Context, execCtx *PipelineExecutionContext) ExecutionResult {
    resultChan := make(chan ExecutionResult, 1)

    go func() {
        // Iterate body steps sequentially (unchanged internal logic)
        result := e.runIterations(ctx, execCtx)
        resultChan <- result
    }()

    return <-resultChan  // DAG worker is released immediately on return,
                         // but the goroutine above runs independently
}
```

Wait — this still blocks the DAG worker (return waits on `resultChan`). The correct
pattern is: the DAG executor tracks an "in-flight" count separately from the worker
pool. Container steps opt out of the worker pool entirely and self-manage:

```go
type StepExecutionMode int
const (
    ModeWorkerPool StepExecutionMode = iota  // standard: uses DAG worker
    ModeSelfManaged                          // containers: own goroutine, signals done channel directly
)
```

Container executors implement `SelfManaged() bool` returning true. The DAG scheduler
spawns them as independent goroutines (not from the pool) and they signal `done`
when their internal iteration finishes. Workers remain available for other steps.

**Practical impact**: Rare in healthcare pipelines. Most pipelines have few or no
Loop containers. For Sprint N+1, implement the simple version (blocks worker) with
a TODO. Refactor in Sprint N+2 only if loop-heavy pipelines are observed in practice.

#### Loop Step Concurrency (Extension)

Loop steps can optionally run iterations in parallel — independent of the outer DAG
worker pool. This is a significant capability with important correctness constraints.

**Configuration per Loop step — UI:**

```
┌─ Advanced ──────────────────────────────────────────────────────┐
│                                                                   │
│  ☐ Enable parallel iteration                                      │
│                                                                   │
│    ⚠ Only enable if iterations are fully independent.            │
│      Shared variables, accumulators, and ordered writes          │
│      will produce incorrect results in parallel mode.            │
│      Sequential mode is always safe.              [Learn more]   │
│                                                                   │
│  (shown only when checkbox is checked)                           │
│  Max Concurrent Iterations  [5    ]                              │
│  On Error   ○ Fail Fast  ○ Continue  ○ Suppress                 │
│  Output     ○ Collect All  ○ Merge  ○ Discard                   │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

- **Unchecked (default)**: Sequential execution. Always correct. No configuration needed.
- **Checked**: Parallel execution. User accepts responsibility for iteration independence.
  Sub-options (Max Concurrency, On Error, Output Strategy) appear only when checked.
- The warning text is always visible when the checkbox is checked — not hidden.

---

##### Loop Counter ({{index}} / {{ctr}}) in Parallel Mode

`{{index}}` and `{{ctr}}` are always safe in parallel — each iteration receives its
own immutable copy as part of `IterationContext`. Using them to access data is safe:

```
items[{{index}}]          ✓ safe — each iteration reads its own item
"Processing {{index}} of {{total}}"   ✓ safe — read-only reference
offset = {{index}} * pageSize         ✓ safe — pure calculation
```

The counter value itself is never shared or mutated across iterations.

---

##### Cases Where Parallel Iterations Are Harmful

**Case 1 — Running accumulator (SILENT DATA LOSS)**

```
// Sequential intent: accumulate a total across all items
total = {{pipeline.total}} + {{item.amount}}
Set pipeline.total = total
```

In parallel: all iterations read `pipeline.total` at the same snapshot value (say 0).
Each independently calculates `0 + their_amount`. Last write wins.
Result: only one item's amount is recorded. No error raised. Silent data loss.

**Case 2 — Reference to previous iteration's output (RUNTIME ERROR or STALE DATA)**

```
// Iteration N uses the result of iteration N-1
previousResult = steps.transform.output  // from last iteration
result = combine(previousResult, {{item}})
```

In parallel: "last iteration's output" is undefined — N-1 may not have run yet.
The step either reads stale data from before the loop, or reads another iteration's
output that happened to finish first. Non-deterministic and wrong.

**Case 3 — Index-as-gate / first-iteration-initializes pattern (RACE CONDITION)**

```
if {{index}} == 0 {
    initialize shared resource
}
// other iterations assume resource is initialized
use shared resource
```

In parallel: iterations 1-N may reach "use shared resource" before iteration 0's
initialization step completes. The assumption that index=0 runs first is broken.

**Case 4 — Ordered writes to an external system (ORDERING VIOLATION)**

```
Loop over records, parallel:
  Write record to audit log with sequence number {{index}}
```

The audit log receives writes in arbitrary order — record 7 before record 3 —
even though `{{index}}` values are correct. If the downstream system requires
ordered ingestion, the data is corrupted.

**Case 5 — State machine / chained transformation (WRONG RESULT)**

```
Loop over state transitions:
  currentState = applyTransition({{currentState}}, {{item.event}})
```

Each transition must start from the state the previous transition left.
In parallel, all iterations start from the same initial state snapshot.
Only the last-completing iteration's final state survives. All intermediate
transitions are silently discarded.

---

##### Guardrails: What the System Must Enforce

Simply letting users toggle "Parallel" without protection would allow all five cases
above to produce silently wrong results with no error. This is unacceptable.

**Guardrail 1 — Snapshot isolation for reads**

At loop start, before any iteration begins, take a read-only snapshot of:
- `execCtx.Message` (the shared message state)
- `VariableContext` (pipeline-level variables)

Every parallel iteration reads from this snapshot — not from live state that other
iterations are mutating. This prevents Case 2 (stale reads) and makes Case 1 visible
(all iterations get the same initial value, making the accumulator bug obvious in testing).

**Guardrail 2 — Write scope enforcement**

In Parallel mode, iterations may only write to:
- Their own `LocalOutputs` (always safe — isolated namespace per iteration)
- Their own `{{item}}` variable (local copy)

In Parallel mode, iterations may NOT write to:
- `pipeline.*` shared variables
- `execCtx.Message` shared fields

Attempting a write to shared scope in Parallel mode → runtime warning + write is
dropped (not silently applied). The user sees: "Iteration 3 attempted to write
pipeline.total in Parallel mode — ignored. Switch to Sequential if shared state
is required."

**Guardrail 3 — Save-time static analysis warning**

When a Loop is set to Parallel mode, scan the body steps' configs for:
- Reads of `steps.{anyAlias}` that refer to a step inside the same loop body
  → warn: "Step X reads from Step Y inside the same loop. In Parallel mode,
  Y may not have completed before X reads its output."
- Writes to `pipeline.*` variables
  → warn: "Step X writes to pipeline.total. In Parallel mode, concurrent writes
  to shared variables produce unpredictable results. Use Sequential mode or
  the Collect All output strategy instead."

These are warnings, not blocks — experienced users may have verified their logic
is safe. But they are shown prominently.

**Guardrail 4 — Parallel mode is opt-in, Sequential is always the default**

The UI defaults to Sequential. Parallel requires an explicit choice. The mode
selector shows a brief explanation of the tradeoff when Parallel is chosen.

---

##### Safe Parallel Patterns (No Guardrails Needed)

```
✓ Each iteration enriches its own item from an external API
  (reads {{item}}, writes to LocalOutputs only)

✓ Each iteration sends its item to an outbound endpoint
  (side-effect only, Discard output strategy)

✓ Each iteration transforms its item independently
  (reads {{item}} + ParentContext snapshot, writes to LocalOutputs)

✓ Collect All: aggregate results into {{loop.results}} array after all complete
  (merge happens after parallel phase, no concurrent writes)
```

---

**When `SelfManaged` + parallel iterations are combined**: the DAG worker is released
immediately; the Loop manages its own internal semaphore pool of size `Max Concurrency`;
signals the DAG `done` channel when all iterations complete (or fail-fast triggers).
Loop concurrency is fully decoupled from the outer DAG worker count and from other
interface worker pools.

---

### Gap 3 — connector.inbound Must Be Pre-Completed (CORRECTNESS)

**Problem**: `connector.inbound` is always a root step (no predecessors) but it has
already been executed — it received the message and triggered the pipeline. If the
DAG executor tries to execute it again, it re-runs the connector (wrong).

**Resolution**: Before DAG execution begins, mark `connector.inbound` steps as
pre-completed with a synthetic result, decrement pending counts of their successors,
and enqueue any successors that become ready.

```go
func (dag *StepDAG) Execute(...) error {
    // Phase 0: Pre-complete special steps
    for _, step := range steps {
        if step.StepType == "connector.inbound" {
            dag.markComplete(step.ID, syntheticResult(step), &pending, queue)
        }
    }

    // Phase 1: Enqueue remaining roots (non-connector roots)
    for _, rootID := range dag.roots {
        if !preCompleted[rootID] {
            queue <- rootID
        }
    }

    // ... rest of execution
}
```

The inbound message (already in `execCtx.Message`) is available to all steps from
time zero. `connector.inbound`'s pre-completion immediately unblocks all steps that
only had it as a predecessor.

---

### Gap 4 — Skip Cascade Not Fully Designed (CORRECTNESS)

**Problem**: When a Switch/Case step marks a branch as "skip", those skipped steps
may themselves be predecessors of further steps. The pending count of those further
steps must also be decremented. The cascade can be N levels deep.

**Resolution**: Skip propagation is a BFS (breadth-first search) through the DAG,
starting from the directly-skipped steps, following successor edges. A step is
cascade-skipped if ALL of its non-skipped predecessors have also been skipped.
(If a step has both a skipped predecessor and a non-skipped predecessor, it is
NOT cascade-skipped — it waits for the non-skipped predecessor normally.)

```go
func (dag *StepDAG) propagateSkips(
    directSkips []string,
    pending map[string]int,
    skipped map[string]bool,
    queue chan string,
) {
    worklist := directSkips
    for len(worklist) > 0 {
        stepID := worklist[0]
        worklist = worklist[1:]

        skipped[stepID] = true
        // Write synthetic skip result to StepOutputs
        execCtx.StepOutputs[stepID] = SkippedResult(stepID)

        for _, successorID := range dag.successors[stepID] {
            pending[successorID]--
            if pending[successorID] == 0 {
                if allPredecessorsSkipped(successorID, dag, skipped) {
                    // Cascade: this successor is also skipped
                    worklist = append(worklist, successorID)
                } else {
                    // Has a non-skipped predecessor that completed normally → run it
                    queue <- successorID
                }
            }
        }
    }
}
```

Cascade skipping is synchronous in the scheduler goroutine — no worker involved.

---

### Gap 5 — Sequential Pipeline Overhead (PERFORMANCE)

**Problem**: For a purely linear pipeline (A→B→C→D), the DAG executor adds unnecessary
overhead per step: channel send/receive, mutex lock/unlock, scheduler iteration. The
current sequential loop has none of this. For healthcare HL7 pipelines that are mostly
linear (validate → enrich → map → send), this overhead accumulates.

**Resolution: Fast path for linear pipelines**

At DAG build time, detect if the pipeline is a simple chain:
each step has at most one predecessor and one successor, no floating steps, no
convergence points. If so, use the existing sequential executor path unchanged.

```go
func BuildDAG(steps []TransformationStep, connections []Connection) *StepDAG {
    dag := buildGraph(steps, connections)
    dag.isLinear = dag.detectLinear()  // O(N) check
    return dag
}

func (dag *StepDAG) Execute(...) error {
    if dag.isLinear {
        return dag.executeLinear(ctx, execCtx)  // existing sequential logic
    }
    return dag.executeParallel(ctx, execCtx, workerCount)
}
```

Linear detection: all steps have `len(predecessors) <= 1` AND `len(successors) <= 1`
AND `len(roots) == 1`. This covers the majority of existing pipelines — zero
performance regression.

---

## Corrected Execution Engine

Incorporating all gap resolutions:

```
Startup:
  1. Build DAG from connections column
  2. If linear → fast path (existing sequential loop)
  3. If parallel → DAG executor:
     a. Pre-complete connector.inbound steps, propagate to successors
     b. Enqueue remaining roots (zero-predecessor non-connector steps)

Scheduler goroutine (main goroutine):
  - Reads from done channel
  - On normal completion: store output, decrement successors, enqueue ready ones
  - On skip result: run propagateSkips() BFS, enqueue any newly-ready non-skipped steps
  - On error: check step's onError strategy (suppress / rethrow)
  - Loop until completed == total_steps

Worker goroutines (N workers):
  - Read stepID from queue
  - SelfManaged steps (containers): launch internal goroutine, release worker immediately
  - Standard steps: execute, write to done

Concurrency safety:
  - execCtx.Message: sync.Map (concurrent-safe reads and writes)
  - StepOutputs: separate namespace keys per step, written once, no conflict
  - pending[]: modified only in scheduler goroutine (single writer, no mutex needed)
  - VariableContext: existing thread-safe implementation unchanged
```

---

## What Does NOT Change

- Container step internal execution (Loop iterations, Try-Catch zones): unchanged, sequential inside the container
- Error handling per step (`onError: suppress | rethrow`): unchanged
- Retry logic: unchanged, runs inside step execution transparent to the DAG
- ACK/NACK for MLLP: sent before DAG execution begins, unchanged
- Step config format: unchanged, no migration needed
- `StepOutputs` namespace format (`alias_shortID`): unchanged

---

## Pipeline Builder UI Changes

### Visual Indicators (minimal additions)

| Indicator | When shown | Meaning |
|-----------|------------|---------|
| `▶ Entry` badge | Zero incoming connections, non-connector | Starts at time zero |
| `⏳ Waits for N` badge | N > 1 incoming connections | Convergence point |

### Save-Time Validation

**Cycle detection** (hard block):
```
⛔ Cannot save: circular dependency detected.
   Map to FHIR → Validate → Map to FHIR. Remove one connection.
```

**Parallel write conflict** (soft warning):
```
⚠ "Enrich Epic" and "Enrich Lab" both write to "patient.mrn".
  Running in parallel — last to finish wins.
```

**Missing data path** (soft warning):
```
⚠ "Map to FHIR" reads steps.epic_enrich.patient_id but has no
  connection from "Enrich from Epic". Add a connection or the
  data may not be available when this step runs.
```

### No New Drawing Mechanics

Users draw connections as today. Parallel paths = multiple outgoing arrows from one
step. Convergence = multiple incoming arrows to one step. Already supported by
FlowchartOrchestratorV2.

---

## Data Availability Model

| Data | Available to | When | Thread-safe? |
|------|-------------|------|-------------|
| Original raw message | All steps | Time zero | Yes (sync.Map) |
| `steps.{alias}.{field}` | Steps with path from that alias | After predecessor completes | Yes (written once, read after) |
| Shared message map mutations | Successors only | After predecessor completes | Yes (sync.Map) |
| Global pipeline variables | All steps | Any time | Yes (existing VariableContext) |

---

## Implementation Plan

### Files to Create
- `services/dag_executor.go` — `BuildDAG()`, `StepDAG.Execute()`, `propagateSkips()`
- `services/dag_validator.go` — cycle detection, write-conflict warning, missing-path warning

### Files to Modify

| File | Change |
|------|--------|
| `services/transformation_pipeline_service.go` | Replace sequence-loop with `dag.Execute()` |
| `models/transformation_models.go` | Change `Message map[string]interface{}` → `Message sync.Map` in `PipelineExecutionContext` |
| `controllers/pipeline_controller.go` | Add cycle-detection on pipeline save |
| `public/js/pipeline/PipelineBuilder.js` | Entry/Wait badges, wire save-time validation errors |

### Migration Required
**None.** `connections` JSONB column (V42) already stores all edge data.
`StepOutputs` / `StepAlias` / namespace system already exists.

### Backward Compatibility
Linear pipelines: fast path → identical behaviour, zero overhead added.
Parallel pipelines: new behaviour, but only pipelines the user explicitly designed
with parallel connections. No existing pipeline is affected.

---

## Known Remaining Tradeoffs

| Tradeoff | Decision | Rationale |
|----------|----------|-----------|
| Loop parallel iteration | Opt-in checkbox, unchecked by default; guardrails active when enabled | User explicitly accepts responsibility; sequential default is always safe |
| `sync.Map` is slower than plain map for sequential access | Fast path uses plain map | Sequential pipelines (the majority) are unaffected |
| Parallel write conflicts resolved by last-write-wins | Warn at save time, allow at runtime | Conflicts should not happen if pipeline is designed correctly; hard-blocking would be too restrictive |
| DAG build cost per pipeline execution | Cache DAG with pipeline definition (invalidated on save) | Build cost is O(steps + connections), negligible; cache eliminates it entirely |
