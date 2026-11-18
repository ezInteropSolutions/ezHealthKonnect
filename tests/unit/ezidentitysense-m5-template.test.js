'use strict';
// The V297 generator output (ezIdentitySense milestone M5): the scheduled
// trigger connector type and the bulk export template.
const { execFileSync } = require('child_process');
const path = require('path');
const fs = require('fs');

const ROOT = path.join(__dirname, '..', '..');
const sql = execFileSync(process.execPath, [path.join(ROOT, 'scripts/templates/ezidentitysense-templates.js'), 'm5'], { encoding: 'utf8' });

function row() {
    const m = sql.match(/'(\{"execution_groups".*?)'::jsonb, 'EXPORT',\s*'(\[.*?\])'::jsonb, ARRAY/s);
    expect(m).not.toBeNull();
    return { config: JSON.parse(m[1].replace(/''/g, "'")), fields: JSON.parse(m[2].replace(/''/g, "'")) };
}

describe('V297 (M5) generated migration', () => {
    test('is the committed file, byte for byte', () => {
        expect(fs.readFileSync(path.join(ROOT, 'database/migrations/V297__EzIdentitySense_M5_Bulk_Export.sql'), 'utf8')).toBe(sql);
    });

    test('adds the scheduled trigger connector type', () => {
        expect(sql).toMatch(/INSERT INTO connectivity_types[\s\S]*'schedule_inbound',\s*'inbound'/);
        expect(sql).toMatch(/"daily_at"[\s\S]*"interval_minutes"[\s\S]*"overlap_minutes"/);
    });
});

describe('Bulk export template', () => {
    const { config, fields } = row();
    const all = config.execution_groups.flatMap(g => g.steps);
    const step = a => all.find(s => s.step_alias === a);

    test('runs on a schedule, with a small overlap, and keeps its own cursor', () => {
        const feed = step('export_schedule').config;
        expect(feed.connectorType).toBe('schedule_inbound');
        expect(feed.config.overlap_minutes).toBeGreaterThan(0);
        expect(feed.config.cursor_key).toBe('ezidentitysense_export');
        expect(fields.find(f => f.field === 'daily_at')).toMatchObject({ section: 'source', required: true });
        expect(fields.find(f => f.field === 'timezone')).toMatchObject({ section: 'source', required: true });
    });

    test('asks only for what changed since the last good run, and waits for the status address', () => {
        const start = step('start_export').config;
        expect(start.endpoint).toBe('/fhir/r4/Patient/$export?_type=Patient&_since={since}');
        expect(start.fieldMappings.since).toBe('message.since');
        expect(start.headers.Prefer).toBe('respond-async');
        expect(start.headerOutputs).toEqual({ statusUrl: 'Content-Location' });
        const wait = step('wait_export').config;
        expect(wait.endpoint).toBe('{statusUrl}');
        expect(wait.encodePlaceholders).toBe(false);
        expect(wait.fieldMappings.statusUrl).toBe('steps.start_export.step_output.status_url');
        expect(wait.poll).toMatchObject({ untilStatus: [200] });
    });

    test('each manifest file is downloaded by a loop, and the manifest is saved last', () => {
        const loop = step('download_files');
        expect(loop.step_type).toBe('control.loop');
        expect(loop.config.collection).toBe('steps.wait_export.step_output.output');
        expect(loop.config.childStepIds).toEqual(['download_file']);
        expect(loop.config.breakOnError).toBe(true);
        const dl = step('download_file').config;
        expect(dl.fieldMappings.fileUrl).toBe('item.url');
        expect(dl.saveTo.directory).toBe('{{var.outputDirectory}}');
        expect(dl.saveTo.fileName).toMatch(/\{stamp\}-part\{part\}\.ndjson$/);
        const last = step('save_manifest');
        expect(last.sequence).toBeGreaterThan(loop.sequence);
        expect(last.config.saveTo.fileName).toMatch(/manifest\.json$/);
        expect(all.every(s => s.sequence <= last.sequence)).toBe(true);
    });

    test('credentials ship blank; the scopes asked for are the export scopes', () => {
        for (const a of ['start_export', 'wait_export', 'download_file', 'save_manifest']) {
            const c = step(a).config;
            expect(c.oauth2ClientSecret).toBe('');
            expect(c.baseUrl).toBe('');
            expect(c.oauth2Scope).toMatch(/identities\.export/);
            expect(c.oauth2Scope).toMatch(/identities\.read/);
        }
        const scope = fields.find(f => f.field === 'oauth2Scope');
        expect(scope).toMatchObject({ section: 'steps', required: false });
        expect(scope.step_aliases).toEqual(['start_export', 'wait_export', 'download_file', 'save_manifest']);
    });

    test('the guide explains the incremental runs, the overlap and the manifest', () => {
        const guide = fs.readFileSync(path.join(ROOT, 'scripts/templates/ezidentitysense-guides/bulk-export-to-files.md'), 'utf8');
        expect(guide).toMatch(/last run that \*\*succeeded\*\*/);
        expect(guide).toMatch(/overlap by 5 minutes/);
        expect(guide).toMatch(/Read the manifest first/);
        expect(guide).toMatch(/does not do/);
    });
});
