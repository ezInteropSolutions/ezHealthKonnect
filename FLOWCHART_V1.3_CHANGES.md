# Flowchart Mode v1.3 - Diagnostic Update

## What Changed in v1.3

This update adds comprehensive diagnostic logging to help verify that the flowchart improvements are working correctly.

## Files Updated

### 1. [pipeline-builder.html](public/pipeline-builder.html)
**Changed:** Version numbers bumped from 1.0/1.1/1.2 → **v1.3**
```html
<!-- OLD -->
<script src="/js/pipeline/utils/FlowchartLayoutEngine.js?v=1.0"></script>
<script src="/js/pipeline/utils/FlowchartConnector.js?v=1.1"></script>
<script src="/js/pipeline/managers/FlowchartRenderer.js?v=1.2"></script>

<!-- NEW -->
<script src="/js/pipeline/utils/FlowchartLayoutEngine.js?v=1.3"></script>
<script src="/js/pipeline/utils/FlowchartConnector.js?v=1.3"></script>
<script src="/js/pipeline/managers/FlowchartRenderer.js?v=1.3"></script>
```

**Why:** Force browser cache refresh to load latest code with bug fixes.

---

### 2. [FlowchartConnector.js](public/js/pipeline/utils/FlowchartConnector.js) - v1.3
**Added:** Initialization logging
```javascript
initializeSVG() {
    console.log('🎯 [FlowchartConnector v1.3] Initializing SVG markers with ezHealthKonnect colors');
    // ... rest of code
}
```

**Added:** Connection rendering logging
```javascript
renderConnection(connection, steps) {
    // ... existing code ...

    console.log(`🔗 [v1.3] Connection ${connection.type}:`, {
        from: connection.from,
        to: connection.to,
        fromPoint,
        toPoint,
        fromBox,
        toBox
    });
}
```

**Why:** Verify that `getOptimalConnectionPoint()` is being called with correct box positions.

---

### 3. [FlowchartRenderer.js](public/js/pipeline/managers/FlowchartRenderer.js) - v1.3
**Added:** Initialization logging
```javascript
init() {
    console.log('🚀 [FlowchartRenderer v1.3] Initializing flowchart mode');
    // ... existing code ...
    console.log('✅ [FlowchartRenderer v1.3] Initialization complete');
}
```

**Added:** Render logging
```javascript
render(steps) {
    console.log('🎨 [FlowchartRenderer v1.3] Render called with', steps?.length || 0, 'steps');

    if (!steps || steps.length === 0) {
        console.warn('⚠️ No steps to render - showing empty state');
        this.renderEmptyState();
        return;
    }

    console.log('📐 Calculating layout...');
    const layout = this.layoutEngine.calculateLayout(steps);
    console.log('📊 Layout calculated:', {
        positionsCount: layout.positions.size,
        connectionsCount: layout.connections.length,
        totalHeight: layout.totalHeight
    });
}
```

**Added:** Drag logging
```javascript
makeNodeDraggable(node, step) {
    node.addEventListener('mousedown', (e) => {
        // ... existing code ...
        console.log('🖱️ [v1.3] Started dragging step:', step.stepName);
    });

    const throttledRedraw = this.throttle(() => {
        console.log('🔄 [v1.3] Redrawing connections (throttled)');
        this.redrawConnections();
    }, 16);
}
```

**Why:** Track execution flow and verify drag-and-drop with throttled redraw is working.

---

## Expected Console Output

When you switch to flowchart mode and interact with it, you should now see:

### On Initialization:
```
🚀 [FlowchartRenderer v1.3] Initializing flowchart mode
🎯 [FlowchartConnector v1.3] Initializing SVG markers with ezHealthKonnect colors
✅ [FlowchartRenderer v1.3] Initialization complete
```

### On View Switch:
```
✅ Switched to flowchart view
📊 Found 10 steps across all layers
🎨 [FlowchartRenderer v1.3] Render called with 10 steps
📐 Calculating layout...
📊 Layout calculated: {positionsCount: 10, connectionsCount: 9, totalHeight: 1280}
```

### On Connection Rendering:
```
🔗 [v1.3] Connection sequential: {
    from: "step-id-1",
    to: "step-id-2",
    fromPoint: { x: 400, y: 170 },
    toPoint: { x: 400, y: 200 },
    fromBox: { x: 400, y: 80, width: 140, height: 90 },
    toBox: { x: 400, y: 200, width: 140, height: 90 }
}
```

### On Dragging:
```
🖱️ [v1.3] Started dragging step: Validate HL7
🔄 [v1.3] Redrawing connections (throttled)
🔄 [v1.3] Redrawing connections (throttled)
🔄 [v1.3] Redrawing connections (throttled)
```

