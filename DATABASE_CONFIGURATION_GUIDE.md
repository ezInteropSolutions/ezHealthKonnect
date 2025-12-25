# Database Configuration Guide

## Overview
Complete guide for configuring all supported databases in ezHealthKonnect Pipeline Builder.

---

## Quick Reference

| Database | Port | Connection Method | Special Features |
|----------|------|-------------------|------------------|
| MySQL | 3306 | TCP | Multi-statement queries |
| PostgreSQL | 5432 | TCP | Advanced data types, JSON support |
| SQL Server | 1433 | TDS | Windows Authentication support |
| MongoDB | 27017 | TCP | NoSQL, document queries |
| Redis | 6379 | TCP | Key-value operations, caching |
| Oracle | 1521 | TNS | Enterprise features |

---

## MySQL Configuration

### Connection Settings

**Individual Fields:**
```
Database Type: MySQL
Host: mysql.example.com
Port: 3306
Database: healthcare_db
Username: app_user
Password: ********
```

**Connection String Format:**
```
username:password@tcp(host:port)/database
```

**Example:**
```
app_user:mypassword@tcp(mysql.example.com:3306)/healthcare_db
```

### Query Examples

**Simple Query:**
```sql
SELECT * FROM patients WHERE mrn = ?
```

**Multiple Parameters:**
```sql
SELECT * FROM patients
WHERE mrn = ?
  AND date_of_birth = ?
  AND last_name = ?
```

**Query Parameters:**
- Parameter `1`: `PID.3.1` (Patient MRN - ID only)
- Parameter `2`: `PID.7` (Date of Birth)
- Parameter `3`: `PID.5.1` (Last Name)

**Complex Query with JOIN:**
```sql
SELECT
    p.patient_name,
    p.mrn,
    i.insurance_id,
    i.policy_number
FROM patients p
LEFT JOIN insurance i ON p.id = i.patient_id
WHERE p.mrn = ?
```

### MySQL-Specific Features

**1. Multi-Statement Queries:**
```sql
SET @mrn = ?;
SELECT * FROM patients WHERE mrn = @mrn;
SELECT * FROM appointments WHERE patient_mrn = @mrn;
```

**2. JSON Functions:**
```sql
SELECT
    patient_id,
    JSON_EXTRACT(metadata, '$.insurance.provider') as provider
FROM patients
WHERE mrn = ?
```

**3. Date Formatting:**
```sql
SELECT
    patient_name,
    DATE_FORMAT(date_of_birth, '%Y-%m-%d') as dob
FROM patients
WHERE mrn = ?
```

### Common Issues & Solutions

**Issue**: `Error 1251: Client does not support authentication protocol`
- **Solution**: Use `mysql_native_password` authentication:
  ```sql
  ALTER USER 'app_user'@'%' IDENTIFIED WITH mysql_native_password BY 'password';
  ```

**Issue**: `Access denied for user`
- **Solution**: Grant proper permissions:
  ```sql
  GRANT SELECT, INSERT, UPDATE ON healthcare_db.* TO 'app_user'@'%';
  FLUSH PRIVILEGES;
  ```

---

## PostgreSQL Configuration

### Connection Settings

**Individual Fields:**
```
Database Type: PostgreSQL
Host: postgres.example.com
Port: 5432
Database: healthcare_db
Username: app_user
Password: ********
```

**Connection String Format:**
```
host=hostname port=port user=username password=password dbname=database sslmode=disable
```

**Example:**
```
host=postgres.example.com port=5432 user=app_user password=mypassword dbname=healthcare_db sslmode=disable
```

**With SSL:**
```
host=postgres.example.com port=5432 user=app_user password=mypassword dbname=healthcare_db sslmode=require
```

### Query Examples

**Parameterized Query (PostgreSQL uses $1, $2, etc.):**
```sql
SELECT * FROM patients WHERE mrn = $1
```

