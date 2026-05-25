// services/WizardMappingService.js
// FIXED VERSION - Uses your existing database setup instead of missing config/database

class WizardMappingService {
    constructor(pgPool = null) {
        // FIXED: Use your existing database connection approach
        this.pool = pgPool || this.getExistingDatabaseConnection();
        this.logger = this.getLogger();
    }

    /**
     * Get existing database connection from your current setup
     */
    getExistingDatabaseConnection() {
        try {
            // Option 1: Try to use existing interfaceService pool (most likely)
            const interfaceService = require('./interfaceService');
            if (interfaceService && interfaceService.pool) {
                console.log('✅ Using interfaceService database pool');
                return interfaceService.pool;
            }
        } catch (error) {
            console.log('⚠️ interfaceService not available, trying alternatives...');
        }

        try {
            // Option 2: Try to use userService pool
            const userService = require('./userService');
            if (userService && userService.pool) {
                console.log('✅ Using userService database pool');
                return userService.pool;
            }
        } catch (error) {
            console.log('⚠️ userService not available, trying alternatives...');
        }

        try {
            // Option 3: Try to create a new pool using your existing pattern
            const { Pool } = require('pg');
            const pool = new Pool({
                host: process.env.DB_HOST || 'localhost',
                port: process.env.DB_PORT || 5432,
                database: process.env.DB_NAME || 'ezhealthkonnect',
                user: process.env.DB_USER || 'postgres',
                password: process.env.DB_PASSWORD || 'password',
                max: 20,
                idleTimeoutMillis: 30000,
                connectionTimeoutMillis: 2000,
            });
            console.log('✅ Created new database pool for WizardMappingService');
            return pool;
        } catch (error) {
            console.error('❌ Failed to create database connection:', error);
            throw new Error('Cannot establish database connection for WizardMappingService');
        }
    }

    /**
     * Get logger - use existing or fallback to console
     */
    getLogger() {
        try {
            return require('../utils/logger');
        } catch (error) {
            console.log('⚠️ Logger not available, using console fallback');
            return {
                info: console.log.bind(console, '[INFO]'),
                warn: console.warn.bind(console, '[WARN]'),
                error: console.error.bind(console, '[ERROR]'),
                debug: console.log.bind(console, '[DEBUG]')
            };
        }
    }

    /**
     * Save complete wizard configuration to existing database schema
     * @param {string} interfaceId - Interface UUID
     * @param {Object} wizardData - Complete wizard configuration data
     * @returns {Promise<Object>} - Save result with mapping IDs
     */
    async saveWizardConfiguration(interfaceId, wizardData) {
        const client = await this.pool.connect();
        
        try {
            await client.query('BEGIN');
            this.logger.info(`Starting wizard configuration save for interface: ${interfaceId}`);

            // 1. Update interfaces table with transformation mapping
            const interfaceResult = await this.updateInterfaceTransformationMapping(
                client, 
                interfaceId, 
                wizardData
            );

            // 2. Save individual field mappings to hl7_fhir_mappings table
            const fieldMappingResults = await this.saveFieldMappings(
                client, 
                interfaceId, 
                wizardData
            );

            // 3. Save FHIR resource templates to message_fhir_templates table
            const templateResult = await this.saveFhirResourceTemplates(
                client, 
                wizardData
            );

            // 4. Save any value set mappings discovered during wizard
            const valueSetResults = await this.saveValueSetMappings(
                client, 
                wizardData
            );

            await client.query('COMMIT');

            const result = {
                success: true,
                interfaceId: interfaceId,
                savedMappings: {
                    interfaceUpdated: !!interfaceResult,
                    fieldMappings: fieldMappingResults.length,
                    templatesCreated: templateResult ? 1 : 0,
                    valueSetMappings: valueSetResults.length
                },
                data: {
                    fieldMappingIds: fieldMappingResults,
                    templateId: templateResult?.id,
                    valueSetMappingIds: valueSetResults
                }
            };

            this.logger.info(`Wizard configuration saved successfully: ${JSON.stringify(result.savedMappings)}`);
            return result;

        } catch (error) {
            await client.query('ROLLBACK');
            this.logger.error(`Failed to save wizard configuration: ${error.message}`, {
                interfaceId,
                error: error.stack
            });
            
            return {
                success: false,
                error: error.message,
                interfaceId: interfaceId
            };
        } finally {
            client.release();
        }
    }

