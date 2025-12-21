# Pipeline Builder - Quick Start Guide

## 🚀 Getting Started (5 Minutes)

### Prerequisites
- ezHealthKonnect system running
- Node.js backend on port 3000
- Go backend on port 8080
- PostgreSQL database with V21 migration applied

---

## Step 1: Access Pipeline Builder

### From Interfaces Page (Recommended)
1. Navigate to `http://localhost:3000/interfaces.html`
2. Find any interface in the list
3. Click the **🔀** (Configure Pipeline) button
4. Pipeline builder opens automatically

### Direct Access
```
http://localhost:3000/pipeline-builder.html?interfaceId=INT_001&messageType=ADT^A01
```

---

## Step 2: Build Your First Pipeline (2 Minutes)

### Add Pre-Processing Step
1. **Left Panel**: Find "Validate Required Fields" template
2. **Drag** the template card
3. **Drop** in the **Pre-Processing** layer (blue section)
4. ✅ Step appears in canvas

### Add Core Transformation
1. **Left Panel**: Find "HL7 to FHIR Mapping" template
2. **Drag & Drop** in **Core Transformation** layer (yellow section)
3. ✅ Transformation step added

### Add Post-Processing
1. **Left Panel**: Find "Validate FHIR Bundle" template
2. **Drag & Drop** in **Post-Processing** layer (green section)
3. ✅ Validation step added

**Result**: You now have a complete 3-step pipeline!

```
┌─────────────────────────────────────┐
│ PRE-PROCESSING                      │
│ ┌─────────────────────────────────┐ │
│ │ ✓ Validate Required Fields      │ │
│ └─────────────────────────────────┘ │
└─────────────────────────────────────┘
              ↓
┌─────────────────────────────────────┐
│ CORE TRANSFORMATION                 │
│ ┌─────────────────────────────────┐ │
│ │ 🔀 HL7 to FHIR Mapping          │ │
│ └─────────────────────────────────┘ │
└─────────────────────────────────────┘
              ↓
┌─────────────────────────────────────┐
│ POST-PROCESSING                     │
│ ┌─────────────────────────────────┐ │
│ │ 🛡️ Validate FHIR Bundle         │ │
│ └─────────────────────────────────┘ │
└─────────────────────────────────────┘
```

---

## Step 3: Configure a Step (1 Minute)

1. **Click** on any step in the canvas
2. **Right Panel** opens with properties
3. Modify:
   - **Name**: Change step name
   - **Timeout**: Set to 5000ms
   - **Error Strategy**: Choose "Skip" or "Fail"
   - **Configuration**: Edit JSON config
4. **Click Save** button
5. ✅ Step updated

---

## Step 4: Test Pipeline (1 Minute)

1. Click **Test** button in header
2. Paste sample HL7 message:
   ```
   MSH|^~\&|EPIC|EPICADT|SMS|SMSADT|199912271408|CHARRIS|ADT^A01|1817457|D|2.5|
   PID||0493575^^^2^ID 1|454721||DOE^JOHN^^^^|DOE^JOHN^^^^|19480203|M||B|254 MYSTREET AVE^^MYTOWN^OH^44123^USA||(216)123-4567|||M|NON|400003403~1129086|
   ```
3. Click **Run Test**
4. View results:
   - ✅ Success/Failure status
   - ⏱️ Execution time
   - 📋 Steps executed
   - 🔍 Detailed output

---

## Step 5: Save Pipeline (30 Seconds)

1. Click **Save Pipeline** button in header
2. Wait for "All changes saved" indicator
3. ✅ Pipeline saved to database

**Auto-save**: Pipeline automatically saves every 30 seconds

---

## 🎨 Advanced Features

### Execution Modes

#### Parallel Execution
- Click **Parallel** button in canvas toolbar
- Next steps added will execute concurrently
- Use for: Independent validation rules, multiple enrichments

#### Sequential Execution (Default)
- Click **Sequential** button in canvas toolbar
- Steps execute one after another
- Use for: Dependent transformations, ordered processing

### Reordering Steps

**Within Same Layer**:
1. Drag step up or down
2. Drop at new position
3. Steps reorder

**Between Layers**:
1. Drag step from one layer
2. Drop in different layer
3. Step moves

### Duplicating Steps

1. Click step to select
2. Click **📋** (Duplicate) button on step
3. Copy appears with "(Copy)" suffix
4. Configure the copy

### Custom Scripts

1. **Left Panel** → **Custom Scripts** section
2. Click "**Add Custom Script**"
3. Script editor opens in properties panel
4. Write JavaScript:
   ```javascript
   function transform(input) {
       // Access parsed HL7 data
       var segments = input.enhancedSegments;
       var pid = segments.PID;

       // Your custom logic
       if (pid.fields.find(f => f.key === "PID.5").value.includes("VIP")) {
           input._metadata.priority = "high";
       }

       return input;
   }
   ```
5. Save step

---

## 🔧 Canvas Controls

