# Flowchart Debug Commands

## Quick Diagnostic Script

Paste this into your browser console (F12 → Console) to verify flowchart is working:

```javascript
// Flowchart Diagnostic Tool v1.3
(function() {
    console.log('=== FLOWCHART DIAGNOSTIC v1.3 ===\n');

    // 1. Check if PipelineBuilder exists
    if (!window.pipelineBuilder) {
        console.error('❌ window.pipelineBuilder not found!');
        return;
    }
    console.log('✅ PipelineBuilder initialized');

    // 2. Check if FlowchartRenderer exists
    if (!window.pipelineBuilder.flowchartRenderer) {
        console.error('❌ FlowchartRenderer not initialized!');
        return;
    }
    console.log('✅ FlowchartRenderer initialized');

    // 3. Check current view mode
    console.log(`📺 Current view mode: ${window.pipelineBuilder.viewMode}`);

    // 4. Check steps loaded
    const steps = window.pipelineBuilder.getAllStepsFlat();
    console.log(`📊 Total steps found: ${steps.length}`);
    if (steps.length > 0) {
        console.table(steps.map(s => ({
            name: s.stepName,
            sequence: s.sequence,
            type: s.stepType,
            subType: s.subType
        })));
    }

    // 5. Check flowchart canvas
    const canvas = document.querySelector('.flowchart-canvas');
    if (canvas) {
        console.log('✅ Flowchart canvas found in DOM');
        console.log(`   - Display: ${canvas.style.display || 'default'}`);
        console.log(`   - Visibility: ${canvas.style.visibility || 'visible'}`);
        console.log(`   - Children: ${canvas.children.length}`);
    } else {
        console.warn('⚠️ Flowchart canvas not in DOM');
    }

    // 6. Check compact nodes
    const nodes = document.querySelectorAll('.step-node-compact');
    console.log(`📦 Step nodes rendered: ${nodes.length}`);

    // 7. Check connections
    const connections = document.querySelectorAll('.connection-path');
    console.log(`🔗 Connection paths rendered: ${connections.length}`);

    // 8. Check CSS loaded
    const canvasWrapper = document.getElementById('canvasWrapper');
    if (canvasWrapper) {
        const styles = window.getComputedStyle(canvasWrapper);
        console.log(`🎨 Canvas wrapper styles:`);
        console.log(`   - touch-action: ${styles.touchAction}`);
        console.log(`   - overflow: ${styles.overflow}`);
    }

    // 9. Check for JavaScript errors
    console.log('\n📋 Check Network tab (F12 → Network) and verify:');
    console.log('   - FlowchartLayoutEngine.js?v=1.3 (Status 200)');
    console.log('   - FlowchartConnector.js?v=1.3 (Status 200)');
    console.log('   - FlowchartRenderer.js?v=1.3 (Status 200)');
    console.log('   - pipeline-builder.css?v=9.3 (Status 200)');

    console.log('\n=== DIAGNOSTIC COMPLETE ===');
})();
```

## Expected Console Output (Healthy State)

When you switch to flowchart mode, you should see:

```
🚀 [FlowchartRenderer v1.3] Initializing flowchart mode
🎯 [FlowchartConnector v1.3] Initializing SVG markers with ezHealthKonnect colors
✅ [FlowchartRenderer v1.3] Initialization complete
✅ Switched to flowchart view
📊 Found 10 steps across all layers
🎨 [FlowchartRenderer v1.2] Render called with 10 steps
📐 Calculating layout...
📊 Layout calculated: {positionsCount: 10, connectionsCount: 9, totalHeight: 1280}
🔗 [v1.1] Connection sequential: {...}
🔗 [v1.1] Connection sequential: {...}
...
```

## Manual Test Commands

### Force Flowchart Render
```javascript
window.pipelineBuilder.switchViewMode('flowchart');
```

### Check Step Positions
```javascript
const renderer = window.pipelineBuilder.flowchartRenderer;
renderer.layoutEngine.positions.forEach((pos, id) => {
    console.log(`Step ${id}:`, pos);
});
```

### Check Connection Data
```javascript
const renderer = window.pipelineBuilder.flowchartRenderer;
console.log('Connections:', renderer.layoutEngine.connections);
```

### Verify getOptimalConnectionPoint (v1.3 fix)
```javascript
const connector = window.pipelineBuilder.flowchartRenderer.connector;
console.log('getOptimalConnectionPoint method:', typeof connector.getOptimalConnectionPoint);
// Should output: "function"
```

### Test Drag Performance
```javascript
// Check if throttle is working
const renderer = window.pipelineBuilder.flowchartRenderer;
console.log('Throttle method:', typeof renderer.throttle);
// Should output: "function"
```

