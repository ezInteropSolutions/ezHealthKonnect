# API Response Mapping - Complete Implementation Guide

**Date**: 2025-12-20
**Version**: 1.0.0
**Status**: ✅ Implementation Complete

## Overview

The API Response Mapping system allows you to extract and transform specific fields from API responses instead of storing the entire response. This feature is fully integrated into the existing pipeline architecture - no separate systems or flows.

### **Key Design Principle**: Aligned to Pipeline Architecture

Response mapping is configured as part of the step configuration (`config` JSONB), not as a separate system. This follows the existing pattern:

```
Interface → Pipeline → Steps → Step Config (including responseMapping)
```

---

## Architecture

### Database Schema (V38 Migration)

**One New Table**: `response_mapping_templates`

```sql
CREATE TABLE response_mapping_templates (
    id UUID PRIMARY KEY,
    template_name VARCHAR(200) NOT NULL,
    description TEXT,
    api_type VARCHAR(100),  -- 'empi', 'ehr', 'lims', 'insurance'
    vendor VARCHAR(100),    -- 'epic', 'cerner', 'custom'
    mapping_rules JSONB NOT NULL,  -- Array of extraction rules
    is_system_template BOOLEAN DEFAULT false,
    created_by UUID REFERENCES users(id),
    organization_id UUID,  -- NULL = shared globally
    version INTEGER DEFAULT 1,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE
);
```

**System-Provided Templates** (Seeded in Migration):
- Epic EMPI Patient Lookup
- Cerner Patient Demographics
- Generic JSON API - Simple Extract

---

## How It Works

### Step Configuration (Stored in `transformation_steps.config`)

Response mapping is configured directly in the step's `config` JSONB field:

```json
{
  "step_type": "pre.enrichment.api",
  "config": {
    "endpoint": "https://api.epic.com/empi/patient/{patientId}",
    "method": "GET",
    "authType": "bearer",
    "bearerToken": "...",
    "fieldMappings": {
      "patientId": "PID.3"
    },
    "targetPath": "empiData",

    // ← Response mapping configuration
    "responseMapping": {
      "mode": "template",
      "templateId": "epic-empi-template-id"
    }
  }
}
```

### Four Mapping Modes

#### **1. Template Mode** (Use Template As-Is)

```json
{
  "responseMapping": {
    "mode": "template",
    "templateId": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

**Behavior**: Load template and apply its mapping rules exactly as defined.

**Use Case**: Standard Epic EMPI integration - same for all hospitals.

---

#### **2. Custom Mode** (No Template, Fully Custom)

```json
{
  "responseMapping": {
    "mode": "custom",
    "extractors": [
      {
        "sourcePath": "$.data.patient.id",
        "targetField": "patientId",
        "transformType": "none",
        "required": true
      },
      {
        "sourcePath": "$.data.patient.name.first",
        "targetField": "patientName",
        "transformType": "combine",
        "transformConfig": {
          "additionalPaths": ["$.data.patient.name.last"],
          "separator": " "
        }
      }
    ]
  }
}
```

**Behavior**: Use custom extractors only, ignore templates.

**Use Case**: Unique API with custom response structure.

---

#### **3. Extend Mode** (Template + Custom Fields)

```json
{
  "responseMapping": {
    "mode": "extend",
    "templateId": "epic-empi-template-id",
    "customExtractors": [
      {
        "sourcePath": "$.patient.extensions.vipStatus",
        "targetField": "vipFlag",
        "transformType": "none"
      }
    ]
  }
}
```

**Behavior**: Load template rules + add custom extractors on top.

**Use Case**: Standard Cerner API but hospital added custom extension fields.

---

#### **4. Override Mode** (Template with Specific Tweaks)

```json
{
  "responseMapping": {
    "mode": "override",
    "templateId": "epic-empi-template-id",
    "overrides": {
      "patientId": {
        "sourcePath": "$.patient.mrn",  // Use MRN instead
        "targetField": "patientId",
        "transformType": "none"
      }
    }
  }
}
```

**Behavior**: Use template but replace specific rules by `targetField`.

**Use Case**: Epic template works but this hospital stores MRN in different location.

---

## Transform Types

### **1. None** (Direct Extraction)

```json
{
  "sourcePath": "$.patient.id",
  "targetField": "patientId",
  "transformType": "none"
}
```

Extracts value as-is using JSONPath.

---

### **2. Combine** (Combine Multiple Fields)

```json
{
  "sourcePath": "$.patient.firstName",  // Not used in combine
  "targetField": "patientName",
  "transformType": "combine",
  "transformConfig": {
    "additionalPaths": ["$.patient.firstName", "$.patient.lastName"],
    "separator": " ",
    "format": "{0} {1}"  // Optional format string
  }
}
```

Combines multiple fields into one value.

**Example**:
- Input: `firstName: "John"`, `lastName: "Doe"`
- Output: `patientName: "John Doe"`

---

### **3. Filter** (Filter Array by Condition)

```json
{
  "sourcePath": "$.patient.insurance",
  "targetField": "primaryInsurance",
  "transformType": "filter",
  "transformConfig": {
    "filterField": "type",
    "filterOperator": "equals",  // equals, contains, startsWith, endsWith, gt, lt
    "filterValue": "primary",
    "extractField": "memberId"
  }
}
```

Filters an array and extracts a specific field.

**Example**:
```json
// Input
{
  "insurance": [
    {"type": "secondary", "memberId": "XYZ"},
    {"type": "primary", "memberId": "ABC123"}
  ]
}

