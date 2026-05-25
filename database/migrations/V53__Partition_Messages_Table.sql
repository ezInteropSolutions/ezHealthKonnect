-- V53__Partition_Messages_Table.sql
-- Creates the parent `messages` table partitioned by interface_id (LIST partitioning).
-- Attempts to attach existing messages_intf_* standalone tables as partitions.
--
-- PostgreSQL 15 behaviour: ATTACH PARTITION requires the child table to have EXACTLY
-- the same column set as the parent (no extra columns allowed). Because existing
-- interface tables have schema drift (different migration histories produced tables with
-- transformation_applied, mongo_document_id, parsed_at, error_stack, etc.), most
-- existing tables CANNOT be attached and are left as standalone — this is expected and safe.
--
-- Forward-only strategy (Phase 4 outcome):
--   • Parent table `messages` is created — enables partition pruning for future tables.
--   • Existing standalone messages_intf_* tables are unaffected and continue to work.
--   • NEW interfaces created after V53 are created as LIST partitions of messages
--     (interface_message_service.go createAsPartition()).
--   • Schema normalization of existing tables (to allow ATTACH) is a future Phase 4b task.
--
-- Key design decisions:
--   • Parent has NO primary key — existing partitions keep PRIMARY KEY (id) without
--     needing interface_id in their PK.
--   • ATTACH is wrapped in BEGIN/EXCEPTION — truncated names, schema drift, or any other
--     error causes a NOTICE and graceful skip, never an abort.
--   • NOT VALID check constraint added before ATTACH to skip full-table scan.

-- ─────────────────────────────────────────────────────────────────────────────
-- Step 1: Create parent partitioned table
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS messages (
    -- Identity
    id                      UUID         NOT NULL,
    message_id              VARCHAR(255) NOT NULL,
    correlation_id          VARCHAR(255),
    interface_id            UUID         NOT NULL,

    -- Processing status
    status                  VARCHAR(50)  NOT NULL DEFAULT 'received',
    priority                INTEGER               DEFAULT 5,

    -- Timestamps
    received_at             TIMESTAMPTZ           DEFAULT CURRENT_TIMESTAMP,
    processing_started_at   TIMESTAMPTZ,
    processing_completed_at TIMESTAMPTZ,

    -- Source metadata
    source_type             VARCHAR(50),
    source_endpoint         VARCHAR(255),
    source_ip               VARCHAR(45),

    -- Message metadata
    message_type            VARCHAR(100),
    message_size            INTEGER,
    message_encoding        VARCHAR(50)           DEFAULT 'UTF-8',
    raw_message             TEXT,

    -- Processing metrics
    processing_time_ms      BIGINT,

    -- Error tracking
    error_count             INTEGER               DEFAULT 0,
    last_error_message      TEXT,

    -- Delivery
    delivery_status         VARCHAR(50)           DEFAULT 'pending',
    delivery_attempts       INTEGER               DEFAULT 0,

    -- Audit
    created_at              TIMESTAMPTZ           DEFAULT CURRENT_TIMESTAMP,
    updated_at              TIMESTAMPTZ           DEFAULT CURRENT_TIMESTAMP
)
PARTITION BY LIST (interface_id);

-- ─────────────────────────────────────────────────────────────────────────────
-- Step 2: Attach existing standalone tables as partitions
-- ─────────────────────────────────────────────────────────────────────────────
DO $$
DECLARE
    r          RECORD;
    raw_id     TEXT;
    iface_uuid UUID;
    attached   INTEGER := 0;
    skipped    INTEGER := 0;
BEGIN
    FOR r IN
        SELECT tablename
        FROM   pg_tables
        WHERE  schemaname = 'public'
          AND  tablename  LIKE 'messages_intf_%'
        ORDER  BY tablename
    LOOP
        BEGIN
            -- Strip prefix to get the sanitized UUID (underscores in place of dashes)
            raw_id := regexp_replace(r.tablename, '^messages_intf_', '');

            -- Reconstruct UUID by replacing underscores with dashes, then validate via cast
            iface_uuid := replace(raw_id, '_', '-')::UUID;

            -- Skip if already a partition of messages
            IF EXISTS (
                SELECT 1
                FROM   pg_inherits i
                JOIN   pg_class    p ON p.oid = i.inhparent
                JOIN   pg_class    c ON c.oid = i.inhrelid
                WHERE  p.relname = 'messages'
                  AND  c.relname = r.tablename
            ) THEN
                RAISE NOTICE 'Already attached: % — skipping', r.tablename;
                skipped := skipped + 1;
                CONTINUE;
            END IF;

            -- Add a NOT VALID check constraint so ATTACH skips a full table scan.
            -- (All existing rows were inserted with the correct interface_id for this table.)
            EXECUTE format(
                'ALTER TABLE %I ADD CONSTRAINT partition_bound_v53 CHECK (interface_id = %L) NOT VALID',
                r.tablename, iface_uuid
            );

            -- Attach as a list partition of the parent messages table
            EXECUTE format(
                'ALTER TABLE messages ATTACH PARTITION %I FOR VALUES IN (%L)',
                r.tablename, iface_uuid
            );

            attached := attached + 1;
            RAISE NOTICE 'Attached % → interface_id = %', r.tablename, iface_uuid;

        EXCEPTION WHEN OTHERS THEN
            -- Table name doesn't decode to a valid UUID, schema mismatch, etc. — skip silently
            RAISE NOTICE 'Skipping % (raw=%): %', r.tablename, raw_id, SQLERRM;
            skipped := skipped + 1;

            -- Best-effort: remove the constraint we may have added before the failure
            BEGIN
                EXECUTE format(
                    'ALTER TABLE %I DROP CONSTRAINT IF EXISTS partition_bound_v53',
                    r.tablename
                );
            EXCEPTION WHEN OTHERS THEN
                NULL;
            END;
        END;
    END LOOP;

    RAISE NOTICE 'V53 complete — attached: %, skipped: %', attached, skipped;
END;
$$;
