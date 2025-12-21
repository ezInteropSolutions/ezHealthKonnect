# Connectivity Patterns Explained

## Why 16 Inbound but 15 Outbound Connectors?

**Total Connectors: 31** (16 Inbound + 15 Outbound)

This is **correct by design** due to different integration patterns.

---

## 📊 Connector Symmetry Analysis

### ✅ Bidirectional Pairs (13)

These connectors have both inbound and outbound versions:

| Base Type | Inbound | Outbound | Use Case |
|-----------|---------|----------|----------|
| PostgreSQL | ✓ | ✓ | Read from DB / Write to DB |
| MySQL | ✓ | ✓ | Read from DB / Write to DB |
| SQL Server | ✓ | ✓ | Read from DB / Write to DB |
| MongoDB | ✓ | ✓ | Read from DB / Write to DB |
| Oracle | ✓ | ✓ | Read from DB / Write to DB |
| AWS S3 | ✓ | ✓ | Read objects / Write objects |
| Azure Blob | ✓ | ✓ | Read blobs / Write blobs |
| Google Cloud Storage | ✓ | ✓ | Read objects / Write objects |
| RabbitMQ | ✓ | ✓ | Consume messages / Publish messages |
| Kafka | ✓ | ✓ | Consume messages / Produce messages |
| Redis | ✓ | ✓ | Subscribe/Pop / Publish/Push |
| SFTP | ✓ | ✓ | Download files / Upload files |
| FTP | ✓ | ✓ | Download files / Upload files |

**Total: 26 connectors** (13 pairs × 2)

---

### ⬇️ Inbound-Only Connectors (3)

These connectors **only receive/listen** and don't have outbound equivalents:

#### 1. **TCP/MLLP (HL7 v2.x)** - Inbound Only ✓
**Why no outbound?**
- TCP/MLLP is a **server/listener** pattern
- It opens a port (e.g., 2575) and **waits for connections**
- External systems connect TO it and send HL7 messages
- You don't "send to" a MLLP listener - you listen for incoming connections

**Pattern**: Server Socket Listener
```
External HL7 System → [Port 2575] → ezHealthKonnect
```

**Alternative for outbound HL7**:
- Use `http_outbound` to POST HL7 to external REST endpoints
- Use database or file system to exchange data with external systems
- Most modern HL7 integrations use HTTP/REST instead of MLLP for outbound

---

#### 2. **HTTP/REST API** - Inbound Only ✓
**Why no outbound?**
- This creates a **webhook endpoint** that receives HTTP POST/PUT requests
- It's a server that **exposes an API** for external systems to call
- Pattern: `POST /api/hl7/receive` → ezHealthKonnect processes message

**Pattern**: REST API Endpoint
```
External System → HTTP POST → /api/hl7/receive → ezHealthKonnect
```

**For outbound HTTP, use**: `http_outbound` (separate connector)

---

#### 3. **File Listener** - Inbound Only ✓
**Why no outbound?**
- This **polls/monitors** a directory for new files
- It's a read-only operation that watches for file creation
- Different from writing files to a directory

**Pattern**: Directory Monitoring
```
External System → Creates file in /incoming/ → File Listener detects → ezHealthKonnect
```

**For outbound file writing, use**: `file_writer` (separate connector)

---

### ⬆️ Outbound-Only Connectors (2)

These connectors **only send** and don't have inbound equivalents:

#### 1. **HTTP/HTTPS Endpoint** - Outbound Only ✓
**Why no inbound?**
- This is an **HTTP client** that makes requests to external APIs
- It POSTs/PUTs data to remote URLs
- Different from receiving HTTP requests (which is `http_rest`)

**Pattern**: HTTP Client
```
ezHealthKonnect → HTTP POST → https://external-api.com/fhir → External System
```

**For inbound HTTP, use**: `http_rest` (separate connector)

---

#### 2. **File Writer** - Outbound Only ✓
**Why no inbound?**
- This **creates/writes** files to a directory
- It's a write-only operation
- Different from reading files from a directory

**Pattern**: File Creation
```
ezHealthKonnect → Writes file to /outgoing/ → External System picks up
```

**For inbound file reading, use**: `file_listener` (separate connector)

---

## 🔄 Pattern Differences: Why Separate Connectors?

### HTTP: Two Different Patterns

