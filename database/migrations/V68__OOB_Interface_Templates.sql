-- V68: OOB Interface Templates
-- 10 pre-built system templates for common healthcare integration patterns.
-- These appear in the Templates gallery immediately after V67 is applied.

INSERT INTO interface_templates
    (id, name, slug, description, category, subcategory, tags, icon, difficulty,
     source_connector_type, source_config_template,
     target_connector_type, target_config_template,
     pipeline_config, message_type,
     required_connection_fields, sanitized_fields, preview_steps,
     estimated_setup_minutes, is_system, is_public, author, usage_count)
VALUES

-- 1. HL7 ADT^A01 → FHIR Patient
(
    gen_random_uuid(),
    'HL7 ADT^A01 → FHIR Patient',
    'hl7-adt-a01-fhir-patient',
    'Receive ADT A01 (Patient Admit) messages from any HL7 v2 source via TCP/MLLP, transform to a FHIR R4 Patient + Encounter bundle, and deliver to any FHIR-compliant REST endpoint.',
    'hl7v2', null,
    ARRAY['adt','hl7v2','fhir','patient','admit','tcp-mllp'],
    '🏥', 'beginner',
    'tcp_mllp_inbound',
    '{"port": 2575, "ack": {"mode": "immediate", "on_error": "suppress"}, "enable_tls": false, "max_connections": 10}',
    'http_outbound',
    '{"method": "POST", "content_type": "application/fhir+json", "auth_type": "bearer", "timeout_seconds": 30}',
    '{
        "execution_groups": [
            {"sequence": 10, "steps": [{"step_name": "Validate HL7 Message", "step_type": "enrichment.validation", "sequence": 10, "config": {"format": "hl7v2", "required_segments": ["MSH","PID"], "fail_on_error": false}, "enabled": true, "required": false}]},
            {"sequence": 20, "steps": [{"step_name": "HL7 → FHIR Transform (ADT^A01)", "step_type": "hl7_fhir_transform", "sequence": 20, "config": {"message_type": "ADT^A01", "target_resources": ["Patient","Encounter"], "include_metadata": true}, "enabled": true, "required": true}]},
            {"sequence": 30, "steps": [{"step_name": "Validate FHIR Bundle", "step_type": "enrichment.validation", "sequence": 30, "config": {"format": "fhir_r4", "fail_on_error": false}, "enabled": true, "required": false}]}
        ]
    }',
    'ADT^A01',
    '[{"section":"source","field":"host","label":"MLLP Host / IP Address","type":"string","hint":"IP or hostname your HL7 sender connects to","required":true},{"section":"target","field":"endpoint","label":"FHIR Endpoint URL","type":"url","hint":"e.g. https://fhir.hospital.org/R4/","required":true},{"section":"target","field":"token","label":"Bearer Token","type":"password","hint":"Authorization token for FHIR server","required":false}]',
    ARRAY['source.host','target.endpoint','target.token'],
    '[{"name":"Validate HL7","icon":"✅","step_type":"enrichment.validation"},{"name":"HL7→FHIR Transform","icon":"🔄","step_type":"hl7_fhir_transform"},{"name":"Validate FHIR","icon":"🔍","step_type":"enrichment.validation"}]',
    10, true, true, 'ezHealthKonnect', 0
),

