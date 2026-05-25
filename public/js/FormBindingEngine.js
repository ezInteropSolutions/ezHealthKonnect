/**
 * FormBindingEngine.js - Automatic Form-to-Model Synchronization
 *
 * OOB/MVC Pattern: Uses FormFieldSchema as single source of truth
 * Provides automatic bidirectional binding between forms and data models
 *
 * Usage:
 *   const engine = new FormBindingEngine(container, FormFieldSchema);
 *   engine.extractAll(sourceConnectivity, targetConnectivity); // Returns complete data object
 *   engine.populate(data, sourceConnectivity, targetConnectivity); // Fills form from data
 */

class FormBindingEngine {
    constructor(container, schema, idPrefix = '') {
        this.container = container;
        this.schema = schema;
        this.idPrefix = idPrefix;
        this.changeListeners = [];
    }

    /**
     * Get element ID with prefix applied
     */
    getElementId(fieldId) {
        if (!this.idPrefix) return fieldId;
        return this.idPrefix + this.capitalize(fieldId);
    }

    /**
     * Get element by field ID
     * Supports alternative IDs for compatibility between wizard and shared components
     */
    getElement(fieldId, field = null) {
        // Try primary ID first
        const id = this.getElementId(fieldId);
        let element = this.container.querySelector(`#${id}`);

        // If not found and field has alternative IDs, try those
        if (!element && field && field.altIds) {
            for (const altId of field.altIds) {
                const altElementId = this.getElementId(altId);
                element = this.container.querySelector(`#${altElementId}`);
                if (element) break;
            }
        }

        return element;
    }

    /**
     * Extract value from a single field
     */
    extractFieldValue(field) {
        const element = this.getElement(field.id, field); // Pass field for altIds support
        if (!element) return undefined;

        switch (field.type) {
            case 'checkbox':
                return element.checked;
            case 'number':
                const numVal = element.value;
                return numVal ? parseInt(numVal, 10) : (field.default || null);
            case 'radio':
                if (element.checked) return field.groupValue;
                return undefined;
            default:
                return element.value || field.default || '';
        }
    }

    /**
     * Extract all basic info fields
     */
    extractBasicInfo() {
        const data = {};
        this.schema.fields.basic.forEach(field => {
            const value = this.extractFieldValue(field);
            if (value !== undefined) {
                this.setNestedValue(data, field.modelPath, value);
            }
        });
        return data;
    }

    /**
     * Extract connectivity-specific fields
     */
    extractConnectivity(direction, connectivity) {
        const data = {};
        const fields = this.schema.getFieldsByConnectivity(direction, connectivity);

        fields.forEach(field => {
            const value = this.extractFieldValue(field);
            if (value !== undefined) {
                this.setNestedValue(data, field.modelPath, value);
            }
        });

        return data;
    }

    /**
     * Extract authentication fields
     */
    extractAuthentication(configPath = 'targetConfig') {
        const data = {};

        // Get auth type first
        const authTypeField = this.schema.fields.authentication.selector[0];
        const authType = this.extractFieldValue(authTypeField);

        if (!authType || authType === 'none') {
            return data;
        }

        this.setNestedValue(data, `${configPath}.authType`, authType);

        // Get auth-specific fields
        const authFields = this.schema.getAuthFields(authType);
        authFields.forEach(field => {
            const value = this.extractFieldValue(field);
            if (value !== undefined) {
                this.setNestedValue(data, `${configPath}.${field.modelPath}`, value);
            }
        });

        return data;
    }

    /**
     * Extract checkbox group values (HTTP methods, FHIR operations, etc.)
     */
    extractCheckboxGroup(fieldArray) {
        const values = [];
        fieldArray.forEach(field => {
            if (field.groupValue) {
                const element = this.getElement(field.id);
                if (element?.checked) {
                    values.push(field.groupValue);
                }
            }
        });
        return values;
    }

