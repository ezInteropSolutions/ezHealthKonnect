-- V31: Add deleted_at column for soft delete tracking
-- This enables proper audit trail for deleted interfaces

ALTER TABLE interfaces
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP WITH TIME ZONE;

COMMENT ON COLUMN interfaces.deleted_at IS 'Timestamp when interface was soft-deleted (NULL = not deleted)';

-- Create index for querying deleted interfaces
CREATE INDEX IF NOT EXISTS idx_interfaces_deleted_at
ON interfaces(deleted_at)
WHERE deleted_at IS NOT NULL;

-- Create index for active interfaces (frequently queried)
CREATE INDEX IF NOT EXISTS idx_interfaces_active
ON interfaces(user_id, is_active, deleted_at)
WHERE is_active = true AND deleted_at IS NULL;
