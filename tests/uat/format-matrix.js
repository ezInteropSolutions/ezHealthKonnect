// FORMAT x CONNECTIVITY end-to-end matrix (docs/UAT_TEST_CASES.md, workflow area 9; docs/QA_STRATEGY.md L4).
// Runs against a live app (BASE_URL, default http://localhost:3000, admin session in .auth/user.json):
//   for every scenario: create the interface the way the wizard/templates do -> activate -> send a REAL sample through the
//   real listener -> wait for a terminal status -> check the output ARRIVED at a local receiver (HTTP / MLLP / WebSocket / file)
//   -> probe every message-viewer endpoint for that message.
//   Usage:  node tests/uat/format-matrix.js                (all)    ONLY=F03,C01 node ...   (some)    KEEP=1 (keep interfaces)
//           DEBUG_RECV=1 prints every request the receiver got.   Needs Docker Desktop (host.docker.internal) and the dev stack.// Format x connectivity end-to-end matrix. Each scenario: create -> activate -> send real sample -> terminal status
// -> output arrived? -> message viewer endpoints reachable? Results to matrix-results.json.
const path = require('path');
const ROOT = path.resolve(__dirname, '..', '..').replace(/\\/g, '/');
const http = require('http'), net = require('net'), fs = require('fs'), os = require('os'), cp = require('child_process'), crypto = require('crypto');
const { request } = require(ROOT + '/node_modules/@playwright/test');
const { sendMLLP, parseAckCode } = require(ROOT + '/tests/playwright/helpers/mllp.js');
const F = require(ROOT + '/tests/playwright/helpers/fixtures.js');
const BASE = 'http://localhost:3000';
const HOST_FROM_APP = 'host.docker.internal';
const CONTAINER = 'ezhealthkonnect-app';
const ONLY = (process.env.ONLY || '').split(',').filter(Boolean);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const read = (p) => fs.readFileSync(ROOT + '/' + p, 'utf8');
let nextPort = 6621;
const port = () => nextPort++;

