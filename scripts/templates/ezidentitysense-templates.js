#!/usr/bin/env node
// scripts/templates/ezidentitysense-templates.js
//
// Generates database/migrations/V290__EzIdentitySense_Templates.sql: the
// ready-to-use interface templates that connect ezHealthKonnect to
// ezIdentitySense (the EMPI) through its public API only. Templates are
// data (interface_templates rows); this script exists so the pipeline
// configs and guides are written as plain JS objects and Markdown instead
// of being hand-escaped into SQL string literals (the same reason V274's
// own generator existed).
//
//   node scripts/templates/ezidentitysense-templates.js > database/migrations/V290__EzIdentitySense_Templates.sql
//
// Every credential ships blank: the "Use template" form asks for it
// (required_connection_fields; section "steps" writes into the named API
// steps), and the pipeline save encrypts secrets.
'use strict';

const fs = require('fs');
const path = require('path');

const GUIDES = path.join(__dirname, 'ezidentitysense-guides');

// ── shared pieces ───────────────────────────────────────────────────────────

// The OAuth2 client-credentials settings every ezIdentitySense API step uses.
// Values are blank here and filled at setup.
function ezisApiAuth(scope) {
    return {
        baseUrl: '',
        authType: 'oauth2',
        oauth2GrantType: 'client_credentials',
        oauth2TokenUrl: '',
        oauth2ClientId: '',
        oauth2ClientSecret: '',
        oauth2Audience: '',
        oauth2Scope: scope,
        encodePlaceholders: true,
        timeoutMs: 10000,
        retryCount: 0,
    };
}

// Setup fields for the ezIdentitySense connection, written into the listed
// API steps.
function ezisConnectionFields(stepAliases) {
    const f = (field, label, type, hint, required = true) => ({
        section: 'steps', section_label: 'ezIdentitySense Connection', step_aliases: stepAliases,
        field, label, type, hint, required,
    });
    return [
        f('baseUrl', 'ezIdentitySense Address', 'url', 'The address of your ezIdentitySense server, e.g. https://empi.example.org'),
        f('oauth2TokenUrl', 'Token URL', 'url', 'Your identity provider\'s OAuth 2.0 token endpoint, where this client gets its access tokens'),
        f('oauth2ClientId', 'Client ID', 'string', 'The client registered for ezHealthKonnect in your identity provider and in ezIdentitySense'),
        f('oauth2ClientSecret', 'Client Secret', 'password', 'That client\'s secret. Stored encrypted.'),
        f('oauth2Audience', 'Audience', 'string', 'Only if your identity provider requires one (the audience ezIdentitySense expects)', false),
    ];
}

// The engine exposes a step's output to later steps as
// steps.<normalized step name>.step_output, normalizing the name with
// models.OutputNormalizer.NormalizeKey. This is a port of that function, so
// the generator can refuse a step whose alias wouldn't match its name: paths
// written against the alias would then silently resolve to nothing.
function normalizeStepName(s) {
    if (!/[A-Z]/.test(s)) return s;
    const isUpper = c => c >= 'A' && c <= 'Z', isLower = c => c >= 'a' && c <= 'z';
    let out = '', prevUnderscore = false;
    for (let i = 0; i < s.length; i++) {
        const r = s[i];
        if (r === ' ' || r === '-' || r === '→') {
            if (!prevUnderscore && i > 0) { out += '_'; prevUnderscore = true; }
            continue;
        }
        if (!isUpper(r) && !isLower(r) && r !== '_' && !(r >= '0' && r <= '9')) continue;
        if (i > 0 && isUpper(r)) {
            const prev = s[i - 1];
            const nextLower = i < s.length - 1 && isLower(s[i + 1]);
            if ((isLower(prev) || nextLower) && prev !== ' ' && prev !== '-' && !prevUnderscore) {
                out += '_'; prevUnderscore = true;
            }
        }
        out += r.toLowerCase();
        prevUnderscore = r === '_';
    }
    return out;
}

function step(seq, name, alias, type, config, extra = {}) {
    // extra may carry parent_conditional_step_id (the parent's step_alias,
    // resolved to its real id when the pipeline is saved) and case_value.
    if (normalizeStepName(name) !== alias) {
        throw new Error(`step "${name}" must have alias "${normalizeStepName(name)}", not "${alias}"`);
    }
    return { sequence: seq, steps: [{ step_name: name, step_alias: alias, step_type: type, sequence: seq, config, enabled: true, required: true, ...extra }] };
}

