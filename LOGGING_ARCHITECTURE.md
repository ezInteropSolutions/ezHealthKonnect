# Logging Architecture

## Overview
Interface-level logging system that captures detailed processing logs for troubleshooting. Users can toggle between error-only mode (default) and debug mode per interface.

## Database Schema

### PostgreSQL: Interface Configuration
```sql
-- interfaces table (logging settings)
CREATE TABLE interfaces (
    -- ... existing columns ...
    debug_logging BOOLEAN DEFAULT FALSE,
    log_retention_days INTEGER DEFAULT 30,
    retain_error_logs_forever BOOLEAN DEFAULT TRUE
);
```

### MongoDB: Log Storage
```javascript
// Collection: processing_logs_intf_<interface-id>
// One collection per interface for performance isolation

{
    _id: ObjectId("..."),
    message_id: "tcp_msg_1234567890",
    correlation_id: "corr_abc123",
    log_level: "error|warning|info|debug",
    category: "connection|parsing|validation|transformation|delivery",
    timestamp: ISODate("2025-01-23T10:30:45.123Z"),
    message: "Failed to connect to destination endpoint",
    details: {
        step: "TCP Connection",
        operation: "connect",
        endpoint: "192.168.1.100:2575",
        error_code: "ECONNREFUSED",
        stack_trace: "...",
        duration_ms: 5000
    },
    context: {
        interface_id: "dee0c902-...",
        interface_name: "Test Interface7",
        message_type: "ADT^A01",
        source_ip: "192.168.1.50"
    }
}
```

### Log Categories

1. **connection**: Inbound/outbound connection events
   - TCP listener start/stop
   - Destination connection attempts
   - TLS handshake
   - Keep-alive pings

2. **parsing**: Message parsing and validation
   - HL7 structure validation
   - Segment parsing
   - Field extraction
   - Data type validation

3. **validation**: Business rule validation
   - Required field checks
   - Value range validation
   - Cross-field dependencies
   - Schema compliance

4. **transformation**: Field-level transformations
   - HL7 → FHIR mapping execution
   - Field value conversions
   - Data enrichment
   - Lookup operations

5. **delivery**: Message delivery to destination
   - HTTP POST attempts
   - TCP/MLLP delivery
   - Response handling
   - Retry logic
   - Acknowledgments (ACK/NACK)

### Log Levels

- **error**: Critical failures preventing processing
- **warning**: Non-critical issues (e.g., missing optional fields)
- **info**: Important milestones (e.g., message received, transformation complete)
- **debug**: Detailed operations (e.g., every field mapping, connection details)

## Logging Behavior

### Error Mode (default: debug_logging = false)
```
✅ Captured:
- error: All errors and exceptions
- warning: All warnings and alerts

❌ Not Captured:
- info: Milestone events
- debug: Detailed operations
```

### Debug Mode (debug_logging = true)
```
✅ Captured:
- error: All errors
- warning: All warnings
- info: Processing milestones
- debug: Every operation detail

Examples:
- "Parsing MSH segment..."
- "Mapping PID.3.1 → Patient.identifier[0].value: '12345'"
- "Connecting to 192.168.1.100:2575..."
- "Transformation completed in 123ms"
```

## Log Retention

### Default Policy
- **Debug/Info logs**: Deleted after `log_retention_days` (default: 30 days)
- **Error logs**: Retained forever if `retain_error_logs_forever = true`
- **Warning logs**: Follow debug/info retention policy

### Configurable Per Interface
```javascript
{
    debug_logging: true,              // Enable debug mode
    log_retention_days: 90,           // Keep logs for 90 days
    retain_error_logs_forever: true   // Never delete errors
}
```

### Cleanup Job
- Runs daily via cron
- Queries MongoDB for logs older than retention policy
- Deletes based on log_level and timestamp
- Respects `retain_error_logs_forever` flag

## API Endpoints

### Get Logs for Message
```
GET /api/messages/:messageId/logs?level=all|error
```