    /**
     * Update interfaces table with compiled transformation mapping
     * Uses existing interfaces.transformation_mapping JSONB column
     */
    async updateInterfaceTransformationMapping(client, interfaceId, wizardData) {
        this.logger.info('🔍 DEBUGGING: updateInterfaceTransformationMapping called');
        
        const transformationMapping = this.compileTransformationMapping(wizardData);
        
        this.logger.info('🔍 Transformation mapping size:', JSON.stringify(transformationMapping).length);
        this.logger.info('🔍 Transformation mapping preview:', {
            messageType: transformationMapping.messageType,
            totalMappings: transformationMapping.mappingData?.totalMappings,
            resourceTypes: transformationMapping.mappingData?.resourceTypes
        });
        
        // Extract message types for multi-message support
        const messageTypes = this.extractMessageTypes(wizardData);

        const query = `
            UPDATE interfaces
            SET
                transformation_mapping = $1,
                message_type = $2,
                supported_message_types = $3,
                updated_at = CURRENT_TIMESTAMP,
                version = COALESCE(version, 0) + 1
            WHERE id = $4
            RETURNING id, version, transformation_mapping, supported_message_types
        `;

        const values = [
            JSON.stringify(transformationMapping),
            wizardData.detectedMessageType || wizardData.messageType,
            JSON.stringify(messageTypes),
            interfaceId
        ];

        this.logger.info('🔍 About to execute UPDATE query for interface:', interfaceId);
        const result = await client.query(query, values);
        
        if (result.rows.length === 0) {
            throw new Error(`Interface not found: ${interfaceId}`);
        }

        const updatedRow = result.rows[0];
        
        this.logger.info('✅ Interface updated successfully:', {
            interfaceId: updatedRow.id,
            version: updatedRow.version,
            transformationMappingSize: JSON.stringify(updatedRow.transformation_mapping).length,
            transformationMappingPreview: {
                messageType: updatedRow.transformation_mapping?.messageType,
                totalMappings: updatedRow.transformation_mapping?.mappingData?.totalMappings
            }
        });

        return updatedRow;
    }