    /**
     * Extract all form data based on current selections
     */
    extractAll(sourceConnectivity, targetConnectivity) {
        // Start with basic info
        const data = this.extractBasicInfo();

        // Get sourceType from the extracted data
        const sourceType = data.sourceType;

        // Source connectivity
        if (sourceConnectivity) {
            const sourceData = this.extractConnectivity('source', sourceConnectivity);
            this.mergeDeep(data, sourceData);

            // HTTP methods for HTTP source
            if (sourceConnectivity === 'http') {
                const methods = this.extractCheckboxGroup(this.schema.fields.httpMethods);
                if (methods.length > 0) {
                    data.sourceConfig = data.sourceConfig || {};
                    data.sourceConfig.allowedMethods = methods;
                }
            }

            // FHIR operations for FHIR source
            if (sourceType === 'fhir' && sourceConnectivity === 'http') {
                const operations = this.extractCheckboxGroup(this.schema.fields.fhirOperations);
                if (operations.length > 0) {
                    data.sourceConfig = data.sourceConfig || {};
                    data.sourceConfig.operations = operations;
                }
            }
        }

        // Target type and connectivity
        this.schema.fields.target.type.forEach(field => {
            const value = this.extractFieldValue(field);
            if (value !== undefined) {
                this.setNestedValue(data, field.modelPath, value);
            }
        });

        // Target connectivity
        if (targetConnectivity) {
            const targetData = this.extractConnectivity('target', targetConnectivity);
            this.mergeDeep(data, targetData);

            // Delivery mode for FHIR targets
            if (targetConnectivity === 'http') {
                const bundleRadio = this.getElement('deliveryModeBundle');
                const individualRadio = this.getElement('deliveryModeIndividual');

                if (bundleRadio?.checked) {
                    data.targetConfig = data.targetConfig || {};
                    data.targetConfig.deliveryMode = 'bundle';
                } else if (individualRadio?.checked) {
                    data.targetConfig = data.targetConfig || {};
                    data.targetConfig.deliveryMode = 'individual';

                    // Get selected resources
                    const resources = this.extractCheckboxGroup(this.schema.fields.fhirResources);
                    if (resources.length > 0) {
                        data.targetConfig.individualResources = resources;
                    }
                }

                // Target authentication
                const authData = this.extractAuthentication('targetConfig');
                this.mergeDeep(data, authData);
            }
        }

        return data;
    }

    /**
     * Populate form fields from data object
     */
    populate(data, sourceConnectivity, targetConnectivity) {
        // Basic info
        this.schema.fields.basic.forEach(field => {
            this.setFieldValue(field, this.getNestedValue(data, field.modelPath));
        });

        // Source connectivity fields
        if (sourceConnectivity) {
            const fields = this.schema.getFieldsByConnectivity('source', sourceConnectivity);
            fields.forEach(field => {
                this.setFieldValue(field, this.getNestedValue(data, field.modelPath));
            });

            // HTTP methods
            if (sourceConnectivity === 'http') {
                const methods = this.getNestedValue(data, 'sourceConfig.allowedMethods') || [];
                this.setCheckboxGroup(this.schema.fields.httpMethods, methods);
            }

            // FHIR operations
            const sourceType = this.getNestedValue(data, 'sourceType');
            if (sourceType === 'fhir' && sourceConnectivity === 'http') {
                const operations = this.getNestedValue(data, 'sourceConfig.operations') || [];
                this.setCheckboxGroup(this.schema.fields.fhirOperations, operations);
            }
        }

        // Target type and connectivity
        this.schema.fields.target.type.forEach(field => {
            this.setFieldValue(field, this.getNestedValue(data, field.modelPath));
        });

        // Target connectivity fields
        if (targetConnectivity) {
            const fields = this.schema.getFieldsByConnectivity('target', targetConnectivity);
            fields.forEach(field => {
                this.setFieldValue(field, this.getNestedValue(data, field.modelPath));
            });

            // Authentication
            const authType = this.getNestedValue(data, 'targetConfig.authType');
            if (authType) {
                this.setFieldValue(this.schema.fields.authentication.selector[0], authType);

                const authFields = this.schema.getAuthFields(authType);
                authFields.forEach(field => {
                    this.setFieldValue(field, this.getNestedValue(data, `targetConfig.${field.modelPath}`));
                });
            }

            // Delivery mode
            const deliveryMode = this.getNestedValue(data, 'targetConfig.deliveryMode');
            if (deliveryMode === 'bundle') {
                const el = this.getElement('deliveryModeBundle');
                if (el) el.checked = true;
            } else if (deliveryMode === 'individual') {
                const el = this.getElement('deliveryModeIndividual');
                if (el) el.checked = true;

                const resources = this.getNestedValue(data, 'targetConfig.individualResources') || [];
                this.setCheckboxGroup(this.schema.fields.fhirResources, resources);
            }
        }
    }

