// controllers/schemaController.js
// API Controller for loading HL7 and FHIR schemas for XPath IntelliSense

const path = require('path');
const fs = require('fs').promises;
const zlib = require('zlib');
const { promisify } = require('util');
const SampleMessageService = require('../services/SampleMessageService');

const gunzip = promisify(zlib.gunzip);

/**
 * Get available HL7 versions
 * GET /api/schemas/hl7/versions
 */
exports.getHL7Versions = async (req, res) => {
    try {
        const hl7Dir = path.join(__dirname, '..', 'schemas', 'hl7');
        const versions = await fs.readdir(hl7Dir);

        // Filter only directories
        const versionList = [];
        for (const version of versions) {
            const stat = await fs.stat(path.join(hl7Dir, version));
            if (stat.isDirectory()) {
                versionList.push({
                    value: version,
                    label: version.replace('v', 'HL7 v'),
                });
            }
        }

        res.json({ success: true, versions: versionList });
    } catch (error) {
        console.error('Error getting HL7 versions:', error);
        res.status(500).json({ success: false, error: error.message });
    }
};

/**
 * Get available message types for HL7 version
 * GET /api/schemas/hl7/:version/message-types
 */
exports.getHL7MessageTypes = async (req, res) => {
    try {
        const { version } = req.params;
        const versionDir = path.join(__dirname, '..', 'schemas', 'hl7', version);

        const files = await fs.readdir(versionDir);
        const messageTypes = files
            .filter(file => file.endsWith('.gz'))
            .map(file => {
                // Convert file name (ADT_A01.gz) to API format (ADT^A01)
                const fileMessageType = file.replace('.gz', '');
                const apiMessageType = fileMessageType.replace(/_/g, '^');
                return {
                    value: apiMessageType,
                    label: apiMessageType,
                };
            });

        res.json({ success: true, messageTypes });
    } catch (error) {
        console.error('Error getting message types:', error);
        res.status(500).json({ success: false, error: error.message });
    }
};

/**
 * Get HL7 schema for specific version and message type
 * Strategy: Load sample parsed message from PostgreSQL
 * GET /api/schemas/hl7/:version/:messageType
 */
exports.getHL7Schema = async (req, res) => {
    try {
        const { version, messageType } = req.params;

        console.log(`📂 Loading sample parsed message: ${messageType} v${version}`);

        // Get sample parsed message from PostgreSQL
        const sampleParsed = await SampleMessageService.getSample(messageType, version, 'hl7v2');

        if (!sampleParsed || !sampleParsed.enhancedSegments) {
            console.error(`❌ No sample found for ${messageType} v${version}`);
            return res.status(404).json({
                success: false,
                error: `No sample message available for ${messageType} v${version}. Please add a sample via /api/schemas/samples endpoint.`
            });
        }

        // Build XPath tree from actual parsed structure
        const xpathTree = SampleMessageService.buildXPathTree(sampleParsed.enhancedSegments);

        console.log(`✅ Sample loaded: ${xpathTree.children.length} segments`);

        res.json({ success: true, xpathTree: xpathTree });
    } catch (error) {
        console.error('❌ Error getting HL7 schema:', error.message);
        res.status(500).json({ success: false, error: error.message });
    }
};

/**
 * Get available FHIR versions
 * GET /api/schemas/fhir/versions
 */
exports.getFHIRVersions = async (req, res) => {
    try {
        const fhirDir = path.join(__dirname, '..', 'schemas', 'fhir');
        const versions = await fs.readdir(fhirDir);

        const versionList = [];
        for (const version of versions) {
            const stat = await fs.stat(path.join(fhirDir, version));
            if (stat.isDirectory()) {
                versionList.push({
                    value: version,
                    label: `FHIR ${version}`,
                });
            }
        }

        res.json({ success: true, versions: versionList });
    } catch (error) {
        console.error('Error getting FHIR versions:', error);
        res.status(500).json({ success: false, error: error.message });
    }
};

/**
 * Get available FHIR resources
 * GET /api/schemas/fhir/:version/resources
 */
exports.getFHIRResources = async (req, res) => {
    try {
        const { version } = req.params;
        const resourcesDir = path.join(__dirname, '..', 'schemas', 'fhir', version, 'resources');

        const files = await fs.readdir(resourcesDir);
        const resources = files
            .filter(file => file.endsWith('.gz'))
            .map(file => {
                const resourceType = file.replace('.gz', '');
                return {
                    value: resourceType,
                    label: resourceType,
                };
            })
            .sort((a, b) => a.label.localeCompare(b.label));

        res.json({ success: true, resources });
    } catch (error) {
        console.error('Error getting FHIR resources:', error);
        res.status(500).json({ success: false, error: error.message });
    }
};