-- 2. HL7 ORU^R01 Lab Results → PostgreSQL
(
    gen_random_uuid(),
    'HL7 ORU^R01 Lab Results → Database',
    'hl7-oru-r01-lab-results-db',
    'Receive ORU R01 lab result messages via TCP/MLLP, transform to FHIR Observation resources, and store in a PostgreSQL database for analytics and reporting.',
    'hl7v2', null,
    ARRAY['oru','hl7v2','lab','results','observation','fhir','postgresql'],
    '🔬', 'intermediate',
    'tcp_mllp_inbound',
    '{"port": 2576, "ack": {"mode": "immediate", "on_error": "nack"}, "enable_tls": false}',
    'postgresql_outbound',
    '{"port": 5432, "ssl_mode": "require", "write_mode": "INSERT", "table_name": "lab_results"}',
    '{
        "execution_groups": [
            {"sequence": 10, "steps": [{"step_name": "Validate HL7 ORU", "step_type": "enrichment.validation", "sequence": 10, "config": {"format": "hl7v2", "required_segments": ["MSH","PID","OBR","OBX"], "fail_on_error": false}, "enabled": true, "required": false}]},
            {"sequence": 20, "steps": [{"step_name": "HL7 → FHIR Transform (ORU^R01)", "step_type": "hl7_fhir_transform", "sequence": 20, "config": {"message_type": "ORU^R01", "target_resources": ["Observation","DiagnosticReport","Patient"], "include_metadata": true}, "enabled": true, "required": true}]},
            {"sequence": 30, "steps": [{"step_name": "Map to Lab Result Schema", "step_type": "field_mapping", "sequence": 30, "config": {"mappings": [{"source": "steps.hl7_fhir_transform.step_output.fhir_bundle", "target": "result", "transform": "identity"}]}, "enabled": true, "required": false}]}
        ]
    }',
    'ORU^R01',
    '[{"section":"source","field":"host","label":"MLLP Host / IP Address","type":"string","hint":"","required":true},{"section":"target","field":"host","label":"PostgreSQL Hostname","type":"string","hint":"","required":true},{"section":"target","field":"username","label":"Database Username","type":"string","hint":"","required":true},{"section":"target","field":"password","label":"Database Password","type":"password","hint":"","required":true},{"section":"target","field":"database","label":"Database Name","type":"string","hint":"","required":true}]',
    ARRAY['source.host','target.host','target.username','target.password','target.database'],
    '[{"name":"Validate HL7 ORU","icon":"✅","step_type":"enrichment.validation"},{"name":"HL7→FHIR Transform","icon":"🔄","step_type":"hl7_fhir_transform"},{"name":"Map to DB Schema","icon":"🗺️","step_type":"field_mapping"}]',
    15, true, true, 'ezHealthKonnect', 0
),

-- 3. HL7 SIU^S12 Scheduling → REST API
(
    gen_random_uuid(),
    'HL7 SIU^S12 Scheduling Notifications',
    'hl7-siu-s12-scheduling',
    'Receive SIU S12 (new appointment) messages via MLLP, transform to FHIR Appointment resources, and post to a scheduling system REST API.',
    'hl7v2', null,
    ARRAY['siu','hl7v2','scheduling','appointment','fhir'],
    '📅', 'intermediate',
    'tcp_mllp_inbound',
    '{"port": 2577, "ack": {"mode": "immediate", "on_error": "suppress"}, "enable_tls": false}',
    'http_outbound',
    '{"method": "POST", "content_type": "application/fhir+json", "auth_type": "api_key", "timeout_seconds": 30}',
    '{
        "execution_groups": [
            {"sequence": 10, "steps": [{"step_name": "Validate SIU Message", "step_type": "enrichment.validation", "sequence": 10, "config": {"format": "hl7v2", "required_segments": ["MSH","SCH","PID"], "fail_on_error": false}, "enabled": true, "required": false}]},
            {"sequence": 20, "steps": [{"step_name": "HL7 → FHIR Transform (SIU^S12)", "step_type": "hl7_fhir_transform", "sequence": 20, "config": {"message_type": "SIU^S12", "target_resources": ["Appointment","Patient","Practitioner"], "include_metadata": true}, "enabled": true, "required": true}]}
        ]
    }',
    'SIU^S12',
    '[{"section":"source","field":"host","label":"MLLP Host / IP Address","type":"string","hint":"","required":true},{"section":"target","field":"endpoint","label":"Scheduling API Endpoint URL","type":"url","hint":"","required":true},{"section":"target","field":"api_key","label":"API Key","type":"password","hint":"","required":false}]',
    ARRAY['source.host','target.endpoint','target.api_key'],
    '[{"name":"Validate SIU","icon":"✅","step_type":"enrichment.validation"},{"name":"HL7→FHIR Transform","icon":"🔄","step_type":"hl7_fhir_transform"}]',
    12, true, true, 'ezHealthKonnect', 0
),

