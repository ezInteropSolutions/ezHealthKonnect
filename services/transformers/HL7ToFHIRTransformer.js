// services/transformers/HL7ToFHIRTransformer.js
// HL7 to FHIR transformation engine using wizard-generated mappings

const { v4: uuidv4 } = require('uuid');

class HL7ToFHIRTransformer {
    constructor() {
        this.logger = this.getLogger();
    }

    /**
     * Transform HL7 message to FHIR using mapping configuration
     * @param {string} hl7Message - Raw HL7 message
     * @param {Object} mappingConfig - Mapping configuration from wizard
     * @param {Object} context - Processing context with messageId, etc.
     * @returns {Object} - FHIR Bundle with transformed resources
     */
    async transform(hl7Message, mappingConfig, context = {}) {
        const transformStartTime = Date.now();

        try {
            this.logger.debug(`🔄 Starting HL7-FHIR transformation for message ${context.messageId}`);

            // Parse HL7 message into segments
            const hl7Data = this.parseHL7Message(hl7Message);

            // Create FHIR Bundle structure
            const fhirBundle = this.createFHIRBundle(context.messageId);

            // Apply transformations based on mapping config
            const transformedResources = await this.applyMappingTransformations(
                hl7Data,
                mappingConfig,
                context
            );

            // Add resources to bundle
            fhirBundle.entry = transformedResources.map(resource => ({
                fullUrl: `urn:uuid:${resource.id}`,
                resource: resource
            }));

            // Add transformation metadata
            fhirBundle.meta = {
                lastUpdated: new Date().toISOString(),
                source: 'ezHealthKonnect-wizard',
                tag: [
                    {
                        system: 'http://terminology.hl7.org/CodeSystem/v3-ObservationValue',
                        code: 'transformed',
                        display: 'Transformed Message'
                    }
                ]
            };

            const transformDuration = Date.now() - transformStartTime;

            this.logger.info(`✅ HL7-FHIR transformation completed in ${transformDuration}ms`, {
                messageId: context.messageId,
                resourceCount: transformedResources.length,
                duration: transformDuration
            });

            return fhirBundle;

        } catch (error) {
            const transformDuration = Date.now() - transformStartTime;
            this.logger.error(`❌ HL7-FHIR transformation failed after ${transformDuration}ms:`, error);
            throw new Error(`Transformation failed: ${error.message}`);
        }
    }

    /**
     * Parse HL7 message into structured data
     */
    parseHL7Message(hl7Message) {
        const segments = hl7Message.split('\r').filter(seg => seg.trim());
        const parsedData = {
            segments: {},
            raw: hl7Message
        };

        segments.forEach(segment => {
            if (segment.length > 3) {
                const segmentType = segment.substring(0, 3);
                const fields = segment.split('|');

                if (!parsedData.segments[segmentType]) {
                    parsedData.segments[segmentType] = [];
                }

                parsedData.segments[segmentType].push({
                    raw: segment,
                    fields: fields
                });
            }
        });

        return parsedData;
    }

    /**
     * Create base FHIR Bundle structure
     */
    createFHIRBundle(messageId) {
        return {
            resourceType: 'Bundle',
            id: messageId || uuidv4(),
            type: 'message',
            timestamp: new Date().toISOString(),
            entry: []
        };
    }

    /**
     * Apply mapping transformations to create FHIR resources
     */
    async applyMappingTransformations(hl7Data, mappingConfig, context) {
        const transformedResources = [];

        try {
            // Check if this is a standard template or custom mapping
            const mappings = mappingConfig.mappingConfig || mappingConfig;

            if (mappings.atomicMappings && mappings.atomicMappings.length > 0) {
                // Custom atomic mappings approach
                const resources = await this.applyAtomicMappings(
                    hl7Data,
                    mappings.atomicMappings,
                    context
                );
                transformedResources.push(...resources);

            } else if (mappings.mappings) {
                // Standard template mappings approach
                const resources = await this.applyStandardMappings(
                    hl7Data,
                    mappings.mappings,
                    context
                );
                transformedResources.push(...resources);

            } else {
                throw new Error('No valid mapping configuration found');
            }

            return transformedResources;

        } catch (error) {
            this.logger.error('❌ Error applying mapping transformations:', error);
            throw error;
        }
    }

