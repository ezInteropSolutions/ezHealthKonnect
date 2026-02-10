# If-Then-Else Actions - Consolidation & Improvement Plan

**Date:** December 28, 2025
**Status:** 🔍 ANALYSIS - User Feedback

---

## User Feedback Analysis ✅

**User is 100% correct** - there is significant redundancy and confusion in the current action list.

### Issues Identified:

1. **Continue vs Log Warning** - Almost identical (both continue processing)
2. **Reject vs Log Error** - Almost identical (both stop processing)
3. **Set Metadata, Set Field, Copy Field** - Could be unified into one "Set Value" action
4. **Route To** - Unclear what values to use, no guidance

---

## Current Problems

### Problem 1: Redundant "Continue" Actions

**Current:**
- `continue` - Does nothing, continues
- `log_warning` - Logs message, continues

**Issue:** Why have two separate actions when log_warning can do everything continue does?

**User Confusion:**
> "When should I use Continue vs Log Warning?"

### Problem 2: Redundant "Stop" Actions

**Current:**
- `reject` - Stops with error message + severity
- `log_error` - Logs error, stops processing

**Issue:** Both stop the pipeline. Why are they separate?

**User Confusion:**
> "What's the difference between Reject and Log Error?"

### Problem 3: Fragmented "Set Value" Actions

**Current:**
- `set_metadata` - Sets `_metadata.key = value`
- `set_field` - Sets `field.path = value`
- `copy_field` - Sets `target = source`

**Issue:** All three are just "assign a value to a path". Why split them?

**User Confusion:**
> "I want to set patient.priority = 'high'. Do I use Set Metadata or Set Field?"

### Problem 4: Unclear "Route To" Configuration

**Current UI:**
```
Route To: [text input: destination]
```

**User Questions:**
- "What values can I enter here?"
- "Is this a URL? A queue name? A system ID?"
- "Where do I find valid destinations?"

---

## Proposed Solution: Consolidate to 5 Clear Actions

### ✅ Recommended Action List (5 Actions)

| Action | Purpose | Configuration | Backend Mapping |
|--------|---------|---------------|-----------------|
| **1. Continue** | Proceed to next step (with optional message) | • Message (optional)<br>• Log Level: None/Info/Warning | `continue` or `log_warning` |
| **2. Stop** | Halt processing with error | • Error Message (required)<br>• Severity: Error/Fatal | `reject` or `log_error` |
| **3. Set Value** | Assign value to any field | • Target Field (search)<br>• Value Type: Constant/Copy/Expression<br>• Value (conditional UI) | `set_field`, `set_metadata`, `copy_field` |
| **4. Delete Field** | Remove field from data | • Field Path (search) | `delete_field` |
| **5. Route To** | Set delivery destination | • Destination (dropdown)<br>• Custom (if "Other") | `route_to` |

---

## Detailed Action Designs

### 1. Continue (Unified with Log Warning)

**UI Design:**
```
┌──────────────────────────────────────────┐
│ Action: [Continue ▼]                     │
├──────────────────────────────────────────┤
│ Log Level: [None ▼]                      │
│            [None/Info/Warning/Debug]     │
│                                          │
│ Message: [Optional message to log]      │
│ (only shown if Log Level ≠ None)        │
└──────────────────────────────────────────┘
```

**Backend Mapping:**
```javascript
// UI Config
{
  action: 'continue',
  logLevel: 'warning',  // or 'none', 'info', 'debug'
  message: 'Patient age verified'
}

// Backend Translation
if (logLevel === 'none') {
  → { action: 'continue' }
} else if (logLevel === 'warning') {
  → { action: 'log_warning', message: '...' }
} else if (logLevel === 'info') {
  → { action: 'log_info', message: '...' }  // New backend action
}
```

**Benefits:**
- ✅ One action instead of two
- ✅ Clear that it always continues
- ✅ Optional logging for debugging

---

### 2. Stop (Unified Reject + Log Error)