-- 4. HL7 MDM^T02 Clinical Documents → Document Store
(
    gen_random_uuid(),
    'HL7 MDM^T02 Clinical Documents → Storage',
    'hl7-mdm-t02-documents',
    'Receive MDM T02 (document notification with content) messages via MLLP, extract document content, and archive to MongoDB or AWS S3 for long-term storage.',
    'hl7v2', null,
    ARRAY['mdm','hl7v2','documents','cda','storage','mongodb'],
    '📄', 'intermediate',
    'tcp_mllp_inbound',
    '{"port": 2578, "ack": {"mode": "immediate", "on_error": "suppress"}, "enable_tls": false}',
    'mongodb_outbound',
    '{"port": 27017, "database": "clinical_documents", "collection": "mdm_documents", "write_mode": "INSERT"}',
    '{
        "execution_groups": [
            {"sequence": 10, "steps": [{"step_name": "Validate MDM Message", "step_type": "enrichment.validation", "sequence": 10, "config": {"format": "hl7v2", "required_segments": ["MSH","EVN","PID","TXA","OBX"], "fail_on_error": false}, "enabled": true, "required": false}]},
            {"sequence": 20, "steps": [{"step_name": "Extract Document Metadata", "step_type": "field_mapping", "sequence": 20, "config": {"mappings": [{"source": "MSH.3", "target": "sending_app"},{"source": "PID.3", "target": "patient_id"},{"source": "TXA.2", "target": "document_type"}]}, "enabled": true, "required": true}]}
        ]
    }',
    'MDM^T02',
    '[{"section":"source","field":"host","label":"MLLP Host / IP Address","type":"string","hint":"","required":true},{"section":"target","field":"host","label":"MongoDB Hostname","type":"string","hint":"","required":true},{"section":"target","field":"username","label":"MongoDB Username","type":"string","hint":"","required":false},{"section":"target","field":"password","label":"MongoDB Password","type":"password","hint":"","required":false}]',
    ARRAY['source.host','target.host','target.username','target.password'],
    '[{"name":"Validate MDM","icon":"✅","step_type":"enrichment.validation"},{"name":"Extract Document Metadata","icon":"🗺️","step_type":"field_mapping"}]',
    15, true, true, 'ezHealthKonnect', 0
),

-- 5. HL7 ORM^O01 Orders → REST API
(
    gen_random_uuid(),
    'HL7 ORM^O01 Orders → Order System',
    'hl7-orm-o01-orders',
    'Receive ORM O01 (general order) messages via MLLP, transform to FHIR ServiceRequest resources, and deliver to an order management system REST API.',
    'hl7v2', null,
    ARRAY['orm','hl7v2','orders','service-request','fhir'],
    '📋', 'intermediate',
    'tcp_mllp_inbound',
    '{"port": 2579, "ack": {"mode": "immediate", "on_error": "nack"}, "enable_tls": false}',
    'http_outbound',
    '{"method": "POST", "content_type": "application/fhir+json", "auth_type": "bearer", "timeout_seconds": 30}',
    '{
        "execution_groups": [
            {"sequence": 10, "steps": [{"step_name": "Validate ORM Message", "step_type": "enrichment.validation", "sequence": 10, "config": {"format": "hl7v2", "required_segments": ["MSH","PID","ORC","OBR"], "fail_on_error": false}, "enabled": true, "required": false}]},
            {"sequence": 20, "steps": [{"step_name": "HL7 → FHIR Transform (ORM^O01)", "step_type": "hl7_fhir_transform", "sequence": 20, "config": {"message_type": "ORM^O01", "target_resources": ["ServiceRequest","Patient","Practitioner"], "include_metadata": true}, "enabled": true, "required": true}]}
        ]
    }',
    'ORM^O01',
    '[{"section":"source","field":"host","label":"MLLP Host / IP Address","type":"string","hint":"","required":true},{"section":"target","field":"endpoint","label":"Order System Endpoint URL","type":"url","hint":"","required":true},{"section":"target","field":"token","label":"Bearer Token","type":"password","hint":"","required":false}]',
    ARRAY['source.host','target.endpoint','target.token'],
    '[{"name":"Validate ORM","icon":"✅","step_type":"enrichment.validation"},{"name":"HL7→FHIR Transform","icon":"🔄","step_type":"hl7_fhir_transform"}]',
    12, true, true, 'ezHealthKonnect', 0
),

