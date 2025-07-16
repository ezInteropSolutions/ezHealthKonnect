// js/modules/hl7-validator.js
// Core validation engine for HL7 messages in ezHealthKonnect

class HL7Validator {
    constructor(schemas, healthcareRules) {
        this.schemas = schemas;
        this.healthcareRules = healthcareRules;
        this.validationResults = [];
        this.fieldIndex = new Map(); // For fast field lookups
    }

    /**
     * Main validation entry point
     * @param {Object} parsedMessage - Parsed HL7 message object
     * @param {string} messageType - Type of message (ADT_A01, ORM_O01, etc.)
     * @returns {Object} Comprehensive validation results
     */
    validateMessage(parsedMessage, messageType = 'ADT_A01') {
        this.validationResults = [];
        this.fieldIndex.clear();
        
        // Build field index for fast lookups
        this.buildFieldIndex(parsedMessage);
        
        const schema = this.schemas.getSchema(messageType);
        if (!schema) {
            return this.createErrorResult(`No schema found for message type: ${messageType}`);
        }

        const results = {
            messageType,
            isValid: true,
            summary: {
                totalFields: 0,
                validFields: 0,
                errors: 0,
                warnings: 0,
                missing: 0
            },
            segments: {},
            fieldValidations: [],
            missingRequired: [],
            typeViolations: [],
            bindingDeviations: [],
            healthcareViolations: []
        };

        // Validate each segment
        for (const [segmentName, segmentSchema] of Object.entries(schema.segments)) {
            const segmentData = parsedMessage[segmentName];
            const segmentResult = this.validateSegment(
                segmentData, 
                segmentSchema, 
                segmentName
            );
            
            results.segments[segmentName] = segmentResult;
            this.updateSummary(results.summary, segmentResult);
        }

        // Perform cross-segment healthcare validations
        const healthcareResults = this.healthcareRules.validateMessage(parsedMessage, messageType);
        results.healthcareViolations = healthcareResults;

        // Categorize validation issues
        this.categorizeValidationIssues(results);
        
        results.isValid = results.summary.errors === 0;
        
        return results;
    }

    /**
     * Validate individual segment
     */
    validateSegment(segmentData, segmentSchema, segmentName) {
        const result = {
            name: segmentName,
            isValid: true,
            isPresent: !!segmentData,
            isRequired: segmentSchema.required || false,
            fields: {},
            errors: [],
            warnings: []
        };

        // Check if required segment is missing
        if (segmentSchema.required && !segmentData) {
            result.isValid = false;
            result.errors.push({
                type: 'MISSING_REQUIRED_SEGMENT',
                message: `Required segment ${segmentName} is missing`,
                severity: 'error'
            });
            return result;
        }

        if (!segmentData) return result;

        // Handle both single segments and arrays
        const segments = Array.isArray(segmentData) ? segmentData : [segmentData];
        
        segments.forEach((segment, index) => {
            const segmentKey = segments.length > 1 ? `${segmentName}[${index}]` : segmentName;
            this.validateSegmentFields(segment, segmentSchema.fields, segmentKey, result);
        });

        return result;
    }

    /**
     * Validate fields within a segment
     */
    validateSegmentFields(segment, fieldSchemas, segmentKey, result) {
        for (const [fieldPosition, fieldSchema] of Object.entries(fieldSchemas)) {
            const fieldPath = `${segmentKey}.${fieldPosition}`;
            const fieldValue = this.getFieldValue(segment, fieldPosition);
            
            const fieldResult = this.validateField(
                fieldValue, 
                fieldSchema, 
                fieldPath
            );
            
            result.fields[fieldPosition] = fieldResult;
            
            if (!fieldResult.isValid) {
                result.isValid = false;
                if (fieldResult.severity === 'error') {
                    result.errors.push(...fieldResult.issues);
                } else {
                    result.warnings.push(...fieldResult.issues);
                }
            }
        }
    }

    /**
     * Validate individual field
     */
    validateField(value, fieldSchema, fieldPath) {
        const result = {
            path: fieldPath,
            value: value,
            isValid: true,
            isPresent: value !== null && value !== undefined && value !== '',
            schema: fieldSchema,
            issues: [],
            severity: 'info'
        };

        // Check required fields
        if (fieldSchema.required && !result.isPresent) {
            result.isValid = false;
            result.severity = 'error';
            result.issues.push({
                type: 'MISSING_REQUIRED',
                message: `Required field ${fieldPath} is missing`,
                severity: 'error',
                code: 'REQ_001'
            });
        }

        if (!result.isPresent) return result;

        // Data type validation
        const typeValidation = this.validateDataType(value, fieldSchema.dataType, fieldPath);
        if (!typeValidation.isValid) {
            result.isValid = false;
            result.severity = Math.max(result.severity, typeValidation.severity);
            result.issues.push(...typeValidation.issues);
        }

        // Length validation
        if (fieldSchema.maxLength && value.length > fieldSchema.maxLength) {
            result.isValid = false;
            result.severity = 'warning';
            result.issues.push({
                type: 'LENGTH_EXCEEDED',
                message: `Field ${fieldPath} exceeds maximum length of ${fieldSchema.maxLength}`,
                severity: 'warning',
                code: 'LEN_001'
            });
        }

        // Binding validation (value sets, code tables)
        if (fieldSchema.binding) {
            const bindingValidation = this.validateBinding(value, fieldSchema.binding, fieldPath);
            if (!bindingValidation.isValid) {
                result.isValid = false;
                result.severity = Math.max(result.severity, bindingValidation.severity);
                result.issues.push(...bindingValidation.issues);
            }
        }

        // Format validation (regex patterns)
        if (fieldSchema.pattern) {
            const patternValidation = this.validatePattern(value, fieldSchema.pattern, fieldPath);
            if (!patternValidation.isValid) {
                result.isValid = false;
                result.severity = Math.max(result.severity, patternValidation.severity);
                result.issues.push(...patternValidation.issues);
            }
        }

        return result;
    }

