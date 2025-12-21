# XPath Autocomplete Implementation Guide

## Overview
Smart field path selector with IntelliSense for pipeline step configuration. Users can type to search and select field paths from parsed message structures.

## Current Implementation Status

### ✅ Completed - HL7 v2.x Support
- Backend API for loading HL7 schemas
- XPath autocomplete frontend component
- Integration with ValidationRuleBuilder
- Support for actual parsed HL7 structure (`enhancedSegments`)

### ⏳ TODO - FHIR Support
- FHIR schema XPath tree builder (placeholder created, needs actual parsed FHIR structure)
- Universal FHIR layout definition (not yet created in codebase)
- FHIR-specific field path resolution

---

## Architecture

### Parsed HL7 Structure (Actual Runtime Format)

When HL7 messages are parsed, they create this JSON structure:

```json
{
  "success": true,
  "data": {
    "version": "2.3",
    "messageType": {
      "code": "ADT",
      "event": "A04",
      "name": "ADT^A04"
    },
    "enhancedSegments": {
      "MSH": {
        "key": "MSH",
        "name": "Message Header",
        "description": "...",
        "fields": [
          {
            "key": "MSH.3",
            "value": "SENDING_APPLICATION",
            "name": "Sending Application",
            "description": "...",
            "dataType": "HD",
            "position": 3,
            "hasValue": true,
            "subfields": [
              {
                "key": "MSH.3.1",
                "value": "...",
                "name": "Namespace Id",
                "dataType": "IS",
                "position": 1
              }
            ]
          }
        ]
      },
      "PID": {
        "key": "PID",
        "name": "Patient Identification",
        "fields": [
          {
            "key": "PID.5",
            "value": "MOUSE^MICKEY^",
            "name": "Patient Name",
            "dataType": "XPN",
            "position": 5,
            "subfields": [...]
          }
        ]
      }
    }
  }
}
```

### XPath Patterns for Parsed HL7

**Accessing Segment:**
```javascript
enhancedSegments.PID
```

**Accessing Field by Array Index:**
```javascript
enhancedSegments.PID.fields[4]          // PID.5 (array is 0-indexed, field is 1-indexed)
enhancedSegments.PID.fields[4].value    // "MOUSE^MICKEY^"
enhancedSegments.PID.fields[4].name     // "Patient Name"
```

**Accessing Subfield:**
```javascript
enhancedSegments.PID.fields[4].subfields[0].value  // First component of Patient Name
```

**Finding Field by Key:**
```javascript
// Using getNestedValue() helper with find logic:
enhancedSegments.PID.fields.find(f => f.key === 'PID.5').value
```

---

## Component Architecture

### Backend: Schema API (`controllers/schemaController.js`)

**Purpose**: Load HL7 schema definitions and build XPath trees for autocomplete

**Endpoints**:
- `GET /api/schemas/hl7/versions` - List HL7 versions (v2.1, v2.3, v2.5, etc.)
- `GET /api/schemas/hl7/:version/message-types` - List message types (ADT_A01, ORU_R01, etc.)
- `GET /api/schemas/hl7/:version/:messageType` - Get schema with XPath tree

**Schema Source**:
- Location: `schemas/hl7/v2.5/ADT_A01.gz`
- Format: Gzipped JSON
- Contains: Field definitions, data types, descriptions, components

**XPath Tree Builder** (`buildHL7XPathTree`):
```javascript
function buildHL7XPathTree(schema) {
  // Transforms HL7 schema into nested tree structure
  return {
    name: 'enhancedSegments',
    path: 'enhancedSegments',
    children: [
      {
        name: 'MSH',
        path: 'enhancedSegments.MSH',
        children: [
          {
            name: 'fields',
            path: 'enhancedSegments.MSH.fields',
            children: [
              {
                name: 'MSH.3',
                path: 'enhancedSegments.MSH.fields[2].value',  // Array index
                description: 'Sending Application',
                dataType: 'HD'
              }
            ]
          }
        ]
      }
    ]
  };
}
```

