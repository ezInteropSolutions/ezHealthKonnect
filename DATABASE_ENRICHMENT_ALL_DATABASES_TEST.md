# Database Enrichment - All 6 Databases Testing Guide

## Overview

This guide covers testing **all 6 supported databases** for Database Enrichment:

### SQL Databases (4)
1. ✅ PostgreSQL
2. ✅ MySQL
3. ✅ SQL Server
4. ✅ Oracle

### NoSQL Databases (2)
5. ✅ MongoDB
6. ✅ Redis

---

## Quick Start

### 1. Start All Database Services (3 minutes)
```bash
# Start all 6 databases + app
docker-compose up -d

# Wait for all services to be healthy (may take 2-3 minutes for Oracle)
docker-compose ps
```

### 2. Rebuild App with All Drivers (3 minutes)
```bash
# Rebuild app container to install all database drivers
docker-compose stop app
docker-compose build --no-cache app
docker-compose up -d app
```

### 3. Seed All Databases (30 seconds)
```bash
# Seed all 6 databases with identical test data
docker-compose exec app node /app/scripts/seed_all_databases.js
```

Expected output:
```
✅ PostgreSQL: (Primary database - already seeded)
✅ MongoDB:    success
✅ Redis:      success
✅ MySQL:      success
✅ SQL Server: success
✅ Oracle:     success

✅ 6/6 databases seeded successfully
```

---

## Database Connection Details

### 1. PostgreSQL (Primary Database)
```
Host: postgres
Port: 5432
Database: ezhealthkonnect
User: ezhealth_user
Password: secure_password_change_me
```

**Test Query:**
```sql
SELECT * FROM test_patients WHERE mrn = $1
```

**Query Parameters:**
- Parameter: `1`
- Field Path: `PID.3`
- Test Value: `MRN123456`

---

### 2. MySQL
```
Host: mysql
Port: 3306
Database: ezhealthkonnect
User: ezhealth_user
Password: secure_password_change_me
```

**Test Query:**
```sql
SELECT * FROM test_patients WHERE mrn = ?
```

**Query Parameters:**
- Parameter: `mrn`
- Field Path: `PID.3`
- Test Value: `MRN123456`

---

### 3. SQL Server
```
Host: sqlserver
Port: 1433
Database: master
User: sa
Password: SecurePassword123!
```

**Test Query:**
```sql
SELECT * FROM test_patients WHERE mrn = @p1
```

**Query Parameters:**
- Parameter: `p1`
- Field Path: `PID.3`
- Test Value: `MRN123456`

---

### 4. Oracle
```
Host: oracle
Port: 1521
Database: FREEPDB1
User: ezhealth_user
Password: secure_password_change_me
```

**Test Query:**
```sql
SELECT * FROM test_patients WHERE mrn = :1
```

**Query Parameters:**
- Parameter: `1`
- Field Path: `PID.3`
- Test Value: `MRN123456`

---

### 5. MongoDB
```
Host: mongodb
Port: 27017
Database: ezhealthkonnect
User: ezhealth_user
Password: secure_password_change_me
Collection: patients
```

**Filter:**
```json
{ "mrn": "{PID.3}" }
```

**Test Value:** `MRN123456`

---

### 6. Redis
```
Host: redis
Port: 6379
Password: secure_password_change_me
Command: GET
```

**Key Pattern:**
```
patient:{PID.3}
```

**Test Value:** `MRN123456`

---

## Testing Each Database in UI

### Step-by-Step for Each Database

1. **Open Pipeline Builder** - http://localhost:3000
2. **Create or select an interface**
3. **Add "Database Enrichment" step**
4. **Configure database** (use connection details above)
5. **Scroll to Query Tester**
6. **Enter test parameters**:
   - Field: `PID.3`
   - Value: `MRN123456`
7. **Click "Run Query"**
8. **Verify results** - Should show John Doe's patient data

### Expected Result (All Databases)
```json
{
  "mrn": "MRN123456",
  "firstName": "John" (or first_name depending on DB),
  "lastName": "Doe" (or last_name),
  "dob": "1980-05-15",
  "gender": "M",
  "phone": "555-1234",
  "insurance": "Blue Cross"
}
```

---

## Parameter Placeholder Syntax by Database

| Database | Placeholder Syntax | Example |
|----------|-------------------|---------|
| PostgreSQL | `$1`, `$2`, ... | `WHERE mrn = $1` |
| MySQL | `?` (positional) | `WHERE mrn = ?` |
| SQL Server | `@p1`, `@p2`, ... | `WHERE mrn = @p1` |
| Oracle | `:1`, `:2`, ... | `WHERE mrn = :1` |
| MongoDB | `{fieldPath}` in filter | `{"mrn": "{PID.3}"}` |
| Redis | `{fieldPath}` in key | `patient:{PID.3}` |

---

## Test Data Available

### Patients (All Databases)
- **MRN123456** - John Doe (Male, 1980-05-15)
- **MRN789012** - Jane Smith (Female, 1975-08-22)
- **MRN345678** - Robert Johnson (Male, 1992-03-10)

### Providers (All Databases)
- **NPI-1234567890** - Dr. Sarah Williams (Cardiology)
- **NPI-0987654321** - Dr. Michael Brown (Family Medicine)
- **NPI-1122334455** - Dr. Emily Chen (Pediatrics)

### SQL Table Names
- `test_patients` - Patient demographics
- `test_providers` - Provider credentials

### MongoDB Collections
- `patients` - Patient documents
- `providers` - Provider documents

### Redis Keys
- `patient:{mrn}` - Patient cache (e.g., `patient:MRN123456`)
- `provider:{npi}` - Provider cache (e.g., `provider:NPI-1234567890`)

---

## Troubleshooting

