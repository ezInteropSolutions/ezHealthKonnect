# Source/Target Configuration Synchronization Strategy

## Problem Statement

Currently, source and target configuration forms exist in multiple places:
1. **Wizard Step 1** - For creating new interfaces
2. **Edit Interface Modal** - For editing existing interfaces
3. Potentially future locations (API testing, quick config, etc.)

**Issue**: Any changes to configuration options (like the HTTP authentication we just added) must be manually replicated across all locations, leading to:
- Code duplication
- Inconsistencies between forms
- Maintenance burden
- High risk of bugs when one location is updated but not others

## Proposed Solution: Shared Component Library

### Architecture Overview

```
┌─────────────────────────────────────────┐
│   InterfaceConfigComponents.js          │
│   (Shared Component Library)            │
├─────────────────────────────────────────┤
│ • getSourceTypeSelector()               │
│ • getSourceConnectivitySelector()       │
│ • getSourceConfigPanel(connectivity)    │
│ • getTargetTypeSelector()               │
│ • getTargetConnectivitySelector()       │
│ • getTargetConfigPanel(connectivity)    │
│ • getHttpAuthConfig()                   │
│ • getHttpAuthDetailsPanel()             │
│ • getTcpMllpConfig()                    │
│ • getDatabaseConfig()                   │
│ • getFileConfig()                       │
└─────────────────────────────────────────┘
           ▲                    ▲
           │                    │
    ┌──────┴──────┐      ┌─────┴──────┐
    │   Wizard    │      │    Edit    │
    │   Step 1    │      │   Modal    │
    └─────────────┘      └────────────┘
```

### Implementation Plan

#### Phase 1: Extract Shared Components (Week 1)

**File Structure**:
```
public/js/
├── components/
│   ├── InterfaceConfigComponents.js  (NEW - Shared form generators)
│   ├── modal-components.js           (Existing - Uses shared components)
│   └── ...
├── wizard/
│   └── optimized/
│       └── WizardView.js             (Refactored - Uses shared components)
└── interfaces.js                     (Refactored - Uses shared components)
```

**New File**: `public/js/components/InterfaceConfigComponents.js`

