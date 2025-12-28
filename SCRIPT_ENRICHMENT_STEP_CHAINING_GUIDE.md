# Script Enrichment: Step Chaining Guide

## Date: December 26, 2025

## Overview

Script Enrichment is for **calculating/transforming data using custom JavaScript logic**, not for data retrieval.

**Key Principle:** Get all external data through Database/API/Metadata Enrichment steps BEFORE the Script Enrichment step.

---

## ❌ Old Way (Context Variables - DEPRECATED)

```javascript
// Step: Script Enrichment
// Config: { "context": { "vipThreshold": 100000 } }

var accountBalance = getNestedValue(input, "patient.accountBalance");
if (accountBalance > vipThreshold) {  // From context
  return { vipStatus: true };
}
```

**Problems:**
- Context is static/hardcoded in pipeline config
- Can't see where the value comes from
- Hard to debug
- Not traceable in step output

---

## ✅ New Way (Step Chaining)

### Pattern: Enrichment → Script → Result

```
Step 1: Database Enrichment (Get config)
   ↓
Step 2: Database Enrichment (Get patient data)
   ↓
Step 3: Script Enrichment (Calculate using data from Step 1 & 2)
```

---

## Example 1: Calculate VIP Status

### Step 1: Database Enrichment (Alias: "hospital-config")
```
Type: Database Enrichment
Alias: hospital-config
Database: PostgreSQL
Query: SELECT vip_threshold, senior_age FROM hospital_config WHERE hospital_id = 'HOSP_001'
Target Path: enriched.database.hospital_config
```

**Output:**
```json
{
  "enriched": {
    "database": {
      "hospital_config": {
        "vip_threshold": 100000,
        "senior_age": 65
      }
    }
  }
}
```

### Step 2: Database Enrichment (Alias: "patient-data")
```
Type: Database Enrichment
Alias: patient-data
Database: PostgreSQL
Query: SELECT account_balance, loyalty_points FROM patients WHERE patient_id = {{ PID.3.1 }}
Target Path: enriched.database.patient_data
```

**Output:**
```json
{
  "enriched": {
    "database": {
      "hospital_config": { "vip_threshold": 100000, "senior_age": 65 },
      "patient_data": {
        "account_balance": 125000,
        "loyalty_points": 4500
      }
    }
  }
}
```

### Step 3: Script Enrichment (Alias: "vip-calculator")
```javascript
// Get hospital config from Step 1
var hospitalConfig = getNestedValue(input, "enriched.database.hospital_config");
var vipThreshold = hospitalConfig.vip_threshold;
var seniorAge = hospitalConfig.senior_age;

// Get patient data from Step 2
var patientData = getNestedValue(input, "enriched.database.patient_data");
var accountBalance = patientData.account_balance;
var loyaltyPoints = patientData.loyalty_points;

// Get patient age from HL7 message
var dob = getNestedValue(input, "enhancedSegments.PID.fields.7.value");
var age = calculateAge(dob);

// Calculate VIP status
var vipStatus = false;
var vipReason = [];

if (accountBalance >= vipThreshold) {
  vipStatus = true;
  vipReason.push("Account balance exceeds threshold");
}

if (loyaltyPoints >= 5000) {
  vipStatus = true;
  vipReason.push("Loyalty points tier");
}

if (age >= seniorAge) {
  vipStatus = true;
  vipReason.push("Senior citizen status");
}

console.log("VIP Status:", vipStatus, "Reasons:", vipReason);

return {
  vipStatus: vipStatus,
  vipReasons: vipReason,
  accountBalance: accountBalance,
  loyaltyPoints: loyaltyPoints,
  age: age
};
```

**Output:**
```json
{
  "enriched": {
    "database": { ... },
    "script": {
      "vip_calculator": {
        "vipStatus": true,
        "vipReasons": ["Account balance exceeds threshold", "Senior citizen status"],
        "accountBalance": 125000,
        "loyaltyPoints": 4500,
        "age": 68
      }
    }
  }
}
```

---

## Example 2: Calculate Readmission Risk

### Step 1: Database Enrichment (Alias: "admission-history")
```sql
SELECT
  admission_date,
  discharge_date,
  diagnosis_codes,
  length_of_stay
FROM admissions
WHERE patient_id = {{ PID.3.1 }}
ORDER BY admission_date DESC
LIMIT 5
```

### Step 2: API Enrichment (Alias: "lab-results")
```
URL: https://lab-api.com/results?patient={{ PID.3.1 }}
Method: GET
Target Path: enriched.api.lab_results
```

