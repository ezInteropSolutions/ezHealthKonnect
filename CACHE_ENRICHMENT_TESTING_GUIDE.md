# Cache Enrichment Testing Guide

## Date: December 25, 2025

## ⚠️ IMPORTANT: Use Database Enrichment with Redis

**Cache Enrichment step has been removed from the toolbox** because the backend executor (`cache_enrichment_executor.go`) is not fully implemented yet and returns placeholder responses only.

**✅ Use "Database Enrichment" step with Redis instead** - This provides full cache lookup functionality with Redis commands (GET, HGETALL, HGET, SMEMBERS, etc.).

This guide shows you how to use **Database Enrichment with Redis** for cache lookups.

## Overview
Complete guide for testing cache lookups using Redis via the Database Enrichment step in the Pipeline Builder.

---

## ✅ Prerequisites

### 1. Verify Redis is Running
```bash
docker-compose ps redis
```
Expected output: Status = "Up" and healthy

### 2. Verify Test Data is Seeded
```bash
docker-compose exec redis redis-cli -a secure_password_change_me KEYS patient:*
```
Expected output:
```
patient:P123456
patient:P789012
```

### 3. Test Redis Connection
```bash
docker-compose exec redis redis-cli -a secure_password_change_me GET patient:P123456
```
Expected output: JSON string with patient data

---

## 🧪 Test Scenario 1: Simple Patient Cache Lookup (GET Command)

### Purpose
Test retrieving cached patient data using Redis GET command.

### Step-by-Step Instructions

1. **Open Pipeline Builder**
   - Navigate to: http://localhost:3000/pipeline-builder.html

2. **Create New Pipeline**
   - Click "New Pipeline" button
   - Name: "Test Cache Enrichment - GET"
   - Description: "Test patient cache lookup with GET command"

3. **Add Cache Enrichment Step**
   - **Option A: Use Database Enrichment with Redis** (Current Implementation)
     - Drag "Database Enrichment" from Pre-Processing section
     - Configure as follows:

   **Database Configuration:**
   ```
   Database Type: Redis
   Host: redis
   Port: 6379
   Database: 0
   Password: secure_password_change_me
   ```

   **Redis Query Builder:**
   ```
   Command: GET - Retrieve a value (like getting patient JSON data)

   Main Category: patient
   Separator: :
   Sub-Category: (leave empty)
   Field with unique ID: PID.3.1

   Generated Command: GET patient:{{ PID.3.1 }}
   ```

   **Advanced Settings:**
   ```
   Target Path: enriched.cache.patient
   Timeout (ms): 1000
   Fail on Error: No (unchecked)
   ```

4. **Save the Pipeline**
   - Click "Save Pipeline" button
   - Verify success message

