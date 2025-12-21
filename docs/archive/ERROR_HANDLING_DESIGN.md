# Error Handling & Message Isolation Architecture

## Overview
Comprehensive error capture and isolation system inspired by Mirth Connect's error handling, ensuring one failing message never blocks others.

## Design Principles

1. **Message Isolation**: Each message processes independently in its own goroutine with panic recovery
2. **Error Capture**: All errors, warnings, and exceptions captured at every stage
3. **Error Context**: Stack traces, timestamps, and stage information attached to errors
4. **Non-Blocking**: Critical errors (OOM, panic) are caught and logged without stopping the engine
5. **Error Visibility**: All errors visible in UI with full diagnostic information
6. **Graceful Degradation**: System continues processing even during partial failures

## Error Types

### 1. Processing Errors (Recoverable)
- Invalid HL7 format
- Missing required fields
- Transformation failures
- Validation errors
- **Action**: Mark message as failed, continue processing queue

### 2. System Errors (Recoverable)
- Database connection issues
- MongoDB unavailable
- Network timeouts
- **Action**: Retry with exponential backoff, mark as failed after max retries

### 3. Critical Errors (Requires Recovery)
- Out of Memory (OOM)
- Goroutine panics
- Stack overflow
- **Action**: Capture panic, log error, mark message as failed, continue engine

## Architecture

### Message Processing Flow with Error Capture

```
Message Received
    ↓
[Stage 1: Reception] ← Error Capture Wrapper
    - TCP/HTTP parsing errors
    - Protocol errors
    - Size limit errors
    ↓
[Stage 2: JSON Conversion] ← Error Capture Wrapper
    - HL7 parsing errors
    - Schema validation errors
    - Dictionary lookup errors
    ↓
[Stage 3: Transformation Pipeline] ← Error Capture Wrapper
    - Pre-validation errors
    - Mapping errors
    - Post-validation errors
    - Custom step errors
    ↓
[Stage 4: Output Delivery] ← Error Capture Wrapper
    - HTTP delivery errors
    - Network errors
    - Endpoint errors
    ↓
Success or Failed (with full error context)
```

### Error Storage Schema

#### PostgreSQL - messages_intf_* tables
```sql
-- Already exists:
error_count INTEGER DEFAULT 0
last_error_message TEXT

-- Add new columns (V23 migration):
error_stack JSONB                    -- Array of all errors from all stages
last_error_timestamp TIMESTAMP
last_error_stage VARCHAR(50)         -- Which stage failed
error_severity VARCHAR(20)           -- error, warning, critical
```

#### Error Stack Format (JSONB)
```json
[
  {
    "timestamp": "2025-10-21T10:30:15Z",
    "stage": "json_conversion",
    "severity": "error",
    "error_type": "HL7ParseError",
    "message": "Missing required field MSH.9",
    "details": "Field MSH.9 (Message Type) is required but not found",
    "stack_trace": "...",
    "recovery_action": "marked_failed"
  },
  {
    "timestamp": "2025-10-21T10:30:16Z",
    "stage": "transformation",
    "severity": "warning",
    "error_type": "ValidationWarning",
    "message": "Patient birth date format non-standard",
    "details": "Date format YYYYMMDD expected, found YYYY-MM-DD",
    "recovery_action": "auto_corrected"
  }
]
```

## Implementation Components

### 1. Error Capture Wrapper (Go)
```go
// processing/error_handler.go

type ErrorContext struct {
    MessageID    string
    Stage        string
    Timestamp    time.Time
    Severity     string // "error", "warning", "critical"
    ErrorType    string
    Message      string
    Details      string
    StackTrace   string
    RecoveryAction string
}

func (pe *ProcessingEngine) executeWithErrorCapture(
    messageID string,
    stage string,
    fn func() error,
) error {
    defer func() {
        if r := recover(); r != nil {
            // Capture panic
            err := fmt.Errorf("PANIC: %v", r)
            stack := string(debug.Stack())

            pe.captureError(messageID, ErrorContext{
                Stage:        stage,
                Severity:     "critical",
                ErrorType:    "Panic",
                Message:      err.Error(),
                StackTrace:   stack,
                RecoveryAction: "panic_recovered",
            })

            log.Printf("🚨 PANIC RECOVERED in %s for message %s: %v", stage, messageID, r)
        }
    }()

    return fn()
}
```

### 2. Error Storage Service (Go)
```go
// services/error_storage_service.go

type ErrorStorageService struct {
    db *sql.DB
}

func (ess *ErrorStorageService) CaptureError(
    interfaceID string,
    messageID string,
    errCtx ErrorContext,
) error {
    // Store in error_stack JSONB array
    // Update error_count, last_error_message, last_error_timestamp
}

func (ess *ErrorStorageService) GetMessageErrors(
    interfaceID string,
    messageID string,
) ([]ErrorContext, error) {
    // Retrieve all errors for a message
}
```

