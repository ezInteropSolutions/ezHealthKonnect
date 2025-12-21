# Target Type Backend Fix - November 3, 2025

## Issue
Backend returned error when saving edited interface:
```json
{
    "success": false,
    "error": "Failed to update interface",
    "debug": "Named replacement \":targetType\" has no entry in the replacement map."
}
```

## Root Cause
The backend SQL query expects a `targetType` parameter, but the frontend no longer sends it:

**Backend SQL** (interfacesController.js:717-732):
```sql
UPDATE interfaces SET
    target_type = :targetType,
    target_connectivity = :targetConnectivity,
    ...
WHERE id = :interfaceId
```

**Frontend Issue**:
- We removed the "Target Type" selector from the edit modal
- Now only "Target Connectivity" is collected
- Backend still requires `targetType` field

## Solution

Added automatic `targetType` derivation from `targetConnectivity`:

```javascript
// Map connectivity to appropriate type
const targetTypeMap = {
    'http': 'fhir',      // HTTP/REST → FHIR server
    'tcp': 'hl7v2',      // TCP/MLLP → HL7 v2.x system
    'file': 'file',      // File output → File system
    'database': 'database' // Database → Database
};
formData.targetType = targetTypeMap[formData.targetConnectivity] || 'fhir';
```

## Why This Mapping?

**HTTP/REST → FHIR**:
- HTTP/REST endpoints are typically FHIR servers
- Default assumption for healthcare HTTP APIs

**TCP/MLLP → HL7 v2.x**:
- TCP/MLLP protocol is exclusively used for HL7 v2.x messages
- Industry standard for HL7 messaging

**File → File**:
- File output type matches file connectivity

**Database → Database**:
- Database output type matches database connectivity

## Files Modified

**[interface-config-manager.js](public/js/interface-config-manager.js:367-375)**:
- Lines 367-375: Added targetType mapping logic

**[interfaces.html](public/interfaces.html:189)**:
- Line 189: Updated cache buster to `?v=3`

## Testing

1. **Hard Refresh**: Ctrl+Shift+R (or Cmd+Shift+R on Mac)
2. **Edit Interface**: Click Edit (⚙️) on any interface
3. **Change Target**: Select "File Output" for target connectivity
4. **Save Changes**: Click "Save Changes" button
5. **Verify Success**: Should see success message, no errors

### Expected Console Output

```
📋 Starting form data collection...
🔍 Field availability: { editInterfaceId: true, ... }
📝 Basic fields collected: { 
    id: "...",
    name: "...",
    sourceType: "hl7v2",
    sourceConnectivity: "tcp",
    targetConnectivity: "file"
}
```

**After mapping**:
```
formData.targetType = "file"  // Derived from targetConnectivity
```

## Impact

- ✅ Backend receives required `targetType` parameter
- ✅ No need to show redundant "Target Type" selector in UI
- ✅ Automatic, logical mapping from connectivity to type
- ✅ Edit modal save functionality fully restored

## Complete Fix Summary

All issues from this session are now resolved:

1. ✅ **Port Validation** - Real-time validation instead of hardcoded list
2. ✅ **Dynamic URL Preview** - Updates as user types
3. ✅ **File Target Fields** - Shows correct fields, not endpoint
4. ✅ **Field ID Mismatch** - Uses correct shared component IDs
5. ✅ **Missing targetType** - Automatically derived from connectivity

The edit modal is now fully functional with all target connectivity types!
