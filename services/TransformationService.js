// services/TransformationService.js
// Enterprise Transformation Service - HL7 ↔ FHIR conversion with mapping engine
// Handles schema validation, field mapping, and transformation rules

const fs = require('fs');
const path = require('path');

class TransformationService {
    constructor() {
        this.transformationCache = new Map();
        this.schemaCache = new Map();
        this.mappingRules = new Map();

        // Performance metrics
        this.metrics = {
            transformationsExecuted: 0,
            transformationsFailed: 0,
            avgTransformationTime: 0,
            cacheHits: 0,
            startTime: new Date()
        };

        // Load built-in transformation templates
        this.loadBuiltInMappings();
    }

    /**
     * Load built-in HL7 to FHIR mapping templates
     */
    loadBuiltInMappings() {
        // ADT^A01 (Patient Admission) to FHIR Patient
        this.mappingRules.set('ADT^A01_to_FHIR_Patient', {
            sourceType: 'HL7',
            sourceMessageType: 'ADT^A01',
            targetType: 'FHIR',
            targetResourceType: 'Patient',
            mappingVersion: '1.0',
            fieldMappings: [
                // Patient identification
                { source: 'PID.5.1', target: 'name[0].family', transform: 'toString' },
                { source: 'PID.5.2', target: 'name[0].given[0]', transform: 'toString' },
                { source: 'PID.5.3', target: 'name[0].given[1]', transform: 'toString', optional: true },
                { source: 'PID.3.1', target: 'identifier[0].value', transform: 'toString' },
                { source: 'PID.3.4', target: 'identifier[0].system', transform: 'identifierSystem' },

                // Demographics
                { source: 'PID.7', target: 'birthDate', transform: 'hl7DateToFhir' },
                { source: 'PID.8', target: 'gender', transform: 'hl7GenderToFhir' },

                // Address
                { source: 'PID.11.1', target: 'address[0].line[0]', transform: 'toString' },
                { source: 'PID.11.3', target: 'address[0].city', transform: 'toString' },
                { source: 'PID.11.4', target: 'address[0].state', transform: 'toString' },
                { source: 'PID.11.5', target: 'address[0].postalCode', transform: 'toString' },
                { source: 'PID.11.6', target: 'address[0].country', transform: 'toString' },

                // Contact
                { source: 'PID.13.1', target: 'telecom[0].value', transform: 'toString' },
                { source: 'PID.13.2', target: 'telecom[0].system', transform: 'telecomSystem', default: 'phone' },

                // Status
                { source: 'EVN.1', target: 'active', transform: 'eventToActive', default: true }
            ]
        });

        // ORU^R01 (Observation Report) to FHIR Observation
        this.mappingRules.set('ORU^R01_to_FHIR_Observation', {
            sourceType: 'HL7',
            sourceMessageType: 'ORU^R01',
            targetType: 'FHIR',
            targetResourceType: 'Observation',
            mappingVersion: '1.0',
            fieldMappings: [
                // Observation identification
                { source: 'OBX.3.1', target: 'code.coding[0].code', transform: 'toString' },
                { source: 'OBX.3.2', target: 'code.coding[0].display', transform: 'toString' },
                { source: 'OBX.5', target: 'valueQuantity.value', transform: 'toNumber' },
                { source: 'OBX.6.1', target: 'valueQuantity.unit', transform: 'toString' },
                { source: 'OBX.14', target: 'effectiveDateTime', transform: 'hl7DateTimeToFhir' },

                // Status
                { source: 'OBX.11', target: 'status', transform: 'hl7StatusToFhir', default: 'final' },

                // Patient reference
                { source: 'PID.3.1', target: 'subject.reference', transform: 'patientReference' }
            ]
        });

        console.log(`📋 Loaded ${this.mappingRules.size} built-in transformation mappings`);
    }

    /**
     * Transform HL7 message to FHIR resource
     */
    async transformHl7ToFhir(hl7Message, transformationConfig) {
        const startTime = Date.now();

        try {
            console.log(`🔄 Starting HL7 to FHIR transformation...`);

            // Parse HL7 message
            const parsedHl7 = this.parseHl7Message(hl7Message);
            const messageType = this.extractMessageType(parsedHl7);

            console.log(`📋 Detected HL7 message type: ${messageType}`);

            // Get mapping rules
            const mappingKey = `${messageType}_to_FHIR_${transformationConfig.targetResourceType || 'Patient'}`;
            const mappingRules = this.getMappingRules(mappingKey, transformationConfig);

            if (!mappingRules) {
                throw new Error(`No mapping rules found for ${mappingKey}`);
            }

            // Execute transformation
            const fhirResource = await this.executeMappingRules(parsedHl7, mappingRules);

            // Add metadata
            fhirResource.meta = {
                source: 'HL7-Transform',
                versionId: '1',
                lastUpdated: new Date().toISOString(),
                profile: [`http://hl7.org/fhir/StructureDefinition/${mappingRules.targetResourceType}`]
            };

            // Performance tracking
            const transformationTime = Date.now() - startTime;
            this.updateMetrics(true, transformationTime);

            console.log(`✅ HL7 to FHIR transformation completed in ${transformationTime}ms`);
            console.log(`📦 Generated FHIR ${mappingRules.targetResourceType} resource`);

            return {
                success: true,
                fhirResource,
                sourceMessageType: messageType,
                targetResourceType: mappingRules.targetResourceType,
                transformationTime,
                mappingVersion: mappingRules.mappingVersion
            };

        } catch (error) {
            this.updateMetrics(false, Date.now() - startTime);
            console.error(`❌ HL7 to FHIR transformation failed:`, error.message);

            return {
                success: false,
                error: error.message,
                sourceMessage: hl7Message.substring(0, 200) + '...',
                transformationTime: Date.now() - startTime
            };
        }
    }

