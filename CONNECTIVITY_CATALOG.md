# ezHealthKonnect Connectivity Catalog

## Complete OOB (Out-of-Box) Connector Suite

**Total Connectors: 31** (16 Inbound, 15 Outbound)
**Cron-Enabled Connectors: 11** (All pull-based inbound connectors)

---

## 📊 Connector Categories

### 1. **Database Connectors (10)** 🗄️

Enterprise-grade database integration with incremental polling and upsert support.

| Connector | Type | Cron | Key Features |
|-----------|------|------|--------------|
| **PostgreSQL Reader** | Inbound | ✅ | SSL modes, incremental polling, connection pooling |
| **PostgreSQL Writer** | Outbound | ❌ | UPSERT, batch operations, connection pooling |
| **MySQL Reader** | Inbound | ✅ | TLS support, incremental tracking, after-processing |
| **MySQL Writer** | Outbound | ❌ | REPLACE INTO, batch operations, TLS encryption |
| **SQL Server Reader** | Inbound | ✅ | Windows/SQL auth, encrypted connections, incremental |
| **SQL Server Writer** | Outbound | ❌ | MERGE operations, Windows/SQL auth, encryption |
| **MongoDB Reader** | Inbound | ✅ | Connection URI, query filters, collection archiving |
| **MongoDB Writer** | Outbound | ❌ | CRUD operations, write concern, upsert support |
| **Oracle Reader** | Inbound | ✅ | Service name connections, SSL, incremental polling |
| **Oracle Writer** | Outbound | ❌ | MERGE operations, SSL encryption, batch processing |

**Common Database Features:**
- 🔄 Incremental column tracking (id, updated_at, timestamp)
- 🏷️ After-processing options (update_flag, delete, archive, nothing)
- 🔐 SSL/TLS encryption support
- ⚡ Connection pooling and batch operations
- 📊 Max records/documents per poll configuration

---

### 2. **Message Queue Connectors (6)** 📨

Enterprise messaging systems for asynchronous communication.

| Connector | Type | Cron | Key Features |
|-----------|------|------|--------------|
| **RabbitMQ Consumer** | Inbound | ❌ | Prefetch control, auto-ack, durable queues, TLS |
| **RabbitMQ Publisher** | Outbound | ❌ | Exchange types, routing keys, persistent messages |
| **Kafka Consumer** | Inbound | ❌ | Consumer groups, offset management, SASL auth |
| **Kafka Producer** | Outbound | ❌ | Compression, batching, acknowledgment levels |
| **Redis Consumer** | Inbound | ❌ | List/Stream/Pub-Sub modes, blocking operations |
| **Redis Publisher** | Outbound | ❌ | Multiple modes, key expiry, list trimming |

**RabbitMQ Features:**
- 🔌 Virtual hosts and multiple exchange types (direct, topic, fanout, headers)
- ⚙️ Prefetch count and auto-acknowledgment control
- 💾 Durable queues and persistent messages
- 🔐 TLS/SSL encryption support

**Kafka Features:**
- 📊 Consumer groups with auto-offset management
- 🗜️ Compression (gzip, snappy, lz4, zstd)
- 🔒 SASL authentication (PLAIN, SCRAM-SHA-256, SCRAM-SHA-512)
- ⚡ Batch operations and configurable acknowledgments

**Redis Features:**
- 🔄 Multiple modes: List (LPUSH/RPOP), Pub/Sub, Streams
- 🕐 Blocking operations with timeout control
- 📦 Consumer groups for streams
- ⏱️ Key expiry and list length management

---

### 3. **Cloud Storage Connectors (6)** ☁️

Major cloud provider object storage integration.

| Connector | Type | Cron | Key Features |
|-----------|------|------|--------------|
| **AWS S3 Reader** | Inbound | ✅ | Prefix filtering, file patterns, move/delete/tag |
| **AWS S3 Writer** | Outbound | ❌ | Storage classes, SSE encryption, KMS keys |
| **Azure Blob Reader** | Inbound | ✅ | Container polling, blob patterns, archiving |
| **Azure Blob Writer** | Outbound | ❌ | Access tiers (Hot/Cool/Archive), HTTPS |
| **Google Cloud Storage Reader** | Inbound | ✅ | Service account auth, prefix filtering, archiving |
| **Google Cloud Storage Writer** | Outbound | ❌ | Storage classes, encryption, service account |

