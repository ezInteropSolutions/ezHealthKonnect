# Query Parameters Field Search Integration

## Overview
Integrated the reusable `FieldPathSearchComponent` into the Query Parameters VALUE field, replacing the plain text input with a smart autocomplete search for HL7 field paths.

## Implementation Date
December 25, 2025

## What Changed

### 1. QueryParamBuilder.js Enhancement
**File**: `public/js/pipeline/components/QueryParamBuilder.js`

**Changes**:
- Replaced plain text input for VALUE field with wrapper div
- Integrated `FieldPathSearchComponent` for smart autocomplete
- Added cleanup logic (destroy method) when rows are deleted
- Updated placeholder text to guide users

**Key Code**:
```javascript
// Create value input with FieldPathSearchComponent
const valueInput = document.createElement('input');
valueInput.type = 'text';
valueInput.className = 'form-control form-control-sm param-value';
valueInput.value = this.escapeHtml(value);
valueInput.placeholder = 'e.g., enhancedSegments.PID.fields[3].value or search...';
valueWrapper.appendChild(valueInput);

// Initialize FieldPathSearchComponent if available
let fieldPathSearch = null;
if (typeof FieldPathSearchComponent !== 'undefined') {
    fieldPathSearch = new FieldPathSearchComponent(valueInput, {
        onSelect: (fieldPath) => {
            valueInput.value = fieldPath;
            this.onChange();
            this.updateUrlPreview();
        },
        placeholder: 'Search HL7 fields or enter custom value...',
        allowCustom: true,
        showCategories: true
    });
    row._fieldPathSearch = fieldPathSearch;
}
```

### 2. Script Include
**File**: `public/pipeline-builder.html`

**Changes**:
- Added `FieldPathSearchComponent.js` script include
- Updated `QueryParamBuilder.js` version to 1.1

**Script Order**:
```html
<script src="/js/pipeline/components/FieldPathSearchComponent.js?v=1.0"></script>
<script src="/js/pipeline/components/FieldPathInputWithAutocomplete.js?v=2.4"></script>
<script src="/js/pipeline/components/ValidationRuleBuilder.js?v=12.0"></script>
<script src="/js/pipeline/components/MetadataBuilder.js?v=1.0"></script>
<script src="/js/pipeline/components/HeaderBuilder.js?v=1.0"></script>
<script src="/js/pipeline/components/QueryParamBuilder.js?v=1.1"></script>
```

## User Experience

### Before
- Plain text input field
- Users had to manually type full HL7 field paths like `enhancedSegments.PID.fields[3].value`
- No suggestions or autocomplete
- Easy to make typos in complex field paths

### After
- Smart autocomplete search as you type
- Search by field name (e.g., "Patient MRN"), description, or path
- Intelligent ranking of matches
- Category-based visual grouping with color-coded icons:
  - 👤 Patient (Blue)
  - 🏥 Visit (Green)
  - 💳 Insurance (Amber)
  - 🔬 Clinical (Red)
  - 📞 Contact (Purple)
  - 👨‍⚕️ Provider (Indigo)
  - 📋 Administrative (Gray)
  - ✏️ Custom (Cyan)
- Recent paths tracking (localStorage)
- Full keyboard navigation (Arrow keys, Enter, Escape)
- Custom value support (can still type manual paths)

## How to Use

### Basic Usage
1. Open Pipeline Builder
2. Add a Database Enrichment step (MySQL, PostgreSQL, SQL Server)
3. In the configuration, scroll to "Query Parameters"
4. Click "Add Parameter"
5. Enter a parameter key (e.g., `mrn`)
6. Click in the VALUE field

### Smart Search
**Option 1: Search by name**
- Type: `patient mrn`
- See: "Patient MRN" suggestion appears
- Press Enter or click to select
- Result: `enhancedSegments.PID.fields[3].value`

**Option 2: Search by description**
- Type: `medical record`
- See: "Patient MRN - Medical Record Number (PID-3)" suggestion
- Select it

**Option 3: Search by path**
- Type: `PID.fields[3]`
- See: Matching field paths appear

**Option 4: Custom path**
- Type: `enhancedSegments.ZZZ.custom`
- Press Enter to use custom path

### Keyboard Navigation
- **Arrow Down/Up**: Navigate suggestions
- **Enter**: Select highlighted suggestion
- **Escape**: Close dropdown
- **Tab**: Move to next field

### Example Configuration
**Scenario**: Query MySQL for patient by MRN from HL7 message

**SQL Query**:
```sql
SELECT * FROM patients WHERE mrn = ?
```

**Query Parameters Configuration**:
| Enabled | Key  | Value                                   | Description         |
|---------|------|-----------------------------------------|---------------------|
| ✅      | mrn  | enhancedSegments.PID.fields[3].value   | Patient MRN from HL7|

**How to configure**:
1. Enter key: `mrn`
2. Click VALUE field
3. Type: `patient mrn`
4. Select "Patient MRN" from dropdown
5. Enter description: `Patient MRN from HL7`

## Available Field Paths

The component includes 40+ pre-defined common HL7 field paths:

### Patient Identification
- Patient MRN → `enhancedSegments.PID.fields[3].value`
- Patient ID → `enhancedSegments.PID.fields[2].value`
- Patient First Name → `enhancedSegments.PID.fields[5].subfields[1].value`
- Patient Last Name → `enhancedSegments.PID.fields[5].subfields[0].value`
- Date of Birth → `enhancedSegments.PID.fields[7].value`
- Gender → `enhancedSegments.PID.fields[8].value`
- SSN → `enhancedSegments.PID.fields[19].value`

