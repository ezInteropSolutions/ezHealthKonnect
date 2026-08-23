// tests/unit/services/HL7MessageAnalyzer.test.js
//
// Regex/string-parsing field extraction from raw HL7, used for smart-routing
// decisions. No external dependencies at all — `new HL7MessageAnalyzer()`
// works standalone with a literal HL7 string as input.
const HL7MessageAnalyzer = require('../../../services/HL7MessageAnalyzer');

const SAMPLE_ADT = [
    'MSH|^~\\&|SENDING_APP|MAIN_CAMPUS|RECEIVING_APP|RECEIVING_FAC|20260315143000||ADT^A01|MSG001|P|2.5',
    'EVN|A01|20260315143000',
    'PID|1||PATID12345^^^MRN^MR||Doe^John||19800101|M',
    'PV1|1|I|WARD1^ROOM1^BED1||||CARDIO_DR^Smith^Jane', // PV1.7 (attending) = CARDIO_DR...
].join('\r');

const SAMPLE_ORU_STAT = [
    'MSH|^~\\&|LAB_APP|CLINIC_LAB|RECEIVING_APP|RECEIVING_FAC|20260315143000||ORU^R01|MSG002|P|2.5',
    'PID|1||PATID99999||Roe^Jane',
    'OBX|1|NM|GLUCOSE||STAT critical value|mg/dL',
].join('\r');

describe('HL7MessageAnalyzer field extraction', () => {
    let analyzer;
    beforeEach(() => { analyzer = new HL7MessageAnalyzer(); });

    it('extracts message type and trigger event from MSH.9', () => {
        expect(analyzer.extractMessageType(SAMPLE_ADT)).toBe('ADT');
        expect(analyzer.extractTriggerEvent(SAMPLE_ADT)).toBe('A01');
    });

    it('extracts sending facility from MSH.4', () => {
        expect(analyzer.extractFacility(SAMPLE_ADT)).toBe('MAIN_CAMPUS');
    });

    it('extracts patient ID from PID.3', () => {
        expect(analyzer.extractPatientId(SAMPLE_ADT)).toBe('PATID12345');
    });

    it('extracts patient class from PV1.2', () => {
        expect(analyzer.extractPatientClass(SAMPLE_ADT)).toBe('I');
    });

    it('extracts department from PV1.3', () => {
        expect(analyzer.extractDepartment(SAMPLE_ADT)).toBe('WARD1');
    });

    it('returns UNKNOWN/null defaults gracefully when a segment is absent, never throws', () => {
        const noSegments = 'MSH|^~\\&|APP|FAC|||||ADT^A01|MSG|P|2.5';
        expect(analyzer.extractPatientClass(noSegments)).toBe('UNKNOWN'); // no PV1 at all
        expect(analyzer.extractPatientId(noSegments)).toBeNull(); // no PID at all
    });

    it('extracts and parses the MSH.7 timestamp', () => {
        const ts = analyzer.extractTimestamp(SAMPLE_ADT);
        expect(ts.getFullYear()).toBe(2026);
        expect(ts.getMonth()).toBe(2); // March, 0-based
        expect(ts.getDate()).toBe(15);
        expect(ts.getHours()).toBe(14);
    });
});

describe('HL7MessageAnalyzer.determineUrgency', () => {
    let analyzer;
    beforeEach(() => { analyzer = new HL7MessageAnalyzer(); });

    it('flags an Emergency-class patient as HIGH regardless of message type', () => {
        const emergencyMsg = SAMPLE_ADT.replace('|I|', '|E|');
        expect(analyzer.determineUrgency(emergencyMsg)).toBe('HIGH');
    });

    it('flags ADT admit/transfer/discharge (A01/A02/A03) as MEDIUM', () => {
        expect(analyzer.determineUrgency(SAMPLE_ADT)).toBe('MEDIUM'); // A01, patient class I (not E)
    });

    it('flags an ORU containing STAT/CRITICAL as HIGH', () => {
        expect(analyzer.determineUrgency(SAMPLE_ORU_STAT)).toBe('HIGH');
    });

    it('flags a routine ORU (no STAT/CRITICAL) as MEDIUM', () => {
        const routineOru = SAMPLE_ORU_STAT.replace('STAT critical value', 'normal value');
        expect(analyzer.determineUrgency(routineOru)).toBe('MEDIUM');
    });

    it('defaults to LOW for an unrecognized combination', () => {
        const other = 'MSH|^~\\&|APP|FAC|||||MFN^M02|MSG|P|2.5\rPV1|1|O|';
        expect(analyzer.determineUrgency(other)).toBe('LOW');
    });
});

