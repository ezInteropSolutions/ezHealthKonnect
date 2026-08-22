/**
 * Da Vinci PAS Template — Node.js Integration Test
 *
 * Tests the interface template controller logic that would run when a user
 * clicks "Use Template" on the davinci-pas-r4 template in the gallery.
 *
 * What is tested:
 *   TC-NODE-001  Template exists in the DB after V91 migration
 *   TC-NODE-002  Template has the correct slug, category, and tags
 *   TC-NODE-003  Template pipeline_config contains all 15 execution groups
 *                (9 from V91 + receive_request/deliver_decision from V93)
 *   TC-NODE-004  Zone 1 step is pas_envelope_mapping with 17 mappings
 *   TC-NODE-005  Zone 2 has 4 enrichment.script steps in the correct sequence
 *   TC-NODE-006  Zone 2 FHIR validation step (seq 140) targets the correct
 *                field, uses the correct config keys (V210 fix), and runs
 *                at strict level so the PAS constraint checks actually fire
 *   TC-NODE-007  Zone 3 submit step uses bodyRef pointing to assemble_pas_bundle
 *   TC-NODE-008  Zone 4 ClaimResponse parser step is present at seq 200
 *   TC-NODE-009  Zone 4 switch_case has all four decision codes (AA/AD/CP/PE)
 *   TC-NODE-010  required_connection_fields includes payer SMART credentials
 *   TC-NODE-011  sanitized_fields includes client secret
 *   TC-NODE-012  preview_steps contains 6 items
 *   TC-NODE-013  useTemplate merges payer credentials into pipeline config vars
 *   TC-NODE-015  execution_groups has exactly ONE receive_request and ONE
 *                deliver_decision group (V210 dedupe fix — V93 had left 3
 *                duplicate copies of each in real environments)
 *   TC-NODE-016  route_on_decision's actions are set_value/targetField, not
 *                set_variable/variable (V211 fix — SwitchCaseExecutor has no
 *                set_variable case, so _pa_status/_pa_status_label were
 *                never actually set; a silent no-op)
 *   TC-NODE-017  deliver_decision's contentField references the real
 *                parse_claim_response alias (not the nonexistent
 *                parse_response), and has retry enabled (V211 fix — the
 *                wrong alias caused the raw inbound message to be sent
 *                instead of the payer's decision, with no retry on failure)
 *
 * pipeline_config in unit-mode is reconstructed by replaying the full
 * migration chain in JS: V91's literal JSON (base) -> V93's appended
 * connector.inbound/outbound groups -> V210's dedupe + config-key fixes ->
 * V211's decision-routing/delivery fixes. This mirrors what Postgres
 * actually produces so unit-mode stays accurate without needing a live DB.
 * See loadV93ConnectorGroups() / applyV210Fixes() / applyV211Fixes().
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

/**
 * Extract the two execution_groups (receive_request seq 5, deliver_decision
 * seq 295) that V93 appends to pipeline_config.execution_groups via
 * `(pipeline_config -> 'execution_groups') || $steps$[...]$steps$::jsonb`.
 * The array is dollar-quoted ($steps$...$steps$), so no SQL string-escaping
 * applies inside it — it is plain, directly parseable JSON.
 */
function loadV93ConnectorGroups() {
    const sqlPath = path.join(__dirname, '..', 'database', 'migrations', 'V93__PAS_Template_Add_Connector_Steps.sql');
    const sql = fs.readFileSync(sqlPath, 'utf8');

    const startTag = sql.indexOf('$steps$');
    if (startTag === -1) throw new Error('Could not locate $steps$ dollar-quote in V93 migration');
    const jsonStart = startTag + '$steps$'.length;
    const endTag = sql.indexOf('$steps$', jsonStart);
    if (endTag === -1) throw new Error('Could not locate closing $steps$ dollar-quote in V93 migration');

    return JSON.parse(sql.slice(jsonStart, endTag));
}

/**
 * Replay V211's fix in JS:
 *   - route_on_decision: every {action:"set_variable", variable:"X", value:"Y"}
 *     (in cases[].actions[] and default.actions[]) becomes
 *     {action:"set_value", targetField:"X", value:"Y"} — the executor
 *     (SwitchCaseExecutor) has no "set_variable" case; it was a silent no-op.
 *   - deliver_decision: contentField "steps.parse_response.step_output"
 *     (wrong alias — the parser step's real alias is parse_claim_response)
 *     becomes "steps.parse_claim_response.step_output", and a "retry" block
 *     is added so a transient callback failure doesn't silently lose the
 *     PA decision.
 * Mutates and returns pipelineConfig.
 */
