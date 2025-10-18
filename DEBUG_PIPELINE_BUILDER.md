# Pipeline Builder - Debug Instructions

## 🔍 Step 1: Check Browser Console

**IMPORTANT: Please do this first!**

1. Open Pipeline Builder in your browser
2. Press **F12** to open Developer Tools
3. Click **Console** tab
4. Look for red error messages

**Send me these errors** - they will tell us exactly what's wrong.

---

## Common Errors & What They Mean

### Error: "Uncaught ReferenceError: StepTemplate is not defined"
**Cause**: JavaScript files not loading in correct order
**Fix**: Check script tags in pipeline-builder.html

### Error: "Failed to load resource: net::ERR_FILE_NOT_FOUND"
**Cause**: JavaScript file paths incorrect
**Fix**: Verify files exist at specified paths

### Error: "Cannot read property 'addEventListener' of null"
**Cause**: HTML elements not found (wrong IDs)
**Fix**: Check element IDs match

---

## 🔍 Step 2: Check Network Tab

1. Press **F12** → **Network** tab
2. Refresh page
3. Look for red/failed requests
4. Check if these files loaded:
   - PipelineModels.js
   - PipelineAPIService.js
   - DragDropManager.js
   - ToolboxManager.js
   - etc.

---

## 🔍 Step 3: Manual Test

Open browser console (F12) and type:

```javascript
// Test 1: Check if classes are loaded
typeof PipelineBuilder
// Should return: "function"

// Test 2: Check if instance exists
window.pipelineBuilder
// Should return: PipelineBuilder object

// Test 3: Check templates
window.pipelineBuilder?.toolboxManager?.templates
// Should return: Array of 5 templates
```

**Copy the output and send to me**

---

## 🔍 Likely Issues

Based on "no change" and "buttons don't work":

1. **JavaScript not loading** - Files not found or wrong paths
2. **Script order wrong** - Dependencies loading after dependents
3. **Initialization failed** - Error in init.js
4. **Browser cache** - Old version still loaded

---

## Quick Fixes to Try

### Fix 1: Hard Refresh
```
Windows: Ctrl + Shift + R
Mac: Cmd + Shift + R
```

### Fix 2: Clear Cache
```
Chrome: F12 → Network tab → Check "Disable cache"
Then refresh page
```

### Fix 3: Check File Paths
Verify these files exist:
```
public/js/pipeline/models/PipelineModels.js
public/js/pipeline/services/PipelineAPIService.js
public/js/pipeline/managers/DragDropManager.js
public/js/pipeline/managers/CanvasRenderer.js
public/js/pipeline/managers/StepNodeManager.js
public/js/pipeline/managers/ToolboxManager.js
public/js/pipeline/managers/PropertiesPanel.js
public/js/pipeline/managers/LayerContainer.js
public/js/pipeline/PipelineBuilder.js
public/js/pipeline/init.js
```

---

Please run these checks and send me the console errors!
