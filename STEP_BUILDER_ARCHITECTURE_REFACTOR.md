# Step Builder Architecture - OOP Refactoring Plan

**Date:** December 28, 2025
**Status:** 📋 PLANNING
**Priority:** HIGH - Current architecture not maintainable

---

## Current Problem

**User Feedback:** "I also noticed that everything is going in properties panel, the way i see it every step should be its own file, and will be referenced, we need to make them all as building blocks and reusable with true object oriented concepts."

**Current State:**
- PropertiesPanel.js is **5,700+ lines** and growing
- All step-specific UI logic embedded in one massive file
- Hard to maintain, test, and extend
- No separation of concerns
- Not reusable

---

## Proposed Architecture

### 1. Base Class: StepBuilder

**File:** `public/js/pipeline/builders/BaseStepBuilder.js`

```javascript
/**
 * Base class for all step-specific builders
 */
class BaseStepBuilder {
    constructor(container, step, propertiesPanel) {
        this.container = container;
        this.step = step;
        this.propertiesPanel = propertiesPanel;
    }

    /**
     * Render the step configuration UI
     * @returns {HTMLElement|string} UI element or HTML string
     */
    render() {
        throw new Error('render() must be implemented by subclass');
    }

    /**
     * Get current configuration from UI
     * @returns {Object} Step configuration
     */
    getConfig() {
        throw new Error('getConfig() must be implemented by subclass');
    }

    /**
     * Set configuration in UI
     * @param {Object} config - Configuration to load
     */
    setConfig(config) {
        throw new Error('setConfig() must be implemented by subclass');
    }

    /**
     * Validate configuration
     * @returns {Object} {valid: boolean, errors: string[]}
     */
    validate() {
        return { valid: true, errors: [] };
    }

    /**
     * Clean up event listeners and resources
     */
    destroy() {
        // Override if cleanup needed
    }

    /**
     * Get help documentation for this step
     * @returns {string} HTML help content
     */
    getHelp() {
        return '<p>No documentation available</p>';
    }
}
```

---

### 2. Step-Specific Builders

Each step type gets its own builder class:

#### Structure:
```
public/js/pipeline/builders/
├── BaseStepBuilder.js           // Base class
├── validation/
│   └── FieldValidationBuilder.js
├── transformation/
│   └── FieldMappingBuilder.js
├── enrichment/
│   ├── APIEnrichmentBuilder.js
│   ├── DatabaseEnrichmentBuilder.js
│   └── ScriptEnrichmentBuilder.js
├── logic/
│   ├── IfThenElseBuilder.js     // Already exists, refactor to extend BaseStepBuilder
│   ├── SwitchCaseBuilder.js
│   └── ForEachLoopBuilder.js
└── mapping/
    └── HL7FHIRMappingBuilder.js
```

#### Example: FieldValidationBuilder

**File:** `public/js/pipeline/builders/validation/FieldValidationBuilder.js`

```javascript
class FieldValidationBuilder extends BaseStepBuilder {
    constructor(container, step, propertiesPanel) {
        super(container, step, propertiesPanel);
        this.validationRuleBuilder = null;
    }

    render() {
        const html = `
            <div class="field-validation-builder">
                <div class="validation-rules-section" id="validation-rules-container"></div>
                <div class="form-group">
                    <label>
                        <input type="checkbox" id="detailedOutput" ${this.step.config?.detailedOutput ? 'checked' : ''}>
                        Show detailed field-by-field validation results
                    </label>
                </div>
            </div>
        `;

        this.container.innerHTML = html;

        // Initialize ValidationRuleBuilder component
        const rulesContainer = this.container.querySelector('#validation-rules-container');
        this.validationRuleBuilder = new ValidationRuleBuilder(
            rulesContainer,
            this.step.config?.rules || []
        );

        return this.container;
    }

    getConfig() {
        return {
            rules: this.validationRuleBuilder.getRules(),
            detailedOutput: this.container.querySelector('#detailedOutput').checked
        };
    }

    setConfig(config) {
        if (config.rules && this.validationRuleBuilder) {
            this.validationRuleBuilder.setRules(config.rules);
        }
        if (config.detailedOutput !== undefined) {
            const checkbox = this.container.querySelector('#detailedOutput');
            if (checkbox) checkbox.checked = config.detailedOutput;
        }
    }

    validate() {
        const config = this.getConfig();
        const errors = [];

        if (!config.rules || config.rules.length === 0) {
            errors.push('At least one validation rule is required');
        }

        return {
            valid: errors.length === 0,
            errors
        };
    }

    destroy() {
        if (this.validationRuleBuilder) {
            this.validationRuleBuilder.destroy();
        }
    }

    getHelp() {
        return `
            <h4>Field Validation</h4>
            <p>Validate HL7 message fields against rules.</p>
            <h5>Supported Rules:</h5>
            <ul>
                <li><strong>Required:</strong> Field must not be empty</li>
                <li><strong>Format:</strong> Field must match pattern (email, phone, date)</li>
                <li><strong>Length:</strong> Field length constraints</li>
                <li><strong>Range:</strong> Numeric value ranges</li>
            </ul>
        `;
    }
}
```

