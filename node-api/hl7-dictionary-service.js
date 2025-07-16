// smart-hybrid-hl7-dictionary-service.js - Smart hybrid approach
const express = require('express');
const cors = require('cors');

const app = express();
const PORT = 3001;

app.use(cors());
app.use(express.json());

// Manual definitions for CRITICAL fields that must be perfect
const criticalFieldDefinitions = {
    "MSH": {
        name: "Message Header",
        description: "Contains message control information",
        purpose: "Message header segment with control information",
        fields: [
            { name: "Field Separator", description: "Field separator character", dataType: "ST", optionality: "R", cardinality: "[1..1]", length: "1" },
            { name: "Encoding Characters", description: "Component separator, repetition separator, escape character, subcomponent separator", dataType: "ST", optionality: "R", cardinality: "[1..1]", length: "4" },
            { name: "Sending Application", description: "Application that sent the message", dataType: "HD", optionality: "O", cardinality: "[0..1]", length: "227" },
            { name: "Sending Facility", description: "Facility that sent the message", dataType: "HD", optionality: "O", cardinality: "[0..1]", length: "227" },
            { name: "Receiving Application", description: "Application that receives the message", dataType: "HD", optionality: "O", cardinality: "[0..1]", length: "227" },
            { name: "Receiving Facility", description: "Facility that receives the message", dataType: "HD", optionality: "O", cardinality: "[0..1]", length: "227" },
            { name: "Date/Time of Message", description: "Date and time message was created", dataType: "TS", optionality: "O", cardinality: "[0..1]", length: "26" },
            { name: "Security", description: "Security information", dataType: "ST", optionality: "O", cardinality: "[0..1]", length: "40" },
            { name: "Message Type", description: "Message type and trigger event", dataType: "MSG", optionality: "R", cardinality: "[1..1]", length: "15" },
            { name: "Message Control ID", description: "Unique message control identifier", dataType: "ST", optionality: "R", cardinality: "[1..1]", length: "20" },
            { name: "Processing ID", description: "Processing ID (P=Production, T=Test, D=Debug)", dataType: "PT", optionality: "R", cardinality: "[1..1]", length: "3" },
            { name: "Version ID", description: "HL7 version number", dataType: "VID", optionality: "R", cardinality: "[1..1]", length: "60" }
        ]
    },
    "PID": {
        name: "Patient Identification",
        description: "Patient demographic and identification information", 
        purpose: "Contains patient identification and demographic data",
        fields: [
            { name: "Set ID - PID", description: "Sequence number for patient identification", dataType: "SI", optionality: "O", cardinality: "[0..1]", length: "4" },
            { name: "Patient ID", description: "External patient identifier", dataType: "CX", optionality: "O", cardinality: "[0..1]", length: "20" },
            { name: "Patient Identifier List", description: "List of patient identifiers", dataType: "CX", optionality: "R", cardinality: "[1..*]", length: "250" },
            { name: "Alternate Patient ID - PID", description: "Alternate patient identifier", dataType: "CX", optionality: "O", cardinality: "[0..*]", length: "20" },
            { name: "Patient Name", description: "Legal name of the patient", dataType: "XPN", optionality: "R", cardinality: "[1..*]", length: "250" },
            { name: "Mother's Maiden Name", description: "Mother's maiden name", dataType: "XPN", optionality: "O", cardinality: "[0..*]", length: "250" },
            { name: "Date/Time of Birth", description: "Patient's date and time of birth", dataType: "TS", optionality: "O", cardinality: "[0..1]", length: "26" },
            { name: "Administrative Sex", description: "Patient's gender (M=Male, F=Female, O=Other, U=Unknown)", dataType: "IS", optionality: "O", cardinality: "[0..1]", length: "1", table: "0001" },
            { name: "Patient Alias", description: "Other names by which patient is known", dataType: "XPN", optionality: "O", cardinality: "[0..*]", length: "250" },
            { name: "Race", description: "Patient's race", dataType: "CE", optionality: "O", cardinality: "[0..*]", length: "250", table: "0005" },
            { name: "Patient Address", description: "Patient's primary address", dataType: "XAD", optionality: "O", cardinality: "[0..*]", length: "250" },
            { name: "County Code", description: "County code", dataType: "IS", optionality: "O", cardinality: "[0..1]", length: "4" },
            { name: "Phone Number - Home", description: "Patient's home phone number", dataType: "XTN", optionality: "O", cardinality: "[0..*]", length: "250" },
            { name: "Phone Number - Business", description: "Patient's business phone number", dataType: "XTN", optionality: "O", cardinality: "[0..*]", length: "250" },
            { name: "Primary Language", description: "Patient's primary language", dataType: "CE", optionality: "O", cardinality: "[0..1]", length: "250" },
            { name: "Marital Status", description: "Patient's marital status (S=Single, M=Married, D=Divorced, W=Widowed)", dataType: "CE", optionality: "O", cardinality: "[0..1]", length: "250", table: "0002" },
            { name: "Religion", description: "Patient's religion", dataType: "CE", optionality: "O", cardinality: "[0..1]", length: "250", table: "0006" },
            { name: "Patient Account Number", description: "Patient account number", dataType: "CX", optionality: "O", cardinality: "[0..1]", length: "250" },
            { name: "SSN Number - Patient", description: "Patient's social security number", dataType: "ST", optionality: "O", cardinality: "[0..1]", length: "16" },
            { name: "Driver's License Number - Patient", description: "Patient's driver's license number", dataType: "DLN", optionality: "O", cardinality: "[0..1]", length: "25" }
        ]
    },
    "PV1": {
        name: "Patient Visit",
        description: "Patient visit information",
        purpose: "Contains information about the patient's visit",
        fields: [
            { name: "Set ID - PV1", description: "Sequence number for visit", dataType: "SI", optionality: "O", cardinality: "[0..1]", length: "4" },
            { name: "Patient Class", description: "Patient class (I=Inpatient, O=Outpatient, E=Emergency, N=Not Applicable)", dataType: "IS", optionality: "R", cardinality: "[1..1]", length: "1", table: "0004" },
            { name: "Assigned Patient Location", description: "Patient's assigned location (room, bed, etc.)", dataType: "PL", optionality: "O", cardinality: "[0..1]", length: "80" },
            { name: "Admission Type", description: "Type of admission (R=Routine, U=Urgent, E=Elective)", dataType: "IS", optionality: "O", cardinality: "[0..1]", length: "2", table: "0007" },
            { name: "Preadmit Number", description: "Preadmission identifier", dataType: "CX", optionality: "O", cardinality: "[0..1]", length: "250" },
            { name: "Prior Patient Location", description: "Patient's previous location", dataType: "PL", optionality: "O", cardinality: "[0..1]", length: "80" },
            { name: "Attending Doctor", description: "Attending physician", dataType: "XCN", optionality: "O", cardinality: "[0..*]", length: "250" },
            { name: "Referring Doctor", description: "Referring physician", dataType: "XCN", optionality: "O", cardinality: "[0..*]", length: "250" },
            { name: "Consulting Doctor", description: "Consulting physician", dataType: "XCN", optionality: "O", cardinality: "[0..*]", length: "250" },
            { name: "Hospital Service", description: "Hospital service (MED=Medicine, SUR=Surgery, PED=Pediatrics)", dataType: "IS", optionality: "O", cardinality: "[0..1]", length: "3", table: "0069" }
        ]
    }
};