5. **Test with Sample HL7 Message**

   **Test Message 1** (Cache Hit - Data exists):
   ```
   MSH|^~\&|EPIC|LAB|HL7LISTENER|HOSPITAL|20241225100000||ADT^A01|MSG001|P|2.5
   PID|||P123456^^^MRN||Doe^John||19800115|M|||123 Main St^^Boston^MA^02101
   ```

   **Expected Result:**
   - ✅ Step executes successfully
   - Response time: < 10ms
   - Enriched data stored at: `enriched.cache.patient`
   - Data returned:
     ```json
     {
       "name": "John Doe",
       "dob": "1980-01-15",
       "email": "john.doe@email.com",
       "insuranceId": "INS123456",
       "status": "active"
     }
     ```

   **Test Message 2** (Cache Miss - Data doesn't exist):
   ```
   MSH|^~\&|EPIC|LAB|HL7LISTENER|HOSPITAL|20241225100000||ADT^A01|MSG002|P|2.5
   PID|||P999999^^^MRN||Unknown^Patient||19900101|M|||456 Oak Ave^^Boston^MA^02101
   ```

   **Expected Result:**
   - ⚠️ Step fails gracefully (because Fail on Error = No)
   - Error message: "key not found: patient:P999999"
   - Pipeline continues (no crash)
   - Default value returned (if configured)

---

## 🧪 Test Scenario 2: Provider Hash Lookup (HGETALL Command)

### Purpose
Test retrieving provider data using Redis HGETALL command (returns all fields from a hash).

### Configuration

**Redis Query Builder:**
```
Command: HGETALL - Get all fields from a record

Main Category: provider
Separator: :
Sub-Category: npi
Field with unique ID: PV1.7.1

Generated Command: HGETALL provider:npi:{{ PV1.7.1 }}
```

**Test Message:**
```
MSH|^~\&|EPIC|LAB|HL7LISTENER|HOSPITAL|20241225100000||ADT^A01|MSG003|P|2.5
PID|||P123456^^^MRN||Doe^John||19800115|M|||123 Main St^^Boston^MA^02101
PV1||I|ICU^101^A|||1234567890^Johnson^Sarah^MD||||||||||
```

**Expected Result:**
```json
{
  "name": "Dr. Sarah Johnson",
  "specialty": "Cardiology",
  "phone": "555-0100",
  "hospital": "General Hospital"
}
```

---

## 🧪 Test Scenario 3: Cache-Aside Pattern (Cache → Database Fallback)

### Purpose
Implement cache-aside pattern where you check cache first, then fall back to database on miss.

### Pipeline Configuration

**Step 1: Check Cache (Sequence 10)**
```
Step Type: Database Enrichment (Redis)
Step Name: Check Patient Cache
Step Alias: patient_cache

Database Type: Redis
Redis Command: GET patient:{{ PID.3.1 }}
Target Path: enriched.cache
Fail on Error: No ✅ IMPORTANT
```

**Step 2: Database Lookup on Cache Miss (Sequence 20)**
```
Step Type: Database Enrichment (PostgreSQL)
Step Name: Fetch Patient from Database
Step Alias: patient_db

Query: SELECT * FROM patients WHERE mrn = $1
Parameters: {"1": "PID.3.1"}
Target Path: enriched.database
Execute Only If: Cache step returned null
```

### Test Cases

**Test 1: Cache Hit (Patient P123456)**
- Expected Flow: Step 1 → Returns data → Skip Step 2
- Performance: ~5ms total

**Test 2: Cache Miss (Patient P999999)**
- Expected Flow: Step 1 → Returns null → Execute Step 2 → Query database
- Performance: ~300ms total (database query time)

---

## 🧪 Test Scenario 4: Performance Comparison

### Purpose
Compare cache vs database performance.

### Test Setup

Create two identical pipelines:
1. **Pipeline A:** Direct database enrichment
2. **Pipeline B:** Cache enrichment

### Performance Test

Send 100 messages with same patient ID (P123456):

**Pipeline A (Database Only):**
```
Avg Response Time: 150ms per message
Total Time: 15,000ms (15 seconds)
Database Queries: 100
```

**Pipeline B (Cache with 99% Hit Rate):**
```
Avg Response Time: 5ms per message (cache hit) + 150ms (1 miss)
Total Time: 650ms (0.65 seconds)
Database Queries: 1
Speedup: 23x faster
```

---

## 🧪 Test Scenario 5: TTL (Time To Live) Testing

### Purpose
Verify cache expiration works correctly.

### Test Steps

1. **Set Cache Data with Short TTL**
   ```bash
   docker-compose exec redis redis-cli -a secure_password_change_me SET patient:P999999 '{"name":"Test Patient"}' EX 10
   ```

2. **Test Immediately**
   - Send message with MRN: P999999
   - Expected: Data returned from cache

3. **Wait 15 Seconds**
   - Send same message again
   - Expected: Cache miss (key expired)

4. **Verify Expiration**
   ```bash
   docker-compose exec redis redis-cli -a secure_password_change_me GET patient:P999999
   ```
   - Expected: `(nil)`

---

## 🔧 Troubleshooting

### Issue 1: "Connection refused" Error

**Symptoms:**
```
Error: Failed to connect to Redis: connection refused
```

**Solutions:**
1. Verify Redis is running:
   ```bash
   docker-compose ps redis
   ```

2. Check Redis port is exposed:
   ```bash
   docker-compose port redis 6379
   ```

3. Test connection:
   ```bash
   docker-compose exec redis redis-cli -a secure_password_change_me PING
   ```
   Expected: `PONG`

---

### Issue 2: "NOAUTH Authentication required"

**Symptoms:**
```
Error: NOAUTH Authentication required
```

**Solution:**
Verify password in configuration matches docker-compose.yml:
- Password: `secure_password_change_me`

---

### Issue 3: "Key not found" Error

**Symptoms:**
```
Error: key not found: patient:P123456
```

**Solutions:**
1. Verify key exists:
   ```bash
   docker-compose exec redis redis-cli -a secure_password_change_me GET patient:P123456
   ```

2. Check key pattern matches:
   - Expected: `patient:P123456`
   - Check for typos, extra spaces, wrong separator

3. Verify field path resolves correctly:
   - Field: `PID.3.1` should extract `P123456` from HL7 message
   - Check composite field vs simple field

---

### Issue 4: Data Not Returned (Empty Response)

**Symptoms:**
- Step executes successfully
- No error
- But `enriched.cache` is empty

**Solutions:**
1. Check Target Path configuration
2. Verify Redis command returned data (check logs)
3. Inspect step output in execution context

---

## 📊 Expected Performance Metrics

| Operation | Response Time | Notes |
|-----------|---------------|-------|
| Cache Hit (GET) | 2-5ms | In-memory lookup |
| Cache Hit (HGETALL) | 3-8ms | Hash with 4-10 fields |
| Cache Miss | ~300ms | Falls back to database |
| Database Direct | 100-300ms | Without cache |

---

## 🎯 Success Criteria

- [x] Redis container is healthy
- [x] Test data seeded successfully
- [x] GET command retrieves JSON data
- [x] HGETALL command retrieves hash data
- [x] Cache miss handled gracefully
- [x] TTL expiration works
- [x] Performance improvement measurable
- [x] Step output accessible in subsequent steps

---

## 📝 Next Steps

After successful testing:

1. **Implement Cache Warmup**
   - Pre-populate cache with frequently accessed data
   - Scheduled job to refresh cache periodically

2. **Monitor Cache Hit Rate**
   - Track cache hits vs misses
   - Optimize TTL based on hit rate
   - Target: 80%+ hit rate

3. **Implement Cache Invalidation**
   - Clear cache when source data changes
   - Use Redis pub/sub for notifications

4. **Add Cache Metrics**
   - Response time tracking
   - Hit/miss ratio
   - Cache size monitoring

---

## 📚 Related Documentation

- [DATABASE_CONFIGURATION_GUIDE.md](DATABASE_CONFIGURATION_GUIDE.md) - Redis connection setup
- [STEP_OUTPUT_CHAINING_GUIDE.md](STEP_OUTPUT_CHAINING_GUIDE.md) - Using cache data in subsequent steps
- [DATABASE_QUICK_REFERENCE.md](DATABASE_QUICK_REFERENCE.md) - Redis commands reference

---

## ✅ Quick Test Commands

```bash
# Check Redis is running
docker-compose ps redis

# Verify test data
docker-compose exec redis redis-cli -a secure_password_change_me KEYS patient:*

# Get patient data
docker-compose exec redis redis-cli -a secure_password_change_me GET patient:P123456

# Get provider data (hash)
docker-compose exec redis redis-cli -a secure_password_change_me HGETALL provider:npi:1234567890

# Check TTL
docker-compose exec redis redis-cli -a secure_password_change_me TTL patient:P123456

# Clear test cache
docker-compose exec redis redis-cli -a secure_password_change_me FLUSHDB
```

---

**Happy Testing! 🚀**
