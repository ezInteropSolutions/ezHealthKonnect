-- V223: Add Oracle Windows auth, SFTP outbound filename templating, file_writer key fixes
-- Applied: 2026-08-30
--
-- Closes the remaining "smaller named gaps" items:
--
-- 1) Oracle Windows/OS Authentication (oracle_inbound, oracle_outbound)
--    Previously a schema/capability gap with no backing code at all. Verified
--    the existing driver (github.com/sijms/go-ora/v2, already used by both
--    connectors) genuinely supports Windows OS authentication via
--    AUTH TYPE=OS + OS USER/OS PASS/DOMAIN/AUTH SERV=NTS (its own documented
--    mechanism, confirmed against the driver's README and godoc -- not
--    invented here). services/connectors/oracle_inbound.go and
--    oracle_outbound.go both gained a buildWindowsAuthDSN() using
--    go_ora.BuildUrl() with these exact option keys, gated on
--    auth_type="windows". Zero real usage of either connector confirmed
--    before this session's V222 fix, so purely additive fields here carry no
--    compatibility risk. UNVERIFIED against a real domain-joined Oracle
--    server -- no such infrastructure available in this environment; only
--    the DSN-construction logic itself is unit-tested.
--
-- 2) SFTP outbound filename templating (sftp_outbound)
--    "filename_pattern" already existed in this connector's schema (implying
--    templating worked) but the Go connector never read it at all -- selecting
--    it silently did nothing, matching the same class of bug found across
--    every other connector this session. services/connectors/sftp_outbound.go
--    now reads filename_pattern using the identical placeholder convention as
--    aws_s3_outbound.go/azure_blob_outbound.go (including support for
--    subdirectories, e.g. "{interface_id}/{message_id}.hl7" -- backed by a new
--    mkdir -p safety step before every upload, since SCP itself never creates
--    directories). The legacy filename_field/filename_prefix/file_extension
--    behavior is preserved in the Go code as a fallback when filename_pattern
--    is unset, but removed from the schema (going forward, filename_pattern
--    is the one documented way to control naming).
--    Also fixed while touching this row (found only once compared directly
--    against the real Go code, same as every other connector this session):
--      remote_directory -> remote_dir, connection_timeout -> connect_timeout,
--      removed private_key_path (Go rejects filesystem key paths entirely --
--      never mapped to any real capability) and private_key_passphrase (no
--      passphrase support in the key-auth code path), removed dead
--      file_permissions (SCP header hardcodes 0644, not configurable) and
--      create_directories/overwrite_existing (no such toggles exist in Go --
--      directories are now always created as needed via the new mkdir -p
--      step, and SCP uploads always overwrite). Added the REAL, already-
--      working key-auth mechanism: auth_type (password|key) + key_content
--      (paste PEM text) -- mirrors the exact same fix already applied to
--      sftp_inbound in V222. Added write_timeout (real Go field, was never
--      exposed). Confirmed zero real usage before this rewrite (same query
--      pattern as V222's audit).
--
-- 3) file_writer key fixes (file_writer, 35 real active pipeline steps)
--    Checked directly against all 35 active transformation_steps rows before
--    touching anything: 100% use the already-correct "directory_path" and
--    "filename_pattern" keys -- neither is touched here. The two genuinely
--    wrong keys (file_encoding -> encoding, create_subdirectories ->
--    create_subdirs) are confirmed used by ZERO of the 35 active rows, so
--    this is a safe direct fix; services/connectors/file_writer.go was also
--    given fallback support for both old key names anyway (defense in depth,
--    matching the same file's own established multi-name-fallback style),
--    so no real risk either way. NOTE: a separate, unrelated, currently-dead
--    code path (services/wizardConfigService.js, confirmed never imported by
--    any other file in the codebase) constructs an entirely different,
--    non-functional file_writer config shape (outputPath/fileNamePattern/
--    createDirectories) -- since it is provably unreachable dead code, not a
--    live bug, it was deliberately NOT touched as part of this fix; flagged
--    to the user as a separate cleanup decision.

-- Oracle: full replacement (existing correct fields from V222 carried
-- forward verbatim, plus the 4 new Windows-auth fields).
UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["host","port","database"],"properties":{"host":{"type":"string","title":"Database Host","default":"localhost"},"port":{"type":"integer","title":"Database Port","default":1521},"database":{"type":"string","title":"Service Name","description":"Oracle service name or SID"},"auth_type":{"enum":["password","windows"],"type":"string","title":"Authentication Type","default":"password"},"username":{"type":"string","title":"Username"},"password":{"type":"string","title":"Password","format":"password"},"os_user":{"type":"string","title":"Windows Username","description":"Used when Authentication Type is windows -- optional, driver uses the process logon user if omitted"},"os_password":{"type":"string","title":"Windows Password","format":"password","description":"Required when Authentication Type is windows"},"domain":{"type":"string","title":"Windows Domain","description":"Optional -- used when Authentication Type is windows"},"table_name":{"type":"string","title":"Table Name","description":"Table to poll (required if Custom Query is not set)"},"query":{"type":"string","title":"Custom SQL Query"},"incremental_column":{"type":"string","title":"Incremental Column"},"incremental_type":{"enum":["integer","timestamp","datetime"],"type":"string","title":"Incremental Type","default":"integer"},"order_by":{"type":"string","title":"Order By Clause"},"polling_interval":{"type":"integer","title":"Polling Interval (seconds)","default":60},"max_records":{"type":"integer","title":"Max Records Per Poll","default":100},"after_processing":{"enum":["update_flag","delete","nothing"],"type":"string","title":"After Processing","default":"update_flag"},"processed_flag_col":{"type":"string","title":"Processed Flag Column"},"processed_flag_val":{"type":"string","title":"Processed Flag Value"},"ssl_mode":{"enum":["disable","require"],"type":"string","title":"SSL Mode","default":"disable"},"max_open_conns":{"type":"integer","title":"Max Open Connections","default":10},"max_idle_conns":{"type":"integer","title":"Max Idle Connections","default":5}}}'::jsonb,
    parameter_groups = '{"basic":["host","port","database"],"authentication":["auth_type","username","password","os_user","os_password","domain"],"query":["table_name","query","incremental_column","incremental_type","order_by","max_records"],"advanced":["polling_interval","ssl_mode","max_open_conns","max_idle_conns"],"processing":["after_processing","processed_flag_col","processed_flag_val"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'oracle_inbound';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["host","port","database","table_name"],"properties":{"host":{"type":"string","title":"Database Host","default":"localhost"},"port":{"type":"integer","title":"Database Port","default":1521},"database":{"type":"string","title":"Service Name","description":"Oracle service name (used as the connect-string SERVICE_NAME)"},"auth_type":{"enum":["password","windows"],"type":"string","title":"Authentication Type","default":"password"},"username":{"type":"string","title":"Username"},"password":{"type":"string","title":"Password","format":"password"},"os_user":{"type":"string","title":"Windows Username","description":"Used when Authentication Type is windows -- optional, driver uses the process logon user if omitted"},"os_password":{"type":"string","title":"Windows Password","format":"password","description":"Required when Authentication Type is windows"},"domain":{"type":"string","title":"Windows Domain","description":"Optional -- used when Authentication Type is windows"},"ssl_mode":{"enum":["disable","require"],"type":"string","title":"SSL Mode","default":"disable","description":"Only \"require\" is currently supported by the connector"},"batch_size":{"type":"integer","title":"Batch Size","default":1},"table_name":{"type":"string","title":"Target Table"},"unique_key":{"type":"string","title":"Unique Key Columns","description":"Comma-separated column name(s) for UPSERT/UPDATE operations"},"write_mode":{"enum":["insert","upsert","update"],"type":"string","title":"Operation","default":"insert"}}}'::jsonb,
    parameter_groups = '{"basic":["host","port","database","table_name"],"authentication":["auth_type","username","password","os_user","os_password","domain"],"advanced":["ssl_mode"],"operation":["write_mode","unique_key","batch_size"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'oracle_outbound';

