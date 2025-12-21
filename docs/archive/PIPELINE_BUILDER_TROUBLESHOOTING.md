# Pipeline Builder - Troubleshooting & FAQ

## ✅ Issue Fixed: Empty Left Panel

### What Was Wrong
The left panel showed section headers but no template cards because:
- Built-in templates were missing `isSystem: true` property
- Rendering function filtered templates by `isSystem` flag
- Result: No templates matched the filter

### What Was Fixed
✅ Added `isSystem: true` to all 5 built-in templates
✅ Added 2 more templates (Enrich, Deliver) for better coverage

**File Modified**: `public/js/pipeline/managers/ToolboxManager.js`

### Now You Should See

After refreshing the page, left panel should show:

```
🔧 Components

🔍 [Search box]

⭐ TEMPLATES
┌─────────────────────────────┐
│ ✅ Validate Required Fields │
│ Validates required HL7...   │
│ [Built-in]                  │
└─────────────────────────────┘
┌─────────────────────────────┐
│ ➕ Enrich Patient Data      │
│ Enrich from external...     │
│ [Built-in]                  │
└─────────────────────────────┘
[... more templates ...]

🔵 PRE-PROCESSING
(Templates specific to Pre layer)

🟡 TRANSFORMATION
(Templates specific to Core layer)

🟢 POST-PROCESSING
(Templates specific to Post layer)

📝 CUSTOM SCRIPTS
┌─────────────────────────────┐
│ ➕ Add Custom Script        │
└─────────────────────────────┘
```

---

## 🎯 Understanding the Three Panes

### LEFT: Toolbox (Your "Parts Drawer")

**Purpose**: Browse and select transformation components

**What to do here**:
1. **Browse** available templates
2. **Search** for specific steps
3. **Drag** components you want to use
4. **Drop** them in center canvas

**Think of it as**: IKEA catalog - you browse what's available

---

### CENTER: Canvas (Your "Assembly Area")

**Purpose**: Build your transformation pipeline visually

**What you see**:
- **Three horizontal layers** (Pre, Core, Post)
- **Steps** you've added to each layer
- **Visual connections** showing data flow

**What to do here**:
1. **Drop** components from left panel
2. **Arrange** steps in desired order
3. **Click** steps to configure them
4. **Drag** steps to reorder or move between layers

**Think of it as**: IKEA assembly area - you put pieces together

---

### RIGHT: Properties (Your "Instruction Manual")

**Purpose**: Configure how each step behaves

**What you see**:
- Empty by default ("Select a step to configure")
- Fills with configuration form when you click a step

**What to do here**:
1. **Click** a step in center canvas
2. **Edit** step name, timeout, error handling
3. **Configure** step-specific settings (JSON)
4. **Write** custom JavaScript (if custom script step)
5. **Save** changes

**Think of it as**: IKEA instruction manual - tells you how to adjust each piece

---

## 🔗 Linking to Existing Transformation Mapping

### Your Current Setup

You already have transformation mappings in the database:

**Table**: `interface_message_mappings`
**Columns**:
- `interface_id`: e.g., "INT_001"
- `message_type`: e.g., "ADT^A01"
- `transformation_mapping`: JSON with MSH→MessageHeader, PID→Patient, etc.

### How Pipeline Builder Uses It

**The "HL7 to FHIR Mapping" template** in the toolbox is a **wrapper** around your existing mapping:

```javascript
// When you drag "HL7 to FHIR Mapping" template to canvas
Step {
  name: "HL7 to FHIR Mapping",
  type: "core.mapping",
  config: {
    use_template: true,     // ← KEY SETTING!
    interface_id: "INT_001", // ← Your interface
    message_type: "ADT^A01"  // ← Your message type
  }
}
```

**At runtime**, when this step executes:

1. **Looks up** your existing `transformation_mapping` from database
2. **Applies** that mapping (your existing logic!)
3. **Returns** FHIR output

**You don't lose anything** - your existing mappings are reused!

---

## 🎬 Step-by-Step: Building Your First Pipeline

### Scenario
You have an ADT^A01 interface and want to:
1. Validate message before transforming
2. Use your existing HL7→FHIR mapping
3. Validate FHIR output

### Steps

