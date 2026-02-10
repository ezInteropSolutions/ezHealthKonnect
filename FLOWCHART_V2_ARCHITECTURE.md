# Flowchart V2 - Modern Architecture Design

## Overview
Complete redesign of the flowchart system with horizontal-first layout, swim lanes, and professional-grade rendering.

## Architecture Layers

### 1. Coordinate System (Foundation)
```javascript
class FlowchartCoordinateSystem {
    // Unified world coordinates
    // Handle zoom/pan transformations
    // Convert between screen space and world space

    worldToScreen(x, y) { }
    screenToWorld(x, y) { }
    applyTransform(zoom, panX, panY) { }
}
```

### 2. Layout Engine (Brain)
```javascript
class HorizontalLayoutEngine {
    // Calculate positions for horizontal flow
    // Swim lane allocation (Pre/Core/Post)
    // Grid-based positioning (columns)
    // Fork detection and branching

    calculateLayout(steps) {
        // Group by layer
        // Arrange in columns (3-4 steps per column)
        // Calculate swim lane Y positions
        // Return { positions, connections, bounds }
    }
}
```

### 3. Connection Router (Smart Paths)
```javascript
class OrthogonalRouter {
    // Manhattan routing (90° angles only)
    // Obstacle avoidance
    // Minimal bends
    // Connection points on box edges

    route(fromBox, toBox, obstacles) {
        // A* pathfinding with orthogonal constraints
        // Return array of waypoints
    }
}
```

### 4. Canvas Renderer (Drawing)
```javascript
class FlowchartCanvas {
    // HTML5 Canvas for connections
    // DOM elements for step nodes
    // Layered rendering (background → connections → nodes)

    render(layout, zoom, pan) {
        // Clear and redraw
        // Apply transformations
        // Handle viewport culling
    }
}
```

### 5. Interaction Controller (UX)
```javascript
class FlowchartInteraction {
    // Drag nodes with snap-to-grid
    // Pan canvas (drag background)
    // Zoom (mouse wheel + buttons)
    // Click handlers

    enableDrag(node) { }
    enablePan() { }
    enableZoom() { }
}
```

## Visual Layout

### Horizontal Swim Lanes
```
┌────────────────────────────────────────────────────────────────┐
│  🔵 PRE-PROCESSING                                             │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│   [1] ──→ [2] ──→ [3]                                         │
│   Val    API    DB                                            │
│                                                                │
└────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────┐
│  🟢 CORE TRANSFORMATION                                        │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│   [4] ──→ [5] ──→ [6]                                         │
│   Map   HL7→   Script                                         │
│              FHIR                                              │
│                                                                │
└────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────┐
│  🟣 POST-PROCESSING                                            │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│   [7] ──→ [8] ──→ [9]                                         │
│  Validate Anon  Send                                          │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### Grid System
- **Column width**: 200px (step width 160px + 40px gap)
- **Row height**: 120px (step height 80px + 40px gap)
- **Swim lane height**: Auto-calculated based on max rows in layer
- **Snap-to-grid**: When dragging, snap to nearest grid position

## Connection Routing

### Orthogonal (Manhattan) Routing
```
From Box A to Box B:

      A ──┐
          │
          └──→ B

Not Bezier curves:
      A
       ╲
        ╲
         ╲→ B
```

### Routing Algorithm
1. **Start point**: Right edge of source box
2. **End point**: Left edge of target box
3. **Path finding**:
   - If same Y level → straight horizontal line
   - If different Y → right → down/up → right
   - Avoid overlapping other boxes
4. **Optimization**: Minimize bends

### Example Paths
```
Same row:
[A] ──────→ [B]

Different rows:
[A] ──┐
      │
      └────→ [B]

Around obstacle:
[A] ──┐
      │
      ├───→ [C]
      │
      └─────→ [B]
```

## Color Scheme (ezHealthKonnect Branding)

### Swim Lane Headers
- **Pre-Processing**: `#1e3a8a` (Navy blue)
- **Core Transformation**: `#059669` (Green)
- **Post-Processing**: `#7c3aed` (Purple)

### Step Nodes
- **Border**: `#f8bbd9` (Pastel pink)
- **Background**: White gradient
- **Hover**: Navy blue border
- **Selected**: Navy blue background with white text

