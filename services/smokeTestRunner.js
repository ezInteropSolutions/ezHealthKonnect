'use strict';
/**
 * SmokeTestRunner — headless in-process smoke test runner.
 *
 * No Playwright, no browser. Pure Node.js: axios for HTTP, net for TCP/MLLP.
 * Logs in as admin, creates 4 seed interfaces, exercises every connectivity
 * path, then deletes all seed data. Safe to run against a live system.
 *
 * Usage:
 *   const { SmokeTestRunner } = require('./services/smokeTestRunner');
 *   const runner = new SmokeTestRunner('http://localhost:3000', process.env.INTERNAL_PROXY_SECRET);
 *   runner.on('step', ({ id, label, status, message }) => ...);
 *   runner.on('done', ({ passed, failed, total, durationMs }) => ...);
 *   await runner.run();
 */

const net  = require('net');
const http = require('http');
const { EventEmitter } = require('events');

// ── MLLP framing constants ────────────────────────────────────────────────────
const MLLP_START = 0x0b;
const MLLP_END1  = 0x1c;
const MLLP_END2  = 0x0d;

// ── Ports used by seed interfaces ─────────────────────────────────────────────
const PORT_ADT  = 6620;
const PORT_ORU  = 6621;
const PORT_FILE = 6622;
const PORT_FHIR = 8091;

// ── HL7 test messages ─────────────────────────────────────────────────────────
const HL7_ADT_A01 = [
    'MSH|^~\\&|SYSSMOKE|SMOKE_FAC|EHK|EHK|20260607120000||ADT^A01|SYSSMOKE001|P|2.5',
    'EVN|A01|20260607120000',
    'PID|1||SYSSMOKE001^^^SMOKE^MR||TEST^SMOKEPATIENT||19800515|M',
    'PV1|1|I|WARD^301^1||||DOC001^SMOKE^DOCTOR|||MED||||ADM|A0',
].join('\r');

const HL7_ORU_R01 = [
    'MSH|^~\\&|SYSSMOKE_LAB|SMOKE_FAC|EHK|EHK|20260607140000||ORU^R01|SYSSMOKE003|P|2.5',
    'PID|1||SYSSMOKE002^^^LAB^MR||TEST^SMOKEORUPATIENT||19750320|F',
    'OBR|1|ORD001|FIL001|CBC^Complete Blood Count^LN|||20260607',
    'OBX|1|NM|718-7^Hemoglobin^LN||13.5|g/dL|12.0-16.0||||F',
].join('\r');

// ── Seed interface name prefix (used for cleanup detection) ────────────────────
const NAME_PREFIX = 'SYSTEM_SMOKE_';

class SmokeTestRunner extends EventEmitter {
    /**
     * @param {string} baseURL            App base URL, e.g. 'http://localhost:3000'
     * @param {string} internalSecret     Value of INTERNAL_PROXY_SECRET env var
     * @param {string} sessionCookie      Existing admin session cookie (forwarded from the browser request)
     */
    constructor(baseURL, internalSecret, sessionCookie) {
        super();
        this._base    = (baseURL || 'http://localhost:3000').replace(/\/$/, '');
        this._secret  = internalSecret || '';
        this._cookie  = sessionCookie  || '';   // pre-seeded from the requesting admin's browser session
        this._state   = { adtId: null, oruId: null, fhirId: null, fileId: null };
        this._results = [];
        this._aborted = false;
        this._startMs = 0;
    }

    /** Ask an in-progress run to stop gracefully (called when SSE client disconnects). */
    abort() { this._aborted = true; }

    // ── Public entry point ───────────────────────────────────────────────────

