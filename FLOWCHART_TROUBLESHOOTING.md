# Flowchart Mode - Troubleshooting & Debug Guide

## Issues Fixed ✅

### Issue 1: Steps Not Showing in Flowchart
**Problem**: Interface 8 has 10 steps in list view, but flowchart shows empty canvas

**Root Cause**: Pipeline uses layer-based structure (`pipeline.layers.pre/core/post.executionGroups[].steps`), not flat `pipeline.steps` array

**Fix Applied**:
- Added `getAllStepsFlat()` method to extract steps from all layers
- Sorts by sequence number
- Logs step count to console for debugging

**File**: [PipelineBuilder.js:955-980](public/js/pipeline/PipelineBuilder.js#L955-L980)

**Verification**:
```javascript
// Open browser console (F12)
// You should see:
📊 Found 10 steps across all layers
🎨 Rendering flowchart with steps: 10 [Array of steps]
```

---

### Issue 2: Touchpad Scroll Not Working in List View
**Problem**: Scrollbar works, but touchpad swipe doesn't scroll canvas

**Root Cause**: Missing CSS touch-action properties for modern trackpad support

**Fix Applied**:
```css
.canvas-wrapper {
    touch-action: pan-y pan-x;      /* Enable trackpad gestures */
    overscroll-behavior: contain;   /* Prevent parent scroll bounce */
}
```

**File**: [pipeline-builder.css:438-439](public/css/pipeline-builder.css#L438-L439)

**Verification**:
- Try two-finger swipe on touchpad → Canvas should scroll smoothly
- Try mouse wheel → Should still work
- Try scrollbar → Should still work

---

## Debug Checklist

### If Flowchart Shows Empty Canvas

1. **Check Console Logs** (F12 → Console):
   ```
   Expected:
   📊 Found X steps across all layers
   🎨 Rendering flowchart with steps: X [Array]

   If you see:
   ⚠️ No pipeline or layers found
   📊 Found 0 steps across all layers
   → Pipeline structure issue
   ```

2. **Inspect Pipeline Structure**:
   ```javascript
   // In browser console
   window.pipelineBuilder.pipeline

   // Should show:
   {
       layers: {
           pre: { executionGroups: [...] },
           core: { executionGroups: [...] },
           post: { executionGroups: [...] }
       }
   }
   ```

3. **Check Layer Groups**:
   ```javascript
   // In console
   window.pipelineBuilder.pipeline.layers.pre.executionGroups

   // Each group should have:
   {
       id: "...",
       steps: [
           { id: "...", stepName: "...", sequence: 10, ... }
       ]
   }
   ```

4. **Verify FlowchartRenderer Loaded**:
   ```javascript
   // In console
   window.pipelineBuilder.flowchartRenderer

   // Should be an object, not null
   ```

5. **Check for JavaScript Errors**:
   - F12 → Console → Look for red errors
   - Common: "Cannot read property 'steps' of undefined"
   - Common: "FlowchartLayoutEngine is not defined"

---

### If Touchpad Scroll Still Doesn't Work

1. **Hard Refresh** (Clear Cache):
   - Windows: Ctrl+Shift+R
   - Mac: Cmd+Shift+R
   - Verify CSS version loaded: `/css/pipeline-builder.css?v=9.1`

2. **Check Browser Support**:
   ```javascript
   // In console
   getComputedStyle(document.querySelector('.canvas-wrapper')).touchAction

   // Should return: "pan-x pan-y" or "pan-y pan-x"
   ```

3. **Test with Different Input**:
   - Touchpad two-finger swipe
   - Mouse wheel
   - Scrollbar drag
   - Arrow keys (if focused)

4. **Check for Overlapping Elements**:
   ```javascript
   // In console - hover over canvas and run:
   document.elementFromPoint(event.clientX, event.clientY)

   // Should return .canvas-wrapper or child elements
   // If it returns something else, that element is blocking scroll
   ```

---

### If Steps Render But Connections Missing

1. **Check SVG Layer**:
   ```javascript
   // In console
   document.querySelector('.flowchart-connections')

   // Should exist and have <path> children
   ```

2. **Inspect Connection Data**:
   ```javascript
   // In console
   window.pipelineBuilder.flowchartRenderer.connector.connections

   // Should have array of connection objects
   ```

3. **Check for SVG Errors**:
   ```javascript
   // In console
   document.querySelectorAll('.connection-path')

   // Should return NodeList with path elements
   ```

4. **Verify Layout Calculated**:
   ```javascript
   // In console
   window.pipelineBuilder.flowchartRenderer.layoutEngine.positions

   // Should be Map with step positions
   ```

---

### If Fork Detection Not Working

**Symptoms**: If-then-else steps don't show Y-fork, just sequential lines

**Debug Steps**:

1. **Check Step Configuration**:
   ```javascript
   // In console - find your if-then-else step
   const steps = window.pipelineBuilder.getAllStepsFlat();
   const ifThenStep = steps.find(s => s.subType === 'conditional');

   console.log(ifThenStep.config.conditions);

   // Should show:
   [{
       ifTrue: { action: 'route_to_step', stepId: 'xxx' },
       ifFalse: { action: 'route_to_step', stepId: 'yyy' }
   }]
   ```

2. **Verify Step IDs Exist**:
   ```javascript
   // Check if target steps exist
   const targetTrueId = ifThenStep.config.conditions[0].ifTrue.stepId;
   const targetFalseId = ifThenStep.config.conditions[0].ifFalse.stepId;

   const trueStep = steps.find(s => s.id === targetTrueId);
   const falseStep = steps.find(s => s.id === targetFalseId);

   console.log('TRUE target found:', !!trueStep);
   console.log('FALSE target found:', !!falseStep);
   ```

3. **Check Fork Detection**:
   ```javascript
   // In console
   const layoutEngine = window.pipelineBuilder.flowchartRenderer.layoutEngine;
   const forkInfo = layoutEngine.detectFork(ifThenStep, steps);

   console.log('Fork detected:', forkInfo.isFork);
   console.log('TRUE branch:', forkInfo.trueBranch);
   console.log('FALSE branch:', forkInfo.falseBranch);
   ```

---

### If Nodes Overlap or Layout Broken

1. **Check Step Sequences**:
   ```javascript
   // In console
   const steps = window.pipelineBuilder.getAllStepsFlat();
   console.table(steps.map(s => ({
       name: s.stepName,
       seq: s.sequence,
       layer: s.sequence < 100 ? 'pre' : s.sequence < 200 ? 'core' : 'post'
   })));

   // Sequences should be unique and ordered
   ```

2. **Check Position Calculations**:
   ```javascript
   // In console
   const layoutEngine = window.pipelineBuilder.flowchartRenderer.layoutEngine;
   const layout = layoutEngine.calculateLayout(steps);

   console.log('Positions:', layout.positions);
   console.log('Bounding box:', layoutEngine.getBoundingBox());
   ```

3. **Verify No NaN Coordinates**:
   ```javascript
   // In console
   layout.positions.forEach((pos, id) => {
       if (isNaN(pos.x) || isNaN(pos.y)) {
           console.error('Invalid position for step:', id, pos);
       }
   });
   ```

---

## Common Error Messages

### "Cannot read property 'steps' of null"
**Cause**: Pipeline not loaded yet
**Fix**: Wait for pipeline to load before switching to flowchart

### "FlowchartLayoutEngine is not defined"
**Cause**: Script not loaded
**Fix**: Check script order in HTML, verify /js/pipeline/utils/FlowchartLayoutEngine.js loads

### "getConnectionPoints of undefined"
**Cause**: Step ID not found in position map
**Fix**: Check step IDs in routing configuration match actual steps

### Empty canvas with no errors
**Cause**: Steps extracted as empty array
**Fix**: Check `getAllStepsFlat()` returns steps (see debug above)

---

## Browser DevTools Tips

### View Step Nodes
```javascript
// Select all compact nodes
document.querySelectorAll('.step-node-compact')

// Get specific node
document.getElementById('step-node-YOUR-STEP-ID')

// Check node position
const node = document.querySelector('.step-node-compact');
console.log(node.style.left, node.style.top);
```

### View Connections
```javascript
// All connections
document.querySelectorAll('.connection-path')

// Connection by type
document.querySelectorAll('.connection-path.true-branch')
document.querySelectorAll('.connection-path.false-branch')

// Check path data
const path = document.querySelector('.connection-path');
console.log(path.getAttribute('d')); // SVG path commands
```

### View Layout State
```javascript
// Current view mode
window.pipelineBuilder.viewMode  // 'list' or 'flowchart'

// Current zoom level
window.pipelineBuilder.flowchartRenderer.zoomLevel  // 1 = 100%

// Current pan offset
window.pipelineBuilder.flowchartRenderer.panOffset  // {x: 0, y: 0}
```

---

## Performance Monitoring

### Check Render Time
```javascript
// In console - before switching to flowchart
console.time('flowchart-render');

// Click flowchart button

// In console - after render complete
console.timeEnd('flowchart-render');

// Should be < 200ms for typical pipeline
```

### Check Memory Usage
```javascript
// In console
performance.memory.usedJSHeapSize / 1024 / 1024  // MB

// Switch to flowchart

// Check again
performance.memory.usedJSHeapSize / 1024 / 1024  // MB

// Difference should be < 5MB for typical pipeline
```

---

## File Checklist

### Verify All Files Loaded (Network Tab - F12)

✅ Required files:
- `/css/pipeline-builder.css?v=9.1` (Status 200)
- `/js/pipeline/utils/FlowchartLayoutEngine.js?v=1.0` (Status 200)
- `/js/pipeline/utils/FlowchartConnector.js?v=1.0` (Status 200)
- `/js/pipeline/managers/FlowchartRenderer.js?v=1.0` (Status 200)
- `/js/pipeline/PipelineBuilder.js?v=10.1` (Status 200)

### Verify File Sizes
- FlowchartLayoutEngine.js: ~15 KB
- FlowchartConnector.js: ~13 KB
- FlowchartRenderer.js: ~11 KB

If files show 0 bytes or 404, check file paths and Docker restart.

---

## Quick Fix Commands

### Clear Browser Cache
```javascript
// In console
localStorage.clear();
sessionStorage.clear();
location.reload(true);
```

### Reset View Mode
```javascript
// In console
localStorage.removeItem('pipelineViewMode');
location.reload();
```

### Force Flowchart Render
```javascript
// In console
window.pipelineBuilder.switchViewMode('flowchart');
window.pipelineBuilder.renderFlowchart();
```

---

## Testing Commands

### Create Test Pipeline with Fork
```javascript
// In browser console after loading pipeline builder

// Get all current steps
const steps = window.pipelineBuilder.getAllStepsFlat();
console.log('Current steps:', steps.length);

// If you have an if-then-else step, inspect it
const conditionalStep = steps.find(s => s.subType === 'conditional');
if (conditionalStep) {
    console.log('Conditional step config:', conditionalStep.config);
} else {
    console.log('No conditional step found - create one in List view first');
}
```

### Test Zoom
```javascript
// In console
const renderer = window.pipelineBuilder.flowchartRenderer;

// Zoom in
renderer.zoom(0.25);

// Zoom out
renderer.zoom(-0.25);

// Reset
renderer.resetView();
```

### Test Pan
```javascript
// In console
const renderer = window.pipelineBuilder.flowchartRenderer;

// Pan right
renderer.panOffset.x += 100;
renderer.updateTransform();

// Pan down
renderer.panOffset.y += 100;
renderer.updateTransform();

// Reset
renderer.resetView();
```

---

## Expected Console Output (Normal Operation)

```
Pipeline Builder initialized
✅ Switched to flowchart view
📊 Found 10 steps across all layers
🎨 Rendering flowchart with steps: 10 [Array(10)]
  0: {id: "...", stepName: "Validate", sequence: 10, ...}
  1: {id: "...", stepName: "If-Then-Else", sequence: 20, ...}
  ...
```

---

## Need Help?

1. **Check console first** (F12 → Console)
2. **Look for emoji indicators**: 📊 ✅ ⚠️ 🎨
3. **Run debug commands** from this guide
4. **Check Network tab** for failed file loads
5. **Verify Docker logs**: `docker-compose logs -f app | grep flowchart`

**Files to reference**:
- [FLOWCHART_MODE_IMPLEMENTATION_COMPLETE.md](FLOWCHART_MODE_IMPLEMENTATION_COMPLETE.md) - Full docs
- [FLOWCHART_MODE_QUICK_START.md](FLOWCHART_MODE_QUICK_START.md) - User guide