---

## How to Verify Changes

### Step 1: Hard Refresh Browser
```
Windows: Ctrl + Shift + R
Mac: Cmd + Shift + R
```

### Step 2: Open Browser Console
```
F12 → Console tab
```

### Step 3: Switch to Flowchart Mode
Click the "Flowchart" button in the pipeline builder.

### Step 4: Check Console Output
You should see the emoji-prefixed log messages listed above.

### Step 5: Run Diagnostic Script
Open [FLOWCHART_DEBUG_COMMANDS.md](FLOWCHART_DEBUG_COMMANDS.md) and paste the diagnostic script into the console.

---

## What This Verifies

### ✅ v1.3 Files Loaded
If you see `[v1.3]` in console logs, the new files are loaded.

### ✅ getOptimalConnectionPoint() Working
The `🔗 Connection` logs show `fromPoint` and `toPoint` values, proving edge detection is running.

### ✅ Throttled Redraw Working
During drag, you should see `🔄 Redrawing` messages, but NOT on every pixel (max 60/second).

### ✅ Brand Colors Applied
Inspect a `.step-node-compact` element:
- Border: `#f8bbd9` (pastel pink)
- Hover border: `#1e3a8a` (navy blue)

### ✅ Steps Rendering
`📊 Found X steps` should match the number of steps in your pipeline.

---

## Previous Fixes (Still Active)

All previous fixes from v1.1 and v1.2 are still active:

### From v1.1 (Arrow Edge Detection):
- `getOptimalConnectionPoint(fromBox, toBox, isSource)` - Intelligent edge selection
- Arrows connect box edges based on flow direction (not floating on side)

### From v1.2 (Performance):
- `throttle(func, delay)` - Limit redraws to 60fps
- `will-change: transform` CSS - GPU acceleration
- Faster transitions (0.15s)

### From v9.3 (Colors):
- Pastel pink borders (#f8bbd9)
- Navy blue hover (#1e3a8a)
- Lightweight shadows

---

## Troubleshooting

### If You Don't See Console Logs

**Problem:** No emoji logs in console
**Cause:** Old version still cached
**Solution:**
1. Hard refresh: Ctrl+Shift+R
2. Check Network tab (F12 → Network)
3. Verify files show `?v=1.3`
4. Clear browser cache completely

### If Arrows Still on Side

**Problem:** Arrows not connecting box edges
**Cause:** `getOptimalConnectionPoint()` not being called
**Solution:**
1. Check console for `🔗 Connection` logs
2. If missing, v1.3 not loaded
3. Verify in Network tab: `FlowchartConnector.js?v=1.3` status 200

### If Dragging Still Laggy

**Problem:** Choppy drag performance
**Cause:** Throttle not working
**Solution:**
1. While dragging, watch console
2. Should see `🔄 Redrawing (throttled)` but not constantly
3. If constant spam, v1.3 not loaded

---

## Testing Checklist

After Docker restart and hard refresh:

- [ ] Console shows `🚀 [FlowchartRenderer v1.3] Initializing`
- [ ] Console shows `🎯 [FlowchartConnector v1.3] Initializing`
- [ ] Switching to flowchart shows `📊 Found X steps`
- [ ] Console shows `🔗 Connection` logs with `fromPoint`/`toPoint`
- [ ] Boxes are connected with smooth arrows (not floating)
- [ ] Dragging is smooth (not laggy)
- [ ] Boxes have pink borders
- [ ] Hovering shows navy blue border
- [ ] Dragging shows `🔄 Redrawing (throttled)` (not every pixel)

---

## Next Steps

If all checklist items pass:
1. ✅ v1.3 diagnostic logging working
2. ✅ Arrow edge detection verified
3. ✅ Performance throttling verified
4. ✅ Brand colors applied

If issues persist:
1. Run diagnostic script from [FLOWCHART_DEBUG_COMMANDS.md](FLOWCHART_DEBUG_COMMANDS.md)
2. Share console output
3. Share Network tab screenshot (filtered to "Flowchart")

---

## Summary

**What v1.3 Does:**
- Adds comprehensive logging to verify all fixes are active
- Makes it easy to diagnose if browser is using cached files
- Confirms edge detection, throttling, and colors are working

**What v1.3 Does NOT Change:**
- No new features (just diagnostic logging)
- No architectural changes
- No CSS changes (already in v9.3)

**Expected Result:**
User can now verify in console that:
- Latest code is loaded (v1.3 tags)
- Arrows connect box edges (fromPoint/toPoint logs)
- Dragging is throttled (60fps logs)
- All 10 steps render correctly
