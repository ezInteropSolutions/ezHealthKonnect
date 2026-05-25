// public/js/hl7Service.js - Frontend HL7 API Service
// Handles all HL7-related API calls and data transformation

class HL7Service {
    constructor() {
        this.baseUrl = '/api/hl7';
        this.cache = new Map();
        this.stats = {
            requests: 0,
            errors: 0,
            cacheHits: 0
        };
    }

    /**
     * Parse single HL7 message
     * @param {string} hl7Content - Raw HL7 message content
     * @param {Object} options - Parsing options
     * @returns {Promise<Object>} Parsed HL7 data in wizard format
     */
    // In public/js/services/hl7Service.js
// Replace the parseMessage method in HL7Service class

async parseMessage(hl7Content, options = {}) {
    console.log('🧬 HL7Service: Parsing message...');
    this.stats.requests++;

    try {
        // Check cache first
        const cacheKey = this.generateCacheKey(hl7Content, options);
        if (this.cache.has(cacheKey) && !options.skipCache) {
            console.log('📋 Using cached result');
            this.stats.cacheHits++;
            return this.cache.get(cacheKey);
        }

        // FIX 1: Prepare request body with correct field names for Go backend
        const requestBody = {
            RawMessage: hl7Content.trim(),  // Changed from hl7Content to RawMessage
            UseEnhanced: options.validateStrict !== false,
            IncludeValidation: true,  // Changed from includeRaw
            Timestamp: new Date().toISOString()
        };

        // FIX 2: Use correct base URL construction
        const apiUrl = this.baseUrl.startsWith('http') 
            ? `${this.baseUrl}/parse` 
            : `http://localhost:8080${this.baseUrl}/parse`;

        // Call backend API
        const response = await fetch(apiUrl, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(requestBody)
        });

        const result = await response.json();

        if (!response.ok) {
            throw new Error(result.Error || `HTTP ${response.status}: ${response.statusText}`);
        }

        // FIX 3: Check Success field (capitalized from Go)
        if (!result.Success) {
            throw new Error(result.Error || 'Failed to parse HL7 message');
        }

        // FIX 4: Transform backend response to wizard format
        const wizardData = this.transformToWizardFormat(result.Data, result.Meta);

        // Cache the result
        this.cache.set(cacheKey, wizardData);
        if (this.cache.size > 100) { // Limit cache size
            const firstKey = this.cache.keys().next().value;
            this.cache.delete(firstKey);
        }

        console.log('✅ HL7 message parsed successfully');
        return wizardData;

    } catch (error) {
        console.error('❌ HL7 parsing failed:', error);
        this.stats.errors++;
        throw new Error(this.formatErrorMessage(error));
    }
}

