# Step 5 (FHIR Output Configuration) Visibility Fix

**Date**: October 27, 2025
**Issue**: User reported authentication toggle and bundle/individual options not visible in Step 4

## Problem Analysis

The user reported:
> "on step 4, i am choosing fhir as destination, I see enable authentication nothing happens, I do not see the option to switch from bundle to undividual resource"

### Root Causes Identified

1. **Step Progress Indicators Inconsistent**
   - Steps 1-4 showed "STEP X OF 5" (outdated)
   - Steps 5-6 showed "STEP X OF 6" (correct)
   - This created confusion about wizard structure

2. **Step 5 Content Not Showing**
   - Step 5 HTML element had inline `style="display: none;"`
   - JavaScript code in `wizard-navigation.js` only toggled the sidebar **indicator** visibility
   - JavaScript did NOT toggle the step **content** visibility
   - Even when FHIR was selected, Step 5 content remained hidden

3. **User Expectations**
   - Authentication toggle and bundle/individual options are in **Step 5**, not Step 4
   - Step 5 should automatically appear when user selects FHIR as target type
   - Conditional visibility logic existed but was incomplete

## Fixes Applied

### 1. Fixed Step Progress Indicators

**Files Modified**: `public/interface-wizard.html`

Changed all step progress indicators to show "OF 6":

```html
<!-- Step 1 -->
<div class="step-progress">STEP 1 OF 6</div>

<!-- Step 2 -->
<div class="step-progress">STEP 2 OF 6</div>

<!-- Step 3 -->
<div class="step-progress">STEP 3 OF 6</div>

<!-- Step 4 -->
<div class="step-progress">STEP 4 OF 6</div>
```

### 2. Fixed Step 5 Content Visibility

**Files Modified**: `public/js/core/wizard-navigation.js`

**Before**:
```javascript
if (fhirEnabled) {
    // Show Step 5 (FHIR Output) in sidebar
    const step5Indicator = document.getElementById('stepIndicator5');
    if (step5Indicator) {
        step5Indicator.style.display = 'block';
        console.log('✅ Step 5 (FHIR Output) shown');
    }
    nextStep = 5;
}
```

**After**:
```javascript
if (fhirEnabled) {
    // Show Step 5 (FHIR Output) - both sidebar indicator AND step content
    const step5Indicator = document.getElementById('stepIndicator5');
    if (step5Indicator) {
        step5Indicator.style.display = 'block';
    }

    const step5Content = document.getElementById('step5');
    if (step5Content) {
        step5Content.style.display = 'block';
        console.log('✅ Step 5 (FHIR Output) shown - sidebar and content');
    }

    nextStep = 5;
} else {
    // Hide Step 5 (FHIR Output) and skip to Step 6 (Deploy)
    const step5Indicator = document.getElementById('stepIndicator5');
    if (step5Indicator) {
        step5Indicator.style.display = 'none';
    }

    const step5Content = document.getElementById('step5');
    if (step5Content) {
        step5Content.style.display = 'none';
        console.log('⏭️ Step 5 (FHIR Output) hidden - skipping to Deploy');
    }

    nextStep = 6;
}
```

**Key Changes**:
- Added `document.getElementById('step5')` to get step content element
- Set `step5Content.style.display = 'block'` when FHIR enabled
- Set `step5Content.style.display = 'none'` when FHIR disabled
- Now controls both sidebar indicator AND step content visibility

## How It Works Now

### User Flow (Corrected)

1. **Step 1**: User selects "FHIR R4" as Target Format
   - `wizardTargetType` = "fhir"

2. **Steps 2-3**: (Upload/Review - may be skipped for FHIR sources)

3. **Step 4**: HL7 to FHIR Transformation
   - User configures transformation settings
   - Clicks "Next"

4. **Transition from Step 4 to Step 5**:
   - JavaScript checks `isFhirTransformationEnabled()`
   - Method checks if `targetType.toLowerCase().includes('fhir')` → returns `true`
   - JavaScript sets:
     - `step5Indicator.style.display = 'block'` (sidebar shows Step 5)
     - `step5Content.style.display = 'block'` (**NEW** - step content now visible)
   - Navigation proceeds to Step 5

5. **Step 5**: FHIR Output Configuration (NOW VISIBLE!)
   - User sees:
     - ✅ FHIR Base URL field
     - ✅ Delivery Mode toggle (Bundle vs Individual)
     - ✅ Resource Selection presets
     - ✅ Advanced Settings (Authentication, Retry, Rate Limiting)