```javascript
/**
 * Shared Interface Configuration Components
 * Single source of truth for source/target configuration forms
 */
class InterfaceConfigComponents {

    /**
     * Get source type selector dropdown
     */
    static getSourceTypeSelector(selectedValue = '', config = {}) {
        return `
            <div class="form-group">
                <label for="${config.idPrefix || ''}sourceType" class="form-label required">
                    Source Format
                </label>
                <select id="${config.idPrefix || ''}sourceType"
                        class="form-control"
                        name="sourceType"
                        required>
                    <option value="">Select source format...</option>
                    <option value="hl7v2" ${selectedValue === 'hl7v2' ? 'selected' : ''}>HL7 v2.x</option>
                    <option value="fhir" ${selectedValue === 'fhir' ? 'selected' : ''}>FHIR</option>
                    <option value="cda" ${selectedValue === 'cda' ? 'selected' : ''}>CDA/C-CDA</option>
                    <option value="x12" ${selectedValue === 'x12' ? 'selected' : ''}>X12 (Claims)</option>
                    <option value="json" ${selectedValue === 'json' ? 'selected' : ''}>JSON</option>
                    <option value="xml" ${selectedValue === 'xml' ? 'selected' : ''}>XML</option>
                    <option value="csv" ${selectedValue === 'csv' ? 'selected' : ''}>CSV</option>
                </select>
                ${config.showHint !== false ? '<div class="form-hint">💡 OOB: HL7 v2.x is most common in healthcare</div>' : ''}
            </div>
        `;
    }

    /**
     * Get source connectivity selector dropdown
     */
    static getSourceConnectivitySelector(selectedValue = '', config = {}) {
        return `
            <div class="form-group">
                <label for="${config.idPrefix || ''}sourceConnectivity" class="form-label required">
                    Connectivity
                </label>
                <select id="${config.idPrefix || ''}sourceConnectivity"
                        class="form-control"
                        name="sourceConnectivity"
                        required>
                    <option value="">Select connectivity...</option>
                    <option value="tcp" ${selectedValue === 'tcp' ? 'selected' : ''}>TCP/MLLP</option>
                    <option value="http" ${selectedValue === 'http' ? 'selected' : ''}>HTTP/REST</option>
                    <option value="file" ${selectedValue === 'file' ? 'selected' : ''}>File Listener</option>
                    <option value="database" ${selectedValue === 'database' ? 'selected' : ''}>Database Query</option>
                    <option value="sftp" ${selectedValue === 'sftp' ? 'selected' : ''}>SFTP</option>
                    <option value="rabbitmq" ${selectedValue === 'rabbitmq' ? 'selected' : ''}>RabbitMQ</option>
                    <option value="kafka" ${selectedValue === 'kafka' ? 'selected' : ''}>Apache Kafka</option>
                </select>
                ${config.showHint !== false ? '<div class="form-hint">💡 OOB: TCP/MLLP for HL7, HTTP for FHIR</div>' : ''}
            </div>
        `;
    }

    /**
     * Get source configuration panel (dynamic based on connectivity)
     * This delegates to connectivity-specific methods
     */
    static getSourceConfigPanel(connectivity, sourceType, config = {}, formConfig = {}) {
        const idPrefix = formConfig.idPrefix || '';

        switch (connectivity) {
            case 'tcp':
                return this.getTcpMllpConfig('source', config, idPrefix);
            case 'http':
                const isFhirReceiver = (sourceType === 'fhir');
                if (isFhirReceiver) {
                    return this.getFhirReceiverConfig(config, idPrefix);
                } else {
                    return this.getHttpConfig('source', config, idPrefix);
                }
            case 'file':
                return this.getFileConfig('source', config, idPrefix);
            case 'database':
                return this.getDatabaseConfig('source', config, idPrefix);
            case 'sftp':
                return this.getSftpConfig('source', config, idPrefix);
            case 'rabbitmq':
                return this.getRabbitMQConfig('source', config, idPrefix);
            case 'kafka':
                return this.getKafkaConfig('source', config, idPrefix);
            default:
                return '<div class="config-placeholder">Select connectivity type to configure</div>';
        }
    }

    /**
     * TCP/MLLP Configuration (Source or Target)
     */
    static getTcpMllpConfig(direction = 'source', config = {}, idPrefix = '') {
        const isSource = direction === 'source';
        const prefix = idPrefix + direction;

        return `
            <div class="config-group">
                <h4>TCP/MLLP Configuration</h4>
                ${isSource ? `
                    <div class="form-row">
                        <div class="form-group">
                            <label for="${prefix}Host" class="form-label required">Host</label>
                            <input type="text" id="${prefix}Host" class="form-control"
                                   value="${config.host || 'localhost'}"
                                   placeholder="localhost">
                            <div class="form-hint">💡 OOB: localhost for development</div>
                        </div>
                        <div class="form-group">
                            <label for="${prefix}Port" class="form-label required">Port</label>
                            <input type="number" id="${prefix}Port" class="form-control"
                                   value="${config.port || 2575}"
                                   min="1" max="65535"
                                   placeholder="2575">
                            <div class="form-hint">💡 OOB: 2575 is standard HL7 port</div>
                        </div>
                    </div>
                ` : `
                    <div class="form-group">
                        <label for="${prefix}Endpoint" class="form-label required">Remote Endpoint</label>
                        <input type="text" id="${prefix}Endpoint" class="form-control"
                               value="${config.endpoint || 'localhost:2575'}"
                               placeholder="hostname:port">
                    </div>
                `}
                <div class="form-row">
                    <div class="form-group">
                        <label for="${prefix}Timeout" class="form-label">Timeout (ms)</label>
                        <input type="number" id="${prefix}Timeout" class="form-control"
                               value="${config.timeout || 30000}"
                               min="1000" max="300000">
                    </div>
                    <div class="form-group">
                        <label for="${prefix}Encoding" class="form-label">Encoding</label>
                        <select id="${prefix}Encoding" class="form-control">
                            <option value="utf8" ${config.encoding === 'utf8' ? 'selected' : ''}>UTF-8</option>
                            <option value="ascii" ${config.encoding === 'ascii' ? 'selected' : ''}>ASCII</option>
                            <option value="latin1" ${config.encoding === 'latin1' ? 'selected' : ''}>Latin-1</option>
                        </select>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * HTTP/REST Configuration (Source or Target)
     */
    static getHttpConfig(direction = 'source', config = {}, idPrefix = '') {
        const prefix = idPrefix + direction;

        return `
            <div class="config-group">
                <h4>HTTP/REST Configuration</h4>
                <div class="form-group">
                    <label for="${prefix}Endpoint" class="form-label required">Endpoint URL</label>
                    <input type="url" id="${prefix}Endpoint" class="form-control"
                           value="${config.endpoint || 'http://localhost:3000/api/hl7/receive'}"
                           placeholder="http://localhost:3000/api/hl7/receive">
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label for="${prefix}Method" class="form-label">HTTP Method</label>
                        <select id="${prefix}Method" class="form-control">
                            <option value="POST" ${config.method === 'POST' ? 'selected' : ''}>POST</option>
                            <option value="PUT" ${config.method === 'PUT' ? 'selected' : ''}>PUT</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label for="${prefix}ContentType" class="form-label">Content Type</label>
                        <select id="${prefix}ContentType" class="form-control">
                            <option value="text/plain" ${config.contentType === 'text/plain' ? 'selected' : ''}>text/plain</option>
                            <option value="application/hl7-v2" ${config.contentType === 'application/hl7-v2' ? 'selected' : ''}>application/hl7-v2</option>
                            <option value="application/json" ${config.contentType === 'application/json' ? 'selected' : ''}>application/json</option>
                        </select>
                    </div>
                </div>

                <!-- HTTP Authentication (Universal) -->
                ${this.getHttpAuthConfig(config, prefix)}
            </div>
        `;
    }

    /**
     * HTTP Authentication Configuration (Universal for all HTTP connections)
     */
    static getHttpAuthConfig(config = {}, idPrefix = '') {
        return `
            <div class="config-group http-auth-config">
                <h4>🔐 HTTP Authentication</h4>

                <div class="form-group">
                    <label for="${idPrefix}HttpAuthType" class="form-label required">
                        Authentication Type
                        <a href="#" class="help-link" onclick="event.preventDefault(); alert('Secure your HTTP endpoint:\\n\\n• No Auth - Development only\\n• API Key - Simple token\\n• Basic Auth - Username/password\\n• Bearer Token - OAuth tokens\\n• OAuth 2.0 - Industry standard\\n• mTLS - Certificate-based');" title="Click for help">ⓘ</a>
                    </label>
                    <select id="${idPrefix}HttpAuthType" class="form-control http-auth-type-selector">
                        <option value="none" ${!config.authType || config.authType === 'none' ? 'selected' : ''}>No Authentication (Development)</option>
                        <option value="api_key" ${config.authType === 'api_key' ? 'selected' : ''}>API Key (Header)</option>
                        <option value="basic" ${config.authType === 'basic' ? 'selected' : ''}>Basic Authentication</option>
                        <option value="bearer" ${config.authType === 'bearer' ? 'selected' : ''}>Bearer Token</option>
                        <option value="oauth2" ${config.authType === 'oauth2' ? 'selected' : ''}>OAuth 2.0</option>
                        <option value="mtls" ${config.authType === 'mtls' ? 'selected' : ''}>Mutual TLS (Certificate)</option>
                    </select>
                    <div class="form-hint">Choose based on security requirements</div>
                </div>

                <!-- Auth Details Panel -->
                <div id="${idPrefix}HttpAuthDetails" class="auth-details-panel">
                    ${this.getHttpAuthDetailsPanel(config.authType || 'none', config, idPrefix)}
                </div>
            </div>
        `;
    }

    /**
     * HTTP Authentication Details Panel (Dynamic based on auth type)
     */
    static getHttpAuthDetailsPanel(authType, config = {}, idPrefix = '') {
        // Same implementation as in WizardView.js
        // (Copy the full switch statement from lines 827-995)
        // Just replace all IDs with ${idPrefix}authFieldName

        switch (authType) {
            case 'none':
                return `
                    <div class="auth-info">
                        <div class="alert alert-warning">
                            ⚠️ <strong>No Authentication</strong> - Endpoint is publicly accessible.
                        </div>
                    </div>
                `;
            case 'api_key':
                return `
                    <div class="auth-fields">
                        <div class="form-group">
                            <label for="${idPrefix}AuthApiKeyHeader" class="form-label">Header Name</label>
                            <input type="text" id="${idPrefix}AuthApiKeyHeader" class="form-control"
                                   value="${config.apiKeyHeader || 'X-API-Key'}">
                        </div>
                        <div class="form-group">
                            <label for="${idPrefix}AuthApiKeyValue" class="form-label">Expected API Key</label>
                            <input type="password" id="${idPrefix}AuthApiKeyValue" class="form-control"
                                   value="${config.apiKeyValue || ''}">
                        </div>
                    </div>
                `;
            // ... similar for basic, bearer, oauth2, mtls
            default:
                return '';
        }
    }

    /**
     * FHIR Receiver Configuration
     */
    static getFhirReceiverConfig(config = {}, idPrefix = '') {
        // Extract from WizardView.js lines 570-766
        // Just add idPrefix to all IDs
        return `
            <div class="config-group fhir-receiver-config">
                <h4>🏥 FHIR Receiver Configuration</h4>
                <!-- All FHIR receiver fields with ${idPrefix} -->
                ${this.getHttpAuthConfig(config, idPrefix)}
            </div>
        `;
    }

    /**
     * Initialize dynamic behavior for auth type changes
     * Call this after rendering forms
     */
    static attachAuthTypeListeners(containerElement, idPrefix = '') {
        const authTypeSelect = containerElement.querySelector(`#${idPrefix}HttpAuthType`);
        if (authTypeSelect) {
            authTypeSelect.addEventListener('change', (e) => {
                const authType = e.target.value;
                const authDetailsPanel = containerElement.querySelector(`#${idPrefix}HttpAuthDetails`);
                if (authDetailsPanel) {
                    authDetailsPanel.innerHTML = this.getHttpAuthDetailsPanel(authType, {}, idPrefix);
                }
            });
        }
    }
}

