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
