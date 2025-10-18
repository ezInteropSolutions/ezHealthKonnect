# Pipeline Builder - Visual Guide

## 🖼️ Screen Layout

```
┌────────────────────────────────────────────────────────────────────────────────┐
│  ← BACK    Pipeline Builder: ADT^A01 Pipeline          [TEST] [SAVE PIPELINE] │ HEADER
├──────────────┬─────────────────────────────────────────────┬──────────────────┤
│              │  🔍+ 🔍- ⊡  100%   [⚡Parallel] [→Sequential] │                  │
│  🔧 TOOLBOX  ├─────────────────────────────────────────────┤   ⚙️ PROPERTIES  │
│              │                                             │                  │
│ 🔍 Search... │  ┌────────────────────────────────────┐   │  Select a step   │
│              │  │  🔵 PRE-PROCESSING LAYER           │   │  to configure    │
│ ⭐ TEMPLATES │  │  Drag validation/enrichment here    │   │  its properties  │
│ • Validate   │  │                                     │   │                  │
│ • Enrich     │  │  (Empty - drag steps here)         │   │                  │
│ • Map HL7    │  │                                     │   │                  │
│              │  └────────────────────────────────────┘   │                  │
│ 🔵 PRE       │              ↓ (Data flows down)          │                  │
│ • Validate   │  ┌────────────────────────────────────┐   │                  │
│ • Enrich     │  │  🟡 CORE TRANSFORMATION LAYER      │   │                  │
│              │  │  Drag HL7→FHIR mapping here         │   │                  │
│ 🟡 CORE      │  │                                     │   │                  │
│ • HL7→FHIR   │  │  (Empty - drag steps here)         │   │                  │
│ • Custom Map │  │                                     │   │                  │
│              │  └────────────────────────────────────┘   │                  │
│ 🟢 POST      │              ↓                             │                  │
│ • Validate   │  ┌────────────────────────────────────┐   │                  │
│ • Deliver    │  │  🟢 POST-PROCESSING LAYER          │   │                  │
│              │  │  Drag validation/delivery here      │   │                  │
│ 📝 CUSTOM    │  │                                     │   │                  │
│ • Add Script │  │  (Empty - drag steps here)         │   │                  │
│              │  └────────────────────────────────────┘   │                  │
│              │                                             │                  │
└──────────────┴─────────────────────────────────────────────┴──────────────────┘
   LEFT (280px)              CENTER (flexible)                   RIGHT (320px)
```

---

## 📱 Step-by-Step Visual Walkthrough

### STEP 1: Initial View (Empty Canvas)

```
LEFT PANEL                  CENTER CANVAS                    RIGHT PANEL
─────────────               ──────────────                   ────────────

🔧 Components               ┌─────────────────────┐          ℹ️ Info
                            │  PRE-PROCESSING     │
⭐ Templates                │                     │          "Select a step
┌─────────────────┐         │  Drop steps here    │          to configure
│ ✅ Validate     │         │                     │          properties"
│ Required Fields │         └─────────────────────┘
└─────────────────┘                   ↓
┌─────────────────┐         ┌─────────────────────┐
│ 🔀 HL7→FHIR    │         │  CORE TRANSFORM     │
│ Mapping         │         │                     │
└─────────────────┘         │  Drop steps here    │
┌─────────────────┐         │                     │
│ 🛡️ Validate    │         └─────────────────────┘
│ FHIR Bundle     │                   ↓
└─────────────────┘         ┌─────────────────────┐
                            │  POST-PROCESSING    │
🔵 Pre-Processing           │                     │
• Validate                  │  Drop steps here    │
• Enrich                    │                     │
                            └─────────────────────┘
```

---

### STEP 2: Dragging a Template

```
LEFT PANEL                  CENTER CANVAS                    RIGHT PANEL
─────────────               ──────────────                   ────────────

🔧 Components               ┌─────────────────────┐          ℹ️ Info
                            │  PRE-PROCESSING     │
⭐ Templates                │    ↙ DROP HERE      │          "Select a step
┌─────────────────┐         │   ╔═══════════════╗ │          to configure
│ ✅ Validate     │         │   ║ Validate      ║ │ ← DRAGGING
│ Required Fields │─────────┼──>║ Required      ║ │          properties"
└─────────────────┘         │   ║ Fields        ║ │
                            │   ╚═══════════════╝ │
                            └─────────────────────┘
```

---

### STEP 3: After Dropping Step