-- SFTP outbound: full rewrite (same shape/rigor as sftp_inbound's V222 fix).
UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["host","username","remote_dir"],"properties":{"host":{"type":"string","title":"SFTP Host"},"port":{"type":"integer","title":"SFTP Port","default":22},"username":{"type":"string","title":"Username"},"auth_type":{"enum":["password","key"],"type":"string","title":"Authentication Type","default":"password"},"password":{"type":"string","title":"Password","format":"password"},"key_content":{"type":"string","title":"Private Key (paste PEM content)","format":"password","description":"Used when Authentication Type is key -- paste the full PEM private key text"},"remote_dir":{"type":"string","title":"Remote Directory","default":"/upload"},"filename_pattern":{"type":"string","title":"Filename Pattern","default":"{message_id}_{timestamp}.hl7","description":"Placeholders: {message_id} {interface_id} {timestamp} {date} {time}. Can include subdirectories, e.g. {interface_id}/{message_id}.hl7 -- created automatically if they do not exist"},"connect_timeout":{"type":"integer","title":"Connect Timeout (seconds)","default":10},"write_timeout":{"type":"integer","title":"Write Timeout (seconds)","default":60}}}'::jsonb,
    parameter_groups = '{"basic":["host","port","username"],"authentication":["auth_type","password","key_content"],"file":["remote_dir","filename_pattern"],"advanced":["connect_timeout","write_timeout"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'sftp_outbound';

-- file_writer: two key fixes only, everything else already correct.
UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["directory_path"],"properties":{"encoding":{"type":"string","title":"File Encoding","default":"UTF-8"},"directory_path":{"type":"string","title":"Output Directory"},"filename_pattern":{"type":"string","title":"Filename Pattern","default":"{message_id}_{timestamp}.hl7"},"overwrite_existing":{"type":"boolean","title":"Overwrite Existing Files","default":false},"create_subdirs":{"type":"boolean","title":"Create Subdirectories","default":true}}}'::jsonb,
    parameter_groups = '{"basic":["directory_path","filename_pattern"],"advanced":["encoding","create_subdirs","overwrite_existing"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'file_writer';