// FHIR Patient (golden view) -> PID, shared by every outbound message.
// Paths read the "Fetch Patient" step's output, whose keys the platform
// normalizes to snake_case (birthDate -> birth_date, postalCode -> postal_code).
const P = 'steps.fetch_patient.step_output';
const notFound = { field: `${P}.resource_type`, operator: 'not_equals', value: 'Patient' };

function pidFromGoldenPatient() {
    return {
        segment: 'PID',
        fields: [
            { fieldKey: 'PID.1', literalValue: '1' },
            // Every identifier the golden view lists: the enterprise ID first,
            // then each linked record's local ID and other identifiers, with
            // the assigning authority as a URI.
            { fieldKey: 'PID.3', repeatPath: `${P}.identifier`, components: [
                { position: '1', sourcePath: 'value' },
                { position: '4.2', sourcePath: 'system' },
                { position: '4.3', literalValue: 'URI' },
            ] },
            // An identity that no longer exists (erased): its EID alone.
            { fieldKey: 'PID.3.1', sourcePath: 'message.subject', condition: notFound },
            { fieldKey: 'PID.5', repeatPath: `${P}.name`, components: [
                { position: '1', sourcePath: 'family' },
                { position: '2', sourcePath: 'given[0]' },
                { position: '3', sourcePath: 'given[1]' },
                { position: '5', sourcePath: 'prefix[0]' },
                { position: '4', sourcePath: 'suffix[0]' },
                { position: '7', sourcePath: 'use', valueMap: { official: 'L', usual: 'D', nickname: 'N', maiden: 'M', old: 'U', anonymous: 'S' } },
            ] },
            { fieldKey: 'PID.6', repeatPath: `${P}.extension`, maxRepetitions: 1,
              itemCondition: { field: 'url', operator: 'equals', value: 'http://hl7.org/fhir/StructureDefinition/patient-mothersMaidenName' },
              components: [{ position: '1', sourcePath: 'value_string' }] },
            { fieldKey: 'PID.7', sourcePath: `${P}.birth_date`, transform: 'hl7_date' },
            { fieldKey: 'PID.8', sourcePath: `${P}.gender`, valueMap: { male: 'M', female: 'F', other: 'O', unknown: 'U' } },
            { fieldKey: 'PID.11', repeatPath: `${P}.address`, components: [
                { position: '1', sourcePath: 'line[0]' },
                { position: '2', sourcePath: 'line[1]' },
                { position: '3', sourcePath: 'city' },
                { position: '4', sourcePath: 'state' },
                { position: '5', sourcePath: 'postal_code' },
                { position: '6', sourcePath: 'country', transform: 'iso3166_alpha3' },
            ] },
            { fieldKey: 'PID.13', repeatPath: `${P}.telecom`,
              itemCondition: { field: 'system', operator: 'equals', value: 'phone' },
              components: [
                { position: '1', sourcePath: 'value' },
                { position: '2', sourcePath: 'use', valueMap: { home: 'PRN', work: 'WPN', mobile: 'ORN', temp: 'PRN', old: 'PRN' } },
                { position: '3', literalValue: 'PH' },
              ] },
            { fieldKey: 'PID.29', sourcePath: `${P}.deceased_date_time`, transform: 'hl7_datetime' },
            { fieldKey: 'PID.30', sourcePath: `${P}.deceased_boolean`, valueMap: { true: 'Y', false: 'N' } },
        ],
    };
}