    /**
     * Save individual field mappings to existing hl7_fhir_mappings table
     * Converts wizard atomicMappings to database format
     */
    async saveFieldMappings(client, interfaceId, wizardData) {
        const atomicMappings = this.extractAtomicMappings(wizardData);
        const savedMappingIds = [];

        if (!atomicMappings || atomicMappings.length === 0) {
            this.logger.info('No atomic mappings to save');
            return savedMappingIds;
        }

        // Get user ID from interface
        const userResult = await client.query(
            'SELECT user_id FROM interfaces WHERE id = $1',
            [interfaceId]
        );
        const userId = userResult.rows[0]?.user_id;

        for (const mapping of atomicMappings) {
            try {
                const fieldMapping = this.convertAtomicMappingToFieldMapping(mapping, wizardData);
                
                const insertQuery = `
                    INSERT INTO hl7_fhir_mappings (
                        interface_id,
                        hl7_version,
                        hl7_message_type,
                        hl7_segment,
                        hl7_field,
                        hl7_component,
                        fhir_resource,
                        fhir_profile,
                        fhir_path,
                        transformation_rule,
                        condition_expression,
                        is_required,
                        priority,
                        created_by,
                        is_active
                    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
                    ON CONFLICT (hl7_message_type, hl7_segment, hl7_field, fhir_resource, fhir_path)
                    DO UPDATE SET
                        interface_id = EXCLUDED.interface_id,
                        transformation_rule = EXCLUDED.transformation_rule,
                        updated_at = CURRENT_TIMESTAMP,
                        is_active = true
                    RETURNING id
                `;

                const values = [
                    interfaceId, // NEW: Include interface_id
                    fieldMapping.hl7_version,
                    fieldMapping.hl7_message_type,
                    fieldMapping.hl7_segment,
                    fieldMapping.hl7_field,
                    fieldMapping.hl7_component,
                    fieldMapping.fhir_resource,
                    fieldMapping.fhir_profile,
                    fieldMapping.fhir_path,
                    JSON.stringify(fieldMapping.transformation_rule),
                    fieldMapping.condition_expression,
                    fieldMapping.is_required,
                    fieldMapping.priority,
                    userId,
                    true
                ];

                const result = await client.query(insertQuery, values);
                savedMappingIds.push(result.rows[0].id);

            } catch (error) {
                this.logger.warn(`Failed to save field mapping: ${error.message}`, {
                    mapping: mapping,
                    error: error.message
                });
                // Continue with other mappings
            }
        }

        this.logger.info(`Saved ${savedMappingIds.length} field mappings out of ${atomicMappings.length} total`);
        return savedMappingIds;
    }

    /**
     * Save FHIR resource templates to existing message_fhir_templates table
     */
    async saveFhirResourceTemplates(client, wizardData) {
        const messageType = wizardData.detectedMessageType || wizardData.messageType;
        const fhirResources = this.extractFhirResources(wizardData);

        if (!fhirResources || fhirResources.length === 0) {
            this.logger.info('No FHIR resources to save as template');
            return null;
        }

        const templateData = {
            resourceTypes: [...new Set(fhirResources.map(r => r.resourceType))],
            sampleResources: fhirResources.slice(0, 5), // Store first 5 as samples
            totalResourceCount: fhirResources.length,
            generatedAt: new Date().toISOString(),
            wizardVersion: '1.0'
        };

        const insertQuery = `
            INSERT INTO message_fhir_templates (
                message_type,
                fhir_resources,
                template_version,
                description,
                is_active
            ) VALUES ($1, $2, $3, $4, $5)
            ON CONFLICT (message_type) 
            DO UPDATE SET 
                fhir_resources = EXCLUDED.fhir_resources,
                template_version = COALESCE(message_fhir_templates.template_version, '1.0'),
                updated_at = CURRENT_TIMESTAMP,
                is_active = true
            RETURNING id
        `;

        const values = [
            messageType,
            JSON.stringify(templateData),
            '1.0',
            `Template generated from wizard for ${messageType}`,
            true
        ];

        const result = await client.query(insertQuery, values);
        
        this.logger.info(`Saved FHIR resource template for message type: ${messageType}`, {
            templateId: result.rows[0].id,
            resourceTypes: templateData.resourceTypes
        });

        return result.rows[0];
    }

    /**
     * Save value set mappings discovered during wizard process
     */
    async saveValueSetMappings(client, wizardData) {
        const valueSetMappings = this.extractValueSetMappings(wizardData);
        const savedMappingIds = [];

        if (!valueSetMappings || valueSetMappings.length === 0) {
            this.logger.info('No value set mappings to save');
            return savedMappingIds;
        }

        for (const mapping of valueSetMappings) {
            try {
                const insertQuery = `
                    INSERT INTO value_set_mappings (
                        mapping_name,
                        hl7_table,
                        hl7_value,
                        fhir_system,
                        fhir_code,
                        fhir_display,
                        mapping_type,
                        context_conditions,
                        source
                    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
                    ON CONFLICT (hl7_value, fhir_system, fhir_code)
                    DO NOTHING
                    RETURNING id
                `;

                const values = [
                    mapping.mapping_name,
                    mapping.hl7_table,
                    mapping.hl7_value,
                    mapping.fhir_system,
                    mapping.fhir_code,
                    mapping.fhir_display,
                    'wizard_generated',
                    mapping.context_conditions ? JSON.stringify(mapping.context_conditions) : null,
                    'interface_wizard'
                ];

                const result = await client.query(insertQuery, values);
                if (result.rows.length > 0) {
                    savedMappingIds.push(result.rows[0].id);
                }

            } catch (error) {
                this.logger.warn(`Failed to save value set mapping: ${error.message}`, {
                    mapping: mapping
                });
                // Continue with other mappings
            }
        }

        this.logger.info(`Saved ${savedMappingIds.length} value set mappings`);
        return savedMappingIds;
    }

