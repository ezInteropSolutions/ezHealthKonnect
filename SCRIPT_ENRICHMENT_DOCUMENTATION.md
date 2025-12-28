# Script Enrichment Step - Comprehensive Documentation

**Date:** December 26, 2025
**Feature:** Complete documentation for Script Enrichment step in Step tab

---

## Overview

Added comprehensive documentation for the Script Enrichment step (`pre.enrichment.script`) in the getStepDocumentation section of PropertiesPanel.js. This documentation provides users with clear guidance on writing JavaScript code to perform calculations, validations, and custom transformations using data from HL7 messages and previous enrichment steps.

---

## 🎯 Documentation Sections Added

### 1. **Description** ✅

**File:** [PropertiesPanel.js](public/js/pipeline/managers/PropertiesPanel.js:4358-4359)

Clear explanation of script enrichment purpose:
- Execute custom JavaScript logic for calculations and transformations
- Access HL7 message fields and previous enrichment step results
- Store results in `enriched_data` for use in subsequent steps
- Handle complex business rules that can't be achieved with simple mappings

### 2. **Use Cases** (8 Real-World Scenarios) ✅

**Lines:** 4360-4369

```javascript
useCases: [
    'Calculate patient risk scores based on age, diagnoses, and lab values',
    'Determine insurance eligibility by combining patient demographics with external data',
    'Perform complex date calculations (e.g., days since last admission, appointment intervals)',
    'Apply business rules for message routing or prioritization',
    'Validate and transform data formats (e.g., normalize phone numbers, format addresses)',
    'Enrich data by combining HL7 fields with database/API results from previous steps',
    'Generate derived fields (e.g., BMI from height/weight, age from date of birth)',
    'Implement conditional logic for data transformation (if-then-else scenarios)'
]
```

### 3. **Complete Working Example** ✅

**Lines:** 4370-4418

Comprehensive risk scoring example showing:
- Accessing HL7 message fields (`getHL7Field`)
- Accessing database enrichment results (`getNestedValue` with database step)
- Accessing API enrichment results (`getNestedValue` with API step)
- Performing calculations (age, days since admission)
- Implementing business logic (risk score calculation)
- Conditional logic (risk level determination)
- Building arrays of factors
- Returning structured enriched data

**Key features demonstrated:**
```javascript
// Access HL7 fields
var dateOfBirth = getHL7Field(input, "PID.7");

// Access previous enrichment steps
var chronicConditions = getNestedValue(input, '["database_enrichment"].enriched_data.chronicConditions');
var insuranceStatus = getNestedValue(input, '["API_Enrichment"].enriched_data.insuranceActive');

// Calculate and apply business rules
var riskScore = 0;
if (patientAge > 65) riskScore += 3;
if (chronicConditions > 2) riskScore += 4;

// Return structured data
return {
    riskScore: riskScore,
    riskLevel: riskLevel,
    riskFactors: riskFactors,
    calculatedAt: new Date().toISOString()
};
```

### 4. **Parameters Documentation** ✅

**Lines:** 4419-4438

Three parameters clearly documented:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `script` | string (JavaScript) | Yes | JavaScript code to execute. Receives input object with HL7 message and enriched data. Must return object with calculated fields. |
| `timeout_ms` | number | No | Maximum execution time in milliseconds (default: 5000). Script terminated if exceeded. |
| `failOnError` | boolean | No | Whether to fail pipeline if script fails (default: false). Set true for critical calculations. |

### 5. **Reference Variables Section** ✅

**Lines:** 4439-4469

Five detailed examples showing data access patterns:

| Scenario | Code Example | Explanation |
|----------|--------------|-------------|
| Access HL7 Fields | `var patientId = getHL7Field(input, "PID.3");` | Extract values from HL7 message segments |
| Database Enrichment | `var conditions = getNestedValue(input, '["database_enrichment"].enriched_data.chronicConditions');` | Access data from previous database steps |
| API Enrichment | `var externalId = getNestedValue(input, '["API_Enrichment"].enriched_data.externalPatientId');` | Access data from API calls |
| Metadata Fields | `var customValue = getNestedValue(input, "metadata.customField");` | Access metadata enrichment results |
| Return Values | `return { riskScore: 9, riskLevel: "moderate", calculatedAt: new Date().toISOString() };` | How to structure return object for subsequent steps |

### 6. **Available Functions Reference** ✅

**Lines:** 4470-4510

Complete documentation of 5 helper functions:

1. **getHL7Field(input, path)**
   - Extract field values from HL7 message
   - Example: `var name = getHL7Field(input, "PID.5");`

2. **getNestedValue(input, xpath)**
   - Access nested enriched_data using XPath notation
   - Example: `var conditions = getNestedValue(input, '["database_enrichment"].enriched_data.chronicConditions');`

3. **calculateAge(dateOfBirth)**
   - Calculate age from HL7 date (YYYYMMDD)
   - Example: `var age = calculateAge("19800515");`

4. **calculateDaysSince(dateString)**
   - Calculate days between date and now
   - Example: `var days = calculateDaysSince("20240101");`

5. **formatDate(hl7Date, format)**
   - Convert HL7 date to custom format
   - Example: `var isoDate = formatDate("20240115", "YYYY-MM-DD");`

### 7. **Best Practices** (5 Key Recommendations) ✅

**Lines:** 4511-4537

