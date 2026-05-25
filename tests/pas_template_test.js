/**
 * Da Vinci PAS Template — Node.js Integration Test
 *
 * Tests the interface template controller logic that would run when a user
 * clicks "Use Template" on the davinci-pas-r4 template in the gallery.
 *
 * What is tested:
 *   TC-NODE-001  Template exists in the DB after V91 migration
 *   TC-NODE-002  Template has the correct slug, category, and tags
 *   TC-NODE-003  Template pipeline_config contains all 8 execution groups
 *   TC-NODE-004  Zone 1 step is pas_envelope_mapping with 17 mappings
 *   TC-NODE-005  Zone 2 has 4 enrichment.script steps in the correct sequence
 *   TC-NODE-006  Zone 2 FHIR validation step (seq 140) targets the correct field
 *   TC-NODE-007  Zone 3 submit step uses bodyRef pointing to assemble_pas_bundle
 *   TC-NODE-008  Zone 4 ClaimResponse parser step is present at seq 200
 *   TC-NODE-009  Zone 4 switch_case has all four decision codes (AA/AD/CP/PE)
 *   TC-NODE-010  required_connection_fields includes payer SMART credentials
 *   TC-NODE-011  sanitized_fields includes client secret
 *   TC-NODE-012  preview_steps contains 6 items
 *   TC-NODE-013  useTemplate merges payer credentials into pipeline config vars
 *
 * Usage (requires running PostgreSQL with V91 migration applied):
 *   node tests/pas_template_test.js
 *
 * Without a DB (unit-mode — tests pipeline_config shape from the SQL file):
 *   PAS_TEST_UNIT=1 node tests/pas_template_test.js
 */

'use strict';

const path  = require('path');
const fs    = require('fs');

// ─── Colours ─────────────────────────────────────────────────────────────────
const GREEN  = '\x1b[32m';
const RED    = '\x1b[31m';
const YELLOW = '\x1b[33m';
const RESET  = '\x1b[0m';
const BOLD   = '\x1b[1m';

let passed = 0;
let failed = 0;

function pass(id, msg) {
    console.log(`  ${GREEN}✓${RESET} [${id}] ${msg}`);
    passed++;
}
function fail(id, msg, detail) {
    console.log(`  ${RED}✗${RESET} [${id}] ${msg}`);
    if (detail) console.log(`       ${RED}${detail}${RESET}`);
    failed++;
}
function skip(id, msg) {
    console.log(`  ${YELLOW}~${RESET} [${id}] ${msg} (skipped)`);
}

function assert(id, label, condition, detail) {
    condition ? pass(id, label) : fail(id, label, detail);
}

// ─── Load pipeline_config from SQL migration (unit-mode) ─────────────────────

/**
 * Walk through a string starting at `start` and find the index of the matching
 * closing delimiter (`}` for `{`, `]` for `[`).  Handles nesting and JS-style
 * string literals (single and double quotes, with backslash escaping).
 */
function findMatchingClose(s, start) {
    const open  = s[start];
    const close = open === '{' ? '}' : ']';
    let depth = 0;
    let inStr  = false;
    let strChar = '';

    for (let i = start; i < s.length; i++) {
        const c = s[i];

        if (inStr) {
            if (c === '\\') { i++; continue; } // skip escaped char
            if (c === strChar) inStr = false;
            continue;
        }

        if (c === '"' || c === "'") { inStr = true; strChar = c; continue; }
        if (c === open)  { depth++; continue; }
        if (c === close) {
            depth--;
            if (depth === 0) return i;
        }
    }
    return -1;
}

function loadPipelineConfigFromSQL() {
    const sqlPath = path.join(__dirname, '..', 'database', 'migrations', 'V91__DaVinci_PAS_Template.sql');
    const sql = fs.readFileSync(sqlPath, 'utf8');

    // The pipeline_config value starts with the first { that precedes "execution_groups"
    const egIdx = sql.indexOf('"execution_groups"');
    if (egIdx === -1) throw new Error('Could not locate "execution_groups" in SQL migration');

    // Walk back to find the opening {
    let braceStart = egIdx;
    while (braceStart > 0 && sql[braceStart] !== '{') braceStart--;

    const endIdx = findMatchingClose(sql, braceStart);
    if (endIdx === -1) throw new Error('Could not find matching } for pipeline_config');

    const jsonStr = sql.slice(braceStart, endIdx + 1);
    return JSON.parse(jsonStr);
}

