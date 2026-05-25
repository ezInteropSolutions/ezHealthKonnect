/**
 * SampleMessageService - Manage sample parsed messages for XPath autocomplete
 *
 * Purpose: Provide pre-parsed message samples so field selection works
 * even when no real messages exist in the system.
 */

const { Pool } = require('pg');

// Create a database connection pool
function getPool() {
    return new Pool({
        host: process.env.DB_HOST || 'localhost',
        port: process.env.DB_PORT || 5432,
        database: process.env.DB_NAME || 'ezhealthkonnect',
        user: process.env.DB_USER || 'postgres',
        password: process.env.DB_PASSWORD || 'postgres'
    });
}

class SampleMessageService {
    /**
     * Get sample parsed message for autocomplete
     * @param {string} messageType - e.g., 'ADT^A01'
     * @param {string} version - e.g., '2.5'
     * @param {string} format - e.g., 'hl7v2'
     * @returns {Object|null} Parsed message with enhancedSegments
     */
    static async getSample(messageType, version = '2.5', format = 'hl7v2') {
        const pool = getPool();
        try {
            const query = `
                SELECT parsed_content, description
                FROM sample_parsed_messages
                WHERE message_type = $1
                  AND hl7_version = $2
                  AND format = $3
                  AND is_active = TRUE
                LIMIT 1
            `;

            const result = await pool.query(query, [messageType, version, format]);

            if (result.rows.length > 0) {
                return result.rows[0].parsed_content;
            }

            // Fallback: try to find any version of this message type
            const fallbackQuery = `
                SELECT parsed_content, description, hl7_version
                FROM sample_parsed_messages
                WHERE message_type = $1
                  AND format = $2
                  AND is_active = TRUE
                ORDER BY hl7_version DESC
                LIMIT 1
            `;

            const fallbackResult = await pool.query(fallbackQuery, [messageType, format]);

            if (fallbackResult.rows.length > 0) {
                console.log(`⚠️  Using ${fallbackResult.rows[0].hl7_version} sample for ${messageType} (requested ${version})`);
                return fallbackResult.rows[0].parsed_content;
            }

            return null;
        } catch (error) {
            console.error('Error getting sample message:', error);
            return null;
        } finally {
            await pool.end();
        }
    }

    /**
     * Insert or update a sample parsed message
     * @param {Object} sampleData
     */
    static async upsertSample(sampleData) {
        const {
            messageType,
            hlVersion,
            format = 'hl7v2',
            parsedContent,
            description,
            interfaceId = null,
            sampleScope = interfaceId ? 'interface' : 'global',
            source = 'user-upload',
            priority = interfaceId ? 10 : 50  // Interface samples higher priority
        } = sampleData;

        const pool = getPool();
        try {
            const query = `
                INSERT INTO sample_parsed_messages
                    (message_type, hl7_version, format, parsed_content, description,
                     interface_id, sample_scope, source, priority)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
                ON CONFLICT (message_type, hl7_version, format, is_active)
                DO UPDATE SET
                    parsed_content = EXCLUDED.parsed_content,
                    description = EXCLUDED.description,
                    interface_id = EXCLUDED.interface_id,
                    sample_scope = EXCLUDED.sample_scope,
                    source = EXCLUDED.source,
                    priority = EXCLUDED.priority,
                    updated_at = CURRENT_TIMESTAMP
                RETURNING id
            `;

            const result = await pool.query(query, [
                messageType,
                hlVersion,
                format,
                parsedContent,
                description,
                interfaceId,
                sampleScope,
                source,
                priority
            ]);

            const scope = interfaceId ? `interface ${interfaceId}` : 'global';
            console.log(`✅ Sample message saved: ${messageType} v${hlVersion} (${scope}, ID: ${result.rows[0].id})`);
            return result.rows[0].id;
        } catch (error) {
            console.error('Error upserting sample message:', error);
            throw error;
        } finally {
            await pool.end();
        }
    }

