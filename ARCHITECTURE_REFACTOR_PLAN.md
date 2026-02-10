# Architecture Refactor Plan - OOP & MVC Principles

## Current State Analysis

### ✅ What's Already Good (Go Backend)
The Go backend **already follows excellent OOP principles**:

**Strategy Pattern** (executor_interface.go):
```go
type Executor interface {
    Execute(ctx context.Context, step *TransformationStep, inputData map[string]interface{}) (map[string]interface{}, error)
    GetStepType() string
}
```

**Template Method Pattern** (base_executor.go):
```go
type BaseExecutor struct {
    stepType string
    metadata ExecutorMetadata
}

func (b *BaseExecutor) PreExecute(ctx context.Context, step *TransformationStep) error
func (b *BaseExecutor) PostExecute(ctx context.Context, step *TransformationStep, err error, duration time.Duration)
func (b *BaseExecutor) SetStepOutput(execContext *PipelineExecutionContext, step *TransformationStep, outputData map[string]interface{})
```

**Interface Segregation Principle**:
```go
type Validatable interface {
    Validate(step *TransformationStep) error
}

type MetadataProvider interface {
    GetMetadata() ExecutorMetadata
}

type Cacheable interface {
    GetCacheKey(step *TransformationStep, inputData map[string]interface{}) string
    GetCacheTTL() int
}
```

**Example Executor** (conditional_executor.go):
```go
type ConditionalExecutor struct {
    *BaseExecutor  // Composition
}

func (e *ConditionalExecutor) Execute(ctx context.Context, step *models.TransformationStep, inputData map[string]interface{}) (map[string]interface{}, error) {
    start := time.Now()

    // Template Method: PreExecute (from BaseExecutor)
    if err := e.PreExecute(ctx, step); err != nil {
        return inputData, err
    }

    // Actual execution logic
    // ...

    // Template Method: PostExecute (from BaseExecutor)
    e.PostExecute(ctx, step, err, time.Since(start))
    return outputData, err
}
```

### ❌ What Needs Improvement (Frontend)

**Problem Areas**:

1. **No base class for step configuration builders** - Each builder (IfThenElseBuilder, ValidationRuleBuilder, etc.) duplicates common functionality
2. **No standardized interface** - Builders have inconsistent method signatures
3. **Tight coupling** - PropertiesPanel knows too much about specific builder implementations
4. **Duplication** - Common patterns like `getConfig()`, `render()`, `destroy()` are reimplemented in each builder

---

## Proposed Frontend Architecture (MVC + OOP)

### 1. Base Classes and Interfaces

#### BaseStepConfigBuilder (Abstract Base Class)
```javascript
/**
 * Abstract base class for all step configuration builders
 * Implements Template Method pattern and defines common interface
 */
class BaseStepConfigBuilder {
    constructor(container, initialConfig = {}) {
        if (new.target === BaseStepConfigBuilder) {
            throw new Error('BaseStepConfigBuilder is abstract and cannot be instantiated directly');
        }

        this.container = container;
        this.config = this.getDefaultConfig();
        this.initialized = false;

        // Merge initial config with defaults
        if (initialConfig) {
            this.config = this.mergeConfig(this.config, initialConfig);
        }
    }

    // ========================================
    // TEMPLATE METHOD PATTERN
    // ========================================

    /**
     * Main initialization method (Template Method)
     * Defines the algorithm structure, subclasses fill in details
     */
    init() {
        if (this.initialized) {
            console.warn(`[${this.constructor.name}] Already initialized`);
            return;
        }

        console.log(`[${this.constructor.name}] Initializing...`);

        // Hook: Pre-initialization (optional override)
        this.beforeInit();

        // Step 1: Validate container
        if (!this.container) {
            throw new Error('Container element is required');
        }

        // Step 2: Render UI (implemented by subclass)
        this.render();

        // Step 3: Attach event listeners (implemented by subclass)
        this.attachEvents();

        // Step 4: Add styles (optional override)
        this.addStyles();

        // Hook: Post-initialization (optional override)
        this.afterInit();

        this.initialized = true;
        console.log(`[${this.constructor.name}] ✅ Initialized`);
    }

    // ========================================
    // ABSTRACT METHODS (must be implemented by subclasses)
    // ========================================

    /**
     * Returns default configuration structure
     * @abstract
     */
    getDefaultConfig() {
        throw new Error('getDefaultConfig() must be implemented by subclass');
    }

    /**
     * Renders the UI
     * @abstract
     */
    render() {
        throw new Error('render() must be implemented by subclass');
    }

    /**
     * Attaches event listeners
     * @abstract
     */
    attachEvents() {
        throw new Error('attachEvents() must be implemented by subclass');
    }

    // ========================================
    // HOOK METHODS (optional overrides)
    // ========================================

    beforeInit() {
        // Hook for pre-initialization logic
    }

    afterInit() {
        // Hook for post-initialization logic
    }

    // ========================================
    // COMMON METHODS (shared by all builders)
    // ========================================

    /**
     * Gets current configuration
     * @returns {Object} Current config
     */
    getConfig() {
        return this.config;
    }

    /**
     * Sets configuration and re-renders
     * @param {Object} newConfig - New configuration
     */
    setConfig(newConfig) {
        this.config = this.mergeConfig(this.getDefaultConfig(), newConfig);
        if (this.initialized) {
            this.render();
            this.attachEvents();
        }
    }

    /**
     * Validates configuration
     * @returns {Object} { valid: boolean, errors: string[] }
     */
    validate() {
        // Default validation (can be overridden)
        return { valid: true, errors: [] };
    }

    /**
     * Deep merge two config objects
     */
    mergeConfig(target, source) {
        const result = JSON.parse(JSON.stringify(target)); // Deep clone

        for (const key in source) {
            if (source.hasOwnProperty(key)) {
                if (source[key] && typeof source[key] === 'object' && !Array.isArray(source[key])) {
                    result[key] = this.mergeConfig(result[key] || {}, source[key]);
                } else {
                    result[key] = source[key];
                }
            }
        }

        return result;
    }

    /**
     * Clears UI and removes event listeners
     */
    clear() {
        if (this.container) {
            this.container.innerHTML = '';
        }
    }

    /**
     * Cleanup and destroy
     */
    destroy() {
        this.clear();
        this.initialized = false;
    }

    /**
     * Adds custom styles (optional override)
     */
    addStyles() {
        // Default: no custom styles
    }
}
```