    /**
     * Validate data types (ST, NM, TS, etc.)
     */
    validateDataType(value, dataType, fieldPath) {
        const result = { isValid: true, issues: [], severity: 'info' };
        
        switch (dataType) {
            case 'NM': // Numeric
                if (!/^-?\d*\.?\d+$/.test(value)) {
                    result.isValid = false;
                    result.severity = 'error';
                    result.issues.push({
                        type: 'INVALID_DATA_TYPE',
                        message: `Field ${fieldPath} must be numeric, got: "${value}"`,
                        severity: 'error',
                        code: 'TYPE_001'
                    });
                }
                break;
                
            case 'TS': // Timestamp
                if (!/^\d{4}(\d{2}(\d{2}(\d{2}(\d{2}(\d{2}(\.\d{1,4})?)?)?)?)?)?([+-]\d{4})?$/.test(value)) {
                    result.isValid = false;
                    result.severity = 'error';
                    result.issues.push({
                        type: 'INVALID_TIMESTAMP',
                        message: `Field ${fieldPath} has invalid timestamp format: "${value}"`,
                        severity: 'error',
                        code: 'TYPE_002'
                    });
                }
                break;
                
            case 'DT': // Date
                if (!/^\d{4}(\d{2}(\d{2})?)?$/.test(value)) {
                    result.isValid = false;
                    result.severity = 'error';
                    result.issues.push({
                        type: 'INVALID_DATE',
                        message: `Field ${fieldPath} has invalid date format: "${value}"`,
                        severity: 'error',
                        code: 'TYPE_003'
                    });
                }
                break;
                
            case 'IS': // Coded Value
            case 'ID': // Coded Value
                // Should be validated against binding tables
                break;
                
            case 'ST': // String
            case 'TX': // Text
            case 'FT': // Formatted Text
                // Generally any string is valid
                break;
                
            default:
                // Unknown data type - warning
                result.severity = 'warning';
                result.issues.push({
                    type: 'UNKNOWN_DATA_TYPE',
                    message: `Unknown data type "${dataType}" for field ${fieldPath}`,
                    severity: 'warning',
                    code: 'TYPE_999'
                });
        }
        
        return result;
    }

    /**
     * Validate against binding tables/value sets
     */
    validateBinding(value, binding, fieldPath) {
        const result = { isValid: true, issues: [], severity: 'info' };
        
        if (!binding.valueSet) return result;
        
        const valueSet = this.schemas.getValueSet(binding.valueSet);
        if (!valueSet) {
            result.severity = 'warning';
            result.issues.push({
                type: 'MISSING_VALUE_SET',
                message: `Value set "${binding.valueSet}" not found for field ${fieldPath}`,
                severity: 'warning',
                code: 'BIND_001'
            });
            return result;
        }
        
        const isValidValue = valueSet.values.some(v => 
            v.code === value || v.displayName === value
        );
        
        if (!isValidValue) {
            const severity = binding.strength === 'required' ? 'error' : 'warning';
            result.isValid = binding.strength === 'required' ? false : true;
            result.severity = severity;
            result.issues.push({
                type: 'INVALID_BINDING_VALUE',
                message: `Value "${value}" not found in ${binding.valueSet} value set for field ${fieldPath}`,
                severity: severity,
                code: binding.strength === 'required' ? 'BIND_002' : 'BIND_003',
                suggestions: this.getSimilarValues(value, valueSet.values)
            });
        }
        
        return result;
    }

    /**
     * Validate against regex patterns
     */
    validatePattern(value, pattern, fieldPath) {
        const result = { isValid: true, issues: [], severity: 'info' };
        
        try {
            const regex = new RegExp(pattern.regex);
            if (!regex.test(value)) {
                result.isValid = false;
                result.severity = 'error';
                result.issues.push({
                    type: 'PATTERN_MISMATCH',
                    message: `Field ${fieldPath} does not match required pattern: ${pattern.description || pattern.regex}`,
                    severity: 'error',
                    code: 'PAT_001'
                });
            }
        } catch (e) {
            result.severity = 'warning';
            result.issues.push({
                type: 'INVALID_PATTERN',
                message: `Invalid regex pattern for field ${fieldPath}: ${e.message}`,
                severity: 'warning',
                code: 'PAT_002'
            });
        }
        
        return result;
    }

