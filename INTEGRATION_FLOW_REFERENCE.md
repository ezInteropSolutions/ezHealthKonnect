# INTEGRATION FLOW REFERENCE - DO NOT BREAK!

## ⚠️ CRITICAL SYSTEM ARCHITECTURE

This document describes the **COMPLETE END-TO-END MESSAGE FLOW** in ezHealthKonnect integration engine. **DO NOT modify, disable, or remove any component without updating this document and testing the full flow.**

**Last Updated**: 2025-10-18
**Status**: ✅ PRODUCTION ACTIVE
**Maintainer**: Claude Code AI Assistant

---

## 🔴 CRITICAL RULES

1. **NEVER disable `transformationService` in `processing/engine.go`**
2. **NEVER remove automatic parsing/transformation logic from `engine_message_processor.go`**
3. **NEVER convert `.go` files to `.bak` without team approval**
4. **ALWAYS test end-to-end flow after ANY changes to processing/services**
5. **ALWAYS update this document when changing the flow**

---

## 📊 COMPLETE MESSAGE FLOW

### Flow Diagram

```
┌──────────────────────────────────────────────────────────────────────────┐
│                    HL7 MESSAGE ARRIVES (TCP/MLLP)                         │
└────────────────────────────────┬─────────────────────────────────────────┘
                                 │
                ┌────────────────▼────────────────┐
                │  tcp_input_connector.go         │
                │  - Receives MLLP message        │
                │  - Sends to messageChan         │
                │  - Sends ACK immediately        │
                └────────────────┬────────────────┘
                                 │
                ┌────────────────▼────────────────┐
                │  engine_message_processor.go    │
                │  processMessages() goroutine    │
                └────────────────┬────────────────┘
                                 │
     ┌───────────────────────────┼───────────────────────────┐
     │                           │                           │
     ▼                           ▼                           ▼
┌─────────┐              ┌──────────────┐          ┌──────────────┐
│ STEP 1  │              │   STEP 2     │          │   STEP 3     │
│  STORE  │              │    PARSE     │          │  TRANSFORM   │
└─────────┘              └──────────────┘          └──────────────┘
     │                           │                           │
     ▼                           ▼                           ▼
PostgreSQL + MongoDB      MongoDB + PostgreSQL      MongoDB (output)
                                                    + PostgreSQL (status)
```

### Detailed Step-by-Step Flow