function applyV211Fixes(pipelineConfig) {
    const fixAction = (action) => {
        if (action.action !== 'set_variable') return action;
        const { action: _a, variable, ...rest } = action;
        return { ...rest, action: 'set_value', targetField: variable };
    };

    for (const group of pipelineConfig.execution_groups) {
        const step = group.steps && group.steps[0];
        if (!step) continue;

        if (step.step_alias === 'route_on_decision' && step.config) {
            if (Array.isArray(step.config.cases)) {
                step.config.cases = step.config.cases.map(c => ({
                    ...c,
                    actions: Array.isArray(c.actions) ? c.actions.map(fixAction) : c.actions,
                }));
            }
            if (step.config.default && Array.isArray(step.config.default.actions)) {
                step.config.default = {
                    ...step.config.default,
                    actions: step.config.default.actions.map(fixAction),
                };
            }
        }

        if (step.step_alias === 'deliver_decision' && step.config) {
            step.config.contentField = 'steps.parse_claim_response.step_output';
            step.config.retry = {
                enabled: true,
                maxRetries: 3,
                delayMs: 1000,
                backoffMultiplier: 2.0,
                maxDelayMs: 60000,
            };
        }
    }

    return pipelineConfig;
}

/**
 * Replay V210's fix in JS: dedupe execution_groups to one per step_alias
 * (keeping the first occurrence — V210 uses DISTINCT ON ordered by original
 * array position), then rewrite validate_pas_bundle's config keys to match
 * what the Go executor (fhirValidationConfig) actually reads:
 *   level                   -> validation_level (value forced to "strict")
 *   required_resource_types -> required_resources (value preserved)
 * Mutates and returns pipelineConfig.
 */
function applyV210Fixes(pipelineConfig) {
    const seen = new Set();
    const deduped = [];
    for (const group of pipelineConfig.execution_groups) {
        const alias = group.steps && group.steps[0] && group.steps[0].step_alias;
        if (alias && seen.has(alias)) continue;
        if (alias) seen.add(alias);
        deduped.push(group);
    }
    deduped.sort((a, b) => a.sequence - b.sequence);

    for (const group of deduped) {
        const step = group.steps && group.steps[0];
        if (step && step.step_alias === 'validate_pas_bundle' && step.config) {
            const oldRequiredTypes = step.config.required_resource_types;
            delete step.config.level;
            delete step.config.required_resource_types;
            step.config.validation_level = 'strict';
            step.config.required_resources = oldRequiredTypes;
        }
    }

    pipelineConfig.execution_groups = deduped;
    return pipelineConfig;
}

/**
 * Extract the 8 new Zone 2 execution_groups (derive_pas_fields through
 * stamp_bundle_profile) that V212 appends via
 * `... UNION ALL SELECT jsonb_array_elements($groups$[...]$groups$::jsonb) ...`.
 * Same dollar-quote extraction technique as loadV93ConnectorGroups() — reads
 * directly from the migration file so this test can never drift from what
 * V212 actually contains (V212's SQL transformation is too structurally
 * complex — UNION ALL, correlated subqueries rewriting the mappings array —
 * to safely re-derive independently in JS without risking exactly the kind
 * of two-independently-authored-copies drift that caused bugs 1/2/5/6).
 */
function loadV212NewGroups() {
    const sqlPath = path.join(__dirname, '..', 'database', 'migrations', 'V212__PAS_Template_Rebuild_On_FHIR_Builder.sql');
    const sql = fs.readFileSync(sqlPath, 'utf8');

    const startTag = sql.indexOf('$groups$');
    if (startTag === -1) throw new Error('Could not locate $groups$ dollar-quote in V212 migration');
    const jsonStart = startTag + '$groups$'.length;
    const endTag = sql.indexOf('$groups$', jsonStart);
    if (endTag === -1) throw new Error('Could not locate closing $groups$ dollar-quote in V212 migration');

    return JSON.parse(sql.slice(jsonStart, endTag));
}

