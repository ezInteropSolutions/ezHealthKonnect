# Validation Builder - Status & Testing Guide

## ✅ Completed Features

### 1. Step Renamed
- **Old**: "Validate Required Fields"
- **New**: "HL7 Field Validation"
- **File**: `public/js/pipeline/managers/ToolboxManager.js`
- **Reason**: Step supports multiple validation types, not just required fields

### 2. Current Validation Builder Features
✅ Visual field builder (no JSON editing)
✅ Add/Remove validation rules
✅ Validation types dropdown:
   - Required Field
   - Format Check
   - Length Validation
   - Regex Pattern

### 3. JSON Config Format

**Your Current Config**:
```json
{
  "config": {
    "rules": [
      {
        "field": "MSH.9",
        "required": true  ← Old format
      }
    ]
  }
}
```

**New UI-Generated Config**:
```json
{
  "config": {
    "rules": [
      {
        "field": "MSH.9",
        "type": "required",  ← New format
        "errorMessage": "Message type is required"
      }
    ]
  }
}
```

## 🔄 Pending Enhancements

### Format Check Presets (Requested)
Need to add conditional fields that show/hide based on validation type:

**Format Type** → Show dropdown:
- Phone Number
- SSN (XXX-XX-XXXX)
- Email Address
- Date (YYYYMMDD)
- DateTime (YYYYMMDDHHMMSS)
- MRN (Medical Record Number)
- ZIP Code

**Length Type** → Show inputs:
- Min Length
- Max Length

**Pattern Type** → Show input:
- Regex Pattern (user-defined)

**Required Type** → No additional fields

### Toggle Logic Needed
Add `toggleValidationOptions()` method to show/hide conditional fields when validation type changes.

## 🧪 Testing Guide

### How to Test Validation Builder

1. **Open Pipeline Builder**:
   ```
   http://localhost:3000/pipeline-builder.html?interfaceId={your-interface-id}
   ```

2. **Add Validation Step**:
   - Double-click "HL7 Field Validation" in left toolbox
   - Properties modal should open

3. **Test Add Rule**:
   - Click "+ Add Rule" button
   - Should add new empty row

4. **Test Remove Rule**:
   - Click × button on a rule
   - Should remove row (or clear if last row)

5. **Test Form Inputs**:
   - Enter HL7 field path (e.g., "PID.5")
   - Select validation type
   - Enter error message
   - Check hidden JSON field updates

6. **Test Save**:
   - Click "Import & Add to Pipeline"
   - Step should appear in pipeline
   - Check step config in browser console

7. **Test JSON Export**:
   - Switch to "Import JSON" tab
   - Click "Export Current"
   - Should download `.json` file

## 📋 Import/Export Button Clarification

### Three Different Buttons:

1. **"Import & Add to Pipeline"** (Preview Mode):
   - **When**: Double-click step in toolbox
   - **What**: Creates NEW step from JSON → Adds to pipeline
   - **Use Case**: Quick step creation

2. **"Import & Update"** (Edit Mode):
   - **When**: Edit existing step in pipeline
   - **What**: Updates CURRENT step config from JSON
   - **Use Case**: Modifying existing step

3. **"Export Current"** (Edit Mode):
   - **When**: Edit existing step
   - **What**: Downloads step config as `.json` file
   - **Use Case**: Backup/sharing

### Info Cards (Not Buttons):
- "Import Configuration" card = Explanation only
- "Export Configuration" card = Explanation only

## ⚠️ Known Issues

### 1. Validation Doesn't Execute Yet
- Steps are configuration only
- No actual validation logic runs
- Need executor implementation in Go backend

### 2. JSON Format Mismatch
- Old config uses `required: true`
- New UI uses `type: "required"`
- Backend needs to handle both formats

## 🚀 Next Steps

### Immediate:
1. Add format presets dropdown
2. Add length min/max inputs
3. Add pattern input field
4. Add toggle logic for conditional fields
5. Update cache version to v=8.3

### Future:
1. Implement validation executor in Go
2. Test with real HL7 messages
3. Add validation result display
4. Add pre-populated regex patterns for formats

## 📁 Files Modified

- ✅ `public/js/pipeline/managers/ToolboxManager.js` - Step renamed
- ✅ `public/js/pipeline/managers/PropertiesPanel.js` - Validation builder added
- ✅ `public/css/pipeline-builder.css` - Styling added
- ✅ `public/pipeline-builder.html` - Cache v=8.2

## 🔍 Quick Test Script

```javascript
// Open browser console in Pipeline Builder
// Check if validation builder is working:

// 1. Check if propertiesPanel is available
console.log(window.propertiesPanel);

// 2. Open a validation step
// Double-click "HL7 Field Validation" step

// 3. Check validation rules
const builderContainer = document.querySelector('.validation-builder');
if (builderContainer) {
    console.log('✅ Validation builder found');
    const hiddenField = builderContainer.querySelector('input[type="hidden"]');
    console.log('Current rules JSON:', hiddenField.value);
} else {
    console.log('❌ Validation builder not found');
}
```

## 📞 Support

If validation builder isn't working:
1. Check browser console for errors
2. Clear cache (Ctrl+Shift+R)
3. Verify cache version is v=8.2 in page source
4. Check if `propertiesPanel` is available in console