    /**
     * Get all available sample message types
     */
    static async listSamples() {
        const pool = getPool();
        try {
            const query = `
                SELECT message_type, hl7_version, format, description
                FROM sample_parsed_messages
                WHERE is_active = TRUE
                ORDER BY message_type, hl7_version
            `;

            const result = await pool.query(query);
            return result.rows;
        } catch (error) {
            console.error('Error listing samples:', error);
            return [];
        } finally {
            await pool.end();
        }
    }

    /**
     * Build XPath tree from enhancedSegments structure
     * @param {Object} enhancedSegments - Parsed message segments
     * @returns {Object} XPath tree structure
     */
    static buildXPathTree(enhancedSegments) {
        const tree = {
            name: 'enhancedSegments',
            path: 'enhancedSegments',
            type: 'object',
            description: 'HL7 parsed message segments',
            children: []
        };

        // Process each segment
        for (const [segmentKey, segment] of Object.entries(enhancedSegments)) {
            const segmentNode = {
                name: segmentKey,
                path: `enhancedSegments.${segmentKey}`,
                type: 'segment',
                description: segment.description || segment.name,
                children: []
            };

            // Add fields array
            if (segment.fields && Array.isArray(segment.fields)) {
                const fieldsNode = {
                    name: 'fields',
                    path: `enhancedSegments.${segmentKey}.fields`,
                    type: 'array',
                    description: 'Segment fields',
                    children: []
                };

                // Process each field
                segment.fields.forEach((field, index) => {
                    // PRIMARY: Create direct path to field.value (most common use case)
                    const fieldValueNode = {
                        name: field.key || `${segmentKey}.${index + 1}`,
                        path: `enhancedSegments.${segmentKey}.fields[${index}].value`,
                        type: 'field-value',
                        dataType: field.dataType,
                        description: field.name || field.description,
                        cardinality: field.cardinality || '',
                        children: []
                    };

                    // Add this to the main level (what users will select)
                    fieldsNode.children.push(fieldValueNode);

                    // OPTIONAL: Also keep field object for advanced users
                    const fieldNode = {
                        name: `${field.key || segmentKey} (object)`,
                        path: `enhancedSegments.${segmentKey}.fields[${index}]`,
                        type: 'field-object',
                        dataType: field.dataType,
                        description: `${field.name || field.description} - Full field object`,
                        children: []
                    };

                    // Add other field properties as children of the object node
                    ['key', 'name', 'position', 'dataType'].forEach(prop => {
                        fieldNode.children.push({
                            name: prop,
                            path: `enhancedSegments.${segmentKey}.fields[${index}].${prop}`,
                            type: 'string',
                            description: `Field ${prop}`,
                            example: field[prop]
                        });
                    });

                    // Add subfields if they exist
                    if (field.subfields && Array.isArray(field.subfields)) {
                        const subfieldsNode = {
                            name: 'subfields',
                            path: `enhancedSegments.${segmentKey}.fields[${index}].subfields`,
                            type: 'array',
                            description: 'Field components/subfields',
                            children: []
                        };

                        field.subfields.forEach((subfield, subIndex) => {
                            subfieldsNode.children.push({
                                name: subfield.key || `${field.key}.${subIndex + 1}`,
                                path: `enhancedSegments.${segmentKey}.fields[${index}].subfields[${subIndex}].value`,
                                type: 'string',
                                dataType: subfield.dataType,
                                description: subfield.name || subfield.description,
                                example: subfield.value
                            });
                        });

                        fieldNode.children.push(subfieldsNode);
                    }

                    fieldsNode.children.push(fieldNode);
                });

                segmentNode.children.push(fieldsNode);
            }

            tree.children.push(segmentNode);
        }

        return tree;
    }

