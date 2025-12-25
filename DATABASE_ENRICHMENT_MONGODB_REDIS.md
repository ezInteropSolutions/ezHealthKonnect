# Database Enrichment - MongoDB & Redis Support

## Overview

Database Enrichment now supports **6 database types** including NoSQL databases:

### SQL Databases
1. ✅ PostgreSQL
2. ✅ MySQL
3. ✅ SQL Server
4. ✅ Oracle

### NoSQL Databases
5. ✅ **MongoDB** (Document database - NEWLY ADDED)
6. ✅ **Redis** (Key-value cache - NEWLY ADDED)

---

## MongoDB Integration

### Use Cases
- **Patient Master Index (EMPI)**: Query patient demographics from MongoDB patient collection
- **Provider Directory**: Look up provider credentials stored in MongoDB
- **Clinical Data**: Retrieve FHIR resources stored in MongoDB
- **Flexible Querying**: Use MongoDB's powerful query language with field paths from HL7 messages

### Configuration

#### Connection Fields
```
Host: mongodb (or localhost)
Port: 27017
Database: ezhealthkonnect
User: ezhealth_user
Password: secure_password_change_me
```

#### MongoDB-Specific Fields
- **Collection**: MongoDB collection name (e.g., `patients`, `providers`)
- **Filter**: MongoDB query filter as JSON object
- **Projection** (optional): Fields to include/exclude in results

### Filter Syntax

MongoDB filters support **field placeholders** using `{fieldPath}` syntax:

```json
{
  "mrn": "{PID.3}"
}
```

This will replace `{PID.3}` with the actual value from the HL7 message's PID.3 field.

#### Example Filters

**Patient lookup by MRN:**
```json
{
  "mrn": "{PID.3}"
}
```

**Provider lookup by NPI:**
```json
{
  "npi": "{PV1.7}"
}
```

**Patient by SSN:**
```json
{
  "ssn": "{PID.19}"
}
```

**Complex query (multiple conditions):**
```json
{
  "lastName": "{PID.5.1}",
  "dateOfBirth": "{PID.7}"
}
```

### Projection Example

To only return specific fields:
```json
{
  "firstName": 1,
  "lastName": 1,
  "dateOfBirth": 1,
  "insurance": 1,
  "_id": 0
}
```

### MongoDB Query Result

**Single document** - Returns as object:
```json
{
  "mrn": "MRN123456",
  "firstName": "John",
  "lastName": "Doe",
  "dateOfBirth": "1980-05-15",
  "insurance": {
    "provider": "Blue Cross",
    "memberId": "BC-12345678"
  }
}
```

**Multiple documents** - Returns as array:
```json
[
  { "mrn": "MRN123456", "firstName": "John", ... },
  { "mrn": "MRN789012", "firstName": "Jane", ... }
]
```

---

## Redis Integration

### Use Cases
- **High-Performance Caching**: Sub-millisecond patient/provider lookups
- **Session Data**: Retrieve cached authentication tokens or session data
- **Real-Time Reference Data**: Lab reference ranges, drug formularies
- **Temporary Data Storage**: Recent lookups, frequently accessed data

### Configuration

#### Connection Fields
```
Host: redis (or localhost)
Port: 6379
Password: secure_password_change_me
Database: 0 (Redis database number, default 0)
```

#### Redis-Specific Fields
- **Redis Key**: Key pattern with field placeholders
- **Redis Command**: GET, HGETALL, SMEMBERS, LRANGE

### Redis Key Patterns

Redis keys support **field placeholders** using `{fieldPath}` syntax:

```
patient:{PID.3}
```

This will replace `{PID.3}` with the actual MRN value.

#### Example Key Patterns

**Patient data:**
```
patient:{PID.3}
```
Becomes: `patient:MRN123456`

**Provider data:**
```
provider:{PV1.7}
```
Becomes: `provider:NPI-1234567890`

**Lab reference range:**
```
lab:ref-range:{OBX.3}
```
Becomes: `lab:ref-range:CBC`

**Nested field example:**
```
session:{PID.3}:{PV1.19}
```
Becomes: `session:MRN123456:VN-98765`

