-- V59__Interface_Log_Level.sql
-- Replaces the binary debug_logging flag with a granular log_level column.
--
-- Log levels (most → least verbose):
--   debug   — everything: errors, warnings, info, and debug traces  (DEFAULT)
--   info    — errors, warnings, and informational messages
--   warning — errors and warnings only
--   error   — errors only
--   off     — no processing logs written to object storage
--
-- Existing interfaces:
--   • debug_logging = TRUE  → migrated to log_level = 'debug'
--   • debug_logging = FALSE → migrated to log_level = 'debug'
--     (the user asked for "keep it all by default" — new default is debug)
--
-- The debug_logging column is preserved for backward compatibility but is no
-- longer the primary control; it is set to TRUE for all rows and its default
-- is changed to TRUE.  The application reads log_level going forward.

-- 1. Add log_level column
ALTER TABLE interfaces
    ADD COLUMN IF NOT EXISTS log_level VARCHAR(20) NOT NULL DEFAULT 'debug';

COMMENT ON COLUMN interfaces.log_level
    IS 'Per-interface logging verbosity: debug | info | warning | error | off';

-- 2. Set all existing interfaces to debug (log everything by default)
UPDATE interfaces
   SET log_level = 'debug'
 WHERE log_level IS NULL OR log_level = '';

-- 3. Enable debug_logging for ALL existing interfaces so the current code path
--    still works while the Go binary is being updated.
UPDATE interfaces SET debug_logging = TRUE;

-- 4. Change the default for new interfaces to TRUE (was FALSE)
ALTER TABLE interfaces
    ALTER COLUMN debug_logging SET DEFAULT TRUE;

-- 5. Add a CHECK constraint so only known levels are stored
ALTER TABLE interfaces
    DROP CONSTRAINT IF EXISTS chk_interfaces_log_level;
ALTER TABLE interfaces
    ADD CONSTRAINT chk_interfaces_log_level
    CHECK (log_level IN ('debug', 'info', 'warning', 'error', 'off'));