### Step 3: Script Enrichment (Alias: "risk-calculator")
```javascript
// Get admission history from Step 1
var admissions = getNestedValue(input, "enriched.database.admission_history");

// Get lab results from Step 2
var labs = getNestedValue(input, "enriched.api.lab_results");

// Get patient demographics from HL7
var dob = getNestedValue(input, "enhancedSegments.PID.fields.7.value");
var age = calculateAge(dob);

// Calculate risk score
var riskScore = 0;

// Check recent readmissions
if (admissions && admissions.length > 0) {
  var lastAdmission = new Date(admissions[0].admission_date);
  var daysSince = (new Date() - lastAdmission) / (1000 * 60 * 60 * 24);

  if (daysSince < 30) {
    riskScore += 10;  // High risk if readmitted within 30 days
  }

  // Multiple recent admissions
  if (admissions.length >= 3) {
    riskScore += 5;
  }
}

// Age factor
if (age >= 75) {
  riskScore += 3;
} else if (age >= 65) {
  riskScore += 2;
}

// Lab abnormalities
if (labs) {
  if (labs.creatinine > 2.0) {
    riskScore += 5;  // Renal dysfunction
  }
  if (labs.hemoglobin < 10) {
    riskScore += 3;  // Anemia
  }
  if (labs.sodium < 135 || labs.sodium > 145) {
    riskScore += 2;  // Electrolyte imbalance
  }
}

// Determine risk level
var riskLevel = "low";
if (riskScore >= 15) {
  riskLevel = "critical";
} else if (riskScore >= 10) {
  riskLevel = "high";
} else if (riskScore >= 5) {
  riskLevel = "moderate";
}

console.log("Readmission Risk - Score:", riskScore, "Level:", riskLevel);

return {
  readmissionRiskScore: riskScore,
  riskLevel: riskLevel,
  factors: {
    recentReadmission: daysSince < 30,
    multipleAdmissions: admissions.length >= 3,
    age: age,
    renalDysfunction: labs?.creatinine > 2.0,
    anemia: labs?.hemoglobin < 10,
    electrolyteImbalance: labs?.sodium < 135 || labs?.sodium > 145
  },
  daysSinceLastAdmission: Math.floor(daysSince),
  totalPriorAdmissions: admissions.length
};
```

---

## Example 3: Use Metadata for Constants

### Step 1: Metadata Enrichment (Alias: "system-config")
```
Type: Add Metadata
Alias: system-config
Config:
  PI: 3.14159
  EARTH_RADIUS_KM: 6371
  COMPANY_NAME: "ezHealthKonnect"
  VERSION: "1.0.0"
Target Path: enriched.metadata.config
```

### Step 2: Script Enrichment (Alias: "distance-calculator")
```javascript
// Get constants from metadata enrichment
var config = getNestedValue(input, "enriched.metadata.config");
var EARTH_RADIUS_KM = config.EARTH_RADIUS_KM;

// Get GPS coordinates from HL7 message (custom fields)
var patientLat = getNestedValue(input, "enhancedSegments.ZPI.fields.1.value");
var patientLon = getNestedValue(input, "enhancedSegments.ZPI.fields.2.value");
var hospitalLat = 40.7128;  // Hospital location
var hospitalLon = -74.0060;

// Haversine formula to calculate distance
function toRadians(degrees) {
  return degrees * config.PI / 180;
}

var dLat = toRadians(hospitalLat - patientLat);
var dLon = toRadians(hospitalLon - patientLon);

var a = Math.sin(dLat/2) * Math.sin(dLat/2) +
        Math.cos(toRadians(patientLat)) * Math.cos(toRadians(hospitalLat)) *
        Math.sin(dLon/2) * Math.sin(dLon/2);

var c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1-a));
var distance = EARTH_RADIUS_KM * c;

console.log("Distance to hospital:", distance.toFixed(2), "km");

return {
  distanceKm: parseFloat(distance.toFixed(2)),
  distanceMiles: parseFloat((distance * 0.621371).toFixed(2)),
  withinServiceArea: distance <= 50,
  calculatedBy: config.COMPANY_NAME,
  version: config.VERSION
};
```

---

## Benefits of Step Chaining vs Context Variables

| Aspect | Context Variables ❌ | Step Chaining ✅ |
|--------|---------------------|-----------------|
| **Traceability** | Hidden in config | Visible in step output |
| **Debugging** | Can't see values | Can inspect each step |
| **Reusability** | Config duplicated | Same enrichment for multiple scripts |
| **Dynamic Values** | Static/hardcoded | From database/API |
| **Testing** | Hard to test | Each step testable independently |
| **Multi-tenancy** | Same for all | Different per interface |

---

## Common Patterns

### Pattern 1: Config → Data → Calculate
```
Step 1: Get configuration (thresholds, weights, rules)
Step 2: Get patient/message data
Step 3: Calculate using Step 1 config + Step 2 data
```

### Pattern 2: Multiple Sources → Combine
```
Step 1: Database lookup
Step 2: API call
Step 3: Cache lookup
Step 4: Script combines all results
```

### Pattern 3: Constants → Transform
```
Step 1: Metadata (constants, formulas)
Step 2: Script uses constants for calculation
```

---

## Available JavaScript Functions

### Built-in Helpers
- `getNestedValue(obj, path)` - Access nested fields (e.g., `"enriched.database.patient.name"`)
- `calculateAge(hl7Date)` - Calculate age from HL7 date (YYYYMMDD)
- `parseHL7Date(hl7Date)` - Convert HL7 date to ISO format
- `console.log(...)` - Debug logging (appears in backend logs)

### Standard JavaScript
- `Math.*` - Math functions (Math.floor, Math.ceil, Math.round, etc.)
- `Date` - Date manipulation
- `String.*` - String methods (split, trim, includes, etc.)
- `Array.*` - Array methods (map, filter, reduce, etc.)
- `JSON.stringify()` / `JSON.parse()` - JSON manipulation

---

## Summary

**Script Enrichment is for:**
- ✅ Calculations (age, BMI, risk scores)
- ✅ Business logic (VIP status, eligibility)
- ✅ Data transformation (parsing, formatting)
- ✅ Combining multiple enrichment results

**Script Enrichment is NOT for:**
- ❌ Database queries (use Database Enrichment)
- ❌ API calls (use API Enrichment)
- ❌ Adding constants (use Metadata Enrichment)

**Key Pattern:**
```
Enrichment Steps (get data) → Script Enrichment (calculate) → Result
```

All configuration and external data should flow through enrichment steps, not context variables.
