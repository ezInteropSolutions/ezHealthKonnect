-- Load Mickey Mouse sample message into sample_parsed_messages table
-- This will enable autocomplete to show all fields including "version"

-- First, delete any existing ADT^A04 v2.3 samples to avoid duplicates
DELETE FROM sample_parsed_messages
WHERE message_type = 'ADT^A04' AND hl7_version = '2.3';

-- Insert the sample (using the enhancedSegments structure from the sample interface message)
-- This INSERT will be populated by querying an existing parsed message
INSERT INTO sample_parsed_messages (message_type, hl7_version, format, parsed_content, description, is_active)
SELECT
    'ADT^A04' as message_type,
    '2.3' as hl7_version,
    'hl7v2' as format,
    parsed_content,
    'Mickey Mouse sample message (ADT^A04 v2.3) - Copied from interface messages' as description,
    true as is_active
FROM raw_messages_intf_762aebb9_0408_4a42_82c5_202f13f28315
WHERE parsed_content IS NOT NULL
  AND parsed_content::text LIKE '%MICKEY%'
LIMIT 1;

-- Verify the insert
SELECT
    id,
    message_type,
    hl7_version,
    description,
    jsonb_object_keys(parsed_content) as segments,
    created_at
FROM sample_parsed_messages
WHERE message_type = 'ADT^A04' AND hl7_version = '2.3';
