-- V30: Add Interface-Level Logging Configuration
-- Description: Adds debug logging and log retention settings to interfaces table

-- Add logging configuration columns
ALTER TABLE interfaces
ADD COLUMN IF NOT EXISTS debug_logging BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS log_retention_days INTEGER DEFAULT 30,
ADD COLUMN IF NOT EXISTS retain_error_logs_forever BOOLEAN DEFAULT TRUE;

-- Add comments for documentation
COMMENT ON COLUMN interfaces.debug_logging IS 'When true, captures detailed debug logs for all operations. When false, only captures errors and warnings.';
COMMENT ON COLUMN interfaces.log_retention_days IS 'Number of days to retain debug/info logs. Error logs retention controlled separately.';
COMMENT ON COLUMN interfaces.retain_error_logs_forever IS 'When true, error logs are never deleted. When false, error logs follow log_retention_days policy.';

-- Add index for log cleanup queries
CREATE INDEX IF NOT EXISTS idx_interfaces_log_retention
ON interfaces(debug_logging, log_retention_days)
WHERE status = 'active';
