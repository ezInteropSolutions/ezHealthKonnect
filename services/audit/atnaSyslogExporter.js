// services/audit/atnaSyslogExporter.js
//
// Node-side mirror of the Go ATNA syslog exporter
// (services/audit/{atna_message.go,syslog_sender.go,syslog_audit_logger.go}).
// The Go audit path (~9 NewPostgresAuditLogger construction sites) already
// exports every Go-emitted event over ATNA syslog — this file gives the
// SAME coverage to every Node-emitted event (routes/auth.js,
// routes/users.js, controllers/interfacesController.js,
// controllers/pipelineController.js, etc. — all wired through
// services/auditService.js's own logEvent, the single Node audit
// chokepoint). Without this, roughly half this codebase's audited surface
// (login/logout, interface CRUD, user management, pipeline CRUD,
// credential changes — several of them HIGH risk) would silently never
// reach an external Audit Record Repository, defeating the point of ATNA
// export as a compliance feature.
//
// Same environment variables, same RFC 5424 framing, same DICOM PS3.15/
// RFC 3881 AuditMessage XML shape as the Go implementation — see
// atna_message.go's own header for the full scope/honesty caveat (a
// pragmatic subset, not independently validated against a live ARR; no
// real target is available in this environment).
const dgram = require('dgram');
const net = require('net');
const tls = require('tls');
const os = require('os');
const settingsService = require('../settingsService');

// resolveConfig checks the LIVE, admin-UI-editable settings (system_settings
// key "atna_syslog", the same row the Go side's SettingsProvider hook reads)
// first, falling back to ATNA_SYSLOG_* env vars when that row is
// disabled/unconfigured — mirrors resolveSyslogConfig() in
// services/audit/syslog_sender.go (Go) field-for-field. Async because the DB
// read is; exportEvent below awaits it without ever propagating a failure to
// its own caller (services/auditService.js's logEvent never awaits
// exportEvent at all — fire-and-forget, same as the Go decorator).
async function resolveConfig() {
    try {
        const live = await settingsService.getATNASyslogSettings();
        if (live.enabled && live.host) {
            return {
                host: live.host,
                port: live.port,
                protocol: live.protocol,
                facility: live.facility,
                appName: live.app_name,
                sourceId: live.audit_source_id || live.app_name,
                enterpriseSiteId: live.enterprise_site_id || '',
                tlsInsecureSkipVerify: !!live.tls_insecure_skip_verify,
            };
        }
    } catch (err) {
        console.warn(`⚠️  [atna-syslog] failed to load live settings, falling back to env vars: ${err.message}`);
    }
    return configFromEnv();
}

function configFromEnv() {
    const host = (process.env.ATNA_SYSLOG_HOST || '').trim();
    if (!host) return null;

    const protocol = (process.env.ATNA_SYSLOG_PROTOCOL || 'udp').trim().toLowerCase();
    let port = protocol === 'tls' ? 6514 : 514;
    if (process.env.ATNA_SYSLOG_PORT) {
        const p = parseInt(process.env.ATNA_SYSLOG_PORT, 10);
        if (!isNaN(p) && p > 0 && p <= 65535) port = p;
    }
    let facility = 10; // security/authorization messages (RFC 5424 Table 1)
    if (process.env.ATNA_SYSLOG_FACILITY) {
        const f = parseInt(process.env.ATNA_SYSLOG_FACILITY, 10);
        if (!isNaN(f) && f >= 0 && f <= 23) facility = f;
    }
    const appName = process.env.ATNA_SYSLOG_APP_NAME || 'ezHealthKonnect';

    return {
        host, port, protocol, facility, appName,
        sourceId: process.env.ATNA_AUDIT_SOURCE_ID || appName,
        enterpriseSiteId: process.env.ATNA_AUDIT_ENTERPRISE_SITE_ID || '',
        tlsInsecureSkipVerify: (process.env.ATNA_SYSLOG_TLS_INSECURE_SKIP_VERIFY || '').toLowerCase() === 'true',
    };
}

