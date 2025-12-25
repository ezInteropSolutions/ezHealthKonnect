# Database Enrichment - Multi-Database Support Plan

## 🎯 Objective
Add support for 6 additional database types beyond the current PostgreSQL, MySQL, SQL Server, and Oracle.

## 📊 Target Databases

### Currently Supported (4)
1. ✅ PostgreSQL
2. ✅ MySQL
3. ✅ SQL Server
4. ✅ Oracle

### New Additions (6)
5. 🔄 **MongoDB** - NoSQL document database
6. 🔄 **DynamoDB** - AWS NoSQL key-value/document store
7. 🔄 **Snowflake** - Cloud data warehouse
8. 🔄 **Databricks** - Analytics platform (Spark SQL)
9. 🔄 **Redis** - In-memory data structure store
10. 🔄 **Cassandra** - Distributed NoSQL database

**Total: 10 database types**

---

## 🏗️ Implementation Architecture

### Phase 1: Go Driver Integration

**MongoDB**
- **Driver**: `go.mongodb.org/mongo-driver/mongo`
- **Connection**: `mongodb://user:pass@host:port/database`
- **Query**: MongoDB query language (JSON-based filter)
- **Use Case**: Patient documents, clinical notes, imaging metadata

**DynamoDB**
- **Driver**: `github.com/aws/aws-sdk-go-v2/service/dynamodb`
- **Connection**: AWS credentials + region + table name
- **Query**: DynamoDB Query/Scan with filter expressions
- **Use Case**: Patient records, real-time vitals, IoT device data

**Snowflake**
- **Driver**: `github.com/snowflakedb/gosnowflake`
- **Connection**: `user:pass@account.snowflakecomputing.com/dbname?warehouse=wh&schema=sch`
- **Query**: Standard SQL
- **Use Case**: Healthcare analytics, claims data warehouse, population health

**Databricks**
- **Driver**: `github.com/databricks/databricks-sql-go`
- **Connection**: Token + HTTP path + catalog/schema
- **Query**: Spark SQL
- **Use Case**: ML model results, large-scale analytics, clinical data lake

**Redis**
- **Driver**: `github.com/redis/go-redis/v9` (already imported!)
- **Connection**: `redis://user:pass@host:port/db`
- **Query**: GET/HGETALL/SMEMBERS commands
- **Use Case**: Patient cache, recent lab results, session data

**Cassandra**
- **Driver**: `github.com/gocql/gocql`
- **Connection**: Contact points + keyspace
- **Query**: CQL (Cassandra Query Language)
- **Use Case**: Time-series data, audit logs, high-volume writes

---

## 📋 Implementation Checklist

### Step 1: Update Go Dependencies
```bash
go get go.mongodb.org/mongo-driver/mongo@latest
go get github.com/aws/aws-sdk-go-v2/service/dynamodb@latest
go get github.com/aws/aws-sdk-go-v2/config@latest
go get github.com/snowflakedb/gosnowflake@latest
go get github.com/databricks/databricks-sql-go@latest
go get github.com/gocql/gocql@latest
# Redis already imported: github.com/redis/go-redis/v9
```

### Step 2: Update Models (enrichment_models.go)
- Add new database type enum values
- Add cloud credentials fields (AWS keys, Snowflake account, Databricks token)
- Add NoSQL query format fields

### Step 3: Update Executor (database_enrichment_executor.go)
- Extend `buildConnectionString()` with 6 new cases
- Add `executeNoSQLQuery()` for MongoDB/DynamoDB/Redis
- Add `executeCQLQuery()` for Cassandra
- Add credential handling for cloud databases

### Step 4: Update UI (PropertiesPanel.js)
- Add 6 new database types to dropdown
- Add conditional fields:
  - **MongoDB**: Collection name
  - **DynamoDB**: AWS region, table name, credentials
  - **Snowflake**: Account, warehouse, schema
  - **Databricks**: HTTP path, catalog, token
  - **Redis**: Database index (0-15)
  - **Cassandra**: Keyspace, consistency level

### Step 5: Update Database Test Controller
- Add support for testing all 6 new database types
- Add query validation for NoSQL formats

---

## 🧪 Testing Strategy

### Option A: Docker Compose Test Suite (Recommended)
**Pros**: Free, local, reproducible, CI/CD ready
**Implementation**: Add services to `docker-compose.yml`

```yaml
# Add to docker-compose.yml
  mongodb-test:
    image: mongo:7
    ports: ["27018:27017"]
    environment:
      MONGO_INITDB_ROOT_USERNAME: testuser
      MONGO_INITDB_ROOT_PASSWORD: testpass
      MONGO_INITDB_DATABASE: testdb

  redis-test:
    image: redis:7-alpine
    ports: ["6380:6379"]
    command: redis-server --requirepass testpass

  cassandra-test:
    image: cassandra:4
    ports: ["9043:9042"]
    environment:
      CASSANDRA_CLUSTER_NAME: testcluster
```

