# If-Then-Else UI Integration Instructions

## How to Integrate the IfThenElseBuilder into PropertiesPanel

### Step 1: Add Script Tag to HTML

Add to `public/pipeline-builder.html`:

```html
<!-- Load IfThenElseBuilder component -->
<script src="js/pipeline/components/IfThenElseBuilder.js"></script>
```

Place this **before** the `PropertiesPanel.js` script tag.

### Step 2: Update PropertiesPanel.js

Add custom UI rendering for If-Then-Else steps.

**Location:** `public/js/pipeline/managers/PropertiesPanel.js`

**Find the `createDynamicFormFields()` method and add:**

```javascript
createDynamicFormFields(step) {
    // Check if this is an If-Then-Else step
    if (step.stepType === 'pre.logic' && step.templateId === 'if-then-else') {
        return this.createIfThenElseUI(step);
    }

    // ... existing code for other step types ...
}

/**
 * Create If-Then-Else visual builder
 */
createIfThenElseUI(step) {
    const container = document.createElement('div');
    container.id = 'if-then-else-container';
    container.style.marginTop = '1rem';

    // Initialize builder with step config
    const initialConfig = step.config || null;
    this.ifThenElseBuilder = new IfThenElseBuilder(container, initialConfig);

    return container;
}
```

### Step 3: Update Save Handler

**Find the form save handler and update to read builder config:**

```javascript
async saveStepConfiguration(step, formData) {
    // ... existing code ...

    // If this is an If-Then-Else step, get config from builder
    if (step.stepType === 'pre.logic' && step.templateId === 'if-then-else') {
        if (this.ifThenElseBuilder) {
            step.config = this.ifThenElseBuilder.getConfig();
        }
    }

    // ... rest of save logic ...
}
```

### Step 4: Update Load Handler

**Update the form load to initialize builder with existing config:**

```javascript
showStepProperties(step, isPreview = false) {
    // ... existing code ...

    // If this is an If-Then-Else step with existing config
    if (step.stepType === 'pre.logic' &&
        step.templateId === 'if-then-else' &&
        step.config &&
        this.ifThenElseBuilder) {
        this.ifThenElseBuilder.setConfig(step.config);
    }

    // ... rest of code ...
}
```

---

## Alternative: Standalone Integration

If you prefer to keep PropertiesPanel unchanged, you can detect the step type and show the builder in a modal:

```javascript
// In your pipeline builder code
function editIfThenElseStep(step) {
    // Create modal
    const modal = document.createElement('div');
    modal.className = 'ifthen-edit-modal';
    modal.innerHTML = `
        <div class="ifthen-edit-content">
            <div class="ifthen-edit-header">
                <h3>Configure If-Then-Else Logic</h3>
                <button class="ifthen-close">&times;</button>
            </div>
            <div id="ifthen-builder-container"></div>
            <div class="ifthen-edit-footer">
                <button class="ifthen-cancel">Cancel</button>
                <button class="ifthen-save">Save</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);

    // Initialize builder
    const container = modal.querySelector('#ifthen-builder-container');
    const builder = new IfThenElseBuilder(container, step.config);

    // Save handler
    modal.querySelector('.ifthen-save').addEventListener('click', () => {
        step.config = builder.getConfig();
        modal.remove();
        // Update visual step display
        updateStepDisplay(step);
    });

    // Cancel handler
    modal.querySelector('.ifthen-cancel').addEventListener('click', () => {
        modal.remove();
    });

    modal.querySelector('.ifthen-close').addEventListener('click', () => {
        modal.remove();
    });
}
```

---

## Testing the Integration

### Quick Test:

1. **Load Pipeline Builder:**
   ```
   http://localhost:3000/pipeline-builder.html
   ```

2. **Add If-Then-Else Step:**
   - Drag from toolbox to pipeline
   - Double-click to open properties

3. **Verify UI Loads:**
   - Check console for errors
   - Verify IfThenElseBuilder appears
   - Test adding conditions
   - Test changing actions

4. **Test Save/Load:**
   - Configure a condition
   - Save step
   - Close properties
   - Reopen properties
   - Verify configuration persists

### Debug Steps:

**If UI doesn't appear:**

```javascript
// Check in browser console:
console.log('IfThenElseBuilder loaded:', typeof IfThenElseBuilder);
// Should print: "function"

