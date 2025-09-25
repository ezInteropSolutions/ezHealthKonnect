// services/HL7MessageAnalyzer.js
// Intelligent HL7 Message Analyzer for Smart Routing Decisions

class HL7MessageAnalyzer {
    constructor() {
        this.fieldCache = new Map();
        this.analysisCache = new Map();

        // Common HL7 field patterns
        this.fieldPatterns = {
            messageType: /MSH\|[^|]*\|[^|]*\|[^|]*\|[^|]*\|[^|]*\|[^|]*\|[^|]*\|([^|^]+)/,
            triggerEvent: /MSH\|[^|]*\|[^|]*\|[^|]*\|[^|]*\|[^|]*\|[^|]*\|[^|]*\|[^|^]*\^([^|^]+)/,
            facility: /MSH\|[^|]*\|[^|]*\|([^|]*)\|/,
            patientId: /PID\|[^|]*\|[^|]*\|([^|^]*)/,
            patientClass: /PV1\|[^|]*\|([^|]*)\|/,
            department: /PV1\|[^|]*\|[^|]*\|([^|^]*)/
        };
    }

    /**
     * Analyze HL7 message for routing decisions
     */
    analyzeMessage(hl7Message, messageId = null) {
        const cacheKey = messageId || this.generateMessageHash(hl7Message);

        // Check cache first
        if (this.analysisCache.has(cacheKey)) {
            return this.analysisCache.get(cacheKey);
        }

        const analysis = {
            messageId: messageId || cacheKey,
            messageType: this.extractMessageType(hl7Message),
            triggerEvent: this.extractTriggerEvent(hl7Message),
            facility: this.extractFacility(hl7Message),
            patientId: this.extractPatientId(hl7Message),
            patientClass: this.extractPatientClass(hl7Message),
            department: this.extractDepartment(hl7Message),
            urgency: this.determineUrgency(hl7Message),
            specialty: this.extractSpecialty(hl7Message),
            messageSize: hl7Message.length,
            timestamp: this.extractTimestamp(hl7Message),
            segments: this.analyzeSegments(hl7Message),
            metadata: this.extractMetadata(hl7Message),
            routingHints: this.generateRoutingHints(hl7Message)
        };

        // Cache the analysis
        this.analysisCache.set(cacheKey, analysis);

        // Cleanup cache if it gets too large
        if (this.analysisCache.size > 1000) {
            const firstKey = this.analysisCache.keys().next().value;
            this.analysisCache.delete(firstKey);
        }

        return analysis;
    }

    /**
     * Extract message type (ADT, ORU, ORM, etc.)
     */
    extractMessageType(hl7Message) {
        try {
            const mshSegment = this.extractSegment(hl7Message, 'MSH');
            if (!mshSegment) return 'UNKNOWN';

            const fields = mshSegment.split('|');
            if (fields.length >= 9) {
                const messageTypeField = fields[8]; // MSH.9
                const parts = messageTypeField.split('^');
                return parts[0] || 'UNKNOWN';
            }
            return 'UNKNOWN';
        } catch (error) {
            console.warn('Error extracting message type:', error.message);
            return 'UNKNOWN';
        }
    }

    /**
     * Extract trigger event (A01, A02, R01, etc.)
     */
    extractTriggerEvent(hl7Message) {
        try {
            const mshSegment = this.extractSegment(hl7Message, 'MSH');
            if (!mshSegment) return 'UNKNOWN';

            const fields = mshSegment.split('|');
            if (fields.length >= 9) {
                const messageTypeField = fields[8]; // MSH.9
                const parts = messageTypeField.split('^');
                return parts[1] || 'UNKNOWN';
            }
            return 'UNKNOWN';
        } catch (error) {
            console.warn('Error extracting trigger event:', error.message);
            return 'UNKNOWN';
        }
    }

    /**
     * Extract sending facility
     */
    extractFacility(hl7Message) {
        try {
            const mshSegment = this.extractSegment(hl7Message, 'MSH');
            if (!mshSegment) return 'UNKNOWN';

            const fields = mshSegment.split('|');
            return fields[3] || 'UNKNOWN'; // MSH.4 - Sending Facility
        } catch (error) {
            console.warn('Error extracting facility:', error.message);
            return 'UNKNOWN';
        }
    }