// The second PID of an A37, from whichever lookup ran (Fetch Unmerged
// Identity or Fetch Split Identity); its EID alone if that identity is gone.
function secondPID() {
    const F = 'steps.fetch_unmerged_identity.step_output', C = 'steps.fetch_split_identity.step_output';
    const ids = (path, type) => ({ fieldKey: 'PID.3', repeatPath: `${path}.identifier`,
        condition: { field: 'message.type', operator: 'equals', value: type },
        components: [
            { position: '1', sourcePath: 'value' },
            { position: '4.2', sourcePath: 'system' },
            { position: '4.3', literalValue: 'URI' },
        ] });
    const either = (fieldKey, rel, extra = {}) => ({ fieldKey, sourcePath: `${F}.${rel}`, fallbackPaths: [`${C}.${rel}`], ...extra });
    return {
        segment: 'PID', condition: { field: 'message.type', operator: 'in_list', value: ['identity.unmerged', 'identity.split'] },
        fields: [
            { fieldKey: 'PID.1', literalValue: '2' },
            ids(F, 'identity.unmerged'),
            ids(C, 'identity.split'),
            // The EID is the golden view's first identifier, so this repeats
            // it when the lookup worked and supplies it when it didn't.
            { fieldKey: 'PID.3.1', sourcePath: 'message.data.from', fallbackPaths: ['message.data.created'] },
            either('PID.5.1', 'name[0].family'),
            either('PID.5.2', 'name[0].given[0]'),
            either('PID.5.3', 'name[0].given[1]'),
            either('PID.5.7', 'name[0].use', { valueMap: { official: 'L', usual: 'D', nickname: 'N', maiden: 'M', old: 'U', anonymous: 'S' } }),
            either('PID.7', 'birth_date', { transform: 'hl7_date' }),
            either('PID.8', 'gender', { valueMap: { male: 'M', female: 'F', other: 'O', unknown: 'U' } }),
        ],
    };
}

// ── Template 1: ezIdentitySense events -> HL7 v2 ADT ───────────────────────

const OUTBOUND_EVENT_TYPES = [
    'identity.created', 'record.linked', 'record.relinked', 'record.updated', 'record.deactivated',
    'identity.golden-changed', 'identity.merged', 'identity.unmerged', 'identity.split', 'identity.purged',
];