// Common message types
const messageTypeDefinitions = {
    "ADT_A01": { name: "Admit/Visit Notification", description: "Patient admission message", purpose: "Notifies systems of patient admission" },
    "ADT_A03": { name: "Discharge/End Visit", description: "Patient discharge message", purpose: "Notifies systems of patient discharge" },
    "ADT_A04": { name: "Register a Patient", description: "Patient registration message", purpose: "Registers a new patient in the system" },
    "ADT_A08": { name: "Update Patient Information", description: "Patient information update", purpose: "Updates existing patient information" },
    "ORM_O01": { name: "Order Message", description: "General order message", purpose: "Transmits orders to clinical systems" },
    "ORU_R01": { name: "Observation Report", description: "Lab results message", purpose: "Transmits lab results and observations" }
};

// Try to load hl7-dictionary for broader coverage
let hl7Dictionary = null;
try {
    hl7Dictionary = require('hl7-dictionary');
    console.log('✅ hl7-dictionary npm package loaded - will use for broader coverage');
} catch (error) {
    console.log('⚠️  hl7-dictionary npm package not available - using critical definitions only');
}

// Smart field info extraction
function getSmartFieldInfo(field, index, segmentName) {
    // Default fallback
    let fieldInfo = {
        name: `Field ${index + 1}`,
        description: 'Field description not available',
        dataType: 'Unknown',
        optionality: 'Unknown',
        cardinality: 'Unknown',
        length: 'Unknown'
    };

    // Try multiple approaches to get rich data
    if (field) {
        // Extract from hl7-dictionary (various possible structures)
        fieldInfo.name = field.name || field.desc || field.description || fieldInfo.name;
        fieldInfo.description = field.description || field.desc || field.longName || fieldInfo.description;
        fieldInfo.dataType = field.dataType || field.type || field.dt || fieldInfo.dataType;
        fieldInfo.optionality = field.optionality || field.opt || field.usage || fieldInfo.optionality;
        fieldInfo.cardinality = field.cardinality || field.card || field.rep || fieldInfo.cardinality;
        fieldInfo.length = field.length || field.len || field.maxLength || fieldInfo.length;
        fieldInfo.table = field.table || field.valueSet || null;
    }

    return fieldInfo;
}

