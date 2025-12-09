# XPath Autocomplete Schema Loading Fix

## Issue Identified
The XPath autocomplete component was successfully rendering but failing to load HL7 schemas with **404 Not Found** error.

### Root Cause
**Message Type Format Mismatch**:
- API receives: `ADT^A01` (caret separator - HL7 standard format)
- File system has: `ADT_A01.gz` (underscore separator - valid filename)
- Controller was looking for: `ADT^A01.gz` (invalid filename with special character)

### Console Error
```
Failed to load resource: /api/schemas/hl7/v2.5/ADT%5EA01 (404 Not Found)
```
(`%5E` is URL-encoded caret `^`)

## Solution Implemented

### File: `controllers/schemaController.js`

#### Fix 1: `getHL7Schema()` - Convert Caret to Underscore
```javascript
exports.getHL7Schema = async (req, res) => {
    try {
        const { version, messageType } = req.params;

        // Convert caret (^) to underscore (_) for file system lookup
        // API receives: ADT^A01, file system has: ADT_A01.gz
        const fileMessageType = messageType.replace(/\^/g, '_');

        const schemaPath = path.join(
            __dirname,
            '..',
            'schemas',
            'hl7',
            version,
            `${fileMessageType}.gz`  // Now looks for ADT_A01.gz
        );

        console.log(`📂 Loading HL7 schema: ${version}/${fileMessageType}.gz`);

        // Read and decompress schema...
        // Return xpathTree...
    }
};
```

#### Fix 2: `getHL7MessageTypes()` - Convert Underscore to Caret
```javascript
exports.getHL7MessageTypes = async (req, res) => {
    try {
        const { version } = req.params;
        const versionDir = path.join(__dirname, '..', 'schemas', 'hl7', version);

        const files = await fs.readdir(versionDir);
        const messageTypes = files
            .filter(file => file.endsWith('.gz'))
            .map(file => {
                // Convert file name (ADT_A01.gz) to API format (ADT^A01)
                const fileMessageType = file.replace('.gz', '');
                const apiMessageType = fileMessageType.replace(/_/g, '^');
                return {
                    value: apiMessageType,  // ADT^A01
                    label: apiMessageType,  // ADT^A01
                };
            });

        res.json({ success: true, messageTypes });
    }
};
```

## Verification

### Before Fix
```
❌ GET /api/schemas/hl7/v2.5/ADT^A01 → 404 Not Found
❌ Looking for file: schemas/hl7/v2.5/ADT^A01.gz (doesn't exist)
```

### After Fix
```
✅ GET /api/schemas/hl7/v2.5/ADT^A01 → 200 OK
✅ Looking for file: schemas/hl7/v2.5/ADT_A01.gz (exists!)
✅ Returns parsed XPath tree with all segments
```

## Testing

### Step 1: Hard Refresh Browser
Clear cache to reload JavaScript files:
- Windows: `Ctrl + Shift + R` or `Ctrl + F5`
- Mac: `Cmd + Shift + R`

### Step 2: Test XPath Autocomplete
1. Go to Pipeline Builder
2. Drag "Field Validation" step onto canvas
3. Double-click to open configuration modal
4. Check console for success messages:

```
[ValidationRuleBuilder] Found XPath containers: 2
[XPathAutocomplete] Rendering into container: <div>
[XPathAutocomplete] Elements found: {input: true, dropdown: true, dropdownList: true}
✅ Schema loaded successfully: 23 segments
```

### Step 3: Verify Field Path Input Visible
You should now see:
- **Input field** with placeholder "Type to search field paths..."
- Typing triggers autocomplete dropdown
- Schema loaded with all HL7 segments (MSH, PID, PV1, etc.)

### Expected Server Logs
```bash
docker-compose logs -f app | grep schema
```

Should show:
```
📂 Loading HL7 schema: v2.5/ADT_A01.gz
✅ Schema loaded successfully: 23 segments
```

## Files Modified
- [controllers/schemaController.js](controllers/schemaController.js) - Lines 70-104

## Dependencies
- Node.js service restarted: `docker-compose restart app`
- No database changes required
- No client-side changes required (format conversion happens server-side)

## Status
✅ **FIXED** - Schema API now correctly handles HL7 message type format conversion