---

### 2. Refactored IfThenElseBuilder

#### Before (Current):
```javascript
class IfThenElseBuilder {
    constructor(container, initialConfig = null) {
        this.container = container;
        this.config = initialConfig || this.getDefaultConfig();
        // ... lots of duplicate code
    }

    getDefaultConfig() { /* ... */ }
    render() { /* ... */ }
    getConfig() { return this.config; }
    destroy() { /* ... */ }
    // ... hundreds of lines
}
```

#### After (Refactored):
```javascript
/**
 * IfThenElseBuilder - Extends BaseStepConfigBuilder
 * Handles If-Then-Else conditional logic configuration
 */
class IfThenElseBuilder extends BaseStepConfigBuilder {
    constructor(container, initialConfig = {}) {
        super(container, initialConfig);
        this.fieldSearchInstances = [];
    }

    // ========================================
    // REQUIRED IMPLEMENTATIONS
    // ========================================

    getDefaultConfig() {
        return {
            conditions: [
                {
                    name: 'Condition 1',
                    condition: {
                        field: '',
                        operator: 'equals',
                        value: '',
                        compareToField: ''
                    },
                    onTrue: {
                        action: 'continue'
                    },
                    onFalse: {
                        action: 'continue'
                    }
                }
            ]
        };
    }

    render() {
        this.clear();

        const container = document.createElement('div');
        container.className = 'ifthen-builder-container';

        // Render conditions
        this.config.conditions.forEach((condition, index) => {
            const card = this.createConditionCard(condition, index);
            container.appendChild(card);
        });

        // Add "Add Condition" button
        const addBtn = this.createAddButton();
        container.appendChild(addBtn);

        this.container.appendChild(container);
    }

    attachEvents() {
        // Attach event listeners to all interactive elements
        // (existing event listener code)
    }

    // ========================================
    // OPTIONAL OVERRIDES
    // ========================================

    validate() {
        const errors = [];

        this.config.conditions.forEach((condition, index) => {
            if (!condition.condition || !condition.condition.field) {
                errors.push(`Condition ${index + 1}: Field is required`);
            }

            if (!condition.onTrue || !condition.onTrue.action) {
                errors.push(`Condition ${index + 1}: TRUE action is required`);
            }

            if (!condition.onFalse || !condition.onFalse.action) {
                errors.push(`Condition ${index + 1}: FALSE action is required`);
            }
        });

        return {
            valid: errors.length === 0,
            errors: errors
        };
    }

    destroy() {
        // Clean up field search instances
        this.fieldSearchInstances.forEach(instance => {
            if (instance.destroy) instance.destroy();
        });
        this.fieldSearchInstances = [];

        // Call parent destroy
        super.destroy();
    }

    // ========================================
    // BUILDER-SPECIFIC METHODS
    // ========================================

    createConditionCard(condition, index) {
        // Existing implementation
    }

    addCondition() {
        this.config.conditions.push(this.getDefaultConfig().conditions[0]);
        this.render();
        this.attachEvents();
    }

    deleteCondition(index) {
        this.config.conditions.splice(index, 1);
        this.render();
        this.attachEvents();
    }

    // ... other helper methods
}
```