/**
 * Get FHIR resource schema
 * GET /api/schemas/fhir/:version/:resourceType
 */
exports.getFHIRSchema = async (req, res) => {
    try {
        const { version, resourceType } = req.params;
        const schemaPath = path.join(
            __dirname,
            '..',
            'schemas',
            'fhir',
            version,
            'resources',
            `${resourceType}.gz`
        );

        // Read compressed schema
        const compressed = await fs.readFile(schemaPath);
        const decompressed = await gunzip(compressed);
        const schema = JSON.parse(decompressed.toString());

        // Transform schema into XPath-friendly structure
        const xpathTree = buildFHIRXPathTree(schema, resourceType);

        res.json({ success: true, schema: xpathTree });
    } catch (error) {
        console.error('Error getting FHIR schema:', error);
        res.status(500).json({ success: false, error: error.message });
    }
};

/**
 * Build HL7 XPath tree from schema
 * Based on actual parsed HL7 structure (see parsedhl7.json and XPATH_AUTOCOMPLETE_IMPLEMENTATION.md)
 *
 * Parsed structure format:
 * {
 *   "enhancedSegments": {
 *     "MSH": {
 *       "key": "MSH",
 *       "name": "Message Header",
 *       "fields": [
 *         {
 *           "key": "MSH.3",
 *           "value": "...",
 *           "name": "Sending Application",
 *           "position": 3,
 *           "dataType": "HD",
 *           "subfields": [...]
 *         }
 *       ]
 *     }
 *   }
 * }
 */
function buildHL7XPathTree(schema) {
    const tree = {
        name: 'enhancedSegments',
        path: 'enhancedSegments',
        type: 'object',
        description: 'HL7 parsed message segments',
        children: [],
    };

    const structure = schema.structure || {};

    // Process each segment
    for (const [segmentName, segmentDef] of Object.entries(structure)) {
        const segmentNode = {
            name: segmentName,
            path: `enhancedSegments.${segmentName}`,
            type: 'segment',
            description: segmentDef.description,
            usage: segmentDef.usage,
            children: [],
        };

        // Add fields child
        const fieldsNode = {
            name: 'fields',
            path: `enhancedSegments.${segmentName}.fields`,
            type: 'array',
            description: 'Array of segment fields',
            children: [],
        };

        // Process each field
        const fields = segmentDef.fields || {};
        for (const [fieldKey, fieldDef] of Object.entries(fields)) {
            const fieldIndex = parseInt(fieldKey.split('.')[1]) - 1; // MSH.1 → index 0

            const fieldNode = {
                name: fieldKey,
                path: `enhancedSegments.${segmentName}.fields[${fieldIndex}]`,
                type: 'field',
                dataType: fieldDef.dataType,
                description: fieldDef.name || fieldDef.description,
                usage: fieldDef.usage,
                children: [
                    {
                        name: 'key',
                        path: `enhancedSegments.${segmentName}.fields[${fieldIndex}].key`,
                        type: 'string',
                        description: 'Field key (e.g., MSH.3)',
                    },
                    {
                        name: 'name',
                        path: `enhancedSegments.${segmentName}.fields[${fieldIndex}].name`,
                        type: 'string',
                        description: 'Field name',
                    },
                    {
                        name: 'value',
                        path: `enhancedSegments.${segmentName}.fields[${fieldIndex}].value`,
                        type: 'string',
                        description: 'Field value',
                    },
                ],
            };

            // Add components if they exist
            if (fieldDef.components) {
                const subfieldsNode = {
                    name: 'subfields',
                    path: `enhancedSegments.${segmentName}.fields[${fieldIndex}].subfields`,
                    type: 'array',
                    description: 'Field components/subfields',
                    children: [],
                };

                for (const [compKey, compDef] of Object.entries(fieldDef.components)) {
                    const compIndex = parseInt(compKey.split('.')[2]) - 1;
                    subfieldsNode.children.push({
                        name: compKey,
                        path: `enhancedSegments.${segmentName}.fields[${fieldIndex}].subfields[${compIndex}].value`,
                        type: 'string',
                        dataType: compDef.dataType,
                        description: compDef.name,
                        usage: compDef.usage,
                    });
                }

                fieldNode.children.push(subfieldsNode);
            }

            fieldsNode.children.push(fieldNode);
        }

        segmentNode.children.push(fieldsNode);
        tree.children.push(segmentNode);
    }

    return tree;
}

