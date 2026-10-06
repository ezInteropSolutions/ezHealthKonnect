'use strict';
/**
 * Fixtures + pipeline builders for coverage-audit-e2e.spec.js.
 *
 * Coverage Audit tracks fields *read by a mapping step*, not bytes accepted
 * by a connector — so each format below reuses the exact mapping-step chain
 * already proven correct in a live, shipped OOB template (pulled directly
 * from interface_templates.pipeline_config in the real DB at the time this
 * file was written), with only the connector.inbound step swapped for
 * http_rest_inbound (a real HTTP port a host-side Playwright test can reach
 * directly, unlike the templates' own SFTP-only inbound connectors).
 *
 * http_rest_inbound's real config shape (confirmed directly from
 * services/connectors/http_rest_inbound.go, NOT the stale shape the live
 * cda-ccd-inbound-fhir-r4 template itself uses — that template's own
 * connector.inbound config uses the wrong field names and would fail real
 * activation, a real bug found while building this fixture):
 *   { port, endpoint_path, http_methods: ['POST'], authentication_type: 'none', content_type }
 */

const fs   = require('fs');
const path = require('path');

const REPO_ROOT = path.resolve(__dirname, '../../..');

function readSample(relPath, encoding) {
    return fs.readFileSync(path.join(REPO_ROOT, relPath), encoding);
}

// ── Real/self-authored sample content per format ──────────────────────────

// Composite PID.5 (name) / PID.11 (address) / PV1.8 (attending doctor) —
// deliberately the same shape hand-verified live earlier this session, so
// component-level decomposition AND the one real accepted gap (PID.1, a
// Set-ID field with no FHIR target) are both exercised, not just a trivial
// message.
const HL7_ADT_SAMPLE = [
    'MSH|^~\\&|SENDAPP|SENDFAC|RECVAPP|RECVFAC|20261004084500||ADT^A01|COVAUDIT0001|P|2.5',
    'EVN|A01|20261004084500',
    'PID|1||MRN900111^^^MRN||Doe^Jane^Marie||19800101|F|||100 Main St^Apt 4^Springfield^IL^62701^USA||5551234567',
    'PV1|1|I|WARDA^101^1|||||ATT001^Smith^Robert|||||||||||VISIT001',
].join('\r');

// A plausible real MedicationRequest — FHIR-as-inbound-source, proving
// resolveJSONPathValue's already-hooked tracking works for this direction.
const FHIR_MEDICATION_REQUEST_SAMPLE = JSON.stringify({
    resourceType: 'MedicationRequest',
    status: 'active',
    intent: 'order',
    subject: { display: 'COVAUDIT TestPatient' },
    medicationCodeableConcept: { text: 'Amoxicillin 500mg capsule' },
    dosageInstruction: [{ text: 'Take 1 capsule by mouth three times daily' }],
});

function ediSample() {
    return readSample('edi/testdata/real_samples/x12org_837p_ben_kildare_sample.txt', 'utf8');
}

function ncpdpScriptSample() {
    return readSample('ncpdp/testdata/real_samples/dgoradia_sample_newrx.xml', 'utf8');
}

function ncpdpTelecomSample() {
    // Real, control-character-delimited wire format — must stay a raw
    // Buffer, never decoded as UTF-8 text (it would corrupt the 0x1E/0x1C
    // delimiters).
    return readSample('ncpdptelecom/testdata/generated_samples/request_baseline_male.txt', null);
}

function cdaSample() {
    return readSample('cda/document/testdata/full_ccd_nist.xml', 'utf8');
}

// ── http_rest_inbound connector config (the one piece every non-HL7/FHIR
//    format needs, since their own OOB templates are SFTP-only) ───────────

function httpRestInboundStep(port, endpointPath, sequence = 5, stepName = 'Receive via HTTP') {
    return {
        config: {
            config: {
                port,
                endpoint_path: endpointPath,
                http_methods: ['POST'],
                authentication_type: 'none',
                content_type: 'text/plain',
            },
            connectorType: 'http_rest_inbound',
        },
        enabled: true,
        required: true,
        sequence,
        step_name: stepName,
        step_type: 'connector.inbound',
    };
}

function sinkOutboundStep(sequence = 295, stepName = 'Store Result') {
    return {
        config: {
            config: { enable_logging: true, enable_validation: true },
            contentField: 'raw',
            connectorType: 'sink_outbound',
        },
        enabled: true,
        required: true,
        sequence,
        step_name: stepName,
        step_type: 'connector.outbound',
    };
}

// ── Interface payload builders (POST /api/wizard/complete via ApiHelper) ──
// These only need to produce a valid base interface row; the real pipeline
// is set explicitly afterward via api.savePipeline() with the step lists
// below, so wizard's own source/targetType mapping (hl7/fhir-only, falls
// back to hl7/tcp for anything else — confirmed in wizardController.js's
// mapSourceTypeAndConnectivity) doesn't need to recognize these formats.

function basePayload(name, messageType) {
    return {
        name,
        description: `Coverage Audit E2E — ${name}`,
        sourceType: 'hl7v2',
        sourceConnectivity: 'tcp',
        sourceConfig: {},
        targetType: 'fhir',
        targetConnectivity: 'http',
        targetConfig: {},
        messageType,
        mappings: [],
        transformationFlow: 'custom',
        auto_start: true,
        deployment_mode: 'manual',
        status: 'active',
        debug_logging: true,
        log_retention_days: 7,
    };
}

function hl7Payload(name, port) {
    return {
        ...basePayload(name, 'ADT^A01'),
        sourceType: 'hl7v2',
        sourceConnectivity: 'tcp_mllp_inbound',
        sourceConfig: { host: '0.0.0.0', port },
        sourceConnectorConfig: { connectorType: 'tcp_mllp_inbound', config: { host: '0.0.0.0', port } },
    };
}

function fhirPayload(name, port) {
    return {
        ...basePayload(name, 'FHIR:MedicationRequest'),
        sourceType: 'fhir',
        sourceConnectivity: 'http_fhir_inbound',
        sourceConfig: { host: '0.0.0.0', port },
        sourceConnectorConfig: { connectorType: 'http_fhir_inbound', config: { host: '0.0.0.0', port } },
    };
}

function genericHttpRestPayload(name, messageType, port, endpointPath) {
    // A real, concrete bug found building this fixture: interfaces.source_
    // connectivity/source_config are NOT display-only for a plain (non-
    // template) createInterface call — processing/engine.go's own
    // ActivateInterface reads them directly, independent of the pipeline's
    // own connector.inbound step. Leaving them at basePayload()'s
    // placeholder {"type":"tcp","config":{}} made the Go engine try to
    // start a DEFAULT TCP/MLLP listener on port 2575 instead of this
    // format's real http_rest_inbound step — these must agree with the
    // pipeline step's own port/path, not just be filler.
    return {
        ...basePayload(name, messageType),
        sourceType: 'http',
        sourceConnectivity: 'http_rest_inbound',
        sourceConfig: { port, endpoint_path: endpointPath },
        sourceConnectorConfig: {
            connectorType: 'http_rest_inbound',
            config: { port, endpoint_path: endpointPath, http_methods: ['POST'], authentication_type: 'none' },
        },
    };
}

module.exports = {
    HL7_ADT_SAMPLE,
    FHIR_MEDICATION_REQUEST_SAMPLE,
    ediSample,
    ncpdpScriptSample,
    ncpdpTelecomSample,
    cdaSample,
    httpRestInboundStep,
    sinkOutboundStep,
    hl7Payload,
    fhirPayload,
    genericHttpRestPayload,
};