### Frontend: XPath Autocomplete Component (`public/js/pipeline/components/XPathAutocomplete.js`)

**Purpose**: Intelligent field path selector with search and autocomplete

**Features**:
- Real-time search as user types
- Fuzzy matching (path, name, description)
- Keyboard navigation (arrow keys, enter, escape)
- Scoring algorithm (exact match > starts with > contains)
- Top 50 results displayed
- Visual highlighting of matched text

**Usage**:
```javascript
const autocomplete = new XPathAutocomplete(container, {
  format: 'hl7v2',
  version: 'v2.5',
  messageType: 'ADT_A01',
  placeholder: 'Type to search field paths...',
  onChange: (path) => {
    console.log('Selected:', path);
    // path = "enhancedSegments.PID.fields[4].value"
  }
});
```

**Search Algorithm**:
```javascript
searchPaths(query) {
  // Score and rank results
  const scored = this.flattenedPaths.map(item => {
    let score = 0;

    // Exact match: +100 points
    if (pathLower === lowerQuery) score += 100;

    // Starts with: +50 points
    if (pathLower.startsWith(lowerQuery)) score += 50;

    // Contains: +20 points
    if (pathLower.includes(lowerQuery)) score += 20;

    // Shorter paths (more specific): +10 points
    score += Math.max(0, 10 - item.level);

    return { item, score };
  })
  .filter(({ score }) => score > 0)
  .sort((a, b) => b.score - a.score)
  .slice(0, 50);
}
```

### Integration: ValidationRuleBuilder

**Updated Constructor**:
```javascript
constructor(container, initialRules = [], options = {}) {
  this.options = {
    format: options.format || 'hl7v2',
    version: options.version || 'v2.5',
    messageType: options.messageType || '',
  };

  this.initializeXPathAutocompletes();
}
```

**Auto-initializes XPath Autocomplete** for each validation rule's field path input.

**Dynamic Schema Updates**:
```javascript
validationBuilder.updateSchemaOptions({
  format: 'hl7v2',
  version: 'v2.3',
  messageType: 'ADT_A04'
});
// All autocompletes reload with new schema
```

---

## File Structure

```
Backend (Node.js):
├── controllers/schemaController.js      ✅ Complete (HL7 only)
├── routes/schemaRoutes.js               ✅ Complete
└── schemas/
    └── hl7/
        ├── v2.1/
        ├── v2.3/
        │   └── ADT_A01.gz               ✅ Schema files exist
        └── v2.5/

Frontend (JavaScript):
├── public/js/pipeline/components/
│   ├── XPathAutocomplete.js             ✅ Complete
│   └── ValidationRuleBuilder.js         ✅ Updated with XPath support
├── public/css/components/
│   └── xpath-autocomplete.css           ✅ Complete
└── public/test-xpath-autocomplete.html  ✅ Test page created
```

---

## How It Works (Step-by-Step)

### 1. User Opens Validation Rule Builder
```javascript
const builder = new ValidationRuleBuilder(container, [], {
  format: 'hl7v2',
  version: 'v2.5',
  messageType: 'ADT_A01'
});
```

### 2. Component Initializes XPath Autocomplete
- Calls `initializeXPathAutocompletes()`
- Creates autocomplete for each field path input
- Loads schema from backend API

### 3. Backend Loads Schema
```javascript
// Request: GET /api/schemas/hl7/v2.5/ADT_A01

// Backend:
1. Read schemas/hl7/v2.5/ADT_A01.gz
2. Decompress gzip
3. Parse JSON schema
4. Build XPath tree (buildHL7XPathTree)
5. Return tree structure
```