/**
 * Build FHIR XPath tree from schema
 *
 * ⚠️ TODO: This is a PLACEHOLDER implementation!
 *
 * CURRENT STATUS:
 * - Universal FHIR parsed structure NOT YET DEFINED in codebase
 * - Need to capture actual parsed FHIR output (similar to parsedhl7.json)
 * - Need to finalize FHIR transformation runtime structure
 *
 * IMPLEMENTATION STEPS (when FHIR transformations are ready):
 * 1. Create sample FHIR transformation in production
 * 2. Capture actual parsed FHIR JSON structure
 * 3. Save as parsedFHIR.json (like parsedhl7.json)
 * 4. Update this function to match actual parsed structure
 * 5. Test autocomplete with real FHIR field paths
 *
 * See XPATH_AUTOCOMPLETE_IMPLEMENTATION.md section "TODO: FHIR Support" for details
 */
function buildFHIRXPathTree(schema, resourceType) {
    const tree = {
        name: 'entry[0].resource',
        path: 'entry[0].resource',
        type: 'object',
        description: `FHIR ${resourceType} resource in Bundle`,
        children: [],
    };

    const elements = schema.elements || {};

    // Group elements by hierarchy
    const hierarchy = {};
    for (const [elementPath, elementDef] of Object.entries(elements)) {
        const parts = elementPath.split('.');
        const parentPath = parts.slice(0, -1).join('.');
        const elementName = parts[parts.length - 1];

        if (!hierarchy[parentPath]) {
            hierarchy[parentPath] = [];
        }

        hierarchy[parentPath].push({
            name: elementName,
            fullPath: elementPath,
            def: elementDef,
        });
    }

    // Build tree recursively
    function buildNode(basePath, relativePath = '') {
        const fullPath = relativePath ? `${basePath}.${relativePath}` : basePath;
        const children = hierarchy[fullPath] || [];

        return children.map(child => {
            const childPath = `entry[0].resource.${child.fullPath.replace(resourceType + '.', '')}`;

            const node = {
                name: child.name,
                path: childPath,
                type: child.def.dataType || 'element',
                description: child.def.description,
                cardinality: child.def.cardinality,
                required: child.def.required,
                children: [],
            };

            // Recursively add children
            const grandchildren = buildNode(basePath, child.fullPath.replace(basePath + '.', ''));
            node.children = grandchildren;

            return node;
        });
    }

    tree.children = buildNode(resourceType);

    return tree;
}

/**
 * Search XPath across schemas (autocomplete helper)
 * POST /api/schemas/search
 * Body: { format: 'hl7v2|fhir', version: '2.5|R4', query: 'patient' }
 */
exports.searchXPath = async (req, res) => {
    try {
        const { format, version, messageType, query } = req.body;

        let schema;
        if (format === 'hl7v2') {
            const schemaPath = path.join(__dirname, '..', 'schemas', 'hl7', version, `${messageType}.gz`);
            const compressed = await fs.readFile(schemaPath);
            const decompressed = await gunzip(compressed);
            const rawSchema = JSON.parse(decompressed.toString());
            schema = buildHL7XPathTree(rawSchema);
        } else if (format === 'fhir') {
            const schemaPath = path.join(__dirname, '..', 'schemas', 'fhir', version, 'resources', `${messageType}.gz`);
            const compressed = await fs.readFile(schemaPath);
            const decompressed = await gunzip(compressed);
            const rawSchema = JSON.parse(decompressed.toString());
            schema = buildFHIRXPathTree(rawSchema, messageType);
        }

        // Search schema tree for matching paths
        const results = searchTree(schema, query.toLowerCase());

        res.json({ success: true, results });
    } catch (error) {
        console.error('Error searching XPath:', error);
        res.status(500).json({ success: false, error: error.message });
    }
};

/**
 * Recursively search tree for matching nodes
 */
