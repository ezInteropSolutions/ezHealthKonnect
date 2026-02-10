# Pentaho-Inspired Flowchart Design for Pipeline Builder

## Problem Statement

**Current Issues:**
1. ❌ Steps are too large (full width cards with padding)
2. ❌ Sequential layout doesn't show flow forks/branches
3. ❌ Can't see entire pipeline at a glance
4. ❌ No visual distinction for conditional routing
5. ❌ Drag-and-drop between layers is confusing

**User Reference:** Pentaho Data Integration (Kettle/Spoon)
- Small icon-based step boxes (100x60px approximate)
- Clear visual connectors showing data flow
- Fork/branch visualization for conditional logic
- Compact, scannable layout

---

## Proposed Solution: Flowchart Mode

### Visual Comparison

**Before (Current):**
```
┌─────────────────────────────────────────┐
│  📝 Step 1: Validate Patient ID         │
│  Validation - Sequence 10                │
│  [Configure] [Delete]                    │
└─────────────────────────────────────────┘
        ↓
┌─────────────────────────────────────────┐
│  🔀 Step 2: If-Then-Else                │
│  Control Flow - Sequence 20              │
│  [Configure] [Delete]                    │
└─────────────────────────────────────────┘
```

**After (Proposed):**
```
     ┌─────────┐
     │  📝 10  │  Validate
     │ Validate│  Patient ID
     └────┬────┘
          │
     ┌────▼────┐
     │  🔀 20  │  If-Then
     │If-Then  │  VIP Check
     └─┬───┬───┘
       │   └──────────┐
       │              │
  ┌────▼────┐    ┌───▼────┐
  │  ⚙️ 30  │    │  ⚙️ 40  │
  │ Standard│    │   VIP   │
  │ Mapping │    │ Mapping │
  └────┬────┘    └────┬────┘
       │              │
       └──────┬───────┘
              │
         ┌────▼────┐
         │  ✅ 50  │
         │Validate │
         │  FHIR   │
         └─────────┘
```

---

## Design Specifications

### 1. Compact Step Boxes

**Size:**
- Width: 120px (fixed)
- Height: 80px (fixed)
- Padding: 8px
- Border-radius: 8px

**Layout:**
```
┌──────────────────┐
│ 🔀 SEQ           │  ← Icon + Sequence number (top-left)
│                  │
│   Step Name      │  ← Centered, 2-line max, ellipsis
│   (max 2 lines)  │
│                  │
│ [⚙️] [❌]        │  ← Action buttons (bottom-right)
└──────────────────┘
```

**CSS:**
```css
.step-node-compact {
    width: 120px;
    height: 80px;
    padding: 8px;
    background: white;
    border: 2px solid #cbd5e1;
    border-radius: 8px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: space-between;
    cursor: pointer;
    transition: all 0.2s;
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
}

.step-node-compact:hover {
    border-color: #3b82f6;
    box-shadow: 0 4px 8px rgba(0, 0, 0, 0.15);
    transform: translateY(-2px);
}

.step-node-compact.has-routing {
    border-color: #8b5cf6;  /* Purple for conditional routing */
}

.step-sequence-badge {
    position: absolute;
    top: 4px;
    left: 4px;
    background: #1e3a8a;
    color: white;
    font-size: 10px;
    padding: 2px 6px;
    border-radius: 4px;
    font-weight: 600;
}

.step-name-compact {
    font-size: 11px;
    text-align: center;
    line-height: 1.2;
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    max-width: 100%;
}

.step-icon-large {
    font-size: 24px;
    margin-bottom: 4px;
}
```

### 2. Visual Connectors (SVG Lines)

**Types:**
1. **Sequential:** Straight vertical line
2. **Fork:** Y-shaped split (if-then-else)
3. **Merge:** Y-shaped join (after conditional paths)
4. **Skip:** Curved line (route to step X)