### Redis Commands

#### GET (Default)
Returns single value. If value is JSON, automatically parses it.

**Example:**
```
Command: GET
Key: patient:{PID.3}
```

**Result (JSON stored as string):**
```json
{
  "mrn": "MRN123456",
  "firstName": "John",
  "lastName": "Doe",
  "insurance": { ... }
}
```

#### HGETALL
Returns all fields and values from a Redis hash.

**Example:**
```
Command: HGETALL
Key: patient:{PID.3}:hash
```

**Result:**
```json
{
  "mrn": "MRN123456",
  "firstName": "John",
  "lastName": "Doe",
  "dob": "1980-05-15",
  "gender": "M"
}
```

#### SMEMBERS
Returns all members of a set.

**Example:**
```
Command: SMEMBERS
Key: patient:{PID.3}:allergies
```

**Result:**
```json
["Penicillin", "Peanuts"]
```

#### LRANGE
Returns all elements from a list (uses LRANGE 0 -1 internally).

**Example:**
```
Command: LRANGE
Key: patient:{PID.3}:medications
```

**Result:**
```json
["Lisinopril 10mg", "Metformin 500mg"]
```

---

## Testing with Docker

### 1. Start Services

```bash
# Start MongoDB and Redis
docker-compose up -d mongodb redis

# Verify services are running
docker-compose ps
```

### 2. Seed Test Data

```bash
# From host machine (Node.js must be installed)
node scripts/seed_nosql_test_data.js

# OR from inside app container
docker-compose exec app node /app/scripts/seed_nosql_test_data.js
```

This creates:
- **3 patients** in MongoDB `patients` collection
- **3 providers** in MongoDB `providers` collection
- **Patient cache** in Redis (keys: `patient:MRN123456`, etc.)
- **Provider cache** in Redis (keys: `provider:NPI-1234567890`, etc.)
- **Lab reference ranges** in Redis (keys: `lab:ref-range:CBC`, etc.)

### 3. Test in UI

#### MongoDB Test
1. Open Pipeline Builder
2. Add "Database Enrichment" step
3. Configure:
   - **Database Type**: MongoDB
   - **Host**: mongodb
   - **Port**: 27017
   - **Database**: ezhealthkonnect
   - **User**: ezhealth_user
   - **Password**: secure_password_change_me
   - **Collection**: patients
   - **Filter**: `{ "mrn": "{PID.3}" }`
4. Scroll to Query Tester
5. **Test MRN**: MRN123456
6. Click **Run Query**
7. ✅ Should return patient demographics

#### Redis Test
1. Add "Database Enrichment" step
2. Configure:
   - **Database Type**: Redis
   - **Host**: redis
   - **Port**: 6379
   - **Password**: secure_password_change_me
   - **Redis Command**: GET
   - **Redis Key**: patient:{PID.3}
3. Scroll to Query Tester
4. **Test MRN**: MRN123456
5. Click **Run Query**
6. ✅ Should return cached patient data

### 4. Manual Verification (Optional)

**MongoDB:**
```bash
# Connect to MongoDB
docker-compose exec mongodb mongosh -u ezhealth_user -p secure_password_change_me ezhealthkonnect

# Query patients
db.patients.find({ mrn: "MRN123456" }).pretty()

# Exit
exit
```

**Redis:**
```bash
# Connect to Redis
docker-compose exec redis redis-cli -a secure_password_change_me

# Get patient data
GET patient:MRN123456

# Get all patient keys
KEYS patient:*

# Exit
exit
```

---

## Performance Comparison

| Database Type | Typical Latency | Best For |
|--------------|-----------------|----------|
| **Redis** | < 1ms | High-frequency lookups, caching |
| **MongoDB** | 1-10ms | Flexible queries, complex documents |
| **PostgreSQL** | 5-20ms | Relational data, complex joins |
| **MySQL** | 5-20ms | Relational data, ACID transactions |

---

## Error Handling

### MongoDB Errors

**Connection failed:**
```
Error: failed to connect to MongoDB: connection refused
```
→ Check MongoDB is running: `docker-compose ps mongodb`