### 4. Frontend Flattens Tree
```javascript
// XPath tree becomes flat list:
[
  { path: 'enhancedSegments.MSH.fields[2].value', name: 'MSH.3', description: 'Sending Application' },
  { path: 'enhancedSegments.PID.fields[4].value', name: 'PID.5', description: 'Patient Name' },
  { path: 'enhancedSegments.PID.fields[4].subfields[0].value', name: 'PID.5.1', description: 'Family Name' },
  // ... 200+ paths
]
```

### 5. User Types to Search
```
User types: "patient"

Search algorithm finds:
- enhancedSegments.PID.fields[4].value (Patient Name) ← Score: 50
- enhancedSegments.PID.fields[2].value (Patient ID) ← Score: 40
- ... (sorted by relevance)

Dropdown shows top 50 results
```

### 6. User Selects Path
```javascript
onChange: (path) => {
  // path = "enhancedSegments.PID.fields[4].value"

  // Updates validation rule:
  rule.field = path;

  // At runtime, executor uses getNestedValue():
  const value = getNestedValue(parsedHL7, path);
  // value = "MOUSE^MICKEY^"
}
```

---

## TODO: FHIR Support (Future Implementation)

### What's Needed

#### 1. Define Universal Parsed FHIR Structure
**Current Status**: Not yet defined in codebase

**Proposed Structure** (to be finalized):
```json
{
  "resourceType": "Bundle",
  "entry": [
    {
      "resource": {
        "resourceType": "Patient",
        "identifier": [
          {
            "system": "http://...",
            "value": "12345"
          }
        ],
        "name": [
          {
            "family": "Mouse",
            "given": ["Mickey"]
          }
        ],
        "birthDate": "1928-11-18"
      }
    }
  ]
}
```

**XPath Pattern** (proposed):
```javascript
entry[0].resource.identifier[0].value  // Patient ID
entry[0].resource.name[0].family       // Family Name
entry[0].resource.birthDate            // Birth Date
```

#### 2. Update Schema Controller

**File**: `controllers/schemaController.js`

**Function to Update**: `buildFHIRXPathTree(schema, resourceType)`

**Current Status**: Placeholder implementation exists, but needs actual parsed FHIR structure

**What to Do**:
```javascript
function buildFHIRXPathTree(schema, resourceType) {
  // TODO: Once universal FHIR structure is defined:
  // 1. Map FHIR schema elements to parsed structure
  // 2. Build tree matching actual runtime JSON
  // 3. Handle nested resources, arrays, extensions

  // Placeholder structure (not production-ready)
  return {
    name: 'entry[0].resource',
    path: 'entry[0].resource',
    children: [
      // Build from actual parsed FHIR structure
    ]
  };
}
```

#### 3. Test with Real FHIR Messages

**Steps**:
1. Create sample FHIR transformation
2. Capture actual parsed FHIR JSON structure
3. Save as `parsedFHIR.json` (like `parsedhl7.json`)
4. Update `buildFHIRXPathTree()` to match actual structure
5. Test autocomplete with real FHIR paths

#### 4. Update Frontend Options

**File**: `public/js/pipeline/components/XPathAutocomplete.js`

**Current**:
```javascript
if (this.options.format === 'hl7v2') {
  url = `/api/schemas/hl7/${version}/${messageType}`;
} else if (this.options.format === 'fhir') {
  url = `/api/schemas/fhir/${version}/${resourceType}`;  // ✅ Already implemented
}
```

**Status**: Frontend already supports FHIR format switching, just needs backend to return correct structure.

---

## Testing

### Test Page
**Location**: `/test-xpath-autocomplete.html`

**Tests**:
1. Standalone XPath Autocomplete
2. Validation Rule Builder Integration
3. API Endpoint Tests

**To Run**:
```bash
# Start Node.js frontend
node server.js

# Navigate to:
http://localhost:3000/test-xpath-autocomplete.html
```

### Manual Testing Steps

1. **Test HL7 Schema Loading**:
   - Click "Get HL7 Versions" → Should show v2.1, v2.3, v2.5, etc.
   - Click "Get ADT_A01 Schema" → Should show XPath tree