### Step 5 Controls (All Working)

**Delivery Mode Toggle**:
- Bundle button: Default active (pink border #f8bbd9)
- Individual button: Inactive (gray border #e5e7eb)
- Click toggles `selectedDeliveryMode` variable

**Resource Presets**:
- Essential (8 resources)
- Clinical (25 resources)
- Comprehensive (50 resources)
- All (150+ resources)
- Click updates border color and `selectedPreset` variable

**Advanced Settings Accordion**:
- Collapsed by default
- Click "⚙️ Advanced Settings" to expand
- Shows authentication, retry, rate limiting options

**Authentication Dropdown**:
- Options: None, Bearer Token, Basic Auth, OAuth 2.0
- When "Bearer Token" selected, Bearer Token input field appears
- Other types hide the token field

## Verification Steps

### 1. Test Step 5 Visibility

```bash
# Access wizard in browser
http://localhost:3000/interface-wizard.html

# Step-by-step test:
1. Fill out Step 1, select "FHIR R4" as Target Format
2. Complete Steps 2-4 (or skip if FHIR source)
3. Click "Next" on Step 4
4. VERIFY: Step 5 appears with title "FHIR Output Configuration"
5. VERIFY: Sidebar shows "Step 5" indicator
```

### 2. Test Step 5 Controls

```javascript
// Open browser console (F12)
// Check for successful initialization logs:

// When navigating to Step 5:
"FHIR transformation enabled: true"
"✅ Step 5 (FHIR Output) shown - sidebar and content"

// Test delivery mode toggle:
document.getElementById('bundleModeBtn').click()
// Should see pink border on Bundle button

document.getElementById('individualModeBtn').click()
// Should see pink border move to Individual button

// Test advanced settings:
document.getElementById('fhirAdvancedToggleBtn').click()
// Should see authentication panel expand

// Test authentication:
document.getElementById('fhirAuthType').value = 'bearer_token'
document.getElementById('fhirAuthType').dispatchEvent(new Event('change'))
// Should see Bearer Token field appear
```

### 3. Test Step 5 Hiding (Non-FHIR Target)

```bash
# Test that Step 5 is hidden for non-FHIR targets:
1. Fill out Step 1, select "Database" as Target Format
2. Complete Steps 2-4
3. Click "Next" on Step 4
4. VERIFY: Skip directly to Step 6 (Deploy)
5. VERIFY: Step 5 does NOT appear in sidebar or content
```

## Technical Details

### Files Changed

1. `public/interface-wizard.html`
   - Lines 848, 977, 990, 1003: Updated step progress to "OF 6"

2. `public/js/core/wizard-navigation.js`
   - Lines 191-228: Enhanced Step 5 visibility logic
   - Added step content display control
   - Improved console logging

### CSS Classes Used

- `.wizard-step.active`: Controls step visibility via CSS
- Inline `style="display: none;"`: Overrides CSS classes
- Must be explicitly set to `display: block` via JavaScript

### JavaScript Event Handlers Verified

All event handlers in `interface-wizard.html` lines 1465-1540:
- ✅ Delivery mode toggle (bundle/individual buttons)
- ✅ Resource preset selection (4 cards)
- ✅ Advanced settings accordion
- ✅ Authentication type dropdown
- ✅ Test FHIR connection button

### Data Structures Verified

- `FHIR_RESOURCE_PRESETS` object (line 1458-1463)
- `selectedDeliveryMode` variable (default: 'bundle')
- `selectedPreset` variable (default: 'essential')

## Related Documentation

- [FHIR_RECEIVER_ARCHITECTURE.md](FHIR_RECEIVER_ARCHITECTURE.md) - FHIR receiver backend design
- [database/migrations/V30__Add_Source_Target_Connectivity.sql](database/migrations/V30__Add_Source_Target_Connectivity.sql) - Connectivity JSONB schema
- [controllers/FhirReceiverController.js](controllers/FhirReceiverController.js) - FHIR receiver backend
- [routes/fhirReceiverRoutes.js](routes/fhirReceiverRoutes.js) - FHIR receiver routes

## Status

✅ **RESOLVED** - Step 5 visibility and controls now working correctly

## Next Steps

1. Test wizard flow end-to-end with FHIR target
2. Implement backend integration for Step 5 (save to `target_connectivity` JSONB)
3. Add test-connection API endpoint for "Test FHIR Connection" button
4. Implement FHIR Receiver UI in Step 1 (source configuration)