```
LEFT PANEL                  CENTER CANVAS                    RIGHT PANEL
─────────────               ──────────────                   ────────────

🔧 Components               ┌─────────────────────┐          ℹ️ Info
                            │  PRE-PROCESSING     │
⭐ Templates                │  ┌───────────────┐  │          "Select a step
┌─────────────────┐         │  │ ✅ Validate   │  │          to configure
│ ✅ Validate     │         │  │ Required      │  │ ← STEP ADDED!
│ Required Fields │         │  │ Fields   [⚙️🗑️]│  │          properties"
└─────────────────┐         │  └───────────────┘  │
                            └─────────────────────┘
Now try dragging                      ↓
more steps!             ┌─────────────────────┐
                        │  CORE TRANSFORM     │
                        │                     │
                        │  Drop mapping here  │
                        └─────────────────────┘
```

---

### STEP 4: Clicking a Step (Shows Properties)

```
LEFT PANEL                  CENTER CANVAS                    RIGHT PANEL
─────────────               ──────────────                   ────────────

🔧 Components               ┌─────────────────────┐          ⚙️ Configuration
                            │  PRE-PROCESSING     │
⭐ Templates                │  ┌───────────────┐  │          Step: Validate
┌─────────────────┐         │  │ ✅ Validate   │  │          Required Fields
│ ✅ Validate     │         │  │ Required      │  │ ← SELECTED!
│ Required Fields │         │  │ Fields   [⚙️🗑️]│  │          ┌─────────────┐
└─────────────────┐         │  └───────────────┘  │          │ Name:       │
                            └─────────────────────┘          │ [Validate...│
🔵 Pre-Processing                     ↓                      │             │
• Validate                ┌─────────────────────┐          │ Timeout:    │
• Enrich                  │  CORE TRANSFORM     │          │ [5000] ms   │
                          │                     │          │             │
                          │  (empty)            │          │ Config:     │
                          └─────────────────────┘          │ {...}       │
                                                            │             │
                                                            │ [SAVE]      │
                                                            └─────────────┘
```

---

### STEP 5: Complete Pipeline

```
LEFT PANEL                  CENTER CANVAS                    RIGHT PANEL
─────────────               ──────────────                   ────────────

🔧 Components               ┌─────────────────────┐          ⚙️ Configuration
                            │  PRE-PROCESSING     │
⭐ Templates                │  ┌───────────────┐  │          Step: HL7→FHIR
┌─────────────────┐         │  │ ✅ Validate   │  │          Mapping
│ ✅ Validate     │         │  │ Required      │  │
│ Required Fields │         │  │ Fields        │  │          ┌─────────────┐
└─────────────────┐         │  └───────────────┘  │          │ FHIR Ver:   │
┌─────────────────┐         │  ┌───────────────┐  │          │ [R4]        │
│ 🔀 HL7→FHIR    │         │  │ ➕ Enrich     │  │          │             │
│ Mapping         │         │  │ Patient Data  │  │          │ Use Template│
└─────────────────┘         │  └───────────────┘  │          │ [✓] Yes     │
┌─────────────────┐         └─────────────────────┘          │             │
│ 🛡️ Validate    │                   ↓                      │ Interface:  │
│ FHIR Bundle     │         ┌─────────────────────┐          │ [INT_001]   │
└─────────────────┘         │  CORE TRANSFORM     │          │             │
                            │  ┌───────────────┐  │          │ Msg Type:   │
🔵 Pre-Processing           │  │ 🔀 HL7→FHIR   │  │ ← SELECTED│ [ADT^A01]  │
• Validate                  │  │ Mapping       │  │          │             │
• Enrich                    │  └───────────────┘  │          │ [SAVE]      │
                            └─────────────────────┘          └─────────────┘
🟡 Core                               ↓
• HL7→FHIR                  ┌─────────────────────┐
                            │  POST-PROCESSING    │
🟢 Post                     │  ┌───────────────┐  │
• Validate FHIR             │  │ 🛡️ Validate   │  │
• Deliver                   │  │ FHIR Bundle   │  │
                            │  └───────────────┘  │
                            │  ┌───────────────┐  │
                            │  │ 📤 Deliver    │  │
                            │  │ to FHIR       │  │
                            │  └───────────────┘  │
                            └─────────────────────┘
```

---

## 🎨 Color Coding

