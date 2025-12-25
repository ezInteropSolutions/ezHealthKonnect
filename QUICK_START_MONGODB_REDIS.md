# Quick Start: MongoDB & Redis Database Enrichment

## 🚀 1-Minute Setup

### Step 1: Rebuild Docker (3 minutes)
```bash
# Rebuild app container with MongoDB and Redis drivers
docker-compose stop app
docker-compose build --no-cache app
docker-compose up -d
```

### Step 2: Seed Test Data (10 seconds)
```bash
# Load sample patients, providers, and lab data
docker-compose exec app node /app/scripts/seed_nosql_test_data.js
```

### Step 3: Test in UI (2 minutes)
1. Open http://localhost:3000
2. Go to Pipeline Builder
3. Add "Database Enrichment" step
4. Select **MongoDB** or **Redis**
5. Click **Run Query** in Query Tester

---

## 📋 MongoDB Quick Config

### Connection
```
Database Type: MongoDB
Host: mongodb
Port: 27017
Database: ezhealthkonnect
User: ezhealth_user
Password: secure_password_change_me
```

### Patient Lookup
```
Collection: patients
Filter: { "mrn": "{PID.3}" }
```

**Test with**: `MRN123456`

### Provider Lookup
```
Collection: providers
Filter: { "npi": "{PV1.7}" }
```

**Test with**: `NPI-1234567890`

---

## 🔴 Redis Quick Config

### Connection
```
Database Type: Redis
Host: redis
Port: 6379
Password: secure_password_change_me
```

### Patient Cache Lookup
```
Redis Command: GET
Redis Key: patient:{PID.3}
```

**Test with**: `MRN123456`

### Provider Cache Lookup
```
Redis Command: GET
Redis Key: provider:{PV1.7}
```

**Test with**: `NPI-1234567890`

### Lab Reference Range
```
Redis Command: GET
Redis Key: lab:ref-range:{OBX.3}
```

**Test with**: `CBC`

---

## 🧪 Test Data Reference

### Patients (MongoDB & Redis)
- **MRN123456** - John Doe (Male, 1980-05-15)
- **MRN789012** - Jane Smith (Female, 1975-08-22)
- **MRN345678** - Robert Johnson (Male, 1992-03-10)

### Providers (MongoDB & Redis)
- **NPI-1234567890** - Dr. Sarah Williams (Cardiology)
- **NPI-0987654321** - Dr. Michael Brown (Family Medicine)
- **NPI-1122334455** - Dr. Emily Chen (Pediatrics)

### Lab Ref Ranges (Redis only)
- **CBC** - Complete Blood Count
- **BMP** - Basic Metabolic Panel
- **HBA1C** - Hemoglobin A1C

---

## ❓ Common Issues

### "Connection refused"
```bash
# Restart services
docker-compose restart mongodb redis
```

### "Auth failed"
- Password is: `secure_password_change_me`

### "No data found"
```bash
# Re-seed test data
docker-compose exec app node /app/scripts/seed_nosql_test_data.js
```

### "Build errors"
```bash
# Force rebuild
docker-compose build --no-cache app
```

---

## 📚 Full Documentation
- **User Guide**: [DATABASE_ENRICHMENT_MONGODB_REDIS.md](DATABASE_ENRICHMENT_MONGODB_REDIS.md)
- **Implementation**: [DATABASE_ENRICHMENT_NOSQL_IMPLEMENTATION.md](DATABASE_ENRICHMENT_NOSQL_IMPLEMENTATION.md)
- **Multi-DB Plan**: [DATABASE_ENRICHMENT_MULTI_DB_PLAN.md](DATABASE_ENRICHMENT_MULTI_DB_PLAN.md)

---

**Ready?** Run the rebuild command and start testing! 🎉
