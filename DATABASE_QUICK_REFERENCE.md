# Database Enrichment - Quick Reference Card

## 🎯 Supported Databases

| # | Database | Port | Test MRN |
|---|----------|------|----------|
| 1 | PostgreSQL | 5432 | MRN123456 |
| 2 | MySQL | 3306 | MRN123456 |
| 3 | SQL Server | 1433 | MRN123456 |
| 4 | Oracle | 1521 | MRN123456 |
| 5 | MongoDB | 27017 | MRN123456 |
| 6 | Redis | 6379 | MRN123456 |

---

## ⚡ Quick Commands

### Start Everything
```bash
docker-compose up -d
docker-compose exec app node /app/scripts/seed_all_databases.js
```

### Rebuild App (Required After Code Changes)
```bash
docker-compose stop app
docker-compose build --no-cache app
docker-compose up -d app
```

### Seed Test Data
```bash
# All databases
docker-compose exec app node /app/scripts/seed_all_databases.js

# MongoDB + Redis only
docker-compose exec app node /app/scripts/seed_nosql_test_data.js
```

### Check Status
```bash
docker-compose ps
docker-compose logs app | tail -50
```

---

## 📊 Connection Quick Copy

### PostgreSQL
```
Host: postgres | Port: 5432 | DB: ezhealthkonnect
User: ezhealth_user | Pass: secure_password_change_me
Query: SELECT * FROM test_patients WHERE mrn = $1
```

### MySQL
```
Host: mysql | Port: 3306 | DB: ezhealthkonnect
User: ezhealth_user | Pass: secure_password_change_me
Query: SELECT * FROM test_patients WHERE mrn = ?
```

### SQL Server
```
Host: sqlserver | Port: 1433 | DB: master
User: sa | Pass: SecurePassword123!
Query: SELECT * FROM test_patients WHERE mrn = @p1
```

### Oracle
```
Host: oracle | Port: 1521 | DB: FREEPDB1
User: ezhealth_user | Pass: secure_password_change_me
Query: SELECT * FROM test_patients WHERE mrn = :1
```

### MongoDB
```
Host: mongodb | Port: 27017 | DB: ezhealthkonnect
User: ezhealth_user | Pass: secure_password_change_me
Collection: patients | Filter: { "mrn": "{PID.3}" }
```

### Redis
```
Host: redis | Port: 6379 | Pass: secure_password_change_me
Command: GET | Key: patient:{PID.3}
```

---

## 🧪 Test Data

**Patients:** MRN123456, MRN789012, MRN345678
**Providers:** NPI-1234567890, NPI-0987654321, NPI-1122334455

---

## ❌ Troubleshooting

**Build fails:**
```bash
docker-compose build --no-cache app
```

**Connection refused:**
```bash
docker-compose restart <database>
```

**No test data:**
```bash
docker-compose exec app node /app/scripts/seed_all_databases.js
```

**Oracle slow:**
Wait 2-3 minutes on first start

---

## 📚 Documentation

- [Quick Start](QUICK_START_MONGODB_REDIS.md)
- [All Databases Testing](DATABASE_ENRICHMENT_ALL_DATABASES_TEST.md)
- [Complete Guide](DATABASE_ENRICHMENT_COMPLETE_IMPLEMENTATION.md)