    /**
     * Extract patient ID
     */
    extractPatientId(hl7Message) {
        try {
            const pidSegment = this.extractSegment(hl7Message, 'PID');
            if (!pidSegment) return null;

            const fields = pidSegment.split('|');
            if (fields.length >= 4) {
                const patientIdField = fields[3]; // PID.3
                const parts = patientIdField.split('^');
                return parts[0] || null;
            }
            return null;
        } catch (error) {
            console.warn('Error extracting patient ID:', error.message);
            return null;
        }
    }

    /**
     * Extract patient class (E=Emergency, I=Inpatient, O=Outpatient)
     */
    extractPatientClass(hl7Message) {
        try {
            const pv1Segment = this.extractSegment(hl7Message, 'PV1');
            if (!pv1Segment) return 'UNKNOWN';

            const fields = pv1Segment.split('|');
            return fields[2] || 'UNKNOWN'; // PV1.2 - Patient Class
        } catch (error) {
            console.warn('Error extracting patient class:', error.message);
            return 'UNKNOWN';
        }
    }

    /**
     * Extract department/location
     */
    extractDepartment(hl7Message) {
        try {
            const pv1Segment = this.extractSegment(hl7Message, 'PV1');
            if (!pv1Segment) return 'UNKNOWN';

            const fields = pv1Segment.split('|');
            if (fields.length >= 4) {
                const locationField = fields[3]; // PV1.3
                const parts = locationField.split('^');
                return parts[0] || 'UNKNOWN';
            }
            return 'UNKNOWN';
        } catch (error) {
            console.warn('Error extracting department:', error.message);
            return 'UNKNOWN';
        }
    }

    /**
     * Determine message urgency based on various factors
     */
    determineUrgency(hl7Message) {
        const patientClass = this.extractPatientClass(hl7Message);
        const messageType = this.extractMessageType(hl7Message);
        const triggerEvent = this.extractTriggerEvent(hl7Message);

        // Emergency patients = high urgency
        if (patientClass === 'E') return 'HIGH';

        // Certain message types are time-sensitive
        if (messageType === 'ADT' && ['A01', 'A02', 'A03'].includes(triggerEvent)) {
            return 'MEDIUM';
        }

        // Lab results can be urgent
        if (messageType === 'ORU') {
            // Check for critical values or stat orders
            if (hl7Message.includes('STAT') || hl7Message.includes('CRITICAL')) {
                return 'HIGH';
            }
            return 'MEDIUM';
        }

        return 'LOW';
    }

    /**
     * Extract medical specialty from message
     */
    extractSpecialty(hl7Message) {
        try {
            const pv1Segment = this.extractSegment(hl7Message, 'PV1');
            if (!pv1Segment) return 'GENERAL';

            // Look for attending physician specialty
            const fields = pv1Segment.split('|');
            if (fields.length >= 8) {
                const attendingField = fields[7]; // PV1.7
                if (attendingField.includes('CARDIO')) return 'CARDIOLOGY';
                if (attendingField.includes('NEURO')) return 'NEUROLOGY';
                if (attendingField.includes('ONCO')) return 'ONCOLOGY';
                if (attendingField.includes('PEDS')) return 'PEDIATRICS';
                if (attendingField.includes('EMERG')) return 'EMERGENCY';
            }

            return 'GENERAL';
        } catch (error) {
            console.warn('Error extracting specialty:', error.message);
            return 'GENERAL';
        }
    }

    /**
     * Extract timestamp from message
     */
    extractTimestamp(hl7Message) {
        try {
            const mshSegment = this.extractSegment(hl7Message, 'MSH');
            if (!mshSegment) return new Date();

            const fields = mshSegment.split('|');
            if (fields.length >= 7) {
                const timestampField = fields[6]; // MSH.7
                if (timestampField) {
                    // Parse HL7 timestamp format (YYYYMMDDHHMMSS)
                    const year = timestampField.substr(0, 4);
                    const month = timestampField.substr(4, 2) - 1; // JS months are 0-based
                    const day = timestampField.substr(6, 2);
                    const hour = timestampField.substr(8, 2) || 0;
                    const minute = timestampField.substr(10, 2) || 0;
                    const second = timestampField.substr(12, 2) || 0;

                    return new Date(year, month, day, hour, minute, second);
                }
            }
            return new Date();
        } catch (error) {
            console.warn('Error extracting timestamp:', error.message);
            return new Date();
        }
    }

