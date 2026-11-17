'use strict';
// The V296 generator output (ezIdentitySense milestone M4): the ADT template's
// event routing and the HL7-to-FHIR mappings it depends on.
const { execFileSync } = require('child_process');
const path = require('path');
const fs = require('fs');

const ROOT = path.join(__dirname, '..', '..');
const sql = execFileSync(process.execPath, [path.join(ROOT, 'scripts/templates/ezidentitysense-templates.js'), 'm4'], { encoding: 'utf8' });

// The template's pipeline_config, from its INSERT.
function pipelineConfig() {
    const m = sql.match(/'(\{"execution_groups".*?)'::jsonb, 'ADT'/s);
    expect(m).not.toBeNull();
    return JSON.parse(m[1].replace(/''/g, "'"));
}
const steps = () => pipelineConfig().execution_groups.flatMap(g => g.steps);

describe('V296 (M4) generated migration', () => {
    test('is the committed file, byte for byte', () => {
        const committed = fs.readFileSync(path.join(ROOT, 'database/migrations/V296__EzIdentitySense_M4_ADT_Events.sql'), 'utf8');
        expect(committed).toBe(sql);
    });

    test('routes events through a value map, with the site table as overrides', () => {
        const sw = steps().find(s => s.step_alias === 'adt_event');
        expect(sw.config.valueMap).toMatchObject({ A40: 'merge', A30: 'merge', A34: 'merge', A18: 'merge', A47: 'replace_id', A23: 'remove', A29: 'remove' });
        expect(sw.config.valueMapOverrides).toBe('{{var.eventActions}}');
    });

    test('links, unlinks and account events are refused, never silently applied', () => {
        const sw = steps().find(s => s.step_alias === 'adt_event');
        for (const ev of ['A24', 'A37', 'A35', 'A36', 'A41', 'A42', 'A43', 'A44', 'A45']) expect(sw.config.valueMap[ev]).toBe('unsupported');
        const c = sw.config.cases.find(x => x.value === 'unsupported');
        // the executor reads "action", not "type": a typo here silently accepts the message
        expect(c.actions).toEqual([expect.objectContaining({ action: 'reject' })]);
        expect(c.actions[0].errorMessage).toMatch(/not applied/);
    });

    test('every case has a branch step, A47 registers then merges, in that order', () => {
        const all = steps();
        const byCase = v => all.filter(s => s.case_value === v).sort((a, b) => a.sequence - b.sequence).map(s => s.step_alias);
        expect(byCase('merge')).toEqual(['merge_record']);
        expect(byCase('remove')).toEqual(['remove_record']);
        expect(byCase('replace_id')).toEqual(['register_new_id', 'merge_old_id']);
        expect(byCase('default')).toEqual(['register_patient']);
        for (const s of all.filter(x => x.parent_conditional_step_id)) expect(s.parent_conditional_step_id).toBe('adt_event');
    });

    test('merge steps read the retired id from MRG-1', () => {
        const m = steps().find(s => s.step_alias === 'merge_old_id');
        expect(m.config.fieldMappings.retiredId).toBe('MRG.1.1');
        expect(m.config.requestBody.active).toBe(false);
    });

    test('the event table is an optional setup field; credentials ship blank', () => {
        expect(sql).toMatch(/"field":"eventActions"[^}]*"type":"keyvalue"[^}]*"required":false/);
        expect(sql).not.toMatch(/"oauth2ClientSecret":"[^"{]/);
    });

    test('adds HL7 to FHIR mappings for A30, A18 and A47 once, from the A40 mapping', () => {
        for (const t of ['ADT^A30', 'ADT^A18', 'ADT^A47']) expect(sql).toContain(`('${t}',`);
        expect(sql).toContain('NOT EXISTS');
        expect(sql).toContain("OOB_ADT_A40_v2.5.1_R4");
    });
});

describe('V296 (M4) outbound ADT templates: the site event table', () => {
    const configs = [...sql.matchAll(/'(\{"execution_groups".*?)'::jsonb, '(?:CloudEvents|FHIR[^']*|[A-Za-z]+)'/gs)].map(m => JSON.parse(m[1].replace(/''/g, "'")));
    const builders = configs.map(c => c.execution_groups.flatMap(g => g.steps).find(s => s.step_alias === 'build_adt')).filter(Boolean);

    test('both ADT-sending templates take the table, and can build only what they have structures for', () => {
        expect(builders).toHaveLength(2);
        for (const b of builders) {
            expect(b.config.triggerEventMapOverrides).toBe('{{var.triggerOverrides}}');
            expect(Object.keys(b.config.messageStructureMap).sort()).toEqual(['A04', 'A08', 'A28', 'A29', 'A31', 'A37', 'A40']);
            // A04/A08 are ADT_A01; the merge, unlink and delete structures are unchanged
            expect(b.config.messageStructureMap).toMatchObject({ A04: 'ADT_A01', A08: 'ADT_A01', A40: 'ADT_A39', A37: 'ADT_A37', A29: 'ADT_A21' });
        }
    });

    test('the setup field is optional and lists the allowed events', () => {
        expect((sql.match(/"field":"triggerOverrides"/g) || []).length).toBe(2);
        expect(sql).toMatch(/"field":"triggerOverrides"[^}]*"type":"keyvalue"[^}]*"required":false/);
    });
});
