# Script Enrichment Troubleshooting Guide

## Date: December 26, 2025

## Issue: Slow Pipeline Execution (15 seconds)

### Root Cause

**SQL Server Database Enrichment step taking 8 seconds** due to connection timeout.

### Performance Breakdown

From backend logs:
```
PostgreSQL Database Enrichment:    6.6ms   ✅ Fast
SQL Server Database Enrichment:    8.0s    ❌ SLOW (causing 15s total)
Redis Database Enrichment:         86µs    ✅ Fast
Script Enrichment:                 336µs   ✅ Fast
```

**Total**: ~8 seconds (plus overhead)

---

## Solution

### Remove Unnecessary Database Enrichment Steps

Your pipeline should have **only 3 steps** for the risk calculator:

1. ✅ **Metadata Enrichment** (`risk-weights`)
   - Adds risk weights configuration
   - Target Path: `enriched.metadata.riskWeights`

2. ✅ **Database Enrichment - Redis** (`patient-cache`)
   - Gets patient data from Redis cache
   - Redis Command: `GET`
   - Key Pattern: `patient:{{ PID.3.1 }}`
   - Target Path: `enriched.database.patient`

3. ✅ **Script Enrichment** (`risk-calculator`)
   - Calculates risk score using data from Steps 1 & 2
   - Target Path: `enriched.script.riskScore`

### Steps to Fix

1. **Open Pipeline Builder**
2. **Delete these steps** (if present):
   - PostgreSQL Database Enrichment
   - SQL Server Database Enrichment
   - MySQL Database Enrichment
3. **Keep only**:
   - Metadata Enrichment
   - Redis Database Enrichment
   - Script Enrichment
4. **Save Pipeline**
5. **Test Again** - should complete in < 100ms

---

## How to Troubleshoot Performance Issues

### Backend Logs (Detailed Timing)

Check Docker logs for step-by-step timing:

```bash
docker-compose logs app --tail=100 | grep -E "(Executing step|Completed in)"
```

**Expected Output**:
```
✅ [metadata_enrichment] Completed in 245µs
✅ [database_enrichment_redis] Completed in 86µs
✅ [Script Enrichment] Completed in 336µs
```

**Problem Indicators**:
```
✅ [database_enrichment_sqlserver] Completed in 8.0039041s  ← TOO SLOW!
```

### API Response (Execution Times)

The test pipeline API returns timing for each step:

```json
{
  "execution_results": [
    {
      "step_name": "metadata_enrichment",
      "execution_ms": 0,
      "success": true
    },
    {
      "step_name": "database_enrichment_redis",
      "execution_ms": 0,
      "success": true
    },
    {
      "step_name": "Script Enrichment",
      "execution_ms": 0,
      "success": true
    }
  ],
  "execution_time_ms": 45
}
```

### Common Performance Issues

| Symptom | Cause | Solution |
|---------|-------|----------|
| 8-10s delay | Database connection timeout | Remove unused database steps |
| 5s delay | Script timeout | Check script for infinite loops |
| 1-3s delay | API enrichment timeout | Reduce timeout or check API availability |
| Random 100ms+ | Network latency | Check Redis/DB connectivity |

---

## User-Facing Performance Monitoring

### Current Capabilities

✅ **Backend Logs** - Detailed execution timing per step
✅ **API Response** - `execution_ms` for each step
❌ **Frontend Display** - Timing not shown in UI (enhancement needed)

### Enhancement: Show Timing in Test Results

**Recommended Addition**: Display execution time next to each step in test results modal.

**Example**:
```
Step 1: Metadata Enrichment ✅ (0.2ms)
Step 2: Database Enrichment ✅ (0.08ms)
Step 3: Script Enrichment ✅ (0.3ms)
Total: 45ms
```

This would help users identify slow steps without checking backend logs.

---

## Expected Performance

### Enrichment Steps

| Step Type | Expected Time | Acceptable Max |
|-----------|--------------|----------------|
| Metadata Enrichment | < 1ms | 5ms |
| Database (Redis) | < 5ms | 50ms |
| Database (SQL) | 5-50ms | 500ms |
| API Enrichment | 10-100ms | 3000ms |
| Script Enrichment | < 10ms | 100ms |

### Full Pipeline

- **Optimal**: < 50ms
- **Acceptable**: < 500ms
- **Slow**: > 1 second (investigate)
- **Critical**: > 5 seconds (connection issues)

---

## Debugging Script Errors

### Check Script Compilation

```bash
docker-compose logs app | grep "Script"
```

**Common Errors**:

1. **"Illegal return statement"** - Script incomplete or truncated
   - Solution: Re-paste complete script

2. **"undefined is not a function"** - Helper function not available
   - Solution: Use `getNestedValue()`, `calculateAge()`, `console.log()`

3. **"Cannot read property of undefined"** - Data from previous step missing
   - Solution: Check if enrichment step succeeded and data path is correct

### Enable Script Logging

Use `console.log()` in your script:

```javascript
console.log("=== Risk Calculation Start ===");
console.log("Patient:", patientData.name);
console.log("Risk Score:", riskScore);
```

View logs:
```bash
docker-compose logs -f app | grep "\\[Script\\]"
```

---

## Summary

**Problem**: Pipeline taking 15 seconds due to SQL Server database step timing out (8s)

**Solution**: Remove unnecessary database enrichment steps, keep only:
1. Metadata Enrichment
2. Redis Database Enrichment
3. Script Enrichment

**Expected Result**: Pipeline completes in < 100ms

**Troubleshooting**: Use `docker-compose logs app` to see detailed step timing