    // ====================================
    // DATA TRANSFORMATION HELPER METHODS
    // ====================================

    /**
     * Compile wizard data into transformation mapping for interfaces.transformation_mapping
     */
    compileTransformationMapping(wizardData) {
        this.logger.info('🔍 DEBUGGING: compileTransformationMapping called');
        this.logger.info('🔍 WizardData keys:', Object.keys(wizardData || {}));
        
        const messageType = wizardData.detectedMessageType || wizardData.messageType;
        this.logger.info('🔍 Message Type:', messageType);
        
        const segments = wizardData.enhancedSegments || {};
        this.logger.info('🔍 Enhanced Segments count:', Object.keys(segments).length);
        
        const atomicMappings = this.extractAtomicMappings(wizardData);
        this.logger.info('🔍 Atomic Mappings count:', atomicMappings?.length || 0);

        // ENHANCED: Log the actual atomic mappings
        if (atomicMappings && atomicMappings.length > 0) {
            this.logger.info('✅ Found atomic mappings:', atomicMappings.slice(0, 3)); // Log first 3
        } else {
            this.logger.error('❌ NO ATOMIC MAPPINGS FOUND - transformation_mapping will be minimal');
            this.logger.info('🔍 Full wizardData structure:', JSON.stringify(wizardData, null, 2));
        }

        const compiledMapping = {
            messageType: messageType,
            version: '1.0',
            compiledAt: new Date().toISOString(),
            sourceData: {
                segmentCount: Object.keys(segments).length,
                segmentTypes: Object.keys(segments),
                totalFields: this.countTotalFields(segments)
            },
            mappingData: {
                totalMappings: atomicMappings.length,
                resourceTypes: [...new Set(atomicMappings.map(m => m.resourceType))],
                mappingsByResource: this.groupMappingsByResource(atomicMappings)
            },
            transformationConfig: {
                strategy: 'wizard_generated',
                strictValidation: false,
                createBundle: true,
                includeMetadata: true
            },
            wizardMetadata: {
                completedSteps: ['configuration', 'upload', 'review', 'mapping', 'summary'],
                generatedAt: new Date().toISOString(),
                wizardVersion: '1.0'
            }
        };

        this.logger.info('✅ Compiled transformation mapping:', JSON.stringify(compiledMapping, null, 2));
        return compiledMapping;
    }