// Make it globally available
window.InterfaceConfigComponents = InterfaceConfigComponents;
```

#### Phase 2: Refactor Wizard to Use Shared Components

**WizardView.js** changes:
```javascript
// OLD (duplicated code):
getSourceConfigPanel(connectivity, config, sourceType) {
    switch (connectivity) {
        case 'tcp':
            return `<div>...TCP config HTML...</div>`;
        case 'http':
            // ... 200 lines of HTML
    }
}

// NEW (delegates to shared component):
getSourceConfigPanel(connectivity, config, sourceType) {
    return InterfaceConfigComponents.getSourceConfigPanel(
        connectivity,
        sourceType,
        config,
        { idPrefix: 'wizard_' }  // Prefix to avoid ID conflicts
    );
}

getHttpAuthConfig(config) {
    return InterfaceConfigComponents.getHttpAuthConfig(config, 'wizard_');
}

getHttpAuthDetailsPanel(authType, config) {
    return InterfaceConfigComponents.getHttpAuthDetailsPanel(authType, config, 'wizard_');
}
```

#### Phase 3: Refactor Edit Modal to Use Shared Components

**modal-components.js** changes:
```javascript
function loadEditModal() {
    container.innerHTML = `
        <div class="modal-overlay" id="editModal">
            <div class="modal-content large">
                <form id="editInterfaceForm">
                    <!-- Source Configuration -->
                    <div id="editSourceTypeContainer"></div>
                    <div id="editSourceConnectivityContainer"></div>
                    <div id="editSourceConfigContainer"></div>

                    <!-- Target Configuration -->
                    <div id="editTargetTypeContainer"></div>
                    <div id="editTargetConnectivityContainer"></div>
                    <div id="editTargetConfigContainer"></div>
                </form>
            </div>
        </div>
    `;

    // Populate forms using shared components
    document.getElementById('editSourceTypeContainer').innerHTML =
        InterfaceConfigComponents.getSourceTypeSelector('', { idPrefix: 'edit' });

    document.getElementById('editSourceConnectivityContainer').innerHTML =
        InterfaceConfigComponents.getSourceConnectivitySelector('', { idPrefix: 'edit' });
}

