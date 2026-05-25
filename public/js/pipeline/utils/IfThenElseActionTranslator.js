/**
 * If-Then-Else Action Translator
 * Translates between user-friendly UI actions and backend executor format
 *
 * UI Actions (6):
 * - continue (+ optional logging)
 * - stop (+ error message)
 * - set_value (+ value type: constant/copy)
 * - transform (+ transform type: uppercase/lowercase/trim/concat/replace/regex_replace/substring/split/pad)
 * - delete_field
 * - route_to_step
 *
 * Backend Actions (10+):
 * - continue, log_info, log_warning, log_debug
 * - reject
 * - set_metadata, set_field, copy_field
 * - transform_field
 * - delete_field
 * - route_to_step
 */

class IfThenElseActionTranslator {
    /**
     * Translate UI-friendly action config to backend format
     * @param {Object} uiAction - UI action configuration
     * @returns {Object} Backend action configuration
     */
    static toBackend(uiAction) {
        if (!uiAction || !uiAction.action) {
            throw new Error('Invalid action configuration');
        }

        switch (uiAction.action) {
            case 'continue':
                return this.translateContinue(uiAction);

            case 'stop':
                return this.translateStop(uiAction);

            case 'set_value':
                return this.translateSetValue(uiAction);

            case 'transform':
                return this.translateTransform(uiAction);

            case 'delete_field':
                return this.translateDeleteField(uiAction);

            case 'route_to_step':
                return this.translateRouteToStep(uiAction);

            default:
                // Unknown action, pass through as-is
                console.warn('[IfThenElseActionTranslator] Unknown action:', uiAction.action);
                return uiAction;
        }
    }

    /**
     * Translate Continue action (with optional logging)
     */
    static translateContinue(config) {
        const { logLevel, message } = config;

        // No logging - simple continue
        if (!logLevel || logLevel === 'none') {
            return { action: 'continue' };
        }

        // With logging
        const logAction = `log_${logLevel}`;
        return {
            action: logAction,
            message: message || ''
        };
    }

    /**
     * Translate Stop action (maps to reject)
     */
    static translateStop(config) {
        return {
            action: 'reject',
            errorMessage: config.message || 'Processing stopped',
            severity: config.severity || 'error'
        };
    }

    /**
     * Translate Set Value action (auto-detects backend action)
     * - If target starts with _metadata. → set_metadata
     * - If valueType is 'copy' → copy_field
     * - Otherwise → set_field
     */
    static translateSetValue(config) {
        const { targetField, valueType, value, sourceField } = config;

        if (!targetField) {
            throw new Error('targetField is required for set_value action');
        }

        // Copy from another field
        if (valueType === 'copy') {
            if (!sourceField) {
                throw new Error('sourceField is required when valueType is "copy"');
            }

            return {
                action: 'copy_field',
                source: sourceField,
                target: targetField
            };
        }

        // Set metadata (auto-detect by field prefix)
        if (targetField.startsWith('_metadata.')) {
            const metadataKey = targetField.replace('_metadata.', '');

            // Parse value if it looks like JSON
            let parsedValue = value;
            if (typeof value === 'string' && (value.startsWith('{') || value.startsWith('['))) {
                try {
                    parsedValue = JSON.parse(value);
                } catch (err) {
                    // Keep as string if not valid JSON
                }
            }

            return {
                action: 'set_metadata',
                metadata: { [metadataKey]: parsedValue }
            };
        }

        // Regular field assignment
        return {
            action: 'set_field',
            field: targetField,
            value: value
        };
    }

    /**
     * Translate Transform action
     * Supports: uppercase, lowercase, trim, concat, replace, regex_replace, substring, split, pad_left, pad_right
     */
    static translateTransform(config) {
        const { sourceField, targetField, transformType } = config;

        if (!sourceField) {
            throw new Error('sourceField is required for transform action');
        }
        if (!targetField) {
            throw new Error('targetField is required for transform action');
        }

        const baseAction = {
            action: 'transform_field',
            source: sourceField,
            target: targetField,
            transformType: transformType || 'uppercase'
        };

        // Add transform-specific options
        switch (transformType) {
            case 'concat':
                return {
                    ...baseAction,
                    appendText: config.appendText || '',
                    appendField: config.appendField || ''
                };

            case 'replace':
                return {
                    ...baseAction,
                    findText: config.findText || '',
                    replaceText: config.replaceText || ''
                };

            case 'regex_replace':
                return {
                    ...baseAction,
                    regexPattern: config.regexPattern || '',
                    replaceText: config.replaceText || ''
                };

            case 'substring':
                return {
                    ...baseAction,
                    startIndex: parseInt(config.startIndex, 10) || 0,
                    length: config.length ? parseInt(config.length, 10) : null
                };

            case 'split':
                return {
                    ...baseAction,
                    delimiter: config.delimiter || '',
                    selectIndex: parseInt(config.selectIndex, 10) || 0
                };

            case 'pad_left':
            case 'pad_right':
                return {
                    ...baseAction,
                    padLength: parseInt(config.padLength, 10) || 10,
                    padChar: config.padChar || '0'
                };

            default:
                // Simple transforms: uppercase, lowercase, trim
                return baseAction;
        }
    }

    /**
     * Translate Delete Field action
     */
    static translateDeleteField(config) {
        if (!config.targetField) {
            throw new Error('targetField is required for delete_field action');
        }

        return {
            action: 'delete_field',
            field: config.targetField
        };
    }

    /**
     * Translate Route To Step action
     */
    static translateRouteToStep(config) {
        if (!config.stepId) {
            throw new Error('stepId is required for route_to_step action');
        }

        return {
            action: 'route_to_step',
            stepId: config.stepId
        };
    }