    /**
     * Extract atomic mappings from wizard data
     */
    extractAtomicMappings(wizardData) {
        this.logger.info('🔍 DEBUGGING: extractAtomicMappings called');
        this.logger.info('🔍 WizardData available keys:', Object.keys(wizardData || {}));
        
        // Try multiple possible locations for atomic mappings
        let mappings = [];

        // Strategy 1: Direct atomicMappings
        if (wizardData.atomicMappings) {
            mappings = wizardData.atomicMappings;
            this.logger.info('✅ Found atomicMappings directly:', mappings.length);
        }
        // Strategy 2: In mappingData
        else if (wizardData.mappingData?.atomicMappings) {
            mappings = wizardData.mappingData.atomicMappings;
            this.logger.info('✅ Found atomicMappings in mappingData:', mappings.length);
        }
        // Strategy 3: In mappingConfiguration
        else if (wizardData.mappingConfiguration?.atomicMappings) {
            mappings = wizardData.mappingConfiguration.atomicMappings;
            this.logger.info('✅ Found atomicMappings in mappingConfiguration:', mappings.length);
        }
        // Strategy 4: In fhirTransformResult
        else if (wizardData.fhirTransformResult?.mappings) {
            mappings = wizardData.fhirTransformResult.mappings;
            this.logger.info('✅ Found mappings in fhirTransformResult:', mappings.length);
        }
        // Strategy 5: Look for step4Data
        else if (wizardData.step4Data?.atomicMappings) {
            mappings = wizardData.step4Data.atomicMappings;
            this.logger.info('✅ Found atomicMappings in step4Data:', mappings.length);
        }
        else {
            this.logger.error('❌ ATOMIC MAPPINGS NOT FOUND IN ANY EXPECTED LOCATION');
            this.logger.info('🔍 Detailed wizardData structure:');
            this.logger.info('  - wizardData.atomicMappings:', wizardData.atomicMappings ? 'EXISTS' : 'MISSING');
            this.logger.info('  - wizardData.mappingData:', wizardData.mappingData ? 'EXISTS' : 'MISSING');
            this.logger.info('  - wizardData.mappingConfiguration:', wizardData.mappingConfiguration ? 'EXISTS' : 'MISSING');
            this.logger.info('  - wizardData.fhirTransformResult:', wizardData.fhirTransformResult ? 'EXISTS' : 'MISSING');
            this.logger.info('  - wizardData.step4Data:', wizardData.step4Data ? 'EXISTS' : 'MISSING');
            
            // Log the full structure for debugging
            if (this.debugMode) {
                this.logger.info('🔍 FULL wizardData for debugging:', JSON.stringify(wizardData, null, 2));
            }
        }
        
        // Validate mapping structure
        if (mappings && mappings.length > 0) {
            const validMappings = mappings.filter(mapping => 
                mapping && 
                mapping.hl7Field && 
                mapping.fhirPath && 
                mapping.resourceType
            );
            
            if (validMappings.length !== mappings.length) {
                this.logger.warn(`⚠️ Found ${mappings.length} mappings, but only ${validMappings.length} are valid`);
            }
            
            return validMappings;
        }
        
        return [];
    }

    /**
     * Extract FHIR resources from wizard data
     */
    extractFhirResources(wizardData) {
        return wizardData.fhirTransformResult?.fhirResources ||
               wizardData.transformationData?.fhirResources ||
               [];
    }

    /**
     * Extract message types from wizard data for multi-message support
     * Returns array of HL7 message types this interface can handle
     */
    extractMessageTypes(wizardData) {
        // Primary message type from wizard detection
        const primaryMessageType = wizardData.detectedMessageType || wizardData.messageType;

        // Start with primary message type
        const messageTypes = primaryMessageType ? [primaryMessageType] : [];

        // Check if there are additional message types in the enhanced segments
        if (wizardData.enhancedSegments) {
            Object.values(wizardData.enhancedSegments).forEach(segment => {
                if (segment.messageType && !messageTypes.includes(segment.messageType)) {
                    messageTypes.push(segment.messageType);
                }
            });
        }

        // Check atomic mappings for additional message types
        const atomicMappings = this.extractAtomicMappings(wizardData);
        atomicMappings.forEach(mapping => {
            if (mapping.messageType && !messageTypes.includes(mapping.messageType)) {
                messageTypes.push(mapping.messageType);
            }
        });

        // Default to ADT^A01 if no message types found
        if (messageTypes.length === 0) {
            messageTypes.push('ADT^A01');
        }

        this.logger.info('🔍 Extracted message types:', messageTypes);
        return messageTypes;
    }