**Response**:
```json
{
    "success": true,
    "data": {
        "logs": [
            {
                "timestamp": "2025-01-23T10:30:45.123Z",
                "level": "info",
                "category": "transformation",
                "message": "HL7 to FHIR transformation completed",
                "details": {
                    "duration_ms": 123,
                    "fields_mapped": 45
                }
            },
            {
                "timestamp": "2025-01-23T10:30:45.890Z",
                "level": "error",
                "category": "delivery",
                "message": "Failed to deliver to destination",
                "details": {
                    "endpoint": "http://fhir.example.com/Bundle",
                    "error": "Connection timeout",
                    "retry_attempt": 1
                }
            }
        ],
        "summary": {
            "total": 15,
            "errors": 1,
            "warnings": 0,
            "info": 5,
            "debug": 9
        }
    }
}
```

## Go Implementation (Processing Engine)

### Logger Interface
```go
type ProcessingLogger struct {
    interfaceID     string
    messageID       string
    correlationID   string
    debugMode       bool
    mongoClient     *mongo.Client
}

func (l *ProcessingLogger) Error(category, message string, details map[string]interface{})
func (l *ProcessingLogger) Warning(category, message string, details map[string]interface{})
func (l *ProcessingLogger) Info(category, message string, details map[string]interface{})
func (l *ProcessingLogger) Debug(category, message string, details map[string]interface{})
```

### Usage Example
```go
logger := NewProcessingLogger(interfaceID, messageID, debugMode)

// Connection attempt
logger.Info("connection", "Connecting to destination", map[string]interface{}{
    "endpoint": "192.168.1.100:2575",
    "protocol": "TCP/MLLP",
})

// Field transformation (only in debug mode)
logger.Debug("transformation", "Field mapping executed", map[string]interface{}{
    "source": "PID.3.1",
    "target": "Patient.identifier[0].value",
    "value": "12345",
    "mapping_rule": "direct_copy",
})

// Error
logger.Error("delivery", "Failed to deliver message", map[string]interface{}{
    "endpoint": "http://fhir.example.com/Bundle",
    "status_code": 500,
    "error": "Internal Server Error",
    "retry_attempt": 2,
})
```

## UI Components

### Interface Configuration
```
┌─────────────────────────────────────────────────────┐
│ Logging & Troubleshooting                           │
├─────────────────────────────────────────────────────┤
│                                                      │
│ ☐ Enable Debug Logging                              │
│   Captures detailed logs for all operations         │
│   (Increases storage usage)                          │
│                                                      │
│ Log Retention: [30] days                             │
│                                                      │
│ ☑ Retain Error Logs Forever                         │
│   Error logs will never be deleted                  │
│                                                      │
└─────────────────────────────────────────────────────┘
```

### Message Detail - Logs Tab
```
┌─────────────────────────────────────────────────────┐
│ Processing Logs                   [Errors Only ▼]   │
├─────────────────────────────────────────────────────┤
│                                                      │
│ ⚠️  10:30:45.890 | Delivery Error                   │
│     Failed to connect to destination                │
│     Endpoint: 192.168.1.100:2575                    │
│     Error: Connection refused                       │
│                                                      │
│ ✓  10:30:45.123 | Transformation Complete           │
│     Mapped 45 fields in 123ms                       │
│                                                      │
│ ℹ️  10:30:44.567 | Message Received                 │
│     Source: TCP/MLLP Listener                       │
│     Size: 2.4 KB                                    │
│                                                      │
└─────────────────────────────────────────────────────┘
```

## Performance Considerations

1. **Collection per Interface**: Prevents cross-interface query slowdowns
2. **Indexed Queries**: message_id, timestamp, log_level indexes
3. **Async Writing**: Logs written asynchronously to avoid blocking processing
4. **Batch Cleanup**: Delete logs in batches to avoid long-running operations
5. **Debug Mode Warning**: UI shows warning about increased storage when enabled

## Storage Estimates

### Error Mode (default)
- ~100 bytes per error log entry
- Low volume interfaces: ~10 MB/month
- High volume (1000 msg/day, 5% error rate): ~150 MB/month

### Debug Mode
- ~500 bytes per log entry
- ~50-100 log entries per message
- High volume (1000 msg/day): ~1.5-3 GB/month

## Migration Path

1. ✅ V30 migration adds columns to interfaces table
2. Create MongoDB collections on-demand (first log per interface)
3. Implement ProcessingLogger in Go engine
4. Add UI toggle in interface configuration
5. Implement log retention cleanup job
6. Update message detail Logs tab to fetch from MongoDB
