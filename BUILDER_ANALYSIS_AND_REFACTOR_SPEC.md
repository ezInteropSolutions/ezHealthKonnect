# Builder Analysis & Refactor Specification

## Current State Analysis

### Existing Builders (11 total)
1. **IfThenElseBuilder** (~750 lines) - Conditional logic configuration
2. **ValidationRuleBuilder** (~500 lines) - Field validation rules
3. **MetadataBuilder** (~300 lines) - Key-value metadata pairs
4. **OAuth2ConfigBuilder** (~400 lines) - OAuth 2.0 configuration
5. **HeaderBuilder** - HTTP header configuration
6. **QueryParamBuilder** - Query parameter builder
7. **ResultMappingBuilder** - Result field mapping
8. **MongoDBFilterBuilder** - MongoDB filter queries
9. **MongoDBProjectionBuilder** - MongoDB projection
10. **RedisQueryBuilder** - Redis query builder
11. **BaseStepConfigBuilder** (NEW) - Abstract base class

### Common Patterns Identified

| Pattern | ValidationRuleBuilder | MetadataBuilder | OAuth2ConfigBuilder | IfThenElseBuilder |
|---------|----------------------|-----------------|---------------------|-------------------|
| Constructor(container, initialConfig) | ✅ | ✅ | ✅ | ✅ |
| getConfig() / getData() | ✅ | ✅ | ✅ | ✅ |
| render() | ✅ | ✅ | ✅ (init()) | ✅ |
| Validation | ❌ None | ❌ None | ❌ None | ⚠️ Partial |
| Error Display | ❌ | ❌ | ❌ | ❌ |
| Destroy cleanup | ❌ | ❌ | ❌ | ✅ |
| Event handling | Manual | Manual | Manual | Manual |

### Code Duplication Issues

**Duplicated Code (~2000 lines total)**:
- Constructor boilerplate: 11 × 10 lines = 110 lines
- getConfig() logic: 11 × 5 lines = 55 lines
- render() scaffolding: 11 × 20 lines = 220 lines
- Event attachment: 11 × 30 lines = 330 lines
- No validation framework: 0 lines × 11 = MISSING
- No error display: 0 lines × 11 = MISSING

---

## Enterprise-Grade Refactor Specification

### Architecture Goals

1. **100% Validation Coverage** - Every builder validates input
2. **Standardized API** - Consistent interface across all builders
3. **DRY Principle** - Zero code duplication
4. **SOLID Principles** - Proper OOP design
5. **Error Handling** - User-friendly validation errors
6. **Testability** - Easy to unit test

### Validation Framework Design

#### Validation Rule Types

```javascript
const ValidationRuleTypes = {
    REQUIRED: 'required',           // Field must exist and not be empty
    TYPE: 'type',                   // Must be specific type (string, number, array, object)
    MIN_LENGTH: 'minLength',        // String/array minimum length
    MAX_LENGTH: 'maxLength',        // String/array maximum length
    MIN_VALUE: 'minValue',          // Number minimum value
    MAX_VALUE: 'maxValue',          // Number maximum value
    PATTERN: 'pattern',             // Regex pattern match
    ENUM: 'enum',                   // Must be one of allowed values
    CUSTOM: 'custom',               // Custom validation function
    ARRAY_ITEMS: 'arrayItems',      // Validate each array item
    OBJECT_SCHEMA: 'objectSchema'   // Validate object properties
};
```

#### Validation Schema Format