#### **STEP 0: Message Reception**
- **File**: `processing/tcp_input_connector.go`
- **Function**: `handleConnection()`
- **Actions**:
  1. Receive MLLP-framed message
  2. Extract HL7 content
  3. Send to `messageChan` (buffered channel)
  4. **IMMEDIATELY send ACK** (don't wait for processing)
  5. Reset connection timeout

#### **STEP 1: Store Raw Message**
- **File**: `processing/engine_message_processor.go`
- **Function**: `storeMessageHybrid()`
- **Services Used**:
  - `services.HybridMessageStorage` (if MongoDB available)
  - Fallback to direct PostgreSQL insert
- **Actions**:
  1. Store metadata in PostgreSQL (`messages_intf_<interface_id>` table)
  2. Store full raw content in MongoDB (`raw_messages_<interface_id>` collection)
  3. Link via `message_id`
  4. Mark status as `received`

**PostgreSQL Columns**:
```sql
message_id, interface_id, status='received', priority, received_at,
source_type='tcp', source_endpoint, source_ip, message_type='hl7v2',
message_size, message_encoding='UTF-8', raw_message, created_at, updated_at
```

**MongoDB Document**:
```javascript
{
  message_id: "tcp_...",
  interface_id: "...",
  raw_content: "MSH|^~\\&|...",
  received_at: ISODate("..."),
  message_size: 338,
  metadata: {...}
}
```

#### **STEP 2: Parse HL7 to JSON**
- **File**: `processing/engine_message_processor.go`
- **Function**: `parseAndStoreJSON()`
- **Services Used**:
  - `hl7.ParseWithRealSchema()` (existing HL7 parser with dictionary)
  - `services.MongoDBMessageService.UpdateParsedContent()`
  - `services.InterfaceMessageService.UpdateMessageParsingStatus()`
- **Actions**:
  1. Parse HL7 using enhanced schema parser
  2. Extract `messageType` (e.g., "ADT^A01")
  3. Convert to JSON structure (preserves FULL enhanced schema)
  4. Update MongoDB with `parsed_content` field
  5. Update PostgreSQL:
     - `status = 'parsed'`
     - `parsing_status = 'success'`
     - `parsed_at = NOW()`
     - `parsing_time_ms = ...`
     - `message_type = 'ADT^A01'`

**MongoDB Update**:
```javascript
{
  // ... existing fields ...
  parsed_content: {
    enhancedSegments: {...},  // Full schema-based parsing
    segmentOrder: ["MSH", "PID", ...],
    messageType: {code: "ADT", event: "A01", ...},
    version: "2.5",
    dictionaryUsed: true,
    schemaLoaded: true
  },
  parsed_at: ISODate("..."),
  parsing_time_ms: 125
}
```

#### **STEP 3: Transform via Pipeline** (Async)
- **File**: `processing/engine_message_processor.go`
- **Function**: `transformMessageAsync()` (goroutine)
- **Services Used**:
  - `services.TransformationService.TransformStoredMessage()`
  - `services.TransformationPipelineService.ExecuteTransformation()`
  - `services.HL7FHIRTransformServiceV3.Transform()`
  - `services.OutputMessageService.StoreTransformationResult()`
- **Actions**:
  1. Retrieve `parsed_content` from MongoDB
  2. Load interface mapping from PostgreSQL (`interfaces.transformation_mapping`)
  3. Execute transformation pipeline:
     - Pre-processing validation
     - Core HL7→FHIR mapping
     - Post-processing validation
  4. Store FHIR output in MongoDB (`transformed_messages_intf_<interface_id>`)
  5. Update PostgreSQL status to `transformed`

**MongoDB Output**:
```javascript
// Collection: transformed_messages_intf_<interface_id>
{
  message_id: "out_...",
  source_message_id: "tcp_...",
  interface_id: "...",
  transformation_type: "hl7_to_fhir",
  transformed_at: ISODate("..."),
  transformed_content: {
    resourceType: "Bundle",
    type: "transaction",
    entry: [...]  // FHIR Patient, Encounter, etc.
  },
  resources_generated: ["Patient", "Encounter"]
}
```

---

## 🗂️ KEY FILES & THEIR ROLES

### Core Processing Files

| File | Role | **DO NOT TOUCH** |
|------|------|------------------|
| `processing/engine.go` | Main engine, initializes `transformationService` | ✅ CRITICAL |
| `processing/engine_message_processor.go` | **Message flow orchestrator** | ✅ CRITICAL |
| `processing/tcp_input_connector.go` | MLLP protocol handler, ACK sender | ⚠️ Modify carefully |
| `processing/connectors.go` | Connector interfaces | ✅ Stable |

### Essential Services

| File | Role | **DO NOT TOUCH** |
|------|------|------------------|
| `services/transformation_service.go` | **Main transformation orchestrator** | ✅ CRITICAL |
| `services/transformation_pipeline_service.go` | Pipeline execution engine | ✅ CRITICAL |
| `services/hybrid_message_storage.go` | PostgreSQL + MongoDB storage | ✅ CRITICAL |
| `services/mongodb_message_service.go` | MongoDB operations | ✅ CRITICAL |
| `services/interface_message_service.go` | PostgreSQL message table ops | ✅ CRITICAL |
| `services/hl7_fhir_transform_service_v3.go` | HL7→FHIR transformation logic | ⚠️ Modify carefully |

### HL7 Parsing

| File | Role | **DO NOT TOUCH** |
|------|------|------------------|
| `hl7/real_schema_parser.go` | **Schema-based HL7 parser** | ✅ CRITICAL |
| `hl7/parser.go` | Basic HL7 parser | ✅ Stable |
| `hl7/types.go` | HL7 data structures | ✅ Stable |

---

## 🔧 OOB (Out-of-Box) AUTO-INITIALIZATION

The system follows **OOB principle**: Components auto-initialize if dependencies are available.

### Auto-Init Chain

```go
main.go
  └─> ProcessingEngine = NewProcessingEngine(db)
        └─> transformationService = InitializeTransformationService(db)
              └─> mongoConnService = NewMongoDBConnectionService()  // From env vars
                    └─> Connect to MongoDB (auto-detects MONGODB_URI)
                          └─> If success: Full hybrid storage + transformation
                          └─> If fail: PostgreSQL-only mode (graceful degradation)
```

### Environment Variables Required

```bash
# MongoDB (required for transformation)
MONGODB_URI=mongodb://ezhealth_user:password@mongodb:27017/ezhealthkonnect?authSource=admin
MONGODB_DATABASE=ezhealthkonnect

# PostgreSQL (required)
DB_HOST=postgres
DB_PORT=5432
DB_NAME=ezhealthkonnect
DB_USER=ezhealth_user
DB_PASSWORD=secure_password_change_me
```

---

## 🚨 WHAT WENT WRONG (Oct 2025)

### Problem
While building the transformation pipeline UI, the following changes broke the automatic flow:

1. **`services/transformation_service.go` → `.bak`** (disabled)
2. **`processing/engine.go` line 27**: Changed to `// transformationService disabled temporarily`
3. **`processing/engine.go` line 66**: Commented out `InitializeTransformationService(db)`
4. **`processing/engine.go` line 328**: Returned error instead of calling service

### Result
- ❌ Messages received and stored
- ❌ Parsing DID NOT happen automatically
- ❌ Transformation DID NOT happen automatically
- ❌ Only ACK was sent (but channel blocking prevented even that)

### Root Cause
**Integration flow was disabled to avoid conflicts during UI development.**

---

## ✅ HOW WE FIXED IT (2025-10-18)

1. **Restored `services/transformation_service.go`** from `.bak`
2. **Re-enabled `transformationService`** in `processing/engine.go`
3. **Rewrote `engine_message_processor.go`** to wire complete flow:
   - Hybrid storage
   - Automatic parsing
   - Automatic transformation trigger
4. **Created this document** to prevent future breakage

---

## 🧪 HOW TO TEST END-TO-END FLOW

### 1. Prerequisites
```bash
# Ensure services are running
docker-compose up -d postgres mongodb app

# Verify MongoDB connection
docker-compose logs app | grep "MongoDB"
# Should see: "✅ MongoDB service initialized"

# Verify transformation service
docker-compose logs app | grep "Transformation"
# Should see: "✅ Transformation Service initialized"
```

### 2. Activate Interface
```bash
# Via UI: Go to Interfaces → Click "Start" button
# OR via API:
curl -X POST http://localhost:3000/api/interfaces/{interface_id}/activate \
  -H "Cookie: connect.sid=..."
```

### 3. Send Test Message
```powershell
# PowerShell example
$hl7 = "MSH|^~\&|HIS|RIH|EKG|EKG|202508251530||ADT^A01|MSG00001|P|2.5`r`nPID|1||123456^^^RIH^MR||Doe^John^A||19800115|M`r`nPV1|1|I|2000^2012^01||||004777^Primary^Physician"
$bytes = [System.Text.Encoding]::UTF8.GetBytes($hl7)
$mllp = [byte]0x0B + $bytes + [byte]0x1C + [byte]0x0D
$client = New-Object System.Net.Sockets.TcpClient("localhost", 6001)
$stream = $client.GetStream()
$stream.Write($mllp, 0, $mllp.Length)
$buffer = New-Object byte[] 1024
$count = $stream.Read($buffer, 0, 1024)
$ack = [System.Text.Encoding]::UTF8.GetString($buffer, 0, $count)
Write-Host "ACK: $ack"
$client.Close()
```

### 4. Verify Each Step

#### Check Storage (Step 1)
```sql
-- PostgreSQL
SELECT message_id, status, received_at
FROM messages_intf_<interface_id>
ORDER BY received_at DESC LIMIT 1;
```

```javascript
// MongoDB
db.getCollection('raw_messages_<interface_id>').findOne(
  {},
  {sort: {received_at: -1}}
)
```

#### Check Parsing (Step 2)
```sql
-- PostgreSQL
SELECT message_id, parsing_status, parsed_at, message_type
FROM messages_intf_<interface_id>
WHERE parsing_status = 'success'
ORDER BY received_at DESC LIMIT 1;
```

```javascript
// MongoDB - Check for parsed_content field
db.getCollection('raw_messages_<interface_id>').findOne(
  {parsed_content: {$exists: true}},
  {sort: {received_at: -1}}
)
```

#### Check Transformation (Step 3)
```javascript
// MongoDB
db.getCollection('transformed_messages_intf_<interface_id>').findOne(
  {},
  {sort: {transformed_at: -1}}
)
```

### 5. Expected Logs
```
📨 Message processor started for interface <id>
✅ MongoDB service initialized for parsing updates
📥 Processing message for interface <id>: ID=tcp_..., Size=332 bytes
✅ Message stored in hybrid storage (PostgreSQL + MongoDB): tcp_...
✅ Parsed JSON stored in MongoDB: tcp_...
✅ Parsing status updated in PostgreSQL: tcp_...
🔄 Triggering async transformation: tcp_...
✅ Message processed for interface <id>: ID=tcp_... (took 150ms)
✅ Transformation completed for tcp_...: Patient
```

---

## 📝 MAINTENANCE CHECKLIST

Before making changes to processing/services:

- [ ] Read this document completely
- [ ] Understand which files are CRITICAL
- [ ] If modifying critical files, create `.backup` first (NOT `.bak`)
- [ ] Test end-to-end flow after changes
- [ ] Verify all 3 steps complete successfully
- [ ] Update this document if flow changes
- [ ] Commit with clear message explaining WHY change was needed

---

## 🆘 TROUBLESHOOTING

### Problem: Messages stored but not parsed
**Check**: `docker-compose logs app | grep "MongoDB service initialized"`
- If missing → MongoDB connection failed
- **Fix**: Check `MONGODB_URI` environment variable

### Problem: Messages parsed but not transformed
**Check**: `docker-compose logs app | grep "Transformation Service"`
- If "temporarily disabled" → service was disabled
- **Fix**: Re-enable in `engine.go` (see line 67)

### Problem: No ACK received
**Check**: Channel buffer size in `engine.go` line 165
- Default is 100 messages
- If processing is slow, channel fills up
- **Fix**: Increase buffer OR optimize processing speed

### Problem: Transformation fails with "no mapping"
**Check**: PostgreSQL `interfaces` table
```sql
SELECT id, name, transformation_mapping
FROM interfaces
WHERE id = '<interface_id>';
```
- If `transformation_mapping` is NULL → Run wizard to create mapping
- **Fix**: Complete transformation wizard in UI

---

## 📚 RELATED DOCUMENTATION

- [SYSTEM_DOCUMENTATION.md](SYSTEM_DOCUMENTATION.md) - Complete system reference
- [JSON_CONVERSION_ARCHITECTURE.md](JSON_CONVERSION_ARCHITECTURE.md) - Parsing details
- [TRANSFORMATION_PIPELINE_DESIGN.md](TRANSFORMATION_PIPELINE_DESIGN.md) - Pipeline architecture
- [HYBRID_STORAGE_ARCHITECTURE.md](HYBRID_STORAGE_ARCHITECTURE.md) - Storage design
- [CLAUDE.md](CLAUDE.md) - Project overview for AI assistants

---

**🔴 REMEMBER: This is a WORKING PRODUCTION FLOW. DO NOT BREAK IT! 🔴**
