# MongoDB Advanced Mode Guide

## 🎯 Overview

MongoDB database enrichment now supports **TWO modes**:

1. **👁️ User-Friendly Mode** (Visual Builder) - No coding required
2. **⚡ Advanced Mode** (Aggregation Pipeline) - Full MongoDB power

---

## 🔀 Choosing the Right Mode

### Use User-Friendly Mode When:
- ✅ You want to find documents by simple conditions (mrn = "12345")
- ✅ You want to select specific fields to return
- ✅ You're new to MongoDB
- ✅ You don't need field transformations

### Use Advanced Mode When:
- ⚡ You need to **concatenate fields** (combine firstName + lastName)
- ⚡ You need to **rename fields** (alias mrn to patientId)
- ⚡ You need to **calculate values** (age from date of birth)
- ⚡ You need **complex filters** (OR logic, array operations)
- ⚡ You need to **join multiple collections** ($lookup)
- ⚡ You need to **group and aggregate** data

---

## 🚀 How to Use Advanced Mode

### Step 1: Select Advanced Mode

In the Database Enrichment configuration:

```
Database Type: MongoDB
Query Mode: ⚡ Advanced (Aggregation Pipeline - For MongoDB experts)
```

### Step 2: Write Aggregation Pipeline JSON

The **Aggregation Pipeline** textarea appears with a helpful placeholder example.

---

## 📚 Aggregation Pipeline Basics

### What is an Aggregation Pipeline?

A **pipeline** is an array of **stages** that process documents sequentially.

**Think of it like a factory assembly line**:
1. Stage 1: Filter raw materials ($match)
2. Stage 2: Transform them ($project)
3. Stage 3: Group them ($group)
4. Stage 4: Sort them ($sort)

### Pipeline Structure

```json
[
  { "stage1": { ...config... } },
  { "stage2": { ...config... } },
  { "stage3": { ...config... } }
]
```

---

## 🔧 Common Pipeline Stages

### 1. `$match` - Filter Documents (Like WHERE clause)

**Find patient by MRN**:
```json
[
  {
    "$match": { "mrn": "{PID.3}" }
  }
]
```

**Multiple conditions (AND logic)**:
```json
[
  {
    "$match": {
      "mrn": "{PID.3}",
      "status": "active"
    }
  }
]
```

**OR logic**:
```json
[
  {
    "$match": {
      "$or": [
        { "status": "active" },
        { "status": "pending" }
      ]
    }
  }
]
```

---

### 2. `$project` - Select and Transform Fields

**Select specific fields**:
```json
[
  { "$match": { "mrn": "{PID.3}" } },
  {
    "$project": {
      "mrn": 1,
      "firstName": 1,
      "lastName": 1
    }
  }
]
```

**Concatenate fields** (combine firstName + lastName):
```json
[
  { "$match": { "mrn": "{PID.3}" } },
  {
    "$project": {
      "mrn": 1,
      "fullName": { "$concat": ["$firstName", " ", "$lastName"] },
      "displayName": { "$concat": ["$lastName", ", ", "$firstName"] }
    }
  }
]
```

**Result**:
```json
{
  "mrn": "MRN123456",
  "fullName": "John Doe",
  "displayName": "Doe, John"
}
```

**Rename fields** (alias):
```json
[
  { "$match": { "mrn": "{PID.3}" } },
  {
    "$project": {
      "patientId": "$mrn",        // Rename mrn → patientId
      "patientName": "$firstName", // Rename firstName → patientName
      "phone": 1                   // Keep phone as-is
    }
  }
]
```

**Calculate age from date of birth**:
```json
[
  { "$match": { "mrn": "{PID.3}" } },
  {
    "$project": {
      "mrn": 1,
      "firstName": 1,
      "age": {
        "$subtract": [
          { "$year": "$$NOW" },
          { "$year": { "$dateFromString": { "dateString": "$dateOfBirth" } } }
        ]
      }
    }
  }
]
```

---

### 3. `$addFields` - Add New Fields (Without Removing Others)

**Add fullName without removing other fields**:
```json
[
  { "$match": { "mrn": "{PID.3}" } },
  {
    "$addFields": {
      "fullName": { "$concat": ["$firstName", " ", "$lastName"] }
    }
  }
]
```

**Result**: Returns ALL original fields PLUS the new `fullName` field.

---

### 4. `$group` - Aggregate Data

**Count patients by insurance provider**:
```json
[
  {
    "$group": {
      "_id": "$insurance.provider",
      "count": { "$sum": 1 }
    }
  }
]
```

**Average age by gender**:
```json
[
  {
    "$group": {
      "_id": "$gender",
      "avgAge": { "$avg": "$age" },
      "count": { "$sum": 1 }
    }
  }
]
```