    /**
     * Translate backend config to UI-friendly format
     * @param {Object} backendAction - Backend action configuration
     * @returns {Object} UI action configuration
     */
    static fromBackend(backendAction) {
        if (!backendAction || !backendAction.action) {
            return { action: 'continue', logLevel: 'none', message: '' };
        }

        switch (backendAction.action) {
            case 'continue':
                return {
                    action: 'continue',
                    logLevel: 'none',
                    message: ''
                };

            case 'log_info':
            case 'log_warning':
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
                    message: backendAction.errorMessage || backendAction.message || '',
                    severity: backendAction.severity || 'error'
                };

            case 'set_metadata':
                // Extract first metadata key
                const metadata = backendAction.metadata || {};
                const metadataKey = Object.keys(metadata)[0];

                if (metadataKey) {
                    return {
                        action: 'set_value',
                        targetField: `_metadata.${metadataKey}`,
                        valueType: 'constant',
                        value: metadata[metadataKey]
                    };
                }

                // Fallback if no metadata
                return {
                    action: 'set_value',
                    targetField: '_metadata.',
                    valueType: 'constant',
                    value: ''
                };

            case 'set_field':
            case 'set_value':
                return {
                    action: 'set_value',
                    targetField: backendAction.field || '',
                    valueType: 'constant',
                    value: backendAction.value || ''
                };

            case 'copy_field':
                return {
                    action: 'set_value',
                    targetField: backendAction.target || '',
                    valueType: 'copy',
                    sourceField: backendAction.source || ''
                };

            case 'transform_field':
                return {
                    action: 'transform',
                    sourceField: backendAction.source || '',
                    targetField: backendAction.target || '',
                    transformType: backendAction.transformType || 'uppercase',
                    // Include all transform-specific options
                    appendText: backendAction.appendText || '',
                    appendField: backendAction.appendField || '',
                    findText: backendAction.findText || '',
                    replaceText: backendAction.replaceText || '',
                    regexPattern: backendAction.regexPattern || '',
                    startIndex: backendAction.startIndex || 0,
                    length: backendAction.length || null,
                    delimiter: backendAction.delimiter || '',
                    selectIndex: backendAction.selectIndex || 0,
                    padLength: backendAction.padLength || 10,
                    padChar: backendAction.padChar || '0'
                };

            case 'delete_field':
                return {
                    action: 'delete_field',
                    targetField: backendAction.field || ''
                };

            case 'route_to_step':
                return {
                    action: 'route_to_step',
                    stepId: backendAction.stepId || ''
                };

            default:
                console.warn('[IfThenElseActionTranslator] Unknown backend action:', backendAction.action);
                return backendAction;
        }
    }

    /**
     * Translate full condition configuration (UI to Backend)
     * Handles the conditions array structure
     *
     * UI Format:
     * {
     *   conditions: [
     *     {
     *       name: "Condition 1",
     *       condition: { field, operator, value, compareToField },
     *       onTrue: { action: "continue", ... },
     *       onFalse: { action: "stop", ... }
     *     }
     *   ]
     * }
     *
     * Backend Format (for single condition):
     * {
     *   condition: { field, operator, value, compareToField },
     *   then_actions: [{ action: "continue" }],
     *   else_actions: [{ action: "reject", ... }]
     * }
     */
    static translateConfig(uiConfig) {
        if (!uiConfig || !uiConfig.conditions || !Array.isArray(uiConfig.conditions)) {
            throw new Error('Invalid config: conditions array required');
        }

        // If single condition, return flattened format for backend
        if (uiConfig.conditions.length === 1) {
            const cond = uiConfig.conditions[0];

            return {
                condition: cond.condition,
                then_actions: [this.toBackend(cond.onTrue)],
                else_actions: [this.toBackend(cond.onFalse)]
            };
        }

        // Multiple conditions - evaluate in sequence
        // Backend will execute conditions one by one
        return {
            conditions: uiConfig.conditions.map(cond => ({
                condition: cond.condition,
                then_actions: [this.toBackend(cond.onTrue)],
                else_actions: [this.toBackend(cond.onFalse)]
            }))
        };
    }

    /**
     * Translate backend config to UI format
     */
    static parseConfig(backendConfig) {
        if (!backendConfig) {
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
                        onTrue: { action: 'continue', logLevel: 'none', message: '' },
                        onFalse: { action: 'continue', logLevel: 'none', message: '' }
                    }
                ]
            };
        }

        // Handle single condition format
        if (backendConfig.condition && backendConfig.then_actions && backendConfig.else_actions) {
            return {
                conditions: [
                    {
                        name: 'Condition 1',
                        condition: backendConfig.condition,
                        onTrue: this.fromBackend(backendConfig.then_actions[0] || {}),
                        onFalse: this.fromBackend(backendConfig.else_actions[0] || {})
                    }
                ]
            };
        }

        // Handle multiple conditions format
        if (backendConfig.conditions && Array.isArray(backendConfig.conditions)) {
            return {
                conditions: backendConfig.conditions.map((cond, index) => ({
                    name: cond.name || `Condition ${index + 1}`,
                    condition: cond.condition,
                    onTrue: this.fromBackend(cond.then_actions ? cond.then_actions[0] : {}),
                    onFalse: this.fromBackend(cond.else_actions ? cond.else_actions[0] : {})
                }))
            };
        }

        // Unknown format, return default
        console.warn('[IfThenElseActionTranslator] Unknown config format, using default');
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
                    onTrue: { action: 'continue', logLevel: 'none', message: '' },
                    onFalse: { action: 'continue', logLevel: 'none', message: '' }
                }
            ]
        };
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = IfThenElseActionTranslator;
}
