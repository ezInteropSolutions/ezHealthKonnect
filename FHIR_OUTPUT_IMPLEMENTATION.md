# FHIR Output Configuration - Implementation Guide

**Date**: October 26, 2025
**Status**: READY TO IMPLEMENT
**Priority**: HIGH
**Theme**: Navy Blue (#1e3a8a) + Pastel Pink (#f8bbd9)

---

## Overview

Add FHIR Output configuration as Step 6 in the interface wizard, allowing users to configure where and how FHIR resources are delivered after HL7→FHIR transformation.

---

## Implementation Checklist

### Frontend (UI)
- [ ] Add Step 6 HTML to wizard
- [ ] Update step indicators (1-6 instead of 1-5)
- [ ] Create FHIR destination selector
- [ ] Build resource preset buttons
- [ ] Implement bundle/individual toggle
- [ ] Add test connection button
- [ ] Create advanced settings panel
- [ ] Update navigation logic
- [ ] Add CSS styles matching theme

### Backend (API & DB)
- [ ] Create migration V32 (add fhir_output_config column)
- [ ] Create `/api/fhir/resource-metadata` endpoint
- [ ] Create `/api/fhir/test-connection` endpoint
- [ ] Update wizard save logic
- [ ] Update interface model to include FHIR config

### Testing
- [ ] Test complete wizard flow
- [ ] Test preset selection
- [ ] Test bundle/individual toggle
- [ ] Test connection validation
- [ ] Test configuration save/load

---

## Step 1: Add HTML for Step 6

**File**: `public/interface-wizard.html`

### 1.1 Add Step Indicator in Sidebar

Find the sidebar steps list (around line 813) and add after Step 5:

```html
<!-- Step 6: FHIR Output -->
<li class="step-item" id="stepIndicator6">
    <div class="step-wrapper">
        <div class="step-number" id="stepCircle6">6</div>
        <div class="step-content">
            <div class="step-title">FHIR Output</div>
            <div class="step-subtitle">Destination & Resources</div>
        </div>
    </div>
</li>
```

### 1.2 Add Step 6 Content Panel

Find where step content divs are (after Deploy step, around line 1087) and add:

```html
<!-- Step 6: FHIR Output Configuration -->
<div class="step-content" id="step6" style="display: none;">
    <h3 class="step-title">
        <span class="step-icon">🌐</span>
        FHIR Output Configuration
    </h3>
    <p class="step-description">
        Configure where and how to send transformed FHIR resources
    </p>

    <!-- FHIR Destination Section -->
    <div class="wizard-card">
        <div class="card-header">
            <h4>1. FHIR Destination</h4>
        </div>
        <div class="card-body">
            <div class="form-group">
                <label class="form-label">Destination Type</label>
                <select id="fhirDestinationType" class="form-input">
                    <option value="fhir_http">FHIR Server (HTTP/HTTPS)</option>
                    <option value="epic_fhir">Epic FHIR Endpoint</option>
                    <option value="cerner_fhir">Cerner FHIR Endpoint</option>
                    <option value="azure_fhir">Azure FHIR Service</option>
                    <option value="aws_healthlake">AWS HealthLake</option>
                    <option value="gcp_healthcare">Google Cloud Healthcare API</option>
                    <option value="ezhealth_store" disabled title="Premium feature - Coming soon">
                        ezHealthKonnect FHIR Store (Premium) ⭐
                    </option>
                </select>
            </div>

            <div class="form-row">
                <div class="form-group form-col-4">
                    <label class="form-label">FHIR Version</label>
                    <select id="fhirVersion" class="form-input">
                        <option value="R4" selected>FHIR R4</option>
                        <option value="STU3">FHIR STU3</option>
                        <option value="R5">FHIR R5 (Preview)</option>
                    </select>
                </div>
                <div class="form-group form-col-8">
                    <label class="form-label">Base URL</label>
                    <div class="input-with-button">
                        <input type="url" id="fhirBaseUrl" class="form-input"
                               placeholder="https://fhir.example.com/fhir/r4">
                        <button class="btn-test" id="testConnectionBtn">
                            🔗 Test Connection
                        </button>
                    </div>
                    <div id="connectionStatus" class="status-message"></div>
                </div>
            </div>
        </div>
    </div>

    <!-- Delivery Mode Section -->
    <div class="wizard-card">
        <div class="card-header">
            <h4>2. Delivery Mode</h4>
        </div>
        <div class="card-body">
            <div class="delivery-mode-toggle">
                <button class="mode-option active" data-mode="bundle" id="bundleModeBtn">
                    <div class="mode-icon">📦</div>
                    <div class="mode-content">
                        <div class="mode-title">Bundle</div>
                        <div class="mode-desc">All resources in one transaction</div>
                    </div>
                    <div class="mode-badge badge-success">✅ Recommended</div>
                </button>
                <button class="mode-option" data-mode="individual" id="individualModeBtn">
                    <div class="mode-icon">📄</div>
                    <div class="mode-content">
                        <div class="mode-title">Individual</div>
                        <div class="mode-desc">Separate API call per resource</div>
                    </div>
                    <div class="mode-badge badge-warning">⚠️ Slower</div>
                </button>
            </div>
            <input type="hidden" id="deliveryMode" value="bundle">
        </div>
    </div>

    <!-- Resource Selection Section -->
    <div class="wizard-card">
        <div class="card-header">
            <h4>3. Resource Selection</h4>
        </div>
        <div class="card-body">
            <div class="preset-grid">
                <button class="preset-card active" data-preset="essential">
                    <div class="preset-icon">📋</div>
                    <div class="preset-name">Essential</div>
                    <div class="preset-count">8 resources</div>
                </button>
                <button class="preset-card" data-preset="clinical">
                    <div class="preset-icon">🩺</div>
                    <div class="preset-name">Clinical</div>
                    <div class="preset-count">25 resources</div>
                </button>
                <button class="preset-card" data-preset="comprehensive">
                    <div class="preset-icon">📚</div>
                    <div class="preset-name">Comprehensive</div>
                    <div class="preset-count">50+ resources</div>
                </button>
                <button class="preset-card" data-preset="all">
                    <div class="preset-icon">☑️</div>
                    <div class="preset-name">All</div>
                    <div class="preset-count">150+ resources</div>
                </button>
            </div>

            <div class="resource-summary">
                <strong>Selected:</strong>
                <span id="selectedResourceCount">8</span> resource types
                <button class="btn-link-sm" id="viewResourcesBtn">View All ▼</button>
            </div>

            <div id="selectedResourcesList" class="resource-chips" style="display: none;">
                <!-- Dynamically populated chips -->
            </div>
        </div>
    </div>

    <!-- Advanced Settings (Collapsed) -->
    <div class="wizard-card">
        <button class="advanced-toggle-btn" id="advancedToggleBtn">
            ⚙️ Advanced Settings <span class="toggle-arrow">▼</span>
        </button>

        <div id="advancedSettingsPanel" class="advanced-panel" style="display: none;">
            <div class="form-row">
                <div class="form-group form-col-6">
                    <label class="form-label">Authentication Type</label>
                    <select id="authType" class="form-input">
                        <option value="none">None</option>
                        <option value="bearer_token">Bearer Token</option>
                        <option value="basic_auth">Basic Auth</option>
                        <option value="oauth2">OAuth 2.0</option>
                    </select>
                </div>
                <div class="form-group form-col-6" id="bearerTokenGroup" style="display: none;">
                    <label class="form-label">Bearer Token</label>
                    <input type="password" id="bearerToken" class="form-input"
                           placeholder="Enter token">
                </div>
            </div>

            <div class="form-row">
                <div class="form-group form-col-6">
                    <label class="form-label">Max Retry Attempts</label>
                    <input type="number" id="maxRetries" class="form-input"
                           value="3" min="0" max="10">
                </div>
                <div class="form-group form-col-6">
                    <label class="form-label">Connection Timeout (ms)</label>
                    <input type="number" id="connectionTimeout" class="form-input"
                           value="30000" min="1000" step="1000">
                </div>
            </div>

            <div class="form-checkbox">
                <label>
                    <input type="checkbox" id="validateBeforeSend" checked>
                    Validate resources before sending
                </label>
            </div>
        </div>
    </div>

    <!-- Navigation Buttons -->
    <div class="wizard-nav-buttons">
        <button class="wizard-btn-secondary" id="prevBtn6">
            <span>←</span> Previous
        </button>
        <button class="wizard-btn-primary" id="nextBtn6">
            Next: Review & Deploy <span>→</span>
        </button>
    </div>
</div>
```

---

## Step 2: Add CSS Styles

**File**: `public/interface-wizard.html` (in `<style>` section)

Add these styles following the existing theme:

```css
/* FHIR Output Configuration Styles */

/* Delivery Mode Toggle */
.delivery-mode-toggle {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 16px;
    margin: 16px 0;
}

.mode-option {
    background: white;
    border: 2px solid #e5e7eb;
    border-radius: 12px;
    padding: 20px;
    cursor: pointer;
    transition: all 0.2s;
    display: flex;
    align-items: center;
    gap: 12px;
    position: relative;
}

.mode-option:hover {
    border-color: var(--wizard-navy);
    transform: translateY(-2px);
    box-shadow: 0 4px 12px rgba(30, 58, 138, 0.15);
}

.mode-option.active {
    border-color: var(--wizard-navy);
    background: linear-gradient(135deg, var(--wizard-pink-light) 0%, white 100%);
}

.mode-icon {
    font-size: 32px;
}

.mode-content {
    flex: 1;
}

.mode-title {
    font-size: 16px;
    font-weight: 600;
    color: var(--wizard-navy);
    margin-bottom: 4px;
}

.mode-desc {
    font-size: 13px;
    color: var(--gray-600);
}

.mode-badge {
    position: absolute;
    top: 12px;
    right: 12px;
    padding: 4px 10px;
    border-radius: 12px;
    font-size: 11px;
    font-weight: 600;
}

.badge-success {
    background: #dcfce7;
    color: #166534;
}

.badge-warning {
    background: #fef3c7;
    color: #92400e;
}

/* Preset Grid */
.preset-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: 12px;
    margin: 16px 0;
}

.preset-card {
    background: white;
    border: 2px solid #e5e7eb;
    border-radius: 12px;
    padding: 16px;
    cursor: pointer;
    transition: all 0.2s;
    text-align: center;
}

.preset-card:hover {
    border-color: var(--wizard-navy);
    transform: translateY(-2px);
    box-shadow: 0 4px 12px rgba(30, 58, 138, 0.1);
}

.preset-card.active {
    border-color: var(--wizard-navy);
    background: linear-gradient(135deg, var(--wizard-pink-light) 0%, white 100%);
    border-width: 3px;
}

.preset-icon {
    font-size: 32px;
    margin-bottom: 8px;
}

.preset-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--wizard-navy);
    margin-bottom: 4px;
}

.preset-count {
    font-size: 12px;
    color: var(--gray-600);
}

/* Resource Summary */
.resource-summary {
    margin: 16px 0;
    padding: 12px;
    background: var(--gray-50);
    border-radius: 8px;
    display: flex;
    align-items: center;
    gap: 8px;
}

.resource-summary strong {
    color: var(--wizard-navy);
}

.resource-summary #selectedResourceCount {
    color: var(--wizard-pink);
    font-weight: 700;
    font-size: 16px;
}

/* Resource Chips */
.resource-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 12px;
}

.resource-chip {
    background: var(--wizard-navy);
    color: white;
    padding: 6px 12px;
    border-radius: 16px;
    font-size: 12px;
    font-weight: 500;
}

/* Advanced Settings */
.advanced-toggle-btn {
    background: none;
    border: none;
    color: var(--wizard-navy);
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
    padding: 12px 0;
    width: 100%;
    text-align: left;
    display: flex;
    align-items: center;
    gap: 8px;
}

.advanced-toggle-btn:hover {
    color: var(--navy-secondary);
}

.toggle-arrow {
    margin-left: auto;
    transition: transform 0.3s;
}

.advanced-toggle-btn.expanded .toggle-arrow {
    transform: rotate(180deg);
}

.advanced-panel {
    padding: 16px 0;
    animation: slideDown 0.3s ease-out;
}

@keyframes slideDown {
    from {
        opacity: 0;
        transform: translateY(-10px);
    }
    to {
        opacity: 1;
        transform: translateY(0);
    }
}

/* Input with Button */
.input-with-button {
    display: flex;
    gap: 8px;
}

.input-with-button input {
    flex: 1;
}

.btn-test {
    background: white;
    border: 2px solid var(--wizard-navy);
    color: var(--wizard-navy);
    padding: 8px 16px;
    border-radius: 8px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s;
    white-space: nowrap;
}

.btn-test:hover {
    background: var(--wizard-navy);
    color: white;
}

/* Status Messages */
.status-message {
    margin-top: 8px;
    padding: 8px 12px;
    border-radius: 6px;
    font-size: 13px;
    display: none;
}

.status-message.success {
    background: #dcfce7;
    color: #166534;
    border: 1px solid #86efac;
    display: block;
}

.status-message.error {
    background: #fee2e2;
    color: #991b1b;
    border: 1px solid #fca5a5;
    display: block;
}

.status-message.loading {
    background: #dbeafe;
    color: #1e40af;
    border: 1px solid #93c5fd;
    display: block;
}
```

---

## Step 3: Add JavaScript Logic

**File**: `public/interface-wizard.html` (in `<script>` section)

### 3.1 Update Step Count

Find where `totalSteps` is defined and update:

```javascript
const totalSteps = 6; // Changed from 5 to 6
```

### 3.2 Update Step Titles

Find the `stepTitles` array and add the new step:

```javascript
const stepTitles = [
    'Interface Configuration',
    'Upload HL7 Message',
    'Review Parsed Content',
    'HL7 to FHIR Transformation',
    'FHIR Output',  // NEW
    'Deploy Interface'
];
```

### 3.3 Add Resource Presets

Add this configuration object at the top of the script section:

```javascript
const FHIR_RESOURCE_PRESETS = {
    essential: {
        name: "Essential",
        icon: "📋",
        resources: [
            "Patient",
            "Practitioner",
            "Encounter",
            "Observation",
            "Condition",
            "Procedure",
            "MedicationRequest",
            "MessageHeader"
        ]
    },
    clinical: {
        name: "Clinical",
        icon: "🩺",
        resources: [
            "Patient", "Practitioner", "Encounter", "Observation",
            "Condition", "Procedure", "MedicationRequest", "MessageHeader",
            "DiagnosticReport", "Specimen", "ImagingStudy",
            "AllergyIntolerance", "Immunization", "CarePlan",
            "Goal", "ServiceRequest", "ClinicalImpression",
            "FamilyMemberHistory", "MedicationAdministration",
            "MedicationDispense", "MedicationStatement",
            "DeviceUseStatement", "BodyStructure", "RiskAssessment"
        ]
    },
    comprehensive: {
        name: "Comprehensive",
        icon: "📚",
        resources: [
            // Include essential + clinical + administrative + financial (50+ resources)
            // ... (truncated for brevity, will define full list)
        ]
    },
    all: {
        name: "All Resources",
        icon: "☑️",
        resources: "*"
    }
};
```

### 3.4 Add Event Listeners

Add after existing event listeners:

```javascript
// ===== STEP 6: FHIR OUTPUT CONFIGURATION =====

// Delivery mode toggle
document.getElementById('bundleModeBtn')?.addEventListener('click', function() {
    document.getElementById('individualModeBtn').classList.remove('active');
    this.classList.add('active');
    document.getElementById('deliveryMode').value = 'bundle';
});

document.getElementById('individualModeBtn')?.addEventListener('click', function() {
    document.getElementById('bundleModeBtn').classList.remove('active');
    this.classList.add('active');
    document.getElementById('deliveryMode').value = 'individual';
});

// Resource preset selection
document.querySelectorAll('.preset-card').forEach(btn => {
    btn.addEventListener('click', function() {
        document.querySelectorAll('.preset-card').forEach(b => b.classList.remove('active'));
        this.classList.add('active');

        const preset = this.dataset.preset;
        updateResourceSelection(preset);
    });
});

// Test connection button
document.getElementById('testConnectionBtn')?.addEventListener('click', async function() {
    const baseUrl = document.getElementById('fhirBaseUrl').value;
    const statusEl = document.getElementById('connectionStatus');

    if (!baseUrl) {
        showStatus(statusEl, 'error', 'Please enter a FHIR base URL');
        return;
    }

    showStatus(statusEl, 'loading', 'Testing connection...');

    try {
        const response = await fetch('/api/fhir/test-connection', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                base_url: baseUrl,
                fhir_version: document.getElementById('fhirVersion').value
            })
        });

        const result = await response.json();

        if (result.success) {
            showStatus(statusEl, 'success',
                `✅ Connected to ${result.server_info?.software || 'FHIR Server'} ` +
                `(FHIR ${result.server_info?.fhir_version || 'R4'})`
            );
        } else {
            showStatus(statusEl, 'error', `❌ Connection failed: ${result.error}`);
        }
    } catch (error) {
        showStatus(statusEl, 'error', `❌ Connection failed: ${error.message}`);
    }
});

// Advanced settings toggle
document.getElementById('advancedToggleBtn')?.addEventListener('click', function() {
    const panel = document.getElementById('advancedSettingsPanel');
    const arrow = this.querySelector('.toggle-arrow');

    if (panel.style.display === 'none') {
        panel.style.display = 'block';
        this.classList.add('expanded');
    } else {
        panel.style.display = 'none';
        this.classList.remove('expanded');
    }
});

// Auth type change
document.getElementById('authType')?.addEventListener('change', function() {
    const tokenGroup = document.getElementById('bearerTokenGroup');
    tokenGroup.style.display = this.value === 'bearer_token' ? 'block' : 'none';
});

// View resources button
document.getElementById('viewResourcesBtn')?.addEventListener('click', function() {
    const list = document.getElementById('selectedResourcesList');
    if (list.style.display === 'none') {
        list.style.display = 'flex';
        this.textContent = 'Hide ▲';
    } else {
        list.style.display = 'none';
        this.textContent = 'View All ▼';
    }
});

// Navigation buttons
document.getElementById('prevBtn6')?.addEventListener('click', () => navigateStep(5));
document.getElementById('nextBtn6')?.addEventListener('click', () => navigateStep(7)); // Changed to 7

// Helper Functions
function updateResourceSelection(preset) {
    const resources = FHIR_RESOURCE_PRESETS[preset].resources;
    const count = resources === '*' ? '150+' : resources.length;

    document.getElementById('selectedResourceCount').textContent = count;

    // Update chips
    const chipContainer = document.getElementById('selectedResourcesList');
    chipContainer.innerHTML = '';

    if (resources !== '*') {
        resources.forEach(resource => {
            const chip = document.createElement('span');
            chip.className = 'resource-chip';
            chip.textContent = resource;
            chipContainer.appendChild(chip);
        });
    } else {
        const chip = document.createElement('span');
        chip.className = 'resource-chip';
        chip.textContent = 'All FHIR R4 Resources';
        chipContainer.appendChild(chip);
    }
}

function showStatus(element, type, message) {
    element.className = `status-message ${type}`;
    element.textContent = message;
}

// Initialize default selection
updateResourceSelection('essential');
```

---

## Step 4: Database Migration

**File**: `database/migrations/V32__Add_FHIR_Output_Config.sql`

```sql
-- V32: Add FHIR Output Configuration to Interfaces

-- Add FHIR output configuration column
ALTER TABLE interfaces
ADD COLUMN IF NOT EXISTS fhir_output_config JSONB DEFAULT '{}'::jsonb;

-- Add index for FHIR config queries
CREATE INDEX IF NOT EXISTS idx_interfaces_fhir_output
  ON interfaces USING GIN (fhir_output_config);

-- Add comments
COMMENT ON COLUMN interfaces.fhir_output_config IS
  'FHIR output destination and resource configuration (JSONB). Structure: {
    enabled: boolean,
    destination_type: string,
    fhir_version: string,
    base_url: string,
    delivery_mode: string,
    resource_selection: {preset: string, custom_resources: string[]},
    authentication: {type: string, ...},
    retry_config: {max_attempts: int, ...}
  }';
```

---

## Step 5: Backend API Endpoints

### 5.1 Test Connection Endpoint

**File**: `routes/fhirRoutes.js` (NEW)

```javascript
const express = require('express');
const router = express.Router();
const fetch = require('node-fetch');

// Test FHIR connection
router.post('/test-connection', async (req, res) => {
    const { base_url, fhir_version } = req.body;

    if (!base_url) {
        return res.status(400).json({
            success: false,
            error: 'Base URL is required'
        });
    }

    try {
        // Try to fetch CapabilityStatement
        const metadataUrl = `${base_url.replace(/\/$/, '')}/metadata`;
        const response = await fetch(metadataUrl, {
            method: 'GET',
            headers: {
                'Accept': 'application/fhir+json'
            },
            timeout: 10000
        });

        if (!response.ok) {
            return res.json({
                success: false,
                error: `Server returned ${response.status}: ${response.statusText}`
            });
        }

        const capability = await response.json();

        res.json({
            success: true,
            server_info: {
                software: capability.software?.name || 'Unknown',
                version: capability.software?.version || 'Unknown',
                fhir_version: capability.fhirVersion || fhir_version
            }
        });
    } catch (error) {
        res.json({
            success: false,
            error: error.message
        });
    }
});

// Get resource metadata (categories and presets)
router.get('/resource-metadata', (req, res) => {
    res.json({
        presets: {
            essential: {
                name: "Essential",
                icon: "📋",
                count: 8,
                resources: [
                    "Patient", "Practitioner", "Encounter", "Observation",
                    "Condition", "Procedure", "MedicationRequest", "MessageHeader"
                ]
            },
            clinical: {
                name: "Clinical",
                icon: "🩺",
                count: 25,
                resources: [/* ... */]
            },
            comprehensive: {
                name: "Comprehensive",
                icon: "📚",
                count: 50,
                resources: [/* ... */]
            },
            all: {
                name: "All Resources",
                icon: "☑️",
                count: 150,
                resources: "*"
            }
        },
        categories: [
            {
                id: "core",
                name: "Core Resources",
                icon: "👤",
                resources: ["Patient", "Practitioner", "Organization", "Location"]
            },
            {
                id: "clinical",
                name: "Clinical Resources",
                icon: "🩺",
                resources: ["Observation", "Condition", "Procedure", "DiagnosticReport"]
            }
            // ... more categories
        ]
    });
});

module.exports = router;
```

### 5.2 Register Routes

**File**: `app.js`

Add after other route registrations:

```javascript
const fhirRoutes = require('./routes/fhirRoutes');
app.use('/api/fhir', fhirRoutes);
```

---

## Testing Plan

1. **Visual Test**: Open wizard, navigate to Step 6, verify layout matches theme
2. **Preset Test**: Click each preset, verify resource count updates
3. **Toggle Test**: Switch between bundle/individual, verify state changes
4. **Connection Test**: Enter FHIR URL, click test, verify success/failure message
5. **Advanced Test**: Expand advanced settings, verify auth type shows/hides token field
6. **Save Test**: Complete wizard, verify FHIR config saves to database
7. **Load Test**: Edit interface, verify FHIR config loads correctly

---

**Next Session**: Begin implementation with Step 1 (Add HTML)