**AWS S3 Features:**
- 🔐 Multiple auth methods (access keys, IAM role, assumed role)
- 💾 Storage classes (STANDARD, INTELLIGENT_TIERING, STANDARD_IA)
- 🔒 Server-side encryption with KMS support
- 📁 After-processing (delete, move, tag, nothing)

**Azure Blob Features:**
- 🔑 Account key or connection string authentication
- ❄️ Access tiers (Hot, Cool, Archive)
- 📦 Container-based organization
- 🔐 HTTPS enforcement

**Google Cloud Storage Features:**
- 🔐 Service account JSON credentials
- 💾 Storage classes (STANDARD, NEARLINE, COLDLINE, ARCHIVE)
- 🔒 Encryption at rest
- 📁 Bucket-to-bucket archiving

---

### 4. **File Transfer Connectors (4)** 🔐

Secure file transfer protocols for B2B integration.

| Connector | Type | Cron | Key Features |
|-----------|------|------|--------------|
| **SFTP Reader** | Inbound | ✅ | SSH keys, password auth, file patterns, archiving |
| **SFTP Writer** | Outbound | ❌ | SSH keys, directory creation, file permissions |
| **FTP Reader** | Inbound | ✅ | FTPS support, passive mode, file patterns |
| **FTP Writer** | Outbound | ❌ | FTPS, binary/ASCII transfer, passive mode |

**SFTP Features:**
- 🔐 Private key authentication with passphrase support
- 📁 After-processing (delete, move, rename, nothing)
- ⚙️ File permissions configuration (octal)
- 🔒 Secure encrypted transfer

**FTP Features:**
- 🔒 FTPS (FTP over SSL) support
- 🔄 Passive and active mode support
- 📝 Binary and ASCII transfer modes
- 📁 Directory creation and archiving

---

### 5. **Network Connectors (3)** 🌐

Direct network communication protocols.

| Connector | Type | Cron | Key Features |
|-----------|------|------|--------------|
| **TCP/MLLP (HL7 v2.x)** | Inbound | ❌ | TLS 1.2/1.3, IP whitelisting, max connections |
| **HTTP/REST API** | Inbound | ❌ | Multiple HTTP methods, API key/Bearer token auth |
| **HTTP/HTTPS Endpoint** | Outbound | ❌ | POST/PUT methods, timeout, authentication |

**TCP/MLLP Features:**
- 🔐 TLS/SSL encryption (TLS 1.2, 1.3)
- 🔒 Authentication methods (basic, token, IP whitelist)
- ⚡ Max connections and timeout configuration
- 📡 HL7 v2.x MLLP protocol support

**HTTP Features:**
- 🔐 Multiple authentication types (none, basic_auth, bearer_token, api_key)
- ⚙️ Configurable HTTP methods (GET, POST, PUT)
- ⏱️ Timeout and retry configuration
- 📝 Content-Type negotiation

---

### 6. **File System Connectors (2)** 📁

Local and network file system integration.

| Connector | Type | Cron | Key Features |
|-----------|------|------|--------------|
| **File System Listener** | Inbound | ✅ | File patterns, polling, after-processing |
| **File System Writer** | Outbound | ❌ | Filename patterns, subdirectories, encoding |

**File System Features:**
- 📂 File pattern matching (*.hl7, *.xml, etc.)
- 🔄 After-processing (delete, move, archive, nothing)
- 📝 File encoding configuration (UTF-8, ASCII, etc.)
- 📁 Automatic subdirectory creation

---

## 🎯 Connector Selection Guide

### **Inbound (Receive Messages)**

**Real-time Push:**
- TCP/MLLP for HL7 feeds
- HTTP/REST API for webhook endpoints
- RabbitMQ/Kafka/Redis for message queue subscriptions

**Scheduled Polling (Cron):**
- Database readers for incremental data extraction
- Cloud storage (S3, Azure, GCS) for object processing
- File System/SFTP/FTP for file-based integration

### **Outbound (Send Messages)**

**Direct Delivery:**
- HTTP endpoints for REST API delivery
- Database writers for data persistence
- File system for local storage

**Queued Delivery:**
- RabbitMQ/Kafka/Redis for asynchronous messaging
- Cloud storage for object-based delivery
- SFTP/FTP for B2B file transfer

---

## 🔧 Configuration Examples

