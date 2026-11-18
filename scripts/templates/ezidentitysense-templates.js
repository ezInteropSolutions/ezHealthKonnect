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

// The trigger events these templates can build, and the message structure
// each uses (MSH-9.3). A site's own event table (the triggerOverrides
// variable) may only choose among them: a registration or update can be sent
// as A28/A31 (the default) or as A04/A08; merges, unlinks and deletes keep
// their own events, which carry segments the others lack.
const TRIGGER_STRUCTURES = {
    A28: 'ADT_A05', A31: 'ADT_A05', A04: 'ADT_A01', A08: 'ADT_A01',
    A40: 'ADT_A39', A37: 'ADT_A37', A29: 'ADT_A21',
};

// The setup field for that table, keyed by the template's own event names.
function triggerOverridesField(example) {
    return { section: 'variables', section_label: 'ADT Trigger Events', field: 'triggerOverrides',
        label: 'Which ADT Event Each Change Sends', type: 'keyvalue', required: false, placeholder: example,
        hint: 'Optional, one CHANGE=EVENT per line, to send a change as another ADT event. A28, A31, A04 and A08 can stand in for each other; merges (A40), unlinks (A37) and deletes (A29) always use their own events. Left blank, the guide\'s table applies.' };
}

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
                triggerEventMapOverrides: '{{var.triggerOverrides}}',
                messageStructureMap: TRIGGER_STRUCTURES,
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
        triggerOverridesField('record.updated=A08, identity.created=A04'),
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

// What each HL7 trigger event does in ezIdentitySense. The Switch/Case
// step translates MSH-9.2 through this table (valueMap) and routes on the
// result; a site changes any line in the setup form (valueMapOverrides).
// An event with no line registers the PID's patient, as before.
const EVENT_ACTIONS = {
    A40: 'merge', A30: 'merge', A34: 'merge', A18: 'merge',
    A23: 'remove', A29: 'remove',
    A47: 'replace_id',
    // Links and account-level events change nothing here: a source's own
    // claim that two patients are one must not bypass matching, and an
    // account is not an identity. They fail visibly (NACK) instead.
    A24: 'unsupported', A37: 'unsupported', A35: 'unsupported', A36: 'unsupported',
    A41: 'unsupported', A42: 'unsupported', A43: 'unsupported', A44: 'unsupported', A45: 'unsupported',
};

// The record in MRG-1 was merged, in the source, into the one in PID-3.
function mergeRecordStep(seq, name, alias, caseValue) {
    return feedStep(seq, name, alias, caseValue, {
        method: 'PUT', endpoint: FEED + '{retiredId}',
        fieldMappings: { retiredId: 'MRG.1.1', survivingId: LOCAL_ID },
        requestBody: {
            resourceType: 'Patient',
            active: false,
            identifier: [{ system: '{{var.sourceAuthority}}', value: 'retiredId' }],
            link: [{ type: 'replaced-by', other: { identifier: { system: '{{var.sourceAuthority}}', value: 'survivingId' } } }],
        },
    });
}

const registerBody = { method: 'PUT', endpoint: FEED + '{localId}', fieldMappings: { localId: LOCAL_ID }, bodyRef: PATIENT };

const INBOUND_STEPS = ['register_patient', 'merge_record', 'remove_record', 'register_new_id', 'merge_old_id'];

