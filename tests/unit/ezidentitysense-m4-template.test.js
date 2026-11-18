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

// The FHIR relay template (its own INSERT, found by slug).
function relayRow() {
    const i = sql.indexOf("'fhir-patient-relay-to-ezidentitysense'");
    expect(i).toBeGreaterThan(-1);
    const m = sql.slice(i).match(/'(\{"execution_groups".*?)'::jsonb, 'FHIR:Patient',\s*'(\[.*?\])'::jsonb, ARRAY/s);
    expect(m).not.toBeNull();
    return { config: JSON.parse(m[1].replace(/''/g, "'")), fields: JSON.parse(m[2].replace(/''/g, "'")) };
}

describe('FHIR Patient relay template', () => {
    const { config, fields } = relayRow();
    const all = config.execution_groups.flatMap(g => g.steps);
    const byAlias = a => all.find(s => s.step_alias === a);

    test('the reply mode is a setup switch, off unless chosen', () => {
        const inbound = byAlias('fhir_patient_feed');
        expect(inbound.config.config.syncReply).toMatchObject({ enabled: '{{var.immediateReply}}', serialize: true });
        const f = fields.find(x => x.field === 'immediateReply');
        expect(f).toMatchObject({ section: 'variables', type: 'boolean', required: false });
        expect(f.hint).toMatch(/Off \(default\)/);
    });

    test('creates and updates PUT the Patient to the feed exactly as received', () => {
        const put = byAlias('put_patient');
        expect(put.case_value).toBe('default');
        expect(put.config.method).toBe('PUT');
        expect(put.config.endpoint).toBe('/fhir/r4/Patient?identifier={{var.sourceAuthority}}|{localId}');
        expect(put.config.fieldMappings.localId).toBe('message.identifier[system={{var.sourceAuthority}}].value');
        expect(put.config.bodyRef).toBe('message.raw');
    });

    test('a delete takes the local ID from the URL, and every step routes from request_kind', () => {
        const del = byAlias('delete_patient');
        expect(del.config.method).toBe('DELETE');
        expect(del.config.fieldMappings.localId).toBe('message._http.resourceId');
        expect(byAlias('request_kind').config.field).toBe('message._http.method');
        for (const s of [byAlias('put_patient'), del]) expect(s.parent_conditional_step_id).toBe('request_kind');
    });

    test('who may send is a required choice, credentials ship blank, the secrets are not left in the config', () => {
        const auth = fields.find(x => x.field === 'authType');
        expect(auth).toMatchObject({ section: 'source', type: 'select', required: true });
        expect(auth.options.map(o => o.value)).toEqual(['bearer', 'api_key', 'basic', 'none']);
        for (const s of [byAlias('put_patient'), byAlias('delete_patient')]) {
            expect(s.config.oauth2ClientSecret).toBe('');
            expect(s.config.baseUrl).toBe('');
        }
        expect(byAlias('fhir_patient_feed').config.config.bearerToken).toBeUndefined();
    });

    test('the guide says when to use each mode', () => {
        const guide = fs.readFileSync(path.join(ROOT, 'scripts/templates/ezidentitysense-guides/fhir-relay-to-ezidentitysense.md'), 'utf8');
        expect(guide).toMatch(/Use it when/);
        expect(guide.match(/\*\*Use it when\*\*/g)).toHaveLength(2);
        expect(guide).not.toMatch(/IHE conformance|conformant/i);
    });
});

// The three CSV load templates (folder, SFTP, S3), found by slug.
function csvRow(slug) {
    const i = sql.indexOf(`'${slug}'`);
    expect(i).toBeGreaterThan(-1);
    const m = sql.slice(i).match(/'(\{"execution_groups".*?)'::jsonb, 'CSV',\s*'(\[.*?\])'::jsonb, ARRAY/s);
    expect(m).not.toBeNull();
    return { config: JSON.parse(m[1].replace(/''/g, "'")), fields: JSON.parse(m[2].replace(/''/g, "'")) };
}

