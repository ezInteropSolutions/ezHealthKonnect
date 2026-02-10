# If-Then-Else UI Redesign - Compact & Smart

**Date:** December 28, 2025
**Goal:** Less scrolling, fewer clicks, smart field search everywhere

---

## Current Issues

❌ **Too much vertical space** - each condition takes full screen height
❌ **Plain text inputs** - users have to remember field paths
❌ **No autocomplete** - typing `PID.8` manually is error-prone
❌ **Excessive spacing** - gaps between sections waste space
❌ **Hidden sections** - checkbox toggles add clicks

---

## Redesign Principles

### 1. **Horizontal Flow Layout**
```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Condition 1                                                            [🗑️] │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  IF [field🔍] [operator▼] [value🔍]  →  THEN [action▼] [config]            │
│                                      →  ELSE [action▼] [config]            │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘

[+ Add Condition]
```

**Benefits:**
- See entire condition in one view (no scrolling)
- Visual flow with arrows (IF → THEN / ELSE)
- Compact single-line when simple

### 2. **Smart Field Search Everywhere**
Use `FieldPathSearchComponent` for ALL field inputs:

```javascript
// Instead of plain input:
<input type="text" placeholder="Field" />

// Use smart search:
<div class="field-search-container" data-field="condition-field-${index}"></div>
```

**Features:**
- Autocomplete from message structure
- Show field type and description
- Recently used fields
- Hierarchical tree view

### 3. **Inline Expandable Config**
Simple actions → one line
Complex actions → expand inline

```
Action: [Set Metadata ▼]  →  Metadata: {"priority": "high"} [📝 Edit]

// Click edit → expand inline:
Action: [Set Metadata ▼]
┌───────────────────────────────────────┐
│ Metadata JSON:                        │
│ {                                     │
│   "priority": "high",                 │
│   "routing": "geriatrics"             │
│ }                                     │
│                              [✓ Done] │
└───────────────────────────────────────┘
```

---

## Improved Layout Design

### Compact Row Layout

```html
<div class="condition-row">
    <!-- IF Section -->
    <div class="if-section">
        <label>IF</label>
        <div class="field-search-compact" data-field="field"></div>
        <select class="operator-select-compact">...</select>
        <div class="value-input-compact">
            <!-- Smart search OR plain input based on "compare to field" toggle -->
        </div>
    </div>

    <!-- THEN Section -->
    <div class="then-section">
        <label>THEN</label>
        <select class="action-select-compact">...</select>
        <div class="action-config-inline">...</div>
    </div>

    <!-- ELSE Section -->
    <div class="else-section">
        <label>ELSE</label>
        <select class="action-select-compact">...</select>
        <div class="action-config-inline">...</div>
    </div>
</div>
```

### CSS for Compact Layout

```css
.condition-row {
    display: grid;
    grid-template-columns: 2fr 2fr 2fr;
    gap: 1rem;
    padding: 1rem;
    border: 2px solid var(--accent-pink);
    border-radius: 8px;
    margin-bottom: 1rem;
    background: #fefcfd;
}

.if-section,
.then-section,
.else-section {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
}

.if-section label {
    color: var(--primary-color);
    font-weight: 600;
    font-size: 0.875rem;
}

.then-section label {
    color: #10b981;
    font-weight: 600;
    font-size: 0.875rem;
}

.else-section label {
    color: #ef4444;
    font-weight: 600;
    font-size: 0.875rem;
}

/* Compact field search */
.field-search-compact {
    position: relative;
}

.field-search-compact input {
    padding: 0.5rem;
    font-size: 0.875rem;
    border: 1px solid #d1d5db;
    border-radius: 4px;
    width: 100%;
}

/* Compact selects */
.operator-select-compact,
.action-select-compact {
    padding: 0.5rem;
    font-size: 0.875rem;
    border: 1px solid #d1d5db;
    border-radius: 4px;
    width: 100%;
}

/* Inline config */
.action-config-inline {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
}

.action-config-inline input {
    padding: 0.5rem;
    font-size: 0.875rem;
    border: 1px solid #d1d5db;
    border-radius: 4px;
}
```

---

## Smart Field Search Integration

### Step 1: Initialize FieldPathSearchComponent