console.log('Container found:', document.getElementById('if-then-else-container'));
// Should print: div element

console.log('Builder initialized:', this.ifThenElseBuilder);
// Should print: IfThenElseBuilder instance
```

**If config doesn't save:**

```javascript
// Add debug logging:
console.log('Saving config:', this.ifThenElseBuilder.getConfig());
// Should print: { conditions: [...] }
```

---

## Example: Complete Integration Code

Here's a complete example of integrating into PropertiesPanel:

```javascript
// In PropertiesPanel.js

class PropertiesPanel {
    constructor(pipelineBuilder) {
        this.builder = pipelineBuilder;
        this.ifThenElseBuilder = null;  // Add this property
        this.init();
    }

    createDynamicFormFields(step) {
        // Handle If-Then-Else steps
        if (this.isIfThenElseStep(step)) {
            return this.createIfThenElseBuilder(step);
        }

        // Handle other step types
        // ... existing code ...
    }

    isIfThenElseStep(step) {
        return (step.stepType === 'pre.logic' &&
                (step.templateId === 'if-then-else' ||
                 step.id === 'if-then-else'));
    }

    createIfThenElseBuilder(step) {
        const section = document.createElement('div');
        section.className = 'form-section';
        section.innerHTML = `
            <h4 style="color: var(--primary-color); margin-bottom: 1rem;">
                Conditional Logic Configuration
            </h4>
            <div id="if-then-else-builder-container"></div>
        `;

        // Get container after DOM insertion
        setTimeout(() => {
            const container = document.getElementById('if-then-else-builder-container');
            if (container) {
                this.ifThenElseBuilder = new IfThenElseBuilder(
                    container,
                    step.config || null
                );
            }
        }, 0);

        return section;
    }

    async saveStepConfiguration(step, formElement) {
        // Read If-Then-Else config from builder
        if (this.isIfThenElseStep(step) && this.ifThenElseBuilder) {
            step.config = this.ifThenElseBuilder.getConfig();
            console.log('✅ Saved If-Then-Else config:', step.config);
        }

        // ... rest of save logic ...
    }
}
```

---

## CSS Additions (if needed)

If you need custom modal styles:

```css
/* Add to pipeline-builder.css */

.ifthen-edit-modal {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 10000;
}

.ifthen-edit-content {
    background: white;
    border-radius: 8px;
    width: 90%;
    max-width: 800px;
    max-height: 90vh;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1);
}

.ifthen-edit-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 1.5rem;
    border-bottom: 2px solid var(--accent-pink);
}

.ifthen-edit-header h3 {
    margin: 0;
    color: var(--primary-color);
}

.ifthen-close {
    background: none;
    border: none;
    font-size: 2rem;
    color: #9ca3af;
    cursor: pointer;
    line-height: 1;
}

#ifthen-builder-container {
    flex: 1;
    overflow-y: auto;
    padding: 1rem;
}

.ifthen-edit-footer {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    padding: 1rem 1.5rem;
    border-top: 1px solid #e5e7eb;
}

.ifthen-cancel,
.ifthen-save {
    padding: 0.5rem 1.5rem;
    border-radius: 4px;
    border: none;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s;
}

.ifthen-cancel {
    background: #f3f4f6;
    color: #374151;
}

.ifthen-cancel:hover {
    background: #e5e7eb;
}

.ifthen-save {
    background: var(--primary-color);
    color: white;
}

.ifthen-save:hover {
    background: var(--primary-hover);
}
```

---

## Verification Checklist

- [ ] IfThenElseBuilder.js loaded in HTML
- [ ] Builder appears when editing If-Then-Else step
- [ ] Can add/delete conditions
- [ ] Can configure all operators
- [ ] Can configure all action types
- [ ] Cross-field comparison works
- [ ] Help modal shows examples
- [ ] Config saves to step
- [ ] Config loads from step
- [ ] Colors match theme (navy blue, pastel pink)

---

**Status:** Ready for integration
**Estimated Time:** 15-30 minutes
**Difficulty:** Easy (if following instructions)

Need help? Check the browser console for errors and refer to the testing guide!