**UI Design:**
```
┌──────────────────────────────────────────┐
│ Action: [Stop ▼]                         │
├──────────────────────────────────────────┤
│ Error Message: [Required - why stopped] │
│                                          │
│ Severity: [Error ▼]                      │
│           [Warning/Error/Fatal]          │
└──────────────────────────────────────────┘
```

**Backend Mapping:**
```javascript
// UI Config
{
  action: 'stop',
  message: 'Patient MRN is required',
  severity: 'error'
}

// Backend Translation (always maps to reject)
→ {
  action: 'reject',
  errorMessage: 'Patient MRN is required',
  severity: 'error'
}
```

**Benefits:**
- ✅ Clear name: "Stop" means stop
- ✅ Always requires explanation (message)
- ✅ One action for all stop scenarios

---

### 3. Set Value (Unified Set Field + Set Metadata + Copy Field)

**UI Design (Smart Multi-Mode):**

```
┌──────────────────────────────────────────┐
│ Action: [Set Value ▼]                    │
├──────────────────────────────────────────┤
│ Target Field: [patient.priority 🔍]      │
│ (field search with autocomplete)         │
│                                          │
│ Value Type: [Constant ▼]                 │
│             [Constant/Copy From/Expression] │
│                                          │
│ ┌─ If Constant ──────────────────────┐  │
│ │ Value: [high]                       │  │
│ └────────────────────────────────────┘  │
│                                          │
│ ┌─ If Copy From ─────────────────────┐  │
│ │ Source Field: [PID.8 🔍]           │  │
│ │ (field search)                      │  │
│ └────────────────────────────────────┘  │
│                                          │
│ ┌─ If Expression ────────────────────┐  │
│ │ Expression: [{PID.5} - {PID.3}]    │  │
│ │ (template syntax, future)           │  │
│ └────────────────────────────────────┘  │
└──────────────────────────────────────────┘
```

**Backend Mapping (Smart Detection):**

```javascript
// UI Config - Constant
{
  action: 'set_value',
  targetField: 'patient.priority',
  valueType: 'constant',
  value: 'high'
}

// Backend Translation
if (targetField.startsWith('_metadata.')) {
  → {
    action: 'set_metadata',
    metadata: { priority: 'high' }
  }
} else {
  → {
    action: 'set_field',
    field: 'patient.priority',
    value: 'high'
  }
}

// UI Config - Copy From
{
  action: 'set_value',
  targetField: 'patient.gender',
  valueType: 'copy',
  sourceField: 'PID.8'
}

// Backend Translation
→ {
  action: 'copy_field',
  source: 'PID.8',
  target: 'patient.gender'
}
```

**Smart Metadata Detection:**

Instead of user choosing "Set Metadata" vs "Set Field", automatically detect:

```javascript
function translateSetValue(config) {
  const { targetField, valueType, value, sourceField } = config;

  // If copying from another field
  if (valueType === 'copy') {
    return {
      action: 'copy_field',
      source: sourceField,
      target: targetField
    };
  }

  // If target is metadata (starts with _metadata.)
  if (targetField.startsWith('_metadata.')) {
    const metadataKey = targetField.replace('_metadata.', '');
    return {
      action: 'set_metadata',
      metadata: { [metadataKey]: value }
    };
  }

  // Regular field assignment
  return {
    action: 'set_field',
    field: targetField,
    value: value
  };
}
```

**Benefits:**
- ✅ User thinks: "I want to set a value"
- ✅ Don't need to know about _metadata vs regular fields
- ✅ One action for all assignment scenarios
- ✅ Value Type selector makes it clear (constant vs copy)

---

### 4. Delete Field (No Change - Already Clear)

**Current UI is Good:**
```
┌──────────────────────────────────────────┐
│ Action: [Delete Field ▼]                 │
├──────────────────────────────────────────┤
│ Field Path: [patient.ssn 🔍]             │
│ (field search with autocomplete)         │
└──────────────────────────────────────────┘
```

**No changes needed** - this action is clear and simple.

---

### 5. Route To (Smart Destination Selector)

**Current Problem:**
```
Route To: [____________]  ← What do I type here?
```

