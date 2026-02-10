# Flowchart V2 - Quick Testing Guide

## 🚀 What's New?

You now have a **brand new horizontal flowchart** with swim lanes! This replaces the old vertical layout with a professional, space-efficient visualization.

---

## 🎯 How to Test

### Step 1: Load the Pipeline Builder

1. Open your browser
2. Navigate to **Interface 8** (the one with 9 steps)
3. Click the **Pipeline Builder** button

### Step 2: Switch to Flowchart View

1. Look for the **view toggle** button (List/Flowchart)
2. Click **Flowchart**

### Step 3: What You Should See

**Expected Visual**:
```
┌─────────────────────────────────────────────────┐
│  🔵 PRE-PROCESSING                              │
│  ┌───┐  ┌───┐  ┌───┐                           │
│  │ 1 │→ │ 2 │→ │ 3 │                           │
│  └───┘  └───┘  └───┘                           │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│  🟢 CORE TRANSFORMATION                         │
│  ┌───┐  ┌───┐  ┌───┐  ┌───┐                    │
│  │ 4 │→ │ 5 │→ │ 6 │→ │ 7 │                    │
│  └───┘  └───┘  └───┘  └───┘                    │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│  🟣 POST-PROCESSING                             │
│  ┌───┐  ┌───┐                                   │
│  │ 8 │→ │ 9 │                                   │
│  └───┘  └───┘                                   │
└─────────────────────────────────────────────────┘
```

**Key Visual Elements**:
- ✅ **3 horizontal swim lanes** (Pre, Core, Post)
- ✅ **Colored headers** (Navy blue, Green, Purple)
- ✅ **Compact step boxes** with pink borders
- ✅ **Arrows** connecting boxes left-to-right
- ✅ **Sequence numbers** in blue badges
- ✅ **Step icons** (✅, 🔍, 🔄, ⚙️, etc.)

---

## 🖱️ Interactions to Test

### 1. Drag a Step Node

**Action**: Click and drag any step box

**Expected**:
- Box moves smoothly with your cursor
- Arrows redraw automatically to stay connected
- Performance is smooth (60fps)

### 2. Pan the Canvas

**Action**: Click and drag on **empty space** (not on a step box)

**Expected**:
- Entire canvas moves (like panning a map)
- All steps and lanes move together

### 3. Zoom In/Out

**Action**: Use **mouse wheel** while hovering over canvas

**Expected**:
- Scroll up = zoom in
- Scroll down = zoom out
- Zoom targets your cursor position (like Google Maps)

### 4. Hover Over a Step