**Authentication failed:**
```
Error: failed to ping MongoDB: auth error
```
→ Verify username/password match docker-compose.yml

**Collection not found:**
```
MongoDB query returned 0 documents
```
→ Run seed script: `node scripts/seed_nosql_test_data.js`

### Redis Errors

**Key not found:**
```
Error: key not found: patient:MRN999999
```
→ Check Redis has data: `docker-compose exec redis redis-cli -a secure_password_change_me KEYS patient:*`

**Authentication failed:**
```
Error: failed to ping Redis: NOAUTH Authentication required
```
→ Verify password in configuration

**Unsupported command:**
```
Error: unsupported Redis command: ZADD
```
→ Only GET, HGETALL, SMEMBERS, LRANGE are supported (add more if needed)

---

## Architecture

### Code Structure

```
models/enrichment_models.go
  ├─ DatabaseEnrichmentConfigV2
  │   ├─ Collection (MongoDB)
  │   ├─ Filter (MongoDB)
  │   ├─ Projection (MongoDB)
  │   ├─ RedisKey (Redis)
  │   └─ RedisCommand (Redis)

services/executors/enrichment/database_enrichment_executor.go
  ├─ Execute() - Routes to database-specific method
  ├─ executeSQLQuery() - SQL databases (existing)
  ├─ executeMongoQuery() - MongoDB (NEW)
  │   ├─ buildMongoFilter() - Replace {field} placeholders
  │   └─ Returns single doc or array
  └─ executeRedisQuery() - Redis (NEW)
      ├─ buildRedisKey() - Replace {field} placeholders
      └─ Supports GET, HGETALL, SMEMBERS, LRANGE
```

### Dependencies

**go.mod:**
```go
require (
    go.mongodb.org/mongo-driver v1.17.1  // MongoDB driver
    github.com/redis/go-redis/v9 v9.7.0  // Redis client
)
```

**Docker:**
```yaml
services:
  mongodb:
    image: mongo:7
    ports: ["27017:27017"]

  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]
    command: redis-server --requirepass secure_password_change_me
```

---

## Future Enhancements

### Phase 2: Additional NoSQL Databases (Planned)
- ✅ MongoDB
- ✅ Redis
- ⏳ DynamoDB (AWS)
- ⏳ Snowflake (Cloud data warehouse)
- ⏳ Databricks (Lakehouse)
- ⏳ Cassandra (Wide-column store)

### Phase 3: Advanced Features (Planned)
- Connection pooling for MongoDB/Redis
- MongoDB aggregation pipeline support
- Redis Lua script execution
- TTL management for cached data
- Automatic retry with exponential backoff
- Circuit breaker pattern for fault tolerance

---

## Test Data Reference

### MongoDB Patients
```
MRN123456 - John Doe (Male, DOB: 1980-05-15)
MRN789012 - Jane Smith (Female, DOB: 1975-08-22)
MRN345678 - Robert Johnson (Male, DOB: 1992-03-10)
```

### MongoDB Providers
```
NPI-1234567890 - Dr. Sarah Williams (Internal Medicine, Cardiology)
NPI-0987654321 - Dr. Michael Brown (Family Medicine)
NPI-1122334455 - Dr. Emily Chen (Pediatrics, Neonatology)
```

### Redis Keys
```
patient:MRN123456 - John Doe patient data (JSON)
patient:MRN123456:hash - John Doe patient data (Hash)
provider:NPI-1234567890 - Dr. Williams provider data
lab:ref-range:CBC - Complete Blood Count reference ranges
lab:ref-range:BMP - Basic Metabolic Panel reference ranges
lab:ref-range:HBA1C - Hemoglobin A1C reference ranges
```

---

## Support

For issues or questions:
1. Check Docker logs: `docker-compose logs mongodb redis`
2. Verify test data: `node scripts/seed_nosql_test_data.js`
3. Review CLAUDE.md for architecture details
4. Check DATABASE_ENRICHMENT_IMPROVEMENTS.md for SQL database setup

---

**Status**: ✅ Production Ready (MongoDB & Redis)
**Version**: 1.0.0
**Last Updated**: December 2024