**SVG Rendering:**
```javascript
class FlowchartConnector {
    // Sequential connection
    drawSequential(fromNode, toNode) {
        const fromCenter = this.getNodeCenter(fromNode);
        const toCenter = this.getNodeCenter(toNode);

        return `
            <line
                x1="${fromCenter.x}"
                y1="${fromCenter.y}"
                x2="${toCenter.x}"
                y2="${toCenter.y}"
                stroke="#64748b"
                stroke-width="2"
                marker-end="url(#arrowhead)"
            />
        `;
    }

    // Fork connection (if-then-else)
    drawFork(fromNode, trueNode, falseNode) {
        const fromCenter = this.getNodeCenter(fromNode);
        const trueCenter = this.getNodeCenter(trueNode);
        const falseCenter = this.getNodeCenter(falseNode);

        // Calculate fork point (below fromNode)
        const forkY = fromCenter.y + 40;

        return `
            <!-- Main line to fork point -->
            <line
                x1="${fromCenter.x}"
                y1="${fromCenter.y}"
                x2="${fromCenter.x}"
                y2="${forkY}"
                stroke="#8b5cf6"
                stroke-width="2"
            />

            <!-- Left branch (TRUE) -->
            <path
                d="M ${fromCenter.x} ${forkY} L ${trueCenter.x} ${trueCenter.y}"
                stroke="#22c55e"
                stroke-width="2"
                fill="none"
                marker-end="url(#arrowhead-success)"
            />
            <text x="${trueCenter.x - 30}" y="${forkY + 20}"
                  font-size="11" fill="#22c55e" font-weight="600">TRUE</text>

            <!-- Right branch (FALSE) -->
            <path
                d="M ${fromCenter.x} ${forkY} L ${falseCenter.x} ${falseCenter.y}"
                stroke="#ef4444"
                stroke-width="2"
                fill="none"
                marker-end="url(#arrowhead-error)"
            />
            <text x="${falseCenter.x + 10}" y="${forkY + 20}"
                  font-size="11" fill="#ef4444" font-weight="600">FALSE</text>
        `;
    }

    // Merge connection (paths rejoining)
    drawMerge(leftNode, rightNode, toNode) {
        const leftCenter = this.getNodeCenter(leftNode);
        const rightCenter = this.getNodeCenter(rightNode);
        const toCenter = this.getNodeCenter(toNode);

        const mergeY = toCenter.y - 40;

        return `
            <!-- Left path to merge point -->
            <path
                d="M ${leftCenter.x} ${leftCenter.y} L ${toCenter.x} ${mergeY}"
                stroke="#64748b"
                stroke-width="2"
                fill="none"
            />

            <!-- Right path to merge point -->
            <path
                d="M ${rightCenter.x} ${rightCenter.y} L ${toCenter.x} ${mergeY}"
                stroke="#64748b"
                stroke-width="2"
                fill="none"
            />

            <!-- Merged line to target -->
            <line
                x1="${toCenter.x}"
                y1="${mergeY}"
                x2="${toCenter.x}"
                y2="${toCenter.y}"
                stroke="#64748b"
                stroke-width="2"
                marker-end="url(#arrowhead)"
            />
        `;
    }

    // Route to step (skip ahead)
    drawRouteToStep(fromNode, toNode) {
        const fromCenter = this.getNodeCenter(fromNode);
        const toCenter = this.getNodeCenter(toNode);

        // Curved path for skip connections
        const controlX = fromCenter.x + 100;
        const controlY = (fromCenter.y + toCenter.y) / 2;

        return `
            <path
                d="M ${fromCenter.x} ${fromCenter.y}
                   Q ${controlX} ${controlY}
                     ${toCenter.x} ${toNode.y}"
                stroke="#f59e0b"
                stroke-width="2"
                stroke-dasharray="5,5"
                fill="none"
                marker-end="url(#arrowhead-warning)"
            />
            <text x="${controlX}" y="${controlY}"
                  font-size="10" fill="#f59e0b" font-weight="600">ROUTE TO</text>
        `;
    }
}
```

### 3. Layout Algorithm

**Auto-Layout Strategy:**
1. Start at top of canvas
2. For each step:
   - If no conditional routing → place vertically below previous
   - If conditional routing (if-then-else) → detect fork
   - Calculate branch positions (left/right offset)
   - Detect merge points (where branches rejoin)

