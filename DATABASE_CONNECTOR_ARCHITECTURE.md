# Database Connector Architecture

## Overview

ezHealthKonnect supports **14 database connectors** spanning traditional RDBMS, NoSQL, and modern cloud data platforms.

## Architecture Pattern: **Generic Base + Specific Implementations**

```
┌─────────────────────────────────────────────────────────────┐
│                  database_base.go                            │
│  (Generic: 80% of functionality)                             │
│  - Config parsing                                            │
│  - Connection pooling                                        │
│  - Polling logic                                             │
│  - After-processing (update_flag, delete, archive)           │
│  - Row-to-message conversion                                 │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
        ┌─────────────────────┬─────────────────────┐
        │                     │                     │
┌───────▼────────┐  ┌────────▼────────┐  ┌────────▼────────┐
│   PostgreSQL   │  │   Snowflake     │  │   Databricks    │
│  (lib/pq)      │  │ (snowflake-go)  │  │  (dbsql-go)     │
│                │  │                 │  │                 │
│ Connection:    │  │ Connection:     │  │ Connection:     │
│ postgres://... │  │ account+token   │  │ https+token     │
│                │  │                 │  │                 │
│ UPSERT:        │  │ MERGE:          │  │ MERGE:          │
│ ON CONFLICT    │  │ INTO + MATCHED  │  │ Delta Lake      │
└────────────────┘  └─────────────────┘  └─────────────────┘
```

## Supported Databases (14 Total)

### Traditional RDBMS (5)
1. **PostgreSQL** - `lib/pq` driver
   - Connection: `postgres://user:pass@host:port/db?sslmode=verify-full`
   - UPSERT: `INSERT ... ON CONFLICT ... DO UPDATE`
   - Features: LISTEN/NOTIFY, COPY for bulk ops

2. **MySQL/MariaDB** - `go-sql-driver/mysql` driver
   - Connection: `user:pass@tcp(host:port)/db?tls=skip-verify`
   - UPSERT: `INSERT ... ON DUPLICATE KEY UPDATE` or `REPLACE INTO`
   - Features: Binary log streaming (CDC)

3. **SQL Server** - `denisenkom/go-mssqldb` driver
   - Connection: `server=host;port=1433;database=db;user id=user;password=pass;encrypt=true`
   - UPSERT: `MERGE` statement
   - Features: Windows Auth, Always Encrypted

4. **Oracle** - `godror/godror` driver
   - Connection: `oracle://user:pass@host:port/service_name`
   - UPSERT: `MERGE` statement
   - Features: Wallet authentication, RAC support

5. **IBM DB2** - `ibmdb/go_ibm_db` driver
   - Connection: `HOSTNAME=host;PORT=50000;DATABASE=db;UID=user;PWD=pass;Security=SSL`
   - UPSERT: `MERGE` statement

### NoSQL Databases (2)
6. **MongoDB** - `mongo-go-driver` driver
   - Connection: `mongodb://user:pass@host:port/db?ssl=true`
   - Operations: Find, Insert, Update, Upsert, Aggregation
   - Features: Change streams (CDC), Atlas support

7. **Cassandra** - `gocql/gocql` driver
   - Connection: Cluster contact points + keyspace
   - Operations: CQL queries, lightweight transactions
   - Features: Multi-datacenter, tunable consistency

### Cloud Data Warehouses (5)
8. **Snowflake** - `snowflakedb/gosnowflake` driver
   - Connection: `account.region.snowflakecomputing.com`
   - Auth: Username/password, key pair, OAuth, SSO
   - Features:
     - Virtual warehouses (auto-suspend/resume)
     - Time travel queries
     - Zero-copy cloning
     - Multi-cluster warehouses
   - Example:
     ```go
     dsn := fmt.Sprintf("%s:%s@%s/%s/%s?warehouse=%s&role=%s",
         user, pass, account, database, schema, warehouse, role)
     ```

9. **Databricks SQL** - `databricks/databricks-sql-go` driver
   - Connection: HTTPS endpoint + HTTP path
   - Auth: Personal access token (PAT), OAuth 2.0
   - Features:
     - Delta Lake ACID transactions
     - Photon engine acceleration
     - Unity Catalog governance
     - Serverless SQL warehouses
   - Example:
     ```go
     dsn := fmt.Sprintf("token:%s@%s:443/sql/1.0/endpoints/%s",
         token, host, httpPath)
     ```

10. **Google BigQuery** - `cloud.google.com/go/bigquery` (REST API)
    - Connection: Project ID + service account JSON
    - Auth: Service account, OAuth, ADC
    - Features:
      - Standard SQL, ML models
      - Streaming inserts
      - Federated queries
      - BI Engine acceleration
    - Example:
      ```go
      client, _ := bigquery.NewClient(ctx, projectID,
          option.WithCredentialsFile("service-account.json"))
      ```