**Improved UI Design:**

```
┌──────────────────────────────────────────────────────────────┐
│ Action: [Route To ▼]                                         │
├──────────────────────────────────────────────────────────────┤
│ Destination Type: [Interface ▼]                              │
│                   [Interface/Queue/Custom URL/Step]          │
│                                                              │
│ ┌─ If Interface ─────────────────────────────────────────┐  │
│ │ Select Interface: [Epic Production ▼]                  │  │
│ │                   (dropdown from configured interfaces) │  │
│ └────────────────────────────────────────────────────────┘  │
│                                                              │
│ ┌─ If Queue ──────────────────────────────────────────────┐ │
│ │ Queue Name: [high-priority ▼]                          │ │
│ │             [high-priority/standard/geriatrics/vip]    │ │
│ └────────────────────────────────────────────────────────┘ │
│                                                              │
│ ┌─ If Custom URL ─────────────────────────────────────────┐ │
│ │ URL: [https://api.example.com/fhir]                    │ │
│ └────────────────────────────────────────────────────────┘ │
│                                                              │
│ ┌─ If Step (Future: Flow Control) ────────────────────────┐ │
│ │ Jump to Step: [Step 200 - FHIR Validation ▼]          │ │
│ │               (dropdown from pipeline steps)            │ │
│ └────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

**Backend Mapping:**

```javascript
// UI Config - Interface
{
  action: 'route_to',
  destinationType: 'interface',
  interfaceId: 'epic-prod-123'
}

// Backend Translation
→ {
  action: 'route_to',
  destination: 'interface:epic-prod-123',
  queue: ''
}

// UI Config - Queue
{
  action: 'route_to',
  destinationType: 'queue',
  queueName: 'high-priority'
}

// Backend Translation
→ {
  action: 'route_to',
  destination: '',
  queue: 'high-priority'
}

// UI Config - Custom URL
{
  action: 'route_to',
  destinationType: 'custom',
  url: 'https://api.example.com/fhir'
}

// Backend Translation
→ {
  action: 'route_to',
  destination: 'https://api.example.com/fhir',
  queue: ''
}
```

**Dynamic Interface Loading:**

```javascript
// In IfThenElseBuilder.js
async loadInterfaces() {
  const response = await fetch('/api/interfaces');
  const interfaces = await response.json();

  return interfaces.map(intf => ({
    value: intf.id,
    label: `${intf.name} (${intf.type})`
  }));
}

// Populate dropdown when "Interface" selected
if (destinationType === 'interface') {
  const interfaces = await this.loadInterfaces();
  // Show dropdown with interfaces
}
```

**Benefits:**
- ✅ No more guessing what to type
- ✅ Dropdowns show valid options
- ✅ Interface selector loads from database
- ✅ Queue names predefined (or custom)
- ✅ URL validation for custom endpoints
- ✅ Future-proof for step-level flow control

---

## Comparison: Before vs After

### Before (9 Actions - Confusing)

```
1. Continue              ← What's the difference?
2. Log Warning           ← What's the difference?
3. Reject                ← What's the difference?
4. Log Error             ← What's the difference?
5. Set Metadata          ← When to use which?
6. Set Field             ← When to use which?
7. Copy Field            ← When to use which?
8. Delete Field
9. Route To              ← What do I type here?
```

### After (5 Actions - Clear)

```
1. Continue (+ optional logging)    ← Clear: proceeds to next step
2. Stop (+ severity)                ← Clear: halts with error
3. Set Value (+ value type)         ← Clear: assign any value
4. Delete Field                     ← Clear: remove field
5. Route To (+ destination type)    ← Clear: guided selection
```

---

## Implementation Plan

### Phase 1: Backend Updates (1-2 hours)

**Add Missing Actions:**
```go
// services/executors/control/conditional_executor.go

case "log_info":
    message := getStringValue(actionMap, "message")
    log.Printf("   ℹ️  INFO: %s", message)
    // Always continue