#### 1. Access Pipeline Builder
```
Interfaces Page → Find ADT^A01 interface → Click 🔀 button
```

#### 2. Add Validation (Pre-Processing)
```
LEFT PANEL:
  Find "Validate Required Fields" template

ACTION:
  Click and hold the card
  Drag to CENTER CANVAS → PRE-PROCESSING layer (blue section)
  Release mouse

RESULT:
  ✅ Step appears in Pre-Processing layer
```

#### 3. Add Your Existing Mapping (Core)
```
LEFT PANEL:
  Find "HL7 to FHIR Mapping" template

ACTION:
  Drag to CENTER CANVAS → CORE TRANSFORMATION layer (yellow section)

RESULT:
  ✅ Step appears in Core layer
```

#### 4. Configure the Mapping Step
```
CENTER CANVAS:
  Click on "HL7 to FHIR Mapping" step

RIGHT PANEL (opens automatically):
  Shows configuration form

EDIT:
  Config (JSON):
  {
    "use_template": true,        // Uses your existing mapping!
    "fhir_version": "R4",
    "interface_id": "INT_001",   // Your interface ID
    "message_type": "ADT^A01"    // Your message type
  }

SAVE:
  Click "Save" button in properties panel
```

#### 5. Add FHIR Validation (Post-Processing)
```
LEFT PANEL:
  Find "Validate FHIR Bundle" template

ACTION:
  Drag to CENTER CANVAS → POST-PROCESSING layer (green section)

RESULT:
  ✅ Step appears in Post layer
```

#### 6. Save Pipeline
```
HEADER:
  Click "SAVE PIPELINE" button (top right)

RESULT:
  ✅ Pipeline saved to database
  ✅ "All changes saved" indicator shows
```

#### 7. Test Pipeline
```
HEADER:
  Click "TEST" button

MODAL OPENS:
  Paste sample HL7 message:

  MSH|^~\&|EPIC|EPICADT|SMS|SMSADT|199912271408|CHARRIS|ADT^A01|1817457|D|2.5|
  PID||0493575^^^2^ID 1|454721||DOE^JOHN^^^^|DOE^JOHN^^^^|19480203|M||B|

  Click "RUN TEST"

RESULT:
  Shows execution results:
  ✅ Step 1: Validate Required Fields - PASSED
  ✅ Step 2: HL7 to FHIR Mapping - PASSED
  ✅ Step 3: Validate FHIR Bundle - PASSED
```

---

## 🔍 Common Questions Answered

### Q1: "I see empty canvas - is something broken?"
**A**: No! Empty canvas means you haven't added any steps yet. This is normal.

**What to do**: Drag templates from left panel to canvas.

---

### Q2: "Where is my existing transformation mapping?"
**A**: It's still in the database! Pipeline Builder doesn't delete it.

**To use it**:
1. Drag "HL7 to FHIR Mapping" template to Core layer
2. Configure it with your interface_id and message_type
3. Set `use_template: true`

---

### Q3: "What if I don't want pre/post processing?"
**A**: That's fine! Just add the HL7→FHIR mapping step to Core layer and nothing else.

**Minimal pipeline**:
```
PRE: (empty)
CORE: HL7 to FHIR Mapping (uses your existing mapping)
POST: (empty)
```

---

### Q4: "Can I edit the transformation mapping JSON?"
**A**: Yes! Two ways:

**Option 1** (Direct DB edit - existing way):
- Edit `transformation_mapping` in `interface_message_mappings` table
- Pipeline Builder will use updated mapping

**Option 2** (Future - not yet implemented):
- Visual mapping editor inside Pipeline Builder
- Planned for Phase 2

---

### Q5: "What's the difference between parallel and sequential?"
**A**: Execution order of steps.

**Sequential** (default):
```
Step 1 completes → Step 2 starts → Step 2 completes → Step 3 starts
```

**Parallel**:
```
Step 1, Step 2, Step 3 all start at same time
Wait for all to complete → Next layer
```

**When to use parallel**:
- Multiple validations that don't depend on each other
- Enriching from multiple APIs simultaneously
- Delivering to multiple destinations

---

### Q6: "How do I delete a step?"
**A**: Three ways:

1. **Click step** → Click **🗑️** (trash icon) on step card
2. **Click step** → Press **Delete** key on keyboard
3. **Right-click step** → Select "Delete" (not yet implemented)

