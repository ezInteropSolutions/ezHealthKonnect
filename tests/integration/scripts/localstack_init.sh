#!/bin/bash
# LocalStack initialization — creates S3 test bucket and uploads seed HL7 files

echo "🪣 LocalStack init: creating test S3 bucket..."
awslocal s3 mb s3://ehk-test-bucket --region us-east-1

echo "📤 Uploading seed HL7 files..."
cat > /tmp/test_adt_a01.hl7 << 'EOF'
MSH|^~\&|SEND|SEND_FAC|RECV|RECV_FAC|20240315120000||ADT^A01|S3MSG001|P|2.5
PID|1||MRN_S3_001^^^HospA^MR||Brown^Alice^M||19851010|F|||500 Pine St^^Boston^MA^02101
PV1|1|I|MED^12^B|U|||654321^Jones^Robert^J|||INT
EOF

cat > /tmp/test_oru_r01.hl7 << 'EOF'
MSH|^~\&|LAB|LAB_FAC|RECV|RECV_FAC|20240315120010||ORU^R01|S3MSG002|P|2.5
PID|1||MRN_S3_002^^^HospA^MR||Green^Charlie||19720315|M
OBR|1|||LIPID^Lipid Panel
OBX|1|NM|CHOL^Cholesterol||185|mg/dL|<200|N
OBX|2|NM|TRIG^Triglycerides||120|mg/dL|<150|N
EOF

awslocal s3 cp /tmp/test_adt_a01.hl7 s3://ehk-test-bucket/inbound/test_adt_a01.hl7
awslocal s3 cp /tmp/test_oru_r01.hl7  s3://ehk-test-bucket/inbound/test_oru_r01.hl7

echo "✅ LocalStack init complete. Bucket contents:"
awslocal s3 ls s3://ehk-test-bucket/inbound/