-- 6. FHIR R4 Bundle Receiver → PostgreSQL
(
    gen_random_uuid(),
    'FHIR R4 Bundle Receiver → Database',
    'fhir-r4-bundle-receiver-db',
    'Accept inbound FHIR R4 Bundle POSTs via HTTP/HTTPS, validate the bundle structure, map to relational columns, and persist to PostgreSQL for downstream analytics.',
    'fhir', null,
    ARRAY['fhir','r4','bundle','http','postgresql','inbound'],
    '🌐', 'intermediate',
    'http_rest_inbound',
    '{"path": "/fhir/bundle", "method": "POST", "content_type": "application/fhir+json", "enable_auth": true, "auth_type": "bearer"}',
    'postgresql_outbound',
    '{"port": 5432, "ssl_mode": "require", "write_mode": "UPSERT", "table_name": "fhir_resources"}',
    '{
        "execution_groups": [
            {"sequence": 10, "steps": [{"step_name": "Validate FHIR Bundle", "step_type": "enrichment.validation", "sequence": 10, "config": {"format": "fhir_r4", "fail_on_error": true}, "enabled": true, "required": true}]},
            {"sequence": 20, "steps": [{"step_name": "Map FHIR Fields to DB Columns", "step_type": "field_mapping", "sequence": 20, "config": {"mappings": [{"source": "resourceType", "target": "resource_type"},{"source": "id", "target": "fhir_id"},{"source": "meta.lastUpdated", "target": "last_updated"}]}, "enabled": true, "required": false}]}
        ]
    }',
    null,
    '[{"section":"target","field":"host","label":"PostgreSQL Hostname","type":"string","hint":"","required":true},{"section":"target","field":"username","label":"Database Username","type":"string","hint":"","required":true},{"section":"target","field":"password","label":"Database Password","type":"password","hint":"","required":true},{"section":"target","field":"database","label":"Database Name","type":"string","hint":"","required":true}]',
    ARRAY['target.host','target.username','target.password','target.database'],
    '[{"name":"Validate FHIR Bundle","icon":"🔍","step_type":"enrichment.validation"},{"name":"Map to DB Schema","icon":"🗺️","step_type":"field_mapping"}]',
    12, true, true, 'ezHealthKonnect', 0
),

-- 7. FHIR Patient Sync → PostgreSQL
(
    gen_random_uuid(),
    'FHIR Patient Sync → Database',
    'fhir-patient-sync-db',
    'Poll a FHIR R4 server for Patient resource updates using incremental timestamps, apply optional PHI masking for analytics, and store in PostgreSQL.',
    'fhir', null,
    ARRAY['fhir','r4','patient','sync','polling','postgresql','phi','masking'],
    '🔄', 'intermediate',
    'http_outbound',
    '{"method": "GET", "content_type": "application/fhir+json", "auth_type": "bearer", "timeout_seconds": 60}',
    'postgresql_outbound',
    '{"port": 5432, "ssl_mode": "require", "write_mode": "UPSERT", "table_name": "patients"}',
    '{
        "execution_groups": [
            {"sequence": 10, "steps": [{"step_name": "Map FHIR Patient Fields", "step_type": "field_mapping", "sequence": 10, "config": {"mappings": [{"source": "id", "target": "fhir_id"},{"source": "name[0].family", "target": "family_name"},{"source": "name[0].given[0]", "target": "given_name"},{"source": "birthDate", "target": "birth_date"},{"source": "gender", "target": "gender"}]}, "enabled": true, "required": true}]},
            {"sequence": 20, "steps": [{"step_name": "Mask PHI for Analytics", "step_type": "data_masking", "sequence": 20, "config": {"mask_all_phi": false, "fields": [{"field": "birth_date", "strategy": "date_shift"}]}, "enabled": false, "required": false}]}
        ]
    }',
    null,
    '[{"section":"source","field":"endpoint","label":"FHIR Server Base URL","type":"url","hint":"e.g. https://fhir.hospital.org/R4/Patient","required":true},{"section":"source","field":"token","label":"Bearer Token","type":"password","hint":"Authorization token for FHIR server","required":false},{"section":"target","field":"host","label":"PostgreSQL Hostname","type":"string","hint":"","required":true},{"section":"target","field":"username","label":"Database Username","type":"string","hint":"","required":true},{"section":"target","field":"password","label":"Database Password","type":"password","hint":"","required":true},{"section":"target","field":"database","label":"Database Name","type":"string","hint":"","required":true}]',
    ARRAY['source.endpoint','source.token','target.host','target.username','target.password','target.database'],
    '[{"name":"Map FHIR Patient Fields","icon":"🗺️","step_type":"field_mapping"},{"name":"Mask PHI (optional)","icon":"🔒","step_type":"data_masking"}]',
    18, true, true, 'ezHealthKonnect', 0
),