    /**
     * Analyze all segments in the message
     */
    analyzeSegments(hl7Message) {
        const segments = hl7Message.split(/\r\n|\r|\n/);
        const segmentAnalysis = {};

        segments.forEach(segment => {
            if (segment.length >= 3) {
                const segmentType = segment.substr(0, 3);
                if (!segmentAnalysis[segmentType]) {
                    segmentAnalysis[segmentType] = 0;
                }
                segmentAnalysis[segmentType]++;
            }
        });

        return segmentAnalysis;
    }

    /**
     * Extract additional metadata
     */
    extractMetadata(hl7Message) {
        return {
            hasObservations: hl7Message.includes('OBX|'),
            hasAllergies: hl7Message.includes('AL1|'),
            hasDiagnosis: hl7Message.includes('DG1|'),
            hasInsurance: hl7Message.includes('IN1|'),
            hasGuarantor: hl7Message.includes('GT1|'),
            segmentCount: hl7Message.split(/\r\n|\r|\n/).length,
            messageLength: hl7Message.length
        };
    }

    /**
     * Generate routing hints based on message analysis
     */
    generateRoutingHints(hl7Message) {
        const hints = {
            preferredEndpoints: [],
            excludedEndpoints: [],
            routingReasons: [],
            transformationHints: []
        };

        const patientClass = this.extractPatientClass(hl7Message);
        const messageType = this.extractMessageType(hl7Message);
        const facility = this.extractFacility(hl7Message);

        // Emergency routing hints
        if (patientClass === 'E') {
            hints.preferredEndpoints.push('emergency_fhir', 'emergency_dashboard');
            hints.routingReasons.push('Emergency patient detected');
            hints.transformationHints.push('include_emergency_flags');
        }

        // Facility-based routing
        if (facility.startsWith('MAIN')) {
            hints.preferredEndpoints.push('main_campus_fhir');
            hints.routingReasons.push('Main campus facility');
        } else if (facility.startsWith('CLINIC')) {
            hints.preferredEndpoints.push('clinic_fhir');
            hints.routingReasons.push('Clinic facility');
        }

        // Message type specific routing
        if (messageType === 'ORU') {
            hints.preferredEndpoints.push('lab_results_fhir', 'analytics_warehouse');
            hints.routingReasons.push('Lab results message');
            hints.transformationHints.push('create_diagnostic_report');
        }

        return hints;
    }

    /**
     * Extract specific segment from HL7 message
     */
    extractSegment(hl7Message, segmentName) {
        const segments = hl7Message.split(/\r\n|\r|\n/);
        return segments.find(segment => segment.startsWith(segmentName + '|'));
    }

    /**
     * Extract specific field from HL7 segment
     */
    extractField(segment, fieldIndex) {
        if (!segment) return null;
        const fields = segment.split('|');
        return fields[fieldIndex] || null;
    }

    /**
     * Generate hash for message caching
     */
    generateMessageHash(hl7Message) {
        // Simple hash function for caching
        let hash = 0;
        for (let i = 0; i < hl7Message.length; i++) {
            const char = hl7Message.charCodeAt(i);
            hash = ((hash << 5) - hash) + char;
            hash = hash & hash; // Convert to 32-bit integer
        }
        return `msg_${Math.abs(hash)}_${Date.now()}`;
    }

    /**
     * Clear analysis cache
     */
    clearCache() {
        this.analysisCache.clear();
        this.fieldCache.clear();
    }

    /**
     * Get cache statistics
     */
    getCacheStats() {
        return {
            analysisCache: this.analysisCache.size,
            fieldCache: this.fieldCache.size,
            totalMemory: this.analysisCache.size + this.fieldCache.size
        };
    }
}

module.exports = HL7MessageAnalyzer;