// tests/unit/services/interfaceTemplateVariables.test.js
// Template variables: {{var.NAME}} tokens in a template's pipeline_config are
// replaced at "Use template" time with values the user entered for fields the
// template declares in its "variables" section.
const { substituteTemplateVariables } = require('../../../services/interfaceTemplateSanitizer');

describe('substituteTemplateVariables', () => {
    const defs = [
        { section: 'variables', field: 'sourceAuthority' },
        { section: 'variables', field: 'unused' },
        { section: 'steps', field: 'notAVariable' },
    ];
    const cfg = () => ({ execution_groups: [{ steps: [{ config: {
        endpoint: '/fhir/r4/Patient?identifier={{var.sourceAuthority}}|{id}',
        fieldMappings: { id: 'x.identifier[system={{var.sourceAuthority}}].value' },
        list: ['{{var.sourceAuthority}}', 3, null],
        other: '{{var.unused}} {{var.notAVariable}} {{var.undeclared}}',
    } }] }] });

    it('replaces declared variables everywhere, as text', () => {
        const out = substituteTemplateVariables(cfg(), defs, { sourceAuthority: 'urn:oid:1.2.3', notAVariable: 'x' });
        const c = out.execution_groups[0].steps[0].config;
        expect(c.endpoint).toBe('/fhir/r4/Patient?identifier=urn:oid:1.2.3|{id}');
        expect(c.fieldMappings.id).toBe('x.identifier[system=urn:oid:1.2.3].value');
        expect(c.list).toEqual(['urn:oid:1.2.3', 3, null]);
        // blank, non-variable-section and undeclared tokens stay visible
        expect(c.other).toBe('{{var.unused}} {{var.notAVariable}} {{var.undeclared}}');
    });

    it('cannot change structure and does not mutate the input', () => {
        const input = cfg();
        const before = JSON.stringify(input);
        const hostile = '"}],"evil":{"';
        const out = substituteTemplateVariables(input, defs, { sourceAuthority: hostile });
        expect(JSON.stringify(input)).toBe(before);
        const c = out.execution_groups[0].steps[0].config;
        expect(Object.keys(c)).toEqual(['endpoint', 'fieldMappings', 'list', 'other']);
        expect(c.list[0]).toBe(hostile);
    });

    it('tolerates missing arguments', () => {
        const c = cfg();
        expect(substituteTemplateVariables(c, null, {})).toBe(c);
        expect(substituteTemplateVariables(c, defs, null)).toBe(c);
        expect(substituteTemplateVariables(c, defs, {})).toBe(c);
    });
});

describe('keyvalue template variables', () => {
    const { parseKeyValueTable } = require('../../../services/interfaceTemplateSanitizer');
    const defs = [{ section: 'variables', field: 'namespaces', type: 'keyvalue' }];
    const cfg = () => ({ execution_groups: [{ steps: [{ config: {
        namespaces: '{{var.namespaces}}',
        byUniversalId: '{{var.namespaces|inverted}}',
        inText: 'see {{var.namespaces}}',
    } }] }] });

    it('becomes an object, and its reverse with both OID spellings', () => {
        const out = substituteTemplateVariables(cfg(), defs, { namespaces: 'HOSPA=1.2.840.99999.1\n EID = https://empi.example.org/eid \nCLINIC=urn:oid:2.16.840.99999.3' });
        const c = out.execution_groups[0].steps[0].config;
        expect(c.namespaces).toEqual({ HOSPA: '1.2.840.99999.1', EID: 'https://empi.example.org/eid', CLINIC: 'urn:oid:2.16.840.99999.3' });
        expect(c.byUniversalId).toEqual({
            '1.2.840.99999.1': 'HOSPA', 'urn:oid:1.2.840.99999.1': 'HOSPA',
            'https://empi.example.org/eid': 'EID',
            'urn:oid:2.16.840.99999.3': 'CLINIC', '2.16.840.99999.3': 'CLINIC',
        });
        // never spliced into a longer string
        expect(c.inText).toBe('see {{var.namespaces}}');
    });

    it('left blank is an empty object, so the config keeps its shape', () => {
        for (const values of [{}, { namespaces: '' }, undefined]) {
            const c = substituteTemplateVariables(cfg(), defs, values).execution_groups[0].steps[0].config;
            expect(c.namespaces).toEqual({});
            expect(c.byUniversalId).toEqual({});
        }
    });

    it('skips malformed and unsafe entries', () => {
        expect(parseKeyValueTable('A=1,=2,B=,C\nD=4\u0001,__proto__=x, E = 5 ')).toEqual({ A: '1', E: '5' });
        expect(parseKeyValueTable({ A: '1', B: '', C: 3 })).toEqual({ A: '1', C: '3' });
        expect(parseKeyValueTable(['A=1'])).toEqual({});
        expect(Object.getPrototypeOf(parseKeyValueTable('__proto__=x'))).toBe(Object.prototype);
    });
});