case "log_debug":
    message := getStringValue(actionMap, "message")
    log.Printf("   🔍 DEBUG: %s", message)
    // Always continue
```

### Phase 2: UI Translation Layer (2-3 hours)

**Create Action Translator:**
```javascript
// public/js/pipeline/components/IfThenElseActionTranslator.js

class IfThenElseActionTranslator {
    /**
     * Translate UI-friendly action config to backend format
     */
    static toBackend(uiAction) {
        switch (uiAction.action) {
            case 'continue':
                return this.translateContinue(uiAction);

            case 'stop':
                return this.translateStop(uiAction);

            case 'set_value':
                return this.translateSetValue(uiAction);

            case 'delete_field':
                return this.translateDeleteField(uiAction);

            case 'route_to':
                return this.translateRouteTo(uiAction);

            default:
                throw new Error(`Unknown action: ${uiAction.action}`);
        }
    }

    static translateContinue(config) {
        const { logLevel, message } = config;

        if (!logLevel || logLevel === 'none') {
            return { action: 'continue' };
        }

        if (logLevel === 'warning') {
            return { action: 'log_warning', message };
        }

        if (logLevel === 'info') {
            return { action: 'log_info', message };
        }

        if (logLevel === 'debug') {
            return { action: 'log_debug', message };
        }

        return { action: 'continue' };
    }

    static translateStop(config) {
        return {
            action: 'reject',
            errorMessage: config.message || 'Processing stopped',
            severity: config.severity || 'error'
        };
    }

    static translateSetValue(config) {
        const { targetField, valueType, value, sourceField } = config;

        // Copy from another field
        if (valueType === 'copy') {
            return {
                action: 'copy_field',
                source: sourceField,
                target: targetField
            };
        }

        // Set metadata (auto-detect)
        if (targetField.startsWith('_metadata.')) {
            const metadataKey = targetField.replace('_metadata.', '');
            return {
                action: 'set_metadata',
                metadata: { [metadataKey]: value }
            };
        }

        // Regular field assignment
        return {
            action: 'set_field',
            field: targetField,
            value: value
        };
    }

    static translateDeleteField(config) {
        return {
            action: 'delete_field',
            field: config.targetField
        };
    }

    static translateRouteTo(config) {
        const { destinationType, interfaceId, queueName, url, stepId } = config;

        let destination = '';
        let queue = '';

        if (destinationType === 'interface') {
            destination = `interface:${interfaceId}`;
        } else if (destinationType === 'queue') {
            queue = queueName;
        } else if (destinationType === 'custom') {
            destination = url;
        } else if (destinationType === 'step') {
            destination = `step:${stepId}`;
        }

        return {
            action: 'route_to',
            destination,
            queue
        };
    }