    /**
     * Convert atomic mapping to hl7_fhir_mappings table format
     */
    convertAtomicMappingToFieldMapping(atomicMapping, wizardData) {
        const messageType = wizardData.detectedMessageType || wizardData.messageType || 'Unknown';
        const hl7Parts = this.parseHL7FieldPath(atomicMapping.hl7Field);
        
        return {
            hl7_version: '2.5',
            hl7_message_type: messageType,
            hl7_segment: hl7Parts.segment,
            hl7_field: hl7Parts.field,
            hl7_component: hl7Parts.component,
            fhir_resource: atomicMapping.resourceType,
            fhir_profile: this.getFhirProfile(atomicMapping.resourceType),
            fhir_path: atomicMapping.fhirPath,
            transformation_rule: {
                type: 'direct',
                sourceValue: atomicMapping.value,
                transformationType: atomicMapping.transformationType || 'direct',
                validation: atomicMapping.validated !== false
            },
            condition_expression: null,
            is_required: false,
            priority: 0
        };
    }

    /**
     * Parse HL7 field path (e.g., "PID.5.1" -> {segment: "PID", field: "5", component: "1"})
     */
    parseHL7FieldPath(fieldPath) {
        const parts = fieldPath.split('.');
        return {
            segment: parts[0] || '',
            field: parts[1] || '',
            component: parts[2] || null
        };
    }

    /**
     * Get FHIR profile for resource type
     */
    getFhirProfile(resourceType) {
        const profiles = {
            'Patient': 'http://hl7.org/fhir/StructureDefinition/Patient',
            'MessageHeader': 'http://hl7.org/fhir/StructureDefinition/MessageHeader',
            'Encounter': 'http://hl7.org/fhir/StructureDefinition/Encounter',
            'Observation': 'http://hl7.org/fhir/StructureDefinition/Observation',
            'Organization': 'http://hl7.org/fhir/StructureDefinition/Organization',
            'Practitioner': 'http://hl7.org/fhir/StructureDefinition/Practitioner'
        };
        return profiles[resourceType] || `http://hl7.org/fhir/StructureDefinition/${resourceType}`;
    }

    /**
     * Extract value set mappings from wizard data
     */
    extractValueSetMappings(wizardData) {
        const mappings = [];
        const atomicMappings = this.extractAtomicMappings(wizardData);

        // Look for common value transformations
        const commonMappings = {
            'M': { system: 'http://hl7.org/fhir/administrative-gender', code: 'male', display: 'Male' },
            'F': { system: 'http://hl7.org/fhir/administrative-gender', code: 'female', display: 'Female' },
            'A04': { system: 'http://terminology.hl7.org/CodeSystem/v2-0003', code: 'A04', display: 'Register a patient' }
        };

        atomicMappings.forEach(mapping => {
            if (commonMappings[mapping.value]) {
                const fhirMapping = commonMappings[mapping.value];
                mappings.push({
                    mapping_name: `${mapping.value}_mapping`,
                    hl7_table: null,
                    hl7_value: mapping.value,
                    fhir_system: fhirMapping.system,
                    fhir_code: fhirMapping.code,
                    fhir_display: fhirMapping.display,
                    context_conditions: { sourceField: mapping.hl7Field }
                });
            }
        });

        return mappings;
    }

    /**
     * Count total fields in all segments
     */
    countTotalFields(segments) {
        return Object.values(segments).reduce((total, segment) => {
            return total + (segment.fields ? segment.fields.length : 0);
        }, 0);
    }

    /**
     * Group mappings by FHIR resource type
     */
    groupMappingsByResource(mappings) {
        return mappings.reduce((grouped, mapping) => {
            const resourceType = mapping.resourceType;
            if (!grouped[resourceType]) {
                grouped[resourceType] = [];
            }
            grouped[resourceType].push({
                hl7Field: mapping.hl7Field,
                fhirPath: mapping.fhirPath,
                value: mapping.value
            });
            return grouped;
        }, {});
    }
}

module.exports = WizardMappingService;