```javascript
// Example for IfThenElseBuilder
const validationSchema = {
    conditions: {
        type: 'array',
        required: true,
        minLength: 1,
        errorMessage: 'At least one condition is required',
        items: {
            type: 'object',
            required: true,
            schema: {
                name: {
                    type: 'string',
                    required: false,
                    maxLength: 100
                },
                condition: {
                    type: 'object',
                    required: true,
                    errorMessage: 'Condition configuration is required',
                    schema: {
                        field: {
                            type: 'string',
                            required: true,
                            minLength: 1,
                            errorMessage: 'Field path is required'
                        },
                        operator: {
                            type: 'string',
                            required: true,
                            enum: ['equals', 'not_equals', 'greater_than', 'less_than', 'contains', 'matches_regex'],
                            errorMessage: 'Invalid operator'
                        },
                        value: {
                            required: function(config) {
                                // Conditional: required if not comparing to field
                                return !config.compareToField;
                            }
                        }
                    }
                },
                onTrue: {
                    type: 'object',
                    required: true,
                    schema: {
                        action: {
                            type: 'string',
                            required: true,
                            enum: ['continue', 'stop', 'set_value', 'delete_field', 'route_to_step'],
                            errorMessage: 'TRUE action is required'
                        },
                        stepId: {
                            required: function(config) {
                                return config.action === 'route_to_step';
                            },
                            errorMessage: 'Step ID is required for route_to_step action'
                        }
                    }
                },
                onFalse: {
                    type: 'object',
                    required: true,
                    schema: {
                        action: {
                            type: 'string',
                            required: true,
                            enum: ['continue', 'stop', 'set_value', 'delete_field', 'route_to_step'],
                            errorMessage: 'FALSE action is required'
                        },
                        stepId: {
                            required: function(config) {
                                return config.action === 'route_to_step';
                            },
                            errorMessage: 'Step ID is required for route_to_step action'
                        }
                    }
                }
            }
        }
    }
};
```

---

## Refactored Builder Specifications

### 1. IfThenElseBuilder (Refactored)

**File**: `public/js/pipeline/components/IfThenElseBuilder.js`

**Purpose**: Configure conditional logic with TRUE/FALSE routing

**Config Structure**:
```javascript
{
    conditions: [
        {
            name: "Check Gender",
            condition: {
                field: "PID.8",
                operator: "equals",
                value: "M",
                compareToField: ""
            },
            onTrue: {
                action: "route_to_step",
                stepId: "step-90",
                field: "",
                value: ""
            },
            onFalse: {
                action: "route_to_step",
                stepId: "step-100"
            }
        }
    ]
}
```

**Validation Rules**:
- ✅ At least 1 condition required
- ✅ Each condition.field must be non-empty
- ✅ Each condition.operator must be valid enum
- ✅ condition.value required if not comparing to field
- ✅ onTrue.action required (enum validation)
- ✅ onTrue.stepId required if action = "route_to_step"
- ✅ onFalse.action required (enum validation)
- ✅ onFalse.stepId required if action = "route_to_step"
- ✅ Circular routing detection (step cannot route to itself)

**Error Messages**:
```javascript
[
    "Condition 1: Field path is required",
    "Condition 1: Invalid operator 'invalid_op'",
    "Condition 1: Value is required when not comparing to field",
    "Condition 1: TRUE action is required",
    "Condition 1: Step ID is required for route_to_step action",
    "Condition 2: FALSE action routes to itself (circular reference)"
]
```

---

### 2. ValidationRuleBuilder (Refactored)

**File**: `public/js/pipeline/components/ValidationRuleBuilder.js`

**Purpose**: Configure field validation rules (required, format, length, pattern)

**Config Structure**:
```javascript
{
    rules: [
        {
            field: "PID.3",
            validationType: "required",
            errorMessage: "Patient ID is required"
        },
        {
            field: "PID.5",
            validationType: "format",
            format: "phone",
            errorMessage: "Invalid phone number format"
        },
        {
            field: "PID.7",
            validationType: "pattern",
            pattern: "^\\d{8}$",
            errorMessage: "Date of birth must be YYYYMMDD"
        }
    ]
}
```

**Validation Rules**:
- ✅ At least 1 rule required
- ✅ Each rule.field must be non-empty
- ✅ Each rule.validationType must be valid enum
- ✅ rule.format required if validationType = "format"
- ✅ rule.pattern required and valid regex if validationType = "pattern"
- ✅ rule.minLength/maxLength must be positive integers if type = "length"
- ✅ Duplicate field detection (same field with same validation type)

---

### 3. MetadataBuilder (Refactored)

**File**: `public/js/pipeline/components/MetadataBuilder.js`

**Purpose**: Configure custom key-value metadata pairs

**Config Structure**:
```javascript
{
    priority: "high",
    source_system: "EPIC",
    environment: "production",
    custom_field_1: "value1"
}
```

**Validation Rules**:
- ✅ All keys must be non-empty strings
- ✅ Keys must match pattern: `^[a-zA-Z_][a-zA-Z0-9_]*$` (valid identifier)
- ✅ No duplicate keys
- ✅ Reserved keys warning: "step_id", "pipeline_id", "timestamp" (system reserved)

---

### 4. OAuth2ConfigBuilder (Refactored)

**File**: `public/js/pipeline/components/OAuth2ConfigBuilder.js`

