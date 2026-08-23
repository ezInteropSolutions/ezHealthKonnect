// tests/unit/services/interfaceTemplateSanitizer.test.js
//
// The single best-designed module found in the whole Node.js layer for unit
// testing: plain functions, no classes, no DB. Security-relevant — its whole
// job is preventing real credentials from leaking into a shareable interface
// template. Previously zero test coverage.
const {
    isSensitiveKey,
    sanitizeConfig,
    buildRequiredFieldsManifest,
    buildPreviewSteps,
    sanitizePipelineConfig,
    sanitizeInterfaceForTemplate,
    mergeTemplateWithUserValues,
    SENSITIVE_PATTERNS,
} = require('../../../services/interfaceTemplateSanitizer');

describe('isSensitiveKey', () => {
    it('flags every declared sensitive pattern as a bare key', () => {
        for (const pattern of SENSITIVE_PATTERNS) {
            expect(isSensitiveKey(pattern)).toBe(true);
        }
    });

    it('is case-insensitive', () => {
        expect(isSensitiveKey('PASSWORD')).toBe(true);
        expect(isSensitiveKey('Api_Key')).toBe(true);
    });

    it('flags a key that merely contains a sensitive substring', () => {
        expect(isSensitiveKey('db_password')).toBe(true);
        expect(isSensitiveKey('sql_server_host')).toBe(true);
    });

    it('does not flag an unrelated field', () => {
        expect(isSensitiveKey('resourceType')).toBe(false);
        expect(isSensitiveKey('enabled')).toBe(false);
    });

    it('a SAFE_OVERRIDE wins even when the key also contains a genuinely sensitive substring', () => {
        // "endpoint_timeout" contains "endpoint" (a real SENSITIVE_PATTERNS
        // entry) AND ends with "_timeout" (a real SAFE_OVERRIDES entry) — a
        // real collision, not a vacuous case. Proves the override list is
        // checked, and wins, BEFORE the sensitive-pattern list, per the
        // function's own documented precedence — a config field describing
        // *how long to wait* for an endpoint is not itself a credential.
        expect(isSensitiveKey('endpoint_timeout')).toBe(false);
        // Sanity: with the override suffix removed, the same substring alone
        // is correctly flagged, proving this isn't just "timeout is exempt."
        expect(isSensitiveKey('endpoint')).toBe(true);
    });
});

describe('sanitizeConfig', () => {
    it('clears sensitive fields but keeps the key present (empty string, not deleted)', () => {
        const { sanitized, clearedFields } = sanitizeConfig({ host: '10.0.0.5', password: 'hunter2', method: 'POST' });
        expect(sanitized).toEqual({ host: '', password: '', method: 'POST' });
        expect(clearedFields.sort()).toEqual(['host', 'password']);
    });

    it('recurses into nested objects with dot-notation field paths', () => {
        const { sanitized, clearedFields } = sanitizeConfig({
            oauth2: { client_secret: 'abc', scope: 'read' },
        });
        expect(sanitized.oauth2).toEqual({ client_secret: '', scope: 'read' });
        expect(clearedFields).toEqual(['oauth2.client_secret']);
    });

    it('does not recurse into arrays, and leaves array values untouched', () => {
        const { sanitized, clearedFields } = sanitizeConfig({ headers: ['a', 'b'] });
        expect(sanitized.headers).toEqual(['a', 'b']);
        expect(clearedFields).toEqual([]);
    });

    it('handles a non-object input by returning it unchanged with no cleared fields', () => {
        expect(sanitizeConfig(null)).toEqual({ sanitized: null, clearedFields: [] });
        expect(sanitizeConfig('not an object')).toEqual({ sanitized: 'not an object', clearedFields: [] });
    });

    it('leaves an already-empty config untouched', () => {
        expect(sanitizeConfig({})).toEqual({ sanitized: {}, clearedFields: [] });
    });
});

describe('buildRequiredFieldsManifest', () => {
    it('produces a friendly label for a known field and marks it required per the REQUIRED list', () => {
        const manifest = buildRequiredFieldsManifest(['host'], 'source');
        expect(manifest).toEqual([
            expect.objectContaining({ section: 'source', field: 'host', label: 'Host / IP Address', type: 'string', required: true }),
        ]);
    });

    it('falls back to a humanized label for an unknown field', () => {
        const manifest = buildRequiredFieldsManifest(['some_custom_field'], 'target');
        expect(manifest[0].label).toBe('Some Custom Field');
        expect(manifest[0].required).toBe(false);
    });

    it('uses only the leaf segment of a dot-notation path for the label lookup', () => {
        const manifest = buildRequiredFieldsManifest(['oauth2.client_secret'], 'source');
        expect(manifest[0].field).toBe('client_secret');
        expect(manifest[0].full_path).toBe('oauth2.client_secret');
        expect(manifest[0].type).toBe('password');
    });
});

describe('buildPreviewSteps', () => {
    it('extracts name/icon/step_type from execution_groups', () => {
        const steps = buildPreviewSteps({
            execution_groups: [{ steps: [{ step_type: 'hl7_fhir_transform', step_name: 'Transform' }] }],
        });
        expect(steps).toEqual([{ name: 'Transform', icon: '🔄', step_type: 'hl7_fhir_transform' }]);
    });

    it('falls back to a default icon for an unknown step type', () => {
        const steps = buildPreviewSteps({ execution_groups: [{ steps: [{ step_type: 'some.custom.step' }] }] });
        expect(steps[0].icon).toBe('⚙️');
        expect(steps[0].name).toBe('some.custom.step'); // no step_name -> falls back to type
    });

    it('never throws on malformed pipeline_config — returns an empty array instead', () => {
        expect(buildPreviewSteps(null)).toEqual([]);
        expect(buildPreviewSteps({ execution_groups: 'not-an-array' })).toEqual([]);
        expect(buildPreviewSteps(undefined)).toEqual([]);
    });
});

