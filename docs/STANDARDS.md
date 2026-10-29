# ezHealthKonnect — Code Standards

Every file written in this project — today and going forward — must follow these standards.
Run `./scripts/check-standards.sh` before every commit. CI enforces the same checks.

---

## Table of Contents
1. [Logging](#1-logging)
2. [Go: New Executor Files](#2-go-new-executor-files)
3. [Go: General](#3-go-general)
4. [Frontend: New Step Builder Files](#4-frontend-new-step-builder-files)
5. [Frontend: General](#5-frontend-general)
6. [Database Migrations](#6-database-migrations)
7. [Testing](#7-testing)
8. [How check-standards.sh enforces this](#8-enforcement)

---

## 1. Logging

### Rule: All logging through `services/logger` (slog-based)

**NEVER use in production Go code:**
```go
fmt.Printf(...)          // ❌
fmt.Println(...)         // ❌
log.Printf(...)          // ❌
log.Println(...)         // ❌
```

**ALWAYS use:**
```go
import "ezhealthkonnect/services/logger"

logger.Info("step executed",
    "pipeline_id", pipelineID,
    "step_name",   step.StepName,
    "step_type",   step.StepType,
    "duration_ms", elapsed.Milliseconds(),
    "success",     err == nil,
)
logger.Warn("retry triggered", "attempt", i, "max", maxRetries)
logger.Error("step failed",    "error", err)
logger.Debug("input data",     "keys", inputKeys)  // stripped in production via LOG_LEVEL
```

### Rule: Interface-level events go to `logger.ForInterface()`

Any code that processes a message for a specific interface MUST log to the interface logger:

```go
ilog := logger.ForInterface(interfaceID, interfaceName)
ilog.Info("message received",
    "message_id",   msgID,
    "message_type", msgType,
    "source_ip",    sourceIP,
    "pipeline_id",  pipelineID,
)
ilog.Error("pipeline failed",
    "message_id",   msgID,
    "error",        err,
    "step_failed",  stepName,
)
```

**Why two log streams?**
| Log | Location | Format | Purpose |
|-----|----------|--------|---------|
| Application | `logs/application/app.log` + stdout | text or JSON (LOG_FORMAT) | System health, startup, cross-interface events |
| Interface | `logs/interfaces/{id}/interface.log` | JSON (always) | Per-interface audit trail, debugging, SLA monitoring |

Interface logs are isolated — if Interface 42 floods with errors, it doesn't pollute Interface 17's log.

### Rule: Standard log fields

Every log entry in the pipeline execution path MUST include:

| Field | Type | Example |
|-------|------|---------|
| `pipeline_id` | string | `"intf-42-adt"` |
| `step_name` | string | `"api_enrichment"` |
| `step_type` | string | `"enrichment.api"` |
| `duration_ms` | int64 | `127` |
| `interface_id` | string | `"42"` (via ForInterface) |

Optional but encouraged:
- `execution_id` — correlates all steps of one pipeline run
- `message_id` — the message being processed
- `message_type` — `"ADT^A01"`, `"ORU^R01"`, etc.

### Exemptions (explicitly allowed to use fmt/log)
- `services/logger/` — bootstrap only, before logger is initialized
- `main.go` — one `log.Printf` allowed before `logger.Init()` runs
- `_test.go` files — test output is intentionally unstructured
- `services/backup/` — legacy package, pre-dates this standard

---

## 2. Go: New Executor Files

Every new step executor MUST follow this template.

### File location
```
services/executors/{category}/{step_type}_executor.go
```

Categories: `enrichment`, `transform`, `control`, `validation`

### Required structure

```go
package {category}  // e.g., package enrichment

import (
    "context"
    "ezhealthkonnect/models"
    "ezhealthkonnect/services/executors"
    "ezhealthkonnect/services/logger"
    "fmt"
    "time"
)

// MyStepExecutor handles [describe what this executor does].
type MyStepExecutor struct {
    *executors.BaseExecutor
}

func NewMyStepExecutor() *MyStepExecutor {
    return &MyStepExecutor{
        BaseExecutor: executors.NewBaseExecutor("my_step_type", models.ExecutorMetadata{
            Name:        "Human Readable Name",
            Description: "What this step does for the user",
            Version:     "1.0.0",
            Author:      "ezHealthKonnect",
            Category:    "Category",
        }),
    }
}

// Execute runs the step logic.
func (e *MyStepExecutor) Execute(
    ctx context.Context,
    step *models.TransformationStep,
    inputData map[string]interface{},
) (map[string]interface{}, error) {
    start := time.Now()

    // 1. Parse config
    // 2. Validate required fields
    // 3. Do work
    // 4. Build output

    // ✅ Always use structured logging — never fmt.Printf
    logger.Info("step completed",
        "step_name",   step.StepName,
        "step_type",   step.StepType,
        "duration_ms", time.Since(start).Milliseconds(),
    )

    return outputData, nil
}

// GetOutputVariables declares what fields this step produces.
// REQUIRED: implement this so IntelliSense works at config time.
func (e *MyStepExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
    namespace := models.GenerateStepNamespace(step.StepName, step.ID, step.StepAlias)
    base := "steps." + namespace + ".step_output"

    return []models.VariableDefinition{
        {
            Name:         "result",
            Path:         base + ".result",
            DataType:     "string",
            Description:  "The result of this step",
            UsageExample: base + ".result",
            Category:     "My Step",
        },
    }
}

// Validate checks configuration before execution.
func (e *MyStepExecutor) Validate(step *models.TransformationStep) error {
    if step.Config == nil {
        return fmt.Errorf("my_step requires config")
    }
    return nil
}
```

### Register the executor
In `services/executor_registry.go`:
```go
er.Register(category.NewMyStepExecutor())
```

### No layer field
Never set or reference `step.Layer`. It was removed in V50. Steps execute by sequence only.

---

## 3. Go: General

- **No `enriched.*` wrapper keys in executor output.** Return flat maps. The engine stores them under `steps.{ns}.step_output.*` automatically.
- **No JSON round-trip for cycle detection.** Use reflect-based depth guards (see `output_normalizer.go` pattern).
- **New DB connections go through pool registry** — never `sql.Open()` per-call in executors (see `db_pool_registry.go` pattern, Sprint 2 / P6).
- **Error handling** — always return `fmt.Errorf("context: %w", err)` so errors wrap correctly.
- **No global variables** for mutable state — use sync.Map or mutexed structs.

---

## 4. Frontend: New Step Builder Files

Every step type MUST have its own builder class file (Sprint 6 / P10).

### File location
```
public/js/pipeline/components/{StepTypeName}Builder.js
```

### Required structure

```javascript
/**
 * MyStepBuilder — configuration UI for the "my_step_type" step type.
 * Extends BaseStepConfigBuilder (Template Method pattern).
 */
class MyStepBuilder extends BaseStepConfigBuilder {

    getDefaultConfig() {
        return {
            myField: '',
            myOption: 'default'
        };
    }

    render(step) {
        const config = ConfigUtils.mergeConfig(this.getDefaultConfig(), step.config);
        return `
            <div class="step-config-section">
                <label>My Field</label>
                <input id="myField" value="${this._esc(config.myField)}" />
            </div>
        `;
    }

    attachEvents(step) {
        // Wire up all DOM event listeners here
        // Use this._container as the root
    }

    collectConfig() {
        return {
            myField: document.getElementById('myField')?.value || '',
            myOption: 'default'
        };
    }

    // Escape HTML — always escape user-facing data
    _esc(s) {
        return String(s ?? '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
    }
}

StepBuilderRegistry.register('my_step_type', MyStepBuilder);
```

### Add script tag
In `public/pipeline-builder.html`, after `StepBuilderRegistry.js`:
```html
<script src="/js/pipeline/components/MyStepBuilder.js"></script>
```

### No inline step config in PropertiesPanel.js
**Do not add step-type-specific UI code to PropertiesPanel.js.** It must go in the dedicated builder file. PropertiesPanel is a thin orchestrator only.

---

## 5. Frontend: General

- **No `step.layer`** anywhere. Removed in Sprint 1 / P12.
- **No `pipeline.layers`** getter call. Use `pipeline.executionGroups` directly.
- **`findStepPosition`** returns `{ stepIndex }` — no `layerName`.
- **Console.log**: allowed during development; must be removed before production build.
- **Error handling**: catch and display errors to user — never silently swallow.

---

## 6. Database Migrations

### Naming convention (enforced by check-standards.sh)
```
V{N}__{PascalCaseDescription}.sql
```

Examples: `V50__Remove_Layer_Column.sql`, `V51__Add_Dead_Letter_Queue.sql`

### Rules
- Never modify an existing migration file — add a new one.
- Never skip a version number (check with `check-standards.sh migrations`).
- Every migration must be self-contained and idempotent where possible (`IF EXISTS`, `IF NOT EXISTS`).
- No backward-compatibility shims — we are in development, not production.

### Current version
V50 is the latest. Next migration: **V51**.

---

## 7. Testing

### Playwright (Frontend)
- Every Sprint gets a regression spec: `tests/playwright/sprint{N}-regression.spec.js`
- Every new UI feature gets a spec in `tests/playwright/`
- Login helper: copy pattern from `pipeline-builder-sprint1.spec.js`
- On failure: screenshot saved automatically (configured in `playwright.config.js`)

Run:
```bash
npx playwright test tests/playwright/ --config=tests/playwright/playwright.config.js
```

### Go (Backend)
- Unit tests in `*_test.go` files alongside the code
- Run in Docker: `docker exec ezhealthkonnect-app sh -c "cd /app && go test ./..."`
- Race detector for concurrent code: `go test -race ./services/...`

### Backend API smoke test
```bash
curl -sf http://localhost:8080/api/system/health
curl -sf http://localhost:3000/api/interfaces
```

---

## 8. Enforcement

### Before every commit (manual)
```bash
./scripts/check-standards.sh
```

### Quick check (< 10 seconds)
```bash
./scripts/check-standards.sh quick
```

### What gets checked
| Check | Tool | Failure = block? |
|-------|------|-----------------|
| Go vet | `go vet` | Yes |
| Go build | `go build` | Yes |
| Unstructured logging | grep | Yes |
| golangci-lint (if installed) | golangci-lint | Yes |
| OOP builders exist | find | Warning |
| Migration naming | find + regex | Yes |
| Duplicate migration versions | find + sort | Yes |
| Layer field removal (P12) | grep | Yes |

### Install golangci-lint (one-time)
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### Future: Pre-commit hook
Add to `.git/hooks/pre-commit`:
```bash
#!/bin/sh
./scripts/check-standards.sh quick
```

---

*Last updated: Sprint 1 (2026-02-23). Update this document with each sprint that introduces new standards.*