### Visit Information
- Visit Number → `enhancedSegments.PV1.fields[19].value`
- Admission Date → `enhancedSegments.PV1.fields[44].value`
- Discharge Date → `enhancedSegments.PV1.fields[45].value`
- Patient Class → `enhancedSegments.PV1.fields[2].value`
- Hospital Service → `enhancedSegments.PV1.fields[10].value`

### Contact Information
- Phone Number → `enhancedSegments.PID.fields[13].value`
- Address → `enhancedSegments.PID.fields[11].value`
- Email → `enhancedSegments.PID.fields[13].value`

### Insurance
- Insurance Plan → `enhancedSegments.IN1.fields[2].value`
- Insurance Company → `enhancedSegments.IN1.fields[3].value`
- Group Number → `enhancedSegments.IN1.fields[8].value`
- Policy Number → `enhancedSegments.IN1.fields[36].value`

### Provider Information
- Attending Doctor → `enhancedSegments.PV1.fields[7].value`
- Referring Doctor → `enhancedSegments.PV1.fields[8].value`
- Admitting Doctor → `enhancedSegments.PV1.fields[17].value`

### Administrative
- Message Control ID → `enhancedSegments.MSH.fields[10].value`
- Message Type → `enhancedSegments.MSH.fields[9].value`
- Sending Application → `enhancedSegments.MSH.fields[3].value`
- Sending Facility → `enhancedSegments.MSH.fields[4].value`

### Clinical
- Observation Value → `enhancedSegments.OBX.fields[5].value`
- Observation ID → `enhancedSegments.OBX.fields[3].value`
- Observation Units → `enhancedSegments.OBX.fields[6].value`

## Technical Details

### Component Architecture
- **Modular Design**: FieldPathSearchComponent is a standalone, reusable component
- **Graceful Degradation**: If component not loaded, falls back to plain input
- **Memory Management**: Proper cleanup via destroy() method when rows deleted
- **State Management**: Recent paths stored in localStorage
- **Performance**: Intelligent ranking algorithm with score-based sorting

### Integration Pattern
```javascript
// Check if component is available
if (typeof FieldPathSearchComponent !== 'undefined') {
    // Initialize with options
    const search = new FieldPathSearchComponent(inputElement, {
        onSelect: (fieldPath) => { /* handle selection */ },
        placeholder: 'Custom placeholder...',
        allowCustom: true,
        showCategories: true
    });

    // Store reference for cleanup
    row._fieldPathSearch = search;
}
```

### Cleanup Pattern
```javascript
deleteBtn.addEventListener('click', () => {
    // Cleanup FieldPathSearchComponent if exists
    if (row._fieldPathSearch) {
        row._fieldPathSearch.destroy();
    }

    row.remove();
    this.rows = this.rows.filter(r => r !== row);
});
```

## Future Integration Points

The FieldPathSearchComponent is designed to be reusable. Potential future integration points:

1. **API Enrichment Field Mapping** - Use for mapping API response fields to HL7 paths
2. **Result Mapping Builder** - Use for selecting source fields in result mappings
3. **Validation Rule Builder** - Use for selecting fields to validate
4. **Custom Script Editor** - Use for inserting field references in JavaScript code
5. **Metadata Builder** - Use for dynamic metadata field selection
6. **Header Builder** - Use for selecting HL7 fields for HTTP headers

## Testing Checklist

- ✅ Component loads without errors
- ✅ Autocomplete dropdown appears when typing
- ✅ Suggestions match search query (name, path, description)
- ✅ Keyboard navigation works (Arrow keys, Enter, Escape)
- ✅ Mouse selection works
- ✅ Custom paths can be entered
- ✅ Recent paths are saved and loaded
- ✅ Category icons display correctly
- ✅ Component cleanup works when row deleted
- ✅ Multiple rows can have independent search instances
- ✅ URL preview updates correctly when field selected

## Related Files

### Created
- `public/js/pipeline/components/FieldPathSearchComponent.js` - Main reusable component

### Modified
- `public/js/pipeline/components/QueryParamBuilder.js` - Integrated search component
- `public/pipeline-builder.html` - Added script include

### Documentation
- `QUERY_PARAMS_FIELD_SEARCH_INTEGRATION.md` - This file
- `MONGODB_NO_CODE_UI_GUIDE.md` - MongoDB UI guide (previous work)
- `MONGODB_ADVANCED_MODE_GUIDE.md` - Advanced MongoDB guide (previous work)

## Benefits

1. **Reduced Errors**: Autocomplete prevents typos in complex field paths
2. **Faster Configuration**: Search by name instead of remembering exact paths
3. **Better UX**: Visual grouping and color coding makes it easier to find fields
4. **Consistent**: Same search experience across all components that use it
5. **Discoverable**: Users can explore available HL7 fields via suggestions
6. **Flexible**: Still allows custom paths for advanced use cases
7. **Intelligent**: Smart ranking shows most relevant matches first
8. **Remembers**: Recent paths make it faster to reuse common fields

## Notes

- The component is optional - if not loaded, falls back to plain text input
- Works with all browsers that support ES6
- LocalStorage used for recent paths (10 most recent)
- No external dependencies required
- Fully keyboard accessible
- Mobile-friendly (responsive dropdown positioning)
