-- V147__Docker_Port_Settings.sql
-- Stores Docker port range configuration so users can manage it from the UI.

INSERT INTO system_settings (key, value, description, updated_by)
VALUES (
    'docker_ports',
    '{
        "hl7_port_range": "6500-6700",
        "http_port_range": "8081-8099",
        "standard_mllp_port": 2575,
        "compose_file_path": "/app/docker-compose.prod.yml",
        "deployment_mode": "docker"
    }',
    'Docker port range configuration for HL7/MLLP and HTTP/FHIR interfaces',
    'system'
)
ON CONFLICT (key) DO NOTHING;
