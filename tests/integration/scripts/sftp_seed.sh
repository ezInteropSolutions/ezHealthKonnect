#!/bin/bash
# Seed SFTP inbound directory with test HL7 files

mkdir -p /home/testuser/upload

cat > /home/testuser/upload/adt_a01_sftp.hl7 << 'EOF'
MSH|^~\&|SEND|SEND_FAC|RECV|RECV_FAC|20240315120000||ADT^A01|SFTP001|P|2.5
PID|1||MRN_SFTP_001^^^HospA^MR||Taylor^Emily^M||19900520|F|||700 Maple Ave^^Seattle^WA^98101
PV1|1|O|OPD^01^A|U
EOF

cat > /home/testuser/upload/oru_r01_sftp.hl7 << 'EOF'
MSH|^~\&|LAB|LAB_FAC|RECV|RECV_FAC|20240315120030||ORU^R01|SFTP002|P|2.5
PID|1||MRN_SFTP_002^^^HospA^MR||Wilson^James||19551230|M
OBR|1|||HBA1C^Hemoglobin A1c
OBX|1|NM|HBA1C^HbA1c||6.2|%|<5.7|H
EOF

chown -R testuser:users /home/testuser/upload
echo "✅ SFTP seed files created"
