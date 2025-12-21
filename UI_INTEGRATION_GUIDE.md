# UI Integration Guide - API Endpoint Tester

**Purpose**: Add "Test Endpoint" button to API Enrichment step configuration
**File to Modify**: `public/js/pipeline/managers/PropertiesPanel.js`
**Status**: Ready for Integration

---

## What You'll See After Integration

When you click on an API Enrichment step in the Pipeline Builder, you'll see:

```
┌────────────────────────────────────────────────┐
│ API Enrichment Configuration                   │
├────────────────────────────────────────────────┤
│ Endpoint URL: [https://api.example.com/{id}]   │
│ Method: [GET ▼]                                │
│ Auth Type: [Bearer ▼]                          │
│ ... other fields ...                           │
├────────────────────────────────────────────────┤
│ 🧪 Test API Endpoint                            │
│                                                │
│ Sample Message Data (JSON):                    │
│ ┌────────────────────────────────────────────┐ │
│ │ {                                          │ │
│ │   "PID.3": "12345"                         │ │
│ │ }                                          │ │
│ └────────────────────────────────────────────┘ │
│                                                │
│ [🧪 Test API Endpoint]  ← NEW BUTTON          │
│                                                │
│ ✅ API Call Successful                         │
│ Status: 200 OK (333ms)                         │
│                                                │
│ 📋 Response Fields (Click to Add):             │
│ ┌────────────────────────────────────────────┐ │
│ │ $.patient.id        [string]    + Add      │ │
│ │ $.patient.name      [string]    + Add      │ │
│ └────────────────────────────────────────────┘ │
└────────────────────────────────────────────────┘
```

---

## Integration Steps

### **Step 1: Find the API Enrichment Configuration Section**

In `PropertiesPanel.js`, search for where the API enrichment step form is rendered. This is typically in a method that builds the HTML form for step configuration.

Look for code that renders:
- Endpoint URL input
- Method dropdown
- Auth Type selector
- Headers/Query Params builders

### **Step 2: Add the API Endpoint Tester Container**

Add this HTML **after** the main configuration fields but **before** the save/cancel buttons:

```html
<!-- API Endpoint Tester Section -->
<div class="config-section">
    <div id="api-endpoint-tester-container"></div>
</div>
```

### **Step 3: Initialize the Component**

Add this JavaScript code after the form is rendered:

```javascript
// Initialize API Endpoint Tester for API Enrichment steps
if (step.step_type === 'pre.enrichment.api') {
    // Create tester instance
    const tester = new APIEndpointTester('api-endpoint-tester-container');

    // Set callback for when user adds a field
    tester.setOnAddMappingRule((ruleData) => {
        console.log('User added field:', ruleData);
        // ruleData = {
        //   sourcePath: "$.patient.id",
        //   targetField: "patientId",
        //   transformType: "none",
        //   fieldType: "string"
        // }

        // Add to response mapping configuration
        addResponseMappingRule(ruleData);
    });

    // Render with current step configuration
    const getCurrentStepConfig = () => {
        return {
            endpoint: document.getElementById('step-endpoint')?.value || '',
            method: document.getElementById('step-method')?.value || 'GET',
            authType: document.getElementById('step-auth-type')?.value || 'none',
            bearerToken: document.getElementById('step-bearer-token')?.value || '',
            headers: headerBuilder?.getHeaders() || {},
            queryParams: queryParamBuilder?.getQueryParams() || {},
            fieldMappings: fieldMappingsBuilder?.getFieldMappings() || {}
        };
    };

    tester.render(getCurrentStepConfig());
}
```

### **Step 4: Add Response Mapping Rules Function**

Add this helper function to handle adding fields from the picker:

```javascript
function addResponseMappingRule(ruleData) {
    // Get or create response mapping config
    if (!step.config.responseMapping) {
        step.config.responseMapping = {
            mode: 'custom',
            extractors: []
        };
    }

    // Add the new extractor rule
    step.config.responseMapping.extractors.push({
        sourcePath: ruleData.sourcePath,
        targetField: ruleData.targetField,
        transformType: ruleData.transformType || 'none',
        required: false
    });

    // Re-render response mapping section to show new rule
    renderResponseMappingRules();

    // Show success message
    showToast('✓ Field added to response mapping: ' + ruleData.targetField, 'success');
}
```