### Zoom
- **Zoom In**: Click 🔍+ button or Ctrl+Plus
- **Zoom Out**: Click 🔍- button or Ctrl+Minus
- **Reset**: Click ⊡ button
- **Current**: Shows percentage (50% - 200%)

### Auto Layout
- Click **✨ Auto Layout** button
- Steps automatically organized
- Connections redrawn

### Clear Canvas
- Click **🗑️ Clear Canvas** button
- Confirms before clearing
- Removes all steps (cannot be undone)

---

## 💡 Tips & Tricks

### 1. Use Templates First
- Start with built-in templates
- Customize configurations
- Create custom scripts only when needed

### 2. Organize by Layers
- **Pre-Processing**: Validation, enrichment, data cleanup
- **Core Transformation**: HL7→FHIR mapping
- **Post-Processing**: FHIR validation, delivery, logging

### 3. Test Frequently
- Test after each major change
- Use real sample messages
- Verify error handling

### 4. Name Steps Clearly
- Use descriptive names: "Validate Patient ID" not "Validation"
- Include version info if needed: "FHIR R4 Patient Mapper"
- Add description for complex steps

### 5. Configure Error Handling
- **Fail**: Stop pipeline on error (default)
- **Skip**: Continue pipeline, log error
- **Default**: Use fallback value

### 6. Monitor Auto-Save
- Check "All changes saved" indicator
- Don't navigate away while "Saving..."
- Manual save available anytime

---

## 🐛 Troubleshooting

### Problem: Templates Not Showing
**Solution**:
1. Check browser console for errors
2. Verify API connection: `GET /api/templates`
3. Refresh page (Ctrl+R)

### Problem: Drag & Drop Not Working
**Solution**:
1. Ensure browser supports HTML5 drag-drop
2. Check if step card has `draggable="true"`
3. Try different browser

### Problem: Pipeline Not Saving
**Solution**:
1. Check browser console for errors
2. Verify backend connection
3. Check PostgreSQL V21 migration applied
4. Validate pipeline JSON format

### Problem: Test Fails
**Solution**:
1. Verify sample message format (valid HL7)
2. Check step configurations
3. Review error message in test results
4. Test steps individually

### Problem: Steps Not Visible After Drop
**Solution**:
1. Check if dropped in correct layer
2. Scroll in canvas area
3. Click zoom reset
4. Check browser console

---

## 🎯 Common Workflows

### Workflow 1: Simple HL7→FHIR Pipeline
```
1. Validate Required Fields (Pre)
2. HL7 to FHIR Mapping (Core)
3. Validate FHIR Bundle (Post)
4. Save & Test
```

### Workflow 2: Pipeline with Enrichment
```
1. Validate Patient ID (Pre)
2. Enrich from Epic API (Pre - Parallel)
3. Enrich from Lab System (Pre - Parallel)
4. HL7 to FHIR Mapping (Core)
5. Validate FHIR Bundle (Post)
6. Save & Test
```

### Workflow 3: Custom Transformation
```
1. Validate Message Type (Pre)
2. Custom VIP Detection Script (Pre)
3. HL7 to FHIR Mapping (Core)
4. Custom PHI Anonymization Script (Post)
5. Validate FHIR Bundle (Post)
6. Deliver to FHIR Server (Post)
7. Save & Test
```

---

## 📋 Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Escape` | Deselect step / Close properties |
| `Ctrl+S` | Manual save (same as Save button) |
| `Delete` | Delete selected step (with confirmation) |
| `Ctrl +` | Zoom in |
| `Ctrl -` | Zoom out |
| `Ctrl 0` | Reset zoom |

---

## 🔗 Integration Points

### From Interfaces Page
- Click 🔀 button on any interface
- Automatically loads existing pipeline
- Context (interfaceId, messageType) preserved

### Return to Interfaces
- Click ← Back button in header
- Warns if unsaved changes
- Returns to interfaces list

### Edit Interface While Building
- Open edit modal for interface
- Modify source/target settings
- Pipeline configuration separate

---

## 📖 Next Steps

### Learn More
- Read full documentation: `PIPELINE_BUILDER_IMPLEMENTATION.md`
- Review architecture: `TRANSFORMATION_PIPELINE_DESIGN.md`
- API reference: `SYSTEM_DOCUMENTATION.md`

### Advanced Topics
- Execution groups and dependencies
- Custom executor development
- Template marketplace
- Performance optimization

### Get Help
- Check browser console for errors
- Review integration guide: `INTEGRATION_GUIDE.md`
- Contact support team

---

## ✅ Quick Checklist

Before going live:
- [ ] V21 database migration applied
- [ ] Pipeline routes registered in app.js
- [ ] Go backend controllers initialized
- [ ] Templates loading correctly
- [ ] Can create and save pipeline
- [ ] Test execution works
- [ ] Interface integration working
- [ ] Auto-save functioning

---

**Time to First Pipeline**: ~5 minutes
**Complexity**: Beginner-friendly
**Support**: Full drag-and-drop with visual feedback

Happy pipeline building! 🎉