    /**
     * Translate backend config to UI-friendly format
     */
    static fromBackend(backendAction) {
        switch (backendAction.action) {
            case 'continue':
                return {
                    action: 'continue',
                    logLevel: 'none',
                    message: ''
                };

            case 'log_warning':
            case 'log_info':
            case 'log_debug':
                return {
                    action: 'continue',
                    logLevel: backendAction.action.replace('log_', ''),
                    message: backendAction.message || ''
                };

            case 'reject':
            case 'log_error':
                return {
                    action: 'stop',
                    message: backendAction.errorMessage || backendAction.message,
                    severity: backendAction.severity || 'error'
                };

            case 'set_metadata':
                const metadataKey = Object.keys(backendAction.metadata)[0];
                return {
                    action: 'set_value',
                    targetField: `_metadata.${metadataKey}`,
                    valueType: 'constant',
                    value: backendAction.metadata[metadataKey]
                };

            case 'set_field':
            case 'set_value':
                return {
                    action: 'set_value',
                    targetField: backendAction.field,
                    valueType: 'constant',
                    value: backendAction.value
                };

            case 'copy_field':
                return {
                    action: 'set_value',
                    targetField: backendAction.target,
                    valueType: 'copy',
                    sourceField: backendAction.source
                };

            case 'delete_field':
                return {
                    action: 'delete_field',
                    targetField: backendAction.field
                };

            case 'route_to':
                let destinationType = 'custom';
                let interfaceId = '';
                let queueName = backendAction.queue || '';
                let url = backendAction.destination || '';

                if (backendAction.destination?.startsWith('interface:')) {
                    destinationType = 'interface';
                    interfaceId = backendAction.destination.replace('interface:', '');
                }

                return {
                    action: 'route_to',
                    destinationType,
                    interfaceId,
                    queueName,
                    url
                };

            default:
                return backendAction;
        }
    }
}
```

### Phase 3: Update IfThenElseBuilder UI (3-4 hours)

**Update Action Options:**
```javascript
createActionOptions(selectedAction) {
    const actions = [
        { value: 'continue', label: 'Continue', icon: 'arrow-right', color: '#10b981' },
        { value: 'stop', label: 'Stop', icon: 'ban', color: '#ef4444' },
        { value: 'set_value', label: 'Set Value', icon: 'edit', color: '#3b82f6' },
        { value: 'delete_field', label: 'Delete Field', icon: 'trash', color: '#ef4444' },
        { value: 'route_to', label: 'Route To', icon: 'directions', color: '#8b5cf6' }
    ];

    return actions.map(a => `
        <option value="${a.value}" ${selectedAction === a.value ? 'selected' : ''}>
            ${a.label}
        </option>
    `).join('');
}
```

**Update Action Config Rendering:**
```javascript
updateActionConfig(index, actionType, actionData) {
    const container = document.getElementById(`${actionType === 'onTrue' ? 'then' : 'else'}-config-${index}`);
    if (!container) return;

    container.innerHTML = '';

    switch (actionData.action) {
        case 'continue':
            container.innerHTML = `
                <select class="ifthen-select-compact" data-field="logLevel" data-action-type="${actionType}" data-index="${index}">
                    <option value="none" ${!actionData.logLevel || actionData.logLevel === 'none' ? 'selected' : ''}>No logging</option>
                    <option value="info" ${actionData.logLevel === 'info' ? 'selected' : ''}>Log Info</option>
                    <option value="warning" ${actionData.logLevel === 'warning' ? 'selected' : ''}>Log Warning</option>
                    <option value="debug" ${actionData.logLevel === 'debug' ? 'selected' : ''}>Log Debug</option>
                </select>
                <input type="text" class="ifthen-input-compact ${!actionData.logLevel || actionData.logLevel === 'none' ? 'ifthen-hidden' : ''}"
                       placeholder="Log message"
                       value="${actionData.message || ''}"
                       data-field="message" data-action-type="${actionType}" data-index="${index}"
                       id="continue-message-${actionType}-${index}">
            `;
            break;

        case 'stop':
            container.innerHTML = `
                <input type="text" class="ifthen-input-compact" placeholder="Error message (required)"
                       value="${actionData.message || ''}"
                       data-field="message" data-action-type="${actionType}" data-index="${index}">
                <select class="ifthen-select-compact" data-field="severity" data-action-type="${actionType}" data-index="${index}">
                    <option value="warning" ${actionData.severity === 'warning' ? 'selected' : ''}>Warning</option>
                    <option value="error" ${actionData.severity === 'error' ? 'selected' : ''}>Error</option>
                    <option value="fatal" ${actionData.severity === 'fatal' ? 'selected' : ''}>Fatal</option>
                </select>
            `;
            break;

        case 'set_value':
            container.innerHTML = `
                <div class="field-search-container-inline" id="set-value-target-${actionType}-${index}"></div>
                <select class="ifthen-select-compact" data-field="valueType" data-action-type="${actionType}" data-index="${index}">
                    <option value="constant" ${!actionData.valueType || actionData.valueType === 'constant' ? 'selected' : ''}>Constant Value</option>
                    <option value="copy" ${actionData.valueType === 'copy' ? 'selected' : ''}>Copy From Field</option>
                </select>
                <div id="set-value-input-${actionType}-${index}"></div>
            `;

            // Initialize target field search
            setTimeout(() => {
                this.initializeFieldSearch(`set-value-target-${actionType}-${index}`, actionData.targetField || '', (value) => {
                    actionData.targetField = value;
                });

                // Initialize value input (constant or copy)
                this.updateSetValueInput(index, actionType, actionData);
            }, 0);
            break;

        case 'delete_field':
            const deleteFieldDiv = document.createElement('div');
            deleteFieldDiv.className = 'field-search-container-inline';
            deleteFieldDiv.id = `delete-field-target-${actionType}-${index}`;
            container.appendChild(deleteFieldDiv);

            setTimeout(() => {
                this.initializeFieldSearch(`delete-field-target-${actionType}-${index}`, actionData.targetField || '', (value) => {
                    actionData.targetField = value;
                });
            }, 0);
            break;

        case 'route_to':
            container.innerHTML = `
                <select class="ifthen-select-compact" data-field="destinationType" data-action-type="${actionType}" data-index="${index}">
                    <option value="interface" ${actionData.destinationType === 'interface' ? 'selected' : ''}>Interface</option>
                    <option value="queue" ${actionData.destinationType === 'queue' ? 'selected' : ''}>Queue</option>
                    <option value="custom" ${actionData.destinationType === 'custom' ? 'selected' : ''}>Custom URL</option>
                </select>
                <div id="route-to-input-${actionType}-${index}"></div>
            `;

            setTimeout(() => {
                this.updateRouteToInput(index, actionType, actionData);
            }, 0);
            break;
    }
}

