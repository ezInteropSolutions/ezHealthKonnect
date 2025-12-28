# Script Enrichment Testing Guide

## Date: December 26, 2025

## Overview
Step-by-step guide to test Script Enrichment with step chaining pattern (no context variables).

---

## Test Scenario: Calculate Patient Risk Score

We'll create a 4-step pipeline that:
1. **Metadata Enrichment** - Add risk weights configuration
2. **Database Enrichment** - Get patient data from Redis cache
3. **Script Enrichment** - Calculate risk score using data from Steps 1 & 2
4. **HL7→FHIR Mapping** - Transform to FHIR (optional)

---

## Prerequisites

### 1. Verify Redis is Running
```bash
docker-compose ps redis
# Should show: Up (healthy)
```

### 2. Seed Test Data in Redis
```bash
# Add patient data to Redis
docker-compose exec redis redis-cli -a secure_password_change_me SET "patient:P123456" '{"name":"John Doe","dob":"19800115","chronicConditions":3,"lastAdmission":"2025-01-10","smokingStatus":"current"}'

docker-compose exec redis redis-cli -a secure_password_change_me SET "patient:P789012" '{"name":"Jane Smith","dob":"19750520","chronicConditions":1,"lastAdmission":"2024-11-15","smokingStatus":"never"}'

# Verify data
docker-compose exec redis redis-cli -a secure_password_change_me GET "patient:P123456"
```

### 3. Sample HL7 Message
```
MSH|^~\&|EPIC|HOSPITAL|FHIR|EHK|20250126103000||ADT^A01|MSG123456|P|2.5
EVN|A01|20250126103000
PID|1||P123456^^^MRN||Doe^John||19800115|M|||123 Main St^^Springfield^IL^62701||555-1234|||M|NON|12345678
PV1|1|I|ICU^101^01||||1234^Smith^John^^^Dr|||SUR||||ADM|A0
```

---

## Step-by-Step Testing

### Step 1: Create Interface

1. **Go to Interface Management**
   - Navigate to: http://localhost:3000/interface-management.html

2. **Create New Interface**
   - Name: `Script Enrichment Test`
   - Type: `TCP/MLLP`
   - Port: `6661`
   - Click **Save**

3. **Note the Interface ID** (e.g., `interface_abc123`)

---

### Step 2: Create Pipeline

1. **Go to Pipeline Builder**
   - Navigate to: http://localhost:3000/pipeline-builder.html

2. **Create New Pipeline**
   - Name: `Patient Risk Score Calculator`
   - Interface: `Script Enrichment Test`

---

### Step 3: Add Step 1 - Metadata Enrichment

1. **Drag "Add Metadata" step** from toolbox to canvas

2. **Configure Properties:**
   - **Step Alias:** `risk-weights`
   - **Metadata (JSON):**
   ```json
   {
     "weights": {
       "ageOver65": 2,
       "ageOver75": 3,
       "chronicConditions": 2,
       "recentAdmission": 10,
       "smokingCurrent": 3,
       "smokingFormer": 1
     },
     "thresholds": {
       "highRisk": 10,
       "moderateRisk": 5,
       "lowRisk": 0
     }
   }
   ```
   - **Target Path:** `enriched.metadata.riskWeights`

3. **Save Step**

---

### Step 4: Add Step 2 - Database Enrichment (Redis)

1. **Drag "Database Enrichment" step** from toolbox to canvas

2. **Configure Properties:**
   - **Step Alias:** `patient-cache`
   - **Database Type:** `Redis`
   - **Host:** `redis`
   - **Port:** `6379`
   - **Database:** `0`
   - **Password:** `secure_password_change_me`

3. **Configure Redis Query:**
   - **Redis Command:** `GET`
   - **Key Pattern:** `patient:{{ PID.3.1 }}`
   - **Target Path:** `enriched.database.patient`

4. **Advanced Settings:**
   - **Timeout (ms):** `1000`
   - **Fail on Error:** `No` (unchecked)

5. **Save Step**

---

### Step 5: Add Step 3 - Script Enrichment

1. **Drag "Script Enrichment" step** from toolbox to canvas

2. **Configure Properties:**
   - **Step Alias:** `risk-calculator`

