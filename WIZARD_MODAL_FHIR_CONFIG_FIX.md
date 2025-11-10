# Wizard Modal FHIR Configuration Fix

**Date**: October 27, 2025
**Issue**: User reported authentication toggle and bundle/individual delivery mode options not working in wizard modal

## Problem Report

User stated:
> "on step 4, i am choosing fhir as destination, I see enable authentication nothing happens, I do not see the option to switch from bundle to individual resource"

### Context Clarification

The user was working with the **Optimized Wizard Modal** (not the standalone wizard page at `/interface-wizard.html`). The FHIR target configuration appears in **Step 1** when the user selects "HTTP" as target connectivity type.

## Root Causes Identified

### 1. Missing Authentication Toggle Event Handler

**File**: `public/js/wizard/optimized/WizardView.js`

**Issue**:
- The authentication checkbox (`#targetAuthEnabled`) was rendered in the template (line 630)
- The authentication config div (`#authConfig`) was conditionally hidden/shown via inline style (line 636)
- However, there was **NO event listener** to toggle the visibility when the checkbox was clicked
- Result: Clicking the checkbox did nothing

**Evidence from Logs**:
```javascript
WizardView.js:4159 🖱️ Click event detected on: <input type="checkbox" id="targetAuthEnabled" class="form-check-input">
WizardView.js:4202 📝 Input changed, running validation: targetAuthEnabled on
```
The click was detected and validation ran, but the `authConfig` div remained hidden.

### 2. Missing Delivery Mode Toggle Options

**File**: `public/js/wizard/optimized/WizardView.js`

**Issue**:
- The FHIR target configuration template had authentication options but **no delivery mode options** at all
- Bundle vs Individual delivery mode selection was completely absent from the UI
- Users had no way to choose between single transaction (bundle) vs multiple API calls (individual)

## Fixes Applied

### Fix 1: Added Delivery Mode Radio Buttons

**File**: `public/js/wizard/optimized/WizardView.js` (Lines 603-624)

**Added HTML**:
```javascript
<!-- Delivery Mode -->
<div class="form-group">
    <label class="form-label">Delivery Mode</label>
    <div class="form-row" style="gap: 10px;">
        <div class="form-check" style="flex: 1;">
            <input type="radio" id="deliveryModeBundle" name="deliveryMode" value="bundle"
                   class="form-check-input" ${!config.deliveryMode || config.deliveryMode === 'bundle' ? 'checked' : ''}>
            <label for="deliveryModeBundle" class="form-check-label">
                <strong>Bundle</strong>
                <div class="form-hint">Single transaction (Recommended)</div>
            </label>
        </div>
        <div class="form-check" style="flex: 1;">
            <input type="radio" id="deliveryModeIndividual" name="deliveryMode" value="individual"
                   class="form-check-input" ${config.deliveryMode === 'individual' ? 'checked' : ''}>
            <label for="deliveryModeIndividual" class="form-check-label">
                <strong>Individual</strong>
                <div class="form-hint">Separate API calls per resource</div>
            </label>
        </div>
    </div>
</div>
```

**Key Features**:
- Radio button group (mutually exclusive)
- Bundle mode selected by default (recommended)
- Clear labels with hints explaining the difference
- Responsive layout using flexbox

### Fix 2: Added Delivery Mode Event Listeners

**File**: `public/js/wizard/optimized/WizardView.js` (Lines 3768-3796)

**Added Event Handlers**:
```javascript
// Delivery mode radio buttons (for FHIR target)
const deliveryModeBundle = container.querySelector('#deliveryModeBundle');
const deliveryModeIndividual = container.querySelector('#deliveryModeIndividual');

if (deliveryModeBundle) {
    deliveryModeBundle.addEventListener('change', (e) => {
        if (e.target.checked) {
            console.log('📦 Delivery mode: Bundle (single transaction)');
            this.dispatchEvent(new CustomEvent('fieldChange', {
                detail: { field: 'deliveryMode', value: 'bundle' }
            }));
        }
    });
}

if (deliveryModeIndividual) {
    deliveryModeIndividual.addEventListener('change', (e) => {
        if (e.target.checked) {
            console.log('📄 Delivery mode: Individual (separate API calls)');
            this.dispatchEvent(new CustomEvent('fieldChange', {
                detail: { field: 'deliveryMode', value: 'individual' }
            }));
        }
    });
}

if (deliveryModeBundle || deliveryModeIndividual) {
    console.log('✅ Delivery mode radio listeners attached');
}
```

