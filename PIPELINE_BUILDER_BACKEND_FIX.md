# Pipeline Builder - Backend Routes Fix

## 🐛 Issue: API Routes Not Registered

**Error in Browser Console**:
```
GET http://localhost:3000/api/templates 404 (Not Found)
GET http://localhost:3000/api/pipelines/interface/... 404 (Not Found)
```

**Root Cause**: The pipeline routes were created (`routes/pipelineRoutes.js`) but never registered in `app.js`.

---

## ✅ Fix Applied

**File Modified**: `app.js`

**Added** (after line 263):
```javascript
// Pipeline Builder routes (proxies to Go backend)
console.log('🔄 Mounting /api/pipelines...');
const pipelineRoutes = require('./routes/pipelineRoutes');
app.use('/api', pipelineRoutes);
console.log('✅ Pipeline routes mounted successfully');
```

This registers all pipeline-related routes:
- `GET /api/templates` - List available templates
- `POST /api/pipelines` - Save pipeline
- `GET /api/pipelines/:id` - Load pipeline by ID
- `GET /api/pipelines/interface/:id/:messageType` - Load by interface
- `POST /api/pipelines/test` - Test pipeline
- ... and more

---

## 🚀 Action Required: RESTART SERVER

**IMPORTANT**: The routes won't be active until you restart the Node.js server.

### Option 1: If using npm start
```bash
# Press Ctrl+C to stop
# Then restart:
npm start
```

### Option 2: If using nodemon
```bash
# Should auto-restart, but if not:
# Press Ctrl+C and run:
npm run dev
```

### Option 3: If running directly
```bash
# Press Ctrl+C to stop
# Then restart:
node server.js
```

---

## 📊 What Will Happen After Restart

### Server Startup Logs
You should see these new lines in your server console:
```
🔄 Mounting /api/auth...
🔄 Mounting /api/users...
🔄 Mounting /api/interfaces...
🔄 Mounting /api/wizard...
🔄 Mounting /api/messages...
🔄 Mounting /api/pipelines...    ← NEW!
✅ Pipeline routes mounted successfully  ← NEW!
✅ Essential routes mounted successfully
```

### Browser Behavior
After server restart and browser refresh:

**Before** (404 errors):
```
❌ GET /api/templates 404 (Not Found)
❌ GET /api/pipelines/interface/... 404 (Not Found)
```

**After** (fallback still works, but cleaner):
```
✅ GET /api/templates 200 OK (or uses fallback gracefully)
✅ GET /api/pipelines/interface/... 200 OK (or creates new pipeline)
```

---

## 🎯 Expected Results

### If Go Backend is Running
**Best case**: All API calls work, data loads from database
- Templates load from Go backend
- Pipelines load from database
- Save/test operations work

### If Go Backend is NOT Running
**Fallback case**: Built-in templates still work
- Frontend uses built-in templates (5 templates)
- Can still build pipelines visually
- Save won't work until Go backend is running

---

## 🔍 Verification Steps

After restarting server and refreshing browser:

### 1. Check Server Console
Look for:
```
✅ Pipeline routes mounted successfully
```

### 2. Check Browser Console
Should see EITHER:
- No errors (if Go backend running)
- OR cleaner fallback messages (if Go backend not running)

### 3. Check Left Panel
**Should now show templates** regardless of backend status (fallback works!)

---

## 🎨 Templates That Should Appear

Even without Go backend (using fallback):

1. ✅ **Validate Required Fields** (Pre-Processing)
2. ➕ **Enrich Patient Data** (Pre-Processing)
3. 🔀 **HL7 to FHIR Mapping** (Core Transformation)
4. 🛡️ **Validate FHIR Bundle** (Post-Processing)
5. 📤 **Deliver to FHIR Server** (Post-Processing)

---

## 🔧 Troubleshooting

### Problem: Still getting 404 after restart
**Possible causes**:
1. Server didn't restart properly
2. Cache issue

**Solutions**:
```bash
# Fully stop server
Ctrl+C

# Wait 2 seconds

# Start again
npm start

# Hard refresh browser
Ctrl+Shift+R
```

### Problem: "Cannot find module './routes/pipelineRoutes'"
**Cause**: File doesn't exist

**Solution**: File was created in Phase 1B, but verify:
```bash
ls routes/pipelineRoutes.js
```

If missing, the file was not created properly.

### Problem: Templates still don't show
**Check**:
1. Is left panel completely empty? → JavaScript issue
2. Are section headers visible but no cards? → Template rendering issue

**Debug**:
```javascript
// In browser console:
window.pipelineBuilder.toolboxManager.templates
// Should show array of 5 templates
```

---

## 📋 Next Steps After Restart

1. ✅ Restart Node.js server
2. ✅ Refresh browser (Ctrl+Shift+R)
3. ✅ Check left panel has template cards
4. ✅ Try dragging a template to canvas
5. ✅ Click step to configure
6. ✅ Test save pipeline

---

**RESTART YOUR SERVER NOW!** 🚀
