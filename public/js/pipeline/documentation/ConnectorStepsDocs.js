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
                    description: 'Polls a remote SFTP server directory for new files on a configurable schedule (cron). After downloading each file it can delete, move, or rename it on the server. Supports both password and SSH private key authentication.',
                    notes: 'Requires a cron schedule on the interface. Set "After Processing" to delete or move to prevent reprocessing the same file.',
                    required: ['host', 'port', 'username', 'remote_directory'],
                    keyFields: [
                        { name: 'host', type: 'string', required: true, default: '—', notes: 'SFTP server hostname or IP.' },
                        { name: 'port', type: 'integer', required: true, default: '22', notes: 'Standard SSH/SFTP port.' },
                        { name: 'username', type: 'string', required: true, default: '—', notes: 'SSH login username.' },
                        { name: 'password', type: 'string (password)', default: '—', notes: 'SSH password. Leave blank if using private key.' },
                        { name: 'private_key_path', type: 'string', default: '—', notes: 'Path to SSH private key file (alternative to password).' },
                        { name: 'remote_directory', type: 'string', required: true, default: '—', notes: 'Remote path to poll for new files.' },
                        { name: 'file_pattern', type: 'string', default: '*.hl7', notes: 'Glob pattern to match files (e.g. *.hl7, ADT_*.txt).' },
                        { name: 'after_processing', type: 'enum', default: 'move', notes: 'delete | move | rename | nothing.' },
                        { name: 'archive_directory', type: 'string', default: '—', notes: 'Remote path to move processed files to.' },
                        { name: 'max_files_per_poll', type: 'integer', default: '100', notes: 'Cap per cron run to avoid overload.' }
                    ],
                    example: { connectorType: 'sftp_inbound', config: { host: 'sftp.partner.org', port: 22, username: 'hl7feed', private_key_path: '/certs/sftp_key', remote_directory: '/outbound/hl7', file_pattern: '*.hl7', after_processing: 'move', archive_directory: '/processed/hl7', max_files_per_poll: 50 } }
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
            description: 'Sends data to external systems via configurable outbound connectors. Supports TCP/MLLP, HTTP/REST, file writers, databases, message queues, and cloud storage.',
            useCases: [
                'Deliver transformed FHIR bundles to a REST endpoint',
                'Send HL7 messages to downstream systems via TCP/MLLP',
                'Write processed data to a database',
                'Archive messages to cloud storage (S3, Azure Blob, GCS)',
                'Publish events to Kafka or RabbitMQ'
            ],
            example: { connectorType: 'http_outbound', config: { url: 'https://fhir-server/api/Bundle', method: 'POST' }, contentField: 'transformed', contentType: 'application/fhir+json' },
            parameters: [
                { name: 'connectorType', type: 'string', required: true, description: 'The type of outbound connector (e.g., http_outbound, tcp_mllp_outbound, file_writer)' },
                { name: 'config', type: 'object', required: true, description: 'Connector-specific configuration (host, port, URL, credentials, etc.) - fields are driven by the connector type config_schema' },
                { name: 'contentField', type: 'string', required: false, description: 'Which field from the pipeline data to send (default: transformed)' },
                { name: 'contentType', type: 'string', required: false, description: 'Content type of the outgoing data (default: application/json)' }
            ]
        };
    Object.keys(docs).forEach((stepType) => StepDocumentationRegistry.register(stepType, docs[stepType]));
})();