// Enhanced processing with hybrid approach
app.post('/api/v1/hl7/enhance', (req, res) => {
    try {
        const { segments, messageType, version = '2.5' } = req.body;
        
        if (!segments) {
            return res.status(400).json({ error: 'segments are required' });
        }

        console.log(`📚 [Smart Hybrid] Processing ${Object.keys(segments).length} segments for ${messageType || 'unknown message'} (HL7 v${version})`);

        const enhancedSegments = {};
        
        // Process each segment
        for (const [segmentName, segmentData] of Object.entries(segments)) {
            try {
                // 1. Try critical (manual) definitions first for perfect quality
                let segmentDef = criticalFieldDefinitions[segmentName];
                let source = 'critical_manual_definitions';
                
                // 2. Fall back to hl7-dictionary for broader coverage
                if (!segmentDef && hl7Dictionary) {
                    const npmDef = hl7Dictionary.definitions[version]?.segments?.[segmentName];
                    if (npmDef) {
                        segmentDef = {
                            name: npmDef.name || segmentName,
                            description: npmDef.description || `${segmentName} segment`,
                            purpose: npmDef.purpose || 'HL7 standard segment',
                            fields: npmDef.fields || []
                        };
                        source = 'hl7_dictionary_npm_with_enhancements';
                    }
                }
                
                // 3. Create basic definition if nothing found
                if (!segmentDef) {
                    segmentDef = {
                        name: segmentName,
                        description: `${segmentName} segment`,
                        purpose: 'Segment definition not available',
                        fields: []
                    };
                    source = 'basic_fallback';
                }

                // Parse segment fields
                const fields = segmentData.split('|');
                const enhancedFields = {};
                
                // Process fields with smart extraction
                const maxFields = Math.max(fields.length - 1, segmentDef.fields.length, 20);
                
                for (let i = 0; i < maxFields; i++) {
                    const fieldValue = fields[i + 1] || '';
                    const fieldDef = segmentDef.fields[i];
                    const fieldInfo = source === 'critical_manual_definitions' 
                        ? segmentDef.fields[i] || {} 
                        : getSmartFieldInfo(fieldDef, i, segmentName);
                    
                    // Only add fields that have values or are part of the definition
                    if (fieldValue || fieldDef || i < 15) { // Limit to first 15 fields to avoid clutter
                        enhancedFields[`${segmentName}.${i + 1}`] = {
                            value: fieldValue,
                            name: fieldInfo.name || `Field ${i + 1}`,
                            description: fieldInfo.description || 'Field description not available',
                            dataType: fieldInfo.dataType || 'Unknown',
                            optionality: fieldInfo.optionality || 'Unknown',
                            cardinality: fieldInfo.cardinality || 'Unknown',
                            length: fieldInfo.length || 'Unknown',
                            table: fieldInfo.table || null,
                            position: i + 1,
                            hasValue: !!fieldValue
                        };
                    }
                }

                enhancedSegments[segmentName] = {
                    raw: segmentData,
                    name: segmentDef.name,
                    description: segmentDef.description,
                    purpose: segmentDef.purpose,
                    fields: enhancedFields,
                    fieldCount: Object.keys(enhancedFields).length,
                    dictionarySource: source
                };

                console.log(`✅ Enhanced ${segmentName} with ${Object.keys(enhancedFields).length} fields (source: ${source})`);

            } catch (segmentError) {
                console.error(`❌ Error processing segment ${segmentName}:`, segmentError);
                enhancedSegments[segmentName] = {
                    raw: segmentData,
                    error: `Error processing segment: ${segmentError.message}`,
                    fields: {}
                };
            }
        }

        // Get message type information
        let messageInfo = {};
        if (messageType) {
            const msgParts = messageType.split('^');
            const msgCode = msgParts[0];
            const eventCode = msgParts[1];
            const msgKey = `${msgCode}_${eventCode}`;
            
            const messageDef = messageTypeDefinitions[msgKey];
            if (messageDef) {
                messageInfo = {
                    code: msgCode,
                    event: eventCode,
                    name: messageDef.name,
                    description: messageDef.description,
                    purpose: messageDef.purpose
                };
            } else {
                messageInfo = {
                    code: msgCode,
                    event: eventCode,
                    name: messageType,
                    description: 'Message type not in critical definitions',
                    purpose: 'Standard HL7 message type'
                };
            }
        }

        const response = {
            success: true,
            version: version,
            messageType: messageInfo,
            enhancedSegments: enhancedSegments,
            summary: {
                totalSegments: Object.keys(enhancedSegments).length,
                enhancedSegments: Object.keys(enhancedSegments).filter(key => !enhancedSegments[key].error).length,
                errorSegments: Object.keys(enhancedSegments).filter(key => enhancedSegments[key].error).length,
                totalFields: Object.values(enhancedSegments).reduce((sum, seg) => sum + (seg.fieldCount || 0), 0),
                sources: {
                    critical_manual: Object.values(enhancedSegments).filter(seg => seg.dictionarySource === 'critical_manual_definitions').length,
                    npm_enhanced: Object.values(enhancedSegments).filter(seg => seg.dictionarySource === 'hl7_dictionary_npm_with_enhancements').length,
                    basic_fallback: Object.values(enhancedSegments).filter(seg => seg.dictionarySource === 'basic_fallback').length
                }
            },
            dictionaryStrategy: 'smart_hybrid_critical_first',
            timestamp: new Date().toISOString()
        };

        console.log(`✅ Smart hybrid processing: ${response.summary.sources.critical_manual} critical, ${response.summary.sources.npm_enhanced} npm-enhanced, ${response.summary.sources.basic_fallback} basic fallback`);
        res.json(response);

    } catch (error) {
        console.error('❌ Error in smart hybrid enhance endpoint:', error);
        res.status(500).json({
            error: 'Internal server error',
            message: error.message,
            timestamp: new Date().toISOString()
        });
    }
});