const inbound = {
    name: 'HL7 ADT to ezIdentitySense',
    slug: 'hl7-adt-to-ezidentitysense',
    description: 'Feeds a source system\'s HL7 v2 ADT messages into ezIdentitySense (the enterprise master patient index) through its standard IHE PIXm Patient Identity Feed: registrations and updates (A01, A04, A05, A08, A28, A31 and any other ADT carrying PID) register or update the patient; A40 (and the older A30, A34, A18) merge two of the source\'s records; A47 changes a patient\'s identifier; A23 and A29 remove one. What each event does can be changed in the setup form. Processed one at a time, in order; an older message never overwrites a newer one.',
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
                    ack: { mode: 'after_processing', on_error: 'nack', sending_app: 'EZHEALTHKONNECT', sending_facility: '' },
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
                valueMap: EVENT_ACTIONS,
                valueMapOverrides: '{{var.eventActions}}',
                cases: [
                    { value: 'merge', label: 'Merge', actions: [] },
                    { value: 'remove', label: 'Remove', actions: [] },
                    { value: 'replace_id', label: 'Identifier changed', actions: [] },
                    { value: 'ignore', label: 'Ignore', actions: [] },
                    { value: 'unsupported', label: 'Not supported', actions: [{ action: 'reject',
                        errorMessage: 'This ADT event is not applied to ezIdentitySense. Links and account-level events never change an identity on their own; see the interface guide, or choose what the event does in this interface\'s settings.' }] },
                ],
                default: { actions: [] },
                options: { caseInsensitive: true, trimWhitespace: true },
            }),
            feedStep(30, 'Register Patient', 'register_patient', 'default', registerBody),
            mergeRecordStep(32, 'Merge Record', 'merge_record', 'merge'),
            feedStep(34, 'Remove Record', 'remove_record', 'remove', {
                method: 'DELETE', endpoint: FEED + '{localId}', fieldMappings: { localId: LOCAL_ID },
            }),
            // A47: PID-3 holds the new identifier, MRG-1 the old. The new
            // one is registered, then the old record is merged into it.
            feedStep(36, 'Register New ID', 'register_new_id', 'replace_id', registerBody),
            mergeRecordStep(38, 'Merge Old ID', 'merge_old_id', 'replace_id'),
        ],
    },
    required_connection_fields: [
        { section: 'source', field: 'port', label: 'MLLP Port', type: 'number', required: true,
          hint: 'The port the source system sends its ADT messages to.' },
        { section: 'variables', section_label: 'Source System', field: 'sourceAuthority',
          label: 'This Source\'s ID Authority', type: 'string', required: true,
          hint: 'The ID authority set for this source system in ezIdentitySense, e.g. urn:oid:2.16.840.1.113883.3.999.1. Its PID-3 identifier is the record\'s local ID.' },
        { section: 'variables', field: 'eventActions', label: 'What Each ADT Event Does', type: 'keyvalue', required: false,
          placeholder: 'A02=ignore, A24=ignore',
          hint: 'Optional, one EVENT=ACTION per line. Actions: register, merge, remove, replace_id (identifier changed), ignore, unsupported (refuse the message). Left blank, the defaults in the guide apply; any event not listed registers the patient.' },
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
                // A query naming an unknown page is answered AE; it must not
                // replace a page stored for the same tag.
                skipWhen: `${PAGE}.missing`,
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

// ── Templates 4 and 5: ezIdentitySense FHIR Subscriptions (topic-based) ────
//
// ezIdentitySense speaks FHIR; ezHealthKonnect speaks HL7 v2 (decided
// 2026-11-12). These templates subscribe to its topics (HL7 Subscriptions
// R5 Backport, connector fhir_subscription_inbound) and turn each event
// into HL7 v2 ADT, or forward the Patients to another FHIR server. The
// notifications carry the Patients (full-resource), so neither calls back.
// Generated into V294 (node … fhir).

const TOPIC_BASE = 'https://ezinteropsolutions.com/fhir/ezidentitysense/SubscriptionTopic/';
const TOPICS = ['patient-created', 'patient-changed', 'patient-records', 'patient-merged', 'patient-separated', 'patient-erased']
    .map(t => TOPIC_BASE + t);

// The subscription connector, credentials blank (asked for at setup).
function subscriptionConnector() {
    return step(5, 'ezIdentitySense Subscriptions', 'ez_identity_sense_subscriptions', 'connector.inbound', {
        connectorType: 'fhir_subscription_inbound',
        config: {
            port: 8089, endpoint_path: '/fhir-notifications', callback_url: '', fhir_base_url: '',
            topics: TOPICS, filters: [], content: 'full-resource', heartbeat_seconds: 0,
            auth_type: 'oauth2', oauth2_token_url: '', oauth2_client_id: '', oauth2_client_secret: '', oauth2_audience: '',
            oauth2_scope: 'identities.subscribe identities.read identities.read.demographics',
            reply_within_seconds: 8, event_timeout_seconds: 300,
        },
        timeoutMs: 30000,
    });
}

function subscriptionSetupFields() {
    const f = (field, label, type, hint, required = true) => ({ section: 'source', field, label, type, hint, required });
    return [
        f('port', 'Listen Port', 'number',
            'The port ezHealthKonnect listens on for ezIdentitySense\'s notifications, e.g. 8089. Each interface needs a port of its own: two interfaces on one port stop both.'),
        f('callback_url', 'Address ezIdentitySense Sends To', 'url',
            'The https address ezIdentitySense reaches this port at, ending in /fhir-notifications, e.g. https://ezhealthkonnect.example.org:8089/fhir-notifications'),
        f('fhir_base_url', 'ezIdentitySense FHIR Address', 'url', 'Its FHIR base, e.g. https://empi.example.org/fhir/r4'),
        f('oauth2_token_url', 'Token URL', 'url', 'Your identity provider\'s OAuth 2.0 token endpoint'),
        f('oauth2_client_id', 'Client ID', 'string', 'The client registered for ezHealthKonnect in your identity provider and in ezIdentitySense'),
        f('oauth2_client_secret', 'Client Secret', 'password', 'That client\'s secret. Stored encrypted.'),
        f('oauth2_audience', 'Audience', 'string', 'Only if your identity provider requires one', false),
    ];
}

// A Patient carried in an event (message.<base>.resource, FHIR's own key
// names) as a PID; setId is PID-1. An erased patient carries no resource:
// its EID alone.
function pidFromEventPatient(base, setId, condition) {
    const R = `${base}.resource`;
    const erased = { field: `${base}.erased`, operator: 'equals', value: true };
    return {
        segment: 'PID', ...(condition ? { condition } : {}),
        fields: [
            { fieldKey: 'PID.1', literalValue: setId },
            { fieldKey: 'PID.3', repeatPath: `${R}.identifier`, components: [
                { position: '1', sourcePath: 'value' },
                { position: '4.2', sourcePath: 'system' },
                { position: '4.3', literalValue: 'URI' },
            ] },
            { fieldKey: 'PID.3.1', sourcePath: `${base}.id`, condition: erased },
            { fieldKey: 'PID.5', repeatPath: `${R}.name`, components: [
                { position: '1', sourcePath: 'family' },
                { position: '2', sourcePath: 'given[0]' },
                { position: '3', sourcePath: 'given[1]' },
                { position: '5', sourcePath: 'prefix[0]' },
                { position: '4', sourcePath: 'suffix[0]' },
                { position: '7', sourcePath: 'use', valueMap: { official: 'L', usual: 'D', nickname: 'N', maiden: 'M', old: 'U', anonymous: 'S' } },
            ] },
            { fieldKey: 'PID.6', repeatPath: `${R}.extension`, maxRepetitions: 1,
              itemCondition: { field: 'url', operator: 'equals', value: 'http://hl7.org/fhir/StructureDefinition/patient-mothersMaidenName' },
              components: [{ position: '1', sourcePath: 'valueString' }] },
            { fieldKey: 'PID.7', sourcePath: `${R}.birthDate`, transform: 'hl7_date' },
            { fieldKey: 'PID.8', sourcePath: `${R}.gender`, valueMap: { male: 'M', female: 'F', other: 'O', unknown: 'U' } },
            { fieldKey: 'PID.11', repeatPath: `${R}.address`, components: [
                { position: '1', sourcePath: 'line[0]' },
                { position: '2', sourcePath: 'line[1]' },
                { position: '3', sourcePath: 'city' },
                { position: '4', sourcePath: 'state' },
                { position: '5', sourcePath: 'postalCode' },
                { position: '6', sourcePath: 'country', transform: 'iso3166_alpha3' },
            ] },
            { fieldKey: 'PID.13', repeatPath: `${R}.telecom`,
              itemCondition: { field: 'system', operator: 'equals', value: 'phone' },
              components: [
                { position: '1', sourcePath: 'value' },
                { position: '2', sourcePath: 'use', valueMap: { home: 'PRN', work: 'WPN', mobile: 'PRS', temp: 'PRN', old: 'PRN' } },
                { position: '3', literalValue: 'PH' },
              ] },
            { fieldKey: 'PID.29', sourcePath: `${R}.deceasedDateTime`, transform: 'hl7_datetime' },
            { fieldKey: 'PID.30', sourcePath: `${R}.deceasedBoolean`, valueMap: { true: 'Y', false: 'N' } },
        ],
    };
}

const subscriptionToADT = {
    name: 'ezIdentitySense FHIR Subscription to HL7 ADT',
    slug: 'ezidentitysense-fhir-subscription-to-hl7-adt',
    description: 'Subscribes to ezIdentitySense\'s FHIR topics (new patient, details changed, records linked, merged, merge undone or split, erased) and sends each change to a downstream system as HL7 v2 ADT: A28, A31, A40, A37, A29. The subscriptions are made and renewed for you; each change is processed one at a time, in order, and ezIdentitySense is answered only once the ADT has been accepted, so nothing is lost or reordered.',
    category: 'empi',
    subcategory: 'ezidentitysense',
    tags: ['ezidentitysense', 'empi', 'mpi', 'fhir', 'subscription', 'hl7v2', 'adt', 'outbound'],
    icon: '📡',
    difficulty: 'intermediate',
    message_type: 'FHIR Subscription',
    source_connector_type: 'fhir_subscription_inbound',
    source_config_template: { port: 8089, endpoint_path: '/fhir-notifications' },
    target_connector_type: 'tcp_mllp_outbound',
    target_config_template: { host: '', port: 2575 },
    pipeline_config: {
        execution_groups: [
            subscriptionConnector(),
            step(10, 'Build ADT', 'build_adt', 'hl7.build', {
                messageType: 'ADT', version: '2.5.1', escapeValues: true,
                sendingApplication: 'EZIDENTITYSENSE', sendingFacility: '', receivingApplication: '', receivingFacility: '',
                triggerEventPath: 'message.topic',
                triggerEventMap: {
                    'patient-created': 'A28', 'patient-changed': 'A31', 'patient-records': 'A31',
                    'patient-merged': 'A40', 'patient-separated': 'A37', 'patient-erased': 'A29',
                },
                triggerEventMapOverrides: '{{var.triggerOverrides}}',
                messageStructureMap: TRIGGER_STRUCTURES,
                segments: [
                    { segment: 'EVN', fields: [{ fieldKey: 'EVN.2', sourcePath: 'message.timestamp', transform: 'hl7_datetime' }] },
                    pidFromEventPatient('message.focus', '1'),
                    { segment: 'PV1', condition: { field: 'message.topic', operator: 'in_list',
                        value: ['patient-created', 'patient-changed', 'patient-records', 'patient-erased'] },
                      fields: [{ fieldKey: 'PV1.1', literalValue: '1' }, { fieldKey: 'PV1.2', literalValue: 'N' }] },
                    // A40: the identity merged away (the focus is the survivor).
                    { segment: 'MRG', condition: { field: 'message.topic', operator: 'equals', value: 'patient-merged' },
                      fields: [{ fieldKey: 'MRG.1.1', sourcePath: 'message.context[0].id' }] },
                    // A37: the second, now separate, identity.
                    pidFromEventPatient('message.context[0]', '2', { field: 'message.topic', operator: 'equals', value: 'patient-separated' }),
                ],
            }),
            step(20, 'Send ADT', 'send_adt', 'connector.outbound', {
                connectorType: 'tcp_mllp_outbound',
                config: { host: '', port: 2575, expect_ack: true, ack_timeout_seconds: 30, connect_timeout_seconds: 10 },
                contentField: 'hl7Message',
                contentType: 'x-application/hl7-v2+er7',
            }),
        ],
    },
    required_connection_fields: [
        ...subscriptionSetupFields(),
        triggerOverridesField('patient-changed=A08, patient-created=A04'),
        { section: 'steps', section_label: 'Downstream System', step_aliases: ['build_adt'], field: 'receivingApplication',
          label: 'Receiving Application (MSH-5)', type: 'string', required: false, hint: 'Optional: the receiving system\'s application name.' },
        { section: 'steps', section_label: 'Downstream System', step_aliases: ['build_adt'], field: 'receivingFacility',
          label: 'Receiving Facility (MSH-6)', type: 'string', required: false, hint: 'Optional: the receiving system\'s facility.' },
        { section: 'target', field: 'host', label: 'Downstream System Host', type: 'string', required: true,
          hint: 'The host name or address of the system that receives the ADT messages over MLLP.' },
        { section: 'target', field: 'port', label: 'Downstream System Port', type: 'number', required: true, hint: 'Its MLLP port.' },
    ],
    preview_steps: [
        { name: 'Receive FHIR Notification', icon: '📡', step_type: 'connector.inbound' },
        { name: 'Build ADT', icon: '🔄', step_type: 'hl7.build' },
        { name: 'Send over MLLP', icon: '📤', step_type: 'connector.outbound' },
    ],
    estimated_setup_minutes: 15,
    processing_order: 'in_order',
    guide: 'fhir-subscription-to-adt.md',
};

// The target FHIR server: address and credential, asked for at setup.
function targetApi(scopeHint) {
    return { baseUrl: '', authType: 'oauth2', oauth2GrantType: 'client_credentials', oauth2TokenUrl: '', oauth2ClientId: '',
        oauth2ClientSecret: '', oauth2Audience: '', oauth2Scope: scopeHint, encodePlaceholders: true, timeoutMs: 15000, retryCount: 2,
        retryDelayMs: 1000, headers: { 'Content-Type': 'application/fhir+json', Accept: 'application/fhir+json' } };
}

function forwardStep(seq, name, alias, caseValue, cfg) {
    return step(seq, name, alias, 'enrichment.api', { ...targetApi('system/Patient.write'), ...cfg },
        { parent_conditional_step_id: 'change_kind', case_value: caseValue });
}

const FORWARD_STEPS = ['put_patient', 'put_survivor', 'put_retired', 'put_restored', 'put_separated', 'delete_patient'];

const subscriptionToFHIR = {
    name: 'ezIdentitySense FHIR Subscription to FHIR Server',
    slug: 'ezidentitysense-fhir-subscription-to-fhir-server',
    description: 'Keeps another FHIR server\'s copy of ezIdentitySense\'s patients current: subscribes to its FHIR topics and writes each change there, as the golden Patient, keyed by the enterprise ID. A merge updates both patients (the retired one points to the survivor), an erasure deletes. Processed one at a time, in order; nothing is lost or reordered.',
    category: 'empi',
    subcategory: 'ezidentitysense',
    tags: ['ezidentitysense', 'empi', 'mpi', 'fhir', 'subscription', 'patient', 'outbound'],
    icon: '📡',
    difficulty: 'intermediate',
    message_type: 'FHIR Subscription',
    source_connector_type: 'fhir_subscription_inbound',
    source_config_template: { port: 8089, endpoint_path: '/fhir-notifications' },
    target_connector_type: 'http_outbound',
    target_config_template: {},
    pipeline_config: {
        execution_groups: [
            subscriptionConnector(),
            step(10, 'Change Kind', 'change_kind', 'switch_case', {
                field: 'message.topic',
                cases: [
                    { value: 'patient-merged', label: 'Merged', actions: [] },
                    { value: 'patient-separated', label: 'Separated', actions: [] },
                    { value: 'patient-erased', label: 'Erased', actions: [] },
                ],
                default: { actions: [] },
                options: { caseInsensitive: false, trimWhitespace: true },
            }),
            // New patient, details changed, records: the Patient as it is now.
            forwardStep(20, 'Put Patient', 'put_patient', 'default', {
                method: 'PUT', endpoint: '/Patient/{id}', fieldMappings: { id: 'message.focus.id' }, bodyRef: 'message.focus.resource',
            }),
            // Merged: the survivor, then the retired one (inactive, replaced-by).
            forwardStep(22, 'Put Survivor', 'put_survivor', 'patient-merged', {
                method: 'PUT', endpoint: '/Patient/{id}', fieldMappings: { id: 'message.focus.id' }, bodyRef: 'message.focus.resource',
            }),
            forwardStep(24, 'Put Retired', 'put_retired', 'patient-merged', {
                method: 'PUT', endpoint: '/Patient/{id}', fieldMappings: { id: 'message.context[0].id' }, bodyRef: 'message.context[0].resource',
            }),
            // Separated: both identities as they are now.
            forwardStep(26, 'Put Restored', 'put_restored', 'patient-separated', {
                method: 'PUT', endpoint: '/Patient/{id}', fieldMappings: { id: 'message.focus.id' }, bodyRef: 'message.focus.resource',
            }),
            forwardStep(28, 'Put Separated', 'put_separated', 'patient-separated', {
                method: 'PUT', endpoint: '/Patient/{id}', fieldMappings: { id: 'message.context[0].id' }, bodyRef: 'message.context[0].resource',
            }),
            // Erased: removed there too; already gone is fine.
            forwardStep(30, 'Delete Patient', 'delete_patient', 'patient-erased', {
                method: 'DELETE', endpoint: '/Patient/{id}', fieldMappings: { id: 'message.focus.id' }, acceptStatuses: [404, 410],
            }),
        ],
    },
    required_connection_fields: [
        ...subscriptionSetupFields(),
        ...[['baseUrl', 'Target FHIR Address', 'url', 'The FHIR base of the server to keep current, e.g. https://fhir.example.org/r4', true],
            ['oauth2TokenUrl', 'Target Token URL', 'url', 'The target\'s OAuth 2.0 token endpoint', true],
            ['oauth2ClientId', 'Target Client ID', 'string', 'The client ezHealthKonnect uses there', true],
            ['oauth2ClientSecret', 'Target Client Secret', 'password', 'Its secret. Stored encrypted.', true],
            ['oauth2Scope', 'Target Scope', 'string', 'The scope the target needs to write Patients (default system/Patient.write)', false],
        ].map(([field, label, type, hint, required]) => ({ section: 'steps', section_label: 'Target FHIR Server', step_aliases: FORWARD_STEPS,
            field, label, type, hint, required })),
    ],
    preview_steps: [
        { name: 'Receive FHIR Notification', icon: '📡', step_type: 'connector.inbound' },
        { name: 'Change Kind', icon: '🔀', step_type: 'switch_case' },
        { name: 'Write to FHIR Server', icon: '📤', step_type: 'enrichment.api' },
    ],
    estimated_setup_minutes: 15,
    processing_order: 'in_order',
    guide: 'fhir-subscription-to-fhir-server.md',
};

// ── Template: FHIR Patient relay -> ezIdentitySense ────────────────────────
//
// A client system that already speaks FHIR (a FHIR server, an app) sends its
// Patient create and update here, as it would to any FHIR server; each is
// forwarded to ezIdentitySense's patient feed (IHE PIXm, ITI-104). Two ways
// to answer, chosen at setup ("immediateReply"): acknowledge at once and
// process in order (default), or process first and answer with
// ezIdentitySense's own status and OperationOutcome.
//
// The Patient is sent exactly as received (the receiver keeps the original
// text in "raw"); its
// local ID is the identifier in the source's ID authority.

const RELAY_STEPS = ['put_patient', 'delete_patient'];
const RELAY_LOCAL_ID = 'message.identifier[system={{var.sourceAuthority}}].value';

function relayStep(seq, name, alias, caseValue, cfg) {
    return step(seq, name, alias, 'enrichment.api', { ...ezisApiAuth('records.write'), headers: FHIR_JSON, ...cfg },
        { parent_conditional_step_id: 'request_kind', case_value: caseValue });
}

const fhirRelay = {
    name: 'FHIR Patient Relay to ezIdentitySense',
    slug: 'fhir-patient-relay-to-ezidentitysense',
    description: 'Lets a system that speaks FHIR send its Patient creates and updates here, as to any FHIR server, and feeds each to ezIdentitySense (the enterprise master patient index) through its IHE PIXm patient feed. Two modes, chosen at setup: acknowledge at once and send on in order (default), or send first and answer with ezIdentitySense\'s own reply, so the sender knows straight away whether the patient was accepted. A delete deactivates the record.',
    category: 'empi',
    subcategory: 'ezidentitysense',
    tags: ['ezidentitysense', 'empi', 'mpi', 'fhir', 'patient', 'pixm', 'iti-104', 'patient-feed', 'inbound'],
    icon: '🔁',
    difficulty: 'intermediate',
    message_type: 'FHIR:Patient',
    source_connector_type: 'http_fhir_inbound',
    source_config_template: { port: 8086, basePath: '/fhir/r4' },
    target_connector_type: 'http_outbound',
    target_config_template: {},
    pipeline_config: {
        execution_groups: [
            step(5, 'FHIR Patient Feed', 'fhir_patient_feed', 'connector.inbound', {
                connectorType: 'http_fhir_inbound',
                config: {
                    port: 8086, basePath: '/fhir/r4', fhirVersion: 'R4', bundleMode: 'bundle_as_unit',
                    allowedMethods: ['POST', 'PUT', 'PATCH', 'DELETE'],
                    syncReply: { enabled: '{{var.immediateReply}}', timeoutSeconds: 20, serialize: true },
                },
                timeoutMs: 30000,
            }),
            step(10, 'Request Kind', 'request_kind', 'switch_case', {
                field: 'message._http.method',
                cases: [{ value: 'DELETE', label: 'Delete', actions: [] }],
                default: { actions: [] },
                options: { caseInsensitive: true, trimWhitespace: true },
            }),
            // Create or update: the Patient as received.
            relayStep(20, 'Put Patient', 'put_patient', 'default', {
                method: 'PUT', endpoint: FEED + '{localId}', fieldMappings: { localId: RELAY_LOCAL_ID },
                bodyRef: 'message.raw',
            }),
            // Delete: the record in the URL (its local ID) is deactivated.
            relayStep(30, 'Delete Patient', 'delete_patient', 'DELETE', {
                method: 'DELETE', endpoint: FEED + '{localId}', fieldMappings: { localId: 'message._http.resourceId' },
            }),
        ],
    },
    required_connection_fields: [
        { section: 'source', field: 'port', label: 'FHIR Port', type: 'number', required: true,
          hint: 'The port your system sends Patients to, at /fhir/r4/Patient.' },
        { section: 'source', field: 'authType', label: 'Who May Send', type: 'select', required: true,
          options: [{ value: 'bearer', label: 'A bearer token' }, { value: 'api_key', label: 'An API key' },
                    { value: 'basic', label: 'A user name and password' },
                    { value: 'none', label: 'Anyone (testing only: patient data)' }],
          hint: 'How the sending system proves who it is.' },
        { section: 'source', field: 'bearerToken', label: 'Bearer Token', type: 'password', required: false,
          hint: 'If you chose a bearer token. Stored encrypted.' },
        { section: 'source', field: 'apiKey', label: 'API Key', type: 'password', required: false,
          hint: 'If you chose an API key (sent in the X-API-Key header). Stored encrypted.' },
        { section: 'source', field: 'username', label: 'User Name', type: 'string', required: false,
          hint: 'If you chose a user name and password.' },
        { section: 'source', field: 'password', label: 'Password', type: 'password', required: false,
          hint: 'If you chose a user name and password. Stored encrypted.' },
        { section: 'variables', section_label: 'Source System', field: 'sourceAuthority',
          label: 'This Source\'s ID Authority', type: 'string', required: true,
          hint: 'The ID authority set for this source system in ezIdentitySense, e.g. urn:oid:2.16.840.1.113883.3.999.1. The Patient\'s identifier with this system is its local ID.' },
        { section: 'variables', section_label: 'Reply', field: 'immediateReply', label: 'Answer With ezIdentitySense\'s Own Reply',
          type: 'boolean', required: false,
          hint: 'Off (default): reply "accepted" at once and send each Patient on in order; a refusal shows in this interface\'s message list. On: send first and reply with ezIdentitySense\'s own status and OperationOutcome; one request at a time; requests are not listed in the message list. See the guide.' },
        ...ezisConnectionFields(RELAY_STEPS),
    ],
    preview_steps: [
        { name: 'Receive FHIR Patient', icon: '🔌', step_type: 'connector.inbound' },
        { name: 'Create, Update or Delete', icon: '🔀', step_type: 'switch_case' },
        { name: 'ezIdentitySense Patient Feed', icon: '📤', step_type: 'enrichment.api' },
    ],
    estimated_setup_minutes: 10,
    processing_order: 'in_order',
    guide: 'fhir-relay-to-ezidentitysense.md',
};

// ── Templates: CSV file load -> ezIdentitySense ────────────────────────────
//
// A file of patients dropped in a folder, on an SFTP server or in an S3
// bucket is loaded into ezIdentitySense by its bulk-load API: the column
// mapping from the setup form is saved, the file is uploaded, a dry run
// checks every row, and only a clean dry run is followed by the load. Each
// job is waited for (a generic "poll until" on the API step), and a refused
// row or a job that does not complete fails the message, with the per-row
// reasons in the "Check Rejects" or "Load Rejects" step. ezIdentitySense
// runs the job itself: it keeps its place in the file across a restart or a
// pause, and loads rows in file order. Files are taken one at a time, in
// the order received (processing order), so a second file waits for the
// first.
//
// The client needs only the bulk.load scope.

const CSV_STEPS = ['save_mapping', 'upload_file', 'start_check', 'wait_check', 'check_rejects',
    'start_load', 'wait_load', 'load_rejects'];
const JOB_DONE = ['completed', 'failed', 'cancelled', 'paused', 'interrupted'];

function csvApi(seq, name, alias, cfg) {
    return step(seq, name, alias, 'enrichment.api', { ...ezisApiAuth('bulk.load'),
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, ...cfg });
}

// A job's state, awaited: every terminal state stops the wait, and only
// "completed" lets the pipeline go on.
function waitJob(seq, name, alias, startAlias) {
    return csvApi(seq, name, alias, {
        method: 'GET', endpoint: '/v1/bulk/jobs/{jobId}',
        fieldMappings: { jobId: `steps.${startAlias}.step_output.id` },
        poll: { untilField: 'state', untilValues: JOB_DONE, intervalSeconds: 2, timeoutSeconds: 3600 },
        timeoutMs: 15000,
    });
}

function stopUnless(seq, name, alias, field, wantValue, label, message) {
    return step(seq, name, alias, 'switch_case', {
        field, cases: [{ value: wantValue, label, actions: [] }],
        default: { actions: [{ action: 'reject', errorMessage: message }] },
        options: { caseInsensitive: true, trimWhitespace: true },
    });
}

const CSV_VARIANTS = {
    folder: {
        title: 'a Folder', slug: 'folder', icon: '📂', type: 'file_listener',
        where: 'a folder on the server this interface runs on',
        connector: { directory_path: '', file_pattern: '*.csv', polling_interval: 30, after_processing: 'archive',
            archive_path: '', create_dirs: false },
        fields: [
            ['directory_path', 'Folder to Watch', 'string', true, 'The folder where the CSV files are placed, as the server sees it.'],
            ['archive_path', 'Folder for Loaded Files', 'string', true, 'A processed file is moved here, so it is not loaded twice.'],
            ['file_pattern', 'File Name Pattern', 'string', false, 'Which files to take; default *.csv.'],
        ],
    },
    sftp: {
        title: 'SFTP', slug: 'sftp', icon: '🔐', type: 'sftp_inbound',
        where: 'an SFTP server',
        connector: { host: '', port: 22, username: '', auth_type: 'password', password: '', key_content: '', remote_dir: '',
            file_pattern: '*.csv', poll_interval_sec: 60, after_processing: 'archive', archive_dir: '' },
        fields: [
            ['host', 'SFTP Server', 'string', true, 'The server\'s host name or address.'],
            ['port', 'Port', 'number', false, 'Default 22.'],
            ['username', 'User Name', 'string', true, ''],
            ['password', 'Password', 'password', false, 'If the server uses a password. Stored encrypted.'],
            ['key_content', 'Private Key', 'password', false, 'If the server uses a key instead: the key\'s text. Stored encrypted.'],
            ['remote_dir', 'Folder to Watch', 'string', true, 'The folder on the server where the CSV files are placed.'],
            ['archive_dir', 'Folder for Loaded Files', 'string', true, 'A processed file is moved here, so it is not loaded twice.'],
        ],
    },
    s3: {
        title: 'S3', slug: 's3', icon: '🪣', type: 'aws_s3_inbound',
        where: 'an S3 bucket (or a compatible store)',
        connector: { bucket: '', region: '', prefix: '', file_pattern: '*.csv', access_key_id: '', secret_access_key: '',
            endpoint: '', force_path_style: false, after_processing: 'archive', archive_prefix: '', polling_interval: 60 },
        fields: [
            ['bucket', 'Bucket', 'string', true, ''],
            ['region', 'Region', 'string', true, 'e.g. us-east-1'],
            ['prefix', 'Folder in the Bucket', 'string', false, 'Only objects under this prefix are taken, e.g. incoming/'],
            ['access_key_id', 'Access Key ID', 'string', true, ''],
            ['secret_access_key', 'Secret Access Key', 'password', true, 'Stored encrypted.'],
            ['archive_prefix', 'Folder for Loaded Files', 'string', true, 'A processed object is moved under this prefix, so it is not loaded twice.'],
            ['endpoint', 'Store Address', 'url', false, 'Only for a store that is not Amazon S3 (e.g. MinIO); leave blank for Amazon. Such a store is reached by path (host/bucket/key).'],
        ],
    },
};

function csvLoad(v) {
    return {
        name: `CSV Load from ${v.title} to ezIdentitySense`,
        slug: `csv-load-${v.slug}-to-ezidentitysense`,
        description: `Loads a CSV file of patients from ${v.where} into ezIdentitySense (the enterprise master patient index) by its bulk-load API. You say which column holds what in the setup form; each file is uploaded, checked row by row with nothing registered, and loaded only if the check refuses nothing. A refused row or an unfinished job fails the message, with the reason for each row. Files are taken one at a time, in order; ezIdentitySense runs the load and keeps its place across a pause or restart.`,
        category: 'empi',
        subcategory: 'ezidentitysense',
        tags: ['ezidentitysense', 'empi', 'mpi', 'csv', 'bulk', 'batch', 'load', 'inbound'],
        icon: v.icon,
        difficulty: 'intermediate',
        message_type: 'CSV',
        source_connector_type: v.type,
        source_config_template: v.connector,
        target_connector_type: 'http_outbound',
        target_config_template: {},
        pipeline_config: {
            execution_groups: [
                step(5, 'CSV File Feed', 'csv_file_feed', 'connector.inbound', { connectorType: v.type, config: v.connector, timeoutMs: 30000 }),
                // The column mapping, saved for the source system each run
                // (saving the same mapping again changes nothing).
                csvApi(10, 'Save Mapping', 'save_mapping', {
                    method: 'PUT', endpoint: '/v1/source-systems/{{var.sourceSystem}}/bulk-mapping',
                    requestBody: { mapping: { columns: '{{var.columnMapping}}', constants: '{{var.constants}}' } },
                }),
                csvApi(20, 'Upload File', 'upload_file', {
                    method: 'POST', endpoint: '/v1/bulk/files?name={fileName}',
                    fieldMappings: { fileName: 'message._sourceFile' }, bodyRef: 'message.raw',
                    headers: { 'Content-Type': 'text/csv', Accept: 'application/json' }, timeoutMs: 600000,
                }),
                // The dry run: every row checked, nothing registered.
                csvApi(30, 'Start Check', 'start_check', {
                    method: 'POST', endpoint: '/v1/bulk/jobs',
                    fieldMappings: { fileId: 'steps.upload_file.step_output.id' },
                    requestBody: { fileId: 'fileId', system: '{{var.sourceSystem}}', encoding: '{{var.encoding}}', kind: 'check' },
                }),
                waitJob(32, 'Wait Check', 'wait_check', 'start_check'),
                stopUnless(34, 'Check State', 'check_state', 'steps.wait_check.step_output.state', 'completed', 'Completed',
                    'The check did not finish (it failed, was cancelled, or was paused). Nothing was loaded; see Wait Check.'),
                csvApi(36, 'Check Rejects', 'check_rejects', {
                    method: 'GET', endpoint: '/v1/bulk/jobs/{jobId}/rejects',
                    fieldMappings: { jobId: 'steps.start_check.step_output.id' },
                }),
                stopUnless(38, 'Check Clean', 'check_clean', 'steps.wait_check.step_output.rejects', '0', 'No refused rows',
                    'The check refused some rows, so nothing was loaded. See Check Rejects for the line and reason of each; fix the file and send it again.'),
                // The load: only reached after a clean check.
                csvApi(40, 'Start Load', 'start_load', {
                    method: 'POST', endpoint: '/v1/bulk/jobs',
                    fieldMappings: { fileId: 'steps.upload_file.step_output.id' },
                    requestBody: { fileId: 'fileId', system: '{{var.sourceSystem}}', encoding: '{{var.encoding}}', kind: 'load' },
                }),
                waitJob(42, 'Wait Load', 'wait_load', 'start_load'),
                stopUnless(44, 'Load State', 'load_state', 'steps.wait_load.step_output.state', 'completed', 'Completed',
                    'The load did not finish (it failed, was cancelled, or was paused). What it registered stays registered; see Wait Load. Resume it in ezIdentitySense.'),
                csvApi(46, 'Load Rejects', 'load_rejects', {
                    method: 'GET', endpoint: '/v1/bulk/jobs/{jobId}/rejects',
                    fieldMappings: { jobId: 'steps.start_load.step_output.id' },
                }),
                stopUnless(48, 'Load Clean', 'load_clean', 'steps.wait_load.step_output.rejects', '0', 'No refused rows',
                    'The load finished, but some rows were refused. See Load Rejects for the line and reason of each.'),
            ],
        },
        required_connection_fields: [
            ...v.fields.map(([field, label, type, required, hint]) => ({ section: 'source', field, label, type, required, hint })),
            { section: 'variables', section_label: 'File Layout', field: 'sourceSystem', label: 'Source System Code', type: 'string', required: true,
              hint: 'The source system these patients belong to, as set up in ezIdentitySense (its code, e.g. HOSP-EHR).' },
            { section: 'variables', field: 'encoding', label: 'File Encoding', type: 'select', required: true,
              options: ['UTF-8', 'windows-1252', 'ISO-8859-1', 'ISO-8859-15', 'windows-1251', 'KOI8-R', 'Shift_JIS', 'EUC-JP', 'GB18030', 'Big5', 'EUC-KR', 'UTF-16LE', 'UTF-16BE'],
              hint: 'How the file\'s text is encoded. Unsure? UTF-8 is usual for new exports; older Windows exports are often windows-1252.' },
            { section: 'variables', field: 'columnMapping', label: 'Which Column Holds What', type: 'keyvalue', required: true,
              placeholder: 'Patient No=localId\nFirst=names.0.given.0\nLast=names.0.family.0\nDOB=birthDate',
              hint: 'One COLUMN HEADER=field per line, using the file\'s own header names. Fields: localId (required: the record\'s ID in the source), version, names.0.given.0, names.0.family.0, birthDate, sex, identifiers.0.authority, identifiers.0.value, telecom.0.value, addresses.0.lines.0, addresses.0.city, addresses.0.postalCode, and so on. The same mapping is saved for the source each time a file arrives.' },
            { section: 'variables', field: 'constants', label: 'Fixed Values', type: 'keyvalue', required: false,
              placeholder: 'version=2026-01-01T00:00:00Z',
              hint: 'Optional, one FIELD=value per line: a value every row gets (for example a version, if the file has no version column).' },
            ...ezisConnectionFields(CSV_STEPS),
        ],
        preview_steps: [
            { name: 'Pick Up CSV File', icon: v.icon, step_type: 'connector.inbound' },
            { name: 'Upload and Check Every Row', icon: '🔎', step_type: 'enrichment.api' },
            { name: 'Load When the Check Is Clean', icon: '📥', step_type: 'enrichment.api' },
            { name: 'Report Refused Rows', icon: '📋', step_type: 'switch_case' },
        ],
        estimated_setup_minutes: 15,
        processing_order: 'in_order',
        guide: 'csv-load-to-ezidentitysense.md',
    };
}

const CSV_TEMPLATES = Object.values(CSV_VARIANTS).map(csvLoad);

const FHIR_TEMPLATES = [subscriptionToADT, subscriptionToFHIR];

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

// V296 (milestone M4): templates added or replaced after V295. A template
// is replaced by slug; interfaces already created from it are unaffected.
const M4_TEMPLATES = [inbound, outbound, subscriptionToADT, fhirRelay, ...CSV_TEMPLATES];

if (process.argv[2] === 'm4') {
    process.stdout.write(`-- V296: milestone M4 of the ezIdentitySense templates.
--
-- GENERATED by scripts/templates/ezidentitysense-templates.js (argument
-- "m4") from the JS definitions and the Markdown guides beside it. Edit
-- those and regenerate; don't edit this file by hand.
--
-- Replaces "HL7 ADT to ezIdentitySense" (A47, the older merge events A30,
-- A34 and A18, and an editable event table) and the two templates that send
-- ADT (an editable change-to-event table). Interfaces already created from
-- them keep what they have.

-- HL7 v2 to FHIR mappings for the merge-family events that had none: A30
-- (merge person information), A18 (merge patient information) and A47
-- (change patient identifier list) all carry PID and MRG, like A40, so they
-- start from A40's mapping (PID to Patient). Without one, the Patient is
-- not built and the message cannot be filed.
INSERT INTO hl7_fhir_templates
    (message_type, hl7_version, template_name, template_description, template_config,
     is_default, fhir_resources, template_type, is_system, is_public, version,
     referencing_mode, profile_version, fhir_version)
SELECT m.message_type, s.hl7_version, 'OOB_' || replace(m.message_type, '^', '_') || '_v' || s.hl7_version || '_' || s.fhir_version,
       m.description, s.template_config, true, s.fhir_resources, s.template_type, true, true, s.version,
       s.referencing_mode, s.profile_version, s.fhir_version
FROM hl7_fhir_templates s
CROSS JOIN (VALUES
    ('ADT^A30', 'Merge person information (PID and MRG, as A40)'),
    ('ADT^A18', 'Merge patient information (PID and MRG, as A40)'),
    ('ADT^A47', 'Change patient identifier list (PID and MRG, as A40)')
) AS m(message_type, description)
WHERE s.template_name = 'OOB_ADT_A40_v2.5.1_R4'
  AND NOT EXISTS (SELECT 1 FROM hl7_fhir_templates e WHERE e.message_type = m.message_type AND e.is_default);
${M4_TEMPLATES.map(templateSQL).join('')}`);
    process.exit(0);
}

if (process.argv[2] === 'fhir') {
    process.stdout.write(`-- V294: ready-to-use templates that subscribe to ezIdentitySense's FHIR
-- topics (HL7 Subscriptions R5 Backport, connector fhir_subscription_inbound,
-- V293) and send each change on as HL7 v2 ADT, or to another FHIR server.
--
-- GENERATED by scripts/templates/ezidentitysense-templates.js (argument
-- "fhir") from the JS definitions and the Markdown guides beside it. Edit
-- those and regenerate; don't edit this file by hand.
--
-- Every credential ships blank and is asked for by the "Use template" form.
${FHIR_TEMPLATES.map(templateSQL).join('')}`);
    process.exit(0);
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
