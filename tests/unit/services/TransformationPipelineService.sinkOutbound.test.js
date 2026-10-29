// tests/unit/services/TransformationPipelineService.sinkOutbound.test.js
//
// Regression: the wizard's store-only flows (Passthrough, FHIR Receiver) send targetConnectivity "sink".
// The outbound step was saved with connectorType "sink", which no connector is registered under (the
// registered one is "sink_outbound"), so every message to such an interface ended `failed` with
// "outbound connector type 'sink' not found". Found by tests/playwright/pipeline/wizard-data-path-e2e.spec.js.
jest.mock('../../../config/database', () => ({}));

const TransformationPipelineService = require('../../../services/TransformationPipelineService');

function makeSequelize() {
    const inserted = [];
    return {
        inserted,
        QueryTypes: { SELECT: 'SELECT' },
        // connectivity_types lookup (display name) returns nothing; the INSERT is what we capture.
        query: jest.fn(async (sql, opts) => {
            if (/INSERT INTO transformation_steps/.test(sql)) inserted.push(opts.bind);
            return [];
        }),
    };
}

describe('addConnectorStep — store-only (sink) target', () => {
    const service = new TransformationPipelineService();

    it('saves the outbound step under the registered connector type sink_outbound', async () => {
        const sequelize = makeSequelize();
        await service.addConnectorStep(sequelize, {}, 'pipeline-1', 'outbound', 'sink', {
            mode: 'sink', generateAck: true, enableLogging: true, retentionDays: 30, enableValidation: false,
        });

        expect(sequelize.inserted).toHaveLength(1);
        const [pipelineId, , stepType, sequence, configJson] = sequelize.inserted[0];
        const config = JSON.parse(configJson);
        expect(pipelineId).toBe('pipeline-1');
        expect(stepType).toBe('connector.outbound');
        expect(sequence).toBe(295);
        expect(config.connectorType).toBe('sink_outbound');
    });

    it('maps the wizard\'s camelCase options to the snake_case keys the Go connector reads', async () => {
        const sequelize = makeSequelize();
        await service.addConnectorStep(sequelize, {}, 'p', 'outbound', 'sink', {
            generateAck: false, enableLogging: false, retentionDays: 7, enableValidation: true,
        });
        const config = JSON.parse(sequelize.inserted[0][4]);
        expect(config.config).toEqual({
            generate_ack: false, enable_logging: false, retention_days: 7, enable_validation: true,
        });
    });

    it('does not change other outbound types', async () => {
        const sequelize = makeSequelize();
        await service.addConnectorStep(sequelize, {}, 'p', 'outbound', 'file', { directory_path: '/tmp/x' });
        expect(JSON.parse(sequelize.inserted[0][4]).connectorType).toBe('file_writer');
    });
});

describe('normalizeWizardConfig — sink_outbound', () => {
    const service = new TransformationPipelineService();

    it('prefers an already snake_case value over the camelCase one', () => {
        expect(service.normalizeWizardConfig({ enable_logging: false, enableLogging: true }, 'sink_outbound'))
            .toEqual({ enable_logging: false });
    });

    it('returns an empty config for an empty wizard config', () => {
        expect(service.normalizeWizardConfig({}, 'sink_outbound')).toEqual({});
    });
});