    /**
     * Parse HL7 message into structured segments
     */
    parseHl7Message(hl7Message) {
        const segments = {};
        const lines = hl7Message.split(/\r?\n/).filter(line => line.trim());

        for (const line of lines) {
            if (line.length < 3) continue;

            const segmentType = line.substring(0, 3);
            const fields = line.split('|');

            if (!segments[segmentType]) {
                segments[segmentType] = [];
            }

            // Create field object with dot notation support
            const segmentData = {};
            fields.forEach((field, index) => {
                if (index === 0) return; // Skip segment identifier

                // Handle component separators (^)
                const components = field.split('^');
                if (components.length > 1) {
                    components.forEach((component, compIndex) => {
                        segmentData[`${index}.${compIndex + 1}`] = component.trim();
                    });
                } else {
                    segmentData[index.toString()] = field.trim();
                }
            });

            segments[segmentType].push(segmentData);
        }

        return segments;
    }

    /**
     * Extract message type from parsed HL7
     */
    extractMessageType(parsedHl7) {
        if (parsedHl7.MSH && parsedHl7.MSH[0]) {
            const messageTypeField = parsedHl7.MSH[0]['9'] || parsedHl7.MSH[0]['9.1'];
            if (messageTypeField) {
                const parts = messageTypeField.split('^');
                return parts[0] + '^' + parts[1]; // e.g., "ADT^A01"
            }
        }
        return 'UNKNOWN';
    }

    /**
     * Get mapping rules for transformation
     */
    getMappingRules(mappingKey, customConfig) {
        // Check for custom mapping rules first
        if (customConfig && customConfig.customMappings) {
            return customConfig.customMappings;
        }

        // Use built-in mapping rules
        return this.mappingRules.get(mappingKey);
    }

    /**
     * Execute field mapping rules to create FHIR resource
     */
    async executeMappingRules(parsedHl7, mappingRules) {
        const fhirResource = {
            resourceType: mappingRules.targetResourceType,
            id: `hl7-transform-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`
        };

        for (const mapping of mappingRules.fieldMappings) {
            try {
                const sourceValue = this.extractSourceValue(parsedHl7, mapping.source);

                if (sourceValue !== null && sourceValue !== undefined && sourceValue !== '') {
                    const transformedValue = await this.applyTransformation(sourceValue, mapping.transform);
                    this.setNestedProperty(fhirResource, mapping.target, transformedValue);
                } else if (mapping.default !== undefined) {
                    this.setNestedProperty(fhirResource, mapping.target, mapping.default);
                } else if (!mapping.optional) {
                    console.warn(`⚠️ Required field ${mapping.source} not found in HL7 message`);
                }
            } catch (error) {
                console.error(`❌ Error mapping field ${mapping.source} -> ${mapping.target}:`, error.message);

                if (!mapping.optional) {
                    throw new Error(`Failed to map required field ${mapping.source}: ${error.message}`);
                }
            }
        }

        return fhirResource;
    }

    /**
     * Extract value from parsed HL7 using dot notation
     */
    extractSourceValue(parsedHl7, sourcePath) {
        const parts = sourcePath.split('.');
        const segmentType = parts[0];
        const fieldPath = parts.slice(1).join('.');

        if (!parsedHl7[segmentType] || parsedHl7[segmentType].length === 0) {
            return null;
        }

        // For now, use first segment instance (can be enhanced for repeating segments)
        const segment = parsedHl7[segmentType][0];
        return segment[fieldPath] || null;
    }

    /**
     * Apply transformation function to source value
     */
    async applyTransformation(value, transformFunction) {
        if (!transformFunction) return value;

        switch (transformFunction) {
            case 'toString':
                return String(value);

            case 'toNumber':
                const num = parseFloat(value);
                return isNaN(num) ? null : num;

            case 'hl7DateToFhir':
                // Convert YYYYMMDD to YYYY-MM-DD
                if (value && value.length >= 8) {
                    return `${value.substr(0, 4)}-${value.substr(4, 2)}-${value.substr(6, 2)}`;
                }
                return null;

            case 'hl7DateTimeToFhir':
                // Convert YYYYMMDDHHMMSS to ISO format
                if (value && value.length >= 14) {
                    const year = value.substr(0, 4);
                    const month = value.substr(4, 2);
                    const day = value.substr(6, 2);
                    const hour = value.substr(8, 2);
                    const minute = value.substr(10, 2);
                    const second = value.substr(12, 2);
                    return `${year}-${month}-${day}T${hour}:${minute}:${second}Z`;
                }
                return null;

            case 'hl7GenderToFhir':
                const genderMap = { 'M': 'male', 'F': 'female', 'U': 'unknown', 'O': 'other' };
                return genderMap[value] || 'unknown';

            case 'hl7StatusToFhir':
                const statusMap = { 'F': 'final', 'P': 'preliminary', 'C': 'corrected' };
                return statusMap[value] || 'final';

            case 'identifierSystem':
                return value === 'MRN' ? 'http://hospital.org/mrn' : `http://hospital.org/${value}`;

            case 'telecomSystem':
                return value.includes('@') ? 'email' : 'phone';

            case 'eventToActive':
                return ['A01', 'A04', 'A05'].includes(value) ? true : false;

            case 'patientReference':
                return `Patient/${value}`;

            default:
                console.warn(`⚠️ Unknown transformation function: ${transformFunction}`);
                return value;
        }
    }