**Test Data Seeding**:
- Create `tests/database-enrichment/seed-data/` with sample data for each DB
- Automated seeding scripts run on container startup

### Option B: Cloud Free Tiers (For Cloud DBs)
**DynamoDB**: AWS Free Tier (25 GB storage, 25 WCU/RCU)
**Snowflake**: 30-day free trial + $400 credits
**Databricks**: Community Edition (free, limited features)

### Option C: Mock/Stub Testing
**Pros**: No external dependencies
**Cons**: Doesn't test actual connectivity
**Use**: Unit tests only, not integration tests

---

## 🎯 Recommended Testing Plan

### Phase 1: Local Docker Databases (Week 1)
**Test**: MongoDB, Redis, Cassandra
**Setup**: Add to docker-compose.yml
**Tests**:
1. Connection test
2. Simple query (SELECT/GET equivalent)
3. Query with filters
4. Result mapping
5. Error handling (connection failure, invalid query)

**Sample Test Data**:
```javascript
// MongoDB - patients collection
{
  "patient_id": "P123456",
  "first_name": "John",
  "last_name": "Doe",
  "mrn": "MRN001",
  "dob": "1980-05-15"
}

// Redis - patient cache
Key: "patient:P123456"
Value: JSON.stringify(patientData)

// Cassandra - vitals table
patient_id | timestamp           | heart_rate | blood_pressure
-----------+---------------------+------------+----------------
P123456    | 2025-12-22 10:00:00 | 72         | 120/80
```

### Phase 2: Cloud Database Trials (Week 2)
**Test**: Snowflake, Databricks, DynamoDB
**Setup**: Create free trial accounts
**Cost**: $0 (using free tiers/trials)

**Test Approach**:
1. Create test tables with healthcare-like data
2. Test from localhost (no Docker needed)
3. Document connection parameters
4. Create automated test suite
5. **Take screenshots/logs for documentation**
6. Delete resources after testing (avoid charges)

### Phase 3: Automated Test Suite (Week 3)
**File**: `tests/database-enrichment-multi-db.js`

**Test Cases** (per database):
- ✅ Connection successful
- ✅ Connection failure handling
- ✅ Simple query returns results
- ✅ Query with parameters
- ✅ Result mapping (DB columns → camelCase)
- ✅ Empty result set handling
- ✅ Query timeout
- ✅ Invalid credentials error
- ✅ Target path storage (`enriched.database`)

**Total Tests**: 9 tests × 10 databases = 90 test cases

---

## 📊 Query Format Differences

### SQL Databases (PostgreSQL, MySQL, SQL Server, Oracle, Snowflake, Databricks)
```sql
SELECT patient_id, first_name, last_name
FROM patients
WHERE mrn = $1
```

### MongoDB
```javascript
{
  "collection": "patients",
  "filter": { "mrn": "$1" },
  "projection": { "patient_id": 1, "first_name": 1, "last_name": 1 }
}
```

### DynamoDB
```javascript
{
  "TableName": "patients",
  "KeyConditionExpression": "patient_id = :pid",
  "ExpressionAttributeValues": { ":pid": "$1" }
}
```

### Redis
```
HGETALL patient:$1
```

### Cassandra (CQL)
```sql
SELECT patient_id, first_name, last_name
FROM patients
WHERE mrn = ?
```

---

## 🔐 Security Considerations

### Credential Storage
- **PostgreSQL, MySQL, SQL Server, Oracle**: Username/password in config
- **MongoDB**: Connection string with auth
- **DynamoDB**: AWS Access Key + Secret Key (encrypted)
- **Snowflake**: Username/password + account name
- **Databricks**: Personal Access Token (PAT)
- **Redis**: Password
- **Cassandra**: Username/password

### Recommendations
1. Add credential encryption for cloud DB credentials
2. Support environment variables for sensitive values
3. Add credential vault integration (future)
4. Mask passwords in logs (already done: `****`)

---

## 📝 UI Changes Needed

### Database Type Dropdown
```javascript
options: [
  { value: 'postgresql', label: 'PostgreSQL' },
  { value: 'mysql', label: 'MySQL' },
  { value: 'sqlserver', label: 'SQL Server' },
  { value: 'oracle', label: 'Oracle' },
  { value: 'mongodb', label: 'MongoDB' },
  { value: 'dynamodb', label: 'AWS DynamoDB' },
  { value: 'snowflake', label: 'Snowflake' },
  { value: 'databricks', label: 'Databricks' },
  { value: 'redis', label: 'Redis' },
  { value: 'cassandra', label: 'Apache Cassandra' }
]
```