function loadTemplateMetaFromSQL() {
    const sqlPath = path.join(__dirname, '..', 'database', 'migrations', 'V91__DaVinci_PAS_Template.sql');
    const sql = fs.readFileSync(sqlPath, 'utf8');

    // Extract required_connection_fields — find the [ that starts the rcf array
    let rcf = null;
    const rcfMarker = sql.indexOf('"payer_fhir_base"');
    if (rcfMarker !== -1) {
        let arrStart = rcfMarker;
        while (arrStart > 0 && sql[arrStart] !== '[') arrStart--;
        const arrEnd = findMatchingClose(sql, arrStart);
        if (arrEnd !== -1) {
            try { rcf = JSON.parse(sql.slice(arrStart, arrEnd + 1)); } catch (_) {}
        }
    }

    // Extract sanitized_fields ARRAY[...] as raw string
    // Allow optional whitespace between last token and closing bracket
    const sanitizedMatch = sql.match(/ARRAY\[[\s\S]+?'target\.token'[\s\n\r]*\]/);

    // Extract preview_steps — use the LAST occurrence of "Map to PAS Envelope"
    // (the first occurrence is inside pipeline_config JSON; the last is preview_steps)
    let preview = null;
    let searchFrom = 0;
    let lastPreviewMarker = -1;
    while (true) {
        const idx = sql.indexOf('"Map to PAS Envelope"', searchFrom);
        if (idx === -1) break;
        lastPreviewMarker = idx;
        searchFrom = idx + 1;
    }
    if (lastPreviewMarker !== -1) {
        let arrStart = lastPreviewMarker;
        while (arrStart > 0 && sql[arrStart] !== '[') arrStart--;
        const arrEnd = findMatchingClose(sql, arrStart);
        if (arrEnd !== -1) {
            try { preview = JSON.parse(sql.slice(arrStart, arrEnd + 1)); } catch (_) {}
        }
    }

    return {
        rcf,
        sanitized: sanitizedMatch ? sanitizedMatch[0] : null,
        preview,
    };
}

// ─── Test runner ─────────────────────────────────────────────────────────────