// Output
{
  "primaryInsurance": "ABC123"
}
```

---

### **4. Format** (Date/Number Formatting)

```json
{
  "sourcePath": "$.patient.dateOfBirth",
  "targetField": "dateOfBirth",
  "transformType": "format",
  "transformConfig": {
    "inputFormat": "MM/DD/YYYY",
    "outputFormat": "YYYY-MM-DD",
    "formatType": "date"
  }
}
```

Converts date formats.

**Example**:
- Input: `"12/25/1990"`
- Output: `"1990-12-25"`

---

### **5. Conditional** (If-Then-Else Logic)

```json
{
  "sourcePath": null,
  "targetField": "statusDetails",
  "transformType": "conditional",
  "transformConfig": {
    "conditions": [
      {
        "if": {
          "field": "$.patient.status",
          "operator": "equals",
          "value": "active"
        },
        "then": {
          "field": "$.patient.activeDetails"
        }
      },
      {
        "if": {
          "field": "$.patient.status",
          "operator": "equals",
          "value": "inactive"
        },
        "then": {
          "value": "Patient inactive"
        }
      }
    ],
    "default": "Unknown status"
  }
}
```

Applies conditional logic to extract different values based on conditions.

---

## API Endpoints

### Base URL: `/api/response-mapping-templates`

#### **1. Create Template**

```http
POST /api/response-mapping-templates
Content-Type: application/json

{
  "template_name": "Custom Insurance API",
  "description": "Maps insurance verification API response",
  "api_type": "insurance",
  "vendor": "custom",
  "mapping_rules": [
    {
      "sourcePath": "$.policy.id",
      "targetField": "policyId",
      "transformType": "none",
      "required": true
    },
    {
      "sourcePath": "$.coverage.status",
      "targetField": "coverageStatus",
      "transformType": "none"
    }
  ]
}
```

**Response**:
```json
{
  "success": true,
  "template": {
    "id": "uuid-here",
    "template_name": "Custom Insurance API",
    "mapping_rules": [...],
    "created_at": "2025-12-20T10:00:00Z"
  }
}
```

---

#### **2. List Templates**

```http
GET /api/response-mapping-templates?apiType=empi&vendor=epic
```

**Response**:
```json
{
  "success": true,
  "templates": [
    {
      "id": "template-1",
      "template_name": "Epic EMPI Patient Lookup",
      "api_type": "empi",
      "vendor": "epic",
      "is_system_template": true
    }
  ],
  "total_count": 1
}
```

---

#### **3. Get Template**

```http
GET /api/response-mapping-templates/{templateId}
```

---

#### **4. Update Template**

```http
PUT /api/response-mapping-templates/{templateId}
Content-Type: application/json