## Version Verification

### Check Loaded Versions
```javascript
// Check script tags
const scripts = Array.from(document.querySelectorAll('script[src*="Flowchart"]'));
scripts.forEach(s => console.log(s.src));

// Expected output:
// .../FlowchartLayoutEngine.js?v=1.3
// .../FlowchartConnector.js?v=1.3
// .../FlowchartRenderer.js?v=1.3
```

### Check CSS Version
```javascript
const links = Array.from(document.querySelectorAll('link[href*="pipeline-builder"]'));
links.forEach(l => console.log(l.href));

// Expected:
// .../pipeline-builder.css?v=9.3
```

## Common Issues

### Issue: Old version cached
**Solution:**
```javascript
// Hard refresh
location.reload(true);
// Or Ctrl+Shift+R (Windows) / Cmd+Shift+R (Mac)
```

### Issue: Steps not showing
**Check:**
```javascript
const steps = window.pipelineBuilder.getAllStepsFlat();
console.log('Steps:', steps.length);

// If 0, check pipeline structure:
console.log('Pipeline:', window.pipelineBuilder.pipeline);
```

### Issue: Arrows not connecting boxes
**Verify v1.3 loaded:**
```javascript
const connector = window.pipelineBuilder.flowchartRenderer.connector;

// Test getOptimalConnectionPoint exists
const fromBox = { x: 100, y: 100, width: 140, height: 90 };
const toBox = { x: 100, y: 250, width: 140, height: 90 };

try {
    const point = connector.getOptimalConnectionPoint(fromBox, toBox, true);
    console.log('✅ v1.3 getOptimalConnectionPoint working:', point);
} catch (e) {
    console.error('❌ v1.3 method missing:', e);
}
```

### Issue: Colors wrong
**Check CSS:**
```javascript
const node = document.querySelector('.step-node-compact');
if (node) {
    const styles = window.getComputedStyle(node);
    console.log('Border color:', styles.borderColor);
    console.log('Box shadow:', styles.boxShadow);
    // Should see pink (#f8bbd9) and navy (#1e3a8a)
}
```

### Issue: Laggy dragging
**Check throttle:**
```javascript
// Watch for throttle messages in console while dragging
// Should see: "🔄 [v1.2] Redrawing connections (throttled)"
// But NOT on every pixel movement (max 60 times per second)
```

## Clear All Caches

### Nuclear Option (Reset Everything)
```javascript
// Clear localStorage
localStorage.clear();

// Clear sessionStorage
sessionStorage.clear();

// Force reload
location.reload(true);
```

### Clear Just Flowchart Positions
```javascript
// Find and remove flowchart position data
Object.keys(localStorage)
    .filter(k => k.startsWith('flowchart_positions_'))
    .forEach(k => {
        console.log('Removing:', k);
        localStorage.removeItem(k);
    });

// Reload
location.reload();
```

## Performance Monitoring

### Check Render Time
```javascript
console.time('flowchart-render');
window.pipelineBuilder.switchViewMode('flowchart');
console.timeEnd('flowchart-render');

// Should be < 200ms for typical pipeline
```

### Monitor Connection Redraw During Drag
```javascript
// While dragging, watch console for:
// "🔄 [v1.2] Redrawing connections (throttled)"
// Should NOT appear more than 60 times per second
```

## Save Diagnostic Report

```javascript
// Generate full diagnostic report
const report = {
    timestamp: new Date().toISOString(),
    versions: {
        layoutEngine: '1.3',
        connector: '1.3',
        renderer: '1.3',
        css: '9.3'
    },
    pipelineBuilder: !!window.pipelineBuilder,
    flowchartRenderer: !!window.pipelineBuilder?.flowchartRenderer,
    viewMode: window.pipelineBuilder?.viewMode,
    stepsCount: window.pipelineBuilder?.getAllStepsFlat()?.length || 0,
    canvasInDOM: !!document.querySelector('.flowchart-canvas'),
    nodesRendered: document.querySelectorAll('.step-node-compact').length,
    connectionsRendered: document.querySelectorAll('.connection-path').length,
    scripts: Array.from(document.querySelectorAll('script[src*="Flowchart"]')).map(s => s.src),
    styles: Array.from(document.querySelectorAll('link[href*="pipeline-builder"]')).map(l => l.href)
};

console.log('DIAGNOSTIC REPORT:', JSON.stringify(report, null, 2));

// Copy to clipboard
navigator.clipboard.writeText(JSON.stringify(report, null, 2));
console.log('✅ Report copied to clipboard');
```