const outbound = {
    name: 'ezIdentitySense Patient Events to HL7 ADT',
    slug: 'ezidentitysense-events-to-hl7-adt',
    description: 'Sends HL7 v2 ADT messages to a downstream system whenever ezIdentitySense (the enterprise master patient index) creates, updates, merges or unlinks a patient identity: A28 new person, A31 update, A40 merge, A37 unlink, A29 delete. Events arrive by signed webhook and are processed one at a time, in order; ezIdentitySense is answered only when the message has been accepted downstream, so nothing is lost or reordered.',
    category: 'empi',
    subcategory: 'ezidentitysense',
    tags: ['ezidentitysense', 'empi', 'mpi', 'hl7v2', 'adt', 'webhook', 'cloudevents', 'outbound'],
    icon: '🪪',
    difficulty: 'intermediate',
    message_type: 'CloudEvents',
    source_connector_type: 'webhook_events_inbound',
    source_config_template: { port: 8095, endpoint_path: '/events', authentication_type: 'standard_webhooks' },
    target_connector_type: 'tcp_mllp_outbound',
    target_config_template: { host: '', port: 2575 },
    pipeline_config: {
        execution_groups: [
            step(5, 'ezIdentitySense Events', 'ez_identity_sense_events', 'connector.inbound', {
                connectorType: 'webhook_events_inbound',
                config: {
                    port: 8095, endpoint_path: '/events', authentication_type: 'standard_webhooks', webhook_secret: '',
                    event_types: OUTBOUND_EVENT_TYPES, reply_within_seconds: 8, event_timeout_seconds: 300,
                },
                timeoutMs: 30000,
            }),
            // The golden view of the identity the event is about. A 404/410
            // (an erased identity) is a normal answer: the message then
            // carries the EID only.
            step(10, 'Fetch Patient', 'fetch_patient', 'enrichment.api', {
                ...ezisApiAuth('identities.read identities.read.demographics'),
                endpoint: '/fhir/r4/Patient/{eid}', method: 'GET',
                fieldMappings: { eid: 'message.subject' },
                headers: { Accept: 'application/fhir+json' },
                acceptStatuses: [404, 410],
            }),
            // A37 names two identities: fetch the second one too. Only the
            // matching case's lookup runs; any other event takes "default".
            step(12, 'Second Identity', 'second_identity', 'switch_case', {
                field: 'message.type',
                cases: [
                    { value: 'identity.unmerged', label: 'Unmerge', actions: [] },
                    { value: 'identity.split', label: 'Split', actions: [] },
                ],
                default: { actions: [] },
                options: { caseInsensitive: false, trimWhitespace: true },
            }),
            step(14, 'Fetch Unmerged Identity', 'fetch_unmerged_identity', 'enrichment.api', {
                ...ezisApiAuth('identities.read identities.read.demographics'),
                endpoint: '/fhir/r4/Patient/{eid}', method: 'GET',
                fieldMappings: { eid: 'message.data.from' },
                headers: { Accept: 'application/fhir+json' },
                acceptStatuses: [404, 410],
            }, { parent_conditional_step_id: 'second_identity', case_value: 'identity.unmerged' }),
            step(16, 'Fetch Split Identity', 'fetch_split_identity', 'enrichment.api', {
                ...ezisApiAuth('identities.read identities.read.demographics'),
                endpoint: '/fhir/r4/Patient/{eid}', method: 'GET',
                fieldMappings: { eid: 'message.data.created' },
                headers: { Accept: 'application/fhir+json' },
                acceptStatuses: [404, 410],
            }, { parent_conditional_step_id: 'second_identity', case_value: 'identity.split' }),
            step(20, 'Build ADT', 'build_adt', 'hl7.build', {
                messageType: 'ADT', version: '2.5.1', escapeValues: true,
                sendingApplication: 'EZIDENTITYSENSE', sendingFacility: '',
                receivingApplication: '', receivingFacility: '',
                triggerEventPath: 'message.type',
                triggerEventMap: {
                    'identity.created': 'A28',
                    'record.linked': 'A31', 'record.relinked': 'A31', 'record.updated': 'A31',
                    'record.deactivated': 'A31', 'identity.golden-changed': 'A31',
                    'identity.merged': 'A40',
                    'identity.unmerged': 'A37', 'identity.split': 'A37',
                    'identity.purged': 'A29',
                },
                messageStructureMap: { A28: 'ADT_A05', A31: 'ADT_A05', A40: 'ADT_A39', A37: 'ADT_A37', A29: 'ADT_A21' },
                segments: [
                    { segment: 'EVN', fields: [
                        { fieldKey: 'EVN.2', sourcePath: 'message.time', transform: 'hl7_datetime' },
                    ] },
                    pidFromGoldenPatient(),
                    // A28/A31/A29 carry a visit segment; "N" = not applicable.
                    { segment: 'PV1', condition: { field: 'message.type', operator: 'in_list', value: ['identity.created', 'record.linked', 'record.relinked', 'record.updated', 'record.deactivated', 'identity.golden-changed', 'identity.purged'] },
                      fields: [{ fieldKey: 'PV1.1', literalValue: '1' }, { fieldKey: 'PV1.2', literalValue: 'N' }] },
                    // A40: the retired identity merged into the one in PID.
                    { segment: 'MRG', condition: { field: 'message.type', operator: 'equals', value: 'identity.merged' },
                      fields: [{ fieldKey: 'MRG.1.1', sourcePath: 'message.data.retired' }] },
                    // A37: the second, now separate, identity.
                    secondPID(),
                ],
            }),
            step(30, 'Send ADT', 'send_adt', 'connector.outbound', {
                connectorType: 'tcp_mllp_outbound',
                config: { host: '', port: 2575, expect_ack: true, ack_timeout_seconds: 30, connect_timeout_seconds: 10 },
                contentField: 'hl7Message',
                contentType: 'x-application/hl7-v2+er7',
            }),
        ],
    },
    required_connection_fields: [
        { section: 'source', field: 'port', label: 'Webhook Port', type: 'number', required: true,
          hint: 'The port ezHealthKonnect listens on for ezIdentitySense\'s webhooks.' },
        { section: 'source', field: 'webhook_secret', label: 'Webhook Signing Secret', type: 'password', required: true,
          hint: 'The whsec_… secret ezIdentitySense showed when you created the webhook. Stored encrypted.' },
        ...ezisConnectionFields(['fetch_patient', 'fetch_unmerged_identity', 'fetch_split_identity']),
        { section: 'steps', section_label: 'ezIdentitySense Connection', step_aliases: ['build_adt'], field: 'receivingApplication',
          label: 'Receiving Application (MSH-5)', type: 'string', required: false, hint: 'Optional: the receiving system\'s application name.' },
        { section: 'steps', section_label: 'ezIdentitySense Connection', step_aliases: ['build_adt'], field: 'receivingFacility',
          label: 'Receiving Facility (MSH-6)', type: 'string', required: false, hint: 'Optional: the receiving system\'s facility.' },
        { section: 'target', field: 'host', label: 'Downstream System Host', type: 'string', required: true,
          hint: 'The host name or address of the system that receives the ADT messages over MLLP.' },
        { section: 'target', field: 'port', label: 'Downstream System Port', type: 'number', required: true,
          hint: 'Its MLLP port.' },
    ],
    preview_steps: [
        { name: 'Receive ezIdentitySense Event', icon: '🔔', step_type: 'connector.inbound' },
        { name: 'Fetch Golden Patient', icon: '🔍', step_type: 'enrichment.api' },
        { name: 'Build ADT', icon: '🔄', step_type: 'hl7.build' },
        { name: 'Send over MLLP', icon: '📤', step_type: 'connector.outbound' },
    ],
    estimated_setup_minutes: 15,
    processing_order: 'in_order',
    guide: 'events-to-adt.md',
};

