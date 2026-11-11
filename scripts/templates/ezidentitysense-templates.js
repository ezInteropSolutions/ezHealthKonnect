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
    source_config_template: { port: 8088, endpoint_path: '/events', authentication_type: 'standard_webhooks' },
    target_connector_type: 'tcp_mllp_outbound',
    target_config_template: { host: '', port: 2575 },
    pipeline_config: {
        execution_groups: [
            step(5, 'ezIdentitySense Events', 'ez_identity_sense_events', 'connector.inbound', {
                connectorType: 'webhook_events_inbound',
                config: {
                    port: 8088, endpoint_path: '/events', authentication_type: 'standard_webhooks', webhook_secret: '',
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

// ── Template 2: HL7 v2 ADT -> ezIdentitySense ──────────────────────────────

// The record's local ID: the transformed Patient's identifier in this
// source's ID authority (a PID-3 repetition whose assigning authority is it).
const PATIENT = 'message.fhirResources[resourceType=Patient]';
const LOCAL_ID = `${PATIENT}.identifier[system={{var.sourceAuthority}}].value`;
const FEED = '/fhir/r4/Patient?identifier={{var.sourceAuthority}}|';
const FHIR_JSON = { 'Content-Type': 'application/fhir+json', Accept: 'application/fhir+json' };

function feedStep(seq, name, alias, caseValue, cfg) {
    return step(seq, name, alias, 'enrichment.api', { ...ezisApiAuth('records.write'), headers: FHIR_JSON, ...cfg },
        { parent_conditional_step_id: 'adt_event', case_value: caseValue });
}

const INBOUND_STEPS = ['register_patient', 'merge_record', 'remove_record_a23', 'remove_record_a29'];

const inbound = {
    name: 'HL7 ADT to ezIdentitySense',
    slug: 'hl7-adt-to-ezidentitysense',
    description: 'Feeds a source system\'s HL7 v2 ADT messages into ezIdentitySense (the enterprise master patient index) through its standard IHE PIXm Patient Identity Feed: registrations and updates (A01, A04, A05, A08, A28, A31 and any other ADT carrying PID) register or update the patient, A40 merges two of the source\'s records, A23 and A29 remove one. Processed one at a time, in order; an older message never overwrites a newer one.',
    category: 'empi',
    subcategory: 'ezidentitysense',
    tags: ['ezidentitysense', 'empi', 'mpi', 'hl7v2', 'adt', 'pix', 'pixm', 'iti-104', 'patient-feed', 'inbound'],
    icon: '🪪',
    difficulty: 'intermediate',
    message_type: 'ADT',
    source_connector_type: 'tcp_mllp_inbound',
    source_config_template: { host: '0.0.0.0', port: 6660 },
    target_connector_type: 'http_outbound',
    target_config_template: {},
    pipeline_config: {
        execution_groups: [
            step(5, 'ADT Feed', 'adt_feed', 'connector.inbound', {
                connectorType: 'tcp_mllp_inbound',
                config: {
                    host: '0.0.0.0', port: 6660, max_connections: 10,
                    ack: { mode: 'immediate', on_error: 'nack', sending_app: 'EZHEALTHKONNECT', sending_facility: '' },
                },
                timeoutMs: 30000,
            }),
            // HL7 v2 -> FHIR Patient (PID: identifiers, names, birth date,
            // sex, addresses, phones, mother's maiden name; MSH-7 as the
            // version in meta.lastUpdated).
            step(10, 'HL7 to FHIR', 'hl7_to_fhir', 'hl7_fhir_transform', {
                target_resources: ['Patient'],
            }),
            step(20, 'ADT Event', 'adt_event', 'switch_case', {
                field: 'MSH.9.2',
                cases: [
                    { value: 'A40', label: 'Merge', actions: [] },
                    { value: 'A23', label: 'Delete visit', actions: [] },
                    { value: 'A29', label: 'Delete person', actions: [] },
                ],
                default: { actions: [] },
                options: { caseInsensitive: true, trimWhitespace: true },
            }),
            feedStep(30, 'Register Patient', 'register_patient', 'default', {
                method: 'PUT', endpoint: FEED + '{localId}',
                fieldMappings: { localId: LOCAL_ID },
                bodyRef: PATIENT,
            }),
            // A40: the record in MRG-1 was merged, in the source, into the
            // record in PID-3.
            feedStep(32, 'Merge Record', 'merge_record', 'A40', {
                method: 'PUT', endpoint: FEED + '{retiredId}',
                fieldMappings: { retiredId: 'MRG.1.1', survivingId: LOCAL_ID },
                requestBody: {
                    resourceType: 'Patient',
                    active: false,
                    identifier: [{ system: '{{var.sourceAuthority}}', value: 'retiredId' }],
                    link: [{ type: 'replaced-by', other: { identifier: { system: '{{var.sourceAuthority}}', value: 'survivingId' } } }],
                },
            }),
            feedStep(34, 'Remove Record A23', 'remove_record_a23', 'A23', {
                method: 'DELETE', endpoint: FEED + '{localId}', fieldMappings: { localId: LOCAL_ID },
            }),
            feedStep(36, 'Remove Record A29', 'remove_record_a29', 'A29', {
                method: 'DELETE', endpoint: FEED + '{localId}', fieldMappings: { localId: LOCAL_ID },
            }),
        ],
    },
    required_connection_fields: [
        { section: 'source', field: 'port', label: 'MLLP Port', type: 'number', required: true,
          hint: 'The port the source system sends its ADT messages to.' },
        { section: 'variables', section_label: 'Source System', field: 'sourceAuthority',
          label: 'This Source\'s ID Authority', type: 'string', required: true,
          hint: 'The ID authority set for this source system in ezIdentitySense, e.g. urn:oid:2.16.840.1.113883.3.999.1. Its PID-3 identifier is the record\'s local ID.' },
        ...ezisConnectionFields(INBOUND_STEPS),
    ],
    preview_steps: [
        { name: 'Receive ADT over MLLP', icon: '🔌', step_type: 'connector.inbound' },
        { name: 'HL7 to FHIR Patient', icon: '🔄', step_type: 'hl7_fhir_transform' },
        { name: 'Register, Merge or Remove', icon: '🔀', step_type: 'switch_case' },
        { name: 'ezIdentitySense Patient Feed', icon: '📤', step_type: 'enrichment.api' },
    ],
    estimated_setup_minutes: 15,
    processing_order: 'in_order',
    guide: 'adt-to-ezidentitysense.md',
};

// ── Template 3: HL7 PIX / PDQ queries -> ezIdentitySense ──────────────────
//
// IHE PIX Query (ITI-9, QBP^Q23 -> RSP^K23) and Patient Demographics Query
// (ITI-21, QBP^Q22 -> RSP^K22, with the continuation protocol and QCN^J01
// cancel), answered over the open MLLP connection from ezIdentitySense's
// PIXm $ihe-pix and PDQm search. Requirements: docs/ihe-pix-pdq-hl7v2-requirements.md.

const QT = 'steps.pix_query.step_output';      // PIX query, read
const PIX = 'steps.pix_lookup.step_output';    // $ihe-pix answer
const PQ = 'steps.pdq_query.step_output';      // PDQ query, read
const PAGE = 'steps.load_page.step_output';    // a continuation's stored page
const SR = 'steps.pdq_search.step_output';     // PDQm searchset Bundle
const SAVED = 'steps.save_page.step_output';   // the next page, stored
const CQ = 'steps.cancel_query.step_output';   // QCN, read
const OQ = 'steps.other_message.step_output';  // anything else, read

const NAMESPACES = '{{var.namespaces}}';
const BY_UNIVERSAL_ID = '{{var.namespaces|inverted}}';
const INTERNAL_ERROR = '207^Application internal error^HL70357';
const UNKNOWN_KEY = '204^Unknown key identifier^HL70357';

const is = (field, value) => ({ field, operator: 'equals', value });
const exists = field => ({ field, operator: 'exists' });

// The reply comes from the system the query was addressed to, to the one
// that sent it: MSH-3/4 are the query's MSH-5/6 and the other way round.
function replyHeader(q, profile) {
    return [
        { fieldKey: 'MSH.3', sourcePath: `${q}.receiving_application`, encoded: true, literalValue: 'EZIDENTITYSENSE' },
        { fieldKey: 'MSH.4', sourcePath: `${q}.receiving_facility`, encoded: true },
        { fieldKey: 'MSH.5', sourcePath: `${q}.sending_application`, encoded: true },
        { fieldKey: 'MSH.6', sourcePath: `${q}.sending_facility`, encoded: true },
        ...(profile ? [{ fieldKey: 'MSH.21', literalValue: profile }] : []),
    ];
}

function msa(q) {
    return { segment: 'MSA', fields: [
        { fieldKey: 'MSA.1', sourcePath: 'derived.ack' },
        { fieldKey: 'MSA.2', sourcePath: `${q}.control_id`, encoded: true },
    ] };
}

// One ERR per problem found in the query, then one for the lookup's own
// failure (derived.err_*), if any.
function errSegments(q) {
    return [
        { segment: 'ERR', cardinality: 'repeating', rowsPath: `${q}.problems`, fields: [
            { fieldKey: 'ERR.2', sourcePath: 'location', encoded: true },
            { fieldKey: 'ERR.3', sourcePath: 'code', encoded: true },
            { fieldKey: 'ERR.4', literalValue: 'E' },
            { fieldKey: 'ERR.8', sourcePath: 'text' },
        ] },
        { segment: 'ERR', condition: exists('derived.err_code'), fields: [
            { fieldKey: 'ERR.2', sourcePath: 'derived.err_location', encoded: true },
            { fieldKey: 'ERR.3', sourcePath: 'derived.err_code', encoded: true },
            { fieldKey: 'ERR.4', literalValue: 'E' },
            { fieldKey: 'ERR.8', sourcePath: 'derived.err_text' },
        ] },
    ];
}

// A derived value set by the lookup's HTTP status; problems in the query
// come first and leave it empty (their own ERR rows report them).
function byStatus(name, statusPath, q, cases, otherwise) {
    return {
        name, default: otherwise,
        rules: [
            { condition: is(`${q}.has_problems`, true), value: '' },
            ...cases.map(([status, value]) => ({ condition: is(statusPath, status), value })),
        ],
    };
}

// A CX from an identifier system and value: the namespace from the setup
// table (the universal ID itself when it isn't listed), universal ID, type.
function cx(valuePath, systemPath) {
    return [
        { position: '1', sourcePath: valuePath },
        { position: '4.1', sourcePath: systemPath, transform: 'hl7_universal_id', valueMap: BY_UNIVERSAL_ID },
        { position: '4.2', sourcePath: systemPath, transform: 'hl7_universal_id' },
        { position: '4.3', sourcePath: systemPath, transform: 'hl7_universal_id_type' },
    ];
}

function queryLookup(seq, name, alias, caseValue, scope, cfg) {
    return step(seq, name, alias, 'enrichment.api', {
        ...ezisApiAuth(scope), timeoutMs: 7000, statusOutputField: 'http_status', ...cfg,
    }, {
        parent_conditional_step_id: 'query_type', case_value: caseValue,
        // A failed lookup still gets an answer: AE with ERR, not silence.
        required: false, on_error_strategy: 'skip', timeout_ms: 8000,
    });
}

const inQuery = (caseValue) => ({ parent_conditional_step_id: 'query_type', case_value: caseValue });

// PDQ parameters (QPD-3 "@PID.x^value") -> PDQm search parameters. IHE
// requires PID.3, 5, 7, 8, 11 and 18 (6 and 13 for pediatric demographics);
// ezIdentitySense has no account number, so @PID.18 is answered as
// unsupported rather than ignored.
const GENDER = { M: 'male', F: 'female', O: 'other', A: 'other', U: 'unknown', N: 'unknown' };
const PDQ_PARAMETERS = [
    ['PID.3.1', 'identifier', { domainField: 'PID.3.4' }],
    ['PID.5.1.1', 'family'], ['PID.5.1', 'family'], ['PID.5', 'family'],
    ['PID.5.2', 'given'], ['PID.5.3', 'given'],
    ['PID.6.1.1', 'mothersMaidenName'], ['PID.6.1', 'mothersMaidenName'],
    ['PID.7', 'birthdate', { transform: 'fhir_date' }], ['PID.7.1', 'birthdate', { transform: 'fhir_date' }],
    ['PID.8', 'gender', { valueMap: GENDER }],
    ['PID.11.1', 'address'], ['PID.11.1.1', 'address'], ['PID.11.3', 'address-city'], ['PID.11.4', 'address-state'],
    ['PID.11.5', 'address-postalcode'], ['PID.11.6', 'address-country', { transform: 'iso3166_alpha2' }],
    ['PID.13.1', 'telecom'], ['PID.13.4', 'telecom'], ['PID.13.12', 'telecom'],
].map(([field, param, extra = {}]) => ({ field, param, ...extra }));

const NAME_USE = { official: 'L', usual: 'D', nickname: 'N', maiden: 'M', old: 'U', anonymous: 'S', temp: '' };

// One PID per patient found, from the golden view.
function pdqPID() {
    const ids = (condition, itemCondition) => ({ fieldKey: 'PID.3', repeatPath: 'resource.identifier', condition,
        ...(itemCondition ? { itemCondition } : {}), components: cx('value', 'system') });
    return {
        segment: 'PID', cardinality: 'repeating', rowsPath: `${SR}.entry`, condition: is('derived.ack', 'AA'),
        fields: [
            // QPD-8 lists the domains to return; with none, every one.
            ids({ field: `${PQ}.domain_count`, operator: 'equals', value: 0 }),
            ids({ field: `${PQ}.domain_count`, operator: 'greater_than', value: 0 },
                { field: 'system', operator: 'in_list', compareToField: `${PQ}.domain_systems` }),
            { fieldKey: 'PID.5', repeatPath: 'resource.name', components: [
                { position: '1', sourcePath: 'family' },
                { position: '2', sourcePath: 'given[0]' },
                { position: '3', sourcePath: 'given[1]' },
                { position: '4', sourcePath: 'suffix[0]' },
                { position: '5', sourcePath: 'prefix[0]' },
                { position: '7', sourcePath: 'use', valueMap: NAME_USE },
            ] },
            { fieldKey: 'PID.6', repeatPath: 'resource.extension', maxRepetitions: 1,
              itemCondition: is('url', 'http://hl7.org/fhir/StructureDefinition/patient-mothersMaidenName'),
              components: [{ position: '1', sourcePath: 'value_string' }] },
            { fieldKey: 'PID.7', sourcePath: 'resource.birth_date', transform: 'hl7_date' },
            { fieldKey: 'PID.8', sourcePath: 'resource.gender', valueMap: { male: 'M', female: 'F', other: 'O', unknown: 'U' } },
            { fieldKey: 'PID.11', repeatPath: 'resource.address', components: [
                { position: '1', sourcePath: 'line[0]' },
                { position: '2', sourcePath: 'line[1]' },
                { position: '3', sourcePath: 'city' },
                { position: '4', sourcePath: 'state' },
                { position: '5', sourcePath: 'postal_code' },
                { position: '6', sourcePath: 'country', transform: 'iso3166_alpha3' },
                { position: '7', sourcePath: 'use', valueMap: { home: 'H', work: 'B', temp: 'C', old: '', billing: '' } },
            ] },
            { fieldKey: 'PID.13', repeatPath: 'resource.telecom', components: [
                { position: '1', sourcePath: 'value', condition: { field: 'system', operator: 'not_equals', value: 'email' } },
                { position: '2', sourcePath: 'use', valueMap: { home: 'PRN', work: 'WPN', mobile: 'PRS', temp: '', old: '' } },
                { position: '3', sourcePath: 'system', valueMap: { phone: 'PH', fax: 'FX', email: 'Internet', pager: 'BP', sms: 'CP', url: '', other: '' } },
                { position: '4', sourcePath: 'value', condition: is('system', 'email') },
            ] },
            { fieldKey: 'PID.29', sourcePath: 'resource.deceased_date_time', transform: 'hl7_datetime' },
            { fieldKey: 'PID.30', sourcePath: 'resource.deceased_boolean', valueMap: { true: 'Y', false: 'N' } },
        ],
    };
}

// The page store's key: the sender and its query tag. A continuation names
// its page with DSC-1, the random token stored with it.
const pageKey = q => [`${q}.sending_application`, `${q}.sending_facility`, `${q}.query_tag`];

const queries = {
    name: 'HL7 PIX/PDQ Queries to ezIdentitySense',
    slug: 'hl7-pix-pdq-to-ezidentitysense',
    description: 'Answers HL7 v2 patient identifier and demographics queries from ezIdentitySense (the enterprise master patient index): IHE PIX Query (QBP^Q23 -> RSP^K23) returns a patient\'s identifiers in other domains; Patient Demographics Query (QBP^Q22 -> RSP^K22) finds patients by name, birth date, sex, address, phone or identifier, with paging (DSC continuation) and cancel (QCN^J01). Each reply goes back on the same MLLP connection.',
    category: 'empi',
    subcategory: 'ezidentitysense',
    tags: ['ezidentitysense', 'empi', 'mpi', 'hl7v2', 'pix', 'pdq', 'iti-9', 'iti-21', 'qbp', 'query', 'inbound'],
    icon: '🪪',
    difficulty: 'intermediate',
    message_type: 'QBP',
    source_connector_type: 'tcp_mllp_inbound',
    source_config_template: { host: '0.0.0.0', port: 6663 },
    target_connector_type: 'http_outbound',
    target_config_template: {},
    pipeline_config: {
        execution_groups: [
            step(5, 'PIX PDQ Queries', 'pix_pdq_queries', 'connector.inbound', {
                connectorType: 'tcp_mllp_inbound',
                config: {
                    host: '0.0.0.0', port: 6663, max_connections: 10,
                    // Every message on this port is answered by the pipeline
                    // itself, on the same connection: RSP, or AR for a type
                    // it doesn't take.
                    host_query: { enabled: true, message_types: ['*'], pipeline_message_type: 'QBP', timeout_seconds: 10, on_failure_nack: true },
                    ack: { mode: 'immediate', on_error: 'nack', sending_app: 'EZIDENTITYSENSE', sending_facility: '' },
                },
                timeoutMs: 30000,
            }),
            step(10, 'Query Type', 'query_type', 'switch_case', {
                field: 'MSH.9.2',
                cases: [
                    { value: 'Q23', label: 'PIX Query', actions: [] },
                    { value: 'Q22', label: 'Demographics Query', actions: [] },
                    { value: 'J01', label: 'Cancel Query', actions: [] },
                ],
                default: { actions: [] },
                options: { caseInsensitive: true, trimWhitespace: true },
            }),

            // ── PIX Query (ITI-9) ──
            step(20, 'PIX Query', 'pix_query', 'hl7.query_parse', {
                identifierField: 'QPD.3', identifierParam: 'sourceIdentifier',
                domainsField: 'QPD.4', domainsParam: 'targetSystem',
                namespaces: NAMESPACES,
            }, inQuery('Q23')),
            queryLookup(22, 'PIX Lookup', 'pix_lookup', 'Q23', 'identities.read', {
                method: 'GET', endpoint: '/fhir/r4/Patient/$ihe-pix?{query}',
                // The query is URL-encoded already by PIX Query.
                fieldMappings: { query: `${QT}.query` }, encodePlaceholders: false,
                headers: { Accept: 'application/fhir+json' },
                // 404 unknown patient, 400 unknown source domain, 403 unknown
                // target domain: answers, not failures.
                acceptStatuses: [400, 403, 404],
            }),
            step(24, 'PIX Response', 'pix_response', 'hl7.build', {
                messageType: 'RSP', triggerEvent: 'K23', messageStructure: 'RSP_K23', version: '2.5', escapeValues: true,
                mshFields: replyHeader(QT, 'ITI-9^IHE'),
                derivedValues: [
                    { name: 'ack', default: 'AE', rules: [
                        { condition: is(`${QT}.has_problems`, true), value: 'AE' },
                        { condition: is(`${PIX}.http_status`, 200), value: 'AA' },
                    ] },
                    { name: 'first_id', rules: [{ sourcePath: `${PIX}.parameter[name=targetIdentifier].value_identifier.value` }] },
                    // IHE cases 1-6: OK with identifiers, NF found but none
                    // in the domains asked for, AE otherwise.
                    { name: 'status', default: 'NF', rules: [
                        { condition: is('derived.ack', 'AE'), value: 'AE' },
                        { condition: exists('derived.first_id'), value: 'OK' },
                    ] },
                    byStatus('err_location', `${PIX}.http_status`, QT,
                        [[404, 'QPD^1^3^1^1'], [400, 'QPD^1^3^1^4'], [403, 'QPD^1^4'], [200, '']], ''),
                    byStatus('err_code', `${PIX}.http_status`, QT,
                        [[404, UNKNOWN_KEY], [400, UNKNOWN_KEY], [403, UNKNOWN_KEY], [200, '']], INTERNAL_ERROR),
                    byStatus('err_text', `${PIX}.http_status`, QT, [
                        [404, 'The patient identifier is not known.'],
                        [400, 'The identifier\'s assigning authority is not a known domain.'],
                        [403, 'A requested domain is not known.'],
                        [200, ''],
                    ], 'ezIdentitySense could not answer the query.'),
                ],
                segments: [
                    msa(QT),
                    ...errSegments(QT),
                    { segment: 'QAK', fields: [
                        { fieldKey: 'QAK.1', sourcePath: `${QT}.query_tag`, encoded: true },
                        { fieldKey: 'QAK.2', sourcePath: 'derived.status' },
                    ] },
                    { segment: 'QPD', rawPath: `${QT}.qpd_segment` },
                    // The patient's identifiers in the other domains (never
                    // the one asked about), fully qualified; PID-5 is the
                    // empty name IHE prescribes.
                    { segment: 'PID', condition: is('derived.status', 'OK'), fields: [
                        { fieldKey: 'PID.3', repeatPath: `${PIX}.parameter`, itemCondition: is('name', 'targetIdentifier'),
                          components: cx('value_identifier.value', 'value_identifier.system') },
                        { fieldKey: 'PID.5.7', literalValue: 'S', repetition: 2 },
                    ] },
                ],
            }, inQuery('Q23')),

            // ── Patient Demographics Query (ITI-21) ──
            step(30, 'PDQ Query', 'pdq_query', 'hl7.query_parse', {
                parametersField: 'QPD.3', parameters: PDQ_PARAMETERS,
                domainsField: 'QPD.8', namespaces: NAMESPACES,
                countParam: '_count', defaultCount: 10, quantityField: 'RCP.2', continuationField: 'DSC.1',
            }, inQuery('Q22')),
            // A continuation (DSC-1 set): the stored next page.
            step(32, 'Load Page', 'load_page', 'state.store', {
                operation: 'get', namespace: 'pdq_continuation', keyFrom: pageKey(PQ),
                when: `${PQ}.continuation`, matchToken: `${PQ}.continuation`,
            }, inQuery('Q22')),
            queryLookup(34, 'PDQ Search', 'pdq_search', 'Q22', 'identities.search identities.read.demographics', {
                method: 'POST', endpoint: '/fhir/r4/Patient/_search',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded', Accept: 'application/fhir+json' },
                // The stored next page, else the query's own search; a form
                // body keeps demographics out of URLs.
                bodyRef: `${PAGE}.value.search`, bodyRefFallbacks: [`${PQ}.query`],
                // 422: not one of the parameter combinations ezIdentitySense
                // accepts (it won't search on a family name alone).
                acceptStatuses: [400, 422],
            }),
            step(36, 'Save Page', 'save_page', 'state.store', {
                operation: 'put', namespace: 'pdq_continuation', keyFrom: pageKey(PQ),
                values: [
                    { name: 'search', path: `${SR}.link[relation=next].url`, transform: 'url_query' },
                    { name: 'tag', path: `${PQ}.query_tag` },
                ],
                requireValue: 'search', token: true, ttlSeconds: 600,
            }, { ...inQuery('Q22'), required: false, on_error_strategy: 'skip' }),
            step(38, 'PDQ Response', 'pdq_response', 'hl7.build', {
                messageType: 'RSP', triggerEvent: 'K22', messageStructure: 'RSP_K21', version: '2.5', escapeValues: true,
                mshFields: replyHeader(PQ, 'ITI-21^IHE'),
                derivedValues: [
                    { name: 'ack', default: 'AE', rules: [
                        { condition: is(`${PQ}.has_problems`, true), value: 'AE' },
                        { condition: is(`${PAGE}.missing`, true), value: 'AE' },
                        { condition: is(`${SR}.http_status`, 200), value: 'AA' },
                    ] },
                    { name: 'first_match', rules: [{ sourcePath: `${SR}.entry[0].full_url` }] },
                    { name: 'status', default: 'NF', rules: [
                        { condition: is('derived.ack', 'AE'), value: 'AE' },
                        { condition: exists('derived.first_match'), value: 'OK' },
                    ] },
                    { name: 'err_location', default: '', rules: [
                        { condition: is(`${PQ}.has_problems`, true), value: '' },
                        { condition: is(`${PAGE}.missing`, true), value: 'DSC^1^1' },
                        { condition: is(`${SR}.http_status`, 200), value: '' },
                        { condition: is(`${SR}.http_status`, 400), value: 'QPD^1^3' },
                        { condition: is(`${SR}.http_status`, 422), value: 'QPD^1^3' },
                    ] },
                    { name: 'err_code', default: INTERNAL_ERROR, rules: [
                        { condition: is(`${PQ}.has_problems`, true), value: '' },
                        { condition: is(`${PAGE}.missing`, true), value: UNKNOWN_KEY },
                        { condition: is(`${SR}.http_status`, 200), value: '' },
                        { condition: is(`${SR}.http_status`, 422), value: '101^Required field missing^HL70357' },
                    ] },
                    { name: 'err_text', default: 'ezIdentitySense could not answer the query.', rules: [
                        { condition: is(`${PQ}.has_problems`, true), value: '' },
                        { condition: is(`${PAGE}.missing`, true), value: 'The continuation pointer is not known or has expired.' },
                        { condition: is(`${SR}.http_status`, 200), value: '' },
                        { condition: is(`${SR}.http_status`, 400), value: 'ezIdentitySense refused the search.' },
                        { condition: is(`${SR}.http_status`, 422), value: 'The search needs more parameters: an identifier, or the family name with the birth date.' },
                    ] },
                    { name: 'total', rules: [
                        { condition: is('derived.ack', 'AE'), value: '' },
                        { sourcePath: `${SR}.total` },
                    ] },
                    // More results: DSC-1 names the stored next page.
                    { name: 'pointer', rules: [
                        { condition: is('derived.ack', 'AE'), value: '' },
                        { sourcePath: `${SAVED}.token` },
                    ] },
                ],
                segments: [
                    msa(PQ),
                    ...errSegments(PQ),
                    { segment: 'QAK', fields: [
                        { fieldKey: 'QAK.1', sourcePath: `${PQ}.query_tag`, encoded: true },
                        { fieldKey: 'QAK.2', sourcePath: 'derived.status' },
                        { fieldKey: 'QAK.4', sourcePath: 'derived.total' },
                    ] },
                    { segment: 'QPD', rawPath: `${PQ}.qpd_segment` },
                    pdqPID(),
                    { segment: 'DSC', condition: exists('derived.pointer'), fields: [
                        { fieldKey: 'DSC.1', sourcePath: 'derived.pointer' },
                        { fieldKey: 'DSC.2', literalValue: 'I' },
                    ] },
                ],
            }, inQuery('Q22')),

            // ── Cancel Query (QCN^J01) ──
            step(40, 'Cancel Query', 'cancel_query', 'hl7.query_parse', { tagField: 'QID.1' }, inQuery('J01')),
            step(42, 'Forget Page', 'forget_page', 'state.store', {
                operation: 'delete', namespace: 'pdq_continuation', keyFrom: pageKey(CQ),
            }, { ...inQuery('J01'), required: false, on_error_strategy: 'skip' }),
            step(44, 'Cancel Response', 'cancel_response', 'hl7.build', {
                messageType: 'ACK', triggerEvent: 'J01', messageStructure: 'ACK', version: '2.5', escapeValues: true,
                mshFields: replyHeader(CQ),
                derivedValues: [{ name: 'ack', default: 'AA', rules: [] }],
                segments: [msa(CQ)],
            }, inQuery('J01')),

            // ── Anything else: rejected (AR), per IHE ITI TF-2 Appendix C ──
            step(50, 'Other Message', 'other_message', 'hl7.query_parse', {}, inQuery('default')),
            step(52, 'Reject Message', 'reject_message', 'hl7.build', {
                messageType: 'ACK', version: '2.5', escapeValues: true,
                mshFields: [
                    ...replyHeader(OQ),
                    { fieldKey: 'MSH.9.1', literalValue: 'ACK' },
                    { fieldKey: 'MSH.9.2', sourcePath: `${OQ}.trigger_event` },
                    { fieldKey: 'MSH.9.3', literalValue: 'ACK' },
                ],
                derivedValues: [
                    { name: 'ack', default: 'AR', rules: [] },
                    { name: 'known_type', rules: [
                        { condition: { field: `${OQ}.message_type`, operator: 'in_list', value: ['QBP', 'QCN'] }, value: 'yes' },
                    ] },
                    { name: 'err_location', default: 'MSH^1^9^1^1', rules: [{ condition: exists('derived.known_type'), value: 'MSH^1^9^1^2' }] },
                    { name: 'err_code', default: '200^Unsupported message type^HL70357',
                      rules: [{ condition: exists('derived.known_type'), value: '201^Unsupported event code^HL70357' }] },
                    { name: 'err_text', default: 'This port answers PIX and PDQ queries only.', rules: [] },
                ],
                segments: [msa(OQ), ...errSegments(OQ)],
            }, inQuery('default')),
        ],
    },
    required_connection_fields: [
        { section: 'source', field: 'port', label: 'MLLP Port', type: 'number', required: true,
          hint: 'The port PIX and PDQ consumers send their queries to.' },
        { section: 'variables', section_label: 'Patient Identifier Domains', field: 'namespaces',
          label: 'Assigning Authorities by Namespace', type: 'keyvalue', required: false,
          placeholder: 'HOSPA=1.2.840.99999.1',
          hint: 'For domains sent by namespace only, NAMESPACE=OID (or the identifier system URI), one per line. Replies name each domain by its namespace from this list; a domain not listed is named by its OID.' },
        ...ezisConnectionFields(['pix_lookup', 'pdq_search']),
    ],
    preview_steps: [
        { name: 'Receive Query over MLLP', icon: '🔌', step_type: 'connector.inbound' },
        { name: 'PIX, PDQ or Cancel', icon: '🔀', step_type: 'switch_case' },
        { name: 'Read the Query', icon: '🔎', step_type: 'hl7.query_parse' },
        { name: 'ezIdentitySense PIXm / PDQm', icon: '📤', step_type: 'enrichment.api' },
        { name: 'Build RSP', icon: '🔄', step_type: 'hl7.build' },
    ],
    estimated_setup_minutes: 15,
    processing_order: 'parallel',
    guide: 'pix-pdq-to-ezidentitysense.md',
};

const TEMPLATES = [outbound, inbound, queries];

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

-- Default port: 8088, inside the published HTTP range and outside the
-- ports the connector forms reserve (8081, 8090-8097).
UPDATE connectivity_types
SET config_schema = jsonb_set(config_schema, '{properties,port,default}', '8088'::jsonb),
    updated_at = NOW()
WHERE type_name = 'webhook_events_inbound'
  AND config_schema->'properties'->'port'->>'default' = '8095';

UPDATE connectivity_types
SET config_schema = jsonb_set(config_schema, '{properties,event_types}',
        '{"type":"array","items":{"type":"string"},"title":"Event Types","description":"Only these event types are processed; others are acknowledged and skipped. Empty = all."}'::jsonb),
    parameter_groups = jsonb_set(parameter_groups, '{advanced}', (parameter_groups->'advanced') || '["event_types"]'::jsonb),
    updated_at = NOW()
WHERE type_name = 'webhook_events_inbound'
  AND NOT (config_schema->'properties' ? 'event_types');
${TEMPLATES.map(templateSQL).join('')}`);