    /**
     * Build field tree with intelligent fallback
     * Priority: Interface-specific → System library → Aggregated
     * @param {string} format - 'hl7v2', 'fhir', etc.
     * @param {string} messageType - Optional: 'ADT^A01', 'Patient', etc.
     * @param {number} interfaceId - Optional: specific interface
     * @returns {Object} XPath tree structure
     */
    static async buildFieldTreeWithFallback(format = 'hl7v2', messageType = null, interfaceId = null) {
        const pool = getPool();
        try {
            // Priority 1: Interface-specific sample (user uploaded)
            if (interfaceId && messageType) {
                const interfaceSample = await pool.query(`
                    SELECT parsed_content, source
                    FROM sample_parsed_messages
                    WHERE interface_id = $1
                      AND message_type = $2
                      AND format = $3
                      AND is_active = TRUE
                    ORDER BY priority ASC, updated_at DESC
                    LIMIT 1
                `, [interfaceId, messageType, format]);

                if (interfaceSample.rows.length > 0) {
                    console.log(`✅ Using interface-specific sample (${interfaceSample.rows[0].source})`);
                    return this.buildXPathTree(interfaceSample.rows[0].parsed_content.enhancedSegments);
                }
            }

            // Priority 2: System library for this message type
            if (messageType) {
                const librarySample = await pool.query(`
                    SELECT parsed_content
                    FROM sample_parsed_messages
                    WHERE message_type = $1
                      AND format = $2
                      AND sample_scope = 'global'
                      AND is_active = TRUE
                    ORDER BY priority ASC, updated_at DESC
                    LIMIT 1
                `, [messageType, format]);

                if (librarySample.rows.length > 0) {
                    console.log(`✅ Using system library sample for ${messageType}`);
                    return this.buildXPathTree(librarySample.rows[0].parsed_content.enhancedSegments);
                }
            }

            // Priority 3: Aggregate all samples of this format (universal fallback)
            console.log(`⚠️  No specific sample found, using aggregated fields from all samples`);
            return await this.buildUniversalFieldTree(format);

        } catch (error) {
            console.error('Error in buildFieldTreeWithFallback:', error);
            throw error;
        } finally {
            await pool.end();
        }
    }

    /**
     * Build universal field tree by aggregating ALL samples
     * This creates a message-type-agnostic tree with all possible fields
     * @param {string} format - 'hl7v2', 'fhir', etc.
     * @returns {Object} Universal XPath tree with all segments/fields
     */
    static async buildUniversalFieldTree(format = 'hl7v2') {
        const pool = getPool();
        try {
            // Get all active samples for this format
            const query = `
                SELECT parsed_content, message_type, hl7_version
                FROM sample_parsed_messages
                WHERE format = $1 AND is_active = TRUE
                ORDER BY message_type, hl7_version DESC
            `;

            const result = await pool.query(query, [format]);

            if (result.rows.length === 0) {
                return null;
            }

            console.log(`📊 Aggregating fields from ${result.rows.length} sample(s)`);

            // Merge all enhancedSegments into a universal structure
            const allSegments = {};

            for (const row of result.rows) {
                const enhancedSegments = row.parsed_content.enhancedSegments;

                if (!enhancedSegments) continue;

                // Merge segments
                for (const [segmentKey, segment] of Object.entries(enhancedSegments)) {
                    if (!allSegments[segmentKey]) {
                        allSegments[segmentKey] = {
                            key: segmentKey,
                            name: segment.name,
                            description: segment.description,
                            fields: []
                        };
                    }

                    // Merge fields (use a Map to deduplicate by field key)
                    const existingFieldKeys = new Set(
                        allSegments[segmentKey].fields.map(f => f.key)
                    );

                    if (segment.fields && Array.isArray(segment.fields)) {
                        for (const field of segment.fields) {
                            if (!existingFieldKeys.has(field.key)) {
                                allSegments[segmentKey].fields.push(field);
                                existingFieldKeys.add(field.key);
                            }
                        }
                    }
                }
            }

            // Build XPath tree from merged segments
            const universalTree = this.buildXPathTree(allSegments);

            console.log(`✅ Universal tree contains ${Object.keys(allSegments).length} unique segments`);

            return universalTree;
        } catch (error) {
            console.error('Error building universal field tree:', error);
            throw error;
        } finally {
            await pool.end();
        }
    }
}

module.exports = SampleMessageService;