---

### Q7: "Can I reorder steps?"
**A**: Yes!

**Within same layer**: Click and drag step up or down

**Between layers**: Drag step from one layer and drop in another

---

### Q8: "What's the properties panel for?"
**A**: Configuring step behavior.

**When to use**:
- Change step name: "Validate" → "Validate Patient ID"
- Set timeout: 5000ms
- Configure error handling: Fail vs Skip
- Edit step-specific config: Validation rules, API endpoints, etc.
- Write custom JavaScript: For custom script steps

---

### Q9: "I accidentally closed properties panel - how do I reopen?"
**A**: Click on any step in the canvas. Properties panel will show that step's config.

---

### Q10: "Can multiple people edit same pipeline?"
**A**: Not yet - only single-user editing supported currently.

**Workaround**: Coordinate with team on who's editing when.

**Future**: Multi-user collaboration in Phase 3.

---

## 🐛 Debugging Checklist

### Problem: Templates not showing in left panel

**Check**:
1. ✅ Refresh browser (Ctrl+R or F5)
2. ✅ Open browser console (F12)
3. ✅ Look for errors in console
4. ✅ Check network tab - is `/api/templates` being called?
5. ✅ Verify file `ToolboxManager.js` was updated

**If still broken**:
- Hard refresh: Ctrl+Shift+R
- Clear cache
- Check browser console for specific error message

---

### Problem: Can't drag templates

**Check**:
1. ✅ Are template cards visible?
2. ✅ Does cursor change to "move" on hover?
3. ✅ Browser supports HTML5 drag-drop? (Chrome, Firefox, Edge, Safari)
4. ✅ Check console for JavaScript errors

**Try**:
- Use different browser
- Disable browser extensions
- Check if JavaScript is enabled

---

### Problem: Properties panel not showing config

**Check**:
1. ✅ Is a step selected? (Should have blue glow)
2. ✅ Click directly on step card (not on action buttons)
3. ✅ Check console for errors

**Try**:
- Click on a different step
- Refresh page
- Check if step has valid configuration

---

### Problem: Save button not working

**Check**:
1. ✅ Is pipeline valid? (At least one step)
2. ✅ Check browser console for errors
3. ✅ Is backend running? (Node.js on :3000, Go on :8080)
4. ✅ Network tab - is POST `/api/pipelines` being called?

**Try**:
- Check backend logs
- Verify database connection
- Test with simpler pipeline (1 step only)

---

### Problem: Test fails

**Check**:
1. ✅ Is sample message valid HL7?
2. ✅ Are all steps configured correctly?
3. ✅ Check error message in test results

**Try**:
- Test with known-good HL7 message
- Remove steps one by one to isolate issue
- Check step configurations

---

## 📚 Additional Resources

### Documentation Files
1. **PIPELINE_BUILDER_EXPLAINED.md** - Detailed concept explanation
2. **PIPELINE_BUILDER_VISUAL_GUIDE.md** - Visual walkthrough with diagrams
3. **PIPELINE_BUILDER_QUICKSTART.md** - 5-minute getting started guide
4. **PIPELINE_BUILDER_IMPLEMENTATION.md** - Technical documentation

### Architecture References
1. **TRANSFORMATION_PIPELINE_DESIGN.md** - Pipeline architecture design
2. **SYSTEM_DOCUMENTATION.md** - Complete system documentation
3. **INTEGRATION_GUIDE.md** - Integration steps for main.go and app.js

---

## 🎓 Next Steps

### Immediate (After Fix)
1. ✅ Refresh browser
2. ✅ Verify templates appear in left panel
3. ✅ Try dragging one template to canvas
4. ✅ Click step to see properties panel
5. ✅ Save pipeline

### Short-term (This Week)
1. Build your first complete pipeline (Pre + Core + Post)
2. Test with real HL7 messages
3. Configure step properties
4. Try parallel vs sequential execution

### Long-term (Next Month)
1. Migrate all interfaces to use Pipeline Builder
2. Create custom scripts for specific logic
3. Build template library for common patterns
4. Share pipelines across team

---

**Status**: Left panel issue FIXED ✅
**Action Required**: Refresh your browser to see templates
**Next**: Try dragging a template to canvas!