describe('HL7MessageAnalyzer.extractSpecialty', () => {
    let analyzer;
    beforeEach(() => { analyzer = new HL7MessageAnalyzer(); });

    it('detects CARDIOLOGY from the attending physician field (PV1.7)', () => {
        expect(analyzer.extractSpecialty(SAMPLE_ADT)).toBe('CARDIOLOGY');
    });

    it('defaults to GENERAL when no specialty keyword matches', () => {
        const generic = SAMPLE_ADT.replace('CARDIO_DR^Smith^Jane', 'DR001^Smith^Jane');
        expect(analyzer.extractSpecialty(generic)).toBe('GENERAL');
    });
});

describe('HL7MessageAnalyzer.analyzeSegments / extractMetadata', () => {
    let analyzer;
    beforeEach(() => { analyzer = new HL7MessageAnalyzer(); });

    it('counts occurrences of each segment type', () => {
        const segments = analyzer.analyzeSegments(SAMPLE_ADT);
        expect(segments).toEqual({ MSH: 1, EVN: 1, PID: 1, PV1: 1 });
    });

    it('flags presence of clinically-relevant segments in metadata', () => {
        const meta = analyzer.extractMetadata(SAMPLE_ORU_STAT);
        expect(meta.hasObservations).toBe(true); // OBX| present
        expect(meta.hasAllergies).toBe(false); // no AL1|
        expect(meta.segmentCount).toBe(3);
    });
});

describe('HL7MessageAnalyzer.generateRoutingHints', () => {
    let analyzer;
    beforeEach(() => { analyzer = new HL7MessageAnalyzer(); });

    it('routes an emergency patient to emergency endpoints with a stated reason', () => {
        const emergencyMsg = SAMPLE_ADT.replace('|I|', '|E|');
        const hints = analyzer.generateRoutingHints(emergencyMsg);
        expect(hints.preferredEndpoints).toEqual(expect.arrayContaining(['emergency_fhir', 'emergency_dashboard']));
        expect(hints.routingReasons).toEqual(expect.arrayContaining(['Emergency patient detected']));
    });

    it('routes based on facility prefix (MAIN vs CLINIC)', () => {
        expect(analyzer.generateRoutingHints(SAMPLE_ADT).preferredEndpoints).toContain('main_campus_fhir');
        expect(analyzer.generateRoutingHints(SAMPLE_ORU_STAT).preferredEndpoints).toContain('clinic_fhir');
    });

    it('adds lab-results routing hints for ORU messages', () => {
        const hints = analyzer.generateRoutingHints(SAMPLE_ORU_STAT);
        expect(hints.preferredEndpoints).toEqual(expect.arrayContaining(['lab_results_fhir', 'analytics_warehouse']));
        expect(hints.transformationHints).toContain('create_diagnostic_report');
    });
});

describe('HL7MessageAnalyzer.analyzeMessage (full integration + caching)', () => {
    let analyzer;
    beforeEach(() => { analyzer = new HL7MessageAnalyzer(); });

    it('assembles a complete analysis object from all the individual extractors', () => {
        const result = analyzer.analyzeMessage(SAMPLE_ADT, 'msg-1');
        expect(result).toMatchObject({
            messageId: 'msg-1',
            messageType: 'ADT',
            triggerEvent: 'A01',
            facility: 'MAIN_CAMPUS',
            patientId: 'PATID12345',
            patientClass: 'I',
            department: 'WARD1',
            urgency: 'MEDIUM',
            specialty: 'CARDIOLOGY',
        });
    });

    it('caches by messageId — a second call with the same ID returns the identical cached object', () => {
        const first = analyzer.analyzeMessage(SAMPLE_ADT, 'msg-cache-1');
        const second = analyzer.analyzeMessage('completely different raw text, ignored on cache hit', 'msg-cache-1');
        expect(second).toBe(first); // same object reference — proves the cache short-circuited re-analysis
    });
});

describe('HL7MessageAnalyzer.extractSegment / extractField', () => {
    let analyzer;
    beforeEach(() => { analyzer = new HL7MessageAnalyzer(); });

    it('finds a segment by its 3-letter prefix', () => {
        expect(analyzer.extractSegment(SAMPLE_ADT, 'PID')).toContain('PATID12345');
    });

    it('returns undefined when the segment is not present', () => {
        expect(analyzer.extractSegment(SAMPLE_ADT, 'OBX')).toBeUndefined();
    });

    it('extractField returns the field at the given index, or null if out of range', () => {
        const pidSegment = analyzer.extractSegment(SAMPLE_ADT, 'PID');
        expect(analyzer.extractField(pidSegment, 3)).toBe('PATID12345^^^MRN^MR');
        expect(analyzer.extractField(pidSegment, 99)).toBeNull();
        expect(analyzer.extractField(null, 3)).toBeNull();
    });
});

describe('HL7MessageAnalyzer cache management', () => {
    it('clearCache empties both caches', () => {
        const analyzer = new HL7MessageAnalyzer();
        analyzer.analyzeMessage(SAMPLE_ADT, 'msg-1');
        expect(analyzer.getCacheStats().analysisCache).toBe(1);
        analyzer.clearCache();
        expect(analyzer.getCacheStats().analysisCache).toBe(0);
    });
});