function xmlEscape(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function atnaOutcomeIndicator(result) {
    switch (result) {
        case 'success': return '0';
        case 'failure': return '4';
        case 'error': return '8';
        default: return '4';
    }
}

// buildAuditMessageXML mirrors atna_message.go's buildAuditMessageXML
// exactly — same element shape, same attribute set, same fallback rules.
function buildAuditMessageXML(eventData, resolved, taxonomyEntry, cfg) {
    const eventId = (taxonomyEntry && taxonomyEntry.atnaEventId) || '110100';
    const eventIdDisplay = (taxonomyEntry && taxonomyEntry.atnaEventIdDisplay) || 'Application Activity';
    const eventActionCode = (taxonomyEntry && taxonomyEntry.eventActionCode) || '';
    const userId = eventData.userId || 'system';
    const ip = eventData.ipAddress || '';
    const entityId = eventData.entityId || '';
    const entityType = eventData.entityType || '';

    const eventDateTime = new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');

    let participantObject = '';
    if (entityId) {
        const typeCode = entityType === 'User' ? '1' : '2';
        participantObject =
            `<ParticipantObjectIdentification ParticipantObjectID="${xmlEscape(entityId)}" ParticipantObjectTypeCode="${typeCode}">` +
            `<ParticipantObjectIDTypeCode code="1" codeSystemName="RFC-3881" displayName="${xmlEscape(entityType || 'Entity')}"/>` +
            `</ParticipantObjectIdentification>`;
    }

    const networkAttrs = ip ? ` NetworkAccessPointID="${xmlEscape(ip)}" NetworkAccessPointTypeCode="2"` : '';
    const siteAttr = cfg.enterpriseSiteId ? ` AuditEnterpriseSiteID="${xmlEscape(cfg.enterpriseSiteId)}"` : '';

    return `<AuditMessage>` +
        `<EventIdentification EventActionCode="${xmlEscape(eventActionCode)}" EventDateTime="${eventDateTime}" EventOutcomeIndicator="${atnaOutcomeIndicator(resolved.result)}">` +
        `<EventID code="${xmlEscape(eventId)}" codeSystemName="DCM" displayName="${xmlEscape(eventIdDisplay)}"/>` +
        `</EventIdentification>` +
        `<ActiveParticipant UserID="${xmlEscape(userId)}" UserIsRequestor="true"${networkAttrs}/>` +
        `<AuditSourceIdentification AuditSourceID="${xmlEscape(cfg.sourceId)}"${siteAttr}>` +
        `<AuditSourceTypeCode>4</AuditSourceTypeCode>` +
        `</AuditSourceIdentification>` +
        participantObject +
        `</AuditMessage>`;
}

function syslogSeverity(riskLevel) {
    switch (riskLevel) {
        case 'critical': return 2;
        case 'high': return 3;
        case 'medium': return 5;
        case 'low': return 6;
        default: return 6;
    }
}

// buildSyslogMessage mirrors syslog_sender.go's buildSyslogMessage exactly
// — same RFC 5424 header shape, same UTF-8 BOM convention before MSG.
function buildSyslogMessage(cfg, riskLevel, atnaEventId, xmlBody) {
    const pri = cfg.facility * 8 + syslogSeverity(riskLevel);
    const timestamp = new Date().toISOString(); // RFC3339 w/ millisecond precision — valid RFC 5424 TIMESTAMP
    const hostname = os.hostname() || '-';
    const procId = String(process.pid);
    const msgId = atnaEventId || '-';
    const header = `<${pri}>1 ${timestamp} ${hostname} ${cfg.appName} ${procId} ${msgId} - `;
    const bom = Buffer.from([0xEF, 0xBB, 0xBF]);
    return Buffer.concat([Buffer.from(header, 'utf8'), bom, Buffer.from(xmlBody, 'utf8')]);
}

// Persistent TCP/TLS connection, reconnected lazily on the next send after a
// failure — same convention as syslog_sender.go's own sendStream.
let streamConn = null;

function sendUDP(cfg, message) {
    return new Promise((resolve, reject) => {
        const sock = dgram.createSocket('udp4');
        sock.send(message, cfg.port, cfg.host, (err) => {
            sock.close();
            if (err) reject(err); else resolve();
        });
    });
}

function dialStream(cfg) {
    return new Promise((resolve, reject) => {
        const onError = (err) => reject(err);
        let sock;
        if (cfg.protocol === 'tls') {
            sock = tls.connect({ host: cfg.host, port: cfg.port, rejectUnauthorized: !cfg.tlsInsecureSkipVerify }, () => {
                sock.removeListener('error', onError);
                resolve(sock);
            });
        } else {
            sock = net.connect({ host: cfg.host, port: cfg.port }, () => {
                sock.removeListener('error', onError);
                resolve(sock);
            });
        }
        sock.once('error', onError);
        sock.setTimeout(5000, () => sock.destroy(new Error('atna syslog: connect timeout')));
    });
}

async function sendStream(cfg, message) {
    // RFC 5425 octet-counted framing: "MSG-LEN SP SYSLOG-MSG" — same
    // ambiguity-avoidance reasoning as syslog_sender.go's own sendStream.
    const framed = Buffer.concat([Buffer.from(`${message.length} `, 'utf8'), message]);

    if (streamConn && !streamConn.destroyed) {
        try {
            await writeToSocket(streamConn, framed);
            return;
        } catch (e) {
            streamConn = null;
        }
    }
    streamConn = await dialStream(cfg);
    await writeToSocket(streamConn, framed);
}

function writeToSocket(sock, data) {
    return new Promise((resolve, reject) => {
        sock.write(data, (err) => { if (err) reject(err); else resolve(); });
    });
}

async function send(cfg, message) {
    if (cfg.protocol === 'udp') {
        return sendUDP(cfg, message);
    }
    if (cfg.protocol === 'tcp' || cfg.protocol === 'tls') {
        return sendStream(cfg, message);
    }
    throw new Error(`atna syslog: unsupported protocol "${cfg.protocol}" (expected udp, tcp, or tls)`);
}

// exportEvent is the one function services/auditService.js calls — fires
// and forgets (never awaited by the caller, and never throws/rejects itself
// since every failure path below is caught internally), matching the Go
// decorator's "never let export failure/slowness affect the compliance-
// critical DB write, or the caller" design. A no-op when export is disabled
// per both the live settings and the env-var fallback.
async function exportEvent(eventData, resolved, taxonomyEntry) {
    try {
        const cfg = await resolveConfig();
        if (!cfg) return;

        const xmlBody = buildAuditMessageXML(eventData, resolved, taxonomyEntry, cfg);
        const message = buildSyslogMessage(cfg, resolved.riskLevel, taxonomyEntry && taxonomyEntry.atnaEventId, xmlBody);
        await send(cfg, message);
    } catch (err) {
        console.warn(`⚠️  [atna-syslog] failed to export action "${eventData.action}": ${err.message}`);
    }
}

module.exports = {
    configFromEnv,
    resolveConfig,
    exportEvent,
    // exported for unit testing only — mirrors the Go package's own
    // "exported for unit testing only" convention for its analogous helpers.
    buildAuditMessageXML,
    buildSyslogMessage,
    syslogSeverity,
    atnaOutcomeIndicator,
    xmlEscape,
};
