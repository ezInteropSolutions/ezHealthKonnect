# Reset Flowchart Positions - Updated Command

## The Current Issue

Your flowchart is loading with the FULL interface ID key. The localStorage key is:
```
flowchart_positions_762aebb9-0408-4a42-82c5-202f13f28315_ADT^A01
```

## Command to Clear (Copy and Paste)

**Paste this entire block into your browser console** (F12 → Console):

```javascript
// Clear the correct key with full interface ID
localStorage.removeItem('flowchart_positions_762aebb9-0408-4a42-82c5-202f13f28315_ADT^A01');

// Also clear the short ID version (just in case)
localStorage.removeItem('flowchart_positions_8_ADT^A01');

// Verify it's cleared
console.log('Remaining flowchart keys:',
    Object.keys(localStorage).filter(k => k.startsWith('flowchart_positions_'))
);

// Reload
location.reload();
```

## What You'll See After Reload

The console should show clean auto-layout positions:

```
📦 Rendering node for Field Validation at: {x: 400, y: 80, width: 140, height: 90}
📦 Rendering node for API Enrichment at: {x: 400, y: 200, width: 140, height: 90}
📦 Rendering node for database_enrichment_postgres at: {x: 400, y: 320, width: 140, height: 90}
... all at X = 400
```

And visually you'll see all 9 boxes in a perfect vertical line with arrows connecting them!

## Alternative - Clear ALL Flowchart Data

If you want to start completely fresh:

```javascript
// Nuclear option - clear all flowchart positions
Object.keys(localStorage)
    .filter(k => k.startsWith('flowchart_'))
    .forEach(k => {
        console.log('Clearing:', k);
        localStorage.removeItem(k);
    });

location.reload();
```

## Why This Keeps Happening

Every time you **drag a node**, the position is automatically saved to localStorage. This is intentional - it's a feature so your manual layout survives page refreshes.

But right now, the old scattered positions are causing confusion. Once you clear them, you'll start with the clean auto-layout.

**After clearing**, if you drag nodes to customize the layout, those new positions will be saved and remembered!