### PostgreSQL Inbound (Incremental Polling)
```json
{
  "host": "db.hospital.com",
  "port": 5432,
  "database": "ehr_system",
  "username": "hl7_reader",
  "password": "***",
  "query": "SELECT * FROM hl7_messages WHERE processed = false ORDER BY id",
  "incremental_column": "id",
  "after_processing": "update_flag",
  "update_column": "processed",
  "update_value": "true",
  "ssl_mode": "require",
  "max_records_per_poll": 100
}
```

### RabbitMQ Consumer
```json
{
  "host": "rabbitmq.hospital.com",
  "port": 5672,
  "username": "hl7_consumer",
  "password": "***",
  "vhost": "/hl7",
  "queue_name": "adt_messages",
  "exchange_name": "hl7_exchange",
  "routing_key": "adt.a01",
  "prefetch_count": 10,
  "auto_ack": false,
  "durable_queue": true,
  "enable_tls": true
}
```

### Kafka Producer
```json
{
  "bootstrap_servers": "kafka1:9092,kafka2:9092,kafka3:9092",
  "topic": "fhir_bundles",
  "client_id": "ezhealthkonnect_producer",
  "acks": "all",
  "compression_type": "snappy",
  "batch_size": 16384,
  "linger_ms": 10,
  "security_protocol": "SASL_SSL",
  "sasl_mechanism": "SCRAM-SHA-256",
  "sasl_username": "fhir_producer",
  "sasl_password": "***"
}
```

### AWS S3 Inbound (Scheduled)
```json
{
  "bucket_name": "hospital-hl7-feeds",
  "region": "us-east-1",
  "prefix": "incoming/adt/",
  "file_pattern": "*.hl7",
  "authentication_method": "iam_role",
  "after_processing": "move",
  "max_objects_per_poll": 100,
  "enable_server_side_encryption": true
}
```

### MongoDB Writer
```json
{
  "connection_uri": "mongodb+srv://user:pass@cluster.mongodb.net/",
  "database": "ehr_data",
  "collection": "fhir_bundles",
  "operation": "upsert",
  "unique_key_fields": ["bundle.id"],
  "write_concern": "majority",
  "batch_size": 10,
  "connection_pool_size": 5
}
```

### Azure Blob Storage Reader
```json
{
  "account_name": "hospitaldata",
  "account_key": "***",
  "container_name": "hl7-inbox",
  "prefix": "adt/",
  "blob_pattern": "*.hl7",
  "after_processing": "move",
  "archive_container": "hl7-archive",
  "max_blobs_per_poll": 100,
  "enable_https": true
}
```

---

## 🔐 Security Features

### Authentication Methods Supported
- **Basic Authentication** (username/password)
- **API Keys** (header-based)
- **Bearer Tokens** (OAuth2/JWT)
- **SSH Keys** (SFTP with passphrase)
- **TLS/SSL** (mutual TLS)
- **SASL** (Kafka SCRAM-SHA-256/512)
- **IAM Roles** (AWS)
- **Service Accounts** (GCP)
- **IP Whitelisting** (TCP/MLLP)

### Encryption Support
- ✅ TLS 1.2/1.3 for TCP/MLLP
- ✅ HTTPS for HTTP endpoints
- ✅ SSL/TLS for databases (PostgreSQL, MySQL, SQL Server, Oracle)
- ✅ TLS for RabbitMQ, Redis
- ✅ SASL_SSL for Kafka
- ✅ FTPS for FTP
- ✅ SFTP (SSH encryption)
- ✅ Server-side encryption for cloud storage (S3, Azure, GCS)

---

## 📈 Performance & Scalability

### Cron-Enabled Connectors (11)
All inbound pull-based connectors support scheduled polling:
- File System Listener
- AWS S3 Reader
- Azure Blob Reader
- Google Cloud Storage Reader
- PostgreSQL Reader
- MySQL Reader
- SQL Server Reader
- MongoDB Reader
- Oracle Reader
- SFTP Reader
- FTP Reader

### Batch Operations
- Database writers support batch inserts (1-1000 records)
- Kafka supports message batching with compression
- Cloud storage supports bulk operations
- Connection pooling for high-throughput scenarios

### Circuit Breaker Pattern
- Auto-disable failing cron jobs after threshold
- Configurable failure count before circuit break
- Automatic recovery and retry logic

---

## 🚀 API Endpoints

All connectors accessible via REST API:

```bash
# List all connectors
GET /api/connectivity/types
# Returns 31 connectors

# Filter by category
GET /api/connectivity/types/category/inbound
# Returns 16 inbound connectors

GET /api/connectivity/types/category/outbound
# Returns 15 outbound connectors

# Filter by cron support
GET /api/connectivity/types?supports_cron=true
# Returns 11 poll-based connectors

# Get specific connector by name
GET /api/connectivity/types/postgresql_inbound
GET /api/connectivity/types/kafka_outbound
GET /api/connectivity/types/azure_blob_inbound

# Configure interface connectivity
POST /api/connectivity/interfaces/{interface_id}
GET /api/connectivity/interfaces/{interface_id}
PUT /api/connectivity/interfaces/{interface_id}
DELETE /api/connectivity/interfaces/{interface_id}

# Manage cron jobs
POST /api/connectivity/interfaces/{interface_id}/cron/enable
POST /api/connectivity/interfaces/{interface_id}/cron/disable
GET /api/connectivity/interfaces/{interface_id}/cron/status

# Execution logs and statistics
GET /api/connectivity/interfaces/{interface_id}/executions
GET /api/connectivity/interfaces/{interface_id}/executions/stats
```

---

## 📋 Implementation Status

### Phase 1: Foundation (✅ Complete)
- ✅ Database migrations (V26, V27, V28, V29)
- ✅ Data models and schemas
- ✅ Service layer (CRUD operations)
- ✅ REST API controller (16 endpoints)
- ✅ 32 OOB connector definitions (16 inbound + 16 outbound)
- ✅ JSON schema validation
- ✅ Parameter grouping
- ✅ TCP/MLLP outbound added for middleware scenarios

### Phase 2A: Connector Framework (✅ Complete)
- ✅ Universal connector interface ([connector_interface.go](services/connectors/connector_interface.go))
- ✅ Base connector implementations ([base_connector.go](services/connectors/base_connector.go))
- ✅ Connector factory pattern ([connector_factory.go](services/connectors/connector_factory.go))
- ✅ All 32 connector stubs created ([connector_stubs.go](services/connectors/connector_stubs.go))
- ✅ Thread-safe state management
- ✅ Global factory registration
- ✅ Implementation guide ([CONNECTOR_IMPLEMENTATION_GUIDE.md](CONNECTOR_IMPLEMENTATION_GUIDE.md))

### Phase 2B: Connector Implementation (In Progress)
- ⏳ TCP/MLLP Inbound connector (HL7 v2.x listener)
- ⏳ TCP/MLLP Outbound connector (HL7 v2.x client)
- ⏳ HTTP Outbound connector (REST API delivery)
- ⏳ File Writer connector (local archiving)
- ⏳ Database connectors (PostgreSQL, MySQL, SQL Server, MongoDB, Oracle)
- ⏳ Message queue connectors (RabbitMQ, Kafka, Redis)
- ⏳ Cloud storage connectors (AWS S3, Azure Blob, GCS)
- ⏳ File transfer connectors (SFTP, FTP)

### Phase 2C: Integration & Testing (Planned)
- ⏳ Connect connectors to processing engine
- ⏳ Cron scheduler service integration
- ⏳ Connection testing framework
- ⏳ Unit tests for all connectors
- ⏳ Integration tests for critical paths

### Phase 3: UI & Monitoring (Planned)
- ⏳ Frontend connectivity wizard
- ⏳ Configuration form generation from schemas
- ⏳ Connection test UI
- ⏳ Execution monitoring dashboard
- ⏳ Real-time connector status display
- ⏳ End-to-end integration tests

---

## 📚 Documentation References

- **Architecture**: [CONNECTIVITY_ARCHITECTURE.md](CONNECTIVITY_ARCHITECTURE.md)
- **Cloud & Security**: [CONNECTIVITY_CLOUD_AND_SECURITY.md](CONNECTIVITY_CLOUD_AND_SECURITY.md)
- **System Documentation**: [SYSTEM_DOCUMENTATION.md](SYSTEM_DOCUMENTATION.md)
- **Database Migrations**:
  - V26: Multi-Connectivity Support
  - V27: Database Connectivity Support
  - V28: Message Queue and Cloud Connectors

---

**Last Updated**: October 26, 2025
**Version**: 1.0
**Total Connectors**: 31 (16 Inbound, 15 Outbound, 11 Cron-Enabled)