| Practice | Recommendation | Example |
|----------|----------------|---------|
| Error Handling | Always validate input before calculations | `if (chronicConditions && chronicConditions > 0) { /* safe */ }` |
| Performance | Keep scripts lightweight, avoid complex loops | Use database enrichment for heavy operations, script for calculations |
| Naming | Use clear, descriptive field names | `return { patientRiskScore: score }` not `return { r: score }` |
| Testing | Use Test Execution feature before deployment | Click "Test Execution" and review Script Enrichment output |
| Data Access | Use Variables tab for copy-paste XPaths | Copy XPath from Variables tab, use with getNestedValue() |

---

## 📊 Documentation Structure

### Complete Section Organization:
```javascript
'pre.enrichment.script': {
    description: '...',           // What it does
    useCases: [...],              // 8 real-world scenarios
    example: {                    // Complete working example
        script: `...`,            // Full JavaScript code
        timeout_ms: 5000,
        failOnError: false
    },
    parameters: [...],            // 3 parameters documented
    referenceVariables: {         // 5 data access examples
        title: '...',
        description: '...',
        examples: [...]
    },
    availableFunctions: {         // 5 helper functions
        title: '...',
        description: '...',
        functions: [...]
    },
    bestPractices: [...]          // 5 recommendations
}
```

---

## 🔗 Integration with Reference Variables System

The script enrichment documentation seamlessly integrates with the Reference Variables system:

1. **Variables Tab Reference**
   - Best practices section directs users to click Variables tab
   - Shows how to copy XPaths and use them with `getNestedValue()`
   - Encourages NO-CODE workflow (click to copy, paste into script)

2. **Data Access Patterns**
   - Uses exact XPath format from Variables tab: `["Step Name"].enriched_data.fieldName`
   - Matches runtime structure from test execution output
   - Examples show accessing database, API, and metadata enrichment results

3. **Step Chaining**
   - Clear examples of accessing previous step data
   - Shows how script enrichment can combine multiple data sources
   - Demonstrates building complex calculations from simple enrichment steps

---

## 🎨 Visual Consistency

Follows the same documentation pattern as:
- **Database Enrichment** (lines 4075-4357) - Similar structure with connection details
- **API Enrichment** (lines 4026-4074) - HTTP request configuration patterns
- **Field Mapping** (lines 4594+) - Reference variables usage examples

**Common Elements:**
- Purple gradient theme for info boxes
- Comprehensive parameter documentation
- Real-world use cases
- Working code examples
- Reference variables integration
- Best practices section

---

## ✅ Completeness Checklist

- [x] Description clearly explains purpose and capabilities
- [x] 8 real-world use cases covering common scenarios
- [x] Complete working example (45+ lines of actual code)
- [x] All 3 parameters fully documented
- [x] Reference variables section with 5 access patterns
- [x] All 5 helper functions documented with examples
- [x] 5 best practices with actionable recommendations
- [x] Integration with Variables tab workflow
- [x] Matches runtime data structure (`["Step Name"].enriched_data`)
- [x] Follows documentation pattern of other enrichment steps

---

## 📚 Related Documentation

- [FIELD_MAPPING_ENHANCEMENTS.md](FIELD_MAPPING_ENHANCEMENTS.md) - Field mapping reference variables integration
- [REFERENCE_VARIABLES_CACHE_IMPLEMENTATION.md](REFERENCE_VARIABLES_CACHE_IMPLEMENTATION.md) - Cache implementation
- [CACHE_TESTING_GUIDE.md](CACHE_TESTING_GUIDE.md) - Testing the variables system
- [PropertiesPanel.js](public/js/pipeline/managers/PropertiesPanel.js) - Main implementation file
- [ReferenceVariablesPanel.js](public/js/pipeline/components/ReferenceVariablesPanel.js) - Variables display component
- [Testoutput.json](C:\Users\ShanawazKhan\Downloads\Testoutput.json) - Runtime structure reference

---

## 🎯 Impact

**Developer Experience:**
- ✅ Complete JavaScript execution environment documented
- ✅ All helper functions clearly explained with examples
- ✅ Real-world risk scoring example ready to copy and modify
- ✅ Clear guidance on accessing HL7 and enriched data

**User Workflow:**
1. Add database/API enrichment steps (if needed)
2. Add script enrichment step
3. Click Step tab to read comprehensive documentation
4. Copy example code and modify for specific needs
5. Click Variables tab to see available data from previous steps
6. Copy XPaths from Variables tab
7. Paste XPaths into script using `getNestedValue()`
8. Click Test Execution to verify logic
9. Review Script Enrichment output in step_outputs
10. Use calculated fields in subsequent field mapping step

**NO-CODE Alignment:**
- Click Variables tab to see available data (no memorization)
- Copy-paste XPaths directly into script code (no manual typing)
- Working example ready to modify (no starting from scratch)
- Test Execution shows immediate feedback (no guesswork)

---

## 🚀 Next Step Recommendation

Based on the user's feedback: **"field mapping is not aligned to No code goal"**

The field mapping step documentation should be enhanced to emphasize NO-CODE features:
- Visual mapping builder (click to add field mappings)
- Auto-complete for HL7 field paths
- Dropdown selection for FHIR paths
- Visual data type picker
- Copy-paste from Variables tab workflow
- Template-based approach (use standard templates, minimal customization)

This would complete the NO-CODE transformation of the entire pipeline builder:
1. ✅ **Database Enrichment** - Click to configure connection, visual query builder
2. ✅ **Script Enrichment** - Copy working example, paste XPaths from Variables tab
3. ⏳ **Field Mapping** - Needs NO-CODE visual builder emphasis

---

**Script enrichment documentation is now complete with enterprise-grade quality and NO-CODE workflow integration!** 🎉