2. **Test Autocomplete**:
   - Type "patient" → Should show PID fields
   - Type "MSH.3" → Should show Sending Application
   - Type "name" → Should show Patient Name, Operator Name, etc.

3. **Test Validation Builder**:
   - Add validation rule
   - Type in field path input
   - Should see autocomplete dropdown
   - Select path
   - Click "Get Current Rules" → Should show selected path in JSON

### Example Searches

| Search Query | Expected Results |
|--------------|-----------------|
| `patient` | PID.5 (Patient Name), PID.3 (Patient ID), etc. |
| `MSH` | All MSH fields |
| `date` | Birth Date, Event Date/Time, etc. |
| `identifier` | Patient ID, Message Control ID, etc. |
| `PID.5` | Patient Name field |
| `name` | All name-related fields across segments |

---

## Configuration Examples

### Validation Rule Builder with XPath

```javascript
// Initialize with HL7 v2.5 ADT^A01
const builder = new ValidationRuleBuilder(container, [], {
  format: 'hl7v2',
  version: 'v2.5',
  messageType: 'ADT_A01'
});

// User configures validation rule:
// Field: enhancedSegments.PID.fields[2].value (Patient ID)
// Type: required
// Error: "Patient ID is required"

// At runtime, validation step will:
const patientId = getNestedValue(parsedHL7, 'enhancedSegments.PID.fields[2].value');
if (!patientId) {
  throw new Error("Patient ID is required");
}
```

### Switching Message Types

```javascript
// User switches from ADT^A01 to ORU^R01
builder.updateSchemaOptions({
  messageType: 'ORU_R01'
});

// All autocompletes reload with new schema
// Now shows OBR, OBX segments instead of PID, PV1
```

---

## Best Practices

### For Developers

1. **Always use actual parsed structure** - Don't guess paths, use real `parsedhl7.json` format
2. **Test with multiple HL7 versions** - Schemas differ between v2.3, v2.5, v2.7
3. **Handle missing fields gracefully** - Not all segments have all fields populated
4. **Use `getNestedValue()` helper** - Already handles array access and null safety

### For Users

1. **Start typing the segment name** (PID, MSH, EVN) for fastest results
2. **Use field numbers** (PID.5, MSH.3) for exact matches
3. **Search by description** ("patient name") for discovery
4. **Check field data type** - Shown in autocomplete dropdown

---

## Migration Path for FHIR

When ready to implement FHIR support:

### Step 1: Define Parsed Structure
```bash
# Create example FHIR transformation
# Capture output to parsedFHIR.json
# Document structure in this file
```

### Step 2: Update Backend
```javascript
// File: controllers/schemaController.js
function buildFHIRXPathTree(schema, resourceType) {
  // Implement based on parsedFHIR.json structure
}
```

### Step 3: Test and Iterate
```javascript
// Use test page to verify:
// - Schema loading works
// - Autocomplete shows correct paths
// - Selected paths work with getNestedValue()
```

### Step 4: Document Patterns
```markdown
# Update this file with:
# - FHIR XPath patterns
# - Real examples from parsedFHIR.json
# - Edge cases and gotchas
```

---

## Summary

### What Works Now (HL7)
✅ Backend API loads HL7 schemas
✅ XPath tree builder creates searchable structure
✅ Frontend autocomplete with fuzzy search
✅ Validation rule builder integration
✅ Test page for verification

### What's Pending (FHIR)
⏳ Define universal parsed FHIR structure
⏳ Implement FHIR XPath tree builder
⏳ Test with real FHIR transformations
⏳ Document FHIR-specific patterns

### Next Steps
1. Use system with HL7 messages
2. Gather feedback on autocomplete UX
3. When FHIR transformations are ready, capture parsed structure
4. Implement FHIR support using same pattern as HL7