### Conditional Fields (Dynamic UI)
Show/hide fields based on selected database type:

**MongoDB**:
- Collection Name (text)
- Database Name (text)

**DynamoDB**:
- AWS Region (dropdown: us-east-1, us-west-2, etc.)
- Table Name (text)
- AWS Access Key ID (text)
- AWS Secret Access Key (password)

**Snowflake**:
- Account (text) - e.g., `abc12345.us-east-1`
- Warehouse (text)
- Schema (text)

**Databricks**:
- HTTP Path (text) - e.g., `/sql/1.0/warehouses/abc123`
- Catalog (text)
- Access Token (password)

**Redis**:
- Database Index (number, 0-15)

**Cassandra**:
- Contact Points (text) - comma-separated IPs
- Keyspace (text)
- Consistency Level (dropdown: ONE, QUORUM, ALL)

---

## 📦 Deliverables

### Code Files
1. `models/enrichment_models.go` - Extended config struct
2. `services/executors/enrichment/database_enrichment_executor.go` - Multi-DB support
3. `controllers/database_test_controller.go` - Test endpoint for all DBs
4. `public/js/pipeline/managers/PropertiesPanel.js` - Conditional UI fields
5. `go.mod` - New dependencies

### Documentation
1. `DATABASE_ENRICHMENT_MULTI_DB_PLAN.md` (this file)
2. `DATABASE_ENRICHMENT_TESTING_GUIDE.md` - How to test each database
3. `DATABASE_ENRICHMENT_EXAMPLES.md` - Query examples for each DB type

### Test Files
1. `docker-compose.test.yml` - Test database services
2. `tests/database-enrichment-multi-db.js` - Automated test suite
3. `tests/database-enrichment/seed-data/` - Sample data scripts

---

## ⏱️ Timeline Estimate

| Phase | Tasks | Duration | Effort |
|-------|-------|----------|--------|
| **Phase 1** | Go driver integration (6 DBs) | 2-3 days | Medium |
| **Phase 2** | Backend executor updates | 1-2 days | Medium |
| **Phase 3** | UI conditional fields | 1-2 days | Low |
| **Phase 4** | Docker test setup (3 DBs) | 1 day | Low |
| **Phase 5** | Cloud DB testing (3 DBs) | 2-3 days | Medium |
| **Phase 6** | Automated test suite | 1-2 days | Medium |
| **Phase 7** | Documentation | 1 day | Low |
| **TOTAL** | | **9-14 days** | |

---

## 🚀 Quick Start Testing

### Test MongoDB (Immediate)
```bash
# Add to docker-compose.yml
docker-compose up -d mongodb-test

# Seed test data
docker-compose exec mongodb-test mongosh -u testuser -p testpass --eval '
  use testdb;
  db.patients.insertMany([
    {patient_id: "P001", first_name: "John", last_name: "Doe", mrn: "MRN001"},
    {patient_id: "P002", first_name: "Jane", last_name: "Smith", mrn: "MRN002"}
  ]);
'

# Test query in UI
Collection: patients
Query: { "mrn": "MRN001" }
```

---

## 🎯 Priority Order

Based on healthcare use cases:

1. **MongoDB** (High) - Clinical documents, imaging metadata
2. **Redis** (High) - Patient cache, real-time data
3. **Snowflake** (Medium) - Analytics, claims data
4. **DynamoDB** (Medium) - Scalable patient records
5. **Cassandra** (Low) - Time-series vitals, audit logs
6. **Databricks** (Low) - ML results, research data

**Recommendation**: Start with MongoDB and Redis (both have Docker images and healthcare relevance).

---

## ✅ Success Criteria

- [ ] All 10 database types work in UI dropdown
- [ ] Connection test succeeds for each database
- [ ] Query execution works for each database
- [ ] Result mapping works (DB format → JSON)
- [ ] Error handling works (connection failures, invalid queries)
- [ ] Documentation complete with examples
- [ ] Automated tests pass for all databases
- [ ] No performance degradation on existing PostgreSQL/MySQL

---

## 📞 Next Steps

**Option 1: Full Implementation**
- Implement all 6 databases
- Complete testing suite
- Timeline: 2-3 weeks

**Option 2: Phased Rollout**
- Week 1: MongoDB + Redis (Docker-based)
- Week 2: Snowflake + DynamoDB (Cloud trials)
- Week 3: Cassandra + Databricks (if needed)

**Option 3: Proof of Concept**
- Implement MongoDB only (1 NoSQL example)
- If successful, expand to others
- Timeline: 3-5 days

**Which approach do you prefer?**