// â”€â”€ local receivers (what the app delivers TO) â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
const received = []; // {kind, key, body, headers}
let httpRecv, mllpRecv, wsRecv;
const mllpFrames = [];
function startReceivers() {
    return Promise.all([
        new Promise((res) => { httpRecv = http.createServer((req, rsp) => { let b = ''; req.on('data', (c) => (b += c)); req.on('end', () => { received.push({ kind: 'http', key: req.url, body: b, headers: req.headers, method: req.method }); const bad = req.url.startsWith('/fail'); rsp.writeHead(bad ? 500 : 200, { 'Content-Type': 'application/json' }); rsp.end(bad ? '{"error":"simulated downstream failure"}' : '{"ok":true,"resourceType":"OperationOutcome"}'); }); }); httpRecv.listen(0, '0.0.0.0', res); }),
        new Promise((res) => { mllpRecv = net.createServer((s) => { let buf = Buffer.alloc(0); s.on('data', (d) => { buf = Buffer.concat([buf, d]); const e = buf.indexOf(Buffer.from([0x1c, 0x0d])); if (e > 0) { const msg = buf.slice(1, e).toString(); mllpFrames.push(msg); received.push({ kind: 'mllp', key: 'mllp', body: msg }); const ctl = (msg.match(/MSH\|[^|]*\|[^|]*\|[^|]*\|[^|]*\|[^|]*\|[^|]*\|[^|]*\|[^|]*\|([^|\r]*)/) || [])[1] || '1'; s.write(Buffer.concat([Buffer.from([0x0b]), Buffer.from(`MSH|^~\\&|QA|QA|EHK|EHK|20260101||ACK|A1|P|2.5\rMSA|AA|${ctl}\r`), Buffer.from([0x1c, 0x0d])])); buf = Buffer.alloc(0); } }); s.on('error', () => {}); }); mllpRecv.listen(0, '0.0.0.0', res); }),
        new Promise((res) => { wsRecv = net.createServer((s) => wsServerConn(s)); wsRecv.listen(0, '0.0.0.0', res); }),
    ]);
}
// minimal RFC6455 server: handshake, read masked text frames, echo "ECHO: <text>"
function wsServerConn(s) {
    let shook = false, buf = Buffer.alloc(0);
    s.on('error', () => {});
    s.on('data', (d) => {
        buf = Buffer.concat([buf, d]);
        if (!shook) { const t = buf.toString('latin1'); const m = t.match(/Sec-WebSocket-Key: (.+)\r\n/i); if (!m || !t.includes('\r\n\r\n')) return; const acc = crypto.createHash('sha1').update(m[1].trim() + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64'); s.write(`HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${acc}\r\n\r\n`); shook = true; buf = buf.slice(t.indexOf('\r\n\r\n') + 4); }
        while (buf.length >= 2) {
            let len = buf[1] & 0x7f, off = 2; if (len === 126) { if (buf.length < 4) return; len = buf.readUInt16BE(2); off = 4; } else if (len === 127) { if (buf.length < 10) return; len = Number(buf.readBigUInt64BE(2)); off = 10; }
            const masked = buf[1] & 0x80; const total = off + (masked ? 4 : 0) + len; if (buf.length < total) return;
            const mask = masked ? buf.slice(off, off + 4) : null; const payload = Buffer.from(buf.slice(off + (masked ? 4 : 0), total)); if (mask) for (let i = 0; i < payload.length; i++) payload[i] ^= mask[i % 4];
            const op = buf[0] & 0x0f; buf = buf.slice(total);
            if (op === 1) { const text = payload.toString(); received.push({ kind: 'ws', key: 'ws', body: text }); const out = Buffer.from('ECHO: ' + text.slice(0, 40)); s.write(Buffer.concat([Buffer.from([0x81, out.length]), out])); }
            if (op === 8) s.end();
        }
    });
}
function wsSend(portN, p, text) { // minimal client
    return new Promise((resolve, reject) => {
        const s = net.createConnection({ host: '127.0.0.1', port: portN }); const key = crypto.randomBytes(16).toString('base64'); let shook = false, buf = Buffer.alloc(0);
        s.on('connect', () => s.write(`GET ${p} HTTP/1.1\r\nHost: localhost:${portN}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: ${key}\r\nSec-WebSocket-Version: 13\r\n\r\n`));
        s.on('data', (d) => { buf = Buffer.concat([buf, d]); if (!shook && buf.includes('\r\n\r\n')) { if (!/101/.test(buf.toString('latin1', 0, 20))) return reject(new Error('ws handshake refused: ' + buf.toString('latin1', 0, 60))); shook = true; const payload = Buffer.from(text); const mask = crypto.randomBytes(4); const body = Buffer.from(payload); for (let i = 0; i < body.length; i++) body[i] ^= mask[i % 4]; const hdr = payload.length < 126 ? Buffer.from([0x81, 0x80 | payload.length]) : Buffer.from([0x81, 0x80 | 126, payload.length >> 8, payload.length & 255]); s.write(Buffer.concat([hdr, mask, body])); setTimeout(() => { s.destroy(); resolve(); }, 1500); } });
        s.on('error', reject); setTimeout(() => reject(new Error('ws timeout')), 8000);
    });
}
const ENQ = 5, ACK = 6, STX = 2, ETX = 3, EOT = 4, CR = 13, LF = 10;
function astmSend(portN, records) {
    return new Promise((resolve, reject) => {
        const sock = net.createConnection({ host: '127.0.0.1', port: portN }, () => sock.write(Buffer.from([ENQ])));
        let stage = 'enq', n = 1, i = 0; const to = setTimeout(() => { sock.destroy(); reject(new Error('astm timeout')); }, 10000);
        const next = () => { if (i >= records.length) { sock.write(Buffer.from([EOT])); clearTimeout(to); sock.end(); return resolve(); } const body = Buffer.from(`${n}${records[i]}\r`, 'ascii'); const bp = Buffer.concat([body, Buffer.from([ETX])]); let sum = 0; for (const b of bp) sum += b; const cs = (sum % 256).toString(16).toUpperCase().padStart(2, '0'); sock.write(Buffer.concat([Buffer.from([STX]), bp, Buffer.from(cs, 'ascii'), Buffer.from([CR, LF])])); stage = 'frame'; };
        sock.on('data', (c) => { for (const b of c) { if (stage === 'enq' && b === ACK) next(); else if (stage === 'frame' && b === ACK) { i++; n = n === 7 ? 0 : n + 1; next(); } } });
        sock.on('error', (e) => { clearTimeout(to); reject(e); });
    });
}
const recvUrl = (p, kind = 'http') => (kind === 'http' ? `http://${HOST_FROM_APP}:${httpRecv.address().port}${p}` : kind === 'ws' ? `ws://${HOST_FROM_APP}:${wsRecv.address().port}${p}` : null);

// â”€â”€ app API â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
let api;
const J = async (r) => { const t = await r.text(); try { return JSON.parse(t); } catch { return { _raw: t.slice(0, 300), _status: r.status() }; } };
const listMsgs = async (id) => { const b = await J(await api.get(`/api/messages/interface/${id}?limit=100`)); const x = b.data || b; return Array.isArray(x) ? x : x.messages || x.data || []; };

async function createWizard(spec) {
    const p = {
        name: spec.name, description: 'QA matrix', sourceType: spec.srcFormat || 'hl7v2', sourceConnectivity: spec.src.type, sourceConfig: spec.src.config,
        sourceConnectorConfig: { connectorType: spec.src.type, config: spec.src.config }, targetType: spec.tgtFormat || 'hl7v2', targetConnectivity: spec.tgt.type, targetConfig: spec.tgt.config,
        targetConnectorConfig: { connectorType: spec.tgt.type, config: spec.tgt.config }, messageType: spec.msgType || 'ADT^A01', mappings: [], transformationFlow: spec.flow,
        auto_start: true, deployment_mode: 'manual', status: 'active', debug_logging: true, log_retention_days: 7,
    };
    const b = await J(await api.post('/api/wizard/complete', { data: { wizardData: p } }));
    const id = b.interfaceId || b.data?.interfaceId || b.id;
    if (!id) throw new Error('create failed: ' + JSON.stringify(b).slice(0, 200));
    return id;
}
async function createFromTemplate(spec) {
    const use = await J(await api.post(`/api/interface-templates/${spec.template}/use`, { data: { interface_name: spec.name, source_values: {}, target_values: {} } }));
    if (!use.success) throw new Error('use template: ' + JSON.stringify(use).slice(0, 200));
    const s = use.interface_scaffold; const cfg = JSON.parse(JSON.stringify(s.pipeline_config));
    for (const g of cfg.execution_groups || []) for (const st of g.steps || []) {
        delete st.id;
        if (st.step_type === 'connector.inbound' && spec.src) st.config = { ...st.config, connectorType: spec.src.type, config: spec.src.config };
        if (st.step_type === 'connector.outbound' && spec.tgt) st.config = { ...st.config, connectorType: spec.tgt.type, config: spec.tgt.config, ...(spec.tgt.contentField ? { contentField: spec.tgt.contentField } : {}), ...(spec.tgt.contentType ? { contentType: spec.tgt.contentType } : {}) };
    }
    const mt = spec.msgType || s.message_type;
    const i = await J(await api.post('/api/interfaces', { data: { name: spec.name, messageType: mt || 'HL7v2', description: 'QA matrix: ' + spec.template, sourceType: '', targetType: '', sourceConfig: {}, targetConfig: {} } }));
    const id = i.interface?.id || i.id; if (!id) throw new Error('create iface: ' + JSON.stringify(i).slice(0, 200));
    const p = await J(await api.post('/api/pipelines', { data: { interface_id: id, message_type: mt, name: spec.name + ' Pipeline', execution_groups: cfg.execution_groups, pipeline_config: cfg } }));
    if (!p.success) throw new Error('save pipeline: ' + JSON.stringify(p).slice(0, 200));
    return { id, steps: cfg.execution_groups.flatMap((g) => g.steps.map((x) => x.step_type)) };
}
const containerExec = (cmd) => { try { return cp.execSync(`docker exec ${CONTAINER} sh -c "${cmd.replace(/"/g, '\\"')}"`, { encoding: 'utf8', timeout: 20000, stdio: ['ignore', 'pipe', 'ignore'] }); } catch (e) { return ''; } };
function containerPut(dest, content) { const t = path.join(os.tmpdir(), 'qa-' + Date.now() + '-' + path.basename(dest)); fs.writeFileSync(t, content); cp.execSync(`docker cp "${t}" ${CONTAINER}:${dest}`, { stdio: 'ignore' }); fs.unlinkSync(t); }

const TERMINAL = /processed|completed|delivered|failed|error|rejected|dead/i;
async function waitMessage(id, known, ms = 45000) {
    const end = Date.now() + ms; let m;
    while (Date.now() < end) { m = (await listMsgs(id)).find((x) => !known.has(x.message_id || x.id)); if (m && TERMINAL.test(m.status || '')) return m; await sleep(1000); }
    return m || null;
}
// raw/parsed/transformed/logs are keyed by message_id (tcp_â€¦); lineage/coverage-audit by the row uuid; trace by correlation_id;
// errors needs ?interfaceId= â€” exactly what the message viewer UI passes.
// Every viewer part must answer 200 for a processed message; mapping-log is a CDA->FHIR-only artifact (404 elsewhere).
const expectedViewerStatus = (name, isCda) => (name === 'mapping-log' && !isCda ? 404 : 200);
const VIEWER = (mid, uid, corr, iid) => [
    ['detail', `/api/messages/${mid}`], ['raw', `/api/messages/${mid}/raw`], ['parsed-content', `/api/messages/${mid}/parsed-content`], ['transformed', `/api/messages/${mid}/transformed`],
    ['logs', `/api/messages/${mid}/logs`], ['errors', `/api/messages/${mid}/errors?interfaceId=${iid}`], ['mapping-log', `/api/messages/${mid}/mapping-log`], ['lineage', `/api/messages/${encodeURIComponent(mid)}/lineage`],
    ['coverage-audit', `/api/messages/${encodeURIComponent(mid)}/coverage-audit`], ['trace', `/api/system/message-trace/${encodeURIComponent(mid)}?interfaceId=${iid}`],
];
async function viewerChecks(mid, iid) {
    const out = {};
    const d = await J(await api.get(`/api/messages/${mid}`)); const dm = (d.data && d.data.message) || {};
    const uid = dm.id || mid, corr = dm.correlation_id || dm.correlationId;
    out._ids = { s: 0, n: 0, sample: `uuid=${uid} correlation_id=${corr}` };
    for (const [name, url] of VIEWER(mid, uid, corr, iid)) {
        const r = await api.get(url).catch((e) => ({ status: () => 'ERR', text: async () => e.message }));
        const t = await r.text(); let n = 0; try { const j = JSON.parse(t); const d = j.data !== undefined ? j.data : j; n = d == null ? 0 : typeof d === 'string' ? d.length : Array.isArray(d) ? d.length : Object.keys(d).length; } catch { n = t.length; }
        out[name] = { s: r.status(), n, sample: t.slice(0, 90).replace(/\s+/g, ' ') };
    }
    return out;
}

// â”€â”€ samples â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
const S = {
    adt: F.HL7_ADT_A01, oru: F.HL7_ORU_R01,
    fhirPatient: JSON.stringify({ resourceType: 'Patient', id: 'qa-p1', name: [{family: 'Matrix', given: ['Qa']}], gender: 'female', birthDate: '1985-02-03' }),
    medReq: JSON.stringify({ resourceType: 'MedicationRequest', id: 'qa-mr1', status: 'active', intent: 'order', medicationCodeableConcept: { text: 'Lisinopril 10 mg tablet', coding: [{ system: 'http://www.nlm.nih.gov/research/umls/rxnorm', code: '314076' }] }, subject: { reference: 'Patient/1', display: 'Jane Doe' }, requester: { display: 'Dr Who', identifier: { value: '1234567893' } }, authoredOn: '2026-10-01', dosageInstruction: [{ text: 'Take 1 tablet daily' }], dispenseRequest: { quantity: { value: 30 }, numberOfRepeatsAllowed: 2 } }),
    edi835: () => read('edi/testdata/real_samples/blue_cross_nc_sample.txt'),
    edi837p: () => read('edi/testdata/real_samples/x12org_837p_ben_kildare_sample.txt'),
    edi270: () => read('edi/testdata/real_samples/self_authored_270_sample.txt'),
    ccd: () => read('tests/fixtures/sample_ccda_2_1.xml'),
    newrx: () => read('ncpdp/testdata/real_samples/dgoradia_sample_newrx.xml'),
    b1: () => read('ncpdptelecom/testdata/generated_samples/request_baseline_female.txt'),
    astm: ['H|\\^&|||QA-Analyzer|||||LIS||P|1|20260101120000', 'P|1||QAPID1||Doe^Jane||19800101|F', 'O|1|QASPEC1||^^^GLU|R|20260101120000', 'R|1|^^^GLU|95|mg/dL|70-110|N||F', 'L|1|N'],
};
const httpPost = async (p, body, ct = 'text/plain', headers = {}) => { const r = await fetch(`http://localhost:${p}${body.path}`, { method: 'POST', headers: { 'content-type': ct, ...headers }, body: body.data }); return `${r.status}`; };

// â”€â”€ scenarios â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
function scenarios() {
    const sc = [];
    const rest = (p, ep) => ({ type: 'http_rest_inbound', config: { port: p, endpoint_path: ep, http_methods: ['POST'], authentication_type: 'none', content_type: 'text/plain' } });
    const mllp = (p) => ({ type: 'tcp_mllp_inbound', config: { host: '0.0.0.0', port: p } });
    const toHttp = (pathKey, contentField, ct) => ({ type: 'http_outbound', config: { url: recvUrl(pathKey), method: 'POST', content_type: ct || 'application/json', authentication_type: 'none', retry_attempts: 1, timeout_seconds: 15 }, contentField, contentType: ct });
    // ---- FORMAT matrix (templates: each format has its own step chain) ----
    { const p = port(); sc.push({ id: 'F01', label: 'HL7 v2 ORU â†’ FHIR (validation, hl7_fhir_transform, script, hl7.build)', template: 'device-hl7-tcp', src: mllp(p), tgt: toHttp('/f01', 'fhirBundle', 'application/fhir+json'), msgType: 'ORU^R01', send: async () => parseAckCode(await sendMLLP('localhost', p, S.oru, 15000)), expectOut: { kind: 'http', key: '/f01', has: /Patient|Observation|Bundle/ } }); }
    { const p = port(); sc.push({ id: 'F02', label: 'FHIR Patient passthrough (http_fhir_inbound â†’ http_fhir_outbound)', flow: 'fhir_passthrough', srcFormat: 'fhir', tgtFormat: 'fhir', src: { type: 'http_fhir_inbound', config: { port: p, basePath: '/fhir/r4', fhirVersion: 'R4', authType: 'none' } }, tgt: { type: 'http_fhir_outbound', config: { baseUrl: recvUrl('/fhirbase'), timeout: 15, authType: 'none' } }, msgType: 'FHIR:Patient', send: async () => httpPost(p, { path: '/fhir/r4/Patient', data: S.fhirPatient }, 'application/fhir+json'), expectOut: { kind: 'http', key: /fhirbase/, has: /Patient/, exact: S.fhirPatient } }); }
    { const p = port(); sc.push({ id: 'F03', label: 'EDI 835 remittance â†’ FHIR (edi.parse, edi.validate, script, fhir.build Ã—2, payload.builder, fhir_validation)', template: 'edi-835-to-fhir-sftp', src: rest(p, '/edi'), tgt: toHttp('/f03', 'payload', 'application/fhir+json'), send: async () => httpPost(p, { path: '/edi', data: S.edi835() }), expectOut: { kind: 'http', key: '/f03', has: /ExplanationOfBenefit|PaymentReconciliation/ } }); }
    { const p = port(); sc.push({ id: 'F04', label: 'EDI 837P claim â†’ FHIR', template: 'edi-837p-to-fhir-sftp', src: rest(p, '/edi'), tgt: toHttp('/f04', 'payload', 'application/fhir+json'), send: async () => httpPost(p, { path: '/edi', data: S.edi837p() }), expectOut: { kind: 'http', key: '/f04', has: /Claim/ } }); }
    { const p = port(); sc.push({ id: 'F05', label: 'EDI 270 eligibility â†’ FHIR', template: 'edi-270-to-fhir-sftp', src: rest(p, '/edi'), tgt: toHttp('/f05', 'payload', 'application/fhir+json'), send: async () => httpPost(p, { path: '/edi', data: S.edi270() }), expectOut: { kind: 'http', key: '/f05', has: /CoverageEligibilityRequest|Patient|Bundle/ } }); }
    { const p = port(); sc.push({ id: 'F06', label: 'CDA / C-CDA CCD â†’ FHIR (cda.normalize, cda.parse, cda.to_fhir)', template: 'cda-ccd-inbound-fhir-r4', src: rest(p, '/cda'), tgt: { type: 'http_fhir_outbound', config: { baseUrl: recvUrl('/f06'), timeout: 15, authType: 'none' }, contentField: 'fhirBundle' }, send: async () => httpPost(p, { path: '/cda', data: S.ccd() }, 'text/xml'), expectOut: { kind: 'http', key: /f06/, has: /Bundle|Patient|resourceType/ } }); }
    { const p = port(); sc.push({ id: 'F07', label: 'NCPDP SCRIPT NewRx â†’ FHIR (ncpdp.parse, ncpdp.validate, fhir.build Ã—4)', template: 'ncpdp-newrx-to-fhir-sftp', src: rest(p, '/rx'), tgt: toHttp('/f07', 'payload', 'application/fhir+json'), send: async () => httpPost(p, { path: '/rx', data: S.newrx() }, 'application/xml'), expectOut: { kind: 'http', key: '/f07', has: /MedicationRequest/ } }); }
    { const p = port(); sc.push({ id: 'F08', label: 'NCPDP Telecom D.0 B1 claim â†’ FHIR (ncpdptelecom.parse, validate, script, fhir.build Ã—3)', template: 'ncpdp-telecom-b1-request-to-fhir-sftp', src: rest(p, '/b1'), tgt: toHttp('/f08', 'payload', 'application/fhir+json'), send: async () => httpPost(p, { path: '/b1', data: S.b1() }, 'text/plain'), expectOut: { kind: 'http', key: '/f08', has: /Claim|Patient/ } }); }
    { const p = port(); sc.push({ id: 'F09', label: 'ASTM lab results over TCP (astm.parse, astm.validate) â†’ HTTP', template: 'device-astm-tcp', src: { type: 'astm_tcp_inbound', config: { port: p, max_connections: 10, read_timeout_sec: 60, checksum_severity: 'error' } }, tgt: toHttp('/f09', 'parsedASTM', 'application/json'), send: async () => { await astmSend(p, S.astm); return 'sent'; }, expectOut: { kind: 'http', key: '/f09', has: /QAPID1|GLU|Doe/ } }); }
    { const p = port(); sc.push({ id: 'F10', label: 'FHIR MedicationRequest â†’ NCPDP NewRx XML outbound (script, ncpdp.map_to_canonical, validate, build)', template: 'fhir-medicationrequest-to-newrx-outbound', src: { type: 'http_fhir_inbound', config: { port: p, basePath: '/fhir/r4', fhirVersion: 'R4', authType: 'none' } }, tgt: toHttp('/f10', 'ncpdpScript', 'application/xml'), send: async () => httpPost(p, { path: '/fhir/r4/MedicationRequest', data: S.medReq }, 'application/fhir+json'), expectOut: { kind: 'http', key: '/f10', has: /NewRx|Message/ } }); }
    // ---- CONNECTIVITY matrix (wizard; HL7 passthrough through each connector pair) ----
    { const p = port(); sc.push({ id: 'C01', label: 'TCP/MLLP â†’ HTTP outbound', flow: 'hl7_passthrough', src: mllp(p), tgt: { type: 'http_outbound', config: { url: recvUrl('/c01'), method: 'POST', content_type: 'text/plain', authentication_type: 'none', retry_attempts: 1 } }, send: async () => parseAckCode(await sendMLLP('localhost', p, S.adt, 15000)), expectOut: { kind: 'http', key: '/c01', has: /MSH\|/, exact: S.adt } }); }
    { const p = port(); sc.push({ id: 'C02', label: 'TCP/MLLP â†’ TCP/MLLP outbound', flow: 'hl7_passthrough', src: mllp(p), tgt: { type: 'tcp_mllp_outbound', config: { host: HOST_FROM_APP, port: mllpRecv.address().port, expect_ack: true, keep_alive: false, max_retries: 1 } }, send: async () => parseAckCode(await sendMLLP('localhost', p, S.adt, 15000)), expectOut: { kind: 'mllp', has: /MSH\|/, exact: S.adt } }); }
    { const p = port(); sc.push({ id: 'C03', label: 'TCP/MLLP â†’ file writer', flow: 'hl7_passthrough', src: mllp(p), tgt: { type: 'file_writer', config: { directory_path: '/tmp/qa-out/c03', filename_pattern: '{message_id}.hl7', create_subdirs: true } }, send: async () => parseAckCode(await sendMLLP('localhost', p, S.adt, 15000)), expectOut: { kind: 'file', dir: '/tmp/qa-out/c03', has: /MSH\|/, exact: S.adt } }); }
    { const p = port(); sc.push({ id: 'C04', label: 'HTTP REST inbound â†’ sink (store only)', flow: 'passthrough', src: rest(p, '/in'), tgt: { type: 'sink_outbound', config: {} }, send: async () => httpPost(p, { path: '/in', data: S.adt }), expectOut: null }); }
    { const p = port(); sc.push({ id: 'C05', label: 'WebSocket inbound â†’ WebSocket outbound', flow: 'hl7_passthrough', src: { type: 'websocket_inbound', config: { port: p, base_path: '/ws', authentication_type: 'none' } }, tgt: { type: 'websocket_outbound', config: { url: recvUrl('/out', 'ws'), connection_mode: 'persistent', wait_for_response: true, response_timeout_seconds: 5 } }, send: async () => { await wsSend(p, '/ws', S.adt.replace(/\r/g, '\n')); return 'sent'; }, expectOut: { kind: 'ws', has: /MSH\|/ } }); }
    { const p = port(); sc.push({ id: 'C06', label: 'File listener â†’ file writer', flow: 'hl7_passthrough', src: { type: 'file_listener', config: { directory_path: '/tmp/qa-in/c06', file_pattern: '*.hl7', polling_interval: 2, after_processing: 'delete', create_dirs: true } }, tgt: { type: 'file_writer', config: { directory_path: '/tmp/qa-out/c06', filename_pattern: '{message_id}.hl7', create_subdirs: true } }, prep: () => containerExec('mkdir -p /tmp/qa-in/c06 /tmp/qa-out/c06'), send: async () => { containerPut('/tmp/qa-in/c06/qa.hl7', S.adt); return 'dropped'; }, expectOut: { kind: 'file', dir: '/tmp/qa-out/c06', has: /MSH\|/, exact: S.adt } }); }
    { const p = port(); sc.push({ id: 'C07', label: 'EDI 835 any-format passthrough (REST inbound → HTTP outbound, flow others)', flow: 'others', src: rest(p, '/in'), tgt: { type: 'http_outbound', config: { url: recvUrl('/c07'), method: 'POST', content_type: 'text/plain', authentication_type: 'none', retry_attempts: 1 } }, send: async () => httpPost(p, { path: '/in', data: S.edi835() }), expectOut: { kind: 'http', key: '/c07', has: /ISA\*/, exact: S.edi835() } }); }
    // ---- FAILURE scenarios: the message must end up failed AND the user must be able to see why ----
    { const p = port(); sc.push({ id: 'X01', failure: true, label: 'HL7 → HTTP destination refuses the connection (delivery failure)', flow: 'hl7_passthrough', src: mllp(p), tgt: { type: 'http_outbound', config: { url: `http://${HOST_FROM_APP}:9/none`, method: 'POST', content_type: 'text/plain', authentication_type: 'none', retry_attempts: 1, retry_delay_seconds: 1, timeout_seconds: 5 } }, send: async () => parseAckCode(await sendMLLP('localhost', p, S.adt, 15000)), expectOut: null }); }
    { const p = port(); sc.push({ id: 'X02', failure: true, label: 'HL7 → HTTP destination answers 500 (delivery failure)', flow: 'hl7_passthrough', src: mllp(p), tgt: { type: 'http_outbound', config: { url: recvUrl('/fail'), method: 'POST', content_type: 'text/plain', authentication_type: 'none', retry_attempts: 1, retry_delay_seconds: 1, timeout_seconds: 10 } }, send: async () => parseAckCode(await sendMLLP('localhost', p, S.adt, 15000)), expectOut: null }); }
    // F06: the CDA template has no message_type; the dashboard's own "Use Template" falls back to ADT^A01 (dashboard.js:936).
    // Run it both ways â€” as the UI would create it, and with the type existing CDA pipelines use (CCD).
    const f6 = sc.find((s) => s.id === 'F06'); f6.msgType = 'ADT^A01'; f6.label += ' [as the UI creates it: message_type ADT^A01]';
    sc.push({ ...f6, id: 'F06b', msgType: 'CCD', label: f6.label.replace(/\[.*\]/, '[message_type CCD]') });
    return sc;
}

(async () => {
    api = await request.newContext({ baseURL: BASE, storageState: ROOT + '/.auth/user.json' });
    await startReceivers();
    const results = []; const all = scenarios();
    for (const sp of all) {
        if (ONLY.length && !ONLY.includes(sp.id)) continue;
        sp.name = `QA-${sp.id}-${Date.now()}`; const r = { id: sp.id, label: sp.label, steps: '', create: '', activate: '', send: '', status: '', output: '', viewer: {}, notes: [] };
        let id;
        try {
            if (sp.prep) sp.prep();
            if (sp.expectOut && sp.expectOut.kind === 'file') containerExec(`rm -rf ${sp.expectOut.dir} && mkdir -p ${sp.expectOut.dir}`);
            if (sp.template) { const c = await createFromTemplate(sp); id = c.id; r.steps = c.steps.join(' > '); } else id = await createWizard(sp);
            r.create = 'ok'; r.id_ = id;
            const act = await J(await api.post(`/api/runtime/interfaces/${id}/activate`));
            r.activate = act.warnings && act.warnings.length ? 'WARN: ' + act.warnings.join('; ').slice(0, 160) : act.success === false ? 'FAIL: ' + JSON.stringify(act).slice(0, 160) : 'ok';
            await sleep(1500);
            const known = new Set((await listMsgs(id)).map((m) => m.message_id || m.id));
            const before = received.length;
            r.send = String(await sp.send());
            const m = await waitMessage(id, known);
            r.status = m ? m.status : 'NO MESSAGE STORED';
            if (m) {
                const mid = m.message_id || m.id; r.mid = mid; r.viewer = await viewerChecks(mid, id);
                r.viewerProblems = Object.entries(r.viewer).filter(([k, v]) => k !== '_ids' && v.s !== expectedViewerStatus(k, /^F06/.test(sp.id))).map(([k, v]) => `${k}=${v.s}`);
                const dd = await J(await api.get(`/api/messages/${mid}`)); const dm = (dd.data && dd.data.message) || {};
                r.errState = { message_type: dm.message_type, status: dm.status, error_count: dm.error_count, last_error_message: dm.last_error_message, delivery_status: dm.delivery_status };
                const ee = await J(await api.get(`/api/messages/${mid}/errors?interfaceId=${id}`));
                r.errorsApi = ee.success ? { entries: (ee.data.errors || []).length, summary: ee.data.summary } : { error: ee.error, debug: ee.debug };
                if (/failed|error/i.test(m.status)) { const e = await J(await api.get(`/api/messages/${mid}/errors?interfaceId=${id}`)); r.notes.push('errors: ' + JSON.stringify(e).slice(0, 360)); }
            }
            // output verification
            const ex = sp.expectOut;
            if (!ex) r.output = 'n/a (store only)';
            else {
                let hit; const end = Date.now() + 20000;
                while (Date.now() < end && !hit) {
                    if (ex.kind === 'file') { const txt = containerExec(`cat ${ex.dir}/* 2>/dev/null`); if (txt && ex.has.test(txt)) hit = { body: txt }; }
                    else hit = received.slice(before).find((x) => x.kind === ex.kind && (!ex.key || (ex.key instanceof RegExp ? ex.key.test(x.key) : x.key === ex.key)));
                    if (!hit) await sleep(1000);
                }
                r.output = !hit ? 'NOT DELIVERED'
                    : ex.exact !== undefined ? (hit.body.trim() === ex.exact.trim() ? `delivered (${hit.body.length}B, EXACT original)` : `delivered but NOT the original (${hit.body.length}B vs ${ex.exact.length}B): ${hit.body.slice(0, 80).replace(/s+/g, ' ')}`)
                    : (ex.has.test(hit.body) ? `delivered (${hit.body.length}B, content ok)` : `delivered but content mismatch: ${hit.body.slice(0, 100)}`);
                if (hit) r.outHead = hit.body.slice(0, 220).replace(/\s+/g, ' ');
                if (process.env.DEBUG_RECV) received.slice(before).forEach((x) => console.log(`   recv ${x.kind} ${x.method || ''} ${x.key} ct=${(x.headers && x.headers['content-type']) || ''} ${x.body.length}B :: ${x.body.slice(0, 90).replace(/\s+/g, ' ')}`));
            }
        } catch (e) { r.notes.push('EXC ' + e.message.slice(0, 200)); }
        finally {
            if (id && !process.env.KEEP) { await api.post(`/api/runtime/interfaces/${id}/deactivate`).catch(() => {}); await sleep(500); await api.delete(`/api/interfaces/${id}`).catch(() => {}); }
        }
        results.push(r);
        const vOk = Object.entries(r.viewer).filter(([k, v]) => k !== '_ids' && v.s === 200).length, vN = Object.keys(r.viewer).filter((k) => k !== '_ids').length;
        if (r.errState) console.log(`   [error state] ${JSON.stringify(r.errState)}  errors-api=${JSON.stringify(r.errorsApi).slice(0, 260)}`);
        console.log(`${r.id.padEnd(4)} viewer-problems=${(r.viewerProblems || ['n/a']).join(',') || 'none'}`);
        console.log(`${r.id.padEnd(4)} create=${r.create || 'FAIL'} act=${r.activate.slice(0, 40)} send=${r.send} status=${r.status} out=${r.output} viewer=${vOk}/${vN}${r.notes.length ? '\n      ' + r.notes.join('\n      ') : ''}`);
    }
    const outFile = process.env.MATRIX_OUT || path.join(os.tmpdir(), 'format-matrix-results.json');
    fs.writeFileSync(outFile, JSON.stringify(results, null, 2)); console.log('results ->', outFile);
    httpRecv.close(); mllpRecv.close(); wsRecv.close(); await api.dispose(); process.exit(0);
})().catch((e) => { console.error('FATAL', e); process.exit(1); });


