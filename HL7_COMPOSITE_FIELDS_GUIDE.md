# HL7 Composite Fields Guide

## Date
December 25, 2025

## Issue
MySQL query returning 0 rows even though:
- Field path `PID.3` is working correctly ✅
- Query parameter is being extracted ✅
- But value is `P123456^^^RIH^MR` instead of just `P123456` ❌

## Root Cause
`PID.3` is a **composite field** in HL7, meaning it contains multiple subcomponents separated by `^`.

### PID-3 Structure (Patient Identifier List)
```
P123456^^^RIH^MR
   ↓    ↓ ↓  ↓  ↓
  3.1  3.2 3.3 3.4 3.5

PID.3.1 = P123456  (ID Number - the actual patient ID)
PID.3.2 = (empty)  (Check Digit)
PID.3.3 = (empty)  (Check Digit Scheme)
PID.3.4 = RIH      (Assigning Authority)
PID.3.5 = MR       (Identifier Type Code - "Medical Record")
```

## Solution
Use the **first subcomponent** `PID.3.1` instead of the full composite `PID.3`.

### Field Path Comparison
| Path | Value | Use Case |
|------|-------|----------|
| `PID.3` | `P123456^^^RIH^MR` | Full composite - use for display or when you need all components |
| `PID.3.1` | `P123456` | Just the ID number - **use for database lookups** |
| `PID.3.4` | `RIH` | Just the assigning authority |
| `PID.3.5` | `MR` | Just the identifier type |

## How to Fix Your MySQL Query

### Current Configuration (Wrong)
```javascript
Query: SELECT * FROM patients WHERE mrn = ?
Parameter "1": PID.3
Result: Searches for mrn = 'P123456^^^RIH^MR' → No match ❌
```

### Correct Configuration
```javascript
Query: SELECT * FROM patients WHERE mrn = ?
Parameter "1": PID.3.1
Result: Searches for mrn = 'P123456' → Match! ✅
```

## Steps to Update

1. **Open MySQL database enrichment step**
2. **Click in Query Parameters VALUE field**
3. **Delete current value** (`PID.3`)
4. **Type "patient mrn"** in search
5. **Select "Patient MRN"** from dropdown
6. **Manually edit** to change `PID.3` to `PID.3.1`
7. **Save** and **Test Query** with test value: `P123456`

## Common HL7 Composite Fields

### Patient Name (PID-5)
```
Doe^John^M^Jr^Dr
 ↓   ↓   ↓ ↓  ↓
5.1 5.2 5.3 5.4 5.5
```
| Path | Value | Description |
|------|-------|-------------|
| `PID.5` | `Doe^John^M^Jr^Dr` | Full name composite |
| `PID.5.1` | `Doe` | Family Name (Last Name) |
| `PID.5.2` | `John` | Given Name (First Name) |
| `PID.5.3` | `M` | Middle Initial |
| `PID.5.4` | `Jr` | Suffix |
| `PID.5.5` | `Dr` | Prefix |

### Patient Address (PID-11)
```
123 Main St^^Springfield^IL^62701
     ↓      ↓      ↓     ↓    ↓
   11.1   11.2   11.3  11.4  11.5
```
| Path | Value | Description |
|------|-------|-------------|
| `PID.11` | `123 Main St^^Springfield^IL^62701` | Full address |
| `PID.11.1` | `123 Main St` | Street Address |
| `PID.11.2` | (empty) | Other Designation |
| `PID.11.3` | `Springfield` | City |
| `PID.11.4` | `IL` | State |
| `PID.11.5` | `62701` | Zip Code |

### Phone Number (PID-13)
```
555-1234^PRN^^john@email.com
    ↓     ↓  ↓       ↓
  13.1  13.2 13.3   13.4
```
| Path | Value | Description |
|------|-------|-------------|
| `PID.13` | `555-1234^PRN^^john@email.com` | Full phone/contact |
| `PID.13.1` | `555-1234` | Phone Number |
| `PID.13.2` | `PRN` | Telecommunication Use Code |
| `PID.13.3` | (empty) | Telecommunication Equipment Type |
| `PID.13.4` | `john@email.com` | Email Address |