-- 8. Epic ADT Feed (MLLP-tuned)
(
    gen_random_uuid(),
    'Epic ADT Feed → FHIR Endpoint',
    'epic-adt-feed-fhir',
    'Receive ADT messages from Epic Systems via TCP/MLLP (pre-configured with Epic-compatible ACK settings and Z-segment awareness), transform to FHIR R4 Patient + Encounter, and deliver to a FHIR server.',
    'emr', 'epic',
    ARRAY['epic','adt','hl7v2','fhir','patient','emr','tcp-mllp'],
    '🏥', 'intermediate',
    'tcp_mllp_inbound',
    '{"port": 2575, "ack": {"mode": "immediate", "on_error": "suppress", "sending_app": "ezHealthKonnect", "sending_facility": "EHK", "text_success": "Message received by ezHealthKonnect"}, "enable_tls": false, "max_connections": 25, "keep_alive": true}',
    'http_outbound',
    '{"method": "POST", "content_type": "application/fhir+json", "auth_type": "bearer", "timeout_seconds": 30, "retry_count": 3}',
    '{
        "execution_groups": [
            {"sequence": 10, "steps": [{"step_name": "Validate HL7 (Epic)", "step_type": "enrichment.validation", "sequence": 10, "config": {"format": "hl7v2", "required_segments": ["MSH","EVN","PID","PV1"], "warn_unknown_segments": true, "fail_on_error": false}, "enabled": true, "required": false}]},
            {"sequence": 20, "steps": [{"step_name": "HL7 → FHIR Transform (ADT^A01)", "step_type": "hl7_fhir_transform", "sequence": 20, "config": {"message_type": "ADT^A01", "target_resources": ["Patient","Encounter"], "include_metadata": true, "preserve_z_segments": true}, "enabled": true, "required": true}]},
            {"sequence": 30, "steps": [{"step_name": "Validate FHIR Output", "step_type": "enrichment.validation", "sequence": 30, "config": {"format": "fhir_r4", "fail_on_error": false}, "enabled": true, "required": false}]}
        ]
    }',
    'ADT^A01',
    '[{"section":"source","field":"host","label":"MLLP Listener IP / Hostname","type":"string","hint":"IP or hostname for Epic to send HL7 messages to","required":true},{"section":"target","field":"endpoint","label":"FHIR Endpoint URL","type":"url","hint":"e.g. https://fhir.hospital.org/R4/","required":true},{"section":"target","field":"token","label":"Bearer Token","type":"password","hint":"FHIR server authorization token","required":false}]',
    ARRAY['source.host','target.endpoint','target.token'],
    '[{"name":"Validate HL7 (Epic)","icon":"✅","step_type":"enrichment.validation"},{"name":"HL7→FHIR Transform","icon":"🔄","step_type":"hl7_fhir_transform"},{"name":"Validate FHIR","icon":"🔍","step_type":"enrichment.validation"}]',
    10, true, true, 'ezHealthKonnect', 0
),

-- 9. Cerner ORU Results → Kafka
(
    gen_random_uuid(),
    'Cerner ORU Results → Kafka Stream',
    'cerner-oru-results-kafka',
    'Receive ORU R01 lab result messages from Cerner Millennium via TCP/MLLP, transform to FHIR Observation resources, and publish to Kafka for real-time event streaming to downstream consumers.',
    'emr', 'cerner',
    ARRAY['cerner','oru','hl7v2','fhir','observation','kafka','streaming','emr'],
    '🏥', 'advanced',
    'tcp_mllp_inbound',
    '{"port": 2580, "ack": {"mode": "immediate", "on_error": "nack", "sending_app": "ezHealthKonnect", "sending_facility": "EHK"}, "enable_tls": false, "max_connections": 20}',
    'kafka_outbound',
    '{"port": 9092, "topic": "hl7-lab-results", "compression": "gzip", "required_acks": "all"}',
    '{
        "execution_groups": [
            {"sequence": 10, "steps": [{"step_name": "Validate HL7 ORU (Cerner)", "step_type": "enrichment.validation", "sequence": 10, "config": {"format": "hl7v2", "required_segments": ["MSH","PID","OBR","OBX"], "warn_unknown_segments": true, "fail_on_error": false}, "enabled": true, "required": false}]},
            {"sequence": 20, "steps": [{"step_name": "HL7 → FHIR Transform (ORU^R01)", "step_type": "hl7_fhir_transform", "sequence": 20, "config": {"message_type": "ORU^R01", "target_resources": ["Observation","DiagnosticReport","Patient"], "include_metadata": true}, "enabled": true, "required": true}]},
            {"sequence": 30, "steps": [{"step_name": "Add Kafka Message Key", "step_type": "field_mapping", "sequence": 30, "config": {"mappings": [{"source": "PID.3", "target": "kafka_key"}]}, "enabled": true, "required": false}]}
        ]
    }',
    'ORU^R01',
    '[{"section":"source","field":"host","label":"MLLP Listener IP / Hostname","type":"string","hint":"IP or hostname for Cerner to send HL7 messages to","required":true},{"section":"target","field":"host","label":"Kafka Bootstrap Server Host","type":"string","hint":"e.g. kafka.hospital.org","required":true},{"section":"target","field":"username","label":"Kafka SASL Username","type":"string","hint":"Leave blank if no auth required","required":false},{"section":"target","field":"password","label":"Kafka SASL Password","type":"password","hint":"Leave blank if no auth required","required":false}]',
    ARRAY['source.host','target.host','target.username','target.password'],
    '[{"name":"Validate HL7 ORU","icon":"✅","step_type":"enrichment.validation"},{"name":"HL7→FHIR Transform","icon":"🔄","step_type":"hl7_fhir_transform"},{"name":"Add Kafka Key","icon":"🗺️","step_type":"field_mapping"}]',
    20, true, true, 'ezHealthKonnect', 0
),