**Purpose**: Configure OAuth 2.0 authentication settings

**Config Structure**:
```javascript
{
    grantType: "client_credentials",
    tokenUrl: "https://auth.example.com/oauth/token",
    clientId: "my-client-id",
    clientSecret: "my-client-secret",
    scope: "read write",
    audience: "https://api.example.com"
}
```

**Validation Rules**:
- ✅ grantType required (enum: client_credentials, password, refresh_token)
- ✅ tokenUrl required and valid URL format
- ✅ clientId required if grantType = "client_credentials"
- ✅ clientSecret required if grantType = "client_credentials"
- ✅ username/password required if grantType = "password"
- ✅ refreshToken required if grantType = "refresh_token"

---

### 5. HeaderBuilder (Refactored)

**Purpose**: Configure HTTP headers for API calls

**Config Structure**:
```javascript
{
    headers: [
        { key: "Content-Type", value: "application/json" },
        { key: "Authorization", value: "Bearer ${token}" },
        { key: "X-Custom-Header", value: "custom-value" }
    ]
}
```

**Validation Rules**:
- ✅ Each header.key must be non-empty
- ✅ Header keys must match HTTP header name format
- ✅ No duplicate header keys
- ✅ Reserved headers warning: "Host", "Connection" (auto-set)

---

### 6. QueryParamBuilder (Refactored)

**Purpose**: Configure URL query parameters

**Config Structure**:
```javascript
{
    params: [
        { key: "page", value: "1" },
        { key: "limit", value: "100" },
        { key: "filter", value: "${dynamic_filter}" }
    ]
}
```

**Validation Rules**:
- ✅ Each param.key must be non-empty
- ✅ Param keys must be valid URL-safe characters
- ✅ No duplicate param keys

---

### 7. MongoDBFilterBuilder (Refactored)

**Purpose**: Configure MongoDB query filters

**Config Structure**:
```javascript
{
    filter: {
        "$and": [
            { "patient.gender": "M" },
            { "patient.age": { "$gte": 65 } }
        ]
    }
}
```

**Validation Rules**:
- ✅ Filter must be valid JSON object
- ✅ MongoDB operators must be valid ($eq, $ne, $gt, $gte, $lt, $lte, $and, $or, etc.)
- ✅ Syntax validation for complex queries

---

## Implementation Plan

### Phase 1: Foundation (Week 1)
- [x] Create BaseStepConfigBuilder.js with validation framework
- [ ] Create ValidationEngine.js (validation rule processor)
- [ ] Create StepConfigBuilderFactory.js
- [ ] Create comprehensive test suite for base class

### Phase 2: Rebuild Critical Builders (Week 2)
- [ ] Rebuild IfThenElseBuilder (100% validation)
- [ ] Rebuild ValidationRuleBuilder (100% validation)
- [ ] Rebuild MetadataBuilder (100% validation)
- [ ] Rebuild OAuth2ConfigBuilder (100% validation)

### Phase 3: Rebuild Remaining Builders (Week 3)
- [ ] Rebuild HeaderBuilder
- [ ] Rebuild QueryParamBuilder
- [ ] Rebuild ResultMappingBuilder
- [ ] Rebuild MongoDB builders
- [ ] Rebuild RedisQueryBuilder

### Phase 4: Integration (Week 4)
- [ ] Update PropertiesPanel to use Factory
- [ ] Update ToolboxManager to register all builders
- [ ] End-to-end testing
- [ ] Documentation

---

## Expected Outcomes

### Code Reduction
- **Before**: 5,807 lines across 11 files
- **After**: ~3,500 lines (40% reduction)
  - BaseStepConfigBuilder: 500 lines
  - ValidationEngine: 300 lines
  - Factory: 100 lines
  - Each builder: ~200-300 lines (down from 400-750)

### Quality Improvements
- **Validation Coverage**: 0% → 100%
- **Error Messages**: None → User-friendly
- **Code Duplication**: ~2000 lines → 0 lines
- **Test Coverage**: 0% → 80%+
- **Time to add new builder**: 4-6 hours → 30-60 minutes

### Developer Experience
- **Consistent API**: All builders have same methods
- **Self-documenting**: Validation schemas document config structure
- **Type Safety**: Validation enforces correct types
- **Error Prevention**: Catches issues before backend

---

## Next Step

Create **ValidationEngine.js** - the core validation framework that BaseStepConfigBuilder will use.