function populateEditForm(interfaceData) {
    // Re-render config panel when data is loaded
    document.getElementById('editSourceConfigContainer').innerHTML =
        InterfaceConfigComponents.getSourceConfigPanel(
            interfaceData.sourceConnectivity,
            interfaceData.sourceType,
            interfaceData.sourceConfig,
            { idPrefix: 'edit' }
        );

    // Attach event listeners
    InterfaceConfigComponents.attachAuthTypeListeners(
        document.getElementById('editSourceConfigContainer'),
        'edit'
    );
}
```

### Benefits of This Approach

✅ **Single Source of Truth**: All forms generated from one place
✅ **Automatic Sync**: Changes propagate to all usages automatically
✅ **ID Conflict Prevention**: `idPrefix` parameter prevents duplicate IDs
✅ **Testable**: Can unit test component generators independently
✅ **Extensible**: Easy to add new connectivity types
✅ **Maintainable**: Fix once, works everywhere
✅ **Consistent UX**: Identical behavior across wizard and edit

### ID Prefix Strategy

Different contexts use different prefixes to avoid conflicts:

| Context | ID Prefix | Example ID |
|---------|-----------|------------|
| Wizard Step 1 | `wizard_` | `wizard_sourceHost` |
| Edit Modal | `edit` | `editsourceHost` |
| API Test Form | `test_` | `test_sourceHost` |

### Event Handling Strategy

```javascript
// Shared event handler setup
InterfaceConfigComponents.attachAllListeners(containerElement, idPrefix, callbacks) {
    // Auth type change
    this.attachAuthTypeListeners(containerElement, idPrefix);

    // Connectivity change
    this.attachConnectivityChangeListeners(containerElement, idPrefix, callbacks.onConnectivityChange);

    // Source type change
    this.attachSourceTypeListeners(containerElement, idPrefix, callbacks.onSourceTypeChange);
}