**Positioning:**
```javascript
class FlowchartLayoutEngine {
    constructor() {
        this.stepSpacing = 100;      // Vertical spacing
        this.branchSpacing = 150;    // Horizontal spacing for forks
        this.startX = 400;           // Center of canvas
        this.startY = 50;            // Top margin
    }

    calculateLayout(steps) {
        const positions = new Map();
        let currentY = this.startY;
        let currentX = this.startX;

        for (let i = 0; i < steps.length; i++) {
            const step = steps[i];

            // Check if step has conditional routing
            if (this.hasConditionalRouting(step)) {
                // This is a fork point
                positions.set(step.id, { x: currentX, y: currentY });
                currentY += this.stepSpacing;

                // Find TRUE and FALSE branches
                const branches = this.detectBranches(step, steps);

                if (branches.truePath.length > 0) {
                    // Position TRUE branch to the left
                    let branchY = currentY;
                    branches.truePath.forEach(branchStep => {
                        positions.set(branchStep.id, {
                            x: currentX - this.branchSpacing,
                            y: branchY
                        });
                        branchY += this.stepSpacing;
                    });
                }

                if (branches.falsePath.length > 0) {
                    // Position FALSE branch to the right
                    let branchY = currentY;
                    branches.falsePath.forEach(branchStep => {
                        positions.set(branchStep.id, {
                            x: currentX + this.branchSpacing,
                            y: branchY
                        });
                        branchY += this.stepSpacing;
                    });
                }

                // Find merge point (first step both branches connect to)
                const mergeStep = this.findMergePoint(branches, steps);
                if (mergeStep) {
                    currentY = Math.max(
                        positions.get(branches.truePath[branches.truePath.length - 1].id).y,
                        positions.get(branches.falsePath[branches.falsePath.length - 1].id).y
                    ) + this.stepSpacing;

                    positions.set(mergeStep.id, { x: currentX, y: currentY });
                }

                // Skip to end of branches
                i += branches.truePath.length + branches.falsePath.length;

            } else {
                // Sequential step
                positions.set(step.id, { x: currentX, y: currentY });
                currentY += this.stepSpacing;
            }
        }

        return positions;
    }

    hasConditionalRouting(step) {
        if (step.stepType !== 'control' || step.subType !== 'conditional') {
            return false;
        }

        const config = step.config;
        return config.conditions && config.conditions.some(cond =>
            (cond.ifTrue && cond.ifTrue.action === 'route_to_step') ||
            (cond.ifFalse && cond.ifFalse.action === 'route_to_step')
        );
    }

    detectBranches(forkStep, allSteps) {
        const config = forkStep.config;
        const truePath = [];
        const falsePath = [];

        // Find TRUE branch target
        const trueAction = config.conditions[0]?.ifTrue;
        if (trueAction && trueAction.action === 'route_to_step') {
            const targetStep = allSteps.find(s => s.id === trueAction.stepId);
            if (targetStep) truePath.push(targetStep);
        }

        // Find FALSE branch target
        const falseAction = config.conditions[0]?.ifFalse;
        if (falseAction && falseAction.action === 'route_to_step') {
            const targetStep = allSteps.find(s => s.id === falseAction.stepId);
            if (targetStep) falsePath.push(targetStep);
        }

        return { truePath, falsePath };
    }

    findMergePoint(branches, allSteps) {
        // Find first step after both branches that's sequential
        const lastTrueStep = branches.truePath[branches.truePath.length - 1];
        const lastFalseStep = branches.falsePath[branches.falsePath.length - 1];

        if (!lastTrueStep || !lastFalseStep) return null;

        const maxSequence = Math.max(lastTrueStep.sequence, lastFalseStep.sequence);

        return allSteps.find(s => s.sequence > maxSequence && !this.hasConditionalRouting(s));
    }
}
```

### 4. Canvas Mode Toggle

**Two View Modes:**

1. **List View (Current):** Full-width cards, good for configuration
2. **Flowchart View (New):** Compact boxes with connectors, good for visualization