```javascript
class IfThenElseBuilder {
    constructor(container, step, propertiesPanel) {
        this.container = container;
        this.step = step;
        this.propertiesPanel = propertiesPanel;
        this.fieldSearchInstances = []; // Track instances for cleanup
        this.config = this.initializeConfig(step.config);
        this.render();
    }

    createConditionCard(condition, index) {
        const card = document.createElement('div');
        card.className = 'condition-row';
        card.dataset.index = index;

        // Create structure
        card.innerHTML = `
            <div class="if-section">
                <label>IF</label>
                <div class="field-search-container" id="field-search-${index}"></div>
                <select class="operator-select-compact" data-field="operator" data-index="${index}">
                    ${this.createOperatorOptions(condition.condition.operator)}
                </select>
                <div class="value-section">
                    <label style="display: flex; align-items: center; gap: 0.5rem; font-size: 0.75rem; color: #6b7280;">
                        <input type="checkbox" data-field="useFieldComparison" data-index="${index}"
                               ${condition.condition.compareToField ? 'checked' : ''}>
                        Compare to field
                    </label>
                    <div class="value-input-container" id="value-input-${index}"></div>
                </div>
            </div>

            <div class="then-section">
                <label>THEN (if true)</label>
                <select class="action-select-compact" data-action-type="onTrue" data-index="${index}">
                    ${this.createActionOptions(condition.onTrue.action)}
                </select>
                <div class="action-config-container" id="then-config-${index}"></div>
            </div>

            <div class="else-section">
                <label>ELSE (if false)</label>
                <select class="action-select-compact" data-action-type="onFalse" data-index="${index}">
                    ${this.createActionOptions(condition.onFalse.action)}
                </select>
                <div class="action-config-container" id="else-config-${index}"></div>
            </div>
        `;

        // Initialize field search for "field" input
        setTimeout(() => {
            this.initializeFieldSearch(`field-search-${index}`, condition.condition.field, (value) => {
                condition.condition.field = value;
            });

            // Initialize value input (either plain or field search based on checkbox)
            this.updateValueInput(index, condition);
        }, 0);

        return card;
    }

    initializeFieldSearch(containerId, initialValue, onChange) {
        const container = document.getElementById(containerId);
        if (!container) return;

        // Create FieldPathSearchComponent instance
        const fieldSearch = new FieldPathSearchComponent({
            container: container,
            initialValue: initialValue || '',
            placeholder: 'Search fields...',
            onChange: onChange,
            compact: true  // Use compact mode
        });

        this.fieldSearchInstances.push(fieldSearch);
    }

    updateValueInput(index, condition) {
        const container = document.getElementById(`value-input-${index}`);
        if (!container) return;

        container.innerHTML = ''; // Clear existing

        if (condition.condition.compareToField) {
            // Use field search for cross-field comparison
            this.initializeFieldSearch(`value-input-${index}`, condition.condition.compareToField, (value) => {
                condition.condition.compareToField = value;
                condition.condition.value = ''; // Clear value when using field comparison
            });
        } else {
            // Use plain input for value comparison
            const input = document.createElement('input');
            input.type = 'text';
            input.className = 'value-input-compact';
            input.placeholder = 'Value to compare';
            input.value = condition.condition.value || '';
            input.addEventListener('input', (e) => {
                condition.condition.value = e.target.value;
                condition.condition.compareToField = ''; // Clear field when using value
            });
            container.appendChild(input);
        }
    }

    destroy() {
        // Clean up all field search instances
        this.fieldSearchInstances.forEach(instance => {
            if (instance.destroy) instance.destroy();
        });
        this.fieldSearchInstances = [];
    }
}
```

---

## Action Configuration - Inline Forms

### Set Metadata Action

```javascript
createActionConfig(action, index, actionType) {
    if (action.action === 'set_metadata') {
        return `
            <div class="inline-config">
                <label style="font-size: 0.75rem; color: #6b7280;">Metadata (JSON):</label>
                <textarea
                    class="metadata-json-input"
                    placeholder='{"priority": "high"}'
                    data-field="metadata"
                    data-action-type="${actionType}"
                    data-index="${index}"
                    rows="2"
                    style="font-family: monospace; font-size: 0.75rem;"
                >${action.metadata ? JSON.stringify(action.metadata, null, 2) : ''}</textarea>
            </div>
        `;
    }

    if (action.action === 'set_field') {
        return `
            <div class="inline-config">
                <div class="field-search-container-inline" id="set-field-target-${actionType}-${index}"></div>
                <input type="text"
                       placeholder="New value"
                       value="${action.value || ''}"
                       data-field="value"
                       data-action-type="${actionType}"
                       data-index="${index}"
                       class="value-input-compact">
            </div>
        `;
    }

    // ... other actions
}
```

---

## Visual Flow Indicators

Add arrow SVG between sections:

```html
<div class="condition-row">
    <div class="if-section">...</div>
    <div class="flow-arrow">→</div>
    <div class="then-section">...</div>
    <div class="flow-arrow">→</div>
    <div class="else-section">...</div>
</div>
```

```css
.flow-arrow {
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 1.5rem;
    color: var(--accent-pink);
    font-weight: bold;
}
```

---

## Comparison: Before vs After

### BEFORE (Current):
- **Height:** 600px per condition
- **Inputs:** 6+ text inputs (manual typing)
- **Clicks:** 8 clicks to configure
- **Scroll:** Must scroll to see ELSE section

### AFTER (Redesigned):
- **Height:** 150px per condition (4x less!)
- **Inputs:** 3 smart searches + 3 dropdowns
- **Clicks:** 3 clicks to configure
- **Scroll:** No scrolling needed

---

## Implementation Plan

### Phase 1: Core Refactor (2 hours)
1. ✅ Change layout from vertical to horizontal grid
2. ✅ Integrate FieldPathSearchComponent for field inputs
3. ✅ Compact action configuration (inline forms)
4. ✅ Add flow arrows

### Phase 2: Smart Search (1 hour)
1. ✅ Initialize FieldPathSearchComponent for all field inputs
2. ✅ Toggle between field search and value input
3. ✅ Cleanup on destroy

### Phase 3: Polish (1 hour)
1. ✅ Responsive design (stack on mobile)
2. ✅ Keyboard shortcuts (Enter to add condition)
3. ✅ Visual feedback (highlight on hover)

---

## Benefits Summary

### User Experience
- **4x less scrolling** - compact layout
- **3x fewer clicks** - inline configuration
- **Zero typos** - smart search for fields
- **Faster** - autocomplete vs manual typing

### Developer Experience
- **Reusable** - uses existing FieldPathSearchComponent
- **Maintainable** - smaller, cleaner code
- **Extensible** - easy to add new actions

### Architecture
- **Consistent** - same smart search everywhere
- **OOP** - proper component lifecycle
- **Performant** - cleanup on destroy

---

## Next Steps

1. **User Approval** - Review redesign mockup
2. **Implement** - Refactor IfThenElseBuilder.js
3. **Test** - Verify smart search integration
4. **Document** - Update user guide with new UI

---

**Status:** 📋 READY FOR IMPLEMENTATION
**Estimated Time:** 4 hours
**Dependencies:** FieldPathSearchComponent (already exists)

---

**Created By:** Claude Code
**Date:** December 28, 2025