updateSetValueInput(index, actionType, actionData) {
    const container = document.getElementById(`set-value-input-${actionType}-${index}`);
    if (!container) return;

    container.innerHTML = '';

    if (actionData.valueType === 'copy') {
        // Field search for source
        const div = document.createElement('div');
        div.className = 'field-search-container-inline';
        div.id = `set-value-source-${actionType}-${index}`;
        container.appendChild(div);

        setTimeout(() => {
            this.initializeFieldSearch(`set-value-source-${actionType}-${index}`, actionData.sourceField || '', (value) => {
                actionData.sourceField = value;
            });
        }, 0);
    } else {
        // Plain input for constant value
        const input = document.createElement('input');
        input.type = 'text';
        input.className = 'ifthen-input-compact';
        input.placeholder = 'Value';
        input.value = actionData.value || '';
        input.dataset.field = 'value';
        input.dataset.actionType = actionType;
        input.dataset.index = index;
        container.appendChild(input);
    }
}

async updateRouteToInput(index, actionType, actionData) {
    const container = document.getElementById(`route-to-input-${actionType}-${index}`);
    if (!container) return;

    container.innerHTML = '';

    if (actionData.destinationType === 'interface') {
        // Load interfaces from API
        const interfaces = await this.loadInterfaces();
        const select = document.createElement('select');
        select.className = 'ifthen-select-compact';
        select.dataset.field = 'interfaceId';
        select.dataset.actionType = actionType;
        select.dataset.index = index;
        select.innerHTML = interfaces.map(intf => `
            <option value="${intf.id}" ${actionData.interfaceId === intf.id ? 'selected' : ''}>
                ${intf.name}
            </option>
        `).join('');
        container.appendChild(select);

    } else if (actionData.destinationType === 'queue') {
        const select = document.createElement('select');
        select.className = 'ifthen-select-compact';
        select.dataset.field = 'queueName';
        select.dataset.actionType = actionType;
        select.dataset.index = index;
        select.innerHTML = `
            <option value="high-priority" ${actionData.queueName === 'high-priority' ? 'selected' : ''}>High Priority</option>
            <option value="standard" ${actionData.queueName === 'standard' ? 'selected' : ''}>Standard</option>
            <option value="low-priority" ${actionData.queueName === 'low-priority' ? 'selected' : ''}>Low Priority</option>
            <option value="geriatrics" ${actionData.queueName === 'geriatrics' ? 'selected' : ''}>Geriatrics</option>
            <option value="vip" ${actionData.queueName === 'vip' ? 'selected' : ''}>VIP</option>
        `;
        container.appendChild(select);

    } else if (actionData.destinationType === 'custom') {
        const input = document.createElement('input');
        input.type = 'url';
        input.className = 'ifthen-input-compact';
        input.placeholder = 'https://api.example.com/endpoint';
        input.value = actionData.url || '';
        input.dataset.field = 'url';
        input.dataset.actionType = actionType;
        input.dataset.index = index;
        container.appendChild(input);
    }
}