describe('CSV load templates', () => {
    const variants = {
        'csv-load-folder-to-ezidentitysense': 'file_listener',
        'csv-load-sftp-to-ezidentitysense': 'sftp_inbound',
        'csv-load-s3-to-ezidentitysense': 'aws_s3_inbound',
    };

    test.each(Object.entries(variants))('%s picks up the file with %s and archives it', (slug, type) => {
        const { config } = csvRow(slug);
        const feed = config.execution_groups[0].steps[0];
        expect(feed.config.connectorType).toBe(type);
        expect(feed.config.config.after_processing).toBe('archive');
    });

    test('every variant runs the same steps after the file is picked up, in order', () => {
        const aliases = slug => csvRow(slug).config.execution_groups.flatMap(g => g.steps).slice(1).map(s => s.step_alias);
        const base = aliases('csv-load-folder-to-ezidentitysense');
        expect(base).toEqual(['save_mapping', 'upload_file', 'start_check', 'wait_check', 'check_state', 'check_rejects',
            'check_clean', 'start_load', 'wait_load', 'load_state', 'load_rejects', 'load_clean']);
        for (const slug of Object.keys(variants)) expect(aliases(slug)).toEqual(base);
    });

    const { config, fields } = csvRow('csv-load-folder-to-ezidentitysense');
    const step = a => config.execution_groups.flatMap(g => g.steps).find(s => s.step_alias === a);

    test('the file is uploaded as received, under its own name; the load needs a clean check first', () => {
        const up = step('upload_file').config;
        expect(up.bodyRef).toBe('message.raw');
        expect(up.fieldMappings.fileName).toBe('message._sourceFile');
        expect(up.endpoint).toBe('/v1/bulk/files?name={fileName}');
        expect(step('start_check').config.requestBody.kind).toBe('check');
        expect(step('start_load').config.requestBody.kind).toBe('load');
        const clean = step('check_clean').config;
        expect(clean.field).toBe('steps.wait_check.step_output.rejects');
        expect(clean.cases[0].value).toBe('0');
        expect(clean.default.actions[0]).toMatchObject({ action: 'reject' });
        expect(step('check_rejects').sequence).toBeLessThan(step('check_clean').sequence);
        expect(step('check_clean').sequence).toBeLessThan(step('start_load').sequence);
    });

    test('each job is waited for until it reaches an end state, and only "completed" goes on', () => {
        for (const a of ['wait_check', 'wait_load']) {
            const p = step(a).config.poll;
            expect(p.untilField).toBe('state');
            expect(p.untilValues).toEqual(expect.arrayContaining(['completed', 'failed', 'cancelled', 'paused', 'interrupted']));
            expect(p.timeoutSeconds).toBeGreaterThan(0);
        }
        for (const a of ['check_state', 'load_state']) {
            expect(step(a).config.cases[0].value).toBe('completed');
            expect(step(a).config.default.actions[0].action).toBe('reject');
        }
    });

    test('the client needs only the bulk.load scope, and the mapping comes from the setup form', () => {
        for (const s of config.execution_groups.flatMap(g => g.steps).filter(x => x.step_type === 'enrichment.api')) {
            expect(s.config.oauth2Scope).toBe('bulk.load');
            expect(s.config.oauth2ClientSecret).toBe('');
        }
        expect(step('save_mapping').config.requestBody).toEqual({ mapping: { columns: '{{var.columnMapping}}', constants: '{{var.constants}}' } });
        expect(fields.find(f => f.field === 'columnMapping')).toMatchObject({ section: 'variables', type: 'keyvalue', required: true });
        expect(fields.find(f => f.field === 'constants')).toMatchObject({ type: 'keyvalue', required: false });
        expect(fields.find(f => f.field === 'encoding')).toMatchObject({ type: 'select', required: true });
    });

    test('files are taken one at a time, in order', () => {
        expect(sql.match(/'in_order'/g).length).toBeGreaterThanOrEqual(7);
    });

    test('the guide says which template to use', () => {
        const guide = fs.readFileSync(path.join(ROOT, 'scripts/templates/ezidentitysense-guides/csv-load-to-ezidentitysense.md'), 'utf8');
        expect(guide).toMatch(/Which template to use/);
        expect(guide).toMatch(/bulk\.load/);
    });
});
