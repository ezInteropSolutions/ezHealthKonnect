// wizard-services.js - Core services working with actual ezHealthKonnect API response structure
// Includes ValidationService, NotificationService, and HL7Service

/**
 * ValidationService - Handles schema validation using actual API response structure
 */
class ValidationService {
    constructor() {
        this.validationRules = {
            // HL7 v2.x validation rules based on actual API structure
            hl7: {
                requiredSegments: {
                    'ADT^A01': ['MSH', 'EVN', 'PID', 'PV1'],
                    'ADT^A03': ['MSH', 'EVN', 'PID', 'PV1'],
                    'ADT^A04': ['MSH', 'EVN', 'PID'],
                    'ORU^R01': ['MSH', 'PID', 'OBR', 'OBX'],
                    'default': ['MSH']
                },
                segmentOrder: [
                    'MSH', 'EVN', 'PID', 'PD1', 'NK1', 'PV1', 'PV2', 
                    'ORC', 'OBR', 'OBX', 'NTE', 'AL1', 'DG1'
                ],
                requiredFields: {
                    'MSH': ['MSH.1', 'MSH.2', 'MSH.9', 'MSH.10', 'MSH.12'],
                    'PID': ['PID.3', 'PID.5'],
                    'PV1': ['PV1.2'],
                    'OBR': ['OBR.1', 'OBR.4'],
                    'OBX': ['OBX.1', 'OBX.2', 'OBX.3']
                }
            }
        };
    }

    /**
     * Validates API response data structure
     * Works with actual ezHealthKonnect API response format
     */
    validateApiResponse(apiResponse) {
        const validation = {
            isValid: true,
            errors: [],
            warnings: [],
            summary: {},
            hasApiErrors: false
        };

        if (!apiResponse) {
            validation.isValid = false;
            validation.errors.push('No API response received');
            return validation;
        }

        if (!apiResponse.success) {
            validation.isValid = false;
            validation.hasApiErrors = true;
            validation.errors.push(apiResponse.error || 'API parsing failed');
            return validation;
        }

        if (!apiResponse.data) {
            validation.isValid = false;
            validation.errors.push('No data in API response');
            return validation;
        }

        const data = apiResponse.data;

        // Use validation errors from API response if available
        if (data.validationErrors && Array.isArray(data.validationErrors)) {
            data.validationErrors.forEach(error => {
                if (error.severity === 'error') {
                    validation.errors.push(error.message);
                    validation.isValid = false;
                } else if (error.severity === 'warning') {
                    validation.warnings.push(error.message);
                }
            });
        }

        // Additional validation based on our rules
        if (data.enhancedSegments) {
            this.validateMessageStructure(data.messageType?.name, data.enhancedSegments, validation);
            this.validateSegments(data.enhancedSegments, data.messageType?.name, validation);
            this.validateSegmentOrder(data.enhancedSegments, validation);
        }
        
        // Generate validation summary
        validation.summary = this.generateValidationSummary(validation, data);
        
        return validation;
    }

    validateMessageStructure(messageType, segments, validation) {
        const requiredSegs = this.validationRules.hl7.requiredSegments[messageType] || 
                           this.validationRules.hl7.requiredSegments.default;

        requiredSegs.forEach(segName => {
            if (!segments[segName]) {
                validation.isValid = false;
                validation.errors.push(`Required segment ${segName} is missing for message type ${messageType}`);
            }
        });

        // Check for MSH segment specifically
        if (!segments.MSH) {
            validation.isValid = false;
            validation.errors.push('MSH (Message Header) segment is required for all HL7 messages');
        }
    }

    validateSegments(segments, messageType, validation) {
        Object.entries(segments).forEach(([segName, segment]) => {
            this.validateSegment(segName, segment, validation);
        });
    }