// FIX 5: Update constructor to use correct base URL
constructor() {
    this.baseUrl = 'http://localhost:8080/api/hl7';  // Full URL instead of relative
    this.cache = new Map();
    this.stats = {
        requests: 0,
        errors: 0,
        cacheHits: 0
    };
}

    /**
     * Parse HL7 file
     * @param {File} file - File object from input
     * @param {Object} options - Parsing options
     * @returns {Promise<Object>} Parsed HL7 data
     */
    async parseFile(file, options = {}) {
        console.log('📁 HL7Service: Parsing file:', file.name);

        try {
            // Validate file
            this.validateFile(file);

            // Read file content
            const content = await this.readFileContent(file);

            // Parse the content
            const result = await this.parseMessage(content, options);

            // Add file metadata
            result.fileInfo = {
                name: file.name,
                size: file.size,
                type: file.type,
                lastModified: file.lastModified
            };

            return result;

        } catch (error) {
            console.error('❌ File parsing failed:', error);
            throw error;
        }
    }

    /**
     * Validate HL7 message structure (quick validation)
     * @param {string} hl7Content - Raw HL7 content
     * @returns {Promise<Object>} Validation result
     */
    async validateMessage(hl7Content) {
        console.log('🔍 HL7Service: Validating message structure...');

        try {
            const response = await fetch(`${this.baseUrl}/validate`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ hl7Content })
            });

            const result = await response.json();

            if (!response.ok) {
                throw new Error(result.error || 'Validation failed');
            }

            return result.validation;

        } catch (error) {
            console.error('❌ Validation failed:', error);
            throw error;
        }
    }

    /**
     * Get supported HL7 versions and message types
     * @returns {Promise<Object>} Supported configurations
     */
    async getSupportedFormats() {
        try {
            const response = await fetch(`${this.baseUrl}/supported`);
            const result = await response.json();
            return result.supported;
        } catch (error) {
            console.error('❌ Failed to get supported formats:', error);
            return {
                versions: ['2.5'],
                messageTypes: ['ADT^A01', 'ORU^R01'],
                customSegments: true
            };
        }
    }

    /**
     * Transform backend response to wizard-expected format
     * @param {Object} backendData - Response from backend parser
     * @param {Object} meta - Response metadata
     * @returns {Object} Wizard-compatible data structure
     */
    transformToWizardFormat(backendData, meta = {}) {
        console.log('🔄 Transforming backend data to wizard format');

        return {
            messageType: backendData.messageType || 'Unknown',
            version: backendData.version || '2.5',
            sendingApplication: backendData.sendingApplication || 'Unknown',
            sendingFacility: backendData.sendingFacility || 'Unknown',
            timestamp: backendData.timestamp,
            controlId: backendData.controlId,
            
            segments: (backendData.segments || []).map(seg => ({
                name: seg.name,
                fields: seg.fieldCount || seg.fields || 0,
                required: seg.required,
                repeating: seg.repeating,
                description: this.getSegmentDescription(seg.name),
                sequence: seg.sequence,
                isStandard: seg.isStandard,
                isCustom: seg.isCustom || seg.name.startsWith('Z'),
                validation: seg.validation || { valid: true, errors: [], warnings: [] }
            })),
            
            customSegments: (backendData.customSegments || []).map(seg => ({
                name: seg.name,
                description: seg.description || `Custom ${seg.name} segment`,
                fields: seg.fields,
                requiresMapping: seg.requiresMapping !== false
            })),
            
            validation: {
                valid: backendData.validation?.valid !== false,
                errors: backendData.validation?.errors || [],
                warnings: backendData.validation?.warnings || []
            },
            
            statistics: {
                totalSegments: backendData.statistics?.totalSegments || backendData.segments?.length || 0,
                validSegments: backendData.statistics?.validSegments || (backendData.segments || []).filter(s => !s.error).length,
                customSegments: backendData.customSegments?.length || 0,
                parseTime: meta.processingTime || backendData.performance?.parseTime || 0,
                source: 'backend',
                parser: backendData.parser || 'node-hl7-complete'
            },
            
            // Store original data for debugging
            _originalData: backendData,
            _meta: meta
        };
    }

    /**
     * Get human-readable segment descriptions
     */
    getSegmentDescription(segmentName) {
        const descriptions = {
            'MSH': 'Message Header',
            'EVN': 'Event Type',
            'PID': 'Patient Identification',
            'PV1': 'Patient Visit',
            'PV2': 'Patient Visit - Additional Info',
            'OBX': 'Observation Result',
            'OBR': 'Observation Request',
            'ORC': 'Common Order',
            'AL1': 'Allergy Information',
            'NK1': 'Next of Kin',
            'DG1': 'Diagnosis',
            'PR1': 'Procedures',
            'GT1': 'Guarantor',
            'IN1': 'Insurance',
            'SCH': 'Scheduling Activity Information',
            'RGS': 'Resource Group',
            'AIS': 'Appointment Information - Service',
            'AIL': 'Appointment Information - Location',
            'AIP': 'Appointment Information - Personnel',
            'TXA': 'Transcription Document Header',
            'NTE': 'Notes and Comments',
            'FT1': 'Financial Transaction',
            'DRG': 'Diagnosis Related Group',
            'UB1': 'Uniform Billing 1',
            'UB2': 'Uniform Billing 2'
        };
        
        if (segmentName.startsWith('Z')) {
            return `Custom ${segmentName} Segment`;
        }
        
        return descriptions[segmentName] || 'Standard HL7 Segment';
    }

    /**
     * Utility methods
     */
    validateFile(file) {
        // Size check (10MB limit)
        if (file.size > 10 * 1024 * 1024) {
            throw new Error('File size must be less than 10MB');
        }

        // Type check
        const allowedTypes = ['.hl7', '.txt', '.dat', '.msg'];
        const fileExtension = '.' + file.name.split('.').pop().toLowerCase();
        
        if (!allowedTypes.includes(fileExtension) && 
            !['text/plain', 'application/octet-stream'].includes(file.type)) {
            throw new Error('Please upload a valid HL7 file (.hl7, .txt, .dat, .msg)');
        }
    }

    readFileContent(file) {
        return new Promise((resolve, reject) => {
            const reader = new FileReader();
            reader.onload = e => resolve(e.target.result);
            reader.onerror = () => reject(new Error('Failed to read file'));
            reader.readAsText(file);
        });
    }

    generateCacheKey(content, options) {
        const data = content + JSON.stringify(options);
        let hash = 0;
        for (let i = 0; i < data.length; i++) {
            const char = data.charCodeAt(i);
            hash = ((hash << 5) - hash) + char;
            hash = hash & hash; // Convert to 32-bit integer
        }
        return hash.toString(36);
    }

    formatErrorMessage(error) {
        if (error.message.includes('fetch')) {
            return 'Could not connect to HL7 parsing service. Please check your connection and try again.';
        }
        if (error.message.includes('Invalid HL7')) {
            return 'The file does not contain valid HL7 format. Please check the file and try again.';
        }
        if (error.message.includes('timeout')) {
            return 'Parsing took too long. Please try with a smaller file.';
        }
        return error.message || 'An unknown error occurred during HL7 parsing.';
    }

    /**
     * Get service statistics
     */
    getStats() {
        return {
            ...this.stats,
            cacheSize: this.cache.size,
            successRate: this.stats.requests > 0 ? 
                ((this.stats.requests - this.stats.errors) / this.stats.requests * 100).toFixed(1) + '%' : 
                '0%'
        };
    }

    /**
     * Clear cache and reset stats
     */
    reset() {
        this.cache.clear();
        this.stats = { requests: 0, errors: 0, cacheHits: 0 };
    }
}

// Create global instance
window.HL7Service = new HL7Service();

// Export for potential module use
if (typeof module !== 'undefined' && module.exports) {
    module.exports = HL7Service;
}