    async run() {
        this._aborted = false;
        this._results = [];
        this._state   = { adtId: null, oruId: null, fhirId: null, fileId: null };
        this._startMs = Date.now();

        try {
            // ── S0: System health ────────────────────────────────────────────
            // Poll until both services are fully ready (up to 60 s) before
            // running any test steps. This avoids false failures right after restart.
            await this._step('S0-001', 'Services ready (wait up to 60 s)', () => this._waitReady(60000));

            // ── S1: Setup — create + activate seed interfaces ────────────────
            await this._step('S1-001', 'Clean up any leftover smoke-test interfaces', () => this._cleanStale());
            await this._step('S1-002', 'Create 4 seed interfaces', () => this._createInterfaces());
            await this._step('S1-003', 'Activate all interfaces',  () => this._activateAll());
            await this._step('S1-004', 'Wait for listeners to bind (10 s)', () => this._sleep(10000));

            // ── S2: ADT MLLP ─────────────────────────────────────────────────
            await this._step('S2-001', `ADT MLLP port ${PORT_ADT} accepts connections`,  () => this._checkPort(PORT_ADT));
            await this._step('S2-002', 'ADT^A01 returns AA acknowledgment',               () => this._sendMLLP(PORT_ADT, HL7_ADT_A01));
            await this._step('S2-003', 'ADT messages recorded in database',               () => this._checkMessages(this._state.adtId, 3000));

            // ── S3: ORU MLLP ─────────────────────────────────────────────────
            await this._step('S3-001', `ORU MLLP port ${PORT_ORU} accepts connections`,   () => this._checkPort(PORT_ORU));
            await this._step('S3-002', 'ORU^R01 returns AA acknowledgment',               () => this._sendMLLP(PORT_ORU, HL7_ORU_R01));
            await this._step('S3-003', 'ORU messages recorded in database',               () => this._checkMessages(this._state.oruId, 3000));

            // ── S4: FHIR HTTP ────────────────────────────────────────────────
            await this._step('S4-001', `FHIR HTTP port ${PORT_FHIR} accepts POST /Patient`, () => this._checkFHIR());
            await this._step('S4-002', 'FHIR messages recorded in database',               () => this._checkMessages(this._state.fhirId, 3000));

            // ── S5: HL7 → File ───────────────────────────────────────────────
            await this._step('S5-001', `File-writer MLLP port ${PORT_FILE} returns AA`,   () => this._sendMLLP(PORT_FILE, HL7_ADT_A01));
            await this._step('S5-002', 'File messages recorded in database',               () => this._checkMessages(this._state.fileId, 3000));

            // ── S6: Monitoring API ───────────────────────────────────────────
            await this._step('S6-001', 'Monitoring summary API returns 200',              () => this._checkMonitoring());

        } finally {
            // ── S9: Always clean up ──────────────────────────────────────────
            await this._cleanup();

            const passed   = this._results.filter(r => r.status === 'pass').length;
            const failed   = this._results.filter(r => r.status === 'fail').length;
            const total    = this._results.length;
            const duration = Date.now() - this._startMs;
            this.emit('done', { passed, failed, total, durationMs: duration });
        }
    }

    // ── Step runner ──────────────────────────────────────────────────────────

    async _step(id, label, fn) {
        if (this._aborted) return;
        this.emit('step', { id, label, status: 'running', message: '' });
        try {
            const message = await fn() || '';
            const result  = { id, label, status: 'pass', message: String(message) };
            this._results.push(result);
            this.emit('step', result);
        } catch (err) {
            const result = { id, label, status: 'fail', message: err.message || String(err) };
            this._results.push(result);
            this.emit('step', result);
        }
    }

    // ── S0: Health ───────────────────────────────────────────────────────────

    /**
     * Poll until Go reports the database is connected (up to timeoutMs ms).
     * No login required — the admin session cookie is pre-seeded from the browser.
     */
    async _waitReady(timeoutMs) {
        const deadline = Date.now() + timeoutMs;
        let lastErr    = 'not started';

        while (Date.now() < deadline) {
            try {
                await this._checkDB();
                return 'Node.js + Go + PostgreSQL ready';
            } catch (err) {
                lastErr = err.message;
                await this._sleep(2000);
            }
        }

        throw new Error(`Services not ready after ${timeoutMs / 1000} s — last error: ${lastErr}`);
    }