// Health check
app.get('/health', (req, res) => {
    res.json({
        status: 'healthy',
        service: 'smart-hybrid-hl7-dictionary-service',
        port: PORT,
        strategy: 'critical_manual_first_npm_fallback',
        coverage: {
            critical_segments: Object.keys(criticalFieldDefinitions).length,
            npm_available: !!hl7Dictionary,
            npm_versions: hl7Dictionary ? Object.keys(hl7Dictionary.definitions || {}) : []
        },
        timestamp: new Date().toISOString()
    });
});

// Test endpoint
app.get('/api/v1/hl7/test-enhanced', (req, res) => {
    const sampleSegments = {
        "MSH": "MSH|^~\\&|EPIC|EPIC|||20200301100000||ADT^A01|12345|P|2.5",
        "PID": "PID|1|123456789|||DOE^JOHN||19800115|M||W|123 MAIN ST^^ANYTOWN^ST^12345",
        "PV1": "PV1|1|I|2000^2012^01||||004777^ATTEND^ATTENDDOC||MED"
    };

    req.body = {
        segments: sampleSegments,
        messageType: "ADT^A01",
        version: "2.5"
    };

    const enhanceHandler = app._router.stack.find(layer => 
        layer.route?.path === '/api/v1/hl7/enhance' && 
        layer.route?.methods.post
    );
    
    if (enhanceHandler) {
        enhanceHandler.route.stack[0].handle(req, res);
    } else {
        res.status(500).json({ error: 'Test endpoint configuration error' });
    }
});

// Coverage info
app.get('/api/v1/hl7/coverage', (req, res) => {
    const coverage = {
        critical_segments: {
            count: Object.keys(criticalFieldDefinitions).length,
            segments: Object.keys(criticalFieldDefinitions),
            total_fields: Object.values(criticalFieldDefinitions).reduce((sum, seg) => sum + seg.fields.length, 0)
        },
        npm_coverage: null,
        strategy: "Focus on quality over quantity - perfect metadata for common segments, broader coverage via npm package"
    };

    if (hl7Dictionary) {
        const v25 = hl7Dictionary.definitions['2.5'];
        if (v25) {
            coverage.npm_coverage = {
                segments: Object.keys(v25.segments || {}).length,
                messages: Object.keys(v25.messages || {}).length,
                note: "Additional segments available via hl7-dictionary npm package"
            };
        }
    }

    res.json(coverage);
});

app.listen(PORT, () => {
    console.log(`\n📚 Smart Hybrid HL7 Dictionary Service Started`);
    console.log(`🌐 URL: http://localhost:${PORT}`);
    console.log(`🎯 Strategy: Critical manual definitions + npm fallback`);
    console.log(`✅ Perfect metadata for: ${Object.keys(criticalFieldDefinitions).join(', ')}`);
    console.log(`📖 Broader coverage via: ${hl7Dictionary ? 'hl7-dictionary npm package' : 'manual definitions only'}`);
    console.log(`🔗 Coverage info: http://localhost:${PORT}/api/v1/hl7/coverage`);
    console.log(`\n🎪 Best of both worlds: Quality + Coverage!`);
});