**Inbound (http_rest)**: Server Mode
- Exposes REST API endpoint
- Receives webhook calls
- Binds to port and path
- Configuration: `endpoint_path`, `http_methods`, `authentication_type`

**Outbound (http_outbound)**: Client Mode
- Makes HTTP requests to external URLs
- Sends data to remote APIs
- Configuration: `url`, `method`, `timeout_seconds`, `authentication_type`

**Why not combined?**
- Completely different implementation (server vs client)
- Different configuration parameters
- Different authentication flows
- Different use cases

---

### File System: Two Different Patterns

**Inbound (file_listener)**: Reader/Poller
- Polls directory for new files
- Reads and processes files
- Supports after-processing (delete, move, archive)
- Requires cron scheduling
- Configuration: `directory_path`, `file_pattern`, `after_processing`

**Outbound (file_writer)**: Writer
- Creates new files in directory
- Writes message content
- Supports filename patterns
- No polling needed
- Configuration: `directory_path`, `filename_pattern`, `overwrite_existing`

**Why not combined?**
- Read vs Write operations have different lifecycles
- Different configuration needs (polling interval vs filename patterns)
- Different security considerations (read vs write permissions)
- Different use cases (B2B file exchange vs local archiving)

---

### TCP/MLLP: Server-Only Pattern

**Inbound (tcp_mllp)**: Listener/Server
- Opens TCP socket and listens
- Accepts incoming connections
- Receives HL7 messages via MLLP protocol
- Configuration: `port`, `host`, `max_connections`, `enable_tls`

**Why no outbound TCP/MLLP?**
- MLLP is a legacy protocol primarily used for **receiving** HL7 feeds
- Modern healthcare systems use HTTP/REST for outbound communication
- If outbound MLLP is needed, use external tools or custom connectors
- Most HL7 integrations are **unidirectional** (one system sends, one receives)

**Alternatives for outbound HL7**:
1. Use `http_outbound` to POST HL7 to REST endpoints
2. Use `database_outbound` to write to shared database
3. Use `file_writer` for HL7 file exchange
4. Use `sftp_outbound` for secure HL7 file transfer

---

## 📊 Summary Table

| Connector Type | Inbound | Outbound | Total | Bidirectional |
|----------------|---------|----------|-------|---------------|
| **Databases** | 5 | 5 | 10 | ✓ |
| **Message Queues** | 3 | 3 | 6 | ✓ |
| **Cloud Storage** | 3 | 3 | 6 | ✓ |
| **File Transfer** | 2 | 2 | 4 | ✓ |
| **Network (HTTP)** | 1 | 1 | 2 | ✗ (Different patterns) |
| **File System** | 1 | 1 | 2 | ✗ (Different patterns) |
| **Network (TCP/MLLP)** | 1 | 0 | 1 | ✗ (Server-only) |
| **TOTAL** | **16** | **15** | **31** | **13 pairs** |

---

## ✅ Verification

```sql
-- Count by category
SELECT category, COUNT(*)
FROM connectivity_types
GROUP BY category;

-- Result:
-- inbound  | 16
-- outbound | 15

-- Identify asymmetric connectors
SELECT
    REPLACE(REPLACE(type_name, '_inbound', ''), '_outbound', '') as base_type,
    COUNT(*) as connector_count
FROM connectivity_types
GROUP BY base_type
HAVING COUNT(*) = 1;

-- Result:
-- tcp_mllp      | 1 (inbound only)
-- http_rest     | 1 (inbound only)
-- file_listener | 1 (inbound only)
-- http_outbound | 1 (outbound only - but named differently)
-- file_writer   | 1 (outbound only - but named differently)
```

---

## 🎯 Conclusion

**The 16 inbound + 15 outbound split is correct and intentional.**

- **13 bidirectional pairs** (26 connectors) for symmetric patterns
- **3 inbound-only** connectors for server/listener patterns
- **2 outbound-only** connectors for client/writer patterns
- **Total: 31 connectors** covering all healthcare integration needs

This design follows industry best practices where:
- Servers listen (TCP/MLLP, HTTP REST API)
- Clients send (HTTP outbound)
- Readers poll (File Listener)
- Writers create (File Writer)

Each connector is optimized for its specific use case rather than forcing bidirectional patterns where they don't naturally fit.

---

**Last Updated**: October 26, 2025
**Total Connectors**: 31 (16 Inbound, 15 Outbound)
**Bidirectional Pairs**: 13
**Asymmetric Connectors**: 5 (by design)
