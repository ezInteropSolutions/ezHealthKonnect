-- V272__ATNA_Syslog_Settings.sql
-- Seeds the "atna_syslog" system_settings row so ATNA syslog export
-- (services/audit/{syslog_sender.go,syslog_audit_logger.go} on the Go side,
-- services/audit/atnaSyslogExporter.js on the Node side) becomes admin-UI
-- configurable at runtime, instead of only via ATNA_SYSLOG_* environment
-- variables read once at process startup. Disabled by default — this row's
-- own "enabled": false is what a fresh deployment starts with; ATNA_SYSLOG_*
-- env vars remain a working fallback for anyone who configured it that way
-- before this admin UI existed (see resolveSyslogConfig() in
-- services/audit/syslog_sender.go).

INSERT INTO system_settings (key, value, description, updated_by)
VALUES (
    'atna_syslog',
    '{
        "enabled": false,
        "host": "",
        "port": 514,
        "protocol": "udp",
        "facility": 10,
        "app_name": "ezHealthKonnect",
        "audit_source_id": "ezHealthKonnect",
        "enterprise_site_id": "",
        "tls_insecure_skip_verify": false
    }',
    'ATNA (IHE Audit Trail and Node Authentication) syslog export configuration — forwards every audit event as an RFC 5424 + DICOM AuditMessage XML record to an external Audit Record Repository (ARR)',
    'system'
)
ON CONFLICT (key) DO NOTHING;