11. **AWS Redshift** - `lib/pq` driver (PostgreSQL-compatible)
    - Connection: `postgres://user:pass@cluster.region.redshift.amazonaws.com:5439/db`
    - Auth: IAM database authentication, temporary credentials
    - Features:
      - Columnar storage (SORTKEY, DISTKEY)
      - Spectrum (query S3 data)
      - Concurrency scaling
      - COPY from S3 for bulk load
    - UPSERT Pattern:
      ```sql
      -- Stage in temp table, then merge
      BEGIN TRANSACTION;
      CREATE TEMP TABLE staging (LIKE target);
      COPY staging FROM 's3://...';
      DELETE FROM target USING staging WHERE target.id = staging.id;
      INSERT INTO target SELECT * FROM staging;
      COMMIT;
      ```

12. **Azure Synapse Analytics** - `denisenkom/go-mssqldb` driver (SQL Server-compatible)
    - Connection: `server=workspace.sql.azuresynapse.net;database=pool;user id=user;password=pass`
    - Auth: SQL auth, Azure AD authentication
    - Features:
      - Dedicated SQL pools (formerly SQL DW)
      - Serverless SQL pools
      - Polybase (query external data)
      - Columnstore indexes
    - UPSERT: `MERGE` statement with distribution key awareness

### Specialized Analytics Databases (2)
13. **ClickHouse** - `clickhouse/clickhouse-go` driver
    - Connection: `clickhouse://host:9000?username=user&password=pass&database=db`
    - Features:
      - Column-oriented OLAP
      - Real-time analytics
      - MergeTree engine family
      - Distributed queries
    - UPSERT: `ALTER TABLE ... UPDATE` or `ReplacingMergeTree`

14. **TimescaleDB** - `lib/pq` driver (PostgreSQL extension)
    - Connection: Same as PostgreSQL
    - Features:
      - Hypertables (automatic partitioning)
      - Continuous aggregates
      - Data retention policies
      - Time-series compression
    - UPSERT: Same as PostgreSQL + time-series optimizations

## Driver Selection Strategy

### Why NOT JDBC/ODBC in Go?

❌ **JDBC (Java Database Connectivity)**
- Java-only, doesn't exist in Go ecosystem
- Would require JNI bridging (complex, slow)

❌ **ODBC (Open Database Connectivity)**
- Requires CGo (C bindings)
- Platform-specific compilation
- Binary portability nightmare
- Breaks Docker "single binary" philosophy

✅ **Native Go Drivers**
- Pure Go code (no CGo, no C dependencies)
- Cross-platform compilation (Linux, macOS, Windows, ARM)
- Single binary deployment
- Better performance (no FFI overhead)

## Connection String Examples

### Traditional Databases
```go
// PostgreSQL
postgres://user:pass@localhost:5432/db?sslmode=require

// MySQL
user:pass@tcp(localhost:3306)/db?parseTime=true&tls=skip-verify

// SQL Server
server=localhost;port=1433;database=db;user id=sa;password=pass;encrypt=true

// Oracle
oracle://user:pass@localhost:1521/ORCL

// MongoDB
mongodb://user:pass@localhost:27017/db?authSource=admin&ssl=true
```

### Cloud Data Warehouses
```go
// Snowflake
user:pass@account.us-east-1.snowflakecomputing.com/db/schema?warehouse=WH_LARGE&role=ANALYST

// Databricks
token:dapi123abc@workspace.cloud.databricks.com:443/sql/1.0/endpoints/abc123def

// BigQuery (uses library, not DSN)
projectID := "my-gcp-project"
credFile := "/path/to/service-account.json"

// Redshift
postgres://user:pass@cluster.us-east-1.redshift.amazonaws.com:5439/analytics

// Synapse
server=workspace.sql.azuresynapse.net;database=pool;user id=admin;password=pass;encrypt=true
```

## Authentication Methods

### Traditional Databases
- **PostgreSQL**: Password, SCRAM-SHA-256, peer, cert
- **MySQL**: Native password, caching_sha2_password, cert
- **SQL Server**: SQL auth, Windows auth, Azure AD
- **Oracle**: Password, wallet, Kerberos

### Cloud Platforms
- **Snowflake**: Password, key pair (RSA), OAuth, SSO (Okta, ADFS)
- **Databricks**: Personal access token (PAT), OAuth 2.0, Azure AD
- **BigQuery**: Service account JSON, OAuth, Application Default Credentials
- **Redshift**: Password, IAM database auth, temporary credentials
- **Synapse**: SQL auth, Azure AD (service principal, managed identity)

## UPSERT Syntax Comparison