**Toggle Button:**
```html
<div class="view-mode-toggle">
    <button id="listViewBtn" class="view-mode-btn active">
        <i class="fas fa-list"></i> List
    </button>
    <button id="flowchartViewBtn" class="view-mode-btn">
        <i class="fas fa-project-diagram"></i> Flowchart
    </button>
</div>
```

**Behavior:**
- **List View:** Steps displayed in layers (pre/core/post) as full-width cards
- **Flowchart View:** Steps displayed on freeform canvas with auto-layout
- User preference saved in localStorage
- Drag-and-drop works in both modes

---

## Implementation Plan

### Phase 1: Compact Step Rendering (Week 1)
1. Create `.step-node-compact` CSS class
2. Update `StepNodeManager.js` to support compact rendering
3. Add view mode toggle to toolbar
4. Implement localStorage persistence

**Files to Modify:**
- `public/css/pipeline-builder.css` - Add compact styles
- `public/js/pipeline/managers/StepNodeManager.js` - Dual rendering modes
- `public/pipeline-builder.html` - Add toggle button

### Phase 2: SVG Connector System (Week 2)
1. Create `FlowchartConnector.js` class
2. Implement arrow markers (SVG defs)
3. Add sequential connection rendering
4. Add visual feedback on hover (highlight path)

**New File:**
- `public/js/pipeline/utils/FlowchartConnector.js`

### Phase 3: Auto-Layout Engine (Week 2-3)
1. Create `FlowchartLayoutEngine.js` class
2. Implement fork detection algorithm
3. Implement branch positioning
4. Implement merge point detection
5. Add manual adjustment (drag nodes in flowchart mode)

**New File:**
- `public/js/pipeline/utils/FlowchartLayoutEngine.js`

### Phase 4: Conditional Flow Visualization (Week 3)
1. Detect if-then-else steps with routing
2. Render fork connectors (TRUE/FALSE labels)
3. Render merge connectors
4. Render skip connectors (route to step)
5. Add hover tooltips explaining routing logic

**Files to Modify:**
- `public/js/pipeline/utils/FlowchartConnector.js` - Add fork/merge rendering
- `public/js/pipeline/components/IfThenElseBuilder.js` - Export routing metadata

### Phase 5: Polish & UX (Week 4)
1. Add zoom controls (scale SVG + nodes together)
2. Add pan controls (drag canvas background)
3. Add minimap (thumbnail view in corner)
4. Add export as image (PNG/SVG)
5. Add animation (step execution visualization)

---

## Interaction Design

### Drag and Drop in Flowchart Mode

**Behavior:**
1. User drags step from toolbox → drop anywhere on canvas
2. System auto-assigns sequence based on Y position
3. System runs auto-layout to reposition all steps
4. Connectors redraw automatically

**Alternative: Manual Positioning**
- User can toggle "Free Layout" mode
- Steps stay where user places them
- Connectors still auto-draw based on sequence
- Good for complex pipelines with many branches

### Click Interactions

**Single Click:** Select step (highlight + show properties panel)
**Double Click:** Open configuration modal
**Right Click:** Context menu (configure, delete, duplicate, disable)
**Hover:** Show tooltip with step details

### Connection Interaction

**Hover on Connector:** Highlight path + show tooltip
```
Tooltip Example:
┌─────────────────────────┐
│ Sequential Flow         │
│ Step 10 → Step 20       │
│ Data passes through     │
└─────────────────────────┘
```

**Hover on Fork:** Highlight both branches
```
Tooltip Example:
┌─────────────────────────┐
│ Conditional Fork        │
│ IF: patient.vip = true  │
│ ✓ TRUE → Step 30        │
│ ✗ FALSE → Step 40       │
└─────────────────────────┘
```

---

## Color Coding System

