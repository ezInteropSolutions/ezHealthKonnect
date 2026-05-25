// field-mapping-validator.js - Clean version without duplicates
// Enhanced Field Mapping Validator
// Validates source-to-destination mapping compatibility

(function() {
    'use strict';
    
    // Prevent duplicate declarations
    if (typeof window.FieldMappingValidator !== 'undefined') {
        console.log('ℹ️ FieldMappingValidator already loaded, skipping redeclaration');
        return;
    }

    /**
     * Enhanced Field Mapping Validator
     * Validates source-to-destination mapping compatibility
     */
    class FieldMappingValidator {
        constructor() {
            this.typeCompatibilityMatrix = {
                'string': ['string', 'code', 'uri', 'id'],
                'number': ['integer', 'decimal', 'unsignedInt'],
                'date': ['date', 'dateTime', 'instant'],
                'boolean': ['boolean'],
                'code': ['string', 'code', 'uri']
            };
        }

        /**
         * Validate if HL7 field can be mapped to FHIR field
         * @param {Object} hl7Field - HL7 field metadata
         * @param {Object} fhirField - FHIR field metadata  
         * @returns {Object} Validation result with warnings/errors
         */
        validateMapping(hl7Field, fhirField) {
            const result = {
                valid: true,
                warnings: [],
                errors: [],
                suggestions: []
            };

            // Type compatibility check
            const typeCheck = this.checkTypeCompatibility(hl7Field.type, fhirField.type);
            if (!typeCheck.compatible) {
                result.errors.push(`Type mismatch: ${hl7Field.type} cannot map to ${fhirField.type}`);
                result.valid = false;
            }

            // Cardinality validation
            if (this.isRequiredField(fhirField) && this.isEmptyField(hl7Field)) {
                result.warnings.push(`Required FHIR field ${fhirField.path} mapped to empty HL7 field`);
            }

            // Value format validation
            const formatCheck = this.checkValueFormat(hl7Field, fhirField);
            if (!formatCheck.valid) {
                result.warnings.push(formatCheck.message);
            }

            // Suggest transform type
            result.suggestedTransform = this.suggestTransformType(hl7Field, fhirField);

            return result;
        }

        checkTypeCompatibility(hl7Type, fhirType) {
            const hl7BaseType = this.getBaseType(hl7Type);
            const compatibleTypes = this.typeCompatibilityMatrix[hl7BaseType] || [];
            const isCompatible = compatibleTypes.includes(fhirType);
            
            return {
                compatible: isCompatible,
                confidence: this.calculateConfidence(hl7Type, fhirType, isCompatible)
            };
        }

        calculateConfidence(hl7Type, fhirType, isCompatible) {
            // Return confidence score 0-100 without circular reference
            if (hl7Type === fhirType) return 100;
            return isCompatible ? 75 : 25;
        }

        suggestTransformType(hl7Field, fhirField) {
            // Direct mapping for compatible types
            if (this.checkTypeCompatibility(hl7Field.type, fhirField.type).compatible) {
                return 'direct';
            }

            // Date transformation
            if (hl7Field.type === 'TS' && fhirField.type === 'dateTime') {
                return 'dateConvert';
            }

            // Code lookup for coded values
            if (hl7Field.type === 'CWE' && fhirField.type === 'CodeableConcept') {
                return 'lookup';
            }

            // String concatenation for name fields
            if (fhirField.path && fhirField.path.includes('name') && hl7Field.value && hl7Field.value.includes(' ')) {
                return 'split';
            }

            return 'custom';
        }

        getBaseType(type) {
            const typeMap = {
                'ST': 'string', 'TX': 'string', 'FT': 'string',
                'NM': 'number', 'SI': 'number',
                'DT': 'date', 'TS': 'date', 'TM': 'date',
                'ID': 'code', 'IS': 'code', 'CWE': 'code'
            };
            return typeMap[type] || 'string';
        }

        isRequiredField(fhirField) {
            const requiredPaths = [
                'Patient.identifier',
                'Patient.name',
                'MessageHeader.source',
                'Bundle.type'
            ];
            return requiredPaths.some(path => fhirField.path && fhirField.path.startsWith(path));
        }

        isEmptyField(hl7Field) {
            return !hl7Field.value || hl7Field.value.trim() === '';
        }

        checkValueFormat(hl7Field, fhirField) {
            if (fhirField.type === 'date' && hl7Field.value) {
                const dateRegex = /^\d{4}-\d{2}-\d{2}$/;
                if (!dateRegex.test(hl7Field.value)) {
                    return {
                        valid: false,
                        message: 'Date format may need conversion to YYYY-MM-DD'
                    };
                }
            }
            return { valid: true };
        }

        getCompatibilityConfidence(hl7Type, fhirType) {
            // Return confidence score 0-100
            if (hl7Type === fhirType) return 100;
            
            // Check compatibility without circular reference
            const hl7BaseType = this.getBaseType(hl7Type);
            const compatibleTypes = this.typeCompatibilityMatrix[hl7BaseType] || [];
            const isCompatible = compatibleTypes.includes(fhirType);
            
            return isCompatible ? 75 : 25;
        }
    }

    // Make globally available
    window.FieldMappingValidator = FieldMappingValidator;
    console.log('✅ FieldMappingValidator loaded and available globally');

})();