    async _checkGoHealth() {
        const r = await this._api('GET', '/api/system/health');
        const status = r.data?.status || r.data?.database?.status;
        if (!status) throw new Error(`Unexpected Go health response: ${JSON.stringify(r.data).slice(0, 150)}`);
        return `status=${status}`;
    }

    async _checkDB() {
        const r  = await this._api('GET', '/api/system/health');
        const db = r.data?.database || {};
        // Two response shapes exist: { connected: true } and { status: "connected" }
        const ok = db.connected === true || db.status === 'connected' || db.status === 'operational';
        if (!ok) throw new Error(`Go health reports database not connected (${JSON.stringify(db)})`);
        return `PostgreSQL ${db.status || 'connected'}`;
    }

    // ── S1: Setup ────────────────────────────────────────────────────────────

    async _cleanStale() {
        const r    = await this._api('GET', '/api/interfaces');
        const list = r.data?.data || r.data?.interfaces || r.data || [];
        const stale = (Array.isArray(list) ? list : []).filter(i => (i.name || '').startsWith(NAME_PREFIX));
        for (const iface of stale) {
            await this._api('POST', `/api/wizard/interfaces/${iface.id}/deactivate`, { reason: 'smoke-stale-cleanup' }).catch(() => {});
            await this._sleep(300);
            await this._api('DELETE', `/api/interfaces/${iface.id}`).catch(() => {});
        }
        return stale.length ? `Removed ${stale.length} stale interface(s)` : 'No stale interfaces found';
    }

    async _createInterfaces() {
        const [adt, oru, fhir, file] = await Promise.all([
            this._createOne(this._adtPayload()),
            this._createOne(this._oruPayload()),
            this._createOne(this._fhirPayload()),
            this._createOne(this._filePayload()),
        ]);
        this._state.adtId  = adt;
        this._state.oruId  = oru;
        this._state.fhirId = fhir;
        this._state.fileId = file;
        return `ADT=${adt.slice(0,8)}… ORU=${oru.slice(0,8)}… FHIR=${fhir.slice(0,8)}… File=${file.slice(0,8)}…`;
    }

    async _createOne(payload) {
        const r  = await this._api('POST', '/api/wizard/complete', { wizardData: payload });
        const id = r.data?.interfaceId;
        if (!id) throw new Error(`Wizard returned no interfaceId for "${payload.name}": ${JSON.stringify(r.data).slice(0, 200)}`);
        return id;
    }

    async _activateAll() {
        const ids = Object.values(this._state).filter(Boolean);
        await Promise.all(ids.map(id =>
            this._api('POST', `/api/processing/interfaces/${id}/activate`, {}).catch(() => {})
        ));
        return `Activated ${ids.length} interfaces`;
    }

    // ── S2/S3/S5: TCP/MLLP ───────────────────────────────────────────────────

    _checkPort(port) {
        return new Promise((resolve, reject) => {
            const s = net.createConnection(port, '127.0.0.1');
            s.setTimeout(5000);
            s.once('connect', () => { s.destroy(); resolve(`Port ${port} is open`); });
            s.on('timeout',   () => { s.destroy(); reject(new Error(`Connection to port ${port} timed out`)); });
            s.on('error',     err => reject(new Error(`Port ${port}: ${err.message}`)));
        });
    }

    _sendMLLP(port, hl7) {
        return new Promise((resolve, reject) => {
            const socket = net.createConnection(port, '127.0.0.1');
            let buf  = Buffer.alloc(0);
            let done = false;

            const finish = (err, val) => {
                if (done) return;
                done = true;
                socket.destroy();
                if (err) reject(err);
                else resolve(val);
            };

            const frame = Buffer.concat([
                Buffer.from([MLLP_START]),
                Buffer.from(hl7, 'utf8'),
                Buffer.from([MLLP_END1, MLLP_END2]),
            ]);

            socket.setTimeout(10000);
            socket.once('connect', () => socket.write(frame));
            socket.on('data', chunk => {
                buf = Buffer.concat([buf, chunk]);
                if (buf.indexOf(MLLP_END1) !== -1) {
                    const ackCode = this._parseAckCode(buf.toString('utf8'));
                    if (ackCode === 'AA') finish(null, `ACK=${ackCode}`);
                    else finish(new Error(`Received ACK code "${ackCode}" — expected "AA"`));
                }
            });
            socket.on('timeout', () => finish(new Error('MLLP ACK timeout (10 s)')));
            socket.on('error',   e  => finish(e));
            socket.on('close',   () => finish(new Error('Socket closed before ACK was received')));
        });
    }