---

### 3. Builder Registry

**File:** `public/js/pipeline/builders/BuilderRegistry.js`

```javascript
/**
 * Registry for step-specific builders
 */
class BuilderRegistry {
    constructor() {
        this.builders = new Map();
        this.registerDefaultBuilders();
    }

    /**
     * Register a builder for a step type
     */
    register(stepType, builderClass) {
        this.builders.set(stepType, builderClass);
    }

    /**
     * Get builder class for step type
     */
    getBuilder(stepType) {
        return this.builders.get(stepType);
    }

    /**
     * Check if builder exists for step type
     */
    hasBuilder(stepType) {
        return this.builders.has(stepType);
    }

    /**
     * Register all default builders
     */
    registerDefaultBuilders() {
        // Validation
        this.register('pre.validation', FieldValidationBuilder);

        // Transformation
        this.register('core.transformation', FieldMappingBuilder);
        this.register('core.mapping', HL7FHIRMappingBuilder);

        // Enrichment
        this.register('pre.enrichment.api', APIEnrichmentBuilder);
        this.register('pre.enrichment.database', DatabaseEnrichmentBuilder);
        this.register('pre.enrichment.script', ScriptEnrichmentBuilder);

        // Logic
        this.register('pre.logic', IfThenElseBuilder);
        this.register('core.logic', SwitchCaseBuilder);

        // Add more as needed
    }

    /**
     * Create builder instance for step
     */
    createBuilder(container, step, propertiesPanel) {
        const stepType = step.stepType || step.type;
        const BuilderClass = this.getBuilder(stepType);

        if (!BuilderClass) {
            return null;
        }

        return new BuilderClass(container, step, propertiesPanel);
    }
}

// Global singleton
window.builderRegistry = new BuilderRegistry();
```

---

### 4. Refactored PropertiesPanel

**Simplified PropertiesPanel.js:**

```javascript
class PropertiesPanel {
    constructor(pipelineBuilder) {
        this.builder = pipelineBuilder;
        this.currentBuilder = null; // Current step builder instance
        this.currentStep = null;
    }

    showStepProperties(step, isPreview = false) {
        this.currentStep = step;
        this.isPreviewMode = isPreview;

        // Clean up previous builder
        if (this.currentBuilder) {
            this.currentBuilder.destroy();
            this.currentBuilder = null;
        }

        // Render modal structure
        this.renderModal(step);

        // Create step-specific builder
        const formContainer = document.getElementById('formTabContent');
        this.currentBuilder = window.builderRegistry.createBuilder(
            formContainer,
            step,
            this
        );

        if (this.currentBuilder) {
            // Render step-specific UI
            this.currentBuilder.render();
            this.currentBuilder.setConfig(step.config || {});
        } else {
            // Fallback to generic form
            this.renderGenericForm(step);
        }

        // Show modal
        this.openModal();
    }

    saveStepProperties(step) {
        if (!this.currentBuilder) {
            throw new Error('No builder instance');
        }

        // Validate configuration
        const validation = this.currentBuilder.validate();
        if (!validation.valid) {
            this.showErrors(validation.errors);
            return;
        }

        // Get configuration from builder
        step.config = this.currentBuilder.getConfig();

        // Update pipeline
        this.builder.updateStep(step);
        this.builder.markAsUnsaved();
        this.builder.savePipeline();

        this.closeModal();
    }

    closeModal() {
        // Clean up builder
        if (this.currentBuilder) {
            this.currentBuilder.destroy();
            this.currentBuilder = null;
        }

        this.currentStep = null;
        // ... rest of cleanup
    }

    renderGenericForm(step) {
        // Fallback for steps without custom builders
        // Simple textarea for JSON config
    }
}
```

---

## Migration Plan

### Phase 1: Infrastructure (Week 1)
1. ✅ Create `BaseStepBuilder.js`
2. ✅ Create `BuilderRegistry.js`
3. ✅ Refactor `PropertiesPanel.js` to use registry
4. ✅ Test with one builder (IfThenElseBuilder)

