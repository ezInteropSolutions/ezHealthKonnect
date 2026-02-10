# Clear Flowchart Saved Positions

## The Issue

Your flowchart is loading **saved positions** from a previous manual drag session. These scattered positions were saved to localStorage, and now the arrows are correctly connecting those positions - but it looks wrong because the nodes aren't in the expected layout.

## Quick Fix - Clear Saved Positions

Paste this in your browser console (F12 → Console):

```javascript
// Clear all flowchart positions for interface 8, message type ADT^A01
localStorage.removeItem('flowchart_positions_8_ADT^A01');

// Reload the page
location.reload();
```

Then switch back to flowchart mode to see the clean auto-layout.

## Alternative - Clear All Flowchart Positions

If you want to reset ALL flowchart positions for all interfaces:

```javascript
// Find and remove all flowchart position data
Object.keys(localStorage)
    .filter(k => k.startsWith('flowchart_positions_'))
    .forEach(k => {
        console.log('Removing:', k);
        localStorage.removeItem(k);
    });

// Reload
location.reload();
```

## What You'll See After Clearing

After clearing and reloading, you should see:
- All 9 steps in a clean vertical line (auto-layout)
- Boxes centered at X = 400
- Evenly spaced (120px apart)
- Arrows connecting from bottom of one box to top of next
- Smooth Bezier curves

## Why This Happened

The flowchart saves your manual positions to localStorage so they persist across page refreshes. This is a **feature** - it remembers where you dragged nodes.

But in your case, the old positions from testing are causing confusion. Once cleared, you'll start fresh with the auto-layout.

## Expected Auto-Layout Positions

After clearing, you should see these positions in the console:

```
📦 Rendering node for Field Validation at: {x: 400, y: 80, width: 140, height: 90}
📦 Rendering node for API Enrichment at: {x: 400, y: 200, width: 140, height: 90}
📦 Rendering node for database_enrichment_postgres at: {x: 400, y: 320, width: 140, height: 90}
📦 Rendering node for database_enrichment_sqlserver at: {x: 400, y: 440, width: 140, height: 90}
📦 Rendering node for database_enrichment at: {x: 400, y: 560, width: 140, height: 90}
📦 Rendering node for Field Mapping at: {x: 400, y: 680, width: 140, height: 90}
📦 Rendering node for Script Enrichment at: {x: 400, y: 800, width: 140, height: 90}
📦 Rendering node for HL7→FHIR Transform at: {x: 400, y: 920, width: 140, height: 90}
📦 Rendering node for If-Then-Else at: {x: 400, y: 1040, width: 140, height: 90}
```

All at X = 400 (centered), evenly spaced vertically.
