// services/transformers/TransformerFactory.js
// Factory for creating transformers that bridge with Go backend

const axios = require('axios');

class TransformerFactory {
    /**
     * Create transformer for specific source->target conversion
     */
    static async createTransformer(sourceType, targetType, messageMappings) {
        console.log(`🔄 Creating transformer: ${sourceType} -> ${targetType}`);

        // For HL7 to FHIR transformation, use Go backend
        if (sourceType.toLowerCase().includes('hl7') && targetType.toLowerCase().includes('fhir')) {
            return new HL7ToFHIRTransformer(messageMappings);
        }

        // For FHIR to HL7 transformation
        if (sourceType.toLowerCase().includes('fhir') && targetType.toLowerCase().includes('hl7')) {
            return new FHIRToHL7Transformer(messageMappings);
        }

        // Generic transformer for other types
        return new GenericTransformer(sourceType, targetType, messageMappings);
    }
}

/**
 * HL7 to FHIR transformer using Go backend
 */
class HL7ToFHIRTransformer {
    constructor(messageMappings) {
        this.messageMappings = messageMappings;
        this.goBackendUrl = process.env.API_PORT ?
            `http://localhost:${process.env.API_PORT}` :
            'http://localhost:8080';
    }

    async transform(hl7Message, messageId, lineageId) {
        console.log(`🔄 Transforming HL7 message ${messageId} to FHIR`);

        try {
            // Extract message type from HL7
            const messageType = this.extractMessageType(hl7Message);

            // Find mapping configuration for this message type
            const mappingConfig = this.findMappingForMessageType(messageType);

            // Call Go backend for transformation
            const response = await axios.post(`${this.goBackendUrl}/api/fhir/transform`, {
                hl7Message: hl7Message,
                messageType: messageType,
                mappingConfig: mappingConfig,
                messageId: messageId,
                lineageId: lineageId
            }, {
                headers: {
                    'Content-Type': 'application/json'
                },
                timeout: 30000 // 30 second timeout
            });

            console.log(`✅ HL7 message ${messageId} transformed successfully`);
            return response.data;

        } catch (error) {
            console.error(`❌ HL7 transformation failed for message ${messageId}:`, error.message);

            // Enhanced error context for debugging
            const enhancedError = new Error(`HL7 to FHIR transformation failed: ${error.message}`);
            enhancedError.originalError = error;
            enhancedError.messageId = messageId;
            enhancedError.lineageId = lineageId;
            enhancedError.hl7MessagePreview = hl7Message?.substring(0, 200) + '...';

            throw enhancedError;
        }
    }

    extractMessageType(hl7Message) {
        if (typeof hl7Message === 'string' && hl7Message.startsWith('MSH')) {
            const segments = hl7Message.split('\r');
            const mshSegment = segments[0];
            const fields = mshSegment.split('|');
            return fields[8] || 'UNKNOWN'; // MSH.9 contains message type
        }
        return 'UNKNOWN';
    }

    findMappingForMessageType(messageType) {
        // Find mapping configuration for this specific message type
        if (this.messageMappings && Array.isArray(this.messageMappings)) {
            const mapping = this.messageMappings.find(m =>
                m.messageType === messageType ||
                m.messageType === messageType.split('^')[0] // Handle trigger event variations
            );

            if (mapping) {
                return mapping.mappingConfig;
            }
        }

        console.warn(`⚠️ No specific mapping found for message type: ${messageType}, using default`);
        return {}; // Return empty config, Go backend will use defaults
    }
}

/**
 * FHIR to HL7 transformer
 */
class FHIRToHL7Transformer {
    constructor(messageMappings) {
        this.messageMappings = messageMappings;
        this.goBackendUrl = process.env.API_PORT ?
            `http://localhost:${process.env.API_PORT}` :
            'http://localhost:8080';
    }

    async transform(fhirBundle, messageId, lineageId) {
        console.log(`🔄 Transforming FHIR bundle ${messageId} to HL7`);

        try {
            const response = await axios.post(`${this.goBackendUrl}/api/hl7/transform`, {
                fhirBundle: fhirBundle,
                mappingConfig: this.messageMappings[0]?.mappingConfig || {},
                messageId: messageId,
                lineageId: lineageId
            }, {
                headers: {
                    'Content-Type': 'application/json'
                },
                timeout: 30000
            });

            console.log(`✅ FHIR bundle ${messageId} transformed successfully`);
            return response.data;

        } catch (error) {
            console.error(`❌ FHIR transformation failed for message ${messageId}:`, error.message);

            const enhancedError = new Error(`FHIR to HL7 transformation failed: ${error.message}`);
            enhancedError.originalError = error;
            enhancedError.messageId = messageId;
            enhancedError.lineageId = lineageId;

            throw enhancedError;
        }
    }
}

/**
 * Generic transformer for other data format conversions
 */
class GenericTransformer {
    constructor(sourceType, targetType, messageMappings) {
        this.sourceType = sourceType;
        this.targetType = targetType;
        this.messageMappings = messageMappings;
    }

    async transform(sourceData, messageId, lineageId) {
        console.log(`🔄 Generic transformation ${messageId}: ${this.sourceType} -> ${this.targetType}`);

        try {
            // Implement generic transformation logic based on mapping rules
            const transformedData = this.applyMappingRules(sourceData);

            console.log(`✅ Generic transformation ${messageId} completed`);
            return {
                transformedData: transformedData,
                messageId: messageId,
                lineageId: lineageId,
                sourceType: this.sourceType,
                targetType: this.targetType
            };

        } catch (error) {
            console.error(`❌ Generic transformation failed for message ${messageId}:`, error.message);
            throw error;
        }
    }

    applyMappingRules(sourceData) {
        // Basic mapping logic - can be enhanced based on requirements
        if (!this.messageMappings || this.messageMappings.length === 0) {
            return sourceData; // Pass-through if no mappings defined
        }

        let transformedData = {};

        // Apply each mapping rule
        this.messageMappings.forEach(mapping => {
            if (mapping.mappingConfig) {
                // Apply transformation rules from mapping config
                Object.keys(mapping.mappingConfig).forEach(targetField => {
                    const sourceField = mapping.mappingConfig[targetField];
                    if (sourceData[sourceField] !== undefined) {
                        transformedData[targetField] = sourceData[sourceField];
                    }
                });
            }
        });

        return transformedData;
    }
}

module.exports = TransformerFactory;