    _parseAckCode(raw) {
        const msaLine = raw.split(/[\r\n]/).find(l => l.startsWith('MSA'));
        if (!msaLine) return 'UNKNOWN';
        return (msaLine.split('|')[1] || 'UNKNOWN').trim();
    }

    // ── S4: FHIR HTTP ────────────────────────────────────────────────────────

    _checkFHIR() {
        const body = JSON.stringify({
            resourceType: 'Patient',
            name: [{ family: 'SMOKETEST', given: ['System'] }],
            birthDate: '1990-01-01',
            gender: 'unknown',
        });
        return new Promise((resolve, reject) => {
            const req = http.request({
                hostname: '127.0.0.1',
                port:     PORT_FHIR,
                path:     '/fhir/r4/Patient',
                method:   'POST',
                headers:  {
                    'Content-Type':   'application/fhir+json',
                    'Content-Length': Buffer.byteLength(body),
                },
                timeout: 10000,
            }, res => {
                const status = res.statusCode;
                res.resume(); // drain
                if ([200, 201, 202].includes(status)) resolve(`HTTP ${status}`);
                else reject(new Error(`FHIR endpoint returned HTTP ${status}`));
            });
            req.on('timeout', () => { req.destroy(); reject(new Error('FHIR HTTP request timed out')); });
            req.on('error',   reject);
            req.write(body);
            req.end();
        });
    }

    // ── Message check (shared by S2-003, S3-003, S4-002, S5-002) ────────────

    async _checkMessages(interfaceId, waitMs) {
        if (!interfaceId) throw new Error('Interface ID not available (creation may have failed earlier)');
        await this._sleep(waitMs);
        const r     = await this._api('GET', `/api/messages/interface/${interfaceId}?limit=5`);
        const inner = r.data?.data || r.data;
        const msgs  = Array.isArray(inner) ? inner : (inner?.messages || []);
        if (msgs.length === 0) throw new Error('No messages found — pipeline may not have stored the message');
        return `${msgs.length} message(s) stored (status: ${msgs[0]?.status || '?'})`;
    }

    // ── S6: Monitoring ───────────────────────────────────────────────────────

    async _checkMonitoring() {
        const r = await this._api('GET', '/api/analytics/monitoring/summary');
        if (r.status >= 400) throw new Error(`Monitoring API returned HTTP ${r.status}`);
        return 'Monitoring summary OK';
    }

    // ── S9: Cleanup ──────────────────────────────────────────────────────────

    async _cleanup() {
        const ids = Object.values(this._state).filter(Boolean);
        if (ids.length === 0) return;

        await this._step('S9-001', `Delete ${ids.length} seed interface(s)`, async () => {
            let ok = 0;
            for (const id of ids) {
                try {
                    await this._api('POST', `/api/wizard/interfaces/${id}/deactivate`, { reason: 'smoke-teardown' }).catch(() => {});
                    await this._sleep(300);
                    await this._api('DELETE', `/api/interfaces/${id}`);
                    ok++;
                } catch (_) {}
            }
            this._state = {};
            return `Deleted ${ok} of ${ids.length}`;
        });
    }

    // ── Interface payloads ───────────────────────────────────────────────────

