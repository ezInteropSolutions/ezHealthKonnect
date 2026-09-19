/**
 * ConnectorStepsDocs — Documentation for connector.* steps
 *
 * connector.inbound, connector.outbound.
 *
 * Self-registers into StepDocumentationRegistry at load time — this file must be
 * loaded (via <script>) after StepDocumentationRegistry.js and before any step's
 * Documentation tab is opened. Mirrors the StepBuilderRegistry.register() pattern
 * already used by every step's Configuration-tab builder
 * (public/js/pipeline/components/StepBuilderRegistry.js).
 */

(function () {
    const docs = {};
        docs['connector.inbound'] = {
            description: 'Starts a long-lived listener that receives messages from external systems. The most common type is TCP/MLLP Inbound which receives HL7 v2 messages from EHRs, lab systems, and other healthcare senders. Placed at sequence 5 in a pipeline — the engine starts one listener goroutine per connector.inbound step.',
            useCases: [
                'Receive HL7 ADT, ORU, or SIU messages from an EHR via TCP/MLLP (port 2575)',
                'Listen on multiple ports simultaneously by adding multiple connector.inbound steps',
                'Accept TLS-encrypted MLLP connections from external partners',
                'Receive messages and send AA/AE/AR acknowledgments back to the sender'
            ],
            example: {
                connectorType: 'tcp_mllp',
                config: {
                    host: '0.0.0.0',
                    port: 2575,
                    max_connections: 10,
                    ack: {
                        mode: 'immediate',
                        on_error: 'suppress',
                        sending_app: 'ezHealthKonnect',
                        sending_facility: 'EHK',
                        text_success: 'Message received successfully',
                        text_error: 'Message processing error'
                    }
                }
            },
            examples: [
                {
                    label: 'Basic TCP/MLLP — receive HL7 on port 6610 with immediate ACK',
                    config: {
                        connectorType: 'tcp_mllp',
                        config: {
                            host: '0.0.0.0',
                            port: 6610,
                            max_connections: 20,
                            timeout_seconds: 300,
                            ack: {
                                mode: 'immediate',
                                on_error: 'suppress',
                                sending_app: 'ezHealthKonnect',
                                sending_facility: 'EHK',
                                text_success: 'Message received',
                                text_error: 'Processing error'
                            }
                        }
                    }
                },
                {
                    label: 'TCP/MLLP with TLS — encrypted connections (port 2576)',
                    config: {
                        connectorType: 'tcp_mllp',
                        config: {
                            host: '0.0.0.0',
                            port: 2576,
                            enable_tls: true,
                            tls_cert_path: '/certs/server.crt',
                            tls_key_path: '/certs/server.key',
                            max_connections: 10,
                            ack: {
                                mode: 'immediate',
                                on_error: 'nack',
                                sending_app: 'SecureHIS',
                                sending_facility: 'PROD'
                            }
                        }
                    }
                },
                {
                    label: 'Custom ACK script — reject non-ADT messages with AR, accept all others with AA',
                    description: 'Use a custom script when you need conditional ACK logic beyond the standard mode/on_error settings. The function must be named buildACK(msg) and return { ackCode, textMessage }. Valid codes: AA (Accept), AE (Application Error), AR (Application Reject).',
                    config: {
                        connectorType: 'tcp_mllp',
                        config: {
                            host: '0.0.0.0',
                            port: 2575,
                            ack: {
                                mode: 'immediate',
                                on_error: 'nack',
                                sending_app: 'ezHealthKonnect',
                                sending_facility: 'EHK',
                                script: [
                                    'function buildACK(msg) {',
                                    '  // msg properties available:',
                                    '  //   msg.controlID      — MSH-10 message control ID',
                                    '  //   msg.messageType    — e.g. "ADT^A01", "ORU^R01"',
                                    '  //   msg.sendingApp     — MSH-3',
                                    '  //   msg.sendingFacility — MSH-4',
                                    '  //   msg.raw            — full raw HL7 message string',
                                    '  //   msg.defaultCode    — "AA" or "AE" (from mode/on_error)',
                                    '  //   msg.defaultText    — default MSA-3 text',
                                    '  var type = (msg.messageType || "").split("^")[0];',
                                    '  if (type !== "ADT" && type !== "ORU") {',
                                    '    return {',
                                    '      ackCode: "AR",',
                                    '      textMessage: "Unsupported message type: " + type',
                                    '    };',
                                    '  }',
                                    '  return {',
                                    '    ackCode: "AA",',
                                    '    textMessage: "Message accepted"',
                                    '  };',
                                    '}'
                                ].join('\n')
                            }
                        }
                    }
                },
                {
                    label: 'Custom ACK script — add patient ID to ACK text from parsed HL7',
                    config: {
                        connectorType: 'tcp_mllp',
                        config: {
                            host: '0.0.0.0',
                            port: 2575,
                            ack: {
                                mode: 'immediate',
                                on_error: 'suppress',
                                sending_app: 'ezHealthKonnect',
                                sending_facility: 'EHK',
                                script: [
                                    'function buildACK(msg) {',
                                    '  // Extract PID-3 (patient ID) from raw HL7',
                                    '  var patientId = "";',
                                    '  var lines = (msg.raw || "").split("\\r");',
                                    '  for (var i = 0; i < lines.length; i++) {',
                                    '    if (lines[i].indexOf("PID") === 0) {',
                                    '      var fields = lines[i].split("|");',
                                    '      patientId = fields[3] || "";',
                                    '      break;',
                                    '    }',
                                    '  }',
                                    '  return {',
                                    '    ackCode: "AA",',
                                    '    textMessage: patientId',
                                    '      ? "Accepted patient " + patientId',
                                    '      : "Message received"',
                                    '  };',
                                    '}'
                                ].join('\n')
                            }
                        }
                    }
                }
            ],
            connectorTypeCards: [
                {
                    typeName: 'tcp_mllp', displayName: 'TCP/MLLP (HL7 v2.x)', icon: '🔌', mode: 'push',
                    description: 'Long-lived TCP socket listener using the MLLP (Minimal Lower Layer Protocol) framing. The primary transport for HL7 v2 messages between EHRs, lab systems, and healthcare intermediaries. Sends AA/AE/AR acknowledgments back to the sender.',
                    notes: 'Supports TLS 1.2/1.3. Configure ACK behaviour in the Acknowledgment tab.',
                    required: ['port'],
                    keyFields: [
                        { name: 'port', type: 'integer', required: true, default: '2575', notes: 'Standard MLLP port. Use 2576 for TLS.' },
                        { name: 'host', type: 'string', default: '0.0.0.0', notes: 'Bind address. 0.0.0.0 listens on all interfaces.' },
                        { name: 'max_connections', type: 'integer', default: '10', notes: 'Max simultaneous HL7 sender connections.' },
                        { name: 'enable_tls', type: 'boolean', default: 'false', notes: 'Require TLS; also set tls_cert_path and tls_key_path.' },
                        { name: 'tls_cert_path', type: 'string', default: '—', notes: 'Path to PEM certificate file (when TLS enabled).' },
                        { name: 'tls_key_path', type: 'string', default: '—', notes: 'Path to PEM private key file (when TLS enabled).' }
                    ],
                    example: { connectorType: 'tcp_mllp', config: { host: '0.0.0.0', port: 2575, max_connections: 10, ack: { mode: 'immediate', on_error: 'suppress' } } }
                },
                {
                    typeName: 'http_rest', displayName: 'HTTP/REST API', icon: '🌐', mode: 'push',
                    description: 'Exposes an HTTP endpoint that external systems POST messages to. Useful for FHIR R4 sources, webhooks, and any system that speaks REST. Supports API key and bearer token authentication.',
                    required: ['endpoint_path'],
                    keyFields: [
                        { name: 'endpoint_path', type: 'string', required: true, default: '/api/hl7/receive', notes: 'URL path the server listens on.' },
                        { name: 'http_methods', type: 'array', default: '["POST"]', notes: 'Allowed HTTP methods: POST or PUT.' },
                        { name: 'authentication_type', type: 'enum', default: 'api_key', notes: 'none | api_key | basic_auth | bearer_token.' },
                        { name: 'api_key_header', type: 'string', default: 'X-API-Key', notes: 'Header name carrying the API key.' }
                    ],
                    example: { connectorType: 'http_rest', config: { endpoint_path: '/api/hl7/receive', http_methods: ['POST'], authentication_type: 'api_key', api_key_header: 'X-API-Key' } }
                },
                {
                    typeName: 'sftp_inbound', displayName: 'SFTP File Reader', icon: '🔐', mode: 'pull',
                    description: 'Polls a remote SFTP server directory for new files on a configurable interval. Uses the real SFTP protocol (not shell commands over SSH), so it works against properly locked-down SFTP-only servers. After downloading each file it can delete, archive, or leave it in place. Supports both password and SSH private key authentication.',
                    notes: 'Set "After Processing" to delete or archive to prevent reprocessing the same file on the next poll.',
                    required: ['host', 'username', 'remote_dir'],
                    keyFields: [
                        { name: 'host', type: 'string', required: true, default: '—', notes: 'SFTP server hostname or IP.' },
                        { name: 'port', type: 'integer', default: '22', notes: 'Standard SSH/SFTP port.' },
                        { name: 'username', type: 'string', required: true, default: '—', notes: 'SSH login username.' },
                        { name: 'auth_type', type: 'enum', default: 'password', notes: 'password | key. Selecting "key" shows the Private Key field below instead of Password.' },
                        { name: 'password', type: 'string (password)', default: '—', notes: 'SSH password. Only used when Auth Type = password.' },
                        { name: 'key_content', type: 'string (password)', default: '—', notes: 'Full PEM private key text. Only used when Auth Type = key — paste it directly, or use the "Browse for key file…" button to read it from a local file.' },
                        { name: 'remote_dir', type: 'string', required: true, default: '/inbox', notes: 'Remote path to poll for new files.' },
                        { name: 'file_pattern', type: 'string', default: '*.hl7', notes: 'Glob pattern to match files (e.g. *.hl7, ADT_*.txt).' },
                        { name: 'after_processing', type: 'enum', default: 'archive', notes: 'delete | archive | none.' },
                        { name: 'archive_dir', type: 'string', default: '<remote_dir>/processed', notes: 'Remote path to move processed files to when After Processing = archive.' },
                        { name: 'max_files_per_run', type: 'integer', default: '100', notes: 'Cap per poll cycle to avoid overload.' },
                        { name: 'poll_interval_sec', type: 'integer', default: '30', notes: 'Seconds between poll cycles.' },
                        { name: 'connect_timeout', type: 'integer', default: '10', notes: 'SSH connect timeout in seconds.' },
                        { name: 'read_timeout', type: 'integer', default: '60', notes: 'Per-file download timeout in seconds.' }
                    ],
                    example: { connectorType: 'sftp_inbound', config: { host: 'sftp.partner.org', port: 22, username: 'hl7feed', auth_type: 'key', key_content: '-----BEGIN PRIVATE KEY-----...', remote_dir: '/outbound/hl7', file_pattern: '*.hl7', after_processing: 'archive', archive_dir: '/outbound/hl7/processed', max_files_per_run: 50 } }
                },
                {
                    typeName: 'edi_x12_inbound', displayName: 'EDI X12 Inbound (835 / 837 / 999)', icon: '💰', mode: 'pull',
                    description: 'Polls a remote SFTP directory for X12 EDI files. Supports 835 (Health Care Claim Payment/Advice), 837P/837I (Professional/Institutional claims), and 999 (Functional Acknowledgment). A file may contain multiple ST...SE transaction sets; the engine splits them centrally after ingestion, so this connector does not need to. Downstream, use edi.parse → edi.validate to turn the raw X12 into structured JSON and check it against the X12 5010 standard — every message is also auto-parsed right after ingestion the same way, so an explicit edi.parse step is only needed when re-parsing content mid-pipeline.',
                    notes: 'Transport is locked to SFTP — HTTP and AS2 are handled by the separate as2_inbound connector, not a transport option here. 270/271 (eligibility) is transform-only (no connector changes needed) — see edi.parse/edi.validate/edi.build\'s own docs.',
                    required: ['transport', 'host', 'username'],
                    keyFields: [
                        { name: 'transport', type: 'enum', required: true, default: 'sftp', notes: 'Only "sftp" is implemented — any other value fails validation with a clear error rather than silently no-op\'ing.' },
                        { name: 'host', type: 'string', required: true, default: '—', notes: 'SFTP server hostname or IP.' },
                        { name: 'port', type: 'integer', default: '22', notes: 'Standard SSH/SFTP port.' },
                        { name: 'username', type: 'string', required: true, default: '—', notes: 'SSH login username.' },
                        { name: 'auth_type', type: 'enum', default: 'password', notes: 'password | key. Selecting "key" shows the Private Key field below instead of Password.' },
                        { name: 'password', type: 'string (password)', default: '—', notes: 'SSH password. Only used when Auth Type = password.' },
                        { name: 'key_content', type: 'string (password)', default: '—', notes: 'Full PEM private key text. Only used when Auth Type = key — paste it directly, or use the "Browse for key file…" button to read it from a local file.' },
                        { name: 'remote_path', type: 'string', default: '/incoming', notes: 'Remote directory to poll for new EDI files.' },
                        { name: 'file_pattern', type: 'string', default: '*.edi', notes: 'Glob pattern to match files.' },
                        { name: 'transaction_types', type: 'array (checkbox list)', default: '["835"]', notes: 'Which X12 transaction sets to accept: 835, 837P, 837I, or 999. edi.parse auto-detects the real variant per file from its own GS08, so this is a display/filter aid, not a hard gate.' },
                        { name: 'after_processing', type: 'enum', default: 'archive', notes: 'archive | delete | none.' },
                        { name: 'archive_dir', type: 'string', default: '<remote_path>/processed', notes: 'Remote path to move processed files to when After Processing = archive.' },
                        { name: 'max_files_per_run', type: 'integer', default: '100', notes: 'Cap per poll cycle to avoid overload.' },
                        { name: 'polling_interval_seconds', type: 'integer', default: '300', notes: 'Seconds between poll cycles.' },
                        { name: 'connect_timeout', type: 'integer', default: '10', notes: 'SSH connect timeout in seconds.' },
                        { name: 'read_timeout', type: 'integer', default: '60', notes: 'Per-file download timeout in seconds.' }
                    ],
                    example: { connectorType: 'edi_x12_inbound', config: { transport: 'sftp', host: 'payer-sftp.example.com', remote_path: '/incoming', file_pattern: '*.edi', polling_interval_seconds: 300, after_processing: 'archive', transaction_types: ['835', '837P', '837I', '999'] } }
                },
                {
                    typeName: 'as2_inbound', displayName: 'AS2 Receiver', icon: '🔐', mode: 'push',
                    description: 'Receives signed+encrypted EDI documents over HTTPS (AS2/RFC 4130) and returns a signed synchronous MDN receipt on the same connection. A persistent HTTPS listener, not a poller — long-lived, like TCP/MLLP. Real, direct-trust security: the partner\'s signature is verified against a manually-configured certificate, not just their claimed AS2 ID.',
                    notes: 'Synchronous MDN only — the partner gets its delivery receipt in the same HTTP response. Async MDN (partner posts the receipt back later to a separate URL) is a named, not-yet-built item.',
                    required: ['port', 'as2_to', 'own_cert_pem', 'own_key_pem', 'partner_cert_pem'],
                    keyFields: [
                        { name: 'port', type: 'integer', required: true, default: '—', notes: 'Listener port.' },
                        { name: 'base_path', type: 'string', default: '/as2', notes: 'URL path the partner POSTs to.' },
                        { name: 'as2_from', type: 'string', default: '—', notes: 'Expected partner AS2 ID — used for logging/MDN fields only; the real trust decision is the certificate, not this header value.' },
                        { name: 'as2_to', type: 'string', required: true, default: '—', notes: 'Your own AS2 station ID.' },
                        { name: 'own_cert_pem', type: 'string (password)', required: true, default: '—', notes: 'Signs outgoing MDNs and is the encryption recipient for incoming messages.' },
                        { name: 'own_key_pem', type: 'string (password)', required: true, default: '—', notes: 'Decrypts incoming messages.' },
                        { name: 'partner_cert_pem', type: 'string', required: true, default: '—', notes: 'Verifies the partner\'s signature on incoming messages.' },
                        { name: 'tls_enabled', type: 'boolean', default: 'false', notes: 'Real partner traffic should enable this.' },
                        { name: 'tls_cert_file', type: 'string', default: '—', notes: 'TLS certificate file path (when TLS enabled).' },
                        { name: 'tls_key_file', type: 'string', default: '—', notes: 'TLS key file path (when TLS enabled).' },
                        { name: 'request_timeout_seconds', type: 'integer', default: '30', notes: 'Per-request timeout.' }
                    ],
                    example: { connectorType: 'as2_inbound', config: { port: 8443, base_path: '/as2', as2_to: 'EZHEALTHKONNECT', own_cert_pem: '-----BEGIN CERTIFICATE-----...', own_key_pem: '-----BEGIN PRIVATE KEY-----...', partner_cert_pem: '-----BEGIN CERTIFICATE-----...', tls_enabled: true } }
                },
                {
                    typeName: 'direct_messaging_inbound', displayName: 'Direct Messaging Inbound', icon: '📧', mode: 'pull',
                    description: 'Receives clinical documents via DirectTrust — S/MIME-encrypted email polled over IMAP, decrypted and signature-verified against a configured partner certificate. Reuses the same CMS sign/verify/encrypt/decrypt primitives as AS2, over an email transport instead of HTTP.',
                    notes: 'Manually-configured partner cert only — no DNS CERT-record or LDAP auto-discovery. MDN-over-email (delivery/read receipts) is a named, not-yet-built item.',
                    required: ['imap_host', 'username', 'password', 'cert_content', 'private_key', 'partner_cert_pem'],
                    keyFields: [
                        { name: 'imap_host', type: 'string', required: true, default: '—', notes: 'IMAP server hostname.' },
                        { name: 'imap_port', type: 'integer', default: '993', notes: 'IMAP port.' },
                        { name: 'use_tls', type: 'boolean', default: 'true', notes: 'Disable only for a plain-text test mailbox — real Direct Trust traffic always uses TLS.' },
                        { name: 'username', type: 'string', required: true, default: '—', notes: 'Your Direct address, e.g. provider@direct.hospital.org.' },
                        { name: 'password', type: 'string (password)', required: true, default: '—', notes: 'Mailbox password.' },
                        { name: 'cert_content', type: 'string (password)', required: true, default: '—', notes: 'Own S/MIME certificate (PEM) — the encryption recipient for incoming messages.' },
                        { name: 'private_key', type: 'string (password)', required: true, default: '—', notes: 'Own private key (PEM) — decrypts incoming messages.' },
                        { name: 'partner_cert_pem', type: 'string', required: true, default: '—', notes: 'Verifies the sender\'s signature — direct-trust model, same as AS2.' },
                        { name: 'polling_interval_seconds', type: 'integer', default: '60', notes: 'Seconds between IMAP poll cycles.' }
                    ],
                    example: { connectorType: 'direct_messaging_inbound', config: { imap_host: 'imap.directtrust-hisp.com', imap_port: 993, username: 'provider@direct.hospital.org', password: '••••', cert_content: '-----BEGIN CERTIFICATE-----...', private_key: '-----BEGIN PRIVATE KEY-----...', partner_cert_pem: '-----BEGIN CERTIFICATE-----...', polling_interval_seconds: 60 } }
                },
                {
                    typeName: 'websocket_inbound', displayName: 'WebSocket Server', icon: '🔌', mode: 'push',
                    description: 'Accepts real-time websocket connections from external systems and enqueues each received frame as a message. A persistent listener, like TCP/MLLP, but framed as websocket instead of MLLP.',
                    notes: 'No pipeline-driven synchronous reply is sent back to the client after a message is enqueued (unlike MLLP\'s ACK) — replying with an actual pipeline result would require blocking the connection on full async pipeline completion, which no connector in this codebase does today.',
                    required: ['port'],
                    keyFields: [
                        { name: 'port', type: 'integer', required: true, default: '—', notes: 'Listener port.' },
                        { name: 'base_path', type: 'string', default: '/ws', notes: 'URL path clients connect to.' },
                        { name: 'tls_enabled', type: 'boolean', default: 'false', notes: 'Enable wss://.' },
                        { name: 'tls_cert_file', type: 'string', default: '—', notes: 'TLS certificate file path (when TLS enabled).' },
                        { name: 'tls_key_file', type: 'string', default: '—', notes: 'TLS key file path (when TLS enabled).' },
                        { name: 'max_message_size_mb', type: 'integer', default: '10', notes: 'Per-frame size cap (hard cap 100 MB).' },
                        { name: 'ping_interval_seconds', type: 'integer', default: '30', notes: 'Keepalive ping sent to each connected client.' },
                        { name: 'max_connections', type: 'integer', default: '100', notes: 'Concurrent connection cap — returns HTTP 503 on the upgrade request past capacity.' },
                        { name: 'authentication_type', type: 'enum', default: 'none', notes: 'none | bearer | basic, checked on the upgrade request.' },
                        { name: 'bearer_token', type: 'string (password)', default: '—', notes: 'Required when authentication_type = bearer.' },
                        { name: 'username', type: 'string', default: '—', notes: 'Required when authentication_type = basic.' },
                        { name: 'password', type: 'string (password)', default: '—', notes: 'Required when authentication_type = basic.' }
                    ],
                    example: { connectorType: 'websocket_inbound', config: { port: 9501, base_path: '/ws', max_connections: 50, authentication_type: 'none' } }
                },
                {
                    typeName: 'file_listener', displayName: 'File System Listener', icon: '📁', mode: 'pull',
                    description: 'Monitors a local directory on the server/container filesystem for new files on a schedule. Best for shared NFS mounts, local drop folders, or integration test scenarios where files are placed by another process.',
                    notes: 'Requires a cron schedule. The directory must be accessible inside the container — use a Docker volume mount.',
                    required: ['directory_path', 'file_pattern'],
                    keyFields: [
                        { name: 'directory_path', type: 'string', required: true, default: '—', notes: 'Absolute path on server, e.g. /data/hl7/inbox.' },
                        { name: 'file_pattern', type: 'string', required: true, default: '*.hl7', notes: 'Glob to match new files.' },
                        { name: 'after_processing', type: 'enum', default: 'move', notes: 'delete | move | archive | nothing.' },
                        { name: 'archive_directory', type: 'string', default: '—', notes: 'Path to move processed files.' },
                        { name: 'file_encoding', type: 'string', default: 'UTF-8', notes: 'Character encoding of incoming files.' }
                    ],
                    example: { connectorType: 'file_listener', config: { directory_path: '/data/hl7/inbox', file_pattern: '*.hl7', after_processing: 'move', archive_directory: '/data/hl7/archive' } }
                },
                {
                    typeName: 'postgresql_inbound', displayName: 'PostgreSQL Database Reader', icon: '🐘', mode: 'pull',
                    description: 'Polls a PostgreSQL table or view for new records on a schedule. Use an incremental column (e.g. id or updated_at) to fetch only new/changed rows. After fetching, can mark rows as processed via an update flag or delete them.',
                    notes: 'Requires a cron schedule. Use incremental_column to avoid reprocessing rows on every poll.',
                    required: ['host', 'port', 'database', 'query'],
                    keyFields: [
                        { name: 'host', type: 'string', required: true, default: 'localhost', notes: 'PostgreSQL host.' },
                        { name: 'port', type: 'integer', required: true, default: '5432', notes: 'PostgreSQL port.' },
                        { name: 'database', type: 'string', required: true, default: '—', notes: 'Database name.' },
                        { name: 'username', type: 'string', default: '—', notes: 'Login user.' },
                        { name: 'password', type: 'string (password)', default: '—', notes: 'Login password.' },
                        { name: 'query', type: 'string', required: true, default: '—', notes: 'SELECT query to fetch records. Use WHERE processed_flag = false for incremental.' },
                        { name: 'incremental_column', type: 'string', default: '—', notes: 'Column to track last polled value (id or updated_at).' },
                        { name: 'after_processing', type: 'enum', default: 'update_flag', notes: 'update_flag | delete | archive | nothing.' },
                        { name: 'update_column', type: 'string', default: '—', notes: 'Column to set when after_processing=update_flag.' },
                        { name: 'update_value', type: 'string', default: 'processed', notes: 'Value to set in update_column.' },
                        { name: 'max_records_per_poll', type: 'integer', default: '100', notes: 'LIMIT applied to the query per cron run.' }
                    ],
                    example: { connectorType: 'postgresql_inbound', config: { host: 'db.internal', port: 5432, database: 'ehr', username: 'reader', password: '••••', query: 'SELECT * FROM hl7_outbox WHERE processed = false ORDER BY created_at LIMIT 100', after_processing: 'update_flag', update_column: 'processed', update_value: 'true', max_records_per_poll: 100 } }
                },
                {
                    typeName: 'rabbitmq_inbound', displayName: 'RabbitMQ Consumer', icon: '🐰', mode: 'push',
                    description: 'Connects to a RabbitMQ broker and consumes messages from a queue using AMQP. Long-lived subscription — no cron needed. Supports exchange binding, manual acknowledgment, and TLS connections.',
                    required: ['host', 'port', 'queue_name'],
                    keyFields: [
                        { name: 'host', type: 'string', required: true, default: 'localhost', notes: 'RabbitMQ broker host.' },
                        { name: 'port', type: 'integer', required: true, default: '5672', notes: 'AMQP port (5671 for TLS).' },
                        { name: 'username', type: 'string', default: 'guest', notes: 'AMQP username.' },
                        { name: 'password', type: 'string (password)', default: 'guest', notes: 'AMQP password.' },
                        { name: 'vhost', type: 'string', default: '/', notes: 'RabbitMQ virtual host.' },
                        { name: 'queue_name', type: 'string', required: true, default: '—', notes: 'Queue to consume from.' },
                        { name: 'exchange_name', type: 'string', default: '—', notes: 'Optional exchange to bind the queue to.' },
                        { name: 'routing_key', type: 'string', default: '—', notes: 'Routing key for exchange binding.' },
                        { name: 'prefetch_count', type: 'integer', default: '1', notes: 'Messages to prefetch per consumer (flow control).' },
                        { name: 'auto_ack', type: 'boolean', default: 'false', notes: 'false = manual ack (safer); true = auto-ack.' }
                    ],
                    example: { connectorType: 'rabbitmq_inbound', config: { host: 'rabbitmq.internal', port: 5672, username: 'hl7consumer', password: '••••', vhost: '/healthcare', queue_name: 'hl7.inbound.adt', prefetch_count: 5, auto_ack: false, durable_queue: true } }
                },
                {
                    typeName: 'kafka_inbound', displayName: 'Kafka Consumer', icon: '⚡', mode: 'push',
                    description: 'Subscribes to a Kafka topic as a consumer group member. Long-lived connection — no cron needed. Offsets are committed after successful processing. Supports SASL/SCRAM authentication and SSL for secured Kafka clusters.',
                    required: ['bootstrap_servers', 'topic', 'group_id'],
                    keyFields: [
                        { name: 'bootstrap_servers', type: 'string', required: true, default: 'localhost:9092', notes: 'Comma-separated broker list, e.g. broker1:9092,broker2:9092.' },
                        { name: 'topic', type: 'string', required: true, default: '—', notes: 'Kafka topic name to consume from.' },
                        { name: 'group_id', type: 'string', required: true, default: '—', notes: 'Consumer group ID — use a unique ID per pipeline.' },
                        { name: 'client_id', type: 'string', default: '—', notes: 'Optional client identifier for monitoring.' },
                        { name: 'auto_offset_reset', type: 'enum', default: 'latest', notes: 'earliest = read from start; latest = only new messages.' },
                        { name: 'security_protocol', type: 'enum', default: 'PLAINTEXT', notes: 'PLAINTEXT | SSL | SASL_PLAINTEXT | SASL_SSL.' },
                        { name: 'sasl_mechanism', type: 'enum', default: 'PLAIN', notes: 'PLAIN | SCRAM-SHA-256 | SCRAM-SHA-512.' },
                        { name: 'sasl_username', type: 'string', default: '—', notes: 'SASL username for secured clusters.' },
                        { name: 'sasl_password', type: 'string (password)', default: '—', notes: 'SASL password.' },
                        { name: 'max_poll_records', type: 'integer', default: '500', notes: 'Max records per poll cycle.' }
                    ],
                    example: { connectorType: 'kafka_inbound', config: { bootstrap_servers: 'kafka1:9092,kafka2:9092', topic: 'hl7.inbound.adt', group_id: 'ezHealthKonnect-adt-pipeline', auto_offset_reset: 'latest', security_protocol: 'SASL_SSL', sasl_mechanism: 'SCRAM-SHA-256', sasl_username: 'hl7consumer', sasl_password: '••••' } }
                },
                {
                    typeName: 'redis_inbound', displayName: 'Redis Consumer', icon: '🔴', mode: 'push',
                    description: 'Consumes messages from Redis using list (LPOP/BRPOP), pub/sub channel subscription, or Redis Streams. Long-lived connection — no cron needed. List mode with blocking timeout is the simplest queue pattern.',
                    required: ['host', 'port', 'mode'],
                    keyFields: [
                        { name: 'host', type: 'string', required: true, default: 'localhost', notes: 'Redis host.' },
                        { name: 'port', type: 'integer', required: true, default: '6379', notes: 'Redis port.' },
                        { name: 'password', type: 'string (password)', default: '—', notes: 'AUTH password (if Redis requires auth).' },
                        { name: 'database', type: 'integer', default: '0', notes: 'Redis DB number (0–15).' },
                        { name: 'mode', type: 'enum', required: true, default: 'list_pop', notes: 'list_pop | pub_sub | stream.' },
                        { name: 'key_name', type: 'string', default: '—', notes: 'List key / channel name / stream name.' },
                        { name: 'consumer_group', type: 'string', default: '—', notes: 'For stream mode — consumer group name.' },
                        { name: 'blocking_timeout', type: 'integer', default: '0', notes: 'BLPOP timeout in seconds; 0 = block indefinitely.' }
                    ],
                    example: { connectorType: 'redis_inbound', config: { host: 'redis.internal', port: 6379, password: '••••', database: 0, mode: 'list_pop', key_name: 'hl7:queue:adt', blocking_timeout: 30 } }
                },
                {
                    typeName: 'aws_s3_inbound', displayName: 'AWS S3 Bucket Reader', icon: '☁️', mode: 'pull',
                    description: 'Polls an S3 bucket prefix for new objects on a schedule. After downloading each object, can delete, move to another prefix, or tag it. Supports IAM role (ECS/EKS task role) or explicit access keys. Credentials are stored encrypted.',
                    notes: 'Requires a cron schedule. Use IAM role auth in AWS-hosted deployments — no keys needed.',
                    required: ['bucket_name', 'region'],
                    keyFields: [
                        { name: 'bucket_name', type: 'string', required: true, default: '—', notes: 'S3 bucket name.' },
                        { name: 'region', type: 'string', required: true, default: 'us-east-1', notes: 'AWS region, e.g. us-east-1.' },
                        { name: 'prefix', type: 'string', default: '—', notes: 'Folder prefix, e.g. hl7/inbound/. Include trailing slash.' },
                        { name: 'file_pattern', type: 'string', default: '*.hl7', notes: 'Glob to filter object keys.' },
                        { name: 'authentication_method', type: 'enum', default: 'access_keys', notes: 'access_keys | iam_role | assumed_role.' },
                        { name: 'access_key_id', type: 'string', default: '—', notes: 'AWS access key (only for access_keys method).' },
                        { name: 'secret_access_key', type: 'string (password)', default: '—', notes: 'AWS secret key — stored encrypted.' },
                        { name: 'after_processing', type: 'enum', default: 'move', notes: 'delete | move | tag | nothing.' },
                        { name: 'max_objects_per_poll', type: 'integer', default: '100', notes: 'Max S3 objects downloaded per cron run.' }
                    ],
                    example: { connectorType: 'aws_s3_inbound', config: { bucket_name: 'ehr-hl7-feeds', region: 'us-east-1', prefix: 'inbound/adt/', file_pattern: '*.hl7', authentication_method: 'iam_role', after_processing: 'move', max_objects_per_poll: 100 } }
                },
                {
                    typeName: 'azure_blob_inbound', displayName: 'Azure Blob Storage Reader', icon: '☁️', mode: 'pull',
                    description: 'Polls an Azure Blob Storage container for new blobs on a schedule. Supports account key or full connection string authentication. After downloading, can delete, move to an archive container, or tag the blob.',
                    notes: 'Requires a cron schedule. Use connection_string for simplicity in dev; use account_name + account_key in production.',
                    required: ['account_name', 'container_name'],
                    keyFields: [
                        { name: 'account_name', type: 'string', required: true, default: '—', notes: 'Azure Storage account name.' },
                        { name: 'account_key', type: 'string (password)', default: '—', notes: 'Storage account key — stored encrypted.' },
                        { name: 'connection_string', type: 'string (password)', default: '—', notes: 'Alternative to account_name+key. Full Azure connection string.' },
                        { name: 'container_name', type: 'string', required: true, default: '—', notes: 'Blob container name.' },
                        { name: 'prefix', type: 'string', default: '—', notes: 'Blob prefix / virtual folder.' },
                        { name: 'blob_pattern', type: 'string', default: '*.hl7', notes: 'Glob to filter blob names.' },
                        { name: 'after_processing', type: 'enum', default: 'move', notes: 'delete | move | tag | nothing.' },
                        { name: 'archive_container', type: 'string', default: '—', notes: 'Container to move blobs to after processing.' },
                        { name: 'max_blobs_per_poll', type: 'integer', default: '100', notes: 'Max blobs per cron run.' }
                    ],
                    example: { connectorType: 'azure_blob_inbound', config: { account_name: 'myehrstorage', account_key: '••••', container_name: 'hl7-inbound', prefix: 'adt/', blob_pattern: '*.hl7', after_processing: 'move', archive_container: 'hl7-processed', max_blobs_per_poll: 50 } }
                },
                {
                    typeName: 'gcs_inbound', displayName: 'Google Cloud Storage Reader', icon: '☁️', mode: 'pull',
                    description: 'Polls a GCS bucket for new objects on a schedule. Authenticates using a Google Cloud service account JSON key. After downloading, can delete or move objects to an archive bucket/prefix.',
                    notes: 'Requires a cron schedule. The service account needs storage.objects.get and storage.objects.list permissions.',
                    required: ['bucket_name', 'credentials_json'],
                    keyFields: [
                        { name: 'bucket_name', type: 'string', required: true, default: '—', notes: 'GCS bucket name.' },
                        { name: 'credentials_json', type: 'string (password)', required: true, default: '—', notes: 'Full service account JSON content — stored encrypted.' },
                        { name: 'prefix', type: 'string', default: '—', notes: 'Object prefix / folder, e.g. hl7/inbound/.' },
                        { name: 'file_pattern', type: 'string', default: '*.hl7', notes: 'Glob pattern to filter object names.' },
                        { name: 'after_processing', type: 'enum', default: 'move', notes: 'delete | move | nothing.' },
                        { name: 'archive_bucket', type: 'string', default: '—', notes: 'GCS bucket to copy processed objects to.' },
                        { name: 'archive_prefix', type: 'string', default: '—', notes: 'Prefix in the archive bucket.' },
                        { name: 'max_objects_per_poll', type: 'integer', default: '100', notes: 'Max objects per cron run.' }
                    ],
                    example: { connectorType: 'gcs_inbound', config: { bucket_name: 'ehr-hl7-bucket', credentials_json: '<paste GCP service account JSON here>', prefix: 'inbound/', file_pattern: '*.hl7', after_processing: 'move', archive_bucket: 'ehr-hl7-archive', max_objects_per_poll: 100 } }
                }
            ],
            parameters: [
                { name: 'connectorType', type: 'string', required: true, description: 'The listener type. tcp_mllp is the primary HL7 transport (also accepted as tcp_mllp_inbound). See Connector Type Reference section below for all supported types.' },
                { name: 'config', type: 'object', required: true, description: 'Connector-specific settings driven by the connector type. See sub-fields below.' },
                { name: 'config.host', type: 'string', required: false, description: 'Bind address for TCP/MLLP (default: 0.0.0.0 — all interfaces).' },
                { name: 'config.port', type: 'number', required: false, description: 'TCP port to listen on (default: 2575 — standard MLLP port).' },
                { name: 'config.tls_enabled', type: 'boolean', required: false, description: 'Enable TLS 1.2/1.3. Requires tls_cert_file and tls_key_file.' },
                { name: 'config.max_connections', type: 'number', required: false, description: 'Maximum simultaneous client connections (default: 100).' },
                { name: 'config.ack', type: 'object', required: false, description: 'ACK/NACK configuration for TCP/MLLP Inbound. Controls how the connector acknowledges received HL7 messages.' },
                { name: 'config.ack.mode', type: 'string', required: false, description: '"immediate" — send AA as soon as the message is queued (default). "none" — do not send any ACK (sender must not expect a response).' },
                { name: 'config.ack.on_error', type: 'string', required: false, description: '"suppress" — always send AA even on errors, handle failures internally (default). "nack" — send AE so the sender can retry when the queue is full or a critical error occurs.' },
                { name: 'config.ack.sending_app', type: 'string', required: false, description: 'MSH-3 in the generated ACK message (default: "ezHealthKonnect").' },
                { name: 'config.ack.sending_facility', type: 'string', required: false, description: 'MSH-4 in the generated ACK message (default: "EHK").' },
                { name: 'config.ack.text_success', type: 'string', required: false, description: 'MSA-3 text when sending AA (default: "Message received successfully").' },
                { name: 'config.ack.text_error', type: 'string', required: false, description: 'MSA-3 text when sending AE or AR (default: "Message processing error").' },
                { name: 'config.ack.script', type: 'string', required: false, description: 'Advanced: JavaScript function that fully overrides ACK logic. Must define buildACK(msg) returning { ackCode, textMessage }. Valid ackCode values: AA, AE, AR. Available on msg: controlID, messageType, sendingApp, sendingFacility, raw, defaultCode, defaultText. Errors fall back to the default ACK.' },
                { name: 'timeoutMs', type: 'number', required: false, description: 'Maximum wait time for data fetch in milliseconds (default: 30000). Not used for long-lived TCP listeners.' }
            ],
            bestPractices: [
                {
                    practice: 'Error Handling — configure per-step error handling on the connector.inbound step',
                    reason: 'If the inbound step itself fails (e.g. port already in use, bad config), the pipeline will not start. Setting onError: "suppress" lets the engine log the failure and continue trying to activate other steps.',
                    example: 'In the step properties, open "Error Handling & Retry" → set On Error = suppress, Default Value = {} to prevent pipeline abort on transient startup failures.'
                },
                {
                    practice: 'Retry — add retry config for pull-mode connectors (SFTP, S3, DB)',
                    reason: 'Pull connectors run on a schedule. If the remote server is temporarily unreachable, retry logic can transparently retry 2–3 times within the same poll window before failing.',
                    example: 'Error Handling & Retry → Enable Retry = on, Max Retries = 3, Delay = 5000 ms, Backoff Multiplier = 2 (exponential: 5s, 10s, 20s).'
                },
                {
                    practice: 'Error Handling — use onError: nack for TCP/MLLP ACK on critical errors',
                    reason: 'When a message causes a fatal downstream error (e.g. DB unavailable), sending AE (application error) or AR lets the HL7 sender retry rather than silently dropping the message.',
                    example: 'ACK tab → On Error = nack. The sender receives AE and can retry after its own retry interval.'
                },
                {
                    practice: 'ACK mode = none for fire-and-forget senders',
                    reason: 'Some legacy HL7 systems do not read ACK responses and will block the socket waiting for data that never matters. Setting mode=none closes the send side immediately.',
                    example: 'ACK tab → ACK Mode = none. No ACK message is sent; the MLLP session ends after receiving the message.'
                }
            ]
        };
        docs['connector.outbound'] = {
            description: 'Sends data to external systems via configurable outbound connectors — TCP/MLLP, HTTP/REST, WebSocket, AS2, Direct Messaging (DirectTrust email), EDI X12 (SFTP), file writers, databases, message queues, and cloud storage. For request/response-shaped connectors (HTTP, TCP/MLLP, WebSocket), the destination\'s response is captured and surfaced into this step\'s _stepOutput automatically — a later pipeline step can read it via steps.<this_step_alias>.step_output.<field> (see "Reading the response" below) rather than the delivery being a one-way fire-and-forget.',
            useCases: [
                'Deliver transformed FHIR bundles to a REST endpoint',
                'Send HL7 messages to downstream systems via TCP/MLLP',
                'Send a request to a partner system and branch a later step on its response (e.g. an HTTP 4xx status, or a websocket reply payload)',
                'Deliver a signed+encrypted EDI document to a trading partner over AS2 or send a clinical document via DirectTrust email',
                'Write processed data to a database',
                'Archive messages to cloud storage (S3, Azure Blob, GCS)',
                'Publish events to Kafka or RabbitMQ'
            ],
            example: { connectorType: 'http_outbound', config: { url: 'https://fhir-server/api/Bundle', method: 'POST' }, contentField: 'transformed', contentType: 'application/fhir+json' },
            connectorTypeCards: [
                {
                    typeName: 'http_outbound', displayName: 'HTTP/HTTPS Endpoint', icon: '🌐', mode: 'push',
                    description: 'Delivers content via HTTP POST/PUT/PATCH to any REST endpoint. The response status, headers, and body are captured into this step\'s _stepOutput (response_status, response_body, response_headers).',
                    required: ['url'],
                    keyFields: [
                        { name: 'url', type: 'string', required: true, default: '—', notes: 'Destination URL.' },
                        { name: 'method', type: 'enum', default: 'POST', notes: 'POST | PUT | PATCH.' },
                        { name: 'content_type', type: 'string', default: 'application/json', notes: 'Request Content-Type header.' },
                        { name: 'timeout_seconds', type: 'integer', default: '30', notes: 'Request timeout.' },
                        { name: 'retry_attempts', type: 'integer', default: '3', notes: 'Retries on failure.' },
                        { name: 'authentication_type', type: 'enum', default: 'none', notes: 'none | basic_auth | bearer_token | api_key.' }
                    ],
                    example: { connectorType: 'http_outbound', config: { url: 'https://fhir-server/api/Bundle', method: 'POST', content_type: 'application/fhir+json', authentication_type: 'bearer_token', bearer_token: '••••' } }
                },
                {
                    typeName: 'tcp_mllp_outbound', displayName: 'TCP/MLLP (HL7 v2.x) Client', icon: '🔌', mode: 'push',
                    description: 'Sends HL7 v2.x messages to a downstream MLLP endpoint (Epic, Cerner, Meditech, etc.) and reads back the ACK/NACK response. The raw ACK text and its parsed code (AA/AE/AR) are both captured into this step\'s _stepOutput.',
                    required: ['host', 'port'],
                    keyFields: [
                        { name: 'host', type: 'string', required: true, default: '—', notes: 'Destination hostname or IP.' },
                        { name: 'port', type: 'integer', required: true, default: '2575', notes: 'Destination MLLP port.' },
                        { name: 'connection_mode', type: 'enum', default: 'persistent', notes: 'persistent (reused across sends) | per-message (fresh connection each time).' },
                        { name: 'enable_tls', type: 'boolean', default: 'false', notes: 'Require TLS.' },
                        { name: 'max_retries', type: 'integer', default: '0', notes: 'Retries within Send() itself, with retry_delay_ms between attempts.' }
                    ],
                    example: { connectorType: 'tcp_mllp_outbound', config: { host: 'downstream-his.internal', port: 2575, connection_mode: 'persistent' } }
                },
                {
                    typeName: 'websocket_outbound', displayName: 'WebSocket Client', icon: '🔌', mode: 'push',
                    description: 'Dials a remote websocket server, sends one frame, and by default reads back a response frame over the same connection. A read timeout is not treated as a delivery failure — the write already succeeded, and many websocket sends are fire-and-forget.',
                    required: ['url'],
                    keyFields: [
                        { name: 'url', type: 'string', required: true, default: '—', notes: 'ws:// or wss:// address.' },
                        { name: 'connection_mode', type: 'enum', default: 'persistent', notes: 'persistent | per-message.' },
                        { name: 'wait_for_response', type: 'boolean', default: 'true', notes: 'Read one response frame after sending.' },
                        { name: 'response_timeout_seconds', type: 'integer', default: '10', notes: 'How long to wait for a reply before giving up (still a successful send either way).' },
                        { name: 'headers', type: 'object', default: '—', notes: 'Custom handshake headers, e.g. Authorization.' }
                    ],
                    example: { connectorType: 'websocket_outbound', config: { url: 'wss://partner.example.com/ws', connection_mode: 'per-message', wait_for_response: true, response_timeout_seconds: 5 } }
                },
                {
                    typeName: 'as2_outbound', displayName: 'AS2 Sender', icon: '🔐', mode: 'push',
                    description: 'Signs+encrypts an EDI document as S/MIME (AS2/RFC 4130) and delivers it to a trading partner over HTTPS, verifying their signed synchronous MDN receipt in the same response.',
                    notes: 'Synchronous MDN only — async MDN is a named, not-yet-built item.',
                    required: ['partner_url', 'as2_from', 'as2_to', 'own_cert_pem', 'own_key_pem', 'partner_cert_pem'],
                    keyFields: [
                        { name: 'partner_url', type: 'string', required: true, default: '—', notes: 'Partner\'s AS2 endpoint URL.' },
                        { name: 'as2_from', type: 'string', required: true, default: '—', notes: 'Your own AS2 station ID.' },
                        { name: 'as2_to', type: 'string', required: true, default: '—', notes: 'Partner\'s AS2 ID.' },
                        { name: 'own_cert_pem', type: 'string (password)', required: true, default: '—', notes: 'Signs outgoing messages.' },
                        { name: 'own_key_pem', type: 'string (password)', required: true, default: '—', notes: 'Signs outgoing messages.' },
                        { name: 'partner_cert_pem', type: 'string', required: true, default: '—', notes: 'Encrypts outgoing messages and verifies the partner MDN.' },
                        { name: 'request_mdn', type: 'boolean', default: 'true', notes: 'Synchronous MDN only in this phase.' }
                    ],
                    example: { connectorType: 'as2_outbound', config: { partner_url: 'https://partner.example.com/as2', as2_from: 'EZHEALTHKONNECT', as2_to: 'PARTNERSTATION', own_cert_pem: '-----BEGIN CERTIFICATE-----...', own_key_pem: '-----BEGIN PRIVATE KEY-----...', partner_cert_pem: '-----BEGIN CERTIFICATE-----...' } }
                },
                {
                    typeName: 'direct_messaging_outbound', displayName: 'Direct Messaging Outbound', icon: '📧', mode: 'push',
                    description: 'Signs+encrypts a clinical document and delivers it as a DirectTrust email via SMTP, encrypted for the configured partner certificate. Reuses the same CMS sign/encrypt primitives as AS2, over an email transport.',
                    required: ['smtp_host', 'username', 'password', 'cert_content', 'private_key', 'partner_cert_pem', 'recipient_address'],
                    keyFields: [
                        { name: 'smtp_host', type: 'string', required: true, default: '—', notes: 'SMTP server hostname.' },
                        { name: 'smtp_port', type: 'integer', default: '587', notes: 'SMTP port.' },
                        { name: 'username', type: 'string', required: true, default: '—', notes: 'Sender Direct address.' },
                        { name: 'cert_content', type: 'string (password)', required: true, default: '—', notes: 'Own S/MIME certificate (PEM) — signs outgoing messages.' },
                        { name: 'private_key', type: 'string (password)', required: true, default: '—', notes: 'Own private key (PEM) — signs outgoing messages.' },
                        { name: 'partner_cert_pem', type: 'string', required: true, default: '—', notes: 'Encrypts outgoing messages to the recipient.' },
                        { name: 'recipient_address', type: 'string', required: true, default: '—', notes: 'Default recipient Direct address.' }
                    ],
                    example: { connectorType: 'direct_messaging_outbound', config: { smtp_host: 'smtp.directtrust-hisp.com', smtp_port: 587, username: 'provider@direct.hospital.org', cert_content: '-----BEGIN CERTIFICATE-----...', private_key: '-----BEGIN PRIVATE KEY-----...', partner_cert_pem: '-----BEGIN CERTIFICATE-----...', recipient_address: 'labs@direct.partner.org' } }
                },
                {
                    typeName: 'edi_x12_outbound', displayName: 'EDI X12 Outbound (SFTP)', icon: '💰', mode: 'push',
                    description: 'A transport-only "dumb byte shipper" that uploads a built X12 interchange (from an edi.build step) to a remote SFTP directory. All X12 envelope/business logic lives in edi.build, not this connector.',
                    notes: 'Transport is locked to SFTP — AS2 delivery uses the separate as2_outbound connector instead of a transport option here.',
                    required: ['transport', 'host', 'username'],
                    keyFields: [
                        { name: 'transport', type: 'enum', required: true, default: 'sftp', notes: 'Only "sftp" is implemented.' },
                        { name: 'host', type: 'string', required: true, default: '—', notes: 'SFTP server hostname or IP.' },
                        { name: 'username', type: 'string', required: true, default: '—', notes: 'SSH login username.' },
                        { name: 'remote_path', type: 'string', default: '/outgoing', notes: 'Remote directory to upload built EDI files to.' },
                        { name: 'filename_pattern', type: 'string', default: '—', notes: 'Placeholders: {message_id} {interface_id} {timestamp} {date} {time}.' }
                    ],
                    example: { connectorType: 'edi_x12_outbound', config: { transport: 'sftp', host: 'payer-sftp.example.com', username: 'edifeed', remote_path: '/outgoing', filename_pattern: '835_{timestamp}_{message_id}.edi' } }
                }
            ],
            parameters: [
                { name: 'connectorType', type: 'string', required: true, description: 'The type of outbound connector (e.g., http_outbound, tcp_mllp_outbound, websocket_outbound, as2_outbound, direct_messaging_outbound, edi_x12_outbound, file_writer). See the Connector Type Reference below.' },
                { name: 'config', type: 'object', required: true, description: 'Connector-specific configuration (host, port, URL, credentials, etc.) - fields are driven by the connector type config_schema' },
                { name: 'contentField', type: 'string', required: false, description: 'Which field from the pipeline data to send (default: transformed)' },
                { name: 'contentType', type: 'string', required: false, description: 'Content type of the outgoing data (default: application/json)' }
            ],
            stepOutput: {
                description: 'Every outbound connector writes delivery outcome fields into _stepOutput; connectors that get a response back over the same connection (HTTP, TCP/MLLP, WebSocket) additionally surface that response — generically, so a new connector type\'s response fields need no changes to this step\'s own code to become readable downstream.',
                fields: [
                    { name: '_stepOutput.success', type: 'boolean', description: 'True if the send operation itself completed without a network/connector error (does not mean the destination accepted the content — see delivery_success).' },
                    { name: '_stepOutput.delivery_success', type: 'boolean', description: 'The connector\'s own judgment of whether the destination actually accepted the message (e.g. false on an MLLP NACK even though the TCP write itself succeeded).' },
                    { name: '_stepOutput.acknowledgment', type: 'string', description: 'Raw acknowledgment text when the destination sends one (e.g. the full MSH+MSA ACK text for TCP/MLLP).' },
                    { name: '_stepOutput.response_status', type: 'number', description: 'HTTP outbound only: the response status code.' },
                    { name: '_stepOutput.response_body', type: 'string', description: 'HTTP outbound: the response body text. WebSocket outbound: the echoed/reply text frame, when wait_for_response is enabled and the partner replied within the timeout.' },
                    { name: '_stepOutput.ack_code', type: 'string', description: 'TCP/MLLP outbound only: the parsed acknowledgment code (AA = accept, AE = application error, AR = application reject) — parsed from the raw acknowledgment text so a later step can branch on it directly instead of re-parsing MSA itself.' },
                    { name: '_stepOutput.response_received', type: 'boolean', description: 'WebSocket outbound only: whether a response frame actually arrived within response_timeout_seconds. false is not a failure — many websocket sends are fire-and-forget with no inline reply; the message was still delivered.' },
                    { name: '_stepOutput.response_frame_type', type: 'string', description: 'WebSocket outbound only: "text" or "binary" — which of response_body / response_binary_base64 is populated.' },
                    { name: '_stepOutput.response_binary_base64', type: 'string', description: 'WebSocket outbound only: present instead of response_body when the partner replied with a binary frame — base64-encoded so it stays JSON-safe rather than being silently dropped or corrupted.' },
                ],
            },
            bestPractices: [
                {
                    practice: 'Read a captured response with steps.<alias>.step_output.<field>, not a bare field name',
                    reason: 'A later step\'s sourcePath/config must reference the SENDING step\'s own alias, not this step\'s output merged at the top level — the same addressing every other cross-step reference in this pipeline engine uses.',
                    example: 'A "Deliver to Partner" step aliased deliver_to_partner produces steps.deliver_to_partner.step_output.response_body — reference that exact path from an if_then_else condition or a later fhir.build sourcePath.',
                },
                {
                    practice: 'Branch on ack_code/response_status/delivery_success, not on the presence of _stepOutput.success alone',
                    reason: 'success only reflects whether the network operation itself completed — a TCP/MLLP NACK or an HTTP 4xx both still leave success=true, since the send itself worked; the DESTINATION\'s outcome lives in the more specific fields.',
                    example: 'if_then_else on steps.deliver_to_partner.step_output.ack_code === "AE" → route to a review queue, rather than checking success.',
                },
            ],
        };
    Object.keys(docs).forEach((stepType) => StepDocumentationRegistry.register(stepType, docs[stepType]));
})();