    validateSegment(segName, segment, validation) {
        // Check required fields for this segment using API response structure
        const requiredFields = this.validationRules.hl7.requiredFields[segName] || [];
        
        if (segment.fields) {
            requiredFields.forEach(fieldKey => {
                const field = segment.fields[fieldKey];
                if (!field || !field.hasValue || !field.value || field.value.trim() === '') {
                    validation.isValid = false;
                    validation.errors.push(`Required field ${fieldKey} is missing or empty in segment ${segName}`);
                }
            });

            // Validate individual fields using API field structure
            Object.entries(segment.fields).forEach(([fieldKey, field]) => {
                this.validateField(segName, fieldKey, field, validation);
            });
        }

        // Custom segment warnings
        if (segName.startsWith('Z')) {
            validation.warnings.push(`Custom segment ${segName} detected - ensure receiving system supports it`);
        }
    }

    validateField(segName, fieldKey, field, validation) {
        if (!field.hasValue || !field.value) return;

        const value = field.value.trim();
        const dataType = field.dataType?.toUpperCase();

        if (!dataType || dataType === 'UNKNOWN' || dataType === 'ST') return;

        switch (dataType) {
            case 'NM': // Numeric
                if (!/^[\d\.\-\+eE]+$/.test(value)) {
                    validation.warnings.push(`Field ${fieldKey} in ${segName} should be numeric, got: "${value}"`);
                }
                break;
                
            case 'DT': // Date (YYYYMMDD)
                if (!/^\d{8}$/.test(value.replace(/[-\/]/g, ''))) {
                    validation.warnings.push(`Field ${fieldKey} in ${segName} should be date format YYYYMMDD, got: "${value}"`);
                }
                break;
                
            case 'TM': // Time (HHMMSS)
                if (!/^\d{2,6}$/.test(value.replace(/[:]/g, ''))) {
                    validation.warnings.push(`Field ${fieldKey} in ${segName} should be time format HHMMSS, got: "${value}"`);
                }
                break;
                
            case 'TS': // Timestamp
                if (!/^\d{8,14}/.test(value.replace(/[-:\/\s]/g, ''))) {
                    validation.warnings.push(`Field ${fieldKey} in ${segName} should be timestamp format, got: "${value}"`);
                }
                break;
                
            case 'ID': // Coded value
                if (value.length === 0) {
                    validation.warnings.push(`Field ${fieldKey} in ${segName} is a coded field but appears empty`);
                }
                break;
        }

        // Length validation from API field metadata
        if (field.length && value.length > field.length) {
            validation.warnings.push(`Field ${fieldKey} in ${segName} exceeds maximum length of ${field.length} characters`);
        }

        // Required field validation using API optionality
        if (field.optionality === 'R' && (!value || value.length === 0)) {
            validation.isValid = false;
            validation.errors.push(`Required field ${fieldKey} in segment ${segName} is empty`);
        }
    }

    validateSegmentOrder(segments, validation) {
        const segmentNames = Object.keys(segments);
        const standardOrder = this.validationRules.hl7.segmentOrder;
        
        let lastStandardIndex = -1;
        
        segmentNames.forEach(segName => {
            const standardIndex = standardOrder.indexOf(segName);
            if (standardIndex !== -1) {
                if (standardIndex < lastStandardIndex) {
                    validation.warnings.push(`Segment ${segName} appears out of standard order`);
                }
                lastStandardIndex = Math.max(lastStandardIndex, standardIndex);
            }
        });
    }

    generateValidationSummary(validation, apiData) {
        const segments = apiData.enhancedSegments || {};
        const apiErrors = apiData.validationErrors || [];
        
        return {
            totalSegments: Object.keys(segments).length,
            errorCount: validation.errors.length,
            warningCount: validation.warnings.length,
            apiErrorCount: apiErrors.filter(err => err.severity === 'error').length,
            apiWarningCount: apiErrors.filter(err => err.severity === 'warning').length,
            isCompliant: validation.isValid && validation.warnings.length === 0,
            hasErrors: validation.errors.length > 0 || apiErrors.some(err => err.severity === 'error'),
            hasWarnings: validation.warnings.length > 0 || apiErrors.some(err => err.severity === 'warning'),
            customSegments: Object.keys(segments).filter(seg => seg.startsWith('Z')).length,
            dictionaryUsed: apiData.dictionaryUsed || false,
            messageType: apiData.messageType?.name || 'Unknown'
        };
    }