**Multiple Parameters:**
```sql
SELECT * FROM patients
WHERE mrn = $1
  AND date_of_birth = $2
  AND last_name = $3
```

**Query Parameters:**
- Parameter `1`: `PID.3.1`
- Parameter `2`: `PID.7`
- Parameter `3`: `PID.5.1`

### PostgreSQL-Specific Features

**1. JSON/JSONB Queries:**
```sql
SELECT
    patient_id,
    metadata->>'insurance_provider' as provider,
    metadata->'demographics'->>'age' as age
FROM patients
WHERE mrn = $1
```

**2. Array Operations:**
```sql
SELECT
    patient_name,
    allergies[1] as primary_allergy
FROM patients
WHERE mrn = $1 AND 'penicillin' = ANY(allergies)
```

**3. Advanced Types:**
```sql
SELECT
    patient_name,
    age(date_of_birth) as current_age,
    date_of_birth::date as dob
FROM patients
WHERE mrn = $1
```

**4. Window Functions:**
```sql
SELECT
    visit_id,
    visit_date,
    ROW_NUMBER() OVER (ORDER BY visit_date DESC) as visit_number
FROM visits
WHERE patient_mrn = $1
```

### Common Issues & Solutions

**Issue**: `SSL connection required`
- **Solution**: Enable SSL in connection string or disable with `sslmode=disable`

**Issue**: `FATAL: password authentication failed`
- **Solution**: Check pg_hba.conf authentication method:
  ```
  # pg_hba.conf
  host    healthcare_db    app_user    0.0.0.0/0    md5
  ```

---

## SQL Server Configuration

### Connection Settings

**Individual Fields:**
```
Database Type: SQL Server
Host: sqlserver.example.com
Port: 1433
Database: HealthcareDB
Username: app_user
Password: ********
```

**Connection String Format:**
```
sqlserver://username:password@host:port?database=dbname
```

**Example:**
```
sqlserver://app_user:mypassword@sqlserver.example.com:1433?database=HealthcareDB
```

**With Windows Authentication:**
```
sqlserver://host:port?database=dbname&integrated security=SSPI
```

### Query Examples

**Parameterized Query (SQL Server uses @p1, @p2, etc.):**
```sql
SELECT * FROM patients WHERE mrn = @p1
```

**Multiple Parameters:**
```sql
SELECT * FROM patients
WHERE mrn = @p1
  AND DateOfBirth = @p2
  AND LastName = @p3
```

**Query Parameters:**
- Parameter `1`: `PID.3.1`
- Parameter `2`: `PID.7`
- Parameter `3`: `PID.5.1`

### SQL Server-Specific Features

**1. TOP Clause:**
```sql
SELECT TOP 10 *
FROM patients
WHERE mrn = @p1
ORDER BY admission_date DESC
```

**2. JSON Support (SQL Server 2016+):**
```sql
SELECT
    patient_id,
    JSON_VALUE(metadata, '$.insurance.provider') as provider
FROM patients
WHERE mrn = @p1
FOR JSON PATH
```

**3. Common Table Expressions (CTE):**
```sql
WITH RecentVisits AS (
    SELECT TOP 5 *
    FROM visits
    WHERE patient_mrn = @p1
    ORDER BY visit_date DESC
)
SELECT * FROM RecentVisits
```

---

## MongoDB Configuration

### Connection Settings

**Individual Fields:**
```
Database Type: MongoDB
Host: mongodb.example.com
Port: 27017
Database: healthcare_db
Username: app_user
Password: ********
```

**Connection String Format:**
```
mongodb://username:password@host:port/database?authSource=admin
```

**Example:**
```
mongodb://app_user:mypassword@mongodb.example.com:27017/healthcare_db?authSource=admin
```

**For Replica Set:**
```
mongodb://app_user:mypassword@host1:27017,host2:27017,host3:27017/healthcare_db?replicaSet=rs0
```

### Query Configuration