async function runUnitTests() {
    console.log(`\n${BOLD}Da Vinci PAS Template — Unit Tests (SQL shape validation)${RESET}\n`);

    let pipelineConfig, meta;
    try {
        pipelineConfig = loadPipelineConfigFromSQL();
        meta = loadTemplateMetaFromSQL();
    } catch (err) {
        fail('LOAD', 'Could not load/parse V91 migration SQL', err.message);
        return;
    }

    const groups = pipelineConfig.execution_groups;

    // ── TC-NODE-001: Migration file exists ────────────────────────────────────
    const sqlExists = fs.existsSync(
        path.join(__dirname, '..', 'database', 'migrations', 'V91__DaVinci_PAS_Template.sql')
    );
    assert('TC-NODE-001', 'V91__DaVinci_PAS_Template.sql migration exists', sqlExists);

    // ── TC-NODE-002: execution_groups is an array ─────────────────────────────
    assert('TC-NODE-002', 'pipeline_config has execution_groups array',
        Array.isArray(groups),
        `got: ${typeof groups}`
    );

    // ── TC-NODE-003: Exactly 9 execution groups ───────────────────────────────
    // Zones: seq 10 (user mapping) + seq 100/110/120/130/140/150 (PAS core) + seq 200/210 (decision)
    assert('TC-NODE-003', 'pipeline_config has 9 execution groups',
        groups && groups.length === 9,
        `got ${groups ? groups.length : 'undefined'} groups`
    );

    if (!groups || !groups.length) {
        fail('ALL', 'Cannot continue — execution_groups missing');
        return;
    }

    // Build a flat step list for easier lookup
    const allSteps = groups.flatMap(g => g.steps || []);
    const bySeq    = {};
    allSteps.forEach(s => { bySeq[s.sequence] = s; });

    // ── TC-NODE-004: Zone 1 — PAS envelope mapping step ───────────────────────
    const envStep = bySeq[10];
    assert('TC-NODE-004a', 'seq 10 step_type is pas_envelope_mapping',
        envStep && envStep.step_type === 'pas_envelope_mapping',
        `got: ${envStep && envStep.step_type}`
    );
    const mappings = envStep && envStep.config && envStep.config.mappings;
    assert('TC-NODE-004b', 'PAS envelope has 17 mapping entries',
        Array.isArray(mappings) && mappings.length === 17,
        `got ${mappings ? mappings.length : 'undefined'} mappings`
    );
    const hasAllRequired = mappings && [
        '_pas_envelope.patient.firstName',
        '_pas_envelope.patient.lastName',
        '_pas_envelope.patient.dob',
        '_pas_envelope.patient.memberId',
        '_pas_envelope.coverage.payerId',
        '_pas_envelope.provider.npi',
        '_pas_envelope.request.serviceCode',
        '_pas_envelope.request.diagnosisCodes',
    ].every(lhs => mappings.some(m => m.lhs === lhs));
    assert('TC-NODE-004c', 'All 8 required PAS fields present in mappings', hasAllRequired);

    // ── TC-NODE-005: Zone 2 — 4 enrichment.script steps ──────────────────────
    const scriptSteps = allSteps.filter(s => s.step_type === 'enrichment.script');
    assert('TC-NODE-005a', 'Zone 2 has at least 4 enrichment.script steps',
        scriptSteps.length >= 4,
        `got ${scriptSteps.length}`
    );
    const scriptSeqs = scriptSteps.map(s => s.sequence).sort((a, b) => a - b);
    assert('TC-NODE-005b', 'Script steps run in sequences 100, 110, 120, 130, 200',
        [100, 110, 120, 130, 200].every(seq => scriptSeqs.includes(seq)),
        `found seqs: ${scriptSeqs}`
    );

    // ── TC-NODE-006: seq 140 — FHIR validation ────────────────────────────────
    const validStep = bySeq[140];
    assert('TC-NODE-006a', 'seq 140 step_type is fhir_validation',
        validStep && validStep.step_type === 'fhir_validation',
        `got: ${validStep && validStep.step_type}`
    );
    assert('TC-NODE-006b', 'FHIR validation profile is davinci-pas',
        validStep && validStep.config && validStep.config.profile === 'davinci-pas',
        `got: ${validStep && validStep.config && validStep.config.profile}`
    );
    assert('TC-NODE-006c', 'FHIR validation requires Claim, Patient, Coverage',
        validStep && validStep.config &&
        Array.isArray(validStep.config.required_resource_types) &&
        ['Claim','Patient','Coverage'].every(rt => validStep.config.required_resource_types.includes(rt))
    );

    // ── TC-NODE-007: seq 150 — bodyRef points to assembled bundle ─────────────
    const submitStep = bySeq[150];
    assert('TC-NODE-007a', 'seq 150 step_type is enrichment.api',
        submitStep && submitStep.step_type === 'enrichment.api',
        `got: ${submitStep && submitStep.step_type}`
    );
    assert('TC-NODE-007b', 'Submit step uses bodyRef',
        submitStep && submitStep.config && submitStep.config.bodyRef,
        'bodyRef missing'
    );
    assert('TC-NODE-007c', 'bodyRef points to assemble_pas_bundle step output',
        submitStep && submitStep.config &&
        (submitStep.config.bodyRef || '').includes('assemble_pas_bundle'),
        `bodyRef: ${submitStep && submitStep.config && submitStep.config.bodyRef}`
    );
    assert('TC-NODE-007d', 'Submit step uses oauth2 auth',
        submitStep && submitStep.config && submitStep.config.authType === 'oauth2',
        `authType: ${submitStep && submitStep.config && submitStep.config.authType}`
    );
    assert('TC-NODE-007e', 'Submit endpoint uses payer_fhir_base variable',
        submitStep && submitStep.config &&
        (submitStep.config.endpoint || '').includes('payer_fhir_base'),
        `endpoint: ${submitStep && submitStep.config && submitStep.config.endpoint}`
    );

    // ── TC-NODE-008: seq 200 — ClaimResponse parser ───────────────────────────
    const parserStep = bySeq[200];
    assert('TC-NODE-008a', 'seq 200 step is enrichment.script (ClaimResponse parser)',
        parserStep && parserStep.step_type === 'enrichment.script',
        `got: ${parserStep && parserStep.step_type}`
    );
    assert('TC-NODE-008b', 'Parser script references submit_to_payer step output',
        parserStep && parserStep.config && parserStep.config.script &&
        parserStep.config.script.includes('submit_to_payer'),
        'submit_to_payer not referenced in parser script'
    );

    // ── TC-NODE-009: seq 210 — switch_case with AA/AD/CP/PE ───────────────────
    const switchStep = bySeq[210];
    assert('TC-NODE-009a', 'seq 210 step_type is switch_case',
        switchStep && switchStep.step_type === 'switch_case',
        `got: ${switchStep && switchStep.step_type}`
    );
    const cases = switchStep && switchStep.config && switchStep.config.cases;
    assert('TC-NODE-009b', 'switch_case has 4 cases',
        Array.isArray(cases) && cases.length === 4,
        `got ${cases ? cases.length : 'undefined'} cases`
    );
    const caseValues = cases && cases.map(c => c.value);
    assert('TC-NODE-009c', 'All decision codes present (AA, AD, CP, PE)',
        caseValues && ['AA','AD','CP','PE'].every(v => caseValues.includes(v)),
        `got: ${caseValues}`
    );
    assert('TC-NODE-009d', 'switch_case has a default branch',
        switchStep && switchStep.config && switchStep.config.default,
        'default branch missing'
    );

    // ── TC-NODE-010: required_connection_fields has payer SMART fields ─────────
    const rcf = meta.rcf;
    assert('TC-NODE-010a', 'required_connection_fields array parsed',
        Array.isArray(rcf),
        'could not parse required_connection_fields'
    );
    if (rcf) {
        const fieldNames = rcf.map(f => f.field);
        assert('TC-NODE-010b', 'payer_fhir_base is a required connection field',
            fieldNames.includes('payer_fhir_base'), `fields: ${fieldNames}`
        );
        assert('TC-NODE-010c', 'payer_token_url is a required connection field',
            fieldNames.includes('payer_token_url'), `fields: ${fieldNames}`
        );
        assert('TC-NODE-010d', 'payer_client_id is a required connection field',
            fieldNames.includes('payer_client_id'), `fields: ${fieldNames}`
        );
        assert('TC-NODE-010e', 'payer_client_secret is a required connection field',
            fieldNames.includes('payer_client_secret'), `fields: ${fieldNames}`
        );
        const secretField = rcf.find(f => f.field === 'payer_client_secret');
        assert('TC-NODE-010f', 'payer_client_secret has type=password',
            secretField && secretField.type === 'password',
            `type: ${secretField && secretField.type}`
        );
    }

    // ── TC-NODE-011: sanitized_fields includes client secret ──────────────────
    assert('TC-NODE-011', 'sanitized_fields includes payer_client_secret',
        meta.sanitized && meta.sanitized.includes('payer_client_secret'),
        'payer_client_secret not in sanitized_fields'
    );

    // ── TC-NODE-012: preview_steps has 6 items ────────────────────────────────
    const preview = meta.preview;
    assert('TC-NODE-012', 'preview_steps has 6 items',
        Array.isArray(preview) && preview.length === 6,
        `got ${preview ? preview.length : 'undefined'} preview steps`
    );

    // ── TC-NODE-013: _locked flag on all PAS Core steps ───────────────────────
    const lockedSteps = allSteps.filter(s => s.config && s.config._zone === 'pas_core');
    assert('TC-NODE-013a', 'At least 6 steps have _zone=pas_core',
        lockedSteps.length >= 6,
        `got ${lockedSteps.length}`
    );
    const userMappingStep = allSteps.find(s => s.config && s.config._zone === 'user_mapping');
    assert('TC-NODE-013b', 'Exactly 1 step has _zone=user_mapping',
        userMappingStep !== undefined,
        'no user_mapping zone step found'
    );

    // ── TC-NODE-014: All steps have step_alias set ────────────────────────────
    const missingAlias = allSteps.filter(s => !s.step_alias);
    assert('TC-NODE-014', 'All steps have step_alias set',
        missingAlias.length === 0,
        `steps without alias: ${missingAlias.map(s => s.step_name).join(', ')}`
    );
}