### Database Not Started
```bash
# Check which services are running
docker-compose ps

# Start specific database
docker-compose up -d mysql  # or sqlserver, oracle, etc.

# Check logs
docker-compose logs mysql
```

### Connection Refused
```bash
# Restart database service
docker-compose restart mysql

# Wait for health check to pass
docker-compose ps
```

### No Test Data Found
```bash
# Re-seed all databases
docker-compose exec app node /app/scripts/seed_all_databases.js

# Or seed individual databases
docker-compose exec app node /app/scripts/seed_nosql_test_data.js  # MongoDB + Redis
```

### Driver Not Found
```bash
# Rebuild app container (downloads Go drivers)
docker-compose stop app
docker-compose build --no-cache app
docker-compose up -d app
```

### Oracle Takes Long to Start
Oracle database can take 2-3 minutes to initialize on first start. Check logs:
```bash
docker-compose logs -f oracle
```

Wait for message: `DATABASE IS READY TO USE!`

### SQL Server Authentication Error
SQL Server uses a different admin user (`sa`) and stronger password requirement:
- User: `sa`
- Password: `SecurePassword123!` (must have uppercase, lowercase, number, special char)

---

## Performance Comparison

| Database | Typical Latency | Container Size | Startup Time |
|----------|----------------|----------------|--------------|
| Redis | < 1ms | ~50 MB | 5 seconds |
| MongoDB | 1-10ms | ~700 MB | 10 seconds |
| PostgreSQL | 5-20ms | ~400 MB | 10 seconds |
| MySQL | 5-20ms | ~500 MB | 15 seconds |
| SQL Server | 10-30ms | ~1.5 GB | 30 seconds |
| Oracle | 10-30ms | ~2.5 GB | 60-180 seconds |

---

## Go Database Drivers

### Installed Drivers

```go
// go.mod
require (
    github.com/lib/pq v1.10.9                    // PostgreSQL
    github.com/go-sql-driver/mysql v1.8.1        // MySQL
    github.com/microsoft/go-mssqldb v1.7.2       // SQL Server
    github.com/sijms/go-ora/v2 v2.8.22           // Oracle
    go.mongodb.org/mongo-driver v1.17.1          // MongoDB
    github.com/redis/go-redis/v9 v9.7.0          // Redis
)
```

### Imported in Executor

```go
// database_enrichment_executor.go
import (
    _ "github.com/lib/pq"                // PostgreSQL
    _ "github.com/go-sql-driver/mysql"   // MySQL
    _ "github.com/microsoft/go-mssqldb"  // SQL Server
    _ "github.com/sijms/go-ora/v2"       // Oracle

    "go.mongodb.org/mongo-driver/mongo"
    "github.com/redis/go-redis/v9"
)
```

---

## Docker Compose Services

### Resource Requirements

**Minimum:**
- CPU: 4 cores
- RAM: 8 GB
- Disk: 10 GB free space

**Recommended:**
- CPU: 6+ cores
- RAM: 16 GB
- Disk: 20 GB free space

### Ports Used

| Service | Port | Protocol |
|---------|------|----------|
| App (Node.js) | 3000 | HTTP |
| App (Go API) | 8080 | HTTP |
| PostgreSQL | 5432 | TCP |
| MongoDB | 27017 | TCP |
| Redis | 6379 | TCP |
| MySQL | 3306 | TCP |
| SQL Server | 1433 | TCP |
| Oracle | 1521 | TCP |

---

## Testing Checklist

### Basic Tests
- [ ] PostgreSQL query returns John Doe
- [ ] MySQL query returns John Doe
- [ ] SQL Server query returns John Doe
- [ ] Oracle query returns John Doe
- [ ] MongoDB query returns John Doe
- [ ] Redis query returns John Doe

### Advanced Tests
- [ ] Provider lookup works in each database
- [ ] Multiple query parameters work (SQL databases)
- [ ] Empty results handled gracefully
- [ ] Connection timeout works (invalid host)
- [ ] Authentication failure handled (wrong password)
- [ ] Query syntax errors handled

### Integration Tests
- [ ] Pipeline with multiple database enrichment steps
- [ ] PostgreSQL → enrich from MongoDB
- [ ] MySQL → cache in Redis
- [ ] SQL Server → validate against Oracle reference data

---

## Next Steps After Testing

### If All Tests Pass ✅
1. Document specific use cases for each database
2. Create production connection configurations
3. Set up monitoring for database connections
4. Implement connection pooling (future enhancement)

### If Some Tests Fail ❌
1. Check Docker container status
2. Verify test data was seeded
3. Review connection strings
4. Check database logs
5. Verify Go drivers imported correctly

---

## Clean Up

### Stop All Services
```bash
docker-compose down
```

### Remove All Data (Fresh Start)
```bash
# WARNING: This deletes all database data!
docker-compose down -v

# Then restart
docker-compose up -d
docker-compose exec app node /app/scripts/seed_all_databases.js
```

### Stop Specific Database
```bash
docker-compose stop oracle  # Example
```

---

## Documentation References

- **MongoDB/Redis Guide**: [DATABASE_ENRICHMENT_MONGODB_REDIS.md](DATABASE_ENRICHMENT_MONGODB_REDIS.md)
- **Implementation Summary**: [DATABASE_ENRICHMENT_NOSQL_IMPLEMENTATION.md](DATABASE_ENRICHMENT_NOSQL_IMPLEMENTATION.md)
- **Quick Start**: [QUICK_START_MONGODB_REDIS.md](QUICK_START_MONGODB_REDIS.md)
- **Project Guide**: [CLAUDE.md](CLAUDE.md)

---

**Status**: ✅ Ready for Comprehensive Testing
**Last Updated**: December 2024
**Total Databases**: 6 (4 SQL + 2 NoSQL)