const TEMPLATES = [outbound];

// ── SQL output ──────────────────────────────────────────────────────────────

const lit = s => "'" + String(s).replace(/'/g, "''") + "'";
const json = v => lit(JSON.stringify(v)) + '::jsonb';
const arr = a => 'ARRAY[' + a.map(lit).join(',') + ']::text[]';

function guideSQL(t) {
    const text = fs.readFileSync(path.join(GUIDES, t.guide), 'utf8');
    if (text.includes('$GUIDE$')) throw new Error(`${t.guide} contains the dollar-quote tag`);
    return `UPDATE interface_templates SET user_guide = $GUIDE$\n${text}$GUIDE$\nWHERE slug = ${lit(t.slug)};\n`;
}

function sensitiveFields(t) {
    return t.required_connection_fields
        .filter(f => f.type === 'password' || /secret|token|host|url|address/i.test(f.field))
        .map(f => `${f.section}.${f.field}`);
}

function templateSQL(t) {
    return `
-- ${t.name}
DELETE FROM interface_templates WHERE slug = ${lit(t.slug)} AND is_system = true;
INSERT INTO interface_templates
    (id, name, slug, description, category, subcategory, tags, icon, difficulty,
     source_connector_type, source_config_template,
     target_connector_type, target_config_template,
     pipeline_config, message_type,
     required_connection_fields, sanitized_fields, preview_steps,
     estimated_setup_minutes, is_system, is_public, author, usage_count, processing_order)
VALUES (
    gen_random_uuid(), ${lit(t.name)}, ${lit(t.slug)}, ${lit(t.description)},
    ${lit(t.category)}, ${lit(t.subcategory)}, ${arr(t.tags)}, ${lit(t.icon)}, ${lit(t.difficulty)},
    ${lit(t.source_connector_type)}, ${json(t.source_config_template)},
    ${lit(t.target_connector_type)}, ${json(t.target_config_template)},
    ${json(t.pipeline_config)}, ${lit(t.message_type)},
    ${json(t.required_connection_fields)}, ${arr(sensitiveFields(t))}, ${json(t.preview_steps)},
    ${t.estimated_setup_minutes}, true, true, 'ezHealthKonnect', 0, ${lit(t.processing_order)}
);
${guideSQL(t)}`;
}

process.stdout.write(`-- V290: ready-to-use templates connecting ezHealthKonnect to ezIdentitySense
-- (the enterprise master patient index) through its public API only.
--
-- GENERATED by scripts/templates/ezidentitysense-templates.js from the JS
-- definitions and the Markdown guides beside it. Edit those and regenerate;
-- don't edit this file by hand.
--
-- Every credential ships blank and is asked for by the "Use template" form.
-- Also adds event_types to the Ordered Webhook Events connector's settings
-- form (the field was added to the connector after V289).

-- The processing order an interface created from the template starts with.
ALTER TABLE interface_templates
    ADD COLUMN IF NOT EXISTS processing_order TEXT NOT NULL DEFAULT 'parallel';

UPDATE connectivity_types
SET config_schema = jsonb_set(config_schema, '{properties,event_types}',
        '{"type":"array","items":{"type":"string"},"title":"Event Types","description":"Only these event types are processed; others are acknowledged and skipped. Empty = all."}'::jsonb),
    parameter_groups = jsonb_set(parameter_groups, '{advanced}', (parameter_groups->'advanced') || '["event_types"]'::jsonb),
    updated_at = NOW()
WHERE type_name = 'webhook_events_inbound'
  AND NOT (config_schema->'properties' ? 'event_types');
${TEMPLATES.map(templateSQL).join('')}`);