**Action**: Move mouse over a step box (don't click)

**Expected**:
- Border changes from pink to **navy blue**
- Box shadow becomes more prominent
- Box slightly lifts up (translateY effect)

---

## 🔍 Console Checks

### 1. Open Browser Console

**Windows/Linux**: `F12` or `Ctrl+Shift+I`
**Mac**: `Cmd+Option+I`

### 2. Look for These Messages

**On Page Load**:
```
✅ Using Flowchart V2 (Horizontal Swim Lanes)
✅ CoordinateSystem initialized
✅ HorizontalLayoutEngine initialized
✅ FlowchartCanvas initialized
✅ FlowchartOrchestratorV2 initialized
```

**On Switching to Flowchart View**:
```
📊 Found 9 steps across all layers
📐 Calculating horizontal layout for 9 steps
📊 Steps by layer: {pre: 3, core: 4, post: 2}
🏊 Swim lanes calculated: {pre: {...}, core: {...}, post: {...}}
🔗 Created 8 connections
✅ Layout calculated: {positions: 9, connections: 8, swimLanes: 3}
🎨 FlowchartCanvas rendering... {steps: 9, positions: 9, connections: 8}
✅ Flowchart V2 rendered successfully
```

**On Dragging**:
```
🖱️ Started dragging step: <step-id>
✅ Finished dragging step: <step-id>
```

**On Panning**:
```
🖱️ Started panning canvas
✅ Finished panning canvas
```

**On Zooming**:
```
🔍 Zoom: 1.10x
🔍 Zoom: 1.21x
🔍 Zoom: 0.90x
```

### 3. Check for Errors

**❌ If you see**:
```
⚠️ No pipeline or layers found
📊 Found 0 steps across all layers
```

**Solution**: Refresh the page - this was the timing issue we fixed

---

## 🐛 Common Issues & Fixes

### Issue 1: Empty Flowchart (No Steps)

**Symptom**: Blank canvas when switching to flowchart

**Fix**: Refresh the page (F5). If persists, check console for errors.

### Issue 2: Arrows Not Connected

**Symptom**: Arrows floating disconnected from boxes

**Fix**: This was the V1 bug - V2 shouldn't have this. If you see it, let me know!

### Issue 3: Can't See All Steps

**Symptom**: Steps cut off or require scrolling

**Try**: Use mouse wheel to zoom out, or drag canvas to pan

**Note**: V2 should fit all 9 steps without scrolling on 1920×1080 screen

### Issue 4: Laggy Dragging

**Symptom**: Choppy performance when dragging nodes

**Check**: Open console and look for excessive log messages

**Note**: V2 throttles redraws to 60fps - should be smooth

---

## 🔄 Rollback to V1 (If Needed)

If you prefer the old vertical layout, you can easily switch back:

**Step 1**: Open `public/js/pipeline/PipelineBuilder.js`

**Step 2**: Find line 79:
```javascript
const useV2 = true; // Set to true for V2
```

**Step 3**: Change to:
```javascript
const useV2 = false; // Set to false to use V1
```

**Step 4**: Refresh the page

---

## 📊 Comparison: V1 vs V2

| Feature | V1 (Old) | V2 (New) |
|---------|----------|----------|
| **Layout Direction** | Vertical (↓) | Horizontal (→) |
| **Screen Space** | Requires scrolling | Fits in viewport |
| **Swim Lanes** | No | Yes (3 layers) |
| **Connection Style** | Bezier curves | Straight arrows |
| **Coordinate System** | Buggy (40px offset) | Fixed from start |
| **Performance** | Laggy dragging | Smooth 60fps |
| **Color Scheme** | Generic blue/gray | ezHealthKonnect branding |
| **Professional Look** | Basic | Pentaho-inspired |

---

## 🎨 Design Details

### Colors

**Swim Lane Headers** (brand colors):
- 🔵 Pre-Processing: Navy Blue `#1e3a8a`
- 🟢 Core Transformation: Green `#059669`
- 🟣 Post-Processing: Purple `#7c3aed`

**Step Nodes**:
- Border: Pastel Pink `#f8bbd9` (signature ezHealthKonnect color)
- Background: White
- Hover: Navy Blue border `#1e3a8a`

### Dimensions

- **Step Box**: 160px × 80px
- **Column Gap**: 80px
- **Row Gap**: 40px
- **Max Steps Per Column**: 4 steps

---

## 📝 Feedback Checklist

After testing, please note:

### Visual Design
- [ ] Do the swim lanes look clear and organized?
- [ ] Are the colors visually appealing?
- [ ] Is the layout intuitive (left→right flow)?
- [ ] Can you see all 9 steps without scrolling?

### Interactions
- [ ] Is dragging smooth and responsive?
- [ ] Does panning feel natural?
- [ ] Does zooming work as expected?
- [ ] Are hover effects noticeable?

### Performance
- [ ] Does the flowchart load quickly?
- [ ] Are interactions smooth (no lag)?
- [ ] Does it feel "professional"?

### Functionality
- [ ] Are arrows properly connected to boxes?
- [ ] Do sequence numbers show correctly?
- [ ] Do step icons display?
- [ ] Can you switch between List and Flowchart?

---

## 🚀 Next Features (Coming Soon)

After you test and approve Phase 1, I'll implement:

### Phase 2: Fork Detection
- Detect If-Then-Else steps
- Spread TRUE/FALSE branches horizontally
- Show conditional routing visually

### Phase 3: Smart Routing
- Replace straight arrows with 90° angle routing
- Avoid overlapping other boxes
- Add TRUE/FALSE labels on fork branches

### Phase 4: Advanced Interactions
- Snap-to-grid when dragging
- Save manual positions
- Keyboard shortcuts (Ctrl+0 for fit-to-screen)

### Phase 5: Polish
- Minimap for navigation
- Zoom buttons (+/-)
- Export as PNG/SVG
- Smooth animations

---

## 📞 Questions?

If you encounter any issues or have feedback, note:

1. **What you were doing** (e.g., "dragging step 5")
2. **What you expected** (e.g., "arrow should stay connected")
3. **What actually happened** (e.g., "arrow disappeared")
4. **Console output** (copy any error messages)
5. **Screenshot** (if visual issue)

---

## ✅ Quick Test Checklist

Run through this quick test:

1. [ ] Load Interface 8
2. [ ] Switch to Flowchart view
3. [ ] Verify 9 steps appear in 3 swim lanes
4. [ ] Drag a step box → arrows redraw
5. [ ] Drag empty space → canvas pans
6. [ ] Scroll wheel → zooms in/out
7. [ ] Hover step → border changes to navy
8. [ ] Check console for success messages
9. [ ] Switch back to List view → works
10. [ ] Switch back to Flowchart → renders again

---

**Last Updated**: 2024-12-28
**Version**: V2 Phase 1
**Status**: Ready for Testing ✅