MongoDB uses **visual query builders** instead of SQL:

#### Filter Builder (Match Documents)
```javascript
{
  "mrn": "{{ PID.3.1 }}",
  "status": "active"
}
```

#### Projection Builder (Select Fields)
```javascript
{
  "patient_name": 1,
  "mrn": 1,
  "date_of_birth": 1,
  "_id": 0
}
```

### MongoDB-Specific Features

**1. Nested Document Queries:**
```javascript
// Filter
{
  "demographics.insurance.provider": "{{ stepOutput.previous_step.provider }}"
}
```

**2. Array Queries:**
```javascript
// Filter - Find patients with specific allergy
{
  "allergies": { "$in": ["penicillin"] }
}
```

**3. Advanced Operators:**
```javascript
// Filter - Date range query
{
  "date_of_birth": {
    "$gte": "1980-01-01",
    "$lte": "2000-12-31"
  },
  "mrn": "{{ PID.3.1 }}"
}
```

### Common Issues & Solutions

**Issue**: `Authentication failed`
- **Solution**: Ensure `authSource=admin` in connection string

**Issue**: `Collection not found`
- **Solution**: Use Collection Schema Loader to browse available collections

---

## Redis Configuration

### Connection Settings

**Individual Fields:**
```
Database Type: Redis
Host: redis.example.com
Port: 6379
Database: 0
Password: ********
```

**Connection String Format:**
```
redis://[:password@]host:port/database
```

**Example:**
```
redis://:mypassword@redis.example.com:6379/0
```

### Query Examples

**Key-Value Lookup:**
```
GET patient:{{ PID.3.1 }}
```

**Hash Field Retrieval:**
```
HGETALL patient:{{ PID.3.1 }}:details
```

**Set Operations:**
```
SMEMBERS patient:{{ PID.3.1 }}:allergies
```

### Redis-Specific Features

**1. Key Patterns:**
```
KEYS patient:*
SCAN 0 MATCH patient:P* COUNT 100
```

**2. Expiration:**
```
SETEX cache:patient:{{ PID.3.1 }} 3600 "cached_value"
```

**3. Data Structures:**
- Strings: `GET`, `SET`
- Hashes: `HGET`, `HGETALL`
- Lists: `LRANGE`, `LPUSH`
- Sets: `SMEMBERS`, `SADD`
- Sorted Sets: `ZRANGE`, `ZADD`

---

## Oracle Configuration

### Connection Settings

**Individual Fields:**
```
Database Type: Oracle
Host: oracle.example.com
Port: 1521
Database: ORCL (SID or Service Name)
Username: app_user
Password: ********
```

**Connection String Format (Easy Connect):**
```
oracle://username:password@host:port/servicename
```

**Example:**
```
oracle://app_user:mypassword@oracle.example.com:1521/ORCL
```

**TNS Names Format:**
```
(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=oracle.example.com)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=ORCL)))
```

### Query Examples

**Parameterized Query (Oracle uses :1, :2, etc.):**
```sql
SELECT * FROM patients WHERE mrn = :1
```

**Multiple Parameters:**
```sql
SELECT * FROM patients
WHERE mrn = :1
  AND date_of_birth = :2
  AND last_name = :3
```

### Oracle-Specific Features

**1. ROWNUM:**
```sql
SELECT * FROM patients
WHERE mrn = :1
  AND ROWNUM <= 10
```

**2. Hierarchical Queries:**
```sql
SELECT patient_name, level
FROM patients
START WITH mrn = :1
CONNECT BY PRIOR referring_physician_id = physician_id
```

**3. Date Functions:**
```sql
SELECT
    patient_name,
    TO_CHAR(date_of_birth, 'YYYY-MM-DD') as dob
FROM patients
WHERE mrn = :1
```

---

## Testing Your Configuration

### Step 1: Test Connection
1. Enter all connection details
2. Click **Test Connection** button
3. Wait for ✅ Success or ❌ Error message