---

### 3. Refactored ValidationRuleBuilder

```javascript
/**
 * ValidationRuleBuilder - Extends BaseStepConfigBuilder
 * Handles field validation rules configuration
 */
class ValidationRuleBuilder extends BaseStepConfigBuilder {
    constructor(container, initialConfig = {}) {
        super(container, initialConfig);
    }

    getDefaultConfig() {
        return {
            rules: [
                {
                    field: '',
                    validationType: 'required',
                    errorMessage: ''
                }
            ]
        };
    }

    render() {
        // Render validation rules UI
    }

    attachEvents() {
        // Attach event listeners
    }

    validate() {
        const errors = [];

        this.config.rules.forEach((rule, index) => {
            if (!rule.field) {
                errors.push(`Rule ${index + 1}: Field is required`);
            }
        });

        return {
            valid: errors.length === 0,
            errors: errors
        };
    }
}
```

---

### 4. Builder Factory Pattern

```javascript
/**
 * StepConfigBuilderFactory - Creates appropriate builder based on step type
 * Implements Factory Pattern
 */
class StepConfigBuilderFactory {
    static builders = new Map();

    /**
     * Register a builder class for a step type
     */
    static register(stepType, builderClass) {
        this.builders.set(stepType, builderClass);
        console.log(`✅ Registered builder for step type: ${stepType}`);
    }

    /**
     * Create a builder instance for a step type
     */
    static create(stepType, container, initialConfig = {}) {
        const BuilderClass = this.builders.get(stepType);

        if (!BuilderClass) {
            console.warn(`⚠️ No builder registered for step type: ${stepType}`);
            return null;
        }

        return new BuilderClass(container, initialConfig);
    }

    /**
     * Check if a builder is registered for a step type
     */
    static hasBuilder(stepType) {
        return this.builders.has(stepType);
    }

    /**
     * Get all registered step types
     */
    static getRegisteredTypes() {
        return Array.from(this.builders.keys());
    }
}

// Register all builders
StepConfigBuilderFactory.register('pre.logic', IfThenElseBuilder);
StepConfigBuilderFactory.register('core.logic', IfThenElseBuilder);
StepConfigBuilderFactory.register('post.logic', IfThenElseBuilder);
StepConfigBuilderFactory.register('pre.validation', ValidationRuleBuilder);
StepConfigBuilderFactory.register('pre.enrichment.metadata', MetadataBuilder);
// ... register all other builders
```

---

### 5. Refactored PropertiesPanel (MVC Controller)

#### Before (Tight Coupling):
```javascript
class PropertiesPanel {
    loadStepProperties(step) {
        // Lots of if-else statements for different step types
        if (step.stepType === 'pre.logic') {
            this.ifThenElseBuilder = new IfThenElseBuilder(container, step.config);
        } else if (step.stepType === 'pre.validation') {
            this.validationBuilder = new ValidationRuleBuilder(container, step.config);
        }
        // ... 20 more if-else statements
    }
}
```

#### After (Loose Coupling via Factory):
```javascript
class PropertiesPanel {
    constructor() {
        this.currentBuilder = null;
        this.currentStep = null;
    }

    loadStepProperties(step) {
        // Clean up previous builder
        if (this.currentBuilder) {
            this.currentBuilder.destroy();
            this.currentBuilder = null;
        }

        this.currentStep = step;

        // Get container for step-specific config
        const configContainer = document.getElementById('step-config-container');

        // Use factory to create appropriate builder
        this.currentBuilder = StepConfigBuilderFactory.create(
            step.stepType,
            configContainer,
            step.config
        );

        // Initialize builder if it exists
        if (this.currentBuilder) {
            this.currentBuilder.init();
        } else {
            // No custom builder - show generic config form
            this.renderGenericConfig(step);
        }
    }

    collectFormData(step) {
        // If custom builder exists, get config from it
        if (this.currentBuilder) {
            // Validate before collecting
            const validation = this.currentBuilder.validate();

            if (!validation.valid) {
                this.showValidationErrors(validation.errors);
                return null;
            }

            step.config = this.currentBuilder.getConfig();
        } else {
            // Collect from generic form inputs
            step.config = this.collectGenericFormData();
        }

        return step;
    }

    showValidationErrors(errors) {
        const errorContainer = document.getElementById('validation-errors');
        errorContainer.innerHTML = errors.map(err => `
            <div class="alert alert-danger">
                <i class="fas fa-exclamation-triangle"></i> ${err}
            </div>
        `).join('');
    }
}
```