---

## Quick Integration Example

Here's a minimal example you can copy/paste into the appropriate section:

```javascript
// In the method that renders API Enrichment step config form:

// ... existing form fields ...

// Add API Endpoint Tester
const testerHTML = `
    <div class="config-section">
        <div id="api-endpoint-tester-container"></div>
    </div>
`;

// Append to form
document.getElementById('step-config-form').insertAdjacentHTML('beforeend', testerHTML);

// Initialize tester
setTimeout(() => {
    const tester = new APIEndpointTester('api-endpoint-tester-container');
    tester.setOnAddMappingRule((ruleData) => {
        if (!step.config.responseMapping) {
            step.config.responseMapping = { mode: 'custom', extractors: [] };
        }
        step.config.responseMapping.extractors.push({
            sourcePath: ruleData.sourcePath,
            targetField: ruleData.targetField,
            transformType: 'none'
        });
        console.log('Added field:', ruleData.targetField);
    });

    tester.render({
        endpoint: document.getElementById('step-endpoint').value,
        method: document.getElementById('step-method').value,
        authType: document.getElementById('step-auth-type')?.value || 'none'
    });
}, 100);
```

---

## Alternative: Simple Button Approach

If you want to start with just the "Test Endpoint" button without the full component:

```html
<!-- Add this button after the endpoint configuration -->
<button id="test-api-endpoint-btn" class="btn btn-secondary" style="margin-top: 10px;">
    🧪 Test API Endpoint
</button>
<div id="test-results" style="display:none; margin-top: 10px;"></div>

<script>
document.getElementById('test-api-endpoint-btn').addEventListener('click', async () => {
    const stepConfig = {
        endpoint: document.getElementById('step-endpoint').value,
        method: document.getElementById('step-method').value || 'GET',
        authType: document.getElementById('step-auth-type')?.value || 'none',
        bearerToken: document.getElementById('step-bearer-token')?.value || ''
    };

    const response = await fetch('/api/fhir/pipeline/test-api-endpoint', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ stepConfig, testData: {} })
    });

    const result = await response.json();
    document.getElementById('test-results').style.display = 'block';
    document.getElementById('test-results').innerHTML = `
        <pre>${JSON.stringify(result, null, 2)}</pre>
    `;
});
</script>
```

---

## Testing the Integration

After integrating, test it by:

1. Open Pipeline Builder
2. Create new pipeline
3. Drag "API Enrichment" step to canvas
4. Click on the step to open properties panel
5. **You should see**: "Test API Endpoint" section
6. Configure endpoint: `https://jsonplaceholder.typicode.com/users/1`
7. Click "Test API Endpoint" button
8. **You should see**: Response with field picker
9. Click a field (e.g., `$.id`)
10. **You should see**: Field added to response mapping config

---

## File Locations

- **Component**: `public/js/pipeline/components/APIEndpointTester.js` ✅ Created
- **CSS**: `public/css/api-endpoint-tester.css` ✅ Created
- **Backend API**: `/api/fhir/pipeline/test-api-endpoint` ✅ Implemented
- **Integration Target**: `public/js/pipeline/managers/PropertiesPanel.js` ⏳ Needs modification

---

## Need Help?

If you need help finding the exact location in PropertiesPanel.js:

1. Search for: `'pre.enrichment.api'`
2. Find the function that renders the form (usually has HTML template strings)
3. Look for where endpoint, method, auth type inputs are created
4. Add the tester container div after those inputs
5. Initialize the component after the form is rendered

---

## What Happens After Integration

**User clicks API Enrichment step** →
**Properties panel opens with config form** →
**User sees "Test API Endpoint" section** →
**User configures endpoint + auth** →
**User clicks "Test API Endpoint" button** →
**Backend makes actual API call** →
**Response shown with field picker** →
**User clicks fields to add to mapping** →
**Response mapping config auto-generated** →
**User saves step with mapping configured**

**Result**: Configuration time reduced from 30+ minutes to 2-3 minutes! 🎉

---

**Status**: Component ready, awaiting UI integration in PropertiesPanel.js