3. **JavaScript Code:**
```javascript
// Get risk weights from Step 1 (Metadata Enrichment)
var riskConfig = getNestedValue(input, "enriched.metadata.riskWeights");
var weights = riskConfig.weights;
var thresholds = riskConfig.thresholds;

// Get patient data from Step 2 (Database Enrichment - Redis)
var patientData = getNestedValue(input, "enriched.database.patient");

// Get patient DOB from original HL7 message
var dob = getNestedValue(input, "enhancedSegments.PID.fields.7.value");
var age = calculateAge(dob);

console.log("=== Risk Calculation Start ===");
console.log("Patient:", patientData.name);
console.log("Age:", age);
console.log("Chronic Conditions:", patientData.chronicConditions);

// Initialize risk score
var riskScore = 0;
var riskFactors = [];

// Age factor (using weights from metadata)
if (age >= 75) {
  riskScore += weights.ageOver75;
  riskFactors.push("Age 75+");
  console.log("+ Age 75+:", weights.ageOver75, "points");
} else if (age >= 65) {
  riskScore += weights.ageOver65;
  riskFactors.push("Age 65+");
  console.log("+ Age 65+:", weights.ageOver65, "points");
}

// Chronic conditions (using config from metadata)
var conditionPoints = patientData.chronicConditions * weights.chronicConditions;
riskScore += conditionPoints;
if (patientData.chronicConditions > 0) {
  riskFactors.push(patientData.chronicConditions + " chronic conditions");
  console.log("+ Chronic conditions:", conditionPoints, "points");
}

// Recent admission check (using data from Redis)
if (patientData.lastAdmission) {
  var lastAdmit = new Date(patientData.lastAdmission);
  var daysSince = (new Date() - lastAdmit) / (1000 * 60 * 60 * 24);

  if (daysSince < 30) {
    riskScore += weights.recentAdmission;
    riskFactors.push("Readmission within 30 days");
    console.log("+ Recent admission:", weights.recentAdmission, "points");
  }
}

// Smoking status (using data from Redis)
if (patientData.smokingStatus === "current") {
  riskScore += weights.smokingCurrent;
  riskFactors.push("Current smoker");
  console.log("+ Current smoker:", weights.smokingCurrent, "points");
} else if (patientData.smokingStatus === "former") {
  riskScore += weights.smokingFormer;
  riskFactors.push("Former smoker");
  console.log("+ Former smoker:", weights.smokingFormer, "points");
}

// Determine risk level (using thresholds from metadata)
var riskLevel = "low";
if (riskScore >= thresholds.highRisk) {
  riskLevel = "high";
} else if (riskScore >= thresholds.moderateRisk) {
  riskLevel = "moderate";
}

console.log("=== Final Risk Score:", riskScore, "Level:", riskLevel, "===");

// Return enrichment data
return {
  riskScore: riskScore,
  riskLevel: riskLevel,
  riskFactors: riskFactors,
  patientAge: age,
  chronicConditions: patientData.chronicConditions,
  daysSinceLastAdmission: Math.floor(daysSince),
  smokingStatus: patientData.smokingStatus,
  calculatedAt: new Date().toISOString()
};
```

4. **Advanced Settings:**
   - **Target Path:** `enriched.script.riskScore`
   - **Timeout (ms):** `5000`
   - **Fail on Error:** `No` (unchecked)

5. **Save Step**

---

### Step 6: Save Pipeline

1. Click **"Save Pipeline"** button (top right)
2. Pipeline should show 3 connected steps:
   ```
   [Add Metadata] → [Database Enrichment] → [Script Enrichment]
   ```

---

## Testing the Pipeline

### Method 1: Send HL7 Message via TCP/MLLP

```bash
# Create test message file
cat > test_message.hl7 << 'EOF'
MSH|^~\&|EPIC|HOSPITAL|FHIR|EHK|20250126103000||ADT^A01|MSG123456|P|2.5
EVN|A01|20250126103000
PID|1||P123456^^^MRN||Doe^John||19800115|M|||123 Main St^^Springfield^IL^62701||555-1234|||M|NON|12345678
PV1|1|I|ICU^101^01||||1234^Smith^John^^^Dr|||SUR||||ADM|A0
EOF

# Send message using netcat (add MLLP framing)
(echo -ne '\x0b'; cat test_message.hl7; echo -ne '\x1c\x0d') | nc localhost 6661
```

### Method 2: Use Message Interface Page

1. Navigate to: http://localhost:3000/messages.html?interfaceId={your-interface-id}
2. Click **"Send Test Message"**
3. Paste the HL7 message
4. Click **"Send"**

---

## Verify Results

### 1. Check Backend Logs
```bash
docker-compose logs -f app | grep -A 20 "Risk Calculation"
```

**Expected Output:**
```
=== Risk Calculation Start ===
Patient: John Doe
Age: 45
Chronic Conditions: 3
+ Chronic conditions: 6 points
+ Recent admission: 10 points
+ Current smoker: 3 points
=== Final Risk Score: 19 Level: high ===
✅ [Script Enrichment] Result stored at: enriched.script.riskScore
```

### 2. Check Message Output

**Query message from database:**
```bash
# Get the interface table name
docker-compose exec postgres psql -U postgres -d ezhealthkonnect -c "SELECT table_name FROM interface_table_metadata ORDER BY created_at DESC LIMIT 1;"

# Query the message
docker-compose exec postgres psql -U postgres -d ezhealthkonnect -c "SELECT message_id, status, created_at FROM messages_intf_{id} ORDER BY created_at DESC LIMIT 1;"
```

**Check MongoDB for full output:**
```bash
# Connect to MongoDB
docker-compose exec mongodb mongosh ezhealthkonnect

# Find the message
db.transformed_messages.findOne(
  { message_id: /MSG123456/ },
  {
    'enriched.metadata': 1,
    'enriched.database': 1,
    'enriched.script': 1
  }
)
```