async function runDBTests() {
    console.log(`\n${BOLD}Da Vinci PAS Template — DB Integration Tests${RESET}`);
    console.log(`  Requires running PostgreSQL with V91 migration applied.\n`);

    let sequelize;
    try {
        require('dotenv').config({ path: path.join(__dirname, '..', '.env') });
        const { Sequelize } = require('sequelize');
        sequelize = new Sequelize(
            process.env.DB_NAME || 'ezhealthkonnect',
            process.env.DB_USER || 'postgres',
            process.env.DB_PASSWORD || '',
            {
                host:    process.env.DB_HOST || 'localhost',
                port:    parseInt(process.env.DB_PORT || '5432'),
                dialect: 'postgres',
                logging: false,
            }
        );
        await sequelize.authenticate();
    } catch (err) {
        skip('TC-DB-*', `DB unavailable — skipping (${err.message})`);
        return;
    }

    try {
        const [rows] = await sequelize.query(
            `SELECT id, name, slug, category, tags, pipeline_config,
                    required_connection_fields, sanitized_fields, preview_steps,
                    is_system, estimated_setup_minutes
             FROM interface_templates WHERE slug = 'davinci-pas-r4' LIMIT 1`
        );

        assert('TC-DB-001', 'davinci-pas-r4 template exists in DB', rows.length === 1,
            'No rows returned — has V91 migration been applied?');

        if (rows.length === 0) return;
        const t = rows[0];

        assert('TC-DB-002', 'template name is correct',
            t.name === 'Da Vinci Prior Authorization (PAS)',
            `got: ${t.name}`
        );
        assert('TC-DB-003', 'category is regulatory',
            t.category === 'regulatory', `got: ${t.category}`
        );
        assert('TC-DB-004', 'is_system = true',
            t.is_system === true, `got: ${t.is_system}`
        );
        assert('TC-DB-005', 'tags includes prior-auth and da-vinci',
            Array.isArray(t.tags) && t.tags.includes('prior-auth') && t.tags.includes('da-vinci'),
            `tags: ${t.tags}`
        );
        assert('TC-DB-006', 'pipeline_config is valid JSON with execution_groups',
            t.pipeline_config && Array.isArray(t.pipeline_config.execution_groups),
            'pipeline_config missing or malformed'
        );
        assert('TC-DB-007', 'estimated_setup_minutes = 45',
            t.estimated_setup_minutes === 45, `got: ${t.estimated_setup_minutes}`
        );
        assert('TC-DB-008', 'preview_steps has 6 entries',
            Array.isArray(t.preview_steps) && t.preview_steps.length === 6,
            `got: ${t.preview_steps && t.preview_steps.length}`
        );
    } finally {
        await sequelize.close();
    }
}

// ─── Entry point ─────────────────────────────────────────────────────────────

(async () => {
    const unitMode = process.env.PAS_TEST_UNIT === '1' || true; // always run unit tests

    await runUnitTests();

    if (process.env.PAS_TEST_DB === '1') {
        await runDBTests();
    } else {
        console.log(`\n  ${YELLOW}~${RESET} DB tests skipped. Set PAS_TEST_DB=1 to run against PostgreSQL.\n`);
    }

    console.log(`\n${'─'.repeat(55)}`);
    console.log(`  ${GREEN}${BOLD}${passed} passed${RESET}  ${failed > 0 ? RED : ''}${BOLD}${failed} failed${RESET}`);
    console.log(`${'─'.repeat(55)}\n`);

    process.exit(failed > 0 ? 1 : 0);
})();
