-- Integration test seed data for MySQL inbound connector tests
-- This runs automatically on container startup via docker-entrypoint-initdb.d

USE testdb;

-- HL7 inbox table for connector polling tests
CREATE TABLE IF NOT EXISTS hl7_inbox (
    id           INT AUTO_INCREMENT PRIMARY KEY,
    message_type VARCHAR(50)    NOT NULL DEFAULT 'ADT^A01',
    raw_message  TEXT           NOT NULL,
    received_at  DATETIME       NOT NULL DEFAULT NOW(),
    processed    TINYINT(1)     NOT NULL DEFAULT 0,
    source_ip    VARCHAR(50),
    INDEX idx_processed (processed),
    INDEX idx_received_at (received_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Insert test HL7 messages
INSERT INTO hl7_inbox (message_type, raw_message, processed) VALUES
(
    'ADT^A01',
    'MSH|^~\\&|SEND|SEND_FAC|RECV|RECV_FAC|20240315120000||ADT^A01|MSG1001|P|2.5\rPID|1||MRN001^^^HospA^MR||Smith^John^M||19800101|M|||100 Main St^^Anytown^IL^60601\r',
    0
),
(
    'ADT^A01',
    'MSH|^~\\&|SEND|SEND_FAC|RECV|RECV_FAC|20240315120005||ADT^A01|MSG1002|P|2.5\rPID|1||MRN002^^^HospA^MR||Doe^Jane^F||19750420|F|||200 Oak Ave^^Springfield^IL^62701\r',
    0
),
(
    'ORU^R01',
    'MSH|^~\\&|LAB|LAB_FAC|RECV|RECV_FAC|20240315120010||ORU^R01|MSG1003|P|2.5\rPID|1||MRN001^^^HospA^MR||Smith^John^M||19800101|M\rOBR|1|||CBC^Complete Blood Count\rOBX|1|NM|WBC^White Blood Cells||7.5|K/uL|4.5-11.0|N\r',
    0
),
(
    'ADT^A01',
    'MSH|^~\\&|SEND|SEND_FAC|RECV|RECV_FAC|20240315120015||ADT^A01|MSG1004|P|2.5\rPID|1||MRN003^^^HospA^MR||Williams^Robert||19900615|M|||300 Elm St^^Chicago^IL^60601\r',
    1  -- already processed, should be skipped by update_flag after-processing test
);
