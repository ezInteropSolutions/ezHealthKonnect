-- V288: per-interface processing order.
--
-- 'parallel' (the default, and every interface's behaviour before this
-- column existed): up to the worker pool's size of messages are processed at
-- once, and each pipeline runs in its own goroutine, so messages can finish
-- and be delivered in a different order than they arrived.
--
-- 'in_order': one message at a time, in arrival order; a message's pipeline
-- must finish before the next one starts. For feeds where order carries
-- meaning (an ADT merge must not overtake the registration it merges).
-- Read by the Go engine at interface activation (processing/engine_ordering.go).

ALTER TABLE interfaces
    ADD COLUMN IF NOT EXISTS processing_order TEXT NOT NULL DEFAULT 'parallel';

ALTER TABLE interfaces
    DROP CONSTRAINT IF EXISTS interfaces_processing_order_check;

ALTER TABLE interfaces
    ADD CONSTRAINT interfaces_processing_order_check
    CHECK (processing_order IN ('parallel', 'in_order'));

COMMENT ON COLUMN interfaces.processing_order IS
    'parallel (default) or in_order: one message at a time, in arrival order';