    _adtPayload() {
        return {
            name:        `${NAME_PREFIX}ADT_${PORT_ADT}`,
            description: 'System smoke test — ADT to FHIR',
            sourceType:  'hl7v2',
            sourceConnectivity:     'tcp_mllp_inbound',
            sourceConfig:           { host: '0.0.0.0', port: PORT_ADT },
            sourceConnectorConfig:  { connectorType: 'tcp_mllp_inbound', config: { host: '0.0.0.0', port: PORT_ADT } },
            targetType:  'fhir',
            targetConnectivity:     'http_fhir_outbound',
            targetConfig:           { baseUrl: 'http://hapi.fhir.org/baseR4', timeout: 30 },
            targetConnectorConfig:  { connectorType: 'http_fhir_outbound', config: { baseUrl: 'http://hapi.fhir.org/baseR4', timeout: 30 } },
            messageType: 'ADT^A01',
            mappings:    [],
            transformationFlow: 'hl7_to_fhir',
            auto_start:  true,
            status:      'active',
        };
    }

    _oruPayload() {
        return {
            ...this._adtPayload(),
            name:                  `${NAME_PREFIX}ORU_${PORT_ORU}`,
            description:           'System smoke test — ORU to FHIR',
            sourceConfig:          { host: '0.0.0.0', port: PORT_ORU },
            sourceConnectorConfig: { connectorType: 'tcp_mllp_inbound', config: { host: '0.0.0.0', port: PORT_ORU } },
            messageType:           'ORU^R01',
        };
    }

    _fhirPayload() {
        return {
            name:        `${NAME_PREFIX}FHIR_${PORT_FHIR}`,
            description: 'System smoke test — FHIR passthrough',
            sourceType:  'fhir',
            sourceConnectivity:     'http_fhir_inbound',
            sourceConfig:           { host: '0.0.0.0', port: PORT_FHIR },
            sourceConnectorConfig:  { connectorType: 'http_fhir_inbound', config: { host: '0.0.0.0', port: PORT_FHIR } },
            targetType:  'fhir',
            targetConnectivity:     'http_fhir_outbound',
            targetConfig:           { baseUrl: 'http://hapi.fhir.org/baseR4', timeout: 30 },
            targetConnectorConfig:  { connectorType: 'http_fhir_outbound', config: { baseUrl: 'http://hapi.fhir.org/baseR4', timeout: 30 } },
            messageType: 'FHIR:Patient',
            mappings:    [],
            transformationFlow: 'fhir_passthrough',
            auto_start:  true,
            status:      'active',
        };
    }

    _filePayload() {
        return {
            ...this._adtPayload(),
            name:                  `${NAME_PREFIX}FILE_${PORT_FILE}`,
            description:           'System smoke test — HL7 to file',
            sourceConfig:          { host: '0.0.0.0', port: PORT_FILE },
            sourceConnectorConfig: { connectorType: 'tcp_mllp_inbound', config: { host: '0.0.0.0', port: PORT_FILE } },
            targetType:            'file',
            targetConnectivity:    'file_writer',
            targetConfig:          { outputPath: '/tmp/smoke-test/{timestamp}.hl7', createDirs: true },
            targetConnectorConfig: { connectorType: 'file_writer', config: { outputPath: '/tmp/smoke-test/{timestamp}.hl7', createDirs: true } },
            transformationFlow:    'hl7_passthrough',
        };
    }

    // ── Internal HTTP helper ─────────────────────────────────────────────────

    async _api(method, path, body) {
        const axios   = require('axios');
        const headers = {};
        if (this._cookie)  headers['Cookie']                  = this._cookie;
        if (this._secret)  headers['X-Internal-Proxy-Secret'] = this._secret;
        if (body && method !== 'GET' && method !== 'DELETE') {
            headers['Content-Type'] = 'application/json';
        }
        try {
            return await axios({
                method,
                url:            this._base + path,
                data:           (method !== 'GET' && method !== 'DELETE') ? body : undefined,
                headers,
                timeout:        20000,
                validateStatus: null,
            });
        } catch (err) {
            throw new Error(`${method} ${path} → ${err.message}`);
        }
    }

    _sleep(ms) { return new Promise(r => setTimeout(r, ms)); }
}

module.exports = { SmokeTestRunner };