    generateValidationRules(apiData) {
        const rules = {
            messageType: apiData.messageType?.name,
            expectedSegments: Object.keys(apiData.enhancedSegments || {}),
            requiredFields: {},
            customValidation: [],
            validationErrors: apiData.validationErrors || []
        };

        // Generate required fields based on API field optionality
        Object.entries(apiData.enhancedSegments || {}).forEach(([segName, segment]) => {
            if (segment.fields) {
                const requiredFields = Object.entries(segment.fields)
                    .filter(([_, field]) => field.optionality === 'R')
                    .map(([fieldKey, _]) => fieldKey);
                
                if (requiredFields.length > 0) {
                    rules.requiredFields[segName] = requiredFields;
                }
            }
        });

        return rules;
    }
}

/**
 * NotificationService - Enhanced notification system with better styling
 */
class NotificationService {
    constructor() {
        this.notifications = [];
        this.maxNotifications = 5;
        this.defaultDuration = 5000;
        this.container = null;
    }

    show(message, type = 'info', duration = null) {
        const notification = {
            id: Date.now() + Math.random(),
            message,
            type,
            duration: duration || this.getDurationForType(type),
            timestamp: new Date()
        };

        this.notifications.push(notification);
        this.renderNotification(notification);
        this.scheduleRemoval(notification);

        // Remove oldest if we have too many
        if (this.notifications.length > this.maxNotifications) {
            const oldest = this.notifications.shift();
            this.removeNotification(oldest.id);
        }

        return notification.id;
    }

    getDurationForType(type) {
        switch (type) {
            case 'error': return 8000;
            case 'warning': return 6000;
            case 'success': return 4000;
            case 'info': return 5000;
            default: return this.defaultDuration;
        }
    }

    renderNotification(notification) {
        const container = this.getNotificationContainer();
        
        const notificationElement = document.createElement('div');
        notificationElement.className = `notification-toast ${notification.type}`;
        notificationElement.id = `notification-${notification.id}`;
        notificationElement.innerHTML = `
            <div class="notification-content">
                <span class="notification-icon">${this.getIconForType(notification.type)}</span>
                <span class="notification-message">${notification.message}</span>
                <button class="notification-close" onclick="window.wizard.notificationService.remove(${notification.id})">×</button>
            </div>
            <div class="notification-progress">
                <div class="notification-progress-bar" style="animation-duration: ${notification.duration}ms;"></div>
            </div>
        `;

        container.appendChild(notificationElement);
        
        // Trigger animation
        setTimeout(() => notificationElement.classList.add('show'), 100);
    }

    getNotificationContainer() {
        if (!this.container) {
            this.container = document.getElementById('notification-container');
            if (!this.container) {
                this.container = document.createElement('div');
                this.container.id = 'notification-container';
                this.container.className = 'notification-container';
                this.container.style.cssText = `
                    position: fixed;
                    top: 20px;
                    right: 20px;
                    z-index: 10000;
                    max-width: 400px;
                    pointer-events: none;
                `;
                document.body.appendChild(this.container);
            }
        }
        return this.container;
    }

    getIconForType(type) {
        switch (type) {
            case 'success': return '✅';
            case 'error': return '❌';
            case 'warning': return '⚠️';
            case 'info': return 'ℹ️';
            default: return '📢';
        }
    }

    scheduleRemoval(notification) {
        setTimeout(() => {
            this.remove(notification.id);
        }, notification.duration);
    }

    remove(notificationId) {
        const element = document.getElementById(`notification-${notificationId}`);
        if (element) {
            element.classList.remove('show');
            setTimeout(() => {
                if (element.parentNode) {
                    element.parentNode.removeChild(element);
                }
            }, 300);
        }

        this.notifications = this.notifications.filter(n => n.id !== notificationId);
    }

    removeAll() {
        this.notifications.forEach(notification => {
            this.remove(notification.id);
        });
        this.notifications = [];
    }

    // Convenience methods
    success(message, duration) {
        return this.show(message, 'success', duration);
    }

    error(message, duration) {
        return this.show(message, 'error', duration);
    }

    warning(message, duration) {
        return this.show(message, 'warning', duration);
    }

    info(message, duration) {
        return this.show(message, 'info', duration);
    }
}

