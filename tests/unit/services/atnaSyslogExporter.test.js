// tests/unit/services/atnaSyslogExporter.test.js
//
// Node-side mirror of services/audit/atna_syslog_test.go — this file is the
// checked-in, repeatable version of what the Node ATNA exporter's own wire
// format and configuration parsing must produce, matching the Go
// implementation's own shape byte-for-byte where they're meant to agree.
//
// services/settingsService.js is mocked throughout: resolveConfig() (added
// when ATNA export became admin-UI-configurable, see the settings-related
// tests near the bottom) checks it BEFORE falling back to env vars, and it
// opens a real Postgres pool — without mocking it, every test would attempt
// a real DB connection (flaky/slow depending on whether the test host has
// Postgres reachable at localhost:5432, and irrelevant to what these tests
// actually verify).
jest.mock('../../../services/settingsService');

const dgram = require('dgram');
const net = require('net');

const ORIGINAL_ENV = { ...process.env };

function resetEnv() {
    for (const key of Object.keys(process.env)) {
        if (key.startsWith('ATNA_')) delete process.env[key];
    }
    Object.assign(process.env, ORIGINAL_ENV);
}

describe('atnaSyslogExporter', () => {
    let atnaSyslog;
    let mockSettingsService;

    beforeEach(() => {
        resetEnv();
        for (const key of Object.keys(process.env)) {
            if (key.startsWith('ATNA_')) delete process.env[key];
        }
        jest.resetModules();
        mockSettingsService = require('../../../services/settingsService');
        // Disabled by default so every pre-existing test below (which drives
        // behavior purely via env vars) exercises the fallback path exactly
        // as before this mock existed.
        mockSettingsService.getATNASyslogSettings = jest.fn().mockResolvedValue({
            enabled: false, host: '', port: 514, protocol: 'udp', facility: 10,
            app_name: 'ezHealthKonnect', audit_source_id: 'ezHealthKonnect',
            enterprise_site_id: '', tls_insecure_skip_verify: false,
        });
        atnaSyslog = require('../../../services/audit/atnaSyslogExporter');
    });

    afterAll(() => {
        resetEnv();
    });

    describe('configFromEnv', () => {
        it('returns null when ATNA_SYSLOG_HOST is unset', () => {
            expect(atnaSyslog.configFromEnv()).toBeNull();
        });

        it('applies defaults and overrides', () => {
            process.env.ATNA_SYSLOG_HOST = 'arr.example.internal';
            let cfg = atnaSyslog.configFromEnv();
            expect(cfg.protocol).toBe('udp');
            expect(cfg.port).toBe(514);
            expect(cfg.facility).toBe(10);
            expect(cfg.appName).toBe('ezHealthKonnect');
            expect(cfg.sourceId).toBe('ezHealthKonnect');

            process.env.ATNA_SYSLOG_PROTOCOL = 'tls';
            cfg = atnaSyslog.configFromEnv();
            expect(cfg.port).toBe(6514);

            process.env.ATNA_SYSLOG_PORT = '1514';
            cfg = atnaSyslog.configFromEnv();
            expect(cfg.port).toBe(1514);
        });
    });

    describe('atnaOutcomeIndicator', () => {
        it.each([
            ['success', '0'],
            ['failure', '4'],
            ['error', '8'],
            ['unknown', '4'],
            [undefined, '4'],
        ])('maps %s to %s', (result, expected) => {
            expect(atnaSyslog.atnaOutcomeIndicator(result)).toBe(expected);
        });
    });

    describe('syslogSeverity', () => {
        it.each([
            ['critical', 2],
            ['high', 3],
            ['medium', 5],
            ['low', 6],
            ['', 6],
        ])('maps risk %s to severity %d', (risk, expected) => {
            expect(atnaSyslog.syslogSeverity(risk)).toBe(expected);
        });
    });

    describe('xmlEscape', () => {
        it('escapes all 4 XML special characters', () => {
            expect(atnaSyslog.xmlEscape(`a & b < c > d "e"`)).toBe('a &amp; b &lt; c &gt; d &quot;e&quot;');
        });
    });

    describe('buildAuditMessageXML', () => {
        const cfg = { sourceId: 'ezHealthKonnect', enterpriseSiteId: 'site-1' };
        const taxonomyEntry = { atnaEventId: '110114', atnaEventIdDisplay: 'User Authentication', eventActionCode: 'E' };

        it('produces well-formed XML with the real taxonomy EventID', () => {
            const eventData = { action: 'LOGIN_SUCCESS', userId: 'user-1', entityType: 'User', entityId: 'user-1', ipAddress: '10.0.0.5' };
            const resolved = { result: 'success' };
            const xml = atnaSyslog.buildAuditMessageXML(eventData, resolved, taxonomyEntry, cfg);

            expect(xml).toContain('code="110114"');
            expect(xml).toContain('displayName="User Authentication"');
            expect(xml).toContain('EventOutcomeIndicator="0"');
            expect(xml).toContain('UserID="user-1"');
            expect(xml).toContain('NetworkAccessPointID="10.0.0.5"');
            expect(xml).toContain('AuditSourceID="ezHealthKonnect"');
            expect(xml).toContain('AuditEnterpriseSiteID="site-1"');
            expect(xml).toContain('ParticipantObjectID="user-1"');
            expect(xml).toContain('ParticipantObjectTypeCode="1"'); // User -> Person
        });

        it('uses System Object type code for non-User entities', () => {
            const eventData = { action: 'INTERFACE_UPDATED', entityType: 'Interface', entityId: 'iface-1' };
            const xml = atnaSyslog.buildAuditMessageXML(eventData, { result: 'success' }, {}, cfg);
            expect(xml).toContain('ParticipantObjectTypeCode="2"');
        });

        it('omits ParticipantObjectIdentification when there is no entityId', () => {
            const xml = atnaSyslog.buildAuditMessageXML({ action: 'APPLICATION_STARTED' }, { result: 'success' }, {}, cfg);
            expect(xml).not.toContain('ParticipantObjectIdentification');
        });

        it('defaults UserID to "system" for a userless event', () => {
            const xml = atnaSyslog.buildAuditMessageXML({ action: 'APPLICATION_STARTED' }, { result: 'success' }, {}, cfg);
            expect(xml).toContain('UserID="system"');
        });

        it('falls back to generic Application Activity EventID for an unregistered action', () => {
            const xml = atnaSyslog.buildAuditMessageXML({ action: 'UNKNOWN' }, { result: 'success' }, undefined, cfg);
            expect(xml).toContain('code="110100"');
            expect(xml).toContain('displayName="Application Activity"');
        });

        it('escapes user-controlled values embedded in attributes', () => {
            const eventData = { action: 'LOGIN_FAILED', userId: '<script>alert(1)</script>', entityType: 'User', entityId: 'x' };
            const xml = atnaSyslog.buildAuditMessageXML(eventData, { result: 'failure' }, {}, cfg);
            expect(xml).not.toContain('<script>');
            expect(xml).toContain('&lt;script&gt;');
        });
    });

    describe('buildSyslogMessage — RFC 5424 framing', () => {
        it('produces the correct PRI, headers, BOM, and body', () => {
            const cfg = { facility: 10, appName: 'ezHealthKonnect' };
            const body = '<AuditMessage/>';
            const msg = atnaSyslog.buildSyslogMessage(cfg, 'high', '110100', body);
            const s = msg.toString('utf8');

            // facility(10)*8 + severity(high=3) = 83
            expect(s.startsWith('<83>1 ')).toBe(true);
            expect(s).toContain('ezHealthKonnect');
            expect(s).toContain('110100');
            expect(s).toContain('<AuditMessage/>');

            // Both offsets computed on the raw Buffer (byte offsets) to
            // avoid a byte-vs-character-index mismatch — the BOM decodes to
            // a single character in a UTF-8 string but is 3 bytes on the wire.
            const bomIndex = msg.indexOf(Buffer.from([0xEF, 0xBB, 0xBF]));
            const bodyIndex = msg.indexOf(Buffer.from('<AuditMessage/>', 'utf8'));
            expect(bomIndex).toBeGreaterThan(-1);
            expect(bomIndex + 3).toBe(bodyIndex);
        });
    });

    // --- Real wire-level tests: actual local UDP/TCP listeners ---

    it('delivers a real UDP packet end-to-end', (done) => {
        const server = dgram.createSocket('udp4');
        server.on('message', (msg) => {
            expect(msg.toString('utf8')).toContain('test-udp-node');
            server.close();
            done();
        });
        server.bind(0, '127.0.0.1', () => {
            const port = server.address().port;
            process.env.ATNA_SYSLOG_HOST = '127.0.0.1';
            process.env.ATNA_SYSLOG_PORT = String(port);
            process.env.ATNA_SYSLOG_PROTOCOL = 'udp';
            jest.resetModules();
            // jest.resetModules() gives resolveConfig() a FRESH auto-mock of
            // settingsService with no return value configured — re-apply the
            // same "disabled" default beforeEach sets, so this test exercises
            // the env-var fallback path cleanly instead of an unrelated
            // "settings read failed" warning.
            require('../../../services/settingsService').getATNASyslogSettings = jest.fn().mockResolvedValue({ enabled: false, host: '' });
            const fresh = require('../../../services/audit/atnaSyslogExporter');
            fresh.exportEvent(
                { action: 'LOGIN_SUCCESS', userId: 'u1' },
                { result: 'success', riskLevel: 'low' },
                { atnaEventId: '110114', atnaEventIdDisplay: 'test-udp-node', eventActionCode: 'E' }
            );
        });
    }, 10000);

    it('delivers a real TCP packet with correct octet-counted framing', (done) => {
        const server = net.createServer((conn) => {
            let received = Buffer.alloc(0);
            conn.on('data', (chunk) => {
                received = Buffer.concat([received, chunk]);
                const s = received.toString('utf8');
                const spaceIdx = s.indexOf(' ');
                if (spaceIdx === -1) return;
                const declaredLen = parseInt(s.slice(0, spaceIdx), 10);
                const actualLen = received.length - spaceIdx - 1;
                if (actualLen < declaredLen) return; // wait for more data
                expect(actualLen).toBe(declaredLen);
                expect(s).toContain('test-tcp-node');
                server.close();
                conn.end();
                done();
            });
        });
        server.listen(0, '127.0.0.1', () => {
            const port = server.address().port;
            process.env.ATNA_SYSLOG_HOST = '127.0.0.1';
            process.env.ATNA_SYSLOG_PORT = String(port);
            process.env.ATNA_SYSLOG_PROTOCOL = 'tcp';
            jest.resetModules();
            require('../../../services/settingsService').getATNASyslogSettings = jest.fn().mockResolvedValue({ enabled: false, host: '' });
            const fresh = require('../../../services/audit/atnaSyslogExporter');
            fresh.exportEvent(
                { action: 'LOGIN_SUCCESS', userId: 'u1' },
                { result: 'success', riskLevel: 'low' },
                { atnaEventId: '110114', atnaEventIdDisplay: 'test-tcp-node', eventActionCode: 'E' }
            );
        });
    }, 10000);

    it('is a silent no-op when ATNA_SYSLOG_HOST is unset (and live settings disabled)', async () => {
        delete process.env.ATNA_SYSLOG_HOST;
        await expect(atnaSyslog.exportEvent({ action: 'LOGIN_SUCCESS' }, { result: 'success' }, {})).resolves.toBeUndefined();
    });

    describe('resolveConfig — live settings take priority over env vars', () => {
        it('uses live settings when enabled, ignoring env vars entirely', async () => {
            process.env.ATNA_SYSLOG_HOST = 'env-host-should-be-ignored';
            mockSettingsService.getATNASyslogSettings.mockResolvedValue({
                enabled: true, host: 'db-configured-host', port: 1234, protocol: 'tcp', facility: 4,
                app_name: 'CustomApp', audit_source_id: 'CustomSource', enterprise_site_id: 'site-42',
                tls_insecure_skip_verify: true,
            });
            const cfg = await atnaSyslog.resolveConfig();
            expect(cfg.host).toBe('db-configured-host');
            expect(cfg.port).toBe(1234);
            expect(cfg.protocol).toBe('tcp');
            expect(cfg.facility).toBe(4);
            expect(cfg.appName).toBe('CustomApp');
            expect(cfg.sourceId).toBe('CustomSource');
            expect(cfg.enterpriseSiteId).toBe('site-42');
            expect(cfg.tlsInsecureSkipVerify).toBe(true);
        });

        it('falls back to env vars when live settings are disabled', async () => {
            process.env.ATNA_SYSLOG_HOST = 'env-configured-host';
            mockSettingsService.getATNASyslogSettings.mockResolvedValue({ enabled: false, host: '' });
            const cfg = await atnaSyslog.resolveConfig();
            expect(cfg.host).toBe('env-configured-host');
        });

        it('falls back to env vars when live settings are enabled but host is blank', async () => {
            process.env.ATNA_SYSLOG_HOST = 'env-configured-host';
            mockSettingsService.getATNASyslogSettings.mockResolvedValue({ enabled: true, host: '' });
            const cfg = await atnaSyslog.resolveConfig();
            expect(cfg.host).toBe('env-configured-host');
        });

        it('falls back to env vars gracefully when the settings read itself throws', async () => {
            process.env.ATNA_SYSLOG_HOST = 'env-configured-host';
            mockSettingsService.getATNASyslogSettings.mockRejectedValue(new Error('DB unreachable'));
            const cfg = await atnaSyslog.resolveConfig();
            expect(cfg.host).toBe('env-configured-host');
        });

        it('resolves to null when both live settings and env vars are unconfigured', async () => {
            delete process.env.ATNA_SYSLOG_HOST;
            mockSettingsService.getATNASyslogSettings.mockResolvedValue({ enabled: false, host: '' });
            const cfg = await atnaSyslog.resolveConfig();
            expect(cfg).toBeNull();
        });
    });
});
