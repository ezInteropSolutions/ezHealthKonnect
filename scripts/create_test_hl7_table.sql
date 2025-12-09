-- Create test table for HL7 messages (Database Connector Testing)
-- Run this script in your PostgreSQL database

-- Drop table if exists (for clean testing)
DROP TABLE IF EXISTS test_hl7_messages;
DROP TABLE IF EXISTS test_hl7_messages_archive;

-- Create main test table
CREATE TABLE test_hl7_messages (
    id SERIAL PRIMARY KEY,
    message_id VARCHAR(100) UNIQUE,
    patient_id VARCHAR(50),
    message_type VARCHAR(20),
    raw_message TEXT NOT NULL,
    priority VARCHAR(10) DEFAULT 'normal',
    processed BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create archive table (same schema for archive testing)
CREATE TABLE test_hl7_messages_archive (
    id SERIAL PRIMARY KEY,
    message_id VARCHAR(100),
    patient_id VARCHAR(50),
    message_type VARCHAR(20),
    raw_message TEXT NOT NULL,
    priority VARCHAR(10) DEFAULT 'normal',
    processed BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    archived_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for performance
CREATE INDEX idx_test_hl7_updated_at ON test_hl7_messages(updated_at);
CREATE INDEX idx_test_hl7_processed ON test_hl7_messages(processed);
CREATE INDEX idx_test_hl7_message_type ON test_hl7_messages(message_type);

-- Insert sample HL7 ADT^A01 messages (Patient Admission)
INSERT INTO test_hl7_messages (message_id, patient_id, message_type, raw_message, priority) VALUES

-- Patient 1: John Smith - Emergency admission
('MSG001', 'PID001', 'ADT^A01',
'MSH|^~\&|EPIC|HOSPITAL|EZHEALTHKONNECT|DEST|20250118120000||ADT^A01|MSG001|P|2.5
EVN|A01|20250118120000
PID|1||PID001^^^HOSPITAL^MR||Smith^John^A||19800515|M|||123 Main St^^Chicago^IL^60601||312-555-0101|||S||ACC001
NK1|1|Smith^Jane|SPO|456 Oak Ave^^Chicago^IL^60602|312-555-0102
PV1|1|E|ER^101^A|E|||DOC001^Johnson^Robert|||MED||||1|||DOC001^Johnson^Robert|ER|||||||||||||||||||||||||20250118120000
DG1|1||I10^Essential hypertension^ICD10|||A
IN1|1|BCBS001|Blue Cross Blue Shield|PO Box 1234^^Chicago^IL^60690|||||GRP001|||20240101|20251231|||Smith^John^A|SELF|19800515|123 Main St^^Chicago^IL^60601||||||||||||||||POL001',
'high'),

-- Patient 2: Mary Johnson - Scheduled admission
('MSG002', 'PID002', 'ADT^A01',
'MSH|^~\&|CERNER|HOSPITAL|EZHEALTHKONNECT|DEST|20250118130000||ADT^A01|MSG002|P|2.5
EVN|A01|20250118130000
PID|1||PID002^^^HOSPITAL^MR||Johnson^Mary^B||19750220|F|||789 Elm St^^Naperville^IL^60540||630-555-0201|||M||ACC002
NK1|1|Johnson^Robert|SPO|789 Elm St^^Naperville^IL^60540|630-555-0202
PV1|1|I|MED^201^B|R|||DOC002^Williams^Sarah|||SUR||||2|||DOC002^Williams^Sarah|IN|||||||||||||||||||||||||20250118130000
DG1|1||K80.20^Calculus of gallbladder^ICD10|||A
IN1|1|AETNA001|Aetna Health|PO Box 5678^^Hartford^CT^06102|||||GRP002|||20240601|20251231|||Johnson^Mary^B|SELF|19750220|789 Elm St^^Naperville^IL^60540||||||||||||||||POL002',
'normal'),

-- Patient 3: Robert Davis - ICU admission
('MSG003', 'PID003', 'ADT^A01',
'MSH|^~\&|EPIC|HOSPITAL|EZHEALTHKONNECT|DEST|20250118140000||ADT^A01|MSG003|P|2.5
EVN|A01|20250118140000
PID|1||PID003^^^HOSPITAL^MR||Davis^Robert^C||19650810|M|||456 Pine Rd^^Evanston^IL^60201||847-555-0301|||W||ACC003
NK1|1|Davis^Linda|SPO|456 Pine Rd^^Evanston^IL^60201|847-555-0302
PV1|1|I|ICU^301^A|E|||DOC003^Martinez^Carlos|||CAR||||3|||DOC003^Martinez^Carlos|IC|||||||||||||||||||||||||20250118140000
DG1|1||I21.0^ST elevation myocardial infarction^ICD10|||A
DG1|2||I25.10^Atherosclerotic heart disease^ICD10|||A
IN1|1|UHC001|United Healthcare|PO Box 9012^^Minneapolis^MN^55440|||||GRP003|||20240101|20251231|||Davis^Robert^C|SELF|19650810|456 Pine Rd^^Evanston^IL^60201||||||||||||||||POL003',
'critical'),

-- Patient 4: Sarah Wilson - Maternity admission
('MSG004', 'PID004', 'ADT^A01',
'MSH|^~\&|MEDITECH|HOSPITAL|EZHEALTHKONNECT|DEST|20250118150000||ADT^A01|MSG004|P|2.5
EVN|A01|20250118150000
PID|1||PID004^^^HOSPITAL^MR||Wilson^Sarah^D||19900315|F|||321 Cedar Ln^^Schaumburg^IL^60173||847-555-0401|||M||ACC004
NK1|1|Wilson^Michael|SPO|321 Cedar Ln^^Schaumburg^IL^60173|847-555-0402
PV1|1|I|OB^401^A|R|||DOC004^Thompson^Lisa|||OBG||||1|||DOC004^Thompson^Lisa|OB|||||||||||||||||||||||||20250118150000
DG1|1||O80^Encounter for full-term uncomplicated delivery^ICD10|||A
IN1|1|CIGNA001|Cigna Health|PO Box 3456^^Bloomfield^CT^06002|||||GRP004|||20240301|20251231|||Wilson^Sarah^D|SELF|19900315|321 Cedar Ln^^Schaumburg^IL^60173||||||||||||||||POL004',
'normal'),

-- Patient 5: James Brown - Surgical admission
('MSG005', 'PID005', 'ADT^A01',
'MSH|^~\&|EPIC|HOSPITAL|EZHEALTHKONNECT|DEST|20250118160000||ADT^A01|MSG005|P|2.5
EVN|A01|20250118160000
PID|1||PID005^^^HOSPITAL^MR||Brown^James^E||19550720|M|||654 Maple Dr^^Oak Park^IL^60302||708-555-0501|||D||ACC005
NK1|1|Brown^Patricia|EX|987 Birch St^^Oak Park^IL^60302|708-555-0502
NK1|2|Brown^Michael|SON|654 Maple Dr^^Oak Park^IL^60302|708-555-0503
PV1|1|I|SUR^501^A|E|||DOC005^Anderson^Michael|||ORT||||2|||DOC005^Anderson^Michael|SU|||||||||||||||||||||||||20250118160000
DG1|1||M17.11^Primary osteoarthritis right knee^ICD10|||A
IN1|1|MEDICARE|Medicare|PO Box 7890^^Baltimore^MD^21244|||||||||20200101||||Brown^James^E|SELF|19550720|654 Maple Dr^^Oak Park^IL^60302||||||||||||||||MED005',
'normal');

-- Insert sample HL7 ORU^R01 messages (Lab Results)
INSERT INTO test_hl7_messages (message_id, patient_id, message_type, raw_message, priority) VALUES

-- Lab Result 1: Complete Blood Count
('MSG006', 'PID001', 'ORU^R01',
'MSH|^~\&|LAB|HOSPITAL|EZHEALTHKONNECT|DEST|20250118121500||ORU^R01|MSG006|P|2.5
PID|1||PID001^^^HOSPITAL^MR||Smith^John^A||19800515|M
ORC|RE|ORD001|LAB001||CM||||20250118121500|||DOC001^Johnson^Robert
OBR|1|ORD001|LAB001|58410-2^CBC panel^LN|||20250118120000|||||||||DOC001^Johnson^Robert||||||20250118121500|||F
OBX|1|NM|6690-2^WBC^LN||12.5|10*3/uL|4.5-11.0|H|||F
OBX|2|NM|789-8^RBC^LN||4.8|10*6/uL|4.5-5.5||||F
OBX|3|NM|718-7^Hemoglobin^LN||14.2|g/dL|13.5-17.5||||F
OBX|4|NM|4544-3^Hematocrit^LN||42.1|%|38.8-50.0||||F
OBX|5|NM|777-3^Platelets^LN||245|10*3/uL|150-400||||F',
'high'),

-- Lab Result 2: Basic Metabolic Panel
('MSG007', 'PID002', 'ORU^R01',
'MSH|^~\&|LAB|HOSPITAL|EZHEALTHKONNECT|DEST|20250118131500||ORU^R01|MSG007|P|2.5
PID|1||PID002^^^HOSPITAL^MR||Johnson^Mary^B||19750220|F
ORC|RE|ORD002|LAB002||CM||||20250118131500|||DOC002^Williams^Sarah
OBR|1|ORD002|LAB002|51990-0^Basic metabolic panel^LN|||20250118130000|||||||||DOC002^Williams^Sarah||||||20250118131500|||F
OBX|1|NM|2345-7^Glucose^LN||95|mg/dL|70-100||||F
OBX|2|NM|3094-0^BUN^LN||18|mg/dL|7-20||||F
OBX|3|NM|2160-0^Creatinine^LN||0.9|mg/dL|0.6-1.2||||F
OBX|4|NM|2951-2^Sodium^LN||140|mmol/L|136-145||||F
OBX|5|NM|2823-3^Potassium^LN||4.2|mmol/L|3.5-5.1||||F
OBX|6|NM|2075-0^Chloride^LN||102|mmol/L|98-106||||F
OBX|7|NM|1963-8^Bicarbonate^LN||24|mmol/L|22-29||||F',
'normal'),

-- Lab Result 3: Cardiac Enzymes (Critical)
('MSG008', 'PID003', 'ORU^R01',
'MSH|^~\&|LAB|HOSPITAL|EZHEALTHKONNECT|DEST|20250118141500||ORU^R01|MSG008|P|2.5
PID|1||PID003^^^HOSPITAL^MR||Davis^Robert^C||19650810|M
ORC|RE|ORD003|LAB003||CM||||20250118141500|||DOC003^Martinez^Carlos
OBR|1|ORD003|LAB003|2157-6^Cardiac enzymes panel^LN|||20250118140000|||||||||DOC003^Martinez^Carlos||||||20250118141500|||F
OBX|1|NM|10839-9^Troponin I^LN||2.5|ng/mL|0.0-0.04|HH|||F
OBX|2|NM|2154-3^CK-MB^LN||45|ng/mL|0-5|H|||F
OBX|3|NM|30522-7^BNP^LN||850|pg/mL|0-100|H|||F
OBX|4|NM|33762-6^NT-proBNP^LN||2500|pg/mL|0-125|H|||F',
'critical'),

-- Lab Result 4: Prenatal Panel
('MSG009', 'PID004', 'ORU^R01',
'MSH|^~\&|LAB|HOSPITAL|EZHEALTHKONNECT|DEST|20250118151500||ORU^R01|MSG009|P|2.5
PID|1||PID004^^^HOSPITAL^MR||Wilson^Sarah^D||19900315|F
ORC|RE|ORD004|LAB004||CM||||20250118151500|||DOC004^Thompson^Lisa
OBR|1|ORD004|LAB004|24364-2^Prenatal panel^LN|||20250118150000|||||||||DOC004^Thompson^Lisa||||||20250118151500|||F
OBX|1|CE|882-1^ABO group^LN||A||||||F
OBX|2|CE|10331-7^Rh type^LN||Positive||||||F
OBX|3|NM|718-7^Hemoglobin^LN||12.8|g/dL|12.0-16.0||||F
OBX|4|NM|4544-3^Hematocrit^LN||38.5|%|36.0-44.0||||F
OBX|5|CE|5196-1^Hepatitis B surface antigen^LN||Negative||||||F
OBX|6|CE|5195-3^Hepatitis C antibody^LN||Negative||||||F
OBX|7|CE|5292-8^Rubella antibody^LN||Immune||||||F',
'normal'),

-- Lab Result 5: Pre-operative Panel
('MSG010', 'PID005', 'ORU^R01',
'MSH|^~\&|LAB|HOSPITAL|EZHEALTHKONNECT|DEST|20250118161500||ORU^R01|MSG010|P|2.5
PID|1||PID005^^^HOSPITAL^MR||Brown^James^E||19550720|M
ORC|RE|ORD005|LAB005||CM||||20250118161500|||DOC005^Anderson^Michael
OBR|1|ORD005|LAB005|34528-3^Pre-operative panel^LN|||20250118160000|||||||||DOC005^Anderson^Michael||||||20250118161500|||F
OBX|1|NM|5902-2^PT^LN||12.5|seconds|11.0-13.5||||F
OBX|2|NM|5894-1^INR^LN||1.1||0.8-1.2||||F
OBX|3|NM|3173-2^PTT^LN||28|seconds|25-35||||F
OBX|4|CE|882-1^ABO group^LN||O||||||F
OBX|5|CE|10331-7^Rh type^LN||Positive||||||F
OBX|6|NM|718-7^Hemoglobin^LN||13.5|g/dL|13.5-17.5||||F
OBX|7|NM|777-3^Platelets^LN||280|10*3/uL|150-400||||F',
'normal');

-- Verify the data
SELECT
    'Total Messages: ' || COUNT(*) as summary,
    'ADT^A01: ' || COUNT(*) FILTER (WHERE message_type = 'ADT^A01') as adt_count,
    'ORU^R01: ' || COUNT(*) FILTER (WHERE message_type = 'ORU^R01') as oru_count
FROM test_hl7_messages;

-- Show sample data
SELECT id, message_id, patient_id, message_type, priority, processed, created_at
FROM test_hl7_messages
ORDER BY id;

-- Usage notes for testing:
--
-- TABLE-BASED POLLING:
--   Table Name: test_hl7_messages
--   Incremental Column: updated_at (timestamp) or id (integer)
--   After Processing: update_flag with processed=true
--
-- CUSTOM QUERY EXAMPLES:
--   All unprocessed: SELECT * FROM test_hl7_messages WHERE processed = false
--   By message type: SELECT * FROM test_hl7_messages WHERE message_type = 'ADT^A01'
--   High priority: SELECT * FROM test_hl7_messages WHERE priority IN ('high', 'critical')
--   By patient: SELECT * FROM test_hl7_messages WHERE patient_id = 'PID001'
--
-- AFTER PROCESSING OPTIONS:
--   update_flag: Column=processed, Value=true
--   archive: Archive Table=test_hl7_messages_archive
--   delete: Removes after processing