/**
 * HL7Service - Enhanced service working with actual ezHealthKonnect API
 */
class HL7Service {
    constructor(apiBaseUrl) {
        this.API_BASE_URL = apiBaseUrl;
        this.cache = new Map();
        this.requestTimeout = 30000; // 30 seconds
    }

    async parseHL7Message(rawMessage, useEnhanced = true) {
        console.log('📤 Sending HL7 message to ezHealthKonnect API for parsing...');
        
        try {
            const requestData = {
                rawMessage,
                useEnhanced,
                includeValidation: true,
                timestamp: new Date().toISOString()
            };

            const response = await this.makeRequest('/hl7/parse', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(requestData)
            });

            if (!response.success) {
                throw new Error(response.error || 'Parsing failed');
            }

            console.log('✅ HL7 message parsed successfully by ezHealthKonnect API');
            return response;

        } catch (error) {
            console.error('❌ HL7 parsing failed:', error);
            throw error;
        }
    }

    async validateMessage(parsedData) {
        try {
            const response = await this.makeRequest('/hl7/validate', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(parsedData)
            });

            return response;
        } catch (error) {
            console.error('❌ HL7 validation failed:', error);
            throw error;
        }
    }

    async makeRequest(endpoint, options = {}) {
        const url = `${this.API_BASE_URL}${endpoint}`;
        
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), this.requestTimeout);

        try {
            const response = await fetch(url, {
                ...options,
                signal: controller.signal
            });

            clearTimeout(timeoutId);

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            clearTimeout(timeoutId);
            
            if (error.name === 'AbortError') {
                throw new Error('Request timed out');
            }
            
            throw error;
        }
    }

    /**
     * Generates mock schema structure for testing when API is not available
     * Mimics the structure of the actual API response
     */
    generateMockSchemaStructure(messageType) {
        console.log('🏗️ Generating mock schema structure for', messageType);
        
        const schemaDefinitions = {
            'ADT^A01': {
                description: 'Admit/visit notification',
                segments: [
                    { name: 'MSH', fields: 21, required: true, description: 'Message Header' },
                    { name: 'EVN', fields: 6, required: false, description: 'Event Type' },
                    { name: 'PID', fields: 30, required: true, description: 'Patient Identification' },
                    { name: 'NK1', fields: 35, required: false, description: 'Next of Kin', repeating: true },
                    { name: 'PV1', fields: 52, required: true, description: 'Patient Visit' },
                    { name: 'PV2', fields: 38, required: false, description: 'Patient Visit - Additional' },
                    { name: 'OBX', fields: 17, required: false, description: 'Observation/Result', repeating: true },
                    { name: 'AL1', fields: 6, required: false, description: 'Patient Allergy', repeating: true },
                    { name: 'DG1', fields: 20, required: false, description: 'Diagnosis', repeating: true }
                ]
            },
            'ADT^A04': {
                description: 'Register a patient',
                segments: [
                    { name: 'MSH', fields: 21, required: true, description: 'Message Header' },
                    { name: 'EVN', fields: 6, required: false, description: 'Event Type' },
                    { name: 'PID', fields: 30, required: true, description: 'Patient Identification' },
                    { name: 'NK1', fields: 35, required: false, description: 'Next of Kin', repeating: true },
                    { name: 'PV1', fields: 52, required: false, description: 'Patient Visit' },
                    { name: 'PV2', fields: 38, required: false, description: 'Patient Visit - Additional' }
                ]
            },
            'ORU^R01': {
                description: 'Observation result',
                segments: [
                    { name: 'MSH', fields: 21, required: true, description: 'Message Header' },
                    { name: 'PID', fields: 30, required: true, description: 'Patient Identification' },
                    { name: 'PV1', fields: 52, required: false, description: 'Patient Visit' },
                    { name: 'ORC', fields: 29, required: false, description: 'Common Order', repeating: true },
                    { name: 'OBR', fields: 47, required: true, description: 'Observation Request', repeating: true },
                    { name: 'OBX', fields: 17, required: false, description: 'Observation/Result', repeating: true },
                    { name: 'NTE', fields: 3, required: false, description: 'Notes and Comments', repeating: true }
                ]
            }
        };

        const schema = schemaDefinitions[messageType] || schemaDefinitions['ADT^A01'];
        
        return {
            success: true,
            data: {
                raw: `Generated schema for ${messageType}`,
                success: true,
                version: 'v2.5',
                messageType: {
                    code: messageType.split('^')[0],
                    event: messageType.split('^')[1],
                    name: messageType,
                    description: schema.description,
                    structure: 'ezHealthKonnect schema-based structure analysis'
                },
                enhancedSegments: this.convertSchemaToEnhancedSegments(schema.segments, messageType),
                parsedAt: new Date().toISOString(),
                dictionaryUsed: false,
                validationErrors: []
            },
            meta: {
                parsingTime: 0,
                dictionaryUsed: false,
                validationLevel: 'schema',
                parserVersion: '1.0.0'
            }
        };
    }

    convertSchemaToEnhancedSegments(segments, messageType) {
        const enhanced = {};
        
        segments.forEach(seg => {
            const fields = {};
            
            // Generate field definitions based on common HL7 patterns
            const fieldDefinitions = this.getFieldDefinitionsForSegment(seg.name);
            
            for (let i = 1; i <= Math.min(seg.fields, 15); i++) {
                const fieldDef = fieldDefinitions[i] || { 
                    name: `Field ${i}`, 
                    dataType: 'ST', 
                    optionality: 'O',
                    description: `${seg.name} field ${i} - From HL7 specification`
                };
                
                fields[`${seg.name}.${i}`] = {
                    value: '',
                    name: fieldDef.name,
                    description: fieldDef.description,
                    dataType: fieldDef.dataType,
                    optionality: fieldDef.optionality,
                    cardinality: seg.repeating ? '[0..*]' : '[0..1]',
                    position: i,
                    hasValue: false,
                    length: fieldDef.length
                };
            }
            
            enhanced[seg.name] = {
                raw: `${seg.name}|...`,
                name: seg.description,
                description: seg.description,
                purpose: `${seg.description} (from HL7 v2.5 specification)`,
                fields: fields,
                fieldCount: Object.keys(fields).length,
                dictionarySource: 'hl7_specification',
                required: seg.required,
                repeating: seg.repeating || false
            };
        });
        
        return enhanced;
    }

    getFieldDefinitionsForSegment(segmentName) {
        const definitions = {
            'MSH': {
                1: { name: 'Field Separator', dataType: 'ST', optionality: 'R', length: 1, description: 'Field separator character' },
                2: { name: 'Encoding Characters', dataType: 'ST', optionality: 'R', length: 4, description: 'Component and escape characters' },
                3: { name: 'Sending Application', dataType: 'HD', optionality: 'O', length: 227, description: 'Application sending this message' },
                4: { name: 'Sending Facility', dataType: 'HD', optionality: 'O', length: 227, description: 'Facility sending this message' },
                5: { name: 'Receiving Application', dataType: 'HD', optionality: 'O', length: 227, description: 'Application receiving this message' },
                6: { name: 'Receiving Facility', dataType: 'HD', optionality: 'O', length: 227, description: 'Facility receiving this message' },
                7: { name: 'Date/Time Of Message', dataType: 'TS', optionality: 'O', description: 'Timestamp when message was created' },
                8: { name: 'Security', dataType: 'ST', optionality: 'O', description: 'Security information' },
                9: { name: 'Message Type', dataType: 'MSG', optionality: 'R', length: 15, description: 'Message type and trigger event' },
                10: { name: 'Message Control ID', dataType: 'ST', optionality: 'R', length: 20, description: 'Unique message identifier' },
                11: { name: 'Processing ID', dataType: 'PT', optionality: 'R', description: 'Processing mode (P/T/D)' },
                12: { name: 'Version ID', dataType: 'VID', optionality: 'R', description: 'HL7 version number' }
            },
            'PID': {
                1: { name: 'Set ID - PID', dataType: 'SI', optionality: 'O', length: 4, description: 'Sequence number for PID segment' },
                2: { name: 'Patient ID', dataType: 'CX', optionality: 'B', description: 'External patient identifier (deprecated)' },
                3: { name: 'Patient Identifier List', dataType: 'CX', optionality: 'R', description: 'Primary patient identifiers including MRN' },
                4: { name: 'Alternate Patient ID - PID', dataType: 'CX', optionality: 'B', description: 'Alternate patient identifier (deprecated)' },
                5: { name: 'Patient Name', dataType: 'XPN', optionality: 'R', description: 'Patient full legal name' },
                6: { name: 'Mother\'s Maiden Name', dataType: 'XPN', optionality: 'O', description: 'Mother\'s maiden name' },
                7: { name: 'Date/Time of Birth', dataType: 'TS', optionality: 'O', description: 'Patient birth date and time' },
                8: { name: 'Administrative Sex', dataType: 'IS', optionality: 'O', description: 'Patient administrative gender' },
                9: { name: 'Patient Alias', dataType: 'XPN', optionality: 'B', description: 'Patient alias (deprecated)' },
                10: { name: 'Race', dataType: 'CE', optionality: 'O', description: 'Patient race' },
                11: { name: 'Patient Address', dataType: 'XAD', optionality: 'O', description: 'Patient home address' },
                12: { name: 'County Code', dataType: 'IS', optionality: 'B', description: 'County code (deprecated)' },
                13: { name: 'Phone Number - Home', dataType: 'XTN', optionality: 'O', description: 'Patient home phone number' },
                14: { name: 'Phone Number - Business', dataType: 'XTN', optionality: 'O', description: 'Patient business phone' },
                15: { name: 'Primary Language', dataType: 'CE', optionality: 'O', description: 'Patient primary language' }
            },
            'PV1': {
                1: { name: 'Set ID - PV1', dataType: 'SI', optionality: 'O', length: 4, description: 'Sequence number for PV1 segment' },
                2: { name: 'Patient Class', dataType: 'IS', optionality: 'R', description: 'Patient class (I/O/E/P/R/B/N)' },
                3: { name: 'Assigned Patient Location', dataType: 'PL', optionality: 'O', description: 'Patient room and bed assignment' },
                4: { name: 'Admission Type', dataType: 'IS', optionality: 'O', description: 'Type of admission' },
                5: { name: 'Preadmit Number', dataType: 'CX', optionality: 'O', description: 'Pre-admission identifier' },
                6: { name: 'Prior Patient Location', dataType: 'PL', optionality: 'O', description: 'Previous patient location' },
                7: { name: 'Attending Doctor', dataType: 'XCN', optionality: 'O', description: 'Primary attending physician' },
                8: { name: 'Referring Doctor', dataType: 'XCN', optionality: 'O', description: 'Referring physician' },
                9: { name: 'Consulting Doctor', dataType: 'XCN', optionality: 'O', description: 'Consulting physicians' },
                10: { name: 'Hospital Service', dataType: 'IS', optionality: 'O', description: 'Hospital service' }
            },
            'OBX': {
                1: { name: 'Set ID - OBX', dataType: 'SI', optionality: 'O', description: 'Sequence number for OBX segment' },
                2: { name: 'Value Type', dataType: 'ID', optionality: 'C', description: 'Data type of observation value' },
                3: { name: 'Observation Identifier', dataType: 'CE', optionality: 'R', description: 'Observation/test identifier' },
                4: { name: 'Observation Sub-ID', dataType: 'ST', optionality: 'O', description: 'Observation sub-identifier' },
                5: { name: 'Observation Value', dataType: 'Varies', optionality: 'C', description: 'Observation result value' },
                6: { name: 'Units', dataType: 'CE', optionality: 'O', description: 'Units of measure' },
                7: { name: 'References Range', dataType: 'ST', optionality: 'O', description: 'Reference range for result' },
                8: { name: 'Abnormal Flags', dataType: 'IS', optionality: 'O', description: 'Abnormal result flags' },
                9: { name: 'Probability', dataType: 'NM', optionality: 'O', description: 'Probability of abnormality' },
                10: { name: 'Nature of Abnormal Test', dataType: 'ID', optionality: 'O', description: 'Nature of abnormal test' }
            }
        };
        
        return definitions[segmentName] || {};
    }
}

// Export for module systems
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { ValidationService, NotificationService, HL7Service };
}