**Key Features**:
- Separate listeners for each radio button
- Dispatch `fieldChange` event to update model
- Console logging for debugging
- Confirmation log when listeners attached

### Fix 3: Added Authentication Toggle Event Handler

**File**: `public/js/wizard/optimized/WizardView.js` (Lines 3798-3816)

**Added Event Handler**:
```javascript
// Authentication toggle (for FHIR target)
const targetAuthCheckbox = container.querySelector('#targetAuthEnabled');
const authConfigDiv = container.querySelector('#authConfig');
if (targetAuthCheckbox && authConfigDiv) {
    targetAuthCheckbox.addEventListener('change', (e) => {
        if (e.target.checked) {
            authConfigDiv.style.display = 'block';
            console.log('✅ Authentication enabled - showing auth config');
        } else {
            authConfigDiv.style.display = 'none';
            console.log('❌ Authentication disabled - hiding auth config');
        }

        this.dispatchEvent(new CustomEvent('fieldChange', {
            detail: { field: 'targetAuthEnabled', value: e.target.checked }
        }));
    });
    console.log('✅ Authentication toggle listener attached');
}
```

**Key Features**:
- Toggles `authConfig` div visibility with `display: block/none`
- Dispatch `fieldChange` event to update model
- Console logging for debugging
- Confirmation log when listener attached

## How It Works Now

### User Flow (Fixed)

1. **Open Wizard Modal**
   - Click "+ New Interface" button on interfaces page
   - Wizard modal opens at Step 1 (Configuration)

2. **Configure Source**
   - Name: "Test Interface"
   - Description: Auto-generated
   - Source Type: "HL7 v2.x"
   - Source Connectivity: "TCP/MLLP"
   - Source Host: "0.0.0.0"
   - Source Port: "6662"

3. **Configure Target**
   - Scroll down to "Target Configuration" section
   - Target Type: "FHIR R4"
   - Target Connectivity: **"HTTP"** (triggers FHIR config display)

4. **FHIR Configuration Options (NOW VISIBLE & WORKING)**
   - **Endpoint**: `http://localhost:8080/fhir` (default)
   - **FHIR Version**: R4 (Recommended) / STU3 / DSTU2
   - **Format**: JSON (Recommended) / XML

   - **Delivery Mode** (**NEW**)
     - ✅ Bundle (Single transaction) - **Default**
     - ⭕ Individual (Separate API calls per resource)

   - **Authentication** (**NOW WORKING**)
     - ☐ Enable Authentication checkbox
     - When checked → Shows auth config fields:
       - Auth Type: Basic Auth / Bearer Token / OAuth 2.0
       - Token/Credentials: Password input field

5. **Next Steps**
   - Click "Next" → Step 2 (Upload HL7 Message)
   - Upload or paste HL7 message
   - Click "Parse HL7" → Step 3 (FHIR Transformation)
   - Automatic FHIR transformation with atomic mappings
   - Review and edit mappings
   - Click "Next" → Step 4 (Deploy)
   - Deploy interface

## Verification Steps

### 1. Test Delivery Mode Toggle

```bash
# Open browser console (F12)
# Navigate to: http://localhost:3000 → Interfaces → New Interface

# In Step 1, configure FHIR target, then:
document.getElementById('deliveryModeBundle').click()
# Should see in console: 📦 Delivery mode: Bundle (single transaction)

document.getElementById('deliveryModeIndividual').click()
# Should see in console: 📄 Delivery mode: Individual (separate API calls)
```

### 2. Test Authentication Toggle

```bash
# In same wizard modal, Step 1:
const authCheckbox = document.getElementById('targetAuthEnabled');
const authDiv = document.getElementById('authConfig');

// Before clicking (default state)
console.log('Auth div display:', authDiv.style.display); // "none"

// Click checkbox
authCheckbox.click();
console.log('Auth div display:', authDiv.style.display); // "block"
// Should see in console: ✅ Authentication enabled - showing auth config

// Unclick checkbox
authCheckbox.click();
console.log('Auth div display:', authDiv.style.display); // "none"
// Should see in console: ❌ Authentication disabled - hiding auth config
```