---

### 5. `$lookup` - Join Collections

**Get patient and their provider details**:
```json
[
  { "$match": { "mrn": "{PID.3}" } },
  {
    "$lookup": {
      "from": "providers",
      "localField": "primaryProvider",
      "foreignField": "npi",
      "as": "providerDetails"
    }
  }
]
```

---

### 6. `$sort` - Sort Results

**Sort patients by lastName**:
```json
[
  { "$match": { "status": "active" } },
  { "$sort": { "lastName": 1 } }  // 1 = ascending, -1 = descending
]
```

---

### 7. `$limit` - Limit Number of Results

**Get first 10 patients**:
```json
[
  { "$match": { "status": "active" } },
  { "$sort": { "lastName": 1 } },
  { "$limit": 10 }
]
```

---

## 💡 Real-World Examples

### Example 1: Patient Lookup with Full Name

**Goal**: Find patient by MRN and return full name (firstName + lastName)

```json
[
  {
    "$match": { "mrn": "{PID.3}" }
  },
  {
    "$project": {
      "mrn": 1,
      "fullName": { "$concat": ["$firstName", " ", "$lastName"] },
      "dateOfBirth": 1,
      "phone": 1,
      "insurance": 1
    }
  }
]
```

**Result**:
```json
{
  "mrn": "MRN123456",
  "fullName": "John Doe",
  "dateOfBirth": "1980-05-15",
  "phone": "555-1234",
  "insurance": { "provider": "Blue Cross" }
}
```

---

### Example 2: Rename Fields for Downstream System

**Goal**: Transform MongoDB field names to match HL7 FHIR naming conventions

```json
[
  {
    "$match": { "mrn": "{PID.3}" }
  },
  {
    "$project": {
      "id": "$mrn",
      "name": { "$concat": ["$firstName", " ", "$lastName"] },
      "birthDate": "$dateOfBirth",
      "telecom": "$phone",
      "managingOrganization": "$insurance.provider"
    }
  }
]
```

**Result** (FHIR-like structure):
```json
{
  "id": "MRN123456",
  "name": "John Doe",
  "birthDate": "1980-05-15",
  "telecom": "555-1234",
  "managingOrganization": "Blue Cross"
}
```

---

### Example 3: Filter by Array Element

**Goal**: Find patients allergic to Penicillin

```json
[
  {
    "$match": {
      "mrn": "{PID.3}",
      "allergies": { "$in": ["Penicillin"] }
    }
  },
  {
    "$project": {
      "mrn": 1,
      "fullName": { "$concat": ["$firstName", " ", "$lastName"] },
      "allergies": 1
    }
  }
]
```

---

### Example 4: Conditional Field Transformation

**Goal**: Format phone number based on presence

```json
[
  {
    "$match": { "mrn": "{PID.3}" }
  },
  {
    "$project": {
      "mrn": 1,
      "fullName": { "$concat": ["$firstName", " ", "$lastName"] },
      "formattedPhone": {
        "$cond": {
          "if": { "$ne": ["$phone", null] },
          "then": { "$concat": ["(", { "$substr": ["$phone", 0, 3] }, ") ", { "$substr": ["$phone", 3, 3] }, "-", { "$substr": ["$phone", 6, 4] }] },
          "else": "N/A"
        }
      }
    }
  }
]
```

**Result**:
```json
{
  "mrn": "MRN123456",
  "fullName": "John Doe",
  "formattedPhone": "(555) 123-4567"
}
```

---

### Example 5: Join Patient with Provider

**Goal**: Get patient data AND their primary provider's details

```json
[
  {
    "$match": { "mrn": "{PID.3}" }
  },
  {
    "$lookup": {
      "from": "providers",
      "localField": "primaryProvider",
      "foreignField": "npi",
      "as": "providerInfo"
    }
  },
  {
    "$project": {
      "mrn": 1,
      "fullName": { "$concat": ["$firstName", " ", "$lastName"] },
      "providerName": { "$arrayElemAt": ["$providerInfo.name", 0] },
      "providerSpecialty": { "$arrayElemAt": ["$providerInfo.specialty", 0] }
    }
  }
]
```

---

## 🎓 Aggregation Operators Reference

### String Operators
- `$concat` - Combine strings: `{ "$concat": ["$firstName", " ", "$lastName"] }`
- `$substr` - Extract substring: `{ "$substr": ["$phone", 0, 3] }`
- `$toLower` - Convert to lowercase: `{ "$toLower": "$email" }`
- `$toUpper` - Convert to uppercase: `{ "$toUpper": "$state" }`
- `$trim` - Remove whitespace: `{ "$trim": { "input": "$name" } }`

