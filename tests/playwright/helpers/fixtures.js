'use strict';
/**
 * Shared test fixtures — HL7 messages and interface payloads used across the MVP suite.
 */

// ── HL7 Messages ─────────────────────────────────────────────────────────────

const HL7_ADT_A01 = [
    'MSH|^~\\&|MVP_TEST|MVP_FAC|EHK|EHK|20260607120000||ADT^A01|MVPTEST001|P|2.5',
    'EVN|A01|20260607120000',
    'PID|1||MVPTEST001^^^MVP^MR||SMOKE^PATIENT^MVP||19800515|M|||1 TEST ST^^TESTVILLE^IL^60601^USA',
    'PV1|1|I|3EAST^301^1^MVP||||DOC123^SMOKE^DOCTOR^^^DR|||SUR||||ADM|A0',
].join('\r');

const HL7_ADT_A03 = [
    'MSH|^~\\&|MVP_TEST|MVP_FAC|EHK|EHK|20260607130000||ADT^A03|MVPTEST002|P|2.5',
    'EVN|A03|20260607130000',
    'PID|1||MVPTEST001^^^MVP^MR||SMOKE^PATIENT^MVP||19800515|M',
    'PV1|1|O|3EAST^301^1^MVP||||DOC123^SMOKE^DOCTOR^^^DR|||SUR||||DIS|A0',
].join('\r');

const HL7_ORU_R01 = [
    'MSH|^~\\&|MVP_LAB|MVP_FAC|EHK|EHK|20260607140000||ORU^R01|MVPTEST003|P|2.5',
    'PID|1||MVPTEST002^^^LAB^MR||SMOKE^PATIENT^ORU||19750320|F',
    'OBR|1|ORD001|FIL001|CBC^Complete Blood Count^LN|||20260607',
    'OBX|1|NM|718-7^Hemoglobin^LN||13.5|g/dL|12.0-16.0||||F',
    'OBX|2|NM|787-2^MCV^LN||88.0|fL|80.0-100.0||||F',
].join('\r');

// ── Interface payloads (sent to /api/wizard/complete) ─────────────────────────

function adtToFhirPayload(port) {
    return {
        name: `MVP_SMOKE_ADT_${port}`,
        description: 'MVP smoke test — ADT to FHIR',
        sourceType: 'hl7v2',
        sourceConnectivity: 'tcp_mllp_inbound',
        sourceConfig: { host: '0.0.0.0', port },
        sourceConnectorConfig: {
            connectorType: 'tcp_mllp_inbound',
            config: { host: '0.0.0.0', port },
        },
        targetType: 'fhir',
        targetConnectivity: 'http_fhir_outbound',
        targetConfig: { baseUrl: 'http://hapi.fhir.org/baseR4', timeout: 30 },
        targetConnectorConfig: {
            connectorType: 'http_fhir_outbound',
            config: { baseUrl: 'http://hapi.fhir.org/baseR4', timeout: 30 },
        },
        messageType: 'ADT^A01',
        mappings: [],
        transformationFlow: 'hl7_to_fhir',
        auto_start: true,
        deployment_mode: 'manual',
        status: 'active',
        debug_logging: true,
        log_retention_days: 7,
    };
}

function oruToFhirPayload(port) {
    return {
        name: `MVP_SMOKE_ORU_${port}`,
        description: 'MVP smoke test — ORU to FHIR',
        sourceType: 'hl7v2',
        sourceConnectivity: 'tcp_mllp_inbound',
        sourceConfig: { host: '0.0.0.0', port },
        sourceConnectorConfig: {
            connectorType: 'tcp_mllp_inbound',
            config: { host: '0.0.0.0', port },
        },
        targetType: 'fhir',
        targetConnectivity: 'http_fhir_outbound',
        targetConfig: { baseUrl: 'http://hapi.fhir.org/baseR4', timeout: 30 },
        targetConnectorConfig: {
            connectorType: 'http_fhir_outbound',
            config: { baseUrl: 'http://hapi.fhir.org/baseR4', timeout: 30 },
        },
        messageType: 'ORU^R01',
        mappings: [],
        transformationFlow: 'hl7_to_fhir',
        auto_start: true,
        deployment_mode: 'manual',
        status: 'active',
        debug_logging: true,
        log_retention_days: 7,
    };
}

function fhirPassthroughPayload(port) {
    return {
        name: `MVP_SMOKE_FHIR_PASS_${port}`,
        description: 'MVP smoke test — FHIR passthrough',
        sourceType: 'fhir',
        sourceConnectivity: 'http_fhir_inbound',
        sourceConfig: { host: '0.0.0.0', port },
        sourceConnectorConfig: {
            connectorType: 'http_fhir_inbound',
            config: { host: '0.0.0.0', port },
        },
        targetType: 'fhir',
        targetConnectivity: 'http_fhir_outbound',
        targetConfig: { baseUrl: 'http://hapi.fhir.org/baseR4', timeout: 30 },
        targetConnectorConfig: {
            connectorType: 'http_fhir_outbound',
            config: { baseUrl: 'http://hapi.fhir.org/baseR4', timeout: 30 },
        },
        messageType: 'FHIR:Patient',
        mappings: [],
        transformationFlow: 'fhir_passthrough',
        auto_start: true,
        deployment_mode: 'manual',
        status: 'active',
        debug_logging: true,
        log_retention_days: 7,
    };
}

function hl7ToFilePayload(port, filePath) {
    return {
        name: `MVP_SMOKE_HL7_FILE_${port}`,
        description: 'MVP smoke test — HL7 to file (passthrough)',
        sourceType: 'hl7v2',
        sourceConnectivity: 'tcp_mllp_inbound',
        sourceConfig: { host: '0.0.0.0', port },
        sourceConnectorConfig: {
            connectorType: 'tcp_mllp_inbound',
            config: { host: '0.0.0.0', port },
        },
        targetType: 'file',
        targetConnectivity: 'file_writer',
        targetConfig: { outputPath: filePath, createDirs: true },
        targetConnectorConfig: {
            connectorType: 'file_writer',
            config: { outputPath: filePath, createDirs: true },
        },
        messageType: 'ADT^A01',
        mappings: [],
        transformationFlow: 'hl7_passthrough',
        auto_start: true,
        deployment_mode: 'manual',
        status: 'active',
        debug_logging: true,
        log_retention_days: 7,
    };
}

module.exports = {
    HL7_ADT_A01,
    HL7_ADT_A03,
    HL7_ORU_R01,
    adtToFhirPayload,
    oruToFhirPayload,
    fhirPassthroughPayload,
    hl7ToFilePayload,
};