    /**
     * Build index for fast field lookups
     */
    buildFieldIndex(parsedMessage) {
        const indexField = (obj, path = '') => {
            for (const [key, value] of Object.entries(obj)) {
                const currentPath = path ? `${path}.${key}` : key;
                this.fieldIndex.set(currentPath, value);
                
                if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
                    indexField(value, currentPath);
                }
            }
        };
        
        indexField(parsedMessage);
    }

    /**
     * Get field value by position
     */
    getFieldValue(segment, fieldPosition) {
        // Handle both numeric positions and named fields
        if (segment[fieldPosition] !== undefined) {
            return segment[fieldPosition];
        }
        
        // Try as array index
        const numPos = parseInt(fieldPosition);
        if (!isNaN(numPos) && segment[numPos] !== undefined) {
            return segment[numPos];
        }
        
        return null;
    }

    /**
     * Categorize validation issues
     */
    categorizeValidationIssues(results) {
        for (const segmentResult of Object.values(results.segments)) {
            for (const fieldResult of Object.values(segmentResult.fields)) {
                for (const issue of fieldResult.issues) {
                    switch (issue.type) {
                        case 'MISSING_REQUIRED':
                        case 'MISSING_REQUIRED_SEGMENT':
                            results.missingRequired.push({
                                ...issue,
                                path: fieldResult.path,
                                value: fieldResult.value
                            });
                            break;
                            
                        case 'INVALID_DATA_TYPE':
                        case 'INVALID_TIMESTAMP':
                        case 'INVALID_DATE':
                            results.typeViolations.push({
                                ...issue,
                                path: fieldResult.path,
                                value: fieldResult.value,
                                expectedType: fieldResult.schema.dataType
                            });
                            break;
                            
                        case 'INVALID_BINDING_VALUE':
                        case 'MISSING_VALUE_SET':
                            results.bindingDeviations.push({
                                ...issue,
                                path: fieldResult.path,
                                value: fieldResult.value,
                                binding: fieldResult.schema.binding
                            });
                            break;
                    }
                }
            }
        }
    }

    /**
     * Update validation summary
     */
    updateSummary(summary, segmentResult) {
        const fieldCount = Object.keys(segmentResult.fields).length;
        summary.totalFields += fieldCount;
        
        for (const fieldResult of Object.values(segmentResult.fields)) {
            if (fieldResult.isValid) {
                summary.validFields++;
            } else {
                if (fieldResult.severity === 'error') {
                    summary.errors++;
                } else {
                    summary.warnings++;
                }
            }
            
            if (fieldResult.schema.required && !fieldResult.isPresent) {
                summary.missing++;
            }
        }
    }

    /**
     * Get similar values for suggestions
     */
    getSimilarValues(value, validValues, maxSuggestions = 3) {
        const suggestions = validValues
            .map(v => ({
                value: v,
                distance: this.levenshteinDistance(value.toLowerCase(), v.code.toLowerCase())
            }))
            .sort((a, b) => a.distance - b.distance)
            .slice(0, maxSuggestions)
            .map(s => s.value);
            
        return suggestions;
    }

    /**
     * Calculate Levenshtein distance for suggestions
     */
    levenshteinDistance(str1, str2) {
        const matrix = [];
        
        for (let i = 0; i <= str2.length; i++) {
            matrix[i] = [i];
        }
        
        for (let j = 0; j <= str1.length; j++) {
            matrix[0][j] = j;
        }
        
        for (let i = 1; i <= str2.length; i++) {
            for (let j = 1; j <= str1.length; j++) {
                if (str2.charAt(i - 1) === str1.charAt(j - 1)) {
                    matrix[i][j] = matrix[i - 1][j - 1];
                } else {
                    matrix[i][j] = Math.min(
                        matrix[i - 1][j - 1] + 1,
                        matrix[i][j - 1] + 1,
                        matrix[i - 1][j] + 1
                    );
                }
            }
        }
        
        return matrix[str2.length][str1.length];
    }

    /**
     * Create error result
     */
    createErrorResult(message) {
        return {
            isValid: false,
            error: message,
            summary: {
                totalFields: 0,
                validFields: 0,
                errors: 1,
                warnings: 0,
                missing: 0
            },
            segments: {},
            fieldValidations: [],
            missingRequired: [],
            typeViolations: [],
            bindingDeviations: [],
            healthcareViolations: []
        };
    }

    /**
     * Get field drill-down information
     */
    getFieldDrillDown(fieldPath) {
        const value = this.fieldIndex.get(fieldPath);
        if (!value) return null;
        
        // Analyze field components for composite fields
        if (typeof value === 'string' && value.includes('^')) {
            const components = value.split('^');
            return {
                path: fieldPath,
                value: value,
                isComposite: true,
                components: components.map((comp, index) => ({
                    index: index + 1,
                    value: comp,
                    isEmpty: comp === ''
                }))
            };
        }
        
        return {
            path: fieldPath,
            value: value,
            isComposite: false,
            type: typeof value
        };
    }
}

// Export for use in other modules
export { HL7Validator };