function searchTree(node, query) {
    const results = [];

    function search(n, depth = 0) {
        // Check if node matches query
        const nameMatch = n.name && n.name.toLowerCase().includes(query);
        const pathMatch = n.path && n.path.toLowerCase().includes(query);
        const descMatch = n.description && n.description.toLowerCase().includes(query);

        if (nameMatch || pathMatch || descMatch) {
            results.push({
                path: n.path,
                name: n.name,
                type: n.type,
                description: n.description,
                dataType: n.dataType,
                depth,
            });
        }

        // Search children
        if (n.children && n.children.length > 0) {
            for (const child of n.children) {
                search(child, depth + 1);
            }
        }
    }

    search(node);
    return results.slice(0, 50); // Limit to 50 results
}

/**
 * Upload/Insert a sample parsed message
 * POST /api/schemas/samples
 * Body: { messageType, hlVersion, parsedContent, description }
 */
exports.uploadSample = async (req, res) => {
    try {
        const { messageType, hlVersion, parsedContent, description } = req.body;

        if (!messageType || !hlVersion || !parsedContent) {
            return res.status(400).json({
                success: false,
                error: 'Missing required fields: messageType, hlVersion, parsedContent'
            });
        }

        const sampleId = await SampleMessageService.upsertSample({
            messageType,
            hlVersion,
            format: 'hl7v2',
            parsedContent,
            description
        });

        res.json({
            success: true,
            message: `Sample message saved: ${messageType} v${hlVersion}`,
            sampleId
        });
    } catch (error) {
        console.error('Error uploading sample:', error);
        res.status(500).json({ success: false, error: error.message });
    }
};

/**
 * List all available samples
 * GET /api/schemas/samples
 */
exports.listSamples = async (req, res) => {
    try {
        const samples = await SampleMessageService.listSamples();
        res.json({ success: true, samples });
    } catch (error) {
        console.error('Error listing samples:', error);
        res.status(500).json({ success: false, error: error.message });
    }
};

/**
 * Get universal HL7 field paths (aggregated from all samples)
 * GET /api/schemas/hl7/fields
 *
 * This is format-agnostic - returns ALL possible field paths
 * from all HL7 message samples in the database
 */
exports.getUniversalHL7Fields = async (req, res) => {
    try {
        console.log('📂 Loading universal HL7 field paths from all samples...');

        const universalTree = await SampleMessageService.buildUniversalFieldTree('hl7v2');

        if (!universalTree || universalTree.children.length === 0) {
            return res.status(404).json({
                success: false,
                error: 'No HL7 samples available. Please add samples via /api/schemas/samples endpoint.'
            });
        }

        console.log(`✅ Universal tree built: ${universalTree.children.length} segments`);

        res.json({ success: true, xpathTree: universalTree });
    } catch (error) {
        console.error('❌ Error building universal field tree:', error.message);
        res.status(500).json({ success: false, error: error.message });
    }
};

/**
 * Get universal FHIR field paths (aggregated from all samples)
 * GET /api/schemas/fhir/fields
 */
exports.getUniversalFHIRFields = async (req, res) => {
    try {
        console.log('📂 Loading universal FHIR field paths from all samples...');

        const universalTree = await SampleMessageService.buildUniversalFieldTree('fhir');

        if (!universalTree || universalTree.children.length === 0) {
            return res.status(404).json({
                success: false,
                error: 'No FHIR samples available. Please add samples via /api/schemas/samples endpoint.'
            });
        }

        console.log(`✅ Universal FHIR tree built: ${universalTree.children.length} resources`);

        res.json({ success: true, xpathTree: universalTree });
    } catch (error) {
        console.error('❌ Error building universal FHIR field tree:', error.message);
        res.status(500).json({ success: false, error: error.message });
    }
};

/**
 * Get universal CDA/CCD field paths (aggregated from all samples)
 * GET /api/schemas/cda/fields
 */
exports.getUniversalCDAFields = async (req, res) => {
    try {
        console.log('📂 Loading universal CDA/CCD field paths from all samples...');

        const universalTree = await SampleMessageService.buildUniversalFieldTree('cda');

        if (!universalTree || universalTree.children.length === 0) {
            return res.status(404).json({
                success: false,
                error: 'No CDA/CCD samples available. Please add samples via /api/schemas/samples endpoint.'
            });
        }

        console.log(`✅ Universal CDA tree built: ${universalTree.children.length} sections`);

        res.json({ success: true, xpathTree: universalTree });
    } catch (error) {
        console.error('❌ Error building universal CDA field tree:', error.message);
        res.status(500).json({ success: false, error: error.message });
    }
};

module.exports = exports;
