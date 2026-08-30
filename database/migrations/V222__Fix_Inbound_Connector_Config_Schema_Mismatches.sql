-- V222: Fix inbound connector config_schema/parameter_groups mismatches
-- Applied: 2026-08-30
--
-- Same class of bug V216 fixed for 8 outbound connectors, audited here for the
-- inbound side (never previously checked). Real usage was checked directly
-- against interfaces.source_connectivity/target_connectivity (NOT the empty,
-- unused interface_connectivity table):
--
--   tcp_mllp_inbound   -> 116 real interfaces
--   http_fhir_inbound  ->  38 real interfaces
--   file_listener      ->   1 real interface
--   everything else    ->   0 real interfaces
--
-- For the 3 connectors with real usage, every renamed/removed field was
-- individually checked against all real stored configs before touching it:
--   - tcp_mllp_inbound: 0 of 116 use tls_cert_path/tls_key_path/authentication_method
--     (the 3 fields being renamed here). "host" (a pre-existing no-op -- the
--     Go listener always binds all interfaces regardless) is used by all 116
--     but is left in the schema unchanged, matching the V216 precedent of
--     leaving harmless no-ops alone rather than scope-creeping into new bind-
--     address support. "enable_authentication" (a second, redundant no-op --
--     Go only ever reads authentication_type) is used by 2 of 116 and IS
--     removed this time, since keeping it next to the newly-corrected
--     authentication_type would actively mislead a user into thinking it's a
--     master toggle. Note: TCP/MLLP inbound's authentication_type is validated
--     at config time but never actually enforced against incoming connections
--     anywhere in the connection-accept code -- a real, separate gap, out of
--     scope for a schema-naming fix and not touched here.
--   - http_fhir_inbound: only ADDING two fields (tls, allowedIPs) that the Go
--     code already supports but the schema never exposed -- purely additive,
--     cannot affect any of the 38 real interfaces.
--   - file_listener: the 1 real interface uses "file_encoding" (the old,
--     wrong name). Rather than a plain rename, services/connectors/
--     file_listener.go's Initialize() was updated to accept "file_encoding" as
--     a fallback alongside the corrected "encoding" -- so this migration is
--     safe even though real data uses the old key.
--
-- "http_rest" is a separate connectivity_types row that resolves to the exact
-- same Go connector as http_fhir_inbound (HTTPFHIRInboundConnector) but had a
-- completely different, unrelated schema (endpoint_path, http_methods, etc.)
-- that matched neither the code nor http_fhir_inbound's own schema. Given the
-- identical underlying implementation, it now gets the identical corrected
-- schema.
--
-- Also fixed: several after_processing enum lists offered "archive" for
-- mysql_inbound/postgresql_inbound/sqlserver_inbound even though all three
-- share database_base.go's SharedAfterProcess helper, whose switch statement
-- has no "archive" case at all -- selecting it would have produced a real
-- runtime error every poll cycle (not a silent no-op). Removed from all three
-- enums; oracle_inbound's enum was already correct.
--
-- Widely-shared gap found across mysql/oracle/postgresql/sqlserver inbound:
-- "table_name" was missing from the schema entirely on all four, despite full
-- Go support -- the only way to configure a simple "poll this table" setup via
-- the UI was to hand-write a raw SQL query every time. Added to all four.
--
-- Per-connector fixes (see the audit summary given to the user for the full
-- narrative):
--   aws_s3_inbound:      bucket_name->bucket, max_objects_per_poll->max_keys,
--                        after_processing enum corrected to nothing|delete|archive
--                        (was delete|move|tag|nothing -- none of which except
--                        delete/nothing are real), removed dead
--                        authentication_method, added session_token/endpoint/
--                        force_path_style/archive_prefix/polling_interval/
--                        max_file_size_mb
--   file_listener:       file_encoding->encoding (Go now also accepts old name,
--                        see above), archive_directory->archive_path, added
--                        recursive/polling_interval/create_dirs
--   http_fhir_inbound:   added tls{enabled,certFile,keyFile}, allowedIPs
--   http_rest:           replaced entirely with http_fhir_inbound's schema
--                        (same underlying connector, was completely wrong)
--   kafka_inbound:       bootstrap_servers->brokers, group_id->consumer_group,
--                        removed dead security_protocol/client_id/
--                        max_poll_records/enable_auto_commit/
--                        auto_commit_interval_ms, added real
--                        session_timeout/heartbeat_interval/max_bytes/
--                        sasl_enabled/tls_enabled/tls_skip_verify
--   mongodb_inbound:     connection_uri->connection_string, query_filter->query,
--                        update_field->processed_flag_field,
--                        update_value->processed_flag_value,
--                        connection_timeout(ms)->connect_timeout(seconds, unit
--                        change too), max_documents_per_poll->max_records,
--                        removed "move_collection" from after_processing enum
--                        and archive_collection field (Go's afterProcess never
--                        implemented that mode), added watch_changes/
--                        operation_timeout
--   mysql_inbound:       enable_tls(bool)->ssl_mode(string),
--                        update_column->processed_flag_col,
--                        max_records_per_poll->max_records, added table_name/
--                        order_by/processed_flag_val/ssl_cert_path/
--                        max_open_conns/max_idle_conns
--   oracle_inbound:      enable_ssl(bool)->ssl_mode(string),
--                        service_name->database (Go's Database field IS the
--                        Oracle service name -- this was the most severe
--                        single-field bug found: Oracle inbound could not be
--                        configured via the UI at all), max_records_per_poll->
--                        max_records, added table_name/order_by/
--                        processed_flag_col/processed_flag_val/max_open_conns/
--                        max_idle_conns
--   postgresql_inbound:  update_column->processed_flag_col,
--                        update_value->processed_flag_val, removed dead
--                        connection_timeout field, max_records_per_poll->
--                        max_records, added table_name/order_by/ssl_cert_path/
--                        max_open_conns/max_idle_conns (ssl_mode was already correct)
--   rabbitmq_inbound:    queue_name->queue (required field, was completely
--                        broken), enable_tls->use_tls, exchange_name->exchange,
--                        durable_queue->queue_durable, removed dead
--                        connection_timeout, added queue_exclusive/tls_skip_verify
--   redis_inbound:       removed "mode" (Go has no such field -- it auto-
--                        detects Pub/Sub vs Stream purely from whether "stream"
--                        is set), removed dead key_name/connection_pool_size,
--                        database->db_number, enable_tls->use_tls,
--                        blocking_timeout->read_timeout (Go's real unit is
--                        milliseconds, not seconds -- schema previously implied
--                        seconds), added tls_skip_verify
--   sftp_inbound:        remote_directory->remote_dir,
--                        archive_directory->archive_dir,
--                        connection_timeout->connect_timeout,
--                        max_files_per_poll->max_files_per_run, removed
--                        private_key_path (Go rejects filesystem key paths
--                        entirely -- "private_key_path" never mapped to any
--                        real capability) and private_key_passphrase (Go's key
--                        auth has no passphrase support), added the REAL,
--                        already-working key-auth mechanism: auth_type
--                        (password|key) + key_content (paste PEM text) -- this
--                        closes the "SFTP key login" gap from the smaller-items
--                        list with a schema fix alone, no new Go code needed.
--                        Also added poll_interval_sec and read_timeout (per-
--                        file download timeout), both previously unexposed.
--   sqlserver_inbound:   encrypt_connection(bool)->ssl_mode(string), removed
--                        authentication_type (no Windows-auth code path exists
--                        at all -- also corrected the false
--                        "supports_windows_auth" capability claim in
--                        services/connectors/sqlserver_inbound.go) and
--                        trust_server_certificate (hardcoded internally, never
--                        read from config), max_records_per_poll->max_records,
--                        added table_name/order_by/processed_flag_col/
--                        processed_flag_val/max_open_conns/max_idle_conns
--   tcp_mllp:            tls_cert_path->certificate_file,
--                        tls_key_path->key_file,
--                        authentication_method->authentication_type (also
--                        corrected enum from basic|token|ip_whitelist to the
--                        real none|basic|token), removed enable_authentication
--                        (redundant no-op, see above), added username/password
--                        (previously no way to enter Basic auth credentials at
--                        all despite Validate() requiring them) and tls_version.
--                        "host" left unchanged (pre-existing no-op, see above).

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["bucket","region"],"properties":{"bucket":{"type":"string","title":"S3 Bucket Name"},"region":{"type":"string","title":"AWS Region","default":"us-east-1"},"prefix":{"type":"string","title":"Object Prefix/Folder"},"file_pattern":{"type":"string","title":"File Pattern","default":"*.hl7"},"access_key_id":{"type":"string","title":"Access Key ID"},"secret_access_key":{"type":"string","title":"Secret Access Key","format":"password"},"session_token":{"type":"string","title":"Session Token","format":"password","description":"For temporary credentials"},"endpoint":{"type":"string","title":"Custom Endpoint","description":"For S3-compatible storage (e.g. LocalStack, MinIO)"},"force_path_style":{"type":"boolean","title":"Use Path-Style URLs","default":false},"after_processing":{"enum":["nothing","delete","archive"],"type":"string","title":"After Processing","default":"nothing"},"archive_prefix":{"type":"string","title":"Archive Prefix","default":"processed/","description":"Used when After Processing is archive"},"polling_interval":{"type":"integer","title":"Polling Interval (seconds)","default":60},"max_keys":{"type":"integer","title":"Max Objects Per Poll","default":100},"max_file_size_mb":{"type":"integer","title":"Max File Size (MB)","default":50}}}'::jsonb,
    parameter_groups = '{"basic":["bucket","region","prefix","file_pattern"],"authentication":["access_key_id","secret_access_key","session_token"],"advanced":["endpoint","force_path_style"],"processing":["after_processing","archive_prefix","polling_interval","max_keys","max_file_size_mb"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'aws_s3_inbound';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["directory_path","file_pattern"],"properties":{"directory_path":{"type":"string","title":"Monitor Directory"},"file_pattern":{"type":"string","title":"File Pattern","default":"*.hl7"},"recursive":{"type":"boolean","title":"Scan Subdirectories","default":false},"polling_interval":{"type":"integer","title":"Polling Interval (seconds)","default":10},"encoding":{"type":"string","title":"File Encoding","default":"UTF-8"},"after_processing":{"enum":["delete","move","archive","nothing"],"type":"string","title":"After Processing","default":"move"},"archive_path":{"type":"string","title":"Archive Directory"},"create_dirs":{"type":"boolean","title":"Auto-Create Directory If Missing","default":false}}}'::jsonb,
    parameter_groups = '{"basic":["directory_path","file_pattern","recursive"],"advanced":["encoding","polling_interval","create_dirs"],"processing":["after_processing","archive_path"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'file_listener';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["port"],"properties":{"port":{"type":"number","title":"Listen Port","maximum":65535,"minimum":1,"description":"TCP port to listen on (e.g. 7250)"},"apiKey":{"type":"string","title":"API Key","format":"password"},"authType":{"enum":["none","basic","bearer","api_key"],"type":"string","title":"Authentication Type","default":"none"},"basePath":{"type":"string","title":"Base Path","default":"/fhir/r4","description":"URL prefix for all FHIR endpoints (e.g. /fhir/r4)"},"password":{"type":"string","title":"Password","format":"password"},"username":{"type":"string","title":"Username"},"bundleMode":{"enum":["bundle_as_unit","bundle_unwrap"],"type":"string","title":"Bundle Mode","default":"bundle_as_unit","description":"bundle_as_unit: entire Bundle is one message. bundle_unwrap: each Bundle entry becomes its own message routed individually."},"enableCORS":{"type":"boolean","title":"Enable CORS","default":true,"description":"Add CORS headers to allow browser / cross-origin callers"},"bearerToken":{"type":"string","title":"Bearer Token","format":"password"},"fhirVersion":{"enum":["R4","R5","STU3"],"type":"string","title":"FHIR Version","default":"R4"},"apiKeyHeader":{"type":"string","title":"API Key Header Name","default":"X-API-Key","description":"HTTP header the caller sends the API key in"},"maxBodySizeMB":{"type":"number","title":"Max Body Size (MB)","default":10,"maximum":500,"minimum":1,"description":"Maximum accepted request body size"},"allowedMethods":{"type":"array","items":{"enum":["GET","POST","PUT","PATCH","DELETE"],"type":"string"},"title":"Allowed HTTP Methods","default":["GET","POST","PUT","PATCH","DELETE"],"description":"Restrict which HTTP methods this receiver accepts. Leave empty to allow all FHIR methods."},"requestTimeoutSeconds":{"type":"number","title":"Request Timeout (seconds)","default":30,"maximum":300,"minimum":1,"description":"Server read and write timeout per HTTP request"},"tls":{"type":"object","title":"TLS/SSL","properties":{"enabled":{"type":"boolean","title":"Enable TLS","default":false},"certFile":{"type":"string","title":"Certificate File Path"},"keyFile":{"type":"string","title":"Private Key File Path"}}},"allowedIPs":{"type":"array","items":{"type":"string"},"title":"Allowed IP Addresses/CIDR Ranges","description":"Leave empty to allow all IPs"}}}'::jsonb,
    parameter_groups = '{"basic":["port","basePath","fhirVersion","bundleMode","allowedMethods"],"advanced":["maxBodySizeMB","enableCORS","requestTimeoutSeconds","tls","allowedIPs"],"security":["authType","username","password","bearerToken","apiKey","apiKeyHeader"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'http_fhir_inbound';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["port"],"properties":{"port":{"type":"number","title":"Listen Port","maximum":65535,"minimum":1,"description":"TCP port to listen on (e.g. 7250)"},"apiKey":{"type":"string","title":"API Key","format":"password"},"authType":{"enum":["none","basic","bearer","api_key"],"type":"string","title":"Authentication Type","default":"none"},"basePath":{"type":"string","title":"Base Path","default":"/fhir/r4","description":"URL prefix for all FHIR endpoints (e.g. /fhir/r4)"},"password":{"type":"string","title":"Password","format":"password"},"username":{"type":"string","title":"Username"},"bundleMode":{"enum":["bundle_as_unit","bundle_unwrap"],"type":"string","title":"Bundle Mode","default":"bundle_as_unit"},"enableCORS":{"type":"boolean","title":"Enable CORS","default":true},"bearerToken":{"type":"string","title":"Bearer Token","format":"password"},"fhirVersion":{"enum":["R4","R5","STU3"],"type":"string","title":"FHIR Version","default":"R4"},"apiKeyHeader":{"type":"string","title":"API Key Header Name","default":"X-API-Key"},"maxBodySizeMB":{"type":"number","title":"Max Body Size (MB)","default":10,"maximum":500,"minimum":1},"allowedMethods":{"type":"array","items":{"enum":["GET","POST","PUT","PATCH","DELETE"],"type":"string"},"title":"Allowed HTTP Methods","default":["GET","POST","PUT","PATCH","DELETE"]},"requestTimeoutSeconds":{"type":"number","title":"Request Timeout (seconds)","default":30,"maximum":300,"minimum":1},"tls":{"type":"object","title":"TLS/SSL","properties":{"enabled":{"type":"boolean","title":"Enable TLS","default":false},"certFile":{"type":"string","title":"Certificate File Path"},"keyFile":{"type":"string","title":"Private Key File Path"}}},"allowedIPs":{"type":"array","items":{"type":"string"},"title":"Allowed IP Addresses/CIDR Ranges"}}}'::jsonb,
    parameter_groups = '{"basic":["port","basePath","fhirVersion","bundleMode","allowedMethods"],"advanced":["maxBodySizeMB","enableCORS","requestTimeoutSeconds","tls","allowedIPs"],"security":["authType","username","password","bearerToken","apiKey","apiKeyHeader"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'http_rest';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["brokers","topic","consumer_group"],"properties":{"brokers":{"type":"string","title":"Bootstrap Servers","default":"localhost:9092","description":"Comma-separated list (e.g., localhost:9092,localhost:9093)"},"topic":{"type":"string","title":"Topic Name"},"consumer_group":{"type":"string","title":"Consumer Group ID"},"auto_offset_reset":{"enum":["earliest","latest","none"],"type":"string","title":"Auto Offset Reset","default":"latest"},"session_timeout":{"type":"integer","title":"Session Timeout (seconds)","default":30},"heartbeat_interval":{"type":"integer","title":"Heartbeat Interval (seconds)","default":3},"max_bytes":{"type":"integer","title":"Max Bytes Per Fetch","default":1048576},"sasl_enabled":{"type":"boolean","title":"Enable SASL Authentication","default":false},"sasl_mechanism":{"enum":["PLAIN","SCRAM-SHA-256","SCRAM-SHA-512"],"type":"string","title":"SASL Mechanism","default":"PLAIN"},"sasl_username":{"type":"string","title":"SASL Username"},"sasl_password":{"type":"string","title":"SASL Password","format":"password"},"tls_enabled":{"type":"boolean","title":"Enable TLS","default":false},"tls_skip_verify":{"type":"boolean","title":"Skip TLS Certificate Verification","default":false}}}'::jsonb,
    parameter_groups = '{"basic":["brokers","topic","consumer_group"],"consumer":["auto_offset_reset","session_timeout","heartbeat_interval","max_bytes"],"security":["sasl_enabled","sasl_mechanism","sasl_username","sasl_password","tls_enabled","tls_skip_verify"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'kafka_inbound';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["connection_string","database","collection"],"properties":{"connection_string":{"type":"string","title":"MongoDB Connection URI","default":"mongodb://localhost:27017","description":"mongodb://user:password@host:port or mongodb+srv://..."},"database":{"type":"string","title":"Database Name"},"collection":{"type":"string","title":"Collection Name"},"query":{"type":"string","title":"Query Filter (JSON)","default":"{}","description":"MongoDB query filter in JSON format"},"incremental_field":{"type":"string","title":"Incremental Field","description":"Field to track last polled value (e.g., _id, updatedAt)"},"watch_changes":{"type":"boolean","title":"Use Change Streams (Real-Time)","default":false,"description":"Requires a MongoDB replica set or Atlas cluster"},"polling_interval":{"type":"integer","title":"Polling Interval (seconds)","default":60},"max_records":{"type":"integer","title":"Max Documents Per Poll","default":100},"after_processing":{"enum":["nothing","delete","update_flag"],"type":"string","title":"After Processing","default":"nothing"},"processed_flag_field":{"type":"string","title":"Processed Flag Field","default":"processed"},"processed_flag_value":{"type":"string","title":"Processed Flag Value","default":"true"},"connect_timeout":{"type":"integer","title":"Connect Timeout (seconds)","default":10},"operation_timeout":{"type":"integer","title":"Operation Timeout (seconds)","default":30}}}'::jsonb,
    parameter_groups = '{"basic":["connection_string","database","collection"],"query":["query","incremental_field","max_records","watch_changes"],"processing":["after_processing","processed_flag_field","processed_flag_value"],"advanced":["polling_interval","connect_timeout","operation_timeout"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'mongodb_inbound';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["host","port","database"],"properties":{"host":{"type":"string","title":"Database Host","default":"localhost"},"port":{"type":"integer","title":"Database Port","default":3306,"minimum":1,"maximum":65535},"database":{"type":"string","title":"Database Name"},"username":{"type":"string","title":"Username"},"password":{"type":"string","title":"Password","format":"password"},"table_name":{"type":"string","title":"Table Name","description":"Table to poll (required if Custom Query is not set)"},"query":{"type":"string","title":"Custom SQL Query","description":"Overrides Table Name -- full custom SELECT statement"},"incremental_column":{"type":"string","title":"Incremental Column"},"incremental_type":{"enum":["integer","timestamp","datetime"],"type":"string","title":"Incremental Type","default":"integer"},"order_by":{"type":"string","title":"Order By Clause"},"polling_interval":{"type":"integer","title":"Polling Interval (seconds)","default":60},"max_records":{"type":"integer","title":"Max Records Per Poll","default":100},"after_processing":{"enum":["nothing","delete","update_flag"],"type":"string","title":"After Processing","default":"update_flag"},"processed_flag_col":{"type":"string","title":"Processed Flag Column"},"processed_flag_val":{"type":"string","title":"Processed Flag Value"},"ssl_mode":{"enum":["disable","require","verify-ca","verify-full"],"type":"string","title":"SSL Mode","default":"disable"},"max_open_conns":{"type":"integer","title":"Max Open Connections","default":10},"max_idle_conns":{"type":"integer","title":"Max Idle Connections","default":5}}}'::jsonb,
    parameter_groups = '{"basic":["host","port","database","username","password"],"query":["table_name","query","incremental_column","incremental_type","order_by","max_records"],"advanced":["polling_interval","ssl_mode","max_open_conns","max_idle_conns"],"processing":["after_processing","processed_flag_col","processed_flag_val"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'mysql_inbound';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["host","port","database"],"properties":{"host":{"type":"string","title":"Database Host","default":"localhost"},"port":{"type":"integer","title":"Database Port","default":1521},"database":{"type":"string","title":"Service Name","description":"Oracle service name or SID"},"username":{"type":"string","title":"Username"},"password":{"type":"string","title":"Password","format":"password"},"table_name":{"type":"string","title":"Table Name","description":"Table to poll (required if Custom Query is not set)"},"query":{"type":"string","title":"Custom SQL Query"},"incremental_column":{"type":"string","title":"Incremental Column"},"incremental_type":{"enum":["integer","timestamp","datetime"],"type":"string","title":"Incremental Type","default":"integer"},"order_by":{"type":"string","title":"Order By Clause"},"polling_interval":{"type":"integer","title":"Polling Interval (seconds)","default":60},"max_records":{"type":"integer","title":"Max Records Per Poll","default":100},"after_processing":{"enum":["update_flag","delete","nothing"],"type":"string","title":"After Processing","default":"update_flag"},"processed_flag_col":{"type":"string","title":"Processed Flag Column"},"processed_flag_val":{"type":"string","title":"Processed Flag Value"},"ssl_mode":{"enum":["disable","require"],"type":"string","title":"SSL Mode","default":"disable"},"max_open_conns":{"type":"integer","title":"Max Open Connections","default":10},"max_idle_conns":{"type":"integer","title":"Max Idle Connections","default":5}}}'::jsonb,
    parameter_groups = '{"basic":["host","port","database","username","password"],"query":["table_name","query","incremental_column","incremental_type","order_by","max_records"],"advanced":["polling_interval","ssl_mode","max_open_conns","max_idle_conns"],"processing":["after_processing","processed_flag_col","processed_flag_val"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'oracle_inbound';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["host","port","database","username"],"properties":{"host":{"type":"string","title":"Database Host","default":"localhost"},"port":{"type":"integer","title":"Database Port","default":5432,"minimum":1,"maximum":65535},"database":{"type":"string","title":"Database Name"},"username":{"type":"string","title":"Username"},"password":{"type":"string","title":"Password","format":"password"},"table_name":{"type":"string","title":"Table Name","description":"Table to poll (required if Custom Query is not set)"},"query":{"type":"string","title":"Custom SQL Query","description":"SELECT query to fetch records (use WHERE clause for incremental polling)"},"incremental_column":{"type":"string","title":"Incremental Column","description":"Column to track last polled value (e.g., id, updated_at)"},"incremental_type":{"enum":["integer","bigint","timestamp","datetime"],"type":"string","title":"Incremental Type","default":"integer"},"order_by":{"type":"string","title":"Order By Clause"},"polling_interval":{"type":"integer","title":"Polling Interval (seconds)","default":60},"max_records":{"type":"integer","title":"Max Records Per Poll","default":100,"maximum":10000,"minimum":1},"after_processing":{"enum":["nothing","delete","update_flag"],"type":"string","title":"After Processing","default":"update_flag"},"processed_flag_col":{"type":"string","title":"Processed Flag Column"},"processed_flag_val":{"type":"string","title":"Processed Flag Value","default":"processed"},"ssl_mode":{"enum":["disable","require","verify-ca","verify-full"],"type":"string","title":"SSL Mode","default":"disable"},"ssl_cert_path":{"type":"string","title":"SSL Root Certificate Path"},"max_open_conns":{"type":"integer","title":"Max Open Connections","default":10},"max_idle_conns":{"type":"integer","title":"Max Idle Connections","default":5}}}'::jsonb,
    parameter_groups = '{"basic":["host","port","database","username","password"],"query":["table_name","query","incremental_column","incremental_type","order_by","max_records"],"advanced":["polling_interval","ssl_mode","ssl_cert_path","max_open_conns","max_idle_conns"],"processing":["after_processing","processed_flag_col","processed_flag_val"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'postgresql_inbound';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["host","port","queue"],"properties":{"host":{"type":"string","title":"RabbitMQ Host","default":"localhost"},"port":{"type":"integer","title":"Port","default":5672},"username":{"type":"string","title":"Username","default":"guest"},"password":{"type":"string","title":"Password","format":"password","default":"guest"},"vhost":{"type":"string","title":"Virtual Host","default":"/"},"queue":{"type":"string","title":"Queue Name"},"exchange":{"type":"string","title":"Exchange Name"},"routing_key":{"type":"string","title":"Routing Key"},"prefetch_count":{"type":"integer","title":"Prefetch Count","default":10,"description":"Number of messages to prefetch"},"auto_ack":{"type":"boolean","title":"Auto Acknowledge","default":false},"queue_durable":{"type":"boolean","title":"Durable Queue","default":true},"queue_exclusive":{"type":"boolean","title":"Exclusive Queue","default":false},"use_tls":{"type":"boolean","title":"Enable TLS/SSL (amqps://)","default":false},"tls_skip_verify":{"type":"boolean","title":"Skip TLS Certificate Verification","default":false}}}'::jsonb,
    parameter_groups = '{"basic":["host","port","username","password","vhost"],"queue":["queue","exchange","routing_key"],"advanced":["use_tls","tls_skip_verify"],"consumer":["prefetch_count","auto_ack","queue_durable","queue_exclusive"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'rabbitmq_inbound';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["host","port"],"properties":{"host":{"type":"string","title":"Redis Host","default":"localhost"},"port":{"type":"integer","title":"Redis Port","default":6379},"password":{"type":"string","title":"Password","format":"password"},"db_number":{"type":"integer","title":"Database Number","default":0,"maximum":15,"minimum":0},"channel":{"type":"string","title":"Pub/Sub Channel","description":"Set this for Pub/Sub mode"},"stream":{"type":"string","title":"Stream Key","description":"Set this for Stream mode (takes priority over Pub/Sub channel)"},"consumer_group":{"type":"string","title":"Consumer Group (for streams)","default":"ehk-consumer-group"},"consumer_name":{"type":"string","title":"Consumer Name (for streams)"},"read_timeout":{"type":"integer","title":"Block Timeout (ms)","default":2000},"max_pending_ack":{"type":"integer","title":"Max Messages Per Read (for streams)","default":10},"use_tls":{"type":"boolean","title":"Enable TLS/SSL","default":false},"tls_skip_verify":{"type":"boolean","title":"Skip TLS Certificate Verification","default":false}}}'::jsonb,
    parameter_groups = '{"basic":["host","port","password","db_number"],"consumer":["channel","stream","consumer_group","consumer_name"],"advanced":["read_timeout","max_pending_ack","use_tls","tls_skip_verify"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'redis_inbound';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["host","username","remote_dir"],"properties":{"host":{"type":"string","title":"SFTP Host"},"port":{"type":"integer","title":"SFTP Port","default":22},"username":{"type":"string","title":"Username"},"auth_type":{"enum":["password","key"],"type":"string","title":"Authentication Type","default":"password"},"password":{"type":"string","title":"Password","format":"password"},"key_content":{"type":"string","title":"Private Key (paste PEM content)","format":"password","description":"Used when Authentication Type is key -- paste the full PEM private key text"},"remote_dir":{"type":"string","title":"Remote Directory","default":"/inbox"},"file_pattern":{"type":"string","title":"File Pattern","default":"*.hl7"},"poll_interval_sec":{"type":"integer","title":"Polling Interval (seconds)","default":30},"after_processing":{"enum":["delete","archive","none"],"type":"string","title":"After Processing","default":"archive"},"archive_dir":{"type":"string","title":"Archive Directory","default":"/inbox/processed"},"max_files_per_run":{"type":"integer","title":"Max Files Per Poll","default":100},"connect_timeout":{"type":"integer","title":"Connect Timeout (seconds)","default":10},"read_timeout":{"type":"integer","title":"File Download Timeout (seconds)","default":60}}}'::jsonb,
    parameter_groups = '{"basic":["host","port","username"],"authentication":["auth_type","password","key_content"],"file":["remote_dir","file_pattern","poll_interval_sec","max_files_per_run"],"processing":["after_processing","archive_dir"],"advanced":["connect_timeout","read_timeout"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'sftp_inbound';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["host","port","database"],"properties":{"host":{"type":"string","title":"Database Host","default":"localhost"},"port":{"type":"integer","title":"Database Port","default":1433},"database":{"type":"string","title":"Database Name"},"username":{"type":"string","title":"Username"},"password":{"type":"string","title":"Password","format":"password"},"table_name":{"type":"string","title":"Table Name","description":"Table to poll (required if Custom Query is not set)"},"query":{"type":"string","title":"Custom SQL Query"},"incremental_column":{"type":"string","title":"Incremental Column"},"incremental_type":{"enum":["integer","timestamp","datetime"],"type":"string","title":"Incremental Type","default":"integer"},"order_by":{"type":"string","title":"Order By Clause"},"polling_interval":{"type":"integer","title":"Polling Interval (seconds)","default":60},"max_records":{"type":"integer","title":"Max Records Per Poll","default":100},"after_processing":{"enum":["nothing","delete","update_flag"],"type":"string","title":"After Processing","default":"update_flag"},"processed_flag_col":{"type":"string","title":"Processed Flag Column"},"processed_flag_val":{"type":"string","title":"Processed Flag Value"},"ssl_mode":{"enum":["disable","require","verify-full"],"type":"string","title":"SSL Mode","default":"disable"},"max_open_conns":{"type":"integer","title":"Max Open Connections","default":10},"max_idle_conns":{"type":"integer","title":"Max Idle Connections","default":5}}}'::jsonb,
    parameter_groups = '{"basic":["host","port","database","username","password"],"query":["table_name","query","incremental_column","incremental_type","order_by","max_records"],"advanced":["polling_interval","ssl_mode","max_open_conns","max_idle_conns"],"processing":["after_processing","processed_flag_col","processed_flag_val"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'sqlserver_inbound';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["port"],"properties":{"host":{"type":"string","title":"Bind Address","default":"0.0.0.0","description":"Note: the current listener always binds all interfaces regardless of this value"},"port":{"type":"integer","title":"Listen Port","default":2575,"maximum":65535,"minimum":1024},"max_connections":{"type":"integer","title":"Max Connections","default":10},"read_timeout_seconds":{"type":"integer","title":"Read Timeout (seconds)","default":300},"write_timeout_seconds":{"type":"integer","title":"Write Timeout (seconds)","default":300},"enable_tls":{"type":"boolean","title":"Enable TLS/SSL","default":false},"tls_version":{"enum":["1.2","1.3"],"type":"string","title":"Minimum TLS Version","default":"1.2"},"certificate_file":{"type":"string","title":"TLS Certificate Path"},"key_file":{"type":"string","title":"TLS Private Key Path"},"authentication_type":{"enum":["none","basic","token"],"type":"string","title":"Authentication Type","default":"none"},"username":{"type":"string","title":"Username","description":"Used when Authentication Type is basic"},"password":{"type":"string","title":"Password","format":"password","description":"Used when Authentication Type is basic"}}}'::jsonb,
    parameter_groups = '{"basic":["port","host"],"advanced":["max_connections","read_timeout_seconds","write_timeout_seconds"],"security":["enable_tls","tls_version","certificate_file","key_file","authentication_type","username","password"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'tcp_mllp';