### Math Operators
- `$add` - Addition: `{ "$add": ["$price", "$tax"] }`
- `$subtract` - Subtraction: `{ "$subtract": [2025, "$birthYear"] }`
- `$multiply` - Multiplication: `{ "$multiply": ["$hours", "$rate"] }`
- `$divide` - Division: `{ "$divide": ["$total", "$count"] }`

### Date Operators
- `$year` - Extract year: `{ "$year": "$dateOfBirth" }`
- `$month` - Extract month: `{ "$month": "$dateOfBirth" }`
- `$dayOfMonth` - Extract day: `{ "$dayOfMonth": "$dateOfBirth" }`
- `$dateFromString` - Parse date: `{ "$dateFromString": { "dateString": "$dob" } }`

### Conditional Operators
- `$cond` - If-then-else: `{ "$cond": { "if": condition, "then": value1, "else": value2 } }`
- `$ifNull` - Default if null: `{ "$ifNull": ["$phone", "N/A"] }`
- `$switch` - Multi-way branch (like switch/case)

### Array Operators
- `$size` - Array length: `{ "$size": "$allergies" }`
- `$arrayElemAt` - Get element: `{ "$arrayElemAt": ["$allergies", 0] }`
- `$in` - Check if value in array: `{ "$in": ["Penicillin", "$allergies"] }`
- `$filter` - Filter array elements

---

## 🔗 Using HL7 Field Placeholders

**You can use `{PID.3}` syntax in aggregation pipelines!**

### Example: Dynamic Patient Lookup

```json
[
  {
    "$match": {
      "mrn": "{PID.3}",
      "facility": "{MSH.4}"
    }
  },
  {
    "$project": {
      "mrn": 1,
      "fullName": { "$concat": ["$firstName", " ", "$lastName"] }
    }
  }
]
```

**When HL7 message arrives**:
- `{PID.3}` → replaced with actual MRN from HL7 message
- `{MSH.4}` → replaced with sending facility from HL7 message

---

## ⚠️ Important Notes

### 1. Aggregation Pipeline is an Array
```json
[  ← Must start with opening bracket
  { ... },
  { ... }
]  ← Must end with closing bracket
```

### 2. Each Stage is an Object
```json
[
  { "$match": { ... } },  ← Notice the curly braces
  { "$project": { ... } }
]
```

### 3. MongoDB Field References Use `$`
```json
{
  "$project": {
    "fullName": { "$concat": ["$firstName", " ", "$lastName"] }
                                    ↑              ↑
                         Dollar sign means "get field value"
  }
}
```

### 4. HL7 Field Placeholders Use `{}`
```json
{
  "$match": {
    "mrn": "{PID.3}"  ← Curly braces = HL7 field placeholder
  }
}
```

---

## 🧪 Testing Your Pipeline

### Step 1: Configure in UI
```
Database Type: MongoDB
Query Mode: ⚡ Advanced
Collection: patients
Aggregation Pipeline: [your JSON here]
```

### Step 2: Send Test Message
Send an HL7 message with test data:
```
PID|1||MRN123456^^^MRN||Doe^John||19800515|M
```

### Step 3: Check Logs
```bash
docker-compose logs -f app | grep MongoDB
```

Look for:
```
⚡ MongoDB Advanced Mode: Using aggregation pipeline with 2 stages
📋 MongoDB aggregation pipeline after substitution: ...
✅ MongoDB aggregation returned 1 documents
```

---

## 📖 Learn More

**Official MongoDB Documentation**:
- [Aggregation Pipeline](https://docs.mongodb.com/manual/core/aggregation-pipeline/)
- [Aggregation Operators](https://docs.mongodb.com/manual/reference/operator/aggregation/)
- [Pipeline Stages](https://docs.mongodb.com/manual/reference/operator/aggregation-pipeline/)

**Interactive Tutorial**:
- [MongoDB University - Aggregation](https://university.mongodb.com/)

---

## 🎯 Quick Reference Card

| Task | Visual Mode | Advanced Mode |
|------|-------------|---------------|
| Find by field | ✅ Easy | ✅ Possible |
| Select fields | ✅ Checkboxes | ✅ $project |
| Concatenate fields | ❌ Not possible | ✅ $concat |
| Rename fields | ❌ Not possible | ✅ Alias in $project |
| Calculate values | ❌ Not possible | ✅ Math operators |
| OR logic | ❌ Not possible | ✅ $or in $match |
| Array filtering | ❌ Not possible | ✅ $filter, $in |
| Join collections | ❌ Not possible | ✅ $lookup |
| Group/aggregate | ❌ Not possible | ✅ $group |

---

**Happy Advanced Querying! ⚡**

You now have the full power of MongoDB at your fingertips!