    /**
     * Set value for a single field
     */
    setFieldValue(field, value) {
        const element = this.getElement(field.id);
        if (!element) return;

        if (value === undefined || value === null) {
            value = field.default;
        }

        switch (field.type) {
            case 'checkbox':
                element.checked = Boolean(value);
                break;
            case 'radio':
                element.checked = (value === field.groupValue);
                break;
            default:
                element.value = value || '';
        }
    }

    /**
     * Set checkbox group values
     */
    setCheckboxGroup(fieldArray, values) {
        fieldArray.forEach(field => {
            const element = this.getElement(field.id);
            if (element) {
                element.checked = values.includes(field.groupValue);
            }
        });
    }

    /**
     * Setup automatic change detection
     * Calls callback whenever any field changes
     */
    setupAutoSync(callback) {
        const handler = (event) => {
            const target = event.target;
            if (target.matches('input, select, textarea')) {
                callback(event);
            }
        };

        this.container.addEventListener('change', handler);
        this.container.addEventListener('input', handler);

        this.changeListeners.push({ type: 'change', handler });
        this.changeListeners.push({ type: 'input', handler });
    }

    /**
     * Remove all change listeners
     */
    cleanup() {
        this.changeListeners.forEach(({ type, handler }) => {
            this.container.removeEventListener(type, handler);
        });
        this.changeListeners = [];
    }

    /**
     * Validate required fields
     */
    validateRequired(sourceConnectivity, targetConnectivity) {
        const errors = [];

        // Check basic required fields
        this.schema.fields.basic.forEach(field => {
            if (field.required) {
                const value = this.extractFieldValue(field);
                if (!value || (typeof value === 'string' && !value.trim())) {
                    errors.push({
                        fieldId: field.id,
                        label: field.label,
                        message: `${field.label} is required`
                    });
                }
            }
        });

        // Check connectivity-specific required fields
        if (sourceConnectivity) {
            const fields = this.schema.getFieldsByConnectivity('source', sourceConnectivity);
            fields.forEach(field => {
                if (field.required) {
                    const value = this.extractFieldValue(field);
                    if (!value || (typeof value === 'string' && !value.trim())) {
                        errors.push({
                            fieldId: field.id,
                            label: field.label,
                            message: `${field.label} is required`
                        });
                    }
                }
            });
        }

        if (targetConnectivity) {
            const fields = this.schema.getFieldsByConnectivity('target', targetConnectivity);
            fields.forEach(field => {
                if (field.required) {
                    const value = this.extractFieldValue(field);
                    if (!value || (typeof value === 'string' && !value.trim())) {
                        errors.push({
                            fieldId: field.id,
                            label: field.label,
                            message: `${field.label} is required`
                        });
                    }
                }
            });
        }

        return errors;
    }

    // Helper methods
    capitalize(str) {
        return str.charAt(0).toUpperCase() + str.slice(1);
    }

    setNestedValue(obj, path, value) {
        const keys = path.split('.');
        let current = obj;
        for (let i = 0; i < keys.length - 1; i++) {
            if (!current[keys[i]]) {
                current[keys[i]] = {};
            }
            current = current[keys[i]];
        }
        current[keys[keys.length - 1]] = value;
    }

    getNestedValue(obj, path) {
        const keys = path.split('.');
        let current = obj;
        for (const key of keys) {
            if (current === undefined || current === null) return undefined;
            current = current[key];
        }
        return current;
    }

    mergeDeep(target, source) {
        for (const key in source) {
            if (source[key] && typeof source[key] === 'object' && !Array.isArray(source[key])) {
                if (!target[key]) target[key] = {};
                this.mergeDeep(target[key], source[key]);
            } else {
                target[key] = source[key];
            }
        }
        return target;
    }
}

// Export for module systems
if (typeof module !== 'undefined' && module.exports) {
    module.exports = FormBindingEngine;
}