### Connections
- **Sequential**: `#64748b` (Slate gray)
- **Fork (TRUE)**: `#22c55e` (Green)
- **Fork (FALSE)**: `#ef4444` (Red)
- **Merge**: `#64748b` (Slate gray)

## Component Structure

### File Organization
```
public/js/pipeline/flowchart-v2/
├── core/
│   ├── CoordinateSystem.js          # Zoom/pan transforms
│   ├── ViewportManager.js           # Canvas bounds, culling
│   └── EventBus.js                  # Component communication
│
├── layout/
│   ├── HorizontalLayoutEngine.js    # Main layout logic
│   ├── GridAllocator.js             # Grid positioning
│   ├── SwimLaneCalculator.js        # Layer heights
│   └── ForkDetector.js              # Branch analysis
│
├── rendering/
│   ├── FlowchartCanvas.js           # Main renderer
│   ├── OrthogonalRouter.js          # Connection paths
│   ├── StepNodeRenderer.js          # DOM node rendering
│   └── ConnectionRenderer.js        # Canvas connection drawing
│
├── interaction/
│   ├── DragController.js            # Node dragging
│   ├── PanController.js             # Canvas panning
│   ├── ZoomController.js            # Zoom controls
│   └── SelectionManager.js          # Click/select handling
│
└── FlowchartOrchestrator.js         # Main coordinator
```

### CSS Organization
```
public/css/flowchart-v2.css
├── Canvas layout
├── Swim lane styles
├── Step node styles (compact horizontal)
├── Connection styles
├── Controls (zoom, minimap)
└── Responsive breakpoints
```

## Key Features

### 1. Smart Auto-Layout
- **Column allocation**: 3-4 steps per column max
- **Swim lane sizing**: Auto-height based on content
- **Fork spreading**: Branches spread horizontally
- **Merge convergence**: Paths rejoin cleanly

### 2. Interactive Controls
- **Minimap** (bottom-right): Overview of entire flow
- **Zoom controls** (top-right): +/- buttons, fit-to-screen
- **Reset layout** button: Return to auto-layout
- **View mode toggle**: Horizontal ⟷ Vertical (future)

### 3. Performance Optimizations
- **Viewport culling**: Only render visible nodes
- **Connection caching**: Pre-calculate paths
- **Transform matrix**: GPU-accelerated canvas transforms
- **Throttled redraws**: 60fps max during drag

### 4. Accessibility
- **Keyboard navigation**: Arrow keys to move between steps
- **Keyboard shortcuts**:
  - `Ctrl + 0`: Fit to screen
  - `Ctrl + +/-`: Zoom in/out
  - `Spacebar`: Pan mode (drag anywhere)
  - `Esc`: Deselect all

## Implementation Phases

### Phase 1: Foundation (2-3 hours)
- [x] CoordinateSystem class
- [x] HorizontalLayoutEngine skeleton
- [x] FlowchartCanvas setup
- [x] Basic rendering (boxes only)

### Phase 2: Layout Logic (3-4 hours)
- [x] Grid allocation algorithm
- [x] Swim lane calculation
- [x] Step positioning in columns
- [x] Test with 9 steps

### Phase 3: Connections (2-3 hours)
- [x] OrthogonalRouter implementation
- [x] A* pathfinding with 90° constraints
- [x] Connection rendering on Canvas
- [x] Arrow heads and labels

### Phase 4: Interactions (2-3 hours)
- [x] Drag with snap-to-grid
- [x] Pan canvas
- [x] Zoom (wheel + buttons)
- [x] Click to select

### Phase 5: Polish (2 hours)
- [x] Minimap component
- [x] Fit-to-screen button
- [x] Animations (smooth zoom/pan)
- [x] Tooltips on hover

### Phase 6: Integration (1 hour)
- [x] Replace old FlowchartRenderer
- [x] Migration path for existing users
- [x] Testing with real pipelines

## Total Estimated Time: 12-15 hours

## Success Metrics

After implementation, we should have:
- ✅ All 9 steps visible without scrolling (on 1920x1080)
- ✅ Arrows connecting box-to-box with no floating
- ✅ Smooth drag-and-drop with grid snapping
- ✅ Professional appearance (like Pentaho/Azure)
- ✅ Fast performance (60fps interactions)
- ✅ Zero external dependencies
- ✅ Clean, maintainable code

## Next Step

Start with Phase 1: Build the foundation classes.