### Phase 2: Migrate Existing Builders (Week 2-3)
1. ✅ Refactor `IfThenElseBuilder` to extend `BaseStepBuilder`
2. ✅ Extract `ValidationRuleBuilder` from PropertiesPanel
3. ✅ Extract `FieldMappingBuilder` from PropertiesPanel
4. ✅ Extract `HL7FHIRMappingBuilder` from PropertiesPanel
5. ✅ Extract enrichment builders (API, Database, Script)

### Phase 3: New Builders (Week 4)
1. ✅ Create `SwitchCaseBuilder`
2. ✅ Create `ForEachLoopBuilder`
3. ✅ Create remaining step builders

### Phase 4: Cleanup (Week 5)
1. ✅ Remove old code from PropertiesPanel
2. ✅ Update documentation
3. ✅ Add unit tests for builders
4. ✅ Performance testing

---

## Benefits

### 1. Maintainability
- **Small, focused files** (200-500 lines each vs 5,700+ line monolith)
- **Easy to locate** step-specific code
- **Independent testing** of each builder

### 2. Reusability
- **Builders can be used standalone** (not tied to PropertiesPanel)
- **Share builders** across different UIs
- **Plugin architecture** - easy to add new step types

### 3. Scalability
- **Add new step types** without modifying PropertiesPanel
- **Parallel development** - multiple devs can work on different builders
- **No merge conflicts** - each builder in separate file

### 4. Testability
- **Unit test each builder** independently
- **Mock dependencies** easily
- **Integration testing** simplified

### 5. Code Organization
- **Clear structure** - know where to find step logic
- **Consistent patterns** - all builders follow same interface
- **Self-documenting** - builder name = step type

---

## Example Usage

### Adding a New Step Type

**1. Create builder class:**

```javascript
// public/js/pipeline/builders/custom/MyCustomBuilder.js
class MyCustomBuilder extends BaseStepBuilder {
    render() {
        this.container.innerHTML = `<h4>My Custom Step</h4>`;
        return this.container;
    }

    getConfig() {
        return { customField: 'value' };
    }

    setConfig(config) {
        // Load config into UI
    }
}
```

**2. Register builder:**

```javascript
// In BuilderRegistry.registerDefaultBuilders()
this.register('pre.custom', MyCustomBuilder);
```

**3. Done!** PropertiesPanel automatically uses it.

---

## Fixing templateId Issue

**Problem:** Steps saved without `templateId` can't be detected

**Solution 1: Fix at source (VisualStep.toJSON)**

```javascript
// In VisualStep.toJSON()
toJSON() {
    return {
        id: this.id,
        stepName: this.stepName,
        stepType: this.stepType,
        templateId: this.templateId || this.inferTemplateId(), // FIX: Infer if missing
        // ... rest
    };
}

inferTemplateId() {
    // Map stepType to templateId for backward compatibility
    const typeToTemplate = {
        'pre.logic': 'if-then-else',
        'core.logic': 'switch-case',
        'pre.validation': 'field-validation',
        // etc.
    };
    return typeToTemplate[this.stepType] || null;
}
```

**Solution 2: Database migration to backfill templateId**

```sql
-- Update existing steps with missing templateId
UPDATE transformation_steps
SET config = jsonb_set(
    config,
    '{templateId}',
    CASE
        WHEN step_type = 'pre.logic' AND step_name = 'If-Then-Else' THEN '"if-then-else"'
        WHEN step_type = 'core.logic' AND step_name = 'Switch/Case' THEN '"switch-case"'
        -- Add more mappings
    END::jsonb
)
WHERE config->>'templateId' IS NULL;
```

---

## Next Steps

### Immediate (Today)
1. ✅ Fix templateId detection (use stepName fallback) - DONE
2. ✅ Test IfThenElseBuilder appears correctly
3. ⏳ User testing and feedback

### Short-term (This Week)
1. Create `BaseStepBuilder.js`
2. Create `BuilderRegistry.js`
3. Refactor `IfThenElseBuilder` to extend BaseStepBuilder
4. Update PropertiesPanel to use registry

### Medium-term (Next 2 Weeks)
1. Extract all existing builders from PropertiesPanel
2. Create remaining builders (SwitchCase, ForEach)
3. Remove old code from PropertiesPanel
4. Add tests

---

## Questions for User

1. **Priority:** Should we start the refactoring now or finish conditional logic first?

2. **Migration:** Should we migrate all builders at once or incrementally?

3. **Backward Compatibility:** Should we fix existing steps' templateId in database?

4. **Testing:** Do you want unit tests for each builder or just integration tests?

---

**Status:** 📋 READY FOR DISCUSSION
**Estimated Effort:** 4-5 weeks (with parallel work on conditional logic)

---

**Created By:** Claude Code
**Date:** December 28, 2025