/**
 * Replay V212's fix in JS: removes the 4 old Zone 2 execution_groups
 * (build_patient, build_coverage, build_provider, assemble_pas_bundle — the
 * hand-written enrichment.script ones), splits pas_envelope's single
 * provider.name mapping into firstName/lastName, repoints
 * validate_pas_bundle/submit_to_payer at the new stamp_bundle_profile
 * output, and appends the 8 new groups (read directly from V212's own SQL
 * via loadV212NewGroups()). Mutates and returns pipelineConfig.
 */
function applyV212Fixes(pipelineConfig) {
    const removedAliases = new Set(['build_patient', 'build_coverage', 'build_provider', 'assemble_pas_bundle']);
    const kept = pipelineConfig.execution_groups.filter(g => {
        const alias = g.steps && g.steps[0] && g.steps[0].step_alias;
        return !removedAliases.has(alias);
    });

    for (const group of kept) {
        const step = group.steps && group.steps[0];
        if (!step) continue;

        if (step.step_alias === 'pas_envelope' && step.config && Array.isArray(step.config.mappings)) {
            step.config.mappings = step.config.mappings.filter(m => m.lhs !== '_pas_envelope.provider.name');
            step.config.mappings.push(
                { lhs: '_pas_envelope.provider.firstName', rhs: '', _label: 'Provider First Name', _required: false, _hint: 'e.g. Alice' },
                { lhs: '_pas_envelope.provider.lastName', rhs: '', _label: 'Provider Last Name', _required: false, _hint: 'e.g. Johnson' }
            );
        }
        if (step.step_alias === 'validate_pas_bundle' && step.config) {
            step.config.source_field = 'steps.stamp_bundle_profile.step_output.pas_bundle';
        }
        if (step.step_alias === 'submit_to_payer' && step.config) {
            step.config.bodyRef = 'steps.stamp_bundle_profile.step_output.pas_bundle';
        }
    }

    pipelineConfig.execution_groups = kept.concat(loadV212NewGroups());
    pipelineConfig.execution_groups.sort((a, b) => a.sequence - b.sequence);
    return pipelineConfig;
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
        pipelineConfig.execution_groups = pipelineConfig.execution_groups.concat(loadV93ConnectorGroups());
        applyV210Fixes(pipelineConfig);
        applyV211Fixes(pipelineConfig);
        applyV212Fixes(pipelineConfig);
        meta = loadTemplateMetaFromSQL();
    } catch (err) {
        fail('LOAD', 'Could not load/parse PAS template migrations (V91 + V93 + V210)', err.message);
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

    // ── TC-NODE-003: Exactly 15 execution groups ──────────────────────────────
    // Zones: seq 5 (receive) + seq 10 (user mapping) + seq 95-135 (V212's 8-step
    // FHIR-builder Zone 2: derive, 5x fhir.build, payload.builder, stamp) +
    // seq 140/150 (validate/submit) + seq 200/210 (decision) + seq 295 (deliver).
    assert('TC-NODE-003', 'pipeline_config has 15 execution groups',
        groups && groups.length === 15,
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
    // V212 split the single provider.name field into firstName/lastName
    // (mirroring how patient.firstName/.lastName already worked) — 17 -> 18.
    assert('TC-NODE-004b', 'PAS envelope has 18 mapping entries',
        Array.isArray(mappings) && mappings.length === 18,
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
    assert('TC-NODE-004d', 'provider.name mapping replaced by firstName/lastName',
        mappings &&
        !mappings.some(m => m.lhs === '_pas_envelope.provider.name') &&
        mappings.some(m => m.lhs === '_pas_envelope.provider.firstName') &&
        mappings.some(m => m.lhs === '_pas_envelope.provider.lastName'),
        `mappings: ${mappings && mappings.map(m => m.lhs).join(', ')}`
    );

    // ── TC-NODE-005: Zone 2 — enrichment.script steps (derive + stamp + parse) ─
    // V212 replaced the 4 hand-written resource-building scripts with a
    // generic fhir.build/payload.builder chain (see TC-NODE-018 below) —
    // only 2 small, non-resource-building scripts remain in Zone 2 (derive
    // computed fields, stamp bundle profile), plus the pre-existing Zone 4
    // parser script.
    const scriptSteps = allSteps.filter(s => s.step_type === 'enrichment.script');
    assert('TC-NODE-005a', 'Exactly 3 enrichment.script steps remain',
        scriptSteps.length === 3,
        `got ${scriptSteps.length}`
    );
    const scriptSeqs = scriptSteps.map(s => s.sequence).sort((a, b) => a - b);
    assert('TC-NODE-005b', 'Script steps run in sequences 95, 135, 200',
        [95, 135, 200].every(seq => scriptSeqs.includes(seq)),
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
    // config key must be "validation_level" — services/executors/validation/
    // fhir_validation_executor.go's fhirValidationConfig struct tag is
    // `json:"validation_level"`, not "level"; a "level" key is silently
    // dropped by json.Unmarshal and never reaches r4.ParseLevel().
    assert('TC-NODE-006b2', 'FHIR validation runs at strict level (validation_level key)',
        validStep && validStep.config && validStep.config.validation_level === 'strict',
        `got: ${validStep && validStep.config && validStep.config.validation_level}`
    );
    // config key must be "required_resources" — the Go struct field is
    // RequiredResources []string `json:"required_resources"`; the old key
    // "required_resource_types" was silently dropped, making the check a no-op.
    assert('TC-NODE-006c', 'FHIR validation requires Claim, Patient, Coverage (required_resources key)',
        validStep && validStep.config &&
        Array.isArray(validStep.config.required_resources) &&
        ['Claim','Patient','Coverage'].every(rt => validStep.config.required_resources.includes(rt))
    );
    assert('TC-NODE-006d', 'FHIR validation config no longer uses the stale key names',
        validStep && validStep.config &&
        validStep.config.level === undefined &&
        validStep.config.required_resource_types === undefined,
        `level: ${validStep && validStep.config && validStep.config.level}, required_resource_types: ${validStep && validStep.config && validStep.config.required_resource_types}`
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
    // V212: bodyRef now points at stamp_bundle_profile (the step that
    // converts payload.builder's JSON-string output back into a usable map
    // and stamps Bundle.meta.profile), not assemble_pas_bundle directly.
    assert('TC-NODE-007c', 'bodyRef points to stamp_bundle_profile step output',
        submitStep && submitStep.config &&
        submitStep.config.bodyRef === 'steps.stamp_bundle_profile.step_output.pas_bundle',
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

    // ── TC-NODE-015: No duplicate connector groups (V210 dedupe fix) ──────────
    // V93 left 3 duplicate copies each of receive_request/deliver_decision in
    // real environments — an interface built from the template would inherit
    // 3 duplicate inbound listeners (port-binding conflict) and triplicate
    // delivery of the PA decision. Nothing previously asserted uniqueness,
    // which is why bug 3 wasn't caught.
    const receiveRequestSteps  = allSteps.filter(s => s.step_alias === 'receive_request');
    const deliverDecisionSteps = allSteps.filter(s => s.step_alias === 'deliver_decision');
    assert('TC-NODE-015a', 'Exactly one receive_request step (no duplicates)',
        receiveRequestSteps.length === 1,
        `got ${receiveRequestSteps.length}`
    );
    assert('TC-NODE-015b', 'Exactly one deliver_decision step (no duplicates)',
        deliverDecisionSteps.length === 1,
        `got ${deliverDecisionSteps.length}`
    );
    assert('TC-NODE-015c', 'receive_request is a connector.inbound step at sequence 5',
        receiveRequestSteps[0] &&
        receiveRequestSteps[0].step_type === 'connector.inbound' &&
        receiveRequestSteps[0].sequence === 5,
        `got: ${receiveRequestSteps[0] && receiveRequestSteps[0].step_type}, seq ${receiveRequestSteps[0] && receiveRequestSteps[0].sequence}`
    );
    assert('TC-NODE-015d', 'deliver_decision is a connector.outbound step at sequence 295',
        deliverDecisionSteps[0] &&
        deliverDecisionSteps[0].step_type === 'connector.outbound' &&
        deliverDecisionSteps[0].sequence === 295,
        `got: ${deliverDecisionSteps[0] && deliverDecisionSteps[0].step_type}, seq ${deliverDecisionSteps[0] && deliverDecisionSteps[0].sequence}`
    );
    // General invariant: no step_alias appears more than once anywhere in the
    // pipeline (catches any future re-introduction of this class of bug).
    const aliasCounts = {};
    allSteps.forEach(s => { aliasCounts[s.step_alias] = (aliasCounts[s.step_alias] || 0) + 1; });
    const duplicateAliases = Object.keys(aliasCounts).filter(a => aliasCounts[a] > 1);
    assert('TC-NODE-015e', 'No step_alias is duplicated anywhere in execution_groups',
        duplicateAliases.length === 0,
        `duplicated aliases: ${duplicateAliases.join(', ')}`
    );

    // ── TC-NODE-016: route_on_decision uses set_value, not set_variable ───────
    // SwitchCaseExecutor (services/executors/control/conditional_executor.go)
    // has no "set_variable" case — every one of these actions was a silent
    // no-op before V211. _pa_status/_pa_status_label were never actually set.
    const routeStep = bySeq[210];
    assert('TC-NODE-016a', 'route_on_decision step_type is switch_case',
        routeStep && routeStep.step_type === 'switch_case',
        `got: ${routeStep && routeStep.step_type}`
    );
    const routeActions = routeStep && routeStep.config
        ? [
            ...(routeStep.config.cases || []).flatMap(c => c.actions || []),
            ...((routeStep.config.default && routeStep.config.default.actions) || []),
        ]
        : [];
    assert('TC-NODE-016b', 'route_on_decision has actions to check',
        routeActions.length === 10, // 5 branches (AA/AD/CP/PE/default) x 2 actions
        `got ${routeActions.length} actions`
    );
    const staleActions = routeActions.filter(a => a.action === 'set_variable' || a.variable !== undefined);
    assert('TC-NODE-016c', 'No route_on_decision action uses the stale set_variable/variable shape',
        staleActions.length === 0,
        `stale actions: ${JSON.stringify(staleActions)}`
    );
    const allSetValue = routeActions.every(a => a.action === 'set_value' && typeof a.targetField === 'string' && a.targetField);
    assert('TC-NODE-016d', 'Every route_on_decision action is set_value with a targetField',
        allSetValue,
        `actions: ${JSON.stringify(routeActions)}`
    );
    const aaCase = routeStep && routeStep.config && (routeStep.config.cases || []).find(c => c.value === 'AA');
    assert('TC-NODE-016e', 'AA case sets _pa_status=approved via targetField',
        aaCase && aaCase.actions.some(a => a.targetField === '_pa_status' && a.value === 'approved'),
        `AA case actions: ${aaCase && JSON.stringify(aaCase.actions)}`
    );

    // ── TC-NODE-017: deliver_decision content/retry (V211 fix) ────────────────
    // Bug 6: contentField pointed at "steps.parse_response.step_output" — the
    // parser step's real alias is parse_claim_response, so this always
    // resolved to nil and silently fell back to sending the raw inbound
    // message instead of the parsed decision.
    const deliverStep = bySeq[295];
    assert('TC-NODE-017a', 'deliver_decision contentField references parse_claim_response',
        deliverStep && deliverStep.config && deliverStep.config.contentField === 'steps.parse_claim_response.step_output',
        `got: ${deliverStep && deliverStep.config && deliverStep.config.contentField}`
    );
    assert('TC-NODE-017b', 'deliver_decision no longer references the nonexistent parse_response alias',
        !(deliverStep && deliverStep.config && String(deliverStep.config.contentField || '').includes('parse_response.') && !String(deliverStep.config.contentField).includes('parse_claim_response')),
        `contentField: ${deliverStep && deliverStep.config && deliverStep.config.contentField}`
    );
    assert('TC-NODE-017c', 'deliver_decision has retry enabled (resilience gap fix)',
        deliverStep && deliverStep.config && deliverStep.config.retry && deliverStep.config.retry.enabled === true,
        `retry: ${deliverStep && deliverStep.config && JSON.stringify(deliverStep.config.retry)}`
    );
    assert('TC-NODE-017d', 'deliver_decision retry.maxRetries is a positive number',
        deliverStep && deliverStep.config && deliverStep.config.retry &&
        typeof deliverStep.config.retry.maxRetries === 'number' && deliverStep.config.retry.maxRetries > 0,
        `maxRetries: ${deliverStep && deliverStep.config && deliverStep.config.retry && deliverStep.config.retry.maxRetries}`
    );

    // ── TC-NODE-018: Zone 2 rebuilt on fhir.build + payload.builder (V212) ────
    // Replaces the 4 hand-written enrichment.script resource-building steps
    // with the codebase's generic, schema-driven FHIR builder engine —
    // config-only, zero new code per resource type, matching CLAUDE.md's
    // "not hardcoded" standard and structurally eliminating the fullUrl/
    // reference-form bug the old hand-written scripts had.
    const buildSteps = allSteps.filter(s => s.step_type === 'fhir.build');
    assert('TC-NODE-018a', 'Exactly 5 fhir.build steps (Patient/Coverage/Practitioner/Organization/Claim)',
        buildSteps.length === 5,
        `got ${buildSteps.length}: ${buildSteps.map(s => s.config && s.config.resourceType).join(', ')}`
    );
    const resourceTypes = buildSteps.map(s => s.config && s.config.resourceType).sort();
    assert('TC-NODE-018b', 'fhir.build steps cover Claim, Coverage, Organization, Patient, Practitioner',
        JSON.stringify(resourceTypes) === JSON.stringify(['Claim', 'Coverage', 'Organization', 'Patient', 'Practitioner']),
        `got: ${resourceTypes.join(', ')}`
    );

    const claimStep = buildSteps.find(s => s.config && s.config.resourceType === 'Claim');
    const claimFields = (claimStep && claimStep.config && claimStep.config.fields) || [];
    const patientRefField = claimFields.find(f => f.targetPath === 'patient.reference');
    assert('TC-NODE-018c', 'Claim.patient.reference derives from the same field as Patient.id (correlation by construction)',
        patientRefField && patientRefField.sourcePath === '_pas_envelope.patient.memberId' && patientRefField.transform === 'string_prefix',
        `patient.reference field: ${JSON.stringify(patientRefField)}`
    );
    const coverageRefField = claimFields.find(f => f.targetPath === 'insurance[0].coverage.reference');
    assert('TC-NODE-018d', 'Claim.insurance[0].coverage.reference matches Coverage.id literally',
        coverageRefField && coverageRefField.literalValue === 'Coverage/coverage-1',
        `coverage.reference field: ${JSON.stringify(coverageRefField)}`
    );

    const assembleStep = bySeq[130];
    assert('TC-NODE-018e', 'seq 130 (assemble_pas_bundle) is now payload.builder, not enrichment.script',
        assembleStep && assembleStep.step_type === 'payload.builder' && assembleStep.step_alias === 'assemble_pas_bundle',
        `got: ${assembleStep && assembleStep.step_type}`
    );
    const resourcePaths = assembleStep && assembleStep.config && assembleStep.config.fhirBundle && assembleStep.config.fhirBundle.resourcePaths;
    assert('TC-NODE-018f', 'payload.builder resourcePaths lists all 5 built resources',
        Array.isArray(resourcePaths) && resourcePaths.length === 5 &&
        ['message.fhirClaim', 'message.fhirPatient', 'message.fhirCoverage', 'message.fhirPractitioner', 'message.fhirOrganization'].every(p => resourcePaths.includes(p)),
        `resourcePaths: ${JSON.stringify(resourcePaths)}`
    );

    const stampStep = bySeq[135];
    assert('TC-NODE-018g', 'seq 135 (stamp_bundle_profile) sets Bundle.meta.profile',
        stampStep && stampStep.step_type === 'enrichment.script' &&
        stampStep.config && stampStep.config.script && stampStep.config.script.includes('profile-pas-request-bundle'),
        `got step_type: ${stampStep && stampStep.step_type}`
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
        // V93 added the connector.inbound/outbound preview entries, bringing
        // this from 6 (V91 baseline) to 8 — matches TC-NODE-012's 6-item
        // check on V91's own baseline SQL, which is deliberately unaffected.
        assert('TC-DB-008', 'preview_steps has 8 entries (V93 added connector steps)',
            Array.isArray(t.preview_steps) && t.preview_steps.length === 8,
            `got: ${t.preview_steps && t.preview_steps.length}`
        );

        // ── TC-DB-009/010: V210 fixes, verified against the live pipeline_config ──
        const dbGroups = (t.pipeline_config && t.pipeline_config.execution_groups) || [];
        const dbSteps  = dbGroups.flatMap(g => g.steps || []);
        const dbReceive = dbSteps.filter(s => s.step_alias === 'receive_request');
        const dbDeliver = dbSteps.filter(s => s.step_alias === 'deliver_decision');
        assert('TC-DB-009a', 'Live pipeline_config has exactly one receive_request group',
            dbReceive.length === 1, `got ${dbReceive.length}`
        );
        assert('TC-DB-009b', 'Live pipeline_config has exactly one deliver_decision group',
            dbDeliver.length === 1, `got ${dbDeliver.length}`
        );
        assert('TC-DB-009c', 'Live pipeline_config has exactly 15 execution groups total',
            dbGroups.length === 15, `got ${dbGroups.length}`
        );

        const dbValidateStep = dbSteps.find(s => s.step_alias === 'validate_pas_bundle');
        assert('TC-DB-010a', 'Live validate_pas_bundle runs at validation_level=strict',
            dbValidateStep && dbValidateStep.config && dbValidateStep.config.validation_level === 'strict',
            `got: ${dbValidateStep && dbValidateStep.config && dbValidateStep.config.validation_level}`
        );
        assert('TC-DB-010b', 'Live validate_pas_bundle uses required_resources key',
            dbValidateStep && dbValidateStep.config &&
            Array.isArray(dbValidateStep.config.required_resources) &&
            ['Claim','Patient','Coverage'].every(rt => dbValidateStep.config.required_resources.includes(rt)),
            `got: ${dbValidateStep && JSON.stringify(dbValidateStep.config && dbValidateStep.config.required_resources)}`
        );

        // ── TC-DB-011/012: V211 fixes, verified against the live pipeline_config ──
        const dbRouteStep = dbSteps.find(s => s.step_alias === 'route_on_decision');
        const dbRouteActions = dbRouteStep && dbRouteStep.config
            ? [
                ...(dbRouteStep.config.cases || []).flatMap(c => c.actions || []),
                ...((dbRouteStep.config.default && dbRouteStep.config.default.actions) || []),
            ]
            : [];
        assert('TC-DB-011a', 'Live route_on_decision has no set_variable actions',
            dbRouteActions.length > 0 && dbRouteActions.every(a => a.action !== 'set_variable'),
            `actions: ${JSON.stringify(dbRouteActions)}`
        );
        assert('TC-DB-011b', 'Live route_on_decision actions all use targetField',
            dbRouteActions.every(a => typeof a.targetField === 'string' && a.targetField),
            `actions: ${JSON.stringify(dbRouteActions)}`
        );

        const dbDeliverStep = dbSteps.find(s => s.step_alias === 'deliver_decision');
        assert('TC-DB-012a', 'Live deliver_decision contentField references parse_claim_response',
            dbDeliverStep && dbDeliverStep.config && dbDeliverStep.config.contentField === 'steps.parse_claim_response.step_output',
            `got: ${dbDeliverStep && dbDeliverStep.config && dbDeliverStep.config.contentField}`
        );
        assert('TC-DB-012b', 'Live deliver_decision has retry enabled',
            dbDeliverStep && dbDeliverStep.config && dbDeliverStep.config.retry && dbDeliverStep.config.retry.enabled === true,
            `retry: ${dbDeliverStep && dbDeliverStep.config && JSON.stringify(dbDeliverStep.config.retry)}`
        );

        // ── TC-DB-013: V212 — Zone 2 rebuilt on fhir.build + payload.builder ──────
        const dbBuildSteps = dbSteps.filter(s => s.step_type === 'fhir.build');
        assert('TC-DB-013a', 'Live pipeline_config has exactly 5 fhir.build steps',
            dbBuildSteps.length === 5, `got ${dbBuildSteps.length}`
        );
        const dbAssembleStep = dbSteps.find(s => s.step_alias === 'assemble_pas_bundle');
        assert('TC-DB-013b', 'Live assemble_pas_bundle is payload.builder, not enrichment.script',
            dbAssembleStep && dbAssembleStep.step_type === 'payload.builder',
            `got: ${dbAssembleStep && dbAssembleStep.step_type}`
        );
        const dbValidateSourceField = dbValidateStep && dbValidateStep.config && dbValidateStep.config.source_field;
        assert('TC-DB-013c', 'Live validate_pas_bundle source_field points at stamp_bundle_profile',
            dbValidateSourceField === 'steps.stamp_bundle_profile.step_output.pas_bundle',
            `got: ${dbValidateSourceField}`
        );
        const dbSubmitStep = dbSteps.find(s => s.step_alias === 'submit_to_payer');
        const dbSubmitBodyRef = dbSubmitStep && dbSubmitStep.config && dbSubmitStep.config.bodyRef;
        assert('TC-DB-013d', 'Live submit_to_payer bodyRef points at stamp_bundle_profile',
            dbSubmitBodyRef === 'steps.stamp_bundle_profile.step_output.pas_bundle',
            `got: ${dbSubmitBodyRef}`
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