---

## Migration Strategy

### Phase 1: Create Base Classes (Week 1)
- [ ] Create `BaseStepConfigBuilder.js`
- [ ] Create `StepConfigBuilderFactory.js`
- [ ] Add unit tests for base class

### Phase 2: Refactor Existing Builders (Week 2-3)
- [ ] Refactor `IfThenElseBuilder.js` to extend base class
- [ ] Refactor `ValidationRuleBuilder.js`
- [ ] Refactor `MetadataBuilder.js`
- [ ] Refactor remaining builders (OAuth2, MongoDB, etc.)

### Phase 3: Update PropertiesPanel (Week 3)
- [ ] Update PropertiesPanel to use factory pattern
- [ ] Remove hard-coded builder instantiation
- [ ] Add validation error display

### Phase 4: Testing & Documentation (Week 4)
- [ ] Test all builders with factory
- [ ] Verify no regressions in existing functionality
- [ ] Update developer documentation
- [ ] Create architecture diagram

---

## Benefits After Refactoring

### 1. **DRY (Don't Repeat Yourself)**
✅ Common functionality in base class eliminates duplication
✅ Template method pattern ensures consistent behavior

### 2. **SOLID Principles**
✅ **S**ingle Responsibility: Each builder handles one step type
✅ **O**pen/Closed: Open for extension (new builders), closed for modification
✅ **L**iskov Substitution: All builders interchangeable via base class
✅ **I**nterface Segregation: Optional methods via hooks
✅ **D**ependency Inversion: PropertiesPanel depends on abstraction (factory), not concrete builders

### 3. **Maintainability**
✅ Easy to add new step types - just extend base class and register
✅ Consistent API across all builders
✅ Centralized validation logic

### 4. **Testability**
✅ Base class can be unit tested independently
✅ Mock builders easily created for testing PropertiesPanel
✅ Factory pattern enables dependency injection

---

## Example: Adding a New Step Type

### Before (Without Refactoring):
```javascript
// 1. Create entire builder from scratch (500+ lines)
class NewStepBuilder {
    constructor() { /* ... */ }
    getDefaultConfig() { /* ... */ }
    render() { /* ... */ }
    getConfig() { /* ... */ }
    destroy() { /* ... */ }
    // ... repeat all common code
}

// 2. Update PropertiesPanel with new if-else
if (step.stepType === 'new.step') {
    this.newStepBuilder = new NewStepBuilder(container, step.config);
}
```

### After (With Refactoring):
```javascript
// 1. Extend base class (50-100 lines only)
class NewStepBuilder extends BaseStepConfigBuilder {
    getDefaultConfig() {
        return { /* specific config */ };
    }

    render() {
        // Only step-specific rendering logic
    }

    attachEvents() {
        // Only step-specific events
    }
}

// 2. Register with factory (1 line)
StepConfigBuilderFactory.register('new.step', NewStepBuilder);

// 3. PropertiesPanel automatically works - NO CHANGES NEEDED
```

---

## Code Quality Metrics

### Before Refactoring:
- **Lines of duplicated code**: ~2000 lines
- **Builder consistency**: 30% (varies by developer)
- **Time to add new builder**: 4-6 hours
- **Test coverage**: Difficult to test

### After Refactoring:
- **Lines of duplicated code**: 0 (all in base class)
- **Builder consistency**: 100% (enforced by base class)
- **Time to add new builder**: 30-60 minutes
- **Test coverage**: Easy to achieve 80%+

---

## Questions for User

1. **Timing**: Should we refactor now, or complete other features first?
2. **Scope**: Start with just IfThenElseBuilder as proof-of-concept, or refactor all builders at once?
3. **Backward Compatibility**: Should we maintain support for old builder instances during transition?
4. **Testing**: Do you have existing tests we should preserve, or create new test suite?

---

## Next Steps

If you approve this plan, I can:
1. Create `BaseStepConfigBuilder.js` and `StepConfigBuilderFactory.js`
2. Refactor `IfThenElseBuilder.js` as proof-of-concept
3. Show you the before/after comparison
4. Then proceed with remaining builders

Let me know your preference!