async loadInterfaces() {
    try {
        const response = await fetch('/api/interfaces');
        const data = await response.json();
        return data.interfaces || [];
    } catch (err) {
        console.error('Failed to load interfaces:', err);
        return [];
    }
}
```

### Phase 4: Integration with PropertiesPanel (1 hour)

**Update Save Handler:**
```javascript
// In PropertiesPanel.js - collectFormData()

if (this.ifThenElseBuilder && (step.stepType === 'pre.logic' || ...)) {
    const uiConfig = this.ifThenElseBuilder.getConfig();

    // Translate UI config to backend format
    const backendConfig = {
        conditions: uiConfig.conditions.map(cond => ({
            condition: cond.condition,
            then_actions: [IfThenElseActionTranslator.toBackend(cond.onTrue)],
            else_actions: [IfThenElseActionTranslator.toBackend(cond.onFalse)]
        }))
    };

    step.config = backendConfig;
}
```

---

## Benefits Summary

### For Users

| Before | After | Improvement |
|--------|-------|-------------|
| 9 confusing actions | 5 clear actions | **44% fewer choices** |
| "Continue vs Log Warning?" | "Continue (with optional logging)" | **No confusion** |
| "Set Metadata vs Set Field?" | "Set Value (auto-detects)" | **Don't need to know internals** |
| "What's a valid route?" | "Dropdown of interfaces/queues" | **Guided selection** |
| Manual typing | Autocomplete everywhere | **Fewer errors** |

### For Developers

| Before | After | Improvement |
|--------|-------|-------------|
| UI exposes backend details | UI hides complexity | **Better abstraction** |
| No translation layer | Smart translator | **Flexibility to change backend** |
| Hard-coded dropdowns | Dynamic from API | **Always in sync** |
| 9 action renderers | 5 action renderers | **Less code to maintain** |

---

## Timeline

| Phase | Task | Time | Priority |
|-------|------|------|----------|
| 1 | Add `log_info`, `log_debug` to backend | 30 min | Medium |
| 2 | Create IfThenElseActionTranslator | 2 hours | HIGH |
| 3 | Update IfThenElseBuilder UI | 3 hours | HIGH |
| 4 | Integrate with PropertiesPanel | 1 hour | HIGH |
| 5 | Testing | 2 hours | HIGH |
| **Total** | | **8.5 hours** | |

---

## Questions for User

### 1. Action Consolidation
**Do you approve consolidating 9 actions → 5 actions as proposed?**
- Continue (+ optional logging)
- Stop (+ severity)
- Set Value (+ value type)
- Delete Field
- Route To (+ destination type)

### 2. Route To Destinations
**What routing destinations do you want to support?**
- ✅ Interface (dropdown from configured interfaces)
- ✅ Queue (predefined queues: high-priority, standard, etc.)
- ✅ Custom URL (for external endpoints)
- ⏳ Step (for flow control - future feature)

### 3. Set Value Modes
**Do you want to add "Expression" mode for Set Value?**
```
Set Value: patient.fullName
Value Type: Expression
Expression: {PID.5.1} + " " + {PID.5.2}
```

This would allow concatenation, calculations, etc. without scripting.

### 4. Implementation Priority
**Should we implement this consolidation now or after testing current UI?**
- **Option A:** Implement now (cleaner from the start)
- **Option B:** Test current UI, then consolidate based on feedback

---

**Status:** 📋 AWAITING USER APPROVAL
**Recommended:** Proceed with consolidation (cleaner UX)

---

**Created By:** Claude Code
**Date:** December 28, 2025