    /**
     * Apply atomic mappings (custom wizard mappings)
     */
    async applyAtomicMappings(hl7Data, atomicMappings, context) {
        const resourceMap = new Map(); // resourceType -> resource

        for (const mapping of atomicMappings) {
            try {
                const resourceType = mapping.resourceType;
                const hl7Value = this.extractHL7Value(hl7Data, mapping.hl7Field);

                if (hl7Value !== null && hl7Value !== undefined) {
                    // Get or create resource
                    if (!resourceMap.has(resourceType)) {
                        resourceMap.set(resourceType, this.createBaseResource(resourceType));
                    }

                    const resource = resourceMap.get(resourceType);

                    // Apply the mapping
                    this.setFHIRValue(resource, mapping.fhirPath, hl7Value, mapping);
                }

            } catch (error) {
                this.logger.warn(`⚠️ Skipping atomic mapping due to error:`, {
                    mapping: mapping.hl7Field,
                    error: error.message
                });
            }
        }

        return Array.from(resourceMap.values());
    }

    /**
     * Apply standard template mappings
     */
    async applyStandardMappings(hl7Data, mappings, context) {
        const resources = [];

        for (const [resourceType, config] of Object.entries(mappings)) {
            try {
                const resource = this.createBaseResource(resourceType);

                for (const mapping of config.mappings) {
                    const hl7Value = this.extractHL7Value(hl7Data, mapping.hl7Path);

                    if (hl7Value !== null && hl7Value !== undefined) {
                        this.setFHIRValue(resource, mapping.fhirPath, hl7Value, mapping);
                    }
                }

                resources.push(resource);

            } catch (error) {
                this.logger.warn(`⚠️ Error processing ${resourceType} mapping:`, error);
            }
        }

        return resources;
    }

    /**
     * Extract value from HL7 data using field path
     */
    extractHL7Value(hl7Data, fieldPath) {
        try {
            // Parse field path like "PID.5.1" or "MSH.9"
            const parts = fieldPath.split('.');
            const segmentType = parts[0];
            const fieldIndex = parseInt(parts[1]) - 1; // HL7 fields are 1-indexed
            const componentIndex = parts[2] ? parseInt(parts[2]) - 1 : null;

            const segments = hl7Data.segments[segmentType];
            if (!segments || segments.length === 0) {
                return null;
            }

            // Use first segment of this type (can be enhanced for multiple segments)
            const segment = segments[0];
            if (!segment.fields || segment.fields.length <= fieldIndex) {
                return null;
            }

            let value = segment.fields[fieldIndex];

            // Handle component extraction if specified
            if (componentIndex !== null && value) {
                const components = value.split('^');
                value = components[componentIndex] || null;
            }

            return value ? value.trim() : null;

        } catch (error) {
            this.logger.warn(`⚠️ Error extracting HL7 value from ${fieldPath}:`, error);
            return null;
        }
    }

    /**
     * Set FHIR value using path notation
     */
    setFHIRValue(resource, fhirPath, value, mapping = {}) {
        try {
            // Transform value if needed
            const transformedValue = this.transformValue(value, mapping.transform);

            // Set the value using path notation like "name[0].family"
            this.setNestedValue(resource, fhirPath, transformedValue);

        } catch (error) {
            this.logger.warn(`⚠️ Error setting FHIR value at ${fhirPath}:`, error);
        }
    }

    /**
     * Transform value based on transformation type
     */
    transformValue(value, transformType) {
        if (!transformType || !value) {
            return value;
        }

        switch (transformType.toLowerCase()) {
            case 'date':
                return this.transformToFHIRDate(value);
            case 'datetime':
                return this.transformToFHIRDateTime(value);
            case 'gender':
                return this.transformGender(value);
            case 'boolean':
                return this.transformBoolean(value);
            default:
                return value;
        }
    }