### Step Types (Border Colors)
- 🟦 **Validation:** Blue (#3b82f6)
- 🟩 **Enrichment:** Green (#22c55e)
- 🟨 **Mapping:** Yellow (#f59e0b)
- 🟪 **Control Flow:** Purple (#8b5cf6)
- 🟥 **Custom Script:** Red (#ef4444)

### Connection Types (Line Colors)
- ⬛ **Sequential:** Gray (#64748b)
- 🟩 **TRUE Branch:** Green (#22c55e)
- 🟥 **FALSE Branch:** Red (#ef4444)
- 🟧 **Route To:** Orange (#f59e0b) + dashed
- 🟪 **Conditional:** Purple (#8b5cf6)

### Layer Background (Canvas Zones)
- 🔵 **Pre-Processing:** Light blue (#dbeafe)
- 🟡 **Core Transformation:** Light yellow (#fef3c7)
- 🟢 **Post-Processing:** Light green (#dcfce7)

---

## Example: VIP Patient Routing

**Pipeline:**
1. Validate Patient ID (seq 10)
2. If-Then-Else: Check VIP Status (seq 20)
   - TRUE → VIP Mapping Template (seq 30)
   - FALSE → Standard Mapping Template (seq 40)
3. Validate FHIR Bundle (seq 50)

**Flowchart Visualization:**
```
                     ┌─────────┐
                     │  📝 10  │
                     │Validate │
                     │Patient  │
                     └────┬────┘
                          │
                     ┌────▼────┐
                     │  🔀 20  │
                     │If-Then  │
                     │VIP Check│
                     └─┬───┬───┘
          TRUE         │   │         FALSE
          (green)      │   │         (red)
                  ┌────▼   ▼────┐
                  │              │
             ┌────┴────┐    ┌───┴─────┐
             │  ⚙️ 30  │    │  ⚙️ 40   │
             │   VIP   │    │ Standard │
             │ Mapping │    │ Mapping  │
             └────┬────┘    └────┬─────┘
                  │              │
                  └──────┬───────┘
                         │
                    ┌────▼────┐
                    │  ✅ 50  │
                    │Validate │
                    │  FHIR   │
                    └─────────┘
```

**Rendered in Flowchart Mode:**
- 5 compact boxes (120x80px each)
- 1 fork connector (purple Y-shape)
- 2 branch connectors (green TRUE, red FALSE)
- 1 merge connector (gray Y-join)
- Total canvas space: ~400x500px (vs 1200x800px in list mode)

---

## Benefits

### Space Efficiency
- **Before:** 5 steps = 1200px height (full cards)
- **After:** 5 steps = 500px height (compact boxes)
- **Savings:** 58% less vertical space

### Comprehension
- ✅ Instant understanding of flow logic
- ✅ Clear visualization of forks/branches
- ✅ Easy to spot routing complexity
- ✅ Better mental model of pipeline

### Usability
- ✅ Both modes available (list for config, flowchart for overview)
- ✅ Auto-layout reduces manual positioning
- ✅ Visual connectors eliminate ambiguity
- ✅ Pentaho-familiar UX for ETL users

---

## Open Questions for User

1. **Default View Mode:** Should we default to List or Flowchart view?
   - Recommendation: Flowchart (better UX)

2. **Manual vs Auto Layout:** Should users be able to manually position nodes in flowchart mode?
   - Recommendation: Auto-layout with manual override option

3. **Connection Labels:** Should we show labels on every connector or only on hover?
   - Recommendation: Labels on conditional forks only, hover tooltips for others

4. **Minimap:** Include minimap for large pipelines (10+ steps)?
   - Recommendation: Yes, show in bottom-right corner when zoom < 50%

5. **Export Options:** What formats? PNG, SVG, PDF?
   - Recommendation: PNG (screenshot) + SVG (scalable)

---

## Next Steps

Would you like me to:
1. ✅ **Implement Phase 1** (Compact rendering + view toggle)?
2. ✅ **Create prototype** (static HTML demo of flowchart view)?
3. ✅ **Refine design** (adjust sizes, colors, spacing)?
4. ✅ **Start implementation** (begin coding the solution)?

Please review this design and let me know:
- Does this match your vision from Pentaho?
- Any specific requirements for fork visualization?
- Preferred timeline for implementation?