    /**
     * Set nested property in object using dot notation
     */
    setNestedProperty(obj, path, value) {
        const parts = path.split('.');
        let current = obj;

        for (let i = 0; i < parts.length - 1; i++) {
            const part = parts[i];

            // Handle array notation like 'name[0]'
            const arrayMatch = part.match(/(.+)\[(\d+)\]/);
            if (arrayMatch) {
                const arrayName = arrayMatch[1];
                const index = parseInt(arrayMatch[2]);

                if (!current[arrayName]) current[arrayName] = [];
                if (!current[arrayName][index]) current[arrayName][index] = {};
                current = current[arrayName][index];
            } else {
                if (!current[part]) current[part] = {};
                current = current[part];
            }
        }

        // Set the final value
        const finalPart = parts[parts.length - 1];
        const arrayMatch = finalPart.match(/(.+)\[(\d+)\]/);

        if (arrayMatch) {
            const arrayName = arrayMatch[1];
            const index = parseInt(arrayMatch[2]);

            if (!current[arrayName]) current[arrayName] = [];
            current[arrayName][index] = value;
        } else {
            current[finalPart] = value;
        }
    }

    /**
     * Transform FHIR resource back to HL7 (reverse transformation)
     */
    async transformFhirToHl7(fhirResource, transformationConfig) {
        // Implementation for reverse transformation
        // This would be needed for bidirectional interfaces
        throw new Error('FHIR to HL7 transformation not yet implemented');
    }

    /**
     * Validate FHIR resource against schema
     */
    async validateFhirResource(fhirResource) {
        // Basic validation - can be enhanced with full FHIR schema validation
        const requiredFields = ['resourceType', 'id'];

        for (const field of requiredFields) {
            if (!fhirResource[field]) {
                throw new Error(`Missing required field: ${field}`);
            }
        }

        return {
            valid: true,
            errors: [],
            warnings: []
        };
    }

    /**
     * Get available transformation mappings
     */
    getAvailableMappings() {
        return Array.from(this.mappingRules.keys()).map(key => {
            const mapping = this.mappingRules.get(key);
            return {
                key,
                sourceType: mapping.sourceType,
                sourceMessageType: mapping.sourceMessageType,
                targetType: mapping.targetType,
                targetResourceType: mapping.targetResourceType,
                version: mapping.mappingVersion,
                fieldCount: mapping.fieldMappings.length
            };
        });
    }

    /**
     * Update performance metrics
     */
    updateMetrics(success, transformationTime) {
        if (success) {
            this.metrics.transformationsExecuted++;

            // Update average transformation time
            const totalTransformations = this.metrics.transformationsExecuted;
            this.metrics.avgTransformationTime =
                ((this.metrics.avgTransformationTime * (totalTransformations - 1)) + transformationTime) / totalTransformations;
        } else {
            this.metrics.transformationsFailed++;
        }
    }

    /**
     * Get transformation service metrics
     */
    getMetrics() {
        const uptime = Math.floor((new Date() - this.metrics.startTime) / 1000);
        const totalTransformations = this.metrics.transformationsExecuted + this.metrics.transformationsFailed;

        return {
            uptime: `${Math.floor(uptime / 60)}m ${uptime % 60}s`,
            transformations: {
                executed: this.metrics.transformationsExecuted,
                failed: this.metrics.transformationsFailed,
                total: totalTransformations,
                successRate: totalTransformations > 0 ?
                    ((this.metrics.transformationsExecuted / totalTransformations) * 100).toFixed(2) + '%' : '0%'
            },
            performance: {
                avgTransformationTime: Math.round(this.metrics.avgTransformationTime),
                cacheHits: this.metrics.cacheHits,
                availableMappings: this.mappingRules.size
            }
        };
    }

    /**
     * Add custom mapping rule
     */
    addCustomMapping(key, mappingRule) {
        this.mappingRules.set(key, mappingRule);
        console.log(`📝 Added custom mapping rule: ${key}`);
    }

    /**
     * Clear transformation cache
     */
    clearCache() {
        this.transformationCache.clear();
        this.schemaCache.clear();
        console.log('🧹 Transformation cache cleared');
    }
}

module.exports = TransformationService;