    /**
     * Transform HL7 date to FHIR date format
     */
    transformToFHIRDate(hl7Date) {
        if (!hl7Date || hl7Date.length < 8) return hl7Date;

        // HL7 format: YYYYMMDD -> FHIR format: YYYY-MM-DD
        const year = hl7Date.substring(0, 4);
        const month = hl7Date.substring(4, 6);
        const day = hl7Date.substring(6, 8);

        return `${year}-${month}-${day}`;
    }

    /**
     * Transform HL7 datetime to FHIR datetime format
     */
    transformToFHIRDateTime(hl7DateTime) {
        if (!hl7DateTime || hl7DateTime.length < 8) return hl7DateTime;

        // Basic transformation for YYYYMMDDHHMMSS format
        try {
            const year = hl7DateTime.substring(0, 4);
            const month = hl7DateTime.substring(4, 6);
            const day = hl7DateTime.substring(6, 8);
            const hour = hl7DateTime.substring(8, 10) || '00';
            const minute = hl7DateTime.substring(10, 12) || '00';
            const second = hl7DateTime.substring(12, 14) || '00';

            return `${year}-${month}-${day}T${hour}:${minute}:${second}Z`;
        } catch (error) {
            return hl7DateTime;
        }
    }

    /**
     * Transform HL7 gender to FHIR gender
     */
    transformGender(hl7Gender) {
        const genderMap = {
            'M': 'male',
            'F': 'female',
            'O': 'other',
            'U': 'unknown'
        };

        return genderMap[hl7Gender?.toUpperCase()] || 'unknown';
    }

    /**
     * Transform to boolean
     */
    transformBoolean(value) {
        if (typeof value === 'boolean') return value;
        if (typeof value === 'string') {
            return ['Y', 'YES', 'TRUE', '1'].includes(value.toUpperCase());
        }
        return false;
    }

    /**
     * Set nested value in object using dot notation with array support
     */
    setNestedValue(obj, path, value) {
        const keys = path.match(/([^.\[]+)|\[(\d+)\]/g);

        let current = obj;
        for (let i = 0; i < keys.length - 1; i++) {
            const key = keys[i];

            if (key.startsWith('[') && key.endsWith(']')) {
                // Array index
                const index = parseInt(key.slice(1, -1));
                if (!Array.isArray(current)) {
                    current = [];
                }
                if (!current[index]) {
                    current[index] = {};
                }
                current = current[index];
            } else {
                // Object key
                if (!current[key]) {
                    // Determine if next key is array index
                    const nextKey = keys[i + 1];
                    current[key] = (nextKey && nextKey.startsWith('[')) ? [] : {};
                }
                current = current[key];
            }
        }

        // Set the final value
        const finalKey = keys[keys.length - 1];
        if (finalKey.startsWith('[') && finalKey.endsWith(']')) {
            const index = parseInt(finalKey.slice(1, -1));
            if (!Array.isArray(current)) {
                current = [];
            }
            current[index] = value;
        } else {
            current[finalKey] = value;
        }
    }

    /**
     * Create base FHIR resource
     */
    createBaseResource(resourceType) {
        const baseResource = {
            resourceType: resourceType,
            id: uuidv4()
        };

        // Add resource-specific defaults
        switch (resourceType) {
            case 'Patient':
                baseResource.identifier = [];
                baseResource.name = [];
                break;
            case 'Encounter':
                baseResource.status = 'unknown';
                baseResource.class = { code: 'IMP' };
                break;
            case 'Observation':
                baseResource.status = 'preliminary';
                break;
            case 'MessageHeader':
                baseResource.eventCoding = { code: 'unknown' };
                baseResource.source = { name: 'ezHealthKonnect' };
                break;
        }

        return baseResource;
    }

    /**
     * Get logger instance
     */
    getLogger() {
        try {
            return require('../../utils/logger');
        } catch (error) {
            return {
                info: console.log.bind(console, '[INFO]'),
                warn: console.warn.bind(console, '[WARN]'),
                error: console.error.bind(console, '[ERROR]'),
                debug: console.log.bind(console, '[DEBUG]')
            };
        }
    }
}

module.exports = HL7ToFHIRTransformer;