{
  "description": "Updated description",
  "mapping_rules": [...]
}
```

---

#### **5. Delete Template**

```http
DELETE /api/response-mapping-templates/{templateId}
```

**Note**: Soft delete (sets `is_active = false`). Cannot delete system templates.

---

#### **6. Get Template Usage**

```http
GET /api/response-mapping-templates/{templateId}/usage
```

**Response**:
```json
{
  "success": true,
  "template_id": "uuid",
  "usage": [
    {
      "interface_name": "Hospital A - ADT Feed",
      "pipeline_name": "ADT^A01 Processing",
      "step_name": "Enrich from Epic EMPI",
      "step_id": "step-123",
      "mapping_mode": "template"
    }
  ],
  "usage_count": 1
}
```

---

## Complete Example: Epic EMPI Integration

### **1. Create Template** (One Time)

```json
{
  "template_name": "Epic EMPI Patient Lookup",
  "api_type": "empi",
  "vendor": "epic",
  "mapping_rules": [
    {
      "sourcePath": "$.patient.id",
      "targetField": "patientId",
      "transformType": "none",
      "required": true
    },
    {
      "sourcePath": "$.patient.firstName",
      "targetField": "patientName",
      "transformType": "combine",
      "transformConfig": {
        "additionalPaths": ["$.patient.lastName"],
        "separator": " "
      }
    },
    {
      "sourcePath": "$.patient.insurance",
      "targetField": "primaryInsurance",
      "transformType": "filter",
      "transformConfig": {
        "filterField": "type",
        "filterValue": "primary",
        "extractField": "memberId"
      }
    }
  ]
}
```

---

### **2. Create Pipeline Step** (Reference Template)

```sql
INSERT INTO transformation_steps (
  id, pipeline_id, step_name, step_type, sequence, config
) VALUES (
  'step-123',
  'pipeline-abc',
  'Enrich from Epic EMPI',
  'pre.enrichment.api',
  20,
  '{
    "endpoint": "https://api.epic.com/empi/patient/{patientId}",
    "method": "GET",
    "authType": "bearer",
    "bearerToken": "...",
    "fieldMappings": {
      "patientId": "PID.3"
    },
    "responseMapping": {
      "mode": "template",
      "templateId": "epic-template-id-here"
    }
  }'::jsonb
);
```

---

### **3. API Response Example**

```json
{
  "patient": {
    "id": "12345",
    "firstName": "John",
    "lastName": "Doe",
    "dateOfBirth": "12/25/1990",
    "insurance": [
      {"type": "secondary", "memberId": "XYZ"},
      {"type": "primary", "memberId": "ABC123"}
    ]
  }
}
```

---

### **4. Extracted Fields** (After Response Mapping)

```json
{
  "patientId": "12345",
  "patientName": "John Doe",
  "primaryInsurance": "ABC123"
}
```

These fields are stored **directly in the message** (not nested under `empiData`), making them immediately accessible for transformation and mapping.

---

## Benefits of This Design

✅ **Aligned to Pipeline**: Part of step config, follows existing pattern
✅ **Reusable**: Templates shared across steps, interfaces, organizations
✅ **Flexible**: 4 modes (template, custom, extend, override) cover all use cases
✅ **No New Flow**: Same pipeline → steps → config architecture
✅ **Performance**: Extract only needed fields, reduce message size
✅ **Maintainable**: Update template → all steps using it get updated
✅ **Testable**: Test mapping independently before applying to production

---

## Implementation Files

### Database
- [database/migrations/V38__Add_Response_Mapping_Templates.sql](database/migrations/V38__Add_Response_Mapping_Templates.sql)

### Models
- [models/response_mapping_models.go](models/response_mapping_models.go)

### Services
- [services/response_mapping_service.go](services/response_mapping_service.go) - Template CRUD
- [services/response_extractor_service.go](services/response_extractor_service.go) - Field extraction & transformation

### Controllers
- [controllers/response_mapping_controller.go](controllers/response_mapping_controller.go) - REST API endpoints

### Executors
- [services/executors/enrichment/api_enrichment_executor.go](services/executors/enrichment/api_enrichment_executor.go) - Extended with response mapping support

---

## Testing

### Manual Test (After Docker Build)

```bash
# 1. Create a template
curl -X POST http://localhost:8080/api/response-mapping-templates \
  -H "Content-Type: application/json" \
  -d '{
    "template_name": "Test Template",
    "api_type": "test",
    "vendor": "custom",
    "mapping_rules": [
      {
        "sourcePath": "$.id",
        "targetField": "id",
        "transformType": "none"
      }
    ]
  }'

# 2. List templates
curl http://localhost:8080/api/response-mapping-templates

# 3. Test pipeline with response mapping
# (Use existing pipeline test scripts)
```

---

## Next Steps

1. ✅ Database migration created (V38)
2. ✅ Models created
3. ✅ Services implemented (mapping + extraction)
4. ✅ API endpoints created
5. ✅ API enrichment executor extended
6. ⏳ **Docker rebuild and testing** (pending)
7. ⏳ **UI implementation** (drag & drop template builder)

---

## Migration Guide

### Before (Storing Full Response)

```json
{
  "config": {
    "endpoint": "https://api.epic.com/patient/{id}",
    "targetPath": "empiData"
  }
}
```

**Result**: Full API response stored at `message.empiData`

### After (With Response Mapping)

```json
{
  "config": {
    "endpoint": "https://api.epic.com/patient/{id}",
    "targetPath": "empiData",  // Fallback if mapping fails
    "responseMapping": {
      "mode": "custom",
      "extractors": [
        {"sourcePath": "$.id", "targetField": "patientId"}
      ]
    }
  }
}
```

**Result**: Only `patientId` extracted and stored at `message.patientId`

**Backward Compatibility**: If `responseMapping` is not configured, full response is stored at `targetPath` (existing behavior).

---

**Documentation Complete** | **Status**: ✅ Ready for Testing
