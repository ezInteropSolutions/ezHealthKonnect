#!/bin/bash
# SQL Server test seed — runs after the container is healthy.
# The healthcheck in docker-compose.test.yml waits for SQL Server to accept connections
# before this script is executed.
#
# Usage (called manually or from a CI step):
#   docker exec ehk_test_sqlserver bash /sqlserver_seed.sh

set -e

SQLCMD="/opt/mssql-tools18/bin/sqlcmd"
SERVER="localhost"
USER="sa"
PASS="TestPassword123!"
DB="testdb"

echo "⏳ Waiting for SQL Server to be ready..."
for i in $(seq 1 30); do
    if $SQLCMD -S $SERVER -U $USER -P "$PASS" -No -Q "SELECT 1" >/dev/null 2>&1; then
        echo "✅ SQL Server is ready"
        break
    fi
    echo "  Attempt $i/30 — waiting 3s..."
    sleep 3
done

echo "📦 Creating test database and seeding data..."

$SQLCMD -S $SERVER -U $USER -P "$PASS" -No -Q "
IF NOT EXISTS (SELECT name FROM sys.databases WHERE name = N'$DB')
BEGIN
    CREATE DATABASE [$DB];
END
"

$SQLCMD -S $SERVER -U $USER -P "$PASS" -No -d "$DB" -Q "
-- Create test login and user
IF NOT EXISTS (SELECT name FROM sys.server_principals WHERE name = 'testuser')
BEGIN
    CREATE LOGIN testuser WITH PASSWORD = 'TestPassword123!';
END
IF NOT EXISTS (SELECT name FROM sys.database_principals WHERE name = 'testuser')
BEGIN
    CREATE USER testuser FOR LOGIN testuser;
    ALTER ROLE db_owner ADD MEMBER testuser;
END

-- HL7 inbox table for connector polling tests
IF NOT EXISTS (SELECT * FROM sysobjects WHERE name='hl7_inbox' AND xtype='U')
BEGIN
    CREATE TABLE hl7_inbox (
        id           INT IDENTITY(1,1) PRIMARY KEY,
        message_type NVARCHAR(50)   NOT NULL DEFAULT 'ADT^A01',
        raw_message  NVARCHAR(MAX)  NOT NULL,
        received_at  DATETIME2      NOT NULL DEFAULT GETDATE(),
        processed    BIT            NOT NULL DEFAULT 0,
        source_ip    NVARCHAR(50)   NULL,
        INDEX idx_processed (processed),
        INDEX idx_received_at (received_at)
    );
END

-- Truncate and reseed on every run
TRUNCATE TABLE hl7_inbox;

INSERT INTO hl7_inbox (message_type, raw_message, processed) VALUES
(
    'ADT^A01',
    'MSH|^~\&|SEND|SEND_FAC|RECV|RECV_FAC|20240315120000||ADT^A01|SQLMSG1001|P|2.5\rPID|1||MRN_SS_001^^^HospA^MR||Smith^John^M||19800101|M|||100 Main St^^Anytown^IL^60601\r',
    0
),
(
    'ADT^A01',
    'MSH|^~\&|SEND|SEND_FAC|RECV|RECV_FAC|20240315120005||ADT^A01|SQLMSG1002|P|2.5\rPID|1||MRN_SS_002^^^HospA^MR||Doe^Jane^F||19750420|F|||200 Oak Ave^^Springfield^IL^62701\r',
    0
),
(
    'ORU^R01',
    'MSH|^~\&|LAB|LAB_FAC|RECV|RECV_FAC|20240315120010||ORU^R01|SQLMSG1003|P|2.5\rPID|1||MRN_SS_001^^^HospA^MR||Smith^John^M||19800101|M\rOBR|1|||CBC^Complete Blood Count\rOBX|1|NM|WBC^White Blood Cells||7.5|K/uL|4.5-11.0|N\r',
    0
),
(
    'ADT^A01',
    'MSH|^~\&|SEND|SEND_FAC|RECV|RECV_FAC|20240315120015||ADT^A01|SQLMSG1004|P|2.5\rPID|1||MRN_SS_003^^^HospA^MR||Williams^Robert||19900615|M|||300 Elm St^^Chicago^IL^60601\r',
    1
);
"

echo "✅ SQL Server seed complete (4 rows in hl7_inbox, testdb)"