**Expected Structure:**
```json
{
  "enriched": {
    "metadata": {
      "riskWeights": {
        "weights": { "ageOver65": 2, "ageOver75": 3, ... },
        "thresholds": { "highRisk": 10, ... }
      }
    },
    "database": {
      "patient": {
        "name": "John Doe",
        "dob": "19800115",
        "chronicConditions": 3,
        "lastAdmission": "2025-01-10",
        "smokingStatus": "current"
      }
    },
    "script": {
      "riskScore": {
        "riskScore": 19,
        "riskLevel": "high",
        "riskFactors": [
          "3 chronic conditions",
          "Readmission within 30 days",
          "Current smoker"
        ],
        "patientAge": 45,
        "chronicConditions": 3,
        "daysSinceLastAdmission": 16,
        "smokingStatus": "current",
        "calculatedAt": "2025-01-26T10:30:00.000Z"
      }
    }
  }
}
```

---

## Test Different Scenarios

### Scenario 1: Low Risk Patient (Jane Smith)

**HL7 Message:**
```
MSH|^~\&|EPIC|HOSPITAL|FHIR|EHK|20250126103000||ADT^A01|MSG789012|P|2.5
EVN|A01|20250126103000
PID|1||P789012^^^MRN||Smith^Jane||19750520|F|||456 Oak Ave^^Chicago^IL^60601||555-5678|||F|NON|87654321
PV1|1|O|CLINIC^201^01||||5678^Jones^Mary^^^Dr|||MED||||ADM|A0
```

**Expected Risk Score:** ~2 (age 50 + 1 chronic condition)
**Expected Risk Level:** low

---

### Scenario 2: Moderate Risk Patient (modify weights)

**Change Metadata in Step 1:**
```json
{
  "weights": {
    "ageOver65": 3,
    "ageOver75": 5,
    "chronicConditions": 1,
    "recentAdmission": 7,
    "smokingCurrent": 2,
    "smokingFormer": 1
  },
  "thresholds": {
    "highRisk": 15,
    "moderateRisk": 7,
    "lowRisk": 0
  }
}
```

**Re-test John Doe:** Should now be moderate risk

---

## Troubleshooting

### Issue 1: "enriched.database.patient is null"

**Cause:** Redis didn't return data

**Check:**
```bash
# Verify Redis has the data
docker-compose exec redis redis-cli -a secure_password_change_me GET "patient:P123456"

# Check Database Enrichment output in logs
docker-compose logs app | grep "Database Enrichment.*Redis"
```

**Fix:** Re-seed Redis with correct patient ID

---

### Issue 2: "Script execution timeout"

**Cause:** Script is taking too long (> 5000ms)

**Check:**
```bash
docker-compose logs app | grep "Script execution timeout"
```

**Fix:** Increase timeout or simplify script

---

### Issue 3: "Context variables deprecated warning"

**Cause:** Script config still has `context` field

**Check:**
```bash
docker-compose logs app | grep "DEPRECATED.*Context"
```

**Fix:** This is just a warning - context still works but should be migrated to step chaining

---

### Issue 4: Script errors

**Check console.log output:**
```bash
docker-compose logs -f app | grep "\[Script\]"
```

**Common errors:**
- `getNestedValue` returns null → Check field path
- `calculateAge` fails → Check date format (YYYYMMDD)
- Undefined variable → Check enrichment step completed successfully

---

## Performance Verification

### Expected Timings:

| Step | Expected Time |
|------|--------------|
| Metadata Enrichment | < 1ms |
| Database Enrichment (Redis) | 2-5ms |
| Script Enrichment | 10-50ms |
| **Total Pipeline** | **< 100ms** |

**Check actual timings:**
```bash
docker-compose logs app | grep "execution time"
```

---

## Key Learnings

### ✅ What Works Well

1. **Step Chaining Pattern**
   - Clear data flow: Metadata → Database → Script
   - Each step output is visible and debuggable
   - Configuration is dynamic (from database)

2. **JavaScript Helpers**
   - `getNestedValue()` makes data access simple
   - `calculateAge()` handles HL7 dates automatically
   - `console.log()` helps with debugging

3. **Flexibility**
   - Can change risk weights without modifying script
   - Can add more enrichment steps (API calls, more DB queries)
   - Can use same config for multiple scripts

### ❌ Common Mistakes

1. **Wrong Field Paths**
   - ✅ `enriched.metadata.riskWeights.weights.ageOver65`
   - ❌ `enriched.riskWeights.weights.ageOver65` (missing metadata)

2. **Null Checking**
   - Always check if enrichment returned data before using it
   - Use `if (patientData)` or `patientData?.field` syntax

3. **Context Variables**
   - ❌ Don't use context - deprecated
   - ✅ Use Metadata/Database enrichment instead

---

## Summary

**What We Tested:**
- ✅ Metadata Enrichment (risk weights configuration)
- ✅ Database Enrichment (Redis cache lookup)
- ✅ Script Enrichment (business logic calculation)
- ✅ Step chaining pattern (no context variables)
- ✅ JavaScript helpers (`getNestedValue`, `calculateAge`, `console.log`)

**Key Takeaway:** Script Enrichment is for **calculations and business logic**, not data retrieval. Always get configuration and external data through enrichment steps first.
