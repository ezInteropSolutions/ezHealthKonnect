// tests/unit/services/TransformationPipelineService.passthroughContent.test.js
//
// Regression (v26.10 UAT, format x connectivity matrix): the wizard saved EVERY outbound connector step (except
// EDI/AS2) with contentField "fhirBundle" + contentType "application/fhir+json", whatever the flow. A passthrough has
// no fhirBundle, so the executor fell back to the whole parsed-message envelope and labelled it FHIR: an HL7
// passthrough delivered a ~387 KB JSON document (full schema embedded) instead of the HL7 message, and a FHIR
// passthrough handed the destination FHIR server {"_format":"fhir","_semantic_index":...} instead of the resource.
// A passthrough must deliver the ORIGINAL content (the parsed message's `raw` field), byte for byte.
//
// Also: normalizeWizardConfig kept only directory_path/file_pattern for the file connectors, silently discarding a
// file writer's filename_pattern/create_subdirs/overwrite_existing/encoding and most of a file listener's settings.
jest.mock('../../../config/database', () => ({}));

const TransformationPipelineService = require('../../../services/TransformationPipelineService');

function makeSequelize() {
    const inserted = [];
    return {
        inserted,
        QueryTypes: { SELECT: 'SELECT' },
        query: jest.fn(async (sql, opts) => {
            if (/INSERT INTO transformation_steps/.test(sql)) inserted.push(opts.bind);
            return [];
        }),
    };
}

async function outboundConfig(connectivity, flow, wizardConfig = {}) {
    const service = new TransformationPipelineService();
    const sequelize = makeSequelize();
    await service.addConnectorStep(sequelize, {}, 'p1', 'outbound', connectivity, wizardConfig, flow);
    return JSON.parse(sequelize.inserted[0][4]);
}

describe('outbound step content depends on the flow', () => {
    it('HL7 passthrough delivers the original HL7 (raw), not a FHIR-labelled envelope', async () => {
        const cfg = await outboundConfig('file', 'hl7_passthrough', { directory_path: '/tmp/x' });
        expect(cfg.contentField).toBe('raw');
        expect(cfg.contentType).toBe('application/hl7-v2');
    });

    it('FHIR passthrough delivers the original FHIR resource (raw) as FHIR JSON', async () => {
        const cfg = await outboundConfig('http', 'fhir_passthrough', { url: 'http://x' });
        expect(cfg.contentField).toBe('raw');
        expect(cfg.contentType).toBe('application/fhir+json');
    });

    it.each(['passthrough', 'file_processor', 'others', 'fhir_receiver', undefined, 'some_future_flow'])(
        'flow %s adds no transform, so it delivers the original content whatever its format', async (flow) => {
            const cfg = await outboundConfig('http', flow, { url: 'http://x' });
            expect(cfg.contentField).toBe('raw');
            expect(cfg.contentType).toBeUndefined();
        });

    it.each(['hl7_to_fhir', 'hl7_to_fhir_r5', 'hl7_to_fhir_stu3', 'ccd_to_fhir'])(
        'flow %s still delivers the FHIR bundle it builds', async (flow) => {
            const cfg = await outboundConfig('http', flow, { url: 'http://x' });
            expect(cfg.contentField).toBe('fhirBundle');
            expect(cfg.contentType).toBe('application/fhir+json');
        });

    it.each(['edi_x12_outbound', 'as2_outbound'])('%s still delivers the built X12 document, whatever the flow', async (type) => {
        const cfg = await outboundConfig(type, 'hl7_passthrough', {});
        expect(cfg.contentField).toBe('ediX12');
        expect(cfg.contentType).toBe('application/edi-x12');
    });

    it('an inbound step carries no content settings', async () => {
        const service = new TransformationPipelineService();
        const sequelize = makeSequelize();
        await service.addConnectorStep(sequelize, {}, 'p1', 'inbound', 'tcp', { port: 2575 }, 'hl7_passthrough');
        const cfg = JSON.parse(sequelize.inserted[0][4]);
        expect(cfg.contentField).toBeUndefined();
        expect(cfg.contentType).toBeUndefined();
    });
});

describe('normalizeWizardConfig keeps the file connectors\' real settings', () => {
    const service = new TransformationPipelineService();

    it('file_writer keeps its naming and overwrite options', () => {
        const wizard = { directory_path: '/out', filename_pattern: '{message_id}.hl7', create_subdirs: true, overwrite_existing: false, encoding: 'UTF-8', junk: 'x' };
        expect(service.normalizeWizardConfig(wizard, 'file_writer')).toEqual({
            directory_path: '/out', filename_pattern: '{message_id}.hl7', create_subdirs: true, overwrite_existing: false, encoding: 'UTF-8',
        });
    });

    it('file_listener keeps its polling and after-processing options', () => {
        const wizard = { directory_path: '/in', file_pattern: '*.hl7', polling_interval: 2, after_processing: 'delete', archive_path: '/arch', recursive: true, create_dirs: true, encoding: 'UTF-8' };
        expect(service.normalizeWizardConfig(wizard, 'file_listener')).toEqual(wizard);
    });

    it('does not invent keys that were not supplied', () => {
        expect(service.normalizeWizardConfig({ directory_path: '/out' }, 'file_writer')).toEqual({ directory_path: '/out' });
    });
});