### Visit Number (PV1-19)
```
V12345^^^RIH^VN
   ↓   ↓ ↓  ↓  ↓
 19.1 19.2 19.3 19.4 19.5
```
| Path | Value | Description |
|------|-------|-------------|
| `PV1.19` | `V12345^^^RIH^VN` | Full visit identifier |
| `PV1.19.1` | `V12345` | Visit Number (the actual ID) |
| `PV1.19.4` | `RIH` | Assigning Authority |
| `PV1.19.5` | `VN` | Identifier Type (Visit Number) |

## When to Use Full Composite vs Subcomponent

### Use Full Composite (`PID.3`)
- **Display** - Show complete identifier with authority
- **Logging** - Audit trail with full context
- **Export to FHIR** - Map entire identifier object
- **Forward to another system** - Preserve all components

### Use Subcomponent (`PID.3.1`)
- **Database lookups** - Match against simple varchar column ✅
- **Comparisons** - Check if IDs match
- **Display single value** - Show just the ID number
- **Search** - Find by specific component

## FieldPathSearchComponent Update Needed

Currently, the search component suggests:
```javascript
{ name: 'Patient MRN', path: 'PID.3', ... }
```

For database queries, this should probably be:
```javascript
{ name: 'Patient MRN', path: 'PID.3.1', ... }
{ name: 'Patient MRN (Full)', path: 'PID.3', ... }
```

Or add a note in the description:
```javascript
{ name: 'Patient MRN', path: 'PID.3.1', description: 'Patient ID Number (PID-3.1) - use for database lookups', category: 'Patient' }
{ name: 'Patient MRN (Full)', path: 'PID.3', description: 'Full MRN with authority (PID-3)', category: 'Patient' }
```

## Database Column Design Recommendations

### Option 1: Store Just the ID (Current)
```sql
CREATE TABLE patients (
    mrn VARCHAR(50)  -- Store just "P123456"
);
```
**Query**: `WHERE mrn = ?` with parameter `PID.3.1`

### Option 2: Store Full Composite
```sql
CREATE TABLE patients (
    mrn VARCHAR(200)  -- Store "P123456^^^RIH^MR"
);
```
**Query**: `WHERE mrn = ?` with parameter `PID.3`

### Option 3: Store Components Separately
```sql
CREATE TABLE patients (
    mrn_id VARCHAR(50),           -- "P123456"
    mrn_authority VARCHAR(50),    -- "RIH"
    mrn_type VARCHAR(10)          -- "MR"
);
```
**Query**: `WHERE mrn_id = ?` with parameter `PID.3.1`

## Testing Your Fix

After changing to `PID.3.1`:

```bash
docker-compose logs app --tail 50 | grep "Parameter 1"
```

**Expected output**:
```
📋 Parameter 1 = P123456 (from PID.3.1)
✅ [Database Enrichment] Query successful, 1 rows returned
```

**NOT** (current broken state):
```
📋 Parameter 1 = P123456^^^RIH^MR (from PID.3)
✅ [Database Enrichment] Query successful, 0 rows returned
```

## Related Documentation
- [HL7_FIELD_PATH_FORMAT_UPDATE.md](HL7_FIELD_PATH_FORMAT_UPDATE.md)
- [MYSQL_QUERY_PARAMETER_FIX.md](MYSQL_QUERY_PARAMETER_FIX.md)

## Summary
✅ Field path format is correct (`PID.3`)
⚠️ **Issue**: Need subcomponent (`PID.3.1`) for database lookup
📝 **Action**: Change query parameter from `PID.3` to `PID.3.1`
🎯 **Result**: Query will match `mrn = 'P123456'` in database