```sql
-- PostgreSQL / TimescaleDB / Redshift (with staging)
INSERT INTO table (id, name) VALUES (1, 'John')
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name;

-- MySQL / MariaDB
INSERT INTO table (id, name) VALUES (1, 'John')
ON DUPLICATE KEY UPDATE name = VALUES(name);
-- OR
REPLACE INTO table (id, name) VALUES (1, 'John');

-- SQL Server / Azure Synapse
MERGE INTO table AS target
USING (VALUES (1, 'John')) AS source (id, name)
ON target.id = source.id
WHEN MATCHED THEN UPDATE SET name = source.name
WHEN NOT MATCHED THEN INSERT (id, name) VALUES (source.id, source.name);

-- Oracle
MERGE INTO table target
USING (SELECT 1 AS id, 'John' AS name FROM dual) source
ON (target.id = source.id)
WHEN MATCHED THEN UPDATE SET target.name = source.name
WHEN NOT MATCHED THEN INSERT (id, name) VALUES (source.id, source.name);

-- Snowflake
MERGE INTO table AS target
USING source_table AS source
ON target.id = source.id
WHEN MATCHED THEN UPDATE SET name = source.name
WHEN NOT MATCHED THEN INSERT (id, name) VALUES (source.id, source.name);

-- Databricks (Delta Lake)
MERGE INTO table AS target
USING source_table AS source
ON target.id = source.id
WHEN MATCHED THEN UPDATE SET *
WHEN NOT MATCHED THEN INSERT *;

-- MongoDB (not SQL)
db.collection.updateOne(
  { _id: 1 },
  { $set: { name: "John" } },
  { upsert: true }
);

-- ClickHouse (ReplacingMergeTree auto-deduplicates)
INSERT INTO table (id, name) VALUES (1, 'John');
-- OR
ALTER TABLE table UPDATE name = 'John' WHERE id = 1;

-- Cassandra (upsert is insert)
INSERT INTO table (id, name) VALUES (1, 'John');
```

## Implementation Priority

### Phase 1: Traditional RDBMS (Week 1-2)
1. ✅ PostgreSQL (most common in healthcare)
2. ✅ MySQL (Epic MyChart, Cerner)
3. ✅ SQL Server (Epic Clarity)

### Phase 2: Cloud Data Warehouses (Week 3-4)
4. ✅ Snowflake (analytics, population health)
5. ✅ Databricks (ML/AI, lakehouse)
6. ✅ Redshift (AWS healthcare customers)

### Phase 3: NoSQL + Specialized (Week 5)
7. ✅ MongoDB (document storage)
8. ✅ BigQuery (Google Cloud Healthcare API)
9. ✅ Azure Synapse (Epic on Azure)

### Phase 4: Advanced (Future)
10. ⏳ Oracle (legacy EHRs)
11. ⏳ ClickHouse (real-time analytics)
12. ⏳ TimescaleDB (IoMT, vitals data)
13. ⏳ Cassandra (global scale)
14. ⏳ IBM DB2 (legacy systems)

## Configuration Example (Interface JSON)

```json
{
  "source_connectivity": {
    "type": "database",
    "config": {
      "db_type": "snowflake",
      "account": "mycompany.us-east-1",
      "warehouse": "HEALTHCARE_WH",
      "database": "EPIC_DATA",
      "schema": "PATIENT_RECORDS",
      "username": "hl7_reader",
      "password": "***",
      "auth_type": "password",
      "table_name": "adt_events",
      "incremental_column": "updated_at",
      "incremental_type": "timestamp",
      "polling_interval": 300,
      "max_records": 1000,
      "after_processing": "update_flag",
      "processed_flag_col": "hl7_exported",
      "processed_flag_val": "true"
    }
  }
}
```

## Healthcare-Specific Use Cases

### Epic Integration (SQL Server Clarity)
```
Epic Clarity DB → SQL Server Connector → HL7 Transform → FHIR Endpoint
```

### Population Health Analytics (Snowflake)
```
EHR Databases → Snowflake Connector → Data Warehouse → BI Tools
```

### Real-Time Patient Monitoring (Databricks)
```
IoMT Devices → Kafka → Databricks Connector → ML Models → Alerts
```

### Legacy System Modernization (Oracle)
```
Oracle Clinical DB → Oracle Connector → FHIR Transform → Cloud FHIR Server
```

## Performance Optimization

### Connection Pooling
```go
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(10)
db.SetConnMaxLifetime(time.Hour)
```

### Batch Operations
- PostgreSQL: `COPY` command (100x faster than INSERT)
- MySQL: Multi-row INSERT (50x faster)
- Snowflake: `COPY INTO` from staged files
- Databricks: `COPY INTO` Delta Lake tables
- Redshift: `COPY` from S3

### Incremental Polling
```sql
-- Only fetch new/updated records
SELECT * FROM patients
WHERE updated_at > :last_processed_timestamp
ORDER BY updated_at ASC
LIMIT 1000;
```

## Security Best Practices

1. **Credentials Management**
   - Use environment variables for passwords
   - Support secret managers (AWS Secrets Manager, Azure Key Vault)
   - Key pair authentication for Snowflake (no passwords in config)

2. **Encryption**
   - TLS/SSL for all connections
   - Column-level encryption for PII/PHI
   - Transparent Data Encryption (TDE) for SQL Server/Oracle

3. **Access Control**
   - Read-only database users for inbound connectors
   - Least privilege principle
   - IP whitelisting where possible

4. **Audit Logging**
   - Log all database queries (de-identified)
   - Track data lineage
   - HIPAA compliance reporting