```
┌─────────────────────────────────────────┐
│  🔵 PRE-PROCESSING (Blue)              │  Sequence 0-99
│  Validation, Enrichment, Cleanup        │
└─────────────────────────────────────────┘
            ↓
┌─────────────────────────────────────────┐
│  🟡 CORE TRANSFORMATION (Yellow)       │  Sequence 100-199
│  HL7→FHIR Mapping, Main Logic          │
└─────────────────────────────────────────┘
            ↓
┌─────────────────────────────────────────┐
│  🟢 POST-PROCESSING (Green)            │  Sequence 200-299
│  Validation, Delivery, Logging          │
└─────────────────────────────────────────┘
```

---

## 🖱️ Mouse Interactions

### Dragging from Toolbox
```
1. Hover over template card      → Cursor changes to 'move'
2. Click and hold                → Card becomes semi-transparent
3. Drag to canvas layer          → Layer highlights (blue border)
4. Release mouse                 → Step appears in canvas ✅
```

### Clicking a Step
```
1. Click step in canvas          → Step highlighted (blue glow)
2. Properties panel updates      → Shows step configuration
3. Edit properties               → Modify name, config, etc.
4. Click "Save"                  → Changes applied ✅
```

### Reordering Steps
```
1. Click and hold step           → Step becomes draggable
2. Drag up or down               → Other steps shift
3. Release                       → Step reordered ✅
```

### Moving Between Layers
```
1. Drag step from Pre layer      → Step lifts
2. Drag to Core layer            → Core layer highlights
3. Release                       → Step moves to Core ✅
```

---

## 📋 Action Buttons on Steps

```
┌──────────────────────────────────┐
│ ✅ Validate Required Fields      │
│                         [⚙️][📋][🗑️]│
└──────────────────────────────────┘
                         │  │  │
                         │  │  └─ Delete step
                         │  └──── Duplicate step
                         └─────── Configure (same as clicking step)
```

---

## 🎛️ Toolbar Controls

```
┌─────────────────────────────────────────────────────────┐
│ [🔍+] [🔍-] [⊡] 100%    [⚡Parallel] [→Sequential]     │
│   │     │    │    │         │            │             │
│   │     │    │    │         │            └─ Sequential mode
│   │     │    │    │         └───────────── Parallel mode
│   │     │    │    └─────────────────────── Zoom level
│   │     │    └──────────────────────────── Reset zoom
│   │     └───────────────────────────────── Zoom out
│   └─────────────────────────────────────── Zoom in
└─────────────────────────────────────────────────────────┘
```

---

## 💾 Saving & Testing

### Save Button
```
Header: [TEST] [SAVE PIPELINE]
                      │
                      └─ Saves entire pipeline to database
                         Auto-saves every 30 seconds
```

### Test Button
```
Header: [TEST] [SAVE PIPELINE]
          │
          └─ Opens test modal:
             ┌────────────────────────────┐
             │ 🧪 Test Pipeline           │
             ├────────────────────────────┤
             │ Sample HL7 Message:        │
             │ ┌────────────────────────┐ │
             │ │MSH|^~\&|EPIC|...       │ │
             │ │PID||0493575^^^2...     │ │
             │ └────────────────────────┘ │
             │                            │
             │          [RUN TEST]        │
             └────────────────────────────┘
```

---

## 🔄 Execution Flow Visualization

### Sequential Execution (Default)
```
Step 1: Validate
    ↓ (waits for completion)
Step 2: Enrich
    ↓ (waits for completion)
Step 3: Map
    ↓ (waits for completion)
Done
```

### Parallel Execution
```
        ┌─ Step 1: Validate ─┐
        │                     │
Start ──┼─ Step 2: Enrich   ──┼─→ All complete → Next layer
        │                     │
        └─ Step 3: Check    ─┘
```

---

## 📱 Responsive Behavior

### Desktop (>1200px)
```
┌─────────┬────────────────┬──────────┐
│ Toolbox │     Canvas     │Properties│
│ 280px   │   (flexible)   │  320px   │
└─────────┴────────────────┴──────────┘
```

### Tablet (768-1200px)
```
┌─────────┬────────────────┬──────────┐
│ Toolbox │     Canvas     │Properties│
│ 240px   │   (flexible)   │  280px   │
└─────────┴────────────────┴──────────┘
```

### Mobile (<768px)
```
┌──────────────────────────────────────┐
│           Canvas                     │
├──────────────────────────────────────┤
│           Toolbox (tabs)             │
├──────────────────────────────────────┤
│           Properties (tabs)          │
└──────────────────────────────────────┘
```

---

**Refresh your browser and you should now see the template cards!**
