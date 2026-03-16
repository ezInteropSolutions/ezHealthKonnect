-- Oracle XE test seed — executed by gvenzl/oracle-xe via container-entrypoint-initdb.d
-- Runs as the APP_USER (testuser) created by the image's ORACLE_PASSWORD/APP_USER env vars.
-- Oracle 21c syntax: FETCH FIRST N ROWS ONLY is supported.

-- HL7 inbox table for connector polling tests
CREATE TABLE hl7_inbox (
    id           NUMBER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    message_type VARCHAR2(50)   DEFAULT 'ADT^A01' NOT NULL,
    raw_message  CLOB           NOT NULL,
    received_at  TIMESTAMP      DEFAULT CURRENT_TIMESTAMP NOT NULL,
    processed    NUMBER(1)      DEFAULT 0 NOT NULL,
    source_ip    VARCHAR2(50)
);

CREATE INDEX idx_hl7_processed   ON hl7_inbox (processed);
CREATE INDEX idx_hl7_received_at ON hl7_inbox (received_at);

-- Seed test HL7 messages
INSERT INTO hl7_inbox (message_type, raw_message, processed) VALUES (
    'ADT^A01',
    'MSH|^~\&|SEND|SEND_FAC|RECV|RECV_FAC|20240315120000||ADT^A01|ORCMSG1001|P|2.5' || CHR(13) ||
    'PID|1||MRN_ORA_001^^^HospA^MR||Smith^John^M||19800101|M|||100 Main St^^Anytown^IL^60601' || CHR(13),
    0
);

INSERT INTO hl7_inbox (message_type, raw_message, processed) VALUES (
    'ADT^A01',
    'MSH|^~\&|SEND|SEND_FAC|RECV|RECV_FAC|20240315120005||ADT^A01|ORCMSG1002|P|2.5' || CHR(13) ||
    'PID|1||MRN_ORA_002^^^HospA^MR||Doe^Jane^F||19750420|F|||200 Oak Ave^^Springfield^IL^62701' || CHR(13),
    0
);

INSERT INTO hl7_inbox (message_type, raw_message, processed) VALUES (
    'ORU^R01',
    'MSH|^~\&|LAB|LAB_FAC|RECV|RECV_FAC|20240315120010||ORU^R01|ORCMSG1003|P|2.5' || CHR(13) ||
    'PID|1||MRN_ORA_001^^^HospA^MR||Smith^John^M||19800101|M' || CHR(13) ||
    'OBR|1|||CBC^Complete Blood Count' || CHR(13) ||
    'OBX|1|NM|WBC^White Blood Cells||7.5|K/uL|4.5-11.0|N' || CHR(13),
    0
);

INSERT INTO hl7_inbox (message_type, raw_message, processed) VALUES (
    'ADT^A01',
    'MSH|^~\&|SEND|SEND_FAC|RECV|RECV_FAC|20240315120015||ADT^A01|ORCMSG1004|P|2.5' || CHR(13) ||
    'PID|1||MRN_ORA_003^^^HospA^MR||Williams^Robert||19900615|M|||300 Elm St^^Chicago^IL^60601' || CHR(13),
    1
);

COMMIT;