-- 10. Multi-System ADT Fan-Out (parallel delivery)
(
    gen_random_uuid(),
    'ADT Fan-Out → Multiple Destinations',
    'adt-fan-out-multi-destination',
    'Receive ADT messages via MLLP, transform to FHIR, and simultaneously deliver to multiple downstream systems (e.g. EHR FHIR endpoint + analytics database + event stream). Uses parallel pipeline execution.',
    'hl7v2', null,
    ARRAY['adt','hl7v2','fhir','fan-out','multi-destination','parallel','kafka','postgresql'],
    '🔀', 'advanced',
    'tcp_mllp_inbound',
    '{"port": 2575, "ack": {"mode": "immediate", "on_error": "suppress"}, "enable_tls": false, "max_connections": 20}',
    'http_outbound',
    '{"method": "POST", "content_type": "application/fhir+json", "auth_type": "bearer", "timeout_seconds": 30}',
    '{
        "execution_groups": [
            {"sequence": 10, "steps": [{"step_name": "Validate HL7", "step_type": "enrichment.validation", "sequence": 10, "config": {"format": "hl7v2", "fail_on_error": false}, "enabled": true, "required": false}]},
            {"sequence": 20, "steps": [{"step_name": "HL7 → FHIR Transform", "step_type": "hl7_fhir_transform", "sequence": 20, "config": {"message_type": "ADT^A01", "target_resources": ["Patient","Encounter"], "include_metadata": true}, "enabled": true, "required": true}]},
            {"sequence": 30, "steps": [
                {"step_name": "Deliver to FHIR Server", "step_type": "connector.outbound", "sequence": 30, "config": {"connectorType": "http_outbound", "config": {"method": "POST", "content_type": "application/fhir+json", "auth_type": "bearer"}, "contentField": "fhirBundle"}, "enabled": true, "required": false},
                {"step_name": "Store in Analytics DB", "step_type": "connector.outbound", "sequence": 30, "config": {"connectorType": "postgresql_outbound", "config": {"ssl_mode": "require", "write_mode": "UPSERT", "table_name": "fhir_events"}, "contentField": "fhirBundle"}, "enabled": true, "required": false},
                {"step_name": "Publish to Event Stream", "step_type": "connector.outbound", "sequence": 30, "config": {"connectorType": "kafka_outbound", "config": {"topic": "adt-events", "compression": "gzip"}, "contentField": "fhirBundle"}, "enabled": false, "required": false}
            ]}
        ]
    }',
    'ADT^A01',
    '[{"section":"source","field":"host","label":"MLLP Listener IP / Hostname","type":"string","hint":"","required":true},{"section":"target","field":"endpoint","label":"Primary FHIR Endpoint URL","type":"url","hint":"Main FHIR destination","required":true},{"section":"target","field":"token","label":"FHIR Bearer Token","type":"password","hint":"","required":false}]',
    ARRAY['source.host','target.endpoint','target.token'],
    '[{"name":"Validate HL7","icon":"✅","step_type":"enrichment.validation"},{"name":"HL7→FHIR Transform","icon":"🔄","step_type":"hl7_fhir_transform"},{"name":"Fan-Out (3 destinations)","icon":"🔀","step_type":"connector.outbound"}]',
    25, true, true, 'ezHealthKonnect', 0
);
