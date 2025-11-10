# Complete Message Processing Flow Analysis

## Overview

ezHealthKonnect follows a three-stage message processing architecture:

```
INGESTION → STORAGE & PARSING → TRANSFORMATION → DELIVERY
```

---

## STAGE 1: MESSAGE INGESTION

Two input connector types:

### TCP/MLLP (HL7)
- File: `processing/tcp_input_connector.go`
- Port: 2575 (default)
- Protocol: MLLP (0x0B...0x1C0x0D)
- Message fields: ID=tcp_{timestamp}, Type=hl7, Source=tcp

### HTTP/FHIR
- File: `processing/http_input_connector.go`
- Port: 8090 (default)
- Path: /fhir/r4/{resourceType}
- Message fields: ID=fhir_http_{timestamp}, Type=fhir, Source=http

### Interface Activation
File: `processing/engine.go` - ActivateInterface()

Process:
1. Query database for interface source_config
2. Detect connector type (tcp vs http)
3. CreateInputConnector(type, config)
4. Create buffered message channel (100)
5. Start connector goroutine
6. Start message processor goroutine
7. Update interface status to active

**INTEGRATION POINT 1**: Add connector type detection here

---

## STAGE 2: STORAGE & PARSING

### Dual-Storage
PostgreSQL (Sync) + MongoDB (Async)

PostgreSQL Table: messages_intf_{interface_id}
- Stores: Raw message, metadata, status
- Key columns: message_id, raw_message, source_type, message_type, parsing_status, mongo_synced

MongoDB Collection: raw_messages_intf_{interface_id}
- Stores: raw_content, parsed_content (JSON)
- Parsed content: Full enhanced schema for HL7 or JSON for FHIR

### Message Processing Pipeline
File: `processing/engine_message_processor.go` - processMessages()

Step 1: PostgreSQL store (blocking)
Step 2 (async): MongoDB store
Step 3 (async): JSON parsing (auto-detect format)

For HL7: Parse with dictionary → enhancedSegments
For FHIR: Store JSON as-is

**INTEGRATION POINT 2**: Transformation pipeline entry (currently commented out)

---

## STAGE 3: TRANSFORMATION

File: `processing/transformation_engine.go`

Status: Implemented but NOT called from message processor

Structure:
- Pipeline per interface
- Steps in sequence (10, 20, 100, 200...)
- Step types: field_mapping, fhir_build, postgres_atomic_mapping, validation, custom_code
- Error handling: skip, fail, retry, default

To enable: Uncomment line 260-262 in engine_message_processor.go

---

## STAGE 4: DELIVERY

File: `services/output_delivery_service.go`

Process:
1. CreateOutputConnector(type, config)
2. Send message
3. Handle response
4. Retry with exponential backoff on 5xx
5. Update delivery_status in database

**INTEGRATION POINT 3**: Output connector factory
File: `processing/connectors.go` - CreateOutputConnector()

Supported types:
- http/rest/api → RESTOutputConnector (implemented)
- fhir/fhir-server → FHIROutputConnector (wraps REST)
- tcp, file, database → Stubs (not implemented)

---

## ERROR HANDLING (V23)

File: `processing/error_handler.go`

Features:
- Panic recovery with stack traces
- Error capture per stage
- Message isolation (one error ≠ crash)
- Async-safe execution
- Error persistence in database

Stages:
- Database, JSONConversion, Transformation, Delivery

Severities:
- Error, Warning, Critical

---

## MESSAGE MODELS

Input (from connector):
```
Message {
  ID: string
  Content: string
  Type: "hl7" | "fhir"
  Source: "tcp" | "http"
  Metadata: map[string]interface{}
}
```

Output (for delivery):
```
ProcessedMessage {
  OriginalID: string
  ProcessedID: string
  Content: string (FHIR)
  Type: string
  Metadata: map[string]interface{}
}
```

---

## KEY INTEGRATION POINTS

### Point 1: Input Connector Selection
Location: engine.go - ActivateInterface() lines 176-203

Add detection for new input connector types:
```go
if sourceType == "newtype" && connectivity == "newconnectivity" {
    connector, err = CreateInputConnector("newtype", sourceConfig)
}
```

### Point 2: Transformation Pipeline
Location: engine_message_processor.go lines 258-262

Uncomment to enable:
```go
if pe.transformationService != nil {
    go pe.transformationService.ExecutePipeline(ctx, interfaceID, msg.ID, result)
}
```

### Point 3: Output Connector
Location: connectors.go - CreateOutputConnector() lines 91-105

Add new output type:
```go
case "newtype":
    return NewXxxOutputConnector(config)
```

---

## DATABASE SCHEMA RULE (CRITICAL)

ALL interface tables MUST use IDENTICAL schema:
- No variations
- No conditional columns
- If different → DROP and RECREATE

Standard columns (from InterfaceTableManager):
id, message_id, correlation_id, interface_id, status, priority,
received_at, source_type, source_endpoint, source_ip,
message_type, message_size, message_encoding,
raw_message, processing_completed_at, processing_time_ms,
error_count, last_error_message, delivery_status, delivery_attempts,
created_at, updated_at

V19+ additions:
parsed_at, parsing_status, parsing_time_ms, parsing_error, mongo_synced

---

## ADD NEW CONNECTOR: QUICK GUIDE

### Input Connector
1. Create processing/xxx_input_connector.go
2. Implement InputConnector interface
3. Register in factory
4. Add detection in ActivateInterface()

### Output Connector
1. Create processing/xxx_output_connector.go
2. Implement OutputConnector interface
3. Add case to CreateOutputConnector() factory

---

## FILE REFERENCE

Core Files:
- processing/engine.go (orchestrator)
- processing/engine_message_processor.go (message processor)
- processing/connectors.go (factories)
- processing/tcp_input_connector.go (MLLP)
- processing/http_input_connector.go (FHIR)
- processing/transformation_engine.go (transformation)
- services/output_delivery_service.go (delivery)
- services/message_parser_service.go (JSON parsing)
- services/error_capture_service.go (error handling)

Models:
- models/message_models.go
- models/connectivity_models.go
- models/error_models.go
- models/transformation_models.go

---

## PRESERVATION RULES

1. Schema Standardization: ALL tables identical
2. Message Isolation: One error ≠ crash
3. Async Processing: No blocking on input
4. Interface Compliance: Implement required methods
5. Data Linearity: Each stage independent