### Step 2: Test Query
1. Enter SQL query with `?` placeholders (MySQL, PostgreSQL)
2. Add query parameters mapping HL7 fields
3. Enter test values
4. Click **Run Query**
5. Verify results show expected columns and data

### Step 3: Configure Result Mapping
1. After successful query test, columns are auto-detected
2. Click **Add to Mapping** for each column
3. Map to output field paths (e.g., `enriched.database.patient_name`)

---

## Performance Best Practices

### 1. Use Indexes
Ensure database columns used in WHERE clauses have indexes:
```sql
CREATE INDEX idx_patients_mrn ON patients(mrn);
```

### 2. Limit Result Sets
```sql
-- MySQL/PostgreSQL
SELECT * FROM patients WHERE mrn = ? LIMIT 100;

-- SQL Server
SELECT TOP 100 * FROM patients WHERE mrn = @p1;

-- Oracle
SELECT * FROM patients WHERE mrn = :1 AND ROWNUM <= 100;
```

### 3. Use Connection Pooling
- Set appropriate `Max Open Connections` (default: 10)
- Set `Max Idle Connections` (default: 5)
- Set `Connection Max Lifetime` (default: 5 minutes)

### 4. Parameterize Queries
Always use parameterized queries (never string concatenation):
```sql
-- ✅ GOOD
SELECT * FROM patients WHERE mrn = ?

-- ❌ BAD (SQL Injection risk)
SELECT * FROM patients WHERE mrn = 'P123456'
```

---

## Security Considerations

### 1. Principle of Least Privilege
Grant only necessary permissions:
```sql
-- MySQL/PostgreSQL
GRANT SELECT ON healthcare_db.patients TO 'app_user';

-- SQL Server
GRANT SELECT ON dbo.patients TO app_user;
```

### 2. Use Service Accounts
- Create dedicated database users for integration
- Never use admin/root accounts
- Rotate credentials regularly

### 3. Enable SSL/TLS
```
PostgreSQL: sslmode=require
MySQL: Add SSL certificates in connection config
SQL Server: Encrypt=true in connection string
```

### 4. Network Security
- Use firewall rules to restrict access
- Whitelist only integration server IP addresses
- Use VPN for external database connections

---

## Troubleshooting

### Connection Timeout
**Symptoms**: "Connection timeout" or "Unable to connect"

**Solutions**:
1. Check firewall rules allow traffic on database port
2. Verify host is reachable: `ping database_host`
3. Test port connectivity: `telnet database_host 3306`
4. Check database server is running
5. Verify credentials are correct

### Query Returns 0 Rows
**Symptoms**: Query executes successfully but returns no data

**Solutions**:
1. Verify test parameter values match actual data
2. Check query syntax (especially parameter placeholders)
3. Test query directly in database client with same parameters
4. Check for composite field issues (e.g., `PID.3` vs `PID.3.1`)

### Performance Issues
**Symptoms**: Query takes >5 seconds to execute

**Solutions**:
1. Add indexes on columns used in WHERE clauses
2. Reduce result set size with LIMIT/TOP
3. Optimize query (avoid SELECT *, use specific columns)
4. Check database server load
5. Review query execution plan

---

## Related Documentation
- [HL7_COMPOSITE_FIELDS_GUIDE.md](HL7_COMPOSITE_FIELDS_GUIDE.md) - Understanding HL7 field structures
- [DATABASE_COLUMN_AUTOCOMPLETE.md](DATABASE_COLUMN_AUTOCOMPLETE.md) - Using column autocomplete
- [RESULT_MAPPING_ENHANCEMENTS.md](RESULT_MAPPING_ENHANCEMENTS.md) - Mapping query results
- [STEP_OUTPUT_CHAINING_GUIDE.md](STEP_OUTPUT_CHAINING_GUIDE.md) - Using database results in subsequent steps