describe('sanitizePipelineConfig', () => {
    it('sanitizes connector step configs but leaves business-logic steps completely untouched', () => {
        const pipeline = {
            execution_groups: [{
                steps: [
                    { step_type: 'connector.inbound', step_name: 'TCP In', config: { host: '10.0.0.1', port: 6613 } },
                    { step_type: 'hl7_fhir_transform', step_name: 'Transform', config: { host: 'this-is-not-a-real-host-field-but-must-survive' } },
                ],
            }],
        };
        const { sanitizedPipeline, clearedFields } = sanitizePipelineConfig(pipeline);
        const [connectorStep, transformStep] = sanitizedPipeline.execution_groups[0].steps;

        expect(connectorStep.config.host).toBe('');
        expect(connectorStep.config.port).toBe('');
        expect(transformStep.config.host).toBe('this-is-not-a-real-host-field-but-must-survive'); // NOT a connector step type — untouched
        expect(clearedFields).toEqual(
            expect.arrayContaining(['pipeline.group0.TCP In.host', 'pipeline.group0.TCP In.port'])
        );
    });

    it('returns the pipeline unchanged when execution_groups is missing/malformed', () => {
        expect(sanitizePipelineConfig(null)).toEqual({ sanitizedPipeline: {}, clearedFields: [] });
        expect(sanitizePipelineConfig({ execution_groups: 'nope' })).toEqual({ sanitizedPipeline: { execution_groups: 'nope' }, clearedFields: [] });
    });

    it('skips steps whose config is missing or not an object', () => {
        const pipeline = { execution_groups: [{ steps: [{ step_type: 'connector.inbound' }] }] };
        const { sanitizedPipeline } = sanitizePipelineConfig(pipeline);
        expect(sanitizedPipeline.execution_groups[0].steps[0]).toEqual({ step_type: 'connector.inbound' });
    });
});

describe('sanitizeInterfaceForTemplate (full integration of the module)', () => {
    it('combines source/target connectivity sanitization with pipeline sanitization and builds a required-fields manifest', () => {
        const iface = { message_type: 'ADT^A01' };
        const connectivity = {
            source_connector_type: 'tcp_mllp_inbound',
            source_config: { host: '10.0.0.1', port: 6613 },
            target_connector_type: 'http_fhir_outbound',
            target_config: { base_url: 'https://fhir.example.org', password: 'secret' },
        };
        const pipeline = { execution_groups: [] };

        const result = sanitizeInterfaceForTemplate(iface, connectivity, pipeline);

        expect(result.source_connector_type).toBe('tcp_mllp_inbound');
        expect(result.source_config_template).toEqual({ host: '', port: '' });
        expect(result.target_config_template).toEqual({ base_url: '', password: '' });
        expect(result.message_type).toBe('ADT^A01');
        expect(result.sanitized_fields).toEqual(
            expect.arrayContaining(['source.host', 'source.port', 'target.base_url', 'target.password'])
        );
        // Required-fields manifest deduped and covers both sections
        const sections = result.required_connection_fields.map(f => `${f.section}:${f.field}`);
        expect(sections).toEqual(expect.arrayContaining(['source:host', 'source:port', 'target:base_url', 'target:password']));
    });

    it('falls back to a connector type found inside pipeline steps when connectivity has none', () => {
        const pipeline = {
            execution_groups: [{
                steps: [{ step_type: 'connector.inbound', step_name: 'In', config: { connectorType: 'tcp_mllp_inbound', host: '1.2.3.4' } }],
            }],
        };
        const result = sanitizeInterfaceForTemplate({}, {}, pipeline);
        expect(result.source_connector_type).toBe('tcp_mllp_inbound');
    });

    it('deduplicates required fields that appear in both legacy connectivity and a pipeline step', () => {
        const connectivity = { source_config: { host: '1.1.1.1' } };
        const pipeline = {
            execution_groups: [{
                steps: [{ step_type: 'connector.inbound', step_name: 'In', config: { host: '1.1.1.1' } }],
            }],
        };
        const result = sanitizeInterfaceForTemplate({}, connectivity, pipeline);
        const hostEntries = result.required_connection_fields.filter(f => f.field === 'host' && f.section === 'source');
        expect(hostEntries).toHaveLength(1); // not duplicated despite appearing in two places
    });
});

describe('mergeTemplateWithUserValues', () => {
    it('overlays non-empty user values onto the template', () => {
        const merged = mergeTemplateWithUserValues({ host: '', port: '' }, { host: 'real-host.example.org', port: 6613 });
        expect(merged).toEqual({ host: 'real-host.example.org', port: 6613 });
    });

    it('does not overwrite template values with null/undefined/empty-string user input', () => {
        const merged = mergeTemplateWithUserValues({ host: 'template-default' }, { host: '' });
        expect(merged.host).toBe('template-default');
    });

    it('returns a shallow copy of the template when userValues is absent', () => {
        const template = { host: 'x' };
        const merged = mergeTemplateWithUserValues(template, null);
        expect(merged).toEqual(template);
        expect(merged).not.toBe(template); // copy, not the same reference
    });
});