### 3. Message Isolation (Go)
```go
// processing/engine.go

func (pe *ProcessingEngine) processMessageIsolated(msg *MessageData) {
    // Each message in its own goroutine with panic recovery
    go func() {
        defer func() {
            if r := recover(); r != nil {
                pe.handleCriticalError(msg.MessageID, r)
            }
        }()

        // Process with error capture at each stage
        pe.processWithErrorHandling(msg)
    }()
}
```

### 4. Error Viewer UI (Frontend)

#### New Tab in Message Viewer
```
📊 Details | 🔍 Data Lineage | 🔄 Transformations | ⚠️ Errors & Warnings
```

#### Error Display
```
⚠️ Errors & Warnings (3 errors, 1 warning)

🔴 CRITICAL - JSON Conversion Stage
    10:30:15 AM - Panic Recovered
    Type: NullPointerException
    Message: Attempted to access nil parser
    Stack Trace: [Expandable]
    Recovery: Message marked as failed, engine continued

🔴 ERROR - Transformation Stage
    10:30:16 AM - Validation Failed
    Type: HL7ValidationError
    Message: Missing required field PID.5
    Details: Patient name (PID.5) is required for ADT^A01 messages
    Recovery: Transformation aborted

🟡 WARNING - Output Delivery
    10:30:17 AM - Network Timeout
    Type: HTTPTimeoutWarning
    Message: Endpoint responded after 5s (threshold: 3s)
    Recovery: Delivery succeeded after retry
```

## Error Recovery Strategies

### 1. Automatic Retry
- Network errors: 3 retries with exponential backoff
- Database errors: 5 retries with 1s delay
- HTTP delivery: Configurable retry policy

### 2. Dead Letter Queue (DLQ)
- Messages that fail after max retries → DLQ
- DLQ messages can be manually reprocessed
- Store original error context

### 3. Circuit Breaker
- If endpoint fails 10 times in 1 minute → circuit open
- Stop sending to endpoint for 5 minutes
- Automatic recovery when endpoint healthy

## Database Migration (V23)

```sql
-- V23__Error_Handling_Enhancement.sql

-- Add error tracking columns to message tables
ALTER TABLE messages_intf_template ADD COLUMN IF NOT EXISTS error_stack JSONB DEFAULT '[]'::jsonb;
ALTER TABLE messages_intf_template ADD COLUMN IF NOT EXISTS last_error_timestamp TIMESTAMP WITH TIME ZONE;
ALTER TABLE messages_intf_template ADD COLUMN IF NOT EXISTS last_error_stage VARCHAR(50);
ALTER TABLE messages_intf_template ADD COLUMN IF NOT EXISTS error_severity VARCHAR(20);

-- Create index for error queries
CREATE INDEX IF NOT EXISTS idx_messages_error_severity ON messages_intf_template(error_severity) WHERE error_severity IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_messages_error_timestamp ON messages_intf_template(last_error_timestamp) WHERE last_error_timestamp IS NOT NULL;

-- Function to add error to stack
CREATE OR REPLACE FUNCTION add_error_to_stack(
    p_table_name TEXT,
    p_message_id TEXT,
    p_error_context JSONB
) RETURNS VOID AS $$
DECLARE
    v_sql TEXT;
BEGIN
    v_sql := format('
        UPDATE %I
        SET
            error_stack = error_stack || $1::jsonb,
            error_count = error_count + 1,
            last_error_message = $1->>''message'',
            last_error_timestamp = ($1->>''timestamp'')::timestamp,
            last_error_stage = $1->>''stage'',
            error_severity = $1->>''severity'',
            updated_at = NOW()
        WHERE message_id = $2
    ', p_table_name);

    EXECUTE v_sql USING p_error_context, p_message_id;
END;
$$ LANGUAGE plpgsql;
```

## Monitoring & Alerts

### Error Metrics Dashboard
- Total errors by stage (last 24h)
- Critical error rate
- Most common error types
- Messages in failed state
- Error recovery success rate

### Alerting Rules
- Critical error → Immediate alert
- Error rate > 10% → Warning alert
- OOM detected → Critical alert + restart recommendation
- Circuit breaker opened → Info alert

## Benefits

1. **Resilience**: System never stops due to one bad message
2. **Debuggability**: Full error context and stack traces
3. **Observability**: Real-time error monitoring
4. **Recovery**: Automatic retry and manual reprocessing
5. **Production-Ready**: Handle edge cases gracefully

## Implementation Priority

**Phase 1 (Critical - Week 1)**:
- [ ] Error capture wrapper
- [ ] Panic recovery
- [ ] Error storage (PostgreSQL + V23 migration)
- [ ] Message isolation (goroutine per message)

**Phase 2 (High - Week 2)**:
- [ ] Error viewer UI
- [ ] Error stack display
- [ ] Retry mechanisms
- [ ] Dead letter queue

**Phase 3 (Medium - Week 3)**:
- [ ] Circuit breaker
- [ ] Error metrics dashboard
- [ ] Alerting system
- [ ] Manual reprocessing UI

---

**Next Steps**: Implement Phase 1 components for immediate resilience improvement.