// Usage in Wizard:
InterfaceConfigComponents.attachAllListeners(
    container,
    'wizard_',
    {
        onConnectivityChange: (value) => this.updateSourceConfigPanel(container),
        onSourceTypeChange: (value) => this.updateSourceConfigPanel(container)
    }
);

// Usage in Edit Modal:
InterfaceConfigComponents.attachAllListeners(
    container,
    'edit',
    {
        onConnectivityChange: (value) => reloadEditConfigPanel(),
        onSourceTypeChange: (value) => reloadEditConfigPanel()
    }
);
```

## Implementation Timeline

### Week 1: Foundation
- [ ] Create `InterfaceConfigComponents.js` class
- [ ] Extract TCP/MLLP config method
- [ ] Extract HTTP config method
- [ ] Extract HTTP auth methods
- [ ] Add ID prefix support

### Week 2: Wizard Refactor
- [ ] Refactor WizardView source config to use shared components
- [ ] Refactor WizardView target config to use shared components
- [ ] Test wizard thoroughly
- [ ] Fix any ID conflicts

### Week 3: Edit Modal Refactor
- [ ] Refactor edit modal to use shared components
- [ ] Add event listeners for dynamic updates
- [ ] Test edit functionality
- [ ] Ensure data loads correctly

### Week 4: Additional Connectivity Types
- [ ] Add FHIR receiver shared component
- [ ] Add Database config shared component
- [ ] Add File config shared component
- [ ] Add SFTP/FTP config shared component
- [ ] Add Message Queue configs (RabbitMQ, Kafka)

## Alternative Approach: Web Components

For a more modern approach, could use **Custom Elements (Web Components)**:

```html
<!-- Usage in Wizard -->
<interface-source-config
    connectivity="tcp"
    source-type="hl7v2"
    id-prefix="wizard_"
    config='{"host": "localhost", "port": 2575}'>
</interface-source-config>

<!-- Usage in Edit Modal -->
<interface-source-config
    connectivity="http"
    source-type="fhir"
    id-prefix="edit"
    config='{"endpoint": "http://example.com", "authType": "oauth2"}'>
</interface-source-config>
```

**Pros**:
- Native browser support
- True encapsulation
- Shadow DOM prevents style conflicts
- Can use in any context (React, Vue, plain JS)

**Cons**:
- More complex implementation
- Requires modern browser (already using ES6+ so OK)
- Learning curve for team

## Recommendation

**Phase 1-3 (Class-based shared components)** is recommended because:
1. Easier migration from current code
2. No new technology/patterns to learn
3. Can refactor incrementally (one connectivity type at a time)
4. Full control over rendering and behavior

**Web Components** could be Phase 2 if team wants more modularity.

## Questions for Decision

1. **Timing**: Should we refactor now or wait until more features are stable?
2. **Scope**: Start with just auth config, or refactor all connectivity types?
3. **Breaking Changes**: Can we change HTML IDs (with prefixes) or must we maintain exact compatibility?
4. **Technology**: Class-based components or Web Components?

---

**Status**: 📋 Proposal - Awaiting feedback
**Estimated Effort**: 4 weeks (incremental, no downtime)
**Risk Level**: Low (can refactor incrementally, test at each step)
**Maintenance Savings**: High (50%+ reduction in duplicated code)