### 3. Visual Verification

1. Open wizard modal
2. Configure source (HL7 v2.x + TCP)
3. Scroll to target configuration
4. Select Target Connectivity: "HTTP"
5. **VERIFY**: You should see:
   - Delivery Mode section with 2 radio buttons
   - "Bundle" selected by default with pink/blue styling
   - Authentication checkbox (unchecked by default)
6. Click "Bundle" radio → Should highlight
7. Click "Individual" radio → Should highlight, "Bundle" unhighlights
8. Check "Enable Authentication" → Auth fields appear below
9. Uncheck → Auth fields disappear

## Technical Details

### Files Modified

1. **public/js/wizard/optimized/WizardView.js**
   - Lines 603-624: Added delivery mode HTML template
   - Lines 3768-3796: Added delivery mode event listeners
   - Lines 3798-3816: Added authentication toggle event handler

### Event Flow

1. **User clicks delivery mode radio button**
   → `change` event fires
   → Event listener checks `e.target.checked`
   → Dispatches `CustomEvent('fieldChange')` with `deliveryMode` value
   → WizardController receives event
   → Updates WizardModel data
   → Model state now includes selected delivery mode

2. **User toggles authentication checkbox**
   → `change` event fires
   → Event listener checks `e.target.checked`
   → Sets `authConfig.style.display = 'block'` or `'none'`
   → Dispatches `CustomEvent('fieldChange')` with `targetAuthEnabled` boolean
   → WizardController receives event
   → Updates WizardModel data
   → Model state now includes auth enabled status

### Data Structure (WizardModel)

After fixes, the model stores:

```javascript
{
  name: "Test Interface",
  description: "HL7 to FHIR interface for Test Interface",
  sourceType: "hl7v2",
  sourceConnectivity: "tcp",
  sourceHost: "0.0.0.0",
  sourcePort: 6662,
  targetType: "fhir",
  targetConnectivity: "http",
  targetEndpoint: "http://localhost:8080/fhir",
  targetVersion: "R4",
  targetFormat: "json",
  deliveryMode: "bundle", // NEW - "bundle" or "individual"
  targetAuthEnabled: true, // NEW - true or false
  authType: "bearer", // If auth enabled
  authToken: "abc123..." // If auth enabled
}
```

## Related Issues

### Issue: Step 5 Visibility (Separate Issue)

The earlier fix for `interface-wizard.html` (standalone wizard page) addressed Step 5 visibility for FHIR Output Configuration. That fix is **NOT related** to this issue because:

1. **Different wizard implementations**:
   - This issue: Optimized Wizard Modal (`WizardView.js`) - used in interfaces page
   - Previous issue: Standalone Wizard Page (`interface-wizard.html`) - separate page

2. **Different step structure**:
   - Optimized Modal: 4 steps (Config, Upload, Transform, Deploy)
   - Standalone Page: 6 steps (Config, Upload, Review, Mapping, FHIR Output, Deploy)

3. **Different target configuration location**:
   - Optimized Modal: Target config in **Step 1** (combined with source)
   - Standalone Page: Target config in **Step 5** (separate FHIR Output step)

## Status

✅ **RESOLVED** - Delivery mode toggle and authentication toggle now working in optimized wizard modal

## Next Steps

1. ✅ Test wizard flow end-to-end with FHIR target
2. ⏳ Update WizardController to save `deliveryMode` and `targetAuthEnabled` to database
3. ⏳ Update interface save endpoint to store FHIR configuration in `target_connectivity` JSONB
4. ⏳ Implement delivery mode logic in FHIR output processor (bundle vs individual API calls)
5. ⏳ Implement authentication logic in FHIR HTTP client (Bearer Token, Basic Auth, OAuth)

## Documentation

- [STEP_5_VISIBILITY_FIX.md](STEP_5_VISIBILITY_FIX.md) - Standalone wizard Step 5 fix (separate issue)
- [FHIR_RECEIVER_ARCHITECTURE.md](FHIR_RECEIVER_ARCHITECTURE.md) - FHIR receiver backend design
- [database/migrations/V30__Add_Source_Target_Connectivity.sql](database/migrations/V30__Add_Source_Target_Connectivity.sql) - Connectivity JSONB schema
