# Browser Cache Refresh Guide

## The Problem

The error you're seeing is from **OLD cached JavaScript** in your browser. The stack trace shows:

```
StepNodeManager.js?v=4.0:115  ← OLD version (now v5.0)
PropertiesPanel.js?v=8.3:1398 ← OLD version (now v9.0)
```

The infinite loop fix has been applied to the code, but your browser is running the old cached version.

## Quick Fix - Hard Refresh

### Windows/Linux
1. **Chrome/Edge**: Press `Ctrl + Shift + F5` or `Ctrl + Shift + R`
2. **Firefox**: Press `Ctrl + Shift + R`

### Mac
1. **Chrome/Edge**: Press `Cmd + Shift + R`
2. **Firefox**: Press `Cmd + Shift + R`
3. **Safari**: Press `Cmd + Option + R`

## Nuclear Option - Clear All Cache

If hard refresh doesn't work:

### Chrome/Edge
1. Press `F12` to open DevTools
2. Right-click the refresh button (while DevTools is open)
3. Select **"Empty Cache and Hard Reload"**

### Firefox
1. Press `Ctrl + Shift + Delete`
2. Select "Cached Web Content"
3. Click "Clear Now"
4. Refresh the page

## Verify New Code is Loaded

After clearing cache, check the browser console. You should see:

### Version Check
The error stack trace should show NEW version numbers:
```
StepNodeManager.js?v=5.0    ← NEW (was v4.0)
PropertiesPanel.js?v=9.0    ← NEW (was v8.3)
FieldPathInput.js?v=4.0     ← NEW (was v3.0)
ValidationRuleBuilder.js?v=4.0 ← NEW (was v3.0)
```

### No More Infinite Loop
You should NOT see:
```
❌ Maximum call stack size exceeded
❌ at StepNodeManager.deselectNode
❌ at PropertiesPanel.hideProperties
```

### FieldPathInput Loaded
You SHOULD see in console when adding validation:
```
✅ [ValidationRuleBuilder] Initializing field selectors...
✅ [FieldPathInput] Initialized with mode: path
```

You should NOT see:
```
❌ [ValidationRuleBuilder] FieldPathInput component not loaded!
```

## Version Numbers Updated

The following files now have updated version numbers:

| File | Old Version | New Version |
|------|-------------|-------------|
| StepNodeManager.js | v4.0 | v5.0 |
| PropertiesPanel.js | v8.3 | v9.0 |
| FieldPathInput.js | v3.0 | v4.0 |
| ValidationRuleBuilder.js | v3.0 | v4.0 |

## Testing After Cache Clear

1. Open: `http://localhost:3000/pipeline-builder.html`
2. Open browser DevTools Console (F12)
3. Add a validation step
4. Click to configure the step
5. Add a validation rule

### Expected Behavior

**Field Path Input Should Appear**:
```
┌─────────────────────────────────────────────────┐
│                                        │ Path  │
└─────────────────────────────────────────────────┘
Searching by: Field Path
```

**Toggle Button Should Work**:
- Click toggle button
- Text should change from "Path" to "Desc"
- Placeholder should change

**No Crashes**:
- Clicking steps should work smoothly
- Deselecting steps should work
- No infinite loop errors
- No out of memory errors

## Why This Happened

### Browser Caching Behavior

Browsers aggressively cache JavaScript files to improve performance. Even with:

```html
<meta http-equiv="Cache-Control" content="no-cache, no-store, must-revalidate">
```

Some browsers (especially Chrome) will still use cached JavaScript files.

### Version Query Parameters

We use version query parameters (`?v=5.0`) to force cache refresh:

```html
<script src="/js/pipeline/managers/StepNodeManager.js?v=5.0"></script>
```

When the version changes from `v4.0` → `v5.0`, the browser treats it as a completely different file and downloads it fresh.

**However**: If the HTML itself is cached, the browser won't see the new version numbers!

### Solution

1. **Hard refresh** clears HTML cache → sees new version numbers → downloads new JavaScript
2. **Empty cache** clears everything → guaranteed fresh load

## If Still Not Working

### Check Network Tab

1. Open DevTools (F12)
2. Go to "Network" tab
3. Filter by "JS"
4. Refresh page
5. Look for the JavaScript files

**Check the version numbers in URLs**:
```
✅ StepNodeManager.js?v=5.0
✅ PropertiesPanel.js?v=9.0
✅ FieldPathInput.js?v=4.0
```

If you still see old versions (v4.0, v8.3, v3.0), the HTML is still cached.

### Force No-Cache Headers (Server-Side)

If the problem persists, we may need to add no-cache headers to the Express server:

```javascript
// In server.js or app.js
app.use((req, res, next) => {
    if (req.url.endsWith('.js') || req.url.endsWith('.html')) {
        res.setHeader('Cache-Control', 'no-store, no-cache, must-revalidate, proxy-revalidate');
        res.setHeader('Pragma', 'no-cache');
        res.setHeader('Expires', '0');
    }
    next();
});
```

## Summary

🔄 **Version numbers updated** in pipeline-builder.html
🚫 **Cache must be cleared** for changes to take effect
✅ **Hard refresh (Ctrl+Shift+F5)** should fix it
⚠️ **If not, use "Empty Cache and Hard Reload"**

After clearing cache, all issues should be resolved! 🎉
